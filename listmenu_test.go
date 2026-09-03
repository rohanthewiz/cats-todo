package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// rightClickRow opens the list's context menu with a right-click on row n,
// driving the whole path a terminal does — Update, not the handler — so the
// routing in updateMouse is under test too.
func rightClickRow(t *testing.T, m model, row int) model {
	t.Helper()
	next, _ := m.Update(tea.MouseClickMsg{X: 8, Y: listRowsRow + row, Button: tea.MouseRight})
	got := next.(model)
	if !got.listMenu.open {
		t.Fatalf("a right-click on row %d did not open the menu", row)
	}
	return got
}

// markTodo persists a todo's state the way the manager does.
//
// It goes through the store's setters rather than poking the in-memory slice
// because every write reloads from disk first (see store.reload): a field set
// only in memory is a field the next save silently drops, so a test that set one
// that way would be building its menu over a backlog no real launch could have.
// done and frozen are mutually exclusive — the store enforces it — so a caller
// asks for at most one.
func markTodo(t *testing.T, s *store, id string, done, frozen bool, a annots) {
	t.Helper()
	if err := s.setAnnots(id, a); err != nil {
		t.Fatal(err)
	}
	if done {
		if err := s.setDone(id, true); err != nil {
			t.Fatal(err)
		}
	}
	if frozen {
		if err := s.setFrozen(id, true); err != nil {
			t.Fatal(err)
		}
	}
}

// TestListMenuOpensOnTheRightButton: the right button over a row is the whole
// gesture — the menu comes up naming every action in a fixed order, and the
// press also moves the highlight, so what the menu acts on is what the keyboard
// is parked on when it hands control back.
func TestListMenuOpensOnTheRightButton(t *testing.T) {
	m := withTodos(t, "first", "second", "third")
	m = rightClickRow(t, m, 1)

	if got, want := len(m.listMenu.items), listMenuActionCount; got != want {
		t.Fatalf("menu has %d rows, want %d", got, want)
	}
	for i, it := range m.listMenu.items {
		if it.act != i {
			t.Errorf("row %d holds action %d — the order is what the menu is learned by", i, it.act)
		}
	}
	if got := m.listMenu.ref.id; got != "b" {
		t.Errorf("menu opened on todo %q, want the one under the pointer", got)
	}
	ref, ok := m.selectedRef()
	if !ok || ref.id != "b" {
		t.Errorf("highlight = %+v, want it moved onto the clicked row", ref)
	}
	// The cursor opens on the first row that can act, so enter is never a
	// refusal straight after the click.
	if m.listMenu.cursor != listMenuEdit {
		t.Errorf("cursor on row %d, want the edit", m.listMenu.cursor)
	}
	// And a right-click is not half of a double-click: pairing it with the next
	// left press would open a form nobody pointed at.
	if !m.lastClickAt.IsZero() {
		t.Error("a right-click armed the double-click timer")
	}
	if m.dragging {
		t.Error("a right-click took hold of the row for a drag")
	}
}

// TestListMenuOnlyOverARow: a context menu is a question about a thing, so the
// header line, the action bar and the empty space below the rows have none to
// open — and the right button aimed at any of them is also the way out of a menu
// that is up.
func TestListMenuOnlyOverARow(t *testing.T) {
	m := withTodos(t, "first")
	m.height = 40

	for _, tc := range []struct {
		name string
		y    int
	}{
		{"the header line", headerRow},
		{"the action bar", actionBarRow},
		{"below the last row", listRowsRow + 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next, _ := m.Update(tea.MouseClickMsg{X: 8, Y: tc.y, Button: tea.MouseRight})
			if next.(model).listMenu.open {
				t.Fatalf("a right-click on %s opened a menu", tc.name)
			}
		})
	}

	t.Run("and closes an open one", func(t *testing.T) {
		open := rightClickRow(t, m, 0)
		next, _ := open.Update(tea.MouseClickMsg{X: 8, Y: headerRow, Button: tea.MouseRight})
		if next.(model).listMenu.open {
			t.Fatal("a right-click off the rows left the menu up")
		}
	})
}

