package main

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/rohanthewiz/cats/wire"
)

// --- Phase 3: loops ---------------------------------------------------------------
//
// No test here talks to cats. A loop's sends and polls are commands the tests
// never run; instead each test hands Update the message the command would
// have produced (loopSentMsg, loopPollMsg) and reads the record on disk and
// the model afterwards. That is the whole of the loop's logic: what it does
// with each answer.

// loopBatch is a loop over the named prompts, with opts.
func loopBatch(m model, opts LoopOpts, titles ...string) Batch {
	b := launchFrom(m, deliverLoop, titles...)
	b.Loop = &opts
	return b
}

// startedLoop launches b from the composer's road and returns the model with
// the first prompt's send in flight.
func startedLoop(t *testing.T, b Batch) model {
	t.Helper()
	m, _, _ := batchModel(t, 120, 30)
	return startLoopOn(t, m, b)
}

func startLoopOn(t *testing.T, m model, b Batch) model {
	t.Helper()
	m.client = &catsClient{}
	next, cmd := m.launchBatch(b)
	m = next.(model)
	if cmd == nil || !m.dropping || m.loops[b.ID] == nil || !m.loops[b.ID].sending {
		t.Fatalf("launch: cmd %v dropping %v runner %+v", cmd != nil, m.dropping, m.loops[b.ID])
	}
	return m
}

// update feeds Update one message.
func update(m model, msg tea.Msg) (model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(model), cmd
}

// panesWith is a pane.list answer holding one claude pane in state.
func panesWith(pane uint32, state string) []wire.PaneInfo {
	return []wire.PaneInfo{{Pane: pane, PaneMeta: wire.PaneMeta{Agent: "claude", AgentState: state}}}
}

// poll hands the loop a pane.list answer taken at at.
func poll(m model, at time.Time, panes []wire.PaneInfo) model {
	m, _ = update(m, loopPollMsg{panes: panes, at: at})
	return m
}

// runStep plays out one prompt's life in pane 7: its send lands, the agent is
// seen working, then idle.
func runStep(t *testing.T, m model, id string, item int, at time.Time) model {
	t.Helper()
	m, _ = update(m, loopSentMsg{batchID: id, what: loopPhasePrompt, item: item, desc: "new Claude Code session", landing: dropLanding{pane: 7}})
	m = poll(m, at.Add(time.Second), panesWith(7, "working"))
	return poll(m, at.Add(2*time.Second), panesWith(7, "idle"))
}

// TestLoopJudge: the one judgment a loop makes about its pane, case by case.
// Finished means working then idle; the exceptions are the between command's
// instant rule and a resumed loop's first look.
func TestLoopJudge(t *testing.T) {
	t0 := time.Now()
	cases := []struct {
		name    string
		phase   string
		seen    bool
		relaxed bool
		maxWait time.Duration
		obs     paneObs
		after   time.Duration
		want    loopVerdict
		why     string
	}{
		{"idle before it started", loopPhasePrompt, false, false, 0, paneObs{true, "claude", "idle"}, 2 * time.Second, loopWaiting, ""},
		{"working", loopPhasePrompt, false, false, 0, paneObs{true, "claude", "working"}, time.Second, loopWaiting, ""},
		{"idle after working", loopPhasePrompt, true, false, 0, paneObs{true, "claude", "idle"}, time.Minute, loopFinished, ""},
		{"blocked is still going", loopPhasePrompt, true, false, 0, paneObs{true, "claude", "blocked"}, time.Minute, loopWaiting, ""},
		{"pane gone", loopPhasePrompt, true, false, 0, paneObs{}, time.Second, loopFailed, errLoopPaneGone},
		{"never started", loopPhasePrompt, false, false, 0, paneObs{true, "claude", "idle"}, loopStartWait, loopFailed, "never started"},
		{"past the max wait", loopPhasePrompt, true, false, time.Hour, paneObs{true, "claude", "working"}, 2 * time.Hour, loopFailed, "max wait"},
		{"inside the max wait", loopPhasePrompt, true, false, time.Hour, paneObs{true, "claude", "working"}, 30 * time.Minute, loopWaiting, ""},
		{"no agent, briefly", loopPhasePrompt, true, false, 0, paneObs{true, "", ""}, 3 * time.Second, loopWaiting, ""},
		{"no agent for good", loopPhasePrompt, true, false, 0, paneObs{true, "", ""}, loopNoAgentWait, loopFailed, "no agent"},
		{"instant between command", loopPhaseBetween, false, false, 0, paneObs{true, "claude", "idle"}, loopBetweenGrace, loopFinished, ""},
		{"between still in its grace", loopPhaseBetween, false, false, 0, paneObs{true, "claude", "idle"}, time.Second, loopWaiting, ""},
		{"resumed and idle", loopPhasePrompt, false, true, 0, paneObs{true, "claude", "idle"}, time.Second, loopFinished, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lr := &loopRunner{
				batch:   Batch{Progress: &LoopProgress{Phase: c.phase, Since: t0, Pane: 7}},
				seen:    c.seen,
				relaxed: c.relaxed,
				maxWait: c.maxWait,
				opts:    LoopOpts{MaxWait: "1h"},
			}
			got, why := lr.judge(c.obs, t0.Add(c.after))
			if got != c.want || !strings.Contains(why, c.why) {
				t.Errorf("judge = %v %q, want %v containing %q", got, why, c.want, c.why)
			}
		})
	}
}

