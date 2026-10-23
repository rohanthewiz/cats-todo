// softwrap.go — the ☑ Soft wrap toggle on the prompt editor's toolbar and on
// the Next List page's bar: the editor's unwrapped mode (a long line stays on
// one row and the box scrolls sideways after the caret), the chip both bars
// share, and its place at the far right of each row.
//
// The Next List's half of the work is mostly in fuzzylist.go (rows that carry
// continuation lines, and a scroll window counted in lines) and nextlist.go
// (cutting an item's text into those lines); this file holds the toggle.
package main

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// noWrapWidth is the text width the editor's textarea is given while soft wrap
// is off: wide enough that no line a person types reaches it, so the library's
// own wrap — which cannot be switched off — never fires in practice.
//
// Why widen the textarea rather than teach it not to wrap: the wrap is not an
// option of bubbles' textarea but the core of it. Its caret motion, its
// LineInfo, its scrolling and every display-line table this program builds on
// top of them (promptLines, promptDisplayLines — and through those the
// selection, the carets, the spell marks, the code paints and every click)
// are all phrased in display lines. A box so wide that each logical line is
// exactly one display line keeps all of that true unchanged, and the only new
// work is showing a window of it: promptEditorView crops each drawn row to the
// pane, and promptTextX moves a click by the same amount the other way.
//
// The price is that a line longer than this still wraps, at 2048 cells, and
// that the library pads every drawn row out to this width before the crop
// throws the padding away. The padding is a few kilobytes per frame for a
// screenful of rows, which is nothing next to a terminal redraw.
const noWrapWidth = 2048

// promptMaxWidth is bubbles' textarea default MaxWidth (its unexported
// defaultMaxWidth), restored when the wrap comes back on.
const promptMaxWidth = 500

// sizePromptBox gives the editor's textarea its width: the pane's box width w
// while wrapping, so a line breaks at the edge the eye sees, or noWrapWidth
// while not. MaxWidth is the library's cap on SetWidth (500 by default) and has
// to be lifted for the wide box; it goes back to that default when wrapping
// (promptMaxWidth), so a wrapping editor behaves exactly as it did before the
// toggle existed.
//
// The SetHeight at the end is the library's one public way to make it
// re-scroll its view around the caret (setPromptCaretOffset leans on the same
// call): a width change re-flows every line, and without it the caret could be
// left below the box until the next keystroke moved it.
func sizePromptBox(ta *textarea.Model, w int, wrap bool) {
	if wrap {
		ta.MaxWidth = promptMaxWidth
		ta.SetWidth(w)
	} else {
		ta.MaxWidth = 0
		ta.SetWidth(noWrapWidth + promptGutterWidth(*ta))
	}
	ta.SetHeight(ta.Height())
}

// promptBoxWidth is the width the form gives the editor's box — newFormInputs'
// and applySizes' rule, pane less four cells — measured here so the unwrapped
// view can crop to the same edge a wrapping editor breaks at.
func (m model) promptBoxWidth() int {
	if w := m.width - 4; w >= 20 {
		return w
	}
	return 60
}

// promptTextCells is how many cells of text the box shows across, past its
// "┃ " gutter: the window the unwrapped view crops to.
func (m model) promptTextCells() int {
	return max(m.promptBoxWidth()-promptGutterWidth(m.promptArea), 1)
}

// promptTextX turns a pointer column on the pane into the column the
// textarea's own layout has under it. While wrapping the two are the same; in
// the unwrapped view the text is drawn promptXOff cells to the left of where
// the library laid it out, so a click is moved right by as much. The gutter is
// not scrolled — it is drawn at the left edge whatever the offset — so a click
// in it keeps its column, which is what makes it still mean "the start of what
// is on this row".
func (m model) promptTextX(x int) int {
	if m.softWrap || x < promptGutterWidth(m.promptArea) {
		return x
	}
	return x + m.promptXOff
}

