package main

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakePane is a scripted pane: a screen that each send can redraw, and a
// record of what was sent. Its waits match against the screen as it stands,
// which is what the server's seed does. The live stream isn't modelled,
// because the fake redraws synchronously inside sendInput.
type fakePane struct {
	screen string
	// react maps a sent line ("" is a bare Enter) to the screen it leaves.
	react map[string]string
	sent  []string
}

func (f *fakePane) sendInput(_ uint32, text string, submit bool) error {
	if !submit {
		panic("setup commands are always submitted")
	}
	f.sent = append(f.sent, text)
	if s, ok := f.react[text]; ok {
		f.screen = s
	}
	return nil
}

func (f *fakePane) waitForOutputIn(_ uint32, pattern string, regex bool, _ uint32, _ time.Duration) (bool, error) {
	if regex {
		return regexp.MustCompile(pattern).MatchString(f.screen), nil
	}
	return strings.Contains(f.screen, pattern), nil
}

// The dialogs as Claude Code 2.1.282 draws them, reduced to the lines the
// watch reads.
const (
	modelDialog  = "Switch model?\nYour next response will be slower and use more tokens\n❯ 1. Yes, switch to Opus\n  2. No, go back"
	effortDialog = "Change effort level?\nYour next response will be slower and use more tokens\n❯ 1. Yes, switch to high\n  2. No, go back"
	hookDialog   = "Switch model?\nA PreModelSwitch hook asked you to confirm\n❯ 1. Yes, switch to Opus\n  2. No, go back"
	idle         = "> \n  ? for shortcuts"
)

// TestPaneSetupAnswersTheSwitchConfirm is N-018. Mid-conversation, /model
// opens a dialog that takes the next Enter. Unanswered, the /effort line
// (and then the prompt) would be typed into it and lost.
func TestPaneSetupAnswersTheSwitchConfirm(t *testing.T) {
	p := &fakePane{screen: idle, react: map[string]string{
		"/model opus": modelDialog,
		"":            idle,
	}}
	note, err := applyPaneSetup(p, 1, []string{"/model opus", "/effort high"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/model opus", "", "/effort high"}; !slices.Equal(p.sent, want) {
		t.Errorf("sent %q, want %q — the dialog answered before /effort goes", p.sent, want)
	}
	if !strings.Contains(note, "confirmed the model switch") {
		t.Errorf("note = %q, want it to say the switch was confirmed", note)
	}
}

// TestPaneSetupAnswersTheEffortConfirm: /effort raises the same dialog on its
// own when the model didn't change but the effort did.
func TestPaneSetupAnswersTheEffortConfirm(t *testing.T) {
	p := &fakePane{screen: idle, react: map[string]string{
		"/effort high": effortDialog,
		"":             idle,
	}}
	note, err := applyPaneSetup(p, 1, []string{"/model opus", "/effort high"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/model opus", "/effort high", ""}; !slices.Equal(p.sent, want) {
		t.Errorf("sent %q, want %q", p.sent, want)
	}
	if !strings.Contains(note, "effort switch") || strings.Contains(note, "model") {
		t.Errorf("note = %q, want it to name the effort switch only", note)
	}
}

// TestPaneSetupNoDialogIsUnchanged: a cold cache or an unchanged model shows
// nothing, and the drop sends what it always sent.
func TestPaneSetupNoDialogIsUnchanged(t *testing.T) {
	p := &fakePane{screen: idle}
	note, err := applyPaneSetup(p, 1, []string{"/model opus", "/effort high"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/model opus", "/effort high"}; !slices.Equal(p.sent, want) {
		t.Errorf("sent %q, want %q", p.sent, want)
	}
	if note != "" {
		t.Errorf("note = %q, want nothing to report", note)
	}
}

// TestPaneSetupAfterClearDoesNotWatch: /clear acknowledges the cache miss in
// Claude Code, so the gate can't fire and there is nothing to look for. A
// screen that looks like the dialog must not draw an Enter.
func TestPaneSetupAfterClearDoesNotWatch(t *testing.T) {
	p := &fakePane{screen: idle, react: map[string]string{"/model opus": modelDialog}}
	if _, err := applyPaneSetup(p, 1, []string{"/clear", "/model opus"}, 0); err != nil {
		t.Fatal(err)
	}
	if want := []string{"/clear", "/model opus"}; !slices.Equal(p.sent, want) {
		t.Errorf("sent %q, want %q — no Enter after a /clear", p.sent, want)
	}
}

// TestPaneSetupLeavesAHookAskAlone: a PreModelSwitch hook's ask is the user's
// own policy. The drop stops with the dialog still up, and neither answers it
// nor types anything more into it.
func TestPaneSetupLeavesAHookAskAlone(t *testing.T) {
	p := &fakePane{screen: idle, react: map[string]string{"/model opus": hookDialog}}
	_, err := applyPaneSetup(p, 1, []string{"/model opus", "/effort high"}, 0)
	if err == nil || !strings.Contains(err.Error(), "PreModelSwitch hook") {
		t.Fatalf("err = %v, want the drop stopped on the hook's ask", err)
	}
	if want := []string{"/model opus"}; !slices.Equal(p.sent, want) {
		t.Errorf("sent %q, want %q", p.sent, want)
	}
}

// TestPaneSetupDistrustsAStaleScreen: the dialog's words already on screen
// before /model goes (an old dialog, or conversation quoting it) can't be
// told from a new one, so the drop falls back to its old behaviour instead of
// pressing Enter on a guess.
func TestPaneSetupDistrustsAStaleScreen(t *testing.T) {
	p := &fakePane{screen: "we talked about the Switch model? dialog\n> "}
	if _, err := applyPaneSetup(p, 1, []string{"/model opus"}, 0); err != nil {
		t.Fatal(err)
	}
	if want := []string{"/model opus"}; !slices.Equal(p.sent, want) {
		t.Errorf("sent %q, want %q — no Enter on a stale match", p.sent, want)
	}
}

// TestDropDoneStatusCarriesTheNote: a confirm answered on the user's behalf
// is named on the success line, ahead of the paste mode's instruction.
func TestDropDoneStatusCarriesTheNote(t *testing.T) {
	note := "confirmed the model switch (the conversation is re-read uncached)"
	run := dropDoneStatus(dropResultMsg{desc: "pane 3", mode: dropRun, note: note})
	if run != "dropped → pane 3 · "+note {
		t.Errorf("run status = %q", run)
	}
	paste := dropDoneStatus(dropResultMsg{desc: "pane 3", mode: dropPaste, note: note})
	if !strings.HasSuffix(paste, "press enter there to run") || !strings.Contains(paste, note) {
		t.Errorf("paste status = %q, want the note and the instruction last", paste)
	}
}
