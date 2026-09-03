// fuzzylist.go — a reusable fuzzy-filtered, keyboard-navigable list, used by
// both the todo list and the drop-target picker. The list owns the query input
// and the filtering; the caller decides where the query line is drawn — view
// renders it boxed above the rows (the picker), rowsView leaves it to the
// caller entirely (the manager's header line).
//
// Adapted from herdr-plus (https://github.com/cloudmanic/herdr-plus),
// Copyright (c) 2026 Cloudmanic Labs, LLC, MIT License. See NOTICE.

package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
)

// listItem is one row in a fuzzyList. A selectable row shows a name (matched and
// highlighted) plus an optional dimmer description, and carries ref — the
// caller's identifier for the row. A row with selectable=false is a
// non-selectable separator: with a name it renders as a dim group heading,
// without one as a blank spacer. badge, when set, is drawn before the name
// (used to mark done todos) in badgeStyle. strike renders the name
// struck-through (done); dim renders it in the same receded greys without the
// strike (frozen), so a row can say "not active work" without also claiming the
// work was carried out.
//
// The badge arrives as a glyph and a style rather than pre-rendered text: the
// highlighted row puts a field behind every one of its segments, and text that
// is already styled cannot be given a background without nesting a second style
// inside the first — which is how the outer reset ends up clobbering the inner.
//
// tag, when set, is a quiet marker drawn right after the name (" · tag"), in
// the list's faintest tier. It exists because group headings are not always
// there to say what a row is: a heading is a separator row, and separators are
// dropped while a query filters — so a fact that must survive filtering has to
// ride the row itself.
// annots are the annotation marks between the badge and the name — the todo
// list's priority mark and its low-hanging-fruit apple (see annotations.go).
// They are separate from badge because the badge holds one mutually-exclusive
// state and an annotation is not one of them: a row can be critical and cheap
// and scheduled all at once, and three facts need three glyphs to be true
// together.
//
// They carry only the marks the row actually has, packed one after another, so
// an unannotated row spends nothing on them and its name starts where the badge
// ends. Nothing here reserves space for a mark this row does not wear.
//
// descMarks are the small flags that lead the description — the fire time, the
// session cog, the attachment count — each drawn in its own hue and in the order
// given, ahead of the description text itself. They are separate from desc for
// the same reason badge is separate from name: a mark that carries a color has
// to reach the row as text plus a style, because the highlighted row puts its
// field behind every segment it draws and a segment that already ends in a reset
// cannot be given a background without nesting one style inside another.
type listItem struct {
	name       string
	desc       string
	descMarks  []descMark
	search     string // when set, replaces desc in the fuzzy-match haystack
	badge      string
	badgeStyle lipgloss.Style
	annots     []annotMark
	tag        string
	strike     bool
	dim        bool
	selectable bool
	// marked is the row's membership in the list's current selection — the set
	// an export acts on (see model.marked). It rides on the item rather than on
	// the list because the list is rebuilt from the backlogs on every change,
	// while the selection is keyed by todo and survives that rebuild.
	marked bool
	ref    int
}

// annotMark is one annotation on one row: the glyph to draw and the two styles
// to draw it in — ordinary and on the highlighted row, so a mark drawn in a
// receded tone can be lifted there the way the name's own greys are. A mark that
// does not recede carries the same style twice.
type annotMark struct {
	text     string
	style    lipgloss.Style
	selStyle lipgloss.Style
}

// descMark is one such flag: what to print, and the style to print it in.
type descMark struct {
	text  string
	style lipgloss.Style
}

// scoredItem is a listItem that survived the current query filter, carrying the
// name-character positions that matched, for highlighting.
type scoredItem struct {
	item    listItem
	matched []int
}

