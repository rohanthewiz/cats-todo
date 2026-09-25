// titleundo_test.go — the title's own undo history (N-028).
package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// titleFormInTemp opens an add form with the keys on the title.
func titleFormInTemp(t *testing.T, title, prompt string) model {
	t.Helper()
	m, _, _ := splitFormInTemp(t, prompt)
	m.titleInput.SetValue(title)
	m.focusForm(formFieldTitle)
	return m
}

// TestTitleUndoTakesBackATypedWord: the title coalesces the way the prompt
// does, a word per step, and the caret comes back with the text.
func TestTitleUndoTakesBackATypedWord(t *testing.T) {
	m := titleFormInTemp(t, "", "")
	m = typeInto(t, m, "fix the bug")
	if got := m.titleInput.Value(); got != "fix the bug" {
		t.Fatalf("title = %q, want %q — the fixture never typed", got, "fix the bug")
	}
	if n := len(m.titleUndo.stack); n != 3 {
		t.Fatalf("title history holds %d steps, want 3 — one per word", n)
	}

	m = undoForm(t, m)
	if got := m.titleInput.Value(); got != "fix the " {
		t.Errorf("title = %q, want %q — one press takes back the last word", got, "fix the ")
	}
	if got := m.titleInput.Position(); got != 8 {
		t.Errorf("caret at %d, want 8 — where the hand was when the word started", got)
	}
	if !strings.Contains(m.formNote, "title undone") {
		t.Errorf("form note = %q, want it to say the title was undone", m.formNote)
	}
}

// TestTitleAndPromptHistoriesAreSeparate is the reason the title's history is a
// second stack: cmd+z in either field takes back that field's last edit and
// never the other's, whichever was edited last.
func TestTitleAndPromptHistoriesAreSeparate(t *testing.T) {
	m := titleFormInTemp(t, "", "")
	m = typeInto(t, m, "title")
	m.focusForm(formFieldPrompt)
	m = typeInto(t, m, "body")

	m.focusForm(formFieldTitle)
	m = undoForm(t, m)
	if got := m.titleInput.Value(); got != "" {
		t.Errorf("title = %q, want it taken back", got)
	}
	if got := m.promptArea.Value(); got != "body" {
		t.Errorf("prompt = %q, want it untouched by an undo in the title", got)
	}

	m.focusForm(formFieldPrompt)
	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != "" {
		t.Errorf("prompt = %q, want it taken back", got)
	}
	if n := len(m.titleUndo.redo); n != 1 {
		t.Errorf("title redo holds %d, want 1 — an undo in the prompt must not touch it", n)
	}
}

// TestTitleRedo: an undo in the title can be redone, and the next real edit
// to the title lets the redo go, the prompt's linear rule.
func TestTitleRedo(t *testing.T) {
	m := titleFormInTemp(t, "", "")
	m = typeInto(t, m, "one two")
	m = undoForm(t, m)
	m = redoForm(t, m)
	if got := m.titleInput.Value(); got != "one two" {
		t.Errorf("title = %q after redo, want %q", got, "one two")
	}
	if got := m.titleInput.Position(); got != 7 {
		t.Errorf("caret at %d after redo, want 7 — where cmd+z was pressed", got)
	}
	if !strings.Contains(m.formNote, "title redone") {
		t.Errorf("form note = %q, want it to say the title was redone", m.formNote)
	}

	m = undoForm(t, m)
	m = typeInto(t, m, "x")
	m = redoForm(t, m)
	if got := m.titleInput.Value(); got != "one x" {
		t.Errorf("title = %q, want the edit kept and nothing redone over it", got)
	}
	if !strings.Contains(m.formNote, "nothing to redo in the title") {
		t.Errorf("form note = %q, want the refusal in words", m.formNote)
	}
}

// TestTitleUndoRefusesInWords: an untouched title says so, naming the title.
func TestTitleUndoRefusesInWords(t *testing.T) {
	m := titleFormInTemp(t, "as opened", "")
	m = undoForm(t, m)
	if got := m.titleInput.Value(); got != "as opened" {
		t.Errorf("title = %q, want it untouched", got)
	}
	if !strings.Contains(m.formNote, "the title has not changed") {
		t.Errorf("form note = %q, want the refusal in words", m.formNote)
	}
}

// TestTitleKillLineIsItsOwnStep: ctrl+u takes out everything before the caret
// in one press, and that press is a step of its own rather than part of the
// backspaces before it, so one undo brings back exactly what it took.
func TestTitleKillLineIsItsOwnStep(t *testing.T) {
	m := titleFormInTemp(t, "", "")
	m = typeInto(t, m, "abc")
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = typeInForm(t, m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if got := m.titleInput.Value(); got != "" {
		t.Fatalf("title = %q, want ctrl+u to have cleared it", got)
	}
	m = undoForm(t, m)
	if got := m.titleInput.Value(); got != "ab" {
		t.Errorf("title = %q, want %q — the kill undone on its own", got, "ab")
	}
}

// TestTitleHistoryIsPerEditingSession: like the prompt's, the title's history
// starts empty when a form opens, so cmd+z can never bring back the title of
// the todo edited before.
func TestTitleHistoryIsPerEditingSession(t *testing.T) {
	m := titleFormInTemp(t, "", "")
	m = typeInto(t, m, "first")
	next, _ := m.beginAdd()
	m = next.(model)
	if n := len(m.titleUndo.stack) + len(m.titleUndo.redo); n != 0 {
		t.Errorf("a new form's title history holds %d states, want none", n)
	}
}