// TestLoopSameSessionRunsInOrder: a same-session loop sends one prompt, waits
// for it to go working → idle, then sends the next into the pane the first
// opened. Next is on disk before each send; each landed prompt is marked done
// with its pane on the record; the last one ends the loop.
func TestLoopSameSessionRunsInOrder(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	b := loopBatch(m, LoopOpts{}, "Fix flaky drop test", "Rename headings", "Add cleanup command")
	m = startLoopOn(t, m, b)

	// Written before the send: a manager dying now leaves prompt 1 unsent,
	// never sent twice.
	rec, _ := readBatch(t, project, b.ID)
	if rec.State != batchRunning || rec.Progress == nil || rec.Progress.Next != 1 ||
		rec.Progress.Phase != loopPhasePrompt || rec.Progress.Owner != os.Getpid() {
		t.Fatalf("record before the first send: %+v %+v", rec, rec.Progress)
	}

	now := time.Now()
	m, _ = update(m, loopSentMsg{batchID: b.ID, what: loopPhasePrompt, item: 0, desc: "new Claude Code session", landing: dropLanding{pane: 7}})
	if m.dropping {
		t.Error("the guard is still held while the loop only waits")
	}
	if td, _ := m.resolve(b.Items[0].ref()); !td.Done {
		t.Error("the delivered prompt was not marked done")
	}
	rec, _ = readBatch(t, project, b.ID)
	if run, ok := rec.itemRun(0); !ok || run.Pane != 7 || rec.Progress.Pane != 7 {
		t.Errorf("run %+v, progress pane %d — want pane 7 on both", run, rec.Progress.Pane)
	}

	// Idle before it was seen working: not finished yet.
	m = poll(m, now.Add(time.Second), panesWith(7, "idle"))
	if m.dropping {
		t.Fatal("sent the next prompt before the first was seen working")
	}
	m = poll(m, now.Add(2*time.Second), panesWith(7, "working"))
	m = poll(m, now.Add(3*time.Second), panesWith(7, "idle"))
	if !m.dropping || !m.loops[b.ID].sending {
		t.Fatal("the next prompt was not sent after working → idle")
	}
	// The next one goes into the pane the first opened.
	if act := m.loopAction(m.loops[b.ID], Todo{Title: "x"}, b.Items[1].ref()); act.target.kind != targetExistingPane || act.target.pane != 7 {
		t.Errorf("continuation target = %+v, want pane 7", act.target)
	}

	m = runStep(t, m, b.ID, 1, now.Add(10*time.Second))
	m = runStep(t, m, b.ID, 2, now.Add(20*time.Second))
	rec, _ = readBatch(t, project, b.ID)
	if rec.State != batchDone || rec.Progress.Owner != 0 || m.loops[b.ID] != nil || m.dropping {
		t.Errorf("after the last: state %q owner %d runner %v dropping %v", rec.State, rec.Progress.Owner, m.loops[b.ID] != nil, m.dropping)
	}
	if ok, total := rec.deliveredCounts(); ok != 3 || total != 3 {
		t.Errorf("delivered %d/%d", ok, total)
	}
}

