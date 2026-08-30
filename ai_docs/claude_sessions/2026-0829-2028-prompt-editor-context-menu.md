# Session: a context menu on the prompt editor — split, sort, a caret per line

Session ID: `e26d76e8-d1e7-4824-9c6a-395d961c5906`
Date: 2026-08-29

The ask arrived in three messages, each one widening the last:

> "In the prompt editor I would like the ability to make individual prompts from
> a selected bulleted list (markdown format). If all of the prompt body is
> selected then make all new notes from the existing and delete the existing."

> "For the right click pls use a context menu. Perhaps another item could be sort
> or drop a cursor on every line. Please take a shot at implementing those also."

New files: `promptsplit.go`, `promptsort.go`, `promptcarets.go`, `promptmenu.go`,
`promptlines.go` (plus a test file each).
Touched: `ui.go`, `promptsel.go`, `spellpanel.go`, `store.go`, `styles.go`,
`README.md`.
**No version bump** — asked for explicitly; the release commit is its own step.

## The shape that fell out

One gesture, three things a swept run is worth:

```
                      sweep a run in the prompt
                                 │
                    right-click ─┴─ ctrl+x (the split only)
                                 │
                    ╭────────────┴───────────────╮
                    │ ✂ Split into prompts       │  promptsplit.go
                    │ ⇅ Sort lines               │  promptsort.go
                    │ ⌶ Caret on every line      │  promptcarets.go
                    │ ✓ Spelling…                │  spellpanel.go (moved onto it)
                    ╰────────────────────────────╯
```

They compose in that order on purpose, and the README says so: carets turn plain
lines into a list, the sort puts the list in an order, the split turns the list
into backlog prompts *in that order* — because `addAfter` keeps it.

## Decisions worth keeping

### The chord: `ctrl+x`, asked rather than guessed

`AskUserQuestion` offered `ctrl+x`, `cmd+p`, and `ctrl+x` + a toolbar chip. The
user took the bare `ctrl+x`. It is free in *both* the textarea's keymap and the
textinput's — the survey the `ctrl+r` comment in `updateForm` already makes — and
it is already this program's "take this out" (delete on the list, remove in the
attachment editor). No chip: the form's toolbar is already seven chips and drops
to glyphs sooner than the list's.

Inheritance was the second question: **scope, annotations, session options — not
attachments**. An image belongs to the prompt it illustrates; N bullets would put
N copies on disk. When a whole-body split deletes an original that had
attachments, the status line says so rather than letting them go quietly.

### Where the split's selection semantics come from

Both rules are the user's second sentence, generalised:

- Only the selection is consumed, **and only from its first bullet on**. A sweep
  that caught the sentence above the list has not asked for that sentence to
  become a prompt or to vanish — `splitBulletList` returns a `head` rune count so
  the caller can leave it alone.
- **What is left behind decides the original's fate.** Text survives → the form
  stays open on it. The list *was* the whole body → nothing is left to be a
  prompt, so the original is deleted and we land back on the list.

A nested list stays with its parent: splitting "write the notes" from "link the
diff" leaves two prompts, neither of which says the whole task.

### The footer fight, and why the hint is contextual

This is the one place the design was forced by measurement rather than taste.
The form's caret line is **exactly 118 cells** for its seven standing segments,
which is what lets `tab switch field` survive a 120-cell pane. The spell test
also pins `ctrl+l spelling` at 160. Doing the arithmetic:

```
first 7 segments:  118   ← must fit 120
+ ctrl+g scope:    133
+ ctrl+l spelling: 151   ← must fit 160
```

Nine cells of slack at 160, and none at 120. A permanent eighth concept could
only be bought by tightening the seven already there past what they can say. So
the hint is **shown only while something is swept** — the same principle
`ctrl+g scope` already follows ("advertised only where it works") — and it names
the *menu*, not the chord:

```
right-click: split/sort/carets
```

because the menu prints `ctrl+x` on its own ✂ row. One gesture on the footer
teaches every key behind it, which is the whole reason those three live on a menu
instead of on three chords nobody would guess. While the mode is on, the column
editor takes the footer outright — the keys mean something different for as long
as it lasts.

### Rendering the menu: lipgloss's compositor, not a string splice

The instinct was to reuse `paintPromptSelection`'s technique (cut with
`ansi.Cut`, splice, re-render the tail). That works *inside one editor line*,
where the only styles in play are the textarea's. It does not work for a box
that lands anywhere on the frame: `ansi.TruncateLeft` drops the escapes opened
before its cut, so every form line's tail right of the box would lose its colour.

`charm.land/lipgloss/v2` v2.0.5 has `Canvas`/`Layer`/`Compositor`, which merges
cell by cell and keeps both sides' styling. Probed it before building on it:

```go
lipgloss.NewCompositor(
    lipgloss.NewLayer(view),
    lipgloss.NewLayer(m.viewPromptMenu()).X(m.menu.x).Y(m.menu.y).Z(1),
).Render()
```

Note `Compositor`, not `Canvas.Compose(layer)` — `Layer.Draw` ignores its own
`x`/`y`; only the compositor's `flattenRecursive` applies absolute positions.

It is composited in **`renderStage`**, not inside `viewForm`: every row constant
the form hit-tests against is measured on the frame underneath, and a menu
spliced in earlier would move the toolbar out from under the pointer for as long
as it was open. A test pins the line count either way.

### Dim rows rather than a shrinking menu

An item that cannot act on the current selection is drawn dim, keeps its place,
and says why when pressed. *"Why is this one grey" is a question the program can
answer; "where did that item go" is not.* The cursor opens on the first live row,
so `enter` straight after the click is never a refusal.

