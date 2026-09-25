// annotbar.go — the form's annotation bar: a segmented menu between the title
// and the prompt body where a prompt's own marks are set.
//
// The annotations used to live as the first two rows of the ⚙ session panel,
// above a seam — accurate, but a screen away: the marks describe the prompt,
// the panel describes the session that will read it, and the one screen where a
// prompt is actually written showed neither. The bar puts them on the form
// itself, where the title they qualify is, as one horizontal line of segments:
//
//	☐ 🍏 Quick win │ Value (•) none ( ) ◇ low ( ) ◆ medium ( ) 🔷 high │ Priority (•) none ( ) △ high ( ) ▲ critical │ ☐ ｉ Info ☐ ⚑ Flag
//
// ☐ ｉ Info marks the prompt as a note rather than work — something to collect
// into a notes program later, never to hand to an agent (Todo.Info). It sits
// with the flag at the tail because both are about how to *read* the prompt
// rather than how to rank it, and before the flag so the flag stays the last
// segment, the one that opens something beneath it.
//
// Three checkboxes and two radio groups, because that is what the five facts
// are: the fruit is independent ("cheap, whatever else is true"), the value is
// exactly one of three levels and so is the priority; info and the flag
// are independent again ("this is a note, not work"; "and there is something
// to say about it"). The glyphs each segment carries — 🍏, ◇ ◆ 🔷, △ ▲, ｉ, ⚑ —
// are the marks the choice will draw on the list row, so the bar teaches the
// legend at the moment the mark is made. A rule (│) divides the four groups;
// see annotGroupStart for why the narrow tiers need it.
//
// The Value radios sit immediately beside ☐ 🍏 Quick win because the two are
// one estimate read from both ends: the apple is what a prompt costs, the
// value is what it pays. Ticking one invites the question the other answers, and a
// hand that has just answered "cheap" is one ←/→ away from answering "and worth
// it". Priority is a different question — how much it matters *now* — so it
// keeps its own radios on the far side of its own label.
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
// the radio groups keeps each group's label adjacent to the holes it names.
const (
	annotSegFruit        = iota // the Quick win checkbox
	annotSegValueLow            // the value radios, one per level, in the order
	annotSegValueMedium         // they escalate (valueLevels) — the fruit's
	annotSegValueHigh           // other half: what the prompt pays
	annotSegPrioNone            // the priority radios, one per level, in
	annotSegPrioHigh            // the order they escalate — the same walk
	annotSegPrioCritical        // the old panel row cycled
	annotSegInfo                // the ｉ Info checkbox — a note, not work
	annotSegFlag                // the ⚑ Flag checkbox, and its note field
	annotSegCount
)

// annotSegValueLevel maps the value radios onto the levels they set, so the
// segment order and the escalation order cannot drift apart (the table the
// list menu keeps for its priority rows, listMenuPrio).
var annotSegValueLevel = map[int]string{
	annotSegValueLow:    valueLow,
	annotSegValueMedium: valueMedium,
	annotSegValueHigh:   valueHigh,
}

// annotGroupStart names the segments that open a group, and the group's word
// ("" for a group with none). A separator is drawn in front of each, so the
// bar reads as four groups — the fruit, the value radios, the priority radios,
// and the two reading marks — rather than as one run of ten segments:
//
//	☐ 🍏 Quick win │ Value (•) ◇ low … ( ) 🔷 high │ Priority (•) none … │ ☐ ｉ Info  ☐ ⚑ Flag
//
// The separator is what the narrow tiers lean on. Once the words are gone, two
// radio groups side by side are a run of six holes whose only boundary is
// which glyphs follow them — and the diamonds and triangles are both shapes
// that fill as they escalate. The label says which question a group answers
// on the tiers that can afford it; the rule says where one group ends on
// every tier.
var annotGroupStart = map[int]string{
	annotSegValueLow: "Value",
	annotSegPrioNone: "Priority",
	annotSegInfo:     "",
}

