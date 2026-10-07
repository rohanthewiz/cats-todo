// batchhover.go — the batch composer's hover card, over the Pick pane.
//
// A Pick row is a title cut to the pane (rebuildBatchPick), which is exactly
// the question the list's card answers: "which one was this again?". Picking
// a batch is the moment that question matters most, since every row ticked
// here is a prompt about to be sent, so the composer floats the same card the
// list and the Next List page do — the backlog's card for a backlog prompt
// (buildHoverCard), the Next List's for an item (nextCardFor) — with the same
// dwell, warm window and teardown rules (listhover.go).
//
// The one difference is the checkbox. A Pick row is a choice, and its box is
// the control the hand is reaching for:
//
//	 ❯[x] Fix flaky drop test
//	  ^^^
//	  └─ cells indentWidth … indentWidth+len("[x]"): no card here
//
// A card opening while the pointer rests on the box would land beside the
// thing about to be pressed, describing the row instead of getting out of the
// way of the press. So those three cells count as "off the rows": the card
// comes down, the warm window is left as it was, and moving on to the title
// of the same row is an ordinary arrival.
//
// The Batch pane (the right one) gets no card: its rows are the picks the Pick
// pane already described, and a press there starts a drag, which is the
// gesture the card would be floating over.

package main

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// batchHoverRow is the Pick pane row under (x, y), or false when the pointer
// is on anything that should not have a card: the other pane, the splitter,
// the tabs/query/"all" lines above the rows, a heading, or a row's checkbox.
//
// It reads the same geometry the view and the click hit-test read
// (batchGeom), so the row the card names is the row a press would toggle.
func (m model) batchHoverRow(x, y int) (int, bool) {
	g := m.batchGeom()
	// The Pick pane always starts at column 0; side by side it ends where the
	// splitter does, and alone it has the whole width.
	if !g.showPick || (g.split && x >= g.pickW) {
		return 0, false
	}
	if y < g.pickRowsY || y >= g.barY-1 {
		return 0, false
	}
	// The checkbox column sits right after the cursor's two cells, on every
	// row (fuzzyList.checkboxes). Measured off the glyph rather than hard-coded
	// so a restyled box moves its dead zone with it.
	if box := lipgloss.Width(checkOff); x >= indentWidth && x < indentWidth+box {
		return 0, false
	}
	return m.batch.pick.rowAtLine(y - g.pickRowsY)
}

// batchHoverMotion is hoverMotion's twin for the composer; the three-way
// split (same row: nothing; the row already waited for: follow the pointer;
// a new row: warm → card now, cold → arm the dwell) is documented there.
//
// The composer's surfaces that refuse a card are the drop dialog (modal, and
// a card under it would be read through it) and the two drags, where the
// gesture is about where something is going, not what it says.
func (m model) batchHoverMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	bc := m.batch
	if bc.confirm.open || bc.dragging || bc.splitDrag {
		m.clearHover()
		return m, nil
	}
	i, ok := m.batchHoverRow(msg.X, msg.Y)
	if !ok {
		// Off the rows — or on a checkbox. Either way the pointer is still
		// travelling, so the warm window stays as it was (hoverMovedOn).
		m.hoverMovedOn()
		return m, nil
	}
	if m.hover.open && m.hover.row == i {
		return m, nil
	}
	if m.hoverPend.armed && m.hoverPend.row == i && m.hoverPend.stage == stageBatchCompose {
		m.hoverPend.x, m.hoverPend.y = msg.X, msg.Y
		return m, nil
	}
	warm := m.hoverIsWarm() // asked before the teardown, which is what opens it
	m.hoverMovedOn()
	if warm {
		if card, ok := m.batchCardFor(i, msg.X, msg.Y); ok {
			m.hover = card
		}
		return m, nil
	}
	m.hoverGen++
	m.hoverPend = hoverPending{armed: true, stage: stageBatchCompose, row: i, x: msg.X, y: msg.Y, gen: m.hoverGen}
	return m, hoverTick(m.hoverGen)
}

// batchHoverDwell is hoverDwell's composer branch: the rest has lasted, so
// the card is built — unless something that refuses one has turned up during
// the wait, or the pointer's last reported cell is now a checkbox (the Pick
// pane can have been re-laid-out under a still pointer by a resize or a
// splitter drag).
func (m model) batchHoverDwell(p hoverPending) (tea.Model, tea.Cmd) {
	bc := m.batch
	if bc.confirm.open || bc.dragging || bc.splitDrag {
		return m, nil
	}
	if i, ok := m.batchHoverRow(p.x, p.y); !ok || i != p.row {
		return m, nil
	}
	if card, ok := m.batchCardFor(p.row, p.x, p.y); ok {
		m.hover = card
	}
	return m, nil
}

// batchCardFor resolves a filtered Pick row to what it stands for and builds
// that thing's own card: the backlog card for a prompt, read from the store
// at this moment (a peer may have edited it since the pane was built), and
// the Next List card for an item.
func (m model) batchCardFor(row, x, y int) (hoverCard, bool) {
	bc := m.batch
	idx, ok := bc.pick.refAt(row)
	if !ok || idx < 0 || idx >= len(bc.cands) {
		return hoverCard{}, false
	}
	c := bc.cands[idx]
	if c.next.ID != "" {
		return m.nextItemCard(c.next, row, x, y)
	}
	td, ok := m.resolve(c.ref)
	if !ok {
		return hoverCard{}, false
	}
	return m.buildHoverCard(td, row, x, y)
}
