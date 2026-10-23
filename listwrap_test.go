package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// wrappedList is a temp-store model (so the toggle's save lands in a temp
// config dir) holding one titled prompt with a multi-line body and one
// untitled prompt, at a width that makes the long line wrap.
func wrappedList(t *testing.T) model {
	t.Helper()
	m, project, _ := newModelInTemp(t)
	for _, td := range []Todo{
		{ID: "a", Title: "Fix the drop timeout", Prompt: "The twelve second wait comes from stale ready probes, so capture a startup first.\n\n  - then check the panesetup pattern"},
		{ID: "b", Prompt: "untitled first line\nsecond line"},
	} {
		if err := project.add(td); err != nil {
			t.Fatal(err)
		}
	}
	m.width, m.height = 60, 30
	m.applySizes()
	m.rebuildList()
	return m
}

// TestListSoftWrapShowsThePrompt is the feature: alt+z on the list draws each
// prompt's whole text under its row — wrapped at the pane, line breaks and the
// bullet's indent kept, the blank line dropped — and saves the choice.
func TestListSoftWrapShowsThePrompt(t *testing.T) {
	m := wrappedList(t)
	before := ansi.Strip(m.viewList())
	if strings.Contains(before, "panesetup pattern") {
		t.Fatalf("an unwrapped row already shows the prompt's last line:\n%s", before)
	}

	next, _ := m.updateList(tea.KeyPressMsg{Code: 'z', Mod: tea.ModAlt})
	m = next.(model)
	if !m.listSoftWrap {
		t.Fatal("alt+z did not turn the list's soft wrap on")
	}
	if !loadSettings().listSoftWrap {
		t.Error("the toggle was not saved to settings.json")
	}
	if m.list.maxLines == 0 || m.list.maxRows != 0 {
		t.Errorf("wrapped window: maxLines=%d maxRows=%d, want a line window", m.list.maxLines, m.list.maxRows)
	}

	view := ansi.Strip(m.viewList())
	for _, want := range []string{
		"Fix the drop timeout",
		"The twelve second wait",
		"    - then check the panesetup pattern", // two cells of indent, kept
		"untitled first line",
		"second line",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("wrapped list is missing %q:\n%s", want, view)
		}
	}
	// The untitled prompt's first line is its name, drawn once, not again as
	// the first continuation line.
	if n := strings.Count(view, "untitled first line"); n != 1 {
		t.Errorf("the untitled prompt's first line is drawn %d times, want once", n)
	}
	for i, ln := range listRowLines(view) {
		if w := ansi.StringWidth(ln); w > m.width {
			t.Errorf("row line %d is %d cells in a %d-cell pane: %q", i, w, m.width, ln)
		}
	}

	// alt+z again puts it back.
	next, _ = m.updateList(tea.KeyPressMsg{Code: 'z', Mod: tea.ModAlt})
	m = next.(model)
	if m.listSoftWrap || strings.Contains(ansi.Strip(m.viewList()), "panesetup pattern") {
		t.Error("alt+z did not turn soft wrap back off")
	}
}

// TestListSoftWrapHitTestsEveryLine: a click on a continuation line picks the
// row it belongs to — the rows below a wrapped one sit lower than one line per
// row, and rowAtLine has to count the extra lines or hand clicks to the wrong
// prompt.
func TestListSoftWrapHitTestsEveryLine(t *testing.T) {
	m := wrappedList(t)
	next, _ := m.toggleListWrap()
	m = next.(model)

	first := m.list.filtered[0].item
	if !first.selectable || len(first.more) < 2 {
		t.Fatalf("the first row did not wrap: %+v", first)
	}
	// The row's last continuation line, and the line after it — the next
	// prompt's first.
	last := len(first.more)
	if i, ok := m.list.rowAtLine(last); !ok || i != 0 {
		t.Errorf("line %d (the first row's last wrapped line) hit row %d, %v; want 0", last, i, ok)
	}
	if i, ok := m.list.rowAtLine(last + 1); !ok || i != 1 {
		t.Errorf("line %d hit row %d, %v; want the second prompt", last+1, i, ok)
	}
}

// TestListSoftWrapCapsALongPrompt: one prompt longer than the window cannot run
// past the pane; it is cut to the window with an ellipsis saying there is more.
func TestListSoftWrapCapsALongPrompt(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	if err := project.add(Todo{ID: "a", Title: "long", Prompt: strings.Repeat("a line\n", 100)}); err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 60, 20
	m.listSoftWrap = true
	m.applySizes()
	it := m.list.filtered[0].item
	if got, want := 1+len(it.more), m.listWrapLines(); got != want {
		t.Errorf("a 100-line prompt drew %d lines, want the %d-line window", got, want)
	}
	if !strings.HasSuffix(it.more[len(it.more)-1], "…") {
		t.Errorf("the cut row's last line %q does not say there is more", it.more[len(it.more)-1])
	}
	if lines := strings.Split(m.viewList(), "\n"); len(lines) > m.height {
		t.Errorf("the list is %d lines in a %d-line pane", len(lines), m.height)
	}
}

