# Session: ☑ Soft wrap on the editor's toolbar and the Next List's bar

Session ID: b1151eb2-71be-41b6-bf60-0381ed050322
Date: 2026-10-23
Not released. Under the release rules this is a minor bump (v0.46.0): a new
capability, a mode on two screens.

## The ask

From the user's cats-todo backlog:

> In the prompt editor and Next list can we add a Soft Wrap button to the far
> right of the toolbar?

## What it does

Both bars end in a **☑ Soft wrap** chip, flush against the pane's right edge,
with the chord `alt+z` (VS Code's Toggle Word Wrap). Each screen has its own
toggle, persisted in `settings.json`:

- `softWrap`, the editor's. Defaults **on**, which is how the editor always
  drew. Off: every line stays on one row and the box scrolls sideways after
  the caret.
- `nextListSoftWrap`, the Next List's. Defaults **off**, so the page still
  shows one cut row per item. On: each item's whole text is wrapped under its
  first line, indented to the text column.

## Design decisions

**Checkbox, not words.** The first cut said `↩ Soft wrap on`/`off`. That chip
plus its chord made the editor's toolbar 122 cells, past the 120 the form's
footer is budgeted against (`TestFormBarTiers` and
`TestFormFooterTeachesCaretKeys` failed). `☑ Soft wrap alt+z` brings the row
to exactly 118. It also borrows the retired ☑ Spell chip's grammar (box for
state, two greys `colMuted`/`colDim` for on and off, no hue because the bar's
hues mean consequence), and the box still carries the state at `tierIcons`.

**Flush right.** `rightAlignLast` moves a bar's last chip to the pane edge
when there is room for a gap. `writeChipsAt` draws the chips at their computed
columns instead of joining them with a fixed gap, so the hit test and the
drawing cannot disagree.

**The add form's scope tag moved to the title.** It used to sit after
✖ Cancel. With the wrap chip on the right, a 120-cell pane left only 4 cells
between them. It now reads `CatsTodo vX - Prompt Editor · Project backlog` (add
mode only; an edit still names no scope). `ctrl+g` still flips it.
`TestFormBarShape` and `TestAppTitleLeadsBothScreens` were updated, and the
README's Autosave paragraph now says the scope "leaves the title".

**The editor's no-wrap is a widened textarea plus a crop.** Bubbles v2's
textarea cannot switch its wrap off. The wrap underlies its caret motion,
`LineInfo` and scrolling, and every display-line table built on them
(`promptLines`, `promptDisplayLines`), which in turn drive the selection,
carets, spell marks, code paints and clicks. So `sizePromptBox` gives it
`noWrapWidth` (2048) cells, lifting `MaxWidth` and restoring the library's
default of 500 when wrapping. Every logical line is then one display line and
all of that keeps working unchanged. Showing a window of it took three pieces:

- `cropPromptLine` keeps the gutter plus cells `[gutter+xoff, gutter+xoff+cells)`.
  It is hand-written because `ansi.Cut` uses `TruncateLeft`, which drops every
  escape before the cut and so loses the caret row's CursorLine field. It
  keeps all zero-width sequences and filters only printing cells. A
  double-width glyph straddling an edge becomes the spaces it covers.
- `followPromptCaret` is Update's last step (wraps `watchAutosave`). It keeps
  `promptXOff` on the caret using `LineInfo().CharOffset`, with
  least-movement scrolling (`promptXOffFor`), and zeroes the offset while
  wrapping. The offset also resets when a form opens.
- `promptTextX` shifts pointer x by the offset at the four entry points
  (click, sweep, alt+click, right-click's spell ask). Gutter clicks are left
  alone.

Known limit: a line longer than 2048 cells still wraps there.

**Next List wrap lives in fuzzyList.** `listItem.more` holds continuation
lines. `rowsView` draws them indented to the name's column and lit with the
row. `rowAtLine` maps every one of them to the row. `fuzzyList.maxLines` is a
line-counted window that replaces `maxRows` when set; `windowEnd` and
`ensureVisibleLines` handle it, and a section heading comes along with the
first item under it when both fit. `nextPage.rebuild` wraps with
`wrapText` (Wordwrap + Hardwrap, the textarea's two passes). The overflow
counts' 8-cell reserve is settled by building once without it and again with
it if the lines overflow. pgup/pgdown step by the rows the window holds.
Other lists are untouched (no `more`, no `maxLines`).

## Files

- `softwrap.go` (new): constants, `sizePromptBox`, `promptBoxWidth`,
  `promptTextCells`, `promptTextX`, `followPromptCaret`, `promptXOffFor`,
  `cropPromptLine`/`cropPromptView`, `softWrapAction`, `softWrapKey`,
  `rightAlignLast`, `writeChipsAt`, `toggleSoftWrap`, `toggleNextWrap`.
- `ui.go`: model fields (`softWrap`, `promptXOff`, `nextSoftWrap`), prefs
  load, `formActionWrap` chip/click/key, footer chord legends, the title's
  scope, `sizePromptBox` in `newFormInputs`/`applySizes`.
- `promptsel.go`: `promptEditorView` = `paintedPromptView` + crop.
- `promptmenu.go`: right-click spell ask goes through `promptTextX`.
- `fuzzylist.go`: `more`, `maxLines`, `itemLines`, `windowEnd`,
  `ensureVisibleLines`, `setMaxLines`, continuation rendering and hit-testing.
- `nextlist.go`: `wrap`, `rowItems`/`listLines`/`wrapText`, the line window in
  `resize`, `nextActionWrap`, key, footer, page step.
- `settings.go`: `softWrap` (default true), `nextSoftWrap`; file keys
  `softWrap`, `nextListSoftWrap`.
- Tests: `softwrap_test.go` (11 tests), plus updates to `formmouse_test.go`
  and `apptitle_test.go`. `go test ./...` is green.
- README: title/toolbar paragraphs, the Next List bar sample, and a new
  `## Soft wrap` section. The cats-todo-dev skill's file map and chord table
  list `softwrap.go` and `alt+z`.

## Not done

- No live run in cats. That check is N-087.
- `listhover.go` is not gofmt-clean, but that was already true before this
  session and it was left alone.

## Next

Closed: None. Declined: None. Raised: N-087, N-088.
Deferred: None. Promoted: None. Moved: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
