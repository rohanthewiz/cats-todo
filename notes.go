// notes.go — sending an info prompt to a notes plugin.
//
// An ｉ-marked prompt is a note that landed in the backlog because that is where
// the hand was (see Todo.Info). Its Send — shift+enter, the menu's row, the
// form's ✉ Send — does not open the agent picker; it files the note in the
// notes plugin running in cats, and the prompt is marked done, exactly as a
// drop marks it.
//
//	cats-todo                        cats                      notes pane
//	─────────                        ────                      ──────────
//	pane.list ─────────────────────▶ every pane + plugin_type
//	pick plugin_type == "notes_mgr"
//	pane.send_input(envelope, ─────▶ paste-encoded (bracketed) ──▶ tea.PasteMsg
//	                submit=false)                                  sentinel? → a new
//	pane.focus ────────────────────▶                               UNSAVED note form
//	mark done (dropResultMsg)                                      ctrl+s is the user's
//
// WHY THE PANE AND NOT A FILE, A CLI OR AN API. cats has no plugin-to-plugin
// message; a pane's input is the one door every TUI already has. It is also the
// right door: the process behind it already owns the notes store (the bytdb
// lock in local mode, the login token in HTTP mode), so this side needs neither
// to file a note. The cost is that a notes pane has to be OPEN — with none, the
// send is refused in words that say so.
//
// WHY BY TYPE AND NOT BY ID. The pane is found by its plugin_type,
// "notes_mgr", never by "rohanthewiz.gonotes". cats sets the type from the
// manifest's declared kind when a plugin action launches the pane, and from
// its tools.types map (gonotes → notes_mgr by default) when the program was
// typed into a shell and only reports its label. Any notes plugin that speaks the
// envelope below is a destination; nothing here names GoNotes except the
// wording of a refusal.
//
// THE ENVELOPE (v1) is the receiver's contract — gonotes tui/intake.go is its
// reference implementation, and the long comment there is the spec:
//
//	<!-- cats-note v1 -->
//	---
//	title: "…"
//	description: "from cats-todo · <project>"
//	tags: ["cats-todo"]
//	---
//
//	<the prompt, as markdown>
//
// An older notes plugin that does not know the sentinel receives it as an
// ordinary paste, and as an HTML comment the marker is invisible in rendered
// markdown — the one failure this side cannot rule out is also the mildest.

package main

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/rohanthewiz/cats/wire"
)

// notesEnvelopeSentinel is the envelope's first line. The version is the one
// this side writes; a receiver refuses a version newer than it knows rather
// than guessing at it.
const notesEnvelopeSentinel = "<!-- cats-note v1 -->"

// notesTag is the tag every sent note carries, so what arrived from the backlog
// can be found again in the notes program as one set.
const notesTag = "cats-todo"

// notesTitleWidth bounds a title derived from the prompt's first line (a
// prompt with no title of its own). The receiver's title field is 200
// characters; a note title is a label, and a sentence cut at 80 still reads
// as one.
const notesTitleWidth = 80

// errNoNotesPane is the refusal when no notes plugin is open. It names both
// ways out, because either can be the one wanted: open a notes pane and send
// again, or decide this is work after all and send it to an agent.
var errNoNotesPane = errors.New(infoSendWhy)

// notesEnvelope renders td as a v1 note envelope. project names where the note
// came from, for the description line; "" leaves the line off.
//
// The frontmatter values are written as JSON strings. YAML is a superset of
// JSON for scalars and flow sequences, so this is valid YAML with no YAML
// dependency — and, more to the point, quoting every value means nothing a
// prompt's title can contain (a colon, a leading "-", a "#", a lone "---")
// can be read as structure. Only the body is left raw, and the body is after
// the closing "---", where the receiver reads nothing as frontmatter.
//
// Attachments go in as a list of paths. The body is the note; an agent's
// "read these files" instruction (composePrompt) would be words to a reader
// who is not there, and the paths are what keep the images findable.
func notesEnvelope(td Todo, project string, images []string) string {
	var b strings.Builder
	b.WriteString(notesEnvelopeSentinel + "\n---\n")
	b.WriteString("title: " + jsonString(notesTitle(td)) + "\n")
	if project != "" {
		b.WriteString("description: " + jsonString("from cats-todo · "+project) + "\n")
	}
	b.WriteString("tags: [" + jsonString(notesTag) + "]\n")
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimRight(td.Prompt, "\n"))
	if len(images) > 0 {
		b.WriteString("\n\nAttached images:\n")
		for _, p := range images {
			b.WriteString("- " + p + "\n")
		}
	}
	return b.String()
}

