// listmenu.go — the todo list's context menu.
//
// The list can do eight things to a prompt and the action bar has room for
// five, so three of them — view, done, freeze — have only ever been chords, and
// a chord is not something a pointer can find. The prompt's two annotations were
// worse off still: setting a priority or marking a quick win meant opening the
// edit form and finding the annotation bar, for a fact about a row that is read
// off that row. Right-click a row and the whole set is named in one place, on
// the prompt that was pointed at:
//
//	╭─────────────────────────────────╮
//	│ ✎ Edit…                   enter │
//	│ ◉ View                   ctrl+v │
//	│ ✉ Send…             shift+enter │
//	│ ◷ Schedule…              ctrl+s │
//	│ ✓ Mark done              ctrl+t │
//	│ ❄ Freeze                 ctrl+f │
//	│ ☐ 🍏 Quick win                  │
//	│ (•) Priority: none              │
//	│ ( ) Priority: △ high            │
//	│ ( ) Priority: ▲ critical        │
//	│ ☐ ⚑ Flag                        │
//	│ ➦ Export…                ctrl+o │
//	│ ✖ Delete…                ctrl+x │
//	╰─────────────────────────────────╯
//
// Every row that has a chord keeps it, so the menu doubles as the keyboard's own
// reference — the action bar's five chips already work this way, and the actions
// that never had a chip are exactly the ones nothing was teaching. The five
// annotation rows carry no chord because there is none to carry: this menu is
// the list's only road to them.
//
// The flag is a checkbox here like the fruit, and it flips the mark alone: its
// note is words, and a menu row is a press rather than a place to type. A flag
// raised from the menu therefore comes up bare, which is the honest shape of the
// gesture — "there is something about this one" is exactly what a right-click
// and one press can say. The words are added on the form, where the ⚑ segment
// raises a note field beside the prompt they are about (annotbar.go). A flagged
// prompt's row here shows the note it already carries, so pressing ✎ Edit… to
// write one is a decision made with the current note in sight rather than a
// screen away.
//
// Rows that name a state rather than an action say what the press will do, which
// is the only thing a menu row ever promises: ✓ Mark done reads ↺ Reopen on a
// finished prompt, ❄ Freeze reads ☀ Unfreeze on a shelved one. The annotations
// go the other way and draw their state in the margin — a ☐/☑ box and one filled
// radio out of three — because unlike done and frozen they are not a flip: the
// priority is exactly one of three levels, so the menu has to be able to show
// which, and pressing the level a prompt already has has to be a no-op rather
// than a toggle back off it. The glyphs are the form's own annotation bar's
// (annotbar.go), which are in turn the marks the list row draws, so the same
// legend is taught in all three places.
//
// Everything a menu *is* — the box, its keys, its drawing — is menuBox
// (menu.go), shared with the editor's context menu. The dim-rather-than-omit
// rule comes with it: a row that cannot act right now (sending a frozen prompt,
// scheduling with no cats socket) is drawn grey and says why when it is pressed,
// in the same words the chord uses, rather than disappearing.

package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// listMenuNoteWidth is how much of a flag's note the menu row will show. Wide
// enough for the phrase a note usually is, short enough that the box stays a
// menu rather than becoming a reading pane — the rest is on the hover card and
// the prompt view, both of which have room for the whole line.
const listMenuNoteWidth = 32

// The menu's actions, which are also its row order. Reading order is roughly
// least to most committing: the two that only look at the prompt, then the two
// that hand it to an agent, then the two that change its state, then the two
// that move or destroy it. Delete is last for the reason it is last on the
// action bar — the row a slipped hand is least likely to land on.
const (
	listMenuEdit = iota
	listMenuView
	listMenuSend
	listMenuSchedule
	listMenuDone
	listMenuFreeze
	// The annotations sit directly after the two state rows, and in the
	// annotation bar's own order — the fruit, then the three priority levels as
	// they escalate, then the flag. All five are per-row marks and are learned together, which
	// is the same reason the list footer keeps done, freeze and priority
	// adjacent; putting them between the state rows and the two that move or
	// destroy the prompt also means the destructive end of the menu stays the
	// destructive end.
	listMenuFruit
	listMenuPrioNone
	listMenuPrioHigh
	listMenuPrioCritical
	listMenuFlag
	listMenuExport
	listMenuDelete
	listMenuActionCount
)

