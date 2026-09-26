# A bigger Next List hover card, and other projects' agents folded in the drop picker

Session: `df02ea75-36f1-4364-b9ec-5990e364848a`
Date: 2026-09-26

## Ask

1. The Next List hover card was too short (and a bit narrow) to show most of
   an item: cap it at 15 lines and go a tad wider. Committed and pushed as
   `661a610`.
2. By default the drop target picker should not offer agents running in other
   projects; hide them behind a "more agents" / "more drop targets" row.
3. `/sw`.

## 1 — Next List hover card (`nexthover.go`)

- `nextCardWidth` 62 → 76, `nextCardMaxRows` 7 → 15 (ID row + 13 text rows +
  fields row). Seven rows cut nearly every item short.
- **Short panes.** A 17-line box (15 rows + border) would overflow a small
  pane, so `nextCardFor` now gives the body
  `min(nextCardBodyLines, m.height-3-2)` rows (at least 1) — the box plus one
  row to sit off the pointer's line. `nextCardLines(it, inner)` stays as the
  full-budget wrapper over the new `nextCardLinesMax(it, inner, bodyMax)`;
  `nextCardBody` takes the limit as a parameter. The ID row, fields row and
  the trailing `…` always survive.
- Tests: `TestNextHoverCardIsCapped` comments updated; a tight-budget case
  (`nextCardLinesMax(long, 40, 3)` → 5 rows ending in `…`) added.
- README: "seven rows … five lines" → "fifteen rows … thirteen lines". The
  ASCII example box is still drawn at the old width.

## 2 — Folding other projects' agents in the drop picker (`ui.go`)

- **New kind `targetMore`** — not a destination, the fold row. It never
  leaves the picker: `chooseTarget` intercepts it first (before the
  drop-in-flight guard, since unfolding sends nothing) and calls
  `expandTargets`, so `performDrop`, batch drafts and schedules never see it.
- **`dropTarget.elsewhere`** marks a running pane whose workspace differs
  from this launch's (`paneWorkspaceID(p) != m.ctx.WorkspaceID`, both
  non-empty). A pane with no workspace ID, or a launch with none, is never
  folded — hiding it would be a guess.
- **`buildTargets()`** is now `buildTargetsFor(false)`; all four call sites
  (backlog drop, schedule, Next List send, batch target) are unchanged and get
  the folded list. Folded panes are counted and a single
  `… More drop targets (N running agents in other projects)` row is appended
  **last**, so the default highlight (New Claude Code session) is unaffected.
- **`expandTargets`** rebuilds with `buildTargetsFor(true)`, highlights the
  first `elsewhere` row, and calls `applySizes` to re-width the new list's
  input. The picker stays open; the typed filter is reset by the rebuild.
- **Batch picker** (`batchcompose.go`, `beginBatchTarget`): if the draft is
  already aimed at an existing pane that the folded list doesn't contain, it
  opens unfolded, so the choice already made is never hidden.
- **Test** `droptargets_test.go`: `fakeCatsSocket` serves `pane.list` and
  `workspace.list` over a unix socket (under `os.TempDir` — darwin caps
  socket paths near 104 bytes). `TestBuildTargetsFoldsOtherProjects` checks
  the fold, the row's count and position, that choosing it keeps the picker up
  with nothing dropped, the highlight on the first revealed pane, and that a
  launch with no workspace ID folds nothing. First test in the repo to fake
  the control socket; reusable for other picker tests.
- README: one sentence in the opening drop description.

All tests pass; `gofmt`/`go vet` clean. Neither change has been tried in a
running cats.

## Next

Closed: None. Declined: None. Raised: N-060, N-061.
Deferred: None. Promoted: None.
Updated: N-041. Full list: `ai_docs/todo/next-list.md`.
