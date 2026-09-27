# Session: autosave and the Next List send, live (N-034, N-038)

Session ID: 48536950-83c8-462f-92e4-5896c04d09cb
Date: 2026-09-27
Mode: unattended backlog run, third pair (after
`2026-0927-0037-probe-driven-live-tests`).

## N-034: autosave in a live pane (closed)

A scratch config set `autosaveSeconds: 15`. Typing one key a second through
the tick, frame by frame through the probe:

- `autosaved HH:MM` appears on its own row under `⚙ default session`, in the
  same muted `#92a498` as the 📎 and ⚙ lines;
- the prompt rows and the caret stay put (row 60 stayed on screen row 32,
  and the top row stayed `row 38`), and only the footer drops one row;
- the next key clears it (formNote is cleared on any key, by design);
- after the first write the add is an edit, so `ctrl+g scope` leaves the
  footer;
- esc after an autosaved add leaves no row, and esc after an autosaved edit
  (`ZZZ` already on disk) restores the original.

## N-038: the Next List's ✉ Send (closed)

The scratch project for this lived at `.cats-todo/live-n038` inside this
repo. The folder is gitignored, and claude inherits this repo's trust there
once the scratch dir has no `.git` of its own. With its own `.git`, claude
stopped at the folder-trust dialog. Accepting that would write
`~/.claude.json`, so the session did not. The dir had its own `next-list.md`
and an empty `.cats-todo/todos.json`. Without that backlog, the root walk
would have climbed to this repo's dogfood backlog.

All the checks passed: esc keeps the highlight and writes nothing, and the
send worked into a running pane (run and paste), a new session (opened in
the list's project) and a worktree session (paste). The headings and the done
copies were as specified. The worktree, its branch and its workspace were
removed afterwards.

## The finding: a drop is "pasted content" to the model (N-072)

The first run-mode send was delivered whole, and Haiku answered that it
follows instructions in pasted content only when asked. Claude Code 2.1.283
tags bracketed pastes as `<pasted_content>`, and `pane.send_input` is a
bracketed paste. A matrix on fresh Opus panes: realistic read-only tasks
were done, with or without the Next List framing. The synthetic "reply
ALPHA11" was done bare but questioned once framed. That framing plus a
`Please work on …` lead was also questioned, and Opus spelled out the cause:
"Your message contains only pasted text, with nothing you wrote around it."
Sonnet did everything. Raised as N-072, with the user to choose the road.
The recommended one is a typed send in cats.

## On the side

- The test claude sessions ran on Haiku, because the user's default model is
  still the `haiku` that N-017's first probe wrote. Only the user can put it
  back.
- N-060 in passing: the picker listed only this project's agent pane, plus
  `… More drop targets (1 running agent in other projects)` as the last row.
- N-062 in part: a successful send left the done copy and the heading, and
  an esc wrote nothing. Still unchecked: a failed send and the ⤓ mark.

## Next

Closed: N-034, N-038. Declined: None. Raised: N-072.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
