// listwrap.go — ☑ Soft wrap on the backlog list.
//
// A backlog row is one line: the title, then the prompt's first line cut at 70
// runes as its description. Everything past that (the rest of the prompt, its
// bullets, its code) has been behind the hover card, ctrl+v, or the editor.
// The toggle (alt+z, or the chip flush right on the bar) draws the whole
// prompt under each row instead, the way the Next List's own toggle does for
// its items (nextlist.go), on the same list machinery: continuation lines
// (listItem.more) indented to the name's column, and a scroll window counted
// in screen lines (fuzzyList.maxLines) rather than rows.
//
//	 ❯ ○ ▲ Fix the drop timeout  ⚙                       ← the row: title + marks,
//	       The 12s wait comes from stale ready probes —     no cut description
//	       capture a startup and check claudeReadyProbes  ← the prompt, wrapped at
//	       - then the panesetup pattern                     the pane's edge, line
//	   ○ Tidy the README                                    breaks kept
//	       Merge the two Soft wrap paragraphs.
//
// The prompt's lines keep their breaks and their leading indent (a list's
// sub-bullets are the structure of the thought), and a line too long for the
// pane hangs its wrapped rest under its own indent rather than at the margin.
// Blank lines are dropped: a paragraph break is not worth a row of a list
// that is being scanned.
//
// With the wrap on there is no hover card (listhover.go): the card's main job
// is the body the row cannot show, and that body is now on the screen — a
// card would repeat it, and cover the rows being read.
//
// Off by default, so the list draws as it always has until someone asks;
// remembered in settings.json as listSoftWrap, apart from the editor's and the
// Next List's toggles.

package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// listWrapRow turns one todo's row into its wrapped form: the name the first
// line keeps, and the prompt as continuation lines, at most maxMore of them.
//
// A titled prompt keeps its title as the name, and every line of the prompt
// goes under it. An untitled one has only the prompt to be called by, so the
// prompt's first wrapped line becomes the name — the same text the unwrapped
// row leads with — and the rest goes under it, rather than the first line
// being drawn twice.
//
// room is the width the continuation lines have: the pane, less the column
// the name starts at. maxMore caps them so one long prompt cannot run past
// the window (a row taller than the window is drawn whole and overruns the
// pane — see fuzzyList.windowEnd); the last kept line then says there was
// more with an ellipsis, and ctrl+v reads the rest.
func listWrapRow(title, prompt string, room, maxMore int) (name string, more []string) {
	lines := listWrapPrompt(prompt, max(room, 8))
	name = title
	if name == "" && len(lines) > 0 {
		name, lines = strings.TrimSpace(lines[0]), lines[1:]
	}
	if maxMore >= 0 && len(lines) > maxMore {
		lines = lines[:maxMore]
		if n := len(lines); n > 0 {
			lines[n-1] = truncate(lines[n-1], max(room-1, 1)) + "…"
		}
	}
	return name, lines
}

// listWrapPrompt wraps a prompt at width cells, one logical line at a time.
//
// wrapText (nextlist.go) does the breaking, so a line breaks where the Next
// List and the editor would break the same text. What it adds is the indent:
// wrapText trims every line, which suits a flattened paragraph but would push
// a prompt's sub-bullets back to the margin. So the leading whitespace is
// taken off first, the rest wrapped in what is left of the width, and the
// indent put back on every piece — a wrapped bullet hangs under its own text.
// A tab counts as four spaces, the editor's own indent (promptindent.go).
func listWrapPrompt(prompt string, width int) []string {
	var out []string
	for _, ln := range strings.Split(strings.ReplaceAll(prompt, "\t", "    "), "\n") {
		body := strings.TrimLeft(ln, " ")
		if strings.TrimSpace(body) == "" {
			continue
		}
		// An indent wider than half the line would leave the text a sliver
		// to wrap in; past that it is capped, and the line wraps in the rest.
		indent := min(len(ln)-len(body), width/2)
		pad := strings.Repeat(" ", indent)
		for _, piece := range wrapText(body, max(width-indent, 8)) {
			out = append(out, pad+piece)
		}
	}
	return out
}

// listNameCol is the column a backlog row's name starts at, which is where
// its continuation lines are indented to (rowsView's nameCol): the cursor
// gutter, the ✓ column when a selection is held, the badge and its space, and
// each annotation mark and its space. Measured here from the same parts
// rowsView draws, since the room to wrap in has to be known before the row is
// built.
func listNameCol(it listItem, marks bool) int {
	col := indentWidth
	if marks {
		col += lipgloss.Width(markGlyph) + 1
	}
	if it.badge != "" {
		col += lipgloss.Width(it.badge) + 1
	}
	for _, an := range it.annots {
		col += lipgloss.Width(an.text) + 1
	}
	return col
}

// listWrapLines is the line window the backlog gets with the wrap on: the
// pane's height less the chrome above and below the rows — sizeListWindow's
// budget, before the separator lines it hands back, since a line window
// counts the headings itself.
func (m model) listWrapLines() int {
	return max(m.height-listRowsRow-listChromeBelow, 1)
}

// wrapListItem applies the wrap to a built row when the toggle is on and the
// pane's width is known. It is done after the row is built, rather than inside
// rebuildList's long appendTodo, so the unwrapped row stays exactly what it
// was and this is the only place the two differ: the name, the continuation
// lines, and no cut description (its text is now the first of those lines).
func (m model) wrapListItem(it listItem, title, prompt string, marks bool) listItem {
	if !m.listSoftWrap || m.width <= 0 {
		return it
	}
	maxMore := -1 // no window yet (no height): nothing to overrun
	if m.height > 0 {
		maxMore = m.listWrapLines() - 1 // the row's own first line
	}
	room := m.width - listNameCol(it, marks)
	it.name, it.more = listWrapRow(title, prompt, room, maxMore)
	it.desc = ""
	it.moreDesc = true
	return it
}

// toggleListWrap is the list's chip and alt+z: flip the wrap, rebuild the rows
// (each prompt is one line or several now, and rebuildList re-sizes the window
// to count lines or rows — sizeListWindow), remember it, and say so on the
// status line, since with short prompts the change can be hard to see.
//
// A failed save keeps the toggle and says so, as toggleNextWrap does.
func (m model) toggleListWrap() (tea.Model, tea.Cmd) {
	m.listSoftWrap = !m.listSoftWrap
	m.rebuildList()
	note := "soft wrap off · one row per prompt"
	if m.listSoftWrap {
		note = "soft wrap on · each prompt's whole text"
	}
	m.setStatus(note, false)
	s := loadSettings()
	s.listSoftWrap = m.listSoftWrap
	if err := s.save(); err != nil {
		m.setStatus("soft wrap preference not saved: "+err.Error(), true)
	}
	return m, nil
}
