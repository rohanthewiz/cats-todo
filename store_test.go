package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// tempStore returns a project store backed by a fresh file under t.TempDir(), so
// each test gets isolated, auto-cleaned persistence.
func tempStore(t *testing.T) *store {
	t.Helper()
	return &store{scope: scopeProject, path: filepath.Join(t.TempDir(), "todos.json")}
}

func TestStoreAvailable(t *testing.T) {
	if (&store{path: ""}).available() {
		t.Error("a store with no path should be unavailable")
	}
	if !(&store{path: "/x/todos.json"}).available() {
		t.Error("a store with a path should be available")
	}
}

// TestUnavailableStoreTouchesNoDisk pins the documented contract that an
// unavailable store (an empty path, e.g. launched outside a project) loads and
// saves to nothing rather than erroring or writing a stray file.
func TestUnavailableStoreTouchesNoDisk(t *testing.T) {
	dir := t.TempDir()
	s := &store{scope: scopeProject, path: ""}

	if err := s.load(); err != nil {
		t.Fatalf("load on unavailable store = %v, want nil", err)
	}
	if s.todos != nil {
		t.Errorf("load left todos = %v, want nil", s.todos)
	}
	if err := s.save(); err != nil {
		t.Fatalf("save on unavailable store = %v, want nil", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("save on unavailable store wrote %d entries, want 0", len(entries))
	}
}

// TestSaveLoadRoundTrip writes todos through one store and reads them back
// through a second store at the same path, proving on-disk persistence survives.
func TestSaveLoadRoundTrip(t *testing.T) {
	s := tempStore(t)
	created := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	want := []Todo{
		{ID: "a1", Title: "first", Prompt: "do the first thing", Created: created},
		{ID: "b2", Title: "", Prompt: "no title here", Done: true, Created: created},
	}
	s.todos = want
	if err := s.save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	reloaded := &store{scope: scopeProject, path: s.path}
	if err := reloaded.load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(reloaded.todos) != len(want) {
		t.Fatalf("loaded %d todos, want %d", len(reloaded.todos), len(want))
	}
	for i, td := range reloaded.todos {
		if td.ID != want[i].ID || td.Title != want[i].Title || td.Prompt != want[i].Prompt || td.Done != want[i].Done {
			t.Errorf("todo[%d] = %+v, want %+v", i, td, want[i])
		}
		if !td.Created.Equal(want[i].Created) {
			t.Errorf("todo[%d].Created = %v, want %v", i, td.Created, want[i].Created)
		}
	}
}

func TestLoadMissingFileIsEmptyNotError(t *testing.T) {
	// A first run has no file yet — that must read as an empty backlog.
	s := &store{scope: scopeGlobal, path: filepath.Join(t.TempDir(), "does-not-exist.json")}
	if err := s.load(); err != nil {
		t.Fatalf("load of missing file = %v, want nil", err)
	}
	if len(s.todos) != 0 {
		t.Errorf("load of missing file yielded %d todos, want 0", len(s.todos))
	}
}

func TestLoadEmptyFileIsEmptyNotError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todos.json")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &store{scope: scopeGlobal, path: path}
	if err := s.load(); err != nil {
		t.Fatalf("load of empty file = %v, want nil", err)
	}
	if len(s.todos) != 0 {
		t.Errorf("load of empty file yielded %d todos, want 0", len(s.todos))
	}
}

