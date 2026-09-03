// listflagnote.go — the note pad the list's ⚑ Flag row opens.
//
// Raising a flag from the context menu (listmenu.go) used to be the whole of
// the gesture: the mark went up bare and the status line pointed at the edit
// form, which is a screen change away from the row that was just pointed at.
// But a flag is only half a thought — "there is something about this one"
// wants "…because" straight after it, and by the time the form is open the
// because is a sentence someone has to reconstruct. So the words are asked for
// where the press happened, in a pad floated on the same cell the menu was:
//
//	╭──────────────────────────────────────╮
//	│ Fix the drop timeout                 │  ← which prompt this is about
//	│ why this one is flagged (optional)   │  ← the note, with the caret in it
//	│ enter save · esc leave it bare       │  ← both ways out, always shown
//	╰──────────────────────────────────────╯
//
// The mark is already on disk before the pad opens. That is deliberate: the
// press on ⚑ Flag is a complete answer on its own, so it is honoured on its
// own, and the pad is an invitation rather than a form the flag is trapped
// behind. Escaping leaves exactly what the press promised — a flagged prompt
// with no note — and nothing here can lose the mark.
//
// The pad borrows the menus' box and field (menu.go) because it is the same
// kind of thing: a temporary surface floating over the list, placed by the same
// rule, dismissed by a click off it. What it is not is a menu — it has no rows
// and takes text — which is why it carries its own type rather than a menuBox,
// the same call listhover.go makes for the hover card.
package main

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The pad's shape, on the hover card's terms: a preference for the width, a
// floor under which there is no room for a field and a caret both, at which
// point the pad does not appear at all and the status line falls back to
// naming the form (see beginFlagNote).
const (
	flagPadWidth = 46
	flagPadMin   = 28
	// The hint row, which is never abbreviated: the pad is modal, and the two
	// keys that end it are the only thing on screen that says how to get out.
	flagPadHint = "enter save · esc leave it bare"
)

// flagNotePad is the open pad: where its box sits, which todo it is writing a
// note for, the field itself, and the two rows around it.
//
// The title and hint are rendered once at open time — like a menu's labels and
// a hover card's rows — because they describe the prompt as it was when the pad
// opened; only the field is drawn per frame, since only the field changes.
//
// ref is carried rather than re-read from the highlight for the reason the
// menu's is: the pad was opened for one prompt and must write to that one, even
// if something moved the list underneath it.
type flagNotePad struct {
	open   bool
	x, y   int // top-left cell of the box, in screen coordinates
	w, h   int
	ax, ay int // the cell the pad is anchored to, kept so a resize can re-place it
	ref    todoRef
	title  string // the prompt's own title row, pre-rendered
	hint   string // the keys row, pre-rendered
	input  textinput.Model
	// raised says the press that opened this pad is what put the flag up, as
	// against a pad opened on a prompt that was already flagged. It decides
	// nothing about the write and everything about what walking away says.
	raised bool
}

// beginFlagNote opens the pad over the list, anchored at (x, y) — the cell the
// menu was opened on, so the answer appears where the question was asked.
//
// raised says the press that got here is what put the flag up, which is the
// only thing that changes: it decides what escaping says afterwards ("the mark
// stands, and you chose not to say why") as against editing an existing note
// ("nothing changed"). The mark itself is already saved either way.
//
// A pane too narrow for the pad is not an error. The flag is up, the press was
// honoured, and the status line says where the words can still be written — the
// behaviour this feature replaced, kept as its own fallback.
func (m model) beginFlagNote(ref todoRef, x, y int, raised bool) (tea.Model, tea.Cmd) {
	td, ok := m.resolve(ref)
	if !ok {
		m.setStatus("could not find that prompt", true)
		return m, nil
	}
	w := min(flagPadWidth, m.width-2)
	if w < flagPadMin || m.height < 7 {
		m.setStatus("flagged — enter to add a note", false)
		return m, nil
	}
	// The border and the one space of padding on each side, measured exactly as
	// the hover card measures its own (see buildHoverCard).
	const chrome = 4
	inner := w - chrome

	title := strings.TrimSpace(td.Title)
	if title == "" {
		title = firstLine(td.Prompt, inner)
	}
	pad := flagNotePad{
		open:   true,
		ref:    ref,
		ax:     x,
		ay:     y,
		w:      w,
		h:      5, // three rows and the border
		title:  hoverTitleStyle.Width(inner + 2).Render(truncate(flagGlyph+" "+title, inner)),
		hint:   hoverFieldStyle.Width(inner + 2).Render(truncate(flagPadHint, inner)),
		input:  newFlagPadInput(td.FlagNote, inner),
		raised: raised,
	}
	pad.x, pad.y = placeBelowRight(x, y, pad.w, pad.h, m.width, m.height)
	m.flagPad = pad
	// The status line is where the pad's own answers land, so whatever the last
	// action left there goes now rather than being read as one of them. The
	// invitation is on the pad itself, in the field's placeholder.
	m.setStatus("", false)
	return m, textinput.Blink
}

