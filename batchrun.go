// batchrun.go — delivering a batch.
//
// A batch is dropped as a chain of ordinary drops, one at a time:
//
//	launchBatch ──► step 0 ──batchStepMsg──► step 1 ──batchStepMsg──► … ──► done
//	   │               │                        │
//	   │ record        │ performDrop            │ record the run, mark its
//	   │ "running"     │ (off the UI thread)    │ prompts done, save, next
//
// Each step is exactly the pendingAction chooseTarget would have built for a
// single drop, so a batch cannot deliver anything a single drop would not: the
// same prompt composition, the same agent-ready wait, the same worktree cut.
// "All at once" is one step per prompt; "one prompt, listed" is one step for
// the lot.
//
// The chain rather than N concurrent drops is deliberate. m.dropping is the
// manager's one-drop-at-a-time guard, and it exists because two drops typing
// into panes at the same moment is how prompts get garbled — a new-session drop
// spends seconds waiting for its agent, and a second one launched meanwhile
// would race it for focus and keystrokes. So the batch holds the guard for its
// whole run, which also keeps a schedule from firing into the middle of it
// (fireDueSchedules waits while a drop is in flight). "All at once" therefore
// means "without waiting for any of them to finish their work", not
// "simultaneously": the tabs open one after another, each as soon as the last
// has its prompt.
//
// The record is written before the first step and after every one, so a
// manager that dies part-way leaves a batch that says "running, 2 of 5
// delivered" rather than one that looks unsent — and the two prompts that did
// land are already marked done, the way a single drop marks its prompt.
package main

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
)

// batchStep is one drop in a batch's chain.
type batchStep struct {
	items []int     // indexes into Batch.Items that this drop carries
	refs  []todoRef // the prompts to mark done when it lands
	act   pendingAction
}

// batchRunner is a batch mid-delivery: its record, and the steps still to go.
type batchRunner struct {
	batch Batch
	steps []batchStep
	next  int // the step in flight
	// fired is set when the batch came off a schedule rather than the
	// composer; see batchStepCmd.
	fired bool
}

// batchStepMsg reports one step's outcome back to Update, the batch's
// counterpart of dropResultMsg. It is a type of its own rather than a flag on
// that one because what happens next is different in kind: a single drop's
// result ends the drop, and a step's result starts the next step.
type batchStepMsg struct {
	batchID string
	step    int
	desc    string
	note    string
	err     error
}

// batchStoreForScope is the batches.json a batch in scope s is kept in.
func (m model) batchStoreForScope(s scope) *batchStore {
	return batchStoreFor(m.storeFor(s))
}

// launchBatch is the composer's ▶ Drop now: start b, and leave the composer
// once its record is safely on disk. A record that cannot be written keeps the
// composer open with the reason, so nothing has been sent and nothing is lost.
func (m model) launchBatch(b Batch) (tea.Model, tea.Cmd) {
	if b.Deliver == deliverLoop {
		// A loop is not a chain of drops but a runner that waits on each
		// prompt finishing (batchloop.go).
		return m.launchLoop(b)
	}
	cmd, err := m.startBatch(b, false)
	if err != nil {
		m.batchSay(err.Error(), true)
		return m, nil
	}
	m.leaveBatchCompose()
	if m.batchRun == nil {
		m.batchStatus("batch "+b.displayName()+": nothing sent — every prompt was closed or gone (see ctrl+k)", true)
		return m, nil
	}
	m.batchStatus(m.batchProgress("dropping"), false)
	return m, cmd
}

