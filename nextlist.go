// nextlist.go — the Next List page: a project's living list of follow-ups
// (ai_docs/todo/next-list.md), shown as rows a prompt can be started from.
//
// The file is written and kept by the /next-list and /sess-save skills, not by
// this program, so the page only reads it. It is a Markdown document with a
// small fixed grammar, and only its two "still to do" sections are listed:
//
//	## Open                                         ← what is next
//	- **N-014** · raised `2026-0904-…` · value medium
//	  The item's text, indented two columns, over as
//	  many lines (and sub-bullets) as it needs.
//	## Roadmap                                      ← wanted, but later
//	## Non-goals / ## Closed                        ← not listed: nothing to start
//
// The page is a full-screen stage of the list, the same shape as the pickers:
//
//	Next list  ai_docs/todo/next-list.md · 12 open · 3 roadmap
//
//	╭ 🔍 query ───────────────────╮  15/15
//
//	  ✚ New prompt enter  ✉ Send shift+enter  ↻ Refresh ctrl+r  ← Back esc
//
//	Open
//	❯ N-001 Hands-on pass in a rebuilt, reinstalled Cats.app. Sessions run insi…
//	  N-002 Seeds: the demo backlog ships three prompts that no longer match th…
//
// One difference from the backlog's rows is deliberate: a row here is NOT split
// into a title and a dimmer body. A next-list item has no title — its first
// sentence is just the start of a paragraph — so the row is the item's own text,
// flattened to one line and cut only where the pane runs out.
//
// An item can leave the page two ways. ✚ New prompt (enter) opens the add form
// on it, to be tightened and kept in the backlog. ✉ Send (shift+enter) hands it
// straight to an agent through the backlog's own target picker, as a one-off
// prompt that is never written to any backlog: the item already has a home in
// the file, and a backlog copy would be a second record of the same work for
// someone to close. The agent is told the item's ID, so it can close it there.
//
// The file is re-read on open and on ↻ Refresh, never watched: it is edited in
// another pane (by a session wrapping up, usually), and a refresh the user asks
// for is both cheaper and more predictable than a list that reshuffles under a
// cursor that is walking it.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// nextListRel is where a project keeps its living list, relative to the
// project root — the path the next-list skill writes.
var nextListRel = filepath.Join("ai_docs", "todo", "next-list.md")

// nextItem is one follow-up from the file.
type nextItem struct {
	ID      string // "N-014" — permanent, and what a prompt made from it cites
	Value   string // "high" / "medium" / "low", or "" when the header has none
	Raised  string // the session-doc stem the item first appeared in
	Section string // "Open" or "Roadmap"
	// Text is the item's body with its two-column indent removed, newlines and
	// sub-bullets intact: the row flattens it, but a prompt made from it keeps
	// the shape it was written in.
	Text string
}

// The item header: a bullet at column 0 whose first bold run is the ID. The
// rest of the line is " · "-separated fields, parsed apart by the two
// expressions below it; anything that is neither field is text that happened
// to start on the header line, and is kept.
var (
	nextHeaderRe = regexp.MustCompile(`^- \*\*(N-\d+)\*\*(.*)$`)
	nextRaisedRe = regexp.MustCompile("^raised `([^`]*)`$")
	nextValueRe  = regexp.MustCompile(`^value (\w+)$`)
)

// nextListedSections are the sections the page shows, in the order it shows
// them. Non-goals and Closed hold items too, but nothing there is waiting to be
// started, and a page for picking what to do next has no use for them.
var nextListedSections = []string{"Open", "Roadmap"}

