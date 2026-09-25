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
	"time"

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

// hoverDelay is how long the pointer has to stay on a row before its card is
// built. Without it the card opened on the first motion message the row saw,
// which turned a pass down the list — reaching for a row further on, or for the
// scrollbar — into a run of boxes opening and closing under the hand, each one
// landing over the rows still to be crossed.
//
// The wait is the whole difference between "the pointer went over this row" and
// "the pointer is asking about this row", and it is deliberately the same
// number catway's card uses (TIP_DELAY_MS in 09-hovercard.js): the two are the
// same feature on two front ends, and a hand that learns the timing in one
// should not have to learn it again in the other.
const hoverDelay = 400 * time.Millisecond

// hoverWarm is how long after a card comes down the next row's card opens with
// no wait at all.
//
// One dwell per row is the right price for a pointer arriving from elsewhere
// and the wrong one for a pointer already reading the list: walking down the
// rows comparing two prompts, or looking for which one carries the flag note,
// would mean holding still over every row in turn. So the dwell is the cost of
// the first card and the rows after it are free, for as long as the hand keeps
// moving between them — which is the rule every native tooltip already teaches,
// and the reason a run of them reads as one surface being moved rather than a
// series of separate waits.
//
// Only the pointer keeps the window open: clearHover, which is what a
// keystroke, a click, a resize, a rebuild and a lost terminal focus all go
// through, closes it along with the card. A card that appeared on contact when
// the hand came back from the keyboard would be the eagerness the dwell exists
// to fix, arriving by another door.
//
// Same number as catway's TIP_WARM_MS (09-hovercard.js), for the reason
// hoverDelay matches TIP_DELAY_MS: one feature, two front ends, one timing to
// learn.
const hoverWarm = 800 * time.Millisecond

// hoverPending is a card that has been asked for but not yet earned — the row
// the pointer is resting on, where it was resting when it last moved, and the
// generation that says whether the tick coming back is still about this rest.
//
// gen is what makes the timer safe to leave running. A tea.Cmd cannot be
// cancelled once it is in flight, so every stale tick has to be recognisable
// when it lands: each arming takes the next number, and a tick carrying any
// other one is answered by doing nothing. That covers the pointer moving to
// another row, the hand going back to the keyboard, and a menu opening in the
// meantime, without a cancellation channel for any of them.
type hoverPending struct {
	armed bool
	// stage is the page the wait was armed on: the backlog list, or the Next
	// List page (nexthover.go), which shares this state. row indexes that
	// page's rows, so a tick landing after the page changed has to be told
	// apart from one that is still about the rows under the pointer.
	stage uiStage
	row   int // the filtered-list index the wait is for
	x, y  int // where the card will be placed, in screen coordinates
	gen   uint64
}

// hoverIsWarm reports whether the next row's card is owed no wait: either one
// is on screen this instant, or one came down recently enough that the window
// is still open.
//
// The open card counts because on this front end there is no separate "left the
// row" event to have already taken it down. catway gets a mouseleave before the
// next row's mouseenter, so by the time it asks, the card it is stepping off is
// already gone and only the window remains; here one motion message is both the
// leaving and the arriving, and the card is still standing when the question is
// put. Same condition either way (TIP_WARM_MS in 09-hovercard.js), reached from
// two different places in the sequence.
//
// Read *before* the teardown on any path about to open a card, since taking a
// card down is itself what opens the window.
func (m model) hoverIsWarm() bool { return m.hover.open || time.Now().Before(m.hoverWarmUntil) }

// hoverMovedOn is the pointer's own teardown: the card goes and, if there was
// one to go, the warm window opens behind it. It is the counterpart to
// clearHover, which is the teardown for everything that is not the pointer.
//
// A row the pointer crosses that had no card of its own — a heading, a spacer,
// a title-only todo — leaves the window as it found it rather than refreshing
// it, so the warmth is measured from the last card actually read and not
// extended by every empty row on the way past.
func (m *model) hoverMovedOn() {
	if m.hover.open {
		m.hoverWarmUntil = time.Now().Add(hoverWarm)
	}
	m.hover = hoverCard{}
	m.hoverPend = hoverPending{}
}

// hoverTickMsg is a dwell that has elapsed. It says nothing about which row —
// only which arming it belongs to; the row is read back off the model, which is
// the only copy that can still be current.
type hoverTickMsg struct{ gen uint64 }

