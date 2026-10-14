# Session: the drop picker lists this project's agents first

Session ID: 8b99d5dc-0ccd-4198-ac0f-ad1c64c9a5d2
Date: 2026-10-14
Not released. Under the release rules this is a patch (v0.45.1): it changes
the order of an existing picker and adds no capability.

## The ask

From the user's backlog, with a screenshot of the picker:

> In the list of drop targets move the (this project) option to the top so it
> is the first choice

The picker opened on the four new-session rows (Claude Code / Copilot, plain
then worktree), with `claude · chronos_update_jobs (this project)` fifth,
just above `… More drop targets`.

## What changed

`buildTargetsFor` (`ui.go`) now builds the running-pane rows into two
slices as it goes: `hereRows` (the pane's workspace ID equals this launch's,
the same test that earns the `(this project)` suffix) and `otherRows`
(elsewhere, folded or not, panes whose workspace can't be told, and the More
row). The final order is spliced before the `listItem`s are built, since each
row's index is its `ref`:

```
claude · proj (this project)          ← default highlight when there is one
＋ New <agent> session …              ← default highlight otherwise
＋ New <agent> session on a new worktree …
<agent> · elsewhere / unplaced panes
… More drop targets (N running agents in other projects)
```

The split is done while building rather than by re-sorting afterwards, so the
existing stable Claude-first sort still holds within each block.

Things that deliberately did not change:

- Without a socket, or with no agent in this workspace, the list opens on
  "New Claude Code session" as before. The existing tests pinning that
  default (`model_test.go`) run socketless and still pass untouched.
- A pane with no workspace ID stays below the new sessions. Calling it
  "this project" would be a guess, the same reason the fold never hides it.
- `expandTargets` still highlights the first `elsewhere` row, which is now
  after the new-session rows; nothing needed changing there.
- The batch composer's Target picker uses the same builder, so a new batch
  (no target yet) now opens on this project's pane; one with a target still
  lands on it via `selectRef`.

## Tests

`TestBuildTargetsPutsThisProjectFirst` (`droptargets_test.go`) uses the fake
cats socket with a codex and a claude pane here, a claude pane in another
workspace and an unplaced one. It pins the whole order
(`p4 p2 claude codex p1 p3 +`: here claude, here codex, the two new sessions,
the folded pane — kept in `targets` for the filter — the unplaced one, More),
the `(this project)` label on row 0 and the default selection 0. The first
draft left the folded pane out of the expected order. Folded rows are hidden
at rest but still in `targets`, which the fix to the test records.

`go vet` and `go test ./...` green.

## Docs

README "Using the manager": the picker paragraph now says this project's
agents come first and the highlight opens on one, then new sessions (plain,
then worktree), with the fold last.

## Next

Closed: None. Declined: None. Raised: N-085.
Deferred: None. Promoted: None. Moved: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
