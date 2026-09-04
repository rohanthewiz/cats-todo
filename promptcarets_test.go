package main

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// typeChar is one printable key as a terminal reports it — Text populated, which
// is the same test the editor uses to decide a key inserts.
func typeChar(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// caretsOver sweeps the whole prompt and drops a caret on each of its lines.
func caretsOver(t *testing.T, prompt string) model {
	t.Helper()
	m, _, _ := splitFormInTemp(t, prompt)
	m = selectWholePrompt(t, m)
	next, _ := m.dropPromptCarets()
	got := next.(model)
	if !got.carets.on {
		t.Fatalf("the carets did not go down on %q", prompt)
	}
	return got
}

// TestCaretsPrefixEveryLine is the gesture the mode exists for: three plain
// lines swept from the left margin become a markdown list in one go — which is
// then exactly the shape ✂ Split into prompts wants.
func TestCaretsPrefixEveryLine(t *testing.T) {
	m := caretsOver(t, "tag v2\nwrite the notes\nannounce it")
	if got, want := m.carets.rows, []int{0, 1, 2}; len(got) != len(want) {
		t.Fatalf("carets on rows %v, want %v", got, want)
	}
	if got, want := m.carets.cols, []int{0, 0, 0}; !slices.Equal(got, want) {
		t.Fatalf("carets at columns %v, want %v — the sweep began at the left margin", got, want)
	}

	for _, r := range "- " {
		m = typeInForm(t, m, typeChar(r))
	}
	want := "- tag v2\n- write the notes\n- announce it"
	if m.promptArea.Value() != want {
		t.Errorf("value = %q, want %q", m.promptArea.Value(), want)
	}
	if got, want := m.carets.cols, []int{2, 2, 2}; !slices.Equal(got, want) {
		t.Errorf("carets at columns %v, want them stepped past what was typed", got)
	}
	// Still on, so the next character continues the same edit on all three.
	if !m.carets.on {
		t.Error("typing ended the mode")
	}
}

// TestCaretsBackspaceUnprefixes: the reverse gesture, which is the other half of
// what a column mode gets used for.
func TestCaretsBackspaceUnprefixes(t *testing.T) {
	m := caretsOver(t, "- tag v2\n- write the notes")
	for i := range m.carets.cols {
		m.carets.cols[i] = 2 // as if "- " had just been typed
	}
	m.syncPromptCaret()

	for range 2 {
		m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	if want := "tag v2\nwrite the notes"; m.promptArea.Value() != want {
		t.Errorf("value = %q, want %q", m.promptArea.Value(), want)
	}
	if got, want := m.carets.cols, []int{0, 0}; !slices.Equal(got, want) {
		t.Errorf("carets at columns %v, want 0", got)
	}
	// And with every caret at the start of its line there is nothing behind them
	// to take out — said in words rather than joining every row onto the one
	// above, which is not what a backspace here can sensibly mean.
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if !strings.Contains(m.formNote, "start of their lines") {
		t.Errorf("form note = %q, want it to say there is nothing behind the carets", m.formNote)
	}
	if want := "tag v2\nwrite the notes"; m.promptArea.Value() != want {
		t.Errorf("value = %q, want it untouched", m.promptArea.Value())
	}
}

// TestCaretsGoalColumnSurvivesAShortLine: a row too short for the column takes
// its caret at its end and is not stranded there — the goal column is what makes
// a ragged block editable in one gesture.
func TestCaretsGoalColumnSurvivesAShortLine(t *testing.T) {
	m := caretsOver(t, "ab\nabcdef")
	for range 4 {
		m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	}
	if got, want := m.carets.cols, []int{4, 4}; !slices.Equal(got, want) {
		t.Fatalf("goal columns %v, want %v — not bounded by the shortest row", got, want)
	}
	m = typeInForm(t, m, typeChar('!'))
	if want := "ab!\nabcd!ef"; m.promptArea.Value() != want {
		t.Errorf("value = %q, want %q — the short row clamped, the long one did not", m.promptArea.Value(), want)
	}
}

// TestCaretsLineEnds: ctrl+a puts them all at the line starts, ctrl+e at the
// line ends — prefix and suffix from one mode.
func TestCaretsLineEnds(t *testing.T) {
	m := caretsOver(t, "one\nthree")
	m = typeInForm(t, m, tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	m = typeInForm(t, m, typeChar('.'))
	if want := "one.\nthree."; m.promptArea.Value() != want {
		t.Errorf("value = %q, want %q", m.promptArea.Value(), want)
	}

	m = typeInForm(t, m, tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	m = typeInForm(t, m, typeChar('>'))
	if want := ">one.\n>three."; m.promptArea.Value() != want {
		t.Errorf("value = %q, want %q", m.promptArea.Value(), want)
	}
}

// TestCaretsEndOnTheKeysThatMeanOneCaret: esc ends the mode, and so does
// anything vertical — ↑/↓ mean "move the caret to another line", which is the
// one thing a caret per line has been asked not to do.
func TestCaretsEndOnTheKeysThatMeanOneCaret(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"esc", tea.KeyPressMsg{Code: tea.KeyEscape}},
		{"up", tea.KeyPressMsg{Code: tea.KeyUp}},
		{"down", tea.KeyPressMsg{Code: tea.KeyDown}},
		{"enter", enterKey(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := caretsOver(t, "one\ntwo")
			got := typeInForm(t, m, tc.key)
			if got.carets.on {
				t.Errorf("%q left the mode on", tc.name)
			}
			if got.stage != stageForm {
				t.Errorf("%q left the form", tc.name)
			}
		})
	}
	// Enter in particular must not also insert: it is the key most likely to be
	// pressed *because* the user thinks the mode is already over.
	m := caretsOver(t, "one\ntwo")
	if got := typeInForm(t, m, enterKey(0)); got.promptArea.Value() != "one\ntwo" {
		t.Errorf("enter inserted while ending the mode: %q", got.promptArea.Value())
	}
}

// TestCaretsHandBackKeysTheyDoNotOwn: a chord the mode has no meaning for ends
// it and then takes its ordinary path, so shift+enter still saves from inside
// the mode without being listed anywhere in it.
func TestCaretsHandBackKeysTheyDoNotOwn(t *testing.T) {
	m := caretsOver(t, "- one\n- two")
	got := typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	if got.carets.on {
		t.Error("shift+enter left the mode on")
	}
	if got.stage != stageList {
		t.Errorf("stage = %v, want the save to have gone through", got.stage)
	}
}

// TestCaretsEndOnAClick: the pointer is about to name one caret, which is a
// disagreement with having several.
func TestCaretsEndOnAClick(t *testing.T) {
	m := caretsOver(t, "one\ntwo")
	next, _ := m.Update(tea.MouseClickMsg{X: 4, Y: formPromptRow, Button: tea.MouseLeft})
	if next.(model).carets.on {
		t.Error("a click left the mode on")
	}
}

// TestCaretsPasteGoesToEveryLine: a paste is an insertion, and the mode is about
// where insertions land. Only its first line goes in — the rest would land
// somewhere no caret was asked to be.
func TestCaretsPasteGoesToEveryLine(t *testing.T) {
	m := caretsOver(t, "one\ntwo")
	next, _ := m.Update(tea.PasteMsg{Content: "» "})
	if want := "» one\n» two"; next.(model).promptArea.Value() != want {
		t.Errorf("value = %q, want %q", next.(model).promptArea.Value(), want)
	}

	m = caretsOver(t, "one\ntwo")
	next, _ = m.Update(tea.PasteMsg{Content: "a\nb"})
	if want := "aone\natwo"; next.(model).promptArea.Value() != want {
		t.Errorf("value = %q, want only the paste's first line on each: %q",
			next.(model).promptArea.Value(), want)
	}
}

// TestCaretsRefuseInWords covers the two ways the menu item cannot apply.
func TestCaretsRefuseInWords(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "one\ntwo")
	m.focusForm(formFieldPrompt)
	next, _ := m.dropPromptCarets()
	if got := next.(model); got.carets.on || !strings.Contains(got.formNote, "nothing selected") {
		t.Errorf("with no sweep: on=%v note=%q", got.carets.on, got.formNote)
	}

	m, _, _ = splitFormInTemp(t, "just the one line")
	m = selectWholePrompt(t, m)
	next, _ = m.dropPromptCarets()
	if got := next.(model); got.carets.on || !strings.Contains(got.formNote, "two or more lines") {
		t.Errorf("on one line: on=%v note=%q", got.carets.on, got.formNote)
	}
}

// TestCaretsAreDrawn: every caret but the first is painted by the editor's
// overlay — the first is the library's own, parked on one of the mode's rows so
// there is never a caret on screen that none of the keys move.
func TestCaretsAreDrawn(t *testing.T) {
	m := caretsOver(t, "abc\ndef\nghi")
	lines := strings.Split(m.promptEditorView(), "\n")
	if len(lines) < 3 {
		t.Fatalf("editor drew %d lines", len(lines))
	}
	reverse := ansiOf(promptCaretStyle)
	for i := range 3 {
		if !strings.Contains(lines[i], reverse) {
			t.Errorf("line %d carries no caret:\n%q", i, lines[i])
		}
	}
	// The frame keeps its width and line count: the overlay must not move
	// anything the form hit-tests.
	if got, want := len(lines), len(strings.Split(m.promptArea.View(), "\n")); got != want {
		t.Errorf("overlay drew %d lines, the editor %d", got, want)
	}
}

// TestCaretsFooterTeachesTheMode: while it is on, the footer is the mode's, not
// the editor's — the keys mean something different for as long as it lasts.
func TestCaretsFooterTeachesTheMode(t *testing.T) {
	m := caretsOver(t, "one\ntwo")
	foot := m.formFooter()
	for _, want := range []string{"every line", "esc ends"} {
		if !strings.Contains(foot, want) {
			t.Errorf("footer does not name %q:\n%s", want, foot)
		}
	}
	if strings.Contains(foot, "tab switch field") {
		t.Errorf("the ordinary editor footer is still up during the mode:\n%s", foot)
	}
}

// altClickAt presses alt+left on the editor's box: row counts from the top of
// the box, col is a text column of the line drawn there (the gutter is added on,
// the way a real pointer's X carries it).
func altClickAt(t *testing.T, m model, col, row int) model {
	t.Helper()
	next, _ := m.Update(tea.MouseClickMsg{
		X: promptGutterWidth(m.promptArea) + col, Y: formPromptRow + row,
		Button: tea.MouseLeft, Mod: tea.ModAlt,
	})
	return next.(model)
}

// TestAltClickAddsCarets is the pointer's road into the mode: the first
// alt+click puts a second caret beside the editor's own, each one after adds
// another, and typing lands at every caret's own column — the part the sweep
// cannot express, since its carets share the column it began in.
func TestAltClickAddsCarets(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "alpha\nbravo\ncharlie")
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 0) // the editor's caret: row 0, column 0

	m = altClickAt(t, m, 2, 2)
	if !m.carets.on {
		t.Fatal("alt+click did not enter the mode")
	}
	if got, want := m.carets.rows, []int{0, 2}; !slices.Equal(got, want) {
		t.Fatalf("carets on rows %v, want %v — the editor's own plus the clicked one", got, want)
	}
	if got, want := m.carets.cols, []int{0, 2}; !slices.Equal(got, want) {
		t.Fatalf("carets at columns %v, want %v", got, want)
	}

	m = altClickAt(t, m, 4, 1)
	if got, want := m.carets.rows, []int{0, 1, 2}; !slices.Equal(got, want) {
		t.Fatalf("carets on rows %v, want %v", got, want)
	}

	m = typeInForm(t, m, typeChar('!'))
	if want := "!alpha\nbrav!o\nch!arlie"; m.promptArea.Value() != want {
		t.Errorf("value = %q, want %q — each caret types in its own column", m.promptArea.Value(), want)
	}
}