func TestStoreCRUD(t *testing.T) {
	s := tempStore(t)

	if err := s.add(Todo{ID: "1", Title: "one", Prompt: "p1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.add(Todo{ID: "2", Title: "two", Prompt: "p2"}); err != nil {
		t.Fatal(err)
	}

	t.Run("find", func(t *testing.T) {
		got, ok := s.find("2")
		if !ok || got.Title != "two" {
			t.Errorf("find(2) = %+v, %v; want title two, true", got, ok)
		}
		if _, ok := s.find("nope"); ok {
			t.Error("find(nope) reported found for an unknown id")
		}
		// find returns a copy — mutating it must not touch the store.
		got.Title = "mutated"
		if again, _ := s.find("2"); again.Title != "two" {
			t.Errorf("find returned a live reference: store title became %q", again.Title)
		}
	})

	t.Run("update edits title and prompt only", func(t *testing.T) {
		if err := s.update(Todo{ID: "1", Title: "one-edited", Prompt: "p1-edited"}); err != nil {
			t.Fatal(err)
		}
		got, _ := s.find("1")
		if got.Title != "one-edited" || got.Prompt != "p1-edited" {
			t.Errorf("after update find(1) = %+v, want edited title/prompt", got)
		}
		// Updating an unknown id reports the todo as gone (it may have been
		// deleted from another pane), rather than claiming success.
		if err := s.update(Todo{ID: "ghost"}); err != errTodoNotFound {
			t.Errorf("update of unknown id = %v, want errTodoNotFound", err)
		}
	})

	t.Run("toggle flips done", func(t *testing.T) {
		if err := s.toggle("2"); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.find("2"); !got.Done {
			t.Error("toggle(2) did not mark done")
		}
		if err := s.toggle("2"); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.find("2"); got.Done {
			t.Error("second toggle(2) did not reopen")
		}
	})

	t.Run("delete removes and persists", func(t *testing.T) {
		if err := s.delete("1"); err != nil {
			t.Fatal(err)
		}
		if _, ok := s.find("1"); ok {
			t.Error("delete(1) left the todo in memory")
		}
		// Reload from disk to confirm the deletion was saved, not just in-memory.
		reloaded := &store{scope: s.scope, path: s.path}
		if err := reloaded.load(); err != nil {
			t.Fatal(err)
		}
		if len(reloaded.todos) != 1 || reloaded.todos[0].ID != "2" {
			t.Errorf("after delete, disk has %+v, want only id 2", reloaded.todos)
		}
	})

	t.Run("mutations of unknown ids report not found", func(t *testing.T) {
		if err := s.delete("ghost"); err != errTodoNotFound {
			t.Errorf("delete of unknown id = %v, want errTodoNotFound", err)
		}
		if err := s.toggle("ghost"); err != errTodoNotFound {
			t.Errorf("toggle of unknown id = %v, want errTodoNotFound", err)
		}
		if err := s.setDone("ghost", true); err != errTodoNotFound {
			t.Errorf("setDone of unknown id = %v, want errTodoNotFound", err)
		}
	})
}

// TestMutationsStartFromDisk pins the lost-update fix: two store instances at
// the same path (two manager panes sharing the global backlog) each add a todo,
// and both todos survive — the second write must not clobber the first, because
// every mutation reloads from disk before applying itself.
func TestMutationsStartFromDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todos.json")
	s1 := &store{scope: scopeGlobal, path: path}
	s2 := &store{scope: scopeGlobal, path: path}
	if err := s1.load(); err != nil {
		t.Fatal(err)
	}
	if err := s2.load(); err != nil {
		t.Fatal(err)
	}

	if err := s1.add(Todo{ID: "from-pane-1", Prompt: "p1"}); err != nil {
		t.Fatal(err)
	}
	// s2 still has an empty in-memory list; its add must pick up pane 1's todo.
	if err := s2.add(Todo{ID: "from-pane-2", Prompt: "p2"}); err != nil {
		t.Fatal(err)
	}

	reloaded := &store{scope: scopeGlobal, path: path}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if len(reloaded.todos) != 2 {
		t.Fatalf("disk has %d todos, want 2 (a stale pane clobbered the other's write)", len(reloaded.todos))
	}
	if _, ok := reloaded.find("from-pane-1"); !ok {
		t.Error("pane 1's todo was lost")
	}
	if _, ok := reloaded.find("from-pane-2"); !ok {
		t.Error("pane 2's todo was lost")
	}
}

// TestSaveLeavesNoTempFiles pins the write-then-rename save: after a save the
// directory holds exactly the backlog file, with the expected content and no
// leftover temp files.
func TestSaveLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	s := &store{scope: scopeProject, path: filepath.Join(dir, "todos.json")}
	if err := s.add(Todo{ID: "a", Prompt: "p"}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "todos.json" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("dir contains %v, want only todos.json", names)
	}
}

// TestMove covers reordering: a move swaps only with neighbors in the same done
// state, persists, and quietly no-ops at the edge of the group.
func TestMove(t *testing.T) {
	s := tempStore(t)
	for _, td := range []Todo{
		{ID: "a", Prompt: "a"},
		{ID: "b", Prompt: "b"},
		{ID: "done1", Prompt: "d", Done: true},
		{ID: "c", Prompt: "c"},
	} {
		if err := s.add(td); err != nil {
			t.Fatal(err)
		}
	}

	order := func() []string {
		ids := make([]string, len(s.todos))
		for i, td := range s.todos {
			ids[i] = td.ID
		}
		return ids
	}
	want := func(expect ...string) {
		t.Helper()
		got := order()
		for i := range expect {
			if got[i] != expect[i] {
				t.Fatalf("order = %v, want %v", got, expect)
			}
		}
	}

	// b moves down past the done todo to swap with c — same-done-state neighbors only.
	if err := s.move("b", 1); err != nil {
		t.Fatal(err)
	}
	want("a", "c", "done1", "b")

	// b moves back up, again skipping the done todo.
	if err := s.move("b", -1); err != nil {
		t.Fatal(err)
	}
	want("a", "b", "done1", "c")

	// A move past the edge is a no-op, not an error.
	if err := s.move("a", -1); err != nil {
		t.Fatal(err)
	}
	want("a", "b", "done1", "c")

	if err := s.move("ghost", 1); err != errTodoNotFound {
		t.Errorf("move of unknown id = %v, want errTodoNotFound", err)
	}

	// The new order must be on disk, not just in memory.
	reloaded := &store{scope: s.scope, path: s.path}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if reloaded.todos[0].ID != "a" || reloaded.todos[1].ID != "b" {
		t.Errorf("disk order = %v, want the moved order persisted", reloaded.todos)
	}
}

