// client.go — a small client for the cats control API (internal/ctlproto).
//
// Ported from herdr-todo's herdr.go (a JSON-RPC plugin-socket client), itself
// adapted from herdr-plus (https://github.com/cloudmanic/herdr-plus),
// Copyright (c) 2026 Cloudmanic Labs, LLC, MIT License. See NOTICE.

package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rohanthewiz/cats-todo/internal/ctlproto"
	"github.com/rohanthewiz/cats/wire"
)

// catsClient talks to the running cats server (cmd/catway) over its control
// unix socket, driving the same §7 command table as the browser front-end and
// catctl. Each call is one short-lived connection: one newline-framed JSON
// Request in, one Response out (ctlproto.Call owns the transport). The socket
// resolves like catctl's: CATS_CONTROL_SOCKET when set, else the default
// /tmp/cats-control.sock — so cats-todo works from any pane of a default-config
// catway with no wiring. The one non-path value is ctlproto.SocketNone ("-"),
// which cats exports into panes on a remote cathost it does not relay the
// control API to; there ResolveSocket yields "" and the ping below fails with a
// message naming the reason, rather than silently dialing whatever unrelated
// session happens to own /tmp/cats-control.sock on that machine.
type catsClient struct {
	socket string
}

// callTimeout bounds one ordinary control round trip. Only pane.wait_for_output
// legitimately outlives it, and waitForOutput sizes its own deadline.
const callTimeout = 10 * time.Second

// newCatsClient resolves the control socket and verifies a cats server is
// actually behind it with a ping, so the manager can degrade gracefully (todos
// still work, drops don't) when launched outside cats.
func newCatsClient() (*catsClient, error) {
	c := &catsClient{socket: ctlproto.ResolveSocket("")}
	var pong ctlproto.Pong
	if err := c.call(ctlproto.MethodPing, nil, &pong, callTimeout); err != nil {
		return nil, err
	}
	return c, nil
}

// call sends one request and decodes the response data into out (which may be
// nil when the caller does not care about the payload). A server-side failure
// (Response.OK false) surfaces as an error carrying the server's message.
func (c *catsClient) call(method string, params any, out any, timeout time.Duration) error {
	req := ctlproto.Request{ID: "cats-todo", Method: method}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("encode params: %w", err)
		}
		req.Params = raw
	}
	resp, err := ctlproto.Call(c.socket, req, timeout)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("cats error: %s", resp.Error)
	}
	if out != nil && len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, out); err != nil {
			return fmt.Errorf("decode result: %w", err)
		}
	}
	return nil
}

// paneList returns every pane cats currently knows about (across all workspaces
// and tabs), each carrying its runtime metadata — the Agent label is how we find
// Claude Code sessions to drop a prompt into.
func (c *catsClient) paneList() ([]wire.PaneInfo, error) {
	var out wire.PaneListResult
	if err := c.call(wire.CmdPaneList, nil, &out, callTimeout); err != nil {
		return nil, err
	}
	return out.Panes, nil
}

// workspaceList returns every workspace cats has open, in the server's order —
// the order the sidebar lists them in, which is what a picker that shows them
// should keep. Note what is not here: a workspace's directory. The workspace
// model has one (its identity cwd) but workspace.list does not put it on the
// wire; pane.list's per-pane cwd is how a caller finds where a workspace is
// working (see catsWorkspaceDirs in export.go).
func (c *catsClient) workspaceList() ([]wire.WorkspaceEntry, error) {
	var out wire.WorkspaceListResult
	if err := c.call(wire.CmdWorkspaceList, nil, &out, callTimeout); err != nil {
		return nil, err
	}
	return out.Workspaces, nil
}

// workspaceLabels returns a map of public workspace id → display name, for
// showing where a candidate pane lives (a pane's workspace id is the prefix of
// its "w1:p3" handle).
func (c *catsClient) workspaceLabels() (map[string]string, error) {
	wss, err := c.workspaceList()
	if err != nil {
		return nil, err
	}
	labels := make(map[string]string, len(wss))
	for _, ws := range wss {
		labels[ws.ID] = ws.Name
	}
	return labels, nil
}

