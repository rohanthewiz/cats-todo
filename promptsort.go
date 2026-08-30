// promptsort.go — putting the swept lines of the prompt in order.
//
// The gesture this belongs to is the same one the split belongs to: a list has
// been pasted into the editor and is about to become work. Sorting it first is
// how a pasted jumble becomes something a reader can scan — and, since the split
// keeps the backlog order of the items it makes, sorting before splitting is
// also how the resulting prompts land in an order somebody chose.
//
//	- write the notes            - announce it
//	- announce it        ──▶     - tag v2
//	- tag v2                     - write the notes
//
// Two rules shape it:
//
//   - It sorts whole lines, always. A sweep that stops mid-word still means the
//     lines it crossed (promptlines.go), because half a line has no place in an
//     order.
//   - A markdown list is sorted as *items*, not as lines. An item's sub-points
//     and wrapped continuation lines travel with it — sorting those as lines of
//     their own would shuffle a list's details away from the items they explain.

package main

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// sortPromptLines is ⇅ Sort lines on the editor's context menu: order the swept
// block, in place.
//
// The selection survives, moved onto the sorted text rather than dropped. That
// is what makes the two menu items compose: sort a list, then split it, without
// having to sweep it a second time — and it is also the only visible proof of
// what the sort took as its input, since the block is otherwise the same
// characters in a different order.
func (m model) sortPromptLines() (tea.Model, tea.Cmd) {
	if m.formFocus != formFieldPrompt {
		m.formNote = "sorting lines works in the prompt"
		return m, nil
	}
	first, last, start, end, ok := m.promptSelRows()
	if !ok {
		m.formNote = "nothing selected — sweep the lines to sort"
		return m, nil
	}
	if first == last {
		// One line is already in order. Say so rather than reporting a sort that
		// could not have changed anything — a no-op dressed as a success is the
		// one answer that teaches the user nothing about what the item does.
		m.formNote = "sweep two or more lines to sort them"
		return m, nil
	}
	runes := []rune(m.promptArea.Value())
	sorted, n, kind := sortPromptBlock(string(runes[start:end]))
	m.replacePromptRunes(start, end, sorted)
	// Re-anchor over what was just written. replacePromptRunes has already left
	// the caret at the end of the replacement, so the span runs from start to
	// wherever it is now.
	m.promptSel = promptSel{anchor: start, active: true}
	m.formNote = fmt.Sprintf("sorted %d %s", n, kind)
	return m, nil
}

// sortPromptBlock orders one block of text and reports how many things it
// ordered and what it called them.
//
// Which of the two sorts runs is decided by the text, not by a mode the user has
// to choose: a block holding two or more markdown items is a list and is sorted
// as one, and anything else is sorted as plain lines. That is the same reading
// splitBulletList already applies to a sweep, so the two menu items agree about
// what "a list" means — a block the split will take is a block the sort treats
// as items.
func sortPromptBlock(block string) (out string, n int, kind string) {
	if head, items := splitBulletList(block); len(items) > 1 {
		return sortBulletItems(block, head, items), len(items), "items"
	}
	lines := strings.Split(block, "\n")
	sortLinesInPlace(lines)
	return strings.Join(lines, "\n"), len(lines), "lines"
}

// sortBulletItems orders a markdown list's items and puts them back into the
// markers they came in.
//
// The markers stay where they are and the bodies move between them, which is the
// whole trick: an ordered list comes back numbered 1, 2, 3 down the page instead
// of carrying its old numbers along with the text, and a list whose markers all
// read "-" is unaffected either way. It also means an item's continuation lines
// are re-indented to the marker they land under (bulletBlock.render), so a "10."
// item and a "9." item both line up under their own text.
//
// Anything above the first bullet is not part of the list and is left exactly
// where it was, the same rule the split follows for a sweep that caught the
// sentence introducing its list.
func sortBulletItems(block string, head int, items []bulletBlock) string {
	markers := make([]bulletBlock, len(items))
	copy(markers, items)

	sort.SliceStable(items, func(i, j int) bool { return items[i].sortKey() < items[j].sortKey() })

	out := make([]string, 0, len(items))
	for i, it := range items {
		// The i'th marker with the i'th sorted body: position keeps the marker,
		// order carries the text.
		it.marker, it.content = markers[i].marker, markers[i].content
		out = append(out, it.render())
	}
	return string([]rune(block)[:head]) + strings.Join(out, "\n")
}

// sortLinesInPlace orders plain lines: case and surrounding space are out of the
// comparison, because "Tag v2" and "tag v2" belong beside each other and a line
// that happens to be indented one space further is not a different kind of line.
//
// Blank lines sort to the end rather than to the top. A gap between two lines is
// a separator, and a separator has nothing left to separate once the order has
// changed — collecting them at the bottom is the one placement that neither
// deletes them nor leaves a hole punched through the middle of the block.
func sortLinesInPlace(lines []string) {
	key := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	sort.SliceStable(lines, func(i, j int) bool {
		a, b := key(lines[i]), key(lines[j])
		if (a == "") != (b == "") {
			return b == ""
		}
		return a < b
	})
}
