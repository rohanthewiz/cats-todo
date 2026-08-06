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
	prompt := composePrompt(act.todo.Prompt, act.images)
	switch act.target.kind {
	case targetExistingPane:
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

// composePrompt is the text actually delivered to an agent: the prompt body,
// plus one absolute path per attachment.
//
// This is the whole of image "support" on the wire. pane.send_input types
// keystrokes, so the bytes of an image can never cross it; the path can, and
// the agent reads the file itself. Paths are given one per line and bare — no
// "@" prefix, which Claude Code's input treats as the start of a file-picker
// mention and would rewrite mid-paste.
//
// The result never ends in a newline: sendInput's contract is that a trailing
// newline in the text would be inserted literally by the line editor rather
// than submitting (submission is the separate Enter that dropRun sends).
func composePrompt(prompt string, images []string) string {
	if len(images) == 0 {
		return prompt
	}
	var b strings.Builder
	if prompt != "" {
		b.WriteString(prompt)
		b.WriteString("\n\n")
	}
	b.WriteString(imageBlockHeader)
	for _, p := range images {
		b.WriteString("\n")
		b.WriteString(p)
	}
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
	// and tab.create execs the argv directly.
	_, pane, err := client.tabCreate(cwd, label, strings.Fields(command))
	if err != nil {
		return err
	}
	client.waitForAgentReady(pane, command)
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
