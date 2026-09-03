package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
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
	"github.com/rohanthewiz/cats-todo/internal/spell"
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
	stageSession                 // edit the form's per-todo session options
	stageFiles                   // browse the file system for an @mention in the prompt, or a folder to export to
	stageSnippets                // pick a prompt (or a slash command) out of the user's library to insert
	stageExport                  // pick another project's backlog to copy / move the chosen prompt into
	stageSpell                   // correct, or accept, the misspelled word nearest the prompt's caret
	// stageViewOpts is the list's View panel: how the list is drawn, as against
	// stageView above, which is one prompt's text. The names are close because
	// the words are; stageView is the older of the two and renaming it to
	// stagePrompt (which is what its renderer is already called) would be tidier
	// but is a separate change.
	stageViewOpts
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
	// worktree asks for a fresh git checkout to launch in, rather than the
	// project's own tree (see worktree.go). A flag on targetNewSession rather
	// than a third kind: every step of the drop is identical, down to the
	// waiting and the paste — only the directory the tab is rooted at differs.
	worktree bool
	label    string
	desc     string
}

// dropMode is the per-drop submit choice. Dropping a prompt is asking for the
// work to start, so dropRun — type it, then press Enter — is what the picker
// does unless told otherwise. dropPaste is the opt-in "pause after drop": the
// prompt lands in the agent's input and stops there, for the times it wants one
// last read (or a line of context only you can add) before it goes.
//
// dropPaste stays the zero value on purpose. A pendingAction assembled without
// naming a mode should be the one that starts nothing by itself; every path
// that means to run says so.
type dropMode int

const (
	dropPaste dropMode = iota // type the prompt and stop — you press Enter
	dropRun                   // type the prompt and submit it (the default)
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
	// anchorPane is the manager's own pane id (0 when unresolved), naming the
	// repo a worktree drop branches from — worktree.create reads the addressed
	// pane's working directory. Captured at dispatch alongside cwd, and for the
	// same reason: the drop goroutine holds no RunContext. Only a worktree
	// target reads it.
	anchorPane uint32
}

// dropResultMsg reports the outcome of an asynchronous drop back to the Update
// loop, so the manager can clear its "dropping…" state and show where the prompt
// landed (or why it failed) while staying open.
type dropResultMsg struct {
	desc string  // human description of the destination, for the status line
	ref  todoRef // the dropped todo, so a successful drop can auto-mark it done
	err  error
	// mode is carried back only so the success line can say whether the prompt
	// is already running or is sitting in the agent's input waiting on an Enter
	// that only the user can press. A paused drop that reported itself the same
	// way a run does is a prompt that quietly never starts.
	mode dropMode
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
	list fuzzyList
	rows []todoRef // selectable row index -> todo
	// hideDone folds completed prompts out of the list, and showFrozen says
	// whether the "will not do" ones are drawn at all.
	//
	// These were one flag (hideClosed) until the View panel gave frozen prompts
	// a switch of their own, because the two turn out to be different questions.
	// "Hide what is finished" is a tidy-up, reached for when the done pile is in
	// the way and reset by the next launch. "Show frozen at all" is a standing
	// decision about whether the record of what was declined is worth its rows,
	// which is why only that one is a saved preference (see settings.go).
	//
	// ctrl+d still moves both together, so the one key that has always meant
	// "show me what is left to do" still means exactly that — the panel is the
	// fine control, not a replacement for it.
	hideDone bool
	// orderByPriority is the View panel's other toggle: the list is drawn
	// critical-first inside each group rather than in the order the file holds.
	// A lens over the backlog and never a rewrite of it — see rebuildList.
	orderByPriority bool
	showFrozen      bool
	// The View panel's cursor, and its one line of feedback — which is only
	// ever a settings write that failed. The toggle itself always takes; it is
	// the memory of it that can be lost, and that split is worth saying out
	// loud rather than swallowing (the same report toggleSpell makes).
	viewOptsCursor int
	viewOptsNote   string
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
	// Drag-to-reorder. drag is the todo the button went down on and dragging says
	// it is still down, so the motion the terminal reports between the two is
	// known to belong to that row rather than to whatever it happens to be over.
	// dragMoved records that a reorder actually happened during the gesture,
	// which is what stops a drag from also counting as half of a double-click:
	// press, move, release is one gesture, and pairing its click with the next
	// one would open a form the user never pointed at.
	//
	// The state is deliberately not a copy of the todo. A drag saves to disk on
	// every step, so the row under the hand is re-read from the store each time;
	// holding a stale copy of it would be a way to write back a prompt another
	// pane had edited mid-gesture.
	drag      todoRef
	dragging  bool
	dragMoved bool
	// The list's context menu (listmenu.go) — right-click, and everything that
	// can be done to the todo under the pointer. Its zero value is "closed",
	// which is what lets every caller test one field; like the editor's it lives
	// and dies with the gesture that opened it.
	listMenu listMenu
	// The list's hover card (listhover.go) — what the prompt under the pointer
	// says, without leaving the list to find out. Like the menu its zero value
	// is "not showing", and it lives and dies with the gesture: the pointer
	// leaving the row takes it down, and so does anything the hand does next.
	hover hoverCard

	// Form stage.
	formMode   formMode
	formScope  scope
	editID     string
	titleInput textinput.Model
	promptArea textarea.Model
	formFocus  int // the formField* stop holding the keys (title, prompt, annotation bar)
	formErr    string
	// formNote is the form's one non-error message, drawn on the same line
	// formErr uses and yielding to it when both are set. Today it says a
	// selection was copied. It is not folded into formErr because a save failure
	// and a successful copy must not look alike, which is the same split the
	// attachment editor draws between imgStatus and imgStatusErr.
	formNote string
	// The prompt editor's text selection: the anchor a shift+motion or a drag
	// started from (see promptsel.go), and whether the pointer is still down on
	// a selection drag. promptSelDrag is separate from the list's dragging flag
	// because the two gestures live on different stages and mean different
	// things — one reorders a backlog, the other sweeps a highlight — and one
	// flag serving both would let a release meant for one end the other.
	promptSel     promptSel
	promptSelDrag bool
	// The editor's context menu (promptmenu.go) — right-click, and what the
	// swept run is worth. Its zero value is "closed", which is what lets every
	// caller test one field; it lives and dies with the gesture that opened it.
	menu promptMenu
	// The editor's column mode (promptcarets.go): a caret on each of the swept
	// lines, and typing that lands on all of them. It is on the model rather
	// than inside promptSel because it *replaces* a selection rather than
	// decorating one — dropping the carets clears the highlight, and clearing
	// the highlight ends the mode.
	carets promptCarets
	// pendingPaste marks that a Cmd+V asked the terminal for the clipboard over
	// OSC 52 and is waiting for the tea.ClipboardMsg carrying it. Only that one
	// message may paste, which is what this flag is for: the reply is
	// indistinguishable from an unsolicited clipboard report, and only the
	// request knows one was wanted.
	pendingPaste bool
	// Spell check (see spell.go): whether it is on — a persisted preference,
	// read at start-up and flipped by ctrl+l on the form — and the dictionary,
	// nil until the first form opens with the check on. On the model rather
	// than a package variable because the tests build many models against
	// different config directories, and a dictionary is per-launch state.
	spellOn   bool
	spellDict *spell.Dictionary
	// The Spelling panel — the form's fourth sub-stage, opened by ctrl+l (see
	// spellpanel.go). spellSpan and spellWord are the flagged word it opened on,
	// resolved once so the row that names a word and the text a correction
	// replaces cannot come from two different reads of the value; spellChoices
	// is what each row does, indexed by the row's ref. Rebuilt on every open,
	// so nothing here outlives the panel.
	spellList    fuzzyList
	spellChoices []spellChoice
	spellWord    string
	spellSpan    spell.Span
	// spellPick is the word a right-click in the editor pointed at, and
	// spellPicked says the panel was opened that way. The pointer names its own
	// target — the panel opened by ctrl+l has to guess one from the caret
	// (promptSpellTarget), and a gesture aimed at a particular word must not be
	// answered with a different one. It also decides which row opens
	// highlighted: a right-click on a word means "this is a word", so the
	// highlight starts on the ✚ Add row rather than the first correction.
	// Cleared when the panel closes, so it never outlives the gesture.
	spellPick   spell.Span
	spellPicked bool
	// spellErr is the panel's own message line, for the one thing that can go
	// wrong while it is up: a dictionary file that could not be written. It is
	// not the form's formErr because the panel is what is on screen when it
	// happens, and an error the user cannot see until they leave is one they
	// will act on too late.
	spellErr string

	// File picker — the form's third sub-stage, opened by '@' in the prompt
	// (see filepick.go). Rebuilt on every open, so nothing here outlives the
	// gesture that asked for it.
	files filePicker

	// Prompt-library picker — the form's fourth sub-stage, opened by ctrl+p
	// (cmd+P) or by a '/' at the start of a line (see promptpick.go). Rebuilt on
	// every open over a freshly read library, so nothing here outlives the
	// gesture that asked for it and a hand-edited file is never stale.
	snips snippetPicker

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

	// Session options editor (the form's other sub-stage, so its state lives and
	// dies with the form's the way the attachment editor's does).
	//
	// formSession is the whole record being edited — seeded from the todo in
	// beginEditRef, written back by saveForm. It is held by value rather than as
	// a pointer so that cancelling the form cannot have touched the stored one.
	formSession SessionOpts
	// formAnnots is the prompt's annotations while the form holds them — its
	// priority and its low-hanging-fruit mark (see annotations.go). A field of
	// its own rather than a member of formSession, because they are not session
	// options: they say what is true about the prompt, not how the agent that
	// reads it will be set up, and they are stored on the Todo rather than in its
	// Session record. They are edited on the form's own annotation bar
	// (annotbar.go), not in the ⚙ panel.
	//
	// Held by value for the same reason formSession is copied: an abandoned form
	// must leave the stored annotations exactly as they were.
	formAnnots annots
	// annotCursor is the annotation bar's own cursor — which segment ←/→ and
	// space act on while the bar holds the form's focus (see annotbar.go).
	annotCursor int
	sessCursor  int
	// sessInput is the box the two free-text rows share: the context argument
	// on one, the file being added on the other. One input rather than two
	// because only ever one row holds the keys, and the value is committed to
	// formSession as the cursor leaves the row (see moveSessCursor).
	sessInput textinput.Model
	// sessSkills records whether this machine has the sess-* commands
	// installed, probed once when the panel opens rather than on every frame —
	// the answer cannot change while a panel is up, and the check is three
	// stats. It only greys the context rows; it never blocks a save.
	sessSkills    bool
	sessStatus    string
	sessStatusErr bool

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

	// Export stage (see export.go). exportRef is the todo being sent, and the
	// list is rebuilt on every open — which projects are open, and what their
	// backlogs hold, are facts read fresh each time.
	exportRef     todoRef
	exportTargets []exportTarget
	exportList    fuzzyList

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
// list and view. (The form no longer needs it — enter is the newline there
// again — but the chord stays bound in the editor beside it.) shift+enter is the one to show
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
	// Preferences before the list is built, not after: rebuildList reads both
	// of the View panel's toggles, so a load that came second would draw one
	// list from the defaults and only pick up the saved view on the next
	// keystroke that happened to rebuild.
	pref := loadSettings()
	m.spellOn = pref.spellcheck
	m.orderByPriority, m.showFrozen = pref.orderByPriority, pref.showFrozen
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
		// A context menu is placed against the pane it was opened in (see
		// menuBox.place), so a resize leaves its box aimed at cells that may no
		// longer exist. It is a transient answer to a press, not state worth
		// re-fitting: it goes, and the next press gets one that fits. Both of
		// them, since a resize can arrive on either stage.
		m.menu = promptMenu{}
		m.listMenu = listMenu{}
		// And the hover card with them, for the same reason and one more: it was
		// placed against a row that has just been re-laid-out, so it would be
		// naming a prompt that is no longer under it.
		m.clearHover()
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
		if msg.mode == dropPaste {
			// Paused: the prompt is delivered but nothing is running yet, and
			// the only place that can be said is here — the agent's pane looks
			// exactly like a session someone typed into and walked away from.
			status = "pasted → " + msg.desc + " · press enter there to run"
		}
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
	case tea.MouseMotionMsg:
		// A held button makes this a drag, which is answered only when the button
		// went down on a todo row or inside the prompt editor.
		if m.dragging {
			return m.dragOver(msg)
		}
		if m.promptSelDrag {
			return m.promptSelOver(msg)
		}
		// Otherwise it is the pointer moving with nothing held. Everywhere but
		// the list that is a message the terminal was never asked for
		// (MouseModeCellMotion reports motion only under a button — see View),
		// so it falls through to the active input as it always has. The list
		// asks for all motion, and this is where its hover card is built.
		if m.stage == stageList {
			return m.hoverMotion(msg)
		}
	case tea.MouseReleaseMsg:
		if m.dragging {
			return m.endDrag()
		}
		if m.promptSelDrag {
			// The anchor stays behind: the sweep is over, but what it selected is
			// still selected, which is the only reason to have swept.
			m.promptSelDrag = false
			return m, nil
		}
	case tea.ClipboardMsg:
		// The terminal's answer to the OSC 52 read pasteFormClipboard asks for
		// when there is no local pasteboard. It is consumed only by the request
		// that asked for it: a terminal may volunteer a clipboard report of its
		// own, and text appearing in a prompt nobody asked to paste into is the
		// one outcome worse than a paste that does not arrive.
		if !m.pendingPaste {
			return m, nil
		}
		m.pendingPaste = false
		if m.stage != stageForm || msg.Content == "" {
			return m, nil
		}
		// The selection, if there was one, was already taken out by the chord
		// that started this (see updateForm); the caret is sitting where it was.
		return m.forwardForm(tea.PasteMsg{Content: msg.Content})
	case tea.PasteMsg:
		// A bracketed paste is an insertion like a typed character, so it
		// replaces a standing selection rather than landing beside it. The
		// editor is the only place here that has one, which is why it is the
		// only stage named; everything else falls through to forward unchanged.
		if m.stage == stageForm && m.formFocus == formFieldPrompt {
			if m.carets.on {
				// Every caret takes it, the way every caret takes a typed
				// character — a paste is an insertion, and the mode is about
				// where insertions land.
				m.insertAtCarets(msg.Content)
				m.formNote = m.caretNote()
				return m, nil
			}
			m.deletePromptSelection()
		}
		return m.forward(msg)
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
		case stageSession:
			return m.updateSession(msg)
		case stageFiles:
			return m.updateFiles(msg)
		case stageSnippets:
			return m.updateSnippets(msg)
		case stageExport:
			return m.updateExport(msg)
		case stageSpell:
			return m.updateSpell(msg)
		case stageViewOpts:
			return m.updateViewOpts(msg)
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
	case stageSession:
		m.sessInput, cmd = m.sessInput.Update(msg)
	case stageFiles:
		// The blink and a paste both land here; edit is the one way into the
		// picker's query, so a pasted path is normalized like a typed one.
		cmd = m.files.edit(msg)
	case stageSnippets:
		// The blink and a paste both land here; the query box is the only thing
		// on the screen that takes text, so both go straight to it.
		cmd = m.snips.list.editQuery(msg)
	case stageExport:
		cmd = m.exportList.editQuery(msg)
	case stageSpell:
		cmd = m.spellList.editQuery(msg)
	}
	return m, cmd
}

// --- List stage ---------------------------------------------------------------

