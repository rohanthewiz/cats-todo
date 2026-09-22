// annotbar.go — the form's annotation bar: a segmented menu between the title
// and the prompt body where a prompt's own marks are set.
//
// The annotations used to live as the first two rows of the ⚙ session panel,
// above a seam — accurate, but a screen away: the marks describe the prompt,
// the panel describes the session that will read it, and the one screen where a
// prompt is actually written showed neither. The bar puts them on the form
// itself, where the title they qualify is, as one horizontal line of segments:
//
//	☐ 🍏 Quick win   ☐ 💎 High value   Priority  (•) none   ( ) △ high   ( ) ▲ critical   ☐ ℹ Info   ☐ ⚑ Flag
//
// ☐ ℹ Info marks the prompt as a note rather than work — something to collect
// into a notes program later, never to hand to an agent (Todo.Info). It sits
// with the flag at the tail because both are about how to *read* the prompt
// rather than how to rank it, and before the flag so the flag stays the last
// segment, the one that opens something beneath it.
//
// Four checkboxes and one radio group, because that is what the five facts
// are: the fruit and the gem are each independent ("cheap, whatever else is
// true"; "worth a lot, whatever else is true"), and the priority is exactly one
// of three levels; info and the flag are independent again ("this is a note,
// not work"; "and there is something to say about it"). The glyphs each
// segment carries — 🍏, 💎, △, ▲, ℹ, ⚑ — are the
// marks the choice will draw on the list row, so the bar teaches the legend at
// the moment the mark is made.
//
// ☐ 💎 High value sits immediately beside ☐ 🍏 Quick win because the two are
// one estimate read from both ends: the apple is what a prompt costs, the gem
// is what it pays. Ticking one invites the question the other answers, and a
// hand that has just answered "cheap" is one ←/→ away from answering "and worth
// it". Priority is a different question — how much it matters *now* — so it
// keeps the radios to itself on the far side of its own label.
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
	annotSegValue               // the High value checkbox, its other half
	annotSegPrioNone            // the priority radios, one per level, in
	annotSegPrioHigh            // the order they escalate — the same walk
	annotSegPrioCritical        // the old panel row cycled
	annotSegInfo                // the ℹ Info checkbox — a note, not work
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

// annotBarTier is one way of spelling the whole bar: the segment texts, the
// gap between them, and the group label — empty on a tier that cannot afford
// it. Grouping the three means a tier is chosen once, as a unit, instead of the
// gap and the wording being narrowed by separate conditions that could each be
// true at a different width.
type annotBarTier struct {
	texts   [annotSegCount]string
	gap     int
	divider string
}

// width is what this tier costs, measured the way annotBarLayout spends it, so
// the measurement and the drawing cannot disagree about when to concede.
func (t annotBarTier) width() int {
	w := 0
	for i, text := range t.texts {
		if i > 0 {
			w += t.gap
		}
		w += lipgloss.Width(text)
	}
	if t.divider != "" {
		w += lipgloss.Width(t.divider) + t.gap
	}
	return w
}

