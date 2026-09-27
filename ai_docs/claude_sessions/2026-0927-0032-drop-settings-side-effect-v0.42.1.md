# Session: a drop's /model saves the user's default, and v0.42.1

Session ID: 48536950-83c8-462f-92e4-5896c04d09cb
Date: 2026-09-27
Mode: unattended backlog run (the user away for several hours), starting
at N-017, with a commit per item and a wrap every two.

## N-017: the live run stopped on a side effect

The plan was to drive a real claude pane through cats' control socket. A
fresh workspace (`workspace.create`) got a tab running `claude --model
sonnet` in this repo. A warm-up question (`PONG17`) made the conversation
warm, and then `/model haiku` and `/effort low` went in by hand to see what
each prints before scripting the drops.

What it showed:

- The *Switch model?* dialog comes up on a warm pane in Claude Code
  2.1.283, worded as `panesetup.go` expects (`Switch model?`, `❯ 1. Yes,
  switch to Haiku 4.5`). Enter on it switched the model.
- The confirmation line reads `Set model to Haiku 4.5 **and saved as your
  default for new sessions**`, and `/effort low` reads `Set effort level to
  low (saved as your default for new sessions): …`. `~/.claude/settings.json`
  now said `"model": "haiku"` and had a new `modelSettings.claude-haiku-4-5`
  entry.
- `/effort`, sent about 0.5s after the dialog was answered, printed its line
  whole.

The session's attempt to put the settings file back was refused by the
auto-mode classifier (self-modification). From `~/.claude/history.jsonl`,
the last values the user set were `/model opus` and `/effort high`, and no
haiku effort had ever been set. **The user needs to restore `"model":
"opus"` and may drop the `claude-haiku-4-5` entry.** A desktop notification
said so. Because every further live `/model` or `/effort` would rewrite the
file again, the timing check was not run, and N-017 is marked
**Needs the user**, behind the new N-070.

The Claude Code binary has a `for this session only` branch for both
commands. What selects it was not traced; the command's help says a pick
"becomes the default for new sessions".

The test workspace and pane were closed afterwards.

### A tool found along the way

`catctl probe --viewer --url ws://127.0.0.1:8422/ws` attaches to this
session's catway (the Cats.app child, port 8422; other catways on the
machine belong to other checkouts) as a headless browser client. It can type
keys with modifiers, paste, click at cells, and dump or capture pane grids,
so the "live-test in cats" items can be driven without a person, except
where they are about how something looks.

## N-069: v0.42.1

`main.go` and `cats-plugin.toml` bumped to 0.42.1, `chore(release): v0.42.1`
(`a2bae17`), annotated tag `v0.42.1 — a headless host install leaves the
backlog offer unspent`, and `git push origin main v0.42.1`, which also
carried `56c5c4d`.

## Next

Closed: N-069. Declined: None. Raised: N-070.
Deferred: None. Promoted: None.
Updated: N-017. Full list: `ai_docs/todo/next-list.md`.