// TestListMenuLabelsFollowTheState pins the two rows that name a state rather
// than an action: they have to say what the press will do, or the menu is
// lying about the prompt it was opened on.
func TestListMenuLabelsFollowTheState(t *testing.T) {
	for _, tc := range []struct {
		name         string
		done, frozen bool
		wantDone     string
		wantFreeze   string
	}{
		{name: "open", wantDone: "✓ Mark done", wantFreeze: "❄ Freeze"},
		{name: "done", done: true, wantDone: "↺ Reopen", wantFreeze: "❄ Freeze"},
		{name: "frozen", frozen: true, wantDone: "✓ Mark done", wantFreeze: "☀ Unfreeze"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := withTodos(t, "the prompt")
			markTodo(t, m.project, "a", tc.done, tc.frozen, annots{})
			m.rebuildList()
			m = rightClickRow(t, m, 0)

			if got := m.listMenu.items[listMenuDone].label; got != tc.wantDone {
				t.Errorf("done row = %q, want %q", got, tc.wantDone)
			}
			if got := m.listMenu.items[listMenuFreeze].label; got != tc.wantFreeze {
				t.Errorf("freeze row = %q, want %q", got, tc.wantFreeze)
			}
		})
	}
}

// TestListMenuDimsWhatCannotAct: a row that cannot act is greyed and says why
// when it is pressed, in the words the chord uses — never left off the menu, and
// never silently doing nothing.
func TestListMenuDimsWhatCannotAct(t *testing.T) {
	// No control socket: nothing can be dropped and nothing can be scheduled,
	// but everything that is local to the backlog still can be.
	t.Run("no cats socket", func(t *testing.T) {
		m := withTodos(t, "the prompt")
		m = rightClickRow(t, m, 0)

		for _, row := range []int{listMenuSend, listMenuSchedule} {
			if m.listMenu.items[row].live() {
				t.Errorf("row %d is live with no control socket", row)
			}
			if !strings.Contains(m.listMenu.items[row].why, "control socket") {
				t.Errorf("row %d says %q, want the socket named", row, m.listMenu.items[row].why)
			}
		}
		for _, row := range []int{
			listMenuEdit, listMenuView, listMenuDone, listMenuFreeze,
			listMenuFruit, listMenuPrioNone, listMenuPrioHigh, listMenuPrioCritical,
			listMenuFlag, listMenuExport, listMenuDelete,
		} {
			if !m.listMenu.items[row].live() {
				t.Errorf("row %d is dim, but it never needed the socket", row)
			}
		}
		// The cursor still opens somewhere it can act.
		if !m.listMenu.items[m.listMenu.cursor].live() {
			t.Error("the menu opened with the cursor on a dim row")
		}
	})

	// Frozen is the decision not to do the work, and both roads out to an agent
	// hold it — the same refusal startDrop and beginSchedule make to the chords.
	t.Run("frozen refuses send and schedule", func(t *testing.T) {
		m := withTodos(t, "the prompt")
		m.client = &catsClient{} // enough to clear the socket guard
		markTodo(t, m.project, "a", false, true, annots{})
		m.rebuildList()
		m = rightClickRow(t, m, 0)

		for _, row := range []int{listMenuSend, listMenuSchedule} {
			if m.listMenu.items[row].live() {
				t.Errorf("row %d is live on a frozen prompt", row)
			}
			if !strings.Contains(m.listMenu.items[row].why, "unfreeze") {
				t.Errorf("row %d says %q, want it to name the way out", row, m.listMenu.items[row].why)
			}
		}
	})

	// Done is where the two part company: sending a finished prompt reopens it
	// by handing it to an agent, but a timer on closed work is a promise about
	// something that is over.
	t.Run("done refuses only the schedule", func(t *testing.T) {
		m := withTodos(t, "the prompt")
		m.client = &catsClient{}
		markTodo(t, m.project, "a", true, false, annots{})
		m.rebuildList()
		m = rightClickRow(t, m, 0)

		if !m.listMenu.items[listMenuSend].live() {
			t.Error("send is dim on a done prompt — a drop is how one is picked back up")
		}
		if m.listMenu.items[listMenuSchedule].live() {
			t.Error("schedule is live on a done prompt")
		}
	})

	// And pressing a dim row says why on the status line rather than doing
	// nothing, which on a menu row reads as a dead control.
	t.Run("pressing a dim row says why", func(t *testing.T) {
		m := withTodos(t, "the prompt")
		m = rightClickRow(t, m, 0)
		next, _ := m.pressListMenu(listMenuSend)
		got := next.(model)

		if got.listMenu.open {
			t.Error("the menu stayed up over its own answer")
		}
		if !strings.Contains(got.status, "control socket") {
			t.Errorf("status = %q, want the refusal", got.status)
		}
		if got.stage != stageList {
			t.Errorf("stage = %v, want to still be on the list", got.stage)
		}
	})
}

