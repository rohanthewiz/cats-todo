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
	// The standard-priority dot. This is cats' own `todo` key (internal/theme's
	// builtin table, and --todo in the served page) — the hue the mux paints the
	// paw print it counts this backlog with. Taking it from there means a row's
	// dot and the workspace badge that counts that row are the same color by
	// construction rather than by two people picking a yellow.
	//
	// Deliberately not colWarn, even though amber is the obvious "standard"
	// yellow. Amber is spoken for by matchStyle, which paints fuzzy hits inside
	// the row names — and standard is the default, so its dot lands on more rows
	// than any other mark on screen. A second amber on every one of those lines
	// would cost the highlight the one thing it exists to do.
	colTodo = "#f0dfa0"
	// The low-priority dot. A muted brown, and the one hue here that is neither
	// cats' nor a mix of something that is — the warm end of the palette had
	// nothing dark left in it, and this level needs to be dark.
	//
	// It is chosen on lightness rather than on hue, because lightness is what
	// separates the three dots at a glance in a single cell. The ramp runs
	// standard 78% → critical 67% → low 53% → the closed rows' 40%: low sits
	// plainly below active work and plainly above the tier that means "not work
	// any more", which is exactly what "whenever" should look like.
	//
	// Warm and saturated enough (28° 38%) not to be mistaken for colFaint's
	// near-neutral grey one step below it, and far enough under colTodo's 78%
	// that sharing the warm half of the wheel with it costs nothing. Clears
	// 4.7:1 on the page.
	colBrown = "#b5835a"
	// The action bars' extra hues. colInfo is the coolest color in a warm
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
	colInfo = "#6ea9d8"
	// The attachment hue, and the app's answer to "which color means images"
	// wherever that question comes up (today: the form toolbar's ❐ Images chip).
	// It is colInfo's sibling by construction — same lightness (64%) and the same
	// saturation (58%), 31° round the wheel from it (176° against colInfo's 207°)
	// — because the two now sit next to each other on the form's toolbar, one on
	// what the prompt carries and one on how it will run. Matching everything but
	// hue is what lets a glance separate them by color alone while the row still
	// reads as one set; a cyan picked freehand would have differed in brightness
	// too, and brightness is the bar's grammar for live-versus-inert.
	colCyan  = "#6ed8d0"
	colStraw = "#eee5c9"
	// The row cursor. It sits outside the palette on purpose: the mark saying
	// "here" has to be findable at a glance in a pane full of green, and a green
	// mark on green rows is the one thing that can't be.
	//
	// A bright, saturated yellow rather than colWarn's amber, even though both
	// are warm-against-green. matchStyle already owns amber inside the rows, and
	// the cursor sits one column to their left; at the same hue and brightness
	// the gutter mark and a fuzzy hit on the same line would read as one run of
	// color. Pushing the cursor up in both (50° 100% 60% against colWarn's
	// 43° 71% 59%) keeps them separable, and the extra brightness is what a
	// pointer drawn in a single glyph needs to compete with a whole highlighted
	// word.
	colCursor = "#ffd633"
	// The highlighted row's field. It used to be colPanel — the palette's one
	// step up from the page — and that step was too small to see: against
	// colBg it lands at roughly 1.2:1, which is a shade, not a highlight.
	//
	// This is the same green family taken up to about 1.9:1 against the page, so
	// the lit row is unmistakable from across the pane while still reading as a
	// surface rather than as a block of color. It stays a separate constant from
	// colPanel on purpose: colPanel means "a recessed surface" (the inert button
	// field) and this means "the row the keys are on", and a highlight that has
	// to be legible cannot be pinned to a tone chosen to recede.
	//
	// colFgHi on top of it still clears 10:1, so nothing on the row loses
	// contrast to gain the field.
	colSel = "#3b5245"
	// The field under selected text in the prompt editor. Same green family as
	// colSel and chosen the same way, but a full step brighter — about 2.5:1
	// against the page where the row highlight is 1.9:1.
	//
	// The extra step is not decoration. colSel marks a whole row, edge to edge,
	// and a wide band of color is legible at a contrast a few characters in the
	// middle of a paragraph are not; a selection has to be findable when it is
	// three letters long. It also has to be told apart from the editor's own
	// cursor-line field, which the row highlight never has to compete with.
	//
	// colFg on top of it still clears 4.5:1, so selected text is no harder to
	// read than the text around it — which matters here more than on a list row,
	// because the reason to select something is to read it before copying it.
	colTextSel = "#4a6656"
)