// followPromptCaret keeps the caret inside the unwrapped view's window after
// every message (Update's last step). It runs after the message rather than
// inside the handful of handlers that move the caret because nearly every key
// in the editor moves it — the textarea's own motions, a paste, an undo, a
// click — and one rule after all of them cannot miss one.
//
// While wrapping there is nothing to follow, and the offset is zeroed so that
// turning the wrap off again starts from the left edge.
func followPromptCaret(next tea.Model, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	m, ok := next.(model)
	if !ok {
		return next, cmd
	}
	switch {
	case m.softWrap:
		m.promptXOff = 0
	case m.stage == stageForm:
		m.promptXOff = promptXOffFor(m.promptXOff, m.promptArea.LineInfo().CharOffset, m.promptTextCells())
	}
	return m, cmd
}

// promptXOffFor is the scroll rule: the least movement that puts the caret's
// cell inside [xoff, xoff+cells). Least movement, as the lists' ensureVisible
// does it, so walking the caret along a line that already fits moves nothing,
// and typing at the right edge slides the text one cell at a time the way a
// one-line field does — the view never jumps by a page under the hand.
//
// cell is the caret's offset in cells from the start of its row, the
// library's LineInfo().CharOffset, so a double-width glyph before the caret
// counts as the two cells it is drawn in.
func promptXOffFor(xoff, cell, cells int) int {
	switch {
	case cells <= 0 || cell < xoff:
		return max(cell, 0)
	case cell >= xoff+cells:
		return cell - cells + 1
	}
	return xoff
}

// cropPromptLine shows the window of one drawn editor row: its gutter (the
// first gutter cells), then the cells [gutter+xoff, gutter+xoff+cells) of the
// text, and nothing else.
//
// It is written out rather than built from ansi.Cut because Cut is
// TruncateLeft underneath, and TruncateLeft drops every escape sequence before
// its cut point. The library draws the caret's row in its own style (the
// CursorLine field), opened once at the start of a long run of text; cut into
// the middle of that run, the window would come back unstyled. So every escape
// is kept wherever it falls — they draw nothing, and replaying all of them in
// order leaves the terminal in exactly the state the uncropped row would have —
// and only the printing cells are filtered by position.
//
//	drawn:   ┃ ·······[ the quick brown fox jumps over ]·········
//	          ↑ gutter ↑ gutter+xoff              ↑ gutter+xoff+cells
//	shown:   ┃ the quick brown fox jumps over
//
// A double-width glyph straddling either edge of the window is drawn as the
// spaces it covers inside it: half a glyph cannot be printed, and dropping it
// whole would make the row a cell short and shift what follows it.
func cropPromptLine(line string, gutter, xoff, cells int) string {
	in := func(c int) bool {
		return c < gutter || (c >= gutter+xoff && c < gutter+xoff+cells)
	}
	var b strings.Builder
	var state byte
	col := 0
	for len(line) > 0 {
		seq, width, n, newState := ansi.DecodeSequence(line, state, nil)
		state = newState
		line = line[n:]
		if width == 0 {
			b.WriteString(seq)
			continue
		}
		kept := 0
		for c := col; c < col+width; c++ {
			if in(c) {
				kept++
			}
		}
		switch {
		case kept == width:
			b.WriteString(seq)
		case kept > 0:
			b.WriteString(strings.Repeat(" ", kept))
		}
		col += width
	}
	return b.String()
}

// cropPromptView is cropPromptLine over every row of the editor's view.
func (m model) cropPromptView(view string) string {
	gutter := promptGutterWidth(m.promptArea)
	cells := m.promptTextCells()
	rows := strings.Split(view, "\n")
	for i, r := range rows {
		rows[i] = cropPromptLine(r, gutter, m.promptXOff, cells)
	}
	return strings.Join(rows, "\n")
}

// --- The toggle -----------------------------------------------------------------

// softWrapAction is the ☑ Soft wrap chip, the same on both bars: a toggle, not
// an action, so it takes the retired ☑ Spell chip's grammar rather than the
// action chips' — a checkbox for the state and two greys, the brighter for on.
// The bar's hues mean consequence (cyan and blue for what a prompt carries,
// green for the send that leaves the program, red for the throw-away), and
// this button does none of those: it changes how the screen draws. A hue of
// its own would claim a weight it does not have.
//
// The box is what carries the state, not the grey: two greys are a difference
// the eye only reads with both in view, and there is only ever one of this
// chip. The box also survives every tier — at tierIcons the chip is the glyph
// alone, and ☑ against ☐ still answers "is it on". The two glyphs are the same
// width, so flipping the toggle moves nothing on the row. Words for the state
// ("on"/"off") were tried and cost the editor's bar its chords at 120 columns,
// the width the form's footer is budgeted against (formFooter).
func softWrapAction(on bool) listAction {
	a := listAction{label: "☐ Soft wrap", hint: softWrapKey, tint: colDim}
	if on {
		a.label, a.tint = "☑ Soft wrap", colMuted
	}
	return a
}

