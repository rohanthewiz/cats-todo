package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// withForm returns a model sitting on the add form at a known pane size, with
// the given title and prompt already in the fields. The size is set before the
// form is opened so newFormInputs budgets the editor against it — the same order
// a real launch takes, and the thing every row constant below depends on.
func withForm(t *testing.T, title, prompt string, width, height int) model {
	t.Helper()
	m := newTestModel()
	m.width, m.height = width, height
	next, _ := m.beginAdd()
	m = next.(model)
	m.titleInput.SetValue(title)
	m.promptArea.SetValue(prompt)
	return m
}

func clickForm(m model, x, y int) model {
	next, _ := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	return next.(model)
}

// TestFormRowsMatchWhatIsDrawn pins the form's row constants to the frame it
// actually renders. Every click on the form is hit-tested against them, so a
// layout change above the editor — or a line added between the editor and the
// toolbar — would silently aim the pointer at the wrong thing.
func TestFormRowsMatchWhatIsDrawn(t *testing.T) {
	m := withForm(t, "the title", "first line\nsecond line", 100, 40)
	lines := strings.Split(m.viewForm(), "\n")

	for _, tc := range []struct {
		row  int
		want string
		what string
	}{
		{formTitleLabelRow, "Title", "the Title label"},
		{formTitleRow, "the title", "the title field"},
		{formPromptLabelRow, "Prompt", "the Prompt label"},
		{formPromptRow, "first line", "the editor's first line"},
		{formPromptRow + 1, "second line", "the editor's second line"},
		{m.formBarRow(), "Save", "the toolbar"},
	} {
		if tc.row >= len(lines) {
			t.Fatalf("row %d (%s) is past the end of a %d-line view", tc.row, tc.what, len(lines))
		}
		if !strings.Contains(lines[tc.row], tc.want) {
			t.Errorf("row %d is %q, want %s (%q):\n%s", tc.row, lines[tc.row], tc.what, tc.want, m.viewForm())
		}
	}
}

// TestFormClickPlacesCaret is the point of the whole exercise: a click inside
// the prompt lands the caret on the character that was pointed at, on the line
// that was pointed at, without the user walking there with arrow keys.
func TestFormClickPlacesCaret(t *testing.T) {
	const prompt = "alpha beta\ngamma delta\nepsilon"
	gutter := promptGutterWidth(withForm(t, "", prompt, 100, 40).promptArea)

	for _, tc := range []struct {
		name     string
		row, col int
		wantRow  int
		wantCol  int
	}{
		{"the first character of the first line", 0, 0, 0, 0},
		{"mid-word on the first line", 0, 6, 0, 6},
		{"the second line", 1, 3, 1, 3},
		{"past the end of a short line clamps to it", 2, 40, 2, len("epsilon")},
		{"below the last line lands on it", 20, 0, 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := withForm(t, "", prompt, 100, 40)
			got := clickForm(m, gutter+tc.col, formPromptRow+tc.row)
			if got.formFocus != formFieldPrompt {
				t.Fatalf("formFocus = %d, want the prompt to have taken the keys", got.formFocus)
			}
			if got.promptArea.Line() != tc.wantRow || got.promptArea.Column() != tc.wantCol {
				t.Errorf("caret at line %d col %d, want line %d col %d",
					got.promptArea.Line(), got.promptArea.Column(), tc.wantRow, tc.wantCol)
			}
		})
	}
}

// TestFormClickOnWrappedLine covers the case the mapping exists for: a soft
// wrap means a screen line is not a value line, so clicking the second half of a
// wrapped paragraph has to land inside that one logical line rather than on the
// line below it.
func TestFormClickOnWrappedLine(t *testing.T) {
	// One logical line, far wider than the pane, so the editor wraps it.
	long := strings.TrimSpace(strings.Repeat("wrapme ", 40))
	m := withForm(t, "", long+"\ntail", 40, 40)
	gutter := promptGutterWidth(m.promptArea)

	first := clickForm(m, gutter+2, formPromptRow)
	second := clickForm(m, gutter+2, formPromptRow+1)

	if first.promptArea.Line() != 0 || second.promptArea.Line() != 0 {
		t.Fatalf("lines %d and %d, want both clicks inside the first (wrapped) line",
			first.promptArea.Line(), second.promptArea.Line())
	}
	if second.promptArea.Column() <= first.promptArea.Column() {
		t.Errorf("columns %d then %d, want the second display line further into the value",
			first.promptArea.Column(), second.promptArea.Column())
	}
	// The wrap is only worth trusting if the caret is where the eye is: the
	// column the second click landed on must be about one editor width along.
	if got, w := second.promptArea.Column(), m.promptArea.Width(); got < w-len("wrapme ") || got > w+len("wrapme ") {
		t.Errorf("second display line starts at column %d, want it within a word of the %d-cell width", got, w)
	}
}

