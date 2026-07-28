package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TestEnterKeysEndToEnd drives the whole program with the raw bytes a terminal
// actually sends, rather than the KeyPressMsg values the other tests
// construct. That is the point of it: the enter bindings live or die on how
// bubbletea's parser reads the wire, and only a real run proves that a legacy
// terminal's alt+enter — ESC followed by CR, with no kitty protocol in sight —
// arrives as "alt+enter" and inserts a newline instead of saving the form. It
// also covers the whole stage machine booting under bubbletea v2: the alt
// screen, the sized inputs, and a clean exit.
func TestEnterKeysEndToEnd(t *testing.T) {
	dir := t.TempDir()
	project := &store{scope: scopeProject, path: filepath.Join(dir, "project", "todos.json")}
	global := &store{scope: scopeGlobal, path: filepath.Join(dir, "global", "todos.json")}
	m := newModel(RunContext{WorkDir: filepath.Join(dir, "project")}, project, global, nil)

	// enter (add form, the list being empty) · "line one" · alt+enter (newline)
	// · "line two" · enter (save) · enter (reopen it for editing) · ctrl+c.
	//
	// Keep esc out of the tail: the parser reads a buffer at a time, so an ESC
	// byte sitting in front of ctrl+c is read as one chord (ctrl+alt+c) and the
	// quit never happens.
	keys := "\r" + "line one" + "\x1b\r" + "line two" + "\r" + "\r" + "\x03"

	var out bytes.Buffer
	p := tea.NewProgram(m,
		tea.WithInput(strings.NewReader(keys)),
		tea.WithOutput(&out),
	)

	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("program run: %v", err)
		}
	case <-time.After(5 * time.Second):
		p.Kill()
		t.Fatal("program did not exit on ctrl+c")
	}

	if len(project.todos) != 1 {
		t.Fatalf("project todos = %+v, want the single prompt the keystrokes entered", project.todos)
	}
	if got := project.todos[0].Prompt; got != "line one\nline two" {
		t.Errorf("prompt = %q, want both lines joined by the alt+enter newline "+
			"(one line means alt+enter saved instead of inserting; a missing todo means enter did not save)", got)
	}
	if out.Len() == 0 {
		t.Error("the program rendered nothing")
	}
}
