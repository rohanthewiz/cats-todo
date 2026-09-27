# Two-line footers on every page, and gofmt for hangup.go (N-067)

Session: `6fc7dec0-c341-4099-8dd9-862e9f3ed15b`

## What was asked

1. "Use two lines for the shortcut hints at the bottom of each page."
2. Close N-067: `gofmt -l` flagged `hangup.go`.

## What changed

### Two-line footers — `5e86766 feat(ui): …`

- **`footerRows` / `footerBlock`** (`ui.go`, after `fitFooter`), with
  `footerRowCap = 2`. They fill the first line, then the second, and only then
  drop segments from the tail. Segment order is kept (a short late segment
  never jumps a longer earlier one), because every caller's order is its
  priority. The first segment always survives, and an unknown width
  (`m.width <= 0`) keeps everything on one line. `footerBlock` renders each
  line on its own: a lipgloss render of a multi-line string pads every line to
  the widest.
- **`fitFooter` stays** as the one-line form, used where a footer is already
  two hand-picked lines: the session panel, the View options panel, and the
  form's narrow case (chords line + caret line).
- **Pages switched** to `footerBlock`: list (wide branch, `listFooter`; the
  narrow branch was already two fixed lines), form (wide: the caret segments
  get both lines; narrow: two lines in total as before), column mode, the
  Batches page and the batch record, the Next List, the file picker (and so
  import/export browse), the prompt library, spelling, export, import, peer
  address, the scheduler, the drop picker, the prompt view. Plain-string
  footers were split on `" · "`.
- **Row budgets** grew by one where they had counted a single footer line:
  `sizeBatchesPage`, the batch view's cut, `filePicker.resize`,
  `snippetPicker.resize`, `nextPage.resize`. The list (`listChromeBelow`) and
  form (`formChromeHeight`) already budgeted two, and the prompt view's
  `viewHeight` has slack.
- **Batch composer** (`batchGeom`): `footY = h - footerRowCap`, bar and note
  one row up, and the frame is `footY + footerRowCap` lines.
- Tests: `footer_test.go` (`TestFooterRowsWrapBeforeConceding`,
  `TestComposerFrameHoldsTwoFooterLines`). `TestPickerWindowFitsThePane` now
  expects `40-filesRowsRow-2-footerRowCap`.
- README: a paragraph after "Both button rows shrink rather than wrap".

A footer that fits one line stays one line; it is not forced into two. If
two lines should always show, the change is in `footerRows`.

### N-067 — `d46ab33 style(hangup): …`

`gofmt -w hangup.go` added the blank `//` line before the SIGHUP bullet.
`gofmt -l .` is empty.

## Next

Closed: N-067. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
