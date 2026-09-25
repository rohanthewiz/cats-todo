package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// hoverNext rests the pointer on the Next List page's screen line y and lets
// the dwell elapse (see hoverAt for why the tick is delivered directly).
func hoverNext(t *testing.T, m model, y int) model {
	t.Helper()
	next, _ := m.Update(tea.MouseMotionMsg{X: 6, Y: y})
	m = next.(model)
	if !m.hoverPend.armed {
		return m
	}
	next, _ = m.Update(hoverTickMsg{gen: m.hoverPend.gen})
	return next.(model)
}

// TestNextHoverCardShowsTheItem is the feature: resting on a row brings up the
// item's ID and section, its text with the sub-bullets the row flattened, and
// the header fields the row never draws.
func TestNextHoverCardShowsTheItem(t *testing.T) {
	m, _ := nextModel(t, sampleNextList)
	m = openNext(t, m)
	// The rows block opens with the Open section's spacer and heading, so the
	// first item is two lines below nextRowsRow.
	y := nextRowsRow + 2
	if i, ok := m.next.list.rowAtLine(y - nextRowsRow); !ok || m.next.list.filtered[i].item.badge != "N-001" {
		t.Fatalf("screen line %d is not N-001's row", y)
	}
	m = hoverNext(t, m, y)
	if !m.hover.open {
		t.Fatal("no card after resting on an item")
	}
	got := cardText(m)
	for _, want := range []string{
		"N-001 · Open",
		"Hands-on pass in a rebuilt Cats.app.",
		"- hover cards: the 400ms dwell;", // its own line, not flattened
		"value medium · raised 2026-0904-1753-a-dwell",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("card missing %q:\n%s", want, got)
		}
	}
	// And it is drawn: the overlay is composited onto the page's frame.
	if !strings.Contains(ansi.Strip(m.renderStage()), "N-001 · Open") {
		t.Error("the card is built but not drawn over the page")
	}
}

// TestNextHoverCardIsCapped pins the seven-row budget: a long item fills five
// text rows and ends in the ellipsis that says there is more, and the card
// never grows past nextCardMaxRows however much the item says.
func TestNextHoverCardIsCapped(t *testing.T) {
	long := nextItem{
		ID: "N-009", Section: "Open", Value: "high", Raised: "2026-0920-0000-x",
		Text: strings.Repeat("a sentence that goes on and on ", 40) + "\n- one\n- two\n- three",
	}
	lines := nextCardLines(long, 40)
	if len(lines) != nextCardMaxRows {
		t.Fatalf("card is %d rows, want the cap of %d", len(lines), nextCardMaxRows)
	}
	body := strings.TrimSpace(ansi.Strip(lines[nextCardMaxRows-2]))
	if !strings.HasSuffix(body, "…") {
		t.Errorf("last text row %q does not say there is more", body)
	}
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != 42 {
			t.Errorf("row %d is %d cells, want the full interior of 42 (an opaque box)", i, w)
		}
	}

	// A short item with no fields spends two rows, not seven.
	short := nextCardLines(nextItem{ID: "N-010", Text: "tiny"}, 40)
	if len(short) != 2 {
		t.Errorf("a short, field-less item got %d rows, want 2", len(short))
	}
}

// TestNextHoverCardYieldsToTheHand: the page's keys and clicks take the card
// down, as the list's do, and they are also every way off the page — so a
// card can never be left standing over a screen it does not describe.
func TestNextHoverCardYieldsToTheHand(t *testing.T) {
	base, _ := nextModel(t, sampleNextList)
	base = openNext(t, base)
	y := nextRowsRow + 2

	t.Run("a keystroke", func(t *testing.T) {
		m := hoverNext(t, base, y)
		if !m.hover.open {
			t.Fatal("no card to begin with")
		}
		if m = pressNext(t, m, "down"); m.hover.open {
			t.Error("the card survived a keystroke on the page")
		}
	})
	t.Run("a click", func(t *testing.T) {
		m := hoverNext(t, base, y)
		next, _ := m.Update(tea.MouseClickMsg{X: 6, Y: y, Button: tea.MouseLeft})
		if next.(model).hover.open {
			t.Error("the card survived a click")
		}
	})
	t.Run("off the rows", func(t *testing.T) {
		m := hoverNext(t, base, y)
		next, _ := m.Update(tea.MouseMotionMsg{X: 6, Y: nextBarRow})
		if next.(model).hover.open {
			t.Error("the card survived the pointer moving onto the bar")
		}
	})
}

// TestNextHoverDwellIsStageBound: a dwell armed on the backlog list that lands
// after ctrl+g opened the page must not build a card from the list's row index
// against the page's rows — the two share the hover state, and hoverPending's
// stage is what keeps them apart.
func TestNextHoverDwellIsStageBound(t *testing.T) {
	m, _ := nextModel(t, sampleNextList)
	m.hoverGen++
	m.hoverPend = hoverPending{armed: true, stage: stageList, row: 1, x: 6, y: nextRowsRow + 2, gen: m.hoverGen}
	m.stage = stageNextList
	m.next = newNextPage(m.nextListRoot())
	m.next.resize(m.width, m.height)
	next, _ := m.Update(hoverTickMsg{gen: m.hoverGen})
	if next.(model).hover.open {
		t.Error("a list-armed dwell built a card on the Next List page")
	}
}

// TestNextListAsksForAllMotion: the card is built from idle motion, which the
// terminal only reports under MouseModeAllMotion.
func TestNextListAsksForAllMotion(t *testing.T) {
	m, _ := nextModel(t, sampleNextList)
	m = openNext(t, m)
	if got := m.View().MouseMode; got != tea.MouseModeAllMotion {
		t.Errorf("Next List mouse mode = %v, want all motion", got)
	}
}
