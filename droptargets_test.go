package main

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/rohanthewiz/cats-todo/internal/ctlproto"
	"github.com/rohanthewiz/cats/wire"
)

// fakeCatsSocket serves pane.list and workspace.list from fixed answers on a
// unix socket, so the picker's running-pane block can be built without a real
// cats. Every other method is answered with an error. The socket lives under
// os.TempDir rather than t.TempDir: a unix socket path is capped near 104
// bytes on darwin, and a test's temp dir alone can come close to that.
func fakeCatsSocket(t *testing.T, panes []wire.PaneInfo, wss []wire.WorkspaceEntry) *catsClient {
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
				resp := ctlproto.Response{ID: req.ID}
				var data any
				switch req.Method {
				case wire.CmdPaneList:
					data = wire.PaneListResult{Panes: panes}
				case wire.CmdWorkspaceList:
					data = wire.WorkspaceListResult{Workspaces: wss}
				default:
					resp.Error = "unsupported in the fake"
				}
				if data != nil {
					resp.OK = true
					resp.Data, _ = json.Marshal(data)
				}
				b, _ := json.Marshal(resp)
				c.Write(append(b, '\n'))
			}(conn)
		}
	}()
	return &catsClient{socket: sock}
}

// TestBuildTargetsFoldsOtherProjects pins the fold: running agents in another
// workspace sit behind one "more drop targets" row, last in the list, and
// choosing that row rebuilds the picker with them in it and the highlight on
// the first one revealed — without dropping anything.
func TestBuildTargetsFoldsOtherProjects(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // claude alone among the new-session rows
	m, _, _ := newModelInTemp(t)
	m.ctx.WorkspaceID = "w1"
	m.client = fakeCatsSocket(t,
		[]wire.PaneInfo{
			{Pane: 1, Handle: "w1:p1", PaneMeta: wire.PaneMeta{Agent: "claude", Cwd: "/here"}},
			{Pane: 2, Handle: "w2:p2", PaneMeta: wire.PaneMeta{Agent: "claude", Cwd: "/there"}},
			{Pane: 3, Handle: "w3:p3", PaneMeta: wire.PaneMeta{Agent: "claude", Cwd: "/yonder"}},
		},
		[]wire.WorkspaceEntry{{ID: "w1", Name: "here"}, {ID: "w2", Name: "there"}, {ID: "w3", Name: "yonder"}},
	)

	m.targets, m.targetList = m.buildTargets()
	var panes []uint32
	for _, sc := range m.targetList.filtered {
		if tg := m.targets[sc.item.ref]; tg.kind == targetExistingPane {
			panes = append(panes, tg.pane)
		}
	}
	if len(panes) != 1 || panes[0] != 1 {
		t.Fatalf("folded picker lists panes %v, want just this project's pane 1", panes)
	}
	last := m.targets[len(m.targets)-1]
	if last.kind != targetMore || !strings.Contains(last.label, "2 running agents") {
		t.Fatalf("last row = %+v, want the fold naming 2 agents", last)
	}

	// Choose the fold: the picker stays up, unfolded.
	m.stage = stageTarget
	m.targetList.selectRef(len(m.targets) - 1)
	next, _ := m.chooseTarget(dropRun)
	m = next.(model)
	if m.stage != stageTarget || m.dropping {
		t.Fatalf("after the fold: stage=%v dropping=%v, want the picker still up and nothing sent", m.stage, m.dropping)
	}
	panes = panes[:0]
	for _, tg := range m.targets {
		if tg.kind == targetMore {
			t.Errorf("the unfolded picker still has a fold row: %+v", tg)
		}
		if tg.kind == targetExistingPane {
			panes = append(panes, tg.pane)
		}
	}
	if len(panes) != 3 {
		t.Errorf("unfolded picker offers panes %v, want all 3", panes)
	}
	if idx := m.targetList.selectedIndex(); idx < 0 || m.targets[idx].pane != 2 {
		t.Errorf("highlight on %d, want the first revealed pane (2)", idx)
	}

	// No workspace ID for this launch: nothing can be called "elsewhere", so
	// nothing is folded.
	m.ctx.WorkspaceID = ""
	targets, _ := m.buildTargets()
	for _, tg := range targets {
		if tg.kind == targetMore {
			t.Error("a launch with no workspace ID folded panes it cannot place")
		}
	}
}

