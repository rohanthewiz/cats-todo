# Setting the caret is linear in the prompt's rows (N-063)

Date: 2026-09-26

## Ask

The Next List item N-063 was pasted. `setPromptCaretOffset` hopped one
`CursorDown` per logical row. Each hop ran the library's `repositionView` →
`cursorLineNumber`, which hashes every row above the caret through the wrap
memo, so the walk was quadratic in the line count. A caret to the end of a
9,999-line prompt took ~30s. N-024 had fixed the per-display-line half of
this cost, but not the per-row half.

## Measured first

Before the change, with the caret moved to the end: 1,000 lines 343ms, 2,000
lines 1.26s, 4,000 lines 4.98s. Each doubling of the rows took about four
times as long, so the walk was quadratic.

## The fix

The caret is no longer walked. The library can't set the row directly, but
its insert leaves the caret at the end of what it inserted. So
`setPromptCaretOffset` (`spellpanel.go`) rebuilds the value around the
offset:

1. `SetValue(suffix)`, whose `Reset` also sends the viewport to the top
2. `MoveToBegin`
3. `InsertString(prefix)`, which leaves the caret exactly at the offset
4. `SetHeight(Height())`, only to reach the unexported `repositionView` once

Every step is linear, and only step 4 runs `cursorLineNumber`.

Why N-024's objection to a rebuild does not apply here: N-024 rejected one
because it "truncates differently at the 10,000-line cap". That rebuild
changed the value. This one puts back exactly the lines that were there, so
`len(value)+len(lines)-1` never exceeds the cap. Running the sanitizer again
does nothing, since every rune already went through it on the way in. The
editor has no `CharLimit` or `MaxContentHeight`.

After: 1,000 lines 1.6ms, 4,000 lines 5.5ms, 9,999 lines ~10ms. Enter on a
20k-rune line (N-024's benchmark) is 1.17ms, down from ~1.5ms.

## A scroll difference, found by the oracle test

The first version of the scroll-parity test passed even with the reposition
removed. The viewport clamps a scroll to the content its last `View()`
loaded, so a model that has never rendered cannot scroll. The test now
renders between moves, the way the app does.

Once the test rendered, it found a real difference. The old walk ended with
a bare `SetCursorColumn`, which does not reposition. So a caret placed deep
inside a wrapped row was scrolled for the row's first display line, and
could stay off screen until the next key went through the textarea's
`Update`. The rebuild repositions last and scrolls to the caret's own
display line. The oracle is therefore "the walk, then one repositionView".
Without step 4 the test fails 28 times.

## Tests

In `promptlimit_test.go`:

- `TestPromptCaretOffsetIsLinearInRows` moves the caret to the end of 9,999
  lines. It checks the row and column and fails over 1s. It runs in ~10ms.
- `TestPromptCaretOffsetKeepsTheValueAtTheCap` uses a prompt of exactly
  10,000 lines and moves the caret to the start, the middle and the end. The
  value must come back whole and the offset must read back.
- `TestPromptCaretOffsetScrollsLikeTheWalk` checks 7×7 from/to pairs, over
  plain and wrapped rows. The caret and `ScrollYOffset` must match the old
  walk (`hopCaretTo`, kept as the oracle) followed by a settle.

The comment on `promptcap_test.go`'s workaround was updated. `go test ./...`
and `go vet .` pass, and so do the caret tests under `-race`.

**Docs:** no README change. The fix is only about speed, apart from the
scroll improvement above.

## Found along the way

A click in the prompt has the same cost. `placePromptCursor` builds its
display-line map with `promptLines`, which walks every display line with
`CursorDown`: 1,000 lines take ~0.35s and 2,000 lines ~1.3s. That fix needs
each display line's start, not just a caret move, so it is raised as its own
item.

## Next

Closed: N-063. Raised: N-068. Full list: `ai_docs/todo/next-list.md`.
