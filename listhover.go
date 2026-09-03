// listhover.go — the list's hover card.
//
// A todo row is one line, so it can only ever show the prompt's first line (see
// firstLine): the body — the thing that decides whether this is the prompt to
// send right now — is behind ctrl+v or the edit form. That is a screen change
// to answer "what is this one again?", and the session setup a drop will run
// under (model, effort) was worse off still, hidden a panel deeper.
//
// Resting the pointer on a row answers both without leaving the list:
//
//	╭──────────────────────────────────────╮
//	│ Fix the drop timeout                 │  ← the todo's title
//	│ The 12s wait comes from stale ready  │  ← the prompt body, wrapped
//	│ probes — capture a startup and check  │
//	│ claudeReadyProbes before anything…   │  ← … when there is more of it
//	│                                      │
//	│ Model   claude-opus-5                │  ← only the fields that are set
//	│ Effort  high                         │
//	╰──────────────────────────────────────╯
//
// It is cats' own pane hover card (catway's `09-hovercard.js`) brought to the
// TUI, and follows its two rules: one card, reused, since only ever one can be
// visible; and label/value rows where an empty value drops out, so the card is
// as tall as this todo has things to say rather than a fixed form with blanks
// in it.
//
// Everything a floating box *is* — where it lands, how it is composited over
// the frame — is shared with the two context menus (menu.go). What it is not is
// a menu: nothing on it can be pressed, it takes no keys, and it is taken down
// by the next thing the hand does. That is why it carries its own type instead
// of another menuBox — a box with a cursor and rows that refuse to act would be
// teaching the wrong thing about every other box in the program.
//
// The pointer only reports idle motion while the list stage asks for
// MouseModeAllMotion (see View), which is the whole cost of this feature: a
// message per cell the pointer crosses, paid on one stage.

package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The card's shape. The width is a preference rather than a rule — a narrow
// pane gets a narrower card (down to hoverCardMin, under which there is no room
// to wrap prose into and the card simply does not appear).
//
// Four body lines is the most that can be read at a glance without the card
// becoming the prompt view with a border on it; a longer prompt says so with an
// ellipsis, which is also the invitation to press ctrl+v.
const (
	hoverCardWidth = 52
	hoverCardMin   = 28
	hoverBodyLines = 4
)

// hoverCard is the card as it currently stands: which row it was built for,
// where its box sits, and its already-rendered rows.
//
// The rows are rendered at build time rather than at draw time for the reason
// the menus resolve their labels at open time: the card describes the todo as it
// was when the pointer arrived, and a card that re-read the store on every frame
// would be a box that changed its mind under a hand that had not moved.
//
// row is kept so that motion *within* the row the card already describes is a
// no-op. Rebuilding on every cell would re-place the box under the pointer and
// the card would crawl sideways as the hand drifted — the card belongs to the
// row, not to the cell.
type hoverCard struct {
	open  bool
	row   int // the filtered-list index the card is describing
	x, y  int // top-left cell of the box, in screen coordinates
	w, h  int
	lines []string // the inner rows, each already padded to the card's width
}

// hoverMotion is the pointer moving with no button down over the list.
//
// Every reason not to have a card is answered here rather than at draw time, so
// the model never holds a card that should not be on screen: a menu is up (it
// owns the pointer, and a card floating over an open menu would be two answers
// to two different questions on top of each other), the flag's note pad is up
// (the same reason, and it is being typed into), a drag is in progress (the
// gesture is about where the row is going, not what is in it), or the pointer
// is on chrome rather than on a row.
func (m model) hoverMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if m.listMenu.open || m.flagPad.open || m.dragging {
		m.clearHover()
		return m, nil
	}
	i, ok := m.list.rowAtLine(msg.Y - listRowsRow)
	if !ok {
		m.clearHover() // a heading, a spacer, or the chrome above and below
		return m, nil
	}
	if m.hover.open && m.hover.row == i {
		return m, nil // still on the row the card is already about
	}
	idx, ok := m.list.refAt(i)
	if !ok || idx < 0 || idx >= len(m.rows) {
		m.clearHover()
		return m, nil
	}
	td, ok := m.resolve(m.rows[idx])
	if !ok {
		// On screen but no longer in the store — another pane deleted it since
		// the last rebuild. Nothing to say about it, and the next rebuild takes
		// the row away.
		m.clearHover()
		return m, nil
	}
	card, ok := m.buildHoverCard(td, i, msg.X, msg.Y)
	if !ok {
		m.clearHover()
		return m, nil
	}
	m.hover = card
	return m, nil
}