// annotSepGlyph is the rule between groups: a light vertical, one cell, drawn
// in the faint grey so it divides without competing with the marks.
const annotSepGlyph = "│"

var annotSepStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colFaint))

// annotSeg is one drawn segment: its text, the style it is drawn in, and the
// half-open column span it occupies — one description for drawing and
// hit-testing both, so a segment's click target cannot disagree with the
// glyphs the eye sees (the contract every chip bar in this program keeps).
type annotSeg struct {
	text       string
	style      lipgloss.Style
	start, end int
}

// annotBarTier is one way of spelling the whole bar. Everything that varies
// with the width is in here and chosen once, as a unit, instead of the gap and
// the wording being narrowed by separate conditions that could each be true at
// a different width.
type annotBarTier struct {
	texts [annotSegCount]string
	gap   int
	// labels draws each group's word after its separator.
	labels bool
	// bare is the last resort: the radios lose their holes and stand as their
	// glyphs alone, the chosen one lit (drawn in reverse), and the separators
	// lose the gaps around them. See annotBarTiers.
	bare bool
}

// sepWidth is what a group boundary costs in this tier, measured the way
// annotBarLayout spends it: the rule with a gap either side (none in the bare
// tier), then the group's word and one more gap when the tier has labels.
func (t annotBarTier) sepWidth(label string) int {
	if t.bare {
		return lipgloss.Width(annotSepGlyph)
	}
	w := t.gap + lipgloss.Width(annotSepGlyph) + t.gap
	if t.labels && label != "" {
		w += lipgloss.Width(label) + t.gap
	}
	return w
}

// width is what this tier costs, measured the way annotBarLayout spends it, so
// the measurement and the drawing cannot disagree about when to concede.
func (t annotBarTier) width() int {
	w := 0
	for i, text := range t.texts {
		if i > 0 {
			if label, ok := annotGroupStart[i]; ok {
				w += t.sepWidth(label)
			} else {
				w += t.gap
			}
		}
		w += lipgloss.Width(text)
	}
	return w
}

// annotSpelling is how much of each segment a tier prints: the checkboxes'
// words, the radios' words, the space between a state glyph and its mark, and
// whether the radios keep their holes at all.
type annotSpelling struct {
	checkWords, radioWords, inner, bare bool
}

// annotBarTiers is the bar spelled eight ways, widest first, for the pane to
// choose from.
//
// The bar must shrink rather than wrap, because it sits on a row every click is
// hit-tested against (formAnnotRow) and a bar that wrapped would put the prompt
// editor one line down from where the pointer finds it. So it concedes in the
// order every chip bar in this program concedes in — words, then gaps, then
// bare glyphs — and never drops a segment. Widths with every box empty:
//
//	full      ☐ 🍏 Quick win   │   Value   (•) ◇ low   ( ) ◆ medium …     150
//	snug      the same, a cell closer together                           137
//	labelled  ☐ 🍏 Quick win  │  Value  (•) ◇  ( ) ◆  ( ) 🔷  │ …         104
//	legend    ☐ 🍏  │  Value  (•) ◇  ( ) ◆  ( ) 🔷  │  Priority  (•) – …   84
//	compact   ☐ 🍏  │  (•) ◇  ( ) ◆  ( ) 🔷  │  (•) –  ( ) △ …            67
//	tight     ☐🍏  │  (•)◇  ( )◆  ( )🔷  │  (•)–  ( )△  ( )▲ …            58
//	tightest  ☐🍏 │ (•)◇ ( )◆ ( )🔷 │ (•)– ( )△ ( )▲ │ ☐ｉ ☐⚑             47
//	bare      ☐🍏│◇ ◆ 🔷│– △ ▲│☐ｉ ☐⚑                                     23
//
// The radio words go before the checkbox words because the radios' glyphs
// already say their level (◇ ◆ 🔷, △ ▲), where a checkbox's glyph alone does
// not say what ticking it claims until the legend is learned. The group labels
// outlast both, down to the 84-cell tier, so a form in the common 100-cell pane
// still says which row of holes is Value and which is Priority.
//
// The bare tier exists for the narrowest pane the form is drawn in, 30 cells
// (pinned by TestAnnotBarFitsNarrowPanes). Three value radios left nothing of
// the old tightest tier's slack, so this one gives up the holes themselves: a
// radio is its glyph, and the chosen one is drawn in reverse, a lit key in a
// row of unlit ones. That is still a state glyph the eye can read without
// colour, which is the one thing no tier gives up. The boxes stay, because a
// checkbox has no other way to show that it is ticked.
func (m model) annotBarTiers() [8]annotBarTier {
	spell := func(sp annotSpelling) [annotSegCount]string {
		return m.annotSegTexts(sp)
	}
	return [8]annotBarTier{
		{texts: spell(annotSpelling{checkWords: true, radioWords: true, inner: true}), gap: 3, labels: true},
		{texts: spell(annotSpelling{checkWords: true, radioWords: true, inner: true}), gap: 2, labels: true},
		{texts: spell(annotSpelling{checkWords: true, inner: true}), gap: 2, labels: true},
		{texts: spell(annotSpelling{inner: true}), gap: 2, labels: true},
		{texts: spell(annotSpelling{inner: true}), gap: 2},
		{texts: spell(annotSpelling{}), gap: 2},
		{texts: spell(annotSpelling{}), gap: 1},
		{texts: spell(annotSpelling{bare: true}), gap: 1, bare: true},
	}
}

