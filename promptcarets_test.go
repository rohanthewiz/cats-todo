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
// it and then takes its ordinary path, so ctrl+s still saves from inside the
// mode without being listed anywhere in it.
func TestCaretsHandBackKeysTheyDoNotOwn(t *testing.T) {
	m := caretsOver(t, "- one\n- two")
	got := typeInForm(t, m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if got.carets.on {
		t.Error("ctrl+s left the mode on")
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

// TestAltClickMovesTheLinesCaret: a line holds at most one caret, so a press
// elsewhere on a line that already has one moves it — the pointer said where.
func TestAltClickMovesTheLinesCaret(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "alpha\nbravo\ncharlie")
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 0)

	m = altClickAt(t, m, 2, 2)
	m = altClickAt(t, m, 5, 2)
	if !m.carets.on {
		t.Fatal("moving a caret ended the mode")
	}
	if got, want := m.carets.cols, []int{0, 5}; !slices.Equal(got, want) {
		t.Errorf("carets at columns %v, want %v — the clicked line's caret moved", got, want)
	}
}

// TestAltClickRefusesInWords covers the presses that cannot mean a caret: below
// the last line there is no line to put one on, and on the caret's own line with
// the mode off there is nothing multiple about the gesture — it is a plain move.
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

	m = altClickAt(t, m, 2, 0) // the caret's own line: a plain caret move
	if m.carets.on {
		t.Error("a press on the caret's own line entered the mode")
	}
	if got := promptCaretOffset(m.promptArea); got != 2 {
		t.Errorf("caret at offset %d, want 2 — alt or no alt, the pointer places it", got)
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
