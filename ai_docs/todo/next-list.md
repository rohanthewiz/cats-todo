# Next list — cats-todo

The project's living list of follow-ups. Sessions edit this file in place:
they add what they raise, move what they finish to Closed, and correct what
they find stale. They do not copy the list into their session doc. A session
doc's `## Next` says only `Closed: N-… Raised: N-…`, so each item has one
home, and an item that disappears shows up as a deletion in git history.

The cats-todo **Next List** page (`ctrl+g`, `nextlist.go`) reads the Open and
Roadmap sections of this file, so keep the item grammar below exact.

Seeded 2026-09-24 by `/next-list seed` from the 15 session docs
`2026-0904-1132-form-toolbar-to-the-top` … `2026-0924-1231-prompt-editor-autosave`.

## Conventions

- **IDs are permanent** (`N-001`, `N-002`, …) and never reused, even after
  an item closes.
- **`raised`** is the stem of the session doc the item first appeared in
  (`ai_docs/claude_sessions/<stem>.md`), looked up across all the docs, not
  just a recent window.
- **Age is computed, never stored.** It is the number of session docs written
  since `raised`, so an item raised in the newest doc has age 0.
- **Value** is the payoff of doing the item, not the effort:
  - `high`: something is worked around today, or a second independent
    consumer has arrived;
  - `medium`: it blocks one named thing, or it is a visible defect nobody has
    to route around yet;
  - `low`: a gap nobody has bumped into, or it depends on something that does
    not exist yet.
- **Open** is what we intend to pick up next. **Roadmap** is wanted, but
  later: parked, not declined. **Non-goals** are what we are likely never to
  do, kept here so they stay visibly declined.
- **Nothing leaves Open or Roadmap without a line in another section.** A
  finished item moves to Closed with its date and what showed it. A declined
  one moves to Non-goals with its reason. A duplicate moves to Closed as
  `merged into N-xxx`. Moving between Open and Roadmap is fine.
- **Open and Roadmap stay in ID order.** Sorting is done by `/next-list` when
  it prints a view, never in this file.
- Item grammar: `- **N-###** · raised \`<stem>\` · value <v>` at column 0, then
  the text indented two spaces on the lines below.

**Next ID:** N-060

## Open

- **N-005** · raised `2026-0912-2110-multi-caret-enter-paste-tab-indent` · value low
  Add a test for the Cmd+V chord's *local pasteboard* road into the column
  mode's carets (`pasteFormClipboard` → `pasteFormClipboardInMode` →
  `pasteLinesAtCarets`). `readClipboardText` (`clipboard.go`) is a swappable
  `var` and `stubClipboard` (`promptsel_test.go`) already stubs it, but no
  test stubs the clipboard with the column mode on. The OSC 52 road is
  covered. *Lapsed* in `2026-0922-0943-info-annotation`.

- **N-007** · raised `2026-0912-2110-multi-caret-enter-paste-tab-indent` · value low
  The column-mode footer is over 120 cells, so narrow panes lose its tail
  (`←/→ moves them`, `ctrl+a/e line ends`). Acceptable for now; revisit if
  the mode gains another key. *Lapsed* in
  `2026-0922-0943-info-annotation`.

- **N-011** · raised `2026-0912-2234-carried-indent-and-release` · value low
  Enter with the caret *inside* an indent (`"  |  x"`) leaves the spaces left
  of the caret as trailing whitespace on the upper line (`"  "` / `"    x"`).
  `promptCarriedIndent` (`promptindent.go`) moves the indent down only when
  the caret is at the end of a blank row. Decide whether the upper line
  should be trimmed too. *Lapsed* in
  `2026-0915-1836-prompt-editor-undo-v0.32.0`.

- **N-012** · raised `2026-0912-2234-carried-indent-and-release` · value low
  Consider back-tagging the releases that have a `chore(release)` commit but
  no tag: v0.14.0, v0.21.1, v0.30.0, v0.30.1 and v0.30.2 (checked
  2026-09-24). Not asked for; raise it with the user before doing it. *Lapsed* in
  `2026-0922-0943-info-annotation`.

- **N-017** · raised `2026-0913-1932-existing-pane-session-settings` · value medium
  Live-test an existing-pane drop into a real claude pane with model and
  effort set, in both run and paste mode. Confirm that `clearSettle` (400ms)
  is long enough after `/model` and `/effort`, not just after `/clear`. The
  new-session path needed 2s (`newSessionSettle`). *Lapsed* in
  `2026-0922-0943-info-annotation`.

