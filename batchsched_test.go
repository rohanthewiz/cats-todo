package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// --- Phase 2: scheduled batches ------------------------------------------------------

// scheduledBatch writes a scheduled batch holding the named prompts, due at at,
// into the project's batches.json, and returns it as written.
func scheduledBatch(t *testing.T, m model, id string, at time.Time, titles ...string) Batch {
	t.Helper()
	b := launchFrom(m, deliverEach, titles...)
	b.ID, b.State, b.At = id, batchScheduled, at
	if err := batchStoreFor(m.project).put(b); err != nil {
		t.Fatal(err)
	}
	return b
}

// readBatch is the record with id as it now stands on disk.
func readBatch(t *testing.T, s *store, id string) (Batch, bool) {
	t.Helper()
	bs := batchStoreFor(s)
	if err := bs.load(); err != nil {
		t.Fatal(err)
	}
	for _, b := range bs.batches {
		if b.ID == id {
			return b, true
		}
	}
	return Batch{}, false
}

// TestBatchSwapIsAClaim: a swap succeeds only against the record as it was
// read. The second of two panes holding the same scheduled batch loses, and a
// deleted record is a lost swap rather than an insert.
func TestBatchSwapIsAClaim(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	at := time.Now().Add(time.Hour).Truncate(time.Second)
	b := scheduledBatch(t, m, "s1", at, "Rename headings")
	bs := batchStoreFor(project)

	running := b
	running.State = batchRunning
	if won, err := bs.swapBatch(b, running); err != nil || !won {
		t.Fatalf("first swap: won %v err %v", won, err)
	}
	// The other pane still holds the scheduled copy.
	if won, _ := bs.swapBatch(b, running); won {
		t.Error("a swap against a stale record won")
	}
	if won, _ := bs.swapBatch(Batch{ID: "gone", State: batchScheduled}, running); won {
		t.Error("a swap against a missing record won")
	}
	if n := len(bs.batches); n != 1 {
		t.Errorf("records = %d, want 1 — a lost swap must not insert", n)
	}
}

// TestBatchWatchSeesEveryWrite: the tick's cache re-reads a file another write
// has changed, and forgets one that was deleted.
func TestBatchWatchSeesEveryWrite(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	w := &batchWatch{}
	bs := batchStoreFor(project)
	if got, _ := w.read(bs); len(got) != 0 {
		t.Fatalf("no file yet, read %d batches", len(got))
	}
	scheduledBatch(t, m, "s1", time.Now().Add(time.Hour), "Rename headings")
	if got, _ := w.read(batchStoreFor(project)); len(got) != 1 {
		t.Fatalf("after the first write, read %d batches, want 1", len(got))
	}
	scheduledBatch(t, m, "s2", time.Now().Add(2*time.Hour), "Fix flaky drop test")
	if got, _ := w.read(batchStoreFor(project)); len(got) != 2 {
		t.Errorf("after the second write, read %d batches, want 2 — a stale snapshot", len(got))
	}
}

// TestPendingRefsOnlyScheduled: only a scheduled batch speaks for its prompts,
// and a prompt in two of them wears the earlier time.
func TestPendingRefsOnlyScheduled(t *testing.T) {
	early, late := time.Now().Add(time.Hour), time.Now().Add(3*time.Hour)
	item := BatchItem{Scope: batchScopeProject, ID: "p1"}
	got := pendingRefs([]Batch{
		{State: batchScheduled, At: late, Items: []BatchItem{item}},
		{State: batchScheduled, At: early, Items: []BatchItem{item}},
		{State: batchMissed, At: early, Items: []BatchItem{{Scope: batchScopeProject, ID: "p2"}}},
		{State: batchDone, Items: []BatchItem{{Scope: batchScopeProject, ID: "p3"}}},
	})
	if len(got) != 1 || !got[item.ref()].Equal(early) {
		t.Errorf("pending = %v, want only p1 at the earlier time", got)
	}
}

