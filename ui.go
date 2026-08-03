package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/rohanthewiz/cats-todo/internal/app"
)

// uiStage is which screen the manager is currently showing.
type uiStage int

const (
	stageList     uiStage = iota // the project+global todo list
	stageForm                    // add / edit a prompt
	stageConfirm                 // confirm a delete / clear-completed
	stageTarget                  // pick where to drop the chosen prompt
	stageView                    // read-only view of a prompt's full body
	stageImages                  // attach / detach the form's images
	stageSchedule                // set (or clear) a prompt's auto-drop time
)

// confirmKind distinguishes what the confirm stage is about to do.
type confirmKind int

const (
	confirmDelete    confirmKind = iota // delete the selected todo
	confirmClearDone                    // remove every done todo in both scopes
)

// formMode distinguishes adding a new todo from editing an existing one.
type formMode int

const (
	formAdd formMode = iota
	formEdit
)

// formImage is one row of the form's attachment editor. An entry is either
// already attached (rel — the path recorded on the todo) or pending (src — an
// absolute source path not yet copied into the backlog): an add has no todo id
// to copy under until it saves, so everything it collects stays pending until
// then. Both kinds are edited the same way; only saveForm cares which is which.
type formImage struct {
	rel     string // backlog-relative path of an attachment already on the todo
	src     string // absolute source path of one not yet copied in
	name    string // basename, for display
	missing bool   // an existing attachment whose file has gone
	// pasted marks a pending source that came off the clipboard rather than
	// from a path the user can see. Its src is a temp file this process wrote,
	// so the row says "pasted" instead of showing plumbing.
	pasted bool
}

// dropTargetKind is the two ways a prompt can land: into a brand-new Claude Code
// session, or into an already-running agent pane.
type dropTargetKind int

const (
	targetNewSession dropTargetKind = iota
	targetExistingPane
)

// dropTarget is one selectable destination in the target picker.
type dropTarget struct {
	kind    dropTargetKind
	pane    uint32 // for targetExistingPane
	agent   string // detected agent label, for existing panes
	command string // launch command, for targetNewSession ("claude", "codex", …)
	label   string
	desc    string
}

// dropMode is the per-drop submit choice.
type dropMode int

const (
	dropPaste dropMode = iota // type the prompt but don't press Enter
	dropRun                   // type the prompt and submit it
)

// todoRef identifies a todo by its scope and id, so the list can map a row back
// to the right store.
type todoRef struct {
	scope scope
	id    string
}

// pendingAction is the drop the user chose. The manager performs it without
// quitting — off the UI thread (see performDropCmd) — so the pane persists and
// you can drop more prompts in the same session.
type pendingAction struct {
	todo   Todo
	target dropTarget
	mode   dropMode
	// cwd roots a new-session drop's tab: the project directory the todo is
	// scoped to, so the agent opens on the same tree the prompt was written
	// about. Captured here at dispatch rather than read inside performDrop so the
	// drop goroutine needs nothing from the model (which it does not own a
	// reference to). Only targetNewSession uses it; a drop into an existing pane
	// inherits that pane's directory.
	cwd string
	// images are the todo's attachments as absolute paths, resolved against
	// their store at dispatch for the same reason as cwd — the drop goroutine
	// holds no store to resolve them itself. Already filtered to files that
	// exist (see store.imagePaths).
	images []string
}

// dropResultMsg reports the outcome of an asynchronous drop back to the Update
// loop, so the manager can clear its "dropping…" state and show where the prompt
// landed (or why it failed) while staying open.
type dropResultMsg struct {
	desc string  // human description of the destination, for the status line
	ref  todoRef // the dropped todo, so a successful drop can auto-mark it done
	err  error
	// sched is set when this drop was a schedule firing rather than a
	// keystroke. The claim already cleared the schedule from disk, so on
	// failure this copy is what gets written back — as Missed, keeping the
	// failure on the row instead of vanishing with the status line.
	sched *Schedule
}

// scheduleTickMsg is the schedule loop's heartbeat (see scheduleTick).
type scheduleTickMsg time.Time

// scheduleTick arms the next schedule check, one second out. The tick runs
// unconditionally for the life of the program: no arm/disarm state machine
// means no stale-generation or double-loop bugs, each tick compares wall
// clock against fire times so drift can't accumulate, and a once-a-second
// no-op message costs nothing worth engineering away.
func scheduleTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return scheduleTickMsg(t)
	})
}

// model is the Bubble Tea state for the whole manager: a small stage machine
// over the list, the add/edit form, the delete confirm, and the target picker.
type model struct {
	ctx     RunContext
	project *store
	global  *store
	client  *catsClient // nil when the cats control socket is unavailable

	stage uiStage

	// List stage.
	list     fuzzyList
	rows     []todoRef // selectable row index -> todo
	hideDone bool      // fold completed todos out of the list
	// Action bar. actionFocus says whether tab has walked the focus out of the
	// query box and onto a button; actionIdx is which one. Focus is a bool
	// rather than a -1 sentinel in actionIdx so the zero-value model starts
	// where every launch should — typing into the filter.
	actionFocus bool
	actionIdx   int
	// The last row a click landed on, and when — the whole of double-click
	// detection, since the terminal reports every click as a first one.
	lastClickRow int
	lastClickAt  time.Time

	// Form stage.
	formMode   formMode
	formScope  scope
	editID     string
	titleInput textinput.Model
	promptArea textarea.Model
	formFocus  int // 0 = title, 1 = prompt
	formErr    string

	// Attachment editor (a sub-stage of the form, so its state lives and dies
	// with the form's).
	formImages []formImage
	// formImagesOrig is the attachment list the form opened with, so a save can
	// tell which of the todo's files the edit dropped. The form's own list is
	// not enough: it records what survived, not what was there.
	formImagesOrig []string
	imgInput       textinput.Model
	imgCursor      int
	// imgStatus is the editor's one message line, error or not — the same split
	// as the list's status/statusErr, since "removed shot.png" and "that is not
	// an image" must not look alike.
	imgStatus    string
	imgStatusErr bool
	// clipboardDirs are the temp directories holding clipboard captures queued
	// in this form. They are removed once the images have been copied into a
	// backlog, or when the form is abandoned — the capture is the one pending
	// source with a file of its own to answer for.
	clipboardDirs []string
	// recent is the lazily-scanned recent-image list that ctrl+r cycles, with
	// the next index to offer. Scanned once per visit to the editor — a
	// screenshot taken while it is open is the rare case, and rescanning on
	// every keypress to catch it is not worth the directory walk.
	recent    []string
	recentIdx int

	// Confirm stage.
	confirmKind       confirmKind
	pendingDelete     todoRef
	pendingTitle      string
	pendingClearCount int

	// Target stage.
	dropTodo   todoRef
	targets    []dropTarget
	targetList fuzzyList
	// pickForSchedule flips the picker's meaning: enter stores the choice on
	// the todo (commitSchedule) instead of firing a drop. Set only by the
	// schedule stage handing over to the picker; reset by backToList so an
	// esc out of the picker can never leave the next manual drop scheduling.
	pickForSchedule bool

	// Schedule stage.
	schedRef   todoRef
	schedInput textinput.Model
	schedErr   string
	schedAt    time.Time // the parsed fire time, carried into the picker

	// View stage.
	viewRef todoRef
	viewVP  viewport.Model

	width, height int

	status    string // transient message under the list
	statusErr bool

	// kbEnhanced records that the terminal answered bubbletea's keyboard
	// enhancement request (kitty protocol) — the only way shift+enter arrives
	// as its own key rather than a bare CR. It decides which of the two
	// equivalent modifier bindings the footers advertise; both stay live either
	// way. See modEnter.
	kbEnhanced bool

	dropping bool // a drop is in flight (off the UI thread); guards re-entry
	quitting bool
}

// modEnter names the modifier+enter chord for the footers: the drop key in the
// list and view, the newline key in the form. shift+enter is the one to show
// when the terminal can actually send it; alt+enter is the legacy fallback
// every terminal encodes as ESC CR, so it leads until the terminal says
// otherwise.
func (m model) modEnter() string {
	if m.kbEnhanced {
		return "shift+enter"
	}
	return "alt+enter"
}

// newModel builds the initial manager state showing the todo list.
func newModel(ctx RunContext, project, global *store, client *catsClient) model {
	m := model{ctx: ctx, project: project, global: global, client: client, stage: stageList}
	m.list = newFuzzyList("Type to filter prompts…", nil)
	m.rebuildList()
	return m
}

// Init starts the cursor blinking and the schedule loop ticking. The pane
// title and alt screen are properties of the view in bubbletea v2 (see View),
// not one-shot startup commands.
func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, scheduleTick())
}

// Update routes by stage; non-key messages flow to whatever input is active so
// cursors keep blinking and text keeps flowing.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.applySizes()
		return m, nil
	case scheduleTickMsg:
		// Handled above the stage switch, so schedules fire whatever screen is
		// up; the status lands when the user is next on the list. The next
		// tick is armed in the same breath — the loop must survive every path
		// out of fireDueSchedules.
		return m, tea.Batch(m.fireDueSchedules(time.Time(msg)), scheduleTick())
	case dropResultMsg:
		m.dropping = false
		if msg.err != nil {
			if msg.sched != nil {
				// The claim took the schedule off disk before the fire; put
				// this copy back as Missed so the failure lives on the row,
				// not just in a status line the next keystroke replaces.
				sc := *msg.sched
				sc.Missed = true
				_ = m.storeFor(msg.ref.scope).setSchedule(msg.ref.id, &sc)
				m.rebuildList()
				m.setStatus("scheduled drop failed: "+msg.err.Error(), true)
				return m, nil
			}
			m.setStatus("drop failed: "+msg.err.Error(), true)
			return m, nil
		}
		status := "dropped → " + msg.desc
		// Handing a prompt to an agent is what "done" means here, so any successful
		// drop closes the todo out — paste drops included. The prompt now lives in
		// the agent's input where the user can see it; leaving a duplicate open in
		// the backlog only invites dropping the same work twice. Reopening is a
		// single keystroke if the paste gets discarded.
		//
		// setDone is idempotent and best effort — a save failure shouldn't undo a
		// successful drop, so we just skip the "marked done" note.
		if err := m.storeFor(msg.ref.scope).setDone(msg.ref.id, true); err == nil {
			status += " · marked done"
			m.rebuildList()
		}
		m.setStatus(status, false)
		return m, nil
	case tea.KeyboardEnhancementsMsg:
		// The terminal accepted the kitty keyboard request, so shift+enter is a
		// distinct key here and the footers can name it.
		m.kbEnhanced = true
		return m, nil
	case tea.MouseClickMsg:
		return m.updateMouse(msg)
	case tea.KeyPressMsg:
		switch m.stage {
		case stageList:
			return m.updateList(msg)
		case stageForm:
			return m.updateForm(msg)
		case stageConfirm:
			return m.updateConfirm(msg)
		case stageTarget:
			return m.updateTarget(msg)
		case stageView:
			return m.updateView(msg)
		case stageImages:
			return m.updateImages(msg)
		case stageSchedule:
			return m.updateSchedule(msg)
		}
	}
	return m.forward(msg)
}

// forward passes a non-key message to the active input.
func (m model) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.stage {
	case stageList:
		cmd = m.list.editQuery(msg)
	case stageTarget:
		cmd = m.targetList.editQuery(msg)
	case stageForm:
		return m.forwardForm(msg)
	case stageImages:
		m.imgInput, cmd = m.imgInput.Update(msg)
	case stageSchedule:
		m.schedInput, cmd = m.schedInput.Update(msg)
	}
	return m, cmd
}

// --- List stage ---------------------------------------------------------------

