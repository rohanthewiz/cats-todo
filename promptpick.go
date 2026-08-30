// promptpick.go — the picker that puts a library prompt into the editor.
//
// The library itself is promptlib.go; this is the screen that shows it. It is a
// full-screen sub-stage of the form, the same shape as the '@' file picker and
// for the same reasons: the form has no floating list, and a library that grows
// past a handful of entries wants the whole pane. Everything about the browsing
// is the fuzzy list every other picker here uses, so there is nothing new to
// learn — type to narrow, arrows to walk, enter to take it.
//
//	╭ Insert a prompt ─────────────────────────────────────────────╮
//	│ 🔍 repro                                              1/6     │
//	│                                                              │
//	│ ❯ repro steps          how to file a bug                     │
//	╰──────────────────────────────────────────────────────────────╯
//
// Two ways in, and the difference is only which entries are listed:
//
//   - ctrl+p (cmd+P) from the editor lists the whole library.
//   - '/' typed at the start of a line lists the COMMANDS alone — a slash there
//     is someone already writing one, so the picker answers the question that
//     was actually asked, and the slash they typed is consumed by the entry
//     they pick rather than doubled (see snippetInsertion).
//
// The picker is also where entries are made. Saving from here rather than from
// a form of its own is what keeps the feature one screen: the query box is
// already a focused text field, so the name of the thing being saved is typed
// where the name of the thing being looked for is typed, and ctrl+s writes the
// editor's swept text — or the whole prompt, when nothing is swept — under it.
// A library that can only be grown by opening another editor is a library that
// stays empty.

package main

