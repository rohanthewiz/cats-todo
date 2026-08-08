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
pastes the prompt staged for review and `shift+enter` submits it to run (and
marks the todo done). Inside the editor `enter` saves and `shift+enter` inserts
a newline. Outside cats it still manages backlogs; only drops need the socket.

The picker's own list is every place the prompt could land: a new Claude Code
session, a new **GitHub Copilot** session when `copilot` is on your `PATH`, a
new session for any other agent cats currently has running somewhere, then the
same set again **on a new worktree**, and finally each live agent pane with its
state and location.

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
a row of action buttons — **Add**, **Edit**, **Send**, **Delete** — each
labelled with the chord it stands for. `tab` walks the focus
out of the filter and across them (`shift+tab` walks back, `←`/`→` move along
the row, `enter` presses, `esc` returns to the filter); `↑`/`↓` keep moving the
row highlight the whole time, so you can pick a prompt and then press the button
that acts on it. Typing anything hands the focus straight back to the filter. A
button that needs a highlighted prompt is greyed out until there is one.

The pointer works too, and the same way round: a click on a button presses it, a
click on a prompt selects it (which is what makes the buttons useful with the
mouse — they act on the highlight), and a **double-click** on a prompt opens it
for editing. To send one, click the prompt and then the **Send** button, which
opens the drop picker, where a click on a target hands the prompt over. So a
prompt gets from the backlog into an agent without the keyboard — but never on
one stray gesture, and nothing is *sent* by clicking either: the picker still
asks where, and pastes without running. Submitting stays on `shift+enter`. Mouse
reporting is only asked for on the two screens with something to click; the
prompt view leaves the terminal's own text selection alone.

Completed prompts collect below the open ones, newest first, so what you just
finished is at the top of the pile rather than the bottom. `ctrl+d` folds them
away and `ctrl+w` clears them out.

`ctrl+f` **freezes** a prompt — "will not do". It is deliberately not the same
thing as done: marking work finished that nobody ever did makes the backlog lie,
and deleting it throws away the fact that the decision was made at all. A frozen
prompt is drawn `❄` in the list, dimmed but *not* struck through, and sits in its
own group between the open prompts and the completed ones. `ctrl+d` folds it away
with them, `ctrl+w` leaves it alone, and `ctrl+f` again thaws it — back into the
exact place it held, since freezing never cost it its priority. A frozen prompt
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

## Quick capture from a shell

`add` puts a prompt in the backlog without opening the manager — for the moment
you notice the thing rather than the moment you sit down to work on it. It is
the same backlog either way; nothing about the entry marks where it came from.

```bash
cats-todo add fix the flaky reconnect       # → this project's backlog
cats-todo add -g clean up the dotfiles      # → the global one
cats-todo add -t "flaky test" fix the …     # → an explicit title
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
attachment-carrying prompt with `📎n`, and `ctrl+v` lists the files.

`ctrl+r` scans `~/Desktop` and `~/Downloads`; set `CATS_TODO_IMAGE_DIR` to point
it somewhere else (macOS can be told to save screenshots anywhere, and cats-todo
does not shell out to `defaults` to find out where).

Nothing binary crosses the wire: a drop delivers the prompt with each
attachment's absolute path appended, and the agent reads the files itself. An
attachment that has since been deleted is left out of the delivered prompt and
flagged in the `ctrl+v` view rather than sent for the agent to chase. Accepted
formats are `.png`, `.jpg`/`.jpeg`, `.gif` and `.webp`, up to 10 MiB each.

`alt+enter` is bound everywhere `shift+enter` is, and `ctrl+j` also inserts a
newline in the editor. Distinguishing shift+enter from a bare enter needs the
kitty keyboard protocol — cats speaks it, but a terminal that does not will
send the two identically, so the footers advertise `alt+enter` (which every
terminal encodes as `ESC CR`) until the terminal answers the protocol
handshake.

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
`pane.list` to find agent panes, `tab.create` to open a new session already
named and running the agent (no shell in between),
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
