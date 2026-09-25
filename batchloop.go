// batchloop.go — delivering a batch as a loop: one prompt at a time, each sent
// when the one before it has finished.
//
// The other two modes are fire-and-forget: every prompt is typed in and the
// batch is done (batchrun.go). A loop is for work that has to happen in
// sequence — each step builds on the last, or the steps must not run at the
// same time — so it has to know when a prompt has *finished*, which a drop
// alone never learns. It learns it from cats: pane.list reports each agent
// pane's AgentState (idle / working / blocked), the same field the drop picker
// shows as [working]. One step of a loop is:
//
//	send prompt k ──► watch its pane: working … idle ──► between command? ──► pause? ──► k+1
//	                                   (must see working first)   (watched the same way)
//
// and after the last prompt, optionally the between command once more and, in a
// same-session loop, the finish message (commit / push / wrap) once, rather
// than once per step.
//
// # Who drives it
//
// The loop runs in the manager, like schedules, driven by the one-second tick:
// each tick polls pane.list off the UI thread (loopPollMsg), and each
// observation moves the loop's phase on or leaves it waiting (judge). Typing —
// a prompt, the between command, the finish message — takes the manager's
// m.dropping guard exactly like a single drop, but only for as long as the
// typing takes. The waits, which can be hours, hold nothing, so single drops,
// schedules and other batches go on meanwhile; they only ever take turns at the
// keyboard.
//
// # Surviving a closed manager
//
// The record carries the loop's progress (LoopProgress): the next prompt, the
// phase, the pane being watched, and the manager driving it (Owner, a pid, plus
// a heartbeat). Next is written *before* each send, so a manager that dies
// mid-loop can leave a prompt unsent but never lets one be sent twice. Another
// manager — or the same project's manager reopened — finds a running loop whose
// owner is gone, takes it over with a compare-and-swap (the claim rule every
// batch change follows, see swapBatch), and resumes from the recorded phase —
// but only within loopResumeWindow of the last heartbeat. Past it the loop is
// stopped with the reason instead, as a schedule past its grace is missed.
// Every write the driver makes is such a swap, which is also how a loop is
// stopped from another pane: the stop changes the record, the driver's next
// write loses, and it stands down before typing anything more.
package main

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/rohanthewiz/cats/wire"
)

// LoopOpts are a loop's options, set on the composer's loop rows. Every field's
// zero value is the default, so a loop with nothing chosen writes "loop": {}.
type LoopOpts struct {
	// Fresh opens a new session per prompt (a new tab, or a new worktree when
	// the target is one), each only once the previous one is idle. The default,
	// the same session, sends every prompt into the pane the first one opened
	// (or the chosen running pane), since a sequence usually builds on what
	// came before and one conversation keeps that context.
	Fresh bool `json:"fresh,omitempty"`
	// Between is one line submitted to the pane after a prompt finishes and
	// before the next is sent — a slash command (/compact, /sess-save step) or
	// plain words — and waited on like a prompt. With Fresh it goes to the
	// session that just finished, which is where a /sess-save or a
	// /code-review has something to act on.
	Between string `json:"between,omitempty"`
	// BetweenLast runs Between after the final prompt too. Off by default:
	// /compact after the last step is wasted work, while /sess-save after it
	// is often the point, so it is asked rather than guessed.
	BetweenLast bool `json:"betweenLast,omitempty"`
	// Pause is a wait before each next prompt ("30s"); "" is none.
	Pause string `json:"pause,omitempty"`
	// OnFail is what a failed step does to the rest: "" stops the loop (a
	// sequence usually means later steps depend on earlier ones), "skip"
	// records the failure and carries on.
	OnFail string `json:"onFail,omitempty"`
	// MaxWait is the longest a prompt (or the between command) may run before
	// it counts as stuck, which is a failure ("2h"); "" is no limit.
	MaxWait string `json:"maxWait,omitempty"`
}

// loopOnFailSkip is OnFail's one non-default value.
const loopOnFailSkip = "skip"

// LoopProgress is where a running loop stands, written to the record at every
// step so a manager opened later can pick it up (see the file comment).
type LoopProgress struct {
	// Next is the index of the next item to send. It is advanced *before* the
	// send, so a crash between the two leaves the item unsent rather than sent
	// twice; a resumed loop says so on the item's row.
	Next int `json:"next"`
	// Phase is what the loop is doing: nothing sent yet (""), watching a prompt,
	// the between command or the finish message, or pausing.
	Phase string `json:"phase,omitempty"`
	// Pane is the pane being watched — in a same-session loop, the session
	// every prompt goes into.
	Pane uint32 `json:"pane,omitempty"`
	// Since is when the phase began, the clock the max wait and the start
	// waits run on. Until is when a pause ends.
	Since time.Time `json:"since,omitzero"`
	Until time.Time `json:"until,omitzero"`
	// Owner is the pid of the manager driving the loop, and Beat the last time
	// it said it still was (see loopBeat). Owner is 0 once the loop ends.
	Owner int       `json:"owner,omitempty"`
	Beat  time.Time `json:"beat,omitzero"`
}

// The phases, as written to LoopProgress.Phase.
const (
	loopPhaseStart   = ""        // nothing sent yet
	loopPhasePrompt  = "prompt"  // item Next-1 was sent; waiting for it to finish
	loopPhaseBetween = "between" // the between command was sent; waiting on it
	loopPhasePause   = "pause"   // waiting until Until
	loopPhaseFinish  = "finish"  // the finish message was sent; waiting on it
)

