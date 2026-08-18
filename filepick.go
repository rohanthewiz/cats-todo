// filepick.go — the file picker behind '@' in the prompt editor.
//
// Typing '@' at the start of a word in the prompt opens this picker; choosing an
// entry drops "@path " into the prompt at the caret, so a prompt written here
// reads as a file mention when it is dropped into an agent (Claude Code's own
// input treats "@path" that way). The picker is a full-screen sub-stage of the
// form, the same shape as the attachment editor and the session panel — the
// form has no floating overlays, and a list that can grow to a directory's
// worth of rows wants the whole pane anyway.
//
// It browses one directory at a time. That is the design of cdx (../cdx), whose
// path completion this borrows: expandPath, shortenHome, the drill-in on tab,
// the keep-the-highlight-visible scroll. cdx is a `cd` picker and lists only
// directories; here files are listed too — dirs first, then files — and the
// rest of cdx (frecency, the directory stack, quit-on-select, exec) stays where
// it is. One os.ReadDir per keystroke, no recursion, no ignore rules: a level at
// a time is cheap enough to never need a cache, and it is what makes the picker
// a browser rather than a search — the way in to a deep file is the same as at
// a shell prompt, a segment and a slash at a time.
//
// The picker's query box holds only the partial typed within the current
// directory; the directory itself is separate state, shown in the heading. A
// query that turns into a path — a slash typed, "~/", "../", or a whole path
// pasted — is normalized on the spot: the directory part moves into dir and the
// list re-reads there, the last segment stays in the box as the filter. That
// one rule is what makes typing "src/ui" and pasting "~/projs/x/main.go" both do
// the obvious thing.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// fileEntry is one row of the current directory listing.
type fileEntry struct {
	name string
	dir  bool
}

// filePicker is the browsing state: where the walk started, where it is now,
// what is there, and the list that shows it. entries keeps every entry the
// directory holds, hidden ones included — which of them are on the list is the
// query's call (see refresh), so hidden entries can appear and disappear as the
// leading dot is typed and deleted without another read of the disk.
type filePicker struct {
	// root is where browsing starts and what inserted paths are made relative
	// to: the project directory when there is one (see beginFiles).
	root string
	// dir is the directory whose entries are listed, absolute.
	dir string
	// entries are the rows in the list's ref order — a ref is an index here.
	entries []fileEntry
	// err is why dir could not be read, or "" when it could. It rides the
	// picker rather than the model's status line because it belongs to the
	// listing it describes: a new dir clears it, and nothing else does.
	err string
	// list draws the rows and owns the query box. Row descriptions are left
	// empty on purpose: fuzzyList matches the query against name+desc, and a
	// description of "dir" would make "d" match every folder.
	list fuzzyList
}

// newFilePicker builds a picker listing root, with an empty query.
func newFilePicker(root string) filePicker {
	p := filePicker{root: root, list: newFuzzyList("type to filter · / opens a folder", nil)}
	// Names that start with the query lead (see fuzzyList.prefixFirst): the
	// filter is a path segment being typed, and "int/" has to mean internal/.
	p.list.prefixFirst = true
	p.setDir(root)
	return p
}

// setDir moves the picker to dir: re-reads the entries, clears the query, and
// rebuilds the list. The query is cleared because it was a filter within the
// directory just left, and a filter carried into a new directory would hide
// what is there for no reason anyone typed.
func (p *filePicker) setDir(dir string) {
	p.dir = dir
	p.entries, p.err = nil, ""
	entries, err := readDirEntries(dir)
	if err != nil {
		p.err = "cannot read " + shortenHome(dir) + ": " + err.Error()
	}
	p.entries = entries
	p.list.input.SetValue("")
	p.refresh()
}

// query is what the box holds, untrimmed — a leading dot has to count.
func (p filePicker) query() string {
	return p.list.input.Value()
}

// refresh rebuilds the list's rows from entries against the current query. It
// runs after every edit because which rows exist depends on the query: hidden
// entries are listed only while it starts with a dot (cdx's rule — nobody wants
// .git and .DS_Store in the way, and anyone who wants a dotfile types the dot).
func (p *filePicker) refresh() {
	p.list.setItems(fileListItems(p.entries, strings.HasPrefix(p.query(), ".")))
}

