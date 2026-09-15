// promptundo.go — taking back the last thing that happened to the prompt.
//
// The editor gained a lot of ways to change a prompt in one keystroke — a sort,
// a block indent, a line move, a duplicate, typing at a dozen carets at once, a
// snippet dropped in at the caret. Every one of them is an edit a hand can aim
// wrong, and until now the only way back was to retype what was there. So the
// editor keeps a stack of where it has been, and cmd+z walks back down it.
//
// Two decisions shape the whole file:
//
//   - **It records by watching, not by asking.** Every one of those operations
//     would otherwise have to remember to snapshot itself, and the one that
//     forgot would be a silent hole in the feature. Instead there is a single
//     commit point (model.Update, ui.go): the editor's text before the message
//     is routed, compared with the text after. Anything that changed the prompt
//     is recorded, whether it was a chord, a menu row, a paste, a picker on
//     another stage, or a keystroke the textarea handled by itself.
//
//   - **A run of like keys is one step.** Typing "hello" and pressing cmd+z five
//     times would be a machine's idea of undo, not an editor's. Consecutive
//     insertions coalesce into the entry that started them, as do consecutive
//     deletions; a word boundary, a newline, a caret motion, a click, or any
//     other kind of edit ends the run, and the next change starts a fresh entry.
//     So one press takes back the word just typed, the block just indented, or
//     the sort just run — a unit somebody would recognize as "what I last did".
//
//	     stack (oldest ─▶ newest)              kind
//	   ┌──────────┬──────────┬──────────┐
//	   │ ""       │ "the "   │ "the cat"│      typing   ← coalescing here
//	   └──────────┴──────────┴──────────┘
//	     ▲ each entry is the text as it was *before* the edit that pushed it,
//	       plus where the caret stood, so restoring one restores both.
//
// What it does not cover is anything that has already left the editor. ✂ Split
// writes prompts into the backlog and then takes the bullets out of the text:
// undo brings the text back, and the prompts it wrote stay written. That is the
// honest half — the editor can only take back what is still in the editor — and
// the README says so where the split is documented.

package main

import (
	"unicode"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

// promptUndoDepth is how many steps back the editor remembers, and
// promptUndoBytes the ceiling on what those steps may weigh.
//
// Both, because either alone has a bad case: 200 steps over a pasted 2MB plan
// would hold 400MB of near-identical strings, and a byte budget on its own would
// let a thousand one-character steps accumulate unbounded slice overhead. The
// numbers are chosen to be far past any editing session a person actually has —
// what they are really guarding is the pathological one.
const (
	promptUndoDepth = 200
	promptUndoBytes = 4 << 20 // 4MB of history, oldest dropped first
)

// promptEditKind is what sort of change produced an entry, which is the only
// thing coalescing needs to know. The zero value means "no run is open", so a
// cleared or freshly-broken stack can never merge the next edit into whatever
// happened to be on top of it.
type promptEditKind int

const (
	editNone     promptEditKind = iota // no open run: the next change starts an entry
	editTyping                         // a character or a newline went in
	editDeleting                       // a backspace or a delete took something out
	editOther                          // everything else: never coalesces, always its own step
)

// promptEdit is one restorable state of the editor: its whole text, and the
// caret's absolute rune offset into it.
//
// The whole text rather than a diff. A diff would be smaller, but every
// operation here can rewrite the block arbitrarily (a sort permutes lines, a
// multi-caret paste inserts in a dozen places), so a patch model would need a
// real diff engine to build the patches — and the budget above already bounds
// what the simple thing costs.
type promptEdit struct {
	text  string
	caret int
}

// promptUndo is the editor's history. Its zero value is an empty one, which is
// what beginAdd/beginEditRef restore when a form opens: a stack is about one
// editing session, and offering to "undo" into the text of the todo edited
// before this one would be the worst kind of surprise.
type promptUndo struct {
	stack []promptEdit
	// kind is what produced the entry on top, or editNone when the run has been
	// broken. It is the entire coalescing rule.
	kind promptEditKind
	// applied marks that the change the commit point is about to see *is* an
	// undo restoring an earlier state. Without it, undo would push the state it
	// just left back onto the stack and the next press would redo it — cmd+z
	// flip-flopping between two versions forever.
	applied bool
}

// promptEditState is the editor as it stands, ready to be pushed if the message
// about to be routed turns out to change it.
func (m model) promptEditState() promptEdit {
	text := m.promptArea.Value()
	return promptEdit{text: text, caret: promptCaretOffsetIn(text, m.promptArea.Line(), m.promptArea.Column())}
}

// editsPrompt reports whether the prompt editor's text is live on this stage:
// the form, and the sub-stages opened from it that write into the editor — the
// spelling panel replaces a word, the file picker inserts a path, the prompt
// library inserts a body.
//
// The commit point requires it on both sides of a message, which is what keeps
// the history about one editor. Arriving on the form from the list replaces the
// editor's whole value; that is a new session, not an edit, and the stage it
// came from says so.
func (s uiStage) editsPrompt() bool {
	switch s {
	case stageForm, stageSpell, stageFiles, stageSnippets:
		return true
	}
	return false
}

// commitPromptEdit is the commit point itself, called by Update with the state
// the editor was in before the message was routed.
//
// Three outcomes: the change was an undo (consume the flag and record nothing),
// the text is unchanged (a key or a click that only moved the caret ends the
// open run), or the text changed (push, or coalesce into the run on top).
//
// Only a key press or a click breaks a run, and that distinction is load
// bearing: the cursor's blink is a message like any other and arrives every few
// hundred milliseconds, so a rule of "anything that didn't change the text ends
// the run" would put every typed character in its own step, on a timer.
func (m *model) commitPromptEdit(before promptEdit, msg tea.Msg) {
	if m.promptUndo.applied {
		m.promptUndo.applied = false
		return
	}
	if m.promptArea.Value() == before.text {
		switch msg.(type) {
		case tea.KeyPressMsg, tea.MouseClickMsg:
			m.promptUndo.kind = editNone
		}
		return
	}
	kind, endsRun := promptEditKindOf(msg, m.promptArea.KeyMap)
	m.promptUndo.push(before, kind)
	if endsRun {
		m.promptUndo.kind = editNone
	}
}

// push adds before to the history, unless it belongs to the run already on top.
func (u *promptUndo) push(before promptEdit, kind promptEditKind) {
	// A run coalesces by *not* pushing: the entry already on top is the state the
	// run started from, which is exactly where one press should land.
	if len(u.stack) > 0 && kind == u.kind && (kind == editTyping || kind == editDeleting) {
		return
	}
	u.stack = append(u.stack, before)
	u.kind = kind
	u.trim()
}

// trim enforces both budgets by dropping the oldest steps — the far end of the
// history is the part nobody is walking back to, and losing it costs less than
// the alternatives (refusing to record, or growing without bound).
func (u *promptUndo) trim() {
	drop := max(len(u.stack)-promptUndoDepth, 0)
	total := 0
	for _, e := range u.stack[drop:] {
		total += len(e.text)
	}
	// Never below one entry: a single pasted prompt can be over budget all by
	// itself, and one step of undo is still a feature.
	for drop < len(u.stack)-1 && total > promptUndoBytes {
		total -= len(u.stack[drop].text)
		drop++
	}
	if drop > 0 {
		u.stack = append(u.stack[:0], u.stack[drop:]...)
	}
}

// promptEditKindOf classifies the message that changed the text.
//
// The two coalescing kinds are recognized with the same predicates the selection
// uses to decide what replaces a highlight (promptsel.go), read off the editor's
// own keymap — so "this is typing" means here exactly what it means there, and
// every spelling the library binds (ctrl+h for backspace, ctrl+w for the word
// behind) is classified without being listed again.
//
// Everything else — a paste, a menu row, a sort, an indent, a picker's
// insertion, a message that is not a key at all — is editOther, which never
// coalesces. That is the safe default: an operation nobody thought about here
// gets its own undo step rather than being silently folded into the keystrokes
// around it.
func promptEditKindOf(msg tea.Msg, km textarea.KeyMap) (kind promptEditKind, endsRun bool) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return editOther, true
	}
	switch {
	case promptSelDeleteKey(press, km):
		return editDeleting, false
	case key.Matches(press, km.InsertNewline):
		// A newline ends the run for the same reason a word boundary does, only
		// more so: the line just finished is the obvious unit to take back.
		return editTyping, true
	case press.Text != "":
		// A space closes the word it ended, so one press takes back one word
		// rather than everything typed since the last arrow key. The space goes
		// with the word it followed — it was typed as part of finishing it.
		return editTyping, isSpaceText(press.Text)
	}
	return editOther, true
}

