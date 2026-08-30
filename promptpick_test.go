package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// libraryFixture is the library every stage test here browses: one prose
// snippet, two commands. Small enough to reason about by hand, and shaped to
// have both kinds — which is the only distinction the picker makes.
const libraryFixture = `{"prompts":[
  {"name":"repro steps","desc":"how to file a bug","body":"Steps to reproduce:\n1. "},
  {"name":"load session","desc":"pick up where we left off","body":"/sess-load"},
  {"name":"wrap up","body":"/sess-wrap"}
]}`

// formWithLibrary opens an add form with prompt already typed and the caret at
// its end, over a library written to the temp config directory. The prompt field
// has the keys, since both ways into the picker are from the editor.
func formWithLibrary(t *testing.T, prompt, lib string) model {
	t.Helper()
	m, _, _ := newModelInTemp(t)
	m.width, m.height = 100, 30
	if lib != "" {
		writeLib(t, promptLibPath(), lib)
	}
	next, _ := m.beginAdd()
	m = next.(model)
	m.focusForm(formFieldPrompt)
	m.promptArea.SetValue(prompt)
	setPromptCaretOffset(&m.promptArea, len([]rune(prompt)))
	return m
}

// openLibrary is the chord's path in: ctrl+p from the editor.
func openLibrary(t *testing.T, prompt string) model {
	t.Helper()
	m := formWithLibrary(t, prompt, libraryFixture)
	m = typeInForm(t, m, pressKey("ctrl+p"))
	if m.stage != stageSnippets {
		t.Fatalf("ctrl+p left stage = %v, want stageSnippets", m.stage)
	}
	return m
}

