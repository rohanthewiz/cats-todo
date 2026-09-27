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

// walkPromptLines is how promptLines measured the display lines before N-068:
// CursorDown from the top, one display line per step, until a step stops
// moving. It is quadratic in the line count, but it is the library's own
// motion with nothing inferred, so it is kept as the oracle the probe build is
// checked against.
func walkPromptLines(ta textarea.Model) []promptLine {
	var lines []promptLine
	seen := map[promptLine]bool{}
	probe := ta
	probe.MoveToBegin()
	for range 20000 {
		key := promptLine{probe.Line(), probe.LineInfo().StartColumn}
		if seen[key] {
			break
		}
		seen[key] = true
		lines = append(lines, key)
		probe.CursorDown()
	}
	return lines
}

// clickValues are the shapes a display-line table can get wrong: plain rows,
// empty rows (leading, doubled, trailing), rows that wrap many times, a row
// that ends exactly on a wrap, wide runes, and runs of spaces the wrap eats.
func clickValues() []string {
	long := longOneLiner(300)
	return []string{
		"",
		"one line",
		"\nafter an empty first row\n\nand a doubled one\n",
		long + "\nshort\n" + long,
		strings.Repeat("abcdefghij", 12) + "\nnext",
		"漢字の行が折り返す " + strings.Repeat("漢字 ", 40) + "\nascii",
		"spaces      between      words " + strings.Repeat("x    ", 30),
		linesOf(50),
	}
}

// TestPromptLinesMatchesTheWalk: the probe build (one appended row at a time,
// LineInfo per display line) must produce the same table, entry for entry,
// as the old CursorDown walk. Several widths are used, so rows wrap at
// different places. None of them fills a row to an exact multiple of the
// text width, which is where the walk itself is wrong; that case has its own
// test, TestPromptClickReachesPastAnExactlyFilledRow.
func TestPromptLinesMatchesTheWalk(t *testing.T) {
	for _, width := range []int{24, 45, 100} {
		for i, v := range clickValues() {
			ta := withForm(t, "", v, width, 30).promptArea
			want := walkPromptLines(ta)
			got, index := promptLines(ta)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("width %d value %d: table\n got %v\nwant %v", width, i, got, want)
				continue
			}
			for j, l := range got {
				if index[l] != j {
					t.Errorf("width %d value %d: index[%v] = %d, want %d", width, i, l, index[l], j)
				}
			}
		}
	}
}

// walkPlacePromptCursor is placePromptCursor before N-068: the table from the
// walk, then the caret stepped to the bottom, up to y0 and down to d, one
// display line per step, then the same column arithmetic. It is the oracle for
// where a click must leave the caret and the scroll.
func walkPlacePromptCursor(m *model, x, row int) {
	y0 := m.promptArea.ScrollYOffset()
	lines := walkPromptLines(m.promptArea)
	if len(lines) == 0 {
		return
	}
	index := map[promptLine]int{}
	for i, l := range lines {
		index[l] = i
	}
	last := len(lines) - 1
	y0 = min(y0, last)
	d := min(y0+max(row, 0), last)
	stepPromptTo(&m.promptArea, index, last)
	stepPromptTo(&m.promptArea, index, y0)
	stepPromptTo(&m.promptArea, index, d)
	li := m.promptArea.LineInfo()
	rowRunes := promptRowRunes(m.promptArea)
	start := min(li.StartColumn, len(rowRunes))
	avail := min(li.Width, len(rowRunes)-start)
	if li.RowOffset+1 < li.Height && avail > 0 {
		avail--
	}
	seg := rowRunes[start : start+max(avail, 0)]
	m.promptArea.SetCursorColumn(start + colAtWidth(seg, x-promptGutterWidth(m.promptArea)))
}

