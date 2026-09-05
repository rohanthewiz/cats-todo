# Importing the wire vocabulary

Session: https://claude.ai/code/session_01AaoUtxuk5QmaSW7YqYSxJj
Date: 2026-09-04
Repo: `~/projs/go/cats-todo` (branch `main`)
Companion doc: `~/projs/go/cats/ai_docs/claude_sessions/2026-0904-1909-the-vocabulary-stops-being-copied.md`

## Request

> update the protocol to cats-todo and cats-mobile accordingly

cats had just grown `workspace.clean` / `.sleep` / `.wake` (cats `5d1e4a6`), and
the question was how that reaches here.

## The answer turned out not to be "re-sync the copy"

`internal/app/command_vocab.go` was a hand-maintained copy of cats' §7
vocabulary. Its header stated the reason plainly:

> …which cannot be imported across the module boundary because it lives under
> `internal/`.

That stopped being true on 2026-09-02. cats' `c0a250f` ("wire: carve the browser
protocol out of internal into a leaf package") moved the vocabulary to
`github.com/rohanthewiz/cats/wire` — public, and importing nothing outside the
standard library. Inside cats, `internal/app/wire_aliases.go` re-exports it, so
`app.PaneInfo` there is an alias; there is one declaration left in the world.

Meanwhile our copy had drifted **~1200 lines behind** cats. It kept working only
because this repo uses a small subset. That is the failure mode of a copy nobody
diffs: it does not break, it just quietly stops describing the server.

So the copy was deleted rather than re-synced.

## What the migration touched

Every symbol we used was already in `wire`. The `app.Dispatcher`,
`app.JSONParamDecoder` and `app.EventPane*` references turned out to be
**comments only** — no code depended on anything that stayed behind in cats'
`internal/`.

1. `go get github.com/rohanthewiz/cats@5d1e4a6 && go mod tidy`
2. `git rm internal/app/command_vocab.go` — the package had no other file, so
   `internal/app` is gone
3. `app.X` → `wire.X` in `client.go`, `context.go`, `drop.go`, `ui.go`,
   `context_test.go`, `schedule_test.go`, then `gofmt` (the import sorts
   differently: `cats/wire` is third-party where `cats-todo/internal/app` was
   ours)

**One rename to absorb.** `app.WorkspaceInfo` — our `workspace.list` row type —
is `wire.WorkspaceEntry`. In `wire`, the name `WorkspaceInfo` belongs to the
*layout down-message's* workspace, a different shape for a different protocol.
`workspaceList()` in `client.go` and the projection comment in `export.go`
follow the rename.

**`go.sum` grew by two lines.** `wire` being stdlib-only is the whole reason
this is a clean trade rather than dragging cats' dependency tree in behind it.

## `go.mod` is now the contract version

The require line carries a note, mirroring cats-mobile's:

```go
// cats is the §7 wire contract, and this line IS the pin. The vocabulary is
// imported, never copied: `wire` is a public stdlib-only leaf package, so
// this dependency adds no transitive ones. Bump it with
//
//	go get github.com/rohanthewiz/cats@<sha> && go mod tidy
//
// and let the compiler name whatever moved. (internal/ctlproto and
// internal/integration are still hand-copied — those live under cats'
// internal/ and cannot be imported.)
github.com/rohanthewiz/cats v0.2.3-0.20260904234655-5d1e4a6716fe
```

Keeping up with a cats protocol change is now two commands and a build, with the
compiler naming anything that moved — not a `cp` and a careful read.

## The sleep protocol needs nothing from us — checked, not assumed

It came along with the import (`wire.CmdWorkspaceClean` / `.Sleep` / `.Wake`,
`CleanWorkspaceParams` / `Result`, `ParkedAgentInfo`, `asleep` + `parked` on
`WorkspaceEntry`). Two places could plausibly have cared:

- **`workspaceList` consumers.** Only `workspaceLabels` (id → display name) and
  `gatherExportSources`. The export picker's rows are project *roots* resolved
  from each workspace's pane cwd — and a sleeping workspace has no live PTY, so
  it reports no cwd and falls out of the picker on its own. A label for one is
  still the right label.
- **`tabCreate`.** Every call omits `Workspace`, so a new agent session lands in
  the active workspace, and cats guarantees the active workspace is never asleep
  (`SleepWorkspace` moves the active index away and refuses the last awake one).
  cats' "refused — use workspace.wake" path is unreachable from here.

## Prose that would have sent the next person to a deleted file

- `internal/ctlproto/proto.go` — comments referenced `app.Cmd*`,
  `app.CommandNames()`, `app.ReadResult`, `internal/app`. Retargeted at `wire`
  where the symbol actually moved there; left as "the server's dispatcher" for
  `Dispatcher` / `JSONParamDecoder`, and as literal strings (`"pane_exited"`)
  for the event names, which stay in cats' `internal/app/events.go` and are
  **not** in `wire`.
- `README.md`, "How it talks to cats" — rewritten: the vocabulary is imported,
  the pin is the contract, and only `ctlproto` + `integration` are copies.
- `.claude/skills/cats-todo-dev/SKILL.md` — the file table and contract 3
  ("Lockstep with cats") both told you to copy a file that no longer exists.
  Contract 3 now says the vocabulary is imported and never redeclared locally,
  and keeps the mirror rule for the two packages it still applies to.

## Still copied, and why

`internal/ctlproto/{client,proto}.go` (the control-socket envelope) and
`internal/integration` (the `CATS_PANE_ID` sliver) remain hand-copied — those
*are* still under cats' `internal/`, so the original argument holds. Neither
changed in cats `5d1e4a6`.

Worth knowing: our `ctlproto` is itself an older copy — cats has grown
`server.go`, `stream.go` and further methods since. Untouched here because it is
the transport envelope and this was a vocabulary change.

## Verification

`gofmt`, `go build ./...`, `go vet ./...`, `go test ./...` — all clean.
`go doc github.com/rohanthewiz/cats/wire CleanWorkspaceResult` confirms the new
shapes resolve from this module.

## Not done / next

- **No version bump.** The dev skill's two-place bump (`const version` in
  `main.go` + `version =` in `cats-plugin.toml`) is for a feature or a fix; this
  is internal plumbing with no user-visible change. A release can carry it.
- `internal/ctlproto` re-sync against cats' current file, whenever the envelope
  next matters.

## Commit

`6d8eb6d` — "wire: import cats' vocabulary instead of copying it", pushed to
`main`. cats-mobile got the matching pin bump in `6d5c5d8`.
