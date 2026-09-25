package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The sample list's first two items sit two and three lines below
// nextRowsRow: the Open section's spacer and heading come first.
const (
	nextN001Y = nextRowsRow + 2
	nextN002Y = nextRowsRow + 3
)

// rightClickNextAt opens the page's menu with a right-click on screen line y,
// through Update so updateMouse's routing is under test too.
func rightClickNextAt(t *testing.T, m model, y int) model {
	t.Helper()
	next, _ := m.Update(tea.MouseClickMsg{X: 8, Y: y, Button: tea.MouseRight})
	got := next.(model)
	if !got.nextMenu.open {
		t.Fatalf("a right-click on line %d did not open the menu", y)
	}
	return got
}

// nextMenuRow is the index of the menu row whose action is act.
func nextMenuRow(t *testing.T, m model, act int) int {
	t.Helper()
	for i, it := range m.nextMenu.items {
		if it.act == act {
			return i
		}
	}
	t.Fatalf("menu has no row for action %d", act)
	return -1
}

// TestNextMenuOpensOnTheRightButton is the feature: a right-click on an item
// opens the menu on that item, moves the highlight there, names the item on
// its copy row, and is drawn over the page.
func TestNextMenuOpensOnTheRightButton(t *testing.T) {
	m, _ := nextModel(t, sampleNextList)
	m = openNext(t, m)
	m = rightClickNextAt(t, m, nextN002Y)

	if m.nextMenu.item.ID != "N-002" {
		t.Errorf("menu opened on %q, want N-002", m.nextMenu.item.ID)
	}
	if it, _ := m.next.highlighted(); it.ID != "N-002" {
		t.Errorf("highlight on %q, want it moved to the right-clicked N-002", it.ID)
	}
	frame := ansi.Strip(m.renderStage())
	for _, want := range []string{
		"✚ New prompt…", "⚙ Session…", "◫ Images…", "✉ Send…", "◷ Schedule…",
		"⤓ Add to backlog", "⤓ Add as 🍏 quick win", "⤓ Add as ▲ critical priority", "⤓ Add as ⚑ flagged",
		"⧉ Copy ID: N-002", "⧉ Copy as prompt",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("drawn page is missing the row %q:\n%s", want, frame)
		}
	}
}

// TestNextMenuOnlyOverAnItem: the bar, a section heading and the query box
// open nothing, and a right-click there closes a menu that was open.
func TestNextMenuOnlyOverAnItem(t *testing.T) {
	m, _ := nextModel(t, sampleNextList)
	m = openNext(t, m)
	for _, y := range []int{nextBarRow, nextRowsRow + 1, 2} {
		next, _ := m.Update(tea.MouseClickMsg{X: 8, Y: y, Button: tea.MouseRight})
		if next.(model).nextMenu.open {
			t.Errorf("a right-click on line %d opened a menu, want only item rows to", y)
		}
	}
	m = rightClickNextAt(t, m, nextN001Y)
	next, _ := m.Update(tea.MouseClickMsg{X: 8, Y: nextBarRow, Button: tea.MouseRight})
	if next.(model).nextMenu.open {
		t.Error("a right-click off the rows left the menu open")
	}
}

// TestNextMenuDimsSendWithoutASocket: with no cats socket the Send row is
// drawn dim, the cursor opens on the first live row, and pressing Send says
// why on the heading instead of opening the picker.
func TestNextMenuDimsSendWithoutASocket(t *testing.T) {
	m, _ := nextModel(t, sampleNextList)
	m = openNext(t, m)
	m = rightClickNextAt(t, m, nextN001Y)
	send := m.nextMenu.items[nextMenuRow(t, m, nextMenuSend)]
	if send.live() || !strings.Contains(send.why, "socket unavailable") {
		t.Fatalf("Send row = %+v, want it dim with the socket refusal", send)
	}
	next, _ := m.pressNextMenu(nextMenuRow(t, m, nextMenuSend))
	m = next.(model)
	if m.stage != stageNextList || m.nextMenu.open || !strings.Contains(m.next.note, "socket unavailable") {
		t.Errorf("pressing a dim Send: stage=%v open=%v note=%q, want a refusal on the page", m.stage, m.nextMenu.open, m.next.note)
	}

	// With a socket it is live, like the chord.
	m.client = &catsClient{socket: "/nonexistent/cats.sock"}
	m = rightClickNextAt(t, m, nextN001Y)
	if !m.nextMenu.items[nextMenuRow(t, m, nextMenuSend)].live() {
		t.Error("Send is dim with a socket and nothing in flight")
	}
}

