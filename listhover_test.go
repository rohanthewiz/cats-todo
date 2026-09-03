package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// hoverModel is a list of the given todos in a pane big enough to float a card
// in, with the rows rebuilt so row n is drawn on listRowsRow+n. Only the project
// backlog is filled, for the reason withTodos does the same: with one scope
// holding rows there are no group headings, so a row's index and its screen line
// stay the same number.
func hoverModel(todos ...Todo) model {
	m := newTestModel()
	m.width, m.height = 100, 24
	m.project.todos = todos
	m.applySizes()
	m.rebuildList()
	return m
}

// hoverOver is the pointer moving onto a row with nothing held — the message a
// terminal sends only while the stage has asked for MouseModeAllMotion.
func hoverOver(t *testing.T, m model, row int) model {
	t.Helper()
	next, _ := m.Update(tea.MouseMotionMsg{X: 6, Y: listRowsRow + row})
	return next.(model)
}

// cardText is the card as plain text, one line per row, for asserting on what it
// says rather than on how it is styled.
func cardText(m model) string {
	if !m.hover.open {
		return ""
	}
	var b strings.Builder
	for _, ln := range m.hover.lines {
		b.WriteString(strings.TrimSpace(ansi.Strip(ln)) + "\n")
	}
	return b.String()
}

// TestHoverCardShowsTheBody is the feature itself: rest the pointer on a row and
// the prompt the row can only show one line of is readable without leaving the
// list.
func TestHoverCardShowsTheBody(t *testing.T) {
	m := hoverModel(Todo{
		ID: "t1", Title: "Fix the drop timeout",
		Prompt:  "Fix the drop timeout\nThe wait comes from stale ready probes in client.go.",
		Session: &SessionOpts{Model: "claude-opus-5", Effort: "high"},
	})
	m = hoverOver(t, m, 0)

	if !m.hover.open {
		t.Fatal("no card after hovering a row")
	}
	got := cardText(m)
	for _, want := range []string{"Fix the drop timeout", "stale ready probes", "Model", "claude-opus-5", "Effort", "high"} {
		if !strings.Contains(got, want) {
			t.Errorf("card is missing %q:\n%s", want, got)
		}
	}
	// The title is said once, at the top: the prompt's own first line is the
	// title here, and repeating it would spend half the card's height on it.
	if n := strings.Count(got, "Fix the drop timeout"); n != 1 {
		t.Errorf("title appears %d times, want once:\n%s", n, got)
	}
	// And the card is actually on the frame, not merely in the model.
	if frame := ansi.Strip(m.renderStage()); !strings.Contains(frame, "stale ready probes") {
		t.Errorf("the card is not composited onto the list frame:\n%s", frame)
	}
}

// TestHoverCardOnlyOverRows pins that the card belongs to a prompt: chrome has
// nothing to say, so nothing floats over it.
func TestHoverCardOnlyOverRows(t *testing.T) {
	m := hoverModel(Todo{ID: "t1", Title: "one", Prompt: "one\nbody text"})
	m = hoverOver(t, m, 0)
	if !m.hover.open {
		t.Fatal("no card on the row to begin with")
	}
	next, _ := m.Update(tea.MouseMotionMsg{X: 6, Y: headerRow})
	if next.(model).hover.open {
		t.Error("the card survived the pointer moving onto the header")
	}
	next, _ = m.Update(tea.MouseMotionMsg{X: 6, Y: listRowsRow + 5}) // past the last row
	if next.(model).hover.open {
		t.Error("the card survived the pointer moving off the rows")
	}
}

// TestHoverCardStaysPutWithinARow: the card belongs to the row, not to the cell,
// so drifting across the same row must not re-place the box under the pointer —
// a card that crawled sideways under a still hand would be unreadable.
func TestHoverCardStaysPutWithinARow(t *testing.T) {
	m := hoverModel(Todo{ID: "t1", Title: "one", Prompt: "one\nbody text"})
	m = hoverOver(t, m, 0)
	at := m.hover
	next, _ := m.Update(tea.MouseMotionMsg{X: 40, Y: listRowsRow})
	if got := next.(model).hover; got.x != at.x || got.y != at.y {
		t.Errorf("card moved to (%d,%d) from (%d,%d) without changing rows", got.x, got.y, at.x, at.y)
	}
}