// TestListMenuRunsTheRow: a live row hands off to the same helper its chord
// does, and the menu closes on the way.
func TestListMenuRunsTheRow(t *testing.T) {
	t.Run("edit opens the form on the clicked prompt", func(t *testing.T) {
		m := withTodos(t, "first", "second")
		m = rightClickRow(t, m, 1)
		next, _ := m.pressListMenu(listMenuEdit)
		got := next.(model)

		if got.stage != stageForm {
			t.Fatalf("stage = %v, want the edit form", got.stage)
		}
		if got.editID != "b" {
			t.Errorf("editing %q, want the row the menu was opened on", got.editID)
		}
		if got.listMenu.open {
			t.Error("the menu outlived the action it ran")
		}
	})

	t.Run("done marks the clicked prompt done", func(t *testing.T) {
		m := withTodos(t, "first", "second")
		m = rightClickRow(t, m, 1)
		next, _ := m.pressListMenu(listMenuDone)
		got := next.(model)

		td, ok := got.project.find("b")
		if !ok || !td.Done {
			t.Fatalf("todo b = %+v, want it done", td)
		}
		if first, _ := got.project.find("a"); first.Done {
			t.Error("the menu acted on the highlighted row rather than the clicked one")
		}
	})

	// The reversible half of the done pair, and the reason the row flips its
	// label: a done pressed by accident is undone from the same menu, on the row
	// that now offers exactly that.
	t.Run("reopen clears the done flag", func(t *testing.T) {
		m := withTodos(t, "first", "second")
		markTodo(t, m.project, "b", true, false, annots{})
		m.rebuildList()
		// The done row renders below the open one, whatever the array order.
		row := 0
		if ref, _ := m.list.refAt(0); m.rows[ref].id != "b" {
			row = 1
		}
		m = rightClickRow(t, m, row)
		if got := m.listMenu.items[listMenuDone].label; got != "↺ Reopen" {
			t.Fatalf("done row = %q on a finished prompt, want the reopen", got)
		}

		next, _ := m.pressListMenu(listMenuDone)
		got := next.(model)
		if td, _ := got.project.find("b"); td.Done {
			t.Fatal("the reopen row left the prompt done")
		}
		if !strings.Contains(got.status, "reopened") {
			t.Errorf("status = %q, want the reopen named", got.status)
		}
		// And the highlight rides back out of the done group with it.
		if ref, _ := got.selectedRef(); ref.id != "b" {
			t.Errorf("highlight = %q, want it still on the reopened prompt", ref.id)
		}
	})

	t.Run("delete arms the confirm stage", func(t *testing.T) {
		m := withTodos(t, "first")
		m = rightClickRow(t, m, 0)
		next, _ := m.pressListMenu(listMenuDelete)
		got := next.(model)

		if got.stage != stageConfirm || got.pendingDelete.id != "a" {
			t.Fatalf("stage = %v, pending = %+v, want the delete confirm", got.stage, got.pendingDelete)
		}
	})
}

// TestListMenuOwnsTheKeys: while the box is up it takes every keystroke — the
// list's own chords must not act on a row from behind it — and anything that is
// not a move or a press is spent taking it down.
func TestListMenuOwnsTheKeys(t *testing.T) {
	m := withTodos(t, "first", "second")
	m = rightClickRow(t, m, 0)

	m = pressList(t, m, "down")
	if m.listMenu.cursor != listMenuView {
		t.Fatalf("cursor on row %d after ↓, want the view", m.listMenu.cursor)
	}
	m = pressList(t, m, "up")
	if m.listMenu.cursor != listMenuEdit {
		t.Fatalf("cursor on row %d after ↑, want back on the edit", m.listMenu.cursor)
	}
	m = pressList(t, m, "end")
	if m.listMenu.cursor != listMenuActionCount-1 {
		t.Fatalf("cursor on row %d after end, want the last row", m.listMenu.cursor)
	}

	// ctrl+x is the list's delete. Behind an open menu it may only close the box.
	closed := pressList(t, m, "ctrl+x")
	if closed.listMenu.open {
		t.Fatal("a stray key left the menu up")
	}
	if closed.stage != stageList {
		t.Fatalf("stage = %v — a key the menu swallowed reached the list", closed.stage)
	}

	// And a character does not fall through into the filter either.
	typed := pressList(t, m, "z")
	if typed.listMenu.open {
		t.Fatal("typing left the menu up")
	}
	if got := typed.list.input.Value(); got != "" {
		t.Errorf("filter = %q — the key the menu swallowed was also typed", got)
	}
}

