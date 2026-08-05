package main

import "charm.land/lipgloss/v2"

// The muted green palette, shared with cats itself (internal/config's
// defaultColors, which are the served page's :root custom properties). cats-todo
// runs inside a cats pane far more often than it runs anywhere else, so the two
// reading as one product matters more than the manager having a look of its own.
// Keep these in sync with that table.
//
// Only the greys are ours: cats' chrome tones are surfaces for a web page and
// too close together to separate a terminal's four tiers of text, so the
// name/desc/footer/done ramp is interpolated down from fg toward line rather
// than taken from the map.
const (
	colBg     = "#1f2420" // page background — the dark side of a title chip
	colFg     = "#d6ddd6" // ordinary text
	colFgHi   = "#f0f5f0" // the selected row, a step brighter than fg
	colAccent = "#4db380" // the green everything of consequence is drawn in
	colTitle  = "#1d4330" // colAccent at three-eighths brightness — the title chip's field
	colPanel  = "#2b322c" // the recessed surface an inert button sits on
	colChrome = "#3b453d" // the raised surface a live button sits on
	colMuted  = "#9db0a2" // secondary text — group headings
	colDim    = "#7d8f83" // tertiary text — descriptions, counts
	colFaint  = "#5f6f64" // quietest text — footers, completed prompts
	colOk     = "#6ac47a"
	colWarn   = "#e0b64e"
	colErr    = "#e57373"
	// The action bar's two extra hues. colInfo is the one cool color in a warm
	// green palette, mixed to sit at colOk and colErr's brightness so the bar's
	// tints read as one set rather than four unrelated colors that happen to be
	// adjacent.
	//
	// colStraw is the set's one exception, and deliberately: it is pale rather
	// than level with the others, because a yellow taken down to their
	// brightness stops being yellow and turns olive. Paleness is also what keeps
	// it off colWarn — amber is spoken for by matchStyle, which paints fuzzy
	// hits in the rows directly under this bar, and a second amber a line above
	// them would cost that highlight the thing it exists to do.
	//
	// In HSL it is 45° 52% 86%: warmed five degrees toward red from where it
	// started, which leaves it a hair off colWarn's 43° so the two stay separable
	// by hue and not only by paleness.
	colInfo  = "#6ea9d8"
	colStraw = "#eee5c9"
	// The row cursor, and the only place magenta appears. It sits outside the
	// palette on purpose: the mark saying "here" has to be findable at a glance
	// in a pane full of green, and a green mark on green rows is the one thing
	// that can't be. Muted rather than full magenta because it is a pointer, not
	// an alarm — it should be the first thing found, not the loudest.
	colCursor = "#b47fae"
)

// onRow puts st on the highlighted row's field, and returns it untouched
// otherwise. Every segment of a row goes through this — the cursor, the badge,
// each letter of the name, the description, the padding out to the right edge —
// because a terminal background only covers the cells a style actually writes.
// Wrapping the finished row in one background style cannot work: the segments
// end in resets, and the first of them drops the field for the rest of the line.
//
// colPanel is reused rather than a new tone invented. It is already the
// palette's one step up from the page, and a highlight the eye has to hunt for
// is the wrong kind of subtle — but so is a second surface color that means
// something different by a shade.
func onRow(st lipgloss.Style, selected bool) lipgloss.Style {
	if !selected {
		return st
	}
	return st.Background(lipgloss.Color(colPanel))
}

// cursorGlyph marks the highlighted row, trailing space included: it occupies
// the two columns of indentWidth that every other row leaves blank, so the
// names below it stay in one column whether the mark is there or not.
//
// It is the shell's own prompt arrow. A list of prompts waiting to be handed to
// an agent is closer to a shell than to a menu, and the arrow says "this one is
// next" where the block it replaced only said "this one". Both call sites use
// this constant so the two lists can't drift apart — one is the backlog, the
// other the attachment editor, and they are the same gesture.
//
// One terminal column wide (checked with lipgloss.Width, since a glyph the font
// draws double would push every row a column right of the action bar).
const cursorGlyph = "❯ "