// edit feeds a message to the query box — a keystroke, a paste, the cursor
// blink — and then does the two things every edit must: normalize a query that
// has become a path, and rebuild the rows. It is the one path through which the
// query changes, so those two never get skipped.
func (p *filePicker) edit(msg tea.Msg) tea.Cmd {
	cmd := p.list.editQuery(msg)
	p.normalize()
	p.refresh()
	return cmd
}

// normalize turns a query that names a directory into a move there. A query
// holding a slash, or spelling ~ or .., is split into the directory it points at
// and the last segment; when that directory exists the picker moves into it and
// keeps the segment as the filter. "src/" lands in src with an empty filter,
// "src/ui" lands in src filtering on "ui", "../" goes up, "~/" goes home, and a
// pasted absolute path lands in its directory filtering on its file name. A
// path into a directory that isn't there is left alone — the list shows nothing
// matched, and backspace repairs it.
func (p *filePicker) normalize() {
	q := p.query()
	if !isPathQuery(q) {
		return
	}
	base, partial := splitPathQuery(q, p.dir)
	if !isDir(base) {
		return
	}
	p.setDir(base)
	p.list.input.SetValue(partial)
	p.list.input.CursorEnd()
}

// descend opens the highlighted directory, or — on a highlighted file —
// completes the query to its name, the way a shell's tab does. Reports whether
// anything happened. Descending goes through setDir, so the query clears: the
// filter that found the folder has done its job.
func (p *filePicker) descend() bool {
	e, abs, ok := p.highlighted()
	if !ok {
		return false
	}
	if e.dir {
		p.setDir(abs)
		return true
	}
	p.list.input.SetValue(e.name)
	p.list.input.CursorEnd()
	p.refresh()
	return true
}

// up moves to the parent directory and parks the highlight on the folder just
// left, so backspace-backspace-backspace walks up a tree with the way back down
// always under the cursor. At the filesystem root there is no parent, and the
// call is a no-op rather than a jump to nowhere.
func (p *filePicker) up() {
	parent := filepath.Dir(p.dir)
	if parent == p.dir {
		return
	}
	child := filepath.Base(p.dir)
	p.setDir(parent)
	for i, e := range p.entries {
		if e.name == child {
			// The child may be hidden; a fresh query never shows those, so
			// the ref may not be on the list — selectRef leaves the cursor
			// alone in that case, which is the right answer.
			p.list.selectRef(i)
			return
		}
	}
}

// highlighted is the entry under the cursor and its absolute path.
func (p filePicker) highlighted() (fileEntry, string, bool) {
	i := p.list.selectedIndex()
	if i < 0 || i >= len(p.entries) {
		return fileEntry{}, "", false
	}
	e := p.entries[i]
	return e, filepath.Join(p.dir, e.name), true
}

// resize fits the picker to the pane: the query box to the width, and the
// list's scroll window to what is left of the height once the picker's chrome
// — the rows above filesRowsRow, and the blank line, footer and "… more" marker
// below — is spent. A height that isn't known yet (before the first
// WindowSizeMsg, and in tests that never send one) leaves the list unwindowed.
func (p *filePicker) resize(width, height int) {
	if w := width - 4; w >= 20 {
		p.list.input.SetWidth(w)
	}
	rows := 0
	if height > 0 {
		rows = max(height-filesRowsRow-3, 1)
	}
	p.list.setMaxRows(rows)
}

// --- Pure helpers ---------------------------------------------------------------

