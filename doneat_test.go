package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TestDoneAt pins the completion stamp's lifecycle: every road to done stamps
// it, every road away from done clears it, and a todo that was never completed
// writes no key at all (the todos.json compatibility contract).
func TestDoneAt(t *testing.T) {
	t.Run("toggle stamps on completion and clears on reopen", func(t *testing.T) {
		s := tempStore(t)
		if err := s.add(Todo{ID: "a1", Prompt: "p"}); err != nil {
			t.Fatal(err)
		}
		before := time.Now()
		if err := s.toggle("a1"); err != nil {
			t.Fatal(err)
		}
		td, _ := s.find("a1")
		if td.DoneAt.Before(before) || td.DoneAt.After(time.Now()) {
			t.Fatalf("DoneAt = %v, want a stamp taken at completion", td.DoneAt)
		}
		if err := s.toggle("a1"); err != nil {
			t.Fatal(err)
		}
		if td, _ := s.find("a1"); !td.DoneAt.IsZero() {
			t.Fatalf("DoneAt = %v after reopen, want it cleared", td.DoneAt)
		}
	})

	t.Run("setDone stamps once and clears on false", func(t *testing.T) {
		s := tempStore(t)
		if err := s.add(Todo{ID: "a1", Prompt: "p"}); err != nil {
			t.Fatal(err)
		}
		if err := s.setDone("a1", true); err != nil {
			t.Fatal(err)
		}
		first, _ := s.find("a1")
		if first.DoneAt.IsZero() {
			t.Fatal("setDone(true) left no stamp")
		}
		// A repeat (e.g. a second auto-complete after a drop) must not move the
		// stamp forward — the work was finished the first time.
		if err := s.setDone("a1", true); err != nil {
			t.Fatal(err)
		}
		if again, _ := s.find("a1"); !again.DoneAt.Equal(first.DoneAt) {
			t.Fatalf("repeat setDone moved DoneAt %v → %v", first.DoneAt, again.DoneAt)
		}
		if err := s.setDone("a1", false); err != nil {
			t.Fatal(err)
		}
		if td, _ := s.find("a1"); !td.DoneAt.IsZero() {
			t.Fatal("setDone(false) left the stamp behind")
		}
	})

	t.Run("freezing a done todo clears the stamp", func(t *testing.T) {
		s := tempStore(t)
		if err := s.add(Todo{ID: "a1", Prompt: "p"}); err != nil {
			t.Fatal(err)
		}
		if err := s.setDone("a1", true); err != nil {
			t.Fatal(err)
		}
		if _, err := s.toggleFrozen("a1"); err != nil {
			t.Fatal(err)
		}
		if td, _ := s.find("a1"); !td.DoneAt.IsZero() {
			t.Fatal("a frozen todo still carries a completion stamp")
		}
	})

	t.Run("the stamp survives a reload in the same instant", func(t *testing.T) {
		s := tempStore(t)
		if err := s.add(Todo{ID: "a1", Prompt: "p"}); err != nil {
			t.Fatal(err)
		}
		if err := s.setDone("a1", true); err != nil {
			t.Fatal(err)
		}
		want, _ := s.find("a1")
		if err := s.reload(); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.find("a1"); !got.DoneAt.Equal(want.DoneAt) {
			t.Fatalf("DoneAt after reload = %v, want %v", got.DoneAt, want.DoneAt)
		}
	})

	t.Run("an unstamped todo writes no doneAt key", func(t *testing.T) {
		b, err := json.Marshal(Todo{ID: "a1", Prompt: "p", Done: true})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "doneAt") {
			t.Fatalf("marshalled %s, want no doneAt key for a zero stamp", b)
		}
	})
}

// TestDoneAtIsDrawn checks both places the stamp surfaces: the compact mark on
// a done row, and the full local stamp (date, time, zone) in the prompt view.
func TestDoneAtIsDrawn(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	if err := project.add(Todo{ID: "x", Title: "task", Prompt: "do it"}); err != nil {
		t.Fatal(err)
	}
	if err := project.setDone("x", true); err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = next.(model)
	m.rebuildList()

	td, _ := project.find("x")
	if row := stripANSI(m.viewList()); !strings.Contains(row, "done "+formatDoneTime(td.DoneAt, time.Now())) {
		t.Errorf("list does not draw the completion stamp:\n%s", row)
	}

	m.viewRef = todoRef{scope: project.scope, id: "x"}
	want := "done " + td.DoneAt.Local().Format("2006-01-02 15:04 MST")
	if view := stripANSI(m.viewPrompt()); !strings.Contains(view, want) {
		t.Errorf("prompt view meta lacks %q:\n%s", want, view)
	}
}

// TestFormatDoneTime covers the one rung formatDoneTime adds to the schedule
// ladder: a year that is not this one is spelled out.
func TestFormatDoneTime(t *testing.T) {
	now := time.Date(2026, 9, 13, 16, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		at   time.Time
		want string
	}{
		{time.Date(2026, 9, 13, 14, 5, 0, 0, time.Local), "14:05"},
		{time.Date(2026, 9, 10, 14, 5, 0, 0, time.Local), "Thu 14:05"},
		{time.Date(2026, 3, 2, 9, 0, 0, 0, time.Local), "Mar 2 09:00"},
		{time.Date(2025, 12, 31, 23, 30, 0, 0, time.Local), "2025-12-31 23:30"},
	} {
		if got := formatDoneTime(tc.at, now); got != tc.want {
			t.Errorf("formatDoneTime(%v) = %q, want %q", tc.at, got, tc.want)
		}
	}
}