// TestListMenuClickDismissesWithoutActing: a click off the box takes the menu
// down and does nothing else. Acting on the way out would make "never mind" the
// riskiest gesture on the screen.
func TestListMenuClickDismissesWithoutActing(t *testing.T) {
	m := withTodos(t, "first", "second")
	m.height = 40
	m = rightClickRow(t, m, 0)

	// A press on the box's border is inside the menu: it dismisses nothing and
	// presses nothing.
	next, _ := m.Update(tea.MouseClickMsg{X: m.listMenu.x, Y: m.listMenu.y, Button: tea.MouseLeft})
	if !next.(model).listMenu.open {
		t.Fatal("a click on the border dismissed the menu")
	}

	// A press on a row runs it.
	row := m.listMenu.y + 1 + listMenuView
	next, _ = m.Update(tea.MouseClickMsg{X: m.listMenu.x + 2, Y: row, Button: tea.MouseLeft})
	if got := next.(model); got.stage != stageView {
		t.Fatalf("stage = %v after clicking the view row, want the prompt view", got.stage)
	}

	// A press well off the box only closes it — the row it landed on is not also
	// selected, and no form opens.
	m = rightClickRow(t, m, 0)
	next, _ = m.Update(tea.MouseClickMsg{X: 4, Y: listRowsRow + 1, Button: tea.MouseLeft})
	got := next.(model)
	if got.listMenu.open {
		t.Fatal("a click off the box left the menu up")
	}
	if got.stage != stageList {
		t.Fatalf("stage = %v — the dismissing click also acted", got.stage)
	}
	if ref, _ := got.selectedRef(); ref.id != "a" {
		t.Errorf("highlight moved to %q — the dismissing click also selected", ref.id)
	}
}

// TestListMenuIsDrawnOverTheList: the box has to be in the rendered frame, and
// the row it was opened on has to still be readable under it — a context menu
// that hid its own context would be asking about a prompt the user can no longer
// see. The box drops below the pointer for exactly that reason (menuBox.place),
// so the rows it does cover are the ones the question is not about.
func TestListMenuIsDrawnOverTheList(t *testing.T) {
	m := withTodos(t, "the first prompt", "the second prompt")
	m.height = 40
	m.applySizes()
	m = rightClickRow(t, m, 0)

	frame := m.renderStage()
	if !strings.Contains(frame, "✎ Edit") {
		t.Error("the menu is not in the rendered frame")
	}
	if !strings.Contains(frame, "the first prompt") {
		t.Error("the menu covered the row it was opened on")
	}
	if !strings.Contains(frame, "✚ Add") {
		t.Error("the menu replaced the list instead of floating over it")
	}
}

// TestListMenuDiesWithItsStage: a resize aims the box at cells that may no
// longer exist, and leaving the list some other way leaves it composited over a
// screen nobody is looking at. Both take it down.
func TestListMenuDiesWithItsStage(t *testing.T) {
	t.Run("a resize", func(t *testing.T) {
		m := withTodos(t, "first")
		m.height = 40
		m = rightClickRow(t, m, 0)
		next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		if next.(model).listMenu.open {
			t.Fatal("the menu survived a resize aimed at cells it was placed against")
		}
	})

	t.Run("backToList", func(t *testing.T) {
		m := withTodos(t, "first")
		m = rightClickRow(t, m, 0)
		m.backToList()
		if m.listMenu.open {
			t.Fatal("the menu survived the trip back to the list")
		}
	})
}

