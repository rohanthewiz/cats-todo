// promptlimit_test.go — the prompt editor has no logical-line cap.
package main

import (
	"strings"
	"testing"
	"time"

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

// longOneLiner is a single logical row of about n runes, words and spaces so
// the editor soft-wraps it the way a real paste of prose wraps.
func longOneLiner(n int) string {
	const word = "lorem ipsum dolor sit amet "
	return strings.Repeat(word, n/len(word))
}

// TestPromptCaretOffsetCrossesAWrappedRow pins where setPromptCaretOffset lands
// now that it hops a logical row per step (CursorEnd, then CursorDown) instead
// of walking display lines. The caret must still land on the right row and
// column: inside the wrapped row itself, on the row after it, and two rows on,
// past a second wrapped row.
func TestPromptCaretOffsetCrossesAWrappedRow(t *testing.T) {
	long := longOneLiner(2000)
	n := len([]rune(long))
	m, _ := openAddForm(t, "", long+"\nbeta\n"+long+"\ngamma")
	m.promptArea.SetWidth(30) // the long rows span many display lines each

	for _, tc := range []struct {
		name     string
		off      int
		row, col int
	}{
		{"mid wrapped row", 777, 0, 777},
		{"end of wrapped row", n, 0, n},
		{"row after it", n + 1 + 2, 1, 2},
		{"past two wrapped rows", 2*n + 2 + 5 + 3, 3, 3},
	} {
		setPromptCaretOffset(&m.promptArea, tc.off)
		if l, c := m.promptArea.Line(), m.promptArea.Column(); l != tc.row || c != tc.col {
			t.Errorf("%s: caret at row %d col %d, want row %d col %d", tc.name, l, c, tc.row, tc.col)
		}
		if got := promptCaretOffset(m.promptArea); got != tc.off {
			t.Errorf("%s: caret offset reads back %d, want %d", tc.name, got, tc.off)
		}
	}
}

// TestEnterOnALongOneLinerIsCheap: enter at the end of a 20k-rune single line
// took ~140ms (N-024). newlineCarryingIndent edits through replacePromptRunes,
// whose caret placement walked the long row one display line at a time, and
// each step re-hashed the whole row in the library's wrap memo — quadratic in
// the row's length. The hop per logical row brought it to ~1.5ms. The bound is
// deliberately loose (and the best of three runs is taken) so a busy machine
// or -race does not trip it, while the old walk still would.
func TestEnterOnALongOneLinerIsCheap(t *testing.T) {
	long := longOneLiner(20000)
	best := time.Hour
	for range 3 {
		m, _ := openAddForm(t, "", "")
		m.focusForm(formFieldPrompt)
		m.promptArea.SetValue(long)
		start := time.Now()
		m = typeInForm(t, m, enterKey(0))
		best = min(best, time.Since(start))
		if want := long + "\n"; m.promptArea.Value() != want {
			t.Fatalf("enter left %d bytes, want the line plus a newline", len(m.promptArea.Value()))
		}
		if l, c := m.promptArea.Line(), m.promptArea.Column(); l != 1 || c != 0 {
			t.Fatalf("caret at row %d col %d after enter, want row 1 col 0", l, c)
		}
	}
	if best > 50*time.Millisecond {
		t.Errorf("enter on a %d-rune line took %v, want well under 50ms", len(long), best)
	}
}

// BenchmarkEnterOnALongOneLiner measures the case N-024 was raised on.
func BenchmarkEnterOnALongOneLiner(b *testing.B) {
	long := longOneLiner(20000)
	m0, _ := openAddForm(b, "", "")
	m0.focusForm(formFieldPrompt)
	for b.Loop() {
		b.StopTimer()
		m := m0
		m.promptArea.SetValue(long)
		b.StartTimer()
		next, _ := m.Update(enterKey(0))
		_ = next
	}
}
