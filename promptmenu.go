// promptmenu.go — the prompt editor's context menu.
//
// A swept run of the prompt is worth several different things — split it into
// one prompt per bullet, sort it, put a caret on each of its lines — and none of
// them is guessable from a chord. The right button is where every editor on this
// machine keeps that list, so this is where it is kept here: sweep, right-click,
// and the menu names what can be done with what is under the pointer.
//
//	╭──────────────────────────────╮
//	│ ✂ Split into prompts  ctrl+x │
//	│ ⇅ Sort lines                 │
//	│ ⌶ Caret on every line        │
//	│ ✓ Spelling…           ctrl+l │
//	│ ≡ Insert a prompt…    ctrl+p │
//	╰──────────────────────────────╯
//
// It is built fresh on every press, from what the press was actually aimed at:
// an item that cannot act on this selection is drawn dim and says why when it is
// pressed, rather than being left off the menu.
//
// The box itself — its geometry, its keys, its drawing and the compositing that
// floats it over the form — is menuBox (menu.go), shared with the list's own
// context menu. What is left here is the part that is about a prompt: which rows
// there are, what makes each one live, and what pressing one does.

package main

import (
	tea "charm.land/bubbletea/v2"

	"github.com/rohanthewiz/cats-todo/internal/spell"
)

// The menu's actions, which are also its row order. The order is fixed rather
// than contextual for the reason the file header gives: a menu is learned by
// where its items sit.
const (
	menuSplit = iota
	menuSort
	menuCarets
	menuSpell
	// menuInsert opens the prompt library (promptpick.go). It is last because
	// it is the one row that is not about the text under the pointer — every
	// other item acts on what was swept or clicked, and this one brings text in
	// — and because it is always live: putting an always-live row first would
	// make it the default the keyboard lands on, ahead of the items the press
	// was almost certainly aimed at.
	menuInsert
	menuActionCount
)

// promptMenu is the open menu: the shared box, plus the one thing this menu
// carries that a generic one cannot — the flagged word ✓ Spelling would open on,
// resolved from the cell the press landed in rather than guessed from the caret.
// The zero value is a closed menu, which is what makes `m.menu.open` the one
// thing every caller has to test.
type promptMenu struct {
	menuBox
	word          spell.Span
	wordAvailable bool
}

// openPromptMenu builds the menu for a right-click at msg and opens it.
//
// Every item's availability is resolved here, once, from the state the press
// landed in — so what the menu draws and what pressing a row does cannot come
// from two different reads of the selection.
func (m model) openPromptMenu(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	var mu promptMenu
	mu.open = true

	// Split, sort and carets all read the selection; each says the first thing
	// that is wrong with it rather than a generic "not available".
	selWhy, rowsWhy := "sweep some text first", "sweep some text first"
	if lo, hi, ok := m.promptSelSpan(); ok {
		_, items := splitBulletList(string([]rune(m.promptArea.Value())[lo:hi]))
		selWhy = ""
		if len(items) == 0 {
			selWhy = "no bulleted list in the selection — items start with -, * or + (or 1.)"
		}
		first, last, _, _, _ := m.promptSelRows()
		rowsWhy = ""
		if first == last {
			rowsWhy = "sweep two or more lines"
		}
	}
	mu.items = []menuItem{
		{act: menuSplit, label: "✂ Split into prompts", hint: "ctrl+x", why: selWhy},
		{act: menuSort, label: "⇅ Sort lines", why: rowsWhy},
		{act: menuCarets, label: "⌶ Caret on every line", why: rowsWhy},
		{act: menuSpell, label: "✓ Spelling…", hint: "ctrl+l"},
		// Never dim: an empty library still opens a screen that says where
		// entries go, which is the answer someone with none actually needs.
		{act: menuInsert, label: "≡ Insert a prompt…", hint: "ctrl+p"},
	}

	// The spell row is the one item that is about the cell the pointer is on
	// rather than about the selection, so its answer comes from where the press
	// landed (spellAskAt, spellpanel.go) — that is what keeps the gesture naming
	// its own word instead of guessing one from the caret, which is the whole
	// reason it exists beside ctrl+l.
	mu.word, mu.wordAvailable, mu.items[menuSpell].why = m.spellAskAt(msg.X, msg.Y-formPromptRow)

	mu.cursor = mu.firstLive()
	mu.size()
	mu.place(msg.X, msg.Y, m.width, m.height)
	m.menu = mu
	m.formNote, m.formErr = "", ""
	return m, nil
}

// --- Driving it -----------------------------------------------------------------

// updatePromptMenu is the keyboard while the menu is up — the shared walk (see
// menuBox.key), with this menu's own answer to a press.
func (m model) updatePromptMenu(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.menu.key(msg) {
	case menuKeyPress:
		return m.pressPromptMenu(m.menu.cursor)
	case menuKeyClose:
		m.menu = promptMenu{}
	}
	return m, nil
}

// clickPromptMenu is the pointer while the menu is up: a press on a row presses
// it, a press on the box's border does nothing, and a press anywhere else
// dismisses without acting — the three things a click can mean on an open menu.
func (m model) clickPromptMenu(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if !m.menu.inside(msg.X, msg.Y) {
		m.menu = promptMenu{}
		return m, nil
	}
	row, ok := m.menu.hit(msg.X, msg.Y)
	if !ok {
		return m, nil
	}
	return m.pressPromptMenu(row)
}

// pressPromptMenu runs row i. The menu closes either way: an item that acted has
// nothing left to offer, and one that could not act has said why on the note
// line, which the menu would otherwise be covering.
func (m model) pressPromptMenu(i int) (tea.Model, tea.Cmd) {
	if i < 0 || i >= len(m.menu.items) {
		return m, nil
	}
	it := m.menu.items[i]
	word, hasWord := m.menu.word, m.menu.wordAvailable
	m.menu = promptMenu{}
	if !it.live() {
		m.formNote = it.why
		return m, nil
	}
	switch it.act {
	case menuSplit:
		return m.splitPromptList()
	case menuSort:
		return m.sortPromptLines()
	case menuCarets:
		return m.dropPromptCarets()
	case menuSpell:
		if !hasWord {
			return m, nil
		}
		// Straight into the panel on the word the press was aimed at, with the
		// ✚ Add row highlighted (openSpellPanelOn, spellpanel.go).
		return m.openSpellPanelOn(word)
	case menuInsert:
		// The same call the chord makes. Anything swept is still standing here —
		// the menu does not clear it — so the picker's ctrl+s can offer to save
		// exactly the run the right-click was aimed at.
		return m.beginSnippets(loadPromptLib(), snippetsAll)
	}
	return m, nil
}

// --- Drawing --------------------------------------------------------------------

// overlayPromptMenu floats the menu over the form's rendered frame (see
// overlayMenu, menu.go, for why it is composited rather than spliced).
func (m model) overlayPromptMenu(view string) string {
	return overlayMenu(view, m.menu.menuBox)
}
