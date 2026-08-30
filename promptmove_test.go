package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// altArrow is alt+↑ / alt+↓ as a terminal reports it, optionally with shift.
func altArrow(code rune, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Mod: tea.ModAlt | mod}
}

// TestMovePromptLine: the caret's line walks up and down, and the caret rides it
// in the column it held — so the press after the move carries on where the hand
// already was, the same thing duplicatePromptLine buys.
func TestMovePromptLine(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "alpha\nbeta\ngamma\ndelta")
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 8) // row 1 ("beta"), column 2

	m = typeInForm(t, m, altArrow(tea.KeyDown, 0))
	if want := "alpha\ngamma\nbeta\ndelta"; m.promptArea.Value() != want {
		t.Fatalf("after alt+↓ value = %q, want %q", m.promptArea.Value(), want)
	}
	// "alpha\n" (6) + "gamma\n" (6) + 2 — column 2 of the line where it landed.
	if off := promptCaretOffset(m.promptArea); off != 14 {
		t.Errorf("caret at %d, want 14 — column 2 of the moved line", off)
	}

	m = typeInForm(t, m, altArrow(tea.KeyUp, 0))
	m = typeInForm(t, m, altArrow(tea.KeyUp, 0))
	if want := "beta\nalpha\ngamma\ndelta"; m.promptArea.Value() != want {
		t.Errorf("after two alt+↑ value = %q, want %q", m.promptArea.Value(), want)
	}
	if off := promptCaretOffset(m.promptArea); off != 2 {
		t.Errorf("caret at %d, want 2 — still column 2", off)
	}
}

// TestMovePromptLineRefusesAtTheEnds: the first line has nowhere to go up and
// the last nowhere to go down, and both say so rather than doing nothing — a
// chord that is silent on the boundary reads as a chord that stopped working.
func TestMovePromptLineRefusesAtTheEnds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		caret  int
		key    tea.KeyPressMsg
		expect string
	}{
		{"up from the first line", 1, altArrow(tea.KeyUp, 0), "already at the top"},
		{"down from the last", 13, altArrow(tea.KeyDown, 0), "already at the last line"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _ := splitFormInTemp(t, "alpha\nbeta\ngamma")
			m.focusForm(formFieldPrompt)
			setPromptCaretOffset(&m.promptArea, tc.caret)

			got := typeInForm(t, m, tc.key)
			if got.promptArea.Value() != "alpha\nbeta\ngamma" {
				t.Errorf("value = %q, want it untouched", got.promptArea.Value())
			}
			if !strings.Contains(got.formNote, tc.expect) {
				t.Errorf("form note = %q, want %q", got.formNote, tc.expect)
			}
		})
	}
}

// TestMovePromptLineMovesTheSweptBlock: with a run swept, every line it touches
// moves together and the highlight travels with it — exactly as it was, a
// selection that starts and ends mid-word included. The block's text does not
// change, only where it begins.
func TestMovePromptLineMovesTheSweptBlock(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "alpha\nbeta\ngamma\ndelta")
	m = selectPromptRange(t, m, 2, 9) // "pha\nbet" — rows 0 and 1, both partial

	m = typeInForm(t, m, altArrow(tea.KeyDown, 0))
	if want := "gamma\nalpha\nbeta\ndelta"; m.promptArea.Value() != want {
		t.Fatalf("value = %q, want %q", m.promptArea.Value(), want)
	}
	if got := m.selectedPromptText(); got != "pha\nbet" {
		t.Errorf("selection is %q, want the same run it was before the move", got)
	}
	lo, _, ok := m.promptSelSpan()
	if !ok || lo != 8 {
		t.Errorf("selection starts at %d (ok=%v), want 8 — shifted by the row it hopped", lo, ok)
	}

	// And back, which must land exactly where it started.
	m = typeInForm(t, m, altArrow(tea.KeyUp, 0))
	if want := "alpha\nbeta\ngamma\ndelta"; m.promptArea.Value() != want {
		t.Errorf("value = %q, want the original back", m.promptArea.Value())
	}
	if got := m.selectedPromptText(); got != "pha\nbet" {
		t.Errorf("selection is %q after coming back, want %q", got, "pha\nbet")
	}
}

// TestMovePromptLineMovesLogicalRows: a row long enough to soft-wrap is still
// one line. Moving what the eye sees as a line — a wrap segment — would cut a
// sentence at a boundary the value does not contain, which is the same rule
// duplicatePromptLine follows.
func TestMovePromptLineMovesLogicalRows(t *testing.T) {
	long := "the quick brown fox jumps over the lazy dog and keeps on going well past the edge"
	m, _, _ := splitFormInTemp(t, "first\n"+long)
	m.promptArea.SetWidth(30) // forces the long row over several display lines
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 6) // the start of the long row

	m = typeInForm(t, m, altArrow(tea.KeyUp, 0))
	if want := long + "\nfirst"; m.promptArea.Value() != want {
		t.Errorf("value = %q, want the whole wrapped row moved as one", m.promptArea.Value())
	}
}