- **N-023** · raised `2026-0915-1637-prompt-editor-paste-line-cap` · value low
  Hand-test in cats: paste a few hundred lines, then type, press enter,
  scroll, click and sweep. Watch whether the caret stays in view on the long
  prompt. *Lapsed* in `2026-0915-1836-prompt-editor-undo-v0.32.0`.

- **N-025** · raised `2026-0915-1637-prompt-editor-paste-line-cap` · value low
  Pastes past the library's 10000-line `maxLines` are still truncated
  silently (bubbles `textarea.go` `maxLines = 10000`). Contract 4 ("refuse in
  words") suggests a status note if that is ever hit. *Lapsed* in
  `2026-0915-1836-prompt-editor-undo-v0.32.0`.

- **N-026** · raised `2026-0915-1836-prompt-editor-undo-v0.32.0` · value medium
  Hand-test undo in a cats pane: that Cmd+z actually arrives (cats forwards
  Cmd), the menu row, and undo after a `@` insert and after a spelling
  correction, the two cross-stage paths only the commit point covers. Since
  `2026-0924-2007-redo-and-switch-confirm`, redo too: that shift+cmd+z
  arrives as one of the bound spellings, and ctrl+y where Cmd is eaten.
  *Lapsed* in `2026-0922-0943-info-annotation`.

- **N-028** · raised `2026-0915-1836-prompt-editor-undo-v0.32.0` · value low
  Undo does not cover the title field. It is a one-line `textinput` with no
  history of its own; if it is ever wanted, it is a second stack, not the
  prompt's. *Lapsed* in `2026-0922-0943-info-annotation`.

- **N-033** · raised `2026-0924-1146-info-mark-blue-chip` · value low
  Check the info chip by eye. Look at an info row in cats, both plain and
  highlighted, plus the bar and the menu. If the fullwidth `ｉ` looks thin or
  odd in the fallback font, the fallbacks are a plain `i` padded inside the
  chip, or the `ℹ️` emoji (blue square, but its "i" isn't italic).

- **N-034** · raised `2026-0924-1231-prompt-editor-autosave` · value medium
  Try autosave in a live cats pane. Only the tests have exercised it so far.
  Check that the `autosaved HH:MM` note is readable, that it appears while
  typing without moving the caret, and that esc after an autosaved add
  leaves no row behind.

- **N-035** · raised `2026-0924-1231-prompt-editor-autosave` · value low
  Change the autosave delay from inside the app (optional). Today the delay
  is read only from settings.json (`autosaveSeconds`) at launch. The list's
  View panel could offer it if hand-editing turns out to be a nuisance.

- **N-037** · raised `2026-0924-1802-code-highlight-and-next-list-seed` · value low
  The comment in `viewContent` (`ui.go`) says a pre-styled span "loses its
  reset at the wrap points". lipgloss v2.0.5 closes a style at each break and
  opens it again on the next line, which the code highlighting now relies on
  (`TestStyleCodeSpansSurvivesWrap` pins it). Correct the comment, and decide
  whether the session and attachment lines still need to stay plain.

- **N-038** · raised `2026-0924-1924-nextlist-send-value-levels-v0.36.0` · value medium
  Live-test the Next List's ✉ Send (`shift+enter`, `nextlist.go`
  `sendFromNext`) in cats: a new session, a worktree session and a running
  pane, in both run and paste mode. Check that esc from the picker lands back
  on the page with the highlight kept, that the heading shows
  `N-0xx dropped → …`, and that a new session opens in the list's project.
  Only the tests have exercised it.

- **N-039** · raised `2026-0924-1924-nextlist-send-value-levels-v0.36.0` · value low
  Check the value marks and the annotation bar by eye in cats: that 🔷 draws
  two cells in the cats font, that the faint `│` rules read as dividers, and
  that the bare tier's reverse-lit radio is visible. At 100 cells the bar keeps
  the Value/Priority labels and drops the checkbox words (the words need 104);
  swap the two tiers if the words turn out to matter more.

- **N-040** · raised `2026-0924-1924-nextlist-send-value-levels-v0.36.0` · value low
  Decide whether a Next List send should leave a done copy in the backlog as a
  record of what was sent and when. Today it writes nothing (the item already
  lives in the file); the form's ✉ Send, by contrast, saves then drops. Raised
  to the user on 2026-09-24, not yet answered. Since the context menu
  (`nextmenu.go`) a record can be kept deliberately: ⤓ Add to backlog, then
  send from the list.

- **N-041** · raised `2026-0924-1939-nextlist-hover-card` · value medium
  Live-test the Next List hover card (`nexthover.go`) in cats: the 400ms
  dwell and the warm window across rows, that the card closes on a key, a
  click and on leaving the rows, and that the 62-cell box and its 7-row cap
  read well on real items (sub-bullets kept, `…` on long ones). Also check
  that asking for all motion on the page causes no lag. Only the tests have
  exercised it.

- **N-043** · raised `2026-0924-2007-redo-and-switch-confirm` · value medium
  Live-test the switch confirm (`panesetup.go`) in cats. Drop a prompt with
  a different model set and Clear off into a claude pane that answered
  within the last few minutes. Check that the *Switch model?* dialog is
  seen, answered, and the prompt arrives whole, with the status line's
  `confirmed the model switch`. Then check the same with only the effort
  changed, and with a PreModelSwitch hook that asks. Only a scripted fake
  pane has exercised it.

- **N-044** · raised `2026-0924-2007-redo-and-switch-confirm` · value low
  Check whether `/model fable` can raise its usage-credits consent dialog on
  an existing pane (Claude Code 2.1.282 has one: `F3t`/`T5`, "uses usage
  credits"). If so it would eat the prompt the way the switch confirm did,
  and `applyPaneSetup` doesn't watch for it. A pane that has consented once
  won't show it.

- **N-045** · raised `2026-0924-2029-notes-send-to-gonotes` · value low
  `TestProgramExitsOnHangup/sighup` fails under `go test -race`: the helper
  process exits 66, the race detector's code, so `runProgram` races with
  itself on SIGHUP. It fails on a clean HEAD as well as with the notes send,
  and passes without `-race`. The race report goes to the helper's pty, so
  capturing it means teeing the drained pty bytes in the test.

- **N-046** · raised `2026-0925-1104-nextlist-context-menu` · value medium
  Live-test the Next List context menu (`nextmenu.go`) in cats. Check the
  right-click on an item, both ⧉ Copy rows (OSC 52 and `pbcopy`), that
  ⚙ Session… and ◫ Images… open over the draft with esc landing on it, that
  ◷ Schedule… adds the item and lands in the list's scheduler, and that the
  ⤓ Add rows grey out once an open copy is in the backlog. Also check that
  the 15-line box fits, or flips above the pointer, in a short pane. Only the
  tests have exercised it.

- **N-047** · raised `2026-0925-1104-nextlist-context-menu` · value low
  The Next List rows don't show which items are already in the backlog; only
  the menu's greyed ⤓ Add rows reveal it. A mark on the row (from
  `nextBacklogCopy`) would show it at a glance. The copy is recognised by
  the citation line (`nextItemCite`), so a prompt whose first line was
  rewritten in the form isn't recognised as a copy.

- **N-048** · raised `2026-0925-1315-multi-drop-batches-phase1` · value medium
  Live-test batches in cats. Check an all-at-once batch onto new worktrees
  (the tabs open one after another, each prompt lands whole, and each is
  marked done), a one-prompt-listed batch into a running pane, and a
  failure part-way (the rest still go, and the record shows ✗ with the
  error). Also check the composer's drag and clicks in both layouts (100
  columns or more, and narrower), Next List items becoming backlog prompts,
  and ⧉ Duplicate after a partial failure. Only the tests have exercised it.

