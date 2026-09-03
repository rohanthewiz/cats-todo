package main

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// pressFlagSeg presses the bar's ⚑ segment from the keyboard, through the whole
// dispatch a real keystroke takes: focus the bar, walk the cursor onto the flag,
// press space. Driving it this way rather than calling activateAnnotSeg is the
// point — the focus move the press performs is half of what is under test.
func pressFlagSeg(t *testing.T, m model) model {
	t.Helper()
	m.formFocus = formFieldAnnots
	m.annotCursor = annotSegFlag
	next, _ := m.updateForm(pressKey(" "))
	return next.(model)
}

// TestFlagSegmentRaisesTheNoteField: ticking ⚑ Flag is one gesture with the note
// it opens — the field appears and takes the keys in the same press, because
// "flag this, because…" is a single thought and a field that had to be tabbed to
// would break it in half. Typing then reaches the note and lands in the
// annotation set on every keystroke, so a save from any stop writes what is on
// screen.
func TestFlagSegmentRaisesTheNoteField(t *testing.T) {
	m := withForm(t, "t", "p", 100, 40)
	m = pressFlagSeg(t, m)

	if !m.formAnnots.Flag {
		t.Fatal("pressing the ⚑ segment did not raise the flag")
	}
	if m.formFocus != formFieldFlagNote {
		t.Fatalf("formFocus = %d, want the note field the press just opened", m.formFocus)
	}

	for _, k := range []string{"h", "i"} {
		next, _ := m.updateForm(pressKey(k))
		m = next.(model)
	}
	if got := m.flagInput.Value(); got != "hi" {
		t.Errorf("the note field holds %q, want the typing that followed the press", got)
	}
	if got := m.formAnnots.FlagNote; got != "hi" {
		t.Errorf("formAnnots.FlagNote = %q — the set did not keep up with the field", got)
	}
}

// TestClearingTheFlagDropsItsNote: the note belongs to the mark. Clearing the
// flag takes the words with it — the same turn annots.applyTo makes on the way
// to the file — and hands the keys back to the bar the press came from, since a
// caret left blinking on a row the form has stopped drawing is a field that
// isn't there.
func TestClearingTheFlagDropsItsNote(t *testing.T) {
	m := withForm(t, "t", "p", 100, 40)
	m = pressFlagSeg(t, m)
	m.flagInput.SetValue("blocked")
	m.formAnnots.FlagNote = "blocked"

	m = pressFlagSeg(t, m)
	if m.formAnnots.Flag {
		t.Fatal("a second press did not clear the flag")
	}
	if m.formAnnots.FlagNote != "" || m.flagInput.Value() != "" {
		t.Errorf("note = %q / field = %q, want both cleared with the mark",
			m.formAnnots.FlagNote, m.flagInput.Value())
	}
	if m.formFocus != formFieldAnnots {
		t.Errorf("formFocus = %d, want the keys back on the bar", m.formFocus)
	}
}

// TestFlagNoteRowKeepsTheFormsGeometry is the regression this feature is most
// likely to cause. The note takes over the blank line that was already between
// the bar and the Prompt label (formFlagNoteRow), so the editor, the toolbar and
// every hit-test counting from formPromptRow stay exactly where they were. A
// note field that pushed the form down one row when a checkbox was ticked would
// move the buttons out from under the pointer.
func TestFlagNoteRowKeepsTheFormsGeometry(t *testing.T) {
	down := withForm(t, "the title", "first line\nsecond line", 100, 40)
	up := pressFlagSeg(t, down)
	up.flagInput.SetValue("blocked on the api")

	downLines := strings.Split(down.viewForm(), "\n")
	upLines := strings.Split(up.viewForm(), "\n")

	if got := strings.TrimSpace(downLines[formFlagNoteRow]); got != "" {
		t.Errorf("row %d is %q with the flag down, want the blank it has always been", formFlagNoteRow, got)
	}
	if got := upLines[formFlagNoteRow]; !strings.Contains(got, "blocked on the api") {
		t.Errorf("row %d is %q, want the note field", formFlagNoteRow, got)
	}
	// Everything under it is untouched, which is the whole promise.
	if len(upLines) != len(downLines) {
		t.Fatalf("the form is %d lines with the flag up and %d with it down", len(upLines), len(downLines))
	}
	for _, tc := range []struct {
		row  int
		want string
	}{
		{formPromptLabelRow, "Prompt"},
		{formPromptRow, "first line"},
		{formPromptRow + 1, "second line"},
		{up.formBarRow(), "Save"},
	} {
		if !strings.Contains(upLines[tc.row], tc.want) {
			t.Errorf("with the flag up, row %d is %q, want %q", tc.row, upLines[tc.row], tc.want)
		}
	}
	// And the row is still one line however long the note is — it sits on a
	// hit-tested row, so a wrap here would cost the rows below it their aim.
	up.flagInput.SetValue(strings.Repeat("wordy ", 40))
	if got := lipgloss.Width(strings.Split(up.viewForm(), "\n")[formFlagNoteRow]); got > 100 {
		t.Errorf("a long note drew %d cells in a 100-cell pane", got)
	}
}

