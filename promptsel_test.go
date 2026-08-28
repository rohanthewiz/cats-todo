package main

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// shiftKey is a caret motion with shift held — the gesture that grows a
// selection from the keyboard.
func shiftKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Mod: tea.ModShift}
}

// typeInForm drives one key at the form, the way a terminal reports it.
func typeInForm(t *testing.T, m model, msg tea.KeyPressMsg) model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(model)
}

// dragFormTo is the pointer moving inside the editor with the button still down.
func dragFormTo(t *testing.T, m model, x, y int) model {
	t.Helper()
	next, _ := m.Update(tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft})
	return next.(model)
}

func releaseForm(t *testing.T, m model, x, y int) model {
	t.Helper()
	next, _ := m.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	return next.(model)
}

// TestPromptDisplayLinesMatchTheTextarea is the load-bearing test of this
// feature. wrapPromptRunes is a transcription of a function the library keeps
// private, and the highlight is painted on the cells that wrap produces — so if
// the copy and the original ever disagree about where a long line breaks, the
// selection lands on the wrong characters.
//
// The library is used as the oracle rather than a table of expected wraps.
// LineInfo answers, for whatever line the caret is on, which column of the
// logical row that display line starts at and how many display lines down from
// the row's top it is. Walking the caret across every column of every row and
// checking both answers against the table pins the copy to the original at every
// seam, including the ones a hand-written table would never think to include.
func TestPromptDisplayLinesMatchTheTextarea(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("wrapme ", 12))
	for _, tc := range []struct {
		name  string
		value string
		width int
	}{
		{"a short line", "hello world", 40},
		{"several short lines", "one\ntwo\nthree", 40},
		{"an empty row in the middle", "one\n\nthree", 40},
		{"a line that wraps", long, 20},
		{"a line that wraps many times", long, 9},
		{"a wrapped line above a short one", long + "\ntail", 20},
		{"a word longer than the pane", "supercalifragilistic", 8},
		{"runs of spaces", "a     b     c     d", 7},
		{"trailing spaces", "alpha beta   ", 9},
		{"double-width glyphs", "日本語のテキストです", 9},
		{"an empty value", "", 20},
		{"only newlines", "\n\n\n", 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ta := textarea.New()
			ta.ShowLineNumbers = false
			ta.CharLimit = 0
			ta.SetWidth(tc.width)
			ta.SetHeight(20)
			ta.SetValue(tc.value)

			lines := promptDisplayLines(ta)
			rows := strings.Split(ta.Value(), "\n")

			// rowFirst[r] is the index in lines of row r's first display line.
			rowFirst := make(map[int]int, len(rows))
			rowStart := make(map[int]int, len(rows))
			for i, d := range lines {
				if _, seen := rowFirst[d.row]; !seen {
					rowFirst[d.row], rowStart[d.row] = i, d.start
				}
			}
			if len(rowFirst) != len(rows) {
				t.Fatalf("table covers %d rows, the value has %d", len(rowFirst), len(rows))
			}

			for r, row := range rows {
				walkPromptCaretToRow(t, &ta, r)
				for col := 0; col <= len([]rune(row)); col++ {
					ta.SetCursorColumn(col)
					li := ta.LineInfo()
					d := rowFirst[r] + li.RowOffset
					if d >= len(lines) {
						t.Fatalf("row %d col %d: the library puts the caret %d lines into the row, "+
							"the table has only %d lines in total", r, col, li.RowOffset, len(lines))
					}
					if got := lines[d].start - rowStart[r]; got != li.StartColumn {
						t.Fatalf("row %d col %d: display line %d starts at column %d in the table, "+
							"the library says %d\ntable: %+v", r, col, d, got, li.StartColumn, lines)
					}
					if lines[d].row != r {
						t.Fatalf("row %d col %d resolved to a display line belonging to row %d",
							r, col, lines[d].row)
					}
				}
			}

			// Every rune of the value is shown exactly once: the display lines
			// tile it end to end with no gap and no overlap. Without that a span
			// of offsets could not be turned into a set of cells at all.
			want := 0
			for _, d := range lines {
				if d.start != want {
					t.Fatalf("display line starting at %d leaves a gap after %d\ntable: %+v", d.start, want, lines)
				}
				want = d.end()
				if d.row < len(rows)-1 && want == d.start+d.length && d.end() == rowStart[d.row]+len([]rune(rows[d.row])) {
					want++ // step over the newline that ends the row
				}
			}
			if want != len([]rune(tc.value)) {
				t.Errorf("the table covers %d runes, the value has %d", want, len([]rune(tc.value)))
			}
		})
	}
}

