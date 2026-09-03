// annotations.go — the marks a todo wears beside its state.
//
// A row in the list answers three different questions, and they were being asked
// of one column. What state is this prompt in (open, scheduled, frozen, done) is
// exclusive: a todo is in exactly one, and the badge that says so has always been
// one glyph. How much does it matter and how cheap is it are not exclusive, not
// exclusive of each other, and not exclusive of the state either — a critical
// one-liner is critical *and* a quick win *and* still open. Those are
// annotations: independent facts a todo carries, each with its own glyph, drawn
// together in front of the name so a marked prompt is spotted at the left edge.
//
// The row therefore reads outward from the cursor as
//
//	❯ ✓ ▲ 🍏 fix the thing              the prompt's first line
//	  │ └─┴── annotations: what is true about this prompt
//	  └────── the state badge: which of the three groups it lives in
//
// The badge leads because it is the fact the list is grouped by — the eye
// arriving at a row wants "is this still work" before "how much work". The
// annotations follow it as one compact group, in a fixed order among themselves,
// and a row draws only the marks it actually has: a prompt with nothing said
// about it spends no cells at all, and its name starts where the badge ends.
//
// They used to be columns — one reserved slot per mark on every row, blanks
// included, so the glyphs could be scanned straight down the pane. That bought
// the scan by charging every row for every mark anyone might use, and it grows
// with each mark added: the names in a mostly-unannotated backlog all sat three
// cells right of where they belonged, pushed there by two glyphs on one row. The
// marks are few and they lead the row, so they are found by reading the left
// edge rather than by their column; packing them is the cheaper trade.
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
	Flag     bool   // singled out — see Todo.Flag
	// FlagNote is the flag's optional words. It rides in the set rather than
	// beside it because it is not a mark of its own: it is what this mark says,
	// and every screen that edits the flag edits the two together.
	FlagNote string
}

// annotsOf reads a todo's annotations off it.
func annotsOf(t Todo) annots {
	return annots{Priority: t.Priority, Fruit: t.Fruit, Flag: t.Flag, FlagNote: t.FlagNote}
}

// applyTo writes the set back onto a todo, leaving everything else alone.
// The note is dropped with the flag rather than kept for a later re-flag: a
// stored note nothing draws is a fact about the prompt that no screen shows,
// and the next flag would come up wearing words the user did not just write.
func (a annots) applyTo(t *Todo) {
	t.Priority = a.Priority
	t.Fruit = a.Fruit
	t.Flag = a.Flag
	t.FlagNote = ""
	if a.Flag {
		t.FlagNote = strings.TrimSpace(a.FlagNote)
	}
}