func (m model) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// An open context menu takes the keys first and owns all of them, the same
	// bargain the editor's makes (see updatePromptMenu): a menu is up, and the
	// list's own chords would otherwise act on a row from behind it.
	if m.listMenu.open {
		return m.updateListMenu(msg)
	}
	// The hand is on the keyboard, so the pointer is not what the eye is
	// following: the hover card goes before the key is even read. A card left
	// standing over a list the keys are moving through would be describing a row
	// the highlight has already left.
	m.clearHover()
	// A keystroke ends any drag still thought to be in progress. The release
	// that should have ended it can genuinely go missing — a button let go
	// outside the pane, a terminal that reports presses but not releases — and
	// the hand is demonstrably on the keyboard now, so a grip left drawn in the
	// gutter would be a lie nothing else was going to correct.
	if m.dragging {
		m.releaseDrag()
	}
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
	case "ctrl+f":
		return m.freezeSelected()
	case "ctrl+l":
		return m.beginViewOpts()
	case "ctrl+v":
		return m.beginView()
	case "ctrl+d":
		return m.toggleClosedFold()
	case "ctrl+s":
		return m.beginSchedule()
	case "ctrl+w":
		return m.beginClearDone()
	case "ctrl+o":
		return m.beginExport()
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
// action bar on the list, a target row in the drop picker, the fields and the
// toolbar on the form. Everything else ignores the pointer.
//
// A click always moves the keyboard's place onto what was clicked before acting,
// so the two ways in leave the manager in the same state — the pointer puts the
// focus where the eye already is, rather than acting from one place while tab or
// ↑/↓ resumes from another.
func (m model) updateMouse(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	// Any press answers the card: the question it was floating ("what is this
	// one?") has been answered by the hand doing something about it, and a card
	// left up would sit over the menu or the form the press is about to open.
	m.clearHover()
	if msg.Button == tea.MouseRight {
		// The right button means one thing on the two screens that have a menu's
		// worth of answers to give: on the form, what a swept run of the prompt
		// can be turned into (promptmenu.go); on the list, everything that can be
		// done to the todo under the pointer (listmenu.go). Both are worth the
		// exception to "the left button is the pointer" for the same reason — the
		// actions are more numerous than any bar or chord can teach, and a menu
		// is where every other program on the machine keeps that list.
		switch m.stage {
		case stageForm:
			return m.rightClickForm(msg)
		case stageList:
			return m.rightClickList(msg)
		}
		return m, nil
	}
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	// A fresh press ends whatever the last one was holding, for the same reason
	// a keystroke does (see updateList): the release may never have arrived, and
	// a button going down is proof the one before it came up. clickRow re-arms
	// from here when the press lands on a row.
	m.releaseDrag()
	switch m.stage {
	case stageList:
		// An open menu takes the press first, wherever it landed: a click off the
		// box dismisses it, which is the gesture every menu answers to and one the
		// list must not also act on.
		if m.listMenu.open {
			return m.clickListMenu(msg)
		}
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
	case stageForm:
		return m.clickForm(msg)
	case stageFiles:
		return m.clickFiles(msg)
	case stageSnippets:
		return m.clickSnippets(msg)
	case stageExport:
		return m.clickExport(msg)
	case stageSpell:
		return m.clickSpell(msg)
	case stageViewOpts:
		return m.clickViewOpts(msg)
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
// off a gesture the hand can make by accident, on the bar's ✉ Send chip and
// shift+enter. Sending with the pointer is two clicks in two places, and that
// deliberateness is the point.
//
// Selecting also hands the bar's focus back to the list, so the pointer and the
// keyboard agree on what the next enter acts on. And it takes hold of the row
// for a possible drag — the third thing a press on a row can turn out to be, and
// the only one that is not decided until the button comes up (see dragOver).
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
	// The button is down on a prompt, so take hold of it: if the pointer moves
	// before it comes up, that motion reorders the backlog (see dragOver). The
	// hold is taken on every press rather than after some threshold of movement —
	// a press that never moves releases with nothing done, and the grip drawn in
	// the gutter is what tells the user the gesture is available at all.
	if ref, ok := m.selectedRef(); ok {
		m.drag, m.dragging, m.dragMoved = ref, true, false
		// The grip is only honest where the drag can act: under a filter the
		// rows are in match order and reordering is refused (see canReorder), so
		// the row shows the ordinary cursor and the refusal is said in words on
		// the first motion.
		m.list.grab = m.canReorder()
	}
	return m, blink
}

// canReorder reports whether the list is showing the backlog's own order, which
// is the only state a drag can honestly act on.
//
// A query does not merely hide rows: fuzzy.Find returns its matches best-score
// first (see fuzzyList.filter), so a filtered list is in relevance order and the
// row above another on screen may be anywhere in the backlog. "Put this one
// there" has no meaning against that — the position the user pointed at is not a
// position the file has — so a drag under a filter is refused in words rather
// than guessing at an array slot.
// Priority order is the same problem arriving from the other direction: the
// rows are in an order the file does not have, so the slot the pointer names is
// again not a slot the array holds.
func (m model) canReorder() bool {
	return strings.TrimSpace(m.list.input.Value()) == "" && m.orderIsBacklogOrder()
}

// orderIsBacklogOrder reports whether the rows are in the file's own order
// rather than under the priority lens.
//
// Split out from canReorder because the two reorder paths are not in the same
// position. A drag names a destination row, so it needs the whole of
// canReorder — under a filter the row above another on screen may be anywhere
// in the backlog. ctrl+↑/↓ names a direction and store.move walks the array,
// which stays coherent under a filter and always has; it is only the priority
// lens that breaks it, by putting the rows in an order the array does not hold.
func (m model) orderIsBacklogOrder() bool { return !m.orderByPriority }

// reorderRefusal names which of the two orders is in the way. On screen they
// look identical — rows in an order the file does not have — and what to do
// about them is different, so the words have to tell them apart.
func (m model) reorderRefusal() string {
	if !m.orderIsBacklogOrder() {
		return "turn priority order off (ctrl+l) to reorder — the list is in priority order, not backlog order"
	}
	return "clear the filter to reorder — a filtered list is in match order, not backlog order"
}

// dragOver is the pointer moving with the button still down: the held prompt is
// moved to whatever row it is now over, one store write per row crossed.
//
// The reorder is expressed as a destination rather than as a direction (see
// store.reorder), so a pointer moved faster than the terminal reports motion —
// three rows in one message — lands where it actually is instead of one step
// along. The highlight is re-parked on the held todo after every move, so the
// cursor rides with the row and the keyboard resumes from it when the drag ends.
//
// The drop is refused, quietly, for anything the list cannot honestly express:
// another backlog (project rows and global rows are separate files, and a drag
// is a reordering gesture, not a move-between-backlogs one) and another render
// group (store.reorder's own check). In both cases the row stays put under the
// pointer, which is the feedback.
func (m model) dragOver(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if m.stage != stageList {
		// The screen changed under the gesture — a key opened a form while the
		// button was still down. Let go rather than reorder a list nobody is
		// looking at.
		m.releaseDrag()
		return m, nil
	}
	i, ok := m.list.rowAtLine(msg.Y - listRowsRow)
	if !ok {
		return m, nil // a heading, a spacer, or the chrome above and below the rows
	}
	idx, ok := m.list.refAt(i)
	if !ok || idx < 0 || idx >= len(m.rows) {
		return m, nil
	}
	over := m.rows[idx]
	if over == m.drag {
		return m, nil // still on the row it is already occupying: nothing to do
	}
	if !m.canReorder() {
		m.setStatus(m.reorderRefusal(), true)
		return m, nil
	}
	if over.scope != m.drag.scope {
		return m, nil
	}
	moved, err := m.storeFor(m.drag.scope).reorder(m.drag.id, over.id)
	if err != nil {
		m.setStatus("move failed: "+err.Error(), true)
		m.releaseDrag()
		return m, nil
	}
	if !moved {
		return m, nil // refused by the store: another render group
	}
	m.dragMoved = true
	m.rebuildList()
	m.selectRow(m.drag)
	return m, nil
}

// endDrag lets go of the dragged row when the button comes up.
func (m model) endDrag() (tea.Model, tea.Cmd) {
	moved := m.dragMoved && m.stage == stageList
	m.releaseDrag()
	if !moved {
		return m, nil
	}
	// The press that started this drag is not the first half of a double-click:
	// it was a grab, and the pointer left the row it landed on. Forgetting it
	// keeps the next click on that row from opening the edit form.
	m.lastClickAt = time.Time{}
	m.setStatus("moved", false)
	return m, nil
}

// releaseDrag drops the hold on the dragged row, including the grip drawn in its
// gutter. Every way a drag can end goes through here so none of them can leave
// the list looking held when it isn't.
func (m *model) releaseDrag() {
	m.dragging, m.dragMoved = false, false
	m.list.grab = false
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
// exactly what enter on that row does — including submitting it. Clicking a
// target is choosing it: the same bargain the action bar makes, where a click
// presses the button rather than merely lighting it. The whole row answers, not
// just the text on it, since asking the user to land on the label would make the
// pointer worse than the keyboard it is there to replace.
//
// The click deliberately carries the same mode as plain enter rather than the
// cautious one. Reaching this row with the mouse already took two considered
// clicks (the prompt, then ✉ Send), so the pointer is not one stray gesture from
// an agent run — and a mouse that quietly did something *different* from the key
// beside it in the footer would be the worse surprise. Pausing is the chord, for
// both hands.
func (m model) clickTarget(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	i, ok := m.targetList.rowAtLine(msg.Y - targetRowsRow)
	if !ok || !m.targetList.focusRow(i) {
		return m, nil
	}
	return m.chooseTarget(dropRun)
}

// rightClickList opens the list's context menu (listmenu.go) on the todo row the
// press landed on.
//
// The press also moves the highlight, the same rule every other click on this
// list obeys: the menu acts on a prompt, so the prompt it acts on is the one the
// keyboard is parked on when the menu hands control back — and the row the box
// is asking about is drawn selected while it is up, which is the only thing on
// screen tying the two together.
//
// Only over a row. A right-click on a heading, a spacer, the header line or the
// action bar opens nothing: a context menu is a question about a thing, and
// there is no thing there to ask about. An open menu closes instead, so a right
// button aimed off the list is still a way out of one.
//
// Unlike a left press this takes no hold for a drag and does not count toward a
// double-click. Both of those are gestures the left button makes, and a right
// press that quietly armed either would leave the list mid-gesture behind a menu
// the user is about to dismiss.
func (m model) rightClickList(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	// A button going down is proof the one before it came up, the same reading
	// the left button and a keystroke both make (see updateList). It matters more
	// here: a hold left armed would hand the next motion message to dragOver,
	// which would reorder the backlog behind an open menu.
	m.releaseDrag()
	i, ok := m.list.rowAtLine(msg.Y - listRowsRow)
	if !ok || !m.list.focusRow(i) {
		m.listMenu = listMenu{}
		return m, nil
	}
	blink := m.setActionFocus(false)
	ref, ok := m.selectedRef()
	if !ok {
		m.listMenu = listMenu{}
		return m, blink
	}
	next, cmd := m.openListMenu(msg, ref)
	return next, tea.Batch(blink, cmd)
}

// rightClickForm opens the editor's context menu (promptmenu.go) on the cell the
// press landed in.
//
// Only inside the editor's box. Everywhere else on the form — the title, the
// annotation bar, the toolbar — the right button does nothing, because nothing
// there has a menu's worth of things to do with it; a box that opened over a
// button row would be answering a question nobody asked.
func (m model) rightClickForm(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if row := msg.Y - formPromptRow; row < 0 || row >= m.promptArea.Height() {
		return m, nil // outside the editor: the pointer has nothing to ask about
	}
	// The keys follow the pointer, the same rule every other click on this form
	// obeys — a menu item acts on the prompt, so the prompt is where the focus
	// should be when the menu hands it back. Set directly rather than through
	// focusForm because nothing here wants a blink command back.
	m.formFocus = formFieldPrompt
	// A press inside the editor while the column mode is up ends it: the mode is
	// a set of carets the pointer is about to disagree with.
	m.endPromptCarets()
	return m.openPromptMenu(msg)
}

func (m model) clickForm(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	// An open menu takes the press first, wherever it landed: a click off the
	// box dismisses it, which is the gesture every menu answers to and one the
	// form must not also act on.
	if m.menu.open {
		return m.clickPromptMenu(msg)
	}
	// Alt on a press inside the editor names *another* caret rather than the
	// only one: it is the pointer's road into the caret mode (promptcarets.go),
	// so it is answered before the clear below would take the standing carets
	// down. Only inside the editor's box — a caret is the one thing alt+click
	// asks for, and only the editor has them.
	if msg.Mod&tea.ModAlt != 0 && msg.Y >= formPromptRow && msg.Y < formPromptRow+m.promptArea.Height() {
		return m.altClickPrompt(msg.X, msg.Y-formPromptRow)
	}
	// A press anywhere on the form ends the last selection: the pointer is
	// about to say where the caret goes next, and a highlight that outlived the
	// click that moved away from it would misreport what ctrl+c copies. The
	// column mode goes with it for the same reason — the pointer is about to
	// name one caret, which is a disagreement with having several.
	m.clearPromptSel()
	m.endPromptCarets()
	m.formNote = ""
	switch {
	case msg.Y == formTitleLabelRow:
		return m, m.focusForm(formFieldTitle)
	case msg.Y == formTitleRow:
		cmd := m.focusForm(formFieldTitle)
		m.placeTitleCursor(msg.X)
		return m, cmd
	case msg.Y == formAnnotRow:
		return m.clickAnnotBar(msg)
	case msg.Y == formPromptLabelRow:
		return m, m.focusForm(formFieldPrompt)
	case msg.Y >= formPromptRow && msg.Y < formPromptRow+m.promptArea.Height():
		cmd := m.focusForm(formFieldPrompt)
		m.placePromptCursor(msg.X, msg.Y-formPromptRow)
		m.anchorPromptSel()
		m.promptSelDrag = true
		return m, cmd
	case msg.Y == m.formBarRow():
		return m.clickFormBar(msg)
	}
	return m, nil
}

// promptSelOver is the pointer sweeping across the editor with the button still
// down: the caret follows it and the anchor stays where the press put it, so the
// span between them grows and shrinks with the hand.
//
// The row is clamped into the editor's box rather than ignored when it leaves
// it. A sweep that runs off the bottom of the prompt on its way to the toolbar
// means "everything down to here", and dropping those messages would freeze the
// highlight partway while the button is plainly still moving. Clamping is also
// what makes a sweep off the top select back to the first visible line.
func (m model) promptSelOver(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	row := min(max(msg.Y-formPromptRow, 0), m.promptArea.Height()-1)
	m.placePromptCursor(msg.X, row)
	return m, nil
}

// placeTitleCursor drops the title field's caret on the column that was clicked.
// The field's own prompt is empty (see newFormInputs), so it starts at column 0
// of its row and the click's X is a column of the value itself.
//
// A title too long for the field scrolls sideways inside it, and bubbles keeps
// that horizontal offset private — so on a scrolled field the click only takes
// the focus and the caret stays where it was. Moving it to a column computed
// from the wrong origin would put it somewhere the user did not point at, and a
// caret in the wrong place is worse than one that didn't move.
func (m *model) placeTitleCursor(x int) {
	runes := []rune(m.titleInput.Value())
	if lipgloss.Width(string(runes)) > m.titleInput.Width() {
		return
	}
	m.titleInput.SetCursor(colAtWidth(runes, x))
}

// colAtWidth turns a cell offset into a rune index in runes: the index of the
// character the offset falls on, clamped to the end of the line. Widths are
// added up rather than counted so a click lands on the same character the eye
// picked even on a line holding double-width glyphs.
func colAtWidth(runes []rune, x int) int {
	if x <= 0 {
		return 0
	}
	w := 0
	for i, r := range runes {
		rw := lipgloss.Width(string(r))
		if w+rw > x {
			return i
		}
		w += rw
	}
	return len(runes)
}

// promptLine identifies one display line of the prompt editor: the logical row
// it belongs to, and the column of the value that row's soft-wrapped segment
// starts at. The pair is what a textarea.LineInfo reports for any caret sitting
// on that line, so it is the key a click's screen row is resolved through.
type promptLine struct {
	row, startCol int
}

// promptLines maps the prompt editor's value to its display lines, in order:
// the slice is display-line index → line, and the map is the inverse.
//
// The textarea soft-wraps, so a display line is not a logical line and the
// mapping cannot be computed from the value alone without reimplementing the
// library's private word-wrap. It is measured instead: a copy of the model is
// walked from the top with the library's own line motion, one display line per
// step, until stepping stops moving.
//
// The copy shares the model's viewport pointer, so the walk scrolls the real
// editor — placePromptCursor is what puts the scroll back, and is the only
// caller for that reason. Everything else the walk touches (the caret's row and
// column) lives in the copy.
func promptLines(ta textarea.Model) ([]promptLine, map[promptLine]int) {
	lines := make([]promptLine, 0, ta.LineCount())
	index := make(map[promptLine]int, ta.LineCount())
	probe := ta
	probe.MoveToBegin()
	// maxLines in the library is 10000; the same order of magnitude here is a
	// backstop for a walk that somehow neither advances nor repeats, not a limit
	// any real prompt reaches.
	for range 20000 {
		key := promptLine{probe.Line(), probe.LineInfo().StartColumn}
		if _, seen := index[key]; seen {
			break // the walk stopped moving: the end of the value
		}
		index[key] = len(lines)
		lines = append(lines, key)
		probe.CursorDown()
	}
	return lines, index
}

// stepPromptTo walks the editor's caret to display line want, one line at a
// time. The step count is not trusted: after every move the caret's own
// LineInfo is looked up in the index, so an off-by-one in the library's wrap
// arithmetic costs a wasted step rather than a caret on the wrong line. A move
// that fails to change the line gives up instead of spinning.
func stepPromptTo(ta *textarea.Model, index map[promptLine]int, want int) {
	at := func() int {
		i, ok := index[promptLine{ta.Line(), ta.LineInfo().StartColumn}]
		if !ok {
			return -1
		}
		return i
	}
	for range len(index) + 1 {
		cur := at()
		if cur < 0 || cur == want {
			return
		}
		if cur < want {
			ta.CursorDown()
		} else {
			ta.CursorUp()
		}
		if at() == cur {
			return
		}
	}
}

// placePromptCursor moves the prompt editor's caret to the character the pointer
// landed on: row is the clicked line counted from the top of the editor's box,
// x its column on the screen.
//
// The textarea has no notion of a click, and the two things a mapping needs —
// which display line the caret is on, and how to jump to another — are private
// (cursorLineNumber, setCursorLineRelative). What is public is one-line-at-a-
// time motion, so the caret is walked there through the display-line table:
//
//	         ┌─ the value, in display lines ──┐
//	   0     │ ...                            │  above the viewport
//	  y0 ───▶│ first visible line             │  ScrollYOffset
//	         │ ...                            │
//	   d ───▶│ the clicked line               │  y0 + row
//	         │ ...                            │
//	last ───▶│ ...                            │
//
// The scroll has to come out of this where it went in, because the click was
// aimed at what is on screen now. Two things move it: building the table (which
// walks to the bottom, leaving the view scrolled there), and the caret itself,
// since the library scrolls to keep it visible. That second behaviour is also
// the lever back: a caret arriving from below lands the view exactly on its own
// line, so stepping to the bottom and then up to y0 restores the offset, and the
// remaining steps down to d stay inside the viewport and move nothing.
func (m *model) placePromptCursor(x, row int) {
	y0 := m.promptArea.ScrollYOffset() // before the table walk scrolls it away
	lines, index := promptLines(m.promptArea)
	if len(lines) == 0 {
		return
	}
	last := len(lines) - 1
	y0 = min(y0, last) // a value shortened since the last scroll
	d := min(y0+max(row, 0), last)

	stepPromptTo(&m.promptArea, index, last)
	stepPromptTo(&m.promptArea, index, y0)
	stepPromptTo(&m.promptArea, index, d)

	// The caret is on the right line; the column is the clicked cell measured
	// from the first character of that line, which sits one gutter in (the
	// textarea's own prompt, "┃ ").
	li := m.promptArea.LineInfo()
	rowRunes := promptRowRunes(m.promptArea)
	start := min(li.StartColumn, len(rowRunes))
	avail := min(li.Width, len(rowRunes)-start)
	// A soft-wrapped line ends with the space the wrap ate. Clicking off the end
	// of one means "after the last word", not "the first column of the next
	// line", so the reachable run stops one short of that space. The last line
	// of a logical row has no such space and keeps its whole length, which is
	// what lets a click past the end of a paragraph land at the end of it.
	if li.RowOffset+1 < li.Height && avail > 0 {
		avail--
	}
	seg := rowRunes[start : start+max(avail, 0)]
	m.promptArea.SetCursorColumn(start + colAtWidth(seg, x-promptGutterWidth(m.promptArea)))
}

// promptRowRunes is the logical line the prompt editor's caret sits on. The
// textarea exposes its value only as a whole string, so the row is cut out of
// it here.
func promptRowRunes(ta textarea.Model) []rune {
	lines := strings.Split(ta.Value(), "\n")
	if r := ta.Line(); r >= 0 && r < len(lines) {
		return []rune(lines[r])
	}
	return nil
}

// promptGutterWidth is how many columns the editor draws before the first
// character of a line: its prompt ("┃ " by default), which the form leaves
// alone. Line numbers are off (see newFormInputs), so there is nothing else in
// front of the text. Measured rather than hardcoded so a change to the prompt
// can't quietly aim every click two columns off.
func promptGutterWidth(ta textarea.Model) int {
	return lipgloss.Width(ta.Prompt)
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

// chipTier is how much of a button a bar has room to print. A narrowing pane
// walks down the list, and each step gives up the least useful thing left: the
// chord first (the footer names every chord the chips stop teaching), then the
// word (an icon still points at something, and the footer is still naming the
// chords for it), never the button itself.
//
// Dropping a button was the alternative and is the wrong trade: a control that
// vanishes below some width is a control the user cannot learn is there, and the
// bars are the only place two of these actions exist at all — the form's ✉ Send
// has no chord anywhere, so a bar that dropped it would take the feature with it.
//
// The tiers are ordered widest-first so they can be compared: a tier at or below
// tierLabels is one that still prints words.
type chipTier int

const (
	tierHints  chipTier = iota // label and chord — "✔ Save enter"
	tierLabels                 // label alone — "✔ Save"
	tierIcons                  // the leading dingbat alone — "✔"
)

// chipText is what a button prints at a given tier. Every bar goes through here,
// drawing and measuring alike, so a chip's hit-test span cannot disagree with the
// glyphs the eye sees.
//
// An action with no chord at all — the form's ✉ Send, which is click-only on
// purpose — is label-only even at tierHints. Joining an empty hint on would pad
// the chip with a trailing space, and that pad would become a live column of
// button hanging off the right of the label.
func (a listAction) chipText(tier chipTier) string {
	switch {
	case tier <= tierHints && a.hint != "":
		return a.label + " " + a.hint
	case tier <= tierLabels:
		return a.label
	}
	return a.icon()
}

// chipGap is the space a bar leaves between two chips. It is the last thing a
// narrowing pane gives up, after the chords and after the labels: at tierIcons
// the chips sit flush against each other, and what separates the glyphs is the
// padding each chip carries inside its own field — two cells, which is as much
// air as the gap was buying.
//
// It is worth giving up because the alternative is worse in both directions. A
// bar that kept the gap and wrapped would put half its buttons on a line no
// click is hit-tested against (see formBarRow); a bar that dropped a button to
// stay on one line would hide a control the user cannot then learn exists. The
// gap costs nothing but air — and flush chips make the row a continuous strip of
// targets, so a click that lands between two glyphs presses one of them instead
// of nothing.
//
// Both bars measure and draw through this, so the spans a click is tested
// against cannot disagree with the glyphs the eye sees.
func chipGap(tier chipTier) int {
	if tier >= tierIcons {
		return 0
	}
	return 1
}

// icon is the chip's leading dingbat — the whole of the button at tierIcons.
// Every label is written "<glyph> <word>" and every glyph is one cell wide
// (TestFormBarIconsAreOneCell and its list counterpart hold that), so the first
// rune is the icon and an icon chip is exactly three columns with its padding.
func (a listAction) icon() string {
	runes := []rune(a.label)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[0])
}

// barTier picks the widest tier whose chips fit width, given the columns the bar
// is indented by. Zero width means "not sized yet" — the first frame renders
// before the terminal reports its size — and takes the full tier rather than
// guessing a narrow one and flickering wider a frame later.
//
// The gap after the last chip is counted too. It is one column of slack, and
// slack is the right side to err on: a bar measured exactly to the pane wraps
// its last chip onto the next line, where the pointer no longer finds it.
func barTier(acts []listAction, width, indent int) chipTier {
	if width <= 0 {
		return tierHints
	}
	for _, tier := range []chipTier{tierHints, tierLabels} {
		w := indent
		for _, a := range acts {
			w += lipgloss.Width(btnStyle.Render(a.chipText(tier))) + 1
		}
		if w <= width {
			return tier
		}
	}
	return tierIcons
}

// Indexes into listActions — used by runAction to name what it is dispatching.
const (
	actionAdd = iota
	actionEdit
	actionSend
	actionExport
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
	// actually goes: make it, change it, send it, hand it to another project,
	// throw it away. Green is spent on Send rather than Add because it is the
	// palette's color of consequence, and handing a prompt to a live agent is
	// the one thing on this screen that reaches out of the program; Delete's
	// red is the only warning the bar gives before the confirm asks. Export
	// sits between the two in the muted cyan of the form's Images chip — it
	// touches another backlog, which is more than an edit and less than a send.
	return []listAction{
		{label: "✚ Add", hint: "ctrl+a", tint: colInfo},
		{label: "✎ Edit", hint: "enter", tint: colStraw, needsSel: true},
		{label: "✉ Send", hint: m.modEnter(), tint: colAccent, needsSel: true},
		{label: "➦ Export", hint: "ctrl+o", tint: colCyan, needsSel: true},
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
	// The editor's two transient modes end with the screen they belong to: a
	// menu drawn over a form that is no longer up, or carets on lines nothing is
	// typing into, would both outlive the gesture that made them.
	m.menu = promptMenu{}
	m.endPromptCarets()
	// The list's own menu goes too. Pressing a row already closes it, so this is
	// for the paths that leave the list some other way — a scheduled drop firing
	// a stage change, a form opened by a chord while the box was up — where a
	// menu left standing would be composited over a list nobody is looking at
	// and would swallow the next keystroke on arriving back.
	m.listMenu = listMenu{}
	// Nothing on the list stage reads the editor's selection, but leaving a drag
	// flag set would hand the next stray motion message to a handler that expects
	// to be on the form.
	m.clearPromptSel()
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
	case actionExport:
		return m.beginExport()
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

// toggleSelected flips the highlighted todo between open and done — and back,
// which is what makes a done pressed by accident a one-key mistake rather than
// something to go and undo in the file.
//
// The highlight is re-parked on the todo afterwards for the reason freezeSelected
// re-parks it, and more so: completing a prompt does not merely move it into the
// done group, it files it at the *head* of that group (store.fileAsLatestDone),
// so the row can travel most of a pane on one keystroke. A cursor left at the old
// index lands on whichever prompt slid up into the gap — which is how a slipped
// ctrl+t becomes two mistakes, the second one landing on a row nobody chose.
// Riding with the todo means the key that closed it is also the key that reopens
// it, with nothing in between.
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
		m.setStatus("reopened — back on the list", false)
	} else {
		// The way back is named because this is the reversible half of the pair
		// and the row it acts on has just jumped somewhere else. With the closed
		// fold on (ctrl+d) it has jumped out of sight entirely, and the status
		// line is then the only thing on screen that can say so.
		m.setStatus("marked done (ctrl+t to reopen)", false)
	}
	m.rebuildList()
	m.selectRow(ref)
	return m, nil
}

// freezeSelected flips the highlighted todo between open and frozen — "I am not
// going to do this", said without deleting the prompt or lying that it was
// done.
//
// The highlight is re-parked on the todo afterwards because the row moves: a
// freeze drops it out of the open group and into the frozen one further down (or
// out of sight entirely while the fold is on), and a cursor left at its old
// index would land on whichever prompt slid up into the gap. The status line
// says which way the flip went, since with the fold on the row itself is gone
// and there is nothing else to read it from.
func (m model) freezeSelected() (tea.Model, tea.Cmd) {
	ref, ok := m.selectedRef()
	if !ok {
		return m, nil
	}
	// Read before the flip: freezing clears any pending schedule, and once it is
	// gone the row shows nothing to tell the user their auto-drop went with it.
	td, _ := m.resolve(ref)
	frozen, err := m.storeFor(ref.scope).toggleFrozen(ref.id)
	if err != nil {
		m.setStatus("save failed: "+err.Error(), true)
		return m, nil
	}
	m.rebuildList()
	m.selectRow(ref)
	if frozen {
		msg := "frozen — will not do (ctrl+f to unfreeze)"
		if td.Schedule != nil {
			msg += " · scheduled drop cancelled"
		}
		m.setStatus(msg, false)
	} else {
		m.setStatus("unfrozen — back on the list", false)
	}
	return m, nil
}

func (m *model) setStatus(s string, isErr bool) {
	m.status = s
	m.statusErr = isErr
}

// rebuildList regenerates the list rows from the two stores. Project todos come
// first; within each scope the three render groups follow in order — open, then
// frozen, then done (array order is the backlog's priority order within each) —
// and both closed groups disappear entirely when hideClosed is set. The
// "Project"/"Global" group headings only show when both groups have at least one
// visible todo; global rows additionally carry a " · global" tag of their own
// (see tagGlobal below for why the headings aren't enough).
//
// Frozen sits between open and done rather than among either. Above done because
// it is not finished work and should not be filed with what is; below open
// because it is not work waiting to be picked up. The middle is the only place
// left, and it happens to be the honest one.
func (m *model) rebuildList() {
	// The rows are about to move, so a card placed against one of them stops
	// being about the row it is sitting next to. It goes rather than being
	// re-placed: the pointer is the only thing that should ever put one on
	// screen, and the next motion message builds a fresh one.
	m.clearHover()
	var items []listItem
	var rows []todoRef

	visible := func(s *store) int {
		n := 0
		for _, t := range s.todos {
			if !m.folded(t) {
				n++
			}
		}
		return n
	}
	grouped := visible(m.project) > 0 && visible(m.global) > 0

	// Global rows carry their scope on the row (" · global") whenever the list
	// can also hold project rows. The group headings alone cannot say this: they
	// only render when both scopes have visible todos, and they are separator
	// rows, which filtering drops — so a filtered list, or a lone global todo
	// under a project's, showed nothing about which backlog a row edits. In a
	// global-only launch every row is global and the header already says so;
	// tagging them all would be noise.
	tagGlobal := m.project.available()

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
			case t.Frozen:
				// Ahead of the schedule badges deliberately, though freezing
				// clears a schedule so the two should never meet: if a backlog
				// edited by hand ever produced both, "will not do" is the fact
				// that matters and the ◷ would be a promise this manager has
				// already stopped keeping (fireDueSchedules skips frozen todos).
				badge, badgeStyle = "❄", frozenBadgeStyle
			case t.Schedule != nil && t.Schedule.Missed:
				badge, badgeStyle = "◷", errStyle
			case t.Schedule != nil:
				badge, badgeStyle = "◷", schedStyle
			}
			name := t.Title
			if name == "" {
				name = firstLine(t.Prompt, 60)
			}
			desc := firstLine(t.Prompt, 70)
			// The flags that lead the description, appended in the order they
			// are drawn — see descMark for why they are segments of their own
			// rather than text pasted onto the front of desc.
			var marks []descMark
			// The fire time leads even the attachments: of everything on the
			// row it is the one fact that changes on its own — and "missed"
			// is the row asking for a hand.
			if sc := t.Schedule; sc != nil && !t.Done {
				when := formatScheduleTime(sc.At, time.Now())
				if sc.Missed {
					when = "missed " + when
				}
				marks = append(marks, descMark{text: "⏰ " + when, style: descStyle})
			}
			// Session options get a bare ⚙ rather than their summary: the row
			// has one line, the summary can be six segments long, and what the
			// row has to answer is "does this prompt carry a setup" — the
			// answer to "which one" is one keystroke away in the form.
			if t.Session.configured() {
				marks = append(marks, descMark{text: "⚙", style: descStyle})
			}
			// Attachments lead the prompt text: a prompt written about a
			// screenshot usually reads as a fragment without it ("this is
			// wrong"), so the fact that one is carried is what makes the row
			// make sense. It is the one mark with a hue — the app's attachment
			// cyan, the same color the editor's ❐ Images chip carries — so
			// "this one has a picture" is answered by a glance down the list
			// rather than by reading each row's greys.
			//
			// But only while the row is still work. A done or frozen row recedes
			// on purpose — faint name, quiet badge, no strike-through of the
			// cyan — and a mark holding full saturation there would out-shout
			// the open todos above it, which is the exact opposite of what the
			// hue is for: it points at prompts that still need a picture
			// understood. On those rows the count drops to the description's
			// grey, the same tier the ⚙ beside it already sits in, so the row
			// recedes as one thing rather than as a grey row with a lit flag.
			if n := len(t.Images); n > 0 {
				attach := attachStyle
				if t.Done || t.Frozen {
					attach = descStyle
				}
				marks = append(marks, descMark{text: fmt.Sprintf("📎%d", n), style: attach})
			}
			tag := ""
			if tagGlobal && s.scope == scopeGlobal {
				tag = "global"
			}
			items = append(items, listItem{
				name:      name,
				desc:      desc,
				descMarks: marks,
				// The annotation columns, after the badge — every slot, blanks
				// included, because which columns survive is a decision about
				// the whole list rather than this row (see trimAnnotColumns,
				// applied once the last row is in).
				annots: annotMarksFor(t),
				// Match against the whole prompt (flattened to one line), not
				// just the rendered first-line preview, so a filter can hit
				// text buried deep in a multi-line prompt.
				search:     strings.Join(strings.Fields(t.Title+" "+t.Prompt), " "),
				badge:      badge,
				badgeStyle: badgeStyle,
				tag:        tag,
				strike:     t.Done,
				// Frozen recedes like a done row but keeps its letters: the
				// strike is a claim that the work happened, which is exactly what
				// freezing does not say.
				dim:        t.Frozen,
				selectable: true,
				ref:        len(rows),
			})
			rows = append(rows, ref)
		}
		// One pass per render group, in render order. Passing over the slice
		// three times is what keeps the grouping in the view instead of in the
		// file: the array stays the user's priority order, so a thaw puts a
		// prompt back exactly where it was rather than at the end of the queue.
		for _, g := range []int{groupOpen, groupFrozen, groupDone} {
			if g == groupFrozen && !m.showFrozen {
				continue
			}
			if g == groupDone && m.hideDone {
				continue
			}
			// The group is collected before it is drawn so the priority lens
			// has something to sort. With the lens off this is the same walk
			// the three passes always did, one slice longer.
			var grp []Todo
			for _, t := range s.todos {
				if t.group() == g {
					grp = append(grp, t)
				}
			}
			// Sorted inside the pass — one backlog, one group — and never
			// across them. A sort over the whole list would file a finished
			// critical prompt above open work and a critical global prompt
			// above the project's; the groups and the two backlogs are the
			// frame the user reads, and priority orders what sits inside that
			// frame rather than the frame itself.
			//
			// Stable, so prompts of equal priority keep the hand-set order
			// among themselves: the lens is meant to lift the urgent ones, not
			// to shuffle everything that is not.
			//
			// It sorts a copy. The store's array is the order the user dragged
			// the backlog into and the only record of it — sorting in place
			// would mean turning the lens off had nothing to restore, and would
			// write one pane's view preference into a file the other panes
			// share.
			if m.orderByPriority {
				sort.SliceStable(grp, func(i, j int) bool {
					return priorityRank(grp[i].Priority) < priorityRank(grp[j].Priority)
				})
			}
			for _, t := range grp {
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
	// Now that every row is built, drop the annotation columns nobody filled —
	// a whole-list decision, so it can only be made once the last row is in (see
	// trimAnnotColumns).
	trimAnnotColumns(items)
	// Swap in the new rows while keeping the existing query box and cursor
	// (setItems re-filters and clamps), so an add/edit/toggle doesn't disturb
	// what the user has typed or where they were.
	m.list.setItems(items)
	// The window budget moves with the list too, and for a reason the width has
	// no equivalent of: the group headings are items, they come and go with the
	// two backlogs, and each one costs the pane a line the window is not
	// counting (see sizeListWindow). Sized here rather than only on resize,
	// because nothing resizes when a heading appears.
	m.sizeListWindow()
	// The header budget moves with the list — the count's reserved digits and
	// the done-hidden tag both feed it — so the query input is re-sized here,
	// not only on resize. Before the first WindowSizeMsg there is no budget to
	// apply and the construction-time width stands.
	if m.width > 0 {
		m.sizeSearchInput()
	}
}

// folded reports whether either fold is currently holding this todo out of the
// list. The two questions are asked in one place so the row count, the header's
// hidden note and the render loop can never disagree about what is on screen.
func (m model) folded(t Todo) bool {
	switch {
	case t.Done:
		return m.hideDone
	case t.Frozen:
		return !m.showFrozen
	}
	return false
}

// hiddenClosedCount is how many todos the folds are holding back, for the
// header note. It counts what is actually hidden rather than everything closed,
// so hiding only one of the two states reports only that one.
func (m model) hiddenClosedCount() int {
	n := 0
	for _, s := range []*store{m.project, m.global} {
		for _, t := range s.todos {
			if m.folded(t) {
				n++
			}
		}
	}
	return n
}

// toggleClosedFold is ctrl+d: one key for "show me what is left to do", which
// is both folds at once.
//
// Mixed state resolves toward showing. With one of the two already hidden the
// key has to pick a direction, and revealing is the recoverable one — the next
// press hides both, where guessing the other way could fold away a group the
// user had just deliberately turned on and leave no sign it had happened.
// saveViewPrefs writes the View panel's two toggles back to the preferences
// file. Load-modify-write rather than saving the model's idea of the whole
// file, so this pane cannot blank a preference another pane — or another
// version — wrote between launches. Same shape toggleSpell uses.
func (m model) saveViewPrefs() error {
	s := loadSettings()
	s.orderByPriority, s.showFrozen = m.orderByPriority, m.showFrozen
	return s.save()
}

func (m model) toggleClosedFold() (tea.Model, tea.Cmd) {
	hiding := !(m.hideDone || !m.showFrozen)
	m.hideDone, m.showFrozen = hiding, !hiding
	m.rebuildList()
	// showFrozen is a saved preference, so this key writes it like the panel
	// does. A failure is worth a word but not the fold: the view has already
	// changed, and only its memory is lost.
	if err := m.saveViewPrefs(); err != nil {
		m.setStatus("view preference not saved: "+err.Error(), true)
		return m, nil
	}
	if hiding {
		m.setStatus("hiding completed and frozen prompts", false)
	} else {
		m.setStatus("showing completed and frozen prompts", false)
	}
	return m, nil
}

// doneCount is how many completed todos sit across both backlogs — what ctrl+w
// would sweep away. Frozen todos are not counted: the sweep leaves them alone
// (see store.clearDone), so counting them would put a number in the confirm
// prompt that does not match what the key does.
func (m model) doneCount() int {
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
	// Only the priority lens is checked, not the whole of canReorder: this
	// chord has always worked under a filter, because store.move steps through
	// the array rather than through what is on screen, and taking that away
	// would be a change nobody asked for. Under the lens it genuinely cannot
	// work — the array would shuffle beneath a list showing something else.
	if !m.orderIsBacklogOrder() {
		m.setStatus(m.reorderRefusal(), true)
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
	m.loadSpellDict()
	m.formImages, m.formImagesOrig = nil, nil
	// A new prompt starts on the defaults — the zero SessionOpts is exactly the
	// behaviour a drop had before options existed.
	m.formSession = SessionOpts{}
	m.formAnnots = annots{}
	m.annotCursor = 0
	m.discardClipboardCaptures()
	// Start in the prompt — that's the point of an entry.
	cmd := m.focusForm(formFieldPrompt)
	m.formErr, m.formNote = "", ""
	// The anchor is an offset into the value this form is replacing, so it means
	// nothing here. Both entry points clear it rather than trusting cancelForm to
	// have, because a form can also be reached from a stage that never had one.
	m.clearPromptSel()
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
	m.loadSpellDict()
	m.formImages = m.newFormImages(ref.scope, td)
	m.formImagesOrig = td.Images
	// A copy, not the pointer: an abandoned form must leave the stored options
	// exactly as they were, and editing through the pointer would have changed
	// them the moment a row was cycled.
	m.formSession = SessionOpts{}
	if td.Session != nil {
		m.formSession = td.Session.clone()
	}
	m.formAnnots = annotsOf(td)
	m.annotCursor = 0
	m.discardClipboardCaptures()
	cmd := m.focusForm(formFieldPrompt)
	m.formErr, m.formNote = "", ""
	m.clearPromptSel() // see beginAdd
	m.stage = stageForm
	return m, cmd
}

// formChromeHeight is how many lines the form spends on everything that isn't
// the prompt editor, so the editor gets the rest: the eight above it (see
// formPromptRow), then the attachment note, the session note, the toolbar, an
// error line, a blank, and the footer — fourteen — plus two of slack for a
// footer that needs a second line in a narrow pane.
const formChromeHeight = 16

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
	// Plain enter is a newline here, because that is what enter means in every
	// other editor a prompt gets typed into; save lives on ctrl+s (and the ✔
	// Save chip) instead, with shift+enter as a second save chord — see
	// updateForm, which catches it before the textarea ever sees it. The
	// remaining modifier spellings stay bound to newline: alt+enter (the legacy
	// ESC CR every terminal sends) and ctrl+j (the raw line feed that survives
	// a terminal which eats Option). ctrl+m is enter's wire form on a legacy
	// terminal, so it rides along for free.
	ta.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("enter", "ctrl+m", "alt+enter", "ctrl+j"),
		key.WithHelp("enter", "insert newline"),
	)
	ta.SetValue(prompt)

	w := m.width - 4
	if w < 20 {
		w = 60
	}
	ti.SetWidth(w)
	ta.SetWidth(w)
	h := m.height - formChromeHeight
	if h < 4 {
		h = 8
	}
	ta.SetHeight(h)
	return ti, ta
}

func (m model) updateForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// The context menu is modal while it is up: it owns every key, because that
	// is what a menu does — the one the user pressed is spent on choosing from
	// it or on taking it down (see updatePromptMenu).
	if m.menu.open {
		return m.updatePromptMenu(msg)
	}
	// The column mode owns the keys that would otherwise act on one caret —
	// typing, the deletes, the horizontal motions — and hands back everything
	// else, which ends the mode and then takes its ordinary path below. It sits
	// above the selection block because the two are exclusive by construction:
	// dropping carets clears the highlight.
	if m.carets.on && m.formFocus == formFieldPrompt {
		if next, cmd, handled := m.updatePromptCarets(msg); handled {
			return next, cmd
		}
		m.endPromptCarets()
	}
	// Selection first, because both of its keys are spellings of something this
	// switch already claims: shift+← would otherwise reach the editor as a plain
	// ←, and ctrl+c is the quit chord two lines down.
	if m.formFocus == formFieldPrompt {
		if plain, ok := promptSelectionKey(msg); ok {
			// Anchor before the motion, so the span runs from where the caret was
			// when shift was first held rather than from where it ends up.
			m.anchorPromptSel()
			return m.forwardForm(plain)
		}
		if promptCopyChord(msg.String()) && m.selectedPromptText() != "" {
			return m.copyPromptSelection()
		}
		// ctrl+x turns a swept markdown list into one backlog prompt per bullet
		// (promptsplit.go). It has to be answered up here, above the
		// clearPromptSel below, for the same reason the copy does: the selection
		// is its whole input, and by the time the switch is reached there is no
		// longer one to read. The switch keeps a case of its own so the chord
		// still explains itself when nothing is selected.
		if msg.String() == "ctrl+x" {
			return m.splitPromptList()
		}
		// alt+↑/↓ walks the caret's line — or every line the selection touches —
		// one row (promptmove.go). Up here for the same reason the split is: a
		// standing highlight is half its input, and the clearPromptSel below
		// would have taken it. The switch keeps a case of its own so the chord
		// still explains itself from the other fields.
		if dir, ok := promptLineMoveKey(msg); ok {
			return m.movePromptLines(dir)
		}
		// ctrl+p (cmd+P) opens the prompt library (promptpick.go). It is up here
		// with the other selection readers because the picker OFFERS TO SAVE
		// what is swept: by the time the switch below is reached the highlight
		// would already have been dropped, and the offer with it. The switch
		// keeps a case of its own for the title field, where the chord has no
		// editor caret to insert at and says so.
		if snippetLibChord(msg.String()) {
			return m.beginSnippets(loadPromptLib(), snippetsAll)
		}
		// Typing over a selection replaces it, and backspace or delete takes it
		// out — what a highlight is for besides copying it, and what every other
		// editor on the machine does with one. The span is removed first and the
		// key then takes its ordinary path below: an insertion lands at the
		// caret the deletion left behind (so 'x' over a swept "alpha" leaves
		// "x"), while a delete key has nothing left to do and is answered here.
		//
		// This has to sit above clearPromptSel, which is what these keys would
		// otherwise meet first. The anchor is dropped either way; dropping it
		// *without* acting on it is what left typed text sitting beside the run
		// it was meant to replace.
		if _, _, ok := m.promptSelSpan(); ok {
			switch {
			case promptSelDeleteKey(msg, m.promptArea.KeyMap):
				m.deletePromptSelection()
				m.formNote = ""
				return m, nil
			case promptPasteChord(msg.String()):
				// A paste is an insertion like any other, so it lands *on* the
				// highlighted run rather than beside it — the same rule Update
				// applies to a bracketed tea.PasteMsg. The span goes here and
				// the chord carries on to the switch below, which reads the
				// clipboard and inserts at the caret the deletion left behind.
				m.deletePromptSelection()
			case promptSelInsertKey(msg, m.promptArea.KeyMap):
				m.deletePromptSelection()
			}
		}
	}
	// Anything else ends the selection. This is on the way in rather than spread
	// across the handlers below because the rule is about the gesture, not about
	// any one key: a selection lasts exactly as long as the run of keys building
	// it, and the first key that isn't part of that run ends it. The copy note
	// goes with it: it reports on a selection, so it must not outlive one.
	m.clearPromptSel()
	m.formNote = ""

	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "super+c", "meta+c":
		// Cmd+C is the mac spelling of the copy above, and reaching this case at
		// all means there was nothing highlighted to copy — a selection would
		// have been answered by the block at the top of this function. So it
		// says why instead of doing nothing, and it deliberately does not fall
		// through to the quit that ctrl+c carries: overloading a quit on the
		// chord that always works is a liberty worth taking (see
		// copyPromptSelection), overloading it on a second spelling as well is
		// not — a hand that reaches for Cmd+C over a stray click is asking to
		// copy, never to leave.
		m.formNote = "nothing selected — sweep the prompt, or hold shift with ←/→, then copy"
		return m, nil
	case "super+v", "meta+v":
		return m.pasteFormClipboard()
	case "esc":
		return m.cancelForm()
	case "enter":
		// Enter belongs to whichever stop is focused, and they want different
		// things from it. The prompt is a text editor: enter is a newline
		// there, so the key falls through to the textarea (see newFormInputs,
		// which binds it on InsertNewline). On the annotation bar it presses
		// the segment under the cursor, which is what enter does on anything
		// shaped like a button. The title is one line and cannot hold a
		// newline at all, so enter does what enter does in every single-line
		// form — commits it. Save is ctrl+s from any stop, and the ✔ Save
		// chip from none.
		if m.formFocus == formFieldPrompt {
			break
		}
		if m.formFocus == formFieldAnnots {
			m.activateAnnotSeg(m.annotCursor)
			return m, nil
		}
		return m.saveForm()
	case "tab":
		return m.cycleFormFocus(1)
	case "shift+tab":
		return m.cycleFormFocus(-1)
	case "ctrl+s", "super+s", "meta+s", "shift+enter":
		// ctrl+s is the binding that always works; shift+enter is the chord
		// chat-style editors put on "send", so a hand that reaches for it here
		// gets a save rather than a newline (it needs the kitty protocol to be
		// distinguishable from enter at all, and where it is not, plain enter
		// arrives and inserts a newline as usual). super+s is the mac mnemonic
		// that mostly cannot — same shape of answer as ctrl+o vs ctrl+i above.
		// Cmd only reaches a TUI at all when the terminal encodes modifiers in
		// the kitty keyboard protocol *and* has not claimed the chord for its
		// own menu: Terminal.app eats Cmd+S outright, iTerm2 needs the key
		// mapped to "Send Text with vim Special Chars"/CSI u reporting, while
		// Ghostty, Kitty and WezTerm forward it once the chord is unbound on
		// their side. meta+s rides along because a terminal that reports Cmd as
		// the meta bit rather than the super bit spells the same press that way.
		return m.saveForm()
	case "super+d", "meta+d":
		// Cmd+D duplicates the caret's line — the chord every editor on this
		// machine puts a line copy on, and the reason it is Cmd-only is the
		// mirror of the ctrl+o/ctrl+i story above: the obvious fallback, ctrl+d,
		// is the textarea's own delete-character-forward (its DefaultKeyMap),
		// and a duplicate bound over a delete is the one collision a text editor
		// must not ship. So this rides the same road ctrl+s's cmd spelling
		// does — see the comment there for which terminals forward Cmd at all —
		// and every terminal that cannot send it is left with a chord that
		// deletes nothing rather than one that deletes a character.
		return m.duplicatePromptLine()
	case "ctrl+o", "ctrl+i":
		// ctrl+o is the binding that always works; ctrl+i is the mnemonic that
		// mostly cannot. ctrl+i *is* tab on the wire — both are 0x09 — so a
		// terminal without the kitty protocol delivers it as the tab above,
		// switching fields. Under kitty they are distinct keys and the mnemonic
		// works, which is why it stays bound; the footer advertises ctrl+o,
		// the one that cannot lose. (Same problem, same shape of answer, as
		// shift+enter vs alt+enter — see modEnter.)
		return m.beginImages()
	case "alt+up", "alt+down":
		// Reached only from the title or the annotation bar — the prompt's own
		// presses are answered at the top of this function, where the selection
		// is still standing. movePromptLines says why rather than no-opping.
		return m.movePromptLines(map[string]int{"alt+up": -1, "alt+down": 1}[msg.String()])
	case "ctrl+x":
		// Reached only with nothing selected — a live selection is answered at
		// the top of this function. splitPromptList says so itself rather than
		// no-opping. ctrl+x is free in both the textarea's keymap and the
		// textinput's (the survey the ctrl+r comment makes below), and it is
		// already this program's "take this out" everywhere it is bound: delete
		// in the list, remove in the attachment editor, and here the list that
		// leaves the prompt to become prompts of its own.
		return m.splitPromptList()
	case "ctrl+p", "super+p", "meta+p":
		// Reached only from the title field or the annotation bar — the prompt's
		// own presses are answered at the top of this function, where the
		// selection is still standing. A library entry is inserted at the
		// editor's caret, and neither of those two stops has one, so this is
		// refused in words the way the other caret tools refuse from here.
		m.formNote = "inserting a saved prompt works in the prompt — tab to it first"
		return m, nil
	case "ctrl+r":
		// The session panel — the one thing on the form that is about how the
		// prompt will *run*, which is what ctrl+r already means one screen over
		// (the picker's drop & run). Reusing the letter for the same idea in a
		// different stage is the pattern ctrl+x follows too — delete in the
		// list, remove in the attachment editor.
		//
		// Every chord this switch takes is one the editor below never sees, so
		// the choice is constrained to what a text editor does not already own:
		// ctrl+a/e are the caret's line ends, ctrl+w and ctrl+u and ctrl+k
		// delete, ctrl+f/b/n/p move, ctrl+s is save and ctrl+o is images. ctrl+r
		// is free in both the textarea's keymap and the textinput's.
		return m.beginSession()
	case "ctrl+l":
		// The Spelling panel (spellpanel.go): the flagged word nearest the
		// caret, what the dictionary thinks was meant by it, the two files it
		// could be added to, and the on/off switch.
		//
		// ctrl+l is free in both the textarea's keymap and the textinput's —
		// the survey the ctrl+r comment makes above holds here too — and,
		// unlike ctrl+y or ctrl+z, bound to nothing a terminal or shell might
		// take first. It is spent on the panel rather than on the toggle it
		// used to fire because spelling is not done often enough to earn three
		// chords: one key opens a screen that names everything the feature can
		// do, and the screen is where the choice is made. The toggle keeps a
		// one-press path of its own on the toolbar's ☑ Spell chip.
		//
		// It works from either field since it is about the form, not the caret.
		return m.beginSpell()
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
	// '@' at the start of a word opens the file picker (filepick.go). The
	// character goes into the editor first, like any other key, and the picker
	// opens after it: esc then leaves a plain '@' behind, which is what someone
	// who wanted the character gets, and a choice appends its path right where
	// the caret already is. Only the prompt field, and only at a word start —
	// an '@' inside a word is an e-mail address, not a request. A paste never
	// arrives here at all (it is a tea.PasteMsg, routed through forward), so a
	// pasted address cannot open it either.
	if m.formFocus == formFieldPrompt && msg.String() == "@" && promptAtWordStart(m.promptArea) {
		next, cmd := m.forwardForm(msg)
		m = next.(model)
		opened, blink := m.beginFiles()
		return opened, tea.Batch(cmd, blink)
	}
	// '/' at the start of a line opens the library's slash commands
	// (promptpick.go), on exactly the terms '@' opens the file picker on: the
	// character goes into the editor first, so esc leaves a plain slash behind,
	// and a chosen command replaces it rather than doubling it (snippetInsertion).
	//
	// Two guards instead of '@'s one, because a slash is ordinary punctuation
	// where an '@' is not (see promptAtLineStart), and because the library is
	// asked whether it has anything to offer before the screen changes: someone
	// who keeps no commands never meant to ask, and their "/Users/…" typed at a
	// line start is left alone. That read is why the library is passed in — the
	// picker must not answer the same question twice and risk two answers.
	if m.formFocus == formFieldPrompt && msg.String() == "/" && promptAtLineStart(m.promptArea) {
		if lib := loadPromptLib(); lib.hasCommands() {
			next, cmd := m.forwardForm(msg)
			m = next.(model)
			opened, blink := m.beginSnippets(lib, snippetsCommands)
			return opened, tea.Batch(cmd, blink)
		}
	}
	// On the annotation bar the remaining keys walk and press its segments
	// rather than reaching any editor (see annotbar.go).
	if m.formFocus == formFieldAnnots {
		return m.updateAnnotBar(msg)
	}
	return m.forwardForm(msg)
}

