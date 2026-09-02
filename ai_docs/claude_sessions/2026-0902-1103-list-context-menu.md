# Session: the list's context menu — v0.24.0

Session ID: `70690701-14f4-4c6f-a7b1-d24040917538`
Date: 2026-09-02

The ask, in full:

> "I need to be able to right click on a todo in the list and do a few things
> from the todo's context menu"

then, mid-build:

> "I want to be able to set/unset priority and quick win from the context menu.
> Are those included?"

and finally:

> "While you are there I do want the option to mark a done todo as 'not done' in
> case it was set as done by accident"

Touched: `menu.go` (new), `listmenu.go` (new), `listmenu_test.go` (new),
`promptmenu.go`, `ui.go`, `model_test.go`, `formimages_test.go`, `README.md`,
and the two version places.

## The box was already written — it was just welded to the editor

`promptmenu.go` already had everything a context menu needs: placement below and
right of the pointer with a flip when it would fall off the bottom, hit-testing
that treats the border as inside-but-inert, a keyboard walk where anything that
is not a move or a press closes the box and is spent doing so, the
dim-rather-than-omit rule, and a lipgloss compositor overlay so the menu floats
over its own context instead of replacing it.

None of that is about a prompt. What *is* about a prompt is which rows there are,
what makes each live, and what pressing one does.

So the first move was to lift the generic half into `menu.go`:

```go
type menuItem struct{ act int; label, hint, why string }
type menuBox  struct{ open bool; x, y, w, h int; items []menuItem; cursor int }
```

`promptMenu` now embeds `menuBox` and keeps only `word spell.Span` /
`wordAvailable` — the one thing a generic menu cannot carry, since the spell row
is aimed at the cell the press landed in rather than at the selection. Because
Go promotes embedded fields, every existing call site and every existing test
(`m.menu.open`, `m.menu.items`, `m.menu.cursor`, `m.menu.x`) kept working
untouched. `promptmenu.go` went from 316 lines to 184.

The keyboard walk became a small classifier the owner answers:

```go
switch m.menu.key(msg) {
case menuKeyPress: return m.pressPromptMenu(m.menu.cursor)
case menuKeyClose: m.menu = promptMenu{}
}
```

### One comment that was lying

`viewPromptMenu`'s doc claimed "a dim row is drawn in colFaint **with its chord
dropped**". The code drew the hint unconditionally and always had. Kept the
behaviour, rewrote the comment: a greyed row that also loses its chord reads as a
thing with no keyboard road at all, when in fact the chord exists and refuses in
the same words the row does.

## What the list menu is for

The list can do a dozen things to a prompt and the action bar has room for five.
Three — view, done, freeze — had only ever been chords, and a chord is not
something a pointer can find. The annotations were worse off: setting a priority
or a quick-win mark meant opening the edit form and finding the annotation bar, a
full round trip through a form to change a fact you were already reading off the
row.

```
╭─────────────────────────────────╮
│ ✎ Edit…                   enter │
│ ◉ View                   ctrl+v │
│ ✉ Send…             shift+enter │
│ ◷ Schedule…              ctrl+s │
│ ✓ Mark done              ctrl+t │  ← ↺ Reopen on a finished prompt
│ ❄ Freeze                 ctrl+f │  ← ☀ Unfreeze on a shelved one
│ ☐ 🍏 Quick win                  │
│ (•) Priority: none              │
│ ( ) Priority: △ high            │
│ ( ) Priority: ▲ critical        │
│ ➦ Export…                ctrl+o │
│ ✖ Delete…                ctrl+x │
╰─────────────────────────────────╯
```

Every row that has a chord prints it, so the menu doubles as the keyboard's own
reference — the same bargain the action bar's chips already make. The four
annotation rows print nothing because there is nothing to print: this menu is the
list's only road to them.

### Two ways a row can carry state, and why they differ

Done and frozen **flip their label**: the row says what the press will do, which
is the only thing a menu row ever promises.

The annotations **draw their state in the margin** instead — a `☐`/`☑` box and one
filled radio out of three. They are not flips. The priority is exactly one of
three levels, so the menu has to be able to show *which*, and pressing the level a
prompt already holds has to be a no-op rather than a toggle back off it. The
glyphs are the annotation bar's own (`annotbar.go`), which are in turn the marks
the list row draws, so one legend is taught in three places.

