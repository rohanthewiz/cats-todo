// promptcarets.go — a caret on several lines at once, and typing into all of them.
//
// Two roads lead in. The first is ⌶ Caret on every line: sweep a block, and a
// caret goes down on each of its lines in the column the sweep began in. It is
// the third thing a swept block is worth, beside splitting it and sorting it,
// and it is the one that turns *not yet a list* into a list:
//
//	sweep three plain lines        drop carets at column 0        type "- "
//	  tag v2                         ▌tag v2                        - tag v2
//	  write the notes                ▌write the notes               - write the notes
//	  announce it                    ▌announce it                   - announce it
//
// which is then exactly the shape ✂ Split into prompts wants. Prefixing,
// unprefixing and cutting a column out of a block of lines are what a column
// mode gets used for in every editor that has one.
//
// The second road is the pointer's: alt+click puts a caret where the press
// landed, beside the one the editor already has. More presses add more; a press
// exactly on a standing caret takes that caret away. That is the gesture every
// multi-cursor editor answers to, and it reaches what the sweep cannot — lines
// that are not neighbours, columns that are not equal.
//
// The design, and why it is not "N textareas":
//
// The library has one caret and no notion of a second. Rather than fight that,
// the mode keeps its own carets as a set of logical rows with a goal column
// each, and performs each edit on the value directly — the same road every
// other programmatic edit in this program takes (replacePromptRunes, and see
// the comment there for why walking the library's caret to do it is worse).
// The library's own caret is parked on the first of the rows, so the one the
// textarea draws is one of the ones the user asked for; the rest are painted by
// promptEditorView through the same overlay the selection and the spell marks
// use.
//
// Each column is a *goal* column, not a position: a row shorter than its goal
// takes its caret at its end and keeps it there, and a later move right does
// not strand that row's caret behind the others. It is the rule every editor's
// ↑/↓ already follows, and here it is what lets a block of ragged lines be
// prefixed in one gesture. The sweep starts every goal in the same column;
// alt+click starts each at the cell it was aimed at.
//
// SEVERAL CARETS MAY SHARE A ROW. The mode began with a row carrying at most
// one, which reads as a reasonable rule until you notice what a prompt actually
// looks like: one long paragraph, soft-wrapped across half the box. Every
// display line the hand aims at belongs to the same *logical* row, so the rule
// turned the pointer gesture into a no-op on the commonest shape there is —
// while it worked perfectly on a list of short lines, which is why it survived
// as long as it did. A caret is a position, so the set is keyed by (row,
// column) and nothing else, the way ced's is (internal/editor/multicaret.go).
//
// That is what makes the edit order load-bearing. Carets are held sorted by
// row, then column, and every edit walks them BACKWARDS:
//
//	row:  "alpha bravo"     carets at 5 and 11, typing "!"
//	      ────────┬──┬──
//	              5  11
//	  ← 11 first: "alpha bravo!"   (5 still means what it meant)
//	  ← then 5:   "alpha! bravo!"  (11 shifted right by the one insert before it)
//
// Going forwards instead would aim every caret after the first at offsets the
// edit before it had already moved. It is the same bottom-up rule ced's
// applyAtCarets follows, and the same one editAtCarets already relied on across
// rows — sharing a row is only what makes it visible.

package main

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// promptCarets is the mode's whole state: which logical rows carry a caret, and
// the column each of them aims at. rows and cols are parallel, and rows stays
// sorted — the order carets go down in is not information, but "the first row"
// is (syncPromptCaret parks the library's caret there, and topmost is the one
// the eye expects).
//
// Rows are stored rather than a range because an edit must not have to
// re-derive which lines were asked for — a selection is gone the moment the
// mode starts, and with alt+click in play the rows need not even be contiguous.
//
// rows may repeat: a row carries as many carets as were put on it. The pair is
// kept sorted by row, then by goal column, which is the order every edit walks
// backwards (see the file comment) and the order the overlay paints in.
type promptCarets struct {
	rows []int
	cols []int
	on   bool
}

// caretAt is where caret i sits on its row given its goal column: the column
// itself, or the row's end when the row is too short to reach it.
func (pc promptCarets) caretAt(i int, row []rune) int { return min(pc.cols[i], len(row)) }

