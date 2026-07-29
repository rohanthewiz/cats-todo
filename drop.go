package main

import (
	"errors"
	"strings"
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

// dropIntoNewSession opens a fresh tab (in the active workspace — the one the
// manager pane lives in, rooted at the manager's own working directory so the
// agent sees the same project the todo was scoped to), launches the target's
// agent (claude by default),
// waits for its input UI, and delivers the prompt as typed input. Run mode adds
// a real Enter so the agent starts working; paste mode stops short so the user
// can review and edit. One delivery path for both modes — and for any agent —
// with no shell quoting to get wrong and no prompt leaking into shell history
// or `ps` output.
//
// tab.create already returns the new tab's root pane id and leaves the tab
// focused, so unlike the herdr original there is no workspace resolution or
// pane discovery step — create, label, drive.
func dropIntoNewSession(client *catsClient, act pendingAction, prompt string) error {
	command := firstNonEmpty(act.target.command, "claude")
	label := command
	if t := firstNonEmpty(act.todo.Title, firstLine(prompt, 18)); t != "" {
		label = command + ": " + truncate(t, 18)
	}

	num, pane, err := client.tabCreate(act.cwd)
	if err != nil {
		return err
	}
	// Label the tab after the work it hosts. Best effort: an unlabeled tab must
	// not abort a drop that can still deliver.
	_ = client.tabRename(num, label)

	if err := client.runCommand(pane, command); err != nil {
		return err
	}
	client.waitForAgentReady(pane, command)
	return client.sendInput(pane, prompt, act.mode == dropRun)
}
