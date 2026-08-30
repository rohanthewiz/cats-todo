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
`ctrl+s` saves. Outside cats it still manages backlogs; only drops need the
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
editing, and holding the button down and moving **drags the prompt into a new
place in the list** (below). To send one, click the prompt and then the **Send**
button, which opens the drop picker, where a click on a target hands the prompt
over and starts it — a click on a row is the same choice `enter` makes, mode
and all. So a prompt gets from the backlog into an agent without the keyboard,
and never on one stray gesture: it takes a click on the prompt, a click on
**Send**, and then a click on the target you meant. Pausing instead of running
is the one thing the pointer does not offer, because it is a modifier chord.
Mouse reporting is only asked for on the screens with something to click; the
prompt view leaves the terminal's own text selection alone.

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

Completed prompts collect below the open ones, newest first, so what you just
finished is at the top of the pile rather than the bottom. `ctrl+d` folds them
away and `ctrl+w` clears them out.

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
❯ ○ ▲ 🍏  fix the drop path       the daemon cannot resolve a bare agent name
  ○ △     context menu grammar    right click across the manager screens
  ○   🍏  bump the version        two files, one number
  ○       ordinary work           nothing said about it
  ❄       shelved idea            not doing this
  ✓ ▲ 🍏  shipped it              done and dusted
```

The badge leads because it is what the list is grouped by — arriving at a row you
want "is this still work" before "how much work". Each annotation then has a
column of its own, in a fixed order, and **keeps that column on every row whether
or not the row fills it**: a mark you cannot scan straight down the pane is not
worth the cell it costs. The columns nobody in the list uses are dropped
entirely, so a backlog with nothing annotated looks exactly as it did before
annotations existed, and one that uses a single mark pays for a single column.

Two annotations exist today:

| Mark | Means | Set by |
|---|---|---|
| `▲` `△` | **priority** — critical, high | the editor's **Priority** radios, `--priority` |
| `🍏` | **low-hanging fruit** — a quick win | the editor's **Quick win** checkbox, `--fruit` |

Freezing is *not* an annotation. It is a state, mutually exclusive with done, and
it stays in the badge (`❄`) where the three groups are read from.

Both are stored as nothing at all when nothing has been said — so a backlog
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
badge two cells to its left, which is a circle in every one of its four forms
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
column rather than a fourth level: a critical one-liner is both, and one column
could only have told you one of them.

Green rather than red: the critical mark one column to its left is red already,
and two reds on a row read as one signal repeated.

#### Where they are set

Both are set on the editor itself, on a segmented bar between the title and the
prompt body — a checkbox and a radio group, because that is what the two facts
are: the fruit is independent, and the priority is exactly one of three levels.

```
Title
fix the drop path

☐ 🍏 Quick win   Priority  (•) none   ( ) △ high   ( ) ▲ critical

Prompt
…
```

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
marks as they were. `ctrl+v` on a list row spells the marks out in words as
well — `▲ critical · 🍏 low-hanging fruit` on the prompt view's meta line —
which is where to look when a glyph on a row is not yet familiar.

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

`--priority` and `--fruit` set the prompt's [annotations](#annotations)
(`critical`, `high`, `none`; and the `🍏` quick-win mark), so a prompt captured
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
`--fruit` follows it for the same reason.

Without `-g`, `add` writes to the project backlog rooted the way everything else
here roots it — nearest `.cats-todo/`, else the repo root, else the current
directory. Run it somewhere no project owns, and rather than inventing a backlog
in the current directory it stops and says so, pointing at `-g`:

```
cats-todo: no project backlog here — run from a project directory, or use -g for the global backlog
```

A prompt captured on the way past is worth little if it lands where you will
never look for it.

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
of the line, `shift+↑`/`↓` across lines — and sweeping with the mouse button
held down selects too. `ctrl+c` copies what is highlighted, and only while
something is (with nothing selected it still quits, as it does everywhere else).
Typing replaces the selection the way it does in every other editor: the next
character, newline or paste lands *on* the highlighted run rather than beside
it, and `backspace` or `delete` takes the run out. Anything else — a plain
arrow, a click, a save — simply drops the highlight, because a highlight left
standing over text the caret has walked away from is a lie about what the next
`ctrl+c` would copy.

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

**Right-click an underlined word** to open the same panel on *that* word. It is
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
