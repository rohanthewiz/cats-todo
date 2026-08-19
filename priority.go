// Priority is the backlog's second axis.
//
// The first is array order, which the user sets by hand and which answers "what
// is next". It cannot answer "how much does this matter": once a critical bug
// and a nice-to-have are adjacent, the order between them is the only thing
// distinguishing them, and every drag churns it. Priority is the fact that
// survives the churn — three levels, drawn as a colored dot in a fixed column so
// the list can be read down rather than across.
//
// Sorting on it is deliberately a lens rather than a rule (see rebuildList and
// orderIsBacklogOrder): the file's order stays the user's, and turning the lens
// off gives it back exactly.
package main

import "fmt"

// prioValues is the ring the list's ctrl+p steps through, in the order it
// steps. Standard leads because it is the default and therefore where every
// cycle starts; critical comes next so the level people reach for is one press
// away rather than two.
//
// cycleValue keeps a value that is not in the ring (session.go), so a backlog
// hand-edited to an unknown priority is stepped off rather than clobbered.
var prioValues = []string{priorityStandard, priorityCritical, priorityLow}

// normalizePriority folds what a person would type onto the stored spelling and
// rejects the rest. The set is short and closed, so — unlike the model name —
// there is nothing to be gained by passing an unknown value through.
//
// "standard" and "normal" are spelled out as well as left blank so that
// `--priority standard` can override a value rather than only being the absence
// of one, the same turn normalizeFinish makes for "none".
//
// "urgent" and "high" fold onto critical, "minor" onto low: with three levels
// the top and bottom are unambiguous however they are named, and the word that
// comes to mind first is not always the word this program stores. Rejecting
// "urgent" would be pedantry with nothing on the other side of it.
func normalizePriority(s string) (string, error) {
	switch foldOption(s) {
	case "", "standard", "normal", "medium", "med", "std", "default":
		return priorityStandard, nil
	case "critical", "crit", "urgent", "high":
		return priorityCritical, nil
	case "low", "minor":
		return priorityLow, nil
	}
	return "", fmt.Errorf("priority %q is not one of critical, standard, low", s)
}

// priorityLabel is the word for a priority wherever one is shown — the status
// line after a cycle, the View panel's value column. Standard is named rather
// than left blank: the empty string is a storage detail, and a panel row that
// went blank would read as "unset" instead of "ordinary".
func priorityLabel(p string) string {
	switch p {
	case priorityCritical:
		return "critical"
	case priorityLow:
		return "low"
	case priorityStandard:
		return "standard"
	}
	// A hand-edited backlog can hold anything. Show it rather than claim it is
	// standard — the row is the only place the user would ever find out.
	return p
}

// priorityRank orders the levels for the list's priority lens: critical first,
// then standard, then low. It is a render-time comparison key and nothing else,
// so — like the render groups — the numbers can be changed freely.
//
// An unknown value ranks with standard rather than at either end: a typo in a
// hand-edited backlog should leave the row where it was, not promote it above
// work that was actually marked critical.
func priorityRank(p string) int {
	switch p {
	case priorityCritical:
		return 0
	case priorityLow:
		return 2
	}
	return 1
}

// priorityNote is what the status line says after a cycle. It names the level
// rather than the color: the dot is right there on the row, and a note reading
// "red" would be teaching the legend to someone already looking at it.
//
// The reorder is called out only when one happened. With the lens on the row can
// travel most of a pane on a single keystroke, and this note is the only thing
// that says the list moved on purpose rather than under the user's feet.
func priorityNote(p string, reordered bool) string {
	label := "standard — the default"
	switch p {
	case priorityCritical:
		label = "critical — do this first"
	case priorityLow:
		label = "low — whenever"
	}
	s := "priority " + label + " (ctrl+p cycles)"
	if reordered {
		s += " · the list reordered"
	}
	return s
}
