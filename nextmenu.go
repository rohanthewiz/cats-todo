// nextmenu.go — the Next List page's context menu.
//
// A Next List item is not a backlog prompt: it is a paragraph in a file this
// page only reads. Everything the add form offers (session options,
// attachments, the annotation marks, a schedule) is a setting on a backlog
// prompt, so none of it can be done to the item itself. What the menu can do is
// make that prompt, from the item, with the setting already applied. Every row
// below is therefore one of three things: open the draft form (on the panel
// asked for), send the item unsaved, or save it to the backlog in one press,
// marked:
//
//	╭────────────────────────────────╮
//	│ ✚ New prompt…            enter │   the draft form (nothing saved yet)
//	│ ⚙ Session…                     │   … with its session panel up
//	│ ◫ Images…                      │   … with its attachments editor up
//	│ ✉ Send…            shift+enter │   straight to an agent, unsaved
//	│ ◷ Schedule…                    │   save (or reuse the saved copy), schedule
//	│ ⤓ Add to backlog               │   saved in one press, no form
//	│ ⤓ Add as 🍏 quick win          │   … with one mark set
//	│ ⤓ Add as △ high priority       │
//	│ ⤓ Add as ▲ critical priority   │
//	│ ⤓ Add as ｉ info               │
//	│ ⤓ Add as ⚑ flagged             │
//	│ ⧉ Copy ID: N-014               │   the item's words, off the page
//	│ ⧉ Copy as prompt               │
//	╰────────────────────────────────╯
//
// Order runs from least to most committing, as the backlog's menu does: the
// form rows write nothing until the form saves, Send writes nothing at all,
// Schedule and the Add rows write a backlog prompt, and the copies — the quiet
// ones, which change nothing but the clipboard — close the box.
//
// Every saved prompt carries the item's value (the file rates items on the
// backlog's own three levels), exactly as the draft form does. The Add rows
// grey out once the target backlog already holds an open copy of the item
// (one whose prompt still cites it), since a second one would be two records
// of the same work for someone to close; ◷ Schedule… schedules that copy
// instead of making another.
//
// The box itself is menuBox (menu.go): the same placement, keys, drawing and
// dim-rather-than-omit rule as the backlog's menu (listmenu.go), so a menu
// learned on one page is learned on both. Rows with a chord print it.
// ↻ Refresh and ← Back are about the page, not the item, and stay on the bar,
// as the backlog's menu leaves Import off.

package main

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The menu's actions, which are also its row order (see the diagram above).
const (
	nextMenuPrompt = iota
	nextMenuSession
	nextMenuImages
	nextMenuSend
	nextMenuSchedule
	nextMenuAdd
	nextMenuAddFruit
	nextMenuAddHigh
	nextMenuAddCritical
	nextMenuAddInfo
	nextMenuAddFlag
	nextMenuCopyID
	nextMenuCopyPrompt
)

// nextMenuAddMark is what each Add row sets on the prompt it saves — a table
// rather than a switch, so the row order and the marks cannot drift apart (the
// turn listMenuPrio makes for the backlog menu's priority rows). The plain
// Add row sets nothing; the item's value is added to every row's set when it
// is pressed (see addNextItem).
var nextMenuAddMark = map[int]annots{
	nextMenuAdd:         {},
	nextMenuAddFruit:    {Fruit: true},
	nextMenuAddHigh:     {Priority: priorityHigh},
	nextMenuAddCritical: {Priority: priorityCritical},
	nextMenuAddInfo:     {Info: true},
	nextMenuAddFlag:     {Flag: true},
}

// nextMenu is the open menu: the shared box, plus the item it was opened on.
//
// The item is carried by value rather than re-read from the highlight on a
// press, for the backlog menu's reason (see listMenu): the labels were resolved
// from this item, and a row that says "Copy ID: N-014" must copy N-014. A
// refresh cannot happen under an open menu — the menu owns every key, and a
// click off it only dismisses — but holding the item makes that a non-question.
type nextMenu struct {
	menuBox
	item nextItem
}

