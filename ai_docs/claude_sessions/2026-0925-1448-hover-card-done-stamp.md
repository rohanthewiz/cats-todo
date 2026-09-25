# The done stamp on the list's hover card

Session: `8fa9dc42-6fe4-4ae4-ac02-d6e0a567261f`
Date: 2026-09-25

## Ask

N-014, pasted from the Next List: the list hover card (`listhover.go`) shows
no completion stamp. Decide whether the row's compact form is enough, or
whether the card should spell out the full stamp the way `viewPrompt` does.
Then `/sw`.

## The decision: spell it out

The compact form on the row is not enough, for two reasons:

- **The row can lose it.** `done 14:05` is a `descMark`, drawn after the
  badge, the annotations, the name and the tag. A titleless prompt's name is
  its first line cut to 60 cells, so in a side pane of ordinary width the
  stamp goes off the right edge. `withOverflowMark` does not rescue it, and
  it wasn't built to.
- **It abbreviates.** `formatDoneTime` drops the date within the week and
  never shows the zone, and "when exactly" is the question someone hovering
  a finished prompt is usually asking.

The card is where the pointer already is and has 48 cells of text budget,
so it is the one place in the list that always has room for the whole stamp.

## What changed

- **`listhover.go`.** The label/value block is now a gathered list of
  fields, so the separator is drawn once and only when a field has a value.
  `Done` leads, followed by `Model` and `Effort`. On a done card the stamp
  is the live fact; the session fields describe a launch that has already
  happened. There is no row without a `DoneAt`. A stamped title-only done
  todo now earns a card (before, it got none), and the comment on the
  "nothing to say" rule notes that exception. `hoverLabelWidth` stays 8:
  "Effort" is still the widest label.
- **`schedule.go`.** `formatDoneStamp` (`2006-01-02 15:04 MST`, local),
  beside `formatDoneTime`. The prompt view (`ui.go`) and the bundle
  (`bundle.go`) now call it instead of their own inline copies of the
  format, so the three can't drift apart.
- **README.** A paragraph in "The hover card" on the done stamp and why the
  row's form isn't enough.

## Tests

`TestHoverCardSpellsOutTheDoneStamp` (`listhover_test.go`) has three
subtests, at the `hoverLines` level:

| Case | Expect |
|---|---|
| done, stamped, with a model | `Done    <full stamp>` before `Model`, one separator |
| done, stamped, title-only | a card, carrying the stamp |
| done, no stamp | no card when title-only; otherwise no Done row and no stray separator |

`go vet` and `go test ./...` pass. `gofmt -l` flags `ui.go` and
`promptcode.go`, but it did before this change too (a field-alignment
drift in the model struct), so they were left alone.

Not released. It is a refinement of the existing hover card, so it would
ship as a patch.

## Next

Closed: N-014. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
