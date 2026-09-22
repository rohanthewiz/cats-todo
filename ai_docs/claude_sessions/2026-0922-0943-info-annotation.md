# ℹ Info annotation — a note, never sent to an agent — v0.33.0

Session: `be26d34e-0868-4d03-ac59-d2254b81196f`
Date: 2026-09-22

## Ask

"Add an info annotation - maybe an "i" icon"

I asked what the mark should mean, and the answer was: "An info is a benign
prompt. It's essentially just a note that should be collected into a proper
note program in the future. So we would never drop this to an agent. Hmm, but
rather we could drop this into a notes plugin like gonotes."

## What was built

A fifth annotation, `Todo.Info` (`json:"info,omitempty"`), wired through the
usual steps for adding a mark: a field on `Todo`, a field on `annots`, a line in
`annotsOf`/`applyTo`/`any`, and an entry in `annotSlots`.

### The mark

- Glyph `ℹ` (U+2139 with no emoji variation selector, so it is a one-cell text
  glyph that takes a foreground colour). `infoGlyph`, `infoStyle` in `styles.go`.
- Drawn in `colMuted` grey, not a hue. The other coloured marks each say
  something about the work, and a note has no work in it. `colInfo` blue
  already belongs to the flag, which sits right next to it.
- On closed rows it fades to `prioClosedStyle`, the same way the flag does (it
  is a text glyph, so the grey reaches it). It is not hidden like the emoji
  marks are.
- Slot order: priority → fruit → gem → **info** → flag. The flag stays last
  because it is the mark with words that has to be read.
- Label: `info — a note, not for agents`. It appears in the prompt view and the
  CLI echo.

### Never dropped

- `startDrop` refuses it with `infoSendWhy`. This covers every drop road:
  shift+enter, ✉ Send from the form, the context menu, and the view stage.
- `beginSchedule` refuses it with `infoScheduleWhy`.
- In the list menu, the Send and Schedule rows are dimmed with the same words.
- `annots.applyTo` clears `Schedule` when Info is set, the same rule freezing
  follows. `setMenuAnnots` says so on the status line when a schedule is
  removed.
- `fireDueSchedules` skips `t.Info` as a backstop for hand-edited backlogs. It
  neither fires the prompt nor marks it Missed.
- It is an annotation, not a fourth state, so a note can still be open, done
  or frozen, and can carry a priority.

### Where it is set

- **Form annotation bar:** `annotSegInfo`, placed before `annotSegFlag`, so the
  flag is still the last segment and keeps its note field.
- **List context menu:** `listMenuInfo`, labelled `☐ ℹ Info (a note)` and
  placed before Flag.
- **CLI:** `cats-todo add --info` (long-only, like the other annotation flags).
- **Bundle markdown note:** writes `info (a note)`.

### Annotation bar tiers

A seventh segment broke the width budget. The full tier grew from 95 to 106
cells, which lost the words at 100 cells, and the tight tier grew to 34, over
the 30-cell floor. The bar now has five tiers:

| tier | gap | cells |
|---|---|---|
| full | 3 | 106 |
| snug (words kept, narrower gaps) | 2 | 99 |
| compact | 2 | 41 |
| tight | 2 | 34 |
| tightest | 1 | 28 |

## Tests

- New `info_test.go`:
  - drop and schedule are refused in words
  - raising the mark clears a schedule, and an unrelated annotation save does
    not
  - the tick skips a hand-edited info+schedule prompt
  - round trip through the accessors, plus a check that no `info` key is
    written for an unmarked todo
- `TestEverySlotDrawsADistinctGlyph` now includes Info.
- `go test ./...` passes.

## Touched

`store.go`, `annotations.go`, `annotations_test.go`, `annotbar.go`,
`listmenu.go`, `ui.go`, `cli.go`, `bundle.go`, `styles.go`, `info_test.go`
(new), `README.md` (new `#### Info` section, the marks table, the bar tiers,
the context-menu diagram, the `add` examples).

## Release

**v0.33.0**, a minor bump because this is a new capability. The steps were:

1. Committed the feature as `37e51c8` (`feat(annotations): ℹ info mark — a note, never sent to an agent`).
2. Bumped the version in `main.go` and `cats-plugin.toml`.
3. Committed the bump as `f058fcc` (`chore(release): v0.33.0`).
4. Created the annotated tag `v0.33.0` ("v0.33.0 — an ℹ info mark keeps notes in the backlog and away from agents").
5. Pushed `main` and the tag.

## Next

- **Send info prompts to gonotes.** Add a drop target for info-marked prompts
  that delivers them to a notes plugin (gonotes) instead of an agent. Probably
  an Export-like "➦ Send to notes" that is available only when `Info` is set,
  through the cats control socket or a gonotes CLI or API. It could mark the
  prompt done once it is filed. Work out gonotes' intake contract first.