func TestCtrlPListsTheWholeLibrary(t *testing.T) {
	m := openLibrary(t, "")
	if len(m.snips.shown) != 3 {
		t.Fatalf("shown = %+v, want all three entries", m.snips.shown)
	}
	if m.snips.purpose != snippetsAll {
		t.Errorf("purpose = %v, want the whole library", m.snips.purpose)
	}
	// The rows carry the command beside the name, so what is about to land in
	// the prompt is readable before enter.
	view := m.viewSnippets()
	for _, want := range []string{"Insert a prompt", "repro steps", "how to file a bug", "/sess-load"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
}

// TestSnippetLandsAtTheCaret: a prose entry is inserted exactly where the caret
// is and changes nothing around it — its author already decided its shape.
func TestSnippetLandsAtTheCaret(t *testing.T) {
	m := formWithLibrary(t, "before after", libraryFixture)
	setPromptCaretOffset(&m.promptArea, 7) // between "before " and "after"
	m = typeInForm(t, m, pressKey("ctrl+p"))
	next, _ := m.updateSnippets(pressKey("enter")) // "repro steps" is the first row
	m = next.(model)

	if m.stage != stageForm {
		t.Fatalf("stage = %v after insert, want the form back", m.stage)
	}
	if got, want := m.promptArea.Value(), "before Steps to reproduce:\n1. after"; got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
	if !strings.Contains(m.formNote, "repro steps") {
		t.Errorf("note = %q, want it to name what was inserted", m.formNote)
	}
}

// TestCommandGetsItsOwnLine: a slash command is only a command at a line start,
// so inserting one after text opens a line for it and leaves the caret on the
// next one.
func TestCommandGetsItsOwnLine(t *testing.T) {
	m := openLibrary(t, "fix the crash in drop.go")
	m.snips.list.moveDown() // "load session" — /sess-load
	next, _ := m.updateSnippets(pressKey("enter"))
	m = next.(model)

	if got, want := m.promptArea.Value(), "fix the crash in drop.go\n/sess-load\n"; got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
	if got := promptCaretOffset(m.promptArea); got != len([]rune(m.promptArea.Value())) {
		t.Errorf("caret at %d, want the end of the fresh line", got)
	}
	if !strings.Contains(m.formNote, "/sess-load") {
		t.Errorf("note = %q, want it to name the command", m.formNote)
	}
}

// TestSlashAtALineStartOpensTheCommands is the second way in. Both halves
// matter: only commands are listed, and the slash the user typed is consumed by
// the entry rather than left in front of it.
func TestSlashAtALineStartOpensTheCommands(t *testing.T) {
	m := formWithLibrary(t, "fix the crash\n", libraryFixture)
	m = typeInForm(t, m, pressKey("/"))
	if m.stage != stageSnippets {
		t.Fatalf("a '/' at a line start left stage = %v, want the picker", m.stage)
	}
	if m.snips.purpose != snippetsCommands {
		t.Errorf("purpose = %v, want commands only", m.snips.purpose)
	}
	if len(m.snips.shown) != 2 {
		t.Errorf("shown = %+v, want only the two commands", m.snips.shown)
	}
	if !strings.Contains(m.viewSnippets(), "Insert a command") {
		t.Errorf("heading does not say what is being picked:\n%s", m.viewSnippets())
	}
	// The slash is in the editor already — esc has to leave it behind.
	if got, want := m.promptArea.Value(), "fix the crash\n/"; got != want {
		t.Errorf("prompt = %q while the picker is up, want %q", got, want)
	}

	next, _ := m.updateSnippets(pressKey("enter"))
	m = next.(model)
	if got, want := m.promptArea.Value(), "fix the crash\n/sess-load\n"; got != want {
		t.Errorf("prompt = %q, want %q — the command replaces the typed slash rather than doubling it", got, want)
	}
}

func TestEscLeavesThePlainSlash(t *testing.T) {
	m := formWithLibrary(t, "notes\n", libraryFixture)
	m = typeInForm(t, m, pressKey("/"))
	next, _ := m.updateSnippets(pressKey("esc"))
	m = next.(model)
	if m.stage != stageForm {
		t.Fatalf("esc left stage = %v, want the form", m.stage)
	}
	if got, want := m.promptArea.Value(), "notes\n/"; got != want {
		t.Errorf("prompt = %q, want %q — someone who wanted the character keeps it", got, want)
	}
}

// TestSlashIsOrdinaryPunctuationEverywhereElse: the guard is what keeps the
// trigger out of the way of prose and of paths.
func TestSlashIsOrdinaryPunctuationEverywhereElse(t *testing.T) {
	t.Run("mid-line", func(t *testing.T) {
		m := formWithLibrary(t, "read and", libraryFixture)
		m = typeInForm(t, m, pressKey("/"))
		if m.stage != stageForm {
			t.Errorf("a '/' inside a line opened %v", m.stage)
		}
		if got, want := m.promptArea.Value(), "read and/"; got != want {
			t.Errorf("prompt = %q, want %q", got, want)
		}
	})
	t.Run("a library with no commands never opens", func(t *testing.T) {
		m := formWithLibrary(t, "", `{"prompts":[{"name":"repro","body":"Steps:"}]}`)
		m = typeInForm(t, m, pressKey("/"))
		if m.stage != stageForm {
			t.Errorf("a '/' opened %v with no commands to offer", m.stage)
		}
	})
	t.Run("no library at all never opens", func(t *testing.T) {
		m := formWithLibrary(t, "", "")
		m = typeInForm(t, m, pressKey("/"))
		if m.stage != stageForm {
			t.Errorf("a '/' opened %v with no library file", m.stage)
		}
		if got, want := m.promptArea.Value(), "/"; got != want {
			t.Errorf("prompt = %q, want %q", got, want)
		}
	})
}

// TestSavingTheSelection is the library growing while you work: sweep a run,
// ctrl+p, type a name, ctrl+s. The offer is named in the footer before it is
// taken, and the entry is on disk afterwards.
func TestSavingTheSelection(t *testing.T) {
	m := formWithLibrary(t, "keep this\nnot this", libraryFixture)
	m = selectPromptRange(t, m, 0, 9) // "keep this"
	m = typeInForm(t, m, pressKey("ctrl+p"))

	if m.snips.capture != "keep this" || m.snips.captureWhat != "the selection" {
		t.Fatalf("capture = %q/%q, want the swept run", m.snips.capture, m.snips.captureWhat)
	}
	if !strings.Contains(m.viewSnippets(), "ctrl+s saves the selection") {
		t.Errorf("footer does not offer the save:\n%s", m.viewSnippets())
	}
	// The highlight ends when the picker takes the keys — its text has been
	// read, and an insertion would move the anchor out from under it.
	if _, _, ok := m.promptSelSpan(); ok {
		t.Error("the selection is still standing behind the picker")
	}

	m = typeAll(t, m, "kept")
	next, _ := m.updateSnippets(pressKey("ctrl+s"))
	m = next.(model)

	if m.snips.err {
		t.Fatalf("save reported %q", m.snips.note)
	}
	if !strings.Contains(m.snips.note, "kept") {
		t.Errorf("note = %q, want it to name what was saved", m.snips.note)
	}
	if m.snips.list.input.Value() != "" {
		t.Errorf("query still holds %q — it was the name, and it would now filter the list to one row", m.snips.list.input.Value())
	}
	if len(m.snips.shown) != 4 {
		t.Errorf("shown = %d rows, want the new entry among them", len(m.snips.shown))
	}
	lib := loadPromptLib()
	if i := lib.find("kept"); i < 0 || lib.snippets[i].Body != "keep this" {
		t.Errorf("library on disk = %+v, want the swept run under “kept”", lib.snippets)
	}
}

// With nothing swept the offer falls back to the whole prompt, which is the
// other thing worth keeping — a prompt you just wrote and will write again.
func TestSavingTheWholePromptWhenNothingIsSwept(t *testing.T) {
	m := openLibrary(t, "  the whole thing  ")
	if m.snips.capture != "the whole thing" || m.snips.captureWhat != "this prompt" {
		t.Fatalf("capture = %q/%q, want the trimmed prompt", m.snips.capture, m.snips.captureWhat)
	}
	if !strings.Contains(m.viewSnippets(), "ctrl+s saves this prompt") {
		t.Errorf("footer does not say which text would be saved:\n%s", m.viewSnippets())
	}
}

// Every refusal says what is missing and leaves the picker up, so the fix is one
// keystroke away.
func TestSaveRefusesInWords(t *testing.T) {
	t.Run("no name typed", func(t *testing.T) {
		m := openLibrary(t, "something")
		next, _ := m.updateSnippets(pressKey("ctrl+s"))
		m = next.(model)
		if !m.snips.err || !strings.Contains(m.snips.note, "name") {
			t.Errorf("note = %q (err %v), want it to ask for a name", m.snips.note, m.snips.err)
		}
		if m.stage != stageSnippets {
			t.Errorf("a refused save closed the picker (stage %v)", m.stage)
		}
	})
	t.Run("nothing to save", func(t *testing.T) {
		m := openLibrary(t, "")
		m = typeAll(t, m, "name")
		next, _ := m.updateSnippets(pressKey("ctrl+s"))
		m = next.(model)
		if !m.snips.err || !strings.Contains(m.snips.note, "nothing to save") {
			t.Errorf("note = %q, want it to say there is no text", m.snips.note)
		}
	})
	t.Run("the name is taken", func(t *testing.T) {
		m := openLibrary(t, "something")
		m = typeAll(t, m, "wrap up")
		next, _ := m.updateSnippets(pressKey("ctrl+s"))
		m = next.(model)
		if !m.snips.err || !strings.Contains(m.snips.note, "already in the library") {
			t.Errorf("note = %q, want it to say the name is taken", m.snips.note)
		}
		if got := loadPromptLib().snippets; len(got) != 3 {
			t.Errorf("library = %d entries, want the original three untouched", len(got))
		}
	})
	// A key that edits the query answers the note: a stale "type a name first"
	// over a box with a name in it is worse than no message at all.
	t.Run("typing clears the complaint", func(t *testing.T) {
		m := openLibrary(t, "something")
		next, _ := m.updateSnippets(pressKey("ctrl+s"))
		m = next.(model)
		next, _ = m.updateSnippets(pressKey("a"))
		m = next.(model)
		if m.snips.note != "" {
			t.Errorf("note = %q after typing, want it cleared", m.snips.note)
		}
	})
}

// TestEmptyLibrarySaysWhereItLives: the one question someone with no library
// has is where to put one, so the empty state answers it.
func TestEmptyLibrarySaysWhereItLives(t *testing.T) {
	m := formWithLibrary(t, "", "")
	m = typeInForm(t, m, pressKey("ctrl+p"))
	if m.stage != stageSnippets {
		t.Fatalf("ctrl+p over an empty library left stage = %v", m.stage)
	}
	if got := m.snips.emptyMessage(); !strings.Contains(got, "prompts.json") {
		t.Errorf("empty message = %q, want it to name the file", got)
	}
}

// TestCtrlPFromTheTitleRefusesInWords: a library entry is inserted at the
// editor's caret, and the title has no editor. Silence would be the wrong
// answer — the program says where the chord works.
func TestCtrlPFromTheTitleRefusesInWords(t *testing.T) {
	m := formWithLibrary(t, "", libraryFixture)
	m.focusForm(formFieldTitle)
	m = typeInForm(t, m, pressKey("ctrl+p"))
	if m.stage != stageForm {
		t.Fatalf("ctrl+p from the title opened %v", m.stage)
	}
	if !strings.Contains(m.formNote, "prompt") {
		t.Errorf("note = %q, want it to name where the chord works", m.formNote)
	}
}

// TestSnippetsRowsMatchWhatIsDrawn pins snippetsRowsRow to a rendered frame, the
// way the file picker pins filesRowsRow: clickSnippets subtracts it from the
// pointer's row, so a line added above the rows would aim clicks one row off.
func TestSnippetsRowsMatchWhatIsDrawn(t *testing.T) {
	m := openLibrary(t, "")
	lines := strings.Split(m.viewSnippets(), "\n")
	if len(lines) <= snippetsRowsRow+1 {
		t.Fatalf("view has only %d lines:\n%s", len(lines), m.viewSnippets())
	}
	if !strings.Contains(lines[0], "Insert a prompt") {
		t.Errorf("heading is %q", lines[0])
	}
	if !strings.Contains(lines[snippetsRowsRow], "repro steps") {
		t.Errorf("row %d is %q, want the first entry", snippetsRowsRow, lines[snippetsRowsRow])
	}
	if !strings.Contains(lines[snippetsRowsRow+1], "load session") {
		t.Errorf("row %d is %q, want the second entry", snippetsRowsRow+1, lines[snippetsRowsRow+1])
	}
}

func TestClickOnASnippetRowChoosesIt(t *testing.T) {
	m := openLibrary(t, "")
	m = clickForm(m, 3, snippetsRowsRow+1) // the second row: /sess-load
	if m.stage != stageForm {
		t.Fatalf("stage = %v after a row click, want the form", m.stage)
	}
	if got, want := m.promptArea.Value(), "/sess-load\n"; got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}

	// A click off the rows does nothing.
	m = openLibrary(t, "")
	m = clickForm(m, 3, 0)
	if m.stage != stageSnippets {
		t.Errorf("a click on the heading changed the stage to %v", m.stage)
	}
}

