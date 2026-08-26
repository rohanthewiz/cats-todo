// Priority is the backlog's second axis, and the first of its annotations
// (see annotations.go).
//
// The first axis is array order, which the user sets by hand and which answers
// "what is next". It cannot answer "how much does this matter": once a critical
// bug and a nice-to-have are adjacent, the order between them is the only thing
// distinguishing them, and every drag churns it. Priority is the fact that
// survives the churn — two ways of saying "raise this", drawn as a colored
// triangle in a column of its own so the list can be read down rather than
// across.
//
// Sorting on it is deliberately a lens rather than a rule (see rebuildList and
// orderIsBacklogOrder): the file's order stays the user's, and turning the lens
// off gives it back exactly.
package main

import "fmt"

// prioValues is the levels in the order they escalate: none, then up. The
// form's annotation bar lays them out as radios in this same order (see
// annotbar.go), so one step right of "none" means "this matters" and two mean
// "this matters most" — the order a person thinks in when they reach for the
// control at all.
var prioValues = []string{priorityNone, priorityHigh, priorityCritical}

// normalizePriority folds what a person would type onto the stored spelling and
// rejects the rest. The set is short and closed, so — unlike the model name —
// there is nothing to be gained by passing an unknown value through.
//
// "none" is spelled out as well as left blank so that `--priority none` can
// clear a value rather than only being the absence of one, the same turn
// normalizeFinish makes.
//
// The words the old three-level scheme used still fold, onto whichever new level
// means what they meant: "standard"/"normal"/"medium" said "ordinary", and
// ordinary is now none; "low" and "minor" said "not raised", which is also none.
// A script or a shell history holding `--priority low` keeps working and keeps
// meaning what it meant. "urgent" folds onto critical for the reason it always
// did — the word that comes to mind first is not always the word this program
// stores, and refusing it would be pedantry with nothing on the other side.
func normalizePriority(s string) (string, error) {
	switch foldOption(s) {
	case "", "none", "standard", "normal", "medium", "med", "std", "default", "low", "minor":
		return priorityNone, nil
	case "high", "raised", "important":
		return priorityHigh, nil
	case "critical", "crit", "urgent":
		return priorityCritical, nil
	}
	return "", fmt.Errorf("priority %q is not one of critical, high, none", s)
}

// priorityLabel is the word for a priority wherever one is spelled out — the
// prompt view's meta line, the CLI's echo (via priorityAnnotLabel). None is
// named rather than left blank: the empty string is a storage detail, and a
// value that went blank would read as broken rather than as "nothing said".
func priorityLabel(p string) string {
	switch p {
	case priorityCritical:
		return "critical"
	case priorityHigh:
		return "high"
	case priorityNone:
		return "none"
	}
	// A hand-edited backlog can hold anything, and an old one holds "low". Show
	// it rather than claim it is none — the panel is the only place the user
	// would ever find out, and one cycle of the row replaces it with a level
	// this program does draw.
	return p
}

// priorityRank orders the levels for the list's priority lens: critical first,
// then high, then everything unraised. It is a render-time comparison key and
// nothing else, so — like the render groups — the numbers can be changed freely.
//
// An unknown value ranks with none rather than at either end: a typo in a
// hand-edited backlog, or the retired "low", should leave the row among the
// unmarked rather than promote it above work that was actually raised.
func priorityRank(p string) int {
	switch p {
	case priorityCritical:
		return 0
	case priorityHigh:
		return 1
	}
	return 2
}

// priorityNote is what a status line says after a cycle. It names the level
// rather than the color: the mark is right there on the row, and a note reading
// "red" would be teaching the legend to someone already looking at it.
//
// The reorder is called out only when one happened. With the priority lens on,
// the row can travel most of a pane on a single keystroke, and this note is the
// only thing that says the list moved on purpose rather than under the user's
// feet.
func priorityNote(p string, reordered bool) string {
	label := "none — nothing said"
	switch p {
	case priorityCritical:
		label = "critical — do this first"
	case priorityHigh:
		label = "high — before the unmarked work"
	}
	s := "priority " + label
	if reordered {
		s += " · the list reordered"
	}
	return s
}
