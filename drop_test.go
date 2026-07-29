package main

import (
	"strings"
	"testing"
)

// TestComposePrompt pins the delivered text: paths get appended as their own
// lines, and the result never ends in a newline (sendInput's contract — a
// trailing newline is inserted literally instead of submitting).
func TestComposePrompt(t *testing.T) {
	t.Run("no images passes the prompt through", func(t *testing.T) {
		if got := composePrompt("just words", nil); got != "just words" {
			t.Errorf("composePrompt = %q, want the prompt unchanged", got)
		}
	})

	t.Run("images are appended one per line", func(t *testing.T) {
		got := composePrompt("fix this", []string{"/tmp/a.png", "/tmp/b.png"})
		want := "fix this\n\n" + imageBlockHeader + "\n/tmp/a.png\n/tmp/b.png"
		if got != want {
			t.Errorf("composePrompt =\n%q\nwant\n%q", got, want)
		}
	})

	t.Run("no trailing newline", func(t *testing.T) {
		got := composePrompt("p", []string{"/tmp/a.png"})
		if strings.HasSuffix(got, "\n") {
			t.Errorf("composePrompt = %q, must not end in a newline", got)
		}
	})

	t.Run("an empty prompt leaves no leading blank lines", func(t *testing.T) {
		got := composePrompt("", []string{"/tmp/a.png"})
		want := imageBlockHeader + "\n/tmp/a.png"
		if got != want {
			t.Errorf("composePrompt = %q, want %q", got, want)
		}
	})

	t.Run("paths are bare", func(t *testing.T) {
		// "@" starts a file-picker mention in Claude Code's input, which would
		// rewrite the paste mid-flight.
		if got := composePrompt("p", []string{"/tmp/a.png"}); strings.Contains(got, "@") {
			t.Errorf("composePrompt = %q, want no @ prefix on paths", got)
		}
	})
}
