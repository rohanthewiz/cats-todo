# Session: model and effort for a running pane, session only (N-079)

Date: 2026-11-05

The ask, dropped from the backlog: Next List item N-079. Switch a running
claude pane's model and effort for that session only, through the picker the
bare `/model` opens, since a typed `/model X` saves the user's default
(N-070). Then turn `paneSetsModelEffort` on and run N-017, N-043 and N-044
live. The tree already held the uncommitted typed-drops work (N-072,
`2026-1105-1848-typed-drops-pane-send-keys`). This session builds on it and
is uncommitted too.

## What the binary and a live picker showed (2.1.318)

- `ipe({canEnter:!1,startInSearchMode:!0})`: canEnter false means the
  picker no longer opens in search mode. 2.1.283's did, so the drive still
  leaves a *Type to search* footer with `↓`.
- `modelPicker:thisSessionOnly` runs only off search mode and only when
  `onSetDefault` is passed (the `/model` command passes it). It calls
  onSelect without the set-default flag. Enter calls `onSetDefault`, then
  onSelect.
- The model picker's `←/→` effort (clamped, per-model levels) rides with the
  pick. `Ie` prints `Set model to X for this session only with Y effort`
  and persists effort only when saving the default. So one picker carries
  both, and effort-only also goes through `/model` on the current row. The
  `/effort` slider shows its level only as a ▲, which can't be checked from
  text.
- Keybindings: `ModelPicker` has `left/right/s`, and `Select` has
  `home/end` (select:first/last). The footer prints a literal `s`, so a
  remap shows only in keybindings.json.
- The Fable consent's picker variant is titled `Switch to Fable?`.
- `Scroll wheel is sending arrow keys · use PgUp/PgDn to scroll` is a
  12-second hint plus telemetry. It doesn't change how keys are read.

Captured picker (pane started `--model haiku`): `❯ 5.  Haiku 5.5 ✔`, `↓ 7.`,
`… +6 models`, `◐ Medium effort (default) ←/→ to adjust`, footer
`Enter to set as default · s to use this session only · Esc to cancel`. 13
rows; Home/End work.

## Code

- `modelpick.go`: `pickSessionOnly(d, pane, model, effort, settle)`.
  - Refusals that touch nothing come first: `sessionOnlyKey`
    (keybindings.json) and an empty `send_keys` probe. An old server answers
    "not supported" (`errNoSendKeys`); a new one refuses "no keys". Neither
    types anything.
  - Then `/model` ⏎, and `parsePicker` reads every capture: rows below the
    last `Select model`, open only while the footer is the last line drawn.
  - `focusModel`: Home, every row past each window, `pickerRowFor` (alias =
    family's first row, never Default; `claude-x-5-5` → *X 5.5*; a date
    suffix is dropped), ↑/↓ by number, check ❯.
  - `setEffort`: one press at a time. A press that doesn't move means a
    clamp; a direction flip means an overshoot. Either refuses.
  - `commit`: press the key, then `readOutcome` below the last `❯ /model`
    echo (`resultLine` joins Ink-wrapped rows).
    - Session-only: the note.
    - Saved: an error.
    - *Switch model?*: Enter (Yes).
    - Hook ask or consent: stop with it up.
  - Every refusal after open is Esc first.
- `paneDriver` gains `capture` (client: `capture`, scope 0, unwrap).
- `session.go`: `paneSetsModelEffort = true`. `paneSetupCommands` is
  `/clear` only, never a typed `/model X`. `paneSessionPick` and
  `paneSessionPickWords` feed the Drop now dialog (`flowSetup`).
- `drop.go`: `performDropAt` runs `applyPaneSetup` (/clear), then
  `pickSessionOnly`, then delivers; `joinNotes`.
- Comments in `panesetup.go`, `ui.go`, `batchloop.go` updated.

Tests:

- `TestParsePickerLive` (the real capture), `TestParsePickerIgnoresTheConversation`
- `TestPickerLabelFor`, `TestPickerRowFor`, `TestSessionOnlyKey`
- `TestPickSessionOnly{ModelAndEffort,Alias,EffortOnly,Refusals,SearchMode,Outcomes,NeedsSendKeys,ReadsBelowTheEcho}`.
  These run against `simPicker`, which draws 2.1.318's picker and fails the
  test on Enter while the picker is up.
- `TestPaneSessionPick`

`TestPaneSetupCommands`, `TestPaneUnapplied`,
`TestRunningClaudeRowSaysModelAndEffortWontApply` and
`TestDropConfirmSummaryLoop` now pin the on state. `go test ./...` passes.
`TestPromptClickMatchesTheWalk` takes ~17s on its own; that predates this
work.

## Live (dev catway on 127.0.0.1:8431, Claude Code 2.1.318)

`~/.claude/settings.json` was checksummed before the run and checked after
every step: unchanged throughout (model `opus`, effortLevel `medium`). Drops
went through `performDropAt` from a throwaway `livedrop`-tagged test (not
committed).

- Clear on, sonnet/high: `/clear`, then `Set model to Sonnet 5.5 for this
  session only with high effort`. The prompt was typed whole 400ms later
  (`PONG-A`), and the banner read *Sonnet 5.5 with high effort* (N-017).
- Clear off, haiku: *Switch model?* came up, was answered, and `PONG-B`
  arrived (N-043).
- Clear off, effort low only: no confirm; `PONG-C`.
- PreModelSwitch `ask` hook (scratch project settings): the drop stopped
  with the dialog up. Esc there returns to the picker. Claude Code reloads
  project settings live, so the other pane in that directory got the hook
  too.
- Fable, pick only: no consent on this account; the cache confirm was
  answered and nothing was saved (N-044).
- Haiku 5.5 took xhigh: the sim's levels were wrong, and are now the sim's
  own. `claude-haiku-4-5` + max refused: *Haiku 4.5 takes no effort
  setting*, the picker was cancelled, and the pane said *Kept model as*.

## Next

Closed: N-079, N-017, N-043, N-044. Raised: N-097.
