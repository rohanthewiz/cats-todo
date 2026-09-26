# Lighter greys, a column-mode paste test, and the Next List's backlog record

Session: `0ddad61e-1c10-4d7d-a09c-7fedc8171af2`
Date: 2026-09-26

## Ask

1. From a screenshot of the Prompts page: "The dimmed text is a tad too dim."
   Committed as `0e98bb7`.
2. N-005: test the Cmd+V chord's local pasteboard road into the column mode.
   Committed as `dc7abab`.
3. N-040, answered **yes**: a Next List send should also add to the backlog.
   Committed as `e3bcbf9`.
4. N-047: mark the Next List rows whose item is already in the backlog.
5. `/sw`.

## 1 — The dim and faint greys (`styles.go`)

- `colDim` `#7d8f83` → `#899b8f` (HSL lightness 52% → 57%). It draws
  descriptions, counts, the page subtitle ("Prompts", "only") and chip hints.
- `colFaint` `#5f6f64` → `#6a7b6f` (40% → 45%). It draws the filter
  placeholder, completed rows, footers and inert chip hints.
- The ramp keeps its order (muted 65% > dim 57% > faint 45%), and closed rows
  stay under the low-value brown's 53%. The `colBrown` comment's "closed
  rows' 40%" became 45%.

## 2 — N-005: Cmd+V's local pasteboard into the carets (`promptcarets_test.go`)

- `TestCaretsTakeTheLocalPasteboard` stubs `readClipboardText` with
  `stubClipboard`. It drives both `super+v` and `meta+v` (`cmdKey('v')`) over
  two carets and checks three cases. One line per caret spreads, after the
  trailing newline is dropped. A count mismatch pastes the whole text at each
  caret. An empty pasteboard gives the "no text" note and keeps the mode on
  (`pasteFormClipboardInMode` returns handled=true).
- The fall-through check uses `pendingPaste`, not the returned Cmd:
  `watchAutosave` arms a tick after any edit, so a Cmd is always returned.
  `pendingPaste` is set only beside `tea.ReadClipboard`.
- The OSC 52 test's comment no longer says the local road can't be driven.

## 3 — N-040: a Next List send leaves a done record (`nextlist.go`, `ui.go`)

- `finishNextDrop` calls the new `recordNextSend(it)` on success only. If
  `nextBacklogCopy` finds an open copy, that copy is marked done. Otherwise
  `saveNextItem` adds the item (value carried) and it is marked done. The
  heading reads `N-014 dropped → … · recorded done in the project backlog`.
- It is best effort: a failed write adds "not recorded: …" to a green line,
  and `errNoBacklog` is silent.
- The order differs from the form's ✉ Send (save, then drop) on purpose.
  Writing before the picker would leave a row behind on esc. The end state is
  the same.
- `dropResultMsg.nextID string` became `nextItem *nextItem`, and
  `nextSend.id` became `nextSend.item`. The whole item rides along because
  the page may reload the file before a slow drop lands.
- Tests: `TestNextListSendDispatches` now expects a done, stamped record after
  a success and nothing after a failure. `TestNextListSendMarksAnOpenCopyDone`
  covers the case where a copy already exists.
- README's send paragraph and the ✉ Send… menu bullet were rewritten, and the
  `nextmenu.go` header comment updated.

## 4 — N-047: the ⤓ row mark (`nextlist.go`, `ui.go`)

- `nextPage.inBacklog map[string]bool` is filled by `nextBacklogIDs()`, one
  pass over the target store. `nextCitedID(prompt)` reads the citation back
  and is round-tripped through `nextItemCite` so it accepts only what the
  writer writes. Open and frozen copies count; done ones don't, the same rule
  as `nextBacklogCopy`.
- `nextBacklogMark(held)` is a green `⤓` (the Add rows' glyph) placed after
  the value mark. `nextBacklogMarkWidth = 1` is reserved on every row, and
  `rebuild`'s room calculation subtracts it.
- The set is refreshed in `beginNextList`, in `refreshNextList` (ctrl+r) and
  by `syncNextBacklog()`, which is called at the end of `rebuildList` and
  does nothing off the page. So an Add or a send's record redraws the marks.
- Tests: `TestNextListValueMarks`'s column math includes the new slot;
  `TestNextListMarksItemsInTheBacklog` and
  `TestNextCitedIDReadsBackTheCitation` are new. A first draft of the mark
  test matched the heading's "added N-002 …" note instead of the row; rows
  are now matched by the cursor gutter plus the ID.
- README: the example rows show the slot, and a paragraph explains the mark
  and its citation-line limit.

All tests pass; `gofmt`/`go vet` clean. None of 2–4 has been tried in a
running cats (N-062).

## Next

Closed: N-005, N-040, N-047. Declined: None. Raised: N-062.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
