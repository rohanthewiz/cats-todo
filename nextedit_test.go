package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// editNextList is the file the splice tests work on: the real file's shape,
// with each section holding enough entries to test the placement rules — an
// Open run to cut from, a Roadmap with opening prose, packed Non-goals in ID
// order, and a Closed with prose above its newest-first entries.
const editNextList = "# Next list\n" +
	"\n" +
	"**Next ID:** N-010\n" +
	"\n" +
	"## Open\n" +
	"\n" +
	"- **N-002** · raised `s-two` · value high\n" +
	"  Two, over\n" +
	"  two lines.\n" +
	"\n" +
	"- **N-005** · raised `s-five` · value low\n" +
	"  Five.\n" +
	"\n" +
	"- **N-008** · raised `s-eight` · value medium\n" +
	"  Eight.\n" +
	"\n" +
	"## Roadmap\n" +
	"\n" +
	"Wanted, but later.\n" +
	"\n" +
	"- **N-003** · raised `s-three` · value low\n" +
	"  Three.\n" +
	"\n" +
	"## Non-goals\n" +
	"\n" +
	"- **N-001** · declined `s-one` — One.\n" +
	"- **N-006** · declined `s-six` — Six.\n" +
	"\n" +
	"## Closed\n" +
	"\n" +
	"Older closures live in the session docs.\n" +
	"\n" +
	"- **N-007** · closed 2026-09-27 · raised `s-seven`\n" +
	"  — Seven.\n" +
	"- **N-004** · closed 2026-09-20 · raised `s-four`\n" +
	"  — Four.\n"

func mustMove(t *testing.T, src string, req nextMoveReq) string {
	t.Helper()
	if req.Date == "" {
		req.Date = "2026-10-03"
	}
	out, _, err := moveNextItemText(src, req)
	if err != nil {
		t.Fatalf("move %s: %v", req.ID, err)
	}
	return out
}

// section is the named section's body as text, for comparing one section at a
// time without restating the whole file.
func sectionText(t *testing.T, src, name string) string {
	t.Helper()
	lines := strings.Split(src, "\n")
	head, end, ok := nextSectionSpan(lines, name)
	if !ok {
		t.Fatalf("no ## %s in:\n%s", name, src)
	}
	return strings.Join(lines[head+1:end], "\n")
}

// TestNextMoveToClosed: the item leaves Open without a doubled blank, and its
// record goes on top of Closed — under the opening prose, above the newest
// entry, packed — with the date, its raised stem, and the comment wrapped as a
// "— " body. Its value is dropped.
func TestNextMoveToClosed(t *testing.T) {
	out := mustMove(t, editNextList, nextMoveReq{ID: "N-005", To: nextToClosed, Note: "Shipped in v0.44.0; TestFive pins it."})

	wantOpen := "\n" +
		"- **N-002** · raised `s-two` · value high\n" +
		"  Two, over\n" +
		"  two lines.\n" +
		"\n" +
		"- **N-008** · raised `s-eight` · value medium\n" +
		"  Eight.\n"
	if got := sectionText(t, out, "Open"); got != wantOpen {
		t.Errorf("Open after the move:\n%q\nwant\n%q", got, wantOpen)
	}
	wantClosed := "\n" +
		"Older closures live in the session docs.\n" +
		"\n" +
		"- **N-005** · closed 2026-10-03 · raised `s-five`\n" +
		"  — Shipped in v0.44.0; TestFive pins it.\n" +
		"- **N-007** · closed 2026-09-27 · raised `s-seven`\n" +
		"  — Seven.\n" +
		"- **N-004** · closed 2026-09-20 · raised `s-four`\n" +
		"  — Four.\n"
	if got := sectionText(t, out, "Closed"); got != wantClosed {
		t.Errorf("Closed after the move:\n%q\nwant\n%q", got, wantClosed)
	}
	// Everything else is byte-for-byte what it was.
	for _, s := range []string{"Roadmap", "Non-goals"} {
		if sectionText(t, out, s) != sectionText(t, editNextList, s) {
			t.Errorf("## %s changed, want it untouched:\n%s", s, sectionText(t, out, s))
		}
	}
	if !strings.HasPrefix(out, "# Next list\n\n**Next ID:** N-010\n") {
		t.Errorf("the preamble changed:\n%s", out)
	}
}

