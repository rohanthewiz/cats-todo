// annotbar.go — the form's annotation bar: a segmented menu between the title
// and the prompt body where a prompt's own marks are set.
//
// The annotations used to live as the first two rows of the ⚙ session panel,
// above a seam — accurate, but a screen away: the marks describe the prompt,
// the panel describes the session that will read it, and the one screen where a
// prompt is actually written showed neither. The bar puts them on the form
// itself, where the title they qualify is, as one horizontal line of segments:
//
//	☐ 🍏 Quick win   Priority  (•) none   ( ) △ high   ( ) ▲ critical   ☐ ⚑ Flag
//
// Two checkboxes and one radio group, because that is what the three facts are:
// the fruit is independent ("cheap, whatever else is true"), and the priority
// is exactly one of three levels; the flag is independent again ("and there is
// something to say about it"). The glyphs each segment carries — 🍏, △, ▲, ⚑ —
// are the marks the choice will draw on the list row, so the bar teaches the
// legend at the moment the mark is made.
//
// The flag trails the radios rather than joining the fruit at the head, because
// it is the one segment that is not the whole of its own answer: pressing it
// raises a note field on the line below (formFieldFlagNote, ui.go), and a
// control that opens something belongs at the end of the row it opens it under
// rather than in the middle of a group the eye is still reading across.
//
// The bar edits m.formAnnots, the same by-value copy the panel rows used to
// edit, so every promise the form makes still holds: nothing reaches the
// backlog until the form is saved, and a cancelled edit changes nothing.
package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The bar's segments, left to right. The cursor (m.annotCursor) is an index
// into this set, so the numbering is the layout and nothing else. The fruit
// leads for the reason its column trails on the list row inverted: here the
// checkbox is the one segment that is whole on its own, and putting it before
// the radio group keeps "Priority" adjacent to the holes it names.
const (
	annotSegFruit        = iota // the Quick win checkbox
	annotSegPrioNone            // the priority radios, one per level, in
	annotSegPrioHigh            // the order they escalate — the same walk
	annotSegPrioCritical        // the old panel row cycled
	annotSegFlag                // the ⚑ Flag checkbox, and its note field
	annotSegCount
)

// annotSeg is one drawn segment: its text, the style it is drawn in, and the
// half-open column span it occupies — one description for drawing and
// hit-testing both, so a segment's click target cannot disagree with the
// glyphs the eye sees (the contract every chip bar in this program keeps).
type annotSeg struct {
	text       string
	style      lipgloss.Style
	start, end int
}

// annotBarLayout lays the bar out for the current pane: the four live
// segments with their spans, and the finished line.
//
// Two tiers, like the button bars, and for the same reason — the bar must
// shrink rather than wrap, because it sits on a row every click is hit-tested
// against (formAnnotRow), and a bar that wrapped would put the prompt editor
// one line down from where the pointer finds it. The full tier spells the
// levels out; the compact one keeps the state glyphs (the boxes, the holes, the
// marks) and gives up the words, which the marks themselves still teach. The
// compact tier comes to 30 cells with all five segments on it, which is the
// narrowest pane this form is drawn in at all.
func (m model) annotBarLayout() (segs [annotSegCount]annotSeg, line string) {
	a := m.formAnnots
	box := "☐"
	if a.Fruit {
		box = "☑"
	}
	flagBox := "☐"
	if a.Flag {
		flagBox = "☑"
	}
	// The radio that is filled. An exact match on purpose: a hand-edited
	// backlog can hold anything, including the retired "low", and a value this
	// program cannot read is not a level it should claim was chosen — so an
	// unknown value fills no hole, exactly as it draws no mark on the row
	// (see priorityMark), and choosing any segment replaces it.
	radio := func(level string) string {
		if a.Priority == level {
			return "(•)"
		}
		return "( )"
	}

	texts := [annotSegCount]string{
		annotSegFruit:        box + " " + fruitGlyph + " Quick win",
		annotSegPrioNone:     radio(priorityNone) + " none",
		annotSegPrioHigh:     radio(priorityHigh) + " " + prioHighGlyph + " high",
		annotSegPrioCritical: radio(priorityCritical) + " " + prioCriticalGlyph + " critical",
		annotSegFlag:         flagBox + " " + flagGlyph + " Flag",
	}
	gap, divider := 3, "Priority"
	if m.width > 0 && annotBarWidth(texts, gap, divider) > m.width {
		texts = [annotSegCount]string{
			annotSegFruit: box + " " + fruitGlyph,
			// "none" gives up its word with the rest of them, and takes a dash
			// in its place rather than standing as a bare hole: it is the one
			// radio with no mark of its own, so an unlabelled "( )" would be
			// the only segment on the compact bar saying nothing at all. The
			// dash is the mark for "nothing said", which is exactly the level.
			annotSegPrioNone:     radio(priorityNone) + " –",
			annotSegPrioHigh:     radio(priorityHigh) + " " + prioHighGlyph,
			annotSegPrioCritical: radio(priorityCritical) + " " + prioCriticalGlyph,
			annotSegFlag:         flagBox + " " + flagGlyph,
		}
		gap, divider = 2, ""
	}

	var b strings.Builder
	x := 0
	for i, text := range texts {
		if i > 0 {
			b.WriteString(strings.Repeat(" ", gap))
			x += gap
		}
		// The group label sits between the checkbox and the radios it names.
		// It is inert — part of the layout, not a segment — so it takes no
		// span and a click on it presses nothing.
		if i == annotSegPrioNone && divider != "" {
			b.WriteString(promptStyle.Render(divider))
			b.WriteString(strings.Repeat(" ", gap))
			x += lipgloss.Width(divider) + gap
		}
		st := m.annotSegStyle(i)
		w := lipgloss.Width(text)
		segs[i] = annotSeg{text: text, style: st, start: x, end: x + w}
		b.WriteString(st.Render(text))
		x += w
	}
	return segs, b.String()
}

