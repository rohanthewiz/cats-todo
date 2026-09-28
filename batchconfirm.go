// batchconfirm.go — the dialog ▶ Drop now opens before a batch goes.
//
// A batch is several prompts, a delivery mode, a target and a session setup,
// and the composer spreads those over two panes and up to ten settings rows.
// Each row is legible on its own. What none of them says is what they add up
// to: "loop, same session, new worktree, /compact between, stop on a failure"
// is five facts that together mean one specific sequence of tabs, keystrokes
// and waits. A drop cannot be taken back — prompts typed into an agent start
// work — so the press asks first, and asks in sentences:
//
//	╭──────────────────────────────────────────────────────────────╮
//	│ Drop “nightly cleanup” now? · 3 prompts                      │
//	│                                                              │
//	│ Sends the prompts one at a time, in this order. The first    │
//	│ opens a new Claude Code session in ~/projs/x; every later    │
//	│ one is typed into that same session, only after the one…     │
//	│                                                              │
//	│  1. Fix flaky drop test                                      │
//	│     starts claude --model sonnet · then: run /code-review    │
//	│  2. Rename fuzzyList headings                                │
//	│     first submits /model sonnet                              │
//	│  …                                                           │
//	│                                                              │
//	│  ▶ Drop now   ✕ Back                                         │
//	│ enter confirm · esc back · ↑/↓ scroll                        │
//	╰──────────────────────────────────────────────────────────────╯
//
// # The text is derived, not written per case
//
// Every sentence is computed from the same values the delivery reads, through
// the same functions: overlaySession for a prompt's effective options,
// launchArgs for a new session's argv, paneSetupCommands for what a running
// pane is sent first, contextCommand/postamble for what wraps the prompt, and
// loopFinishText for a same-session loop's single wrap-up. So the dialog
// cannot describe a flow the batch will not run; a change to any of those is
// a change to what the dialog says. The one piece mirrored rather than shared
// is loopAction's rule that a same-session loop strips the finish from each
// prompt and sends it into the pane the first prompt opened, which is what
// flowStep reproduces for the loop.
//
// # Snapshot, then re-check
//
// The text is built once, when the dialog opens, from the prompts as they
// stand then (re-read from the backlogs, not the picks' snapshot). The
// composer cannot change under it because the dialog is modal. The world
// can: a schedule may fire meanwhile and hold the drop guard, or the target
// pane may close. Confirming therefore goes through dropBatch, which runs all
// of its checks again and refuses in words if something moved. The dialog is
// an extra question in front of that path, never a replacement for its
// checks.
//
// # Modal, in the composer's own stage
//
// Like the form's context menu, the dialog lives on its stage rather than
// being a stage of its own, so the composer's draft and focus are exactly as
// they were when it closes. It is answered first in updateBatchCompose and
// clickBatchCompose, and composited over the composer's frame (renderStage),
// so every row the composer hit-tests against stays where it was.
package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The dialog's shape. The width is chosen for reading: about 80 columns of
// text is a comfortable line, and anything wider turns a paragraph into two
// long lines the eye has to track across the pane. The floor is where a
// numbered line and its detail stop fitting at all; below it the dialog
// still opens (the confirmation is not optional), just narrow.
const (
	dropConfirmWidth = 84
	dropConfirmMin   = 36
)

// The dialog's two buttons, in drawing order.
const (
	dropConfirmBtnGo = iota
	dropConfirmBtnBack
	dropConfirmBtnCount
)

var dropConfirmLabels = [dropConfirmBtnCount]string{"▶ Drop now", "✕ Back"}

// flowLine is one logical line of the summary before wrapping. lead is a
// bullet or number drawn in front of the first row. Continuation rows are
// indented by lead's width, so a wrapped item stays visibly one item. An
// empty flowLine is a blank line between paragraphs.
type flowLine struct {
	lead string
	text string
	// detail marks a sub-line under a numbered prompt (its launch flags,
	// setup commands, wrap-up). It is drawn in the quieter field tone so the
	// titles carry the list and the details read as annotations to them.
	detail bool
}