// indexAt is the caret standing exactly at (row, col), or -1.
//
// It compares the *effective* column — what caretAt draws, clamped into the row
// — because that is the cell the eye sees and the only one a press can land on.
// A caret whose goal ran off the end of a short row is reachable at that row's
// end, which is where it is drawn.
func (pc promptCarets) indexAt(row, col int, runes []rune) int {
	for i, r := range pc.rows {
		if r == row && pc.caretAt(i, runes) == col {
			return i
		}
	}
	return -1
}

// add puts a caret on row r aiming at column c, keeping the pair sorted by row
// then column.
//
// The scan is linear rather than a binary search because the sort key is now a
// pair spread across two parallel slices, and a caret set is a handful of
// entries — the shape of the answer matters more here than the shape of the
// search.
func (pc *promptCarets) add(r, c int) {
	i := 0
	for i < len(pc.rows) && (pc.rows[i] < r || (pc.rows[i] == r && pc.cols[i] < c)) {
		i++
	}
	pc.rows = slices.Insert(pc.rows, i, r)
	pc.cols = slices.Insert(pc.cols, i, c)
}

// remove takes caret i away.
func (pc *promptCarets) remove(i int) {
	pc.rows = slices.Delete(pc.rows, i, i+1)
	pc.cols = slices.Delete(pc.cols, i, i+1)
}

// dedupe folds carets that have landed on the same place into one.
//
// The horizontal motions are what make this necessary: ctrl+a sends every goal
// on a row to 0, so two carets that shared that row are now one caret written
// twice — and a set that holds it twice would type every character twice on
// that line. Two carets in one place *are* one caret, so the duplicate goes.
//
// Walking backwards keeps the surviving index stable as entries are dropped,
// and the neighbours-only comparison is enough because the pair is sorted.
func (pc *promptCarets) dedupe() {
	for i := len(pc.rows) - 1; i > 0; i-- {
		if pc.rows[i] == pc.rows[i-1] && pc.cols[i] == pc.cols[i-1] {
			pc.remove(i)
		}
	}
}

// dropPromptCarets is ⌶ Caret on every line: put one caret on each swept row and
// enter the mode.
//
// The column every caret lands in is the column the *sweep began* in, which is
// column 0 for the sweep this feature is for — a drag down the left margin, or a
// shift+↓ run from the start of a line. That is what makes "type `- `" prefix
// the block. Taking it from the sweep's start rather than from the pointer's
// last position is what keeps the gesture predictable: the hand chose where to
// begin, and the release landed wherever the block ended.
func (m model) dropPromptCarets() (tea.Model, tea.Cmd) {
	if m.formFocus != formFieldPrompt {
		m.formNote = "carets go on the prompt's lines"
		return m, nil
	}
	lo, _, ok := m.promptSelSpan()
	if !ok {
		m.formNote = "nothing selected — sweep the lines to put a caret on"
		return m, nil
	}
	first, last, _, _, _ := m.promptSelRows()
	if first == last {
		// One line already has a caret: the editor's own. Say so rather than
		// entering a mode that would behave exactly like not being in it.
		m.formNote = "sweep two or more lines to put a caret on each"
		return m, nil
	}
	rows := strings.Split(m.promptArea.Value(), "\n")
	start, _ := promptRowSpan(rows, first, first)

	pc := promptCarets{on: true}
	col := max(lo-start, 0)
	for r := first; r <= last; r++ {
		pc.add(r, col)
	}
	// The selection goes: the mode replaces it, and a highlight left standing
	// over rows that are about to be edited from several places at once would be
	// a lie about what the next keystroke does. clearPromptSel would take the
	// carets with it (see the note there), so the field is set afterwards.
	m.clearPromptSel()
	m.carets = pc
	m.syncPromptCaret()
	m.formNote = m.caretNote()
	return m, nil
}

