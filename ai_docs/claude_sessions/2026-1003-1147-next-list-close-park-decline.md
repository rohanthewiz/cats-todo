# Session: close, park and decline items from the Next List page

Session ID: b24b465b-c197-403c-869f-8c83dbe0902a
Date: 2026-10-03
Not released (a minor: v0.44.0 when it is).

## The ask

The user dropped this from the backlog: on the Next List, give options to
properly close an open item as done (perhaps with a comment), move an item to
the Roadmap, and mark (move) an item as a Non-goal.

## What was built

Until now the page only read `ai_docs/todo/next-list.md`. It now writes three
moves, and nothing else. It never re-rates, rewords, renumbers or deletes an
item.

| Chord | Menu row | Effect |
|---|---|---|
| `ctrl+t` | ✓ Close as done… | a pad asks "what showed it's done" (optional), then the item goes to the top of Closed |
| `ctrl+f` | ⇣ Move to Roadmap / ⇡ Move to Open | one press; the lines move verbatim, in ID order; the same chord moves the item back |
| `ctrl+x` | ⊘ Mark as non-goal… | a pad asks "why we won't do it" (optional), then the item goes to Non-goals in ID order |

The chords are the list's own for the nearest act (done, freeze, delete).
The three menu rows sit after the ⤓ Add rows and before the copies.

### Files

- `nextedit.go` (new) is the one write path, and pure:
  - `moveNextItemText(src, nextMoveReq)` re-parses the file, finds the item's
    block by ID in Open/Roadmap (`findNextBlock`, the parser's own
    continuation rule), cuts it, and collapses the doubled blank line.
  - For Open/Roadmap the block travels verbatim. For Closed/Non-goals it is
    built as a record (`nextRecordBlock`):
    `- **N-###** · closed|declined <date> · raised \`<stem>\``, with a
    `  — <note>` body wrapped at 76 columns. With no note, the item's own
    text becomes the body, so a record never loses what the item was.
  - `insertNextBlock` puts Closed on top and everything else in ID order.
    Open and Roadmap are blank-separated; the record sections are packed.
    A missing section is created in the file's order
    (`insertNextSection`).
  - A CRLF file stays CRLF.
  - `moveNextItemFile` reads the file fresh, splices it, and writes it via a
    temp file and a rename, keeping the file's permissions.
- `nextmove.go` (new) is the page side:
  - The chords (`closeFromNext`, `parkFromNext`, `declineFromNext`).
  - `nextNotePad`: the ⚑ pad's shape. It is anchored at the menu's cell when
    opened from the menu, and centred when opened by a chord. It owns every
    key: `enter` moves, and `esc`, `ctrl+c` or a click off the pad leaves the
    item where it was.
  - `moveNext` writes, then re-reads the page. It keeps the highlight on a
    parked item and moves it to the `neighbour` after a close or decline.
  - On Close it also marks the open backlog copy done (`nextBacklogCopy`).
    On a decline the copy is left alone, and the heading says it is still
    open.
- `nextlist.go`: `reload` was split into `reread(width, height, keep)`. It
  also has the three chords, the pad routing, and footer segments
  (`ctrl+t done · ctrl+f roadmap · ctrl+x non-goal`).
- `nextmenu.go`: three rows (`nextMenuClose`, `nextMenuPark`,
  `nextMenuDecline`). The park row's label comes from `nextParkLabel`. The
  pad opens at the menu's `x, y`.
- `ui.go`: the `nextPad` field. Resize re-places the pad, `backToList`
  clears it, `forward` sends the blink and paste to the pad's field, and the
  view composites the pad on top.
- `nexthover.go` and `listhover.go`: no hover card while the pad is up.

### Design points

- **A splice, not a re-render.** The file is edited in other panes, so the
  write re-reads it at the moment of writing and keys on the ID. Every line
  it doesn't own is left as it is. An item already moved elsewhere is
  refused in words ("N-001 is no longer Open or on the Roadmap … ↻
  refresh").
- **Pads only for the record moves.** The pad's `enter` is also the
  confirmation for a final decision. Open ⇄ Roadmap is reversible with the
  same chord, so it takes one press. If the pane is too small for the pad,
  the move is refused rather than made without asking.

## Tests

- `nextedit_test.go`:
  - Closed on top with the comment wrapped.
  - Without a comment, the item's text becomes the body.
  - Non-goals packed in ID order, both mid-section and at the end.
  - Open ⇄ Roadmap both ways, with an exact round trip back to the
    original file.
  - An empty section, and missing sections created in their place.
  - Refusals.
  - CRLF kept.
  - The file route keeps the mode and leaves no temp file behind.
- `nextmove_test.go`:
  - `ctrl+t` end to end: the pad owns keys, the file is written, the
    highlight moves to the neighbour, and the page is re-read.
  - Close marks the backlog copy done.
  - `ctrl+f` toggles and restores the file byte for byte.
  - The menu's decline pad is anchored at the menu, and `esc` writes
    nothing.
  - A click off the pad dismisses it, and `ctrl+c` doesn't quit.
  - A refused move after another writer changed the file.
- `go test ./...` is green. Frames were checked by rendering them (page, pad
  and menu). It has not been live-tested in cats yet (N-082).

## Docs

- README: a new "Closing, parking and declining an item" section, plus the
  menu diagram and bullets, and the send paragraph's "closing is left to the
  file".
- `cats-todo-dev` skill: file-map rows for `nextedit.go` and `nextmove.go`,
  and the Next List chords.
- `next-list.md` preamble: it now says the page writes these three moves,
  and how the records it writes look.

## Next

Closed: None. Declined: None. Raised: N-082.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