// TestNextMenuRunsTheRow: each row does what its chord or label says, on the
// item the menu was opened for.
func TestNextMenuRunsTheRow(t *testing.T) {
	base, _ := nextModel(t, sampleNextList)
	base.client = &catsClient{socket: "/nonexistent/cats.sock"}
	base = openNext(t, base)
	base = rightClickNextAt(t, base, nextN002Y)

	t.Run("new prompt", func(t *testing.T) {
		next, _ := base.pressNextMenu(nextMenuRow(t, base, nextMenuPrompt))
		m := next.(model)
		if m.stage != stageForm || m.titleInput.Value() != "N-002 Seeds ship stale prompts." {
			t.Errorf("stage=%v title=%q, want the add form on N-002", m.stage, m.titleInput.Value())
		}
	})
	t.Run("send", func(t *testing.T) {
		next, _ := base.pressNextMenu(nextMenuRow(t, base, nextMenuSend))
		m := next.(model)
		if m.stage != stageTarget {
			t.Fatalf("stage = %v, want the target picker", m.stage)
		}
		if td, ok := m.dropSubject(); !ok || !strings.HasPrefix(td.Prompt, "Next list item N-002 ") {
			t.Errorf("picker is sending %q, want N-002", td.Prompt)
		}
	})
	t.Run("copy id", func(t *testing.T) {
		next, cmd := base.pressNextMenu(nextMenuRow(t, base, nextMenuCopyID))
		m := next.(model)
		if cmd == nil || m.next.note != "copied N-002" || m.nextMenu.open {
			t.Errorf("cmd=%v note=%q open=%v, want a clipboard write and a note", cmd != nil, m.next.note, m.nextMenu.open)
		}
	})
	t.Run("copy as prompt", func(t *testing.T) {
		next, cmd := base.pressNextMenu(nextMenuRow(t, base, nextMenuCopyPrompt))
		m := next.(model)
		if cmd == nil || m.next.note != "copied N-002 as a prompt" || m.stage != stageNextList {
			t.Errorf("cmd=%v note=%q stage=%v, want a clipboard write and a note on the page", cmd != nil, m.next.note, m.stage)
		}
	})
	t.Run("by a click on the row", func(t *testing.T) {
		b := base.nextMenu.menuBox
		// Row 0 (New prompt) is one line under the box's top border.
		next, _ := base.Update(tea.MouseClickMsg{X: b.x + 2, Y: b.y + 1, Button: tea.MouseLeft})
		if m := next.(model); m.stage != stageForm {
			t.Errorf("clicking ✚ New prompt: stage = %v, want the add form", m.stage)
		}
	})
}

// TestNextMenuOwnsTheKeys: while it is up, ↓ walks it and enter presses the
// row; any other key closes it and is swallowed — ctrl+r does not refresh
// from behind the box.
func TestNextMenuOwnsTheKeys(t *testing.T) {
	m, root := nextModel(t, sampleNextList)
	m = openNext(t, m)
	m = rightClickNextAt(t, m, nextN001Y)

	// Edit the file so a refresh that leaked through would be visible.
	writeNextList(t, root, strings.Replace(sampleNextList, "N-002", "N-099", 1))
	m = pressNext(t, m, "ctrl+r")
	if m.nextMenu.open || m.stage != stageNextList {
		t.Fatalf("ctrl+r on the menu: open=%v stage=%v, want it closed and still on the page", m.nextMenu.open, m.stage)
	}
	if len(m.next.items) < 2 || m.next.items[1].ID != "N-002" {
		t.Error("ctrl+r refreshed the page from behind the menu")
	}

	// end onto Copy as prompt, ↑ onto Copy ID, enter copies.
	m = rightClickNextAt(t, m, nextN001Y)
	m = pressNext(t, m, "end")
	m = pressNext(t, m, "up")
	next, cmd := m.Update(pressKey("enter"))
	m = next.(model)
	if cmd == nil || m.next.note != "copied N-001" {
		t.Errorf("↓↓ enter: note=%q, want N-001's ID copied", m.next.note)
	}
}

