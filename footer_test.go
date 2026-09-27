package main

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestFooterRowsWrapBeforeConceding pins the two-line footer: segments fill
// the first line, then the second, and only a tail that fits neither is cut —
// in order, so a short late segment never jumps a longer earlier one.
func TestFooterRowsWrapBeforeConceding(t *testing.T) {
	m := newTestModel()
	segs := []string{"aaaa aaaa", "bbbb bbbb", "cccc cccc", "dddd dddd", "e"}

	m.width = 200
	if rows := m.footerRows(segs); len(rows) != 1 {
		t.Fatalf("a wide pane should read one line, got %q", rows)
	}

	// 24 cells: "aaaa aaaa · bbbb bbbb" is 21, a third segment would be 33.
	m.width = 24
	rows := m.footerRows(segs)
	want := []string{"aaaa aaaa · bbbb bbbb", "cccc cccc · dddd dddd"}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Fatalf("rows = %q, want %q (e is the tail and concedes)", rows, want)
	}
	for _, r := range rows {
		if lipgloss.Width(r) > m.width {
			t.Errorf("row %q is wider than the %d-cell pane", r, m.width)
		}
	}

	// The first segment survives a pane narrower than itself.
	m.width = 4
	if rows := m.footerRows(segs); len(rows) != 1 || rows[0] != "aaaa aaaa" {
		t.Errorf("narrow pane rows = %q, want only the first segment", rows)
	}

	// An unknown width keeps everything on one line.
	m.width = 0
	if rows := m.footerRows(segs); len(rows) != 1 || !strings.HasSuffix(rows[0], "· e") {
		t.Errorf("unsized rows = %q, want every segment on one line", rows)
	}
}

// TestComposerFrameHoldsTwoFooterLines: the composer draws a fixed frame by
// row index (batchGeom), so the footer's second line has to be a row of the
// frame, not an extra line past the pane's bottom.
func TestComposerFrameHoldsTwoFooterLines(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 60, 30
	m.stage = stageBatchCompose
	g := m.batchGeom()
	if g.footY != m.height-footerRowCap || g.noteY != g.footY-1 || g.barY != g.footY-2 {
		t.Fatalf("geometry bar/note/foot = %d/%d/%d in a %d-row pane", g.barY, g.noteY, g.footY, m.height)
	}
	if n := strings.Count(m.viewBatchCompose(), "\n") + 1; n != m.height {
		t.Errorf("composer frame is %d lines, want the pane's %d", n, m.height)
	}
}
