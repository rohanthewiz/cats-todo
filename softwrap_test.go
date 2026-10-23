package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// altZ is the ☑ Soft wrap chord on both screens.
var altZ = tea.KeyPressMsg{Code: 'z', Mod: tea.ModAlt}

// unwrappedForm is an add form over a temp config directory (the toggle writes
// settings.json) holding prompt, with soft wrap turned off by the chord.
func unwrappedForm(t *testing.T, prompt string, width int) model {
	t.Helper()
	t.Setenv(configDirEnvVar, t.TempDir())
	m := withForm(t, "", prompt, width, 40)
	m.softWrap = true
	sizePromptBox(&m.promptArea, m.promptBoxWidth(), true)
	next, _ := m.Update(altZ)
	m = next.(model)
	if m.softWrap {
		t.Fatal("alt+z left soft wrap on")
	}
	return m
}

// promptRows is the editor's drawn rows, stripped, from the form's own view.
func promptRows(m model) []string {
	lines := strings.Split(m.viewForm(), "\n")
	rows := lines[formPromptRow : formPromptRow+m.promptArea.Height()]
	for i, r := range rows {
		rows[i] = ansi.Strip(r)
	}
	return rows
}

// TestSoftWrapOffKeepsALineOnOneRow: the toggle's whole point. Wrapping, a long
// line takes several rows of the box; unwrapped it takes one, cut at the box's
// edge, and no drawn row is wider than the wrapping box was — a wider row would
// wrap in the terminal and move every click on the form below it.
func TestSoftWrapOffKeepsALineOnOneRow(t *testing.T) {
	long := strings.Repeat("word ", 60) // 300 cells, in a 96-cell box
	m := withForm(t, "", long, 100, 40)
	m.softWrap = true
	sizePromptBox(&m.promptArea, m.promptBoxWidth(), true)
	if got := len(promptDisplayLines(m.promptArea)); got < 3 {
		t.Fatalf("wrapping, the line is %d display lines, want it wrapped", got)
	}

	m = unwrappedForm(t, long+"\nsecond", 100)
	if got := len(promptDisplayLines(m.promptArea)); got != 2 {
		t.Errorf("unwrapped, two lines are %d display lines, want 2", got)
	}
	rows := promptRows(m)
	if !strings.HasPrefix(rows[1], "┃ second") {
		t.Errorf("row 1 is %q, want the second line directly under the first", rows[1])
	}
	for i, r := range strings.Split(m.viewForm(), "\n") {
		if w := lipgloss.Width(r); w > m.width {
			t.Errorf("line %d is %d cells in a %d-cell pane: %q", i, w, m.width, ansi.Strip(r))
		}
	}
	// And it persisted.
	if loadSettings().softWrap {
		t.Error("settings.json still says soft wrap on")
	}
}

// TestSoftWrapOffFollowsTheCaret: with the caret walked to the end of a line
// longer than the box, the view scrolls so the caret's cell is on screen, the
// text before it has slid off the left — and the gutter has not.
func TestSoftWrapOffFollowsTheCaret(t *testing.T) {
	long := strings.Repeat("a", 150) + "END"
	m := unwrappedForm(t, long, 100)
	m.promptArea.Focus()
	m.promptArea.MoveToBegin()
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = next.(model)
	if m.promptXOff == 0 {
		t.Fatal("caret at the end of a 153-cell line, but the view did not scroll")
	}
	row := promptRows(m)[0]
	if !strings.HasPrefix(row, "┃ ") || !strings.Contains(row, "END") {
		t.Errorf("scrolled row is %q, want the gutter and the line's end", row)
	}
	// Home brings it back to the left edge.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	if m = next.(model); m.promptXOff != 0 {
		t.Errorf("home left the view scrolled by %d", m.promptXOff)
	}
}

// TestSoftWrapOffClickLandsUnderThePointer: a click on a scrolled row places
// the caret on the character drawn under it, not on the one the library has
// at that column of the unscrolled line.
func TestSoftWrapOffClickLandsUnderThePointer(t *testing.T) {
	long := strings.Repeat("a", 150) + "XYZ" + strings.Repeat("b", 20)
	m := unwrappedForm(t, long, 100)
	m.promptArea.Focus()
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = next.(model)
	row := promptRows(m)[0]
	x := strings.Index(row, "XYZ")
	if x < 0 {
		t.Fatalf("row %q does not show XYZ", row)
	}
	x = lipgloss.Width(row[:x]) + 1 // on the Y
	m = clickForm(m, x, formPromptRow)
	if got := m.promptArea.Column(); got != 151 {
		t.Errorf("click on the Y put the caret at column %d, want 151", got)
	}
}

