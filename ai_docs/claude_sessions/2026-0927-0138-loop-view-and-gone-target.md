# Session: a loop keeps the view, and Drop now checks its pane (N-073, N-074)

Session ID: 48536950-83c8-462f-92e4-5896c04d09cb
Date: 2026-09-27
Mode: unattended backlog run, eighth pair (after
`2026-0927-0127-scheduled-batches-and-loops-live`). Both items were raised
earlier tonight by the live runs, and both had a small, clear fix.

## N-073: a same-session loop moves the view once (`9ffbf76`)

Every loop step went through `performDropAt`, which ends a running-pane drop
with `focusPane` (`agent.focus`). While watching a loop from the Batches
page, the view was pulled to the agent's tab at each step, and any menu open
on the page closed, because hiding a tab resizes it.

- `pendingAction.keepView` (`ui.go`) makes `performDropAt` skip the focus.
- `loopAction` sets it once any run has landed (`Batch.landedOnce`: a run
  with no error and a pane). The first step shows where the loop is
  running, and the rest leave the view alone.
- Fresh-each steps open new tabs, and cats focuses a new tab itself. That
  is untouched, and it is right for them.
- A scheduled single drop still moves the view when it fires. That was not
  asked about here, so it was left alone.
- `TestLoopSameSessionRunsInOrder` checks the first step moves the view and
  a later one does not. The README's Loop bullet says so.

## N-074: Drop now refuses a pane that has gone (`a57c04a`)

⧉ Duplicate and ✎ Edit copy the old record's target. A duplicate aimed at a
closed pane failed every step with `cats error: unknown pane 356` and left a
✗ record.

- `batchTargetGoneWhy` (`batchcompose.go`) runs in `dropBatch` on the press.
  For a running-pane target it asks pane.list and refuses in the scheduled
  fire's two cases: `the target pane is gone …` and `… agent has exited …`,
  each pointing at the Target row. A socket error lets the drop go on and
  report what the socket says.
- It is not in `batchDropWhy`, which also greys the button on every frame,
  and a round trip per frame would be too much.
- `TestComposerRefusesAGoneTargetPane` covers gone, agent exited and alive,
  using `fakeCatsSocket`. Checked live on the rebuilt binary: the duplicate
  of the ✗ batch was refused, and nothing was written.

## Next

Closed: N-073, N-074. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
