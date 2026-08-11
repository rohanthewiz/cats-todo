package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// newTestModel builds a manager model with empty project and global backlogs and
// no cats control socket, suitable for driving Update directly. The project store gets a
// path so available() is true — matching a real in-project launch, the context
// under which the startup crash reproduced.
func newTestModel() model {
	project := &store{scope: scopeProject, path: "/tmp/cats-todo-test/project/todos.json"}
	global := &store{scope: scopeGlobal, path: "/tmp/cats-todo-test/global/todos.json"}
	return newModel(RunContext{WorkDir: "/tmp/cats-todo-test/project"}, project, global, nil)
}

// TestWindowSizeMsgNeverPanics is a regression test (inherited from herdr-todo)
// for a bug that closed the manager pane on launch: applySizes resized the
// form's textarea/textinput and the
// target picker unconditionally, but those are zero-value until their stage is
// entered (built in beginAdd/beginEdit/beginDrop). The first WindowSizeMsg lands
// while the model is still on the list, so applySizes called textarea.SetWidth on
// a nil-initialized model and panicked — Bubble Tea unwound, the process exited,
// and cats tore the pane down. A WindowSizeMsg must be safe on every stage,
// whether or not that stage's inputs have been built yet.
func TestWindowSizeMsgNeverPanics(t *testing.T) {
	// A width whose usable area (width-4) clears applySizes' w >= 20 guard, so the
	// resize actually reaches the inputs rather than returning early.
	resize := tea.WindowSizeMsg{Width: 120, Height: 40}

	t.Run("list stage (where the first resize lands)", func(t *testing.T) {
		m := newTestModel()
		if m.stage != stageList {
			t.Fatalf("initial stage = %v, want stageList", m.stage)
		}
		m.Update(resize) // before the fix this panicked on the zero-value textarea
	})

	t.Run("form stage (textarea built and resizable)", func(t *testing.T) {
		next, _ := newTestModel().beginAdd()
		m := next.(model)
		if m.stage != stageForm {
			t.Fatalf("beginAdd stage = %v, want stageForm", m.stage)
		}
		m.Update(resize)
	})

	t.Run("target stage (picker built without a socket)", func(t *testing.T) {
		m := newTestModel()
		// beginDrop needs a control socket, so build the picker the way it would but
		// with a nil client (buildTargets degrades to just the new-session target).
		m.targets, m.targetList = m.buildTargets()
		m.stage = stageTarget
		m.Update(resize)
	})
}

// pressList sends one key to the list stage and returns the model that came
// back, so a test can drive the manager the way a user does.
func pressList(t *testing.T, m model, key string) model {
	t.Helper()
	next, _ := m.updateList(pressKey(key))
	return next.(model)
}

// withTodo returns a test model whose project backlog holds one prompt, with
// the list rebuilt so a row is highlighted. The store is populated in memory —
// these tests are about the action bar, not about persistence.
func withTodo(title string) model {
	m := newTestModel()
	m.project.todos = []Todo{{ID: "t1", Title: title, Prompt: "do the thing"}}
	m.rebuildList()
	return m
}

// TestActionFocusRing pins the tab ring: the focus starts in the query box,
// walks the buttons in order, and comes back to the box rather than sticking on
// the last button. Typing must land the focus back in the box too — a lit
// button while characters go into the filter would show the focus in one place
// and put it in another.
func TestActionFocusRing(t *testing.T) {
	m := newTestModel()
	if m.actionFocus {
		t.Fatal("a fresh manager must start in the filter, not on a button")
	}

	n := len(m.listActions())
	for i := 0; i < n; i++ {
		m = pressList(t, m, "tab")
		if !m.actionFocus || m.actionIdx != i {
			t.Fatalf("tab %d: focus=%v idx=%d, want button %d", i+1, m.actionFocus, m.actionIdx, i)
		}
	}
	if m = pressList(t, m, "tab"); m.actionFocus {
		t.Fatal("tab past the last button must return to the filter")
	}

	// shift+tab off the query box wraps to the far end of the bar.
	m = pressList(t, m, "shift+tab")
	if !m.actionFocus || m.actionIdx != n-1 {
		t.Fatalf("shift+tab: focus=%v idx=%d, want the last button", m.actionFocus, m.actionIdx)
	}

	m = pressList(t, m, "x")
	if m.actionFocus {
		t.Fatal("typing must hand the focus back to the filter")
	}
	if m.list.input.Value() != "x" {
		t.Fatalf("filter = %q, want the typed character", m.list.input.Value())
	}
}

// TestActionButtonPress checks that enter on a focused button runs that
// button's action rather than the bare-enter behaviour.
func TestActionButtonPress(t *testing.T) {
	t.Run("add opens the form", func(t *testing.T) {
		m := pressList(t, newTestModel(), "tab") // onto Add
		m = pressList(t, m, "enter")
		if m.stage != stageForm || m.formMode != formAdd {
			t.Fatalf("stage=%v mode=%v, want the add form", m.stage, m.formMode)
		}
	})

	t.Run("delete arms the confirm", func(t *testing.T) {
		m := withTodo("ship it")
		for i := 0; i <= actionDelete; i++ {
			m = pressList(t, m, "tab")
		}
		if m.actionIdx != actionDelete {
			t.Fatalf("idx=%d, want the delete button", m.actionIdx)
		}
		m = pressList(t, m, "enter")
		if m.stage != stageConfirm || m.confirmKind != confirmDelete {
			t.Fatalf("stage=%v kind=%v, want the delete confirm", m.stage, m.confirmKind)
		}
	})

	// The begin* helpers return silently with nothing highlighted, which on a
	// button press reads as a dead control — the bar has to say why instead.
	t.Run("selection-less action explains itself", func(t *testing.T) {
		m := newTestModel() // empty backlogs: nothing to highlight
		for i := 0; i <= actionEdit; i++ {
			m = pressList(t, m, "tab")
		}
		m = pressList(t, m, "enter")
		if m.stage != stageList {
			t.Fatalf("stage=%v, want to stay on the list", m.stage)
		}
		if m.status == "" {
			t.Fatal("pressing an unavailable button must report why nothing happened")
		}
	})
}

// TestActionBarEscape pins esc's order of business on the list: leave the
// button bar, then clear the filter, and only then quit.
func TestActionBarEscape(t *testing.T) {
	m := newTestModel()
	m.list.input.SetValue("query")
	m = pressList(t, m, "tab")

	m = pressList(t, m, "esc")
	if m.actionFocus {
		t.Fatal("esc on a button must return the focus to the filter")
	}
	if m.list.input.Value() != "query" {
		t.Fatal("esc leaving the bar must not also clear the filter")
	}
	if m.quitting {
		t.Fatal("esc leaving the bar must not quit")
	}

	if m = pressList(t, m, "esc"); m.list.input.Value() != "" || m.quitting {
		t.Fatalf("second esc should clear the filter and stay open (value=%q quitting=%v)",
			m.list.input.Value(), m.quitting)
	}
	if m = pressList(t, m, "esc"); !m.quitting {
		t.Fatal("esc with nothing left to clear should quit")
	}
}

