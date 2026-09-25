# Multi-drop batches: the design, and phase 1

Session: `023a6820-acbe-4563-8771-4c8738f83a79`
Date: 2026-09-25

## Ask

The ask was a UI for dropping several todos onto one or more agents, for
both the backlog and the Next List:

- **Picking:** a left side with individual checkboxes and select-all.
- **Ordering:** a right side where the picked todos are dragged into order or
  sorted alphabetically.
- **Groups:** several groups, built one at a time, each with an optional name.
- **Options:** common session options for a group, including running in a
  worktree.
- **Timing:** drop a group now, or schedule it, where scheduling can also mean
  "a loop".
- **History:** a list of groups showing each one as dropped or scheduled.

Then "Yes start phase 1", and `/sw`.

## Design (`ai_docs/plans/multi-drop-batches.md`)

The code already had most of what this needed:

- **Drop target picker:** already offers new session, new worktree and
  running panes.
- **Session options:** the `SessionOpts` record and its ⚙ panel.
- **Scheduling:** schedules with a grace window, *missed* marking and
  claim-before-fire.
- **One drop at a time:** the `m.dropping` guard.
- **Reorder:** the list's drag.
- **Next List to backlog:** the Next List's `nextBacklogCopy` / add path.

The plan builds on those instead of adding parallel machinery.

**Naming.** The user's "group" became **batch**, because "group" already
names the list's open / frozen / done sections.

**Decisions, asked with AskUserQuestion:**

1. **Delivery:** all three modes: *all at once* (a session or worktree per
   prompt), *loop, in order*, and *one prompt, listed*.
2. **"Loop":** in the user's words, "send the items in sequence until the
   batch is complete". It is not Claude Code's `/loop` and not a repeating
   schedule. My first draft had read it as `/loop` wrapping, and the plan
   was reworked around a sequential runner:
   - **Finished** means the pane's `AgentState` goes working → idle.
   - **Loop options:**
     - Same session or a fresh one per prompt.
     - A pause between prompts.
     - On failure: stop or skip.
     - Max wait.
     - Pause and resume across a manager restart, with a `Next` resume index
       written before each send.
3. **Precedence:** the batch wins per field; overridden prompts wear ✱.
4. **Between command** (raised by the user mid-turn): "we could allow a
   command between each loop iteration".
   - It is one line (`/clear`, `/compact`, `/sess-save`, plain text), waited
     on like a prompt.
   - A command that never goes *working* within about 2s counts as instant.
   - It is optionally run after the last prompt too.
   - With fresh sessions it goes to the session that just finished.

**Phases:**

1. Compose and drop now.
2. Schedule.
3. Loop.

## Phase 1, as built

**New files:**

- `batch.go`:
  - **Types:** `Batch`, `BatchItem` (a title snapshot so the history still
    reads after edits), `BatchRun` (`Items []int`, `Where`, `Err`), and
    `batchTarget` (Schedule's destination fields, minus the non-omitempty
    `At`).
  - **`batchStore`:** `batches.json` beside `todos.json`, with the store's
    reload-then-write and temp-and-rename discipline.
  - **`overlaySession`:** batch fields win. The context mode and its
    argument travel as a pair. The two bools can only be turned on.
  - **Other helpers:** `overriddenFields` (the ✱) and `combinedPrompt`
    (`## 1. <title>` sections under an intro line).
- `batchrun.go`: `launchBatch` re-reads every item from its backlog and
  records why it skipped any closed or deleted one. Each step is exactly the
  `pendingAction` `chooseTarget` would build, chained through `batchStepMsg`
  under `m.dropping`.
  - The record is written as *running* before the first step and after
    every step.
  - Each success marks its prompt done, and a failure doesn't stop the chain.
  - `batchStatus` also reports on the Next List or Batches page when that is
    what's on screen.
