# Session: the hover card in the batch composer's Pick pane, except on the checkbox

Session ID: cfc0d91d-3641-4c8d-9200-3361a4b12d40
Date: 2026-10-07
Released as **v0.44.0**. The minor bump covers this and the previous session's
splitter (`2026-1007-0200-batch-splitter-and-bracket-checkboxes`). Both are new
capabilities.

## The ask

> In batch mode still allow the popup cards on hover of a backlog item. Don't
> popup if I am exactly over the `[x]` checkbox

## Why there was no card

The composer (`stageBatchCompose`) asked for `MouseModeCellMotion`, which
reports motion only while a button is held. The list and the Next List page
ask for `MouseModeAllMotion`, and their cards are built from that idle motion.
So the composer never heard a pointer at rest.

## What changed

- **`batchhover.go`** (new). It is `hoverMotion`'s twin for the Pick pane and
  uses the shared hover state (`m.hover`, `m.hoverPend`, the warm window).
  - `batchHoverRow(x, y)` is the Pick pane row under the pointer. It reads
    `batchGeom`, the same geometry the view and `clickBatchCompose` use. It
    answers *no row* for the Batch pane, the splitter, the tabs/query/"all"
    lines, headings, and **the checkbox cells**
    `[indentWidth, indentWidth+lipgloss.Width(checkOff))`, which are cells 2–4.
    The width comes from the glyph, so a restyled box moves the dead zone with
    it.
  - Landing on the box goes through `hoverMovedOn`, like crossing a heading:
    the card comes down and the warm window stays as it was. Moving on to the
    same row's title is then an ordinary arrival.
  - `batchHoverMotion` refuses a card under the drop dialog, during a row
    drag, and during a splitter drag.
  - `batchHoverDwell` re-runs `batchHoverRow` on the pending (x, y) when the
    tick lands. A resize or splitter drag can move a box under a still
    pointer.
  - `batchCardFor`: a backlog candidate gets `buildHoverCard` on the todo as
    the store has it now (`m.resolve`). A Next List candidate gets the Next
    List card. Both tabs of the Pick pane therefore show the card their own
    page would.
- **`nexthover.go`**: `nextCardFor`'s builder is split out as
  `nextItemCard(it, row, x, y)`. The composer keeps its own copy of the
  items, so it needs an item-based entry point.
- **`listhover.go`**: `hoverDwell` sends `stageBatchCompose` to
  `batchHoverDwell`. The stage stamp on `hoverPending` already keeps a dwell
  from one page from landing on another.
- **`ui.go`**:
  - The composer joins the list and the Next List in asking for
    `MouseModeAllMotion`. All motion includes held-button motion, so row
    drags and the splitter still work: they are matched first in the
    `MouseMotionMsg` case.
  - Idle motion on the stage goes to `batchHoverMotion`.
  - The card is composited under `overlayDropConfirm`.
- **`batchcompose.go`**: `updateBatchCompose` calls `clearHover()` first.
  The list's key teardown lives in `updateList`, so it never reached this
  stage. A click was already covered, since `updateMouse` clears before
  dispatching. The view's comment about the splitter having "no hover" is
  rewritten: the stage now hears motion, but the rule still lights only
  while held, so it does not flicker on every sweep between the panes.
- The Batch pane (right) gets no card. Its rows are the picks the left pane
  already described, and a press there starts a drag.

## Tests

`batchhover_test.go`:

- `TestComposerHoverCardShowsThePrompt`: the composer asks for all motion;
  resting on a title builds the card, and it is drawn.
- `TestComposerHoverSkipsTheCheckbox`: each of the three box cells gives no
  card and arms no dwell. The cells either side (the cursor gutter and the
  space before the title) still get one. Moving onto the box takes a
  standing card down.
- `TestComposerHoverYieldsToTheHand`: a key, a click, crossing to the Batch
  pane, and the drop dialog.
- `TestComposerHoverDwellChecksTheBox`: a dwell whose pointer is on the box
  when the tick lands builds nothing.

The Next List tab's card has no test of its own. It is `nextItemCard`, which
the Next List tests already cover through `nextCardFor`.

README: the composer section gains a paragraph on the card and the
checkbox exception. The "Mouse reporting" sentence and the hover card's
cost paragraph now name the composer. The `cats-todo-dev` skill's file map
has a `batchhover.go` row.

## Not done

- No live run in cats. Pointer motion needs the Chrome road, so it is folded
  into N-083.

## Next

Closed: None. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: N-083 (now also live-tests the composer's hover card and its
checkbox exception). Full list: `ai_docs/todo/next-list.md`.