// TestAltClickTogglesACaretAway: a press exactly on a standing caret removes it,
// and removing down to one ends the mode — one caret is what the editor is when
// the mode is off.
func TestAltClickTogglesACaretAway(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "alpha\nbravo\ncharlie")
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 0)

	m = altClickAt(t, m, 2, 2)
	m = altClickAt(t, m, 4, 1)
	m = altClickAt(t, m, 4, 1) // on the caret itself: take it away
	if got, want := m.carets.rows, []int{0, 2}; !slices.Equal(got, want) {
		t.Fatalf("carets on rows %v, want %v after the toggle", got, want)
	}

	m = altClickAt(t, m, 2, 2)
	if m.carets.on {
		t.Error("removing down to one caret left the mode on")
	}
	if !strings.Contains(m.formNote, "one caret") {
		t.Errorf("form note = %q, want it to say one caret remains", m.formNote)
	}
	// The survivor is where the library's caret is parked, so what remains on
	// screen is what remains.
	if got := promptCaretOffset(m.promptArea); got != 0 {
		t.Errorf("caret at offset %d, want 0 — the surviving caret's place", got)
	}
}

// TestAltClickAddsASecondCaretToARow: a row holds as many carets as were put on
// it, so a press elsewhere on a line that already has one ADDS rather than
// moves. This is the case a row-keyed caret set could not express, and the one a
// soft-wrapped prompt is made of.
func TestAltClickAddsASecondCaretToARow(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "alpha\nbravo\ncharlie")
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 0)

	m = altClickAt(t, m, 2, 2)
	m = altClickAt(t, m, 5, 2)
	if !m.carets.on {
		t.Fatal("a second caret on one row ended the mode")
	}
	if got, want := m.carets.rows, []int{0, 2, 2}; !slices.Equal(got, want) {
		t.Fatalf("carets on rows %v, want %v — row 2 carries two", got, want)
	}
	if got, want := m.carets.cols, []int{0, 2, 5}; !slices.Equal(got, want) {
		t.Fatalf("carets at columns %v, want %v — sorted by row then column", got, want)
	}

	// Both carets on row 2 type, and the right-hand one keeps its place over the
	// insert that lands to its left — the backwards walk is what makes that true.
	m = typeInForm(t, m, typeChar('!'))
	// "charlie" takes its marks after "ch" (column 2) and after "charl" (5).
	if want := "!alpha\nbravo\nch!arl!ie"; m.promptArea.Value() != want {
		t.Errorf("value = %q, want %q", m.promptArea.Value(), want)
	}
	if got, want := m.carets.cols, []int{1, 3, 7}; !slices.Equal(got, want) {
		t.Errorf("carets at columns %v, want %v — the later caret carried over the earlier insert", got, want)
	}
}

