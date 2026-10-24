# Session: the mouse wheel scrolls every windowed list

Session ID: 28863ab4-ab3e-4a65-a102-c42d4d97c774
Date: 2026-10-24

The ask, from the backlog:

> When Soft-wrap is on the Prompt list and the Next list, scroll stops working

Then, once that was fixed: "apply it to other lists". Everything is committed
with this doc. It is not released; the wheel is a new capability, so the next
release is a minor bump (v0.47.0, which the prompt list's soft wrap already
called for).

## The cause

`↑/↓` and `pgup/pgdn` were never broken. A scratch test walked a wrapped
25-prompt backlog with the arrow keys and the window followed the highlight
the whole way. The wheel was the problem, and it had never worked anywhere.

- The list, the Next List, the composer, the Batches page and the pickers ask
  for mouse reporting (`View`: all motion on the first three, cell motion on
  the rest), for clicks, drags and hover cards.
- A terminal reporting the mouse hands the wheel to the program as button 4/5
  presses instead of scrolling for it. cats' encoder does exactly that
  (`~/projs/go/cats/internal/inputenc/encoder.go`, `wheel`).
- Nothing in cats-todo handled `tea.MouseWheelMsg`. It fell through to the
  focused text input, which ignores it.

Unwrapped, one row per prompt mostly fitted the pane, so nobody needed the
wheel. Soft wrap made every row as tall as its text, the list outgrew the
pane, and the wheel was the obvious way down it.

## The fix

**`fuzzyList.scrollBy(n)`** (`fuzzylist.go`) moves the window's `top` by `n`
rows, clamped to `[0, maxTop()]`. The cursor moves only when it fell off the
window: onto the first selectable row in the window when it fell off the top,
the last when it fell off the bottom. The page moves under a still highlight,
the way a pager scrolls, and the renderers' invariant (the highlighted row is
always a drawn one) holds.

- It deliberately does **not** call `ensureVisible` afterwards. In the line
  window, `ensureVisibleLines` pulls a window whose top row is the cursor up
  onto that row's section heading. A wheel step down that dragged the cursor
  onto the top row would be taken straight back, and the wheel would stick at
  every heading. The window `scrollBy` leaves already satisfies everything
  else `ensureVisible` enforces.
- **`maxTop()`**: for the item window it is `len - maxRows`. For the line
  window it is the first row from which everything to the end fits in
  `maxLines`, the same last page `ensureVisibleLines`' pull-up clamp leaves.
  `TestMaxTopIsTheLastPage` pins the two together. A last row taller than the
  whole window is a page of its own.
- One **row** per notch, not one line: `top` is a row, and a wrapped row is
  never drawn half scrolled off, so a tall prompt is one notch.

**`wheel.go`** (new) routes the wheel per stage, from a `tea.MouseWheelMsg`
case in `Update`:

| Stage | Handler | Swallowed while |
|---|---|---|
| list | `wheelList` | list menu, flag note pad, row drag |
| Next List | `wheelNext` | Next menu, note pad |
| Batches page | `wheelBatches` | Batches menu |
| batch composer | `wheelBatchCompose` | drop dialog, row drag, splitter drag |
| `@` files | `wheelFiles` | — |
| snippets | `wheelSnippets` | — |

- The menus and pads are anchored to a row, and scrolling the rows out from
  under them would leave them pointing at a different item. A drag owns the
  pointer until release.
- The list, the Next List and the composer clear the hover card on a notch:
  it was placed against a row that has moved.
- **The composer** sends the wheel to the pane under the pointer, using the
  same `inPick`/`inBatch` test `clickBatchCompose` uses. The splitter's cells
  belong to neither pane. In the narrow layout the one pane drawn takes the
  wheel anywhere. The Batch pane keeps its own `top`/`cursor` over
  `bc.picked` rather than a `fuzzyList`, so it gets `scrollBatchPane`, the
  same rule within `ensureBatchVisible`'s range.
- **The Batches page's armed delete** needs no clearing when the highlight is
  dragged to another batch. `armDelete` is keyed by batch ID, so a second
  `ctrl+x` on a different row arms that one instead of deleting.

**Not covered:** the drop picker, the spelling panel, export and import have
no scroll window. They draw every row and the pane clips the rest, so there is
nothing for a wheel to move. Each needs a window first (N-091 – N-094).

## Tests (`wheel_test.go`)

- `TestWheelScrollsTheWrappedList`: a wrapped backlog across both scopes, with
  frozen and done rows, so it has Project/Global headings and spacers. Every
  notch has to make progress down to `maxTop` (the heading-stick regression).
  The cursor is always drawn, the frame never exceeds the pane, the wheel stops
  at the end, the last prompt is drawn, and it goes back up to 0.
- `TestWheelMovesThePageNotTheHighlight`: a notch leaves a still-drawn
  highlight alone.
- `TestWheelIgnoredUnderTheListMenu`.
- `TestWheelScrollsTheWrappedNextList`: 16 items across Open → Roadmap.
- `TestMaxTopIsTheLastPage`.
- `TestScrollByItemWindow`: the item window at the `fuzzyList` level, with
  dragging off both edges, the stop at the end, and a list with no window.
- `TestWheelScrollsTheBatchesPage` (also ignored under its menu),
  `TestWheelScrollsTheComposerPaneUnderThePointer` (each pane moves alone,
  the Batch pane's cursor is dragged, its end clamp holds),
  `TestWheelScrollsTheFilePicker`, `TestWheelScrollsTheSnippetsPicker`.

Every routing test fails with its stage's case removed from `Update`, and the
first three fail on the pre-fix `ui.go`. `go test ./...` and `go vet` are
green. `gofmt -l` flags `listhover.go`, which this session did not touch.

Not seen live in cats: the only thing the unit tests take on trust is cats
delivering the wheel as mouse events, which its encoder says it does (N-095).

## Docs

- README: a paragraph under the list's scrolling section (where the wheel
  works, how a notch moves, why it was dead, when it is ignored, which lists
  have no window), and a clause on the Next List's soft-wrap paragraph.
- cats-todo-dev skill: a `wheel.go` row in the file map.

## Next

Closed: None. Declined: None. Raised: N-091, N-092, N-093, N-094, N-095.
Deferred: None. Promoted: None. Moved: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