// altClickPrompt is the pointer's road into the mode: alt held on a left press
// inside the editor's box. The first press puts a second caret beside the
// editor's own; each press after that adds one, or — when the press lands
// exactly on a standing caret — takes that caret away. Down to one caret, the
// mode ends: one caret is what the editor is when the mode is off.
//
// The only press that does not add a caret is one landing on a caret that is
// already there, and "there" is a cell, not a line: two presses on the same
// wrapped paragraph put two carets on it, which is the whole point of a
// pointer gesture and what a row-keyed set could not express.
//
// x, row are pane cell and editor-box row, the coordinates clickForm hands out.
func (m model) altClickPrompt(x, row int) (tea.Model, tea.Cmd) {
	// The keys follow the pointer, the same rule every other click on this form
	// obeys — what gets typed next lands at the carets, so the prompt is where
	// the focus must be.
	cmd := m.focusForm(formFieldPrompt)
	r, c, ok := promptRowColAt(m.promptArea, x, row)
	if !ok {
		m.formNote = "no line there to put a caret on"
		return m, cmd
	}
	// A press that names carets still un-names the selection, for the reason
	// every other press does (see clickForm): the highlight would misreport what
	// ctrl+c copies once the mode starts fielding the keys.
	m.clearPromptSel()
	rows := strings.Split(m.promptArea.Value(), "\n")
	if r >= len(rows) {
		return m, cmd // the display table and the value disagree; do nothing
	}
	c = min(c, len([]rune(rows[r])))
	switch {
	case !m.carets.on:
		cr := min(max(m.promptArea.Line(), 0), len(rows)-1)
		cc := min(max(m.promptArea.Column(), 0), len([]rune(rows[cr])))
		if cr == r && cc == c {
			// The press landed on the editor's own caret, the one cell on the
			// screen that already has one. There is no second caret to put
			// here, so this is a plain press — and it says so rather than
			// doing nothing in silence, because "nothing happened" is also
			// what a terminal that ate the modifier looks like. The note
			// appearing is the proof that alt arrived.
			m.placePromptCursor(x, row)
			m.formNote = "the caret is already there — alt+click another cell"
			return m, cmd
		}
		m.carets = promptCarets{on: true}
		m.carets.add(cr, cc)
		m.carets.add(r, c)
	default:
		if i := m.carets.indexAt(r, c, []rune(rows[r])); i >= 0 {
			// The press landed on a caret itself: the ask is to take it away.
			m.carets.remove(i)
			if len(m.carets.rows) == 1 {
				// Park the library's caret on the survivor before the mode's
				// state goes, so what remains on screen is what remains.
				m.syncPromptCaret()
				m.endPromptCarets()
				m.formNote = "one caret again"
				return m, cmd
			}
		} else {
			m.carets.add(r, c)
		}
	}
	m.syncPromptCaret()
	m.formNote = m.caretNote()
	return m, cmd
}

// endPromptCarets leaves the mode. It is deliberately not "cancel": everything
// typed while it was on is already in the value, exactly as if it had been typed
// once per line by hand.
func (m *model) endPromptCarets() {
	if !m.carets.on {
		return
	}
	m.carets = promptCarets{}
	m.formNote = ""
}

// caretNote is the mode's standing message. It names the count because that is
// the one thing about the mode that is not visible at a glance on a tall prompt
// — carets below the fold are still being typed into — and it teaches the
// pointer gesture, which no key on the footer can stand for.
func (m model) caretNote() string {
	return fmt.Sprintf("%d carets · alt+click adds or removes one", len(m.carets.rows))
}

// syncPromptCaret parks the library's own caret on the first of the mode's rows,
// at that caret's goal column.
//
// The textarea draws exactly one caret and will keep drawing it wherever it
// thinks it is; leaving it behind on the line the gesture ended on would put a
// second kind of caret on screen that none of the keys move. Parking it on one
// of ours means the library's caret *is* one of the mode's, and the overlay only
// has to draw the rest.
func (m *model) syncPromptCaret() {
	if !m.carets.on || len(m.carets.rows) == 0 {
		return
	}
	rows := strings.Split(m.promptArea.Value(), "\n")
	first := m.carets.rows[0]
	if first >= len(rows) {
		return
	}
	start, _ := promptRowSpan(rows, first, first)
	setPromptCaretOffset(&m.promptArea, start+m.carets.caretAt(0, []rune(rows[first])))
}

