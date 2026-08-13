package main

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// withTodos returns a manager whose project backlog holds one prompt per title,
// backed by a real file so a drag's saves have somewhere to go, with the list
// rebuilt so row n is drawn on line listRowsRow+n. The global backlog is left
// empty on purpose: with only one scope holding rows there are no group
// headings, so a row's index and its screen line stay the same number.
func withTodos(t *testing.T, titles ...string) model {
	t.Helper()
	m, project, _ := newModelInTemp(t)
	for i, title := range titles {
		if err := project.add(Todo{ID: string(rune('a' + i)), Title: title, Prompt: title}); err != nil {
			t.Fatal(err)
		}
	}
	m.width = 200
	m.rebuildList()
	return m
}

// projectOrder is the project backlog's titles in array order — the order the
// list draws, and the order a drag rewrites.
func projectOrder(m model) []string {
	got := make([]string, len(m.project.todos))
	for i, td := range m.project.todos {
		got[i] = td.Title
	}
	return got
}

// press, dragTo and release drive one gesture the way a terminal reports it:
// a click on a row, motion while the button is down, then the release.
func press(t *testing.T, m model, row int) model {
	t.Helper()
	next, _ := m.Update(tea.MouseClickMsg{X: 8, Y: listRowsRow + row, Button: tea.MouseLeft})
	return next.(model)
}

func dragTo(t *testing.T, m model, row int) model {
	t.Helper()
	next, _ := m.Update(tea.MouseMotionMsg{X: 8, Y: listRowsRow + row, Button: tea.MouseLeft})
	return next.(model)
}

func release(t *testing.T, m model, row int) model {
	t.Helper()
	next, _ := m.Update(tea.MouseReleaseMsg{X: 8, Y: listRowsRow + row, Button: tea.MouseLeft})
	return next.(model)
}

// TestDragReordersTheBacklog is the gesture end to end: press on a row, move the
// pointer to another one, let go — and the backlog is rewritten to match what
// the eye saw, on disk as well as in memory.
func TestDragReordersTheBacklog(t *testing.T) {
	t.Run("dragged down to the last row", func(t *testing.T) {
		m := withTodos(t, "one", "two", "three")
		m = press(t, m, 0)
		m = dragTo(t, m, 2)
		m = release(t, m, 2)

		if got := projectOrder(m); !slices.Equal(got, []string{"two", "three", "one"}) {
			t.Fatalf("order = %v, want the dragged prompt last", got)
		}
		if got := m.status; got != "moved" {
			t.Errorf("status = %q, want the move confirmed", got)
		}
	})

	t.Run("dragged up, one row at a time", func(t *testing.T) {
		m := withTodos(t, "one", "two", "three")
		m = press(t, m, 2)
		m = dragTo(t, m, 1) // the terminal reports motion per cell, not per gesture
		m = dragTo(t, m, 0)
		m = release(t, m, 0)

		if got := projectOrder(m); !slices.Equal(got, []string{"three", "one", "two"}) {
			t.Fatalf("order = %v, want the dragged prompt first", got)
		}
	})

	t.Run("the highlight rides with the dragged row", func(t *testing.T) {
		m := withTodos(t, "one", "two", "three")
		m = press(t, m, 0)
		m = dragTo(t, m, 2)
		if ref, ok := m.selectedRef(); !ok || ref.id != "a" {
			t.Fatalf("selected = %+v (ok=%v), want the dragged todo still highlighted", ref, ok)
		}
		if m.list.cursor != 2 {
			t.Errorf("cursor row = %d, want the row under the pointer", m.list.cursor)
		}
	})

	t.Run("it persists", func(t *testing.T) {
		m := withTodos(t, "one", "two", "three")
		m = release(t, dragTo(t, press(t, m, 0), 2), 2)

		reloaded := &store{scope: scopeProject, path: m.project.path}
		if err := reloaded.load(); err != nil {
			t.Fatal(err)
		}
		if len(reloaded.todos) != 3 || reloaded.todos[2].Title != "one" {
			t.Errorf("disk order = %+v, want the dragged order written through", reloaded.todos)
		}
	})
}