// TestReorder covers the drag's store primitive: a todo lands exactly on the
// slot its target held, in either direction, everything in between slides along
// keeping its own order, and a target in another render group is refused
// silently rather than moving the row somewhere the list cannot draw it.
func TestReorder(t *testing.T) {
	build := func(t *testing.T) *store {
		t.Helper()
		s := tempStore(t)
		for _, td := range []Todo{
			{ID: "a", Prompt: "a"},
			{ID: "b", Prompt: "b"},
			{ID: "done1", Prompt: "d", Done: true},
			{ID: "c", Prompt: "c"},
			{ID: "e", Prompt: "e"},
		} {
			if err := s.add(td); err != nil {
				t.Fatal(err)
			}
		}
		return s
	}
	order := func(s *store) []string {
		ids := make([]string, len(s.todos))
		for i, td := range s.todos {
			ids[i] = td.ID
		}
		return ids
	}

	t.Run("dragged down, it takes the target's slot", func(t *testing.T) {
		s := build(t)
		// a is dragged onto e, three rows down: everything it passed — the done
		// todo caught in the middle included — slides up one and keeps its order.
		moved, err := s.reorder("a", "e")
		if err != nil || !moved {
			t.Fatalf("reorder = (%v, %v), want it moved", moved, err)
		}
		if got := order(s); !slices.Equal(got, []string{"b", "done1", "c", "e", "a"}) {
			t.Fatalf("order = %v, want a landed where e was", got)
		}
	})

	t.Run("dragged up, it takes the target's slot", func(t *testing.T) {
		s := build(t)
		moved, err := s.reorder("e", "a")
		if err != nil || !moved {
			t.Fatalf("reorder = (%v, %v), want it moved", moved, err)
		}
		if got := order(s); !slices.Equal(got, []string{"e", "a", "b", "done1", "c"}) {
			t.Fatalf("order = %v, want e landed where a was", got)
		}
	})

	t.Run("a target in another group is a quiet no-op", func(t *testing.T) {
		s := build(t)
		// Silent, but it must not claim to have moved anything: the caller
		// reports the result, and the user is looking at the row that didn't go.
		moved, err := s.reorder("a", "done1")
		if err != nil {
			t.Fatalf("cross-group reorder = %v, want a silent no-op", err)
		}
		if moved {
			t.Error("cross-group reorder reported a move it did not make")
		}
		if got := order(s); !slices.Equal(got, []string{"a", "b", "done1", "c", "e"}) {
			t.Fatalf("order = %v, want it untouched", got)
		}
	})

	t.Run("onto itself, and onto nothing", func(t *testing.T) {
		s := build(t)
		if moved, err := s.reorder("a", "a"); err != nil || moved {
			t.Errorf("reorder onto itself = (%v, %v), want a no-op", moved, err)
		}
		if _, err := s.reorder("a", "ghost"); err != errTodoNotFound {
			t.Errorf("reorder onto an unknown id = %v, want errTodoNotFound", err)
		}
		if _, err := s.reorder("ghost", "a"); err != errTodoNotFound {
			t.Errorf("reorder of an unknown id = %v, want errTodoNotFound", err)
		}
		if got := order(s); !slices.Equal(got, []string{"a", "b", "done1", "c", "e"}) {
			t.Fatalf("order = %v, want it untouched", got)
		}
	})

	t.Run("the new order is on disk", func(t *testing.T) {
		s := build(t)
		if _, err := s.reorder("a", "c"); err != nil {
			t.Fatal(err)
		}
		reloaded := &store{scope: s.scope, path: s.path}
		if err := reloaded.load(); err != nil {
			t.Fatal(err)
		}
		if got := order(reloaded); !slices.Equal(got, []string{"b", "done1", "c", "a", "e"}) {
			t.Errorf("disk order = %v, want the dragged order persisted", got)
		}
	})
}