import (
	"fmt"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// snippetPurpose is which slice of the library the picker was opened over. The
// browsing is identical either way — that is the point of one picker — and only
// the rows, the heading and the one insertion rule differ (see eatSlash).
type snippetPurpose int

const (
	// snippetsAll lists the whole library: the chord's flavour.
	snippetsAll snippetPurpose = iota
	// snippetsCommands lists only the slash commands, and is opened by a '/'
	// already typed into the prompt, which the chosen entry replaces.
	snippetsCommands
)

// snippetPicker is the open picker: the library it was built over, the rows it
// is showing, and the text a save would write.
//
// The library is a snapshot taken at open time rather than a live read, so the
// rows cannot change under a cursor that is walking them; the next open takes a
// fresh one (see loadPromptLib on why that is cheap and why it matters).
type snippetPicker struct {
	purpose snippetPurpose
	lib     promptLibrary
	// shown are the entries on the list in ref order — a ref is an index here,
	// exactly as the file picker's is into its entries.
	shown []promptSnippet
	// capture is what ctrl+s would save: the editor's selection, else the whole
	// prompt, else "" when there is nothing to save. captureWhat names it in the
	// footer, so the offer says which of the two it is before it is taken.
	capture     string
	captureWhat string
	// note is the last thing that happened here — a save, or why one did not —
	// and err marks it as the second kind. It rides the picker rather than the
	// form's note because it reports on this screen and must not outlive it.
	note string
	err  bool
	list fuzzyList
}

// newSnippetPicker builds the picker over lib, listing what purpose asks for.
func newSnippetPicker(lib promptLibrary, purpose snippetPurpose, capture, captureWhat string) snippetPicker {
	placeholder := "type to filter · enter inserts"
	if capture != "" {
		placeholder = "type to filter, or a name to save under"
	}
	p := snippetPicker{
		purpose:     purpose,
		lib:         lib,
		capture:     capture,
		captureWhat: captureWhat,
		list:        newFuzzyList(placeholder, nil),
	}
	// Names that start with the query lead, the file picker's rule: the query
	// here is the beginning of a name people already know far more often than it
	// is a scatter of letters from the middle of one.
	p.list.prefixFirst = true
	p.refresh()
	return p
}

// refresh rebuilds the rows from the library. It runs after every save as well
// as at open, so an entry written here is on the list before the keys come back.
func (p *snippetPicker) refresh() {
	p.shown = p.shown[:0]
	items := make([]listItem, 0, len(p.lib.snippets))
	for _, s := range p.lib.snippets {
		if p.purpose == snippetsCommands && !s.isCommand() {
			continue
		}
		it := listItem{
			name: s.label(),
			desc: s.summary(),
			// The haystack is the name, the description AND the body, so a query
			// finds an entry by what it inserts as well as by what it is called
			// — "sess" has to find "/sess-load" whether or not its name says the
			// word, and a paragraph is easier to remember a phrase from than a
			// name for. Flattened, since the matcher works on one line.
			search:     collapseLines(s.Name + " " + s.Desc + " " + s.Body),
			selectable: true,
			ref:        len(p.shown),
		}
		// A command rides beside the name rather than being the name: what the
		// entry is called is how it was found, what it runs is what is about to
		// land in the prompt, and both are worth a glance before enter. The
		// whole command line, arguments included, since "/sess-load 2" and
		// "/sess-load" are different entries and the number is the difference.
		//
		// It then takes the description's job as well, unless the entry has
		// words of its own: summary falls back to the body, and a row reading
		// "wrap up · /sess-wrap  /sess-wrap" says one thing twice.
		if c := s.commandLine(); c != "" {
			it.tag = truncate(c, 48)
			it.desc = strings.TrimSpace(s.Desc)
		}
		// A tag that only repeats the name it sits beside (an unnamed command,
		// whose label is already its body) is dropped rather than doubled.
		if it.tag == it.name {
			it.tag = ""
		}
		items = append(items, it)
		p.shown = append(p.shown, s)
	}
	p.list.setItems(items)
}

// highlighted is the entry under the cursor.
func (p snippetPicker) highlighted() (promptSnippet, bool) {
	i := p.list.selectedIndex()
	if i < 0 || i >= len(p.shown) {
		return promptSnippet{}, false
	}
	return p.shown[i], true
}

// resize fits the picker to the pane, the same arithmetic filePicker.resize
// uses: the query box to the width, and the list's window to what is left once
// the chrome above the rows and the blank-plus-footer below it are spent.
func (p *snippetPicker) resize(width, height int) {
	if w := width - 4; w >= 20 {
		p.list.input.SetWidth(w)
	}
	rows := 0
	if height > 0 {
		rows = max(height-snippetsRowsRow-3, 1)
	}
	p.list.setMaxRows(rows)
}

// snippetsRowsRow is the first line the picker's rows are drawn on: the heading
// (0), a blank (1), the boxed query line (2), and the blank fuzzyList.view puts
// under it (3). The same geometry as filesRowsRow, and pinned by a test for the
// same reason — clickSnippets subtracts it from the pointer's row.
const snippetsRowsRow = 4

// emptyMessage is what the rows block says when nothing is listed. Each case is
// a different problem with a different answer, so each gets its own sentence
// rather than one "nothing here": the file could not be read, the library is
// empty, it holds no commands, or the query simply matched nothing.
func (p snippetPicker) emptyMessage() string {
	if p.lib.err != "" {
		return p.lib.err
	}
	if len(p.shown) == 0 {
		if p.purpose == snippetsCommands {
			return "no commands in the library yet — an entry whose text starts with / is one"
		}
		if p.capture != "" {
			return "the library is empty — type a name above and press ctrl+s to save " + p.captureWhat
		}
		return "the library is empty — add entries to " + shortenHome(p.lib.path)
	}
	return "nothing matched"
}

func (p snippetPicker) String() string {
	return fmt.Sprintf("snippetPicker{purpose:%d shown:%d query:%q}", p.purpose, len(p.shown), p.list.input.Value())
}

// --- The stage -----------------------------------------------------------------

// beginSnippets opens the picker over the form. Both fields blur so their
// carets stop blinking under a screen they are not on.
//
// The library is handed in rather than read here, because one of the two
// callers has already read it to decide whether to open at all (the '/' trigger
// asks hasCommands first) and a second read would be a second answer to a
// question already settled.
//
// The capture is decided at open time, from the editor as it stands: a swept run
// if there is one, otherwise the whole prompt. Deciding it here rather than at
// save time is what lets the footer name it — the offer is made before it is
// taken, and it cannot change while the picker is up.
func (m model) beginSnippets(lib promptLibrary, purpose snippetPurpose) (tea.Model, tea.Cmd) {
	capture, what := m.selectedPromptText(), "the selection"
	if capture == "" {
		capture, what = strings.TrimSpace(m.promptArea.Value()), "this prompt"
	}
	if capture == "" {
		what = ""
	}
	// The highlight ends here, now that its text has been taken. The form's
	// standing rule is that a selection lasts exactly as long as the run of keys
	// building it, and this is one of the keys that ends the run — but the rule
	// is applied here rather than by the clearPromptSel in updateForm, which
	// this chord is answered above precisely so that the capture can be read.
	// Leaving it would put the editor back under a highlight whose anchor an
	// insertion had since moved.
	m.clearPromptSel()
	m.formNote = ""
	m.snips = newSnippetPicker(lib, purpose, capture, what)
	m.snips.resize(m.width, m.height)
	m.titleInput.Blur()
	m.promptArea.Blur()
	m.stage = stageSnippets
	return m, textinput.Blink
}

// closeSnippets hands the keys back to the form, to the field that had them —
// always the prompt, since both ways in are from the editor, but restored the
// same way every other sub-stage restores them so none of them can be the one
// that quietly differs.
func (m model) closeSnippets() (tea.Model, tea.Cmd) {
	m.stage = stageForm
	return m, m.restoreFormFocus()
}

// chooseSnippet inserts the highlighted entry and closes. Nothing highlighted —
// an empty library, a query that matched nothing — is a no-op rather than a
// close, so the query can be fixed where it went wrong.
//
// eatSlash is true exactly for the commands flavour, which is the one opened by
// a '/' the user typed: that slash is part of the command about to be inserted,
// so it is consumed rather than left in front of it.
func (m model) chooseSnippet() (tea.Model, tea.Cmd) {
	s, ok := m.snips.highlighted()
	if !ok {
		return m, nil
	}
	note := m.insertSnippet(s, m.snips.purpose == snippetsCommands)
	m.formNote = note
	return m.closeSnippets()
}

// saveSnippet writes what the editor had swept (or its whole prompt) into the
// library under the name in the query box.
//
// Every refusal says what is missing rather than doing nothing — the empty
// query, the empty editor, the name already taken and the failed write are four
// different problems, and the picker stays open on all of them so the fix is one
// keystroke away. On success the query clears: it was the name, and leaving it
// in place would filter the list down to the row just written.
func (m model) saveSnippet() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.snips.list.input.Value())
	switch {
	case m.snips.capture == "":
		m.snips.note, m.snips.err = "nothing to save — there is no text in the prompt yet", true
		return m, nil
	case name == "":
		m.snips.note, m.snips.err = "type a name in the box first, then ctrl+s", true
		return m, nil
	}
	if err := m.snips.lib.add(promptSnippet{Name: name, Body: m.snips.capture}); err != nil {
		m.snips.note, m.snips.err = err.Error(), true
		return m, nil
	}
	m.snips.list.input.SetValue("")
	m.snips.refresh()
	// Park on the row just written, which is the last one: a save is worth
	// seeing land, and the entry is very often the one about to be inserted
	// somewhere else.
	m.snips.list.selectRef(len(m.snips.shown) - 1)
	m.snips.note, m.snips.err = "saved “"+name+"” to "+shortenHome(m.snips.lib.path), false
	return m, nil
}

