# Session: `@` opens a file picker in the editor

Session ID: `f46e56f0-cd83-44a3-bcda-3dfc99bcff83`
Date: 2026-08-17

The ask, in one line:

> "In the prompt editor allow @ to browse the file system. Feel free to borrow
> code from CDX (../cdx) but take it down to the file-level"

Three questions were put back before any code, and all three took the
recommended answer: insert `@relative/path` (not a bare path), **enter chooses
whatever is highlighted** — file or folder, CDX-style — with tab/→/`/` to open a
folder, and the picker is a **full-screen sub-stage** like the attachment editor,
not an inline dropdown.

New files: `filepick.go`, `filepick_test.go`. Touched: `ui.go`, `fuzzylist.go`,
`fuzzylist_test.go`, `formimages_test.go` (the `pressKey` table), `drop.go`
(comment only).

## What CDX turned out to be

Worth recording, because the ask assumed otherwise: **cdx has no `@`**. It is a
`cd` picker — a zoxide/fzf-style directory chooser — and its "path completion" is
a mode the whole query flips into when it holds a `/`, `~` or leading `.`. Its
filesystem read (`readSubdirs`) drops every non-directory on purpose, its
`choose` rejects anything that isn't a folder and quits the program, and around
that sit a frecency store (`state.json`), a directory stack panel, clipboard
chords, `exec`/`chdir`, and a stderr renderer because stdout carries the result.

"Take it down to the file level" therefore meant: borrow the *mechanism* —
`expandPath`, `shortenHome`, `homeDir`, `isDir`, the one-`os.ReadDir`-per-
keystroke listing, tab as drill-in, the keep-the-highlight-visible scroll — list
files as well as folders, and leave everything above the file level where it is.
The only true `@`-mention implementation in the tree is `rcode`'s
`fileMention.js` (Monaco, JS); its *semantics* — trigger on `@` at a word start,
Enter/Tab insert, Esc hides, replace the token — are the spec this follows.

## The design

### The picker is a browser, not a search

One directory at a time. `filePicker{root, dir, entries, err, list fuzzyList}`:
`dir` is where it is, `entries` is everything there (hidden included), and the
`fuzzyList`'s query box holds only the **partial typed within `dir`**. There is
no recursion, no `.gitignore`, no cache — a level at a time is cheap enough that
none of that is needed, and it is what makes the way in to a deep file the same
as at a shell prompt: a segment and a slash at a time.

```
  Insert a path   ~/projs/go/cats-todo/internal/

  │ 🔍 type to filter · / opens a folder                    │  3/3

  ❯ app/
    ctlproto/
    integration/

  enter insert · tab/→ or / open folder · backspace up · esc back
```

### One rule for every way a path arrives