// TestLoopFinishIsSentOnceInTheSameSession: in the same session the wrap-up is
// lifted out of every prompt and sent once after the last; with a fresh
// session each it stays on each prompt, since each is its own conversation.
func TestLoopFinishIsSentOnceInTheSameSession(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	b := loopBatch(m, LoopOpts{}, "Fix flaky drop test", "Rename headings")
	b.Session = &SessionOpts{Finish: finishPush, Model: "sonnet"}
	m = startLoopOn(t, m, b)
	lr := m.loops[b.ID]

	td, _ := m.resolve(b.Items[0].ref())
	act := m.loopAction(lr, td, b.Items[0].ref())
	if act.todo.Session == nil || act.todo.Session.Finish != finishNone || act.todo.Session.Model != "sonnet" {
		t.Errorf("same-session prompt options = %+v, want the model kept and the finish lifted", act.todo.Session)
	}
	if got := m.loopFinishText(lr.batch); !strings.HasPrefix(got, loopFinishIntro) || !strings.Contains(got, "commit and push") {
		t.Errorf("finish text = %q", got)
	}

	now := time.Now()
	m = runStep(t, m, b.ID, 0, now)
	m = runStep(t, m, b.ID, 1, now.Add(10*time.Second))
	rec, _ := readBatch(t, project, b.ID)
	if rec.State != batchRunning || rec.Progress.Phase != loopPhaseFinish || !m.dropping {
		t.Fatalf("after the last prompt: state %q phase %q dropping %v — want the finish sent", rec.State, rec.Progress.Phase, m.dropping)
	}
	m, _ = update(m, loopSentMsg{batchID: b.ID, what: loopPhaseFinish, landing: dropLanding{pane: 7}})
	m = poll(m, now.Add(time.Minute), panesWith(7, "working"))
	m = poll(m, now.Add(2*time.Minute), panesWith(7, "idle"))
	if rec, _ := readBatch(t, project, b.ID); rec.State != batchDone {
		t.Errorf("state after the finish = %q", rec.State)
	}

	fresh := &loopRunner{batch: b, opts: LoopOpts{Fresh: true}}
	fresh.batch.Progress = &LoopProgress{Pane: 7}
	act = m.loopAction(fresh, td, b.Items[0].ref())
	if act.target.kind != targetNewSession || act.todo.Session.Finish != finishPush {
		t.Errorf("fresh-each action: target %+v finish %q, want a new session keeping the finish", act.target, act.todo.Session.Finish)
	}
}

// TestLoopBetweenCommandAndPause: after a prompt finishes, the between command
// is sent and waited on (an instant one counts as done once its grace passes),
// then the pause, then the next prompt. With "after the last too" the command
// runs once more at the end.
func TestLoopBetweenCommandAndPause(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	b := loopBatch(m, LoopOpts{Between: "/clear", BetweenLast: true, Pause: "30s"}, "Fix flaky drop test", "Rename headings")
	m = startLoopOn(t, m, b)
	now := time.Now()

	m = runStep(t, m, b.ID, 0, now)
	rec, _ := readBatch(t, project, b.ID)
	if rec.Progress.Phase != loopPhaseBetween || !m.dropping {
		t.Fatalf("after prompt 1: phase %q dropping %v, want the between command going", rec.Progress.Phase, m.dropping)
	}
	sentAt := time.Now()
	m, _ = update(m, loopSentMsg{batchID: b.ID, what: loopPhaseBetween, landing: dropLanding{pane: 7}})
	// /clear never shows working: done once the grace has passed.
	m = poll(m, sentAt.Add(time.Second), panesWith(7, "idle"))
	if rec, _ := readBatch(t, project, b.ID); rec.Progress.Phase != loopPhaseBetween {
		t.Fatalf("phase %q — the between command was taken as done inside its grace", rec.Progress.Phase)
	}
	m = poll(m, sentAt.Add(loopBetweenGrace+time.Second), panesWith(7, "idle"))
	rec, _ = readBatch(t, project, b.ID)
	if rec.Progress.Phase != loopPhasePause || rec.Progress.Until.IsZero() || m.dropping {
		t.Fatalf("after the between command: phase %q until %v dropping %v, want a pause", rec.Progress.Phase, rec.Progress.Until, m.dropping)
	}

	// The tick ends the pause only once Until has passed.
	var cmd tea.Cmd
	m, cmd = update(m, scheduleTickMsg(rec.Progress.Until.Add(-time.Second)))
	_ = cmd
	if m.dropping {
		t.Fatal("the next prompt went before the pause was over")
	}
	m, _ = update(m, scheduleTickMsg(rec.Progress.Until.Add(time.Second)))
	if !m.dropping || m.loops[b.ID].batch.Progress.Next != 2 {
		t.Fatal("the next prompt did not go when the pause ended")
	}

	// The last prompt, then the command once more.
	m = runStep(t, m, b.ID, 1, time.Now())
	if rec, _ := readBatch(t, project, b.ID); rec.Progress.Phase != loopPhaseBetween {
		t.Fatalf("after the last prompt: phase %q, want the between command again", rec.Progress.Phase)
	}
	m, _ = update(m, loopSentMsg{batchID: b.ID, what: loopPhaseBetween, landing: dropLanding{pane: 7}})
	m = poll(m, time.Now().Add(time.Minute), panesWith(7, "idle"))
	if rec, _ := readBatch(t, project, b.ID); rec.State != batchDone {
		t.Errorf("state = %q, want done after the last between command", rec.State)
	}
}