// walkPromptCaretToRow moves the caret to the top of logical row r using the
// library's own one-display-line-at-a-time motion, which is the only public way
// to change the caret's row.
func walkPromptCaretToRow(t *testing.T, ta *textarea.Model, r int) {
	t.Helper()
	ta.MoveToBegin()
	for range 1000 {
		if ta.Line() == r {
			return
		}
		before := ta.Line()
		ta.CursorDown()
		if ta.Line() == before && before != r {
			continue // still inside a soft-wrapped row, which is progress
		}
		if ta.Line() > r {
			t.Fatalf("walked past row %d to %d", r, ta.Line())
		}
	}
	t.Fatalf("could not reach row %d", r)
}

// TestPromptShiftArrowsSelect is the keyboard half of the gesture: shift+→ grows
// a selection from where the caret was when shift was first held, and shift+←
// shrinks it back through zero and out the other side.
func TestPromptShiftArrowsSelect(t *testing.T) {
	m := withForm(t, "", "alpha beta", 100, 40)
	m.promptArea.SetCursorColumn(0)

	for range 5 {
		m = typeInForm(t, m, shiftKey(tea.KeyRight))
	}
	if got := m.selectedPromptText(); got != "alpha" {
		t.Errorf("five shift+→ selected %q, want %q", got, "alpha")
	}

	// Back the other way: the anchor has not moved, so the span shrinks rather
	// than re-anchoring at the caret.
	for range 3 {
		m = typeInForm(t, m, shiftKey(tea.KeyLeft))
	}
	if got := m.selectedPromptText(); got != "al" {
		t.Errorf("three shift+← left %q selected, want %q", got, "al")
	}

	// Past the anchor the span inverts, which is the whole reason lo and hi are
	// sorted rather than assumed.
	for range 4 {
		m = typeInForm(t, m, shiftKey(tea.KeyLeft))
	}
	if got := m.selectedPromptText(); got != "" {
		t.Errorf("walking back past the start selected %q, want nothing", got)
	}
}

