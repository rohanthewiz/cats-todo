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

**Next ID:** N-073

## Open

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
  **Needs the user** (2026-09-27, `2026-0927-0032-drop-settings-side-effect-v0.42.1`): the
  live run was stopped after one manual `/model haiku` + `/effort low`,
  because in Claude Code 2.1.283 each of them is **saved as the default for
  new sessions** in `~/.claude/settings.json` (see N-070), and the session
  may not edit that file to put it back. What it did show on a warm pane:
  the *Switch model?* dialog comes up with the wording `panesetup.go`
  matches, Enter on it prints `Set model to Haiku 4.5 and saved as your
  default…`, and `/effort` sent ~0.5s later printed its line whole. The
  400ms timing question stays open until N-070 is decided.

- **N-033** · raised `2026-0924-1146-info-mark-blue-chip` · value low
  Check the info chip by eye. Look at an info row in cats, both plain and
  highlighted, plus the bar and the menu. If the fullwidth `ｉ` looks thin or
  odd in the fallback font, the fallbacks are a plain `i` padded inside the
  chip, or the `ℹ️` emoji (blue square, but its "i" isn't italic).

- **N-039** · raised `2026-0924-1924-nextlist-send-value-levels-v0.36.0` · value low
  Check the value marks and the annotation bar by eye in cats: that 🔷 draws
  two cells in the cats font, that the faint `│` rules read as dividers, and
  that the bare tier's reverse-lit radio is visible. At 100 cells the bar keeps
  the Value/Priority labels and drops the checkbox words (the words need 104);
  swap the two tiers if the words turn out to matter more.

- **N-041** · raised `2026-0924-1939-nextlist-hover-card` · value medium
  Live-test the Next List hover card (`nexthover.go`) in cats: the 400ms
  dwell and the warm window across rows, that the card closes on a key, a
  click and on leaving the rows, and that the 88-cell box and its 15-row cap
  (raised from 62, then 76 cells, and 7 rows) read well on real items
  (sub-bullets kept, `…` on long ones) and shrink on a short pane. The title
  now carries the value (`N-001 · Open · value medium`), the foot only
  `raised …`, and the body is a pale yellow (`colCardBody`). Also check
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

- **N-048** · raised `2026-0925-1315-multi-drop-batches-phase1` · value medium
  Live-test batches in cats. Check an all-at-once batch onto new worktrees
  (the tabs open one after another, each prompt lands whole, and each is
  marked done), a one-prompt-listed batch into a running pane, and a
  failure part-way (the rest still go, and the record shows ✗ with the
  error). Also check the composer's drag and clicks in both layouts (100
  columns or more, and narrower), Next List items becoming backlog prompts,
  and ⧉ Duplicate after a partial failure. Only the tests have exercised it.

- **N-053** · raised `2026-0925-1344-batch-scheduling-phase2` · value medium
  Live-test scheduled batches in cats. Schedule one `in 2m` onto new
  worktrees and watch it fire (the status line, each prompt marked done, the
  record's Scheduled and Dropped times). Check the `⧉ HH:MM` mark on the list
  rows and that it goes on ✕ Unschedule. Close the manager past a batch's
  time and reopen it: the batch should read *missed* with the reason, and
  enter on it should open the composer. Also schedule into a running pane,
  close that pane, and let it fire: each step should fail with "the
  scheduled pane is gone". Only the tests have exercised it.

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

- **N-064** · raised `2026-0926-2033-prompt-editor-double-click-word` · value low
  Live-check the prompt editor's double-click in a real cats pane (and a
  plain terminal). Tests drive `MouseClickMsg` pairs directly; confirm the mux
  delivers both presses and the release between them fast enough to land
  inside `doubleClickWindow` (500ms), and that the word highlight paints.

