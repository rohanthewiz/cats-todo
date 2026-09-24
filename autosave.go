// autosave.go — the prompt editor's timed autosave.
//
// The form only writes on ✔ Save (or ✉ Send, or the title's road home), so
// everything typed since the form opened lived only in this process. If the pane
// closed, cats restarted or the machine slept for good, that work was gone. The
// autosave caps the loss at one delay's worth of typing: 45 seconds unless
// settings.json says otherwise (see defaultAutosave and autosaveFromSeconds).
//
// The timer is a throttle, not a debounce. It is armed by the first change
// since the last write and fires one delay later whether or not the typing has
// stopped. A debounce ("45 seconds after the last keystroke") never fires for
// someone writing steadily, and a steady writer has the most to lose:
//
//	edit ──► arm (gen n) ──── delay ────► tick(n) ──► write ──► saved = sig
//	  ▲                                                          │
//	  └───────────── the next change re-arms (gen n+1) ◄─────────┘
//
// "A change" means the fingerprint of what a save would write (formSig) differs
// from the one last written. Watching the fingerprint instead of individual keys
// means no editing path can forget to report itself. That includes the prompt's
// twenty-odd operations, the annotation bar, the ⚙ panel and a snippet pasted in
// from another stage. It is the same argument the undo history's choke point
// makes (see Update), and it runs in the same place.
//
// What an autosave writes, and what it leaves for ✔ Save:
//
//   - Title, prompt, session options and annotations are written. On an add, the
//     first autosave creates the todo and the form becomes an edit of it, so the
//     next write updates that todo instead of adding a second one.
//   - Attachments are left alone. Attaching copies files into images/<id>/ and
//     detaching deletes them, and doing either on a timer behind someone who is
//     still deciding would create or destroy files they never confirmed. ✔ Save
//     reconciles the list exactly as before. The edit path attaches whatever is
//     still pending, which after an add-turned-edit is every image.
//
// esc (✖ Cancel) keeps its meaning: "leave, keeping nothing from this form".
// The autosave is a safety net under the form, not a second save button, so
// cancel takes back whatever the timer wrote (revertAutosave). An autosaved add
// is deleted, and an autosaved edit gets the todo's original text, options and
// marks restored. Only a form that never reached esc (a killed pane, a crash)
// leaves its autosave standing, and that is the case the feature is for.
//
// The stale-tick problem is solved as in listhover.go. A tea.Cmd cannot be
// cancelled once issued, so every arming carries a generation number, and a
// tick whose number is not the current one does nothing. Leaving the form bumps
// the generation, so a tick still in flight from a form that has since closed
// cannot write into whatever form is open when it lands.

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// defaultAutosave is how long a change may sit unsaved when settings.json does
// not say otherwise (autosaveSeconds; see settings.go). It balances two costs.
// A shorter wait means a reload-and-save of todos.json every few seconds of
// typing, with the list in another pane redrawing each time. A longer one means
// a crash takes more work with it. 45 seconds is short enough that losing that
// much typing is a small annoyance.
const defaultAutosave = 45 * time.Second

// minAutosave is the shortest wait settings.json can ask for (see
// autosaveFromSeconds).
const minAutosave = 5 * time.Second

// formAutosave is the form's autosave state. The zero value is "no form open",
// which is what backToList leaves behind.
type formAutosave struct {
	live  bool   // a form is open and its edits are being watched
	armed bool   // a tick for gen is in flight
	gen   uint64 // never reset, only bumped, so an old tick can never match a new form
	saved string // formSig as of the last write, or as of opening when nothing was written

	// wrote says the timer has written this form at least once. That is the
	// only case in which cancel has anything to take back.
	wrote bool
	// added says the todo on disk exists only because an autosave created it.
	// The form began as an add and is now an edit of that todo. Cancel deletes
	// it instead of restoring it, and ✔ Save still reports an add.
	added bool
	// orig is the todo as an edit form found it: what cancel restores after an
	// autosave. Unused on an add, since there was nothing to restore.
	orig Todo
}

