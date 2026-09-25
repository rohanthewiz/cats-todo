# Multi-drop batches — picking, ordering, and dropping many prompts at once

## Context

Every drop today is **one prompt → one target**: `shift+enter` on a list row (or
✉ Send on a Next List item) opens the drop picker, and the prompt lands in a new
session, a new worktree, or a running pane. A per-prompt `Schedule` can defer it.
That covers a single job well and a set of jobs badly. Five prompts meant for
five parallel worktrees take five trips through the picker, the same session
options are set five times, and nothing records that the five went out together.

The ask: a UI that works on **sets** of prompts, from the main backlog and from
the Next List. Pick several on the left, order them on the right, give the set
shared session options (worktree included), and then either drop it now or
schedule it, either all at once or as a **loop** that sends the prompts
one after another until the batch is complete. Each set can be named, several can be built
(one at a time), and afterwards every set shows in a list as *dropped* or
*scheduled for <time>*.

### Naming: "batch", not "group"

The user's word was *group*, but the code and README already use it for the
list's open / frozen / done sections (`fuzzylist.go` grouping, `ctrl+d` "fold
closed"). If one word meant two things, every comment and status line would need
to say which one it meant. **Batch** says the right thing (a set sent as one
unit) and collides with nothing, so it is the name used below and in the UI.

## The UI

Two new stages:

- `stageBatchCompose`: the composer, where a batch is built or edited.
- `stageBatches`: the Batches page, listing every batch and its state.

### 1. The composer (`stageBatchCompose`)

Wide layout (≥ 100 cells): two panes side by side, the batch's settings under
the right pane, and a button row.

```
 cats-todo · New batch                                                        v0.39.0
╭─ Pick ─────────────────────────────────╮╭─ Batch · nightly cleanup ─────────────────────╮
│  Backlog │ Next List      ⌕ filter…    ││ ⠿ 1  Fix flaky drop test              ▲   ⚙   │
│ ☑ all 3 of 12 open                     ││ ⠿ 2  Rename fuzzyList headings                │
│ ── project ──                          ││ ⠿ 3  N-014 Tidy promptsel comments     ✚      │
│ ☐ ▲ Add worktree cleanup command       ││                                               │
│ ☑   Fix flaky drop test                ││                         order: manual · [A→Z] │
│ ☑   Rename fuzzyList headings          │├───────────────────────────────────────────────┤
│ ☐ 🍏 README typo in export section     ││ Name     [nightly cleanup                   ] │
│ ☐ ❄ Port listmenu to menuBox   frozen  ││ Deliver  ( ) all at once  (•) loop, in order  │
│ ── global ──                           ││          ( ) one prompt, listed               │
│ ☐   Try the new effort flag            ││ Loop     (•) same session  ( ) fresh each     │
│ ☐ ✎ Draft blog post notes     info     ││          between [/compact      ] fail [stop▾]│
│                                        ││ Target   ＋ New claude session  ☐ on worktrees │
│                                        ││ Session  ⚙ sonnet · high · acceptEdits · wrap │
│                                        ││ When     (•) now  ( ) at [tomorrow 9:00     ] │
╰────────────────────────────────────────╯╰───────────────────────────────────────────────╯
 [▶ Drop now]  [◷ Schedule]  [✕ Cancel]
 space pick · ctrl+a all · alt+↑/↓ reorder · ctrl+r session · tab next pane · esc back
```

Narrow layout (< 100 cells): the same three regions (Pick, Batch, Settings)
become tabs across the top, and `tab` / a click moves between them. This follows
the "button rows shrink, never wrap" rule (contract 6): the panes change
arrangement rather than squeezing each other down to unreadable widths.

**Left, the Pick pane**

- Two source tabs: **Backlog** (project and global, under the list's own
  section headings) and **Next List** (the `ai_docs/todo/next-list.md` items,
  Open and Roadmap).
- A checkbox means *in the batch*. There is no separate "add →" step, so the
  right pane is simply the checked rows, in the order they were checked.
  Unchecking a row removes it on both sides.
- `☑ all` selects every visible row, which respects the filter. So "filter
  `docs`, then ctrl+a" picks every docs prompt. When the rows are partly checked,
  the box shows ☒ and the count ("3 of 12").
- Rows that can't be dropped stay visible but can't be checked, and they say
  why on the row: *frozen*, *info*, *done* (contract 4, refuse in words). A
  prompt that already has its own schedule can be checked, but shows `◷ 15:30`
  so the double booking is visible before the batch is saved.
- The filter reuses `fuzzyList` (already shared by the list and the drop picker),
  so matching, highlighting and headings behave the same as everywhere else.

