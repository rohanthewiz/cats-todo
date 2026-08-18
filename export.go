// export.go — sending a prompt to another project's backlog.
//
// A backlog is scoped to one project (or to the user, for the global one), and
// a prompt does not always get captured in the right one: `cats-todo add` from
// a shell in project A while thinking about project B, or a todo written for
// one repo that turns out to be about the sibling. Export is the way across —
// the todo is copied (or moved) into the destination project's own
// .cats-todo/todos.json, exactly as if it had been added there.
//
// This file holds the destination list (which projects can be offered), the
// transfer itself, and — under "The stage" at the bottom — the picker screen
// that shows the list. The directory browser the picker can fall through to is
// the file picker (filepick.go) in a folders-only flavour; ui.go only routes
// keys, clicks and frames here the way it does for every other stage.
//
// Where the destinations come from, in the order the picker shows them:
//
//	open cats workspaces  — every workspace the running cats has open, each
//	                        with the project(s) its panes are working in, via
//	                        the control socket (workspace.list joined with
//	                        pane.list — see catsWorkspaceDirs). This is the
//	                        quick path: the projects the user is working on
//	                        right now are the ones a stray prompt is most
//	                        likely meant for.
//	the other backlog     — this manager's own other scope (global when the
//	                        todo is a project one, this project when it is
//	                        global). Not another *project*, but the same
//	                        operation, and otherwise the only way to move a
//	                        prompt between the two scopes is to retype it.
//	recent projects       — directories cdx (the user's cd picker) has seen
//	                        them visit lately that already keep a backlog. cdx's
//	                        state file is read directly, as cats itself does
//	                        for its path picker; no cdx means no block.
//	browse for a folder   — any directory at all, picked in the file browser
//	                        (filepick.go, folders only). The fallback for a
//	                        project none of the above names, and the whole
//	                        list when there is neither a socket nor a cdx.
//
// A destination directory becomes a backlog the same way the manager's own
// launch directory does — the nearest ancestor with a .cats-todo, else the git
// root, else the directory itself (see findProjectRoot) — so exporting to a
// subdirectory of a project reaches that project's one backlog, and exporting
// to a project that has none yet creates one, as `cats-todo add` there would.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// exportKind is the three shapes a row in the export picker can take (see the
// file comment for what each means).
type exportKind int

const (
	exportToDir   exportKind = iota // a directory: an open workspace's, or one browsed to
	exportToStore                   // this manager's other backlog (project <-> global)
	exportBrowse                    // open the folder browser to name a directory
)

// exportTarget is one selectable destination in the export picker.
type exportTarget struct {
	kind exportKind
	// dir is the destination directory for exportToDir: the project root, for
	// a row the picker listed (buildExportTargets resolves it so the row can
	// describe the backlog there), or the raw folder browsed to. Either way
	// destinationStore resolves it again at choose time — the same answer for
	// a root, and the project's backlog for a subdirectory of one.
	dir string
	// scope is the destination scope for exportToStore.
	scope scope
	label string
	desc  string
	// tag is the row's quiet marker after the name (see listItem.tag): "recent"
	// on the cdx rows, so the name stays the project's own and the filter does
	// not match every one of them on "rec".
	tag string
}

// exportWorkspace is what the picker needs to know about one open cats
// workspace: its display name and a directory it is working in. It is a
// projection of app.WorkspaceInfo plus pane.list rather than those types
// themselves, so the picker's tests can feed workspaces in without a socket.
type exportWorkspace struct {
	name string
	dir  string
}

// exportSources is everything buildExportTargets draws on besides the manager's
// own stores: the open workspaces and the recently visited directories. Both
// are gathered by the caller (gatherExportSources) so the assembly stays pure
// — no socket, no home directory — and testable.
type exportSources struct {
	workspaces []exportWorkspace
	recents    []string
}

