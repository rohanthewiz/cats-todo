// menu.go — the box every context menu in this program is drawn in.
//
// There are two of them: the prompt editor's (promptmenu.go), which asks what a
// swept run of text is worth, and the list's (listmenu.go), which asks what can
// be done to the todo under the pointer. They differ only in what their rows
// mean; everything a menu *is* — where the box lands, how a click hits a row,
// how the keyboard walks it, how it draws and how it is composited over the
// screen it is asking about — is the same, and lives here.
//
// The shared piece is a value rather than an interface because a menu has no
// behaviour of its own to dispatch on: it is geometry plus a list of rows, and
// the two owners embed it so that mu.open, mu.cursor and mu.place() read the
// same at both call sites. What a pressed row *does* stays with the owner,
// where the state it acts on is.
//
//	╭──────────────────────────────╮
//	│ ✂ Split into prompts  ctrl+x │  ← a live row
//	│ ⇅ Sort lines                 │  ← live, but nothing worth teaching a chord for
//	│ ⌶ Caret on every line        │  ← dim: it says why when pressed
//	╰──────────────────────────────╯
//
// The dim-rather-than-omit rule is the shared design decision and the reason
// `why` sits on the item instead of the row simply being left off: a menu whose
// contents move between presses is a menu nobody learns the shape of, and "why
// is this one grey" is a question the program can answer — "where did that item
// go" is not.

package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// menuItem is one row. why non-empty means the item cannot act right now: it is
// drawn dim, and pressing it says this instead of doing nothing.
//
// act is the owner's own action constant, carried rather than inferred from the
// row index so that a row can be moved in the slice without silently changing
// what it runs.
type menuItem struct {
	act   int
	label string
	hint  string
	why   string
}

func (it menuItem) live() bool { return it.why == "" }

// menuBox is an open menu: where its box sits, what is on it, and which row the
// keyboard is on. The zero value is a closed menu, which is what makes `.open`
// the one field every caller has to test.
type menuBox struct {
	open   bool
	x, y   int // top-left cell of the box, in screen coordinates
	w, h   int // its size, kept so a click can be hit-tested against it
	items  []menuItem
	cursor int
}

// menuChromeWidth is the box's border and inner padding: one border cell and one
// space on each side, plus the two spaces between a label and its chord.
const menuChromeWidth = 6

// firstLive is the row a menu opens on: the first one that can actually act. A
// menu that opened with the cursor on a dim row would make enter — the key a
// hand reaches for straight after the click — a refusal.
func (b menuBox) firstLive() int {
	for i, it := range b.items {
		if it.live() {
			return i
		}
	}
	return 0
}

// size measures the box around its widest row.
func (b *menuBox) size() {
	w := 0
	for _, it := range b.items {
		w = max(w, lipgloss.Width(it.label)+lipgloss.Width(it.hint))
	}
	b.w, b.h = w+menuChromeWidth, len(b.items)+2 // +2 for the border rows
}

// place puts the box below and right of the pointer, then pulls it back inside
// the pane.
//
// Below-right first because that is where a menu goes when there is room, and
// because it leaves the cell that was clicked — and whatever the question is
// about — visible rather than covered by the answer to it. When the box would
// fall off the bottom it flips above the pointer instead of being pushed up over
// it, which keeps the same cell uncovered on the other side.
func (b *menuBox) place(x, y, width, height int) {
	b.x, b.y = placeBelowRight(x, y, b.w, b.h, width, height)
}

// placeBelowRight is that rule on its own, so the list's hover card
// (listhover.go) lands the way a menu does without being one. A floating box is
// a floating box: whatever the user learned about where the answer to a press
// appears should hold for where the answer to a hover appears too.
func placeBelowRight(x, y, w, h, width, height int) (int, int) {
	bx := min(max(x, 0), max(width-w, 0))
	by := y + 1
	if by+h > height {
		by = y - h
	}
	return bx, min(max(by, 0), max(height-h, 0))
}

// hit reports which row a click at (x, y) landed on. The border is not a row, so
// a click on it is inside the menu (it does not dismiss) but presses nothing.
func (b menuBox) hit(x, y int) (int, bool) {
	if x < b.x || x >= b.x+b.w || y < b.y || y >= b.y+b.h {
		return 0, false
	}
	row := y - b.y - 1 // the top border
	if row < 0 || row >= len(b.items) {
		return 0, false
	}
	return row, true
}

// inside reports whether a click landed anywhere on the box, border included —
// what decides whether a press dismisses the menu.
func (b menuBox) inside(x, y int) bool {
	return x >= b.x && x < b.x+b.w && y >= b.y && y < b.y+b.h
}

// What a keystroke on an open menu turned out to mean. The owner acts on
// menuKeyPress and takes the box down on menuKeyClose; menuKeyMoved is already
// applied to the cursor by the time it is returned.
const (
	menuKeyMoved = iota
	menuKeyPress
	menuKeyClose
)

// key walks the cursor and classifies the keystroke.
//
// A menu owns every key while it is up: anything that is not a move or a press
// closes it and is swallowed, which is what a menu does everywhere else — the
// next keystroke after that reaches the screen underneath as usual. ctrl+c is
// deliberately not special-cased into a quit: a menu is up, and the first thing
// that chord should do is take it down.
func (b *menuBox) key(msg tea.KeyPressMsg) int {
	if len(b.items) == 0 {
		return menuKeyClose
	}
	switch msg.String() {
	case "down", "ctrl+n", "tab":
		b.cursor = (b.cursor + 1) % len(b.items)
	case "up", "ctrl+p", "shift+tab":
		b.cursor = (b.cursor - 1 + len(b.items)) % len(b.items)
	case "home":
		b.cursor = 0
	case "end":
		b.cursor = len(b.items) - 1
	case "enter", " ":
		return menuKeyPress
	default:
		return menuKeyClose
	}
	return menuKeyMoved
}

// render draws the box.
//
// The cursor's row takes the accent field the action bars use for a pressed
// chip, so "where the keyboard is" reads the same on a menu as it does
// everywhere else in the program. A dim row goes grey in colFaint but keeps its
// chord: the chord is still that action's chord, and it refuses in the same
// words the row does — hiding it would only make the greyed row look like a
// thing with no keyboard road at all.
func (b menuBox) render() string {
	inner := b.w - 2 // less the border columns
	rows := make([]string, 0, len(b.items))
	for i, it := range b.items {
		gap := inner - 2 - lipgloss.Width(it.label) - lipgloss.Width(it.hint)
		text := " " + it.label + strings.Repeat(" ", max(gap, 0)) + it.hint + " "
		switch {
		case i == b.cursor:
			rows = append(rows, menuRowSelStyle.Render(text))
		case !it.live():
			rows = append(rows, menuRowOffStyle.Render(text))
		default:
			rows = append(rows, menuRowStyle.Render(text))
		}
	}
	return menuBoxStyle.Render(strings.Join(rows, "\n"))
}

// overlayMenu composites an open menu over the stage's rendered frame.
//
// The menu is drawn over its screen rather than replacing it, because a context
// menu that hid its own context would be asking about something the user can no
// longer see. lipgloss's compositor does the merge cell by cell, so the frame
// keeps its own styling everywhere the box does not cover and the box keeps its
// own where it does. Doing this by cutting and re-joining the rendered lines —
// the technique the prompt's selection overlay uses inside one editor line —
// would work only as far as the first escape sequence the frame opened left of
// the box's left edge.
func overlayMenu(view string, b menuBox) string {
	if !b.open {
		return view
	}
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(view),
		lipgloss.NewLayer(b.render()).X(b.x).Y(b.y).Z(1),
	).Render()
}