// batchDropConfirm is the open dialog. Its zero value is closed.
type batchDropConfirm struct {
	open  bool
	title string
	lines []flowLine
	top   int // first wrapped body row shown, when the body outruns the pane
	btn   int
}

// askDropBatch is ▶ Drop now (the button and shift/alt+enter): run the same
// refusals dropBatch would, so a batch that cannot go says why straight away
// instead of after a confirmation, and otherwise open the dialog.
//
// batchTargetGoneWhy is checked here too, even though it costs a round trip
// to cats. Asking someone to confirm a flow into a pane that has already
// closed would be a question with only one honest answer.
func (m model) askDropBatch() (tea.Model, tea.Cmd) {
	if why := m.batchDropWhy(); why != "" {
		m.batchSay(why, true)
		return m, nil
	}
	if why := m.batchTargetGoneWhy(); why != "" {
		m.batchSay(why, true)
		return m, nil
	}
	title, lines := m.batchFlowSummary()
	m.batch.confirm = batchDropConfirm{open: true, title: title, lines: lines, btn: dropConfirmBtnGo}
	m.batch.dragging = false
	return m, nil
}

// closeDropConfirm puts the dialog away without sending anything, and says
// so: the note line is where the composer reports every outcome, and a
// dialog that vanished without a word would leave it unclear whether the
// batch had gone.
func (m *model) closeDropConfirm() {
	m.batch.confirm = batchDropConfirm{}
	m.batchSay("not dropped — nothing was sent", false)
}

// confirmDropBatch is the dialog's yes. The dialog closes first, so that a
// refusal from dropBatch (see the file comment on re-checking) lands on the
// composer's note line, where it can be read and acted on.
func (m model) confirmDropBatch() (tea.Model, tea.Cmd) {
	m.batch.confirm = batchDropConfirm{}
	m.batchSay("", false)
	return m.dropBatch()
}

// updateDropConfirm owns every key while the dialog is up.
//
// enter presses the highlighted button, and Drop is highlighted on open. The
// press that opened the dialog was already a deliberate "drop", so the common
// path is one more key. y and a second shift/alt+enter also confirm, which
// lets the gesture that opened the dialog finish it. n and esc go back.
func (m model) updateDropConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	c := &m.batch.confirm
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "n", "N":
		m.closeDropConfirm()
		return m, nil
	case "y", "Y", "shift+enter", "alt+enter":
		return m.confirmDropBatch()
	case "enter", "space":
		if c.btn == dropConfirmBtnGo {
			return m.confirmDropBatch()
		}
		m.closeDropConfirm()
		return m, nil
	case "left", "right", "tab", "shift+tab", "h", "l":
		c.btn = (c.btn + 1) % dropConfirmBtnCount
	case "up", "k":
		c.top--
	case "down", "j":
		c.top++
	case "pgup":
		c.top -= m.dropConfirmGeom().bodyH
	case "pgdown":
		c.top += m.dropConfirmGeom().bodyH
	case "home":
		c.top = 0
	case "end":
		c.top = len(m.dropConfirmBody(m.dropConfirmGeom().textW))
	}
	c.top = m.clampDropConfirmTop(c.top)
	return m, nil
}

// clickDropConfirm is a press while the dialog is up. A click on a button
// presses it. A click off the box goes back, the rule every floating surface
// here follows (menus, the flag pad), because it is how people dismiss a
// popup without looking for its key. A click inside the box on text does
// nothing.
func (m model) clickDropConfirm(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	g := m.dropConfirmGeom()
	x, y := msg.X, msg.Y
	if x < g.x || x >= g.x+g.w || y < g.y || y >= g.y+g.h {
		m.closeDropConfirm()
		return m, nil
	}
	if y == g.btnY {
		for i, sp := range g.btnSpans {
			if x >= sp[0] && x < sp[1] {
				if i == dropConfirmBtnGo {
					return m.confirmDropBatch()
				}
				m.closeDropConfirm()
				return m, nil
			}
		}
	}
	return m, nil
}

