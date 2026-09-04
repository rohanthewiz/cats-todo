// context.go — where cats-todo is running from. Unlike the herdr original —
// which relayed a plugin-action context from a server-side process into a
// plugin pane via env blobs — cats-todo runs directly in a shell pane, so the
// context is gathered in-process at startup: the working directory from the
// pane itself, the pane identity from the CATS_PANE_ID env cats injects into
// every pane, and the workspace from a control-socket query.
//
// Adapted from herdr-plus (https://github.com/cloudmanic/herdr-plus),
// Copyright (c) 2026 Cloudmanic Labs, LLC, MIT License. See NOTICE.

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rohanthewiz/cats-todo/internal/integration"
	"github.com/rohanthewiz/cats/wire"
)

// RunContext describes where cats-todo is running: the pane's working directory
// and the project root above it (which scopes project todos and roots any new
// session's tab), the pane's own handle (so the drop-target picker can exclude
// the manager's pane), and the workspace it lives in (so the picker can mark
// same-project sessions).
type RunContext struct {
	WorkDir string
	// ProjectRoot is the directory that owns the project backlog: WorkDir or an
	// ancestor of it, and "" when no directory qualifies — a pane sitting at the
	// filesystem root has no project (see findProjectRoot). Kept separate from
	// WorkDir so the pane's actual location stays available, and so a context
	// built without a root — the tests, and any caller that only knows a
	// directory — still scopes sensibly via projectDir.
	ProjectRoot    string
	OwnPane        string // CATS_PANE_ID handle: "w1:p3", or the "p_<id>" fallback
	WorkspaceID    string // public workspace id ("w1")
	WorkspaceLabel string
	// OwnPaneID is the manager pane's numeric cats id, 0 when unresolved. The
	// handle above is what the picker compares panes against, but the control
	// API addresses panes by number, and a worktree drop has to name the pane it
	// anchors on: worktree.create resolves the repo from the addressed pane's
	// working directory and defaults to the *focused* pane — which, during an
	// unattended scheduled fire, is wherever the user happens to be looking.
	OwnPaneID uint32
	// Scope is the launch's backlog selection (--project / --global). The
	// bare launch shows both backlogs merged; the only-modes exist for the
	// plugins dialog, whose two actions promise a *project* view and a
	// *global* view — a combined list under a "this project" label reads as
	// the project's todos leaking global entries (and vice versa). In an
	// only-mode the other store is simply unavailable, the state the UI
	// already renders (header note, add-form default, no scope toggle).
	// WorkDir is still gathered in every mode: a new-session drop has to root
	// its tab somewhere, and the pane's own directory remains the honest
	// default.
	Scope launchScope
}

// launchScope selects which backlogs a manager launch manages.
type launchScope int

const (
	launchBoth        launchScope = iota // project + global merged (bare `cats-todo`)
	launchProjectOnly                    // --project: just this project's backlog
	launchGlobalOnly                     // --global: just the global backlog
)

// projectDir is the directory that scopes project todos and roots a new
// session's tab: the resolved project root, falling back to the raw working
// directory when no root was resolved. Empty when neither is known — launched
// somewhere we could not even name, where only the global backlog applies —
// and equally empty at the filesystem root, which we *can* name but which is
// never a project (see findProjectRoot). The fallback needs that check of its
// own: a global-only launch skips the root walk entirely, so a WorkDir of "/"
// would otherwise arrive here unfiltered.
func (c RunContext) projectDir() string {
	dir := firstNonEmpty(c.ProjectRoot, c.WorkDir)
	if isFilesystemRoot(dir) {
		return ""
	}
	return dir
}

// paneTitle is the terminal title the manager advertises while it runs, named
// after the project whose backlog it is showing. Cats scans OSC 0/2 off the PTY
// into its pane chrome, so this is what labels the pane in the browser — a
// shell pane otherwise keeps whatever title it last had, which says nothing
// about the backlog now on screen.
//
// The project basename, not the full path: pane chrome already prints the cwd,
// and a title has to stay short to survive tab-bar truncation. Degrades to the
// bare app name when no directory resolved (global-only launch).
func (c RunContext) paneTitle() string {
	// Global-only mode is scoped to no project, so no project name belongs in
	// the title — "global" is the fact that distinguishes this pane from a
	// project-scoped manager sitting in the next tab.
	if c.Scope == launchGlobalOnly {
		return "todo: global"
	}
	if name := baseName(c.projectDir()); name != "" {
		return "todo: " + name
	}
	return "todo"
}

// findProjectRoot resolves the directory that owns the project backlog (see
// walkProjectRoot), except that the filesystem root is never one: "" — no
// project — is the answer there.
//
// A pane can genuinely wake up at "/": a GUI launch of the mac app inherits
// launchd's cwd, and every pane spawned from it used to inherit that in turn.
// Rooting the backlog there produces a path (/.cats-todo/todos.json) that looks
// fine until the first save, which fails on macOS's read-only system volume
// with a bare errno. Refusing the root up front turns that into the state the
// UI already knows how to render — no project, offer the global backlog.
func findProjectRoot(dir string) string {
	if root := walkProjectRoot(dir); !isFilesystemRoot(root) {
		return root
	}
	return ""
}

