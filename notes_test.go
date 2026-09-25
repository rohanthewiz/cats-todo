package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/rohanthewiz/cats/wire"
)

// ---- the envelope ----------------------------------------------------------

// The envelope is a contract with another repo (gonotes tui/intake.go), so its
// exact shape is pinned: sentinel first, JSON-quoted frontmatter, then the body
// raw after a blank line.
func TestNotesEnvelopeShape(t *testing.T) {
	td := Todo{Title: "pane.list quirk", Prompt: "Handles are w1:p3.\n\nNot numeric.\n"}
	got := notesEnvelope(td, "cats", []string{"/tmp/a.png"})
	want := `<!-- cats-note v1 -->
---
title: "pane.list quirk"
description: "from cats-todo · cats"
tags: ["cats-todo"]
---

Handles are w1:p3.

Not numeric.

Attached images:
- /tmp/a.png
`
	if got != want {
		t.Errorf("notesEnvelope =\n%s\nwant\n%s", got, want)
	}
}

// Every frontmatter value is quoted, so nothing a title can hold is read as
// YAML structure — and "<" stays "<" rather than becoming < in the form.
func TestNotesEnvelopeQuotesHostileTitles(t *testing.T) {
	for _, title := range []string{`a: b`, `- item`, `# hash`, `---`, `say "hi" <now>`} {
		got := notesEnvelope(Todo{Title: title, Prompt: "p"}, "", nil)
		line := strings.Split(got, "\n")[2]
		if want := "title: " + jsonString(title); line != want {
			t.Errorf("title %q: line = %q, want %q", title, line, want)
		}
		if strings.Contains(line, "\\u003c") {
			t.Errorf("title %q was HTML-escaped: %q", title, line)
		}
		if strings.Contains(got, "description:") {
			t.Errorf("an empty project must leave the description off:\n%s", got)
		}
	}
}

// A prompt without a title still needs one — a notes program requires it —
// so the first non-blank line stands in, trimmed to a label.
func TestNotesTitleFallsBackToTheFirstLine(t *testing.T) {
	long := strings.Repeat("x", 200)
	for _, tc := range []struct {
		td   Todo
		want string
	}{
		{Todo{Title: "  own  ", Prompt: "body"}, "own"},
		{Todo{Prompt: "\n\n  first line  \nsecond"}, "first line"},
		{Todo{Prompt: "   \n"}, "Note from cats-todo"},
	} {
		if got := notesTitle(tc.td); got != tc.want {
			t.Errorf("notesTitle(%+v) = %q, want %q", tc.td, got, tc.want)
		}
	}
	if got := notesTitle(Todo{Prompt: long}); len([]rune(got)) > notesTitleWidth {
		t.Errorf("a long first line became a %d-rune title", len([]rune(got)))
	}
}

// ---- choosing the pane -----------------------------------------------------

func TestPickNotesPaneFindsByTypeNotByName(t *testing.T) {
	panes := []wire.PaneInfo{
		{Pane: 1, Handle: "w1:p1", PaneMeta: wire.PaneMeta{Agent: "claude"}},
		// Labelled like GoNotes but not typed as a notes manager — an older
		// cats, or a label opted out of tools.types. Not a destination: the
		// type is the contract, and cats is what decides it.
		{Pane: 2, Handle: "w1:p2", PaneMeta: wire.PaneMeta{Agent: "gonotes"}},
		{Pane: 3, Handle: "w1:p3", PaneMeta: wire.PaneMeta{Agent: "othernotes", PluginType: wire.PluginTypeNotesMgr}},
	}
	p, ok := pickNotesPane(panes, RunContext{WorkspaceID: "w1"})
	if !ok || p.Pane != 3 {
		t.Errorf("pickNotesPane = %d, %v; want pane 3, the one typed notes_mgr", p.Pane, ok)
	}
	if _, ok := pickNotesPane(panes[:2], RunContext{WorkspaceID: "w1"}); ok {
		t.Error("pickNotesPane found a notes pane where none is typed notes_mgr")
	}
}

