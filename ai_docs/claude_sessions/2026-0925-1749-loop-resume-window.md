# Batches: a loop left over an hour is stopped, not resumed (N-057)

Session: `902e2f09-7e49-49aa-b184-d0ea922b3b16`
Date: 2026-09-25

## Ask

1. The Next List item N-057 was pasted. A loop orphaned by a closed manager
   was resumed by the next manager opened on its backlog, however long ago
   that was. It suggested a resume window, with an hour-old heartbeat meaning
   the loop is stopped with the reason and ⧉ Duplicate is the way to go on.
   That was built.
2. `/sw`.

## The resume window

Two parts, both in `batchloop.go`:

- **`loopResumeWindow = time.Hour`**, beside `loopBeat` and `loopLease`.
- **`loopLapsed(p, now)`** says whether a loop's heartbeat (`Progress.Beat`)
  is past the window and why, in the words the record keeps. A record with no
  heartbeat counts as lapsed. Only a hand edit leaves one, and nothing on it
  shows the loop was driven recently.

It is checked in three places:

- **`adoptLoops`**: an orphan past the window goes to **`expireLoop`**
  instead of being taken over. That is a `swapBatch` to `stopped`, with no
  owner and no pane, as `stopLoop` leaves a loop. The status line says why.
  - This check now runs **before** the `m.client == nil` return. Stopping
    sends nothing, so a manager outside cats expires the loop too, and the
    page stops calling a days-old loop "paused".
- **`loopTick`**: the driving manager's **own** runner is checked too. With
  the laptop asleep and the manager open, the manager's own beat goes stale.
  Its first tick back stops the loop through `loopClose`. Without this, the
  result would depend on which manager woke first. Another manager on the same
  backlog would see the stale beat and expire the loop, while the owner would
  carry on.
- **`loopPolled`**: skips a lapsed runner. A pane.list answer in flight across
  the sleep can arrive before the tick, and judging it would send the next
  prompt into a loop the tick is about to stop.

The "prompt mid-send, no run" marking moved out of `adoptLoops` into
**`markLoopInFlight`**, which the take-over and the expiry share. An expired
loop's in-flight prompt is also recorded as unknown ("look in its pane").

Design choices:

- **Stop rather than ask.** A stopped record is reversible: ⧉ Duplicate
  starts a batch from the prompts still open, since the ones that landed were
  marked done. Asking would need a modal the tick raises by itself.
- **An hour, not the schedules' two minutes.** The loop was already running.
  The user set it off, and reopening a manager closed by accident is worth
  carrying on from. An hour is also well past `loopLease` (5 minutes), so a
  take-over is always tried first.
- **Measured on the heartbeat, not the phase.** A prompt that runs for five
  hours in an open, awake manager keeps beating every minute and is never
  touched. Only a stretch with no driver counts.

`batches.go`: the record view of an undriven running loop now says until
when it can be picked up ("… picks it up until 15:05, then stops it").

## Tests

`batchloop_test.go`, 3 new:

- `TestLoopPastTheResumeWindowIsStopped`: the owner is alive but stale, and
  there is no client. The loop is stopped with "not resumed", Next is kept,
  there is no owner or pane, and the in-flight prompt is marked unknown.
- `TestLoopInsideTheResumeWindowIsResumed`: just under the hour, it is
  adopted.
- `TestOwnLoopLapsedBySleepIsStopped`: a poll across the sleep sends nothing,
  and the first tick back stops the loop.

`TestLoopOnFail`'s stop case jumped the clock two hours with no tick between,
which now reads as a lapsed loop. It now uses a 20m max wait and polls at
30m, inside the window, with a comment saying why.

`go test ./...` and `go vet` pass. `gofmt -l` names only `promptcode.go` and
`ui.go`, which this session did not touch.

**Docs:**

- README: a paragraph after *It survives the manager closing*: **It is picked
  up again only within the hour.**
- `batchloop.go` file comment: the window in *Surviving a closed manager*.
- The dev skill: the `batchloop.go` row names `loopResumeWindow` and
  `expireLoop`.

Not released. N-059 now includes this change. The window hasn't been
seen in a live cats pane; N-056's checklist gained that case.

## Next

Closed: N-057. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: N-056, N-059. Full list: `ai_docs/todo/next-list.md`.