// duplicatePromptLine copies the logical line the caret sits on and inserts the
// copy directly below it, leaving the caret in the same column of the new line.
// Landing on the copy rather than staying on the original is what lets a held
// Cmd+D stack copies the way it does in a code editor; landing in the same
// column is what lets the press that follows carry on editing where the hand
// already was.
//
// "Line" here is a logical row of the value — a run between newlines — not a
// drawn one. A long row that soft-wraps over three display lines is a single
// line to duplicate, which is what someone looking at a wrapped paragraph means
// by the word; duplicating a wrap segment would cut a sentence at a boundary the
// value does not contain.
//
// The insertion goes through replacePromptRunes (spellpanel.go) rather than
// through the library's InsertString, for the reason that helper exists: the
// textarea only edits at its caret, so inserting at the row's end would mean
// walking the caret there first and then walking it back, and a walk that lands
// a character off is a bug that shows up only on soft-wrapped rows.
func (m model) duplicatePromptLine() (tea.Model, tea.Cmd) {
	if m.formFocus != formFieldPrompt {
		// Say why rather than no-op. The title is one line by construction — it
		// cannot hold a second — and the annotation bar is not text at all.
		m.formNote = "duplicate line works in the prompt"
		return m, nil
	}
	rows := strings.Split(m.promptArea.Value(), "\n")
	row := min(max(m.promptArea.Line(), 0), len(rows)-1)
	col := min(max(m.promptArea.Column(), 0), len([]rune(rows[row])))

	// The absolute rune offset of the caret's row-end, which is where the copy
	// is spliced in: every row above it plus the newline that ended each, then
	// the row itself.
	end := 0
	for i := range row {
		end += len([]rune(rows[i])) + 1
	}
	end += len([]rune(rows[row]))

	m.replacePromptRunes(end, end, "\n"+rows[row])
	// +1 steps over the newline just inserted, so the caret lands on the copy at
	// the column it held on the original.
	setPromptCaretOffset(&m.promptArea, end+1+col)
	m.formNote = "duplicated line"
	return m, nil
}