// TestLoopOnFail: stop ends the loop at the failing step with the reason; skip
// records it and goes on. A delivered prompt the loop gave up on stays
// delivered, with the reason as Stalled rather than Err.
func TestLoopOnFail(t *testing.T) {
	t.Run("stop on a stuck prompt", func(t *testing.T) {
		m, project, _ := batchModel(t, 120, 30)
		b := loopBatch(m, LoopOpts{MaxWait: "20m"}, "Fix flaky drop test", "Rename headings")
		m = startLoopOn(t, m, b)
		now := time.Now()
		m, _ = update(m, loopSentMsg{batchID: b.ID, what: loopPhasePrompt, item: 0, desc: "d", landing: dropLanding{pane: 7}})
		m = poll(m, now.Add(time.Minute), panesWith(7, "working"))
		// Past the max wait but inside loopResumeWindow: no tick runs here to
		// refresh the heartbeat, and a jump past the window is a lapsed loop
		// (TestOwnLoopLapsedBySleepIsStopped), not a stuck prompt.
		m = poll(m, now.Add(30*time.Minute), panesWith(7, "working"))
		rec, _ := readBatch(t, project, b.ID)
		run, _ := rec.itemRun(0)
		if rec.State != batchStopped || !strings.Contains(rec.Why, "max wait") || run.Err != "" || !strings.Contains(run.Stalled, "max wait") {
			t.Errorf("state %q why %q run %+v", rec.State, rec.Why, run)
		}
		if ok, _ := rec.deliveredCounts(); ok != 1 {
			t.Errorf("delivered = %d, want the stuck prompt still counted", ok)
		}
		if m.loops[b.ID] != nil || m.dropping {
			t.Error("the runner outlived its stop")
		}
	})

	t.Run("skip a failed send", func(t *testing.T) {
		m, project, _ := batchModel(t, 120, 30)
		b := loopBatch(m, LoopOpts{OnFail: loopOnFailSkip, Between: "/compact"}, "Fix flaky drop test", "Rename headings")
		m = startLoopOn(t, m, b)
		m, _ = update(m, loopSentMsg{batchID: b.ID, what: loopPhasePrompt, item: 0, desc: "d", err: errors.New("the agent never came up")})
		// Nothing ran, so no between command: the next prompt goes at once.
		rec, _ := readBatch(t, project, b.ID)
		if !m.dropping || rec.Progress.Next != 2 || rec.Progress.Phase != loopPhasePrompt {
			t.Fatalf("after a failed send under skip: dropping %v next %d phase %q", m.dropping, rec.Progress.Next, rec.Progress.Phase)
		}
		if run, _ := rec.itemRun(0); !strings.Contains(run.Err, "never came up") {
			t.Errorf("failed run = %+v", run)
		}
		if td, _ := m.resolve(b.Items[0].ref()); td.Done {
			t.Error("a prompt that never landed was marked done")
		}
	})

	t.Run("a closed pane stops a same-session loop even under skip", func(t *testing.T) {
		m, project, _ := batchModel(t, 120, 30)
		b := loopBatch(m, LoopOpts{OnFail: loopOnFailSkip}, "Fix flaky drop test", "Rename headings")
		m = startLoopOn(t, m, b)
		m, _ = update(m, loopSentMsg{batchID: b.ID, what: loopPhasePrompt, item: 0, desc: "d", landing: dropLanding{pane: 7}})
		m = poll(m, time.Now().Add(time.Second), nil)
		if rec, _ := readBatch(t, project, b.ID); rec.State != batchStopped || !strings.Contains(rec.Why, errLoopPaneGone) {
			t.Errorf("state %q why %q", rec.State, rec.Why)
		}
	})

	t.Run("fresh each skips past a closed pane", func(t *testing.T) {
		m, project, _ := batchModel(t, 120, 30)
		b := loopBatch(m, LoopOpts{Fresh: true, OnFail: loopOnFailSkip}, "Fix flaky drop test", "Rename headings")
		m = startLoopOn(t, m, b)
		m, _ = update(m, loopSentMsg{batchID: b.ID, what: loopPhasePrompt, item: 0, desc: "d", landing: dropLanding{pane: 7}})
		m = poll(m, time.Now().Add(time.Second), nil)
		rec, _ := readBatch(t, project, b.ID)
		if rec.State != batchRunning || !m.dropping || rec.Progress.Next != 2 {
			t.Errorf("state %q dropping %v next %d, want the next prompt going", rec.State, m.dropping, rec.Progress.Next)
		}
	})
}

