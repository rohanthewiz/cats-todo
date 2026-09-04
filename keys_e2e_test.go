package main

import (
	"bytes"
	"os"
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
	// · "line two" · enter (newline) · "line three" · shift+enter (save, as the
	// kitty protocol's CSI 13;2u — the chord the ✔ Save chip advertises, and one
	// that only exists on the wire at all under that protocol) · enter (reopen it
	// for editing) · ctrl+c.
	//
	// Plain enter is in the middle of the prompt on purpose: in the form it is
	// a newline like the chord beside it, and only the wire proves the CR the
	// terminal sends reaches the textarea rather than the save.
	//
	// Keep esc out of the tail: the parser reads a buffer at a time, so an ESC
	// byte sitting in front of ctrl+c is read as one chord (ctrl+alt+c) and the
	// quit never happens.
	keys := "\r" + "line one" + "\x1b\r" + "line two" + "\r" + "line three" + "\x1b[13;2u" + "\r" + "\x03"

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
	if got := project.todos[0].Prompt; got != "line one\nline two\nline three" {
		t.Errorf("prompt = %q, want all three lines joined by the alt+enter and enter newlines "+
			"(a lost line means that key saved instead of inserting; a missing todo means shift+enter did not save)", got)
	}
	if out.Len() == 0 {
		t.Error("the program rendered nothing")
	}
}

// TestAttachKeysEndToEnd drives the attachment editor with real terminal bytes,
// for the reason the test above exists: the binding is the feature, and only the
// wire proves it. It is here because the obvious mnemonic does not survive the
// wire at all — ctrl+i and tab are both 0x09, so on a terminal without the kitty
// protocol ctrl+i arrives as the tab that switches form fields, and the editor
// never opens. ctrl+o (0x0f) is bound because it cannot collide.
//
// Keys: enter (the list is empty, so the add form) · the prompt · ctrl+o (open
// the attachment editor) · ctrl+r (offer the newest image in CATS_TODO_IMAGE_DIR)
// · enter (attach it) · enter (empty box — back to the form) · shift+enter
// (save) · ctrl+c.
func TestAttachKeysEndToEnd(t *testing.T) {
	dir := t.TempDir()
	shot := filepath.Join(dir, "shot.png")
	if err := os.WriteFile(shot, make([]byte, 16), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(imageSourceDirEnvVar, dir)

	project := &store{scope: scopeProject, path: filepath.Join(dir, "project", "todos.json")}
	global := &store{scope: scopeGlobal, path: filepath.Join(dir, "global", "todos.json")}
	m := newModel(RunContext{WorkDir: filepath.Join(dir, "project")}, project, global, nil)

	keys := "\r" + "this layout is wrong" + "\x0f" + "\x12" + "\r" + "\r" + "\x1b[13;2u" + "\x03"

	var out bytes.Buffer
	p := tea.NewProgram(m, tea.WithInput(strings.NewReader(keys)), tea.WithOutput(&out))
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
	td := project.todos[0]
	if len(td.Images) != 1 {
		t.Fatalf("Images = %v, want the one ctrl+o/ctrl+r/enter attached "+
			"(empty means the editor never opened, or ctrl+r offered nothing)", td.Images)
	}
	if refs := project.resolveImages(td); refs[0].missing {
		t.Errorf("attachment %s was recorded but never copied in", refs[0].rel)
	}
}