// parseNextList pulls the listed sections' items out of the file, in file
// order (which the file's own conventions keep in ID order).
//
// The grammar is line-based and forgiving, because the file is hand-edited as
// often as it is skill-written:
//
//   - A "## " heading switches the section; only the listed ones collect items.
//   - An item starts at a header bullet (nextHeaderRe) and continues over every
//     indented or blank line after it.
//   - Any other line at column 0 ends it — the next bullet, a heading, or the
//     paragraph of prose a section is allowed to open with.
//
// Trailing blank lines are trimmed off each item, so the gap before the next
// bullet does not become part of the text.
func parseNextList(src string) []nextItem {
	var (
		items   []nextItem
		section string
		cur     *nextItem
		body    []string
	)
	flush := func() {
		if cur == nil {
			return
		}
		for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
			body = body[:len(body)-1]
		}
		cur.Text = dedent(strings.Join(body, "\n"))
		items = append(items, *cur)
		cur, body = nil, nil
	}
	listed := func(s string) bool {
		for _, want := range nextListedSections {
			if strings.EqualFold(s, want) {
				return true
			}
		}
		return false
	}

	for _, ln := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(ln, "## ") {
			flush()
			section = strings.TrimSpace(strings.TrimPrefix(ln, "## "))
			continue
		}
		if cur != nil && (strings.TrimSpace(ln) == "" || ln[0] == ' ' || ln[0] == '\t') {
			body = append(body, ln)
			continue
		}
		flush()
		if !listed(section) {
			continue
		}
		mt := nextHeaderRe.FindStringSubmatch(ln)
		if mt == nil {
			continue
		}
		cur = &nextItem{ID: mt[1], Section: sectionName(section)}
		// The header's fields, then whatever is left of it: a hand-written item
		// may put its first words on the bullet line itself.
		var rest []string
		for _, f := range strings.Split(mt[2], "·") {
			f = strings.TrimSpace(f)
			switch {
			case f == "":
			case nextRaisedRe.MatchString(f):
				cur.Raised = nextRaisedRe.FindStringSubmatch(f)[1]
			case nextValueRe.MatchString(f):
				cur.Value = strings.ToLower(nextValueRe.FindStringSubmatch(f)[1])
			default:
				rest = append(rest, f)
			}
		}
		if len(rest) > 0 {
			// Two columns of indent, so dedent treats it like the lines below.
			body = append(body, "  "+strings.Join(rest, " · "))
		}
	}
	flush()
	return items
}

// sectionName is the heading as the page spells it — the canonical casing of a
// listed section, whatever case the file wrote it in.
func sectionName(s string) string {
	for _, want := range nextListedSections {
		if strings.EqualFold(s, want) {
			return want
		}
	}
	return s
}