- **N-066** · raised `2026-0926-2057-card-width-tint-and-brighter-greys` · value low
  Eyeball the brightened grey ramp in cats (`styles.go`): `colMuted`,
  `colDim` and `colFaint` each rose ~9 points per channel, and
  `colCardBody` to `#d0ccae`. Check that the four text tiers still separate
  on colBg and colPanel, that finished prompts and footers still read as
  quiet, and that the Next List card's yellow stays a tint, not a color.

- **N-070** · raised `2026-0927-0032-drop-settings-side-effect-v0.42.1` · value high
  A drop's `/model` and `/effort` now change the user's defaults. In Claude
  Code 2.1.283 `/model X` answers `Set model to … and saved as your default
  for new sessions` (its help: "Your pick becomes the default for new
  sessions"), and `/effort X` writes `effortLevel` / `modelSettings` the
  same way. So an existing-pane drop with a model or effort set silently
  rewrites `~/.claude/settings.json`, and every later session starts on the
  prompt's model. Found live while starting N-017. **Needs the user**:
  (a) keep sending them and say so on the picker row and in the status
  note; (b) stop sending them to running panes and list model/effort as
  unapplied, as permission mode is (`paneUnapplied`); or (c) look for a
  session-only road (the binary has a `for this session only` branch; what
  selects it was not found). Recommendation: (b) until a session-only road
  is found, since a drop should not change settings the user never opened.
  Blocks N-017, N-043 and N-044, which all need live `/model` runs.

- **N-071** · raised `2026-0927-0037-probe-driven-live-tests` · value low
  Arrow keys slow down linearly with the prompt's length: measured live,
  ~7ms per key at 30 lines, ~20ms at 300 and ~50ms at 1,000. In process,
  1,000 lines cost ~13ms in Update and ~10ms in View, and 3,000 lines cost
  ~37ms + ~28ms. The cost is bubbles' textarea (v2.1.1): `Update` and
  `View` both render every line of the value into the viewport
  (`textarea.go:1329` and `:1455`) and let it slice. cats-todo adds almost
  nothing on top. At a fast key repeat (~30ms), a held arrow falls behind
  past a few hundred lines. The fix is a textarea that renders only the
  rows in view: a fork, or an upstream patch.

- **N-072** · raised `2026-0927-0100-live-autosave-and-next-send` · value high
  Claude Code 2.1.283 hands a dropped prompt to the model as pasted text,
  not as the user's request. `pane.send_input` is a bracketed paste, and
  Claude Code tags pasted input `<pasted_content>` (flag-gated, `FP()` in
  the binary), whose instructions the model is told to follow only where
  the user's own words ask. A drop is all paste, so Opus 5.5 said: "Your
  message contains only pasted text, with nothing you wrote around it, so I
  haven't acted on it yet." Measured on fresh panes: realistic read-only
  tasks were done by Opus (framed as a Next List item or not) and by a
  fresh Haiku session. A synthetic "reply with only ALPHA11" was declined
  by Opus and Haiku once framed as `Next list item N-001 (…):`, but done
  bare. Sonnet 5 did it either way. Haiku declined a realistic Next List
  item in a pane whose conversation was already about pasted content. The
  risk is highest where nobody watches: a scheduled drop or a batch loop
  sees the pane go idle after a question and counts the prompt as sent.
  **Needs the user** (the road is cats' wire): (a) a typed send in cats,
  key events with shift+enter for newlines, so the message is the user's;
  (b) cats-todo leads every drop with an explicit ask, which still sits
  inside the paste; or (c) accept it and document it. Recommendation: (a),
  with (b) as a cheap interim for the Next List's framing.

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

- **N-060** · closed 2026-09-27, `2026-0927-0125-picker-fold-and-double-click-live` · raised `2026-0926-1520-next-card-and-target-fold`
  — Driven live through `catctl probe`, with claude panes in three
  workspaces: the scratch project's own, a decoy `ct-other`, and this
  session's. The backlog, Next List, schedule and batch pickers each listed
  only the two new-session rows and `claude · ct-live (this project)`, with
  `… More drop targets (2 running agents in other projects)` last. Enter on
  it (backlog, schedule) and a click on it (Next List, batch) both unfolded
  the list, with the highlight on the first revealed agent. The unfolded
  list is in workspace order, so this project's agent sits between the two
  others. Each picker was left with esc. On the way, an ↑ from the top row
  (it does not wrap) and an enter scheduled one drop by accident, and it was
  cleared from the list (ctrl+s, empty box, enter).
- **N-046** · closed 2026-09-27, `2026-0927-0110-next-list-marks-and-menu-live` · raised `2026-0925-1104-nextlist-context-menu`
  — Driven live through `catctl probe`. The right-click opened all 15
  rows. ⧉ Copy ID put `N-002` on the pasteboard, and ⧉ Copy as prompt the
  whole cited prompt. ⚙ Session… and ◫ Images… opened over the draft, and esc
  landed on it. A second esc went to the *prompt list*, off the page, which
  was fixed in `511feaf`: a draft made from an item now cancels back to the
  Next List with the highlight kept (`TestNextListDraftEscReturnsToThePage`).
  ◷ Schedule… added the item (6 → 7) and opened the scheduler, whose esc
  lands on the prompt list with the new row highlighted, as the README says.
  With an open copy in the backlog the ⤓ Add rows grey out (`#728377` against
  `#d6ddd6`). The box fits below the pointer in a 30-row pane and flips above
  it for a low row. At 20 and 24 rows neither side fits, so it pins to the
  top with every row showing. ✔ Save on such a draft still lands on the
  prompt list, which was left alone.
- **N-062** · closed 2026-09-27, `2026-0927-0110-next-list-marks-and-menu-live` · raised `2026-0926-1840-dimmer-greys-and-next-list-backlog-record`
  — Driven live through `catctl probe`. A successful send left a done copy
  and read `… · recorded done in the project backlog` (run and paste). An esc
  from the picker, and a send whose pane was closed under the open picker
  (`send failed: cats error: unknown pane 332`), both left the backlog as it
  was. After ⤓ Add from the menu, the green ⤓ (`#4db380`) showed on the row
  at once. The probe's `read` found the text starting on cell 13 for a ◆ ⤓
  row, a bare row and a 🔷 ⤓ row alike. Done copies earn no mark, as
  designed.
- **N-038** · closed 2026-09-27, `2026-0927-0100-live-autosave-and-next-send` · raised `2026-0924-1924-nextlist-send-value-levels-v0.36.0`
  — Driven live through `catctl probe`, in a scratch project with its own
  next-list.md and backlog. esc from the picker went back to the page with
  the highlight kept (N-003), and the backlog stayed `[]`. A running pane,
  run mode: `N-001 dropped → claude · recorded done in the project
  backlog`, the prompt arrived whole and submitted, and a done copy was
  written with the value carried. A running pane, paste mode: `N-002 pasted →
  claude · press enter there to run · …`, with the text left in the input
  box. A new session: the tab opened in the list's project, and the prompt
  landed and was worked on. A worktree session, paste mode: branch
  `todo/n-002-…` in a new workspace, and claude in the checkout with the
  paste waiting. The workspace, worktree and branch were removed afterwards.
  What claude then *did* with a drop is another matter, raised as N-072.
- **N-034** · closed 2026-09-27, `2026-0927-0100-live-autosave-and-next-send` · raised `2026-0924-1231-prompt-editor-autosave`
  — Driven live through `catctl probe`, with `autosaveSeconds: 15` in a
  scratch config. Typing one key a second through the tick, the note
  `autosaved HH:MM` came up on its own row under the ⚙ line, in the same
  muted tone as the 📎 and ⚙ lines (`#92a498`). The prompt rows and the
  caret did not move, only the footer dropped a row, and the next key cleared
  the note as designed. After the first write the add became an edit, so the
  footer lost `ctrl+g scope`, which is right. esc after an autosaved add left
  no row (`cancelled · autosaved changes taken back`). esc after an
  autosaved edit (`ZZZ` on disk) put the original text back.
- **N-026** · closed 2026-09-27, `2026-0927-0037-probe-driven-live-tests` · raised `2026-0915-1836-prompt-editor-undo-v0.32.0`
  — Driven live through `catctl probe`, which sends the same `key` message
  (mods bit 8 for Cmd) the cats page sends. cmd+z undid a word at a time,
  and shift+cmd+z, ctrl+y, ctrl+z and ctrl+shift+z all did their jobs. The
  menu's ↶ Undo and ↷ Redo rows (printing `cmd+z` and `shift+cmd+z`) did the
  same on a click. Undo after an `@` insert went back to the typed `@`, and
  after a spelling fix (`teh` → `the` from the ctrl+l panel) back to `teh`.
  Redo put both back. The page forwards ⌘Z/⌘⇧Z to a kitty pane
  (cats `cmd/catway/web/js/20-keys.js:214`). In Cats.app the Edit menu's
  Undo/Redo key equivalents don't swallow them; cats checked that in
  `2026-0727-1706-cmdz-undo-forwarding-and-esc-leader-chaining` with r-ed.
- **N-023** · closed 2026-09-27, `2026-0927-0037-probe-driven-live-tests` · raised `2026-0915-1637-prompt-editor-paste-line-cap`
  — Driven in a live cats pane through `catctl probe` (catway's browser
  socket: keys, paste and clicks, with the screen captured in ANSI so the
  drawn caret shows). With 300 pasted lines the caret stayed in view after
  typing at the end, enter, three PageUps (it rode the top row), a click on
  a middle row (typing landed there), and a shift+↓ sweep of 35 rows past
  the bottom (the view followed). Wheel over the prompt does nothing, since
  the form has no wheel handling; that is not new. The one finding is speed,
  raised as N-071.
- **N-069** · closed 2026-09-27, `2026-0927-0032-drop-settings-side-effect-v0.42.1` · raised `2026-0926-2228-headless-install-offer`
  — Released v0.42.1: both version files bumped, `chore(release): v0.42.1`
  (`a2bae17`) tagged `v0.42.1` and pushed with `56c5c4d`, so a seeded cats
  install now builds the fix that leaves the backlog offer unspent.
- **N-067** · closed 2026-09-26, `2026-0926-2211-two-line-footers-and-gofmt-n067` · raised `2026-0926-2057-card-width-tint-and-brighter-greys`
  — `gofmt -w hangup.go` added the blank `//` line before the
  `- SIGHUP asks the program to quit` bullet. `gofmt -l .` is now empty.
- **N-068** · closed 2026-09-26, `2026-0926-2141-prompt-click-linear-n068` · raised `2026-0926-2129-caret-offset-rebuild-n063`
  — A click in the prompt is linear in its lines. `promptLines` (`ui.go`)
  builds the display-line table on a probe grown one row at a time
  (`InsertString` changes the row and never repositions). It reads each
  line's start with `SetCursorColumn` + `LineInfo`, which look only at the
  caret's own row. `placePromptCursor` places the caret with
  `setPromptCaretOffset` while the view is `d-y0+1` lines tall. That lands the
  scroll on y0, and then the real height goes back. The old stepper stays as
  a backstop. Timings: 2,000 lines went from ~1.3s to ~5ms, and 9,999 lines
  now take ~21ms. Along the way it fixed a wrong click the old walk made: on
  a row with no spaces, exactly a multiple of the width long, the walk's
  table stopped short. That dropped the row's trailing empty line and every
  row below, so clicks under it landed on the wrong line and the view jumped.
  Tests in `promptlimit_test.go`: `TestPromptLinesMatchesTheWalk`,
  `TestPromptClickMatchesTheWalk` (the old code kept as oracles),
  `TestPromptClickIsLinearInRows`,
  `TestPromptClickReachesPastAnExactlyFilledRow`.
- **N-063** · closed 2026-09-26, `2026-0926-2129-caret-offset-rebuild-n063` · raised `2026-0926-1928-indent-trim-fold-search-line-cap-v0.41.0`
  — `setPromptCaretOffset` (`spellpanel.go`) no longer walks the caret. It
  rebuilds the value: `SetValue(suffix)`, `MoveToBegin`,
  `InsertString(prefix)`, and the library's insert leaves the caret on the
  offset. Then `SetHeight(Height())` runs the one `repositionView`. The line
  count never changes, so the 10,000-line cap cannot truncate the re-insert.
  A caret to the end of 9,999 lines went from ~30s to ~10ms (1,000 lines:
  ~0.35s → ~1.6ms), and enter on a 20k-rune line is ~1.2ms. The view now
  also scrolls to a caret set deep in a wrapped row, which the walk's final
  `SetCursorColumn` left unrepositioned. Tests in `promptlimit_test.go`:
  `TestPromptCaretOffsetIsLinearInRows`,
  `TestPromptCaretOffsetKeepsTheValueAtTheCap`,
  `TestPromptCaretOffsetScrollsLikeTheWalk` (the old walk kept as oracle).
  The click path has the same cost and is raised as N-068.
- **N-065** · closed 2026-09-26, `2026-0926-2123-drop-picker-context-fill-and-triple-click-n065` · raised `2026-0926-2033-prompt-editor-double-click-word`
  — A triple-click selects the logical line (the whole wrapped paragraph,
  newline left out), and a drag after a double- or triple-click extends by
  whole words or lines, keeping the pressed span and flipping its anchor as
  the pointer crosses it (`promptClickCount`, `promptGrain`,
  `extendPromptSelByGrain` in `promptsel.go`). The count cycles 1 → 2 → 3 → 1,
  and the third press may drift within the first press's word. Tests:
  `TestPromptTripleClick*`, `TestPromptDoubleClickDragExtendsByWords`. README
  updated.
- **N-045** · closed 2026-09-26, `2026-0926-2015-hangup-sighup-race-n045` · raised `2026-0924-2029-notes-send-to-gonotes`
  — A bubbletea kill path race, not ours to fix upstream (still in v2.0.10).
  SIGHUP cancelled the program's context, and bubbletea's
  `shutdown(kill=true)` cancels the cancelreader but skips `waitForReadLoop`,
  then Closes its cancel pipe while the woken reader goroutine is still in
  `kqueueCancelReader.wait` calling `Fd` on it (cancelreader v0.2.2,
  `cancelreader_bsd.go:109` vs `:141`). SIGHUP now sets a flag and calls
  `p.Quit()`, the graceful path, which waits for the read loop first
  (`terminalWatch.start`, `hangup.go`). EOF/EIO keeps the kill, since its read
  loop has already ended. The report was captured with `GORACE=log_path=…`,
  not by teeing the pty. It reproduced only with `-cpu` > 1 on a loaded
  machine: 8 of 60 runs before the fix, 0 of 60 after.
- **N-025** · closed 2026-09-26, `2026-0926-1928-indent-trim-fold-search-line-cap-v0.41.0` · raised `2026-0915-1637-prompt-editor-paste-line-cap`
  — Edits past the 10,000-line limit are refused whole with a note, and
  never trimmed (`promptcap.go`). The limit had more roads than the paste:
  every SetValue edit (the column mode, enter, an inserted prompt) cut the
  prompt's own tail. Guards: `pasteFitsPrompt` (bracketed paste and the Cmd+V
  read, one caret or all; a replaced selection's breaks count as room),
  `newlinesFitPrompt` (enter, one caret or every caret), `insertSnippet`, and
  `beginEditRef`, which will not open a stored prompt already past the limit.
  `promptMaxLines` mirrors the library's unexported `maxLines`, pinned by
  `TestPromptMaxLinesMatchesTheLibrary`. README updated.
- **N-061** · closed 2026-09-26, `2026-0926-1928-indent-trim-fold-search-line-cap-v0.41.0` · raised `2026-0926-1520-next-card-and-target-fold`
  — The filter now searches the folded agents, and nothing unfolds. The
  folded panes are built into the picker, marked `folded`, as `queryOnly`
  rows, a new `listItem` flag: `fuzzyList.filter` lists them only while the
  query is non-empty. The More row is `browseOnly`, so it steps aside while
  a query is typed. `counts` totals what the current mode can list. Clearing
  the query folds them away again. The batch composer's "draft aims at a
  folded pane" check ignores folded rows. README updated. Test:
  `TestTargetFilterSearchesFoldedAgents`.
- **N-011** · closed 2026-09-26, `2026-0926-1928-indent-trim-fold-search-line-cap-v0.41.0` · raised `2026-0912-2234-carried-indent-and-release`
  — Answered yes: the upper line is trimmed. `promptCarriedIndent`'s second
  result is now `bare`, meaning only spaces stand left of the caret. A blank
  indented row and a caret inside an indent are both that one case, so enter
  moves those spaces down instead of copying them: `"  |  x"` becomes `""` /
  `"    x"`. The column mode does the same at a caret alone on its row. It
  cuts the row's head and keeps the text after it (`newlineAtCarets`).
  Tests: `TestPromptEnterInsideTheIndentLeavesNoTrailingSpaces`,
  `TestCaretsEnterInsideTheIndentLeavesNoTrailingSpaces`.
- **N-047** · closed 2026-09-26, `2026-0926-1840-dimmer-greys-and-next-list-backlog-record` · raised `2026-0925-1104-nextlist-context-menu`
  — A Next List row whose item has an open or frozen copy in the target
  backlog wears a green ⤓ after its value mark (`nextBacklogMark`). The page
  holds the set of such IDs (`nextPage.inBacklog`), filled by
  `nextBacklogIDs`, which does one pass over the store and reads each
  citation back with `nextCitedID`, the inverse of `nextItemCite`. The set is
  taken when the page opens, on `ctrl+r`, and from `rebuildList` whenever the
  page is up, so an Add or a send's record redraws the mark. The column is
  reserved on every row. A copy whose citation was rewritten is still not
  recognised; that limit is stated in the README. Tests:
  `TestNextListMarksItemsInTheBacklog`, `TestNextCitedIDReadsBackTheCitation`.
- **N-040** · closed 2026-09-26, `2026-0926-1840-dimmer-greys-and-next-list-backlog-record` · raised `2026-0924-1924-nextlist-send-value-levels-v0.36.0`
  — Answered yes: a Next List send leaves a done copy in the backlog.
  `recordNextSend` (`nextlist.go`) runs from `finishNextDrop` only on a
  successful drop. It marks an open copy of the item done if the backlog
  already holds one (`nextBacklogCopy`), and otherwise adds the item (value
  carried) and marks it done. A failed send, or an esc out of the picker,
  writes nothing, and having no backlog at all is not an error. The heading
  reads `N-014 dropped → … · recorded done in the project backlog`.
  `dropResultMsg` carries the whole `nextItem` in place of `nextID`.
  Tests: `TestNextListSendDispatches`, `TestNextListSendMarksAnOpenCopyDone`.
- **N-005** · closed 2026-09-26, `2026-0926-1840-dimmer-greys-and-next-list-backlog-record` · raised `2026-0912-2110-multi-caret-enter-paste-tab-indent`
  — `TestCaretsTakeTheLocalPasteboard` (`promptcarets_test.go`) drives the
  Cmd+V chord's local pasteboard road with the column mode on, under both
  `super+v` and `meta+v`, with `stubClipboard` in place of the pasteboard.
  It checks that one line per caret spreads (a trailing newline is dropped
  first), that a count mismatch pastes the whole text at every caret, and
  that an empty pasteboard is refused in words without ending the mode. It
  also checks that a local read never falls through to the OSC 52 request.
- **N-059** · closed 2026-09-25 · raised `2026-0925-1739-batches-menu-and-loop-default`
  — Released v0.40.0: both version files bumped, `chore(release): v0.40.0`
  tagged `v0.40.0` and pushed with the code. It carries the Batches page's
  right-click menu (the minor), loop as the default Deliver, the one-hour
  resume window (N-057), N-020, N-024, N-028, N-035 and the column-mode
  footer (N-007).
- **N-007** · closed 2026-09-25 · raised `2026-0912-2110-multi-caret-enter-paste-tab-indent`
  — The column-mode footer is 117 cells (was 128), so a 120-cell pane shows
  all of it. Two segments lost the word that restated the lead ("typing goes
  on every line"): `enter breaks each` → `enter breaks`, `←/→ moves them` →
  `←/→ move`. `ctrl+a/e line ends` stays word-for-word with the editor's
  footer. `TestCaretsFooterFitsA120CellPane` pins every segment at 120 and
  the line at ≤118, so a new key for the mode has to replace a segment.
- **N-035** · closed 2026-09-25, `2026-0925-1836-gofmt-and-autosave-view-row` · raised `2026-0924-1231-prompt-editor-autosave`
  — The list's View panel (`ctrl+l`) has a third row, **Autosave**. It is a
  stepper over off, 15s, 30s, 45s, 1m, 90s, 2m and 5m that wraps at both
  ends: `→`/space/click step longer, `←` shorter. A change is written to
  settings.json at once (`setViewAutosave`, which sets only that field, so
  ctrl+d's save never overwrites a hand edit), and the next form arms with
  it. Opening the panel re-reads the file, so a hand edit shows there and
  takes effect without a restart.
- **N-052** · closed 2026-09-25, `2026-0925-1836-gofmt-and-autosave-view-row` · raised `2026-0925-1315-multi-drop-batches-phase1`
  — `gofmt -l .` is empty. `ui.go` was only the form-stage field block's
  alignment. `promptcode.go` could not take a plain `gofmt -w`: its doc
  comment had ``` ``a ` b`` ``` in running prose, and gofmt reads a double
  backtick there as a quote and rewrites it to `“a ` b“`. The prose now
  points at the same example in the indented block below it, which gofmt
  leaves alone.
- **N-055** · closed 2026-09-25, `2026-0925-1821-composer-footer-and-view-labels` · raised `2026-0925-1344-batch-scheduling-phase2`
  — Once `batchBarTier` drops the hints (under 89 columns, not the 110 the
  item guessed), `batchFooterSegs` leads with the button chords as a legend
  for the row: `▶ alt+enter · ◷ ctrl+s · ☰ ctrl+k · ✕ esc`. The focused
  region's keys follow in the width left. The legend is 41 cells where the
  worded form was 60, so `space pick` still fits at 60 columns. Wide panes
  are unchanged.
- **N-037** · closed 2026-09-25, `2026-0925-1821-composer-footer-and-view-labels` · raised `2026-0924-1802-code-highlight-and-next-list-seed`
  — The `viewContent` comment now says what lipgloss v2 does (a style closed
  at each break and reopened on the next line). It also no longer calls
  this the list badges' hazard, which is style nesting, not wrapping. The
  appended lines are styled now: `⚙ session:` and `📎 n attached:` are dim
  labels, and `(missing — will not be sent)` is in the error hue, pinned
  across a wrap by `TestViewMarksWhatItAppends`.
- **N-028** · closed 2026-09-25, `2026-0925-1808-title-undo` · raised `2026-0915-1836-prompt-editor-undo-v0.32.0`
  — The title has its own undo/redo history (`titleUndo`), a second
  `promptUndo` fed by the same commit point (`recordUndo`), as the item said
  it should be: a second stack, not a share of the prompt's. `undoForm` /
  `redoForm` send the chords to the focused field's history. It coalesces by
  word as the prompt does. `ctrl+u` / `ctrl+k` are each a step of their own.
  The annotation bar and the flag's note keep none, and say where undo works.
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
