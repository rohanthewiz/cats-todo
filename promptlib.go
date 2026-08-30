// promptlib.go — the user-level prompt library.
//
// The same paragraphs get typed into the prompt editor over and over: the way
// you like a bug reproduced, the review checklist, the "/sess-load" that opens
// every session on this machine. The library is where those live, once, so that
// writing one is a pick from a list rather than a retype.
//
// It is deliberately USER-level, not project-level. A phrasing that is worth
// keeping is a habit of the person, not of the repository — the same wording
// goes into a prompt whichever checkout the manager was launched from — so it
// sits beside settings.json in the global config directory (see configBaseDir)
// and every project sees the same list. Backlogs stay per-project; the words you
// write them with do not.
//
//	~/.config/cats-todo/prompts.json
//	{
//	  "prompts": [
//	    {"name": "repro steps", "desc": "how to file a bug", "body": "Steps to reproduce:\n1. "},
//	    {"name": "load session", "body": "/sess-load"}
//	  ]
//	}
//
// Two kinds of entry, told apart by the body rather than by a field anyone has
// to set: a body that starts with '/' is a COMMAND (a Claude Code slash command
// or skill), everything else is a snippet. The distinction is not cosmetic —
// a slash command is only a command when it begins a line, so inserting one
// puts it on a line of its own, while a snippet lands exactly at the caret and
// nowhere else (see snippetInsertion). Deriving the kind from the text means a
// hand-written file never has to declare it, and an entry cannot be typed as one
// thing and stored as another.
//
// The file is read fresh on every open rather than cached on the model. It is a
// few hundred bytes, the read happens on a keystroke a person made on purpose,
// and freshness is the point: this is a file people edit by hand in another
// window, and a library that needed a restart to notice would be the kind of
// tool you stop trusting. (filepick.go takes the same position on os.ReadDir,
// for the same reason.)

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// promptLibFileName is the library's name inside configBaseDir.
const promptLibFileName = "prompts.json"

// promptSnippet is one entry: a short name to find it by, an optional
// description, and the text that goes into the prompt.
//
// Only the body is load-bearing. Name and desc are how a person picks the entry
// out of a list; body is the whole of what the editor receives, verbatim,
// trailing space included — an entry ending in "1. " means to leave the caret
// after that space, and trimming it here would quietly undo the author's
// formatting.
type promptSnippet struct {
	Name string `json:"name"`
	Desc string `json:"desc,omitempty"`
	Body string `json:"body"`
}

// isCommand reports whether the entry is a slash command rather than prose.
//
// Leading blanks are ignored so an indented entry still counts, and a body that
// starts with "//" does not: that is a line of code (or a URL fragment) far more
// often than it is a command, and forcing it onto a line of its own would
// reformat a snippet its author had already laid out.
func (s promptSnippet) isCommand() bool {
	b := strings.TrimLeft(s.Body, " \t")
	return strings.HasPrefix(b, "/") && !strings.HasPrefix(b, "//")
}

// commandWord is the command itself — "/sess-load" out of "/sess-load 2" — or
// "" for a snippet. It is what the note line says after an insert: the command
// is the part of the body that identifies what just landed, and echoing its
// arguments back would only repeat what is now on screen.
func (s promptSnippet) commandWord() string {
	if !s.isCommand() {
		return ""
	}
	return strings.Fields(strings.TrimLeft(s.Body, " \t"))[0]
}

// label is what the picker draws as the entry's name: the name it was given,
// or — for an entry written without one, which a hand-edited file is entitled
// to hold — its body flattened to a line. An unnamed row is still a row someone
// can recognize and pick.
func (s promptSnippet) label() string {
	if n := strings.TrimSpace(s.Name); n != "" {
		return n
	}
	return truncate(collapseLines(s.Body), 40)
}

// commandLine is the whole command as one line — "/sess-load 2" — or "" for a
// snippet. It is what the picker draws beside the name: the command word alone
// would hide the arguments, which are half of what an entry like "/sess-load 2"
// is for.
func (s promptSnippet) commandLine() string {
	if !s.isCommand() {
		return ""
	}
	return collapseLines(s.Body)
}

// summary is the row's description: the entry's own words when it has them,
// otherwise the body flattened to one line. The body is the better fallback
// than nothing — what an entry inserts is the question a picker is being asked.
func (s promptSnippet) summary() string {
	if d := strings.TrimSpace(s.Desc); d != "" {
		return d
	}
	return collapseLines(s.Body)
}