- **N-052** · raised `2026-0925-1315-multi-drop-batches-phase1` · value low
  `gofmt -l` lists `promptcode.go` and `ui.go` on a clean HEAD (in `ui.go`,
  the form-stage field block's alignment). One `gofmt -w` commit on its own
  would keep that noise out of feature diffs.

- **N-053** · raised `2026-0925-1344-batch-scheduling-phase2` · value medium
  Live-test scheduled batches in cats. Schedule one `in 2m` onto new
  worktrees and watch it fire (the status line, each prompt marked done, the
  record's Scheduled and Dropped times). Check the `⧉ HH:MM` mark on the list
  rows and that it goes on ✕ Unschedule. Close the manager past a batch's
  time and reopen it: the batch should read *missed* with the reason, and
  enter on it should open the composer. Also schedule into a running pane,
  close that pane, and let it fire: each step should fail with "the
  scheduled pane is gone". Only the tests have exercised it.

- **N-055** · raised `2026-0925-1344-batch-scheduling-phase2` · value low
  The composer's footer loses its tail below about 110 columns, and at 80
  the bar has gone to bare chips (`◷ Schedule`, `☰ Batches`, `✕ Cancel`
  without chords) while the footer no longer names `ctrl+s`, `ctrl+k` or
  `esc`. Contract 6 says the footer should then name what the chips stopped
  teaching. `batchFooterSegs` could put the button chords ahead of the
  region's keys once `batchBarTier` drops the hints.

- **N-056** · raised `2026-0925-1409-batch-loop-phase3` · value medium
  Live-test batch loops in cats. Only the tests have driven one, with made-up
  pane states. Check:
  - a same-session loop of three into a new session: each prompt waits for
    the last, and the status line walks `on 1/3` → `2/3 sent`;
  - that cats reports *working* soon enough, and for long enough, for the
    1-second poll to see it (`loopStartWait` is 45s, the between grace 3s),
    with a quick prompt as well as a long one;
  - `/compact` and `/clear` as the between command;
  - fresh each onto worktrees;
  - a permission question mid-prompt (*blocked*) holding the loop;
  - quitting the manager mid-loop and reopening it (`‖ paused`, then
    resumed), and a loop left over an hour (stopped, "not resumed");
  - a scheduled drop (a prompt or a batch) into a claude pane whose agent is
    quit before it fires: marked missed, "the scheduled pane's agent has
    exited", and nothing typed at the shell (N-020);
  - ■ Stop from the page.

- **N-058** · raised `2026-0925-1739-batches-menu-and-loop-default` · value medium
  Live-test the Batches page's right-click menu (`batchmenu.go`) in cats.
  Check the right-click on a plan (✎ Edit… opens the composer) and on a
  record (☰ Open record), ■ Stop on a running loop, and the two-press
  Delete. After the first press, the heading still says "press ctrl+x
  again". Decide whether a mouse user needs it to name the menu's
  ✖ Confirm delete too. Also check that the box stays placed while a running
  batch's progress re-sorts the rows under it. Only the tests have
  exercised it.

- **N-059** · raised `2026-0925-1739-batches-menu-and-loop-default` · value medium
  Release v0.40.0: the Batches page's right-click menu (a new capability, so
  minor), loop as the composer's default Deliver, and the loop's one-hour
  resume window (N-057), the fire-time agent check on a scheduled pane
  drop (N-020), and the caret walk that made enter on a long one-line
  prompt slow (N-024). Bump `main.go` and
  `cats-plugin.toml`, commit `chore(release): v0.40.0`, tag it, and push the
  code and the tag.

## Roadmap

Wanted, but not now: parked until something they wait on arrives, not
declined.

- **N-021** · raised `2026-0913-1932-existing-pane-session-settings` · value low
  Apply a todo's permission mode to a running pane, if Claude Code gains a
  set-mode command. Until then, not applying it is deliberate (see N-022).
  *Lapsed* in `2026-0915-1637-prompt-editor-paste-line-cap`.

## Non-goals

- **N-006** · declined `2026-0912-2234-carried-indent-and-release` — Fixed tab
  stops, both for `tab` at a caret (raised in
  `2026-0912-2110-multi-caret-enter-paste-tab-indent` and built in
  `2026-0912-2125-tab-stops-and-skill-fixes`) and for rounding swept lines.
  The user chose carried indents instead.
- **N-008** · declined `2026-0912-2110-multi-caret-enter-paste-tab-indent` —
  Literal `\t` characters in the prompt. They are blocked by the textarea's
  private sanitizer, cell-width rendering, and delivery as keystrokes to
  Claude Code.
- **N-009** · declined `2026-0912-2110-multi-caret-enter-paste-tab-indent` — A
  keyboard road *out of* the prompt field. The user chose `tab`/`shift+tab`
  as indent/outdent there, with a click as the way out.
- **N-015** · declined `2026-0913-1813-done-stamp-v0.31.0` — Preserving
  `doneAt` when a binary older than v0.31.0 saves a backlog. It drops the
  key, the same accepted limitation as `images`/`schedule`. Nothing to fix,
  but worth knowing if stamps "disappear" on a machine running an old build.
- **N-016** · declined `2026-0913-1813-done-stamp-v0.31.0` — Sorting the done
  group by `doneAt`. Array order stays the user's order, and the stamp is a
  record only.
- **N-022** · declined `2026-0913-1932-existing-pane-session-settings` —
  Setting permission mode on a running pane with `shift+tab` or `/plan`.
  `shift+tab` cycles from an unknown starting mode, and `/plan` opens the
  plan view on a pane already in plan mode. See N-021 for the version that
  waits on Claude Code.

## Closed

Closures from before this file was seeded live in the session docs. The ones
below were found done or overtaken while seeding.

- **N-024** · closed 2026-09-25, `2026-0925-1802-long-line-caret-hop` · raised `2026-0915-1637-prompt-editor-paste-line-cap`
  — The premise was off: `SetValue` was cheap. The time went to
  `setPromptCaretOffset`, which walked to the caret's row one *display* line
  at a time. Each step re-hashed the whole row in the library's wrap memo, so
  the cost was quadratic in a long row's length. It now hops a logical row
  per step (`CursorEnd`, then `CursorDown`), keeping the display-line walk as
  a backstop. Enter on a 20k-rune line went from ~140ms to ~1.5ms. Undo/redo,
  indent, line moves and the column mode use the same walk and got faster
  too.
- **N-020** · closed 2026-09-25, `2026-0925-1757-scheduled-drop-agent-gate` · raised `2026-0913-1932-existing-pane-session-settings`
  — A scheduled existing-pane drop now checks, when it fires, that the pane
  still runs an agent and not just that it still exists
  (`scheduledPaneAgent`, `drop.go`). It uses the same `isDropAgent` rule the
  picker applies. A pane whose agent has exited is marked Missed with
  "the scheduled pane's agent has exited — send manually". The live agent
  label replaces the one saved with the schedule, so `/model` and `/effort`
  are gated on what runs at fire time. Scheduled batches get the same check.
  `paneSetupCommands` now also withholds `/clear` when no agent is detected,
  as a backstop for any caller that builds a target by hand.
- **N-057** · closed 2026-09-25, `2026-0925-1749-loop-resume-window` · raised `2026-0925-1409-batch-loop-phase3`
  — A resume window: a loop whose heartbeat is over an hour old
  (`loopResumeWindow`) is stopped with the reason instead of resumed
  (`loopLapsed`, `expireLoop` in `adoptLoops`), in a manager outside cats too.
  The driving manager's own beat lapsing (a laptop asleep with it open) ends
  it the same way on its first tick back, so which manager wakes first never
  changes the outcome; a pane.list answer in flight across the sleep is not
  judged. The paused record view says until when it can still be picked up.
  ⧉ Duplicate goes on.
- **N-054** · closed 2026-09-25, `2026-0925-1739-batches-menu-and-loop-default` · raised `2026-0925-1344-batch-scheduling-phase2`
  — The Batches page's right-click menu (`batchmenu.go`, on `menuBox`): the
  page's row actions on the batch pointed at — ✎ Edit… (a plan) / ☰ Open
  record, ⧉ Duplicate…, ✕ Unschedule / ■ Stop (a running loop), ✖ Delete
  record… with the page's two presses (the second reads ✖ Confirm delete).
  ＋ New and ← Back stay on the bar. The menu carries the batch's ID and
  re-reads the rows before a press, because a running batch's progress
  re-sorts them under an open menu. Its refusals are the chords' own words
  (`unscheduleWhy`, `deleteWhy`, now shared).
- **N-051** · closed 2026-09-25, `2026-0925-1739-batches-menu-and-loop-default` · raised `2026-0925-1315-multi-drop-batches-phase1`
  — Found done: `dc7cce5 chore(release): v0.39.0` and tag `v0.39.0` are on
  origin. It shipped batches phases 1–3 and the hover card's done stamp.
- **N-014** · closed 2026-09-25, `2026-0925-1448-hover-card-done-stamp` · raised `2026-0913-1813-done-stamp-v0.31.0`
  — Decided yes. The compact form on the row was not enough, because it is
  the row's last mark and the first thing a narrow pane cuts off (a titleless
  prompt's name alone runs to 60 cells). The card now leads its fields with
  `Done    2026-09-13 18:13 CDT`, the prompt view's full stamp. There is no
  row when there is no stamp, and a stamped title-only done todo now earns a
  card. The format moved into `formatDoneStamp` (`schedule.go`), which the
  view, the card and the bundle share.
- **N-050** · closed 2026-09-25, `2026-0925-1409-batch-loop-phase3` · raised `2026-0925-1315-multi-drop-batches-phase1`
  — Batches phase 3, the loop (`batchloop.go`). One prompt at a time, the next
  when the pane goes working → idle (judged per poll by the pure
  `loopRunner.judge`; blocked counts as working). Same session or fresh each;
  the between command (3s with no *working* = instant; after the last too);
  pause, max wait, on fail stop or skip. `Next` written before each send; the
  record carries its driver (pid + heartbeat) and a manager reopened mid-loop
  takes it over by swap and resumes. ■ Stop on the page. The dropping guard is
  held only while typing. `performDropAt` returns the pane and branch, which
  `BatchRun` now records. The plan's "Phase 3 as built" lists the departures.

- **N-049** · closed 2026-09-25, `2026-0925-1344-batch-scheduling-phase2` · raised `2026-0925-1315-multi-drop-batches-phase1`
  — Batches phase 2, scheduling. The composer's When row (empty = now,
  otherwise `parseScheduleTime`, with the time it comes to shown beside it)
  and ◷ Schedule (`ctrl+s`), the one that doesn't apply greyed and refusing in
  words. `fireDueBatches` on the schedule tick, after `fireDueSchedules`:
  grace, *missed* with a reason, claim by compare-and-swap (`swapBatch`), the
  backlogs re-read at fire time, a running-pane target re-checked. Enter on a
  scheduled, missed or unscheduled batch edits it in the composer, saved back
  under the same swap; ✕ Unschedule (`ctrl+u`) keeps it as a plan. List rows
  wear `⧉ HH:MM` while their prompt sits in a scheduled batch. The plan's
  "Phase 2 as built" section lists where it departs from the design.
