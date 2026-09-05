# Re-syncing the socket envelope

Session: https://claude.ai/code/session_01AaoUtxuk5QmaSW7YqYSxJj
Date: 2026-09-04
Repos: `~/projs/go/cats` (read only), `~/projs/go/cats-todo`

Picks up the one item left open by `2026-0904-1921-importing-the-wire-vocabulary.md`
(cats-side twin: `cats/ai_docs/claude_sessions/2026-0904-1909-…`).

## Request

> re-sync cats-todo's ctlproto against cats' current file

## The shape of the problem

`internal/ctlproto` is a **copy**, not an import — it lives under cats'
`internal/`, so the module boundary forbids importing it. (The §7 *vocabulary*
stopped being a copy last session: it is `github.com/rohanthewiz/cats/wire` now.
This is only the envelope — Request/Response/Event and the socket dial.)

Last synced 2026-07-25. cats' side had moved on.

## What actually differed

Four files exist in cats' `internal/ctlproto`; cats-todo carries two.

| cats file | copied? | why |
|---|---|---|
| `proto.go` | yes | envelope shapes, stdlib only |
| `client.go` | yes | `ResolveSocket` + `Call`, stdlib only |
| `server.go` | **no** | imports `cats/internal/app` |
| `stream.go` | **no** | its server half needs `server.go`'s `Server` |

`stream.go:133 Subscribe` is the one genuinely client-side thing stranded on the
far side. cats-todo does not consume `events.subscribe`, so it stays there; a
future streaming client here would port that function alone rather than the file.

### `client.go` — the one behavioral change

`SocketNone`, the constant `"-"`.

cats exports `CATS_CONTROL_SOCKET=-` into panes running on a **remote cathost
that the session does not relay the control API to**. On the old copy that value
was merely "non-empty", so:

```
ResolveSocket("")  ->  "-"        // treated as a path
Call("-", …)       ->  dial fails
```

…and the failure is not the honest one. The comment cats added spells out why:
a pane on such a host inherits whatever environment the cathost was launched
with, which on a box where somebody started it from inside *another* cats
session is a different session's socket. Falling back to `DefaultSocket` is the
same hazard by a tidier route. Either way an in-pane client drives the wrong
terminals.

Now `ResolveSocket` returns `""` for it and `Call` refuses with a message naming
`CATS_CONTROL_SOCKET=-` and the relay. In cats-todo this surfaces at
`newCatsClient`'s ping, which already returns an error the manager degrades on
(todos still work, drops don't) — so no call-site change was needed, only an
honest error where there had been a misdirected one.

`client.go`'s doc comment in the repo root was updated to say so; it had promised
"CATS_CONTROL_SOCKET when set, else the default", which is no longer the whole
rule.

### `proto.go` — additions, none of them used yet

- `MethodClipboardRead` + `ClipboardData`. Deliberately off the §7 table for the
  same reason as `pair`: that table is shared with the browser front end, and the
  clipboard is the user's, not the session's.
  **Does not overlap cats-todo's `clipboard.go`**, which reads pasteboard
  *images* via `osascript`; `clipboard.read` is text.
- `TransportMethods()` / `IsTransportMethod()` — the single list of methods the
  control layer answers itself instead of routing. cats grew it because three
  places had each spelled out their own `m != MethodPing && m != …` chain.

## Method

A literal `cp` of both files, then re-apply the prose deltas that are *true here*
and false there:

- `app.Cmd*` → `wire.Cmd*`, `app.ReadResult` → `wire.ReadResult`,
  `app.CommandNames()` → `wire.CommandNames()` — those symbols really did move to
  the public package.
- `app.Dispatcher` / `app.JSONParamDecoder` → "the server's dispatcher" / "the
  dispatcher's JSON param decoder". These are **not** in `wire` and a reader here
  cannot follow the name.
- Event names stay described as literal strings (`"pane_exited"`, …) — they live
  in cats' `internal/app/events.go`, also not in `wire`.
- Package doc keeps the "this is a copy" note and now carries the recipe.

Result, verified with `diff -u` against cats:

- `client.go` — **byte-identical**.
- `proto.go` — differs **only in comments**.

That is the invariant worth keeping: the next re-sync is a `cp` and a comment
pass, and a `diff` tells you immediately whether anyone has edited the code
locally (nobody should).

## Docs

- `internal/ctlproto/proto.go` package doc — the re-sync recipe, and *why*
  `server.go`/`stream.go` cannot come.
- `.claude/skills/cats-todo-dev/SKILL.md` "Lockstep with cats" — same recipe,
  plus **"Last synced 2026-09-04 against cats `3da85b2`"** so the next drift is
  measurable instead of guessed at.
- `README.md` needed nothing; its line about `internal/ctlproto` still being a
  client-side copy is still exactly right.
- `go.mod`'s note also still right — unchanged.

## Checks

`gofmt -l .` silent, `go build ./...`, `go vet ./...`, `go test ./...` all green.

## Not done / next

- **No version bump.** The dev skill's two-place bump (`const version` in
  `main.go` + `version =` in `cats-plugin.toml`) is for a feature or fix a user
  can see. The `SocketNone` fix only fires on a remote cathost without
  `control_relay` — worth folding into the next release's notes rather than
  cutting one for.
- `internal/integration` (the `CATS_PANE_ID` sliver) was **not** examined this
  session. It is the other copy, and it is small, but "small" is not "checked".
- Nothing in cats changed; it was read only.

## Commit

- cats-todo `19fe7dd` — "ctlproto: re-sync the socket envelope against cats"

Pushed to `main`.