// TestActionBarRender checks what the bar shows as the pane narrows: every label
// with its key hint when there is room, labels alone when there is not, and the
// glyphs alone when even those don't fit. A button is never dropped — the bar is
// where two of these actions are learned at all.
func TestActionBarRender(t *testing.T) {
	m := withTodo("ship it")

	m.width = 200
	wide := m.actionBar()
	for _, a := range m.listActions() {
		if !strings.Contains(wide, a.label) {
			t.Fatalf("wide bar %q is missing %q", wide, a.label)
		}
		if !strings.Contains(wide, a.hint) {
			t.Fatalf("wide bar %q is missing the %q hint", wide, a.hint)
		}
	}

	m.width = 45
	narrow := m.actionBar()
	if got := m.barTier(); got != tierLabels {
		t.Fatalf("a 45-column bar is at tier %d, want labels alone", got)
	}
	for _, a := range m.listActions() {
		if !strings.Contains(narrow, a.label) {
			t.Fatalf("narrow bar %q dropped the label %q", narrow, a.label)
		}
	}
	if strings.Contains(narrow, "ctrl+a") {
		t.Fatalf("narrow bar %q should drop the hints, not wrap", narrow)
	}

	m.width = 30
	tiny := m.actionBar()
	if got := m.barTier(); got != tierIcons {
		t.Fatalf("a 30-column bar is at tier %d, want the glyphs alone", got)
	}
	for _, a := range m.listActions() {
		if !strings.Contains(tiny, a.icon()) {
			t.Fatalf("tiny bar %q dropped the %q button entirely", tiny, a.label)
		}
	}
	// The words are what a 30-column pane cannot afford; a bar that kept them
	// would wrap, and a wrapped chip is one the pointer can no longer find.
	if strings.Contains(tiny, "Add") {
		t.Fatalf("tiny bar %q kept its labels — it will wrap", tiny)
	}
	if w := lipgloss.Width(tiny); w > 30 {
		t.Fatalf("tiny bar is %d cells wide in a 30-cell pane: %q", w, tiny)
	}
}

// TestListViewShowsActionBar guards the wiring: the bar has to reach the
// rendered list, between the filter line and the rows.
func TestListViewShowsActionBar(t *testing.T) {
	m := withTodo("ship it")
	m.width = 200
	out := m.viewList()

	bar := strings.Index(out, "Add")
	row := strings.Index(out, "ship it")
	if bar < 0 || row < 0 {
		t.Fatalf("list view is missing the bar (%d) or the row (%d):\n%s", bar, row, out)
	}
	if bar > row {
		t.Fatalf("the action bar must sit above the rows, not below them:\n%s", out)
	}
}

// TestActionBarRow guards the constant a click's Y is compared against. The
// hit test can't re-measure the frame, so if anything above the bar ever grows
// a line, this is what says so.
func TestActionBarRow(t *testing.T) {
	m := withTodo("ship it")
	m.width = 200
	lines := strings.Split(m.viewList(), "\n")
	if actionBarRow >= len(lines) {
		t.Fatalf("actionBarRow = %d, but the view has %d lines", actionBarRow, len(lines))
	}
	if !strings.Contains(lines[actionBarRow], "Add") {
		t.Fatalf("row %d is %q, want the action bar:\n%s", actionBarRow, lines[actionBarRow], m.viewList())
	}
}

// TestActionBarIconsAreOneCell pins the icons to single-column glyphs. The
// double-width emoji forms were drawn clipped by the terminal — half an arrow,
// half a cross — and no layout width fixes that, so the fix has to be the
// glyph. It also keeps the chip spans honest for the click hit test.
func TestActionBarIconsAreOneCell(t *testing.T) {
	for _, a := range withTodo("ship it").listActions() {
		icon := string([]rune(a.label)[0])
		if w := lipgloss.Width(icon); w != 1 {
			t.Errorf("%q icon %q is %d cells wide, want a one-cell dingbat", a.label, icon, w)
		}
	}
}

// TestActionBarClick drives the bar with the pointer: a click inside a chip
// runs that chip's action and leaves the keyboard focus on it, and a click that
// misses every chip does nothing at all.
func TestActionBarClick(t *testing.T) {
	build := func() model {
		m := withTodo("ship it")
		m.width = 200
		return m
	}

	click := func(t *testing.T, m model, x, y int) model {
		t.Helper()
		next, _ := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
		return next.(model)
	}

	t.Run("a chip runs its action and takes the focus", func(t *testing.T) {
		m := build()
		chips := m.actionChips()
		got := click(t, m, chips[actionEdit].start+1, actionBarRow)
		if got.stage != stageForm || got.formMode != formEdit {
			t.Fatalf("clicking Edit gave stage=%v mode=%v, want the edit form", got.stage, got.formMode)
		}
		if !got.actionFocus || got.actionIdx != actionEdit {
			t.Fatalf("focus=%v idx=%d, want the focus parked on Edit", got.actionFocus, got.actionIdx)
		}
	})

	t.Run("Add works with nothing highlighted", func(t *testing.T) {
		m := newTestModel()
		m.width = 200
		got := click(t, m, m.actionChips()[actionAdd].start+1, actionBarRow)
		if got.stage != stageForm || got.formMode != formAdd {
			t.Fatalf("clicking Add gave stage=%v mode=%v, want the add form", got.stage, got.formMode)
		}
	})

	t.Run("a miss changes nothing", func(t *testing.T) {
		m := build()
		chips := m.actionChips()
		// The gap between two chips, the row above the bar, and the empty run
		// past the last chip: all outside every button.
		for _, at := range [][2]int{
			{chips[0].end, actionBarRow},
			{chips[0].start + 1, actionBarRow - 1},
			{chips[len(chips)-1].end + 5, actionBarRow},
			{0, actionBarRow},
		} {
			got := click(t, m, at[0], at[1])
			if got.stage != stageList || got.actionFocus {
				t.Fatalf("click at %v gave stage=%v focus=%v, want the list untouched", at, got.stage, got.actionFocus)
			}
		}
	})

	t.Run("Send with no socket says so and stays put", func(t *testing.T) {
		// Send is the pointer's only way out to an agent now that a double-click
		// edits, so the launched-outside-cats path has to answer on the chip.
		m := build() // no client, as when launched outside cats
		got := click(t, m, m.actionChips()[actionSend].start+1, actionBarRow)
		if got.stage != stageList || got.status == "" {
			t.Fatalf("stage=%v status=%q, want the list with an explanation", got.stage, got.status)
		}
	})

	t.Run("a needsSel button with no selection only says so", func(t *testing.T) {
		m := newTestModel()
		m.width = 200
		got := click(t, m, m.actionChips()[actionDelete].start+1, actionBarRow)
		if got.stage != stageList {
			t.Fatalf("stage = %v, want to stay on the list with nothing to delete", got.stage)
		}
		if got.status == "" {
			t.Fatal("an inert button must explain itself in the status line")
		}
	})
}

// TestActionFocusReleasedOnReturn is a regression test for the bug where enter
// on a highlighted prompt opened a blank new todo. Entering a stage leaves the
// bar's focus on the chip that took the user there — deliberately, so pointer
// and keyboard agree — but coming back left it parked there with nothing
// running. With Add still lit, the next bare enter pressed Add again instead of
// editing the prompt under the highlight.
func TestActionFocusReleasedOnReturn(t *testing.T) {
	// Every way into another stage, and the key that walks back out of it.
	for _, tc := range []struct {
		name string
		open func(model) model
		back func(model) model
	}{
		{
			name: "add form, escaped",
			open: func(m model) model { return clickChip(t, m, actionAdd) },
			back: func(m model) model { return stepForm(t, m, "esc") },
		},
		{
			name: "delete confirm, declined",
			open: func(m model) model { return clickChip(t, m, actionDelete) },
			back: func(m model) model {
				next, _ := m.updateConfirm(pressKey("n"))
				return next.(model)
			},
		},
		{
			// The keyboard falls into the same trap: one tab lands on Add.
			name: "add form, reached by tab",
			open: func(m model) model { return pressList(t, pressList(t, m, "tab"), "enter") },
			back: func(m model) model { return stepForm(t, m, "esc") },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := withTodo("ship it")
			m.width = 200
			m = tc.back(tc.open(m))
			if m.stage != stageList {
				t.Fatalf("stage = %v, want to be back on the list", m.stage)
			}
			if m.actionFocus {
				t.Fatalf("the bar kept the focus on button %d after the action finished", m.actionIdx)
			}
			// The point of releasing it: enter means "open what's highlighted".
			m = pressList(t, m, "enter")
			if m.stage != stageForm || m.formMode != formEdit {
				t.Fatalf("enter gave stage=%v mode=%v, want the highlighted prompt's edit form", m.stage, m.formMode)
			}
		})
	}
}

