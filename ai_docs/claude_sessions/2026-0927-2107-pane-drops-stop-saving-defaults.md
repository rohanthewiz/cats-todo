# Session: running-pane drops stop saving the user's model default (N-070)

Date: 2026-09-27

## The question

N-070 asked the user to choose. Since Claude Code 2.1.283, a typed
`/model X` or `/effort X` also saves the pick as the user's default for new
sessions. So a drop into a running claude pane with a model or effort set
rewrote `~/.claude/settings.json`. The options were (a) keep sending them
and warn, (b) stop and list them as unapplied, or (c) find a session-only
road.

## What the binary says (2.1.283)

It was read with `strings` and a Python byte search over
`~/.local/share/claude/versions/2.1.283`. A plain `grep -a` with a long
context regex timed out on the 200MB binary.

- The typed command's path calls the switch with
  `save = !isNonInteractiveSession`. No argument or flag changes that.
  `/effort X` is the same (`Oje(e, setAppState, !isNonInteractiveSession, …)`).
- `/model` with no argument opens the **ModelPicker**. It starts in search
  mode (`startInSearchMode: true`): typing filters, and `↓` (or Enter with a
  filter) moves into the list. In the list, **Enter = set as default** and
  **`s` = `modelPicker:thisSessionOnly`** (footer: `s use this session
  only`). `←/→` are `modelPicker:decreaseEffort/increaseEffort`. Enter with
  an *empty* filter picks the highlighted row and saves the default.
- `/effort` with no argument opens the **EffortSlider**: `←/→` move, Enter
  saves, and **`s` = `effortSlider:thisSessionOnly`**.
- The picker road still goes through the mid-conversation *Switch model?*
  check (`URn`), so `applyPaneSetup`'s confirm handling still matters there.

Driving the pickers needs `↓`, `←/→` and a bare `s`. `pane.send_input`
pastes its text, and Enter is the only bare key it can send. So (c) needs a
key-sending verb in cats' wire, the same thing N-072 (a) wants.

## Decision and change

The user chose **(b) now, then (c)**.

- `session.go`: a new `const paneSetsModelEffort = false` gates both
  `paneSetupCommands` (no more `/model`/`/effort`) and `paneUnapplied`
  (claude now names `model/effort` too). One value drives both, so the picker
  can't promise what the drop won't do. The command plumbing and
  `applyPaneSetup` stay for road (c). The section comment explains why.
- Picker row: `the session's model/effort won't be applied to a running
  claude`. It used to say "can't be set", which is no longer the reason.
- Drop now dialog: `model not applied, the running pane keeps its own`.
- Comments in `drop.go`, `batchloop.go` and `panesetup.go`, plus the skill's
  file map and live-test warning.
- README: the existing-pane section rewritten, with a new "Model and effort
  in a running pane" subsection. The loop note now says a same-session loop
  runs on the model the first prompt's session started with.
- Tests: `TestPaneSetupCommands` and `TestPaneUnapplied` pin the new
  behaviour, `TestDropConfirmSummaryLoop` has the new wording, and the new
  `TestRunningClaudeRowSaysModelAndEffortWontApply` covers the picker row.

`go test ./...` passes. There was no live run: nothing new is typed into a
pane, and the rows are checked by unit tests.

Not released. The unreleased drop dialog is waiting for v0.43.0 (N-078), and
this fix can go with it.

## Next

Closed: N-070. Declined: None. Raised: N-079.
Deferred: None. Promoted: None.
Updated: N-017, N-043, N-044 (now wait on N-079). Full list:
`ai_docs/todo/next-list.md`.
