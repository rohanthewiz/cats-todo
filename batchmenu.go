// batchmenu.go — the Batches page's context menu.
//
// The page can do four things to a batch, and each is already a chip or a
// chord (batches.go). The menu is those same four, named on the batch that was
// pointed at, so a hand already on the mouse does not have to go to the bar
// and then back to the row it meant:
//
//	╭──────────────────────────────╮
//	│ ✎ Edit…                enter │   a plan: the composer, to change or drop it
//	│ ⧉ Duplicate…          ctrl+d │   a new batch starting from this one
//	│ ✕ Unschedule          ctrl+u │   ■ Stop on a running loop
//	│ ✖ Delete record…      ctrl+x │   ✖ Confirm delete once armed
//	╰──────────────────────────────╯
//
// The first row says what enter does on this batch: ✎ Edit… on a plan
// (scheduled, missed or unscheduled — Batch.editable), ☰ Open record on one
// that has gone. The third says what ctrl+u does: ✕ Unschedule, or ■ Stop on a
// running loop, the bar's own swap. A row names the press, which is the only
// thing a menu row promises (see listmenu.go).
//
// ＋ New and ← Back are about the page, not the batch, so they stay on the bar,
// as the backlog's menu leaves Import off and the Next List's leaves Refresh.
//
// Delete keeps the page's two-press rule rather than being made one press
// here. The rule exists because a record is cheap to lose but not free, and a
// menu row is as easy to press by mistake as a chord. The first press arms it
// (the heading says so), and the next menu opened on the same batch reads
// ✖ Confirm delete, which is the second press. A right-click does not disarm,
// so the two presses can each come from either road.
//
// The rows' refusals are the chords' own words (unscheduleWhy, deleteWhy),
// resolved when the menu opens so a row that cannot act is drawn dim — the
// dim-rather-than-omit rule every menu here follows (menu.go). The actions
// keep their guards too, since a tick can change a batch between the
// right-click and the press.

package main

import (
	tea "charm.land/bubbletea/v2"
)

// The menu's actions, which are also its row order.
const (
	batchesMenuOpen = iota
	batchesMenuDup
	batchesMenuUnsched
	batchesMenuDelete
)

// batchesMenu is the open menu: the shared box, plus the batch it was opened
// on, by ID.
//
// The ID rather than the row index, because the page's rows are re-read and
// re-sorted under an open menu: a running batch's progress reaches the page
// through batchStatus → reloadBatches, and a batch that fires or finishes
// changes tier (running sorts first). The row index the right-click landed on
// can then be a different batch by the time a row is pressed; the ID cannot.
type batchesMenu struct {
	menuBox
	id string
}

// rightClickBatches opens the menu on the batch row the press landed on.
//
// The press moves the highlight too, as every click on this page does: the
// batch the box is asking about is drawn selected while it is up, and the
// keyboard resumes from it afterwards. Off a row (the heading, the query box,
// the bar) it opens nothing and closes a menu that was open, so the right
// button aimed off the rows is still a way out of one.
//
// Like the other menus' right-clicks, it does not arm a double-click: that is
// a left-button gesture.
func (m model) rightClickBatches(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	i, ok := m.batches.list.rowAtLine(msg.Y - batchesRowsRow)
	if !ok || !m.batches.list.focusRow(i) {
		m.batchesMenu = batchesMenu{}
		return m, nil
	}
	b, ok := m.highlightedBatch()
	if !ok {
		m.batchesMenu = batchesMenu{}
		return m, nil
	}
	return m.openBatchesMenu(msg, b)
}

