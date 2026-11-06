// typing.go — delivering a message to Claude Code as key presses
// (pane.send_keys) rather than as a paste (pane.send_input). N-072 (a).
//
// Why type at all. Since 2.1.283 Claude Code tags pasted input as pasted
// content, whose instructions the model follows only where the user's own
// words ask it to. A drop delivered by paste is nothing *but* paste, so a model
// could, and sometimes did, answer "your message contains only pasted text"
// and do nothing. Unattended (a scheduled drop, a batch loop) that is the
// worst failure a drop has: the pane goes idle after the question and the
// prompt counts as sent. Typed, the same text is the user's message. Measured
// live against 2.1.318 on Haiku 5.5: asked how the message reached it, the
// model said TYPED for a send_keys delivery and PASTED for the same text
// through send_input.
//
// What gets typed, and what does not:
//
//	message                         road
//	───────────────────────────────  ───────────────────────────────────────────
//	prose into a claude pane         typed: paced send_keys, shift+Enter newlines
//	"/command …"                     pasted bare, as before: a slash command is
//	                                 run, not read, and typing "/" opens the
//	                                 command menu, whose Enter could pick a
//	                                 longer command the typed name prefixes
//	"!…"                             pasted with pasteAsk: a typed "!" on an
//	                                 empty input switches Claude Code to bash
//	                                 mode, and the prompt would run as a shell
//	                                 command (seen live)
//	text holding other control       pasted with pasteAsk: there is no honest
//	characters (ESC, NUL…)           key for them, and dropping them silently
//	                                 would change the prompt
//	any other agent                  pasted with pasteAsk: shift+Enter as a
//	                                 newline is Claude Code's; another TUI may
//	                                 read it as Enter and submit half a prompt
//	cats without pane.send_keys      pasted with pasteAsk (the interim, (b))
//
// Pacing. Claude Code guesses a paste from the size of what one read hands it,
// bracketed or not: live, ~1000 typed characters in a single write came out as
// "[Pasted text #2]" (and once with the head of the text lost), while 700 went
// in as typing. So a message goes over several calls of at most typeChunk
// keys, typeGap apart. 64 keys is an order of magnitude under the threshold,
// which leaves room for the agent to fall behind for a frame or two while it
// redraws a long input; at 10ms a 3000-character prompt types in about half
// a second. The settle before the final Enter is one more gap, so the Enter
// is never read in the same chunk as the text it submits.

package main

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rohanthewiz/cats/wire"
)

const (
	// typeChunk is the most keys one pane.send_keys call carries. A newline
	// (shift+Enter) counts as one.
	typeChunk = 64
	// typeGap is the pause after each call, and before the submitting Enter.
	typeGap = 10 * time.Millisecond
	// typeTabWidth is what a tab in a prompt becomes when typed. A typed Tab is
	// a key with a meaning of its own in Claude Code (it completes), not
	// whitespace, so it goes in as the editor's own indent: four spaces, the
	// same width the form's tab indents by (promptindent.go).
	typeTabWidth = 4
)

// errNoSendKeys is a cats server that predates pane.send_keys. It is told apart
// from every other failure because nothing reached the pane, so the message
// can still go the old way.
var errNoSendKeys = errors.New("cats has no pane.send_keys")

// shiftEnter is the newline key: a line break in Claude Code's input, where a
// bare Enter submits. cats encodes it as CSI 13;2u under the kitty protocol
// and CSI 27;2;13~ without it, so it never collapses into a bare "\r".
var shiftEnter = wire.KeyStroke{Code: "Enter", Mods: wire.ModShift}

// typesKeys reports whether a message to agent goes in as key presses (see
// the table at the top of the file). It is the one place the rule lives:
// deliverPrompt and deliverLine both ask it.
func typesKeys(agent, text string) bool {
	if !isClaudeCommand(agent) {
		return false
	}
	lead := strings.TrimLeftFunc(text, unicode.IsSpace)
	if lead == "" || strings.HasPrefix(lead, "/") || strings.HasPrefix(lead, "!") {
		return false
	}
	if !utf8.ValidString(text) {
		return false
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

// typeBatches cuts text into the pane.send_keys calls that type it: runs of
// text, a shift+Enter for each line break, at most typeChunk keys per call.
// "\r\n" and a lone "\r" are line breaks too, and a tab is typeTabWidth spaces.
// The caller has already refused any other control character (typesKeys).
//
//	"ab\ncd"  →  [ {text:"ab"} {shift+Enter} {text:"cd"} ]   (one call)
//
// A call never ends up empty, and the strokes concatenate back to the text, so
// what reaches the pane is the prompt, keystroke for keystroke.
func typeBatches(text string) [][]wire.KeyStroke {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\t", strings.Repeat(" ", typeTabWidth))

	var (
		batches [][]wire.KeyStroke
		cur     []wire.KeyStroke
		run     strings.Builder // text being gathered into cur's last stroke
		n       int             // keys in cur, counting run
	)
	flushRun := func() {
		if run.Len() > 0 {
			cur = append(cur, wire.KeyStroke{Text: run.String()})
			run.Reset()
		}
	}
	flushCall := func() {
		flushRun()
		if len(cur) > 0 {
			batches = append(batches, cur)
		}
		cur, n = nil, 0
	}
	for _, r := range text {
		if n == typeChunk {
			flushCall()
		}
		if r == '\n' {
			flushRun()
			cur = append(cur, shiftEnter)
		} else {
			run.WriteRune(r)
		}
		n++
	}
	flushCall()
	return batches
}

// typeText types text into pane, paced, and presses Enter after it when
// submit is set. The first call failing with errNoSendKeys returns it before
// anything was typed; a failure after that is a real one, and the pane may
// hold part of the message, which the error says.
func typeText(d paneDriver, pane uint32, text string, submit bool) error {
	batches := typeBatches(text)
	for i, keys := range batches {
		if err := d.sendKeys(pane, keys); err != nil {
			if i == 0 {
				return err
			}
			return errors.New("typing stopped part-way (" + strconv.Itoa(i) + " of " +
				strconv.Itoa(len(batches)) + " chunks sent): " + err.Error())
		}
		time.Sleep(typeGap)
	}
	if !submit {
		return nil
	}
	return d.sendKeys(pane, []wire.KeyStroke{{Code: "Enter"}})
}

// deliverLine sends a loop's between command: the user's own command line,
// usually a slash command, so it goes bare when it is pasted — no pasteAsk,
// which would turn "/compact" into prose. Typed when typesKeys says so, like
// any message; always submitted.
func deliverLine(d paneDriver, pane uint32, agent, text string) error {
	if typesKeys(agent, text) {
		err := typeText(d, pane, text, true)
		if !errors.Is(err, errNoSendKeys) {
			return err
		}
	}
	return d.sendInput(pane, text, true)
}
