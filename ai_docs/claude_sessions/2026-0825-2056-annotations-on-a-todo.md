# Session: annotations on a todo — an apple, a re-cut priority, and a badge that leads

Session ID: `f0bc0fb8-78a8-45ae-b323-8a26bfba7c9a`
Date: 2026-08-25

The ask, in one message:

> "Add the concept of annotations to todos. I'm thinking:
> - An apple or other distinguishable fruit icon for 'low hanging fruit'
> - Priority as a priority-high or priority-critical, or none - this is a change
>   to the existing rule
> - Frozen - we already have that!
>
> Allow multiple icons per todo. Put the todo/done circle indicator first. Change
> the priority rules to none, high, critical"

New: `annotations.go`, `annotations_test.go`.
Touched: `store.go`, `priority.go`, `styles.go`, `fuzzylist.go`, `ui.go`,
`cli.go`, `complete.go`, `main.go`, `cats-plugin.toml`, `README.md`,
`.claude/skills/cats-todo-dev/SKILL.md`, and the tests beside each.

Version 0.15.0 → **0.16.0** (feature; both files).

## The concept, and why it needed a file of its own

The row was already answering three questions through one column. **What state is
this prompt in** — open, scheduled, frozen, done — is exclusive: exactly one is
true, and the badge that says so has always been a single glyph. **How much does
it matter** and **how cheap is it** are not exclusive of each other, and not
exclusive of the state either: a critical one-liner is critical *and* a quick win
*and* still open. Three facts, three columns.

So `annotations.go` names the family rather than bolting a second flag onto the
priority code:

```
❯ ○ ▲ 🍏 fix the drop path      the daemon cannot resolve a bare agent name
  ○ △    context menu grammar   right click across the manager screens
  ○   🍏 bump the version       two files, one number
  ○      ordinary work          nothing said about it
  ❄      shelved idea           not doing this
  ✓ ▲ 🍏 shipped it             done and dusted
```

Frozen deliberately did **not** move out of the badge, despite being listed in
the ask. It is a *state*, mutually exclusive with done, and the three render
groups (open / frozen / done) are read from it. Moving it would have broken
contract #1 for no gain — "we already have that" reads as "no work needed", not
"reclassify it".

### The two pieces of machinery

`annots` is the set as one value (`Priority string`, `Fruit bool`) with
`annotsOf` / `applyTo` / `any`. The marks stay separate fields on `Todo` for the
compat contract, but they are *edited* together and *saved* together, so passing
them as a set is what stopped the form, the store and the CLI each growing a
parameter per mark. `store.setPriority` became `store.setAnnots` — one
reload-and-save for an edit that touches both, with no window where the file
holds half of it.

`annotSlots` is the layout table: name, column width, a `mark` func returning
glyph + the two styles, and a `label` func returning the same fact in words.
`summary()` walks the table rather than switching on its own, so the ⚙ line, the
CLI echo and the prompt view can never fall into a different order from the
columns. Adding a mark later is: a field on `Todo`, a field on `annots`, an entry
in `annotSlots`. Nothing else has to know.

## The width problem, and the answer that made the whole thing free

Fixed columns are what make a glyph worth more than the word it replaces — a
mark you cannot scan straight down the pane is not doing the job it was added
for. But reserving every slot on every row means a backlog that annotates nothing
pays three cells of indent forever, growing each time a mark is added.

`trimAnnotColumns` drops the columns **no visible row fills**, once, after the
last row is built. It is a whole-list decision on purpose: deciding per row would
leave the names ragged, which costs more than the indent saves. The result:

- nothing annotated → no annotation columns at all, the list looks exactly as it
  did before the feature;
- one mark in use → one column, blank on the rows that don't carry it;
- both in use → both.

The widths are *declared* per slot rather than measured per row, because a slot's
possible glyphs are all the same width and a blank slot has to reserve exactly
that much. `🍏` is two cells (emoji); `▲`/`△` are one (East Asian Ambiguous, same
class as the `○` this list has always drawn). `TestFruitGlyphFitsItsColumn` pins
the emoji against the width its slot declares, which is the assertion that fails
if a glyph is ever swapped for a wider one.

## Priority, re-cut

Levels are now **none / high / critical**, and only *raising* a prompt leaves a
mark. The old scheme drew a dot on every single row, which meant the column could
not be scanned for the rows that actually wanted attention — every row looked the
same — and "low" never said more than "not raised". Both of those are now the
same answer: say nothing.

Glyph choices, both load-bearing:

- **Triangle, not a dot.** The badge two cells to the left is a circle in all
  four of its forms (`○ ✓ ❄ ◷`). A different *shape* says "different kind of
  fact"; a different hue on the same shape only says "different value". The old
  `●` sat next to `○` and the pair read as one thing.
- **Hollow → solid.** `△` high, `▲` critical. The level survives a colourblind
  reader, a monochrome capture, and a terminal theme that has flattened the
  palette. Colour is the second signal, not the only one.