func (m model) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		// Step back out of the action bar first, then clear an active filter;
		// quit only when there's nothing left to back out of. Esc is the "undo
		// the state I'm in" key, and quitting the manager is the last resort.
		if m.actionFocus {
			return m, m.setActionFocus(false)
		}
		if strings.TrimSpace(m.list.input.Value()) != "" {
			m.list.input.SetValue("")
			m.list.filter()
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
	case "tab":
		return m, m.moveActionFocus(1)
	case "shift+tab":
		return m, m.moveActionFocus(-1)
	case "left", "right":
		// Only while a button holds the focus; otherwise these belong to the
		// query box's cursor, which is where they fall through to.
		if m.actionFocus {
			delta := 1
			if msg.String() == "left" {
				delta = -1
			}
			n := len(m.listActions())
			m.actionIdx = (m.actionIdx + delta + n) % n
			return m, nil
		}
	case "up", "ctrl+p":
		// The row highlight keeps moving while a button is focused: pick the
		// prompt, then press the button that acts on it.
		m.list.moveUp()
		return m, nil
	case "down", "ctrl+n":
		m.list.moveDown()
		return m, nil
	case "enter":
		// On a button, enter presses it. Otherwise enter is the "open what's in
		// front of me" key: the highlighted todo into the edit form, or — with
		// nothing to open, whether the backlog is empty or the filter matched
		// nothing — straight into a new entry. Dropping, the one action that
		// leaves the manager and touches another pane, is deliberately not on a
		// bare keystroke.
		if m.actionFocus {
			return m.runAction(m.actionIdx)
		}
		if _, ok := m.selectedRef(); ok {
			return m.beginEdit()
		}
		return m.beginAdd()
	case "shift+enter", "alt+enter":
		// Two spellings of one chord: shift+enter is what the user presses when
		// the terminal speaks the kitty protocol, alt+enter is the legacy
		// encoding every terminal manages. Both are bound always so the binding
		// never depends on what the terminal negotiated.
		return m.beginDrop()
	case "ctrl+a":
		return m.beginAdd()
	case "ctrl+e":
		return m.beginEdit()
	case "ctrl+x":
		return m.beginDelete()
	case "ctrl+t":
		return m.toggleSelected()
	case "ctrl+v":
		return m.beginView()
	case "ctrl+d":
		m.hideDone = !m.hideDone
		m.rebuildList()
		if m.hideDone {
			m.setStatus("hiding completed prompts", false)
		} else {
			m.setStatus("showing completed prompts", false)
		}
		return m, nil
	case "ctrl+s":
		return m.beginSchedule()
	case "ctrl+w":
		return m.beginClearDone()
	case "ctrl+up":
		return m.moveSelected(-1)
	case "ctrl+down":
		return m.moveSelected(1)
	}
	// Anything else is text for the filter — so the focus goes back with it.
	// Leaving a button lit while the characters land in the query box would
	// show the focus in one place and put it in another.
	//
	// The focus goes back first: a blurred input drops the keys it is given, so
	// the character that brought the focus home would be the one character
	// swallowed.
	blink := m.setActionFocus(false)
	return m, tea.Batch(blink, m.list.editQuery(msg))
}

// updateMouse routes a click to whatever the stage draws as clickable: the
// action bar on the list, a target row in the drop picker. Everything else
// ignores the pointer.
//
// A click always moves the keyboard's place onto what was clicked before acting,
// so the two ways in leave the manager in the same state — the pointer puts the
// focus where the eye already is, rather than acting from one place while tab or
// ↑/↓ resumes from another.
func (m model) updateMouse(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	switch m.stage {
	case stageList:
		switch msg.Y {
		case headerRow:
			// The query box lives on the header line now; a click anywhere on
			// it hands the keys back to the box — the one thing a click up
			// there could mean, and idempotent when they're already home.
			return m, m.setActionFocus(false)
		case actionBarRow:
			return m.clickActionBar(msg)
		}
		return m.clickRow(msg)
	case stageTarget:
		return m.clickTarget(msg)
	}
	return m, nil
}

// clickRow moves the highlight to the todo row under the pointer, and opens the
// prompt for editing on the second click.
//
// A single click deliberately only selects. The list's rows and its buttons
// share a screen: the bar acts on whatever is highlighted, so a click that ran
// anything outright would make "click the prompt, then click Send" — the
// plainest thing the pointer is for — impossible to express.
//
// The second click opens the edit form, which is what a double-click means
// everywhere else a pointer is used: open the thing under it. It is also the
// gesture's safest reading. Opening a form is undone by esc, and the one action
// that reaches out of this program — handing a prompt to a live agent — stays
// off a gesture the hand can make by accident, on the bar's ➜ Send chip and
// shift+enter. Sending with the pointer is two clicks in two places, and that
// deliberateness is the point.
//
// Selecting also hands the bar's focus back to the list, so the pointer and the
// keyboard agree on what the next enter acts on.
func (m model) clickRow(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	i, ok := m.list.rowAtLine(msg.Y - listRowsRow)
	if !ok || !m.list.focusRow(i) {
		return m, nil
	}
	blink := m.setActionFocus(false)
	if i == m.lastClickRow && time.Since(m.lastClickAt) < doubleClickWindow {
		m.lastClickAt = time.Time{} // a third click starts over, not another open
		return m.beginEdit()
	}
	m.lastClickRow, m.lastClickAt = i, time.Now()
	return m, blink
}

// doubleClickWindow is how close together two clicks on one row have to be to
// count as a double. Terminals report clicks one at a time with no count, so the
// pairing is ours to do.
const doubleClickWindow = 500 * time.Millisecond

// clickActionBar presses the button under the pointer, if it is on one.
func (m model) clickActionBar(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	for i, c := range m.actionChips() {
		if msg.X >= c.start && msg.X < c.end {
			m.actionIdx = i
			m.setActionFocus(true) // blurs the box; nothing to restart
			return m.runAction(i)
		}
	}
	// The gaps between chips and the rest of the row: not a button, so nothing
	// to run — and the focus stays where it was rather than being cleared by a
	// miss.
	return m, nil
}

// clickTarget picks the agent whose row was clicked and drops into it, which is
// what enter on that row does: the prompt is pasted into the agent's input and
// left unsubmitted. Clicking a target is choosing it — the same bargain the
// action bar makes, where a click presses the button rather than merely lighting
// it. The whole row answers, not just the text on it: the row is the target, and
// asking the user to land on the label would make the pointer worse than the
// keyboard it is there to replace.
//
// Drop & run stays on the modifier chord. Nothing on this screen submits a
// prompt to an agent on one click.
func (m model) clickTarget(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	i, ok := m.targetList.rowAtLine(msg.Y - targetRowsRow)
	if !ok || !m.targetList.focusRow(i) {
		return m, nil
	}
	return m.chooseTarget(dropPaste)
}

// listAction is one button in the action bar under the filter. Every button
// mirrors a key binding rather than replacing it: hint is the chord it stands
// for, printed on the chip so the bar teaches the keyboard path instead of
// competing with it. needsSel marks the actions that have nothing to act on
// until a prompt is highlighted.
//
// tint is the chip's own color: its letters ordinarily, and the whole field it
// lights up with under the focus. One color serving both is what keeps a
// focused button recognizably the same button — the chip doesn't change hue
// when pressed, it just trades which half of itself is carrying it.
type listAction struct {
	label    string
	hint     string
	tint     string
	needsSel bool
}

// Indexes into listActions — used by runAction to name what it is dispatching.
const (
	actionAdd = iota
	actionEdit
	actionSend
	actionDelete
)

// listActions is the action bar's contents, in order. Send's hint follows
// modEnter because which spelling of the drop chord the terminal can send is
// only known once it answers the keyboard-enhancement request.
//
// Every icon is a one-cell text dingbat. The emoji forms (✏️ ➡️ ❌) were bigger
// and were drawn clipped: lipgloss measures them as two columns, which is what
// Unicode says, but the emoji font draws a glyph the terminal then cuts at a
// cell edge — half an arrow, half a cross. There is no width the layout can
// reserve to fix that, because the clipping happens in the terminal's own
// rasteriser. Dingbats are drawn by the text font at the size the pane is set
// to, whole every time, and they take the chip's foreground, so the bar stays
// inside the green instead of dropping four emoji palettes into it.
func (m model) listActions() []listAction {
	// The row runs cool to hot, left to right, in the order a prompt's life
	// actually goes: make it, change it, send it, throw it away. Green is spent
	// on Send rather than Add because it is the palette's color of consequence,
	// and handing a prompt to a live agent is the one thing on this screen that
	// reaches out of the program; Delete's red is the only warning the bar gives
	// before the confirm asks.
	return []listAction{
		{label: "✚ Add", hint: "ctrl+a", tint: colInfo},
		{label: "✎ Edit", hint: "enter", tint: colStraw, needsSel: true},
		{label: "➜ Send", hint: m.modEnter(), tint: colAccent, needsSel: true},
		{label: "✖ Delete", hint: "ctrl+x", tint: colErr, needsSel: true},
	}
}

// setActionFocus moves the focus between the query box and the buttons, and
// takes the query box's cursor with it.
//
// The cursor has to move, not just the flag. The box was focused once at
// construction and never blurred, so it blinked a cursor the whole time a
// button was lit — the box looked like it held the keys when it didn't, which
// is what made handing the focus back invisible: nothing about the box changed,
// because it had never stopped claiming to be focused.
//
// The returned command restarts the blink. Dropping it costs only the blink —
// a focused box still draws a steady cursor — so callers with nothing to return
// it through can ignore it.
func (m *model) setActionFocus(on bool) tea.Cmd {
	m.actionFocus = on
	if on {
		m.list.input.Blur()
		return nil
	}
	return m.list.input.Focus()
}

// moveActionFocus walks the focus one stop around the ring of query box and
// buttons: tab out of the filter, across the buttons, and back into the filter.
func (m *model) moveActionFocus(delta int) tea.Cmd {
	n := len(m.listActions())
	i := m.actionIdx
	if !m.actionFocus {
		i = -1 // the query box, the ring's first stop
	}
	switch i += delta; {
	case i < -1:
		i = n - 1
	case i >= n:
		i = -1
	}
	if i >= 0 {
		m.actionIdx = i
	}
	return m.setActionFocus(i >= 0)
}

// backToList returns to the list stage and hands the action bar's focus back to
// the query box.
//
// The focus has to be given back, not just left where it was: every stage is
// entered by running an action, and once that action is over the lit chip is
// stale. Left parked on Add — where a click on the Add chip, or a single tab,
// puts it — the next bare enter pressed that button again, so enter on a
// highlighted prompt opened a blank new todo instead of editing the one in
// front of the user. The query box is where the list's keys belong once nothing
// is mid-action.
//
// The blink command is dropped here rather than threaded back through ten
// callers: a stage change redraws the whole screen, which is signal enough on
// its own, and the box still shows a steady cursor.
func (m *model) backToList() {
	m.stage = stageList
	// Every path back through here clears the picker's schedule flavor: the
	// flag may only live for one list → schedule → picker traversal, or an
	// esc out of the picker would leave the next manual drop scheduling.
	m.pickForSchedule = false
	_ = m.setActionFocus(false)
}

// runAction presses button i. A button whose action needs a highlighted prompt
// says so rather than doing nothing: the underlying begin* helpers return
// silently when there is no selection, which on a button press reads as a dead
// control.
func (m model) runAction(i int) (tea.Model, tea.Cmd) {
	acts := m.listActions()
	if i < 0 || i >= len(acts) {
		return m, nil
	}
	if _, ok := m.selectedRef(); acts[i].needsSel && !ok {
		m.setStatus("highlight a prompt first — ↑/↓ to choose one", false)
		return m, nil
	}
	switch i {
	case actionAdd:
		return m.beginAdd()
	case actionEdit:
		return m.beginEdit()
	case actionSend:
		return m.beginDrop()
	case actionDelete:
		return m.beginDelete()
	}
	return m, nil
}

