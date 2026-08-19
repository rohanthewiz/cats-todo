package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// openSpell puts the form on the Spelling panel the way ctrl+l does, and fails
// if it did not get there — every test below starts from the panel, and a
// silent stay on the form would make each of them assert about the wrong screen.
func openSpell(t *testing.T, m model) model {
	t.Helper()
	m = typeInForm(t, m, ctrlKey('l'))
	if m.stage != stageSpell {
		t.Fatalf("ctrl+l left the form on stage %v, want the Spelling panel", m.stage)
	}
	return m
}

// spellRowNames is what the panel is offering, in the order it draws them.
func spellRowNames(m model) []string {
	var names []string
	for _, s := range m.spellList.filtered {
		if s.item.selectable {
			names = append(names, s.item.name)
		}
	}
	return names
}

// withProjectSpellForm is withSpellForm with a real project directory on disk,
// so the panel can offer — and write — a project dictionary.
func withProjectSpellForm(t *testing.T, prompt string) model {
	t.Helper()
	t.Setenv(configDirEnvVar, filepath.Join(t.TempDir(), "config"))
	m := newTestModel()
	m.ctx.WorkDir = filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(m.ctx.WorkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 100, 40
	m.spellOn = true
	next, _ := m.beginAdd()
	m = next.(model)
	m.promptArea.SetValue(prompt)
	m.promptArea.MoveToEnd()
	if m.spellDict == nil {
		t.Fatal("the dictionary did not load when the form opened")
	}
	return m
}

// TestSpellPanelOpensOnTheWordAtTheCaret: ctrl+l opens the panel on the word
// being typed — the one word the underline deliberately spares, which makes the
// panel the only way to ask about it.
func TestSpellPanelOpensOnTheWordAtTheCaret(t *testing.T) {
	m := withSpellForm(t, "fix teh")
	m.promptArea.MoveToEnd()
	if spans := m.promptSpellSpans(); len(spans) != 0 {
		t.Fatalf("the word under the caret is marked (%v); this test is about the one that isn't", spans)
	}
	m = openSpell(t, m)
	if m.spellWord != "teh" {
		t.Errorf("the panel opened on %q, want %q", m.spellWord, "teh")
	}
	if got := spellRowNames(m); len(got) == 0 || got[0] != "the" {
		t.Errorf("panel rows are %v, want %q first", got, "the")
	}
	if !strings.Contains(ansi.Strip(m.viewSpell()), "teh") {
		t.Errorf("the panel does not name the word it is about:\n%s", ansi.Strip(m.viewSpell()))
	}
}

// TestSpellPanelTargetsTheNearestWord: with the caret clear of every flagged
// word, the panel takes the last one behind it — prose is written left to
// right, so the mistake you have noticed is one you have already passed — and
// only looks forward when there is nothing behind.
func TestSpellPanelTargetsTheNearestWord(t *testing.T) {
	m := withSpellForm(t, "wrods and more wrods here")
	value := []rune(m.promptArea.Value())
	spans := m.spellDict.Check(string(value))
	if len(spans) != 2 {
		t.Fatalf("expected two flagged words, got %v", spans)
	}

	// Between the two: the one behind wins.
	m.promptArea.SetCursorColumn(spans[1].Start - 1)
	sp, word, ok := m.promptSpellTarget()
	if !ok || sp != spans[0] || word != "wrods" {
		t.Errorf("caret between the words targeted %+v (%q), want the first %+v", sp, word, spans[0])
	}

	// Before both: the only one it can look at is ahead.
	m.promptArea.MoveToBegin()
	sp, _, ok = m.promptSpellTarget()
	if !ok || sp != spans[0] {
		t.Errorf("caret at the top targeted %+v, want the first %+v", sp, spans[0])
	}

	// Past both: the last one.
	m.promptArea.MoveToEnd()
	sp, _, ok = m.promptSpellTarget()
	if !ok || sp != spans[1] {
		t.Errorf("caret at the end targeted %+v, want the last %+v", sp, spans[1])
	}
}

// TestSpellPanelFixesTheWord: choosing a suggestion replaces exactly the
// flagged word, leaves the caret where the word ends, says what it did, and
// hands the keys back to the editor.
func TestSpellPanelFixesTheWord(t *testing.T) {
	m := withSpellForm(t, "first line\nplease chekc the wrods now")
	// Caret on the second row, past both misspellings, so the target is the one
	// behind it — and so the replacement has to survive a line break above it.
	m.promptArea.MoveToEnd()
	m = openSpell(t, m)
	if m.spellWord != "wrods" {
		t.Fatalf("the panel opened on %q, want %q", m.spellWord, "wrods")
	}
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.stage != stageForm {
		t.Errorf("after a correction the stage is %v, want the form", m.stage)
	}
	if got, want := m.promptArea.Value(), "first line\nplease chekc the words now"; got != want {
		t.Errorf("prompt is %q, want %q", got, want)
	}
	if !strings.Contains(m.formNote, "wrods") || !strings.Contains(m.formNote, "words") {
		t.Errorf("form note is %q, want it to name both spellings", m.formNote)
	}
	if !m.promptArea.Focused() {
		t.Error("the editor did not get the keys back")
	}
	// The caret sits just past what was inserted, so typing carries on from
	// there rather than from wherever the value happened to end.
	runes := []rune(m.promptArea.Value())
	at := promptCaretOffset(m.promptArea)
	if at > len(runes) || string(runes[:at]) != "first line\nplease chekc the words" {
		t.Errorf("caret is at rune %d (%q), want it just past the corrected word",
			at, string(runes[:min(at, len(runes))]))
	}
	// And the corrected word is no longer marked, while the untouched one is.
	var marked []string
	for _, sp := range m.spellDict.Check(m.promptArea.Value()) {
		marked = append(marked, string(runes[sp.Start:sp.End]))
	}
	if len(marked) != 1 || marked[0] != "chekc" {
		t.Errorf("still flagged: %v, want just the word that was left alone", marked)
	}
}

// TestSpellPanelAddsToADictionary: the add rows write the word to the file they
// name and teach the running dictionary the same word, so the underline goes
// now rather than at the next launch.
func TestSpellPanelAddsToADictionary(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  string
		path func(model) string
	}{
		{"global", "my dictionary", model.spellDictGlobalPath},
		{"project", "this project's dictionary", model.spellDictProjectPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := withProjectSpellForm(t, "the zorbulate thing")
			m = openSpell(t, m)
			if m.spellWord != "zorbulate" {
				t.Fatalf("the panel opened on %q", m.spellWord)
			}
			// Find the row by what it says, the way a user does.
			idx := -1
			for i, c := range m.spellChoices {
				if c.kind == spellAdd && c.where == tc.row {
					idx = i
				}
			}
			if idx < 0 {
				t.Fatalf("no %q row on the panel: %v", tc.row, spellRowNames(m))
			}
			m.spellList.selectRef(idx)
			m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

			if m.stage != stageForm {
				t.Errorf("after an add the stage is %v, want the form", m.stage)
			}
			if !strings.Contains(m.formNote, "zorbulate") || !strings.Contains(m.formNote, tc.row) {
				t.Errorf("form note is %q, want it to name the word and the list", m.formNote)
			}
			data, err := os.ReadFile(tc.path(m))
			if err != nil {
				t.Fatalf("reading the dictionary that was written: %v", err)
			}
			if !strings.Contains(string(data), "zorbulate") {
				t.Errorf("the word is not in the file:\n%s", data)
			}
			// The mark is gone without a reload — the point of teaching the
			// loaded dictionary as well as the file.
			if spans := m.promptSpellSpans(); len(spans) != 0 {
				t.Errorf("the added word is still flagged: %v", spans)
			}
			// And a fresh launch reading that file agrees.
			if m2 := withProjectSpellForm(t, "x"); m2.spellDict.Known("zorbulate") {
				t.Error("a model built against a different temp directory somehow knows the word")
			}
		})
	}
}