// openBatchesMenu builds the menu for one batch and opens it at the pointer.
func (m model) openBatchesMenu(msg tea.MouseClickMsg, b Batch) (tea.Model, tea.Cmd) {
	open := menuItem{act: batchesMenuOpen, label: "☰ Open record", hint: "enter"}
	if b.editable() {
		open.label = "✎ Edit…"
	}
	unsched := menuItem{act: batchesMenuUnsched, label: "✕ Unschedule", hint: "ctrl+u", why: unscheduleWhy(b)}
	if b.State == batchRunning && b.Deliver == deliverLoop {
		unsched.label = "■ Stop"
	}
	del := menuItem{act: batchesMenuDelete, label: "✖ Delete record…", hint: "ctrl+x", why: m.deleteWhy(b)}
	if m.batches.armDelete == b.ID {
		del.label = "✖ Confirm delete"
	}

	var mu batchesMenu
	mu.open, mu.id = true, b.ID
	mu.items = []menuItem{
		open,
		// No why: duplicateBatch's one refusal (every prompt done or gone)
		// needs the backlogs read, and it says so in words on the press.
		{act: batchesMenuDup, label: "⧉ Duplicate…", hint: "ctrl+d"},
		unsched,
		del,
	}
	mu.cursor = mu.firstLive()
	mu.size()
	mu.place(msg.X, msg.Y, m.width, m.height)
	m.batchesMenu = mu
	// The heading's note is the menu's answer channel, as on the Next List
	// page, so the last action's note goes now rather than being read as a
	// reply to this gesture. An armed delete's note is left: it is the
	// explanation of the ✖ Confirm delete row the box is about to show.
	if m.batches.armDelete != b.ID {
		m.batches.say("", false)
	}
	return m, nil
}

// updateBatchesMenu is the keyboard while the menu is up — the shared walk
// (see menuBox.key), with this menu's own answer to a press.
func (m model) updateBatchesMenu(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.batchesMenu.key(msg) {
	case menuKeyPress:
		return m.pressBatchesMenu(m.batchesMenu.cursor)
	case menuKeyClose:
		m.batchesMenu = batchesMenu{}
	}
	return m, nil
}

// clickBatchesMenu is the pointer while the menu is up: a row presses it, the
// border does nothing, and anywhere else dismisses without acting — the chip
// or row underneath must not also take a click that meant "never mind".
func (m model) clickBatchesMenu(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if !m.batchesMenu.inside(msg.X, msg.Y) {
		m.batchesMenu = batchesMenu{}
		return m, nil
	}
	row, ok := m.batchesMenu.hit(msg.X, msg.Y)
	if !ok {
		return m, nil
	}
	return m.pressBatchesMenu(row)
}

// pressBatchesMenu runs row i on the batch the menu was opened for.
//
// The page's actions all work on the highlighted batch, so the highlight is
// first put back on the menu's batch by ID (see batchesMenu) and the action is
// then the very function its chord runs — one code path, with its own guards
// and its own words. A batch deleted from another pane meanwhile is said
// rather than acted on in its place.
func (m model) pressBatchesMenu(i int) (tea.Model, tea.Cmd) {
	if i < 0 || i >= len(m.batchesMenu.items) {
		return m, nil
	}
	row, id := m.batchesMenu.items[i], m.batchesMenu.id
	m.batchesMenu = batchesMenu{}
	if !row.live() {
		m.batches.say(row.why, false)
		return m, nil
	}
	if !m.focusBatch(id) {
		m.batches.say("that batch is gone — deleted in another pane; the rows are re-read", true)
		return m, nil
	}
	// Any row but Delete disarms a pending delete, as any chip but ✖ Delete
	// does (pressBatches).
	if row.act != batchesMenuDelete {
		m.batches.armDelete = ""
	}
	switch row.act {
	case batchesMenuOpen:
		return m.beginBatchView()
	case batchesMenuDup:
		return m.duplicateBatch()
	case batchesMenuUnsched:
		return m.unscheduleBatch()
	case batchesMenuDelete:
		return m.deleteBatch()
	}
	return m, nil
}

// focusBatch re-reads the rows and moves the page's highlight onto the batch
// with this ID, reporting whether it is still there.
//
// The re-read is unconditional. Looking in the rows already held first would
// find a batch another pane deleted (or fired, or edited) since the page last
// read the files, and the press would then act on that stale copy. Two small
// JSON files are cheap next to a press on the wrong version of a record.
func (m *model) focusBatch(id string) bool {
	m.reloadBatches()
	for i, b := range m.batches.rows {
		if b.ID == id {
			m.batches.list.selectRef(i)
			return true
		}
	}
	return false
}

// overlayBatchesMenu floats the menu over the page's rendered frame (see
// overlayMenu, menu.go, for why it is composited rather than spliced).
func (m model) overlayBatchesMenu(view string) string {
	return overlayMenu(view, m.batchesMenu.menuBox)
}
