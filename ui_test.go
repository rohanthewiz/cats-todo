package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