// updateSnippets is the picker's key loop, modeled on the file picker's: the
// list's keys (arrows, enter, esc), the one key that writes (ctrl+s), and
// everything else, which is the query.
//
// ctrl+s is the form's save chord one stage up, and it means something else
// here. That is deliberate rather than an oversight: the two are the same idea —
// commit what is in front of me — applied to what this screen is about, the
// footer names it whenever it can act, and the form's own save is not reachable
// from here anyway, so there is no version of ctrl+s that gets the wrong one.
func (m model) updateSnippets(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		return m.closeSnippets()
	case "up", "ctrl+p":
		m.snips.list.moveUp()
		return m, nil
	case "down", "ctrl+n":
		m.snips.list.moveDown()
		return m, nil
	case "pgup", "pgdown":
		// A page is the window's height; with no window (no size known yet) one
		// row is as far as a page can be trusted to go — filePicker's rule.
		step := max(m.snips.list.maxRows, 1)
		for range step {
			if msg.String() == "pgup" {
				m.snips.list.moveUp()
			} else {
				m.snips.list.moveDown()
			}
		}
		return m, nil
	case "enter":
		return m.chooseSnippet()
	case "ctrl+s", "super+s", "meta+s":
		return m.saveSnippet()
	}
	// A keystroke that edits the query has answered the last note: it was about
	// the state the box was in before the key, and a stale "type a name first"
	// sitting over a box with a name in it is worse than no message at all.
	m.snips.note, m.snips.err = "", false
	return m, m.snips.list.editQuery(msg)
}

