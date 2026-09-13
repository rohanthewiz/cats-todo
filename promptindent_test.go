package main

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

var (
	tabKey      = tea.KeyPressMsg{Code: tea.KeyTab}
	shiftTabKey = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
)

// TestPromptTabTypesAnIndentAtTheCaret: with nothing swept, tab types four
// spaces where the caret stands, mid-line included, and the keys stay in the
// prompt. Tab used to walk the focus to the annotation bar, which left no way to
// type indentation.
func TestPromptTabTypesAnIndentAtTheCaret(t *testing.T) {
	m := withForm(t, "", "alpha beta", 100, 40)
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 5)

	m = typeInForm(t, m, tabKey)
	if want := "alpha     beta"; m.promptArea.Value() != want {
		t.Errorf("value = %q, want %q", m.promptArea.Value(), want)
	}
	if got := promptCaretOffset(m.promptArea); got != 9 {
		t.Errorf("caret at %d, want 9 — past the indent it typed", got)
	}
	if m.formFocus != formFieldPrompt {
		t.Errorf("tab moved the focus to %d; in the prompt it indents", m.formFocus)
	}
}

// TestPromptShiftTabOutdentsTheCaretsLine: shift+tab takes up to one unit of
// leading spaces off the caret's line, however far along the line the caret
// is, and says so when there is nothing left to take.
func TestPromptShiftTabOutdentsTheCaretsLine(t *testing.T) {
	m := withForm(t, "", "top\n      deep", 100, 40)
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 4+8) // row 1, column 8: inside "deep"

	m = typeInForm(t, m, shiftTabKey)
	if want := "top\n  deep"; m.promptArea.Value() != want {
		t.Fatalf("value = %q, want %q", m.promptArea.Value(), want)
	}
	if got := promptCaretOffset(m.promptArea); got != 4+4 {
		t.Errorf("caret at %d, want %d — it rides left with its text", got, 4+4)
	}
	// Only two spaces are left, fewer than a unit: both go.
	m = typeInForm(t, m, shiftTabKey)
	if want := "top\ndeep"; m.promptArea.Value() != want {
		t.Fatalf("value = %q, want %q", m.promptArea.Value(), want)
	}
	m = typeInForm(t, m, shiftTabKey)
	if !strings.Contains(m.formNote, "nothing to outdent") {
		t.Errorf("form note = %q, want the refusal in words", m.formNote)
	}
	if m.formFocus != formFieldPrompt {
		t.Errorf("shift+tab moved the focus to %d; in the prompt it outdents", m.formFocus)
	}
}

// TestPromptTabIndentsASweep: tab on a sweep indents every line it touches and
// skips blank ones. The sweep stays over the whole block, indents included, so
// a second press pushes it in another level and shift+tab brings it back.
func TestPromptTabIndentsASweep(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "- one\n\n- two")
	m = selectWholePrompt(t, m)

	m = typeInForm(t, m, tabKey)
	if want := "    - one\n\n    - two"; m.promptArea.Value() != want {
		t.Fatalf("value = %q, want %q — blank line left bare", m.promptArea.Value(), want)
	}
	if got := m.selectedPromptText(); got != m.promptArea.Value() {
		t.Errorf("selected %q, want the whole block, indents included", got)
	}

	m = typeInForm(t, m, tabKey)
	if want := "        - one\n\n        - two"; m.promptArea.Value() != want {
		t.Errorf("second tab: value = %q, want %q", m.promptArea.Value(), want)
	}

	for range 2 {
		m = typeInForm(t, m, shiftTabKey)
	}
	if want := "- one\n\n- two"; m.promptArea.Value() != want {
		t.Errorf("after two outdents value = %q, want %q", m.promptArea.Value(), want)
	}
	if got := m.selectedPromptText(); got != m.promptArea.Value() {
		t.Errorf("selected %q after outdenting, want the whole block", got)
	}
	m = typeInForm(t, m, shiftTabKey)
	if !strings.Contains(m.formNote, "none of the swept lines") {
		t.Errorf("form note = %q, want the refusal in words", m.formNote)
	}
}