- **N-042** · closed 2026-09-24 · raised `2026-0924-1939-nextlist-hover-card`
  — Released v0.37.0 (tag `v0.37.0`). It ships the notes drop target, redo,
  the Next List hover card, the switch-confirm fix, the footer's trimmed
  tail, and the bundle's done stamp.
- **N-030** · closed 2026-09-24, `2026-0924-2029-notes-send-to-gonotes` · raised `2026-0922-0943-info-annotation`
  — An info prompt's Send (shift+enter, the form's ✉ Send, the menu row that
  now reads ✉ Send to notes) files it in the notes plugin rather than
  refusing. `notes.go` finds the pane by `plugin_type == "notes_mgr"` in
  `pane.list`, preferring this workspace, and pastes a `<!-- cats-note v1 -->`
  envelope (JSON-quoted YAML frontmatter, then the prompt) with Submit off,
  then focuses the pane. The result rides `dropResultMsg{toNotes}`, so it is
  marked done like a drop. With no notes pane open, the refusal names both ways
  out. The intake contract was settled with the user: a paste into the pane
  rather than an inbox dir or a CLI, because the pane's process already owns
  the store. gonotes `08eb409` is the receiver (`tui/intake.go`), which opens
  an unsaved form. Schedule still refuses info prompts. Shipped in v0.37.0.
