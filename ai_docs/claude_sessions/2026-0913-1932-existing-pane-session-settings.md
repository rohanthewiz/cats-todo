# Existing-pane drops apply session settings — v0.31.1

Session: `session_01LfoREcj6akjh5isVJVE6zG` (https://claude.ai/code/session_01LfoREcj6akjh5isVJVE6zG)
Date: 2026-09-13

## Ask

"When dropping a todo unto an existing session, if the todo has specific session
settings, always apply those to the existing session."

Then, about the version: "I would argue that this is fix for unexpected behavior.
If the user goes through the trouble to setup session attributes they should be
applied. I say v0.31.1"

## The problem

A todo's session settings reach the agent three ways (see `session.go`'s header):

```
model / effort / permission → flags on the agent's argv   (new sessions only)
clear first                 → "/clear" as its own message  (existing panes)
context, files, wrap-up     → text around the prompt body  (every drop)
```

A drop into a running pane has no argv, so model, effort and permission were
silently skipped there. The work ran on whatever the pane was already set to.

## What Claude Code accepts in a running session

Checked against the installed Claude Code 2.1.270 by reading the command table
in its binary:

- `/model [model]`: `local-jsx`, `immediate`. With an argument it switches the
  model without opening the picker.
- `/effort [<levels>|auto]`: `local-jsx`, `immediate`. The level list depends on
  the current model (`NNr` builds the hint from `n7(model)`).
- `/permissions`: manages allow/deny *rules*, not the mode.
- `/plan [open|<description>]`: "Enable plan mode or view the current session
  plan". On a pane already in plan mode it opens the plan view, which would
  swallow the prompt.
- The only general in-session mode control is `shift+tab`. It *cycles* from the
  current mode, which nothing on the wire reports.

So model and effort can be applied, and permission mode cannot.

## What shipped

### `session.go`: two pure helpers

- `paneSetupCommands(agent) []string` returns the slash commands to submit ahead
  of the prompt, in order: `/clear` (any agent, as before), then `/model M` and
  `/effort E`, sent only when `isClaudeCommand(agent)` is true. In a shell pane
  in run mode, `/model` would be executed as a command line.
  - `/clear` goes first, so the settings apply to the session that reads the
    prompt.
  - `/model` goes before `/effort`, because a model switch can clamp the effort.
- `paneUnapplied(agent) string` names what won't land, for the picker row:
  `permission mode` always, plus `model/effort` on a non-claude pane. It lives
  next to `paneSetupCommands` so the two can't disagree.

### `drop.go`: `performDrop`, existing-pane branch

The single `/clear` block became a loop over `paneSetupCommands(act.target.agent)`.
Each command is submitted with `sendInput(…, true)`, followed by `clearSettle`
(400ms). They are submitted in paste mode too: the pause is for the prompt, and a
setting left unsent would have the prompt glued onto it. A failure aborts the
drop with `applying /model to the pane first: …`, the same rule `/clear` already
had.

Scheduled drops get this for free, because `performScheduledDrop` reaches
`performDrop`, and `Schedule` already persists the pane's `Agent` (`schedule.go`
`sc.Agent`).

### `ui.go`

- `buildTargets`: an existing-pane row appends
  `· the session's <lost> can't be set on a running <agent> and won't be applied`
  when `paneUnapplied` is non-empty. The warning sits on the row so it is read
  before the pick ("refuse in words").
- `sessRowLabels`: the Model and Effort notes now read
  `--model on a new claude session, /model on a running one` (and the same for
  effort). The Permission note reads `…, on a new claude session only`.
- The panel footer's second line reads
  `model/effort also switch a running claude pane; permission can't`.

### `cli.go`

The `--model` and `--effort` help now mentions both roads.

### `README.md`

In "Session options", the table splits Model/Effort from Permission. There is a
new paragraph and block showing `/clear → /model → /effort → <prompt>`, with the
reasons for the order, the claude gate, paste mode, abort-on-failure, and why
permission mode can't be applied.

### Tests: `session_test.go`

- `TestPaneSetupCommands`: nil and unconfigured options; the full order for
  claude; a path to claude still counts as claude; codex and an undetected agent
  get only `/clear`; permission alone sends nothing.
- `TestPaneUnapplied`: claude loses only permission; another agent loses all
  three; clear and the text options are never reported.

`go test ./...` is green and `gofmt` is clean. **Not exercised against a live
cats pane.**

## Release

The version was argued to a patch (see the Ask). I had proposed v0.32.0. The
reasoning is recorded as a precedent in `.claude/skills/cats-todo-dev/SKILL.md`:
new code, but a fix, because configured settings were expected to apply.

- `4581dea fix(drop): apply a todo's model and effort to an existing pane`
- `ad87f66 chore(release): v0.31.1` (`main.go` + `cats-plugin.toml`, plus the
  skill precedent)
- Annotated tag `v0.31.1 — dropping onto an existing pane applies the todo's
  model and effort`. Pushed `main` and the tag.

## Next

- Live-test an existing-pane drop into a real claude pane with model and effort
  set, in both run and paste mode. Confirm that `clearSettle` (400ms) is long
  enough after `/model` and `/effort`, not just after `/clear`. The new-session
  path needed 2s (`newSessionSettle`).
- Check whether `/model <m>` on a pane *mid-conversation* (Clear first off) ever
  asks for confirmation, such as a cache or context warning. A modal there would
  eat the prompt.
- An effort level the target model doesn't accept (e.g. `xhigh`) is still
  "delivered": `sendInput` succeeds, Claude shows an error, and the prompt runs
  on the old effort without saying so. Consider a model-aware check, or at least
  README wording.
- `/clear` is still sent to any agent, including a pane cats didn't detect as an
  agent, where run mode would execute it at a shell. This behavior predates this
  session. Consider gating it on a detected agent the way `/model` and `/effort`
  are gated.
- Revisit permission mode on running panes if Claude Code gains a
  set-mode command. Until then, not applying it is a deliberate non-goal (see
  below).
- Carried: the markdown export of a bundle (`bundle.go`, around the
  `Created.Format` line) does not show when a done prompt was finished. Consider
  adding the stamp there too.
- Carried: the list hover card shows no completion stamp. Decide whether the
  card should spell out the full stamp the way `viewPrompt` does.
- Carried: an older binary (< v0.31.0) that saves a backlog drops `doneAt`. This
  is an accepted limitation, and worth knowing if stamps "disappear".
- Carried: add a test for the Cmd+V chord's *local pasteboard* road into the
  carets (`pasteFormClipboard` → `pasteIntoForm`). `readClipboardText` is a
  swappable `var`, and there is a stub helper in `promptsel_test.go`.
- Carried: Enter with the caret *inside* an indent (`"  |  x"`) leaves the spaces
  left of the caret as trailing whitespace on the upper line. Decide whether to
  trim.
- Carried: the column-mode footer is over 120 cells, so narrow panes lose its
  tail. Revisit if the mode gains another key.
- Carried: consider back-tagging older untagged releases (e.g.
  v0.30.0–v0.30.2). Raise it with the user before doing it.
- Non-goal: setting permission mode on a running pane with `shift+tab` or
  `/plan`. `shift+tab` cycles from an unknown starting mode, and `/plan` opens
  the plan view on a pane already in plan mode.
- Non-goal (carried): sorting the done group by `doneAt`. Array order stays the
  user's order.
- Non-goal (carried): fixed tab stops. The user chose carried indents.
- Non-goal (carried): literal `\t` characters in the prompt.
- Non-goal (carried): a keyboard road *out of* the prompt field.
