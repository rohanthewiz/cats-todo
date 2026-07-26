# Session: scoped launch modes (--project / --global) for the plugin actions

Session ID: `7cbbef7b-28e3-4b06-9185-1e37c6ac7010`
Date: 2026-07-26
(First session doc in this repo — earlier cats-todo history is documented in
the cats repo's `ai_docs/claude_sessions/`.)

## Arc

Three commits, driven from the cats plugins dialog:

1. `9d9f806` (v0.2.0) — `--global` launch mode + a second manifest action.
   The dialog's "run" button becomes a picker when a plugin has >1 action, so
   this added "Cats Todo — global only" next to the project launch.
2. `296a8ea` — version bump to 0.2.2 (user request).
3. `7cfee2b` (v0.2.3) — user saw `gowex + global` in the header of a
   "this project" launch: an empty project backlog showing only global rows
   under a project label. The merged view was the manager's historical
   default, but under that picker label it reads as leakage. Each action is
   now pinned to exactly one backlog.

## Design (v0.2.3)

- `launchScope` enum: `launchBoth` (bare `cats-todo`, merged view — the
  shell-launch default, unchanged), `launchProjectOnly` (`-p|--project`),
  `launchGlobalOnly` (`-g|--global`). Carried as `RunContext.Scope`;
  `gatherRunContext(client, scope)`.
- Global-only skips the project-root walk (no ProjectRoot; WorkDir stays so
  new-session drops root at the pane's directory). Project-only keeps the
  walk — it narrows what shows, not where it looks.
- `loadStores` withholds the far store in an only-mode via the existing
  unavailable-store mechanism (empty path = loads/saves to nothing). A
  project-only launch doesn't even resolve the global path.
- UI fallout, all riding existing unavailable-store handling:
  - header note: `<project> + global` / `<project> only` / `global only`
  - pane title: `todo: <project>` / `todo: global`
  - add-form default scope follows availability; the ctrl+g scope toggle and
    its footer hint now require BOTH stores so an only-mode can't save into
    an unavailable store.
- Manifest actions: `todo` = "Cats Todo — this project" → `--project`
  (first action = default for bare `catctl plugin run`, so that is now
  project-only); `todo-global` = "Cats Todo — global only" → `--global`.

## Tests

`context_test.go`: global-only skips the walk / project-only keeps it; pane
titles for both modes. `store_test.go`: `loadStores` withholds the correct
store in each only-mode. Everything builds/vets/tests clean.

## Standing notes

- The wire-contract copies in `internal/{app,ctlproto,integration}` must stay
  in lockstep with cats (unchanged this session — no wire changes).
- The plugin manager row for this plugin was also fixed on the cats side this
  session (status column + modal width; cats `225a088`).