// startBatch records b as running and returns its first step. It is shared by
// the composer (launchBatch) and a schedule firing (fireDueBatches), which is
// why it touches no screen: the composer leaves itself, and a fire happens
// behind whatever the user is looking at. fired says the batch came off a
// schedule, so each step re-checks a pane target still exists before typing
// into it (see batchStepCmd).
//
// When every item turns out to be closed or gone, the record is still written
// (done, each item with its reason) and the returned command is nil with
// m.batchRun left nil — the caller's cue to say that nothing went.
//
// Every item is re-read from its backlog here, not taken from the composer:
// the composer may have been open for minutes, and a prompt frozen, completed
// or deleted meanwhile in another pane must not be sent. Such an item is
// recorded as a failed run with the reason, so the batch's history says what
// happened to it instead of silently holding one prompt fewer than was picked.
func (m *model) startBatch(b Batch, fired bool) (tea.Cmd, error) {
	now := time.Now()
	b.State, b.Dropped, b.Why = batchRunning, now, ""

	type live struct {
		idx int
		ref todoRef
		td  Todo
	}
	var ready []live
	for i, it := range b.Items {
		ref := it.ref()
		td, ok := m.resolve(ref)
		why := ""
		switch {
		case !ok:
			why = "the prompt is no longer in the backlog"
		case td.Done:
			why = "the prompt was completed before the batch went"
		case td.Frozen:
			why = "the prompt is frozen"
		case td.Info:
			why = "the prompt is marked info — a note, not work"
		}
		if why != "" {
			b.Runs = append(b.Runs, BatchRun{Items: []int{i}, At: now, Err: why})
			continue
		}
		ready = append(ready, live{idx: i, ref: ref, td: td})
	}

	target := b.Target.dropTarget()
	cwd := firstNonEmpty(b.Target.Cwd, m.ctx.projectDir())
	var steps []batchStep
	switch {
	case len(ready) == 0:
		// Nothing left to send; the record still goes to disk below, with
		// every item's reason, so the page shows what became of the batch.
	case b.Deliver == deliverCombined:
		// One drop carrying every prompt. The body is built from the prompts
		// as they stand now, and wrapped once with the batch's own options: a
		// combined drop is one session, so it can only have one setup, and the
		// prompts' own options have no session of their own to apply to (the
		// composer says so, with ✱, before the drop).
		var todos []Todo
		var images []string
		st := batchStep{}
		for _, r := range ready {
			todos = append(todos, r.td)
			images = append(images, m.storeFor(r.ref.scope).imagePaths(r.td)...)
			st.items = append(st.items, r.idx)
			st.refs = append(st.refs, r.ref)
		}
		st.act = pendingAction{
			// The batch's name is the title: it is what a worktree drop
			// slugs into the branch name, and what the agent's tab is called.
			todo:       Todo{Title: b.displayName(), Prompt: combinedPrompt(todos), Session: sessionPtr(sessionValue(b.Session))},
			target:     target,
			mode:       dropRun,
			cwd:        cwd,
			images:     images,
			anchorPane: m.ctx.OwnPaneID,
		}
		steps = append(steps, st)
	default:
		// One drop per prompt, each with the batch's options laid over its own.
		for _, r := range ready {
			td := r.td
			td.Session = overlaySession(r.td.Session, b.Session)
			steps = append(steps, batchStep{
				items: []int{r.idx},
				refs:  []todoRef{r.ref},
				act: pendingAction{
					todo:       td,
					target:     target,
					mode:       dropRun,
					cwd:        cwd,
					images:     m.storeFor(r.ref.scope).imagePaths(td),
					anchorPane: m.ctx.OwnPaneID,
				},
			})
		}
	}

	if len(steps) == 0 {
		b.State = batchDone
	}
	if err := m.batchStoreForScope(b.scope).put(b); err != nil {
		// Without a record the batch would run with no history to show for it,
		// and the page is where the user will look for what happened. Refusing
		// before anything is sent is the cheap moment to fail.
		return nil, fmt.Errorf("could not save the batch: %w", err)
	}
	if len(steps) == 0 {
		return nil, nil
	}
	m.dropping = true
	m.batchRun = &batchRunner{batch: b, steps: steps, fired: fired}
	return m.batchStepCmd(0), nil
}

// sessionValue dereferences an optional record to the value the clone-based
// helpers take; nil reads as the defaults.
func sessionValue(o *SessionOpts) SessionOpts {
	if o == nil {
		return SessionOpts{}
	}
	return o.clone()
}

// batchStepCmd runs step i off the UI thread, like performDropCmd.
func (m model) batchStepCmd(i int) tea.Cmd {
	br := m.batchRun
	if br == nil || i >= len(br.steps) {
		return nil
	}
	client, act, id := m.client, br.steps[i].act, br.batch.ID
	desc := targetDesc(act.target)
	// A fired batch goes through the schedule's drop, which first checks that
	// a running-pane target still exists. The pane was chosen when the batch
	// was scheduled, maybe hours ago; a pane ID is not a promise, and typing a
	// prompt into whatever now holds that number would be worse than failing.
	sched := Schedule{Kind: br.batch.Target.Kind, Pane: br.batch.Target.Pane}
	fired := br.fired
	return func() tea.Msg {
		var note string
		var err error
		if fired {
			note, err = performScheduledDrop(client, sched, act)
		} else {
			note, err = performDrop(client, act)
		}
		return batchStepMsg{batchID: id, step: i, desc: desc, note: note, err: err}
	}
}

