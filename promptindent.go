// promptindent.go — tab and shift+tab in the prompt: indent and outdent.
//
// A prompt is often a plan, and plans nest: sub-bullets under a step, a code
// block copied from somewhere. With tab spent on the form's focus ring there
// was no way to type that structure except with the space bar. In the prompt,
// tab and shift+tab now do what they do in a code editor. The title and the
// annotation bar keep tab for the ring, and a click is how the prompt is left.
//
//	nothing swept   tab        four spaces at the caret, mid-line included
//	                shift+tab  up to four leading spaces off the caret's line
//	lines swept     tab        four spaces in front of every line touched
//	                shift+tab  up to four leading spaces off each of them
//	column mode     tab        four spaces at every caret   (promptcarets.go)
//	                shift+tab  outdent every line a caret is on
//	                enter      each new line takes its caret's line's indent
//	any caret       enter      a new line starting at the indent of its line
//	                backspace  right after that enter: the whole carried indent
//
// ENTER CARRIES THE INDENT. A new line starts at the indent of the line enter
// was pressed on, so a nested list or a code block keeps its level while it is
// typed, without re-spacing every line. There are no tab stops. The new line
// copies whatever indent the line has (2, 6, 4), rather than rounding it to a
// multiple of four. Two keys override it:
//
//   - backspace straight after the enter takes the whole carried indent back in
//     one press, for a line that should start at the margin again;
//   - shift+tab takes one unit off, for a line that should step out a level.
//
//	"    - one|"  enter      → "    - one" / "    |"
//	              backspace  → "    - one" / "|"
//
// Only an enter pressed on the keyboard carries. A paste goes in verbatim (it is
// a tea.PasteMsg and never meets the newline key), because pasted text brings
// its own indentation, and adding the caret's on top would push every line in.
//
// SPACES, NOT TAB CHARACTERS. That is not a preference; three things force it:
//
//   - The textarea cleans every insert, SetValue included, and its cleaner
//     turns '\t' into four spaces. The setting is private, so a tab
//     character would not survive the next programmatic edit (and every line
//     tool here edits through SetValue).
//   - Everything that turns offsets into screen cells (the selection overlay,
//     caret paints, click hit-testing) sums rune widths. A literal tab is
//     drawn by the terminal out to the next tab stop, so every cell after it
//     would be misplaced.
//   - A prompt is delivered by typing it into Claude Code, and a tab typed
//     into that input means something else there.
//
// Four, then, because it is what the library already turns a pasted tab into,
// so a typed tab and a pasted one leave identical text.
//
// A sweep resolves to whole logical rows the same way every other line tool
// does (promptlines.go). Unlike alt+↑/↓, the text of those rows changes, so the
// sweep cannot be moved by one fixed amount. Each end is remapped by its own
// row's change instead (remapIndentOffset).

package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// promptIndentUnit is one level of indentation. See the file comment for why
// it is spaces and why four.
const promptIndentUnit = "    "

// promptIndentDir answers whether msg is tab (+1), shift+tab (-1), or neither (0).
//
// It checks the key code and modifier bits, the way promptLineMoveKey does,
// rather than the printed chord name, which would also carry lock bits from a
// kitty terminal. Any other modifier means it is not an indent, so those chords
// stay free. ctrl+i is not caught here on a kitty terminal, since it arrives as
// 'i' with ctrl and is the attachments chord. On a legacy terminal ctrl+i is
// the same 0x09 byte as tab, and there is no way to tell them apart.
func promptIndentDir(msg tea.KeyPressMsg) int {
	if msg.Code != tea.KeyTab || msg.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper|tea.ModMeta) != 0 {
		return 0
	}
	if msg.Mod&tea.ModShift != 0 {
		return -1
	}
	return 1
}

