package main

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// sampleItems is two groups of selectable rows separated by non-selectable
// headings — the shape the todo list and target picker both produce.
func sampleItems() []listItem {
	return []listItem{
		{name: "Project", selectable: false},
		{name: "alpha task", desc: "do alpha", selectable: true, ref: 10},
		{name: "beta task", desc: "do beta", selectable: true, ref: 11},
		{name: "Global", selectable: false},
		{name: "gamma task", desc: "do gamma", selectable: true, ref: 12},
	}
}

// setQuery types a query into the list and re-filters, mirroring editQuery
// without needing a tea.Msg.
func setQuery(l *fuzzyList, q string) {
	l.input.SetValue(q)
	l.filter()
}

func TestFilterEmptyQueryShowsEverything(t *testing.T) {
	l := newFuzzyList("", sampleItems())
	// An empty query keeps every row, separators included, in natural order.
	if len(l.filtered) != 5 {
		t.Fatalf("empty-query filtered = %d rows, want 5 (all incl. separators)", len(l.filtered))
	}
	// The cursor must land on the first selectable row, skipping the "Project"
	// heading at index 0.
	if l.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (first selectable, past the heading)", l.cursor)
	}
	if got := l.selectedIndex(); got != 10 {
		t.Errorf("selectedIndex = %d, want ref 10", got)
	}
}

func TestFilterExcludesSeparatorsAndNonMatches(t *testing.T) {
	l := newFuzzyList("", sampleItems())
	setQuery(&l, "alpha")
	if len(l.filtered) != 1 {
		t.Fatalf("query 'alpha' filtered = %d rows, want 1", len(l.filtered))
	}
	if l.filtered[0].item.ref != 10 {
		t.Errorf("matched ref = %d, want 10", l.filtered[0].item.ref)
	}
	if got := l.selectedIndex(); got != 10 {
		t.Errorf("selectedIndex after filter = %d, want 10", got)
	}
}

func TestFilterMatchesDescriptionToo(t *testing.T) {
	// "do gamma" lives in the description; filter searches name+desc together.
	l := newFuzzyList("", sampleItems())
	setQuery(&l, "gamma")
	if len(l.filtered) != 1 || l.filtered[0].item.ref != 12 {
		t.Fatalf("query 'gamma' filtered = %+v, want only ref 12", l.filtered)
	}
}

func TestFilterNoMatchesGivesNoSelection(t *testing.T) {
	l := newFuzzyList("", sampleItems())
	setQuery(&l, "zzzznotpresent")
	if len(l.filtered) != 0 {
		t.Fatalf("no-match query filtered = %d rows, want 0", len(l.filtered))
	}
	if got := l.selectedIndex(); got != -1 {
		t.Errorf("selectedIndex with no matches = %d, want -1", got)
	}
}

// TestMatchedIndexesStayWithinName guards highlighting: only name characters
// should be reported as matched, never characters that matched in the
// description (which is appended to the haystack but not highlighted).
func TestMatchedIndexesStayWithinName(t *testing.T) {
	// name "abc" (3 runes) + "  " + desc "xyz"; query "ax" matches 'a' at name
	// index 0 and 'x' at haystack index 5 (inside the desc). Only index 0 should
	// survive as an in-name match.
	l := newFuzzyList("", []listItem{{name: "abc", desc: "xyz", selectable: true, ref: 0}})
	setQuery(&l, "ax")
	if len(l.filtered) != 1 {
		t.Fatalf("filtered = %d rows, want 1", len(l.filtered))
	}
	got := l.filtered[0].matched
	if len(got) != 1 || got[0] != 0 {
		t.Errorf("matched = %v, want [0] (desc matches must be dropped)", got)
	}
}

func TestMoveDownAndUpSkipSeparators(t *testing.T) {
	l := newFuzzyList("", sampleItems())
	// Start parked on the first selectable (index 1, ref 10).
	if l.cursor != 1 {
		t.Fatalf("setup: cursor = %d, want 1", l.cursor)
	}

	l.moveDown() // -> index 2, ref 11
	if l.selectedIndex() != 11 {
		t.Errorf("after one moveDown selectedIndex = %d, want 11", l.selectedIndex())
	}
	l.moveDown() // skips the "Global" heading at index 3 -> index 4, ref 12
	if l.selectedIndex() != 12 {
		t.Errorf("after two moveDowns selectedIndex = %d, want 12", l.selectedIndex())
	}
	l.moveDown() // already on the last selectable: stays put
	if l.selectedIndex() != 12 {
		t.Errorf("moveDown past the end moved off the last row: %d", l.selectedIndex())
	}

	l.moveUp() // skips the heading going back up -> index 2, ref 11
	if l.selectedIndex() != 11 {
		t.Errorf("after moveUp selectedIndex = %d, want 11", l.selectedIndex())
	}
	l.moveUp() // -> index 1, ref 10
	l.moveUp() // already first selectable: stays put
	if l.selectedIndex() != 10 {
		t.Errorf("moveUp past the start selectedIndex = %d, want 10", l.selectedIndex())
	}
}