// TestListSoftWrapNoHoverCard: with the wrap on, resting on a row brings up no
// card — the prompt it would show is already on screen — and a dwell armed
// before the toggle lands as nothing.
func TestListSoftWrapNoHoverCard(t *testing.T) {
	m := wrappedList(t)
	y := listRowsRow
	if m = hoverAt(t, m, 6, y); !m.hover.open {
		t.Fatal("no card with soft wrap off — the test is not resting on a row")
	}
	m.clearHover()

	next, _ := m.toggleListWrap()
	m = next.(model)
	if m = hoverAt(t, m, 6, y); m.hover.open || m.hoverPend.armed {
		t.Error("a card (or its dwell) came up over a soft-wrapped row")
	}

	// Armed while unwrapped, landing after the wrap went on.
	m.listSoftWrap = false
	next, _ = m.Update(tea.MouseMotionMsg{X: 6, Y: y})
	m = next.(model)
	if !m.hoverPend.armed {
		t.Fatal("no dwell armed with soft wrap off")
	}
	m.listSoftWrap = true
	next, _ = m.Update(hoverTickMsg{gen: m.hoverPend.gen})
	if next.(model).hover.open {
		t.Error("a dwell landing after soft wrap went on built a card")
	}
}

// TestListSoftWrapChip: the bar carries the toggle flush right, ☐ while off
// and ☑ while on, and a click on it flips the wrap — the drawn chip and the
// click spans agree after the chip moved to the edge.
func TestListSoftWrapChip(t *testing.T) {
	m := wrappedList(t)
	m.width = 200
	chips := m.actionChips()
	wrap := chips[actionWrap]
	if wrap.end != m.width {
		t.Errorf("the soft wrap chip ends at %d, want flush with the %d-cell pane", wrap.end, m.width)
	}
	bar := ansi.Strip(m.actionBar())
	if got := strings.Index(bar, "☐ Soft wrap"); got < 0 || ansi.StringWidth(bar[:got]) != wrap.start+1 {
		t.Errorf("chip drawn at %d, spans say %d (+1 padding):\n%q", ansi.StringWidth(bar[:max(got, 0)]), wrap.start, bar)
	}
	next, _ := m.clickActionBar(tea.MouseClickMsg{X: wrap.start + 2, Y: actionBarRow, Button: tea.MouseLeft})
	m = next.(model)
	if !m.listSoftWrap {
		t.Fatal("a click on the chip did not turn soft wrap on")
	}
	if !strings.Contains(m.actionBar(), "☑ Soft wrap") {
		t.Error("the chip does not show ☑ once the wrap is on")
	}
}

// TestListSoftWrapFollowsTheWidth: a resize re-wraps the rows to the new
// width, since a wrapped line cut for a wider pane would run off a narrower one.
func TestListSoftWrapFollowsTheWidth(t *testing.T) {
	m := wrappedList(t)
	m.listSoftWrap = true
	m.rebuildList()
	wide := len(m.list.filtered[0].item.more)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 30})
	m = next.(model)
	if narrow := len(m.list.filtered[0].item.more); narrow <= wide {
		t.Errorf("40 cells wrapped into %d lines, 60 into %d — the rows were not re-wrapped", narrow, wide)
	}
	for i, ln := range listRowLines(ansi.Strip(m.viewList())) {
		if ansi.StringWidth(ln) > 40 {
			t.Errorf("row line %d is %d cells in a 40-cell pane: %q", i, ansi.StringWidth(ln), ln)
		}
	}
}

// TestListWrapPromptKeepsIndent pins the wrapping rules on their own: blank
// lines dropped, a long line wrapped at the width, an indented line's wrapped
// rest hung under its own indent, tabs as four spaces.
func TestListWrapPromptKeepsIndent(t *testing.T) {
	got := listWrapPrompt("one two three four\n\n  - five six seven eight\n\tnine", 12)
	want := []string{"one two", "three four", "  - five six", "  seven", "  eight", "    nine"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("listWrapPrompt = %q, want %q", got, want)
	}
}

// listRowLines is the rows' part of a list frame: past the chrome above them
// (listRowsRow) and short of the chrome below (listChromeBelow). The narrow
// footer is not cut to the pane (the terminal clips it), so a width check
// over the whole frame would be about the footer, not the wrap.
func listRowLines(view string) []string {
	lines := strings.Split(view, "\n")
	return lines[listRowsRow:max(len(lines)-listChromeBelow, listRowsRow)]
}