// newFlagPadInput is the pad's field: the form's note field (newFlagInput) in
// the box's own colours.
//
// The panel background is set on the text and the placeholder both, because the
// pad is composited over the list rather than drawn into a cleared screen: a
// run of unstyled cells inside the box would let the row underneath show
// through it, the same reason every menu row is padded to the box's width.
func newFlagPadInput(note string, width int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "why this one is flagged (optional)"
	ti.Prompt = ""
	// The same limit the form's field keeps, for the same reason: a note is a
	// phrase, and the rows it is read back on (the menu row, the hover card,
	// the prompt view's meta line) have one line each for it.
	ti.CharLimit = 120
	st := ti.Styles()
	panel := lipgloss.Color(colPanel)
	st.Focused.Text = st.Focused.Text.Background(panel).Foreground(lipgloss.Color(colFg))
	st.Focused.Placeholder = st.Focused.Placeholder.Background(panel).Foreground(lipgloss.Color(colFaint))
	ti.SetStyles(st)
	ti.SetWidth(width)
	ti.SetValue(note)
	ti.CursorEnd()
	ti.Focus()
	return ti
}

// clearFlagNote takes the pad down without writing anything. The flag it may
// have raised stays up: it was saved by the press that opened the pad, and a
// dismissal is a decision about the words, never about the mark.
func (m *model) clearFlagNote() { m.flagPad = flagNotePad{} }

// --- Driving it -----------------------------------------------------------------

// updateFlagNote is the keyboard while the pad is up. It owns every key, the
// same bargain a menu makes: enter saves, esc (and ctrl+c, which must take the
// pad down before it can mean quit) walks away, and everything else is the
// field's — including the deletes and the motions, which is the whole point of
// a pad rather than a row.
func (m model) updateFlagNote(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return m.commitFlagNote()
	case "esc", "ctrl+c":
		raised := m.flagPad.raised
		m.clearFlagNote()
		if raised {
			m.setStatus("flagged, with no note", false)
		} else {
			m.setStatus("note unchanged", false)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.flagPad.input, cmd = m.flagPad.input.Update(msg)
	return m, cmd
}

// clickFlagNote is the pointer while the pad is up: a press on the field row
// drops the caret where it landed, a press elsewhere on the box does nothing,
// and a press off the box takes the pad down.
//
// Dismissing rather than saving is the menus' own rule (see clickListMenu) — a
// click off a floating box is "never mind", everywhere else on this machine —
// and it costs nothing here, because the mark the gesture was really about is
// already on disk.
func (m model) clickFlagNote(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	p := m.flagPad
	if msg.X < p.x || msg.X >= p.x+p.w || msg.Y < p.y || msg.Y >= p.y+p.h {
		raised := m.flagPad.raised
		m.clearFlagNote()
		if raised {
			m.setStatus("flagged, with no note", false)
		}
		return m, nil
	}
	// The field is the second inner row: one border row, then the title.
	if msg.Y == p.y+2 {
		m.placeFlagPadCursor(msg.X - p.x - 2) // the border and the padding column
	}
	return m, nil
}

// placeFlagPadCursor drops the field's caret on the column that was clicked, on
// placeFlagCursor's terms — nothing at all on a value scrolled sideways, where
// the column the click names is not the column the value starts at.
func (m *model) placeFlagPadCursor(col int) {
	runes := []rune(m.flagPad.input.Value())
	if lipgloss.Width(string(runes)) > m.flagPad.input.Width() {
		return
	}
	m.flagPad.input.SetCursor(colAtWidth(runes, col))
}

// commitFlagNote writes the field back to the backlog and takes the pad down.
//
// The set is read from the store and written whole through store.setAnnots, for
// the reason setMenuAnnots does it that way: the marks are edited together
// everywhere in this program, and a path that wrote one field would be the one
// that could clobber the others with a stale copy. The flag is asserted rather
// than assumed — a note is what a flag says, so writing one raises it — which
// also makes this safe if anything cleared the mark between the two presses.
func (m model) commitFlagNote() (tea.Model, tea.Cmd) {
	ref, note := m.flagPad.ref, strings.TrimSpace(m.flagPad.input.Value())
	m.clearFlagNote()
	td, ok := m.resolve(ref)
	if !ok {
		m.setStatus("could not find that prompt", true)
		return m, nil
	}
	a := annotsOf(td)
	a.Flag, a.FlagNote = true, note
	if err := m.storeFor(ref.scope).setAnnots(ref.id, a); err != nil {
		m.setStatus("save failed: "+err.Error(), true)
		return m, nil
	}
	m.rebuildList()
	// The row can move under a lens that sorts (see setMenuAnnots), and the
	// keyboard should resume on the prompt the note was just written for.
	m.selectRow(ref)
	if note == "" {
		// An emptied field is an answer too: the note is gone, the mark stays.
		m.setStatus("flagged, with no note", false)
		return m, nil
	}
	m.setStatus("flag note saved", false)
	return m, nil
}

// --- Drawing --------------------------------------------------------------------

// render draws the pad. Same border and field as a context menu and the hover
// card, because it is the third of the same kind of thing; only the middle row
// is rendered here rather than at open time, since only the field changes as it
// is typed into.
func (p flagNotePad) render() string {
	field := hoverBodyStyle.Width(p.w - 2).Render(p.input.View())
	return menuBoxStyle.Render(strings.Join([]string{p.title, field, p.hint}, "\n"))
}

// overlayFlagNote floats the pad over the list's rendered frame (see
// overlayMenu, menu.go, for why it is composited rather than spliced).
//
// It goes on above the hover card and beside the menu — the two are never up at
// once, since opening the pad is what closes the menu — for the reason the menu
// goes on top of the card: the box that takes input is the box that has to be
// reachable.
func (m model) overlayFlagNote(view string) string {
	if !m.flagPad.open {
		return view
	}
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(view),
		lipgloss.NewLayer(m.flagPad.render()).X(m.flagPad.x).Y(m.flagPad.y).Z(1),
	).Render()
}
