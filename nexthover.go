// nexthover.go — the Next List page's hover card.
//
// A Next List row is the item's text flattened to one line and cut where the
// pane ends (see nextPage.rebuild), so everything past the edge — the rest of
// the paragraph, the sub-bullets, and the header fields the row never draws
// (when the item was raised, its value in words, which section it sits in) —
// was behind ✚ New prompt, which opens a form to answer a question that was
// only ever "what does this one say?".
//
// Resting the pointer on a row answers it the way the backlog's card does
// (listhover.go), with the same box, the same dwell and warm window, and the
// same teardown rules. What differs is only what is in it, and it is capped at
// nextCardMaxRows rows so it stays a glance rather than a reading pane:
//
//	╭──────────────────────────────────────────────────────────╮
//	│ N-001 · Open                                             │  ← ID and section
//	│ Hands-on pass in a rebuilt Cats.app. Merged from checks: │  ← the item's text,
//	│ - hover cards: the 400ms dwell;                          │    line breaks kept,
//	│ - DEC 1004: blur a window.                               │    wrapped, up to 5
//	│ value medium · raised 2026-0904-1753-a-dwell             │  ← the header's fields
//	╰──────────────────────────────────────────────────────────╯
//
// The state is the backlog card's own (m.hover, m.hoverPend, m.hoverGen,
// m.hoverWarmUntil) rather than a second copy: only one of the two pages is
// ever on screen, so only one card can ever be up, and the one-card rule the
// backlog's card follows holds across both for free. hoverPending carries the
// stage it was armed on, so a dwell that lands after the page changed is
// recognised as stale rather than read against the other page's rows.

package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The card's shape. Wider than the backlog's card (hoverCardWidth): an item is
// a paragraph of prose with no title to lean on, and each extra column is a
// few more words per line out of a fixed row budget. Still a preference — a
// narrow pane gets a narrower card, down to hoverCardMin.
//
// Seven rows in all is the most the card spends: one for the ID, five for the
// text, one for the fields. A short item spends fewer, since every part drops
// out when it has nothing to say.
const (
	nextCardWidth     = 62
	nextCardMaxRows   = 7
	nextCardBodyLines = nextCardMaxRows - 2 // minus the ID row and the fields row
)

// nextHoverMotion is the pointer moving with no button down over the Next List
// page — hoverMotion's twin, over this page's rows.
//
// The logic is the same three-way split (same row: nothing; the row already
// being waited for: follow the pointer; a new row: warm → card now, cold → arm
// the dwell), and is documented there. The page has no note pad or drag, so
// its context menu (nextmenu.go) is the one surface a card is refused over.
func (m model) nextHoverMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if m.nextMenu.open {
		// The menu has taken over the page the card floats on, so the warm
		// window closes with it, as hoverMotion's does under the list's menu.
		m.clearHover()
		return m, nil
	}
	i, ok := m.next.list.rowAtLine(msg.Y - nextRowsRow)
	if !ok {
		// A section heading, a spacer, or the chrome — the pointer crossing
		// the page, so the warm window stays open over it.
		m.hoverMovedOn()
		return m, nil
	}
	if m.hover.open && m.hover.row == i {
		return m, nil
	}
	if m.hoverPend.armed && m.hoverPend.row == i && m.hoverPend.stage == stageNextList {
		m.hoverPend.x, m.hoverPend.y = msg.X, msg.Y
		return m, nil
	}
	warm := m.hoverIsWarm() // asked before the teardown, which is what opens it
	m.hoverMovedOn()
	if warm {
		if card, ok := m.nextCardFor(i, msg.X, msg.Y); ok {
			m.hover = card
		}
		return m, nil
	}
	m.hoverGen++
	m.hoverPend = hoverPending{armed: true, stage: stageNextList, row: i, x: msg.X, y: msg.Y, gen: m.hoverGen}
	return m, hoverTick(m.hoverGen)
}

// nextCardFor resolves a filtered row of the page to its item and builds the
// card. The row is resolved at the moment the card appears, not when the
// pointer arrived, for hoverCardFor's reason: a ↻ Refresh can have rebuilt the
// rows in between.
func (m model) nextCardFor(row, x, y int) (hoverCard, bool) {
	idx, ok := m.next.list.refAt(row)
	if !ok || idx < 0 || idx >= len(m.next.items) {
		return hoverCard{}, false
	}
	const chrome = 4 // border + one space of padding each side, as buildHoverCard
	w := min(nextCardWidth, m.width-2)
	if w < hoverCardMin || m.height < 6 {
		return hoverCard{}, false
	}
	lines := nextCardLines(m.next.items[idx], w-chrome)
	card := hoverCard{open: true, row: row, lines: lines, w: w, h: len(lines) + 2}
	card.x, card.y = placeBelowRight(x, y, card.w, card.h, m.width, m.height)
	return card, true
}

// nextCardLines is the card's content for one item, never more than
// nextCardMaxRows rows.
//
// Unlike the backlog's card, an item always gets one, even when its whole text
// fits on the row: the row never shows when the item was raised or its value
// in words (only as a mark), and those are the two facts that decide whether
// it is the thing to pick up next.
//
// The ID leads for the reason the backlog card's title does — the box is
// placed off the row, among others, so it has to name which one it is about —
// and it carries the section, since the heading that says Open or Roadmap has
// usually scrolled out of sight by the time a long list is being read.
func nextCardLines(it nextItem, inner int) []string {
	var lines []string
	row := func(s string, style lipgloss.Style) {
		// Padded to the box's full interior so the card composites opaque
		// (see hoverLines).
		lines = append(lines, style.Width(inner+2).Render(s))
	}

	head := it.ID
	if it.Section != "" {
		head += " · " + it.Section
	}
	row(truncate(head, inner), hoverTitleStyle)

	for _, ln := range nextCardBody(it.Text, inner) {
		row(ln, hoverBodyStyle)
	}

	// The header's fields, in words, on one row: two short facts do not earn
	// a labelled table's two rows out of seven. Either drops out when the file
	// did not record it, and the row goes with them.
	var fields []string
	if it.Value != "" {
		fields = append(fields, "value "+it.Value)
	}
	if it.Raised != "" {
		fields = append(fields, "raised "+it.Raised)
	}
	if len(fields) > 0 {
		row(truncate(strings.Join(fields, " · "), inner), hoverFieldStyle)
	}
	return lines
}

// nextCardBody is an item's text as up to nextCardBodyLines wrapped lines,
// with an ellipsis on the last when there was more.
//
// Line breaks are kept, unlike on the row: an item's sub-bullets are the
// structure of the thought, and the card is the one place on the page with
// room to show them as bullets. Blank lines are dropped — a paragraph break is
// not worth one of five rows.
func nextCardBody(text string, inner int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var out []string
	// ansi.Wrap breaks on words and hard-breaks a word too long to fit, so a
	// long path or identifier cannot run off the card's right edge.
	for _, ln := range strings.Split(ansi.Wrap(text, inner, " -"), "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		if len(out) == nextCardBodyLines {
			// More text than rows: say so rather than simply stopping, which is
			// the difference between "that's all of it" and "press enter to
			// read the rest in the form".
			out[len(out)-1] = truncate(out[len(out)-1], max(inner-1, 1)) + "…"
			break
		}
		out = append(out, ln)
	}
	return out
}
