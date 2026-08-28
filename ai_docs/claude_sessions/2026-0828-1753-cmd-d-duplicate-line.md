# Session: cmd+d duplicates a line in the prompt editor

Session ID: `bbf46f8e-2a10-4095-b12a-e1989c3d1f0c`
Date: 2026-08-28

The ask, in one message:

> "In the prompt editor can we duplicate a line with CMD+D?"

Touched: `ui.go`, `promptsel_test.go`, `README.md`, `main.go`,
`cats-plugin.toml`.

Released as **v0.18.0** (a feature — minor bump, both files).

## The chord, and why it has no ctrl fallback

The house pattern for a mac mnemonic is a pair: the chord that always works,
plus the one that only works where the terminal will report it (`ctrl+s` /
`cmd+s`, `ctrl+o` / `ctrl+i`). Duplicate-line breaks the pattern deliberately —
it ships as **Cmd only**.

The obvious fallback is `ctrl+d`, and `ctrl+d` is already spoken for by the
textarea itself:

```go
// charm.land/bubbles/v2@v2.1.1/textarea/textarea.go, DefaultKeyMap()
DeleteCharacterForward: key.NewBinding(key.WithKeys("delete", "ctrl+d"), …)
```

A duplicate bound over a delete is the one collision a text editor must not
ship — every press meant to remove a character would instead add a line. The
survey the `ctrl+r` comment in `updateForm` already makes holds here: the
editor owns `ctrl+a/b/d/e/f/h/k/m/n/p/t/u/v/w` and most `alt+<letter>`, the form
owns `ctrl+s/o/i/r/l/g/c`, so there is no free single-letter ctrl chord that
reads as "duplicate" anyway. Rather than spend an arbitrary survivor (`ctrl+y`,
`ctrl+z`) on a mnemonic nobody would guess, the feature accepts being
unavailable where Cmd cannot arrive. This is a cats plugin; cats forwards Cmd
chords, which is the terminal that matters.

Both spellings are bound, for the reason the `cmd+s` handler already gives:

```go
case "super+d", "meta+d":
```

A kitty-protocol terminal sets the super bit for Command; a terminal that
reports the same press on the meta bit sends the other. Which one arrives is
the terminal's choice, not the user's.

`alt+shift+↓` (VS Code's copy-line-down) was considered as a universal fallback
and dropped: `promptSelectionKey` claims every `shift`+motion before the switch
sees it, so the chord would have to be carved out of the selection layer — a
special case in the one file that works by having no special cases.

## What the duplicate actually does

`duplicatePromptLine` (ui.go, beside `copyPromptSelection`):

```
value:  alpha \n b e t a \n gamma          caret: row 1, col 2
                    ▲
              end of row 1 ─────┐
after:  alpha \n beta \n b e t a \n gamma
                             ▲
                       caret: row 2, col 2   (= end + 1 + col)
```

Two decisions carry the feel of it:

- **The caret lands on the copy, in the column it held.** Staying on the
  original would make a held `cmd+d` re-copy the same line forever; landing at
  column 0 would throw away where the hand was. Landing on the copy at the same
  column is what makes repeated presses stack — the behaviour every code editor
  has trained for — and what lets the next keystroke carry on editing.
- **A "line" is a logical row, not a drawn one.** A paragraph long enough to
  soft-wrap over three display lines is one line to duplicate. Copying a wrap
  segment would cut the sentence at a boundary the value does not contain, and
  the user looking at the wrapped paragraph does not think of it as three
  things.

The splice goes through `replacePromptRunes` + `setPromptCaretOffset` — the two
helpers spellpanel.go already built — rather than the library's `InsertString`.
That is exactly what those helpers exist for: the textarea only edits *at* its
caret, so inserting at a row-end means walking the caret there and back, and a
walk that lands a character off goes wrong quietly on precisely the
soft-wrapped rows this feature has to get right.

Off the prompt field the chord **says why** rather than no-op'ing
(contract 4): the title is one line by construction and the annotation bar is
not text at all, so the press sets `formNote` to
`"duplicate line works in the prompt"`.

## Footer

`cmd+d dup line` joins the second footer line, at the very end — after
`ctrl+l spelling`. The line is dropped from its tail as the pane narrows, and
the segment naming a chord that some terminals cannot send at all is the right
one to lose first.

## Tests

Five, appended to `promptsel_test.go` under a `Cmd+D` heading:

| Test | Pins |
|---|---|
| `TestDuplicatePromptLine` | the copy lands below, the caret lands on it at the same column — both `super+d` and `meta+d` |
| `TestDuplicatePromptLineStacks` | three presses give four lines, not two |
| `TestDuplicatePromptLineDuplicatesLogicalRows` | a soft-wrapped row (width 30) duplicates whole; the fixture asserts it really wrapped |
| `TestDuplicatePromptLineOffTheEditorSaysWhy` | the title press edits nothing and names the prompt |
| `TestCtrlDStillDeletesForward` | the guard for the whole no-fallback decision |

`go test ./...` green.

## Docs

README's editor section gains a paragraph after the selection prose and before
the `alt+enter` one — placed so it sits just above the `cmd+s` paragraph it
points at for the "which terminals forward Cmd" story.
