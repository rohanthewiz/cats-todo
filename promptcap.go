// promptcap.go — the editor's hard line limit, refused in words.
//
// The textarea holds at most 10,000 logical lines (bubbles textarea.go,
// the unexported maxLines). The library enforces it by cutting, silently,
// in insertRunesFromUserInput, which every road into the value goes through:
// a paste, InsertString, and SetValue (Reset, then InsertString). What gets
// cut depends on the road:
//
//	paste, one caret     the paste's tail is dropped; the text after the
//	                     caret survives, so the prompt reads as whole
//	SetValue (every      the value itself is cut at line 10,000, so the
//	line tool, carets)   prompt's own tail is lost, text never pasted
//
// Neither is announced, and a save or the autosave then writes the cut prompt
// over the stored one. So nothing here trims to fit. An edit that would go past
// the limit is refused whole, and the form note says why (contract 4, refuse in
// words). A cut paste would be a different prompt that looks like the one
// pasted, and a refusal leaves the choice of what to drop to the user.
//
// The guarded roads are the ones that can add lines: a paste (bracketed, or
// read by the Cmd+V chord), enter (one caret, or every caret), and opening a
// stored prompt that is already past the limit in the editor. The line tools
// (indent, sort, move, split) never add lines, and undo only returns to values
// the editor has held.

package main

import (
	"fmt"
	"strings"
)

// promptMaxLines mirrors the library's unexported maxLines. It cannot be read
// from the library, so a bubbles upgrade that changes it has to change this
// too. TestPromptMaxLinesMatchesTheLibrary pins the two together.
const promptMaxLines = 10000

// promptLineBreaks counts the line breaks text adds once it is in the editor.
// The library's sanitizer turns every '\r' and every '\n' into a break, so a
// "\r\n" counts twice on the one-caret road. The column mode folds "\r\n" to
// one '\n' first (insertAtCarets), so folded counts it once there. Either
// way the count is the one the road will really make.
func promptLineBreaks(text string, folded bool) int {
	if folded {
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}
	return strings.Count(text, "\n") + strings.Count(text, "\r")
}

// pasteFitsPrompt reports whether pasting text into the prompt keeps it within
// promptMaxLines. When it would not, the form note says so and nothing is
// changed, not even a standing selection the paste would have replaced.
//
// The count is the lines the value would have afterwards. That is the lines it
// has, less the breaks a selection the paste replaces takes with it, plus the
// paste's own breaks once per insertion point (every caret in the column mode
// takes the whole text, unless the lines spread one per caret, which adds
// fewer, so the count errs toward refusing).
func (m *model) pasteFitsPrompt(text string) bool {
	value := m.promptArea.Value()
	lines := strings.Count(value, "\n") + 1
	if m.carets.on {
		lines += promptLineBreaks(text, true) * len(m.carets.rows)
	} else {
		if lo, hi, ok := m.promptSelSpan(); ok {
			runes := []rune(value)
			lo, hi = min(max(lo, 0), len(runes)), min(max(hi, 0), len(runes))
			lines -= strings.Count(string(runes[lo:max(lo, hi)]), "\n")
		}
		lines += promptLineBreaks(text, false)
	}
	if lines <= promptMaxLines {
		return true
	}
	m.formNote = fmt.Sprintf("paste refused — it would make the prompt %s lines, and the editor holds at most %s",
		thousands(lines), thousands(promptMaxLines))
	return false
}

// newlinesFitPrompt is the same check for enter: n new lines (one per caret)
// on a value of rows lines. The library's own InsertNewline does not check the
// limit, but the enter here goes through SetValue, which would cut the last
// line of the prompt to make room.
func (m *model) newlinesFitPrompt(rows, n int) bool {
	if rows+n <= promptMaxLines {
		return true
	}
	m.formNote = fmt.Sprintf("no new line — the prompt is at the editor's %s-line limit", thousands(promptMaxLines))
	return false
}

// promptOverCapWhy is why a stored prompt cannot be opened in the editor, or ""
// when it can. The editor would show only its first promptMaxLines lines, and
// the first save (or autosave) would write that cut copy over the whole one.
func promptOverCapWhy(prompt string) string {
	lines := strings.Count(prompt, "\n") + strings.Count(prompt, "\r") + 1
	if lines <= promptMaxLines {
		return ""
	}
	return fmt.Sprintf("this prompt is %s lines, and the editor holds at most %s — editing it would cut the rest (ctrl+v views it whole)",
		thousands(lines), thousands(promptMaxLines))
}

// thousands writes n with comma separators, so "12345" reads as "12,345".
func thousands(n int) string {
	s := fmt.Sprint(n)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}
