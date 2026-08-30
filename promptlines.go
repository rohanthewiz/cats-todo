// promptlines.go — the logical-line arithmetic the editor's line tools share.
//
// Sorting a swept block (promptsort.go) and dropping a caret on each of its
// lines (promptcarets.go) are both operations on *whole logical rows*, not on
// the rune span the hand actually swept: a sweep that stops halfway through the
// last line still means that line, and one that runs a single cell past a
// newline does not mean the line after it. Getting that conversion wrong is the
// kind of bug that only shows up on the first or last line of a block, so it is
// written once here rather than twice in the two files that need it.
//
//	value:  "- one\n- two\n- three"
//	rows:      0       1      2
//	sweep [2, 9)  ─────────┘        → rows 0..1, whole-line span [0, 11)

package main

import "strings"

// promptRowRange is the inclusive range of logical rows the rune span [lo, hi)
// touches.
//
// hi is stepped back one rune before it is placed, which is what stops a sweep
// that ends exactly at a row's first character — the shape a downward drag
// naturally lands in — from claiming a row it has not put a single character of
// under the highlight. A caret-width span (lo == hi) is left alone: it names the
// one row it sits on.
func promptRowRange(rows []string, lo, hi int) (first, last int) {
	if hi > lo {
		hi--
	}
	first, last = len(rows)-1, len(rows)-1
	found := false
	off := 0
	for i, r := range rows {
		end := off + len([]rune(r)) // the row's last offset, before its newline
		if !found && lo <= end {
			first, found = i, true
		}
		if hi <= end {
			last = i
			break
		}
		off = end + 1 // step over the '\n' that ended the row
	}
	return first, max(last, first)
}

// promptRowSpan is the rune range [start, end) that rows first..last occupy in
// the value, newlines between them included and the one after the last row
// excluded — the range a line tool replaces.
func promptRowSpan(rows []string, first, last int) (start, end int) {
	first = min(max(first, 0), len(rows)-1)
	last = min(max(last, first), len(rows)-1)
	for i := range first {
		start += len([]rune(rows[i])) + 1
	}
	end = start
	for i := first; i <= last; i++ {
		end += len([]rune(rows[i])) + 1
	}
	return start, end - 1 // less the newline that ends the last row
}

// promptSelRows is the selection resolved into whole rows: the row range it
// touches and the rune span those rows occupy. ok is false when nothing is
// selected, which is every line tool's first refusal.
func (m model) promptSelRows() (first, last, start, end int, ok bool) {
	lo, hi, ok := m.promptSelSpan()
	if !ok {
		return 0, 0, 0, 0, false
	}
	rows := strings.Split(m.promptArea.Value(), "\n")
	first, last = promptRowRange(rows, lo, hi)
	start, end = promptRowSpan(rows, first, last)
	return first, last, start, end, true
}
