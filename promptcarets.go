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
// caret: typing, the two deletes, and the two horizontal motions. Everything
// vertical is left out on purpose — ↑ and ↓ mean "move the caret to another
// line", which is the one thing a caret per line has already been asked not to
// do, so they end the mode instead.
func (m model) updatePromptCarets(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
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
		// A newline would turn N lines into 2N and leave the carets with nothing
		// coherent to be on. It ends the mode instead, and does not also insert:
		// enter is the most likely key to be pressed *because* the user thinks
		// the mode is already over.
		m.endPromptCarets()
		return m, nil, true

	case msg.Text != "":
		// The same test the textarea itself applies to decide a key inserts —
		// see promptSelInsertKey, which leans on it for the same reason.
		m.insertAtCarets(msg.Text)

	default:
		return m, nil, false
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
	// A pasted newline would split the rows the carets are counted against, so
	// only the paste's first line goes in — the rest would land somewhere no
	// caret was asked to be.
	text, _, _ = strings.Cut(text, "\n")
	if text == "" {
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
