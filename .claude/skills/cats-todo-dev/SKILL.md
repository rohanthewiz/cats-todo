---
name: cats-todo-dev
description: "Working on the cats-todo codebase itself (this repo): architecture and file map, the contracts that must stay in lockstep with cats (control-socket vocabulary, palette, ready probes), the todos.json compatibility rule, key-chord ownership, how tests drive the Bubble Tea model, the README-as-spec and session-doc habits, commit style, and the two-file version bump for a release. Use for any feature, fix, refactor, or release in cats-todo."
---

# cats-todo — developing the tool

cats-todo is a single-`main`-package Go TUI (Bubble Tea **v2**: `charm.land/bubbletea/v2`,
`charm.land/bubbles/v2`, `charm.land/lipgloss/v2` — not the old `github.com/charmbracelet/*`
v1 imports). It is a cats plugin; ported from herdr-todo. For *using* the tool, see the
global `cats-todo-prompt-backlog` skill; this one is for changing it.

```bash
go build -o bin/cats-todo . && ./bin/cats-todo      # build + run in this pane
go test ./...                                        # ~3s; run before every commit
catctl plugin link .                                 # dev mode: cats launches this checkout's bin/
```

Drops need a live cats server (`CATS_CONTROL_SOCKET`); outside cats the manager still
edits backlogs, so most UI work can be exercised in any terminal. `.cats-todo/` and
`.claude/` here are **gitignored** (dogfood scratch), `bin/` too.

## File map

| File | Owns |
|---|---|
| `main.go` | argv dispatch (`add`, `init`, `__complete`, `-p/-g`, `version`, `help`); the `version` const |
| `ui.go` | the Bubble Tea model: `uiStage` (list, form, confirm, view, drop picker, images, schedule, export, session/view/spelling panels…), `Update` per-stage `update*` funcs, `View`, footers, button rows, `paneTitle()` (the mux scrapes `todo (N)` from it for the paw badge) |
| `store.go` | `Todo`, `store` (load/save/add/move/setDone/setFrozen…), root resolution (`findProjectRoot`, `projectTodosPath`, `globalTodosPath`) |
| `fuzzylist.go` | reusable fuzzy-filtered list used by the todo list *and* the drop picker; grouping, headings, priority lens |
| `drop.go` / `client.go` / `launch.go` | performing a drop; cats control-socket client (`pane.list`, `tab.create`, `pane.wait_for_output`, `pane.send_input`), `waitForAgentReady`, `claudeReadyProbes` |
| `worktree.go` | "on a new worktree" drops (`todo/<slug>-<4hex>` branches via cats) |
| `session.go` | `SessionOpts`, normalizers (`normalizeModel/Effort/Permission/Finish/Review`, `foldOption`), launch flags, prompt wrapping |
| `annotations.go` | the `annots` set, the `annotSlot` table (priority, low-hanging fruit), `trimAnnotColumns` |
| `annotbar.go` | the form's segmented annotation bar (Quick-win checkbox, Priority radios) between title and prompt |
| `priority.go` | `normalizePriority`, labels, rank; none = `""` (levels: none/high/critical) |
| `schedule.go` | parse `15:30` / `in 2h` / `tomorrow 9:00`; `Schedule` ⇄ dropTarget |
| `cli.go` | `add` flags (incl. `expandSessLoad`, `optString`, `stringList`) |
| `init.go` | `init [-f] [--post-install]` with the show-then-ask overwrite guard |
| `complete.go` | `__complete` protocol for `catctl completion` |
| `export.go` | Export-to-project picker: cats workspaces (pane.list ⋈ workspace.list), other backlog, cdx recents, folder browser |
| `images.go` / `clipboard.go` | attachments copied into `images/<id>/`; macOS pasteboard |
| `filepick.go` | `@` file picker in the editor (borrowed from cdx, file-level) |
| `promptsel.go` | text selection inside the textarea |
| `spell.go` / `spellpanel.go` / `internal/spell` | spell check + panel; embedded SCOWL list + `extra.txt` |
| `settings.go` | `~/.config/cats-todo/settings.json` (`spellcheck`, `orderByPriority`, `showFrozen`) |
| `styles.go` | the palette (see lockstep below) and lipgloss styles |
| `context.go` | where we're running from (`RunContext`, `CATS_PANE_ID`, cwd) |
| `internal/app`, `internal/ctlproto`, `internal/integration` | client-side copies of cats' wire vocabulary / socket client / env contract |

## Contracts that must not drift

1. **`todos.json` compatibility.** Every field added to `Todo` (or `SessionOpts`) is
   `omitempty` with a zero value meaning "default", so an untouched backlog stays
   byte-identical and an older binary ignores the key. `done` and `frozen` are
   mutually exclusive (open / frozen / done = the three list groups). Array order is
   the user's order — never sort the file; sort only as a lens in the list.
   Annotations (`priority`, `fruit`) are one set, edited and saved together
   (`annots`, `store.setAnnots`); a new mark is a field on `Todo`, a field on
   `annots`, a line in its three methods and an entry in `annotSlots`. The row
   reads badge → annotation columns → name, each column fixed-width and dropped
   list-wide when nobody fills it.
