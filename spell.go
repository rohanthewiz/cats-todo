// Spell check in the prompt editor: words the dictionary does not know are
// underlined in red as the prompt is typed, and a panel one key away says what
// to do about one. The underline is the whole of the passive feature and is
// deliberately silent about everything else — the point is a glance that
// catches "teh" before it is sent to an agent, and a glance is all a highlight
// costs. Acting on it is opt-in, and lives in spellpanel.go.
//
// The checker itself is internal/spell; this file is what the editor does with
// it. Three things live here:
//
//   - the dictionary's lifetime: loaded once, lazily, the first time a form
//     opens with the check on, so a launch that never edits a prompt never
//     pays for the 90k-word gunzip — and where the user's own word lists are
//     read from and written to;
//   - the toggle: the toolbar's ☑ Spell chip flips it, the choice persists
//     (see settings.go), and the chip's glyph is what says which way it is;
//   - the paint: turning the checker's rune spans into underlined cells on the
//     editor's own rendered lines, by the same overlay technique the text
//     selection uses (see promptsel.go, promptEditorView) — the editor is
//     drawn by the library and marked up afterwards, so it keeps its gutter,
//     cursor line and caret without this file having to redraw them.
//
// The word under the caret is never marked. A word is misspelled the whole
// time it is half-typed, and an underline that appears on "th" and vanishes on
// "the" is a flicker on every word, not a check; so a span the caret is inside
// or at the end of is left alone, and it earns its mark when the caret moves
// on. This is what every editor with squiggles does, and for the same reason.
package main

import (
	"path/filepath"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rohanthewiz/cats-todo/internal/spell"
)

// spellDictFileName is the name of the user's own word lists, one per line:
// <config>/dictionary.txt for words that are theirs everywhere, and
// <project>/.cats-todo/dictionary.txt for a project's jargon (a product name,
// an internal service), which is the kind of word a teammate is also typing
// and so belongs in the repo with the backlog.
const spellDictFileName = "dictionary.txt"

// spellDictGlobalPath is the user's own word list, the one that applies
// wherever they are, or "" when no config directory could be resolved (no home
// directory) — in which case there is nowhere to put it and the panel does not
// offer the row.
func (m model) spellDictGlobalPath() string {
	base, err := configBaseDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, spellDictFileName)
}

// spellDictProjectPath is this project's word list, or "" when the manager was
// not launched inside a project. It lives beside the backlog so a product name
// or an internal service — the kind of word a teammate is also typing — can be
// committed with it.
func (m model) spellDictProjectPath() string {
	dir := m.ctx.projectDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, projectConfigDirName, spellDictFileName)
}