// listMenuPrio maps the three priority rows onto the levels they set, so the row
// order and the escalation order cannot drift apart — the same table-over-switch
// turn prioValues makes for the annotation bar.
var listMenuPrio = map[int]string{
	listMenuPrioNone:     priorityNone,
	listMenuPrioHigh:     priorityHigh,
	listMenuPrioCritical: priorityCritical,
}

// listMenu is the open menu: the shared box, plus the todo it was opened on.
//
// The ref is carried rather than re-read from the highlight when a row is
// pressed. Opening the menu moves the highlight onto the row that was clicked,
// so the two agree — but the labels on the menu were resolved from this todo's
// state at open time, and a row that says "↺ Reopen" must reopen the prompt it
// was drawn for. Holding the ref is what makes that impossible to get wrong.
type listMenu struct {
	menuBox
	ref todoRef
}

// openListMenu builds the menu for a right-click on a todo row and opens it.
//
// Every row's availability is resolved here, once, from the state the press
// landed in — so what the menu draws and what pressing a row does cannot come
// from two different reads of the todo. The refusals are the ones the chords
// already give (see startDrop and beginSchedule); saying them the same way in
// both places is contract 4 of the project's own rules — refuse in words — kept
// for the pointer as well as the keyboard.
func (m model) openListMenu(msg tea.MouseClickMsg, ref todoRef) (tea.Model, tea.Cmd) {
	td, ok := m.resolve(ref)
	if !ok {
		// The row is on screen but its todo is not in the store — another pane
		// deleted it between the last rebuild and this press. Nothing to offer a
		// menu about, and the next rebuild takes the row away.
		m.setStatus("could not find that prompt", true)
		return m, nil
	}

	// Send's refusals are startDrop's own, in startDrop's words.
	sendWhy := ""
	switch {
	case m.client == nil:
		sendWhy = "cats control socket unavailable — can't drop into a session"
	case m.dropping:
		sendWhy = "a drop is still in progress…"
	case td.Frozen:
		sendWhy = "that prompt is frozen — unfreeze it (ctrl+f) to send it"
	}

	// Schedule's are beginSchedule's, and they are not the same set. A drop
	// already in flight does not block one, because scheduling is a note written
	// on the todo rather than a socket call — it fires later. A finished prompt
	// does block one, though sending it would have been allowed: a drop can
	// reopen work by handing it to an agent, while a timer set on closed work is
	// a promise about something that is over. The socket still has to exist
	// either way, since a schedule with nothing to fire into is a promise the
	// manager cannot keep.
	schedWhy := ""
	switch {
	case m.client == nil:
		schedWhy = "cats control socket unavailable — can't schedule a drop"
	case td.Done:
		schedWhy = "that prompt is done — reopen it (ctrl+t) to schedule it"
	case td.Frozen:
		schedWhy = "that prompt is frozen — unfreeze it (ctrl+f) to schedule it"
	}

	// The two flipping rows. Freeze is offered from every state, including done,
	// because the store's own freeze clears the done flag (see store.freeze):
	// "I am not going to do this after all" is a decision that can arrive late.
	doneLabel, freezeLabel := "✓ Mark done", "❄ Freeze"
	if td.Done {
		doneLabel = "↺ Reopen"
	}
	if td.Frozen {
		freezeLabel = "☀ Unfreeze"
	}

	// The annotation rows draw their own state in the margin, in the annotation
	// bar's glyphs. The radio match is exact on purpose, exactly as the bar's is
	// (see annotBarLayout): a hand-edited backlog can hold anything, including
	// the retired "low", and a value this program cannot read fills no hole —
	// which leaves all three rows offering to replace it, the only honest
	// reading of a level that is not one.
	box := "☐"
	if td.Fruit {
		box = "☑"
	}
	// The flag's row wears its note, trimmed to something a menu can hold — the
	// menu sizes itself to its widest row (menuBox.size), and a long note would
	// stretch the whole box across the pane for one line of it.
	flagLabel := "☐ " + flagGlyph + " Flag"
	if td.Flag {
		flagLabel = "☑ " + flagGlyph + " Flag"
		if note := strings.TrimSpace(td.FlagNote); note != "" {
			flagLabel += ": " + truncate(note, listMenuNoteWidth)
		}
	}
	radio := func(level string) string {
		if td.Priority == level {
			return "(•)"
		}
		return "( )"
	}

	var mu listMenu
	mu.open, mu.ref = true, ref
	mu.items = []menuItem{
		{act: listMenuEdit, label: "✎ Edit…", hint: "enter"},
		{act: listMenuView, label: "◉ View", hint: "ctrl+v"},
		{act: listMenuSend, label: "✉ Send…", hint: m.modEnter(), why: sendWhy},
		{act: listMenuSchedule, label: "◷ Schedule…", hint: "ctrl+s", why: schedWhy},
		{act: listMenuDone, label: doneLabel, hint: "ctrl+t"},
		{act: listMenuFreeze, label: freezeLabel, hint: "ctrl+f"},
		// "Priority:" is repeated on all three rather than written once as a
		// heading over them. A heading row would be a row that cannot be pressed
		// sitting in a list of rows that can, and the eye landing halfway down a
		// menu should not have to look up to find out what "△ high" is high of.
		{act: listMenuFruit, label: box + " " + fruitGlyph + " Quick win"},
		{act: listMenuPrioNone, label: radio(priorityNone) + " Priority: none"},
		{act: listMenuPrioHigh, label: radio(priorityHigh) + " Priority: " + prioHighGlyph + " high"},
		{act: listMenuPrioCritical, label: radio(priorityCritical) + " Priority: " + prioCriticalGlyph + " critical"},
		{act: listMenuFlag, label: flagLabel},
		// Export needs no socket — the picker is shorter without one (no
		// workspace rows), not gone — and delete is answerable from any state,
		// so neither is ever dim.
		{act: listMenuExport, label: "➦ Export…", hint: "ctrl+o"},
		{act: listMenuDelete, label: "✖ Delete…", hint: "ctrl+x"},
	}
	mu.cursor = mu.firstLive()
	mu.size()
	mu.place(msg.X, msg.Y, m.width, m.height)
	m.listMenu = mu
	// The status line is the menu's own answer channel — a dim row pressed says
	// why there — so whatever the last action left behind goes now rather than
	// being mistaken for a reply to this gesture.
	m.setStatus("", false)
	return m, nil
}

