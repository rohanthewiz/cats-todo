package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

// runTodoUI renders the manager TUI in the current terminal — normally a cats
// shell pane, where the drop machinery has a control socket to talk to. It
// gathers the launch context (cwd, own pane, workspace), loads the project and
// global backlogs, and runs the manager. Drops happen in-loop, off the UI
// thread (see chooseTarget), so one manager pane serves many drops; Run() only
// returns when the user quits. scope narrows the launch to one backlog:
// --project drops the global list, --global drops the project scope.
func runTodoUI(scope launchScope) {
	// The socket client drives session drops; the manager still works without it
	// (you can add/edit/organize prompts), so a missing/unresponsive control
	// socket is not fatal here — startDrop reports it when a drop is attempted.
	client, _ := newCatsClient()

	ctx := gatherRunContext(client, scope)

	project, global, err := loadStores(ctx)
	if err != nil {
		errExit(err)
	}

	runProgram(newModel(ctx, project, global, client))
}

// runProgram runs a model as the TUI and hands the terminal back afterwards.
// It is split from runTodoUI so the hangup path can be exercised with a
// trivial model, without stores or a cats socket.
func runProgram(m tea.Model) {
	// A pane whose terminal is torn down (pty master closed, e.g. the host
	// SIGKILLed) must take the TUI with it; see terminalWatch for why bubbletea
	// alone keeps running.
	watch := watchTerminal()
	defer watch.stop()

	// The alt screen is declared by the model's View in bubbletea v2, not here.
	p := tea.NewProgram(m, watch.options()...)
	_, err := p.Run()

	// A hung-up terminal has no screen to reset and no reader for an error, and
	// stderr is that same dead tty. Return straight away so the process exits.
	if watch.gone() {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cats-todo:", err)
	}

	// Hand the terminal back on the way out. The title and the colours the view
	// sets are properties of the terminal, not of this process, so without a
	// reset they outlive the TUI — the title keeps naming a pane that is a
	// plain shell again, and the ground stays green under the shell's own
	// palette. OSC 2 with an empty payload is the standard title clear (cats
	// reads it as "no title" and the pane falls back to whatever the shell sets
	// next); OSC 111 and 110 put the background and foreground back to the
	// terminal's defaults. Bubbletea v2 does all three itself when it tears the
	// view down; these stay as the backstop for an exit that skips the
	// renderer, and print nothing visible either way.
	fmt.Print("\x1b]2;\a\x1b]111\a\x1b]110\a")
}
