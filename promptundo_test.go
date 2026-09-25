package main

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/cursor"
	tea "charm.land/bubbletea/v2"
)

// The undo chord in the three spellings a terminal may report it with.
var (
	cmdZ  = tea.KeyPressMsg{Code: 'z', Mod: tea.ModSuper}
	metaZ = tea.KeyPressMsg{Code: 'z', Mod: tea.ModMeta}
	ctrlZ = tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl}
)

// typeInto drives a run of printable keys at the form the way a terminal reports
// them — one message each, through Update, which is where the history is kept.
func typeInto(t *testing.T, m model, s string) model {
	t.Helper()
	for _, r := range s {
		m = typeInForm(t, m, typeChar(r))
	}
	return m
}

// undoForm presses cmd+z once.
func undoForm(t *testing.T, m model) model {
	t.Helper()
	return typeInForm(t, m, cmdZ)
}

// TestUndoTakesBackATypedWord is the shape of the whole feature: a run of typing
// is one step, not one step per character, and the caret comes back with the
// text.
func TestUndoTakesBackATypedWord(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "alpha ")
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 6)

	m = typeInto(t, m, "beta")
	if got := m.promptArea.Value(); got != "alpha beta" {
		t.Fatalf("value = %q, want %q — the fixture never typed", got, "alpha beta")
	}
	if n := len(m.promptUndo.stack); n != 1 {
		t.Fatalf("history holds %d steps, want 1 — four keystrokes are one run", n)
	}

	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != "alpha " {
		t.Errorf("value = %q, want %q — one press takes back the word", got, "alpha ")
	}
	if got := promptCaretOffset(m.promptArea); got != 6 {
		t.Errorf("caret at %d, want 6 — where the hand was when the word started", got)
	}
	if !strings.Contains(m.formNote, "undone") {
		t.Errorf("form note = %q, want it to say the edit was undone", m.formNote)
	}
}

// TestUndoStepsAreWords: a space closes the word it ended, so walking back goes
// word by word rather than throwing away everything typed since the last arrow
// key.
func TestUndoStepsAreWords(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "")
	m = typeInto(t, m, "one two three")

	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != "one two " {
		t.Fatalf("value = %q, want %q", got, "one two ")
	}
	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != "one " {
		t.Fatalf("value = %q, want %q", got, "one ")
	}
	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != "" {
		t.Fatalf("value = %q, want the empty prompt back", got)
	}
	if !strings.Contains(m.formNote, "nothing left") {
		t.Errorf("form note = %q, want the empty history said out loud", m.formNote)
	}
}

// TestUndoRunEndsAtACaretMotion: a key that only moves the caret closes the run,
// because what is typed after moving somewhere else is a different edit.
func TestUndoRunEndsAtACaretMotion(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "")
	m = typeInto(t, m, "abc")
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyLeft})
	m = typeInto(t, m, "XY")
	if got := m.promptArea.Value(); got != "abXYc" {
		t.Fatalf("value = %q, want %q — the fixture typed somewhere unexpected", got, "abXYc")
	}

	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != "abc" {
		t.Errorf("value = %q, want %q — the motion broke the run", got, "abc")
	}
}

// TestTheBlinkDoesNotBreakATypingRun is a regression guard on the one rule that
// makes coalescing work at all. The cursor's blink is an ordinary message
// arriving on a timer; if "any message that did not change the text ends the
// run" were the rule, every character typed slowly enough would become its own
// undo step, and how many steps a word cost would depend on how fast it was
// typed.
func TestTheBlinkDoesNotBreakATypingRun(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "")
	m = typeInForm(t, m, typeChar('a'))
	next, _ := m.Update(cursor.BlinkMsg{})
	m = next.(model)
	m = typeInForm(t, m, typeChar('b'))

	if n := len(m.promptUndo.stack); n != 1 {
		t.Fatalf("history holds %d steps, want 1 — a blink landed mid-word", n)
	}
	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != "" {
		t.Errorf("value = %q, want both characters taken back in one press", got)
	}
}

