# Double-click selects a word in the prompt editor

Session: `ad8078c6-a069-4745-9d2f-ad475e3d52de`
Date: 2026-09-26

## Ask

"In the prompt editor, dbl clk'g on a word should select the word."

## 1. Pairing the presses

Terminals report presses one at a time with no click count (the list and the
Next List already pair row clicks themselves with `doubleClickWindow`, 500ms).
The editor now does the same, but on a *character*:

- Two new model fields, `promptClickAt` / `promptClickOff` (`ui.go`), record the
  last editor press: when, and the caret offset it produced. They are kept apart
  from the list's `lastClickRow`/`lastClickAt` so a list click followed by an
  editor click can never add up to a double.
- `promptDoubleClick` (`promptsel.go`) is asked right after
  `placePromptCursor` in `clickForm`'s editor case. It matches on the **caret
  offset**, not the raw cell: two presses a cell apart on the same letter are one
  gesture, and two presses on the same cell after a scroll are not.
- A pair that fires is spent (stamp cleared), so a third quick press is a plain
  click (caret, highlight gone) instead of re-selecting the same word forever.

## 2. Selecting the word

`selectPromptWord` sets the anchor to the word's start and moves the caret to
its end: the same shape a `shift+alt+→` sweep leaves. Copy, typing-over,
delete and the right-click menu all read it unchanged. Design points:

- The caret moves with `SetCursorColumn` inside its own logical row, **not**
  `setPromptCaretOffset`. A word never crosses a newline and the press left the
  caret on the visible clicked line, so the view doesn't scroll. A walk from
  the top would scroll it.
- The second press does **not** arm a sweep (`promptSelDrag = false`), so a
  jittery release can't shrink the word to one letter.

`promptWordSpan(runes, off)` is the boundary rule, pure and table-tested:

| Under the pointer | Selects |
|---|---|
| letters, digits, `_`, combining marks | the maximal run (`fooBar_2`, `wörld`) |
| an apostrophe between two word runes | part of the word (`don't`); a quote around a word is not |
| spaces / tabs | the blank run |
| other punctuation | that one rune (glued delimiters like `()` want one of them) |
| past the end of a line / the value | the word the line ends with |
| an empty line | nothing, just the caret |

## 3. Tests and docs

- `promptsel_test.go`: `TestPromptDoubleClickSelectsTheWord` (select, no sweep
  armed, caret at the word's end, survives the release, third press clears),
  `…NeedsTheSameCharacter`, `…SlowIsTwoClicks`, `…OnALaterLine`, and
  `TestPromptWordSpan` (the table above). `go test ./...` is green.
- README: a paragraph in the editor's selection section.
- The `cats-todo-dev` skill's file-map row for `promptsel.go` names the new
  functions.
- I didn't try it in a live cats pane; that check is N-064.

## Release

Not released. By the skill's rules this is a patch (v0.41.2): an existing
mode learning a gesture. I offered it and the user hasn't answered yet.

## Next

Closed: None. Declined: None. Raised: N-064, N-065.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