// The loop's clocks.
const (
	// loopBetweenGrace is how long a between command gets to show *working*.
	// Some commands make the agent work (/compact, plain words) and some return
	// at once (/clear, /model); one rule serves both: a pane still idle this
	// long after the command went counts it as an instant one. Well past
	// clearSettle, so the next keystrokes are never typed into a pane still
	// handling it.
	loopBetweenGrace = 3 * time.Second
	// loopStartWait is how long a sent prompt may sit with its agent idle,
	// never seen working, before the loop gives up on it. A prompt that
	// finished inside a single poll would be the innocent reading; one that
	// was never submitted is the likely one, and sending the next prompt on
	// top of it would glue the two together.
	loopStartWait = 45 * time.Second
	// loopNoAgentWait is how long a watched pane may go without a detected
	// agent before the loop decides the agent has exited. Not zero: detection
	// reads the screen, and a redraw can hide the agent for a poll.
	loopNoAgentWait = 10 * time.Second
	// loopBeat is how often a driving manager rewrites its heartbeat, and
	// loopLease how stale one may get before another manager may take the
	// loop over even though the owner's pid still answers (a suspended or
	// hung manager, or a pid reused after a reboot).
	loopBeat  = time.Minute
	loopLease = 5 * time.Minute
	// loopResumeWindow is how long a loop may go undriven — its heartbeat
	// that old — and still be picked up again. Past it the loop is stopped
	// with the reason instead (loopLapsed). It is the loop's version of
	// scheduleGrace: opening the manager, or waking the laptop, should not set
	// off a sequence of agent runs left off hours or days ago, into a pane and
	// a working tree that have moved on since. Wider than the grace because a
	// loop was already running — the user did set it off, and a manager closed
	// by accident and reopened within the hour is the case worth carrying on
	// from — and well past loopLease, so a take-over is always tried first.
	loopResumeWindow = time.Hour
)

// parseLoopDuration reads a pause or max wait as typed: "" is none, anything
// else is a Go duration ("30s", "5m", "1h30m"), spaces ignored.
func parseLoopDuration(what, s string) (time.Duration, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s: can't read “%s” — try 30s, 5m or 1h30m", what, s)
	}
	return d, nil
}

// normalizeLoopOpts checks a loop's options and trims them to what is stored.
// It is the one validator, shared by the composer (which refuses in words
// before anything is saved) and the runner (which meets a hand-edited file).
func normalizeLoopOpts(o LoopOpts) (LoopOpts, error) {
	o.Between = strings.TrimSpace(o.Between)
	// A second line would be submitted as a second message, one the loop
	// would not know to wait for.
	if strings.ContainsAny(o.Between, "\r\n") {
		return o, errors.New("between command: one line only — a second line would be sent as a message the loop does not wait for")
	}
	if o.Between == "" {
		o.BetweenLast = false
	}
	o.Pause = strings.TrimSpace(o.Pause)
	o.MaxWait = strings.TrimSpace(o.MaxWait)
	if _, err := parseLoopDuration("pause", o.Pause); err != nil {
		return o, err
	}
	if _, err := parseLoopDuration("max wait", o.MaxWait); err != nil {
		return o, err
	}
	switch o.OnFail {
	case "", loopOnFailSkip:
	default:
		return o, fmt.Errorf("on fail: “%s” is not stop or skip", o.OnFail)
	}
	return o, nil
}

// summary is the loop's options in one line, for the page's record view.
func (o *LoopOpts) summary() string {
	if o == nil {
		return "same session"
	}
	segs := []string{"same session"}
	if o.Fresh {
		segs[0] = "fresh session each"
	}
	if o.Between != "" {
		b := "between " + o.Between
		if o.BetweenLast {
			b += " (after the last too)"
		}
		segs = append(segs, b)
	}
	if o.Pause != "" {
		segs = append(segs, "pause "+o.Pause)
	}
	if o.MaxWait != "" {
		segs = append(segs, "max wait "+o.MaxWait)
	}
	if o.OnFail == loopOnFailSkip {
		segs = append(segs, "on fail skip")
	} else {
		segs = append(segs, "on fail stop")
	}
	return strings.Join(segs, " · ")
}

// deliverDesc is the delivery mode as a batch's row describes it: a loop says
// which kind, since same session and fresh each are different jobs.
func (b Batch) deliverDesc() string {
	if b.Deliver != deliverLoop {
		return deliverLabel(b.Deliver)
	}
	if b.loopOpts().Fresh {
		return "loop, fresh each"
	}
	return "loop, same session"
}

// loopOpts is the batch's loop options, or the defaults.
func (b Batch) loopOpts() LoopOpts {
	if b.Loop == nil {
		return LoopOpts{}
	}
	return *b.Loop
}

// cloneLoop copies the parts of a record a loop step changes, so that editing
// the copy never reaches the original — the original is the swap's
// expectation, and may be a snapshot the tick's read cache is still holding.
func (b Batch) cloneLoop() Batch {
	b.Runs = slices.Clone(b.Runs)
	if b.Progress != nil {
		p := *b.Progress
		b.Progress = &p
	}
	return b
}

// loopWord is where a running loop stands, in words, for its page row:
// "on 2/5", "after 2/5", "pausing after 2/5", "finishing" — or "paused at 2/5"
// when nobody is driving it (the record view says what that means).
func (b Batch) loopWord(alive bool) string {
	p := b.Progress
	if p == nil {
		return "starting"
	}
	n := len(b.Items)
	if !alive {
		return fmt.Sprintf("paused at %d/%d", p.Next, n)
	}
	switch p.Phase {
	case loopPhasePrompt:
		return fmt.Sprintf("on %d/%d", p.Next, n)
	case loopPhaseBetween:
		return fmt.Sprintf("between, after %d/%d", p.Next, n)
	case loopPhasePause:
		return fmt.Sprintf("pausing after %d/%d", p.Next, n)
	case loopPhaseFinish:
		return "finishing"
	}
	return fmt.Sprintf("starting, 0/%d", n)
}

