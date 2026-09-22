package main

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TestInfoPromptsRefuseToLeave pins the rule the ℹ mark exists for: a note is
// never handed to an agent. Both keyboard roads — a drop and a schedule — are
// refused in words that name the mark as the way out, mirroring the frozen
// refusal (TestFrozenPromptsRefuseToLeave), with a live client so the socket
// guard cannot be what stopped them.
func TestInfoPromptsRefuseToLeave(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"drop", tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}},
		{"schedule", tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, project, _ := newModelInTemp(t)
			if err := project.add(Todo{ID: "a", Prompt: "remember the api quirk", Info: true}); err != nil {
				t.Fatal(err)
			}
			m.rebuildList()
			m.client = &catsClient{}

			next, _ := m.updateList(tc.key)
			m = next.(model)
			if m.stage != stageList {
				t.Fatalf("stage = %v, want to stay on the list — a note has nowhere to go", m.stage)
			}
			if !strings.Contains(m.status, "info") || !strings.Contains(m.status, "ℹ Info") {
				t.Errorf("status = %q, want it to name the mark and the way out", m.status)
			}
		})
	}
}

// TestInfoMarkClearsSchedule: raising the mark takes a pending schedule with
// it, since a schedule is a deferred drop and a note is never dropped. Lowering
// it restores nothing — the schedule is gone, not parked — and a row that was
// never marked keeps its schedule through an unrelated annotation save.
func TestInfoMarkClearsSchedule(t *testing.T) {
	s := tempStore(t)
	at := time.Now().Add(time.Hour)
	for _, id := range []string{"a", "b"} {
		if err := s.add(Todo{ID: id, Prompt: "p", Schedule: &Schedule{At: at, Kind: scheduleKindNew, Command: "claude"}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.setAnnots("a", annots{Info: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.setAnnots("b", annots{Fruit: true}); err != nil {
		t.Fatal(err)
	}
	a, _ := s.find("a")
	b, _ := s.find("b")
	if !a.Info || a.Schedule != nil {
		t.Errorf("after marking info: Info=%v Schedule=%v, want the mark up and the schedule gone", a.Info, a.Schedule)
	}
	if b.Schedule == nil {
		t.Error("an unrelated annotation save cleared a schedule")
	}
}

// TestInfoPromptsAreNotFired is the backstop for a backlog edited by hand into
// holding both a schedule and the mark: the tick skips it outright rather than
// firing it or recording it as missed (which would advertise a manual send the
// mark forbids). No client, so a fire attempt would show up as Missed.
func TestInfoPromptsAreNotFired(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	due := time.Now().Add(-time.Second)
	if err := project.add(Todo{ID: "a", Prompt: "p", Info: true,
		Schedule: &Schedule{At: due, Kind: scheduleKindNew, Command: "claude"}}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()
	m.fireDueSchedules(time.Now())
	td, _ := project.find("a")
	if td.Schedule == nil || td.Schedule.Missed {
		t.Errorf("schedule = %+v, want it left untouched by the tick", td.Schedule)
	}
}

// TestInfoRoundTripsAndStaysOutOfTheFile: the mark survives the accessors and
// the JSON, and an unmarked todo writes no "info" key at all — the todos.json
// compatibility contract every annotation keeps.
func TestInfoRoundTripsAndStaysOutOfTheFile(t *testing.T) {
	var td Todo
	annots{Info: true}.applyTo(&td)
	if !annotsOf(td).Info {
		t.Error("Info did not survive applyTo → annotsOf")
	}
	if got := annotsOf(td).summary(); !strings.Contains(got, "info") {
		t.Errorf("summary = %q, want the mark spelled out", got)
	}

	s := tempStore(t)
	if err := s.add(Todo{ID: "plain", Prompt: "p"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"info"`) {
		t.Errorf("an unmarked todo wrote an info key:\n%s", raw)
	}
}
