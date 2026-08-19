// Package spell is the prompt editor's spell checker: a word list and a
// tokenizer that knows what a prompt to a coding agent looks like.
//
// It is deliberately a *checker* and not a speller. There are no suggestions,
// no affix rules and no language model — just "is this word in the list", asked
// of every run of letters the tokenizer decides is prose. All of the judgement
// is in that decision (see Check): a prompt is full of paths, identifiers,
// flags and code, and a checker that painted those red would be noise nobody
// left switched on. So the tokenizer skips anything that looks like it was
// meant for a machine rather than a reader, and only what is left is looked up.
//
// The word list is SCOWL's en_US (size 60) expanded from its hunspell affix
// rules into a flat list — about 90k forms, 250 KB gzipped, embedded in the
// binary — plus extra.txt, the everyday vocabulary of software work that a
// general-purpose English list leaves out. Both are embedded so the checker
// works the same on every machine; a system dictionary would have made the
// feature Unix-only and, on macOS, considerably noisier (its web2 list is
// headwords only — no plurals, no "running").
package spell

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

//go:embed en_US.txt.gz
var enUS []byte

//go:embed extra.txt
var extra []byte

// Span is one run of the checked text the dictionary did not know, as rune
// offsets [Start, End) into that text. Rune offsets rather than bytes because
// the editor addresses its value the same way, and the caller's job is to turn
// a span into screen cells (see promptEditorView) — a byte offset would just
// have to be converted first.
type Span struct {
	Start, End int
}

// Dictionary is a set of known words. It is safe for concurrent reads once
// built; it is not meant to be modified after Load returns.
type Dictionary struct {
	words map[string]struct{}
}

// Load builds the dictionary from the embedded lists plus each of userFiles
// that exists. The files are one word per line, '#' comments and blank lines
// ignored — the same shape as extra.txt, so a user can grow it in an editor.
//
// A missing user file is not an error: it is the normal state of a fresh
// install, and the check is what tells the caller it needs to be created, not
// the loader. Any other failure to read one is returned, since a dictionary
// silently missing the words someone added is a checker that flags what they
// told it not to.
func Load(userFiles ...string) (*Dictionary, error) {
	d := &Dictionary{words: make(map[string]struct{}, 100_000)}

	zr, err := gzip.NewReader(bytes.NewReader(enUS))
	if err != nil {
		return nil, err
	}
	if err := d.addFrom(zr); err != nil {
		return nil, err
	}
	if err := d.addFrom(bytes.NewReader(extra)); err != nil {
		return nil, err
	}
	for _, path := range userFiles {
		if path == "" {
			continue
		}
		f, err := os.Open(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		err = d.addFrom(f)
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	return d, nil
}

// addFrom reads one word per line into the set. Comments and surrounding
// space are dropped; a "word" that has spaces inside is kept whole and will
// simply never match, which is harmless.
func (d *Dictionary) addFrom(r io.Reader) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		d.words[line] = struct{}{}
	}
	return sc.Err()
}

// Len is how many words the dictionary holds.
func (d *Dictionary) Len() int { return len(d.words) }

// Known reports whether word is in the dictionary, allowing for the ways a
// correctly spelled word differs from its listed form:
//
//   - a sentence-initial capital ("The" for "the"),
//   - a proper noun written lowercase ("english", "monday", "python") — the
//     list has these capitalized, and flagging the case would be flagging
//     style, which is not this checker's business,
//   - a possessive ("editor's"), which the list does not carry — the "'s"
//     forms were dropped when it was built, since they double its size and
//     say nothing a suffix strip does not,
//   - a typographic apostrophe ("don’t" for "don't").
func (d *Dictionary) Known(word string) bool {
	if word == "" {
		return false
	}
	word = strings.ReplaceAll(word, "’", "'")
	if d.has(word) {
		return true
	}
	if base, ok := strings.CutSuffix(word, "'s"); ok && d.has(base) {
		return true
	}
	return false
}

// has is the case-lenient lookup Known describes: as written, then lowercased,
// then with just the first letter capitalized.
func (d *Dictionary) has(w string) bool {
	if _, ok := d.words[w]; ok {
		return true
	}
	lower := strings.ToLower(w)
	if lower != w {
		if _, ok := d.words[lower]; ok {
			return true
		}
	}
	// Title-case is built from the lowercase form so "aARON" does not sneak
	// through — though a word with an internal capital never reaches here
	// (Check skips it as an identifier), so this is belt and braces.
	first, size := utf8.DecodeRuneInString(lower)
	title := string(unicode.ToUpper(first)) + lower[size:]
	if title != w {
		if _, ok := d.words[title]; ok {
			return true
		}
	}
	return false
}