// collapseLines flattens text to a single line: every run of whitespace becomes
// one space. Used wherever a multi-line body has to fit on a row, since a raw
// "\n" in a rendered row would break the list's geometry rather than wrap.
func collapseLines(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// promptLibrary is the library as read from disk: the entries in file order,
// where they came from, and why the read failed when it did.
//
// File order is the user's order, the same rule the backlogs follow: a list
// someone arranged by hand is not re-sorted behind their back, and the fuzzy
// query is the only thing that ever reorders the rows.
type promptLibrary struct {
	snippets []promptSnippet
	// path is the file, even when it does not exist yet — the empty state names
	// it, which is the whole answer to "where do I put these?".
	path string
	// err is why the file could not be read or parsed, or "" when it could. A
	// missing file is NOT an error: it is an empty library, which is what
	// everyone starts with. A malformed one is, and it is reported rather than
	// swallowed — a typo in a hand-edited file that silently emptied the list
	// would look exactly like a lost library.
	err string
}

// promptLibFile is the on-disk shape. The wrapper object exists so the file can
// grow keys later without breaking every reader — the same reason settingsFile
// is an object — but a bare top-level array is accepted too, because that is
// what a hand-written file naturally looks like and refusing it would be
// pedantry the user pays for.
type promptLibFile struct {
	Prompts []promptSnippet `json:"prompts"`
}

// promptLibPath is where the library lives, or "" when the config directory
// cannot be resolved (no home directory) — in which case the library is simply
// unavailable for this run, and the picker says so.
func promptLibPath() string {
	base, err := configBaseDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, promptLibFileName)
}

// loadPromptLib reads the library. Every outcome is a usable value: no path, no
// file, and a broken file all return a library the picker can open — empty, and
// carrying the sentence that explains itself.
func loadPromptLib() promptLibrary {
	lib := promptLibrary{path: promptLibPath()}
	if lib.path == "" {
		lib.err = "no config directory to keep a prompt library in"
		return lib
	}
	data, err := os.ReadFile(lib.path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			lib.err = "cannot read " + shortenHome(lib.path) + ": " + err.Error()
		}
		return lib
	}
	lib.snippets, lib.err = parsePromptLib(data, shortenHome(lib.path))
	return lib
}

// parsePromptLib reads either accepted shape and drops entries with nothing to
// insert. It is separate from the file handling so both shapes and the failure
// message can be tested without a disk.
//
// An entry whose body is empty is skipped rather than refused: it inserts
// nothing, so it is a row that can only waste a pick, and one such line should
// not cost the reader the rest of a file they hand-wrote.
func parsePromptLib(data []byte, where string) ([]promptSnippet, string) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, ""
	}
	var wrapped promptLibFile
	err := json.Unmarshal(data, &wrapped)
	list := wrapped.Prompts
	if err != nil {
		// Not an object — try the bare array before giving up, and report the
		// object's error if that fails too, since the object is the documented
		// shape and its message is the more useful of the two.
		var bare []promptSnippet
		if json.Unmarshal(data, &bare) != nil {
			return nil, "cannot read " + where + ": " + err.Error()
		}
		list = bare
	}
	out := make([]promptSnippet, 0, len(list))
	for _, s := range list {
		if strings.TrimSpace(s.Body) == "" {
			continue
		}
		out = append(out, s)
	}
	return out, ""
}

// hasCommands reports whether any entry is a slash command. It is the guard on
// the '/' trigger in the editor (see updateForm): someone who keeps no commands
// never typed '/' to ask for one, and a picker that opened on every slash typed
// at a line start would be in the way of "/Users/ro/…" for no gain.
func (l promptLibrary) hasCommands() bool {
	for _, s := range l.snippets {
		if s.isCommand() {
			return true
		}
	}
	return false
}

// find returns the index of the entry with this name, case-insensitively, or
// -1. Names are how a person refers to an entry, and "Repro" and "repro" are
// the same reference to one — so they collide when a new entry is saved.
func (l promptLibrary) find(name string) int {
	name = strings.TrimSpace(name)
	for i, s := range l.snippets {
		if strings.EqualFold(strings.TrimSpace(s.Name), name) {
			return i
		}
	}
	return -1
}