// dedent removes the indent every non-blank line shares, so an item's body
// comes out flush-left with its nested bullets still nested.
func dedent(s string) string {
	lines := strings.Split(s, "\n")
	common := -1
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		n := len(ln) - len(strings.TrimLeft(ln, " \t"))
		if common < 0 || n < common {
			common = n
		}
	}
	if common <= 0 {
		return strings.TrimSpace(s)
	}
	for i, ln := range lines {
		if len(ln) >= common {
			lines[i] = ln[common:]
		} else {
			lines[i] = strings.TrimLeft(ln, " \t")
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// loadNextList reads the list under root. The error is a sentence for the
// page's empty state rather than an error value, the promptLibrary convention:
// each way of having nothing to show has its own answer, and the page says it
// where the rows would be.
func loadNextList(root string) (items []nextItem, path, errMsg string) {
	if root == "" {
		return nil, "", "no project here — the next list lives in a project's " + nextListRel
	}
	path = filepath.Join(root, nextListRel)
	data, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		return nil, path, "this project has no " + nextListRel + " — /next-list seed creates one"
	case err != nil:
		return nil, path, "cannot read " + nextListRel + ": " + err.Error()
	}
	items = parseNextList(string(data))
	if len(items) == 0 {
		return nil, path, "nothing Open or on the Roadmap in " + nextListRel
	}
	return items, path, ""
}

// nextPage is the open page: the items it was built from, the rows it is
// showing, and the one line of feedback the heading carries.
type nextPage struct {
	root  string
	path  string
	items []nextItem
	err   string // why there is nothing to list; "" when items were read
	note  string // the last action's outcome: a refresh, a refusal, a send
	// noteErr draws the note as a failure: a send that failed, or one refused
	// for want of a cats socket. Refusals the user can fix with a keystroke
	// (nothing highlighted) stay in the ordinary colour.
	noteErr bool
	list    fuzzyList
	width   int // the pane width the rows were cut to; see rebuild
}

// newNextPage reads the list under root and builds the page over it.
func newNextPage(root string) nextPage {
	// The placeholder stays inside searchFieldWidth; the bar under the box is
	// what says what enter does.
	p := nextPage{root: root, list: newFuzzyList("type to filter", nil)}
	p.items, p.path, p.err = loadNextList(root)
	return p
}

// reload re-reads the file, keeping the query and — when it is still there —
// the highlighted item, so a refresh after an edit elsewhere leaves the eye
// where it was.
func (p *nextPage) reload(width, height int) {
	keep, _ := p.highlighted()
	p.items, p.path, p.err = loadNextList(p.root)
	p.resize(width, height)
	for i, it := range p.items {
		if it.ID == keep.ID && keep.ID != "" {
			p.list.selectRef(i)
			break
		}
	}
	if p.err != "" {
		p.say("", false)
		return
	}
	p.say(fmt.Sprintf("refreshed · %d items", len(p.items)), false)
}

// say sets the heading's feedback line. The two fields change together so a
// failed send's red cannot outlive it onto the next, unrelated note.
func (p *nextPage) say(note string, isErr bool) {
	p.note, p.noteErr = note, isErr
}

// highlighted is the item under the cursor.
func (p nextPage) highlighted() (nextItem, bool) {
	i := p.list.selectedIndex()
	if i < 0 || i >= len(p.items) {
		return nextItem{}, false
	}
	return p.items[i], true
}

// nextBarRow and nextRowsRow are the page's two hit-tested lines: the heading
// (0), a blank (1), the boxed query line (2), the blank fuzzyList.view puts
// before a bar (3), the bar (4), the blank after it (5), and the first row (6).
// clickNext subtracts nextRowsRow from the pointer's row, so the geometry is
// pinned by a test against a rendered frame.
const (
	nextBarRow  = 4
	nextRowsRow = 6
)

// resize fits the page to the pane: the query box to the width, the window to
// what is left under the chrome, and the rows re-cut to the new width.
//
// The window takes back the lines the section headings spend (see
// separatorLines), since it counts items and the pane counts lines. Below the
// rows are a blank and the footer.
func (p *nextPage) resize(width, height int) {
	if w := width - 4; w >= 20 {
		p.list.input.SetWidth(min(w, searchFieldWidth))
	}
	p.width = width
	p.rebuild()
	rows := 0
	if height > 0 {
		rows = max(height-nextRowsRow-2-p.list.separatorLines(), 1)
	}
	p.list.setMaxRows(rows)
	// The window is only known now, and whether it overflows decides how much
	// of each row the scroll markers need (see rebuild) — so cut once more.
	p.rebuild()
}

// rebuild turns the items into rows, each cut to the pane.
//
// The row is the ID, a value mark, and then as much of the item's text as the
// line holds. The ID is the only badge: it is what a prompt made from the item
// cites. Its hue follows the item's value (bright for high, pale for medium,
// dim for low), but a hue alone was too quiet to read the value from — the
// three sit close together on the warm side of the ramp — so the value also
// gets a glyph of its own in the annotation slot after the ID (see
// nextValueMark). Every row spends the same two cells on it, so the text column
// stays straight and the marks line up to be scanned down the page.
//
// The text is cut here rather than left to the terminal: a row that wrapped
// would push every row under it a line lower than rowAtLine believes, and hand
// clicks to the wrong item. The cut leaves room for the cursor gutter, the ID,
// and — only when the list is scrolled — the ▴/▾ counts that ride the first and
// last rows, which withOverflowMark would otherwise have to drop for want of
// space.
func (p *nextPage) rebuild() {
	var items []listItem
	section := ""
	overflows := false
	if p.list.maxRows > 0 {
		overflows = len(p.items)+len(nextListedSections) > p.list.maxRows
	}
	for i, it := range p.items {
		if it.Section != section {
			section = it.Section
			items = append(items, listItem{name: section})
		}
		flat := collapseLines(it.Text)
		name := flat
		if p.width > 0 {
			// ID + its space, then the value mark + its space.
			room := p.width - indentWidth - lipgloss.Width(it.ID) - 1 - nextValueMarkWidth - 1
			if overflows {
				room -= 8 // "▾ 123" and the pad before it
			}
			name = truncate(flat, max(room, 8))
		}
		items = append(items, listItem{
			name:       name,
			badge:      it.ID,
			badgeStyle: nextValueStyle(it.Value),
			annots:     []annotMark{nextValueMark(it.Value)},
			// The haystack is the whole item, not the cut row, so a query finds
			// words from past the edge of the pane — plus the ID, the value and
			// the section, so "N-014", "high" and "roadmap" all narrow the page.
			search:     strings.Join([]string{it.ID, it.Value, it.Section, flat}, " "),
			selectable: true,
			ref:        i,
		})
	}
	p.list.setItems(items)
}

// nextValueStyle is the ID's hue for an item's value: cats' todo yellow for
// high (the list's own "this matters" tone, as on the ▲ priority mark), straw
// for medium, and the grey ramp's dim step for low or unrated.
func nextValueStyle(v string) lipgloss.Style {
	switch v {
	case "high":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colTodo)).Bold(true)
	case "medium":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colStraw))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colDim))
}