// --- The summary ------------------------------------------------------------------

// batchFlowSummary describes, in sentences, exactly what dropping the
// composer's batch now will do, in the order it happens. It reads the
// composer and the backlogs and writes nothing.
//
// The layout is three blocks:
//
//	what happens     one paragraph for the delivery mode and target
//	the prompts      numbered in delivery order, each with what it runs with
//	the rules        a loop's waits and failure rule, then the bookkeeping
//	                 every batch does (Next List items saved, the record, done)
func (m model) batchFlowSummary() (string, []flowLine) {
	bc := m.batch
	n := len(bc.picked)
	title := fmt.Sprintf("Drop %d prompt%s now?", n, plural(n))
	if name := strings.TrimSpace(bc.name.Value()); name != "" {
		title = fmt.Sprintf("Drop “%s” now? · %d prompt%s", name, n, plural(n))
	}

	var out []flowLine
	para := func(s string) {
		if len(out) > 0 {
			out = append(out, flowLine{})
		}
		out = append(out, flowLine{text: s})
	}
	bullet := func(s string) { out = append(out, flowLine{lead: " • ", text: s}) }

	t := bc.target
	loop := bc.deliver == deliverLoop
	fresh := loop && bc.fresh

	// --- What happens.
	para(m.flowOpening())

	// --- The prompts, in delivery order.
	out = append(out, flowLine{})
	width := len(fmt.Sprint(n))
	for i, c := range bc.picked {
		lead := fmt.Sprintf(" %*d. ", width, i+1)
		text := c.title
		if c.next.ID != "" && !strings.HasPrefix(text, c.next.ID) {
			text = c.next.ID + " " + text
		}
		out = append(out, flowLine{lead: lead, text: text})
		if bc.deliver == deliverCombined {
			// One message carries them all, so the setup is the batch's and
			// is described once, below the list.
			continue
		}
		if d := m.flowStep(i, c, t, loop, fresh); d != "" {
			out = append(out, flowLine{lead: strings.Repeat(" ", len(lead)), text: d, detail: true})
		}
	}
	if bc.deliver == deliverCombined {
		if d := m.flowCombinedStep(); d != "" {
			para("The one message " + d + ".")
		}
		if n := m.flowOwnOptionsIgnored(); n > 0 {
			who := choose(n == 1, "One of these prompts has options of its own", fmt.Sprintf("%d of these prompts have options of their own", n))
			para(who + "; one combined message can only use the batch's ⚙ options, so " +
				choose(n == 1, "those are", "theirs are") + " not applied.")
		}
	}

	// --- A loop's rules.
	if loop {
		out = append(out, flowLine{})
		o, _ := normalizeLoopOpts(bc.loopOpts())
		if o.Between != "" {
			where := "that same session"
			if fresh {
				where = "the session that just finished"
			}
			s := fmt.Sprintf("After each prompt, submits “%s” to %s and waits for it to finish too", o.Between, where)
			if o.BetweenLast {
				s += " — after the last prompt as well."
			} else {
				s += " — but not after the last prompt."
			}
			bullet(s)
		}
		if o.Pause != "" {
			bullet("Then waits " + o.Pause + " before sending the next prompt.")
		}
		if o.MaxWait != "" {
			bullet("A prompt still working after " + o.MaxWait + " counts as failed.")
		} else {
			bullet("No time limit: a prompt may work for as long as it takes.")
		}
		bullet(fmt.Sprintf("A prompt the agent has not started on after %s, or whose pane closes or whose agent exits, counts as failed.",
			shortDuration(loopStartWait)))
		if o.OnFail == loopOnFailSkip {
			bullet("A failure is recorded and the loop moves on to the next prompt.")
		} else {
			bullet("A failure stops the loop there: the prompts after it are not sent.")
		}
		if !fresh {
			if fin := m.flowLoopFinish(); fin != "" {
				bullet("After the last prompt, sends one wrap-up message into the session: " + fin + ". Each prompt's own finish step is held back for it, so the run ends in one commit rather than one per prompt.")
			}
		}
		bullet(fmt.Sprintf("This manager drives the loop. If it closes, a cats-todo manager opened within %s can take the loop over; after that it is stopped.",
			shortDuration(loopResumeWindow)))
	}

	// --- Bookkeeping every batch does.
	out = append(out, flowLine{})
	if ids := m.flowNextIDs(); len(ids) > 0 {
		bullet(fmt.Sprintf("First, %s %s saved to the %s backlog as prompt%s (an open copy already there is used instead).",
			joinAnd(ids), choose(len(ids) == 1, "is", "are"), m.nextAddScopeName(), plural(len(ids))))
	}
	if e := bc.edit; e.ID != "" {
		bullet("The batch is taken off its " + m.flowEditState(e) + ", so it cannot also go later.")
	}
	bullet("A record of the batch is written before anything is sent and updated after each prompt — ctrl+k shows it.")
	bullet("Each prompt is marked done once it is delivered. One that is done, frozen or deleted by its turn is skipped, and the record says why.")
	return title, out
}