// TestPromptSelectionRidesEveryMotion is the payoff for stripping shift instead
// of reimplementing the motions: every chord the textarea already binds to
// caret movement extends a selection, without this file knowing what any of them
// do. Word motion and the line ends come along for free.
func TestPromptSelectionRidesEveryMotion(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
		want string
	}{
		{"shift+end takes the rest of the line", shiftKey(tea.KeyEnd), "alpha beta gamma"},
		{"shift+alt+→ takes a word", tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift | tea.ModAlt}, "alpha"},
		{"shift+↓ takes the line and its break", shiftKey(tea.KeyDown), "alpha beta gamma\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := withForm(t, "", "alpha beta gamma\nsecond line", 100, 40)
			m.promptArea.MoveToBegin()
			m = typeInForm(t, m, tc.key)
			if got := m.selectedPromptText(); got != tc.want {
				t.Errorf("selected %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPromptShiftArrowsSelectAcrossLines covers the case the offset arithmetic
// exists for: a span that crosses a newline is one range, not a pair of
// row/column points, and the newline itself is part of what gets copied.
func TestPromptShiftArrowsSelectAcrossLines(t *testing.T) {
	m := withForm(t, "", "one\ntwo\nthree", 100, 40)
	m.promptArea.MoveToBegin()

	m = typeInForm(t, m, shiftKey(tea.KeyDown))
	m = typeInForm(t, m, shiftKey(tea.KeyEnd))
	if got := m.selectedPromptText(); got != "one\ntwo" {
		t.Errorf("shift+↓ then shift+end selected %q, want %q", got, "one\ntwo")
	}
}

// TestPromptDragSelects is the pointer half: press, sweep, release. The press
// alone must not select anything — a plain click on a word is how the caret is
// placed, and it would be unusable if it left a highlight behind.
func TestPromptDragSelects(t *testing.T) {
	m := withForm(t, "", "alpha beta gamma", 100, 40)
	gutter := promptGutterWidth(m.promptArea)

	m = clickForm(m, gutter+6, formPromptRow)
	if got := m.selectedPromptText(); got != "" {
		t.Errorf("a bare press selected %q, want nothing", got)
	}
	if !m.promptSelDrag {
		t.Fatal("a press in the editor should arm a possible sweep")
	}

	m = dragFormTo(t, m, gutter+10, formPromptRow)
	if got := m.selectedPromptText(); got != "beta" {
		t.Errorf("sweeping four cells selected %q, want %q", got, "beta")
	}

	// The release ends the gesture but not the selection: what was swept stays
	// selected, which is the only reason to have swept it.
	m = releaseForm(t, m, gutter+10, formPromptRow)
	if m.promptSelDrag {
		t.Error("the release should have ended the sweep")
	}
	if got := m.selectedPromptText(); got != "beta" {
		t.Errorf("after the release %q is selected, want %q", got, "beta")
	}
}

// TestPromptDragBelowTheEditorClamps: a sweep that runs off the bottom of the
// editor on its way to the toolbar means "everything down to here". Dropping
// those motions would freeze the highlight while the button is plainly still
// moving.
func TestPromptDragBelowTheEditorClamps(t *testing.T) {
	m := withForm(t, "", "one\ntwo\nthree", 100, 40)
	gutter := promptGutterWidth(m.promptArea)

	m = clickForm(m, gutter, formPromptRow)
	m = dragFormTo(t, m, gutter+40, formPromptRow+200)
	if got := m.selectedPromptText(); got != "one\ntwo\nthree" {
		t.Errorf("sweeping past the bottom selected %q, want the whole value", got)
	}
}

// TestPromptSelectionCopy: ctrl+c copies while something is selected, and still
// quits when nothing is.
func TestPromptSelectionCopy(t *testing.T) {
	m := withForm(t, "", "alpha beta", 100, 40)
	m.promptArea.SetCursorColumn(0)
	for range 5 {
		m = typeInForm(t, m, shiftKey(tea.KeyRight))
	}

	copied, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	got := copied.(model)
	if got.quitting {
		t.Fatal("ctrl+c over a selection quit the manager instead of copying")
	}
	if cmd == nil {
		t.Error("ctrl+c over a selection produced no clipboard command")
	}
	if !strings.Contains(got.formNote, "copied 5") {
		t.Errorf("form note is %q, want it to report five characters copied", got.formNote)
	}
	// Copying is not a gesture that ends anything.
	if sel := got.selectedPromptText(); sel != "alpha" {
		t.Errorf("after the copy %q is selected, want it left alone", sel)
	}

	bare := withForm(t, "", "alpha beta", 100, 40)
	quit, _ := bare.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !quit.(model).quitting {
		t.Error("ctrl+c with nothing selected should still quit")
	}
}

// TestPromptSelectionEndsOnTheNextKey: a selection lasts exactly as long as the
// run of keys building it. A highlight left standing over text the caret has
// walked away from is a lie about what the next ctrl+c would copy.
func TestPromptSelectionEndsOnTheNextKey(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"a plain arrow", tea.KeyPressMsg{Code: tea.KeyLeft}},
		{"a typed character", tea.KeyPressMsg{Code: 'x', Text: "x"}},
		{"tab to the other field", tea.KeyPressMsg{Code: tea.KeyTab}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := withForm(t, "", "alpha beta", 100, 40)
			m.promptArea.SetCursorColumn(0)
			m = typeInForm(t, m, shiftKey(tea.KeyRight))
			if m.selectedPromptText() == "" {
				t.Fatal("nothing selected to begin with")
			}
			m = typeInForm(t, m, tc.key)
			if got := m.selectedPromptText(); got != "" {
				t.Errorf("%q is still selected after %s", got, tc.name)
			}
		})
	}
}