func TestSnippetsStageAsksForTheMouse(t *testing.T) {
	m := openLibrary(t, "")
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Errorf("snippets stage MouseMode = %v, want cell motion so rows are clickable", got)
	}
}

// TestSnippetQueryMatchesTheBody: an entry is as often remembered by what it
// inserts as by what it is called, so the body is in the haystack too.
func TestSnippetQueryMatchesTheBody(t *testing.T) {
	m := openLibrary(t, "")
	m = typeAll(t, m, "reproduce")
	if got := len(m.snips.list.filtered); got != 1 {
		t.Fatalf("%d rows matched “reproduce”, want the one whose body says it", got)
	}
	s, ok := m.snips.highlighted()
	if !ok || s.Name != "repro steps" {
		t.Errorf("highlighted %+v, want the repro entry", s)
	}
}

// A resize must be safe on this stage like every other — the regression
// TestWindowSizeMsgNeverPanics guards, applied to the newest sub-stage.
func TestResizeOnTheSnippetsStage(t *testing.T) {
	m := openLibrary(t, "")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(model)
	if m.snips.list.maxRows <= 0 {
		t.Errorf("maxRows = %d after a resize, want the list windowed to the pane", m.snips.list.maxRows)
	}
}

// TestContextMenuOpensTheLibrary: the right-click menu is where the editor's
// gestures are learned, so the library has a row on it. The row is always live,
// and it carries the swept run in with it — right-click a paragraph, Insert a
// prompt…, name it, ctrl+s.
func TestContextMenuOpensTheLibrary(t *testing.T) {
	m := formWithLibrary(t, "keep this line", libraryFixture)
	m = selectPromptRange(t, m, 0, 9) // "keep this"
	m = rightClickAt(t, m, 3, 0)

	if got := m.menu.items[menuInsert]; !got.live() || !strings.Contains(got.label, "Insert a prompt") {
		t.Fatalf("menu row = %+v, want a live Insert a prompt row", got)
	}
	next, _ := m.pressPromptMenu(menuInsert)
	m = next.(model)
	if m.stage != stageSnippets {
		t.Fatalf("the menu row left stage = %v, want the picker", m.stage)
	}
	if m.snips.capture != "keep this" {
		t.Errorf("capture = %q, want the run the right-click was aimed at", m.snips.capture)
	}
	if m.menu.open {
		t.Error("the menu is still up behind the picker")
	}
}