- **Green apple, not red.** `🍏` sits one column from the red critical mark, and
  two reds on a row read as one signal repeated.

High takes `colTodo` — cats' own todo hue, the same yellow the mux paints the paw
badge with, so a row and the workspace badge that counts it match by
construction. `colWarn` (amber) is still unavailable: contract #3 reserves it for
the fuzzy-match highlight inside the row names a few columns right.

### The compat turn on the retired words

A backlog holding `"priority": "low"` from the old scheme had to keep working.
It does, three ways over:

- `normalizePriority` folds `standard` / `normal` / `medium` / `low` / `minor` →
  `priorityNone`, so `--priority low` in a shell history or a script keeps
  running and keeps meaning what it meant ("not raised");
- `priorityMark` draws nothing for any unrecognised value, so a stored `"low"`
  reads as none on the row;
- `priorityRank` parks it with the unmarked, so the priority lens does not
  promote it above work someone actually raised.

The key itself is left in the file until something rewrites the todo — no
migration pass, same as every other field here.

## Where they are set

Both live in the ⚙ panel, now headed **Prompt & session options** — the old
heading was contradicted by the two rows under it. `Priority` and `Quick win` are
rows 0 and 1, with a blank line after them:

```
Prompt & session options   default session

❯ Priority    critical   how much this prompt matters — △ high, ▲ critical, on its row
  Quick win   yes        low-hanging fruit — cheap for what it pays, marked 🍏 on its row

  Model       default
  …
```

The seam is the point: everything above describes **the prompt**, everything
below describes **the session that will read it**. Eleven rows with no break in
them read as one list of options rather than as two answers to two questions, and
the note column alone was not enough to say so at a glance.

`m.formPriority string` became `m.formAnnots annots` — still held by value, so an
abandoned form leaves the stored marks exactly as they were, now for the whole
set rather than one field.

No list-level chord was added. The list deliberately does not set priorities
(`TestCtrlPStillNavigatesTheList` pins that `ctrl+p` stayed the emacs alias), and
giving the fruit one alone would have made the two annotations behave
differently. `ctrl+y` is free if that turns out to be the wrong call.

## The other surfaces

- **CLI**: `--fruit` beside `--priority`, both long-only for the same reason (a
  bare `-p` next to `--perm` reads as an abbreviation of it). The echo names what
  was marked — `added to the project backlog, marked critical · low-hanging
  fruit` — and stays silent when nothing was said.
- **Completion**: `--fruit` in `addCompletions`; `--priority`'s values re-cut to
  `critical` / `high` / `none`.
- **Prompt view** (`ctrl+v`): the meta line now spells the marks out —
  `Project backlog · ▲ critical · 🍏 low-hanging fruit`. It never showed the
  priority before, which meant the one screen someone opens to find out what a
  glyph meant was the screen that didn't say.
- **Export**: `out = td` already copied the whole struct, so the fruit rides
  along; the test was extended to prove it rather than the code changed.

## Tests

`go test ./...` green throughout. Two existing tests broke on the row insertion
and were fixed at the cause rather than the symptom: `TestSessionPanelCycles` and
`TestSessionSavedWithTodo` walked to the Model row by a fixed number of `down`
presses, and now walk `for m.sessCursor < sessRowModel` — so the next row added
above Model does not silently retarget them.

Notable new pins:

- `TestOnlyRaisedRowsCarryAPriorityMark` — the rule that changed, including the
  retired `"low"` drawing nothing.
- `TestTheBadgeLeadsTheAnnotations` — asserts on the ANSI-stripped row, so the
  order being pinned is the order the reader sees.
- `TestUnusedAnnotationColumnsAreDropped` — the trim, at all three sizes.
- `TestEverySlotDeclaresItsOwnWidth` — every slot draws something for a fully
  annotated todo, and no two slots share a glyph.
- `TestAnnotationsStayOutOfTheJSONWhenUnset` — the compat contract for `fruit`.
- The test helper `keptSlots` *recomputes* which columns survived from the
  todos, the same way `rebuildList` decided, rather than hard-coding an index —
  so the tests keep pointing at the right column as slots are added, and fail if
  the trim ever disagrees with its own rule.

`TestPriorityDotUsesTheBrownForLow` became `TestPriorityRampIsInOrder`: `colBrown`
is no longer used by any priority level, and the ramp is now high → critical →
the greys, with an added assertion that the two glyphs differ so colour is never
the only thing telling them apart.

## Files worth knowing about next time

| File | What it now owns |
|---|---|
| `annotations.go` | `annots`, the `annotSlot` table, `trimAnnotColumns`, both mark/label funcs |
| `priority.go` | the levels, folding (incl. the retired words), label, rank |
| `styles.go` | `prioCriticalGlyph` / `prioHighGlyph` / `fruitGlyph`, their styles |
| `fuzzylist.go` | `annotMark`, and the row order — badge, then the annotation columns |
| `ui.go` | `formAnnots`, `sessRowFruit`, the panel seam, the prompt-view meta line |