// TestPromptTabSweepEndingAtALineStart: a sweep that ends exactly at the next
// row's first character does not claim that row (promptRowRange), so that row is
// not indented. Its caret still has to move to the right place, because the
// row above it grew by four.
func TestPromptTabSweepEndingAtALineStart(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "a\nb\nc")
	m = selectPromptRange(t, m, 0, 2) // "a\n", ending at b's first character

	m = typeInForm(t, m, tabKey)
	if want := "    a\nb\nc"; m.promptArea.Value() != want {
		t.Fatalf("value = %q, want %q — only the row the sweep covers", m.promptArea.Value(), want)
	}
	if got, want := m.selectedPromptText(), "    a\n"; got != want {
		t.Errorf("selected %q, want %q", got, want)
	}
}

// TestPromptTabEmptySweepRefuses: indenting only blank lines is refused in
// words rather than doing nothing silently.
func TestPromptTabEmptySweepRefuses(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "\n\n")
	m = selectWholePrompt(t, m)
	m = typeInForm(t, m, tabKey)
	if m.promptArea.Value() != "\n\n" {
		t.Errorf("value = %q, want the blank lines untouched", m.promptArea.Value())
	}
	if !strings.Contains(m.formNote, "nothing to indent") {
		t.Errorf("form note = %q, want the refusal in words", m.formNote)
	}
}

// TestTabStillWalksTheRingOutsideThePrompt: the title keeps tab for the focus
// ring, so title → prompt still works, and shift+tab from the title still wraps
// round to the annotation bar.
func TestTabStillWalksTheRingOutsideThePrompt(t *testing.T) {
	m := withForm(t, "a title", "body", 100, 40)
	m.focusForm(formFieldTitle)
	m = typeInForm(t, m, tabKey)
	if m.formFocus != formFieldPrompt {
		t.Errorf("tab from the title reached %d, want the prompt", m.formFocus)
	}
	if m.promptArea.Value() != "body" {
		t.Errorf("tab from the title edited the prompt: %q", m.promptArea.Value())
	}

	m.focusForm(formFieldTitle)
	m = typeInForm(t, m, shiftTabKey)
	if m.formFocus != formFieldAnnots {
		t.Errorf("shift+tab from the title reached %d, want the annotation bar", m.formFocus)
	}
}

// TestPromptTabLeavesModifiedChordsAlone: only a bare or shifted tab indents.
// ctrl+tab and alt+tab are not indents, so they must not edit the prompt.
func TestPromptTabLeavesModifiedChordsAlone(t *testing.T) {
	for _, mod := range []tea.KeyMod{tea.ModCtrl, tea.ModAlt} {
		if dir := promptIndentDir(tea.KeyPressMsg{Code: tea.KeyTab, Mod: mod}); dir != 0 {
			t.Errorf("tab with mod %v read as an indent (%d)", mod, dir)
		}
	}
}

// TestFormFooterTabSegmentFollowsFocus: the footer names the key the focused
// stop actually has. In the prompt that is the indent, and elsewhere the field
// switch.
func TestFormFooterTabSegmentFollowsFocus(t *testing.T) {
	m := withForm(t, "", "body", 200, 40)
	m.focusForm(formFieldPrompt)
	if foot := m.formFooter(); !strings.Contains(foot, "tab indents") || strings.Contains(foot, "tab switch field") {
		t.Errorf("prompt-focused footer does not teach the indent:\n%s", foot)
	}
	m.focusForm(formFieldTitle)
	if foot := m.formFooter(); !strings.Contains(foot, "tab switch field") {
		t.Errorf("title-focused footer lost the field switch:\n%s", foot)
	}
}

