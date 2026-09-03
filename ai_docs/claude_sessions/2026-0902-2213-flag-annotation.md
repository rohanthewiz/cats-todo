# Session: the flag annotation, and the note it carries — v0.25.0

Session ID: `15ea5e86-a095-45be-928d-097ea3f7e9b7`
Date: 2026-09-02

The ask, in full:

> "I would like to add one more annotation type - 'Flag' (with note). Add it to
> the prompt editor right after priority and in the prompt list context menu"

Touched: `store.go`, `annotations.go`, `styles.go`, `annotbar.go`, `ui.go`,
`listmenu.go`, `listhover.go`, `cli.go`, `complete.go`, `README.md`,
`flagnote_test.go` (new), `annotations_test.go`, `listmenu_test.go`.

## The one design fork worth asking about

Everything except the note was already decided by the codebase. `annotations.go`
says in as many words what adding a mark costs — "a field on Todo, a field here,
a line in each of the three methods below, and an entry in `annotSlots`" — and
the annotation bar and the list menu each have an obvious next segment/row.

The note is the part the existing shapes had no answer for, because it is the
first annotation that carries *words*. The bar is a segmented line that must
shrink rather than wrap (it sits on `formAnnotRow`, a hit-tested row), so a text
field cannot live on it. Three options were put to the user:

1. a note field on the form, the menu row a plain toggle;
2. a note prompt everywhere — its own stage, opened from both the bar and the
   menu;
3. flag only, no note.

They chose (1). It is also the one that keeps the menu honest: a menu row is a
press, and a press is not a place to type.

## The flag is the *open* question

The framing that ended up driving every other decision. Priority asks how much a
prompt matters; the fruit asks how cheap it is. Both are closed questions with an
answer the program can read and rank. The flag says only *there is something
about this one* — and what that something is cannot be an enum, which is exactly
why it is the only mark with a note.

That gave the colour: `colInfo`, the palette's one cool blue, deliberately off
the warm ramp the other two sit on. That ramp runs from "ordinary outstanding
work" (`colTodo`) up to "alarm" (`colErr`), and the flag is not a third loudness
on it — it is a different kind of claim.

And it gave the closed-row behaviour, which lands between the two marks that came
before it. Priority *recedes* to the row's greys; the fruit *goes away*, because
an emoji ignores a foreground and there is no grey apple to swap in. `⚑` is a
text glyph, so a grey actually reaches it — and it should recede rather than
vanish, because "there was something about this one" is worth as much on
finished work as on open work once it has stopped competing for attention.

```go
func flagMark(t Todo) (string, lipgloss.Style, lipgloss.Style) {
	if !t.Flag { return "", lipgloss.NewStyle(), lipgloss.NewStyle() }
	if t.closed() { return flagGlyph, prioClosedStyle, prioClosedSelStyle }
	return flagGlyph, flagStyle, flagStyle
}
```

## The note lives and dies with the mark

`Flag bool` + `FlagNote string`, both `omitempty`. `annots.applyTo` is where the
invariant is enforced, once, on the only road to the file:

```go
t.Flag = a.Flag
t.FlagNote = ""
if a.Flag { t.FlagNote = strings.TrimSpace(a.FlagNote) }
```

So no backlog ever holds a note about a prompt whose row draws nothing, and the
list menu's flag row can flip `a.Flag` alone without thinking about the words.
The form does the same thing early (`setFormFlag`), so the screen and the file
agree *before* the save rather than after it.

## The row that must not move

This is the part that would have been the regression. The form's geometry is a
set of constants — `formTitleRow`, `formAnnotRow`, `formPromptRow`, and
`formBarRow()` computed from the editor's height — and every click on the form is
hit-tested against them rather than the view being re-measured. A note field
inserted under the bar would push the editor, the toolbar and every one of those
hit-tests down a row *when a checkbox was ticked*.

The layout already had the line to spend. Row 6 was the blank between the bar and
the **Prompt** label:

```
5   ☐ 🍏 Quick win   Priority  (•) none  ( ) △ high  ( ) ▲ critical   ☑ ⚑ Flag
6   ⚑ note  blocked until the api rename lands      ← was the blank
7   Prompt
8   the editor…
```

