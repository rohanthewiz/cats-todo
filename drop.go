package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/rohanthewiz/cats/wire"
)

// performDrop carries out the chosen drop. For an existing pane it types the
// prompt straight in; for a new session it opens a tab, launches the agent, and
// feeds the prompt. In both cases dropRun — the picker's default, since a drop
// is a request for the work to start — submits with Enter, while dropPaste is
// the opt-in pause that leaves the text unsubmitted for the user to review.
//
// The note is for the status line: something the drop did on the user's behalf
// that they should hear about even though it succeeded (see applyPaneSetup).
func performDrop(client *catsClient, act pendingAction) (note string, err error) {
	l, err := performDropAt(client, act)
	return l.note, err
}

// dropLanding is where a drop put its prompt: the pane that received it and,
// for a worktree drop, the branch cut for it — plus performDrop's status note.
//
// Most callers need only the note, which is why performDrop keeps its old
// shape. A batch loop needs the pane: it is the pane the loop then watches for
// the agent going idle, and — in a same-session loop — the pane every later
// prompt is typed into. The branch is recorded so a batch's history can say
// which checkout each prompt ran in.
type dropLanding struct {
	note   string
	pane   uint32
	branch string
}

// performDropAt is performDrop reporting where the prompt landed.
func performDropAt(client *catsClient, act pendingAction) (dropLanding, error) {
	if client == nil {
		return dropLanding{}, errors.New("cats control socket unavailable")
	}
	prompt := composePrompt(act.todo.Prompt, act.images, act.todo.Session)
	switch act.target.kind {
	case targetExistingPane:
		// The prompt's session settings are applied to the running session
		// first — /clear, then /model and /effort for a claude pane (see
		// paneSetupCommands). Each is delivered as its own submitted message,
		// because they are built-ins of the agent's input rather than anything
		// this prompt could carry: pasted at the top of a body they would be
		// text. They are submitted in paste mode too — dropPaste pauses before
		// the *prompt* goes, and a setting left typed-but-unsent would sit in
		// the input box with the prompt glued onto the end of it.
		//
		// A failed command aborts the drop rather than typing the prompt anyway.
		// The user asked for this prompt to run on a particular setup;
		// delivering into whatever state the pane is actually in — half-cleared,
		// on the wrong model, or a modal waiting on an answer — is the one
		// outcome they ruled out. The switch confirm Claude Code raises
		// mid-conversation is the modal that does come up, and applyPaneSetup
		// (panesetup.go) is what answers it.
		//
		// Between commands the agent has to finish handling one before it
		// will read the next keystrokes; typing into the gap loses the head of
		// whatever comes next. That wait is clearSettle, spent inside
		// applyPaneSetup.
		note, err := applyPaneSetup(client, act.target.pane, act.todo.Session.paneSetupCommands(act.target.agent), clearSettle)
		if err != nil {
			return dropLanding{}, err
		}
		if err := client.sendInput(act.target.pane, prompt, act.mode == dropRun); err != nil {
			return dropLanding{}, err
		}
		// Switch to the pane we just dropped into, mirroring how a new-session
		// drop focuses its freshly-created tab. Best effort: the prompt is
		// already delivered, so a focus failure must not fail the drop.
		_ = client.focusPane(act.target.pane)
		return dropLanding{note: note, pane: act.target.pane}, nil
	case targetNewSession:
		pane, branch, err := dropIntoNewSession(client, act, prompt)
		return dropLanding{pane: pane, branch: branch}, err
	}
	return dropLanding{}, errors.New("unknown drop target")
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
func performScheduledDrop(client *catsClient, sc Schedule, act pendingAction) (string, error) {
	l, err := performScheduledDropAt(client, sc, act)
	return l.note, err
}

// performScheduledDropAt is performScheduledDrop reporting where the prompt
// landed (see dropLanding).
func performScheduledDropAt(client *catsClient, sc Schedule, act pendingAction) (dropLanding, error) {
	if client == nil {
		return dropLanding{}, errors.New("cats control socket unavailable")
	}
	if sc.Kind == scheduleKindPane {
		panes, err := client.paneList()
		if err != nil {
			return dropLanding{}, fmt.Errorf("checking the scheduled pane: %w", err)
		}
		if !paneExists(panes, sc.Pane) {
			return dropLanding{}, errors.New("the scheduled pane is gone — send manually")
		}
	}
	act.mode = dropRun
	return performDropAt(client, act)
}

// paneExists reports whether pane.list still knows the pane id — split out
// pure so the fire path's one judgment call is testable without a socket.
func paneExists(panes []wire.PaneInfo, id uint32) bool {
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
//
// It returns the new tab's root pane — the agent's pane — and the worktree's
// branch when it cut one. The pane is returned even when the final send fails,
// since the tab exists by then and is where the user will find the agent.
func dropIntoNewSession(client *catsClient, act pendingAction, prompt string) (pane uint32, branch string, err error) {
	command := firstNonEmpty(act.target.command, "claude")
	label := command
	if t := firstNonEmpty(act.todo.Title, firstLine(prompt, 18)); t != "" {
		label = command + ": " + truncate(t, 18)
	}

	cwd := act.cwd
	if act.target.worktree {
		path, br, err := dropWorktree(client, act)
		if err != nil {
			return 0, "", err
		}
		cwd, branch = path, br
	}

	// Fields, not a shell: the command is an agent name (possibly with flags),
	// and tab.create execs the argv directly. The todo's session flags go last,
	// after anything the target's own command carried — claude takes the last
	// spelling of a repeated flag, so the prompt's explicit choice outranks the
	// picker row's default, which is the right way round. launchArgs yields
	// nothing for a non-claude agent (see its comment), which is what keeps this
	// one line right for every row in the picker.
	argv := append(strings.Fields(command), act.todo.Session.launchArgs(command)...)
	argv[0] = resolveAgentPath(argv[0])
	_, pane, err = client.tabCreate(cwd, label, argv)
	if err != nil {
		return 0, branch, err
	}
	client.waitForAgentReady(pane, command)
	// Unconditional, including after a probe timeout: the timeout case is the one
	// where we know least about the agent's state, so it is the last place to
	// start typing early.
	time.Sleep(newSessionSettle)
	return pane, branch, client.sendInput(pane, prompt, act.mode == dropRun)
}

// resolveAgentPath turns an agent's name into the absolute path of the binary,
// when this process can find one.
//
// The manager runs inside a pane, so its PATH is the user's — the login shell
// built it. cathost, which is what actually exec's the argv, may not be in that
// position at all: a GUI-launched Cats.app inherits launchd's bare
// /usr/bin:/bin:/usr/sbin:/sbin, and Go resolves a bare program name against
// the *spawning process's* PATH, never against the environment prepared for the
// child. An agent installed under ~/.local/bin (or a version manager's shim
// directory) is then unreachable, cathost degrades the pane to a plain shell,
// and the drop types a whole prompt at a shell prompt — which, in run mode,
// the shell then tries to execute.
//
// Sending the path we resolved here removes the daemon's PATH from the
// question. It is best effort in both directions: an unresolvable name is sent
// through unchanged (the daemon may well have a PATH we do not, and its own
// error message is the better one), and a name that is already a path is left
// alone. Newer cats daemons resolve this themselves; passing an absolute path
// is what makes a drop work against the ones that do not.
func resolveAgentPath(name string) string {
	if name == "" || strings.ContainsRune(name, os.PathSeparator) {
		return name
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return name
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
//
// The branch returned is the server's resolved name (it may differ from the one
// asked for), falling back to the one asked for when the server says nothing.
func dropWorktree(client *catsClient, act pendingAction) (path, branch string, err error) {
	branch = todoBranchName(act.todo, time.Now().UnixMicro())
	res, err := client.worktreeCreate(act.anchorPane, branch)
	if err != nil {
		return "", "", fmt.Errorf("creating the worktree: %w", err)
	}
	if res.Path == "" {
		// Defensive: a server that reported success without a checkout path
		// leaves us nothing to root the tab at, and tab.create would silently
		// fall back to the workspace default.
		return "", "", errors.New("cats created the worktree but reported no path")
	}
	return res.Path, firstNonEmpty(res.Branch, branch), nil
}
