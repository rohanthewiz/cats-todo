// promptlimit_test.go — the prompt editor has no logical-line cap.
package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestPromptTypingAfterLargePaste: a paste of more than 99 lines used to go in
// whole (the library's paste path does not check MaxHeight) and then leave
// enter refused, because the textarea's default MaxHeight of 99 doubles as a
// logical-line cap on InsertNewline. The editor looked stuck after any big
// paste. Every key must still land after one.
func TestPromptTypingAfterLargePaste(t *testing.T) {
	m, _ := openAddForm(t, "", "")
	m.formFocus = formFieldPrompt
	m.promptArea.Focus()

	paste := strings.Repeat("the quick brown fox\n", 150)
	next, _ := m.Update(tea.PasteMsg{Content: paste})
	m = next.(model)
	if m.promptArea.Value() != paste {
		t.Fatalf("paste landed %d bytes, want all %d", len(m.promptArea.Value()), len(paste))
	}

	m = typeInForm(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = typeInForm(t, m, enterKey(0))
	m = typeInForm(t, m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	if want := paste + "x\ny"; m.promptArea.Value() != want {
		t.Errorf("value ends %q, want it to end %q",
			m.promptArea.Value()[len(paste):], want[len(paste):])
	}
}