**Right, the Batch pane**

- One row per picked prompt: a `⠿` drag handle, a 1-based position, the title,
  and the row's own marks (priority and value, from `annotSlots`).
- **Reorder:** drag by mouse (the list's `drag`/`dragging`/`dragMoved` gesture,
  reused), or `alt+↑/↓`, the same chord `promptmove.go` uses to move a line.
- **[A→Z]** sorts the rows once, by title. It is an action, not a mode: the
  result is an ordinary manual order that can still be dragged afterwards. The
  label reads `order: manual` again as soon as anything moves. The alternative,
  a sort lens like the list's priority order, is ruled out because a batch's
  order is what gets delivered, so it has to be something the user can see and
  edit.
- `⚙` on a row means that prompt has session options of its own. `✱` means a
  batch option overrides one of them (see *Session options*). `✚` means the row
  is a Next List item that becomes a backlog prompt when the batch is saved.
- `delete`/`backspace` removes the row (it unchecks on the left too).

**The settings block (below the Batch pane)**

| Row | What it holds |
|---|---|
| Name | Optional. When it is empty, the batch is shown as `<first title> +N`. |
| Deliver | How the prompts reach the agents. See *Delivery modes*. |
| Target | The drop picker's rows (new \<agent\> session, running panes) plus a `☑ on worktrees` toggle. Enter on it opens the existing picker, so a target is chosen the same way as for a single drop. |
| Session | The ⚙ summary line. `ctrl+r` (the form's chord for the same panel) opens `stageSession` on the batch's own `SessionOpts`. |
| When | `now` or `at [...]`. The field reuses `parseScheduleTime`, so `15:30`, `in 2h`, `tomorrow 9:00` and `2026-09-26 09:00` all work the same as in the schedule editor. |
| Loop | Shown only when Deliver is *loop*: the loop's options. See *Loop*. |

The primary button follows **When**: it reads **▶ Drop now** when When is `now`
and **◷ Schedule** when a time is set. Both buttons stay in the row so the chord
footer does not shift, and the one that doesn't apply is dimmed and explains
itself if pressed.

### 2. Delivery modes

The order on the right only matters if the delivery mode uses it, so the mode is
chosen explicitly:

| Mode | What happens | Order means | Valid targets |
|---|---|---|---|
| **All at once** (fan-out) | Each prompt gets its own new session, or its own worktree when the toggle is on. Tabs open one after another, without waiting for any of them to finish. | Launch order | New session only. With a running pane as the target it would be the loop, so choosing a pane here is refused in words. |
| **Loop, in order** | Prompt 1 is sent. When it finishes, prompt 2 is sent, and so on until the batch is complete. See *Loop*. | Execution order | New session (optionally on a worktree) or a running pane |
| **One prompt, listed** (combined) | The prompts are joined into one body, `## 1. <title>` sections in batch order, and dropped once. | Reading order | Any |

All at once with worktrees is the "run three jobs in parallel without them
editing each other's files" case that `worktree.go` was built for. The loop is
for work that has to happen in sequence: each step builds on the last, or the
steps must not run concurrently. Combined is for small related tasks where one
agent should see all of them together.

### 3. Session options: batch over prompt

The batch has its own `SessionOpts`. When each prompt is delivered, the options
are merged **field by field, and a field the batch sets wins**. A field the batch
leaves empty falls through to the prompt's own setting, so "every prompt on
sonnet" doesn't wipe out one prompt's `sess-use` pattern. The composer marks
each prompt the batch overrides with `✱`. The hover card on that mark lists what
was replaced ("effort high → low").

Finish (commit / push / wrap) depends on the delivery mode:

- **all at once:** runs per prompt, in each session.
- **combined:** runs once, at the end of the body.
- **loop, same session:** runs after the **last** prompt only. Each prompt
  wrapping up and pushing on its own would give one commit per step in a single
  conversation.
- **loop, fresh session each:** runs per prompt, since each session is its own
  conversation.

### 4. Loop

A loop sends the batch's prompts one at a time, in batch order, and moves to the
next only when the current one is finished. **When** controls when the loop
starts: `now`, or a scheduled time. The loop then runs until every prompt has
been delivered.

**"Finished"** means the pane's `AgentState` (from `pane.list`, the same field
the drop picker shows as `[working]`) goes from *working* to *idle*. It has to
see *working* first: an agent that is still idle just after the prompt landed
hasn't started it yet, and treating that as done would send the next prompt too
early.

The loop's options (the **Loop** rows, shown only for this mode):