// copyPromptSelection puts the highlighted run of the prompt on the system
// clipboard. It is ctrl+c, and only while something is selected: with nothing
// highlighted that chord still quits, which is what it does on every other
// screen and what a hand reaching for it expects.
//
// Overloading the quit key is the one liberty this feature takes, and it is the
// right one. ctrl+c is what every other program on the machine copies with, and
// a selection that had to be copied with some other chord would be a selection
// nobody uses. The guard is the selection itself: the only way to reach the copy
// is to have deliberately swept or shift-walked over some text, and the way back
// to quitting is the same esc that already backs out of the form.
//
// The selection survives the copy. Copying is not a gesture that ends anything —
// the run is still highlighted, still visible, and can be copied again or
// extended.
func (m model) copyPromptSelection() (tea.Model, tea.Cmd) {
	text := m.selectedPromptText()
	if text == "" {
		return m, nil
	}
	n := len([]rune(text))
	unit := "characters"
	if n == 1 {
		unit = "character"
	}
	m.formNote = fmt.Sprintf("copied %d %s", n, unit)
	return m, copyTextToClipboard(text)
}

// promptCopyChord and promptPasteChord name the chords the editor treats as
// copy and paste.
//
// ctrl+c is the copy that always works, and a bracketed paste (tea.PasteMsg, in
// Update) is the paste that always works. The Cmd spellings ride along wherever
// the host is willing to hand the Command key over — the same bargain
// ctrl+s/cmd+s and cmd+d already make; see the ctrl+s handler for which
// terminals forward Cmd at all.
//
// Under cats the two halves arrive by different roads, which is why paste needs
// a chord here at all and copy needs one too:
//
//	⌘C  →  passed down to a kitty-protocol pane  →  super+c  →  this file copies
//	⌘V  →  the client reads the clipboard itself →  a paste event  →  tea.PasteMsg
//
// So under cats the paste chord below never fires — the text is already on its
// way as a paste — and it is here for the hosts that forward the keystroke
// instead of acting on it. meta+* is the same press from a terminal that reports
// Cmd as the meta bit rather than the super bit.
func promptCopyChord(s string) bool {
	switch s {
	case "ctrl+c", "super+c", "meta+c":
		return true
	}
	return false
}

func promptPasteChord(s string) bool {
	switch s {
	case "super+v", "meta+v":
		return true
	}
	return false
}

// pasteFormClipboard puts the system clipboard's text into the focused field at
// the caret — Cmd+V, for a host that delivers the chord rather than pasting for
// us (see promptPasteChord).
//
// The text is handed on as a tea.PasteMsg rather than inserted here, so both
// roads end in the same place: the field's own paste handling, which already
// knows about the caret, the value's undo state and the textarea's soft wraps.
// A second insertion path in this file would be a lesser copy of it, and the
// two would disagree exactly where it matters — a multi-line paste into a
// wrapped prompt.
func (m model) pasteFormClipboard() (tea.Model, tea.Cmd) {
	if m.formFocus == formFieldAnnots {
		// The annotation bar is a row of buttons, not a text field. Say so
		// rather than spraying the clipboard into whichever field last held the
		// keys — refusing in words is the rule everywhere else on this form.
		m.formNote = "pasting works in the title and the prompt"
		return m, nil
	}
	text, supported, err := readClipboardText()
	switch {
	case err != nil:
		m.formErr = err.Error()
		return m, nil
	case !supported:
		// Nothing local to ask (see readClipboardText): ask the terminal over
		// OSC 52 and paste if it answers. The flag is what makes the answer
		// belong to this request — see the tea.ClipboardMsg case in Update.
		m.pendingPaste = true
		return m, tea.ReadClipboard
	case text == "":
		m.formNote = "the clipboard has no text"
		return m, nil
	}
	return m.forwardForm(tea.PasteMsg{Content: text})
}

