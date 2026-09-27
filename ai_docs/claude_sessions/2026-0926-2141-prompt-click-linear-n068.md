# A click in the prompt is linear in its lines (N-068)

Date: 2026-09-26

## Ask

The Next List item N-068 was pasted. A click in the prompt was quadratic in
its display-line count. `placePromptCursor` built its display-line table with
`promptLines`, which walked the whole value one `CursorDown` at a time. Every
step ran the library's `repositionView` → `cursorLineNumber` over all the
rows above. The placement then stepped the caret to the bottom, up to the
view's top line and down to the clicked one, which cost the same again. A
click in 2,000 lines took ~1.3s.

## The table: a probe grown one row at a time

The first try walked a copy holding a single row, with `SetValue(row)` per
row. That was linear (9,999 rows in ~230ms), but the profile was mostly GC.
Every `SetValue` → `Reset` allocates a `maxLines`-capacity grid, about 240KB.

The approach that shipped uses the fact that `InsertString` moves the caret
onto the row it just inserted and never repositions. The probe starts with a
single `SetValue("")` and then appends `"\n"+row` for each row. Each row's
display lines are read with `SetCursorColumn(start)` + `LineInfo`. Neither
repositions, and `LineInfo` looks only at the caret's own row. It reports
the line's width, and the next line starts that many runes on. A column
exactly at the end of a wrapped segment reads as the start of the next
segment, which is `LineInfo`'s own rule.

## The placement: one caret placement with the view shortened for it

A placement from the top scrolls the view just far enough that the caret sits
on its last visible line. So `placePromptCursor` sets the height to
`d-y0+1`, places the caret on display line `d` with `setPromptCaretOffset`
(linear since N-063), and puts the real height back. The view lands on `y0`.
The caret is then inside the restored view, so the second reposition moves
nothing. The viewport's own `SetHeight` does not clamp, and a shorter view
only raises `maxYOffset`, so the scroll to `y0` cannot be refused. The old
`stepPromptTo` stays as a backstop in case the landing rule ever changes.

## Found along the way: the old walk was wrong on an exactly-filled row

The click-parity test failed on one value: a 120-rune row with no spaces, at
a text width of 24. The new code was right and the old walk was wrong. A row
filled exactly to a multiple of the width wraps into full lines plus an empty
display line, which the library draws. From the row's second-to-last line,
`CursorDown` clamps the column to `len-1` and stays on the same line. The
walk took that as the end of the value. Its table was missing the empty line
and every row below it. So in such a prompt, clicks on screen rows 0 and 2
both landed on `next`, and the view jumped from 3 to 1. The probe build gets
all seven lines.

## Measured

| Case | Before | After |
|---|---|---|
| Click, 1,000 lines | ~0.35s | ~3.3ms |
| Click, 2,000 lines | ~1.3s | ~4.6ms |
| Click, 9,999 lines | (quadratic, minutes) | ~21ms |
| Click, two 20k-rune rows | ~150ms (table alone) | ~33ms |

## Tests

In `promptlimit_test.go`, with the old code kept in the test file as oracles
(`walkPromptLines`, `walkPlacePromptCursor`):

- `TestPromptLinesMatchesTheWalk` compares the tables entry for entry, and
  the index, over eight value shapes at three widths.
- `TestPromptClickMatchesTheWalk` compares caret row, column and scroll
  across views scrolled to the top, middle and bottom, every screen row
  (plus one past the end), and three x positions. It fails if the height
  trick is removed.
- `TestPromptClickIsLinearInRows` clicks in 9,999 lines. It must land on the
  right row, keep the scroll, and take under 1s. It takes ~0.2s under the
  test harness.
- `TestPromptClickReachesPastAnExactlyFilledRow` is the regression for the
  old walk's bug. Run against the old code, it fails.

The oracle widths avoid exactly filled rows, where the old walk is wrong.
`withForm` now takes `testing.TB`. gofmt flattens a diagram that directly
follows a list in a doc comment, so the `promptLines` diagram has a line of
prose in front of it. `go test ./...` and `go vet .` pass, and the click and
caret tests pass under `-race`.

**Docs:** no README change. The README doesn't describe how a click lands
the caret; this change makes it faster and fixes where it lands.

## Next

Closed: N-068. Raised: none. Full list: `ai_docs/todo/next-list.md`.
