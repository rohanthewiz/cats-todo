# Session: scheduled batches and loops, live (N-053, N-056)

Session ID: 48536950-83c8-462f-92e4-5896c04d09cb
Date: 2026-09-27
Mode: unattended backlog run, seventh pair (after
`2026-0927-0111-batches-menu-and-batch-runs-live`).

## N-053: scheduled batches (closed)

- Missed: a batch was scheduled `in 1m`, the manager was quit, and it was
  reopened after the 2-minute grace (`scheduleGrace`). The row read
  `missed 01:21`, and enter opened the composer with `missed 01:21 — the
  manager was not open at 01:21 · set a new time, or drop it now`.
- The list marks: `⧉ 01:21` while scheduled, `⧉ 02:24` after a
  reschedule, and gone after ✕ Unschedule (ctrl+u).
- A worktree fire: the same batch, rescheduled onto a new worktree, fired
  on time, cut `todo/n-006-…`, and read `1/1 sent, finished · marked
  done`. It was cleaned up afterwards.
- Pane gone, and Scheduled/Dropped times: covered last pair under N-048.

## N-056: loops (closed, one fix)

The run covered all these cases:

- a same-session loop of three with `/clear` between;
- `/compact` between;
- fresh each onto worktrees;
- a permission question holding the loop;
- a quit and reopen mid-loop;
- a scheduled drop into a pane whose agent had quit.

The closure in `next-list.md` has the detail.

### The fix: the record screen went stale (`11b250a`)

The loop was quit mid-run and reopened. The new manager took it over and
finished it on its tick, but its record screen (open throughout) kept
saying `Now: paused at 2/3 — no manager is driving it`. Going back to the
page still showed ⟳. Only a fresh ctrl+k showed ✓.

Cause: `batchStatus` reloaded the rows only when the stage was the page
itself. The record screen shows `m.batches.view`, a copy taken when it
opened. Its Now line is worked out live from `m.loops`, and the finished
loop is no longer in that map, so the old copy read as paused. Now
`batchStatus` also reloads on `stageBatchView` and swaps the view for the
fresh copy by ID and scope (`refreshBatchView`). A batch deleted meanwhile
keeps its last copy on screen. Test: `TestBatchViewFollowsTheRecord`, which
fails on the old code.

### Noted, not raised: cats' agent label lags an exit

After `/exit` in a shell-hosted claude, `pane.list` kept `agent: claude,
idle` for a few seconds with the shell prompt already back. The fire at
01:27:30 came after it cleared, and was marked missed as N-020 designed. A
fire inside that window would pass `scheduledPaneAgent` and type the prompt
at the shell. The window is cats' detection latency and it is narrow. It is
recorded here in case it ever shows up.

## Rig notes

- Waiting for a clock time: `until [ "$(date +%H%M%S)" -ge 012550 ]; do
  sleep 5; done`. Chained sleeps are blocked in this harness.
- Workspaces made by worktree drops are named after the branch. After a
  cleanup, compare the workspace list with the one from the session start;
  it was 21 of the user's own plus `ct-live` and `ct-other`.

## Next

Closed: N-053, N-056. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
