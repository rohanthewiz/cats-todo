// annotations.go — the marks a todo wears beside its state.
//
// A row in the list answers three different questions, and they were being asked
// of one column. What state is this prompt in (open, scheduled, frozen, done) is
// exclusive: a todo is in exactly one, and the badge that says so has always been
// one glyph. How much does it matter and how cheap is it are not exclusive, not
// exclusive of each other, and not exclusive of the state either — a critical
// one-liner is critical *and* a quick win *and* still open. Those are
// annotations: independent facts a todo carries, each with its own glyph, each in
// its own column so the pane can be read straight down.
//
// The row therefore reads outward from the cursor as
//
//	❯ ✓  ▲ 🍏  fix the thing            the prompt's first line
//	  │  └─┴── annotations: what is true about this prompt
//	  └─────── the state badge: which of the three groups it lives in
//
// The badge leads because it is the fact the list is grouped by — the eye
// arriving at a row wants "is this still work" before "how much work". The
// annotations follow in a fixed order, so the same mark is always in the same
// column, which is the only thing that makes a glyph worth more than the word it
// replaces.
//
// Frozen is not here. It is a state, mutually exclusive with done, and it stays
// in the badge where the three render groups are read from (see rebuildList).
package main

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// annots is the annotation set as one value. The marks are stored as separate
// fields on Todo (each with its own omitempty, so an unmarked backlog is
// byte-identical to what it was before they existed), but they are *edited*
// together and *saved* together, and passing them around as a set is what keeps
// the form, the store and the CLI from each growing a parameter per mark.
//
// Adding a mark is: a field on Todo, a field here, a line in each of the three
// methods below, and an entry in annotSlots. Nothing else has to know.
type annots struct {
	Priority string // priorityNone | priorityHigh | priorityCritical
	Fruit    bool   // low-hanging fruit — a quick win
}

// annotsOf reads a todo's annotations off it.
func annotsOf(t Todo) annots {
	return annots{Priority: t.Priority, Fruit: t.Fruit}
}

// applyTo writes the set back onto a todo, leaving everything else alone.
func (a annots) applyTo(t *Todo) {
	t.Priority = a.Priority
	t.Fruit = a.Fruit
}

// any reports whether anything has actually been said. It is what lets a screen
// stay silent about a prompt nobody has annotated — the CLI's echo — rather
// than announce the defaults on every one.
func (a annots) any() bool {
	return a.Priority != priorityNone || a.Fruit
}

// summary is the annotations in words, for the screens with no room to draw
// them — the CLI's echo after an add. (The form no longer needs it: its
// annotation bar draws the marks live, annotbar.go.) Empty when nothing has
// been said, so the caller can decide whether a line is worth spending at all.
//
// Built by walking the slot table rather than by a switch of its own, so the
// words and the columns can never fall into different orders or disagree about
// what a mark is called.
func (a annots) summary() string {
	var t Todo
	a.applyTo(&t)
	var parts []string
	for _, sl := range annotSlots {
		if l := sl.label(t); l != "" {
			parts = append(parts, l)
		}
	}
	return strings.Join(parts, " · ")
}

// annotSlot is one annotation column: what it is called, how many cells it keeps
// whether or not a given row fills it, and how to read it off a todo.
//
// width is declared rather than measured because it is a property of the column,
// not of the row being drawn: every glyph a slot can produce is the same width,
// and a blank slot has to reserve exactly that much so the names below it stay in
// one line. The fruit is two cells because it is an emoji and emoji are wide;
// the geometric marks are one, being East Asian Ambiguous like the ○ this list
// has always drawn.
//
// mark returns the glyph and the two styles to draw it in — ordinary and on the
// highlighted row. A slot with nothing to say returns an empty glyph, and the
// column goes blank rather than falling back to a default: an annotation is
// something someone said about this prompt, and "nobody said anything" is not a
// value to be drawn.
//
// label is the same fact in words, for the screens that have room to spell it
// out (the prompt view) and the ones with no room to draw a glyph at all (the
// CLI's echo). Empty exactly when mark's glyph is, so one table answers
// both and the words cannot drift from the columns. It is the *value* rather
// than the column — "critical", not "priority" — because a reader who needed the
// word instead of the mark needed the whole fact.
type annotSlot struct {
	name  string
	width int
	mark  func(t Todo) (glyph string, style, selStyle lipgloss.Style)
	label func(t Todo) string
}

