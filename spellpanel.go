// spellpanel.go — the Spelling panel behind ctrl+l in the prompt editor.
//
// The checker underlines what it does not know (spell.go); this is where a
// underline turns into a decision. The panel opens on the flagged word nearest
// the caret and offers the three answers there are to one:
//
//   - it was a typo — pick the spelling that was meant, and the editor's text
//     is corrected in place;
//   - it was a word, just not one the list has heard of — add it to a
//     dictionary of your own, globally or beside this project's backlog, and it
//     stops being flagged everywhere from now on;
//   - none of the above, and the underlines are in the way — turn the check off
//     without leaving the panel.
//
// It is a full-screen sub-stage of the form, like the attachment editor, the
// session panel and the file picker. The form has no floating overlays, and the
// alternative — a popup anchored under the word — would have to dodge the pane's
// edges, the toolbar and the caret line to show a list whose length is not known
// until it is built.
//
// One panel rather than three chords is the whole point of the shape. Spelling
// is not a thing anyone does often enough to memorize three keys for, so the
// feature gets one key that opens a screen naming everything it can do, and the
// screen is where the choice is made. ctrl+l is that key; the ☑ Spell chip on
// the toolbar is the pointer's way to the toggle, and the panel's last row is
// the keyboard's.
//
// There are two ways in, and they differ only in which word the panel is about.
// ctrl+l takes the flagged word nearest the caret, which is a guess — a good one
// in a prompt with one mistake in it, and a guess all the same in a prompt with
// three. ✓ Spelling… on the editor's context menu (spellAskAt) names the word
// outright, and opens the panel with the ✚ Add row already highlighted, since a
// hand that points at a squiggle and asks for a menu is usually about to say
// "that is a word".
package main

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/rohanthewiz/cats-todo/internal/spell"
)

// spellChoiceKind is what a row of the panel does when it is chosen.
type spellChoiceKind int

const (
	// spellFix replaces the flagged word in the prompt with word.
	spellFix spellChoiceKind = iota
	// spellAdd writes word into the dictionary file at path and teaches the
	// loaded dictionary the same word, so the mark clears without a reload.
	spellAdd
	// spellToggle turns the check off or on. It is on the panel as well as the
	// toolbar because the panel is what a keyboard reaches, and because someone
	// who has just been shown a word they meant to type is exactly the person
	// who wants the underlines to stop.
	spellToggle
)

// spellChoice is one selectable row: what it does, the word it does it to, and
// — for an add — the file it writes to and the name to call that file by
// afterwards. The name is carried rather than derived from the path because it
// is what the form's note line says, and a note is read at a glance: "added to
// this project's dictionary" answers the question, where the path it went to is
// something the row that was pressed already showed.
type spellChoice struct {
	kind  spellChoiceKind
	word  string
	path  string
	where string
}

// spellSuggestions is how many corrections the panel offers. It is the
// package's own cap (spell.suggestMax): past the first handful the candidates
// are all at the far edge of the distance, and a list that has to be read
// rather than glanced at is not a suggestion any more.
const spellSuggestions = 8

// beginSpell is ctrl+l: open the panel on whatever the caret is nearest. The
// pick is cleared first because this entry point has no word of its own —
// leaving a right-click's target standing would answer a keystroke aimed at the
// caret with the word some earlier click pointed at.
func (m model) beginSpell() (tea.Model, tea.Cmd) {
	m.spellPick, m.spellPicked = spell.Span{}, false
	return m.openSpellPanel()
}

// openSpellPanel opens the panel over the form. The form's own inputs are blurred
// so only one cursor blinks — the shape beginImages and beginSession keep — and
// closeSpell hands the focus back to whichever field had it.
func (m model) openSpellPanel() (tea.Model, tea.Cmd) {
	m.titleInput.Blur()
	m.promptArea.Blur()
	m.spellList = newFuzzyList("Type to filter…", nil)
	m.refreshSpell()
	m.stage = stageSpell
	return m, textinput.Blink
}