// onRow puts st on the highlighted row's field, and returns it untouched
// otherwise. Every segment of a row goes through this — the cursor, the badge,
// each letter of the name, the description, the padding out to the right edge —
// because a terminal background only covers the cells a style actually writes.
// Wrapping the finished row in one background style cannot work: the segments
// end in resets, and the first of them drops the field for the rest of the line.
//
// The field is colSel, a tone of its own rather than the panel grey the button
// bar uses: a highlight the eye has to hunt for is the wrong kind of subtle, and
// colPanel — one shade off the page — was exactly that.
func onRow(st lipgloss.Style, selected bool) lipgloss.Style {
	if !selected {
		return st
	}
	return st.Background(lipgloss.Color(colSel))
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

// grabGlyph replaces the arrow on a row the pointer has hold of, for the length
// of a drag (see fuzzyList.grab). The braille block is the drag handle every
// list on the web draws, so it reads as "this row is in your hand" without a
// legend — and it occupies the same single column the arrow does, so the names
// beside it do not shift by a cell the moment the button goes down.
const grabGlyph = "⠿ "

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

	// The backlog's name where it leads the list header (see headerTitle). It is
	// the heading of that line now that the tool's chip is gone from it, but it
	// is also a name that changes with the pane, so it stays flat text rather
	// than taking the chip's field: a filled block reads as chrome, and chrome
	// is exactly what the header was carrying too much of. Weight plus the
	// brightest foreground is enough to sit a step above the dim remainder of
	// the note that trails it.
	headerNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colFgHi)).
			Bold(true)

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
	// A row's 📎N attachment count, in the app's attachment cyan — the same hue
	// the editor's ❐ Images chip carries, so the two screens answer "images" with
	// one color. It is the only one of a row's description marks with a hue of its
	// own: ⏰ and ⚙ say something about a prompt that its own words also say, while
	// an attachment is the one thing on the row that is not in the text anywhere.
	//
	// Not a badge and not on the badge's ramp: the badge column holds the row's
	// state (done, frozen, scheduled) and there is only one of those at a time,
	// where the marks are a list that leads the description. Sharing the badge's
	// styles would have made a picture look like a state.
	attachStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colCyan))

	// The priority mark, one style per raised level. Named for the level rather
	// than the color so the row code reads as triage instead of as painting.
	//
	// It gets a column of its own, after the badge, rather than joining either of
	// the row's existing marks. The badge holds the row's *state* and there is
	// exactly one of those at a time (see the note above it), while priority is
	// orthogonal to all four — a critical prompt can also be scheduled. The
	// descMarks were the other candidate and are worse for the opposite reason:
	// they follow the name, so they start at a ragged column, and a triage color
	// that cannot be scanned straight down the list is not doing the job it was
	// added for.
	//
	// Critical keeps the error red. High takes cats' own todo yellow — the hue
	// the mux paints a waiting prompt in — which is the right tier for "this
	// matters" precisely because it is the color of ordinary outstanding work
	// turned up rather than an alarm. The amber colWarn is not available for it:
	// that one is spoken for by the fuzzy-match highlight, and two meanings for
	// one hue on the same row is one too many.
	prioCriticalStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colErr))
	prioHighStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(colTodo))
	// The same mark on a row that is no longer active work. Closed rows are drawn
	// to recede (see doneStyle), and a red mark on finished work would compete
	// with the open prompts above it for exactly the attention priority exists
	// to direct. The glyph stays so the record of the rating reads; only the
	// shout comes off.
	prioClosedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colFaint))
	// And on a closed row under the cursor, one tier brighter — the same
	// concession doneSelStyle makes, for the same reason: a tone chosen to
	// recede recedes further still against the highlight's lifted field.
	prioClosedSelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colDim))
	// The low-hanging-fruit mark. An emoji paints itself and ignores a
	// foreground, so this style carries no color and exists for the one thing it
	// still has to do: give the segment something for onRow to hang the
	// highlighted row's field on (see the badge note in fuzzylist.view). It is
	// also why there is no closed tier here to match prioClosedStyle — a grey
	// that cannot reach the glyph is not a recession, so a closed row stops
	// drawing the apple instead (see fruitMark).
	fruitStyle = lipgloss.NewStyle()
	// The flag. colInfo is the palette's one cool hue, and that is the point:
	// the other two annotations sit on the warm ramp that runs from "ordinary
	// work" up to "alarm", and the flag is not a point on that ramp — it says
	// "there is something else about this one", which is a different kind of
	// claim and should not read as a third loudness. It is a text glyph rather
	// than an emoji, so unlike the apple it can take a foreground at all, which
	// is what lets it recede on a closed row instead of going quiet (flagMark).
	flagStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colInfo))
)