// editAtCarets applies one edit at every caret and writes the result back,
// carrying the goal columns across the edit.
//
// The rows are edited in a copy of the split value and the whole thing is put
// back with SetValue, rather than each row being patched through
// replacePromptRunes in turn: every edit but the first would then be aimed at
// offsets the edit before it had already moved, and "insert two characters on
// each of six lines" is exactly the shape where that goes wrong quietly.
//
// fn is handed the row's runes and the caret's effective column in it, and
// returns the row as it should be together with how far that caret's own goal
// moved — +2 for a two-rune insert, -1 for a backspace that bit, 0 for an edit
// it declined (a backspace at column 0). A row fn leaves alone simply comes
// back as it went in.
//
// TWO CARETS ON ONE ROW is what the walk order and the second adjustment are
// for. Carets are visited in descending order, so each edit lands after every
// caret still waiting its turn and none of their columns go stale. The carets
// ALREADY visited on that row sit to the right of the edit, though, and their
// columns were measured against the row as it was — so each one shifts by the
// row's net change in length. Both adjustments are additive and disjoint (a
// caret takes its own goalDelta once, plus one shift per edit to its left), so
// the order they are applied in does not matter.
//
//	"alpha bravo", carets at 5 and 11, inserting "!"
//	  i=1 (col 11): row → "alpha bravo!", goal 11 → 12
//	  i=0 (col  5): row → "alpha! bravo!", goal 5 → 6, and caret 1 shifts +1 → 13
//	                                                    ^ one insert now precedes it
func (m *model) editAtCarets(fn func(row []rune, col int) ([]rune, int)) {
	rows := strings.Split(m.promptArea.Value(), "\n")
	for i := len(m.carets.rows) - 1; i >= 0; i-- {
		r := m.carets.rows[i]
		if r < 0 || r >= len(rows) {
			continue // the value shrank under us; the row is simply not there
		}
		runes := []rune(rows[r])
		out, goalDelta := fn(runes, m.carets.caretAt(i, runes))
		rows[r] = string(out)
		m.carets.cols[i] += goalDelta
		// Carry the carets further along this row over the change. Sorted by
		// (row, column) means they are exactly the entries after i that still
		// name row r, so the scan stops at the first that does not.
		if d := len(out) - len(runes); d != 0 {
			for j := i + 1; j < len(m.carets.rows) && m.carets.rows[j] == r; j++ {
				m.carets.cols[j] += d
			}
		}
	}
	for i := range m.carets.cols {
		m.carets.cols[i] = max(m.carets.cols[i], 0)
	}
	m.promptArea.SetValue(strings.Join(rows, "\n"))
}

