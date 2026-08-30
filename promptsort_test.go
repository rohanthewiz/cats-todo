package main

import (
	"strings"
	"testing"
)

// TestSortPromptBlock is the sort on its own: which of the two orderings runs,
// and what each does with the block it is given.
func TestSortPromptBlock(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		out  string
		n    int
		kind string
	}{
		{
			name: "a bulleted list sorts as items",
			in:   "- write the notes\n- announce it\n- tag v2",
			out:  "- announce it\n- tag v2\n- write the notes",
			n:    3, kind: "items",
		},
		{
			name: "an ordered list keeps its numbers in place and renumbers",
			in:   "1. write the notes\n2. announce it\n3. tag v2",
			out:  "1. announce it\n2. tag v2\n3. write the notes",
			n:    3, kind: "items",
		},
		{
			name: "an item's sub-points travel with it",
			in:   "- write the notes\n  - link the diff\n- announce it",
			out:  "- announce it\n- write the notes\n  - link the diff",
			n:    2, kind: "items",
		},
		{
			name: "text above the first bullet is not part of the list",
			in:   "Ship it:\n- write the notes\n- announce it",
			out:  "Ship it:\n- announce it\n- write the notes",
			n:    2, kind: "items",
		},
		{
			name: "plain lines sort as lines, case and space out of it",
			in:   "  Write the notes\nannounce it\nTag v2",
			out:  "announce it\nTag v2\n  Write the notes",
			n:    3, kind: "lines",
		},
		{
			name: "blank lines collect at the end rather than the top",
			in:   "beta\n\nalpha",
			out:  "alpha\nbeta\n",
			n:    3, kind: "lines",
		},
		{
			name: "one bullet is not a list to sort as items",
			in:   "- only\nplain",
			out:  "- only\nplain",
			n:    2, kind: "lines",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, n, kind := sortPromptBlock(tc.in)
			if out != tc.out {
				t.Errorf("sorted:\n%q\nwant:\n%q", out, tc.out)
			}
			if n != tc.n || kind != tc.kind {
				t.Errorf("reported %d %s, want %d %s", n, kind, tc.n, tc.kind)
			}
		})
	}
}

// TestSortPromptBlockIsStable: two items that compare equal keep the order they
// were written in, so sorting a list twice cannot shuffle it a second time.
func TestSortPromptBlockIsStable(t *testing.T) {
	in := "- same\n- SAME\n- same"
	out, _, _ := sortPromptBlock(in)
	if out != in {
		t.Errorf("sorted %q to %q, want it untouched", in, out)
	}
	again, _, _ := sortPromptBlock(out)
	if again != out {
		t.Errorf("a second sort changed %q to %q", out, again)
	}
}

// TestSortPromptLinesKeepsTheSelection: the highlight moves onto the sorted
// text rather than being dropped, which is what lets the two menu items compose
// — sort a list, then split it, without sweeping twice.
func TestSortPromptLinesKeepsTheSelection(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "- write the notes\n- announce it")
	m = selectWholePrompt(t, m)

	next, _ := m.sortPromptLines()
	got := next.(model)
	if want := "- announce it\n- write the notes"; got.promptArea.Value() != want {
		t.Fatalf("value = %q, want %q", got.promptArea.Value(), want)
	}
	lo, hi, ok := got.promptSelSpan()
	if !ok {
		t.Fatal("the sort dropped the selection it had just reordered")
	}
	if lo != 0 || hi != len([]rune(got.promptArea.Value())) {
		t.Errorf("selection = [%d, %d), want the whole sorted block", lo, hi)
	}
	if !strings.Contains(got.formNote, "sorted 2 items") {
		t.Errorf("form note = %q, want it to report two items", got.formNote)
	}
}