// refreshSpell rebuilds the panel's rows from the state of the moment: which
// word is being corrected, what the dictionary suggests for it, and whether the
// check is on. It runs on open and again after the toggle row is pressed, which
// is what lets turning the check on from inside the panel fill it with the
// suggestions the panel could not offer a keystroke earlier.
//
// The flagged word is resolved once, here, and remembered as a rune span. The
// panel is modal — the prompt cannot be edited while it is up — so the span
// stays true until it is applied, and resolving it once means the row that says
// "wrods" and the text that gets replaced cannot come from two different reads.
func (m *model) refreshSpell() {
	m.spellChoices = nil
	m.spellWord, m.spellSpan = "", spell.Span{}

	var items []listItem
	// row adds a selectable row and the choice it stands for, keeping the two
	// in step: a row's ref is its index into spellChoices.
	row := func(c spellChoice, it listItem) {
		it.ref = len(m.spellChoices)
		it.selectable = true
		m.spellChoices = append(m.spellChoices, c)
		items = append(items, it)
	}
	gap := func() {
		if len(items) > 0 {
			items = append(items, listItem{})
		}
	}

	// The ref of the first ✚ Add row, so a right-click's panel can open on it
	// (see below). -1 while there is no such row — no word, or no dictionary
	// file to write to.
	addRef := -1
	if sp, word, ok := m.spellPanelTarget(); ok {
		m.spellSpan, m.spellWord = sp, word
		for _, s := range m.spellDict.Suggest(word, spellSuggestions) {
			row(spellChoice{kind: spellFix, word: s}, listItem{name: s})
		}
		gap()
		// Both dictionaries are offered whenever both exist, rather than one
		// being chosen for the user: "this is a word" and "this is a word *here*"
		// are different claims, and only the person typing knows which they mean.
		// The project's file is second because the global one is the safer
		// default — it commits nothing to a repo.
		addRow := func(p, where string) {
			if addRef < 0 {
				addRef = len(m.spellChoices)
			}
			label := "✚ Add “" + word + "” to " + where
			row(spellChoice{kind: spellAdd, word: word, path: p, where: where},
				listItem{name: label, desc: m.spellPathNote(label, p)})
		}
		if p := m.spellDictGlobalPath(); p != "" {
			addRow(p, "my dictionary")
		}
		if p := m.spellDictProjectPath(); p != "" {
			addRow(p, "this project's dictionary")
		}
	}

	gap()
	// The toggle's glyph and words describe what pressing it will leave behind,
	// not what is true now — a row that reads "☑ Spell check is on" would be a
	// statement, and a row in a list of actions has to be an instruction.
	toggle := listItem{name: "☐ Turn spell check off", desc: "remembered across launches"}
	if !m.spellOn {
		toggle = listItem{name: "☑ Turn spell check on", desc: "remembered across launches"}
	}
	row(spellChoice{kind: spellToggle}, toggle)

	m.spellList.setItems(items)
	// A right-click on an underlined word is a claim about the word — "this is
	// spelled fine, you just don't know it" — so the panel it opens starts on
	// the answer to that claim, and enter is the whole of the gesture. The
	// suggestions stay one ↑ away for the click that turns out to have been a
	// typo after all, and the project's dictionary one ↓ away, which is the
	// choice this panel exists to keep offering (see addRow).
	if m.spellPicked && addRef >= 0 {
		m.spellList.selectRef(addRef)
	}
}

// spellPanelTarget is the word the panel is about: the one a right-click
// pointed at when the panel was opened that way, and otherwise the flagged word
// nearest the caret.
//
// A picked span is re-read against the value here rather than trusted, even
// though the panel is modal and the text cannot change under it: the span is
// carried across an open, and a range used to slice runes is worth bounding
// where it is used. A span that no longer fits reports no word at all, which
// the panel already knows how to draw.
func (m model) spellPanelTarget() (spell.Span, string, bool) {
	if !m.spellPicked {
		return m.promptSpellTarget()
	}
	runes := []rune(m.promptArea.Value())
	sp := m.spellPick
	if sp.Start < 0 || sp.Start >= sp.End || sp.End > len(runes) {
		return spell.Span{}, "", false
	}
	return sp, string(runes[sp.Start:sp.End]), true
}

// spellPathNote is the file an add row writes to, drawn beside its label and
// cut to what is left of the pane. The cut is from the front, the way a heading
// shortens a directory (see truncateLeft): the end of a path is the part that
// says which file it is, and a config directory's first few segments are the
// same on every machine anyway.
//
// It has to be cut rather than allowed to run: fuzzyList draws one line per row
// and clickSpell counts on that, so a row long enough to wrap would put every
// row below it one line off from where the pointer looks for it.
func (m model) spellPathNote(label, path string) string {
	note := shortenHome(path)
	if m.width <= 0 {
		return note // not sized yet: the first frame renders before the terminal says
	}
	room := m.width - lipgloss.Width(label) - 4 // the cursor gutter, and the gap before the note
	if room < 8 {
		return "" // no honest room for it; the row's words are the part that matters
	}
	return truncateLeft(note, room)
}

