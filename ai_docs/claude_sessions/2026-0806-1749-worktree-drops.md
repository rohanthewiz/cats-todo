# Session: dropping a todo onto a new git worktree

Session ID: `13d25ade-6473-44e4-a332-7c1753941399`
Date: 2026-08-06

One line of ask:

> "From the cats-todo plugin I want the ability to drop new todos into a new
> agent on a new worktree"

No plan mode. Read `drop.go`, `launch.go`, `client.go`, `context.go`,
`schedule.go` and the picker half of `ui.go`, then grepped both repos for
"worktree" — which is where the shape of the whole session was decided.

## The discovery that set the design

cats already owns the git side. `internal/app/command_vocab.go` (vendored into
this plugin) declares `worktree.list/create/open/remove`, and
`cmd/catway/worktrees.go` implements create as: `git worktree add -b <branch>`
off the anchoring pane's repo at HEAD, then `CreateWorkspaceAt(path)` — a new
workspace, focused, renamed to the branch — replying with
`{workspace, branch, path}`.

So the plugin needed to write no git at all. Two decisions were left:

1. what to call the branch, and
2. when the option may be offered.

That is the entire content of the new `worktree.go`. Shelling out to `git
worktree add` from the plugin was never seriously on the table: it would have
meant a second convention for where checkouts live, in a tool whose whole
premise is that cats already knows.

Confirmed against the live server before writing anything downstream of it —
the MacApp's catway answers `worktree.list`:

```
❯ CATS_CONTROL_SOCKET=… catctl worktree.list
{ "repo_root": "~/projs/go/cats", "repo_name": "cats",
  "worktree_root": "~/.cats/worktrees", "worktrees": [ … ] }
```

Read-only on purpose. No worktree was actually created during the session —
that leaves a branch and a checkout behind on the user's machine, and the
build/test evidence didn't need one.

## `worktree.go` — the two decisions

**The branch name.** `todo/<slug>-<4 hex>`, e.g. `todo/fix-the-sidebar-3f9c`.
Three parts, each earning its place:

- `todo/` — a directory in git's ref layout, so the batch folds into one group
  in ref-tree tooling and `git branch -D 'todo/*'` clears it. Free on disk:
  cats's `BranchToPathSlug` flattens the "/" when deriving the folder.
- the slug — title, falling back to the prompt's first line.
- the suffix — **the load-bearing part**. `git worktree add -b` refuses an
  existing branch, so a bare title slug fails on the *second* drop of the same
  todo. And dropping one prompt onto three parallel worktrees (three attempts,
  compared afterwards) is exactly the workflow this feature exists for. Seeded
  from the clock rather than the todo id so each drop differs while the
  readable half stays stable and one todo's branches sort together. `seed` is a
  parameter, not a `time.Now()` inside — deterministic under test, matching how
  cats seeds `GeneratedBranchSlug`.

**`branchSlug` permits only `[a-z0-9-]`, no edge dashes.** git's ref rules are
a *list of prohibitions* (`..`, `@{`, trailing `.lock`, leading/trailing `/`
or `-`, control chars, spaces), and matching them one by one means
re-deriving them correctly forever. A permitted set that cannot spell any of
them is valid by construction. Truncation is applied before the final trim so
a cut landing mid-word can't leave a trailing separator.

**`gitRepoRoot`** walks up for a `.git` entry, returning `""` when there is
none — which is what gates the rows. A `.git` **file** counts as much as a
directory: that is the shape of a linked worktree, i.e. exactly what this
feature produces, so a manager opened in a checkout it made must still be able
to cut the next one. Deliberately *not* reusing `walkProjectRoot`'s `.git`
pass — that walk answers "which directory owns the backlog" and falls back to
`dir`; this one must be able to answer "no repo here" at all.

## `dropTarget.worktree` — a flag, not a third kind

`targetNewWorktree` alongside `targetNewSession` was the obvious alternative
and is wrong: every step of the drop is identical — the tab, the readiness
wait, the paste, the submit — and only the directory the tab is rooted at
differs. A third kind would have forced a second `switch` arm in `performDrop`
that immediately re-converged.

`drop.go` reflects that. `dropIntoNewSession` gained four lines:

```go
cwd := act.cwd
if act.target.worktree {
    path, err := dropWorktree(client, act)   // ← the only new step
    if err != nil { return err }
    cwd = path
}
```

**A failure aborts rather than falling back to the project checkout.** Choosing
the row is choosing the isolation, and quietly starting an agent in the shared
tree is precisely the outcome the user asked to avoid. Same reasoning as
`performScheduledDrop`'s refusal to substitute a new session for a vanished
pane.

`res.Path == ""` is checked defensively: a server reporting success with no
path would leave `tab.create` silently falling back to the workspace default,
which is the failure that looks like success.

## The anchor pane

`worktree.create` resolves the repo from the *addressed* pane's cwd, and
`Pane: nil` means **the focused pane**. Fine when a human just pressed a key;
wrong for a scheduled fire, which happens unattended with the focus wherever
the user last left it — it would branch some other repo entirely.