// updatePromptCarets is the mode's key handling. handled false means the key is
// not one of the mode's, and the caller ends the mode and lets the key take its
// ordinary path — so ctrl+s still saves and ctrl+o still opens the attachments,
// from inside the mode, without either being listed here.
//
// What the mode does own is the set of keys that would otherwise act on one
// caret: typing, the newline, the two deletes, and the two horizontal motions. Everything
// vertical is left out on purpose — ↑ and ↓ mean "move the caret to another
// line", which is the one thing a caret per line has already been asked not to
// do, so they end the mode instead.
func (m model) updatePromptCarets(msg tea.KeyPressMsg, carry promptCarry) (tea.Model, tea.Cmd, bool) {
	km := m.promptArea.KeyMap
	switch {
	case msg.String() == "esc":
		m.endPromptCarets()
		return m, nil, true

	case key.Matches(msg, km.CharacterBackward):
		for i := range m.carets.cols {
			m.carets.cols[i] = max(m.carets.cols[i]-1, 0)
		}

	case key.Matches(msg, km.CharacterForward):
		// Unbounded on purpose: a goal column may run past its row's end, and
		// caretAt clamps as it draws and edits. Bounding a goal to its row is
		// what would strand that caret when the row grows back.
		for i := range m.carets.cols {
			m.carets.cols[i]++
		}

	case key.Matches(msg, km.LineStart):
		for i := range m.carets.cols {
			m.carets.cols[i] = 0
		}

	case key.Matches(msg, km.LineEnd):
		// Every caret to the end of its own row — the counterpart of ctrl+a and
		// the half of the mode that appends to a block rather than prefixing it.
		rows := strings.Split(m.promptArea.Value(), "\n")
		for i, r := range m.carets.rows {
			if r >= 0 && r < len(rows) {
				m.carets.cols[i] = len([]rune(rows[r]))
			}
		}

	case key.Matches(msg, km.DeleteCharacterBackward):
		if m.takeBackCarriedIndentAtCarets(carry) {
			// The enter just before carried indents. One press takes them all
			// back, leaving every caret at its line start (promptindent.go).
			break
		}
		if m.caretsAllAtLineStart() {
			// Every caret is already at the start of its line; there is nothing
			// behind them to take out, and joining every row onto the one above
			// is not what a backspace in this mode can sensibly mean.
			m.formNote = "the carets are at the start of their lines"
			return m, nil, true
		}
		m.editAtCarets(func(row []rune, col int) ([]rune, int) {
			if col == 0 {
				return row, 0 // nothing behind this one; its goal does not move
			}
			return append(append([]rune{}, row[:col-1]...), row[col:]...), -1
		})

	case key.Matches(msg, km.DeleteCharacterForward):
		// The goal does not move: the character taken out was ahead of the
		// caret, so the caret is still where it was.
		m.editAtCarets(func(row []rune, col int) ([]rune, int) {
			if col >= len(row) {
				return row, 0
			}
			return append(append([]rune{}, row[:col]...), row[col+1:]...), 0
		})

	case key.Matches(msg, km.InsertNewline):
		// A newline goes in at every caret, the way every multi-cursor editor
		// does it. This used to end the mode on the theory that enter is pressed
		// because the user thinks the mode is already over. In practice the
		// opposite happened: alt+click several places and press enter to break
		// each one, and nothing broke. esc is how the mode ends.
		m.newlineAtCarets()

	case promptIndentDir(msg) > 0:
		// Four spaces at every caret: the indent the ordinary editor types at
		// its one caret (promptindent.go), typed at all of them.
		m.insertAtCarets(promptIndentUnit)

	case promptIndentDir(msg) < 0:
		if !m.outdentAtCarets() {
			m.formNote = "nothing to outdent — no caret's line starts with a space"
			return m, nil, true
		}

	case promptPasteChord(msg.String()):
		// Cmd+V from a host that sends the chord rather than pasting for us.
		// Without this case the chord would fall to default, end the mode, and
		// paste at one caret. pasteFormClipboard reads the clipboard and sends
		// the text back through pasteIntoForm, which sees the carets are still
		// up. Its result is returned directly because it may also return a Cmd
		// (the OSC 52 read) that the code below would drop.
		return m.pasteFormClipboardInMode()

	case msg.Text != "":
		// The same test the textarea itself applies to decide a key inserts —
		// see promptSelInsertKey, which leans on it for the same reason.
		m.insertAtCarets(msg.Text)

	default:
		return m, nil, false
	}
	if !m.carets.on {
		// spliceAtCarets may already have merged the set down to one caret and
		// ended the mode (a newline at two goals that clamp to one cell). There
		// is nothing left to fold, and a note naming the count would say
		// "0 carets" on a screen that has none.
		return m, nil, true
	}
	// Every branch above may have driven two carets that shared a row onto the
	// same cell — ctrl+a is the plain case, and ← does it to neighbours at the
	// left margin. Folding them here rather than in each branch is what keeps
	// "a place holds one caret" true for the key that lands next, whichever key
	// that is.
	m.carets.dedupe()
	if len(m.carets.rows) == 1 {
		// The fold left one caret, which is what the editor is with the mode
		// off. Park the library's caret on the survivor before the state goes.
		m.syncPromptCaret()
		m.endPromptCarets()
		m.formNote = "one caret again"
		return m, nil, true
	}
	m.syncPromptCaret()
	if m.formNote == "" || strings.Contains(m.formNote, "carets") {
		m.formNote = m.caretNote()
	}
	return m, nil, true
}

// outdentAtCarets is shift+tab in the column mode: up to one indent unit of
// leading spaces comes off every line a caret is on. It reports whether any
// line changed.
//
// It works per ROW, not per caret. That is why it does not go through
// editAtCarets, which calls its edit once per caret: two carets on one row
// would outdent that row twice. The carets sharing a row are the next run in
// the sorted list, so the walk takes each run together, trims the row once,
// and moves every caret in the run left by what was removed. A caret inside the
// removed spaces lands at the line start, and the dedupe in updatePromptCarets
// then merges any that meet there.
func (m *model) outdentAtCarets() bool {
	rows := strings.Split(m.promptArea.Value(), "\n")
	changed := false
	for i := 0; i < len(m.carets.rows); {
		r := m.carets.rows[i]
		j := i + 1
		for j < len(m.carets.rows) && m.carets.rows[j] == r {
			j++
		}
		if r >= 0 && r < len(rows) {
			if k := promptOutdentWidth(rows[r]); k > 0 {
				rows[r] = rows[r][k:]
				for x := i; x < j; x++ {
					m.carets.cols[x] = max(m.carets.cols[x]-k, 0)
				}
				changed = true
			}
		}
		i = j
	}
	if changed {
		m.promptArea.SetValue(strings.Join(rows, "\n"))
	}
	return changed
}