// nextValueMarkWidth is the cells every row's value mark takes, whatever the
// value: the gem is two cells wide, so the one-cell diamonds are padded to
// match it and an unrated item gets two blanks.
const nextValueMarkWidth = 2

// nextValueMark is the item's value drawn as a glyph, a descending ramp of one
// shape:
//
//	💎  high     the backlog's own High value gem — the same fact, the same mark
//	◆   medium   a solid diamond in straw: the gem's shape without its sparkle
//	◇   low      the diamond's outline in the dim grey — present, but hollow
//	    unrated  nothing, so an item the skill has not scored makes no claim
//
// The medium step is a text glyph rather than a "dimmer gem" because the gem
// is an emoji: terminals draw it in its own colours and ignore the foreground,
// so it cannot be faded. ◆ and ◇ are text, so the palette reaches them, and
// the solid → hollow step keeps them apart even where colour is not seen.
func nextValueMark(v string) annotMark {
	var glyph string
	st := lipgloss.NewStyle()
	switch v {
	case "high":
		glyph = valueGlyph
		st = valueStyle
	case "medium":
		glyph = nextMediumGlyph
		st = st.Foreground(lipgloss.Color(colStraw))
	case "low":
		glyph = nextLowGlyph
		st = st.Foreground(lipgloss.Color(colDim))
	}
	// Pad to the fixed width so the text starts in the same column on every
	// row (the diamonds are East Asian Ambiguous, one cell like the triangles).
	glyph += strings.Repeat(" ", max(nextValueMarkWidth-lipgloss.Width(glyph), 0))
	return annotMark{text: glyph, style: st, selStyle: st}
}

const (
	nextMediumGlyph = "◆"
	nextLowGlyph    = "◇"
)

// counts is the heading's tally: how many items each listed section holds. An
// empty section is left out rather than counted as "0 roadmap" — a freshly
// seeded list has an empty Roadmap, and the zero would be the heading's most
// prominent fact about it.
func (p nextPage) counts() string {
	n := map[string]int{}
	for _, it := range p.items {
		n[it.Section]++
	}
	var parts []string
	for _, s := range nextListedSections {
		if n[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n[s], strings.ToLower(s)))
		}
	}
	return strings.Join(parts, " · ")
}

// --- The page's buttons ---------------------------------------------------------

// Indexes into nextActions.
const (
	nextActionPrompt = iota
	nextActionSend
	nextActionRefresh
	nextActionBack
)

