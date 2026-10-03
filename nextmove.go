// nextmove.go — the Next List page's three decisions about an item: close it
// as done, park it on the Roadmap (or bring it back to Open), or decline it as
// a non-goal. Each one moves the item in the file (nextedit.go does the
// splice); this file is the page's side of it: the chords, the menu's rows'
// actions, and the note pad that asks for the words a record keeps.
//
//	ctrl+t   ✓ Close as done…       pad: "what showed it" (optional) → Closed
//	ctrl+f   ⇣ Move to Roadmap      one press, no words — Open ⇄ Roadmap
//	         ⇡ Move to Open         (the same row and chord on a Roadmap item)
//	ctrl+x   ⊘ Mark as non-goal…    pad: "why not" (optional) → Non-goals
//
// The chords are the list's own for the nearest act: ctrl+t marks a prompt
// done there, ctrl+f freezes one (parks it, "not now"), and ctrl+x removes
// one. Here none of the three deletes anything — the file's rule is that
// nothing leaves Open or Roadmap without a line in another section — but the
// hand that knows the list already knows which key means which.
//
// Why a pad for two of them and not the third: a Closed or Non-goals entry is
// a record, and the file's convention is that it says why ("what showed it",
// "its reason"). Asking at the moment of the decision is when the why is
// known. The words stay optional, since a record without them keeps the
// item's own text, and the pad's enter is also the confirmation those two
// moves need. A move between Open and Roadmap is neither a record nor final:
// the same chord moves it back, so it takes one press.
//
//	╭──────────────────────────────────────────────────────────────╮
//	│ ✓ Close N-012 as done                                        │  title
//	│ Consider back-tagging the releases that have a chore(rele…   │  the item
//	│ what showed it's done (optional)                             │  the field
//	│ enter close as done · esc cancel                             │  the keys
//	╰──────────────────────────────────────────────────────────────╯
//
// The pad is the list's ⚑ note pad (listflagnote.go) in shape and behaviour —
// a menu's box with a field in it, modal, and dismissed by a click off it.
// Unlike that pad, nothing is written before it opens: here the press asks a
// question, and esc is "never mind", which moves nothing.

package main

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The pad's shape: wider than the ⚑ pad, since a closing comment is a
// sentence rather than a phrase; and the floor under which a field and a caret
// no longer fit.
const (
	nextPadWidth = 64
	nextPadMin   = 30
	nextPadRows  = 6 // four rows and the border
)

// nextNotePad is the open pad: the item and move it is asking about, where its
// box sits, and its rows. The title, item line and hint are rendered once at
// open time, as the ⚑ pad's are; only the field changes per frame.
//
// The item is carried by value for the menu's reason (see nextMenu): the pad
// was opened about one item and must move that one, by its ID, whatever the
// highlight does meanwhile.
type nextNotePad struct {
	open     bool
	x, y     int // top-left cell of the box
	w, h     int
	ax, ay   int  // the cell the pad is anchored to, so a resize can re-place it
	anchored bool // opened from the menu (at its cell), or by a chord (centred)
	item     nextItem
	to       nextDest
	title    string
	text     string
	hint     string
	input    textinput.Model
}

// --- Opening -----------------------------------------------------------------

// nextHighlightedOr is the highlighted item, or a refusal said on the heading
// — the chords' shared guard, in promptFromNext's words.
func (m *model) nextHighlightedOr() (nextItem, bool) {
	it, ok := m.next.highlighted()
	if !ok {
		m.next.say("highlight an item first — ↑/↓ to choose one", false)
	}
	return it, ok
}

// closeFromNext, parkFromNext and declineFromNext are the three chords, on the
// highlighted item. The menu's rows reach the same functions with the item the
// menu was opened on.
func (m model) closeFromNext() (tea.Model, tea.Cmd) {
	it, ok := m.nextHighlightedOr()
	if !ok {
		return m, nil
	}
	return m.askNextNote(it, nextToClosed, 0, 0, false)
}

func (m model) parkFromNext() (tea.Model, tea.Cmd) {
	it, ok := m.nextHighlightedOr()
	if !ok {
		return m, nil
	}
	return m.parkNextItem(it)
}

func (m model) declineFromNext() (tea.Model, tea.Cmd) {
	it, ok := m.nextHighlightedOr()
	if !ok {
		return m, nil
	}
	return m.askNextNote(it, nextToNonGoals, 0, 0, false)
}

