package spell

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// dict loads the embedded lists once for the whole file — a 90k-word gunzip
// per test would dominate the suite for no coverage.
var dict = func() *Dictionary {
	d, err := Load()
	if err != nil {
		panic(err)
	}
	return d
}()

// flagged runs Check and returns the flagged words as strings, which is what
// every test here actually wants to assert on; the offsets are covered once,
// separately, in TestCheckOffsets.
func flagged(t *testing.T, text string) []string {
	t.Helper()
	var words []string
	rs := []rune(text)
	for _, sp := range dict.Check(text) {
		if sp.Start < 0 || sp.End > len(rs) || sp.Start >= sp.End {
			t.Fatalf("Check(%q): bad span %+v", text, sp)
		}
		words = append(words, string(rs[sp.Start:sp.End]))
	}
	return words
}

func TestLoadSize(t *testing.T) {
	if n := dict.Len(); n < 80_000 {
		t.Fatalf("dictionary has %d words; the embedded list looks truncated", n)
	}
}

func TestKnown(t *testing.T) {
	yes := []string{
		"the", "The", "cats", "running", "happiest", "don't", "don’t",
		"editor's", "english", "monday", "python", "json", "worktree",
	}
	for _, w := range yes {
		if !dict.Known(w) {
			t.Errorf("Known(%q) = false, want true", w)
		}
	}
	no := []string{"", "teh", "recieve", "definately", "wrods"}
	for _, w := range no {
		if dict.Known(w) {
			t.Errorf("Known(%q) = true, want false", w)
		}
	}
}

func TestCheck(t *testing.T) {
	cases := []struct {
		name, text string
		want       []string
	}{
		{"clean prose", "Refactor the parser so the tests pass again.", nil},
		{"one typo", "Refactor teh parser.", []string{"teh"}},
		{"two typos in order", "Plese fix the wrods here", []string{"Plese", "wrods"}},
		{"sentence-initial capital", "The quick brown fox.", nil},
		{"punctuation around words", `He said "hello", (really!) — twice; ok?`, nil},
		{"punctuation around typo", `(wrods)`, []string{"wrods"}},
		{"possessive and plural possessive", "the editor's caret, the cats' tails", nil},
		{"typographic apostrophe", "don’t and won’t", nil},
		{"hyphenated parts checked separately", "a well-knwon re-run of e-mail", []string{"knwon"}},
		{"paths skipped", "open ./cmd/foo/main.go and /etc/hosts and ~/.zshrc", nil},
		{"at-path skipped", "look at @internal/app/thing.go and @wrods", nil},
		{"flags skipped", "run with --sess-load and -v", nil},
		{"identifiers skipped", "call promptEditorView, snake_case_thing, ALLCAPS, URLs, x2", nil},
		{"urls and emails skipped", "see https://example.com/wrods and me@wrods.io", nil},
		{"digits skipped", "3rd sha256 v2 int64", nil},
		{"short words skipped", "db ui js ot", nil},
		{"non-ascii skipped", "Müller and naïve", nil},
		{"inline code skipped", "call `wrods()` then fix wrodz", []string{"wrodz"}},
		{"unclosed backtick is a character", "a lone ` then wrods", []string{"wrods"}},
		{"backtick does not cross lines", "open `wrods\nthen wrodz` here", []string{"wrods", "wrodz"}},
		{"fenced block skipped", "before wrodz\n```\nwrods here\n```\nafter wrodx", []string{"wrodz", "wrodx"}},
		{"unclosed fence swallows the rest", "before wrodz\n```go\nwrods here", []string{"wrodz"}},
		{"dev vocabulary", "commit the json config to github, rebase, and lint the goroutine", nil},
		{"markdown emphasis", "this is *wrods* and _also_ **bold**", []string{"wrods"}},
		{"bullets", "- item one\n- wrods here", []string{"wrods"}},
		{"empty", "", nil},
		{"whitespace only", "  \n\t ", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := flagged(t, c.text); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Check(%q)\n got %q\nwant %q", c.text, got, c.want)
			}
		})
	}
}

// TestCheckOffsets pins the spans to rune offsets, on a line whose byte and
// rune offsets differ — the em dash is three bytes — since a caller that turns
// spans into screen cells with a byte count would drift after it.
func TestCheckOffsets(t *testing.T) {
	text := "ok — wrods here"
	//       0123456789
	got := dict.Check(text)
	want := []Span{{Start: 5, End: 10}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Check(%q) = %+v, want %+v", text, got, want)
	}
}

func TestLoadUserFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dictionary.txt")
	if err := os.WriteFile(path, []byte("# mine\n\nwrods\n  Zorbulate  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Load(path, filepath.Join(dir, "missing.txt"), "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, w := range []string{"wrods", "Zorbulate", "zorbulate", "Wrods"} {
		if !d.Known(w) {
			t.Errorf("Known(%q) = false after loading the user file", w)
		}
	}
	if got := d.Check("wrods zorbulate wrodz"); len(got) != 1 || string([]rune("wrods zorbulate wrodz")[got[0].Start:got[0].End]) != "wrodz" {
		t.Errorf("Check with user words: got %+v", got)
	}

	// An unreadable file (a directory) is an error, unlike a missing one.
	if _, err := Load(dir); err == nil {
		t.Error("Load(directory) = nil error, want one")
	}
}