// clearHover takes the card down. Called from everywhere the card's premise
// stops holding — a keystroke (the hand is on the keyboard, so the pointer is
// not what the eye is following), a click, a resize, a rebuild — rather than
// only when the pointer leaves the row, because most of those never produce
// another motion message to notice.
func (m *model) clearHover() { m.hover = hoverCard{} }

// buildHoverCard renders the card for one todo and places it. false means there
// is nothing worth floating — no room in the pane, or a prompt with nothing in
// it beyond the title the row is already showing.
func (m model) buildHoverCard(td Todo, row, x, y int) (hoverCard, bool) {
	// The border and the one space of padding on each side, exactly as a menu
	// measures itself (menuChromeWidth counts the label/hint gap too, which a
	// card has no equivalent of).
	const chrome = 4
	w := min(hoverCardWidth, m.width-2)
	if w < hoverCardMin || m.height < 6 {
		return hoverCard{}, false
	}
	inner := w - chrome

	lines := hoverLines(td, inner)
	if len(lines) == 0 {
		return hoverCard{}, false
	}
	card := hoverCard{open: true, row: row, lines: lines, w: w, h: len(lines) + 2}
	card.x, card.y = placeBelowRight(x, y, card.w, card.h, m.width, m.height)
	return card, true
}

// hoverLines is the card's content: the title, the body, and the session's
// launch flags — each dropped when it has nothing to say, so the card is only
// ever as tall as this prompt earns.
//
// The title leads even though the row under the pointer is already showing it,
// because the card is placed *off* that row: with several rows within a cell or
// two of the box, the card has to name which one it is about. The body follows
// as prose, wrapped rather than truncated per line, since a prompt is written in
// sentences and the first four lines of it are the point of the whole card.
func hoverLines(td Todo, inner int) []string {
	var lines []string
	row := func(s string, style lipgloss.Style) {
		// Width pads every row out to the box's full interior — inner is the
		// text budget, the +2 is the space of padding on each side — so the card
		// composites as an opaque box. A short row left unpadded would let the
		// list's own text show through it, exactly as an unfilled menu row would.
		lines = append(lines, style.Width(inner+2).Render(s))
	}

	title := strings.TrimSpace(td.Title)
	if title == "" {
		title = firstLine(td.Prompt, inner)
	}
	row(truncate(title, inner), hoverTitleStyle)

	// The body is the prompt minus the line the row (and, above, the title) is
	// already showing — repeating it would spend the card's first line saying
	// what the pointer is already on.
	if body := hoverBody(td, inner); len(body) > 0 {
		for _, ln := range body {
			row(ln, hoverBodyStyle)
		}
	}

	// The flag's note, when it has one. This is the card's reason for carrying
	// anything other than the prompt's own text: the row draws ⚑ and the glyph
	// alone cannot say what it was about, so the words go where the pointer
	// already is rather than behind the edit form. A bare flag adds no row — the
	// mark on the row has already said everything there is.
	if td.Flag {
		if note := strings.TrimSpace(td.FlagNote); note != "" {
			row("", hoverBodyStyle) // the separator the fields below use too
			row(truncate(flagGlyph+" "+note, inner), hoverFieldStyle)
		}
	}

	// The session's launch flags — the two that decide what the receiving agent
	// *is*, and the pair a drop is most often reconsidered over. The rest of the
	// setup (context, reviews, wrap-up) is deliberately left to the ⚙ panel:
	// those are things the agent will do, and this card is about what is being
	// sent and to what.
	if td.Session != nil {
		labelled := func(label, value string) {
			if value == "" {
				return // an empty value drops its row, as it does on cats' card
			}
			pad := hoverLabelWidth - lipgloss.Width(label)
			text := label + strings.Repeat(" ", max(pad, 0)) + value
			row(truncate(text, inner), hoverFieldStyle)
		}
		if td.Session.Model != "" || td.Session.Effort != "" {
			row("", hoverBodyStyle) // the separator between the prose and the fields
			labelled("Model", td.Session.Model)
			labelled("Effort", td.Session.Effort)
		}
	}

	// A title-only todo with no session and no body has nothing the row is not
	// already saying, so it gets no card at all rather than a box repeating one
	// line back at the pointer.
	if len(lines) < 2 {
		return nil
	}
	return lines
}