// TestCaretsTabIndentsEveryCaret: in the column mode tab types the indent at
// every caret and shift+tab outdents each caret's line once, even when two
// carets share it.
func TestCaretsTabIndentsEveryCaret(t *testing.T) {
	m := caretsOver(t, "one\ntwo")
	m = typeInForm(t, m, tabKey)
	if want := "    one\n    two"; m.promptArea.Value() != want {
		t.Fatalf("value = %q, want %q", m.promptArea.Value(), want)
	}
	if !m.carets.on {
		t.Fatal("tab ended the mode")
	}
	m = typeInForm(t, m, shiftTabKey)
	if want := "one\ntwo"; m.promptArea.Value() != want {
		t.Errorf("value = %q, want %q", m.promptArea.Value(), want)
	}
	m = typeInForm(t, m, shiftTabKey)
	if !strings.Contains(m.formNote, "nothing to outdent") || !m.carets.on {
		t.Errorf("note = %q on=%v, want a worded refusal with the mode still up", m.formNote, m.carets.on)
	}

	// Two carets on one row: the row is outdented once, not twice.
	m, _, _ = splitFormInTemp(t, "        deep\nx")
	m.focusForm(formFieldPrompt)
	m.carets = promptCarets{on: true}
	m.carets.add(0, 8)
	m.carets.add(0, 10)
	m.syncPromptCaret()
	m = typeInForm(t, m, shiftTabKey)
	if want := "    deep\nx"; m.promptArea.Value() != want {
		t.Errorf("value = %q, want %q — one unit off a shared row", m.promptArea.Value(), want)
	}
	if got, want := m.carets.cols, []int{4, 6}; !slices.Equal(got, want) {
		t.Errorf("carets at columns %v, want %v", got, want)
	}
}

var backspaceKey = tea.KeyPressMsg{Code: tea.KeyBackspace}

// promptAt opens a form whose prompt holds value, focused, with the caret at off.
func promptAt(t *testing.T, value string, off int) model {
	t.Helper()
	m := withForm(t, "", value, 100, 40)
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, off)
	return m
}

// TestPromptEnterCarriesTheIndent: a new line starts at the indent of the line
// enter was pressed on, whatever that indent is. There are no tab stops, so an
// indent of two carries as two. Before this every new line started at the
// margin, and a nested list had to be re-spaced line by line.
func TestPromptEnterCarriesTheIndent(t *testing.T) {
	cases := []struct {
		name, value string
		caret       int
		want        string
		wantCaret   int
	}{
		{"four", "    code", 8, "    code\n    ", 13},
		{"two, not rounded to a stop", "  - one", 7, "  - one\n  ", 10},
		{"mid-line splits the text", "    ab", 5, "    a\n    b", 10},
		{"no indent carries nothing", "plain", 5, "plain\n", 6},
		// Only what is left of the caret: from the line start there is nothing.
		{"line start carries nothing", "    x", 0, "\n    x", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := typeInForm(t, promptAt(t, tc.value, tc.caret), enterKey(0))
			if m.promptArea.Value() != tc.want {
				t.Errorf("value = %q, want %q", m.promptArea.Value(), tc.want)
			}
			if got := promptCaretOffset(m.promptArea); got != tc.wantCaret {
				t.Errorf("caret at %d, want %d", got, tc.wantCaret)
			}
		})
	}
}

// TestPromptEnterOnABlankIndentMovesItDown: enter on a line that is only the
// carried indent moves the indent to the new line instead of copying it, so
// the line left behind holds no invisible trailing spaces.
func TestPromptEnterOnABlankIndentMovesItDown(t *testing.T) {
	m := promptAt(t, "    - one", 9)
	m = typeInForm(t, m, enterKey(0))
	m = typeInForm(t, m, enterKey(0))
	if want := "    - one\n\n    "; m.promptArea.Value() != want {
		t.Errorf("value = %q, want %q", m.promptArea.Value(), want)
	}
	if got, want := promptCaretOffset(m.promptArea), len("    - one\n\n    "); got != want {
		t.Errorf("caret at %d, want %d", got, want)
	}
}

