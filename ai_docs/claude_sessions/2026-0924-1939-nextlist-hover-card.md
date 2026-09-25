# Next List hover card

Session: `4cca603f-32d7-4f26-98ff-3eebe8115f0b`
Date: 2026-09-24

## Ask

"Can we do the same popup cards on the Next List screen as we have on the
main Prompt list? Just include more info on the card — maybe no more than 7
rows." Then `/sw`.

## The card (`6cbcde9`)

Rest the pointer on a Next List item and a card floats below and right of it:

```
╭────────────────────────────────────────────────────────────╮
│ N-001 · Open                                               │  ID · section
│ Hands-on pass in a rebuilt Cats.app. Merged from checks:   │  text, ≤ 5 lines,
│ - hover cards: the 400ms dwell;                            │  line breaks and
│ - DEC 1004: blur a window.                                 │  sub-bullets kept
│ value medium · raised 2026-0904-1753-a-dwell               │  header fields
╰────────────────────────────────────────────────────────────╯
```

- **Seven rows at most** (`nextCardMaxRows`): the ID row, up to
  `nextCardBodyLines` = 5 wrapped text lines (the last ends in `…` when cut),
  and one fields row. Each part drops out when it has nothing to say.
- **Every item gets a card**, unlike a backlog prompt whose body is only its
  title line: the raised stem and the value in words are never on the row.
- **62 cells wide** (`nextCardWidth`), 10 more than the backlog card, since
  an item is prose with no title. It narrows with the pane, down to
  `hoverCardMin`.
- The section rides on the ID row because the section heading has usually
  scrolled away in a long list.

### Wiring

- `nexthover.go` (new): `nextHoverMotion`, which works like `hoverMotion`
  (same row: nothing; the row being waited for: follow the pointer; new row:
  warm → card now, cold → arm the dwell), plus `nextCardFor`,
  `nextCardLines` and `nextCardBody`.
- **Shared state, not a copy.** The card reuses `m.hover`, `m.hoverPend`,
  `m.hoverGen` and `m.hoverWarmUntil`, since only one of the two pages is ever
  on screen. `hoverPending` gained a `stage` field. `hoverDwell` drops a tick
  armed on the other page and sends the Next List's ticks to `nextCardFor`.
  Without the stage check, a dwell armed on the backlog that landed after
  `ctrl+g` would build a card from the list's row index against the page's
  rows (pinned by `TestNextHoverDwellIsStageBound`).
- `ui.go`: idle motion on `stageNextList` goes to `nextHoverMotion`. The page
  moved from `MouseModeCellMotion` to `MouseModeAllMotion`, and
  `renderStage` composites `overlayHoverCard` over `viewNextList`.
- `nextlist.go`: `updateNextList` and `clickNext` call `clearHover()` first.
  Every way off the page is a key or a click, so those two cover leaving it as
  well. A resize and a lost focus already cleared it globally.
- Tests (`nexthover_test.go`): the content, the 7-row cap and opaque row
  widths, the teardown by a key, a click and moving onto the bar, the stale
  dwell across stages, and the mouse mode.

README: a new part at the end of **The Next List**, and the hover-card and
mouse notes now name both pages. The dev skill's file map lists
`nexthover.go`.

Not released. v0.37.0 would be a minor bump, since it is a new capability
(N-042).

## Next

Closed: None. Declined: None. Raised: N-041, N-042.
Deferred: None. Promoted: None. Updated: None.
Full list: `ai_docs/todo/next-list.md`.
