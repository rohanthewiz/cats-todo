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
)

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

	// The query line's chrome: side rails only, so the field reads as a box you
	// type into without costing the layout the two lines a full border would —
	// every row below it is hit-tested against a constant (see actionBarRow), and
	// a taller header would silently move the rows out from under the mouse.
	searchFieldStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder(), false, true).
				BorderForeground(lipgloss.Color(colChrome)).
				Padding(0, 1)

	nameStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colFg))
	nameSelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colFgHi)).Bold(true)
	descStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colDim))
	// matchStyle stays amber. It is the one thing on screen that must not read
	// as part of the green ramp — a fuzzy hit inside a name has to jump out of
	// the letters around it, and warm-against-green is what does that.
	matchStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(colWarn)).Bold(true)
	barStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(colAccent)).Bold(true)
	footerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colFaint))
	headingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).Bold(true)

	// Todo-specific accents. checkStyle is deliberately unbolded: a completed
	// todo should recede, so the green tick reads as a quiet marker rather than
	// competing with the open rows above it for attention.
	doneStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colFaint)).Strikethrough(true)
	checkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colOk))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(colErr))
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colOk))

	// Action-bar buttons. Each chip is rendered by exactly one of these styles —
	// label and key hint together — because nesting a second style inside a
	// chip would let the outer reset clobber the inner one (see the badge note
	// in fuzzylist.view). btnOffStyle marks a button whose action needs a
	// highlighted todo when there isn't one: still readable, plainly inert.
	//
	// None of the three is bold: the chips already separate themselves from the
	// pane by their fields, and weight on top of that made the bar shout over the
	// list it acts on. The surface carries the tier; the letters stay quiet.
	btnStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(colFgHi)).Background(lipgloss.Color(colChrome)).Padding(0, 1)
	btnFocusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colBg)).Background(lipgloss.Color(colAccent)).Padding(0, 1)
	btnOffStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(colDim)).Background(lipgloss.Color(colPanel)).Padding(0, 1)
)