// buildExportTargets assembles the destinations for exporting a todo that
// lives in the from scope, in the order the picker shows them — see the file
// comment.
//
// Every directory row is keyed by the project root it resolves to, and each
// root appears once: two workspaces rooted in the same project (a repo and one
// of its subdirectories, say) name one backlog, and a recent directory that is
// an open workspace's is already on the list. The backlog the todo is already
// in is left off — there is nowhere to go there — and so is this manager's
// project when it is offered as the other-backlog row, which says what it is.
// A global-only launch has no project store, so the project the manager sits
// in is then just another workspace row, which is what lets a global prompt
// reach it. Workspaces without a directory are skipped; there is nothing to
// export to.
func buildExportTargets(from scope, src exportSources, project, global *store) []exportTarget {
	var targets []exportTarget
	seen := map[string]bool{}
	if project != nil && project.available() {
		// Whichever role the project store plays — the source (a project
		// todo), or the other-backlog row (a global one) — its root is spoken
		// for, and a workspace row naming it would be a second door to the
		// same place.
		seen[backlogRoot(project)] = true
	}

	for _, ws := range src.workspaces {
		root := findProjectRoot(ws.dir)
		if ws.dir == "" || root == "" || seen[root] {
			continue
		}
		seen[root] = true
		targets = append(targets, exportTarget{
			kind:  exportToDir,
			dir:   root,
			label: firstNonEmpty(ws.name, baseName(root)),
			desc:  describeBacklog(root),
		})
	}

	// The other backlog of this manager. Only when it is there to receive —
	// a project-only or global-only launch has just the one store, and a
	// launch outside any project has no project store to offer.
	if from == scopeProject && global != nil && global.available() {
		targets = append(targets, exportTarget{
			kind:  exportToStore,
			scope: scopeGlobal,
			label: "Global backlog",
			desc:  backlogCountNote(global) + " · " + shortenHome(global.path),
		})
	}
	if from == scopeGlobal && project != nil && project.available() {
		targets = append(targets, exportTarget{
			kind:  exportToStore,
			scope: scopeProject,
			label: "This project — " + firstNonEmpty(baseName(backlogRoot(project)), "project backlog"),
			desc:  backlogCountNote(project) + " · " + shortenHome(project.path),
		})
	}

	// Recent projects: only the ones that already keep a backlog. cdx remembers
	// every directory the user has ever cd'd into, and most of those are not
	// projects anyone would export a prompt to; a .cats-todo on disk is the one
	// sign that says "this place takes prompts". Capped so the block stays a
	// short list of habits above the browse row rather than a page of history —
	// the fuzzy filter reaches everything either way.
	added := 0
	for _, dir := range src.recents {
		if added == recentExportLimit {
			break
		}
		root := findProjectRoot(dir)
		if root == "" || seen[root] || !hasBacklog(root) {
			continue
		}
		seen[root] = true
		added++
		targets = append(targets, exportTarget{
			kind:  exportToDir,
			dir:   root,
			label: baseName(root),
			tag:   "recent",
			desc:  describeBacklog(root),
		})
	}

	targets = append(targets, exportTarget{
		kind:  exportBrowse,
		label: "Browse for a folder…",
		desc:  "pick any directory; its project's backlog is created if it has none",
	})
	return targets
}

// recentExportLimit caps the recent-projects block of the picker.
const recentExportLimit = 8

// backlogRoot is the project directory a project store belongs to — the parent
// of the .cats-todo directory its file sits in (see projectTodosPath). "" for
// an unavailable store.
func backlogRoot(s *store) string {
	if s == nil || s.path == "" {
		return ""
	}
	return filepath.Clean(filepath.Dir(filepath.Dir(s.path)))
}

// hasBacklog reports whether root already keeps a project backlog on disk.
func hasBacklog(root string) bool {
	_, err := os.Stat(projectTodosPath(root))
	return err == nil
}

// gatherExportSources collects what the picker offers beyond the manager's own
// two stores: the open cats workspaces with the directories they are working
// in, and cdx's recently visited directories. Either half may come back empty
// — no socket, no cdx — and the picker is simply shorter for it.
func gatherExportSources(client *catsClient) exportSources {
	var src exportSources
	if client != nil {
		src.workspaces = catsWorkspaceDirs(client)
	}
	src.recents = cdxRecents(recentExportScan)
	return src
}

// recentExportScan is how many of cdx's best-ranked directories are examined
// for backlogs. Wider than the block that results from it, since most of a
// shell's history is not a project with a backlog.
const recentExportScan = 60

