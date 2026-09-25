package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// batchesRowY is the screen line the page draws batch id's row on, found
// through the same hit-test a click uses, so the tests aim where a hand would.
func batchesRowY(t *testing.T, m model, id string) int {
	t.Helper()
	for line := 0; line < m.height; line++ {
		i, ok := m.batches.list.rowAtLine(line)
		if !ok {
			continue
		}
		if ref := m.batches.list.filtered[i].item.ref; m.batches.rows[ref].ID == id {
			return batchesRowsRow + line
		}
	}
	t.Fatalf("batch %q is not drawn on the page", id)
	return -1
}

// rightClickBatch opens the page's menu on batch id, through Update so
// updateMouse's routing is under test too.
func rightClickBatch(t *testing.T, m model, id string) model {
	t.Helper()
	next, _ := m.Update(tea.MouseClickMsg{X: 8, Y: batchesRowY(t, m, id), Button: tea.MouseRight})
	got := next.(model)
	if !got.batchesMenu.open {
		t.Fatalf("a right-click on %q did not open the menu", id)
	}
	return got
}

// batchesMenuRow is the menu row whose action is act.
func batchesMenuRow(t *testing.T, m model, act int) menuItem {
	t.Helper()
	for _, it := range m.batchesMenu.items {
		if it.act == act {
			return it
		}
	}
	t.Fatalf("menu has no row for action %d", act)
	return menuItem{}
}

// pressBatchesMenuAct presses the row for act with the keyboard: walk the
// cursor there, then enter — the road a hand on the keys takes.
func pressBatchesMenuAct(t *testing.T, m model, act int) model {
	t.Helper()
	for i, it := range m.batchesMenu.items {
		if it.act == act {
			m.batchesMenu.cursor = i
			next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			return next.(model)
		}
	}
	t.Fatalf("menu has no row for action %d", act)
	return m
}

// batchesPageWith puts records on disk and opens the page on them.
func batchesPageWith(t *testing.T, bs ...Batch) model {
	t.Helper()
	m, _, _ := batchModel(t, 120, 30)
	for _, b := range bs {
		if err := batchStoreFor(m.project).put(b); err != nil {
			t.Fatal(err)
		}
	}
	m.openBatchesPage()
	m.stage = stageBatches
	return m
}

// TestBatchesMenuOpensOnTheRightButton is the feature: a right-click on a
// batch opens the menu on it, moves the highlight there, and is drawn over the
// page. The rows name what the press will do on this batch — a plan is
// edited, a record opened.
func TestBatchesMenuOpensOnTheRightButton(t *testing.T) {
	now := time.Now()
	m := batchesPageWith(t,
		Batch{ID: "plan", Name: "plan", State: batchScheduled, At: now.Add(time.Hour)},
		Batch{ID: "rec", Name: "rec", State: batchDone, Dropped: now.Add(-time.Hour)},
	)
	m = rightClickBatch(t, m, "rec")
	if m.batchesMenu.id != "rec" {
		t.Errorf("menu opened on %q, want rec", m.batchesMenu.id)
	}
	if b, _ := m.highlightedBatch(); b.ID != "rec" {
		t.Errorf("highlight on %q, want it moved to the right-clicked rec", b.ID)
	}
	frame := ansi.Strip(m.renderStage())
	for _, want := range []string{"☰ Open record", "⧉ Duplicate…", "✕ Unschedule", "✖ Delete record…"} {
		if !strings.Contains(frame, want) {
			t.Errorf("drawn page is missing the row %q:\n%s", want, frame)
		}
	}
	// A record has no time to take off: the row is dim, in the chord's words.
	if why := batchesMenuRow(t, m, batchesMenuUnsched).why; !strings.Contains(why, "only a scheduled batch") {
		t.Errorf("Unschedule on a record: why %q", why)
	}

	m = rightClickBatch(t, m, "plan")
	if got := batchesMenuRow(t, m, batchesMenuOpen).label; got != "✎ Edit…" {
		t.Errorf("first row on a plan = %q, want ✎ Edit…", got)
	}
	if !batchesMenuRow(t, m, batchesMenuUnsched).live() {
		t.Error("Unschedule is dim on a scheduled batch")
	}
}

// TestBatchesMenuOnlyOverABatch: the heading, the query box and the bar open
// nothing, and a right-click there closes a menu that was open.
func TestBatchesMenuOnlyOverABatch(t *testing.T) {
	m := batchesPageWith(t, Batch{ID: "rec", State: batchDone, Dropped: time.Now()})
	for _, y := range []int{0, 2, batchesBarRow} {
		next, _ := m.Update(tea.MouseClickMsg{X: 8, Y: y, Button: tea.MouseRight})
		if next.(model).batchesMenu.open {
			t.Errorf("a right-click on line %d opened a menu, want only batch rows to", y)
		}
	}
	m = rightClickBatch(t, m, "rec")
	next, _ := m.Update(tea.MouseClickMsg{X: 8, Y: batchesBarRow, Button: tea.MouseRight})
	if next.(model).batchesMenu.open {
		t.Error("a right-click off the rows left the menu open")
	}
}

// TestBatchesMenuClickOffDismissesOnly: a left click off the box closes it and
// nothing under it acts — here the ✖ Delete chip, which would otherwise arm.
func TestBatchesMenuClickOffDismissesOnly(t *testing.T) {
	m := batchesPageWith(t, Batch{ID: "rec", State: batchDone, Dropped: time.Now()})
	m = rightClickBatch(t, m, "rec")
	chip := m.batchesChips()[batchesBtnDelete]
	next, _ := m.Update(tea.MouseClickMsg{X: chip.start, Y: batchesBarRow, Button: tea.MouseLeft})
	m = next.(model)
	if m.batchesMenu.open {
		t.Error("a click off the box left the menu open")
	}
	if m.batches.armDelete != "" {
		t.Error("the click that dismissed the menu also pressed the chip under it")
	}
}