// flowOpening is the paragraph that says what the delivery mode does with
// the target: how many sessions, where, and in what rhythm.
func (m model) flowOpening() string {
	bc := m.batch
	t := bc.target
	n := len(bc.picked)
	cwd := shortenHome(m.ctx.projectDir())
	agent := flowAgentName(t.command)
	wt := ""
	if t.worktree {
		wt = " on a new worktree (a fresh checkout on its own todo/… branch)"
	}
	pane := "the running pane “" + firstNonEmpty(strings.TrimSpace(t.label), fmt.Sprint(t.pane)) + "”"

	switch bc.deliver {
	case deliverCombined:
		dest := pane
		if t.kind == targetNewSession {
			dest = fmt.Sprintf("a new %s session%s in %s", agent, wt, cwd)
		}
		named := "tab"
		if t.worktree {
			named = "tab and branch"
		}
		return fmt.Sprintf("Sends ONE message to %s that lists all %d prompts as numbered sections, in this order. Its %s %s named “%s”.",
			dest, n, named, choose(named == "tab", "is", "are"), m.flowDisplayName())
	case deliverLoop:
		if bc.fresh {
			return fmt.Sprintf("Sends the prompts one at a time, in this order, each into its own new %s session%s in %s. The next session opens only once the one before it has finished (its agent went from working back to idle).",
				agent, wt, cwd)
		}
		if t.kind == targetNewSession {
			return fmt.Sprintf("Sends the prompts one at a time, in this order. The first opens a new %s session%s in %s; every later one is typed into that same session, only after the one before it has finished (its agent went from working back to idle).",
				agent, wt, cwd)
		}
		return fmt.Sprintf("Sends the prompts one at a time, in this order, all into %s. Each is typed in only after the one before it has finished (its agent went from working back to idle).", pane)
	}
	// All at once.
	if t.kind == targetExistingPane {
		return "Types the prompt into " + pane + " and submits it."
	}
	if n == 1 {
		return fmt.Sprintf("Opens a new %s session%s in %s and submits the prompt there.", agent, wt, cwd)
	}
	return fmt.Sprintf("Opens %d new %s sessions%s in %s, one after another, and submits one prompt in each. Each tab opens as soon as the last has its prompt; none waits for another's work to finish.",
		n, agent, strings.Replace(wt, "a new worktree", "a new worktree each", 1), cwd)
}