// TestLoopSkipsPromptsClosedMeanwhile: a prompt completed in another pane while
// the loop was busy is skipped at its turn, with the reason — hours pass in a
// loop, and the backlog is re-read before each send.
func TestLoopSkipsPromptsClosedMeanwhile(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	b := loopBatch(m, LoopOpts{}, "Fix flaky drop test", "Rename headings", "Add cleanup command")
	m = startLoopOn(t, m, b)
	other := &store{scope: scopeProject, path: project.path}
	if err := other.setDone(b.Items[1].ID, true); err != nil {
		t.Fatal(err)
	}
	m = runStep(t, m, b.ID, 0, time.Now())
	rec, _ := readBatch(t, project, b.ID)
	if rec.Progress.Next != 3 || !m.dropping {
		t.Fatalf("next %d dropping %v, want prompt 3 going", rec.Progress.Next, m.dropping)
	}
	if run, _ := rec.itemRun(1); !strings.Contains(run.Err, "completed") {
		t.Errorf("skipped run = %+v", run)
	}
}

// TestLoopResumesAfterTheManagerClosed: a running loop whose owner is gone is
// taken over by the next manager with a swap, and picks up from the recorded
// phase — its first look at the pane relaxed, since nobody watched it. A
// prompt the record says was being sent but has no run is marked as unknown
// rather than sent again. A loop with a live owner is left alone.
func TestLoopResumesAfterTheManagerClosed(t *testing.T) {
	old := processAlive
	t.Cleanup(func() { processAlive = old })

	m, project, _ := batchModel(t, 120, 30)
	m.client = &catsClient{}
	b := loopBatch(m, LoopOpts{}, "Fix flaky drop test", "Rename headings", "Add cleanup command")
	b.State = batchRunning
	b.Runs = []BatchRun{{Items: []int{0}, At: time.Now(), Pane: 7}}
	b.Progress = &LoopProgress{Next: 2, Phase: loopPhasePrompt, Pane: 7, Since: time.Now(), Owner: 999999, Beat: time.Now()}
	if err := batchStoreFor(project).put(b); err != nil {
		t.Fatal(err)
	}

	processAlive = func(int) bool { return true }
	m, _ = update(m, scheduleTickMsg(time.Now()))
	if m.loops[b.ID] != nil {
		t.Fatal("took over a loop whose owner is alive and beating")
	}

	processAlive = func(int) bool { return false }
	m, _ = update(m, scheduleTickMsg(time.Now()))
	lr := m.loops[b.ID]
	if lr == nil || !lr.relaxed {
		t.Fatalf("not adopted: %+v", lr)
	}
	rec, _ := readBatch(t, project, b.ID)
	if rec.Progress.Owner != os.Getpid() {
		t.Errorf("owner = %d, want this manager", rec.Progress.Owner)
	}
	if run, ok := rec.itemRun(1); !ok || !strings.Contains(run.Err, "manager closed") {
		t.Errorf("the prompt in flight at the close: %+v", run)
	}
	// Idle at the first look is finished: the loop goes on to prompt 3.
	m = poll(m, time.Now(), panesWith(7, "idle"))
	if !m.dropping || m.loops[b.ID].batch.Progress.Next != 3 {
		t.Error("the resumed loop did not move on")
	}
}

// TestLoopPastTheResumeWindowIsStopped: an orphaned loop whose heartbeat is
// over an hour old is not taken over — opening the manager days later must not
// set its prompts off, any more than a schedule past its grace fires. It is
// stopped with the reason instead, the prompt in flight at the close marked
// unknown as a take-over would, and that happens outside cats too, since
// stopping sends nothing. The Owner is left alive but stale, which is the
// suspended-laptop case as another manager sees it.
func TestLoopPastTheResumeWindowIsStopped(t *testing.T) {
	old := processAlive
	t.Cleanup(func() { processAlive = old })
	processAlive = func(int) bool { return true }

	m, project, _ := batchModel(t, 120, 30)
	m.client = nil
	now := time.Now()
	beat := now.Add(-loopResumeWindow - time.Minute)
	b := loopBatch(m, LoopOpts{}, "Fix flaky drop test", "Rename headings", "Add cleanup command")
	b.State = batchRunning
	b.Runs = []BatchRun{{Items: []int{0}, At: beat, Pane: 7}}
	b.Progress = &LoopProgress{Next: 2, Phase: loopPhasePrompt, Pane: 7, Since: beat, Owner: 999999, Beat: beat}
	if err := batchStoreFor(project).put(b); err != nil {
		t.Fatal(err)
	}

	m, _ = update(m, scheduleTickMsg(now))
	if m.loops[b.ID] != nil {
		t.Fatal("took over a loop left past the resume window")
	}
	rec, _ := readBatch(t, project, b.ID)
	if rec.State != batchStopped || !strings.Contains(rec.Why, "not resumed") {
		t.Fatalf("record = %q (%s), want stopped with the reason", rec.State, rec.Why)
	}
	if rec.Progress.Owner != 0 || rec.Progress.Pane != 0 || rec.Progress.Next != 2 {
		t.Errorf("progress = %+v, want no owner or pane, Next kept", rec.Progress)
	}
	if run, ok := rec.itemRun(1); !ok || !strings.Contains(run.Err, "manager closed") {
		t.Errorf("the prompt in flight at the close: %+v", run)
	}
}