// indentPromptLines is tab / shift+tab with the prompt focused and the column
// mode off.
func (m model) indentPromptLines(dir int) (tea.Model, tea.Cmd) {
	rows := strings.Split(m.promptArea.Value(), "\n")
	first, last, _, _, swept := m.promptSelRows()

	if !swept && dir > 0 {
		// Nothing swept: the tab is typed where the caret stands, like any
		// character. Mid-line counts too, which is how text after a label gets
		// lined up. Indenting the whole line instead would make tab do nothing
		// visible from the middle of a word.
		m.clearPromptSel()
		caret := promptCaretOffset(m.promptArea)
		m.replacePromptRunes(caret, caret, promptIndentUnit)
		m.formNote = ""
		return m, nil
	}
	if !swept {
		// shift+tab has no "at the caret" meaning, since there is nothing to
		// remove there, so it takes indentation off the caret's whole line, as
		// it does in every editor. An anchored but empty selection goes the
		// same way any other key clears it.
		m.clearPromptSel()
		first = min(max(m.promptArea.Line(), 0), len(rows)-1)
		last = first
	}

	out, deltas := reindentPromptRows(rows, first, last, dir)
	if deltas == nil {
		switch {
		case dir > 0:
			m.formNote = "nothing to indent — the swept lines are empty"
		case swept:
			m.formNote = "nothing to outdent — none of the swept lines starts with a space"
		default:
			m.formNote = "nothing to outdent — the line does not start with a space"
		}
		return m, nil
	}

	// Both ends are read before the value changes, because afterwards they are
	// offsets into a layout that no longer exists.
	caret := promptCaretOffset(m.promptArea)
	anchor := m.promptSel.anchor
	m.promptArea.SetValue(strings.Join(out, "\n"))
	setPromptCaretOffset(&m.promptArea, remapIndentOffset(rows, out, deltas, caret))
	if swept {
		// The sweep stays. A second tab indents the same lines again, which is
		// how a block gets pushed in two levels. Clearing it would make the
		// second press type four spaces over wherever the caret ended up.
		m.promptSel = promptSel{anchor: remapIndentOffset(rows, out, deltas, anchor), active: true}
	}
	m.formNote = ""
	return m, nil
}

// reindentPromptRows returns rows with first..last indented (dir > 0) or
// outdented (dir < 0), plus how many runes each row gained or lost. deltas is
// nil when no row changed, which is the caller's cue to explain why.
//
// Empty rows are skipped when indenting. Four spaces on a blank line add
// trailing whitespace nobody can see, and a later outdent would have to take it
// back off. Code editors skip them for the same reason. An outdent takes only
// spaces, up to one unit's worth, so a line indented by two loses two and a line
// that starts with text is left alone.
func reindentPromptRows(rows []string, first, last, dir int) (out []string, deltas []int) {
	out = append([]string{}, rows...)
	deltas = make([]int, len(rows))
	changed := false
	for r := first; r <= last && r < len(rows); r++ {
		if dir > 0 {
			if rows[r] == "" {
				continue
			}
			out[r] = promptIndentUnit + rows[r]
			deltas[r] = len(promptIndentUnit)
		} else {
			k := promptOutdentWidth(rows[r])
			out[r] = rows[r][k:] // spaces are one byte each, so a byte slice is a rune slice here
			deltas[r] = -k
		}
		changed = changed || deltas[r] != 0
	}
	if !changed {
		return rows, nil
	}
	return out, deltas
}

// promptOutdentWidth is how many leading spaces one outdent takes off row, at
// most one indent unit.
func promptOutdentWidth(row string) int {
	k := 0
	for k < len(promptIndentUnit) && k < len(row) && row[k] == ' ' {
		k++
	}
	return k
}

// remapIndentOffset carries a rune offset in the old value to the matching
// offset in the reindented one.
//
// An offset is a (row, column) pair, and the row count does not change, so only
// the column moves, by that row's own delta:
//
//	old  "a\nb"   sweep [0, 2) → only row 0 is indented, row 1 is not touched
//	new  "    a\nb"
//	  anchor (0,0) → (0,0)   column 0 stays at the line start (see below)
//	  caret  (1,0) → (1,0)   same column, but its offset goes from 2 to 6
//	                         because row 0 grew by four above it
//
// On an indent, column 0 stays at 0. A sweep that begins at a line start
// (the usual shape: a drag down the left margin, or shift+↓ from column 0)
// should still begin at the line start afterwards, with the new indent inside
// the highlight. Otherwise the next shift+tab would look at a sweep that no
// longer covers its lines' leading spaces. On an outdent, an offset inside the
// removed spaces moves back to the line start rather than onto a column that
// no longer exists.
func remapIndentOffset(old, out []string, deltas []int, off int) int {
	row, col := 0, max(off, 0)
	for row < len(old)-1 && col > len([]rune(old[row])) {
		col -= len([]rune(old[row])) + 1
		row++
	}
	if d := deltas[row]; (d > 0 && col > 0) || d < 0 {
		col = max(col+d, 0)
	}
	col = min(col, len([]rune(out[row])))

	start, _ := promptRowSpan(out, row, row)
	return start + col
}

