# Session: cmd+c copies and cmd+v pastes in the prompt editor

Session ID: `6ec3f91d-7032-4c3b-83de-0a963e972952`
Date: 2026-08-29

The ask, in one message:

> "In the prompt editor is it also possible to allow CMD+C and CMD+V to copy and
> paste?"

Touched: `ui.go`, `clipboard.go`, `promptsel_test.go`, `README.md`.
No version bump — a release commit is its own step here.

## What the answer turned on: cats does not treat the two chords alike

Before binding anything, the question "will the host even hand us the press?"
was answered by reading cats itself (`~/projs/go/cats/cmd/catway/web/js/`).
The mux's UI is a web canvas, so its key layer decides every ⌘ chord:

```js
// 20-keys.js — the curated set handed DOWN to a pane
const CMD_TO_PANE = new Set(["KeyS", "KeyP", "KeyE", "KeyF", "KeyD", "KeyG", "Slash"]);
// …and, further down, two explicit exceptions:
if (e.metaKey && !e.ctrlKey && e.code !== "KeyC" && e.code !== "KeyZ" && !cmdGoesToPane(e)) return;
// …but ⌘V is intercepted before any of that:
if (e.type === "keydown" && e.metaKey && !e.ctrlKey && isV) { e.preventDefault(); pasteText(); return; }
```

So the two halves of the ask arrive by different roads:

```
⌘C  →  falls through to a kitty-protocol pane  →  super+c  →  a key we can bind
⌘V  →  the client reads the clipboard itself   →  a paste event  →  tea.PasteMsg
```

Which means ⌘V was *already* pasting under cats (Update's `tea.PasteMsg` case
has handled bracketed pastes since selections landed, and both the textarea and
the textinput handle the message themselves). ⌘C was simply unbound. The `cmd+v`
binding added here is therefore the **fallback road** — for a host that forwards
the keystroke instead of acting on it — and is honestly documented as such
rather than sold as the thing that made paste work.

This also corrected a stored memory that said "the whole Cmd space is bindable
under cats". It is not: it is `S P E F D G /` plus `C` and `Z`, and a chord
outside that set is silently inert.

## The copy half

`promptCopyChord` now names `ctrl+c`, `super+c`, `meta+c`, and the selection
block at the top of `updateForm` matches on it instead of on the literal
`"ctrl+c"`.

The one design decision worth its ink: **cmd+c does not carry the quit.**
Overloading quit on `ctrl+c` is a liberty the feature already takes and defends
(the guard is that something must be selected); taking it a second time on a
second spelling is not defensible — a hand reaching for ⌘C over a stray click is
asking to copy, never to leave. So with nothing selected the chord says

> nothing selected — sweep the prompt, or hold shift with ←/→, then copy

and returns. `ctrl+c` with nothing selected still quits, unchanged.

## The paste half

`pasteFormClipboard` (ui.go) reads the clipboard and hands the text on as a
`tea.PasteMsg` rather than inserting it:

```go
return m.forwardForm(tea.PasteMsg{Content: text})
```

Both roads then end in the field's own paste handling — the one that already
knows the caret, the undo state and the textarea's soft wraps. A second
insertion path in our file would be a lesser copy of it, and the two would
disagree exactly where it matters: a multi-line paste into a wrapped prompt.

A paste is an insertion, so it replaces a standing selection. That is wired in
the same span switch that already handles typing and deleting over a highlight,
one case above `promptSelInsertKey`, so the run is gone before the chord reaches
the main switch and the text lands at the caret the deletion left behind.

Refusals, per the house rule:

| situation | what it says |
|---|---|
| annotation bar focused | `pasting works in the title and the prompt` |
| clipboard holds no text | `the clipboard has no text` |
| pbpaste failed | the error, on `formErr` |

### Reading the clipboard

`clipboard.go` gained `readClipboardText`, the mirror of the existing
`copyTextToClipboard` — and deliberately *not* symmetric with it. A copy can be
shouted down both channels at once (OSC 52 **and** pbcopy) because both carry
the same bytes; a read has to come back with an answer, and only one channel can
give one here and now:

- **pbpaste** — immediate, and the right pasteboard whenever the process is on
  the user's Mac, which is the case this tool lives in.
- **OSC 52's read half** (`tea.ReadClipboard`) — survives ssh and multiplexers,
  but the reply arrives later as a `tea.ClipboardMsg`, and most terminals refuse
  the read outright (it lets a remote program read the local clipboard).

`supported=false` means "no local pasteboard here, ask the terminal instead" —
not a failure. Off macOS the chord takes that road, and a new `pendingPaste`
flag on the model is what makes the answer belong to the request: a terminal may
volunteer a clipboard report, and text appearing in a prompt nobody asked to
paste into is worse than a paste that never arrives.

`readClipboardText` is a `var`, the same test seam `cdxStateFile` uses in
`export.go`, so the tests hand the editor a clipboard instead of reading
whatever the machine running them happens to hold.

## Tests

Seven new tests in `promptsel_test.go`, on a `cmdKey(r)` helper that builds both
spellings of a Cmd chord (`super+x` and `meta+x`) the way `cmdD` already did:

- `TestPromptCmdCCopies` — both spellings, over a selection: copies, does not
  quit, reports the count, leaves the highlight standing.
- `TestPromptCmdCWithNothingSelectedSaysWhy` — the no-quit rule.
- `TestPromptCmdVPastes` / `…ReplacesTheSelection` / `…PastesIntoTheTitle`.
- `TestPromptCmdVSaysWhyWhenItCannot` — empty clipboard, annotation bar.
- `TestPromptCmdVFallsBackToTheTerminal` — the OSC 52 road, including that an
  unrequested `tea.ClipboardMsg` pastes nothing.

`go vet ./...` clean, `go test ./...` green.

## Left alone, on purpose

- **The footer.** It still teaches `ctrl+c copies` — the binding that cannot
  lose. `cmd+d dup line` is on that line only because no chip and no ctrl chord
  stands for it; copy and paste have both.
- **The version.** Feature commits land unbumped here; `chore(release)` is its
  own commit and its own decision.
- **Pasting an image into the prompt.** ⌘V with a picture on the clipboard is
  the attachment editor's job (`ctrl+o`, then `ctrl+v`), and it already has the
  three-way answer for a flavor macOS will not hand over as PNG.
