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

// pickerTree lays out the fixture every picker test browses:
//
//	root/
//	  .env
//	  .git/
//	  README.md
//	  docs/
//	  main.go
//	  src/
//	    sub/
//	      deep.txt
//	    ui.go
//
// Small enough to reason about by hand, shaped enough to have hidden entries,
// nested folders, and files at more than one depth.
func pickerTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{".git", "docs", "src/sub"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{".env", "README.md", "main.go", "src/ui.go", "src/sub/deep.txt"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// t.TempDir on macOS lives under /var, which is a symlink to /private/var;
	// the picker joins and cleans paths but never resolves links, so the tests
	// compare against whatever the OS handed out, unresolved.
	return root
}

// withPicker opens the form over a fixture tree with prompt already typed, then
// types '@' — the gesture under test — and returns the model, which should now
// be on the picker, plus the tree's root.
func withPicker(t *testing.T, prompt string) (model, string) {
	t.Helper()
	root := pickerTree(t)
	m := withForm(t, "", prompt, 100, 30)
	m.ctx.WorkDir = root
	m = typeInForm(t, m, pressKey("@"))
	return m, root
}

// typeAll feeds keys one after another: a name from pressKey's table ("enter",
// "backspace", "ctrl+n"…) is one press, anything else is typed a character at
// a time, so "READ" is four keystrokes and "/" is one.
func typeAll(t *testing.T, m model, keys ...string) model {
	t.Helper()
	named := map[string]bool{
		"enter": true, "esc": true, "up": true, "down": true, "left": true, "right": true,
		"space": true, "tab": true, "shift+tab": true, "backspace": true, "pgup": true, "pgdown": true,
	}
	for _, k := range keys {
		if len(k) == 1 || named[k] || strings.HasPrefix(k, "ctrl+") {
			m = typeInForm(t, m, pressKey(k))
			continue
		}
		for _, r := range k {
			m = typeInForm(t, m, pressKey(string(r)))
		}
	}
	return m
}

// visibleNames are the names on the picker's list, in row order.
func visibleNames(m model) []string {
	var names []string
	for _, s := range m.files.list.filtered {
		names = append(names, s.item.name)
	}
	return names
}

func TestAtOpensThePickerOnlyAtAWordStart(t *testing.T) {
	for _, tc := range []struct {
		prompt string
		opens  bool
	}{
		{"", true},
		{"look at ", true},
		{"first line\n", true},
		{"mail me at foo", false},
		{"x", false},
	} {
		m, _ := withPicker(t, tc.prompt)
		got := m.stage == stageFiles
		if got != tc.opens {
			t.Errorf("prompt %q then '@': picker open = %v, want %v", tc.prompt, got, tc.opens)
		}
		// Whether or not the picker opened, the '@' is in the text: it is a
		// character the editor takes, before it is anything else.
		if v := m.promptArea.Value(); !strings.HasSuffix(v, "@") {
			t.Errorf("prompt %q then '@': editor holds %q, want it to end in '@'", tc.prompt, v)
		}
		if tc.opens && (m.promptArea.Focused() || m.titleInput.Focused()) {
			t.Errorf("prompt %q: form fields still focused under the picker", tc.prompt)
		}
	}
}

func TestAtInTheTitleDoesNotOpenThePicker(t *testing.T) {
	m := withForm(t, "", "", 100, 30)
	m.ctx.WorkDir = pickerTree(t)
	next, _ := m.toggleFormFocus() // to the title
	m = next.(model)
	if m.formFocus != formFieldTitle {
		t.Fatalf("formFocus = %v, want the title", m.formFocus)
	}
	m = typeInForm(t, m, pressKey("@"))
	if m.stage != stageForm {
		t.Fatalf("stage = %v after '@' in the title, want the form", m.stage)
	}
	if got := m.titleInput.Value(); got != "@" {
		t.Errorf("title = %q, want %q", got, "@")
	}
}

func TestPastedAtDoesNotOpenThePicker(t *testing.T) {
	m := withForm(t, "", "", 100, 30)
	m.ctx.WorkDir = pickerTree(t)
	next, _ := m.Update(tea.PasteMsg{Content: "@"})
	m = next.(model)
	if m.stage != stageForm {
		t.Fatalf("stage = %v after a pasted '@', want the form", m.stage)
	}
}

func TestReadDirEntriesListsFoldersThenFiles(t *testing.T) {
	root := pickerTree(t)
	got, err := readDirEntries(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []fileEntry{
		{".git", true}, {"docs", true}, {"src", true},
		{".env", false}, {"README.md", false}, {"main.go", false},
	}
	if len(got) != len(want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %v, want %v", i, got[i], want[i])
		}
	}
	if _, err := readDirEntries(filepath.Join(root, "nope")); err == nil {
		t.Error("reading a missing directory did not error")
	}
}

func TestReadDirEntriesFollowsSymlinksToFolders(t *testing.T) {
	root := pickerTree(t)
	if err := os.Symlink(filepath.Join(root, "docs"), filepath.Join(root, "docs-link")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if err := os.Symlink(filepath.Join(root, "gone"), filepath.Join(root, "broken")); err != nil {
		t.Fatal(err)
	}
	got, err := readDirEntries(root)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, e := range got {
		kinds[e.name] = e.dir
	}
	if !kinds["docs-link"] {
		t.Error("a symlink to a folder is listed as a file")
	}
	if isDirLink, ok := kinds["broken"]; !ok || isDirLink {
		t.Errorf("a broken symlink should list as a file; listed=%v dir=%v", ok, isDirLink)
	}
}

func TestPickerOpensAtTheProjectRootWithHiddenEntriesOut(t *testing.T) {
	m, root := withPicker(t, "")
	if m.files.dir != root {
		t.Fatalf("picker dir = %q, want the project root %q", m.files.dir, root)
	}
	want := []string{"docs/", "src/", "README.md", "main.go"}
	if got := visibleNames(m); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func TestHiddenEntriesFollowTheDot(t *testing.T) {
	m, _ := withPicker(t, "")
	m = typeAll(t, m, ".")
	got := strings.Join(visibleNames(m), ",")
	for _, want := range []string{".git/", ".env"} {
		if !strings.Contains(got, want) {
			t.Errorf("after typing '.', rows %q lack %q", got, want)
		}
	}
	m = typeAll(t, m, "backspace")
	if got := strings.Join(visibleNames(m), ","); strings.Contains(got, ".env") {
		t.Errorf("after deleting the dot, rows %q still show hidden entries", got)
	}
}

func TestFileInsertText(t *testing.T) {
	home := homeDir()
	for _, tc := range []struct {
		abs, project string
		dir          bool
		want         string
	}{
		{"/p/src/ui.go", "/p", false, "src/ui.go"},
		{"/p/src", "/p", true, "src/"},
		{"/p", "/p", true, "./"},
		{"/elsewhere/x.go", "/p", false, "/elsewhere/x.go"},
		{home + "/notes/a.md", "/p", false, "~/notes/a.md"},
		{"/p/src/ui.go", "", false, "/p/src/ui.go"},
		{"/pq/x.go", "/p", false, "/pq/x.go"}, // a sibling that merely shares the prefix
	} {
		if got := fileInsertText(tc.abs, tc.dir, tc.project); got != tc.want {
			t.Errorf("fileInsertText(%q, dir=%v, project=%q) = %q, want %q", tc.abs, tc.dir, tc.project, got, tc.want)
		}
	}
}

func TestEnterInsertsTheMentionAndReturnsToTheForm(t *testing.T) {
	// The first row is docs/ — enter on a folder inserts it with its slash.
	m, _ := withPicker(t, "see ")
	m = typeAll(t, m, "enter")
	if m.stage != stageForm {
		t.Fatalf("stage = %v after enter, want the form", m.stage)
	}
	if got, want := m.promptArea.Value(), "see @docs/ "; got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
	if !m.promptArea.Focused() {
		t.Error("the prompt did not get its focus back")
	}
	if !strings.Contains(m.formNote, "@docs/") {
		t.Errorf("form note = %q, want it to name the mention", m.formNote)
	}

	// A file: filter down to it and choose.
	m, _ = withPicker(t, "")
	m = typeAll(t, m, "READ", "enter")
	if got, want := m.promptArea.Value(), "@README.md "; got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
}

func TestMentionsAreRelativeToTheProjectDir(t *testing.T) {
	m, root := withPicker(t, "")
	m.ctx.ProjectRoot = root // the resolved root wins over WorkDir, as in projectDir
	m = typeAll(t, m, "main", "enter")
	if got, want := m.promptArea.Value(), "@main.go "; got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
}

func TestSlashOpensTheHighlightedFolder(t *testing.T) {
	m, root := withPicker(t, "")
	m = typeAll(t, m, "s", "/") // "s" narrows to src/ (docs has no s… it does: "docs" — fuzzy) — check the highlight is src
	if m.files.dir != filepath.Join(root, "src") {
		t.Fatalf("after 's' '/': dir = %q, want src (rows were %v)", m.files.dir, visibleNames(m))
	}
	if q := m.files.query(); q != "" {
		t.Errorf("query after opening a folder = %q, want empty", q)
	}
	m = typeAll(t, m, "ui", "enter")
	if got, want := m.promptArea.Value(), "@src/ui.go "; got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
}

func TestSlashOnAFileIsSwallowed(t *testing.T) {
	m, root := withPicker(t, "")
	m = typeAll(t, m, "READ", "/")
	if m.files.dir != root {
		t.Errorf("dir moved to %q on '/' over a file", m.files.dir)
	}
	if q := m.files.query(); q != "READ" {
		t.Errorf("query = %q, want the '/' swallowed", q)
	}
}

func TestATypedPathWalksIn(t *testing.T) {
	m, root := withPicker(t, "")
	// The whole folder name then a slash: the query itself is the path, and
	// normalization — not the highlight — is what walks in.
	m = typeAll(t, m, "d", "o", "c", "s", "/")
	if m.files.dir != filepath.Join(root, "docs") {
		t.Fatalf("after typing docs/: dir = %q", m.files.dir)
	}
	// ../ from inside goes back up; a further segment filters there.
	m = typeAll(t, m, ".", ".", "/", "m", "a")
	if m.files.dir != root {
		t.Fatalf("after ../: dir = %q, want root", m.files.dir)
	}
	if q := m.files.query(); q != "ma" {
		t.Errorf("query = %q, want %q", q, "ma")
	}
	if got := visibleNames(m); len(got) != 1 || got[0] != "main.go" {
		t.Errorf("rows = %v, want just main.go", got)
	}
}

func TestAPastedPathLandsInItsFolder(t *testing.T) {
	m, root := withPicker(t, "")
	next, _ := m.Update(tea.PasteMsg{Content: filepath.Join(root, "src", "sub", "deep")})
	m = next.(model)
	if m.files.dir != filepath.Join(root, "src", "sub") {
		t.Fatalf("dir = %q, want src/sub", m.files.dir)
	}
	if q := m.files.query(); q != "deep" {
		t.Errorf("query = %q, want the file name kept as the filter", q)
	}
	m = typeAll(t, m, "enter")
	if got, want := m.promptArea.Value(), "@src/sub/deep.txt "; got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
}

func TestTildeGoesHome(t *testing.T) {
	m, _ := withPicker(t, "")
	m = typeAll(t, m, "~", "/")
	if m.files.dir != homeDir() {
		t.Errorf("dir = %q, want home %q", m.files.dir, homeDir())
	}
}

func TestTabOpensAFolderAndCompletesAFile(t *testing.T) {
	m, root := withPicker(t, "")
	m = typeAll(t, m, "tab") // docs/ is highlighted first
	if m.files.dir != filepath.Join(root, "docs") {
		t.Fatalf("tab on docs/: dir = %q", m.files.dir)
	}
	m = typeAll(t, m, "backspace") // back up, highlight on docs/
	m = typeAll(t, m, "READ", "tab")
	if q := m.files.query(); q != "README.md" {
		t.Errorf("tab on a file: query = %q, want it completed to README.md", q)
	}
	if m.files.dir != root {
		t.Errorf("tab on a file moved dir to %q", m.files.dir)
	}
	// → is tab's other spelling.
	m = typeAll(t, m, "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace")
	m = typeAll(t, m, "s", "right")
	if m.files.dir != filepath.Join(root, "src") {
		t.Errorf("→ on src/: dir = %q", m.files.dir)
	}
}

func TestBackspaceOnAnEmptyQueryGoesUp(t *testing.T) {
	m, root := withPicker(t, "")
	m = typeAll(t, m, "s", "/", "s", "/") // into src/sub
	if m.files.dir != filepath.Join(root, "src", "sub") {
		t.Fatalf("dir = %q, want src/sub", m.files.dir)
	}
	m = typeAll(t, m, "backspace")
	if m.files.dir != filepath.Join(root, "src") {
		t.Fatalf("after backspace: dir = %q, want src", m.files.dir)
	}
	// The way back down is under the cursor.
	if e, _, ok := m.files.highlighted(); !ok || e.name != "sub" {
		t.Errorf("highlight after going up = %v (%v), want sub/", e, ok)
	}
	// Backspace with a filter in the box is just backspace.
	m = typeAll(t, m, "u", "i", "backspace")
	if m.files.dir != filepath.Join(root, "src") {
		t.Errorf("backspace with a query moved dir to %q", m.files.dir)
	}
	if q := m.files.query(); q != "u" {
		t.Errorf("query = %q, want %q", q, "u")
	}
}

func TestUpStopsAtTheRoot(t *testing.T) {
	p := newFilePicker("/")
	p.up()
	if p.dir != "/" {
		t.Errorf("up from / went to %q", p.dir)
	}
}

func TestEscLeavesTheAtSignAndTheFocus(t *testing.T) {
	m, _ := withPicker(t, "hello ")
	m = typeAll(t, m, "s", "esc")
	if m.stage != stageForm {
		t.Fatalf("stage = %v after esc, want the form", m.stage)
	}
	if got, want := m.promptArea.Value(), "hello @"; got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
	if !m.promptArea.Focused() {
		t.Error("the prompt did not get its focus back")
	}
}

func TestEnterOnNothingKeepsThePickerOpen(t *testing.T) {
	m, _ := withPicker(t, "")
	m = typeAll(t, m, "zzzz", "enter")
	if m.stage != stageFiles {
		t.Errorf("stage = %v, want the picker to stay while nothing matches", m.stage)
	}
	if got := m.promptArea.Value(); got != "@" {
		t.Errorf("prompt = %q, want the bare '@' untouched", got)
	}
}

func TestAnUnreadableFolderIsSaidNotPanicked(t *testing.T) {
	m := withForm(t, "", "", 100, 30)
	m.ctx.WorkDir = filepath.Join(t.TempDir(), "gone")
	m = typeInForm(t, m, pressKey("@"))
	if m.stage != stageFiles {
		t.Fatalf("stage = %v", m.stage)
	}
	if m.files.err == "" {
		t.Fatal("no error recorded for a missing start directory")
	}
	if !strings.Contains(m.viewFiles(), "cannot read") {
		t.Errorf("view does not show the read error:\n%s", m.viewFiles())
	}
	// And the way out still works.
	m = typeAll(t, m, "backspace")
	if m.files.err != "" {
		t.Errorf("going up from the missing folder still errors: %q", m.files.err)
	}
}

// TestFilesRowsMatchWhatIsDrawn pins filesRowsRow to a rendered frame, the way
// the form and the drop picker pin theirs: clickFiles subtracts it from the
// pointer's row, so a line added above the rows would aim clicks one row off.
func TestFilesRowsMatchWhatIsDrawn(t *testing.T) {
	m, _ := withPicker(t, "")
	lines := strings.Split(m.viewFiles(), "\n")
	if len(lines) <= filesRowsRow+1 {
		t.Fatalf("view has only %d lines:\n%s", len(lines), m.viewFiles())
	}
	if !strings.Contains(lines[filesRowsRow], "docs/") {
		t.Errorf("row %d is %q, want the first entry docs/", filesRowsRow, lines[filesRowsRow])
	}
	if !strings.Contains(lines[filesRowsRow+1], "src/") {
		t.Errorf("row %d is %q, want the second entry src/", filesRowsRow+1, lines[filesRowsRow+1])
	}
	if !strings.Contains(lines[0], "Insert a path") {
		t.Errorf("heading is %q", lines[0])
	}
}

func TestClickOnARowChoosesIt(t *testing.T) {
	m, _ := withPicker(t, "")
	m = clickForm(m, 3, filesRowsRow+1) // the second row: src/
	if m.stage != stageForm {
		t.Fatalf("stage = %v after a row click, want the form", m.stage)
	}
	if got, want := m.promptArea.Value(), "@src/ "; got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
	// A click off the rows does nothing.
	m, _ = withPicker(t, "")
	m = clickForm(m, 3, 0)
	if m.stage != stageFiles {
		t.Errorf("a click on the heading changed the stage to %v", m.stage)
	}
}

func TestFilesStageAsksForTheMouse(t *testing.T) {
	m, _ := withPicker(t, "")
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Errorf("files stage MouseMode = %v, want cell motion so rows are clickable", got)
	}
}

func TestPickerWindowFitsThePane(t *testing.T) {
	root := t.TempDir()
	for i := range 40 {
		if err := os.WriteFile(filepath.Join(root, "f"+string(rune('a'+i%26))+strings.Repeat("x", i/26)+".txt"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := withForm(t, "", "", 100, 15)
	m.ctx.WorkDir = root
	m = typeInForm(t, m, pressKey("@"))
	if m.stage != stageFiles {
		t.Fatalf("stage = %v", m.stage)
	}
	if m.files.list.maxRows <= 0 || m.files.list.maxRows >= 40 {
		t.Fatalf("maxRows = %d, want a window smaller than the 40-entry list", m.files.list.maxRows)
	}
	view := m.viewFiles()
	if n := strings.Count(view, "\n") + 1; n > 15 {
		t.Errorf("view is %d lines in a 15-line pane:\n%s", n, view)
	}
	if !strings.Contains(view, "more") {
		t.Errorf("a clipped list should say how many rows are below the fold:\n%s", view)
	}
	// Walking past the window scrolls it, and the highlight stays drawn.
	for range m.files.list.maxRows + 3 {
		m = typeAll(t, m, "down")
	}
	if m.files.list.top == 0 {
		t.Error("the window did not scroll when the cursor walked past it")
	}
	lo, hi := m.files.list.window()
	if c := m.files.list.cursor; c < lo || c >= hi {
		t.Errorf("cursor %d is outside the drawn window [%d,%d)", c, lo, hi)
	}
	// A page down/up moves a window's worth.
	before := m.files.list.cursor
	m = typeAll(t, m, "pgup")
	if got := before - m.files.list.cursor; got != m.files.list.maxRows {
		t.Errorf("pgup moved %d rows, want %d", got, m.files.list.maxRows)
	}
	// A resize while open re-fits the window and does not panic.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(model)
	if m.files.list.maxRows != 40-filesRowsRow-3 {
		t.Errorf("maxRows after resize = %d, want %d", m.files.list.maxRows, 40-filesRowsRow-3)
	}
}

func TestPickerHeadingStaysOnOneLine(t *testing.T) {
	root := pickerTree(t)
	deep := filepath.Join(root, strings.Repeat("averyveryverylongfoldername/", 6))
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	m := withForm(t, "", "", 60, 30)
	m.ctx.WorkDir = deep
	m = typeInForm(t, m, pressKey("@"))
	first, _, _ := strings.Cut(m.viewFiles(), "\n")
	if w := lipgloss.Width(first); w > 60 {
		t.Errorf("heading is %d cells in a 60-cell pane: %q", w, first)
	}
	if !strings.Contains(first, "…") {
		t.Errorf("a heading that had to be cut should lead with an ellipsis: %q", first)
	}
	if !strings.HasSuffix(strings.TrimSpace(ansi.Strip(first)), "averyveryverylongfoldername/") {
		t.Errorf("the cut heading should keep the path's end: %q", first)
	}
}

func TestFormFooterTeachesTheAtKey(t *testing.T) {
	m := withForm(t, "", "body", 120, 40)
	if !strings.Contains(m.formFooter(), "@ file") {
		t.Errorf("form footer does not mention '@ file': %q", m.formFooter())
	}
}

func TestPathHelpers(t *testing.T) {
	home := homeDir()
	for _, tc := range []struct{ q, cwd, base, partial string }{
		{"src/", "/p", "/p/src", ""},
		{"src/ui", "/p", "/p/src", "ui"},
		{"../", "/p/src", "/p", ""},
		{"..", "/p/src", "/p", ""},
		{"../ma", "/p/src", "/p", "ma"},
		{"~", "/p", home, ""},
		{"~/", "/p", home, ""},
		{"~/x/y", "/p", filepath.Join(home, "x"), "y"},
		{"/abs/dir/", "/p", "/abs/dir", ""},
		{"/abs/dir/f", "/p", "/abs/dir", "f"},
	} {
		base, partial := splitPathQuery(tc.q, tc.cwd)
		if base != tc.base || partial != tc.partial {
			t.Errorf("splitPathQuery(%q, %q) = (%q, %q), want (%q, %q)", tc.q, tc.cwd, base, partial, tc.base, tc.partial)
		}
	}
	for q, want := range map[string]bool{"": false, "ui": false, ".env": false, "src/": true, "~": false, "~/": true, "..": false, "../x": true, "a/b": true, "...": false} {
		if got := isPathQuery(q); got != want {
			t.Errorf("isPathQuery(%q) = %v, want %v", q, got, want)
		}
	}
	if got := truncateLeft("abcdef", 4); got != "…def" {
		t.Errorf("truncateLeft = %q", got)
	}
	if got := truncateLeft("abc", 4); got != "abc" {
		t.Errorf("truncateLeft on a short string = %q", got)
	}
}
