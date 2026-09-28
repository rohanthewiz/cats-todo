# Batch ▶ Drop now asks first — a flow dialog in plain English

Session: `be66e6ea-c799-4c7b-abff-c1f798508d48`
Date: 2026-09-27

## The ask

> Just before running a batch, give an english summary of the exact flow in a
> confirmation dialog box

At first I read this as being about Claude Code's own batch tools (Claude Docs
`batch`, Chrome `browser_batch`). I saved a memory to confirm those with a
summary. The user corrected me: it means **cats-todo's batches**, the ones
built in the `ctrl+k` composer and sent with **▶ Drop now**. I deleted the
mistaken memory and its `MEMORY.md` line.

## What was built

Pressing **▶ Drop now** (the chip, or `shift/alt+enter`) no longer sends the
batch. It opens a modal dialog over the composer that describes the whole
flow in sentences, in order. Only the dialog's confirm drops.

### New file: `batchconfirm.go`

- `askDropBatch` is the new entry point for the chip and the chord. First it
  runs the refusals `dropBatch` would: `batchDropWhy`, then
  `batchTargetGoneWhy`. So a batch that can't go (no picks, no socket, a time
  on the When row, the target pane gone) says why on the note line, and no
  dialog opens. Otherwise it builds the summary and opens the dialog.