// readDirEntries lists dir: directories first, then files, each in name order,
// hidden entries included (the caller decides whether to show them). A symlink
// counts as a directory when it points at one, which os.Stat — not the entry's
// own type — answers; a broken link lists as a file, which is honest enough for
// a picker whose job is to insert the name.
//
// This is cdx's readSubdirs with the "skip everything that isn't a directory"
// rule removed: cdx picks somewhere to cd to, this picks something to mention.
func readDirEntries(dir string) ([]fileEntry, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var dirs, files []fileEntry
	for _, e := range ents {
		isDirEntry := e.IsDir()
		if !isDirEntry && e.Type()&os.ModeSymlink != 0 {
			isDirEntry = isDir(filepath.Join(dir, e.Name()))
		}
		if isDirEntry {
			dirs = append(dirs, fileEntry{name: e.Name(), dir: true})
		} else {
			files = append(files, fileEntry{name: e.Name()})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	return append(dirs, files...), nil
}

// fileListItems turns entries into list rows: the name (with a trailing slash
// on a directory, as cdx draws them), selectable, ref = index into entries.
// Dot-prefixed entries are dropped unless showHidden.
func fileListItems(entries []fileEntry, showHidden bool) []listItem {
	items := make([]listItem, 0, len(entries))
	for i, e := range entries {
		if !showHidden && strings.HasPrefix(e.name, ".") {
			continue
		}
		name := e.name
		if e.dir {
			name += "/"
		}
		items = append(items, listItem{name: name, selectable: true, ref: i})
	}
	return items
}

// isPathQuery reports whether the query has stopped being a filter within the
// current directory and started being a path — which is to say, whether it
// holds a slash. cdx also counts a bare "~" or a leading "." as a path; here
// they wait for their slash, and the reason is the slash key itself: "~" and
// ".." are meant to be followed by one, and if the bare spelling had already
// moved the picker (home, or up) the slash that follows would land on an empty
// query and open whatever folder was highlighted there. So "~" filters (on
// nothing) until "~/" goes home, and ".." until "../" goes up; the slash key
// knows both spellings and lets it through (see updateFiles).
func isPathQuery(q string) bool {
	return strings.Contains(q, "/")
}

// startsAPath reports whether q, with a slash typed after it, would be a path
// rather than a request to open the highlighted folder: the two spellings that
// name a directory without being one of this directory's entries, or the name
// of a folder that is.
func (p filePicker) startsAPath(q string) bool {
	return q == "~" || q == ".." || (q != "" && isDir(filepath.Join(p.dir, q)))
}

// splitPathQuery resolves q against cwd and splits it into the directory to
// list and the segment to filter on. A query that ends in a slash, or that is
// exactly ~ or .., names a directory outright and filters on nothing; anything
// else filters on its last segment inside the directory before it. This is the
// split cdx's pathComplete makes.
func splitPathQuery(q, cwd string) (base, partial string) {
	exp := expandPath(q, cwd)
	if strings.HasSuffix(exp, "/") || q == "~" || q == ".." || strings.HasSuffix(q, "/..") {
		return filepath.Clean(exp), ""
	}
	base, partial = filepath.Split(exp)
	if base == "" {
		base = cwd
	}
	return filepath.Clean(base), partial
}

// expandPath resolves ~ and relative paths against cwd into an absolute path. A
// trailing separator is preserved, because for a completer it carries meaning:
// "src/" lists inside src, "src" completes src itself. Ported from cdx.
func expandPath(q, cwd string) string {
	trailing := strings.HasSuffix(q, "/")
	switch {
	case q == "~":
		q = homeDir()
	case strings.HasPrefix(q, "~/"):
		q = filepath.Join(homeDir(), q[2:])
	case !filepath.IsAbs(q):
		q = filepath.Join(cwd, q)
	default:
		q = filepath.Clean(q)
	}
	if trailing && !strings.HasSuffix(q, "/") {
		q += "/"
	}
	return q
}

// homeDir is the user's home, or "/" when even that cannot be found — a place
// that always exists beats an empty string that joins into nonsense.
func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "/"
	}
	return h
}

// shortenHome writes a path under the home directory as ~/…, for display and
// for the mention text of a file outside the project. Ported from cdx.
func shortenHome(p string) string {
	h := homeDir()
	if p == h {
		return "~"
	}
	if strings.HasPrefix(p, h+"/") {
		return "~" + p[len(h):]
	}
	return p
}

// isDir reports whether p is a directory (following symlinks).
func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// fileInsertText is what goes after the '@' for a chosen entry: the path
// relative to the project directory when the entry is inside it, else the
// absolute path with the home directory shortened to ~. Directories carry a
// trailing slash, so the mention says what it is. Relative is the form Claude
// Code's own picker inserts, and the one that survives the project moving.
//
// Paths with spaces are inserted as they are — a mention with a space in it is
// the agent's problem to read, and quoting would be a guess at whose rules
// apply. The caller adds the '@' and the space that ends the mention.
func fileInsertText(abs string, dir bool, projectDir string) string {
	text := ""
	if projectDir != "" && (abs == projectDir || strings.HasPrefix(abs, projectDir+"/")) {
		if rel, err := filepath.Rel(projectDir, abs); err == nil {
			text = rel
		}
	}
	if text == "" {
		text = shortenHome(abs)
	}
	if dir && !strings.HasSuffix(text, "/") {
		text += "/"
	}
	return text
}