// annotBarTiers is the bar spelled five ways, widest first, for the pane to
// choose from.
//
// The bar must shrink rather than wrap, because it sits on a row every click is
// hit-tested against (formAnnotRow) and a bar that wrapped would put the prompt
// editor one line down from where the pointer finds it. So it concedes in the
// order every chip bar in this program concedes in — words, then gaps, then
// bare glyphs — and never drops a segment:
//
//	full     ☐ 🍏 Quick win   ☐ 💎 High value   Priority  (•) none  …    106 cells
//	snug     ☐ 🍏 Quick win  ☐ 💎 High value  Priority  (•) none  …        99 cells
//	compact  ☐ 🍏   ☐ 💎   (•) –   ( ) △   ( ) ▲   ☐ ℹ   ☐ ⚑               41 cells
//	tight    ☐🍏  ☐💎  (•)–  ( )△  ( )▲  ☐ℹ  ☐⚑                           34 cells
//	tightest ☐🍏 ☐💎 (•)– ( )△ ( )▲ ☐ℹ ☐⚑                                 28 cells
//
// The full tier spells the levels out. The compact one gives up the words,
// which the marks themselves still teach, and keeps every state glyph — the
// boxes, the holes, the marks — because those *are* the answers and a bar that
// dropped them would be a bar that could not be read. The tight tier gives up
// the space inside each segment, so a box sits against the mark it is the state
// of; that reads as one token rather than two, which is the right reading
// anyway, and it is the last cell the bar has to give.
//
// The narrowest pane this form is drawn in at all is 30 cells — pinned by
// TestAnnotBarFitsNarrowPanes. The sixth segment (the gem) used up the tight
// tier's last slack at exactly 30; the seventh (ℹ Info) is what the tightest
// tier exists for. It gives up the second cell of each gap, the only cells
// left that are not a state glyph, and comes to 28. The tight tier is kept
// above it rather than replaced because a two-cell gap is easier to aim a
// click between on the panes that can still afford it. An eighth mark would
// have nothing left to give but a segment, which the bar never drops.
func (m model) annotBarTiers() [5]annotBarTier {
	a := m.formAnnots
	check := func(on bool) string {
		if on {
			return "☑"
		}
		return "☐"
	}
	box, valueBox, infoBox, flagBox := check(a.Fruit), check(a.HighValue), check(a.Info), check(a.Flag)
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
	// "none" gives up its word with the rest of them on the narrow tiers, and
	// takes a dash in its place rather than standing as a bare hole: it is the
	// one radio with no mark of its own, so an unlabelled "( )" would be the
	// only segment on the bar saying nothing at all. The dash is the mark for
	// "nothing said", which is exactly the level.
	tightest := [annotSegCount]string{
		annotSegFruit:        box + fruitGlyph,
		annotSegValue:        valueBox + valueGlyph,
		annotSegPrioNone:     radio(priorityNone) + "–",
		annotSegPrioHigh:     radio(priorityHigh) + prioHighGlyph,
		annotSegPrioCritical: radio(priorityCritical) + prioCriticalGlyph,
		annotSegInfo:         infoBox + infoGlyph,
		annotSegFlag:         flagBox + flagGlyph,
	}
	full := [annotSegCount]string{
		annotSegFruit:        box + " " + fruitGlyph + " Quick win",
		annotSegValue:        valueBox + " " + valueGlyph + " High value",
		annotSegPrioNone:     radio(priorityNone) + " none",
		annotSegPrioHigh:     radio(priorityHigh) + " " + prioHighGlyph + " high",
		annotSegPrioCritical: radio(priorityCritical) + " " + prioCriticalGlyph + " critical",
		annotSegInfo:         infoBox + " " + infoGlyph + " Info",
		annotSegFlag:         flagBox + " " + flagGlyph + " Flag",
	}
	return [5]annotBarTier{
		{texts: full, gap: 3, divider: "Priority"},
		// The same words a cell closer together. The ℹ Info segment pushed
		// the widest tier past 100 cells, the width a form most often gets;
		// narrowing the gaps first keeps the words there, which is the order
		// of concession above — the gap is the cheaper thing to give.
		{texts: full, gap: 2, divider: "Priority"},
		{
			texts: [annotSegCount]string{
				annotSegFruit:        box + " " + fruitGlyph,
				annotSegValue:        valueBox + " " + valueGlyph,
				annotSegPrioNone:     radio(priorityNone) + " –",
				annotSegPrioHigh:     radio(priorityHigh) + " " + prioHighGlyph,
				annotSegPrioCritical: radio(priorityCritical) + " " + prioCriticalGlyph,
				annotSegInfo:         infoBox + " " + infoGlyph,
				annotSegFlag:         flagBox + " " + flagGlyph,
			},
			gap: 2,
		},
		{
			texts: [annotSegCount]string{
				annotSegFruit:        box + fruitGlyph,
				annotSegValue:        valueBox + valueGlyph,
				annotSegPrioNone:     radio(priorityNone) + "–",
				annotSegPrioHigh:     radio(priorityHigh) + prioHighGlyph,
				annotSegPrioCritical: radio(priorityCritical) + prioCriticalGlyph,
				annotSegInfo:         infoBox + infoGlyph,
				annotSegFlag:         flagBox + flagGlyph,
			},
			gap: 2,
		},
		{texts: tightest, gap: 1},
	}
}

// annotBarLayout lays the bar out for the current pane: the live segments
// with their spans, and the finished line.
//
// The widest tier that fits wins; the narrowest is the floor, drawn even when
// the pane cannot hold it, because a bar with a segment missing would be worse
// than one that overflows — the overflow is visible, a silently absent control
// is not. A pane of unknown width (m.width == 0, before the first resize) takes
// the full tier, which is what it will almost certainly have room for.
func (m model) annotBarLayout() (segs [annotSegCount]annotSeg, line string) {
	tiers := m.annotBarTiers()
	tier := tiers[len(tiers)-1]
	for _, t := range tiers {
		if m.width <= 0 || t.width() <= m.width {
			tier = t
			break
		}
	}

	var b strings.Builder
	x := 0
	for i, text := range tier.texts {
		if i > 0 {
			b.WriteString(strings.Repeat(" ", tier.gap))
			x += tier.gap
		}
		// The group label sits between the checkboxes and the radios it names.
		// It is inert — part of the layout, not a segment — so it takes no
		// span and a click on it presses nothing.
		if i == annotSegPrioNone && tier.divider != "" {
			b.WriteString(promptStyle.Render(tier.divider))
			b.WriteString(strings.Repeat(" ", tier.gap))
			x += lipgloss.Width(tier.divider) + tier.gap
		}
		st := m.annotSegStyle(i)
		w := lipgloss.Width(text)
		segs[i] = annotSeg{text: text, style: st, start: x, end: x + w}
		b.WriteString(st.Render(text))
		x += w
	}
	return segs, b.String()
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
	case i == annotSegValue && a.HighValue:
		st = nameSelStyle
	case i == annotSegPrioNone && a.Priority == priorityNone:
		st = nameSelStyle
	case i == annotSegPrioHigh && a.Priority == priorityHigh:
		st = prioHighStyle
	case i == annotSegPrioCritical && a.Priority == priorityCritical:
		st = prioCriticalStyle
	case i == annotSegInfo && a.Info:
		st = infoStyle
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
	case annotSegValue:
		m.formAnnots.HighValue = !m.formAnnots.HighValue
	case annotSegPrioNone:
		m.formAnnots.Priority = priorityNone
	case annotSegPrioHigh:
		m.formAnnots.Priority = priorityHigh
	case annotSegPrioCritical:
		m.formAnnots.Priority = priorityCritical
	case annotSegInfo:
		m.formAnnots.Info = !m.formAnnots.Info
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