// --- Driving it -----------------------------------------------------------------

// updateListMenu is the keyboard while the menu is up — the shared walk (see
// menuBox.key), with this menu's own answer to a press.
func (m model) updateListMenu(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.listMenu.key(msg) {
	case menuKeyPress:
		return m.pressListMenu(m.listMenu.cursor)
	case menuKeyClose:
		m.listMenu = listMenu{}
	}
	return m, nil
}

// clickListMenu is the pointer while the menu is up: a press on a row presses
// it, a press on the box's border does nothing, and a press anywhere else
// dismisses without acting — the three things a click can mean on an open menu.
//
// Dismissing is deliberately all a click off the box does: the row underneath is
// not also selected, and the button underneath is not also pressed. A menu is
// taken down by the same click that would otherwise have acted, everywhere else
// on this machine, and one that acted on the way out would make "never mind" the
// riskiest gesture on the screen.
func (m model) clickListMenu(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if !m.listMenu.inside(msg.X, msg.Y) {
		m.listMenu = listMenu{}
		return m, nil
	}
	row, ok := m.listMenu.hit(msg.X, msg.Y)
	if !ok {
		return m, nil
	}
	return m.pressListMenu(row)
}

// pressListMenu runs row i on the todo the menu was opened for. The menu closes
// either way: an item that acted has nothing left to offer, and one that could
// not act has said why on the status line, which the menu would otherwise be
// covering.
//
// Every live row hands off to the same helper its chord does, so the two roads
// into an action cannot drift — including the guards inside those helpers, which
// stay in place even though the menu has already checked them. The menu's copy
// is what greys the row; the helper's is what still holds if the world changed
// between the right-click and the press.
func (m model) pressListMenu(i int) (tea.Model, tea.Cmd) {
	if i < 0 || i >= len(m.listMenu.items) {
		return m, nil
	}
	it := m.listMenu.items[i]
	ref := m.listMenu.ref
	m.listMenu = listMenu{}
	if !it.live() {
		m.setStatus(it.why, false)
		return m, nil
	}
	// The highlight was moved onto this row by the press that opened the menu,
	// so the selection-reading helpers below are already aimed at ref. Re-parking
	// it makes that a fact rather than an assumption — cheap, and it keeps the
	// keyboard resuming from the row the pointer just acted on.
	m.selectRow(ref)
	switch it.act {
	case listMenuEdit:
		return m.beginEditRef(ref)
	case listMenuView:
		return m.beginView()
	case listMenuSend:
		return m.startDrop(ref)
	case listMenuSchedule:
		return m.beginSchedule()
	case listMenuDone:
		return m.toggleSelected()
	case listMenuFreeze:
		return m.freezeSelected()
	case listMenuFruit, listMenuFlag, listMenuPrioNone, listMenuPrioHigh, listMenuPrioCritical:
		return m.setMenuAnnots(ref, it.act)
	case listMenuExport:
		return m.startExport(ref)
	case listMenuDelete:
		return m.beginDelete()
	}
	return m, nil
}