// TestLoopInsideTheResumeWindowIsResumed: the window's other side — an
// orphan whose beat is under the hour is taken over as before.
func TestLoopInsideTheResumeWindowIsResumed(t *testing.T) {
	old := processAlive
	t.Cleanup(func() { processAlive = old })
	processAlive = func(int) bool { return false }

	m, project, _ := batchModel(t, 120, 30)
	m.client = &catsClient{}
	now := time.Now()
	beat := now.Add(-loopResumeWindow + time.Minute)
	b := loopBatch(m, LoopOpts{}, "Fix flaky drop test", "Rename headings")
	b.State = batchRunning
	b.Runs = []BatchRun{{Items: []int{0}, At: beat, Pane: 7}}
	b.Progress = &LoopProgress{Next: 1, Phase: loopPhasePrompt, Pane: 7, Since: beat, Owner: 999999, Beat: beat}
	if err := batchStoreFor(project).put(b); err != nil {
		t.Fatal(err)
	}
	m, _ = update(m, scheduleTickMsg(now))
	if m.loops[b.ID] == nil {
		t.Fatal("a loop inside the window was not resumed")
	}
	if rec, _ := readBatch(t, project, b.ID); rec.State != batchRunning {
		t.Errorf("state = %q, want running", rec.State)
	}
}

// TestOwnLoopLapsedBySleepIsStopped: the manager driving a loop was itself
// asleep (a laptop lid) for past the window. Its first tick back stops the
// loop, exactly as another manager finding the stale beat would, so the
// outcome never depends on which one wakes first — and a pane.list answer
// that was in flight across the sleep is not judged before that tick.
func TestOwnLoopLapsedBySleepIsStopped(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	b := loopBatch(m, LoopOpts{}, "Fix flaky drop test", "Rename headings")
	m = startLoopOn(t, m, b)
	m, _ = update(m, loopSentMsg{batchID: b.ID, what: loopPhasePrompt, item: 0, desc: "d", landing: dropLanding{pane: 7}})
	m = poll(m, time.Now().Add(time.Second), panesWith(7, "working"))

	woke := time.Now().Add(loopResumeWindow + time.Minute)
	m = poll(m, woke, panesWith(7, "idle"))
	if m.dropping {
		t.Fatal("a poll across the sleep sent the next prompt")
	}
	m, _ = update(m, scheduleTickMsg(woke))
	if m.loops[b.ID] != nil || m.dropping {
		t.Fatalf("runner %v dropping %v, want it stopped", m.loops[b.ID] != nil, m.dropping)
	}
	rec, _ := readBatch(t, project, b.ID)
	if rec.State != batchStopped || !strings.Contains(rec.Why, "not resumed") || rec.Progress.Next != 1 {
		t.Errorf("record = %q next %d (%s)", rec.State, rec.Progress.Next, rec.Why)
	}
}