2. **Two-place version bump.** `const version` in `main.go` **and** `version =` in
   `cats-plugin.toml` must match (the title chip shows it). Release = bump both,
   commit `chore(release): vX.Y.Z`. Bump the minor for a feature, patch for a fix.
3. **Lockstep with cats** (`~/projs/go/cats`):
   - `internal/app/command_vocab.go`, `internal/ctlproto/{client,proto}.go`,
     `internal/integration` mirror cats' — wire names/values are the contract; when the
     protocol grows, copy the change over rather than inventing local names.
   - `styles.go` palette = cats' built-in green theme (now `internal/theme/builtin.go`
     in cats; the comments/README still say `internal/config defaultColors`). Only the
     grey ramp (`colMuted/Dim/Faint`) is ours. `colTodo` (#f0dfa0) is cats' `todo` hue;
     amber `colWarn` is reserved for fuzzy-match highlights.
   - `claudeReadyProbes` (`client.go`) track Claude Code's banner/footer strings. A
     stale list silently costs every new-session drop the full 12s timeout — when
     drops go slow, capture a startup and re-check this first. Keep probes
     version-agnostic (`"Claude Code v"`), with real spaces.
4. **Refuse in words.** Anything the UI won't do (drag while filtered or in priority
   order, drop/schedule a frozen prompt, send an empty prompt, export into the same
   backlog) says why in the status line — never silently no-op.
5. **Mouse reporting only on screens with something to click** (list, pickers,
   form buttons); the prompt view leaves terminal text selection alone.
6. **Button rows shrink, never wrap or drop a chip** (chords → words → gaps → bare
   glyphs); the footer then names every chord the chips stopped teaching.
7. **Nothing binary over the wire** — attachments are delivered as absolute paths
   appended to the prompt; missing files are omitted, not sent.
8. **CLI and TUI share normalizers** so a value refused in one is refused with the
   same words in the other (`session.go`, `priority.go`). `--priority` is long-only.

## Key-chord ownership (check before binding anything)

- **List:** `enter` edit · `shift/alt+enter` drop · `ctrl+a` add · `ctrl+e` edit ·
  `ctrl+v` view · `ctrl+t` done · `ctrl+f` freeze · `ctrl+s` schedule · `ctrl+o` export ·
  `ctrl+x` delete · `ctrl+↑/↓` move · `ctrl+d` fold closed · `ctrl+l` View panel ·
  `ctrl+w` clear done · `tab` button row · `esc`/`ctrl+c` quit.
- **Form:** `ctrl+s` save (also `cmd+s` as `super+s`/`meta+s`, which only a terminal
  that reports Cmd — cats does — can send; and `enter` from the title field) ·
  `enter`/`shift+enter`/`alt+enter`/`ctrl+j` newline in the prompt · `ctrl+o` (and
  `ctrl+i`) images · `ctrl+r` ⚙ panel — session options only (moved off `ctrl+e`, which is
  the caret's end-of-line) · `ctrl+l` Spelling · `ctrl+g` toggle project/global scope (add mode,
  both backlogs available) · `tab` walks title → prompt → annotation bar (the
  segmented Quick-win/Priority menu between title and prompt, `annotbar.go`;
  on it `←/→` move, `space`/`enter` press) · `@` file picker ·
  **Send** is click-only by design.
- `shift+enter` needs the kitty keyboard protocol; `alt+enter` is the universal alias
  (`m.modEnter()` picks which to advertise). `ctrl+c` in the form copies a selection
  before it means quit.

## Tests

- `*_test.go` beside each file; UI tests drive the model directly with
  `tea.KeyPressMsg{Code: …, Mod: …}` / `tea.WindowSizeMsg` through `m.Update` or the
  stage's `updateList/updateForm/updateConfirm/updateView…`. Helpers:
  `newTestModel()` (ui_test.go, no stores on disk), `newModelInTemp(t)` (model_test.go,
  real temp stores), `tempStore(t)` (store_test.go), `pressList`, `enterKey(mod)`.
- Feature = code + tests + README section + (usually) a session doc. Regression tests
  carry a comment saying what broke and why (see `TestWindowSizeMsgNeverPanics`).
- `go test ./...` must be green before a commit; the repo's allowlist already permits
  `go build/test/list`.

## Docs and commit habits

- **README.md is the spec**, written as prose that explains the *why* of each
  behaviour. Update the relevant section (or add one) with every user-visible change;
  the key tables above were derived from it. `cats-plugin.toml` is the reference
  manifest for cats plugins — keep its comments accurate.
- Code comments follow the same voice: design rationale, not narration; logic-heavy
  sections get the longer comment. Match the existing headers (`// file.go — what it
  owns`).
- Plans in `ai_docs/plans/`, session write-ups in `ai_docs/claude_sessions/` via
  `/sess-save` (`/sess-wrap` = save + commit + push); load prior context with
  `/sess-load [n]` or `/sess-use <pattern>`.
- Commits: conventional, lower-case, descriptive clause — `feat(form): …`,
  `feat(list): …`, `feat(drop): …`, `fix(ui): …`, `docs(session): …`, `docs(readme): …`,
  `chore(release): vX.Y.Z`.
