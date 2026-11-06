# List wrap chip yields first (N-090)

Session: `032bdbbe-47b0-43ff-b945-7a8238666d14`

## Ask

N-090 from the Next List: the prompt list's ☑ Soft wrap chip moved the action
bar's chords threshold from about 108 columns to 128. So a 120-column pane
showed words only, and the footer fell back to its two-line full form. Taking
the chord off that one chip only gets to 122. The item offered two fallbacks:
move the toggle to the `ctrl+l` View panel, or let the wrap chip alone shrink
to its glyph before the others lose their chords.

## What was done

The second fallback. It keeps a visible control, which fits the "button rows
shrink, never drop a chip" contract, and it is the smaller change.

- **`tierLeanHints`** (ui.go): a new `chipTier` between `tierHints` and
  `tierLabels`. At that tier, `chipText` prints a `yieldsFirst` chip as its
  icon. Every other chip keeps label + chord. `barTier` tries the tiers in
  this order: hints, lean, labels. On a bar with no `yieldsFirst` chip the
  lean tier is exactly as wide as hints, so it is never picked there. That
  leaves the form, Next List and composer bars unchanged.
- **`listAction.yieldsFirst`**: set only by `listSoftWrapAction`, which is
  used by `listActions`. The editor's bar was left alone on purpose: its
  footer is budgeted to exactly 118/120 cells and has no room for `alt+z`.
- **`renderChipDimHint`** now asks whether the chip's text actually carries
  the chord, not just which tier it is at. At the lean tier some chips carry
  one and some don't.
- **`barShowsHints`** is now `tier <= tierLeanHints`. The new
  **`barTeachesWrap`** (`tier == tierHints`) tells `listFooter` to add
  `alt+z soft wrap` after `ctrl+l view options`.

Thresholds, measured (alt+enter / shift+enter):

| Tier | From |
|---|---|
| hints | 128 / 130 |
| lean (☑ alone) | 112 / 114 |
| labels | 78 |

At 120 the bar is `✚ Add ctrl+a … » Next List ctrl+g … ☐`, and the footer is
two lines ending `… alt+z soft wrap · ctrl+w clear done · esc quit`. The
trade-off is that `right-click menu`, the footer's last and first-to-concede
segment, no longer fits at 120.

## Tests and docs

- `TestListWrapChipYieldsFirst` (listwrap_test.go) checks:
  - the lean tier at 120, with both drop chords;
  - each chip's text;
  - the bar width against the chip spans (flush right at 120);
  - the footer teaching `alt+z` and not repeating `ctrl+a`/`ctrl+x`;
  - the hints tier at 200 (no `alt+z` in the footer);
  - the labels tier at 78, where the chip's word comes back.
- `TestLeanTierOnlyOnTheList` sweeps widths 20–200 over the form, Next List
  and batch bars.
- README, Soft wrap section: the new widths and the footer behaviour.
- The cats-todo-dev skill's `listwrap.go` row now mentions it.
- `go test ./...` passes. Not run live in cats.

## Next

Closed: N-090. Declined: None. Raised: None. Deferred: None. Promoted: None.
Moved: None. Updated: None. Full list: `ai_docs/todo/next-list.md`.