`"Priority:"` is repeated on all three rows rather than written once as a heading
over them. A heading would be an unpressable row sitting in a list of pressable
ones, and an eye landing halfway down a menu should not have to look up to find
out what `△ high` is high *of*.

An unreadable level — the retired `low` from an old backlog — fills no hole, the
same exact match `annotBarLayout` makes. All three levels then offer to replace
it, which is the honest reading of a level that is not one.

### The refusals are the chords' refusals

`startDrop` and `beginSchedule` already refuse in words (contract 4). The menu
resolves the same set at open time to decide what is grey, and the helpers keep
their own guards for the press — the menu's copy is what greys the row, the
helper's is what still holds if the world changed in between.

They are deliberately *not* the same set:

| | send | schedule |
|---|---|---|
| no socket | refused | refused |
| drop in flight | refused | **allowed** — a schedule is a note on the todo, not a socket call |
| frozen | refused | refused |
| done | **allowed** — a drop reopens work by handing it over | refused — a timer on closed work is a promise about something that is over |

### The gesture's edges

- The press moves the highlight first, so what the menu acts on is what the
  keyboard is parked on when it hands back.
- It takes **no drag hold** and does **not** arm the double-click timer — both are
  left-button gestures. It also calls `releaseDrag()` on the way in, for a reason
  that is not merely tidiness: a hold left armed would hand the next motion
  message to `dragOver`, which would reorder the backlog *behind an open menu*.
- Only a row opens one. Header, action bar, empty space below the rows: nothing
  opens, and a menu that is up comes down.
- A left click off the box only dismisses. The row underneath is not also
  selected and the button underneath is not also pressed — a menu that acted on
  the way out would make "never mind" the riskiest gesture on the screen.
- `backToList` and a resize both clear it, beside the editor's.

Footer gained `right-click menu`, placed **last, past `esc quit`**, so it is the
first thing a narrowing pane drops. That is the right order to lose them in: the
menu is a shortcut to chords the line already names, while `esc` is the way out.

## The third ask found a real bug

"Mark a done todo as not done" was already there — the row flips to ↺ Reopen and
presses `toggleSelected`, which `store.toggle` genuinely un-dones. But verifying
it rather than asserting it turned up the rough edge that made it unpleasant:

**`toggleSelected` never re-parked the highlight.** Completing a prompt files it
at the *head* of the done group (`store.fileAsLatestDone`), so the row travels
most of a pane — and the cursor stayed at the old index, landing on whatever slid
up into the gap. A slipped `ctrl+t` therefore became *two* mistakes: the
corrective `ctrl+t` closed a different prompt.

`freezeSelected` had always re-parked, with a comment saying exactly why. Its
twin just never did. One line:

```go
m.rebuildList()
m.selectRow(ref)   // ← this
```

The status line now also names the way back: `marked done (ctrl+t to reopen)`.
That earns its width in the case the highlight cannot help with — with the closed
fold on (`ctrl+d`) the row leaves the list altogether, `selectRow` finds nothing
to park on, and the status line is the only thing on screen saying the work is
recoverable. The README draft claimed the highlight followed it there too; that
was checked against `selectRow` and corrected before it shipped.

## A test habit worth keeping

The first draft of the menu tests set up state by poking the in-memory slice:

```go
m.project.todos[0].Done = true    // wrong
```

Every store write calls `reload()` first, which re-reads the file — so a field set
only in memory is a field the next save silently drops. Label-only tests passed
anyway (nothing wrote), which is exactly what makes it a trap. The reopen test
was the one that failed loudly, and it was right to.

They now go through a `markTodo` helper that uses `setAnnots` / `setDone` /
`setFrozen`, so every menu is built over a backlog a real launch could have
produced.

Also added `home`/`end` to `pressKey` (`formimages_test.go`), which the menu's
keyboard walk needs.

## Shipped

`go vet` clean, `gofmt` clean, full suite green (~5s). 11 new tests in
`listmenu_test.go`; `TestToggleSelected` rewritten with two todos so it pins that
the second press lands on the same prompt rather than the neighbour.

README grew a `## The list's context menu` chapter with a
`### Marking priority and quick wins from the list` subsection, the annotations
chapter now names both roads to a mark, and the manager chapter documents the
reversible done flip. Version bumped in `main.go` and `cats-plugin.toml`.
