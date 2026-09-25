package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// --- The pure layer ------------------------------------------------------------

// TestOverlaySessionBatchWinsPerField holds the precedence rule the composer
// promises: a field the batch sets wins, a field it leaves blank falls through
// to the prompt's own.
func TestOverlaySessionBatchWinsPerField(t *testing.T) {
	own := &SessionOpts{Model: "opus", Effort: effortHigh, Context: ctxUse, ContextArg: "drop", Finish: finishCommit}
	batch := &SessionOpts{Model: "sonnet", Finish: finishWrap}
	got := overlaySession(own, batch)
	if got == nil {
		t.Fatal("overlay of two configured records is nil")
	}
	if got.Model != "sonnet" || got.Finish != finishWrap {
		t.Errorf("batch fields did not win: %+v", *got)
	}
	if got.Effort != effortHigh || got.Context != ctxUse || got.ContextArg != "drop" {
		t.Errorf("blank batch fields did not fall through to the prompt's: %+v", *got)
	}
	// The prompt's own record is untouched: the overlay is a copy.
	if own.Model != "opus" {
		t.Errorf("overlay wrote into the prompt's record: %+v", *own)
	}
}

// TestOverlaySessionContextTravelsAsAPair: a batch that sets a context mode
// replaces the prompt's mode *and* its argument, since the argument was
// written for the prompt's mode.
func TestOverlaySessionContextTravelsAsAPair(t *testing.T) {
	own := &SessionOpts{Context: ctxUse, ContextArg: "drop-picker"}
	got := overlaySession(own, &SessionOpts{Context: ctxLoad})
	if got.Context != ctxLoad || got.ContextArg != "" {
		t.Errorf("context = %q %q, want the batch's load with its own (blank) argument", got.Context, got.ContextArg)
	}
}

// TestOverlaySessionDefaultsStayNil: nothing said on either side is the
// default session, which composePrompt's byte-for-byte contract depends on.
func TestOverlaySessionDefaultsStayNil(t *testing.T) {
	if got := overlaySession(nil, nil); got != nil {
		t.Errorf("overlay of nothing = %+v, want nil", *got)
	}
	if got := overlaySession(nil, &SessionOpts{}); got != nil {
		t.Errorf("overlay of an empty batch record = %+v, want nil", *got)
	}
}

// TestOverriddenFieldsOnlyRealReplacements: ✱ marks what the batch actually
// replaces — not a field the prompt never set, and not one the batch agrees on.
func TestOverriddenFieldsOnlyRealReplacements(t *testing.T) {
	own := &SessionOpts{Model: "opus", Effort: effortHigh}
	got := overriddenFields(own, &SessionOpts{Model: "sonnet", Effort: effortHigh, Finish: finishPush})
	if !slices.Equal(got, []string{"model"}) {
		t.Errorf("overridden = %v, want [model]", got)
	}
	if got := overriddenFields(nil, &SessionOpts{Model: "sonnet"}); len(got) != 0 {
		t.Errorf("a prompt with no options had %v overridden", got)
	}
}

// TestCombinedPromptNumbersTheTasks pins the combined body's shape: the intro,
// then one numbered heading per prompt in batch order, titles falling back to
// the prompt's first line.
func TestCombinedPromptNumbersTheTasks(t *testing.T) {
	got := combinedPrompt([]Todo{
		{Title: "Fix the test", Prompt: "  The flaky one.\n"},
		{Prompt: "Rename the headings\nin fuzzylist"},
	})
	want := fmt.Sprintf(combinedIntro, 2) +
		"\n\n## 1. Fix the test\n\nThe flaky one." +
		"\n\n## 2. Rename the headings\n\nRename the headings\nin fuzzylist"
	if got != want {
		t.Errorf("combined body:\n%s\n--- want ---\n%s", got, want)
	}
}

// TestBatchDisplayName: a name when there is one, else the first title and a
// count of the rest.
func TestBatchDisplayName(t *testing.T) {
	items := []BatchItem{{Title: "A"}, {Title: "B"}, {Title: "C"}}
	if got := (Batch{Items: items}).displayName(); got != "A +2" {
		t.Errorf("unnamed = %q, want %q", got, "A +2")
	}
	if got := (Batch{Name: " nightly ", Items: items}).displayName(); got != "nightly" {
		t.Errorf("named = %q", got)
	}
}