// promptAtWordStart reports whether the caret sits where a word could begin:
// at the very start of the text, or right after whitespace (a newline counts,
// so every line start qualifies). It is the guard on the '@' trigger — an '@'
// typed inside a word, as in an e-mail address, is text and stays text.
func promptAtWordStart(ta textarea.Model) bool {
	off := promptCaretOffset(ta)
	if off <= 0 {
		return true
	}
	runes := []rune(ta.Value())
	if off > len(runes) {
		off = len(runes)
	}
	return unicode.IsSpace(runes[off-1])
}

// truncateLeft shortens s to at most max runes by cutting from the front,
// leading with "…" when it had to. It is truncate's mirror, for a directory
// path in a heading: the end of a path is the part that says where you are.
func truncateLeft(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[len(r)-max:])
	}
	return "…" + string(r[len(r)-(max-1):])
}

// filesRowsRow is the first line the picker's rows are drawn on: the heading
// (0), a blank (1), the boxed query line — one line, the field's box has only
// left and right rails (2) — and the blank fuzzyList.view puts under it (3).
// Same geometry as the drop-target picker's targetRowsRow, for the same reason:
// clickFiles subtracts it from the pointer's row to find the list row, so
// TestFilesRowsMatchWhatIsDrawn pins it to a real frame.
const filesRowsRow = 4

// filesEmptyMessage is what the rows block says when nothing is listed: the
// read error when there is one, otherwise that the filter matched nothing.
func (p filePicker) filesEmptyMessage() string {
	if p.err != "" {
		return p.err
	}
	if len(p.entries) == 0 {
		return "empty folder"
	}
	return "nothing here matches"
}

// headingDir is the directory as the heading shows it: home shortened, a
// trailing slash, and cut from the front to fit width columns (a heading that
// wrapped would push every row down a line and break filesRowsRow). A width of
// zero — no size known — leaves it whole.
func (p filePicker) headingDir(width int) string {
	dir := shortenHome(p.dir)
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	if width > 0 {
		dir = truncateLeft(dir, max(width, 4))
	}
	return dir
}

// String helps tests and logs say where the picker is.
func (p filePicker) String() string {
	return fmt.Sprintf("filePicker{dir:%q query:%q rows:%d}", p.dir, p.query(), len(p.list.filtered))
}

// --- The stage -----------------------------------------------------------------

// beginFiles opens the picker over the form. It mirrors beginImages: both
// fields blur so their carets stop blinking under a screen they are not on,
// and the picker is built fresh — rooted at the project directory when there
// is one, else where the manager was launched, else home — so nothing from a
// previous open (a directory three levels down, a half-typed filter) greets the
// next '@'.
func (m model) beginFiles() (tea.Model, tea.Cmd) {
	root := firstNonEmpty(m.ctx.projectDir(), m.ctx.WorkDir, homeDir())
	m.files = newFilePicker(root)
	m.files.resize(m.width, m.height)
	m.titleInput.Blur()
	m.promptArea.Blur()
	m.stage = stageFiles
	return m, textinput.Blink
}

// closeFiles returns to the form, restoring focus to the field that had it. The
// trigger only fires from the prompt field, so that is always the prompt — but
// the check is kept, so the picker and the attachment editor close the same
// way and neither can be the one that quietly breaks when that changes.
func (m model) closeFiles() (tea.Model, tea.Cmd) {
	m.stage = stageForm
	if m.formFocus == formFieldTitle {
		return m, m.titleInput.Focus()
	}
	return m, m.promptArea.Focus()
}

// chooseFile inserts the highlighted entry into the prompt and closes the
// picker. The '@' is already in the text (see the trigger in updateForm), so
// what goes in is the path and the space that ends the mention — through the
// textarea's own InsertString, at the caret, the same call the toolbar's ↵ chip
// makes. Nothing highlighted (an empty folder, a filter that matched nothing)
// is a no-op rather than a close: the picker stays, so the filter can be fixed.
func (m model) chooseFile() (tea.Model, tea.Cmd) {
	e, abs, ok := m.files.highlighted()
	if !ok {
		return m, nil
	}
	text := fileInsertText(abs, e.dir, m.ctx.projectDir())
	m.promptArea.InsertString(text + " ")
	m.formNote = "inserted @" + text
	return m.closeFiles()
}