// TestNextMoveRecordKeepsTheTextWithoutAComment: no words from the user, so
// the item's own text is the record's body, shape and all — a closed item
// never loses what it was.
func TestNextMoveRecordKeepsTheTextWithoutAComment(t *testing.T) {
	out := mustMove(t, editNextList, nextMoveReq{ID: "N-002", To: nextToClosed})
	want := "- **N-002** · closed 2026-10-03 · raised `s-two`\n  — Two, over\n  two lines.\n- **N-007**"
	if !strings.Contains(out, want) {
		t.Errorf("record without a comment, want %q in:\n%s", want, out)
	}
}

// TestNextMoveLongCommentWraps: a comment is one line in the pad but a
// paragraph in the file, wrapped to the file's width under the bullet.
func TestNextMoveLongCommentWraps(t *testing.T) {
	note := strings.Repeat("word ", 40)
	out := mustMove(t, editNextList, nextMoveReq{ID: "N-008", To: nextToClosed, Note: note})
	for _, ln := range strings.Split(sectionText(t, out, "Closed"), "\n") {
		if len(ln) > nextRecordWrap {
			t.Errorf("record line is %d wide, want ≤ %d: %q", len(ln), nextRecordWrap, ln)
		}
	}
	// Still one item to the parser of the skills: every body line indented.
	if strings.Contains(sectionText(t, out, "Closed"), "\nword") {
		t.Errorf("a wrapped line fell to column 0:\n%s", out)
	}
}

// TestNextMoveToNonGoals: packed, in ID order — N-005 lands between N-001 and
// N-006 — stamped "declined" with the reason as its body.
func TestNextMoveToNonGoals(t *testing.T) {
	out := mustMove(t, editNextList, nextMoveReq{ID: "N-005", To: nextToNonGoals, Note: "Too costly for what it buys."})
	want := "\n" +
		"- **N-001** · declined `s-one` — One.\n" +
		"- **N-005** · declined 2026-10-03 · raised `s-five`\n" +
		"  — Too costly for what it buys.\n" +
		"- **N-006** · declined `s-six` — Six.\n"
	if got := sectionText(t, out, "Non-goals"); got != want {
		t.Errorf("Non-goals:\n%q\nwant\n%q", got, want)
	}
	// After the last one too: N-008 goes below N-006, still packed, and the
	// blank before ## Closed stays.
	out = mustMove(t, out, nextMoveReq{ID: "N-008", To: nextToNonGoals, Note: "No."})
	if !strings.Contains(out, "- **N-006** · declined `s-six` — Six.\n- **N-008** · declined 2026-10-03 · raised `s-eight`\n  — No.\n\n## Closed") {
		t.Errorf("N-008 not packed at the end of Non-goals:\n%s", sectionText(t, out, "Non-goals"))
	}
}

// TestNextMoveBetweenOpenAndRoadmap: the item's lines travel verbatim (value
// and all), in ID order, a blank between items — both ways.
func TestNextMoveBetweenOpenAndRoadmap(t *testing.T) {
	out := mustMove(t, editNextList, nextMoveReq{ID: "N-002", To: nextToRoadmap})
	wantRoad := "\n" +
		"Wanted, but later.\n" +
		"\n" +
		"- **N-002** · raised `s-two` · value high\n" +
		"  Two, over\n" +
		"  two lines.\n" +
		"\n" +
		"- **N-003** · raised `s-three` · value low\n" +
		"  Three.\n"
	if got := sectionText(t, out, "Roadmap"); got != wantRoad {
		t.Errorf("Roadmap:\n%q\nwant\n%q", got, wantRoad)
	}
	if strings.Contains(sectionText(t, out, "Open"), "N-002") {
		t.Errorf("N-002 still in Open")
	}

	// And N-003 back to Open, between N-002's old place and N-005 — and the
	// Roadmap left with its prose alone.
	out = mustMove(t, out, nextMoveReq{ID: "N-003", To: nextToOpen})
	wantOpen := "\n" +
		"- **N-003** · raised `s-three` · value low\n" +
		"  Three.\n" +
		"\n" +
		"- **N-005** · raised `s-five` · value low\n" +
		"  Five.\n" +
		"\n" +
		"- **N-008** · raised `s-eight` · value medium\n" +
		"  Eight.\n"
	if got := sectionText(t, out, "Open"); got != wantOpen {
		t.Errorf("Open:\n%q\nwant\n%q", got, wantOpen)
	}
	// Moving N-002 back to Open restores the original file exactly.
	out = mustMove(t, out, nextMoveReq{ID: "N-002", To: nextToOpen})
	out = mustMove(t, out, nextMoveReq{ID: "N-003", To: nextToRoadmap})
	if out != editNextList {
		t.Errorf("a round trip changed the file:\n%s", out)
	}
}

