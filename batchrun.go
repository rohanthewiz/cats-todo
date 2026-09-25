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

// launchBatch records b as running and fires its first step.
//
// Every item is re-read from its backlog here, not taken from the composer:
// the composer may have been open for minutes, and a prompt frozen, completed
// or deleted meanwhile in another pane must not be sent. Such an item is
// recorded as a failed run with the reason, so the batch's history says what
// happened to it instead of silently holding one prompt fewer than was picked.
func (m model) launchBatch(b Batch) (tea.Model, tea.Cmd) {
	now := time.Now()
	b.State, b.Dropped = batchRunning, now

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
		m.batchSay("could not save the batch: "+err.Error(), true)
		return m, nil
	}
	m.leaveBatchCompose()
	if len(steps) == 0 {
		m.batchStatus("batch "+b.displayName()+": nothing sent — every prompt was closed or gone (see ctrl+k)", true)
		return m, nil
	}
	m.dropping = true
	m.batchRun = &batchRunner{batch: b, steps: steps}
	m.batchStatus(m.batchProgress("dropping"), false)
	return m, m.batchStepCmd(0)
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
	return func() tea.Msg {
		note, err := performDrop(client, act)
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