// nextActions is the page's bar. The same listAction chips the list's bar
// draws, so they shrink the same way (chords → words → glyphs) and a click on
// one is tested against the same spans the eye sees.
//
// ctrl+r means Refresh here, where on the list it is Import. The two never
// meet — this page has nothing to import into, and the list has nothing to
// refresh — and ctrl+r is the reload chord everywhere else a page is reloaded.
//
// ✉ Send wears the list's own Send chip — label, tint and chord — because it
// is the same act: shift+enter hands the highlighted thing to an agent on both
// screens. It is a method only for the chord, which is spelled the way this
// terminal can send it (modEnter).
func (m model) nextActions() []listAction {
	return []listAction{
		{label: "✚ New prompt", hint: "enter", tint: colInfo, needsSel: true},
		{label: "✉ Send", hint: m.modEnter(), tint: colAccent, needsSel: true},
		{label: "↻ Refresh", hint: "ctrl+r", tint: colCyan},
		{label: "← Back", hint: "esc", tint: colStraw},
	}
}

// nextBarTier is how much of each chip the bar prints at this width.
func (m model) nextBarTier() chipTier {
	return barTier(m.nextActions(), m.width, indentWidth)
}

// nextChips lays the bar out — actionChips' arithmetic over this page's
// buttons, from the same indent so the bar lines up with the rows.
func (m model) nextChips() []actionChip {
	tier := m.nextBarTier()
	var chips []actionChip
	x := indentWidth
	for i, a := range m.nextActions() {
		if i > 0 {
			x += chipGap(tier)
		}
		text := a.chipText(tier)
		w := lipgloss.Width(btnStyle.Render(text))
		chips = append(chips, actionChip{text: text, start: x, end: x + w})
		x += w
	}
	return chips
}

// nextBar renders the bar. New prompt is greyed with nothing highlighted, the
// list bar's rule: the chip answers "why did nothing happen" before it is
// pressed.
func (m model) nextBar() string {
	acts := m.nextActions()
	_, hasSel := m.next.highlighted()
	tier := m.nextBarTier()
	gap := strings.Repeat(" ", chipGap(tier))
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", indentWidth))
	for i, c := range m.nextChips() {
		if i > 0 {
			b.WriteString(gap)
		}
		st, hintFg := btnStyle.Foreground(lipgloss.Color(acts[i].tint)), colDim
		if acts[i].needsSel && !hasSel {
			st, hintFg = btnOffStyle, colFaint
		}
		b.WriteString(renderChipDimHint(st, hintFg, acts[i], tier, c.text))
	}
	return b.String()
}

// --- The stage -----------------------------------------------------------------

// nextListRoot is the project whose list the page shows: the project backlog's
// own root when there is one, else the directory the launch resolved — so a
// --global launch inside a project can still read that project's list, and
// the prompt it makes lands in the global backlog the launch is managing.
func (m model) nextListRoot() string {
	return firstNonEmpty(backlogRoot(m.project), m.ctx.projectDir())
}

// beginNextList opens the page. A project with no list still opens it: the
// empty state names the file and the skill that creates it, which is more use
// than a status line on the screen being left.
func (m model) beginNextList() (tea.Model, tea.Cmd) {
	m.clearHover()
	m.next = newNextPage(m.nextListRoot())
	m.next.resize(m.width, m.height)
	m.stage = stageNextList
	return m, textinput.Blink
}

// closeNextList goes back to the backlog.
func (m model) closeNextList() (tea.Model, tea.Cmd) {
	m.backToList()
	return m, nil
}

// refreshNextList re-reads the file (see nextPage.reload).
func (m model) refreshNextList() (tea.Model, tea.Cmd) {
	m.next.reload(m.width, m.height)
	return m, nil
}

// promptFromNext opens the add form holding the highlighted item: its ID and
// opening words as the title, and a prompt that names where the item came from
// ahead of its text. The citation is for the agent that receives it — an
// agent told which item it is working on can close it in the file when it is
// done, which is the living list's whole bargain.
//
// It goes through the add form rather than straight into the backlog so the
// words can be tightened before they are kept; esc there throws the draft away
// and nothing was written.
func (m model) promptFromNext() (tea.Model, tea.Cmd) {
	it, ok := m.next.highlighted()
	if !ok {
		// Refuse in words: an empty page, or a query that matched nothing.
		m.next.say("highlight an item first — ↑/↓ to choose one", false)
		return m, nil
	}
	return m.beginAddWith(nextItemTitle(it), nextItemPrompt(it))
}

