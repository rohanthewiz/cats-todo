package spell

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestSuggestOffersTheWordThatWasMeant is the whole feature in one table: for a
// misspelling anyone actually types, the word they meant is the first row of
// the panel. Every case here is a real class of mistake — a transposition, a
// doubled letter, a missing one, a substituted vowel, a contraction typed
// without its apostrophe — and the ordering rules exist to make exactly this
// pass (see the package comment).
func TestSuggestOffersTheWordThatWasMeant(t *testing.T) {
	for _, tc := range []struct{ typo, want string }{
		{"teh", "the"},           // the transposition, and the reason edits are weighted
		{"dont", "don't"},        // a contraction, not a typo at all
		{"wrods", "words"},       // a transposition inside a longer word
		{"goign", "going"},       // and one at the end
		{"recieve", "receive"},   // i before e
		{"beleive", "believe"},   // the same, the other way round
		{"seperate", "separate"}, // the vowel everyone gets wrong
		{"occured", "occurred"},  // a consonant that should have doubled
		{"accomodate", "accommodate"},
		{"definately", "definitely"},
		{"langauge", "language"},
		{"existance", "existence"},
		{"sugestion", "suggestion"},
		{"adress", "address"},
		{"Wendesday", "Wednesday"}, // a proper noun, capitalized on the way back
		{"refactorr", "refactor"},  // a word extra.txt supplies, not SCOWL
	} {
		got := dict.Suggest(tc.typo, 8)
		if len(got) == 0 {
			t.Errorf("Suggest(%q) offered nothing, want %q first", tc.typo, tc.want)
			continue
		}
		if got[0] != tc.want {
			t.Errorf("Suggest(%q) = %v, want %q first", tc.typo, got, tc.want)
		}
	}
}

// TestSuggestKeepsTheCaseTyped: a misspelling that started a sentence gets its
// corrections capitalized, so accepting one does not also lowercase the line.
// A word the list holds capitalized keeps its capital either way — that is the
// dictionary's spelling, not the typist's.
func TestSuggestKeepsTheCaseTyped(t *testing.T) {
	if got := dict.Suggest("Teh", 3); len(got) == 0 || got[0] != "The" {
		t.Errorf("Suggest(%q) = %v, want %q first", "Teh", got, "The")
	}
	if got := dict.Suggest("teh", 3); len(got) == 0 || got[0] != "the" {
		t.Errorf("Suggest(%q) = %v, want %q first", "teh", got, "the")
	}
	// Typed lowercase, held capitalized: the correction comes back as the
	// dictionary spells it, since that is the thing being suggested.
	if got := dict.Suggest("wendesday", 3); len(got) == 0 || got[0] != "Wednesday" {
		t.Errorf("Suggest(%q) = %v, want %q first", "wendesday", got, "Wednesday")
	}
}

// TestSuggestQuiets: nothing to correct, nothing offered. A known word has no
// suggestions by definition, and a word far enough from every entry has none
// worth printing — a panel that filled itself with unrelated words for a
// deliberate nonsense string would teach the user to stop reading it.
func TestSuggestQuiets(t *testing.T) {
	for _, word := range []string{"the", "receive", "goroutine", "Wednesday", "don't"} {
		if got := dict.Suggest(word, 8); len(got) != 0 {
			t.Errorf("Suggest(%q) = %v for a word the dictionary knows, want none", word, got)
		}
	}
	if got := dict.Suggest("qwertyuiopasdf", 8); len(got) != 0 {
		t.Errorf("Suggest of a keyboard mash = %v, want none", got)
	}
	if got := dict.Suggest("", 8); got != nil {
		t.Errorf("Suggest(\"\") = %v, want nil", got)
	}
	if got := dict.Suggest("teh", 0); got != nil {
		t.Errorf("Suggest with max 0 = %v, want nil", got)
	}
}

// TestSuggestCaps: the caller's max is honoured, and the package's own cap
// applies over it however large a number is asked for.
func TestSuggestCaps(t *testing.T) {
	if got := dict.Suggest("wrods", 3); len(got) != 3 {
		t.Errorf("Suggest with max 3 returned %d rows: %v", len(got), got)
	}
	if got := dict.Suggest("wrods", 500); len(got) > suggestMax {
		t.Errorf("Suggest with max 500 returned %d rows, want at most %d", len(got), suggestMax)
	}
}

// TestSuggestIsStable: the same misspelling offers the same list in the same
// order every time. It is worth a test because the candidates are gathered by
// ranging over a map, whose order Go deliberately randomizes — a ranking that
// left any pair of rows tied would shuffle between openings of the panel.
func TestSuggestIsStable(t *testing.T) {
	first := dict.Suggest("wrods", 8)
	for range 20 {
		if got := dict.Suggest("wrods", 8); !slices.Equal(got, first) {
			t.Fatalf("Suggest is not stable: %v then %v", first, got)
		}
	}
}

