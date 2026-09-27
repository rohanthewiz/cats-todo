# Session: unattended backlog run, summary and close-out

Session ID: 48536950-83c8-462f-92e4-5896c04d09cb
Date: 2026-09-27
Mode: the closing wrap of an unattended run that started at N-017, with a
commit per item and a sess-wrap every two. This doc indexes the run and
records the work done after the last pair's doc.

## The run's docs, in order

1. `2026-0927-0032-drop-settings-side-effect-v0.42.1`: N-017 stopped (a
   drop's `/model` saves the user's default, N-070), and v0.42.1 (N-069).
2. `2026-0927-0037-probe-driven-live-tests`: the `catctl probe` rig, N-023
   and N-026, and N-071 raised.
3. `2026-0927-0053-live-autosave-and-next-send`: N-034 and N-038, and N-072
   raised (drops arrive as pasted content).
4. `2026-0927-0058-next-list-marks-and-menu-live`: N-062 and N-046, and the
   fix for esc on a Next List draft.
5. `2026-0927-0104-picker-fold-and-double-click-live`: N-060 and N-064.
6. `2026-0927-0111-batches-menu-and-batch-runs-live`: N-058 (the armed-delete
   note) and N-048, and N-073 to N-075 raised.
7. `2026-0927-0127-scheduled-batches-and-loops-live`: N-053 and N-056 (the
   stale record screen).
8. `2026-0927-0138-loop-view-and-gone-target`: fixes for N-073 and N-074.
9. `2026-0927-0145-hover-card-live-and-release`: N-041, N-075 and v0.42.2
   (N-076).

## After the last pair

- Every open item that waits on the user now says **Needs the user** in
  `next-list.md`: N-012, N-017, N-033, N-039, N-043, N-044, N-066, N-070,
  N-071 and N-072 (`742eb51`).
- Clean-up: the test workspaces `ct-live` and `ct-other` were closed, which
  brought the session back to the user's 21 workspaces. The scratch
  projects under `.cats-todo/` were removed, and there are no test worktrees
  or `todo/*` branches left. The pasteboard is empty again, as it was
  before the Copy tests.
- The dev skill's live-test section gained the Chrome recipe for pointer
  motion (cats' page at `?ws=<id>`, the pointer offset, the light theme)
  and the note that a drop moves the view (`ca1fca7`).

## Still waiting on the user, outside the repo

`~/.claude/settings.json` holds `"model": "haiku"` and a
`modelSettings.claude-haiku-4-5` entry, both written by the first `/model
haiku` probe of N-017. The session was not allowed to edit that file. From
`~/.claude/history.jsonl`, the user's last choices were `/model opus` and
`/effort high`.

## Next

Closed: None. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: N-012, N-033, N-039, N-043, N-044, N-066, N-071 (marked Needs the
user). Full list: `ai_docs/todo/next-list.md`.
