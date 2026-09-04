# Session: the form's toolbar moves to the top, and the chords move with it

Session ID: `e6b6d248-400d-46ed-a0eb-347ad9317f65`
Date: 2026-09-04

The ask, in full:

> "In the prompt editor, change the Save button to say "Save shift+enter".
> cmd+s should remain working
> - Also remove the "Newline enter" button
> - remove the spell button - we will rely on right click on the word
> - Session ctrl+s
> - Send shift-option-enter
> - Cancel Esc
> - The toolbar order should be: Images, Session, Save, Send, Cancel
> - Finally remove the "Edit prompt" title and replace that line with the
>   rearranged toolbar (moved from the bottom)"

Touched: `ui.go`, `spell.go`, `README.md`, and seven test files
(`formmouse_test.go`, `keys_e2e_test.go`, `model_test.go`,
`promptcarets_test.go`, `promptmenu_test.go`, `session_test.go`,
`spell_test.go`).

The whole change is one screen's chrome, but it moved a chord that half the
codebase's comments referred to (`ctrl+s`), so most of the work was in the
places that *named* the old arrangement rather than in the arrangement itself.

## The row, before and after

```
before   ✔ Save ctrl+s   ↵ Newline enter   ☑ Spell   ❐ Images ctrl+o   ⚙ Session ctrl+r   ✉ Send   ✖ Cancel esc
         …under the editor, below the 📎 and ⚙ notes, above the error line

after    ❐ Images ctrl+o   ⚙ Session ctrl+s   ✔ Save shift+enter   ✉ Send shift+opt+enter   ✖ Cancel esc   Project backlog
         …on row 0, where the heading was
```

Seven buttons became five. `formActions` reordered, the two dropped chips took
their `formAction*` indexes with them, and `clickFormBar` lost the two cases.

## Why row 0 cost nothing

The form's rows are constants (`formTitleLabelRow`=2 … `formPromptRow`=8) and
every click is hit-tested against them. The toolbar **took the heading's line**
rather than being inserted above it, so not one of those constants moved — the
same trick the flag's note played on the blank line at `formFlagNoteRow`.

`formBarRow()` went from `formPromptRow + m.promptArea.Height() + 2` to a flat
`0`, and that is the real win of the move: the bar used to be positioned by the
editor's height, which meant a wrapped `⚙` line or a taller editor could slide
the buttons out from under the pointer. On row 0 nothing below it can push it.
It stays a method because `clickForm` and six tests ask the model for it.

`formChromeHeight` dropped 16 → 15: the toolbar left the rows *below* the editor
and joined the eight already counted above it.

## The scope tag

