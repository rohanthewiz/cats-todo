# Session: Selection in the editor

Session ID: `5a5f532a-27d3-430c-85d7-8fc589773ea8`
Date: 2026-08-13

The ask, in one line:

> "Selection within the editor is not working"

Then, offered three ways to fix it — a key that drops mouse reporting while the
form is up, reverting the form's mouse reporting entirely, or building real
selection into the textarea — the answer was the third.

New files: `promptsel.go`, `promptsel_test.go`. Touched: `ui.go`, `styles.go`,
`clipboard.go`, `go.mod`.

## What was actually broken

`ui.go`'s `View` asks for mouse reporting on three stages:

```go
if m.stage == stageList || m.stage == stageTarget || m.stage == stageForm {
    v.MouseMode = tea.MouseModeCellMotion
}
```

`stageForm` joined that set in `e0327ce` ("point-and-click editing in the prompt
form, plus a toolbar"). While mouse reporting is on, a drag belongs to this
program rather than to the terminal, so the pane's own click-to-select stops
working — which the `View` comment already said would happen, it just never said
the editor was one of the screens paying it.

So the editor had no selection at all: not the terminal's, because reporting took
it, and not its own, because `charm.land/bubbles/v2`'s textarea has none. There
is no anchor, no mark, and nothing in its `Styles` for a highlight.

Worth recording for the next person who reads that comment: `bubbletea` v2 offers
only `None` / `CellMotion` / `AllMotion`. There is no click-without-motion mode,
so *any* clickability on a stage costs that stage its native selection. The form
could not have had both.

## The design

A selection is **one anchor**, a rune offset into the value. The caret is the
textarea's own state and is read back out of it whenever a span is needed, so
there is no second copy of the cursor to fall out of step:

```
value:   t h e   q u i c k   b r o w n   f o x
                 ▲                   ▲
              anchor               caret
span:            └─── [lo, hi) ─────┘
```

**Nothing in `promptsel.go` ever sets the caret.** `shift+←` hands the textarea a
plain `←` with the shift bit stripped and lets it move its own caret; a drag
hands the pre-existing `placePromptCursor` the pointer's cell. That is the whole
trick, and it is what makes the feature correct across soft wraps, double-width
glyphs and scrolling without re-deriving any of it.

It also means every motion the library already binds extends a selection for
free, because the match is on `Key.Code` plus `ModShift` rather than on a
chord's printed name:

| chord | what it selects | who implements it |
|---|---|---|
| `shift+←/→` | a character | textarea `CharacterForward/Backward` |
| `shift+alt+←/→` | a word | textarea `WordForward/Backward` |
| `shift+↑/↓` | a line, break included | textarea `LinePrevious/Next` |
| `shift+home/end` | to the line's ends | textarea `LineStart/LineEnd` |
| `shift+ctrl+home/end` | to the value's ends | textarea `InputBegin/InputEnd` |

`promptSelectionKey` strips `ModShift` (and clears `ShiftedCode`, so nothing
downstream can conclude shift is still held) and returns the plain motion.

## The wrap, which is the hard part

A highlight is painted on *screen cells*. The map from "rune offset in the value"
to "cell on line N of the box" is exactly the soft-wrap — and the library keeps
its wrap private. `LineInfo` answers the question only for the line the caret
happens to be on, and the one public way to ask about another line is to walk the
caret there, which moves the caret *and* scrolls the viewport. A `View` may do
neither. (The viewport is shared by pointer, so even a copy of the model scrolls
the real editor — this is the same hazard `promptLines` documents.)

So `wrapPromptRunes` is a faithful transcription of `textarea.wrap`. Two
properties of the original carry everything else:

- **It preserves runes.** A space is counted and re-emitted as a space, a
  non-space appended verbatim — so the concatenation of the wrapped lines is the
  input plus exactly one trailing space. That is what lets a display line be
  described as a `(start, length)` window on the value.
- **Only the last line carries that added space.**

`promptDisplayLines` turns that into display-line index → `{row, start, length}`
for the whole value, and the line drawn at screen row *y* is entry
`ScrollYOffset() + y`.

### Pinning the copy to the original

`TestPromptDisplayLinesMatchTheTextarea` uses the library as an oracle rather
than a table of expected wraps. It walks the caret to every column of every row
across twelve corpus values (wrapped lines, runs of spaces, trailing spaces, a
word longer than the pane, double-width glyphs, empty rows, only newlines) and
checks `LineInfo`'s `StartColumn` and `RowOffset` against the table at every
seam — plus that the table tiles the value with no gap and no overlap.

Confirmed it has teeth: flipping the wrap's `> width` to `>= width` fails three
subtests with concrete column mismatches.

## Drawing the highlight

An **overlay**, not a re-render. Only lines the selection touches are rebuilt,
and each of those keeps the editor's own output — escapes and all — for every
cell left of the highlight, so the gutter and the cursor-line field survive
without this file knowing they exist.

```
│ the quick brown fox jumps
│     ├──── highlight ───┤▌
0     a                  b caret
```

From `a` rightwards the line is redrawn from its plain text. `ansi.TruncateLeft`
drops the escapes that opened *before* its cut point, so a tail spliced back in
raw loses the cursor line's background partway across the row; it is rendered
with the style the library drew it in instead, taken from the public
`ta.Styles()`. That is exact for as long as those are the only styles in play,
which the form guarantees — `newFormInputs` never sets its own.

Two details that are easy to get wrong and are now pinned by tests:

- **A selection that runs off the end of a line paints to the right edge.** The
  line break is inside the span, and a highlight that stopped at the last
  character reads as several separate selections stacked up rather than one
  block. An empty row mid-selection gets a full-width bar for the same reason.
- **The caret is redrawn** when a forward selection would have stripped its
  block out of the tail. If the cut comes back empty — the caret is past the last
  cell the editor drew, where the library drew none either — the line is left a
  cell short rather than having one invented. The overlay must never change how
  wide a line is, or the pane's own wrapping takes over and the form's rows stop
  being where every click is aimed.

`TestPromptSelectionIsDrawn` holds that invariant directly: same line count, same
stripped text, same `lipgloss.Width` per line, with and without the overlay.

## ctrl+c

Copies while something is selected; still quits when nothing is.

Overloading the quit key is the one liberty taken here and it is the right one:
`ctrl+c` is what every other program on the machine copies with, and a selection
that needed some other chord would be a selection nobody uses. The guard is the
selection itself — the only way to reach the copy is to have deliberately swept
or shift-walked over some text — and the way back to quitting is the same `esc`
that already backs out of the form.

The selection survives the copy. Copying is not a gesture that ends anything.

`copyTextToClipboard` fires **both** OSC 52 (`tea.SetClipboard`) and, on darwin,
`pbcopy`. Neither covers the ground alone: OSC 52 is the only one that works
across ssh or a multiplexer, but several terminals have it off by default;
`pbcopy` is unconditional but only local. Same bytes, so the race is harmless.

A new `formNote` field reports the copy on the same line `formErr` uses, with the
error winning — the split `imgStatus`/`imgStatusErr` already draws, for the same
reason: a save failure and a successful copy must not look alike.

## The mouse

A press anchors and arms a possible sweep; **motion** with the button down is
what turns the pair into a highlight. A bare click leaves no selection behind,
which is what keeps click-to-place-caret usable.

`promptSelDrag` is deliberately a separate flag from the list's `dragging`: the
two gestures live on different stages and mean different things — one reorders a
backlog, the other sweeps a highlight — and one flag serving both would let a
release meant for one end the other.

A sweep that runs off the bottom of the editor clamps into the box rather than
being ignored. "Everything down to here" is what the hand means, and dropping
those motions would freeze the highlight while the button is plainly still
moving.

## The selection's lifetime

It lasts exactly as long as the run of keys building it. The clear is on the way
*into* `updateForm` rather than spread across the handlers below, because the
rule is about the gesture and not about any one key: the first key that isn't
part of the run ends it. A press anywhere on the form ends it too — the pointer
is about to say where the caret goes next, and a highlight that outlived the
click moving away from it would misreport what the next `ctrl+c` copies.
`beginAdd`, `beginEditRef` and `backToList` all clear it as well.

## Colors

`colTextSel = #4a6656`, built the way `colSel` was: same green family, about
**2.5:1** against the page where the row highlight is 1.9:1.

The extra step is not decoration. `colSel` marks a whole row edge to edge, and a
wide band is legible at a contrast that three characters mid-paragraph are not; a
selection has to be findable when it is short. It also has to separate from the
editor's own cursor-line field, which the row highlight never competes with.
`colFg` on it still clears 4.5:1 — which matters more here than on a list row,
because the reason to select something is to read it before copying it.

`promptCaretStyle` is `Reverse(true)` rather than a color pair, so the redrawn
caret matches the block the textarea draws for itself whatever the terminal
resolves that to.

## The footer

Now: `click places the caret · shift+←/→ selects · ctrl+c copies · ctrl+a/e line
start/end · alt+←/→ word · tab switch field`.

Drag-select is **not** on it, and that is the reason rather than an omission. The
first draft ran 126 cells at a 120-cell pane and `fitFooter` — which trims from
the right — dropped `tab switch field`, failing `TestFormFooterTeachesCaretKeys`.
A line that already says the pointer places the caret has said the pointer works
here, and sweeping one is what a hand tries next without being asked; the shift
chord is the half nobody guesses, so it is the half that gets the ink. Same
rationale the footer already gives for leaving `↑/↓` and `←/→` off entirely.

## Tests

`promptsel_test.go`, nine tests:

- `TestPromptDisplayLinesMatchTheTextarea` — the oracle above.
- `TestPromptShiftArrowsSelect` — grows, shrinks, and inverts through the anchor
  (which is why `lo`/`hi` are sorted rather than assumed).
- `TestPromptSelectionRidesEveryMotion` — `shift+end`, `shift+alt+→`, `shift+↓`,
  none of which this file implements.
- `TestPromptShiftArrowsSelectAcrossLines` — a span crossing a newline is one
  range, and the newline is in what gets copied.
- `TestPromptDragSelects` — press selects nothing, sweep selects, release keeps.
- `TestPromptDragBelowTheEditorClamps`.
- `TestPromptSelectionCopy` — copies with a selection, quits without one, note
  reports the count, selection survives.
- `TestPromptSelectionEndsOnTheNextKey` / `…OnAClick`.
- `TestPromptSelectionIsDrawn` (five cases incl. a caret past the last drawn
  cell), `…IsDrawnWhenScrolled`, `…SurvivesAResize`,
  `TestPromptSelectionOnlyInTheEditor`.

Confirmed the scroll test has teeth too: replacing `top + y` with `y` in
`promptEditorView` fails it at screen row 11.

`gofmt`, `go vet`, `go test ./...` all green. `go mod tidy` promoted
`x/ansi`, `go-runewidth` and `uniseg` from indirect to direct — all three were
already in the build graph.

## Verified by eye, then deleted

A throwaway `zz_visual_test.go` printed the drawn editor with escapes made
visible, over a value with a short line, an empty row, a wrapped paragraph and a
tail. Worth recording, since no unit test asserts on raw escapes: the fully
selected lines carry `48;2;74;102;86` (the new field) out through their padding
to the right edge; the empty row gets its full-width bar; the partially selected
line shows the field for four cells, then `^[[7m` on the caret's character, then
the cursor line's own `^[[40m` background resumed across the rest — which is the
tail-restyling working. The `tail` line below the selection is untouched.

## Deliberately out of scope

Both worth knowing, neither was asked for:

- **Typing over a selection clears it rather than replacing it**, and backspace
  deletes one character, not the span. Modern-editor replace semantics are a real
  change to the edit path.
- **The title field has no selection.** `shift+←` there falls through to the
  field untouched and `ctrl+c` still quits — pinned by
  `TestPromptSelectionOnlyInTheEditor`.

## Not verified

The live path in a real terminal: whether OSC 52 is honoured by the terminal the
manager happens to be running in, and whether `shift+alt+←/→` survives a terminal
without the kitty protocol. Everything up to the escape sequence being emitted is
under test.