// TestMovePromptLineOffTheEditorSaysWhy: the title cannot hold a second line and
// the annotation bar is not text, so the chord says where it works.
func TestMovePromptLineOffTheEditorSaysWhy(t *testing.T) {
	for _, field := range []int{formFieldTitle, formFieldAnnots} {
		m, _, _ := splitFormInTemp(t, "alpha\nbeta")
		m.focusForm(field)
		got := typeInForm(t, m, altArrow(tea.KeyDown, 0))
		if !strings.Contains(got.formNote, "works in the prompt") {
			t.Errorf("from field %d the note is %q, want it to name the prompt", field, got.formNote)
		}
		if got.promptArea.Value() != "alpha\nbeta" {
			t.Errorf("from field %d the prompt changed to %q", field, got.promptArea.Value())
		}
	}
}

// TestShiftAltArrowsSelect is the other half of the pair: with shift held the
// same keys extend the selection a line at a time rather than moving anything.
//
// This is the regression the alt strip in promptSelectionKey exists for. Without
// it the gesture would hand the editor an alt+↓ — the line move — and "extend
// the selection down" would drag the line out from under the highlight instead.
func TestShiftAltArrowsSelect(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "alpha\nbeta\ngamma")
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 0)

	m = typeInForm(t, m, altArrow(tea.KeyDown, tea.ModShift))
	if m.promptArea.Value() != "alpha\nbeta\ngamma" {
		t.Fatalf("shift+alt+↓ moved a line: %q", m.promptArea.Value())
	}
	if got := m.selectedPromptText(); got != "alpha\n" {
		t.Errorf("selected %q, want the first line", got)
	}

	// The anchor holds across a run, so the span grows rather than re-anchoring.
	m = typeInForm(t, m, altArrow(tea.KeyDown, tea.ModShift))
	if got := m.selectedPromptText(); got != "alpha\nbeta\n" {
		t.Errorf("selected %q after a second press, want two lines", got)
	}
	m = typeInForm(t, m, altArrow(tea.KeyUp, tea.ModShift))
	if got := m.selectedPromptText(); got != "alpha\n" {
		t.Errorf("selected %q after shift+alt+↑, want the span shrunk back", got)
	}
}

// TestShiftAltLeftRightStillSelectsWords: the alt strip is for the vertical pair
// only. Horizontally, alt is the textarea's own word motion — which is exactly
// what shift+alt+←/→ is for — so taking it off there would turn word selection
// into character selection.
func TestShiftAltLeftRightStillSelectsWords(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "alpha beta gamma")
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 16) // the end of the value

	m = typeInForm(t, m, altArrow(tea.KeyLeft, tea.ModShift))
	if got := m.selectedPromptText(); got != "gamma" {
		t.Errorf("shift+alt+← selected %q, want the word", got)
	}
}

// TestPromptLineMoveKey: only the bare alt pair is a move. Shift makes it a
// selection gesture (taken one branch earlier) and the horizontal pair is the
// textarea's word motion, so neither may answer here.
func TestPromptLineMoveKey(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  tea.KeyPressMsg
		dir  int
		ok   bool
	}{
		{"alt+up", altArrow(tea.KeyUp, 0), -1, true},
		{"alt+down", altArrow(tea.KeyDown, 0), 1, true},
		{"shift+alt+up", altArrow(tea.KeyUp, tea.ModShift), 0, false},
		{"shift+alt+down", altArrow(tea.KeyDown, tea.ModShift), 0, false},
		{"alt+left", altArrow(tea.KeyLeft, 0), 0, false},
		{"a bare down", tea.KeyPressMsg{Code: tea.KeyDown}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, ok := promptLineMoveKey(tc.msg)
			if dir != tc.dir || ok != tc.ok {
				t.Errorf("= (%d, %v), want (%d, %v)", dir, ok, tc.dir, tc.ok)
			}
		})
	}
}

// TestMovePromptRowBlock is the reorder on its own — the one place an off-by-one
// would swap the wrong pair of lines.
func TestMovePromptRowBlock(t *testing.T) {
	for _, tc := range []struct {
		name        string
		block       []string
		first, last int
		delta       int
		want        []string
	}{
		{"one line down", []string{"b", "c"}, 0, 0, 1, []string{"c", "b"}},
		{"one line up", []string{"a", "b"}, 1, 1, -1, []string{"b", "a"}},
		{"a block down", []string{"b", "c", "d"}, 0, 1, 1, []string{"d", "b", "c"}},
		{"a block up", []string{"a", "b", "c"}, 1, 2, -1, []string{"b", "c", "a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := movePromptRowBlock(tc.block, tc.first, tc.last, tc.delta)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("= %q, want %q", got, tc.want)
			}
		})
	}
}
