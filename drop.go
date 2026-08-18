package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rohanthewiz/cats-todo/internal/app"
)

// performDrop carries out the chosen drop. For an existing pane it types the
// prompt straight in; for a new session it opens a tab, launches the agent, and
// feeds the prompt. In both cases dropRun submits with Enter while dropPaste
// leaves the text unsubmitted for the user to review.
func performDrop(client *catsClient, act pendingAction) error {
	if client == nil {
		return errors.New("cats control socket unavailable")
	}
	prompt := composePrompt(act.todo.Prompt, act.images, act.todo.Session)
	switch act.target.kind {
	case targetExistingPane:
		// "Clear first" is delivered as its own submitted message, because
		// /clear is a built-in of the agent's input rather than anything this
		// prompt could carry: pasted at the top of a body it would be text.
		//
		// A failed /clear aborts the drop rather than typing the prompt anyway.
		// The user asked for a session with nothing behind it; delivering into
		// whatever state the pane is actually in — half-cleared, or a modal
		// waiting on an answer — is the one outcome they ruled out.
		if act.todo.Session != nil && act.todo.Session.Clear {
			if err := client.sendInput(act.target.pane, "/clear", true); err != nil {
				return fmt.Errorf("clearing the pane first: %w", err)
			}
			// The agent has to finish handling /clear before it will read the
			// next keystrokes; typing into the gap loses the head of the prompt.
			time.Sleep(clearSettle)
		}
		if err := client.sendInput(act.target.pane, prompt, act.mode == dropRun); err != nil {
			return err
		}
		// Switch to the pane we just dropped into, mirroring how a new-session
		// drop focuses its freshly-created tab. Best effort: the prompt is
		// already delivered, so a focus failure must not fail the drop.
		_ = client.focusPane(act.target.pane)
		return nil
	case targetNewSession:
		return dropIntoNewSession(client, act, prompt)
	}
	return errors.New("unknown drop target")
}

// imageBlockHeader introduces the attachment paths appended to a dropped
// prompt. It is a sentence rather than a bare "Images:" label because a path
// sitting in a prompt reads as a mention — the agent has to be told the file is
// there to be opened.
const imageBlockHeader = "Attached images — read these files:"

// clearSettle is how long a drop waits after submitting /clear before typing
// the prompt. Long enough for the agent to swap its conversation out, short
// enough that a drop still feels like one action; there is nothing on the wire
// to wait *for*, since /clear produces no output pane.wait_for_output could
// match on.
const clearSettle = 400 * time.Millisecond

// newSessionSettle is an extra pause between "the agent looks ready" and the
// first keystroke of a dropped prompt.
//
// waitForAgentReady already waits on the wire — a banner probe for claude, first
// output for anything else — but a banner is drawn well before the agent is
// actually listening: there is still a TUI to lay out, a terminal to put into
// raw mode, and (for claude) a session to restore. Keystrokes typed into that
// window are read by whatever owns the tty at the time and are simply lost,
// which shows up as a prompt arriving with its head bitten off, or not at all.
//
// Two seconds is deliberately generous rather than tuned. It is paid once per
// new-session drop, against an agent run that lasts minutes; a drop that lands
// intact two seconds later beats a fast drop that has to be retyped. This is
// only on the new-session path — an existing pane's agent has been up for a
// while and needs no such grace.
const newSessionSettle = 2 * time.Second

// composePrompt is the text actually delivered to an agent: the prompt body,
// wrapped in whatever the todo's session options ask for and followed by one
// absolute path per attachment.
//
// The blocks, in delivery order:
//
//	preamble    what to load before reading the prompt (/sess-load, extra files)
//	the body    the prompt as written
//	images      one absolute path per attachment
//	postamble   what to do once the work is done (reviews, commit, release)
//
// Every block is omitted entirely when it has nothing to say, and the separator
// between two blocks is a blank line — so a todo with no options and no images
// still composes to exactly its own prompt text, byte for byte. That is the
// compatibility contract SessionOpts carries, and TestComposePrompt is what
// holds us to it.
//
// The image block stays where it has always been, between the body and anything
// that follows: it is part of the request, and a wrap-up instruction reads as
// the last word only if nothing comes after it.
//
// Paths are given one per line and bare — no "@" prefix, which Claude Code's
// input treats as the start of a file-picker mention and would rewrite
// mid-paste. This is the whole of image "support" on the wire: pane.send_input
// types keystrokes, so the bytes of an image can never cross it; the path can,
// and the agent reads the file itself. The prompt body is a different matter:
// it goes over verbatim, and it may well carry "@path" mentions of its own —
// the editor's file picker (filepick.go) writes exactly those, at the author's
// request, and the whole point of them is that the agent reads them as
// mentions. Only the block this program composes on its own stays bare.
//
// The result never ends in a newline: sendInput's contract is that a trailing
// newline in the text would be inserted literally by the line editor rather
// than submitting (submission is the separate Enter that dropRun sends).
func composePrompt(prompt string, images []string, opts *SessionOpts) string {
	var b strings.Builder
	block := func(s string) {
		if s == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(s)
	}

	block(opts.preamble())
	block(prompt)
	if len(images) > 0 {
		var img strings.Builder
		img.WriteString(imageBlockHeader)
		for _, p := range images {
			img.WriteString("\n")
			img.WriteString(p)
		}
		block(img.String())
	}
	block(opts.postamble())
	return b.String()
}