// fuzzyList is a reusable fuzzy-filtered, keyboard-navigable list with a query box.
type fuzzyList struct {
	input    textinput.Model
	items    []listItem
	filtered []scoredItem
	cursor   int
	// grab marks the highlighted row as held by the pointer: it swaps the cursor
	// arrow for a grip while a drag is under way (see grabGlyph). It lives on the
	// list rather than being passed to rowsView because the two callers of that
	// method render very different chrome around it, and only one of them has a
	// pointer to answer for — a parameter would make the other declare a state it
	// can never be in.
	grab bool
	// maxRows, when set, caps how many filtered rows rowsView draws at once, and
	// top is the index into filtered of the first row it draws — together the
	// list's scroll window. Zero maxRows means no window: every caller before
	// the file picker draws its whole list and lets the pane clip it, and they
	// still do. The window is opt-in because it is only worth having where the
	// list can plausibly outgrow the pane and the caller wants the highlight to
	// stay on screen as it walks — a home directory of sixty entries, say, or a
	// backlog longer than the pane.
	//
	// The window is counted in filtered items, not screen lines: a separator
	// row draws one or two lines of its own, so a windowed list that carries
	// separators would overshoot a cap set from a pane's height. The picker has
	// none, so item and line agree there; a caller with headings hands the
	// difference back itself (see separatorLines), which is what the manager's
	// list does.
	maxRows int
	top     int
	// prefixFirst, when set, ranks the rows whose name begins with the query
	// (case-insensitively) ahead of the rest, in the order the caller listed
	// them, and only then the remaining fuzzy matches by score. It is cdx's
	// rule for completing a path segment, and it is here for the same job: a
	// file picker's query is the start of a name far more often than a scatter
	// of its letters, and a scorer that put init_test.go above internal/ for
	// "int" — which the fuzzy scorer does — would make "int/" open the wrong
	// thing. The todo list keeps pure fuzzy: its queries are words from
	// anywhere in a prompt, where a prefix means nothing special.
	prefixFirst bool
	// showMarks turns on the leading ✓ column. It is a whole-list decision, and
	// the last column left that is one: a column that appeared and vanished per
	// row would leave
	// the names ragged, and one that is always there would cost every backlog an
	// indent for a selection almost nobody is holding. So the column exists
	// exactly while something is marked — including rows the filter is hiding,
	// which is why this is set by the caller from the selection itself rather
	// than measured off the drawn items.
	showMarks bool
}

// searchFieldWidth is how many columns the query box holds. Wide enough for the
// placeholders both lists use, narrow enough to leave the match count beside it
// in a cats pane.
const searchFieldWidth = 34

// newFuzzyList builds a list over items with a focused, empty query box.
func newFuzzyList(placeholder string, items []listItem) fuzzyList {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Prompt = ""
	// A fixed width keeps the field's rails still: without it the box would hug
	// the text and breathe in and out on every keystroke.
	ti.SetWidth(searchFieldWidth)
	ti.Focus()

	l := fuzzyList{input: ti, items: items}
	l.filter()
	return l
}

// setItems replaces the list's rows (e.g. after an add/edit/delete) and keeps
// the query, re-filtering and clamping the cursor.
func (l *fuzzyList) setItems(items []listItem) {
	l.items = items
	l.filter()
}

// filter recomputes the visible rows from the current query. An empty query
// shows every item — separators included — in its natural order. A non-empty
// query fuzzy-matches only the selectable items against the name plus the
// item's search text (its rendered description when no search text is set), so
// a query can hit text that isn't on screen — like the deep lines of a
// multi-line prompt. Only matches inside the name are highlighted.
func (l *fuzzyList) filter() {
	q := strings.TrimSpace(l.input.Value())
	l.filtered = l.filtered[:0]

	if q == "" {
		for _, it := range l.items {
			l.filtered = append(l.filtered, scoredItem{item: it})
		}
		l.clampCursor()
		return
	}

	var sel []listItem
	for _, it := range l.items {
		if it.selectable {
			sel = append(sel, it)
		}
	}
	haystacks := make([]string, len(sel))
	nameLens := make([]int, len(sel))
	for i, it := range sel {
		haystacks[i] = it.name + "  " + firstNonEmpty(it.search, it.desc)
		nameLens[i] = len(it.name)
	}
	// With prefixFirst, the prefix matches lead in list order (the caller's
	// order is the meaningful one there — folders before files, each sorted)
	// and are struck from the fuzzy pass so no row appears twice.
	taken := map[int]bool{}
	if l.prefixFirst {
		lq := strings.ToLower(q)
		for i, it := range sel {
			if strings.HasPrefix(strings.ToLower(it.name), lq) {
				taken[i] = true
				l.filtered = append(l.filtered, scoredItem{item: it, matched: prefixIndexes(it.name, q)})
			}
		}
	}
	for _, mt := range fuzzy.Find(q, haystacks) {
		if taken[mt.Index] {
			continue
		}
		var inName []int
		for _, idx := range mt.MatchedIndexes {
			if idx < nameLens[mt.Index] {
				inName = append(inName, idx)
			}
		}
		l.filtered = append(l.filtered, scoredItem{item: sel[mt.Index], matched: inName})
	}

	l.clampCursor()
}