// rightClickNext opens the menu on the item row the press landed on.
//
// The press moves the highlight too, the rule every click on this page obeys:
// the row the box is asking about is drawn selected while it is up, and the
// keyboard resumes from it afterwards. A right-click anywhere but an item row
// (a section heading, the bar, the query box) opens nothing — there is no item
// there to ask about — and closes a menu that was open, so the right button
// aimed off the rows is still a way out of one.
//
// Like the backlog's, it does not count toward a double-click: that is a left
// button gesture, and a right press that armed one would leave the page
// mid-gesture behind a menu about to be dismissed.
func (m model) rightClickNext(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	i, ok := m.next.list.rowAtLine(msg.Y - nextRowsRow)
	if !ok || !m.next.list.focusRow(i) {
		m.nextMenu = nextMenu{}
		return m, nil
	}
	it, ok := m.next.highlighted()
	if !ok {
		m.nextMenu = nextMenu{}
		return m, nil
	}
	return m.openNextMenu(msg, it)
}

// noBacklogWhy is beginAddWith's refusal, for the rows that need a backlog to
// write (or draft) into.
const noBacklogWhy = "no project backlog here — relaunch from a project directory, or with --global"

// openNextMenu builds the menu for one item and opens it at the pointer.
//
// Every row's availability is resolved here, once, from the helpers' own
// guards and in their words — contract 4, refuse in words, kept for the
// pointer as well as the keyboard. The helpers keep their guards as well: the
// menu's copy is what greys a row, theirs is what still holds if the world
// changed between the right-click and the press.
func (m model) openNextMenu(msg tea.MouseClickMsg, it nextItem) (tea.Model, tea.Cmd) {
	sc, canAdd := m.nextAddScope()

	// The draft rows need somewhere the form could save to.
	draftWhy := ""
	if !canAdd {
		draftWhy = noBacklogWhy
	}

	// Send's refusals are sendNextItem's.
	sendWhy := ""
	switch {
	case m.dropping:
		sendWhy = "a drop is still in progress…"
	case m.client == nil:
		sendWhy = "cats control socket unavailable — can't send to a session"
	}

	// The Add rows: a backlog to write into, and no open copy there already.
	// The copy's title is named so the refusal says where to look for it.
	addWhy := draftWhy
	_, dup, hasDup := m.nextBacklogCopy(it)
	if addWhy == "" && hasDup {
		addWhy = it.ID + " is already in the " + strings.ToLower(sc.String()) + " backlog as “" + truncate(dup.Title, 40) + "”"
	}

	// Schedule's are beginSchedule's socket guard, then the Add rows' backlog
	// guard — but not the duplicate one, since a copy already there is simply
	// the prompt it schedules.
	schedWhy := ""
	switch {
	case m.client == nil:
		schedWhy = "cats control socket unavailable — can't schedule a drop"
	case !canAdd:
		schedWhy = noBacklogWhy
	}

	add := func(act int, label string) menuItem {
		return menuItem{act: act, label: label, why: addWhy}
	}

	var mu nextMenu
	mu.open, mu.item = true, it
	mu.items = []menuItem{
		{act: nextMenuPrompt, label: "✚ New prompt…", hint: "enter", why: draftWhy},
		// The form's own two panels, reached without the extra click through
		// the form's toolbar. No chord to print: theirs belong to the form.
		{act: nextMenuSession, label: "⚙ Session…", why: draftWhy},
		{act: nextMenuImages, label: "◫ Images…", why: draftWhy},
		{act: nextMenuSend, label: "✉ Send…", hint: m.modEnter(), why: sendWhy},
		{act: nextMenuSchedule, label: "◷ Schedule…", why: schedWhy},
		add(nextMenuAdd, "⤓ Add to backlog"),
		// The marks in the annotation bar's order, wearing the glyphs the row
		// will draw, so the menu is a legend for what the press leaves behind.
		// Value is not among them: it is carried from the file on every row.
		add(nextMenuAddFruit, "⤓ Add as "+fruitGlyph+" quick win"),
		add(nextMenuAddHigh, "⤓ Add as "+prioHighGlyph+" high priority"),
		add(nextMenuAddCritical, "⤓ Add as "+prioCriticalGlyph+" critical priority"),
		add(nextMenuAddInfo, "⤓ Add as "+infoGlyph+" info"),
		add(nextMenuAddFlag, "⤓ Add as "+flagGlyph+" flagged"),
		// The ID is on the label so the row says exactly what the clipboard
		// will hold, and so the box names which item it is about, since it
		// floats off the row.
		{act: nextMenuCopyID, label: "⧉ Copy ID: " + it.ID},
		// The bytes ✉ Send delivers (nextItemPrompt), citation and all, so a
		// paste into an agent outside cats tells it the same thing a send
		// would: which item this is, and where to close it.
		{act: nextMenuCopyPrompt, label: "⧉ Copy as prompt"},
	}
	mu.cursor = mu.firstLive()
	mu.size()
	mu.place(msg.X, msg.Y, m.width, m.height)
	m.nextMenu = mu
	// The heading's note is the menu's answer channel (the status line is not
	// on this screen), so the last action's note goes now rather than being
	// read as a reply to this gesture.
	m.next.say("", false)
	return m, nil
}