// TestFormClickKeepsTheScroll is the regression guard for how the mapping is
// implemented. Resolving a click walks a copy of the editor from the top, and
// that walk drags the shared viewport to the bottom of the value; placing the
// caret has to put the scroll back, or every click in a long prompt would jump
// the view out from under the pointer that aimed at it.
func TestFormClickKeepsTheScroll(t *testing.T) {
	var sb strings.Builder
	for i := range 60 {
		sb.WriteString(strings.Repeat("x", 10))
		if i < 59 {
			sb.WriteByte('\n')
		}
	}
	m := withForm(t, "", sb.String(), 80, 24)
	if m.promptArea.Height() >= 60 {
		t.Fatalf("editor height %d, want a value taller than the box", m.promptArea.Height())
	}
	// The editor only learns its content when it renders — the viewport is filled
	// in View — so a scroll asked for before the first frame goes nowhere. One
	// render stands in for the frame a real session would already have drawn.
	_ = m.viewForm()
	m.promptArea.MoveToEnd() // scrolled to the bottom, as after typing a long prompt
	before := m.promptArea.ScrollYOffset()
	if before == 0 {
		t.Fatal("the editor did not scroll, so this test proves nothing")
	}

	got := clickForm(m, promptGutterWidth(m.promptArea)+2, formPromptRow)
	if after := got.promptArea.ScrollYOffset(); after != before {
		t.Errorf("scroll moved from %d to %d — the click jumped the view", before, after)
	}
	// The top visible line is the first line of the value's tail, and the value's
	// lines are all one display line each here, so the clicked row is that line.
	if got, want := got.promptArea.Line(), before; got != want {
		t.Errorf("caret on line %d, want the top visible line %d", got, want)
	}
}

// TestFormClickTitleField covers the other field: a click on the title row takes
// the keys and the caret, and a click on either label focuses its field without
// moving anything.
func TestFormClickTitleField(t *testing.T) {
	m := withForm(t, "ship it", "body", 100, 40)

	got := clickForm(m, 4, formTitleRow)
	if got.formFocus != formFieldTitle {
		t.Fatalf("formFocus = %d, want the title", got.formFocus)
	}
	if pos := got.titleInput.Position(); pos != 4 {
		t.Errorf("title caret at %d, want the clicked column 4", pos)
	}

	if got := clickForm(m, 0, formPromptLabelRow); got.formFocus != formFieldPrompt {
		t.Errorf("clicking the Prompt label left the focus on %d", got.formFocus)
	}
	if got := clickForm(m, 0, formTitleLabelRow); got.formFocus != formFieldTitle {
		t.Errorf("clicking the Title label left the focus on %d", got.formFocus)
	}
}

// TestFormBarClick drives the toolbar with the pointer: every chip does what its
// chord does, and a click in the gaps between them does nothing at all.
func TestFormBarClick(t *testing.T) {
	build := func(t *testing.T) model { return withForm(t, "", "the prompt", 120, 40) }

	t.Run("Save writes the prompt and returns to the list", func(t *testing.T) {
		m := build(t)
		// Saving reaches disk, so this one gets a backlog of its own rather than
		// the shared test path every other case is content to leave alone.
		m.project = &store{scope: scopeProject, path: filepath.Join(t.TempDir(), "todos.json")}
		chips := m.formChips()
		got := clickForm(m, chips[formActionSave].start+1, m.formBarRow())
		if got.stage != stageList {
			t.Fatalf("stage = %v, want the list back", got.stage)
		}
		if len(got.project.todos) != 1 || got.project.todos[0].Prompt != "the prompt" {
			t.Fatalf("project backlog = %+v, want the saved prompt", got.project.todos)
		}
	})

	t.Run("Newline breaks the line at the caret", func(t *testing.T) {
		m := build(t)
		m.promptArea.SetCursorColumn(3)
		chips := m.formChips()
		got := clickForm(m, chips[formActionNewline].start+1, m.formBarRow())
		if want := "the\n prompt"; got.promptArea.Value() != want {
			t.Errorf("value = %q, want %q", got.promptArea.Value(), want)
		}
		if got.stage != stageForm {
			t.Errorf("stage = %v, want to stay on the form", got.stage)
		}
	})

	t.Run("Images opens the attachment editor", func(t *testing.T) {
		m := build(t)
		chips := m.formChips()
		got := clickForm(m, chips[formActionImages].start+1, m.formBarRow())
		if got.stage != stageImages {
			t.Errorf("stage = %v, want the attachment editor", got.stage)
		}
	})

	t.Run("Cancel drops the edit", func(t *testing.T) {
		m := build(t)
		chips := m.formChips()
		got := clickForm(m, chips[formActionCancel].start+1, m.formBarRow())
		if got.stage != stageList {
			t.Fatalf("stage = %v, want the list back", got.stage)
		}
		if len(got.project.todos) != 0 {
			t.Errorf("backlog = %+v, want nothing saved", got.project.todos)
		}
	})

	t.Run("a miss changes nothing", func(t *testing.T) {
		m := build(t)
		chips := m.formChips()
		for _, x := range []int{chips[0].end, chips[len(chips)-1].end + 5} {
			got := clickForm(m, x, m.formBarRow())
			if got.stage != stageForm || got.promptArea.Value() != "the prompt" {
				t.Errorf("click at x=%d gave stage=%v value=%q, want the form untouched",
					x, got.stage, got.promptArea.Value())
			}
		}
	})
}