// annotSegTexts spells every segment the way sp says, from one description of
// each: its state glyph (a box or a hole), its mark, and its word.
//
// A radio with no mark of its own — priority's "none" — takes a dash once
// its word is gone, rather than standing as a bare hole: an unlabelled "( )"
// would be the only segment on the bar saying nothing at all. The dash is the
// mark for "nothing said", which is exactly the level.
func (m model) annotSegTexts(sp annotSpelling) [annotSegCount]string {
	a := m.formAnnots
	check := func(on bool) string {
		if on {
			return "☑"
		}
		return "☐"
	}
	// The radio that is filled. An exact match on purpose: a hand-edited
	// backlog can hold anything, including the retired priority "low", and a
	// value this program cannot read is not a level it should claim was chosen
	// — so an unknown value fills no hole, exactly as it draws no mark on the
	// row (see priorityMark), and choosing any segment replaces it.
	radio := func(on bool) string {
		if on {
			return "(•)"
		}
		return "( )"
	}
	type part struct {
		state, glyph, word string
		isRadio            bool
	}
	valueGlyph := func(v string) string { g, _ := valueMarkFor(v); return g }
	parts := [annotSegCount]part{
		annotSegFruit:        {check(a.Fruit), fruitGlyph, "Quick win", false},
		annotSegValueLow:     {radio(a.Value == valueLow), valueGlyph(valueLow), "low", true},
		annotSegValueMedium:  {radio(a.Value == valueMedium), valueGlyph(valueMedium), "medium", true},
		annotSegValueHigh:    {radio(a.Value == valueHigh), valueGlyph(valueHigh), "high", true},
		annotSegPrioNone:     {radio(a.Priority == priorityNone), "", "none", true},
		annotSegPrioHigh:     {radio(a.Priority == priorityHigh), prioHighGlyph, "high", true},
		annotSegPrioCritical: {radio(a.Priority == priorityCritical), prioCriticalGlyph, "critical", true},
		annotSegInfo:         {check(a.Info), infoGlyph, "Info", false},
		annotSegFlag:         {check(a.Flag), flagGlyph, "Flag", false},
	}
	var texts [annotSegCount]string
	for i, p := range parts {
		words := sp.checkWords
		if p.isRadio {
			words = sp.radioWords
		}
		glyph := p.glyph
		if glyph == "" && !words {
			glyph = "–"
		}
		inner := ""
		if sp.inner {
			inner = " "
		}
		switch {
		case sp.bare && p.isRadio:
			texts[i] = glyph
		case p.glyph == "" && words:
			texts[i] = p.state + " " + p.word // "(•) none": the word is the mark
		case words:
			texts[i] = p.state + inner + glyph + " " + p.word
		default:
			texts[i] = p.state + inner + glyph
		}
	}
	return texts
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
	gap := strings.Repeat(" ", tier.gap)
	for i, text := range tier.texts {
		if i > 0 {
			// A group boundary is the rule (and, on a labelled tier, the
			// group's word) in place of a plain gap. It is inert — part of the
			// layout, not a segment — so it takes no span and a click on it
			// presses nothing.
			if label, ok := annotGroupStart[i]; ok {
				if !tier.bare {
					b.WriteString(gap)
				}
				b.WriteString(annotSepStyle.Render(annotSepGlyph))
				if !tier.bare {
					b.WriteString(gap)
				}
				if tier.labels && label != "" {
					b.WriteString(promptStyle.Render(label))
					b.WriteString(gap)
				}
				x += tier.sepWidth(label)
			} else {
				b.WriteString(gap)
				x += tier.gap
			}
		}
		st := m.annotSegStyle(i, tier.bare)
		w := lipgloss.Width(text)
		segs[i] = annotSeg{text: text, style: st, start: x, end: x + w}
		// Through withInfoChips so the ｉ Info segment's glyph wears its chip
		// whether the box is checked or not — the apple and the high diamond
		// paint themselves in both states too, and the bar is where the row's
		// legend is learned.
		b.WriteString(withInfoChips(st, text))
		x += w
	}
	return segs, b.String()
}

