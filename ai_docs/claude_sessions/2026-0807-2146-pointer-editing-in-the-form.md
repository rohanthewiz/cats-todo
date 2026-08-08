# Session: point-and-click editing in the prompt form

Session ID: `193884e6-00de-4f16-81c1-92f7333b3458`
Date: 2026-08-07

The ask, in three parts:

> "In my editor UI. First thing is: why can't I have point and click editing in
> the prompt body? If that is possible then give me a clear toolbar of quick
> actions as those mentioned below. If that is not possible or feasible, at least
> show in dimmed text the standard navigation keys like Newline - ALT or Shift +
> Enter, Start of line - CTRL + A, End of line - CTRL + E, Word nav - ALT + arrow
> keys, etc
>
> Btw, a one-off UI change: the selected row needs more contrast so the
> highlighted background of the selected row is barely visible"

No plan mode. Read the form half of `ui.go`, then the vendored
`charm.land/bubbles/v2/textarea` and `textinput` sources — which is where the
whole session was decided.

## Why it didn't work, and what the library does not give you

Two independent reasons, and the second is the interesting one.

1. `View()` asked for `MouseMode` on `stageList` and `stageTarget` only, so the
   terminal never sent the form a click at all.
2. bubbles' textarea has **no notion of a click**. Its `Update` handles
   `KeyPressMsg` and `PasteMsg` and nothing else, and the three things a
   click→caret mapping needs are all private:
   - `cursorLineNumber()` — which display line the caret is on,
   - `setCursorLineRelative()` — the jump,
   - `wrap()` / `memoizedWrap()` — the word-wrap both are computed from.

`Cursor()` looked like a way in (it returns a `tea.Cursor` whose Y *is*
`cursorLineNumber() - YOffset`) but it returns `nil` while `useVirtualCursor` is
true, which is the default and what the form uses.

Reimplementing `wrap()` was rejected: it is fiddly around trailing spaces and
double-width runes, and a private function copied out of a dependency drifts
silently at the next `go get`.

## The mapping that survived

What *is* public is one-display-line-at-a-time motion (`CursorUp`/`CursorDown`),
`LineInfo()` for the soft line under the caret, and `Line()`/`SetCursorColumn()`.
So the wrap is measured rather than recomputed:

- `promptLines()` walks a **copy** of the model from `MoveToBegin()` with
  `CursorDown()`, recording `{row, LineInfo().StartColumn}` per step until
  stepping stops moving. That pair identifies a display line uniquely — including
  in `LineInfo`'s wrap-around case, where a caret at the end of a soft line
  reports the *next* line's offset and a `StartColumn` equal to that line's start.
- `stepPromptTo()` walks the real caret to a target line, re-reading the table
  after every step instead of trusting a step count. An off-by-one in the
  library's wrap arithmetic costs a wasted step, not a caret on the wrong line.
- the column comes from `StartColumn` plus a rune-width walk (`colAtWidth`), so
  double-width glyphs don't skew it, and a click off the end of a *wrapped* line
  stops one short of the space the wrap ate — "after the last word", not "the
  first column of the next line".

### The scroll problem, which is the whole trick

`Model.viewport` is a **pointer**, shared by every copy of the model. So the
table walk scrolls the real editor to the bottom of the value. Left alone, every
click in a long prompt would jump the view out from under the pointer that aimed
at it.

There is no `SetYOffset`. The lever back is the same `repositionView` that caused
the problem: a caret arriving from **below** its target scrolls the view up to
sit exactly on the caret's line. So `placePromptCursor` captures `y0 =
ScrollYOffset()` *before* building the table, then steps to `last`, up to `y0`
(YOffset is now provably `y0` again), and finally down to the clicked line —
which is inside the viewport, so nothing scrolls a third time.

```
       ┌─ the value, in display lines ──┐
   0   │ ...                            │  above the viewport
  y0 ──▶ first visible line             │  ScrollYOffset  ← restored here
       │ ...                            │
   d ──▶ the clicked line               │  y0 + clicked row
       │ ...                            │
