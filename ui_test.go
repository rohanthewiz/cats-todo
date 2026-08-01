package main

import (
	"fmt"
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

// TestActionBarRender checks what the bar shows: every label, the key hints
// when the pane is wide enough for them, and labels alone when it is not.
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

	m.width = 30
	narrow := m.actionBar()
	for _, a := range m.listActions() {
		if !strings.Contains(narrow, a.label) {
			t.Fatalf("narrow bar %q dropped the label %q", narrow, a.label)
		}
	}
	if strings.Contains(narrow, "ctrl+a") {
		t.Fatalf("narrow bar %q should drop the hints, not wrap", narrow)
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

// TestScopeNote pins the header's scope line: the project basename must always
// lead (the workspace label used to replace it, which hid launches that landed
// in the wrong directory), the workspace label rides along when it adds
// information, and under width pressure the label — never the project — is
// what compacts and then disappears.
func TestScopeNote(t *testing.T) {
	// Wide enough that the title + note + label all fit uncompacted.
	const roomy = 120

	build := func(wsLabel string, width int) model {
		m := newTestModel()
		m.ctx.WorkspaceLabel = wsLabel
		m.width = width
		return m
	}

	t.Run("project and workspace both shown", func(t *testing.T) {
		got := build("pers", roomy).scopeNote()
		if got != "project + global · ws:pers" {
			t.Fatalf("scopeNote = %q, want project first with ws suffix", got)
		}
	})

	t.Run("no workspace label", func(t *testing.T) {
		if got := build("", roomy).scopeNote(); got != "project + global" {
			t.Fatalf("scopeNote = %q, want bare project note", got)
		}
	})

	t.Run("label matching the project is dropped as redundant", func(t *testing.T) {
		// newTestModel's WorkDir basename is "project".
		if got := build("project", roomy).scopeNote(); got != "project + global" {
			t.Fatalf("scopeNote = %q, want suffix deduped", got)
		}
	})

	t.Run("tight width compacts the label, not the project", func(t *testing.T) {
		got := build("a-rather-long-workspace-name", 60).scopeNote()
		if !strings.HasPrefix(got, "project + global · ws:") {
			t.Fatalf("scopeNote = %q, project note must survive compaction intact", got)
		}
		if !strings.HasSuffix(got, "…") {
			t.Fatalf("scopeNote = %q, want a truncated (…) workspace label", got)
		}
	})

	t.Run("no room at all drops the label entirely", func(t *testing.T) {
		if got := build("pers", 40).scopeNote(); got != "project + global" {
			t.Fatalf("scopeNote = %q, want label dropped, project kept", got)
		}
	})

	t.Run("zero width (before the first resize) shows everything", func(t *testing.T) {
		got := build("a-rather-long-workspace-name", 0).scopeNote()
		if got != "project + global · ws:a-rather-long-workspace-name" {
			t.Fatalf("scopeNote = %q, want full label when width is unknown", got)
		}
	})
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
