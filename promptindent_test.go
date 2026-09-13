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

// TestPromptTabTypesAnIndentAtTheCaret: with nothing swept, tab types spaces
// where the caret stands, mid-line included, and the keys stay in the prompt.
// Tab used to walk the focus to the annotation bar, which left no way to type
// indentation.
func TestPromptTabTypesAnIndentAtTheCaret(t *testing.T) {
	m := withForm(t, "", "alpha beta", 100, 40)
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 5)

	// Column 5 is past the stop at 4, so the fill is 3 cells, to column 8.
	m = typeInForm(t, m, tabKey)
	if want := "alpha    beta"; m.promptArea.Value() != want {
		t.Errorf("value = %q, want %q", m.promptArea.Value(), want)
	}
	if got := promptCaretOffset(m.promptArea); got != 8 {
		t.Errorf("caret at %d, want 8 — on the tab stop past the fill it typed", got)
	}
	if m.formFocus != formFieldPrompt {
		t.Errorf("tab moved the focus to %d; in the prompt it indents", m.formFocus)
	}
}

// TestPromptTabFillsToTheNextStop: a tab typed at a caret fills to the next
// multiple of four cells on its logical row. Before this it was a flat four
// spaces, so text after labels of different lengths never lined up.
func TestPromptTabFillsToTheNextStop(t *testing.T) {
	cases := []struct {
		name, value string
		caret       int
		want        string
		wantCaret   int
	}{
		{"line start is a full unit", "", 0, "    ", 4},
		{"short of a stop", "ab", 2, "ab  ", 4},
		{"on a stop goes to the next", "abcd", 4, "abcd    ", 8},
		{"measured from its own row", "x\nabc", 5, "x\nabc ", 6},
		// 日 is two cells wide, so the caret after it is at cell 2, not 1.
		{"wide glyphs count their cells", "日本", 1, "日  本", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := withForm(t, "", tc.value, 100, 40)
			m.focusForm(formFieldPrompt)
			setPromptCaretOffset(&m.promptArea, tc.caret)
			m = typeInForm(t, m, tabKey)
			if m.promptArea.Value() != tc.want {
				t.Errorf("value = %q, want %q", m.promptArea.Value(), tc.want)
			}
			if got := promptCaretOffset(m.promptArea); got != tc.wantCaret {
				t.Errorf("caret at %d, want %d", got, tc.wantCaret)
			}
		})
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

// TestPromptTabSweepShiftsByAFixedUnit: a sweep moves every line by exactly
// four, whatever column its indent ends at. Tab stops govern what a caret
// types, not whole-line shifts, so a block's inner steps survive the move and
// shift+tab undoes it exactly.
func TestPromptTabSweepShiftsByAFixedUnit(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "  x\n      y")
	m = selectWholePrompt(t, m)
	m = typeInForm(t, m, tabKey)
	if want := "      x\n          y"; m.promptArea.Value() != want {
		t.Fatalf("value = %q, want %q — both lines +4, not rounded to stops", m.promptArea.Value(), want)
	}
	m = typeInForm(t, m, shiftTabKey)
	if want := "  x\n      y"; m.promptArea.Value() != want {
		t.Errorf("after shift+tab value = %q, want the original %q", m.promptArea.Value(), want)
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

// TestCaretsTabFillsEachCaretToItsStop: in the column mode each caret fills to
// its own next stop. Carets at the ends of rows of different lengths line up,
// and a second caret on a row is measured after the first caret's fill.
func TestCaretsTabFillsEachCaretToItsStop(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "ab\nabc")
	m.focusForm(formFieldPrompt)
	m.carets = promptCarets{on: true}
	m.carets.add(0, 2)
	m.carets.add(1, 3)
	m.syncPromptCaret()
	m = typeInForm(t, m, tabKey)
	if want := "ab  \nabc "; m.promptArea.Value() != want {
		t.Fatalf("value = %q, want %q — both carets on column 4", m.promptArea.Value(), want)
	}
	if got, want := m.carets.cols, []int{4, 4}; !slices.Equal(got, want) {
		t.Errorf("carets at columns %v, want %v", got, want)
	}
	if !m.carets.on {
		t.Error("tab ended the mode")
	}

	// Two carets on one row: the first fills 3 to column 4, which moves the
	// second from 3 to 6, and it then fills 2 to column 8. Measured on the
	// unedited row it would have filled 1 and stopped at 7.
	m, _, _ = splitFormInTemp(t, "abcdef\nx")
	m.focusForm(formFieldPrompt)
	m.carets = promptCarets{on: true}
	m.carets.add(0, 1)
	m.carets.add(0, 3)
	m.syncPromptCaret()
	m = typeInForm(t, m, tabKey)
	if want := "a   bc  def\nx"; m.promptArea.Value() != want {
		t.Fatalf("value = %q, want %q", m.promptArea.Value(), want)
	}
	if got, want := m.carets.cols, []int{4, 8}; !slices.Equal(got, want) {
		t.Errorf("carets at columns %v, want %v", got, want)
	}
}