// annotSegStyle is how one segment is drawn. What is chosen takes its own
// mark's hue — critical its red, high priority its yellow, each value level
// its diamond's colour (valueChosenStyle), the same mapping the list row
// draws — so the bar teaches the legend rather than a private one; what is
// not chosen recedes to the greys, still legibly an option. The segment under
// the keyboard's cursor is underlined while the bar holds the form's focus:
// underline rather than a moving glyph, because a caret that shifted the
// layout would move every click target with it.
//
// bare is the tier whose radios have no holes (see annotBarTiers). There the
// chosen radio is also drawn in reverse, because with the hole gone the hue
// would be the only thing saying which one is chosen, and a hue alone is not
// enough (the reason the triangles differ by fill, not just colour).
func (m model) annotSegStyle(i int, bare bool) lipgloss.Style {
	a := m.formAnnots
	st := descStyle
	chosenRadio := false
	switch {
	case i == annotSegFruit && a.Fruit:
		st = nameSelStyle
	case annotSegValueLevel[i] == a.Value && isValueSeg(i):
		st, chosenRadio = valueChosenStyle(a.Value), true
	case i == annotSegPrioNone && a.Priority == priorityNone:
		st, chosenRadio = nameSelStyle, true
	case i == annotSegPrioHigh && a.Priority == priorityHigh:
		st, chosenRadio = prioHighStyle, true
	case i == annotSegPrioCritical && a.Priority == priorityCritical:
		st, chosenRadio = prioCriticalStyle, true
	case i == annotSegInfo && a.Info:
		st = infoStyle
	case i == annotSegFlag && a.Flag:
		st = flagStyle
	}
	if bare && chosenRadio {
		st = st.Reverse(true)
	}
	if m.formFocus == formFieldAnnots && i == m.annotCursor {
		st = st.Underline(true)
	}
	return st
}

// isValueSeg reports whether segment i is one of the value radios.
func isValueSeg(i int) bool {
	_, ok := annotSegValueLevel[i]
	return ok
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
	case annotSegValueLow, annotSegValueMedium, annotSegValueHigh:
		m.formAnnots.Value = annotSegValueLevel[i]
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