// hoverTick arms one dwell. Unlike scheduleTick this loop does not re-arm
// itself: it is one shot per rest, and the next rest arms the next one.
func hoverTick(gen uint64) tea.Cmd {
	return tea.Tick(hoverDelay, func(time.Time) tea.Msg { return hoverTickMsg{gen: gen} })
}

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
//
// What arriving on a row does is one of two things. Cold — the pointer has come
// from outside the list, or the hand from the keyboard — it starts the dwell
// (hoverDelay) and nothing more; the card is built when the tick comes back, in
// hoverDwell. Warm — a card was up moments ago, so the hand is already reading
// the list (hoverWarm) — it builds the card here and now, off this very
// message.
//
// Either way the todo is read at the moment the card appears rather than
// whenever the pointer happened to land, which is why both paths go through
// hoverCardFor rather than carrying a todo along from here.
func (m model) hoverMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if m.listMenu.open || m.flagPad.open || m.dragging {
		// Not the pointer travelling: a surface has taken over the screen the
		// card floats on, so the warm window closes with it.
		m.clearHover()
		return m, nil
	}
	i, ok := m.list.rowAtLine(msg.Y - listRowsRow)
	if !ok {
		// A heading, a spacer, or the chrome above and below. Still the pointer
		// crossing the list, so the window stays open — a group heading between
		// two rows is something to pass over, not a reason to start waiting
		// again on the far side of it.
		m.hoverMovedOn()
		return m, nil
	}
	if m.hover.open && m.hover.row == i {
		return m, nil // still on the row the card is already about
	}
	if m.hoverPend.armed && m.hoverPend.row == i && m.hoverPend.stage == stageList {
		// The wait for this row is already running, so it keeps running: only
		// where the card will land is updated. Restarting the clock on every
		// cell would mean a hand that drifts while it reads never rests long
		// enough anywhere, and the card would only ever appear for a pointer
		// held perfectly still.
		m.hoverPend.x, m.hoverPend.y = msg.X, msg.Y
		return m, nil
	}
	// A different row. Whatever is up or pending belongs to the one just left —
	// a card naming the row above the pointer is worse than no card — so it goes
	// now. Whether this row then waits or answers at once is the warm window's
	// call, and it has to be asked before the teardown, which is what opens it.
	warm := m.hoverIsWarm()
	m.hoverMovedOn()
	if warm {
		// Straight to the card, on the same message the pointer arrived with.
		// A row with nothing to say still spends the arrival: it produces no
		// card, and the window keeps counting down from the last one that did,
		// so the row after it is still free.
		if card, ok := m.hoverCardFor(i, msg.X, msg.Y); ok {
			m.hover = card
		}
		return m, nil
	}
	m.hoverGen++
	m.hoverPend = hoverPending{armed: true, stage: stageList, row: i, x: msg.X, y: msg.Y, gen: m.hoverGen}
	return m, hoverTick(m.hoverGen)
}

// hoverDwell answers the tick: the pointer has rested long enough, so the row
// under it gets read and its card built.
//
// Everything hoverMotion checked is checked again, because the wait is time in
// which any of it can have stopped holding — a menu opened, the stage changed,
// a peer deleted the todo, the list rebuilt under the pointer. The row index is
// resolved fresh for the same reason: it names a line on the screen, and what
// is on that line now is what the card has to be about.
func (m model) hoverDwell(msg hoverTickMsg) (tea.Model, tea.Cmd) {
	if !m.hoverPend.armed || m.hoverPend.gen != msg.gen {
		return m, nil // a tick from a rest that is already over
	}
	p := m.hoverPend
	m.hoverPend = hoverPending{} // spent, whether or not it produces a card
	if p.stage != m.stage {
		return m, nil // armed on the other page; its row means nothing here
	}
	if m.stage == stageNextList {
		// The Next List page's one surface to refuse over is its menu: a
		// dwell armed just before the right-click must not land a card
		// beside a box that has taken over the page.
		if m.nextMenu.open {
			return m, nil
		}
		if card, ok := m.nextCardFor(p.row, p.x, p.y); ok {
			m.hover = card
		}
		return m, nil
	}
	if m.stage != stageList || m.listMenu.open || m.flagPad.open || m.dragging {
		return m, nil
	}
	card, ok := m.hoverCardFor(p.row, p.x, p.y)
	if !ok {
		return m, nil
	}
	m.hover = card
	return m, nil
}