// autosaveTickMsg is a delay that has run out, tagged with the arming it
// belongs to (see the generation note in the file header).
type autosaveTickMsg struct{ gen uint64 }

func autosaveTick(gen uint64, delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg { return autosaveTickMsg{gen: gen} })
}

// formSig fingerprints everything an autosave writes, as the user typed it.
// It uses the raw title rather than the derived one, so clearing a title
// counts as a change. Attachments are left out on purpose: the autosave does
// not write them (see the header), so adding one must not trigger a write that
// would not include it.
//
// The session record is JSON-encoded because it holds slices, which Go cannot
// compare with ==. annots is a flat struct, so %+v is exact.
func (m model) formSig() string {
	sess, _ := json.Marshal(m.formSession)
	return m.titleInput.Value() + "\x00" + m.promptArea.Value() + "\x00" +
		string(sess) + "\x00" + fmt.Sprintf("%+v", m.formAnnots)
}

// startAutosave begins watching a form that has just been filled in, taking its
// contents as the baseline, so opening a form is not itself a change. orig is
// the todo being edited, or nil for an add.
func (m *model) startAutosave(orig *Todo) {
	gen := m.autosave.gen + 1
	m.autosave = formAutosave{live: true, gen: gen, saved: m.formSig()}
	if orig != nil {
		m.autosave.orig = *orig
		// The copy gets its own session record. A Todo's Session is a pointer
		// into the store's slice, and the restore must write the options as
		// they were when the form opened, not as they are after later changes.
		if orig.Session != nil {
			s := orig.Session.clone()
			m.autosave.orig.Session = &s
		}
	}
}

// stopAutosave stops watching and invalidates any tick in flight. backToList
// calls it, so every way off the form ends the watch.
func (m *model) stopAutosave() {
	m.autosave = formAutosave{gen: m.autosave.gen + 1}
}

// formIsNew reports whether the open form is, from the user's point of view,
// adding a prompt. That is true even after the first autosave has turned it
// into an edit of the todo it created: the user has not saved anything.
func (m model) formIsNew() bool {
	return m.formMode == formAdd || m.autosave.added
}

// watchAutosave runs after every message while a form is open. It arms the
// timer when the form differs from what was last written and no tick is already
// on its way. An armed timer is left alone, which is what makes this a throttle
// (see the header): later changes ride on the tick already in flight.
//
// Blinks and pointer motion reach this too. The cost is one string comparison
// of the prompt per message, the same order as the undo snapshot Update
// already takes on these stages.
func watchAutosave(next tea.Model, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	m, ok := next.(model)
	// A zero delay is the autosave turned off in settings.json. The watch
	// still starts and stops with the form, so formIsNew and cancel behave the
	// same either way; it just never arms, so nothing is ever written.
	if !ok || !m.autosave.live || m.autosave.armed || m.autosaveEvery <= 0 {
		return next, cmd
	}
	if m.formSig() == m.autosave.saved {
		return next, cmd
	}
	m.autosave.armed = true
	return m, tea.Batch(cmd, autosaveTick(m.autosave.gen, m.autosaveEvery))
}

// fireAutosave answers a tick. A stale tick, from a form since closed or from
// an arming superseded by a write, is dropped. A current one writes if the form
// still differs from disk. Typing that was undone back to the saved text leaves
// nothing to write.
func (m model) fireAutosave(msg autosaveTickMsg) (tea.Model, tea.Cmd) {
	if !m.autosave.live || msg.gen != m.autosave.gen {
		return m, nil
	}
	m.autosave.armed = false
	if m.formSig() == m.autosave.saved {
		return m, nil
	}
	m.autosaveForm()
	return m, nil
}

