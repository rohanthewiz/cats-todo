# Trimmed indent on enter, a filter that searches the fold, the line cap in words, and v0.41.0

Session: `057e4d05-cb70-45d0-a835-e2efc3b61956`
Date: 2026-09-26

## Ask

1. N-011: should enter with the caret inside an indent trim the upper line?
   Answered **yes**. Committed as `7a0a087`.
2. N-061: should the drop picker's filter search the folded agents? Chose
   "search them, don't unfold", and the user approved the commit. Committed as
   `90050df`.
3. N-025: pastes past the library's 10,000-line `maxLines` were truncated
   silently. Committed as `579d9b9`.
4. Cut a release (v0.41.0), then `/sess-wrap`.

## 1 — N-011: enter inside an indent (`promptindent.go`, `promptcarets.go`)

- `promptCarriedIndent`'s second result went from `blank` to `bare`: only
  spaces stand left of the caret (`indent > 0 && indent == col`). The blank
  indented row is now one case of that rule, so `"  |  x"` + enter →
  `""` / `"    x"` rather than `"  "` / `"    x"`. The spaces move down
  instead of being copied, and the backspace straight after still takes them
  back, since the carry is recorded the same way.
- Column mode (`newlineAtCarets`): a caret alone on its row cuts the row's
  head (`rows[r][col:]`, a safe byte slice because only spaces precede the
  caret) instead of emptying the row. The guard for two carets on one row is
  unchanged.
- Tests: `TestPromptEnterInsideTheIndentLeavesNoTrailingSpaces`,
  `TestCaretsEnterInsideTheIndentLeavesNoTrailingSpaces`. README's
  indenting paragraph was updated.

## 2 — N-061: the target filter searches the folded agents (`fuzzylist.go`, `ui.go`, `batchcompose.go`)

- `listItem` gained `queryOnly` (listed only while the query is non-empty)
  and `browseOnly` (only while it is empty). `filter` honours both, and
  `counts` totals only what the current mode can list, so an empty query
  never reads "3/5".
- `buildTargetsFor(false)` now builds the elsewhere panes too, marked with the
  new `dropTarget.folded` → `queryOnly`. The More row is `browseOnly`, so it
  steps aside while a query is typed. Clearing the query folds them away
  again, and choosing More still unfolds everything (`expandTargets`
  unchanged).
- `beginBatchTarget`'s "draft aims at a folded pane" check now ignores folded
  rows, since they are in `m.targets` but not listed at rest.
- The rejected alternative was to unfold on the first keystroke. That would
  keep the list unfolded after the query is cleared, and it needs a socket
  round-trip.
- Tests: `TestBuildTargetsFoldsOtherProjects` now reads the listed rows
  (`targetList.filtered`); `TestTargetFilterSearchesFoldedAgents` is new
  (the query "yonder" finds pane 3, More is hidden, the count is 1/4, and
  clearing the query restores the fold). README's picker paragraph was
  updated.

## 3 — N-025: the 10,000-line cap, refused in words (`promptcap.go`, new)

- The library cuts in `insertRunesFromUserInput`, which a paste and every
  `SetValue` go through. A one-caret paste loses the paste's tail. Every
  `SetValue` road (column mode, enter, ≡ Insert a prompt) loses the
  **prompt's own** last lines, and a save or the autosave then stores the cut
  copy. So nothing is trimmed: the whole edit is refused.
- `promptMaxLines = 10000` mirrors the unexported `maxLines`, and
  `TestPromptMaxLinesMatchesTheLibrary` pins it by behaviour.
- Guards:
  - `pasteFitsPrompt`: the `tea.PasteMsg` case and `pasteIntoForm` (Cmd+V /
    OSC 52), one caret or every caret. The breaks inside a replaced selection
    count as room. `promptLineBreaks` counts `\r` and `\n` the way the
    library's sanitizer does, and folds `\r\n` first on the carets road.
  - `newlinesFitPrompt`: `newlineCarryingIndent` (which answers the key, so
    the key never reaches the library) and `newlineAtCarets` (all or none).
  - `insertSnippet` (`promptlib.go`) returns a "not inserted — …" note.
  - `beginEditRef` won't open a stored prompt already past the cap. The
    status line names its length and points at `ctrl+v`.
- `thousands()` formats the counts ("10,001").
- Tests (`promptcap_test.go`) run in ~0.6s. They avoid walking the caret down
  thousands of rows, which is quadratic in the library (raised as N-063). The
  first draft took 60s: `promptAt` to the end took ~30s, and a caret on each
  of 9,999 rows took ~33s.
- README: a new paragraph, **The editor holds at most 10,000 lines**, after
  the copy/paste section.

## 4 — Release v0.41.0

- Minor, not patch: the unreleased range since v0.40.0 also holds the More
  drop targets fold, the ⤓ backlog mark, and the Next List send record,
  which are new capabilities.
- `main.go` + `cats-plugin.toml` bumped, `chore(release): v0.41.0`
  (`83f8a2c`), annotated tag `v0.41.0`, and main and the tag pushed.

All tests pass; `gofmt`/`go vet` clean. None of this was tried in a running
cats.

## Next

Closed: N-011, N-025, N-061. Declined: None. Raised: N-063.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
