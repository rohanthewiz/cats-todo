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
//	╰──────────────────────────────╯
//
// It is built fresh on every press, from what the press was actually aimed at:
// an item that cannot act on this selection is drawn dim and says why when it is
// pressed, rather than being left off the menu. A menu whose contents move
// between presses is a menu nobody learns the shape of, and "why is this one
// grey" is a question the program can answer — "where did that item go" is not.
//
// The menu is drawn over the form rather than replacing it (see
// overlayPromptMenu), because a context menu that hid its own context would be
// asking about a selection the user can no longer see. It is composited with
// lipgloss's canvas, which merges cell by cell and keeps the styles on both
// sides — a splice done by hand on the rendered strings would drop the escape
// runs the form opened left of the box.

package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	menuActionCount
)

// promptMenuItem is one row. why non-empty means the item cannot act right now:
// it is drawn dim, and pressing it says this instead of doing nothing.
type promptMenuItem struct {
	act   int
	label string
	hint  string
	why   string
}

func (it promptMenuItem) live() bool { return it.why == "" }

// promptMenu is the open menu: where its box sits, what is on it, and which row
// the keyboard is on. The zero value is a closed menu, which is what makes
// `m.menu.open` the one thing every caller has to test.
type promptMenu struct {
	open          bool
	x, y          int // top-left cell of the box, in screen coordinates
	w, h          int // its size, kept so a click can be hit-tested against it
	items         []promptMenuItem
	cursor        int
	word          spell.Span // the flagged word ✓ Spelling would open on
	wordAvailable bool
}

// menuChromeWidth is the box's border and inner padding: one border cell and one
// space on each side, plus the two spaces between a label and its chord.
const menuChromeWidth = 6

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
	mu.items = []promptMenuItem{
		{act: menuSplit, label: "✂ Split into prompts", hint: "ctrl+x", why: selWhy},
		{act: menuSort, label: "⇅ Sort lines", why: rowsWhy},
		{act: menuCarets, label: "⌶ Caret on every line", why: rowsWhy},
		{act: menuSpell, label: "✓ Spelling…", hint: "ctrl+l"},
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

// firstLive is the row the menu opens on: the first one that can actually act.
// A menu that opened with the cursor on a dim row would make enter — the key a
// hand reaches for straight after the click — a refusal.
func (mu promptMenu) firstLive() int {
	for i, it := range mu.items {
		if it.live() {
			return i
		}
	}
	return 0
}

// size measures the box around its widest row.
func (mu *promptMenu) size() {
	w := 0
	for _, it := range mu.items {
		w = max(w, lipgloss.Width(it.label)+lipgloss.Width(it.hint))
	}
	mu.w, mu.h = w+menuChromeWidth, len(mu.items)+2 // +2 for the border rows
}

// place puts the box below and right of the pointer, then pulls it back inside
// the pane.
//
// Below-right first because that is where a menu goes when there is room, and
// because it leaves the cell that was clicked — and the selection around it —
// visible rather than covered by the answer to the question about it. When the
// box would fall off the bottom it flips above the pointer instead of being
// pushed up over it, which keeps the same cell uncovered on the other side.
func (mu *promptMenu) place(x, y, width, height int) {
	mu.x = min(max(x, 0), max(width-mu.w, 0))
	mu.y = y + 1
	if mu.y+mu.h > height {
		mu.y = y - mu.h
	}
	mu.y = min(max(mu.y, 0), max(height-mu.h, 0))
}

// hit reports which row a click at (x, y) landed on. The border is not a row, so
// a click on it is inside the menu (it does not dismiss) but presses nothing.
func (mu promptMenu) hit(x, y int) (int, bool) {
	if x < mu.x || x >= mu.x+mu.w || y < mu.y || y >= mu.y+mu.h {
		return 0, false
	}
	row := y - mu.y - 1 // the top border
	if row < 0 || row >= len(mu.items) {
		return 0, false
	}
	return row, true
}

// inside reports whether a click landed anywhere on the box, border included —
// what decides whether a press dismisses the menu.
func (mu promptMenu) inside(x, y int) bool {
	return x >= mu.x && x < mu.x+mu.w && y >= mu.y && y < mu.y+mu.h
}

// --- Driving it -----------------------------------------------------------------

// updatePromptMenu is the keyboard while the menu is up. It owns every key:
// anything that is not a move or a press closes the menu and is swallowed, which
// is what a menu does everywhere else — the next keystroke after that reaches the
// editor as usual.
func (m model) updatePromptMenu(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "down", "ctrl+n", "tab":
		m.menu.cursor = (m.menu.cursor + 1) % len(m.menu.items)
		return m, nil
	case "up", "ctrl+p", "shift+tab":
		m.menu.cursor = (m.menu.cursor - 1 + len(m.menu.items)) % len(m.menu.items)
		return m, nil
	case "home":
		m.menu.cursor = 0
		return m, nil
	case "end":
		m.menu.cursor = len(m.menu.items) - 1
		return m, nil
	case "enter", " ":
		return m.pressPromptMenu(m.menu.cursor)
	}
	// esc and everything else: the menu goes, and the key is spent on closing
	// it. ctrl+c is deliberately not special-cased into a quit here — a menu is
	// up, and the first thing that chord should do is take it down.
	m.menu = promptMenu{}
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
	}
	return m, nil
}

// --- Drawing --------------------------------------------------------------------

// overlayPromptMenu composites the menu over the form's rendered frame.
//
// lipgloss's compositor does the merge cell by cell, so the form keeps its own
// styling everywhere the box does not cover and the box keeps its own where it
// does. Doing this by cutting and re-joining the rendered lines — the technique
// the selection overlay uses inside one editor line — would work only as far as
// the first escape sequence the form opened left of the box's left edge.
func (m model) overlayPromptMenu(view string) string {
	if !m.menu.open {
		return view
	}
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(view),
		lipgloss.NewLayer(m.viewPromptMenu()).X(m.menu.x).Y(m.menu.y).Z(1),
	).Render()
}

// viewPromptMenu renders the box.
//
// The cursor's row takes the accent field the action bars use for a pressed
// chip, so "where the keyboard is" reads the same on this menu as it does
// everywhere else in the program. A dim row is drawn in colFaint with its chord
// dropped: an item that cannot act has no key worth teaching.
func (m model) viewPromptMenu() string {
	inner := m.menu.w - 2 // less the border columns
	rows := make([]string, 0, len(m.menu.items))
	for i, it := range m.menu.items {
		gap := inner - 2 - lipgloss.Width(it.label) - lipgloss.Width(it.hint)
		text := " " + it.label + strings.Repeat(" ", max(gap, 0)) + it.hint + " "
		switch {
		case i == m.menu.cursor:
			rows = append(rows, menuRowSelStyle.Render(text))
		case !it.live():
			rows = append(rows, menuRowOffStyle.Render(text))
		default:
			rows = append(rows, menuRowStyle.Render(text))
		}
	}
	return menuBoxStyle.Render(strings.Join(rows, "\n"))
}