// catsWorkspaceDirs asks the running cats which workspaces are open and where
// each is working. workspace.list gives the names but no directory — a
// workspace's identity cwd is not on the wire — so the directories come from
// pane.list: every pane reports its live cwd and carries its workspace's id in
// its "w1:p3" handle. The two are joined here, in the server's own order for
// both (workspaces as the sidebar lists them, panes as they sit in their tabs).
//
// A workspace can be working in more than one project at once — a shell cd'd
// into a sibling repo, say — and each distinct project root among its panes
// becomes its own entry, so a prompt can be sent to the sibling too. The first
// entry for a workspace carries the workspace's name; the ones after it add the
// project's own name, since the workspace's name alone would say the wrong
// place. Panes with no reported cwd are skipped, and a workspace whose panes
// report none at all contributes nothing.
func catsWorkspaceDirs(client *catsClient) []exportWorkspace {
	wss, err := client.workspaceList()
	if err != nil {
		return nil
	}
	panes, err := client.paneList()
	if err != nil {
		return nil
	}
	var out []exportWorkspace
	for _, ws := range wss {
		roots := map[string]bool{}
		for _, p := range panes {
			if p.Cwd == "" || paneWorkspaceID(p) != ws.ID {
				continue
			}
			root := findProjectRoot(p.Cwd)
			if root == "" || roots[root] {
				continue
			}
			name := ws.Name
			if len(roots) > 0 {
				name = ws.Name + " · " + baseName(root)
			}
			roots[root] = true
			out = append(out, exportWorkspace{name: name, dir: root})
		}
	}
	return out
}

// cdxRecents returns up to limit of the directories cdx (the user's `cd`
// picker) remembers, best-first by its own frecency ranking, skipping any that
// no longer exist. Ported from cats' internal/pathpick, which reads the same
// file for the same reason: cdx's chpwd hook already records every directory
// change the user makes, and reading its state is deliberately the whole
// integration — no cdx process, no protocol. No cdx (never installed, never
// used, a corrupt file) means nil, not an error.
func cdxRecents(limit int) []string {
	path, err := cdxStateFile()
	if err != nil {
		return nil
	}
	return cdxRecentsFrom(path, time.Now().Unix(), limit)
}

// cdxStateFile locates cdx's state file: the user config dir cdx itself
// writes to (~/Library/Application Support/cdx/state.json on macOS,
// ~/.config/cdx/state.json on Linux). A variable so the tests can point it at
// a fixture — or at nothing — instead of at whatever the machine running them
// happens to remember.
var cdxStateFile = func() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "cdx", "state.json"), nil
}

// cdxEntry is one remembered directory in cdx's state file. Only the fields the
// ranking needs are decoded; cdx is free to add more.
type cdxEntry struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
	Last  int64  `json:"last"` // unix seconds of the last visit
}

