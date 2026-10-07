// nextedit.go — moving one item between the sections of the Next List file
// (ai_docs/todo/next-list.md): among Open, Validate and Roadmap, or out to
// Closed or Non-goals.
//
// The page used to only read the file, leaving every change to the skills that
// keep it (/next-list, /sess-save). But four of those changes are decisions
// only the user makes, and the page is where the user is looking when they make
// them: "this is done", "only a check is left", "not now", "never". So the page
// writes those four, and nothing else — it never re-rates, rewords, renumbers
// or deletes.
//
// The file's own conventions (its "## Conventions" section) are the spec this
// follows:
//
//	Open, Validate,  in ID order, items separated by a blank line;
//	Roadmap          an item moved among them keeps its lines verbatim
//	Non-goals        in ID order, items packed (no blank line between them),
//	                 `- **N-###** · declined <date> · raised `<stem>``
//	                   — <the reason>
//	Closed           newest first (an item goes on top), packed,
//	                 `- **N-###** · closed <date> · raised `<stem>``
//	                   — <what showed it>
//
// "Nothing leaves Open, Validate or Roadmap without a line in another
// section": a closed or declined item always gets a body. When the user gave
// no words, the item's own text stands in, so the record still says what was
// closed or declined.
//
// The edit is a splice on the file as it is *now*, re-read at the moment of
// writing, and keyed by the item's ID — never a re-render of what the page
// parsed earlier. The file is edited in other panes (a session wrapping up,
// usually), and a splice by ID leaves every line it does not own exactly as it
// found it, including whatever another writer changed since the page loaded.
//
//	read ─► find the item's block (header + indented lines) in a listed section
//	     ─► cut it out (and the blank line it leaves doubled)
//	     ─► build the block the destination wants
//	     ─► find the destination section (creating it if the file lacks it)
//	     ─► insert at its place (ID order, or the top of Closed)
//	     ─► write a temp file beside it and rename over (never a torn file)

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// nextDest is a section an item can be moved to. The values index
// nextSectionOrder, so the two lists are kept in the same order: a
// destination's iota is its place in the file.
type nextDest int

const (
	nextToOpen nextDest = iota
	nextToValidate
	nextToRoadmap
	nextToNonGoals
	nextToClosed
)

// nextSectionOrder is the file's section order, which is also how a missing
// section is placed: before the first one that should come after it. An older
// file may lack Validate or Roadmap (the skill says to treat either as empty),
// and a young one may have no Non-goals yet. Validate sits directly below
// Open, as the next-list skill places it: both are what is picked up next,
// split only by the kind of work left (build vs. a check to run).
var nextSectionOrder = []string{"Open", "Validate", "Roadmap", "Non-goals", "Closed"}

// section is the heading a destination writes under.
func (d nextDest) section() string { return nextSectionOrder[d] }

// spaced says the section separates its items with a blank line — the three
// "still to do" sections, where an item is a paragraph to read. The record
// sections are packed, one entry straight after another.
func (d nextDest) spaced() bool { return !d.record() }

// record says the destination is one of the record sections (Closed,
// Non-goals), where an item arrives as a stamped entry rather than verbatim,
// and leaves the page.
func (d nextDest) record() bool { return d == nextToClosed || d == nextToNonGoals }

// nextMoveReq is one move: which item, where to, and the user's words for the
// record (a closing comment, or a non-goal's reason). Date is the stamp the
// record carries, passed in rather than read from the clock so a test can pin
// it.
type nextMoveReq struct {
	ID   string
	To   nextDest
	Note string
	Date string // "2006-01-02"
}

// nextIDNumRe pulls the number out of an item header, for the ID-order rule.
var nextIDNumRe = regexp.MustCompile(`^- \*\*N-(\d+)\*\*`)

// nextRecordWrap is the column a written record's lines are wrapped to — the
// width the file's own hand- and skill-written paragraphs keep.
const nextRecordWrap = 76