// TestSortPromptLinesTakesWholeLines: a sweep that stops mid-word still means
// the lines it crossed — half a line has no place in an order.
func TestSortPromptLinesTakesWholeLines(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "beta\nalpha\ngamma")
	// From the middle of "beta" to the middle of "alpha".
	m = selectPromptRange(t, m, 2, 8)

	next, _ := m.sortPromptLines()
	if want := "alpha\nbeta\ngamma"; next.(model).promptArea.Value() != want {
		t.Errorf("value = %q, want %q — the swept lines sorted whole, gamma untouched",
			next.(model).promptArea.Value(), want)
	}
}

// TestSortPromptLinesRefusesInWords covers each way the item cannot apply.
func TestSortPromptLinesRefusesInWords(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T) model
		want  string
	}{
		{
			name: "nothing selected",
			setup: func(t *testing.T) model {
				m, _, _ := splitFormInTemp(t, "b\na")
				m.focusForm(formFieldPrompt)
				return m
			},
			want: "nothing selected",
		},
		{
			name: "a single line",
			setup: func(t *testing.T) model {
				m, _, _ := splitFormInTemp(t, "just the one line")
				return selectWholePrompt(t, m)
			},
			want: "two or more lines",
		},
		{
			name: "from the title field",
			setup: func(t *testing.T) model {
				m, _, _ := splitFormInTemp(t, "b\na")
				m.focusForm(formFieldTitle)
				return m
			},
			want: "works in the prompt",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.setup(t)
			next, _ := before.sortPromptLines()
			got := next.(model)
			if got.promptArea.Value() != before.promptArea.Value() {
				t.Errorf("a refused sort changed the value to %q", got.promptArea.Value())
			}
			if !strings.Contains(got.formNote, tc.want) {
				t.Errorf("form note = %q, want it to mention %q", got.formNote, tc.want)
			}
		})
	}
}

// TestPromptRowRange pins the conversion every line tool depends on: which rows
// a rune span names. The boundary cases are the whole point — a sweep that ends
// exactly at a row's first character has not put anything of that row under the
// highlight.
func TestPromptRowRange(t *testing.T) {
	rows := []string{"- one", "- two", "- three"} // offsets 0-5, 6-11, 12-19
	for _, tc := range []struct {
		name                string
		lo, hi              int
		wantFirst, wantLast int
	}{
		{name: "inside one row", lo: 2, hi: 4, wantFirst: 0, wantLast: 0},
		{name: "the whole value", lo: 0, hi: 19, wantFirst: 0, wantLast: 2},
		{name: "through the newline only", lo: 0, hi: 6, wantFirst: 0, wantLast: 0},
		{name: "one cell into the next row", lo: 0, hi: 7, wantFirst: 0, wantLast: 1},
		{name: "an empty span names its own row", lo: 8, hi: 8, wantFirst: 1, wantLast: 1},
		{name: "past the end clamps", lo: 14, hi: 99, wantFirst: 2, wantLast: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first, last := promptRowRange(rows, tc.lo, tc.hi)
			if first != tc.wantFirst || last != tc.wantLast {
				t.Errorf("rows %d..%d, want %d..%d", first, last, tc.wantFirst, tc.wantLast)
			}
		})
	}
}

// TestPromptRowSpan: the rune range a run of rows occupies, newlines between
// them in and the one after the last row out — replacing that range must not
// swallow the line after the block.
func TestPromptRowSpan(t *testing.T) {
	rows := []string{"- one", "- two", "- three"}
	for _, tc := range []struct {
		first, last, wantStart, wantEnd int
	}{
		{0, 0, 0, 5},
		{1, 1, 6, 11},
		{0, 1, 0, 11},
		{0, 2, 0, 19},
		{2, 2, 12, 19},
	} {
		start, end := promptRowSpan(rows, tc.first, tc.last)
		if start != tc.wantStart || end != tc.wantEnd {
			t.Errorf("rows %d..%d span [%d, %d), want [%d, %d)",
				tc.first, tc.last, start, end, tc.wantStart, tc.wantEnd)
		}
	}
}