// pasteFormClipboardInMode adapts pasteFormClipboard to updatePromptCarets'
// three-value return. handled is always true: the chord belongs to the mode
// even when the clipboard turns out to be empty, and that refusal is worded in
// the form note, not handed on to end the mode.
func (m model) pasteFormClipboardInMode() (tea.Model, tea.Cmd, bool) {
	next, cmd := m.pasteFormClipboard()
	return next, cmd, true
}

// caretsAllAtLineStart answers whether every caret sits in column 0 of its row —
// the effective column, not the goal, so a goal stranded past an empty row still
// counts as "nothing behind it".
func (m model) caretsAllAtLineStart() bool {
	rows := strings.Split(m.promptArea.Value(), "\n")
	for i, r := range m.carets.rows {
		if r >= 0 && r < len(rows) && m.carets.caretAt(i, []rune(rows[r])) > 0 {
			return false
		}
	}
	return true
}

// insertAtCarets puts text in at every caret and steps each goal column past it.
// It is its own method because a paste arrives here too, by a different road
// (see the tea.PasteMsg case in Update).
func (m *model) insertAtCarets(text string) {
	// Terminals and pasteboards disagree on line endings: a Windows-sourced
	// copy brings \r\n, and an old-Mac one a bare \r. The value only ever
	// holds \n, so both are folded before anything counts lines.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if text == "" {
		return
	}
	if strings.Contains(text, "\n") {
		m.pasteLinesAtCarets(text)
		return
	}
	// Stepping the goal rather than the effective column is exact: the insert
	// itself landed at min(goal, len), and both ends of that min grew by the
	// same amount — which is why the step is handed back as a goal delta.
	step := len([]rune(text))
	m.editAtCarets(func(row []rune, col int) ([]rune, int) {
		out := append([]rune{}, row[:col]...)
		out = append(out, []rune(text)...)
		return append(out, row[col:]...), step
	})
	m.syncPromptCaret()
}

// newlineAtCarets breaks the value at every caret, and each new line takes the
// indent its caret's line carries (promptCarriedIndent). It is the ordinary
// editor's enter, done at every caret. Each caret lands just past its indent, so
// typing right after enter prefixes every new line together, each at its own
// level. It is a splice of "\n" plus the indent at every caret; see
// spliceAtCarets for how an insert that adds rows is kept straight.
//
// The indents are measured on the rows as they stand, before any break. Two
// carets on one row therefore carry the same indent, the row's, rather than the
// second measuring the fragment the first break made.
//
// A row that is only spaces with its caret at the end moves its indent down
// instead of copying it, as in the ordinary editor. That happens only when the
// caret is alone on its row, because emptying the row would pull the column out
// from under a neighbour. The carets are folded to cells first, so the neighbour
// test is the caret before and after in the sorted list. The splice's own fold
// then finds nothing to merge, which keeps widths indexed like the carets.
func (m *model) newlineAtCarets() {
	rows := strings.Split(m.promptArea.Value(), "\n")
	m.foldCaretsToCells(rows)
	widths := make([]int, len(m.carets.rows))
	emptied, carried := false, false
	for i, r := range m.carets.rows {
		if r < 0 || r >= len(rows) {
			continue // spliceAtCarets drops it too
		}
		n, blank := promptCarriedIndent([]rune(rows[r]), m.carets.cols[i])
		widths[i] = n
		carried = carried || n > 0
		alone := (i == 0 || m.carets.rows[i-1] != r) &&
			(i == len(m.carets.rows)-1 || m.carets.rows[i+1] != r)
		if blank && alone {
			rows[r], m.carets.cols[i] = "", 0
			emptied = true
		}
	}
	if emptied {
		m.promptArea.SetValue(strings.Join(rows, "\n"))
	}
	m.spliceAtCarets(func(i int) string { return "\n" + strings.Repeat(" ", widths[i]) })
	if !carried || !m.carets.on {
		// Nothing was carried, or the fold left one caret and the splice ended
		// the mode. That lone caret's carry is not recorded: it is the rare case,
		// and a backspace there deletes one space as it always did.
		return
	}
	m.promptCarry = promptCarry{
		value:  m.promptArea.Value(),
		widths: widths,
		rows:   slices.Clone(m.carets.rows),
		cols:   slices.Clone(m.carets.cols),
	}
}

