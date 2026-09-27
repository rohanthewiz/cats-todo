# Session: the hover card with a real pointer, session topics, v0.42.2 (N-041, N-075, N-076)

Session ID: 48536950-83c8-462f-92e4-5896c04d09cb
Date: 2026-09-27
Mode: unattended backlog run, ninth pair (after
`2026-0927-0138-loop-view-and-gone-target`).

## A second rig: Chrome on cats' own page

`catctl probe` has no motion op, so a hover card can't be driven with it.
cats serves its browser front-end on the same port as the probe
(`http://127.0.0.1:8422/`), and `?ws=<id>` opens it as a separate window on
one workspace (`36-windows.js`). The user's desktop view is left alone. The
Claude-in-Chrome tools gave real pointer motion over it, and keys still came
from the probe.

Two cautions for next time:

- **Aim.** Chrome's pointer lands lower than the screenshot says, by about one
  row near the top and three near row 20. Clicks and hovers agree with each
  other, so aim by what gets highlighted. An early "the card is for the wrong
  row" was this offset, not a bug: a left click at the same spot selected
  the same row.
- **Theme.** The page drew in a light theme (Chrome's color scheme), so the
  grey ramp and tints (N-066) can't be judged from it. Glyph shapes and cell
  widths can be.

## N-041: the Next List hover card (closed)

It appeared after the dwell, and closed on a click, on a key and on leaving
the rows. The warm window was instant across rows. On a long bulleted item it
kept the bullets with hanging indents, capped the body at 13 rows with `…`
for 15 in all, and was about 88 cells wide. The page asking for all motion
caused no lag. A throwaway test, deleted afterwards, confirmed a card on
every row for pane heights 24–60.

## N-033, N-039: looked at, left open

On the annotation bar the `ｉ` chip is an italic i in a slate box and reads as
a chip. The `│` rules read as faint dividers, and 🔷 is a flat slate diamond,
wider than ◆. Both items now say what was seen and what wasn't: the
list rows and the menu for N-033, and the bare tier for N-039. How they look
in the user's own theme is still for the user to judge.

## N-075: what a running pane's session is about (closed, `00e1594`)

Three claude panes in one project drew three identical picker rows. The row
now carries the pane's topic right after its state. `paneTopic`
(`context.go`) takes a custom pane name first, else the terminal title
Claude Code keeps for the conversation, with its leading status glyph cut
(✳ when idle, a spinner while working), and nothing for a bare "Claude
Code". Live: `[idle] “README.md line count report” · claude-haiku-… ·` and
`[idle] “Report current git branch” · …`. Tests: `TestPaneTopic`, and a
titled row in `TestRunningPaneRowShowsContextFill`. README updated.

## N-076: v0.42.2

The version was bumped in `main.go` and `cats-plugin.toml`, committed as
`chore(release): v0.42.2`, and the tag `v0.42.2` was annotated and pushed
with the code. It is a patch: every change is a fix or a refinement of
something that ships (the picker row gained words, not a new capability).
It carries `511feaf`, `5aa4480`, `11b250a`, `9ffbf76`, `a57c04a` and
`00e1594`.

## Next

Closed: N-041, N-075, N-076. Declined: None. Raised: N-076.
Deferred: None. Promoted: None.
Updated: N-033, N-039. Full list: `ai_docs/todo/next-list.md`.