// TestLoopStandsDownWhenStoppedElsewhere: another pane stops the loop through
// its record; this manager's next write loses the swap and it stops driving —
// before typing anything more.
func TestLoopStandsDownWhenStoppedElsewhere(t *testing.T) {
	b0 := Batch{}
	m, project, _ := batchModel(t, 120, 30)
	b0 = loopBatch(m, LoopOpts{}, "Fix flaky drop test", "Rename headings")
	m = startLoopOn(t, m, b0)
	m, _ = update(m, loopSentMsg{batchID: b0.ID, what: loopPhasePrompt, item: 0, desc: "d", landing: dropLanding{pane: 7}})

	// The other pane: a model of its own on the same files.
	rec, _ := readBatch(t, project, b0.ID)
	other := model{project: &store{scope: scopeProject, path: project.path}, global: m.global}
	if line, isErr := other.stopLoop(rec); isErr {
		t.Fatalf("stop from the other pane: %s", line)
	}

	m = poll(m, time.Now().Add(time.Second), panesWith(7, "working"))
	m = poll(m, time.Now().Add(2*time.Second), panesWith(7, "idle"))
	if m.loops[b0.ID] != nil || m.dropping {
		t.Errorf("runner %v dropping %v — want it stood down, nothing sent", m.loops[b0.ID] != nil, m.dropping)
	}
	if rec, _ := readBatch(t, project, b0.ID); rec.State != batchStopped || rec.Progress.Next != 1 {
		t.Errorf("record = %q next %d, want the stop kept", rec.State, rec.Progress.Next)
	}
}

// TestStopLoopFromThePage: ■ Stop on a loop this manager drives ends it now,
// but not while a prompt is mid-typing.
func TestStopLoopFromThePage(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	b := loopBatch(m, LoopOpts{}, "Fix flaky drop test", "Rename headings")
	m = startLoopOn(t, m, b)
	rec, _ := readBatch(t, project, b.ID)
	if _, isErr := m.stopLoop(rec); !isErr || m.loops[b.ID] == nil {
		t.Fatal("stopped a loop in the middle of typing")
	}
	m, _ = update(m, loopSentMsg{batchID: b.ID, what: loopPhasePrompt, item: 0, desc: "d", landing: dropLanding{pane: 7}})
	rec, _ = readBatch(t, project, b.ID)
	if _, isErr := m.stopLoop(rec); isErr {
		t.Fatal("stop refused")
	}
	rec, _ = readBatch(t, project, b.ID)
	if rec.State != batchStopped || m.loops[b.ID] != nil {
		t.Errorf("state %q runner %v", rec.State, m.loops[b.ID] != nil)
	}
	if glyph, _ := batchStateMark(rec); glyph != "■" {
		t.Errorf("badge = %q", glyph)
	}
}

// TestSameRevisionIgnoresTheHeartbeat: a heartbeat changes nothing anyone acts
// on, so a stop read before it still wins; a step does change the revision.
func TestSameRevisionIgnoresTheHeartbeat(t *testing.T) {
	a := Batch{State: batchRunning, Progress: &LoopProgress{Next: 1, Phase: loopPhasePrompt, Owner: 5, Beat: time.Now()}}
	b := a.cloneLoop()
	b.Progress.Beat = time.Now().Add(time.Minute)
	if !sameRevision(a, b) {
		t.Error("a heartbeat changed the revision")
	}
	b.Progress.Next = 2
	if sameRevision(a, b) {
		t.Error("a step did not change the revision")
	}
	if sameRevision(Batch{State: batchRunning}, a) {
		t.Error("a record with progress matched one without")
	}
}

// TestPendingRefsQueuedLoopPrompts: a running loop's unsent prompts are spoken
// for (the zero time, drawn "queued"); the ones already sent are not.
func TestPendingRefsQueuedLoopPrompts(t *testing.T) {
	b := Batch{State: batchRunning, Deliver: deliverLoop, Progress: &LoopProgress{Next: 1},
		Items: []BatchItem{{Scope: "project", ID: "a"}, {Scope: "project", ID: "b"}}}
	later := Batch{State: batchScheduled, At: time.Now().Add(time.Hour), Items: []BatchItem{{Scope: "project", ID: "b"}}}
	refs := pendingRefs([]Batch{b, later})
	if _, ok := refs[todoRef{scope: scopeProject, id: "a"}]; ok {
		t.Error("a sent prompt is still marked")
	}
	if at, ok := refs[todoRef{scope: scopeProject, id: "b"}]; !ok || !at.IsZero() {
		t.Errorf("queued prompt = %v %v, want the zero time over the scheduled one", at, ok)
	}
}

// TestFiredLoopClaimsAndDrives: a scheduled loop's claim writes its driver into
// the record, and the loop starts sending.
func TestFiredLoopClaimsAndDrives(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	m.client = &catsClient{}
	b := loopBatch(m, LoopOpts{}, "Fix flaky drop test")
	b.State, b.At = batchScheduled, time.Now().Add(-time.Second)
	if err := batchStoreFor(project).put(b); err != nil {
		t.Fatal(err)
	}
	if cmd := m.fireDueBatches(time.Now()); cmd == nil {
		t.Fatal("did not fire")
	}
	rec, _ := readBatch(t, project, b.ID)
	if rec.State != batchRunning || rec.Progress == nil || rec.Progress.Owner != os.Getpid() || m.loops[b.ID] == nil || !m.dropping {
		t.Errorf("record %q progress %+v runner %v dropping %v", rec.State, rec.Progress, m.loops[b.ID] != nil, m.dropping)
	}
	if m.batchRun != nil {
		t.Error("a loop went down the chain's road")
	}
}