// TestUndoCoalescesDeletionsSeparately: backspaces are their own run, so undoing
// a deletion does not also undo the typing in front of it.
func TestUndoCoalescesDeletionsSeparately(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "")
	m = typeInto(t, m, "hello")
	for range 3 {
		m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	if got := m.promptArea.Value(); got != "he" {
		t.Fatalf("value = %q, want %q", got, "he")
	}

	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != "hello" {
		t.Fatalf("value = %q, want the three deletions back in one press", got)
	}
	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != "" {
		t.Errorf("value = %q, want the typing back too", got)
	}
}

// TestUndoTakesBackAWholeBlockOperation: an operation that rewrites several
// lines at once is one step, which is the case the feature was really wanted
// for — a sort, an indent or a line move aimed at the wrong block.
func TestUndoTakesBackAWholeBlockOperation(t *testing.T) {
	const body = "- one\n- two\n- three"
	m, _, _ := splitFormInTemp(t, body)
	m = selectWholePrompt(t, m)
	m = typeInForm(t, m, tabKey) // indent every swept line

	if got := m.promptArea.Value(); !strings.HasPrefix(got, "    - one") {
		t.Fatalf("value = %q, want the block indented", got)
	}
	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != body {
		t.Errorf("value = %q, want the whole block back in one press", got)
	}
}

// TestUndoTakesBackAPaste: a bracketed paste is not a key press at all, which is
// the case the commit point exists for — nothing about the paste path knows the
// history is there, and it is recorded anyway. Its own step, never folded into
// the typing around it.
func TestUndoTakesBackAPaste(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "")
	m = typeInto(t, m, "abc")
	next, _ := m.Update(tea.PasteMsg{Content: "XYZ"})
	m = next.(model)
	if got := m.promptArea.Value(); got != "abcXYZ" {
		t.Fatalf("value = %q, want %q — the fixture never pasted", got, "abcXYZ")
	}

	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != "abc" {
		t.Errorf("value = %q, want the paste taken back on its own", got)
	}
}

// TestUndoDoesNotRedoItself: the restored state must not be pushed back onto the
// history, or two presses would flip between two versions forever instead of
// walking back through them.
func TestUndoDoesNotRedoItself(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "")
	m = typeInto(t, m, "abc")

	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != "" {
		t.Fatalf("value = %q, want the typing taken back", got)
	}
	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != "" {
		t.Errorf("value = %q, want the empty prompt to stay empty", got)
	}
	if n := len(m.promptUndo.stack); n != 0 {
		t.Errorf("history holds %d steps after undoing everything, want 0", n)
	}
}

// TestUndoRefusesInWords: both refusals say why, which is the contract every
// other thing this UI declines to do keeps.
func TestUndoRefusesInWords(t *testing.T) {
	t.Run("nothing to undo", func(t *testing.T) {
		m, _, _ := splitFormInTemp(t, "already here")
		m = undoForm(t, m)
		if got := m.promptArea.Value(); got != "already here" {
			t.Errorf("value = %q, want the prompt untouched", got)
		}
		if !strings.Contains(m.formNote, "nothing to undo") {
			t.Errorf("form note = %q, want the refusal in words", m.formNote)
		}
	})
	t.Run("from another field", func(t *testing.T) {
		m, _, _ := splitFormInTemp(t, "")
		m = typeInto(t, m, "abc")
		m.focusForm(formFieldTitle)
		m = undoForm(t, m)
		if got := m.promptArea.Value(); got != "abc" {
			t.Errorf("value = %q, want the prompt untouched from the title field", got)
		}
		if !strings.Contains(m.formNote, "works in the prompt") {
			t.Errorf("form note = %q, want the refusal in words", m.formNote)
		}
	})
}

