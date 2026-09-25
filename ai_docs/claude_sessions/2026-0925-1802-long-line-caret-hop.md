# Enter on a long one-line prompt is cheap again (N-024)

Session: `42ce69e3-f1d2-48a3-9e1b-73744ab56958`
Date: 2026-09-25

## Ask

The Next List item N-024 was pasted. Enter on a 20k-char single line took
about 147ms, and the item blamed `replacePromptRunes` doing a `SetValue` on
the whole prompt.

## What the profile said

The premise was off. A CPU profile of enter on a 20k-rune line
(~140ms/op) put ~98% of the edit in `setPromptCaretOffset`, and almost none in
`SetValue`. The caret walk went to the target row with `CursorDown`, which
steps one *display* line. Each step calls the library's `LineInfo`, which
looks the row's wrap up in a memo keyed by the row's runes. So every step
turned the whole row into a string and hashed it. On a 20k-rune row that is
~250 display lines × 20k runes per hash, quadratic in the row's length. After
enter the caret goes to row 1, so the walk crossed all of row 0.

## The fix

`setPromptCaretOffset` (`spellpanel.go`) hops a **logical** row per step:
`CursorEnd` puts the caret on the row's last display line, and from there the
library's `setCursorLineRelative` moves one `CursorDown` into the next row.
The cost is now one step per logical row, whatever the rows' lengths. Each
hop must leave the row it started on. If one does not, which would mean the
library's end-of-row rule changed, the hops stop and the old display-line walk
finishes the job, so the result is never wrong, only slower.

Measured result: enter on a 20k-rune line went from ~140ms to ~1.5–1.9ms.

The same walk served undo/redo (`promptundo.go`), indent (`promptindent.go`),
line moves (`promptmove.go`), the column mode (`promptcarets.go`) and the
Insert-a-prompt row (`ui.go`), so all of those are faster on long rows too.

A considered alternative was to rebuild the value as "suffix, then insert
prefix+with at the start", so the library's own insert leaves the caret in
place with no walk at all. It was not taken: the viewport would need a
separate nudge to scroll to the caret, it truncates differently at the
library's 10000-line cap, and it would fix only `replacePromptRunes`, not the
other callers.

## Tests

In `promptlimit_test.go`:

- `TestPromptCaretOffsetCrossesAWrappedRow` covers landing inside a wrapped
  row, at its end, on the row after it, and past two wrapped rows. It checks
  the row, the column, and that the offset reads back.
- `TestEnterOnALongOneLinerIsCheap` is the regression. It takes the best of
  three runs and fails over 50ms. Without `-race` it runs in ~1.5ms; the old
  walk took ~140ms. Under `-race` the test passes.
- `BenchmarkEnterOnALongOneLiner`.

`newModelInTemp` and `openAddForm` now take `testing.TB` so the benchmark can
use them.

`go test ./...` and `go vet ./...` pass.

**Docs:** no README change, since this is not a user-visible behaviour, only
speed. The fix is added to the pending release (N-059).

## Next

Closed: N-024. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: N-059. Full list: `ai_docs/todo/next-list.md`.