func TestPickNotesPaneRanksOwnWorkspaceThenLiveThenVisible(t *testing.T) {
	notes := func(id uint32, handle, agent string, visible bool) wire.PaneInfo {
		return wire.PaneInfo{Pane: id, Handle: handle, Visible: visible,
			PaneMeta: wire.PaneMeta{Agent: agent, PluginType: wire.PluginTypeNotesMgr}}
	}
	ctx := RunContext{WorkspaceID: "w2"}
	for _, tc := range []struct {
		name  string
		panes []wire.PaneInfo
		want  uint32
	}{
		{"own workspace beats a live, visible pane elsewhere",
			[]wire.PaneInfo{notes(1, "w1:p1", "gonotes", true), notes(9, "w2:p9", "", false)}, 9},
		{"a pane reporting its label beats one that does not",
			[]wire.PaneInfo{notes(4, "w2:p4", "", true), notes(5, "w2:p5", "gonotes", false)}, 5},
		{"visible breaks the tie",
			[]wire.PaneInfo{notes(6, "w2:p6", "gonotes", false), notes(7, "w2:p7", "gonotes", true)}, 7},
		{"then the lowest id, so repeat sends agree",
			[]wire.PaneInfo{notes(8, "w2:p8", "gonotes", true), notes(3, "w2:p3", "gonotes", true)}, 3},
	} {
		if p, _ := pickNotesPane(tc.panes, ctx); p.Pane != tc.want {
			t.Errorf("%s: picked %d, want %d", tc.name, p.Pane, tc.want)
		}
	}
}

// ---- the send --------------------------------------------------------------

// TestInfoSendGoesToNotes: shift+enter on an info prompt does not open the
// agent picker — it starts a send to notes, in flight like a drop, from the
// list. The empty client has no socket, so the command's answer is a failure,
// and that failure must say it was the notes send and leave the prompt open.
func TestInfoSendGoesToNotes(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	if err := project.add(Todo{ID: "a", Prompt: "remember the api quirk", Info: true}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()
	m.client = &catsClient{}

	next, cmd := m.updateList(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	m = next.(model)
	if m.stage != stageList {
		t.Fatalf("stage = %v; an info prompt must not open the agent picker", m.stage)
	}
	if !m.dropping || cmd == nil {
		t.Fatalf("dropping = %v, cmd nil = %v; want a send in flight", m.dropping, cmd == nil)
	}

	res, ok := cmd().(dropResultMsg)
	if !ok || !res.toNotes || res.err == nil {
		t.Fatalf("cmd() = %#v; want a failed notes send", res)
	}
	next, _ = m.Update(res)
	m = next.(model)
	if !strings.HasPrefix(m.status, "send to notes failed: ") || !m.statusErr {
		t.Errorf("status = %q (err %v); want the notes send named as what failed", m.status, m.statusErr)
	}
	if td, _ := project.find("a"); td.Done {
		t.Error("a failed send marked the prompt done")
	}
}

// A delivered note closes the prompt the way a drop does, and the status line
// says where it went and what is left to do.
func TestNotesSendSuccessMarksDone(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	if err := project.add(Todo{ID: "a", Prompt: "p", Info: true}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()
	m.dropping = true

	next, _ := m.Update(dropResultMsg{desc: "gonotes (w1:p5)", ref: todoRef{scope: scopeProject, id: "a"}, toNotes: true})
	m = next.(model)
	if td, _ := project.find("a"); !td.Done {
		t.Error("a delivered note was not marked done")
	}
	if want := "sent to notes → gonotes (w1:p5) · save it there · marked done"; m.status != want {
		t.Errorf("status = %q, want %q", m.status, want)
	}
}

// With no notes pane, the refusal names both ways out.
func TestNoNotesPaneRefusalNamesTheWaysOut(t *testing.T) {
	msg := errNoNotesPane.Error()
	if !strings.Contains(msg, "notes") || !strings.Contains(msg, "ℹ Info") {
		t.Errorf("refusal = %q; want it to name a notes plugin and the mark", msg)
	}
}
