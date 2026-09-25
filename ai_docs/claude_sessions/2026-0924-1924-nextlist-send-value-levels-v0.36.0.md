# Next List send, value levels, and v0.36.0

Session: `3cc15fb0-e1cc-40ea-adde-f66750b969ea`
Date: 2026-09-24

## Ask

1. "Shift+Enter on the Next List should send that item to an agent."
2. "Globally let's change the High value option to a set of value options",
   with high as a blue diamond for consistency with the Next List's medium
   and low marks, in the context menus and the prompt editor's bar. Plus the
   question: "Do we need a `|` to separate toolbar sections?"
3. "Can't we combine none and low, by making low the default … So there
   would only be 3 states globally?" Yes: see *Three levels* below.
4. Cut the release (v0.36.0) and `/sw`.

## Next List ✉ Send (`cd453e5`)

`shift+enter` (or `alt+enter`, or a new **✉ Send** chip) on the Next List
page opens the backlog's own target picker on the highlighted item.

- **Not saved to a backlog.** The form's ✉ Send saves and then drops. This
  one doesn't save, because the item already has a home in the file, and an
  esc out of the picker would leave a stray row. The prompt rides in
  `m.nextDrop` (a `nextSend{id, todo}`) instead of a `todoRef`. The picker
  reads its subject through the new `dropSubject()` (`ui.go`), so the rows,
  warnings and heading are the same as for a saved prompt.
- The prompt is `nextItemPrompt(it)`, the text ✚ New prompt prefills
  (`Next list item N-0xx (ai_docs/todo/next-list.md):` + the text), and the
  title is `nextItemTitle(it)`. The title names the tab, and the branch on a
  worktree drop.
- `chooseNextTarget` dispatches from the same `pendingAction`, with `cwd` set
  to the list's project root. It returns to the page and tags the
  `dropResultMsg` with `nextID`. `finishNextDrop` reports on the page's
  heading (red on failure, via the new `nextPage.noteErr`/`say`) and in the
  status line. Nothing is marked done.
- `leaveTarget` sends esc back to the Next List page. `backToList` also
  clears `nextDrop`, so a stale item can't hijack the next backlog drop.
- `dropDoneStatus` factors out the success wording shared by both kinds of
  drop.
- Tests: `TestNextListSendOpensThePicker`, `TestNextListSendDispatches`
  (fails on a missing socket and succeeds on a fed result, and neither writes
  a backlog), `TestNextListSendRefusesInWords`.

## Value levels (`561e990`)

### Three levels

The first cut had four levels (none, ◇ low, ◆ medium, 🔷 high). The user
folded none into low, so the levels are now:

| Level | Stored as | Row mark | Control glyph |
|---|---|---|---|
| high | `"highValue": true` (the gem's old key) | 🔷 | 🔷 |
| medium | `"value": "medium"` (new key) | ◆ straw | ◆ |
| low (default) | nothing | nothing | ◇ |

- `value.go` (new) holds `valueLow = ""`, `valueMedium`, `valueHigh`,
  `valueLevel()`/`setValueLevel()` (the only way to touch the two storage
  fields), `normalizeValue` (`none`, `lo`, `med`, `hi` fold),
  `valueLevelLabel` (empty for low), `valueWord`, `valueMarkFor` (glyph +
  style, shared with the Next List), `valueChosenStyle` and `valueNote`.
- **Compat:** high keeps `highValue`, so untouched backlogs stay
  byte-identical and older builds still see their gems. They ignore medium.
  A hand-written `"value":"high"` reads as high, and `"low"` or an unknown
  word reads as low.
- Low draws nothing on a row, the priority-none rule. The ◇ appears only on
  the bar's radio and the menu row. On the Next List, an unrated item and a
  low one now look the same (blank).
- Closed rows drop medium too, not just the emoji high, so the done tier
  doesn't look as if only its lesser prompts were rated.
- `annots.HighValue` became `annots.Value`, and `Todo` gained
  `Value string` beside `HighValue`.

### The editor's bar (`annotbar.go`)

- Segments: Quick win │ Value (◇ low, ◆ medium, 🔷 high) │ Priority (none,
  △, ▲) │ Info, Flag. `annotGroupStart` names the group openers and their
  labels.
- **The `│`:** yes, on every tier, drawn in `colFaint`. The labels already
  divide the wide tiers. On the narrow ones, two radio groups would otherwise
  run together as one line of holes. Clicks on a rule or a label are inert
  (pinned by `TestAnnotBarSeparatesItsGroups`).
- Tiers are now generated from an `annotSpelling` rather than written as
  literal arrays: 150, 137, 104, 84, 67, 58, 47 and 23 cells. The radios'
  words go before the checkboxes' words, and the group labels last down to
  84 cells. **At 100 cells the bar keeps the labels and drops "Quick win",
  "Info" and "Flag"**; the words would need 104. That trade was flagged to
  the user (N-039).
- The new bare tier (23 cells) fits the 30-cell floor. It drops the radios'
  holes and draws the chosen radio in reverse (`annotSegStyle(i, bare)`).
- `TestFormRowsMatchWhatIsDrawn` now finds the bar row by "Priority" instead
  of "Quick win".

### Elsewhere

- **Context menu:** `Value: ◇ low`, `Value: ◆ medium`, `Value: 🔷 high`
  radio rows (`listMenuValue` table), with `valueNote` on the status line.
- **CLI:** `--value high|medium|low` (and its completions). `--high-value`
  stays as an alias for `--value high`, and giving it with a different
  `--value` is refused.
- **Bundle markdown:** now says "medium value" and "high value".
- **Next List:** `nextValueMark` now uses `valueMarkFor`, and
  `nextMediumGlyph`/`nextLowGlyph` are gone. `valueGlyph`/`valueStyle` left
  `styles.go`.
- `highvalue_test.go` was renamed to `value_test.go` and rewritten.

README: the annotations table, the **Value** section (it replaces
*High value*), the bar section (the `│` rule and a tier table), the context
menu, the CLI, and the Next List marks table. The dev skill's file map lists
`value.go`.

## Release

v0.36.0 (`5679962`, annotated tag `v0.36.0 — value levels, Next List send,
code highlighting`), pushed with main. It is a minor bump because it brings
new capabilities. It ships the code highlighting from the previous session
too.

## Next

Closed: N-036. Declined: None. Raised: N-038, N-039, N-040.
Deferred: None. Promoted: None. Updated: None.
Full list: `ai_docs/todo/next-list.md`.