// isFilesystemRoot reports whether dir is the top of the filesystem ("/"). That
// is the one directory whose parent is itself, which is also how the walks below
// know when to stop. Empty is "unknown", not the root, and is left to callers;
// the absolute-path requirement keeps a relative "." — also its own parent — out
// of it, since a relative directory names a real place we can write to.
func isFilesystemRoot(dir string) bool {
	return dir != "" && filepath.IsAbs(dir) && filepath.Dir(dir) == dir
}

// walkProjectRoot walks up from dir to the directory that owns the project
// backlog: the nearest ancestor with an existing .cats-todo backlog wins, then
// the nearest with a .git directory (the repo root). With neither, dir itself is
// the root — a directory that is not in a repo and has no backlog yet gets its
// own on first save.
//
// The existing-backlog pass runs to completion before the .git pass so an
// established backlog always beats a repo root that merely encloses it — a repo
// with a backlog in a subdirectory keeps using that subdirectory rather than
// silently starting a second one at the root.
func walkProjectRoot(dir string) string {
	for d := dir; ; {
		if fi, err := os.Stat(filepath.Join(d, projectConfigDirName)); err == nil && fi.IsDir() {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return dir
}

// gatherRunContext builds the launch context. The working directory always
// resolves (it is this process's cwd) and the project root is walked up from it
// — empty only when the cwd is the filesystem root — so opening the manager
// from a subdirectory reaches the same backlog `cats-todo add` writes to from
// that subdirectory; the pane handle comes from the
// environment; the workspace resolves via the client — from the pane handle's
// "w1" prefix when present, else the active workspace — and is left empty when
// the control socket is unavailable (client nil). A partial context still runs:
// project todos scope to the resolved directory regardless.
//
// A global-only launch skips the project-root walk entirely: no ProjectRoot
// means projectDir falls back to the raw WorkDir (drops still root there),
// while loadStores sees the mode and withholds the project store.
func gatherRunContext(client *catsClient, scope launchScope) RunContext {
	ctx := RunContext{OwnPane: os.Getenv(integration.CatsPaneIDEnvVar), Scope: scope}
	if wd, err := os.Getwd(); err == nil {
		ctx.WorkDir = wd
		if scope != launchGlobalOnly {
			ctx.ProjectRoot = findProjectRoot(wd)
		}
	}

	if ws, _, ok := strings.Cut(ctx.OwnPane, ":"); ok {
		ctx.WorkspaceID = ws
	}
	if client == nil {
		return ctx
	}
	// No handle-derived workspace (launched outside a cats pane, or the p_<id>
	// fallback form) — fall back to the active workspace: the user is driving
	// this TUI, so their workspace is almost certainly the active one.
	if ctx.WorkspaceID == "" {
		if info, err := client.sessionInfo(); err == nil {
			ctx.WorkspaceID = info.ActiveWorkspace
		}
	}
	if labels, err := client.workspaceLabels(); err == nil {
		ctx.WorkspaceLabel = labels[ctx.WorkspaceID]
	}
	ctx.OwnPaneID = resolveOwnPaneID(client, ctx)
	return ctx
}

// resolveOwnPaneID turns the CATS_PANE_ID handle into the numeric pane id the
// control API addresses (see RunContext.OwnPaneID), or 0 when it cannot.
//
// The "p_<id>" fallback form embeds the number already, so it costs nothing.
// The public "w1:p3" handle does not, and only pane.list maps one to the other
// — hence the round trip, done once at startup rather than per drop, since a
// pane's id is fixed for its lifetime and this is our own pane.
func resolveOwnPaneID(client *catsClient, ctx RunContext) uint32 {
	if raw, ok := strings.CutPrefix(ctx.OwnPane, "p_"); ok {
		if n, err := strconv.ParseUint(raw, 10, 32); err == nil {
			return uint32(n)
		}
	}
	if client == nil || ctx.OwnPane == "" {
		return 0
	}
	panes, err := client.paneList()
	if err != nil {
		return 0
	}
	for _, p := range panes {
		if isOwnPane(ctx, p) {
			return p.Pane
		}
	}
	return 0
}

// isOwnPane reports whether p is the pane this manager runs in, matching both
// CATS_PANE_ID forms: the public "w1:p3" handle, and the "p_<raw>" fallback
// that embeds the internal pane id directly.
func isOwnPane(ctx RunContext, p wire.PaneInfo) bool {
	if ctx.OwnPane == "" {
		return false
	}
	if p.Handle != "" && p.Handle == ctx.OwnPane {
		return true
	}
	if raw, ok := strings.CutPrefix(ctx.OwnPane, "p_"); ok {
		if n, err := strconv.ParseUint(raw, 10, 32); err == nil {
			return uint32(n) == p.Pane
		}
	}
	return false
}

// paneWorkspaceID extracts the public workspace id from a pane's "w1:p3"
// handle, or "" when the handle is absent/unparseable.
func paneWorkspaceID(p wire.PaneInfo) string {
	if ws, _, ok := strings.Cut(p.Handle, ":"); ok {
		return ws
	}
	return ""
}
