package main

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestInfoPromptsRefuseToLeave pins the rule the ｉ mark exists for: a note is
// never handed to an agent. A schedule is refused in words that name the mark
// as the way out, mirroring the frozen refusal (TestFrozenPromptsRefuseToLeave),
// with a live client so the socket guard cannot be what stopped it. The other
// keyboard road, a drop, now files the note in a notes plugin instead of
// refusing — TestInfoSendGoesToNotes (notes_test.go) pins that half.
func TestInfoPromptsRefuseToLeave(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
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

// TestInfoMarkIsAChip pins what makes the info mark findable on a row: it is
// as wide as the emoji it sits beside and it carries its own field — and on a
// closed row it gives the field up, receding like the other marks. It also
// pins withInfoChips' contract: splitting a label around the glyph changes
// neither its text nor its width, so the bar's hit-test spans and the menu's
// box still match what is drawn.
func TestInfoMarkIsAChip(t *testing.T) {
	if w, want := lipgloss.Width(infoGlyph), lipgloss.Width(fruitGlyph); w != want {
		t.Errorf("info glyph is %d cells, want %d like the apple beside it", w, want)
	}

	glyph, st, sel := infoMark(Todo{Info: true})
	if glyph != infoGlyph {
		t.Fatalf("infoMark glyph = %q, want %q", glyph, infoGlyph)
	}
	for name, s := range map[string]lipgloss.Style{"ordinary": st, "selected": sel} {
		if _, bare := s.GetBackground().(lipgloss.NoColor); bare {
			t.Errorf("%s info mark has no field of its own", name)
		}
		if !s.GetItalic() {
			t.Errorf("%s info mark is not italic", name)
		}
	}

	_, closed, _ := infoMark(Todo{Info: true, Done: true})
	if _, bare := closed.GetBackground().(lipgloss.NoColor); !bare {
		t.Error("a closed row's info mark kept its field; it should recede")
	}

	label := "☑ " + infoGlyph + " Info"
	out := withInfoChips(menuRowStyle, label)
	if got := ansi.Strip(out); got != label {
		t.Errorf("withInfoChips text = %q, want %q", got, label)
	}
	if lipgloss.Width(out) != lipgloss.Width(label) {
		t.Errorf("withInfoChips width = %d, want %d", lipgloss.Width(out), lipgloss.Width(label))
	}
	if chip := infoChipStyle.Render(infoGlyph); !strings.Contains(out, chip) {
		t.Errorf("withInfoChips did not draw the glyph as the chip: %q", out)
	}
}
