// Command cats-todo is a prompt-backlog manager for cats: save prompts of
// future work (per-project and globally), then "drop" one into a Claude Code
// session — an existing agent pane or a freshly launched one — either staged
// for review or submitted to run.
//
// It is the cats port of herdr-todo (github.com/rohanthewiz/herdr-todo). It
// runs as a cats plugin (`catctl plugin run rohanthewiz.cats-todo`) or as a
// standalone TUI directly in any cats shell pane:
//
//	cats-todo                     open the manager in this pane
//	cats-todo -g | --global       open the manager on the global backlog only
//	cats-todo add [-g] [-t] ...   quick-capture a prompt without the manager
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

// version is the binary's version.
const version = "0.2.2"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "add":
			addFromCLI(os.Args[2:])
			return
		// The manager's -g mirrors add's -g: same letter, same meaning ("the
		// global backlog, not this project's"), just applied to the TUI.
		case "-g", "--global", "global":
			runTodoUI(true)
			return
		case "version", "--version", "-v", "-V":
			fmt.Println("cats-todo", version)
			return
		case "help", "--help", "-h":
			fmt.Println("usage: cats-todo [-g] [add [-g] [-t title] [prompt...] | version]")
			fmt.Println("  with no arguments, opens the manager TUI in the current pane")
			fmt.Println("  -g / --global opens it on the global backlog only (no project scope)")
			return
		default:
			errExit(fmt.Sprintf("unknown subcommand %q — run `cats-todo help`", os.Args[1]))
		}
	}
	runTodoUI(false)
}