// selectedRef returns the highlighted todo's ref, and whether one is selected.
func (m model) selectedRef() (todoRef, bool) {
	idx := m.list.selectedIndex()
	if idx < 0 || idx >= len(m.rows) {
		return todoRef{}, false
	}
	return m.rows[idx], true
}

func (m *model) storeFor(s scope) *store {
	if s == scopeProject {
		return m.project
	}
	return m.global
}

func (m model) resolve(ref todoRef) (Todo, bool) {
	return m.storeFor(ref.scope).find(ref.id)
}

func (m model) toggleSelected() (tea.Model, tea.Cmd) {
	ref, ok := m.selectedRef()
	if !ok {
		return m, nil
	}
	td, _ := m.resolve(ref)
	if err := m.storeFor(ref.scope).toggle(ref.id); err != nil {
		m.setStatus("save failed: "+err.Error(), true)
		return m, nil
	}
	if td.Done {
		m.setStatus("reopened", false)
	} else {
		m.setStatus("marked done", false)
	}
	m.rebuildList()
	return m, nil
}

func (m *model) setStatus(s string, isErr bool) {
	m.status = s
	m.statusErr = isErr
}

// rebuildList regenerates the list rows from the two stores. Project todos come
// first; within each scope open todos come before done ones (array order is the
// backlog's priority order), and done todos disappear entirely when hideDone is
// set. The "Project"/"Global" group headings only show when both groups have at
// least one visible todo.
func (m *model) rebuildList() {
	var items []listItem
	var rows []todoRef

	visible := func(s *store) int {
		n := 0
		for _, t := range s.todos {
			if !t.Done || !m.hideDone {
				n++
			}
		}
		return n
	}
	grouped := visible(m.project) > 0 && visible(m.global) > 0

	add := func(s *store) {
		appendTodo := func(t Todo) {
			ref := todoRef{scope: s.scope, id: t.ID}
			// Glyph and style travel separately: the renderer has to be able to
			// put the highlighted row's field behind the badge, which it cannot
			// do to text that is already rendered. The open marker names its own
			// dimming rather than inheriting any.
			badge, badgeStyle := "○", descStyle
			switch {
			case t.Done:
				badge, badgeStyle = "✓", checkStyle
			case t.Schedule != nil && t.Schedule.Missed:
				badge, badgeStyle = "◷", errStyle
			case t.Schedule != nil:
				badge, badgeStyle = "◷", schedStyle
			}
			name := t.Title
			if name == "" {
				name = firstLine(t.Prompt, 60)
			}
			// Attachments lead the description: a prompt written about a
			// screenshot usually reads as a fragment without it ("this is
			// wrong"), so the fact that one is carried is what makes the row
			// make sense.
			desc := firstLine(t.Prompt, 70)
			if n := len(t.Images); n > 0 {
				desc = fmt.Sprintf("📎%d  %s", n, desc)
			}
			// The fire time leads even the attachments: of everything on the
			// row it is the one fact that changes on its own — and "missed"
			// is the row asking for a hand.
			if sc := t.Schedule; sc != nil && !t.Done {
				when := formatScheduleTime(sc.At, time.Now())
				if sc.Missed {
					when = "missed " + when
				}
				desc = fmt.Sprintf("⏰ %s  %s", when, desc)
			}
			items = append(items, listItem{
				name: name,
				desc: desc,
				// Match against the whole prompt (flattened to one line), not
				// just the rendered first-line preview, so a filter can hit
				// text buried deep in a multi-line prompt.
				search:     strings.Join(strings.Fields(t.Title+" "+t.Prompt), " "),
				badge:      badge,
				badgeStyle: badgeStyle,
				strike:     t.Done,
				selectable: true,
				ref:        len(rows),
			})
			rows = append(rows, ref)
		}
		for _, t := range s.todos {
			if !t.Done {
				appendTodo(t)
			}
		}
		if m.hideDone {
			return
		}
		for _, t := range s.todos {
			if t.Done {
				appendTodo(t)
			}
		}
	}

	if grouped {
		items = append(items, listItem{name: "Project"})
	}
	add(m.project)
	if grouped {
		items = append(items, listItem{name: "Global"})
	}
	add(m.global)

	m.rows = rows
	// Swap in the new rows while keeping the existing query box and cursor
	// (setItems re-filters and clamps), so an add/edit/toggle doesn't disturb
	// what the user has typed or where they were.
	m.list.setItems(items)
	// The header budget moves with the list — the count's reserved digits and
	// the done-hidden tag both feed it — so the query input is re-sized here,
	// not only on resize. Before the first WindowSizeMsg there is no budget to
	// apply and the construction-time width stands.
	if m.width > 0 {
		m.sizeSearchInput()
	}
}

// hiddenDoneCount is how many completed todos the hideDone fold is holding back,
// for the header note.
func (m model) hiddenDoneCount() int {
	n := 0
	for _, s := range []*store{m.project, m.global} {
		for _, t := range s.todos {
			if t.Done {
				n++
			}
		}
	}
	return n
}

// moveSelected shifts the highlighted todo one step up or down within its scope
// and done-state group, then keeps the highlight on it.
func (m model) moveSelected(delta int) (tea.Model, tea.Cmd) {
	ref, ok := m.selectedRef()
	if !ok {
		return m, nil
	}
	if err := m.storeFor(ref.scope).move(ref.id, delta); err != nil {
		m.setStatus("move failed: "+err.Error(), true)
		return m, nil
	}
	m.rebuildList()
	m.selectRow(ref)
	return m, nil
}

// selectRow parks the list cursor on the row showing ref, so a reorder or
// rebuild keeps the highlight on the same todo.
func (m *model) selectRow(ref todoRef) {
	for i, r := range m.rows {
		if r == ref {
			m.list.selectRef(i)
			return
		}
	}
}

// --- Add / Edit form ----------------------------------------------------------

func (m model) beginAdd() (tea.Model, tea.Cmd) {
	// Neither backlog is writable: a --project launch that found no project
	// (the pane woke up at the filesystem root), the one combination that
	// leaves both stores unavailable. An unavailable store's save is a silent
	// no-op that reports success, so the form would take the prompt and drop
	// it — refuse to open it instead.
	if !m.project.available() && !m.global.available() {
		m.setStatus("no project backlog here — relaunch from a project directory, or with --global", true)
		return m, nil
	}
	m.formMode = formAdd
	// Default to the project backlog when launched inside a project, else global.
	if m.project.available() {
		m.formScope = scopeProject
	} else {
		m.formScope = scopeGlobal
	}
	m.editID = ""
	m.titleInput, m.promptArea = m.newFormInputs("", "")
	m.formImages, m.formImagesOrig = nil, nil
	m.discardClipboardCaptures()
	m.formFocus = 1 // start in the prompt — that's the point of an entry
	m.titleInput.Blur()
	cmd := m.promptArea.Focus()
	m.formErr = ""
	m.stage = stageForm
	return m, cmd
}

func (m model) beginEdit() (tea.Model, tea.Cmd) {
	ref, ok := m.selectedRef()
	if !ok {
		return m, nil
	}
	return m.beginEditRef(ref)
}

// beginEditRef opens the edit form for a specific todo — from the list's
// highlight or from the view stage.
func (m model) beginEditRef(ref todoRef) (tea.Model, tea.Cmd) {
	td, ok := m.resolve(ref)
	if !ok {
		return m, nil
	}
	m.formMode = formEdit
	m.formScope = ref.scope
	m.editID = ref.id
	m.titleInput, m.promptArea = m.newFormInputs(td.Title, td.Prompt)
	m.formImages = m.newFormImages(ref.scope, td)
	m.formImagesOrig = td.Images
	m.discardClipboardCaptures()
	m.formFocus = 1
	m.titleInput.Blur()
	cmd := m.promptArea.Focus()
	m.formErr = ""
	m.stage = stageForm
	return m, cmd
}

// newFormInputs builds the title field and prompt editor, sized to the screen
// and pre-filled with the given values.
func (m model) newFormInputs(title, prompt string) (textinput.Model, textarea.Model) {
	ti := textinput.New()
	ti.Placeholder = "Short title (optional — derived from the prompt if blank)"
	ti.Prompt = ""
	ti.CharLimit = 140
	ti.SetValue(title)

	ta := textarea.New()
	ta.Placeholder = "The prompt to hand Claude Code later…"
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	// Move the newline off plain enter, which now saves the form, onto the
	// modifier chord. All three spellings are bound: shift+enter needs the
	// kitty protocol, alt+enter is the legacy ESC CR every terminal sends, and
	// ctrl+j is the raw line feed that survives even a terminal which eats
	// Option. ctrl+m — the default binding's second key, and literally the CR
	// that plain enter sends on a legacy terminal — has to go, or enter would
	// still insert a newline there instead of saving.
	ta.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("shift+enter", "alt+enter", "ctrl+j"),
		key.WithHelp("shift+enter", "insert newline"),
	)
	ta.SetValue(prompt)

	w := m.width - 4
	if w < 20 {
		w = 60
	}
	ti.SetWidth(w)
	ta.SetWidth(w)
	h := m.height - 12
	if h < 4 {
		h = 8
	}
	ta.SetHeight(h)
	return ti, ta
}

func (m model) updateForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		// Abandoning the form abandons its clipboard captures with it: their temp
		// files exist only because this form was open.
		m.discardClipboardCaptures()
		m.backToList()
		m.formErr = ""
		return m, nil
	case "enter":
		// Enter saves from either field. The prompt area's newline lives on
		// modifier+enter instead (see newFormInputs, which rebinds the
		// textarea's InsertNewline off plain enter so it never swallows this).
		return m.saveForm()
	case "tab", "shift+tab":
		return m.toggleFormFocus()
	case "ctrl+s":
		return m.saveForm()
	case "ctrl+o", "ctrl+i":
		// ctrl+o is the binding that always works; ctrl+i is the mnemonic that
		// mostly cannot. ctrl+i *is* tab on the wire — both are 0x09 — so a
		// terminal without the kitty protocol delivers it as the tab above,
		// switching fields. Under kitty they are distinct keys and the mnemonic
		// works, which is why it stays bound; the footer advertises ctrl+o,
		// the one that cannot lose. (Same problem, same shape of answer, as
		// shift+enter vs alt+enter — see modEnter.)
		return m.beginImages()
	case "ctrl+g":
		// Toggling scope needs both stores: an only-mode launch (--project /
		// --global) has one side unavailable, and switching to it would save
		// into a store that writes to nothing.
		if m.formMode == formAdd && m.project.available() && m.global.available() {
			if m.formScope == scopeProject {
				m.formScope = scopeGlobal
			} else {
				m.formScope = scopeProject
			}
		}
		return m, nil
	}
	return m.forwardForm(msg)
}

func (m model) toggleFormFocus() (tea.Model, tea.Cmd) {
	if m.formFocus == 0 {
		m.formFocus = 1
		m.titleInput.Blur()
		return m, m.promptArea.Focus()
	}
	m.formFocus = 0
	m.promptArea.Blur()
	return m, m.titleInput.Focus()
}

func (m model) forwardForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.formFocus == 0 {
		m.titleInput, cmd = m.titleInput.Update(msg)
	} else {
		m.promptArea, cmd = m.promptArea.Update(msg)
	}
	return m, cmd
}

// --- Attachment editor --------------------------------------------------------

