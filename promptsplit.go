// promptsplit.go — turning a markdown bulleted list in the prompt editor into
// one prompt per bullet.
//
// A backlog item often arrives as a list: a plan pasted out of a chat, a set of
// review notes, the checklist at the bottom of an issue. Every one of those
// bullets is a prompt an agent could be handed on its own, but only if it is a
// todo of its own — a single item holding six bullets can be dropped once,
// scheduled once and marked done once, which is exactly the wrong granularity
// for six pieces of work. This file is the one gesture that fixes that: sweep
// the list and press ctrl+x — or take ✂ Split into prompts off the editor's
// context menu (promptmenu.go) — and each bullet becomes its own prompt in the
// backlog.
//
// The shape of it, on a selection covering the whole list:
//
//	Prompt                             Backlog
//	──────────────────────────         ──────────────────────
//	Ship the release:                  Ship the release:
//	- tag v2                    ──▶    ├─ tag v2
//	- write the notes                  ├─ write the notes
//	  - link the diff                  │    - link the diff
//	- announce it                      └─ announce it
//
// Two rules carry the whole feature:
//
//   - The split consumes exactly the selection, and only from its first bullet
//     onwards. Anything swept in above that first bullet ("Ship the release:"
//     above) is not part of the list, so it stays in the editor rather than
//     being folded into a prompt or quietly dropped.
//   - What is left behind decides what happens to the original. If the editor
//     still holds text, the form stays open on it; if the list *was* the whole
//     body, there is nothing left to be a prompt and the original is deleted —
//     the user asked for these prompts *instead of* that one, not beside it.
//
// New prompts inherit the backlog scope, the annotations and the session
// options of the prompt they came out of — everything that says how the work
// should be run, which is the same for every bullet of one list. Attachments are
// deliberately not inherited: an image belongs to the prompt it illustrates, and
// copying it N times would put N copies on disk for prompts that mostly do not
// want it.

package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// bulletRe matches one markdown list item's opening line: leading whitespace, a
// marker, and the item's text.
//
// Both markdown bullet families are accepted — the unordered `-`, `*`, `+` and
// the ordered `1.` / `1)` — because a pasted list is whichever one its source
// used, and refusing half of them would make the feature look broken rather than
// strict. The ordered marker's digit run is bounded so a line that opens with a
// long number ("2024-08-29 was the deadline") cannot be read as an item.
//
// The text after the marker is optional so that a bare "-" still matches and is
// then discarded as an empty item (see splitBulletList) rather than being
// treated as prose. What it must NOT match is a horizontal rule: "---" fails
// here because the group after the marker requires whitespace before anything
// else, and "--" is not whitespace.
var bulletRe = regexp.MustCompile(`^([ \t]*)([-*+]|\d{1,9}[.)])([ \t]+(.*))?$`)

// bulletItem is one opening line of a list item, resolved into the two columns
// the split needs: where the marker sits, and where the item's own text begins.
//
//	"  - write the notes"
//	   ↑  ↑
//	   │  └── content = 4: what a continuation line is dedented to
//	   └───── indent  = 2: what decides whether the next bullet is a sibling
type bulletItem struct {
	indent  int    // display column the marker starts at
	content int    // display column the item's text starts at
	prefix  string // everything before the text: the indent, the marker, its separator
	text    string // the item's own first line, marker stripped
}

// parseBullet reads one line as a list item's opening line.
func parseBullet(line string) (bulletItem, bool) {
	mm := bulletRe.FindStringSubmatch(line)
	if mm == nil {
		return bulletItem{}, false
	}
	lead, marker := mm[1], mm[2]
	indent := textWidth(lead)
	// The content column is where the text after the marker begins, which is the
	// whole prefix's width — the indent, the marker, and the run of spaces or
	// tabs separating them from the text.
	prefix := mm[1] + marker + strings.TrimSuffix(mm[3], mm[4])
	// The prefix is kept verbatim rather than rebuilt from the marker, because
	// the sort puts items back into the markers they came in (see
	// sortPromptLines) and "1. " must not come back as "1." or "1.  ".
	return bulletItem{indent: indent, content: textWidth(prefix), prefix: prefix, text: mm[4]}, true
}

// textWidth is the display width of a run of leading whitespace, with a tab
// counted as four columns.
//
// Columns rather than characters, because indent is what tells a nested bullet
// from a sibling, and a file that indents its sub-lists with tabs would
// otherwise report every level as one character deep. Four is a choice rather
// than a fact — a terminal's tab stops are eight, an editor's are usually two or
// four — and it is a safe one here because nothing compares a tab-indented line
// against a space-indented one except a list that mixes the two, which is
// already ambiguous in every markdown renderer.
func textWidth(s string) int {
	w := 0
	for _, r := range s {
		if r == '\t' {
			w += 4
			continue
		}
		w++
	}
	return w
}