// cancelForm leaves the form without saving — esc, and the toolbar's ✖ Cancel.
// Abandoning the form abandons its clipboard captures with it: their temp files
// exist only because this form was open.
func (m model) cancelForm() (tea.Model, tea.Cmd) {
	m.discardClipboardCaptures()
	m.backToList()
	m.formErr = ""
	return m, nil
}

// The form's focus stops, in tab order — the values m.formFocus holds. The
// annotation bar is a stop of its own so its segments are reachable without a
// pointer, but it comes after the prompt rather than in its visual place
// between the two fields: the gesture this form lives on is "type a title,
// tab, type the prompt", and a stop inserted into that walk would spray the
// prompt's first keystrokes — spaces included, which the bar treats as a
// toggle — into a row that is not a text field. Tab from the prompt reaches
// the bar; shift+tab from the title reaches it directly.
const (
	formFieldTitle = iota
	formFieldPrompt
	formFieldAnnots
	formFieldCount
)

// focusForm puts the keys on one stop and takes them off the others, returning
// that field's cursor-blink command (nil on the annotation bar, which has no
// caret — its focus is drawn as an underline instead). It is idempotent by
// design: focusing the field that already holds the keys still restarts its
// blink, which is what a click inside it should do.
func (m *model) focusForm(field int) tea.Cmd {
	m.formFocus = field
	m.titleInput.Blur()
	m.promptArea.Blur()
	switch field {
	case formFieldTitle:
		return m.titleInput.Focus()
	case formFieldPrompt:
		return m.promptArea.Focus()
	}
	return nil
}

// cycleFormFocus walks the tab ring one stop in either direction.
func (m model) cycleFormFocus(delta int) (tea.Model, tea.Cmd) {
	next := (m.formFocus + delta + formFieldCount) % formFieldCount
	// The command is taken before m is returned: a returned value is a copy, and
	// taking it first would hand back the model as it was before the focus moved.
	cmd := m.focusForm(next)
	return m, cmd
}

// restoreFormFocus re-arms whichever stop held the keys, for the sub-stages
// (images, session, spelling, files) handing them back: a sub-stage that gave
// the keys back somewhere else would make the form feel like it moved.
func (m *model) restoreFormFocus() tea.Cmd {
	switch m.formFocus {
	case formFieldTitle:
		return m.titleInput.Focus()
	case formFieldPrompt:
		return m.promptArea.Focus()
	}
	return nil
}