// moveNextItemText applies req to the file's text and returns the new text,
// along with the item as it was parsed before the move.
//
// Refusals are errors worded for the page's heading: the item is not in a
// listed section any more (someone else moved it since the page loaded), or
// the move is to where it already is.
func moveNextItemText(src string, req nextMoveReq) (string, nextItem, error) {
	// Line endings: a CRLF file is edited as LF and given its CRLFs back, so a
	// Windows-saved list is not rewritten end to end by a one-item move.
	crlf := strings.Contains(src, "\r\n")
	src = strings.ReplaceAll(src, "\r\n", "\n")

	var it nextItem
	found := false
	for _, x := range parseNextList(src) {
		if x.ID == req.ID {
			it, found = x, true
			break
		}
	}
	if !found {
		return "", nextItem{}, fmt.Errorf("%s is no longer Open, in Validate or on the Roadmap in %s — ↻ refresh", req.ID, filepath.ToSlash(nextListRel))
	}
	if it.Section == req.To.section() {
		return "", nextItem{}, fmt.Errorf("%s is already in %s", req.ID, it.Section)
	}

	lines := strings.Split(src, "\n")
	s, e, ok := findNextBlock(lines, req.ID)
	if !ok {
		// parseNextList found it, so this is a grammar the two readers disagree
		// on; refuse rather than guess which lines are the item.
		return "", nextItem{}, fmt.Errorf("could not find %s's lines in %s", req.ID, filepath.ToSlash(nextListRel))
	}
	block := append([]string(nil), lines[s:e]...)

	// Cut. The item was one of a run of blank-separated paragraphs, so taking
	// it out leaves two blanks where one belongs; drop the second.
	lines = append(lines[:s:s], lines[e:]...)
	if s > 0 && s < len(lines) && isBlank(lines[s-1]) && isBlank(lines[s]) {
		lines = append(lines[:s], lines[s+1:]...)
	}

	switch req.To {
	case nextToClosed:
		block = nextRecordBlock(it, "closed "+req.Date, req.Note)
	case nextToNonGoals:
		block = nextRecordBlock(it, "declined "+req.Date, req.Note)
	}
	lines = insertNextBlock(lines, req.To, nextIDNum(req.ID), block)

	out := strings.Join(lines, "\n")
	if crlf {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	return out, it, nil
}

// findNextBlock locates an item's lines: the header bullet in a listed section
// (Open, Validate, Roadmap), then every blank or indented line after it —
// parseNextList's own rule — less the trailing blanks, which belong to the gap
// before the next item. [s, e).
func findNextBlock(lines []string, id string) (int, int, bool) {
	section := ""
	for i, ln := range lines {
		if strings.HasPrefix(ln, "## ") {
			section = strings.TrimSpace(strings.TrimPrefix(ln, "## "))
			continue
		}
		if !nextListed(section) {
			continue
		}
		mt := nextHeaderRe.FindStringSubmatch(ln)
		if mt == nil || mt[1] != id {
			continue
		}
		e := i + 1
		for e < len(lines) && (isBlank(lines[e]) || lines[e][0] == ' ' || lines[e][0] == '\t') {
			e++
		}
		for e > i+1 && isBlank(lines[e-1]) {
			e--
		}
		return i, e, true
	}
	return 0, 0, false
}

// nextRecordBlock is a Closed or Non-goals entry for it: the header with the
// stamp and the item's raised stem (its value is dropped, as the file's own
// records drop it — a rating is about work still to do), then the body.
//
// The body is the user's words when there are some, wrapped to the file's
// width; otherwise the item's own text, kept with its shape, so the record
// never loses what the item was.
func nextRecordBlock(it nextItem, stamp, note string) []string {
	head := "- **" + it.ID + "** · " + stamp
	if it.Raised != "" {
		head += " · raised `" + it.Raised + "`"
	}
	out := []string{head}
	note = strings.Join(strings.Fields(note), " ")
	var body []string
	if note != "" {
		body = strings.Split(ansi.Wrap("— "+note, nextRecordWrap-2, ""), "\n")
	} else {
		body = strings.Split(it.Text, "\n")
		body[0] = "— " + body[0]
	}
	for _, ln := range body {
		ln = strings.TrimRight(ln, " ")
		if ln == "" {
			out = append(out, "")
			continue
		}
		out = append(out, "  "+ln)
	}
	return out
}

// insertNextBlock puts block into dest's section, creating the section when the
// file has none.
//
// Where in the section:
//
//	Closed            above its first entry — newest first
//	everything else   above the first entry with a higher ID — ID order
//	no such entry     after the section's last line of content
//
// The blank-line rule follows the section: Open, Validate and Roadmap keep one
// blank between items, the record sections none — but the first entry of any
// section is set off from its heading or opening prose by a blank, and the
// last from the next heading.
func insertNextBlock(lines []string, dest nextDest, num int, block []string) []string {
	head, end, ok := nextSectionSpan(lines, dest.section())
	if !ok {
		return insertNextSection(lines, dest, block)
	}

	pos, hasItems := -1, false
	for i := head + 1; i < end; i++ {
		mt := nextIDNumRe.FindStringSubmatch(lines[i])
		if mt == nil {
			continue
		}
		hasItems = true
		n, _ := strconv.Atoi(mt[1])
		if dest == nextToClosed || n > num {
			pos = i
			break
		}
	}

	var ins []string
	if pos >= 0 {
		// Above an existing entry, which already has its own spacing above it.
		ins = block
		if dest.spaced() {
			ins = append(append([]string(nil), block...), "")
		}
	} else {
		// After the last line of content (the heading itself, in an empty
		// section).
		pos = head
		for i := end - 1; i > head; i-- {
			if !isBlank(lines[i]) {
				pos = i
				break
			}
		}
		pos++
		ins = block
		if dest.spaced() || !hasItems {
			ins = append([]string{""}, block...)
		}
	}
	lines = spliceLines(lines, pos, ins)
	// Keep the next heading set off from what was just written.
	if after := pos + len(ins); after < len(lines) && strings.HasPrefix(lines[after], "## ") {
		lines = spliceLines(lines, after, []string{""})
	}
	return lines
}

// insertNextSection adds dest's missing section, holding block, before the
// first section that should follow it — or at the end of the file.
func insertNextSection(lines []string, dest nextDest, block []string) []string {
	sec := append(append([]string{"## " + dest.section(), ""}, block...), "")
	for _, later := range nextSectionOrder[dest+1:] {
		if head, _, ok := nextSectionSpan(lines, later); ok {
			return spliceLines(lines, head, sec)
		}
	}
	// None follows: after the file's last line of content, set off by a blank.
	pos := 0
	for i := len(lines) - 1; i >= 0; i-- {
		if !isBlank(lines[i]) {
			pos = i + 1
			break
		}
	}
	return spliceLines(lines, pos, append([]string{""}, sec[:len(sec)-1]...))
}

// nextSectionSpan is the heading line of the named section and the index of
// the next heading (or the end of the file). Case-insensitive, as the parser is.
func nextSectionSpan(lines []string, name string) (int, int, bool) {
	head := -1
	for i, ln := range lines {
		if !strings.HasPrefix(ln, "## ") {
			continue
		}
		if head >= 0 {
			return head, i, true
		}
		if strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(ln, "## ")), name) {
			head = i
		}
	}
	if head >= 0 {
		return head, len(lines), true
	}
	return 0, 0, false
}