// TestQueryBoxShowsItsFocus covers the signal the query box gives about holding
// the keys. It was focused once at construction and never blurred, so it blinked
// a cursor the whole time a button was lit — the box claimed the focus it didn't
// have, which left handing the focus back with nothing to show for itself.
//
// The input's own Focused() is what the rails are drawn from (see
// fuzzyList.view), so asserting on it covers both.
func TestQueryBoxShowsItsFocus(t *testing.T) {
	build := func() model {
		m := withTodo("ship it")
		m.width = 200
		return m
	}

	t.Run("a lit chip takes the box's cursor", func(t *testing.T) {
		m := build()
		if !m.list.input.Focused() {
			t.Fatal("the box must start focused — it is where typing goes")
		}
		if m = pressList(t, m, "tab"); m.list.input.Focused() {
			t.Fatal("a lit chip must blur the box, or it claims a focus it hasn't got")
		}
	})

	t.Run("every way back refocuses it", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			back func(model) model
		}{
			{"esc", func(m model) model { return pressList(t, m, "esc") }},
			{"tab around the ring", func(m model) model {
				for range m.listActions() {
					m = pressList(t, m, "tab")
				}
				return m
			}},
			{"shift+tab", func(m model) model { return pressList(t, m, "shift+tab") }},
			{"clicking a row", func(m model) model {
				next, _ := m.Update(tea.MouseClickMsg{X: 8, Y: listRowsRow, Button: tea.MouseLeft})
				return next.(model)
			}},
			{"closing a form", func(m model) model {
				return stepForm(t, pressList(t, m, "enter"), "esc")
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				m := tc.back(pressList(t, build(), "tab"))
				if m.actionFocus {
					t.Fatalf("the bar kept the focus on button %d", m.actionIdx)
				}
				if !m.list.input.Focused() {
					t.Fatal("the focus came back to a box that still looks blurred")
				}
			})
		}
	})

	t.Run("the key that brings the focus home is not swallowed", func(t *testing.T) {
		// A blurred input drops the keys it is given, so typing from a lit chip
		// has to refocus the box before the character is handed over — not after.
		m := pressList(t, build(), "tab")
		m = pressList(t, m, "x")
		if got := m.list.input.Value(); got != "x" {
			t.Fatalf("query = %q, want the keystroke that returned the focus to have landed", got)
		}
		if !m.list.input.Focused() || m.actionFocus {
			t.Fatal("typing must leave the focus in the box the characters went to")
		}
	})
}

// clickChip presses action bar button i with the pointer.
func clickChip(t *testing.T, m model, i int) model {
	t.Helper()
	next, _ := m.Update(tea.MouseClickMsg{X: m.actionChips()[i].start + 1, Y: actionBarRow, Button: tea.MouseLeft})
	return next.(model)
}

// stepForm sends one key to the form stage.
func stepForm(t *testing.T, m model, key string) model {
	t.Helper()
	next, _ := m.updateForm(pressKey(key))
	return next.(model)
}

// TestMouseReportingScopedToList pins where the pointer is claimed. The bar is
// the only clickable thing, and mouse reporting costs the pane its own
// click-to-select — so the prompt view, the screen whose text gets copied out,
// must not turn it on.
func TestMouseReportingScopedToList(t *testing.T) {
	m := withTodo("ship it")
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Errorf("list stage MouseMode = %v, want cell motion so the bar is clickable", got)
	}
	m.stage = stageTarget
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Errorf("target picker MouseMode = %v, want cell motion so the rows are clickable", got)
	}
	m.stage = stageView
	if got := m.View().MouseMode; got != tea.MouseModeNone {
		t.Errorf("prompt view MouseMode = %v, want none so selection still works", got)
	}
}

// withTargets returns a model sitting in the drop picker over a todo, with n
// fake agent targets to choose from — enough rows that a click has to land on
// the right one rather than on the only one.
func withTargets(n int) model {
	m := withTodo("ship it")
	m.width = 200
	m.dropTodo = m.rows[0]
	m.targets = make([]dropTarget, n)
	items := make([]listItem, n)
	for i := range m.targets {
		m.targets[i] = dropTarget{
			kind:  targetExistingPane,
			pane:  uint32(i + 1),
			agent: fmt.Sprintf("agent%d", i),
		}
		items[i] = listItem{name: m.targets[i].agent, desc: "/work", selectable: true, ref: i}
	}
	m.targetList = newFuzzyList("Filter targets…", items)
	m.stage = stageTarget
	return m
}

// TestTargetRowsAreClickable covers the pointer in the drop picker: a click on a
// row picks that agent and drops into it, the same as enter on the row, and the
// highlight moves there first so the keyboard resumes from what was clicked.
func TestTargetRowsAreClickable(t *testing.T) {
	click := func(m model, y int) model {
		next, _ := m.Update(tea.MouseClickMsg{X: 6, Y: y, Button: tea.MouseLeft})
		return next.(model)
	}

	t.Run("a row drops into its agent", func(t *testing.T) {
		got := click(withTargets(3), targetRowsRow+2) // the third target
		if got.targetList.cursor != 2 {
			t.Fatalf("cursor = %d, want the clicked row highlighted", got.targetList.cursor)
		}
		if !got.dropping {
			t.Fatal("clicking a target must start the drop, as enter does")
		}
		if !strings.Contains(got.status, "agent2") {
			t.Fatalf("status = %q, want the clicked agent named", got.status)
		}
		if got.stage != stageList {
			t.Fatalf("stage = %v, want the list back while the drop runs", got.stage)
		}
	})

	t.Run("the whole row answers, not just its label", func(t *testing.T) {
		got := click(withTargets(3), targetRowsRow) // far right of the first row
		if !got.dropping || got.targetList.cursor != 0 {
			t.Fatalf("dropping=%v cursor=%d, want the first row taken", got.dropping, got.targetList.cursor)
		}
	})

	t.Run("a click off the rows does nothing", func(t *testing.T) {
		for _, y := range []int{0, targetRowsRow - 1, targetRowsRow + 3} {
			got := click(withTargets(3), y)
			if got.stage != stageTarget || got.dropping {
				t.Fatalf("click at y=%d gave stage=%v dropping=%v, want the picker untouched", y, got.stage, got.dropping)
			}
		}
	})

	t.Run("a filtered-away row can't be clicked into", func(t *testing.T) {
		m := withTargets(3)
		m.targetList.input.SetValue("agent2")
		m.targetList.filter()
		// One row is left, and it is the one the click lands on — the row's
		// position on screen decides, not the target's index.
		got := click(m, targetRowsRow)
		if !strings.Contains(got.status, "agent2") {
			t.Fatalf("status = %q, want the only matching agent", got.status)
		}
		if got = click(m, targetRowsRow+1); got.dropping {
			t.Fatal("a click below the last visible row must not drop")
		}
	})
}

// withBothScopes returns a model holding two project prompts and one global
// one, so the list carries what a click has to see past: the two group headings
// and the blank spacer each one opens with.
func withBothScopes() model {
	m := newTestModel()
	m.project.todos = []Todo{
		{ID: "p1", Title: "first", Prompt: "p"},
		{ID: "p2", Title: "second", Prompt: "p"},
	}
	m.global.todos = []Todo{{ID: "g1", Title: "third", Prompt: "p"}}
	m.width = 200
	m.rebuildList()
	return m
}

