# Session: the drop picker's fold and the double-click, live (N-060, N-064)

Session ID: 48536950-83c8-462f-92e4-5896c04d09cb
Date: 2026-09-27
Mode: unattended backlog run, fifth pair (after
`2026-0927-0058-next-list-marks-and-menu-live`).

## Stems corrected

The two previous docs of this run had stems chosen ahead of the clock
(`0100`, `0110`). They are renamed to their real commit times, `0053` and
`0058`, and every reference in `next-list.md` and the docs follows. The order
is unchanged.

## N-060: the fold, in all four pickers (closed)

Setup: the scratch project's claude pane, a decoy workspace `ct-other` with a
claude pane in `.cats-todo/live-other`, and this session's own pane in
`cats-todo`. Each of the four pickers (backlog `shift+enter`, Next List send,
`ctrl+s` schedule, and the batch composer's Target row) showed:

    ＋ New Claude Code session
    ＋ New Claude Code session on a new worktree
    claude · ct-live (this project)  [idle] …
    … More drop targets (2 running agents in other projects)

Choosing the More row unfolded the list, by enter in two pickers and by a
click in the other two. The highlight landed on the first revealed agent.
The unfolded order follows the workspaces, so this project's agent sits
between the two others.

Care point for the rig: the first revealed agent was this very session.
After an unfold the only safe key is esc.

An accident on the way: from the schedule picker's top row, ↑ does not wrap
to the More row, and the enter after it scheduled a new-session drop at
03:00. It was cleared the documented way: ctrl+s on the row, empty the box,
enter. The scratch backlog holds no schedule now.

## N-064: the double-click (closed)

In a prompt holding `alpha bravo charlie delta`, two probe clicks on
`charlie` with gaps of 0, 150 and 350ms painted the word in the selection
background `#4a6656`. A 650ms gap painted nothing. That fits the 500ms
`doubleClickWindow`. The plain-terminal half of the item was not driven.

## Next

Closed: N-060, N-064. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
