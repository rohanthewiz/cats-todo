// value.go — how much a prompt pays: the value annotation's levels.
//
// Value was one bit, the 💎 "high value" gem. It is now one of three levels —
// low, medium, high — the scale the Next List file rates its items on, so a
// prompt made from a next-list item and a prompt written by hand answer the
// question in one vocabulary and wear one set of marks:
//
//	🔷  high     a blue diamond — the emoji, so it paints itself
//	◆   medium   a solid diamond in straw
//	◇   low      the diamond's outline; the default, drawn only as a legend
//
// **Low is the default.** A prompt nobody has rated is a low-value prompt, so
// there is no fourth "none" level: an unrated prompt and a low one are the
// same fact, and a scale that told them apart would ask the user a question
// with no answer. It follows the priority rule (priorityNone) the rest of the
// way too: the default is stored as nothing and drawn as nothing on a row, so
// an unrated backlog costs neither bytes nor cells. The ◇ is still the level's
// glyph — the form's radio and the menu row wear it — because a control has to
// show what choosing it means, where a row only has to show what was raised.
//
// One shape, stepping up by fill and colour, so the three are read as one
// scale rather than learned as three unrelated marks. The high step is the
// only emoji — the font draws it, big and saturated, which is the loudness the
// top of the scale should have — and the lower two are text so the palette
// reaches them (see valueMarkFor).
//
// Value is orthogonal to priority, as the gem always was: priority says how
// much a prompt matters *now*, value how much it pays whenever it is done. A
// refactor that pays forever but can wait is high value and not critical.
//
// Storage keeps the gem's key. High is still written as `highValue: true`,
// exactly as before; medium is the one level that uses the new `value` key
// (see Todo.Value); low, the default, writes neither. So a backlog nobody
// re-marks stays byte-identical, and an older binary still sees every
// high-value prompt it saw before; it ignores medium, which it has no mark for.
package main

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

// The value levels, as annots.Value holds them. Low is the empty string: the
// default, "nothing raised", the same turn priorityNone makes, so the zero
// annots is unmarked and an unrated prompt reads as low without a migration.
const (
	valueLow    = ""
	valueMedium = "medium"
	valueHigh   = "high"
)

// valueLevels is the levels in the order they escalate. The form's annotation
// bar and the list's context menu lay their radios out in this order, the same
// walk prioValues gives the priority radios.
var valueLevels = []string{valueLow, valueMedium, valueHigh}

// valueLevel reads a todo's value. The gem's old key wins, since it is the one
// every version has written; a hand-edited `"value": "high"` is honoured too,
// and the next save folds it onto the old key (setValueLevel). Anything else
// the file holds — "low" spelled out, or a word this program does not know —
// reads as the default, low.
func (t Todo) valueLevel() string {
	if t.HighValue {
		return valueHigh
	}
	switch t.Value {
	case valueMedium, valueHigh:
		return t.Value
	}
	return valueLow
}

// setValueLevel writes a level onto a todo's two storage fields: high as the
// gem's key, medium as the new one, and low as neither. At most one of them is
// ever set, so the file cannot say two things.
func (t *Todo) setValueLevel(v string) {
	t.HighValue, t.Value = false, ""
	switch v {
	case valueHigh:
		t.HighValue = true
	case valueMedium:
		t.Value = v
	}
}

// normalizeValue folds what a person would type onto a level and refuses the
// rest, for `--value` (cli.go). The set is short and closed, like priority's.
// "med" and "hi"/"lo" fold because they are how the words get abbreviated on a
// command line; "none" folds onto low, since an unrated prompt is a low one.
func normalizeValue(s string) (string, error) {
	switch foldOption(s) {
	case "", "low", "lo", "none":
		return valueLow, nil
	case "medium", "med", "mid":
		return valueMedium, nil
	case "high", "hi":
		return valueHigh, nil
	}
	return "", fmt.Errorf("value %q is not one of high, medium, low", s)
}

// valueLevelLabel is the level in words, wherever it is spelled out: the
// prompt view, the CLI's echo, a bundle's markdown. "medium value" rather than
// "valuable" so it reads as one of a pair with "low-hanging fruit" — the cost
// and the payoff of one estimate. Empty for low, the default: the screens that
// print the marks as words say only what was raised, as they do for priority.
func valueLevelLabel(v string) string {
	if v == valueLow {
		return ""
	}
	return v + " value"
}

// valueWord is the level's name for a control — the radio, the menu row, the
// status note — where the default has to be named, since it is one of the
// choices.
func valueWord(v string) string {
	if v == valueLow {
		return "low"
	}
	return v
}

// The glyphs, one shape stepping down. See the file comment for why only the
// top step is an emoji. The diamonds are East Asian Ambiguous, one cell like
// the priority triangles; the emoji is two, like the apple beside it.
const (
	valueHighGlyph   = "🔷"
	valueMediumGlyph = "◆"
	valueLowGlyph    = "◇"
)

// valueMarkFor is a level's glyph and the style to draw it in. It is the one
// definition the backlog's rows, the form's bar, the context menu and the Next
// List page all draw from, so a level looks the same wherever it is shown.
// The emoji gets a bare style: a foreground never reaches it.
//
// Low has a glyph here even though no row draws it (valueMark, nextValueMark):
// the controls need it, to show what choosing the default means.
func valueMarkFor(v string) (string, lipgloss.Style) {
	switch v {
	case valueHigh:
		return valueHighGlyph, lipgloss.NewStyle()
	case valueMedium:
		return valueMediumGlyph, lipgloss.NewStyle().Foreground(lipgloss.Color(colStraw))
	case valueLow:
		return valueLowGlyph, lipgloss.NewStyle().Foreground(lipgloss.Color(colDim))
	}
	return "", lipgloss.NewStyle()
}

// valueChosenStyle is how a chosen value segment is drawn on the form's bar,
// the words as well as the glyph. The chosen level takes its own mark's hue —
// the rule annotSegStyle keeps for priority — except that high has no text
// hue of its own (its mark is an emoji), so it takes colInfo, the blue its
// diamond is drawn in. Low is bright rather than its dim grey, because a
// chosen segment in the grey of the unchosen ones would read as not chosen.
func valueChosenStyle(v string) lipgloss.Style {
	switch v {
	case valueHigh:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colInfo)).Bold(true)
	case valueMedium:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colStraw)).Bold(true)
	}
	return nameSelStyle
}

// valueNote is the status line after a level is set from the context menu.
// It names the level and what it claims, as priorityNote does.
func valueNote(v string) string {
	switch v {
	case valueHigh:
		return "value high — a large payoff"
	case valueMedium:
		return "value medium — worth doing"
	}
	return "value low — the default"
}