// TestHoverCardYieldsToTheHand pins the three things that take the card down:
// the keyboard, a click, and a resize. None of them produces another motion
// message, so none of them would notice the pointer having left.
func TestHoverCardYieldsToTheHand(t *testing.T) {
	base := hoverModel(Todo{ID: "t1", Title: "one", Prompt: "one\nbody text"})

	t.Run("a keystroke", func(t *testing.T) {
		m := hoverOver(t, base, 0)
		if next, _ := m.Update(pressKey("down")); next.(model).hover.open {
			t.Error("the card survived a keystroke on the list")
		}
	})
	t.Run("a click", func(t *testing.T) {
		m := hoverOver(t, base, 0)
		next, _ := m.Update(tea.MouseClickMsg{X: 6, Y: listRowsRow, Button: tea.MouseLeft})
		if next.(model).hover.open {
			t.Error("the card survived a click")
		}
	})
	t.Run("a resize", func(t *testing.T) {
		m := hoverOver(t, base, 0)
		next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		if next.(model).hover.open {
			t.Error("the card survived a resize that moved the rows under it")
		}
	})
}

// TestHoverCardNotWhileAMenuOrDragIsUp: both gestures own the pointer already,
// and a card floating over either would be a second answer to a question nobody
// asked twice.
func TestHoverCardNotWhileAMenuOrDragIsUp(t *testing.T) {
	base := hoverModel(Todo{ID: "t1", Title: "one", Prompt: "one\nbody text"})

	t.Run("context menu", func(t *testing.T) {
		next, _ := base.Update(tea.MouseClickMsg{X: 6, Y: listRowsRow, Button: tea.MouseRight})
		m := next.(model)
		if !m.listMenu.open {
			t.Fatal("the right-click did not open the menu")
		}
		if hoverOver(t, m, 0).hover.open {
			t.Error("a card was built while the context menu was up")
		}
	})
	t.Run("drag", func(t *testing.T) {
		m := base
		m.dragging = true
		if hoverOver(t, m, 0).hover.open {
			t.Error("a card was built mid-drag, which is about where the row is going")
		}
	})
}

// TestHoverCardSaysNothingTwice: a prompt with nothing beyond the line the row
// already draws gets no card at all, rather than a bordered box repeating that
// line back at the pointer.
func TestHoverCardSaysNothingTwice(t *testing.T) {
	m := hoverModel(Todo{ID: "t1", Title: "ship it", Prompt: "ship it"})
	if hoverOver(t, m, 0).hover.open {
		t.Error("a card floated for a prompt with nothing the row was not showing")
	}
}

// TestHoverBodyIsCappedAndSaysSo: four lines is the reading budget, and a prompt
// with more of it ends in an ellipsis — the difference between "that's all of
// it" and "press ctrl+v".
func TestHoverBodyIsCappedAndSaysSo(t *testing.T) {
	long := "title\n" + strings.Repeat("the body runs on and on and keeps running. ", 20)
	body := hoverBody(Todo{Title: "title", Prompt: long}, hoverCardWidth-4)
	if len(body) != hoverBodyLines {
		t.Fatalf("body lines = %d, want %d", len(body), hoverBodyLines)
	}
	if last := body[len(body)-1]; !strings.HasSuffix(last, "…") {
		t.Errorf("last line = %q, want an ellipsis saying there is more", last)
	}
	for i, ln := range body {
		if w := lipgloss.Width(ln); w > hoverCardWidth-4 {
			t.Errorf("body line %d is %d cells wide, past the %d it was wrapped to: %q", i, w, hoverCardWidth-4, ln)
		}
	}
}

// TestHoverCardStaysInThePane pins the placement contract it borrows from the
// menus (placeBelowRight): the box lands below-right of the pointer when there
// is room and is pulled back inside the pane when there is not, so a card is
// never half off the edge.
func TestHoverCardStaysInThePane(t *testing.T) {
	todo := Todo{ID: "t1", Title: "one", Prompt: "one\n" + strings.Repeat("body ", 40)}
	m := hoverModel(todo)

	// Bottom-right corner of the pane: the box has to flip above the pointer and
	// be pulled left, and still sit entirely on screen.
	next, _ := m.Update(tea.MouseMotionMsg{X: m.width - 1, Y: listRowsRow})
	c := next.(model).hover
	if !c.open {
		t.Fatal("no card near the right edge")
	}
	if c.x < 0 || c.x+c.w > m.width || c.y < 0 || c.y+c.h > m.height {
		t.Errorf("card at (%d,%d) %dx%d falls outside the %dx%d pane", c.x, c.y, c.w, c.h, m.width, m.height)
	}

	// A pane too narrow to wrap prose into gets no card rather than a squeezed
	// one — there is nothing legible to put in it.
	narrow := hoverModel(todo)
	narrow.width = hoverCardMin
	narrow.applySizes()
	if hoverOver(t, narrow, 0).hover.open {
		t.Error("a card floated in a pane with no room for one")
	}
}
