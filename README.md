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
`ctrl+s` (or `shift+enter`) saves. Outside cats it still manages backlogs; only drops need the
socket.

The editor has its own row of buttons — **Save**, **Newline**, **Images**,
**Session**, **Send**, **Cancel** — and **Send** is the one way to hand a prompt
straight to an agent without going back to the list: it saves what you have
typed and opens the target picker on it, so a prompt written from scratch
reaches a session in one gesture. It is the only button on the row with no
chord, and that is deliberate — it is click-only, because the editor is a screen
you are typing into and a key one slip from the caret is how a half-written
prompt gets sent. An empty prompt is refused there exactly as **Save** refuses
it, and everything the picker itself refuses (no socket, a drop already in
flight, a frozen prompt) still leaves your edit saved.

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

The filter rides on the header line — the 🔍 box next to the title, lit while
it holds the keys — and typing from anywhere lands in it. Under the header sits
a row of action buttons — **Add**, **Edit**, **Send**, **Export**, **Delete**
— each labelled with the chord it stands for. `tab` walks the focus
out of the filter and across them (`shift+tab` walks back, `←`/`→` move along
the row, `enter` presses, `esc` returns to the filter); `↑`/`↓` keep moving the
row highlight the whole time, so you can pick a prompt and then press the button
that acts on it. Typing anything hands the focus straight back to the filter. A
button that needs a highlighted prompt is greyed out until there is one.

Both button rows shrink rather than wrap as the pane narrows: the chips give up
their chords first, then their words, then the gaps between them, down to a row
of bare glyphs — `✔ ↵ ☑ ❐ ⚙ ✉ ✖` in the editor — and the footer names every chord
the chips stopped teaching. No button is ever dropped, however narrow the pane;
a control that vanishes is one you cannot learn is there.

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
only the list asks for idle motion, which is what the hover card is drawn from;
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
❯ ○ ▲ 🍏 fix the drop path        the daemon cannot resolve a bare agent name
  ○ △ context menu grammar        right click across the manager screens
  ○ ⚑ port the export picker      blocked until the api rename lands
  ○ 🍏 bump the version           two files, one number
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

Three annotations exist today:

| Mark | Means | Set by |
|---|---|---|
| `▲` `△` | **priority** — critical, high | the editor's **Priority** radios, `--priority` |
| `🍏` | **low-hanging fruit** — a quick win | the editor's **Quick win** checkbox, `--fruit` |
| `⚑` | **flagged** — singled out, with an optional note saying why | the editor's **Flag** checkbox and its note field, `--flag` |

Freezing is *not* an annotation. It is a state, mutually exclusive with done, and
it stays in the badge (`❄`) where the three groups are read from.

All three are stored as nothing at all when nothing has been said — so a backlog
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

#### Flag

`⚑` is the open question, where the other two are closed ones. Priority asks how
much a prompt matters and the fruit asks how cheap it is; both have an answer the
program can read. The flag says only **there is something about this one** — it
is blocked, it is waiting on an answer, it needs a word before anyone starts —
and what that something *is* goes in the flag's **note**.

That makes it the only mark that carries words, and the note is optional: a bare
`⚑` still means "look at this one", and demanding a sentence would make the mark
cost more than it is worth. Where the note exists it is shown wherever there is
room for a line of prose — hover a flagged row and the card carries it, `ctrl+v`
spells it out on the meta line as `⚑ flagged: blocked on the api rename`, and the
list's context menu prints it beside the checkbox so a decision to open the
editor is made with the current note in sight.

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

#### Where they are set

