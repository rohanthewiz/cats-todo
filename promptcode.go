// promptcode.go — finding code in a prompt, and colouring it in the editor and
// the read-only view.
//
// Prompts are Markdown by habit — they are written for an agent that reads
// Markdown — and the code in them is what a reader most wants to pick out:
// a command to run, a flag, a file name. Two forms count, the two a prompt
// actually uses:
//
//	inline:  run `go test ./...` before committing
//	              └──── span ────┘   (the backticks are part of it)
//
//	fenced:  ```go              ┐
//	         x := 1             │ every line, both fences included
//	         ```                ┘
//
// The parser returns rune spans, one per line of code, and the two screens
// that draw a prompt paint those spans in their own way: the editor through
// the overlay that already carries the selection and the spell marks
// (promptEditorView), the view by styling the text before it is wrapped
// (viewContent). One span per line rather than one per block is what lets
// both consumers work line by line without splitting anything themselves.

package main

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// codeSpan is a run of code as the half-open rune range [start, end) into the
// prompt. It never crosses a line break, so it never contains a '\n'.
type codeSpan struct {
	start, end int
}

// codeFenceTick is the only fence character recognised. Markdown also allows
// "~~~", but prompts here do not use it, and every extra rule is one more way
// for ordinary text to turn blue unexpectedly.
const codeFenceTick = '`'

// promptCodeSpans finds the code in text: every line of every fenced block, and
// every inline span on the lines outside them.
//
// The rules follow CommonMark where it matters and stay simpler where it
// doesn't:
//
//   - A fence is a line that, after its indentation, starts with three or more
//     backticks. It closes on the next line that starts with at least as many
//     backticks and has nothing but whitespace after them. The opening fence's
//     info string ("```go") is allowed; a closing fence's is not, which is the
//     rule that keeps "```go" inside a block from ending it.
//   - A fence that is never closed runs to the end of the prompt, as it does in
//     Markdown. While a block is being typed that colours everything below the
//     opening fence until the closing one goes in. Every Markdown editor does
//     this, and the text shows what the agent will actually receive.
//   - An inline span opens on a run of N backticks and closes on the next run of
//     exactly N. A run with no partner is plain text. Unlike CommonMark, a span
//     does not cross a line break: an unmatched backtick typed halfway through
//     a prompt would otherwise colour lines further down, and prompts put code
//     spans on one line anyway.
func promptCodeSpans(text string) []codeSpan {
	if !strings.ContainsRune(text, codeFenceTick) {
		return nil // fast path: most prompts have no code at all
	}
	var (
		spans []codeSpan
		off   int // rune offset of the current line's first rune
		fence int // backtick count of the open fence; 0 = not inside one
	)
	for line := range strings.SplitSeq(text, "\n") {
		runes := []rune(line)
		n := len(runes)
		switch ticks, rest := fenceTicks(line); {
		case fence == 0 && ticks >= 3:
			// Opening fence. Its info string is allowed, so rest is not checked.
			fence = ticks
			spans = appendCodeSpan(spans, off, off+n)
		case fence > 0:
			// Inside a block, both fences included: the whole line is code.
			spans = appendCodeSpan(spans, off, off+n)
			if ticks >= fence && strings.TrimSpace(rest) == "" {
				fence = 0 // closing fence
			}
		default:
			for _, sp := range inlineCodeSpans(runes) {
				spans = append(spans, codeSpan{off + sp.start, off + sp.end})
			}
		}
		off += n + 1 // +1 for the '\n' Split took out
	}
	return spans
}

// fenceTicks reports how many backticks line starts with after its indentation,
// and what follows them. A line that does not start with a backtick gives 0.
func fenceTicks(line string) (ticks int, rest string) {
	s := strings.TrimLeft(line, " \t")
	for ticks < len(s) && s[ticks] == codeFenceTick {
		ticks++
	}
	return ticks, s[ticks:]
}

// appendCodeSpan adds [start, end) unless it is empty. An empty line inside a
// fenced block has nothing to paint, and a zero-width span would only give the
// paint code a case to guard against.
func appendCodeSpan(spans []codeSpan, start, end int) []codeSpan {
	if end <= start {
		return spans
	}
	return append(spans, codeSpan{start, end})
}

// inlineCodeSpans finds the inline code spans on one line, as rune offsets into
// it. Each span includes its backticks.
//
// A run of backticks is taken whole. That is how CommonMark lets code contain a
// backtick (``a ` b``), and it is also why a run is not matched against a
// longer or shorter one:
//
//	``a ` b``      → one span: the single tick inside is not a delimiter
//	`a`` b         → no span: the only other run is two ticks, not one
//
// A run with no partner is skipped as literal text, and the scan carries on
// after it, so a stray tick early on a line does not hide a real span later.
func inlineCodeSpans(runes []rune) []codeSpan {
	var out []codeSpan
	runAt := func(i int) int { // length of the backtick run starting at i
		j := i
		for j < len(runes) && runes[j] == codeFenceTick {
			j++
		}
		return j - i
	}
	for i := 0; i < len(runes); {
		if runes[i] != codeFenceTick {
			i++
			continue
		}
		n := runAt(i)
		closed := false
		for j := i + n; j < len(runes); {
			if runes[j] != codeFenceTick {
				j++
				continue
			}
			m := runAt(j)
			if m == n {
				out = append(out, codeSpan{i, j + m})
				i = j + m
				closed = true
				break
			}
			j += m // a run of another length: part of the code, not its end
		}
		if !closed {
			i += n
		}
	}
	return out
}