func TestSetItemsKeepsQueryAndClampsCursor(t *testing.T) {
	l := newFuzzyList("", sampleItems())
	setQuery(&l, "task") // matches all three selectable rows
	l.moveDown()
	l.moveDown() // cursor now on the third match

	// Replace with a shorter list; the query persists and the cursor must clamp
	// back into range and onto a selectable row.
	l.setItems([]listItem{{name: "alpha task", desc: "", selectable: true, ref: 99}})
	if l.input.Value() != "task" {
		t.Errorf("setItems dropped the query: %q", l.input.Value())
	}
	if got := l.selectedIndex(); got != 99 {
		t.Errorf("after setItems selectedIndex = %d, want 99", got)
	}
}

func TestSelectedIndexAllSeparators(t *testing.T) {
	// A list with no selectable rows (only headings) has nothing to select.
	l := newFuzzyList("", []listItem{{name: "Heading", selectable: false}})
	if got := l.selectedIndex(); got != -1 {
		t.Errorf("selectedIndex with no selectable rows = %d, want -1", got)
	}
}

// TestViewComposesRowsView pins the split between view and rowsView: view is
// the boxed query line stacked on exactly what rowsView renders, so the two
// callers — the picker through view, the manager's list straight into
// rowsView — can never drift apart on what the rows look like.
func TestViewComposesRowsView(t *testing.T) {
	l := newFuzzyList("filter…", sampleItems())
	const width = 40

	v := l.view("nothing here", "", width)
	rows := l.rowsView("nothing here", width)
	if !strings.HasSuffix(v, rows) {
		t.Fatalf("view does not end with rowsView's output\nview:\n%q\nrows:\n%q", v, rows)
	}
	if !strings.Contains(v, "🔍") {
		t.Fatal("view lost the boxed query line the picker depends on")
	}
	if strings.Contains(rows, "🔍") {
		t.Fatal("rowsView must not render a query line — the manager draws its own")
	}
}

// flatItems is n plain selectable rows, "row 0" … "row n-1", ref = index — a
// list with no headings, which is what the file picker builds.
func flatItems(n int) []listItem {
	items := make([]listItem, n)
	for i := range items {
		items[i] = listItem{name: "row " + strings.Repeat("x", i%3) + string(rune('a'+i)), selectable: true, ref: i}
	}
	return items
}

// drawnRows counts the "row …" lines in a rendered rows block.
func drawnRows(view string) int {
	return strings.Count(view, "row ")
}

func TestWindowFollowsTheCursor(t *testing.T) {
	l := newFuzzyList("", flatItems(10))
	l.setMaxRows(3)
	if l.top != 0 {
		t.Fatalf("top = %d at the start, want 0", l.top)
	}
	for range 5 {
		l.moveDown()
	}
	if l.cursor != 5 || l.top != 3 {
		t.Errorf("after 5 moves down: cursor %d top %d, want 5 and 3", l.cursor, l.top)
	}
	if n := drawnRows(l.rowsView("none", 40)); n != 3 {
		t.Errorf("windowed rowsView drew %d rows, want 3", n)
	}
	// The overflow markers say what lies each way, and both directions are
	// reported: the window is in the middle of the list, so there are three
	// rows above it as well as four below.
	v := l.rowsView("none", 40)
	if !strings.Contains(v, overflowDownGlyph+" 4") {
		t.Errorf("rowsView does not say what is below the fold:\n%s", v)
	}
	if !strings.Contains(v, overflowUpGlyph+" 3") {
		t.Errorf("rowsView does not say what is above the fold:\n%s", v)
	}
	for range 5 {
		l.moveUp()
	}
	if l.cursor != 0 || l.top != 0 {
		t.Errorf("after walking back: cursor %d top %d, want 0 and 0", l.cursor, l.top)
	}
}

func TestWindowedRowAtLineIsAbsolute(t *testing.T) {
	l := newFuzzyList("", flatItems(10))
	l.setMaxRows(3)
	for range 5 {
		l.moveDown()
	}
	// top is 3: the first drawn line is filtered row 3.
	if i, ok := l.rowAtLine(0); !ok || i != 3 {
		t.Errorf("rowAtLine(0) = %d,%v, want 3,true", i, ok)
	}
	if _, ok := l.rowAtLine(3); ok {
		t.Error("rowAtLine(3) hit a row past the window")
	}
	// focusRow with the absolute index lands on the drawn row.
	if !l.focusRow(4) || l.cursor != 4 || l.top != 3 {
		t.Errorf("focusRow(4): cursor %d top %d", l.cursor, l.top)
	}
}

func TestWindowClampsWhenTheListShrinks(t *testing.T) {
	l := newFuzzyList("", flatItems(10))
	l.setMaxRows(3)
	for range 9 {
		l.moveDown()
	}
	if l.top != 7 {
		t.Fatalf("top = %d, want 7", l.top)
	}
	l.setItems(flatItems(2))
	if l.top != 0 || l.cursor > 1 {
		t.Errorf("after shrinking to 2 rows: top %d cursor %d", l.top, l.cursor)
	}
	// A window taller than the list draws it all, and marks neither end: a
	// marker that outstayed its content would be a glyph in the corner meaning
	// nothing, which is worse than no marker at all.
	if v := l.rowsView("none", 40); drawnRows(v) != 2 ||
		strings.Contains(v, overflowUpGlyph) || strings.Contains(v, overflowDownGlyph) {
		t.Errorf("rowsView on a short list:\n%s", v)
	}
}