// TestFormBarIconsAreOneCell holds the toolbar to single-column glyphs, for the
// reason the list's bar has the same test: a double-width emoji is drawn clipped
// by the terminal, and it would also put every chip's hit-test span one cell off
// what the eye sees.
func TestFormBarIconsAreOneCell(t *testing.T) {
	for _, a := range withForm(t, "", "", 120, 40).formActions() {
		icon := string([]rune(a.label)[0])
		if w := lipgloss.Width(icon); w != 1 {
			t.Errorf("%q icon %q is %d cells wide, want a one-cell dingbat", a.label, icon, w)
		}
	}
}

// TestFormMouseReportingOnForm pins the form into the set of stages that ask for
// the pointer. Without it the terminal never sends a click and every mapping
// above is dead code.
func TestFormMouseReportingOnForm(t *testing.T) {
	m := withForm(t, "", "body", 100, 40)
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Errorf("form MouseMode = %v, want cell motion so the fields are clickable", got)
	}
}

// TestFormFooterTeachesCaretKeys covers the dimmed help line: it names the keys
// no button can stand for, it leads with the pointer, and it does not repeat the
// chords the chips already print — except in a pane too narrow for chip hints,
// where it is the only teacher left.
func TestFormFooterTeachesCaretKeys(t *testing.T) {
	wide := withForm(t, "", "body", 120, 40)
	foot := wide.formFooter()
	for _, want := range []string{"click", "ctrl+a/e", "alt+", "tab"} {
		if !strings.Contains(foot, want) {
			t.Errorf("footer %q does not name %q", foot, want)
		}
	}
	if !wide.formBarShowsHints() {
		t.Fatal("a 120-cell pane should fit the chip hints")
	}
	for _, dup := range []string{"enter save", "esc cancel", "ctrl+o images"} {
		if strings.Contains(foot, dup) {
			t.Errorf("footer repeats %q, which the toolbar chips already print: %q", dup, foot)
		}
	}

	narrow := withForm(t, "", "body", 30, 40)
	if narrow.formBarShowsHints() {
		t.Fatal("a 30-cell pane cannot fit the chip hints")
	}
	// With the chips gone quiet the footer is the only teacher left, so it opens
	// with the chords — as many of them as the pane can hold.
	if first, _, _ := strings.Cut(narrow.formFooter(), "\n"); !strings.Contains(first, "enter save") {
		t.Errorf("first footer line is %q, want it to start naming the chords", first)
	}
	for line := range strings.SplitSeq(narrow.formFooter(), "\n") {
		if lipgloss.Width(line) > 30 {
			t.Errorf("footer line %q is %d cells wide in a 30-cell pane", line, lipgloss.Width(line))
		}
	}
}

// TestFormFitsThePane holds the form's line budget: the editor is sized against
// formChromeHeight, and a view taller than the pane scrolls the top of the form
// — heading, fields and all — off the screen.
func TestFormFitsThePane(t *testing.T) {
	for _, height := range []int{24, 30, 40, 60} {
		m := withForm(t, "a title", "one\ntwo\nthree", 100, height)
		m.formErr = "something went wrong" // the form at its tallest
		if n := len(strings.Split(m.viewForm(), "\n")); n > height {
			t.Errorf("at height %d the form renders %d lines", height, n)
		}
	}
}