// TestPromptSelectionEndsOnAClick: the pointer is about to say where the caret
// goes next, so the selection it is leaving behind goes with it.
func TestPromptSelectionEndsOnAClick(t *testing.T) {
	m := withForm(t, "", "alpha beta", 100, 40)
	m.promptArea.SetCursorColumn(0)
	for range 5 {
		m = typeInForm(t, m, shiftKey(tea.KeyRight))
	}
	m = clickForm(m, promptGutterWidth(m.promptArea)+8, formPromptRow)
	if got := m.selectedPromptText(); got != "" {
		t.Errorf("%q is still selected after a click elsewhere", got)
	}
}

// TestPromptSelectionIsDrawn checks the overlay against the editor's own view:
// the highlight is added, and nothing else is. Same number of lines, same cells,
// same characters — only the color under some of them changes.
func TestPromptSelectionIsDrawn(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prompt string
		width  int
		steps  int
		key    rune
	}{
		{"inside one line", "alpha beta gamma", 100, 5, tea.KeyRight},
		{"across a newline", "one\ntwo\nthree", 100, 2, tea.KeyDown},
		{"across a soft wrap", strings.TrimSpace(strings.Repeat("wrapme ", 12)), 30, 3, tea.KeyDown},
		{"backwards from the end", "alpha beta gamma", 100, 4, tea.KeyLeft},
		// The caret ends up past the last cell the editor drew, which is the one
		// place the overlay could invent a cell to put it on.
		{"to the end of a line that fills the box", "0123456789012345678901234567890123", 40, 34, tea.KeyRight},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := withForm(t, "", tc.prompt, tc.width, 40)
			if tc.key == tea.KeyLeft {
				m.promptArea.MoveToEnd()
			} else {
				m.promptArea.MoveToBegin()
			}
			plainView := m.promptEditorView()

			for range tc.steps {
				m = typeInForm(t, m, shiftKey(tc.key))
			}
			if m.selectedPromptText() == "" {
				t.Fatal("nothing selected, so there is no overlay to check")
			}
			selView := m.promptEditorView()

			plainLines, selLines := strings.Split(plainView, "\n"), strings.Split(selView, "\n")
			if a, b := len(plainLines), len(selLines); a != b {
				t.Fatalf("the overlay changed the editor from %d lines to %d — the toolbar's row moves with it", a, b)
			}
			for i, line := range selLines {
				was := plainLines[i]
				if got, want := ansi.Strip(line), ansi.Strip(was); got != want {
					t.Errorf("line %d reads %q under the overlay, %q without it", i, got, want)
				}
				if got, want := lipgloss.Width(line), lipgloss.Width(was); got != want {
					t.Errorf("line %d is %d cells under the overlay, %d without it", i, got, want)
				}
			}
			if !strings.Contains(selView, ansiOf(promptSelStyle)) {
				t.Error("the selection field is nowhere in the drawn editor")
			}
			if strings.Contains(plainView, ansiOf(promptSelStyle)) {
				t.Error("the selection field is drawn with nothing selected")
			}
		})
	}
}

// ansiOf is the escape sequence a style opens with, which is what a rendered
// line is searched for to prove the style was applied.
func ansiOf(st lipgloss.Style) string {
	rendered := st.Render("x")
	before, _, ok := strings.Cut(rendered, "x")
	if !ok {
		return rendered
	}
	return before
}

// TestPromptSelectionIsDrawnWhenScrolled: the display-line table covers the
// whole value, but only a window of it is on screen. The line drawn at screen
// row y is table entry ScrollYOffset()+y, and getting that seam wrong would put
// the highlight on the right characters of the wrong lines — which only shows up
// once the editor has scrolled, so it needs its own case.
func TestPromptSelectionIsDrawnWhenScrolled(t *testing.T) {
	// Forty lines into an editor a dozen tall, so a selection walked down from
	// the top runs off the bottom and takes the view with it.
	var rows []string
	for i := range 40 {
		rows = append(rows, string(rune('a'+i%26))+"-line")
	}
	m := withForm(t, "", strings.Join(rows, "\n"), 100, 26)
	m.promptArea.MoveToBegin()
	for range 30 {
		m = typeInForm(t, m, shiftKey(tea.KeyDown))
	}
	if m.promptArea.ScrollYOffset() == 0 {
		t.Fatal("the editor never scrolled, so this test proves nothing")
	}

	top := m.promptArea.ScrollYOffset()
	drawn := strings.Split(m.promptEditorView(), "\n")
	lit := ansiOf(promptSelStyle)
	for y, line := range drawn {
		d := top + y
		// Every visible line up to the caret's is inside the selection; the ones
		// past it are not.
		want := d < 30
		if got := strings.Contains(line, lit); got != want {
			t.Errorf("screen row %d (display line %d) highlighted=%v, want %v: %q",
				y, d, got, want, ansi.Strip(line))
		}
	}
}