// dropLead removes up to cols columns of leading whitespace from s — the
// dedent that turns a nested sub-list into a top-level one inside its own new
// prompt. Whitespace only: a line is never cut into.
func dropLead(s string, cols int) string {
	w := 0
	for i, r := range s {
		if w >= cols || (r != ' ' && r != '\t') {
			return s[i:]
		}
		if r == '\t' {
			w += 4
			continue
		}
		w++
	}
	return ""
}

// bulletBlock is one item of a swept list, taken apart into the three pieces its
// two callers need between them: the opening line's prefix through the marker,
// the column the item's own text sits at, and that text with every continuation
// line dedented to it.
//
// The split (splitPromptList) wants only body — a new prompt is the item's words,
// not its bullet. The sort (sortPromptLines) wants all three, because it puts
// the bodies back into the markers where they were: that is what renumbers
// "1. 2. 3." correctly instead of shuffling the numbers along with the text.
type bulletBlock struct {
	marker  string // the opening line up to and including the marker's separator
	content int    // display column body starts at — what render re-indents to
	body    string // the item's text, continuation lines dedented and included
}

// render is the item put back as markdown: the marker, its first line, and every
// continuation line re-indented to the marker's content column. A blank line
// comes back blank rather than as a run of spaces — trailing whitespace is not
// something this program should be adding to a prompt.
func (bb bulletBlock) render() string {
	lines := strings.Split(bb.body, "\n")
	var b strings.Builder
	b.WriteString(bb.marker)
	b.WriteString(lines[0])
	pad := strings.Repeat(" ", bb.content)
	for _, ln := range lines[1:] {
		b.WriteString("\n")
		if ln != "" {
			b.WriteString(pad)
			b.WriteString(ln)
		}
	}
	return b.String()
}

// sortKey is what an item is ordered by: its first line, cased and spaced out of
// the comparison. Only the first line, because that is the line a reader sees on
// the list and therefore the one they mean by "sort it" — an item's sub-points
// travel with it rather than deciding where it goes.
func (bb bulletBlock) sortKey() string {
	first, _, _ := strings.Cut(bb.body, "\n")
	return strings.ToLower(strings.TrimSpace(first))
}

// splitBulletList reads a swept run of the prompt as a markdown list and returns
// the body of each item, along with head — how many runes at the front of text
// come before the first bullet and are therefore *not* part of the list.
//
// head is what keeps the gesture honest about text the sweep caught by accident.
// A hand selecting a list from the line above it, or from the middle of the
// sentence that introduces it, has not asked for that sentence to become a
// prompt or to disappear; returning its length lets the caller consume only from
// the first bullet on and leave the rest where it was.
//
// Grouping, given the first bullet's indent as the list's level:
//
//	│ - write the notes      ← a bullet at that level: a new item
//	│   - link the diff      ← deeper: part of the item above it
//	│   see the changelog    ← not a bullet at all: part of the item above it
//	│ - announce it          ← back at the level: a new item
//
// A deeper bullet is not promoted to an item of its own because a sub-list is
// the detail of its parent, not a peer of it — splitting "write the notes" away
// from "link the diff" would leave two prompts neither of which says the whole
// task. Both continuation forms are dedented to the parent's content column, so
// the sub-list arrives in the new prompt as a list rather than as an indented
// block whose indentation no longer means anything.
func splitBulletList(text string) (head int, items []bulletBlock) {
	lines := strings.Split(text, "\n")

	// The first opening line is where the list — and the consumed run — starts.
	first := -1
	var base bulletItem
	for i, ln := range lines {
		if b, ok := parseBullet(ln); ok {
			first, base = i, b
			break
		}
	}
	if first < 0 {
		return len([]rune(text)), nil
	}
	for _, ln := range lines[:first] {
		head += len([]rune(ln)) + 1 // +1 for the '\n' that ended the line
	}

	// Each item is accumulated as its own lines and joined at the end, so a
	// trailing blank line (the gap between two items, which lands on the first
	// of them) can be trimmed off without a second pass over the text.
	var cur bulletBlock
	var curLines []string
	flush := func() {
		cur.body = strings.TrimRight(strings.Join(curLines, "\n"), " \t\n")
		if cur.body != "" {
			items = append(items, cur)
		}
		curLines = nil
	}
	for _, ln := range lines[first:] {
		b, ok := parseBullet(ln)
		switch {
		case ok && b.indent <= base.indent:
			// A bullet at the list's own level — or shallower, which a sweep
			// that started inside a sub-list can produce — closes the item
			// before it and opens the next.
			flush()
			cur = bulletBlock{marker: b.prefix, content: b.content}
			curLines = []string{b.text}
		default:
			curLines = append(curLines, dropLead(ln, cur.content))
		}
	}
	flush()
	return head, items
}