// TestSelectedRowIsHighlighted covers the field drawn behind the highlighted
// row. The width assertion is the load-bearing one: a background only covers the
// cells a style writes, so a row whose highlight stops at its text is the exact
// failure this is guarding, and it renders as a box around the words instead of
// a lit row.
//
// The done cases are here because they were the bug. strike used to win over
// selected outright, so a completed row under the cursor drew identically to
// one anywhere else — the highlight did not exist for half the list.
func TestSelectedRowIsHighlighted(t *testing.T) {
	const width = 64

	build := func() model {
		m := newTestModel()
		m.project.todos = []Todo{
			{ID: "p1", Title: "still open", Prompt: "a"},
			{ID: "p2", Title: "already done", Prompt: "b", Done: true},
		}
		m.width = width
		m.rebuildList()
		return m
	}
	// fullWidth counts the rendered lines that run the whole pane — the lit row
	// and nothing else, since every other line stops at its text. rowsView is
	// what the list stage actually draws (the query line lives on the header).
	fullWidth := func(m model) int {
		n := 0
		for ln := range strings.SplitSeq(m.list.rowsView("none", width), "\n") {
			if lipgloss.Width(ln) == width {
				n++
			}
		}
		return n
	}

	for _, tc := range []struct {
		name string
		row  int
	}{
		{"an open row", 0},
		{"a completed row", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := build()
			m.list.cursor = tc.row
			if got := fullWidth(m); got != 1 {
				t.Fatalf("%d full-width lines, want exactly the highlighted row", got)
			}
		})
	}

	t.Run("a completed row answers the cursor", func(t *testing.T) {
		// Same name, same strike, different selection: the two must not render
		// alike, or the cursor is invisible on everything already finished.
		on := highlightName("already done", nil, true, true, false)
		off := highlightName("already done", nil, false, true, false)
		if on == off {
			t.Fatal("a selected done row renders exactly like an unselected one")
		}
	})

	t.Run("a frozen row recedes without a strike", func(t *testing.T) {
		// The dim tier says "not active work"; the strike would say "this was
		// carried out", which is the one thing freezing must never claim.
		got := highlightName("not doing this", nil, false, false, true)
		if strings.Contains(got, "\x1b[9m") || strings.Contains(got, ";9m") {
			t.Fatalf("strikethrough in %q — a frozen row must not read as done", got)
		}
		if got == highlightName("not doing this", nil, false, false, false) {
			t.Fatal("a frozen row renders exactly like an open one")
		}
	})

	t.Run("done and selected both survive", func(t *testing.T) {
		// Neither wins outright: the strike says what the todo is, the field
		// says which row the keys are on.
		got := highlightName("already done", nil, true, true, false)
		if !strings.Contains(got, "\x1b[9m") && !strings.Contains(got, ";9m") {
			t.Fatalf("no strikethrough in %q — selection swallowed the done marking", got)
		}
		if !strings.Contains(got, "48;2;") {
			t.Fatalf("no field in %q — done marking swallowed the selection", got)
		}
	})
}

// TestListRowsAreClickable covers the pointer on the todo list: one click
// selects a row, a second opens it for editing. The split matters — the bar
// acts on the highlight, so a single click that opened the edit form would make
// "click the prompt, then click Send" impossible to say with the pointer.
func TestListRowsAreClickable(t *testing.T) {
	click := func(m model, y int) model {
		next, _ := m.Update(tea.MouseClickMsg{X: 8, Y: y, Button: tea.MouseLeft})
		return next.(model)
	}
	// rowLine is the screen line of the nth selectable row, headings included.
	rowLine := func(m model, n int) int {
		seen := 0
		for line := 0; line < 40; line++ {
			if i, ok := m.list.rowAtLine(line); ok {
				if seen == n {
					_ = i
					return listRowsRow + line
				}
				seen++
			}
		}
		t.Fatalf("row %d is not drawn", n)
		return -1
	}

	t.Run("one click selects, and only selects", func(t *testing.T) {
		m := withBothScopes()
		got := click(m, rowLine(m, 2)) // the global prompt, past both headings
		if got.stage != stageList {
			t.Fatalf("stage = %v, want to stay on the list", got.stage)
		}
		ref, ok := got.selectedRef()
		if !ok || ref.id != "g1" {
			t.Fatalf("selected %+v (ok=%v), want the clicked global prompt", ref, ok)
		}
	})

	t.Run("selecting takes the focus off the bar", func(t *testing.T) {
		m := withBothScopes()
		m = pressList(t, m, "tab") // onto Add, where a stale focus used to strand it
		got := click(m, rowLine(m, 0))
		if got.actionFocus {
			t.Fatal("clicking a row must hand the focus back to the list")
		}
		// …so enter now means "edit what I clicked", not "press Add".
		got = pressList(t, got, "enter")
		if got.stage != stageForm || got.formMode != formEdit {
			t.Fatalf("enter gave stage=%v mode=%v, want the clicked prompt's edit form", got.stage, got.formMode)
		}
	})

	t.Run("a second click opens the prompt for editing", func(t *testing.T) {
		m := withBothScopes()
		y := rowLine(m, 1)
		got := click(click(m, y), y)
		if got.stage != stageForm || got.formMode != formEdit {
			t.Fatalf("stage=%v mode=%v, want the edit form (status: %q)", got.stage, got.formMode, got.status)
		}
		if got.editID != "p2" {
			t.Fatalf("editing %q, want the double-clicked prompt", got.editID)
		}
	})

	t.Run("no gesture on this screen reaches an agent", func(t *testing.T) {
		// The one action that leaves the program is not on the pointer's
		// escalation path: two clicks open a form, and getting to the picker
		// takes a deliberate press of Send.
		m := withBothScopes()
		m.client = &catsClient{socket: "/nonexistent/cats.sock"}
		y := rowLine(m, 1)
		for _, n := range []int{2, 3, 4} {
			got := m
			for i := 0; i < n; i++ {
				got = click(got, y)
			}
			if got.stage == stageTarget || got.dropping {
				t.Fatalf("%d clicks gave stage=%v dropping=%v, want no drop", n, got.stage, got.dropping)
			}
		}
	})

	t.Run("two clicks far apart are two selections", func(t *testing.T) {
		m := withBothScopes()
		y := rowLine(m, 1)
		got := click(m, y)
		got.lastClickAt = got.lastClickAt.Add(-time.Second) // the user paused
		if got = click(got, y); got.stage != stageList {
			t.Fatalf("stage = %v, want a slow second click to select, not open", got.stage)
		}
	})

	t.Run("a second click elsewhere is a first click there", func(t *testing.T) {
		m := withBothScopes()
		got := click(click(m, rowLine(m, 0)), rowLine(m, 2))
		if got.stage != stageList {
			t.Fatalf("stage = %v, want two different rows to be two selections", got.stage)
		}
		if ref, _ := got.selectedRef(); ref.id != "g1" {
			t.Fatalf("selected %q, want the second row clicked", ref.id)
		}
	})

	t.Run("headings and empty space are not rows", func(t *testing.T) {
		m := withBothScopes()
		before, _ := m.selectedRef()
		for _, y := range []int{listRowsRow, listRowsRow + 1, 0, 1, 100} {
			if _, ok := m.list.rowAtLine(y - listRowsRow); ok {
				continue // this one really is a row; the loop is about the gaps
			}
			got := click(m, y)
			if got.stage != stageList {
				t.Fatalf("click at y=%d changed the stage to %v", y, got.stage)
			}
			if ref, _ := got.selectedRef(); ref != before {
				t.Fatalf("click at y=%d moved the highlight to %+v", y, ref)
			}
		}
	})
}