// hoverLabelWidth is the column the field values line up on — "Effort" plus two
// spaces, the widest label the card has. Fixed rather than measured because the
// set is fixed: two labels, and a table that re-measures itself would move its
// values sideways depending on which fields a todo happened to set.
const hoverLabelWidth = 8

// hoverBody is the prompt as up to hoverBodyLines wrapped lines, with the title
// line dropped off the front and an ellipsis on the end when there was more.
//
// Blank lines are dropped rather than preserved: a prompt's paragraph breaks are
// worth their vertical space in a full-height view, and worth nothing in a card
// with four lines to spend.
func hoverBody(td Todo, inner int) []string {
	body := strings.TrimSpace(td.Prompt)
	if body == "" {
		return nil
	}
	// The card's heading is the title, and the title is the prompt's own first
	// line whenever the todo has no separate one (that is how the list draws the
	// row too). Either way that line has already been said at the top of the
	// card, so the body starts after it — a card whose first two lines are the
	// same sentence twice is a card that wasted half its height.
	head, rest, split := strings.Cut(body, "\n")
	if title := strings.TrimSpace(td.Title); title == "" || strings.TrimSpace(head) == title {
		if !split {
			return nil // the whole prompt was that one line
		}
		body = strings.TrimSpace(rest)
	}
	if body == "" {
		return nil
	}

	// ansi.Wrap breaks on words and hard-breaks a word too long to fit, which is
	// what a prompt full of paths and identifiers needs — a word-only wrap would
	// let a long path run off the card's right edge.
	var out []string
	for _, ln := range strings.Split(ansi.Wrap(body, inner, " -"), "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		if len(out) == hoverBodyLines {
			// There is more prompt than the card is going to show: the last line
			// says so rather than simply stopping, which is the difference
			// between "that's all of it" and "press ctrl+v".
			out[len(out)-1] = truncate(out[len(out)-1], max(inner-1, 1)) + "…"
			break
		}
		out = append(out, ln)
	}
	return out
}

// --- Drawing --------------------------------------------------------------------

// render draws the box. Same border and field as a context menu: the card is
// the same kind of thing — a temporary surface floating over the list — and two
// floating surfaces that did not look alike would read as two different
// programs.
func (c hoverCard) render() string {
	return menuBoxStyle.Render(strings.Join(c.lines, "\n"))
}

// overlayHoverCard floats the card over the list's rendered frame (see
// overlayMenu, menu.go, for why it is composited rather than spliced).
//
// It goes on below the context menu for the reason hoverMotion refuses to build
// one while a menu is up: if both were ever somehow on screen, the menu — the
// thing that can be pressed — is the one that has to be reachable.
func (m model) overlayHoverCard(view string) string {
	if !m.hover.open {
		return view
	}
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(view),
		lipgloss.NewLayer(m.hover.render()).X(m.hover.x).Y(m.hover.y).Z(1),
	).Render()
}
