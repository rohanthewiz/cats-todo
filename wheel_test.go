package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	wheelDown = tea.MouseWheelMsg{Button: tea.MouseWheelDown}
	wheelUp   = tea.MouseWheelMsg{Button: tea.MouseWheelUp}
)

// wrappedBacklog is a soft-wrapped backlog far taller than its pane, split
// across both scopes (so the Project/Global headings show) with open, frozen
// and done rows in each, so the window has headings and spacers to scroll past.
func wrappedBacklog(t *testing.T) model {
	t.Helper()
	m, project, global := newModelInTemp(t)
	for i := range 24 {
		td := Todo{
			ID:     fmt.Sprintf("t%02d", i),
			Title:  fmt.Sprintf("Title %02d", i),
			Prompt: strings.Repeat(fmt.Sprintf("word%02d ", i), 20) + "\nsecond line",
		}
		switch i % 6 {
		case 4:
			td.Frozen = true
		case 5:
			td.Done = true
		}
		st := project
		if i >= 16 {
			st = global
		}
		if err := st.add(td); err != nil {
			t.Fatal(err)
		}
	}
	m.width, m.height = 70, 24
	m.listSoftWrap = true
	m.showFrozen = true
	m.applySizes()
	m.rebuildList()
	headings := 0
	for _, s := range m.list.filtered {
		if !s.item.selectable && s.item.name != "" {
			headings++
		}
	}
	if headings < 2 {
		t.Fatalf("the fixture has %d group headings, want Project and Global", headings)
	}
	if m.list.maxLines == 0 || m.list.maxTop() == 0 {
		t.Fatalf("the fixture does not overflow its window (maxLines=%d, maxTop=%d)", m.list.maxLines, m.list.maxTop())
	}
	return m
}

// assertCursorDrawn fails when the highlighted row is outside the window — the
// one invariant the renderers trust every mutator to keep.
func assertCursorDrawn(t *testing.T, l fuzzyList, when string) {
	t.Helper()
	lo, hi := l.window()
	if l.cursor < lo || l.cursor >= hi || !l.filtered[l.cursor].item.selectable {
		t.Errorf("%s: cursor %d is not a drawn row of the window [%d,%d)", when, l.cursor, lo, hi)
	}
}

// TestWheelScrollsTheWrappedList is the bug: with ☑ Soft wrap on, the list
// outgrows the pane and the wheel — handed to the program as mouse events,
// since the list reports the mouse — did nothing at all. Now each notch moves
// the window a row, the highlight is dragged along only when it would leave
// the screen, and the window walks the whole list to its last page and back.
func TestWheelScrollsTheWrappedList(t *testing.T) {
	m := wrappedBacklog(t)
	last := m.list.maxTop()
	prev := m.list.top
	// Walking down must pass every group heading. A step that landed the
	// highlight on the window's top row would, through ensureVisible's
	// heading rule, be pulled back onto the heading above it, and the wheel
	// would stick there forever — so every notch has to make progress.
	for i := 0; m.list.top < last; i++ {
		if i > len(m.list.filtered) {
			t.Fatalf("the wheel stopped at top=%d short of the last page (%d)", m.list.top, last)
		}
		next, _ := m.Update(wheelDown)
		m = next.(model)
		if m.list.top <= prev {
			t.Fatalf("notch %d: top %d did not move past %d", i, m.list.top, prev)
		}
		prev = m.list.top
		assertCursorDrawn(t, m.list, fmt.Sprintf("down notch %d", i))
		if lines := strings.Split(m.viewList(), "\n"); len(lines) > m.height {
			t.Errorf("down notch %d: the list is %d lines in a %d-line pane", i, len(lines), m.height)
		}
	}
	// At the end it stays put, rather than scrolling into blank lines.
	next, _ := m.Update(wheelDown)
	if m = next.(model); m.list.top != last {
		t.Errorf("past the end: top = %d, want %d", m.list.top, last)
	}
	// The last prompt is on screen.
	lastName := m.list.filtered[len(m.list.filtered)-1].item.name
	if !strings.Contains(ansi.Strip(m.viewList()), lastName) {
		t.Errorf("at the last page %q is not drawn", lastName)
	}
	for i := 0; m.list.top > 0; i++ {
		if i > len(m.list.filtered) {
			t.Fatalf("the wheel stopped at top=%d on the way up", m.list.top)
		}
		next, _ := m.Update(wheelUp)
		m = next.(model)
		assertCursorDrawn(t, m.list, fmt.Sprintf("up notch %d", i))
	}
}

