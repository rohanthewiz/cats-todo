# Session: scheduled drops, and the search box moves into the header

Session ID: `06e36eaa-a585-477c-bacf-3c9d36be5c65`
Date: 2026-08-03

Two asks in one line:

> "I would like some kind of scheduled drop. Also Look at how we can improve
> the interface — btw the search row doesn't need a row by itself, and really
> shouldn't be a first-class citizen. Todo list probably won't grow more than
> a half-a-page long anyway."

Planned in plan mode (two Explore agents, then two Plan agents — one per
feature), with the decisions settled up front via questions: auto-drop while
the TUI is open (no daemon), one-shot only, search merged into header line 0,
plus all four offered cleanups (tighter chrome, deduped footer, stable status
row, bounded header). Plan saved at `~/.claude/plans/sparkling-napping-kay.md`.

## Part 1 — the list view restructure

The old chrome spent six lines before the first todo: header, blank, query
row, blank, action bar, blank. The new line map:

```
0  headerRow      📝 Cats Todo v0.6.0  project + global  🔍 fix▏  1/3
1  (blank)
2  actionBarRow   ✚ Add  ✎ Edit  ➜ Send  ✖ Delete
3+ listRowsRow    todo rows
…  status row     ALWAYS rendered ("" when quiet) — the footer no longer jumps
   footer         one deduped help line (two full lines only when the bar is
                  too narrow for chip hints)
```

Mechanics, in order:

1. **`fuzzylist.go` split** — `view()` decomposed into `counts()` +
   `rowsView()`, recomposed byte-identically. The drop picker still renders
   through `view()` (boxed rails and all); the manager's list calls
   `rowsView()` and draws its own header. Gate: the whole suite green with
   zero test edits before anything else moved.
2. **`headerLayout()`** (ui.go) — one budget for the header's five tenants,
   with a fixed concession ladder when the pane narrows: ws label →
   done-hidden tag → search shrinks (34→12) → version → search floors (6),
   then drops → project name truncates *last*, keeping its `+ global`/`only`
   suffix. The match count is reserved at `total`'s digit width and never
   goes. `scopeNote()` now takes a `room` budget instead of reading `m.width`.
3. **Two real bugs fixed by the same engine**: `applySizes` used to size the
   query input to `width-4`, blowing the line to ~width+8 (count pushed
   off-screen, wrap threat to the click map); and a long project basename was
   never bounded. A measured constant `inputCursorPad = 1` records that a
   bubbles textinput renders one cell past its SetWidth (probed against
   charm.land/bubbles v2.1.1 in the scratchpad).
4. **Focus affordance without rails**: the 🔍 glyph renders in accent-bold
   (`promptStyle`) when the box holds the keys, faint (`searchGlyphOffStyle`)
   when a chip does. The input's own `Focused()` stays the single source of
   truth. Clicking the header line refocuses the box (`Y == headerRow` in
   `updateMouse`).
5. **Constants moved in lockstep**: `actionBarRow 4→2`, `listRowsRow 6→3`,
   new `headerRow = 0`. `targetRowsRow` untouched. The constant-pinned click
   tests (`TestActionBarRow`, `TestListRowsMatchWhatIsDrawn`,
   `TestListRowsAreClickable`) passed **unedited** against the new frame —
   which was the point of pinning them to constants in the first place.
6. **Footer dedup** — `barShowsHints()` extracted from `actionChips`; while
   the chips teach their own chords the footer names only the rest, conceding
   segments from the right rather than wrapping. Narrow panes keep the old
   two full lines.

New tests: `TestHeaderLineFits` (widths 20–200 × long-project/long-ws/
hidden-done), `TestHeaderSearchShowsFocus`, `TestStatusRowStable`,
`TestFooterDeduped`, `TestHeaderClickFocusesSearch`, `TestScopeNote` rewritten
for the room signature with a truncation regression case,
`TestViewComposesRowsView` pinning the fuzzylist split.

## Part 2 — scheduled drops

`ctrl+s` on a highlighted prompt → one-input time editor → the *same* target
picker in schedule flavor (`pickForSchedule` on the model; `chooseTarget`
branches to `commitSchedule` instead of firing; `backToList` always resets the
flag so an esc can't leak it into the next manual drop).

**Schema** (`store.go`): `Todo.Schedule *Schedule` with `omitempty` — an
unscheduled backlog's JSON is byte-identical to before. `Schedule{At, Kind
("pane"|"new"), Pane, Agent, Command, Cwd, Missed}`. Kind is a string so the
wire format survives enum reordering. Mode is *not* persisted: scheduled fires
are always drop-&-run, because an unattended paste has nobody to press enter.

**Parser** (`schedule.go`, pure): `15:30` / `9:05` (rolls to tomorrow if
past), `in 2h` / `2h` / `1h30m`, `tomorrow 9:00`, and `2006-01-02 15:04` (the
editor's prefill round-trips). An explicit past datetime errors rather than
rolling — the user spelled out a moment. `formatScheduleTime` gives clock
today / `Mon 15:04` in a week / `Jan 2 15:04` beyond.

**Firing** (`ui.go`): a 1 Hz `tea.Tick` armed in `Init` and re-armed on every
tick message, handled above the stage switch so it fires from any screen.
`fireDueSchedules` walks both stores; the interesting judgment calls:

- **Claim before fire** — `store.claimSchedule(id, at)` clears the schedule
  on disk (reload-mutate-save) before the drop goroutine launches, and a
  claim is only good for the exact `At` it read. That's the whole double-fire
  guard, including across two manager panes sharing the global backlog.
- **Grace window** (`scheduleGrace = 2min`) — a tick later than that (TUI was
  closed, laptop slept) marks the schedule `Missed` instead of firing:
  "opening the todo manager triggers surprise agent runs" is the failure mode
  the window exists to prevent. Same rule covers launch-with-past-schedule
  with zero special startup code.
- **Pane-gone policy** — `performScheduledDrop` re-verifies a pane target
  against `pane.list` at fire time and errors ("send manually") rather than
  falling back to a new session: picking a pane picked that conversation's
  context.
- A failed fire is written back as `Missed` via the extended
  `dropResultMsg{sched}`; success rides the existing path — `setDone`, which
  (like `toggle`) now also clears any schedule, keeping "schedule ⇒ open".
- `m.dropping` serializes against manual drops; a due schedule just waits for
  the next second, still inside grace.

**Row language**: badge `◷` in a new amber `schedStyle` (errStyle when
missed), desc prefixed `⏰ 15:04` / `⏰ missed 15:04` ahead of the 📎 count.
The prompt view's meta line says `· scheduled Thu 09:00`. `ctrl+s schedule`
joined both footer variants.

Tests: parse/format/round-trip tables (`schedule_test.go`),
claim-protocol + done-clears-schedule (`store_test.go`), and a `schedModel`
helper in `ui_test.go` backed by real temp files — the schedule paths reload
from disk before mutating, so the in-memory `withTodo` helper would be wiped
by the first `setSchedule`. Flow, guard, tick, missed, defer-while-dropping,
failed-fire and success-fire cases all drive `Update` with synthetic messages;
no sockets, no goroutines. Full suite green under `-race`.

## Known edges, accepted deliberately

- Firing lives in the running TUI's tick loop — no daemon, no cron. Closed
  manager ⇒ missed, shown on next launch.
- An older binary that saves the backlog drops the `schedule` key (the same
  documented limitation `images` carries).
- At ~80 cols the deduped footer concedes `esc quit` from the right; below
  the hint threshold the full two-line footer returns anyway.
- README's manager section documents both the inline filter and the schedule
  flow.