// TestPromptSelectionSurvivesAResize covers the anchor outliving the thing it
// points into. The anchor is an offset, not a screen position, so a narrower
// pane rewraps the editor under it without moving what is selected.
func TestPromptSelectionSurvivesAResize(t *testing.T) {
	m := withForm(t, "", strings.TrimSpace(strings.Repeat("wrapme ", 12)), 100, 40)
	m.promptArea.MoveToBegin()
	for range 6 {
		m = typeInForm(t, m, shiftKey(tea.KeyRight))
	}
	before := m.selectedPromptText()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 40})
	after := next.(model)
	if got := after.selectedPromptText(); got != before {
		t.Errorf("after a resize %q is selected, want %q", got, before)
	}
	// And the overlay still draws inside the narrower box.
	for line := range strings.SplitSeq(after.promptEditorView(), "\n") {
		if w := lipgloss.Width(line); w > 40 {
			t.Errorf("a drawn line is %d cells wide in a 40-cell pane: %q", w, ansi.Strip(line))
		}
	}
}

// TestPromptSelectionOnlyInTheEditor: the title field has no selection of its
// own, so shift+→ there must not be swallowed as a selection gesture — it falls
// through to the field, and ctrl+c still quits.
func TestPromptSelectionOnlyInTheEditor(t *testing.T) {
	m := withForm(t, "a title", "alpha beta", 100, 40)
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}) // back a stop, onto the title
	if m.formFocus != formFieldTitle {
		t.Fatal("shift+tab did not move the focus to the title")
	}
	m = typeInForm(t, m, shiftKey(tea.KeyLeft))
	if m.promptSel.active {
		t.Error("shift+← in the title field anchored a selection in the editor")
	}
	quit, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !quit.(model).quitting {
		t.Error("ctrl+c in the title field should quit")
	}
}

// selectAlpha returns the add form holding "alpha beta" with the first word
// swept from the keyboard — the state every overwrite test starts from.
func selectAlpha(t *testing.T) model {
	t.Helper()
	m := withForm(t, "", "alpha beta", 100, 40)
	m.promptArea.SetCursorColumn(0)
	for range 5 {
		m = typeInForm(t, m, shiftKey(tea.KeyRight))
	}
	if got := m.selectedPromptText(); got != "alpha" {
		t.Fatalf("setup selected %q, want %q", got, "alpha")
	}
	return m
}

// TestPromptTypingReplacesTheSelection is the rule a selection is worth having
// for: what is highlighted is what the next character lands on. Before this, the
// anchor was simply dropped and the typed character went in beside the run it
// was meant to replace.
func TestPromptTypingReplacesTheSelection(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
		want string
	}{
		{"a letter", tea.KeyPressMsg{Code: 'x', Text: "x"}, "x beta"},
		{"a capital", tea.KeyPressMsg{Code: 'x', ShiftedCode: 'X', Mod: tea.ModShift, Text: "X"}, "X beta"},
		{"a space", tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, "  beta"},
		{"enter, which is a newline here", tea.KeyPressMsg{Code: tea.KeyEnter}, "\n beta"},
		{"alt+enter, the same newline", tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt}, "\n beta"},
		{"@, which also opens the file picker", tea.KeyPressMsg{Code: '@', Text: "@"}, "@ beta"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := typeInForm(t, selectAlpha(t), tc.key)
			if got := m.promptArea.Value(); got != tc.want {
				t.Errorf("typing over the selection left %q, want %q", got, tc.want)
			}
			if got := m.selectedPromptText(); got != "" {
				t.Errorf("%q is still selected after typing over it", got)
			}
		})
	}
}