// clickSnippets is the pointer on the picker: a click on a row chooses it, the
// same as the file picker's. snippetsRowsRow is the offset from the pane's top
// to the first row line (see its comment).
func (m model) clickSnippets(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	i, ok := m.snips.list.rowAtLine(msg.Y - snippetsRowsRow)
	if !ok || !m.snips.list.focusRow(i) {
		return m, nil
	}
	return m.chooseSnippet()
}

// viewSnippets draws the picker: a heading, the list with its boxed query line,
// and a footer of keys. The heading is one line by construction — the note or
// the path beside it is truncated to what is left of the width — because every
// row constant below it depends on that.
func (m model) viewSnippets() string {
	var b strings.Builder
	title := "Insert a prompt"
	if m.snips.purpose == snippetsCommands {
		title = "Insert a command"
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("  ")
	// The last thing that happened, else where the library lives. The path is
	// the standing answer to "where do I put these?", so it is what the heading
	// says whenever it has nothing more urgent to report.
	side, style, isPath := shortenHome(m.snips.lib.path), descStyle, true
	if m.snips.note != "" {
		side, style, isPath = m.snips.note, okStyle, false
		if m.snips.err {
			style = errStyle
		}
	}
	if m.width > 0 {
		// The title, two spaces, and a little slack for the title style's
		// padding — filePicker's arithmetic, for the same one-line guarantee.
		if room := m.width - lipgloss.Width(titleStyle.Render(title)) - 4; room > 1 {
			// A path is cut from the FRONT (truncateLeft, filepick.go): the end
			// of a path is the part that says which file. A sentence is cut from
			// the back, where a sentence keeps its meaning.
			if isPath {
				side = truncateLeft(side, room)
			} else {
				side = truncate(side, room)
			}
		}
	}
	b.WriteString(style.Render(side))
	b.WriteString("\n\n")
	b.WriteString(m.snips.list.view(m.snips.emptyMessage(), "", m.width))
	b.WriteString("\n")

	// In the order they must survive a narrowing pane: what the screen is for,
	// then the way to grow it, then the way out. The save segment names what it
	// would write, because "save" alone would leave the user to guess between
	// the selection and the whole prompt — and it is left off entirely when
	// there is nothing to save, since an offer that cannot be taken is a segment
	// spent teaching nothing.
	segs := []string{"enter insert"}
	if m.snips.capture != "" {
		segs = append(segs, "ctrl+s saves "+m.snips.captureWhat+" under the typed name")
	}
	segs = append(segs, "esc back")
	b.WriteString(footerStyle.Render(m.fitFooter(segs)))
	return b.String()
}

// --- The two ways in ------------------------------------------------------------

// snippetLibChord reports whether a chord asks for the library.
//
// cmd+P is the palette chord on every other editor on this machine, and cats
// forwards it to the pane (only a curated set of Cmd chords arrives at all), so
// it is the one to reach for inside cats; ctrl+p is the same key for a terminal
// that cannot send Cmd at all. Binding it costs the textarea's emacs-style
// ctrl+p — one of two spellings of "line up", the other being ↑, and one this
// program has never advertised — which is a cheap price for the chord whose
// letter matches the word.
func snippetLibChord(s string) bool {
	switch s {
	case "ctrl+p", "super+p", "meta+p":
		return true
	}
	return false
}

// promptAtLineStart reports whether the caret sits where a line's first word
// would go: at the very start of the text, or with nothing but blanks between it
// and the newline above.
//
// It is the guard on the '/' trigger, and it is deliberately stricter than
// promptAtWordStart is on '@'. A slash is ordinary punctuation in the middle of
// a sentence — and/or, src/ui, 3/4 — where an '@' at a word start is nearly
// always a mention; the one place a slash means a command is the place a command
// can be written, which is the start of a line. Indentation still counts as the
// start, since a command inside an indented block is still on its own line.
func promptAtLineStart(ta textarea.Model) bool {
	runes := []rune(ta.Value())
	off := min(max(promptCaretOffset(ta), 0), len(runes))
	for i := off - 1; i >= 0; i-- {
		if runes[i] == '\n' {
			return true
		}
		if !unicode.IsSpace(runes[i]) {
			return false
		}
	}
	return true
}
