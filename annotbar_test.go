package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// clickAnnot presses the annotation bar at column x, through the same full
// dispatch a real click takes (stage router → clickForm → the bar's spans).
func clickAnnot(m model, x int) model {
	return clickForm(m, x, formAnnotRow)
}

// TestAnnotBarClickPressesSegments drives the bar the way the pointer does: a
// click on the checkbox toggles it, a click on a radio takes that level, and a
// click on the air between segments presses nothing.
func TestAnnotBarClickPressesSegments(t *testing.T) {
	m := withForm(t, "t", "p", 100, 40)
	segs, _ := m.annotBarLayout()

	m = clickAnnot(m, segs[annotSegFruit].start)
	if !m.formAnnots.Fruit {
		t.Fatal("a click on the checkbox did not set the fruit")
	}
	m = clickAnnot(m, segs[annotSegFruit].end-1)
	if m.formAnnots.Fruit {
		t.Fatal("a second click (on the segment's last column) did not clear it")
	}

	m = clickAnnot(m, segs[annotSegPrioCritical].start)
	if m.formAnnots.Priority != priorityCritical {
		t.Fatalf("a click on the critical radio left %q", m.formAnnots.Priority)
	}
	// The spans move as the texts change state, so re-measure before aiming.
	segs, _ = m.annotBarLayout()
	m = clickAnnot(m, segs[annotSegPrioHigh].start)
	if m.formAnnots.Priority != priorityHigh {
		t.Fatalf("a click on the high radio left %q", m.formAnnots.Priority)
	}
	// "none" is a hole of its own, so clearing a level is the same gesture.
	segs, _ = m.annotBarLayout()
	m = clickAnnot(m, segs[annotSegPrioNone].start)
	if m.formAnnots.Priority != priorityNone {
		t.Fatalf("a click on the none radio left %q", m.formAnnots.Priority)
	}

	// Between the checkbox and the Priority label is layout, not a control.
	segs, _ = m.annotBarLayout()
	before := m.formAnnots
	m = clickAnnot(m, segs[annotSegFruit].end)
	if m.formAnnots != before {
		t.Error("a click on the gap after the checkbox pressed something")
	}
}

// TestAnnotBarClickLeavesTheKeysAlone pins the bar's chip-like half of the
// pointer contract: unlike a click on a field, a click on a segment acts
// without stealing the focus — the caret stays where the typing is.
func TestAnnotBarClickLeavesTheKeysAlone(t *testing.T) {
	m := withForm(t, "t", "p", 100, 40)
	if m.formFocus != formFieldPrompt {
		t.Fatalf("the form opened with focus %d, want the prompt", m.formFocus)
	}
	segs, _ := m.annotBarLayout()
	m = clickAnnot(m, segs[annotSegPrioHigh].start)
	if m.formFocus != formFieldPrompt || !m.promptArea.Focused() {
		t.Errorf("after the click: focus=%d prompt focused=%v — the keys moved",
			m.formFocus, m.promptArea.Focused())
	}
	// But the bar's own cursor parked on the pressed segment, so a later tab
	// onto the bar resumes from where the hand last was.
	if m.annotCursor != annotSegPrioHigh {
		t.Errorf("annotCursor = %d, want it parked on the pressed segment", m.annotCursor)
	}
}

// TestAnnotBarKeysStayOnTheBar: the bar is a stop with no field under it, so
// text and caret keys pressed there must reach neither editor — a space is a
// press, not a character, and a stray word must not land in the prompt.
func TestAnnotBarKeysStayOnTheBar(t *testing.T) {
	m := withForm(t, "t", "body", 100, 40)
	next, _ := m.updateForm(pressKey("tab")) // prompt → the bar
	m = next.(model)
	for _, key := range []string{"x", " ", "up", "down"} {
		next, _ = m.updateForm(pressKey(key))
		m = next.(model)
	}
	if got := m.promptArea.Value(); got != "body" {
		t.Errorf("prompt = %q — keys pressed on the bar reached the editor", got)
	}
	if got := m.titleInput.Value(); got != "t" {
		t.Errorf("title = %q — keys pressed on the bar reached the title", got)
	}
	// The walk clamps at the ends rather than wrapping: ← from the first
	// segment stays put, like the session panel's cursor at its edges.
	next, _ = m.updateForm(pressKey("left"))
	m = next.(model)
	if m.annotCursor != annotSegFruit {
		t.Errorf("annotCursor = %d, want ← clamped at the checkbox", m.annotCursor)
	}
}