// cdxRecentsFrom is cdxRecents against an explicit state file and clock, for
// tests.
func cdxRecentsFrom(statePath string, now int64, limit int) []string {
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil
	}
	var st struct {
		Entries []cdxEntry `json:"entries"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return nil
	}
	sort.SliceStable(st.Entries, func(i, j int) bool {
		return cdxFrecency(st.Entries[i], now) > cdxFrecency(st.Entries[j], now)
	})
	out := make([]string, 0, min(limit, len(st.Entries)))
	for _, e := range st.Entries {
		if len(out) == limit {
			break
		}
		if e.Path == "" || !filepath.IsAbs(e.Path) || !isDir(e.Path) {
			continue // stale: moved or deleted since cdx saw it
		}
		out = append(out, e.Path)
	}
	return out
}

// cdxFrecency scores a remembered directory by visit count weighted by recency
// — cdx's ranking (zoxide's, in turn), reproduced so the picker offers the same
// order the user sees in cdx itself.
func cdxFrecency(e cdxEntry, now int64) float64 {
	age := now - e.Last
	w := 0.25
	switch {
	case age < 3600: // within the hour
		w = 4
	case age < 86400: // within the day
		w = 2
	case age < 7*86400: // within the week
		w = 0.5
	}
	return float64(e.Count) * w
}

// describeBacklog is a row's description for a destination directory: the
// project root's path (home shortened), and the state of the backlog there —
// how many open prompts it holds, or that it does not exist yet. The count is
// read fresh off disk for the row, since the picker is built per open and a
// backlog on disk is the only truth there is.
func describeBacklog(root string) string {
	s := &store{scope: scopeProject, path: projectTodosPath(root)}
	if _, err := os.Stat(s.path); err != nil {
		return "no backlog yet — will be created · " + shortenHome(root)
	}
	if err := s.load(); err != nil {
		return "backlog unreadable: " + err.Error() + " · " + shortenHome(root)
	}
	return backlogCountNote(s) + " · " + shortenHome(root)
}

// backlogCountNote is "N open" for a loaded store, the number a user weighs
// when deciding whether the destination is the one they mean.
func backlogCountNote(s *store) string {
	n := 0
	for _, t := range s.todos {
		if !t.closed() {
			n++
		}
	}
	if n == 1 {
		return "1 open"
	}
	return fmt.Sprintf("%d open", n)
}

// sameDir reports whether two directory paths name the same place, allowing
// for a trailing slash and for the empty "unknown" (which is never the same as
// anything). Symlinks are not chased: the roots compared here all come from
// the same resolver, so a mismatch that survives Clean is a real difference.
func sameDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// destinationStore opens the project backlog a directory stands for: the
// backlog of the project dir belongs to (see findProjectRoot), loaded, ready to
// receive. The filesystem root is refused for the reason findProjectRoot gives
// — nothing can be a project there — and a directory that does not exist is
// refused rather than created: a typo in the browser must not conjure a
// project out of nothing.
func destinationStore(dir string) (*store, error) {
	if dir == "" {
		return nil, errors.New("no destination directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if !isDir(abs) {
		return nil, fmt.Errorf("%s is not a directory", shortenHome(dir))
	}
	root := findProjectRoot(abs)
	if root == "" {
		return nil, errors.New("the filesystem root cannot hold a backlog")
	}
	s := &store{scope: scopeProject, path: projectTodosPath(root)}
	if err := s.load(); err != nil {
		return nil, fmt.Errorf("read %s: %w", shortenHome(s.path), err)
	}
	return s, nil
}

// exportTodo copies td from src into dst — or moves it, when move is set — and
// returns the todo as it now stands in dst.
//
// What travels: the title, the prompt, the attachments (copied into dst's own
// images/ directory, since a backlog owns its files — see images.go), the
// session options, and the open/frozen/done state, so the destination gets the
// prompt as it was and not a reset of it. What does not: the schedule. A
// schedule names a pane and a launch directory of the *source* project, and a
// prompt that fired itself into the wrong project's session would be worse
// than one that quietly needs re-scheduling; the caller is told (the returned
// note) so the row's clock does not just vanish without a word.
//
// A copy takes a fresh id and a move keeps its own. A copy is a new todo — two
// backlogs, two independent lives from here on — while a move is the same one
// in a new place. Nothing today looks a todo up across backlogs, so this is
// bookkeeping rather than a rule anything depends on, but ids that tell the
// two cases apart cost nothing.
//
// Failure ordering, because a transfer touches two backlogs and some files:
// the images are copied first (all-or-nothing, see attachImages), then the
// todo is added to dst, and only then — for a move — is it deleted from src.
// A failure at any step before the delete leaves src exactly as it was and
// removes whatever this call wrote into dst. A failure *at* the delete is the
// one that cannot be undone cleanly (dst already has the todo, and dst's save
// went through), so it is reported as what it is: copied, not moved.
func exportTodo(src, dst *store, td Todo, move bool) (out Todo, note string, err error) {
	if dst == nil || !dst.available() {
		return Todo{}, "", errors.New("no backlog to export to")
	}
	if src != nil && src.available() && sameDir(filepath.Dir(src.path), filepath.Dir(dst.path)) {
		return Todo{}, "", errors.New("that is the backlog the prompt is already in")
	}

	out = td
	if !move {
		out.ID = newID()
	}
	if out.Schedule != nil {
		note = "its schedule was not carried over"
	}
	out.Schedule = nil
	// The session record is shared by pointer on the Todo; the copy in dst must
	// not alias the one src still holds (compare SessionOpts.clone's reasoning).
	if td.Session != nil {
		c := td.Session.clone()
		out.Session = &c
	}

	// Attachments that have gone missing on disk are dropped rather than carried
	// as dangling names — imagePaths already leaves them out — which is the
	// same view a drop delivers.
	var srcPaths []string
	if src != nil {
		srcPaths = src.imagePaths(td)
	}
	rels, err := dst.attachImages(out.ID, srcPaths)
	if err != nil {
		return Todo{}, "", fmt.Errorf("copy attachments: %w", err)
	}
	out.Images = rels

	if err := dst.add(out); err != nil {
		dst.removeImageFiles(out.ID, rels)
		return Todo{}, "", fmt.Errorf("write %s: %w", shortenHome(dst.path), err)
	}
	if !move || src == nil {
		return out, note, nil
	}
	if err := src.delete(td.ID); err != nil {
		return out, note, fmt.Errorf("copied, but could not remove it from this backlog: %w", err)
	}
	return out, note, nil
}

// exportDesc is the short name of a destination for the status line: the
// workspace or project name for a directory, the scope's name for a store.
func exportDesc(t exportTarget) string {
	switch t.kind {
	case exportToStore:
		if t.scope == scopeGlobal {
			return "the global backlog"
		}
		return "this project's backlog"
	case exportToDir:
		if root := findProjectRoot(t.dir); root != "" {
			return firstNonEmpty(baseName(root), shortenHome(root))
		}
		return shortenHome(t.dir)
	}
	return t.label
}

// --- The stage -----------------------------------------------------------------

// beginExport opens the export picker for the highlighted todo (ctrl+o, or the
// bar's ➦ Export). Nothing highlighted is a quiet no-op, the same as the other
// begin* helpers; runAction says so on the button's behalf.
func (m model) beginExport() (tea.Model, tea.Cmd) {
	ref, ok := m.selectedRef()
	if !ok {
		return m, nil
	}
	return m.startExport(ref)
}

// startExport opens the export picker for a specific todo — from the list's
// highlight or from the view stage. The destinations are gathered here, on
// open, rather than kept: which workspaces are open and what their backlogs
// hold are facts of the moment, and a picker built once at launch would name
// projects closed since. Unlike a drop, this works without a socket — the
// picker is shorter (no workspace rows), not gone.
func (m model) startExport(ref todoRef) (tea.Model, tea.Cmd) {
	if _, ok := m.resolve(ref); !ok {
		m.setStatus("could not find that prompt", true)
		m.backToList()
		return m, nil
	}
	m.exportRef = ref
	m.exportTargets = buildExportTargets(ref.scope, gatherExportSources(m.client), m.project, m.global)
	items := make([]listItem, len(m.exportTargets))
	for i, t := range m.exportTargets {
		items[i] = listItem{name: t.label, desc: t.desc, tag: t.tag, selectable: true, ref: i}
	}
	m.exportList = newFuzzyList("Filter projects…", items)
	m.stage = stageExport
	return m, textinput.Blink
}

// updateExport is the picker's key loop, the drop-target picker's shape: esc
// back, arrows move, enter copies, and the modifier+enter chord — the more
// committing thing enter could do, here as in the drop picker — moves.
func (m model) updateExport(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.backToList()
		return m, nil
	case "up", "ctrl+p":
		m.exportList.moveUp()
		return m, nil
	case "down", "ctrl+n":
		m.exportList.moveDown()
		return m, nil
	case "enter":
		return m.chooseExport(false)
	case "shift+enter", "alt+enter":
		return m.chooseExport(true)
	}
	return m, m.exportList.editQuery(msg)
}

// chooseExport acts on the highlighted row: the browse row opens the folder
// browser (the copy/move choice is made there, on the folder), and any other
// row is a destination to export to now.
func (m model) chooseExport(move bool) (tea.Model, tea.Cmd) {
	idx := m.exportList.selectedIndex()
	if idx < 0 || idx >= len(m.exportTargets) {
		return m, nil
	}
	t := m.exportTargets[idx]
	if t.kind == exportBrowse {
		return m.beginExportBrowse()
	}
	return m.performExport(t, move)
}

// chooseExportFolder is the browser's choice: the highlighted folder, or — on
// the "./" row, which highlighted answers false for — the folder being listed.
func (m model) chooseExportFolder(move bool) (tea.Model, tea.Cmd) {
	dir := m.files.dir
	if e, abs, ok := m.files.highlighted(); ok && e.dir {
		dir = abs
	}
	return m.performExport(exportTarget{kind: exportToDir, dir: dir}, move)
}

// performExport carries the todo across (see exportTodo) and lands back on the
// list with the outcome in the status line. It runs on the UI thread, unlike a
// drop: the work is a JSON write and at most a few small file copies, and a
// picker that returned before the transfer was known to have happened would
// have nothing honest to say in its status.
//
// A destination that is also one of this manager's own stores may have been
// written through a store object of its own (the browser can name this
// project's directory, or a global todo's "This project" row shares a file
// with m.project); those are reloaded so the list shows what the disk now
// holds rather than what this process last wrote.
func (m model) performExport(t exportTarget, move bool) (tea.Model, tea.Cmd) {
	td, ok := m.resolve(m.exportRef)
	if !ok {
		m.setStatus("could not find that prompt", true)
		m.backToList()
		return m, nil
	}
	src := m.storeFor(m.exportRef.scope)
	var dst *store
	var err error
	switch t.kind {
	case exportToStore:
		dst = m.storeFor(t.scope)
	case exportToDir:
		dst, err = destinationStore(t.dir)
	default:
		err = errors.New("nothing to export to")
	}
	verb := "copied"
	if move {
		verb = "moved"
	}
	var note string
	if err == nil {
		_, note, err = exportTodo(src, dst, td, move)
	}
	m.syncStores(dst)
	m.rebuildList()
	m.backToList()
	if err != nil {
		m.setStatus("export failed: "+err.Error(), true)
		return m, nil
	}
	status := verb + " → " + exportDesc(t)
	if note != "" {
		status += " · " + note
	}
	m.setStatus(status, false)
	return m, nil
}

// syncStores reloads whichever of the manager's stores shares a file with dst
// without being it — the case performExport describes. dst itself was just
// written by this process and is current; nil (a failed destinationStore) is
// nothing to sync against.
func (m *model) syncStores(dst *store) {
	if dst == nil {
		return
	}
	for _, s := range []*store{m.project, m.global} {
		if s != nil && s != dst && s.available() && s.path == dst.path {
			_ = s.load()
		}
	}
}

// clickExport is the pointer on the picker: a click on a row chooses it as a
// copy — the safer of the two things a row can do, the way a click in the drop
// picker pastes rather than runs. exportRowsRow is the offset from the top of
// the pane to the first row line (see its comment).
func (m model) clickExport(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	i, ok := m.exportList.rowAtLine(msg.Y - exportRowsRow)
	if !ok || !m.exportList.focusRow(i) {
		return m, nil
	}
	return m.chooseExport(false)
}

// exportRowsRow is the first line the export picker's rows are drawn on: the
// heading (0), a blank (1), the filter line (2), a blank (3), then the rows —
// the drop picker's geometry exactly (targetRowsRow), since viewExport draws
// the same chrome. TestExportRowsMatchWhatIsDrawn pins it to a real frame.
const exportRowsRow = 4

// viewExport draws the picker: a heading naming the prompt, the list with its
// filter box, and a footer of keys.
func (m model) viewExport() string {
	var b strings.Builder
	td, _ := m.resolve(m.exportRef)
	title := firstNonEmpty(td.Title, firstLine(td.Prompt, 50))
	b.WriteString(titleStyle.Render("Export to…"))
	b.WriteString("  ")
	b.WriteString(descStyle.Render(truncate(title, 60)))
	b.WriteString("\n\n")
	b.WriteString(m.exportList.view("nothing matches — clear the filter, or browse for a folder", "", m.width))
	b.WriteString("\n")
	b.WriteString(footerStyle.Render(m.fitFooter([]string{
		"enter copy", m.modEnter() + " move", "esc back",
	})))
	return b.String()
}

// viewExportBrowse draws the folder browser: viewFiles's chrome with the
// export picker's words — what a choice does is the one thing the two screens
// have to say differently.
func (m model) viewExportBrowse() string {
	var b strings.Builder
	title := "Export to folder"
	b.WriteString(titleStyle.Render(title))
	b.WriteString("  ")
	room := 0
	if m.width > 0 {
		room = m.width - lipgloss.Width(titleStyle.Render(title)) - 4
	}
	b.WriteString(descStyle.Render(m.files.headingDir(room)))
	b.WriteString("\n\n")
	b.WriteString(m.files.list.view(m.files.filesEmptyMessage(), "", m.width))
	b.WriteString("\n")
	b.WriteString(footerStyle.Render(m.fitFooter([]string{
		"enter copy here", m.modEnter() + " move here", "tab/→ or / open folder", "backspace up", "esc back",
		"~/ and ../ paths", ". shows hidden",
	})))
	return b.String()
}