// TestDragHoldsAndLetsGo pins the state a gesture leaves behind: the grip shows
// only while the button is down, and a drag is not half of a double-click — a
// press, a move and a release must not open the edit form on the next click.
func TestDragHoldsAndLetsGo(t *testing.T) {
	m := withTodos(t, "one", "two", "three")

	m = press(t, m, 0)
	if !m.dragging || m.drag.id != "a" {
		t.Fatalf("dragging=%v drag=%+v, want the pressed row held", m.dragging, m.drag)
	}
	if !m.list.grab {
		t.Error("the held row must draw the grip, or the gesture is invisible until it acts")
	}
	if !strings.Contains(m.list.rowsView("", m.width), grabGlyph) {
		t.Error("the rendered list shows no grip on the held row")
	}

	m = dragTo(t, m, 1)
	m = release(t, m, 1)
	if m.dragging || m.list.grab {
		t.Fatalf("dragging=%v grab=%v after release, want the row let go", m.dragging, m.list.grab)
	}
	if strings.Contains(m.list.rowsView("", m.width), grabGlyph) {
		t.Error("the grip outlived the drag")
	}

	// The press that started the drag must not pair with the next click on that
	// row: the gesture already spent it.
	m = press(t, m, 1)
	if m.stage != stageList {
		t.Fatalf("stage = %v, want a drag then a click to stay on the list", m.stage)
	}
}

// TestDragLetsGoWhenTheReleaseNeverComes covers the hold outliving its gesture:
// a button let go outside the pane, or a terminal that reports presses but not
// releases. Both of the other ways the manager hears from the user — a keystroke
// and the next press — must clear the hold, or the grip stays drawn on a row
// nobody is touching and the next sweep of the pointer reorders the backlog.
func TestDragLetsGoWhenTheReleaseNeverComes(t *testing.T) {
	t.Run("a keystroke", func(t *testing.T) {
		m := press(t, withTodos(t, "one", "two"), 0)
		m = pressList(t, m, "down")
		if m.dragging || m.list.grab {
			t.Errorf("dragging=%v grab=%v after a keystroke, want the hold dropped", m.dragging, m.list.grab)
		}
	})

	t.Run("the next press", func(t *testing.T) {
		m := press(t, withTodos(t, "one", "two"), 0)
		next, _ := m.Update(tea.MouseClickMsg{X: m.actionChips()[actionAdd].start + 1, Y: actionBarRow, Button: tea.MouseLeft})
		if got := next.(model); got.dragging || got.list.grab {
			t.Errorf("dragging=%v grab=%v after a press elsewhere, want the hold dropped", got.dragging, got.list.grab)
		}
	})
}

