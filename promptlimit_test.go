// promptlimit_test.go — the prompt editor has no logical-line cap.
package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textarea"
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
// around long soft-wrapped rows, which the display-line walk (N-024) and then
// the row-hop walk (N-063) both had to cross. The caret must land on the right row and
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

// TestPromptCaretOffsetIsLinearInRows: setPromptCaretOffset used to hop one
// CursorDown per logical row, and each hop re-ran the library's
// cursorLineNumber over every row above the caret. That is quadratic in the
// line count: a caret to the end of a 9,999-line prompt took ~30s (N-063). The
// rebuild costs one pass. The bound is loose enough for -race and a busy
// machine; the old walk misses it by two orders of magnitude.
func TestPromptCaretOffsetIsLinearInRows(t *testing.T) {
	value := linesOf(promptMaxLines - 1)
	m := withForm(t, "", value, 100, 40)
	end := len([]rune(value))
	start := time.Now()
	setPromptCaretOffset(&m.promptArea, end)
	took := time.Since(start)
	if l, c := m.promptArea.Line(), m.promptArea.Column(); l != promptMaxLines-2 || c != 1 {
		t.Fatalf("caret at row %d col %d, want row %d col 1", l, c, promptMaxLines-2)
	}
	if took > time.Second {
		t.Errorf("caret to the end of %d lines took %v, want well under a second", promptMaxLines-1, took)
	}
}

// TestPromptCaretOffsetKeepsTheValueAtTheCap: the move rebuilds the value, and
// the library's insert truncates past maxLines. The line count never changes,
// so a prompt sitting exactly on the cap must come back whole, wherever the
// caret goes — including the middle, where the re-insert is at its largest.
func TestPromptCaretOffsetKeepsTheValueAtTheCap(t *testing.T) {
	value := linesOf(promptMaxLines)
	m := withForm(t, "", value, 100, 40)
	for _, off := range []int{0, len(value) / 2, len(value)} {
		setPromptCaretOffset(&m.promptArea, off)
		if m.promptArea.Value() != value {
			t.Fatalf("off %d: value now %d lines, want the %d it had", off, m.promptArea.LineCount(), promptMaxLines)
		}
		if got := promptCaretOffset(m.promptArea); got != off {
			t.Errorf("off %d: caret offset reads back %d", off, got)
		}
	}
}

// hopCaretTo is the walk setPromptCaretOffset used before N-063: from the top,
// CursorEnd then CursorDown hops one logical row per step, then the column is
// set. It is kept as the oracle for where the viewport should end up, since
// the rebuild promises the same scroll the walk produced.
func hopCaretTo(ta *textarea.Model, row, col int) {
	ta.MoveToBegin()
	for ta.Line() < row {
		at := ta.Line()
		ta.CursorEnd()
		ta.CursorDown()
		if ta.Line() <= at {
			break
		}
	}
	ta.SetCursorColumn(col)
}

// TestPromptCaretOffsetScrollsLikeTheWalk: the rebuild resets the viewport to
// the top and repositions once. That must leave the scroll the old walk left,
// from a view scrolled anywhere, onto plain rows and into a wrapped one. A
// different offset would make the editor jump on every enter.
//
// There is one deliberate difference. The walk ended with a bare
// SetCursorColumn, which does not reposition, so a caret placed deep in a
// wrapped row was scrolled for the row's *first* display line. It could sit
// off screen until the next key went through the textarea's Update. The
// rebuild scrolls to the caret's own display line. So the oracle is the walk
// followed by one repositionView, the view the library itself would settle on.
func TestPromptCaretOffsetScrollsLikeTheWalk(t *testing.T) {
	long := longOneLiner(400)
	var rows []string
	for i := range 120 {
		if i%17 == 5 {
			rows = append(rows, long)
		} else {
			rows = append(rows, fmt.Sprintf("row %d", i))
		}
	}
	value := strings.Join(rows, "\n")
	offOf := func(row, col int) int {
		off := 0
		for _, r := range rows[:row] {
			off += len([]rune(r)) + 1
		}
		return off + col
	}
	targets := [][2]int{{0, 0}, {3, 2}, {5, 250}, {60, 1}, {119, 3}, {22, 100}, {1, 0}}
	for _, from := range targets {
		for _, to := range targets {
			// Two models, not a copy of one: a textarea.Model copy shares
			// its viewport pointer, so both would scroll the same view.
			got := withForm(t, "", value, 60, 20).promptArea
			want := withForm(t, "", value, 60, 20).promptArea
			// The viewport clamps a scroll to the content its last frame
			// loaded, so each side renders as the app would between updates.
			_, _ = got.View(), want.View()
			hopCaretTo(&got, from[0], from[1])
			hopCaretTo(&want, from[0], from[1])
			_, _ = got.View(), want.View()

			setPromptCaretOffset(&got, offOf(to[0], to[1]))
			hopCaretTo(&want, to[0], to[1])
			want.SetHeight(want.Height()) // settle: see the test's comment
			if got.Line() != want.Line() || got.Column() != want.Column() {
				t.Errorf("%v → %v: caret at %d:%d, the walk put it at %d:%d",
					from, to, got.Line(), got.Column(), want.Line(), want.Column())
			}
			if got.ScrollYOffset() != want.ScrollYOffset() {
				t.Errorf("%v → %v: scrolled to %d, the walk scrolled to %d",
					from, to, got.ScrollYOffset(), want.ScrollYOffset())
			}
		}
	}
}
