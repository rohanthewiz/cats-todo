package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bundleOnDisk writes a bundle of the given prompts into a fresh directory and
// returns the file's path.
func bundleOnDisk(t *testing.T, todos ...Todo) string {
	t.Helper()
	dir := t.TempDir()
	b, files, _ := buildBundle(nil, todos, "test", "elsewhere")
	path, err := writeBundle(dir, "", b, files)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// importModel is a manager with one prompt already in its project backlog, so
// the duplicate arithmetic has something to be about.
func importModel(t *testing.T) model {
	t.Helper()
	m, project, _ := newModelInTemp(t)
	if err := project.add(Todo{ID: "here", Title: "already here", Prompt: "mine"}); err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 100, 30
	m.rebuildList()
	return m
}

func TestImportPickerOpensOnCtrlR(t *testing.T) {
	m := importModel(t)

	next, _ := m.Update(pressKey("ctrl+r"))
	m = next.(model)
	if m.stage != stageImport {
		t.Fatalf("ctrl+r → stage %v, want stageImport", m.stage)
	}
	if len(m.importTargets) == 0 || m.importTargets[0].kind != importFromFile {
		t.Fatalf("rows = %+v, want the disk row first", m.importTargets)
	}
	view := m.viewImport()
	if !strings.Contains(view, "Import from…") || !strings.Contains(view, "A bundle file on disk") {
		t.Errorf("view:\n%s", view)
	}
	// The network block is there even with no peer found: its "Enter a host…"
	// row is how a machine the beacon missed is reached.
	if m.importTargets[len(m.importTargets)-1].kind != importFromAddr {
		t.Errorf("last row = %+v, want the type-a-host row", m.importTargets[len(m.importTargets)-1])
	}
}

// The disk row opens the browser in its import flavour, which lists bundles and
// folders and nothing else.
func TestImportBrowseListsOnlyBundles(t *testing.T) {
	m := importModel(t)
	dir := t.TempDir()
	for _, name := range []string{"notes.txt", "photo.png", "a.catstodo.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	next, _ := m.Update(pressKey("ctrl+r"))
	m = next.(model)
	next, _ = m.Update(enterKey(0)) // the disk row is highlighted
	m = next.(model)
	if m.stage != stageFiles || m.files.purpose != filesForImport || !m.files.onlyBundles {
		t.Fatalf("stage %v purpose %v onlyBundles %v", m.stage, m.files.purpose, m.files.onlyBundles)
	}

	m.files = newFilePicker(dir)
	m.files.purpose = filesForImport
	m.files.onlyBundles = true
	m.files.refresh()
	view := m.viewFiles()
	if !strings.Contains(view, "a.catstodo.json") || !strings.Contains(view, "sub/") {
		t.Errorf("the bundle and the folder should be listed:\n%s", view)
	}
	if strings.Contains(view, "notes.txt") || strings.Contains(view, "photo.png") {
		t.Errorf("only bundles and folders belong here:\n%s", view)
	}
	if !strings.Contains(view, "Import a bundle from") {
		t.Errorf("the browser should say what it is for:\n%s", view)
	}
}

// The whole flow: pick a file, read the confirm's arithmetic, answer y, and the
// prompts are in the backlog.
func TestImportConfirmAndWrite(t *testing.T) {
	m := importModel(t)
	file := bundleOnDisk(t,
		Todo{ID: "x", Title: "new one", Prompt: "fresh"},
		Todo{ID: "y", Title: "already here", Prompt: "mine"}, // a duplicate of what is here
	)

	next, _ := m.Update(pressKey("ctrl+r"))
	m = next.(model)
	m.files = newFilePicker(filepath.Dir(file))
	m.files.purpose = filesForImport
	m.files.onlyBundles = true
	m.files.refresh()
	next, _ = m.chooseImportFile()
	m = next.(model)

	if m.stage != stageConfirm || m.confirmKind != confirmImport {
		t.Fatalf("stage %v kind %v, want the import confirm", m.stage, m.confirmKind)
	}
	view := m.viewConfirm()
	for _, want := range []string{"2 prompts", "project backlog", "1 prompt new", "1 already here"} {
		if !strings.Contains(view, want) {
			t.Errorf("confirm missing %q:\n%s", want, view)
		}
	}

	next, _ = m.Update(pressKey("y"))
	m = next.(model)
	if m.stage != stageList {
		t.Fatalf("stage = %v, want the list", m.stage)
	}
	if len(m.project.todos) != 2 {
		t.Fatalf("project holds %+v, want the original plus the new one", m.project.todos)
	}
	if m.project.todos[1].Title != "new one" {
		t.Errorf("imported %+v", m.project.todos[1])
	}
	if m.project.todos[1].ID == "x" {
		t.Error("an imported prompt should take a fresh id")
	}
	if m.statusErr || !strings.Contains(m.status, "1 already here") {
		t.Errorf("status = %q (err=%v)", m.status, m.statusErr)
	}
}

// tab on the confirm sends it to the other backlog, and re-counts what that
// backlog already holds.
func TestImportConfirmTabSwitchesBacklog(t *testing.T) {
	m := importModel(t)
	file := bundleOnDisk(t, Todo{ID: "x", Title: "already here", Prompt: "mine"})

	next, _ := m.Update(pressKey("ctrl+r"))
	m = next.(model)
	m.files = newFilePicker(filepath.Dir(file))
	m.files.purpose = filesForImport
	m.files.onlyBundles = true
	m.files.refresh()
	next, _ = m.chooseImportFile()
	m = next.(model)

	if m.pendingImport.scope != scopeProject || m.pendingImport.duplicates != 1 {
		t.Fatalf("pending = %+v, want the project backlog with its one duplicate", m.pendingImport)
	}
	next, _ = m.Update(pressKey("tab"))
	m = next.(model)
	if m.pendingImport.scope != scopeGlobal {
		t.Fatalf("tab should switch backlogs, scope = %v", m.pendingImport.scope)
	}
	// The global backlog has never seen it, so nothing is a duplicate there.
	if m.pendingImport.duplicates != 0 {
		t.Errorf("duplicates = %d against the global backlog, want 0", m.pendingImport.duplicates)
	}

	next, _ = m.Update(pressKey("y"))
	m = next.(model)
	if len(m.global.todos) != 1 {
		t.Fatalf("global holds %+v, want the imported prompt", m.global.todos)
	}
	if len(m.project.todos) != 1 {
		t.Errorf("the project backlog should be untouched: %+v", m.project.todos)
	}
}

// esc from the confirm writes nothing and drops the bundle it was holding.
func TestImportConfirmCancel(t *testing.T) {
	m := importModel(t)
	file := bundleOnDisk(t, Todo{ID: "x", Title: "new one", Prompt: "fresh"})

	next, _ := m.Update(pressKey("ctrl+r"))
	m = next.(model)
	m.files = newFilePicker(filepath.Dir(file))
	m.files.purpose = filesForImport
	m.files.onlyBundles = true
	m.files.refresh()
	next, _ = m.chooseImportFile()
	m = next.(model)
	next, _ = m.Update(pressKey("esc"))
	m = next.(model)

	if m.stage != stageList {
		t.Errorf("stage = %v, want the list", m.stage)
	}
	if len(m.project.todos) != 1 {
		t.Errorf("a cancelled import must write nothing: %+v", m.project.todos)
	}
	if len(m.pendingImport.bundle.Todos) != 0 {
		t.Error("the cancelled bundle should not be held on the model")
	}
}

// A file that is not a bundle leaves the browser up with the reason on it —
// the answer is almost always the file next to it.
func TestImportRejectsANonBundle(t *testing.T) {
	m := importModel(t)
	dir := t.TempDir()
	bad := filepath.Join(dir, "not-a.catstodo.json")
	if err := os.WriteFile(bad, []byte("{\"hello\":true}"), 0o644); err != nil {
		t.Fatal(err)
	}

	next, _ := m.Update(pressKey("ctrl+r"))
	m = next.(model)
	// Straight into the browser, pointed at the folder holding the bad file:
	// the picker's own enter would have got here through beginImportBrowse,
	// which would open on Downloads instead.
	m.stage = stageFiles
	m.files = newFilePicker(dir)
	m.files.purpose = filesForImport
	m.files.onlyBundles = true
	m.files.refresh()
	next, _ = m.chooseImportFile()
	m = next.(model)

	if m.stage != stageFiles {
		t.Fatalf("stage = %v, want the browser still up", m.stage)
	}
	if m.files.err == "" || !strings.Contains(m.files.err, "bundle") {
		t.Errorf("files.err = %q, want it to say what is wrong", m.files.err)
	}
}

// TestImportRowsMatchWhatIsDrawn pins importRowsRow and the click hit test to a
// rendered frame, the way the export picker's test does — headings draw two
// lines and answer no hit test, which is the part worth pinning.
func TestImportRowsMatchWhatIsDrawn(t *testing.T) {
	m := importModel(t)
	next, _ := m.Update(pressKey("ctrl+r"))
	m = next.(model)
	lines := strings.Split(m.viewImport(), "\n")

	off := 0
	for i, tg := range m.importTargets {
		if tg.kind == importSection {
			y := importRowsRow + off + 1
			if y >= len(lines) || !strings.Contains(lines[y], tg.label) {
				t.Fatalf("heading %q is not on line %d:\n%s", tg.label, y, m.viewImport())
			}
			off += 2
			continue
		}
		y := importRowsRow + off
		if y >= len(lines) || !strings.Contains(lines[y], tg.label) {
			t.Fatalf("line %d is %q, want the row %q:\n%s", y, lines[y], tg.label, m.viewImport())
		}
		got, ok := m.importList.rowAtLine(off)
		if !ok || got != i {
			t.Fatalf("rowAtLine(%d) = %d,%v, want row %d", off, got, ok, i)
		}
		off++
	}
}

func TestBundleInDir(t *testing.T) {
	dir := t.TempDir()
	if _, err := bundleInDir(dir); err == nil {
		t.Error("an empty directory should report that there is no bundle")
	}
	one := filepath.Join(dir, "a.catstodo.json")
	if err := os.WriteFile(one, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := bundleInDir(dir)
	if err != nil || got != one {
		t.Errorf("bundleInDir = %q, %v; want %q", got, err, one)
	}
	// Two is a question, not a guess: picking the newest would be silently
	// wrong the one time it mattered.
	if err := os.WriteFile(filepath.Join(dir, "b.catstodo.zip"), []byte("PK"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := bundleInDir(dir); err == nil || !strings.Contains(err.Error(), "name the one you mean") {
		t.Errorf("two bundles → %v, want a refusal that says to name one", err)
	}
}

func TestIsBundleName(t *testing.T) {
	for _, ok := range []string{"a.catstodo.json", "A.CATSTODO.ZIP", "my-project-2026-09-02.catstodo.zip"} {
		if !isBundleName(ok) {
			t.Errorf("isBundleName(%q) = false", ok)
		}
	}
	for _, no := range []string{"todos.json", "a.zip", "catstodo.json", "", "notes.md"} {
		if isBundleName(no) {
			t.Errorf("isBundleName(%q) = true", no)
		}
	}
}
