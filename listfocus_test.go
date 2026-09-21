package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The list's cursor is a row index, so leaving a prompt's screen used to come
// back on whatever row now sat at that index. These pin that the list comes
// back highlighting the prompt the screen was about, however far it moved.

// selectedID is the highlighted row's todo id, or "" with nothing highlighted.
func selectedID(m model) string {
	ref, _ := m.selectedRef()
	return ref.id
}

// TestEditReturnsToEditedPrompt: raising a prompt's priority under the priority
// lens moves its row to the top. The old index then named another prompt ("a").
func TestEditReturnsToEditedPrompt(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "a", Title: "a", Prompt: "p"},
		Todo{ID: "b", Title: "b", Prompt: "p", Priority: priorityHigh},
		Todo{ID: "c", Title: "c", Prompt: "p", Priority: priorityCritical},
		Todo{ID: "d", Title: "d", Prompt: "p"},
	)
	m.orderByPriority = true
	m.rebuildList()
	m.selectRow(todoRef{scope: scopeProject, id: "d"})

	next, _ := m.beginEdit()
	m = next.(model)
	m.formAnnots.Priority = priorityCritical
	next, _ = m.saveForm()
	m = next.(model)

	if m.stage != stageList {
		t.Fatalf("stage = %v, want the list", m.stage)
	}
	if got := selectedID(m); got != "d" {
		t.Errorf("highlighted %q after the edit, want the edited prompt %q (rows %v)", got, "d", ids(m.rows))
	}
}

// TestViewReturnsToViewedPrompt: the list is rebuilt under the view (another
// prompt finished, as from another pane), shifting every row below it up one.
func TestViewReturnsToViewedPrompt(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "a", Title: "a", Prompt: "p"},
		Todo{ID: "b", Title: "b", Prompt: "p"},
		Todo{ID: "c", Title: "c", Prompt: "p"},
		Todo{ID: "d", Title: "d", Prompt: "p"},
	)
	m.selectRow(todoRef{scope: scopeProject, id: "c"})

	next, _ := m.beginView()
	m = next.(model)
	if err := m.project.setDone("a", true); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()
	next, _ = m.updateView(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)

	if got := selectedID(m); got != "c" {
		t.Errorf("highlighted %q after leaving the view, want %q (rows %v)", got, "c", ids(m.rows))
	}
}

// TestAddReturnsToNewPrompt: a saved add lands on the prompt it created.
func TestAddReturnsToNewPrompt(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "a", Title: "a", Prompt: "p"},
		Todo{ID: "b", Title: "b", Prompt: "p"},
	)
	next, _ := m.beginAdd()
	m = next.(model)
	m.titleInput.SetValue("fresh")
	m.promptArea.SetValue("a new prompt")
	next, _ = m.saveForm()
	m = next.(model)

	ref, ok := m.selectedRef()
	td, _ := m.resolve(ref)
	if !ok || td.Title != "fresh" {
		t.Errorf("highlighted %q after the add, want the new prompt", td.Title)
	}
}

// TestCancelledAddKeepsCursor: a screen about no prompt leaves the cursor be.
func TestCancelledAddKeepsCursor(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "a", Title: "a", Prompt: "p"},
		Todo{ID: "b", Title: "b", Prompt: "p"},
	)
	m.selectRow(todoRef{scope: scopeProject, id: "b"})
	next, _ := m.beginAdd()
	m = next.(model)
	next, _ = m.cancelForm()
	m = next.(model)
	if got := selectedID(m); got != "b" {
		t.Errorf("highlighted %q after a cancelled add, want %q", got, "b")
	}
}