// flowStep is the detail line under prompt i: what that prompt's session is
// set up with and what is wrapped around it. It mirrors the delivery exactly:
//
//	all at once / fresh loop   new session each: launch flags; finish kept
//	same-session loop, first   new session (or the target pane); finish lifted
//	same-session loop, later   the loop's pane: setup commands; finish lifted
//	existing-pane target       setup commands (/clear, /model, /effort)
//
// "" when the prompt goes as it is.
func (m model) flowStep(i int, c batchCand, t dropTarget, loop, fresh bool) string {
	own, images := m.flowOwn(c)
	eff := overlaySession(own, sessionPtr(m.batch.session))
	if loop && !fresh && eff != nil {
		// loopAction's rule: the finish goes once, after the last prompt.
		o := eff.clone()
		o.Finish, o.Release = finishNone, false
		eff = sessionPtr(o)
	}
	intoPane := t.kind == targetExistingPane || (loop && !fresh && i > 0)
	agent := t.agent
	if t.kind == targetNewSession {
		// The loop's later prompts go to the pane the first one opened, as
		// a drop into a running pane of the launched agent (loopAction).
		agent = firstNonEmpty(t.command, "claude")
	}
	return flowSetup(eff, images, intoPane, t.command, agent)
}

// flowCombinedStep is flowStep for the one combined message: only the batch's
// options, and every prompt's images.
func (m model) flowCombinedStep() string {
	t := m.batch.target
	images := 0
	for _, c := range m.batch.picked {
		_, k := m.flowOwn(c)
		images += k
	}
	return flowSetup(sessionPtr(m.batch.session), images, t.kind == targetExistingPane, t.command, t.agent)
}

// flowSetup renders one step's setup as a clause list: how the session
// starts, what it is sent first, what wraps the prompt, what it carries.
func flowSetup(eff *SessionOpts, images int, intoPane bool, command, agent string) string {
	var segs []string
	if eff != nil {
		if intoPane {
			if cmds := eff.paneSetupCommands(agent); len(cmds) > 0 {
				segs = append(segs, "first submits "+strings.Join(cmds, ", "))
			}
			if lost := eff.paneUnapplied(agent); lost != "" {
				segs = append(segs, lost+" cannot be set on a running pane and is left as it is")
			}
		} else if args := eff.launchArgs(firstNonEmpty(command, "claude")); len(args) > 0 {
			segs = append(segs, "starts "+firstNonEmpty(command, "claude")+" "+strings.Join(args, " "))
		}
		if cmd := eff.contextCommand(); cmd != "" {
			segs = append(segs, "asks for "+cmd+" first")
		}
		if k := len(eff.Files); k > 0 {
			segs = append(segs, fmt.Sprintf("asks it to read %d more file%s", k, plural(k)))
		}
		if post := postambleSteps(eff.postamble()); len(post) > 0 {
			segs = append(segs, "then: "+strings.Join(post, "; "))
		}
	}
	if images > 0 {
		segs = append(segs, fmt.Sprintf("%d image%s attached as file paths", images, plural(images)))
	}
	return strings.Join(segs, " · ")
}

// postambleSteps is a postamble's list items without its header or bullets,
// so the dialog quotes the exact wrap-up the prompt will carry.
func postambleSteps(post string) []string {
	var steps []string
	for _, l := range strings.Split(post, "\n") {
		if s, ok := strings.CutPrefix(strings.TrimSpace(l), "- "); ok {
			steps = append(steps, s)
		}
	}
	return steps
}

// flowOwn is a pick's own options and its attachment count, re-read from the
// backlog: that is what startBatch and the loop deliver, and the pick's
// snapshot may be minutes old. A Next List item has neither yet.
func (m model) flowOwn(c batchCand) (*SessionOpts, int) {
	if c.next.ID != "" {
		return nil, 0
	}
	td, ok := m.resolve(c.ref)
	if !ok {
		return c.session, 0
	}
	return td.Session, len(m.storeFor(c.ref.scope).imagePaths(td))
}

// flowOwnOptionsIgnored counts the picks whose own options a combined drop
// will not apply.
func (m model) flowOwnOptionsIgnored() int {
	k := 0
	for _, c := range m.batch.picked {
		if own, _ := m.flowOwn(c); own.configured() {
			k++
		}
	}
	return k
}

