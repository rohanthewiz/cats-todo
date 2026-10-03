package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// readNextFile is the list as it is on disk now.
func readNextFile(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, nextListRel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// typeNext types s a rune at a time through Update, as a hand would.
func typeNext(t *testing.T, m model, s string) model {
	t.Helper()
	for _, r := range s {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(model)
	}
	return m
}

// TestNextCloseAsDone is the feature end to end: ctrl+t on the highlighted
// item opens the pad, the typed comment and enter move the item to the top of
// Closed with that comment, and the page — re-read — no longer lists it, with
// the highlight on the item that was below it.
func TestNextCloseAsDone(t *testing.T) {
	m, root := nextModel(t, sampleNextList)
	m = openNext(t, m)
	m = pressNext(t, m, "ctrl+t")
	if !m.nextPad.open || m.nextPad.to != nextToClosed || m.nextPad.item.ID != "N-001" {
		t.Fatalf("ctrl+t: pad = %+v, want it open to close N-001", m.nextPad)
	}
	frame := ansi.Strip(m.renderStage())
	for _, want := range []string{"✓ Close N-001 as done", "what showed it's done (optional)", "enter close as done · esc cancel"} {
		if !strings.Contains(frame, want) {
			t.Errorf("pad is missing %q:\n%s", want, frame)
		}
	}

	// The field owns printable keys: "q" is a letter, not the query box's.
	m = typeNext(t, m, "Shipped, TestX pins it")
	if q := m.next.list.input.Value(); q != "" {
		t.Errorf("typing into the pad reached the query box: %q", q)
	}
	m = pressNext(t, m, "enter")
	if m.nextPad.open {
		t.Fatal("enter left the pad up")
	}

	file := readNextFile(t, root)
	if !strings.Contains(file, "## Closed\n\n- **N-001** · closed ") ||
		!strings.Contains(file, "· raised `2026-0904-1753-a-dwell`\n  — Shipped, TestX pins it\n- **N-005**") {
		t.Errorf("N-001 not on top of Closed with the comment:\n%s", file)
	}
	if strings.Contains(sectionText(t, file, "Open"), "N-001") {
		t.Errorf("N-001 still in Open:\n%s", file)
	}
	if it, _ := m.next.highlighted(); it.ID != "N-002" {
		t.Errorf("highlight on %q, want N-002, the item below the closed one", it.ID)
	}
	for _, it := range m.next.items {
		if it.ID == "N-001" {
			t.Errorf("the page still lists N-001 — it was not re-read")
		}
	}
	if m.next.note != "N-001 closed as done" {
		t.Errorf("note = %q", m.next.note)
	}
}

// TestNextCloseMarksTheBacklogCopyDone: the work an open backlog prompt was
// made for is done, so closing the item closes its copy too.
func TestNextCloseMarksTheBacklogCopyDone(t *testing.T) {
	m, _ := nextModel(t, sampleNextList)
	m = openNext(t, m)
	if _, ok := m.addNextItem(m.next.items[0], annots{}); !ok {
		t.Fatal("could not add N-001 to the backlog")
	}
	m = pressNext(t, m, "ctrl+t")
	m = pressNext(t, m, "enter")
	if !m.project.todos[0].Done {
		t.Errorf("backlog copy of N-001 still open after closing the item")
	}
	if !strings.Contains(m.next.note, "its backlog prompt marked done") {
		t.Errorf("note = %q, want it to say the copy was marked done", m.next.note)
	}
}

// TestNextParkTogglesRoadmap: ctrl+f moves an Open item to the Roadmap in one
// press, keeping the highlight on it; the same chord brings it back, and the
// menu's row names the way it will go.
func TestNextParkTogglesRoadmap(t *testing.T) {
	m, root := nextModel(t, sampleNextList)
	m = openNext(t, m)
	m = rightClickNextAt(t, m, nextN002Y)
	if l := m.nextMenu.items[nextMenuRow(t, m, nextMenuPark)].label; l != "⇣ Move to Roadmap" {
		t.Errorf("park row on an Open item = %q", l)
	}
	m = pressNext(t, m, "esc")

	// The right-click left the highlight on N-002.
	m = pressNext(t, m, "ctrl+f")
	file := readNextFile(t, root)
	if !strings.Contains(sectionText(t, file, "Roadmap"), "- **N-002** · raised `2026-0910-1855-boot` · value high\n  Seeds ship stale prompts.\n\n- **N-003**") {
		t.Errorf("N-002 not moved verbatim to the Roadmap, in ID order:\n%s", file)
	}
	it, _ := m.next.highlighted()
	if it.ID != "N-002" || it.Section != "Roadmap" || m.next.note != "N-002 moved to the Roadmap" {
		t.Errorf("after ctrl+f: highlight %+v, note %q", it, m.next.note)
	}

	m = pressNext(t, m, "ctrl+f")
	if readNextFile(t, root) != sampleNextList {
		t.Errorf("ctrl+f twice did not restore the file:\n%s", readNextFile(t, root))
	}
	if m.next.note != "N-002 moved to Open" {
		t.Errorf("note = %q", m.next.note)
	}
}

// TestNextDeclineFromTheMenu: ⊘ Mark as non-goal… opens the pad where the menu
// was; esc moves nothing; a second go with a reason writes the Non-goals entry.
func TestNextDeclineFromTheMenu(t *testing.T) {
	m, root := nextModel(t, sampleNextList)
	m = openNext(t, m)
	m = rightClickNextAt(t, m, nextN002Y)
	bx, by := m.nextMenu.x, m.nextMenu.y
	next, _ := m.pressNextMenu(nextMenuRow(t, m, nextMenuDecline))
	m = next.(model)
	if !m.nextPad.open || m.nextPad.to != nextToNonGoals || m.nextPad.item.ID != "N-002" {
		t.Fatalf("pad = %+v, want it open to decline N-002", m.nextPad)
	}
	if !m.nextPad.anchored || m.nextPad.ax != bx || m.nextPad.ay != by {
		t.Errorf("pad anchored at (%d,%d), want the menu's cell (%d,%d)", m.nextPad.ax, m.nextPad.ay, bx, by)
	}

	m = typeNext(t, m, "nope")
	m = pressNext(t, m, "esc")
	if m.nextPad.open || readNextFile(t, root) != sampleNextList {
		t.Fatalf("esc: pad open=%v, file changed=%v; want neither", m.nextPad.open, readNextFile(t, root) != sampleNextList)
	}
	if m.next.note != "N-002 left where it was" {
		t.Errorf("note after esc = %q", m.next.note)
	}

	// Still on N-002, where the right-click put the highlight.
	m = pressNext(t, m, "ctrl+x")
	m = typeNext(t, m, "Seeds are fine")
	m = pressNext(t, m, "enter")
	want := "- **N-002** · declined "
	ng := sectionText(t, readNextFile(t, root), "Non-goals")
	if !strings.Contains(ng, want) || !strings.Contains(ng, "· raised `2026-0910-1855-boot`\n  — Seeds are fine\n") {
		t.Errorf("Non-goals:\n%s", ng)
	}
	if m.next.note != "N-002 marked a non-goal" {
		t.Errorf("note = %q", m.next.note)
	}
}

// TestNextPadClickOffDismisses: a click off the pad is "never mind" — nothing
// moves, and the page under it does not take the click.
func TestNextPadClickOffDismisses(t *testing.T) {
	m, root := nextModel(t, sampleNextList)
	m = openNext(t, m)
	m = pressNext(t, m, "ctrl+t")
	next, _ := m.Update(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	m = next.(model)
	if m.nextPad.open || readNextFile(t, root) != sampleNextList {
		t.Errorf("click off the pad: open=%v, file changed=%v", m.nextPad.open, readNextFile(t, root) != sampleNextList)
	}
	// ctrl+c takes the pad down rather than quitting.
	m = pressNext(t, m, "ctrl+t")
	next, _ = m.Update(pressKey("ctrl+c"))
	m = next.(model)
	if m.quitting || m.nextPad.open {
		t.Errorf("ctrl+c in the pad: quitting=%v open=%v, want the pad closed and the program running", m.quitting, m.nextPad.open)
	}
}

// TestNextMoveRefusedWhenMovedElsewhere: another pane closed the item after
// the page loaded; the move refuses in words and writes nothing.
func TestNextMoveRefusedWhenMovedElsewhere(t *testing.T) {
	m, root := nextModel(t, sampleNextList)
	m = openNext(t, m)
	changed := strings.Replace(sampleNextList, "## Open\n\n- **N-001**", "## Open\n\n- **N-901**", 1)
	writeNextList(t, root, changed)
	m = pressNext(t, m, "ctrl+f")
	if !m.next.noteErr || !strings.Contains(m.next.note, "N-001 is no longer Open") {
		t.Errorf("note = %q (err %v), want a refusal", m.next.note, m.next.noteErr)
	}
	if readNextFile(t, root) != changed {
		t.Errorf("a refused move wrote the file")
	}
}
