// batchcompose.go — the batch composer: pick prompts, order them, set the
// batch up, drop it.
//
// Two panes side by side, the batch's settings under the right one, and a
// button row:
//
//	CatsTodo v0.39.0 - New batch
//
//	 Backlog │ Next List  ctrl+g        │ Batch · 3 prompts
//	╎🔍 filter                  ╎ 3/12   │ ⠿  1. Fix flaky drop test        ▲ ⚙
//	 ☒ all · 3 of 12 picked            │ ⠿  2. Rename fuzzyList headings
//	                                    │ ⠿  3. N-014 Tidy promptsel…      ✚
//	 Project                            │
//	❯☑ Fix flaky drop test              │ order: manual · s sorts A→Z
//	 ☑ Rename fuzzyList headings        │
//	 ☐ ▲ Add worktree cleanup command   │ Name     nightly cleanup
//	 ☐ Draft blog notes · info          │ Deliver  (•) all at once  ( ) one prompt, listed
//	                                    │ Target   ＋ New Claude Code session on a new worktree
//	                                    │ Session  ⚙ sonnet · high
//
//	  ▶ Drop now shift+enter  ⇅ A→Z s  ☰ Batches ctrl+k  ✕ Cancel esc
//	  <note line>
//	  <footer: the keys of whichever region holds the focus>
//
// Below composerSplitMin columns the two panes do not fit side by side without
// cutting every title to a stub, so they take turns instead: a "Pick · Batch"
// switcher on the line under the title, and tab (or a click on it) moves
// between them. The regions and their keys are the same in both layouts; only
// how many are on screen at once changes.
//
// A checkbox is membership. There is no "add →" step between the panes: the
// right pane is simply the checked rows, in the order they were checked, so
// the only way for the two to disagree would be a bug. Order is then the right
// pane's business — dragged, alt+↑/↓, or sorted once by title. The sort is an
// action, not a lens: the order it leaves is an ordinary order that can be
// dragged afterwards, because a batch's order is what gets delivered and has to
// be something the user can see and edit, not a view over something else.
//
// Next List items can be picked beside backlog prompts. They become backlog
// prompts only when the batch is dropped (nextItemAsPrompt), so a composer
// abandoned with esc has written nothing anywhere.
package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// composerSplitMin is the narrowest pane the two panes share side by side. At
// 100 each gets about 48 columns, which holds a title of useful length; below
// it they take turns (see the file comment).
const composerSplitMin = 100

// The Pick pane's two sources.
const (
	batchSrcBacklog = iota
	batchSrcNext
)

// The composer's regions, in tab order: where the keys go.
const (
	batchFocusPick = iota
	batchFocusBatch
	batchFocusSettings
	batchFocusButtons
	batchFocusCount
)

// The settings rows under the Batch pane, in the order they are drawn.
const (
	batchSetName = iota
	batchSetDeliver
	batchSetTarget
	batchSetSession
	batchSetCount
)

// The button row, in order (see batchActions).
const (
	batchBtnDrop = iota
	batchBtnSort
	batchBtnBatches
	batchBtnCancel
)

// batchDeliverModes is the Deliver row's ring, in the order its radios are
// drawn.
var batchDeliverModes = []string{deliverEach, deliverCombined}

// batchCand is one prompt the composer can pick: a backlog prompt (ref set) or
// a Next List item (next.ID set). The Pick pane's rows and the Batch pane's
// rows are both made of these, so a pick carries everything the right pane
// draws without going back to a store on every frame.
type batchCand struct {
	ref     todoRef
	next    nextItem
	title   string
	session *SessionOpts
	marks   []annotMark
	// why is set on a row that is listed but cannot be picked, and says why —
	// the refusal is on the row before the press, and in words after it.
	why string
}

// key is the pick's identity across rebuilds: the Pick pane is rebuilt on
// every toggle and every source switch, and membership has to survive both.
func (c batchCand) key() string {
	if c.next.ID != "" {
		return "n:" + c.next.ID
	}
	return fmt.Sprintf("b:%d:%s", c.ref.scope, c.ref.id)
}

// batchComposer is the open composer. Its zero value is "not open"; everything
// in it lives and dies with the visit, except the draft settings a trip to the
// target picker or the session panel leaves and comes back to.
type batchComposer struct {
	// from is the screen esc goes back to.
	from   uiStage
	source int
	pick   fuzzyList
	cands  []batchCand // the current source's rows; a listItem's ref indexes this
	// The Next List is read once per visit, the first time its tab is shown.
	nextItems  []nextItem
	nextErr    string
	nextLoaded bool

	picked []batchCand // the batch, in delivery order
	cursor int         // the Batch pane's highlighted row
	top    int         // the first Batch row drawn, when they outrun the pane

	focus  int
	setRow int
	btn    int

	name    textinput.Model
	deliver string
	target  dropTarget
	session SessionOpts
	// sorted says the order is the A→Z sort's, so the order line can say so.
	// Any move clears it: the order is the user's again.
	sorted bool

	note    string
	noteErr bool

	// A drag in the Batch pane: the pick in the hand, and whether it moved.
	dragging  bool
	dragKey   string
	dragMoved bool
}

// defaultBatchTarget is the target a new batch starts on: a new Claude Code
// session, the picker's own first row, so a batch is droppable without a trip
// to the picker.
func defaultBatchTarget() dropTarget {
	return dropTarget{kind: targetNewSession, command: "claude", label: newSessionAgents[0].label}
}

// beginBatchCompose opens the composer with preset already picked, on source.
func (m model) beginBatchCompose(from uiStage, source int, preset []batchCand) (tea.Model, tea.Cmd) {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "optional"
	ti.CharLimit = 80
	bc := batchComposer{
		from:    from,
		source:  source,
		name:    ti,
		deliver: deliverEach,
		target:  defaultBatchTarget(),
		picked:  preset,
	}
	bc.pick = newFuzzyList("filter", nil)
	bc.pick.checkboxes = true
	m.batch = bc
	m.clearHover()
	m.stage = stageBatchCompose
	m.rebuildBatchPick()
	m.sizeBatchCompose()
	return m, m.setBatchFocus(batchFocusPick)
}

