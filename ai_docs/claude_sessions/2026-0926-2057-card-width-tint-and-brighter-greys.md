# A wider, warmer Next List card with the value in its title, and a brighter grey ramp

Session: `5eeac2ad-b103-429b-bdfc-ac8460b6633a`
Date: 2026-09-26
Context loaded: `2026-0926-1520-next-card-and-target-fold.md`

## Ask

1. Make the Next List hover card a couple of words wider.
2. Give the card's body text a subtle yellow tint; it read a bit dim.
3. Move the value phrase into the card's title; leave "raised" at the bottom.
4. Make the card bodies (Next List and the prompt list's) a tiny bit brighter,
   and in fact all dim text.
5. `/sw`.

## 1 — Width (`nexthover.go`)

`nextCardWidth` 76 → 88. `nextCardFor` already clamps to `m.width-2`, so a
narrow pane is unaffected.

## 2 — Yellow body tint (`styles.go`, `nexthover.go`)

- New `nextCardBodyStyle` (colCardBody on colPanel, same padding) used for the
  Next List card's text rows. `hoverBodyStyle` was **not** changed: the
  backlog's hover card (`listhover.go`) and the flag-note pad
  (`listflagnote.go`) share it.
- New const `colCardBody`, a pale low-saturation yellow kept under colFg so
  the bold title still leads the title/body/fields ramp. Started at `#cbc7a9`,
  lifted to `#d0ccae` in step 4.

## 3 — Value in the title (`nexthover.go`)

- Title: `N-001 · Open · value medium`. When a narrow card truncates, the
  value's end goes first and the ID survives.
- Foot row: just `raised <stem>`, in hoverFieldStyle; drops out when not
  recorded. The `fields` slice is gone.
- `TestNextHoverCardShowsTheItem` expectations and the README's example box
  and cap description updated. Row counts are unchanged (still ID + text +
  one foot row), so `TestNextHoverCardIsCapped` passes as is.

## 4 — Brighter greys (`styles.go`)

Each grey rose ~9 per channel, keeping the ramp's spacing; old values kept in
the comments per the file's existing "lifted from" habit:

| const | was | now |
|---|---|---|
| `colMuted` (headings, backlog card body) | `#9db0a2` | `#a6b9ab` |
| `colDim` (descriptions, counts) | `#899b8f` | `#92a498` |
| `colFaint` (footers, done prompts, card fields) | `#6a7b6f` | `#728377` |
| `colCardBody` | `#cbc7a9` | `#d0ccae` |

The greys are cats-todo's own (not from cats' shared palette), so nothing in
`~/projs/go/cats` needs to follow. No hex values appear anywhere else.

All tests pass; `go vet` clean. None of this has been looked at in a running
cats. `gofmt -l` flags `hangup.go` from an earlier commit (raised as N-067,
not touched).

## Next

Closed: None. Declined: None. Raised: N-066, N-067.
Deferred: None. Promoted: None.
Updated: N-041. Full list: `ai_docs/todo/next-list.md`.
