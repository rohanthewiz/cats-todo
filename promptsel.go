// promptsel.go — selecting a run of text inside the prompt editor.
//
// The bubbles textarea has no notion of a selection: no anchor, no mark, no
// highlight, and nothing in its Styles for one. Everything here is the layer
// that gives it one, built entirely from what the library exposes publicly plus
// a faithful copy of the one private thing a highlight cannot be drawn without
// (the soft-wrap — see wrapPromptRunes).
//
// The design in one picture. A selection is two rune offsets into the editor's
// value: an anchor the gesture started at, and the caret, which the textarea
// moves for us:
//
//	value:   t h e   q u i c k   b r o w n   f o x
//	                 ▲                   ▲
//	              anchor               caret
//	span:            └─── [lo, hi) ─────┘
//
// The caret is never set by this file. shift+← hands the textarea a plain ← and
// lets it move its own caret; a drag hands the existing placePromptCursor the
// pointer's cell. Both leave the anchor alone, and the span falls out of the
// two. That is what keeps the selection correct across soft wraps, double-width
// glyphs and scrolling without this file having to re-derive any of it.
//
// Rendering is the other half, and the reason the wrap has to be known: a
// highlight is painted on screen cells, and the map from "rune offset in the
// value" to "cell on line N of the editor's box" is exactly the wrap.

package main