// leaveBatchCompose goes back to the screen the composer was opened from. The
// draft goes with it: a composer is a gesture, not a document.
func (m *model) leaveBatchCompose() {
	from := m.batch.from
	m.batch = batchComposer{}
	m.pickForBatch, m.sessForBatch = false, false
	switch from {
	case stageNextList:
		m.stage = stageNextList
		m.next.resize(m.width, m.height)
	case stageBatches:
		m.openBatchesPage()
	default:
		m.backToList()
	}
}

// batchSay sets the composer's note line.
func (m *model) batchSay(s string, isErr bool) {
	m.batch.note, m.batch.noteErr = s, isErr
}

// --- The Pick pane ------------------------------------------------------------

// isPicked reports whether key is in the batch.
func (bc batchComposer) isPicked(key string) bool {
	return slices.ContainsFunc(bc.picked, func(c batchCand) bool { return c.key() == key })
}

// rebuildBatchPick rebuilds the Pick pane's rows for the current source,
// keeping the query and the highlight.
//
// Only open prompts are listed. Done and frozen ones are closed work — a done
// backlog can be hundreds of rows — and listing them to refuse them would bury
// the rows that can be picked. An info prompt is open but is a note, not work,
// so it is listed dimmed with the reason, since it is exactly the row someone
// would expect to find and wonder about.
func (m *model) rebuildBatchPick() {
	bc := &m.batch
	var items []listItem
	bc.cands = bc.cands[:0]
	add := func(c batchCand, it listItem) {
		it.ref = len(bc.cands)
		it.selectable = true
		it.marked = bc.isPicked(c.key())
		if c.why != "" {
			it.dim = true
			it.tag = c.why
		}
		bc.cands = append(bc.cands, c)
		items = append(items, it)
	}
	// The name is cut to the pane here, like the Next List page's rows: a row
	// that wrapped would push every row under it a line below where the
	// pointer's hit-test believes it is.
	room := func(extra int) int {
		w := m.batchGeom().pickW
		if w <= 0 {
			return 60
		}
		return max(w-indentWidth-2-extra-2, 8)
	}

	switch bc.source {
	case batchSrcBacklog:
		stores := []*store{m.project, m.global}
		open := func(s *store) int {
			n := 0
			for _, t := range s.todos {
				if !t.closed() {
					n++
				}
			}
			return n
		}
		headed := open(m.project) > 0 && open(m.global) > 0
		for _, s := range stores {
			if headed && open(s) > 0 {
				items = append(items, listItem{name: s.scope.String()})
			}
			for _, t := range s.todos {
				if t.closed() {
					continue
				}
				c := batchCand{
					ref:     todoRef{scope: s.scope, id: t.ID},
					title:   firstNonEmpty(t.Title, firstLine(t.Prompt, 60)),
					session: t.Session,
					marks:   annotMarksFor(t),
				}
				tag := ""
				switch {
				case t.Info:
					c.why = "info — a note, not work"
				case t.Schedule != nil && !t.Schedule.Missed:
					// Pickable, but the double booking is said before the
					// batch is saved: this prompt will also fire on its own.
					tag = "◷ " + formatScheduleTime(t.Schedule.At, time.Now())
				}
				marksW := 0
				for _, mk := range c.marks {
					marksW += lipgloss.Width(mk.text) + 1
				}
				tagW := 0
				if tag != "" || c.why != "" {
					tagW = lipgloss.Width(firstNonEmpty(c.why, tag)) + 3
				}
				it := listItem{
					name:   truncate(c.title, room(marksW+tagW)),
					annots: c.marks,
					search: t.Prompt,
				}
				if tag != "" {
					it.tag = tag
				}
				add(c, it)
			}
		}
	case batchSrcNext:
		if !bc.nextLoaded {
			bc.nextItems, _, bc.nextErr = loadNextList(m.nextListRoot())
			bc.nextLoaded = true
		}
		section := ""
		for _, ni := range bc.nextItems {
			if ni.Section != section {
				section = ni.Section
				items = append(items, listItem{name: section})
			}
			flat := collapseLines(ni.Text)
			c := batchCand{next: ni, title: nextItemTitle(ni)}
			add(c, listItem{
				name:       truncate(flat, room(lipgloss.Width(ni.ID)+1+nextValueMarkWidth+1)),
				badge:      ni.ID,
				badgeStyle: nextValueStyle(ni.Value),
				annots:     []annotMark{nextValueMark(ni.Value)},
				search:     strings.Join([]string{ni.ID, ni.Value, ni.Section, flat}, " "),
			})
		}
	}
	bc.pick.setItems(items)
}

// pickEmptyMsg is what the Pick pane says when it has no rows.
func (m model) pickEmptyMsg() string {
	if m.batch.source == batchSrcNext && m.batch.nextErr != "" {
		return m.batch.nextErr
	}
	if strings.TrimSpace(m.batch.pick.input.Value()) != "" {
		return "nothing matched"
	}
	return "no open prompts to pick"
}

// toggleBatchCand checks or unchecks candidate i. Checking appends: the batch's
// order starts as the order things were picked in, which is usually the order
// they came to mind.
func (m *model) toggleBatchCand(i int) {
	bc := &m.batch
	if i < 0 || i >= len(bc.cands) {
		return
	}
	c := bc.cands[i]
	if c.why != "" {
		m.batchSay("can't pick that one: "+c.why, false)
		return
	}
	key := c.key()
	if j := slices.IndexFunc(bc.picked, func(p batchCand) bool { return p.key() == key }); j >= 0 {
		bc.picked = slices.Delete(bc.picked, j, j+1)
		bc.cursor = min(bc.cursor, max(len(bc.picked)-1, 0))
		m.batchSay("", false)
	} else {
		bc.picked = append(bc.picked, c)
		bc.sorted = false
		m.batchSay("", false)
	}
	m.rebuildBatchPick()
}