`formFlagNoteRow = 6`, and `flagNoteLine()` returns `""` when the flag is down —
which is the blank that row has always been. Nothing below it knows the feature
exists. `TestFlagNoteRowKeepsTheFormsGeometry` renders the form both ways and
asserts the line count and every row under it are identical, plus that a
400-character note still draws one line inside the pane.

## A conditional tab stop

`formFieldFlagNote` is the form's first focus stop that is not always there. The
walk steps over it while the flag is down — a tab that appeared to do nothing,
twice on the way round, is worse than one stop fewer:

```go
next := (m.formFocus + delta + formFieldCount) % formFieldCount
for range formFieldCount {
	if next != formFieldFlagNote || m.formAnnots.Flag { break }
	next = (next + delta + formFieldCount) % formFieldCount
}
```

Pressing `⚑ Flag` from the keyboard raises the field *and takes the keys*, which
is new behaviour for the bar — every other segment leaves the focus where it was.
The justification is that "flag this, because…" is one thought, and a field that
appeared but had to be tabbed to would break it in half. A *click* on the segment
still does not steal the keys, keeping the bar's existing chip-like contract; the
only cmd that path can return is the one that fires when clearing the flag has
stranded the caret in the field it just took away.

`activateAnnotSeg` grew a `tea.Cmd` return for that, and `pressAnnotSeg` is the
keyboard's wrapper around it.

The value is committed to `m.formAnnots.FlagNote` in `forwardForm` on every
keystroke rather than on the way out of the field, so `ctrl+s` pressed *in* the
note field writes what is on screen.

## The compact bar tier ran out of room

`TestAnnotBarFitsNarrowPanes` caught this immediately: five segments at gap 2
came to 33 cells in a 30-cell pane. The fix was in the tier that had already
given up its words — `( ) none` became `( ) –`, saving exactly the three cells:

```
☐ 🍏  ( ) –  (•) △  ( ) ▲  ☑ ⚑        30 cells
```

A dash rather than a bare `( )`, because none is the one radio with no mark of
its own and would otherwise have been the only segment on the bar saying nothing
at all. The dash *is* the mark for "nothing said".

## Where the note is read back

Three screens, and each earns it differently:

- the **hover card** (`listhover.go`) — the one place the note is visible without
  leaving the list, which is what the card is for;
- the **prompt view**'s meta line — free, via the existing `annotSlots` walk that
  keys off `label` rather than `mark`, so a closed flagged row still spells out
  `⚑ flagged: blocked on the api`;
- the **list menu**'s flag row, truncated to 32 cells — so the decision to press
  `✎ Edit…` and rewrite the note is made with the current note in sight rather
  than a screen away.

## CLI

`--flag` is an `optString`, the shape `--sess-load` already uses: bare raises a
flag with nothing to say, `--flag="blocked on the api"` raises one with the
words. One flag rather than a `--flag`/`--flag-note` pair, because the two would
only ever be typed together and a note without the mark has nowhere to go.

It gets no `expandSessLoad`-style rewrite, so the value must be attached with
`=` — the words after a bare `--flag` are the prompt, which is the whole shape of
this command:

```
$ cats-todo add --flag="blocked on the api rename" --priority high "port the export picker"
added to the project backlog, marked high · flagged: blocked on the api rename (…)

$ cats-todo add --flag "just look at it"
added to the project backlog, marked flagged (…)      ← the words were the prompt
```

## Tests

`flagnote_test.go` (new, six cases): the segment raises and focuses the field;
clearing drops the note and hands the keys back; the geometry is unmoved; tab
skips the stop while the flag is down and joins it in both directions when it is
up; the whole save → reload → reopen round trip; and a click on the note row
takes the keys while a click on the blank row does not.

Plus flag cases in `listmenu_test.go` (flips both ways, draws state and note,
clearing drops the note, does not clobber the marks beside it), and the two
existing annotation tests that enumerate every slot — the distinct-glyph test in
particular, which fails loudly if a new slot draws nothing for a fully annotated
todo.

`go test ./...` green.

## Left for the release

The version was already at an unreleased `0.25.0` in both places from the two
feature commits since `v0.24.1`, so no bump was needed — only the tag, which is
what this session's wrap-up cuts.
