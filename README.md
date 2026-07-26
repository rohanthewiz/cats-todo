# cats-todo — a prompt backlog for cats

`cats-todo` (ported from [herdr-todo](https://github.com/rohanthewiz/herdr-todo))
is a TUI prompt-backlog manager for [cats](https://github.com/rohanthewiz/cats):
save prompts of future work per-project (`.cats-todo/todos.json`, committed with
the repo) or globally (`~/.config/cats-todo/`), then *drop* one into a Claude
Code session — an existing agent pane (the picker lists every detected agent
pane with its state and location) or a fresh tab that launches the agent first.

```bash
cats-todo                              # open the manager in the current pane
cats-todo add fix the flaky reconnect  # quick-capture to the project backlog
git log -p | cats-todo add -g -t huh   # capture piped stdin to the global backlog
```

Both the manager and `add` scope the project backlog to the same place: the
nearest ancestor holding a `.cats-todo/` directory, else the repo root, else the
current directory — so it does not matter which subdirectory of a project you
launch from. A drop into a fresh tab roots that tab there too.

In the manager: `enter` opens the target picker, then `enter` pastes the prompt
staged for review while `ctrl+r` submits it to run (and marks the todo done).
Outside cats it still manages backlogs; only drops need the socket.

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
