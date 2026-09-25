# The composer's narrow footer (N-055) and the view's appended lines (N-037)

Session: `60d27a95-771b-4065-9e58-791b7ccf1a31`
Date: 2026-09-25

## Ask

Two Next List items were pasted in turn. N-055: below the width where the
batch composer's button row keeps its chords, the footer no longer named
`ctrl+s`, `ctrl+k` or `esc` (contract 6). N-055 was committed on request
(`8936721`). N-037: the `viewContent` comment claimed a pre-styled span
"loses its reset at the wrap points", which lipgloss v2 no longer does, and
asked whether the session and attachment lines still had to stay plain. The
session ended with `/sw`.

## N-055 — the composer's footer

- **Where the bar sheds its chords.** A width probe showed `batchBarTier`
  keeps `tierHints` down to 89 columns (the item guessed 110). At 89 and up
  the old footer is right: the chips print their own chords, so a trimmed
  tail loses nothing, and that branch is unchanged.
- **Below it** (`batchFooterSegs`, `batchcompose.go`), the chords lead the
  line in the bar's order, where `fitFooter` never trims, and the focused
  region's keys take the width left, then `tab next`. The region's keys are
  the easier loss, since they are mostly arrows, space and typing.
- **A glyph legend, at both narrow tiers:** `▶ alt+enter · ◷ ctrl+s ·
  ☰ ctrl+k · ✕ esc`. A worded form (`ctrl+s schedule`…, the Next List
  page's precedent) was tried first. It is 60 cells, and at 60–79 columns it
  left no room for any region key. The legend is 41, keeps `space pick` at
  60 and adds `ctrl+a all` at 70–80. The glyph is on the chip at
  `tierLabels` as well as `tierIcons`, so it reads against either.
- `⇅ A→Z` has no entry: its chip has no chord, and the Batch pane's own
  segments name `s`.
- Test: `TestComposerFooterNamesTheChordsTheBarDropped` (`batch_test.go`)
  covers widths 50/60/80 (all four chords, fits the width), `space pick` at
  80, and a 140-column footer still leading with the region's keys.
- README: a paragraph in the composer section.

## N-037 — the view's comment and its appended lines

- **The comment** on `viewContent` (`ui.go`) now describes what lipgloss v2
  does: a style is closed at each break and reopened on the next line,
  pinned by `TestStyleCodeSpansSurvivesWrap`. The old text also called this
  "the same hazard the list's badges are written verbatim to avoid". That
  one (`fuzzylist.go`) is a different problem: a style nested inside
  another, where the outer reset cancels the inner. That reference is gone.
- **The decision: style them.** With the wrap hazard gone, the plain text
  bought nothing. `⚙ session:` and `📎 n attached:` are now `descStyle`
  labels. Everything above them is what the agent receives, and a label in
  the prompt's own ink could be read as a line of it. The values (model,
  paths) stay in full ink, since they are what a reader checks and copies.
  `(missing — will not be sent)` is in `errStyle`, because this view is the
  only place a dropped attachment shows.
- Test: `TestViewMarksWhatItAppends` (`promptcode_test.go`) checks both
  labels are dimmed. In a 30-column pane the missing note wraps onto two
  lines, and both must carry the error hue.
- README: a sentence in the attachments section.

`go test ./...` passes. Both changes are patch-level refinements (no release
cut this session).

## Next

Closed: N-037, N-055. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
