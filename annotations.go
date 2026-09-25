// annotations.go — the marks a todo wears beside its state.
//
// A row in the list answers several different questions, and they were being
// asked of one column. What state is this prompt in (open, scheduled, frozen,
// done) is exclusive: a todo is in exactly one, and the badge that says so has
// always been one glyph. How much does it matter, how cheap is it and how much
// does it pay are not exclusive, not exclusive of each other, and not exclusive
// of the state either — a critical one-liner is critical *and* a quick win
// *and* worth a lot *and* still open. Those are annotations: independent facts
// a todo carries, each with its own glyph, drawn together in front of the name
// so a marked prompt is spotted at the left edge.
//
// The row therefore reads outward from the cursor as
//
//	❯ ✓ ▲ 🍏 🔷 fix the thing            the prompt's first line
//	  │ └──┴──┴── annotations: what is true about this prompt
//	  └────────── the state badge: which of the three groups it lives in
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
	Value    string // valueLow (the default, "") | valueMedium | valueHigh, see value.go
	Flag     bool   // singled out — see Todo.Flag
	Info     bool   // a note, not work — never dropped, see Todo.Info
	// FlagNote is the flag's optional words. It rides in the set rather than
	// beside it because it is not a mark of its own: it is what this mark says,
	// and every screen that edits the flag edits the two together.
	FlagNote string
}

// annotsOf reads a todo's annotations off it.
func annotsOf(t Todo) annots {
	return annots{Priority: t.Priority, Fruit: t.Fruit, Value: t.valueLevel(), Flag: t.Flag, FlagNote: t.FlagNote, Info: t.Info}
}

// applyTo writes the set back onto a todo, leaving everything else alone.
// The note is dropped with the flag rather than kept for a later re-flag: a
// stored note nothing draws is a fact about the prompt that no screen shows,
// and the next flag would come up wearing words the user did not just write.
func (a annots) applyTo(t *Todo) {
	t.Priority = a.Priority
	t.Fruit = a.Fruit
	t.setValueLevel(a.Value)
	t.Flag = a.Flag
	t.FlagNote = ""
	if a.Flag {
		t.FlagNote = strings.TrimSpace(a.FlagNote)
	}
	// A note is never delivered, so a schedule on one is a promise to do the
	// one thing the mark forbids. It goes as the mark goes up — the way
	// freezing clears it — rather than lingering for fireDueSchedules to skip
	// forever and the row to keep advertising a ◷ that will never fire.
	t.Info = a.Info
	if a.Info {
		t.Schedule = nil
	}
}

