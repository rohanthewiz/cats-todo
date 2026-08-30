# Session: a user-level prompt library, inserted at the caret

Session ID: `3bfdaeba-a045-4d2c-b2e1-89385ecaada2`
Date: 2026-08-29

The ask:

> "Allow creation of a user level prompt library of common prompts that I can
> insert into the prompt body at the cursor position. Allow commands (skills) to
> be specified like (/sess-load)"

New: `promptlib.go`, `promptpick.go`, `promptlib_test.go`, `promptpick_test.go`.
Touched: `ui.go`, `promptmenu.go`, `promptmenu_test.go`, `README.md`.
Released as **v0.20.0** (both places, tagged).

## Three questions asked before writing anything

The ask had two readings that led to materially different work, so the session
opened with one `AskUserQuestion` round rather than a guess. All three
recommendations were taken, and they are worth recording because each one is a
fork the code now sits on:

1. **Authoring** — file *and* capture from the editor, rather than a read-only
   picker over a hand-edited file. This is what bought `ctrl+s` in the picker
   and the whole `add`/`writePromptLib` path.
2. **Commands** — own line *and* a `/` trigger, rather than treating a body
   starting with `/` as ordinary text.
3. **Chord** — `ctrl+p` + `cmd+P`, rather than `ctrl+t` or a menu row alone.

The menu row was added anyway, on top of the chord. The third option in that
question was "context menu **only**" — meaning *no chord* — so the row itself was
never the thing being declined, and it is the surface where the editor's
gestures are actually learned.

## Where it lives, and why the file is read on every open

`<configBaseDir>/prompts.json`, beside `settings.json` — the global config
directory, not `.cats-todo/`. The reasoning is in the file header: a phrasing
worth keeping is a habit of the person, not of the repository. Backlogs stay
per-project; the words you write them with do not.

The library is **not cached on the model**. `loadPromptLib()` runs on every open
— including on the `/` trigger, to answer "is there anything to offer" before
the screen changes. `filepick.go` already takes that position for `os.ReadDir`
per keystroke, and here the case is stronger: this is a file people edit by hand
in another window, and a library that needed a restart to notice would be the
kind of tool you stop trusting.

Two shapes parse — the wrapper object the program writes, and a bare top-level
array, because that is what a hand-written file looks like:

```go
var wrapped promptLibFile
err := json.Unmarshal(data, &wrapped)
list := wrapped.Prompts
if err != nil {
    var bare []promptSnippet
    if json.Unmarshal(data, &bare) != nil {
        return nil, "cannot read " + where + ": " + err.Error()  // the object's error, the documented shape
    }
    list = bare
}
```

A missing file is an empty library, not an error. A **malformed** one is, and it
is reported on the picker — reading a typo as an empty list would make a library
that looks lost and a library that is lost indistinguishable.

## The one interesting thing: two kinds of entry, derived not declared

A body starting with `/` is a command; everything else is a snippet. Nothing in
the file declares it. That means a hand-written entry never has to say the same
thing twice, and an entry cannot be *typed* as one thing and *stored* as
another.

It is not cosmetic. A slash command only **is** a command when it begins a line,
so the two kinds insert differently, and `snippetInsertion` is the pure function
that decides:

```
"fix the crash|"          + /sess-load  →  "fix the crash\n/sess-load\n|"
"fix the crash\n|"        + /sess-load  →  "fix the crash\n/sess-load\n|"
"fix the crash\n/|"       + /sess-load  →  "fix the crash\n/sess-load\n|"   (eatSlash)
"fix the crash\n|\nmore"  + /sess-load  →  "fix the crash\n/sess-load\n|\nmore"
```

The leading newline is added only when there is real text behind the caret on
its line (so a command dropped on a blank line does not push a blank one ahead
of it); the trailing one unless the text already continues on a new line. A
snippet gets neither — its author already decided where its newlines are, and an
entry ending in `"1. "` means to leave the cursor after that space.

`// TODO:` is deliberately **not** a command:

```go
b := strings.TrimLeft(s.Body, " \t")
return strings.HasPrefix(b, "/") && !strings.HasPrefix(b, "//")
```

A line of code forced onto a line of its own would reformat a snippet its author
had already laid out.

## The `/` trigger, and why it needed two guards where `@` needs one

The `@` file picker's rule is one guard: at the start of a word. A slash cannot
afford that generosity — `and/or`, `src/ui`, `3/4` are all just text — so
`promptAtLineStart` is strictly stricter (nothing but blanks between the caret
and the newline above; indentation still counts), **and** the library is asked
whether it holds a command at all:

```go
if m.formFocus == formFieldPrompt && msg.String() == "/" && promptAtLineStart(m.promptArea) {
    if lib := loadPromptLib(); lib.hasCommands() {
        next, cmd := m.forwardForm(msg)
        m = next.(model)
        opened, blink := m.beginSnippets(lib, snippetsCommands)
        return opened, tea.Batch(cmd, blink)
    }
}
```

