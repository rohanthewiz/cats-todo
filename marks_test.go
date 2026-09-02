package main

import (
	"strings"
	"testing"
)

func TestMarkTogglesAndCounts(t *testing.T) {
	m := withTodos(t, "one", "two", "three")

	m = pressList(t, m, "ctrl+space")
	if m.markCount() != 1 {
		t.Fatalf("after one ctrl+space, markCount = %d, want 1", m.markCount())
	}
	if !strings.Contains(m.status, "1 selected") {
		t.Errorf("status = %q, want the count spoken", m.status)
	}
	// A second press on the same row takes it back out — the tick is a toggle,
	// not an accumulator.
	m = pressList(t, m, "ctrl+space")
	if m.markCount() != 0 {
		t.Fatalf("markCount = %d after toggling the same row twice, want 0", m.markCount())
	}

	// ctrl+b is the alias for terminals that eat ctrl+space.
	m = pressList(t, m, "ctrl+b")
	if m.markCount() != 1 {
		t.Errorf("ctrl+b should tick a row too; markCount = %d", m.markCount())
	}
}

func TestMarkedColumnAppearsOnlyWithASelection(t *testing.T) {
	m := withTodos(t, "one", "two")

	if strings.Contains(m.viewList(), markGlyph) {
		t.Fatal("the ✓ column must not be drawn with nothing selected")
	}
	if m.list.showMarks {
		t.Fatal("showMarks should be off with nothing selected")
	}

	m = pressList(t, m, "ctrl+space")
	view := m.viewList()
	if !m.list.showMarks || !strings.Contains(view, markGlyph) {
		t.Fatalf("a selected row should draw the tick:\n%s", view)
	}
	if !strings.Contains(view, "1 selected") {
		t.Errorf("the header should carry the count:\n%s", view)
	}

	// Clearing puts the column away again.
	m.clearMarks()
	m.rebuildList()
	if strings.Contains(m.viewList(), markGlyph) {
		t.Error("the column should go when the selection does")
	}
}

// The set is keyed by todo, not by row: moving a prompt must not hand the
// selection to whatever slid into its old index. This is the whole reason
// marks.go keys on todoRef.
func TestMarksFollowThePromptThroughAMove(t *testing.T) {
	m := withTodos(t, "one", "two", "three")
	m = pressList(t, m, "ctrl+space") // "one", at the top

	m = pressList(t, m, "ctrl+down") // move it down a row
	refs := m.markedRefs()
	if len(refs) != 1 {
		t.Fatalf("markedRefs = %v, want one", refs)
	}
	td, ok := m.resolve(refs[0])
	if !ok || td.Title != "one" {
		t.Errorf("the selection followed the position instead of the prompt: %+v", td)
	}
}

// A prompt deleted out from under a selection leaves it, rather than sitting
// there as a ref that a later export cannot resolve.
func TestMarksPrunedWhenAPromptGoes(t *testing.T) {
	m := withTodos(t, "one", "two")
	m = pressList(t, m, "ctrl+space")
	if m.markCount() != 1 {
		t.Fatalf("setup: markCount = %d", m.markCount())
	}

	m.project.todos = m.project.todos[1:] // "one" is gone
	m.rebuildList()

	if m.markCount() != 0 {
		t.Errorf("markCount = %d after the selected prompt was removed, want 0", m.markCount())
	}
	if m.list.showMarks {
		t.Error("the column should be off once the set has emptied itself")
	}
}

// markedRefs reads in the order the list draws, and keeps rows the fold is
// hiding: the fold is a way of looking, not a way of unselecting.
func TestMarkedRefsOrderAndHiddenRows(t *testing.T) {
	m := withTodos(t, "one", "two", "three")
	m.marked = map[todoRef]bool{
		{scope: scopeProject, id: "c"}: true,
		{scope: scopeProject, id: "a"}: true,
	}
	m.rebuildList()

	refs := m.markedRefs()
	if len(refs) != 2 || refs[0].id != "a" || refs[1].id != "c" {
		t.Fatalf("markedRefs = %+v, want list order a then c", refs)
	}

	// Complete "a" and fold the closed rows away: it is off the screen but still
	// in the set.
	m.project.todos[0].Done = true
	m.hideDone = true
	m.rebuildList()
	refs = m.markedRefs()
	if len(refs) != 2 {
		t.Fatalf("markedRefs = %+v, want the hidden row still selected", refs)
	}
}

// esc backs out of the selection before it touches the filter, and before it
// quits — the "undo the state I'm in" ladder.
func TestEscClearsTheSelectionFirst(t *testing.T) {
	m := withTodos(t, "one", "two")
	m.list.input.SetValue("on")
	m.list.filter()
	m = pressList(t, m, "ctrl+space")

	m = pressList(t, m, "esc")
	if m.markCount() != 0 {
		t.Fatalf("esc should clear the selection; markCount = %d", m.markCount())
	}
	if m.quitting {
		t.Fatal("esc must not quit while there was a selection to drop")
	}
	if m.list.input.Value() != "on" {
		t.Errorf("esc took the filter too soon: %q", m.list.input.Value())
	}

	m = pressList(t, m, "esc")
	if m.list.input.Value() != "" {
		t.Errorf("the next esc should clear the filter, got %q", m.list.input.Value())
	}
	if m.quitting {
		t.Error("still not the quit press")
	}
}

func TestMarkWithNothingHighlightedSaysSo(t *testing.T) {
	m := newTestModel()
	m.rebuildList()
	m = pressList(t, m, "ctrl+space")
	if m.markCount() != 0 {
		t.Fatalf("markCount = %d on an empty list", m.markCount())
	}
	if !m.statusErr || m.status == "" {
		t.Errorf("an empty list should refuse in words, status = %q err = %v", m.status, m.statusErr)
	}
}