// TestSpellPanelReportsAWriteItCouldNotMake: a dictionary that cannot be
// written leaves the panel up with the reason on it, and — the point of writing
// the file first — does not teach the running dictionary a word the next launch
// would forget.
func TestSpellPanelReportsAWriteItCouldNotMake(t *testing.T) {
	m := withProjectSpellForm(t, "the zorbulate thing")
	// A directory where the file should be: openable, unwritable, and not
	// something the panel is allowed to paper over.
	if err := os.MkdirAll(m.spellDictProjectPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	m = openSpell(t, m)
	idx := -1
	for i, c := range m.spellChoices {
		if c.kind == spellAdd && c.where == "this project's dictionary" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatal("no project dictionary row on the panel")
	}
	m.spellList.selectRef(idx)
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.stage != stageSpell {
		t.Errorf("a failed add left the panel (stage %v); the error would never be seen", m.stage)
	}
	if !strings.Contains(m.spellErr, "zorbulate") {
		t.Errorf("panel error is %q, want it to name the word", m.spellErr)
	}
	if !strings.Contains(ansi.Strip(m.viewSpell()), m.spellErr) {
		t.Error("the panel does not draw its own error")
	}
	if m.spellDict.Known("zorbulate") {
		t.Error("the word was accepted in memory despite the write failing")
	}
}

// TestSpellPanelTogglesInPlace: the last row flips the check and the panel
// stays up, rebuilt — which is what lets someone turn the check on and be shown
// the suggestions the panel could not offer a keystroke earlier.
func TestSpellPanelTogglesInPlace(t *testing.T) {
	m := withSpellForm(t, "fix teh")
	m = openSpell(t, m)
	rows := len(spellRowNames(m))
	if rows < 2 {
		t.Fatalf("the panel opened with %d rows: %v", rows, spellRowNames(m))
	}

	m.spellList.selectRef(len(m.spellChoices) - 1) // the toggle is always last
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.stage != stageSpell {
		t.Fatalf("the toggle row left the panel (stage %v)", m.stage)
	}
	if m.spellOn {
		t.Error("the toggle row did not turn the check off")
	}
	if got := spellRowNames(m); len(got) != 1 || !strings.Contains(got[0], "on") {
		t.Errorf("with the check off the panel offers %v, want only the row that turns it back on", got)
	}
	if note := ansi.Strip(m.viewSpell()); !strings.Contains(note, "the check is off") {
		t.Errorf("the panel does not say why it is empty:\n%s", note)
	}
	if loadSettings().spellcheck {
		t.Error("the toggle was not persisted")
	}

	// Back on, and the suggestions are there again without leaving the panel.
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.spellOn {
		t.Fatal("the toggle row did not turn the check back on")
	}
	if got := len(spellRowNames(m)); got != rows {
		t.Errorf("the panel came back with %d rows, want the %d it opened with", got, rows)
	}
	// The note is waiting on the form's line for when the panel closes.
	closed, _ := m.closeSpell()
	if m = closed.(model); m.formNote != "spell check on" {
		t.Errorf("form note after closing is %q, want the toggle's own note", m.formNote)
	}
}

// TestSpellPanelWithNothingToCorrect: opened over a clean prompt, the panel says
// so and offers only the toggle — the three empty states are told apart in
// words, since a list with one row in it looks the same whatever the reason.
func TestSpellPanelWithNothingToCorrect(t *testing.T) {
	m := withSpellForm(t, "every word here is spelled correctly")
	m = openSpell(t, m)
	if m.spellWord != "" {
		t.Errorf("the panel found %q to correct in a clean prompt", m.spellWord)
	}
	if got := spellRowNames(m); len(got) != 1 {
		t.Errorf("panel rows are %v, want only the toggle", got)
	}
	if view := ansi.Strip(m.viewSpell()); !strings.Contains(view, "nothing misspelled") {
		t.Errorf("the panel does not say why it is empty:\n%s", view)
	}
	// A word with no correction to offer still gets its add rows: jargon is the
	// commonest reason a word is flagged and the commonest thing to accept.
	m2 := withProjectSpellForm(t, "the zorbulate thing")
	m2 = openSpell(t, m2)
	adds := 0
	for _, c := range m2.spellChoices {
		if c.kind == spellAdd {
			adds++
		}
	}
	if adds != 2 {
		t.Errorf("a flagged word offered %d add rows, want both dictionaries", adds)
	}
}

// TestSpellPanelClosesBothWays: esc and ctrl+l both back out — the key that
// opened the panel is one a hand expects to be able to press again — and both
// hand the keys back to the field that had them.
func TestSpellPanelClosesBothWays(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEscape}, ctrlKey('l')} {
		m := withSpellForm(t, "fix teh")
		before := m.promptArea.Value()
		m = openSpell(t, m)
		m = typeInForm(t, m, key)
		if m.stage != stageForm {
			t.Errorf("%v left the panel open (stage %v)", key, m.stage)
		}
		if m.promptArea.Value() != before {
			t.Errorf("%v changed the prompt to %q", key, m.promptArea.Value())
		}
		if !m.promptArea.Focused() {
			t.Errorf("%v did not give the editor its keys back", key)
		}
	}

	// From the title field it comes back to the title field.
	m := withSpellForm(t, "fix teh")
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.formFocus != formFieldTitle {
		t.Fatalf("tab did not reach the title field (focus %d)", m.formFocus)
	}
	m = openSpell(t, m)
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.titleInput.Focused() {
		t.Error("the panel gave the keys back to the wrong field")
	}
}