// TestBatchTargetRoundTrip: a picker row survives being recorded and read back,
// down to the worktree flag and the label the page shows.
func TestBatchTargetRoundTrip(t *testing.T) {
	in := dropTarget{kind: targetNewSession, command: "claude", worktree: true, label: "＋ New Claude Code session on a new worktree"}
	out := batchTargetFrom(in, "/proj").dropTarget()
	if out.kind != in.kind || out.command != in.command || !out.worktree || out.label != in.label {
		t.Errorf("round trip = %+v, want %+v", out, in)
	}
	pane := batchTargetFrom(dropTarget{kind: targetExistingPane, pane: 7, agent: "claude"}, "/proj").dropTarget()
	if pane.kind != targetExistingPane || pane.pane != 7 {
		t.Errorf("pane round trip = %+v", pane)
	}
}

// TestBatchStorePutLoadDelete: batches.json sits beside todos.json, put
// inserts then replaces by ID, and delete removes — each reloading first, the
// backlog store's discipline.
func TestBatchStorePutLoadDelete(t *testing.T) {
	s := tempStore(t)
	bs := batchStoreFor(s)
	if want := filepath.Join(filepath.Dir(s.path), batchesFileName); bs.path != want {
		t.Fatalf("path = %q, want %q", bs.path, want)
	}
	b := Batch{ID: "b1", Created: time.Now(), Items: []BatchItem{{Scope: batchScopeProject, ID: "t1", Title: "T"}}, State: batchRunning}
	if err := bs.put(b); err != nil {
		t.Fatal(err)
	}
	b.State = batchDone
	if err := bs.put(b); err != nil {
		t.Fatal(err)
	}
	fresh := batchStoreFor(s)
	if err := fresh.load(); err != nil {
		t.Fatal(err)
	}
	if len(fresh.batches) != 1 || fresh.batches[0].State != batchDone {
		t.Fatalf("after two puts: %+v, want one record, done", fresh.batches)
	}
	if err := fresh.delete("b1"); err != nil {
		t.Fatal(err)
	}
	if err := bs.load(); err != nil || len(bs.batches) != 0 {
		t.Errorf("after delete: %d records, err %v", len(bs.batches), err)
	}
}