func (m model) forwardForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.formFocus {
	case formFieldTitle:
		m.titleInput, cmd = m.titleInput.Update(msg)
	case formFieldPrompt:
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
	return m, m.restoreFormFocus()
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

// --- Session options editor ---------------------------------------------------

// The rows of the session panel, in the order they are drawn — which is the
// order the options take effect: how the session launches, what it starts from,
// what it does, how it ends. The cursor is an index into this set, so the
// numbering is the layout and nothing else; it is not stored anywhere.
const (
	// The prompt's own annotations (priority, quick win) are not here: they
	// describe the prompt rather than the session that will read it, and they
	// are set on the form's annotation bar (annotbar.go), in sight of the
	// title they qualify. Every row of this panel is about the session.
	sessRowModel      = iota // --model
	sessRowEffort            // --effort
	sessRowPermission        // --permission-mode
	sessRowClear             // /clear an existing pane first
	sessRowContext           // /sess-load | /sess-use
	sessRowContextArg        // its argument (free text)
	sessRowFiles             // extra files to read (free text, repeatable)
	sessRowFinish            // what to do when the work is done
	sessRowReviews           // which review skills to run first
	sessRowRelease           // and cut a release
	sessRowCount
)

// The values each enum row cycles through. "" leads every ring because it is
// the default — the panel opens on it, and one press of ← from there lands on
// the last real value rather than on nothing.
//
// The model ring is aliases only, and short: it is a convenience, not the set of
// legal values (normalizeModel takes any id), and a ring of every model ever
// shipped would be a worse way to reach "sonnet" than typing it at the CLI. A
// value set elsewhere and not in the ring is kept — see cycleValue.
var (
	sessModelValues      = []string{"", "opus", "sonnet", "haiku", "fable"}
	sessEffortValues     = []string{"", effortLow, effortMedium, effortHigh, effortXHigh, effortMax}
	sessPermissionValues = []string{"", permAcceptEdits, permAuto, permPlan, permManual, permDontAsk, permBypass}
	sessContextValues    = []string{ctxNone, ctxLoad, ctxUse}
	sessFinishValues     = []string{finishNone, finishCommit, finishPush, finishWrap}
	// The review row cycles presets rather than toggling a set: a row of
	// independent checkboxes needs a second cursor inside one line, and these
	// six combinations are the ones anybody actually asks for. Any other
	// combination is still reachable — `--review` is repeatable at the CLI, and
	// a set that came in that way is shown as-is and only changes if this row is
	// cycled.
	sessReviewValues = [][]string{
		nil,
		{reviewCode},
		{reviewSecurity},
		{reviewSimplify},
		{reviewCode, reviewSecurity},
		{reviewCode, reviewSecurity, reviewSimplify},
	}
)

// beginSession opens the session panel over the form, the way beginImages opens
// the attachment editor: the form's own inputs are blurred so only one cursor
// blinks, and closeSession hands the focus back to whichever field had it.
func (m model) beginSession() (tea.Model, tea.Cmd) {
	m.sessCursor = 0
	m.sessSkills = sessSkillsAvailable()
	m.setSessStatus("", false)

	ti := textinput.New()
	ti.Prompt = ""
	ti.SetWidth(sessInputWidth(m.width))
	m.sessInput = ti
	m.syncSessInput()

	m.titleInput.Blur()
	m.promptArea.Blur()
	m.stage = stageSession
	return m, textinput.Blink
}

// setSessStatus sets the panel's one message line, mirroring setImgStatus.
func (m *model) setSessStatus(s string, isErr bool) {
	m.sessStatus = s
	m.sessStatusErr = isErr
}

// closeSession returns to the form, restoring focus to the field that had it —
// the same contract closeImages keeps, and for the same reason: a sub-stage that
// gives the keys back somewhere else makes the form feel like it moved.
func (m model) closeSession() (tea.Model, tea.Cmd) {
	m.commitSessInput()
	m.setSessStatus("", false)
	m.stage = stageForm
	return m, m.restoreFormFocus()
}

// sessRowIsText reports whether row i is one of the two free-text rows. They are
// the rows where the shared input holds the keys, so ←/→ move the caret there
// instead of cycling a value.
func sessRowIsText(i int) bool {
	return i == sessRowContextArg || i == sessRowFiles
}

// sessInputWidth budgets the panel's shared box. It is capped well short of the
// pane: the box sits in the value column with the row's note beside it, and a
// field stretched to the right edge would push that note off the screen — for
// two values (a count, a filename) that are never long anyway.
func sessInputWidth(paneWidth int) int {
	const maxWidth = 40
	w := paneWidth - 20
	if w > maxWidth || paneWidth <= 0 {
		w = maxWidth
	}
	if w < 12 {
		w = 12
	}
	return w
}

// syncSessInput points the shared box at the row the cursor is now on: the
// context argument's current value on one row, an empty add-box on the other,
// and nothing (blurred) anywhere else — a blinking cursor on a row that ignores
// typing is a promise the panel doesn't keep.
func (m *model) syncSessInput() {
	switch m.sessCursor {
	case sessRowContextArg:
		m.sessInput.Placeholder = "the last saved session"
		if m.formSession.Context == ctxUse {
			m.sessInput.Placeholder = "part of a session doc's name"
		}
		m.sessInput.SetValue(m.formSession.ContextArg)
		m.sessInput.CursorEnd()
		m.sessInput.Focus()
	case sessRowFiles:
		m.sessInput.Placeholder = "path to a file, relative to the project"
		m.sessInput.SetValue("")
		m.sessInput.Focus()
	default:
		m.sessInput.Blur()
	}
}

// commitSessInput writes the context-argument box back to the record. Called
// whenever the cursor leaves that row and whenever the panel closes, so the
// value survives without the row needing an explicit "save" gesture. The files
// row commits per entry instead (enter appends), so there is nothing of its box
// worth keeping.
func (m *model) commitSessInput() {
	if m.sessCursor == sessRowContextArg {
		m.formSession.ContextArg = strings.TrimSpace(m.sessInput.Value())
	}
}

// moveSessCursor walks the cursor by delta, committing the row it leaves and
// arming the row it lands on.
func (m *model) moveSessCursor(delta int) {
	next := m.sessCursor + delta
	if next < 0 || next >= sessRowCount {
		return
	}
	m.commitSessInput()
	m.sessCursor = next
	m.syncSessInput()
}

func (m model) updateSession(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		return m.closeSession()
	case "up", "ctrl+p":
		m.moveSessCursor(-1)
		return m, nil
	case "down", "ctrl+n", "tab":
		m.moveSessCursor(1)
		return m, nil
	case "shift+tab":
		m.moveSessCursor(-1)
		return m, nil
	case "enter":
		// The same "do what's in front of me" split the attachment editor uses:
		// a path in the box gets added, and an empty box (or any other row)
		// means there is nothing left to say here.
		if m.sessCursor == sessRowFiles && strings.TrimSpace(m.sessInput.Value()) != "" {
			return m.addSessFile()
		}
		return m.closeSession()
	case "ctrl+x":
		if m.sessCursor == sessRowFiles {
			return m.removeSessFile()
		}
		return m, nil
	case "left", "right", " ", "space":
		// The caret keys belong to the box on a text row; only the enum rows
		// have anything to cycle.
		if !sessRowIsText(m.sessCursor) {
			delta := 1
			if msg.String() == "left" {
				delta = -1
			}
			m.cycleSessRow(delta)
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.sessInput, cmd = m.sessInput.Update(msg)
	return m, cmd
}

// cycleSessRow steps the highlighted enum row one value along its ring.
func (m *model) cycleSessRow(delta int) {
	o := &m.formSession
	switch m.sessCursor {
	case sessRowModel:
		o.Model = cycleValue(sessModelValues, o.Model, delta)
	case sessRowEffort:
		o.Effort = cycleValue(sessEffortValues, o.Effort, delta)
	case sessRowPermission:
		o.Permission = cycleValue(sessPermissionValues, o.Permission, delta)
	case sessRowClear:
		o.Clear = !o.Clear
	case sessRowContext:
		o.Context = cycleValue(sessContextValues, o.Context, delta)
		// The argument belongs to the mode that asked for it: a count meant for
		// /sess-load is not a pattern for /sess-use, and carrying it across
		// would quietly send "/sess-use 2".
		o.ContextArg = ""
	case sessRowFinish:
		o.Finish = cycleValue(sessFinishValues, o.Finish, delta)
	case sessRowReviews:
		o.Reviews = cycleSessReviews(o.Reviews, delta)
	case sessRowRelease:
		o.Release = !o.Release
	}
	m.setSessStatus("", false)
}

// cycleSessReviews steps the review set one preset along. A set that matches no
// preset (built with repeated --review flags) steps to the first one, which is
// the only sensible landing place: the ring has no position for it to be at.
func cycleSessReviews(cur []string, delta int) []string {
	i := -1
	for j, preset := range sessReviewValues {
		if slices.Equal(preset, cur) {
			i = j
			break
		}
	}
	if i < 0 {
		return sessReviewValues[0]
	}
	n := len(sessReviewValues)
	return sessReviewValues[((i+delta)%n+n)%n]
}

// addSessFile appends what is in the box to the extra-files list. The path is
// taken as typed and never resolved: it is delivered as text for the agent to
// read, in a session whose working directory is the project's, so a relative
// path is both what the user means and what will still be right tomorrow.
func (m model) addSessFile() (tea.Model, tea.Cmd) {
	f := strings.TrimSpace(m.sessInput.Value())
	if f == "" {
		return m, nil
	}
	if slices.Contains(m.formSession.Files, f) {
		m.setSessStatus(f+" is already on the list", true)
		return m, nil
	}
	m.formSession.Files = append(m.formSession.Files, f)
	m.sessInput.SetValue("")
	m.setSessStatus("added "+f, false)
	return m, nil
}

// removeSessFile drops the last file added — the undo for a typo, in the one
// gesture a row without a cursor of its own can offer.
func (m model) removeSessFile() (tea.Model, tea.Cmd) {
	if len(m.formSession.Files) == 0 {
		return m, nil
	}
	last := len(m.formSession.Files) - 1
	gone := m.formSession.Files[last]
	m.formSession.Files = m.formSession.Files[:last]
	m.setSessStatus("removed "+gone, false)
	return m, nil
}

// sessionNote is the form's ⚙ line: what the panel behind it holds, or that it
// holds nothing but defaults. Always something rather than nothing, for the
// reason the 📎 line is always drawn — a panel nobody knows about is a panel
// nobody uses.
//
// The annotations are deliberately not on it any more: the bar between the
// title and the prompt draws them live (annotbar.go), and a line that repeated
// what is already lit two inches up would be the form reading itself aloud.
func (m model) sessionNote() string {
	return firstNonEmpty(m.formSession.summary(), "default session")
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

// saveForm is ✔ Save (and enter, and ctrl+s): write the prompt and go back to
// the list. The write itself is persistForm's, so this path and ✉ Send's cannot
// drift into saving different things.
func (m model) saveForm() (tea.Model, tea.Cmd) {
	m, _, _ = m.persistForm()
	return m, nil
}

// sendForm is the toolbar's ✉ Send: write the prompt, then open the drop picker
// on the todo that was just written, so a prompt can go from the editor to an
// agent without a round trip through the list.
//
// Saving first is what makes the send unsurprising in both directions. The agent
// is handed the text that is on disk rather than a buffer the backlog never saw,
// and a send the picker then refuses (no control socket, a drop already in
// flight, a frozen prompt) still leaves the edit kept — the refusal costs the
// send, never the typing.
//
// An empty prompt never reaches the picker: persistForm refuses it with the same
// message ✔ Save gives, and ok=false stops us here with the form still open.
//
// There is deliberately no chord for this. Every other button on the row is
// recoverable — a save can be edited again, a cancel loses one edit — but a
// prompt handed to a live agent has left the program, and this stage is one the
// user is typing into with both hands. A chord one slip from the caret is
// exactly how a half-written prompt gets sent, so the pointer is the only way in.
func (m model) sendForm() (tea.Model, tea.Cmd) {
	m, ref, ok := m.persistForm()
	if !ok {
		return m, nil
	}
	// startDrop is the list's own entry point, so a send from here lands the
	// prompt through exactly the path shift+enter does — same guards, same
	// picker, same performDropCmd behind it.
	return m.startDrop(ref)
}

// persistForm writes the form to its backlog and reports where the prompt
// landed, so a caller that has something to do with the saved todo — ✉ Send,
// which needs its identity for the picker — can find it without re-deriving the
// id the add path mints internally.
//
// A failure leaves the form open with formErr set and reports ok=false; the
// caller hands the model straight back rather than deciding again what went
// wrong. On success the stage is already back on the list, which is where a
// plain save ends and where every one of startDrop's own refusals wants to be.
func (m model) persistForm() (model, todoRef, bool) {
	title := strings.TrimSpace(m.titleInput.Value())
	prompt := strings.TrimSpace(m.promptArea.Value())
	if prompt == "" {
		m.formErr = "The prompt can't be empty."
		return m, todoRef{}, false
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
		return m, todoRef{}, false
	}
	note := ""
	if n := m.imageCountNote(); n != "" {
		note = " · " + n
	}

	// Where the prompt ended up. Both branches fill it, because a caller that
	// wants to act on the saved todo (see sendForm) must not have to guess which
	// of the two wrote it.
	saved := todoRef{scope: m.formScope}

	if m.formMode == formAdd {
		// The id has to exist before the copy — it names the attachment
		// directory — so it is minted here rather than inline below.
		id := newID()
		added, err := st.attachImages(id, m.pendingSources())
		if err != nil {
			m.formErr = "attach failed: " + err.Error()
			return m, todoRef{}, false
		}
		td := Todo{
			ID: id, Title: title, Prompt: prompt, Images: added,
			// nil when nothing was set, so a prompt taking the defaults writes
			// no "session" key at all (see sessionPtr).
			Session: sessionPtr(m.formSession),
			Created: time.Now(),
		}
		// Applied as a set rather than field by field, so a mark added later
		// cannot be forgotten on this path (see annots). Every zero value means
		// "nothing said", so an unannotated prompt still writes no "priority"
		// or "fruit" key at all.
		m.formAnnots.applyTo(&td)
		if err := st.add(td); err != nil {
			// The copies are on disk but no todo will ever reference them.
			st.removeImages(id)
			m.formErr = "save failed: " + err.Error()
			return m, todoRef{}, false
		}
		saved.id = id
		m.setStatus("added to "+m.formScope.String()+" backlog"+note, false)
	} else {
		// Copy first, then record, then delete: every step leaves the todo
		// either as it was or as the user asked for, and a failure anywhere
		// takes this save's own copies back out with it.
		added, err := st.attachImages(m.editID, m.pendingSources())
		if err != nil {
			m.formErr = "attach failed: " + err.Error()
			return m, todoRef{}, false
		}
		if err := st.update(Todo{ID: m.editID, Title: title, Prompt: prompt}); err != nil {
			st.removeImageFiles(m.editID, added)
			m.formErr = "save failed: " + err.Error()
			return m, todoRef{}, false
		}
		if err := st.setImages(m.editID, append(m.keptRels(), added...)); err != nil {
			st.removeImageFiles(m.editID, added)
			m.formErr = "save failed: " + err.Error()
			return m, todoRef{}, false
		}
		// Same "record it explicitly" split as the images above: update() is
		// text-only, so the options are their own write (see setSession). No
		// rollback of the copies here — by this point the text and the
		// attachments are already saved, so failing loudly and leaving them is
		// more honest than un-attaching files the todo now references.
		if err := st.setSession(m.editID, sessionPtr(m.formSession)); err != nil {
			m.formErr = "save failed: " + err.Error()
			return m, todoRef{}, false
		}
		// And its own write again, for the same reason: the annotations live on
		// the Todo rather than in its session record, and update() would not
		// have carried them.
		if err := st.setAnnots(m.editID, m.formAnnots); err != nil {
			m.formErr = "save failed: " + err.Error()
			return m, todoRef{}, false
		}
		// Only now does the record no longer mention them.
		st.removeImageFiles(m.editID, m.droppedRels())
		saved.id = m.editID
		m.setStatus("updated"+note, false)
	}
	// The backlog holds its own copies now, so the captures' temp files have
	// nothing left to answer for.
	m.discardClipboardCaptures()
	m.rebuildList()
	m.backToList()
	return m, saved, true
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
// both scopes, or reports there is nothing to clear. Frozen prompts are not in
// the count and not in the sweep — see store.clearDone.
func (m model) beginClearDone() (tea.Model, tea.Cmd) {
	count := m.doneCount()
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
	// A frozen prompt is a decision not to do the work, and shift+enter is one
	// keystroke away from every row in the list — including the ones that scroll
	// under the cursor after a rebuild. Refusing here is what makes the decision
	// hold: unfreezing is a deliberate act, and once it is done the drop works
	// like any other.
	if td, ok := m.resolve(ref); ok && td.Frozen {
		m.setStatus("that prompt is frozen — unfreeze it (ctrl+f) to send it", false)
		m.backToList()
		return m, nil
	}
	m.dropTodo = ref
	m.targets, m.targetList = m.buildTargets()
	m.stage = stageTarget
	return m, textinput.Blink
}

// newSessionAgents are the agents offered as "new session" drop targets whether
// or not one is currently running anywhere. The order here is the order in the
// picker, and the first entry is what the picker highlights by default.
//
// claude is unconditional: it is the manager's home ground, and the only entry
// guaranteed to keep the picker non-empty (an environment where the binary is
// reachable from the cats process but not from ours would otherwise be left
// with nothing to drop into). Every other entry is gated on being found in PATH
// — the same rule the running-pane scan gets for free ("if it's running, its
// command is installed") — so we never offer a tab that would exec into
// nothing.
var newSessionAgents = []struct {
	command   string
	label     string
	needsPath bool // resolve command in PATH before offering it
}{
	{command: "claude", label: "＋ New Claude Code session"},
	{command: "copilot", label: "＋ New GitHub Copilot session", needsPath: true},
}

// buildTargets assembles the drop destinations, in the order the picker shows
// them: a new session per known agent (see newSessionAgents), a new session for
// every other agent currently running somewhere (if it's running, its command
// is installed), the same set again on a fresh git worktree when the project is
// a repo, plus every agent pane cats reports (Claude panes first), excluding our
// own. Pane agent identity and cwd come straight from pane.list's runtime
// metadata; the workspace a pane lives in is the "w1" prefix of its handle,
// labeled via workspace.list.
//
// The two new-session flavours are emitted as two blocks rather than
// interleaved per agent, so the second block reads as one answer to one
// question ("somewhere isolated, instead") and the default highlight stays on
// the plain first row.
func (m model) buildTargets() ([]dropTarget, fuzzyList) {
	wsLabel := firstNonEmpty(m.ctx.WorkspaceLabel, baseName(m.ctx.projectDir()), "the current workspace")

	// The launchable agents, gathered before any row is built so the worktree
	// block can mirror the plain one exactly. seenAgent doubles as the dedupe
	// set for the running-agent scan below, so a running copilot doesn't earn a
	// second "＋ New copilot session" row.
	type newAgent struct{ command, label string }
	var newAgents []newAgent
	seenAgent := map[string]bool{}
	for _, a := range newSessionAgents {
		if a.needsPath {
			if _, err := exec.LookPath(a.command); err != nil {
				continue
			}
		}
		seenAgent[a.command] = true
		newAgents = append(newAgents, newAgent{command: a.command, label: a.label})
	}

	// panes is nil when there is no socket (or the call fails), which is what
	// degrades the picker to the new-session rows alone.
	var agents []app.PaneInfo
	if m.client != nil {
		if panes, err := m.client.paneList(); err == nil {
			for _, p := range panes {
				if p.Agent == "" || isOwnPane(m.ctx, p) {
					continue
				}
				agents = append(agents, p)
			}
			sort.SliceStable(agents, func(i, j int) bool {
				return agents[i].Agent == "claude" && agents[j].Agent != "claude"
			})

			// One "new session" entry per distinct agent not already offered
			// above, launched with the agent label cats detected as the command.
			for _, p := range agents {
				if seenAgent[p.Agent] {
					continue
				}
				seenAgent[p.Agent] = true
				newAgents = append(newAgents, newAgent{command: p.Agent, label: "＋ New " + p.Agent + " session"})
			}
		}
	}

	// The todo being dropped is chosen before the picker is built (startDrop and
	// updateSchedule both set it first), so the rows can say what this
	// particular prompt's options will and won't survive on them. The warning is
	// on the row rather than in the status line afterwards, because the point of
	// it is to be read before the choice, not after it.
	td, _ := m.resolve(m.dropTodo)
	flagNote := ""
	if td.Session.hasLaunchFlags() {
		flagNote = " · the session's model/effort flags are claude-only and won't be passed"
	}

	var targets []dropTarget
	for _, a := range newAgents {
		desc := "open a new tab in " + wsLabel + " and launch " + a.command
		if !isClaudeCommand(a.command) {
			desc += flagNote
		}
		targets = append(targets, dropTarget{
			kind:    targetNewSession,
			command: a.command,
			label:   a.label,
			desc:    desc,
		})
	}

	// The worktree block, offered only where there is something to branch from.
	// Not gated on the socket, for the same reason the plain rows above are not:
	// startDrop refuses to open the picker at all without one.
	if repo := gitRepoRoot(m.ctx.projectDir()); repo != "" {
		for _, a := range newAgents {
			desc := "branch " + baseName(repo) + " into a fresh checkout, open it as a workspace, and launch " +
				a.command + " there"
			if !isClaudeCommand(a.command) {
				desc += flagNote
			}
			targets = append(targets, dropTarget{
				kind:     targetNewSession,
				command:  a.command,
				worktree: true,
				label:    a.label + " on a new worktree",
				desc:     desc,
			})
		}
	}

	// The running-pane block. agents is empty without a socket, so the
	// workspace.list call below is only reached when there is something to label.
	if len(agents) > 0 {
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
		return m.chooseTarget(dropRun)
	case "shift+enter", "alt+enter":
		// Pause after drop. The picker's two enters are one choice with two
		// answers, and this is the rarer one: plain enter does what dropping a
		// prompt means, and the chord is for when you want to read it in the
		// agent's own input first.
		//
		// These two chords used to carry the opposite meaning, back when the
		// paste was the rule rather than the option. An old reflex now pauses a
		// drop instead of running one — the harmless direction for the mistake
		// to go, and the reason the swap is worth making rather than inventing
		// a third chord nobody would find.
		return m.chooseTarget(dropPaste)
	case "ctrl+r":
		// The older spelling of "run", still bound to running: r means the same
		// thing here as it does on the form's ⚙ panel one screen over.
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
	// The status names the mode, not just the destination: "dropping" and
	// "pasting" are two different promises about what happens next, and the
	// paused one is the one with a step still owed by the user.
	verb := "dropping into "
	if mode == dropPaste {
		verb = "pasting into "
	}
	m.setStatus(verb+targetDesc(target)+"…", false)
	return m, m.performDropCmd(m.dropTodo, pendingAction{
		todo:       td,
		target:     target,
		mode:       mode,
		cwd:        m.ctx.projectDir(),
		images:     m.storeFor(m.dropTodo.scope).imagePaths(td),
		anchorPane: m.ctx.OwnPaneID,
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
		return dropResultMsg{desc: desc, ref: ref, mode: act.mode, err: performDrop(client, act)}
	}
}

// targetDesc is a short, human label for a drop destination, used in the
// "dropping…" / "dropped →" status lines.
func targetDesc(t dropTarget) string {
	if t.kind == targetNewSession {
		name := "new Claude Code session"
		if t.command != "" && t.command != "claude" {
			name = "new " + t.command + " session"
		}
		if t.worktree {
			// Worth spelling out in the status line: this drop is about to make
			// a branch and a checkout, which the plain flavour never does.
			name += " on a new worktree"
		}
		return name
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
	// Freezing already clears a pending schedule; refusing to set a new one is
	// the same rule kept from the other side, so the state cannot be worked
	// around by scheduling first and freezing after — or after, by scheduling a
	// prompt that is already shelved.
	if td.Frozen {
		m.setStatus("that prompt is frozen — unfreeze it (ctrl+f) to schedule it", false)
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
			// t.closed() covers both states that mean "do not fire": done, and
			// frozen. Each clears the schedule as it is set, so a closed todo
			// holding one should not exist — this is the backstop for a backlog
			// edited by hand or written by an older binary that did not know the
			// invariant, where firing would be the worst possible reading of it.
			if sc == nil || sc.Missed || t.closed() || now.Before(sc.At) {
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
		// Read from the live context, not the schedule: the anchor names a pane
		// that has to exist *now* (this one), where every other field of a
		// schedule describes what was chosen back then.
		anchorPane: m.ctx.OwnPaneID,
	}
	desc := targetDesc(act.target)
	return func() tea.Msg {
		// mode is stated even though act.mode already says it: a fire always
		// runs, and the success line reads off this field.
		return dropResultMsg{desc: desc, ref: ref, mode: dropRun, err: performScheduledDrop(client, sc, act), sched: &sc}
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
	case "ctrl+o":
		return m.startExport(m.viewRef)
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
	// The session line goes above the attachments and below the body, in the
	// same plain text for the same reason: this is one wrapped block, and a
	// pre-styled span inside it loses its reset at the wrap points.
	if s := td.Session.summary(); s != "" {
		body += "\n\n⚙ session: " + s
	}
	if refs := m.storeFor(m.viewRef.scope).resolveImages(td); len(refs) > 0 {
		var b strings.Builder
		b.WriteString(body)
		fmt.Fprintf(&b, "\n\n📎 %d attached:", len(refs))
		for _, ref := range refs {
			b.WriteString("\n  ")
			b.WriteString(ref.rel)
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

// openCount is how many todos still to do sit in the backlog this pane is named
// after: the project's, or the global one when that is the only backlog the
// launch opened (--global, or no project to scope to). Frozen prompts are not
// counted — the number is read as "work waiting here", and the point of freezing
// one is that it is not waiting for anybody.
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
		if !t.closed() {
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
	// stage pays that for the action bar and for dragging rows into order, the
	// picker for its target rows, and the form for placing the caret in a prompt
	// being written; the prompt view — the one screen whose whole point is text
	// worth copying out — keeps its selection.
	//
	// The form is the one stage where losing the terminal's selection would have
	// cost something real, since a prompt is text worth copying too. It doesn't:
	// the editor draws and copies its own selection instead (see promptsel.go),
	// which is what these mouse messages are also being spent on.
	//
	// Cell motion is also exactly the mode a drag needs: it reports motion only
	// while a button is held, so the manager hears the gesture without paying for
	// a message on every idle sweep of the pointer across the pane.
	//
	// The list is the one stage that asks for more than that. Its hover card
	// (listhover.go) is drawn from motion with nothing held, which cell motion
	// by definition never reports, so it takes MouseModeAllMotion and pays a
	// message per cell the pointer crosses. That is a real cost and it buys the
	// one thing the list cannot otherwise say — what is inside a prompt without
	// leaving the list to find out — on the only screen where the rows are too
	// narrow to say it themselves.
	switch {
	case m.stage == stageList:
		v.MouseMode = tea.MouseModeAllMotion
	case m.stage == stageTarget || m.stage == stageForm || m.stage == stageFiles || m.stage == stageSnippets || m.stage == stageExport || m.stage == stageSpell || m.stage == stageViewOpts:
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
		// The menu floats over the form rather than replacing it, so it is
		// composited here rather than inside viewForm: every row constant the
		// form hit-tests against is measured on the frame underneath, and a menu
		// spliced in before those were computed would move the toolbar out from
		// under the pointer for as long as it was open.
		return m.overlayPromptMenu(m.viewForm())
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
	case stageSession:
		return m.viewSession()
	case stageFiles:
		return m.viewFiles()
	case stageSnippets:
		return m.viewSnippets()
	case stageExport:
		return m.viewExport()
	case stageSpell:
		return m.viewSpell()
	case stageViewOpts:
		return m.viewViewOpts()
	default:
		// The menu floats over the list rather than replacing it, and is
		// composited here rather than inside viewList for the reason the form's
		// is: every row constant the list hit-tests against is measured on the
		// frame underneath, and a menu spliced in before those were computed
		// would move the rows out from under the pointer while it was open.
		// The hover card goes on under the menu for the same reason it is never
		// built while one is open (see hoverMotion): the menu is the box that can
		// be pressed, so it is the box that has to be on top.
		return m.overlayListMenu(m.overlayHoverCard(m.viewList()))
	}
}

// headerTitle styles the header line's leading segment. The tool's own name
// and version used to sit here in a chip, which spent the pane's most valuable
// column on a fact the terminal tab already carries ("todo: …") and that never
// changes between frames. With the chip gone the backlog's own name is the
// heading, so it takes the bright weight and everything the note adds after
// it — the scope suffix, the ws label, the hidden tag — stays tertiary, the
// same hierarchy the chip and its trailing note used to draw.
//
// The split point is found rather than threaded through: scopeNote builds the
// note as "<project><suffix>[ · ws:x]" with suffix one of " + global" / " only"
// (see scopeNote), so the first occurrence of either ends the name — including
// when the name itself has been truncated to "long-nam…". The no-project notes
// ("no backlog here") have no such suffix and simply stay dim; "global only"
// splits on " only" and reads correctly as a name plus its scope.
func headerTitle(note string) string {
	for _, suffix := range []string{" + global", " only"} {
		if i := strings.Index(note, suffix); i > 0 {
			return headerNameStyle.Render(note[:i]) + descStyle.Render(note[i:])
		}
	}
	return descStyle.Render(note)
}

// headerGap is the breath between the header line's segments.
const headerGap = 2

// searchLead is the wider breath in front of the query box. The header's left
// half — chip, version, scope note — reads as one block of identity; at a plain
// headerGap the search glyph sat close enough to the note to read as its tail
// rather than as its own control. Being pure whitespace it is also the cheapest
// thing on the line, so it is the first concession the ladder makes when the
// pane narrows (see headerLayout).
const searchLead = 6

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

// The query input's sizes as the header narrows: full, roomy (a size a typed
// query still reads comfortably in — the box gives back this much before the
// lead spacing concedes anything), squeezed (still wide enough to read a typed
// word), and the floor below which the segment is dropped from the line rather
// than rendered as a slot nothing fits in.
// Type-to-filter keeps working with the segment gone — only the echo is lost.
const (
	searchFieldRoomy    = 24
	searchFieldSqueezed = 12
	searchFieldMin      = 6
)

// headerLayout is what survives on the header line at the current width. The
// zero value means "everything": full-size search, whole note.
type headerLayout struct {
	note       string // budgeted scope note (done-hidden tag included); "" = dropped
	searchLead int    // cells of whitespace in front of the query glyph
	searchW    int    // the query input's SetWidth; 0 = segment dropped
	count      string // "matched/total"
	countW     int    // cells reserved for count — total's digits, so typing doesn't shift the line
}

// headerLayout budgets the header's one line: scope note (the heading now that
// the tool's chip is gone), query box, match count. The line must never exceed
// m.width — every row below it is hit-tested against a constant (see
// actionBarRow), and a wrapped header silently moves all of them out from
// under the mouse.
//
// When the pane is too narrow for everything, the segments concede in a fixed
// order, cheapest first:
//
//	ws label → search lead narrows → done-hidden tag → search shrinks →
//	search floors, then drops → project name truncates (inside scopeNote) →
//	count last
//
// The project name is the last thing on the line to give up any width: it is
// the fact that decides what an edit touches.
func (m model) headerLayout() headerLayout {
	matched, total := m.list.counts()
	hl := headerLayout{
		searchLead: searchLead,
		searchW:    searchFieldWidth,
		count:      fmt.Sprintf("%d/%d", matched, total),
	}
	hl.countW = len(fmt.Sprintf("%d/%d", total, total))

	// m.width is 0 until the first WindowSizeMsg lands; with nothing to budget
	// against, show everything rather than guess.
	if m.width <= 0 {
		hl.note = m.scopeNote(-1) + m.hiddenNote()
		return hl
	}

	// Room left for the note at the current concessions. The note leads the line
	// now, so the only fixed cost in front of the count is the gap before it —
	// the chip and the gap that separated it from the note are both gone. The
	// input is counted one cell wider than its SetWidth (inputCursorPad); rune
	// counts stand in for cell width on the note itself — it is plain prose.
	room := func() int {
		r := m.width - hl.countW - headerGap
		if hl.searchW > 0 {
			r -= hl.searchLead + lipgloss.Width(searchGlyph) + hl.searchW + inputCursorPad
		}
		return r
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
	// The box gives back its slack down to a still-comfortable width before the
	// lead spacing pays anything: a query box a few cells narrower reads the
	// same, whereas the header's two halves running together does not.
	if over := core + runew(hidden) - room(); over > 0 && hl.searchW > searchFieldRoomy {
		hl.searchW -= min(over, hl.searchW-searchFieldRoomy)
	}
	// Then the lead, one cell at a time — a pane a couple of columns short keeps
	// most of the spacing instead of snapping shut to the plain gap.
	if over := core + runew(hidden) - room(); over > 0 && hl.searchLead > headerGap {
		hl.searchLead -= min(over, hl.searchLead-headerGap)
	}
	if over := core + runew(hidden) - room(); over > 0 && hl.searchW > searchFieldSqueezed {
		hl.searchW -= min(over, hl.searchW-searchFieldSqueezed)
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

// hiddenNote is the header's " · N hidden" tag while the fold is on. It says
// "hidden" rather than "done hidden" now that the fold takes frozen prompts too:
// naming one of the two states would misreport the other half of the number.
func (m model) hiddenNote() string {
	if n := m.hiddenClosedCount(); n > 0 {
		return fmt.Sprintf(" · %d hidden", n)
	}
	return ""
}

// headerLine renders the list view's single top line from the current budget:
// scope note, inline query box, match count. The glyph in front of the
// input is the focus affordance the boxed field's rails used to be: lit when
// the box holds the keys, faint when a button does. The input's own focus is
// the truth here, same as the picker's rails — setActionFocus moves both.
func (m model) headerLine() string {
	hl := m.headerLayout()
	var b strings.Builder
	// The note starts at column 0 — the pane's first column names the backlog
	// being edited rather than the program doing the editing.
	if hl.note != "" {
		b.WriteString(headerTitle(hl.note))
	}
	if hl.searchW > 0 {
		b.WriteString(strings.Repeat(" ", hl.searchLead))
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
		return footerStyle.Render("enter / dbl-click edit · "+m.modEnter()+" drop · ctrl+v view · ctrl+a add · ctrl+t done · ctrl+f freeze · ctrl+o export · ctrl+x delete") +
			"\n" +
			footerStyle.Render("ctrl+s schedule · tab buttons · ctrl+↑/↓ or drag move · right-click menu · ctrl+d hide/show closed · ctrl+l view options · ctrl+w clear done · esc quit")
	}
	// Freeze rides directly after done: the two are the ways a prompt leaves the
	// open list, and reading them side by side is what teaches that they are
	// different claims. It also puts the pair ahead of the concessions the loop
	// below makes from the right, so a narrow pane keeps them.
	// Priority rides directly after freeze for the same reason freeze rides
	// after done: all three are per-row marks, and the chords that set them are
	// learned together. The View panel follows the fold it generalizes.
	//
	// "view options" spelled out, never shortened to "view": ctrl+v is two
	// segments to the left and means read this prompt's text. Two chords sharing
	// a word on one line is how a reader learns the wrong one.
	//
	// "right-click menu" goes last, past esc, and is therefore the first thing a
	// narrowing pane drops. That is the right order to lose them in: the menu is
	// a shortcut to chords this line is already naming, so a reader who loses it
	// loses nothing they cannot still do — while esc is the way out, and has to
	// survive longer than a convenience does.
	segs := []string{
		"ctrl+s schedule", "ctrl+v view", "ctrl+t done", "ctrl+f freeze",
		"ctrl+↑/↓ or drag move", "ctrl+d hide closed", "ctrl+l view options", "ctrl+w clear done", "esc quit",
		"right-click menu",
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
	gap := strings.Repeat(" ", chipGap(m.barTier()))
	for i, c := range m.actionChips() {
		if i > 0 {
			b.WriteString(gap)
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

// actionChips lays the bar out: the chip contents (as much of each button as the
// pane has room for — see chipTier) and where each one lands. The three button
// styles share one padding, so widths don't depend on which is picked at render
// time and btnStyle can measure for all of them.
func (m model) actionChips() []actionChip {
	acts := m.listActions()
	tier := m.barTier()

	chips := make([]actionChip, 0, len(acts))
	x := indentWidth
	for i, a := range acts {
		if i > 0 {
			x += chipGap(tier) // the space between chips, if the pane can afford one
		}
		text := a.chipText(tier)
		w := lipgloss.Width(btnStyle.Render(text))
		chips = append(chips, actionChip{text: text, start: x, end: x + w})
		x += w
	}
	return chips
}

// barTier is how much of each button the list's bar is printing.
func (m model) barTier() chipTier {
	return barTier(m.listActions(), m.width, indentWidth)
}

// barShowsHints reports whether the action bar is wide enough for every chip
// to carry its key hint. The footer keys off the same answer — chords the
// chips teach stay off it, and the moment the bar goes quiet the footer names
// everything again — so the two can't both drop a chord at once.
func (m model) barShowsHints() bool {
	return m.barTier() == tierHints
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

// listChromeBelow is how many lines the list view spends UNDER the rows: the
// blank that separates them from the status line, the status line itself (drawn
// empty when quiet, so the footer never moves), the blank under that, and the
// footer — two lines of it in a pane too narrow for the action bar to teach its
// own chords, which is the case that has to be budgeted for or the footer is
// what falls off the bottom.
const listChromeBelow = 5

// sizeListWindow fits the backlog's scroll window to the pane: the lines left
// once the chrome above the rows (listRowsRow) and below them (listChromeBelow)
// is spent, minus the lines this list's group headings cost on top of one line
// per row (separatorLines — the window is counted in items, the pane in lines).
//
// Before this the list had no window at all: every row was rendered and the
// terminal clipped whatever ran past the bottom, which meant a backlog longer
// than the pane simply had no bottom — no marker, no count, and a highlight
// that could walk off the screen and keep going with nothing left to say where
// it was. A window is what gives the overflow markers something true to report.
//
// A height that is not known yet — before the first WindowSizeMsg, and in tests
// that never send one — leaves the list unwindowed, which is the behavior every
// caller had before and is still the right one when there is no pane to fit to.
func (m *model) sizeListWindow() {
	if m.height <= 0 {
		m.list.setMaxRows(0)
		return
	}
	m.list.setMaxRows(max(m.height-listRowsRow-listChromeBelow-m.list.separatorLines(), 1))
}

// The form's fixed rows, counting from the top of that view: the heading (0), a
// blank (1), the Title label (2), the title field (3), a blank (4), the
// annotation bar (5), a blank (6), the Prompt label (7), then the prompt
// editor. Every one of those is exactly one line, so a click's Y is compared
// against these constants instead of the view being re-measured.
// TestFormRowsMatchWhatIsDrawn finds each of them in the rendered frame and
// fails if the layout ever grows a line.
//
// Everything below the editor moves with its height, so those rows are computed
// (see formBarRow) rather than named here.
const (
	formTitleLabelRow  = 2
	formTitleRow       = 3
	formAnnotRow       = 5
	formPromptLabelRow = 7
	formPromptRow      = 8
)

// formBarRow is the line the form's toolbar sits on: under the editor, with the
// attachment note and the session note on the two lines between them.
func (m model) formBarRow() int {
	return formPromptRow + m.promptArea.Height() + 2
}

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

	// The annotation bar — the prompt's own marks, between the title they
	// qualify and the body (annotbar.go). One line by construction, which
	// formAnnotRow and every row constant below it depend on.
	b.WriteString(m.annotBar())
	b.WriteString("\n\n")

	b.WriteString(promptStyle.Render("Prompt"))
	b.WriteString("\n")
	// The editor's own view with the selection painted over it. It renders
	// exactly the same number of lines either way (see promptEditorView), which
	// is what the toolbar's row depends on — a highlight must not move the
	// buttons out from under the pointer.
	b.WriteString(m.promptEditorView())
	b.WriteString("\n")

	// The attachment line is always shown, empty or not: an attachment editor
	// nobody knows about is one nobody uses, and "none" is also the answer to
	// "did my screenshot make it in". It no longer names ctrl+o — the toolbar's
	// ❐ Images chip, one line below, says that and is clickable besides.
	//
	// The 📎 glyph alone takes the attachment cyan, the count text stays grey:
	// the icon is the thing the eye looks for when scanning the form, and it is
	// the same hue the list's 📎N mark and the toolbar's ❐ Images chip carry, so
	// all three screens answer "images" with one color. Tinting the words too
	// would have made a standing line read as a status message — this line is
	// always here, and only the icon is the label.
	//
	// Two Renders rather than one styled string: each emits its own reset, which
	// is safe because this line is not run through a width wrap (unlike the ⚙
	// line below, where a pre-styled span would lose its reset at the wrap).
	b.WriteString(attachStyle.Render("📎"))
	b.WriteString(descStyle.Render(" " + firstNonEmpty(m.imageCountNote(), "no images")))
	b.WriteString("\n")

	// And the session line, on the same terms: always drawn, so "how will this
	// run" is answered on the form rather than only inside a panel — and so
	// "default session" is available as the answer.
	//
	// Trimmed to the pane, and that is load-bearing rather than tidy: a fully
	// configured prompt's summary runs past a hundred columns, and a line that
	// wrapped here would push the toolbar one row down from where every click on
	// it is hit-tested (see formBarRow).
	b.WriteString(descStyle.Render(m.fitToPane("⚙ "+m.sessionNote(), 0)))
	b.WriteString("\n")

	// The toolbar sits directly under the editor and above the error line, so its
	// row is a function of the editor's height alone (see formBarRow) — a failed
	// save must not move the buttons out from under the pointer.
	b.WriteString(m.formBar())
	b.WriteString("\n")

	// One line for both, with the error winning: the two never arise from the
	// same keystroke, and a save that failed is the more urgent thing to read.
	if m.formErr != "" {
		b.WriteString(errStyle.Render(m.formErr))
		b.WriteString("\n")
	} else if m.formNote != "" {
		b.WriteString(descStyle.Render(m.formNote))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.formFooter())
	return b.String()
}

// formActions is the form's toolbar, in order. It reuses listAction: the two bars
// are the same object — a labelled chip standing for a chord, tinted by
// consequence — and needsSel simply has nothing to say here, since a form always
// has something to act on.
//
// The order is the order a prompt's editing session ends: keep it, break the line
// you are on, mind how it is spelled, attach to it, say how it should run, hand
// it to an agent, throw it away.
//
// ☑ Spell sits third, among the things that are about the text rather than about
// what the prompt carries, and is the row's one checkbox: it shows the state of
// the check and flips it on the spot rather than opening a screen. See
// spellChipLabel for why that exception is the right one, and spellpanel.go for
// where the rest of the feature lives — the chip is the toggle's whole
// affordance, since ctrl+l now opens the panel instead of firing it. Send takes the accent for the reason listActions spends green on the
// list's Send — it is the one button on this screen that reaches out of the
// program — so Save gives the accent up and takes plain white instead: it is the
// row's default, and a default reads better as the one chip with no hue at all.
// Cancel keeps red as the only warning on the row.
//
// Send sits between Session and Cancel rather than beside Save. The two writes
// are not neighbours on purpose: Save is the button the hand goes to without
// looking, and a send is not undoable once an agent has the text, so the two
// worst mis-clicks to make adjacent are exactly those. Where it does sit reads
// as a sentence — set how it runs, then run it — and Cancel stays last, where a
// row of buttons is read from; nobody scans past the exit.
//
// Send is also the row's one chip with no chord (see sendForm): the hint column
// is empty rather than holding a key, so nothing typed into the form can fire it.
//
// Images is the palette's cyan and Session its blue, a pair chosen together (see
// colCyan): the two chips are the same gesture — "something else this prompt
// carries" — so they get neighbouring hues at one brightness rather than one
// tint each from opposite ends of the bar's set.
//
// Every icon is a one-cell text dingbat, for the reason listActions gives: the
// emoji forms measure as two columns but get drawn clipped at a cell edge, and no
// layout width fixes a terminal's own rasteriser. ❐ is the dingbat squares' pick
// for "images" — same font family as ✔ ✖, so the row stays one set. ✉ is the
// list bar's own Send glyph: the same act from a different screen, so it is the
// same chip.
func (m model) formActions() []listAction {
	return []listAction{
		{label: "✔ Save", hint: "ctrl+s", tint: colFgHi},
		{label: "↵ Newline", hint: "enter", tint: colStraw},
		{label: m.spellChipLabel(), tint: m.spellChipTint()},
		{label: "❐ Images", hint: "ctrl+o", tint: colCyan},
		{label: "⚙ Session", hint: "ctrl+r", tint: colInfo},
		{label: "✉ Send", tint: colAccent},
		{label: "✖ Cancel", hint: "esc", tint: colErr},
	}
}

// Indexes into formActions — used by clickFormBar to name what it is dispatching.
const (
	formActionSave = iota
	formActionNewline
	formActionSpell
	formActionImages
	formActionSession
	formActionSend
	formActionCancel
)

// formChips lays the toolbar out the way actionChips lays out the list's bar:
// the chip contents, and the half-open column span each one occupies, so the
// same description both draws the row and hit-tests it. The row starts at column
// 0, where the heading and both fields start — the list's bar is indented to
// clear its cursor gutter, and the form has none.
func (m model) formChips() []actionChip {
	acts := m.formActions()
	tier := m.formBarTier()

	chips := make([]actionChip, 0, len(acts))
	x := 0
	for i, a := range acts {
		if i > 0 {
			x += chipGap(tier) // the space between chips, if the pane can afford one
		}
		text := a.chipText(tier)
		w := lipgloss.Width(btnStyle.Render(text))
		chips = append(chips, actionChip{text: text, start: x, end: x + w})
		x += w
	}
	return chips
}

// formBarTier is how much of each button the form's toolbar is printing. The
// toolbar sits at column 0 — the form has no cursor gutter to clear — so it is
// measured with no indent.
//
// It reaches tierIcons sooner than the list's bar does, having seven buttons to
// the list's four: a seven-chip row needs about 108 columns to print its chords,
// 74 for its labels alone and 28 for its glyphs, so between the last two widths
// the toolbar is ✔ ↵ ☑ ❐ ⚙ ✉ ✖ and the footer is what names them.
func (m model) formBarTier() chipTier {
	return barTier(m.formActions(), m.width, 0)
}

// formBarShowsHints reports whether the toolbar is wide enough for every chip to
// carry its chord. The footer keys off the same answer — chords the chips teach
// stay off it, and the moment the chips go quiet the footer names them again.
func (m model) formBarShowsHints() bool {
	return m.formBarTier() == tierHints
}

// formBar renders the toolbar. Every chip is live — unlike the list's bar there
// is no such thing as a button with nothing to act on here — so each is drawn in
// btnStyle with its own hue dropped into the letters, derived rather than nested
// for the reason the list's bar gives.
//
// No chip carries a focus field: the form's tab ring is its two text fields, and
// lighting a button the keyboard cannot reach would promise a stop that isn't
// there. These are pointer targets that name their keys.
func (m model) formBar() string {
	acts := m.formActions()
	gap := strings.Repeat(" ", chipGap(m.formBarTier()))
	var b strings.Builder
	for i, c := range m.formChips() {
		if i > 0 {
			b.WriteString(gap)
		}
		b.WriteString(btnStyle.Foreground(lipgloss.Color(acts[i].tint)).Render(c.text))
	}
	return b.String()
}

// clickFormBar presses the toolbar button under the pointer, if it is on one.
// The gaps between chips are not buttons, so a miss does nothing.
func (m model) clickFormBar(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	for i, c := range m.formChips() {
		if msg.X < c.start || msg.X >= c.end {
			continue
		}
		switch i {
		case formActionSave:
			return m.saveForm()
		case formActionNewline:
			// A newline is an edit to the prompt, so the keys follow it there —
			// pressing this from the title field would otherwise put a line break
			// somewhere the user cannot see.
			cmd := m.focusForm(formFieldPrompt)
			m.promptArea.InsertString("\n")
			return m, cmd
		case formActionSpell:
			// The one chip on either bar that changes something rather than
			// opening something — see spellChipLabel for why the toggle is the
			// pointer's business and the panel the keyboard's.
			m, _ = m.toggleSpell()
			return m, nil
		case formActionImages:
			return m.beginImages()
		case formActionSession:
			return m.beginSession()
		case formActionSend:
			return m.sendForm()
		case formActionCancel:
			return m.cancelForm()
		}
	}
	return m, nil
}

// formFooter is the dimmed help line under the toolbar. It teaches the caret
// keys — the ones a text editor lives on and no chip can stand for, since they
// act at the cursor rather than on the form — and leads with the pointer, which
// is the whole reason the rest of the line is now optional.
//
// The chords the toolbar prints on its chips stay off it, for the reason
// listFooter gives: a hint one line under a button that says exactly the same
// thing taught nothing and cost a line of attention. In a pane too narrow for
// chip hints the footer is the only teacher left, so it names them again.
func (m model) formFooter() string {
	// The column mode takes the whole line while it is on. It is a mode, not a
	// chord — the keys mean something different for as long as it lasts — and a
	// footer still teaching the ordinary editor would be teaching the wrong
	// program. Everything it names is a key the mode itself owns
	// (updatePromptCarets); the exit comes last, where a mode's exit belongs.
	if m.carets.on {
		return footerStyle.Render(m.fitFooter([]string{
			"typing goes on every line", "backspace deletes", "←/→ moves them",
			"ctrl+a/e line ends", "esc ends",
		}))
	}
	var lines []string
	if tier := m.formBarTier(); tier != tierHints {
		// ctrl+l (the Spelling panel, spellpanel.go) is on this line as well as
		// the one below: a pane this narrow drops the tail of that line long
		// before it reaches the panel, and this line is what a narrow pane
		// reads.
		chords := []string{
			"ctrl+s save", "enter newline", "ctrl+o images", "ctrl+r session", "esc cancel", "ctrl+l spelling",
		}
		if tier == tierIcons {
			// Down to glyphs, ✉ is the one chip no chord on this line stands for
			// — Send is click-only — so it leads rather than rides at the end,
			// which is the end fitFooter drops segments from. The pane that
			// squeezed the labels off the bar is exactly the pane that will drop
			// the tail of this line, and the thing nothing else can teach must not
			// be what goes first.
			chords = append([]string{"✉ click sends"}, chords...)
		}
		lines = append(lines, m.fitFooter(chords))
	}
	// In the order they must survive a narrowing pane, which is the order they
	// are worth knowing: the pointer, then selecting and what the two chords
	// that read a selection do with it, then the two chords that move the caret
	// furthest per keystroke, then field switching. ctrl+x rides directly behind
	// ctrl+c because the two are one idea — a swept run is worth something —
	// and a reader who has just been told sweeping works wants both answers in
	// the same breath.
	// ↑/↓ and ←/→ are not on the line at all — a text box's arrow keys are the
	// one thing nobody has to be told, and the segments that go without saying
	// are what buy room for the ones that don't.
	//
	// Dragging to select is left out for that same reason, and it is the reason
	// rather than an omission: a line that already says the pointer places the
	// caret has said that the pointer works here, and sweeping one is what a hand
	// tries next without being asked. The shift chord is the half of the gesture
	// nobody guesses, so it is the half that gets the ink.
	//
	// "@ file" rides this line rather than the chords line above it because it
	// is not a chord: it is a character the editor already takes, given a
	// second meaning at a word start (see updateForm). Two segments were
	// tightened to make room for it in a 120-cell pane — "line ends" for "line
	// start/end", "places caret" for "places the caret" — so the field switch
	// at the end of the line still fits there.
	segs := []string{
		"click places caret", "shift+←/→ selects", "ctrl+c copies", "@ file",
		"ctrl+a/e line ends", "alt+←/→ word", "tab switch field",
	}
	// The context menu (promptmenu.go) is taught only while something is swept,
	// on the same principle the scope toggle below is advertised only where it
	// works: a gesture named on a screen it cannot act on is a segment spent
	// teaching nothing.
	//
	// Contextual is what makes it affordable at all. This line is full — its
	// seven standing segments come to exactly 118 cells, which is what lets the
	// field switch at the end survive a 120-cell pane — so a permanent eighth
	// concept could only be bought by tightening the seven already here past
	// what they can say. Shown only while a highlight is standing, it takes its
	// place beside the chord that reads one and pushes the tail out by a
	// segment or two; "tab switches field" is the right thing to spend, because
	// a hand that has just swept a run is asking what can be done with it, not
	// how to leave the field.
	//
	// It names the three items rather than any of their chords. The menu prints
	// ctrl+x on its own ✂ row (see viewPromptMenu), so the one gesture here
	// teaches every key behind it — which is the whole reason those three live
	// on a menu instead of on three chords nobody would guess.
	if _, _, ok := m.promptSelSpan(); ok && m.formFocus == formFieldPrompt {
		segs = append(segs[:3], append([]string{"right-click: split/sort/carets"}, segs[3:]...)...)
	}
	// Advertise the scope toggle only when it works (see the ctrl+g handler): an
	// only-mode launch pins the scope, so the hint would be a lie there. It goes
	// last because it is the one segment that isn't about moving the caret.
	if m.formMode == formAdd && m.project.available() && m.global.available() {
		segs = append(segs, "ctrl+g scope")
	}
	// The Spelling panel (spellpanel.go) is on this line as well as the chords
	// line, for the reason "@ file" is here at all: no chip on the toolbar
	// stands for it — ☑ Spell is the toggle, not the panel — so a pane wide
	// enough to silence the chords line would otherwise leave the key
	// unadvertised. After scope, because it is the one segment that is about the
	// editor rather than about this prompt, and so among the first that can be
	// given up when the pane narrows.
	//
	// The two line operations ride along for the same reason — no chip stands
	// for either — and in that order, because alt+↑/↓ (movePromptLines) always
	// arrives and cmd+d (duplicatePromptLine) is the one segment here that a
	// terminal may not be able to send at all. If the pane is narrow enough to
	// cost a segment, the chord that might never arrive is the right one to lose
	// first.
	//
	// The prompt library (promptpick.go) rides at the very tail, which is where
	// the skill's rule puts a new standing segment: this line is already full at
	// 120 cells, so a chord that is genuinely optional — the library is a
	// convenience, and its other way in is a '/' the user was going to type
	// anyway — goes where only a wide pane will ever read it.
	segs = append(segs, "ctrl+l spelling", "alt+↑/↓ move line", "cmd+d dup line", "ctrl+p prompt library")
	lines = append(lines, m.fitFooter(segs))

	for i, ln := range lines {
		lines[i] = footerStyle.Render(ln)
	}
	return strings.Join(lines, "\n")
}

// fitFooter joins help segments into one line that fits the pane, dropping them
// from the right until it does. Conceding beats wrapping: a wrapped footer pushes
// nothing out of place — the form's clickable rows are all above it — but it
// costs a line every frame in a pane already short enough to be squeezing the
// editor. The first segment always survives, however narrow the pane.
func (m model) fitFooter(segs []string) string {
	line := strings.Join(segs, " · ")
	for m.width > 0 && lipgloss.Width(line) > m.width && len(segs) > 1 {
		segs = segs[:len(segs)-1]
		line = strings.Join(segs, " · ")
	}
	return line
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

// sessValueLabel is how one row's current value is drawn: the value itself, or
// what the default means in words. "default" alone would be true and useless —
// what a reader wants to know is what happens if they leave the row alone, and
// for most of these rows that is "whatever the agent does normally".
func (m model) sessValueLabel(row int) string {
	o := m.formSession
	switch row {
	case sessRowModel:
		return firstNonEmpty(o.Model, "default")
	case sessRowEffort:
		return firstNonEmpty(o.Effort, "default")
	case sessRowPermission:
		return firstNonEmpty(o.Permission, "default")
	case sessRowClear:
		return yesNo(o.Clear)
	case sessRowContext:
		switch o.Context {
		case ctxLoad:
			return "/sess-load"
		case ctxUse:
			return "/sess-use"
		}
		return "none"
	case sessRowContextArg:
		switch o.Context {
		case ctxLoad:
			return "how many sessions back (blank: the last)"
		case ctxUse:
			return "which saved session to match"
		}
		return ""
	case sessRowFinish:
		switch o.Finish {
		case finishCommit:
			return "commit the work"
		case finishPush:
			return "commit and push"
		case finishWrap:
			return "run /sess-wrap"
		}
		return "nothing"
	case sessRowReviews:
		if len(o.Reviews) == 0 {
			return "none"
		}
		return strings.Join(o.Reviews, ", ")
	case sessRowRelease:
		return yesNo(o.Release)
	}
	return ""
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// The session panel's row labels and the note each one carries, in row order.
// The note is what the option actually does — a panel of bare enum names would
// make the user open the README to find out what "dontAsk" costs them.
var sessRowLabels = [sessRowCount]struct{ label, note string }{
	sessRowModel:      {"Model", "--model, on a new claude session"},
	sessRowEffort:     {"Effort", "--effort, on a new claude session"},
	sessRowPermission: {"Permission", "--permission-mode, on a new claude session"},
	sessRowClear:      {"Clear first", "send /clear before the prompt, in an existing pane"},
	sessRowContext:    {"Context", "load prior work before reading the prompt"},
	sessRowContextArg: {"", ""}, // its note is the mode's own (see viewSession)
	sessRowFiles:      {"Files", "enter adds · ctrl+x removes the last"},
	sessRowFinish:     {"Finish", "what to do once the work is done"},
	sessRowReviews:    {"Reviews", "run these first — ←/→ cycles the sets"},
	sessRowRelease:    {"Release", "and cut a release afterwards"},
}

// viewSession renders the session panel: one row per option, the cursor on the
// row the keys are acting on, and the two free-text rows showing the shared box
// in place of a value.
//
// The rows are drawn as label · value · note rather than as a form of boxes.
// Nine of the ten are one-of-a-few choices, and a choice is quicker to make from
// a value you can cycle than from a field you have to spell — which is also why
// only the two rows that cannot be enumerated get a box at all.
func (m model) viewSession() string {
	var b strings.Builder
	heading := titleStyle.Render("Session options")
	b.WriteString(heading)
	b.WriteString("  ")
	// The summary is every option joined, so it is the one thing here with no
	// bound at all — a fully configured prompt runs to well over a hundred
	// columns. It gets whatever the heading leaves and no more.
	b.WriteString(descStyle.Render(m.fitToPane(
		firstNonEmpty(m.formSession.summary(), "default session"), lipgloss.Width(heading)+2)))
	b.WriteString("\n\n")

	const labelWidth = 12
	for row := range sessRowCount {
		// Each row is built on its own before it joins the frame, so its note can
		// be measured against what is already on the line and dropped rather than
		// wrapped: these rows are a table, and one row wrapping puts every row
		// below it a line off from where the eye — and the next glance — expects.
		var line strings.Builder

		// The context argument is the context row's own second line: it belongs
		// to that choice, and giving it a label of its own would read as an
		// eleventh option rather than as the rest of the tenth.
		label := sessRowLabels[row].label
		if row == sessRowContextArg {
			label = ""
		}
		if row == m.sessCursor {
			line.WriteString(cursorStyle.Render(cursorGlyph))
		} else {
			line.WriteString("  ")
		}
		line.WriteString(nameStyle.Render(fmt.Sprintf("%-*s", labelWidth, label)))

		// The context rows go quiet where the commands they call don't exist.
		// Quiet, not disabled: the backlog travels, and the machine that runs
		// the prompt is not always the machine that wrote it.
		dimmed := !m.sessSkills && (row == sessRowContext || row == sessRowContextArg)
		valueStyle := nameSelStyle
		if row != m.sessCursor {
			valueStyle = nameStyle
		}
		if dimmed {
			valueStyle = descStyle
		}

		// Every value is trimmed to what the label column leaves: a model id, a
		// /sess-use pattern and a file list are all user text with no length to
		// rely on, and this is a table.
		value := func(s string) string { return valueStyle.Render(m.fitToPane(s, indentWidth+labelWidth)) }

		switch row {
		case sessRowContextArg:
			switch {
			case m.formSession.Context == ctxNone:
				// Nothing to argue about until a context mode is chosen.
				line.WriteString(descStyle.Render("—"))
			case row == m.sessCursor:
				line.WriteString(m.sessInput.View())
			default:
				line.WriteString(value(firstNonEmpty(m.formSession.ContextArg, "—")))
			}
		case sessRowFiles:
			switch {
			case row == m.sessCursor:
				line.WriteString(m.sessInput.View())
			case len(m.formSession.Files) > 0:
				line.WriteString(value(strings.Join(m.formSession.Files, ", ")))
			default:
				line.WriteString(value("none"))
			}
		default:
			line.WriteString(value(m.sessValueLabel(row)))
		}

		// The note is an aside about the row the cursor is on, so it goes on only
		// where there is room for it beside the value.
		note := sessRowLabels[row].note
		if row == sessRowContextArg {
			note = m.sessValueLabel(row)
		}
		if note != "" && row == m.sessCursor {
			if m.width <= 0 || lipgloss.Width(line.String())+3+lipgloss.Width(note) <= m.width {
				line.WriteString("   ")
				line.WriteString(descStyle.Render(note))
			}
		}
		line.WriteString("\n")

		// The files already on the list, under the box that adds to them —
		// there is no room for them beside it once it holds a path.
		if row == sessRowFiles && row == m.sessCursor {
			for _, f := range m.formSession.Files {
				line.WriteString("      ")
				line.WriteString(descStyle.Render("· " + m.fitToPane(f, 8)))
				line.WriteString("\n")
			}
		}
		b.WriteString(line.String())
	}

	if !m.sessSkills {
		b.WriteString("\n")
		b.WriteString(m.wrapToPane(descStyle,
			"no sess-* skills found here — the context rows still save, for the machine that runs the prompt"))
		b.WriteString("\n")
	}
	if m.sessStatus != "" {
		st := okStyle
		if m.sessStatusErr {
			st = errStyle
		}
		b.WriteString("\n")
		b.WriteString(m.wrapToPane(st, "• "+m.sessStatus))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	// Through fitFooter for the reason the form's footer is: a pane too narrow
	// for the whole line concedes a segment rather than wrapping one.
	b.WriteString(footerStyle.Render(m.fitFooter([]string{
		"↑/↓ row", "←/→ or space change", "enter/esc back to the form",
	})))
	b.WriteString("\n")
	b.WriteString(footerStyle.Render(m.fitFooter([]string{
		"nothing is saved until you save the form", "the launch flags ride on new claude sessions only",
	})))
	return b.String()
}

// fitToPane trims s to the columns left after `used` of them are spent, for a
// value drawn on one line of a table. It is a no-op before the first
// WindowSizeMsg, when there is no width to fit to.
func (m model) fitToPane(s string, used int) string {
	room := m.width - used
	if m.width <= 0 || room >= lipgloss.Width(s) {
		return s
	}
	if room < 4 {
		room = 4
	}
	return truncate(s, room)
}

// wrapToPane renders a whole-line note in st, wrapped to the pane rather than
// truncated: unlike a table value, a sentence that has to be read loses more by
// losing its end than by taking a second line.
func (m model) wrapToPane(st lipgloss.Style, s string) string {
	if m.width <= 0 {
		return st.Render(s)
	}
	return st.Width(m.width).Render(s)
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
	if td.Frozen {
		meta += " · " + frozenBadgeStyle.Render("frozen — will not do")
	}
	if sc := td.Schedule; sc != nil && !td.Done {
		when := "scheduled " + formatScheduleTime(sc.At, time.Now())
		if sc.Missed {
			meta += " · " + errStyle.Render("missed "+formatScheduleTime(sc.At, time.Now()))
		} else {
			meta += " · " + schedStyle.Render(when)
		}
	}
	// The annotations trail the state, the same order the row reads in — and
	// spelled out as well as drawn, because this is the screen with room for the
	// word and the one someone opens to find out what a mark on a row meant.
	//
	// Keyed off the label rather than the glyph: a column that has gone quiet on
	// the row still has something to say here. A closed quick win draws no apple
	// in the list (fruitMark), but this is the screen that answers "what was said
	// about this prompt", and the answer does not stop being true when the work
	// is finished — so the words go out in the slot's own style with whatever
	// glyph it still draws in front of them, and none where it draws none.
	for _, sl := range annotSlots {
		label := sl.label(td)
		if label == "" {
			continue
		}
		glyph, st, _ := sl.mark(td)
		if glyph != "" {
			label = glyph + " " + label
		}
		meta += " · " + st.Render(label)
	}
	b.WriteString(descStyle.Render(meta))
	b.WriteString("\n\n")

	b.WriteString(m.viewVP.View())
	b.WriteString("\n\n")
	b.WriteString(footerStyle.Render("↑/↓ scroll · enter edit · " + m.modEnter() + " drop · ctrl+o export · esc back"))
	return b.String()
}

func (m model) viewTarget() string {
	var b strings.Builder
	td, _ := m.resolve(m.dropTodo)
	title := firstNonEmpty(td.Title, firstLine(td.Prompt, 50))
	heading, foot := "Drop into…", "enter drop & run · "+m.modEnter()+" drop & pause (don't submit) · esc back"
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
	// panes — so it is sized before, and regardless of, the floor below. The
	// list's scroll window is sized here for the same reason: it answers to the
	// pane's HEIGHT, which a width too narrow for the form's fields says
	// nothing about, and a list left unwindowed on a narrow pane is exactly the
	// one that runs off the bottom.
	m.sizeSearchInput()
	m.sizeListWindow()
	w := m.width - 4
	if w < 20 {
		return
	}
	switch m.stage {
	case stageForm:
		m.titleInput.SetWidth(w)
		m.promptArea.SetWidth(w)
		if h := m.height - formChromeHeight; h >= 4 {
			m.promptArea.SetHeight(h)
		}
	case stageTarget:
		m.targetList.input.SetWidth(w)
	case stageImages:
		m.imgInput.SetWidth(w)
	case stageSchedule:
		m.schedInput.SetWidth(w)
	case stageSession:
		m.sessInput.SetWidth(sessInputWidth(m.width))
	case stageFiles:
		m.files.resize(m.width, m.height)
	case stageSnippets:
		m.snips.resize(m.width, m.height)
	case stageExport:
		m.exportList.input.SetWidth(w)
	case stageSpell:
		m.spellList.input.SetWidth(w)
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

// --- The View panel ---------------------------------------------------------
//
// Two switches about how the list is drawn, as against every other panel on the
// list stage, which is about one prompt. That is also why it is not on the
// action bar: every chip there acts on the highlighted row, and a panel that
// needs no row would be the one entry whose greying-out rule differed.

// The panel's rows, in the order they are drawn. The cursor is an index into
// this set, so the numbering is the layout and nothing else.
const (
	viewRowPriority   = iota // draw the list critical-first instead of by hand
	viewRowShowFrozen        // draw the ❄ prompts at all
	viewRowCount
)

// viewOptsRowsRow is the first line the panel's rows are drawn on: the title
// line, then a blank. Pinned by a test, because the hit test cannot re-measure
// the frame — anything gaining a line above the rows would aim every click one
// row off. The same contract listRowsRow and exportRowsRow carry.
const viewOptsRowsRow = 2

// The panel's labels and the note each row carries, in row order — the shape
// sessRowLabels uses, and for the same reason: a column of bare switches makes
// the user guess what flipping one costs them.
var viewRowLabels = [viewRowCount]struct{ label, note string }{
	viewRowPriority:   {"Priority order", "critical first inside each group — dragging and ctrl+↑/↓ are off while it is on"},
	viewRowShowFrozen: {"Frozen prompts", "the ❄ rows — work decided against, kept on the record"},
}

// beginViewOpts opens the panel. Unlike every other panel reachable from the
// list it needs no highlighted prompt, because it is about the list itself.
func (m model) beginViewOpts() (tea.Model, tea.Cmd) {
	m.viewOptsCursor = 0
	m.viewOptsNote = ""
	m.stage = stageViewOpts
	return m, nil
}

func (m model) updateViewOpts(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "enter":
		// Both close, and neither means anything different. Every switch here
		// has already taken and already been written, so there is nothing for
		// enter to confirm and nothing for esc to undo — which is the honest
		// shape for live preferences, as against the form, where esc means
		// "throw this away".
		m.backToList()
		return m, nil
	case "up", "ctrl+p", "shift+tab":
		m.viewOptsCursor = (m.viewOptsCursor - 1 + viewRowCount) % viewRowCount
		return m, nil
	case "down", "ctrl+n", "tab":
		m.viewOptsCursor = (m.viewOptsCursor + 1) % viewRowCount
		return m, nil
	case "left", "right", " ", "space":
		return m.toggleViewOptsRow(m.viewOptsCursor)
	}
	return m, nil
}

// toggleViewOptsRow flips one switch, redraws the list under the panel, and
// writes the choice.
//
// Written now rather than when the panel closes, for the reason toggleSpell
// writes immediately: a preference that only sticks if you leave by the
// approved door is a preference that silently does not stick.
//
// The highlight is captured and re-parked around the rebuild because flipping
// priority order resorts every group — without it the cursor would keep its
// index and so end up on a different prompt than the one it was left on.
func (m model) toggleViewOptsRow(row int) (tea.Model, tea.Cmd) {
	switch row {
	case viewRowPriority:
		m.orderByPriority = !m.orderByPriority
	case viewRowShowFrozen:
		m.showFrozen = !m.showFrozen
	default:
		return m, nil
	}
	ref, had := m.selectedRef()
	m.rebuildList()
	if had {
		m.selectRow(ref)
	}
	m.viewOptsNote = ""
	if err := m.saveViewPrefs(); err != nil {
		// The switch took for this run; only its memory failed. Said rather
		// than swallowed, because a panel reading "on" beside a file that still
		// says off is the wrong thing to be trusted.
		m.viewOptsNote = "not saved: " + err.Error()
	}
	return m, nil
}

// clickViewOpts flips the switch under the pointer, moving the cursor there
// first — the rule every other click in this program follows, so the pointer
// and the keyboard never act from two different places.
func (m model) clickViewOpts(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	row := msg.Y - viewOptsRowsRow
	if row < 0 || row >= viewRowCount {
		return m, nil
	}
	m.viewOptsCursor = row
	return m.toggleViewOptsRow(row)
}

// viewOptsOn reads one row's state.
func (m model) viewOptsOn(row int) bool {
	if row == viewRowShowFrozen {
		return m.showFrozen
	}
	return m.orderByPriority
}

// viewOptsValue is the value column. "on"/"off" rather than the session
// panel's "yes"/"no": those rows answer questions ("clear first?" — yes), and
// these are switches, and a switch says on.
func (m model) viewOptsValue(row int) string {
	if m.viewOptsOn(row) {
		return "on"
	}
	return "off"
}

func (m model) viewViewOpts() string {
	var b strings.Builder
	heading := titleStyle.Render("View")
	b.WriteString(heading)
	b.WriteString("  ")
	b.WriteString(descStyle.Render(m.fitToPane("how this list is drawn — kept between launches", lipgloss.Width(heading)+2)))
	b.WriteString("\n\n")

	const labelWidth = 16
	for row := range viewRowCount {
		// Built a row at a time so the note can be measured against what is
		// already on the line and dropped rather than wrapped: these rows are a
		// table, and one row wrapping puts every row below it a line off from
		// where the click hit test expects it.
		var line strings.Builder
		if row == m.viewOptsCursor {
			line.WriteString(cursorStyle.Render(cursorGlyph))
		} else {
			line.WriteString("  ")
		}
		line.WriteString(nameStyle.Render(fmt.Sprintf("%-*s", labelWidth, viewRowLabels[row].label)))

		valueStyle := nameStyle
		if row == m.viewOptsCursor {
			valueStyle = nameSelStyle
		}
		line.WriteString(valueStyle.Render(fmt.Sprintf("%-5s", m.viewOptsValue(row))))

		// The frozen row keeps its switch while the ctrl+d fold is on — it is a
		// preference that takes effect when the fold lifts, and a row disabled
		// for a reason it cannot show would be the panel lying about what it
		// stores. The note is what changes instead.
		note := viewRowLabels[row].note
		if row == viewRowShowFrozen && m.hideDone && !m.showFrozen {
			note = "the ctrl+d fold is hiding these and the completed ones"
		}
		if note != "" {
			line.WriteString(descStyle.Render(m.fitToPane(note, indentWidth+labelWidth+5)))
		}
		b.WriteString(line.String())
		b.WriteString("\n")
	}

	if m.viewOptsNote != "" {
		b.WriteString("\n")
		b.WriteString(m.wrapToPane(errStyle, "• "+m.viewOptsNote))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(footerStyle.Render(m.fitFooter([]string{
		"↑/↓ row", "←/→ or space toggle", "enter/esc back to the list",
	})))
	b.WriteString("\n")
	b.WriteString(footerStyle.Render(m.fitFooter([]string{
		"a prompt's own priority is set in its editor (ctrl+r)", "both switches are remembered between launches",
	})))
	return b.String()
}