// prefixIndexes are the byte offsets of name's first len(q) bytes — the run a
// prefix match highlights, in the units highlightName reads (it walks the name
// with range, whose index is a byte offset, and filter's nameLens cut is bytes
// too). A multibyte character in the query lights the same number of bytes it
// occupies, so the two stay in step.
func prefixIndexes(name, q string) []int {
	n := min(len(q), len(name))
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	return idx
}

// clampCursor keeps the cursor in range and parked on a selectable row, then
// brings the scroll window along so the row it settles on is a drawn one.
func (l *fuzzyList) clampCursor() {
	l.parkCursor()
	l.ensureVisible()
}

// parkCursor is clampCursor without the window step: the search for the nearest
// selectable row, on its own.
func (l *fuzzyList) parkCursor() {
	if len(l.filtered) == 0 {
		l.cursor = 0
		return
	}
	if l.cursor >= len(l.filtered) {
		l.cursor = len(l.filtered) - 1
	}
	if l.cursor < 0 {
		l.cursor = 0
	}
	if l.filtered[l.cursor].item.selectable {
		return
	}
	for i := l.cursor; i < len(l.filtered); i++ {
		if l.filtered[i].item.selectable {
			l.cursor = i
			return
		}
	}
	for i := l.cursor; i >= 0; i-- {
		if l.filtered[i].item.selectable {
			l.cursor = i
			return
		}
	}
}

// moveUp and moveDown move the highlight to the previous/next selectable row.
func (l *fuzzyList) moveUp() {
	for i := l.cursor - 1; i >= 0; i-- {
		if l.filtered[i].item.selectable {
			l.cursor = i
			break
		}
	}
	l.ensureVisible()
}

func (l *fuzzyList) moveDown() {
	for i := l.cursor + 1; i < len(l.filtered); i++ {
		if l.filtered[i].item.selectable {
			l.cursor = i
			break
		}
	}
	l.ensureVisible()
}

// setMaxRows sets the scroll window's height in rows (see maxRows); zero
// removes the window. The highlight is brought back on screen straight away,
// which is what a resize needs: a window that shrank under the cursor must
// scroll to it, not leave it drawn nowhere.
func (l *fuzzyList) setMaxRows(n int) {
	l.maxRows = max(n, 0)
	l.ensureVisible()
}

// ensureVisible scrolls the window so the cursor's row is drawn: the least
// movement that brings the row inside [top, top+maxRows), then a clamp so the
// window never hangs off the end of the list (a filter that shrank the list
// under a scrolled window would otherwise draw nothing). Every mutator that can
// move the cursor or change the rows ends here, and the renderers — value
// receivers, which cannot scroll — trust that they did.
//
// Adapted from cdx's ensureVisible; the difference is that cdx counts screen
// lines and this counts filtered items (see maxRows).
func (l *fuzzyList) ensureVisible() {
	if l.maxRows <= 0 {
		l.top = 0
		return
	}
	if l.cursor < l.top {
		l.top = l.cursor
	}
	if l.cursor >= l.top+l.maxRows {
		l.top = l.cursor - l.maxRows + 1
	}
	if over := len(l.filtered) - l.maxRows; l.top > over {
		l.top = max(over, 0)
	}
	if l.top < 0 {
		l.top = 0
	}
}

