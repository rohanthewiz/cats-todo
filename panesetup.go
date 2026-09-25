// panesetup.go — applying a todo's session settings to a running Claude Code
// pane, and answering the confirm Claude Code raises when a switch lands
// mid-conversation.
//
// A drop into an existing pane types /clear, /model and /effort ahead of the
// prompt (paneSetupCommands, session.go). With Clear off, the model and effort
// switches land in a live conversation, and Claude Code (checked against
// 2.1.282) guards those with a modal, drawn roughly like this:
//
//	╭ Switch model? ─────────────────────────────────────────────╮
//	│ Your next response will be slower and use more tokens      │
//	│ This conversation is cached for the current model.         │
//	│ Switching to Opus means the full history gets re-read …    │
//	│ ❯ 1. Yes, switch to Opus                                   │
//	│   2. No, go back                                           │
//	╰────────────────────────────────────────────────────────────╯
//
// ("Change effort level?" is the same dialog for /effort.) Its gate is
// `Bvn` for the model and `mCe` for the effort. It opens when all of these hold:
//
//   - the prompt cache is warm: the last main-thread request is inside the
//     cache TTL (five minutes or an hour);
//   - the session has produced output since the last acknowledged switch
//     (/clear and compaction both count as acknowledging, which is why a drop
//     with Clear on never meets it);
//   - the target really differs: another model than the current one and the
//     one that sent the last response, or another effective effort level.
//
// That is exactly the pane a Clear-off drop is usually aimed at: one someone
// was just working in. Unanswered, the modal takes the next Enter. Enter from
// the /effort line or from the prompt answers it (focus is on Yes), and the
// text typed before that Enter goes nowhere, so the drop would report success
// while the prompt was never delivered.
//
// So after each /model and /effort sent without a /clear before it, the drop
// watches the bottom of the pane for the dialog, and:
//
//	dialog up, a PreModelSwitch hook asked   → stop; the drop fails and says so
//	dialog up, Claude Code's own cache check → press Enter (Yes), say so
//	no dialog within the settle time         → carry on, exactly as before
//
// Pressing Yes is the reading of the todo's own settings: it asked for this
// model on this prompt, and the question the dialog puts is "switch even
// though it costs a cache miss?". The drop's status line names the confirm,
// so the cost is not paid silently. A hook's ask is different. It is a policy
// the user wrote for themselves, and answering it on their behalf would defeat
// it, so the dialog is left up for them.

package main

import (
	"fmt"
	"strings"
	"time"
)

// paneDriver is the part of catsClient a pane setup uses. It is an interface
// so the setup's decisions can be tested against a scripted pane, without a
// cats server.
type paneDriver interface {
	sendInput(pane uint32, text string, submit bool) error
	waitForOutputIn(pane uint32, pattern string, regex bool, lines uint32, timeout time.Duration) (bool, error)
}

const (
	// switchConfirmPattern matches the dialog's title in either kind, and its
	// Yes row. The row is there for a tall dialog in a narrow pane: the title
	// can sit above the seeded rows while the options, drawn last, are always
	// at the bottom.
	switchConfirmPattern = `Switch model\?|Change effort level\?|Yes, switch to `
	// switchHookMarker is the subtitle the same dialog carries when a
	// PreModelSwitch hook, not the cache check, is what asked.
	switchHookMarker = "PreModelSwitch hook asked you to confirm"
	// switchConfirmRows bounds the screen seed to where the dialog is drawn.
	// The whole buffer would also seed the conversation above it, and a
	// conversation about this very feature contains the dialog's words.
	switchConfirmRows = 16
	// switchPeek is how long a look at what is already on screen waits. The
	// seed is matched as the wait registers, so this only has to cover a
	// round trip.
	switchPeek = 50 * time.Millisecond
)

// applyPaneSetup submits the setup commands one message each, answering the
// switch confirm where one comes up. It returns a note for the drop's status
// line (empty when there is nothing to report) and an error that aborts the
// drop before the prompt is typed.
//
// settle is how long the agent gets to handle each command before the next
// keystrokes arrive (clearSettle). While a switch is being watched, the watch
// is the wait, so a pane that shows no dialog costs what it cost before.
func applyPaneSetup(d paneDriver, pane uint32, cmds []string, settle time.Duration) (string, error) {
	cleared := false
	var confirmed []string
	for _, cmd := range cmds {
		verb := strings.Fields(cmd)[0]
		watch := !cleared && (verb == "/model" || verb == "/effort")
		if watch {
			// The dialog's words already in the bottom rows means one of two
			// things: a dialog still open from before (typing /model into it
			// would be lost too), or conversation text quoting it. The two
			// can't be told apart from here, so the drop behaves the way it
			// did before this watch existed rather than guess either way.
			if stale, err := d.waitForOutputIn(pane, switchConfirmPattern, true, switchConfirmRows, switchPeek); err != nil || stale {
				watch = false
			}
		}
		if err := d.sendInput(pane, cmd, true); err != nil {
			return "", fmt.Errorf("applying %s to the pane first: %w", verb, err)
		}
		if verb == "/clear" {
			cleared = true
		}
		if !watch {
			time.Sleep(settle)
			continue
		}
		up, err := d.waitForOutputIn(pane, switchConfirmPattern, true, switchConfirmRows, settle)
		if err != nil {
			// The wait failed rather than timed out, so it may not have spent
			// the settle time. Spend it here.
			time.Sleep(settle)
			continue
		}
		if !up {
			continue
		}
		if hook, _ := d.waitForOutputIn(pane, switchHookMarker, false, switchConfirmRows, switchPeek); hook {
			return "", fmt.Errorf("the pane is asking to confirm %s (a PreModelSwitch hook) — answer it there; the prompt was not sent", cmd)
		}
		// A bare Enter presses the focused row, which the dialog opens on Yes.
		// The wire has no way to send a single key other than Enter
		// (send_input pastes its text), and Enter is the one this needs.
		if err := d.sendInput(pane, "", true); err != nil {
			return "", fmt.Errorf("confirming %s in the pane: %w", verb, err)
		}
		confirmed = append(confirmed, strings.TrimPrefix(verb, "/"))
		time.Sleep(settle)
	}
	if len(confirmed) == 0 {
		return "", nil
	}
	return "confirmed the " + strings.Join(confirmed, " and ") + " switch (the conversation is re-read uncached)", nil
}