// TestPromptBackspaceTakesBackTheCarriedIndent: one backspace straight after the
// enter removes the whole carried indent. The next backspace is an ordinary one
// again and joins the lines.
func TestPromptBackspaceTakesBackTheCarriedIndent(t *testing.T) {
	m := promptAt(t, "    - one", 9)
	m = typeInForm(t, m, enterKey(0))
	m = typeInForm(t, m, backspaceKey)
	if want := "    - one\n"; m.promptArea.Value() != want {
		t.Fatalf("value = %q, want %q — the whole indent in one press", m.promptArea.Value(), want)
	}
	if got := promptCaretOffset(m.promptArea); got != 10 {
		t.Errorf("caret at %d, want 10, the new line's margin", got)
	}
	m = typeInForm(t, m, backspaceKey)
	if want := "    - one"; m.promptArea.Value() != want {
		t.Errorf("second backspace: value = %q, want %q", m.promptArea.Value(), want)
	}
}

// TestPromptBackspaceAfterTypingIsOrdinary: the one-press backspace is only for
// the key straight after the enter. Once anything else has been pressed, even a
// character that was then erased, backspace takes one character.
func TestPromptBackspaceAfterTypingIsOrdinary(t *testing.T) {
	m := promptAt(t, "    - one", 9)
	m = typeInForm(t, m, enterKey(0))
	m = typeInForm(t, m, typeChar('x'))
	m = typeInForm(t, m, backspaceKey)
	if want := "    - one\n    "; m.promptArea.Value() != want {
		t.Fatalf("value = %q, want %q — only the x", m.promptArea.Value(), want)
	}
	m = typeInForm(t, m, backspaceKey)
	if want := "    - one\n   "; m.promptArea.Value() != want {
		t.Errorf("value = %q, want %q — one space, not the whole indent", m.promptArea.Value(), want)
	}
}

// TestPromptShiftTabStepsOutOfTheCarriedIndent: shift+tab after the enter takes
// one unit off the carried indent, for a line one level out.
func TestPromptShiftTabStepsOutOfTheCarriedIndent(t *testing.T) {
	m := promptAt(t, "        deep", 12)
	m = typeInForm(t, m, enterKey(0))
	m = typeInForm(t, m, shiftTabKey)
	if want := "        deep\n    "; m.promptArea.Value() != want {
		t.Fatalf("value = %q, want %q", m.promptArea.Value(), want)
	}
	if got, want := promptCaretOffset(m.promptArea), len("        deep\n    "); got != want {
		t.Errorf("caret at %d, want %d", got, want)
	}
}

// TestPromptPasteDoesNotCarry: a paste goes in verbatim. Pasted text brings its
// own indentation, and adding the caret's line's indent to every line would
// push the whole paste in.
func TestPromptPasteDoesNotCarry(t *testing.T) {
	m := promptAt(t, "    a", 5)
	next, _ := m.Update(tea.PasteMsg{Content: "b\nc"})
	m = next.(model)
	if want := "    ab\nc"; m.promptArea.Value() != want {
		t.Errorf("value = %q, want %q", m.promptArea.Value(), want)
	}
}

// TestCaretsEnterCarriesEachLinesIndent: in the column mode each new line takes
// its own caret's line's indent, and one backspace straight after takes all of
// them back with the mode still on.
func TestCaretsEnterCarriesEachLinesIndent(t *testing.T) {
	m := caretsOver(t, "  one\n    two")
	m = typeInForm(t, m, tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	m = typeInForm(t, m, enterKey(0))
	if want := "  one\n  \n    two\n    "; m.promptArea.Value() != want {
		t.Fatalf("value = %q, want %q", m.promptArea.Value(), want)
	}
	if got, want := m.carets.cols, []int{2, 4}; !slices.Equal(got, want) {
		t.Errorf("carets at columns %v, want %v — each past its own indent", got, want)
	}

	m = typeInForm(t, m, backspaceKey)
	if want := "  one\n\n    two\n"; m.promptArea.Value() != want {
		t.Fatalf("after backspace value = %q, want %q", m.promptArea.Value(), want)
	}
	if got, want := m.carets.cols, []int{0, 0}; !slices.Equal(got, want) {
		t.Errorf("carets at columns %v, want %v", got, want)
	}
	if !m.carets.on {
		t.Error("backspace ended the mode")
	}
}