// updateNextMenu is the keyboard while the menu is up — the shared walk (see
// menuBox.key), with this menu's own answer to a press.
func (m model) updateNextMenu(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.nextMenu.key(msg) {
	case menuKeyPress:
		return m.pressNextMenu(m.nextMenu.cursor)
	case menuKeyClose:
		m.nextMenu = nextMenu{}
	}
	return m, nil
}

// clickNextMenu is the pointer while the menu is up: a row presses it, the
// border does nothing, and anywhere else dismisses without acting — a click
// off a menu is "never mind", and the chip or row underneath must not also
// take it (see clickListMenu).
func (m model) clickNextMenu(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if !m.nextMenu.inside(msg.X, msg.Y) {
		m.nextMenu = nextMenu{}
		return m, nil
	}
	row, ok := m.nextMenu.hit(msg.X, msg.Y)
	if !ok {
		return m, nil
	}
	return m.pressNextMenu(row)
}

// pressNextMenu runs row i on the item the menu was opened for. The menu
// closes either way: a row that acted has nothing left to offer, and one that
// could not has said why on the heading, which the box may be covering.
func (m model) pressNextMenu(i int) (tea.Model, tea.Cmd) {
	if i < 0 || i >= len(m.nextMenu.items) {
		return m, nil
	}
	row, it := m.nextMenu.items[i], m.nextMenu.item
	m.nextMenu = nextMenu{}
	if !row.live() {
		m.next.say(row.why, false)
		return m, nil
	}
	switch row.act {
	case nextMenuPrompt:
		return m.promptFromNextItem(it)
	case nextMenuSession, nextMenuImages:
		// The draft form first, then its panel over it — the panel's esc lands
		// on the form, which is where the draft is, rather than back on this
		// page with the draft thrown away.
		next, cmd := m.promptFromNextItem(it)
		nm, ok := next.(model)
		if !ok || nm.stage != stageForm {
			return next, cmd
		}
		if row.act == nextMenuSession {
			return nm.beginSession()
		}
		return nm.beginImages()
	case nextMenuSend:
		return m.sendNextItem(it)
	case nextMenuSchedule:
		return m.scheduleNextItem(it)
	case nextMenuCopyID:
		m.next.say("copied "+it.ID, false)
		return m, copyTextToClipboard(it.ID)
	case nextMenuCopyPrompt:
		m.next.say("copied "+it.ID+" as a prompt", false)
		return m, copyTextToClipboard(nextItemPrompt(it))
	}
	if mark, ok := nextMenuAddMark[row.act]; ok {
		if _, ok := m.addNextItem(it, mark); ok {
			m.next.say("added "+it.ID+addedMarkNote(mark)+" to the "+m.nextAddScopeName()+" backlog", false)
		}
	}
	return m, nil
}

// addedMarkNote names the mark an Add row set, for the heading's note — the
// row can travel (a priority lens lifts it) and the ⚑ or 🍏 is on the list,
// out of sight, so the note is the one place the press is confirmed.
func addedMarkNote(a annots) string {
	switch {
	case a.Fruit:
		return " as a quick win"
	case a.Priority == priorityHigh:
		return " at high priority"
	case a.Priority == priorityCritical:
		return " at critical priority"
	case a.Info:
		return " as info"
	case a.Flag:
		return " flagged"
	}
	return ""
}

// --- Saving an item to the backlog ------------------------------------------