// spellDictPaths is where the user's dictionaries would be if they exist, in
// the order Load reads them. A missing file is fine (spell.Load skips it); this
// only says where to look.
func (m model) spellDictPaths() []string {
	var paths []string
	for _, p := range []string{m.spellDictGlobalPath(), m.spellDictProjectPath()} {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// loadSpellDict makes sure the dictionary is loaded when the check is on. Idempotent
// and cheap after the first call, so both form entry points and the toggle just
// call it. A load failure turns the check off for the run and says so on the
// form's note line rather than failing the form: the editor works without it.
func (m *model) loadSpellDict() {
	if !m.spellOn || m.spellDict != nil {
		return
	}
	d, err := spell.Load(m.spellDictPaths()...)
	if err != nil {
		m.spellOn = false
		m.formNote = "spell check off — could not load a dictionary: " + err.Error()
		return
	}
	m.spellDict = d
}

// toggleSpell flips the check, persists the choice, and says what happened on
// the note line — the change is otherwise visible only if the prompt happens to
// hold a misspelling. It is the toolbar's ☑ Spell chip and the Spelling panel's
// last row; ctrl+l opens that panel rather than coming here (see beginSpell).
func (m model) toggleSpell() (model, error) {
	m.spellOn = !m.spellOn
	m.loadSpellDict()
	if m.spellOn {
		m.formNote = "spell check on"
	} else {
		m.formNote = "spell check off"
	}
	s := loadSettings()
	s.spellcheck = m.spellOn
	if err := s.save(); err != nil {
		// The toggle still took for this run; only its memory failed. Said on
		// the same line, in place of the plain confirmation, because a note that
		// read "spell check off" beside a file that still says on would be the
		// wrong thing to trust.
		m.formNote += " (not saved: " + err.Error() + ")"
		return m, err
	}
	return m, nil
}

// promptSpellSpans is the checker's verdict on the editor's current value:
// the rune spans to underline, minus the one under the caret. Nil when the
// check is off, when the dictionary is not loaded, or when there is nothing
// to say — every caller treats those alike.
func (m model) promptSpellSpans() []spell.Span {
	if !m.spellOn || m.spellDict == nil {
		return nil
	}
	spans := m.spellDict.Check(m.promptArea.Value())
	if len(spans) == 0 {
		return nil
	}
	caret := promptCaretOffset(m.promptArea)
	out := spans[:0]
	for _, sp := range spans {
		// "Inside or at the end of": a caret at Start is before the word and
		// leaves it flagged; a caret anywhere from the second rune to just past
		// the last is typing it.
		if sp.Start < caret && caret <= sp.End {
			continue
		}
		out = append(out, sp)
	}
	return out
}

// promptSpellStyle is how a flagged word is drawn, derived from the style the
// editor drew that line in — the cursor line carries a background, and a mark
// that dropped it would punch a hole in the line's field. Red underline: the
// convention every editor has taught, and readable on either of the editor's
// line tones.
//
// (lipgloss renders an underlined string one rune per escape sequence, so a
// mark costs a few bytes per letter more than a highlight would. It is the
// same cells on screen, and prompts are short.)
func promptSpellStyle(base lipgloss.Style) lipgloss.Style {
	return base.Foreground(lipgloss.Color(colErr)).Underline(true)
}

// promptPaint is one styled run of cells on a drawn editor line: cells [a, b)
// take style. caret says whether the caret, should it fall inside the run, is
// redrawn there. It is for a spell mark (the caret sits in a word all the time)
// and not for the selection, whose highlight already marks the caret's cell —
// see paintPromptSelection, which this generalizes.
type promptPaint struct {
	a, b  int
	style lipgloss.Style
	caret bool
}

// spellPaintsFor turns the checker's spans into cell runs for one display line
// of the editor: only the spans that touch this line, clipped to it, converted
// from rune offsets to screen cells, and with any cells under the selection
// [selA, selB) cut away — the selection wins where the two overlap, since a
// highlight with red letters showing through it reads as two things at once.
//
// dl is the display line, runes the whole value, gutter the width of the
// prompt column left of the text; widths are summed rather than counted so a
// double-width glyph earlier on the line does not shift the underline off its
// word (the same care promptEditorView takes for the selection). hasSel false
// means there is no selection on this line and selA/selB are ignored.
func spellPaintsFor(spans []spell.Span, dl promptDisplayLine, runes []rune, gutter int, base lipgloss.Style, hasSel bool, selA, selB int) []promptPaint {
	var out []promptPaint
	style := promptSpellStyle(base)
	for _, sp := range spans {
		s, e := max(sp.Start, dl.start), min(sp.End, dl.end())
		if s >= e {
			continue
		}
		a := gutter + lipgloss.Width(string(runes[dl.start:s]))
		b := gutter + lipgloss.Width(string(runes[dl.start:e]))
		if !hasSel || b <= selA || a >= selB {
			out = append(out, promptPaint{a: a, b: b, style: style, caret: true})
			continue
		}
		// Overlaps the selection: keep whatever sticks out on either side.
		if a < selA {
			out = append(out, promptPaint{a: a, b: selA, style: style, caret: true})
		}
		if b > selB {
			out = append(out, promptPaint{a: selB, b: b, style: style, caret: true})
		}
	}
	return out
}

// paintPromptSpans redraws one drawn editor line from the first paint's left
// edge rightwards, putting each paint's style on its cells and base on the
// cells between and after them; everything left of the first paint is the
// editor's own output, kept as it came. It is paintPromptSelection for any
// number of runs — see that function for why the head is cut rather than
// rebuilt and the rest rendered from plain text (in short: ansi.TruncateLeft
// drops the escapes opened before its cut, so a raw tail would lose the
// cursor line's field).
//
//	│ the quikc brown fox jumps over teh lazy dog
//	│     ├───┤ underline          ├─┤▌
//	0     a   b                    a b caret
//
// The caret is redrawn where it falls, in the same reversed cell the library
// draws, unless the paint it lands in says not to (the selection). A caret
// past the last drawn cell is left as it was — the library drew none there
// either — and, as in paintPromptSelection, the line comes back exactly as
// wide as it went in.
//
// paints must be sorted by a and non-overlapping.
func paintPromptSpans(line string, paints []promptPaint, caretCell int, base lipgloss.Style) string {
	if len(paints) == 0 {
		return line
	}
	first := paints[0].a
	// The plain text from cell `first` to the end of the line; every segment
	// below is a cell range cut out of this and rendered afresh. Cells are
	// addressed relative to `first`, so a segment [x, y) is plain[x-first,
	// y-first) — the same arithmetic as paintPromptSelection's tail, just
	// carried across several runs.
	plain := ansi.Strip(ansi.TruncateLeft(line, first, ""))
	end := first + ansi.StringWidth(plain)

	var b strings.Builder
	b.WriteString(ansi.Cut(line, 0, first))
	render := func(style lipgloss.Style, x, y int) {
		if s := ansi.Cut(plain, x-first, y-first); s != "" {
			b.WriteString(style.Render(s))
		}
	}
	segment := func(x, y int, style lipgloss.Style, caret bool) {
		if y <= x {
			return
		}
		if caret && caretCell >= x && caretCell < y {
			if cell := ansi.Cut(plain, caretCell-first, caretCell-first+1); cell != "" {
				render(style, x, caretCell)
				b.WriteString(promptCaretStyle.Render(cell))
				render(style, caretCell+1, y)
				return
			}
		}
		render(style, x, y)
	}
	pos := first
	for _, p := range paints {
		segment(pos, p.a, base, true)
		segment(p.a, p.b, p.style, p.caret)
		pos = max(pos, p.b)
	}
	segment(pos, end, base, true)
	return b.String()
}

// sortPromptPaints orders paints left to right, which paintPromptSpans needs
// and the merge of spell marks with a selection does not give for free.
func sortPromptPaints(paints []promptPaint) {
	sort.Slice(paints, func(i, j int) bool { return paints[i].a < paints[j].a })
}