// toggleBatchAll is ☐ all: every visible pickable row in, or — when they are
// all in already — every visible row out. "Visible" is the filter's answer, so
// filtering first and then pressing it picks exactly what matched.
func (m *model) toggleBatchAll() {
	bc := &m.batch
	var vis []int
	for _, s := range bc.pick.filtered {
		if s.item.selectable && bc.cands[s.item.ref].why == "" {
			vis = append(vis, s.item.ref)
		}
	}
	if len(vis) == 0 {
		m.batchSay("nothing here to pick", false)
		return
	}
	all := true
	for _, i := range vis {
		if !bc.isPicked(bc.cands[i].key()) {
			all = false
			break
		}
	}
	for _, i := range vis {
		c := bc.cands[i]
		in := bc.isPicked(c.key())
		switch {
		case all && in:
			key := c.key()
			bc.picked = slices.DeleteFunc(bc.picked, func(p batchCand) bool { return p.key() == key })
		case !all && !in:
			bc.picked = append(bc.picked, c)
			bc.sorted = false
		}
	}
	bc.cursor = min(bc.cursor, max(len(bc.picked)-1, 0))
	m.rebuildBatchPick()
	m.batchSay("", false)
}

// pickCounts is the all-row's arithmetic: picked among the visible rows, and
// how many visible rows can be picked.
func (bc batchComposer) pickCounts() (picked, pickable int) {
	for _, s := range bc.pick.filtered {
		if !s.item.selectable || bc.cands[s.item.ref].why != "" {
			continue
		}
		pickable++
		if s.item.marked {
			picked++
		}
	}
	return picked, pickable
}

// switchBatchSource flips the Pick pane between the backlog and the Next List.
// The query is cleared: a filter typed for one source is words about the other.
func (m *model) switchBatchSource(src int) {
	if m.batch.source == src {
		return
	}
	m.batch.source = src
	m.batch.pick.input.SetValue("")
	m.batch.pick.cursor, m.batch.pick.top = 0, 0
	m.rebuildBatchPick()
	m.sizeBatchCompose()
}

// --- The Batch pane -----------------------------------------------------------

// moveBatchPick moves the highlighted pick by delta, keeping the highlight on
// it.
func (m *model) moveBatchPick(delta int) {
	bc := &m.batch
	j := bc.cursor + delta
	if bc.cursor < 0 || bc.cursor >= len(bc.picked) || j < 0 || j >= len(bc.picked) {
		return
	}
	bc.picked[bc.cursor], bc.picked[j] = bc.picked[j], bc.picked[bc.cursor]
	bc.cursor = j
	bc.sorted = false
	m.ensureBatchVisible()
}

// removeBatchPick takes the highlighted pick out of the batch (and so unchecks
// it on the left).
func (m *model) removeBatchPick() {
	bc := &m.batch
	if bc.cursor < 0 || bc.cursor >= len(bc.picked) {
		m.batchSay("nothing to remove", false)
		return
	}
	title := bc.picked[bc.cursor].title
	bc.picked = slices.Delete(bc.picked, bc.cursor, bc.cursor+1)
	bc.cursor = min(bc.cursor, max(len(bc.picked)-1, 0))
	m.rebuildBatchPick()
	m.ensureBatchVisible()
	m.batchSay("removed “"+truncate(title, 40)+"”", false)
}

// sortBatchPicks sorts the batch by title, once. The highlight stays on the
// pick it was on, wherever the sort put it.
func (m *model) sortBatchPicks() {
	bc := &m.batch
	if len(bc.picked) < 2 {
		m.batchSay("nothing to sort", false)
		return
	}
	var keep string
	if bc.cursor >= 0 && bc.cursor < len(bc.picked) {
		keep = bc.picked[bc.cursor].key()
	}
	slices.SortStableFunc(bc.picked, func(a, b batchCand) int {
		return strings.Compare(strings.ToLower(a.title), strings.ToLower(b.title))
	})
	bc.cursor = max(slices.IndexFunc(bc.picked, func(c batchCand) bool { return c.key() == keep }), 0)
	bc.sorted = true
	m.ensureBatchVisible()
	m.batchSay("sorted A→Z — drag or alt+↑/↓ to adjust", false)
}

// ensureBatchVisible scrolls the Batch pane so its highlight is drawn.
func (m *model) ensureBatchVisible() {
	bc := &m.batch
	n := m.batchGeom().batchRows
	if n <= 0 {
		return
	}
	if bc.cursor < bc.top {
		bc.top = bc.cursor
	}
	if bc.cursor >= bc.top+n {
		bc.top = bc.cursor - n + 1
	}
	bc.top = max(min(bc.top, len(bc.picked)-n), 0)
}

// --- Settings -----------------------------------------------------------------

// cycleBatchDeliver steps the Deliver radio.
func (m *model) cycleBatchDeliver(delta int) {
	m.batch.deliver = cycleValue(batchDeliverModes, m.batch.deliver, delta)
}

// beginBatchTarget opens the ordinary target picker in batch flavour: enter
// records the row on the draft instead of dropping (see chooseTarget).
func (m model) beginBatchTarget() (tea.Model, tea.Cmd) {
	if m.client == nil {
		m.batchSay("cats control socket unavailable — the batch can't be dropped from here", true)
		return m, nil
	}
	m.pickForBatch = true
	m.targets, m.targetList = m.buildTargets()
	// Land on the row the draft already has, so enter keeps the choice.
	for i, t := range m.targets {
		if t.kind == m.batch.target.kind && t.command == m.batch.target.command &&
			t.worktree == m.batch.target.worktree && t.pane == m.batch.target.pane {
			m.targetList.selectRef(i)
			break
		}
	}
	m.stage = stageTarget
	return m, textinput.Blink
}

// chooseBatchTarget is the picker's enter in batch flavour.
func (m model) chooseBatchTarget(t dropTarget) (tea.Model, tea.Cmd) {
	m.pickForBatch = false
	m.batch.target = t
	m.stage = stageBatchCompose
	m.sizeBatchCompose()
	m.batchSay("target: "+t.label, false)
	return m, m.setBatchFocus(m.batch.focus)
}

// beginBatchSession opens the ⚙ panel on the batch's own options. The panel
// edits m.formSession, so it is seeded with a clone and read back on the way
// out (see closeSession), the way the list-opened panel works.
func (m model) beginBatchSession() (tea.Model, tea.Cmd) {
	m.formSession = m.batch.session.clone()
	m.sessForBatch = true
	m.batch.name.Blur()
	m.batch.pick.input.Blur()
	return m.beginSession()
}