// TestUndoChordSpellings: cmd+z arrives as super+z or meta+z depending on which
// bit the terminal sets for Command, and ctrl+z is the spelling that always
// arrives. All three do the same thing.
func TestUndoChordSpellings(t *testing.T) {
	for _, chord := range []tea.KeyPressMsg{cmdZ, metaZ, ctrlZ} {
		m, _, _ := splitFormInTemp(t, "")
		m = typeInto(t, m, "abc")
		m = typeInForm(t, m, chord)
		if got := m.promptArea.Value(); got != "" {
			t.Errorf("%s left %q, want the typing taken back", chord.String(), got)
		}
	}
}

// TestUndoHistoryIsPerEditingSession: a stack that outlived the form would offer
// to replace one todo's prompt with another's.
func TestUndoHistoryIsPerEditingSession(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	m.width, m.height = 120, 40
	if err := project.add(Todo{ID: "a1", Title: "first", Prompt: "first body"}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()

	next, _ := m.beginAdd()
	m = next.(model)
	m = typeInto(t, m, "draft")
	if len(m.promptUndo.stack) == 0 {
		t.Fatal("typing recorded nothing — the rest of this test proves nothing")
	}

	m.backToList()
	if n := len(m.promptUndo.stack); n != 0 {
		t.Errorf("history holds %d steps back on the list, want 0", n)
	}
	next, _ = m.beginEdit()
	m = next.(model)
	if n := len(m.promptUndo.stack); n != 0 {
		t.Fatalf("a freshly opened edit form holds %d undo steps, want 0", n)
	}
	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != "first body" {
		t.Errorf("value = %q, want the stored prompt untouched", got)
	}
	if !strings.Contains(m.formNote, "nothing to undo") {
		t.Errorf("form note = %q, want a fresh form to have nothing to undo", m.formNote)
	}
}

// TestUndoClearsWhatPointsIntoTheOldText: a highlight and a column of carets are
// both offsets into the value undo just replaced.
func TestUndoClearsWhatPointsIntoTheOldText(t *testing.T) {
	m := caretsOver(t, "one\ntwo")
	m = typeInForm(t, m, typeChar('X'))
	if !m.carets.on {
		t.Fatal("typing ended the column mode — the fixture is wrong, not the code")
	}

	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != "one\ntwo" {
		t.Errorf("value = %q, want the multi-caret typing taken back", got)
	}
	if m.carets.on {
		t.Error("the column mode survived an undo of the text its carets point into")
	}
	if _, _, ok := m.promptSelSpan(); ok {
		t.Error("a selection survived an undo")
	}
}

// TestUndoMenuRow: the editor's context menu carries the same act, dim when
// there is nothing to take back, and pressing it undoes.
func TestUndoMenuRow(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "body")
	fresh := rightClickAt(t, m, 1, 0)
	if fresh.menu.items[menuUndo].live() {
		t.Error("↶ Undo is live on a prompt that has not been changed")
	}
	if got := fresh.menu.items[menuUndo].act; got != menuUndo {
		t.Errorf("row %d holds action %d — the order is what the menu is learned by", menuUndo, got)
	}

	m = typeInto(t, m, "X")
	edited := rightClickAt(t, m, 1, 0)
	if !edited.menu.items[menuUndo].live() {
		t.Fatal("↶ Undo is dim after an edit")
	}
	next, _ := edited.pressPromptMenu(menuUndo)
	after := next.(model)
	if after.menu.open {
		t.Error("the menu stayed up over the edit it undid")
	}
	if got := after.promptArea.Value(); got != "body" {
		t.Errorf("value = %q, want %q", got, "body")
	}
}

// TestUndoHistoryIsBounded: a very long editing session must not grow without
// limit, and the newest steps are the ones kept — the far end of the history is
// the part nobody walks back to.
func TestUndoHistoryIsBounded(t *testing.T) {
	var u promptUndo
	for i := range promptUndoDepth + 50 {
		u.push(promptEdit{text: strings.Repeat("x", i)}, editOther)
	}
	if n := len(u.stack); n != promptUndoDepth {
		t.Errorf("history holds %d steps, want it capped at %d", n, promptUndoDepth)
	}
	if got := len(u.stack[len(u.stack)-1].text); got != promptUndoDepth+49 {
		t.Errorf("newest step is %d characters, want the last one pushed", got)
	}

	// And by weight: one step over the byte budget drops everything older.
	var heavy promptUndo
	heavy.push(promptEdit{text: strings.Repeat("x", promptUndoBytes/2)}, editOther)
	heavy.push(promptEdit{text: strings.Repeat("y", promptUndoBytes)}, editOther)
	if n := len(heavy.stack); n != 1 {
		t.Errorf("history holds %d oversized steps, want the budget to have dropped the older", n)
	}
}