// TestWheelMovesThePageNotTheHighlight: while the highlighted row stays on
// screen, a notch leaves it where it is — the reading gesture — and the next
// key still walks on from it.
func TestWheelMovesThePageNotTheHighlight(t *testing.T) {
	m := wrappedBacklog(t)
	// Put the highlight a few rows in, then scroll one notch: the window
	// moves under it, the cursor does not.
	for range 3 {
		m = pressList(t, m, "down")
	}
	cur, top := m.list.cursor, m.list.top
	next, _ := m.Update(wheelDown)
	m = next.(model)
	if m.list.top == top {
		t.Fatal("the notch did not scroll")
	}
	if lo, _ := m.list.window(); cur >= lo && m.list.cursor != cur {
		t.Errorf("the highlight moved from %d to %d though it was still drawn", cur, m.list.cursor)
	}
}

// TestWheelIgnoredUnderTheListMenu: an open context menu is anchored to its
// row, so the wheel must not scroll the rows out from under it.
func TestWheelIgnoredUnderTheListMenu(t *testing.T) {
	m := wrappedBacklog(t)
	m.listMenu.open = true
	next, _ := m.Update(wheelDown)
	if m = next.(model); m.list.top != 0 {
		t.Errorf("the wheel scrolled the list under an open menu: top = %d", m.list.top)
	}
}

// TestWheelScrollsTheWrappedNextList: the same bug and fix on the Next List
// page, across its Open → Roadmap heading.
func TestWheelScrollsTheWrappedNextList(t *testing.T) {
	var b strings.Builder
	b.WriteString("# Next list\n\n## Open\n\n")
	for i := 1; i <= 16; i++ {
		if i == 11 {
			b.WriteString("## Roadmap\n\n")
		}
		fmt.Fprintf(&b, "- **N-%03d** · raised `x` · value low\n  %s\n\n", i, strings.Repeat(fmt.Sprintf("item%02d text ", i), 12))
	}
	m, _ := nextModel(t, b.String())
	m.width, m.height = 60, 24
	m.nextSoftWrap = true
	m.applySizes()
	m = openNext(t, m)
	last := m.next.list.maxTop()
	if m.next.list.maxLines == 0 || last == 0 {
		t.Fatalf("the page does not overflow (maxLines=%d, maxTop=%d)", m.next.list.maxLines, last)
	}
	for i := 0; m.next.list.top < last; i++ {
		if i > len(m.next.list.filtered) {
			t.Fatalf("the wheel stopped at top=%d short of %d", m.next.list.top, last)
		}
		next, _ := m.Update(wheelDown)
		m = next.(model)
		assertCursorDrawn(t, m.next.list, fmt.Sprintf("down notch %d", i))
	}
	if !strings.Contains(ansi.Strip(m.viewNextList()), "N-016") {
		t.Errorf("at the last page N-016 is not drawn:\n%s", ansi.Strip(m.viewNextList()))
	}
	for i := 0; m.next.list.top > 0; i++ {
		if i > len(m.next.list.filtered) {
			t.Fatalf("the wheel stopped at top=%d on the way up", m.next.list.top)
		}
		next, _ := m.Update(wheelUp)
		m = next.(model)
	}
	if !strings.Contains(ansi.Strip(m.viewNextList()), "N-001") {
		t.Error("back at the top N-001 is not drawn")
	}
}