So `RunContext.OwnPaneID` (numeric; the API addresses panes by number while
`CATS_PANE_ID` is the `"w1:p3"` handle), resolved once at startup by
`resolveOwnPaneID`: parse the `p_<id>` fallback form for free, else one
`pane.list` round trip matched with the existing `isOwnPane`. Once, not per
drop — a pane's id is fixed for its lifetime and this is our own pane.

Threaded to the drop goroutine through `pendingAction.anchorPane`, alongside
`cwd` and `images` and for the same reason: that goroutine holds no
`RunContext`. The scheduled path reads it from the **live context, not the
schedule** — the anchor names a pane that has to exist *now*, where every
other field of a `Schedule` describes what was chosen back then.

## `buildTargets`, restructured

Previously: emit known-agent rows → `pane.list` → emit running-agent rows →
emit pane rows, all interleaved with the fetching. The worktree block has to
mirror the launchable-agent set *exactly*, so the function now gathers first
and emits second:

```
newAgents  ← newSessionAgents (PATH-gated) + distinct running agents (deduped)
targets    ← plain rows      (one per newAgent)
           + worktree rows   (one per newAgent, if gitRepoRoot != "")
           + existing panes  (claude first)
```

Row order is byte-identical to before for the first and last blocks, so no
existing test moved.

**Two blocks, not interleaved per agent.** `claude / claude+worktree / copilot
/ copilot+worktree` reads as four unrelated rows; the block form reads as one
answer to one question ("somewhere isolated, instead"), and — the part that
actually matters — the default highlight stays on row 0, the plain drop that
makes no branch. Pinned by a test.

The worktree block is **not** gated on `m.client != nil`, matching the plain
rows: `startDrop` refuses to open the picker at all without a socket, so the
nil-client path exists only in tests. The pane block's guard changed from
`m.client != nil` to `len(agents) > 0`, which is the same condition stated in
terms of what the loop actually needs.

`targetDesc` appends " on a new worktree" — worth spelling out in the status
line, since this drop is about to make a branch and a checkout that the plain
flavour never does.

## Schedules

`Schedule.Worktree bool` with `json:"worktree,omitempty"` — the compat
contract its neighbours carry, pinned by a test that greps the marshalled JSON
for the absent key. Carried both ways through
`scheduleFromTarget`/`targetFromSchedule`; without that, a scheduled worktree
drop would come back as a plain one and launch in the shared checkout, which
is the silent version of the failure the abort-don't-fall-back rule exists to
prevent.

**Recorded, not resolved, at schedule time.** The branch is named and the
checkout cut when the drop *fires*, so it comes off HEAD as it stands then —
not off HEAD as it stood hours earlier.

## Tests

- `worktree_test.go` (new) — `TestBranchSlug` (table + capped-without-trailing-
  separator + a "only safe characters survive" pass over `a..b @{c} d\te/f.lock`),
  `TestTodoBranchName` (prefix, prompt fallback, the `!!!` → `todo/todo-abcd`
  nameless case, and *the same todo dropped twice gets two branches*),
  `TestGitRepoRoot` (no repo, empty, from a subdirectory, and the linked
  worktree's `.git` **file**).
- `TestBuildTargetsOffersWorktrees` — `PATH` set to an empty dir so the row
  counts are about the two flavours and not about what's installed. Outside a
  repo: no worktree rows. Inside: exactly two targets, the flag on the second,
  the label saying "worktree", the desc naming the repo, and the default
  selection still index 0. Third subtest: found from a subdirectory.
- `TestTargetDesc` extended for both worktree spellings.
- `TestScheduleTargetRoundTrip` — the flag survives both directions, plus the
  omitempty assertion.

Full suite, `go vet`, `gofmt` green. The local `bin/cats-todo` was rebuilt.

## Release

Two commits, the repo's convention:

- `674d2c8` feat(drop): drop a todo into an agent on a new git worktree
- `e8d769e` chore(release): v0.10.0 — version bumped in **both**
  `cats-plugin.toml` and `main.go`'s `const version`

Tagged `v0.10.0` and pushed with `main`.

## Left on the table

**The installed plugin is still on 0.9.0.**
`~/.config/cats/plugins/rohanthewiz.cats-todo` is a clone from GitHub (it has
its own `.git` and a `.cats-plugin-source.json`), *not* a link to this
checkout — so nothing in this session reaches the MacApp until
`catctl plugin install rohanthewiz/cats-todo` re-runs the manifest's build
step. Offered rather than run: it mutates the user's installed environment.

**The new workspace arrives with a spare shell tab.** `CreateWorkspaceAt`
gives every workspace one tab and one pane, and the agent lands in a *second*
tab created after it. Driving the agent into that existing shell instead was
considered and rejected: it means typing `claude` into a shell prompt, which
this codebase deliberately moved away from (exec'd argv means no shell
startup, no echo-matching, and quitting the agent closes the tab). The spare
shell is also the obvious place to review the branch and merge it, so it was
left as a feature.

**No end-to-end run.** Everything up to `worktree.create` is covered by unit
tests and the live `worktree.list` probe confirms the server speaks the
command; the create itself is unexercised until the user drops a real todo.
