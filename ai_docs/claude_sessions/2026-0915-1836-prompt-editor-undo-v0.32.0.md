# Undo in the prompt editor — cmd+z and the context menu — v0.32.0

Session: `78d175de-050a-4866-b3de-6c192e7872d4`
Date: 2026-09-15

## Ask

"Please add an undo feature linked to CMD+Z and the context menu in the Prompt
editor"

Then: "yes finish the release".

## The shape of the problem

`charm.land/bubbles/v2/textarea` has no undo of any kind — no keymap entry, no
history, nothing to turn on. So the whole thing is ours.

The editor now has about twenty ways to change a prompt, spread over a dozen
files: typing, the deletes, a bracketed paste, `⇅ Sort`, `tab`/`shift+tab`,
`alt+↑/↓`, `cmd+d`, the column mode's multi-caret typing, the `@` file picker,
the prompt library, a spelling correction from the panel. The obvious design —
each operation snapshots itself before it acts — is the one that rots: the next
operation added forgets, and the hole is silent.

## What was built

`promptundo.go` (new), plus a wrapper around `model.Update`.

### One commit point

`Update` now snapshots the editor around the routing and the old body is
`route`:

```
Update(msg)
  ├─ stage not editsPrompt() ────────────▶ route(msg)          (no cost)
  ├─ before := promptEditState()          text + caret offset
  ├─ next, cmd := route(msg)
  ├─ next's stage not editsPrompt() ─────▶ return              (left the editor)
  └─ nm.commitPromptEdit(before, msg)     compare, push or coalesce
```

`editsPrompt()` is `stageForm`, `stageSpell`, `stageFiles`, `stageSnippets` —
the form and the sub-stages that write *into* the editor. Requiring it on both
sides is what keeps the history about one editor: arriving on the form from the
list replaces the whole value, which is a new session, not an edit.

Nothing that changes a prompt has to know the history exists, which is the whole
point. A paste, a menu row and a picker on another stage are all recorded by the
same comparison.

### Coalescing

An entry is `{text, caret}` as it was *before* the edit, so restoring one
restores both. `promptEditKindOf` classifies the message with the same
predicates the selection uses (`promptSelDeleteKey`, `InsertNewline`,
`msg.Text != ""` — read off the editor's own keymap, so "this is typing" means
the same thing in both places):

| kind | from | coalesces |
|---|---|---|
| `editTyping` | a character or a newline went in | yes, until a space or a newline closes the run |
| `editDeleting` | backspace / delete / ctrl+w / alt+backspace | yes |
| `editOther` | everything else, including every non-key message | never |

`editOther` as the default is deliberate: an operation nobody thought about here
gets its own step rather than being folded into the keys around it.

**Only a key press or a click breaks a run when the text did not change.** This
is the load-bearing rule and it has a regression test. The cursor's blink is an
ordinary message on a timer, so "anything that didn't change the text ends the
run" would put every character typed slowly enough in its own step — how many
presses a word cost would depend on typing speed.

`applied` on the stack marks that the change the commit point is about to see
*is* the undo. Without it, undo pushes the state it just left and cmd+z
flip-flops between two versions forever.

Bounds: 200 steps **and** 4MB, oldest dropped first, never below one entry.
Either budget alone has a bad case (200 × a pasted 2MB plan; a thousand
one-character steps).

### Bindings and where it is taught

`super+z`, `meta+z` (the two spellings a terminal reports Cmd with) and
`ctrl+z`. The ctrl spelling is beyond the ask and worth it: Cmd only arrives
from a terminal that forwards it — cats does, Terminal.app does not — and
`ctrl+z` was doing *nothing* here, since bubbletea holds the pane in raw mode
and it was never a suspend. Opposite call from `cmd+d`'s, for the opposite
reason: `ctrl+d` is the textarea's delete-forward, so a duplicate over it would
break a key that works.

`↶ Undo` is the **last** row of the editor's context menu, against the
convention that puts Undo first. `firstLive()` opens the cursor on the top row,
so the top row is what a bare `enter` presses straight after the click — and
this is the one row that throws work away. Dim with an empty history, like the
selection rows. Its hint follows the terminal (`undoChord()`, the `modEnter`
idiom): `cmd+z` under the kitty protocol, `ctrl+z` otherwise. Same for the
footer segment, which rides at the very tail past `ctrl+p prompt library` —
only a ~240-cell pane ever reads it, which is affordable precisely because the
menu row prints the chord.

### Refusals in words

- not in the prompt → "undo works in the prompt"
- empty history → "nothing to undo — this prompt has not changed since the editor opened"
- last step → "undone · nothing left to take back"

### The honest limit

`✂ Split` writes prompts into the backlog and then takes the bullets out of the
text. Undo brings the text back; the prompts it wrote stay written. The editor
can only take back what is still in the editor. Said in the file header and in
the README.

## Touched

- `promptundo.go` — new: the history, coalescing, `undoPrompt`, `undoChord`.
- `ui.go` — `promptUndo` field; `Update`/`route` split; the `super+z`/`meta+z`/
  `ctrl+z` case in `updateForm`; reset in `beginAdd`, `beginEditRef`,
  `backToList`; the footer's tail segment.
- `promptsel.go` — `promptCaretOffsetIn(value, line, col)` factored out of
  `promptCaretOffset`, so the commit point reads text and caret with one
  `Value()` instead of two.
- `promptmenu.go` — `menuUndo` row.
- `promptundo_test.go` — 14 cases: word-at-a-time steps, the blink regression,
  deletions as their own run, a block indent as one step, a paste as its own
  step, no self-redo, both refusals, the three chord spellings, per-session
  history, selection and column mode cleared, both budgets, the footer chord.
- `keys_e2e_test.go` — `TestUndoKeyEndToEnd` drives `0x1a` down a real
  `tea.NewProgram` and checks the saved prompt lost its second word. The binding
  is the feature; only the wire proves it.
- `README.md` — a `## Undo` section beside the editing prose, above Spell check.
- `.claude/skills/cats-todo-dev/SKILL.md` — file map row and chord ownership.

`go test ./...` green. Not exercised by hand in a live cats pane.

## Release

A **minor**: a new capability.

- `354862d feat(form): undo in the prompt editor, on cmd+z and the context menu`
- `f73e216 chore(release): v0.32.0` (`main.go` + `cats-plugin.toml`)
- Annotated tag `v0.32.0 — undo in the prompt editor, on cmd+z and the context menu`.
  `main` and the tag pushed.

## Next

- Hand-test in a cats pane: that Cmd+z actually arrives (cats forwards Cmd), the
  menu row, and undo after a `@` insert and after a spelling correction — the
  two cross-stage paths that only the commit point covers.
- **Redo** is the obvious follow-up and was deliberately left out of scope.
  The pieces are there: pushing popped entries onto a second stack, cleared by
  the next real edit, on `shift+cmd+z`.
- Undo does not cover the title field. It is a one-line `textinput` with no
  history of its own; if it is ever wanted, it is a second stack, not this one.
- The form footer is now past full — the tail segments need a ~240-cell pane.
  Worth a pass on what that line is still teaching.
- Carried: live-test existing-pane drops with model and effort set; `/model`
  mid-conversation confirm modals; gate `/clear` on a detected agent; done stamp
  in bundle export and hover card; the Cmd+V pasteboard-into-carets test;
  column-mode footer width; back-tagging older untagged releases (ask first).
