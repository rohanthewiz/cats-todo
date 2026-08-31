# Session: a line can carry several carets — v0.22.0

Session ID: `07a1f77a-aa6f-4a19-abed-08f679bf7969`
Date: 2026-08-30

Third report on the same gesture, and the one that ended it. Read the two
before it first — they are what made this one short:

- [tracing the alt bit](2026-0830-2003-alt-click-carets-did-not-work.md)
- [the silent refusal](2026-0830-2136-alt-click-the-silent-refusal.md)

The report this time came with the detail that settled it:

> "In cats-todo, in the prompt editor, I am unable to do multiple cursors with
> ALT+LEFT_CLICK. A couple attempts have already been made in cats-todo itself,
> but we are already achieving this in the CEd editor running in cats."

Touched: `promptcarets.go`, `promptcarets_test.go`, `README.md`, and the two
version places.

## The measurement the first two sessions could not make

Both earlier sessions ended on the same open question — does the alt bit reach
the pane? — and both left it as a command for the user to run. It got run here.
`catctl split v` into the visible workspace, `catctl rename-pane`, then:

```sh
printf "\033[?1002h\033[?1006h"; cat -v
```

The user clicked, then option-clicked. `catctl capture` of the probe pane:

```
^[[<8;15;7M^[[<8;15;7m^[[<0;2;3M^[[<32;3;3M…^[[<0;8;3m
     ▲ option+press: 8 = left(0) + alt(8)
```

**The alt bit arrives.** Every link in the two earlier sessions' chain was
sound, and the bug was in this repo the whole time — in the one place both
sessions looked at, called correct, and moved past.

Worth keeping: **a probe pane is a two-minute experiment, not a paragraph of
instructions.** `catctl split` + `catctl capture` puts a real terminal under a
real pointer, and the earlier note about needing the workspace focused first was
right — splitting the *focused* pane is what makes it visible.

## The rule that was wrong

`promptCarets` held `rows []int` / `cols []int` with at most one caret per row,
enforced by `indexOf(row)`. Every consequence of that followed honestly: a press
on a row that already had a caret moved it, and `altClickPrompt`'s `cr == r`
branch called a press on the caret's own line a plain move.

What the second session found and named but did not fix:

> A line holds one caret. Real multi-cursor editors let alt+click drop a second
> cursor *on the same line*, and on a wrapped paragraph that is very likely what
> the hand is asking for.

That is the bug, and the ced comparison is what makes it obvious rather than
arguable. `internal/editor/multicaret.go` keys carets on `Position` — line *and*
column — and refuses only a caret exactly on the primary. **ced edits source
files: many short real lines, so alt+click lands on a different row every time
and the row-keyed rule never bites.** A cats-todo prompt is one long paragraph
soft-wrapped across half the box — every display line the hand aims at is the
same logical row. Same gesture, same wire bytes, opposite outcome, entirely
because of what the two buffers look like.

So the rule did not merely have an awkward corner. It disabled the gesture on
the *only* shape this editor is normally used with, which is why three reports
in a row said "does nothing" while every test passed.

## The change

A caret is a cell. `rows` may repeat, and the pair stays sorted by (row, column).

- `indexOf(row)` → `indexAt(row, col, runes)`, comparing the *effective* column
  so a goal stranded past a short row is still reachable where it is drawn.
- `add` inserts on the pair (linear scan; a caret set is a handful of entries and
  the sort key is now spread across two slices).
- `altClickPrompt` refuses exactly one press: the one on a cell that already has
  a caret. Everything else adds.
- `editAtCarets` walks carets **backwards** and takes a goal delta back from
  `fn`. Two adjustments, additive and disjoint: a caret takes its own delta once,
  plus one shift per edit to its left on the same row.

  ```
  "alpha bravo", carets at 5 and 11, typing "!"
    i=1 (col 11): "alpha bravo!"   goal 11 → 12
    i=0 (col  5): "alpha! bravo!"  goal 5 → 6, and caret 1 shifts +1 → 13
  ```

- `dedupe`, called once at the tail of `updatePromptCarets`. The motions are what
  make it necessary — `ctrl+a` sends every goal on a row to 0, which is two
  carets asking to be in one place, and a set holding it twice types every
  character twice. Converging backspaces do the same thing (`"abcd"` with carets
  at 0/1/2 → `"cd"`, all three at 0 → one caret, mode ends).

## Verified the way this feature is now always verified

The pty harness from the first session, rebuilt, driving the **real binary** with
the exact wire bytes on the fixture that had been failing — one line of
`"wrap " × 60` in a 120-cell pane, two alt+presses on two *display rows of that
one logical line*:

| binary | note it drew |
| --- | --- |
| shipped v0.21.1 | `the caret is already on that line — alt+click another line` |
| this build | `2 carets · alt+click adds or removes one` |

Same bytes, same fixture, opposite answers. Harness kept in the session
scratchpad (`creack/pty`, writes `.cats-todo/todos.json` directly rather than
typing a todo in — the store is a plain JSON array, which makes fixtures free).

Three tests changed sides, which is the honest signal that the rule itself moved
and not just its edges: `TestAltClickMovesTheLinesCaret` became
`TestAltClickAddsASecondCaretToARow`, `TestAltClickOnAWrappedLineSaysSo` became
`TestAltClickOnAWrappedLineAddsACaret`, and `TestAltClickRefusesInWords` now
pins the cell-sized refusal. New: `TestAltClickTwoCaretsOnARowDelete` and
`TestCaretsOnARowFoldWhenTheyMeet`.

## The one that got away, twice

Both earlier sessions read `altClickPrompt`'s `cr == r` branch, and both wrote
down that it was correct — the second one even improved its wording. It was
correct *given* the row-keyed set, and the set was never the thing under
suspicion because the wire was so much more suspicious. The tell was available
the whole time and was in the user's own report: **the same gesture worked in
ced.** Two programs, one terminal, one modifier — that rules out the wire in one
sentence and points at the two models' difference. Reach for the working
neighbour before the packet capture.

## Still true, and worth repeating

**A running plugin pane is a running old process.** Four `cats-todo` processes
were live during this session, started 18:47 Aug 29 through 22:18 Aug 30 — three
of them predating v0.21.1, two predating alt+click existing at all. `catctl
plugin update` rebuilds the installed binary; it does not touch a pane that is
already open. Any "still doesn't work" needs the pane restarted first.

## Released

`53b908d`, pushed to main, released as **v0.22.0**, and `catctl plugin update
rohanthewiz.cats-todo` run so the installed plugin carries it. The pty harness
was then re-run against the *installed* binary — not just the repo build — and
it draws `2 carets`, which is the check the first session's two stale binaries
are the reason for.

The four panes above were still on their old processes at hand-off; the user was
told which, and that a pane has to be closed and reopened to pick this up.