| Option | Choices | Default and why |
|---|---|---|
| Session | **same session** (each prompt goes into the pane the first one opened, or the chosen running pane) · **fresh session each** (a new tab or worktree per prompt, opened only when the previous one is idle) | same session. A sequence usually builds on what came before, and one conversation keeps that context. Choose fresh each when steps must not see each other's context but still must not overlap. |
| Between command | Any line submitted to the pane after one prompt finishes and before the next is sent: a slash command (`/clear`, `/compact`, `/sess-save step`, `/code-review`) or plain text ("run the tests and fix anything red"). With *fresh session each*, it goes to the session that just finished, before the next one opens. That is where `/sess-save` or `/code-review` has something to act on. | Empty. Every command here changes the context the loop was set up with, so it is opt-in. |
| Pause | A wait before the next prompt, e.g. `30s` | None. |
| On failure | **stop** (the rest stay open in the list; the batch reads *stopped at 3/5*) · **skip** (record the error and carry on) | stop. A sequence usually means later steps depend on earlier ones. |
| Max wait | Longest a prompt may run before it counts as stuck, e.g. `2h` | Empty, meaning no limit. When set, a stuck prompt is treated as a failure, which triggers the On failure rule. |

**How the between command finishes.** Some commands make the agent work
(`/compact`, `/code-review`, plain text), and others return straight away
(`/clear`, `/model`). The loop handles both with one rule: submit the command,
then wait for *working → idle* the same way it waits on a prompt. If the pane
never shows *working* within a short grace (about 2s), the command is treated as
an instant one, and the loop waits `clearSettle` so the next keystrokes aren't
lost, as a single drop does after `/clear`. A command that fails, or that runs
past Max wait, counts as a failure of the step it follows, and On failure
decides what happens next. The field is sent as typed, as a single submitted
line. The only value refused (in words, at save) is a multi-line one, because a
second line would be submitted as a second message the loop doesn't know to
wait for.

Whether the command also runs after the **last** prompt is a checkbox beside
the field, off by default. `/compact` after the final step is wasted work, while
`/sess-save` after it is often the point. The Session panel's Finish (wrap,
push) still runs after everything, the between command included.

The loop runs in the manager, the same as schedules, so the manager has to be
open for it to advance. If the manager closes mid-loop, the batch stays
*paused at 2/5*. When it reopens, the loop picks up again, provided the pane
still exists (same session) or the checkout does (worktrees). Otherwise the
batch is marked *missed* with the reason, and ⧉ Duplicate prefills the prompts
that were never sent.

### 5. The Batches page (`stageBatches`)

```
 cats-todo · Batches (5)                                                      v0.39.0
 ◷ nightly cleanup     3 prompts · each · worktrees · claude     fires Fri 26 · 09:00
 ⟳ refactor trio       3 prompts · loop · same session · cats    running 2/3
 ⏸ docs sweep          4 prompts · loop · fresh each · worktrees paused  1/4
 ✓ quick wins          5 prompts · combined · claude · cats      dropped Wed 24 · 18:40
 ✗ release prep        2 prompts · each                          missed  Wed 24 · 09:00
 [＋ New]  [▶ Drop now]  [✎ Edit]  [⧉ Duplicate]  [✕ Unschedule]  [✖ Delete]
```

- Rows are sorted by state and then by time: scheduled (soonest first), running
  or paused, and then dropped / missed (newest first). The underlying file keeps creation
  order, so like `todos.json` the stored order is never rewritten for display.
- `enter` opens the batch. A **scheduled** batch opens in the composer to edit.
  A **dropped** batch opens a read-only view listing each prompt with ✓ / ✗ and
  where it went (pane, or `todo/<slug>-<hex>` branch), so an all-at-once batch can be traced
  back to its worktrees.
- **⧉ Duplicate** is how a dropped batch is run again: it opens the composer
  prefilled, with *When* reset to `now`.
- There is a right-click menu (on `menuBox`, like `listmenu.go`) with the same
  actions.

### 6. Entry points and chords

`ctrl+b` is free on the list and on the Next List (checked against the
key-chord table and a grep of the key cases).

| Where | Gesture | Opens |
|---|---|---|
| List | `ctrl+b` | The Batches page |
| List | right-click a row → *⧉ Add to batch…* | The composer with that row checked |
| Next List | `ctrl+b` | The composer on the **Next List** tab |
| Next List | right-click → *⧉ Batch…* | The composer with that item checked |
| Batches page | `＋ New` / `ctrl+a` | An empty composer |

