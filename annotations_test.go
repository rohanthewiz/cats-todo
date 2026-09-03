package main

import (
	"os"
	"strings"
	"testing"
)

// TestAnnotsRoundTripThroughTheTodo pins the two accessors against each other.
// They are the only path the form and the CLI use to move annotations on and off
// a todo, so a mark added to the struct and forgotten in one of them would be
// silently dropped on every save.
func TestAnnotsRoundTripThroughTheTodo(t *testing.T) {
	want := annots{Priority: priorityCritical, Fruit: true}
	var td Todo
	want.applyTo(&td)
	if got := annotsOf(td); got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
	// And applying the zero set clears what was there, which is what makes
	// "cycle back to none" and "untick the row" work at all.
	annots{}.applyTo(&td)
	if got := annotsOf(td); got != (annots{}) {
		t.Errorf("clearing left %+v, want the zero set", got)
	}
}

// TestAnnotsSummaryNamesOnlyWhatWasSaid pins the words the screens without room
// to draw the marks fall back on — the form's ⚙ line, the CLI's echo after an
// add. Silence for an unannotated prompt is the load-bearing half: a default
// announcing itself on every add is noise.
func TestAnnotsSummaryNamesOnlyWhatWasSaid(t *testing.T) {
	cases := []struct {
		in   annots
		want string
	}{
		{annots{}, ""},
		{annots{Priority: priorityHigh}, "high"},
		{annots{Priority: priorityCritical}, "critical"},
		{annots{Fruit: true}, "low-hanging fruit"},
		// Slot order, so the words and the columns read the same way round.
		{annots{Priority: priorityCritical, Fruit: true}, "critical · low-hanging fruit"},
	}
	for _, c := range cases {
		if got := c.in.summary(); got != c.want {
			t.Errorf("%+v summary = %q, want %q", c.in, got, c.want)
		}
		if got := c.in.any(); got != (c.want != "") {
			t.Errorf("%+v any() = %v, want %v", c.in, got, c.want != "")
		}
	}
}

// TestAnnotationsStayOutOfTheJSONWhenUnset is the compat contract every field on
// Todo carries, checked for the newest one: a backlog nobody has annotated is
// byte-for-byte the backlog this program wrote before annotations existed, and
// an older binary reading it sees a plain todo rather than choking on a key it
// does not know.
func TestAnnotationsStayOutOfTheJSONWhenUnset(t *testing.T) {
	s := tempStore(t)
	if err := s.add(Todo{ID: "a1", Prompt: "p"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"priority", "fruit"} {
		if strings.Contains(string(data), key) {
			t.Fatalf("an unannotated todo wrote a %q key:\n%s", key, data)
		}
	}

	if err := s.setAnnots("a1", annots{Fruit: true}); err != nil {
		t.Fatal(err)
	}
	reloaded := &store{scope: scopeProject, path: s.path}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if td, _ := reloaded.find("a1"); !td.Fruit {
		t.Fatal("the fruit did not survive a save/load round trip")
	}

	// And unticking it takes the key back out rather than storing "false".
	if err := s.setAnnots("a1", annots{}); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "fruit") {
		t.Fatalf("clearing the fruit left a key behind:\n%s", data)
	}
}

// TestSetAnnotsWritesTheWholeSetAtOnce pins why there is one method rather than
// one per mark: an edit that raises a priority and clears a fruit is one save,
// so there is no window in which the file holds half of it.
func TestSetAnnotsWritesTheWholeSetAtOnce(t *testing.T) {
	s := tempStore(t)
	if err := s.add(Todo{ID: "a1", Prompt: "p", Priority: priorityHigh, Fruit: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.setAnnots("a1", annots{Priority: priorityCritical}); err != nil {
		t.Fatal(err)
	}
	td, _ := s.find("a1")
	if td.Priority != priorityCritical || td.Fruit {
		t.Errorf("after the write the todo is %+v, want critical with no fruit", annotsOf(td))
	}
	// The usual honesty about a stale pane: a mark aimed at a todo that is gone
	// says so rather than reporting success.
	if err := s.setAnnots("gone", annots{}); err != errTodoNotFound {
		t.Errorf("setAnnots on a missing todo returned %v, want errTodoNotFound", err)
	}
}

// TestEverySlotDrawsADistinctGlyph pins what the packed group rests on. With
// reserved columns a mark was identified by where it sat, so two slots could in
// principle have shared a shape; packed, position says nothing — a lone apple
// and a lone triangle both sit in the first cell after the badge — and the glyph
// is the whole of the answer. Two slots drawing the same shape would leave the
// row ambiguous with no way for the reader to resolve it.
func TestEverySlotDrawsADistinctGlyph(t *testing.T) {
	// One todo carrying every mark this build knows how to draw, so each slot
	// has something to hand back.
	all := Todo{ID: "x", Prompt: "p", Priority: priorityCritical, Fruit: true}
	for _, sl := range annotSlots {
		if glyph, _, _ := sl.mark(all); glyph == "" {
			t.Errorf("slot %q drew nothing for a fully annotated todo — this test can no longer measure it", sl.name)
		}
	}
	seen := map[string]string{}
	for _, sl := range annotSlots {
		for _, td := range []Todo{
			{Priority: priorityCritical}, {Priority: priorityHigh}, {Fruit: true},
		} {
			glyph, _, _ := sl.mark(td)
			if glyph == "" {
				continue
			}
			if other, dup := seen[glyph]; dup && other != sl.name {
				t.Errorf("slots %q and %q both draw %q", other, sl.name, glyph)
			}
			seen[glyph] = sl.name
		}
	}
}