// TestCompletionOrder pins the done pile as newest-first: completing a prompt
// files it ahead of everything already finished, however far down the backlog it
// started, and the open todos it passed keep their own order.
func TestCompletionOrder(t *testing.T) {
	s := tempStore(t)
	for _, id := range []string{"a", "b", "c", "d"} {
		if err := s.add(Todo{ID: id, Prompt: id}); err != nil {
			t.Fatal(err)
		}
	}

	order := func() string {
		var b strings.Builder
		for _, td := range s.todos {
			b.WriteString(td.ID)
			if td.Done {
				b.WriteString("✓")
			}
			b.WriteString(" ")
		}
		return strings.TrimSpace(b.String())
	}

	// The first completion has no done pile to lead, so nothing moves.
	if err := s.toggle("c"); err != nil {
		t.Fatal(err)
	}
	if got := order(); got != "a b c✓ d" {
		t.Fatalf("order = %q, want the first completion left in place", got)
	}

	// The next one jumps the whole pile — a and b slide down, still in order.
	if err := s.toggle("d"); err != nil {
		t.Fatal(err)
	}
	if got := order(); got != "a b d✓ c✓" {
		t.Fatalf("order = %q, want d✓ ahead of c✓", got)
	}

	// A drop auto-completes through setDone, which files it the same way — from
	// above the pile this time, where there is nothing to move past.
	if err := s.setDone("a", true); err != nil {
		t.Fatal(err)
	}
	if got := order(); got != "a✓ b d✓ c✓" {
		t.Fatalf("order = %q, want a✓ ahead of the pile", got)
	}
	if err := s.setDone("b", true); err != nil {
		t.Fatal(err)
	}
	if got := order(); got != "b✓ a✓ d✓ c✓" {
		t.Fatalf("order = %q, want newest-completed first", got)
	}

	// Reopening leaves the todo where it is — it rejoins the open list rather
	// than being filed anywhere new.
	if err := s.toggle("d"); err != nil {
		t.Fatal(err)
	}
	if got := order(); got != "b✓ a✓ d c✓" {
		t.Fatalf("order = %q, want the reopened todo left in place", got)
	}

	// The order is on disk, not just in memory.
	reloaded := &store{scope: s.scope, path: s.path}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if reloaded.todos[0].ID != "b" || reloaded.todos[3].ID != "c" {
		t.Errorf("disk order = %+v, want the completion order persisted", reloaded.todos)
	}
}

// TestClearDone covers bulk cleanup: only done todos are removed, the count is
// reported, and clearing an already-clean store is a zero no-op.
func TestClearDone(t *testing.T) {
	s := tempStore(t)
	for _, td := range []Todo{
		{ID: "open1", Prompt: "p"},
		{ID: "done1", Prompt: "p", Done: true},
		{ID: "done2", Prompt: "p", Done: true},
		{ID: "open2", Prompt: "p"},
	} {
		if err := s.add(td); err != nil {
			t.Fatal(err)
		}
	}

	n, err := s.clearDone()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("clearDone removed %d, want 2", n)
	}
	if len(s.todos) != 2 || s.todos[0].ID != "open1" || s.todos[1].ID != "open2" {
		t.Errorf("after clearDone todos = %+v, want the two open ones in order", s.todos)
	}

	n, err = s.clearDone()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("second clearDone removed %d, want 0", n)
	}
}

func TestProjectTodosPath(t *testing.T) {
	if got := projectTodosPath(""); got != "" {
		t.Errorf("projectTodosPath(\"\") = %q, want empty (no project scope)", got)
	}
	want := filepath.Join("/repo", projectConfigDirName, "todos.json")
	if got := projectTodosPath("/repo"); got != want {
		t.Errorf("projectTodosPath(/repo) = %q, want %q", got, want)
	}
}

// TestConfigBaseDir exercises the documented precedence: the CATS_TODO_CONFIG_DIR
// override wins, then XDG_CONFIG_HOME/cats-todo, then ~/.config. These use
// t.Setenv and so must not run in parallel.
func TestConfigBaseDir(t *testing.T) {
	t.Run("CATS_TODO_CONFIG_DIR wins", func(t *testing.T) {
		t.Setenv("CATS_TODO_CONFIG_DIR", "/cats-todo/cfg")
		t.Setenv("XDG_CONFIG_HOME", "/xdg")
		got, err := configBaseDir()
		if err != nil {
			t.Fatal(err)
		}
		if got != "/cats-todo/cfg" {
			t.Errorf("configBaseDir = %q, want the override dir", got)
		}
	})

	t.Run("falls back to XDG_CONFIG_HOME", func(t *testing.T) {
		t.Setenv("CATS_TODO_CONFIG_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", "/xdg")
		got, err := configBaseDir()
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join("/xdg", "cats-todo"); got != want {
			t.Errorf("configBaseDir = %q, want %q", got, want)
		}
	})

	t.Run("falls back to home .config", func(t *testing.T) {
		t.Setenv("CATS_TODO_CONFIG_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("no home dir available: %v", err)
		}
		got, err := configBaseDir()
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(home, ".config", "cats-todo"); got != want {
			t.Errorf("configBaseDir = %q, want %q", got, want)
		}
	})
}

