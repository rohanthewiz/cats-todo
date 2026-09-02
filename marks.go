// marks.go — the list's selection: which prompts an action acts on.
//
// Every action in the list has always meant "the highlighted row", because
// every action was about one prompt: edit it, drop it, freeze it. Sending
// prompts somewhere else is the first thing that is naturally about several —
// a handful of related todos to a colleague, a whole afternoon's captures to
// the other machine — and asking for them one at a time would make the common
// case the tedious one.
//
// So the list grows a selection. `ctrl+space` (`ctrl+b` where a terminal eats
// it) ticks the highlighted row; the ✓ column appears while anything is ticked
// and goes away again when nothing is (fuzzyList.showMarks). An action that can
// take a set takes the set; every other action still means the highlighted row,
// and says so rather than silently doing something bigger.
//
// The set is keyed by todoRef — scope plus id — and never by row index. A row
// index means a different prompt after a move, a delete, a fold or a change of
// filter, and a selection that quietly re-pointed itself at other prompts would
// be the worst kind of bug in a feature whose whole job is to send work
// elsewhere. The cost of that choice is this file: the set has to be pruned
// against the backlogs (pruneMarks) and ordered against the list (markedRefs)
// rather than simply indexed.
package main

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// toggleMark ticks or unticks the highlighted row. It reports what the row now
// is, and false when there was no row to tick — a filtered-to-nothing list, or
// an empty backlog — which the caller answers in words.
func (m *model) toggleMark() (marked bool, ok bool) {
	ref, ok := m.selectedRef()
	if !ok {
		return false, false
	}
	if m.marked == nil {
		m.marked = map[todoRef]bool{}
	}
	if m.marked[ref] {
		delete(m.marked, ref)
		return false, true
	}
	m.marked[ref] = true
	return true, true
}

// markSelected is the ctrl+space handler: tick the highlighted row, rebuild so
// the column appears (or goes), and say what the set now holds. The count is
// spoken every time rather than only on the first tick, because the ✓ column
// shows membership but not size, and the size is what decides whether the next
// ctrl+o is the thing the user meant.
func (m model) markSelected() (tea.Model, tea.Cmd) {
	marked, ok := m.toggleMark()
	if !ok {
		m.setStatus("nothing to select here", true)
		return m, nil
	}
	m.rebuildList()
	n := m.markCount()
	if marked {
		m.setStatus(fmt.Sprintf("selected · %s held · ctrl+o sends them", promptWord(n)), false)
		return m, nil
	}
	if n == 0 {
		m.setStatus("unselected · nothing held now", false)
		return m, nil
	}
	m.setStatus(fmt.Sprintf("unselected · %s still held", promptWord(n)), false)
	return m, nil
}

// clearMarks drops the whole selection. Called when the selection has been
// spent (an export that took it), and by esc — the key that means "never mind"
// everywhere else in this program.
func (m *model) clearMarks() {
	m.marked = nil
	m.list.showMarks = false
}

// markCount is how many prompts are selected.
func (m model) markCount() int { return len(m.marked) }

// pruneMarks drops selected refs that no longer name a prompt in a backlog —
// deleted here, or in another pane since the last rebuild. Called from
// rebuildList, which is the one place that has just re-read both stores.
func (m *model) pruneMarks() {
	for ref := range m.marked {
		if _, ok := m.resolve(ref); !ok {
			delete(m.marked, ref)
		}
	}
	if len(m.marked) == 0 {
		m.marked = nil // so markCount and showMarks agree with "nothing selected"
	}
}

// markedRefs returns the selection in the order the list draws it, so a bundle
// reads the way the screen did.
//
// The visible rows come first, in row order; anything selected but currently
// hidden — a done prompt under the fold, a prompt the filter is excluding — is
// appended in backlog order afterwards. Hidden rows are deliberately still in:
// the fold and the filter are ways of looking at the list, and a prompt does
// not leave a set the user built because they then typed a search.
func (m model) markedRefs() []todoRef {
	if len(m.marked) == 0 {
		return nil
	}
	out := make([]todoRef, 0, len(m.marked))
	seen := make(map[todoRef]bool, len(m.marked))
	for _, ref := range m.rows {
		if m.marked[ref] && !seen[ref] {
			seen[ref] = true
			out = append(out, ref)
		}
	}
	for _, s := range []*store{m.project, m.global} {
		if s == nil || !s.available() {
			continue
		}
		for _, t := range s.todos {
			ref := todoRef{scope: s.scope, id: t.ID}
			if m.marked[ref] && !seen[ref] {
				seen[ref] = true
				out = append(out, ref)
			}
		}
	}
	return out
}

// markedNote is the header's " · N selected" tag. It says "selected" rather
// than "marked" because the row draws a tick, and a reader matching the word to
// what they see should not have to translate.
func (m model) markedNote() string {
	if n := m.markCount(); n > 0 {
		return fmt.Sprintf(" · %d selected", n)
	}
	return ""
}

// resolveRefs turns refs into the prompts themselves, skipping any that have
// gone (the same tolerance pruneMarks has, for the window between a prune and
// an action) and reporting which store the first one came from.
func (m model) resolveRefs(refs []todoRef) []Todo {
	out := make([]Todo, 0, len(refs))
	for _, ref := range refs {
		if td, ok := m.resolve(ref); ok {
			out = append(out, td)
		}
	}
	return out
}