// autosaveForm writes the form without leaving it. It is persistForm's text,
// options and marks half. The attachments are the half it skips, and the stage
// change is the other thing it leaves out: persistForm ends on the list, and an
// autosave must not move the screen under someone typing.
//
// Failures go on the form's error line, and the baseline is left as it was, so
// the next message re-arms the timer and the write is retried one delay later
// instead of the edit being marked saved when it is not.
func (m *model) autosaveForm() {
	sig := m.formSig()
	title := strings.TrimSpace(m.titleInput.Value())
	prompt := strings.TrimSpace(m.promptArea.Value())
	st := m.storeFor(m.formScope)
	// There is nothing writable to save, either because the prompt is empty
	// (persistForm would refuse it) or because the backlog is unavailable. In
	// both cases stay quiet. ✔ Save explains its own refusal when it is pressed,
	// and an error appearing unprompted on every tick would be nagging. The
	// baseline moves so the timer re-arms only on the next real change.
	if prompt == "" || !st.available() {
		m.autosave.saved = sig
		return
	}
	if title == "" {
		title = firstLine(prompt, 60)
	}

	if m.formMode == formAdd {
		id := newID()
		td := Todo{
			ID: id, Title: title, Prompt: prompt,
			Session: sessionPtr(m.formSession),
			Created: time.Now(),
		}
		m.formAnnots.applyTo(&td)
		if err := st.add(td); err != nil {
			m.formErr = "autosave failed: " + err.Error()
			return
		}
		// From here on the form edits the todo it just created. The next write,
		// autosave or ✔ Save, takes the edit path. formImagesOrig stays empty
		// because the new todo owns no attachments yet, so ✔ Save attaches every
		// image still pending and has nothing to detach.
		m.formMode = formEdit
		m.editID = id
		m.autosave.added = true
	} else {
		// The same three writes as persistForm's edit path, minus setImages. The
		// order and the stop-at-first-failure rule are the same too.
		if err := st.update(Todo{ID: m.editID, Title: title, Prompt: prompt}); err != nil {
			m.formErr = "autosave failed: " + err.Error()
			return
		}
		if err := st.setSession(m.editID, sessionPtr(m.formSession)); err != nil {
			m.formErr = "autosave failed: " + err.Error()
			return
		}
		if err := st.setAnnots(m.editID, m.formAnnots); err != nil {
			m.formErr = "autosave failed: " + err.Error()
			return
		}
	}
	m.autosave.saved = sig
	m.autosave.wrote = true
	// A previous autosave failure is no longer true.
	if strings.HasPrefix(m.formErr, "autosave failed: ") {
		m.formErr = ""
	}
	m.formNote = "autosaved " + time.Now().Format("15:04")
	// Keep the list in step with disk. It is not on screen, but a form can be
	// left by roads that do not rebuild on the way (esc after a failed revert,
	// a stage change from a scheduled drop). The list must never show a backlog
	// older than the file.
	m.listFocus = todoRef{scope: m.formScope, id: m.editID}
	m.rebuildList()
}

// revertAutosave takes back what the timer wrote. cancelForm calls it before
// leaving, so esc discards the whole editing session even when parts of it
// already reached disk. It does nothing for a form that was never autosaved.
//
// Attachments need no restoring: the autosave never wrote them.
//
// A failed revert is reported on the list's status line instead of blocking the
// way out. The autosaved version is still a whole, valid todo. It is just
// newer than the user wanted, and trapping them on a form they asked to leave
// would be the worse outcome.
func (m *model) revertAutosave() {
	if !m.autosave.wrote {
		return
	}
	st := m.storeFor(m.formScope)
	var err error
	if m.autosave.added {
		err = st.delete(m.editID)
	} else {
		o := m.autosave.orig
		err = st.update(Todo{ID: o.ID, Title: o.Title, Prompt: o.Prompt})
		if err == nil {
			err = st.setSession(o.ID, o.Session)
		}
		if err == nil {
			err = st.setAnnots(o.ID, annotsOf(o))
		}
	}
	m.rebuildList()
	if err != nil {
		m.setStatus("cancelled, but the autosaved copy could not be taken back: "+err.Error(), true)
		return
	}
	m.setStatus("cancelled · autosaved changes taken back", false)
}