// TestSoftWrapChipIsFlushRight: the chip ends the toolbar's row at the pane's
// right edge on both screens, says its state with its box, and a click on it
// flips the wrap.
func TestSoftWrapChipIsFlushRight(t *testing.T) {
	t.Setenv(configDirEnvVar, t.TempDir())
	m := withForm(t, "", "body", 120, 40)
	m.softWrap = true
	chips := m.formChips()
	c := chips[formActionWrap]
	if c.end != m.width {
		t.Errorf("the wrap chip ends at %d, want the pane's edge %d", c.end, m.width)
	}
	bar := ansi.Strip(m.formBar())
	if lipgloss.Width(bar) != m.width || !strings.HasSuffix(strings.TrimSpace(bar), "☑ Soft wrap alt+z") {
		t.Errorf("toolbar is %q (%d cells), want ☑ Soft wrap at the right edge", bar, lipgloss.Width(bar))
	}
	m = clickForm(m, c.start+1, m.formBarRow())
	if m.softWrap {
		t.Fatal("clicking ☑ Soft wrap left it on")
	}
	if bar := ansi.Strip(m.formBar()); !strings.Contains(bar, "☐ Soft wrap") {
		t.Errorf("toolbar after the click is %q, want the box cleared", bar)
	}
}

// TestCropPromptLineKeepsTheStyle: the crop cuts into the middle of a styled
// run (the caret row's field, opened once at the start of the text) and the
// window must come back still inside it. ansi.Cut would drop the opening
// escape, which is why cropPromptLine exists.
func TestCropPromptLineKeepsTheStyle(t *testing.T) {
	st := lipgloss.NewStyle().Bold(true)
	line := "┃ " + st.Render("abcdefghij")
	got := cropPromptLine(line, 2, 3, 4)
	if ansi.Strip(got) != "┃ defg" {
		t.Errorf("crop shows %q, want %q", ansi.Strip(got), "┃ defg")
	}
	if !strings.Contains(got, "\x1b[1m") {
		t.Errorf("crop %q lost the run's style", got)
	}
	// A double-width glyph cut by the window's edge is drawn as the cell of it
	// inside the window, so the row keeps its width.
	if got := ansi.Strip(cropPromptLine("┃ a世b", 2, 2, 3)); got != "┃  b" {
		t.Errorf("straddled crop is %q, want %q", got, "┃  b")
	}
}

// TestPromptXOffForIsTheLeastScroll pins the follow rule.
func TestPromptXOffForIsTheLeastScroll(t *testing.T) {
	for _, tc := range []struct{ xoff, cell, cells, want int }{
		{0, 10, 20, 0},   // inside: nothing moves
		{0, 20, 20, 1},   // one past the right edge: one cell
		{30, 10, 20, 10}, // left of the window: the caret becomes its first cell
		{5, 24, 20, 5},   // the last cell of the window is still inside
	} {
		if got := promptXOffFor(tc.xoff, tc.cell, tc.cells); got != tc.want {
			t.Errorf("promptXOffFor(%d, %d, %d) = %d, want %d", tc.xoff, tc.cell, tc.cells, got, tc.want)
		}
	}
}

// longNextList is a list whose items are far longer than a 60-column row.
const longNextList = "# Next list\n\n## Open\n\n" +
	"- **N-001** · raised `x` · value high\n" +
	"  " + "alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima mike november oscar papa quebec romeo sierra tango\n" +
	"\n" +
	"- **N-002** · raised `x` · value low\n" +
	"  short one\n" +
	"\n" +
	"- **N-003** · raised `x` · value low\n" +
	"  " + "uniform victor whiskey xray yankee zulu one two three four five six seven eight nine ten eleven twelve thirteen\n"

