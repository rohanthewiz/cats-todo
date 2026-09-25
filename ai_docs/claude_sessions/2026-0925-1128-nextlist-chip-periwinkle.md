# Next List chip goes periwinkle, v0.38.0

Session: `553ba3d7-861a-4455-b39f-a56b13cc2b78`
Date: 2026-09-25

## Ask

"The 'Next List' toolbar button foreground color is too pale. Let's go with a
blue but a little light for contrast." Then: "Actually make it a little more
towards purple." Then: "Love it! Commit. Then let's do a minor bump." Then
`/sw`.

## The color

The list's action bar (`listActions`, `ui.go`) tinted `» Next List` with
`colBrown` (`#b5835a`), the low-priority dot's hue. At 53% lightness it sat
below the rest of the bar, and brightness is the bar's grammar for
live versus inert, so the chip read as dim.

A new palette constant, `colSky`, in `styles.go` (beside `colCyan`/`colStraw`):

- First pass `#a3c4f3`, 215° 77% 80%: a pale sky blue.
- Final `#a5abf3`, 235° 77% 80%: the same lightness and saturation turned
  about twenty degrees toward violet, so a pale periwinkle.

Not `colInfo` (`#6ea9d8`, 207°), which ✚ Add already speaks on the same row.
`colSky` is some sixteen points lighter and almost thirty degrees further
round, so the two separate by hue as well as brightness while still reading
as one blue family. The comment on the constant records both the reason and
the 215° starting point.

`colBrown` is unchanged: it is still the low-priority dot.

## Release

- `49c2b87 fix(ui): tint the Next List chip a pale periwinkle`
- `656f91c chore(release): v0.38.0`: `main.go` and `cats-plugin.toml` bumped
  together; annotated tag `v0.38.0 — Next List context menu, a periwinkle
  Next List chip`; code and tag pushed.

The minor bump is for the Next List context menu (`fcceef9`, the previous
session), a new capability. The color change alone would have been a patch.

`go test ./...` is green.

## Next

Closed: None. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
