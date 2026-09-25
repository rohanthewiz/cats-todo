# Redo, and the switch confirm on existing-pane drops

Session: `cc7a2239-e881-4f1f-b1a9-e3eb843fce11`
Date: 2026-09-24

## Ask

Three Next List items, done in two passes: N-027 (redo, pasted from the
list), then "Do N-018 and N-019". Then "commit and push", then `/sw`.

## Redo (`af86f25`), N-027

`promptUndo` gained a `redo` stack beside `stack`:

```
    stack (undo)                  redo
  ┌─────┬─────┐  ◀── cmd+z ──  ┌─────┐
  │ A   │ B   │  ── ⇧cmd+z ──▶ │ D   │     editor shows C
  └─────┴─────┘                 └─────┘
    a real edit from C: push C onto stack, clear redo
```

- `undoPrompt` pushes the state it leaves (text and caret) onto `redo`.
  `redoPrompt` pops it and pushes the current state straight onto `stack`.
  It does that without `push`, so a redo is never folded into a typing run.
  Both call the new `restorePromptEdit`, so they reset the same things
  (run kind, `applied`, selection, column carets).
- `commitPromptEdit` clears `redo` only when the text changed. A caret motion
  or a click keeps it. The `applied` flag already keeps undo and redo from
  recording themselves.
- `redo` has no budget of its own. Each undo or redo moves one state between
  the stacks, so together they stay inside the undo limits.
- Chords (`updateForm`): `shift+super+z`, `shift+meta+z`, `super+Z`/`meta+Z`
  (a terminal that folds shift into the key), `ctrl+shift+z` (kitty only),
  and `ctrl+y`. The textarea and textinput don't bind ctrl+y, and in raw mode
  the terminal's delayed suspend never sees it. `redoChord()` names
  `shift+cmd+z` or `ctrl+y` depending on `kbEnhanced`.
- `menuRedo` is a new ↷ Redo row under ↶ Undo on the context menu, dim with
  an empty redo stack. The footer's tail gained `… redo`.
- Title field and empty stack both refuse in words, the same way undo does.
- 6 tests in `promptundo_test.go`. README Undo section, the menu diagram, and
  the dev skill's key and file tables updated.

## N-018: `/model` mid-conversation does ask

Read from Claude Code 2.1.282's bundled JS
(`strings ~/.local/share/claude/versions/2.1.282`):

- The dialog is component `m7`. It is titled *Switch model?* or *Change
  effort level?*, with the subtitle "Your next response will be slower and
  use more tokens". It asks the Yes/No question through `zn`, focus on Yes.
- `/model <m>` opens it when `Bvn(...)` holds, and `/effort <e>` when
  `mCe(...)` holds. Both require all of these:
  - a warm cache (`UJ`/`SCt`: the last main-thread request is inside the
    TTL, 5m or 1h);
  - output since the last acknowledgement (`Cp() !== cacheMissAckedAtOutputTokens`);
  - a real change (another model than the current one and the last
    response's, or another effective effort).
- `/clear` runs `Loe`, the compaction cleanup, which sets
  `cacheMissAckedAtOutputTokens = Cp()`. So a drop with Clear on never meets
  the dialog.
- A PreModelSwitch hook returning `ask` shows the same dialog with the
  subtitle "A PreModelSwitch hook asked you to confirm".
- Typed letters don't answer the select, and Enter picks Yes. So a Clear-off
  drop's `/effort` line, or the prompt itself, went into the dialog and was
  lost, while the drop reported success.

### Fix (`c68084d`)

New `panesetup.go`, `applyPaneSetup(paneDriver, pane, cmds, settle)`:

```
dialog up, a PreModelSwitch hook asked   → stop; the drop fails and says so
dialog up, Claude Code's own cache check → press Enter (Yes), say so
no dialog within the settle time         → carry on, exactly as before
```

- It watches only after `/model` and `/effort`, and only when no `/clear`
  came first.
- It reads with `waitForOutputIn` (new on `catsClient`: `wait_for_output`
  with `Lines`), seeded from the bottom 16 rows only. The whole buffer would
  seed conversation text quoting the dialog.
- Before sending, it takes a 50ms look for the dialog's words. If they are
  already there, it can't tell an old dialog from quoted text, so it falls
  back to the old sleep and presses nothing.
- The watch is the 400ms settle, so a pane without the dialog costs the same
  as before.
- `performDrop` and `performScheduledDrop` now return `(note, error)`. The
  note travels on `dropResultMsg.note`, and `dropDoneStatus` puts it after
  the destination: `dropped → pane 3 · confirmed the model switch (the
  conversation is re-read uncached)`.
- Pressing Yes on the user's behalf was a judgement call: the todo already
  asked for that model, and the alternative makes Model useless on Clear-off
  drops into a busy pane. It was flagged to the user as a one-line change if
  they'd rather stop the drop.
- 7 tests in `panesetup_test.go` against a scripted `fakePane`.

## N-019: premise corrected

In 2.1.282, `/effort xhigh` on a model without xhigh no longer errors. The
command clamps only against a settings or org cap (`GG`). The model check
happens at request time (`D`: xhigh→high without `yme`, max→high without
`zq`). So the level is accepted, reported as set, and runs at high. The same
applies to `--effort`. A model-aware check in cats-todo was declined: the
capability table is Claude Code's, partly served at runtime. The README's
session options section now describes both behaviours.

## Committing

Both passes touched `ui.go`, the README, the skill and the next list. To
split them, the redo commit was staged by picking hunks. A first attempt
with `-U0` patches put the redo `case` inside the cmd+d case: with zero
context, `git apply` misplaced the hunk after an earlier one was skipped.
Testing the staged tree on its own (`git checkout-index` into the
scratchpad, then `go vet`) caught it. Redone with `-U3` patches, and each
commit builds and tests on its own. Pushed `main` (`8966e0d..c68084d`). Not
released.

## Next

Closed: N-018, N-019, N-027. Declined: None. Raised: N-043, N-044.
Deferred: None. Promoted: None. Updated: N-026, N-042.
Full list: `ai_docs/todo/next-list.md`.