// TestDragRefusesWhatItCannotExpress covers the drops the list has no honest
// answer for: another backlog, another render group, and a filtered list — whose
// rows are in match order, so "put it here" names no slot in the file.
func TestDragRefusesWhatItCannotExpress(t *testing.T) {
	t.Run("across backlogs", func(t *testing.T) {
		m := withTodos(t, "one", "two")
		if err := m.global.add(Todo{ID: "g1", Title: "global one", Prompt: "global one"}); err != nil {
			t.Fatal(err)
		}
		m.rebuildList()
		// Both scopes hold rows now, so the list is grouped: a heading and its
		// leading blank open each group. Find the global row by its ref rather
		// than by counting lines.
		globalRow := -1
		for i := range m.list.filtered {
			if idx, ok := m.list.refAt(i); ok && m.rows[idx].scope == scopeGlobal {
				globalRow = i
			}
		}
		if globalRow < 0 {
			t.Fatal("no global row in the rebuilt list")
		}

		m = press(t, m, 0) // the first project row — line and index agree at the top
		before := projectOrder(m)
		next, _ := m.dragOver(tea.MouseMotionMsg{X: 8, Y: listRowsRow + lineOfRow(m, globalRow), Button: tea.MouseLeft})
		m = next.(model)
		if got := projectOrder(m); !slices.Equal(got, before) {
			t.Errorf("project order = %v, want %v — a drag reorders one backlog, it does not move between them", got, before)
		}
		if len(m.global.todos) != 1 {
			t.Errorf("global backlog = %+v, want it untouched", m.global.todos)
		}
	})

	t.Run("onto a done prompt", func(t *testing.T) {
		m := withTodos(t, "one", "two")
		if err := m.project.setDone("b", true); err != nil {
			t.Fatal(err)
		}
		m.rebuildList()
		before := projectOrder(m)

		m = release(t, dragTo(t, press(t, m, 0), 1), 1)
		if got := projectOrder(m); !slices.Equal(got, before) {
			t.Errorf("order = %v, want %v — open and done are drawn in separate passes", got, before)
		}
		if m.status == "moved" {
			t.Error("a refused drag must not report a move")
		}
	})

	t.Run("while a filter is on", func(t *testing.T) {
		m := withTodos(t, "one", "two", "three")
		m.list.input.SetValue("e") // matches every prompt, in score order
		m.list.filter()
		before := projectOrder(m)

		m = press(t, m, 0)
		if m.list.grab {
			t.Error("a filtered list must not draw a grip it will refuse to honor")
		}
		m = dragTo(t, m, 1)
		if got := projectOrder(m); !slices.Equal(got, before) {
			t.Fatalf("order = %v, want %v — a filtered list is not in backlog order", got, before)
		}
		if !m.statusErr || !strings.Contains(m.status, "filter") {
			t.Errorf("status = %q (err=%v), want the refusal said in words", m.status, m.statusErr)
		}
	})
}

// lineOfRow is the screen line, counted from listRowsRow, that filtered row i is
// drawn on — the inverse of rowAtLine, walked the same way so a test can point
// at a row in a grouped list without hard-coding where its headings fall.
func lineOfRow(m model, want int) int {
	line := 0
	for i, s := range m.list.filtered {
		if !s.item.selectable {
			line++
			if s.item.name != "" {
				line++
			}
			continue
		}
		if i == want {
			return line
		}
		line++
	}
	return -1
}

// TestDragOnlyFollowsItsOwnPress guards the routing: motion and release messages
// that no press on a row armed must fall through untouched, so the form's own
// pointer handling (and any input that reads them) still gets them.
func TestDragOnlyFollowsItsOwnPress(t *testing.T) {
	m := withTodos(t, "one", "two")
	before := projectOrder(m)

	next, _ := m.Update(tea.MouseMotionMsg{X: 8, Y: listRowsRow + 1, Button: tea.MouseLeft})
	m = next.(model)
	if m.dragging {
		t.Error("motion with no press behind it started a drag")
	}
	if got := projectOrder(m); !slices.Equal(got, before) {
		t.Errorf("order = %v, want %v — motion alone must not reorder", got, before)
	}

	// A press on the action bar is not a press on a row: it must not arm a drag
	// that the next sweep of the pointer would act on.
	m = withTodos(t, "one", "two")
	next, _ = m.Update(tea.MouseClickMsg{X: m.actionChips()[actionAdd].start + 1, Y: actionBarRow, Button: tea.MouseLeft})
	if got := next.(model); got.dragging {
		t.Error("a click on the action bar armed a drag")
	}
}

// TestDragSurvivesTheScreenChangingUnderIt covers the button being held while a
// key opens another stage: the motion that follows must let go rather than
// reorder a list nobody is looking at.
func TestDragSurvivesTheScreenChangingUnderIt(t *testing.T) {
	m := withTodos(t, "one", "two", "three")
	m = press(t, m, 0)
	before := projectOrder(m)

	next, _ := m.beginAdd() // ctrl+a with the button still down
	m = next.(model)
	if m.stage != stageForm {
		t.Fatalf("stage = %v, want the form", m.stage)
	}
	m = dragTo(t, m, 2)
	if m.dragging {
		t.Error("the drag outlived the list it was dragging in")
	}
	if got := projectOrder(m); !slices.Equal(got, before) {
		t.Errorf("order = %v, want %v — a form on screen must not be reordering the list", got, before)
	}
}