// parkNextItem moves an item between Open and Roadmap — whichever it is not
// in. No pad: see the file comment.
func (m model) parkNextItem(it nextItem) (tea.Model, tea.Cmd) {
	to := nextToRoadmap
	if it.Section == nextToRoadmap.section() {
		to = nextToOpen
	}
	return m.moveNext(it, to, "")
}

// nextParkLabel is the park row's label for an item: it names where the item
// will go, which depends on where it is.
func nextParkLabel(it nextItem) string {
	if it.Section == nextToRoadmap.section() {
		return "⇡ Move to Open"
	}
	return "⇣ Move to Roadmap"
}

// askNextNote opens the pad for a move that records words (to Closed or
// Non-goals). From the menu it is anchored at the menu's cell, so the question
// appears where it was asked; from a chord there is no pointer cell to answer
// at, so it is centred.
//
// A pane too small for the pad refuses in words rather than moving the item
// without asking: the pad's enter is the confirmation, and a decision this
// final does not get to skip it.
func (m model) askNextNote(it nextItem, to nextDest, ax, ay int, anchored bool) (tea.Model, tea.Cmd) {
	w := min(nextPadWidth, m.width-2)
	if w < nextPadMin || m.height < nextPadRows+2 {
		m.next.say("the pane is too small to ask for a note — widen it and try again", true)
		return m, nil
	}
	const chrome = 4 // border + one space of padding, each side
	inner := w - chrome

	title, place, hint := "✓ Close "+it.ID+" as done", "what showed it's done (optional)", "enter close as done · esc cancel"
	if to == nextToNonGoals {
		title, place, hint = "⊘ Mark "+it.ID+" as a non-goal", "why we won't do it (optional)", "enter mark as non-goal · esc cancel"
	}
	pad := nextNotePad{
		open:     true,
		w:        w,
		h:        nextPadRows,
		ax:       ax,
		ay:       ay,
		anchored: anchored,
		item:     it,
		to:       to,
		title:    hoverTitleStyle.Width(inner + 2).Render(truncate(title, inner)),
		text:     hoverFieldStyle.Width(inner + 2).Render(truncate(collapseLines(it.Text), inner)),
		hint:     hoverFieldStyle.Width(inner + 2).Render(truncate(hint, inner)),
		input:    newNextPadInput(place, inner),
	}
	m.nextPad = pad
	m.placeNextPad()
	m.clearHover()
	// The heading is where the move's answer lands, so the last action's note
	// goes now rather than being read as one.
	m.next.say("", false)
	return m, textinput.Blink
}

// placeNextPad puts the pad's box on screen: below-right of its anchor, the
// menus' rule, or centred.
func (m *model) placeNextPad() {
	p := &m.nextPad
	if p.anchored {
		p.x, p.y = placeBelowRight(p.ax, p.ay, p.w, p.h, m.width, m.height)
		return
	}
	p.x, p.y = max((m.width-p.w)/2, 0), max((m.height-p.h)/2, 0)
}

// newNextPadInput is the pad's field, in the box's own colours (see
// newFlagPadInput for why the panel background is set on the text too).
func newNextPadInput(placeholder string, width int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Prompt = ""
	// A closing comment is a sentence or two; the field scrolls sideways and
	// the record wraps it in the file.
	ti.CharLimit = 600
	st := ti.Styles()
	panel := lipgloss.Color(colPanel)
	st.Focused.Text = st.Focused.Text.Background(panel).Foreground(lipgloss.Color(colFg))
	st.Focused.Placeholder = st.Focused.Placeholder.Background(panel).Foreground(lipgloss.Color(colFaint))
	ti.SetStyles(st)
	ti.SetWidth(width)
	ti.Focus()
	return ti
}

// --- Driving it -----------------------------------------------------------------

// updateNextPad owns every key while the pad is up, the ⚑ pad's bargain:
// enter moves, esc (and ctrl+c, which must take the pad down before it can
// mean quit) walks away, and everything else is the field's.
func (m model) updateNextPad(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		p := m.nextPad
		m.nextPad = nextNotePad{}
		return m.moveNext(p.item, p.to, p.input.Value())
	case "esc", "ctrl+c":
		id := m.nextPad.item.ID
		m.nextPad = nextNotePad{}
		m.next.say(id+" left where it was", false)
		return m, nil
	}
	var cmd tea.Cmd
	m.nextPad.input, cmd = m.nextPad.input.Update(msg)
	return m, cmd
}