// nextAddScope is the backlog an item saved from this page lands in: the
// project's when there is one, else the global — beginAddWith's own default,
// so a one-press Add and the draft form agree on where a prompt goes.
func (m model) nextAddScope() (scope, bool) {
	switch {
	case m.project.available():
		return scopeProject, true
	case m.global.available():
		return scopeGlobal, true
	}
	return scopeProject, false
}

// nextAddScopeName is nextAddScope in words, for the notes — lower-cased,
// since scope.String() is capitalised for a heading and these are mid-sentence.
func (m model) nextAddScopeName() string {
	sc, _ := m.nextAddScope()
	return strings.ToLower(sc.String())
}

// nextBacklogCopy finds an open prompt in the target backlog that was made
// from this item: one whose prompt still opens with the item's citation line
// (nextItemCite), which every road off this page writes.
//
// Only an open or frozen copy counts. A done one was worked and closed, while
// the item is evidently still listed, so making another is the honest reading
// of pressing Add again. A copy whose first line was rewritten in the form is
// not recognised — the citation is the only fact that ties the two together.
func (m model) nextBacklogCopy(it nextItem) (todoRef, Todo, bool) {
	sc, ok := m.nextAddScope()
	if !ok {
		return todoRef{}, Todo{}, false
	}
	cite := nextItemCite(it.ID)
	for _, td := range m.storeFor(sc).todos {
		if !td.Done && strings.HasPrefix(td.Prompt, cite) {
			return todoRef{scope: sc, id: td.ID}, td, true
		}
	}
	return todoRef{}, Todo{}, false
}

// addNextItem saves the item to the backlog as a prompt — the one the draft
// form would have opened with (nextItemTitle, nextItemPrompt), with mark set
// and the item's value carried — and returns where it landed.
//
// The marks go on through annots.applyTo, the path saveForm uses, so a mark's
// side effects (info clearing a schedule, say) are the same from here as from
// the form. It appends, as the form's add does: the array order is the user's,
// and the newest prompt goes at the end of it.
//
// Refusals and failures are said on the page's heading, which is the line in
// view; ok false means one was said.
func (m *model) addNextItem(it nextItem, mark annots) (todoRef, bool) {
	sc, ok := m.nextAddScope()
	if !ok {
		m.next.say(noBacklogWhy, true)
		return todoRef{}, false
	}
	if lvl, err := normalizeValue(it.Value); err == nil {
		mark.Value = lvl
	}
	td := Todo{ID: newID(), Title: nextItemTitle(it), Prompt: nextItemPrompt(it), Created: time.Now()}
	mark.applyTo(&td)
	if err := m.storeFor(sc).add(td); err != nil {
		m.next.say("save failed: "+err.Error(), true)
		return todoRef{}, false
	}
	// The list behind this page is rebuilt now, so its count (and the pane
	// title's paw badge) is right the moment the page is left.
	m.rebuildList()
	return todoRef{scope: sc, id: td.ID}, true
}

// scheduleNextItem is ◷ Schedule…: a schedule is a note on a backlog prompt,
// so the item has to be one first. An open copy already in the backlog is the
// one scheduled; otherwise the item is added (value carried, no marks), which
// is a real save — backing out of the scheduler leaves the prompt in the
// backlog, where it would have been after ⤓ Add to backlog.
//
// The scheduler is the list's, and it returns to the list when it is done, so
// this is the one row that leaves the page for good; the list lands with the
// prompt highlighted (listFocus), which is where the eye should be after
// scheduling it. beginSchedule's own refusals (a frozen or info copy) are then
// said on the list's status line, the screen they arrive on.
func (m model) scheduleNextItem(it nextItem) (tea.Model, tea.Cmd) {
	if m.client == nil {
		m.next.say("cats control socket unavailable — can't schedule a drop", true)
		return m, nil
	}
	ref, _, ok := m.nextBacklogCopy(it)
	if !ok {
		if ref, ok = m.addNextItem(it, annots{}); !ok {
			return m, nil
		}
	}
	m.listFocus = ref
	m.backToList()
	return m.beginSchedule()
}

// overlayNextMenu floats the menu over the page's rendered frame (see
// overlayMenu, menu.go, for why it is composited rather than spliced).
func (m model) overlayNextMenu(view string) string {
	return overlayMenu(view, m.nextMenu.menuBox)
}