// takeBackCarriedIndentAtCarets is the column mode's backspace straight after an
// enter that carried indents: every caret's carried indent goes in one press. It
// reports false, and does nothing, unless the value and every caret still stand
// exactly where that enter left them.
//
// Each break put its caret on a line of its own, so the carry's rows are distinct
// and each row loses only its own caret's indent. A caret whose line had no
// indent carried nothing and stays where it is.
func (m *model) takeBackCarriedIndentAtCarets(carry promptCarry) bool {
	if carry.rows == nil || m.promptArea.Value() != carry.value ||
		!slices.Equal(m.carets.rows, carry.rows) || !slices.Equal(m.carets.cols, carry.cols) {
		return false
	}
	rows := strings.Split(carry.value, "\n")
	for i, r := range m.carets.rows {
		if w := carry.widths[i]; w > 0 && r >= 0 && r < len(rows) {
			rows[r] = rows[r][w:] // the value is unchanged, so the row starts with w spaces
			m.carets.cols[i] -= w
		}
	}
	m.promptArea.SetValue(strings.Join(rows, "\n"))
	return true
}

// pasteLinesAtCarets is a paste that carries newlines, arriving while the mode
// is on. It follows the rule multi-cursor editors have settled on (VS Code's
// default "spread", and Sublime's):
//
//	lines in the paste == carets  → one line per caret, top to bottom
//	anything else                 → the whole paste at every caret
//
// The first case is what makes a column of values useful. Copy three names from
// somewhere else, alt+click three places, paste, and each place gets its own
// name. The second case is the general one. Pasting a two-line snippet at three
// carets means that snippet three times, newlines included, the same as typing
// it at each caret would.
//
// A single trailing newline is ignored when counting, because copying whole
// lines almost always brings the last line's break with it. Three copied lines
// should count as three, not as three plus an empty fourth. It is dropped only
// when the counts then match. In the whole-paste case the text goes in exactly
// as it was copied.
//
// The count is taken after carets that share a cell are merged, since what the
// user sees and counts is the carets drawn on screen, not the goal columns
// behind them.
func (m *model) pasteLinesAtCarets(text string) {
	rows := strings.Split(m.promptArea.Value(), "\n")
	m.foldCaretsToCells(rows)

	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(lines) == len(m.carets.rows) {
		m.spliceAtCarets(func(i int) string { return lines[i] })
		return
	}
	m.spliceAtCarets(func(int) string { return text })
}

// foldCaretsToCells sets every goal column to its effective one and merges
// carets that end up in the same cell.
//
// A row can hold several goal columns that all clamp to its end (caretAt).
// Splicing at that one cell twice would add the text twice, or, for a newline,
// an empty line nobody asked for, leaving the two carets on different rows.
// Typing never exposed this, because editAtCarets keeps the goals, but an insert
// that adds rows does. Folding to the effective column loses nothing here: after
// a splice every caret sits right after its insert, so a goal past the old end
// of a row no longer means anything.
func (m *model) foldCaretsToCells(rows []string) {
	for i, r := range m.carets.rows {
		if r >= 0 && r < len(rows) {
			m.carets.cols[i] = m.carets.caretAt(i, []rune(rows[r]))
		}
	}
	m.carets.dedupe()
}

