# Batches phase 2: scheduling

Session: `8f9a0bf5-de0f-455e-91c0-64a951170de6`
Date: 2026-09-25

## Ask

`/sl` loaded `2026-0925-1315-multi-drop-batches-phase1` and the next-list.
Then: "continue with phase 2 of the plan", which is N-049 and the plan's
phase 2 (`ai_docs/plans/multi-drop-batches.md`). Then `/sw`.

Phase 2 as planned:

- The composer's **When** row and ◷ Schedule.
- `fireDueBatches` on the schedule tick, with grace, *missed* marking and
  a claim before firing.
- Editing a scheduled batch, and ✕ Unschedule.
- A ⧉ badge on list rows whose prompt sits in a pending batch.

## As built

**`batch.go`**

- **States:** `scheduled`, `missed` and `unscheduled` are *plans*
  (`Batch.editable`); `running` and `done` are history. The file comment
  diagrams the transitions.
- **Record fields:** `At` (the fire time, kept after firing so the record
  shows it went on a schedule) and `Why` (the reason a batch missed).
- **`swapBatch` / `takeBatch`:** a compare-and-swap (or delete) on the record's
  state and `At`, which is `claimSchedule`'s rule generalised. Every change to
  a plan goes through it: the fire's claim, missed marking, unschedule and
  saving an edit. A record deleted elsewhere is a lost swap and is never
  re-inserted.
- **`batchWatch`:** a read cache keyed by file size and mtime. The tick
  (every second) and the list's ⧉ marks (every rebuild) read both files
  through it, so a file that grows with every batch ever sent isn't re-parsed
  on each tick. It is only ever a view: actions reload and swap.
- **`pendingRefs`:** prompt → earliest fire time, counting scheduled batches
  only.

**`batchrun.go`**

- **`startBatch(b, fired)`:** split out of `launchBatch` so the tick can start
  a batch without a composer. `launchBatch` wraps it for the composer (leaving
  it, and reporting status).
- **`fireDueBatches(now)`:** runs on the tick after `fireDueSchedules`, whose
  argument order means it sees the guard a schedule may have just taken.
  - **Grace:** past `scheduleGrace`, or with no socket, the batch is marked
    missed with a reason.
  - **One drop at a time:** it waits while `m.dropping` is held.
  - **Claim:** it claims the batch by swapping scheduled → running.
  - **Fresh read:** it re-reads both backlogs before resolving the items.
  - **Open edits:** it skips a batch open in this pane's composer
    (`m.batch.edit.ID`).
  - **Failed start:** if the start fails after the claim, the record is
    written back as missed rather than left *running* forever.
- **`batchRunner.fired`:** a fired batch's steps go through
  `performScheduledDrop`, so a running-pane target is re-checked.

**`batchcompose.go`**

- **When row:** the fifth settings row. It is one text field (empty = now,
  otherwise `parseScheduleTime`), with a live `→ Sat 09:00` or
  `can't read that` beside it.
- **◷ Schedule chip:** `ctrl+s`, or `enter` on the When row. The When row
  greys whichever of Drop now / Schedule doesn't apply
  (`batchCommonWhy`, `batchDropWhy`, `batchScheduleWhy`). Scheduling with
  the row empty moves the focus there.
- **Shared build step:** `buildBatch` is shared by drop and schedule. It
  turns Next List items into backlog prompts only after every check that
  could refuse.
- **Edits:** `bc.edit` holds the record being edited. `saveEdit` swaps
  against it, and moves the record between files when its scope changes (a
  global batch that gains a project prompt). The title reads "Edit batch",
  and the leave-guard wording says "changes" for an edit.

**`batches.go`**

- **Row order:** running first, then scheduled soonest first, then the rest
  newest first.
- **Row text:** badges ◷ (red when missed) and ◌, and the time phrase from
  `batchWhen` (`fires …` / `missed …` / `not scheduled`).
- **Opening plans:** `composerFromBatch(b, edit)` serves both ⧉ Duplicate
  and `enter` on a plan. The When row is prefilled in the stamp form, so it
  round-trips; the note explains a missed batch, or one whose time has come.
- **✕ Unschedule (`ctrl+u`):** swaps scheduled → unscheduled, keeping the
  batch. The chip is greyed unless the highlighted batch is scheduled.
- **Record view:** a "Scheduled" line.

**`ui.go`**

- **Model:** a `batchWatch` field, created on first use.
- **Tick:** `fireDueBatches` hooked into it.
- **Blink:** forwarded to the When box.
- **List rows:** a `⧉ HH:MM` description mark on open prompts in a scheduled
  batch, beside the prompt's own `⏰`.

**Tests (`batchsched_test.go`, 12):**

- **Store:** the swap as a claim (a stale copy loses, a deleted record isn't
  inserted), the watch re-reading after writes, and `pendingRefs`.
- **Composer:** the When row choosing the button, and scheduling writing a
  plan with ⧉ on the rows and nothing done.
- **Firing, the tick's five cases:** fires inside grace, missed past it, waits
  on a drop in flight, leaves an open edit alone, and not yet due.
- **Firing:** a fired batch reading the backlog afresh (a prompt completed via
  another store is skipped).
- **Edits:** editing in place, an edit losing to a fire (both ctrl+s and Drop
  now), and an edit moving across files.
- **Page:** Unschedule keeping the plan, and page ordering.

Mutation check: with the state/`At` comparison in `swapBatch` disabled,
`TestBatchSwapIsAClaim` and `TestEditLosesToAFire` fail. The composer, list
and page frames were also checked by eye at 120 and 80 columns.
`go test ./...` passes. `gofmt -l` still lists only the pre-existing
`promptcode.go` and `ui.go` (N-052).

**Docs:**

- **README:** a new section, **Scheduling a batch** (firing rules, the ⧉ row
  mark, editing a plan, Unschedule). The composer diagram, the settings list,
  the buttons paragraph, the page diagram, the badges and the sort were
  updated too.
- **Plan:** a "Phase 2 as built" section listing the departures:
  - When is one field.
  - The plan's *cancelled* became *unscheduled*.
  - Everything goes through a compare-and-swap.
  - An open edit isn't fired.
  - An edit can move files.
  - There is a read cache.
  - The pane is re-checked.
  - ⧉ is a row mark, not the badge.
  - The page has no menu yet.
- **`cats-todo-dev` skill:** the file map and the composer and page chords.

Not released: the version is still 0.38.0, and N-051 now covers phases 1
and 2 together.

## Next

Closed: N-049. Declined: None. Raised: N-053, N-054, N-055.
Deferred: None. Promoted: None.
Updated: N-051. Full list: `ai_docs/todo/next-list.md`.