// newFormImages builds the editor's starting rows from a todo's stored
// attachments, carrying each one's missing state so an edit can see — and
// choose to drop — a file that has gone.
func (m model) newFormImages(sc scope, td Todo) []formImage {
	refs := m.storeFor(sc).resolveImages(td)
	if len(refs) == 0 {
		return nil
	}
	imgs := make([]formImage, len(refs))
	for i, ref := range refs {
		imgs[i] = formImage{rel: ref.rel, name: path.Base(ref.rel), missing: ref.missing}
	}
	return imgs
}

// beginImages opens the attachment editor over the form. The form's own inputs
// are blurred so only one cursor blinks; closeImages restores whichever field
// had focus.
func (m model) beginImages() (tea.Model, tea.Cmd) {
	ti := textinput.New()
	ti.Placeholder = "path to an image — paste it, or drag the file onto this pane"
	ti.Prompt = ""
	if w := m.width - 4; w >= 20 {
		ti.SetWidth(w)
	}
	m.imgInput = ti
	m.imgCursor = 0
	m.setImgStatus("", false)
	m.recent, m.recentIdx = nil, 0
	m.titleInput.Blur()
	m.promptArea.Blur()
	m.stage = stageImages
	return m, m.imgInput.Focus()
}

// setImgStatus sets the attachment editor's message line, mirroring setStatus.
func (m *model) setImgStatus(s string, isErr bool) {
	m.imgStatus = s
	m.imgStatusErr = isErr
}

// closeImages returns to the form, restoring focus to the field that had it.
func (m model) closeImages() (tea.Model, tea.Cmd) {
	m.setImgStatus("", false)
	m.stage = stageForm
	if m.formFocus == 0 {
		return m, m.titleInput.Focus()
	}
	return m, m.promptArea.Focus()
}

func (m model) updateImages(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		return m.closeImages()
	case "enter":
		// The same "do what's in front of me" split the list uses: a path in the
		// box gets attached, an empty box means there is nothing left to add.
		if strings.TrimSpace(m.imgInput.Value()) == "" {
			return m.closeImages()
		}
		return m.addFormImage()
	case "up", "ctrl+p":
		if m.imgCursor > 0 {
			m.imgCursor--
		}
		return m, nil
	case "down", "ctrl+n":
		if m.imgCursor < len(m.formImages)-1 {
			m.imgCursor++
		}
		return m, nil
	case "ctrl+x":
		return m.removeFormImage()
	case "ctrl+r":
		return m.cycleRecentImage()
	case "ctrl+v":
		// One key for both kinds of paste. An image on the clipboard gets
		// captured; anything else falls through to the input's own ctrl+v, which
		// pastes text — exactly what someone who just copied a path wants, and
		// the only thing this key could have done before captures existed.
		switch clipboardOffer() {
		case clipPNG:
			return m.pasteClipboardImage()
		case clipUnusableImage:
			m.setImgStatus("the clipboard has an image macOS will not hand over as PNG — save it to a file and attach that", true)
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.imgInput, cmd = m.imgInput.Update(msg)
	return m, cmd
}

// pasteClipboardImage captures the clipboard image to a temp file and queues it
// like any other pending source, so it goes through the same validation (size,
// format) and the same copy-at-save path as a typed one.
func (m model) pasteClipboardImage() (tea.Model, tea.Cmd) {
	if !clipboardImageSupported() {
		m.setImgStatus("pasting an image is only supported on macOS — save it to a file and attach the path", true)
		return m, nil
	}
	src, err := captureClipboardImage()
	if err != nil {
		m.setImgStatus(err.Error(), true)
		return m, nil
	}
	abs, err := validateImageSource(src)
	if err != nil {
		// The capture is ours and no longer wanted — an oversized paste must not
		// leave the temp copy behind.
		os.RemoveAll(filepath.Dir(src))
		m.setImgStatus(err.Error(), true)
		return m, nil
	}

	m.clipboardDirs = append(m.clipboardDirs, filepath.Dir(abs))
	m.formImages = append(m.formImages, formImage{src: abs, name: clipboardImageName, pasted: true})
	m.imgCursor = len(m.formImages) - 1
	m.setImgStatus("pasted from the clipboard", false)
	return m, nil
}

// discardClipboardCaptures removes the temp directories behind this form's
// clipboard captures. Called on both exits from the form: after a save has
// copied them into the backlog, and when the form is abandoned. Best effort —
// a temp file we failed to delete is not worth failing a save over.
func (m *model) discardClipboardCaptures() {
	for _, dir := range m.clipboardDirs {
		_ = os.RemoveAll(dir)
	}
	m.clipboardDirs = nil
}

// addFormImage validates what is in the box and queues it as a pending
// attachment. Nothing is copied here — an add has no todo id yet, and an edit
// should not leave files behind if the form is then cancelled — so this only
// checks that the file is one we could copy at save time.
func (m model) addFormImage() (tea.Model, tea.Cmd) {
	src := cleanSourcePath(m.imgInput.Value())
	if src == "" {
		return m, nil
	}
	abs, err := validateImageSource(src)
	if err != nil {
		m.setImgStatus(err.Error(), true)
		return m, nil
	}
	for _, img := range m.formImages {
		if img.src == abs {
			m.setImgStatus(path.Base(abs)+" is already attached", true)
			return m, nil
		}
	}
	m.formImages = append(m.formImages, formImage{src: abs, name: filepath.Base(abs)})
	m.imgCursor = len(m.formImages) - 1
	m.imgInput.SetValue("")
	m.setImgStatus("", false)
	return m, nil
}

// removeFormImage drops the highlighted row. For a pending source that is the
// whole story; for one already on the todo it only marks it dropped — the file
// goes when the form is saved, so cancelling the form still changes nothing.
func (m model) removeFormImage() (tea.Model, tea.Cmd) {
	if m.imgCursor < 0 || m.imgCursor >= len(m.formImages) {
		return m, nil
	}
	gone := m.formImages[m.imgCursor]
	m.formImages = append(m.formImages[:m.imgCursor], m.formImages[m.imgCursor+1:]...)
	if gone.pasted {
		// Nothing else refers to a capture's temp file, so it goes now rather
		// than waiting for the form to close.
		_ = os.RemoveAll(filepath.Dir(gone.src))
	}
	if m.imgCursor >= len(m.formImages) {
		m.imgCursor = len(m.formImages) - 1
	}
	if m.imgCursor < 0 {
		m.imgCursor = 0
	}
	note := "removed " + gone.name
	if gone.rel != "" {
		note += " — the file goes when you save"
	}
	if gone.pasted {
		note = "removed the pasted image"
	}
	m.setImgStatus(note, false)
	return m, nil
}

// cycleRecentImage fills the box with the most recent image found on disk, then
// the next older one on each press — the screenshot-then-attach path, without
// typing a path at all. It only offers the path; enter still attaches, so a
// wrong guess costs nothing.
func (m model) cycleRecentImage() (tea.Model, tea.Cmd) {
	if m.recent == nil {
		m.recent = recentImages()
		m.recentIdx = 0
	}
	if len(m.recent) == 0 {
		m.setImgStatus("no recent images found — set "+imageSourceDirEnvVar+" to point at your screenshot folder", true)
		return m, nil
	}
	m.imgInput.SetValue(m.recent[m.recentIdx])
	m.imgInput.CursorEnd()
	m.recentIdx = (m.recentIdx + 1) % len(m.recent)
	m.setImgStatus(fmt.Sprintf("recent %d/%d — enter to attach, ctrl+r for the next",
		((m.recentIdx-1)+len(m.recent))%len(m.recent)+1, len(m.recent)), false)
	return m, nil
}

// pendingSources are the queued attachment sources, in the order the user added
// them — the list saveForm copies into the backlog.
func (m model) pendingSources() []string {
	var srcs []string
	for _, img := range m.formImages {
		if img.src != "" {
			srcs = append(srcs, img.src)
		}
	}
	return srcs
}

// keptRels are the todo's existing attachments that survived the edit, in
// display order.
func (m model) keptRels() []string {
	var rels []string
	for _, img := range m.formImages {
		if img.rel != "" {
			rels = append(rels, img.rel)
		}
	}
	return rels
}

// droppedRels are the attachments the todo had when the form opened and no
// longer has — the files to delete once the new list is safely saved.
func (m model) droppedRels() []string {
	kept := make(map[string]bool, len(m.formImages))
	for _, rel := range m.keptRels() {
		kept[rel] = true
	}
	var dropped []string
	for _, rel := range m.formImagesOrig {
		if !kept[rel] {
			dropped = append(dropped, rel)
		}
	}
	return dropped
}

// imageCountNote describes the form's attachment list for a heading or status
// line, or "" when there is nothing attached.
func (m model) imageCountNote() string {
	if len(m.formImages) == 0 {
		return ""
	}
	if len(m.formImages) == 1 {
		return "1 image"
	}
	return fmt.Sprintf("%d images", len(m.formImages))
}

func (m model) saveForm() (tea.Model, tea.Cmd) {
	title := strings.TrimSpace(m.titleInput.Value())
	prompt := strings.TrimSpace(m.promptArea.Value())
	if prompt == "" {
		m.formErr = "The prompt can't be empty."
		return m, nil
	}
	if title == "" {
		title = firstLine(prompt, 60)
	}

	st := m.storeFor(m.formScope)
	// Backstop for the same silent-no-op hazard beginAdd guards: any future
	// path into the form with an unavailable target must fail loudly rather
	// than report a save that wrote nothing.
	if !st.available() {
		m.formErr = "no " + strings.ToLower(m.formScope.String()) + " backlog is available here"
		return m, nil
	}
	note := ""
	if n := m.imageCountNote(); n != "" {
		note = " · " + n
	}

	if m.formMode == formAdd {
		// The id has to exist before the copy — it names the attachment
		// directory — so it is minted here rather than inline below.
		id := newID()
		added, err := st.attachImages(id, m.pendingSources())
		if err != nil {
			m.formErr = "attach failed: " + err.Error()
			return m, nil
		}
		if err := st.add(Todo{ID: id, Title: title, Prompt: prompt, Images: added, Created: time.Now()}); err != nil {
			// The copies are on disk but no todo will ever reference them.
			st.removeImages(id)
			m.formErr = "save failed: " + err.Error()
			return m, nil
		}
		m.setStatus("added to "+m.formScope.String()+" backlog"+note, false)
	} else {
		// Copy first, then record, then delete: every step leaves the todo
		// either as it was or as the user asked for, and a failure anywhere
		// takes this save's own copies back out with it.
		added, err := st.attachImages(m.editID, m.pendingSources())
		if err != nil {
			m.formErr = "attach failed: " + err.Error()
			return m, nil
		}
		if err := st.update(Todo{ID: m.editID, Title: title, Prompt: prompt}); err != nil {
			st.removeImageFiles(m.editID, added)
			m.formErr = "save failed: " + err.Error()
			return m, nil
		}
		if err := st.setImages(m.editID, append(m.keptRels(), added...)); err != nil {
			st.removeImageFiles(m.editID, added)
			m.formErr = "save failed: " + err.Error()
			return m, nil
		}
		// Only now does the record no longer mention them.
		st.removeImageFiles(m.editID, m.droppedRels())
		m.setStatus("updated"+note, false)
	}
	// The backlog holds its own copies now, so the captures' temp files have
	// nothing left to answer for.
	m.discardClipboardCaptures()
	m.rebuildList()
	m.backToList()
	return m, nil
}

// --- Delete confirm -----------------------------------------------------------

func (m model) beginDelete() (tea.Model, tea.Cmd) {
	ref, ok := m.selectedRef()
	if !ok {
		return m, nil
	}
	td, _ := m.resolve(ref)
	m.confirmKind = confirmDelete
	m.pendingDelete = ref
	m.pendingTitle = firstNonEmpty(td.Title, firstLine(td.Prompt, 40))
	m.stage = stageConfirm
	return m, nil
}

// beginClearDone arms the confirm stage to remove every completed todo across
// both scopes, or reports there is nothing to clear.
func (m model) beginClearDone() (tea.Model, tea.Cmd) {
	count := m.hiddenDoneCount()
	if count == 0 {
		m.setStatus("no completed prompts to clear", false)
		return m, nil
	}
	m.confirmKind = confirmClearDone
	m.pendingClearCount = count
	m.stage = stageConfirm
	return m, nil
}

func (m model) updateConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "y", "Y", "enter":
		if m.confirmKind == confirmClearDone {
			m.clearDone()
		} else {
			if err := m.storeFor(m.pendingDelete.scope).delete(m.pendingDelete.id); err != nil {
				m.setStatus("delete failed: "+err.Error(), true)
			} else {
				m.setStatus("deleted", false)
			}
		}
		m.rebuildList()
		m.backToList()
		return m, nil
	case "n", "N", "esc":
		m.backToList()
		return m, nil
	}
	return m, nil
}

