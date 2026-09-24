package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// sampleNextList is the file's real shape (the next-list skill's), with the
// cases the parser has to get right: a conventions preamble with bullets of
// its own, a multi-line item with nested bullets, an item whose text starts on
// its header line, prose opening a section, and the two sections that must not
// be listed.
const sampleNextList = "# Next list\n" +
	"\n" +
	"## Conventions\n" +
	"\n" +
	"- **IDs are permanent** (`N-001`, …) and never reused.\n" +
	"\n" +
	"**Next ID:** N-006\n" +
	"\n" +
	"## Open\n" +
	"\n" +
	"- **N-001** · raised `2026-0904-1753-a-dwell` · value medium\n" +
	"  Hands-on pass in a rebuilt Cats.app. Merged from checks:\n" +
	"  - hover cards: the 400ms dwell;\n" +
	"  - DEC 1004: blur a window.\n" +
	"\n" +
	"- **N-002** · raised `2026-0910-1855-boot` · value high\n" +
	"  Seeds ship stale prompts.\n" +
	"\n" +
	"## Roadmap\n" +
	"\n" +
	"Wanted, but deliberately not next.\n" +
	"\n" +
	"- **N-003** · raised `2026-0911-0000-x` · value low · Sync the peers\n" +
	"  over the LAN.\n" +
	"\n" +
	"## Non-goals\n" +
	"\n" +
	"- **N-004** · raised `2026-0911-0000-x` · value low\n" +
	"  Never this.\n" +
	"\n" +
	"## Closed\n" +
	"\n" +
	"- **N-005** · raised `2026-0911-0000-x` · closed 2026-09-22 — done.\n"

func TestParseNextList(t *testing.T) {
	items := parseNextList(sampleNextList)
	if len(items) != 3 {
		t.Fatalf("got %d items %+v, want N-001..N-003 — Non-goals and Closed are not listed", len(items), items)
	}
	one := items[0]
	if one.ID != "N-001" || one.Value != "medium" || one.Raised != "2026-0904-1753-a-dwell" || one.Section != "Open" {
		t.Errorf("N-001 header parsed as %+v", one)
	}
	// Dedented, nested bullets still nested, and the blank line before the next
	// bullet not carried in.
	want := "Hands-on pass in a rebuilt Cats.app. Merged from checks:\n- hover cards: the 400ms dwell;\n- DEC 1004: blur a window."
	if one.Text != want {
		t.Errorf("N-001 text = %q, want %q", one.Text, want)
	}
	if items[1].Value != "high" || items[1].Text != "Seeds ship stale prompts." {
		t.Errorf("N-002 = %+v", items[1])
	}
	// Words on the header line are text, not dropped as an unknown field; the
	// Roadmap's opening prose is not part of any item.
	three := items[2]
	if three.Section != "Roadmap" || three.Text != "Sync the peers\nover the LAN." {
		t.Errorf("N-003 = %+v", three)
	}
}

// nextModel is a model whose project backlog sits in a real project layout
// (<root>/.cats-todo/todos.json), so nextListRoot resolves to root, with the
// sample list written where the skill keeps it.
func nextModel(t *testing.T, list string) (model, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv(configDirEnvVar, filepath.Join(root, "config"))
	if list != "" {
		writeNextList(t, root, list)
	}
	project := &store{scope: scopeProject, path: projectTodosPath(root)}
	global := &store{scope: scopeGlobal, path: filepath.Join(root, "global", "todos.json")}
	m := newModel(RunContext{WorkDir: root, ProjectRoot: root}, project, global, nil)
	m.width, m.height = 100, 30
	m.applySizes()
	return m, root
}