// spliceAtCarets inserts textFor(i) at caret i, for every caret, where the text
// may contain newlines. Each caret ends up right after its own insert.
//
// editAtCarets cannot do this because it edits a row in place and the row count
// stays the same. An insert with a newline adds rows, which moves every row
// below it, including rows the walk has not reached yet. So instead of patching
// the old value, this builds a new one top to bottom. cur is the output line
// being built. Each caret adds its row's text up to its column, then its
// insert. Every newline in the insert closes cur and starts a new line. The
// caret's new position is wherever cur has got to: the row is the number of
// lines closed so far, and the column is cur's length. Nothing has to be
// shifted afterwards, because nothing was ever measured against the old layout.
//
//	rows "alpha bravo" / "charlie", carets (0,5) (0,11) (1,7), inserting "\n"
//
//	  row 0: "alpha" + "\n"   → closes "alpha",   caret → (1,0)
//	         " bravo" + "\n"  → closes " bravo",  caret → (2,0)
//	         rest ""          → closes ""
//	  row 1: "charlie" + "\n" → closes "charlie", caret → (4,0)
//	         rest ""          → closes ""
//
//	  value "alpha\n bravo\n\ncharlie\n"
//
// The same walk with no newline in the insert is an ordinary same-row insert.
// That is how a spread paste of short lines, with two carets on one row, puts
// the second caret after the first caret's insert.
func (m *model) spliceAtCarets(textFor func(i int) string) {
	rows := strings.Split(m.promptArea.Value(), "\n")
	m.foldCaretsToCells(rows)

	out := make([]string, 0, len(rows)+len(m.carets.rows))
	next := promptCarets{on: true}
	i := 0
	// Carets on rows before 0 cannot exist, but skip them anyway. Otherwise one
	// would stop the cursor below and every caret after it would be lost.
	for i < len(m.carets.rows) && m.carets.rows[i] < 0 {
		i++
	}
	var cur strings.Builder
	for r, line := range rows {
		runes := []rune(line)
		from := 0
		// The carets are sorted by row, then column, so this row's carets are
		// the next run in the list, left to right. Their columns only increase,
		// which keeps each cut after the one before it.
		for ; i < len(m.carets.rows) && m.carets.rows[i] == r; i++ {
			c := min(max(m.carets.cols[i], from), len(runes))
			cur.WriteString(string(runes[from:c]))
			from = c
			// The first part continues the current line, and every part after a
			// newline closes it and starts a new one.
			for j, part := range strings.Split(textFor(i), "\n") {
				if j > 0 {
					out = append(out, cur.String())
					cur.Reset()
				}
				cur.WriteString(part)
			}
			next.rows = append(next.rows, len(out))
			next.cols = append(next.cols, len([]rune(cur.String())))
		}
		cur.WriteString(string(runes[from:]))
		out = append(out, cur.String())
		cur.Reset()
	}
	// A caret on a row past the end of the value is dropped. It is the same case
	// editAtCarets skips: the value got shorter while the caret was still there.
	m.carets = next
	m.promptArea.SetValue(strings.Join(out, "\n"))
	if len(m.carets.rows) == 1 {
		// Merging left one caret, which is the editor with the mode off. The
		// key path checks this itself, but a paste arrives from Update and does
		// not, so the check is made here where both roads meet.
		m.syncPromptCaret()
		m.endPromptCarets()
		return
	}
	m.syncPromptCaret()
}

// promptCaretPaints is the extra carets as cell runs on one display line, for
// promptEditorView's overlay: a one-cell reversed block wherever a caret of the
// mode falls on this line.
//
// The library's own caret is skipped — it is already drawn, on the first of the
// mode's rows (syncPromptCaret) — so what this adds is the second caret onward.
// Offsets are turned into cells by summing widths, the same care the selection
// and the spell underline take with double-width glyphs.
func (m model) promptCaretPaints(dl promptDisplayLine, runes []rune, gutter int) []promptPaint {
	if !m.carets.on {
		return nil
	}
	var out []promptPaint
	rows := strings.Split(m.promptArea.Value(), "\n")
	for i, r := range m.carets.rows {
		if i == 0 || r != dl.row || r >= len(rows) {
			continue
		}
		start, _ := promptRowSpan(rows, r, r)
		off := start + m.carets.caretAt(i, []rune(rows[r]))
		if off < dl.start || off > dl.end() || off > len(runes) {
			continue // on another wrap segment of this row
		}
		cell := gutter + lipgloss.Width(string(runes[dl.start:off]))
		out = append(out, promptPaint{a: cell, b: cell + 1, style: promptCaretStyle})
	}
	return out
}

// mergeCaretPaints folds the mode's carets into the runs already on a line: each
// caret's single cell is cut out of any run it lands inside, then the carets go
// in and the whole set is sorted.
//
// The carets win the overlap because they are the smaller and the more urgent
// mark — a cell is either where the next character goes or it is not, and a
// spell underline that swallowed a caret would hide the one thing the user needs
// to see before typing. paintPromptSpans requires exactly this: sorted runs that
// do not overlap.
func mergeCaretPaints(paints, carets []promptPaint) []promptPaint {
	out := make([]promptPaint, 0, len(paints)+len(carets))
	for _, p := range paints {
		// Split p around every caret cell inside it, left to right. head walks
		// forward as each caret takes its cell out of the run.
		head := p.a
		for _, c := range carets {
			if c.a < head || c.a >= p.b {
				continue
			}
			if c.a > head {
				out = append(out, promptPaint{a: head, b: c.a, style: p.style, caret: p.caret})
			}
			head = c.b
		}
		if head < p.b {
			out = append(out, promptPaint{a: head, b: p.b, style: p.style, caret: p.caret})
		}
	}
	out = append(out, carets...)
	sortPromptPaints(out)
	return out
}