// annotSlots is the layout: the annotation columns, left to right, in the order
// they are drawn. Priority leads because it is the one that decides what happens
// next; the fruit qualifies it ("…and it's cheap").
var annotSlots = []annotSlot{
	{name: "priority", width: 1, mark: priorityMark, label: priorityAnnotLabel},
	{name: "low-hanging fruit", width: 2, mark: fruitMark, label: fruitAnnotLabel},
}

// priorityAnnotLabel names the level, or says nothing at a level that draws
// nothing — the two have to agree, or a prompt would read as unmarked on its row
// and as ranked on the screen that spells it out.
func priorityAnnotLabel(t Todo) string {
	if glyph, _, _ := priorityMark(t); glyph == "" {
		return ""
	}
	return priorityLabel(t.Priority)
}

func fruitAnnotLabel(t Todo) string {
	if !t.Fruit {
		return ""
	}
	return "low-hanging fruit"
}

// priorityMark is the priority column. Only a raised level draws — see the
// Priority constants for why "none" is a blank column rather than a third glyph.
//
// A hand-edited backlog can hold anything, including the "low" the old scheme
// wrote. Anything unrecognized draws nothing, which is the honest reading: the
// level it could not be read as is not a level this program raises.
func priorityMark(t Todo) (string, lipgloss.Style, lipgloss.Style) {
	var glyph string
	var st lipgloss.Style
	switch t.Priority {
	case priorityCritical:
		glyph, st = prioCriticalGlyph, prioCriticalStyle
	case priorityHigh:
		glyph, st = prioHighGlyph, prioHighStyle
	default:
		return "", lipgloss.NewStyle(), lipgloss.NewStyle()
	}
	// A closed row's mark drops to the greys the rest of that row is drawn in.
	// Priority is about what to do next, and finished work arguing for attention
	// is exactly what the done tier exists to prevent — but the glyph stays, so
	// the record of what the prompt was rated still reads.
	if t.closed() {
		return glyph, prioClosedStyle, prioClosedSelStyle
	}
	return glyph, st, st
}

// fruitMark is the low-hanging-fruit column.
//
// The receded styles on a closed row do almost nothing here — an emoji paints
// itself and ignores a foreground — so a done quick win keeps its colour where a
// done critical loses its red. That is a limitation of the glyph rather than a
// decision; the styles are still applied, because the highlighted row needs its
// field behind every segment it draws, colour or no colour.
func fruitMark(t Todo) (string, lipgloss.Style, lipgloss.Style) {
	if !t.Fruit {
		return "", lipgloss.NewStyle(), lipgloss.NewStyle()
	}
	if t.closed() {
		return fruitGlyph, prioClosedStyle, prioClosedSelStyle
	}
	return fruitGlyph, fruitStyle, fruitStyle
}

// annotMarksFor renders a todo's annotations into one column per slot, blanks
// included. The blanks matter: trimAnnotColumns needs to see which columns are
// empty across the whole list before any of them can be dropped, and the
// renderer needs every row to carry the same columns in the same order.
func annotMarksFor(t Todo) []annotMark {
	marks := make([]annotMark, len(annotSlots))
	for i, sl := range annotSlots {
		glyph, st, sel := sl.mark(t)
		marks[i] = annotMark{text: glyph, width: sl.width, style: st, selStyle: sel}
	}
	return marks
}

// trimAnnotColumns drops the columns that no row in the list fills, in place.
//
// Without it every backlog would pay for every annotation anyone might use:
// three cells of indent on every row of a list where nothing is annotated at all,
// growing each time a mark is added. With it, a backlog that uses no annotations
// looks exactly as it did before they existed, one that uses only the fruit
// spends one column, and the columns that are drawn are still drawn on every row
// — which is what keeps them scannable.
//
// It is a whole-list decision on purpose: deciding per row would leave the names
// ragged, which costs more than the indent saves.
func trimAnnotColumns(items []listItem) {
	used := make([]bool, len(annotSlots))
	for _, it := range items {
		for i, an := range it.annots {
			if an.text != "" {
				used[i] = true
			}
		}
	}
	keep := make([]int, 0, len(used))
	for i, u := range used {
		if u {
			keep = append(keep, i)
		}
	}
	if len(keep) == len(used) {
		return // every column is in use; nothing to do
	}
	for i := range items {
		if len(items[i].annots) == 0 {
			continue // separators and headings carry no marks
		}
		trimmed := make([]annotMark, 0, len(keep))
		for _, k := range keep {
			trimmed = append(trimmed, items[i].annots[k])
		}
		items[i].annots = trimmed
	}
}