func isSpaceText(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return s != ""
}

// --- Taking a step back ---------------------------------------------------------

// undoPrompt is cmd+z, and the ↶ Undo row of the editor's context menu: restore
// the state on top of the history and drop it.
//
// The caret goes back with the text. Restoring 200 characters and leaving the
// cursor where it happened to be would make the next keystroke land somewhere
// nobody chose; the offset stored beside the text is where the hand actually
// was when the undone edit began.
func (m model) undoPrompt() (tea.Model, tea.Cmd) {
	if m.formFocus != formFieldPrompt {
		// Refuse in words. The title is a one-line field the textinput handles on
		// its own, and claiming to undo it here would be a lie about what the
		// stack holds.
		m.formNote = "undo works in the prompt"
		return m, nil
	}
	if len(m.promptUndo.stack) == 0 {
		m.formNote = "nothing to undo — this prompt has not changed since the editor opened"
		return m, nil
	}
	prev := m.promptUndo.stack[len(m.promptUndo.stack)-1]
	m.promptUndo.stack = m.promptUndo.stack[:len(m.promptUndo.stack)-1]
	// The run is over whatever happens next: the state the editor is going back
	// to is not the state the keys on top of the stack were typed into.
	m.promptUndo.kind = editNone
	m.promptUndo.applied = true

	m.promptArea.SetValue(prev.text)
	setPromptCaretOffset(&m.promptArea, prev.caret)
	// A highlight and a column of carets are both aimed at offsets in the text
	// that was just replaced, so both end here — the same rule every other
	// operation that rewrites the value follows.
	m.clearPromptSel()
	m.endPromptCarets()

	m.formNote = "undone"
	if len(m.promptUndo.stack) == 0 {
		m.formNote = "undone · nothing left to take back"
	}
	return m, nil
}

// undoChord names the chord for the footer and the menu row, the way modEnter
// names the save chord: cmd+z where the terminal has answered the keyboard
// enhancement request (and so can carry Cmd at all), ctrl+z where it has not.
// Both stay bound either way; this only decides which one gets the ink.
func (m model) undoChord() string {
	if m.kbEnhanced {
		return "cmd+z"
	}
	return "ctrl+z"
}