// TestLoadStores checks that a launch with a project workdir produces an
// available, loaded project store plus the global store, while an empty workdir
// leaves the project store unavailable (global-only mode).
func TestLoadStores(t *testing.T) {
	// Point the global backlog at an isolated dir so the test never touches the
	// real user config.
	t.Setenv("CATS_TODO_CONFIG_DIR", t.TempDir())

	t.Run("with a project workdir", func(t *testing.T) {
		work := t.TempDir()
		project, global, err := loadStores(RunContext{WorkDir: work})
		if err != nil {
			t.Fatal(err)
		}
		if !project.available() {
			t.Error("project store should be available when WorkDir is set")
		}
		if project.path != projectTodosPath(work) {
			t.Errorf("project path = %q, want %q", project.path, projectTodosPath(work))
		}
		if !global.available() {
			t.Error("global store should always be available")
		}
	})

	t.Run("a resolved root outranks the workdir", func(t *testing.T) {
		// The subdirectory launch: the manager runs in <root>/sub but must load
		// the backlog at <root>, the same file `cats-todo add` writes there.
		root := t.TempDir()
		project, _, err := loadStores(RunContext{WorkDir: filepath.Join(root, "sub"), ProjectRoot: root})
		if err != nil {
			t.Fatal(err)
		}
		if project.path != projectTodosPath(root) {
			t.Errorf("project path = %q, want the project root's backlog %q", project.path, projectTodosPath(root))
		}
	})

	t.Run("without a workdir is global-only", func(t *testing.T) {
		project, global, err := loadStores(RunContext{WorkDir: ""})
		if err != nil {
			t.Fatal(err)
		}
		if project.available() {
			t.Error("project store should be unavailable with no WorkDir")
		}
		if !global.available() {
			t.Error("global store should still be available")
		}
	})

	t.Run("the --global launch withholds the project store", func(t *testing.T) {
		// A directory IS resolvable here — global-only must decline it anyway.
		project, global, err := loadStores(RunContext{WorkDir: t.TempDir(), Scope: launchGlobalOnly})
		if err != nil {
			t.Fatal(err)
		}
		if project.available() {
			t.Error("project store should be unavailable in global-only mode")
		}
		if !global.available() {
			t.Error("global store should be available in global-only mode")
		}
	})

	t.Run("the --project launch withholds the global store", func(t *testing.T) {
		work := t.TempDir()
		project, global, err := loadStores(RunContext{WorkDir: work, Scope: launchProjectOnly})
		if err != nil {
			t.Fatal(err)
		}
		if !project.available() || project.path != projectTodosPath(work) {
			t.Errorf("project store = %+v, want available at %q", project, projectTodosPath(work))
		}
		if global.available() {
			t.Error("global store should be unavailable in project-only mode")
		}
	})
}

// TestSetAndClearSchedule round-trips a schedule through disk, and pins the
// omitempty contract: a backlog with no schedules must serialize without the
// key at all, so older binaries and diffs see exactly the file they always did.
func TestSetAndClearSchedule(t *testing.T) {
	s := tempStore(t)
	if err := s.add(Todo{ID: "a1", Prompt: "p"}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "schedule") {
		t.Fatalf("an unscheduled backlog mentions schedules:\n%s", raw)
	}

	at := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)
	sc := &Schedule{At: at, Kind: scheduleKindPane, Pane: 42, Agent: "claude"}
	if err := s.setSchedule("a1", sc); err != nil {
		t.Fatal(err)
	}

	reloaded := &store{scope: scopeProject, path: s.path}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	got, _ := reloaded.find("a1")
	if got.Schedule == nil || !got.Schedule.At.Equal(at) || got.Schedule.Pane != 42 {
		t.Fatalf("schedule did not survive the round trip: %+v", got.Schedule)
	}

	if err := s.setSchedule("a1", nil); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "schedule") {
		t.Fatalf("a cleared schedule left its key behind:\n%s", raw)
	}

	if err := s.setSchedule("ghost", sc); err != errTodoNotFound {
		t.Errorf("setSchedule of unknown id = %v, want errTodoNotFound", err)
	}
}