// TestListRowsMatchWhatIsDrawn holds listRowsRow and the hit test to the frame
// the list actually renders — headings and spacers included, since those are
// exactly what a click must not resolve to.
func TestListRowsMatchWhatIsDrawn(t *testing.T) {
	m := withBothScopes()
	lines := strings.Split(m.viewList(), "\n")

	for i, want := range []string{"first", "second", "third"} {
		ref := m.rows[i]
		var line int
		found := false
		for n := 0; n < 20 && !found; n++ {
			if got, ok := m.list.rowAtLine(n); ok && got >= 0 && m.list.filtered[got].item.ref == i {
				line, found = listRowsRow+n, true
			}
		}
		if !found {
			t.Fatalf("no line hit-tests to row %d (%s)", i, want)
		}
		if line >= len(lines) || !strings.Contains(lines[line], want) {
			t.Fatalf("line %d is %q, want the row for %q (ref %+v):\n%s", line, lines[min(line, len(lines)-1)], want, ref, m.viewList())
		}
	}

	// The group headings sit on lines of their own, and none of them answers.
	for n, line := range lines {
		if !strings.Contains(line, "Project") && !strings.Contains(line, "Global") {
			continue
		}
		if _, ok := m.list.rowAtLine(n - listRowsRow); ok {
			t.Fatalf("the heading on line %d hit-tests as a row: %q", n, line)
		}
	}
}

// TestTargetRowsMatchWhatIsDrawn pins rowAtLine's line arithmetic to the frame
// the picker actually renders: it walks the same lines view writes, and a layout
// change above or between the rows would silently aim every click one row off.
func TestTargetRowsMatchWhatIsDrawn(t *testing.T) {
	m := withTargets(3)
	lines := strings.Split(m.viewTarget(), "\n")
	for i, tg := range m.targets {
		y := targetRowsRow + i
		if y >= len(lines) {
			t.Fatalf("row %d falls outside the %d-line frame:\n%s", i, len(lines), m.viewTarget())
		}
		if !strings.Contains(lines[y], tg.agent) {
			t.Fatalf("line %d is %q, want the row for %s:\n%s", y, lines[y], tg.agent, m.viewTarget())
		}
		got, ok := m.targetList.rowAtLine(y - targetRowsRow)
		if !ok || got != i {
			t.Fatalf("rowAtLine(%d) = %d,%v, want row %d — the hit test and the view disagree", y-targetRowsRow, got, ok, i)
		}
	}
	// Nothing matches: the empty message stands where the first row would, and
	// a click on it must not pick the row it replaced.
	m.targetList.input.SetValue("zzz")
	m.targetList.filter()
	if _, ok := m.targetList.rowAtLine(0); ok {
		t.Fatal("the empty-list message must not hit-test as a row")
	}
}

// TestScopeNote pins the header's scope note against its room budget: the
// project basename must always lead (the workspace label used to replace it,
// which hid launches that landed in the wrong directory), the workspace label
// rides along when it adds information, and under pressure the label compacts
// and disappears first — the project name is cut only as the last resort, and
// keeps its "+ global"/"only" suffix when it is.
func TestScopeNote(t *testing.T) {
	// Room enough that the note + label fit uncompacted.
	const roomy = 60

	build := func(wsLabel string) model {
		m := newTestModel()
		m.ctx.WorkspaceLabel = wsLabel
		return m
	}

	t.Run("project and workspace both shown", func(t *testing.T) {
		got := build("pers").scopeNote(roomy)
		if got != "project + global · ws:pers" {
			t.Fatalf("scopeNote = %q, want project first with ws suffix", got)
		}
	})

	t.Run("no workspace label", func(t *testing.T) {
		if got := build("").scopeNote(roomy); got != "project + global" {
			t.Fatalf("scopeNote = %q, want bare project note", got)
		}
	})

	t.Run("label matching the project is dropped as redundant", func(t *testing.T) {
		// newTestModel's WorkDir basename is "project".
		if got := build("project").scopeNote(roomy); got != "project + global" {
			t.Fatalf("scopeNote = %q, want suffix deduped", got)
		}
	})

	t.Run("tight room compacts the label, not the project", func(t *testing.T) {
		got := build("a-rather-long-workspace-name").scopeNote(30)
		if !strings.HasPrefix(got, "project + global · ws:") {
			t.Fatalf("scopeNote = %q, project note must survive compaction intact", got)
		}
		if !strings.HasSuffix(got, "…") {
			t.Fatalf("scopeNote = %q, want a truncated (…) workspace label", got)
		}
		if n := len([]rune(got)); n > 30 {
			t.Fatalf("scopeNote is %d runes, wider than its %d-rune room: %q", n, 30, got)
		}
	})

	t.Run("no room at all drops the label entirely", func(t *testing.T) {
		if got := build("pers").scopeNote(20); got != "project + global" {
			t.Fatalf("scopeNote = %q, want label dropped, project kept", got)
		}
	})

	t.Run("a long project name truncates instead of overflowing", func(t *testing.T) {
		// The bug this pins: the old header never bounded the project part, so
		// a long directory name overflowed the line — and in a wrapping
		// renderer pushed every click target down a row.
		m := build("")
		m.ctx.WorkDir = "/tmp/an-extremely-long-project-directory-name"
		got := m.scopeNote(25)
		if n := len([]rune(got)); n > 25 {
			t.Fatalf("scopeNote is %d runes, wider than its %d-rune room: %q", n, 25, got)
		}
		if !strings.Contains(got, "…") {
			t.Fatalf("scopeNote = %q, want a truncated (…) project name", got)
		}
		if !strings.HasSuffix(got, " + global") {
			t.Fatalf("scopeNote = %q, the scope suffix must survive the cut", got)
		}
	})

	t.Run("negative room (before the first resize) shows everything", func(t *testing.T) {
		got := build("a-rather-long-workspace-name").scopeNote(-1)
		if got != "project + global · ws:a-rather-long-workspace-name" {
			t.Fatalf("scopeNote = %q, want full label when width is unknown", got)
		}
	})
}

// TestHeaderLeadsWithTheBacklogName pins what the header's first column spends
// itself on. The tool's own name and version used to open the line in a chip,
// which outranked the one fact on it that changes — which backlog is being
// edited — and duplicated what the terminal tab already says. The backlog's
// name must lead, and the program's name and version must be nowhere on the
// line at any width.
func TestHeaderLeadsWithTheBacklogName(t *testing.T) {
	for _, width := range []int{40, 60, 80, 120, 200} {
		m := withTodo("ship it")
		m.width = width
		m.applySizes()
		line := stripANSI(m.headerLine())
		if !strings.HasPrefix(line, "project") {
			t.Fatalf("width %d: header does not lead with the backlog name: %q", width, line)
		}
		if strings.Contains(line, "Cats") || strings.Contains(line, "v"+version) {
			t.Fatalf("width %d: header still carries the tool's name/version: %q", width, line)
		}
	}
}

// TestHeaderTitleWeightsTheName pins the hierarchy headerTitle draws: the
// backlog name renders in its own style and the scope tail keeps the dim one,
// so dropping the chip's filled field did not flatten the line into one tone.
func TestHeaderTitleWeightsTheName(t *testing.T) {
	got := headerTitle("project + global · ws:pers")
	if want := headerNameStyle.Render("project"); !strings.Contains(got, want) {
		t.Fatalf("headerTitle did not weight the project name:\n got %q\nwant %q inside", got, want)
	}
	if want := descStyle.Render(" + global · ws:pers"); !strings.Contains(got, want) {
		t.Fatalf("headerTitle did not dim the scope tail:\n got %q\nwant %q inside", got, want)
	}
	// A note with no scope suffix has no name to weight — all of it is prose.
	if got, want := headerTitle("no backlog here"), descStyle.Render("no backlog here"); got != want {
		t.Fatalf("headerTitle(%q) = %q, want it left dim", "no backlog here", got)
	}
}