- `batchFlowSummary` returns a title and `[]flowLine` (lead, text, detail).
  It has three blocks:
  1. **What happens** (`flowOpening`): the delivery mode and target. That is
     how many sessions, whether worktrees, the directory (via `shortenHome`)
     or the running pane, and the rhythm. A combined drop also names its tab
     and branch (`flowDisplayName`).
  2. **The prompts**, numbered in delivery order. Under each is a detail line
     from `flowStep` / `flowSetup`. It covers the launch argv of a new session
     (`launchArgs`), what a running pane is sent first (`paneSetupCommands`,
     plus `paneUnapplied` for what can't be set there), the context command,
     extra files, the wrap-up steps (`postamble`, parsed by `postambleSteps`),
     and the image count. The options used are the prompt's effective ones:
     `overlaySession` of the prompt's own, re-read from the backlog (not the
     pick's snapshot), with the batch's options laid over them.
  3. **Rules and bookkeeping.** A loop adds the between command (and whether
     it also runs after the last prompt), the pause, the max wait or "no time
     limit", the 45s start wait (`loopStartWait`), stop or skip on a failure,
     the single wrap-up message (`loopFinishText`, same session only), and
     the 1h take-over window (`loopResumeWindow`). Every batch then lists:
     Next List IDs saved to the backlog first, an edited batch taken off its
     schedule (`flowEditState`), the record written before sending, and the
     done marks.
- It mirrors `loopAction`: a same-session loop strips each prompt's
  finish and release, and its later prompts go into the first one's pane as a
  running-pane drop of the launched agent. So their line says `first submits
  /clear, /model …`, not `starts claude …`.
- Keys (`updateDropConfirm`): `enter` presses the lit button (Drop is lit when
  the dialog opens). `y` and a second `shift/alt+enter` confirm. `esc` and `n`
  go back. `←/→`/`tab`/`h`/`l` switch buttons, and `↑/↓`, `pgup/pgdn`,
  `home/end` scroll. `ctrl+c` quits.
- Mouse (`clickDropConfirm`): a click on a button presses it, and a click off
  the box goes back (the rule menus and the flag pad follow).
- Drawing: `dropConfirmGeom` is shared by the view and the pointer, as
  `batchGeom` is. The width is ≤84 and centred. The body scrolls when it
  outruns the pane, and the hint then shows `(a–b of n)`. The styles are
  borrowed: `menuBoxStyle` box, `hoverTitleStyle` title, `nextCardBodyStyle`
  body, `hoverFieldStyle` details and hint, `menuRowSelStyle` for the lit
  button. `overlayDropConfirm` composites it with `lipgloss.NewCompositor`.
- Going back sets the note to `not dropped — nothing was sent`. Confirming
  closes the dialog and calls `dropBatch`, which runs every check again. A
  schedule may have fired meanwhile, and an edited batch's claim may lose.

### Wiring

- `batchcompose.go`: the `confirm batchDropConfirm` field on
  `batchComposer`. `updateBatchCompose` hands every key to the dialog while it
  is open. `shift/alt+enter` and `pressBatchButton(batchBtnDrop)` call
  `askDropBatch`. `clickBatchCompose` goes to `clickDropConfirm` while the
  dialog is open. `dropBatch` is now "Drop now once confirmed", and its name
  is unchanged so the existing tests still call it directly. The file comment
  mentions the dialog.
- `ui.go`: `renderStage` composites the dialog over `viewBatchCompose()`.
  `forward` drops the blink and pastes while the dialog is open, so a paste
  can't edit the draft the dialog describes.

### Tests

- New `batchconfirm_test.go`:
  - `TestDropNowAsksFirst`: the chord and the chip each open the dialog, with
    no command and no `batches.json`. `esc` and `n` each go back with the picks
    intact and say nothing was sent.
  - `TestDropNowRefusesBeforeAsking`: with no picks, it says so on the note
    line and opens no dialog.
  - `TestDropConfirmSends`: `enter`, `y` and `shift+enter` each drop, leaving
    one running record.
  - `TestDropConfirmBackButton`: `→` then `enter` goes back.
  - `TestDropConfirmSummaryAllAtOnce`, `…Loop`, `…Combined`: pin the key
    sentences. In the loop test, the lifted finish must not appear on any
    prompt's line.
  - `TestDropConfirmDrawsInsideThePane`: widths 40/80/120/200. No row is
    wider than the pane, both buttons are drawn, and clicks on Back and off
    the box land.
  - `TestDropConfirmScrolls`: a 60×16 pane, and scrolling clamps at the end.
  - `TestDropConfirmHoldsThePaste`.
- `batchsched_test.go` `TestEditLosesToAFire`: `alt+enter` now opens the
  dialog, and `enter` confirms. The lost claim (`errBatchChanged`) is found
  on the confirm.
- `go test ./...` is green, and `gofmt` and `go vet` are clean.

### Docs

- README: a new `#### Drop now asks first` under the composer section, with a
  sample dialog, what it covers, the keys, and why the text can't drift from
  the flow.
- The dev skill: a `batchconfirm.go` row in the file map, and the dialog's
  keys beside `shift/alt+enter` in the composer's chords.

## A rendered sample (110×36, loop, sonnet · high, `/compact`, pause 30s)

```
╭──────────────────────────────────────────────────────────────────────────────────╮
│ Drop “nightly cleanup” now? · 3 prompts                                          │
│                                                                                  │
│ Sends the prompts one at a time, in this order. The first opens a new Claude     │
│ Code session in ~/…; every later one is typed into that same session, only       │
│ after the one before it has finished (its agent went from working back to idle). │
│                                                                                  │
│  1. Fix flaky drop test                                                          │
│     starts claude --model sonnet --effort high                                   │
│  2. Rename headings                                                              │
│     first submits /model sonnet, /effort high · then: run /code-review           │
│  3. Global task                                                                  │
│     first submits /model sonnet, /effort high                                    │
│                                                                                  │
│  • After each prompt, submits “/compact” to that same session and waits for it   │
│    to finish too — but not after the last prompt.                                │
│  • Then waits 30s before sending the next prompt.                                │
│  • No time limit: a prompt may work for as long as it takes.                     │
│  • A prompt the agent has not started on after 45s, or whose pane closes or      │
│    whose agent exits, counts as failed.                                          │
│  • A failure stops the loop there: the prompts after it are not sent.            │
│  • This manager drives the loop. If it closes, a cats-todo manager opened within │
│    1h can take the loop over; after that it is stopped.                          │
│                                                                                  │
│  • A record of the batch is written before anything is sent and updated after    │
│    each prompt — ctrl+k shows it.                                                │
│  • Each prompt is marked done once it is delivered. One that is done, frozen or  │
│    deleted by its turn is skipped, and the record says why.                      │
│                                                                                  │
│  ▶ Drop now    ✕ Back                                                            │
│ enter confirm · esc back · ←/→ button                                            │
╰──────────────────────────────────────────────────────────────────────────────────╯
```

In this sample, prompt 2 carried its own `/sess-wrap`. The loop takes its
wrap-up only from the *last* prompt (`loopFinishText`), so that wrap never
goes out. The dialog is truthful about it: prompt 2's line omits the wrap,
and there is no wrap-up bullet. That behaviour predates the dialog, and the
dialog now makes it visible.

## Design notes

- **Derived, not written per case.** Each sentence comes from the functions
  the delivery runs, so a change to delivery changes the dialog. The one
  mirrored rule is `loopAction`'s same-session handling, and a comment in
  `batchconfirm.go` names it.
- **Snapshot, then re-check.** The summary is built when the dialog opens.
  The composer can't change under the modal, but the world can, so the
  confirm goes through `dropBatch`'s full checks.
- **The refusals come before the dialog.** Asking someone to confirm a batch
  that will be refused is a question with only one honest answer.
- **Drop is lit when the dialog opens.** The press that opened it was already
  a deliberate "drop", so `enter` or the same chord again finishes it.
- **Scheduling does not ask.** `◷ Schedule` writes a plan that can be edited
  and sends nothing, so it has no dialog. A scheduled batch firing later
  doesn't ask either, since nobody may be there to answer.

## Not done

- No live run in cats yet: the dialog is exercised against the model only
  (N-077).
- Not released. v0.43.0 is proposed (N-078).

## Next

Closed: None. Declined: None. Raised: N-077, N-078.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
