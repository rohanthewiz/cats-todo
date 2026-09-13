# Session: enter and paste at every caret, and tab that indents — v0.30.2

Session ID: `session_01WdVN4nTSETcEQeSCL2pCfu`
Date: 2026-09-12

The asks, in order:

> "Using ALT-Click I do get multiple carets, however the prompt editor doesn't
> allow multiple new lines on them"

> "Yes fix multi-line paste"

> "Commit both changes but hold off on the release for one more tweak. I would
> like to allow tabs as input within the prompt editor"

Commits: `7627311` feat(form): enter and multi-line paste act at every caret ·
`375673c` feat(form): tab and shift+tab indent in the prompt · `efc7fec`
chore(release): v0.30.2 — pushed to `origin/main`.

Touched: `promptcarets.go`, `promptindent.go` (new), `ui.go`, `main.go`,
`cats-plugin.toml`, `README.md`, `.claude/skills/cats-todo-dev/SKILL.md`,
`promptcarets_test.go`, `promptindent_test.go` (new), `promptsel_test.go`,
`spellpanel_test.go`, `annotbar_test.go`, `flagnote_test.go`,
`highvalue_test.go`, `prioview_test.go`, `session_test.go`.

## Enter was never broken — it was designed to end the mode

`updatePromptCarets` matched `InsertNewline` and called `endPromptCarets`
without inserting, on the theory that enter is "the key most likely to be
pressed because you thought the mode was already over". The README and a test
(`enter inserted while ending the mode`) both defended it. In use, though, it
read as the editor refusing newlines: alt+click three places, press enter,
nothing happens.

Enter (and `alt+enter`, `ctrl+j` — everything bound to `InsertNewline`) now
inserts a newline at every caret and keeps the mode on; each caret lands at the
start of the line its break made, so the next keystroke prefixes every new line.
`esc`, `↑`/`↓` and a plain click still end the mode.

### Why a new edit primitive: `spliceAtCarets`

`editAtCarets` edits rows *in place* and relies on walking carets backwards so
earlier offsets never go stale. That only works while the row count is fixed. A
newline adds rows, which shifts every row below — including carets the backward
walk has not reached. Rather than patch offsets, `spliceAtCarets` rebuilds the
value **top to bottom**: a `cur` builder holds the output line under
construction, each caret appends its row up to its column and then its insert,
and every `\n` in the insert closes `cur`. The caret's new `(row, col)` is just
`(len(out), runeLen(cur))` at that moment — nothing measured against the old
layout, so nothing to shift afterwards.

```
rows "alpha bravo" / "charlie", carets (0,5) (0,11) (1,7), inserting "\n"
  → "alpha\n bravo\n\ncharlie\n", carets (1,0) (2,0) (4,0)
```

`foldCaretsToCells` runs first: several goal columns past a short row's end all
clamp to one cell, and splicing there twice would add a phantom blank line and
split the carets across rows. Folding to effective columns loses nothing,
because after a splice every caret sits right after its insert anyway. If the
fold leaves one caret, the mode ends — checked inside the splice because a paste
arrives from `Update`, not through the key path's own check.

## Multi-line paste: the VS Code "spread" rule

A paste used to keep only its first line (`strings.Cut` at `\n`). Now, in
`pasteLinesAtCarets`:

- **lines == carets** → one line per caret, top to bottom (copy three names,
  alt+click three places, paste).
- **otherwise** → the whole paste at every caret, newlines included.

One trailing newline is ignored *for the count only* (copying whole lines brings
one along); the count is taken after the fold, since the user counts drawn
carets. `\r\n` and bare `\r` are normalised to `\n` first.