// clickNextPad is the pointer while the pad is up, either button: off the box
// it dismisses ("never mind" — nothing moves), on the field row it places the
// caret, anywhere else on the box it does nothing.
func (m model) clickNextPad(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	p := m.nextPad
	if msg.X < p.x || msg.X >= p.x+p.w || msg.Y < p.y || msg.Y >= p.y+p.h {
		m.nextPad = nextNotePad{}
		m.next.say(p.item.ID+" left where it was", false)
		return m, nil
	}
	// The field is the third inner row: border, title, the item's line.
	if msg.Y == p.y+3 {
		runes := []rune(p.input.Value())
		if lipgloss.Width(string(runes)) <= p.input.Width() {
			m.nextPad.input.SetCursor(colAtWidth(runes, msg.X-p.x-2))
		}
	}
	return m, nil
}

// --- The move ---------------------------------------------------------------------

// moveNext writes one move to the file and shows the page as it now stands.
//
// After the write the page is re-read from the file rather than patched in
// memory: the splice was made against the file as it is now, which may hold
// edits from another pane, and the page should show exactly that. The
// highlight stays on the item when it is still listed (Open ⇄ Roadmap), and
// otherwise goes to its neighbour, so a run of closes walks down the page.
//
// Closing an item also closes the open backlog prompt made from it, when there
// is one (nextBacklogCopy): the work it was a prompt for is done, and an open
// copy would invite it to be done again. A declined item's copy is left alone
// — a prompt is the user's to delete — and the note says it is still there.
func (m model) moveNext(it nextItem, to nextDest, note string) (tea.Model, tea.Cmd) {
	if m.next.path == "" {
		m.next.say("no "+nextListRel+" to write to", true)
		return m, nil
	}
	keep := it.ID
	if to == nextToClosed || to == nextToNonGoals {
		keep = m.next.neighbour(it.ID)
	}
	req := nextMoveReq{ID: it.ID, To: to, Note: note, Date: time.Now().Format("2006-01-02")}
	if _, err := moveNextItemFile(m.next.path, req); err != nil {
		m.next.say(err.Error(), true)
		return m, nil
	}

	line := it.ID
	switch to {
	case nextToClosed:
		line += " closed as done"
		if ref, _, ok := m.nextBacklogCopy(it); ok {
			if err := m.storeFor(ref.scope).setDone(ref.id, true); err != nil {
				line += " · its backlog prompt not marked done: " + err.Error()
			} else {
				line += " · its backlog prompt marked done"
				m.rebuildList()
			}
		}
	case nextToNonGoals:
		line += " marked a non-goal"
		if _, td, ok := m.nextBacklogCopy(it); ok {
			line += " · its backlog prompt “" + truncate(td.Title, 30) + "” is still open"
		}
	case nextToRoadmap:
		line += " moved to the Roadmap"
	case nextToOpen:
		line += " moved to Open"
	}

	m.next.inBacklog = m.nextBacklogIDs()
	m.next.reread(m.width, m.height, keep)
	m.next.say(line, false)
	return m, nil
}

// neighbour is the item the highlight should land on once id leaves the page:
// the next one in the page's order, else the one before, else none.
func (p nextPage) neighbour(id string) string {
	for i, it := range p.items {
		if it.ID != id {
			continue
		}
		switch {
		case i+1 < len(p.items):
			return p.items[i+1].ID
		case i > 0:
			return p.items[i-1].ID
		}
	}
	return ""
}

// --- Drawing --------------------------------------------------------------------

func (p nextNotePad) render() string {
	field := hoverBodyStyle.Width(p.w - 2).Render(p.input.View())
	return menuBoxStyle.Render(strings.Join([]string{p.title, p.text, field, p.hint}, "\n"))
}

// overlayNextPad floats the pad over the page (see overlayMenu, menu.go, for
// why it is composited rather than spliced). It goes on top of everything:
// the box that takes input is the box that has to be reachable.
func (m model) overlayNextPad(view string) string {
	if !m.nextPad.open {
		return view
	}
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(view),
		lipgloss.NewLayer(m.nextPad.render()).X(m.nextPad.x).Y(m.nextPad.y).Z(1),
	).Render()
}