// spliceLines inserts ins at pos.
func spliceLines(lines []string, pos int, ins []string) []string {
	out := make([]string, 0, len(lines)+len(ins))
	out = append(out, lines[:pos]...)
	out = append(out, ins...)
	return append(out, lines[pos:]...)
}

// nextIDNum is the number in an item ID ("N-014" → 14); 0 for anything else.
func nextIDNum(id string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(id, "N-"))
	return n
}

func isBlank(s string) bool { return strings.TrimSpace(s) == "" }

// moveNextItemFile is moveNextItemText on the file at path: read it fresh,
// splice, and write the result back through a temp file and a rename, so a
// reader in another pane sees the old file or the new one, never half of one.
// The file's permissions are carried over to the replacement.
func moveNextItemFile(path string, req nextMoveReq) (nextItem, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nextItem{}, fmt.Errorf("cannot read %s: %w", filepath.ToSlash(nextListRel), err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nextItem{}, fmt.Errorf("cannot read %s: %w", filepath.ToSlash(nextListRel), err)
	}
	out, it, err := moveNextItemText(string(data), req)
	if err != nil {
		return nextItem{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".next-list-*.md")
	if err != nil {
		return nextItem{}, fmt.Errorf("cannot write %s: %w", filepath.ToSlash(nextListRel), err)
	}
	// Whatever happens below, the temp file does not outlive this call.
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(out); err != nil {
		tmp.Close()
		return nextItem{}, fmt.Errorf("cannot write %s: %w", filepath.ToSlash(nextListRel), err)
	}
	if err := tmp.Close(); err != nil {
		return nextItem{}, fmt.Errorf("cannot write %s: %w", filepath.ToSlash(nextListRel), err)
	}
	if err := os.Chmod(tmp.Name(), info.Mode().Perm()); err != nil {
		return nextItem{}, fmt.Errorf("cannot write %s: %w", filepath.ToSlash(nextListRel), err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return nextItem{}, fmt.Errorf("cannot write %s: %w", filepath.ToSlash(nextListRel), err)
	}
	return it, nil
}
