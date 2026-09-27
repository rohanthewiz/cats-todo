# Model and context fill on the drop picker's pane rows; triple-click and word/line sweeps in the prompt editor (N-065)

Session: `1edb0553-3905-42a1-b7db-4e0b5680ccd4`
Date: 2026-09-26

## Ask

1. On Send, when an agent is listed as a drop target, show its token usage /
   context size, as cats' AGENTS view already does.
2. Commit.
3. Implement N-065: triple-click selects the line; a drag started from a
   double-click extends by whole words.
4. `/sw`.

## 1 — Context fill on running-pane rows (`ui.go`, `dd63192`)

No cats change was needed. catway resolves the agent's model from its own
on-disk history (`cmd/catway/agentmodel.go`) and folds effort and context fill
into one string, `claude-opus-5 · high · 43k/1M` (`formatContext` /
`compactTokens`). It ships as `wire.PaneMeta.AgentModel` (`agent_model`) in
`pane.list`, which is in the cats revision cats-todo already pins. Only Claude
Code and Copilot resolve one, and only Claude carries the context figure.

`buildTargetsFor` now puts it into the running-pane row's `desc`:
`[idle] claude-opus-5 · high · 43k/1M · /cwd`.

- **Ahead of the cwd**, because the cwd is the long tail a narrow pane cuts off.
- **In `desc` rather than a `descMark`**, so the fuzzy filter matches it
  (typing `opus` narrows to those panes).
- The schedule and batch-target pickers use the same builder, so they show it too.
- Test: `TestRunningPaneRowShowsContextFill` (`droptargets_test.go`), covering a
  row with the model string and one without.
- README: a paragraph under the drop picker section.

## 3 — N-065: triple-click and grained sweeps (`promptsel.go`, `ui.go`)

- `promptDoubleClick` (bool) became `promptClickCount` (1/2/3). The model gains
  `promptClickN`, and `promptClickOff` is now the offset of the run's *first*
  press.
  - The second press must hit the same offset, as before.
  - The third may land anywhere in the first press's word
    (`promptWordSpan` of `promptClickOff`), to allow for drift.
  - The count cycles to 1, so a fourth press is a plain click.
  - Each press is timed from the previous one.
- `promptGrain{unit, lo, hi}` on the model (`promptSelGrain`), cleared by
  `clearPromptSel`: the unit a sweep snaps to (rune / word / line) plus the
  core the press selected.
- `selectPromptWord` and the new `selectPromptLine` both return bool and go
  through `selectPromptSpan`.
  - The line is the **logical** row (the whole wrapped paragraph), newline
    **excluded**. That keeps the caret on the clicked row (column move, no
    scroll), and typing over the line doesn't join it to the next.
  - If there is nothing to select (a line break, an empty line), the press
    falls back to a plain anchor + sweep.
- The second and third press now **arm a sweep** (`promptSelDrag = true`).
  This reverses the old "the second press must not arm a sweep", which
  existed to stop jitter shrinking the word to one letter. The grained sweep
  never gives up the core, so that reason is gone.
- `promptSelOver` calls `extendPromptSelByGrain` when the unit isn't rune:
  - Pointer before the core: anchor at `core.hi`, caret at the start of the
    unit under the pointer.
  - Otherwise: anchor at `core.lo`, caret at `max(unit end, core.hi)`.
  - The caret moves by column only. Every target offset lies on the
    pointer's row (`core.hi` is used only while the pointer is inside the core).
- Tests (`promptsel_test.go`):
  - Rewrote `TestPromptDoubleClickSelectsTheWord`: it now expects a word
    sweep to be armed, and a cell of jitter before the release keeps "beta".
  - Added `TestPromptTripleClickSelectsTheLine` (plus the fourth-press reset),
    `TestPromptTripleClickTakesTheWholeParagraph`,
    `TestPromptTripleClickToleratesDrift`,
    `TestPromptDoubleClickDragExtendsByWords` and
    `TestPromptTripleClickDragExtendsByLines`.
- README: the editor-selection paragraph now covers the triple-click and the
  sweeps. The dev skill's `promptsel.go` file-map row is updated too.

## Notes

- `gofmt -l` flags `hangup.go`, which predates this session and was left alone.
- Neither change has been exercised in a live cats pane; tests only.
- No release was cut. Both changes are candidates for the next one: the
  picker figure is a refinement (patch), and the triple-click adds a gesture
  to an existing mode (patch by the precedents).

## Next

Closed: N-065. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