// any reports whether anything has actually been said. It is what lets a screen
// stay silent about a prompt nobody has annotated — the CLI's echo — rather
// than announce the defaults on every one.
func (a annots) any() bool {
	return a.Priority != priorityNone || a.Fruit || a.Value != valueLow || a.Flag || a.Info
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
// wearing several marks always reads the same way round. Priority leads because
// it is the one that decides what happens next; the fruit qualifies it ("…and
// it's cheap") and the value mark qualifies it again ("…and it pays").
//
// The two qualifiers are adjacent because together they are one reading — cheap
// *and* valuable is the prompt to pick up next, and a row that separated them
// with the flag would have made the reader assemble that from two ends of the
// group. The form's annotation bar puts the same pair side by side for the same
// reason (annotbar.go).
var annotSlots = []annotSlot{
	{name: "priority", mark: priorityMark, label: priorityAnnotLabel},
	{name: "low-hanging fruit", mark: fruitMark, label: fruitAnnotLabel},
	{name: "value", mark: valueMark, label: valueAnnotLabel},
	// Info sits after the estimates and before the flag. It is a glance-fact
	// like them — no words to stop for — but it reframes the whole row ("this
	// is a note, not work"), so it closes the quick-read group and hands off to
	// the one mark that has to be read.
	{name: "info", mark: infoMark, label: infoAnnotLabel},
	// The flag trails all three. It is the mark whose meaning is written on the
	// prompt rather than carried by the glyph, so it is the one a reader has to
	// stop for — and a stop belongs at the end of the group, after the facts
	// that can be taken in at a glance.
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

// valueAnnotLabel is the value level in words ("medium value"; see
// valueLevelLabel for the wording).
func valueAnnotLabel(t Todo) string {
	return valueLevelLabel(t.valueLevel())
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

// valueMark is the value annotation, one glyph per raised level
// (valueMarkFor) and nothing for low, the default; it goes quiet on a closed
// row exactly as the fruit does. The high step is an
// emoji, which ignores a foreground, so it has no grey to recede into. "This
// one pays" is an argument for picking a prompt up, and there is nothing to
// pick up in the done and frozen tiers.
//
// Medium is text and *could* recede to grey, the way the flag does. It goes
// quiet with the top step instead, because a slot that drew medium on a done
// row but not high would make the done tier look as if only its lesser
// prompts had been rated.
//
// As with the apple, nothing is lost: the level stays on the todo, the form's
// annotation bar still shows it chosen, and the prompt view still spells it
// out — which is why the closed styles come back with the empty glyph rather
// than a bare style, since that screen prints the label in them.
func valueMark(t Todo) (string, lipgloss.Style, lipgloss.Style) {
	v := t.valueLevel()
	if v == valueLow {
		// The default draws nothing, the way priority's none does (value.go).
		return "", lipgloss.NewStyle(), lipgloss.NewStyle()
	}
	if t.closed() {
		return "", prioClosedStyle, prioClosedSelStyle
	}
	glyph, st := valueMarkFor(v)
	return glyph, st, st
}

// The info mark's refusals, shared by the chords (startDrop, beginSchedule)
// and the list menu's dim rows so the two roads say the same words. They name
// the way out — the mark itself — because a refusal that only said "no" would
// leave the reader hunting for which of the row's facts was the obstacle.
//
// infoSendWhy is now the refusal of a SEND TO NOTES (notes.go) rather than of
// a send: an info prompt's Send files it in the notes plugin, and this is what
// the user reads when no notes pane is open. It names both ways out — open a
// notes plugin, or clear the mark and treat the prompt as work after all.
const (
	infoSendWhy     = "no notes plugin open in cats — open GoNotes and send again, or clear ℹ Info to send it to an agent"
	infoScheduleWhy = "that prompt is marked info — a note, not for agents; clear ℹ Info to schedule it"
)

// infoAnnotLabel is the info mark in words. It says what the mark *does*
// ("not for agents") as well as what it is, because on the screens that spell
// the marks out — the prompt view, the CLI echo — the refusal a later send
// meets should not be the first place the reader learns about it.
func infoAnnotLabel(t Todo) string {
	if !t.Info {
		return ""
	}
	return "info — a note, not for agents"
}

// infoMark is the info annotation, drawn as a chip: the glyph on its own blue
// field (see infoGlyph for why a chip rather than a bare letter).
//
// It was grey, on the argument that every coloured mark on the row is a claim
// about work and a note is the absence of one. That was right about the claim
// and wrong about the cost: the mark's job is to stop an info row being mistaken
// for work at a glance, and a grey letter beside two emoji was not seen at a
// glance at all. The field is the same on the highlighted row — it is the
// mark's own surface, not the row's, so the row renderer leaves it alone (see
// fuzzylist.view).
//
// On a closed row the field drops away and the letter goes to the closed-row
// grey, still italic so it is recognisably the same mark. A filed-away note is
// still worth recognising, but no longer worth the loudest cell on its row.
func infoMark(t Todo) (string, lipgloss.Style, lipgloss.Style) {
	if !t.Info {
		return "", lipgloss.NewStyle(), lipgloss.NewStyle()
	}
	if t.closed() {
		return infoGlyph, prioClosedStyle.Italic(true), prioClosedSelStyle.Italic(true)
	}
	return infoGlyph, infoChipStyle, infoChipStyle
}

// withInfoChips renders text in st, except that each info glyph in it is drawn
// as the chip. It is for the screens that write the glyph inside a label — the
// form's annotation bar, the list's context menu — where the label is one
// string measured as one string, but a single style over all of it would paint
// the blue field across the words too.
//
// The pieces are sibling renders rather than a chip nested inside st: an inner
// render ends in a reset, which would drop st's field for the rest of the label
// (the same reason renderChipDimHint splits its chip). Splitting on the glyph
// keeps the width identical to lipgloss.Width(text), so whatever measured the
// label — a hit-test span, a menu's box — still lines up with what is drawn.
//
//	st  "☑ "   chip "ｉ"   st " Info"
//	   └─ st's field ─┘└ blue ┘└─ st's field ─┘
//
// An underline on st (the bar's keyboard cursor) is carried onto the chip, so
// the cursor still reads as one run under the whole segment.
func withInfoChips(st lipgloss.Style, text string) string {
	if !strings.Contains(text, infoGlyph) {
		return st.Render(text)
	}
	chip := infoChipStyle.Underline(st.GetUnderline())
	var b strings.Builder
	for i, part := range strings.Split(text, infoGlyph) {
		if i > 0 {
			b.WriteString(chip.Render(infoGlyph))
		}
		if part != "" {
			b.WriteString(st.Render(part))
		}
	}
	return b.String()
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