// updateFiles is the picker's key loop, modeled on the drop-target picker's.
//
// The keys divide into the list's (arrows, enter, esc), the browser's (tab and
// → open a folder, backspace on an empty box goes up, a typed / opens the
// highlighted folder), and everything else, which is the filter. The browser
// keys are the ones cdx has, minus its stack and clipboard chords; the one
// addition is → as a spelling of tab, because tab is also what the form uses to
// switch fields and a hand that has just come from there may not reach for it.
//
// ctrl+i is bound beside tab because on a terminal without the kitty keyboard
// protocol the two are the same byte (see the ctrl+o/ctrl+i note in
// updateForm); binding both means the picker cannot tell them apart and does
// not need to.
func (m model) updateFiles(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		return m.closeFiles()
	case "up", "ctrl+p":
		m.files.list.moveUp()
		return m, nil
	case "down", "ctrl+n":
		m.files.list.moveDown()
		return m, nil
	case "pgup", "pgdown":
		// A page is the window's height; with no window (no size known yet)
		// one row is as far as a page can be trusted to go.
		step := max(m.files.list.maxRows, 1)
		for range step {
			if msg.String() == "pgup" {
				m.files.list.moveUp()
			} else {
				m.files.list.moveDown()
			}
		}
		return m, nil
	case "enter":
		return m.chooseFile()
	case "tab", "ctrl+i", "right":
		m.files.descend()
		return m, nil
	case "backspace":
		if m.files.query() == "" {
			m.files.up()
			return m, nil
		}
	case "/":
		// A slash after a filter that is itself a folder's name — or after "~"
		// or ".." — is a path being typed, and the query's own normalization
		// walks into it (so "src/" works whether or not src is highlighted, and
		// "~/" and "../" mean what they mean). Any other slash means "open what
		// is highlighted", the way tab does — the way fish and zsh complete a
		// segment — and a slash with nothing openable under the cursor is
		// swallowed, because a lone "/" in the filter box would match nothing
		// and mean nothing. One consequence, worth knowing: an absolute path
		// cannot be started by typing "/" first; "~/", "../", or backspacing up
		// to the root are the ways there.
		if m.files.startsAPath(m.files.query()) {
			break
		}
		if e, _, ok := m.files.highlighted(); ok && e.dir {
			m.files.descend()
		}
		return m, nil
	}
	return m, m.files.edit(msg)
}

// clickFiles is the pointer on the picker: a click on a row chooses it, the
// same as the drop-target picker's clickTarget. filesRowsRow is the offset from
// the pane's top to the first row line (see its comment).
func (m model) clickFiles(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	i, ok := m.files.list.rowAtLine(msg.Y - filesRowsRow)
	if !ok || !m.files.list.focusRow(i) {
		return m, nil
	}
	return m.chooseFile()
}

// viewFiles draws the picker: a heading naming the directory, the list with its
// boxed query line, and a footer of keys. The heading is one line by
// construction (headingDir cuts the path from the front to fit), because every
// row constant below it depends on that.
func (m model) viewFiles() string {
	var b strings.Builder
	title := "Insert a path"
	b.WriteString(titleStyle.Render(title))
	b.WriteString("  ")
	// The title, two spaces, and a little slack for the title style's padding.
	room := 0
	if m.width > 0 {
		room = m.width - lipgloss.Width(titleStyle.Render(title)) - 4
	}
	b.WriteString(descStyle.Render(m.files.headingDir(room)))
	b.WriteString("\n\n")
	b.WriteString(m.files.list.view(m.files.filesEmptyMessage(), "", m.width))
	b.WriteString("\n")
	// In the order they must survive a narrowing pane: the two things the
	// picker is for, then the ways around, then the two spellings nobody guesses.
	b.WriteString(footerStyle.Render(m.fitFooter([]string{
		"enter insert", "tab/→ or / open folder", "backspace up", "esc back",
		"~/ and ../ paths", ". shows hidden",
	})))
	return b.String()
}