last ──▶ ...                            │
```

`TestFormClickKeepsTheScroll` is the guard, and it found its own trap: the
textarea only learns its content when it *renders* (the viewport is filled in
`view()`), so a scroll asked for before the first frame goes nowhere and the test
proved nothing. One `viewForm()` call stands in for the frame a real session has
already drawn.

## The toolbar

Since clicking is possible, the ask's first branch applied. `formActions()`
reuses `listAction` — the two bars are the same object, a labelled chip standing
for a chord, tinted by consequence — and `formChips()` mirrors `actionChips()`
so one description both draws the row and hit-tests it.

`✔ Save enter` · `↵ Newline alt/shift+enter` · `❐ Images ctrl+o` · `✖ Cancel esc`

Decisions worth keeping:

- **Row placement.** The bar sits between the editor and the error line, so
  `formBarRow()` is a function of the editor height alone — a failed save must
  not move the buttons out from under the pointer.
- **No focus field on any chip.** The form's tab ring is its two text fields;
  lighting a button the keyboard cannot reach would promise a stop that isn't
  there. These are pointer targets that name their keys.
- **Dingbat icons only**, held by `TestFormBarIconsAreOneCell` — the same clipping
  problem `listActions` documents, plus a two-cell glyph would put every chip's
  hit-test span one column off what the eye sees.
- **`❐ Images` chip replaced the attachment line's "— ctrl+o to attach"**. The
  count line stays; it is status, not an action.
- **Scope stayed off the bar.** The heading already names which backlog, and
  `ctrl+g` only works in add mode with both stores available.

The dimmed line went in too, rather than being treated as the discarded branch —
caret keys act at the cursor, so no chip can stand for them:

```
click places the caret · ctrl+a/e line start/end · alt+←/→ word · tab switch field
```

`↑/↓` and `←/→` are deliberately **not** on it: a text box's arrow keys are the
one thing nobody has to be told, and dropping them bought room for the ones that
do. At 96 columns the full line was conceding `alt+←/→ word` — the exact key the
ask named — which is what forced the compression to `ctrl+a/e line start/end`.
Segments concede from the right (`fitFooter`), and when the pane is too narrow
for chip hints the footer names those chords again on a line of its own.

## The one-off: row contrast

`onRow` was reusing `colPanel` (#2b322c) with a comment arguing that a new
surface tone would be a second color meaning something different by a shade.
The comment was wrong about the cost: #2b322c against #1f2420 is ~1.2:1, which
is a shade, not a highlight. New `colSel = #3b5245`, ~1.9:1, with `colFgHi` still
clearing 10:1 on it. The constant is separate from `colPanel` on purpose —
`colPanel` means "a recessed surface", this means "the row the keys are on", and
a highlight that has to be legible cannot be pinned to a tone chosen to recede.

## Incidental cleanups

- `focusForm(field)` replaced four copies of blur-one-focus-the-other
  (`beginAdd`, `beginEditRef`, `toggleFormFocus`, and the new click paths), with
  `formFieldTitle`/`formFieldPrompt` retiring the bare 0/1.
- `cancelForm()` — esc and the ✖ chip are the same exit, including the clipboard
  captures a discarded form has to drop.
- `formChromeHeight = 13` names what the editor is budgeted against, in both
  `newFormInputs` and `applySizes`, instead of a bare `- 12` in two places.
- `toggleFormFocus` takes its `tea.Cmd` into a local before returning `m`: a
  returned value is a copy, and Go's evaluation order would otherwise hand back
  the model as it was before the focus moved.

## Running it, since there is no tmux here

`go test ./...` green, `formmouse_test.go` added (row constants against the
rendered frame, caret placement including wrapped lines and the scroll guard,
every chip, the footer's concessions, and the form's line budget at four pane
heights).

Then the app itself, because a mapping like this is only believable in a real
terminal. No tmux on this machine and no TTY in the tool shell, so:
`pty.openpty()` + `TIOCSWINSZ` at 100×30, `pyte` playing the byte stream into a
screen that can be read cell by cell, keys and **SGR mouse reports**
(`\x1b[<0;X;YM` / `m`) written to the master. The virtual caret shows up as a
reverse-video cell, so where it landed is checkable:

| input | caret cell |
|---|---|
| `enter` (open the form) | 44,7 |
| `click 23,6` | 23,6 |
| `click 32,7` (wrapped line) | 32,7 |
| `click 38,24` (❐ Images chip) | — attachment editor opened |

The frames were then rendered to HTML with their true colors and published as an
artifact, including the selected row recolored back to #2b322c beside the new one
for comparison. Driver and generators are in the session scratchpad, not the
repo.

## Files

- `ui.go` — mouse mode on `stageForm`; `clickForm`, `placeTitleCursor`,
  `colAtWidth`, `promptLine`, `promptLines`, `stepPromptTo`, `placePromptCursor`,
  `promptRowRunes`, `promptGutterWidth`; the form row constants and
  `formBarRow`; `formActions`, `formChips`, `formBarShowsHints`, `formBar`,
  `clickFormBar`, `formFooter`, `fitFooter`; `focusForm`, `cancelForm`,
  `formChromeHeight`.
- `styles.go` — `colSel`, and `onRow` off `colPanel`.
- `formmouse_test.go` — new.