// any reports whether anything has actually been said. It is what lets a screen
// stay silent about a prompt nobody has annotated — the CLI's echo — rather
// than announce the defaults on every one.
func (a annots) any() bool {
	return a.Priority != priorityNone || a.Fruit || a.Flag
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

// annotSlot is one annotation: what it is called and how to read it off a todo.
//
// mark returns the glyph and the two styles to draw it in — ordinary and on the
// highlighted row. A slot with nothing to say returns an empty glyph and is left
// off the row entirely rather than falling back to a default: an annotation is
// something someone said about this prompt, and "nobody said anything" is not a
// value to be drawn.
//
// label is the same fact in words, for the screens that have room to spell it
// out (the prompt view) and the ones with no room to draw a glyph at all (the
// CLI's echo). It is empty exactly when the todo has nothing to say in this
// slot — but not merely when the slot declines to *draw*: a mark can go quiet
// on a closed row (see fruitMark) while the fact it records is still worth
// spelling out, so the screens that print words key off label alone and the row
// keys off mark. It is the *value* rather than the column — "critical", not
// "priority" — because a reader who needed the word instead of the mark needed
// the whole fact.
type annotSlot struct {
	name  string
	mark  func(t Todo) (glyph string, style, selStyle lipgloss.Style)
	label func(t Todo) string
}

// annotSlots is the layout: the annotations, left to right, in the order they
// are drawn — the order is fixed even though the positions are not, so a row
// wearing both marks always reads the same way round. Priority leads because it
// is the one that decides what happens next; the fruit qualifies it ("…and it's
// cheap").
var annotSlots = []annotSlot{
	{name: "priority", mark: priorityMark, label: priorityAnnotLabel},
	{name: "low-hanging fruit", mark: fruitMark, label: fruitAnnotLabel},
	// The flag trails both. It is the mark whose meaning is written on the
	// prompt rather than carried by the glyph, so it is the one a reader has to
	// stop for — and a stop belongs at the end of the group, after the two
	// facts that can be taken in at a glance.
	{name: "flag", mark: flagMark, label: flagAnnotLabel},
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

// flagAnnotLabel is the flag in words, with its note when it has one — this is
// the only annotation whose label carries anything the glyph did not, which is
// the whole reason the screens that spell the marks out exist for it.
func flagAnnotLabel(t Todo) string {
	if !t.Flag {
		return ""
	}
	if note := strings.TrimSpace(t.FlagNote); note != "" {
		return "flagged: " + note
	}
	return "flagged"
}

func fruitAnnotLabel(t Todo) string {
	if !t.Fruit {
		return ""
	}
	return "low-hanging fruit"
}

// priorityMark is the priority annotation. Only a raised level draws — see the
// Priority constants for why "none" draws nothing rather than a third glyph.
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

// fruitMark is the low-hanging-fruit annotation.
//
// A closed row does not recede here the way the priority mark does — it goes
// quiet. That is a limitation of the glyph rather than a different opinion about
// finished work: the apple is an emoji, the font paints it, and a foreground
// never reaches it, so a done quick win drawn at all is drawn at full colour,
// shouting from the one tier of the list that exists to stop shouting. Unicode
// has no grey apple to swap in and a second shape for the same fact would cost
// the mark the thing that makes it legible at a glance, so the honest recession
// is to stop drawing: the mark is for work you might still pick up, and there is none
// of that on a done or frozen row.
//
// Nothing is lost with it. The flag is still on the todo, the editor's
// annotation bar still shows it ticked, and the prompt view still spells it out
// — which is why the closed styles are handed back with the empty glyph rather
// than a bare style: that screen prints the label in them.
func fruitMark(t Todo) (string, lipgloss.Style, lipgloss.Style) {
	if !t.Fruit {
		return "", lipgloss.NewStyle(), lipgloss.NewStyle()
	}
	if t.closed() {
		return "", prioClosedStyle, prioClosedSelStyle
	}
	return fruitGlyph, fruitStyle, fruitStyle
}

// flagMark is the flag annotation.
//
// It recedes on a closed row the way priority does rather than going quiet the
// way the fruit does, and for the same mechanical reason read the other way:
// the pennant is a text glyph, so a grey foreground actually reaches it. The
// mark stays because a flag is a note to a reader — "there was something about
// this one" — and that is worth as much on finished work as on open work, once
// it has stopped competing for attention with what is still to do.
func flagMark(t Todo) (string, lipgloss.Style, lipgloss.Style) {
	if !t.Flag {
		return "", lipgloss.NewStyle(), lipgloss.NewStyle()
	}
	if t.closed() {
		return flagGlyph, prioClosedStyle, prioClosedSelStyle
	}
	return flagGlyph, flagStyle, flagStyle
}

// annotMarksFor renders a todo's annotations, packed: one entry per mark this
// row actually draws, in slot order, and nothing for the slots it has nothing to
// say in. A row with no annotations gets an empty slice and spends no cells —
// which is what lets the marks cost only the backlogs that use them, without the
// whole-list bookkeeping a reserved column needs.
func annotMarksFor(t Todo) []annotMark {
	marks := make([]annotMark, 0, len(annotSlots))
	for _, sl := range annotSlots {
		glyph, st, sel := sl.mark(t)
		if glyph == "" {
			continue
		}
		marks = append(marks, annotMark{text: glyph, style: st, selStyle: sel})
	}
	return marks
}