// sessionInfo returns the one-shot session snapshot (active workspace, counts).
func (c *catsClient) sessionInfo() (wire.SessionInfoResult, error) {
	var out wire.SessionInfoResult
	err := c.call(wire.CmdSessionGet, nil, &out, callTimeout)
	return out, err
}

// sendInput types text into a pane as though at the keyboard. The server
// paste-encodes text against the pane's live modes, so a multi-line prompt
// lands intact in a TUI input (readline/Claude Code) instead of executing
// line-by-line; submit follows it with a real Enter keypress. To RUN a command,
// pass the text with submit=true — never a trailing newline in text, which the
// line editor would insert literally. submit with empty text sends just the
// Enter, firing previously staged input.
func (c *catsClient) sendInput(pane uint32, text string, submit bool) error {
	return c.call(wire.CmdPaneSendInput,
		wire.SendInputParams{Pane: pane, Text: text, Submit: submit}, nil, callTimeout)
}

// waitForOutput blocks until the pane's output matches pattern (a substring, or
// a regexp when regex is set) or timeout elapses, reporting whether it matched.
// The server matches the live output stream seeded with the current screen, so
// it never misses fast-scrolling output. The round-trip deadline is sized past
// the wait's own timeout (catctl does the same) so the transport never gives up
// before the server answers.
func (c *catsClient) waitForOutput(pane uint32, pattern string, regex bool, timeout time.Duration) (bool, error) {
	p := wire.WaitForOutputParams{
		Pane:      pane,
		Pattern:   pattern,
		Regex:     regex,
		TimeoutMs: uint32(timeout / time.Millisecond),
	}
	var out wire.WaitForOutputResult
	if err := c.call(wire.CmdWaitForOutput, p, &out, wire.WaitTimeout(p.TimeoutMs)+10*time.Second); err != nil {
		return false, err
	}
	return out.Matched, nil
}

// tabCreate opens a new tab in the active workspace, returning the new tab's
// public number and its root pane's id. The server focuses the new tab, so the
// returned pane is immediately drivable.
//
// cwd roots the new tab. Passing it matters because the workspace's identity
// cwd — what tab.create defaults to — is not necessarily the project the
// dropped todo belongs to; an agent launched in the wrong directory reads the
// wrong repo.
//
// title and command are tab.create's spawn override, and they are what make a
// new-session drop quick: command is an argv exec'd as the pane's process
// instead of a shell, so there is no shell to start, no prompt to wait for, no
// command to type and echo-match, and no follow-up tab.rename — one round trip
// where there used to be four, and the agent begins booting immediately. All
// three fields are optional; empty ones send no params at all, preserving the
// historical wire shape (and the server's defaults).
func (c *catsClient) tabCreate(cwd, title string, command []string) (num int, pane uint32, err error) {
	var params any // nil, not an empty struct: keeps the no-params request shape
	if cwd != "" || title != "" || len(command) > 0 {
		params = wire.TabCreateParams{Cwd: cwd, Title: title, Command: command}
	}
	var out wire.TabCreateResult
	if err = c.call(wire.CmdTabCreate, params, &out, callTimeout); err != nil {
		return 0, 0, err
	}
	return out.Num, out.Pane, nil
}

// worktreeCreateTimeout bounds worktree.create. Far past callTimeout because
// the server shells out to `git worktree add`, which materialises a whole
// checkout: seconds on a small repo, and a minute or more on a large one with a
// cold page cache. Timing out early would be the worst outcome available — the
// branch and the checkout would still be created, just with nothing launched in
// them and an error on screen saying otherwise.
const worktreeCreateTimeout = 3 * time.Minute