// hoverCardFor reads the todo drawn on a filtered-list row and builds its card,
// placed against (x, y). It is the step between "the pointer has earned a card
// for this row" and the card itself, shared by the two ways of earning one: the
// dwell elapsing, and the warm window letting an arrival through.
//
// The row index names a *line on the screen*, so what is on that line is
// resolved here rather than carried from wherever the pointer last was — the
// list can have been rebuilt in between, by this pane or by a peer.
func (m model) hoverCardFor(row, x, y int) (hoverCard, bool) {
	idx, ok := m.list.refAt(row)
	if !ok || idx < 0 || idx >= len(m.rows) {
		return hoverCard{}, false
	}
	td, ok := m.resolve(m.rows[idx])
	if !ok {
		// On screen but no longer in the store — another pane deleted it since
		// the last rebuild. Nothing to say about it, and the next rebuild takes
		// the row away.
		return hoverCard{}, false
	}
	return m.buildHoverCard(td, row, x, y)
}

// clearHover is the teardown for everything that is not the pointer moving on:
// a keystroke (the hand is on the keyboard, so the pointer is not what the eye
// is following), a click, a resize, a rebuild, a menu, and the terminal losing
// focus altogether. Those are the premises the card stands on, and most of them
// never produce another motion message to notice their going.
//
// It takes down three things. The card, obviously. Any dwell still being waited
// for, because a dwell that survived a keystroke would open a card several
// hundred milliseconds after the hand had already moved on, which is the one
// thing a delay must not introduce. And the warm window, because every caller
// here means the hand has left the list rather than travelled across it — a
// card appearing on contact when it comes back would undo the dwell entirely.
func (m *model) clearHover() {
	m.hover = hoverCard{}
	m.hoverPend = hoverPending{}
	m.hoverWarmUntil = time.Time{}
}

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

// hoverLines is the card's content: the title, the body, the flag's note, a done
// prompt's completion stamp and the session's launch flags. Each is dropped when
// it has nothing to say, so the card is only ever as tall as this prompt earns.
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

	// The label/value fields, gathered first so the separator above them is
	// drawn once, and only when at least one of them has a value.
	type field struct{ label, value string }
	var fields []field

	// When a done prompt was finished, spelled out in full (formatDoneStamp).
	// The row already carries the compact form ("done 14:05"), but it can't be
	// relied on to show it. The stamp is the row's last mark, after the name and
	// any annotations, and a titleless prompt's name alone is up to 60 cells, so
	// in a side pane of ordinary width it is the stamp that goes off the right
	// edge. The card is where the pointer already is, so it is the one place in
	// the list that is sure to show it. The compact form also drops the date
	// within the week and the zone always, and that is the question someone
	// hovering a finished prompt is usually asking.
	//
	// It leads the fields because on a done card it is the live fact. The
	// model and effort below it describe a launch that has already happened.
	// A todo finished before DoneAt existed has no stamp and adds no row, as it
	// adds no mark to its list row.
	if td.Done && !td.DoneAt.IsZero() {
		fields = append(fields, field{"Done", formatDoneStamp(td.DoneAt)})
	}

	// The session's launch flags — the two that decide what the receiving agent
	// *is*, and the pair a drop is most often reconsidered over. The rest of the
	// setup (context, reviews, wrap-up) is deliberately left to the ⚙ panel:
	// those are things the agent will do, and this card is about what is being
	// sent and to what.
	if td.Session != nil {
		fields = append(fields,
			field{"Model", td.Session.Model},
			field{"Effort", td.Session.Effort})
	}

	sep := false
	for _, f := range fields {
		if f.value == "" {
			continue // an empty value drops its row, as it does on cats' card
		}
		if !sep {
			row("", hoverBodyStyle) // the separator between the prose and the fields
			sep = true
		}
		pad := hoverLabelWidth - lipgloss.Width(f.label)
		text := f.label + strings.Repeat(" ", max(pad, 0)) + f.value
		row(truncate(text, inner), hoverFieldStyle)
	}

	// A title-only todo with no body, note, stamp or session has nothing the
	// row is not already saying, so it gets no card at all rather than a box
	// repeating one line back at the pointer. A done title-only todo with a
	// stamp does get one: the card spells out a stamp the row only abbreviates,
	// or may have cut off.
	if len(lines) < 2 {
		return nil
	}
	return lines
}

// hoverLabelWidth is the column the field values line up on — "Effort" plus two
// spaces, the widest label the card has. Fixed rather than measured because the
// set is fixed: three labels (Done, Model, Effort), and a table that
// re-measures itself would move its values sideways depending on which fields
// a todo happened to set.
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