// TestNextMenuClickOffDismissesWithoutActing: a click off the box closes it,
// and the chip under the pointer is not also pressed.
func TestNextMenuClickOffDismissesWithoutActing(t *testing.T) {
	m, _ := nextModel(t, sampleNextList)
	m = openNext(t, m)
	m = rightClickNextAt(t, m, nextN001Y)
	chip := m.nextChips()[nextActionBack]
	next, _ := m.Update(tea.MouseClickMsg{X: chip.start + 1, Y: nextBarRow, Button: tea.MouseLeft})
	m = next.(model)
	if m.nextMenu.open || m.stage != stageNextList {
		t.Errorf("click on ← Back under an open menu: open=%v stage=%v, want the menu gone and the page kept", m.nextMenu.open, m.stage)
	}
}

// TestNextMenuDiesWithItsPage: a resize, or leaving the page, takes the box
// down, so it cannot greet the next visit by swallowing its first key.
func TestNextMenuDiesWithItsPage(t *testing.T) {
	m, _ := nextModel(t, sampleNextList)
	m = openNext(t, m)
	m = rightClickNextAt(t, m, nextN001Y)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 28})
	if next.(model).nextMenu.open {
		t.Error("the menu survived a resize")
	}

	m = rightClickNextAt(t, m, nextN001Y)
	m.backToList()
	if m.nextMenu.open {
		t.Error("the menu survived leaving the page")
	}
}

// TestNextMenuRefusesTheHoverCard: the card is not built while the menu is
// up, by an arrival or by a dwell armed just before the right-click.
func TestNextMenuRefusesTheHoverCard(t *testing.T) {
	m, _ := nextModel(t, sampleNextList)
	m = openNext(t, m)
	m = rightClickNextAt(t, m, nextN001Y)

	next, _ := m.Update(tea.MouseMotionMsg{X: 6, Y: nextN002Y})
	m = next.(model)
	if m.hover.open || m.hoverPend.armed {
		t.Error("motion under an open menu armed or built a card")
	}

	m.hoverGen++
	m.hoverPend = hoverPending{armed: true, stage: stageNextList, row: 1, x: 6, y: nextN001Y, gen: m.hoverGen}
	next, _ = m.Update(hoverTickMsg{gen: m.hoverGen})
	if next.(model).hover.open {
		t.Error("a dwell landed a card beside the open menu")
	}
}

// TestNextMenuOpensTheFormsPanels: ⚙ Session… and ◫ Images… open the draft
// form with that panel up, so esc from the panel lands on the draft.
func TestNextMenuOpensTheFormsPanels(t *testing.T) {
	base, _ := nextModel(t, sampleNextList)
	base = openNext(t, base)
	base = rightClickNextAt(t, base, nextN002Y)
	for act, want := range map[int]uiStage{nextMenuSession: stageSession, nextMenuImages: stageImages} {
		next, _ := base.pressNextMenu(nextMenuRow(t, base, act))
		m := next.(model)
		if m.stage != want || m.formNextID != "N-002" || m.titleInput.Value() != "N-002 Seeds ship stale prompts." {
			t.Errorf("row %d: stage=%v formNextID=%q title=%q, want stage %v over N-002's draft",
				act, m.stage, m.formNextID, m.titleInput.Value(), want)
		}
		if m = pressNext(t, m, "esc"); m.stage != stageForm {
			t.Errorf("row %d: esc from the panel went to %v, want the draft form", act, m.stage)
		}
	}
}

