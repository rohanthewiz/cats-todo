# Batches: loop as the default, and the page's right-click menu

Session: `7af52ef8-4ef2-43d8-9d5e-09b5fc1f0a87`
Date: 2026-09-25

## Ask

1. `/sl batch-loop-phase3` loaded the phase 3 session doc and the next-list.
2. "Make "loop" the default deliver option for batches." Then "commit".
3. The Next List item N-054 was pasted (the Batches page has no right-click
   menu), and it was built.
4. `/sw`.

## Loop as the default Deliver (`87727ec`)

- `beginBatchCompose` (`batchcompose.go`) now starts `deliver` on
  `deliverLoop`. `fresh` is false, so it is a loop in the same session.
  - Why loop: it is the one mode that takes any target and any count. All at
    once refuses a running pane with more than one prompt. A loop also never
    sets two prompts working on the tree at once.
- **Only the starting radio changed.** `deliverEach` stays the stored zero
  value (`""`), so the records already in `batches.json` keep their meaning.
  ⧉ Duplicate and editing a plan copy `b.Deliver` over the default
  (`composerFromBatch`).
- **Tests:**
  - `TestComposerDropRefusals` now chooses all at once by hand before it
    checks the running-pane refusal.
  - `TestComposerLoopRows` now checks that a new composer opens on loop with
    10 rows, that moving off loop hides the loop rows (5 rows), and that
    moving back shows them.
  - The other composer tests still pass on loop. Scheduling a plan
    (`TestScheduleBatchWritesAPlan`) now schedules a loop.
- **Docs:** the composer was rendered at 120 columns and the README diagram
  copied from it: the Deliver radio on loop, the five loop rows, and the
  longer ✱ note. The Deliver bullet says loop is the default and why. The
  `batchcompose.go` header diagram also lost its old two-radio Deliver row.

## The Batches page's context menu (N-054)

**`batchmenu.go` (new)** puts a `batchesMenu` on `menuBox`. It holds the
page's row actions, on the batch that was right-clicked:

- ✎ Edit… on a plan (`Batch.editable`), ☰ Open record otherwise (`enter`);
- ⧉ Duplicate… (`ctrl+d`);
- ✕ Unschedule, or ■ Stop on a running loop (`ctrl+u`, the chip's own swap);
- ✖ Delete record… (`ctrl+x`).

Design choices:

- **Only the row actions.** ＋ New and ← Back are about the page, not a batch,
  so they stay on the bar. The list's menu leaves Import off, and the Next
  List's leaves Refresh, for the same reason.
- **Delete keeps the two-press rule.** A menu row is as easy to press by
  mistake as a chord. The first press arms it. The next menu on that batch
  reads ✖ Confirm delete, and opening that menu keeps the arming note on the
  heading. A right-click doesn't disarm, so each of the two presses can come
  from the chord or the menu.
- **The menu carries the batch ID, not the row index.** A running batch's
  progress re-reads and re-sorts the rows under an open menu
  (`batchStatus` → `reloadBatches`).
  - Before any row acts, `focusBatch` re-reads the files, puts the highlight
    back on that ID, and then runs the chord's own function. There is one
    code path, with its guards.
  - The re-read is unconditional. The first version looked in the rows in
    memory first, and a test caught the result: a batch deleted in another
    pane was still "found" and opened. The press now says it is gone.
- **Refusals are shared.** The Unschedule and Delete refusals were pulled out
  of `batches.go` into `unscheduleWhy` and `m.deleteWhy`. The bar's greying,
  the chords and the menu's dim rows now use them. The Unschedule chip's
  greying rule is `unscheduleWhy(hb) != ""` (the same rule as before).
- **Duplicate is never dim.** Its one refusal (every prompt done or gone)
  needs the backlogs read, so it is said on the press instead.

Wiring:

- `ui.go`: the `batchesMenu` field; cleared on resize and in `backToList`;
  the right button's `stageBatches` case; the overlay in `renderStage`.
- `batches.go`: cleared in `openBatchesPage`. `updateBatches` and
  `clickBatches` hand to the menu first while it is up, before the delete
  arm is touched. The footer gains `right-click menu`.
- `menu.go`: the header now counts four menus.

**Tests** (`batchmenu_test.go`, 7):

- opening on the right button, with each row's label by state;
- only a batch row opens it;
- a click off the box dismisses only, and doesn't arm the ✖ Delete chip under
  it;
- the two-press Delete, and a dim row saying why;
- a press following the batch after a re-sort, and a vanished batch;
- ■ Stop and a dim Delete on a driven loop;
- resize, leaving the page and reopening it all close the box.

Mutation check: bypassing `focusBatch` fails the follow-the-batch test on both
of its halves.

The menu was rendered over the page at 120 and 80 columns. At 80 the bar drops
its chords and the menu still prints them. `go test ./...` and `go vet` pass,
and the new files are gofmt-clean.

**Docs:**

- README: a **Right-click a batch** paragraph and box in *The Batches page and
  the record*.
- The dev skill: a `batchmenu.go` row and the page's chord line.
- The plan: a note under "Phase 2 as built", where it said the menu was not
  there yet.

Not released: v0.39.0 is still the version (N-059). The menu hasn't been
driven in a live cats pane (N-058).

## Next

Closed: N-051, N-054. Declined: None. Raised: N-058, N-059.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