// notesTitle is the note's title: the prompt's own, or failing that its first
// non-blank line, trimmed to a label's length. A notes program requires a
// title, and a prompt added from the CLI often has none.
func notesTitle(td Todo) string {
	if t := strings.TrimSpace(td.Title); t != "" {
		return t
	}
	for _, line := range strings.Split(td.Prompt, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return truncate(line, notesTitleWidth)
		}
	}
	return "Note from cats-todo"
}

// jsonString quotes s as a JSON string, which is also a YAML double-quoted
// scalar. HTML escaping is off so a "<" in a title reaches the note as "<"
// rather than as a < the user would see in the form.
func jsonString(s string) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // a string always encodes
	return strings.TrimSuffix(b.String(), "\n")
}

// pickNotesPane chooses the notes pane to send to, from a pane.list answer.
//
// Only panes cats typed "notes_mgr" qualify. Among several, the ranking is the
// one a user would apply by eye:
//
//  1. in our own workspace — the notes pane beside this backlog is the one the
//     user is looking at, and the one the focus switch will not carry them
//     away to another workspace for;
//  2. reporting an agent label — GoNotes registers over the hook API, so a
//     pane with a label is one whose program is running, rather than a pane
//     cats still lists for a moment after it exited;
//  3. visible, then the lowest pane id, so the choice is stable between sends.
func pickNotesPane(panes []wire.PaneInfo, ctx RunContext) (wire.PaneInfo, bool) {
	var cands []wire.PaneInfo
	for _, p := range panes {
		if p.PluginType == wire.PluginTypeNotesMgr && !isOwnPane(ctx, p) {
			cands = append(cands, p)
		}
	}
	if len(cands) == 0 {
		return wire.PaneInfo{}, false
	}
	rank := func(p wire.PaneInfo) int {
		r := 0
		if ctx.WorkspaceID == "" || paneWorkspaceID(p) != ctx.WorkspaceID {
			r += 4
		}
		if p.Agent == "" {
			r += 2
		}
		if !p.Visible {
			r++
		}
		return r
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if ri, rj := rank(cands[i]), rank(cands[j]); ri != rj {
			return ri < rj
		}
		return cands[i].Pane < cands[j].Pane
	})
	return cands[0], true
}

// notesPaneDesc names a notes pane for the status line: its custom name, else
// the label it reports (GoNotes says "gonotes"), else just "notes", with the
// handle that finds it in the sidebar.
func notesPaneDesc(p wire.PaneInfo) string {
	name := firstNonEmpty(p.Name, p.Agent, "notes")
	if p.Handle != "" {
		return name + " (" + p.Handle + ")"
	}
	return name
}

// sendToNotes delivers envelope to the best notes pane and focuses it,
// returning that pane's description. It runs off the event loop, inside a
// tea.Cmd, since it is two or three socket round trips.
//
// Submit is false: the text is staged, not "entered". The receiver opens it as
// an unsaved form, and an Enter keypress following the paste would land in
// that form's first field.
func sendToNotes(client *catsClient, ctx RunContext, envelope string) (desc string, err error) {
	if client == nil {
		return "", errors.New("cats control socket unavailable")
	}
	panes, err := client.paneList()
	if err != nil {
		return "", err
	}
	p, ok := pickNotesPane(panes, ctx)
	if !ok {
		return "", errNoNotesPane
	}
	if err := client.sendInput(p.Pane, envelope, false); err != nil {
		return "", err
	}
	// The note is waiting in an unsaved form over there; the save is the
	// user's, so take them to it. Best effort, as a pane drop's is: the note is
	// already delivered, and a focus failure must not report the send failed.
	_ = client.focusPane(p.Pane)
	return notesPaneDesc(p), nil
}

// startNotesSend is startDrop's road for an info prompt: no picker, since there
// is only ever one kind of destination and pickNotesPane chooses among them.
// The result comes back as an ordinary dropResultMsg, so a delivered note is
// marked done by the same code that marks a dropped prompt done — and a done
// row is the undo, one ctrl+t away, if the note is then discarded unsaved.
//
// The caller has already applied startDrop's guards (a drop in flight, no
// socket, frozen); nothing a drop refuses is something a notes send should
// allow.
func (m model) startNotesSend(ref todoRef) (tea.Model, tea.Cmd) {
	td, ok := m.resolve(ref)
	if !ok {
		m.setStatus("could not find that prompt", true)
		m.backToList()
		return m, nil
	}
	project := baseName(m.ctx.projectDir())
	if ref.scope == scopeGlobal {
		project = "global backlog"
	}
	envelope := notesEnvelope(td, project, m.storeFor(ref.scope).imagePaths(td))

	m.dropping = true
	m.listFocus = ref
	m.backToList()
	m.setStatus("sending to notes…", false)
	client, ctx := m.client, m.ctx
	return m, func() tea.Msg {
		desc, err := sendToNotes(client, ctx, envelope)
		return dropResultMsg{desc: desc, ref: ref, err: err, toNotes: true}
	}
}