// TestMaxTopIsTheLastPage pins maxTop to what ensureVisibleLines' clamp leaves
// when the cursor is on the last row: the window that ends on the list's end.
func TestMaxTopIsTheLastPage(t *testing.T) {
	m := wrappedBacklog(t)
	for range len(m.list.filtered) {
		m = pressList(t, m, "down")
	}
	if got := m.list.maxTop(); got != m.list.top {
		t.Errorf("maxTop = %d, but walking to the end leaves top at %d", got, m.list.top)
	}
}

// TestScrollByItemWindow is scrollBy on the item-counted window every other
// windowed list uses (maxRows): the window walks to len-maxRows and stops, the
// highlight is dragged along only when it falls off an edge, and up brings it
// back the same way.
func TestScrollByItemWindow(t *testing.T) {
	var items []listItem
	for i := range 20 {
		items = append(items, listItem{name: fmt.Sprintf("row %02d", i), selectable: true, ref: i})
	}
	l := newFuzzyList("", items)
	l.setMaxRows(5)
	if !l.scrollBy(1) || l.top != 1 || l.cursor != 1 {
		t.Fatalf("one notch down: top=%d cursor=%d, want 1 and 1 (dragged off row 0)", l.top, l.cursor)
	}
	for range 30 {
		l.scrollBy(1)
	}
	if l.top != 15 || l.cursor != 15 {
		t.Errorf("at the end: top=%d cursor=%d, want 15 and 15", l.top, l.cursor)
	}
	if l.scrollBy(1) {
		t.Error("scrollBy past the end reported a move")
	}
	l.cursor = 17 // a row inside the window: a notch up leaves it alone
	l.scrollBy(-1)
	if l.top != 14 || l.cursor != 17 {
		t.Errorf("one notch up: top=%d cursor=%d, want 14 and 17", l.top, l.cursor)
	}
	for range 30 {
		l.scrollBy(-1)
	}
	if l.top != 0 || l.cursor != 4 {
		t.Errorf("back at the top: top=%d cursor=%d, want 0 and 4 (dragged off the bottom)", l.top, l.cursor)
	}
	var none fuzzyList
	if none.scrollBy(1) {
		t.Error("a list with no window scrolled")
	}
}

// wheelTo sends n notches through Update (down for n > 0) at column x.
func wheelTo(m model, n, x int) model {
	ev := wheelDown
	if n < 0 {
		ev, n = wheelUp, -n
	}
	ev.X = x
	for range n {
		next, _ := m.Update(ev)
		m = next.(model)
	}
	return m
}

// TestWheelScrollsTheBatchesPage: the Batches page, longer than its pane.
func TestWheelScrollsTheBatchesPage(t *testing.T) {
	now := time.Now()
	var bs []Batch
	for i := range 40 {
		bs = append(bs, Batch{ID: fmt.Sprintf("b%02d", i), State: batchDone, Dropped: now.Add(-time.Duration(i) * time.Minute)})
	}
	m := batchesPageWith(t, bs...)
	if m.batches.list.maxTop() == 0 {
		t.Fatal("the page does not overflow its window")
	}
	m = wheelTo(m, 3, 10)
	if m.batches.list.top != 3 {
		t.Errorf("three notches: top = %d, want 3", m.batches.list.top)
	}
	assertCursorDrawn(t, m.batches.list, "batches page")
	// Under the open menu the rows stay put.
	m = rightClickBatch(t, m, m.batches.rows[m.batches.list.filtered[m.batches.list.cursor].item.ref].ID)
	if m = wheelTo(m, 1, 10); m.batches.list.top != 3 {
		t.Errorf("the wheel scrolled the page under its menu: top = %d", m.batches.list.top)
	}
}