// TestListMenuAnnotationRowsDrawTheirState: the five annotation rows are the
// list's only road to a prompt's marks, so they have to say what is already true
// as well as what pressing them would do — a checkbox and one filled radio out
// of three, in the annotation bar's own glyphs.
func TestListMenuAnnotationRowsDrawTheirState(t *testing.T) {
	for _, tc := range []struct {
		name     string
		priority string
		fruit    bool
		wantBox  string
		filled   int // the row whose radio is filled, or -1 for none
	}{
		{name: "unmarked", priority: priorityNone, wantBox: "☐", filled: listMenuPrioNone},
		{name: "a quick win", priority: priorityNone, fruit: true, wantBox: "☑", filled: listMenuPrioNone},
		{name: "high", priority: priorityHigh, wantBox: "☐", filled: listMenuPrioHigh},
		{name: "critical", priority: priorityCritical, wantBox: "☐", filled: listMenuPrioCritical},
		// A hand-edited backlog can hold the retired "low". It fills no hole —
		// the same exact match the annotation bar makes — which leaves all three
		// levels offering to replace a value this program cannot read.
		{name: "a level we do not know", priority: "low", wantBox: "☐", filled: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := withTodos(t, "the prompt")
			markTodo(t, m.project, "a", false, false, annots{Priority: tc.priority, Fruit: tc.fruit})
			m.rebuildList()
			m = rightClickRow(t, m, 0)

			if got := m.listMenu.items[listMenuFruit].label; !strings.HasPrefix(got, tc.wantBox) {
				t.Errorf("quick-win row = %q, want it to start %q", got, tc.wantBox)
			}
			for _, row := range []int{listMenuPrioNone, listMenuPrioHigh, listMenuPrioCritical} {
				label := m.listMenu.items[row].label
				want := "( )"
				if row == tc.filled {
					want = "(•)"
				}
				if !strings.HasPrefix(label, want) {
					t.Errorf("priority row %d = %q, want it to start %q", row, label, want)
				}
			}
		})
	}
}

// TestListMenuSetsTheAnnotations: pressing a mark writes it straight to the
// backlog — no form, no save step — and says on the status line what it did.
func TestListMenuSetsTheAnnotations(t *testing.T) {
	t.Run("a priority row sets exactly its level", func(t *testing.T) {
		m := withTodos(t, "first", "second")
		m = rightClickRow(t, m, 1)
		next, _ := m.pressListMenu(listMenuPrioCritical)
		got := next.(model)

		td, _ := got.project.find("b")
		if td.Priority != priorityCritical {
			t.Fatalf("priority = %q, want critical", td.Priority)
		}
		if other, _ := got.project.find("a"); other.Priority != priorityNone {
			t.Errorf("todo a = %q — the menu marked a prompt it was not opened on", other.Priority)
		}
		if !strings.Contains(got.status, "critical") {
			t.Errorf("status = %q, want the new level named", got.status)
		}
		if got.listMenu.open {
			t.Error("the menu outlived the mark it set")
		}
		// And the highlight rides with the row, which the priority lens can move.
		if ref, _ := got.selectedRef(); ref.id != "b" {
			t.Errorf("highlight = %q, want it re-parked on the marked prompt", ref.id)
		}
	})

	t.Run("the none row clears a raised level", func(t *testing.T) {
		m := withTodos(t, "the prompt")
		markTodo(t, m.project, "a", false, false, annots{Priority: priorityCritical})
		m.rebuildList()
		m = rightClickRow(t, m, 0)
		next, _ := m.pressListMenu(listMenuPrioNone)

		if td, _ := next.(model).project.find("a"); td.Priority != priorityNone {
			t.Fatalf("priority = %q, want it cleared", td.Priority)
		}
	})

	// The radios are radios: pressing the level a prompt already holds leaves it
	// there rather than toggling it off, and still answers on the status line —
	// silence would read as a dead control.
	t.Run("pressing the standing level is a no-op that still answers", func(t *testing.T) {
		m := withTodos(t, "the prompt")
		markTodo(t, m.project, "a", false, false, annots{Priority: priorityHigh})
		m.rebuildList()
		m = rightClickRow(t, m, 0)
		next, _ := m.pressListMenu(listMenuPrioHigh)
		got := next.(model)

		if td, _ := got.project.find("a"); td.Priority != priorityHigh {
			t.Fatalf("priority = %q, want it left at high", td.Priority)
		}
		if !strings.Contains(got.status, "high") {
			t.Errorf("status = %q, want the level named", got.status)
		}
	})

	t.Run("the quick win row flips both ways", func(t *testing.T) {
		m := withTodos(t, "the prompt")
		m = rightClickRow(t, m, 0)
		next, _ := m.pressListMenu(listMenuFruit)
		on := next.(model)

		if td, _ := on.project.find("a"); !td.Fruit {
			t.Fatal("the quick-win row did not mark the prompt")
		}
		if !strings.Contains(on.status, "quick win") {
			t.Errorf("status = %q, want the mark named", on.status)
		}

		next, _ = rightClickRow(t, on, 0).pressListMenu(listMenuFruit)
		if td, _ := next.(model).project.find("a"); td.Fruit {
			t.Fatal("a second press did not clear the mark")
		}
	})

	// The two marks are edited and saved as one set, so setting one must not
	// carry a stale copy of the other back over it.
	t.Run("one mark does not clobber the other", func(t *testing.T) {
		m := withTodos(t, "the prompt")
		markTodo(t, m.project, "a", false, false, annots{Fruit: true})
		m.rebuildList()
		m = rightClickRow(t, m, 0)
		next, _ := m.pressListMenu(listMenuPrioHigh)

		td, _ := next.(model).project.find("a")
		if !td.Fruit || td.Priority != priorityHigh {
			t.Fatalf("todo = %+v, want both marks standing", td)
		}
	})
}