// TestHeaderLineFits pins the header budget: the line must never exceed the
// pane's width, whatever is competing for it. The bug this guards against —
// the query input used to be sized to width-4, blowing the line to ~width+8 —
// pushed the match count off-screen, and in a wrapping renderer would have
// moved every click target down a row.
func TestHeaderLineFits(t *testing.T) {
	scenarios := []struct {
		name string
		prep func(*model)
	}{
		{"plain", func(m *model) {}},
		{"long project basename", func(m *model) {
			m.ctx.WorkDir = "/tmp/an-extremely-long-project-directory-name-that-overflows"
		}},
		{"long workspace label", func(m *model) {
			m.ctx.WorkspaceLabel = "a-rather-long-workspace-name"
		}},
		{"hidden tag showing", func(m *model) {
			m.project.todos = append(m.project.todos,
				Todo{ID: "d1", Title: "done", Done: true},
				Todo{ID: "f1", Title: "frozen", Frozen: true})
			m.hideClosed = true
			m.rebuildList()
		}},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			for _, width := range []int{20, 30, 40, 60, 80, 200} {
				m := withTodo("ship it")
				sc.prep(&m)
				m.width = width
				m.applySizes()
				line := m.headerLine()
				if got := lipgloss.Width(line); got > width {
					t.Fatalf("width %d: header line renders %d cells:\n%q", width, got, line)
				}
				// The count is the last thing the ladder may give up; no real
				// pane is narrow enough to cost it.
				if !strings.Contains(line, "/") {
					t.Fatalf("width %d: match count missing from header %q", width, line)
				}
			}
		})
	}

	t.Run("comfortable width carries the search box and count", func(t *testing.T) {
		m := withTodo("ship it")
		m.width = 80
		m.applySizes()
		line := m.headerLine()
		if !strings.Contains(line, searchGlyph) {
			t.Fatalf("header at 80 cols dropped the query box: %q", line)
		}
		if !strings.Contains(line, "1/1") {
			t.Fatalf("header at 80 cols lost the match count: %q", line)
		}
	})

	t.Run("the input is sized to the same budget the line renders", func(t *testing.T) {
		// The rendered line and the input's own SetWidth come from one layout;
		// if they ever diverge the line silently over- or under-runs.
		m := withTodo("ship it")
		m.width = 120
		m.applySizes()
		if got, want := m.list.input.Width(), m.searchWidth(); got != want {
			t.Fatalf("input width %d, headerLayout budget %d", got, want)
		}
	})
}

// ansiSeq matches the SGR escapes lipgloss wraps styled text in, so a test can
// assert on the characters a user sees rather than on the styling around them.
var ansiSeq = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiSeq.ReplaceAllString(s, "") }

// TestHeaderSearchLead pins the breath in front of the query box: at any width
// a real pane has, the search glyph must stand clear of the scope note rather
// than reading as its tail. The box concedes its own slack first, so the lead
// survives well below the widths where the field is still full size.
func TestHeaderSearchLead(t *testing.T) {
	for _, width := range []int{80, 100, 120, 200} {
		m := withTodo("ship it")
		m.width = width
		m.applySizes()
		if got := m.headerLayout().searchLead; got != searchLead {
			t.Fatalf("width %d: search lead %d cells, want %d", width, got, searchLead)
		}
		// The gap is whitespace on the rendered line, not just a budget number.
		// The glyph is styled, so the escape codes come off before matching.
		if !strings.Contains(stripANSI(m.headerLine()), strings.Repeat(" ", searchLead)+searchGlyph) {
			t.Fatalf("width %d: header does not render the lead before the glyph:\n%q",
				width, m.headerLine())
		}
	}
}

// TestHeaderSearchShowsFocus pins the inline affordance that replaced the
// boxed field's rails: the header must render differently when the query box
// holds the keys than when a button does, or handing the focus back has
// nothing to show for itself.
func TestHeaderSearchShowsFocus(t *testing.T) {
	m := withTodo("ship it")
	m.width = 120
	m.applySizes()

	focused := m.headerLine()
	m.setActionFocus(true)
	blurred := m.headerLine()
	if focused == blurred {
		t.Fatal("the header reads the same with and without the keys in the box")
	}
}

// TestStatusRowStable pins the footer to its line: a status message appearing
// or clearing must not change the view's height, or the help line jumps two
// rows every time a drop lands.
func TestStatusRowStable(t *testing.T) {
	m := withTodo("ship it")
	m.width = 120
	m.applySizes()

	quiet := strings.Split(m.viewList(), "\n")
	m.setStatus("dropped → somewhere", false)
	loud := strings.Split(m.viewList(), "\n")
	if len(quiet) != len(loud) {
		t.Fatalf("view is %d lines quiet and %d with a status — the footer moved", len(quiet), len(loud))
	}
}

// TestFooterDeduped pins the footer's split with the action bar: while the
// chips carry their own key hints the footer names only the chords the bar
// doesn't, and in a pane too narrow for hints the footer names everything.
func TestFooterDeduped(t *testing.T) {
	t.Run("wide pane trims the chords the chips teach", func(t *testing.T) {
		m := withTodo("ship it")
		m.width = 200
		if !m.barShowsHints() {
			t.Fatal("a 200-column bar should carry its hints")
		}
		footer := m.listFooter()
		for _, taught := range []string{"ctrl+a", "ctrl+x"} {
			if strings.Contains(footer, taught) {
				t.Fatalf("footer repeats %s, which the bar already teaches:\n%s", taught, footer)
			}
		}
		if !strings.Contains(footer, "ctrl+v") {
			t.Fatalf("footer lost a chord the bar does not teach:\n%s", footer)
		}
	})

	t.Run("narrow pane names everything", func(t *testing.T) {
		m := withTodo("ship it")
		m.width = 40
		if m.barShowsHints() {
			t.Fatal("a 40-column bar should have dropped its hints")
		}
		footer := m.listFooter()
		for _, chord := range []string{"ctrl+a add", "ctrl+x delete", "ctrl+v view"} {
			if !strings.Contains(footer, chord) {
				t.Fatalf("narrow footer must teach %s — nothing else on screen does:\n%s", chord, footer)
			}
		}
	})
}

// TestHeaderClickFocusesSearch: the query box is a tab stop and shows its own
// focus, so a click on the header line it lives on has to hand the keys back
// to it — a first-class stop the pointer can't reach would be the old
// inconsistency in a new place.
func TestHeaderClickFocusesSearch(t *testing.T) {
	m := withTodo("ship it")
	m.width = 120
	m.applySizes()
	m = pressList(t, m, "tab")
	if !m.actionFocus {
		t.Fatal("tab should have lit a chip")
	}

	next, _ := m.Update(tea.MouseClickMsg{X: 5, Y: headerRow, Button: tea.MouseLeft})
	m = next.(model)
	if m.actionFocus {
		t.Fatal("a header click must take the focus off the bar")
	}
	if !m.list.input.Focused() {
		t.Fatal("a header click must hand the keys back to the query box")
	}
}

