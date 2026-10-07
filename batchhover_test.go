package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// pickRowY is the screen line of the Pick pane row titled title.
func pickRowY(t *testing.T, m model, title string) int {
	t.Helper()
	g := m.batchGeom()
	for y := g.pickRowsY; y < g.barY-1; y++ {
		i, ok := m.batch.pick.rowAtLine(y - g.pickRowsY)
		if !ok {
			continue
		}
		if idx, ok := m.batch.pick.refAt(i); ok && m.batch.cands[idx].title == title {
			return y
		}
	}
	t.Fatalf("no Pick row titled %q on screen", title)
	return -1
}

// TestComposerHoverCardShowsThePrompt is the feature: resting on a Pick row's
// title floats the backlog card for that prompt, drawn over the composer.
func TestComposerHoverCardShowsThePrompt(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	m = openComposer(t, m)
	if got := m.View().MouseMode; got != tea.MouseModeAllMotion {
		t.Fatalf("composer MouseMode = %v, want all motion so idle hover reports", got)
	}
	y := pickRowY(t, m, "Rename headings")
	m = hoverAt(t, m, 12, y)
	if !m.hover.open {
		t.Fatal("no card after resting on a Pick row's title")
	}
	if got := cardText(m); !strings.Contains(got, "do: Rename headings") {
		t.Errorf("card does not show the prompt:\n%s", got)
	}
	if !strings.Contains(ansi.Strip(m.renderStage()), "do: Rename headings") {
		t.Error("the card is built but not drawn over the composer")
	}
}

// TestComposerHoverSkipsTheCheckbox: the [ ]/[x] cells are the control the
// hand is reaching for, so resting there brings no card — and moving onto the
// box takes down the card the title had opened.
func TestComposerHoverSkipsTheCheckbox(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	m = openComposer(t, m)
	y := pickRowY(t, m, "Rename headings")
	box := lipgloss.Width(checkOff)
	for x := indentWidth; x < indentWidth+box; x++ {
		got := hoverAt(t, m, x, y)
		if got.hover.open || got.hoverPend.armed {
			t.Errorf("x=%d (on the checkbox) brought a card", x)
		}
	}
	// Either side of the box still has one: the cursor's gutter and the
	// space before the title belong to the row.
	for _, x := range []int{0, indentWidth + box} {
		if !hoverAt(t, m, x, y).hover.open {
			t.Errorf("x=%d (beside the checkbox) brought no card", x)
		}
	}

	m = hoverAt(t, m, 12, y)
	if !m.hover.open {
		t.Fatal("no card to begin with")
	}
	next, _ := m.Update(tea.MouseMotionMsg{X: indentWidth + 1, Y: y})
	if next.(model).hover.open {
		t.Error("the card stayed up with the pointer on the checkbox")
	}
}

// TestComposerHoverYieldsToTheHand: a key, a click, the drop dialog and the
// other pane all take the card down.
func TestComposerHoverYieldsToTheHand(t *testing.T) {
	base, _, _ := batchModel(t, 120, 30)
	base = openComposer(t, base)
	y := pickRowY(t, base, "Rename headings")

	t.Run("a keystroke", func(t *testing.T) {
		m := hoverAt(t, base, 12, y)
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		if next.(model).hover.open {
			t.Error("the card survived a keystroke")
		}
	})
	t.Run("a click", func(t *testing.T) {
		m := hoverAt(t, base, 12, y)
		next, _ := m.Update(tea.MouseClickMsg{X: 12, Y: y, Button: tea.MouseLeft})
		if next.(model).hover.open {
			t.Error("the card survived a click")
		}
	})
	t.Run("the Batch pane", func(t *testing.T) {
		m := hoverAt(t, base, 12, y)
		g := m.batchGeom()
		next, _ := m.Update(tea.MouseMotionMsg{X: g.batchX + 4, Y: y})
		if next.(model).hover.open {
			t.Error("the card survived the pointer crossing to the Batch pane")
		}
	})
	t.Run("the drop dialog", func(t *testing.T) {
		m := base
		m.batch.confirm.open = true
		if m = hoverAt(t, m, 12, y); m.hover.open {
			t.Error("a card was built under the drop dialog")
		}
	})
}

// TestComposerHoverDwellChecksTheBox: a dwell armed on a title whose pointer
// is, by the time the tick lands, on a checkbox (the pane re-laid out under a
// still pointer) builds nothing.
func TestComposerHoverDwellChecksTheBox(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	m = openComposer(t, m)
	y := pickRowY(t, m, "Rename headings")
	next, _ := m.Update(tea.MouseMotionMsg{X: 12, Y: y})
	m = next.(model)
	if !m.hoverPend.armed {
		t.Fatal("no dwell armed on the title")
	}
	m.hoverPend.x = indentWidth
	next, _ = m.Update(hoverTickMsg{gen: m.hoverPend.gen})
	if next.(model).hover.open {
		t.Error("a dwell landing on the checkbox built a card")
	}
}
