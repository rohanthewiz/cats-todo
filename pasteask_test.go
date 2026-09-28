package main

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
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

// recordingSocket is a cats control socket that answers pane.send_input and
// agent.focus and remembers every text it was sent, in order.
func recordingSocket(t *testing.T) (*catsClient, func() []string) {
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
				if req.Method == wire.CmdPaneSendInput {
					var p wire.SendInputParams
					_ = json.Unmarshal(req.Params, &p)
					mu.Lock()
					sent = append(sent, p.Text)
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

// TestDropIntoPaneLeadsWithTheAsk drives a real drop into a running pane and
// reads what reached the wire. The ask goes on the message, and the /clear
// before it stays a bare command (N-072).
func TestDropIntoPaneLeadsWithTheAsk(t *testing.T) {
	client, sent := recordingSocket(t)
	act := pendingAction{
		todo:   Todo{Title: "t", Prompt: "fix the flaky test", Session: &SessionOpts{Clear: true}},
		target: dropTarget{kind: targetExistingPane, pane: 7, agent: "claude"},
		mode:   dropRun,
	}
	if _, err := performDrop(client, act); err != nil {
		t.Fatal(err)
	}
	got := sent()
	want := []string{"/clear", pasteAsk + "\n\nfix the flaky test"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("sent %q, want %q", got, want)
	}
}
