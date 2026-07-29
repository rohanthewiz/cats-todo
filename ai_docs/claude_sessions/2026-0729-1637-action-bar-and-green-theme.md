# Session: an action bar under the filter, and cats' muted green theme

Session ID: `c5314cfc-91e3-4542-80b3-93a2d9cbe4bf`
Date: 2026-07-29

## The ask

Two things, committed separately:

1. "Some action buttons below the search filter. For example: Add, Edit, Send (to
   Claude), Delete."
2. "Theme cats-todo with a muted green theme like the cats project at `~/projs/go/cats`."

## Where the green actually lives

Worth recording, because it is not where you would look first. The cats repo has
no lipgloss anywhere — it is not a Bubble Tea app. Its theme is a **web
palette**: `internal/config/config.go`'s `defaultColors`, the `:root` custom
properties served into `cmd/catway/web/index.html`.

```
bg #1f2420   fg #d6ddd6   accent #4db380   accent-dim #3d4a43
panel #242a25   panel2 #2b322c   line #38403a   muted #9db0a2
chrome #2b322c   chrome-focus #3a4a3f
ok #6ac47a   warn #e0b64e   err #e57373
```

Both `config.go` and `index.html` carry the values, each telling the other to
stay in sync — so `styles.go` is now the third copy of the same table, and says
so in a comment.

## Commit 1 — the action bar (`fdfe19d`)

### What it is

A row of chips between the query line and the rows:

```
  + Add ctrl+a   ✎ Edit enter   ➤ Send alt+enter   ✕ Delete ctrl+x
```

Each chip carries **both the label and the chord it stands for**. The buttons
mirror bindings rather than replacing them — a button that names its shortcut
teaches the keyboard path instead of competing with it. When the row would not
fit the pane, the hints drop and the labels stay; the footer still names every
chord.

### The interaction model, and why

`tab` walks the focus out of the query box, across the buttons, and **back into
the box** rather than sticking on the last one — the query box is one stop on
the ring, at index `-1` in `moveActionFocus`. `shift+tab` walks the other way,
`←`/`→` move along the row (only while focused; otherwise they belong to the
input's cursor), `enter` presses.

Two decisions that make the bar usable rather than decorative:

- **`↑`/`↓` keep driving the row highlight while a button is focused.** The
  natural motion is pick a prompt, then press the button that acts on it. If
  focus on a button stole the arrows, every use would be tab → arrow → tab.
- **Typing hands the focus back to the filter.** Anything not otherwise bound
  falls through to `list.editQuery`, and it clears `actionFocus` on the way. A
  lit button while characters land in the query box would show the focus in one
  place and put it in another.

`esc` gained a stop in front of its existing ladder: leave the bar → clear the
filter → quit. It is the "undo the state I'm in" key, and quitting is the last
resort.

### Dead controls

`beginEdit` / `beginDrop` / `beginDelete` all `return m, nil` when nothing is
highlighted. On a keystroke that is fine — nothing visibly happened because you
pressed a key that did not apply. **On a button it reads as broken.** So
`runAction` checks `needsSel` first and reports `"highlight a prompt first —
↑/↓ to choose one"`, and those three chips render in `btnOffStyle` (greyed, on
the recessed `panel` surface) whenever there is no selection.

### Mouse: considered, not done

"Buttons" implies clicking, and bubbletea v2 supports it (`tea.View.OnMouse`).
Skipped deliberately: enabling mouse reporting takes native text selection away
from the pane, and cats-todo lives inside a terminal multiplexer where
select-to-copy is ordinary. The bar is keyboard-driven; mouse can be added later
if it is missed.

### Mechanics

`fuzzyList.view` grew a second parameter — `view(emptyMsg, bar string)`. The bar
is passed in **pre-rendered** rather than built inside the component, because
the buttons act on the manager's model, not on the list. The target picker
passes `""` and renders exactly as before (`"\n\n"` either way when the bar is
empty).

Zero-value care: focus is `actionFocus bool` + `actionIdx int` rather than an
`actionIdx == -1` sentinel, so a zero-value model starts where every launch
should — typing into the filter.

Five tests in `ui_test.go`: the tab ring (including wrap in both directions and
the typing reset), dispatch to the add form and the delete confirm, the
selection-less message, `esc`'s order of business, and the wide/narrow render.
`pressKey` in `formimages_test.go` gained `tab` / `shift+tab`.

## Commit 2 — the muted green theme (`7c5a52d`)

The hexes moved into named constants at the top of `styles.go` (`colBg`,
`colAccent`, …) so the next sync with cats is a diff against one table rather
than a hunt through the styles.

**Two deliberate departures from cats' map**, both documented in the code:

- **The greys are ours.** cats' `panel` / `panel2` / `chrome` / `chrome-focus`
  are surfaces for a web page — they sit within a few points of each other and
  cannot separate a terminal's four tiers of *text*. The
  name → description → footer → done ramp is interpolated down from `fg` toward
  `line` instead: `#d6ddd6` / `#7d8f83` / `#5f6f64`, with `muted #9db0a2` kept
  for group headings.
- **`matchStyle` stays amber** (`warn #e0b64e`). It is the one thing on screen
  that must *not* read as part of the green ramp — a fuzzy hit inside a name has
  to jump out of the letters around it, and warm-against-green is what does
  that. It replaced `#F2A900`, which was already amber; the palette change did
  not touch its job.

`headingStyle` lost its purple (`#9D7CD8` — there is no purple anywhere in cats)
and became bold `muted`, which reads as a quiet group label rather than a fifth
accent.

### Setting the terminal's ground

`View()` now sets `v.BackgroundColor` / `v.ForegroundColor`, so the palette
reads as a *theme* rather than as green text on whatever the pane happened to
be.

Those are properties of the **terminal**, not of this process — the same hazard
as `WindowTitle`, which `runTodoUI` already guards. Bubbletea v2 resets all
three when it tears the view down; the existing OSC-2 backstop for an exit that
skips the renderer grew OSC 111 and OSC 110 alongside it:

```go
fmt.Print("\x1b]2;\a\x1b]111\a\x1b]110\a")
```

Prints nothing visible either way.

## Verification

`gofmt`, `go build`, `go vet`, `go test ./...` all clean; the new tests were run
verbosely to confirm they execute rather than silently skip.

The rendering was checked by **dumping `viewList()` with ANSI intact** through a
throwaway test — filter-focused, Send-focused, empty backlog (greyed chips), and
a 44-column pane (hints dropped) — then reading the escape sequences to confirm
the colours resolved to the green ramp. Deleted afterwards. Same technique that
caught the wrong-style status message last session; it keeps paying.

The one thing captured output cannot judge is the background/foreground
override, since that is the terminal's own state. Flagged for a look in a real
pane.

## Standing notes

- The wire-contract copies in `internal/{app,ctlproto,integration}` are
  **unchanged** this session — nothing here touches the control protocol.
- **Version still 0.3.0.** Both changes are additive (new `tab` binding, new
  styles); the 0.4.0 bump noted last session is still pending.
- `styles.go` is now the third copy of cats' colour table, after
  `internal/config/config.go` and `cmd/catway/web/index.html`. All three say to
  keep the others in sync. A shared source would be better and is not worth it
  across repo boundaries yet.
- The bar's contents are a `[]listAction` built per render by `m.listActions()`
  — Send's hint follows `modEnter()`, which is only correct once the terminal
  answers the keyboard-enhancement handshake. Adding a button is one entry plus
  an `actionX` const plus a `runAction` case.
