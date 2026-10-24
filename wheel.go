// wheel.go — the mouse wheel on every windowed list.
//
// The screens with something to click ask for mouse reporting (see View), and
// a terminal reporting the mouse hands the wheel to the program as button 4/5
// presses instead of scrolling anything itself; cats' encoder does exactly
// that. With no handler the wheel was a message nobody read, which went
// unnoticed while one row per prompt mostly fit the pane — and became "scroll
// stops working" once ☑ Soft wrap made every row as tall as its text, so the
// backlog and the Next List routinely outgrew the pane and the wheel was the
// natural way down them.
//
// A notch scrolls the window one row (fuzzyList.scrollBy), not the highlight:
// the page moves under the pointer the way it does in a pager or a browser, and
// the highlight is only dragged along when it would otherwise leave the
// screen. One row rather than one line because the window's top is a row — a
// wrapped row has no way to be drawn half scrolled off — and a tall row is
// then one notch, which is the right feel for "the next prompt".
//
// Which lists answer it: every one that keeps a scroll window — the backlog,
// the Next List, the Batches page, the composer's two panes, the @ file picker
// and the snippets picker. The lists without a window (the drop picker, the
// spelling panel, export, import) draw every row and leave the pane to clip
// them, so there is no window for a wheel to move; they take the wheel the
// way they always have, by ignoring it.
package main

import tea "charm.land/bubbletea/v2"

// wheelRows is the window's motion for one wheel event: up is back toward the
// top of the list, down further into it. Sideways (buttons 6/7) means nothing
// here — none of these lists scrolls horizontally.
func wheelRows(msg tea.MouseWheelMsg) int {
	switch msg.Button {
	case tea.MouseWheelUp:
		return -1
	case tea.MouseWheelDown:
		return 1
	}
	return 0
}

// wheelList scrolls the backlog's window. Anything modal on the stage — the
// context menu, the flag's note pad — swallows the wheel instead: each is
// anchored to a row, and scrolling the rows out from under it would leave it
// pointing at a different prompt from the one it is about. A row drag owns the
// pointer until release, and the window moving mid-drag would change which row
// the drop lands on without the hand having moved.
//
// The hover card comes down whether or not anything scrolled: it was placed
// against a row that has moved (or, at an end of the list, the hand is plainly
// doing something other than reading the card).
func (m model) wheelList(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.listMenu.open || m.flagPad.open || m.dragging {
		return m, nil
	}
	m.clearHover()
	m.list.scrollBy(wheelRows(msg))
	return m, nil
}

// wheelNext is wheelList for the Next List page, under the same rules: its
// menu and its note pad are both anchored to an item.
func (m model) wheelNext(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.nextMenu.open || m.nextPad.open {
		return m, nil
	}
	m.clearHover()
	m.next.list.scrollBy(wheelRows(msg))
	return m, nil
}

// wheelBatches is the Batches page: its menu is anchored to a batch, as the
// list's is to a prompt. The armed delete needs no clearing even when the
// highlight is dragged to another batch — it is keyed by the batch's ID, so a
// second ctrl+x on a different row arms that one instead of deleting.
func (m model) wheelBatches(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.batchesMenu.open {
		return m, nil
	}
	m.batches.list.scrollBy(wheelRows(msg))
	return m, nil
}

// wheelBatchCompose is the composer, where two panes scroll side by side, so
// the wheel goes to the one under the pointer — the same pane test a click
// makes (clickBatchCompose), the splitter's cells belonging to neither. In the
// narrow layout only one pane is drawn, and it takes the wheel wherever the
// pointer is. The drop dialog is modal over the whole stage, and a row drag or
// a splitter drag owns the pointer until release.
func (m model) wheelBatchCompose(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	bc := &m.batch
	if bc.confirm.open || bc.dragging || bc.splitDrag {
		return m, nil
	}
	m.clearHover() // the Pick pane's card, placed against a row that moves
	g := m.batchGeom()
	n := wheelRows(msg)
	switch {
	case g.showPick && (!g.split || msg.X < g.pickW):
		bc.pick.scrollBy(n)
	case g.showBatch && (!g.split || msg.X >= g.batchX):
		m.scrollBatchPane(n, g.batchRows)
	}
	return m, nil
}

// scrollBatchPane is scrollBy for the composer's Batch pane, which keeps its
// own top and cursor over bc.picked (one line per prompt, rows-many drawn)
// rather than a fuzzyList: top moves within the range ensureBatchVisible
// clamps it to, and the cursor is dragged only as far as the window's nearer
// edge.
func (m *model) scrollBatchPane(n, rows int) {
	bc := &m.batch
	if rows <= 0 || len(bc.picked) <= rows {
		return
	}
	bc.top = min(max(bc.top+n, 0), len(bc.picked)-rows)
	bc.cursor = min(max(bc.cursor, bc.top), bc.top+rows-1)
}

// wheelFiles and wheelSnippets are the editor's two pickers: plain item
// windows with nothing modal over them.
func (m model) wheelFiles(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	m.files.list.scrollBy(wheelRows(msg))
	return m, nil
}

func (m model) wheelSnippets(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	m.snips.list.scrollBy(wheelRows(msg))
	return m, nil
}