// TestNextMoveIntoAnEmptySection: the first entry of an empty section is set
// off from the heading by a blank, and from the next heading by another.
func TestNextMoveIntoAnEmptySection(t *testing.T) {
	src := "## Open\n\n- **N-001** · raised `a` · value low\n  One.\n\n## Roadmap\n\n## Closed\n"
	out := mustMove(t, src, nextMoveReq{ID: "N-001", To: nextToRoadmap})
	want := "## Open\n\n## Roadmap\n\n- **N-001** · raised `a` · value low\n  One.\n\n## Closed\n"
	if out != want {
		t.Errorf("got\n%q\nwant\n%q", out, want)
	}
}

// TestNextMoveCreatesAMissingSection: an older file has no Roadmap, a young one
// no Non-goals. The section is made in its place in the file's order — before
// the first section that follows it, or at the end.
func TestNextMoveCreatesAMissingSection(t *testing.T) {
	src := "## Open\n\n- **N-001** · raised `a` · value low\n  One.\n\n- **N-002** · raised `b` · value low\n  Two.\n\n## Closed\n\n- **N-000** · closed 2026-01-01\n  — Zero.\n"
	out := mustMove(t, src, nextMoveReq{ID: "N-001", To: nextToRoadmap})
	if !strings.Contains(out, "  Two.\n\n## Roadmap\n\n- **N-001** · raised `a` · value low\n  One.\n\n## Closed\n") {
		t.Errorf("Roadmap not made before Closed:\n%s", out)
	}
	out = mustMove(t, out, nextMoveReq{ID: "N-002", To: nextToNonGoals, Note: "No."})
	if !strings.Contains(out, "  One.\n\n## Non-goals\n\n- **N-002** · declined 2026-10-03 · raised `b`\n  — No.\n\n## Closed\n") {
		t.Errorf("Non-goals not made between Roadmap and Closed:\n%s", out)
	}

	// No Closed at all: it goes at the end, after a blank.
	src = "## Open\n\n- **N-001** · raised `a` · value low\n  One.\n"
	out = mustMove(t, src, nextMoveReq{ID: "N-001", To: nextToClosed, Note: "Done."})
	if want := "## Open\n\n## Closed\n\n- **N-001** · closed 2026-10-03 · raised `a`\n  — Done.\n"; out != want {
		t.Errorf("got\n%q\nwant\n%q", out, want)
	}
}

// TestNextMoveRefusals: an item not in Open/Roadmap (moved elsewhere since the
// page loaded), and a move to where it already is, are refused in words.
func TestNextMoveRefusals(t *testing.T) {
	for _, tc := range []struct {
		req  nextMoveReq
		want string
	}{
		{nextMoveReq{ID: "N-007", To: nextToClosed}, "no longer Open"},
		{nextMoveReq{ID: "N-099", To: nextToClosed}, "no longer Open"},
		{nextMoveReq{ID: "N-003", To: nextToRoadmap}, "already in Roadmap"},
	} {
		_, _, err := moveNextItemText(editNextList, tc.req)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%+v: err = %v, want one saying %q", tc.req, err, tc.want)
		}
	}
}

// TestNextMoveKeepsCRLF: a CRLF file stays CRLF throughout.
func TestNextMoveKeepsCRLF(t *testing.T) {
	src := strings.ReplaceAll(editNextList, "\n", "\r\n")
	out := mustMove(t, src, nextMoveReq{ID: "N-005", To: nextToClosed, Note: "x"})
	if strings.Count(out, "\n") != strings.Count(out, "\r\n") {
		t.Errorf("a bare LF crept into a CRLF file")
	}
}

// TestNextMoveFileWritesInPlace: the file route re-reads, writes the move, and
// keeps the file's permissions; no temp file is left beside it.
func TestNextMoveFileWritesInPlace(t *testing.T) {
	root := t.TempDir()
	writeNextList(t, root, editNextList)
	path := filepath.Join(root, nextListRel)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	it, err := moveNextItemFile(path, nextMoveReq{ID: "N-008", To: nextToRoadmap, Date: "2026-10-03"})
	if err != nil {
		t.Fatal(err)
	}
	if it.ID != "N-008" || it.Section != "Open" {
		t.Errorf("returned item %+v, want N-008 as it was (Open)", it)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(sectionText(t, string(data), "Roadmap"), "N-008") {
		t.Errorf("file not written:\n%s", data)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600 kept", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("dir holds %d entries, want only next-list.md", len(entries))
	}
}
