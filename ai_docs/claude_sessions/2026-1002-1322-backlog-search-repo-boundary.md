# Session: the backlog search stops at the repo root (and below $HOME)

Session ID: 65ddd0a3-cf6a-49b9-b7b0-7dd26a1cbac5
Date: 2026-10-02
Released as **v0.43.0**.

## The problem

Wherever cats-todo started, if there was no `.cats-todo/` there it searched
upwards for one. `walkProjectRoot` (`context.go`) ran the `.cats-todo` search
all the way to `/` *before* it ever looked for `.git`. A repo with no backlog
of its own then quietly adopted an unrelated one further up, such as a
`~/projs/.cats-todo` or a `~/.cats-todo`, and edits landed there.

The user asked for the walk to stop at a git repo, at any node including the
starting directory. If no `.cats-todo` is found there, the tool should say so
and offer to create one. A follow-up asked for the non-repo walk to stop at
`$HOME` instead of `/`.

## The new walk (`context.go`)

One upward pass. At each directory:

1. It holds a `.cats-todo` directory: that is the backlog
   (`isProjectBacklogDir`).
2. It holds `.git` (a directory, or the *file* a linked worktree or submodule
   has; `os.Stat`, not an `IsDir` check): the walk stops, and this repo root is
   the answer, backlog or not.
3. Ceiling: the walk never steps *into* `$HOME` from below, and never leaves
   it upwards (`walkCeiling`, the cleaned `os.UserHomeDir`). So `~/.cats-todo`
   is used only when launched in `~` itself. Dirs outside the home tree
   (`/tmp`, `/opt`) still walk to `/`.

With neither marker, the start directory is the root, as before. Checking
`.cats-todo` before `.git` at the same node keeps a repo's own top-level
backlog, and a subdirectory's backlog still wins when launched beneath it.
`findProjectRoot`'s filesystem-root refusal is unchanged.

`projectBacklogMissing(root)` reports whether a resolved root has no
`.cats-todo` yet. The directory is the marker, not `todos.json`.

## Saying so, and offering to create it

- **Manager** (`backlogoffer.go`, new): `offerCreateBacklog` is applied in
  `runTodoUI` after `newModel` (not inside it, so the test models built on
  temp paths that don't exist yet still open on the list). With a missing
  backlog it opens on a new confirm kind, `confirmCreateBacklog`, in the
  existing confirm stage (`ui.go`). The confirm names the full path, and says
  the search stops at the repo root when the root is one (`isRepoRoot`).
  - `y`/enter: `createProjectBacklog` saves an empty `[]` backlog (not
    `null`; it sets `todos = []Todo{}` first). The status line says to
    `git add .cats-todo`.
  - `n`/esc: `declineProjectBacklog` swaps in an unavailable project store,
    so the header reads "global only" ("no backlog here" on `--project`) and
    no later save can create the directory behind the user's back.
  - Skipped on `--global`, with no project, or when the backlog exists. The
    decline is not remembered (N-081).
- **`cats-todo add`** (`cli.go`) cannot ask, since its prompt often comes
  on stdin. It saves and adds a second line,
  `✚ no backlog here yet, so this started one: <dir> — git add .cats-todo …`,
  only on the add that creates it (`freshBacklog`).
- Export into a project with none still creates it silently (N-080).

## Tests

- `context_test.go` `TestFindProjectRoot`: repo boundary blocks an outer
  backlog (from deep and from the repo root itself), `.git` file boundary,
  backlog beside `.git`, ancestor backlog outside a repo, and three `$HOME`
  cases (non-repo stops below `$HOME`, a backlog below `$HOME` is found,
  launched at `$HOME` uses its own and looks no higher). They set `HOME`
  with `t.Setenv`.
- `backlogoffer_test.go`: opens on a missing backlog (path and repo-root line
  in the view), skipped cases, `y` writes `[]`, `n`/esc creates nothing and
  shows "global only", and declining on `--project` leaves the add form
  refused.
- `go test ./...` green.

## Live test

`~/projs/go/roman` is a git repo with no `.cats-todo`, under both
`~/projs/.cats-todo` and `~/.cats-todo`; the old walk would have taken
`~/projs/.cats-todo`. Drove the real binary in a pty (python `pty` + `pyte`,
120×30, `CATS_*` env stripped, scratch `CATS_TODO_CONFIG_DIR`):

- Launch opened on "No backlog in roman", naming
  `~/projs/go/roman/.cats-todo/todos.json` and the repo-root boundary.
- `y` went to the list (`roman + global`), with the created status line.
- `~/projs/go/roman/.cats-todo/todos.json` was created with `[]`. The outer
  backlogs were untouched.
- A second launch went straight to the list.

`roman`'s new `.cats-todo/` is left untracked there for the user to commit or
remove. The `add` path was also smoke-tested in a scratch repo under an outer
`.cats-todo`: the new backlog landed in the repo, and the note printed only
on the first add.

## Docs

The README's project-scope paragraph now explains the boundary, the offer
(with the dialog), `add`'s note and the `$HOME` ceiling. The export and `add`
sections' root rules were updated, as was the `addFromCLI` doc comment.

## Next

Closed: None. Declined: None. Raised: N-080, N-081.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
