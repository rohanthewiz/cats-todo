package main

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/rohanthewiz/cats-todo/internal/ctlproto"
	"github.com/rohanthewiz/cats/wire"
)

// TestWithPasteAsk pins what goes ahead of a dropped message (N-072). A
// request gets the ask as its own paragraph, and the prompt follows
// unchanged. A slash command stays bare, because Claude Code runs it as a
// command only when it comes first in the message: a lead would turn a
// loop's "/compact" or a "/sess-load 2" prompt into prose. Empty stays
// empty, since "" means nothing to send to every caller.
func TestWithPasteAsk(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"a request gets the ask", "fix the flaky test", pasteAsk + "\n\nfix the flaky test"},
		{"a slash command stays bare", "/sess-load 2", "/sess-load 2"},
		{"leading space before a slash is still a command", "  /compact", "  /compact"},
		{"empty is nothing to send", "", ""},
		{"a slash later in the text is not a command", "run /code-review", pasteAsk + "\n\nrun /code-review"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := withPasteAsk(c.in); got != c.want {
				t.Errorf("withPasteAsk(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
	// The ask is the user speaking, and is what the model reads first. It
	// must not end in a newline of its own, or the blank line between it and
	// the prompt would grow.
	if strings.HasSuffix(pasteAsk, "\n") || !strings.HasPrefix(pasteAsk, "Please ") {
		t.Errorf("pasteAsk = %q: want a plain one-paragraph ask", pasteAsk)
	}
}

// recordingSocket is a cats control socket that answers pane.send_input,
// pane.send_keys and agent.focus and remembers every message it was sent, in
// order: a paste as "paste <text>", and a run of key calls as one
// "typed <text>" (strokesText, so a newline reads "\n" and Enter "⏎"). With
// oldServer set it answers pane.send_keys the way a cats server older than
// the verb does.
func recordingSocket(t *testing.T, oldServer bool) (*catsClient, func() []string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "ctd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var mu sync.Mutex
	var sent []string
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				line, _ := bufio.NewReader(c).ReadBytes('\n')
				var req ctlproto.Request
				_ = json.Unmarshal(line, &req)
				resp := ctlproto.Response{ID: req.ID, OK: true}
				switch req.Method {
				case wire.CmdPaneSendInput:
					var p wire.SendInputParams
					_ = json.Unmarshal(req.Params, &p)
					mu.Lock()
					sent = append(sent, "paste "+p.Text)
					mu.Unlock()
				case wire.CmdPaneSendKeys:
					if oldServer {
						// cats' dispatcher's words for a command it lacks.
						resp = ctlproto.Response{ID: req.ID, Error: `command "pane.send_keys" not supported yet (WS2 in progress)`}
						break
					}
					var p wire.SendKeysParams
					_ = json.Unmarshal(req.Params, &p)
					mu.Lock()
					// Consecutive key calls are one message being typed.
					if n := len(sent); n > 0 && strings.HasPrefix(sent[n-1], "typed ") && !strings.HasSuffix(sent[n-1], "⏎") {
						sent[n-1] += strokesText(p.Keys)
					} else {
						sent = append(sent, "typed "+strokesText(p.Keys))
					}
					mu.Unlock()
				}
				b, _ := json.Marshal(resp)
				c.Write(append(b, '\n'))
			}(conn)
		}
	}()
	return &catsClient{socket: sock}, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), sent...)
	}
}

// TestDropIntoPaneLeadsWithTheAsk drives a real drop into a running pane on a
// cats server without pane.send_keys and reads what reached the wire. The
// prompt falls back to the paste, so the ask goes on it, and the /clear before
// it stays a bare command (N-072 (b), the interim that old servers still get).
func TestDropIntoPaneLeadsWithTheAsk(t *testing.T) {
	client, sent := recordingSocket(t, true)
	act := pendingAction{
		todo:   Todo{Title: "t", Prompt: "fix the flaky test", Session: &SessionOpts{Clear: true}},
		target: dropTarget{kind: targetExistingPane, pane: 7, agent: "claude"},
		mode:   dropRun,
	}
	if _, err := performDrop(client, act); err != nil {
		t.Fatal(err)
	}
	got := sent()
	want := []string{"paste /clear", "paste " + pasteAsk + "\n\nfix the flaky test"}
	if !slices.Equal(got, want) {
		t.Errorf("sent %q, want %q", got, want)
	}
}

// TestDropIntoPaneTypes is the same drop on a cats server with
// pane.send_keys: /clear is still pasted bare, and the prompt is typed — no
// ask, newlines as shift+Enter — then submitted with its own Enter (N-072 (a)).
func TestDropIntoPaneTypes(t *testing.T) {
	client, sent := recordingSocket(t, false)
	act := pendingAction{
		todo:   Todo{Title: "t", Prompt: "fix the flaky test\nand say why", Session: &SessionOpts{Clear: true}},
		target: dropTarget{kind: targetExistingPane, pane: 7, agent: "claude"},
		mode:   dropRun,
	}
	if _, err := performDrop(client, act); err != nil {
		t.Fatal(err)
	}
	got := sent()
	want := []string{"paste /clear", "typed fix the flaky test\nand say why⏎"}
	if !slices.Equal(got, want) {
		t.Errorf("sent %q, want %q", got, want)
	}
}