The heading was the only thing on the screen that said which backlog an add
would land in, and `ctrl+g` toggles exactly that. Removing it would have left a
toggle with no visible answer — the thing contract 4 in the skill ("refuse in
words", never silently) exists to prevent.

So the scope rides at the end of the toolbar row, dimmed, in add mode only (an
edit's scope is not a choice, and the heading never named it there either), and
**only when it fits**:

```go
if m.formMode == formAdd {
	tag := "  " + m.formScope.String() + " backlog"
	if m.width <= 0 || lipgloss.Width(bar)+lipgloss.Width(tag) <= m.width {
		b.WriteString(descStyle.Render(tag))
	}
}
```

The width guard is load-bearing, not tidy: this is row 0 of a layout every click
below is counted from, so a wrap here would move every hit-test on the form one
row down from what the eye sees. It is past the last chip's span, so a click on
it is a miss, which is what it should be.

## The chord shuffle

| | before | after |
|---|---|---|
| Save | `ctrl+s` · `cmd+s` · `shift+enter` | `shift+enter` · `cmd+s` |
| Session | `ctrl+r` | `ctrl+s` (`ctrl+r` kept) |
| Send | click-only | `shift+opt+enter` (`alt+shift+enter`) |

Three things worth writing down:

**`ctrl+r` stayed bound.** Nothing costs less than leaving an old spelling
working, and a chord an older README taught should not become a chord that does
nothing.

**Send's chord is the save chord plus Option**, deliberately. Sending *is*
saving plus one more thing (`sendForm` persists, then opens the picker), so it
reads as the save key with something added rather than as an unrelated chord —
and a second modifier is not a key you hit one slip from the caret, which was
the whole argument for Send being click-only before.

**Both spellings of it are bound.** `Key.Keystroke()` prints modifiers in a
fixed order (ctrl, alt, shift, meta, hyper, super), so today it is
`alt+shift+enter` — but a binding that depends on a library's print order is a
binding that breaks on an upgrade nobody would connect to it.

That also made Save and Send neighbours on the row, which the old order
deliberately avoided. It is affordable now because they are held differently and
because a mis-clicked Send lands on the picker with the prompt already saved —
one `esc` from where the hand meant to be.

### What a non-kitty terminal loses

`shift+enter` needs the kitty keyboard protocol to be distinguishable from a
bare enter, and `cmd+s` needs the terminal to forward Cmd. Under cats both
arrive. Elsewhere neither does, and since `ctrl+s` is now the ⚙ panel, keyboard
saving there falls back to `enter` from the Title field or the ✔ Save button.
Flagged to the user rather than papered over with a hidden third binding; it is
in the README's chord section too.

## The footer had to be rethought, not just re-spelled

The chords line only appears when the chips stop printing their hints (under ~98
columns now, against 108 before). Two of the new segments are long
(`shift+opt+enter send` is 20 cells), and `fitFooter` drops from the right — so
segment order is a priority list, and two tests pin what must survive at 95
columns.

`ctrl+l spelling` now goes **ahead** of `esc cancel`: no chip stands for the
spelling panel at all since ☑ Spell left the bar, so it is the one thing on the
line that nothing else can teach, while `esc` is the most guessable key on the
screen and the ✖ Cancel chip is spelling out the word beside it.

At `tierIcons` the line stops being a list of chords and becomes **the legend for
the row above it** — each glyph beside the key that presses it:

```
w=95   ctrl+o images · ctrl+s session · shift+enter save · shift+opt+enter send · ctrl+l spelling
w=40   ✉ shift+opt+enter · ✔ shift+enter
w=30   ✉ shift+opt+enter
```

`✉` leads because it is the one act on this screen that leaves the program and
the one chord nothing else hints at. This replaced the old `✉ click sends`
special case, which existed only because Send had no chord to name.

## Tests

The two **end-to-end wire tests** were the interesting ones. They drive the whole
program with raw terminal bytes and used `\x13` (ctrl+s) to save; that now opens
the ⚙ panel. They feed `\x1b[13;2u` instead — the kitty protocol's CSI-u form of
shift+enter — which is a better test than it was: it proves the save chord that
only exists under the protocol actually parses off the wire.

Also: `TestFormSendIsClickOnly` → `TestFormSendChord` (each "must not send" key
from a model of its own, since shift+enter now writes to the backlog);
`model_test`'s save table swapped ctrl+s for cmd+s; `promptcarets` proves the
column mode hands back `shift+enter`; `promptmenu`'s overlay test looks for the
toolbar instead of the heading; `spell_test`'s toggle test drives the panel's
last row now that the chip is gone (taking `spellChipLabel`/`spellChipTint` with
it — they were the chip's own rendering and nothing else).

New: `TestFormBarShape` pins the five buttons, their order, their chords, that
the bar is on row 0, and that the scope tag shows on an add and not on an edit.

`go test ./...` green.

## Docs

README updated in seven places: the intro (the row is at the top now, and Send
has a chord), the narrowing-chips paragraph (`❐ ⚙ ✔ ✉ ✖`), the annotation-bar
geometry note, the caret mode, the prompt library (`ctrl+s` no longer means two
things one screen apart), the chords section (including a sentence that was
already stale — it claimed `shift+enter` inserts a newline in the editor), the
spell section, and the session-options section.

No version bump: `0.27.0` is already unreleased in both places, ahead of the
`v0.25.0` tag.