// processAlive reports whether a process with pid exists. Signal 0 checks
// without delivering anything; EPERM means it exists but is someone else's. A
// var so tests can say which owners are alive.
var processAlive = func(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// loopDriven says a running loop has a live driver: this manager, or another
// whose pid answers and whose heartbeat is fresh.
func (m model) loopDriven(b Batch, now time.Time) bool {
	if _, ok := m.loops[b.ID]; ok {
		return true
	}
	p := b.Progress
	if p == nil || p.Owner == 0 || p.Owner == os.Getpid() {
		return false
	}
	return processAlive(p.Owner) && now.Sub(p.Beat) < loopLease
}

// loopLapsed says a loop last driven at p.Beat has been left too long to pick
// up again (see loopResumeWindow), and why, in the words its record keeps. A
// record with no heartbeat at all — only a hand edit leaves one — counts as
// lapsed: there is nothing to say it was ever recently driven.
func loopLapsed(p *LoopProgress, now time.Time) (string, bool) {
	if p == nil || (!p.Beat.IsZero() && now.Sub(p.Beat) < loopResumeWindow) {
		return "", false
	}
	const tail = " — a loop is picked up again only within an hour; ⧉ Duplicate sends the prompts not sent"
	if p.Beat.IsZero() {
		return "not resumed: no manager's heartbeat on the record" + tail, true
	}
	return "not resumed: nothing drove it after " + formatScheduleTime(p.Beat, now) + tail, true
}

// --- The runner -------------------------------------------------------------------

// loopRunner is a loop this manager is driving. batch is the record as last
// written, and every write swaps against it; the rest is the in-memory half of
// the state, rebuilt on a resume.
type loopRunner struct {
	batch   Batch
	opts    LoopOpts
	pause   time.Duration
	maxWait time.Duration

	// pending is what to type when the keyboard is free (a phase constant),
	// "" when nothing is waiting to go. sending is a send in flight, holding
	// m.dropping.
	pending string
	sending bool
	// seen says the watched pane has been seen working (or blocked on a
	// question, which is working that needs a hand) since the phase began.
	// relaxed is a resumed loop's first watch: nobody saw what the pane did
	// while no manager was open, so an idle pane counts as finished.
	seen    bool
	relaxed bool
	// agentAt is the last poll that found an agent in the watched pane.
	agentAt time.Time
	// lastDone is the phase that finished last, so the between command is not
	// sent twice in a row when the prompts after it all turn out closed.
	lastDone string
}

// watching says the runner is waiting on its pane — the phases a poll answers.
func (lr *loopRunner) watching() bool {
	if lr.pending != "" || lr.sending || lr.batch.Progress == nil {
		return false
	}
	switch lr.batch.Progress.Phase {
	case loopPhasePrompt, loopPhaseBetween, loopPhaseFinish:
		return true
	}
	return false
}

// paneObs is one poll's view of the watched pane.
type paneObs struct {
	found bool
	agent string
	state string
}

// observePane finds pane in a pane.list answer.
func observePane(panes []wire.PaneInfo, pane uint32) paneObs {
	for _, p := range panes {
		if p.Pane == pane {
			return paneObs{found: true, agent: p.Agent, state: p.AgentState}
		}
	}
	return paneObs{}
}

// The verdicts judge gives.
type loopVerdict int

const (
	loopWaiting loopVerdict = iota // still going
	loopFinished
	loopFailed
)

// errLoopPaneGone is the failure a vanished pane gives. It is told apart from
// the others because a same-session loop cannot skip past it: the session every
// later prompt was meant for is gone.
const errLoopPaneGone = "its pane was closed"

// judge reads one observation of the watched pane: is the phase still going,
// finished, or failed (and why)? It is the loop's one judgment, kept free of
// the model and the socket so every case can be tested with a made-up pane.
//
// "Finished" means working → idle. It has to see working first: an agent still
// idle just after a prompt landed has not started on it yet, and taking that as
// done would send the next prompt on top of it. Two exceptions, both about a
// pane that never shows working: a between command that stays idle past
// loopBetweenGrace was an instant one (/clear), and a resumed loop takes idle
// as done since nobody watched the pane in between (relaxed).
func (lr *loopRunner) judge(o paneObs, now time.Time) (loopVerdict, string) {
	p := lr.batch.Progress
	since := p.Since
	if !o.found {
		return loopFailed, errLoopPaneGone
	}
	if o.agent == "" {
		if lr.agentAt.IsZero() {
			lr.agentAt = since
		}
		if now.Sub(lr.agentAt) >= loopNoAgentWait {
			return loopFailed, "no agent is running in its pane any more"
		}
		return loopWaiting, ""
	}
	lr.agentAt = now
	switch o.state {
	case "working", "blocked":
		lr.seen = true
	case "idle":
		switch {
		case lr.seen || lr.relaxed:
			return loopFinished, ""
		case p.Phase == loopPhaseBetween && now.Sub(since) >= loopBetweenGrace:
			return loopFinished, ""
		case p.Phase != loopPhaseBetween && now.Sub(since) >= loopStartWait:
			return loopFailed, fmt.Sprintf("the agent never started on it (still idle %s after it was sent)", loopStartWait)
		}
	}
	if lr.maxWait > 0 && now.Sub(since) > lr.maxWait {
		return loopFailed, "it ran past the max wait (" + lr.opts.MaxWait + ")"
	}
	return loopWaiting, ""
}

// --- Messages -----------------------------------------------------------------------

// loopSentMsg reports one send of a loop's — a prompt, the between command, or
// the finish message — back to Update.
type loopSentMsg struct {
	batchID string
	what    string // the phase the send starts
	item    int    // the prompt's index, for a prompt
	desc    string
	landing dropLanding
	err     error
}

// loopPollMsg is one pane.list answer, for every loop watching a pane.
type loopPollMsg struct {
	panes []wire.PaneInfo
	err   error
	at    time.Time
}

// --- Starting -----------------------------------------------------------------------

// loopRecord is b made a running loop driven by this manager: the state the
// record is written in before anything is sent. A loop aimed at a running pane
// is watching that pane from the start.
func loopRecord(b Batch, now time.Time) Batch {
	b = b.cloneLoop()
	b.State, b.Dropped, b.Why = batchRunning, now, ""
	b.Progress = &LoopProgress{Owner: os.Getpid(), Beat: now}
	if b.Target.Kind == scheduleKindPane {
		b.Progress.Pane = b.Target.Pane
	}
	return b
}

// launchLoop is the composer's ▶ Drop now for a loop: write the record, leave
// the composer, and start typing the first prompt as soon as the keyboard is
// free.
func (m model) launchLoop(b Batch) (tea.Model, tea.Cmd) {
	if b.Progress == nil {
		b = loopRecord(b, time.Now())
	}
	if err := m.batchStoreForScope(b.scope).put(b); err != nil {
		m.batchSay("could not save the batch: "+err.Error(), true)
		return m, nil
	}
	m.leaveBatchCompose()
	m.rebuildList()
	cmd := m.runLoop(b, false)
	if _, ok := m.loops[b.ID]; ok {
		m.batchStatus("loop "+b.displayName()+": starting — "+fmt.Sprintf("%d prompt%s, one at a time", len(b.Items), plural(len(b.Items))), false)
	}
	return m, cmd
}

// runLoop starts driving b, a record already on disk as a running loop owned
// by this manager. resumed says it was taken over rather than started, and
// picks the phase up where the record left it.
func (m *model) runLoop(b Batch, resumed bool) tea.Cmd {
	b = b.cloneLoop()
	if b.Progress == nil {
		b.Progress = &LoopProgress{Owner: os.Getpid(), Beat: time.Now()}
	}
	lr := &loopRunner{batch: b}
	opts, err := normalizeLoopOpts(b.loopOpts())
	if err == nil {
		lr.opts = opts
		lr.pause, _ = parseLoopDuration("pause", opts.Pause)
		lr.maxWait, _ = parseLoopDuration("max wait", opts.MaxWait)
	}
	if m.loops == nil {
		m.loops = map[string]*loopRunner{}
	}
	m.loops[b.ID] = lr
	if err != nil {
		// Only a hand-edited file gets here: the composer refuses these.
		m.loopClose(lr, batchStopped, "its loop options: "+err.Error())
		return nil
	}
	now := time.Now()
	switch b.Progress.Phase {
	case loopPhaseStart:
		lr.pending = loopPhasePrompt
	case loopPhasePause:
		// The tick sends the next prompt once Until has passed.
	default:
		if resumed {
			lr.relaxed = true
		}
		if b.Progress.Pane == 0 {
			// Nothing to watch: the send that would have said which pane
			// never reported back. Carry on as though it finished.
			return m.loopPhaseDone(lr, now)
		}
	}
	return m.loopDispatch(lr, now)
}

// --- Writing the record -------------------------------------------------------------

// loopWrite swaps next over the runner's record. A lost swap means the record
// changed under the runner — stopped, deleted, or taken over from another pane
// — and the runner stands down on the spot, before it types anything more.
//
// A write that fails outright stands the runner down too. Going on would leave
// the file behind what this manager has done, and the file is the only thing
// that keeps a prompt from being sent twice. Standing down leaves the record as
// the last good write had it, owned by this pid and driven by nobody, which the
// next tick's adoptLoops picks up again from exactly there — a retry that can
// only ever resume from what the file says.
func (m *model) loopWrite(lr *loopRunner, next Batch) bool {
	won, err := m.batchStoreForScope(lr.batch.scope).swapBatch(lr.batch, next)
	if err != nil {
		delete(m.loops, lr.batch.ID)
		m.batchStatus("loop "+lr.batch.displayName()+": record not saved ("+err.Error()+") — retrying from the file", true)
		return false
	}
	if !won {
		delete(m.loops, lr.batch.ID)
		m.rebuildList()
		m.batchStatus("loop "+lr.batch.displayName()+" changed in another pane (stopped, deleted or taken over) — this manager has let it go", true)
		return false
	}
	next.scope = lr.batch.scope
	lr.batch = next
	return true
}

// loopClose ends the loop as done or stopped, with why for the record.
func (m *model) loopClose(lr *loopRunner, state, why string) {
	next := lr.batch.cloneLoop()
	next.State, next.Why = state, why
	if next.Progress != nil {
		next.Progress.Owner = 0
		next.Progress.Pane = 0
	}
	if !m.loopWrite(lr, next) {
		return
	}
	delete(m.loops, lr.batch.ID)
	m.rebuildList()
	ok, total := next.deliveredCounts()
	line := fmt.Sprintf("loop %s: %d/%d sent", next.displayName(), ok, total)
	if state == batchStopped {
		m.batchStatus(line+" — stopped: "+why, true)
		return
	}
	if why != "" {
		m.batchStatus(line+", finished — "+why, true)
		return
	}
	if ok < total {
		m.batchStatus(line+", finished — see ctrl+k for the ones that did not go", true)
		return
	}
	m.batchStatus(line+", finished · marked done", false)
}

// --- Sending ------------------------------------------------------------------------

// loopDispatch types what the runner has pending, if the keyboard is free. It
// is called whenever something may have freed it (a phase ending, a send
// coming back, every tick), so a loop never waits on more than one tick for a
// busy guard.
func (m *model) loopDispatch(lr *loopRunner, now time.Time) tea.Cmd {
	if lr.pending == "" || lr.sending || m.dropping || m.client == nil {
		return nil
	}
	switch lr.pending {
	case loopPhasePrompt:
		return m.loopSendPrompt(lr, now)
	case loopPhaseBetween:
		return m.loopSendLine(lr, now, loopPhaseBetween, lr.opts.Between)
	case loopPhaseFinish:
		return m.loopSendLine(lr, now, loopPhaseFinish, m.loopFinishText(lr.batch))
	}
	return nil
}

// loopSendPrompt sends the next prompt that can still go. Items closed or gone
// since the loop was set up are skipped with the reason on the record (the
// startBatch rule): hours can pass in a loop, and a prompt completed by hand
// meanwhile is work already done, not a failure. When none is left, the loop
// goes to its ending.
func (m *model) loopSendPrompt(lr *loopRunner, now time.Time) tea.Cmd {
	// This pane's copy of the backlogs is refreshed only by its own writes,
	// and the prompt before this one may have run for an hour while other
	// panes completed, froze or deleted what comes next.
	_ = m.project.reload()
	_ = m.global.reload()
	m.rebuildList()
	next := lr.batch.cloneLoop()
	p := next.Progress
	k := -1
	var td Todo
	for i := p.Next; i < len(next.Items); i++ {
		ref := next.Items[i].ref()
		t, ok := m.resolve(ref)
		why := ""
		switch {
		case !ok:
			why = "the prompt is no longer in the backlog"
		case t.Done:
			why = "the prompt was completed before its turn"
		case t.Frozen:
			why = "the prompt is frozen"
		case t.Info:
			why = "the prompt is marked info — a note, not work"
		}
		if why != "" {
			next.Runs = append(next.Runs, BatchRun{Items: []int{i}, At: now, Err: why})
			p.Next = i + 1
			continue
		}
		k, td = i, t
		break
	}
	if k < 0 {
		if !m.loopWrite(lr, next) {
			return nil
		}
		lr.pending = ""
		return m.loopEnd(lr, now)
	}
	ref := next.Items[k].ref()
	act := m.loopAction(lr, td, ref)
	p.Next, p.Phase, p.Since, p.Beat = k+1, loopPhasePrompt, now, now
	// Written before the send: see LoopProgress.Next.
	if !m.loopWrite(lr, next) {
		return nil
	}
	lr.pending, lr.sending, lr.seen = "", true, false
	m.dropping = true
	m.batchStatus(fmt.Sprintf("loop %s: sending %d/%d → %s…", next.displayName(), k+1, len(next.Items), targetDesc(act.target)), false)
	client, id, desc := m.client, next.ID, targetDesc(act.target)
	return func() tea.Msg {
		l, err := performDropAt(client, act)
		return loopSentMsg{batchID: id, what: loopPhasePrompt, item: k, desc: desc, landing: l, err: err}
	}
}

// loopAction is the drop one of the loop's prompts goes out as. The batch's
// options are laid over the prompt's (overlaySession, as for all at once), with
// two differences that make it a loop:
//
//   - In the same session, every prompt after the first goes into the pane the
//     first one opened, as a drop into a running pane — so the prompt's own
//     /clear, /model and /effort are applied to it there (applyPaneSetup), just
//     as a single drop into that pane would apply them.
//   - In the same session, the finish (commit / push / wrap, and the release)
//     is lifted out of every prompt and sent once, after the last
//     (loopFinishText): each step committing and pushing on its own would be
//     one commit per step of a single conversation. A fresh session each is
//     its own conversation, so there it stays on each prompt.
//
// A loop always runs its prompts (dropRun): it waits on the agent working, and
// a pasted-but-unsent prompt would never start.
func (m *model) loopAction(lr *loopRunner, td Todo, ref todoRef) pendingAction {
	b := lr.batch
	opts := overlaySession(td.Session, b.Session)
	if !lr.opts.Fresh && opts != nil {
		o := opts.clone()
		o.Finish, o.Release = finishNone, false
		opts = sessionPtr(o)
	}
	td.Session = opts
	target := b.Target.dropTarget()
	if !lr.opts.Fresh && target.kind == targetNewSession && b.Progress.Pane != 0 {
		target = dropTarget{
			kind:  targetExistingPane,
			pane:  b.Progress.Pane,
			agent: firstNonEmpty(b.Target.Command, "claude"),
			label: "the loop's session",
		}
	}
	return pendingAction{
		todo:       td,
		target:     target,
		mode:       dropRun,
		cwd:        firstNonEmpty(b.Target.Cwd, m.ctx.projectDir()),
		images:     m.storeFor(ref.scope).imagePaths(td),
		anchorPane: m.ctx.OwnPaneID,
	}
}

// loopFinishIntro opens the finish message, so the agent reads the wrap-up as
// about the whole run rather than as a new task.
const loopFinishIntro = "That was the last task in this batch."

// loopFinishText is a same-session loop's finish message: the wrap-up lifted
// out of each prompt (see loopAction), sent once at the end. The batch's
// Finish wins, else the last prompt's own; a release is asked for when either
// asks. "" when there is nothing to finish with.
func (m model) loopFinishText(b Batch) string {
	var o SessionOpts
	if b.Session != nil {
		o.Finish, o.Release = b.Session.Finish, b.Session.Release
	}
	if n := len(b.Items); n > 0 {
		if td, ok := m.resolve(b.Items[n-1].ref()); ok && td.Session != nil {
			if o.Finish == finishNone {
				o.Finish = td.Session.Finish
			}
			o.Release = o.Release || td.Session.Release
		}
	}
	post := o.postamble()
	if post == "" {
		return ""
	}
	return loopFinishIntro + "\n\n" + post
}

// loopSendLine submits one line (or the finish message) to the watched pane and
// starts watching it as phase what.
func (m *model) loopSendLine(lr *loopRunner, now time.Time, what, text string) tea.Cmd {
	lr.pending = ""
	pane := lr.batch.Progress.Pane
	if text == "" || pane == 0 {
		// Nothing to send, or nowhere to send it (every prompt so far failed
		// to open a session): the phase is over before it began.
		next := lr.batch.cloneLoop()
		next.Progress.Phase = what
		if !m.loopWrite(lr, next) {
			return nil
		}
		return m.loopPhaseDone(lr, now)
	}
	next := lr.batch.cloneLoop()
	next.Progress.Phase, next.Progress.Since, next.Progress.Beat = what, now, now
	if !m.loopWrite(lr, next) {
		return nil
	}
	lr.sending, lr.seen = true, false
	m.dropping = true
	client, id := m.client, next.ID
	return func() tea.Msg {
		err := client.sendInput(pane, text, true)
		return loopSentMsg{batchID: id, what: what, err: err, landing: dropLanding{pane: pane}}
	}
}

// loopSent is a send coming back: record it, and start watching what it sent.
func (m model) loopSent(msg loopSentMsg) (tea.Model, tea.Cmd) {
	m.dropping = false
	now := time.Now()
	lr := m.loops[msg.batchID]
	if lr == nil {
		// The runner stood down while the send was in flight (a heartbeat
		// lost to a stop in another pane). The prompt did land, so it is
		// marked done as any delivered prompt is; the record is no longer
		// this manager's to write.
		if msg.what == loopPhasePrompt && msg.err == nil {
			m.loopMarkDone(msg)
		}
		return m, m.loopDispatchAll(now)
	}
	lr.sending = false
	n := len(lr.batch.Items)
	switch msg.what {
	case loopPhasePrompt:
		next := lr.batch.cloneLoop()
		run := BatchRun{Items: []int{msg.item}, At: now, Where: msg.desc, Pane: msg.landing.pane, Branch: msg.landing.branch}
		if msg.err != nil {
			run.Err = msg.err.Error()
		} else {
			m.loopMarkDone(msg)
			if msg.landing.pane != 0 {
				next.Progress.Pane = msg.landing.pane
			}
		}
		next.Runs = append(next.Runs, run)
		next.Progress.Since = now
		if !m.loopWrite(lr, next) {
			return m, nil
		}
		lr.agentAt, lr.seen = now, false
		if msg.err == nil {
			line := fmt.Sprintf("loop %s: %d/%d sent → %s — waiting for it to finish", next.displayName(), msg.item+1, n, msg.desc)
			if msg.landing.note != "" {
				line += " · " + msg.landing.note
			}
			m.batchStatus(line, false)
			return m, m.loopDispatchAll(now)
		}
		why := fmt.Sprintf("prompt %d/%d could not be sent: %s", msg.item+1, n, msg.err)
		if lr.opts.OnFail != loopOnFailSkip {
			m.loopClose(lr, batchStopped, why)
			return m, m.loopDispatchAll(now)
		}
		// Skip: nothing ran, so there is no between command to send and no
		// pause to wait out — the next prompt goes straight away.
		m.batchStatus("loop "+next.displayName()+": "+why+" — skipping to the next", true)
		lr.pending = loopPhasePrompt
		return m, m.loopDispatchAll(now)
	default:
		if msg.err != nil {
			cmd := m.loopFail(lr, "sending "+msg.what+": "+msg.err.Error(), now)
			return m, tea.Batch(cmd, m.loopDispatchAll(now))
		}
		lr.batch.Progress.Since = now
		lr.agentAt, lr.seen = now, false
		if msg.what == loopPhaseBetween {
			m.batchStatus("loop "+lr.batch.displayName()+": sent "+lr.opts.Between+" — waiting on it", false)
		} else {
			m.batchStatus("loop "+lr.batch.displayName()+": sent the finish — waiting on it", false)
		}
		return m, m.loopDispatchAll(now)
	}
}

// loopMarkDone marks a delivered prompt done, as a single drop's is.
func (m *model) loopMarkDone(msg loopSentMsg) {
	lr := m.loops[msg.batchID]
	if lr == nil {
		// Found through the files instead: the record still names the item.
		for _, b := range m.batchFiles() {
			if b.ID == msg.batchID && msg.item < len(b.Items) {
				ref := b.Items[msg.item].ref()
				_ = m.storeFor(ref.scope).setDone(ref.id, true)
			}
		}
		m.rebuildList()
		return
	}
	ref := lr.batch.Items[msg.item].ref()
	_ = m.storeFor(ref.scope).setDone(ref.id, true)
	m.rebuildList()
}

// loopDispatchAll gives every runner with something pending a chance at the
// keyboard. Only one can take it; the rest wait for the next chance.
func (m *model) loopDispatchAll(now time.Time) tea.Cmd {
	var cmds []tea.Cmd
	for _, id := range m.loopIDs() {
		if lr := m.loops[id]; lr != nil {
			cmds = append(cmds, m.loopDispatch(lr, now))
		}
	}
	return tea.Batch(cmds...)
}

// loopIDs is the runners in a stable order — a map's own order would let two
// loops waiting on the keyboard take turns at random.
func (m model) loopIDs() []string {
	ids := make([]string, 0, len(m.loops))
	for id := range m.loops {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// --- Moving on ----------------------------------------------------------------------

// loopPhaseDone is the watched phase finishing: decide what comes next.
func (m *model) loopPhaseDone(lr *loopRunner, now time.Time) tea.Cmd {
	p := lr.batch.Progress
	lr.lastDone, lr.seen, lr.relaxed = p.Phase, false, false
	n := len(lr.batch.Items)
	switch p.Phase {
	case loopPhasePrompt:
		if p.Next < n {
			if lr.opts.Between != "" {
				lr.pending = loopPhaseBetween
				m.batchStatus(fmt.Sprintf("loop %s: %d/%d finished — sending %s", lr.batch.displayName(), p.Next, n, lr.opts.Between), false)
				return m.loopDispatch(lr, now)
			}
			return m.loopPauseOrNext(lr, now)
		}
		return m.loopEnd(lr, now)
	case loopPhaseBetween:
		if p.Next < n {
			return m.loopPauseOrNext(lr, now)
		}
		return m.loopFinishOrDone(lr, now)
	case loopPhasePause:
		lr.pending = loopPhasePrompt
		return m.loopDispatch(lr, now)
	case loopPhaseFinish:
		m.loopClose(lr, batchDone, "")
		return nil
	}
	lr.pending = loopPhasePrompt
	return m.loopDispatch(lr, now)
}

// loopPauseOrNext waits out the pause, if there is one, then sends the next
// prompt.
func (m *model) loopPauseOrNext(lr *loopRunner, now time.Time) tea.Cmd {
	if lr.pause <= 0 {
		lr.pending = loopPhasePrompt
		return m.loopDispatch(lr, now)
	}
	next := lr.batch.cloneLoop()
	next.Progress.Phase, next.Progress.Until = loopPhasePause, now.Add(lr.pause)
	if !m.loopWrite(lr, next) {
		return nil
	}
	m.batchStatus(fmt.Sprintf("loop %s: %d/%d finished — pausing %s", next.displayName(), next.Progress.Next, len(next.Items), lr.opts.Pause), false)
	return nil
}

// loopEnd is the loop past its last prompt: the between command once more if
// asked for — and only if a prompt ran, and it did not just run — then the
// finish.
func (m *model) loopEnd(lr *loopRunner, now time.Time) tea.Cmd {
	if lr.opts.BetweenLast && lr.opts.Between != "" && lr.lastDone != loopPhaseBetween && loopRanAny(lr.batch) {
		lr.pending = loopPhaseBetween
		return m.loopDispatch(lr, now)
	}
	return m.loopFinishOrDone(lr, now)
}

// loopFinishOrDone sends a same-session loop's finish message, or ends the
// loop.
func (m *model) loopFinishOrDone(lr *loopRunner, now time.Time) tea.Cmd {
	if !lr.opts.Fresh && loopRanAny(lr.batch) && m.loopFinishText(lr.batch) != "" {
		lr.pending = loopPhaseFinish
		return m.loopDispatch(lr, now)
	}
	m.loopClose(lr, batchDone, "")
	return nil
}

// loopRanAny says at least one of the loop's prompts reached its agent.
func loopRanAny(b Batch) bool {
	return slices.ContainsFunc(b.Runs, func(r BatchRun) bool { return r.Err == "" })
}

// loopFail is a watched phase failing: the prompt stuck past the max wait, its
// pane gone, the agent never starting. The failure is written on the prompt's
// run (Stalled — it was delivered, and stays delivered), then On failure
// decides: stop, or skip to the next prompt.
//
// Two failures stop whatever On failure says. In the same session, a vanished
// pane leaves nowhere for the next prompt to go. And the finish message is the
// last thing a loop does, so there is nothing to skip to.
func (m *model) loopFail(lr *loopRunner, reason string, now time.Time) tea.Cmd {
	next := lr.batch.cloneLoop()
	p := next.Progress
	k := p.Next - 1
	what := reason
	switch p.Phase {
	case loopPhaseBetween:
		what = "its between command: " + reason
	case loopPhaseFinish:
		what = "the finish message: " + reason
	}
	for j := len(next.Runs) - 1; j >= 0 && k >= 0; j-- {
		if slices.Contains(next.Runs[j].Items, k) {
			if next.Runs[j].Err == "" {
				next.Runs[j].Stalled = what
			}
			break
		}
	}
	lr.lastDone = p.Phase
	stop := lr.opts.OnFail != loopOnFailSkip ||
		(reason == errLoopPaneGone && !lr.opts.Fresh) ||
		p.Phase == loopPhaseFinish
	why := fmt.Sprintf("prompt %d/%d — %s", k+1, len(next.Items), what)
	if p.Phase == loopPhaseFinish {
		why = what
	}
	if stop {
		lr.batch = mergeRuns(lr.batch, next)
		m.loopClose(lr, batchStopped, why)
		return nil
	}
	if !m.loopWrite(lr, next) {
		return nil
	}
	m.batchStatus("loop "+next.displayName()+": "+why+" — skipping on", true)
	// Skipping past a stuck prompt skips its between command too: the pane is
	// still busy with the prompt, and the command would only queue behind it.
	if p.Next < len(next.Items) {
		return m.loopPauseOrNext(lr, now)
	}
	return m.loopEnd(lr, now)
}

// mergeRuns carries next's runs onto cur without touching the revision the
// swap compares — for a close that should record a stalled run in the same
// write as the stop.
func mergeRuns(cur, next Batch) Batch {
	cur = cur.cloneLoop()
	cur.Runs = next.Runs
	return cur
}

// --- The tick -----------------------------------------------------------------------

// loopTick is the tick's work for loops, after the schedules and batches have
// had their turn at the keyboard: take over loops nobody is driving, end the
// pauses that are over, keep the heartbeats fresh, give pending sends their
// chance, and poll pane.list for everything watching a pane.
func (m *model) loopTick(now time.Time) tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, m.adoptLoops(now))
	for _, id := range m.loopIDs() {
		lr := m.loops[id]
		if lr == nil || lr.sending {
			continue
		}
		p := lr.batch.Progress
		// This manager's own heartbeat can lapse too: the laptop slept with
		// the manager open, and the tick is only now running again. The loop
		// ends the same way an orphan past the window does, so which manager
		// wakes first — this one, or another on the same backlog that would
		// find the stale beat in adoptLoops — never changes the outcome.
		if why, lapsed := loopLapsed(p, now); lapsed {
			m.loopClose(lr, batchStopped, why)
			continue
		}
		if p.Phase == loopPhasePause && lr.pending == "" && !now.Before(p.Until) {
			cmds = append(cmds, m.loopPhaseDone(lr, now))
			continue
		}
		if now.Sub(p.Beat) >= loopBeat {
			next := lr.batch.cloneLoop()
			next.Progress.Beat = now
			if !m.loopWrite(lr, next) {
				continue
			}
		}
	}
	cmds = append(cmds, m.loopDispatchAll(now), m.loopPoll())
	return tea.Batch(cmds...)
}

// loopPoll asks pane.list once for every loop watching a pane, unless an
// answer is still on its way.
func (m *model) loopPoll() tea.Cmd {
	if m.loopPolling || m.client == nil {
		return nil
	}
	watched := false
	for _, lr := range m.loops {
		if lr.watching() {
			watched = true
			break
		}
	}
	if !watched {
		return nil
	}
	m.loopPolling = true
	client := m.client
	return func() tea.Msg {
		panes, err := client.paneList()
		return loopPollMsg{panes: panes, err: err, at: time.Now()}
	}
}

// loopPolled hands the answer to each loop watching a pane. A failed poll is
// skipped: the next tick asks again, and a socket that stays down shows as a
// loop that does not move, with the drop picker saying why.
func (m model) loopPolled(msg loopPollMsg) (tea.Model, tea.Cmd) {
	m.loopPolling = false
	if msg.err != nil {
		return m, nil
	}
	var cmds []tea.Cmd
	for _, id := range m.loopIDs() {
		lr := m.loops[id]
		if lr == nil || !lr.watching() {
			continue
		}
		// An answer that was in flight when the laptop slept arrives before
		// the tick has had its look; judging it would send the next prompt
		// into a loop the tick is about to stop.
		if _, lapsed := loopLapsed(lr.batch.Progress, msg.at); lapsed {
			continue
		}
		switch v, why := lr.judge(observePane(msg.panes, lr.batch.Progress.Pane), msg.at); v {
		case loopFinished:
			cmds = append(cmds, m.loopPhaseDone(lr, msg.at))
		case loopFailed:
			cmds = append(cmds, m.loopFail(lr, why, msg.at))
		}
	}
	cmds = append(cmds, m.loopDispatchAll(msg.at))
	return m, tea.Batch(cmds...)
}

// adoptLoops takes over running loops nobody is driving: their owner's process
// is gone, or its heartbeat is past the lease. The take-over is a swap, so of
// two managers finding the same orphan only one gets it.
//
// A prompt the record says was being sent when the owner went, but which has no
// run, may or may not have reached its pane; the record says so on its row
// rather than guess, and the loop carries on from the prompt after it.
//
// An orphan whose heartbeat is past loopResumeWindow is not taken over but
// stopped, with the reason on its record (expireLoop). That needs no cats
// socket — it sends nothing — so it runs in a manager outside cats too, and
// the Batches page stops calling a days-old loop "paused".
func (m *model) adoptLoops(now time.Time) tea.Cmd {
	var cmds []tea.Cmd
	for _, b := range m.batchFiles() {
		if b.Deliver != deliverLoop || b.State != batchRunning || b.Progress == nil || m.loopDriven(b, now) {
			continue
		}
		if why, lapsed := loopLapsed(b.Progress, now); lapsed {
			m.expireLoop(b, why, now)
			continue
		}
		if m.client == nil {
			continue
		}
		next := b.cloneLoop()
		next.Progress.Owner, next.Progress.Beat = os.Getpid(), now
		markLoopInFlight(&next, now)
		won, err := m.batchStoreForScope(b.scope).swapBatch(b, next)
		if err != nil || !won {
			continue
		}
		next.scope = b.scope
		who := "the manager that ran it had closed"
		if b.Progress.Owner == os.Getpid() {
			who = "picked up again after a failed write"
		}
		m.batchStatus(fmt.Sprintf("loop %s: resumed at %d/%d — %s", next.displayName(), next.Progress.Next, len(next.Items), who), false)
		cmds = append(cmds, m.runLoop(next, true))
	}
	return tea.Batch(cmds...)
}

// markLoopInFlight records, on an orphaned loop taken over or expired, that
// the prompt its record says was being sent — one with no run — may or may
// not have reached its pane. Guessing either way is worse: sending it again
// could run it twice, and calling it sent could lose it.
func markLoopInFlight(b *Batch, now time.Time) {
	p := b.Progress
	if p == nil || p.Phase != loopPhasePrompt || p.Next == 0 {
		return
	}
	if _, ran := b.itemRun(p.Next - 1); !ran {
		b.Runs = append(b.Runs, BatchRun{Items: []int{p.Next - 1}, At: now,
			Err: "the manager closed while this prompt was being sent — look in its pane to see whether it arrived"})
	}
}

// expireLoop stops an orphaned loop left past loopResumeWindow, through the
// same swap a take-over uses, so of two managers finding it only one writes
// (and says so). The loop's state is left as stopLoop leaves it — no owner,
// no pane — which is what ⧉ Duplicate starts a new batch from.
func (m *model) expireLoop(b Batch, why string, now time.Time) {
	off := b.cloneLoop()
	off.State, off.Why = batchStopped, why
	off.Progress.Owner, off.Progress.Pane = 0, 0
	markLoopInFlight(&off, now)
	won, err := m.batchStoreForScope(b.scope).swapBatch(b, off)
	if err != nil || !won {
		return
	}
	m.rebuildList()
	m.batchStatus(fmt.Sprintf("loop %s: stopped at %d/%d — %s", b.displayName(), b.Progress.Next, len(b.Items), why), true)
}

// --- Stopping -----------------------------------------------------------------------

// stopLoop is ■ Stop on the page. The prompt already running in its pane is
// not interrupted — nothing on the wire can do that, and the pane is the
// user's to stop — but nothing more is sent. A loop driven by another manager
// is stopped through its record: that manager's next write loses the swap, and
// it stands down before typing again.
func (m *model) stopLoop(b Batch) (string, bool) {
	why := fmt.Sprintf("stopped by hand after %d/%d", progressNext(b), len(b.Items))
	if lr := m.loops[b.ID]; lr != nil {
		if lr.sending {
			return "a prompt is being typed into its pane right now — press ctrl+u again in a moment", true
		}
		m.loopClose(lr, batchStopped, why)
		return "stopped “" + truncate(b.displayName(), 40) + "” — what is running in its pane carries on; ⧉ Duplicate picks up the prompts not sent", false
	}
	off := b.cloneLoop()
	off.State, off.Why = batchStopped, why
	if off.Progress != nil {
		off.Progress.Owner, off.Progress.Pane = 0, 0
	}
	won, err := m.batchStoreForScope(b.scope).swapBatch(b, off)
	switch {
	case err != nil:
		return "stop failed: " + err.Error(), true
	case !won:
		return "that loop moved on in another pane just now — the rows are re-read; try again", true
	}
	m.rebuildList()
	return "stopped “" + truncate(b.displayName(), 40) + "” — the manager driving it lets it go at its next step", false
}

// progressNext is how many of a loop's prompts have been sent (or skipped).
func progressNext(b Batch) int {
	if b.Progress == nil {
		return 0
	}
	return b.Progress.Next
}