// closeBatchSession is the panel's way back to the composer.
func (m model) closeBatchSession() (tea.Model, tea.Cmd) {
	m.batch.session = m.formSession.clone()
	m.formSession = SessionOpts{}
	m.sessForBatch = false
	m.stage = stageBatchCompose
	m.sizeBatchCompose()
	return m, m.setBatchFocus(m.batch.focus)
}

// batchSessionNote is the settings block's aside about options: which picks
// the batch's options override (✱), or — for a combined drop, which has one
// session and so one setup — which picks bring options that will not apply.
func (m model) batchSessionNote() string {
	bc := m.batch
	n := 0
	for _, c := range bc.picked {
		if m.batchPickStar(c) {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	if bc.deliver == deliverCombined {
		return fmt.Sprintf("✱ %d with options of their own — one combined prompt uses only the batch's", n)
	}
	return fmt.Sprintf("✱ %d whose own options the batch overrides", n)
}

// batchPickStar says whether a pick wears ✱: its own options are overridden by
// the batch's, or (combined) will not apply at all.
func (m model) batchPickStar(c batchCand) bool {
	if !c.session.configured() {
		return false
	}
	if m.batch.deliver == deliverCombined {
		return true
	}
	return len(overriddenFields(c.session, &m.batch.session)) > 0
}

// --- Dropping -------------------------------------------------------------------

// batchDropWhy is why the batch cannot be dropped as it stands, or "".
func (m model) batchDropWhy() string {
	bc := m.batch
	switch {
	case len(bc.picked) == 0:
		return "pick at least one prompt first — space on a row on the left"
	case m.client == nil:
		return "cats control socket unavailable — can't drop into a session"
	case m.dropping:
		return "a drop is still in progress…"
	case !m.project.available() && !m.global.available():
		return noBacklogWhy
	case bc.deliver == deliverEach && bc.target.kind == targetExistingPane && len(bc.picked) > 1:
		// A running pane is one conversation. Handing it several prompts at
		// once is not "a session each", and typing them in one after another
		// without waiting would garble them — so say which two ways there are.
		return "all at once gives each prompt its own session — pick a new-session target, or deliver as one prompt, listed"
	}
	return ""
}

// dropBatch is ▶ Drop now: validate, turn picked Next List items into backlog
// prompts, record the batch, and start the chain (launchBatch).
func (m model) dropBatch() (tea.Model, tea.Cmd) {
	if why := m.batchDropWhy(); why != "" {
		m.batchSay(why, true)
		return m, nil
	}
	bc := m.batch
	b := Batch{
		ID:      newID(),
		Name:    strings.TrimSpace(bc.name.Value()),
		Created: time.Now(),
		Deliver: bc.deliver,
		Target:  batchTargetFrom(bc.target, m.ctx.projectDir()),
		Session: sessionPtr(bc.session),
	}
	anyProject := false
	for _, c := range bc.picked {
		ref := c.ref
		if c.next.ID != "" {
			var err error
			if ref, err = m.nextItemAsPrompt(c.next); err != nil {
				m.batchSay(c.next.ID+": "+err.Error(), true)
				return m, nil
			}
		}
		if ref.scope == scopeProject {
			anyProject = true
		}
		b.Items = append(b.Items, BatchItem{Scope: batchScopeName(ref.scope), ID: ref.id, Title: c.title})
	}
	// The record lives with the project whenever it touches the project: that
	// is where someone looking for what happened to these prompts will be.
	b.scope = scopeGlobal
	if (anyProject || !m.global.available()) && m.project.available() {
		b.scope = scopeProject
	}
	// A selection on the list that became this batch has done its job; left
	// standing it would make the next ctrl+k (or ctrl+o) act on prompts that
	// are now done.
	if bc.from == stageList && m.markCount() > 0 {
		m.clearMarks()
		m.rebuildList()
	}
	return m.launchBatch(b)
}

// nextItemAsPrompt is the backlog prompt a picked Next List item becomes: the
// open copy already in the backlog when there is one (the one ⤓ Add would
// refuse to duplicate), or a new one saved the way ⤓ Add saves it.
func (m *model) nextItemAsPrompt(it nextItem) (todoRef, error) {
	if ref, _, ok := m.nextBacklogCopy(it); ok {
		return ref, nil
	}
	return m.saveNextItem(it, annots{})
}

// --- Keys -----------------------------------------------------------------------

// setBatchFocus moves the keys to region f, and the text cursors with them: the
// Pick pane's query box holds them only while the Pick pane does, and the name
// box only on its own row.
func (m *model) setBatchFocus(f int) tea.Cmd {
	bc := &m.batch
	bc.focus = f
	bc.pick.input.Blur()
	bc.name.Blur()
	switch {
	case f == batchFocusPick:
		return bc.pick.input.Focus()
	case f == batchFocusSettings && bc.setRow == batchSetName:
		return bc.name.Focus()
	}
	return nil
}

// cycleBatchFocus is tab / shift+tab. The Batch and Settings regions are
// skipped over nothing: an empty batch still has settings worth reaching.
func (m *model) cycleBatchFocus(delta int) tea.Cmd {
	f := (m.batch.focus + delta + batchFocusCount) % batchFocusCount
	cmd := m.setBatchFocus(f)
	// The narrow layout shows one pane at a time, and which one follows the
	// focus; the Pick pane's window is sized for the pane it is in.
	m.sizeBatchCompose()
	return cmd
}

func (m model) updateBatchCompose(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	bc := &m.batch
	if bc.dragging {
		// A key ends a drag whose release went missing (the list's rule).
		bc.dragging = false
	}
	s := msg.String()
	switch s {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		// The list's esc ladder: out of the button row, then clear a query,
		// and only then leave — and leaving picks behind takes a second press.
		if bc.focus == batchFocusButtons {
			return m, m.setBatchFocus(batchFocusPick)
		}
		if bc.focus == batchFocusPick && strings.TrimSpace(bc.pick.input.Value()) != "" {
			bc.pick.input.SetValue("")
			bc.pick.filter()
			return m, nil
		}
		return m.cancelBatchCompose()
	case "tab":
		return m, m.cycleBatchFocus(1)
	case "shift+tab":
		return m, m.cycleBatchFocus(-1)
	case "shift+enter", "alt+enter":
		// The list's drop chord, meaning the same thing here: send it.
		return m.dropBatch()
	case "ctrl+r":
		// The form's chord for the ⚙ panel.
		return m.beginBatchSession()
	case "ctrl+g":
		// The list's chord for the Next List page, here the Pick pane's
		// flip between the two sources.
		src := batchSrcNext
		if bc.source == batchSrcNext {
			src = batchSrcBacklog
		}
		m.switchBatchSource(src)
		cmd := m.setBatchFocus(batchFocusPick)
		m.sizeBatchCompose()
		return m, cmd
	case "ctrl+k":
		return m.openBatchesFromCompose()
	}

	switch bc.focus {
	case batchFocusPick:
		switch s {
		case "up", "ctrl+p":
			bc.pick.moveUp()
			return m, nil
		case "down", "ctrl+n":
			bc.pick.moveDown()
			return m, nil
		case "space", " ", "enter":
			// Space is the checkbox key, so the filter cannot hold a space;
			// fuzzy matching makes one unnecessary. Enter does the same, since
			// "the thing in front of me" on a checkbox row is the checkbox.
			m.toggleBatchCand(bc.pick.selectedIndex())
			return m, nil
		case "ctrl+a":
			m.toggleBatchAll()
			return m, nil
		}
		m.batchSay("", false)
		return m, bc.pick.editQuery(msg)
	case batchFocusBatch:
		switch s {
		case "up", "ctrl+p":
			bc.cursor = max(bc.cursor-1, 0)
			m.ensureBatchVisible()
		case "down", "ctrl+n":
			bc.cursor = min(bc.cursor+1, max(len(bc.picked)-1, 0))
			m.ensureBatchVisible()
		case "alt+up", "ctrl+up":
			m.moveBatchPick(-1)
		case "alt+down", "ctrl+down":
			m.moveBatchPick(1)
		case "delete", "backspace", "ctrl+x", "x":
			m.removeBatchPick()
		case "s":
			m.sortBatchPicks()
		}
		return m, nil
	case batchFocusSettings:
		switch s {
		case "up", "ctrl+p":
			if bc.setRow > 0 {
				bc.setRow--
			}
			return m, m.setBatchFocus(batchFocusSettings)
		case "down", "ctrl+n":
			if bc.setRow < batchSetCount-1 {
				bc.setRow++
			}
			return m, m.setBatchFocus(batchFocusSettings)
		}
		switch bc.setRow {
		case batchSetName:
			if s == "enter" {
				bc.setRow++
				return m, m.setBatchFocus(batchFocusSettings)
			}
			var cmd tea.Cmd
			bc.name, cmd = bc.name.Update(msg)
			return m, cmd
		case batchSetDeliver:
			switch s {
			case "left":
				m.cycleBatchDeliver(-1)
			case "right", "space", " ", "enter":
				m.cycleBatchDeliver(1)
			}
		case batchSetTarget:
			if s == "enter" || s == "space" || s == " " {
				return m.beginBatchTarget()
			}
		case batchSetSession:
			if s == "enter" || s == "space" || s == " " {
				return m.beginBatchSession()
			}
		}
		return m, nil
	case batchFocusButtons:
		n := len(m.batchActions())
		switch s {
		case "left":
			bc.btn = (bc.btn + n - 1) % n
		case "right":
			bc.btn = (bc.btn + 1) % n
		case "enter", "space", " ":
			return m.pressBatchButton(bc.btn)
		}
		return m, nil
	}
	return m, nil
}

// pressBatchButton runs button i.
func (m model) pressBatchButton(i int) (tea.Model, tea.Cmd) {
	switch i {
	case batchBtnDrop:
		return m.dropBatch()
	case batchBtnSort:
		m.sortBatchPicks()
	case batchBtnBatches:
		return m.openBatchesFromCompose()
	case batchBtnCancel:
		return m.cancelBatchCompose()
	}
	return m, nil
}

// cancelBatchCompose is esc and ✕ Cancel: leave, but with picks on the table
// only on the second press — the first says what would be lost. A composer
// holding a dozen hand-ordered picks is minutes of choosing, and esc is also
// the key that clears a filter one press earlier; the two are close enough
// together on the hand that one of them should not silently cost the other.
func (m model) cancelBatchCompose() (tea.Model, tea.Cmd) {
	if len(m.batch.picked) > 0 && m.batch.note != batchLeaveWarn {
		m.batchSay(batchLeaveWarn, true)
		return m, nil
	}
	m.leaveBatchCompose()
	return m, nil
}

// openBatchesFromCompose is ☰ Batches: the page, abandoning the draft — said
// in the note first when there is a draft to lose, so a stray press costs one
// more press rather than the picks.
func (m model) openBatchesFromCompose() (tea.Model, tea.Cmd) {
	if len(m.batch.picked) > 0 && m.batch.note != batchLeaveWarn {
		m.batchSay(batchLeaveWarn, true)
		return m, nil
	}
	m.batch = batchComposer{}
	m.openBatchesPage()
	return m, nil
}

// batchLeaveWarn is the one-press guard on leaving a draft for the Batches page.
const batchLeaveWarn = "the picks here are not saved until the batch is dropped — press again to leave them"

// batchActions is the button row.
func (m model) batchActions() []listAction {
	return []listAction{
		{label: "▶ Drop now", hint: m.modEnter(), tint: colAccent},
		// No chord on the chip: s sorts only while the Batch pane has the keys
		// (anywhere else it is a letter for a query or a name), and the
		// footer says so there. A chip teaching "s" would be wrong two times
		// out of three.
		{label: "⇅ A→Z", tint: colCyan},
		{label: "☰ Batches", hint: "ctrl+k", tint: colSky},
		{label: "✕ Cancel", hint: "esc", tint: colStraw},
	}
}

// --- Layout -----------------------------------------------------------------------

// batchGeom is where everything in the composer is drawn, computed once for
// both the view and the pointer so the two cannot disagree about a row.
type batchGeom struct {
	split     bool
	showPick  bool
	showBatch bool
	pickX     int
	pickW     int
	batchX    int
	batchW    int
	paneTabsY int // the narrow layout's Pick · Batch switcher; -1 when split
	srcTabsY  int
	queryY    int
	allY      int
	pickRowsY int
	pickRows  int // lines available to the Pick pane's rows (headings included)
	headY     int
	batchRowY int
	batchRows int
	orderY    int
	setY      int
	barY      int
	noteY     int
	footY     int
}

// batchPaneSep is the rule between the two panes when they share the width.
const batchPaneSep = " │ "

// batchSettingsLines is what the Batch pane spends under its rows: a blank, the
// order line, a blank, the settings rows, and the note line.
const batchSettingsLines = 1 + 1 + 1 + batchSetCount + 1

func (m model) batchGeom() batchGeom {
	w, h := m.width, m.height
	if w <= 0 {
		w = 120
	}
	if h <= 0 {
		h = 30
	}
	g := batchGeom{split: w >= composerSplitMin, paneTabsY: -1}
	bodyY := 2
	if g.split {
		g.pickW = (w - lipgloss.Width(batchPaneSep)) * 45 / 100
		g.batchX = g.pickW + lipgloss.Width(batchPaneSep)
		g.batchW = w - g.batchX
		g.showPick, g.showBatch = true, true
	} else {
		g.paneTabsY = 2
		bodyY = 3
		g.pickW, g.batchW = w, w
		g.showPick = m.batch.focus == batchFocusPick
		g.showBatch = !g.showPick
	}
	// Under the body: a blank, the button row, the note, the footer.
	g.barY, g.noteY, g.footY = h-3, h-2, h-1
	bodyH := max(g.barY-1-bodyY, 6)

	g.srcTabsY, g.queryY, g.allY, g.pickRowsY = bodyY, bodyY+1, bodyY+2, bodyY+3
	g.pickRows = max(bodyH-3, 1)

	g.headY, g.batchRowY = bodyY, bodyY+1
	g.batchRows = max(bodyH-1-batchSettingsLines, 1)
	g.orderY = g.batchRowY + g.batchRows + 1
	g.setY = g.orderY + 2
	return g
}

// sizeBatchCompose fits the Pick pane's window and query box to the geometry.
func (m *model) sizeBatchCompose() {
	if m.stage != stageBatchCompose && m.stage != stageTarget && m.stage != stageSession {
		return
	}
	g := m.batchGeom()
	m.batch.pick.input.SetWidth(max(min(g.pickW-16, searchFieldWidth), 6))
	m.batch.name.SetWidth(max(min(g.batchW-14, 50), 8))
	m.batch.pick.setMaxRows(max(g.pickRows-m.batch.pick.separatorLines(), 1))
	m.ensureBatchVisible()
}

// --- View -------------------------------------------------------------------------

func (m model) viewBatchCompose() string {
	g := m.batchGeom()
	h := g.footY + 1
	lines := make([]string, h)
	lines[0] = m.titleLine("New batch")

	if !g.split {
		lines[g.paneTabsY] = m.batchPaneTabs()
	}
	var left, right []string
	if g.showPick {
		left = m.batchPickLines(g)
	}
	if g.showBatch {
		right = m.batchPaneLines(g)
	}
	top := g.srcTabsY
	for i := top; i < g.barY-1; i++ {
		var l, r string
		if k := i - top; k < len(left) {
			l = left[k]
		}
		if k := i - top; k < len(right) {
			r = right[k]
		}
		switch {
		case g.split:
			lines[i] = fitCells(l, g.pickW) + descStyle.Render(batchPaneSep) + ansi.Truncate(r, g.batchW, "…")
		case g.showPick:
			lines[i] = ansi.Truncate(l, g.pickW, "…")
		default:
			lines[i] = ansi.Truncate(r, g.batchW, "…")
		}
	}
	lines[g.barY] = m.batchBar()
	if bc := m.batch; bc.note != "" {
		st := okStyle
		if bc.noteErr {
			st = errStyle
		}
		lines[g.noteY] = st.Render(m.fitToPane("  "+bc.note, 0))
	}
	lines[g.footY] = footerStyle.Render(m.fitFooter(m.batchFooterSegs()))
	return strings.Join(lines, "\n")
}

// fitCells pads or cuts a rendered line to exactly w cells, so the rule
// between the panes stands in one column however long the left line was.
func fitCells(s string, w int) string {
	if lipgloss.Width(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	if pad := w - lipgloss.Width(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// batchPaneTabs is the narrow layout's switcher line.
func (m model) batchPaneTabs() string {
	pick, batch := descStyle, descStyle
	if m.batch.focus == batchFocusPick {
		pick = nameSelStyle
	} else {
		batch = nameSelStyle
	}
	return "  " + pick.Render("Pick") + descStyle.Render("  ·  ") +
		batch.Render(fmt.Sprintf("Batch (%d)", len(m.batch.picked))) + footerStyle.Render("   tab switches")
}

// batchSrcTabs is the Pick pane's source line, and where each tab's cells are
// (for the pointer).
func (m model) batchSrcTabs() (line string, spans [2][2]int) {
	labels := [2]string{"Backlog", "Next List"}
	x := 1
	var b strings.Builder
	b.WriteString(" ")
	for i, l := range labels {
		if i > 0 {
			b.WriteString(descStyle.Render(" │ "))
			x += 3
		}
		st := descStyle
		if m.batch.source == i {
			st = headerNameStyle.Underline(true)
		}
		b.WriteString(st.Render(l))
		spans[i] = [2]int{x, x + lipgloss.Width(l)}
		x += lipgloss.Width(l)
	}
	b.WriteString(footerStyle.Render("  ctrl+g"))
	return b.String(), spans
}

// batchPickLines draws the Pick pane, one string per line from srcTabsY.
func (m model) batchPickLines(g batchGeom) []string {
	bc := m.batch
	tabs, _ := m.batchSrcTabs()
	out := []string{tabs}

	field := searchFieldStyle
	if bc.focus == batchFocusPick {
		field = searchFieldOnStyle
	}
	matched, total := bc.pick.counts()
	out = append(out, field.Render(promptStyle.Render("🔍 ")+bc.pick.input.View())+"  "+
		countStyle.Render(fmt.Sprintf("%d/%d", matched, total)))

	picked, pickable := bc.pickCounts()
	box := "☐"
	switch {
	case pickable > 0 && picked == pickable:
		box = "☑"
	case picked > 0:
		box = "☒"
	}
	out = append(out, "  "+markStyle.Render(box)+descStyle.Render(fmt.Sprintf(" all · %d of %d picked · ctrl+a", picked, pickable)))

	rows := strings.TrimRight(bc.pick.rowsView(m.pickEmptyMsg(), g.pickW), "\n")
	if bc.focus != batchFocusPick {
		// The highlight is where the keys will land when the pane gets them
		// back; drawn at full strength while they are elsewhere it would claim
		// them. So the unfocused pane draws its rows without one.
		l := bc.pick
		l.cursor = -1
		rows = strings.TrimRight(l.rowsView(m.pickEmptyMsg(), g.pickW), "\n")
	}
	out = append(out, strings.Split(rows, "\n")...)
	return out
}

// batchPaneLines draws the Batch pane and its settings, from headY.
func (m model) batchPaneLines(g batchGeom) []string {
	bc := m.batch
	var out []string
	head := "Batch"
	if n := strings.TrimSpace(bc.name.Value()); n != "" {
		head += " · " + n
	}
	out = append(out, headingStyle.Render(head)+descStyle.Render(fmt.Sprintf("  %d prompt%s", len(bc.picked), plural(len(bc.picked)))))

	focused := bc.focus == batchFocusBatch
	for i := 0; i < g.batchRows; i++ {
		k := bc.top + i
		if k >= len(bc.picked) {
			if len(bc.picked) == 0 && i == 0 {
				out = append(out, descStyle.Render("  nothing picked yet — space on a row on the left"))
				continue
			}
			out = append(out, "")
			continue
		}
		out = append(out, m.batchPickRow(k, focused && k == bc.cursor, g.batchW))
	}
	out = append(out, "")
	order := "order: manual · s sorts A→Z · drag or alt+↑/↓ to move"
	if bc.sorted {
		order = "order: A→Z · drag or alt+↑/↓ to adjust"
	}
	out = append(out, descStyle.Render("  "+order), "")

	for row := range batchSetCount {
		out = append(out, m.batchSettingLine(row))
	}
	out = append(out, descStyle.Render("  "+m.batchSessionNote()))
	return out
}

// batchPickRow is one row of the Batch pane.
func (m model) batchPickRow(k int, selected bool, width int) string {
	bc := m.batch
	c := bc.picked[k]
	var r strings.Builder
	switch {
	case selected && bc.dragging:
		r.WriteString(onRow(cursorStyle, true).Render(grabGlyph))
	case selected:
		r.WriteString(onRow(cursorStyle, true).Render(cursorGlyph))
	default:
		r.WriteString(footerStyle.Render("⠿ "))
	}
	r.WriteString(onRow(countStyle, selected).Render(fmt.Sprintf("%2d. ", k+1)))
	var tail strings.Builder
	for _, mk := range c.marks {
		tail.WriteString(" " + mk.text)
	}
	if c.session.configured() {
		tail.WriteString(" ⚙")
	}
	if m.batchPickStar(c) {
		tail.WriteString(" ✱")
	}
	if c.next.ID != "" {
		tail.WriteString(" ✚")
	}
	room := max(width-lipgloss.Width(r.String())-lipgloss.Width(tail.String())-1, 8)
	st := nameStyle
	if selected {
		st = nameSelStyle
	}
	r.WriteString(onRow(st, selected).Render(truncate(c.title, room)))
	r.WriteString(onRow(descStyle, selected).Render(tail.String()))
	row := r.String()
	if pad := width - lipgloss.Width(row); selected && pad > 0 {
		row += onRow(lipgloss.NewStyle(), true).Render(strings.Repeat(" ", pad))
	}
	return row
}

// batchSetLabelWidth is the settings' label column.
const batchSetLabelWidth = 9

// batchSettingLine draws settings row `row`.
func (m model) batchSettingLine(row int) string {
	bc := m.batch
	on := bc.focus == batchFocusSettings && bc.setRow == row
	var b strings.Builder
	if on {
		b.WriteString(cursorStyle.Render(cursorGlyph))
	} else {
		b.WriteString("  ")
	}
	labels := [batchSetCount]string{"Name", "Deliver", "Target", "Session"}
	b.WriteString(nameStyle.Render(fmt.Sprintf("%-*s", batchSetLabelWidth, labels[row])))
	val := nameStyle
	if on {
		val = nameSelStyle
	}
	switch row {
	case batchSetName:
		if on {
			b.WriteString(bc.name.View())
		} else if n := strings.TrimSpace(bc.name.Value()); n != "" {
			b.WriteString(val.Render(n))
		} else {
			b.WriteString(descStyle.Render("optional — defaults to the first prompt's title"))
		}
	case batchSetDeliver:
		for i, d := range batchDeliverModes {
			if i > 0 {
				b.WriteString("  ")
			}
			radio := "( ) "
			st := descStyle
			if bc.deliver == d {
				radio, st = "(•) ", val
			}
			b.WriteString(st.Render(radio + deliverLabel(d)))
		}
	case batchSetTarget:
		b.WriteString(val.Render(firstNonEmpty(bc.target.label, targetDesc(bc.target))))
		if on {
			b.WriteString(descStyle.Render("  enter to change"))
		}
	case batchSetSession:
		b.WriteString(val.Render("⚙ " + firstNonEmpty(bc.session.summary(), "each prompt's own")))
		if on {
			b.WriteString(descStyle.Render("  enter or ctrl+r to edit"))
		}
	}
	return b.String()
}

// batchBarTier is how much of each chip the button row prints.
func (m model) batchBarTier() chipTier {
	return barTier(m.batchActions(), m.width, indentWidth)
}

// batchChips lays the button row out (actionChips' arithmetic).
func (m model) batchChips() []actionChip {
	tier := m.batchBarTier()
	var chips []actionChip
	x := indentWidth
	for i, a := range m.batchActions() {
		if i > 0 {
			x += chipGap(tier)
		}
		text := a.chipText(tier)
		w := lipgloss.Width(btnStyle.Render(text))
		chips = append(chips, actionChip{text: text, start: x, end: x + w})
		x += w
	}
	return chips
}

// batchBar renders the button row. ▶ Drop now is greyed while the batch
// cannot be dropped — the chip answers "why did nothing happen" before it is
// pressed, and the note answers it after.
func (m model) batchBar() string {
	acts := m.batchActions()
	tier := m.batchBarTier()
	gap := strings.Repeat(" ", chipGap(tier))
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", indentWidth))
	for i, c := range m.batchChips() {
		if i > 0 {
			b.WriteString(gap)
		}
		st, hintFg := btnStyle.Foreground(lipgloss.Color(acts[i].tint)), colDim
		if i == batchBtnDrop && m.batchDropWhy() != "" {
			st, hintFg = btnOffStyle, colFaint
		}
		if m.batch.focus == batchFocusButtons && m.batch.btn == i {
			st, hintFg = btnFocusStyle, ""
		}
		b.WriteString(renderChipDimHint(st, hintFg, acts[i], tier, c.text))
	}
	return b.String()
}

// batchFooterSegs are the footer's keys for the region holding the focus.
func (m model) batchFooterSegs() []string {
	var segs []string
	switch m.batch.focus {
	case batchFocusPick:
		segs = []string{"space pick", "ctrl+a all", "type to filter", "ctrl+g backlog/next list"}
	case batchFocusBatch:
		segs = []string{"↑/↓ choose", "alt+↑/↓ move", "x remove", "s sort A→Z", "drag to reorder"}
	case batchFocusSettings:
		segs = []string{"↑/↓ row", "←/→ change", "enter open", "ctrl+r session"}
	case batchFocusButtons:
		segs = []string{"←/→ choose", "enter press"}
	}
	segs = append(segs, "tab next", m.modEnter()+" drop", "esc back")
	return segs
}

// plural is the "s" a count takes.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// --- Mouse ------------------------------------------------------------------------

// clickBatchCompose is a press in the composer. Every region answers the way
// its keys do: a row in the Pick pane toggles its checkbox (a checkbox row's
// click is the check — there is nothing else a press on one could mean), a row
// in the Batch pane is highlighted and taken hold of for a drag, a settings
// row is focused and — for the two that open something — opened.
func (m model) clickBatchCompose(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	g := m.batchGeom()
	x, y := msg.X, msg.Y
	bc := &m.batch

	if y == g.barY {
		for i, c := range m.batchChips() {
			if x >= c.start && x < c.end {
				bc.btn = i
				cmd := m.setBatchFocus(batchFocusButtons)
				next, c2 := m.pressBatchButton(i)
				return next, tea.Batch(cmd, c2)
			}
		}
		return m, nil
	}
	if !g.split && y == g.paneTabsY {
		f := batchFocusPick
		if x >= 10 {
			f = batchFocusBatch
		}
		cmd := m.setBatchFocus(f)
		m.sizeBatchCompose()
		return m, cmd
	}

	inPick := g.showPick && (!g.split || x < g.pickW)
	inBatch := g.showBatch && (!g.split || x >= g.batchX)

	switch {
	case inPick:
		cmd := m.setBatchFocus(batchFocusPick)
		switch {
		case y == g.srcTabsY:
			_, spans := m.batchSrcTabs()
			for i, sp := range spans {
				if x >= sp[0] && x < sp[1] {
					m.switchBatchSource(i)
				}
			}
		case y == g.allY:
			m.toggleBatchAll()
		case y >= g.pickRowsY:
			if i, ok := bc.pick.rowAtLine(y - g.pickRowsY); ok && bc.pick.focusRow(i) {
				m.toggleBatchCand(bc.pick.selectedIndex())
			}
		}
		return m, cmd
	case inBatch:
		switch {
		case y >= g.batchRowY && y < g.batchRowY+g.batchRows:
			k := bc.top + y - g.batchRowY
			cmd := m.setBatchFocus(batchFocusBatch)
			if k < len(bc.picked) {
				bc.cursor = k
				bc.dragging, bc.dragKey, bc.dragMoved = true, bc.picked[k].key(), false
			}
			return m, cmd
		case y >= g.setY && y < g.setY+batchSetCount:
			bc.setRow = y - g.setY
			cmd := m.setBatchFocus(batchFocusSettings)
			switch bc.setRow {
			case batchSetDeliver:
				// The click picks the radio it landed on: the first option's
				// text is the left half of the value column, the second the
				// right — measured, since the labels are the words drawn.
				col := x - g.batchX - indentWidth - batchSetLabelWidth
				first := lipgloss.Width("(•) " + deliverLabel(batchDeliverModes[0]))
				if col < first {
					bc.deliver = batchDeliverModes[0]
				} else {
					bc.deliver = batchDeliverModes[1]
				}
			case batchSetTarget:
				return m.beginBatchTarget()
			case batchSetSession:
				return m.beginBatchSession()
			}
			return m, cmd
		}
	}
	return m, nil
}

// batchDragOver moves the held pick to the Batch row under the pointer.
func (m model) batchDragOver(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	bc := &m.batch
	if m.stage != stageBatchCompose {
		bc.dragging = false
		return m, nil
	}
	g := m.batchGeom()
	if msg.Y < g.batchRowY || msg.Y >= g.batchRowY+g.batchRows {
		return m, nil
	}
	to := min(bc.top+msg.Y-g.batchRowY, len(bc.picked)-1)
	from := slices.IndexFunc(bc.picked, func(c batchCand) bool { return c.key() == bc.dragKey })
	if from < 0 || to < 0 || from == to {
		return m, nil
	}
	c := bc.picked[from]
	bc.picked = slices.Delete(bc.picked, from, from+1)
	bc.picked = slices.Insert(bc.picked, to, c)
	bc.cursor = to
	bc.sorted = false
	bc.dragMoved = true
	return m, nil
}

// endBatchDrag lets go of the held pick.
func (m model) endBatchDrag() (tea.Model, tea.Cmd) {
	moved := m.batch.dragMoved
	m.batch.dragging, m.batch.dragMoved = false, false
	if moved {
		m.batchSay("moved", false)
	}
	return m, nil
}

// errNoBacklog is noBacklogWhy as an error, for the paths that return one.
var errNoBacklog = errors.New(noBacklogWhy)
