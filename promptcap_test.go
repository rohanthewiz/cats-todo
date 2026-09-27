package main

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

// linesOf is a prompt of n lines, "x" each, joined by n-1 breaks.
func linesOf(n int) string {
	return strings.TrimSuffix(strings.Repeat("x\n", n), "\n")
}

// TestPromptMaxLinesMatchesTheLibrary pins promptMaxLines to the library's
// unexported maxLines by behaviour: a value one line over is cut to exactly
// promptMaxLines, and a value at the limit is kept whole. A bubbles upgrade
// that moves the limit fails here instead of making the guards wrong.
func TestPromptMaxLinesMatchesTheLibrary(t *testing.T) {
	ta := textarea.New()
	ta.MaxHeight = 0 // as newFormInputs builds it
	ta.SetValue(linesOf(promptMaxLines + 1))
	if got := ta.LineCount(); got != promptMaxLines {
		t.Fatalf("one line over holds %d lines, want the library to cut to %d", got, promptMaxLines)
	}
	ta.SetValue(linesOf(promptMaxLines))
	if got := ta.LineCount(); got != promptMaxLines {
		t.Fatalf("a value at the limit holds %d lines, want %d", got, promptMaxLines)
	}
}

// TestPromptPastePastTheLimitIsRefused (N-025): a paste that would take the
// prompt past the editor's line limit used to be cut silently by the library.
// It is refused whole now, with a note, and the value is untouched. A paste
// that lands exactly on the limit still goes in.
func TestPromptPastePastTheLimitIsRefused(t *testing.T) {
	// withForm leaves the caret at the end, where SetValue put it, which is
	// all this test needs. (promptAt used to be too slow here: its caret walk
	// was quadratic in the line count until N-063.)
	value := linesOf(promptMaxLines - 1)
	m := withForm(t, "", value, 100, 40)
	m.focusForm(formFieldPrompt)

	next, _ := m.Update(tea.PasteMsg{Content: "a\nb\nc"}) // two breaks: 10,001 lines
	m = next.(model)
	if m.promptArea.Value() != value {
		t.Errorf("value changed to %d lines, want the paste refused", m.promptArea.LineCount())
	}
	if !strings.Contains(m.formNote, "paste refused") || !strings.Contains(m.formNote, "10,001") {
		t.Errorf("note = %q, want the refusal naming 10,001 lines", m.formNote)
	}

	next, _ = m.Update(tea.PasteMsg{Content: "a\nb"}) // one break: exactly the limit
	m = next.(model)
	if got := m.promptArea.LineCount(); got != promptMaxLines {
		t.Errorf("a paste to the limit left %d lines, want %d", got, promptMaxLines)
	}
}

// TestPromptPasteOverASelectionCountsWhatItReplaces: the lines a swept run
// takes with it are room the paste can use, so a paste that replaces them is
// not refused for lines it would not really add.
func TestPromptPasteOverASelectionCountsWhatItReplaces(t *testing.T) {
	value := linesOf(promptMaxLines)
	m := promptAt(t, value, 0)
	m.promptSel = promptSel{anchor: len("x\nx\n"), active: true} // sweeps two breaks
	next, _ := m.Update(tea.PasteMsg{Content: "a\nb\n"})
	m = next.(model)
	if strings.Contains(m.formNote, "refused") {
		t.Fatalf("note = %q, want the paste allowed — it replaces as many breaks as it adds", m.formNote)
	}
	if got := m.promptArea.LineCount(); got != promptMaxLines {
		t.Errorf("value holds %d lines, want %d", got, promptMaxLines)
	}
}

// TestPromptEnterAtTheLimitIsRefused: enter goes through SetValue, which at
// the limit would cut the prompt's last line to make room. It is refused.
func TestPromptEnterAtTheLimitIsRefused(t *testing.T) {
	value := linesOf(promptMaxLines-1) + "\nlast"
	m := promptAt(t, value, 0)
	m = typeInForm(t, m, enterKey(0))
	if m.promptArea.Value() != value {
		t.Errorf("enter at the limit changed the value; the last line is %q",
			m.promptArea.Value()[strings.LastIndex(m.promptArea.Value(), "\n")+1:])
	}
	if !strings.Contains(m.formNote, "10,000-line limit") {
		t.Errorf("note = %q, want the limit named", m.formNote)
	}
}

// TestCaretsEnterPastTheLimitIsRefused: in the column mode enter adds a line
// per caret, all or none. Two carets on a prompt one line short of the limit
// would make 10,001 lines, so neither goes in. The sweep covers only the first
// two rows: a caret on every one of 9,999 rows costs half a minute.
func TestCaretsEnterPastTheLimitIsRefused(t *testing.T) {
	value := linesOf(promptMaxLines - 1)
	m := promptAt(t, value, 0)
	m.promptSel = promptSel{anchor: len("x\nx"), active: true}
	next, _ := m.dropPromptCarets()
	m = next.(model)
	if !m.carets.on || len(m.carets.rows) != 2 {
		t.Fatalf("setup: carets on=%v rows=%v, want two", m.carets.on, m.carets.rows)
	}
	m = typeInForm(t, m, enterKey(0))
	if m.promptArea.Value() != value {
		t.Errorf("value changed to %d lines, want the enter refused", m.promptArea.LineCount())
	}
	if !strings.Contains(m.formNote, "10,000-line limit") {
		t.Errorf("note = %q, want the limit named", m.formNote)
	}
	if !m.carets.on {
		t.Error("the refusal ended the column mode")
	}
}

// TestEditingAPromptPastTheLimitIsRefused: a stored prompt already past the
// limit would open cut, and a save would store the cut copy. It is not opened.
func TestEditingAPromptPastTheLimitIsRefused(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	if err := project.add(Todo{ID: "t1", Title: "huge", Prompt: linesOf(promptMaxLines + 5)}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()
	next, _ := m.beginEditRef(todoRef{scope: scopeProject, id: "t1"})
	m = next.(model)
	if m.stage == stageForm {
		t.Fatal("the form opened on a prompt past the editor's limit")
	}
	if !strings.Contains(m.status, "10,005 lines") {
		t.Errorf("status = %q, want the prompt's length named", m.status)
	}
}

func TestThousands(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 10000: "10,000", 1234567: "1,234,567"} {
		if got := thousands(n); got != want {
			t.Errorf("thousands(%d) = %q, want %q", n, got, want)
		}
	}
}
