# ced is not a drop target — editor panes out of the Drop into… picker

Session: `6f141c61-a28b-422c-9294-bde36933e5ea`
Date: 2026-09-22

## Ask

The user sent a screenshot of the **Drop into…** picker. It listed "＋ New ced
session", "＋ New ced session on a new worktree", and a running
`ced · chronos_converter [idle]` row. Their words: "Ced is just a plugin for
cats, not an LLM agent. It should not be a drop target."

The session started in the ced repo. The fix landed in cats-todo, because the
picker belongs to cats-todo.

## Cause

- ced reports its state over cats' hook socket with the agent label `ced`
  (`internal/cats/hooks.go` in ced). This is on purpose: cats'
  `pane.open_file` uses that label to find the editor pane, and it lets a
  blocked question in ced reach the phone. ced's `cats-plugin.toml` already
  calls it "A TOOL, NOT AN AGENT", and cats' own sidebar keeps it out of the
  coding agents through `editor.agents` (default `["ced"]`,
  `cats/internal/config/config.go`, `wire.EditorInfo.IsEditorAgent`).
- `buildTargets` (`ui.go`) counted any pane with `p.Agent != ""` as an agent.
  That scan feeds two parts of the picker:
  1. the running-pane rows, which gave the `ced · …` row, and
  2. the extra "New <agent> session" rows, one for each running agent not
     already in `newSessionAgents`. These gave the plain and worktree "New ced
     session" rows.
- `pane.list` does not send cats' editor policy, so cats-todo had no way to
  tell an editor pane from an agent pane.

## Fix — commit `e630341`

- `context.go`: new `editorAgents = []string{"ced"}` and
  `isDropAgent(p wire.PaneInfo) bool`, placed next to `isOwnPane`. It returns
  false for an empty label or an editor label, and matches case-insensitively
  (the same way `IsEditorAgent` does).
- `ui.go` `buildTargets`: the filter is now `!isDropAgent(p) ||
  isOwnPane(m.ctx, p)`. The new-session rows come from the same `agents`
  slice, so this one line removes all three ced rows.
- `context_test.go`: `TestIsDropAgent` checks that claude, copilot and codex
  are still offered, and that "", `ced` and `CEd` are refused. The code has no
  fake control socket for testing `buildTargets` as a whole, so the check
  lives in the predicate, the same way `TestIsOwnPane` does it.

`go vet` is clean, and `go test ./...` passes.

## Trade-off

`editorAgents` copies cats' default instead of reading cats' config. If a user
sets a different editor label in cats, that label must also be added here.
The more thorough fix would be for cats to add an editor flag to each pane in
`pane.list`, since the backend already has `EditorInfo`. That is a change to
cats' wire format, so this session did not make it.

## Next

- **Send info prompts to gonotes** (carried over). Add a drop target for
  info-marked prompts that delivers them to a notes plugin (gonotes) instead
  of an agent. Probably an Export-like "➦ Send to notes" that is available
  only when `Info` is set, through the cats control socket or a gonotes CLI or
  API. It could mark the prompt done once it is filed. Work out gonotes'
  intake contract first.
- **Editor flag on the wire** (new). cats could add an `Editor bool` field (or
  similar) to `PaneMeta` in `pane.list`, computed from `EditorInfo`. Then
  cats-todo could drop its `editorAgents` copy and follow whatever
  `editor.agents` the user has configured.
- **Release** (new). This fix is committed but not released. Cut the next
  patch (v0.33.1) when it suits.