// TestAltClickTwoCaretsOnARowDelete pins the same backwards walk for the edit
// that shortens a row: two backspaces on one line take out two characters, and
// the right-hand caret lands where the text left it rather than one place off.
func TestAltClickTwoCaretsOnARowDelete(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "abcd\nzz")
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 0)

	m = altClickAt(t, m, 2, 0) // row 0, column 2 — after 'b'
	m = altClickAt(t, m, 4, 0) // row 0, column 4 — after 'd'
	if got, want := m.carets.rows, []int{0, 0, 0}; !slices.Equal(got, want) {
		t.Fatalf("carets on rows %v, want %v", got, want)
	}

	// The editor's own caret is at column 0 and has nothing behind it; the other
	// two bite, taking out 'b' and 'd'.
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if want := "ac\nzz"; m.promptArea.Value() != want {
		t.Errorf("value = %q, want %q", m.promptArea.Value(), want)
	}
	if got, want := m.carets.cols, []int{0, 1, 2}; !slices.Equal(got, want) {
		t.Errorf("carets at columns %v, want %v", got, want)
	}
}

// TestCaretsOnARowFoldWhenTheyMeet: ctrl+a sends every goal on a row to column
// 0, which is two carets asking to be in one place. Two carets in one place are
// one caret, so the set folds — otherwise the next character would be typed
// twice on that line.
func TestCaretsOnARowFoldWhenTheyMeet(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "alpha\nbravo")
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 0)

	m = altClickAt(t, m, 1, 1)
	m = altClickAt(t, m, 4, 1) // row 1 now carries two carets
	if len(m.carets.rows) != 3 {
		t.Fatalf("%d carets, want 3 before the fold", len(m.carets.rows))
	}

	m = typeInForm(t, m, tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if got, want := m.carets.rows, []int{0, 1}; !slices.Equal(got, want) {
		t.Fatalf("carets on rows %v, want %v — row 1's two folded into one", got, want)
	}
	m = typeInForm(t, m, typeChar('-'))
	if want := "-alpha\n-bravo"; m.promptArea.Value() != want {
		t.Errorf("value = %q, want %q — one dash per line, not two on the folded row", m.promptArea.Value(), want)
	}
}