// Palette — a small, cohesive set of styles for a clean dark-terminal look,
// shared by the fuzzyList component and the manager views.
var (
	// The title chip is colAccent taken down twice: the full-strength green was a
	// lamp in a dark pane, and half of it still glowed. Darkening the field flips
	// the chip's polarity — the text is the bright half now, and with that much
	// contrast behind it the weight is redundant, so the label stays regular.
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colFgHi)).
			Background(lipgloss.Color(colTitle)).
			Padding(0, 1)

	// The version rides inside the title chip, so it carries the chip's field and
	// only the foreground changes: colDim against colTitle keeps it readable but
	// clearly secondary to the name it trails. It owns the chip's right padding
	// because the label gives that padding up to sit flush against it.
	titleVerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colDim)).
			Background(lipgloss.Color(colTitle)).
			Padding(0, 1, 0, 0)

	promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colAccent)).Bold(true)
	countStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colDim))

	// The drop picker's query-line chrome: side rails only, so the field reads
	// as a box you type into without costing the layout the two lines a full
	// border would — the picker's target rows are hit-tested against a
	// constant (see targetRowsRow), and a taller header would silently move
	// them out from under the mouse.
	//
	// The rails light when the box holds the keys and go quiet when it is
	// blurred; the picker never blurs its box, so its rails are always lit.
	// The manager's list no longer draws this box at all — its query input
	// rides inline on the header line, where the 🔍 glyph carries the same
	// focus signal (searchGlyphOffStyle below, promptStyle when lit).
	searchFieldStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder(), false, true).
				BorderForeground(lipgloss.Color(colChrome)).
				Padding(0, 1)
	searchFieldOnStyle = searchFieldStyle.BorderForeground(lipgloss.Color(colAccent))

	// The header's inline query glyph while a button holds the keys. Its lit
	// counterpart is promptStyle — the same accent the boxed field's rails
	// used — so "where does typing land" reads the same in both lists.
	searchGlyphOffStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colFaint))

	nameStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colFg))
	nameSelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colFgHi)).Bold(true)
	descStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colDim))
	// A row's scope tag (" · global"). One tier below the description on
	// purpose: the tag is a whisper about where the row lives, and at the
	// description's own grey it would read as the prompt's first words.
	tagStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colFaint))
	// matchStyle stays amber. It is the one thing on screen that must not read
	// as part of the green ramp — a fuzzy hit inside a name has to jump out of
	// the letters around it, and warm-against-green is what does that.
	matchStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colWarn)).Bold(true)
	// cursorStyle draws the mark on the highlighted row (see cursorGlyph). Bold
	// is what makes an arrow thick — the glyph is already the heavy form, and
	// weight is the only other lever a terminal gives.
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colCursor)).Bold(true)
	footerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colFaint))
	headingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).Bold(true)

	// Todo-specific accents. checkStyle is deliberately unbolded: a completed
	// todo should recede, so the green tick reads as a quiet marker rather than
	// competing with the open rows above it for attention.
	doneStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colFaint)).Strikethrough(true)
	// The same row under the cursor, one tier of grey brighter. colFaint is
	// chosen to recede, which is right until the row is the one being looked
	// at — and it recedes further still against the highlight's lifted field,
	// so holding it there would make selecting a done todo cost legibility.
	doneSelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colDim)).Strikethrough(true)
	// A frozen todo's name: the same two greys as done, minus the strike. Both
	// states recede, so they share the ramp — but the strike is what says "this
	// happened", and a prompt nobody is going to do never did. The badge is what
	// separates them; the greys only say "not active work".
	frozenStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colFaint))
	frozenSelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colDim))
	checkStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(colOk))
	errStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color(colErr))
	okStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color(colOk))
	// A scheduled todo's ◷ badge. Amber on purpose: a pending auto-drop is
	// the one row that will act without being asked, and warm-against-green
	// is this palette's "look here" (matchStyle does the same trick). The
	// missed state swaps to errStyle — a promise broken outranks one pending.
	schedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colWarn))
	// A frozen todo's ❄ badge, in the palette's one cool hue (colInfo, borrowed
	// from the action bar's Add chip). Cold is the whole idea, and it is the one
	// thing the badges can be told apart by at a glance: green ✓ finished, amber
	// ◷ pending, blue ❄ shelved. Grey was the other candidate and is wrong — the
	// row's own greys already say "inactive", so a grey badge would repeat the
	// name's tier instead of naming which kind of inactive it is.
	frozenBadgeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colInfo))

	// Action-bar buttons. Each chip is rendered by exactly one of these styles —
	// label and key hint together — because nesting a second style inside a
	// chip would let the outer reset clobber the inner one (see the badge note
	// in fuzzylist.view). A chip's own hue is dropped in per action (see
	// listAction.tint), which is why btnStyle names no foreground worth relying
	// on. btnOffStyle marks a button whose action needs a highlighted todo when
	// there isn't one: still readable, plainly inert, and colorless — grey is
	// what "nothing to act on" says, so the tint has to drop out with it.
	//
	// The color goes on the letters, not the fields. Four saturated surfaces in
	// a row would out-shout the list the bar acts on, and the fields are already
	// doing the work of separating live from inert; hue on top of them would be
	// a second signal for something already said. A chip fills with its own hue
	// only when the focus is on it — one field lit at a time, which is what
	// "pressed" has to look like to be worth the ink. btnFocusStyle's own
	// background is a fallback that nothing on the bar uses.
	//
	// None of the three is bold: the chips already separate themselves from the
	// pane by their fields, and weight on top of that made the bar shout over the
	// list it acts on. The surface carries the tier; the letters stay quiet.
	btnStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(colFgHi)).Background(lipgloss.Color(colChrome)).Padding(0, 1)
	btnFocusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colBg)).Background(lipgloss.Color(colAccent)).Padding(0, 1)
	btnOffStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(colDim)).Background(lipgloss.Color(colPanel)).Padding(0, 1)
)
