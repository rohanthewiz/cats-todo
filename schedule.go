// schedule.go — the pure half of scheduled drops: parsing what the user typed
// into a fire time, formatting that time back for rows and status lines, and
// converting between the picker's dropTarget and the persisted Schedule. The
// tick loop and the stage UI live in ui.go; the store methods in store.go.
package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// scheduleGrace is how far past its time a schedule may still fire. Inside the
// window, a tick that comes late — the UI thread was busy, the laptop dozed
// for a moment — still delivers. Beyond it the schedule is marked Missed
// instead: the pane it named is hours staler than the user planned for, and
// "opening the todo manager triggers surprise agent runs" is exactly the
// wrong way to learn the laptop slept through a fire time. One rule covers
// launch-with-past-schedule, suspend/resume, and the manager simply having
// been closed.
const scheduleGrace = 2 * time.Minute

// scheduleTimeStamp is the unambiguous datetime form parseScheduleTime
// accepts and the schedule editor prefills — the round trip that lets "edit
// the time" mean pressing enter on what is already there.
const scheduleTimeStamp = "2006-01-02 15:04"

// parseScheduleTime reads the schedule editor's one input into a fire time:
//
//	"15:30", "9:05"     — next occurrence: today, or tomorrow if already past
//	"in 2h", "2h", "1h30m" — a duration from now
//	"tomorrow 9:00"     — tomorrow at that clock time
//	"2006-01-02 15:04"  — an explicit datetime (the prefill's own format)
//
// A bare clock time that has passed today rolls forward — "9:00" typed at
// noon means tomorrow morning, not an error. An explicit datetime in the past
// is an error: the user spelled out a moment, and silently firing now (or
// rolling it a day) would not be what they wrote.
func parseScheduleTime(input string, now time.Time) (time.Time, error) {
	s := strings.ToLower(strings.TrimSpace(input))
	if s == "" {
		return time.Time{}, errors.New("no time given")
	}

	if t, err := time.ParseInLocation(scheduleTimeStamp, s, now.Location()); err == nil {
		if !t.After(now) {
			return time.Time{}, fmt.Errorf("%s is in the past", input)
		}
		return t, nil
	}

	if rest, ok := strings.CutPrefix(s, "tomorrow "); ok {
		hh, mm, err := parseClock(strings.TrimSpace(rest))
		if err != nil {
			return time.Time{}, scheduleParseError(input)
		}
		d := now.AddDate(0, 0, 1)
		return time.Date(d.Year(), d.Month(), d.Day(), hh, mm, 0, 0, now.Location()), nil
	}

	if hh, mm, err := parseClock(s); err == nil {
		t := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, now.Location())
		if !t.After(now) {
			t = t.AddDate(0, 0, 1)
		}
		return t, nil
	}

	if d, err := time.ParseDuration(strings.TrimPrefix(s, "in ")); err == nil {
		if d <= 0 {
			return time.Time{}, fmt.Errorf("%s is not a time in the future", input)
		}
		return now.Add(d), nil
	}

	return time.Time{}, scheduleParseError(input)
}

func scheduleParseError(input string) error {
	return fmt.Errorf("can't read %q — try 15:30 · in 2h · tomorrow 9:00 · 2026-08-03 15:04", input)
}

// parseClock reads a bare "HH:MM". Hand-rolled rather than time.Parse so
// "9:05" and "09:05" are both fine and the failure is a plain error instead
// of a layout mismatch.
func parseClock(s string) (hh, mm int, err error) {
	h, m, ok := strings.Cut(s, ":")
	if !ok {
		return 0, 0, errors.New("not a clock time")
	}
	hh, err = strconv.Atoi(h)
	if err != nil || hh < 0 || hh > 23 {
		return 0, 0, errors.New("bad hour")
	}
	mm, err = strconv.Atoi(m)
	if err != nil || mm < 0 || mm > 59 {
		return 0, 0, errors.New("bad minute")
	}
	return hh, mm, nil
}

// formatScheduleTime renders a fire time at the precision the distance from
// now deserves: bare clock today, weekday inside a week, date beyond. Rows
// re-render every tick, so this is also what keeps "⏰ 15:04" honest as today
// turns into tomorrow.
func formatScheduleTime(at, now time.Time) string {
	sameDay := at.Year() == now.Year() && at.YearDay() == now.YearDay()
	within := func(d time.Duration) bool {
		diff := at.Sub(now)
		return diff > -d && diff < d
	}
	switch {
	case sameDay:
		return at.Format("15:04")
	case within(7 * 24 * time.Hour):
		return at.Format("Mon 15:04")
	default:
		return at.Format("Jan 2 15:04")
	}
}

// scheduleFromTarget records a picker choice as a Schedule: the pane's
// identity for an existing session, the launch command and directory for a
// new one. cwd is captured now because the fire happens with no picker on
// screen to ask — the same reason pendingAction carries it.
func scheduleFromTarget(t dropTarget, at time.Time, cwd string) Schedule {
	sc := Schedule{At: at}
	if t.kind == targetExistingPane {
		sc.Kind = scheduleKindPane
		sc.Pane = t.pane
		sc.Agent = t.agent
		return sc
	}
	sc.Kind = scheduleKindNew
	sc.Command = firstNonEmpty(t.command, "claude")
	sc.Cwd = cwd
	sc.Worktree = t.worktree
	return sc
}

// targetFromSchedule is scheduleFromTarget's inverse — the dropTarget the
// fire hands to performDrop. Only the fields the drop path reads are
// restored; the picker's label and desc were display sugar.
func targetFromSchedule(sc Schedule) dropTarget {
	if sc.Kind == scheduleKindPane {
		return dropTarget{kind: targetExistingPane, pane: sc.Pane, agent: sc.Agent}
	}
	return dropTarget{
		kind:     targetNewSession,
		command:  firstNonEmpty(sc.Command, "claude"),
		worktree: sc.Worktree,
	}
}