// Check returns the spans of text that look like prose words and are not in
// the dictionary, in order of appearance.
//
// What "looks like prose" means is the whole of the checker's judgement, and
// the rules err towards silence: a typo that goes unflagged costs a glance, a
// path or identifier flagged on every prompt costs the feature. So a run of
// text is skipped, not checked, when it is:
//
//   - inside backticks — `code` or a ``` fenced block — since that is code by
//     declaration;
//   - a token starting with '@', '-', '#', '~', '$', '/', '.', '\', '<' or
//     '&': an @path (the file picker's insertion), a --flag, a #tag or
//     #issue, ~/home, an absolute path, a .dotfile, a <tag>, an &entity;
//   - a token that, once quotes and brackets are trimmed off its ends, still
//     holds a digit or any character other than letters, apostrophes and
//     hyphens: paths, URLs, snake_case, e.g., v2, foo/bar, key=value;
//   - a word with a capital after its first letter (CamelCase, ALLCAPS, an
//     acronym) or a letter outside ASCII (a name, or another language — the
//     list is English);
//   - a word of one or two letters, where abbreviations (db, ui, js) outnumber
//     typos and no dictionary settles the difference.
//
// A hyphenated compound is checked one part at a time ("well-knwon" flags
// only "knwon"), which is also how the two-letter rule lets "re-run" and
// "e-mail" through.
//
// Only rune offsets are returned; the caller decides what a span means on
// screen. Check keeps no state, so calling it on every redraw is fine at
// prompt sizes — a few thousand runes are a few thousand table lookups.
func (d *Dictionary) Check(text string) []Span {
	rs := []rune(text)
	var out []Span
	i := 0
	for i < len(rs) {
		switch {
		case hasPrefixRunes(rs, i, "```"):
			// A fenced block runs to its closing fence, or to the end of the
			// text when the fence is still being typed — either way none of it
			// is prose.
			end := indexRunes(rs, i+3, "```")
			if end < 0 {
				return out
			}
			i = end + 3
			continue
		case rs[i] == '`':
			// Inline code closes on the same line. An unclosed backtick is
			// just a character — most likely a closing one whose opener the
			// scanner has already passed as prose, or a typo — and the text
			// after it is checked as usual.
			if end := indexRunesBefore(rs, i+1, '`', '\n'); end >= 0 {
				i = end + 1
			} else {
				i++
			}
			continue
		case unicode.IsSpace(rs[i]):
			i++
			continue
		}
		j := i
		for j < len(rs) && !unicode.IsSpace(rs[j]) && rs[j] != '`' {
			j++
		}
		out = d.checkToken(rs, i, j, out)
		i = j
	}
	return out
}

// checkToken applies the rules Check lists to one whitespace-delimited token,
// rs[lo:hi], appending a span for each part of it that fails lookup.
func (d *Dictionary) checkToken(rs []rune, lo, hi int, out []Span) []Span {
	switch rs[lo] {
	case '@', '-', '#', '~', '$', '/', '.', '\\', '<', '&':
		return out
	}
	// Trim the punctuation prose wraps a word in — quotes, brackets, a trailing
	// comma or full stop — by trimming everything that is not a letter or
	// digit. A digit is kept on purpose: it makes the token fail the scan below,
	// which is the right answer for "3rd", "v2" and "sha256".
	for lo < hi && !isWordRune(rs[lo]) {
		lo++
	}
	for hi > lo && !isWordRune(rs[hi-1]) {
		hi--
	}
	if lo >= hi {
		return out
	}
	// The core is checked only if it is made of nothing but letters,
	// apostrophes and hyphens; anything else marks it as not-prose. Non-ASCII
	// letters count as "anything else" for the reason Check gives.
	for _, r := range rs[lo:hi] {
		if r == '\'' || r == '’' || r == '-' {
			continue
		}
		if r > unicode.MaxASCII || !unicode.IsLetter(r) {
			return out
		}
	}
	// One span per hyphen-separated part.
	start := lo
	for k := lo; k <= hi; k++ {
		if k < hi && rs[k] != '-' {
			continue
		}
		out = d.checkWord(rs, start, k, out)
		start = k + 1
	}
	return out
}

// checkWord looks up one hyphen-free part, rs[lo:hi], and appends a span if
// it is a checkable word the dictionary lacks.
func (d *Dictionary) checkWord(rs []rune, lo, hi int, out []Span) []Span {
	// A part can begin or end with an apostrophe when the token did ("'quoted
	// word'" trimmed to a core still holding them, or "cats'"). Those are
	// quotation, not spelling.
	for lo < hi && (rs[lo] == '\'' || rs[lo] == '’') {
		lo++
	}
	for hi > lo && (rs[hi-1] == '\'' || rs[hi-1] == '’') {
		hi--
	}
	if hi-lo <= 2 {
		return out
	}
	for _, r := range rs[lo+1 : hi] {
		if unicode.IsUpper(r) {
			return out
		}
	}
	if d.Known(string(rs[lo:hi])) {
		return out
	}
	return append(out, Span{Start: lo, End: hi})
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// hasPrefixRunes reports whether rs[at:] starts with s.
func hasPrefixRunes(rs []rune, at int, s string) bool {
	for _, r := range s {
		if at >= len(rs) || rs[at] != r {
			return false
		}
		at++
	}
	return true
}

// indexRunes is the rune index of the first occurrence of s in rs at or after
// from, or -1.
func indexRunes(rs []rune, from int, s string) int {
	for i := from; i < len(rs); i++ {
		if hasPrefixRunes(rs, i, s) {
			return i
		}
	}
	return -1
}

// indexRunesBefore is the rune index of the first r in rs at or after from
// that comes before any stop, or -1 when stop (or the end) comes first.
func indexRunesBefore(rs []rune, from int, r, stop rune) int {
	for i := from; i < len(rs); i++ {
		switch rs[i] {
		case r:
			return i
		case stop:
			return -1
		}
	}
	return -1
}