// nextItemTitle and nextItemPrompt are what an item becomes as a prompt, the
// same whether it goes through the add form or straight to an agent: the ID
// and opening words as the title (which also names a sent item's tab, and its
// branch on a worktree drop), and the text under a line citing where it came
// from.
func nextItemTitle(it nextItem) string {
	return it.ID + " " + truncate(collapseLines(it.Text), 60)
}

func nextItemPrompt(it nextItem) string {
	return "Next list item " + it.ID + " (" + filepath.ToSlash(nextListRel) + "):\n\n" + it.Text
}

// nextSend is a Next List item on its way to an agent: the prompt the picker
// sends (a Todo, because every drop path speaks Todo, but one with no ID and
// no store) and the item's own ID, which the result reports back.
type nextSend struct {
	id   string
	todo Todo
}

// sendFromNext is ✉ Send (shift+enter): open the target picker on the
// highlighted item, as a prompt that exists only for this drop.
//
// The item is not saved to a backlog first, which is where this parts from the
// form's ✉ Send (save, then drop). Saving would leave a backlog row for work
// the file already tracks, and an esc out of the picker would leave it there
// with nothing sent. So the prompt rides in nextDrop instead of a todoRef, and
// with no row there is nothing to mark done after the drop: the item is closed
// in the file, by the agent that did it or by the next session wrap.
//
// The refusals are the list's own (startDrop's), said on this page's heading
// rather than in the status line, which is not on this screen. A Next List
// item is never frozen or info-marked, so those two guards have no counterpart.
func (m model) sendFromNext() (tea.Model, tea.Cmd) {
	it, ok := m.next.highlighted()
	switch {
	case !ok:
		m.next.say("highlight an item first — ↑/↓ to choose one", false)
		return m, nil
	case m.dropping:
		m.next.say("a drop is still in progress…", false)
		return m, nil
	case m.client == nil:
		m.next.say("cats control socket unavailable — can't send to a session", true)
		return m, nil
	}
	m.nextDrop = &nextSend{id: it.ID, todo: Todo{Title: nextItemTitle(it), Prompt: nextItemPrompt(it)}}
	m.dropTodo = todoRef{}
	m.pickForSchedule = false
	m.targets, m.targetList = m.buildTargets()
	m.stage = stageTarget
	return m, textinput.Blink
}

// chooseNextTarget is chooseTarget's ending for a Next List item: the drop is
// dispatched the same way, from the same pendingAction, but the model goes back
// to the page it came from, and the result carries the item's ID instead of a
// todoRef (see finishNextDrop).
//
// A new session opens in the list's project, which is the directory the item
// was written about; the launch's own directory is the fallback, and the same
// directory in every case but a --global launch from outside a project.
func (m model) chooseNextTarget(target dropTarget, mode dropMode) (tea.Model, tea.Cmd) {
	id, td := m.nextDrop.id, m.nextDrop.todo
	m.nextDrop = nil
	m.dropping = true
	m.stage = stageNextList
	m.next.resize(m.width, m.height) // the pane may have changed under the picker
	verb := "sending to "
	if mode == dropPaste {
		verb = "pasting into "
	}
	m.next.say(verb+targetDesc(target)+"…", false)
	act := pendingAction{
		todo:       td,
		target:     target,
		mode:       mode,
		cwd:        firstNonEmpty(m.next.root, m.ctx.projectDir()),
		anchorPane: m.ctx.OwnPaneID,
	}
	client, desc := m.client, targetDesc(target)
	return m, func() tea.Msg {
		return dropResultMsg{desc: desc, mode: mode, nextID: id, err: performDrop(client, act)}
	}
}

// finishNextDrop reports a Next List send. The status line gets it too, since
// the user may have left the page by the time a slow new-session drop lands;
// the page's heading gets it because that is the line in view while they are
// still on it.
func (m model) finishNextDrop(msg dropResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		line := "send failed: " + msg.err.Error()
		m.setStatus(line, true)
		m.next.say(line, true)
		return m, nil
	}
	line := msg.nextID + " " + dropDoneStatus(msg)
	m.setStatus(line, false)
	m.next.say(line, false)
	return m, nil
}