`filePicker.edit` is the single path through which the query changes (typing,
paste, blink all go through it), and after every edit it runs `normalize`: if the
query holds a `/`, split it (`splitPathQuery`, cdx's `pathComplete` split) into
the directory it points at and the last segment; when that directory exists,
move there and keep the segment as the filter. That one rule makes `src/`,
`src/ui`, `../ma`, `~/`, and a pasted `~/projs/x/main.go` all do the obvious
thing without a special case each.

### The slash key, and why `~` and `..` wait for it

`/` is overloaded, deliberately, the way fish and zsh complete a segment:

- after a filter that is a folder's name (or `~`, or `..`) it is a path being
  typed — let it through, normalization walks in;
- otherwise it **opens the highlighted folder**, the same as tab;
- over a file with nothing openable, it is swallowed — a lone `/` in the box
  would match nothing and mean nothing.

The first bug the tests caught lived exactly here. The first cut treated a bare
`~` or `..` as a path query the moment it was typed (cdx does), so `..` went up
at once — and the `/` that naturally follows landed on an empty query and opened
the first folder of the parent. Fix: **only a slash makes a path.** `~` filters
on nothing until `~/` goes home; `..` until `../` goes up; the slash handler
knows both spellings (`startsAPath`) and lets them through. One consequence,
documented in the code: an absolute path can't be *started* with `/` — `~/`,
`../`, or backspacing up to the root are the ways there.

`backspace` on an empty box goes up and parks the highlight on the folder just
left, so backspace-backspace-backspace walks up a tree with the way back down
always under the cursor.

### The trigger

In `updateForm`, after the chord switch (so `clearPromptSel` and every chord case
have run) and only for the prompt field: `msg.String() == "@"` and
`promptAtWordStart` — the rune before the caret is whitespace (a newline counts)
or the caret is at offset 0, read through `promptCaretOffset` from
`promptsel.go`. The `@` goes into the editor **first**, like any other key, and
the picker opens after it: esc leaves a plain `@` behind (what someone who wanted
the character gets), a choice appends `path + " "` at the caret through the
textarea's own `InsertString` — the same call the toolbar's ↵ chip makes. An `@`
mid-word is an e-mail address and stays text; a paste is a `tea.PasteMsg`, routed
through `forward`, and can't open it at all.

`bubbletea` v2 detail that made this simple: `KeyPressMsg.String()` returns
`Text` when set, so a shifted `@` under the kitty protocol still reads `"@"`.

### What gets inserted

`fileInsertText`: relative to `m.ctx.projectDir()` when the entry is inside it
(the form Claude Code's own picker writes, and the one that survives the project
moving), else `~/…` or absolute; folders get a trailing `/`. `drop.go`'s
"no `@` prefix" comment on the image block was true and stays true — but the
prompt body goes over verbatim and may now carry mentions the author chose; the
comment says so.

### Two opt-in additions to `fuzzyList` (existing callers untouched)

1. **A scroll window** — `maxRows`/`top`, `ensureVisible` (cdx's, in filtered-
   item units) run by every mutator, and `rowsView`/`rowAtLine` walking the same
   `[top, top+maxRows)` with *absolute* indexes so `focusRow(i)` from a click still
   means the same row. A dim `… N more` line follows the rows (after, never
   before, so `rowAtLine`'s counting is untouched). Zero `maxRows` is every
   existing caller: nothing changes for the todo list or the drop picker.
2. **`prefixFirst`** — names beginning with the query lead, in list order (folders
   first, each sorted), then the fuzzy rest by score. This came from looking at a
   real frame: for `int` the fuzzy scorer put `init_test.go` above `internal/`,
   so `int/` opened nothing. cdx ranks prefix matches first for exactly this
   reason; a path segment being typed is the start of a name far more often than
   a scatter of its letters. The todo list keeps pure fuzzy — its queries are
   words from anywhere in a prompt.

### The form footer

`@ file` rides the caret-keys line (it is not a chord — it is a character the
editor already takes, given a second meaning). To keep `tab switch field` on the
120-cell line the pinned test guards, two segments were tightened: "line ends"
for "line start/end", "places caret" for "places the caret".

## Wiring, for the map

`stageFiles` in `uiStage`; `files filePicker` on the model; `beginFiles` /
`closeFiles` mirror `beginImages` / `closeImages` (blur both fields, refocus on
close); `updateFiles` is `updateTarget`'s shape plus the browser keys (`tab`,
`ctrl+i` — same byte as tab off-kitty — and `→` open; `backspace`; `/`; `pgup`/
`pgdown` by a window); `clickFiles` is `clickTarget` with `filesRowsRow = 4`
(same geometry as `targetRowsRow`, pinned by `TestFilesRowsMatchWhatIsDrawn`);
routing added in the key switch, `forward`, the click switch, `View`'s mouse-mode
set, `renderStage`, and `applySizes`.

## Tests

27 new, all in package `main`, table-driven where there is a table. The picker
tests browse a fixture tree from `t.TempDir()` (`.env`, `.git/`, `README.md`,
`docs/`, `main.go`, `src/ui.go`, `src/sub/deep.txt`) through the real `Update`,
via a `withPicker` helper that opens the form and types `@`, and a `typeAll` that
types a word a keystroke at a time. Covered: the trigger's word-start rule (and
its non-triggers: title field, mid-word, paste), folders-then-files order,
symlinks, hidden-follow-the-dot, `fileInsertText`, enter on folder and file,
mentions relative to `ProjectRoot`, `/` opening vs swallowing, a typed path
walking in and `../` back out, a pasted path landing in its folder, `~/`,
tab/→, backspace-up with the highlight parked, up stopping at `/`, esc leaving
the `@`, enter on nothing keeping the picker, an unreadable start dir, the row
constant against a frame, click-to-choose, mouse mode, the window fitting a
15-line pane and scrolling/paging/resizing, the heading staying one line under a
very deep path, and the footer segment. `fuzzylist_test.go` gained window and
prefix-first cases.

`newTestModel()`'s `WorkDir` does not exist on disk — every picker test sets
`m.ctx.WorkDir` to its fixture. `t.TempDir()` on macOS is under `/var` →
`/private/var`; the picker never resolves links, so tests compare unresolved.

## Where it stands

`go build`, `go vet`, `gofmt`, `go test ./...` all clean. Not done, by choice:
no chip on the toolbar for the picker (the footer segment teaches it), no mouse
wheel (nothing in the app handles `MouseWheelMsg`), and paths with spaces are
inserted unquoted — noted in `fileInsertText`'s comment as the agent's problem
to read rather than a guess at whose quoting rules apply. Version not bumped.
