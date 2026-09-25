# cats-todo — a prompt backlog for cats

`cats-todo` (ported from [herdr-todo](https://github.com/rohanthewiz/herdr-todo))
is a TUI prompt-backlog manager for [cats](https://github.com/rohanthewiz/cats):
save prompts of future work per-project (`.cats-todo/todos.json`, committed with
the repo) or globally (`~/.config/cats-todo/`), then *drop* one into a Claude
Code session — an existing agent pane (the picker lists every detected agent
pane with its state and location) or a fresh tab that launches the agent first.

```bash
cats-todo                              # open the manager: project + global merged
cats-todo -p                           # open it on this project's backlog only
cats-todo -g                           # open it on the global backlog only
cats-todo add fix the flaky reconnect  # quick-capture to the project backlog
git log -p | cats-todo add -g -t "review this diff"   # capture piped stdin, global backlog
cats-todo add -i ~/Desktop/shot.png this layout is wrong   # attach an image
cats-todo add --model sonnet --finish wrap "say hi"        # …and how to run it
cats-todo init                         # give this project a backlog of its own
```

Both the manager and `add` scope the project backlog to the same place: the
nearest ancestor holding a `.cats-todo/` directory, else the repo root, else the
current directory — so it does not matter which subdirectory of a project you
launch from. A drop into a fresh tab roots that tab there too.
`-p`/`--project` and `-g`/`--global` pin the manager to a single backlog
(project-only works even when its backlog is still empty). The cats plugins
dialog uses exactly these two as its manifest actions — "run" offers "this
project" and "global only" — while the bare merged view stays the shell
launch.

## Getting started

cats-todo is a [cats](https://github.com/rohanthewiz/cats) plugin, so the
shortest path is the plugin host — install, then launch it in a fresh tab:

```bash
catctl plugin install rohanthewiz/cats-todo   # clone from GitHub + build
catctl plugin run rohanthewiz.cats-todo       # launch in a new tab
```

That leaves the binary in the plugin directory, which is all the manager needs.
Put one on your PATH too — quick capture is only quick if it runs from whichever
project you are standing in:

```bash
go install github.com/rohanthewiz/cats-todo@latest
cd ~/dev/some-project && cats-todo init  # give that project a backlog
cats-todo add fix the flaky reconnect    # capture without opening the manager
```

[Installing](#installing) has the rest: building from a checkout, dev-mode
linking, and the first-install `init` offer.

## Using the manager

In the manager, `enter` opens what is in front of you — the highlighted prompt
into the editor, or a brand-new entry when the list is empty — and `shift+enter`
drops the prompt into an agent. That opens the target picker, where `enter`
hands the prompt over **and lets it run**, and `shift+enter` does the same drop
but **pauses**, leaving the prompt sitting unsubmitted in the agent's input.
Either way the todo is marked done. Inside the editor `enter` inserts a newline — the prompt
is a text editor, so enter means there what it means in every other one — and
`shift+enter` (or `cmd+s`) saves. Outside cats it still manages backlogs; only drops need the
socket.

Coming back to the list from a prompt — the editor (saved or not), the prompt
view, a drop or schedule picker, an export, a delete you answered no to —
lands the highlight on **that prompt**, wherever its row now is. The list's
cursor is a position, and the row may well have moved while you were away: a
new priority under the priority lens, a title that re-sorts under a filter, or
another prompt finishing and closing the gap above it. Coming back on the old
position would put the next key on a neighbour. A new prompt you save is the
one highlighted; a cancelled add, or a prompt no longer on screen (deleted,
folded, filtered out), leaves the cursor where it was.

Both screens open with the same title line, directly under the pane's header:
`CatsTodo vX.Y.Z - Prompts` on the list and `CatsTodo vX.Y.Z - Prompt Editor`
in the editor (`Next List Prompt Editor` for a draft made from a
[Next List](#the-next-list) item). It names the program, the running version (the binary's own, so
it can't disagree with what is installed) and which of the two screens you are
on, so switching between them reads as one tool changing section. It is
truncated rather than wrapped in a narrow pane: everything below it is clicked
by row, and a title that grew a line would move every button out from under the
pointer.

In the editor the title is also the way back: click it and you return to the
Prompts list with your changes **saved**, exactly as ✔ Save would (an empty
prompt is refused with the same message, and the editor stays open). The title
names where you're going, not a verb, so nothing about clicking it suggests your
typing would be thrown away. **esc** (✖ Cancel) remains the one way out that
discards the edit, including whatever the [autosave](#autosave)
already wrote. A brand-new prompt that is still completely blank just closes,
since there's nothing to keep.

The editor's row of buttons runs across the top of the form, right under that
title, on the line a "Edit prompt" heading used to take: **Images**, **Session**,
**Save**, **Send**, **Cancel**. A screen whose whole job is to end an editing
session is better opened by the buttons that end it than by a heading repeating
what you can already see — and a row at a fixed line near the top cannot be
pushed around by a taller editor or a wrapped note, so the buttons stay where
your hand left them.

**Send** is the one way to hand a prompt straight to an agent without going back
to the list: it saves what you have typed and opens the target picker on it, so a
prompt written from scratch reaches a session in one gesture. Its chord is the
save chord with Option held — `shift+opt+enter` — because sending *is* saving
plus one more thing, and holding a second modifier is not a key you hit one slip
from the caret. An empty prompt is refused there exactly as **Save** refuses it,
and everything the picker itself refuses (no socket, a drop already in flight, a
frozen prompt) still leaves your edit saved.

The picker's own list is every place the prompt could land: a new Claude Code
session, a new **GitHub Copilot** session when `copilot` is on your `PATH`, a
new session for any other agent cats currently has running somewhere, then the
same set again **on a new worktree**, and finally each live agent pane with its
state and location.

Picking a row is two decisions in one key. `enter` — or a click on the row —
**drops & runs**: the prompt is typed into the agent and submitted, because
dropping a prompt is asking for the work to start, and the default should be
the thing you came here to do. `shift+enter` (`alt+enter` where the terminal
cannot send shift+enter, and `ctrl+r` is still a spelling of run) **drops &
pauses**: the same delivery, stopping one keystroke short, for a prompt that
wants a last read — or a line of context only you can add — in the agent's own
input before it goes. The status line says which promise it made, "dropping
into…" against "pasting into…", and a paused drop's result says so too, because
a prompt that is merely *sitting* in a pane looks exactly like one that is
already working. A scheduled drop always runs; there is nobody standing by to
press enter for it.

This is a reversal of how the picker used to behave, when the paste was the
rule and running was the chord. An old reflex now pauses a drop instead of
running one — nothing starts that you did not ask to start — and pressing enter
in the agent's pane finishes the job.

### Dropping onto a new worktree

A plain new-session drop launches its agent in the project's own checkout,
which is right for one agent and wrong for two: they share a working tree, so
the second one edits files the first is half-way through changing. The
`… on a new worktree` rows fix that. Picking one asks cats to cut a fresh `git
worktree` checkout on a new branch, open it as its own workspace, and launch
the agent there — the prompt gets a tree to itself, and several agents can work
the same backlog in parallel without stepping on each other.

The branch is named after the todo, under a `todo/` namespace and with a short
unique suffix — `todo/fix-the-sidebar-3f9c` — so the same prompt can be dropped
onto several worktrees at once (three attempts at one task, compared
afterwards) and `git branch -D 'todo/*'` clears a finished batch. The checkout
lands wherever cats is configured to put them (`worktrees.directory`, default
`~/.cats/worktrees`); the plugin does not invent a second convention. Alongside
the agent's tab the new workspace has a shell of its own, which is where you
review the branch and merge it.

These rows only appear when the backlog's project is inside a git repository —
including a repository that *is* a worktree, so a manager opened in one
checkout can still cut the next. A worktree drop that fails to create its
checkout fails outright rather than falling back to the shared tree: choosing
the row is choosing the isolation.

Scheduled drops (`ctrl+s`, below) can target a worktree too. The branch is cut
when the drop fires, not when it is scheduled, so it always comes off HEAD as
it stands at that moment.

The filter rides on the header line under the title — the 🔍 box next to the backlog's name, lit while
it holds the keys — and typing from anywhere lands in it. Under the header sits
a row of action buttons — **Add**, **Edit**, **Send**, **Export**, **Delete**
— each labelled with the chord it stands for, the chord drawn a shade dimmer
than the word so the action is what the eye lands on. `tab` walks the focus
out of the filter and across them (`shift+tab` walks back, `←`/`→` move along
the row, `enter` presses, `esc` returns to the filter); `↑`/`↓` keep moving the
row highlight the whole time, so you can pick a prompt and then press the button
that acts on it. Typing anything hands the focus straight back to the filter. A
button that needs a highlighted prompt is greyed out until there is one.

Both button rows shrink rather than wrap as the pane narrows: the chips give up
their chords first, then their words, then the gaps between them, down to a row
of bare glyphs — `❐ ⚙ ✔ ✉ ✖` in the editor — and the footer names every chord the
chips stopped teaching. At that width it stops being a list of chords and becomes
the legend for the row, each glyph beside the key that presses it, `✉` first. No
button is ever dropped, however narrow the pane; a control that vanishes is one
you cannot learn is there.

The pointer works too, and the same way round: a click on a button presses it, a
click on a prompt selects it (which is what makes the buttons useful with the
mouse — they act on the highlight), a **double-click** on a prompt opens it for
editing, holding the button down and moving **drags the prompt into a new
place in the list** (below), and a **right-click** opens [the row's context
menu](#the-lists-context-menu) — everything the list can do to that prompt, named
in one place. Simply **resting** the pointer on a row floats
[the hover card](#the-hover-card), which reads out the prompt's body and the
session it would launch under without leaving the list. To send one, click the prompt and then the **Send**
button, which opens the drop picker, where a click on a target hands the prompt
over and starts it — a click on a row is the same choice `enter` makes, mode
and all. So a prompt gets from the backlog into an agent without the keyboard,
and never on one stray gesture: it takes a click on the prompt, a click on
**Send**, and then a click on the target you meant. Pausing instead of running
is the one thing the pointer does not offer, because it is a modifier chord.
Mouse reporting is only asked for on the screens with something to click — and
only the list and the Next List ask for idle motion, which is what their hover cards are drawn from;
the prompt view leaves the terminal's own text selection alone.

A backlog longer than the pane **scrolls**, and says so. The list keeps a window
sized to whatever the pane has left once the header, the buttons, the status line
and the footer are paid for, and the highlight pulls that window along as it
walks — so `↑`/`↓` can no longer stroll off the bottom of the screen with nothing
left to say where they went. What the window is not drawing is reported by a
single **▴** or **▾** carrying its count, right-aligned into the first and last
row it *is* drawing: `▴ 5` at the top means five prompts above, `▾ 19` at the
bottom means nineteen below.

The marker spends no line of its own — it shares the row it annotates, and it is
there only while there is something to say — because a pane that has run out of
room is the one place a row of backlog cannot be given up for a row of chrome.
It points both ways for the same reason it carries a number: "there is more" is
the half of the answer you can already see for yourself.

The list's order is the backlog's own running order, and it is yours to set:
**drag a prompt** with the mouse to put it where it belongs — the row takes a
`⠿` grip while you hold it, and the rest of the list parts around it — or nudge
it a step at a time with `ctrl+↑`/`ctrl+↓`. Both stay inside one backlog and one
group: a prompt reorders among its own project's or global's open prompts, and
dropping one on a finished or frozen row does nothing, since those are drawn as
separate groups and the row would land somewhere you can't see. Dragging is also
refused while a filter is on, and says so — a filtered list is in best-match
order, not backlog order, so "put it here" would name a place the file hasn't
got. Clear the filter and the order is real again.

`ctrl+t` marks a prompt **done**, and `ctrl+t` again puts it back — the flip is
reversible in both directions, so a completion pressed by accident costs one key
and nothing else. (On the list's context menu the same row reads **↺ Reopen**
once the prompt is finished.) The highlight rides with the prompt through both
presses — whenever the row is still on screen to follow. Completing one files it
at the top of the done pile, so the row moves a long way, and a cursor left
behind would put your correction on whatever slid up into the gap. The status
line names the way back at the moment it is needed.

Completing a prompt also stamps it with the moment it was finished, in your
local time zone. The row carries it compactly — `done 14:05` today, `done Mon
14:05` this week, `done Sep 3 14:05` earlier in the year, with the year added
once it isn't this one — and the prompt view (`ctrl+v`) spells it out in full,
`done 2026-09-13 14:05 CDT`. The stamp is a record of the completion and nothing
more: reopening or freezing a prompt removes it, completing it again stamps it
afresh, and prompts finished before the stamp existed simply show none. It is
saved as `doneAt` in `todos.json` and only on done prompts, so a backlog with
nothing finished since is byte-for-byte what it was.

Completed prompts collect below the open ones, newest first, so what you just
finished is at the top of the pile rather than the bottom. `ctrl+d` folds them
away and `ctrl+w` clears them out. With that fold on a completed prompt leaves
the list altogether rather than moving down it, so the highlight has nothing to
follow — which is the case the status line's `ctrl+t to reopen` is really for:
`ctrl+d` brings the row back, and the prompt with it.

`ctrl+f` **freezes** a prompt — "will not do". It is deliberately not the same
thing as done: marking work finished that nobody ever did makes the backlog lie,
and deleting it throws away the fact that the decision was made at all. A frozen
prompt is drawn `❄` in the list, dimmed but *not* struck through, and sits in its
own group between the open prompts and the completed ones. `ctrl+d` folds it away
with them, `ctrl+w` leaves it alone, and `ctrl+f` again thaws it — back into the
exact place it held, since freezing never cost it its place. A frozen prompt
also stops going anywhere: any pending auto-drop is cancelled the moment it
freezes, and both `shift+enter` and `ctrl+s` refuse it until it is thawed, so a
decision not to do the work can't be undone by a stray keystroke.

A prompt can also be dropped on a timer: `ctrl+s` asks when (`15:30`, `in 2h`,
`tomorrow 9:00`), then opens the same target picker, and the row carries the
fire time — `◷` and `⏰ 15:30` — until the moment comes. The fire is always
"drop & run" (nobody is standing by to press enter), and it marks the todo done
exactly as a manual drop would. The manager has to be open at the time: firing
is the tick loop of the running TUI, not a daemon. A schedule whose moment
passed while the manager was closed — or whose pane has since disappeared — is
marked **missed** on the row instead of firing late into a conversation that
has moved on; send it by hand from there. `ctrl+s` on a scheduled prompt shows
the time again, where enter on an emptied box clears it.

### Annotations

A row answers three different questions, and they are not the same kind of
question. **What state is this prompt in** — open, scheduled, frozen, done — is
exclusive: a prompt is in exactly one, and the badge that says so has always been
one glyph. **What is true about this prompt** is not exclusive at all: a critical
one-liner is critical *and* cheap *and* still open, all at once.

So the row reads outward from the cursor as state, then annotations, then the
prompt:

```
❯ ○ ▲ 🍏 🔷 fix the drop path     the daemon cannot resolve a bare agent name
  ○ △ context menu grammar        right click across the manager screens
  ○ ⚑ port the export picker      blocked until the api rename lands
  ○ ｉ the api returns 204 on …    a note to file away, not work
  ○ 🍏 bump the version           two files, one number
  ○ 🔷 split the store            pays every time anyone touches it
  ○ ◆ tidy the export picker       worth doing, not urgent
  ○ ordinary work                 nothing said about it
  ❄ shelved idea                  not doing this
  ✓ ▲ shipped it                  done and dusted
```

The badge leads because it is what the list is grouped by — arriving at a row you
want "is this still work" before "how much work". The annotations follow it as
**one compact group**: a row draws the marks it actually wears and nothing else,
in a fixed order among themselves, and then the name starts. A prompt nobody has
annotated spends no cells at all on them.

They were columns once — a reserved slot per mark on every row, blanks included,
so the glyphs could be scanned straight down the pane. That bought the scan by
charging every row for every mark anyone might use, and the bill grows with each
mark added: in a backlog where two rows are marked, every other name sat three
cells right of where it belonged. The marks are few and they lead the row, so
they are found by reading the left edge rather than by their column — the group
that varies in width is the cheaper trade, and it is why the names below are
allowed to be ragged.

Five annotations exist today:

| Mark | Means | Set by |
|---|---|---|
| `▲` `△` | **priority** — critical, high | the editor's **Priority** radios, `--priority` |
| `🍏` | **low-hanging fruit** — a quick win | the editor's **Quick win** checkbox, `--fruit` |
| `🔷` `◆` | **value** — how much it pays: high, medium (low, the default, draws nothing) | the editor's **Value** radios, `--value` |
| `ｉ` | **info** — a note, not work; never sent to an agent | the editor's **Info** checkbox, `--info` |
| `⚑` | **flagged** — singled out, with an optional note saying why | the editor's **Flag** checkbox and its note field, `--flag` |

Freezing is *not* an annotation. It is a state, mutually exclusive with done, and
it stays in the badge (`❄`) where the three groups are read from.

All five are stored as nothing at all when nothing has been said — so a backlog
nobody has annotated is byte-for-byte the file it was before the feature existed,
and a teammate on an older build reads it unchanged.

#### Priority

The list's order says what to do next. It cannot say how much a prompt matters —
once a critical bug and a nice-to-have sit next to each other, the only thing
between them is the order, and every drag churns that. So a prompt also carries a
**priority**:

- **critical** — a solid red `▲`
- **high** — a hollow `△` in cats' soft yellow
- **none** — no mark, and the default

Only raising a prompt leaves a mark. The levels used to be standard / critical /
low, with a dot on every single row — which meant the column could not be scanned
for the rows that actually wanted attention, because every row looked the same,
and "low" never said more than "not raised". Both of those are now the same
answer: say nothing. A backlog still holding `low` from the old scheme reads as
none, keeps its key until something rewrites the todo, and `--priority low` still
works and still means what it meant.

The mark is a triangle rather than a dot so it cannot be confused with the state
badge immediately to its left, which is a circle in every one of its four forms
(`○ ✓ ❄ ◷`) — a shape says "different kind of fact" where a hue alone only says
"different value". The pair escalates by *fill* as well as by colour, hollow to
solid, so the level survives a colourblind reader, a monochrome capture, and a
terminal theme that has flattened the palette. High's yellow is cats' own — the
same `todo` hue the mux paints the paw print it counts your backlog with — so a
row and the workspace badge that counts it are the same colour by construction.
Deliberately *not* the palette's amber, which belongs to the fuzzy-match
highlight inside the row names a few columns to the right.

On a completed or frozen row the mark drops to that row's greys — priority is
about what to do next, and finished work should not be arguing for attention —
but the glyph stays, so the record of what the prompt was rated still reads. A
*scheduled* prompt keeps its colour, because it is still work outstanding.

#### Low-hanging fruit

`🍏` marks a prompt whose payoff is out of proportion to what it costs — the one
worth grabbing while you wait on something else. It answers a different question
from priority (how *cheap*, not how *much*), which is exactly why it is a second
mark rather than a fourth level: a critical one-liner is both, and one mark could
only have told you one of them.

Green rather than red: where a row carries both, the critical mark beside it is
red already, and two reds on a row read as one signal repeated.

On a completed or frozen row the apple **goes away** rather than fading, which is
the one place the two annotations part company. Priority can drop to that row's
greys because a triangle takes a colour; an emoji does not — the font paints it,
a foreground never reaches it, and Unicode has no grey apple to swap in. A mark
that cannot recede would be the one full-colour thing in the tier of the list
that exists to stop shouting, so it stops being drawn: the fruit says "worth
grabbing", and there is nothing to grab on work that is finished or shelved. The
flag itself is untouched — the editor still shows it ticked, `ctrl+v` still
spells it out in words, and unticking **Done** brings the apple straight back.
The row gives the cells back with it: a finished quick win reads like any other
finished row.

#### Value

The value mark is the other half of the estimate the apple starts. The fruit
says how *cheap* a prompt is; the value says how much it *pays*. They are
separate marks because they are separate facts, and either one alone is half an
answer: a five-minute typo fix is cheap and worth almost nothing, a month-long
migration is worth a great deal and will not be picked up between two meetings.
A row wearing the apple **and** a high value is the one to reach for — cheap
*and* valuable — and that is the reading the two marks exist to make possible at
a glance.

Value is a level, one of three:

| Mark | Level | |
|---|---|---|
| `🔷` | **high** | a large payoff for whoever picks it up |
| `◆` | **medium** | worth doing — a solid diamond in straw |
| (nothing) | **low** | the default — a small payoff, or not rated yet |

**Low is the default.** A prompt nobody has rated is a low-value prompt, so
there is no separate "none": an unrated prompt and a low one are the same fact,
and a scale that told them apart would be asking a question with no answer.
Like priority's `none`, the default is stored as nothing and drawn as nothing on
a row, so a backlog nobody has rated costs neither bytes nor cells. Its glyph,
`◇`, the diamond's outline, appears only on the controls (the editor's radio and
the menu row), which have to show what choosing it means.

These are the same three levels the [Next List](#the-next-list) rates its items
on, with the same marks, so a prompt made from a next-list item and one written
by hand answer the question in one vocabulary. The marks are one shape filling
in as the level rises, so they read as one scale rather than as unrelated
glyphs. High is the only emoji: the font paints it, big and saturated, which is
the loudness the top of the scale should have. Medium is text, so the palette
reaches it. (High used to be the `💎` gem, back when value was
one bit. It became the blue diamond so the three steps look like one family.)

It is not a fourth priority level, for the same reason the fruit is not a third.
Priority answers **how much does this matter right now**, and value answers **how
much is it worth at all**; the two come apart in both directions. A refactor that
pays forever but can wait until the release is out is high value and not
critical. A build break that has to clear this morning is critical and worth
nothing once it has cleared. A single scale could only ever have told you one of
them.

On a completed or frozen row the mark **goes away** rather than fading, exactly
as the apple does. The high diamond is an emoji and a grey foreground never
reaches it; medium could fade, but it goes with it, so the done tier does not
look as if only its lesser prompts were rated. And "this one pays" is an
argument for picking work up, which there is none of in the tier of the list
that exists to stop shouting. The fact itself is untouched: the editor still
shows the level chosen, `ctrl+v` still spells out `medium value`, and reopening
the prompt brings the mark straight back.

In the file, high is still stored as `"highValue": true`, the gem's key, medium
uses a new `"value": "medium"` key, and low writes nothing. So a backlog of gems
is byte-for-byte what it was, and an older build still sees every high-value
prompt it saw before; it ignores medium, which it has no mark for.

#### Flag

`⚑` is the open question, where the other three are closed ones. Priority asks
how much a prompt matters, the fruit asks how cheap it is and the value asks what
it is worth; all three have an answer the program can read. The flag says only **there is something about this one** — it
is blocked, it is waiting on an answer, it needs a word before anyone starts —
and what that something *is* goes in the flag's **note**.

That makes it the only mark that carries words, and the note is optional: a bare
`⚑` still means "look at this one", and demanding a sentence would make the mark
cost more than it is worth. Where the note exists it is shown wherever there is
room for a line of prose — hover a flagged row and the card carries it, `ctrl+v`
spells it out on the meta line as `⚑ flagged: blocked on the api rename`, and the
list's context menu prints it beside the checkbox so a decision to rewrite it is
made with the current note in sight. It is also written from there: raising the
flag from the menu opens [a pad for the note](#the-flags-note-where-the-flag-was-raised)
on the row it is about.

The pennant takes the palette's one cool blue, deliberately off the warm ramp the
other two marks sit on. That ramp runs from "ordinary outstanding work" up to
"alarm", and the flag is not a point on it: it is a different kind of claim, and
it should not read as a third loudness. Unlike the apple it is a text glyph, so a
foreground actually reaches it — which is why on a completed or frozen row it
**recedes to that row's greys** rather than going away. A flag is a note to a
reader, and "there was something about this one" is worth as much on finished
work as on open work once it has stopped competing for attention.

The note lives and dies with the mark. Clearing the flag drops the words with it,
in the editor and in the file both, so a backlog never holds a note about a
prompt whose row draws nothing.

#### Info

`ｉ` marks a prompt that is **not work at all** — a note that landed in the
backlog because that is where the hand was: a quirk of an API, a decision and
its reason, a link worth keeping. It belongs in a notes program, and until it is
sent there the backlog holds it without mistaking it for something to do.

On a list row the mark is drawn as a chip — a bold italic white `ｉ` on a solid
blue field, two cells wide like 🍏 and 💎 — so an info row is spotted at a
glance rather than read. On a done or frozen row the field drops away and the
letter goes grey, the way the other marks recede on closed work.

So the mark changes where the prompt may go. **It is never handed to an
agent.** An agent handed a note would try to *do* it, and the whole point of
writing it down as a note was that there was nothing to do. Instead its Send —
`shift+enter`, ✉ Send from the editor, or the context menu's row, which reads
**✉ Send to notes** on an info prompt — files it in the **notes plugin** open in
cats, and marks the prompt done the way a drop does:

- The notes pane is found by what it *is*, not by name: the pane cats reports
  with `plugin_type` `notes_mgr` (the type a plugin declares in its manifest;
  GoNotes declares it). One in this workspace is preferred over one elsewhere.
- The note is pasted into that pane as a small envelope — a
  `<!-- cats-note v1 -->` marker line, YAML frontmatter with the title, a
  `from cats-todo · <project>` description and the `cats-todo` tag, then the
  prompt as the body — and the pane is focused. GoNotes opens it as a new,
  **unsaved** note form; `ctrl+s` there keeps it. The receiving side of the
  contract is `tui/intake.go` in GoNotes.
- A note discarded unsaved leaves a done row here, and `ctrl+t` reopens it.
- With no notes pane open the send is refused, and the refusal names both ways
  out: `no notes plugin open in cats — open GoNotes and send again, or clear ℹ
  Info to send it to an agent`.

`ctrl+s` Schedule still refuses an info prompt, in the same shape —
`that prompt is marked info — a note, not for agents; clear ℹ Info to schedule
it` — and the context menu dims that row with the same sentence. Raising the
mark on a prompt that already has a schedule clears the schedule, the same way
freezing does, and a hand-edited backlog holding both is skipped by the schedule
tick rather than fired or recorded as missed.

It is an annotation rather than a fourth state because it sits alongside the
states instead of replacing one: a note can be done (filed away), frozen (not
worth keeping) or open (not yet collected), and it can still carry a priority. It
draws in the muted secondary-text grey rather than a hue — every coloured mark is
an argument about work, and a note is the absence of one — and, being a text
glyph like the flag, it recedes further on a closed row instead of going away.

#### Where they are set

All five are set in two places, on the same controls. On the editor itself, on a
segmented bar between the title and the prompt body — three checkboxes and two
radio groups, because that is what the five facts are: the fruit is
independent, the value and the priority are each exactly one of three levels, and info and the flag are independent again. And
on the list, without opening anything, from
[the row's context menu](#marking-priority-and-quick-wins-from-the-list) — the
same checkboxes and the same radios, laid out down instead of across.

```
Title
fix the drop path

☐ 🍏 Quick win  │  Value  ( ) ◇ low  ( ) ◆ medium  (•) 🔷 high  │  Priority  (•) none  ( ) △ high  ( ) ▲ critical  │  ☐ ｉ Info  ☑ ⚑ Flag
⚑ note  blocked until the api rename lands

Prompt
…
```

**☑ ⚑ Flag** trails the radios because it is the one segment that is not the whole
of its own answer: ticking it raises the note field on the line below and puts the
caret straight in it, because "flag this, because…" is one thought and a field you
had to go and find would break it in half. Unticking it takes the field and its
words away again. The note is a `tab` stop of the form's ring exactly while the
flag is up, and it is the one stop the walk steps over otherwise — a tab that
appeared to do nothing would be worse than one stop fewer.

The note takes over the blank line that was already there between the bar and the
**Prompt** label rather than being inserted below it. Everything under that line
— the editor, its height, and every click hit-tested against them — is arithmetic
on a layout that must not move, and a field that pushed the form down a row when a
checkbox was ticked would slide the rows out from under the pointer. (The buttons
are safe from that in any case now: they sit on the form's first line, above
everything that can grow.)

The **Value** radios sit immediately beside **☐ 🍏 Quick win** because the two
are one estimate read from both ends, and a hand that has just answered "cheap"
is one `→` away from answering "and worth it".

A thin rule (`│`) divides the bar into its four groups: the fruit, the value
radios, the priority radios, and the two reading marks. On a wide pane the
**Value** and **Priority** labels already say where one group ends. The rule is
there for the narrow panes, where the words are gone: two radio groups side by
side are then a run of six holes, the diamonds and the triangles are both shapes
that fill as they rise, and only the rule says where one question stops and the
next begins. The rule and
the labels are inert, so a click on them presses nothing.

**☐ ｉ Info** sits just before the flag: like the flag it is about how to *read*
the prompt rather than how to rank it, and the flag stays last because it is the
segment that opens something beneath it.

The full bar is 150 cells. On a narrower pane it gives things up in order,
widest tier first:

| Cells | What it gives up | Looks like |
|---|---|---|
| 150 | nothing | `☐ 🍏 Quick win   │   Value   (•) ◇ low   ( ) ◆ medium …` |
| 137 | a cell of each gap | the same words, closer together |
| 104 | the radios' words | `☐ 🍏 Quick win  │  Value  (•) ◇  ( ) ◆  ( ) 🔷  │ …` |
| 84 | the checkboxes' words | `☐ 🍏  │  Value  (•) ◇  ( ) ◆  ( ) 🔷  │  Priority  (•) – …` |
| 67 | the group labels | `☐ 🍏  │  (•) ◇  ( ) ◆  ( ) 🔷  │  (•) –  ( ) △ …` |
| 58 | the space inside each segment | `☐🍏  │  (•)◇  ( )◆  ( )🔷  │ …` |
| 47 | the gaps down to one cell | `☐🍏 │ (•)◇ ( )◆ ( )🔷 │ (•)– ( )△ ( )▲ │ ☐ｉ ☐⚑` |
| 23 | the radios' holes | `☐🍏│◇ ◆ 🔷│– △ ▲│☐ｉ ☐⚑` |

The radios' words go first because their glyphs already say the level; a
checkbox's glyph alone does not say what ticking it claims. The group labels
last down to 84 cells, so a form in the common 100-cell pane still says which
row of holes is Value and which is Priority. The last tier is for the narrowest
pane the form is drawn in, 30 cells: there a radio is just its glyph, and the
chosen one is drawn in reverse, a lit key in a row of unlit ones, so the choice
still shows without relying on colour. The bar never drops a segment and never
wraps, because it sits on a hit-tested row, and a bar that wrapped would put the
prompt editor one line below where every click on it is aimed.

They used to be the first two rows of the ⚙ session panel, above a seam —
accurate, but a screen away: the marks describe **the prompt**, the panel
describes **the session that will read it**, and the one screen where a prompt
is actually written showed neither. Now the mark is made in sight of the title
it qualifies, and each segment carries the glyph its choice will draw on the
list row, so the bar teaches the legend at the moment it is used. `none` is a
hole of its own rather than the absence of one, which makes clearing a level
the same gesture as setting it.

A click presses a segment without taking the keys from whichever field you are
typing in; from the keyboard, `tab` walks the form's ring (title → prompt, and
`shift+tab` from the title wraps round to the bar). Inside the prompt `tab`
indents instead (see *Indenting* below), so a click is the way out of it. On
the bar `←`/`→` move between segments — the one under the
cursor is underlined — while `space` or `enter` presses. The bar joins the
ring *after* the prompt rather than in its visual place between the fields, on
purpose: the gesture this form lives on is "type a title, tab, type the
prompt", and a stop inserted into that walk would spray the prompt's first
keystrokes into a row that is not a text field.

Nothing is written until the form is saved, so an abandoned edit leaves the
marks as they were — the one difference from the context menu, which has no form
to abandon and therefore writes on the press. `ctrl+v` on a list row spells the marks out in words as
well — `▲ critical · 🍏 low-hanging fruit · 🔷 high value · ⚑ flagged: blocked on the api` on the
prompt view's meta line —
which is where to look when a glyph on a row is not yet familiar. That line is
built from the words rather than from the glyphs, so it still reads
`▲ critical · low-hanging fruit · high value` on a finished prompt whose row has
dropped the apple and the diamond: the row says what is worth doing, this screen says what was said.

#### Priority order

Priority does not move anything by itself. The backlog stays in the order you
dragged it into, and the mark is a second axis to read it by rather than a
rearrangement of it. When you do want the rearrangement, `ctrl+l` opens the
**View** panel:

```
View  how this list is drawn — kept between launches

❯ Priority order  off  critical first inside each group — dragging and ctrl+↑/↓ are off while it is on
  Frozen prompts  on   the ❄ rows — work decided against, kept on the record
```

`←`/`→` or `space` flips the switch under the cursor, and both switches are
remembered between launches. **Priority order** lifts the critical prompts to the
top of each group, then the high ones, keeping the hand-set order among prompts
of equal level — it
is a lens over the file and never a rewrite of it, so turning it off gives back
the exact order you had. It sorts *inside* each group and each backlog and never
across them: a finished critical prompt does not climb above open work.

While the lens is on, **dragging and `ctrl+↑/↓` are refused, in words** — the
rows are in an order the file does not have, so "put this one there" names a slot
that does not exist. It is the same refusal a filter already earns, and the
message says which of the two is in the way.

**Frozen prompts** is the other half of a fold that used to be one switch.
`ctrl+d` still hides everything closed — completed and frozen together, the
question being "show me what is left to do" — but hiding the ❄ rows for good is a
standing decision about whether that record is worth its rows, which is a
different question and now has its own switch. That one is remembered; the
`ctrl+d` fold is still per-session.

### Exporting a prompt to another project

A prompt does not always get captured in the right backlog — `cats-todo add`
fired from a shell in one project while thinking about another, or a todo that
turns out to be about the sibling repo. `ctrl+o` (or the bar's **➦ Export**
chip, or `ctrl+o` on the prompt view) opens the **Export to…** picker on the
highlighted prompt, and `enter` **copies** it into the chosen project's own
`.cats-todo/todos.json` — `shift+enter` **moves** it there instead, the same
"modifier does the more committing thing" split the drop picker uses. Both are
done on the spot, and the status line says where it went.

The picker's rows are the places the prompt could go, most likely first:

- **Every workspace cats has open**, labelled with the workspace's name and
  described by the project it is working in and what its backlog holds
  (`3 open`, or `no backlog yet — will be created`). cats' `workspace.list`
  names the workspaces but carries no directory, so where each one is working
  comes from `pane.list` — every pane's live cwd, keyed back to its workspace by
  the `w1:p3` handle. A workspace whose panes are in two different projects (a
  shell cd'd into a sibling repo) gets a row for each. Outside cats these rows
  are simply absent; export still works.
- **The other backlog** of this manager — global for a project prompt, this
  project for a global one — which is otherwise the one move there is no other
  way to make.
- **Recent projects**: directories [cdx](https://github.com/rohanthewiz/cdx)
  has seen you `cd` into lately that already keep a backlog, best-first by its
  own frecency ranking. cdx's state file is read directly, the way cats reads
  it for its path picker; without cdx there is no block.
- **Browse for a folder…**, which opens the same directory browser as `@` in
  the editor, folders only, starting among this project's siblings. It leads
  with a `./` row so the folder you have drilled into is the choice, `tab`/`→`
  open a folder, `backspace` goes up, and `enter` copies / `shift+enter` moves
  to the highlighted folder.

A destination directory finds its backlog the way the manager's own launch
directory does — the nearest ancestor with a `.cats-todo`, else the git root,
else the directory itself — so pointing at a subdirectory reaches the project's
one backlog, and pointing at a project with none yet creates it, exactly as
`cats-todo add` there would. What travels: the title, the prompt, the
attachments (copied into the destination's own `images/`), the session options,
and the open/frozen/done state. What does not: a schedule, which names a pane
and a launch directory of the project it was set in — the status line says so
when one is left behind. A copy takes a fresh id; a move keeps its own. Exporting
a prompt into the backlog it already lives in is refused rather than duplicated.

### Selecting more than one prompt

Every action in the list has always meant "the highlighted row", because every
action was about one prompt: edit it, drop it, freeze it. Sending prompts
somewhere else is the first thing that is naturally about *several* — a handful
of related todos to a colleague, an afternoon's captures to the machine across
the room — so the list has a selection.

`ctrl+space` ticks the highlighted row (`ctrl+b` does the same, for terminals
that swallow the first one). A **✓ column** appears while anything is ticked and
goes away again when nothing is, so a backlog nobody is selecting in looks
exactly as it always did; the header counts what is held (`· 3 selected`).
`ctrl+o` then sends the selection instead of the highlight, and `esc` clears it
— before it clears the filter, and long before it quits, since the selection is
the most consequential state on the screen.

The set is remembered by *prompt*, never by row number. Move a prompt, delete
the one above it, type a filter, fold the closed rows away: the ticks stay on
the prompts they were put on, and a prompt that is hidden by a filter or a fold
is still in the set. A prompt that is deleted leaves it. Actions that cannot
take a set — a drop, a schedule — still mean the highlighted row and say so.
The one that can drop a set is `ctrl+k`: with prompts ticked it opens the
[batch composer](#batches-several-prompts-one-drop) holding them.

## Batches: several prompts, one drop

A drop is one prompt into one place. Five related prompts meant for five fresh
worktrees used to be five trips through the target picker, the same session
options set five times, and nothing afterwards to say the five had gone out
together. A **batch** is those five picked once, put in order, given one setup,
and dropped as a unit, with a record kept of where each one landed.

`ctrl+k` on the list opens the **Batches** page, which lists every batch you
have sent or scheduled; **＋ New** (`ctrl+a` there) opens the composer. With prompts ticked on
the list (`ctrl+space`), `ctrl+k` skips the page and opens a composer already
holding them. The list's right-click menu has **⧉ Add to batch…** (the row plus
anything ticked), and on the [Next List](#the-next-list) page `ctrl+k`, or
**⧉ Batch…** on an item's menu, opens the composer on that page's items.

```
CatsTodo vX.Y.Z - New batch

 Backlog │ Next List  ctrl+g            │ Batch  3 prompts
│ 🔍 filter                  │  6/6     │ ❯  1. Fix flaky drop test △ ⚙ ✱
  ☒ all · 3 of 5 picked · ctrl+a        │ ⠿  2. Rename fuzzyList headings
                                        │ ⠿  3. N-014 Tidy promptsel… ✚
Project                                 │
  ☑ △ Fix flaky drop test               │   order: manual · s sorts A→Z
  ☑ Rename fuzzyList headings           │
  ☐ Add worktree cleanup command        │   Name     nightly cleanup
                                        │   Deliver  ( ) all at once  ( ) one prompt  (•) loop, in order
Global                                  │   Loop     (•) same session  ( ) fresh each
  ☐ ｉ Blog notes · info — a note, not… │   Between  none — e.g. /compact      ☐ after the last too
                                        │   Pause    none — e.g.…
                                        │   Max wait no limit — …
                                        │   On fail  (•) stop  ( ) skip and go on
                                        │   Target   ＋ New Claude Code session on a new worktree
                                        │   Session  ⚙ sonnet
                                        │   When     now
                                        │   ✱ 1 whose own options the batch overrides · the finish is sent once, after the last

   ▶ Drop now alt+enter   ◷ Schedule ctrl+s   ⇅ A→Z   ☰ Batches ctrl+k   ✕ Cancel esc
```

**The left pane is what you can pick.** Its two tabs are the backlog (project
and global, under their own headings) and the Next List, and `ctrl+g` flips
between them — the list's chord for the Next List page, meaning the same thing
here. A **checkbox is membership**: there is no separate "add" step, so the right
pane is simply the ticked rows in the order you ticked them, and the two panes
cannot disagree. `space` (or `enter`, or a click) ticks the highlighted row;
`ctrl+a`, or the **☐ all** line, ticks every row the filter is showing, and a
second press takes them all back out. So "type `docs`, press `ctrl+a`" is every
docs prompt in one move. Because `space` is the checkbox key the filter holds no
spaces, and fuzzy matching never needs one.

Only open prompts are listed. Done and frozen ones are closed work — and a done
pile can be hundreds of rows — so offering them only to refuse them would bury
the rows you came for. An **info** prompt is listed but greyed, with the reason on
its row, because it is exactly the one you would go looking for and wonder
about: it is a note, and an agent handed a note would try to do it. A prompt
that already has its own schedule can be picked, and its row shows the `◷` time,
so the double booking is visible before the batch goes.

**The right pane is the batch, in delivery order.** Drag a row by its `⠿`, or
`alt+↑/↓` (the editor's line-move chord) with the pane focused; `x` or
`delete` takes a row out (and unticks it on the left). **⇅ A→Z** — `s` in the
pane — sorts once by title. It is an action rather than a view: what it leaves is
an ordinary order you can keep dragging, because a batch's order is what gets
delivered and has to be something you can see and edit. The marks after a title
are the prompt's own (priority, value), then `⚙` when it has session options of
its own, `✱` when the batch's options override some of them, and `✚` for a Next
List item that becomes a backlog prompt when the batch drops.

`tab` walks the regions — pick, batch, settings, buttons — and the footer
names the keys of whichever holds them. Below 100 columns the two panes take
turns rather than sharing the width (a switcher line says which is up, and
`tab` or a click moves between them); side by side at that size every title
would be cut to a stub.

**The settings** are five rows under the batch (and five more under Deliver
while it says loop — see [Looping a batch](#looping-a-batch)):

- **Name** — optional. An unnamed batch is listed as its first prompt's title
  and a count (`Fix flaky drop test +2`).
- **Deliver** — how the prompts reach the agents (`←/→` or a click). A new
  batch starts on **loop, in order**, in the same session: it is the one mode
  that takes any target and any number of prompts, and never sets two of them
  working on the tree at once, so the careful choice is the one already made.
  The other two are a press away when the prompts really are independent. A
  batch duplicated or edited from the Batches page keeps its own mode.
  - **all at once** — each prompt gets its own new session, or its own
    worktree when the target is a worktree row. This is the case
    [worktree drops](#dropping-onto-a-new-worktree) were built for: several jobs
    in parallel without any of them editing another's files. A running pane is
    one conversation, so it is refused as a target here (in words, naming the
    other two ways) once there is more than one prompt.
  - **one prompt** (listed) — the prompts joined into one body, each under a
    numbered `## 1. <title>` heading in batch order, after a line telling the
    agent they are separate tasks to take in order. One drop, into any target.
  - **loop, in order** — one prompt at a time, each sent only when the one
    before it has *finished*: for work that has to happen in sequence, or must
    not run twice at once. See [Looping a batch](#looping-a-batch).
- **Target** — `enter` opens the ordinary target picker, so a batch's target is
  chosen from exactly the rows a single drop offers (new session, new
  worktree, running panes). It starts on a new Claude Code session, so a batch
  can go without the trip.
- **Session** — `enter`, or `ctrl+r` from anywhere in the composer (the
  editor's chord for the same panel), opens the [session options](#session-options)
  panel on the batch's own options.
- **When** — empty means now. Type a time and the batch is for later: the same
  forms the list's scheduler reads (`15:30`, `in 2h`, `tomorrow 9:00`,
  `2026-09-26 09:00`), with what it comes to shown beside it as you type
  (`→ Sat 09:00`), since "tomorrow 9:00" is easy to write and easy to misjudge.
  See [Scheduling a batch](#scheduling-a-batch).

**The batch's options win, field by field.** Any option the batch sets replaces
the prompt's own; any it leaves at the default falls through to the prompt. So
"every prompt on sonnet" costs one prompt nothing of its own `/sess-use`
pattern, and a context mode travels with its argument (the prompt's argument was
written for the prompt's mode). The two yes/no options can be turned *on* for
every prompt but not *off* for one that asked for them, since "no" and "not set"
are the same value. A one-prompt-listed drop is one session and so has one
setup: only the batch's options apply, and every prompt with options of its own
wears `✱` to say they won't. The line under the settings counts the `✱` rows.

**▶ Drop now** (`shift+enter`/`alt+enter`, the list's drop chord) sends it, and
**◷ Schedule** (`ctrl+s`, the list's schedule chord; or `enter` on the When
row) saves it for the time on the When row. The When row decides which one
applies: with it empty, Schedule is greyed; with a time on it, Drop now is.
Pressing the greyed one says why rather than quietly doing the other thing — a
batch dropped now when the row said 3am, or scheduled for a time you forgot you
typed, is the surprise the grey is there to prevent. Leaving with picks on the
table — `esc`, **✕ Cancel**, **☰ Batches** — takes a second press: the first
says the picks are not saved until the batch is dropped or scheduled.

### What a drop of a batch does

This is *all at once* and *one prompt*; a loop is
[its own thing](#looping-a-batch). The prompts go **one after another**, each an ordinary drop — the same prompt
composition, agent-ready wait and worktree cut a single drop does. "All at once"
means *without waiting for any of them to finish their work*, not
simultaneously: two drops typing into panes at the same moment is how prompts get
garbled, so the batch holds the manager's one-drop-at-a-time guard for its whole
run (which also keeps a scheduled drop, `ctrl+s`, from firing into the
middle of it). The status line counts it through: `batch nightly: dropping 2/3 →
…`.

Picked Next List items are saved as backlog prompts at that moment — or, when the
backlog already holds an open copy of the item, that copy is used — so every
prompt in a batch is a backlog prompt, and nothing is written anywhere by a
composer you leave with `esc`.

Each prompt that lands is **marked done**, as a single drop marks its prompt. A
failed one stays open, and the rest still go: in "all at once" the prompts are
independent by construction, so one branch that could not be cut is no reason
to hold back the others. Every prompt is re-read from its backlog as the batch
goes, and one frozen, completed or deleted in another pane meanwhile is not sent;
the record says so.

### Looping a batch

The other two modes are fire-and-forget: every prompt is typed in, and the batch
is done. A **loop** waits. It sends prompt 1, watches its pane until the agent
has finished, then sends prompt 2 — into the same conversation, or a fresh one —
and so on to the end. It is for a sequence: each step builds on the last, or
two of them must not be editing the tree at the same time.

```
  Deliver  ( ) all at once  ( ) one prompt  (•) loop, in order
  Loop     (•) same session  ( ) fresh each
❯ Between  /compact                  ☐ after the last too
  Pause    30s           before each next prompt
  Max wait 2h            per prompt, then it counts as failed
  On fail  (•) stop  ( ) skip and go on
```

**"Finished" is working, then idle.** cats reads each agent pane's state off
its screen — the `[working]` the drop picker shows — and the manager asks for it
once a second. A prompt is finished when its pane is seen *working* and then
*idle*. It has to be seen working first: an agent still idle a moment after the
prompt landed hasn't started on it, and taking that for done would send the next
prompt on top of it. An agent asking a question (*blocked*) is still working —
it is waiting on you, not done. A prompt that is never seen working within 45
seconds counts as failed (the likely story is that it was never submitted, and
sending the next one would glue the two together), as does a pane that closes,
or one whose agent has exited.

- **Loop** — **same session** (the default) sends every prompt into the pane the
  first one opened, or into the running pane chosen as the target: a sequence
  usually builds on what came before, and one conversation keeps that context.
  Each prompt after the first is a drop into a running pane, so its own
  `/clear`, `/model` and `/effort` are applied there as a single drop would
  apply them. **fresh each** opens a new session (or a new worktree) per
  prompt, each only once the one before is idle — for steps that must not see
  each other's context but still must not overlap. It needs a new-session
  target, and says so otherwise.
- **Between** — one line submitted after a prompt finishes and before the next
  goes: a slash command (`/compact`, `/clear`, `/sess-save step`,
  `/code-review`) or plain words ("run the tests and fix anything red"). It is
  waited on like a prompt. Some commands make the agent work and some return at
  once, and one rule covers both: a pane that doesn't show *working* within
  three seconds of the command counts it as an instant one. With **fresh each**
  it goes to the session that just finished, which is where a `/sess-save` or a
  `/code-review` has something to act on. One line only — a second line would be
  a second message the loop doesn't know to wait for. **☐ after the last too**
  (`ctrl+t` on the row, or a click) runs it once more at the end; off by
  default, since `/compact` after the final step is wasted work while
  `/sess-save` after it is often the point.
- **Pause** — a wait before each next prompt (`30s`, `5m`, `1h30m`).
- **Max wait** — how long one prompt (or the between command) may run before it
  counts as stuck, which is a failure. Empty is no limit.
- **On fail** — **stop** (the default: a sequence usually means later steps
  depend on earlier ones) ends the loop there, and the rest stay open on the
  list. **skip and go on** records the failure and sends the next. Two failures
  stop the loop either way: in the same session, a closed pane leaves nowhere
  for the next prompt to go; and the finish message is the last thing a loop
  sends.

**The finish runs once, in the same session.** Commit, push, wrap and the
release (the [session options](#session-options)' Finish) would otherwise be one
commit per step of a single conversation. So in a same-session loop they are
lifted out of every prompt and sent once, after the last prompt (and after the
between command when it runs after the last) — the batch's Finish if it sets
one, else the last prompt's own. With fresh sessions each prompt is its own
conversation and keeps its own.

**It holds the keyboard only while it types.** The waits can be hours, and the
manager's one-drop-at-a-time guard is taken only for the seconds a prompt, a
between command or the finish takes to type in. Single drops, schedules and
other batches go on meanwhile — they just take turns at the keyboard — and
several loops can run at once. Each prompt is re-read from its backlog as its
turn comes, so one completed, frozen or deleted in the meantime is skipped with
the reason. Each prompt that lands is marked done, as a single drop's is; the
list marks the ones still waiting their turn `⧉ queued`.

**It survives the manager closing.** The loop runs in the manager, like a
schedule, so the manager has to be open for it to move — but where it stands is
written to `batches.json` at every step: which prompt is next, what it is
waiting on, the pane it is watching, and which manager is driving it. Close the
manager mid-loop and the batch reads `‖ paused at 2/5`; open one on the same
backlog and it takes the loop over and carries on from there. The prompt it was
waiting on is looked at afresh — idle counts as finished, since nobody watched
it in between — and the next prompt's number is written *before* the prompt is
sent, so a manager that dies at the wrong moment can leave a prompt unsent but
never sends one twice. A prompt that was mid-send when the manager went is
marked on the record as unknown ("look in its pane"), not sent again.

**■ Stop** (`ctrl+u` on the page, where the Unschedule chip turns into it for a
running loop) ends a loop: nothing more is sent. What is already running in its
pane carries on — nothing on the wire can interrupt an agent, and the pane is
yours to stop. A loop driven by a manager in another pane is stopped through its
record: every write the driver makes is checked against the record as it last
wrote it, so it finds the stop at its next step and lets go before typing
anything more. The same check is what lets only one of two managers take over
an orphaned loop.

### Scheduling a batch

A scheduled batch is written to `batches.json` and sends nothing until its
time. The manager fires it from the same once-a-second tick that fires a
prompt's own schedule, by the same rules:

- **It fires only on time.** A tick up to two minutes late still fires it.
  Later than that, the batch is marked **missed** instead, with the reason, and
  the status line says so. Opening the manager should never set off a batch of
  agent runs planned for hours ago; a missed batch waits on the Batches page to
  be rescheduled or dropped by hand.
- **One drop at a time.** While a drop, a scheduled prompt or another batch is
  in flight, a due batch waits for a later tick, still inside its two minutes.
- **Claimed before it fires.** The record is switched from scheduled to running
  on disk before anything is sent, and only if it still reads as it did. Two
  manager panes open on the same backlog can both see the batch come due; the
  one that switches it first sends it, and the other finds it running and
  stands down.
- **Read fresh.** The backlogs are re-read at fire time, so a prompt completed,
  frozen or deleted in another pane since the batch was scheduled is skipped,
  with the reason on the record. A running-pane target is checked to still
  exist before anything is typed into it: a pane chosen hours ago may be gone,
  and its number is no promise about what now holds it.

The manager has to be open for a batch to fire, as for any schedule. Every
running manager fires the global backlog's batches; a project's batches fire
from a manager in that project.

**The list shows what is spoken for.** A prompt sitting in a scheduled batch
wears `⧉ 09:00` on its row, beside where a prompt's own schedule shows
`⏰ 09:00`. Without it the prompt looks free, and dropping it by hand now means
the batch skips it later — a double booking better seen before it happens.

**A batch that hasn't gone is still a plan.** On the Batches page, `enter` on a
scheduled, missed or unscheduled batch opens it in the composer (**Edit
batch**) with its prompts, its settings and its time on the When row. Change
anything, then **◷ Schedule** to save it back over itself, or clear the When row
and **▶ Drop now**. While it is open there, this manager holds off firing it.
The save is checked against the record as it was opened: if another pane fired,
edited or deleted the batch meanwhile, the composer says so rather than writing
over it or sending it twice. **✕ Unschedule** (`ctrl+u`) takes a scheduled batch
off the clock but keeps it, marked `◌ not scheduled`. Deleting is the separate,
two-press button, so taking a batch off the clock doesn't also throw away the
picking and ordering.

### The Batches page and the record

```
 Batches   4 batches

│ 🔍 type to filter                      │  4/4

   ＋ New ctrl+a   ⧉ Duplicate ctrl+d   ✕ Unschedule ctrl+u   ✖ Delete ctrl+x   ← Back esc

❯ ◷ nightly cleanup  3 prompts · all at once · New Claude Code session on a new worktree · fires Sat 09:00
  ✓ Global task      1 prompt · one prompt, listed · New Claude Code session · 13:08 · 1/1
  ⚠ quick wins       3 prompts · all at once · New Claude Code session · Thu 11:08 · 2/3
  ◷ docs sweep       2 prompts · all at once · New Claude Code session · missed Wed 09:00
```

A row's badge is where the batch stands: `◷` scheduled (red once it has
missed), `◌` not scheduled, `▶` still going, `⟳` a loop running (`‖` when no
manager is driving it), `■` a loop stopped part-way, `✓` every prompt landed,
`⚠` some did, `✗` none did. A running loop's row says where it is (`on 2/5`,
`pausing after 2/5`, `finishing`). `enter` (or a double-click) opens a batch
that hasn't gone in the composer, as above, and one that has gone as its record
— each prompt, where it landed (the pane, and the branch for a worktree drop, so
an all-at-once batch can be traced to its checkouts), and the error beside any
that didn't. A loop's record also says how it was set up, where it stands, and
why it stopped; a prompt it gave up waiting on shows `⚠` with the reason — it
was delivered, and stays counted as delivered. **⧉ Duplicate**
(there or on the page) opens a composer with the batch's settings and whichever
of its prompts are **still open**, which after a partial drop is exactly the
ones that didn't land; the ones that did are done, and reopening them on the list
(`ctrl+t`) is how a finished batch is run again. **✖ Delete** removes the record
(never the prompts) on a second press.

The records are kept in `batches.json` beside `todos.json` — the project's
`.cats-todo/` when the batch holds a project prompt, the global config
directory otherwise. A file of its own, rather than a key in `todos.json`,
because a batch can mix project and global prompts and so belongs to neither
backlog, and because it leaves `todos.json` byte-identical for everyone who
never makes a batch. The file keeps creation order. The page sorts: a batch
still going on top, then the scheduled ones soonest first (what happens next),
then everything else newest first.

## Bundles: disk, email, and the machine across the room

Exporting into another project writes straight into that project's
`todos.json`, which works because both ends are directories this process can
open. Everywhere else a prompt might go — a file to pick up later, a mail
message, another machine — needs the prompts to become something that stands on
its own. That is a **bundle**.

A bundle is deliberately close to a backlog: a small envelope (schema version,
when, who wrote it, which backlog it left) wrapped around the prompts in
exactly the JSON `todos.json` uses. That is the whole compatibility story — the
same additive rule the backlog format keeps means a bundle written by a newer
cats-todo loses only fields an older one never knew about. Two containers,
chosen for you:

- `<project>-<date>.catstodo.json` — the manifest alone, when nothing is
  attached. A file you can read, diff, or paste into a message.
- `<project>-<date>.catstodo.zip` — `manifest.json` plus `images/`, when at
  least one prompt carries an attachment. The paths inside are the same strings
  the backlog stores, so nothing is rewritten at either end.

**A schedule never travels.** It names a pane id and a launch directory of the
machine being left, and a prompt that fired itself into a stranger's session
would be worse than one that quietly needs re-scheduling. The status line says
how many were left behind.

The `ctrl+o` picker's rows are in two blocks now. Above, the backlogs on this
machine, exactly as before. Below an **Off this machine** heading:

- **Save a bundle to disk…** — the folder browser again, opening on Downloads
  (a bundle is a file on its way somewhere), and the status line names what was
  written. Nothing is ever overwritten.
- **Email — prompts in the message body** — your mail client opens with the
  prompts written out as markdown. There is no SMTP server to configure here
  and no password for this tool to keep: it hands a `mailto:` link to the
  machine, which already knows how you send mail.
- **Email — with a bundle file** — the same composer, and the bundle written to
  disk and shown in your file manager to drag in. A `mailto:` URL *cannot*
  carry an attachment; rather than pretend otherwise, this does the two halves
  it can do and says so.
- **The machines on your local network**, and **Enter a host…** for one that
  discovery missed.

Everything in that second block is a copy by construction — there is no backlog
at the far end of a file or a message to have moved a prompt *into* — so the
`shift+enter` move chord is refused there in words rather than deleting your
only copy.

Inside the picker, `ctrl+a` widens what is being sent to **everything in this
backlog**, done and frozen rows included (a backlog handed to another machine is
a record, and dropping the finished half would make the copy a worse record than
the original). `ctrl+a` again puts it back. The heading always says what is
about to travel.

In the markdown (the email body, and `export --markdown`), each prompt gets
one italic line with its state and marks, and a prompt with neither gets no
line at all. A done prompt says when it was finished, as the prompt view does:
`done 2026-09-24 14:05 CDT`, with the zone included because the reader may be
in a different one. A prompt finished before the stamp existed says just
`done`.

## Importing

`ctrl+r` opens **Import from…**: a bundle file on disk, or a machine on the
local network. The disk row browses with everything that is not a bundle
filtered out — a downloads folder holds hundreds of files and exactly one of
them can be imported.

Whatever the source, the bundle is read *before* anything is written and what
would happen goes on screen first:

```
Import

  12 prompts from ~/Downloads/studio-2026-09-02.catstodo.zip
  written by cats-todo v0.28.0 on studio.local
  → the project backlog · 9 prompts new · 3 already here, skipped

y import · tab other backlog · n / esc cancel
```

`tab` sends it to the other backlog instead, re-counting as it goes. Imported
prompts take **fresh ids** — an import is new work in *this* backlog, with its
own life from here — and a prompt whose title and text this backlog already
holds is skipped, because the common mistake is importing the same bundle
twice. A prompt whose attachment cannot be brought across still lands, without
it, and is counted: the text is the part with the value.

## The Next List

A project that keeps a living list of follow-ups — `ai_docs/todo/next-list.md`,
the file the `/next-list` and `/sess-save` skills maintain — can start prompts
straight from it. `ctrl+g`, or the list bar's **» Next List** chip, opens it as a
page of its own:

```
Next list  ai_docs/todo/next-list.md · 24 open · 3 roadmap

│ 🔍 type to filter                  │  27/27

  ✚ New prompt enter   ✉ Send shift+enter   ↻ Refresh ctrl+r   ← Back esc

Open
❯ N-001 ◆  Hands-on pass in a rebuilt, reinstalled Cats.app. Sessions run inside Cats.app, where G…
  N-003    A plugin started by hand from a shell (e.g. `cats-todo` typed at a prompt) has no `CATS_P…
  N-017 🔷 …
Roadmap
  N-019    …
```

Only **Open** and **Roadmap** are listed. Non-goals and Closed hold items too,
but nothing in them is waiting to be started.

A row here is laid out differently from a backlog row. It is **not** split
into a title and a dimmer body, because a next-list item has no title: its
first sentence is just the start of a paragraph. Instead the row is the item's
ID followed by its own text, flattened onto one line and cut only where the
pane ends. Between the ID and the text is a mark for the item's value, the
backlog's own [value marks](#value), since it is the same fact on the same four
levels:

| Mark | Value | |
|---|---|---|
| 🔷 | high | the blue diamond, an emoji that paints itself |
| ◆ | medium | a solid diamond in straw |
| (blank) | low | the default, and what an unrated item is, drawn as a backlog row draws it |

Unlike a backlog row's packed marks, each mark here takes the same two cells, so the
text starts in the same column on every row. The ID's colour follows the value
too (yellow, straw, grey), but that alone was too subtle to read the value from.
Typing filters across the whole item, including text
past the edge of the row, as well as the ID, the value and the section name,
so `N-014`, `high` and `roadmap` all work as queries.

`enter` (or a double-click, or **✚ New prompt**) opens the **add** form
already holding the item. The title is its ID and opening words. The prompt
starts with `Next list item N-014 (ai_docs/todo/next-list.md):` and is followed
by the item's text as written, sub-bullets included, so the agent that gets it
knows which item it is working on and can close it in the file. Nothing is
written until you save; `esc` there throws the draft away.

`shift+enter` (or `alt+enter`, or **✉ Send**) sends the item straight to an
agent instead. It is the list's own drop chord, and it opens the same target
picker a backlog prompt gets: a new session, a new session on a fresh
worktree, or a running agent pane, with `enter` to run and `shift+enter` to
paste and pause. The prompt is the same one the form would have been
prefilled with, citation and all, and a new session opens in the project the
list belongs to.

A sent item is **not** saved to a backlog, which is where this differs from
the form's ✉ Send (save, then drop). The item already has a home in the file,
and a backlog copy would be a second record of the same work, marked done
after the drop and never looked at again. It would also stay behind if you
backed out of the picker without sending anything. Closing the item is left to
the file: the agent that did the work, or the next session wrap-up, moves it
to Closed. `esc` in the picker comes back to this page, with the highlight
still on the item. The outcome (`N-014 dropped → …`, or why it failed) is shown
on the page's heading and in the list's status line, since a slow new-session
drop may land after you have left the page. Without a cats control socket the
page says so and stays put.

The page reads the file when it opens and again on **↻ Refresh**
(`ctrl+r`). It does not watch the file: the list is usually edited in another
pane by a session that is wrapping up, and a refresh you ask for won't reshuffle
the rows while you're moving through them. A refresh keeps the query and keeps
the highlight on the item it was on. On this page `ctrl+r` means refresh rather
than the list's import, because there is nothing to import into here.

A project without the file still opens the page. The page names the missing
file and says that `/next-list seed` creates it. The list is found beside the
project backlog (the directory holding `.cats-todo/`). A `--global` launch
inside a project reads that project's list, and a prompt made from it goes into
the global backlog, since that is the only backlog the launch manages.

**Rest the pointer on an item** and it gets [the hover card](#the-hover-card)
the backlog's rows have, with the same wait, the same box, and the same
rules for what takes it down. A row can only show the start of the item's
text, flattened, and never shows the header's fields, so the card fills in the
rest:

```
╭────────────────────────────────────────────────────────────╮
│ N-001 · Open                                               │
│ Hands-on pass in a rebuilt, reinstalled Cats.app. Sessions │
│ run inside Cats.app, where GUI launches get a minimal      │
│ PATH. Merged from the hands-on checks:                     │
│ - hover cards: the 400ms dwell, the 800ms warm window;     │
│ - DEC 1004: blur a window with a card up…                  │
│ value medium · raised 2026-0904-1753-a-dwell               │
╰────────────────────────────────────────────────────────────╯
```

The card is capped at **seven rows**: the ID and its section, up to five lines
of text with the item's line breaks and sub-bullets kept, and one line of
fields. A longer item ends in an ellipsis, and `enter` opens the whole item in
the form. Unlike a backlog prompt's card, every item gets one, even one whose
text fits on the row, because when it was raised and its value in words are
things the row never shows. The page asks the terminal for all pointer motion
for this, the same cost the list pays.

**Right-click an item** for its context menu. It uses the same box and keys as
[the list's](#the-lists-context-menu). A Next List item is not a backlog
prompt: it is a paragraph in a file this page only reads. So everything the add
form can set (session options, attachments, marks, a schedule) belongs to a
backlog prompt, and none of it can be applied to the item itself. What the menu
does instead is make that prompt from the item, with the setting already
applied:

```
╭────────────────────────────────╮
│ ✚ New prompt…            enter │
│ ⚙ Session…                     │
│ ◫ Images…                      │
│ ✉ Send…            shift+enter │
│ ◷ Schedule…                    │
│ ⧉ Batch…                ctrl+k │
│ ⤓ Add to backlog               │
│ ⤓ Add as 🍏 quick win          │
│ ⤓ Add as △ high priority       │
│ ⤓ Add as ▲ critical priority   │
│ ⤓ Add as ｉ info               │
│ ⤓ Add as ⚑ flagged             │
│ ⧉ Copy ID: N-014               │
│ ⧉ Copy as prompt               │
╰────────────────────────────────╯
```

The rows run from least to most committing:

- **✚ New prompt…** opens the draft form, the same as `enter`. **⚙ Session…**
  and **◫ Images…** open the same draft with the form's session panel or
  attachments editor already up, so `esc` from the panel lands on the draft.
  Nothing is saved until the form is.
- **✉ Send…** hands the item to an agent without saving it, the same as
  `shift+enter`.
- **◷ Schedule…** saves the item to the backlog and opens the list's scheduler
  on it. A schedule belongs to a backlog row, so this is the one row that
  leaves the page. Backing out of the scheduler leaves the prompt in the
  backlog.
- **⤓ Add to backlog** saves the item in one press, with no form, and stays on
  the page. The rows under it save it with one mark already set. The heading
  confirms where it went (`added N-014 as a quick win to the project backlog`).
- **⧉ Copy ID** and **⧉ Copy as prompt** are the only rows that leave nothing
  behind. **Copy ID** puts the bare ID on the clipboard, ready to cite in a
  commit or a chat. **Copy as prompt** copies the exact text ✉ Send would
  deliver, citation line included, so you can paste it into an agent this
  manager can't reach. Both use the list's copy path (OSC 52, plus `pbcopy` on
  a Mac).

Every prompt made from an item carries the item's value, whether it comes from
the form or from one of the Add rows. The file rates items on the same three
levels a prompt is rated on, so a `value high` item becomes a 🔷 prompt rather
than a low one. The form made from an item is titled **Next List Prompt
Editor**, so you can tell where the draft came from once the page is out of
sight.

The Add rows are greyed out once the backlog already has an **open** copy of
the item, meaning a prompt that still begins with the item's citation line.
Pressing one then names the copy. ◷ Schedule… schedules that copy instead of
making another. A copy that is done doesn't count: the work was closed but the
item is still listed, so adding it again is a reasonable thing to do. Rows that
can't act right now are greyed out too, and say why on the heading, in the
same words the chord uses. That covers Send and Schedule without a cats socket
(or Send while a drop is in progress), and every row that writes when there is
no backlog to write into.

↻ Refresh and ← Back are not on the menu. They act on the page, not on an
item, so they stay on the bar, just as the list's menu leaves out Import. The
page never writes the file, so closing or re-rating an item is still done in
the file itself. As on the list, a right-click anywhere but an item opens
nothing (and closes a menu that is open), a click off the box closes it
without doing anything else, and the hover card stays down while the menu is
up.

## Sending to a machine on the local network

cats' control socket is a unix socket — it reaches the cats on *this* machine
and nothing else. So the box on the other side of the desk needs a service of
its own:

```
$ cats-todo serve --name studio
cats-todo v0.28.0 serving on [::]:8422
  project  cats-todo (7 open)
  global   global (2 open)
  inbox    the project backlog
  token    ca7d0e81b137a28cf8f27f6cc7275bf1
  the machine sending to this one needs that token in its settings.json (peerToken)
```

The other machine's export and import pickers then list it by name, usually
before you have finished reading the screen: a manager opening a picker asks a
multicast group who is there and every server answers directly. Asking rather
than announcing is what makes it quick *and* quiet — no chatter on the network
for a screen nobody has open. A machine discovery cannot reach (another subnet,
multicast filtered) is reached with **Enter a host…** and remembered afterwards,
and it keeps a row even while it is asleep, saying so, because "the studio is
not answering" is a more useful screen than an empty list.

Three rules hold the service up, and each is a refusal rather than a warning:

1. **A token is required.** It is generated on the first `serve`, printed every
   time, and lives in `~/.config/cats-todo/settings.json` as `peerToken`; the
   sending machine needs the same string. A `serve` with no token refuses to
   start rather than opening a port that is a stranger's write access to your
   backlog.
2. **The local network only.** A request from outside this machine's own
   private ranges is refused — that is the whole feature — with
   `--allow-remote` there for someone who has deliberately tunnelled in.
3. **Nothing that arrives is ever run.** A bundle becomes rows in a backlog.
   Schedules are stripped on the way in, attachment names are reduced to a bare
   file name, sizes are capped. Getting a prompt into someone's list is not the
   same as getting it into their agent, and the distance between the two is a
   keystroke they make themselves.

`--inbox project|global` chooses where arriving prompts land, `--port` and
`--name` are remembered in the same settings file, and the sender's status line
is the *receiver's* own sentence — what it says landed is what actually landed.

### The same three from a shell

```
cats-todo export [-g] [--all] [--out DIR] [--markdown] [--to HOST] [--mail]
cats-todo import [-g] [--keep-ids] [--allow-duplicates] <file|directory|host>
cats-todo serve  [--port N] [--name LABEL] [--inbox project|global] [--allow-remote]
```

`export` takes the open prompts of a backlog unless `--all`; with no
destination flag it writes a bundle into `--out` (or the current directory).
`import` tells its argument apart by looking: a path that exists is a file (or a
directory holding exactly one bundle — two is a question, not a guess), and
anything else is a machine.

## The hover card

A list row is one line, so it shows the prompt's first line and nothing else.
Everything that decides whether *this* is the prompt to send right now — the
rest of the body, and the model and effort a drop will run it under — was behind
`ctrl+v` or the edit form, which is a screen change to answer "what is this one
again?".

**Rest the pointer on a row** and the card says it in place:

```
╭──────────────────────────────────────────────────╮
│ Fix the drop timeout                             │
│ The 12s wait comes from stale ready probes in    │
│ client.go — capture a startup and re-check       │
│ claudeReadyProbes before touching anything else, │
│ then re-run the drop tests…                      │
│                                                  │
│ Model   claude-opus-5                            │
│ Effort  high                                     │
╰──────────────────────────────────────────────────╯
```

It is [cats' own pane hover card](https://github.com/rohanthewiz/cats) brought to
the TUI, and it keeps that card's rule: a field with nothing in it drops its row,
so the card is as tall as the prompt has things to say rather than a fixed form
with blanks in it. A prompt with no session options gets no **Model**/**Effort**
rows; a prompt whose body is only the line the row is already showing gets no
card at all, rather than a bordered box repeating it back at you.

A **done** prompt's card leads its fields with when it was finished, spelled out
in full the way the prompt view prints it: `Done    2026-09-13 18:13 CDT`. The row
has its own `done 18:13`, but that is the row's last mark, so in a narrow pane it
is the first thing cut off. It also drops the date within the week and never
shows the zone. The card is where the pointer already is, so it is the one place
in the list that always has room for the whole stamp. A prompt finished before
stamps existed has none, and its card gets no row for it.

Four lines of body is the reading budget. A longer prompt ends in an ellipsis,
which is the invitation to press `ctrl+v` — the card is a glance, not the prompt
view with a border on it. It lands below and right of the pointer and flips or
pulls back inside the pane at the edges, exactly as a context menu does, and for
the same reason: it leaves the row it is about visible rather than covering it.

The card belongs to the *row*, not to the cell, so drifting across the same row
leaves the box exactly where it was. It is taken down by the next thing the hand
does — a keystroke, a click, a resize, or the pointer moving onto a heading, a
button or the empty space below the list. Nothing on it can be pressed; while
[the context menu](#the-lists-context-menu) is up or a row is being dragged, no
card is built at all, because those gestures already own the pointer.

The one cost is that the list asks the terminal to report *all* pointer motion
rather than only motion under a held button. That is a message per cell the
pointer crosses, and it is paid only on the list and on
[the Next List](#the-next-list), which has a card of its own. The prompt view,
the one screen whose text gets copied out, still claims no mouse at all.

## The list's context menu

The list can do a dozen things to a prompt and the button bar has room for five,
so most of them have only ever been chords — and a chord is not something a
pointer can find. **Right-click a row** and all of them are named in one place,
on the prompt you pointed at.

```
╭─────────────────────────────────╮
│ ✎ Edit…                   enter │
│ ⚙ Session…                      │
│ ✉ Send…             shift+enter │
│ ◷ Schedule…              ctrl+s │
│ ✓ Mark done              ctrl+t │
│ ❄ Freeze                 ctrl+f │
│ ☐ 🍏 Quick win                  │
│ (•) Value: ◇ low                │
│ ( ) Value: ◆ medium             │
│ ( ) Value: 🔷 high              │
│ (•) Priority: none              │
│ ( ) Priority: △ high            │
│ ( ) Priority: ▲ critical        │
│ ☐ ｉ Info (a note)              │
│ ☑ ⚑ Flag: blocked on the api    │
│ ✎ Edit flag note…               │
│ ✓ Select             ctrl+space │
│ ➦ Export…                ctrl+o │
│ ⧉ Add to batch…          ctrl+k │
│ ✖ Delete…                ctrl+x │
╰─────────────────────────────────╯
```

The press moves the highlight onto the row first, so what the menu acts on is
what the keyboard is parked on when it hands control back — and it takes no hold
for a drag and does not count as half of a double-click, which are gestures the
left button makes. Only a row opens one: a right-click on the header, the button
bar or the empty space below the list opens nothing, and takes down a menu that
is up.

Every row that has a chord prints it, so the menu doubles as the keyboard's own
reference. Rows that name a state say what pressing them will do — **✓ Mark
done** reads **↺ Reopen** on a finished prompt, **❄ Freeze** reads **☀ Unfreeze**
on a shelved one. So a prompt closed by accident is reopened from the same menu,
on the row that now offers exactly that. A row that cannot act right now is drawn **dim and still
there** and says why when you press it, in the same words the chord uses: sending
a frozen prompt or an [info](#info) note, or scheduling one with no cats socket. Everything else about
the box — `↑`/`↓` and `enter`, a click off it to dismiss, any other key taking it
down, floating over the list rather than replacing it — works exactly as [the
prompt editor's context menu](#the-prompt-editors-context-menu) does, because it
is the same box.

**⚙ Session…** opens the prompt's [session options](#session-options) panel
directly, without the editor around it — the launch setup is the thing most
often adjusted just before a send, so it sits right above **✉ Send…**. With no
form behind it to save later, leaving the panel (`enter` or `esc`) *is* the
save: the options are written to that prompt and you are back on the list with
it still highlighted. It is never dim, since the options are local to the
backlog. (The row used to be **◉ View**; the prompt view is still `ctrl+v`.)

Three rows read the *selection* rather than the prompt: **✓ Select** reads
**Unselect** on a row that is already ticked, **➦ Export…** becomes
**➦ Export 3 prompts…** while three are held — a menu opened on one row must not
say "Export" and quietly mean four — and **⧉ Add to batch…** becomes
**⧉ Batch 4 prompts…**, counting the row it was opened on too, since that row
goes into the [batch](#batches-several-prompts-one-drop) whether or not it is
ticked. It is dim on a done, frozen or info prompt, none of which a batch can
drop.

### Marking priority and quick wins from the list

The annotation rows in the middle are the reason this menu exists. A prompt's
annotations — its [priority](#priority), its [low-hanging fruit](#low-hanging-fruit)
and [value](#value) marks, its [info](#info) mark and its [flag](#flag) — are facts you read
straight off a list row, but until now the only way to *set*
one was to open the editor and find the annotation bar: a full round trip
through a form, to change a fact about a row you were already looking at.

They are the editor's controls, in the editor's glyphs, laid out down instead of
across. **☐ 🍏 Quick win**, **☐ ｉ Info** and **☐ ⚑ Flag** are checkboxes and
toggle. The three **Value** rows and the three **Priority** rows are radios and
set exactly their level, so pressing `▲ critical` on a prompt that is already
critical leaves it there rather than switching it off — and the default (`Value:
◇ low`, `Priority: none`) is a row of its own, which makes clearing a level the
same gesture as setting one. Each row repeats its group's name (`Value: ◆
medium`, `Priority: △ high`) because both groups have a `high`, and the word is
what tells them apart.
The status line names the result either way.

Unlike the editor's bar, these write **immediately**: there is no form open to
save, so the mark lands in the backlog on the press. With the priority lens on
(`ctrl+l`) raising a prompt lifts it past everything unraised, and the highlight
rides with the row so the next keystroke still acts on the prompt you just
marked; the status line says the list reordered, since on a tall pane the row can
travel most of it.

The flag row shows whatever note the prompt already carries, trimmed to something
a menu can hold, and clearing the flag from here takes those words with it exactly
as the editor does. Raising it opens the note pad below.

### The flag's note, where the flag was raised

A flag is only half a thought: *there is something about this one* wants
*…because* straight after it. So **☐ ⚑ Flag** does not just tick — the mark is
written to the backlog on the press, and then a small pad opens on the same cell
the menu was on, asking for the words while the prompt is still under the
pointer:

```
╭──────────────────────────────────────╮
│ ⚑ Fix the drop timeout               │
│ blocked on the api rename            │
│ enter save · esc leave it bare       │
╰──────────────────────────────────────╯
```

`enter` saves the note, `esc` walks away. **Escaping never costs the mark** —
the flag went to disk with the press, so the pad is an invitation and not a gate,
and the honest one-press gesture ("there is something about this one, and I'll
say why later") is still exactly one press. An emptied field is an answer too: it
clears the words and leaves the flag standing.

The pad is a text field, so while it is up it owns every key — a list chord fired
from inside it would act on the row you are typing a sentence about. Otherwise it
behaves like the boxes it borrows its frame from: it lands below-right of the
press, a click off it dismisses (right button included), and a resize re-places
it rather than throwing away a half-typed note. On a pane too small to float a
box in, the press falls back to what it always did — the mark goes up, and the
status line names the editor as the place to write the words.

**✎ Flag note…** under the checkbox is the same pad reached deliberately: it
reads **✎ Edit flag note…** once there is a note, opens with the current words in
the field, and is dim on an unflagged prompt — pressing it there says so rather
than doing nothing. It is how a note is rewritten without opening the editor at
all.

A value the program cannot read — the retired `low` from an old backlog, or a
typo in a hand-edited one — fills no radio, exactly as it draws no mark on the
row. All three levels are then offered as replacements, which is the honest
reading of a level that is not one.

## Quick capture from a shell

`add` puts a prompt in the backlog without opening the manager — for the moment
you notice the thing rather than the moment you sit down to work on it. It is
the same backlog either way; nothing about the entry marks where it came from.

```bash
cats-todo add fix the flaky reconnect       # → this project's backlog
cats-todo add -g clean up the dotfiles      # → the global one
cats-todo add -t "flaky test" fix the …     # → an explicit title
cats-todo add --priority critical fix the … # → marked critical
cats-todo add --fruit bump the version …    # → marked 🍏 low-hanging fruit
cats-todo add --value high split the store …  # → marked 🔷 high value
cats-todo add --flag="waiting on the api" …  # → flagged, with a note
cats-todo add --info the api returns 204 on …  # → ｉ a note, never sent to an agent
git log -p | cats-todo add -t "review this diff"   # → the prompt from piped stdin
```

The prompt is the remaining arguments joined by spaces; with none, it is read
from stdin when stdin is a pipe or a file. An interactive stdin is never read —
a bare `cats-todo add` prints usage rather than sitting there waiting for you to
type — so `add` is safe to bind to a key or drop in a script.

`-t` names the entry in the list. Left off, the title is the prompt's first line
(trimmed to 60 characters), which is usually the right thing; it is worth
setting when the prompt starts mid-thought, or when it arrives on stdin and its
first line is a diff header.

`--priority`, `--fruit`, `--value`, `--info` and `--flag` set the prompt's
[annotations](#annotations) (`critical`, `high`, `none`; the `🍏` quick-win mark;
the value, `high`, `medium` or `low`; the `ｉ` info mark; and the `⚑` flag), so a prompt captured mid-firefight
arrives already marked rather than needing to be opened afterwards to say so:

```sh
cats-todo add --priority critical --fruit -t "fix the drop path" "the daemon cannot resolve a bare agent name"
# → added to the project backlog, marked critical · low-hanging fruit (…/.cats-todo/todos.json)
```

Priority spellings fold, so `urgent` reaches critical and `important` reaches
high; the old scheme's words still fold onto what they meant — `standard`,
`normal`, `low` and `minor` all reach none — so a shell history or a script
holding `--priority low` keeps working. Anything outside the set is refused with
the same words the manager would use. Left off, both write nothing to the file:
the flags have no effect on a backlog until someone actually marks something.
`--priority` is long-only on purpose — a bare `-p` beside `--perm` reads as an
abbreviation of it, and a flag that looks like it means permissions while meaning
priority is the kind of thing that gets found out at the wrong moment — and
`--fruit`, `--value`, `--info` and `--flag` follow it for the same reason.
`--value` folds `hi`, `med` and `lo` too, and `none` onto `low`, the default. `--high-value`, the spelling from when
value was one bit, still works and means `--value high`; given together with a
different `--value`, the two are refused rather than one silently winning.

`--flag` carries its note in the same breath as the mark: bare, it raises a flag
with nothing to say; `--flag="blocked on the api rename"` raises one with the
words. The value must be attached with `=`, because the words after a bare
`--flag` are the prompt — which is the whole shape of this command.

Without `-g`, `add` writes to the project backlog rooted the way everything else
here roots it — nearest `.cats-todo/`, else the repo root, else the current
directory. Run it somewhere no project owns, and rather than inventing a backlog
in the current directory it stops and says so, pointing at `-g`:

```
cats-todo: no project backlog here — run from a project directory, or use -g for the global backlog
```

A prompt captured on the way past is worth little if it lands where you will
never look for it.

## The prompt editor's context menu

A swept run of the prompt is worth several different things, and none of them is
a chord anybody would guess. So they live where every editor keeps that list:
**right-click inside the prompt** and a menu names them.

```
╭──────────────────────────────╮
│ ✂ Split into prompts  ctrl+x │
│ ⇅ Sort lines                 │
│ ⌶ Caret on every line        │
│ ✓ Spelling…           ctrl+l │
│ ≡ Insert a prompt…    ctrl+p │
│ ↶ Undo                 cmd+z │
│ ↷ Redo           shift+cmd+z │
╰──────────────────────────────╯
```

↶ Undo and ↷ Redo are last, against the convention that puts them at the top of
a text field's menu. The cursor opens on the first row that can act, so the top
row is what a bare `enter` presses. Every other row on this menu makes a change
you can press again to fix, while those two rewrite the text wholesale.

It is built fresh on every press, from what the press was actually aimed at — but
an item that cannot act on the current selection is drawn **dim and still there**,
and says why when you press it. A menu whose contents move between presses is a
menu nobody learns the shape of; "why is this one grey" is a question the program
can answer, and "where did that item go" is not. The cursor opens on the first
row that can act, so `enter` straight after the click is never a refusal.

`↑`/`↓` walk the rows and `enter` presses one; a click does the same, and a click
anywhere off the box dismisses it. Any other key takes the menu down and is spent
doing so, which is what a menu does everywhere else. It floats **over** the form
rather than replacing it — a context menu that hid its own context would be
asking about a selection you can no longer see.

While a run is swept, the footer names the menu near its front as
`right-click: split/sort/carets`. It does not spend a second segment on
`ctrl+x`: the menu prints that chord on its own ✂ row, so one gesture on the
footer teaches every key behind it. With nothing swept the menu is still named,
as a bare `right-click menu` at the footer's tail, because the menu is also
where the prompt library (`ctrl+p`), undo and redo are taught. Their rows print
their chords, so the footer points at the menu once rather than naming each
chord. That keeps the whole footer within about 207 cells, where it used to
need about 244. `cmd+d dup line` is named only in a terminal that can send Cmd,
because the chord has no ctrl fallback (`ctrl+d` is the editor's
delete-forward).

### ✂ Split into prompts

A backlog item often arrives as a list — a plan pasted out of a chat, the
checklist at the bottom of an issue, a set of review notes. Every bullet in it is
a prompt an agent could be handed on its own, but only if it is a prompt of its
own: one todo holding six bullets can be dropped once, scheduled once and marked
done once, which is exactly the wrong granularity for six pieces of work.

Sweep the list — drag over it, or hold `shift` with `←`/`→` — and press
**`ctrl+x`**, or take the item off the menu. Each bullet becomes its own prompt
in the backlog, landing directly behind the prompt it came out of rather than at
the far end of the file.

```
Prompt                              Backlog
──────────────────────────          ──────────────────────
Ship the release:                   Ship the release:
- tag v2                     ──▶    ├─ tag v2
- write the notes                   ├─ write the notes
  - link the diff                   │    - link the diff
- announce it                       └─ announce it
```

Both markdown families are read as lists — the unordered `-`, `*`, `+` and the
ordered `1.` / `1)` — since a pasted list is whichever one its source used. A
`---` rule is not a bullet, and neither is a line that merely opens with a long
number.

**A nested list stays with its parent.** A sub-list is the detail of the item
above it, not a peer of it: splitting "write the notes" away from "link the diff"
would leave two prompts, neither of which says the whole task. Sub-lists and
plain continuation lines are dedented into the new prompt, so a sub-list arrives
there as a list rather than as an indented block whose indentation no longer
means anything.

**Only the selection is consumed, and only from its first bullet on.** A sweep
that caught the sentence introducing the list ("Ship the release:" above) has not
asked for that sentence to become a prompt or to disappear — it stays in the
editor. So does any bullet you did not sweep.

**What is left behind decides what happens to the prompt you were editing.** If
the editor still holds text, the form stays open on it: the split took a list out
of a prompt that is still being written, and the rest of that edit is still
yours to save. If the list *was* the whole body, there is nothing left to be a
prompt — the new ones are what you asked for *instead of* it — so the original is
deleted and you land back on the list.

The new prompts inherit the **backlog scope**, the **annotations** and the
**session options** of the prompt they came from: everything that says how the
work should run, which is the same for every bullet of one list. Attachments are
deliberately not inherited — an image belongs to the prompt it illustrates, and
copying it once per bullet would put N copies on disk for prompts that mostly do
not want it. When a whole-body split deletes an original that had attachments,
the status line says so rather than letting them go quietly.

They are written to the backlog immediately rather than on the next save, and
that is the point: the gesture means "these are separate items now", and an item
that only existed once the form was saved would leave the editor holding a list
it has already been told is gone. The whole run is one write, so either every
prompt lands or none does.

`ctrl+x` is free in the editor — it is none of the textarea's own bindings — and
it is already this program's "take this out": delete on the list, remove in the
attachment editor, and here the list that leaves the prompt to become prompts of
its own.

### ⇅ Sort lines

The same gesture, one step earlier: a pasted list is usually in the order it was
dictated in rather than an order anyone chose. Sweep it and sort it — and because
the split keeps the order of the items it makes, sorting before splitting is how
the resulting prompts land in that order too.

```
- write the notes            - announce it
- announce it        ──▶     - tag v2
- tag v2                     - write the notes
```

**It sorts whole lines, always.** A sweep that stops mid-word still means the
lines it crossed; half a line has no place in an order.

**A markdown list is sorted as items, not as lines.** An item's sub-points and
wrapped continuation lines travel with it — sorting those as lines of their own
would shuffle a list's details away from the items they explain. Text above the
first bullet is not part of the list and stays where it is, the same rule the
split follows.

**An ordered list is renumbered rather than shuffled.** The markers stay where
they are and the bodies move between them, so `1. 2. 3.` still reads 1, 2, 3 down
the page. A list whose markers all read `-` is unaffected either way, and
continuation lines are re-indented to whichever marker they land under, so a
`10.` item and a `9.` item both line up under their own text.

Case and surrounding space are out of the comparison — "Tag v2" and "tag v2"
belong beside each other — and the sort is stable, so sorting twice cannot
shuffle anything a second time. Blank lines collect at the end rather than the
top: a gap between two lines is a separator, and a separator has nothing left to
separate once the order has changed.

The highlight survives, moved onto the sorted text. That is what makes the two
items compose — sort a list, then split it, without sweeping it again — and it is
also the only visible proof of what the sort took as its input, since the block is
otherwise the same characters in a different order.

### ⌶ Caret on every line

The third thing a swept block is worth, and the one that turns *not yet a list*
into a list:

```
sweep three plain lines      carets go down            type "- "
  tag v2                       ▌tag v2                   - tag v2
  write the notes              ▌write the notes          - write the notes
  announce it                  ▌announce it              - announce it
```

which is then exactly the shape ✂ Split into prompts wants. While the mode is on,
**what you type goes in on every line at once**: `backspace` deletes on every
line, `enter` breaks the line at every caret (each new line keeps its own line's
indent, and a `backspace` straight after takes all of them back), `tab` types four spaces at every
caret and `shift+tab` outdents every caret's line, `←`/`→` move the carets together, `ctrl+a` takes them to the line starts and
`ctrl+e` to the line ends — prefixing, unprefixing and appending to a block, which
is what a column mode gets used for in every editor that has one. A paste goes to
every caret too. When a paste has several lines, it follows the rule other
multi-cursor editors use. If the number of lines matches the number of carets,
**each caret gets one line**, top to bottom: copy three names, alt+click three
places, paste, and each place gets its own name. Otherwise **every caret gets the
whole paste**, newlines included, and ends up after its own copy. A single
trailing newline is not counted as a line, because copying whole lines usually
brings one along. `\r\n` and bare `\r` count as newlines. Until v0.30.2 only the
first line of a paste went in.

Every caret lands in the column the **sweep began** in, which is column 0 for the
sweep this is for — a drag down the left margin, or a `shift`+`↓` run from the
start of a line. That is what makes `- ` prefix the block. Each column is a *goal*
column, not a position: a line too short for it takes its caret at its end and is
not stranded there when the others move on, the same rule `↑`/`↓` already follow
in any editor.

**`alt`+`click`** is the other road in, and the pointer's own: a press with alt
held puts a caret where you clicked, beside the one the editor already has, and
each press after that adds another — on lines that are not neighbours, in columns
that are not equal, which is exactly what the sweep cannot say. Every caret keeps
the column it was aimed at, so typing lands in a different place on each line. A
press **exactly on a standing caret** takes that caret away. Down to one caret
the mode simply ends — one caret is what the editor is when the mode is off.
(The gesture depends on the terminal reporting alt with the press; cats does,
and most terminals do, but one that keeps alt+click for itself never forwards it
— the sweep and the menu's ⌶ remain the keyboard's road in.)

**A line can carry several carets.** A caret is a cell, not a row, so two
presses on one line put two carets on it and typing lands at both. This is what
makes the pointer useful on the commonest prompt there is — one long paragraph
that **soft wraps** across several rows of the box. Those rows look like separate
lines and are all one line; until v0.22.0 the second press on them was refused,
which made alt+click appear dead on exactly the shape it was most wanted for.

The one press that adds nothing is a press on a caret that is already there. It
says so — *the caret is already there* — rather than doing nothing in silence,
because silence is also what a terminal that ate the modifier looks like. Seeing
that note proves alt reached the program; no note and no new caret means it did
not.

**`enter` is a newline at every caret** (so are `alt+enter` and `ctrl+j`), and
the mode stays on. Each caret moves to the start of the line its break created,
so text typed right after enter goes at the start of every new line. A caret in
the middle of a line splits it there, and several carets on one line split it at
each of them. Until v0.30.2 enter ended the mode without inserting anything.
That made multi-caret newlines impossible and looked like the editor refusing
them.

`esc` ends the mode, and so does anything that means *one* caret — `↑`, `↓`, a
plain click. A chord the mode has
no meaning for ends it and then does its usual job, so `shift+enter` still saves
from inside it. Nothing is undone on the way out: everything typed is already in the
prompt, exactly as if it had been typed once per line by hand.

The footer belongs to the mode for as long as it lasts, because the keys do.


## The prompt library

The same paragraphs get typed into the editor over and over: the way you like a
bug reproduced, the review checklist you always paste, the `/sess-load` that
opens every session on this machine. The library is where those live, once.

Press **`ctrl+p`** (`cmd+P` inside cats, which forwards it) in the prompt and it
opens over the form — the same fuzzy list every other picker here uses. Type to
narrow, `↑`/`↓` to walk, `enter` (or a click) to insert at the caret.

```
Insert a prompt  ~/.config/cats-todo/prompts.json

│ 🔍 sess                                   1/3 │

❯ load session · /sess-load  pick up where we left off

enter insert · ctrl+s saves the selection under the typed name · esc back
```

The query matches the name, the description **and the body**, because an entry is
as often remembered by a phrase inside it as by what it was called.

### Where it lives

`~/.config/cats-todo/prompts.json` — beside `settings.json`, in the global config
directory (`$CATS_TODO_CONFIG_DIR` or `$XDG_CONFIG_HOME/cats-todo` if you set
either). It is deliberately **user-level, not per-project**: a phrasing worth
keeping is a habit of the person, not of the repository, and the same wording
goes into a prompt whichever checkout the manager was launched from. Backlogs
stay per-project; the words you write them with do not.

```json
{
  "prompts": [
    {"name": "repro steps", "desc": "how to file a bug", "body": "Steps to reproduce:\n1. "},
    {"name": "load session", "desc": "pick up where we left off", "body": "/sess-load"},
    {"name": "wrap up", "body": "/sess-wrap"}
  ]
}
```

Only `body` is load-bearing; `name` and `desc` are how you find the entry again.
A bare top-level array works too, since that is what a hand-written file
naturally looks like. The file is **read fresh every time the picker opens**, so
editing it in another window needs no restart — and a typo in it is reported on
the picker rather than silently read as an empty library, because a library that
looks lost and a library that is lost should not look the same.

### Commands (skills) like `/sess-load`

An entry whose body starts with `/` is a **command** rather than a snippet, and
nothing has to declare that — deriving it from the text is what keeps a
hand-written file from having to say the same thing twice. The distinction is not
cosmetic: a slash command only *is* one when it begins a line, so a command is
inserted **on a line of its own**, opening one above it when there is text in the
way and leaving the caret on a fresh line below:

```
fix the crash in drop.go     →     fix the crash in drop.go
                     ▲                 /sess-load
                     caret              ▲ caret
```

A snippet, by contrast, lands exactly at the caret and changes nothing around it.
Its author already decided where its newlines are, and an entry ending in `"1. "`
means to leave the cursor after that space.

Because a command is written at a line start, that is also where it can be asked
for: typing **`/` at the start of a line** opens the picker with the commands
alone, and the entry you choose replaces the slash you typed rather than doubling
it. `esc` leaves the plain `/` behind, so nothing is lost by opening it.

Two guards keep that out of the way of ordinary writing, and a slash needs them
where `@` does not — `and/or`, `src/ui`, `3/4` are all just text. It fires only
at a line start (indentation still counts), and only when the library actually
holds a command: if you keep none, `/Users/ro/…` typed at a line start is left
completely alone.

`ctrl+p` remains the way to reach prose snippets and commands together, and the
one that works from anywhere in the prompt.

### Saving what you just wrote

A library you can only grow by opening another editor is a library that stays
empty, so the picker is also where entries are made. Sweep a run of the prompt
(or write the whole thing), press `ctrl+p`, type a **name in the query box**, and
press **`ctrl+s`**:

- with something swept, the selection is what gets saved;
- with nothing swept, the whole prompt is.

The footer says which of the two before you commit to it, and the entry is on
disk — in the shape above — before the keys come back. A name already in the
library is refused rather than overwritten, in those words: overwriting is the
destructive reading of an ambiguous gesture, and renaming is one keystroke.

`ctrl+s` means "save this snippet" here, and one screen up it now opens the ⚙
session panel — the two screens are far enough apart, and the picker takes every
key while it is open, that there is no press which gets the wrong one. The form's
own save (`shift+enter`) is not reachable from the picker either.


## Images

In the editor, `ctrl+o` opens the attachment editor. Three ways to get an image
in:

- **`ctrl+v`** pastes an image straight off the clipboard — copy one out of a
  browser, or take a screenshot with `shift+cmd+ctrl+4`, and it lands as
  `clipboard.png`. macOS only (the pasteboard is the system's, not the
  terminal's); the key is only offered where it works, and with anything other
  than an image on the clipboard it stays an ordinary text paste.
- **`ctrl+r`** fills the box with your most recent screenshot; press again for the
  one before that.
- **paste or drag a path** into the box and press `enter` — dragging a file onto
  the pane inserts its path, quoting and escaping included.

`ctrl+x` removes the highlighted attachment, `esc` goes back to the prompt.
Nothing is copied until you save the prompt, so cancelling costs nothing — and
removing an existing attachment only deletes the file once the save succeeds.

From a shell, `add -i <file>` does the same thing, repeatably:

```bash
cats-todo add -i ~/Desktop/shot.png -i ~/Desktop/other.png the header wraps wrong
```

Either way the file is *copied* into the backlog
(`.cats-todo/images/<todo-id>/`, or the config dir for a global todo), so you can
attach a screenshot and then clear it off your Desktop. The list marks an
attachment-carrying prompt with `📎n` — in cyan, the same hue the editor's
**Images** chip carries, so "this one has a picture" is answered by a glance down
the list rather than by reading each row — and `ctrl+v` lists the files. Done and
frozen rows keep the count but not the color: those rows recede as a whole, and
the cyan is there to point at prompts still waiting on a picture.

`ctrl+r` scans `~/Desktop` and `~/Downloads`; set `CATS_TODO_IMAGE_DIR` to point
it somewhere else (macOS can be told to save screenshots anywhere, and cats-todo
does not shell out to `defaults` to find out where).

Nothing binary crosses the wire: a drop delivers the prompt with each
attachment's absolute path appended, and the agent reads the files itself. An
attachment that has since been deleted is left out of the delivered prompt and
flagged in the `ctrl+v` view rather than sent for the agent to chase. Accepted
formats are `.png`, `.jpg`/`.jpeg`, `.gif` and `.webp`, up to 10 MiB each.

In the editor, holding `shift` with any caret motion **selects**: `shift+←`/`→`
by the character, `shift+alt+←`/`→` by the word, `shift+home`/`end` to the ends
of the line, `shift+↑`/`↓` — or `shift+alt+↑`/`↓` — across lines, and sweeping
with the mouse button held down selects too. `ctrl+c` copies what is highlighted, and only while
something is (with nothing selected it still quits, as it does everywhere else).
Typing replaces the selection the way it does in every other editor: the next
character, newline or paste lands *on* the highlighted run rather than beside
it, and `backspace` or `delete` takes the run out. Anything else — a plain
arrow, a click, a save — simply drops the highlight, because a highlight left
standing over text the caret has walked away from is a lie about what the next
`ctrl+c` would copy.

`cmd+c` copies the highlighted run too, and `cmd+v` **pastes** the clipboard at
the caret — the chords a mac hand actually reaches for. Both are aliases and
neither is the only way in. `ctrl+c` is the copy that always works, and a paste
usually arrives with no chord at all because the host performs it: under cats
⌘V is read by the client and delivered to the pane as a real paste (which is
also why ⌘V never reaches the manager as a keystroke there), and most mac
terminals do the same. The `cmd+v` binding is for the hosts that forward the
press instead of acting on it. Unlike `ctrl+c`, `cmd+c` never quits — the quit
is a liberty worth taking on the chord that always works, and not worth taking
twice — so with nothing selected it says there was nothing to copy. Pasting
works in the title as well as the prompt; on the annotation bar it says where it
does work rather than spraying text at whichever field last held the keys. Off
macOS there is no local pasteboard to read, so the chord asks the terminal over
OSC 52 — a read many terminals refuse — and reports an empty answer rather than
pretending.

Copying is not all a swept run is worth. **Right-click inside the highlight** and
a menu offers the rest — split a markdown list into one backlog prompt per bullet
(also `ctrl+x`), sort the swept lines, or put a caret on each of them and type
into all of them at once. See *The prompt editor's context menu* above.

`alt+↑`/`alt+↓` **moves the line the caret is on**, one row at a time, with the
caret riding it in the column it held — so the press after the move carries on
where your hand already was. It is where every editor on this machine keeps that
gesture, and it is the natural partner of ⇅ Sort lines: the sort puts a whole
block in order, this moves one line to where you actually wanted it.

With a run swept the **whole block moves** and the highlight travels with it,
exactly as it was — a selection that begins and ends mid-word included. The
block's text does not change, only where it starts, so every offset inside it
shifts by the same amount. A first line has nowhere to go up and a last line
nowhere to go down; both say so rather than going quiet, because a chord that
stops answering on the boundary reads as a chord that stopped working.

"Line" here is a logical row, not a drawn one: a paragraph that soft-wraps over
three display lines moves whole, the same rule `cmd+d` follows below.

Held with `shift`, the same two keys **extend the selection** by a line instead —
`shift+alt+↓` does what `shift+↓` does, which matters because distinguishing a
shifted arrow from a bare one needs the kitty keyboard protocol and the alt
spelling is the one a terminal is more likely to send. The horizontal pair is
unaffected: `shift+alt+←`/`→` is still word selection, since there `alt` is the
editor's own word motion rather than a line move.

`cmd+d` **duplicates the line the caret is on**, dropping the copy directly
below it and leaving the caret on the copy in the column it held — so holding the
chord stacks copies the way it does in a code editor, and the press after it
carries on where your hand already was. A line here is a logical row (a run
between newlines), not a drawn one: a long paragraph that soft-wraps over three
display lines duplicates whole, because splitting it at a wrap would cut it at a
boundary the text does not contain. There is deliberately no `ctrl+d` fallback —
`ctrl+d` is the editor's delete-character-forward, and a duplicate bound over a
delete is the one collision a text editor must not ship. Cmd only reaches a TUI
from a terminal that reports it (cats does; see `cmd+s` below), so on a
terminal that eats the chord this is simply unavailable rather than wrong.

**Indenting.** In the prompt, `tab` indents and `shift+tab` outdents, as in a
code editor. With nothing swept, `tab` types four spaces where the caret stands
(mid-line too, for lining things up), and `shift+tab` takes up to four leading
spaces off the caret's line. With lines swept, `tab` puts four spaces in front
of every line the sweep touches (blank lines are skipped, so no invisible
trailing spaces), `shift+tab` takes up to four off each, and the sweep stays so
a second press moves the block another level. A sweep that began at a line start
still begins there afterwards, with the new indent inside the highlight. When
there is nothing to outdent, the status line says so.

**`enter` keeps the indent.** A new line starts at the indent of the line you
pressed `enter` on, so a nested list or a code block stays at its level while you
type it. There are no tab stops. The new line copies whatever indent the line has,
two spaces as readily as four, rather than rounding it. To leave that level,
press `backspace` straight after the `enter`, which takes the whole carried
indent back in one press and puts you at the margin, or press `shift+tab` to step
out one level. Only the `backspace` straight after counts. Once you have typed
anything, `backspace` deletes one character again. Pressing `enter` on a line
that holds nothing but that indent moves the indent down to the new line, so
blank lines are left truly empty. A caret inside the indent carries only the
spaces to its left. A paste goes in exactly as copied, because pasted text
brings its own indentation.

The indent is **spaces, not a tab character**. The editor turns a tab character
into four spaces on every edit, the screen and the click targets are measured in
cells a tab character would misplace, and a prompt is typed into Claude Code,
where a tab keystroke means something else. Four spaces is also what a pasted
tab already becomes, so typed and pasted text agree.

That takes `tab` off the form's focus ring *while you are in the prompt*: a click
leaves it. The title and the annotation bar keep `tab`/`shift+tab` for walking
the ring, and `shift+tab` from the title reaches the bar.

In the list `alt+enter` is bound everywhere `shift+enter` is, and in the editor
`alt+enter` and `ctrl+j` insert a newline alongside plain `enter` — the chords
the form taught first still work, and a hand that learned them is never told it
is now wrong. `shift+enter` is the exception, because in the editor it is the
save: the form catches it before the textarea ever sees it.

Distinguishing shift+enter from a bare enter needs the kitty keyboard protocol —
cats speaks it, but a terminal that does not will send the two identically, so
the list's footers advertise `alt+enter` (which every terminal encodes as
`ESC CR`) until the terminal answers the handshake. In the editor there is
nothing to fall back to: a terminal that cannot tell the two apart sends a plain
enter, which inserts a newline as it should, and the save is then `cmd+s`, the
✔ Save button, or `enter` from the Title field.

`cmd+s` saves the editor too, wherever the terminal is willing to report the
Command key — cats does, so under cats it is the mac chord for the mac hand.
Elsewhere the same press may never leave the terminal: Terminal.app claims Cmd
for its own menus, and iTerm2 needs the chord mapped by hand. It is the reason
`ctrl+s` could be spent on the ⚙ panel: under cats the editor has two save
chords that both arrive, and the letter is worth more to the one control that
had no keyboard road of its own.


## Autosave

The editor keeps your work on disk while you write it. **45 seconds after the
first change since the last save, the form writes itself to the backlog**, and
the line under the editor says `autosaved 15:04`. You stay in the editor with
the caret where it was. Nothing on screen moves, and the undo history is
untouched. A pane that closes, a cats restart or a crash therefore costs at most
45 seconds of typing, where before it cost everything since the form opened.

It is a throttle, not an idle timer: the wait starts at the first change and
does not restart on later ones. An "after you stop typing" timer never fires for
someone writing steadily, and a steady writer has the most to lose. After a
write, the next change starts the next wait. A form that has only been opened,
or whose edits were undone back to what is already saved, has nothing to write
and writes nothing.

What it writes is the title, the prompt, the ⚙ session options and the marks,
which is everything ✔ Save writes **except attachments**. Attaching copies files
into the backlog and detaching deletes them. A timer should not create or delete
files you are still deciding about, so images wait for ✔ Save as before.

On a **new** prompt, the first autosave adds it to the backlog and the form
becomes an edit of that prompt. Later autosaves and your own ✔ Save update the
same entry, so a long session never leaves a trail of duplicates. The backlog is
fixed from that point on: the scope tag leaves the toolbar and `ctrl+g` stops
toggling it, because the prompt already lives in one backlog. ✔ Save still
reports "added to … backlog", since from your side this is the first save.

**esc (✖ Cancel) still means "keep nothing from this form".** Autosave is a
safety net under the editor, not a second Save button, so cancel takes back what
the timer wrote. An autosaved new prompt is removed from the backlog. An
autosaved edit gets its original title, prompt, session options and marks put
back. The only way an autosave outlives the form is if the form never reaches
esc, which is the situation it exists for. An empty prompt is never autosaved,
and no error is shown for that. ✔ Save explains its own refusal when you press
it, and a warning appearing on every tick unprompted would be noise.

**The wait is a setting.** Set `autosaveSeconds` in
`~/.config/cats-todo/settings.json`:

```json
{ "autosaveSeconds": 90 }
```

A shorter wait loses less to a crash. A longer one writes `todos.json` less
often, which matters when another pane is watching the file, or when the
project backlog is committed and every write shows up in `git status`. `0` turns
autosave off. Anything from 1 to 4 is raised to 5 seconds, because each
autosave rewrites the whole backlog and doing that every second while you type
is never what anyone wanted. The value is read when the manager starts, so
restart it after changing the file.


## Undo

The editor changes a lot of text per keystroke — a sort, a block indent, a line
move, `cmd+D`, typing at a dozen carets at once, a snippet dropped in at the
caret. Every one of those is aimed by hand and can be aimed wrong, and until
v0.32.0 the only way back was to retype what had been there.

**`cmd+z`** (`ctrl+z` in a terminal that eats Cmd — both are always bound) takes
back the last thing that happened to the prompt, and **↶ Undo** on the editor's
right-click menu does the same. Press it again for the step before that. The
caret goes back with the text, to where it stood when the undone edit began, so
the next keystroke lands where you left off rather than wherever the cursor
happened to end up.

**A run of like keys is one step.** Undoing "hello" one letter at a time would be
a machine's idea of undo, not an editor's. Consecutive typing coalesces into one
step and so do consecutive deletions; a **word boundary**, a newline, an arrow
key, a click, or an edit of any other kind ends the run. So one press takes back
the word you just typed, the block you just indented, or the sort you did not
mean — a unit you would recognize as *what I last did*. Everything else — a
paste, a menu item, a picker's insertion, a spelling correction — is always a
step of its own, never folded in with the keys around it.

**Redo** is **`shift+cmd+z`** (`ctrl+y` in a terminal that eats Cmd, and
`ctrl+shift+z` where the kitty protocol can report it), and **↷ Redo** on the
same menu. An undo no longer throws away the state it leaves: it keeps it on a
second stack, and redo steps back up through it. So one undo too many costs
nothing. Moving the caret or clicking keeps the redo stack. The next edit to the
text clears it, because once the prompt has gone somewhere new, the undone
states no longer follow from it. This is the linear model every editor on the
Mac uses.

The history is **per editing session**: it starts empty when a form opens and is
gone when you leave, because offering to replace one todo's prompt with the text
of the one you edited before it is the worst thing an undo could do. With nothing
to take back or redo, the chords and the menu rows say so.

What it cannot take back is anything that has already left the editor. **✂ Split
writes prompts into the backlog** and then removes the bullets from the text;
undo brings the text back, and the prompts it wrote stay written — delete those
from the list if you meant to call the whole thing off. The rule is the honest
one: the editor can only undo what is still in the editor.


## Spell check

The editor underlines, in red, words its dictionary does not know — a glance
that catches `teh` before an agent is handed it. The underline is all it does on
its own; acting on one is opt-in, and lives one key away.

`ctrl+l` opens the **Spelling** panel on the flagged word nearest the caret
(the word being typed first, then the nearest one behind it). It offers the
three answers there are to an underline:

- **the spellings it might have been** — pick one and the word is replaced in
  place, the caret left where it ends. The candidates are ranked by the kind of
  mistake they imply rather than the number of them, so `teh` offers `the`
  before `tea`, and `dont` offers `don't` before eight other words one edit
  away;
- **add it to a dictionary** — yours, or this project's (see below). The
  underline goes at once, and the word is written to the file so the next launch
  still knows it;
- **turn the check off** — without leaving the panel.

Type to filter the rows, `enter` presses one, `esc` or `ctrl+l` goes back.

**Right-click an underlined word** and take **✓ Spelling…** to open the same
panel on *that* word. It is
the gesture every editor has taught for a red squiggle, and it earns its place
beside `ctrl+l` because the keyboard path can only guess: in a prompt with three
flagged words in it, "the one nearest the caret" is a guess, and the pointer is
the one input that can simply say which. Because a hand that points at a
squiggle is usually about to say *that is a word*, the panel opens with the
**✚ Add** row already highlighted — right-click, `enter`, and the word is in
your dictionary. The suggestions are still one `↑` away for the click that turns
out to have been a typo after all, and this project's dictionary one `↓` away.

The word being typed can be right-clicked too, though it carries no underline:
the gesture is aimed at a word, not at a mark. A right-click that lands
somewhere the panel has no answer for says so on the note line rather than doing
nothing — a word the dictionary knows, or the check being off, are different
things and are told apart.

The ask is a row on the editor's context menu now rather than the whole meaning
of the right button (see *The prompt editor's context menu* above): right-click,
then take **✓ Spelling…**. The press still names its own word — that is the
whole reason the gesture exists beside `ctrl+l` — so the row is dim, and says
which of the two things is wrong, exactly when the old direct right-click would
have refused.

The check is turned off and on from the panel's own last row — there is no
toolbar chip for it any more, since the underlines are answered by right-clicking
the word they are under and the form's row of buttons is worth more as the five
things an editing session ends with. Either way the choice persists across
launches (in `~/.config/cats-todo/settings.json`).

It is built to be quiet on a prompt about code. Skipped, not checked: anything
in backticks or a fenced block; tokens that start with `@`, `-`, `#`, `~`, `$`,
`/`, `.`, `\`, `<` or `&` (an `@path` from the file picker, a `--flag`, a
`#123`, `~/dir`, `.dotfile`, `<tag>`); anything holding a digit or a character
other than letters, apostrophes and hyphens (paths, URLs, `snake_case`, `v2`);
words with a capital after the first letter (`CamelCase`, `ALLCAPS`); words with
letters outside ASCII (names, other languages — the list is English); and words
of one or two letters (`db`, `ui`, `js`). The word under the caret is left alone
until you move on from it, so nothing flickers while a word is half-typed.

The dictionary is embedded — SCOWL's American English at size 60, plus the
everyday vocabulary of software work (`json`, `rebase`, `goroutine`, `worktree`
…) — so it behaves the same on every machine, and needs nothing installed. Add
your own words one per line to either of:

- `~/.config/cats-todo/dictionary.txt` — yours, everywhere;
- `<project>/.cats-todo/dictionary.txt` — the project's jargon, committed
  beside the backlog so a teammate's editor knows it too.

`#` starts a comment; case does not matter. Both files are read when the editor
first opens, and the panel's add rows write to them — creating the file, with a
header explaining what it is, the first time.

## Code in a prompt

Prompts are written for an agent that reads Markdown, and the code in them —
a command to run, a flag, a file name — is what a reader most wants to pick out.
The editor and the read-only view (`ctrl+v`) both draw it in blue:

- **inline code**: a run of backticks up to the next run of the same length on
  the same line, backticks included — `` `go test ./...` ``. A run with no
  partner is just a character. A span never crosses a line break, so an
  unmatched backtick typed halfway through a prompt colours nothing below it;
- **fenced blocks**: from a line that starts with three or more backticks
  (an info string such as ` ```go ` is fine) to the next line that starts with
  at least as many and has nothing else on it. Both fences are coloured with the
  block. A fence not yet closed runs to the end of the prompt, as it does in
  Markdown, so while a block is being typed everything below it is blue until
  the closing fence goes in.

The colour is only drawn. It does not change what is saved or sent, and the
agent gets the backticks exactly as they were typed. Blue is the one cool hue in
the palette, so code stands apart from prose by colour alone. There is no
background or weight change to make a line jump as a span opens and closes under
the caret. The selection and the spell underline win the cells they share with
code, so both look as they always have. The spell check already skips backticked
text, so the two rarely meet.

## Session options

A drop used to deliver one thing: the prompt. *How* the receiving agent ran —
which model, at what effort, starting from what prior context, and what to do
once the work was finished — was whatever the default was, and had to be
arranged by hand every single time. Session options make those choices a
property of the prompt instead. They are stored with it in `todos.json`, so they
travel with the repo and a drop reproduces the whole setup — whether you pressed
the key or a schedule fired it at 3am.

In the editor, `ctrl+s` opens the ⚙ panel (or click the **Session** chip;
`ctrl+r`, the chord it had before saving moved to `shift+enter`, still works).
`↑`/`↓` walk the rows, `←`/`→` (or `space`) change the one under the cursor, and
`esc` goes back to the prompt. The form shows what is set on its `⚙` line, the
list marks a configured prompt with `⚙`, and nothing is written until you save
the prompt itself. From the list, right-click a row and pick **⚙ Session…** to
open the same panel without the editor; there, leaving the panel saves.

Every row of the panel describes the session that will read the prompt. The
prompt's own marks — priority, quick win, high value — are not here: they are set on the
editor's [annotation bar](#annotations), in sight of the title they qualify.

| Row | What it does |
|---|---|
| Model, Effort | `--model`, `--effort` on the launch; `/model`, `/effort` in a running pane |
| Permission | `--permission-mode` on the launch (a new session only) |
| Clear first | sends `/clear` as its own message before the prompt |
| Context | starts with `/sess-load [n]` or `/sess-use <pattern>` |
| Files | "also read these files" ahead of the prompt |
| Finish | commit · commit and push · run `/sess-wrap` |
| Reviews | `/code-review`, `/security-review`, `/simplify` before finishing |
| Release | cut a release once the work is done |

Three different mechanisms carry them, and which one an option rides is forced
by what the receiving end can accept. On a **new** session the three launch
flags go on the agent's own command line — and only for `claude`, whose flags
they are; the picker says so on any other agent's row, and the prompt still
goes.

A drop into an **existing** pane has no command line, but a prompt that asked
for a model still means that model, so the settings are applied to the running
session instead, each as its own submitted message ahead of the prompt:

```
/clear            (Clear first)
/model sonnet     (Model — claude panes only)
/effort high      (Effort — claude panes only)
<your prompt>
```

`/clear` comes first so the model and effort land on the session that will read
the prompt, and `/model` before `/effort` because the levels a model accepts are
its own. They are submitted in paste mode too — the pause is for the prompt, not
the setup. The two claude commands go only to a pane cats detected as `claude`:
typed into a shell they would be a command line, and run mode would run it. A
command that fails aborts the drop rather than delivering the prompt onto the
wrong setup. Permission mode is the one setting a running session cannot be
given — Claude Code only cycles through modes with `shift+tab`, from a starting
point nothing on the wire reports — so the pane's row in the picker says it will
be left as it is, before you pick it.

**A switch mid-conversation asks first.** With Clear first off, `/model` and
`/effort` land in a live conversation. When the pane's prompt cache is still
warm and the switch would change something, Claude Code puts up a *Switch
model?* (or *Change effort level?*) dialog, because the whole history gets
re-read uncached on the next message. Left alone, that dialog would take the
next Enter, and the prompt typed ahead of it would go nowhere. So after each of
the two commands the drop watches the bottom of the pane, and if the dialog
comes up it presses **Yes**. The prompt asked for that model, and that is the
answer to the question. The status line then says so (`· confirmed the model
switch`), so the cost of the uncached turn is not a surprise. Two cases are left
to you. If a PreModelSwitch hook of yours is what asked, the drop stops with the
dialog still up and says why, since answering your own policy for you would
defeat it. And if the dialog's words are already on screen before `/model` is
sent, the drop can't tell an old dialog from a new one, so it doesn't press
anything. With Clear first on none of this comes up: `/clear` counts as
acknowledging the switch.

**An effort level the model doesn't have runs at high.** `xhigh` and `max` exist
only on some models. On any other, Claude Code accepts `/effort xhigh` (or
`--effort xhigh` on a new session) and reports it as set, then sends high
instead. Nothing errors, and the ⚙ panel can't warn you ahead of time, because
which models take which levels is Claude Code's knowledge, partly served to it
at runtime, and a copy here would go stale. If a prompt needs the top levels, give it a
model that has them, or leave Effort unset.

Everything else is text wrapped around the prompt body, which works everywhere:

```
First, load prior context: run /sess-load 2
Also read these files: ai_docs/design.md

<your prompt>

When the work is done and the tests pass:
- run /code-review
- run /sess-wrap (saves a session doc, commits, and pushes)
```

A prompt with no options set delivers exactly its own text, byte for byte, as it
always did — every option's unset value means "inherit the default", and an
unconfigured prompt writes no `session` key at all.

The context rows call the `sess-*` slash commands (`~/.claude/commands/`). Where
they are not installed the panel greys those rows and says so, but still saves
them: the backlog travels, and the machine that writes a prompt is often not the
machine that runs it. Exiting the agent is deliberately not offered as a
finishing step — the transcript is the one thing worth having after an
unattended run.

The same options from a shell, where they are also what `add` records:

```bash
cats-todo add --model sonnet --effort low --finish wrap "say hi"
cats-todo add --sess-load 2 --review code-review --release "finish the drop panel"
cats-todo add --sess-use drops --ctx ai_docs/design.md --perm accept-edits "wire it up"
```

`--perm` takes the readable spellings too (`accept-edits` → `acceptEdits`,
`bypass` → `bypassPermissions`), and a value neither the TUI nor the CLI
recognises is refused with the same message in both. `--ctx` and `--review`
repeat; `--sess-load` and `--sess-use` are two answers to one question and can't
both be given. `--sess-load`'s count is optional — `--sess-load`, `--sess-load 2`
and `--sess-load=2` all work — and the `⚙` line printed after the add echoes
what was recorded.

The manager wears cats' own muted green: the palette in `styles.go` is cats'
`defaultColors` (`internal/config`) — the same values the served page sets as
its `:root` custom properties — so a manager pane and the terminal around it
read as one product. Keep the two tables in sync. The greys are the exception:
cats' chrome tones are surfaces for a web page and sit too close together to
separate a terminal's four tiers of text, so the name/description/footer/done
ramp is interpolated down from `fg` toward `line` instead. The manager sets the
terminal's background and foreground while it runs and hands both back on the
way out.

## The project backlog is a committed file

`.cats-todo/` is meant to be checked in — the `todos.json` manifest and the
attachments beside it. A backlog of "what this project needs next" is worth
what the repo's other notes are worth, and committing it means a teammate who
clones the repo gets the prompts too, screenshots and all, and can drop one
straight into an agent.

Because it is a file in someone's version control, it is created on request
rather than as a side effect:

```bash
cats-todo init          # create .cats-todo/todos.json for this project
git add .cats-todo      # …then commit it like anything else
```

`init` runs from any subdirectory — it resolves the same project root the
manager and `add` do. It is also the one command here that can destroy todos, so
it never writes over a backlog silently. Point it at a project that already has
one (your own, or one that arrived with a clone) and it shows you what is there
before asking:

```
cats-todo already has a backlog: 12 todos
  · fix the flaky reconnect
  · port the drop picker to v2
  · document the control socket
  …and 9 more
Replace it with an empty backlog? This deletes those 12 todos. [y/N]
```

A bare enter keeps it. With no terminal to answer at — a script, a pipe — the
answer is never assumed: init refuses and leaves the backlog alone. `-f`
replaces without asking, for when you mean it.

(This repo is the exception that proves it: cats-todo's own `.cats-todo/` is
gitignored, because a backlog whose prompts are "test the thing I am building"
is scratch, not a record worth keeping.)

## Installing

cats-todo is a cats plugin — `cats-plugin.toml` here is the reference manifest
for writing new ones. Install through the cats plugin host:

```bash
catctl plugin install rohanthewiz/cats-todo   # clone from GitHub + build
catctl plugin run rohanthewiz.cats-todo       # launch in a new tab
catctl plugin link .                          # dev mode: symlink this checkout
```

A first install offers to run `init` for the project you installed from; later
upgrades stay quiet, since they would only be re-asking a question you have
already answered. The offer needs the plugin host to hand a build step a
terminal and the invoking directory (`CATS_PLUGIN_INSTALL_CWD`) — where it
cannot, it prints how to run `cats-todo init` instead of guessing.

Or build it directly — it is a plain Go module:

```bash
go build -o bin/cats-todo .   # a binary in this checkout
go install .                  # …or one on your PATH, as `cats-todo`
```

`go install` is the one that matters for `add`: quick capture is only quick if
`cats-todo` runs from whichever project you happen to be standing in.

## Shell completion

Install cats's completion and `cats-todo` completes itself — subcommands,
flags, and image files after `-i`:

```bash
eval "$(catctl completion zsh)"    # ~/.zshrc, after compinit; also bash / fish
```

`cats-plugin.toml` declares this in a `[[completions]]` block naming the
command and a `__complete` argv. `catctl completion <shell>` reads it when it
generates the script, so the registration lands in the same file as catctl's
own — which also means a shell started before the plugin was installed will not
have it until the next one. See `complete.go` for the protocol; a plugin with no
completion code of its own can list `subcommands` and `flags` in the manifest
instead and let catctl serve them.

## How it talks to cats

The manager talks to the cats server over the local control socket
(`CATS_CONTROL_SOCKET`) — the same §7 command table `catctl` drives:
`pane.list` to find agent panes (and, joined with `workspace.list`, where each
open workspace is working — the export picker's rows), `tab.create` to open a
new session already named and running the agent (no shell in between),
`pane.wait_for_output` to pace launches, and `pane.send_input` to deliver the
prompt.

The vocabulary itself is not copied: cats publishes it as the leaf package
`github.com/rohanthewiz/cats/wire` (stdlib-only), and this repo imports it, so
`go get github.com/rohanthewiz/cats@<rev>` is the whole of "keep up with the
protocol" — the compiler then points at anything that moved. `go.mod`'s pin is
the version of the contract this build speaks. Only `internal/ctlproto` (the
socket envelope) and `internal/integration` (the env contract) are still
client-side copies, because those live under cats' `internal/`.

### Knowing when a new agent is ready

A drop into a *new* session cannot paste immediately: the agent is still
starting, and keystrokes that arrive before its input box is drawn are simply
lost. So `waitForAgentReady` (in `client.go`) holds the prompt until the pane
looks ready.

For Claude Code it does that by watching the pane's output for any of the
banner and footer strings in `claudeReadyProbes` — `"Claude Code v"` and
`"Welcome back"` for the 2.x startup box, `"Welcome to Claude"`, `"for
shortcuts"` and `"/help for help"` for older layouts, plus `"esc to interrupt"`
and `"Bypassing Permissions"` for a session that came up already busy. They go
to the server as one alternation regex, so a single `pane.wait_for_output`
waiter matches whichever the running version happens to draw, with a 12s
deadline. Any other agent has no banner we know, so it waits for the pane's
first non-blank byte (the pane is exec'd straight into the agent, so that byte
is the agent and not a shell prompt) and then gives it a 600ms settle.

The match is best effort: on timeout the prompt is pasted anyway. That makes a
stale probe a silent cost rather than a failure — every new-session drop pays
the full 12s wait before pasting. Claude Code 2.1.x did exactly this by
replacing the strings the old list probed for, which is why the list is
version-agnostic now (`"Claude Code v"` rather than any particular version) and
why a slow drop is worth checking here first: capture a startup, and see
whether anything in the list still appears in it.

The probes contain spaces on purpose. A TUI draws word gaps as cursor-column
jumps rather than literal spaces, but catway's output stripper renders each
movement sequence as a single separator, so `"Welcome back"` reaches the
matcher spaced. Against a catway older than that fix, the spaced probes never
match and drops quietly fall back to the timeout.
