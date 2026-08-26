package main

import (
	"os"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestNormalizePriority pins the closed set and the folding around it. The
// spellings matter beyond convenience: they are what the CLI flag accepts and
// what the completion menu offers, and a typo silently becoming none would be a
// prompt that quietly stopped being urgent.
//
// The retired words carry their own line here. When the levels changed from
// standard/critical/low to none/high/critical, every shell history and script
// holding `--priority low` had to keep working and keep meaning what it meant —
// "not raised", which is now none.
func TestNormalizePriority(t *testing.T) {
	ok := map[string]string{
		"":          priorityNone,
		"none":      priorityNone,
		"None":      priorityNone,
		"  HIGH  ":  priorityHigh,
		"high":      priorityHigh,
		"raised":    priorityHigh,
		"important": priorityHigh,
		"critical":  priorityCritical,
		"Critical":  priorityCritical,
		"CRITICAL":  priorityCritical,
		"crit":      priorityCritical,
		"urgent":    priorityCritical,
		// The old scheme's words, folded onto what they used to mean.
		"standard": priorityNone,
		"normal":   priorityNone,
		"std":      priorityNone,
		"medium":   priorityNone,
		"default":  priorityNone,
		"low":      priorityNone,
		"minor":    priorityNone,
	}
	for in, want := range ok {
		got, err := normalizePriority(in)
		if err != nil {
			t.Errorf("normalizePriority(%q) errored: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("normalizePriority(%q) = %q, want %q", in, got, want)
		}
	}

	// The rejection names the whole set, so the error is the documentation.
	if _, err := normalizePriority("urgnt"); err == nil {
		t.Fatal("a misspelled priority was accepted")
	} else {
		want := `priority "urgnt" is not one of critical, high, none`
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err, want)
		}
	}
}

// TestPriorityRingWrapsAndKeepsUnknowns pins the cycle key's ring. The wrap is
// what makes one chord enough for three levels; keeping an unknown value is what
// stops a hand-edited backlog from being silently rewritten by a stray press.
//
// The order is the escalation: one press from the default raises the prompt, a
// second raises it as far as it goes.
func TestPriorityRingWrapsAndKeepsUnknowns(t *testing.T) {
	want := []string{priorityHigh, priorityCritical, priorityNone}
	cur := priorityNone
	for i, w := range want {
		cur = cycleValue(prioValues, cur, 1)
		if cur != w {
			t.Fatalf("press %d landed on %q, want %q", i+1, cur, w)
		}
	}
	// A value this build does not know (a hand-edited backlog, a level a later
	// version adds) is carried into the ring rather than replaced, so it takes a
	// deliberate press to leave it — and that press lands on a real level rather
	// than on nothing. Stepping back from the first entry reaches it again,
	// which is what "in the ring" means.
	if got := cycleValue(prioValues, "someday", 1); !slices.Contains(prioValues, got) {
		t.Errorf("cycling off an unknown priority landed on %q, want a known level", got)
	}
	if got := cycleValue(prioValues, "someday", -1); got != priorityCritical {
		t.Errorf("cycling back from an unknown priority = %q, want %q", got, priorityCritical)
	}
	// A backlog holding the retired "low" is one of those unknowns: it is not in
	// the ring, so it survives until someone deliberately steps off it.
	if got := cycleValue(prioValues, "low", 1); !slices.Contains(prioValues, got) {
		t.Errorf("cycling off the retired \"low\" landed on %q, want a known level", got)
	}
}

// TestPriorityGlyphIsOneCell pins the marks' width against the column widths
// annotSlots declares for them. They sit in a column of their own ahead of every
// title, so a glyph the terminal draws wider than its slot would push each name
// right of where the action bar and the click hit tests expect it.
func TestPriorityGlyphIsOneCell(t *testing.T) {
	for _, g := range []string{prioCriticalGlyph, prioHighGlyph} {
		if w := lipgloss.Width(g); w != 1 {
			t.Errorf("priority glyph %q is %d cells wide, want 1", g, w)
		}
	}
	// The precedent it rests on: the open badge has always been one cell.
	if w := lipgloss.Width("○"); w != 1 {
		t.Errorf("the ○ badge is %d cells wide — the ▲/△ assumption no longer holds", w)
	}
}

// TestPriorityRankOrdersCriticalFirstAndParksUnknowns pins the sort key. An
// unrecognized value ranks with none rather than at either edge: a typo — or the
// retired "low" — should leave a row among the unmarked, not promote it above
// work actually marked critical.
func TestPriorityRankOrdersCriticalFirstAndParksUnknowns(t *testing.T) {
	if !(priorityRank(priorityCritical) < priorityRank(priorityHigh) &&
		priorityRank(priorityHigh) < priorityRank(priorityNone)) {
		t.Fatal("ranks do not order critical < high < none")
	}
	if priorityRank("nonsense") != priorityRank(priorityNone) {
		t.Error("an unknown priority does not rank as none")
	}
	if priorityRank("low") != priorityRank(priorityNone) {
		t.Error(`the retired "low" does not rank as none`)
	}
}

// TestPriorityStaysOutOfTheJSONAtNone is the compat contract the field's comment
// promises, in the shape the frozen flag's test uses: a backlog nobody has
// ranked is byte-for-byte the backlog this program wrote before the field
// existed.
func TestPriorityStaysOutOfTheJSONAtNone(t *testing.T) {
	s := tempStore(t)
	if err := s.add(Todo{ID: "a1", Prompt: "p"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "priority") {
		t.Fatalf("an unranked todo wrote a priority key:\n%s", data)
	}

	if err := s.setAnnots("a1", annots{Priority: priorityCritical}); err != nil {
		t.Fatal(err)
	}
	reloaded := &store{scope: scopeProject, path: s.path}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if td, _ := reloaded.find("a1"); td.Priority != priorityCritical {
		t.Fatalf("priority did not survive a save/load round trip, got %q", td.Priority)
	}

	// And back to none takes the key out again, rather than storing the word —
	// which is the whole reason none is the empty string.
	if err := s.setAnnots("a1", annots{}); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "priority") {
		t.Fatalf("setting none left a priority key behind:\n%s", data)
	}
}

// TestUpdateDoesNotClearPriority pins the reason setAnnots is its own method:
// store.update is text-only, so saving an edit to a prompt's wording must not
// reset a triage decision the editing code path knows nothing about.
func TestUpdateDoesNotClearPriority(t *testing.T) {
	s := tempStore(t)
	if err := s.add(Todo{ID: "a1", Title: "t", Prompt: "p"}); err != nil {
		t.Fatal(err)
	}
	if err := s.setAnnots("a1", annots{Priority: priorityCritical, Fruit: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.update(Todo{ID: "a1", Title: "new title", Prompt: "new prompt"}); err != nil {
		t.Fatal(err)
	}
	td, _ := s.find("a1")
	if td.Priority != priorityCritical {
		t.Errorf("an edit blanked the priority: %q", td.Priority)
	}
	if !td.Fruit {
		t.Error("an edit blanked the low-hanging-fruit mark")
	}
	if td.Title != "new title" {
		t.Errorf("the edit did not take: %q", td.Title)
	}
}