// TestComposerWhenPicksTheButton: an empty When row means now, and a time
// means later. The button that doesn't apply refuses in words instead of
// quietly doing the other thing.
func TestComposerWhenPicksTheButton(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	m.client = &catsClient{}
	m = openComposer(t, m)
	m.toggleBatchCand(candIndex(t, m, "Rename headings"))

	if why := m.batchDropWhy(); why != "" {
		t.Fatalf("empty When: drop refused with %q", why)
	}
	if _, why := m.batchScheduleWhy(time.Now()); !strings.Contains(why, "When row") {
		t.Errorf("empty When: schedule refusal = %q, want it to point at the When row", why)
	}
	// Pressing Schedule anyway moves the keys to the row it asked about.
	next, _ := m.updateBatchCompose(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	nm := next.(model)
	if nm.batch.focus != batchFocusSettings || nm.batch.setRow != batchSetWhen || !nm.batch.noteErr {
		t.Errorf("schedule with no time: focus %d row %d note %q", nm.batch.focus, nm.batch.setRow, nm.batch.note)
	}

	m.batch.when.SetValue("in 2h")
	if why := m.batchDropWhy(); !strings.Contains(why, "Schedule") {
		t.Errorf("When set: drop refusal = %q, want it to name ◷ Schedule", why)
	}
	if at, why := m.batchScheduleWhy(time.Now()); why != "" || at.Before(time.Now().Add(119*time.Minute)) {
		t.Errorf("When set: schedule = %v %q", at, why)
	}
	m.batch.when.SetValue("half past never")
	if _, why := m.batchScheduleWhy(time.Now()); !strings.Contains(why, "can't read") {
		t.Errorf("unreadable When: refusal = %q", why)
	}
}

// TestScheduleBatchWritesAPlan: ◷ Schedule writes a scheduled record and
// sends nothing. The composer closes, and the list's rows for the batch's
// prompts wear the ⧉ mark with the fire time.
func TestScheduleBatchWritesAPlan(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	m.client = &catsClient{}
	m = openComposer(t, m)
	m.toggleBatchCand(candIndex(t, m, "Rename headings"))
	m.toggleBatchCand(candIndex(t, m, "Global task"))
	m.batch.when.SetValue("in 2h")

	next, cmd := m.updateBatchCompose(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	m = next.(model)
	if cmd != nil || m.dropping || m.stage != stageList {
		t.Fatalf("schedule: cmd %v dropping %v stage %v (note %q)", cmd != nil, m.dropping, m.stage, m.batch.note)
	}
	bs := batchStoreFor(project)
	if err := bs.load(); err != nil || len(bs.batches) != 1 {
		t.Fatalf("records: %+v (err %v)", bs.batches, err)
	}
	rec := bs.batches[0]
	if rec.State != batchScheduled || rec.At.Before(time.Now().Add(119*time.Minute)) || !rec.Dropped.IsZero() {
		t.Errorf("record: state %q at %v dropped %v", rec.State, rec.At, rec.Dropped)
	}
	// Nothing is done yet: the prompts are only spoken for.
	marked := 0
	for _, it := range m.list.items {
		for _, mk := range it.descMarks {
			if strings.HasPrefix(mk.text, "⧉ ") {
				marked++
			}
		}
	}
	if marked != 2 {
		t.Errorf("rows with ⧉ = %d, want the batch's 2", marked)
	}
	for _, td := range project.todos {
		if td.Done {
			if td.ID != "pd" {
				t.Errorf("%q was marked done by scheduling", td.Title)
			}
		}
	}
}

// TestFireDueBatches walks the tick's rules: a batch inside its grace is
// claimed and started; a batch past it is marked missed with the reason; a
// drop in flight makes it wait; a batch open in the composer is left alone.
func TestFireDueBatches(t *testing.T) {
	now := time.Now()

	t.Run("fires inside the grace", func(t *testing.T) {
		m, project, _ := batchModel(t, 120, 30)
		m.client = &catsClient{}
		scheduledBatch(t, m, "due", now.Add(-30*time.Second), "Rename headings", "Fix flaky drop test")
		cmd := m.fireDueBatches(now)
		if cmd == nil || !m.dropping || m.batchRun == nil || !m.batchRun.fired || len(m.batchRun.steps) != 2 {
			t.Fatalf("fire: cmd %v dropping %v run %+v", cmd != nil, m.dropping, m.batchRun)
		}
		rec, _ := readBatch(t, project, "due")
		if rec.State != batchRunning || rec.Dropped.IsZero() || rec.At.IsZero() {
			t.Errorf("record after the claim: %+v, want running, dropped, and its fire time kept", rec)
		}
	})

	t.Run("missed past the grace", func(t *testing.T) {
		m, project, _ := batchModel(t, 120, 30)
		m.client = &catsClient{}
		scheduledBatch(t, m, "late", now.Add(-10*time.Minute), "Rename headings")
		if cmd := m.fireDueBatches(now); cmd != nil || m.dropping {
			t.Fatal("a batch past its grace was fired")
		}
		rec, _ := readBatch(t, project, "late")
		if rec.State != batchMissed || !strings.Contains(rec.Why, "not open") {
			t.Errorf("record = state %q why %q, want missed with the reason", rec.State, rec.Why)
		}
		if !m.statusErr || !strings.Contains(m.status, "missed batch") {
			t.Errorf("status = %q", m.status)
		}
	})

	t.Run("waits for a drop in flight", func(t *testing.T) {
		m, project, _ := batchModel(t, 120, 30)
		m.client = &catsClient{}
		scheduledBatch(t, m, "due", now.Add(-time.Second), "Rename headings")
		m.dropping = true
		if cmd := m.fireDueBatches(now); cmd != nil {
			t.Fatal("fired over a drop in flight")
		}
		if rec, _ := readBatch(t, project, "due"); rec.State != batchScheduled {
			t.Errorf("state = %q, want still scheduled", rec.State)
		}
	})

	t.Run("leaves an open edit alone", func(t *testing.T) {
		m, project, _ := batchModel(t, 120, 30)
		m.client = &catsClient{}
		b := scheduledBatch(t, m, "due", now.Add(-10*time.Minute), "Rename headings")
		m.batch.edit = b
		if cmd := m.fireDueBatches(now); cmd != nil {
			t.Fatal("fired a batch open in the composer")
		}
		if rec, _ := readBatch(t, project, "due"); rec.State != batchScheduled {
			t.Errorf("state = %q, want untouched while being edited", rec.State)
		}
	})

	t.Run("not yet due", func(t *testing.T) {
		m, project, _ := batchModel(t, 120, 30)
		m.client = &catsClient{}
		scheduledBatch(t, m, "later", now.Add(time.Hour), "Rename headings")
		if cmd := m.fireDueBatches(now); cmd != nil {
			t.Fatal("fired early")
		}
		if rec, _ := readBatch(t, project, "later"); rec.State != batchScheduled {
			t.Errorf("state = %q", rec.State)
		}
	})
}

// TestFiredBatchReadsTheBacklogAfresh: a prompt completed in another pane
// after the batch was scheduled is skipped at fire time, though this pane's
// copy of the backlog never saw the change.
func TestFiredBatchReadsTheBacklogAfresh(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	m.client = &catsClient{}
	b := scheduledBatch(t, m, "due", time.Now().Add(-time.Second), "Rename headings", "Fix flaky drop test")

	// Another pane: its own store on the same file marks one done.
	other := &store{scope: scopeProject, path: project.path}
	if err := other.setDone(b.Items[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if cmd := m.fireDueBatches(time.Now()); cmd == nil {
		t.Fatal("did not fire")
	}
	if n := len(m.batchRun.steps); n != 1 {
		t.Errorf("steps = %d, want the completed prompt skipped", n)
	}
	if run, ok := m.batchRun.batch.itemRun(0); !ok || !strings.Contains(run.Err, "completed") {
		t.Errorf("skipped item's run = %+v", run)
	}
}

// openPlan opens batch id from the Batches page, the way enter does.
func openPlan(t *testing.T, m model, id string) model {
	t.Helper()
	m.openBatchesPage()
	for i, b := range m.batches.rows {
		if b.ID == id {
			m.batches.list.selectRef(i)
		}
	}
	next, _ := m.updateBatches(tea.KeyPressMsg{Code: tea.KeyEnter})
	return next.(model)
}

// TestEditScheduledBatch: enter on a scheduled batch opens it in the composer
// with its time on the When row, and saving writes over the same record.
func TestEditScheduledBatch(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	m.client = &catsClient{}
	at := time.Now().Add(time.Hour).Truncate(time.Minute).Add(time.Minute)
	scheduledBatch(t, m, "s1", at, "Rename headings")

	m = openPlan(t, m, "s1")
	if m.stage != stageBatchCompose || m.batch.edit.ID != "s1" {
		t.Fatalf("enter on a scheduled batch: stage %v edit %q", m.stage, m.batch.edit.ID)
	}
	if got, err := parseScheduleTime(m.batch.when.Value(), time.Now()); err != nil || !got.Equal(at) {
		t.Errorf("When prefill %q reads back as %v (%v), want %v", m.batch.when.Value(), got, err, at)
	}
	m.toggleBatchCand(candIndex(t, m, "Fix flaky drop test"))
	m.batch.when.SetValue("in 3h")
	next, _ := m.updateBatchCompose(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	m = next.(model)
	if m.stage != stageBatches {
		t.Fatalf("after saving the edit: stage %v note %q", m.stage, m.batch.note)
	}
	bs := batchStoreFor(project)
	if err := bs.load(); err != nil || len(bs.batches) != 1 {
		t.Fatalf("records = %d (err %v), want the one, edited in place", len(bs.batches), err)
	}
	rec := bs.batches[0]
	if rec.ID != "s1" || len(rec.Items) != 2 || !rec.At.After(at) || rec.State != batchScheduled {
		t.Errorf("edited record: id %q items %d at %v state %q", rec.ID, len(rec.Items), rec.At, rec.State)
	}
}

// TestEditLosesToAFire: a batch that fired in another pane while it was open
// here is not written over (nor sent twice) — the save says what happened.
func TestEditLosesToAFire(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	m.client = &catsClient{}
	b := scheduledBatch(t, m, "s1", time.Now().Add(time.Hour), "Rename headings")
	m = openPlan(t, m, "s1")

	fired := b
	fired.State = batchRunning
	if won, err := batchStoreFor(project).swapBatch(b, fired); err != nil || !won {
		t.Fatal("the other pane's claim failed")
	}
	for _, key := range []tea.KeyPressMsg{{Code: 's', Mod: tea.ModCtrl}, {Code: tea.KeyEnter, Mod: tea.ModAlt}} {
		m.batch.when.SetValue("in 2h")
		if key.Mod == tea.ModAlt {
			m.batch.when.SetValue("")
		}
		next, cmd := m.updateBatchCompose(key)
		nm := next.(model)
		if cmd != nil || nm.stage != stageBatchCompose || nm.batch.note != errBatchChanged.Error() {
			t.Errorf("%s: cmd %v stage %v note %q", key.String(), cmd != nil, nm.stage, nm.batch.note)
		}
	}
	if rec, _ := readBatch(t, project, "s1"); rec.State != batchRunning {
		t.Errorf("record state = %q, want the other pane's running left alone", rec.State)
	}
}

// TestEditMovesAcrossFiles: a global batch that gains a project prompt moves
// to the project's file, since the global one cannot name a project prompt.
func TestEditMovesAcrossFiles(t *testing.T) {
	m, project, global := batchModel(t, 120, 30)
	m.client = &catsClient{}
	b := launchFrom(m, deliverEach, "Global task")
	b.ID, b.State, b.At, b.scope = "g1", batchScheduled, time.Now().Add(time.Hour), scopeGlobal
	if err := batchStoreFor(global).put(b); err != nil {
		t.Fatal(err)
	}
	m = openPlan(t, m, "g1")
	m.toggleBatchCand(candIndex(t, m, "Rename headings"))
	next, _ := m.updateBatchCompose(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	m = next.(model)
	if _, ok := readBatch(t, global, "g1"); ok {
		t.Error("the batch is still in the global file")
	}
	if rec, ok := readBatch(t, project, "g1"); !ok || len(rec.Items) != 2 {
		t.Errorf("project file holds %+v, want the moved batch with both prompts", rec)
	}
}

// TestUnscheduleKeepsThePlan: ✕ Unschedule takes the batch off the clock but
// keeps it, its prompts lose their ⧉, and enter reopens it to reschedule.
func TestUnscheduleKeepsThePlan(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	m.client = &catsClient{}
	scheduledBatch(t, m, "s1", time.Now().Add(time.Hour), "Rename headings")
	m.rebuildList()
	m.openBatchesPage()

	next, _ := m.updateBatches(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = next.(model)
	rec, _ := readBatch(t, project, "s1")
	if rec.State != batchUnscheduled || !rec.At.IsZero() {
		t.Fatalf("record = state %q at %v, want unscheduled with no time", rec.State, rec.At)
	}
	for _, it := range m.list.items {
		for _, mk := range it.descMarks {
			if strings.HasPrefix(mk.text, "⧉") {
				t.Errorf("row %q still wears %q", it.name, mk.text)
			}
		}
	}
	// A second press has nothing to unschedule, and says so.
	next, _ = m.updateBatches(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if note := next.(model).batches.note; !strings.Contains(note, "only a scheduled batch") {
		t.Errorf("unschedule on an unscheduled batch: note %q", note)
	}
	m = openPlan(t, m, "s1")
	if m.stage != stageBatchCompose || m.batch.edit.ID != "s1" || m.batch.when.Value() != "" {
		t.Errorf("enter on an unscheduled batch: stage %v edit %q when %q", m.stage, m.batch.edit.ID, m.batch.when.Value())
	}
}

// TestBatchesPageOrdersPlansFirst: running on top, then scheduled soonest
// first, then the history newest first.
func TestBatchesPageOrdersPlansFirst(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	now := time.Now()
	bs := batchStoreFor(m.project)
	for _, b := range []Batch{
		{ID: "old", State: batchDone, Dropped: now.Add(-2 * time.Hour)},
		{ID: "later", State: batchScheduled, At: now.Add(3 * time.Hour)},
		{ID: "new", State: batchDone, Dropped: now.Add(-time.Hour)},
		{ID: "soon", State: batchScheduled, At: now.Add(time.Hour)},
		{ID: "run", State: batchRunning, Dropped: now.Add(-5 * time.Hour)},
	} {
		if err := bs.put(b); err != nil {
			t.Fatal(err)
		}
	}
	m.openBatchesPage()
	var got []string
	for _, b := range m.batches.rows {
		got = append(got, b.ID)
	}
	if want := "run soon later new old"; strings.Join(got, " ") != want {
		t.Errorf("rows = %v, want %s", got, want)
	}
}