// TestBatchesMenuDeleteTakesTwoPresses: the menu keeps the page's two-press
// rule. The first press arms, the next menu on the same batch reads
// ✖ Confirm delete, and that press deletes. A dim row says why and does
// nothing.
func TestBatchesMenuDeleteTakesTwoPresses(t *testing.T) {
	now := time.Now()
	m := batchesPageWith(t,
		Batch{ID: "a", State: batchDone, Dropped: now},
		Batch{ID: "b", State: batchDone, Dropped: now.Add(-time.Hour)},
	)
	m = rightClickBatch(t, m, "b")
	m = pressBatchesMenuAct(t, m, batchesMenuUnsched)
	if m.batchesMenu.open || !strings.Contains(m.batches.note, "only a scheduled batch") {
		t.Errorf("a dim row: open %v note %q", m.batchesMenu.open, m.batches.note)
	}

	m = rightClickBatch(t, m, "b")
	m = pressBatchesMenuAct(t, m, batchesMenuDelete)
	if len(m.batches.rows) != 2 || m.batches.armDelete != "b" {
		t.Fatalf("one press: rows %d armed %q", len(m.batches.rows), m.batches.armDelete)
	}
	m = rightClickBatch(t, m, "b")
	if got := batchesMenuRow(t, m, batchesMenuDelete).label; got != "✖ Confirm delete" {
		t.Errorf("armed delete row = %q", got)
	}
	// The armed note survives the right-click: it explains the row.
	if !strings.Contains(m.batches.note, "again to delete") {
		t.Errorf("the arming note was cleared by opening the menu: %q", m.batches.note)
	}
	m = pressBatchesMenuAct(t, m, batchesMenuDelete)
	if len(m.batches.rows) != 1 || m.batches.rows[0].ID != "a" {
		t.Errorf("after the second press: %+v", m.batches.rows)
	}
	if _, ok := readBatch(t, m.project, "b"); ok {
		t.Error("the record is still on disk")
	}
}

// TestBatchesMenuFollowsTheBatchNotTheRow is the regression the ID is carried
// for: the page's rows are re-read and re-sorted under an open menu (a
// running batch's progress does it through batchStatus), so the row index the
// right-click landed on can hold a different batch by the press.
func TestBatchesMenuFollowsTheBatchNotTheRow(t *testing.T) {
	now := time.Now()
	m := batchesPageWith(t,
		Batch{ID: "first", Name: "first", State: batchDone, Dropped: now},
		Batch{ID: "second", Name: "second", State: batchDone, Dropped: now.Add(-time.Hour)},
	)
	m = rightClickBatch(t, m, "second")
	// Another pane writes a newer record, which sorts first, and the page
	// re-reads: "second" moves down a row under the open menu.
	if err := batchStoreFor(m.project).put(Batch{ID: "newest", State: batchDone, Dropped: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	m.reloadBatches()
	m = pressBatchesMenuAct(t, m, batchesMenuOpen)
	if m.stage != stageBatchView || m.batches.view.ID != "second" {
		t.Errorf("Open record went to %v on %q, want the record of second", m.stage, m.batches.view.ID)
	}

	// Deleted from another pane between the right-click and the press: said,
	// and nothing else acted on.
	m.stage = stageBatches
	m = rightClickBatch(t, m, "first")
	if err := batchStoreFor(m.project).delete("first"); err != nil {
		t.Fatal(err)
	}
	m = pressBatchesMenuAct(t, m, batchesMenuOpen)
	if m.stage != stageBatches || !strings.Contains(m.batches.note, "gone") {
		t.Errorf("a vanished batch: stage %v note %q", m.stage, m.batches.note)
	}
}

// TestBatchesMenuRunningLoop: on a running loop the third row is ■ Stop, and
// Delete is dim while this manager drives the loop — the chord's refusal.
func TestBatchesMenuRunningLoop(t *testing.T) {
	m := batchesPageWith(t, Batch{ID: "lp", State: batchRunning, Deliver: deliverLoop, Dropped: time.Now()})
	m.loops = map[string]*loopRunner{"lp": {}}
	m = rightClickBatch(t, m, "lp")
	if row := batchesMenuRow(t, m, batchesMenuUnsched); row.label != "■ Stop" || !row.live() {
		t.Errorf("third row on a running loop = %q (why %q)", row.label, row.why)
	}
	if why := batchesMenuRow(t, m, batchesMenuDelete).why; !strings.Contains(why, "Stop it first") {
		t.Errorf("Delete on a driven loop: why %q", why)
	}
}

// TestBatchesMenuGoesWithTheScreen: a resize takes the box down (it was placed
// against the old pane), and so does leaving the page, so a box is never left
// to swallow the first key of the next visit.
func TestBatchesMenuGoesWithTheScreen(t *testing.T) {
	m := batchesPageWith(t, Batch{ID: "rec", State: batchDone, Dropped: time.Now()})
	m = rightClickBatch(t, m, "rec")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if next.(model).batchesMenu.open {
		t.Error("a resize left the menu open")
	}
	m = rightClickBatch(t, m, "rec")
	m.backToList()
	if m.batchesMenu.open {
		t.Error("leaving the page left the menu open")
	}
	m = rightClickBatch(t, batchesPageAgain(m), "rec")
	m.batchesMenu.open = true
	m.openBatchesPage()
	if m.batchesMenu.open {
		t.Error("opening the page kept a menu from the last visit")
	}
}

// batchesPageAgain reopens the page on the same model.
func batchesPageAgain(m model) model {
	m.openBatchesPage()
	return m
}
