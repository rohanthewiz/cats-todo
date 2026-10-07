# Session: a Validate section on the Next List page, and ◎ Move to Validate

Session ID: 874c3bd1-c28b-47fa-a2df-63757a070010
Date: 2026-10-07
Released as **v0.45.0**: a minor bump, since the page gains a section and the
item menu a move. The only other commit since v0.44.0 was a `.gitignore` edit.

## The ask

> The Next list (ai_docs/todo/next-list) will get a # Validate section. Add a
> context menu action to move a todo there

Then, after looking at it:

> Also show the # Validate section in the Next list view

The second ask was already met by the first change. The user had been
looking at the **installed** plugin (`~/.config/cats/plugins/rohanthewiz.cats-todo`,
built from v0.44.0), not this checkout. v0.44.0 lists only Open and Roadmap,
so the nine items another session had just split into Validate had dropped
off the page. A frame rendered from this checkout's build on a copy of the
real file showed the section. The user then tried it with `go run`, said it
looked good, and asked for the release.

## Where Validate comes from

The global `/next-list` skill (edited the same day) defines Validate: the
section directly below Open for items whose remaining work is only a check
(run, look, hear, measure, or write/repair a test), with no product change
planned unless the check finds a defect. It uses the same item form as Open
(`value`, `raised`), stays in ID order, and if the file lacks it, it is added
below Open the first time an item moves there. While this session worked,
another session restructured `ai_docs/todo/next-list.md`. It added the
section, its conventions, and N-017, N-033, N-039, N-043, N-044, N-066, N-077,
N-082 and N-083. The page follows that definition, and this session didn't
re-sort any items.

## What changed

- **`nextedit.go`**. `nextDest` gains `nextToValidate`, and `nextSectionOrder`
  is now Open, Validate, Roadmap, Non-goals, Closed. The iota indexes that
  slice, so the two are kept in the same order. `spaced()` is now
  `!record()`, and the new `record()` is true for Closed and Non-goals.
  `findNextBlock` uses `nextListed`, the parser's own test. Before, it
  hard-coded Open/Roadmap, so a Validate item could not be moved out again.
  A missing Validate section is placed by the existing `insertNextSection`
  rule ("before the first section that follows"), which puts it directly
  below Open. The refusal now reads "no longer Open, in Validate or on the
  Roadmap".
- **`nextlist.go`**. `nextListedSections` is now Open, Validate, Roadmap.
  The parser's `listed` closure became the package func `nextListed`, so the
  parser and the splice share one test. The heading's `counts()` writes
  Validate as `to validate` ("9 validate" reads as an order). The
  empty-state sentence names Validate too.
- **`nextmove.go`**. `validateNextItem` and `nextValidateLabel` are new. The
  two one-press rows are **toggles against Open**, each owning one section:
  - ◎ Move to Validate moves an Open or Roadmap item to Validate, and
    ⇡ Move to Open takes a Validate item back.
  - the park row (`ctrl+f`) moves an Open or Validate item to the Roadmap,
    and ⇡ Move to Open takes a Roadmap item back.

  So every listed section reaches every other, and neither row ever greys out
  for "already there". `moveNext` keeps the highlight on the item for any
  move that leaves it listed (`!to.record()`), and its note says
  `N-### moved to Validate`.
- **`nextmenu.go`**. `nextMenuValidate` is a row between ✓ Close as done…
  and the park row: the two one-press moves sit together between the two
  records, Validate first as the step before Close. It has no chord,
  because no list chord is the nearest act to borrow (as `ctrl+t`/`ctrl+f`/
  `ctrl+x` are for done/freeze/remove), and the ask was for a menu action.
  ◎ was chosen because no other row or mark wears it.
- **`nexthover.go`**: a comment only. The card's header already carries the
  section (`N-017 · Validate · value medium`).
- The batch composer's Next List tab needed nothing. It groups by
  `Section` generically, so Validate items appear there under their own
  heading.

## Tests

- `TestNextMoveToValidate` (`nextedit_test.go`). Validate is made below Open
  and before Roadmap. Items arrive verbatim in ID order from Open and from
  Roadmap, and the Roadmap is left with its prose. A round trip leaves the
  original file plus an empty `## Validate`. A Validate item closes straight
  to Closed.
- `TestNextMoveRefusals` also covers "already in Validate" and a
  Non-goals item refused with the new wording.
- `TestNextListShowsValidate` (`nextlist_test.go`). It covers parse order
  Open → Validate → Roadmap with the section's prose not leaking into an
  item, the heading's `2 open · 1 to validate · 1 roadmap`, and the headings
  drawn in that order.
- `TestNextValidateFromTheMenu` (`nextmove_test.go`). Open goes to Validate
  through the menu, the labels flip (⇡ Move to Open, and ⇣ Move to Roadmap
  on the park row), then Validate → Roadmap → Validate → Open, ending on the
  original file plus the empty section.
- `go test ./...` was green before both commits. Two throwaway tests (deleted)
  parsed the real file, giving 8 open, 9 to validate and 1 roadmap, and a
  dry-run move of N-084 into Validate landed in ID order. A rendered frame
  showed the section.

## Docs

- README, "The Next List": the example frame, which sections are listed and
  why Validate is, the menu diagram, the row bullets. The section is renamed
  **Closing, validating, parking and declining an item** (both anchor links
  updated). It gets a table row, a paragraph on the two toggles, and wording
  on the missing-section rule.
- `ai_docs/todo/next-list.md`: the intro paragraph on what the page reads and
  writes (four moves). The rest of that file's diff is the other session's
  split, committed with this doc.
- `.claude/skills/cats-todo-dev/SKILL.md`: the file-map rows for
  `nextlist.go`, `nextedit.go` and `nextmove.go`, and the Next List chord
  line (◎ is menu-only).

## Release

- `b779863 feat(next): list the Validate section and move items there from the menu`.
  It staged only this session's hunk of `next-list.md` (via
  `git apply --cached`), leaving the other session's split unstaged.
- `751a961 chore(release): v0.45.0`, with both version files bumped and the
  annotated tag `v0.45.0 — Next List: a Validate section, and ◎ Move to Validate on the item menu`.
  `git push origin main v0.45.0`.
- `catctl plugin update rohanthewiz.cats-todo` rebuilt the installed plugin
  at v0.45.0. Running cats-todo panes keep v0.44.0 until relaunched.

## Next

Closed: None. Declined: None. Raised: None.
Deferred: None. Promoted: None. Moved: None.
Updated: N-082 (now four moves: adds ◎ Move to Validate and its ⇡ back,
`ctrl+f` from Validate, the 18-row menu, and the user's `go run` look).
Full list: `ai_docs/todo/next-list.md`.