// clearDone removes every completed todo from both stores and reports the total
// in the status line. A failure in one store doesn't stop the other; the first
// error wins the status.
func (m *model) clearDone() {
	removed := 0
	var firstErr error
	for _, s := range []*store{m.project, m.global} {
		n, err := s.clearDone()
		removed += n
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	switch {
	case firstErr != nil:
		m.setStatus("clear failed: "+firstErr.Error(), true)
	case removed == 1:
		m.setStatus("cleared 1 completed prompt", false)
	default:
		m.setStatus(fmt.Sprintf("cleared %d completed prompts", removed), false)
	}
}

// --- Target picker ------------------------------------------------------------

func (m model) beginDrop() (tea.Model, tea.Cmd) {
	ref, ok := m.selectedRef()
	if !ok {
		return m, nil
	}
	return m.startDrop(ref)
}

// startDrop opens the target picker for a specific todo — from the list's
// highlight or from the view stage. Guard failures land back on the list, where
// the status line is visible.
func (m model) startDrop(ref todoRef) (tea.Model, tea.Cmd) {
	if m.dropping {
		m.setStatus("a drop is still in progress…", false)
		m.backToList()
		return m, nil
	}
	if m.client == nil {
		m.setStatus("cats control socket unavailable — can't drop into a session", true)
		m.backToList()
		return m, nil
	}
	m.dropTodo = ref
	m.targets, m.targetList = m.buildTargets()
	m.stage = stageTarget
	return m, textinput.Blink
}

// buildTargets assembles the drop destinations: a new Claude session, a new
// session for every other agent currently running somewhere (if it's running,
// its command is installed), plus every agent pane cats reports (Claude panes
// first), excluding our own. Pane agent identity and cwd come straight from
// pane.list's runtime metadata; the workspace a pane lives in is the "w1"
// prefix of its handle, labeled via workspace.list.
func (m model) buildTargets() ([]dropTarget, fuzzyList) {
	wsLabel := firstNonEmpty(m.ctx.WorkspaceLabel, baseName(m.ctx.projectDir()), "the current workspace")
	targets := []dropTarget{{
		kind:    targetNewSession,
		command: "claude",
		label:   "＋ New Claude Code session",
		desc:    "open a new tab in " + wsLabel + " and launch claude",
	}}

	if m.client != nil {
		if panes, err := m.client.paneList(); err == nil {
			var agents []app.PaneInfo
			for _, p := range panes {
				if p.Agent == "" || isOwnPane(m.ctx, p) {
					continue
				}
				agents = append(agents, p)
			}
			sort.SliceStable(agents, func(i, j int) bool {
				return agents[i].Agent == "claude" && agents[j].Agent != "claude"
			})

			// One "new session" entry per distinct non-claude agent, launched
			// with the agent label cats detected as the command.
			seenAgent := map[string]bool{"claude": true}
			for _, p := range agents {
				if seenAgent[p.Agent] {
					continue
				}
				seenAgent[p.Agent] = true
				targets = append(targets, dropTarget{
					kind:    targetNewSession,
					command: p.Agent,
					label:   "＋ New " + p.Agent + " session",
					desc:    "open a new tab in " + wsLabel + " and launch " + p.Agent,
				})
			}

			wsLabels := map[string]string{}
			if labels, err := m.client.workspaceLabels(); err == nil {
				wsLabels = labels
			}
			for _, p := range agents {
				wsID := paneWorkspaceID(p)
				loc := firstNonEmpty(wsLabels[wsID], baseName(p.Cwd))
				here := ""
				if wsID != "" && wsID == m.ctx.WorkspaceID {
					here = " (this project)"
				}
				// Surface the agent's live state ("working", "idle", …) with its
				// location — picking an idle session over a busy one is usually
				// the point of the choice.
				desc := p.Cwd
				if p.AgentState != "" {
					desc = "[" + p.AgentState + "] " + desc
				}
				targets = append(targets, dropTarget{
					kind:  targetExistingPane,
					pane:  p.Pane,
					agent: p.Agent,
					label: fmt.Sprintf("%s · %s%s", p.Agent, firstNonEmpty(loc, "session"), here),
					desc:  desc,
				})
			}
		}
	}

	items := make([]listItem, len(targets))
	for i, t := range targets {
		items[i] = listItem{name: t.label, desc: t.desc, selectable: true, ref: i}
	}
	return targets, newFuzzyList("Filter targets…", items)
}

func (m model) updateTarget(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.backToList()
		return m, nil
	case "up", "ctrl+p":
		m.targetList.moveUp()
		return m, nil
	case "down", "ctrl+n":
		m.targetList.moveDown()
		return m, nil
	case "enter":
		return m.chooseTarget(dropPaste)
	case "shift+enter", "alt+enter", "ctrl+r":
		// modifier+enter keeps its meaning from the list — the more committing
		// of the two things enter could do here — so the picker's "submit it to
		// run" sits on the same chord. ctrl+r stays as the older spelling.
		return m.chooseTarget(dropRun)
	}
	cmd := m.targetList.editQuery(msg)
	return m, cmd
}

func (m model) chooseTarget(mode dropMode) (tea.Model, tea.Cmd) {
	if m.dropping {
		return m, nil
	}
	idx := m.targetList.selectedIndex()
	if idx < 0 || idx >= len(m.targets) {
		return m, nil
	}
	td, ok := m.resolve(m.dropTodo)
	if !ok {
		m.setStatus("could not find that prompt", true)
		m.backToList()
		return m, nil
	}
	target := m.targets[idx]
	// A schedule-flavored picker stores the choice instead of firing it; the
	// mode the chord implied is moot — a scheduled fire always runs.
	if m.pickForSchedule {
		return m.commitSchedule(target)
	}
	// Perform the drop without quitting: the pane persists, so we return to the
	// list and run the (potentially slow) drop off the UI thread, reporting back
	// via dropResultMsg. A drop into a new session focuses the freshly-created
	// tab, leaving this manager alive in the background to reuse later.
	m.dropping = true
	m.backToList()
	m.setStatus("dropping into "+targetDesc(target)+"…", false)
	return m, m.performDropCmd(m.dropTodo, pendingAction{
		todo:   td,
		target: target,
		mode:   mode,
		cwd:    m.ctx.projectDir(),
		images: m.storeFor(m.dropTodo.scope).imagePaths(td),
	})
}

// performDropCmd runs the chosen drop in a goroutine (a tea.Cmd) so cats's
// pane creation and claude launch — which can take several seconds — don't
// freeze the manager. It reports the destination and any error back as a
// dropResultMsg. The client is captured by value; performDrop only makes
// short-lived, independent socket calls, so this is safe to run alongside the
// still-rendering UI.
func (m model) performDropCmd(ref todoRef, act pendingAction) tea.Cmd {
	client := m.client
	desc := targetDesc(act.target)
	return func() tea.Msg {
		return dropResultMsg{desc: desc, ref: ref, err: performDrop(client, act)}
	}
}

// targetDesc is a short, human label for a drop destination, used in the
// "dropping…" / "dropped →" status lines.
func targetDesc(t dropTarget) string {
	if t.kind == targetNewSession {
		if t.command != "" && t.command != "claude" {
			return "new " + t.command + " session"
		}
		return "new Claude Code session"
	}
	return firstNonEmpty(t.agent, "session")
}

// --- Schedule stage -------------------------------------------------------------

// beginSchedule opens the schedule editor for the highlighted todo: one input
// for the fire time, then the ordinary target picker in schedule flavor. Same
// guards as startDrop — scheduling is a promise to drop, and a promise this
// process knows it cannot keep (no socket) should refuse now, on the list,
// not at fire time when nobody is watching.
func (m model) beginSchedule() (tea.Model, tea.Cmd) {
	ref, ok := m.selectedRef()
	if !ok {
		return m, nil
	}
	if m.client == nil {
		m.setStatus("cats control socket unavailable — can't schedule a drop", true)
		return m, nil
	}
	td, ok := m.resolve(ref)
	if !ok {
		m.setStatus("could not find that prompt", true)
		return m, nil
	}
	if td.Done {
		m.setStatus("that prompt is done — reopen it (ctrl+t) to schedule it", false)
		return m, nil
	}

	ti := textinput.New()
	ti.Prompt = ""
	// No placeholder: the example line under the input names the forms and
	// stays put while the user types — a placeholder would say the same thing
	// twice and then vanish the moment it was needed.
	ti.SetWidth(searchFieldWidth)
	// An existing schedule prefills in the editor's own datetime format, so
	// enter round-trips it and typing replaces it.
	if td.Schedule != nil {
		ti.SetValue(td.Schedule.At.Format(scheduleTimeStamp))
	}
	ti.Focus()

	m.schedRef = ref
	m.schedInput = ti
	m.schedErr = ""
	m.stage = stageSchedule
	return m, textinput.Blink
}

func (m model) updateSchedule(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.backToList()
		return m, nil
	case "enter":
		td, ok := m.resolve(m.schedRef)
		if !ok {
			m.setStatus("could not find that prompt", true)
			m.backToList()
			return m, nil
		}
		// Enter on an emptied input is the clear gesture — the schedule editor
		// is also where an existing schedule is looked at, and "delete the
		// text, press enter" is what removing it should cost.
		if strings.TrimSpace(m.schedInput.Value()) == "" {
			if td.Schedule == nil {
				m.schedErr = "type a time — or esc to leave"
				return m, nil
			}
			if err := m.storeFor(m.schedRef.scope).setSchedule(m.schedRef.id, nil); err != nil {
				m.schedErr = "save failed: " + err.Error()
				return m, nil
			}
			m.rebuildList()
			m.backToList()
			m.setStatus("schedule cleared", false)
			return m, nil
		}
		at, err := parseScheduleTime(m.schedInput.Value(), time.Now())
		if err != nil {
			m.schedErr = err.Error()
			return m, nil
		}
		// Time first, then target: the picker is the flow's one exit for both
		// manual and scheduled drops (chooseTarget), so the time has to be in
		// hand before it opens.
		m.schedAt = at
		m.dropTodo = m.schedRef
		m.pickForSchedule = true
		m.targets, m.targetList = m.buildTargets()
		m.stage = stageTarget
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.schedInput, cmd = m.schedInput.Update(msg)
	if m.schedErr != "" {
		m.schedErr = "" // the error described the last attempt, not this text
	}
	return m, cmd
}