// TestAnnotBarFitsNarrowPanes holds the bar to one line at every width: it
// sits on a hit-tested row (formAnnotRow), so wrapping would put the prompt
// editor a line down from where every click on it is aimed. Narrow panes get
// the compact tier — state glyphs without the words — and the spans still
// match what is drawn.
func TestAnnotBarFitsNarrowPanes(t *testing.T) {
	for _, width := range []int{30, 40, 60, 66, 80, 120} {
		m := withForm(t, "t", "p", width, 40)
		m.formAnnots = annots{Priority: priorityCritical, Fruit: true}
		segs, line := m.annotBarLayout()
		if strings.Contains(line, "\n") {
			t.Fatalf("at width %d the bar rendered more than one line", width)
		}
		if w := lipgloss.Width(line); w > width {
			t.Errorf("at width %d the bar is %d cells wide", width, w)
		}
		if last := segs[annotSegCount-1]; lipgloss.Width(line) < last.end {
			t.Errorf("at width %d the last span ends at %d, past the drawn line", width, last.end)
		}
		// Every segment still present and still pressable, however narrow.
		m = clickAnnot(m, segs[annotSegPrioNone].start)
		if m.formAnnots.Priority != priorityNone {
			t.Errorf("at width %d the none radio's span missed its glyphs", width)
		}
	}
}

// TestAnnotBarUnderlinesTheFocusedSegment: the bar has no caret, so the
// underline is the whole answer to "where are my keys" — it must appear
// exactly while the bar holds the form's focus, on exactly the cursor's
// segment, and moving the cursor must not move any click target (the
// underline is styling, not layout).
func TestAnnotBarUnderlinesTheFocusedSegment(t *testing.T) {
	m := withForm(t, "t", "p", 100, 40)
	segsBefore, _ := m.annotBarLayout()
	for i := range annotSegCount {
		if m.annotSegStyle(i).GetUnderline() {
			t.Errorf("segment %d is underlined while the prompt holds the keys", i)
		}
	}
	next, _ := m.updateForm(pressKey("tab"))
	m = next.(model)
	for i := range annotSegCount {
		if got := m.annotSegStyle(i).GetUnderline(); got != (i == m.annotCursor) {
			t.Errorf("segment %d underline = %v with the cursor on %d", i, got, m.annotCursor)
		}
	}
	next, _ = m.updateForm(pressKey("right"))
	m = next.(model)
	segsAfter, _ := m.annotBarLayout()
	for i := range segsBefore {
		if segsBefore[i].start != segsAfter[i].start || segsBefore[i].end != segsAfter[i].end {
			t.Fatalf("moving the bar's cursor moved segment %d's click span", i)
		}
	}
}

// TestAnnotBarTabRing pins the ring's order and the reason for it: the form
// opens in the prompt with tab meaning "the other field", so the bar joins
// the ring after the prompt rather than in its visual place — a stop between
// the two fields would catch the prompt's first keystrokes (spaces included,
// which the bar treats as a press) in a row that is not a text field.
func TestAnnotBarTabRing(t *testing.T) {
	m := withForm(t, "t", "p", 100, 40)
	want := []int{formFieldAnnots, formFieldTitle, formFieldPrompt}
	for _, stop := range want {
		next, _ := m.updateForm(pressKey("tab"))
		m = next.(model)
		if m.formFocus != stop {
			t.Fatalf("tab landed on %d, want %d", m.formFocus, stop)
		}
	}
	next, _ := m.updateForm(pressKey("shift+tab"))
	m = next.(model)
	if m.formFocus != formFieldTitle {
		t.Fatalf("shift+tab from the prompt landed on %d, want the title", m.formFocus)
	}
}

// TestAnnotBarSurvivesSubPanels: opening and closing the ⚙ panel from the bar
// must hand the keys back to the bar — the sub-stage contract every panel
// keeps, extended to the stop that has no cursor to blink.
func TestAnnotBarSurvivesSubPanels(t *testing.T) {
	m := withForm(t, "t", "p", 100, 40)
	next, _ := m.updateForm(pressKey("tab"))
	m = next.(model)
	next, _ = m.updateForm(pressKey("ctrl+r"))
	m = next.(model)
	if m.stage != stageSession {
		t.Fatalf("ctrl+r from the bar left stage %v", m.stage)
	}
	next, _ = m.updateSession(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)
	if m.formFocus != formFieldAnnots {
		t.Errorf("esc gave the keys to %d, want them back on the bar", m.formFocus)
	}
	if m.titleInput.Focused() || m.promptArea.Focused() {
		t.Error("a text field took the keys the bar was holding")
	}
}