// promptSpellTarget is the flagged word the panel opens on, as a rune span into
// the editor's value and the word itself.
//
// The word under the caret comes first — someone who has just typed a
// misspelling and reached for the key means that one, and it is the one word
// the underline deliberately does not mark (see promptSpellSpans), so the panel
// is also how a half-typed word gets looked at. Failing that, the nearest one
// behind the caret: prose is written left to right, and the mistake you have
// noticed is almost always one you have already passed. Only when there is
// nothing behind does it look forward.
func (m model) promptSpellTarget() (spell.Span, string, bool) {
	if !m.spellOn || m.spellDict == nil {
		return spell.Span{}, "", false
	}
	value := m.promptArea.Value()
	spans := m.spellDict.Check(value)
	if len(spans) == 0 {
		return spell.Span{}, "", false
	}
	runes := []rune(value)
	word := func(sp spell.Span) (spell.Span, string, bool) {
		return sp, string(runes[sp.Start:min(sp.End, len(runes))]), true
	}
	caret := promptCaretOffset(m.promptArea)
	for _, sp := range spans {
		if sp.Start <= caret && caret <= sp.End {
			return word(sp)
		}
	}
	for i := len(spans) - 1; i >= 0; i-- {
		if spans[i].End < caret {
			return word(spans[i])
		}
	}
	return word(spans[0])
}

// spellAskAt answers what ✓ Spelling… on the editor's context menu can do about
// the cell the pointer is on: the flagged word there, whether there is one, and
// — when there is not — which of the two reasons applies.
//
// It is asked once, when the menu is built, and the answer is what both draws
// the row and runs it (see openPromptMenu). Resolving it twice would let a menu
// that offered the word and a press that opened the panel disagree about which
// word, which is the one thing a gesture aimed at a particular word must not do.
//
// The two ways it can come back empty are worth telling apart rather than
// collapsing into "not available": the check being off is a state the user can
// change, and nothing being flagged there is a miss.
//
// The word under the caret is included, though it is the one word the underline
// deliberately does not mark (see promptSpellSpans) — the pointer is aimed at a
// word, not at a mark, and refusing the word being typed would be a rule with no
// visible cause.
func (m model) spellAskAt(x, row int) (spell.Span, bool, string) {
	switch {
	case !m.spellOn || m.spellDict == nil:
		return spell.Span{}, false, "spell check is off — the ☑ Spell chip turns it on"
	case row < 0 || row >= m.promptArea.Height():
		return spell.Span{}, false, "point at a word in the prompt"
	}
	sp, _, ok := m.spellWordAt(x, row)
	if !ok {
		return spell.Span{}, false, "nothing the dictionary questions under the pointer"
	}
	return sp, true, ""
}

// openSpellPanelOn opens the panel on a word the pointer named, rather than on
// the one nearest the caret.
//
// Only the span is carried in; the panel re-reads the word from it (see
// spellPanelTarget), so the row that names a word and the text a correction
// replaces still come from one read of the value. spellPicked also decides which
// row opens highlighted: pointing at a word is usually a claim that it *is* a
// word, so the ✚ Add row leads.
func (m model) openSpellPanelOn(sp spell.Span) (tea.Model, tea.Cmd) {
	// A press is a press: the standing selection goes, for the reason clickForm
	// drops it on the left button — a highlight that outlived the click would
	// misreport what ctrl+c copies, and a correction applied from the panel
	// would move the text out from under it besides.
	m.clearPromptSel()
	m.formNote = ""
	m.spellPick, m.spellPicked = sp, true
	// The keys follow the pointer, the way every other click on the form moves
	// the focus to what was clicked: a correction lands in the prompt, so the
	// prompt is where the focus should be when the panel hands it back. Set
	// directly rather than through focusForm because openSpellPanel blurs both
	// inputs on the next line — focusing a field only to blur it would be noise.
	m.formFocus = formFieldPrompt
	return m.openSpellPanel()
}