Every "why" is resolved **once**, when the menu is built, from the state the
press landed in — what the menu draws and what pressing it does cannot come from
two different reads of the selection.

### The spell ask moved onto the menu, and lost its direct road

`rightClickSpell` used to be the right button's whole meaning. It would have
become dead code, so instead of deleting it the logic was split where it belongs:

- `spellAskAt(x, row) (span, ok, why)` in `spellpanel.go` — the word the pointer
  is on, or which of the two reasons there isn't one.
- `openSpellPanelOn(span)` — the hand-off, ✚ Add row leading.

`openPromptMenu` calls the first; `pressPromptMenu` calls the second with the
span it already stored. This costs the old muscle memory one keystroke
(right-click → `enter` → `enter` to add a word) and is the honest price of one
gesture opening one menu. The spell tests' `rightClickPrompt` helper now drives
the menu past itself; `promptmenu_test.go` drives it with real keys and clicks.

### Column mode: a goal column, not N textareas

The library has one caret and no notion of a second. Rather than fight it,
`promptCarets` keeps `{rows []int, col int}` and edits the value directly — the
same road `replacePromptRunes` already takes. The library's own caret is *parked*
on the first of the mode's rows (`syncPromptCaret`), so the caret the textarea
draws **is** one of the mode's; the overlay draws only the rest.

```
sweep three plain lines      carets go down            type "- "
  tag v2                       ▌tag v2                   - tag v2
  write the notes              ▌write the notes          - write the notes
  announce it                  ▌announce it              - announce it
```

The column is a **goal** column: a row too short takes its caret at its end and
is not stranded there when the others move on. That is what makes a ragged block
prefixable in one gesture, and it is the rule `↑`/`↓` already follow everywhere.

Two things the mode deliberately does *not* do:

- `enter` ends the mode and does **not** also insert — it is the key most likely
  to be pressed *because* the user thinks the mode is already over.
- A chord it has no meaning for ends it and then takes its ordinary path, so
  `ctrl+s` still saves from inside without being listed anywhere in it.

`editAtCarets` edits a split `[]string` and writes back once, rather than calling
`replacePromptRunes` per row: every edit but the first would otherwise be aimed
at offsets the edit before it had already moved.

### The one rendering trap: overlapping paints

`paintPromptSpans` requires **sorted, non-overlapping** runs — two claiming one
cell would emit that character twice and push the rest of the line right. A caret
lands inside a spell mark all the time, so `mergeCaretPaints` cuts each caret's
single cell out of any run it falls in before appending. Carets win the overlap:
a cell is either where the next character goes or it is not.

### Sort: markers stay, bodies move

The refactor that made this cheap was turning `splitBulletList`'s items from
`[]string` into `[]bulletBlock{marker, content, body}`. The split wants only
`body`; the sort wants all three, because it puts the sorted bodies back into the
markers **where they were**:

```
1. write the notes        1. announce it
2. announce it     ──▶    2. tag v2
3. tag v2                 3. write the notes
```

That renumbers an ordered list correctly instead of shuffling the numbers along
with the text, is a no-op for `-` markers, and re-indents continuation lines to
whichever marker they land under (so `9.` and `10.` both line up).

Which sort runs is decided by the text, not by a mode: two or more markdown items
→ sorted as items; anything else → sorted as lines. That is the same reading
`splitBulletList` already applies, so the two menu items agree about what "a
list" is. Blank lines collect at the **end**, not the top — a separator has
nothing left to separate once the order changed.

### `promptlines.go` exists because of one off-by-one

Both the sort and the carets need "which whole rows did this sweep name". The
trap is a downward drag that ends exactly at a row's first character: it has not
put a single character of that row under the highlight, so `promptRowRange` steps
`hi` back one rune before placing it. Written once rather than twice, with a
table test on the boundaries — it is the kind of bug that only shows up on a
block's first or last line.

## Store

`store.addAfter(afterID, []Todo)` — one write, splicing behind the anchor;
appends when the id is empty (add mode) or gone (another pane deleted it while
the form was open). A loop of `add()` would reload N times and, worse, recompute
the insertion point against a list it is itself changing. Array order is the
user's order, so prompts born from an item belong where that item was.

## What every refusal says

Rule 4 of the repo's contracts, applied throughout — each of these lands on the
form's note line, and a refused split *keeps the highlight* so the run is still
there to re-sweep:

| Situation | Words |
|---|---|
| `ctrl+x` with nothing swept | `nothing selected — sweep a bulleted list, or hold shift with ←/→, then ctrl+x` |
| a sweep with no bullets | `no bulleted list in the selection — items start with -, * or + (or 1.)` |
| any of the three from the title | `…works in the prompt` |
| sort/carets on one line | `sweep two or more lines…` |
| backspace with every caret at column 0 | `the carets are at the start of their lines` |
| whole-body split, delete failed | `split out N prompts, but removing the original failed: …` |

## Verification

`go build`, `go vet`, `gofmt -l` and `go test ./...` all clean. Throwaway probe
tests (written, run, deleted) rendered the menu over a real form, the three
carets with `- ` typed into them, and each sort case — the compositor output and
the reverse-video caret cells were read directly rather than assumed.

## Left for later

- The version bump. `main.go`'s `const version` and `cats-plugin.toml`'s
  `version =` are both still `0.19.0`; this is a minor-bump feature set when it
  is released.
- The menu is the editor's only one. If the list stage ever wants a right-click
  menu, `promptMenu` is close to general — `openPromptMenu` is the only part that
  knows about prompts.