// updateNextList is the page's key loop: the list's keys, the bar's chords, and
// everything else, which is the query.
func (m model) updateNextList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		// Clear a query before leaving, the list's own esc ladder: the filter
		// is a state to back out of, and leaving is the last resort.
		if strings.TrimSpace(m.next.list.input.Value()) != "" {
			m.next.list.input.SetValue("")
			m.next.list.filter()
			return m, nil
		}
		return m.closeNextList()
	case "up", "ctrl+p":
		m.next.list.moveUp()
		return m, nil
	case "down", "ctrl+n":
		m.next.list.moveDown()
		return m, nil
	case "pgup", "pgdown":
		// A page is the window's height; with no window yet, one row is as far
		// as a page can be trusted to go (the pickers' rule).
		step := max(m.next.list.maxRows, 1)
		for range step {
			if msg.String() == "pgup" {
				m.next.list.moveUp()
			} else {
				m.next.list.moveDown()
			}
		}
		return m, nil
	case "enter":
		return m.promptFromNext()
	case "shift+enter", "alt+enter":
		// The list's drop chord and its legacy alias (see modEnter), meaning
		// the same thing here: this item, to an agent.
		return m.sendFromNext()
	case "ctrl+r":
		return m.refreshNextList()
	}
	m.next.say("", false)
	return m, m.next.list.editQuery(msg)
}

// clickNext is the pointer on the page: a chip presses its button, and a row is
// chosen the way the list's rows are — the first click highlights, a second on
// the same row within the double-click window opens it, so reading a row with
// the pointer never starts a prompt by accident.
func (m model) clickNext(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Y == nextBarRow {
		for i, c := range m.nextChips() {
			if msg.X < c.start || msg.X >= c.end {
				continue
			}
			switch i {
			case nextActionPrompt:
				return m.promptFromNext()
			case nextActionSend:
				return m.sendFromNext()
			case nextActionRefresh:
				return m.refreshNextList()
			case nextActionBack:
				return m.closeNextList()
			}
		}
		return m, nil
	}
	i, ok := m.next.list.rowAtLine(msg.Y - nextRowsRow)
	if !ok || !m.next.list.focusRow(i) {
		return m, nil
	}
	// The list's double-click state is borrowed rather than duplicated: the
	// two screens are never up together, and the stage change that leaves
	// either one takes longer than the window, so a click on one can never
	// pair with a click on the other.
	if i == m.lastClickRow && time.Since(m.lastClickAt) < doubleClickWindow {
		m.lastClickAt = time.Time{} // a third click starts over, not another open
		return m.promptFromNext()
	}
	m.lastClickRow, m.lastClickAt = i, time.Now()
	return m, nil
}

// viewNextList draws the page: the heading, the query box, the bar, the rows,
// and a footer of keys. The heading is one line by construction — every row
// constant below it depends on that.
func (m model) viewNextList() string {
	var b strings.Builder
	const title = "Next list"
	b.WriteString(titleStyle.Render(title))
	b.WriteString("  ")
	side, style := filepath.ToSlash(nextListRel), descStyle
	if m.next.err == "" {
		side += " · " + m.next.counts()
	}
	if m.next.note != "" {
		side, style = m.next.note, okStyle
		if m.next.noteErr {
			style = errStyle
		}
	}
	if m.width > 0 {
		if room := m.width - lipgloss.Width(titleStyle.Render(title)) - 4; room > 1 {
			side = truncate(side, room)
		}
	}
	b.WriteString(style.Render(side))
	b.WriteString("\n\n")
	empty := firstNonEmpty(m.next.err, "nothing matched")
	b.WriteString(m.next.list.view(empty, m.nextBar(), m.width))
	b.WriteString("\n")
	segs := []string{"dbl-click new prompt", "↑/↓ choose", "type to filter"}
	if m.nextBarTier() != tierHints {
		// The chips stopped teaching their chords, so the footer takes over.
		segs = append([]string{"enter new prompt", m.modEnter() + " send", "ctrl+r refresh", "esc back"}, segs...)
	}
	b.WriteString(footerStyle.Render(m.fitFooter(segs)))
	return b.String()
}