// TestPromptTypingLandsWhereTheSelectionWas: the replacement goes in at the
// start of what was removed, not wherever the caret happened to be. A selection
// swept backwards ends with the caret at its *low* end, so both directions have
// to put the character in the same place.
func TestPromptTypingLandsWhereTheSelectionWas(t *testing.T) {
	m := withForm(t, "", "alpha beta", 100, 40)
	m.promptArea.SetCursorColumn(5) // just past "alpha"
	for range 5 {
		m = typeInForm(t, m, shiftKey(tea.KeyLeft))
	}
	if got := m.selectedPromptText(); got != "alpha" {
		t.Fatalf("sweeping backwards selected %q, want %q", got, "alpha")
	}
	m = typeInForm(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if got := m.promptArea.Value(); got != "x beta" {
		t.Errorf("typing over a backwards selection left %q, want %q", got, "x beta")
	}
	if got := promptCaretOffset(m.promptArea); got != 1 {
		t.Errorf("caret at %d after the replacement, want 1 (just past what was typed)", got)
	}
}

// TestPromptBackspaceDeletesTheSelection: a delete key with a highlight standing
// takes out the highlight and nothing more — it must not also eat the character
// beside it, which is what forwarding the key to the editor after dropping the
// anchor would do.
func TestPromptBackspaceDeletesTheSelection(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"backspace", tea.KeyPressMsg{Code: tea.KeyBackspace}},
		{"ctrl+h, backspace's other spelling", tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl}},
		{"delete", tea.KeyPressMsg{Code: tea.KeyDelete}},
		{"ctrl+d, delete's other spelling", tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}},
		{"alt+backspace, the word behind", tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModAlt}},
		{"alt+d, the word ahead", tea.KeyPressMsg{Code: 'd', Mod: tea.ModAlt}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := typeInForm(t, selectAlpha(t), tc.key)
			if got := m.promptArea.Value(); got != " beta" {
				t.Errorf("%s over the selection left %q, want %q", tc.name, got, " beta")
			}
			if got := m.selectedPromptText(); got != "" {
				t.Errorf("%q is still selected after %s", got, tc.name)
			}
		})
	}
}

// TestPromptSweptSelectionIsTypedOver: the pointer's half of the gesture ends up
// in the same place as the keyboard's, since both leave the same anchor behind.
func TestPromptSweptSelectionIsTypedOver(t *testing.T) {
	m := withForm(t, "", "alpha beta", 100, 40)
	gutter := promptGutterWidth(m.promptArea)
	m = clickForm(m, gutter, formPromptRow)
	m = dragFormTo(t, m, gutter+5, formPromptRow)
	m = releaseForm(t, m, gutter+5, formPromptRow)
	if got := m.selectedPromptText(); got != "alpha" {
		t.Fatalf("the sweep selected %q, want %q", got, "alpha")
	}
	m = typeInForm(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if got := m.promptArea.Value(); got != "x beta" {
		t.Errorf("typing over a swept selection left %q, want %q", got, "x beta")
	}
}

// TestPromptPasteReplacesTheSelection: a paste is an insertion like any other,
// and arrives on its own message rather than through the key path — so it needs
// saying separately that it lands *on* the selection.
func TestPromptPasteReplacesTheSelection(t *testing.T) {
	m := selectAlpha(t)
	next, _ := m.Update(tea.PasteMsg{Content: "omega"})
	m = next.(model)
	if got := m.promptArea.Value(); got != "omega beta" {
		t.Errorf("pasting over the selection left %q, want %q", got, "omega beta")
	}
	if got := m.selectedPromptText(); got != "" {
		t.Errorf("%q is still selected after the paste", got)
	}
}

// TestPromptSelectionSurvivesKeysThatAreNotEdits: only insertions and deletes
// act on the highlight. A motion still just drops it, and the chords the form
// claims for itself must not lose the text they were pressed over.
func TestPromptSelectionSurvivesKeysThatAreNotEdits(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"a plain arrow", tea.KeyPressMsg{Code: tea.KeyLeft}},
		{"tab to the annotation bar", tea.KeyPressMsg{Code: tea.KeyTab}},
		{"ctrl+c, which copies it", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := typeInForm(t, selectAlpha(t), tc.key)
			if got := m.promptArea.Value(); got != "alpha beta" {
				t.Errorf("%s changed the value to %q", tc.name, got)
			}
		})
	}
}