- `batchcompose.go`: the composer (`stageBatchCompose`).
  - **Pick pane:** Backlog and Next List tabs (`ctrl+g`), a new
    `fuzzyList.checkboxes` mode, and an all line (`ctrl+a`, which respects
    the filter).
  - **Batch pane:** reorder by drag, `alt+↑/↓`, `x` to remove, `s` to sort
    A→Z once.
  - **Settings:**
    - **Name** and **Deliver**.
    - **Target:** the ordinary picker under a `pickForBatch` flag.
    - **Session:** the ⚙ panel under a `sessForBatch` flag.
  - **Layout:** `batchGeom` is one layout read by both the view and the
    pointer. The panes sit side by side at 100 columns or more and take
    turns below that.
  - **Leave guard:** a second press is needed to leave with picks.
  - **Next List items:** turned into backlog prompts at drop time
    (`nextItemAsPrompt`), reusing an open copy when there is one.
- `batches.go`: the Batches page (`stageBatches`).
  - **Rows:** ✓ / ⚠ / ✗ / ▶, newest first with a running batch on top, cut
    to the pane.
  - **Actions:** ＋ New, ⧉ Duplicate (the still-open prompts, with the
    batch's settings), ✖ Delete (two presses).
  - **Record view:** `stageBatchView`, each prompt with where it went or why
    not.

**Wiring:**

- **`ui.go`:**
  - Three stages and the model fields.
  - `batchStepMsg` handled above the stage switch.
  - Drag motion and release in the composer.
  - Forwarding, mouse routing, mouse mode, render and resize.
  - `ctrl+k` on the list.
  - `dropSubject`, `chooseTarget`, `leaveTarget` and `viewTarget` taught
    `pickForBatch`.
  - `closeSession` and `viewSession` taught `sessForBatch`.
  - `backToList` clears both flags.
  - `ctrl+k batches` added to both footers.
- **`nextmenu.go`:**
  - `addNextItem` split into `saveNextItem`, which returns an error, so the
    composer can report on its own line.
  - New **⧉ Batch…** row.
- **`nextlist.go`:** `ctrl+k`, and `ctrl+k batch` in the footer.
- **`listmenu.go`:** a **⧉ Add to batch…** row (**⧉ Batch N prompts…** with a
  selection), dim on a done, frozen or info prompt. It passes the row as an
  extra preset (`beginBatchesWith`), so abandoning the composer leaves the
  list selection untouched.

**Where it departs from the plan** (the plan's "Phase 1 as built" section has
the reasons):

- **Chord:** `ctrl+k`, not `ctrl+b`. `ctrl+b` is the list's alias for
  `ctrl+space` select.
- **Target:** a picker row, not a picker plus a worktree toggle.
- **Closed prompts:** not listed in the Pick pane.
- **A→Z chip:** no chord printed on it.
- **BatchRun:** no pane or branch recorded yet.

**Tests (`batch_test.go`, 31):**

- The overlay and precedence rules.
- `combinedPrompt`, the display name, the target round trip, and store
  put / delete.
- A batch never touching `todos.json`.
- Composer behaviour:
  - Only open prompts listed.
  - Pick order is check order.
  - Select-all under a filter.
  - Reorder and sort.
  - Remove unchecks the row.
  - The drop refusals.
  - The esc guard.
  - ✱.
  - The ⚙ and picker round trips.
  - Next items becoming prompts, reusing an existing copy.
- Delivery:
  - The chain: a step per prompt, the guard held throughout, done-marking,
    a failure that doesn't stop the rest, the record.
  - Combined as one step.
  - A prompt frozen mid-compose is skipped with its reason.
  - The real step command failing without a socket.
- The Batches page:
  - `ctrl+k` opening the page or a composer.
  - Duplicate and delete.
  - Rows fitting the pane.
- Layout:
  - Every frame line fitting at 160 to 60 columns in every focus, with the
    frame exactly the pane's height.
  - A click on a drawn title toggling that title.
  - A drag reordering the batch.
  - The new stages surviving a resize.

The click test was mutation-checked: shifting the pick-row offset by one
makes it fail. `go test ./...` passes. `gofmt -l` still lists
`promptcode.go` and `ui.go`, and it did on a clean HEAD too.

**Docs:**

- **README:** a new section, **Batches: several prompts, one drop**
  (composer, delivery, precedence, what a drop does, the page and the record,
  `batches.json`), plus updates to the selection section and both menu
  diagrams.
- **`cats-todo-dev` skill:** the file map and the composer and page chords.

Not released: the version is still 0.38.0.

## Next

Closed: None. Declined: None. Raised: N-048, N-049, N-050, N-051, N-052.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