// annotBarWidth is what the full tier would cost, measured the way the layout
// spends it, so the two cannot disagree about when to concede.
func annotBarWidth(texts [annotSegCount]string, gap int, divider string) int {
	w := 0
	for i, t := range texts {
		if i > 0 {
			w += gap
		}
		w += lipgloss.Width(t)
	}
	if divider != "" {
		w += lipgloss.Width(divider) + gap
	}
	return w
}

// annotSegStyle is how one segment is drawn. What is chosen takes its own
// mark's hue — critical its red, high its yellow, the same mapping the list
// row draws — so the bar teaches the legend rather than a private one; what is
// not chosen recedes to the greys, still legibly an option. The segment under
// the keyboard's cursor is underlined while the bar holds the form's focus:
// underline rather than a moving glyph, because a caret that shifted the
// layout would move every click target with it.
func (m model) annotSegStyle(i int) lipgloss.Style {
	a := m.formAnnots
	st := descStyle
	switch {
	case i == annotSegFruit && a.Fruit:
		st = nameSelStyle
	case i == annotSegPrioNone && a.Priority == priorityNone:
		st = nameSelStyle
	case i == annotSegPrioHigh && a.Priority == priorityHigh:
		st = prioHighStyle
	case i == annotSegPrioCritical && a.Priority == priorityCritical:
		st = prioCriticalStyle
	case i == annotSegFlag && a.Flag:
		st = flagStyle
	}
	if m.formFocus == formFieldAnnots && i == m.annotCursor {
		st = st.Underline(true)
	}
	return st
}

// annotBar renders the bar for viewForm.
func (m model) annotBar() string {
	_, line := m.annotBarLayout()
	return line
}

// activateAnnotSeg presses one segment: the checkbox toggles, a radio takes
// the level outright. Choosing an already-filled radio keeps it — radios do
// not un-choose — and "none" is a hole of its own rather than the absence of
// one, so clearing a level is the same gesture as setting it.
// The flag is the one segment with something underneath it: ticking it raises
// the note field on the line below (setFormFlag, ui.go), and clearing it takes
// the field and its words away again.
func (m *model) activateAnnotSeg(i int) tea.Cmd {
	switch i {
	case annotSegFruit:
		m.formAnnots.Fruit = !m.formAnnots.Fruit
	case annotSegPrioNone:
		m.formAnnots.Priority = priorityNone
	case annotSegPrioHigh:
		m.formAnnots.Priority = priorityHigh
	case annotSegPrioCritical:
		m.formAnnots.Priority = priorityCritical
	case annotSegFlag:
		return m.setFormFlag(!m.formAnnots.Flag)
	}
	return nil
}

// pressAnnotSeg is a segment pressed from the keyboard: the state change, and
// then the keys, which the flag moves. Ticking ⚑ Flag puts the caret straight
// into the note it just opened, because the gesture is one thought — "flag this,
// because…" — and a field that appeared but had to be tabbed to would break it
// in half. Nothing else on the bar moves the focus: the other three segments are
// whole answers on their own, and the walk should stay where the hand left it.
func (m model) pressAnnotSeg(i int) (tea.Model, tea.Cmd) {
	flagged := m.formAnnots.Flag
	cmd := m.activateAnnotSeg(i)
	if i == annotSegFlag && !flagged && m.formAnnots.Flag {
		return m, m.focusForm(formFieldFlagNote)
	}
	return m, cmd
}

// updateAnnotBar handles the keys while the bar holds the form's focus: ←/→
// walk the segments, space presses the one under the cursor (enter does too,
// handled with the other enter meanings in updateForm). Everything else is
// deliberately inert rather than forwarded — there is no field under these
// keys, and a keystroke that typed into the prompt from the annotation bar
// would put text where the eye is not.
func (m model) updateAnnotBar(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left":
		if m.annotCursor > 0 {
			m.annotCursor--
		}
	case "right":
		if m.annotCursor < annotSegCount-1 {
			m.annotCursor++
		}
	case " ", "space":
		return m.pressAnnotSeg(m.annotCursor)
	}
	return m, nil
}

// clickAnnotBar presses the segment under the pointer, if it is on one. The
// annotation cursor parks there too, so a later tab onto the bar resumes from
// the segment the hand last used — but the form's focus stays where it was:
// like the toolbar's chips, these are pointer targets, and a click that stole
// the keys from a half-typed field would cost more than it saved.
func (m model) clickAnnotBar(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	segs, _ := m.annotBarLayout()
	for i, seg := range segs {
		if msg.X >= seg.start && msg.X < seg.end {
			m.annotCursor = i
			// The focus deliberately does not follow the pointer here (see the
			// doc comment) — the one cmd this can return is setFormFlag's, and
			// that one only fires when clearing the flag stranded the keys in
			// the note field it just took away.
			return m, m.activateAnnotSeg(i)
		}
	}
	return m, nil
}
