# Session: a draggable splitter in the batch composer, and checkboxes you can see

Session ID: 5e96fc1e-8cdd-46c0-b966-cbbac1da54d7
Date: 2026-10-07
Unreleased. Both changes are on `main`; the next release is a **minor** bump
(the splitter is a new capability).

## The asks

> In batch mode make the divider between the left and right side a draggable splitter

> also generally all the checkboxes are really small

## 1. The composer's splitter

The Pick and Batch panes used to split at a fixed 45 % (`batchGeom`). Now the
`│` rule between them is a handle.

- **Hit area.** All three cells of `batchPaneSep` (`" │ "`), on any body row
  (`srcTabsY ≤ y < barY-1`). One column is a small target for a pointer, and
  those cells belonged to neither pane before, so nothing else lost a click.
  It is checked in `clickBatchCompose` before the panes.
- **Drag.** `batchSplitOver` puts the `│` under the pointer: the Pick pane
  becomes `x-1` wide, clamped to `batchPickMinW` (24) and to
  `avail - batchPaneMinW` (36, because the settings rows spend 14 cells on
  indent and label). Both panes are re-fit on every move
  (`rebuildBatchPick` cuts titles to the width, then `sizeBatchCompose`).
- **Feedback.** The rule is drawn in `promptStyle` (accent green) while held.
  There is no hover cue, because cell-motion mouse mode reports no motion
  without a button down.
- **Persistence.** The share is a fraction on the model (`m.batchSplit`), not
  on `batchComposer`. The composer is rebuilt on every visit, but the split is
  a preference. `endBatchSplit` saves it load-modify-write to settings.json as
  `batchSplit` (0 or absent = default 0.45; a hand-edited value outside (0,1)
  reads as the default). A fraction rather than a column, so one setting is
  right in a 100-column pane and a 220-column one. `batchPickWidth` clamps
  at draw time, so a narrow pane borrows a clamped width without overwriting
  the chosen share.
- **Round trip.** `batchPickWidth` rounds (`+0.5`) so the share stored for
  column `x` draws the rule back at exactly `x`.
- **A lost release.** A key ends the drag (the list's rule) and keeps the
  split for this run, but does not save it: a missing release is not a
  decision.
- The narrow layout (< `composerSplitMin`, 100) has no splitter; the panes
  take turns as before.

Wiring: `ui.go` routes `MouseMotionMsg` / `MouseReleaseMsg` to
`batchSplitOver` / `endBatchSplit` when `m.batch.splitDrag` is set.

Tests (`batch_test.go`): `TestComposerSplitterDrags` (press on the rule's
left blank, rule drawn at the pointer, both limits, saved share) and
`TestComposerSplitShareSurvivesARelaunch` (a fresh model at 160 columns opens
at 0.6 of 157 = 94).

## 2. Checkboxes: `☐ ☑ ☒` → `[ ] [x] [-]`

The memory note says chrome bugs usually live in cats, so I checked catway's
painter first. `18-render.js` draws cells with
`fillText` in `${FONT_PX}px ui-monospace, Menlo, monospace` (14px default,
line 1.25, cell 8.43 × 18). I measured candidates in a scratch page using that
exact font stack in Chrome:

| glyph | width | drawn height (M = 10.2px) |
|---|---|---|
| `☐ ☑ ☒` | 1 cell | 6.7–7.2px |
| `□ ■ ▣ ▢` | 1 cell | 8.3px |
| `◻ ◼`, `⊡ ⊠ ⊟` | 1 cell | 7.1–7.3px |
| `[ ] [x]` | 3 cells | 12.5px |
| `⬜ ✅ 🔲` emoji | ~2.14 cells | 20px (overflows the 18px row) |
| `🗹 🗷` | — | tofu (no font has them) |

The ballot boxes come from a fallback symbol font and draw smaller than the
letters of their own label. The cell is 8.4px wide, so no one-cell glyph gets
much bigger than `□`. The user chose brackets over the geometric swap (+18 %)
and over painting the ballot boxes as vector shapes in catway (still capped at
one cell's width).

- `styles.go`: `checkOff`/`checkOn`/`checkSome` and `checkBox(on)`, with the
  measurements in the comment. Every surface uses them: the composer's Pick
  pane (`fuzzyList.checkboxes`), its "all" line (`[-]` for some), the loop's
  "after the last too", the annotation bar, the list's context menu, the
  spelling panel's toggle.
- Three cells, like the radios `( )`/`(•)`, so the list menu's labels now all
  start in one column.
- **Annotation bar:** every tier is +6 cells (three checkboxes): 156 / 143 /
  110 / 90 / 73 / 64 / 53 / 29. The bare tier is now 29, **one cell inside**
  the 30-column floor `TestAnnotBarFitsNarrowPanes` pins, so a wider box would
  need that tier rethought. Comments and the README table are updated. At 100
  columns the bar still keeps its group labels (the legend tier is 90).
- The composer's Pick pane title budget (`room` in `rebuildBatchPick`) now
  takes the box width from `lipgloss.Width(checkOff)+1` instead of a literal 2.
- The tests that asserted the old glyphs now use the constants. All width and
  fit tests passed unchanged.
- README: the composer and list-menu diagrams were re-aligned (2 cells per
  boxed row), and a paragraph by the annotation bar says why the boxes are
  brackets. Historical comments about the retired "☑ Spell" chip were left as
  they were.

## Not done

- No live run in cats. `catctl probe` cannot send pointer motion, so the drag
  needs the Chrome road (N-083).
- No release was cut.

## Next

Closed: None. Declined: None. Raised: N-083, N-084.
Deferred: None. Promoted: None.
Updated: N-039 (the checkbox words now need 110 cells, not 104).
Full list: `ai_docs/todo/next-list.md`.
