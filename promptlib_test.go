package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// libInTemp points the config directory at a temp dir and returns the library
// file's path. Every test here writes and reads through the real file so the two
// accepted shapes, the tmp-and-rename write, and the missing-file case are all
// exercised where they actually live.
func libInTemp(t *testing.T) string {
	t.Helper()
	t.Setenv(configDirEnvVar, filepath.Join(t.TempDir(), "config"))
	return promptLibPath()
}

func writeLib(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestParsePromptLibShapes pins both file shapes. The wrapper object is what
// the program writes; the bare array is what a person writes by hand, and
// accepting it is the difference between a hand-edited library working and
// looking broken.
func TestParsePromptLibShapes(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
	}{
		{"wrapper object", `{"prompts":[{"name":"a","body":"alpha"},{"name":"b","body":"/beta"}]}`},
		{"bare array", `[{"name":"a","body":"alpha"},{"name":"b","body":"/beta"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, errMsg := parsePromptLib([]byte(tc.data), "prompts.json")
			if errMsg != "" {
				t.Fatalf("error = %q, want none", errMsg)
			}
			if len(got) != 2 || got[0].Name != "a" || got[1].Body != "/beta" {
				t.Errorf("parsed %+v, want the two entries in file order", got)
			}
		})
	}

	t.Run("an entry with no body is dropped, not fatal", func(t *testing.T) {
		got, errMsg := parsePromptLib([]byte(`{"prompts":[{"name":"empty","body":"  "},{"name":"real","body":"x"}]}`), "prompts.json")
		if errMsg != "" {
			t.Fatalf("error = %q, want none", errMsg)
		}
		if len(got) != 1 || got[0].Name != "real" {
			t.Errorf("parsed %+v, want only the entry that inserts something", got)
		}
	})

	t.Run("an empty file is an empty library", func(t *testing.T) {
		got, errMsg := parsePromptLib([]byte("  \n"), "prompts.json")
		if len(got) != 0 || errMsg != "" {
			t.Errorf("parsed %+v / %q, want an empty library and no complaint", got, errMsg)
		}
	})

	// A typo in a hand-edited file has to be reported. Silently reading it as
	// an empty library would look exactly like a lost library.
	t.Run("malformed json says so", func(t *testing.T) {
		got, errMsg := parsePromptLib([]byte(`{"prompts":[{"name":`), "~/.config/cats-todo/prompts.json")
		if len(got) != 0 {
			t.Errorf("parsed %+v from broken json", got)
		}
		if !strings.Contains(errMsg, "~/.config/cats-todo/prompts.json") {
			t.Errorf("error = %q, want it to name the file", errMsg)
		}
	})
}

// TestLoadPromptLibMissingFile: no library yet is the state everyone starts in,
// and it is not an error — only an unreadable or malformed file is.
func TestLoadPromptLibMissingFile(t *testing.T) {
	path := libInTemp(t)
	lib := loadPromptLib()
	if lib.err != "" {
		t.Errorf("err = %q for a library nobody has written yet", lib.err)
	}
	if len(lib.snippets) != 0 {
		t.Errorf("snippets = %+v, want none", lib.snippets)
	}
	if lib.path != path {
		t.Errorf("path = %q, want %q — the empty state names it", lib.path, path)
	}
}

// TestSnippetKindComesFromTheBody pins the rule that decides how an entry is
// inserted. Nothing declares it, so the classification is the whole contract.
func TestSnippetKindComesFromTheBody(t *testing.T) {
	for _, tc := range []struct {
		body    string
		command bool
		word    string
	}{
		{"/sess-load", true, "/sess-load"},
		{"/sess-load 2", true, "/sess-load"},
		{"  /commit", true, "/commit"},
		{"Steps to reproduce:", false, ""},
		{"and/or", false, ""},
		// A line of code is not a command, however it starts.
		{"// TODO: check this", false, ""},
	} {
		s := promptSnippet{Body: tc.body}
		if got := s.isCommand(); got != tc.command {
			t.Errorf("isCommand(%q) = %v, want %v", tc.body, got, tc.command)
		}
		if got := s.commandWord(); got != tc.word {
			t.Errorf("commandWord(%q) = %q, want %q", tc.body, got, tc.word)
		}
	}
}

// TestSnippetLabelAndSummary: a hand-written entry is entitled to leave the
// name or the description off, and the row still has to say something a person
// can recognize.
func TestSnippetLabelAndSummary(t *testing.T) {
	named := promptSnippet{Name: "repro", Desc: "how to file a bug", Body: "Steps:\n1. "}
	if got := named.label(); got != "repro" {
		t.Errorf("label = %q, want the name", got)
	}
	if got := named.summary(); got != "how to file a bug" {
		t.Errorf("summary = %q, want the description", got)
	}

	unnamed := promptSnippet{Body: "/sess-load 2"}
	if got := unnamed.label(); got != "/sess-load 2" {
		t.Errorf("label = %q, want the body it inserts", got)
	}
	if got := unnamed.commandLine(); got != "/sess-load 2" {
		t.Errorf("commandLine = %q, want the arguments kept — they are the difference between two entries", got)
	}
	prose := promptSnippet{Body: "Steps to reproduce:\n1. open the app"}
	if got := prose.label(); got != "Steps to reproduce: 1. open the app" {
		t.Errorf("label = %q, want the body flattened to one line", got)
	}
	if got := prose.summary(); strings.Contains(got, "\n") {
		t.Errorf("summary = %q, want no newline — it is drawn on a list row", got)
	}
}

// TestSnippetInsertion is the load-bearing table: where a chosen entry actually
// lands. A snippet must not be reformatted, and a command must end up alone on
// its line, because a slash command anywhere else is just text.
func TestSnippetInsertion(t *testing.T) {
	for _, tc := range []struct {
		name     string
		value    string
		caret    int
		body     string
		eatSlash bool
		want     string // the value afterwards, with | marking where the caret lands
	}{
		{
			name:  "a snippet lands exactly at the caret",
			value: "fix ", caret: 4, body: "the crash",
			want: "fix the crash|",
		},
		{
			name:  "a snippet keeps its own newlines and its trailing space",
			value: "", caret: 0, body: "Steps:\n1. ",
			want: "Steps:\n1. |",
		},
		{
			name:  "a command after text opens a line of its own",
			value: "fix the crash", caret: 13, body: "/sess-load",
			want: "fix the crash\n/sess-load\n|",
		},
		{
			name:  "a command on an empty line does not push a blank one ahead of it",
			value: "fix the crash\n", caret: 14, body: "/sess-load",
			want: "fix the crash\n/sess-load\n|",
		},
		{
			name:  "indentation still counts as a line start",
			value: "notes:\n  ", caret: 9, body: "/commit",
			want: "notes:\n  /commit\n|",
		},
		{
			name:  "a command before more text keeps the newline that is already there",
			value: "a\n\nmore", caret: 2, body: "/sess-load",
			want: "a\n/sess-load|\nmore",
		},
		{
			name:  "the typed slash is eaten rather than doubled",
			value: "a\n/", caret: 3, body: "/sess-load", eatSlash: true,
			want: "a\n/sess-load\n|",
		},
		{
			name:  "eatSlash with no slash behind the caret changes nothing",
			value: "a\n", caret: 2, body: "/sess-load", eatSlash: true,
			want: "a\n/sess-load\n|",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := []rune(tc.value)
			start, end, text := snippetInsertion(value, tc.caret, promptSnippet{Body: tc.body}, tc.eatSlash)
			got := string(value[:start]) + text + "|" + string(value[end:])
			if got != tc.want {
				t.Errorf("insertion = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPromptLibAddRoundTrips: an entry saved from the picker has to survive a
// restart, which means it has to be on disk in a shape loadPromptLib reads.
func TestPromptLibAddRoundTrips(t *testing.T) {
	libInTemp(t)
	lib := loadPromptLib()
	if err := lib.add(promptSnippet{Name: "repro", Body: "Steps:\n1. "}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := lib.add(promptSnippet{Name: "load", Body: "/sess-load"}); err != nil {
		t.Fatalf("add: %v", err)
	}

	back := loadPromptLib()
	if back.err != "" {
		t.Fatalf("reload err = %q", back.err)
	}
	if len(back.snippets) != 2 {
		t.Fatalf("reloaded %+v, want both entries", back.snippets)
	}
	if back.snippets[0].Name != "repro" || back.snippets[1].Body != "/sess-load" {
		t.Errorf("reloaded %+v, want them in the order they were added", back.snippets)
	}
	if !back.hasCommands() {
		t.Error("hasCommands = false with /sess-load in the library")
	}
}

// TestPromptLibRefusesADuplicateName: overwriting is the destructive reading of
// an ambiguous gesture, so the save is refused in words instead.
func TestPromptLibRefusesADuplicateName(t *testing.T) {
	libInTemp(t)
	lib := loadPromptLib()
	if err := lib.add(promptSnippet{Name: "repro", Body: "first"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	err := lib.add(promptSnippet{Name: "  REPRO ", Body: "second"})
	if err == nil {
		t.Fatal("a second “repro” was accepted, overwriting the first")
	}
	if !strings.Contains(err.Error(), "already in the library") {
		t.Errorf("error = %q, want it to say the name is taken", err)
	}
	if got := loadPromptLib().snippets; len(got) != 1 || got[0].Body != "first" {
		t.Errorf("library = %+v, want the original entry untouched", got)
	}
}

func TestPromptLibRefusesAnEmptyNameOrBody(t *testing.T) {
	libInTemp(t)
	lib := loadPromptLib()
	if err := lib.add(promptSnippet{Name: "  ", Body: "x"}); err == nil {
		t.Error("an entry with no name was accepted")
	}
	if err := lib.add(promptSnippet{Name: "x", Body: "  "}); err == nil {
		t.Error("an entry with no body was accepted")
	}
	if _, err := os.Stat(promptLibPath()); !os.IsNotExist(err) {
		t.Errorf("a refused save still wrote the file (stat err = %v)", err)
	}
}

// TestPromptLibReadsAHandWrittenBareArray closes the loop the parse test opens:
// the shape a person types has to work through the real file, not just through
// the parser.
func TestPromptLibReadsAHandWrittenBareArray(t *testing.T) {
	path := libInTemp(t)
	writeLib(t, path, `[{"name": "commit", "body": "/commit"}]`)
	lib := loadPromptLib()
	if lib.err != "" {
		t.Fatalf("err = %q reading a hand-written array", lib.err)
	}
	if len(lib.snippets) != 1 || !lib.snippets[0].isCommand() {
		t.Errorf("snippets = %+v, want the one command", lib.snippets)
	}
}