// TestPromptClickMatchesTheWalk: a click must land the caret on the same row
// and column, and leave the view scrolled to the same line, as the old walk
// did. That is checked from views scrolled to the top, the middle and the
// bottom, clicking every screen row (and one past the end) at the gutter,
// mid-line and far right. Both sides render between steps, since the viewport
// clamps a scroll to the content of its last frame. The widths (text widths
// 25 and 74) keep clear of an exactly filled row, where the walk is wrong.
func TestPromptClickMatchesTheWalk(t *testing.T) {
	for i, v := range clickValues() {
		for _, width := range []int{31, 80} {
			base := withForm(t, "", v, width, 20)
			n := len([]rune(v))
			for _, from := range []int{0, n / 2, n} {
				for row := range base.promptArea.Height() + 1 {
					for _, x := range []int{0, 7, width} {
						got := withForm(t, "", v, width, 20)
						want := withForm(t, "", v, width, 20)
						for _, mm := range []*model{&got, &want} {
							_ = mm.promptArea.View()
							setPromptCaretOffset(&mm.promptArea, from)
							_ = mm.promptArea.View()
						}
						got.placePromptCursor(x, row)
						walkPlacePromptCursor(&want, x, row)
						g, w := &got.promptArea, &want.promptArea
						if g.Line() != w.Line() || g.Column() != w.Column() || g.ScrollYOffset() != w.ScrollYOffset() {
							t.Fatalf("value %d width %d from %d, click row %d x %d: caret %d:%d scroll %d, the walk gave %d:%d scroll %d",
								i, width, from, row, x, g.Line(), g.Column(), g.ScrollYOffset(), w.Line(), w.Column(), w.ScrollYOffset())
						}
					}
				}
			}
		}
	}
}

// TestPromptClickIsLinearInRows: a click walked the whole value one display
// line at a time, and every step re-counted the lines above the caret, so a
// click in a 2,000-line prompt took ~1.3s and one in 9,999 lines far longer
// (N-068). The bound is loose enough for -race; the old walk misses it by
// orders of magnitude.
func TestPromptClickIsLinearInRows(t *testing.T) {
	value := linesOf(promptMaxLines - 1)
	m := withForm(t, "", value, 100, 40)
	_ = m.promptArea.View()
	setPromptCaretOffset(&m.promptArea, len([]rune(value))/2) // mid-value
	_ = m.promptArea.View()
	y0 := m.promptArea.ScrollYOffset()
	start := time.Now()
	m.placePromptCursor(promptGutterWidth(m.promptArea), 3)
	took := time.Since(start)
	if l := m.promptArea.Line(); l != y0+3 {
		t.Fatalf("click on screen row 3 put the caret on row %d, want %d", l, y0+3)
	}
	if s := m.promptArea.ScrollYOffset(); s != y0 {
		t.Errorf("click scrolled the view from %d to %d", y0, s)
	}
	if took > time.Second {
		t.Errorf("a click in %d lines took %v, want well under a second", promptMaxLines-1, took)
	}
}

// TestPromptClickReachesPastAnExactlyFilledRow: a row with no spaces whose
// length is an exact multiple of the text width wraps into full lines plus one
// empty display line after them (the library draws it; the caret goes there at
// the row's end). The old table walk stopped short on such a row. From the row's
// second-to-last line, CursorDown clamps the column to len-1, stays on the same
// line, and the walk took that for the end of the value. The empty line and
// every row below were missing, so a click on any of them landed somewhere
// else (N-068). The probe build asks LineInfo per line and gets all of them.
func TestPromptClickReachesPastAnExactlyFilledRow(t *testing.T) {
	v := strings.Repeat("abcdefghij", 12) + "\nnext" // 120 runes, then a row
	m := withForm(t, "", v, 30, 20)
	if w := m.promptArea.Width(); w != 24 {
		t.Fatalf("text width %d, want 24 so that 120 runes fill five lines exactly", w)
	}
	lines, _ := promptLines(m.promptArea)
	want := []promptLine{{0, 0}, {0, 24}, {0, 48}, {0, 72}, {0, 96}, {0, 120}, {1, 0}}
	if fmt.Sprint(lines) != fmt.Sprint(want) {
		t.Fatalf("table %v, want %v", lines, want)
	}

	// Caret at the end, so the view (4 lines tall) shows display lines 3–6.
	_ = m.promptArea.View()
	setPromptCaretOffset(&m.promptArea, len([]rune(v)))
	_ = m.promptArea.View()
	y0 := m.promptArea.ScrollYOffset()
	if y0 != 3 {
		t.Fatalf("view scrolled to %d, want 3", y0)
	}
	gutter := promptGutterWidth(m.promptArea)
	for _, tc := range []struct {
		screenRow, row, col int
	}{
		{0, 0, 72 + 2}, // "…" two cells into the line starting at 72
		{2, 0, 120},    // the empty line after the filled row
		{3, 1, 2},      // "next", two cells in
	} {
		m.placePromptCursor(gutter+2, tc.screenRow)
		if l, c := m.promptArea.Line(), m.promptArea.Column(); l != tc.row || c != tc.col {
			t.Errorf("click on screen row %d: caret %d:%d, want %d:%d", tc.screenRow, l, c, tc.row, tc.col)
		}
		if s := m.promptArea.ScrollYOffset(); s != y0 {
			t.Errorf("click on screen row %d scrolled the view to %d, want %d", tc.screenRow, s, y0)
		}
	}
}
