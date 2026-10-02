package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rohanthewiz/cats-todo/internal/integration"
	"github.com/rohanthewiz/cats/wire"
)

// TestGatherRunContextWithoutClient pins the degraded launch: with no control
// socket, the context still resolves the working directory (this process's cwd)
// and the workspace id from the pane handle's "w1" prefix — enough to scope
// project todos and mark same-workspace targets.
func TestGatherRunContextWithoutClient(t *testing.T) {
	t.Run("handle-form pane id yields the workspace", func(t *testing.T) {
		t.Setenv(integration.CatsPaneIDEnvVar, "w2:p5")
		ctx := gatherRunContext(nil, launchBoth)
		if ctx.OwnPane != "w2:p5" {
			t.Errorf("OwnPane = %q, want w2:p5", ctx.OwnPane)
		}
		if ctx.WorkspaceID != "w2" {
			t.Errorf("WorkspaceID = %q, want w2 (handle prefix)", ctx.WorkspaceID)
		}
		if ctx.WorkDir == "" {
			t.Error("WorkDir should resolve to the process cwd")
		}
	})

	t.Run("fallback-form pane id yields no workspace", func(t *testing.T) {
		t.Setenv(integration.CatsPaneIDEnvVar, "p_7")
		ctx := gatherRunContext(nil, launchBoth)
		if ctx.OwnPane != "p_7" || ctx.WorkspaceID != "" {
			t.Errorf("ctx = %+v, want OwnPane p_7 with no workspace (needs the client)", ctx)
		}
	})

	t.Run("outside cats yields a bare cwd context", func(t *testing.T) {
		t.Setenv(integration.CatsPaneIDEnvVar, "")
		ctx := gatherRunContext(nil, launchBoth)
		if ctx.OwnPane != "" || ctx.WorkspaceID != "" || ctx.WorkspaceLabel != "" {
			t.Errorf("ctx = %+v, want only the directory fields set outside cats", ctx)
		}
	})

	t.Run("resolves the project root above the cwd", func(t *testing.T) {
		// Run from a subdirectory of a marker-bearing repo: the walk must land
		// on the ancestor holding the .git marker — never the cwd itself. That
		// is precisely the subdirectory case that used to scope the manager to
		// the wrong backlog. A synthetic repo (rather than this package's own
		// checkout) keeps the test independent of where the module is cloned:
		// the package dir is now itself a repo root, so the real checkout no
		// longer exercises the ancestor case.
		root := t.TempDir()
		mkdir(t, filepath.Join(root, ".git"))
		sub := filepath.Join(root, "cmd", "deep")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Chdir(sub)
		t.Setenv(integration.CatsPaneIDEnvVar, "")
		ctx := gatherRunContext(nil, launchBoth)
		if ctx.ProjectRoot == "" {
			t.Fatal("ProjectRoot should resolve whenever WorkDir does")
		}
		if ctx.ProjectRoot == ctx.WorkDir {
			t.Errorf("ProjectRoot = WorkDir = %q, want an ancestor (the repo root)", ctx.WorkDir)
		}
		if !strings.HasPrefix(ctx.WorkDir, ctx.ProjectRoot) {
			t.Errorf("ProjectRoot %q is not an ancestor of WorkDir %q", ctx.ProjectRoot, ctx.WorkDir)
		}
	})

	t.Run("global-only skips the project walk but keeps the cwd", func(t *testing.T) {
		// Same marker-bearing layout as the subtest above — the point is that
		// --global must NOT resolve the repo root it easily could have, while
		// WorkDir stays for rooting new-session drop tabs.
		root := t.TempDir()
		mkdir(t, filepath.Join(root, ".git"))
		sub := filepath.Join(root, "cmd", "deep")
		mkdir(t, sub)
		t.Chdir(sub)
		t.Setenv(integration.CatsPaneIDEnvVar, "")
		ctx := gatherRunContext(nil, launchGlobalOnly)
		if ctx.Scope != launchGlobalOnly || ctx.ProjectRoot != "" {
			t.Errorf("ctx = %+v, want global-only scope with no ProjectRoot", ctx)
		}
		if ctx.WorkDir == "" {
			t.Error("WorkDir should still resolve in global-only mode")
		}
	})

	t.Run("a launch at the filesystem root has no project", func(t *testing.T) {
		// The mac app's GUI-launch cwd, inherited all the way down to the pane.
		// WorkDir stays honest (that IS where the pane is), but no project owns
		// it, so loadStores leaves the project backlog unavailable instead of
		// aiming it at a read-only /.cats-todo.
		t.Chdir(string(filepath.Separator))
		t.Setenv(integration.CatsPaneIDEnvVar, "")
		ctx := gatherRunContext(nil, launchProjectOnly)
		if ctx.ProjectRoot != "" || ctx.projectDir() != "" {
			t.Errorf("ctx = %+v, projectDir = %q; want no project at the filesystem root",
				ctx, ctx.projectDir())
		}
		project, _, err := loadStores(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if project.available() {
			t.Errorf("project store path = %q, want unavailable", project.path)
		}
	})

	t.Run("project-only keeps the project walk", func(t *testing.T) {
		// --project narrows what shows, not where it looks: the root walk must
		// still run so the subdirectory launch reaches the project's backlog.
		root := t.TempDir()
		mkdir(t, filepath.Join(root, ".git"))
		sub := filepath.Join(root, "cmd", "deep")
		mkdir(t, sub)
		t.Chdir(sub)
		t.Setenv(integration.CatsPaneIDEnvVar, "")
		ctx := gatherRunContext(nil, launchProjectOnly)
		if ctx.Scope != launchProjectOnly || ctx.ProjectRoot == "" || ctx.ProjectRoot == ctx.WorkDir {
			t.Errorf("ctx = %+v, want project-only scope rooted at the repo root", ctx)
		}
	})
}

// TestFindProjectRoot covers the walk both entry points share: an existing
// backlog wins over an enclosing repo root, a repo root is the fallback, and a
// directory with neither marker roots itself.
func TestFindProjectRoot(t *testing.T) {
	t.Run("an existing backlog wins", func(t *testing.T) {
		root := t.TempDir()
		// A repo root above, a backlog below it: the backlog is the answer even
		// though the .git marker is closer to the top of the walk.
		mkdir(t, filepath.Join(root, ".git"))
		mkdir(t, filepath.Join(root, "sub", projectConfigDirName))
		deep := filepath.Join(root, "sub", "a", "b")
		mkdir(t, deep)

		if got := findProjectRoot(deep); got != filepath.Join(root, "sub") {
			t.Errorf("findProjectRoot = %q, want the nearest existing backlog %q", got, filepath.Join(root, "sub"))
		}
	})

	t.Run("falls back to the repo root", func(t *testing.T) {
		root := t.TempDir()
		mkdir(t, filepath.Join(root, ".git"))
		deep := filepath.Join(root, "cmd", "tool")
		mkdir(t, deep)

		if got := findProjectRoot(deep); got != root {
			t.Errorf("findProjectRoot = %q, want the repo root %q", got, root)
		}
	})

	t.Run("the repo root is a boundary an outer backlog does not cross", func(t *testing.T) {
		// The bug this walk used to have: a repo with no backlog of its own
		// adopted an unrelated .cats-todo further up the tree (a ~/.cats-todo,
		// a parent directory's), because the backlog search ran to "/" before
		// .git was ever considered. The repo root must win, backlog or not.
		outer := t.TempDir()
		mkdir(t, filepath.Join(outer, projectConfigDirName))
		repo := filepath.Join(outer, "work", "app")
		mkdir(t, filepath.Join(repo, ".git"))
		deep := filepath.Join(repo, "cmd", "tool")
		mkdir(t, deep)

		if got := findProjectRoot(deep); got != repo {
			t.Errorf("findProjectRoot = %q, want the repo root %q (not the outer backlog at %q)", got, repo, outer)
		}
		if !projectBacklogMissing(repo) {
			t.Error("projectBacklogMissing(repo) = false, want true: the repo has no .cats-todo of its own")
		}
	})

	t.Run("launched at the repo root itself stops there too", func(t *testing.T) {
		outer := t.TempDir()
		mkdir(t, filepath.Join(outer, projectConfigDirName))
		repo := filepath.Join(outer, "app")
		mkdir(t, filepath.Join(repo, ".git"))

		if got := findProjectRoot(repo); got != repo {
			t.Errorf("findProjectRoot = %q, want the starting repo root %q", got, repo)
		}
	})

	t.Run("a .git file (worktree, submodule) is a boundary too", func(t *testing.T) {
		outer := t.TempDir()
		mkdir(t, filepath.Join(outer, projectConfigDirName))
		wt := filepath.Join(outer, "wt")
		mkdir(t, wt)
		if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := findProjectRoot(wt); got != wt {
			t.Errorf("findProjectRoot = %q, want the worktree root %q", got, wt)
		}
	})

	t.Run("a backlog beside .git is the repo's own", func(t *testing.T) {
		repo := t.TempDir()
		mkdir(t, filepath.Join(repo, ".git"))
		mkdir(t, filepath.Join(repo, projectConfigDirName))
		deep := filepath.Join(repo, "a")
		mkdir(t, deep)

		if got := findProjectRoot(deep); got != repo {
			t.Errorf("findProjectRoot = %q, want %q", got, repo)
		}
		if projectBacklogMissing(repo) {
			t.Error("projectBacklogMissing = true for a repo that has a .cats-todo")
		}
	})

	t.Run("outside any repo an ancestor backlog is still found", func(t *testing.T) {
		// No .git anywhere means no boundary: a plain directory tree keeps one
		// backlog at its top, as before.
		top := t.TempDir()
		mkdir(t, filepath.Join(top, projectConfigDirName))
		deep := filepath.Join(top, "notes", "2026")
		mkdir(t, deep)

		if got := findProjectRoot(deep); got != top {
			t.Errorf("findProjectRoot = %q, want the ancestor backlog %q", got, top)
		}
	})

	t.Run("a non-repo walk stops below $HOME", func(t *testing.T) {
		// ~/.cats-todo must not become the backlog of every non-repo folder in
		// the home directory — the same leak the repo boundary closes.
		home := t.TempDir()
		t.Setenv("HOME", home)
		mkdir(t, filepath.Join(home, projectConfigDirName))
		loose := filepath.Join(home, "scratch", "deep")
		mkdir(t, loose)

		if got := findProjectRoot(loose); got != loose {
			t.Errorf("findProjectRoot = %q, want the directory itself %q (not $HOME's backlog)", got, loose)
		}
	})

	t.Run("a backlog below $HOME is still found from beneath it", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		notes := filepath.Join(home, "notes")
		mkdir(t, filepath.Join(notes, projectConfigDirName))
		deep := filepath.Join(notes, "2026", "oct")
		mkdir(t, deep)

		if got := findProjectRoot(deep); got != notes {
			t.Errorf("findProjectRoot = %q, want the ancestor backlog %q", got, notes)
		}
	})

	t.Run("launched at $HOME uses its own backlog and looks no higher", func(t *testing.T) {
		parent := t.TempDir()
		mkdir(t, filepath.Join(parent, projectConfigDirName))
		home := filepath.Join(parent, "u")
		mkdir(t, home)
		t.Setenv("HOME", home)

		if got := findProjectRoot(home); got != home {
			t.Errorf("findProjectRoot($HOME) = %q, want $HOME itself, not the backlog above it", got)
		}
		mkdir(t, filepath.Join(home, projectConfigDirName))
		if got := findProjectRoot(home); got != home {
			t.Errorf("findProjectRoot($HOME) with ~/.cats-todo = %q, want $HOME", got)
		}
	})

	t.Run("no markers roots the directory itself", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "loose")
		mkdir(t, dir)

		if got := findProjectRoot(dir); got != dir {
			t.Errorf("findProjectRoot = %q, want the directory itself %q", got, dir)
		}
	})

	t.Run("the filesystem root is never a project", func(t *testing.T) {
		// A pane that woke up at "/" (mac app GUI launch inheriting launchd's
		// cwd) used to root the backlog there: /.cats-todo/todos.json, a path
		// that only fails at save time, on a read-only volume, with a bare
		// errno. "" — no project — is the answer, whatever markers the root
		// happens to carry.
		if got := findProjectRoot(string(filepath.Separator)); got != "" {
			t.Errorf("findProjectRoot(/) = %q, want \"\" (no project owns the filesystem root)", got)
		}
	})
}

