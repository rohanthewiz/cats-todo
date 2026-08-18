# Session: export a prompt to another project

Session ID: `df93e774-5e71-4a58-a762-17b37345b011`
Date: 2026-08-18

The ask, in one line:

> "I would like the ability to export a note to another project. Possibly use
> the recently included cdx directory changer code here, or comm with the Cats
> mux (~/projs/go/cats) for opened workspaces"

Both halves of the "possibly" were taken, plus two sources the ask did not
name. New files: `export.go`, `export_test.go`. Touched: `ui.go`, `filepick.go`,
`client.go`, `internal/app/command_vocab.go`, `ui_test.go`, `README.md`.

## What was built

`ctrl+o` on the list (or the new **➦ Export** chip on the action bar, or
`ctrl+o` on the prompt view) opens an **Export to…** picker on the highlighted
prompt. `enter` **copies** it into the chosen project's `.cats-todo/todos.json`;
`shift+enter`/`alt+enter` **moves** it — the drop picker's "modifier does the
more committing thing" split, kept on purpose. A click on a row copies. The
transfer is synchronous (a JSON write and a few small file copies), lands back
on the list, and the status says `copied → cats` / `moved → the global backlog`.

The picker's rows, most likely first:

1. **Open cats workspaces.** The key finding of the session: cats'
   `workspace.list` carries **no directory** — `WorkspaceInfo` is
   id/name/active/tabs/locked/host, and the workspace's `IdentityCwd` exists in
   the model (`internal/workspace/workspace.go`) but is never put on the wire.
   `pane.list` does carry every pane's live `cwd` and its `w1:p3` handle, so
   `catsWorkspaceDirs` joins the two: per workspace (server order), each
   distinct project root among its panes' cwds becomes a row; the first carries
   the workspace name, later ones append the project's own name (`lymbic-gws`,
   `lymbic-gws · lymbic`). Verified against the live cats: 14 workspaces
   resolved, own project excluded.
2. **The other backlog** of this manager (global for a project prompt, this
   project for a global one). Not "another project", but the same operation,
   and previously the only way to move a prompt between the two scopes was to
   retype it.
3. **Recent projects** — cdx's frecency state file (`~/Library/Application
   Support/cdx/state.json`), read directly the way cats' `internal/pathpick`
   does (ranking ported verbatim: count × recency weight 4/2/0.5/0.25). Only
   directories that already keep a backlog, capped at 8, tagged `recent` via
   `listItem.tag`. The state path is a package var so tests can silence it.
4. **Browse for a folder…** — `filepick.go` in a new folders-only flavour
   (`purpose: filesForExport`, `dirsOnly`), starting among this project's
   siblings (`exportBrowseRoot`), leading with a `./ this folder` row (ref −1,
   which `highlighted` already answers false for, so "nothing highlighted =
   the listed folder" needed no new state). esc returns to the export picker;
   `enter`/modifier+enter copy/move to the highlighted folder.

`buildExportTargets` is pure (takes `exportSources{workspaces, recents}` and the
two stores); `gatherExportSources` does the socket and disk reads.

## Transfer semantics (`exportTodo`)

Travels: title, prompt, attachments (copied into the destination's own
`images/<id>/` via `attachImages` — a backlog owns its files), session options
(cloned, not aliased), open/frozen/done state, `Created`. **Does not travel: the
schedule** — it names a pane and a launch directory of the source project; the
returned note puts "its schedule was not carried over" on the status line.

Copy takes a fresh id; move keeps its own (a copy is a new todo, a move is the
same one relocated — bookkeeping, nothing looks ids up across backlogs). Order
of operations: images first (all-or-nothing), then `dst.add`, then — for a
move — `src.delete`. Any failure before the delete leaves the source untouched
and removes what was written into the destination; a failure *at* the delete is
reported as "copied, but could not remove it from this backlog".

A destination directory resolves to its backlog exactly as a launch directory
does (`findProjectRoot`: `.cats-todo` ancestor → git root → itself), so a
subdirectory reaches the project's one backlog and a project with none gets
one, as `cats-todo add` there would. `destinationStore` refuses a directory that
does not exist and the filesystem root. Exporting into the backlog the prompt is
already in is refused by directory comparison, not duplicated. `syncStores`
reloads whichever of the manager's own stores shares a file with the
destination store object (the browser can point at this project; a global
prompt's "This project" row is `m.project` itself), so the list shows the disk.

## Small decisions worth keeping

- `ctrl+o` was free on the list (it is Images in the form; chords already
  differ per stage). Icon `➦` is a one-cell dingbat like the others.
- Fifth chip pushed the labels-only tier from ~40 to 50 columns;
  `TestActionBarRender`'s narrow width went 45 → 55. `actionExport` sits before
  `actionDelete` so tests that treat Delete as the ring's last stop still hold.
- Vendored `WorkspaceInfo` gained the `Host` field cats now sends (drift found
  while reading the cats side); `client.workspaceList` keeps server order and
  `workspaceLabels` is built on it.
- Test fixture note: `newModelInTemp` puts the project file at
  `<tmp>/project/todos.json`, not under `.cats-todo/`; the export tests use
  their own `exportModel` with the real layout, because the export paths
  resolve directories back to backlogs and the loose layout let a
  refuse-own-project test pass wrongly.

## Left on the table

- cats' `workspace.list` could carry the identity cwd in ~2 lines
  (`internal/app/query.go` from `ws.IdentityCwd`); the pane join covers it
  today, so the cats repo was not touched.
- No "recent destinations" memory of the picker's own; cdx's habits stand in.
- Export is TUI-only; no `cats-todo export` CLI verb.
