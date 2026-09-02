// Command cats-todo is a prompt-backlog manager for cats: save prompts of
// future work (per-project and globally), then "drop" one into a Claude Code
// session — an existing agent pane or a freshly launched one — either staged
// for review or submitted to run.
//
// It is the cats port of herdr-todo (github.com/rohanthewiz/herdr-todo). It
// runs as a cats plugin (`catctl plugin run rohanthewiz.cats-todo`) or as a
// standalone TUI directly in any cats shell pane:
//
//	cats-todo                     open the manager in this pane (project + global)
//	cats-todo -p | --project      open it on this project's backlog only
//	cats-todo -g | --global       open it on the global backlog only
//	cats-todo add [-g] [-t] [-i] ...  quick-capture a prompt (-i attaches an image)
//	                                  …plus the session options: --model, --effort,
//	                                  --perm, --clear, --sess-load, --sess-use,
//	                                  --ctx, --finish, --review, --release
//	cats-todo init [-f]           create this project's backlog (committed with the repo)
//	cats-todo export [-g] …       write the backlog as a bundle, mail it, or send it to a machine
//	cats-todo import <file|host>  read a bundle into a backlog
//	cats-todo serve …             offer this machine's backlogs to the local network
//	cats-todo version
//
// The manager talks to the cats server over the local control socket
// (internal/ctlproto, CATS_CONTROL_SOCKET) — the same §7 command table catctl
// drives: pane.list to find agent panes, tab.create to open a new session,
// pane.wait_for_output to pace launches, and pane.send_input to deliver the
// prompt. Run outside cats it still manages backlogs; only drops need the
// socket.
package main

import (
	"fmt"
	"os"
)

// version is the binary's version. It shows in the manager's title chip and in
// `cats-todo version`, so it has to track cats-plugin.toml's version — a stale
// const here is a wrong number on screen, not just a wrong flag output.
const version = "0.24.1"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "add":
			addFromCLI(os.Args[2:])
			return
		case "init":
			initFromCLI(os.Args[2:])
			return
		// The transfer commands (transfercli.go): the pickers' three operations
		// for the places a TUI cannot go — a script, an ssh session, a machine
		// that is only ever the receiving end.
		case "export":
			exportFromCLI(os.Args[2:])
			return
		case "import":
			importFromCLI(os.Args[2:])
			return
		case "serve":
			serveFromCLI(os.Args[2:])
			return
		// Hidden: cats's shell completion execs this (see complete.go). It has to
		// come before the flag-shaped cases below, and stays out of `help` — it is
		// a protocol, not something to type.
		case "__complete":
			completeFromCLI(os.Args[2:])
			return
		// The manager's -g mirrors add's -g: same letter, same meaning ("the
		// global backlog, not this project's"), just applied to the TUI. -p is
		// its mirror image; with neither, the manager shows both merged.
		case "-g", "--global", "global":
			runTodoUI(launchGlobalOnly)
			return
		case "-p", "--project", "project":
			runTodoUI(launchProjectOnly)
			return
		case "version", "--version", "-v", "-V":
			fmt.Println("cats-todo", version)
			return
		case "help", "--help", "-h":
			fmt.Println("usage: cats-todo [-p|-g] [add … | init [-f] | export … | import <src> | serve … | version]")
			fmt.Println("  with no arguments, opens the manager TUI on both backlogs (project + global)")
			fmt.Println("  -p / --project opens it on this project's backlog only")
			fmt.Println("  -g / --global opens it on the global backlog only")
			fmt.Println("  add -i / --image attaches an image (repeatable); it rides along when dropped")
			fmt.Println("  add --model/--effort/--perm/--clear/--sess-load/--sess-use/--ctx/--finish/--review/--release")
			fmt.Println("      record how the agent receiving the prompt should be set up (ctrl+r in the editor)")
			fmt.Println("  init creates this project's backlog (.cats-todo/, committed with the repo)")
			fmt.Println("  export [-g] [--all] [--out DIR] [--markdown] [--to HOST] [--mail]")
			fmt.Println("      write the backlog as a bundle, mail it, or send it to a machine running `serve`")
			fmt.Println("  import [-g] <file|directory|host>  read a bundle into a backlog")
			fmt.Println("  serve [--port N] [--name LABEL] [--inbox project|global]")
			fmt.Println("      offer this machine's backlogs to the local network (prints the shared token)")
			return
		default:
			errExit(fmt.Sprintf("unknown subcommand %q — run `cats-todo help`", os.Args[1]))
		}
	}
	runTodoUI(launchBoth)
}