// flowLoopFinish is a same-session loop's wrap-up in words, taken from
// loopFinishText itself so the dialog quotes what will be sent. The last pick
// is its only per-prompt input, and a Next List item there has no options of
// its own, so the batch's options decide.
func (m model) flowLoopFinish() string {
	bc := m.batch
	b := Batch{Session: sessionPtr(bc.session)}
	if n := len(bc.picked); n > 0 && bc.picked[n-1].next.ID == "" {
		last := bc.picked[n-1].ref
		b.Items = []BatchItem{{Scope: batchScopeName(last.scope), ID: last.id}}
	}
	return strings.Join(postambleSteps(m.loopFinishText(b)), "; ")
}

// flowNextIDs are the picked Next List items, which buildBatch turns into
// backlog prompts before anything is sent.
func (m model) flowNextIDs() []string {
	var ids []string
	for _, c := range m.batch.picked {
		if c.next.ID != "" {
			ids = append(ids, c.next.ID)
		}
	}
	return ids
}

// flowEditState says what an edited batch is taken off: its schedule, and
// for when, or its unscheduled/missed plan.
func (m model) flowEditState(e Batch) string {
	switch {
	case e.State == batchScheduled && !e.At.IsZero():
		return "schedule (it was set for " + formatScheduleTime(e.At, time.Now()) + ")"
	case e.State == batchMissed:
		return "missed schedule"
	}
	return "unscheduled plan"
}

// flowDisplayName is what a combined drop's tab (and worktree branch) is
// named: the record's displayName, computed from the composer.
func (m model) flowDisplayName() string {
	b := Batch{Name: strings.TrimSpace(m.batch.name.Value())}
	for _, c := range m.batch.picked {
		b.Items = append(b.Items, BatchItem{Title: c.title})
	}
	return b.displayName()
}

// flowAgentName is a launch command as a person names it: the picker's own
// words for the agents it offers ("Claude Code"), the command otherwise.
func flowAgentName(command string) string {
	command = firstNonEmpty(command, "claude")
	for _, a := range newSessionAgents {
		if a.command == command {
			s := strings.TrimPrefix(a.label, "＋ New ")
			return strings.TrimSuffix(s, " session")
		}
	}
	return command
}

