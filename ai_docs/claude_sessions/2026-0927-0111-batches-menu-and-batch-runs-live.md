# Session: the Batches page menu and real batch runs (N-058, N-048)

Session ID: 48536950-83c8-462f-92e4-5896c04d09cb
Date: 2026-09-27
Mode: unattended backlog run, sixth pair (after
`2026-0927-0104-picker-fold-and-double-click-live`).

## N-058: the Batches page's right-click menu (closed, one fix)

Real loops ran into a claude pane in the scratch project (`live-n038`),
with a 20–40s pause so there was time to act mid-run:

- ✎ Edit… on a scheduled batch and on a plan opens the composer. On a plan
  whose prompt has since been marked done, it opens with that prompt
  dropped, and esc takes two presses ("changes here are not saved…").
- ☰ Open record on a finished batch shows the record view.
- ■ Stop on a running loop stops it mid-pause: no second prompt went, and
  the record reads `Why: stopped by hand after 1/2`, with the unsent prompt
  `✗ … — never sent`.
- ✕ Unschedule turns a scheduled batch into a plan (◌).
- The two-press delete works from the menu.

**Decided:** a mouse user who armed the delete from the menu needs the
heading to name the row. It now reads `press ctrl+x again, or ✖ Confirm
delete on the menu, to delete the record of “…”` (`5aa4480`), with the test
and the README updated.

**The box and the re-sort:** the menu never outlived a loop step, but not
because of the re-sort. Each step's drop ends in `agent.focus`, which moves
the view to the agent's tab, and hiding the manager's tab closes its menus.
That was checked directly, with the menu up and two tab switches. Raised
as N-073: a loop should focus on its first step only.

## N-048: batches, live (closed)

- All at once onto new worktrees (N-005, N-006): two workspaces opened one
  after the other. Both prompts landed whole and ran, and both were marked
  done. The worktrees and branches were removed.
- One prompt, listed, into a running pane: one message (`This is a batch of
  2 tasks. …`) with both sections whole, and both answered.
- Failure part-way: a batch of two onto new sessions, scheduled `in 1m`,
  with N-001 deleted from the backlog before the fire. N-003 dropped and
  was marked done. The record reads `✗ 1. N-001 … — the prompt is no longer
  in the backlog`, `Delivered 1 of 2`, with Scheduled and Dropped times.
- A scheduled batch into a running pane closed before the fire: every step
  read `✗ … — the scheduled pane is gone — send manually` (part of N-053).
  ⧉ Duplicate then brought back both still-open prompts, but it kept the
  dead pane as the target, and ▶ Drop now failed with `cats error: unknown
  pane 356` (N-074).
- ✚ Next List items became backlog prompts, reusing an open copy.
- Narrow layout (90 columns): clicks pick rows, `Pick · Batch (n)` switches
  on a click, alt+↓ reorders, and radio clicks land. Dragging was not
  driven.
- The picker showed three identical rows for three claude panes in one
  project (N-075).

Also seen on the list: the `⧉ 01:15` mark on a scheduled batch's prompts.
All at once into a running pane is refused in words ("all at once gives
each prompt its own session…").

## Next

Closed: N-048, N-058. Declined: None. Raised: N-073, N-074, N-075.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