// add appends an entry and writes the file. It refuses a duplicate name rather
// than replacing one: overwriting is the destructive reading of an ambiguous
// gesture, and the message says exactly what to do about it.
//
// The whole library is rewritten, not appended to, so the file stays valid JSON
// and keeps its order. It is small enough that there is nothing to save by
// being cleverer.
func (l *promptLibrary) add(s promptSnippet) error {
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("a saved prompt needs a name")
	}
	if strings.TrimSpace(s.Body) == "" {
		return errors.New("there is nothing to save")
	}
	if i := l.find(s.Name); i >= 0 {
		return fmt.Errorf("%q is already in the library — pick another name", strings.TrimSpace(l.snippets[i].Name))
	}
	s.Name = strings.TrimSpace(s.Name)
	next := append(append([]promptSnippet{}, l.snippets...), s)
	if err := writePromptLib(l.path, next); err != nil {
		return err
	}
	l.snippets = next
	l.err = ""
	return nil
}

// writePromptLib saves the entries, creating the config directory if it is not
// there yet. Write-then-rename, the shape store.save and settings.save use: a
// crash mid-write leaves the previous library rather than a truncated one, and
// a library is exactly the kind of file whose loss is invisible until the day
// you reach for it.
func writePromptLib(path string, snippets []promptSnippet) error {
	if path == "" {
		return errors.New("no config directory to save a prompt library in")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(promptLibFile{Prompts: snippets}, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// --- Insertion ------------------------------------------------------------------

// snippetInsertion works out what an entry actually splices into the prompt: the
// rune span it replaces and the text that goes there. It is a pure function of
// the value and the caret so the rules below can be tested without a textarea,
// which is the only way the line-edge cases ever get exercised.
//
// value is the editor's text as runes, caret the offset the insertion happens
// at, and eatSlash says the caret has a '/' immediately behind it that the user
// typed to open the picker (see the trigger in updateForm) — that slash is
// consumed, so the command replaces it instead of doubling it.
//
// A SNIPPET goes in exactly as written, at the caret, and changes nothing around
// it. Its author already decided where its newlines are.
//
// A COMMAND has to begin a line and end one, because that is the only place a
// slash command is a command:
//
//	"fix the crash|"          + /sess-load  →  "fix the crash\n/sess-load\n|"
//	"fix the crash\n|"        + /sess-load  →  "fix the crash\n/sess-load\n|"
//	"fix the crash\n/|"       + /sess-load  →  "fix the crash\n/sess-load\n|"   (eatSlash)
//	"fix the crash\n|\nmore"  + /sess-load  →  "fix the crash\n/sess-load\n|\nmore"
//
// The leading newline is added only when there is real text behind the caret on
// its line, so a command dropped on a blank line does not push a blank line
// ahead of it; the trailing one is added unless the text already continues on a
// new line, which leaves the caret at the start of the next line either way —
// ready for the next command, or for the prose the command was an aside from.
func snippetInsertion(value []rune, caret int, s promptSnippet, eatSlash bool) (start, end int, text string) {
	caret = min(max(caret, 0), len(value))
	start, end = caret, caret
	if eatSlash && start > 0 && value[start-1] == '/' {
		start--
	}
	text = s.Body
	if !s.isCommand() {
		return start, end, text
	}
	// What is behind the insertion point on its own line, and what is ahead of
	// it. The slash the trigger typed is already outside the span by now, so it
	// cannot count as text the command has to move past.
	before := string(value[:start])
	if i := strings.LastIndex(before, "\n"); i >= 0 {
		before = before[i+1:]
	}
	if strings.TrimSpace(before) != "" {
		text = "\n" + text
	}
	after := string(value[end:])
	if !strings.HasPrefix(after, "\n") {
		text += "\n"
	}
	return start, end, text
}

// insertSnippet splices the entry into the prompt at the caret and reports what
// it did, in the words the form's note line uses.
//
// It goes through replacePromptRunes (spellpanel.go) rather than the textarea's
// InsertString because the insertion can start one rune behind the caret — the
// '/' the trigger typed — and that helper is the one path that can edit a span
// the caret is not sitting at and then put the caret where the edit ended.
func (m *model) insertSnippet(s promptSnippet, eatSlash bool) string {
	value := []rune(m.promptArea.Value())
	start, end, text := snippetInsertion(value, promptCaretOffset(m.promptArea), s, eatSlash)
	m.replacePromptRunes(start, end, text)
	if s.isCommand() {
		return "inserted " + s.commandWord()
	}
	return "inserted “" + s.label() + "”"
}