// splitPromptList is ctrl+x, and ✂ Split into prompts on the editor's context
// menu: make one backlog prompt out of every bullet of the selected list.
//
// It writes to the backlog immediately rather than waiting for a save, and that
// is the point of it — the gesture is "these are separate items now", and an
// item that only exists once the form is saved would leave the editor holding a
// list it has already been told is gone. The write is one store operation
// (addAfter), so a run of prompts either all land or none do.
//
// Every refusal below says why. The chord sits on a screen the user is typing
// into with both hands, and a key that quietly does nothing there reads as a
// broken editor rather than as a gesture that did not apply.
func (m model) splitPromptList() (tea.Model, tea.Cmd) {
	if m.formFocus != formFieldPrompt {
		// The title is one line — it cannot hold a list — and the annotation bar
		// is not text at all. Same answer duplicatePromptLine gives from the same
		// two stops.
		m.formNote = "splitting a list works in the prompt"
		return m, nil
	}
	lo, hi, ok := m.promptSelSpan()
	if !ok {
		m.formNote = "nothing selected — sweep a bulleted list, or hold shift with ←/→, then ctrl+x"
		return m, nil
	}
	runes := []rune(m.promptArea.Value())
	head, items := splitBulletList(string(runes[lo:hi]))
	if len(items) == 0 {
		m.formNote = "no bulleted list in the selection — items start with -, * or + (or 1.)"
		return m, nil
	}

	st := m.storeFor(m.formScope)
	// The same silent-no-op hazard persistForm guards: an unavailable store's
	// save reports success and writes nothing, so the prompts would be announced
	// and then be gone.
	if !st.available() {
		m.formErr = "no " + strings.ToLower(m.formScope.String()) + " backlog is available here"
		return m, nil
	}

	// One Created for the whole run: the bullets were written as one list and
	// split in one gesture, so a timestamp that walked forward item by item would
	// be recording the loop rather than the act.
	now := time.Now()
	tds := make([]Todo, 0, len(items))
	for _, it := range items {
		td := Todo{
			ID:      newID(),
			Title:   firstLine(it.body, 60),
			Prompt:  it.body,
			Session: sessionPtr(m.formSession),
			Created: now,
		}
		// Applied as a set for the reason persistForm applies it as a set: a mark
		// added to annots later must not have to be remembered here too.
		m.formAnnots.applyTo(&td)
		tds = append(tds, td)
	}

	// Behind the prompt they came out of, so the new items land where the list
	// was rather than at the far end of a long backlog. In add mode there is no
	// such prompt yet and editID is empty, which addAfter reads as "append".
	if err := st.addAfter(m.editID, tds); err != nil {
		m.formErr = "split failed: " + err.Error()
		return m, nil
	}

	// What the editor would hold once the list is taken out of it. The head runs
	// from the selection's start to its first bullet and is never consumed, so
	// the cut is [lo+head, hi).
	cut := lo + head
	rest := string(runes[:cut]) + string(runes[hi:])
	if strings.TrimSpace(rest) != "" {
		// Part of the prompt survives, so the form stays open on it: the split
		// took a list out of a prompt that is still being written, and dropping
		// the user back on the list would cost them the rest of the edit.
		m.replacePromptRunes(cut, hi, "")
		m.clearPromptSel()
		m.formNote = splitNote(len(items)) + " · the rest is still here, unsaved"
		return m, nil
	}

	// The list was the whole body. There is no prompt left to save, so the
	// original is replaced rather than kept beside its own pieces.
	note := splitNote(len(items))
	if m.formMode == formEdit {
		if err := st.delete(m.editID); err != nil {
			// The prompts exist; only the original's removal failed. Say exactly
			// that and stay put — going back to the list would leave the user
			// looking at a duplicate with no explanation of where it came from.
			m.formErr = note + ", but removing the original failed: " + err.Error()
			return m, nil
		}
		if n := m.imageCountNote(); n != "" {
			// delete() takes the attachment directory with it, and the new
			// prompts did not inherit it (see the file header). That is a real
			// loss and it is not visible anywhere on the list, so it is said.
			note += " · " + n + " removed with it"
		}
	}
	m.setStatus(note, false)
	m.discardClipboardCaptures()
	m.rebuildList()
	m.backToList()
	return m, nil
}

// splitNote is the one place the count is put into words, so the status line
// after a whole-body split and the note after a partial one cannot disagree
// about what just happened.
func splitNote(n int) string {
	if n == 1 {
		return "split out 1 prompt"
	}
	return fmt.Sprintf("split out %d prompts", n)
}