// TestBatchLeavesTodosJSONAlone: making and saving a batch never writes the
// backlog file — the reason batches live in a file of their own.
func TestBatchLeavesTodosJSONAlone(t *testing.T) {
	s := tempStore(t)
	if err := s.add(Todo{ID: "t1", Title: "T", Prompt: "p"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.path)
	if err := batchStoreFor(s).put(Batch{ID: "b1", Items: []BatchItem{{ID: "t1"}}}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(s.path)
	if string(before) != string(after) {
		t.Error("saving a batch changed todos.json")
	}
}

// --- The composer ----------------------------------------------------------------

// batchModel is a temp-backed model with a few open prompts, one frozen, one
// done, and an info note, sized to a pane and ready to compose.
func batchModel(t *testing.T, w, h int) (model, *store, *store) {
	t.Helper()
	m, project, global := newModelInTemp(t)
	for i, title := range []string{"Fix flaky drop test", "Rename headings", "Add cleanup command"} {
		if err := project.add(Todo{ID: fmt.Sprintf("p%d", i), Title: title, Prompt: "do: " + title}); err != nil {
			t.Fatal(err)
		}
	}
	_ = project.add(Todo{ID: "pf", Title: "Frozen one", Prompt: "x", Frozen: true})
	_ = project.add(Todo{ID: "pd", Title: "Done one", Prompt: "x", Done: true})
	_ = global.add(Todo{ID: "g0", Title: "Global task", Prompt: "g"})
	_ = global.add(Todo{ID: "gi", Title: "A note", Prompt: "n", Info: true})
	m.rebuildList()
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(model), project, global
}

// openComposer opens the composer from the list on the backlog tab.
func openComposer(t *testing.T, m model) model {
	t.Helper()
	next, _ := m.beginBatchCompose(stageList, batchSrcBacklog, nil)
	return next.(model)
}

// candIndex is the Pick pane row for a title.
func candIndex(t *testing.T, m model, title string) int {
	t.Helper()
	for i, c := range m.batch.cands {
		if c.title == title {
			return i
		}
	}
	t.Fatalf("no pick row titled %q", title)
	return -1
}

func pickedTitles(m model) []string {
	var out []string
	for _, c := range m.batch.picked {
		out = append(out, c.title)
	}
	return out
}

// TestComposerListsOnlyOpenPrompts: done and frozen prompts are not rows at
// all; an info note is a row, dimmed, that refuses the pick in words.
func TestComposerListsOnlyOpenPrompts(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	m = openComposer(t, m)
	for _, c := range m.batch.cands {
		if c.title == "Frozen one" || c.title == "Done one" {
			t.Errorf("closed prompt %q is listed", c.title)
		}
	}
	i := candIndex(t, m, "A note")
	m.toggleBatchCand(i)
	if len(m.batch.picked) != 0 {
		t.Error("an info note was picked")
	}
	if !strings.Contains(m.batch.note, "note") {
		t.Errorf("refusal = %q, want it to say why", m.batch.note)
	}
}

// TestComposerPickOrderIsCheckOrder: checking appends, unchecking removes, and
// the right pane is the checked rows in the order they were checked.
func TestComposerPickOrderIsCheckOrder(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	m = openComposer(t, m)
	m.toggleBatchCand(candIndex(t, m, "Rename headings"))
	m.toggleBatchCand(candIndex(t, m, "Fix flaky drop test"))
	m.toggleBatchCand(candIndex(t, m, "Global task"))
	if got, want := pickedTitles(m), []string{"Rename headings", "Fix flaky drop test", "Global task"}; !slices.Equal(got, want) {
		t.Fatalf("picked = %v, want %v", got, want)
	}
	m.toggleBatchCand(candIndex(t, m, "Fix flaky drop test"))
	if got, want := pickedTitles(m), []string{"Rename headings", "Global task"}; !slices.Equal(got, want) {
		t.Errorf("after uncheck = %v, want %v", got, want)
	}
	// The left pane draws membership: the listItem for a picked row is marked.
	for _, it := range m.batch.pick.items {
		if it.selectable && it.marked != m.batch.isPicked(m.batch.cands[it.ref].key()) {
			t.Errorf("row %q marked=%v disagrees with the batch", it.name, it.marked)
		}
	}
}

// TestComposerSelectAllRespectsTheFilter: ctrl+a picks what the filter shows
// (and skips what cannot be picked), and a second press takes them back out.
func TestComposerSelectAllRespectsTheFilter(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	m = openComposer(t, m)
	m.batch.pick.input.SetValue("head")
	m.batch.pick.filter()
	m.toggleBatchAll()
	if got := pickedTitles(m); !slices.Equal(got, []string{"Rename headings"}) {
		t.Fatalf("filtered all = %v, want only the match", got)
	}
	m.batch.pick.input.SetValue("")
	m.batch.pick.filter()
	m.toggleBatchAll()
	if got := len(m.batch.picked); got != 4 {
		t.Errorf("all = %d picks, want the 4 open non-info prompts", got)
	}
	m.toggleBatchAll()
	if got := len(m.batch.picked); got != 0 {
		t.Errorf("second all left %d picks, want none", got)
	}
}

// TestComposerReorderAndSort: alt+↓ moves the highlighted pick, s sorts once by
// title, and a move after the sort makes the order manual again.
func TestComposerReorderAndSort(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	m = openComposer(t, m)
	for _, title := range []string{"Rename headings", "Fix flaky drop test", "Add cleanup command"} {
		m.toggleBatchCand(candIndex(t, m, title))
	}
	m.setBatchFocus(batchFocusBatch)
	next, _ := m.updateBatchCompose(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModAlt})
	m = next.(model)
	if got, want := pickedTitles(m), []string{"Fix flaky drop test", "Rename headings", "Add cleanup command"}; !slices.Equal(got, want) {
		t.Fatalf("after alt+↓ = %v, want %v", got, want)
	}
	if m.batch.cursor != 1 {
		t.Errorf("cursor = %d, want it to ride with the moved pick (1)", m.batch.cursor)
	}
	next, _ = m.updateBatchCompose(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = next.(model)
	if got, want := pickedTitles(m), []string{"Add cleanup command", "Fix flaky drop test", "Rename headings"}; !slices.Equal(got, want) {
		t.Fatalf("after sort = %v, want %v", got, want)
	}
	if !m.batch.sorted {
		t.Error("sorted flag not set after a sort")
	}
	m.moveBatchPick(-1)
	if m.batch.sorted {
		t.Error("a move after the sort still reads as sorted")
	}
}

// TestComposerRemoveUnchecksTheRow: removing on the right unchecks on the left.
func TestComposerRemoveUnchecksTheRow(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	m = openComposer(t, m)
	i := candIndex(t, m, "Rename headings")
	m.toggleBatchCand(i)
	m.setBatchFocus(batchFocusBatch)
	next, _ := m.updateBatchCompose(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	if len(m.batch.picked) != 0 {
		t.Fatal("x did not remove the pick")
	}
	for _, it := range m.batch.pick.items {
		if it.selectable && it.ref == i && it.marked {
			t.Error("the removed pick is still checked on the left")
		}
	}
}

// TestComposerDropRefusals: each reason a batch cannot go is said in words, and
// nothing is recorded.
func TestComposerDropRefusals(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	m = openComposer(t, m)
	next, _ := m.dropBatch()
	m = next.(model)
	if !strings.Contains(m.batch.note, "pick at least one") {
		t.Errorf("empty batch note = %q", m.batch.note)
	}
	m.toggleBatchCand(candIndex(t, m, "Rename headings"))
	next, _ = m.dropBatch()
	m = next.(model)
	if !strings.Contains(m.batch.note, "control socket") {
		t.Errorf("no-socket note = %q", m.batch.note)
	}
	// With a client, all at once into one running pane is refused for more
	// than one prompt.
	m.client = &catsClient{}
	m.toggleBatchCand(candIndex(t, m, "Fix flaky drop test"))
	m.batch.target = dropTarget{kind: targetExistingPane, pane: 3, agent: "claude", label: "claude · proj"}
	if why := m.batchDropWhy(); !strings.Contains(why, "its own session") {
		t.Errorf("pane + all at once = %q, want the two ways out", why)
	}
	m.batch.deliver = deliverCombined
	if why := m.batchDropWhy(); why != "" {
		t.Errorf("pane + combined refused: %q", why)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(project.path), batchesFileName)); err == nil {
		t.Error("a refused drop wrote batches.json")
	}
}

// TestComposerEscGuardsPicks: esc with picks warns first and leaves on the
// second press; with none it leaves at once.
func TestComposerEscGuardsPicks(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	m = openComposer(t, m)
	next, _ := m.updateBatchCompose(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.(model).stage != stageList {
		t.Fatal("esc on an empty composer did not leave")
	}
	m.toggleBatchCand(candIndex(t, m, "Rename headings"))
	next, _ = m.updateBatchCompose(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)
	if m.stage != stageBatchCompose || m.batch.note != m.batchLeaveWarn() {
		t.Fatalf("first esc with picks: stage %v note %q, want a warning", m.stage, m.batch.note)
	}
	next, _ = m.updateBatchCompose(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.(model).stage != stageList {
		t.Error("second esc did not leave")
	}
}

// TestComposerStarMarksOverrides: ✱ is on a pick whose own options the batch
// replaces, and — for one combined prompt — on every pick with options.
func TestComposerStarMarksOverrides(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	m = openComposer(t, m)
	c := batchCand{title: "x", session: &SessionOpts{Model: "opus", Effort: effortLow}}
	if m.batchPickStar(c) {
		t.Error("✱ with no batch options")
	}
	m.batch.session.Effort = effortLow
	if m.batchPickStar(c) {
		t.Error("✱ where the batch agrees with the prompt")
	}
	m.batch.session.Model = "sonnet"
	if !m.batchPickStar(c) {
		t.Error("no ✱ where the batch replaces the model")
	}
	m.batch.session = SessionOpts{}
	m.batch.deliver = deliverCombined
	if !m.batchPickStar(c) {
		t.Error("no ✱ on a combined drop, where the prompt's options cannot apply")
	}
}

// TestComposerSessionPanelRoundTrip: ctrl+r edits the batch's own options in
// the ⚙ panel, and esc brings them back to the composer — not to a prompt,
// and not to the form.
func TestComposerSessionPanelRoundTrip(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	m = openComposer(t, m)
	before, _ := os.ReadFile(project.path)
	next, _ := m.updateBatchCompose(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = next.(model)
	if m.stage != stageSession || !m.sessForBatch {
		t.Fatalf("ctrl+r: stage %v sessForBatch %v", m.stage, m.sessForBatch)
	}
	m.formSession.Model = "haiku"
	next, _ = m.updateSession(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)
	if m.stage != stageBatchCompose {
		t.Fatalf("esc from the panel went to %v, want the composer", m.stage)
	}
	if m.batch.session.Model != "haiku" || m.sessForBatch {
		t.Errorf("batch session = %+v, sessForBatch %v", m.batch.session, m.sessForBatch)
	}
	after, _ := os.ReadFile(project.path)
	if string(before) != string(after) {
		t.Error("editing the batch's options wrote the backlog")
	}
}

// TestComposerTargetPickerRecordsOnTheDraft: the picker in batch flavour
// records the row and comes back; esc comes back without changing it.
func TestComposerTargetPickerRecordsOnTheDraft(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	m = openComposer(t, m)
	m.pickForBatch = true
	m.targets = []dropTarget{defaultBatchTarget(), {kind: targetNewSession, command: "claude", worktree: true, label: "wt"}}
	m.targetList = newFuzzyList("", []listItem{{name: "a", selectable: true, ref: 0}, {name: "b", selectable: true, ref: 1}})
	m.targetList.moveDown()
	m.stage = stageTarget
	next, _ := m.chooseTarget(dropRun)
	m = next.(model)
	if m.stage != stageBatchCompose || !m.batch.target.worktree || m.pickForBatch {
		t.Fatalf("after enter: stage %v target %+v pickForBatch %v", m.stage, m.batch.target, m.pickForBatch)
	}
	m.pickForBatch, m.stage = true, stageTarget
	next, _ = m.leaveTarget()
	m = next.(model)
	if m.stage != stageBatchCompose || !m.batch.target.worktree {
		t.Errorf("esc: stage %v target %+v", m.stage, m.batch.target)
	}
}

// TestComposerNextItemsBecomePrompts: a picked Next List item is saved as a
// backlog prompt when the batch drops — or, when the backlog already holds an
// open copy of it, that copy is used rather than a second one made.
func TestComposerNextItemsBecomePrompts(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	it := nextItem{ID: "N-007", Value: "high", Section: "Open", Text: "Tidy the comments"}
	ref, err := m.nextItemAsPrompt(it)
	if err != nil {
		t.Fatal(err)
	}
	td, ok := project.find(ref.id)
	if !ok || !strings.HasPrefix(td.Prompt, nextItemCite("N-007")) {
		t.Fatalf("saved prompt = %+v", td)
	}
	again, err := m.nextItemAsPrompt(it)
	if err != nil || again != ref {
		t.Errorf("second pick made %v (err %v), want the existing copy %v", again, err, ref)
	}
}

// --- Delivery --------------------------------------------------------------------

// launchFrom builds a batch the way dropBatch does, without a cats socket.
func launchFrom(m model, deliver string, titles ...string) Batch {
	b := Batch{ID: "b-" + deliver, Created: time.Now(), Deliver: deliver,
		Target: batchTargetFrom(defaultBatchTarget(), m.ctx.projectDir()), scope: scopeProject}
	for _, s := range []*store{m.project, m.global} {
		for _, td := range s.todos {
			if slices.Contains(titles, td.Title) {
				b.Items = append(b.Items, BatchItem{Scope: batchScopeName(s.scope), ID: td.ID, Title: td.Title})
			}
		}
	}
	return b
}

// TestLaunchBatchChainsOneDropAtATime: all at once is one step per prompt,
// dispatched one at a time under the dropping guard; each success marks its
// prompt done and the record says where it went; a failed step does not stop
// the rest; the last step releases the guard and closes the record.
func TestLaunchBatchChainsOneDropAtATime(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	b := launchFrom(m, deliverEach, "Fix flaky drop test", "Rename headings", "Global task")
	next, cmd := m.launchBatch(b)
	m = next.(model)
	if cmd == nil || !m.dropping || m.batchRun == nil || len(m.batchRun.steps) != 3 {
		t.Fatalf("launch: cmd %v dropping %v run %+v", cmd != nil, m.dropping, m.batchRun)
	}
	// The record is on disk as running before anything lands.
	bs := batchStoreFor(project)
	if err := bs.load(); err != nil || len(bs.batches) != 1 || bs.batches[0].State != batchRunning {
		t.Fatalf("record before the first step: %+v (err %v)", bs.batches, err)
	}

	// Step 0 lands, step 1 fails, step 2 lands.
	outcomes := []error{nil, fmt.Errorf("agent never came up"), nil}
	for i, err := range outcomes {
		next, cmd = m.finishBatchStep(batchStepMsg{batchID: b.ID, step: i, desc: "new Claude Code session", err: err})
		m = next.(model)
		if last := i == len(outcomes)-1; (cmd == nil) != last {
			t.Fatalf("step %d: next cmd %v, want one unless last", i, cmd != nil)
		}
	}
	if m.dropping || m.batchRun != nil {
		t.Error("the guard is still held after the last step")
	}
	for title, wantDone := range map[string]bool{"Fix flaky drop test": true, "Rename headings": false} {
		for _, td := range project.todos {
			if td.Title == title && td.Done != wantDone {
				t.Errorf("%q done = %v, want %v", title, td.Done, wantDone)
			}
		}
	}
	if err := bs.load(); err != nil {
		t.Fatal(err)
	}
	rec := bs.batches[0]
	ok, total := rec.deliveredCounts()
	if rec.State != batchDone || ok != 2 || total != 3 {
		t.Errorf("record: state %q %d/%d, want done 2/3", rec.State, ok, total)
	}
	if run, _ := rec.itemRun(1); run.Err != "agent never came up" {
		t.Errorf("item 1's run = %+v, want its error kept", run)
	}
}

// TestLaunchBatchCombinedIsOneStep: one prompt, listed is a single drop that
// carries every prompt, titled after the batch, wrapped with the batch's own
// options only.
func TestLaunchBatchCombinedIsOneStep(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	b := launchFrom(m, deliverCombined, "Fix flaky drop test", "Rename headings")
	b.Name = "pair"
	b.Session = &SessionOpts{Model: "sonnet"}
	next, _ := m.launchBatch(b)
	m = next.(model)
	if len(m.batchRun.steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(m.batchRun.steps))
	}
	st := m.batchRun.steps[0]
	if len(st.items) != 2 || st.act.todo.Title != "pair" || st.act.todo.Session.Model != "sonnet" {
		t.Errorf("step = items %v title %q session %+v", st.items, st.act.todo.Title, st.act.todo.Session)
	}
	if !strings.Contains(st.act.todo.Prompt, "## 2. Rename headings") {
		t.Errorf("combined body missing the second task:\n%s", st.act.todo.Prompt)
	}
}

// TestLaunchBatchSkipsClosedPrompts: a prompt frozen after it was picked is
// not sent, and the record says why.
func TestLaunchBatchSkipsClosedPrompts(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	b := launchFrom(m, deliverEach, "Fix flaky drop test", "Rename headings")
	if err := project.setFrozen(b.Items[1].ID, true); err != nil {
		t.Fatal(err)
	}
	next, _ := m.launchBatch(b)
	m = next.(model)
	if len(m.batchRun.steps) != 1 {
		t.Fatalf("steps = %d, want the frozen prompt left out", len(m.batchRun.steps))
	}
	run, ok := m.batchRun.batch.itemRun(1)
	if !ok || !strings.Contains(run.Err, "frozen") {
		t.Errorf("frozen item's run = %+v, want the reason recorded", run)
	}
}

// TestLaunchBatchStepFailsWithoutSocket runs the real step command: with no
// cats socket performDrop fails at once, and the chain records it.
func TestLaunchBatchStepFailsWithoutSocket(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	next, cmd := m.launchBatch(launchFrom(m, deliverEach, "Rename headings"))
	m = next.(model)
	msg, ok := cmd().(batchStepMsg)
	if !ok || msg.err == nil {
		t.Fatalf("step result = %+v, want a failure", msg)
	}
	next, _ = m.finishBatchStep(msg)
	m = next.(model)
	if m.dropping || !m.statusErr {
		t.Errorf("after the failed only step: dropping %v statusErr %v (%q)", m.dropping, m.statusErr, m.status)
	}
}

// --- The Batches page ------------------------------------------------------------

// TestBatchesChordOpensPageOrComposer: ctrl+k on the list opens the page, or —
// with prompts selected — a composer holding the open ones.
func TestBatchesChordOpensPageOrComposer(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	next, _ := m.updateList(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	if next.(model).stage != stageBatches {
		t.Fatalf("ctrl+k with no selection went to %v", next.(model).stage)
	}
	m.marked = map[todoRef]bool{{scope: scopeProject, id: "p1"}: true, {scope: scopeProject, id: "pf"}: true}
	next, _ = m.updateList(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	m = next.(model)
	if m.stage != stageBatchCompose {
		t.Fatalf("ctrl+k with a selection went to %v", m.stage)
	}
	if got := pickedTitles(m); !slices.Equal(got, []string{"Rename headings"}) {
		t.Errorf("preset = %v, want only the open selected prompt", got)
	}
	if !strings.Contains(m.batch.note, "left out") {
		t.Errorf("note = %q, want the frozen one's absence said", m.batch.note)
	}
}

// TestBatchesPageDuplicateAndDelete: rows are newest first; ⧉ Duplicate picks
// the prompts still open with the batch's settings; delete takes two presses.
func TestBatchesPageDuplicateAndDelete(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	bs := batchStoreFor(project)
	older := launchFrom(m, deliverEach, "Fix flaky drop test")
	older.ID, older.Dropped, older.State = "old", time.Now().Add(-time.Hour), batchDone
	newer := launchFrom(m, deliverCombined, "Rename headings", "Done one")
	newer.ID, newer.Name, newer.Dropped, newer.State = "new", "pair", time.Now(), batchDone
	newer.Session = &SessionOpts{Effort: effortHigh}
	for _, b := range []Batch{older, newer} {
		if err := bs.put(b); err != nil {
			t.Fatal(err)
		}
	}
	m.openBatchesPage()
	if len(m.batches.rows) != 2 || m.batches.rows[0].ID != "new" {
		t.Fatalf("rows = %+v, want newest first", m.batches.rows)
	}
	next, _ := m.updateBatches(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = next.(model)
	if m.stage != stageBatchCompose {
		t.Fatalf("duplicate went to %v (%q)", m.stage, m.batches.note)
	}
	if got := pickedTitles(m); !slices.Equal(got, []string{"Rename headings"}) {
		t.Errorf("duplicate picked %v, want only the still-open prompt", got)
	}
	if m.batch.deliver != deliverCombined || m.batch.session.Effort != effortHigh || m.batch.name.Value() != "pair" {
		t.Errorf("duplicate settings: deliver %q session %+v name %q", m.batch.deliver, m.batch.session, m.batch.name.Value())
	}

	m.batch = batchComposer{}
	m.openBatchesPage()
	next, _ = m.updateBatches(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = next.(model)
	if len(m.batches.rows) != 2 {
		t.Fatal("one ctrl+x deleted the record")
	}
	next, _ = m.updateBatches(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = next.(model)
	if len(m.batches.rows) != 1 || m.batches.rows[0].ID != "old" {
		t.Errorf("after two ctrl+x: %+v", m.batches.rows)
	}
	raw, _ := os.ReadFile(bs.path)
	var onDisk []Batch
	_ = json.Unmarshal(raw, &onDisk)
	if len(onDisk) != 1 {
		t.Errorf("file holds %d records, want 1", len(onDisk))
	}
}

// --- Layout ---------------------------------------------------------------------

// TestComposerFrameFitsThePane: every line of the composer fits the pane at
// every width it can be drawn at, both layouts, every focus, and the frame is
// exactly the pane's height — a line that wrapped would move every row below
// it off the line the pointer's hit-test expects.
func TestComposerFrameFitsThePane(t *testing.T) {
	for _, w := range []int{160, 120, 100, 99, 80, 60} {
		m, _, _ := batchModel(t, w, 28)
		m = openComposer(t, m)
		m.toggleBatchCand(0)
		m.toggleBatchCand(1)
		m.batch.session = SessionOpts{Model: "sonnet", Effort: effortHigh, Finish: finishWrap}
		for f := range batchFocusCount {
			m.setBatchFocus(f)
			m.sizeBatchCompose()
			frame := ansi.Strip(m.View().Content)
			lines := strings.Split(frame, "\n")
			if len(lines) != 28 {
				t.Errorf("width %d focus %d: %d lines, want 28", w, f, len(lines))
			}
			for i, ln := range lines {
				if cw := ansi.StringWidth(ln); cw > w {
					t.Errorf("width %d focus %d line %d is %d cells: %q", w, f, i, cw, ln)
				}
			}
		}
	}
}

// TestComposerClickTogglesTheRowDrawn pins the Pick pane's hit-test against a
// rendered frame: a click on the line a title is drawn on checks that title.
func TestComposerClickTogglesTheRowDrawn(t *testing.T) {
	for _, w := range []int{120, 80} {
		m, _, _ := batchModel(t, w, 28)
		m = openComposer(t, m)
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		y := slices.IndexFunc(lines, func(s string) bool { return strings.Contains(s, "Add cleanup command") })
		if y < 0 {
			t.Fatalf("width %d: title not drawn", w)
		}
		next, _ := m.Update(tea.MouseClickMsg{X: 6, Y: y, Button: tea.MouseLeft})
		m = next.(model)
		if got := pickedTitles(m); !slices.Equal(got, []string{"Add cleanup command"}) {
			t.Errorf("width %d: click on line %d picked %v", w, y, got)
		}
	}
}

// TestComposerDragReordersTheBatch: press on a Batch row, move to another, let
// go — the pick moves there.
func TestComposerDragReordersTheBatch(t *testing.T) {
	m, _, _ := batchModel(t, 120, 28)
	m = openComposer(t, m)
	for _, title := range []string{"Fix flaky drop test", "Rename headings", "Add cleanup command"} {
		m.toggleBatchCand(candIndex(t, m, title))
	}
	g := m.batchGeom()
	x := g.batchX + 4
	next, _ := m.Update(tea.MouseClickMsg{X: x, Y: g.batchRowY, Button: tea.MouseLeft})
	m = next.(model)
	next, _ = m.Update(tea.MouseMotionMsg{X: x, Y: g.batchRowY + 2, Button: tea.MouseLeft})
	m = next.(model)
	next, _ = m.Update(tea.MouseReleaseMsg{X: x, Y: g.batchRowY + 2, Button: tea.MouseLeft})
	m = next.(model)
	if got, want := pickedTitles(m), []string{"Rename headings", "Add cleanup command", "Fix flaky drop test"}; !slices.Equal(got, want) {
		t.Errorf("after drag = %v, want %v", got, want)
	}
	if m.batch.dragging {
		t.Error("still dragging after the release")
	}
}

// TestBatchStagesSurviveResize: the new stages take a WindowSizeMsg without
// panicking (TestWindowSizeMsgNeverPanics' rule, for the stages it predates).
func TestBatchStagesSurviveResize(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	m = openComposer(t, m)
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	m.openBatchesPage()
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	m.stage = stageBatchView
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	_ = m.View()
}

// TestBatchesPageRowsFitThePane: the page's rows are cut to the pane, so none
// wraps under the pointer's hit-test.
func TestBatchesPageRowsFitThePane(t *testing.T) {
	for _, w := range []int{120, 70, 50} {
		m, project, _ := batchModel(t, w, 20)
		b := launchFrom(m, deliverEach, "Fix flaky drop test", "Rename headings", "Add cleanup command")
		b.ID, b.Name, b.State, b.Dropped = "a", strings.Repeat("a long batch name ", 4), batchDone, time.Now()
		b.Target.Label = "＋ New Claude Code session on a new worktree"
		if err := batchStoreFor(project).put(b); err != nil {
			t.Fatal(err)
		}
		m.openBatchesPage()
		for i, ln := range strings.Split(ansi.Strip(m.View().Content), "\n") {
			if cw := ansi.StringWidth(ln); cw > w {
				t.Errorf("width %d line %d is %d cells: %q", w, i, cw, ln)
			}
		}
	}
}