// TestListMenuFlagRow: the flag is a checkbox on this menu like the fruit — it
// flips the mark and nothing else, because a menu row is a press and a note is
// words. The row wears whatever note the prompt already carries, so the decision
// to open the form and write one is made with the current note in sight.
func TestListMenuFlagRow(t *testing.T) {
	t.Run("the row flips the mark both ways", func(t *testing.T) {
		m := withTodos(t, "the prompt")
		m = rightClickRow(t, m, 0)
		next, _ := m.pressListMenu(listMenuFlag)
		on := next.(model)

		if td, _ := on.project.find("a"); !td.Flag {
			t.Fatal("the flag row did not mark the prompt")
		}
		// The row can only raise the mark, so the status has to say where the
		// words are written — otherwise "with note" is a promise nothing keeps.
		if !strings.Contains(on.status, "note") {
			t.Errorf("status = %q, want the note's road named", on.status)
		}

		next, _ = rightClickRow(t, on, 0).pressListMenu(listMenuFlag)
		if td, _ := next.(model).project.find("a"); td.Flag {
			t.Fatal("a second press did not clear the flag")
		}
	})

	t.Run("the row draws the state and the note", func(t *testing.T) {
		m := withTodos(t, "the prompt")
		if got := rightClickRow(t, m, 0).listMenu.items[listMenuFlag].label; !strings.HasPrefix(got, "☐") {
			t.Errorf("flag row = %q, want an empty box on an unflagged prompt", got)
		}

		markTodo(t, m.project, "a", false, false, annots{Flag: true, FlagNote: "blocked on the api"})
		m.rebuildList()
		got := rightClickRow(t, m, 0).listMenu.items[listMenuFlag].label
		if !strings.HasPrefix(got, "☑") || !strings.Contains(got, "blocked on the api") {
			t.Errorf("flag row = %q, want a ticked box carrying the note", got)
		}
	})

	// Clearing the flag from here takes its words with it: applyTo drops the
	// note with the mark, so the file never holds words no screen draws.
	t.Run("clearing the flag drops its note", func(t *testing.T) {
		m := withTodos(t, "the prompt")
		markTodo(t, m.project, "a", false, false, annots{Flag: true, FlagNote: "why"})
		m.rebuildList()
		m = rightClickRow(t, m, 0)
		next, _ := m.pressListMenu(listMenuFlag)

		td, _ := next.(model).project.find("a")
		if td.Flag || td.FlagNote != "" {
			t.Fatalf("todo = %+v, want the flag and its note both gone", td)
		}
	})

	// The set is written whole (store.setAnnots), so the flag must not clobber
	// the marks beside it — the same guard the fruit and priority rows carry.
	t.Run("the flag does not clobber the other marks", func(t *testing.T) {
		m := withTodos(t, "the prompt")
		markTodo(t, m.project, "a", false, false, annots{Priority: priorityCritical, Fruit: true})
		m.rebuildList()
		m = rightClickRow(t, m, 0)
		next, _ := m.pressListMenu(listMenuFlag)

		td, _ := next.(model).project.find("a")
		if !td.Flag || !td.Fruit || td.Priority != priorityCritical {
			t.Fatalf("todo = %+v, want all three marks standing", td)
		}
	})
}