// TestClaimSchedule pins the double-fire guard: exactly one claimant wins, a
// claim is only good for the exact At it read, and a schedule cleared from
// another pane cannot be claimed at all.
func TestClaimSchedule(t *testing.T) {
	at := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)

	build := func(t *testing.T) *store {
		t.Helper()
		s := tempStore(t)
		if err := s.add(Todo{ID: "a1", Prompt: "p"}); err != nil {
			t.Fatal(err)
		}
		if err := s.setSchedule("a1", &Schedule{At: at, Kind: scheduleKindNew, Command: "claude"}); err != nil {
			t.Fatal(err)
		}
		return s
	}

	t.Run("first claim wins, second finds it gone", func(t *testing.T) {
		s := build(t)
		if won, err := s.claimSchedule("a1", at); err != nil || !won {
			t.Fatalf("first claim = %v, %v — want a win", won, err)
		}
		if td, _ := s.find("a1"); td.Schedule != nil {
			t.Fatal("a won claim must clear the schedule on disk")
		}
		if won, _ := s.claimSchedule("a1", at); won {
			t.Fatal("the second claim won a schedule that was already taken")
		}
	})

	t.Run("a changed At is someone else's schedule", func(t *testing.T) {
		s := build(t)
		if won, _ := s.claimSchedule("a1", at.Add(time.Minute)); won {
			t.Fatal("claimed against an At this caller never read")
		}
		if td, _ := s.find("a1"); td.Schedule == nil {
			t.Fatal("the losing claim must leave the schedule in place")
		}
	})

	t.Run("cleared from another pane", func(t *testing.T) {
		s := build(t)
		other := &store{scope: scopeProject, path: s.path}
		if err := other.load(); err != nil {
			t.Fatal(err)
		}
		if err := other.setSchedule("a1", nil); err != nil {
			t.Fatal(err)
		}
		// s's in-memory copy still shows the schedule; the claim must reload
		// and stand down.
		if won, _ := s.claimSchedule("a1", at); won {
			t.Fatal("claimed a schedule another pane had already cleared")
		}
	})

	t.Run("deleted todo", func(t *testing.T) {
		s := build(t)
		if err := s.delete("a1"); err != nil {
			t.Fatal(err)
		}
		if won, err := s.claimSchedule("a1", at); won || err != nil {
			t.Fatalf("claim on a deleted todo = %v, %v — want a quiet stand-down", won, err)
		}
	})
}

// TestDoneClearsSchedule pins the "schedule implies open" invariant on both
// write paths that can complete a todo.
func TestDoneClearsSchedule(t *testing.T) {
	at := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name string
		done func(*store) error
	}{
		{"toggle", func(s *store) error { return s.toggle("a1") }},
		{"setDone", func(s *store) error { return s.setDone("a1", true) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := tempStore(t)
			if err := s.add(Todo{ID: "a1", Prompt: "p"}); err != nil {
				t.Fatal(err)
			}
			if err := s.setSchedule("a1", &Schedule{At: at, Kind: scheduleKindNew}); err != nil {
				t.Fatal(err)
			}
			if err := tc.done(s); err != nil {
				t.Fatal(err)
			}
			td, _ := s.find("a1")
			if !td.Done {
				t.Fatal("the todo should be done")
			}
			if td.Schedule != nil {
				t.Fatal("completing a todo must clear its schedule")
			}
		})
	}

	t.Run("reopening does not resurrect it", func(t *testing.T) {
		s := tempStore(t)
		if err := s.add(Todo{ID: "a1", Prompt: "p"}); err != nil {
			t.Fatal(err)
		}
		if err := s.setSchedule("a1", &Schedule{At: at, Kind: scheduleKindNew}); err != nil {
			t.Fatal(err)
		}
		if err := s.toggle("a1"); err != nil { // done — clears the schedule
			t.Fatal(err)
		}
		if err := s.toggle("a1"); err != nil { // reopened
			t.Fatal(err)
		}
		if td, _ := s.find("a1"); td.Schedule != nil {
			t.Fatal("a reopened todo must come back unscheduled")
		}
	})
}