Someone who keeps no commands never meant to ask, and their `~/projs/…` typed at
a line start is left completely alone. That read is also why `beginSnippets`
takes the library as an argument instead of loading its own — the question is
settled once, and two reads could give two answers.

The `@` precedent is otherwise followed exactly: the character goes into the
editor **first**, so `esc` leaves a plain `/` behind. Which raises the doubling
problem — the slash is already in the text when the command is chosen. The fix
is not to trim the body but to widen the replaced span by one rune:

```go
start, end = caret, caret
if eatSlash && start > 0 && value[start-1] == '/' {
    start--
}
```

That is also what keeps the line-start rule honest: the typed slash falls
*outside* the span, so it cannot count as "text in the way" and provoke a
leading newline. It goes through `replacePromptRunes` (spellpanel.go) rather
than the textarea's `InsertString` for exactly this — the edit can start behind
the caret.

## Where the chord had to be handled

`ctrl+p` sits **above** `updateForm`'s `clearPromptSel()`, with `ctrl+c`,
`ctrl+x` and `alt+↑/↓`. Not because it reads the selection to act on it, but
because the picker **offers to save** it: by the time the switch below is
reached the highlight would already be gone, and the offer with it.

The highlight is then ended inside `beginSnippets`, after the capture is taken —
the form's standing rule is that a selection lasts as long as the run of keys
building it, and leaving it would put the editor back under a highlight whose
anchor an insertion had since moved. The switch below keeps a case of its own,
reached only from the title field, which refuses in words (contract 4).

Cost of the chord, recorded honestly: it shadows the textarea's emacs `ctrl+p` =
line up. `↑` is the other spelling and the README never taught the first.

## Saving from the picker

No new stage. The picker's query box is already a focused text field, so the
name of the thing being saved is typed where the name of the thing being looked
for is typed:

- something swept → the selection is saved;
- nothing swept → the whole prompt is.

Decided at **open** time, not save time, which is what lets the footer name it
before you commit to it (`ctrl+s saves the selection under the typed name`).
Four distinct refusals — no capture, no name, name taken, failed write — each in
its own words, picker left up. A duplicate name is refused rather than
overwritten: overwriting is the destructive reading of an ambiguous gesture.

`ctrl+s` means "save the prompt" one stage up and "save this snippet" here. Same
idea applied to what the screen is about, and the form's own save is not
reachable from the picker, so no press gets the wrong one.

## Rows: saying a thing once

First draft rendered `wrap up · /sess-wrap  /sess-wrap` — the tag carried the
command and `summary()` fell back to the body. Caught by eye, not by a test, in
a throwaway frame dump. Now a command row's tag is the **whole** command line
(arguments included — `/sess-load 2` and `/sess-load` are different entries and
the number is the difference), and its description is only what the entry
actually says:

```go
if c := s.commandLine(); c != "" {
    it.tag = truncate(c, 48)
    it.desc = strings.TrimSpace(s.Desc)
}
if it.tag == it.name {  // an unnamed command, whose label is already its body
    it.tag = ""
}
```

The heading's path is cut with `truncateLeft` and a note with `truncate` — the
end of a path says which file, the front of a sentence says what happened.

## The menu row, and the test it moved

`≡ Insert a prompt…  ctrl+p` goes **last** on the context menu and is always
live. Both facts are one decision: an always-live row placed first would become
the default the keyboard lands on, ahead of the items the right-click was
actually aimed at.

That broke `TestPromptMenuKeyboard`, which asserted the ↑-from-the-top wrap
landed on `menuSpell` "the last row". The intent was *the last row*, so it now
says `menuActionCount-1` — a rename of the assertion to what it meant, not a
change to what it checks. `len(m.menu.items) == menuActionCount` needed nothing.

## Footer

`ctrl+p prompt library` rides at the very tail of the caret line, past
`cmd+d dup line`. The line is full at 118 cells for its seven standing segments,
which is what pins `tab switch field` at 120 and `ctrl+l spelling` at 160 — so
the tail is the only affordable place, and in practice only a very wide pane
ever reads it. Discoverability is carried by the menu row and by the `/` the
user was going to type anyway.

## Tests

`promptlib_test.go` — both file shapes, the malformed-file message, entries with
no body dropped, the kind rule, label/summary/commandLine, the eight-case
`snippetInsertion` table (the load-bearing one), add/round-trip, and every
refusal.

`promptpick_test.go` — both ways in, commands-only filtering, the slash eaten
rather than doubled, `esc` leaving the plain slash, the two places the trigger
must **not** fire, the save flow end-to-end onto disk, the four refusals, the
title-field refusal, the menu row, `snippetsRowsRow` pinned to a rendered frame,
click-to-choose, the mouse mode, body-matching in the query, and resize safety.

`snippetLibChord` has a unit test of its own: `pressKey` cannot build a Cmd
chord, so `super+p`/`meta+p` have no other coverage in the suite.

`go test ./...` green, `gofmt` and `go vet` clean.