func writeNextList(t *testing.T, root, body string) {
	t.Helper()
	p := filepath.Join(root, nextListRel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func openNext(t *testing.T, m model) model {
	t.Helper()
	m = pressList(t, m, "ctrl+g")
	if m.stage != stageNextList {
		t.Fatalf("ctrl+g: stage = %v, want the Next List page", m.stage)
	}
	return m
}

func pressNext(t *testing.T, m model, key string) model {
	t.Helper()
	next, _ := m.Update(pressKey(key))
	return next.(model)
}

// TestNextListRowsAreTheWholeItem pins the one way these rows differ from the
// backlog's: no title/description split — the row is the ID and the item's own
// text, flattened, cut only where the pane ends, and never wider than it (a
// wrapped row would move every row below it off its click line).
func TestNextListRowsAreTheWholeItem(t *testing.T) {
	m, _ := nextModel(t, sampleNextList)
	m = openNext(t, m)

	for _, s := range m.next.list.filtered {
		if s.item.selectable && s.item.desc != "" {
			t.Errorf("row %q carries a description %q; a next-list row is one run of text", s.item.name, s.item.desc)
		}
	}
	first := m.next.list.filtered[1].item // [0] is the Open heading
	if first.badge != "N-001" || !strings.HasPrefix(first.name, "Hands-on pass in a rebuilt Cats.app. Merged from checks: - hover cards") {
		t.Errorf("first row = %q %q", first.badge, first.name)
	}

	for _, w := range []int{100, 50} {
		m.width = w
		m.applySizes()
		for i, ln := range strings.Split(m.viewNextList(), "\n") {
			if lipgloss.Width(ln) > w {
				t.Errorf("width %d: line %d is %d cells wide: %q", w, i, lipgloss.Width(ln), ansi.Strip(ln))
			}
		}
	}
	// Narrow enough to cut, the row ends in the ellipsis that says so.
	if name := m.next.list.filtered[1].item.name; !strings.HasSuffix(name, "…") {
		t.Errorf("a 50-column row was not cut: %q", name)
	}
}

// TestNextListEnterDraftsAPrompt checks enter opens the ADD form holding the
// item — cited by ID and file, so the agent that gets it can close it — and
// that nothing is written until the form saves.
func TestNextListEnterDraftsAPrompt(t *testing.T) {
	m, _ := nextModel(t, sampleNextList)
	m = openNext(t, m)
	m = pressNext(t, m, "down") // N-002

	m = pressNext(t, m, "enter")
	if m.stage != stageForm || m.formMode != formAdd {
		t.Fatalf("stage=%v mode=%v, want the add form", m.stage, m.formMode)
	}
	if got := m.titleInput.Value(); got != "N-002 Seeds ship stale prompts." {
		t.Errorf("title = %q", got)
	}
	want := "Next list item N-002 (ai_docs/todo/next-list.md):\n\nSeeds ship stale prompts."
	if got := m.promptArea.Value(); got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
	if len(m.project.todos) != 0 {
		t.Errorf("opening the draft wrote %d todos; only a save should", len(m.project.todos))
	}
}

// TestNextListRefresh is the page's ↻ button: an edit made to the file in
// another pane shows up on ctrl+r (and on a click of the chip), and the
// highlight stays on the item it was on even when rows above it moved.
func TestNextListRefresh(t *testing.T) {
	m, root := nextModel(t, sampleNextList)
	m = openNext(t, m)
	m = pressNext(t, m, "down") // N-002

	edited := strings.Replace(sampleNextList, "## Open\n\n", "## Open\n\n- **N-006** · raised `x` · value high\n  A fresh one.\n\n", 1)
	writeNextList(t, root, edited)
	m = pressNext(t, m, "ctrl+r")
	if len(m.next.items) != 4 {
		t.Fatalf("after ctrl+r: %d items, want the new one read in", len(m.next.items))
	}
	if it, _ := m.next.highlighted(); it.ID != "N-002" {
		t.Errorf("highlight moved to %s, want it kept on N-002", it.ID)
	}
	if !strings.Contains(m.next.note, "refreshed") {
		t.Errorf("note = %q, want the refresh reported", m.next.note)
	}

	// The chip does the same thing.
	writeNextList(t, root, sampleNextList)
	chip := m.nextChips()[nextActionRefresh]
	next, _ := m.Update(tea.MouseClickMsg{X: chip.start + 1, Y: nextBarRow, Button: tea.MouseLeft})
	m = next.(model)
	if len(m.next.items) != 3 {
		t.Errorf("after clicking ↻ Refresh: %d items, want 3", len(m.next.items))
	}
}

// TestNextListGeometry pins nextBarRow and nextRowsRow against a rendered
// frame: the chips on the bar row, and a click on the line that shows N-003
// highlighting N-003.
func TestNextListGeometry(t *testing.T) {
	m, _ := nextModel(t, sampleNextList)
	m = openNext(t, m)
	lines := strings.Split(m.viewNextList(), "\n")
	if !strings.Contains(ansi.Strip(lines[nextBarRow]), "↻ Refresh") {
		t.Fatalf("line %d is %q, want the bar", nextBarRow, ansi.Strip(lines[nextBarRow]))
	}
	y := -1
	for i, ln := range lines {
		if strings.Contains(ansi.Strip(ln), "N-003") {
			y = i
		}
	}
	next, _ := m.Update(tea.MouseClickMsg{X: 8, Y: y, Button: tea.MouseLeft})
	m = next.(model)
	if it, _ := m.next.highlighted(); it.ID != "N-003" {
		t.Errorf("click on line %d highlighted %q, want N-003", y, it.ID)
	}
}

// TestNextListMissingFileSaysSo: a project with no list still opens the page,
// and the page says which file is missing and what creates it.
func TestNextListMissingFileSaysSo(t *testing.T) {
	m, _ := nextModel(t, "")
	m = openNext(t, m)
	if !strings.Contains(m.viewNextList(), "/next-list seed") {
		t.Errorf("empty page does not say how to create the list:\n%s", ansi.Strip(m.viewNextList()))
	}
	m = pressNext(t, m, "enter")
	if m.stage != stageNextList || m.next.note == "" {
		t.Errorf("enter with nothing highlighted: stage=%v note=%q, want a refusal in words", m.stage, m.next.note)
	}
	m = pressNext(t, m, "esc")
	if m.stage != stageList {
		t.Errorf("esc: stage = %v, want back on the list", m.stage)
	}
}

// TestNextListChipOnTheBar: the list's » Next List chip opens the page.
func TestNextListChipOnTheBar(t *testing.T) {
	m, _ := nextModel(t, sampleNextList)
	m.rebuildList()
	chip := m.actionChips()[actionNext]
	next, _ := m.Update(tea.MouseClickMsg{X: chip.start + 1, Y: actionBarRow, Button: tea.MouseLeft})
	if m = next.(model); m.stage != stageNextList {
		t.Fatalf("clicking » Next List: stage = %v, want the Next List page", m.stage)
	}
}

// TestNextListValueMarks pins the value glyph each row wears after its ID. The
// value used to ride on the ID's hue alone, and three warm hues side by side
// were too close to read it from, so it gets a mark of its own. Every mark is
// the same width, so the text starts in the same column on every row.
func TestNextListValueMarks(t *testing.T) {
	for v, want := range map[string]string{
		"high":   valueGlyph,
		"medium": nextMediumGlyph + " ",
		"low":    nextLowGlyph + " ",
		"":       "  ",
	} {
		mk := nextValueMark(v)
		if mk.text != want {
			t.Errorf("value %q: mark %q, want %q", v, mk.text, want)
		}
		if w := lipgloss.Width(mk.text); w != nextValueMarkWidth {
			t.Errorf("value %q: mark is %d cells, want %d", v, w, nextValueMarkWidth)
		}
	}

	m, _ := nextModel(t, sampleNextList)
	m = openNext(t, m)
	textCol := -1
	for _, ln := range strings.Split(ansi.Strip(m.viewNextList()), "\n") {
		i := strings.Index(ln, "N-00")
		if i < 0 {
			continue
		}
		// ID (5 cells) + space + mark (2) + space.
		col := lipgloss.Width(ln[:i]) + 5 + 1 + nextValueMarkWidth + 1
		if textCol < 0 {
			textCol = col
		}
		body := ansi.Cut(ln, col, col+1)
		if col != textCol || body == " " || body == "" {
			t.Errorf("row %q: text does not start at column %d", ln, textCol)
		}
	}
	if textCol < 0 {
		t.Fatal("no rows rendered")
	}
}
