package main

import (
	"strings"
	"testing"
	"time"

	"github.com/rohanthewiz/cats-todo/internal/app"
)

// TestParseScheduleTime pins the schedule editor's input language against a
// fixed clock: a Monday early afternoon, so "before" and "after" today are
// both reachable with plain times.
func TestParseScheduleTime(t *testing.T) {
	now := time.Date(2026, 8, 3, 13, 0, 0, 0, time.Local) // Mon 13:00

	day := func(d, hh, mm int) time.Time {
		return time.Date(2026, 8, d, hh, mm, 0, 0, time.Local)
	}

	for _, tc := range []struct {
		in   string
		want time.Time
	}{
		{"15:30", day(3, 15, 30)}, // later today
		{"9:05", day(4, 9, 5)},    // already past — rolls to tomorrow
		{"13:00", day(4, 13, 0)},  // exactly now is not "in the future"
		{"in 2h", now.Add(2 * time.Hour)},
		{"2h", now.Add(2 * time.Hour)},
		{"in 1h30m", now.Add(90 * time.Minute)},
		{"tomorrow 9:00", day(4, 9, 0)},
		{"tomorrow 23:59", day(4, 23, 59)},
		{"2026-08-03 15:04", day(3, 15, 4)}, // the prefill's own format round-trips
		{"  15:30  ", day(3, 15, 30)},       // whitespace is the user's business, not an error
	} {
		got, err := parseScheduleTime(tc.in, now)
		if err != nil {
			t.Errorf("parseScheduleTime(%q) error: %v", tc.in, err)
			continue
		}
		if !got.Equal(tc.want) {
			t.Errorf("parseScheduleTime(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}

	for _, in := range []string{
		"",                 // nothing typed
		"soonish",          // not a time in any form
		"25:00",            // no such hour
		"12:75",            // no such minute
		"tomorrow",         // tomorrow when?
		"tomorrow 25:00",   // bad clock behind a good prefix
		"in -2h",           // the past, spelled as a duration
		"2026-08-03 12:00", // the past, spelled out — must not roll or fire now
	} {
		if got, err := parseScheduleTime(in, now); err == nil {
			t.Errorf("parseScheduleTime(%q) = %v, want an error", in, got)
		}
	}

	t.Run("errors name the accepted forms", func(t *testing.T) {
		_, err := parseScheduleTime("whenever", now)
		if err == nil || !strings.Contains(err.Error(), "15:30") {
			t.Fatalf("err = %v, want the forms listed so the user isn't guessing", err)
		}
	})
}

// TestFormatScheduleTime pins the three precisions: clock today, weekday
// inside a week, full date beyond — in both directions, since missed
// schedules render times in the past.
func TestFormatScheduleTime(t *testing.T) {
	now := time.Date(2026, 8, 3, 13, 0, 0, 0, time.Local) // Mon Aug 3

	for _, tc := range []struct {
		name string
		at   time.Time
		want string
	}{
		{"later today", time.Date(2026, 8, 3, 15, 4, 0, 0, time.Local), "15:04"},
		{"earlier today (missed)", time.Date(2026, 8, 3, 9, 30, 0, 0, time.Local), "09:30"},
		{"within the week", time.Date(2026, 8, 6, 9, 0, 0, 0, time.Local), "Thu 09:00"},
		{"yesterday (missed)", time.Date(2026, 8, 2, 20, 0, 0, 0, time.Local), "Sun 20:00"},
		{"beyond the week", time.Date(2026, 8, 20, 9, 0, 0, 0, time.Local), "Aug 20 09:00"},
	} {
		if got := formatScheduleTime(tc.at, now); got != tc.want {
			t.Errorf("%s: formatScheduleTime = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestScheduleTargetRoundTrip pins the conversion both ways: what the picker
// chose is what the fire hands back to performDrop, for both target kinds.
func TestScheduleTargetRoundTrip(t *testing.T) {
	at := time.Date(2026, 8, 3, 15, 0, 0, 0, time.Local)

	t.Run("existing pane", func(t *testing.T) {
		in := dropTarget{kind: targetExistingPane, pane: 42, agent: "claude", label: "x", desc: "y"}
		sc := scheduleFromTarget(in, at, "/tmp/proj")
		if sc.Kind != scheduleKindPane || sc.Pane != 42 || sc.Agent != "claude" {
			t.Fatalf("scheduleFromTarget = %+v, lost the pane identity", sc)
		}
		if !sc.At.Equal(at) {
			t.Fatalf("At = %v, want %v", sc.At, at)
		}
		out := targetFromSchedule(sc)
		if out.kind != targetExistingPane || out.pane != 42 || out.agent != "claude" {
			t.Fatalf("targetFromSchedule = %+v, not the pane that was picked", out)
		}
	})

	t.Run("new session", func(t *testing.T) {
		in := dropTarget{kind: targetNewSession, command: "codex"}
		sc := scheduleFromTarget(in, at, "/tmp/proj")
		if sc.Kind != scheduleKindNew || sc.Command != "codex" || sc.Cwd != "/tmp/proj" {
			t.Fatalf("scheduleFromTarget = %+v, lost the launch spec", sc)
		}
		out := targetFromSchedule(sc)
		if out.kind != targetNewSession || out.command != "codex" {
			t.Fatalf("targetFromSchedule = %+v, not the session that was picked", out)
		}
	})

	t.Run("empty command falls back to claude", func(t *testing.T) {
		sc := scheduleFromTarget(dropTarget{kind: targetNewSession}, at, "")
		if sc.Command != "claude" {
			t.Fatalf("Command = %q, want the claude default", sc.Command)
		}
	})
}

// TestPaneExists pins the fire path's one judgment call.
func TestPaneExists(t *testing.T) {
	panes := []app.PaneInfo{{Pane: 3}, {Pane: 7}}
	if !paneExists(panes, 7) {
		t.Error("pane 7 is right there")
	}
	if paneExists(panes, 9) {
		t.Error("pane 9 is gone and must read as gone")
	}
	if paneExists(nil, 3) {
		t.Error("no panes at all cannot contain one")
	}
}