import (
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	rw "github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

// promptSel is the prompt editor's selection anchor: the rune offset into the
// value where the current gesture began.
//
// Only the anchor is stored. The caret is the textarea's own state and is read
// back out of it (promptCaretOffset) whenever the span is needed, so there is no
// second copy of the cursor here to fall out of step with the real one — an edit,
// a scroll, or a resize moves the caret and the span follows for free.
//
// active distinguishes "no selection" from "an empty selection anchored at
// offset 0". The difference is load-bearing: an empty selection still holds an
// anchor, so a shift+→ that follows a shift+← keeps growing from the same place
// instead of re-anchoring at the caret.
type promptSel struct {
	anchor int
	active bool
}

// promptSelSpan is the half-open rune range [lo, hi) the selection covers, and
// whether there is anything in it. An anchored-but-empty selection reports
// ok=false: there is nothing to paint and nothing to copy, even though the
// anchor is still live.
func (m model) promptSelSpan() (lo, hi int, ok bool) {
	if !m.promptSel.active {
		return 0, 0, false
	}
	caret := promptCaretOffset(m.promptArea)
	lo, hi = m.promptSel.anchor, caret
	if lo > hi {
		lo, hi = hi, lo
	}
	// The anchor is a snapshot of an offset that can be invalidated by an edit
	// the selection did not survive (an undo of a paste, a value set from
	// elsewhere). Clamp rather than trust it: a span past the end of the value
	// would slice out of range in selectedPromptText.
	n := len([]rune(m.promptArea.Value()))
	lo, hi = min(max(lo, 0), n), min(max(hi, 0), n)
	return lo, hi, hi > lo
}

// selectedPromptText is the selected run of the value, or "" when nothing is
// selected. This is what the clipboard gets.
func (m model) selectedPromptText() string {
	lo, hi, ok := m.promptSelSpan()
	if !ok {
		return ""
	}
	return string([]rune(m.promptArea.Value())[lo:hi])
}

// anchorPromptSel drops an anchor at the caret unless one is already down. It is
// called on the first key of a shift+motion run and on the press that starts a
// drag, and is deliberately idempotent-ish: the second shift+← of a run must not
// re-anchor, or the selection would never grow past one character.
func (m *model) anchorPromptSel() {
	if m.promptSel.active {
		return
	}
	m.promptSel = promptSel{anchor: promptCaretOffset(m.promptArea), active: true}
}

// clearPromptSel drops the anchor. Every key that is not part of a selection
// gesture goes through here — a plain arrow, a typed character, a save — because
// a highlight left standing over text the caret has walked away from is a lie
// about what the next ctrl+c would copy.
func (m *model) clearPromptSel() {
	m.promptSel = promptSel{}
	m.promptSelDrag = false
}

// promptCaretOffset is the caret's absolute rune offset into the editor's value.
//
// The textarea reports its caret as (logical row, column within that row), which
// is the wrong coordinate for a selection: a span that crosses a newline has to
// be one range, not a pair of row/column points to compare lexicographically.
// Rows above the caret are summed, one rune each plus the newline that ended
// them.
func promptCaretOffset(ta textarea.Model) int {
	rows := strings.Split(ta.Value(), "\n")
	row := min(max(ta.Line(), 0), len(rows)-1)
	off := 0
	for i := range row {
		off += len([]rune(rows[i])) + 1 // +1 for the '\n' that ends the row
	}
	return off + min(max(ta.Column(), 0), len([]rune(rows[row])))
}

// --- Keyboard -----------------------------------------------------------------

// promptMotionKeys are the caret motions that extend a selection when shift is
// held. They are matched on Key.Code rather than on the chord's printed name so
// every modifier combination the textarea binds comes along for free: shift+←
// selects a character, shift+alt+← selects a word (the textarea's WordBackward),
// shift+ctrl+home selects to the top (its InputBegin), all through the same
// path, because the only thing this file does with shift is take it off.
var promptMotionKeys = map[rune]bool{
	tea.KeyLeft: true, tea.KeyRight: true,
	tea.KeyUp: true, tea.KeyDown: true,
	tea.KeyHome: true, tea.KeyEnd: true,
	tea.KeyPgUp: true, tea.KeyPgDown: true,
}

// promptSelectionKey answers whether msg is "extend the selection", and if so
// returns the plain motion underneath it — the same key with shift stripped,
// which is what the textarea is handed so it moves its own caret.
//
// Stripping rather than reimplementing the motion is the whole trick. The
// library owns what ← does at a soft wrap and what ↑ does across a double-width
// glyph; reproducing that here to compute a new offset would be a second
// implementation of the caret, and the two would disagree on exactly the cases
// this feature exists to handle.
func promptSelectionKey(msg tea.KeyPressMsg) (tea.KeyPressMsg, bool) {
	if msg.Mod&tea.ModShift == 0 || !promptMotionKeys[msg.Code] {
		return msg, false
	}
	msg.Mod &^= tea.ModShift
	// Alt comes off the vertical pair as well, and only that pair. alt+↑/↓ is
	// the line move (promptmove.go), so leaving the bit on would hand the
	// editor a key that reorders the value instead of one that walks the caret
	// down it — shift+alt+↓ would extend the selection by dragging the line out
	// from under it. The horizontal pair keeps its alt, because there it is the
	// textarea's own word motion, which is exactly what shift+alt+←/→ is for.
	if msg.Code == tea.KeyUp || msg.Code == tea.KeyDown {
		msg.Mod &^= tea.ModAlt
	}
	// ShiftedCode is what the terminal reported as the shifted spelling of the
	// key; leaving it set on a now-unshifted message would let a reader
	// downstream conclude shift is still held.
	msg.ShiftedCode = 0
	return msg, true
}

// --- Display lines -------------------------------------------------------------

// promptDisplayLine is one drawn line of the editor: which logical row of the
// value it belongs to, where in the value its text starts, and how many runes of
// the value it shows. A logical row that soft-wraps contributes several of
// these, in order.
//
//	value row 0: "the quick brown fox jumps"     width 12
//	  display 0: start 0  len 10   "the quick "
//	  display 1: start 10 len 6    "brown "
//	  display 2: start 16 len 9    "fox jumps"
type promptDisplayLine struct {
	row    int
	start  int
	length int
}

// end is the offset one past the last rune this line shows.
func (d promptDisplayLine) end() int { return d.start + d.length }

// promptDisplayLines is the editor's whole value as the lines it is drawn on, in
// the order it draws them — display-line index → line. Index i is the i'th line
// of the editor's content, so the line visible at screen row y is index
// ScrollYOffset()+y.
//
// This exists because the textarea will not say where its soft wraps fall.
// LineInfo answers the question only for the line the caret happens to be on,
// and the one public way to ask about another line is to walk the caret there —
// which moves the caret and scrolls the view, neither of which a View may do.
// So the wrap is computed instead (wrapPromptRunes), from the same value and the
// same width the library uses.
func promptDisplayLines(ta textarea.Model) []promptDisplayLine {
	width := ta.Width()
	rows := strings.Split(ta.Value(), "\n")
	out := make([]promptDisplayLine, 0, len(rows))
	off := 0
	for r, row := range rows {
		runes := []rune(row)
		wrapped := wrapPromptRunes(runes, width)
		start := off
		for i, wl := range wrapped {
			n := len(wl)
			if i == len(wrapped)-1 {
				// The wrap appends one space to its final line that is not in the
				// value (see wrapPromptRunes); it is padding for the caret to sit
				// on, not a character, so it is not part of what this line shows.
				n--
			}
			n = max(n, 0)
			out = append(out, promptDisplayLine{row: r, start: start, length: n})
			start += n
		}
		off += len(runes) + 1 // +1 for the '\n' that ended the row
	}
	return out
}

// promptOffsetAt is the rune offset into the editor's value that the pointer
// landed on: row is the clicked line counted from the top of the editor's box,
// x its column on the screen. ok is false when the click is past the last line
// the value fills, where there is no text to have pointed at.
//
// It is placePromptCursor's question asked without moving anything. That
// function has to walk the caret, because moving the caret is what it is for;
// a gesture that only wants to know *which word is under there* must not
// scroll the view or take the caret off the character the user is typing, so
// the answer is computed from the display-line table instead. The table is
// keyed by screen line — the line drawn at screen row y is index
// ScrollYOffset()+y — which is the whole mapping.
//
// A click in the editor's gutter (x left of the first character) resolves to
// the start of that line: the "┃ " sits one cell from the first letter, and a
// hand aiming at the first word of a line lands there often enough that the
// nearest character is the more useful reading of the miss.
func promptOffsetAt(ta textarea.Model, x, row int) (int, bool) {
	lines := promptDisplayLines(ta)
	i := ta.ScrollYOffset() + max(row, 0)
	if i < 0 || i >= len(lines) {
		return 0, false
	}
	dl := lines[i]
	runes := []rune(ta.Value())
	lo := min(dl.start, len(runes))
	hi := min(dl.end(), len(runes))
	// colAtWidth sums widths rather than counting runes, so a double-width
	// glyph earlier on the line does not shift the answer off its word — the
	// same care the selection and the spell underline take.
	return lo + colAtWidth(runes[lo:hi], x-promptGutterWidth(ta)), true
}

// promptCaretDisplayLine is the index into promptDisplayLines of the line the
// caret is drawn on.
//
// The caret's own line is the one thing the library will answer directly, and it
// is worth asking rather than deriving: at a soft wrap two display lines meet at
// a single offset, and which side of the seam the caret is shown on is the
// library's decision, not something the offset alone can settle.
func promptCaretDisplayLine(ta textarea.Model, lines []promptDisplayLine) int {
	row := ta.Line()
	for i, d := range lines {
		if d.row == row {
			return min(i+ta.LineInfo().RowOffset, len(lines)-1)
		}
	}
	return 0
}

// wrapPromptRunes is a faithful copy of the textarea's own soft-wrap, which the
// library keeps private (textarea.wrap). It is transcribed rather than
// approximated: a highlight is painted on the cells the editor actually drew, so
// a wrap that disagreed with the library's by one word would put the highlight
// on the wrong characters — and only on long lines, which is the worst way for
// it to be wrong.
//
// TestPromptDisplayLinesMatchTheTextarea pins it to the library by checking the
// table it produces against what the textarea's own LineInfo reports for a caret
// walked across a corpus of values, so a change to the library's wrapping fails
// here rather than in a user's editor.
//
// Two properties of the original that everything else depends on:
//
//   - It preserves runes. A space is counted and re-emitted as a space, a
//     non-space is appended verbatim, so the concatenation of the returned lines
//     is the input with exactly one space added at the end. That is what lets a
//     display line be described as a (start, length) window on the value.
//   - Only the last returned line carries that added space.
func wrapPromptRunes(runes []rune, width int) [][]rune {
	var (
		lines  = [][]rune{{}}
		word   []rune
		row    int
		spaces int
	)

	for _, r := range runes {
		if unicode.IsSpace(r) {
			spaces++
		} else {
			word = append(word, r)
		}

		if spaces > 0 {
			// A run of spaces closes the pending word: it either fits on the
			// current line with them, or starts the next one.
			if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces > width {
				row++
				lines = append(lines, []rune{})
			}
			lines[row] = append(lines[row], word...)
			lines[row] = append(lines[row], repeatPromptSpaces(spaces)...)
			spaces, word = 0, nil
		} else if uniseg.StringWidth(string(word))+rw.RuneWidth(word[len(word)-1]) > width {
			// No space has closed this word yet and it has grown too wide to
			// place at all, so it is hard-broken. The last rune's width is added
			// on because a double-width glyph must not be allowed to straddle the
			// right edge.
			if len(lines[row]) > 0 {
				row++
				lines = append(lines, []rune{})
			}
			lines[row] = append(lines[row], word...)
			word = nil
		}
	}

	// Whatever is still pending lands on the last line, or on one of its own if
	// it no longer fits. The trailing space is the library's: it gives the caret
	// a cell to sit on past the final character, and having it on every wrapped
	// line's end keeps caret motion uniform across the seams.
	if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces >= width {
		row++
		lines = append(lines, []rune{})
	}
	lines[row] = append(lines[row], word...)
	lines[row] = append(lines[row], repeatPromptSpaces(spaces+1)...)

	return lines
}

func repeatPromptSpaces(n int) []rune { return []rune(strings.Repeat(" ", max(n, 0))) }

// --- Rendering ------------------------------------------------------------------

// promptEditorView is the prompt editor as drawn, with the selection and the
// spell-check marks (spell.go) painted over it. With nothing selected and
// nothing flagged it is the textarea's own view, untouched.
//
// The highlight is an overlay rather than a re-render. Only the lines the
// selection actually touches are rebuilt, and each of those keeps the editor's
// own output for every cell to the left of the highlight — so the gutter, the
// cursor-line field and anything else the library draws there survive without
// this file having to know they exist. The spell marks ride the same overlay:
// a line with marks and no selection, or with both, goes through
// paintPromptSpans, the many-run form of the same operation; a line with only
// the selection on it takes the path below exactly as it always has.
func (m model) promptEditorView() string {
	view := m.promptArea.View()
	lo, hi, ok := m.promptSelSpan()
	marks := m.promptSpellSpans()
	if !ok && len(marks) == 0 && !m.carets.on {
		return view
	}

	var (
		lines   = promptDisplayLines(m.promptArea)
		runes   = []rune(m.promptArea.Value())
		gutter  = promptGutterWidth(m.promptArea)
		right   = gutter + m.promptArea.Width() // one past the last text cell
		top     = m.promptArea.ScrollYOffset()
		caretD  = promptCaretDisplayLine(m.promptArea, lines)
		caretAt = promptCaretOffset(m.promptArea)
		drawn   = strings.Split(view, "\n")
	)

	// The style the unselected tail of a rebuilt line is redrawn in. The library
	// paints the caret's logical row differently from the rest, which is a
	// difference the eye notices the moment a highlight sits on that row and the
	// text after it comes back a shade off.
	styles := m.promptArea.Styles().Blurred
	if m.promptArea.Focused() {
		styles = m.promptArea.Styles().Focused
	}

	for y, line := range drawn {
		d := top + y
		if d < 0 || d >= len(lines) {
			continue // padding past the end of the value: nothing to select there
		}
		dl := lines[d]

		// The selection's cells on this line, if it has any. hasSel false means
		// this line is entirely outside the selection (or there is none), and a
		// and b are meaningless.
		var a, b int
		hasSel := false
		if ok {
			selLo, selHi := max(lo, dl.start), min(hi, dl.end())
			hasSel = selLo < selHi || (hi > dl.end() && lo <= dl.end())
			if hasSel {
				// Rune offsets → screen cells. Widths are summed rather than
				// counted so the highlight lands on the same characters the eye
				// sees even on a line carrying double-width glyphs.
				a = gutter + lipgloss.Width(string(runes[dl.start:max(selLo, dl.start)]))
				b = gutter + lipgloss.Width(string(runes[dl.start:max(selHi, dl.start)]))
				if hi > dl.end() {
					// The selection runs off this line onto the next, so the line
					// break itself is inside it. Painting to the right edge is what
					// says so — a highlight that stopped at the last character
					// would read as several separate selections stacked up rather
					// than one block of text.
					b = right
				}
			}
		}

		base := styles.Text
		if dl.row == m.promptArea.Line() {
			base = styles.CursorLine
		}
		caretCell := -1
		if d == caretD && caretAt >= dl.start {
			caretCell = gutter + lipgloss.Width(string(runes[dl.start:min(caretAt, len(runes))]))
		}

		paints := spellPaintsFor(marks, dl, runes, gutter, base, hasSel, a, b)
		// The column mode's extra carets (promptcarets.go) ride the same
		// overlay. They are merged rather than appended: a caret can land inside
		// a spell mark, and paintPromptSpans requires its runs not to overlap —
		// two runs claiming one cell would emit that cell's character twice and
		// push the rest of the line right.
		if cp := m.promptCaretPaints(dl, runes, gutter); len(cp) > 0 {
			paints = mergeCaretPaints(paints, cp)
		}
		switch {
		case len(paints) == 0 && !hasSel:
			continue // nothing on this line to paint
		case len(paints) == 0:
			drawn[y] = paintPromptSelection(line, a, b, caretCell, base)
		default:
			if hasSel {
				paints = append(paints, promptPaint{a: a, b: b, style: promptSelStyle})
				sortPromptPaints(paints)
			}
			drawn[y] = paintPromptSpans(line, paints, caretCell, base)
		}
	}
	return strings.Join(drawn, "\n")
}

// paintPromptSelection puts the selection field on cells [a, b) of one drawn
// line, and redraws the caret at caretCell when the caret sits outside the
// highlight.
//
//	│ the quick brown fox jumps
//	│     ├──── highlight ───┤▌
//	0     a                  b caret
//
// Everything left of a is the editor's own output, escape sequences and all,
// which is why it is cut rather than rebuilt. From a rightwards the line is
// redrawn from its plain text: ansi.TruncateLeft drops the escapes that opened
// before its cut point, so a tail spliced back in raw would lose the cursor
// line's field partway across the row. Rendering it with the style the library
// drew it in costs one Strip and is exact for as long as those styles are the
// only ones in play — which the form guarantees, since it never sets its own
// (see newFormInputs).
func paintPromptSelection(line string, a, b, caretCell int, base lipgloss.Style) string {
	if b <= a {
		return line
	}
	head := ansi.Cut(line, 0, a)
	sel := promptSelStyle.Render(ansi.Strip(ansi.Cut(line, a, b)))
	tail := ansi.Strip(ansi.TruncateLeft(line, b, ""))

	// The caret is drawn by the library as a reversed cell inside the text; it
	// lives in the tail whenever the selection was grown forwards, and stripping
	// the tail's escapes takes it with them. A selection grown backwards puts the
	// caret at cell a instead, where the highlight already marks it, so there is
	// nothing to put back.
	//
	// An empty cut means the caret is past the last cell this line was drawn
	// with, which happens where the library chose not to draw one either. The
	// line is returned a cell short rather than having one invented for it: the
	// overlay must not change how wide a line is, or the pane's own wrapping
	// takes over and the form's rows stop being where every click is aimed.
	if at := caretCell - b; caretCell >= b {
		if cell := ansi.Cut(tail, at, at+1); cell != "" {
			return head + sel +
				base.Render(ansi.Cut(tail, 0, at)) +
				promptCaretStyle.Render(cell) +
				base.Render(ansi.TruncateLeft(tail, at+1, ""))
		}
	}
	return head + sel + base.Render(tail)
}

// --- Typing over a selection ----------------------------------------------------

// promptSelInsertKey answers whether msg is a key that would put text into the
// editor: a printable character, or one of the spellings of "newline". Those are
// the keys that *replace* a selection — the run comes out and what was typed
// goes in where it was.
//
// The two halves are recognized differently because the library recognizes them
// differently. A newline is a binding — the form rebinds it to five chords (see
// newFormInputs) — so it is matched against that binding rather than against a
// list repeated here that would drift from it. Everything else falls to the
// textarea's default branch, which inserts msg.Text, and Text is populated only
// for keys that carry printable characters. Testing it is therefore not an
// approximation of what the editor will do with the key: it is the same test,
// which is what keeps "the selection was replaced" and "a character was typed"
// from ever disagreeing.
//
// No modifier is filtered out here, and deliberately so. A chord's binding is
// matched on Key.String(), which *is* Key.Text whenever the key carries text —
// so a hypothetical alt+d arriving with a printable 'd' would miss the
// delete-word binding inside the textarea too, and be inserted there. Filtering
// it out here would leave that character going in beside the selection instead
// of over it, which is the exact bug this exists to fix. The delete keys are
// still safe: the caller tests them first, and the real ones carry no text.
func promptSelInsertKey(msg tea.KeyPressMsg, km textarea.KeyMap) bool {
	return key.Matches(msg, km.InsertNewline) || msg.Text != ""
}

// promptSelDeleteKey answers whether msg is a key that removes text to one side
// of the caret. With a selection standing, all four mean the same thing — take
// out what is highlighted — which is what backspace and delete do in every
// editor that has a selection at all.
//
// The bindings are read off the editor's own keymap rather than named here, so
// every spelling the library gives them comes along: ctrl+h for backspace,
// ctrl+d for delete, ctrl+w and alt+backspace for the word behind.
//
// The line kills (ctrl+k, ctrl+u) are deliberately left out. They are not "erase
// what is marked" in a smaller or larger amount; they are a different operation
// with its own meaning — everything to one side of the caret — and a hand
// reaching for one wants that, selection or no selection.
func promptSelDeleteKey(msg tea.KeyPressMsg, km textarea.KeyMap) bool {
	return key.Matches(msg,
		km.DeleteCharacterBackward, km.DeleteCharacterForward,
		km.DeleteWordBackward, km.DeleteWordForward)
}

// deletePromptSelection takes the selected run out of the value and drops the
// anchor, leaving the caret where the run began — which is where the next
// inserted character belongs. It reports whether there was anything to delete,
// so a caller that only wanted the side effect can ignore it.
//
// The edit goes through replacePromptRunes (spellpanel.go) for the reason given
// there: the library has no "replace this range", and walking the caret to one
// end of the span to send as many backspaces as it is long is the same result by
// a longer road, one that goes wrong quietly if the walk lands a character off.
func (m *model) deletePromptSelection() bool {
	lo, hi, ok := m.promptSelSpan()
	if !ok {
		return false
	}
	m.replacePromptRunes(lo, hi, "")
	m.clearPromptSel()
	return true
}
