# Session: why ⌘V didn't paste into the prompt editor

Session ID: 37fe4fed-cc42-44ae-9f4f-5fb01db0aaff
Date: 2026-10-05
No code changes in either repo. Diagnosis only, plus one backlog item in cats.

## The ask

> Why can't CMD+v paste inside of the prompt editor?

Host: Cats.app (the macOS WKWebView client). The toast said "clipboard has
no text".

## Diagnosis

There was no bug. When ⌘V was pressed, the system clipboard was completely
empty, and the toast reported that correctly. A plain copy from a terminal,
pasted straight afterwards, worked. The user couldn't recall where the original
copy came from.

How ⌘V reaches the editor under Cats.app:

```
⌘V keydown ─▶ 20-keys.js:181 pasteText()
           ─▶ 21-clipboard.js clipRead() ─▶ window.catsClipRead (native bridge)
           ─▶ catapp window_darwin.m "read" ─▶ catappClipRead ─▶ pbpaste
           ─▶ "" ? toast "clipboard has no text"
              : {t:"paste"} ─▶ server bracketed-paste ─▶ tea.PasteMsg (ui.go:930)
```

What was ruled out on the cats-todo side:

- Both roads into the editor exist and are tested. A host-performed paste
  arrives as `tea.PasteMsg` (`ui.go:930`). A forwarded chord arrives as
  `super+v`/`meta+v` (`ui.go:3153` → `pasteFormClipboard`, which reads with
  `pbpaste`). `go test -run 'Paste|Clipboard|CmdV|Super' ./...` passes.
- The installed binary is 0.43.0, built today. ⌘V support shipped in v0.20.0
  (`3acc03c`).
- cats-todo only writes the clipboard from three places
  (`copyTextToClipboard`: the selection copy, a Next List item's ID, its
  prompt), and each one copies text it already has. It never clears the
  clipboard.

Evidence for the empty clipboard: `pbpaste` returned 0 bytes, and
`osascript -e 'clipboard info'` returned nothing, sandboxed or not. Copying
an image or a file still leaves an entry, so a completely empty pasteboard
means something cleared it.

## Found along the way

In cats, any pane can wipe the Mac clipboard without anything showing.
`parseOSC52Clipboard` (`internal/orchestration/osc52.go`) treats an empty
`52;c;` payload as a clear, on purpose. The page relays it
(`19-messages.js:147`) to `clipWrite`, and in Cats.app that runs `pbcopy` with
no input. That is one possible cause, unconfirmed. A password manager's timed
clear is the other likely one.

That was added to the **cats** backlog (`~/projs/go/cats/.cats-todo/todos.json`,
which is gitignored there): "Empty OSC 52 write silently wipes the Mac
clipboard", low priority. It asks for one of two fixes: ignore empty writes, or
keep the clear and show a notice that names the pane.

Also noted: Cats.app's processes (`catapp`, `catway`) run with no `LANG` or
`LC_*` set. Nothing broke here because of it, but if the clipboard holds
non-ASCII text, `pbpaste` and `pbcopy` in that environment are worth checking.

## If it happens again

Run `osascript -e 'clipboard info'` right after the failed paste. Empty output
means the clipboard was cleared. Note what was copied and which pane was
active.

## Next

Closed: None. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
The one item raised went to the cats backlog, not this list.