// finishBatchStep records a step's outcome and fires the next one, or closes
// the batch out after the last.
//
// A failed step does not stop the chain. In "all at once" the prompts are
// independent by construction — each has its own session — so one failure (a
// worktree branch that could not be cut, an agent that never came up) is no
// reason to withhold the rest; the record keeps the error beside the item, and
// the page's ⧉ Duplicate re-sends just the ones that did not land.
func (m model) finishBatchStep(msg batchStepMsg) (tea.Model, tea.Cmd) {
	br := m.batchRun
	if br == nil || br.batch.ID != msg.batchID || msg.step != br.next {
		// A result for a chain this manager is no longer running. Nothing can
		// produce one today — the guard admits one batch at a time — but a
		// stale message acting on the current chain would mark the wrong
		// prompts done.
		return m, nil
	}
	st := br.steps[msg.step]
	run := BatchRun{Items: st.items, At: time.Now(), Where: msg.desc}
	if msg.err != nil {
		run.Err = msg.err.Error()
	} else {
		// Delivered means done, as it does for a single drop (see the
		// dropResultMsg case in route). Best effort for the same reason.
		for _, ref := range st.refs {
			_ = m.storeFor(ref.scope).setDone(ref.id, true)
		}
		m.rebuildList()
	}
	br.batch.Runs = append(br.batch.Runs, run)

	br.next++
	last := br.next >= len(br.steps)
	if last {
		br.batch.State = batchDone
	}
	// A failed save is reported but does not stop the chain: the prompts that
	// landed are already in their agents, and the next one is waiting to go.
	saveErr := m.batchStoreForScope(br.batch.scope).put(br.batch)

	if !last {
		m.batchStatus(m.batchProgress("dropping"), false)
		if saveErr != nil {
			m.batchStatus("batch record not saved: "+saveErr.Error(), true)
		}
		return m, m.batchStepCmd(br.next)
	}

	m.dropping = false
	m.batchRun = nil
	ok, total := br.batch.deliveredCounts()
	line := fmt.Sprintf("batch %s: %d/%d dropped → %s", br.batch.displayName(), ok, total, msg.desc)
	switch {
	case saveErr != nil:
		m.batchStatus(line+" · record not saved: "+saveErr.Error(), true)
	case ok < total:
		m.batchStatus(line+" · see ctrl+k for what failed", true)
	default:
		m.batchStatus(line+" · marked done", false)
	}
	return m, nil
}

// batchStatus reports on a batch wherever the user is: the list's status line
// always (it is where they will look later), and the heading of the page in
// front of them when that is the Next List or the Batches page, whose rows are
// also re-read so the batch's own row shows its progress.
func (m *model) batchStatus(line string, isErr bool) {
	m.setStatus(line, isErr)
	switch m.stage {
	case stageNextList:
		m.next.say(line, isErr)
	case stageBatches:
		m.reloadBatches()
		m.batches.say(line, isErr)
	}
}

// batchProgress is the running batch's status line: which step is going, of
// how many, and where. Written in the running form ("dropping 2/5 …") because
// it replaces the line a step later.
func (m model) batchProgress(verb string) string {
	br := m.batchRun
	if br == nil {
		return ""
	}
	st := br.steps[br.next]
	return fmt.Sprintf("batch %s: %s %d/%d → %s…", br.batch.displayName(), verb,
		br.next+1, len(br.steps), targetDesc(st.act.target))
}

// --- Firing scheduled batches -----------------------------------------------------

// batchFiles is the tick's read of both batches.json files, through the
// model's watch (see batchWatch). A file that cannot be read is skipped: the
// tick runs every second, and the page, which reads the files directly, is
// where a broken one says so.
func (m *model) batchFiles() []Batch {
	if m.batchWatch == nil {
		m.batchWatch = &batchWatch{}
	}
	var all []Batch
	for _, s := range []*store{m.project, m.global} {
		if s == nil {
			continue
		}
		bs, err := m.batchWatch.read(batchStoreFor(s))
		if err != nil {
			continue
		}
		all = append(all, bs...)
	}
	return all
}