// TestNextMenuAddsToTheBacklog: each Add row saves the item as the prompt the
// form would have made, with its mark set and the item's value carried, and
// stays on the page; a second Add is refused while that copy is open.
func TestNextMenuAddsToTheBacklog(t *testing.T) {
	cases := []struct {
		act  int
		note string
		want func(Todo) bool
	}{
		{nextMenuAdd, "added N-002 to the project backlog", func(td Todo) bool { return !td.Fruit && td.Priority == "" && !td.Info && !td.Flag }},
		{nextMenuAddFruit, "added N-002 as a quick win to the project backlog", func(td Todo) bool { return td.Fruit }},
		{nextMenuAddHigh, "added N-002 at high priority to the project backlog", func(td Todo) bool { return td.Priority == priorityHigh }},
		{nextMenuAddCritical, "added N-002 at critical priority to the project backlog", func(td Todo) bool { return td.Priority == priorityCritical }},
		{nextMenuAddInfo, "added N-002 as info to the project backlog", func(td Todo) bool { return td.Info }},
		{nextMenuAddFlag, "added N-002 flagged to the project backlog", func(td Todo) bool { return td.Flag }},
	}
	for _, c := range cases {
		m, _ := nextModel(t, sampleNextList)
		m = openNext(t, m)
		m = rightClickNextAt(t, m, nextN002Y)
		next, _ := m.pressNextMenu(nextMenuRow(t, m, c.act))
		m = next.(model)
		if m.stage != stageNextList || m.next.note != c.note {
			t.Errorf("row %d: stage=%v note=%q, want the page with %q", c.act, m.stage, m.next.note, c.note)
		}
		if len(m.project.todos) != 1 {
			t.Fatalf("row %d: project backlog has %d todos, want 1", c.act, len(m.project.todos))
		}
		td := m.project.todos[0]
		if td.Title != "N-002 Seeds ship stale prompts." || !strings.HasPrefix(td.Prompt, "Next list item N-002 (") {
			t.Errorf("row %d: saved %q / %q, want the draft form's title and prompt", c.act, td.Title, td.Prompt)
		}
		if td.valueLevel() != valueHigh || !c.want(td) {
			t.Errorf("row %d: saved %+v, want N-002's high value and the row's mark", c.act, td)
		}

		// The copy is open, so every Add row is now dim and says where it is.
		m = rightClickNextAt(t, m, nextN002Y)
		add := m.nextMenu.items[nextMenuRow(t, m, nextMenuAdd)]
		if add.live() || !strings.Contains(add.why, "already in the project backlog") {
			t.Errorf("row %d: Add after adding = %+v, want it dim naming the copy", c.act, add)
		}
	}
}

// TestNextMenuAddAgainAfterDone: a done copy does not block another Add — the
// work was closed, yet the item is still listed.
func TestNextMenuAddAgainAfterDone(t *testing.T) {
	m, _ := nextModel(t, sampleNextList)
	m = openNext(t, m)
	m = rightClickNextAt(t, m, nextN002Y)
	next, _ := m.pressNextMenu(nextMenuRow(t, m, nextMenuAdd))
	m = next.(model)
	if err := m.project.setDone(m.project.todos[0].ID, true); err != nil {
		t.Fatal(err)
	}
	m = rightClickNextAt(t, m, nextN002Y)
	if !m.nextMenu.items[nextMenuRow(t, m, nextMenuAdd)].live() {
		t.Error("a done copy blocks Add, want it live again")
	}
}

// TestNextMenuSchedules: ◷ Schedule… adds the item and opens the list's
// scheduler on it; with an open copy already there it schedules that copy
// instead of making another. Without a socket it is dim, in beginSchedule's
// words.
func TestNextMenuSchedules(t *testing.T) {
	m, _ := nextModel(t, sampleNextList)
	m = openNext(t, m)
	m = rightClickNextAt(t, m, nextN002Y)
	sched := m.nextMenu.items[nextMenuRow(t, m, nextMenuSchedule)]
	if sched.live() || !strings.Contains(sched.why, "can't schedule a drop") {
		t.Errorf("no socket: Schedule = %+v, want it dim", sched)
	}

	m.nextMenu = nextMenu{}
	m.client = &catsClient{socket: "/nonexistent/cats.sock"}
	m = rightClickNextAt(t, m, nextN002Y)
	next, _ := m.pressNextMenu(nextMenuRow(t, m, nextMenuSchedule))
	got := next.(model)
	if got.stage != stageSchedule || len(got.project.todos) != 1 {
		t.Fatalf("stage=%v todos=%d, want the scheduler over one new prompt", got.stage, len(got.project.todos))
	}
	if ref, _ := got.selectedRef(); ref.id != got.project.todos[0].ID {
		t.Errorf("scheduler is on %q, want the prompt just added", ref.id)
	}

	// Back on the page, Schedule again reuses that copy.
	m.project = got.project
	m.rebuildList()
	m = rightClickNextAt(t, m, nextN002Y)
	next, _ = m.pressNextMenu(nextMenuRow(t, m, nextMenuSchedule))
	if got := next.(model); got.stage != stageSchedule || len(got.project.todos) != 1 {
		t.Errorf("second Schedule: stage=%v todos=%d, want the same prompt scheduled, not a second copy", got.stage, len(got.project.todos))
	}
}