// TestSuggestWeighting pins the three edit costs against each other, since the
// ordering they produce is the whole difference between a useful panel and a
// list of near-misses: an apostrophe left out beats a transposition, which
// beats a substitution.
func TestSuggestWeighting(t *testing.T) {
	buf := newEditBuf(8)
	apostrophe := buf.cost([]rune("dont"), []rune("don't"), 100)
	transpose := buf.cost([]rune("teh"), []rune("the"), 100)
	substitute := buf.cost([]rune("teh"), []rune("tea"), 100)
	if !(apostrophe < transpose && transpose < substitute) {
		t.Errorf("costs are apostrophe=%d transpose=%d substitute=%d, want them in that order",
			apostrophe, transpose, substitute)
	}
	// A word past the limit answers "further than that" rather than a distance —
	// exactly limit+1 when the walk gave up early, and the true cost when the
	// word was short enough to finish first. Either way it is over the limit,
	// which is the only thing the scan asks.
	if got := buf.cost([]rune("teh"), []rune("elephant"), 20); got <= 20 {
		t.Errorf("cost from %q to %q = %d, want over the limit", "teh", "elephant", got)
	}
	if got := buf.cost([]rune("transposition"), []rune("elephantine"), 20); got != 21 {
		t.Errorf("cost of a walk that should have given up = %d, want limit+1", got)
	}
	// An empty side costs one indel per rune, at that rune's own price.
	if got, want := buf.cost(nil, []rune("ab"), 100), 2*costEdit; got != want {
		t.Errorf("cost from nothing to %q = %d, want %d", "ab", got, want)
	}
}

// TestSuggestTypographicApostrophe: a prompt written in an editor that
// straightens quotes for you still gets corrections, because the two spellings
// of the character are folded before the lookup — the same fold Known does.
func TestSuggestTypographicApostrophe(t *testing.T) {
	if got := dict.Suggest("do’nt", 5); len(got) == 0 || got[0] != "don't" {
		t.Errorf("Suggest(%q) = %v, want %q first", "do’nt", got, "don't")
	}
}

// TestAddTeachesALoadedDictionary: a word added to the running dictionary is
// known at once, without the 90k-word reload that learning it from a file
// would otherwise cost.
func TestAddTeachesALoadedDictionary(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if d.Known("zorbulate") {
		t.Fatal("the test word is somehow already in the list")
	}
	d.Add("zorbulate")
	if !d.Known("zorbulate") {
		t.Error("Add did not teach the dictionary the word")
	}
	// Lookup stays as lenient for an added word as for a listed one.
	if !d.Known("Zorbulate") {
		t.Error("an added word is not known capitalized")
	}
	if n := d.Len(); n == 0 {
		t.Error("Len went to zero")
	}
	// Blank input is not a word and must not become one — an empty entry would
	// match nothing and sit in the file forever.
	before := d.Len()
	d.Add("   ")
	if d.Len() != before {
		t.Error("Add stored a blank word")
	}
}

// TestAppendWordCreatesTheFile: the first word written to a dictionary that
// does not exist yet creates it, and its directory, with a header explaining
// what the file is — it is meant to be opened by hand.
func TestAppendWordCreatesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dictionary.txt")
	if err := AppendWord(path, "zorbulate"); err != nil {
		t.Fatalf("AppendWord: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "#") {
		t.Errorf("a created dictionary has no header:\n%s", data)
	}
	if !strings.HasSuffix(string(data), "zorbulate\n") {
		t.Errorf("the word is not the last line:\n%s", data)
	}
	// And Load reads it back, which is the only reason the file is written.
	d, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Known("zorbulate") {
		t.Error("the word written to the file is not known after a reload")
	}
}

// TestAppendWordDoesNotRepeatItself: a word the file already lists is left
// alone, whatever case it was listed in, and a hand-edited file that ends
// mid-line gets its newline before the new word rather than losing both.
func TestAppendWordDoesNotRepeatItself(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.txt")
	if err := os.WriteFile(path, []byte("# mine\nKubernetes\ncatctl"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"kubernetes", "Kubernetes", "KUBERNETES"} {
		if err := AppendWord(path, w); err != nil {
			t.Fatalf("AppendWord(%q): %v", w, err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(strings.ToLower(string(data)), "kubernetes"); got != 1 {
		t.Errorf("the word was written %d times:\n%s", got, data)
	}
	// The last line had no newline; the next word must not be joined onto it.
	if err := AppendWord(path, "bytdb"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "catctlbytdb") {
		t.Errorf("the new word was joined onto an unfinished line:\n%s", data)
	}
	if !strings.HasSuffix(string(data), "bytdb\n") {
		t.Errorf("the new word is not the last line:\n%s", data)
	}
	// A file that already existed is not given a second header.
	if got := strings.Count(string(data), "# mine"); got != 1 {
		t.Errorf("the existing header was disturbed:\n%s", data)
	}
}

// TestAppendWordRefusesWhatCannotBeRead: a line with a space in it is read back
// whole and can then never match a token, so it would be a word that silently
// does nothing. Better to say so than to write it.
func TestAppendWordRefusesWhatCannotBeRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.txt")
	for _, w := range []string{"", "   ", "two words", "with\ttab"} {
		if err := AppendWord(path, w); err == nil {
			t.Errorf("AppendWord(%q) succeeded, want an error", w)
		}
	}
	if err := AppendWord("", "word"); err == nil {
		t.Error("AppendWord with no path succeeded, want an error")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("a refused word still created the file")
	}
	// An unreadable file is an error rather than a silent overwrite: the words
	// already in it are the user's, and appending blind would be a way to lose
	// them.
	dir := filepath.Join(t.TempDir(), "dictionary.txt")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := AppendWord(dir, "word"); err == nil {
		t.Error("AppendWord into a directory succeeded, want an error")
	}
}