- **N-013** · closed 2026-09-24, `2026-0924-2019-bundle-done-stamp` · raised `2026-0913-1813-done-stamp-v0.31.0`
  — `bundleTodoNote` (`bundle.go`) now writes `done 2026-09-24 14:05 CDT`,
  the prompt view's full stamp with the zone, and plain `done` when there is
  no stamp. It also fixes a bug found along the way: since `5e14beb` every
  prompt without a priority rendered as "none priority", because the note
  tested `priorityLabel`'s output rather than the level. Shipped in v0.37.0.
- **N-029** · closed 2026-09-24, `2026-0924-2013-form-footer-tail` · raised `2026-0915-1836-prompt-editor-undo-v0.32.0`
  — The form footer's tail is cut to what nothing else teaches: `ctrl+l
  spelling · right-click menu · alt+↑/↓ move line · cmd+d dup line`. The
  prompt library, undo and redo left it, because their right-click menu rows
  print their chords. `cmd+d` is named only under `kbEnhanced`. The full line
  went from 244 cells to 207 (`TestFormFooterTailFitsAWidePane`). Shipped in
  v0.37.0.
- **N-018** · closed 2026-09-24, `2026-0924-2007-redo-and-switch-confirm` · raised `2026-0913-1932-existing-pane-session-settings`
  — `/model` mid-conversation does ask. Claude Code 2.1.282 raises *Switch
  model?* / *Change effort level?* when the cache is warm and the switch
  changes something, and the next Enter answers it, so a Clear-off drop's
  prompt was lost. `applyPaneSetup` (`panesetup.go`) now watches for the
  dialog after each command and presses Yes. The status line says so, and a
  PreModelSwitch hook's ask stops the drop instead (`c68084d`). Shipped in
  v0.37.0.
- **N-019** · closed 2026-09-24, `2026-0924-2007-redo-and-switch-confirm` · raised `2026-0913-1932-existing-pane-session-settings`
  — Premise corrected: in Claude Code 2.1.282, `/effort xhigh` on a model
  without xhigh no longer errors. It is accepted, and the request runs at high
  (`max` likewise). A model-aware check was declined, because the capability
  table is Claude Code's, partly served at runtime, and a copy would go
  stale. The README's session options section says what happens.
- **N-027** · closed 2026-09-24, `2026-0924-2007-redo-and-switch-confirm` · raised `2026-0915-1836-prompt-editor-undo-v0.32.0`
  — Redo. `promptUndo.redo` (`promptundo.go`): undo moves the state it
  leaves onto it, `redoPrompt` moves it back, and the next change to the text
  clears it (a caret motion does not). The chords are `shift+cmd+z`,
  `ctrl+shift+z` and `ctrl+y`, and ↷ Redo sits under ↶ Undo on the context menu
  (`af86f25`). Shipped in v0.37.0.
- **N-036** · closed 2026-09-24, `2026-0924-1924-nextlist-send-value-levels-v0.36.0` · raised `2026-0924-1802-code-highlight-and-next-list-seed`
  — Release the code highlighting. Shipped in v0.36.0 (tag `v0.36.0`),
  together with the Next List send and the value levels.
- **N-032** · closed 2026-09-24 · raised `2026-0922-1601-ced-not-a-drop-target`
  — Release the ced fix. Shipped as v0.33.1 (`99b6e01`, tagged).
- **N-031** · closed 2026-09-24 · raised `2026-0922-1601-ced-not-a-drop-target`
  — Editor flag on the wire (`Editor bool` on `PaneMeta`) so cats-todo can
  follow the user's `editor.agents`. Overtaken by cats' `plugin_type`, read
  since v0.33.1 (`636999a`): `isDropAgent` (`context.go`) trusts it, and
  `editorAgents` survives only as a fallback for a cats older than
  `plugin_type`.
- **N-010** · closed 2026-09-24 · raised `2026-0912-2125-tab-stops-and-skill-fixes`
  — Cut v0.30.3. Tag `v0.30.3` exists.
- **N-004** · closed 2026-09-24 · raised `2026-0912-2110-multi-caret-enter-paste-tab-indent`
  — Settle the versioning rule (minor for a feature, patch for a fix or a
  refinement). Done in `2026-0912-2125-tab-stops-and-skill-fixes`: the dev
  skill now states the practice, with precedents.
- **N-003** · closed 2026-09-24 · raised `2026-0912-2110-multi-caret-enter-paste-tab-indent`
  — The dev skill said `.claude/` is gitignored, but `.claude/skills/` is
  tracked. Corrected in `2026-0912-2125-tab-stops-and-skill-fixes`.
- **N-002** · closed 2026-09-24 · raised `2026-0904-1933-resyncing-the-socket-envelope`
  — `internal/integration` (the `CATS_PANE_ID` sliver) not yet examined
  against cats. Checked while seeding: its one constant, `CatsPaneIDEnvVar =
  "CATS_PANE_ID"`, matches cats' `internal/integration/integration.go`.
- **N-001** · closed 2026-09-24 · raised `2026-0904-1933-resyncing-the-socket-envelope`
  — Fold the `SocketNone` fix into the next release's notes rather than
  cutting a release for it. Overtaken: every release since, from v0.30.0 on,
  includes it.
