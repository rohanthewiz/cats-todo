package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestPromptCutChords: ctrl+x and both Cmd+X spellings cut a selection. The run
// goes to the clipboard (a command is issued) and leaves the editor, with the
// caret where it began. This is the regression for the editor having no cut at
// all: ctrl+x used to be ✂ Split (now alt+x), and Cmd+X was bound to nothing.
func TestPromptCutChords(t *testing.T) {
	chords := map[string]tea.KeyPressMsg{
		"ctrl+x":  {Code: 'x', Mod: tea.ModCtrl},
		"super+x": cmdKey('x')["super+x"],
		"meta+x":  cmdKey('x')["meta+x"],
	}
	for name, key := range chords {
		t.Run(name, func(t *testing.T) {
			cut, cmd := selectAlpha(t).Update(key)
			got := cut.(model)
			if got.quitting {
				t.Fatal("cut quit the manager")
			}
			if cmd == nil {
				t.Error("cut produced no clipboard command")
			}
			if v := got.promptArea.Value(); v != " beta" {
				t.Errorf("value = %q, want the swept %q taken out", v, "alpha")
			}
			if got.selectedPromptText() != "" {
				t.Error("a selection is still standing over text that was cut")
			}
			if off := promptCaretOffset(got.promptArea); off != 0 {
				t.Errorf("caret at %d, want 0 — where the cut run began", off)
			}
			if !strings.Contains(got.formNote, "cut 5 characters") {
				t.Errorf("form note is %q, want it to report five characters cut", got.formNote)
			}
		})
	}
}

// TestPromptCutIsOneUndoStep: one cmd+z brings a cut run back whole. The cut is
// a chord rather than a delete key, so it must not fold into the deletion run
// before it. Undo is also what recovers the text if the clipboard refused it.
func TestPromptCutIsOneUndoStep(t *testing.T) {
	m := selectAlpha(t)
	m = typeInForm(t, m, tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = typeInForm(t, m, tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	if v := m.promptArea.Value(); v != "alpha beta" {
		t.Errorf("after undo the value is %q, want the cut run back", v)
	}
}

// TestPromptCutWithNothingSelectedSaysWhy: a cut chord that has nothing to act
// on says so, from the prompt and from the title alike, and changes nothing.
func TestPromptCutWithNothingSelectedSaysWhy(t *testing.T) {
	m := withForm(t, "a title", "alpha beta", 100, 40)
	m.focusForm(formFieldPrompt)
	got := typeInForm(t, m, tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	if got.promptArea.Value() != "alpha beta" {
		t.Errorf("value = %q, want it untouched", got.promptArea.Value())
	}
	if !strings.Contains(got.formNote, "nothing selected") {
		t.Errorf("form note is %q, want it to say there was nothing to cut", got.formNote)
	}

	m.focusForm(formFieldTitle)
	got = typeInForm(t, m, cmdKey('x')["super+x"])
	if got.titleInput.Value() != "a title" {
		t.Errorf("title = %q, want it untouched", got.titleInput.Value())
	}
	if !strings.Contains(got.formNote, "cutting works in the prompt") {
		t.Errorf("form note is %q, want it to say where cutting works", got.formNote)
	}
}
