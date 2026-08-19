package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// withSpellForm is withForm against a private config directory, so the spell
// preference these tests flip never touches the real settings file, and with
// the check forced on regardless of what the machine's settings say.
func withSpellForm(t *testing.T, prompt string) model {
	t.Helper()
	t.Setenv(configDirEnvVar, t.TempDir())
	m := withForm(t, "", prompt, 100, 40)
	if !m.spellOn {
		m.spellOn = true
		m.loadSpellDict()
	}
	if m.spellDict == nil {
		t.Fatal("the dictionary did not load when the form opened")
	}
	return m
}

// spellMark is word as a flagged word on the given line tone is drawn — the
// exact bytes paintPromptSpans emits for it — so a test can look for it in the
// view. (Not a prefix plus the word: lipgloss renders underlined text one rune
// per escape sequence.)
func spellMark(m model, cursorLine bool, word string) string {
	st := m.promptArea.Styles().Focused
	if cursorLine {
		return promptSpellStyle(st.CursorLine).Render(word)
	}
	return promptSpellStyle(st.Text).Render(word)
}

func ctrlKey(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code, Mod: tea.ModCtrl} }

func TestSettingsRoundTrip(t *testing.T) {
	t.Setenv(configDirEnvVar, t.TempDir())
	if s := loadSettings(); !s.spellcheck {
		t.Fatal("a missing settings file should mean spell check on")
	}
	s := settings{spellcheck: false}
	if err := s.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := loadSettings(); got.spellcheck {
		t.Error("spellcheck read back as on after saving it off")
	}
	data, err := os.ReadFile(settingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"spellcheck": false`) {
		t.Errorf("settings file is %q, want a spellcheck key", data)
	}
	// A file that says nothing about it means the default, not false.
	if err := os.WriteFile(settingsPath(), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings(); !got.spellcheck {
		t.Error("an empty settings object should leave spell check on")
	}
	// And a corrupt one falls back to the defaults rather than failing.
	if err := os.WriteFile(settingsPath(), []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings(); !got.spellcheck {
		t.Error("a corrupt settings file should fall back to the defaults")
	}
}

// clickSpellChip presses the toolbar's ☑ Spell button, which is the toggle's
// whole affordance now that ctrl+l opens the panel instead (see spellChipLabel).
func clickSpellChip(t *testing.T, m model) model {
	t.Helper()
	c := m.formChips()[formActionSpell]
	return clickForm(m, c.start, m.formBarRow())
}

// TestSpellToggle: the ☑ Spell chip flips the check, says so on the note line,
// changes its own glyph to match, and the choice survives into the next model
// built against the same config directory.
func TestSpellToggle(t *testing.T) {
	m := withSpellForm(t, "teh")
	if got := m.spellChipLabel(); got != "☑ Spell" {
		t.Errorf("chip reads %q with the check on, want the ticked box", got)
	}

	m = clickSpellChip(t, m)
	if m.spellOn {
		t.Fatal("the Spell chip left spell check on")
	}
	if m.formNote != "spell check off" {
		t.Errorf("form note is %q, want %q", m.formNote, "spell check off")
	}
	if got := m.spellChipLabel(); got != "☐ Spell" {
		t.Errorf("chip reads %q with the check off, want the empty box", got)
	}
	if m.spellChipTint() == (model{spellOn: true}).spellChipTint() {
		t.Error("the chip is the same hue off as on — the state is only in the glyph")
	}
	if loadSettings().spellcheck {
		t.Error("the toggle was not persisted")
	}
	// The next launch reads it back.
	if fresh := newTestModel(); fresh.spellOn {
		t.Error("a new model came up with spell check on after it was turned off")
	}

	m = clickSpellChip(t, m)
	if !m.spellOn || m.formNote != "spell check on" {
		t.Errorf("second press: on=%v note=%q, want on with the note saying so", m.spellOn, m.formNote)
	}
	if !loadSettings().spellcheck {
		t.Error("turning it back on was not persisted")
	}
}

// TestSpellOffLeavesTheEditorAlone: with the check off, the editor's view is
// the library's own output, byte for byte.
func TestSpellOffLeavesTheEditorAlone(t *testing.T) {
	m := withSpellForm(t, "teh quick brown fox")
	m.spellOn = false
	if got, want := m.promptEditorView(), m.promptArea.View(); got != want {
		t.Error("with spell check off the editor view differs from the textarea's own")
	}
	if spans := m.promptSpellSpans(); spans != nil {
		t.Errorf("promptSpellSpans = %+v with the check off, want nil", spans)
	}
}

// TestSpellMarksAreDrawn covers the paint: a misspelling gets the mark, the
// words around it do not, and the overlay changes neither the text nor the
// width of any line — the same invariants the selection overlay is held to,
// since a line that grew or shrank would move the toolbar's row.
func TestSpellMarksAreDrawn(t *testing.T) {
	m := withSpellForm(t, "teh quick brown fox\nsecond lnie here")
	m.promptArea.MoveToBegin() // caret on row 0, before "teh": still flagged
	view := m.promptEditorView()
	raw := m.promptArea.View()

	// The caret sits at cell 0 of "teh", so the word is drawn as caret + "eh"
	// under the mark; the second line's "lnie" is whole.
	if !strings.Contains(view, spellMark(m, false, "lnie")) {
		t.Errorf("\"lnie\" on the second line is not marked:\n%s", view)
	}
	if !strings.Contains(view, spellMark(m, true, "eh")) {
		t.Errorf("\"teh\" on the caret line is not marked around the caret:\n%s", view)
	}
	for _, ok := range []string{"quick", "brown", "fox", "second", "here"} {
		if strings.Contains(view, spellMark(m, true, ok)) || strings.Contains(view, spellMark(m, false, ok)) {
			t.Errorf("%q is marked as misspelled", ok)
		}
	}

	rawLines, lines := strings.Split(raw, "\n"), strings.Split(view, "\n")
	if len(rawLines) != len(lines) {
		t.Fatalf("the overlay changed the editor from %d lines to %d", len(rawLines), len(lines))
	}
	for i := range lines {
		if got, want := ansi.Strip(lines[i]), ansi.Strip(rawLines[i]); got != want {
			t.Errorf("line %d reads %q under the marks, %q without", i, got, want)
		}
		if got, want := lipgloss.Width(lines[i]), lipgloss.Width(rawLines[i]); got != want {
			t.Errorf("line %d is %d cells under the marks, %d without", i, got, want)
		}
	}
	// The caret is still drawn, inside the marked word.
	if !strings.Contains(lines[0], ansiOf(promptCaretStyle)+"t") {
		t.Errorf("the caret on \"teh\" was painted over:\n%q", lines[0])
	}
}

// TestSpellSparesTheWordUnderTheCaret: a word being typed is not marked until
// the caret leaves it.
func TestSpellSparesTheWordUnderTheCaret(t *testing.T) {
	m := withSpellForm(t, "fix teh")
	m.promptArea.MoveToEnd() // caret just past "teh": that is typing it
	if spans := m.promptSpellSpans(); len(spans) != 0 {
		t.Errorf("the word under the caret is flagged: %+v", spans)
	}
	if got, want := m.promptEditorView(), m.promptArea.View(); got != want {
		t.Error("with nothing to mark the view should be the textarea's own")
	}
	// A space after it, and it is a finished word.
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if spans := m.promptSpellSpans(); len(spans) != 1 {
		t.Errorf("after moving on, spans = %+v, want the one word", spans)
	}
	// Caret moved back to the start of it: before the word, not in it.
	m.promptArea.SetCursorColumn(4)
	if spans := m.promptSpellSpans(); len(spans) != 1 {
		t.Errorf("caret before the word: spans = %+v, want it still flagged", spans)
	}
	// And one step in, it is being edited again.
	m.promptArea.SetCursorColumn(5)
	if spans := m.promptSpellSpans(); len(spans) != 0 {
		t.Errorf("caret inside the word: spans = %+v, want none", spans)
	}
}

// TestSpellMarksAndSelectionShareALine: both overlays on one line, with the
// selection winning where they overlap, and the line's text and width intact.
func TestSpellMarksAndSelectionShareALine(t *testing.T) {
	m := withSpellForm(t, "teh quick brwon fox")
	raw := m.promptArea.View()
	m.promptArea.MoveToBegin()
	// Select "teh q": the whole first misspelling and a bit more.
	for range 5 {
		m = typeInForm(t, m, shiftKey(tea.KeyRight))
	}
	if got := m.selectedPromptText(); got != "teh q" {
		t.Fatalf("selected %q, want %q", got, "teh q")
	}
	view := m.promptEditorView()
	if !strings.Contains(view, ansiOf(promptSelStyle)) {
		t.Error("the selection is not drawn")
	}
	if !strings.Contains(view, spellMark(m, true, "brwon")) {
		t.Errorf("\"brwon\" is not marked beside a selection:\n%s", view)
	}
	if strings.Contains(view, spellMark(m, true, "teh")) {
		t.Error("the selected \"teh\" is marked under the selection")
	}
	rawLines, lines := strings.Split(raw, "\n"), strings.Split(view, "\n")
	for i := range lines {
		if got, want := ansi.Strip(lines[i]), ansi.Strip(rawLines[i]); got != want {
			t.Errorf("line %d reads %q, want %q", i, got, want)
		}
		if got, want := lipgloss.Width(lines[i]), lipgloss.Width(rawLines[i]); got != want {
			t.Errorf("line %d is %d cells, want %d", i, got, want)
		}
	}
	// A selection sweeping out of a marked word leaves the part it did not
	// cover marked. The anchor is inside the word and the caret leaves it (a
	// caret still inside would spare the whole word — see promptSpellSpans).
	m2 := withSpellForm(t, "brwon fox")
	m2.promptArea.MoveToBegin()
	m2.promptArea.SetCursorColumn(2)
	for range 5 {
		m2 = typeInForm(t, m2, shiftKey(tea.KeyRight))
	}
	if got := m2.selectedPromptText(); got != "won f" {
		t.Fatalf("selected %q, want %q", got, "won f")
	}
	v := m2.promptEditorView()
	if !strings.Contains(v, spellMark(m2, true, "br")) {
		t.Errorf("the unselected head of a marked word lost its mark:\n%s", v)
	}
	if strings.Contains(v, spellMark(m2, true, "won")) || strings.Contains(v, spellMark(m2, true, "brwon")) {
		t.Errorf("the selected part of the word is still marked:\n%s", v)
	}
}

// TestSpellMarksFollowSoftWraps: a flagged word past a soft wrap is marked on
// the display line it is drawn on, and one that straddles the wrap is marked on
// both — the display-line table, not the logical line, decides.
func TestSpellMarksFollowSoftWraps(t *testing.T) {
	t.Setenv(configDirEnvVar, t.TempDir())
	m := withForm(t, "", strings.TrimSpace(strings.Repeat("wrapme ", 6)), 30, 40)
	m.spellOn = true
	m.loadSpellDict()
	m.promptArea.MoveToBegin()
	lines := strings.Split(m.promptEditorView(), "\n")
	marked := 0
	for _, ln := range lines {
		if strings.Contains(ln, spellMark(m, false, "wrapme")) || strings.Contains(ln, spellMark(m, true, "wrapme")) {
			marked++
		}
	}
	if marked < 2 {
		t.Errorf("only %d wrapped lines carry a mark, want the wrapped ones too:\n%s", marked, m.promptEditorView())
	}
}

// TestSpellUserDictionary: a word in the project's dictionary.txt stops being
// flagged, which is the whole point of the file.
func TestSpellUserDictionary(t *testing.T) {
	t.Setenv(configDirEnvVar, t.TempDir())
	m := newTestModel()
	dir := filepath.Join(t.TempDir(), "project")
	m.ctx.WorkDir = dir
	if err := os.MkdirAll(filepath.Join(dir, projectConfigDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, projectConfigDirName, spellDictFileName), []byte("zorbulate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 100, 40
	m.spellOn = true
	next, _ := m.beginAdd()
	m = next.(model)
	m.promptArea.SetValue("zorbulate the wrods")
	m.promptArea.MoveToBegin()
	rs := []rune(m.promptArea.Value())
	spans := m.promptSpellSpans()
	if len(spans) != 1 || string(rs[spans[0].Start:spans[0].End]) != "wrods" {
		t.Errorf("spans = %+v, want just \"wrods\" — \"zorbulate\" is in the project dictionary", spans)
	}
}

// TestSpellFooterNamesTheChord: the form footer advertises ctrl+l — in a pane
// wide enough for the whole caret line, and in one narrow enough that the
// chords line is drawn but not so narrow that its tail is cut. No chip stands
// for the panel (☑ Spell is the toggle), so the footer is its only teacher.
func TestSpellFooterNamesTheChord(t *testing.T) {
	m := withSpellForm(t, "")
	for _, w := range []int{95, 160} {
		m.width = w
		if foot := m.formFooter(); !strings.Contains(foot, "ctrl+l spelling") {
			t.Errorf("footer at width %d does not name the spelling panel:\n%s", w, foot)
		}
	}
}