// TestTargetFilterSearchesFoldedAgents pins N-061: the filter searches the
// folded agents too. Before, it matched only the listed rows, so
// typing another project's name found nothing until the More row was chosen.
// A query now lists the matching folded pane and hides the More row (the
// search already covers what it would reveal); clearing the query folds the
// pane away again and brings the More row back.
func TestTargetFilterSearchesFoldedAgents(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m, _, _ := newModelInTemp(t)
	m.ctx.WorkspaceID = "w1"
	m.client = fakeCatsSocket(t,
		[]wire.PaneInfo{
			{Pane: 1, Handle: "w1:p1", PaneMeta: wire.PaneMeta{Agent: "claude", Cwd: "/here"}},
			{Pane: 2, Handle: "w2:p2", PaneMeta: wire.PaneMeta{Agent: "claude", Cwd: "/there"}},
			{Pane: 3, Handle: "w3:p3", PaneMeta: wire.PaneMeta{Agent: "claude", Cwd: "/yonder"}},
		},
		[]wire.WorkspaceEntry{{ID: "w1", Name: "here"}, {ID: "w2", Name: "there"}, {ID: "w3", Name: "yonder"}},
	)
	m.targets, m.targetList = m.buildTargets()
	m.stage = stageTarget

	listed := func() (panes []uint32, more bool) {
		for _, sc := range m.targetList.filtered {
			switch tg := m.targets[sc.item.ref]; tg.kind {
			case targetExistingPane:
				panes = append(panes, tg.pane)
			case targetMore:
				more = true
			}
		}
		return panes, more
	}

	for _, r := range "yonder" {
		next, _ := m.updateTarget(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(model)
	}
	panes, more := listed()
	if len(panes) != 1 || panes[0] != 3 {
		t.Fatalf("query \"yonder\" lists panes %v, want the folded pane 3", panes)
	}
	if more {
		t.Error("the More row is listed while searching")
	}
	if matched, total := m.targetList.counts(); matched != 1 || total != 4 {
		t.Errorf("counts = %d/%d, want 1/4 — every row but the More row was searched", matched, total)
	}

	// enter drops into the matched folded pane like any other row.
	if idx := m.targetList.selectedIndex(); idx < 0 || m.targets[idx].pane != 3 {
		t.Fatalf("highlight on %d, want the matched pane 3", idx)
	}

	for range "yonder" {
		next, _ := m.updateTarget(tea.KeyPressMsg{Code: tea.KeyBackspace})
		m = next.(model)
	}
	panes, more = listed()
	if len(panes) != 1 || panes[0] != 1 || !more {
		t.Errorf("cleared query lists panes %v, More %v; want pane 1 and the More row", panes, more)
	}
	if matched, total := m.targetList.counts(); matched != total {
		t.Errorf("counts = %d/%d at rest, want all listed rows counted and no more", matched, total)
	}
}

// TestRunningPaneRowShowsContextFill pins the running-pane row carrying the
// agent's model string — model, effort and context fill ("43k/1M"), as cats
// resolves it for its AGENTS hover card — between the state and the cwd, so
// how full a session is can be read before the pick. A pane cats resolved no
// model for keeps the old "[state] cwd" shape.
func TestRunningPaneRowShowsContextFill(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m, _, _ := newModelInTemp(t)
	m.ctx.WorkspaceID = "w1"
	m.client = fakeCatsSocket(t,
		[]wire.PaneInfo{
			{Pane: 1, Handle: "w1:p1", PaneMeta: wire.PaneMeta{Agent: "claude", AgentState: "idle",
				AgentModel: "claude-opus-5 · high · 43k/1M", Cwd: "/here"}},
			{Pane: 2, Handle: "w1:p2", PaneMeta: wire.PaneMeta{Agent: "codex", AgentState: "working", Cwd: "/here"}},
			{Pane: 3, Handle: "w1:p3", PaneMeta: wire.PaneMeta{Agent: "claude", AgentState: "idle",
				Title: "✳ Fix the flaky drop test", Cwd: "/here"}},
		},
		[]wire.WorkspaceEntry{{ID: "w1", Name: "here"}},
	)
	targets, _ := m.buildTargets()
	got := map[uint32]string{}
	for _, tg := range targets {
		if tg.kind == targetExistingPane {
			got[tg.pane] = tg.desc
		}
	}
	if want := "[idle] claude-opus-5 · high · 43k/1M · /here"; got[1] != want {
		t.Errorf("claude row desc = %q, want %q", got[1], want)
	}
	if !strings.HasPrefix(got[2], "[working] /here") {
		t.Errorf("model-less row desc = %q, want it to start %q", got[2], "[working] /here")
	}
	// A titled session says what it is about, right after its state, so two
	// claude panes in one project are told apart (N-075).
	if want := "[idle] “Fix the flaky drop test” · /here"; got[3] != want {
		t.Errorf("titled row desc = %q, want %q", got[3], want)
	}
}