// shortDuration is a whole duration without Go's trailing zero units
// ("45s", "1h", not "1h0m0s").
func shortDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// choose is a sentence's agreement: one word or the other, on a condition.
func choose(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

// joinAnd lists words the way a sentence does: "a", "a and b", "a, b and c".
func joinAnd(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}

// --- Drawing ----------------------------------------------------------------------

// dropConfirmGeometry is where the dialog is and where its rows fall. The view
// and the pointer both read it, as batchGeom serves the composer, so a click
// can never land on a button the frame drew somewhere else.
//
//	y        top border
//	y+1      title
//	y+2      blank
//	y+3 …    body (bodyH rows, scrolled by top)
//	btnY-1   blank
//	btnY     buttons
//	btnY+1   hint
//	         bottom border
type dropConfirmGeometry struct {
	x, y, w, h int
	textW      int // cells for text inside the border and padding
	bodyH      int // body rows shown
	btnY       int
	btnSpans   [dropConfirmBtnCount][2]int // screen columns [start, end) per button
}

// dropConfirmChrome is the rows the dialog spends on everything but the body:
// two borders, title, blank, blank, buttons, hint.
const dropConfirmChrome = 7

func (m model) dropConfirmGeom() dropConfirmGeometry {
	var g dropConfirmGeometry
	g.w = min(dropConfirmWidth, m.width-4)
	if g.w < dropConfirmMin {
		g.w = min(dropConfirmMin, m.width)
	}
	g.textW = max(g.w-4, 1) // border 1 + padding 1 on each side
	total := len(m.dropConfirmBody(g.textW))
	// A row of margin above and below when the pane has it, so the box reads
	// as floating over the composer rather than replacing it.
	maxBody := max(m.height-dropConfirmChrome-2, 1)
	g.bodyH = min(total, maxBody)
	g.h = g.bodyH + dropConfirmChrome
	g.x = max((m.width-g.w)/2, 0)
	g.y = max((m.height-g.h)/2, 0)
	g.btnY = g.y + 3 + g.bodyH + 1
	col := g.x + 2 // border + padding
	for i, l := range dropConfirmLabels {
		w := ansi.StringWidth(l) + 2 // a space either side, the chip's field
		g.btnSpans[i] = [2]int{col, col + w}
		col += w + 2
	}
	return g
}

// clampDropConfirmTop keeps the scroll inside the body.
func (m model) clampDropConfirmTop(top int) int {
	g := m.dropConfirmGeom()
	total := len(m.dropConfirmBody(g.textW))
	return max(min(top, total-g.bodyH), 0)
}

// dropConfirmBody is the summary wrapped to w cells, as rendered rows.
// Wrapping is by words; a lead's continuation rows are indented to the text,
// so a numbered prompt that wraps still reads as one item.
func (m model) dropConfirmBody(w int) []string {
	var rows []string
	for _, l := range m.batch.confirm.lines {
		if l.lead == "" && l.text == "" {
			rows = append(rows, hoverBodyStyle.Width(w+2).Render(""))
			continue
		}
		st := nextCardBodyStyle
		if l.detail {
			st = hoverFieldStyle
		}
		lw := ansi.StringWidth(l.lead)
		tw := max(w-lw, 8)
		wrapped := strings.Split(ansi.Wrap(l.text, tw, ""), "\n")
		for k, r := range wrapped {
			lead := l.lead
			if k > 0 {
				lead = strings.Repeat(" ", lw)
			}
			rows = append(rows, st.Width(w+2).Render(ansi.Truncate(lead+r, w, "…")))
		}
	}
	return rows
}

// renderDropConfirm draws the dialog. The box and field are the menus' (the
// same kind of floating surface), the title the hover card's, and the buttons
// light the way a menu row does when the keys are on it.
func (m model) renderDropConfirm() string {
	g := m.dropConfirmGeom()
	c := m.batch.confirm
	inner := g.w - 2
	blank := hoverBodyStyle.Width(inner).Render("")

	body := m.dropConfirmBody(g.textW)
	top := max(min(c.top, len(body)-g.bodyH), 0)
	body = body[top : top+g.bodyH]

	var chips []string
	for i, l := range dropConfirmLabels {
		st := menuRowStyle
		if i == c.btn {
			st = menuRowSelStyle
		}
		chips = append(chips, st.Render(" "+l+" "))
	}
	btns := menuRowStyle.Render(" ") + strings.Join(chips, menuRowStyle.Render("  "))
	btns = lipgloss.NewStyle().Background(lipgloss.Color(colPanel)).Width(inner).Render(btns)

	hint := "enter confirm · esc back · ←/→ button"
	if len(m.dropConfirmBody(g.textW)) > g.bodyH {
		hint += fmt.Sprintf(" · ↑/↓ scroll (%d–%d of %d)", top+1, top+g.bodyH, len(m.dropConfirmBody(g.textW)))
	}
	rows := []string{
		hoverTitleStyle.Width(inner).Render(ansi.Truncate(c.title, g.textW, "…")),
		blank,
	}
	rows = append(rows, body...)
	rows = append(rows, blank, btns, hoverFieldStyle.Width(inner).Render(ansi.Truncate(hint, g.textW, "…")))
	return menuBoxStyle.Render(strings.Join(rows, "\n"))
}

// overlayDropConfirm floats the dialog over the composer's frame (see
// overlayMenu, menu.go, for why it is composited rather than spliced).
func (m model) overlayDropConfirm(view string) string {
	if !m.batch.confirm.open {
		return view
	}
	g := m.dropConfirmGeom()
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(view),
		lipgloss.NewLayer(m.renderDropConfirm()).X(g.x).Y(g.y).Z(1),
	).Render()
}