// A command row says what it will run beside its name, and does not then repeat
// it as a description — one thing said twice on a row is a row nobody reads.
func TestCommandRowShowsTheWholeCommandOnce(t *testing.T) {
	m := formWithLibrary(t, "", `{"prompts":[{"name":"wrap up","body":"/sess-wrap 2"}]}`)
	m = typeInForm(t, m, pressKey("ctrl+p"))
	it := m.snips.list.items[0]
	if it.tag != "/sess-wrap 2" {
		t.Errorf("tag = %q, want the whole command line, arguments included", it.tag)
	}
	if it.desc != "" {
		t.Errorf("desc = %q, want it empty — the tag already says what runs", it.desc)
	}
}

// TestSnippetLibChordSpellings: cmd+P arrives as super+p or meta+p depending on
// the terminal, and cats forwards it. Nothing else in the suite can press those
// — pressKey has no way to build a Cmd chord — so the predicate is pinned here.
func TestSnippetLibChordSpellings(t *testing.T) {
	for _, s := range []string{"ctrl+p", "super+p", "meta+p"} {
		if !snippetLibChord(s) {
			t.Errorf("%s does not open the library", s)
		}
	}
	for _, s := range []string{"p", "ctrl+n", "alt+p", "ctrl+shift+p"} {
		if snippetLibChord(s) {
			t.Errorf("%s opens the library and should not", s)
		}
	}
}