// promptCodeStyle is how code is drawn in the editor, derived from the style
// the editor drew that line in, for the same reason promptSpellStyle is: the
// cursor line carries a background, and a code span that dropped it would punch
// a hole in the line's field. Only the foreground changes.
//
// colInfo because it is the one cool hue in a warm green palette, so code
// separates from prose by hue alone, with no weight or background change to
// make the line jump as a span opens and closes under the caret. Amber
// (colWarn) is off limits: it belongs to the fuzzy-match highlight.
func promptCodeStyle(base lipgloss.Style) lipgloss.Style {
	return base.Foreground(lipgloss.Color(colInfo))
}

// codePaintsFor turns the code spans into cell runs for one display line of the
// editor, in the form paintPromptSpans takes. The walk is spellPaintsFor's:
// only the spans on this line, clipped to it, rune offsets turned into cells by
// summing widths.
//
// Two other marks win where they overlap, so their cells are cut out of the
// code runs:
//
//   - the selection [selA, selB), for the reason it wins over a spell mark: a
//     highlight with coloured letters showing through reads as two things;
//   - the spell marks already on the line (spell). The checker skips code by
//     its own reading of the backticks (internal/spell's Check), so the two
//     rarely meet. Rarely is not never, because the readings differ at the
//     edges: the checker takes a ``` anywhere on a line as a fence, and this
//     file only at a line's start. Where they disagree the underline keeps
//     its cells, because paintPromptSpans needs runs that do not overlap
//     and the checker should draw exactly as it did before code had a colour.
//
// caret is true on every run: the caret sits inside code all the time, and the
// library's reversed cell has to be redrawn there, as it is for a spell mark.
func codePaintsFor(spans []codeSpan, dl promptDisplayLine, runes []rune, gutter int, base lipgloss.Style, hasSel bool, selA, selB int, spell []promptPaint) []promptPaint {
	var out []promptPaint
	style := promptCodeStyle(base)
	holes := spell
	if hasSel {
		holes = append(append([]promptPaint(nil), spell...), promptPaint{a: selA, b: selB})
	}
	for _, sp := range spans {
		s, e := max(sp.start, dl.start), min(sp.end, dl.end())
		if s >= e {
			continue
		}
		a := gutter + lipgloss.Width(string(runes[dl.start:s]))
		b := gutter + lipgloss.Width(string(runes[dl.start:e]))
		out = append(out, cutPaintHoles(promptPaint{a: a, b: b, style: style, caret: true}, holes)...)
	}
	return out
}

// cutPaintHoles returns what is left of p once every hole's cells are taken out
// of it: none, one, or several pieces, in left-to-right order. holes need not
// be sorted and may overlap each other.
//
//	p:      ├──────────────────────┤
//	holes:      ├──┤       ├───┤
//	out:    ├──┤    ├─────┤     ├──┤
func cutPaintHoles(p promptPaint, holes []promptPaint) []promptPaint {
	pieces := []promptPaint{p}
	for _, h := range holes {
		var next []promptPaint
		for _, q := range pieces {
			if h.b <= q.a || h.a >= q.b {
				next = append(next, q) // no overlap
				continue
			}
			if q.a < h.a {
				next = append(next, promptPaint{a: q.a, b: h.a, style: q.style, caret: q.caret})
			}
			if h.b < q.b {
				next = append(next, promptPaint{a: h.b, b: q.b, style: q.style, caret: q.caret})
			}
		}
		pieces = next
	}
	return pieces
}

// viewCodeStyle is how code is drawn in the read-only view. It is the editor's
// hue on the view's plain field, so a span looks the same in both places.
var viewCodeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colInfo))

// styleCodeSpans returns text with each code span rendered in style and
// everything else left as it is. It is for the view, which styles the prompt
// before wrapping it.
//
// Styling before the wrap works because lipgloss v2 closes an open style at
// each break it makes and opens it again on the next line
// (TestStyleCodeSpansSurvivesWrap pins this), so every wrapped line is complete
// on its own and the viewport can scroll to any of them. Spans never contain a
// '\n', so each Render call is one run on one line.
func styleCodeSpans(text string, spans []codeSpan, style lipgloss.Style) string {
	if len(spans) == 0 {
		return text
	}
	runes := []rune(text)
	var b strings.Builder
	pos := 0
	for _, sp := range spans {
		b.WriteString(string(runes[pos:sp.start]))
		b.WriteString(style.Render(string(runes[sp.start:sp.end])))
		pos = sp.end
	}
	b.WriteString(string(runes[pos:]))
	return b.String()
}