// TestNextListSoftWrapShowsTheWholeItem: on, every item's text is all on
// screen, wrapped under its first line and no line wider than the pane; a
// click on a continuation line picks its item; and the choice persists.
func TestNextListSoftWrapShowsTheWholeItem(t *testing.T) {
	m, _ := nextModel(t, longNextList)
	m.width = 60
	m.applySizes()
	m = openNext(t, m)
	if m.next.wrap {
		t.Fatal("the page opened wrapped; off is the default")
	}
	next, _ := m.Update(altZ)
	m = next.(model)
	if !m.next.wrap || !loadSettings().nextSoftWrap {
		t.Fatalf("alt+z: wrap=%v, saved=%v, want both on", m.next.wrap, loadSettings().nextSoftWrap)
	}
	view := m.viewNextList()
	plain := ansi.Strip(view)
	for _, w := range []string{"tango", "thirteen", "short one"} {
		if !strings.Contains(plain, w) {
			t.Errorf("wrapped page does not show %q:\n%s", w, plain)
		}
	}
	lines := strings.Split(view, "\n")
	for i, ln := range lines {
		if lipgloss.Width(ln) > m.width {
			t.Errorf("line %d is %d cells in a %d-cell pane: %q", i, lipgloss.Width(ln), m.width, ansi.Strip(ln))
		}
	}
	y := -1
	for i, ln := range lines {
		if strings.Contains(ansi.Strip(ln), "tango") {
			y = i
		}
	}
	if y < 0 || strings.Contains(ansi.Strip(lines[y]), "N-001") {
		t.Fatalf("want tango on a continuation line of N-001, got line %d", y)
	}
	// N-002 first, so the click is a change of highlight.
	m.next.list.selectRef(1)
	next, _ = m.Update(tea.MouseClickMsg{X: 20, Y: y, Button: tea.MouseLeft})
	if m = next.(model); func() string { it, _ := m.next.highlighted(); return it.ID }() != "N-001" {
		t.Errorf("a click on N-001's wrapped line did not pick it")
	}
	// Off again: back to one row per item.
	next, _ = m.Update(altZ)
	m = next.(model)
	for _, s := range m.next.list.filtered {
		if len(s.item.more) > 0 {
			t.Errorf("unwrapped row %q still carries %d more lines", s.item.badge, len(s.item.more))
		}
	}
}

// TestNextListSoftWrapWindowKeepsTheCursorOnScreen: in a pane too short for
// every wrapped item, walking down scrolls by lines so the highlighted item is
// always drawn, and the page never runs past the pane's height.
func TestNextListSoftWrapWindowKeepsTheCursorOnScreen(t *testing.T) {
	m, _ := nextModel(t, longNextList)
	m.width, m.height = 40, 18
	m.nextSoftWrap = true
	m.applySizes()
	m = openNext(t, m)
	for range 3 {
		m = pressNext(t, m, "down")
		it, _ := m.next.highlighted()
		lines := strings.Split(m.viewNextList(), "\n")
		if len(lines) > m.height {
			t.Errorf("on %s the page is %d lines in a %d-line pane", it.ID, len(lines), m.height)
		}
		if !strings.Contains(ansi.Strip(strings.Join(lines, "\n")), it.ID) {
			t.Errorf("highlighted %s is not drawn:\n%s", it.ID, ansi.Strip(strings.Join(lines, "\n")))
		}
	}
}

// TestNextListWrapChipIsFlushRight: the page's bar ends in the chip, at the
// pane's edge, and a click on it flips the wrap.
func TestNextListWrapChipIsFlushRight(t *testing.T) {
	m, _ := nextModel(t, longNextList)
	m.width = 120
	m.applySizes()
	m = openNext(t, m)
	chips := m.nextChips()
	c := chips[nextActionWrap]
	if c.end != m.width {
		t.Errorf("the wrap chip ends at %d, want %d", c.end, m.width)
	}
	if bar := ansi.Strip(m.nextBar()); !strings.HasSuffix(bar, "☐ Soft wrap alt+z ") && !strings.HasSuffix(bar, "☐ Soft wrap alt+z") {
		t.Errorf("bar is %q, want it to end in ☐ Soft wrap", bar)
	}
	next, _ := m.Update(tea.MouseClickMsg{X: c.start + 1, Y: nextBarRow, Button: tea.MouseLeft})
	if m = next.(model); !m.next.wrap {
		t.Error("clicking ☐ Soft wrap did not turn it on")
	}
}

// TestSoftWrapSettingsRoundTrip: the editor's wrap defaults on and the page's
// off, a missing key reads as the default, and both read back what was saved.
func TestSoftWrapSettingsRoundTrip(t *testing.T) {
	t.Setenv(configDirEnvVar, t.TempDir())
	if s := loadSettings(); !s.softWrap || s.nextSoftWrap {
		t.Fatalf("defaults: softWrap=%v nextSoftWrap=%v, want on and off", s.softWrap, s.nextSoftWrap)
	}
	s := loadSettings()
	s.softWrap, s.nextSoftWrap = false, true
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings(); got.softWrap || !got.nextSoftWrap {
		t.Errorf("read back softWrap=%v nextSoftWrap=%v", got.softWrap, got.nextSoftWrap)
	}
}
