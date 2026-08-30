# Session: alt+↑/↓ moves a line, shift+alt+↑/↓ extends the selection

Session ID: `e26d76e8-d1e7-4824-9c6a-395d961c5906`
Date: 2026-08-29

Second half of the same session as
[the editor's context menu](2026-0829-2028-prompt-editor-context-menu.md), which
should be read first — the two features share a file family and the second leans
on `promptSelRows` from the first.

The ask:

> "Alt is already coming through to the prompt editor, now let's allow
> - ALT+DOWN_ARROW, ALT+UP_ARROW to move the current line accordingly
> - SHIFT+ALT+DOWN_ARROW, SHIFT+ALT+UP_ARROW to extend the selection down or up
>   accordingly"

New: `promptmove.go`, `promptmove_test.go`.
Touched: `promptsel.go`, `ui.go`, `README.md`,
`.claude/skills/cats-todo-dev/SKILL.md`.
No version bump — still `0.19.0` in both places, as asked last time.

## The one interesting thing: the two chords fight over the same bit

Before this, `promptSelectionKey` had a single job — take shift off a motion and
hand the textarea the plain key, so the library owns what `↓` does at a soft wrap
and this program only tracks the anchor. It matched on `Key.Code`, so *every*
modifier combination came along for free:

```go
if msg.Mod&tea.ModShift == 0 || !promptMotionKeys[msg.Code] { return msg, false }
msg.Mod &^= tea.ModShift
```

That generosity is exactly what breaks the moment `alt+↓` means something. With
alt left on, `shift+alt+↓` would be stripped to `alt+↓` — the line move — so
"extend the selection down" would have **dragged the line out from under the
highlight**. So alt now comes off too, and only from the vertical pair:

```go
if msg.Code == tea.KeyUp || msg.Code == tea.KeyDown {
    msg.Mod &^= tea.ModAlt
}
```

Only that pair, because horizontally `alt` is the textarea's *own* word motion
(`WordBackward` = `alt+left`, `WordForward` = `alt+right`) — which is precisely
what `shift+alt+←/→` is for. Stripping it there would silently demote word
selection to character selection. Both halves are pinned by tests
(`TestShiftAltArrowsSelect`, `TestShiftAltLeftRightStillSelectsWords`), and
`promptLineMoveKey` refuses a shifted press for the same reason from the other
side.

Worth noting the user's stated motive for wanting the alt spelling at all:
distinguishing `shift+↓` from a bare `↓` needs the kitty keyboard protocol, and
`shift+alt+↓` is the spelling a terminal is more likely to actually send. It is
an alias, not a second behaviour — `shift+alt+↓` does exactly what `shift+↓`
does.

## Where the handler had to go

The third chord to learn this lesson, after `ctrl+c` and `ctrl+x`. `updateForm`
calls `m.clearPromptSel()` on the way in for every key that is not part of a
selection gesture, and `alt+↓` is not one — so a handler in the switch below
would find the highlight already gone and could never move a swept block.

The rule is now written down in the skill doc: **anything that reads the
selection is answered above `clearPromptSel()`**, and keeps a case in the switch
below as well, reached only when there is nothing to read and where it explains
itself ("moving lines works in the prompt", from the title or the annotation
bar).

## Moving a block, and carrying the highlight exactly

The naive version moves a line and re-selects the whole thing. This carries the
selection *as it was*, mid-word ends included, which falls out of one
observation: **the block's text does not change, only where it begins.** So one
number carries the caret and both ends of the span:

```go
before, _ := promptRowSpan(rows, first, first)                    // old start
after,  _ := promptRowSpan(moved, first-lo+delta, first-lo+delta) // new start, within the block
shift := (start + after) - before
```

Then `anchor+shift` and `caret+shift`, and nothing else has to be recomputed.

```
alpha                       gamma
beta      alt+↓ on rows     alpha      selection "pha\nbet"
gamma     0..1 (swept) ──▶  beta       still exactly "pha\nbet",
delta                       delta      start 2 → 8
```

The reorder itself is a slice holding exactly the block *and its one neighbour*,
so "move the block one place" is "the neighbour hops over it" —
`movePromptRowBlock`, four lines each way, with `copy` doing the overlap safely
(it moves the range as a whole rather than element by element). It has a table
test of its own because that is the one place an off-by-one swaps the wrong pair.

The whole edit is then a **single contiguous replacement** through
`replacePromptRunes` — the block plus the row it swapped with — which is the same
editing road the split, the sort and the column mode already take.

## Smaller decisions

- **A logical row, not a drawn one.** A soft-wrapped paragraph moves whole, the
  rule `duplicatePromptLine` already follows: a wrap segment is not a boundary
  the value contains. Tested at width 30 with a row that wraps three times.
- **The boundaries refuse in words** — `already at the top`, `already at the last
  line`. Slightly chatty for a held key, but contract 4 is contract 4, and a
  chord that goes silent on the boundary reads as a chord that stopped working.
- **The column mode is unaffected by design.** `alt+↑/↓` is not one of the keys
  `updatePromptCarets` owns, so it hands the key back, the mode ends, and the key
  then does its usual job — which is the documented contract for that mode, and
  the same thing plain `↑`/`↓` already did.
- **Footer:** `alt+↑/↓ move line` goes in *before* `cmd+d dup line`. The
  comment there says why the duplicate takes the very end — it is the one segment
  a terminal may be unable to send at all — and this one always arrives, so it is
  the wrong one to lose first.

## The skill doc was stale, and it is tracked

`.claude/skills/cats-todo-dev/SKILL.md` turned out to be a tracked file, not
dogfood scratch, and its key-chord list opens with *"check before binding
anything"*. The context-menu commit had left it behind. It now carries:

- the six new files in the map (`promptmenu`, `promptsplit`, `promptsort`,
  `promptcarets`, `promptmove`, `promptlines`);
- the new form chords, including right-click and the fact that the spell ask no
  longer has a direct road;
- the two modal states that live on the form stage rather than a stage of their
  own, and that both are answered at the top of `updateForm`;
- the above-`clearPromptSel` rule;
- **the footer's budget**: seven standing segments = exactly 118 cells, which is
  what keeps `tab switch field` in a 120-cell pane and `ctrl+l spelling` at 160,
  both pinned by tests. A new segment must be contextual or go at the tail. That
  number was rediscovered by measurement twice in one session; writing it down
  should be the last time.

## Verification

`gofmt -l`, `go build`, `go vet`, `go test ./...` all clean. Throwaway probes
(written, run, deleted) confirmed the caret column after a move, the block move
with a partial selection, the boundary refusals, that `shift+alt+↓` grows the
span without touching the value, and the footer's three tiers at 120 / 160 / 200
columns.
