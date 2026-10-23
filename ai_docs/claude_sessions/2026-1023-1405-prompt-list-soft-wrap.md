# Session: no hover card while soft-wrapped, v0.46.0, and ☑ Soft wrap on the prompt list

Session ID: fc1ca611-76fe-4cd6-82bf-150ba4ada546
Date: 2026-10-23

Three pieces of work, in order:

1. `9dcf7a3` feat(next): no hover card on the Next List while soft-wrapped
2. `57243c3` chore(release): v0.46.0, tagged and pushed
3. The prompt list's own ☑ Soft wrap toggle, committed with this doc (not
   released; a new mode, so the next release is a minor bump, v0.47.0)

## 1. No hover card while soft-wrapped

The ask, from the backlog:

> On the prompt list and Next list when soft-wrapped do not popup a card

At the time only the Next List could wrap. With its ☑ Soft wrap on, each
item's whole text is already drawn under the row, so the card repeated it and,
at up to fifteen rows, covered the lines being read.

- `nextHoverMotion` (`nexthover.go`) clears the hover and returns while
  `m.next.wrap` is on, so no dwell is armed.
- `hoverDwell` (`listhover.go`) re-checks `m.next.wrap` when the tick lands.
  The toggle's key and click both clear a pending dwell already, so this is
  the usual "re-check every premise" rule rather than a known path.
- What is lost: the card's section, value in words and raised date. The page
  heading and the value mark still carry the first two.
- Test: `TestNextHoverCardOffWhenSoftWrapped`.

The prompt (backlog) list had no wrap at that point, so its card was left
alone and the user was asked whether they wanted a toggle there. They did
(part 3).

## 2. Release v0.46.0

Asked for as a minor release. The commits since v0.45.1 included the new ☑
Soft wrap mode, so minor was right on its own terms too. Both version strings
were bumped (`main.go`, `cats-plugin.toml`), the commit was made, the tag was
annotated as `v0.46.0 — ☑ Soft wrap on the editor and the Next List, ctrl+x
cuts in the prompt, drop picker tags fresh agents`, and `main` and the tag
were pushed.

## 3. ☑ Soft wrap on the prompt list

The ask: *Give the prompt list its own soft-wrap toggle.*

### What it does

`alt+z`, or the **☑ Soft wrap** chip flush right on the list's bar, toggles
it. It is off by default and saved as `listSoftWrap` in `settings.json`,
separate from the editor's `softWrap` and the Next List's `nextListSoftWrap`.

```
  ○ Fix the drop timeout
    The twelve second wait comes from stale ready probes, so capture a startup first.
      - then check the panesetup pattern
❯ ○ untitled first line
    second line
```

- The row's grey one-line description gives way to the whole prompt, wrapped
  under the title and indented to the title's column.
- Line breaks are kept. An indented line keeps its indent, and its wrapped
  rest hangs under that indent. Tabs count as four spaces. Blank lines are
  dropped.
- An untitled prompt's first line is its name, as always, and is not drawn
  again underneath.
- A prompt longer than the window is cut to fit, with "…" on its last line.
- No hover card while it is on (the rule from part 1, now on both lists).

### How it is built

It reuses the Next List's machinery: `listItem.more` continuation lines and
the line-counted window `fuzzyList.maxLines`.

- `listwrap.go` (new):
  - `wrapListItem` is applied to each row after `rebuildList` builds it, so
    the unwrapped row is unchanged and this is the only place the two differ.
  - `listWrapPrompt` does the breaking. It calls `wrapText`, the Next List's
    breaker, so a line breaks where the editor would break it.
  - `listNameCol` measures where the name starts: the gutter, the ✓ column,
    the badge and the annotation marks.
  - `listWrapLines` is the window's height in lines.
  - `toggleListWrap` flips the setting and saves it.
- `fuzzylist.go`: a new `listItem.moreDesc` draws continuation lines in the
  desc grey. A Next List item's text is its name, but a prompt is the body
  under a title, and the title has to keep standing out.
- `ui.go`:
  - `sizeListWindow` switches to `setMaxLines` while wrapped.
  - `applySizes` rebuilds the list on a resize while wrapped, since the
    lines were cut to the old width.
  - `rebuildList` now prunes the selection before building the rows. Whether
    the ✓ column is drawn moves where every name starts, and the wrap
    measures from there. Previously the prune ran after the build.
  - The bar gets `actionWrap`: `softWrapAction`, placed with `rightAlignLast`
    and drawn by `writeChipsAt`, as on the Next List.
  - `alt+z` is added to `updateList`, and the narrow footer names it.

### Design decisions

- **Cap at the window, not at a fixed count.** A row taller than the window
  is drawn whole and overruns the pane (`windowEnd` always takes the first
  row). So each row is held to `listWrapLines()` lines. There is no smaller
  arbitrary cap, because "whole text" is what the toggle promises.
- **Indent preserved.** `wrapText` trims each line, which suits the Next
  List's flattened paragraphs but would flatten a prompt's sub-bullets.
- **The chip's width cost was accepted, and recorded as N-090.** With a
  seventh chip the bar shows its chords only from 128 columns (before: about
  108). Words only now need 78 columns (before: 64), which updated
  `TestActionBarRender`. Taking the hint off this chip alone only gets to
  122. It stayed a chip, matching the other two screens. The fallback (the
  `ctrl+l` View panel) is written up in N-090.

### Tests

`listwrap_test.go`:

- whole prompt shown, saved, and toggled back off
- a click on a continuation line hits its row
- a long prompt is capped to the window with "…"
- no hover card, including a dwell armed before the toggle
- the chip is flush right and clickable
- a resize re-wraps
- `listWrapPrompt` rules

`go test ./...` is green. Nothing has been checked in cats yet (N-089).

### Docs

- README: the Soft wrap section covers three screens and adds the prompt
  list's paragraph and the bar-width cost. The hover card and Next List
  sections say no card while wrapped.
- The cats-todo-dev skill has a `listwrap.go` file-map row and `alt+z` in the
  list's chords.
- Comments in `softwrap.go` say "all three bars".

## Next

Closed: None. Declined: None. Raised: N-089, N-090.
Deferred: None. Promoted: None. Moved: None.
Updated: N-087 (Next List card now stays down while wrapped), N-088 (also
`listSoftWrap`). Full list: `ai_docs/todo/next-list.md`.