// spellWordAt is the word the dictionary does not know at the pointer's cell, as
// a rune span into the editor's value and the word itself.
//
// The checker is run over the whole value rather than over the clicked word
// alone, so what counts as a word here is exactly what counts as one for the
// underline — the same tokenizer, the same handling of apostrophes and hyphens
// and code. Picking a "word" out with local rules would give a gesture that
// selects something slightly different from what is drawn as flagged, which is
// the one thing this must not do.
func (m model) spellWordAt(x, row int) (spell.Span, string, bool) {
	if m.spellDict == nil {
		return spell.Span{}, "", false
	}
	off, ok := promptOffsetAt(m.promptArea, x, row)
	if !ok {
		return spell.Span{}, "", false
	}
	value := m.promptArea.Value()
	runes := []rune(value)
	for _, sp := range m.spellDict.Check(value) {
		// End is exclusive, so a click one cell past the last letter — the gap
		// after the word, or the blank tail of a short line — is not on it.
		if sp.Start <= off && off < sp.End && sp.End <= len(runes) {
			return sp, string(runes[sp.Start:sp.End]), true
		}
	}
	return spell.Span{}, "", false
}

// updateSpell is the panel's key loop — the export picker's shape: esc back,
// arrows move, enter presses the row. ctrl+l closes it as well as esc, so the
// key that opened the panel also dismisses it, which is what a hand that has
// just pressed it expects to be able to press again.
func (m model) updateSpell(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "ctrl+l":
		return m.closeSpell()
	case "up", "ctrl+p":
		m.spellList.moveUp()
		return m, nil
	case "down", "ctrl+n":
		m.spellList.moveDown()
		return m, nil
	case "enter":
		return m.chooseSpell()
	}
	return m, m.spellList.editQuery(msg)
}

// chooseSpell acts on the highlighted row. A correction or an add is done and
// the panel closes, reporting on the form's note line; the toggle stays, since
// turning the check on is a step towards doing something here rather than the
// thing itself.
func (m model) chooseSpell() (tea.Model, tea.Cmd) {
	idx := m.spellList.selectedIndex()
	if idx < 0 || idx >= len(m.spellChoices) {
		return m, nil
	}
	c := m.spellChoices[idx]
	switch c.kind {
	case spellFix:
		return m.applySpellFix(c.word)
	case spellAdd:
		return m.addToDictionary(c)
	case spellToggle:
		next, err := m.toggleSpell()
		m = next
		m.refreshSpell()
		// toggleSpell writes its note to the form's message line, which this
		// panel does not draw — so it simply waits there for the panel to close.
		// A failure to persist the choice is the one thing that cannot wait: it
		// is repeated on the panel's own line, where it is on screen now.
		m.spellErr = ""
		if err != nil {
			m.spellErr = m.formNote
		}
		return m, nil
	}
	return m, nil
}

// applySpellFix puts the chosen spelling into the prompt in place of the
// flagged word and leaves the caret just past it — where it would be if the
// word had been typed correctly in the first place, so the next keystroke
// carries on rather than landing somewhere the eye has to go looking for.
func (m model) applySpellFix(word string) (tea.Model, tea.Cmd) {
	was := m.spellWord
	m.replacePromptRunes(m.spellSpan.Start, m.spellSpan.End, word)
	next, cmd := m.closeSpell()
	m = next.(model)
	m.formNote = "“" + was + "” → “" + word + "”"
	return m, cmd
}

// addToDictionary writes the word to one of the user's own lists and teaches
// the loaded dictionary the same word, so the underline goes as the panel does
// rather than at the next launch.
//
// The file is written first. Adding to the loaded set on a write that failed
// would leave a word accepted for this run and forgotten by the next, which is
// the one outcome nobody could explain — the underline would come back on a
// word they watched the program accept.
func (m model) addToDictionary(c spellChoice) (tea.Model, tea.Cmd) {
	if err := spell.AppendWord(c.path, c.word); err != nil {
		m.spellErr = "could not add “" + c.word + "”: " + err.Error()
		return m, nil
	}
	if m.spellDict != nil {
		m.spellDict.Add(c.word)
	}
	next, cmd := m.closeSpell()
	m = next.(model)
	m.formNote = "added “" + c.word + "” to " + c.where
	return m, cmd
}

// closeSpell returns to the form, restoring focus to the field that had it —
// the contract closeImages and closeSession keep, and for the same reason: a
// sub-stage that gives the keys back somewhere else makes the form feel like it
// moved.
func (m model) closeSpell() (tea.Model, tea.Cmd) {
	m.spellErr = ""
	m.spellPick, m.spellPicked = spell.Span{}, false
	m.stage = stageForm
	return m, m.restoreFormFocus()
}