// worktreeCreate asks cats for a new `git worktree` checkout on branch, opened
// as a new workspace (created, focused, and named after the branch by the
// server) and returns the resolved branch and checkout path. Passing Path empty
// lets the server derive the checkout location under its configured worktree
// root (config: worktrees.directory), which is what the browser front-end's own
// new-worktree dialog does — the plugin has no business inventing a second
// convention for where a user's checkouts live.
//
// anchor is the pane whose working directory names the repo to branch from; 0
// means "the focused pane", the server's own default. Addressing our own pane
// explicitly matters for a scheduled drop, which fires with nobody watching and
// the focus wherever the user last left it.
func (c *catsClient) worktreeCreate(anchor uint32, branch string) (wire.WorktreeCreateResult, error) {
	p := wire.WorktreeCreateParams{Branch: branch}
	if anchor != 0 {
		p.Pane = &anchor
	}
	var out wire.WorktreeCreateResult
	err := c.call(wire.CmdWorktreeCreate, p, &out, worktreeCreateTimeout)
	return out, err
}

// focusPane reveals the pane into the viewport. agent.focus rather than
// pane.focus: the drop target may live in another workspace or tab, and
// agent.focus (like the agents sidebar it serves) crosses both, while
// pane.focus only moves focus within the pane's own tab.
func (c *catsClient) focusPane(pane uint32) error {
	return c.call(wire.CmdAgentFocus, wire.PaneParams{Pane: pane}, nil, callTimeout)
}

// claudeReadyProbes are substrings that signal Claude Code's input UI has drawn
// and is ready to receive a pasted prompt. We wait for any of them before
// pasting so keystrokes are not dropped into a half-started app. Matching is
// best effort — on timeout we paste anyway. These track Claude Code's
// footer/banner strings and may need refreshing as its UI evolves; a probe
// that no longer draws costs every drop the full wait timeout, so when drops
// go slow, capture a startup and re-check this list first.
//
// The spaces here are real matches even though the TUI draws word gaps as
// cursor-column jumps: catway's output stripper turns each movement sequence
// into a single separator (see cats cmd/catway/outscan.go), so "Welcome back"
// arrives spaced. On a catway older than that fix, spaced probes silently
// never match the stream and drops fall back to the timeout.
var claudeReadyProbes = []string{
	"Claude Code v",     // banner box title, every 2.x start, version-agnostic
	"Welcome back",      // returning-user greeting inside the banner
	"Welcome to Claude", // first-run greeting, and pre-2.x banners
	"for shortcuts",     // "? for shortcuts" footer (older layouts)
	"/help for help",    // pre-2.x footer
	"esc to interrupt",  // already mid-turn (e.g. launched with a prompt arg)
	"Bypassing Permissions",
}

// agentFirstDrawSettle is how long an unrecognized agent gets to finish drawing
// after its first byte of output. There is no banner we can probe for, so this
// is the one blind wait left in a drop — kept short because it starts from the
// agent's first draw rather than from launch.
const agentFirstDrawSettle = 600 * time.Millisecond

// waitForAgentReady blocks until a freshly launched agent looks ready to accept
// a pasted prompt. Claude Code has known footer/banner strings to probe for —
// folded into one alternation regex so a single server-side waiter watches for
// all of them at once. Any other agent has no banner we know, so we wait for its
// first output (the pane is exec'd straight into the agent, so any non-blank
// byte is the agent itself, not a shell prompt) and give it a short settle.
// Best effort throughout — on timeout we paste anyway.
func (c *catsClient) waitForAgentReady(pane uint32, command string) {
	if command == "claude" {
		quoted := make([]string, len(claudeReadyProbes))
		for i, p := range claudeReadyProbes {
			quoted[i] = regexp.QuoteMeta(p)
		}
		_, _ = c.waitForOutput(pane, strings.Join(quoted, "|"), true, 12*time.Second)
		return
	}
	if matched, err := c.waitForOutput(pane, `\S`, true, 5*time.Second); err != nil || !matched {
		return // timed out or the call failed: paste anyway, no extra sleep on top
	}
	time.Sleep(agentFirstDrawSettle)
}