// TestSpellRowsMatchWhatIsDrawn pins spellRowsRow to a real frame: clickSpell
// subtracts it from the pointer's row to find the row that was pressed, so a
// line added above the list would aim every click one row off.
func TestSpellRowsMatchWhatIsDrawn(t *testing.T) {
	m := withSpellForm(t, "fix teh")
	m = openSpell(t, m)
	lines := strings.Split(ansi.Strip(m.viewSpell()), "\n")
	if len(lines) <= spellRowsRow {
		t.Fatalf("the panel draws only %d lines", len(lines))
	}
	first := spellRowNames(m)[0]
	if got := lines[spellRowsRow]; !strings.Contains(got, first) {
		t.Errorf("line %d of the panel is %q, want the first row %q", spellRowsRow, got, first)
	}

	// And a click there presses it, which is the whole reason the constant has
	// to be right.
	next, _ := m.Update(tea.MouseClickMsg{X: 4, Y: spellRowsRow, Button: tea.MouseLeft})
	m = next.(model)
	if m.stage != stageForm {
		t.Fatalf("the click did not press a row (stage %v)", m.stage)
	}
	if got, want := m.promptArea.Value(), "fix "+first; got != want {
		t.Errorf("prompt is %q, want %q — the click pressed the wrong row", got, want)
	}
}