Inside the composer, `ctrl+a` is *select all visible*. It means "add" on the
list, but the composer is its own stage, and "all" is the meaning people expect
from that chord in a picker. `ctrl+r` opens the session panel (the form's
chord), and `ctrl+s` saves and schedules (the list's schedule chord). The list
row gains a `⧉` badge while its prompt sits in a pending batch, so the list
still shows that the prompt is taken.

## Data model

### `batches.json`, beside `todos.json`

`.cats-todo/batches.json` in the project, and `~/.config/cats-todo/batches.json`
for the global backlog. The batch goes in the project file whenever any of its
prompts is a project prompt. It is a separate file, not a new key in
`todos.json`, for two reasons:

- A batch can mix project and global prompts, so it doesn't belong to either
  backlog.
- `todos.json` stays byte-identical for anyone who never builds a batch
  (contract 1).

The format is JSON rather than bytdb because the codebase already fixes its
storage format: both backlogs and bundles are human-readable JSON with the
`omitempty` compatibility discipline. Keeping batches in the same form means
`store.go`'s load/save/claim code and its tests carry over. Writes use the same
temp-file-and-rename path as `todos.json`.

```go
type Batch struct {
    ID      string    `json:"id"`
    Name    string    `json:"name,omitempty"`
    Created time.Time `json:"created"`
    Items   []BatchItem `json:"items"`             // delivery order
    Deliver string      `json:"deliver,omitempty"` // "" = all at once | "loop" | "combined"
    Target  Schedule    `json:"target"`            // reuses Kind/Pane/Agent/Command/Cwd/Worktree
    Session *SessionOpts `json:"session,omitempty"` // overlays each item's own
    Loop    *LoopOpts   `json:"loop,omitempty"`     // Deliver == "loop" only
    At      *time.Time  `json:"at,omitempty"`      // nil = dropped immediately
    State   string      `json:"state"`             // scheduled|running|paused|dropped|stopped|missed|cancelled
    Next    int         `json:"next,omitempty"`    // loop: index of the item to send next (resume point)
    Runs    []BatchRun  `json:"runs,omitempty"`    // one per delivered item
}
type BatchItem struct {
    Scope string `json:"scope"` // "project" | "global"
    ID    string `json:"id"`    // the todo's id
    Title string `json:"title"` // snapshot, so history reads after a todo is deleted
}
type LoopOpts struct {
    Fresh   bool   `json:"fresh,omitempty"`   // a new session per prompt; false = same session
    Between     string `json:"between,omitempty"`     // line submitted after each prompt finishes, e.g. "/compact"
    BetweenLast bool   `json:"betweenLast,omitempty"` // also run it after the final prompt
    Pause   string `json:"pause,omitempty"`   // "30s"; "" = none
    OnFail  string `json:"onFail,omitempty"`  // "" = stop | "skip"
    MaxWait string `json:"maxWait,omitempty"` // "2h"; "" = no limit
}
type BatchRun struct {
    Item   int       `json:"item"`
    At     time.Time `json:"at"`
    Pane   uint32    `json:"pane,omitempty"`
    Branch string    `json:"branch,omitempty"`
    Err    string    `json:"err,omitempty"`
}
```

`Target` reuses `Schedule`'s fields because they already describe "a drop
destination recorded now and resolved later", including the rule that a pane ID
is re-checked at fire time.

### Next List items become prompts

A batch only references backlog prompts, never raw Next List text. When the
batch is saved, a checked Next List item is turned into a backlog prompt through
the path `nextmenu.go` already uses for ⤓ Add (`nextBacklogCopy` reuses an open
copy, and the prompt starts with `nextItemCite`). A scheduled batch firing at
3am then reads the same kind of record as every other drop, and the item's
prompt shows in the list with its `⧉` badge like the rest.

### Prompt lifecycle

- **In a pending batch:** the prompt stays open in the list, with the `⧉` badge.
- **Delivered:** it is marked done, exactly as a single drop does
  (`dropResultMsg`).
- **Delivery failed:** it stays open, and its `BatchRun.Err` says why. The batch
  becomes *dropped (2/3)*, and ⧉ Duplicate prefills only the failures.
- **Frozen, marked done or deleted while its batch is pending:** at fire time it
  is skipped, with a reason, never fired. This is the same backstop reading
  `fireDueSchedules` applies to closed todos.

## Firing

- `scheduleTick` gains `fireDueBatches`, with the same grace (`scheduleGrace`),
  missed marking, and claim-before-fire (`claimBatch`, modelled on
  `claimSchedule`) so that two manager panes can't both fire a batch.
- A **batch runner** (`batchrun.go`) holds the running batch's remaining
  `pendingAction`s. The existing `m.dropping` guard lets only one drop type at a
  time, so all at once is a chain: each `dropResultMsg` for a batch item dispatches
  the next one. That means no new concurrency, and no two drops can end up
  typing into each other's panes.
- **Loop** adds a watcher. On each tick it checks the active pane's
  `AgentState` (from `pane.list`). On the first *idle after working*, it applies
  the between command (waiting out its own working → idle), then the pause,
  and then sends the next item. `Next` is written to
  `batches.json` before each send, so a manager that closes mid-loop resumes at
  the right prompt and never re-sends one. The claim rule also stops a second
  manager pane from advancing the same loop.

## Files

| File | Change |
|---|---|
| `batch.go` (new) | `Batch` types, `batchStore` (load / save / claim), the overlay merge, combined-body composition, loop option normalizers, all pure and tested |
| `batchcompose.go` (new) | The composer stage: panes, checkboxes, reorder, settings rows, layouts |
| `batches.go` (new) | The Batches page and its menu |
| `batchrun.go` (new) | Runner chain, loop watcher and resume, `fireDueBatches` |
| `ui.go` | Stage constants, `ctrl+b` on list, the `⧉` badge, tick hook, routing `dropResultMsg` for batch items |
| `nextlist.go` / `nextmenu.go` | `ctrl+b`, the *⧉ Batch…* row |
| `listmenu.go` | The *⧉ Add to batch…* row |
| `README.md` | A "Batches" section: the why of the three modes, the overlay rule, the loop's finish detection |

## Phases

1. **Compose and drop now.** Composer, all at once and combined, session overlay,
   `batches.json` with run history, the Batches page read-only. This is the
   smallest slice that removes the five trips through the picker.
2. **Schedule.** When = at, firing, missed, claim, editing a scheduled batch,
   Duplicate / Unschedule.
3. **Loop.** The idle watcher, same or fresh session, between command and pause
   prompts, on-fail, max wait, pause and resume across manager restarts.

Each phase is a minor version bump: each one adds a capability.

## Decisions (2026-09-25)

1. **Delivery modes:** all three: all at once, loop (in order), and one
   combined prompt.
2. **"Loop":** the batch sends its prompts in sequence until it is complete.
   This is not Claude Code's `/loop` and not a repeating schedule. A schedule
   sets when the loop starts.
3. **Session-option precedence:** the batch wins per field. Fields the batch
   leaves blank keep the prompt's own value, and overridden rows show `✱`.
4. **Between command:** the loop can submit one command or line between
   iterations (`/clear`, `/compact`, `/sess-save`, plain text), optionally after
   the last prompt too. It is waited on like a prompt.
5. **Name:** *batch* in the UI and code, because the list already uses
   *group* for its sections.

## Phase 1 as built (2026-09-25)

The composer, Drop now for *all at once* and *one prompt, listed*, the session
overlay, `batches.json` with run history, and the Batches page with a record
view, ⧉ Duplicate and ✖ Delete. Files: `batch.go` (record, store, overlay,
combined body), `batchrun.go` (the chain), `batchcompose.go` (composer),
`batches.go` (page and record), tests in `batch_test.go`. The build departs
from the design above in these places:

- **The chord is `ctrl+k`, not `ctrl+b`.** `ctrl+b` is already taken on the
  list: it is the selection key's alias for terminals that swallow
  `ctrl+space`. `ctrl+k` is free on the list, the Next List page and in the
  composer (where it is ☰ Batches). On the list, `ctrl+k` with prompts ticked
  goes straight to a composer holding them. Without a selection it opens the
  page.
- **The target is a picker row, not a picker plus an "on worktrees" toggle.**
  The target picker already offers each new-session row a second time "on a
  new worktree", so the Target row opens that picker (batch flavour:
  `pickForBatch`) and records the row. A separate toggle would have been a
  second way to say the same thing.
- **Closed prompts are not listed in the Pick pane.** Done and frozen prompts
  are closed work, and a done backlog can be hundreds of rows. Info notes are
  listed, greyed, with the reason on the row.
- **The ⇅ A→Z chip has no chord printed on it.** `s` sorts only while the Batch
  pane holds the keys. Anywhere else it is a letter typed into the filter or
  the name.
- **`BatchRun` records `Items []int` and `Where` (the target description), not
  `Pane`/`Branch`.** `performDrop` doesn't return the pane or branch it
  created. Recording them means widening that return, which is worth doing
  alongside the loop (phase 3), where the pane is needed anyway.
- **Guards added:** leaving a composer with picks on the table (`esc`,
  ✕ Cancel, ☰ Batches) takes a second press. Deleting a record takes two
  `ctrl+x`.
- **When is absent.** Every batch in this build is dropped now. The When row
  and ◷ Schedule arrive with phase 2, and the `loop` delivery value is reserved
  in `batch.go` for phase 3.