// TestUndoMenuRowNamesTheChordTheTerminalCanSend, the way the save chord's
// footer segment does: cmd+z where Cmd can arrive at all, ctrl+z where it
// cannot.
//
// The ↶ Undo row is where the chord is taught. It used to be a segment at the
// tail of the caret footer as well, which only a pane of ~240 cells ever read.
// The footer now points at the menu ("right-click menu") instead of repeating
// the chords its rows print, so the row's hint is the one to pin.
func TestUndoMenuRowNamesTheChordTheTerminalCanSend(t *testing.T) {
	m := withForm(t, "", "body", 100, 40)
	if hint := rightClickAt(t, m, 1, 0).menu.items[menuUndo].hint; hint != "ctrl+z" {
		t.Errorf("↶ Undo hint = %q without the kitty protocol, want ctrl+z", hint)
	}
	m.kbEnhanced = true
	if hint := rightClickAt(t, m, 1, 0).menu.items[menuUndo].hint; hint != "cmd+z" {
		t.Errorf("↶ Undo hint = %q under the kitty protocol, want cmd+z", hint)
	}
}

// The redo chord in the spellings a terminal may report it with: shift+cmd+z
// under both Cmd bits, the capital-Z form of a terminal that folds shift into
// the key, and the two ctrl chords.
var (
	shiftCmdZ  = tea.KeyPressMsg{Code: 'z', Mod: tea.ModShift | tea.ModSuper}
	shiftMetaZ = tea.KeyPressMsg{Code: 'z', Mod: tea.ModShift | tea.ModMeta}
	cmdCapZ    = tea.KeyPressMsg{Code: 'Z', Mod: tea.ModSuper}
	ctrlShiftZ = tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl | tea.ModShift}
	ctrlY      = tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl}
)

// redoForm presses shift+cmd+z once.
func redoForm(t *testing.T, m model) model {
	t.Helper()
	return typeInForm(t, m, shiftCmdZ)
}

// TestRedoWalksBackUpTheHistory is N-027's reason for existing: one undo too
// many used to lose text with no way back. Each undo can now be redone, in
// order, with the caret where it was when cmd+z was pressed.
func TestRedoWalksBackUpTheHistory(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "")
	m = typeInto(t, m, "one two")
	if n := len(m.promptUndo.stack); n != 2 {
		t.Fatalf("history holds %d steps, want 2 — the fixture assumes one per word", n)
	}

	m = undoForm(t, m)
	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != "" {
		t.Fatalf("value = %q, want both words undone", got)
	}

	m = redoForm(t, m)
	if got := m.promptArea.Value(); got != "one " {
		t.Errorf("value = %q, want %q after one redo", got, "one ")
	}
	if got := promptCaretOffset(m.promptArea); got != 4 {
		t.Errorf("caret at %d, want 4 — where it stood when that undo was pressed", got)
	}
	m = redoForm(t, m)
	if got := m.promptArea.Value(); got != "one two" {
		t.Errorf("value = %q, want %q after two redos", got, "one two")
	}
	if !strings.Contains(m.formNote, "nothing further") {
		t.Errorf("form note = %q, want it to say the redo stack is spent", m.formNote)
	}

	// And a redo is itself one undoable step, so the two chords can be walked
	// back and forth without losing anything.
	m = undoForm(t, m)
	if got := m.promptArea.Value(); got != "one " {
		t.Errorf("value = %q, want the redo undone back to %q", got, "one ")
	}
	if n := len(m.promptUndo.redo); n != 1 {
		t.Errorf("redo holds %d steps, want 1", n)
	}
}