// TestWheelScrollsTheComposerPaneUnderThePointer: the composer's two panes
// scroll apart, each taking the wheel only while the pointer is over it.
func TestWheelScrollsTheComposerPaneUnderThePointer(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	var titles []string
	for i := range 40 {
		title := fmt.Sprintf("Prompt %02d", i)
		titles = append(titles, title)
		if err := project.add(Todo{ID: fmt.Sprintf("p%02d", i), Title: title, Prompt: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	m.rebuildList()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m = composerWith(t, next.(model), titles[:30]...)
	g := m.batchGeom()
	if !g.split {
		t.Fatal("the composer is not split at 140 columns")
	}
	if m.batch.pick.maxTop() == 0 || len(m.batch.picked) <= g.batchRows {
		t.Fatalf("neither pane overflows (pick maxTop=%d, %d picked in %d rows)", m.batch.pick.maxTop(), len(m.batch.picked), g.batchRows)
	}
	pickTop, batchTop := m.batch.pick.top, m.batch.top
	m = wheelTo(m, 2, 2)
	if m.batch.pick.top != pickTop+2 || m.batch.top != batchTop {
		t.Errorf("over the Pick pane: pick top %d→%d, batch top %d→%d; want only the pick to move 2", pickTop, m.batch.pick.top, batchTop, m.batch.top)
	}
	assertCursorDrawn(t, m.batch.pick, "pick pane")
	m.batch.cursor, m.batch.top = 0, 0
	m = wheelTo(m, 2, g.batchX+2)
	if m.batch.top != 2 || m.batch.pick.top != pickTop+2 {
		t.Errorf("over the Batch pane: batch top = %d, pick top = %d; want 2 and %d", m.batch.top, m.batch.pick.top, pickTop+2)
	}
	if m.batch.cursor != 2 {
		t.Errorf("the Batch pane's highlight was not dragged onto its window: cursor = %d", m.batch.cursor)
	}
	m = wheelTo(m, 100, g.batchX+2)
	if want := len(m.batch.picked) - g.batchRows; m.batch.top != want {
		t.Errorf("at the end of the Batch pane: top = %d, want %d", m.batch.top, want)
	}
}

// TestWheelScrollsTheFilePicker: the editor's @ picker over a folder longer
// than its window.
func TestWheelScrollsTheFilePicker(t *testing.T) {
	root := t.TempDir()
	for i := range 60 {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("f%02d.go", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := withForm(t, "", "", 100, 30)
	m.ctx.WorkDir = root
	m = typeInForm(t, m, pressKey("@"))
	if m.stage != stageFiles {
		t.Fatalf("@ left stage = %v, want the file picker", m.stage)
	}
	if m.files.list.maxTop() == 0 {
		t.Fatal("the picker does not overflow its window")
	}
	if m = wheelTo(m, 4, 10); m.files.list.top != 4 {
		t.Errorf("four notches: top = %d, want 4", m.files.list.top)
	}
	assertCursorDrawn(t, m.files.list, "file picker")
}

// TestWheelScrollsTheSnippetsPicker: the prompt library, longer than its
// window.
func TestWheelScrollsTheSnippetsPicker(t *testing.T) {
	var entries []string
	for i := range 60 {
		entries = append(entries, fmt.Sprintf(`{"name":"snippet %02d","body":"text %02d"}`, i, i))
	}
	m := formWithLibrary(t, "", `{"prompts":[`+strings.Join(entries, ",")+`]}`)
	m = typeInForm(t, m, pressKey("ctrl+p"))
	if m.stage != stageSnippets {
		t.Fatalf("ctrl+p left stage = %v, want the snippets picker", m.stage)
	}
	if m.snips.list.maxTop() == 0 {
		t.Fatal("the picker does not overflow its window")
	}
	if m = wheelTo(m, 4, 10); m.snips.list.top != 4 {
		t.Errorf("four notches: top = %d, want 4", m.snips.list.top)
	}
	assertCursorDrawn(t, m.snips.list, "snippets picker")
}
