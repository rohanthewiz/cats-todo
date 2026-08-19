# Session: spell check in the prompt editor

Session ID: `f7956889-1c49-4281-9488-f579c855d700`
Date: 2026-08-18

The ask, in one line:

> "How about spell check in the prompt editor?" … "Go ahead and build step 1
> with an embedded wordlist"

Step 1 = passive highlighting plus a toggle; no suggestions. New: package
`internal/spell/` (`spell.go`, `spell_test.go`, `en_US.txt.gz`, `extra.txt`,
`LICENSE-SCOWL.txt`), `spell.go`, `spell_test.go`, `settings.go`. Touched:
`promptsel.go`, `ui.go`, `README.md`.

## What was built

Words the dictionary does not know are underlined in red in the prompt editor.
`ctrl+l` (free in both the textarea's and textinput's keymaps; ctrl+y is DSUSP
on BSD ttys, ctrl+z suspends) toggles it from either field, says `spell check
on/off` on the form's note line, and the choice persists in a new
`~/.config/cats-todo/settings.json` (`{"spellcheck": bool}`, absent = on;
write-then-rename; corrupt/missing → defaults). Footer names it as
`ctrl+l spell` on both lines.

### The word list

- Options weighed: `/usr/share/dict/words` (macOS `web2` is headwords only —
  `cats`, `todo`, `json`, `refactor` all missing; no Windows), aspell/hunspell
  (not installed), misspell-style maps (too narrow). Chose **embed**.
- Source: SCOWL en_US size 60 as the hunspell zip from
  github.com/en-wl/wordlist releases (rel-2026.02.25; sourceforge tarball
  reset the connection). Its `en_US.aff` is only plain PFX/SFX strip/add rules,
  so a throwaway Go expander (scratchpad, not in repo) applied them to every
  stem — cross-product prefix×suffix included — and dropped `'s` forms whose
  base is listed (possessives are stripped at lookup instead). Result: 90,814
  forms, 840 KB flat, **250 KB gzipped**, `go:embed`. Binary grew ~350 KB
  (9.50 → 9.85 MB). Load ≈ 11 ms once; 5 KB check ≈ 65 µs (no cache needed).
- `extra.txt`: ~650 hand-picked dev/tooling words (`json`, `rebase`,
  `goroutine`, `worktree`, `mcp`, `catctl` …). Case-insensitive via lookup
  leniency, so one lowercase entry covers `JSON`.
- User dictionaries, one word per line, `#` comments:
  `~/.config/cats-todo/dictionary.txt` (global) and
  `<project>/.cats-todo/dictionary.txt` (project jargon, committable). Missing
  is fine; unreadable is an error → check turns off for the run with a note.

### The tokenizer (`spell.Check`) — all the judgement lives here

Skipped, not checked: backtick spans (inline closes on the same line; unclosed
= a character) and ``` fences (unclosed swallows the rest); tokens starting
with `@ - # ~ $ / . \ < &`; a trimmed core holding a digit or anything but
letters/apostrophes/hyphens (paths, URLs, snake_case, e.g., v2); any capital
after the first letter (CamelCase, ALLCAPS); non-ASCII letters; ≤2 letters.
Hyphenated compounds are checked per part. Lookup (`Known`) tries as-is,
lowercase, Title-case (proper nouns typed lowercase are style, not spelling),
`’`→`'`, and `'s` stripped. Spans are rune offsets [Start, End).

### The paint

Rides the selection overlay in `promptEditorView`. `promptSpellSpans` drops
the span the caret is inside or at the end of (`Start < caret <= End`) so a
half-typed word never flickers. `spellPaintsFor` clips spans to the display
line, sums glyph widths for cell positions, and cuts cells under the selection
out (selection wins). A line with only a selection still uses the untouched
`paintPromptSelection`; anything with marks goes through the new
`paintPromptSpans` (head raw up to the first paint, then every run re-rendered
from stripped plain text with `base`/its style, caret redrawn reversed unless
the paint is the selection). Style = `base.Foreground(colErr).Underline(true)`
so the cursor line's background survives.

## Findings worth keeping

- lipgloss v2.0.5 renders underlined text **one rune per escape sequence**
  regardless of `UnderlineSpaces` (`useSpaceStyler` is true whenever underline
  is set). Harmless on screen; tests compare against `style.Render(word)`, not
  prefix+word.
- Footer discoverability of `ctrl+l` is thin: the chords line only draws at
  bar tiers below hints (~92–99 cols wide enough to keep the tail) and the
  caret line only fits it at ≥146 cols — same fate as `ctrl+g scope`. A
  toolbar chip would fix it; left alone (preference, not action).
- `go build .` in the repo root drops a `cats-todo` binary next to `main.go`
  (bin/ is what's ignored). Removed.
- Mishap: a `cd` to a not-yet-created scratch dir failed and the following
  `printf > go.mod` ran in the repo root. Restored with `git checkout -- go.mod`
  (tree was clean), build verified. Use absolute paths / `mkdir -p` first.

## Tests

`internal/spell`: 27 tokenizer cases, offsets on a multibyte line, user files
(missing ok, directory errors), Known leniency, list size guard. Main:
settings round-trip, toggle + persistence + title-field toggle, off = textarea
view byte-for-byte, marks drawn with text/width invariants and caret intact,
caret-word suppression, marks + selection on one line (clipping both ways),
soft-wrap marks, project dictionary, footer at 95 and 160. `withSpellForm`
helper sets `CATS_TODO_CONFIG_DIR` to a temp dir so the real settings file is
never touched. All green; gofmt/vet clean.

## Left on the table

- Step 2/3: suggestions popup (edit-distance candidates, `fuzzyList` picker)
  and "add to dictionary" from the editor — `Known` and the user-file plumbing
  are ready for them.
- A toolbar chip for the toggle, if discoverability matters.
- Version bump / release not done.
