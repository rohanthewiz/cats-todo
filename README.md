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
git log -p | cats-todo add -g -t huh   # capture piped stdin to the global backlog
cats-todo add -i ~/Desktop/shot.png this layout is wrong   # attach an image
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

In the manager, `enter` opens what is in front of you — the highlighted prompt
into the editor, or a brand-new entry when the list is empty — and `shift+enter`
drops the prompt into an agent. That opens the target picker, where `enter`
pastes the prompt staged for review and `shift+enter` submits it to run (and
marks the todo done). Inside the editor `enter` saves and `shift+enter` inserts
a newline. Outside cats it still manages backlogs; only drops need the socket.

Under the filter box sits a row of action buttons — **Add**, **Edit**, **Send**,
**Delete** — each labelled with the chord it stands for. `tab` walks the focus
out of the filter and across them (`shift+tab` walks back, `←`/`→` move along
the row, `enter` presses, `esc` returns to the filter); `↑`/`↓` keep moving the
row highlight the whole time, so you can pick a prompt and then press the button
that acts on it. Typing anything hands the focus straight back to the filter. A
button that needs a highlighted prompt is greyed out until there is one.

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

The manager wears cats' own muted green: the palette in `styles.go` is cats'
`defaultColors` (`internal/config`) — the same values the served page sets as
its `:root` custom properties — so a manager pane and the terminal around it
read as one product. Keep the two tables in sync. The greys are the exception:
cats' chrome tones are surfaces for a web page and sit too close together to
separate a terminal's four tiers of text, so the name/description/footer/done
ramp is interpolated down from `fg` toward `line` instead. The manager sets the
terminal's background and foreground while it runs and hands both back on the
way out.

## Installing

cats-todo is a cats plugin — `cats-plugin.toml` here is the reference manifest
for writing new ones. Install through the cats plugin host:

```bash
catctl plugin install rohanthewiz/cats-todo   # clone from GitHub + build
catctl plugin run rohanthewiz.cats-todo       # launch in a new tab
catctl plugin link .                          # dev mode: symlink this checkout
```

Or build it directly — it is a plain Go module:

```bash
go build -o bin/cats-todo .
```

## How it talks to cats

The manager talks to the cats server over the local control socket
(`CATS_CONTROL_SOCKET`) — the same §7 command table `catctl` drives:
`pane.list` to find agent panes, `tab.create` to open a new session,
`pane.wait_for_output` to pace launches, and `pane.send_input` to deliver the
prompt. `internal/app` and `internal/ctlproto` are client-side copies of the
cats wire vocabulary and control-socket client; the wire values are the
compatibility contract, so keep them in lockstep with cats when the protocol
grows.
