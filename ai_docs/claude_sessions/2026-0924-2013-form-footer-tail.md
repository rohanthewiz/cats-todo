# The form footer's tail

Session: `71d0d416-a283-4979-9387-918c586bcb1c`
Date: 2026-09-24

## Ask

N-029, pasted from the Next List: "The form footer is past full: its tail
segments need a ~240-cell pane. Worth a pass on what that line is still
teaching." Then `/sw form-footer-tail`.

## What the line was teaching

The caret footer (`formFooter`, `ui.go`) has seven standing segments that
come to exactly 118 cells, which keeps the field switch in a 120-cell pane.
That part was left alone. Past it, the tail had grown to eight segments:

```
ctrl+g scope · ctrl+l spelling · alt+↑/↓ move line · cmd+d dup line ·
ctrl+p prompt library · cmd+z undo · shift+cmd+z redo
```

Fully shown, that needed 244 cells (229 without scope), so the segments added
last were taught to almost nobody. Two findings:

- **Four of them already had a second teacher.** The right-click menu prints
  a chord on its ✓ Spelling (`ctrl+l`), ≡ Insert a prompt (`ctrl+p`),
  ↶ Undo and ↷ Redo rows. Undo and redo also go without saying: `cmd+z` is
  the one chord every editor puts in the same place.
- **`cmd+d dup line` was often teaching a key that can't arrive.** It is
  Cmd-only, with no ctrl fallback because `ctrl+d` is the textarea's
  delete-forward. Yet the footer named it in every terminal.

## The change

The new tail keeps only what nothing else teaches, plus one pointer to the
menu:

```
[ctrl+g scope] · ctrl+l spelling · right-click menu · alt+↑/↓ move line · [cmd+d dup line]
     133             151                170                 190                  207
```

- `ctrl+l spelling` leads the tail, so it still fits the 160-cell pane
  `TestSpellFooterNamesTheChord` pins. The squiggles in the prompt raise the
  question it answers, and no chip stands for the panel.
- `right-click menu` is named with nothing swept. While a run is swept, the
  existing contextual `right-click: split/sort/carets` segment (index 3) has
  already named the menu, so the tail pointer steps aside and the menu is
  named once.
- `cmd+d dup line` appears only under `m.kbEnhanced`, the same test
  `undoChord`/`redoChord` use.
- The prompt library, undo and redo segments are gone from the footer. Their
  menu rows teach them.

The full line is now 207 cells in add mode with both backlogs, and 192
without scope.

## Tests

- `TestUndoFooterNamesTheChordTheTerminalCanSend` and its redo partner became
  `TestUndoMenuRowNamesTheChordTheTerminalCanSend` and
  `TestRedoMenuRowNamesTheChordTheTerminalCanSend`. They now pin the menu
  row's `hint` (ctrl+z / cmd+z, ctrl+y / shift+cmd+z), since that is where
  the chord is taught.
- `TestSplitFooterTeachesTheMenuWhileSomethingIsSwept` now checks that
  `right-click: split` is absent when nothing is swept (the bare pointer is
  allowed there), and that `right-click` appears exactly once over a sweep.
- New `TestFormFooterTailFitsAWidePane` checks four things:
  - at 207 cells under `kbEnhanced`, the line ends in `cmd+d dup line`;
  - the menu pointer, spelling and move-line segments are present;
  - no library, undo or redo segment is on the line;
  - `cmd+d` is absent without `kbEnhanced`.

`go vet` and `go test ./...` pass. `gofmt -l` flags `promptcode.go`, which
this session did not touch.

## Docs

- The README's context-menu paragraph now covers both menu segments (the
  contextual one and the tail pointer), why the library, undo and redo left
  the footer, and why `cmd+d` is conditional.
- The `cats-todo-dev` skill's footer note now gives the tail's contents, the
  207-cell pin, and the rule that a chord a menu row prints belongs on the
  menu, not in the footer's tail.

Not released. This is a refinement of the footer, so it would ship as a patch.

## Next

Closed: N-029. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
