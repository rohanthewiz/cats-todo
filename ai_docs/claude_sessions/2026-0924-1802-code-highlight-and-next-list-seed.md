# Code highlighting in prompts, and the living next list seeded

Session: `b43a26ce-a641-4e33-813a-797d86715093`
Date: 2026-09-24

## Ask

1. "Hightlight code blocks bound by carets" (a backlog prompt). Asked which
   delimiter: the user chose **backticks**, `inline` and ``` fences, and
   **both** the editor and the read-only view.
2. `/next-list seed`: build `ai_docs/todo/next-list.md` from session history.
3. `/sw`: save, commit and push.

## Code highlighting (`promptcode.go`, new)

### The parser: `promptCodeSpans(text) []codeSpan`

Returns rune ranges `[start, end)`, **one per line of code**, so neither
consumer ever has to split a span at a line break.

- **Fences** are line-based. A fence line starts with three or more backticks
  after its indent. The block closes on the next line that starts with at
  least as many backticks and has nothing else after them. The opening fence
  may have an info string (```go); a closing one may not, which is what stops
  ```go inside a block from ending it. An unclosed fence runs to the end of
  the prompt, as in Markdown. Both fence lines are coloured with the block.
- **Inline** spans follow CommonMark's run rule. A run of N backticks closes
  on the next run of exactly N, so ``a ` b`` is one span, and a run with no
  partner is literal text. The one deliberate difference from CommonMark: a
  span **does not cross a line break**, so a single unmatched backtick typed
  mid-prompt colours nothing below it.
- `~~~` fences are not recognised. The prompts don't use them, and every
  extra rule is another way for prose to turn blue by surprise.

### Editor: another run on the existing overlay

`promptEditorView` (`promptsel.go`) already rebuilds lines with the selection,
spelling underlines and extra carets on them (`paintPromptSpans`). Code is now
one more kind of run there:

- `codePaintsFor` mirrors `spellPaintsFor`: clip to the display line, turn
  rune offsets into cells by summing widths, and set `caret: true` so the
  caret is redrawn inside code.
- The style is `promptCodeStyle(base)` = `base.Foreground(colInfo)`. Taking
  the line's own style keeps the cursor line's background. `colInfo` is the
  palette's one cool hue; amber is reserved for fuzzy-match highlights.
- **The older marks win their cells.** `cutPaintHoles` removes the selection
  and any spelling runs from the code runs, because `paintPromptSpans` needs
  runs that don't overlap. The spell checker already skips backticked text
  (`internal/spell/spell.go`, `Check`), so the two rarely meet. They can at
  the edges, because the checker treats ``` anywhere on a line as a fence,
  while this file only does so at a line's start.
- The early return now also requires `len(code) == 0`, so a prompt with no
  code still gets the textarea's own view, byte for byte.

### View: style before the wrap

`viewContent` (`ui.go`) now starts from `styleCodeSpans(td.Prompt, …,
viewCodeStyle)`, which colours the prompt's text **before** the session and
attachment lines are appended. That way an unclosed fence can't colour those
lines.

An existing comment there says a pre-styled span loses its reset at wrap
points. That is no longer true of lipgloss v2.0.5, checked by experiment:
`Width(20).Render` closes the style at each line break and opens it again on
the next line. `TestStyleCodeSpansSurvivesWrap` pins this. The old comment was
left in place (N-037).

### Tests (`promptcode_test.go`)

- A parser table covering inline spans, double backticks, unmatched runs,
  per-line spans, multibyte offsets, fences with and without info strings,
  indented fences, unclosed fences, closing-fence length, and no inline spans
  inside a fence.
- `cutPaintHoles`.
- The editor: code is coloured and prose isn't; the overlay changes no
  line's text or width; the caret survives inside a span; soft wraps; no code
  means an untouched view.
- Overlap: a real spelling-and-code overlap can't be produced from a prompt,
  so the cut is tested directly, plus a misspelling and a span on the same
  line, and a selection taking cells out of a span.
- The view: code is coloured, and the session line stays out of an unclosed
  fence.

`go test ./...` is green. The README has a new section, **Code in a prompt**,
and the dev skill's file map lists `promptcode.go`.

## The living next list (`ai_docs/todo/next-list.md`, new)

Seeded from the 15 docs `2026-0904-1132` … `2026-0924-1231`. The file uses the
item format `nextlist.go` parses, and was checked with `parseNextList` (21
Open, 1 Roadmap at seed time).

- **Most of the carried list had lapsed.** `2026-0922-0943-info-annotation`
  restarted its Next with one item and dropped the whole carried list. The two
  2026-09-15 docs dropped five more items, and `2026-0915-1637` dropped every
  non-goal. All are restored and marked *Lapsed* with the doc they fell out of.
- **Premises re-checked against the code:**
  - The `/clear` gate (N-020) is narrower than first written. The drop picker
    only lists `isDropAgent` panes, but `performScheduledDrop` only checks
    `paneExists`. So a scheduled drop to a pane whose agent has exited types
    `/clear` and the prompt at a shell.
  - The editor flag on the wire (N-031) was overtaken by `plugin_type`, read
    since v0.33.1.
  - `internal/integration` (N-002) was checked: its one constant matches cats.
- IDs were assigned in order of when each item was first raised, looking
  across all the docs, not just the window.
- **N-007** (column-mode footer) and **N-028** (title-field undo) are old and
  low-value, and were offered as candidates for Non-goals. They stay in Open
  until the user decides.

## Next

Closed: N-001, N-002, N-003, N-004, N-010, N-031, N-032 (found done or
overtaken while seeding). Declined: None. Raised: N-036, N-037.
Deferred: None. Promoted: None. Updated: N-020 (premise narrowed).
Full list: `ai_docs/todo/next-list.md`.