// TestFreeze pins the frozen state's whole contract: the flags it is exclusive
// with, the schedule it cancels, the array position it leaves alone, and the
// round trip through JSON.
func TestFreeze(t *testing.T) {
	at := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)

	t.Run("toggle reports the state it landed in", func(t *testing.T) {
		s := tempStore(t)
		if err := s.add(Todo{ID: "a1", Prompt: "p"}); err != nil {
			t.Fatal(err)
		}
		frozen, err := s.toggleFrozen("a1")
		if err != nil || !frozen {
			t.Fatalf("toggleFrozen = %v, %v — want true, nil", frozen, err)
		}
		if td, _ := s.find("a1"); !td.Frozen {
			t.Fatal("the todo should be frozen")
		}
		if frozen, err = s.toggleFrozen("a1"); err != nil || frozen {
			t.Fatalf("second toggleFrozen = %v, %v — want false, nil", frozen, err)
		}
		if td, _ := s.find("a1"); td.Frozen {
			t.Fatal("the todo should be thawed")
		}
	})

	t.Run("freezing cancels a pending schedule", func(t *testing.T) {
		s := tempStore(t)
		if err := s.add(Todo{ID: "a1", Prompt: "p"}); err != nil {
			t.Fatal(err)
		}
		if err := s.setSchedule("a1", &Schedule{At: at, Kind: scheduleKindNew}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.toggleFrozen("a1"); err != nil {
			t.Fatal(err)
		}
		if td, _ := s.find("a1"); td.Schedule != nil {
			t.Fatal("freezing a todo must clear its schedule — nothing shelved may fire")
		}
	})

	t.Run("frozen and done are exclusive", func(t *testing.T) {
		s := tempStore(t)
		if err := s.add(Todo{ID: "a1", Prompt: "p", Done: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.toggleFrozen("a1"); err != nil {
			t.Fatal(err)
		}
		if td, _ := s.find("a1"); !td.Frozen || td.Done {
			t.Fatalf("freezing a done todo = {Frozen:%v Done:%v}, want frozen and not done", td.Frozen, td.Done)
		}
		// …and the other way round: completing thaws.
		if err := s.setDone("a1", true); err != nil {
			t.Fatal(err)
		}
		if td, _ := s.find("a1"); td.Frozen || !td.Done {
			t.Fatalf("completing a frozen todo = {Frozen:%v Done:%v}, want done and not frozen", td.Frozen, td.Done)
		}
	})

	t.Run("setFrozen is idempotent", func(t *testing.T) {
		s := tempStore(t)
		if err := s.add(Todo{ID: "a1", Prompt: "p"}); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := s.setFrozen("a1", true); err != nil {
				t.Fatal(err)
			}
		}
		if td, _ := s.find("a1"); !td.Frozen {
			t.Fatal("the todo should be frozen")
		}
		if err := s.setFrozen("missing", true); err != errTodoNotFound {
			t.Fatalf("setFrozen on a missing todo = %v, want errTodoNotFound", err)
		}
	})

	// A thaw hands the prompt back the place it had — unlike completing, which
	// files the todo at the head of the done group. Freezing is a decision about
	// the work, not a claim it happened, so it must not cost the prompt its
	// priority.
	t.Run("freezing holds the array position", func(t *testing.T) {
		s := tempStore(t)
		for _, id := range []string{"a1", "a2", "a3"} {
			if err := s.add(Todo{ID: id, Prompt: id}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.toggleFrozen("a2"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.toggleFrozen("a2"); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, td := range s.todos {
			got = append(got, td.ID)
		}
		if strings.Join(got, ",") != "a1,a2,a3" {
			t.Fatalf("order after a freeze/thaw = %v, want a1,a2,a3", got)
		}
	})

	// The compat contract the field's comment promises: an unfrozen backlog reads
	// exactly as it did before the field existed.
	t.Run("the flag round-trips and stays out of the JSON when false", func(t *testing.T) {
		s := tempStore(t)
		if err := s.add(Todo{ID: "a1", Prompt: "p"}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(s.path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "frozen") {
			t.Fatalf("an unfrozen todo wrote a frozen key:\n%s", data)
		}
		if _, err := s.toggleFrozen("a1"); err != nil {
			t.Fatal(err)
		}
		reloaded := &store{scope: scopeProject, path: s.path}
		if err := reloaded.load(); err != nil {
			t.Fatal(err)
		}
		if td, _ := reloaded.find("a1"); !td.Frozen {
			t.Fatal("the frozen flag did not survive a save/load round trip")
		}
	})

	// Freezing is a record the user asked to keep; the sweep that clears finished
	// work must not take it with them.
	t.Run("clearDone leaves frozen todos alone", func(t *testing.T) {
		s := tempStore(t)
		if err := s.add(Todo{ID: "done", Prompt: "d", Done: true}); err != nil {
			t.Fatal(err)
		}
		if err := s.add(Todo{ID: "frozen", Prompt: "f", Frozen: true}); err != nil {
			t.Fatal(err)
		}
		n, err := s.clearDone()
		if err != nil || n != 1 {
			t.Fatalf("clearDone = %d, %v — want 1, nil", n, err)
		}
		if _, ok := s.find("frozen"); !ok {
			t.Fatal("clearDone swept away a frozen todo")
		}
	})

	// Reordering swaps within a render group, and frozen is its own group: a
	// frozen todo has no neighbor to trade with here, so the move is a quiet
	// no-op rather than a swap the list could not show.
	t.Run("move stays inside the frozen group", func(t *testing.T) {
		s := tempStore(t)
		for _, td := range []Todo{
			{ID: "a1", Prompt: "open"},
			{ID: "a2", Prompt: "frozen", Frozen: true},
			{ID: "a3", Prompt: "done", Done: true},
		} {
			if err := s.add(td); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.move("a2", -1); err != nil {
			t.Fatal(err)
		}
		if err := s.move("a2", 1); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, td := range s.todos {
			got = append(got, td.ID)
		}
		if strings.Join(got, ",") != "a1,a2,a3" {
			t.Fatalf("order = %v, want a1,a2,a3 — a frozen todo must not swap across groups", got)
		}
	})
}

// TestSetAndClearSession mirrors TestSetAndClearSchedule for the other nullable
// field: the options round-trip through the file, a backlog without them
// mentions nothing, and — the part that matters — an ordinary text edit leaves
// them alone. update() is text-only by design (see its comment), so a caller
// that knows nothing about session options cannot blank them.
func TestSetAndClearSession(t *testing.T) {
	s := tempStore(t)
	if err := s.add(Todo{ID: "a1", Title: "t", Prompt: "p"}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "session") {
		t.Fatalf("a backlog with no options mentions sessions:\n%s", raw)
	}

	opts := &SessionOpts{
		Model: "sonnet", Effort: effortHigh, Permission: permAcceptEdits,
		Clear: true, Context: ctxLoad, ContextArg: "2",
		Files: []string{"ai_docs/design.md"}, Finish: finishWrap,
		Reviews: []string{reviewCode}, Release: true,
	}
	if err := s.setSession("a1", opts); err != nil {
		t.Fatal(err)
	}

	reloaded := &store{scope: scopeProject, path: s.path}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	got, _ := reloaded.find("a1")
	if got.Session == nil {
		t.Fatal("the options did not survive the round trip")
	}
	if got.Session.Model != "sonnet" || got.Session.Effort != effortHigh ||
		!got.Session.Clear || got.Session.ContextArg != "2" ||
		len(got.Session.Files) != 1 || got.Session.Finish != finishWrap ||
		len(got.Session.Reviews) != 1 || !got.Session.Release {
		t.Fatalf("round-tripped options = %+v, want them all back", got.Session)
	}

	// The guarantee: editing the text does not touch the options.
	if err := s.update(Todo{ID: "a1", Title: "new title", Prompt: "new prompt"}); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	got, _ = reloaded.find("a1")
	if got.Prompt != "new prompt" {
		t.Errorf("prompt = %q, want the edit saved", got.Prompt)
	}
	if got.Session == nil || got.Session.Model != "sonnet" {
		t.Fatalf("a text edit blanked the session options: %+v", got.Session)
	}

	if err := s.setSession("a1", nil); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "session") {
		t.Fatalf("cleared options left their key behind:\n%s", raw)
	}

	if err := s.setSession("ghost", opts); err != errTodoNotFound {
		t.Errorf("setSession of unknown id = %v, want errTodoNotFound", err)
	}
}

// TestAddAfter: a run of todos goes in behind the anchor in one write, and an
// anchor that is not there — an empty id in add mode, or one another pane
// deleted while the form was open — appends rather than failing.
func TestAddAfter(t *testing.T) {
	s := tempStore(t)
	for _, id := range []string{"a", "b", "c"} {
		if err := s.add(Todo{ID: id, Title: id, Prompt: id}); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.addAfter("b", []Todo{{ID: "b1", Prompt: "b1"}, {ID: "b2", Prompt: "b2"}}); err != nil {
		t.Fatal(err)
	}
	ids := func(st *store) string {
		var b strings.Builder
		for _, td := range st.todos {
			b.WriteString(td.ID + " ")
		}
		return strings.TrimSpace(b.String())
	}
	if got, want := ids(s), "a b b1 b2 c"; got != want {
		t.Errorf("order = %q, want %q", got, want)
	}

	for _, anchor := range []string{"", "ghost"} {
		if err := s.addAfter(anchor, []Todo{{ID: "z" + anchor}}); err != nil {
			t.Fatalf("addAfter(%q): %v", anchor, err)
		}
	}
	if got, want := ids(s), "a b b1 b2 c z zghost"; got != want {
		t.Errorf("order = %q, want the unanchored runs appended: %q", got, want)
	}

	// Nothing to add is not an error, and must not rewrite the file.
	if err := s.addAfter("b", nil); err != nil {
		t.Errorf("addAfter with no todos = %v, want nil", err)
	}

	// And it is on disk, not only in memory.
	reloaded := &store{scope: s.scope, path: s.path}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if got, want := ids(reloaded), ids(s); got != want {
		t.Errorf("disk order = %q, want %q", got, want)
	}
}