// TestSpellPanelFiltersItsRows: the query box narrows the list the way every
// other picker's does, and enter presses whatever is left highlighted. It
// matters most where the misspelling has a long tail of near-misses — typing
// two letters of the word you meant is quicker than walking down to it.
func TestSpellPanelFiltersItsRows(t *testing.T) {
	m := withSpellForm(t, "fix wrods")
	m = openSpell(t, m)
	before := spellRowNames(m)
	if len(before) < 4 {
		t.Fatalf("not enough rows to filter: %v", before)
	}
	for _, r := range "word" {
		m = typeInForm(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	after := spellRowNames(m)
	switch {
	case len(after) == 0:
		t.Fatal("the filter matched nothing")
	case len(after) >= len(before):
		t.Errorf("the filter left %d of %d rows: %v", len(after), len(before), after)
	}
	// The suggestion being typed towards is still there, and the near-misses
	// that share none of its letters are not. (The matching is the same fuzzy
	// subsequence every picker uses, so a row that merely contains the letters
	// scattered through it — an add row naming the misspelling — survives too.)
	if !slices.Contains(after, "words") {
		t.Errorf("filtering towards %q dropped it: %v", "words", after)
	}
	if slices.Contains(after, "prods") {
		t.Errorf("a row sharing none of the query's letters survived: %v", after)
	}

	// Enter still presses the highlighted row, which the filter has moved.
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got, want := m.promptArea.Value(), "fix "+after[0]; got != want {
		t.Errorf("prompt is %q, want %q", got, want)
	}
}

// TestSpellPanelOnAMultibyteLine: the span the panel replaces is in runes, and
// the value it rebuilds has to be too — a byte offset would cut a multi-byte
// character in half one column to the left of the word.
func TestSpellPanelOnAMultibyteLine(t *testing.T) {
	m := withSpellForm(t, "héllo wörld — fix teh now")
	m.promptArea.MoveToEnd()
	m = openSpell(t, m)
	if m.spellWord != "teh" {
		t.Fatalf("the panel opened on %q, want %q", m.spellWord, "teh")
	}
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got, want := m.promptArea.Value(), "héllo wörld — fix the now"; got != want {
		t.Errorf("prompt is %q, want %q", got, want)
	}
}