// window is the range of filtered indexes rowsView draws — the whole list when
// there is no window, else the maxRows rows starting at top. rowAtLine walks the
// same range, which is what keeps a click on a scrolled list landing on the row
// that is actually under it.
func (l fuzzyList) window() (lo, hi int) {
	if l.maxRows <= 0 {
		return 0, len(l.filtered)
	}
	return l.top, min(l.top+l.maxRows, len(l.filtered))
}

// selectRef parks the cursor on the visible selectable row carrying ref, so a
// caller can keep the highlight on an item across a rebuild (e.g. after
// reordering). A ref that isn't visible leaves the cursor where it was.
func (l *fuzzyList) selectRef(ref int) {
	for i, s := range l.filtered {
		if s.item.selectable && s.item.ref == ref {
			l.cursor = i
			break
		}
	}
	l.ensureVisible()
}

// focusRow parks the cursor on the filtered row at index i — the pointer's way
// in, where the keyboard uses moveUp/moveDown. A row that isn't there or isn't
// selectable leaves the cursor alone.
func (l *fuzzyList) focusRow(i int) bool {
	if i < 0 || i >= len(l.filtered) || !l.filtered[i].item.selectable {
		return false
	}
	l.cursor = i
	l.ensureVisible()
	return true
}

// rowAtLine maps a line of the rendered rows block — 0 being the first line the
// rows are drawn on, below the query box — to the filtered row drawn there, for
// hit-testing a click. A line holding a heading, a spacer, or the "nothing
// matched" message answers false: there is no row to pick there.
//
// It walks the same lines view writes, in the same order, so the two have to
// stay in step; TestTargetRowClickHitsWhatIsDrawn pins them together against a
// real rendered frame.
func (l fuzzyList) rowAtLine(n int) (int, bool) {
	if n < 0 {
		return -1, false
	}
	line := 0
	matched := 0
	for _, s := range l.filtered {
		if s.item.selectable {
			matched++
		}
	}
	if matched == 0 {
		line++ // the empty-list message stands where the rows would
	}
	// Only the drawn window is walked, and i stays an index into filtered
	// rather than into the window, so the row this answers is the one focusRow
	// and refAt understand.
	lo, hi := l.window()
	for i := lo; i < hi; i++ {
		s := l.filtered[i]
		if !s.item.selectable {
			line++ // the blank spacer every separator opens with
			if s.item.name != "" {
				line++ // …and the group heading, when it has one
			}
			continue
		}
		if line == n {
			return i, true
		}
		line++
	}
	return -1, false
}

// refAt returns the caller's ref for the filtered row at index i — the other
// half of the lookup rowAtLine starts, so a hit-tested screen line can be turned
// all the way back into the thing the caller put in the list. A row that isn't
// there or isn't selectable answers false, the same as focusRow.
func (l fuzzyList) refAt(i int) (int, bool) {
	if i < 0 || i >= len(l.filtered) || !l.filtered[i].item.selectable {
		return -1, false
	}
	return l.filtered[i].item.ref, true
}

// selectedIndex returns the ref of the highlighted selectable row, or -1 when
// nothing is selectable (empty list, or all matches filtered away).
func (l *fuzzyList) selectedIndex() int {
	if len(l.filtered) == 0 {
		return -1
	}
	it := l.filtered[l.cursor].item
	if !it.selectable {
		return -1
	}
	return it.ref
}

// editQuery feeds a message to the query box and re-filters.
func (l *fuzzyList) editQuery(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	l.input, cmd = l.input.Update(msg)
	l.filter()
	return cmd
}

// counts reports how many selectable rows survive the current query (matched)
// out of how many the list holds (total). Separators are neither: a heading
// that always renders would make every "3/3" read as a lie.
func (l fuzzyList) counts() (matched, total int) {
	for _, it := range l.items {
		if it.selectable {
			total++
		}
	}
	for _, s := range l.filtered {
		if s.item.selectable {
			matched++
		}
	}
	return matched, total
}