// TestAltClickRefusesInWords covers the two presses that cannot mean a caret:
// below the last line there is no line to put one on, and on the cell the
// editor's caret already occupies there is no second caret to add. Both say so
// — silence here is indistinguishable from the alt bit never having reached the
// program, which is the other way alt+click "does nothing".
func TestAltClickRefusesInWords(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "one\ntwo")
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 0)
	m = altClickAt(t, m, 0, 5)
	if m.carets.on {
		t.Error("a press below the value entered the mode")
	}
	if !strings.Contains(m.formNote, "no line") {
		t.Errorf("form note = %q, want it to say there is no line there", m.formNote)
	}

	m = altClickAt(t, m, 0, 0) // the caret's own cell: nothing to add
	if m.carets.on {
		t.Error("a press on the caret's own cell entered the mode")
	}
	if !strings.Contains(m.formNote, "already there") {
		t.Errorf("form note = %q, want it to say the caret is already there", m.formNote)
	}

	// One cell over on that same line is a different cell, so it is a caret.
	m = altClickAt(t, m, 2, 0)
	if !m.carets.on {
		t.Error("a press elsewhere on the caret's own line did not add a caret")
	}
}

// TestAltClickOnAWrappedLineAddsACaret is the shape the gesture was reported
// broken in, three times: a long line wraps across several rows of the box, so
// two presses that look like two lines are two cells of ONE logical line. A
// row-keyed caret set could only refuse that, which made the pointer useless on
// the commonest prompt there is — one long paragraph. Keying on the cell is what
// fixes it, and this is the test that says so.
func TestAltClickOnAWrappedLineAddsACaret(t *testing.T) {
	long := strings.Repeat("wrap ", 60) // far wider than the 120-cell test pane
	m, _, _ := splitFormInTemp(t, long)
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 0)

	lines := promptDisplayLines(m.promptArea)
	if len(lines) < 2 || lines[0].row != 0 || lines[1].row != 0 {
		t.Fatalf("fixture did not wrap onto a second row of the same line: %d display lines", len(lines))
	}

	m = altClickAt(t, m, 3, 1) // the second *row* of the first *line*
	if !m.carets.on {
		t.Fatal("a press on another row of the same line did not enter the mode")
	}
	if got, want := m.carets.rows, []int{0, 0}; !slices.Equal(got, want) {
		t.Fatalf("carets on rows %v, want %v — both on the one wrapped line", got, want)
	}
	// The second caret is on the second display row, so its column is past
	// where that row begins — not column 3 of the logical line.
	if got := m.carets.cols[1]; got <= lines[1].start {
		t.Errorf("second caret at column %d, want past the wrap seam at %d", got, lines[1].start)
	}
}

// TestMergeCaretPaints: a caret inside a spell mark takes its cell out of the
// run rather than overlapping it. paintPromptSpans requires sorted,
// non-overlapping runs — two claiming one cell would emit that cell twice and
// push the rest of the line right.
func TestMergeCaretPaints(t *testing.T) {
	marks := []promptPaint{{a: 2, b: 8, style: promptSelStyle, caret: true}}
	carets := []promptPaint{{a: 4, b: 5, style: promptCaretStyle}}
	got := mergeCaretPaints(marks, carets)

	want := [][2]int{{2, 4}, {4, 5}, {5, 8}}
	if len(got) != len(want) {
		t.Fatalf("merged into %d runs %v, want %d", len(got), got, len(want))
	}
	for i, w := range want {
		if got[i].a != w[0] || got[i].b != w[1] {
			t.Errorf("run %d = [%d, %d), want [%d, %d)", i, got[i].a, got[i].b, w[0], w[1])
		}
	}
	for i := 1; i < len(got); i++ {
		if got[i].a < got[i-1].b {
			t.Errorf("runs %d and %d overlap: %v", i-1, i, got)
		}
	}
}
