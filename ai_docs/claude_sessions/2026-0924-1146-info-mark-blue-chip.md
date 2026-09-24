# Info mark as a blue chip, released as v0.33.2

Session: `da9b4fa8-293b-449d-a089-b236f6703304`
Date: 2026-09-24

## Ask

The user sent a screenshot of the list. The `ℹ` info mark (added in v0.33.0)
was hard to see next to the 🍏 Quick win and 💎 High value emoji. They asked
for a blue background with an italic "i". Then they asked for a patch release
and a session wrap.

## Why it was invisible

The mark was `ℹ` (U+2139) with no emoji variation selector. That is a
one-cell text glyph in `colMuted` grey. The apple and the gem are two-cell
emoji that paint their own colours, so next to them the grey letter looked
like a stray character, not a mark. The grey was deliberate ("a note is not an
argument for attention"), but the mark's job is to be seen at a glance, and it
wasn't.

## What changed

- **Glyph** (`styles.go`): `infoGlyph` is now `ｉ` (U+FF49, FULLWIDTH LATIN
  SMALL LETTER I). It is two cells wide (East Asian Fullwidth), the same as
  the emoji, and the letter sits centred in the badge. A plain `"i "` would
  have pushed the letter against the left edge of its field.
  `lipgloss.Width` counts it as 2, so the packed row and the bar tiers
  needed no special handling.
- **Chip style** (`styles.go`): `infoChipStyle` is white, bold and italic on
  the new `colInfoChip` `#2f6db8`, with fg `colInfoChipFg` `#ffffff`. It
  isn't `colInfo` for two reasons: white on `colInfo` is about 2.5:1, and
  `colInfo` is the flag pennant's blue one slot over. White on `#2f6db8` is
  about 5.2:1. `infoStyle` (grey) now styles only the mark's *words*.
- **`infoMark`** (`annotations.go`): open rows get the chip. Closed
  (done/frozen) rows get `prioClosedStyle.Italic(true)` with no field, so the
  mark fades like the other marks on closed work.
- **Row renderer** (`fuzzylist.go`): a mark whose style has its own background
  is rendered on its own, not through `onRow`, so the highlighted row doesn't
  replace the blue with `colSel`. Its trailing space is rendered separately
  with the row's field.
- **`withInfoChips(st, text)`** (`annotations.go`): splits a label on
  `infoGlyph` and renders the pieces as siblings, `st` for the text and the
  chip for the glyph. Nesting doesn't work here: the inner reset would drop
  `st`'s field for the rest of the label. The plain text and the width are
  unchanged, so hit-test spans and menu box widths still match. An underline
  on `st` (the bar's keyboard cursor) carries onto the chip. It is used by
  the form's annotation bar (`annotbar.go`) and the context menu
  (`menu.go`, all three row styles).
- **Prompt view** (`ui.go`): for a mark with its own field, only the glyph is
  drawn in the mark's style. The label is drawn in `infoStyle`, so the blue
  doesn't run under the words.
- **Status refusals are unchanged.** `infoSendWhy`/`infoScheduleWhy` still say
  "clear ℹ Info to …", because a chip doesn't work inside a line of plain
  text. `TestInfoPromptsRefuseToLeave` still checks `"ℹ Info"`.

## Width knock-on

The glyph grew from one cell to two, so every annotation-bar tier is one cell
wider: 107 / 100 / 42 / 35 / 29 (was 106 / 99 / 41 / 34 / 28). The tightest
tier still fits the 30-cell minimum (`TestAnnotBarFitsNarrowPanes`), and the
snug tier is now exactly 100. The diagram and prose in `annotbar.go` and the
README were updated to match.

## Docs and tests

- README: `ℹ` became `ｉ` wherever it names the mark (legend table, list
  example, bar tiers, context-menu box, CLI echo), except the quoted refusal
  sentence. Added a paragraph to the Info section describing the chip.
  Realigned the context-menu ASCII box.
- `TestInfoMarkIsAChip` (`info_test.go`) checks that the glyph is as wide as
  `fruitGlyph`, that open marks (ordinary and selected) have their own
  background and are italic, that closed marks have no field, and that
  `withInfoChips` keeps the text and width and draws the chip.
- `go vet` is clean and `go test ./...` passes.

## Release

- `db410b6 fix(list): draw the info mark as a blue chip with an italic i`
- `0edec49 chore(release): v0.33.2`: `main.go` and `cats-plugin.toml`
  bumped together.
- Annotated tag `v0.33.2 — the info mark is a blue chip with an italic i, as
  prominent as the emoji beside it`. Pushed `main` and the tag.

This is a patch, not a minor: it refines a mark that already shipped.

## Not verified

The chip hasn't been checked by eye in a real terminal inside cats. Two things
depend on the terminal: whether SGR italic is drawn (cats' cells do carry an
`Italic` attribute), and how the font fallback renders fullwidth `ｉ` next to
Menlo. If italic isn't drawn, the letter shows upright on the blue field.

## Next

- **Check the chip by eye** (new). Look at an info row in cats, both plain
  and highlighted, plus the bar and the menu. If the fullwidth `ｉ` looks thin
  or odd in the fallback font, the fallbacks are a plain `i` padded inside the
  chip, or the `ℹ️` emoji (blue square, but its "i" isn't italic).
- **Send info prompts to gonotes** (carried over). Add a drop target for
  info-marked prompts that delivers them to a notes plugin (gonotes) instead
  of an agent. Probably an Export-like "➦ Send to notes" that is available
  only when `Info` is set, through the cats control socket or a gonotes CLI or
  API. It could mark the prompt done once it is filed. Work out gonotes'
  intake contract first.
- **Editor flag on the wire** (carried over). cats could add an `Editor bool`
  field (or similar) to `PaneMeta` in `pane.list`, computed from `EditorInfo`.
  Then cats-todo could drop its `editorAgents` copy and follow whatever
  `editor.agents` the user has configured.