// view renders the query line, the match count, and the result rows. bar, when
// non-empty, is written on its own line between the query and the rows — the
// manager's action bar. It is passed in pre-rendered rather than built here
// because the buttons act on the caller's model, not on the list.
//
// The manager's list stage no longer calls this — it draws the query segment on
// its own header line and asks for rowsView directly. The drop-target picker
// still renders through here, boxed search field and all.
//
// width is how far the highlighted row's field runs. It is a parameter rather
// than a field on the list because the caller already holds the window size and
// re-renders on every resize; keeping a copy here would be one more thing that
// can go stale, and it goes stale silently — as a highlight that stops short of
// the edge.
func (l fuzzyList) view(emptyMsg, bar string, width int) string {
	var b strings.Builder

	matched, total := l.counts()

	// The input's own focus is the truth, so a caller that blurs the box gets
	// the quiet rails for free and can't leave the two disagreeing. The picker
	// never blurs its box, so its rails are simply always lit.
	field := searchFieldStyle
	if l.input.Focused() {
		field = searchFieldOnStyle
	}
	b.WriteString(field.Render(promptStyle.Render("🔍 ") + l.input.View()))
	b.WriteString("  ")
	b.WriteString(countStyle.Render(fmt.Sprintf("%d/%d", matched, total)))
	b.WriteString("\n")
	if bar != "" {
		b.WriteString("\n")
		b.WriteString(bar)
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString(l.rowsView(emptyMsg, width))
	return b.String()
}

// The overflow markers: what a windowed list says about the rows it is not
// drawing. A '▴' rides the FIRST row of the window and a '▾' the LAST, each
// carrying the count of what lies that way, right-aligned into the row's own
// trailing space.
//
// This replaced a "… N more" line under the list, and the two differences are
// the whole point. It SPENDS NO LINE: a marker that cost one would take a row
// of backlog away from a pane exactly when the pane has run out of room, which
// is the one moment the list can least afford it — and it would have to be
// reserved whether or not there was anything to say, or the rows below would
// jump a line every time the window reached the end. Sharing the row it
// annotates costs nothing while the list fits and a few blank columns while it
// does not. And it points BOTH WAYS: the old marker said nothing at all about
// the rows scrolled off the TOP, so a list scrolled into the middle looked
// exactly like a list read from the beginning.
//
// The count rides the glyph rather than waiting on a hover, because there is
// nothing to hover here — this is a keyboard-and-click TUI with no pointer
// dwell — and "there is more" without "how much" is the half of the answer the
// reader can already see for themselves.
const (
	overflowUpGlyph   = "▴"
	overflowDownGlyph = "▾"
)

// overflowMarks builds the marker text for one drawn row: the up marker when it
// is the window's first row and something is above, the down marker when it is
// the last and something is below, and both — on the one row that is both, in a
// window a single row tall — joined so neither is silently dropped.
func overflowMarks(above, below int) string {
	var parts []string
	if above > 0 {
		parts = append(parts, fmt.Sprintf("%s %d", overflowUpGlyph, above))
	}
	if below > 0 {
		parts = append(parts, fmt.Sprintf("%s %d", overflowDownGlyph, below))
	}
	return strings.Join(parts, "  ")
}

// withOverflowMark right-aligns mark into what is left of the row's width, and
// returns the line untouched when there is not enough of it.
//
// Refusing is deliberate, and it is about the click map rather than about
// looks: rowAtLine counts the lines this function's caller writes, so a row
// pushed past the pane's edge would wrap in the terminal, put every row below
// it one physical line lower than the hit test believes, and hand clicks to the
// wrong prompts. A marker may annotate a row; it may never move one. A pane too
// narrow for the row plus four columns is already a pane the row itself does
// not fit in, so the case this gives up on is one where the list is mis-drawn
// anyway — and the header's own N/M count still says how much is being held
// back.
//
// selected carries the highlighted row's field across the padding and the mark,
// so a marked highlight still runs unbroken to the edge.
func withOverflowMark(line, mark string, width int, selected bool) string {
	if mark == "" || width <= 0 {
		return line
	}
	pad := width - lipgloss.Width(line) - lipgloss.Width(mark)
	if pad < 1 {
		return line
	}
	st := descStyle
	if selected {
		st = onRow(descStyle, true)
	}
	return line +
		onRow(lipgloss.NewStyle(), selected).Render(strings.Repeat(" ", pad)) +
		st.Render(mark)
}

// separatorLines is how many screen lines this list's non-selectable rows cost
// on top of the one line each filtered row is budgeted for: a spacer draws its
// blank, and a group heading draws a blank and a title.
//
// It exists because the scroll window is counted in ITEMS while a pane is
// measured in LINES (see maxRows), so a caller sizing the window to a pane has
// to hand back the difference or the list overruns its own chrome. It counts
// over items rather than over the filtered set on purpose: a query drops the
// separators entirely, so the unfiltered count is the larger of the two and
// using it means the answer stays safe as the query comes and goes, without
// the caller having to re-size the window on every keystroke.
func (l fuzzyList) separatorLines() int {
	n := 0
	for _, it := range l.items {
		if it.selectable {
			continue
		}
		n++ // the blank spacer every separator opens with
		if it.name != "" {
			n++ // …and the heading, when it has one
		}
	}
	return n
}

// rowsView renders just the result rows (or the empty message) — the block
// rowAtLine hit-tests against, with no query line above it. Callers that
// compose their own chrome around the list start from here.
func (l fuzzyList) rowsView(emptyMsg string, width int) string {
	var b strings.Builder

	matched, _ := l.counts()
	if matched == 0 {
		b.WriteString(descStyle.Render("  " + emptyMsg))
		b.WriteString("\n")
	}
	lo, hi := l.window()
	above, below := lo, len(l.filtered)-hi
	for i := lo; i < hi; i++ {
		s := l.filtered[i]
		it := s.item
		// The overflow markers, resolved for this row. The '▴' rides the
		// window's FIRST row and the '▾' its LAST, which is where the eye is
		// standing when it runs out of list — the same two places a viewport
		// with a scrollbar would put the ends of its rail, minus the rail.
		//
		// One row can be both ends at once, in a window a single row tall, so
		// the two are resolved together rather than one overwriting the other.
		up, down := 0, 0
		if i == lo {
			up = above
		}
		if i == hi-1 {
			down = below
		}
		mark := overflowMarks(up, down)
		if !it.selectable {
			// A separator's marker goes on its HEADING, which is the line the
			// eye reads; a nameless spacer has only its blank line to carry
			// one. Either way it is one line where the row drew one, so
			// rowAtLine's count is untouched.
			if it.name != "" {
				b.WriteString("\n")
				b.WriteString(withOverflowMark(headingStyle.Render(it.name), mark, width, false))
				b.WriteString("\n")
				continue
			}
			b.WriteString(withOverflowMark("", mark, width, false))
			b.WriteString("\n")
			continue
		}
		selected := i == l.cursor

		// The row is built apart from the page so it can be measured, then
		// padded out to the right edge. A field that stops where the text does
		// reads as a box drawn around the words; one that runs the width of the
		// pane reads as the row itself being lit, which is the whole point of
		// highlighting a row rather than its name.
		var r strings.Builder
		if selected {
			// The grip replaces the arrow only on the row being dragged, which is
			// always the highlighted one: the drag starts by selecting what it
			// picked up and re-parks the highlight after every reorder, so the
			// mark that says "here" and the row in the hand are the same row.
			glyph := cursorGlyph
			if l.grab {
				glyph = grabGlyph
			}
			r.WriteString(onRow(cursorStyle, true).Render(glyph))
		} else {
			r.WriteString("  ")
		}
		// The selection column leads everything: with a set held, "is this one
		// of the ones about to travel" is the question the eye arrives at a row
		// with, ahead of what state the prompt is in or what is true about it.
		// Blank rather than an empty box on an unmarked row — an unticked
		// checkbox on every line would draw the eye to the rows that are not
		// the answer.
		if l.showMarks {
			cell := " "
			if it.marked {
				cell = markGlyph
			}
			r.WriteString(onRow(markStyle, selected).Render(cell + " "))
		}
		// The badge leads, then the annotations, so the row reads outward from
		// the cursor as "what state is this in" then "what is true about it" —
		// state first because it is what the list is grouped by, and the eye
		// arriving at a row wants "is this still work" before "how much work".
		if it.badge != "" {
			r.WriteString(onRow(it.badgeStyle, selected).Render(it.badge + " "))
		}
		// One segment per mark, each followed by a single space — the last one
		// included, which is what stands the group off the name. No padding and
		// no reserved cells: a row wears the marks it has and no more, so the
		// group is as wide as it has something to say. Its own segment with its
		// own style, like every other part of the row (see descMark).
		for _, an := range it.annots {
			st := an.style
			if selected {
				st = an.selStyle
			}
			r.WriteString(onRow(st, selected).Render(an.text + " "))
		}
		r.WriteString(highlightName(it.name, s.matched, selected, it.strike, it.dim))
		if it.tag != "" {
			// The tag hugs the name (one space, with its own separator dot)
			// where the description stands two off: it qualifies the name, and
			// the faint tier plus the dot are what keep it from reading as the
			// description's first word.
			r.WriteString(onRow(tagStyle, selected).Render(" · " + it.tag))
		}
		// Each mark is its own segment, two columns off whatever precedes it —
		// one style per segment, so the highlighted row's field survives all of
		// them (see descMark).
		for _, mk := range it.descMarks {
			r.WriteString(onRow(mk.style, selected).Render("  " + mk.text))
		}
		if it.desc != "" {
			r.WriteString(onRow(descStyle, selected).Render("  " + it.desc))
		}
		// The marker is right-aligned into the row's own trailing space, so
		// it eats the highlight's padding rather than adding to it — a marked
		// selected row still runs to the edge, and the pad below then has
		// nothing left to do.
		row := withOverflowMark(r.String(), mark, width, selected)

		b.WriteString(row)
		if pad := width - lipgloss.Width(row); selected && pad > 0 {
			b.WriteString(onRow(lipgloss.NewStyle(), true).Render(strings.Repeat(" ", pad)))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// highlightName renders a row's name with the fuzzy-matched characters
// emphasized. When strike is set (a done todo) the base text is dimmed and
// struck through; when dim is set without it (a frozen todo) the text recedes
// the same way but keeps its letters intact. strike wins if a caller somehow
// asks for both — a finished todo reads as finished first.
//
// Done and selected used to be exclusive, done winning: a completed row under
// the cursor drew exactly like a completed row anywhere else, so the highlight
// simply did not exist for half the list. They are independent now — the strike
// says what the todo is, the field says which one the keys are on — and a done
// row that is being looked at comes up one tier of grey, since receding into
// the background is right for a row nobody asked about and wrong for the row
// under the cursor.
func highlightName(name string, matched []int, selected, strike, dim bool) string {
	base := nameStyle
	switch {
	case strike && selected:
		base = doneSelStyle
	case strike:
		base = doneStyle
	case dim && selected:
		// Same reason the done row comes up a tier under the cursor: receding is
		// right for a row nobody asked about and wrong for the one being read.
		base = frozenSelStyle
	case dim:
		base = frozenStyle
	case selected:
		base = nameSelStyle
	}
	base = onRow(base, selected)
	hit := onRow(matchStyle, selected)
	if len(matched) == 0 {
		return base.Render(name)
	}

	set := make(map[int]bool, len(matched))
	for _, idx := range matched {
		set[idx] = true
	}

	var b strings.Builder
	for i, r := range name {
		if set[i] {
			b.WriteString(hit.Render(string(r)))
		} else {
			b.WriteString(base.Render(string(r)))
		}
	}
	return b.String()
}
