# Session: live tests driven through catctl probe (N-023, N-026)

Session ID: 48536950-83c8-462f-92e4-5896c04d09cb
Date: 2026-09-27
Mode: unattended backlog run, second pair (after
`2026-0927-0032-drop-settings-side-effect-v0.42.1`).

## The rig

`catctl probe` attaches to catway's browser WebSocket and sends the same
`key`/`paste`/`mouse` messages the cats page sends. It then dumps or captures
pane grids. The setup:

- this session's catway is the Cats.app child on `127.0.0.1:8422` (four
  other catways on the machine belong to other checkouts' scratchpads);
- a scratch git repo and a `ct-live` workspace rooted there;
- cats-todo v0.42.1 launched by `tab.create` through `/usr/bin/env
  CATS_TODO_CONFIG_DIR=<scratch>/cfg`, so no real backlog or settings file
  was touched;
- two scratchpad helpers: `pr.sh` runs a probe script and strips the frame
  chatter, and `cap.sh` decodes a `capture:…:ansi` result and marks
  reverse-video cells as `[x]`, which is how the drawn caret shows up in
  plain text.

The recipe, including the row-offset and redraw-lag gotchas, is now in the
dev skill under *Live tests without a person*.

## N-023: the caret on a long prompt (closed)

With 300 pasted lines the caret stayed in view after typing at the end,
enter, PageUp ×3 (it rode the top row), a click on a middle row (the typed
`C` landed at the click), and a shift+↓ sweep of 35 rows past the bottom
(the view followed). The wheel does nothing over the prompt. The form has
never handled wheel events, so this was not a regression.

Speed was the one finding. Arrow keys cost ~7ms each at 30 lines, ~20ms at
300 and ~50ms at 1,000 (live, end to end). A throwaway in-process timing
test showed 1,000 lines at ~13ms in Update + ~10ms in View, and the CPU
profile put nearly all of it in bubbles' textarea v2.1.1. Both its `Update`
(`textarea.go:1329`) and its `View` (`:1455`) render every line into the
viewport, and the viewport then slices it. Raised as N-071. The throwaway
test was deleted.

## N-026: undo and redo in a cats pane (closed)

cmd+z (probe mods `m`) undid by word, and shift+cmd+z, ctrl+y, ctrl+z and
ctrl+shift+z all worked. The right-click menu's ↶ Undo / ↷ Redo rows worked
on click. Undo after an `@` file insert returned to the typed `@`. Undo after
a ctrl+l spelling fix (`teh` → `the`) returned to `teh`, and redo restored
both. In cats' page, ⌘Z/⌘⇧Z fall through to kitty panes
(`20-keys.js:214`). In Cats.app, cats' own session
`2026-0727-1706-cmdz-undo-forwarding-and-esc-leader-chaining` found that the
Edit menu's Undo/Redo key equivalents do not swallow them.

## On the side

- N-034 in part: esc after an autosaved add left no row, and the status line
  read `cancelled · autosaved changes taken back`. The rest of N-034 is next.

## Next

Closed: N-023, N-026. Declined: None. Raised: N-071.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