All three are set in two places, on the same controls. On the editor itself, on a
segmented bar between the title and the prompt body — two checkboxes and a radio
group, because that is what the three facts are: the fruit is independent, the
priority is exactly one of three levels, and the flag is independent again. And
on the list, without opening anything, from
[the row's context menu](#marking-priority-and-quick-wins-from-the-list) — the
same checkboxes and the same three radios, laid out down instead of across.

```
Title
fix the drop path

☐ 🍏 Quick win   Priority  (•) none   ( ) △ high   ( ) ▲ critical   ☑ ⚑ Flag
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
— the editor, its height, the toolbar, and every click hit-tested against them —
is arithmetic on a layout that must not move, and a field that pushed the form
down a row when a checkbox was ticked would slide the buttons out from under the
pointer.

A narrow pane drops the bar's words and keeps its glyphs (`☐ 🍏  ( ) –  ( ) △
( ) ▲  ☐ ⚑`); it never wraps, for the same reason — it sits on a hit-tested row.

They used to be the first two rows of the ⚙ session panel, above a seam —
accurate, but a screen away: the marks describe **the prompt**, the panel
describes **the session that will read it**, and the one screen where a prompt
is actually written showed neither. Now the mark is made in sight of the title
it qualifies, and each segment carries the glyph its choice will draw on the
list row, so the bar teaches the legend at the moment it is used. `none` is a
hole of its own rather than the absence of one, which makes clearing a level
the same gesture as setting it.

A click presses a segment without taking the keys from whichever field you are
typing in; from the keyboard, `tab` walks the form's ring (prompt → bar →
title), and on the bar `←`/`→` move between segments — the one under the
cursor is underlined — while `space` or `enter` presses. The bar joins the
ring *after* the prompt rather than in its visual place between the fields, on
purpose: the gesture this form lives on is "type a title, tab, type the
prompt", and a stop inserted into that walk would spray the prompt's first
keystrokes into a row that is not a text field.

Nothing is written until the form is saved, so an abandoned edit leaves the
marks as they were — the one difference from the context menu, which has no form
to abandon and therefore writes on the press. `ctrl+v` on a list row spells the marks out in words as
well — `▲ critical · 🍏 low-hanging fruit · ⚑ flagged: blocked on the api` on the
prompt view's meta line —
which is where to look when a glyph on a row is not yet familiar. That line is
built from the words rather than from the glyphs, so it still reads
`▲ critical · low-hanging fruit` on a finished prompt whose row has dropped the
apple: the row says what is worth doing, this screen says what was said.

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
  written by cats-todo v0.25.0 on studio.local
  → the project backlog · 9 prompts new · 3 already here, skipped

y import · tab other backlog · n / esc cancel
```

`tab` sends it to the other backlog instead, re-counting as it goes. Imported
prompts take **fresh ids** — an import is new work in *this* backlog, with its
own life from here — and a prompt whose title and text this backlog already
holds is skipped, because the common mistake is importing the same bundle
twice. A prompt whose attachment cannot be brought across still lands, without
it, and is counted: the text is the part with the value.

## Sending to a machine on the local network

cats' control socket is a unix socket — it reaches the cats on *this* machine
and nothing else. So the box on the other side of the desk needs a service of
its own:

```
$ cats-todo serve --name studio
cats-todo v0.25.0 serving on [::]:8422
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
pointer crosses, and it is paid on the list stage alone — the prompt view, the
one screen whose text gets copied out, still claims no mouse at all.

## The list's context menu

The list can do a dozen things to a prompt and the button bar has room for five,
so most of them have only ever been chords — and a chord is not something a
pointer can find. **Right-click a row** and all of them are named in one place,
on the prompt you pointed at.

```
╭─────────────────────────────────╮
│ ✎ Edit…                   enter │
│ ◉ View                   ctrl+v │
│ ✉ Send…             shift+enter │
│ ◷ Schedule…              ctrl+s │
│ ✓ Mark done              ctrl+t │
│ ❄ Freeze                 ctrl+f │
│ ☐ 🍏 Quick win                  │
│ (•) Priority: none              │
│ ( ) Priority: △ high            │
│ ( ) Priority: ▲ critical        │
│ ☑ ⚑ Flag: blocked on the api    │
│ ✓ Select             ctrl+space │
│ ➦ Export…                ctrl+o │
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
a frozen prompt, or scheduling one with no cats socket. Everything else about
the box — `↑`/`↓` and `enter`, a click off it to dismiss, any other key taking it
down, floating over the list rather than replacing it — works exactly as [the
prompt editor's context menu](#the-prompt-editors-context-menu) does, because it
is the same box.

Two rows read the *selection* rather than the prompt: **✓ Select** reads
**Unselect** on a row that is already ticked, and **➦ Export…** becomes
**➦ Export 3 prompts…** while three are held — a menu opened on one row must not
say "Export" and quietly mean four.

### Marking priority and quick wins from the list

The five middle rows are the reason this menu exists. A prompt's annotations —
its [priority](#priority), its [low-hanging fruit](#low-hanging-fruit) mark and
its [flag](#flag) — are facts you read straight off a list row, but until now the only way to *set*
one was to open the editor and find the annotation bar: a full round trip
through a form, to change a fact about a row you were already looking at.

They are the editor's controls, in the editor's glyphs, laid out down instead of
across. **☐ 🍏 Quick win** and **☐ ⚑ Flag** are checkboxes and toggle. The three
**Priority** rows are radios and set exactly their level, so pressing `▲ critical` on a prompt that
is already critical leaves it there rather than switching it off — and `none` is
a row of its own, which makes clearing a level the same gesture as setting one.
The status line names the result either way.

Unlike the editor's bar, these write **immediately**: there is no form open to
save, so the mark lands in the backlog on the press. With the priority lens on
(`ctrl+l`) raising a prompt lifts it past everything unraised, and the highlight
rides with the row so the next keystroke still acts on the prompt you just
marked; the status line says the list reordered, since on a tall pane the row can
travel most of it.

The flag row flips the mark alone — a menu row is a press, and a note is words,
so a flag raised from here comes up bare and the status line says where the words
are written. The row shows whatever note the prompt already carries, trimmed to
something a menu can hold, and clearing the flag from here takes those words with
it exactly as the editor does.

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
cats-todo add --flag="waiting on the api" …  # → flagged, with a note
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

`--priority`, `--fruit` and `--flag` set the prompt's [annotations](#annotations)
(`critical`, `high`, `none`; the `🍏` quick-win mark; and the `⚑` flag), so a prompt captured
mid-firefight arrives already marked rather than needing to be opened afterwards
to say so:

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
`--fruit` and `--flag` follow it for the same reason.

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
╰──────────────────────────────╯
```

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

The footer names the menu whenever there is a run swept, and names nothing about
it otherwise. It does not spend a second segment on `ctrl+x`: the menu prints
that chord on its own ✂ row, so one gesture on the footer teaches every key
behind it.

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
line, `←`/`→` move the carets together, `ctrl+a` takes them to the line starts and
`ctrl+e` to the line ends — prefixing, unprefixing and appending to a block, which
is what a column mode gets used for in every editor that has one. A paste goes to
every caret too; only its first line, since the rest would land somewhere no caret
was asked to be.

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

`esc` ends the mode, and so does anything that means *one* caret — `enter`, `↑`,
`↓`, a plain click. Enter in particular does not also insert: it is the key most likely
to be pressed because you thought the mode was already over. A chord the mode has
no meaning for ends it and then does its usual job, so `ctrl+s` still saves from
inside it. Nothing is undone on the way out: everything typed is already in the
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

`ctrl+s` means "save the prompt" one screen up and "save this snippet" here. That
is the same idea — commit what is in front of me — applied to what the screen is
about; the form's own save is not reachable from the picker, so there is no
press that gets the wrong one.


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
from a terminal that reports it (cats does; see `ctrl+s`/`cmd+s` below), so on a
terminal that eats the chord this is simply unavailable rather than wrong.

`alt+enter` is bound everywhere `shift+enter` is, and in the editor
`shift+enter`, `alt+enter` and `ctrl+j` all insert a newline alongside plain
`enter` — the chords the form taught first still work, and a hand that learned
them is never told it is now wrong. Distinguishing shift+enter from a bare enter
needs the kitty keyboard protocol — cats speaks it, but a terminal that does not
will send the two identically, so the footers advertise `alt+enter` (which every
terminal encodes as `ESC CR`) until the terminal answers the protocol handshake.

`cmd+s` saves the editor too, wherever the terminal is willing to report the
Command key — cats does, so under cats it is the mac chord for the mac hand.
Elsewhere the same press may never leave the terminal: Terminal.app claims Cmd
for its own menus, and iTerm2 needs the chord mapped by hand. That is why it is
an alias and never the only way in — `ctrl+s` is the binding that always works,
and it is the one the footer teaches.

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

The **☑ Spell** chip on the toolbar is the toggle on its own: its box says
whether the check is on, and clicking it flips it. Either way the choice
persists across launches (in `~/.config/cats-todo/settings.json`).

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

## Session options

A drop used to deliver one thing: the prompt. *How* the receiving agent ran —
which model, at what effort, starting from what prior context, and what to do
once the work was finished — was whatever the default was, and had to be
arranged by hand every single time. Session options make those choices a
property of the prompt instead. They are stored with it in `todos.json`, so they
travel with the repo and a drop reproduces the whole setup — whether you pressed
the key or a schedule fired it at 3am.

In the editor, `ctrl+r` opens the ⚙ panel (or click the **Session** chip).
`↑`/`↓` walk the rows, `←`/`→` (or `space`) change the one under the cursor, and
`esc` goes back to the prompt. The form shows what is set on its `⚙` line, the
list marks a configured prompt with `⚙`, and nothing is written until you save
the prompt itself.

Every row of the panel describes the session that will read the prompt. The
prompt's own marks — priority, quick win — are not here: they are set on the
editor's [annotation bar](#annotations), in sight of the title they qualify.

| Row | What it does |
|---|---|
| Model, Effort, Permission | `--model`, `--effort`, `--permission-mode` on the launch |
| Clear first | sends `/clear` as its own message before the prompt |
| Context | starts with `/sess-load [n]` or `/sess-use <pattern>` |
| Files | "also read these files" ahead of the prompt |
| Finish | commit · commit and push · run `/sess-wrap` |
| Reviews | `/code-review`, `/security-review`, `/simplify` before finishing |
| Release | cut a release once the work is done |

Three different mechanisms carry them, and which one an option rides is forced
by what the receiving end can accept. The three launch flags go on the agent's
own command line, so they only apply to a **new** session — and only to
`claude`, whose flags they are; the picker says so on any other agent's row, and
the prompt still goes. `/clear` has to be its own submitted message, because
pasted at the top of a prompt it would just be text — so it applies to a drop
into an **existing** pane. Everything else is text wrapped around the prompt
body, which works everywhere:

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
prompt. `internal/app` and `internal/ctlproto` are client-side copies of the
cats wire vocabulary and control-socket client; the wire values are the
compatibility contract, so keep them in lockstep with cats when the protocol
grows.

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