// replacePromptRunes swaps the editor's runes [start, end) for with, and leaves
// the caret at the end of what was inserted.
//
// The value is rebuilt and set rather than edited through the library, because
// the library has no "replace this range": it edits at the caret, and driving it
// there would mean walking the caret to the end of the word and sending as many
// backspaces as the word is long — the same result by a longer road, and one
// that goes wrong quietly if the walk lands a character off.
func (m *model) replacePromptRunes(start, end int, with string) {
	runes := []rune(m.promptArea.Value())
	start = min(max(start, 0), len(runes))
	end = min(max(end, start), len(runes))
	m.promptArea.SetValue(string(runes[:start]) + with + string(runes[end:]))
	setPromptCaretOffset(&m.promptArea, start+len([]rune(with)))
}

// setPromptCaretOffset moves the editor's caret to an absolute rune offset into
// its value — the inverse of promptCaretOffset, which the textarea does not
// offer either.
//
// The offset is turned into a logical row and a column within it, and the caret
// is then walked down to that row. It has to be walked: the library exposes
// SetCursorColumn but nothing that sets the row, and CursorDown steps one
// *display* line, so a soft-wrapped row takes several steps to cross. The loop
// therefore watches the row the caret reports rather than counting steps, and
// stops when a step moves nothing — which is what the last display line does.
func setPromptCaretOffset(ta *textarea.Model, off int) {
	rows := strings.Split(ta.Value(), "\n")
	row, col := 0, max(off, 0)
	for row < len(rows)-1 && col > len([]rune(rows[row])) {
		col -= len([]rune(rows[row])) + 1
		row++
	}
	ta.MoveToBegin()
	// The bound is the backstop promptLines uses, and for the same reason: it is
	// a guard against a walk that neither advances nor repeats, not a limit any
	// real prompt reaches.
	for range 20000 {
		if ta.Line() >= row {
			break
		}
		atRow, atCol := ta.Line(), ta.LineInfo().StartColumn
		ta.CursorDown()
		if ta.Line() == atRow && ta.LineInfo().StartColumn == atCol {
			break
		}
	}
	ta.SetCursorColumn(col)
}

// spellRowsRow is the first line the panel's rows are drawn on: the heading (0),
// a blank (1), the filter line (2), a blank (3), then the rows — the geometry
// viewExport and the drop picker share, since the chrome above the list is the
// same. TestSpellRowsMatchWhatIsDrawn pins it to a real frame.
const spellRowsRow = 4

// clickSpell is the pointer on the panel: a click on a row presses it, the way a
// click in the export picker chooses a destination. Every row here is either
// undoable (a correction, by the editor's own undo-by-retyping) or a preference,
// so there is nothing a stray click can do that a second one cannot walk back.
func (m model) clickSpell(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	i, ok := m.spellList.rowAtLine(msg.Y - spellRowsRow)
	if !ok || !m.spellList.focusRow(i) {
		return m, nil
	}
	return m.chooseSpell()
}

// viewSpell draws the panel: a heading naming the word under repair, the rows,
// and a footer of keys.
func (m model) viewSpell() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Spelling"))
	b.WriteString("  ")
	b.WriteString(descStyle.Render(m.spellHeadingNote()))
	b.WriteString("\n\n")
	b.WriteString(m.spellList.view("nothing matches — clear the filter", "", m.width))
	b.WriteString("\n")
	if m.spellErr != "" {
		b.WriteString(errStyle.Render(m.spellErr))
		b.WriteString("\n")
	}
	// The last segment teaches the way back in rather than a key of this panel:
	// someone who reached here by ctrl+l and had to walk to their word is
	// exactly the person the pointer's path is for, and the panel is the only
	// screen where saying so lands on a reader who already has the problem. It
	// is last because it is the one segment that is not about the screen it is
	// printed on, and so the right one for a narrow pane to give up first.
	b.WriteString(footerStyle.Render(m.fitFooter([]string{
		"enter apply", "↑/↓ move", "type to filter", "esc back", "right-click a word to open this on it",
	})))
	return b.String()
}

// spellHeadingNote is the line beside the panel's title: the word being
// corrected, or why there is not one. The three states are worth spelling out —
// "no suggestions" and "the check is off" look identical from a list with only
// a toggle row in it, and the difference is exactly what the user needs to know.
func (m model) spellHeadingNote() string {
	switch {
	case !m.spellOn:
		return "the check is off"
	case m.spellDict == nil:
		return "no dictionary loaded"
	case m.spellWord == "":
		return "nothing misspelled near the caret"
	}
	return "“" + m.spellWord + "”"
}