// commitSchedule stores the picker's choice on the todo and returns to the
// list. Nothing fires here — the tick loop owns firing — so a scheduled todo
// survives quit/relaunch as data, not as a pending goroutine.
func (m model) commitSchedule(target dropTarget) (tea.Model, tea.Cmd) {
	sc := scheduleFromTarget(target, m.schedAt, m.ctx.projectDir())
	if err := m.storeFor(m.schedRef.scope).setSchedule(m.schedRef.id, &sc); err != nil {
		m.setStatus("save failed: "+err.Error(), true)
		m.backToList()
		return m, nil
	}
	m.rebuildList()
	m.backToList()
	m.setStatus("scheduled for "+formatScheduleTime(sc.At, time.Now())+" → "+targetDesc(target), false)
	return m, nil
}

func (m model) viewSchedule() string {
	td, _ := m.resolve(m.schedRef)
	title := firstNonEmpty(td.Title, firstLine(td.Prompt, 50))

	var b strings.Builder
	b.WriteString(titleStyle.Render("Schedule drop"))
	b.WriteString("  ")
	b.WriteString(descStyle.Render(truncate(title, 60)))
	b.WriteString("\n\n")
	b.WriteString(promptStyle.Render("When"))
	b.WriteString("\n")
	b.WriteString(m.schedInput.View())
	b.WriteString("\n")
	if m.schedErr != "" {
		b.WriteString(errStyle.Render("• " + m.schedErr))
	} else {
		b.WriteString(descStyle.Render("15:30 · in 2h · tomorrow 9:00 · fires as \"drop & run\""))
	}
	b.WriteString("\n\n")
	foot := "enter next: pick target · esc back"
	if td.Schedule != nil {
		foot = "enter next: pick target · empty + enter clears · esc back"
	}
	b.WriteString(footerStyle.Render(foot))
	return b.String()
}

// --- Schedule firing ------------------------------------------------------------

// fireDueSchedules is the tick's work: walk both backlogs for a schedule
// whose moment has come, and fire it — or, when it can no longer be honored,
// mark it Missed where the row will show it. At most one fire per tick: the
// in-flight drop owns m.dropping, and a second due schedule simply waits for
// a later second, still inside its grace window.
func (m *model) fireDueSchedules(now time.Time) tea.Cmd {
	for _, s := range []*store{m.project, m.global} {
		for _, t := range s.todos {
			sc := t.Schedule
			if sc == nil || sc.Missed || t.Done || now.Before(sc.At) {
				continue
			}
			ref := todoRef{scope: s.scope, id: t.ID}

			// Too stale to fire, or nothing to fire through: record it on the
			// row. Late beyond the grace means the manager wasn't here at the
			// time — firing into a pane hours after the moment the user chose
			// is a surprise, not a delivery.
			if now.Sub(sc.At) > scheduleGrace || m.client == nil {
				missed := *sc
				missed.Missed = true
				_ = s.setSchedule(t.ID, &missed)
				m.rebuildList()
				m.setStatus("missed schedule ("+formatScheduleTime(sc.At, now)+") — "+firstNonEmpty(t.Title, "prompt")+" needs a manual send", true)
				continue
			}

			// A drop is already typing into a pane; interleaving a second
			// would garble both. The schedule stays put and the next tick
			// retries, still within grace.
			if m.dropping {
				return nil
			}

			// Claim before fire: the schedule comes off disk first, so a
			// second manager pane scanning the same backlog finds it gone and
			// stands down (see claimSchedule). Losing the claim is the same
			// news — someone else is firing it.
			won, err := s.claimSchedule(t.ID, sc.At)
			if err != nil {
				m.setStatus("schedule claim failed: "+err.Error(), true)
				return nil
			}
			if !won {
				m.rebuildList()
				return nil
			}
			td, ok := m.resolve(ref)
			if !ok {
				return nil
			}
			m.dropping = true
			m.rebuildList()
			m.setStatus("firing scheduled drop → "+targetDesc(targetFromSchedule(*sc))+"…", false)
			return m.performScheduledDropCmd(ref, *sc, td)
		}
	}
	return nil
}

// performScheduledDropCmd mirrors performDropCmd for a schedule firing: the
// pendingAction is assembled the way chooseTarget builds one, and the result
// carries the schedule so a failure can be written back as Missed.
func (m model) performScheduledDropCmd(ref todoRef, sc Schedule, td Todo) tea.Cmd {
	client := m.client
	act := pendingAction{
		todo:   td,
		target: targetFromSchedule(sc),
		mode:   dropRun,
		cwd:    sc.Cwd,
		images: m.storeFor(ref.scope).imagePaths(td),
	}
	desc := targetDesc(act.target)
	return func() tea.Msg {
		return dropResultMsg{desc: desc, ref: ref, err: performScheduledDrop(client, sc, act), sched: &sc}
	}
}

// --- Prompt view ----------------------------------------------------------------

// beginView opens the read-only view of the highlighted todo's full prompt —
// the way to read a long prompt without entering the edit form.
func (m model) beginView() (tea.Model, tea.Cmd) {
	ref, ok := m.selectedRef()
	if !ok {
		return m, nil
	}
	td, ok := m.resolve(ref)
	if !ok {
		return m, nil
	}
	m.viewRef = ref
	m.viewVP = viewport.New(viewport.WithWidth(m.viewWidth()), viewport.WithHeight(m.viewHeight()))
	m.viewVP.SetContent(m.viewContent(td))
	m.stage = stageView
	return m, nil
}

func (m model) updateView(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "q":
		m.backToList()
		return m, nil
	case "enter":
		// Same split as the list: enter opens the prompt for editing,
		// modifier+enter hands it to an agent.
		return m.beginEditRef(m.viewRef)
	case "shift+enter", "alt+enter":
		return m.startDrop(m.viewRef)
	case "ctrl+e":
		return m.beginEditRef(m.viewRef)
	}
	// Everything else (arrows, pgup/pgdn, mouse wheel) scrolls the body.
	var cmd tea.Cmd
	m.viewVP, cmd = m.viewVP.Update(msg)
	return m, cmd
}

// viewWidth and viewHeight size the view's scrollable body to the window,
// leaving room for the heading, meta line, and footer.
func (m model) viewWidth() int {
	if w := m.width - 4; w >= 20 {
		return w
	}
	return 76
}

func (m model) viewHeight() int {
	if h := m.height - 8; h >= 4 {
		return h
	}
	return 16
}

// viewContent renders the todo's full prompt wrapped to the view's width,
// followed by its attachments when it has any.
//
// The attachment lines are plain text rather than styled: the whole body goes
// through one lipgloss Width() wrap, and pre-styled spans inside a wrapped
// block have their ANSI resets clobbered at the wrap points — the same hazard
// the list's badges are written verbatim to avoid.
func (m model) viewContent(td Todo) string {
	body := td.Prompt
	if refs := m.storeFor(m.viewRef.scope).resolveImages(td); len(refs) > 0 {
		var b strings.Builder
		b.WriteString(body)
		fmt.Fprintf(&b, "\n\n📎 %d attached:", len(refs))
		for _, ref := range refs {
			b.WriteString("\n  " + ref.rel)
			// A missing file is dropped from the delivered prompt rather than
			// sent for the agent to chase, so this view is the only place the
			// user finds out it went.
			if ref.missing {
				b.WriteString("   (missing — will not be sent)")
			}
		}
		body = b.String()
	}
	return lipgloss.NewStyle().Width(m.viewWidth()).Render(body)
}

// --- View ---------------------------------------------------------------------

// openCount is how many unfinished todos sit in the backlog this pane is named
// after: the project's, or the global one when that is the only backlog the
// launch opened (--global, or no project to scope to).
//
// A project pane deliberately does not count global todos, even though its list
// shows them. The global backlog is the same list in every project, so counting
// it would have every manager pane in the session report the same floor — and
// the number is read as a statement about the project the title names.
func (m model) openCount() int {
	s := m.project
	if !s.available() {
		s = m.global
	}
	n := 0
	for _, t := range s.todos {
		if !t.Done {
			n++
		}
	}
	return n
}

// paneTitle is the terminal title for this frame: the backlog's name (see
// RunContext.paneTitle) with the open-item count appended when there is work
// left — "todo: cats (3)".
//
// Nothing is appended at zero, so an emptied backlog reads as "todo: cats"
// rather than as "todo: cats (0)". That silence is load-bearing: cats's sidebar
// marks a workspace as holding unfinished work by finding a count here, so a
// backlog with nothing left in it has to look the same as one that never had
// anything (see todoMark in catway's index.html).
func (m model) paneTitle() string {
	t := m.ctx.paneTitle()
	if n := m.openCount(); n > 0 {
		t = fmt.Sprintf("%s (%d)", t, n)
	}
	return t
}