The other two roads a paste can take also bypassed the carets:
`pasteFormClipboard` (the `super+v`/`meta+v` chord's local pasteboard read) and
the `tea.ClipboardMsg` OSC 52 answer both called `forwardForm` straight into the
textarea. Both now go through `pasteIntoForm`, which sends to `insertAtCarets`
when the mode is on. The chord also needed a case in `updatePromptCarets` — it
has no `Text`, so it fell to `default`, which ends the mode before the paste
lands. In cats ⌘V arrives as a bracketed paste anyway (see memory
`cats-mux-forwards-cmd-chords`), so that case was already covered by the
`tea.PasteMsg` fix.

Two "0 carets" notes were guarded: the `tea.PasteMsg` case and the tail of
`updatePromptCarets` both wrote `caretNote()` without checking a fold had ended
the mode.

## Tab indents — spaces, and tab leaves the focus ring in the prompt

**Literal tabs are not possible without fighting the library**, which set the
design:

- bubbles' textarea sanitises *every* insert — `SetValue` included — with
  `runeutil.NewSanitizer()`, whose default rewrites `\t` to four spaces; `rsan`
  is unexported, so it cannot be swapped. Every line tool here edits through
  `SetValue`, so a tab character would not survive the next edit.
- The selection overlay, caret paints and click hit-testing sum rune widths; a
  terminal renders `\t` out to a tab stop, misplacing every cell after it.
- Prompts are delivered by typing into Claude Code, where a tab keystroke has
  its own meaning.

So tab inserts `promptIndentUnit = "    "` — four, matching what a pasted tab
already becomes.

The user chose (asked via AskUserQuestion) **"always outdent, like an editor"**
over two alternatives that kept shift+tab as a way out of the field:

| key | nothing swept | lines swept | column mode |
|---|---|---|---|
| `tab` | 4 spaces at the caret, mid-line too | indent every touched line (blank skipped), sweep kept | 4 spaces at every caret |
| `shift+tab` | up to 4 leading spaces off the caret's line | same off each line, sweep kept | outdent each caret's row once |

Consequence: from the prompt, **a click is the only way out**. Title and the
annotation bar keep tab/shift+tab for the ring; shift+tab from the title wraps
to the bar. Refusals are worded ("nothing to indent — the swept lines are
empty", "nothing to outdent — …").

Implementation notes (`promptindent.go`):

- `promptIndentDir` matches on `Code == KeyTab` and modifier bits (ctrl/alt/
  super/meta mean "not an indent"), like `promptLineMoveKey`, so kitty lock bits
  don't matter. It is hooked **above** `clearPromptSel` in `updateForm`, beside
  the other selection readers.
- `reindentPromptRows` returns per-row deltas; `remapIndentOffset` carries both
  sweep ends through them. On an indent, **column 0 stays 0**, so a sweep that
  began at a line start still covers the new indent (and the next shift+tab
  still sees it). A sweep ending exactly at the next row's first character does
  not claim that row (`promptRowRange`), but its offset still shifts by the
  growth above — pinned by `TestPromptTabSweepEndingAtALineStart`.
- `outdentAtCarets` works per *row run*, not per caret: `editAtCarets` calls its
  edit once per caret and would outdent a shared row twice.

### Footers

- The editor footer's tab segment follows focus: `tab indents` in the prompt,
  `tab switch field` elsewhere. The new text is shorter, so the 118-cell budget
  noted in the skill still holds.
- The column-mode footer grew (`enter breaks each`, `tab indents`) to 128 cells,
  and `fitFooter` trims from the right — it cut `esc ends`, which
  `TestCaretsFooterTeachesTheMode` caught. The exit moved to **second**, just
  after "typing goes on every line": knowing how to leave a mode outranks any
  single thing it does.

### Tests that walked the ring through the prompt

Ten existing tests reached the annotation bar with `tab` from the prompt (or the
title with `shift+tab`). Where the ring was incidental they now call
`m.focusForm(...)` directly; the two that are *about* the ring were rewritten to
walk from the stops that still own tab — `TestAnnotBarTabRing` pins
title⇄prompt/bar both directions, and `TestTabSkipsTheNoteFieldWhileTheFlagIsDown`
checks the skip across the ring's wrap (shift+tab from title, tab from bar),
which is exactly where the note's slot sits.

## Release

v0.30.2 in `main.go` and `cats-plugin.toml`. Strictly the skill's rule says a
feature bumps the minor; the patch number had already been agreed and written
into the README ("Until v0.30.2…"), and v0.30.1 was likewise a patch after a
feat, so the repo's practice was followed.

## Tests

New: `TestCaretsEnterBreaksEveryLine` (all three newline spellings),
`TestCaretsEnterOnASharedRow`, `TestCaretsEnterFoldsCaretsInOneCell`,
`TestCaretsMultiLinePaste` (spread, trailing newline, whole paste, CRLF/CR,
shared-row spread, fold-to-one ends the mode),
`TestCaretsTakeTheTerminalsClipboardAnswer`, and all of
`promptindent_test.go`. The enter case was removed from
`TestCaretsEndOnTheKeysThatMeanOneCaret`; the first-line-only paste assertion
was replaced. `go vet` clean, `gofmt` clean, `go test ./...` green at every
commit; `bin/cats-todo version` → `cats-todo 0.30.2`.

## Next

- `.claude/skills/cats-todo-dev/SKILL.md` says `.claude/` is gitignored, but
  `SKILL.md` is tracked (it showed in `git status` and went out in `375673c`).
  Correct the sentence — or untrack it, if that was the intent.
- The skill's versioning rule ("minor for a feature, patch for a fix") and the
  repo's practice (v0.30.1 and v0.30.2 were patches after `feat` commits)
  disagree; settle which one is the rule and make the skill say it.
- The Cmd+V chord's *local pasteboard* road into the carets
  (`pasteFormClipboard` → `pasteIntoForm`) is untested because
  `readClipboardText` reads the real macOS clipboard; a small injectable seam
  would let a test drive it. The OSC 52 road is covered.
- Possible refinement: tab inserts a fixed four spaces; aligning to the next
  multiple of four (true tab-stop behaviour) would line up mid-line columns
  better. Not requested.
- The column-mode footer is now over 120 cells, so narrow panes lose its tail
  (`←/→ moves them`, `ctrl+a/e line ends`). Acceptable for now; revisit if the
  mode gains another key.
- Non-goal: literal `\t` characters in the prompt — blocked by the textarea's
  private sanitizer, cell-width rendering, and delivery as keystrokes to Claude
  Code (see above).
- Non-goal: a keyboard road *out of* the prompt field. The user chose tab and
  shift+tab as indent/outdent there, with a click as the way out.