// TestTabSkipsTheNoteFieldWhileTheFlagIsDown: the note is the form's one
// conditional stop, so the walk has to step over it — a tab that appeared to do
// nothing (twice, on the way round) is worse than one stop fewer.
func TestTabSkipsTheNoteFieldWhileTheFlagIsDown(t *testing.T) {
	m := withForm(t, "t", "p", 100, 40)
	seen := map[int]bool{}
	for range formFieldCount * 2 {
		next, _ := m.updateForm(pressKey("tab"))
		m = next.(model)
		seen[m.formFocus] = true
	}
	if seen[formFieldFlagNote] {
		t.Error("tab landed on the note field while the flag was down")
	}
	for _, want := range []int{formFieldTitle, formFieldPrompt, formFieldAnnots} {
		if !seen[want] {
			t.Errorf("tab never reached stop %d", want)
		}
	}

	// With the flag up the stop joins the ring, in both directions.
	up := pressFlagSeg(t, m)
	next, _ := up.updateForm(pressKey("tab"))
	if got := next.(model).formFocus; got != formFieldTitle {
		t.Errorf("tab off the note field went to %d, want the ring's next stop", got)
	}
	next, _ = up.updateForm(pressKey("shift+tab"))
	if got := next.(model).formFocus; got != formFieldAnnots {
		t.Errorf("shift+tab off the note field went to %d, want the bar above it", got)
	}
}

// TestFlagNoteSurvivesASave walks the whole round trip the feature exists for:
// flag a prompt on the form, write why, save, and find both on the todo — then
// reopen the form and find the field pre-filled with what was written.
func TestFlagNoteSurvivesASave(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	m.width, m.height = 100, 40
	next, _ := m.beginAdd()
	m = next.(model)
	m.promptArea.SetValue("do the thing")
	m = pressFlagSeg(t, m)
	for _, k := range strings.Split("why", "") {
		n, _ := m.updateForm(pressKey(k))
		m = n.(model)
	}
	next, _ = m.saveForm()
	m = next.(model)

	if len(project.todos) != 1 {
		t.Fatalf("the backlog holds %d todos, want the one just saved", len(project.todos))
	}
	td := project.todos[0]
	if !td.Flag || td.FlagNote != "why" {
		t.Fatalf("saved todo = %+v, want it flagged with its note", td)
	}

	m.rebuildList()
	next, _ = m.beginEditRef(todoRef{scope: scopeProject, id: td.ID})
	back := next.(model)
	if !back.formAnnots.Flag || back.flagInput.Value() != "why" {
		t.Errorf("the reopened form shows flag=%v note=%q, want what was saved",
			back.formAnnots.Flag, back.flagInput.Value())
	}
	// And the row is drawn, so the mark the note explains is findable.
	if !strings.Contains(back.viewForm(), flagGlyph) {
		t.Error("the reopened form draws no ⚑ on its annotation bar")
	}
}

// TestClickingTheNoteRowTakesTheKeys: unlike the bar above it, this row is a
// text field, so a click into it focuses it — that is what clicking a field
// means everywhere else on this form. While the flag is down the row is blank
// and the click does nothing at all.
func TestClickingTheNoteRowTakesTheKeys(t *testing.T) {
	m := withForm(t, "t", "p", 100, 40)
	down := clickForm(m, 12, formFlagNoteRow)
	if down.formFocus == formFieldFlagNote {
		t.Error("a click on the blank row focused a field that is not drawn")
	}

	up := pressFlagSeg(t, m)
	up.formFocus = formFieldPrompt // park the keys elsewhere first
	up = clickForm(up, 12, formFlagNoteRow)
	if up.formFocus != formFieldFlagNote {
		t.Errorf("formFocus = %d after a click on the note row, want the note field", up.formFocus)
	}
}