func TestUnwindowedListDrawsEverything(t *testing.T) {
	l := newFuzzyList("", flatItems(10))
	for range 9 {
		l.moveDown()
	}
	if l.top != 0 {
		t.Errorf("an unwindowed list scrolled: top = %d", l.top)
	}
	if n := drawnRows(l.rowsView("none", 40)); n != 10 {
		t.Errorf("unwindowed rowsView drew %d rows, want 10", n)
	}
	if i, ok := l.rowAtLine(9); !ok || i != 9 {
		t.Errorf("rowAtLine(9) = %d,%v", i, ok)
	}
}

func TestPrefixFirstLeadsWithNamesThatStartWithTheQuery(t *testing.T) {
	items := []listItem{
		{name: "init_test.go", selectable: true, ref: 0},
		{name: "internal/", selectable: true, ref: 1},
		{name: "cli.go", selectable: true, ref: 2},
		{name: "Interfaces.md", selectable: true, ref: 3},
	}
	plain := newFuzzyList("", items)
	setQuery(&plain, "int")
	pf := newFuzzyList("", items)
	pf.prefixFirst = true
	setQuery(&pf, "int")

	names := func(l fuzzyList) []string {
		var out []string
		for _, s := range l.filtered {
			out = append(out, s.item.name)
		}
		return out
	}
	// The prefix hits lead, in list order and case-insensitively; the fuzzy
	// hit that merely contains the letters follows; nothing is listed twice.
	if got := strings.Join(names(pf), ","); got != "internal/,Interfaces.md,init_test.go" {
		t.Errorf("prefixFirst order = %q", got)
	}
	// Without the flag the scorer's own order stands (whatever it is), and the
	// same rows are present — the flag reorders, it never filters.
	if len(names(plain)) != len(names(pf)) {
		t.Errorf("prefixFirst changed the match set: %v vs %v", names(plain), names(pf))
	}
	// A prefix row highlights its prefix.
	if m := pf.filtered[0].matched; len(m) != 3 || m[0] != 0 || m[2] != 2 {
		t.Errorf("prefix highlight = %v, want [0 1 2]", m)
	}
}

// TestOverflowMarkRefusesToMoveARow pins the one rule the marker cannot bend.
// rowAtLine counts the lines rowsView writes, so a marker that pushed a row
// past the pane's edge would wrap it in the terminal, put every row below it a
// physical line lower than the hit test believes, and hand clicks to the wrong
// prompts. Annotating a row is allowed; moving one is not.
func TestOverflowMarkRefusesToMoveARow(t *testing.T) {
	const row = "  a prompt"
	mark := overflowMarks(0, 12)

	// Room to spare: the marker lands flush with the right edge, and the line
	// is exactly as wide as the pane — never wider.
	got := withOverflowMark(row, mark, 40, false)
	if w := lipgloss.Width(got); w != 40 {
		t.Errorf("marked row is %d columns wide, want the pane's 40:\n%q", w, got)
	}
	if !strings.HasSuffix(stripANSI(got), mark) {
		t.Errorf("the marker is not at the right edge: %q", stripANSI(got))
	}

	// No room: the row comes back untouched rather than growing.
	for _, width := range []int{len(row) + 1, len(row), 4, 0, -1} {
		if got := withOverflowMark(row, mark, width, false); got != row {
			t.Errorf("width %d: row was changed to %q", width, got)
		}
	}

	// Nothing to say, nothing drawn.
	if got := withOverflowMark(row, "", 40, false); got != row {
		t.Errorf("an empty mark still changed the row: %q", got)
	}
}

// TestSeparatorLinesCountsWhatHeadingsCost pins the conversion a caller needs
// to size an item-counted window against a line-counted pane: a heading costs
// two lines (its blank and its title), a bare spacer one, and a selectable row
// costs nothing extra because the window already budgets one line for it.
func TestSeparatorLinesCountsWhatHeadingsCost(t *testing.T) {
	l := newFuzzyList("", []listItem{
		{name: "Project"},                     // blank + heading = 2
		{name: "a", selectable: true, ref: 0}, // already budgeted
		{},                                    // a bare spacer = 1
		{name: "Global"},                      // 2
		{name: "b", selectable: true, ref: 1},
	})
	if got := l.separatorLines(); got != 5 {
		t.Errorf("separatorLines = %d, want 5", got)
	}
	// It counts over ITEMS, not the filtered set, so a query that drops every
	// separator does not shrink the answer — that is what lets a caller size
	// the window once instead of on every keystroke.
	l.input.SetValue("a")
	l.filter()
	if got := l.separatorLines(); got != 5 {
		t.Errorf("separatorLines under a filter = %d, want the unfiltered 5", got)
	}
}