// softWrapKey is the toggle's chord: alt+z, the key VS Code gives "Toggle Word
// Wrap" (Option+Z on a Mac), so a hand that has wrapped text elsewhere already
// knows it. Free on both screens: the editor's alt chords are line moves
// (↑/↓), the word motions (←/→) and the split (x), and on the Next List alt is
// not a letter of the query.
const softWrapKey = "alt+z"

// rightAlignLast moves a bar's last chip flush to the pane's right edge, which
// is where the ☑ Soft wrap chip lives on both bars: it is a setting for the
// whole screen rather than something done to the prompt or the item, and
// standing apart from the row of actions is what says so. When the pane is too
// narrow to leave a gap between it and the chip before it, it stays where the
// row put it — barTier already guaranteed the row fits, and the gap is what
// gives way first, as everywhere else on a bar.
func rightAlignLast(chips []actionChip, width int) {
	n := len(chips)
	if n < 2 || width <= 0 {
		return
	}
	last := &chips[n-1]
	w := last.end - last.start
	if start := width - w; start > chips[n-2].end {
		last.start, last.end = start, width
	}
}

// writeChipsAt renders a row of chips at their computed columns, padding the
// space between them — the gap the row leaves, or the run of blanks before a
// right-aligned chip. render draws chip i. Drawing to the columns the hit test
// reads, rather than joining with a fixed gap, is what keeps a click on the
// moved chip landing on it.
func writeChipsAt(b *strings.Builder, chips []actionChip, from int, render func(i int, c actionChip) string) {
	x := from
	for i, c := range chips {
		if c.start > x {
			b.WriteString(strings.Repeat(" ", c.start-x))
		}
		b.WriteString(render(i, c))
		x = c.end
	}
}

// toggleSoftWrap is the editor's chip and alt+z: flip the wrap, re-lay the box,
// remember the choice, and say on the note line what happened — the change is
// otherwise invisible while no line in the prompt is long enough to wrap.
//
// A failed save keeps the toggle and says so, the report toggleSpell makes:
// the screen has already changed, only the memory of it is lost.
func (m model) toggleSoftWrap() (tea.Model, tea.Cmd) {
	m.softWrap = !m.softWrap
	m.promptXOff = 0
	sizePromptBox(&m.promptArea, m.promptBoxWidth(), m.softWrap)
	m.formErr = ""
	m.formNote = "soft wrap off — long lines scroll sideways"
	if m.softWrap {
		m.formNote = "soft wrap on — long lines wrap at the edge"
	}
	s := loadSettings()
	s.softWrap = m.softWrap
	if err := s.save(); err != nil {
		m.formNote = ""
		m.formErr = "soft wrap preference not saved: " + err.Error()
	}
	return m, nil
}

// toggleNextWrap is the Next List's chip and alt+z: flip the page's wrap, re-cut
// the rows (each item is one row or several now, so the scroll window is
// re-measured too — see nextPage.resize), remember it, and say so in the
// heading's note.
func (m model) toggleNextWrap() (tea.Model, tea.Cmd) {
	m.nextSoftWrap = !m.nextSoftWrap
	m.next.wrap = m.nextSoftWrap
	m.next.resize(m.width, m.height)
	note := "soft wrap off · one row per item"
	if m.nextSoftWrap {
		note = "soft wrap on · each item's whole text"
	}
	m.next.say(note, false)
	s := loadSettings()
	s.nextSoftWrap = m.nextSoftWrap
	if err := s.save(); err != nil {
		m.next.say("soft wrap preference not saved: "+err.Error(), true)
	}
	return m, nil
}
