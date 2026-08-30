// promptmove.go — walking a line, or a swept block of them, up and down the
// prompt.
//
// `alt+↑` / `alt+↓` is where every editor on this machine keeps "move this line",
// and a prompt is edited often enough — a plan reordered, a bullet promoted above
// its neighbour — that reaching for it and finding nothing is the sort of small
// absence that makes an editor feel unfinished. It is also the natural partner of
// the two features already on a swept run: sort puts a whole block in order,
// this moves one line to where you actually wanted it.
//
//	- tag v2                     - tag v2
//	- announce it        alt+↓   - write the notes
//	- write the notes    ─────▶  - announce it
//	  ▲ caret here                 ▲ caret still here, same column
//
// Two things it does that a naive swap would not:
//
//   - With a run swept, the whole block of lines moves and the highlight travels
//     with it, exactly as it was — including a selection that starts and ends
//     mid-word. The block's text does not change, only where it begins, so every
//     offset inside it shifts by the same amount and the span can simply be
//     carried along.
//   - "Line" is a logical row, not a drawn one. A long row that soft-wraps over
//     three display lines moves whole, for the reason duplicatePromptLine gives:
//     a wrap segment is not a boundary the value contains.

package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// promptLineMoveKey answers whether msg is alt+↑ or alt+↓ and which way it goes:
// -1 up, +1 down.
//
// Matched on Code and the alt bit rather than on the chord's printed name so the
// answer does not depend on how a terminal spells the modifier. Shift is
// deliberately not tolerated here: shift+alt+↑/↓ extends the selection instead,
// and is taken by promptSelectionKey one branch earlier (see the note there).
func promptLineMoveKey(msg tea.KeyPressMsg) (int, bool) {
	if msg.Mod&tea.ModAlt == 0 || msg.Mod&tea.ModShift != 0 {
		return 0, false
	}
	switch msg.Code {
	case tea.KeyUp:
		return -1, true
	case tea.KeyDown:
		return 1, true
	}
	return 0, false
}

// movePromptLines is alt+↑ / alt+↓: move the caret's line — or every line a
// selection touches — one row in the given direction.
//
// The edit goes through replacePromptRunes over just the rows involved, the same
// road every other programmatic edit in this program takes: the textarea only
// edits at its caret, so swapping two rows by walking the caret over them and
// retyping would be the same result by a much longer road, one that goes wrong
// quietly on a soft-wrapped row.
func (m model) movePromptLines(delta int) (tea.Model, tea.Cmd) {
	if m.formFocus != formFieldPrompt {
		// The title is one line by construction — it has nowhere to move to —
		// and the annotation bar is not text at all. Same answer
		// duplicatePromptLine gives from the same two stops.
		m.formNote = "moving lines works in the prompt"
		return m, nil
	}
	rows := strings.Split(m.promptArea.Value(), "\n")

	// Which rows are going. A standing selection names them; with none, it is
	// the one row the caret is on.
	first, last, _, _, ok := m.promptSelRows()
	if !ok {
		first = min(max(m.promptArea.Line(), 0), len(rows)-1)
		last = first
	}
	if delta < 0 && first == 0 {
		m.formNote = "already at the top"
		return m, nil
	}
	if delta > 0 && last == len(rows)-1 {
		m.formNote = "already at the last line"
		return m, nil
	}

	// The affected span is the block plus the one row it changes places with, so
	// the whole edit is a single contiguous replacement.
	lo, hi := first, last
	if delta < 0 {
		lo--
	} else {
		hi++
	}
	moved := movePromptRowBlock(rows[lo:hi+1], first-lo, last-lo, delta)
	start, end := promptRowSpan(rows, lo, hi)

	// How far every offset inside the block travels: the difference between
	// where its first row began and where it begins now. The block's own text is
	// untouched, so this one number carries the caret and both ends of a
	// selection.
	before, _ := promptRowSpan(rows, first, first)
	after, _ := promptRowSpan(moved, first-lo+delta, first-lo+delta)
	shift := (start + after) - before

	caret := promptCaretOffset(m.promptArea)
	anchor, hadSel := m.promptSel.anchor, m.promptSel.active
	m.replacePromptRunes(start, end, strings.Join(moved, "\n"))
	if hadSel {
		m.promptSel = promptSel{anchor: anchor + shift, active: true}
	}
	setPromptCaretOffset(&m.promptArea, caret+shift)
	m.formNote = ""
	return m, nil
}

// movePromptRowBlock returns block with rows[first..last] shifted one place in
// the given direction — which, inside a slice that holds exactly the block and
// its one neighbour, means the neighbour hops over it.
//
//	delta > 0        delta < 0
//	 b  ┐             a  ┐ neighbour
//	 c  │ block       b  ┤ block
//	 d  ┘             c  ┘
//	 e ── neighbour
//	        ↓                ↓
//	 e                 b
//	 b                 c
//	 c                 a
//	 d
//
// copy is safe over the overlap: it moves the range as a whole rather than
// element by element, so the source is not clobbered as it is read.
func movePromptRowBlock(block []string, first, last, delta int) []string {
	out := append([]string{}, block...)
	if delta < 0 {
		neighbour := out[first-1]
		copy(out[first-1:last], out[first:last+1])
		out[last] = neighbour
		return out
	}
	neighbour := out[last+1]
	copy(out[first+1:last+2], out[first:last+1])
	out[first] = neighbour
	return out
}