// View renders the active stage. In bubbletea v2 the alt screen and the pane
// title are properties of the view rather than startup options, so they are
// declared on every frame — which is also what keeps the title's open count
// current, since every add, edit and toggle redraws.
func (m model) View() tea.View {
	v := tea.NewView(m.renderStage())
	v.AltScreen = true
	v.WindowTitle = m.paneTitle()
	// The manager's ground, so the palette reads as a theme rather than as
	// green text on whatever the pane happened to be. Like the window title
	// this is a property of the terminal, not of this process — bubbletea
	// resets it when it tears the view down, and runTodoUI carries the backstop
	// for an exit that skips the renderer.
	v.BackgroundColor = lipgloss.Color(colBg)
	v.ForegroundColor = lipgloss.Color(colFg)
	// Mouse reporting is asked for only where there is something to click. It
	// isn't free: while it is on, a drag belongs to this program rather than to
	// the terminal, so the pane's own click-to-select stops working. The list
	// stage pays that for the action bar and the picker for its target rows;
	// the prompt view — the one screen whose whole point is text worth copying
	// out — keeps its selection.
	if m.stage == stageList || m.stage == stageTarget {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// renderStage is the rendered body of the current stage.
func (m model) renderStage() string {
	if m.quitting {
		return ""
	}
	switch m.stage {
	case stageForm:
		return m.viewForm()
	case stageConfirm:
		return m.viewConfirm()
	case stageTarget:
		return m.viewTarget()
	case stageView:
		return m.viewPrompt()
	case stageImages:
		return m.viewImages()
	case stageSchedule:
		return m.viewSchedule()
	default:
		return m.viewList()
	}
}

// listTitle is the fixed heading of the manager's list view. Everything else
// on the header line — the scope note, the query box, the match count — is
// budgeted around it by headerLayout, since the five of them share one
// terminal line.
const listTitle = "📝 Cats Todo"

// headerChip renders the title chip, with the binary's version trailing the
// name when the line has room for the footnote — dimmed inside the same field
// so it reads as an aside to the title rather than a second thing on the line.
// The label drops its right padding so the version sits one space after it and
// the chip keeps one padded edge.
func headerChip(withVersion bool) string {
	if !withVersion {
		return titleStyle.Render(listTitle)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		titleStyle.PaddingRight(0).Render(listTitle),
		titleVerStyle.Render(" v"+version),
	)
}

// headerGap is the breath between the header line's segments.
const headerGap = 2

// searchGlyph fronts the inline query input on the header line. With the
// boxed field's rails gone from this view, the glyph carries the focus
// affordance: accent-bold when the box holds the keys, faint when a button
// does (see headerLine).
const searchGlyph = "🔍 "

// inputCursorPad is the one cell a bubbles textinput renders past its
// SetWidth — the column its cursor advances into. Measured against the
// library, and held to it by TestHeaderLineFits: budgeting the input at its
// bare SetWidth is exactly the off-by-one that used to push the match count
// off-screen.
const inputCursorPad = 1

// The query input's sizes as the header narrows: full, squeezed (still wide
// enough to read a typed word), and the floor below which the segment is
// dropped from the line rather than rendered as a slot nothing fits in.
// Type-to-filter keeps working with the segment gone — only the echo is lost.
const (
	searchFieldSqueezed = 12
	searchFieldMin      = 6
)

// headerLayout is what survives on the header line at the current width. The
// zero value means "everything": version shown, full-size search, whole note.
type headerLayout struct {
	withVersion bool
	note        string // budgeted scope note (done-hidden tag included); "" = dropped
	searchW     int    // the query input's SetWidth; 0 = segment dropped
	count       string // "matched/total"
	countW      int    // cells reserved for count — total's digits, so typing doesn't shift the line
}

// headerLayout budgets the header's one line: title chip, scope note, query
// box, match count. The line must never exceed m.width — every row below it
// is hit-tested against a constant (see actionBarRow), and a wrapped header
// silently moves all of them out from under the mouse.
//
// When the pane is too narrow for everything, the segments concede in a fixed
// order, cheapest first:
//
//	ws label → done-hidden tag → search shrinks → version → search floors,
//	then drops → project name truncates (inside scopeNote) → count last
//
// The version goes before any of the project name does: the name is the fact
// that decides what an edit touches, the version is a footnote.
func (m model) headerLayout() headerLayout {
	matched, total := m.list.counts()
	hl := headerLayout{
		withVersion: true,
		searchW:     searchFieldWidth,
		count:       fmt.Sprintf("%d/%d", matched, total),
	}
	hl.countW = len(fmt.Sprintf("%d/%d", total, total))

	// m.width is 0 until the first WindowSizeMsg lands; with nothing to budget
	// against, show everything rather than guess.
	if m.width <= 0 {
		hl.note = m.scopeNote(-1) + m.hiddenNote()
		return hl
	}

	// Room left for the note at the current concessions. The input is counted
	// one cell wider than its SetWidth (inputCursorPad); rune counts stand in
	// for cell width on the note itself — it is plain prose.
	room := func() int {
		r := m.width - lipgloss.Width(headerChip(hl.withVersion)) - headerGap - hl.countW
		if hl.searchW > 0 {
			r -= headerGap + lipgloss.Width(searchGlyph) + hl.searchW + inputCursorPad
		}
		return r - headerGap
	}

	runew := func(s string) int { return len([]rune(s)) }
	hidden := m.hiddenNote()
	// The core is the note's must-keep part — project name and which backlogs
	// are live, no ws tag. The ws label concedes on its own inside scopeNote
	// (it only ever gets the room the core leaves over), so the ladder here
	// starts one rung up.
	core := runew(m.scopeNote(1 << 20))
	if i := strings.Index(m.scopeNote(1<<20), " · ws:"); i >= 0 {
		core = runew(m.scopeNote(1 << 20)[:i])
	}

	// Each rung runs only while the line still overflows with the core intact.
	if core+runew(hidden) > room() {
		hidden = ""
	}
	if over := core + runew(hidden) - room(); over > 0 && hl.searchW > searchFieldSqueezed {
		hl.searchW -= min(over, hl.searchW-searchFieldSqueezed)
	}
	if core+runew(hidden) > room() {
		hl.withVersion = false
	}
	if core+runew(hidden) > room() && hl.searchW > searchFieldMin {
		hl.searchW = searchFieldMin
	}
	if core+runew(hidden) > room() && hl.searchW > 0 {
		hl.searchW = 0
	}

	// Whatever room the ladder ended at, the note fits itself into: the ws
	// label truncates and drops first, the project name only at the very end.
	if r := room() - runew(hidden); r > 0 {
		hl.note = m.scopeNote(r) + hidden
	}
	return hl
}

// searchWidth is the SetWidth for the header's inline query input — read off
// the same layout the header renders from, so the drawn line and the input's
// own idea of its width cannot disagree.
func (m model) searchWidth() int {
	return m.headerLayout().searchW
}

// sizeSearchInput pushes the header budget into the query input. Called from
// applySizes (the width changed) and rebuildList (the count's digits and the
// done-hidden tag changed — both feed the budget).
func (m *model) sizeSearchInput() {
	if sw := m.searchWidth(); sw > 0 {
		m.list.input.SetWidth(sw)
	}
}

// scopeNote names both scopes in the header: which project backlog is on
// screen and which workspace the pane lives in — "cats + global · ws:pers".
// The project basename leads because it is the fact that decides what an edit
// touches; the workspace label used to stand in for it entirely, which masked
// launches that had landed in the wrong directory.
//
// room is the columns the note may occupy (room < 0: unbudgeted, before the
// first resize). Inside that budget the workspace label gives way first —
// truncated to the room left, dropped when even that is too little — and the
// project name itself is cut only as the last resort, keeping its suffix:
// "…which backlogs am I editing" outranks the tail of a directory name the
// user already knows. The old header never cut the name at all, which let a
// long directory overflow the line and push every click target down a row.
func (m model) scopeNote(room int) string {
	// A --project launch with no project has no store at all; "global only" is
	// the ordinary no-project header. Naming a backlog that is not on screen
	// is exactly the confusion the only-modes exist to remove.
	if !m.project.available() {
		note := "global only"
		if !m.global.available() {
			note = "no backlog here"
		}
		if room >= 0 {
			note = truncate(note, room)
		}
		return note
	}

	project := firstNonEmpty(baseName(m.ctx.projectDir()), "project")
	// A project-only launch (--project) has no global store; saying "+ global"
	// would be the exact confusion the mode exists to remove.
	suffix := " + global"
	if !m.global.available() {
		suffix = " only"
	}
	ws := m.ctx.WorkspaceLabel
	// A workspace named after the project adds nothing over the project name;
	// skip the suffix rather than print "cats + global · ws:cats".
	if ws == "" || ws == project {
		ws = ""
	}
	const sep = " · ws:"

	if room < 0 {
		if ws != "" {
			return project + suffix + sep + ws
		}
		return project + suffix
	}

	coreW := len([]rune(project + suffix))
	if ws != "" {
		if wsRoom := room - coreW - len([]rune(sep)); wsRoom >= 2 {
			return project + suffix + sep + truncate(ws, wsRoom)
		}
		// No room for even one rune plus the ellipsis — the label goes.
	}
	if coreW <= room {
		return project + suffix
	}
	if pw := room - len([]rune(suffix)); pw >= 2 {
		return truncate(project, pw) + suffix
	}
	return truncate(project, max(room, 0))
}

// hiddenNote is the header's " · N done hidden" tag while the done fold is on.
func (m model) hiddenNote() string {
	if !m.hideDone {
		return ""
	}
	if n := m.hiddenDoneCount(); n > 0 {
		return fmt.Sprintf(" · %d done hidden", n)
	}
	return ""
}

// headerLine renders the list view's single top line from the current budget:
// chip, scope note, inline query box, match count. The glyph in front of the
// input is the focus affordance the boxed field's rails used to be: lit when
// the box holds the keys, faint when a button does. The input's own focus is
// the truth here, same as the picker's rails — setActionFocus moves both.
func (m model) headerLine() string {
	hl := m.headerLayout()
	var b strings.Builder
	b.WriteString(headerChip(hl.withVersion))
	if hl.note != "" {
		b.WriteString("  ")
		b.WriteString(descStyle.Render(hl.note))
	}
	if hl.searchW > 0 {
		b.WriteString("  ")
		glyph := searchGlyphOffStyle
		if m.list.input.Focused() {
			glyph = promptStyle
		}
		b.WriteString(glyph.Render(searchGlyph))
		b.WriteString(m.list.input.View())
	}
	b.WriteString("  ")
	// Right-aligned in its reserved cells, so "9/10" collapsing to "3/10" as
	// the user types doesn't make the line's tail wobble.
	b.WriteString(countStyle.Render(fmt.Sprintf("%*s", hl.countW, hl.count)))
	return b.String()
}

func (m model) viewList() string {
	var b strings.Builder
	b.WriteString(m.headerLine())
	b.WriteString("\n\n")
	b.WriteString(m.actionBar())
	b.WriteString("\n")

	// The empty list is where "there is nowhere to write" has to be said: with
	// no backlog available, enter is not the answer and pointing at it would
	// send the user in a circle.
	empty := "No prompts yet — press enter to add one."
	if !m.project.available() && !m.global.available() {
		empty = "No backlog here — this pane is not in a project. Relaunch from a project directory, or with --global."
	}
	b.WriteString(m.list.rowsView(empty, m.width))

	// The status row is always there — empty when quiet — so the footer sits
	// still instead of jumping two lines every time a message lands or clears.
	b.WriteString("\n")
	if m.status != "" {
		st := okStyle
		if m.statusErr {
			st = errStyle
		}
		b.WriteString(st.Render("• " + m.status))
	}
	b.WriteString("\n\n")
	b.WriteString(m.listFooter())
	return b.String()
}

// listFooter is the help line under the list. While the action bar is wide
// enough to teach its own chords (barShowsHints), the footer only names the
// rest — repeating "ctrl+a add" one line under a chip that says exactly that
// taught nothing and cost a line of attention. In a pane too narrow for chip
// hints the footer is the only teacher left, so it names everything, on the
// same two lines it always used.
func (m model) listFooter() string {
	if !m.barShowsHints() {
		// The pointer's gesture rides along with the chord it stands for — a
		// double-click is a guess worth confirming, not one worth making blind.
		return footerStyle.Render("enter / dbl-click edit · "+m.modEnter()+" drop · ctrl+v view · ctrl+a add · ctrl+t done · ctrl+x delete") +
			"\n" +
			footerStyle.Render("ctrl+s schedule · tab buttons · ctrl+↑/↓ move · ctrl+d hide/show done · ctrl+w clear done · esc quit")
	}
	segs := []string{
		"ctrl+s schedule", "ctrl+v view", "ctrl+t done", "ctrl+↑/↓ move",
		"ctrl+d hide done", "ctrl+w clear done", "esc quit",
	}
	line := strings.Join(segs, " · ")
	// Concede from the right rather than wrap: the footer sits below the mouse
	// map, so a wrapped line only costs looks — but it costs them every frame.
	for m.width > 0 && len([]rune(line)) > m.width && len(segs) > 1 {
		segs = segs[:len(segs)-1]
		line = strings.Join(segs, " · ")
	}
	return footerStyle.Render(line)
}

// actionBar renders the row of buttons under the filter. A button whose action
// needs a highlighted prompt is greyed when there isn't one, so the bar also
// answers "why did nothing happen".
//
// The key hints ride inside the chips whenever the row fits with them — a
// button that names its shortcut is a lesson, not just a target. In a pane too
// narrow for that, the labels are what has to survive; the footer still names
// every chord.
func (m model) actionBar() string {
	acts := m.listActions()
	_, hasSel := m.selectedRef()

	var b strings.Builder
	b.WriteString(strings.Repeat(" ", indentWidth))
	for i, c := range m.actionChips() {
		if i > 0 {
			b.WriteString(" ")
		}
		// Derived, never nested: each chip is still one style, the hue dropped
		// into it rather than wrapped around it.
		st := btnStyle.Foreground(lipgloss.Color(acts[i].tint))
		switch {
		case m.actionFocus && i == m.actionIdx:
			st = btnFocusStyle.Background(lipgloss.Color(acts[i].tint))
		case acts[i].needsSel && !hasSel:
			st = btnOffStyle
		}
		b.WriteString(st.Render(c.text))
	}
	return b.String()
}

// actionChip is one rendered button: its text, and the half-open column span
// [start, end) it occupies on the bar's row. The span is what turns a click at
// (x, y) back into an action index, so it is computed here rather than in the
// render loop — one description of the layout, used to draw it and to hit-test
// it, so a click can't land on a button the eye doesn't see.
type actionChip struct {
	text       string
	start, end int
}

// actionChips lays the bar out: the chip contents (label, plus the key hint
// when the whole row fits with them) and where each one lands. The three button
// styles share one padding, so widths don't depend on which is picked at render
// time and btnStyle can measure for all of them.
func (m model) actionChips() []actionChip {
	acts := m.listActions()
	withHints := m.barShowsHints()

	chips := make([]actionChip, 0, len(acts))
	x := indentWidth
	for i, a := range acts {
		if i > 0 {
			x++ // the space between chips
		}
		text := a.label
		if withHints {
			text = a.label + " " + a.hint
		}
		w := lipgloss.Width(btnStyle.Render(text))
		chips = append(chips, actionChip{text: text, start: x, end: x + w})
		x += w
	}
	return chips
}

// barShowsHints reports whether the action bar is wide enough for every chip
// to carry its key hint. The footer keys off the same answer — chords the
// chips teach stay off it, and the moment the bar goes quiet the footer names
// everything again — so the two can't both drop a chord at once.
func (m model) barShowsHints() bool {
	if m.width <= 0 {
		return true
	}
	w := indentWidth
	for _, a := range m.listActions() {
		w += lipgloss.Width(btnStyle.Render(a.label+" "+a.hint)) + 1
	}
	return w <= m.width
}

// indentWidth is the left margin the list's rows sit at (the two columns
// cursorGlyph occupies), so the action bar lines up with them.
const indentWidth = 2

// headerRow is the line the title chip, scope note and query box share — the
// top of the list view. A click here hands the keys back to the query box.
const headerRow = 0

// actionBarRow is the row the bar is drawn on, counting from the top of the
// list view: the header (0), a blank (1), the bar (2). Each of those is
// exactly one line — headerLayout budgets the header so it cannot wrap — so a
// click's Y can be compared against a constant instead of the view being
// re-measured. TestActionBarRow finds the bar in the rendered frame and fails
// if the layout above it ever grows a line.
const actionBarRow = 2

// listRowsRow is the first line the todo rows are drawn on, directly under the
// bar. A grouped list opens with its own blank-and-heading, so the first
// backlog row keeps its breathing room either way.
// TestListRowsMatchWhatIsDrawn holds it to the frame.
const listRowsRow = actionBarRow + 1

// targetRowsRow is the first line the drop picker's target rows are drawn on,
// counting from the top of that view: the heading (0), a blank (1), the filter
// line (2), a blank (3), then the rows. The picker draws no action bar, so the
// rows sit where the bar does on the list.
const targetRowsRow = 4

func (m model) viewForm() string {
	var b strings.Builder
	heading := "New prompt — " + m.formScope.String() + " backlog"
	if m.formMode == formEdit {
		heading = "Edit prompt"
	}
	b.WriteString(titleStyle.Render(heading))
	b.WriteString("\n\n")

	b.WriteString(promptStyle.Render("Title"))
	b.WriteString("\n")
	b.WriteString(m.titleInput.View())
	b.WriteString("\n\n")

	b.WriteString(promptStyle.Render("Prompt"))
	b.WriteString("\n")
	b.WriteString(m.promptArea.View())
	b.WriteString("\n")

	// The attachment line is always shown, empty or not: an attachment editor
	// nobody knows about is one nobody uses, and "none" is also the answer to
	// "did my screenshot make it in".
	b.WriteString(descStyle.Render("📎 " + firstNonEmpty(m.imageCountNote(), "no images") + " — ctrl+o to attach"))
	b.WriteString("\n")

	if m.formErr != "" {
		b.WriteString(errStyle.Render(m.formErr))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	help := "enter save · " + m.modEnter() + " newline · tab switch field · ctrl+o images · esc cancel"
	// Advertise the scope toggle only when it works (see the ctrl+g handler):
	// an only-mode launch pins the scope, so the hint would be a lie there.
	if m.formMode == formAdd && m.project.available() && m.global.available() {
		help = "enter save · " + m.modEnter() + " newline · tab switch · ctrl+o images · ctrl+g scope · esc cancel"
	}
	b.WriteString(footerStyle.Render(help))
	return b.String()
}

// viewImages renders the attachment editor: the path box, then one row per
// attachment. A pending row shows where it will be copied from, an existing row
// shows where it already lives — the distinction decides what saving will do, so
// it is on screen rather than implied.
func (m model) viewImages() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Attachments"))
	b.WriteString("  ")
	b.WriteString(descStyle.Render(firstNonEmpty(m.imageCountNote(), "none yet")))
	b.WriteString("\n\n")

	b.WriteString(promptStyle.Render("❯ "))
	b.WriteString(m.imgInput.View())
	b.WriteString("\n\n")

	if len(m.formImages) == 0 {
		hint := "  nothing attached yet — paste a path above, or press ctrl+r for your latest screenshot"
		if clipboardImageSupported() {
			hint = "  nothing attached yet — ctrl+v pastes a copied image, ctrl+r finds your latest screenshot"
		}
		b.WriteString(descStyle.Render(hint))
		b.WriteString("\n")
	}
	for i, img := range m.formImages {
		if i == m.imgCursor {
			b.WriteString(cursorStyle.Render(cursorGlyph))
			b.WriteString(nameSelStyle.Render(img.name))
		} else {
			b.WriteString("  ")
			b.WriteString(nameStyle.Render(img.name))
		}
		b.WriteString("  ")
		switch {
		case img.missing:
			b.WriteString(errStyle.Render("missing — " + img.rel))
		case img.rel != "":
			b.WriteString(descStyle.Render("attached · " + img.rel))
		case img.pasted:
			// The temp path is this process's plumbing, not something the user
			// chose or can act on — where it came from is the useful fact.
			b.WriteString(descStyle.Render("new · pasted from the clipboard"))
		default:
			b.WriteString(descStyle.Render("new · " + img.src))
		}
		b.WriteString("\n")
	}

	if m.imgStatus != "" {
		st := okStyle
		if m.imgStatusErr {
			st = errStyle
		}
		b.WriteString("\n")
		b.WriteString(st.Render("• " + m.imgStatus))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	// Split across two lines to survive an 80-column pane, with the keys that
	// put an image in on the first. The paste key is only advertised where it can
	// work — see clipboardImageSupported.
	paste := ""
	if clipboardImageSupported() {
		paste = " · ctrl+v paste image"
	}
	b.WriteString(footerStyle.Render("enter attach (empty: back)" + paste + " · ctrl+r recent screenshot"))
	b.WriteString("\n")
	b.WriteString(footerStyle.Render("↑/↓ select · ctrl+x remove · esc back — nothing is copied until you save"))
	return b.String()
}

func (m model) viewConfirm() string {
	var b strings.Builder
	if m.confirmKind == confirmClearDone {
		b.WriteString(titleStyle.Render("Clear completed prompts?"))
		b.WriteString("\n\n")
		noun := "prompts"
		if m.pendingClearCount == 1 {
			noun = "prompt"
		}
		b.WriteString(nameStyle.Render(fmt.Sprintf("  delete %d completed %s across both backlogs", m.pendingClearCount, noun)))
		b.WriteString("\n\n")
		b.WriteString(footerStyle.Render("y clear · n / esc cancel"))
		return b.String()
	}
	b.WriteString(titleStyle.Render("Delete prompt?"))
	b.WriteString("\n\n")
	b.WriteString(nameStyle.Render("  " + truncate(m.pendingTitle, 70)))
	b.WriteString("\n\n")
	b.WriteString(footerStyle.Render("y delete · n / esc cancel"))
	return b.String()
}

// viewPrompt renders the read-only prompt view: heading, meta line, the full
// prompt body in a scrollable viewport, and a footer of actions.
func (m model) viewPrompt() string {
	td, ok := m.resolve(m.viewRef)
	if !ok {
		// Deleted from another pane while we were viewing it — fall back gently.
		return descStyle.Render("that prompt no longer exists — esc to go back")
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Prompt"))
	b.WriteString("  ")
	b.WriteString(nameSelStyle.Render(truncate(firstNonEmpty(td.Title, firstLine(td.Prompt, 60)), 60)))
	b.WriteString("\n")

	meta := m.viewRef.scope.String() + " backlog"
	if !td.Created.IsZero() {
		meta += " · added " + td.Created.Format("2006-01-02")
	}
	if td.Done {
		meta += " · " + checkStyle.Render("done")
	}
	if sc := td.Schedule; sc != nil && !td.Done {
		when := "scheduled " + formatScheduleTime(sc.At, time.Now())
		if sc.Missed {
			meta += " · " + errStyle.Render("missed "+formatScheduleTime(sc.At, time.Now()))
		} else {
			meta += " · " + schedStyle.Render(when)
		}
	}
	b.WriteString(descStyle.Render(meta))
	b.WriteString("\n\n")

	b.WriteString(m.viewVP.View())
	b.WriteString("\n\n")
	b.WriteString(footerStyle.Render("↑/↓ scroll · enter edit · " + m.modEnter() + " drop · esc back"))
	return b.String()
}

func (m model) viewTarget() string {
	var b strings.Builder
	td, _ := m.resolve(m.dropTodo)
	title := firstNonEmpty(td.Title, firstLine(td.Prompt, 50))
	heading, foot := "Drop into…", "enter paste (don't submit) · "+m.modEnter()+" drop & run · esc back"
	if m.pickForSchedule {
		// Same picker, schedule flavor: enter stores the target instead of
		// firing, and the paste/run split disappears — a fire with nobody at
		// the keyboard always runs.
		heading = "Schedule drop into… (" + formatScheduleTime(m.schedAt, time.Now()) + ")"
		foot = "enter schedule · esc back"
	}
	b.WriteString(titleStyle.Render(heading))
	b.WriteString("  ")
	b.WriteString(descStyle.Render(truncate(title, 60)))
	b.WriteString("\n\n")
	b.WriteString(m.targetList.view("no agent sessions detected — pick New Claude Code session", "", m.width))
	b.WriteString("\n")
	b.WriteString(footerStyle.Render(foot))
	return b.String()
}

// applySizes pushes the current window size into the inputs of the active stage
// so they wrap and scroll to fit. Only the live stage's components are touched:
// the form's textinput/textarea and the target picker are zero-value until their
// stage is entered (built in beginAdd/beginEdit/beginDrop), and calling a method
// like textarea.SetWidth on a zero-value model dereferences nil internal state.
func (m *model) applySizes() {
	// The query input is budgeted by headerLayout, which self-clamps to narrow
	// panes — so it is sized before, and regardless of, the floor below.
	m.sizeSearchInput()
	w := m.width - 4
	if w < 20 {
		return
	}
	switch m.stage {
	case stageForm:
		m.titleInput.SetWidth(w)
		m.promptArea.SetWidth(w)
		if h := m.height - 12; h >= 4 {
			m.promptArea.SetHeight(h)
		}
	case stageTarget:
		m.targetList.input.SetWidth(w)
	case stageImages:
		m.imgInput.SetWidth(w)
	case stageSchedule:
		m.schedInput.SetWidth(w)
	case stageView:
		m.viewVP.SetWidth(m.viewWidth())
		m.viewVP.SetHeight(m.viewHeight())
		if td, ok := m.resolve(m.viewRef); ok {
			m.viewVP.SetContent(m.viewContent(td))
		}
	}
}

// baseName returns the last path element of p, or "" for an empty/"." path.
func baseName(p string) string {
	if strings.TrimSpace(p) == "" {
		return ""
	}
	b := filepath.Base(p)
	if b == "." || b == string(filepath.Separator) {
		return ""
	}
	return b
}