// The annotation glyphs (see annotations.go for the columns they live in).
//
// Priority is a triangle rather than a dot so it cannot be confused with the
// state badge two cells to its left, which is a circle in every one of its four
// forms (○ ✓ ❄ ◷): a shape says "different kind of fact" at a glance where a hue
// alone only says "different value". The pair escalates by fill as well as by
// color — hollow for high, solid for critical — so the level survives a
// colorblind reader, a monochrome capture, and a terminal theme that has flattened
// the palette. Both are East Asian Ambiguous, the same width class as the ○ this
// list has always drawn, so each costs exactly one cell wherever that one does.
//
// The fruit is the green apple rather than the red one. Red is the critical
// mark's color one column to its left, and two reds on one row would read as one
// signal repeated; green is what the rest of this palette already uses for "go
// ahead". It is an emoji and therefore two cells wide — annotSlots declares that
// width, so the names below it stay in one column.
const (
	prioCriticalGlyph = "▲"
	prioHighGlyph     = "△"
	fruitGlyph        = "🍏"
	// The pennant is the flag mark. A shape with no fill and no colour twin —
	// unlike the triangles, it has no second level to escalate to, because the
	// flag is one bit and everything else it has to say is in its note. One cell
	// wide (East Asian Ambiguous, like the triangles and the badge), so a row
	// wearing all three marks still costs what the packing promised.
	flagGlyph = "⚑"
)

var (
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

	// The prompt editor's selection. The foreground is set as well as the field
	// because the text under a selection may have been drawn in any of the
	// textarea's own tones, and a highlight that inherited them would come out a
	// different brightness on the caret's line than on the lines above it.
	promptSelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colFgHi)).Background(lipgloss.Color(colTextSel))
	// The caret, redrawn when a selection covers the cells around it (see
	// paintPromptSelection). Reverse rather than a color pair so it matches the
	// block the textarea draws for itself the rest of the time, whatever the
	// terminal resolves that to. The extra carets of the editor's column mode
	// (promptcarets.go) are painted with it too, for exactly that reason: they
	// have to be indistinguishable from the one the library draws.
	promptCaretStyle = lipgloss.NewStyle().Reverse(true)

	// The prompt editor's context menu (promptmenu.go). It floats over the form,
	// so unlike every other surface here it has to carry its own background on
	// every cell — a transparent row would let the editor's text show through
	// the box and the menu would read as a frame drawn on the prompt rather than
	// as something on top of it.
	//
	// colPanel is the recessed field an inert button sits on, which is the right
	// tone for a surface that is temporary: raised enough to separate from the
	// pane, quiet enough not to compete with the prompt it is asking about. The
	// border takes colChrome so the box has an edge without a second hue.
	menuBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colChrome)).
			BorderBackground(lipgloss.Color(colPanel))
	menuRowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colFg)).Background(lipgloss.Color(colPanel))
	// The row the keyboard is on, in the accent field the action bars light a
	// pressed chip with — one field lit at a time, the same rule those bars
	// follow, so "where the keys are" reads identically on both.
	menuRowSelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colBg)).Background(lipgloss.Color(colAccent))
	// An item that cannot act on what is selected. Grey on the same field: still
	// legible, plainly inert, and colorless — the same thing btnOffStyle says
	// about a button with nothing to act on.
	menuRowOffStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colFaint)).Background(lipgloss.Color(colPanel))

	// The list's hover card (listhover.go). It floats over the list the way a
	// menu floats over its screen, so it borrows the menu's box and field
	// wholesale — two temporary surfaces that looked different would read as two
	// different programs — and spends its own choices only on the three tiers of
	// text inside it.
	//
	// The title takes colFg at weight, the body the muted grey secondary text is
	// drawn in everywhere else, and the field rows colFaint: a descending ramp,
	// so the eye lands on which prompt this is, then reads it, and finds the
	// launch flags last — which is the order the questions are asked in.
	hoverTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colFg)).Background(lipgloss.Color(colPanel)).Bold(true).
			Padding(0, 1)
	hoverBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colMuted)).Background(lipgloss.Color(colPanel)).
			Padding(0, 1)
	hoverFieldStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colFaint)).Background(lipgloss.Color(colPanel)).
			Padding(0, 1)
)