// performScheduledDrop is the fire-time drop: performDrop with the two things
// an unattended delivery changes. The mode is forced to dropRun — a paste has
// nobody standing by to press Enter — and a pane target is re-verified against
// pane.list first, because panes are ephemeral and the schedule may be hours
// old. A vanished pane is an error rather than a fallback into a new session:
// picking that pane was picking that conversation's context, and silently
// launching an agent run on guessed context is the worse failure. The caller
// records the error as Missed, where a manual send is one keystroke away.
func performScheduledDrop(client *catsClient, sc Schedule, act pendingAction) error {
	if client == nil {
		return errors.New("cats control socket unavailable")
	}
	if sc.Kind == scheduleKindPane {
		panes, err := client.paneList()
		if err != nil {
			return fmt.Errorf("checking the scheduled pane: %w", err)
		}
		if !paneExists(panes, sc.Pane) {
			return errors.New("the scheduled pane is gone — send manually")
		}
	}
	act.mode = dropRun
	return performDrop(client, act)
}

// paneExists reports whether pane.list still knows the pane id — split out
// pure so the fire path's one judgment call is testable without a socket.
func paneExists(panes []app.PaneInfo, id uint32) bool {
	for _, p := range panes {
		if p.Pane == id {
			return true
		}
	}
	return false
}

// dropIntoNewSession opens a fresh tab (in the active workspace — the one the
// manager pane lives in, rooted at the manager's own working directory so the
// agent sees the same project the todo was scoped to) running the target's
// agent (claude by default), waits for its input UI, and delivers the prompt as
// typed input. Run mode adds a real Enter so the agent starts working; paste
// mode stops short so the user can review and edit. One delivery path for both
// modes — and for any agent — with no shell quoting to get wrong and no prompt
// leaking into shell history or `ps` output.
//
// The tab is created named and already running the agent: tab.create's Title
// and Command do in one round trip what used to take four (create, rename, type
// the command into a shell, submit it), and skip a shell startup entirely. The
// agent is the pane's own process, so quitting it closes the tab rather than
// dropping back to a prompt — the same shape as a cats agent pane.
//
// tab.create returns the new tab's root pane id and leaves the tab focused, so
// there is no workspace resolution or pane discovery step — create, then drive.
//
// A worktree target inserts one step ahead of all that: cut a checkout, and
// root the tab there instead (see dropWorktree). Everything downstream is
// unchanged, which is the point — the isolation is a property of the directory
// the agent starts in, not of how the prompt is delivered.
func dropIntoNewSession(client *catsClient, act pendingAction, prompt string) error {
	command := firstNonEmpty(act.target.command, "claude")
	label := command
	if t := firstNonEmpty(act.todo.Title, firstLine(prompt, 18)); t != "" {
		label = command + ": " + truncate(t, 18)
	}

	cwd := act.cwd
	if act.target.worktree {
		path, err := dropWorktree(client, act)
		if err != nil {
			return err
		}
		cwd = path
	}

	// Fields, not a shell: the command is an agent name (possibly with flags),
	// and tab.create execs the argv directly. The todo's session flags go last,
	// after anything the target's own command carried — claude takes the last
	// spelling of a repeated flag, so the prompt's explicit choice outranks the
	// picker row's default, which is the right way round. launchArgs yields
	// nothing for a non-claude agent (see its comment), which is what keeps this
	// one line right for every row in the picker.
	argv := append(strings.Fields(command), act.todo.Session.launchArgs(command)...)
	_, pane, err := client.tabCreate(cwd, label, argv)
	if err != nil {
		return err
	}
	client.waitForAgentReady(pane, command)
	// Unconditional, including after a probe timeout: the timeout case is the one
	// where we know least about the agent's state, so it is the last place to
	// start typing early.
	time.Sleep(newSessionSettle)
	return client.sendInput(pane, prompt, act.mode == dropRun)
}

// dropWorktree cuts the checkout a worktree drop lands in and returns its path.
//
// The whole of the git work happens server-side (worktree.create: `git worktree
// add -b <branch>` off the anchoring pane's repo at HEAD, then a new workspace
// created, focused and named after the branch). Two consequences shape what
// follows this call:
//
//   - The new workspace is already active, so the tab.create that comes next
//     lands in it with no workspace argument to pass — the same "create, then
//     drive" flow an ordinary new-session drop uses. It also arrives with one
//     shell pane of its own, which is the workspace's minimum and a useful
//     thing to have next to an agent working a branch.
//   - The returned path is a real directory, so it is passed to tab.create
//     explicitly rather than relying on the workspace's identity cwd. A tab
//     rooted anywhere else would defeat the entire exercise, so this is the one
//     value in the flow worth being literal about.
//
// A failure here aborts the drop rather than falling back to the project
// checkout: choosing "on a new worktree" is choosing isolation, and quietly
// starting an agent in the shared tree instead is precisely the outcome the
// user asked to avoid.
func dropWorktree(client *catsClient, act pendingAction) (string, error) {
	branch := todoBranchName(act.todo, time.Now().UnixMicro())
	res, err := client.worktreeCreate(act.anchorPane, branch)
	if err != nil {
		return "", fmt.Errorf("creating the worktree: %w", err)
	}
	if res.Path == "" {
		// Defensive: a server that reported success without a checkout path
		// leaves us nothing to root the tab at, and tab.create would silently
		// fall back to the workspace default.
		return "", errors.New("cats created the worktree but reported no path")
	}
	return res.Path, nil
}
