// promptcarets.go — one caret on every swept line, and typing into all of them.
//
// This is the third thing a swept block is worth, beside splitting it and
// sorting it, and it is the one that turns *not yet a list* into a list:
//
//	sweep three plain lines        drop carets at column 0        type "- "
//	  tag v2                         ▌tag v2                        - tag v2
//	  write the notes                ▌write the notes               - write the notes
//	  announce it                    ▌announce it                   - announce it
//
// which is then exactly the shape ✂ Split into prompts wants. Prefixing,
// unprefixing and cutting a column out of a block of lines are what a column
// mode gets used for in every editor that has one, and all three fall out of the
// same small piece of state.
//
// The design, and why it is not "N textareas":
//
// The library has one caret and no notion of a second. Rather than fight that,
// the mode keeps its own carets as a set of logical rows plus a goal column, and
// performs each edit on the value directly — the same road every other
// programmatic edit in this program takes (replacePromptRunes, and see the
// comment there for why walking the library's caret to do it is worse). The
// library's own caret is parked on the first of the rows, so the one the
// textarea draws is one of the ones the user asked for; the rest are painted by
// promptEditorView through the same overlay the selection and the spell marks
// use.
//
// The column is a *goal* column, not a position: a row shorter than it takes its
// caret at its end and keeps it there, and a later move right does not strand
// that row's caret behind the others. It is the rule every editor's ↑/↓ already
// follows, and here it is what lets a block of ragged lines be prefixed in one
// gesture.

package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// promptCarets is the mode's whole state: which logical rows carry a caret, and
// the column all of them aim at.
//
// Rows are stored rather than a range because an edit must not have to re-derive
// which lines were asked for — a selection is gone the moment the mode starts,
// and the rows are the only record of the gesture that opened it.
type promptCarets struct {
	rows []int
	col  int
	on   bool
}

// caretAt is where the caret on row sits given the goal column: the column
// itself, or the row's end when the row is too short to reach it.
func (pc promptCarets) caretAt(row []rune) int { return min(pc.col, len(row)) }

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

	pc := promptCarets{col: max(lo-start, 0), on: true}
	for r := first; r <= last; r++ {
		pc.rows = append(pc.rows, r)
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
// — carets below the fold are still being typed into.
func (m model) caretNote() string {
	return fmt.Sprintf("%d carets, one per swept line", len(m.carets.rows))
}

// syncPromptCaret parks the library's own caret on the first of the mode's rows,
// at the goal column.
//
// The textarea draws exactly one caret and will keep drawing it wherever it
// thinks it is; leaving it behind on the line the sweep ended on would put a
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
	setPromptCaretOffset(&m.promptArea, start+m.carets.caretAt([]rune(rows[first])))
}

// editAtCarets applies one edit to every caret's row and writes the result back.
//
// The rows are edited in a copy of the split value and the whole thing is put
// back with SetValue, rather than each row being patched through
// replacePromptRunes in turn: every edit but the first would then be aimed at
// offsets the edit before it had already moved, and "insert two characters on
// each of six lines" is exactly the shape where that goes wrong quietly.
//
// fn is handed the row's runes and the caret's column in it, and returns the row
// as it should be. A row it declines to change (a backspace at column 0) simply
// comes back as it went in.
func (m *model) editAtCarets(fn func(row []rune, col int) []rune) {
	rows := strings.Split(m.promptArea.Value(), "\n")
	for _, r := range m.carets.rows {
		if r < 0 || r >= len(rows) {
			continue // the value shrank under us; the row is simply not there
		}
		runes := []rune(rows[r])
		rows[r] = string(fn(runes, m.carets.caretAt(runes)))
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
		m.carets.col = max(m.carets.col-1, 0)

	case key.Matches(msg, km.CharacterForward):
		// Unbounded on purpose: the goal column may run past the shorter rows,
		// and caretAt clamps each one as it draws and edits. Bounding it to the
		// shortest row is what would strand the long rows' carets.
		m.carets.col++

	case key.Matches(msg, km.LineStart):
		m.carets.col = 0

	case key.Matches(msg, km.LineEnd):
		// The goal column goes to the longest of the rows, so every shorter one
		// clamps to its own end (caretAt) — which is "the end of each line", the
		// counterpart of ctrl+a and the half of the mode that appends to a block
		// rather than prefixing it.
		rows := strings.Split(m.promptArea.Value(), "\n")
		m.carets.col = 0
		for _, r := range m.carets.rows {
			if r >= 0 && r < len(rows) {
				m.carets.col = max(m.carets.col, len([]rune(rows[r])))
			}
		}

	case key.Matches(msg, km.DeleteCharacterBackward):
		if m.carets.col == 0 {
			// Every caret is already at the start of its line; there is nothing
			// behind them to take out, and joining every row onto the one above
			// is not what a backspace in this mode can sensibly mean.
			m.formNote = "the carets are at the start of their lines"
			return m, nil, true
		}
		m.editAtCarets(func(row []rune, col int) []rune {
			if col == 0 {
				return row
			}
			return append(append([]rune{}, row[:col-1]...), row[col:]...)
		})
		m.carets.col--

	case key.Matches(msg, km.DeleteCharacterForward):
		m.editAtCarets(func(row []rune, col int) []rune {
			if col >= len(row) {
				return row
			}
			return append(append([]rune{}, row[:col]...), row[col+1:]...)
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
	m.syncPromptCaret()
	if m.formNote == "" || strings.Contains(m.formNote, "carets") {
		m.formNote = m.caretNote()
	}
	return m, nil, true
}

// insertAtCarets puts text in at every caret and steps the goal column past it.
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
	m.editAtCarets(func(row []rune, col int) []rune {
		out := append([]rune{}, row[:col]...)
		out = append(out, []rune(text)...)
		return append(out, row[col:]...)
	})
	m.carets.col += len([]rune(text))
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
		off := start + m.carets.caretAt([]rune(rows[r]))
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