// TestPaneTitleOpenCount pins the count cats reads off the terminal title to
// decide whether a workspace is holding unfinished work: present only when the
// named backlog has open todos, and blind to the global list when a project
// backlog is what the pane is named after.
func TestPaneTitleOpenCount(t *testing.T) {
	open := Todo{ID: "a", Title: "open"}
	done := Todo{ID: "b", Title: "done", Done: true}

	tests := []struct {
		name            string
		project, global []Todo
		globalOnly      bool
		want            string
	}{
		{name: "empty backlog carries no count", want: "todo: project"},
		{name: "only done todos carry no count", project: []Todo{done}, want: "todo: project"},
		{name: "open todos are counted", project: []Todo{open, done, open}, want: "todo: project (2)"},
		{
			name:   "a project pane ignores the global backlog",
			global: []Todo{open, open},
			want:   "todo: project",
		},
		{
			name:       "a global-only pane counts the global backlog",
			global:     []Todo{open, open},
			globalOnly: true,
			want:       "todo: global (2)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel()
			m.project.todos, m.global.todos = tt.project, tt.global
			if tt.globalOnly {
				// A global-only launch leaves the project store pathless, which
				// is what makes it unavailable — same as launching outside a project.
				m.ctx.Scope, m.project.path = launchGlobalOnly, ""
			}
			if got := m.paneTitle(); got != tt.want {
				t.Errorf("paneTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

// schedModel builds a manager whose project backlog is a real file under
// t.TempDir(), holding one open prompt. The schedule paths all reload from
// disk before mutating (setSchedule, claimSchedule), so the in-memory-only
// store withTodo uses would be wiped by the first of them.
func schedModel(t *testing.T, client *catsClient) model {
	t.Helper()
	project := &store{scope: scopeProject, path: filepath.Join(t.TempDir(), "todos.json")}
	if err := project.add(Todo{ID: "t1", Title: "ship it", Prompt: "do the thing"}); err != nil {
		t.Fatal(err)
	}
	global := &store{scope: scopeGlobal, path: filepath.Join(t.TempDir(), "todos.json")}
	m := newModel(RunContext{WorkDir: "/tmp/cats-todo-test/project"}, project, global, client)
	m.width = 120
	m.applySizes()
	return m
}

// TestScheduleGuards pins the refusals: no socket means no promise to fire,
// and a done prompt has nothing to fire.
func TestScheduleGuards(t *testing.T) {
	t.Run("nil client refuses on the list", func(t *testing.T) {
		m := schedModel(t, nil)
		m = pressList(t, m, "ctrl+s")
		if m.stage != stageList {
			t.Fatalf("stage = %v, want to stay on the list", m.stage)
		}
		if !m.statusErr || !strings.Contains(m.status, "socket") {
			t.Fatalf("status = %q, want the socket refusal", m.status)
		}
	})

	t.Run("done prompt is redirected to reopen", func(t *testing.T) {
		m := schedModel(t, &catsClient{})
		if err := m.project.setDone("t1", true); err != nil {
			t.Fatal(err)
		}
		m.rebuildList()
		m = pressList(t, m, "ctrl+s")
		if m.stage != stageList || !strings.Contains(m.status, "reopen") {
			t.Fatalf("stage %v status %q, want a stay-put hint about reopening", m.stage, m.status)
		}
	})
}

// TestScheduleFlow drives the happy path end to end: ctrl+s, a time, the
// picker in schedule flavor, and the schedule persisted — with nothing fired
// and nothing in flight.
func TestScheduleFlow(t *testing.T) {
	m := schedModel(t, &catsClient{})
	m = pressList(t, m, "ctrl+s")
	if m.stage != stageSchedule {
		t.Fatalf("stage = %v, want stageSchedule", m.stage)
	}

	m.schedInput.SetValue("in 2h")
	next, _ := m.updateSchedule(pressKey("enter"))
	m = next.(model)
	if m.stage != stageTarget || !m.pickForSchedule {
		t.Fatalf("stage %v pickForSchedule %v, want the schedule-flavored picker", m.stage, m.pickForSchedule)
	}

	// The picker's first target is always the built-in new Claude session
	// (buildTargets degrades to just that when pane.list is unreachable).
	next, _ = m.updateTarget(pressKey("enter"))
	m = next.(model)
	if m.stage != stageList || m.pickForSchedule || m.dropping {
		t.Fatalf("stage %v pickForSchedule %v dropping %v — want back on the list with nothing in flight",
			m.stage, m.pickForSchedule, m.dropping)
	}
	if !strings.Contains(m.status, "scheduled for") {
		t.Fatalf("status = %q, want the scheduled confirmation", m.status)
	}

	td, _ := m.project.find("t1")
	if td.Schedule == nil || td.Schedule.Kind != scheduleKindNew || td.Schedule.Command != "claude" {
		t.Fatalf("persisted schedule = %+v, want a new-claude-session fire", td.Schedule)
	}
	if until := time.Until(td.Schedule.At); until < 110*time.Minute || until > 130*time.Minute {
		t.Fatalf("fire time is %v away, want about two hours", until)
	}

	t.Run("a bad time stays in the editor with the reason", func(t *testing.T) {
		m := schedModel(t, &catsClient{})
		m = pressList(t, m, "ctrl+s")
		m.schedInput.SetValue("soonish")
		next, _ := m.updateSchedule(pressKey("enter"))
		m = next.(model)
		if m.stage != stageSchedule || m.schedErr == "" {
			t.Fatalf("stage %v err %q, want to stay in the editor with the parse error", m.stage, m.schedErr)
		}
	})

	t.Run("esc out of the picker leaks no schedule flavor", func(t *testing.T) {
		m := schedModel(t, &catsClient{})
		m = pressList(t, m, "ctrl+s")
		m.schedInput.SetValue("in 2h")
		next, _ := m.updateSchedule(pressKey("enter"))
		m = next.(model)
		next, _ = m.updateTarget(pressKey("esc"))
		m = next.(model)
		if m.pickForSchedule {
			t.Fatal("esc left the picker flagged — the next manual drop would schedule instead of firing")
		}
		if td, _ := m.project.find("t1"); td.Schedule != nil {
			t.Fatal("nothing was committed, so nothing may be persisted")
		}
	})

	t.Run("enter on an emptied input clears an existing schedule", func(t *testing.T) {
		m := schedModel(t, &catsClient{})
		at := time.Now().Add(time.Hour)
		if err := m.project.setSchedule("t1", &Schedule{At: at, Kind: scheduleKindNew, Command: "claude"}); err != nil {
			t.Fatal(err)
		}
		m.rebuildList()
		m = pressList(t, m, "ctrl+s")
		if m.schedInput.Value() == "" {
			t.Fatal("an existing schedule must prefill the editor")
		}
		m.schedInput.SetValue("")
		next, _ := m.updateSchedule(pressKey("enter"))
		m = next.(model)
		if m.stage != stageList || !strings.Contains(m.status, "cleared") {
			t.Fatalf("stage %v status %q, want back on the list with the cleared note", m.stage, m.status)
		}
		if td, _ := m.project.find("t1"); td.Schedule != nil {
			t.Fatal("the schedule should be gone")
		}
	})
}

// TestScheduleTick pins the fire loop's judgment calls, driven by synthetic
// scheduleTickMsg and dropResultMsg — no goroutines, no sockets.
func TestScheduleTick(t *testing.T) {
	arm := func(t *testing.T, m model, at time.Time) model {
		t.Helper()
		if err := m.project.setSchedule("t1", &Schedule{At: at, Kind: scheduleKindNew, Command: "claude", Cwd: "/tmp"}); err != nil {
			t.Fatal(err)
		}
		m.rebuildList()
		return m
	}
	sched := func(t *testing.T, m model) *Schedule {
		t.Helper()
		td, ok := m.project.find("t1")
		if !ok {
			t.Fatal("the todo vanished")
		}
		return td.Schedule
	}

	t.Run("not yet due: nothing happens", func(t *testing.T) {
		now := time.Now()
		m := arm(t, schedModel(t, &catsClient{}), now.Add(time.Hour))
		next, _ := m.Update(scheduleTickMsg(now))
		m = next.(model)
		if m.dropping || sched(t, m) == nil || sched(t, m).Missed {
			t.Fatal("a future schedule must ride through the tick untouched")
		}
	})

	t.Run("due within grace: claimed and fired", func(t *testing.T) {
		now := time.Now()
		m := arm(t, schedModel(t, &catsClient{}), now.Add(-30*time.Second))
		next, cmd := m.Update(scheduleTickMsg(now))
		m = next.(model)
		if !m.dropping {
			t.Fatal("a due schedule must put a drop in flight")
		}
		if cmd == nil {
			t.Fatal("the fire command went missing")
		}
		if sched(t, m) != nil {
			t.Fatal("the claim must clear the schedule before the drop runs — that is the no-double-fire guard")
		}
	})

	t.Run("due beyond grace: missed, not fired", func(t *testing.T) {
		now := time.Now()
		m := arm(t, schedModel(t, &catsClient{}), now.Add(-scheduleGrace-time.Minute))
		next, _ := m.Update(scheduleTickMsg(now))
		m = next.(model)
		if m.dropping {
			t.Fatal("a stale schedule fired — the grace window exists to stop exactly this")
		}
		sc := sched(t, m)
		if sc == nil || !sc.Missed {
			t.Fatalf("schedule = %+v, want it kept and marked Missed", sc)
		}
		if !m.statusErr || !strings.Contains(m.status, "missed") {
			t.Fatalf("status = %q, want the missed report", m.status)
		}
	})

	t.Run("due with no socket: missed, not fired", func(t *testing.T) {
		now := time.Now()
		m := arm(t, schedModel(t, nil), now.Add(-30*time.Second))
		next, _ := m.Update(scheduleTickMsg(now))
		m = next.(model)
		if sc := sched(t, m); sc == nil || !sc.Missed {
			t.Fatalf("schedule = %+v, want Missed — there is nothing to fire through", sc)
		}
	})

	t.Run("a missed schedule never re-fires", func(t *testing.T) {
		now := time.Now()
		m := arm(t, schedModel(t, &catsClient{}), now.Add(-scheduleGrace-time.Minute))
		next, _ := m.Update(scheduleTickMsg(now))
		m = next.(model)
		next, cmd := m.Update(scheduleTickMsg(now.Add(time.Second)))
		m = next.(model)
		if m.dropping {
			t.Fatal("the second tick fired a schedule already marked Missed")
		}
		_ = cmd // the re-armed tick; the fire cmd inside the batch is what dropping guards
	})

	t.Run("a drop in flight defers the fire", func(t *testing.T) {
		now := time.Now()
		m := arm(t, schedModel(t, &catsClient{}), now.Add(-30*time.Second))
		m.dropping = true
		next, _ := m.Update(scheduleTickMsg(now))
		m = next.(model)
		if sc := sched(t, m); sc == nil || sc.Missed {
			t.Fatalf("schedule = %+v, want it left armed for the next tick", sc)
		}
	})

	t.Run("a failed fire is written back as Missed", func(t *testing.T) {
		m := schedModel(t, &catsClient{})
		sc := Schedule{At: time.Now().Add(-10 * time.Second), Kind: scheduleKindPane, Pane: 42, Agent: "claude"}
		m.dropping = true
		next, _ := m.Update(dropResultMsg{
			desc:  "claude",
			ref:   todoRef{scope: scopeProject, id: "t1"},
			err:   fmt.Errorf("the scheduled pane is gone"),
			sched: &sc,
		})
		m = next.(model)
		if m.dropping {
			t.Fatal("the result must clear the in-flight flag")
		}
		td, _ := m.project.find("t1")
		if td.Schedule == nil || !td.Schedule.Missed || td.Schedule.Pane != 42 {
			t.Fatalf("schedule = %+v, want the original written back as Missed", td.Schedule)
		}
		if !strings.Contains(m.status, "scheduled drop failed") {
			t.Fatalf("status = %q, want the scheduled-failure report", m.status)
		}
	})

	t.Run("a successful fire completes the todo", func(t *testing.T) {
		m := schedModel(t, &catsClient{})
		sc := Schedule{At: time.Now().Add(-10 * time.Second), Kind: scheduleKindNew, Command: "claude"}
		m.dropping = true
		next, _ := m.Update(dropResultMsg{
			desc:  "new Claude Code session",
			ref:   todoRef{scope: scopeProject, id: "t1"},
			sched: &sc,
		})
		m = next.(model)
		td, _ := m.project.find("t1")
		if !td.Done {
			t.Fatal("a delivered prompt is done — same contract as a manual drop")
		}
		if td.Schedule != nil {
			t.Fatal("nothing may be left armed on a completed todo")
		}
	})
}

// TestRowDescMarks covers the flags that lead a row's description: the order
// they are drawn in, and the attachment count carrying the app's cyan while the
// other two stay in the description's grey. The count is a mark of its own
// rather than text on the front of desc precisely so it can hold a color — see
// descMark — so a regression here reads as "📎2 went grey again".
func TestRowDescMarks(t *testing.T) {
	m := newTestModel()
	m.project.todos = []Todo{{
		ID: "t1", Title: "ship it", Prompt: "do the thing",
		Images:  []string{"a.png", "b.png"},
		Session: &SessionOpts{Model: "sonnet"},
	}}
	m.rebuildList()

	marks := m.list.items[0].descMarks
	var texts []string
	for _, mk := range marks {
		texts = append(texts, mk.text)
	}
	// ⚙ before 📎, and both after the prompt's own flags — the order the row is
	// read in, least specific first.
	if got, want := strings.Join(texts, " "), "⚙ 📎2"; got != want {
		t.Fatalf("marks = %q, want %q", got, want)
	}
	// Styles are compared by what they draw: lipgloss.Style holds slices and is
	// not comparable, and what matters here is the escape sequence that reaches
	// the terminal anyway.
	const probe = "x"
	if got, want := marks[1].style.Render(probe), attachStyle.Render(probe); got != want {
		t.Errorf("the 📎 mark renders %q, want the attachment cyan %q", got, want)
	}
	if got, want := marks[0].style.Render(probe), descStyle.Render(probe); got != want {
		t.Errorf("the ⚙ mark renders %q — only the attachment count carries a hue (%q)", got, want)
	}
	// And it survives the trip to the screen, still ahead of the prompt text.
	view := m.viewList()
	if i, j := strings.Index(view, "📎2"), strings.Index(view, "do the thing"); i < 0 || i > j {
		t.Fatalf("📎2 is at %d and the prompt at %d — the count must lead it:\n%s", i, j, view)
	}
}

// TestScheduledRowShowsItself pins the row's schedule marks: the ◷ badge and
// the ⏰ time leading the description, with "missed" spelled out when it is.
func TestScheduledRowShowsItself(t *testing.T) {
	m := schedModel(t, &catsClient{})
	at := time.Now().Add(30 * time.Minute)
	if err := m.project.setSchedule("t1", &Schedule{At: at, Kind: scheduleKindNew, Command: "claude"}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()

	view := m.viewList()
	if !strings.Contains(view, "◷") || !strings.Contains(view, "⏰ "+formatScheduleTime(at, time.Now())) {
		t.Fatalf("a scheduled row must carry the badge and its fire time:\n%s", view)
	}

	sc, _ := m.project.find("t1")
	missed := *sc.Schedule
	missed.Missed = true
	if err := m.project.setSchedule("t1", &missed); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()
	if !strings.Contains(m.viewList(), "⏰ missed") {
		t.Fatal("a missed schedule must say so on the row")
	}
}