// setMenuAnnots applies the annotation row act to a todo and writes it back.
//
// The set is read from the store rather than from the menu's copy of the labels,
// and written whole through store.setAnnots — the two marks are edited and saved
// together everywhere else in this program (see annotations.go), and a path that
// wrote only the field it changed would be the one place that could clobber the
// other with a stale value.
//
// The row is re-parked afterwards because it can move: with the priority lens
// on, raising a prompt lifts it past everything unraised, and a cursor left at
// its old index would land on whichever row slid into the gap. The status line
// says what happened for the same reason the chords' notes do — with the lens on
// the row can travel most of a pane, and the note is the only thing saying the
// list moved on purpose (priorityNote, priority.go).
//
// The note names the mark the pressed row is about rather than what changed,
// which is why act is passed in rather than the two states being compared. The
// three priority rows are radios: pressing the level a prompt already holds is
// a no-op by design, and answering that press with silence — or worse, with a
// sentence about the quick-win flag — would read as a dead control.
func (m model) setMenuAnnots(ref todoRef, act int) (tea.Model, tea.Cmd) {
	td, ok := m.resolve(ref)
	if !ok {
		m.setStatus("could not find that prompt", true)
		return m, nil
	}
	a := annotsOf(td)
	switch act {
	case listMenuFruit:
		a.Fruit = !a.Fruit
	case listMenuFlag:
		// Only the mark: the note is left exactly as it was, and applyTo drops
		// it with the flag on the way to the file (see annots.applyTo), so a
		// prompt unflagged from here does not keep words nothing draws.
		a.Flag = !a.Flag
	default:
		a.Priority = listMenuPrio[act]
	}
	if err := m.storeFor(ref.scope).setAnnots(ref.id, a); err != nil {
		m.setStatus("save failed: "+err.Error(), true)
		return m, nil
	}
	m.rebuildList()
	m.selectRow(ref)
	switch {
	case act == listMenuFruit && a.Fruit:
		m.setStatus("marked a quick win", false)
	case act == listMenuFruit:
		m.setStatus("no longer a quick win", false)
	case act == listMenuFlag && a.Flag:
		// The invitation is the point: the row can only raise the mark, and
		// someone who wanted to say why has to be told where that is done.
		m.setStatus("flagged — enter to add a note", false)
	case act == listMenuFlag:
		m.setStatus("flag cleared", false)
	default:
		// The lens only reorders when the level actually moved; pressing the
		// row a prompt already sits on leaves the list exactly where it was.
		m.setStatus(priorityNote(a.Priority, m.orderByPriority && a.Priority != td.Priority), false)
	}
	return m, nil
}

// --- Drawing --------------------------------------------------------------------

// overlayListMenu floats the menu over the list's rendered frame (see
// overlayMenu, menu.go, for why it is composited rather than spliced).
func (m model) overlayListMenu(view string) string {
	return overlayMenu(view, m.listMenu.menuBox)
}
