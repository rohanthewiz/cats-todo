# Batches phase 3: the loop

Session: `a242a240-e8d6-4ebd-b45f-886c96bf35ea`
Date: 2026-09-25

## Ask

`/sl` loaded `2026-0925-1344-batch-scheduling-phase2` and the next-list.
Then: "do phase 3", which is N-050 and the plan's phase 3
(`ai_docs/plans/multi-drop-batches.md`). Then `/sw`.

Phase 3 as planned:

- Send the prompts in order, each when the pane before goes working → idle.
- Same session or a fresh one per prompt.
- A between command, waited on like a prompt, optionally after the last.
- Pause, max wait, on fail stop or skip.
- A resume index written before each send, so a restarted manager resumes.
- Widen `performDrop` to report the pane and branch it created.

## Design choices made here

- **The guard is held only while typing.** The plan's runner held
  `m.dropping` for a batch's whole delivery. A loop waits for hours, so it
  takes the guard only for the seconds a send takes. The waits hold nothing,
  and several loops can run at once (`m.loops`, keyed by batch ID).
- **A tick-driven poll and a pure verdict.** Each tick sends one `pane.list`
  off the UI thread. `loopRunner.judge` turns an observation into waiting,
  finished or failed, and every case is testable with a made-up pane.
- **Ownership on the record.** `LoopProgress` carries:
  - `Owner`: the pid of the manager driving the loop;
  - `Beat`: a heartbeat every minute, with a five-minute lease.

  A loop with a dead or stale owner is taken over by swap (`adoptLoops`).
  There is no separate *paused* state on disk; the page draws `‖` when nobody
  is driving the loop.
- **Every write the driver makes is a swap.** `sameRevision` compares state,
  `At` and the progress's owner, next and phase. It skips the heartbeat, so a
  heartbeat write never beats a stop. A stop from another pane therefore makes
  the driver's next write lose, and the driver stands down before typing.
- **A failed write stands the runner down too.** The runner stops, and
  adoption retries from what the file says. It never runs ahead of the file,
  which is what keeps a prompt from being sent twice.

## As built

**`drop.go`**

- `performDropAt` returns a `dropLanding` (the note, the pane, the branch).
  `performDrop` and `performScheduledDrop` keep their old shape as wrappers.
- `dropIntoNewSession` now returns the pane and branch; `dropWorktree`
  returns the branch cats resolved.

**`batch.go`**

- New values: `deliverLoop` and `batchStopped`.
- New fields: `Batch.Loop` and `Batch.Progress`, plus
  `BatchRun.Pane/Branch/Stalled`. `Stalled` means the prompt was delivered but
  the loop gave up waiting on it; it still counts as delivered.
- `sameRevision` is the swap's test, shared by `swapBatch` and `takeBatch`.
- `pendingRefs` also returns a running loop's unsent prompts, at the zero
  time. The list draws those as `⧉ queued`.

**`batchloop.go` (new)**

- **Types:** `LoopOpts`, `LoopProgress` and the phases (start, prompt,
  between, pause, finish).
- **Validation:** `normalizeLoopOpts` and `parseLoopDuration`, shared by the
  composer and the runner.
- **`judge`:** decides whether the watched pane's phase is still running,
  finished or failed.
  - *Finished:* seen working (or blocked), then idle.
  - *Between command:* still idle 3 seconds after it was sent counts as an
    instant one (`/clear`).
  - *Resumed loop:* the first idle counts as finished.
  - *Failed:* never seen working within 45s, no agent in the pane for 10s,
    the pane gone, or past the max wait.
- **Sending:**
  - `loopSendPrompt` re-reads the backlogs and skips closed prompts with the
    reason. It writes `Next` before the send.
  - `loopAction` sends continuation prompts into the loop's pane as
    existing-pane drops. In the same session, it lifts the finish and release
    out of each prompt.
  - `loopFinishText` sends that finish once, at the end.
- **Moving on:** `loopPhaseDone`, `loopPauseOrNext`, `loopEnd`,
  `loopFinishOrDone` and `loopFail`.
  - On fail: stop or skip.
  - A closed pane in a same-session loop always stops the loop, and so does a
    failed finish.
- **The tick and control:** `loopTick` handles adoption, pause ends,
  heartbeats, dispatch and the poll. `stopLoop` refuses while a prompt is
  mid-typing.

**`batchrun.go`, `batchcompose.go`, `batches.go`, `ui.go`**

- **Routing:** launch, fire and the composer's drop route loops to the runner.
  A fired loop's claim writes its driver into the record.
- **Composer:**
  - The Deliver radios are now all at once, one prompt, and loop, in order.
  - Five loop rows appear only for a loop (`setRows`): Loop, Between with a
    "☐ after the last too" checkbox on `ctrl+t`, Pause, Max wait, On fail.
  - Refusals are in words: a bad duration, a multi-line between command, and
    fresh-each into a running pane.
- **Page:**
  - New badges: ⟳ running, ‖ paused, ■ stopped.
  - Loop rows say where they stand (`on 2/5`, `pausing after 2/5`).
  - `ctrl+u` becomes ■ Stop on a running loop.
  - Deleting a loop this manager drives is refused.
- **Record view:** shows the loop's options, where it stands (Now), the Why,
  each run's pane and branch, and ⚠ on stalled runs.
- **Model:** a `loops` map and `loopPolling`. The tick calls `loopTick` last.

**Tests (`batchloop_test.go`, 15 tests, 19 cases in the judge table):**

- **Judge table:** working then idle, blocked, a closed pane, never started,
  max wait, no agent, the instant between command, and a resumed loop.
- **A full same-session run:** `Next` is written before the send, the guard is
  released while the loop waits, and later prompts go into pane 7.
- **Finishing and pacing:** the finish sent once, the between command and the
  pause (including after the last), and prompts closed meanwhile being
  skipped.
- **On fail:** stop on a stuck prompt, skip a failed send, a closed pane under
  same session versus fresh each.
- **Resume and stop:** resuming after a dead owner (and not while the owner is
  alive), standing down after a stop from another pane, and Stop from the
  page.
- **The rest:** `sameRevision` ignoring the heartbeat, queued refs, a fired
  loop's claim, the composer's loop rows, and the validator.

Mutation checks:

- Idle-means-finished without seeing *working*: three loop tests fail.
- Dropping the progress comparison from `sameRevision`: its unit test fails.

The composer, page and record were checked by eye at 120 and 80 columns.
`go test ./...` and `go vet` pass. `gofmt -l` still lists only
`promptcode.go` and `ui.go` (N-052).

**Docs:**

- **README:** a new **Looping a batch** section. The composer diagram, the
  Deliver list and the page badges were updated too.
- **Plan:** a "Phase 3 as built" section with the departures above.
- **`cats-todo-dev` skill:** a `batchloop.go` row, the drop.go note, and the
  chords.

Not released: the version is still 0.38.0, and N-051 now covers phases 1–3.
Nothing has driven a real loop yet (N-056).

## Next

Closed: N-050. Declined: None. Raised: N-056, N-057.
Deferred: None. Promoted: None.
Updated: N-051. Full list: `ai_docs/todo/next-list.md`.
