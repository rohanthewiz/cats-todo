package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestComposePrompt pins the delivered text: paths get appended as their own
// lines, and the result never ends in a newline (sendInput's contract — a
// trailing newline is inserted literally instead of submitting).
//
// Every case passes nil session options, which makes this the regression guard
// on SessionOpts' compatibility contract: a prompt with no options set has to
// compose to exactly what it composed to before they existed, byte for byte.
// The options themselves are exercised in session_test.go.
func TestComposePrompt(t *testing.T) {
	t.Run("no images passes the prompt through", func(t *testing.T) {
		if got := composePrompt("just words", nil, nil); got != "just words" {
			t.Errorf("composePrompt = %q, want the prompt unchanged", got)
		}
	})

	t.Run("images are appended one per line", func(t *testing.T) {
		got := composePrompt("fix this", []string{"/tmp/a.png", "/tmp/b.png"}, nil)
		want := "fix this\n\n" + imageBlockHeader + "\n/tmp/a.png\n/tmp/b.png"
		if got != want {
			t.Errorf("composePrompt =\n%q\nwant\n%q", got, want)
		}
	})

	t.Run("no trailing newline", func(t *testing.T) {
		got := composePrompt("p", []string{"/tmp/a.png"}, nil)
		if strings.HasSuffix(got, "\n") {
			t.Errorf("composePrompt = %q, must not end in a newline", got)
		}
	})

	t.Run("an empty prompt leaves no leading blank lines", func(t *testing.T) {
		got := composePrompt("", []string{"/tmp/a.png"}, nil)
		want := imageBlockHeader + "\n/tmp/a.png"
		if got != want {
			t.Errorf("composePrompt = %q, want %q", got, want)
		}
	})

	t.Run("paths are bare", func(t *testing.T) {
		// "@" starts a file-picker mention in Claude Code's input, which would
		// rewrite the paste mid-flight.
		if got := composePrompt("p", []string{"/tmp/a.png"}, nil); strings.Contains(got, "@") {
			t.Errorf("composePrompt = %q, want no @ prefix on paths", got)
		}
	})
}

// A bare agent name is sent to cats as an absolute path when this process can
// resolve one, because the daemon that exec's it may be holding launchd's bare
// PATH (see resolveAgentPath). Anything already a path, or unresolvable, goes
// over untouched.
func TestResolveAgentPath(t *testing.T) {
	dir := t.TempDir()
	prog := filepath.Join(dir, "cats-todo-fake-agent")
	if err := os.WriteFile(prog, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	if got := resolveAgentPath("cats-todo-fake-agent"); got != prog {
		t.Fatalf("resolveAgentPath(name) = %q, want %q", got, prog)
	}
	if got := resolveAgentPath("/opt/agents/claude"); got != "/opt/agents/claude" {
		t.Fatalf("resolveAgentPath(path) = %q, want it unchanged", got)
	}
	if got := resolveAgentPath("no-such-agent-anywhere"); got != "no-such-agent-anywhere" {
		t.Fatalf("resolveAgentPath(unknown) = %q, want it unchanged", got)
	}
	if got := resolveAgentPath(""); got != "" {
		t.Fatalf("resolveAgentPath(\"\") = %q, want \"\"", got)
	}
}

// sendRecorder is a paneDriver that logs every call, in order, so a test can
// check the shape of a delivery: what was typed, when Enter went, and what was
// waited for in between.
type sendRecorder struct {
	calls []string
}

func (r *sendRecorder) sendInput(_ uint32, text string, submit bool) error {
	r.calls = append(r.calls, "send "+text+map[bool]string{true: " ⏎", false: ""}[submit])
	return nil
}

func (r *sendRecorder) waitForOutputIn(_ uint32, pattern string, _ bool, _ uint32, _ time.Duration) (bool, error) {
	r.calls = append(r.calls, "wait "+pattern)
	return true, nil
}

// TestDeliverPromptImages pins the image-drop fix. Claude Code drops an Enter
// that arrives while it is still attaching a pasted image path, so a run-mode
// drop with attachments types first, waits for the "[Image #" token, and sends
// the Enter on its own. Without attachments, and in paste mode, it stays one
// send.
func TestDeliverPromptImages(t *testing.T) {
	cases := []struct {
		name              string
		submit, hasImages bool
		want              []string
	}{
		{"run, no images", true, false, []string{"send P ⏎"}},
		{"paste, no images", false, false, []string{"send P"}},
		{"paste, images", false, true, []string{"send P"}},
		{"run, images", true, true, []string{"send P", "wait " + imageTokenPattern, "send  ⏎"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &sendRecorder{}
			if err := deliverPrompt(r, 1, "P", c.submit, c.hasImages); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(r.calls, c.want) {
				t.Errorf("calls = %q, want %q", r.calls, c.want)
			}
		})
	}
}
