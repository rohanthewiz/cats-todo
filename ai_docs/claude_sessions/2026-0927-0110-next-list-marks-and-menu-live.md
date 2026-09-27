# Session: the Next List's marks and context menu, live (N-062, N-046)

Session ID: 48536950-83c8-462f-92e4-5896c04d09cb
Date: 2026-09-27
Mode: unattended backlog run, fourth pair (after
`2026-0927-0100-live-autosave-and-next-send`).

## N-062: the backlog record and the ⤓ mark (closed)

- A successful send leaves a done copy with the heading `… · recorded done in
  the project backlog`; this was checked last pair. Two cases wrote nothing:
  an esc from the picker, and a send into a pane closed under the open
  picker. The second read `send failed: cats error: unknown pane 332`.
- ⤓ Add from the menu put the green ⤓ (`#4db380`) on the row at once.
- Straight text column: the probe's `read` op reads grid cells, not a dump
  string, so a 🔷's second cell is not lost. It found the text starting on
  cell 13 for `N-001 ◆ ⤓`, a bare `N-002` and `N-003 🔷 ⤓`.

## N-046: the context menu (closed, one fix)

Copy ID and Copy as prompt both reached the pasteboard (it was empty before,
and was left holding the prompt). ⚙ Session… and ◫ Images… opened over the
draft, and esc landed on the draft. ◷ Schedule… saved the item and opened the
scheduler. The Add rows grey out for an item with an open copy. The box
fits below, flips above for a low row, and pins to the top in a pane too
short for either.

### The fix: esc on a Next List draft goes back to the page (`511feaf`)

The second esc, the one that throws the draft away, landed on the prompt list.
From the menu that meant ⚙ Session… → esc → esc took the user off the page.
It also contradicted the README, which calls ◷ Schedule… "the one row that
leaves the page", and the page's other esc road: the send picker's esc keeps
the page and the highlight (`leaveTarget`).

`cancelForm` (`ui.go`) now reads `formNextID` before `backToList` forgets it.
For a draft from the page it returns to `stageNextList`, resizes, and re-reads
the ⤓ set, because the revert may have just deleted an autosaved copy of the
very item. A plain add or edit still goes to the list. Test:
`TestNextListDraftEscReturnsToThePage`, which fails on the old code with
"landed on stage 0". The README's Next List section says where esc lands.

✔ Save on such a draft still lands on the prompt list, with the new row
highlighted. That is a completion rather than a change of mind, and the
session left it alone.

## Rig notes

- A drop's `agent.focus` moves the view to the target's tab, and the probe
  (a `--workspace` view) then no longer sees the manager's pane
  (`no pane N`). `catctl agent.focus --params '{"pane":<manager>}'` brings
  it back.
- esc on the prompt list quits the manager. Relaunch it with `tab.create`.
- Twenty filler items in the scratch next-list gave a row low enough to
  show the menu flipping.

## Next

Closed: N-046, N-062. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
