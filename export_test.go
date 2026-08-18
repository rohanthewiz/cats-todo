package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// projectAt makes dir a project — a .git marker and a loaded (possibly empty)
// backlog store — and returns the store. The marker is what findProjectRoot
// keys on for a directory that has no backlog yet.
func projectAt(t *testing.T, dir string) *store {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &store{scope: scopeProject, path: projectTodosPath(dir)}
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	return s
}

// writePNG drops a tiny fake image at path so attachImages accepts it.
func writePNG(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// exportFixture is two sibling projects under one temp root, with one todo —
// carrying an attachment, session options and a schedule — in the first.
func exportFixture(t *testing.T) (src, dst *store, td Todo) {
	t.Helper()
	root := t.TempDir()
	src = projectAt(t, filepath.Join(root, "alpha"))
	dst = projectAt(t, filepath.Join(root, "beta"))

	shot := filepath.Join(root, "shot.png")
	writePNG(t, shot)
	td = Todo{
		ID: "t1", Title: "move me", Prompt: "do it\nproperly", Created: time.Now(),
		Session:  &SessionOpts{Model: "sonnet", Files: []string{"a.go"}},
		Schedule: &Schedule{At: time.Now().Add(time.Hour), Kind: scheduleKindNew, Command: "claude"},
	}
	rels, err := src.attachImages(td.ID, []string{shot})
	if err != nil {
		t.Fatal(err)
	}
	td.Images = rels
	if err := src.add(td); err != nil {
		t.Fatal(err)
	}
	td, _ = src.find(td.ID)
	return src, dst, td
}

// TestExportTodoCopies pins what a copy carries and what it leaves: the text,
// the attachment (as dst's own file), the session options and the state travel
// under a fresh id; the schedule does not, and says so; the source is
// untouched.
func TestExportTodoCopies(t *testing.T) {
	src, dst, td := exportFixture(t)

	out, note, err := exportTodo(src, dst, td, false)
	if err != nil {
		t.Fatal(err)
	}
	if out.ID == td.ID || out.ID == "" {
		t.Errorf("copy id = %q, want a fresh id (source is %q)", out.ID, td.ID)
	}
	if out.Title != td.Title || out.Prompt != td.Prompt {
		t.Errorf("copy text = %q/%q, want the source's", out.Title, out.Prompt)
	}
	if out.Session == nil || out.Session.Model != "sonnet" || len(out.Session.Files) != 1 {
		t.Errorf("copy session = %+v, want the source's options", out.Session)
	}
	if out.Session == td.Session {
		t.Error("copy shares the source's session record — must be a clone")
	}
	if out.Schedule != nil {
		t.Errorf("copy schedule = %+v, want none carried", out.Schedule)
	}
	if !strings.Contains(note, "schedule") {
		t.Errorf("note = %q, want it to say the schedule stayed behind", note)
	}
	if len(out.Images) != 1 || !strings.HasPrefix(out.Images[0], "images/"+out.ID+"/") {
		t.Fatalf("copy images = %v, want one under dst's images/<newid>/", out.Images)
	}
	if paths := dst.imagePaths(out); len(paths) != 1 || !strings.HasPrefix(paths[0], filepath.Dir(dst.path)) {
		t.Errorf("dst image paths = %v, want a file inside dst's backlog directory", paths)
	}

	// dst has it on disk; src still has the original, image and all.
	reloaded := &store{scope: scopeProject, path: dst.path}
	if err := reloaded.load(); err != nil || len(reloaded.todos) != 1 || reloaded.todos[0].ID != out.ID {
		t.Fatalf("dst on disk = %+v (%v), want the copy", reloaded.todos, err)
	}
	if got, ok := src.find(td.ID); !ok || len(src.imagePaths(got)) != 1 {
		t.Errorf("source lost its todo or attachment after a copy: %+v %v", got, ok)
	}
}

// TestExportTodoMoves pins the move: same id, gone from the source with its
// files, present in the destination.
func TestExportTodoMoves(t *testing.T) {
	src, dst, td := exportFixture(t)
	srcImages := src.imagePaths(td)

	out, _, err := exportTodo(src, dst, td, true)
	if err != nil {
		t.Fatal(err)
	}
	if out.ID != td.ID {
		t.Errorf("moved id = %q, want the source's %q kept", out.ID, td.ID)
	}
	if _, ok := src.find(td.ID); ok {
		t.Error("source still lists the todo after a move")
	}
	if _, err := os.Stat(srcImages[0]); !os.IsNotExist(err) {
		t.Errorf("source attachment still on disk after a move (err=%v)", err)
	}
	if got, ok := dst.find(td.ID); !ok || len(dst.imagePaths(got)) != 1 {
		t.Errorf("destination is missing the todo or its attachment: %+v %v", got, ok)
	}
}

// TestExportTodoRefusesItsOwnBacklog: the browser can point at the directory
// the prompt already lives in, and the answer is a refusal, not a duplicate.
func TestExportTodoRefusesItsOwnBacklog(t *testing.T) {
	src, _, td := exportFixture(t)
	same := &store{scope: scopeProject, path: src.path}
	if _, _, err := exportTodo(src, same, td, false); err == nil {
		t.Fatal("exporting into the source backlog succeeded, want a refusal")
	}
	if len(src.todos) != 1 {
		t.Errorf("source has %d todos after the refusal, want 1", len(src.todos))
	}
	if _, _, err := exportTodo(src, &store{scope: scopeProject}, td, false); err == nil {
		t.Fatal("exporting into an unavailable store succeeded")
	}
}

// TestDestinationStore pins the directory → backlog resolution: a subdirectory
// of a project reaches the project's backlog, a directory that isn't there is
// refused, and the filesystem root is refused.
func TestDestinationStore(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	projectAt(t, proj)
	if err := os.MkdirAll(filepath.Join(proj, "internal", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}

	s, err := destinationStore(filepath.Join(proj, "internal", "deep"))
	if err != nil {
		t.Fatal(err)
	}
	if want := projectTodosPath(proj); s.path != want {
		t.Errorf("path = %q, want the project root's %q", s.path, want)
	}
	if _, err := destinationStore(filepath.Join(root, "nope")); err == nil {
		t.Error("a missing directory was accepted")
	}
	if _, err := destinationStore("/"); err == nil {
		t.Error("the filesystem root was accepted")
	}
	// A directory that is neither a repo nor a backlog is its own project.
	plain := filepath.Join(root, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if s, err := destinationStore(plain); err != nil || s.path != projectTodosPath(plain) {
		t.Errorf("plain dir → %q (%v), want its own backlog", s.path, err)
	}
}

// TestBuildExportTargets pins the picker's rows: one per project root among the
// workspaces (deduped, the source project left off), the other backlog, the
// recent projects that keep a backlog, and the browse row last.
func TestBuildExportTargets(t *testing.T) {
	root := t.TempDir()
	own := projectAt(t, filepath.Join(root, "own"))
	sib := projectAt(t, filepath.Join(root, "sib"))
	if err := sib.add(Todo{ID: "s1", Prompt: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sib", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	projectAt(t, filepath.Join(root, "recent-with"))
	if err := (&store{scope: scopeProject, path: projectTodosPath(filepath.Join(root, "recent-with"))}).save(); err != nil {
		t.Fatal(err)
	}
	projectAt(t, filepath.Join(root, "recent-without"))
	global := &store{scope: scopeGlobal, path: filepath.Join(root, "global", "todos.json")}

	src := exportSources{
		workspaces: []exportWorkspace{
			{name: "own", dir: filepath.Join(root, "own")},            // the source project: left off
			{name: "sib", dir: filepath.Join(root, "sib")},            // a sibling
			{name: "sib pkg", dir: filepath.Join(root, "sib", "pkg")}, // same project: collapsed
			{name: "nowhere", dir: ""},                                // no directory: skipped
			{name: "fresh", dir: filepath.Join(root, "fresh")},        // no backlog yet
		},
		recents: []string{
			filepath.Join(root, "sib"),            // already a workspace row
			filepath.Join(root, "recent-with"),    // keeps a backlog: listed
			filepath.Join(root, "recent-without"), // no backlog: not listed
		},
	}
	if err := os.MkdirAll(filepath.Join(root, "fresh"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := buildExportTargets(scopeProject, src, own, global)
	var labels []string
	for _, tg := range got {
		labels = append(labels, tg.label)
	}
	want := []string{"sib", "fresh", "Global backlog", "recent-with", "Browse for a folder…"}
	if strings.Join(labels, "|") != strings.Join(want, "|") {
		t.Fatalf("labels = %v, want %v", labels, want)
	}
	if !strings.Contains(got[0].desc, "1 open") {
		t.Errorf("sib desc = %q, want its open count", got[0].desc)
	}
	if !strings.Contains(got[1].desc, "no backlog yet") {
		t.Errorf("fresh desc = %q, want the no-backlog note", got[1].desc)
	}
	if got[2].kind != exportToStore || got[2].scope != scopeGlobal {
		t.Errorf("third row = %+v, want the global store", got[2])
	}
	if got[3].tag != "recent" {
		t.Errorf("recent row tag = %q, want \"recent\"", got[3].tag)
	}
	if got[len(got)-1].kind != exportBrowse {
		t.Errorf("last row = %+v, want browse", got[len(got)-1])
	}

	// A global todo: the project is the other-backlog row, and its workspace
	// row is folded into it rather than listed twice.
	got = buildExportTargets(scopeGlobal, src, own, global)
	labels = labels[:0]
	for _, tg := range got {
		labels = append(labels, tg.label)
	}
	want = []string{"sib", "fresh", "This project — own", "recent-with", "Browse for a folder…"}
	if strings.Join(labels, "|") != strings.Join(want, "|") {
		t.Fatalf("global labels = %v, want %v", labels, want)
	}

	// No project store at all (a global-only launch): the manager's own
	// directory is then just a workspace like any other.
	got = buildExportTargets(scopeGlobal, src, &store{scope: scopeProject}, global)
	if got[0].label != "own" {
		t.Errorf("without a project store the first row is %q, want the own workspace", got[0].label)
	}
}

// TestCdxRecentsFrom pins the state-file reader: frecency order, stale entries
// skipped, the cap honoured, and garbage answered with nothing.
func TestCdxRecentsFrom(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"hot", "old", "gone"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(filepath.Join(root, "gone")); err != nil {
		t.Fatal(err)
	}
	now := int64(1_000_000)
	st := map[string]any{"entries": []map[string]any{
		{"path": filepath.Join(root, "old"), "count": 100, "last": now - 30*86400}, // 100 × 0.25 = 25
		{"path": filepath.Join(root, "gone"), "count": 999, "last": now},           // deleted since
		{"path": filepath.Join(root, "hot"), "count": 10, "last": now - 60},        // 10 × 4 = 40
		{"path": "relative/path", "count": 50, "last": now},
	}}
	data, _ := json.Marshal(st)
	statePath := filepath.Join(root, "state.json")
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	got := cdxRecentsFrom(statePath, now, 10)
	want := []string{filepath.Join(root, "hot"), filepath.Join(root, "old")}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("recents = %v, want %v", got, want)
	}
	if got := cdxRecentsFrom(statePath, now, 1); len(got) != 1 || got[0] != want[0] {
		t.Errorf("capped recents = %v, want just %q", got, want[0])
	}
	if got := cdxRecentsFrom(filepath.Join(root, "missing.json"), now, 5); got != nil {
		t.Errorf("missing state file → %v, want nil", got)
	}
	if err := os.WriteFile(statePath, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := cdxRecentsFrom(statePath, now, 5); got != nil {
		t.Errorf("corrupt state file → %v, want nil", got)
	}
}

// exportModel is a manager over two temp backlogs with one project todo
// highlighted, no socket. The temp root is returned for planting siblings.
//
// Unlike newModelInTemp it lays the project out the way a real launch finds it
// — <root>/project/.cats-todo/todos.json with the context rooted at
// <root>/project — because the export paths resolve directories back to
// backlogs, and a fixture with the file somewhere else would let a test pass
// on a layout no launch produces.
func exportModel(t *testing.T) (model, string) {
	t.Helper()
	root := t.TempDir()
	// No cdx habits from the machine running the tests: the picker's rows must
	// be the fixture's alone.
	prev := cdxStateFile
	cdxStateFile = func() (string, error) { return filepath.Join(root, "no-cdx-state.json"), nil }
	t.Cleanup(func() { cdxStateFile = prev })
	projDir := filepath.Join(root, "project")
	project := projectAt(t, projDir)
	global := &store{scope: scopeGlobal, path: filepath.Join(root, "global", "todos.json")}
	if err := project.add(Todo{ID: "t1", Title: "stray prompt", Prompt: "belongs elsewhere"}); err != nil {
		t.Fatal(err)
	}
	m := newModel(RunContext{WorkDir: projDir, ProjectRoot: projDir}, project, global, nil)
	m.width, m.height = 100, 30
	return m, root
}

// TestExportStageFlow walks the keys: ctrl+o opens the picker on the highlighted
// prompt, enter on the global row copies it (both backlogs hold it), and the
// modifier chord moves it (only the destination does). Both land back on the
// list with a status.
func TestExportStageFlow(t *testing.T) {
	m, _ := exportModel(t)

	next, _ := m.Update(pressKey("ctrl+o"))
	m = next.(model)
	if m.stage != stageExport {
		t.Fatalf("ctrl+o → stage %v, want stageExport", m.stage)
	}
	if m.exportRef != (todoRef{scope: scopeProject, id: "t1"}) {
		t.Fatalf("exportRef = %+v, want the highlighted todo", m.exportRef)
	}
	// No socket, no cdx state: the rows are the global backlog and the browse
	// row, in that order.
	if len(m.exportTargets) != 2 || m.exportTargets[0].kind != exportToStore || m.exportTargets[1].kind != exportBrowse {
		t.Fatalf("rows = %+v, want [global, browse]", m.exportTargets)
	}
	globalRow := -1
	for i, tg := range m.exportTargets {
		if tg.kind == exportToStore && tg.scope == scopeGlobal {
			globalRow = i
		}
	}
	if globalRow < 0 {
		t.Fatalf("no global row among %+v", m.exportTargets)
	}
	if !strings.Contains(m.viewExport(), "Export to…") || !strings.Contains(m.viewExport(), "Global backlog") {
		t.Errorf("view:\n%s", m.viewExport())
	}

	m.exportList.selectRef(globalRow)
	next, _ = m.Update(enterKey(0))
	m = next.(model)
	if m.stage != stageList {
		t.Fatalf("after enter stage = %v, want the list", m.stage)
	}
	if !strings.HasPrefix(m.status, "copied → the global backlog") || m.statusErr {
		t.Errorf("status = %q (err=%v), want a copied note", m.status, m.statusErr)
	}
	if len(m.project.todos) != 1 || len(m.global.todos) != 1 {
		t.Fatalf("after copy: project %d, global %d todos, want 1 and 1", len(m.project.todos), len(m.global.todos))
	}
	if m.global.todos[0].ID == "t1" {
		t.Error("the copy kept the source id")
	}
	// The list now shows both.
	if len(m.rows) != 2 {
		t.Errorf("list has %d rows after the copy, want 2", len(m.rows))
	}

	// Move the original: highlight it, ctrl+o, modifier+enter on the global row.
	m.selectRow(todoRef{scope: scopeProject, id: "t1"})
	next, _ = m.Update(pressKey("ctrl+o"))
	m = next.(model)
	for i, tg := range m.exportTargets {
		if tg.kind == exportToStore {
			m.exportList.selectRef(i)
		}
	}
	next, _ = m.Update(enterKey(tea.ModShift))
	m = next.(model)
	if !strings.HasPrefix(m.status, "moved → the global backlog") || m.statusErr {
		t.Errorf("status = %q (err=%v), want a moved note", m.status, m.statusErr)
	}
	if len(m.project.todos) != 0 || len(m.global.todos) != 2 {
		t.Errorf("after move: project %d, global %d todos, want 0 and 2", len(m.project.todos), len(m.global.todos))
	}
	if _, ok := m.global.find("t1"); !ok {
		t.Error("the moved todo did not keep its id")
	}
}

// TestExportBrowseFlow: the browse row opens the folder browser in its
// folders-only shape, leading with "./"; enter on that row exports to the
// listed folder — whose project's backlog is created — and the manager is
// back on the list.
func TestExportBrowseFlow(t *testing.T) {
	m, root := exportModel(t)
	sibling := filepath.Join(root, "sibling")
	if err := os.MkdirAll(filepath.Join(sibling, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "main.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	next, _ := m.Update(pressKey("ctrl+o"))
	m = next.(model)
	browse := len(m.exportTargets) - 1
	m.exportList.selectRef(browse)
	next, _ = m.Update(enterKey(0))
	m = next.(model)
	if m.stage != stageFiles || m.files.purpose != filesForExport || !m.files.dirsOnly {
		t.Fatalf("browse row → stage %v purpose %v dirsOnly %v, want the export browser", m.stage, m.files.purpose, m.files.dirsOnly)
	}
	// It starts among the project's siblings — the temp root — where "sibling"
	// and "project" are listed and "./" leads.
	if m.files.dir != root {
		t.Errorf("browser starts in %q, want the project's parent %q", m.files.dir, root)
	}
	view := m.viewFiles()
	if !strings.Contains(view, "Export to folder") || !strings.Contains(view, "./") || !strings.Contains(view, "sibling/") {
		t.Errorf("browser view:\n%s", view)
	}
	if m.files.list.selectedIndex() != thisFolderRef {
		t.Errorf("first highlight ref = %d, want the ./ row", m.files.list.selectedIndex())
	}

	// Drill into sibling: files are not listed, "./" leads again.
	m = typeAll(t, m, "sib", "tab")
	if m.files.dir != sibling {
		t.Fatalf("after tab dir = %q, want %q", m.files.dir, sibling)
	}
	if v := m.viewFiles(); strings.Contains(v, "main.go") || !strings.Contains(v, "src/") {
		t.Errorf("folders-only view lists files or lost src/:\n%s", v)
	}
	// esc goes back to the export picker, not the list.
	m = typeAll(t, m, "esc")
	if m.stage != stageExport {
		t.Fatalf("esc from the browser → stage %v, want the export picker", m.stage)
	}

	// Back in, straight to sibling, and enter on "./" copies there.
	next, _ = m.Update(enterKey(0))
	m = next.(model)
	m = typeAll(t, m, "sib", "tab")
	next, _ = m.Update(enterKey(0))
	m = next.(model)
	if m.stage != stageList {
		t.Fatalf("after choosing the folder stage = %v, want the list", m.stage)
	}
	if !strings.HasPrefix(m.status, "copied → sibling") || m.statusErr {
		t.Errorf("status = %q (err=%v)", m.status, m.statusErr)
	}
	dst := &store{scope: scopeProject, path: projectTodosPath(sibling)}
	if err := dst.load(); err != nil || len(dst.todos) != 1 || dst.todos[0].Prompt != "belongs elsewhere" {
		t.Fatalf("sibling backlog = %+v (%v), want the copied prompt", dst.todos, err)
	}
	if len(m.project.todos) != 1 {
		t.Errorf("a copy removed the source todo")
	}
}

// TestExportBrowseRefusesOwnProject: browsing back into the prompt's own
// project is refused rather than duplicating it.
func TestExportBrowseRefusesOwnProject(t *testing.T) {
	m, _ := exportModel(t)
	next, _ := m.Update(pressKey("ctrl+o"))
	m = next.(model)
	m.exportList.selectRef(len(m.exportTargets) - 1)
	next, _ = m.Update(enterKey(0))
	m = next.(model)
	m = typeAll(t, m, "project", "tab")
	next, _ = m.Update(enterKey(0))
	m = next.(model)
	if !m.statusErr || !strings.Contains(m.status, "already in") {
		t.Errorf("status = %q (err=%v), want the own-backlog refusal", m.status, m.statusErr)
	}
	if len(m.project.todos) != 1 {
		t.Errorf("project has %d todos, want the original 1", len(m.project.todos))
	}
}

// TestExportRowsMatchWhatIsDrawn pins exportRowsRow and the click hit test to
// a rendered frame, the way the drop picker's test does.
func TestExportRowsMatchWhatIsDrawn(t *testing.T) {
	m, _ := exportModel(t)
	next, _ := m.Update(pressKey("ctrl+o"))
	m = next.(model)
	lines := strings.Split(m.viewExport(), "\n")
	for i, tg := range m.exportTargets {
		y := exportRowsRow + i
		if y >= len(lines) {
			t.Fatalf("row %d falls outside the %d-line frame:\n%s", i, len(lines), m.viewExport())
		}
		if !strings.Contains(lines[y], tg.label) {
			t.Fatalf("line %d is %q, want the row %q:\n%s", y, lines[y], tg.label, m.viewExport())
		}
		got, ok := m.exportList.rowAtLine(y - exportRowsRow)
		if !ok || got != i {
			t.Fatalf("rowAtLine(%d) = %d,%v, want row %d", y-exportRowsRow, got, ok, i)
		}
	}
	// A click on the global row copies.
	globalRow := 0
	for i, tg := range m.exportTargets {
		if tg.kind == exportToStore {
			globalRow = i
		}
	}
	next, _ = m.Update(tea.MouseClickMsg{X: 3, Y: exportRowsRow + globalRow, Button: tea.MouseLeft})
	m = next.(model)
	if m.stage != stageList || len(m.global.todos) != 1 {
		t.Errorf("click → stage %v, global %d todos; want the list and one copy", m.stage, len(m.global.todos))
	}
	if got := m.View().MouseMode; m.stage == stageExport && got != tea.MouseModeCellMotion {
		t.Errorf("export stage MouseMode = %v", got)
	}
}

// TestExportFromView: ctrl+o on the prompt view opens the picker on that
// prompt, and the action bar carries an Export chip that does the same.
func TestExportFromViewAndBar(t *testing.T) {
	m, _ := exportModel(t)
	next, _ := m.beginView()
	m = next.(model)
	next, _ = m.Update(pressKey("ctrl+o"))
	m = next.(model)
	if m.stage != stageExport || m.exportRef.id != "t1" {
		t.Errorf("view ctrl+o → stage %v ref %+v", m.stage, m.exportRef)
	}

	m, _ = exportModel(t)
	next, _ = m.runAction(actionExport)
	m = next.(model)
	if m.stage != stageExport {
		t.Errorf("Export chip → stage %v, want stageExport", m.stage)
	}
	if !strings.Contains(m.listActions()[actionExport].label, "Export") {
		t.Errorf("actionExport is %q", m.listActions()[actionExport].label)
	}
}