// TestComposerLoopRows: a new composer opens on loop with its rows under
// Deliver; moving off loop hides them and moving back shows them; the loop's
// refusals are in words; the rows reach the record.
func TestComposerLoopRows(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	m.client = &catsClient{}
	m = openComposer(t, m)
	m.toggleBatchCand(candIndex(t, m, "Fix flaky drop test"))
	m.toggleBatchCand(candIndex(t, m, "Rename headings"))

	// Loop is the default Deliver, so its five rows are there from the start.
	if m.batch.deliver != deliverLoop || len(m.batch.setRows()) != 10 {
		t.Fatalf("a new composer: deliver %q rows %d, want loop and 10", m.batch.deliver, len(m.batch.setRows()))
	}
	m.batch.setRow = batchSetDeliver
	m.setBatchFocus(batchFocusSettings)
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	if m.batch.deliver != deliverEach || len(m.batch.setRows()) != 5 {
		t.Fatalf("off loop: deliver %q rows %d", m.batch.deliver, len(m.batch.setRows()))
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyRight})
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if m.batch.deliver != deliverLoop || len(m.batch.setRows()) != 10 {
		t.Fatalf("deliver %q rows %d", m.batch.deliver, len(m.batch.setRows()))
	}
	if !strings.Contains(m.View().Content, "Between") {
		t.Error("the loop rows are not drawn")
	}

	// Down from Deliver lands on the Loop row; space flips it.
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.batch.setRow != batchSetLoop {
		t.Fatalf("row = %d, want Loop", m.batch.setRow)
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if !m.batch.fresh {
		t.Error("space did not choose fresh each")
	}
	m.batch.fresh = false

	// The between row: typed text, and ctrl+t for the checkbox.
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	for _, r := range "/compact" {
		m, _ = update(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m, _ = update(m, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	if m.batch.between.Value() != "/compact" || !m.batch.betweenLast {
		t.Errorf("between %q last %v", m.batch.between.Value(), m.batch.betweenLast)
	}

	m.batch.pause.SetValue("soon")
	if why := m.batchDropWhy(); !strings.Contains(why, "pause") {
		t.Errorf("a bad pause: %q", why)
	}
	m.batch.pause.SetValue("30s")
	m.batch.fresh = true
	m.batch.target = dropTarget{kind: targetExistingPane, pane: 3, agent: "claude", label: "claude · w1"}
	if why := m.batchDropWhy(); !strings.Contains(why, "fresh session each") {
		t.Errorf("fresh each into a pane: %q", why)
	}
	m.batch.fresh = false
	if why := m.batchDropWhy(); why != "" {
		t.Errorf("a same-session loop into a pane refused: %q", why)
	}
	b, err := m.buildBatch()
	if err != nil || b.Loop == nil || b.Loop.Between != "/compact" || !b.Loop.BetweenLast || b.Loop.Pause != "30s" {
		t.Errorf("record loop = %+v err %v", b.Loop, err)
	}

	// Off loop again, the focus falls back off a row that is no longer drawn.
	m.batch.setRow = batchSetPause
	m.batch.deliver = deliverEach
	m.setBatchFocus(batchFocusSettings)
	if m.batch.setRow != batchSetDeliver {
		t.Errorf("row = %d after the loop rows went", m.batch.setRow)
	}
}

// TestNormalizeLoopOpts: the one validator, shared by the composer and the
// runner.
func TestNormalizeLoopOpts(t *testing.T) {
	if _, err := normalizeLoopOpts(LoopOpts{Between: "a\nb"}); err == nil {
		t.Error("a two-line between command passed")
	}
	if _, err := normalizeLoopOpts(LoopOpts{MaxWait: "-5m"}); err == nil {
		t.Error("a negative max wait passed")
	}
	o, err := normalizeLoopOpts(LoopOpts{Between: "  ", BetweenLast: true, Pause: " 1h 30m "})
	if err != nil || o.BetweenLast || o.Pause != "1h 30m" {
		t.Errorf("normalized = %+v err %v", o, err)
	}
	if d, _ := parseLoopDuration("pause", o.Pause); d != 90*time.Minute {
		t.Errorf("1h 30m = %v", d)
	}
}