// TestRedoIsClearedByAnEditNotByAMotion: a change to the text abandons what
// could be redone. Moving the caret does not, since the undone state is still
// true of the text on screen.
func TestRedoIsClearedByAnEditNotByAMotion(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "")
	m = typeInto(t, m, "abc")
	m = undoForm(t, m)

	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyLeft})
	if n := len(m.promptUndo.redo); n != 1 {
		t.Fatalf("redo holds %d steps after caret motion, want 1 — a motion is not an edit", n)
	}

	m = typeInto(t, m, "x")
	if n := len(m.promptUndo.redo); n != 0 {
		t.Errorf("redo holds %d steps after typing, want 0 — an edit clears it", n)
	}
	m = redoForm(t, m)
	if got := m.promptArea.Value(); got != "x" {
		t.Errorf("value = %q, want %q — nothing to redo after an edit", got, "x")
	}
	if !strings.Contains(m.formNote, "nothing to redo") {
		t.Errorf("form note = %q, want the refusal in words", m.formNote)
	}
}

// TestRedoRefusesFromTheTitle, like undo: the stack is about the prompt.
func TestRedoRefusesFromTheTitle(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "")
	m = typeInto(t, m, "abc")
	m = undoForm(t, m)
	m.focusForm(formFieldTitle)
	m = redoForm(t, m)
	if got := m.promptArea.Value(); got != "" {
		t.Errorf("value = %q, want the prompt untouched from the title field", got)
	}
	if !strings.Contains(m.formNote, "redo works in the prompt") {
		t.Errorf("form note = %q, want the refusal in words", m.formNote)
	}
}

// TestRedoChordSpellings: every spelling does the same thing, and none of
// them is read as undo (a shift+cmd+z that fell through to the undo case
// would walk the wrong way).
func TestRedoChordSpellings(t *testing.T) {
	for _, chord := range []tea.KeyPressMsg{shiftCmdZ, shiftMetaZ, cmdCapZ, ctrlShiftZ, ctrlY} {
		m, _, _ := splitFormInTemp(t, "")
		m = typeInto(t, m, "abc")
		m = undoForm(t, m)
		m = typeInForm(t, m, chord)
		if got := m.promptArea.Value(); got != "abc" {
			t.Errorf("%s left %q, want the undo redone", chord.String(), got)
		}
	}
}

// TestRedoMenuRow: ↷ Redo sits under ↶ Undo, dim until an undo has left
// something to redo.
func TestRedoMenuRow(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "body")
	m = typeInto(t, m, "X")
	edited := rightClickAt(t, m, 1, 0)
	if edited.menu.items[menuRedo].live() {
		t.Error("↷ Redo is live with nothing undone")
	}

	m = undoForm(t, m)
	undone := rightClickAt(t, m, 1, 0)
	if !undone.menu.items[menuRedo].live() {
		t.Fatal("↷ Redo is dim after an undo")
	}
	next, _ := undone.pressPromptMenu(menuRedo)
	if got := next.(model).promptArea.Value(); got != "Xbody" && got != "bodyX" {
		t.Errorf("value = %q, want the typed X restored", got)
	}
}

// TestRedoMenuRowNamesTheChordTheTerminalCanSend, beside undo's.
func TestRedoMenuRowNamesTheChordTheTerminalCanSend(t *testing.T) {
	m := withForm(t, "", "body", 100, 40)
	if hint := rightClickAt(t, m, 1, 0).menu.items[menuRedo].hint; hint != "ctrl+y" {
		t.Errorf("↷ Redo hint = %q without the kitty protocol, want ctrl+y", hint)
	}
	m.kbEnhanced = true
	if hint := rightClickAt(t, m, 1, 0).menu.items[menuRedo].hint; hint != "shift+cmd+z" {
		t.Errorf("↷ Redo hint = %q under the kitty protocol, want shift+cmd+z", hint)
	}
}