// TestIsFilesystemRoot pins the one directory that can never be a project root,
// and the near-misses that still can.
func TestIsFilesystemRoot(t *testing.T) {
	tests := []struct {
		dir  string
		want bool
	}{
		{"/", true},
		{"/repo", false},
		{"/repo/sub", false},
		{"", false},  // unknown, not the root — callers handle it themselves
		{".", false}, // its own parent too, but a real writable place
	}
	for _, tt := range tests {
		if got := isFilesystemRoot(tt.dir); got != tt.want {
			t.Errorf("isFilesystemRoot(%q) = %v, want %v", tt.dir, got, tt.want)
		}
	}
}

// TestRunContextProjectDir pins the fallback order for the directory that scopes
// project todos: the resolved root, else the raw working directory.
func TestRunContextProjectDir(t *testing.T) {
	tests := []struct {
		name string
		ctx  RunContext
		want string
	}{
		{"root wins when resolved", RunContext{WorkDir: "/repo/sub", ProjectRoot: "/repo"}, "/repo"},
		{"falls back to the workdir", RunContext{WorkDir: "/repo/sub"}, "/repo/sub"},
		{"empty when neither is known", RunContext{}, ""},
		// The filesystem root is a directory we can name but never a project:
		// both the fallback and a (never-produced) root ProjectRoot drop it, so
		// the project store comes out unavailable rather than pointed at
		// /.cats-todo.
		{"empty at the filesystem root", RunContext{WorkDir: "/"}, ""},
		{"empty for a root project root", RunContext{WorkDir: "/", ProjectRoot: "/"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ctx.projectDir(); got != tt.want {
				t.Errorf("projectDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRunContextPaneTitle pins the terminal title the manager sets: the project
// basename, tracking projectDir's own fallback order, and the bare app name when
// no directory resolved.
func TestRunContextPaneTitle(t *testing.T) {
	tests := []struct {
		name string
		ctx  RunContext
		want string
	}{
		{"names the project root", RunContext{WorkDir: "/repo/sub", ProjectRoot: "/repo"}, "todo: repo"},
		{"falls back to the workdir", RunContext{WorkDir: "/repo/sub"}, "todo: sub"},
		{"bare name with no directory", RunContext{}, "todo"},
		{"bare name at the filesystem root", RunContext{WorkDir: "/"}, "todo"},
		// Global-only outranks any directory: the pane shows no project backlog,
		// so no project name belongs in its title.
		{"global-only names the scope, not the directory",
			RunContext{WorkDir: "/repo/sub", Scope: launchGlobalOnly}, "todo: global"},
		// Project-only titles like a normal project launch — the project name
		// IS the scope there.
		{"project-only names the project",
			RunContext{WorkDir: "/repo/sub", ProjectRoot: "/repo", Scope: launchProjectOnly}, "todo: repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ctx.paneTitle(); got != tt.want {
				t.Errorf("paneTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

// mkdir creates dir and its parents, failing the test if it cannot.
func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestIsOwnPane covers both CATS_PANE_ID forms the manager must recognize to
// keep its own pane out of the drop-target picker.
func TestIsOwnPane(t *testing.T) {
	pane := wire.PaneInfo{Pane: 7, Handle: "w1:p3"}

	tests := []struct {
		name    string
		ownPane string
		want    bool
	}{
		{"matches the public handle", "w1:p3", true},
		{"matches the p_<id> fallback", "p_7", true},
		{"different handle", "w1:p4", false},
		{"different fallback id", "p_8", false},
		{"empty own pane never matches", "", false},
		{"malformed fallback never matches", "p_x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOwnPane(RunContext{OwnPane: tt.ownPane}, pane); got != tt.want {
				t.Errorf("isOwnPane(%q, %+v) = %v, want %v", tt.ownPane, pane, got, tt.want)
			}
		})
	}
}

// TestIsDropAgent pins that an editor pane (ced reports an agent label over the
// hook socket so cats can find it) is never offered as a drop target — neither
// as a running pane nor, since the new-session rows are derived from the same
// scan, as "New ced session" — while real coding agents still are.
//
// The plugin_type cases are the current-cats path, where the wire says what a
// pane is. The untyped ced cases are the older-cats path, where only the
// editorAgents fallback stands between ced and the picker.
func TestIsDropAgent(t *testing.T) {
	tests := []struct {
		agent, pluginType string
		want              bool
	}{
		{"claude", "", true},
		{"copilot", "", true},
		{"codex", "", true},
		{"claude", wire.PluginTypeAgent, true}, // a plugin that IS an agent
		{"", "", false},
		{"ced", "", false}, // older cats: no type, the fallback list catches it
		{"CEd", "", false},
		{"ced", wire.PluginTypeEditor, false},
		{"vimx", wire.PluginTypeEditor, false},    // an editor label only cats' config knows
		{"notes", wire.PluginTypeNotesMgr, false}, // a tool reporting over the hook API
		{"x", "dev_server", false},                // a type newer than this build is a tool
	}
	for _, tt := range tests {
		p := wire.PaneInfo{PaneMeta: wire.PaneMeta{Agent: tt.agent, PluginType: tt.pluginType}}
		if got := isDropAgent(p); got != tt.want {
			t.Errorf("isDropAgent(agent=%q, plugin_type=%q) = %v, want %v", tt.agent, tt.pluginType, got, tt.want)
		}
	}
}

// TestPaneWorkspaceID pins the handle-prefix extraction the picker uses to
// group panes by workspace.
func TestPaneWorkspaceID(t *testing.T) {
	if got := paneWorkspaceID(wire.PaneInfo{Handle: "w3:p9"}); got != "w3" {
		t.Errorf("paneWorkspaceID(w3:p9) = %q, want w3", got)
	}
	if got := paneWorkspaceID(wire.PaneInfo{Handle: ""}); got != "" {
		t.Errorf("paneWorkspaceID(no handle) = %q, want empty", got)
	}
}

// TestPaneTopic: a pane's topic is its custom name, else the agent's terminal
// title with the status glyph cut. A title that only names the agent gives
// nothing, since the row's label already does (N-075).
func TestPaneTopic(t *testing.T) {
	for _, c := range []struct {
		name, title, agent, want string
	}{
		{"", "✳ File count in ai_docs/todo", "claude", "File count in ai_docs/todo"},
		{"", "⠐ Fix the flaky drop test", "claude", "Fix the flaky drop test"},
		{"", "✳ Claude Code", "claude", ""},
		{"", "codex", "codex", ""},
		{"", "", "claude", ""},
		{"reviewer", "✳ Something else", "claude", "reviewer"},
		{"", "2026 plan", "claude", "2026 plan"},
	} {
		p := wire.PaneInfo{Name: c.name, PaneMeta: wire.PaneMeta{Title: c.title, Agent: c.agent}}
		if got := paneTopic(p); got != c.want {
			t.Errorf("paneTopic(name %q, title %q) = %q, want %q", c.name, c.title, got, c.want)
		}
	}
}