// fireDueBatches is the tick's work for batches, fireDueSchedules' twin: find a
// scheduled batch whose time has come and start it, or — when it can no longer
// be honoured — mark it missed where the Batches page will show it.
//
// The rules are the prompt schedule's, for the same reasons:
//
//   - Grace. Inside scheduleGrace a late tick still fires; beyond it the batch
//     is missed. Opening the manager should never set off a batch of agent
//     runs planned for hours ago.
//   - One drop at a time. While m.dropping is held (a drop, a schedule, or
//     another batch in flight) the batch waits for a later tick, still inside
//     its grace.
//   - Claim before fire. The record is swapped from scheduled to running on
//     disk before anything is sent (swapBatch), so a second manager pane on
//     the same file finds it running and stands down.
//
// One more is the batch's own: a batch open in this pane's composer is not
// fired or marked missed from under the edit. Saving the edit is what decides
// its time (or cancelling it, after which the next tick reads it as it
// stands). Another manager pane can still fire it; the edit's save then finds
// the record changed and says so rather than writing over it.
func (m *model) fireDueBatches(now time.Time) tea.Cmd {
	for _, b := range m.batchFiles() {
		if b.State != batchScheduled || now.Before(b.At) {
			continue
		}
		if m.batch.edit.ID == b.ID {
			continue
		}
		bs := m.batchStoreForScope(b.scope)

		if late := now.Sub(b.At) > scheduleGrace; late || m.client == nil {
			missed := b
			missed.State = batchMissed
			missed.Why = "the manager was not open at " + formatScheduleTime(b.At, now)
			if !late {
				missed.Why = "no cats control socket to drop through"
			}
			if won, err := bs.swapBatch(b, missed); err == nil && won {
				m.rebuildList()
				m.batchStatus("missed batch "+b.displayName()+" ("+formatScheduleTime(b.At, now)+") — ctrl+k to reschedule or drop it", true)
			}
			continue
		}

		if m.dropping {
			return nil
		}

		claimed := b
		claimed.State = batchRunning
		if b.Deliver == deliverLoop {
			// A loop's claim is also its ownership: the record goes to
			// running with this manager as its driver (see LoopProgress).
			claimed = loopRecord(b, now)
		}
		won, err := bs.swapBatch(b, claimed)
		if err != nil {
			m.batchStatus("batch claim failed: "+err.Error(), true)
			return nil
		}
		if !won {
			// Someone else fired, edited or deleted it between the read and
			// the claim. The next tick reads whatever they left.
			continue
		}
		if b.Deliver == deliverLoop {
			// The loop resolves each prompt as its turn comes (re-reading the
			// backlogs then), so there is nothing to resolve here.
			m.batchStatus("scheduled loop "+b.displayName()+": starting — "+fmt.Sprintf("%d prompt%s, one at a time", len(b.Items), plural(len(b.Items))), false)
			return m.runLoop(claimed, false)
		}
		// The backlogs are re-read before the items are resolved: this pane's
		// copy is only refreshed by its own writes, and hours may have passed
		// in which other panes completed, froze or deleted what the batch
		// holds. startBatch skips those with the reason on the record.
		_ = m.project.reload()
		_ = m.global.reload()
		cmd, err := m.startBatch(claimed, true)
		m.rebuildList()
		switch {
		case err != nil:
			// The claim put "running" on disk and nothing was sent. Left
			// there, the record would claim a delivery forever; written back
			// as missed, it says what happened and can be rescheduled. Best
			// effort — the write that just failed may fail again.
			missed := b
			missed.State, missed.Why = batchMissed, err.Error()
			_, _ = bs.swapBatch(claimed, missed)
			m.batchStatus("scheduled batch "+b.displayName()+": "+err.Error(), true)
		case m.batchRun == nil:
			m.batchStatus("scheduled batch "+b.displayName()+": nothing sent — every prompt was closed or gone (see ctrl+k)", true)
		default:
			m.batchStatus(m.batchProgress("firing scheduled"), false)
		}
		return cmd
	}
	return nil
}