// promptCarry is an indent that enter just carried. It is kept so the backspace
// after it can take the whole indent back (see the file comment).
//
// It is a snapshot of the editor right after the enter, not a flag kept in step
// with it. The backspace honours it only while the value and the caret (or every
// caret) still match. A click elsewhere, or an edit by any road (a paste, a menu
// action), leaves it stale without that road needing to know it exists. It also
// lasts one key: updateForm takes it and zeroes the field on the way in, so
// typing a character and erasing it does not bring the one-press backspace back.
type promptCarry struct {
	value  string
	widths []int // spaces carried at each caret; nil when nothing was carried
	caret  int   // column mode off: the offset just past the indent
	rows   []int // column mode on: every caret as the enter left it
	cols   []int
}

// promptCarriedIndent is what enter at column col of row carries onto the new
// line: the row's leading spaces, counted no further than the caret. A caret
// standing inside the indent splits it, and the new line gets only the part in
// front of the caret, because the rest already travels with the text after it.
//
// blank reports that the row is nothing but those spaces, with the caret at its
// end. That is the line an earlier carry left untouched. Enter there moves the
// indent down instead of copying it, so the line left behind is empty rather
// than holding invisible trailing spaces (the reason reindentPromptRows skips
// blank rows).
//
//	"  - a|"    → indent 2, blank false
//	"  |  - a"  → indent 2, blank false   (only what is left of the caret)
//	"    |"     → indent 4, blank true
func promptCarriedIndent(row []rune, col int) (indent int, blank bool) {
	col = min(max(col, 0), len(row))
	for indent < col && row[indent] == ' ' {
		indent++
	}
	// indent == len(row) also means col == len(row), since indent <= col.
	return indent, indent > 0 && indent == len(row)
}

// newlineCarryingIndent is enter in the prompt with the column mode off: a line
// break at the caret, then the indent the caret's line carries
// (promptCarriedIndent). It stands in for the textarea's own InsertNewline,
// which knows nothing about indentation.
//
// The edit goes through replacePromptRunes, a SetValue, so the library's one
// guard on a newline is repeated here. At MaxHeight logical lines its
// InsertNewline refuses, and SetValue would not. At that limit this reports
// false and the key goes on to the library, which refuses it as it always has.
func (m *model) newlineCarryingIndent() bool {
	rows := strings.Split(m.promptArea.Value(), "\n")
	if m.promptArea.MaxHeight > 0 && len(rows) >= m.promptArea.MaxHeight {
		return false
	}
	caret := promptCaretOffset(m.promptArea)
	row, _ := promptRowRange(rows, caret, caret)
	start, _ := promptRowSpan(rows, row, row)
	n, blank := promptCarriedIndent([]rune(rows[row]), caret-start)
	from := caret
	if blank {
		from -= n // the blank line's spaces move down rather than being copied
	}
	m.replacePromptRunes(from, caret, "\n"+strings.Repeat(" ", n))
	if n > 0 {
		m.promptCarry = promptCarry{
			value:  m.promptArea.Value(),
			widths: []int{n},
			caret:  promptCaretOffset(m.promptArea),
		}
	}
	return true
}

// takeBackCarriedIndent is backspace with the column mode off, straight after an
// enter that carried an indent. The whole indent goes in one press, and the caret
// is back at the margin. It reports false, and does nothing, when carry does not
// describe the editor as it stands; the key then deletes one character as usual.
func (m *model) takeBackCarriedIndent(carry promptCarry) bool {
	if len(carry.widths) != 1 || carry.rows != nil {
		return false
	}
	caret := promptCaretOffset(m.promptArea)
	if caret != carry.caret || m.promptArea.Value() != carry.value {
		return false
	}
	m.replacePromptRunes(caret-carry.widths[0], caret, "")
	return true
}
