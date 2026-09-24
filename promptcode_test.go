package main

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// codeTexts is the text of each span, which reads far better in a failure than
// a list of rune offsets.
func codeTexts(text string, spans []codeSpan) []string {
	runes := []rune(text)
	var out []string
	for _, sp := range spans {
		out = append(out, string(runes[sp.start:sp.end]))
	}
	return out
}

func TestPromptCodeSpans(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{"none", "plain prose, no code", nil},
		{"inline", "run `go test` then `git push`", []string{"`go test`", "`git push`"}},
		{"double ticks hold a single", "a ``x ` y`` b", []string{"``x ` y``"}},
		{"unmatched run is literal", "a `b`` c", nil},
		{"ticks pair left to right", "it`s `ok` now", []string{"`s `"}},
		{"stray run does not hide a later span", "a `` b `c` d", []string{"`c`"}},
		{"inline does not cross lines", "open `here\nand `there`", []string{"`there`"}},
		{"multibyte offsets", "é `ü` ö", []string{"`ü`"}},
		{
			"fenced block, fences included",
			"before\n```go\nx := 1\n\ny := 2\n```\nafter `z`",
			[]string{"```go", "x := 1", "y := 2", "```", "`z`"},
		},
		{"indented fence", "  ```\n  code\n  ```", []string{"  ```", "  code", "  ```"}},
		{"unclosed fence runs to the end", "a\n```\nb\nc", []string{"```", "b", "c"}},
		{
			// "```go" inside a block has an info string, so it cannot close it.
			"closing fence takes no info string",
			"```\n```go\nx\n```\ny",
			[]string{"```", "```go", "x", "```"},
		},
		{
			// A shorter run cannot close a longer fence.
			"closing fence is at least as long",
			"````\n```\n````\nz",
			[]string{"````", "```", "````"},
		},
		{"no inline spans inside a fence", "```\n`a` b\n```", []string{"```", "`a` b", "```"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := codeTexts(tc.text, promptCodeSpans(tc.text))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("promptCodeSpans(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestCutPaintHoles(t *testing.T) {
	p := promptPaint{a: 0, b: 20}
	got := cutPaintHoles(p, []promptPaint{{a: 12, b: 15}, {a: 3, b: 6}, {a: 4, b: 5}, {a: 18, b: 30}})
	var spans [][2]int
	for _, q := range got {
		spans = append(spans, [2]int{q.a, q.b})
	}
	want := [][2]int{{0, 3}, {6, 12}, {15, 18}}
	if !reflect.DeepEqual(spans, want) {
		t.Errorf("cutPaintHoles = %v, want %v", spans, want)
	}
}

// codeForm is an open form holding prompt, with spell check off so only the
// code overlay is in play.
func codeForm(t *testing.T, prompt string, width int) model {
	t.Helper()
	t.Setenv(configDirEnvVar, t.TempDir())
	m := withForm(t, "", prompt, width, 40)
	m.spellOn = false
	return m
}

// TestCodeIsDrawnInTheEditor: code spans take the code hue, prose does not, and
// the overlay changes neither the text nor the width of any line — the
// invariant every mark on this overlay is held to, since a line that grew or
// shrank would move every row below it.
func TestCodeIsDrawnInTheEditor(t *testing.T) {
	m := codeForm(t, "run `go test` now\n```\nx := 1\n```\nplain", 100)
	m.promptArea.MoveToEnd() // caret on "plain", out of the way of the spans
	view := m.promptEditorView()
	raw := m.promptArea.View()
	code := ansiOf(promptCodeStyle(m.promptArea.Styles().Focused.Text))

	for _, want := range []string{"`go test`", "x := 1", "```"} {
		if !strings.Contains(view, code+want) {
			t.Errorf("%q is not drawn as code:\n%q", want, view)
		}
	}
	for _, prose := range []string{"run", "now"} {
		if strings.Contains(view, code+prose) {
			t.Errorf("%q is drawn as code", prose)
		}
	}

	rawLines, lines := strings.Split(raw, "\n"), strings.Split(view, "\n")
	if len(rawLines) != len(lines) {
		t.Fatalf("the overlay changed the editor from %d lines to %d", len(rawLines), len(lines))
	}
	for i := range lines {
		if got, want := ansi.Strip(lines[i]), ansi.Strip(rawLines[i]); got != want {
			t.Errorf("line %d reads %q under the code hue, %q without", i, got, want)
		}
		if got, want := lipgloss.Width(lines[i]), lipgloss.Width(rawLines[i]); got != want {
			t.Errorf("line %d is %d cells under the code hue, %d without", i, got, want)
		}
	}
}

// TestCodeKeepsTheCaret: the caret inside a code span is still drawn — the
// overlay rebuilds the line from plain text, which would otherwise strip the
// library's reversed cell along with every other escape.
func TestCodeKeepsTheCaret(t *testing.T) {
	m := codeForm(t, "a `code` b", 100)
	m.promptArea.MoveToBegin()
	m.promptArea.SetCursorColumn(4) // on the 'o' of "code"
	line := strings.Split(m.promptEditorView(), "\n")[0]
	if !strings.Contains(line, ansiOf(promptCaretStyle)+"o") {
		t.Errorf("the caret inside a code span was painted over:\n%q", line)
	}
}

// TestCodeYieldsToSelectionAndSpell: where code meets the selection or a spell
// mark, the older mark wins its cells, so both look exactly as they did before
// code was coloured.
func TestCodeYieldsToSelectionAndSpell(t *testing.T) {
	m := codeForm(t, "x `teh brown` y", 100)
	base := m.promptArea.Styles().Focused.CursorLine

	// The checker skips code itself, so a real overlap can't be produced from
	// a prompt. The cut is exercised directly: an underline over "teh" takes
	// those cells out of the span, and the span keeps the rest.
	runes := []rune(m.promptArea.Value())
	dl := promptDisplayLines(m.promptArea)[0]
	gutter := promptGutterWidth(m.promptArea)
	spellRun := []promptPaint{{a: gutter + 3, b: gutter + 6, style: promptSpellStyle(base), caret: true}}
	got := codePaintsFor(promptCodeSpans(m.promptArea.Value()), dl, runes, gutter, base, false, 0, 0, spellRun)
	var cells [][2]int
	for _, p := range got {
		cells = append(cells, [2]int{p.a - gutter, p.b - gutter})
	}
	if want := [][2]int{{2, 3}, {6, 13}}; !reflect.DeepEqual(cells, want) {
		t.Errorf("code runs around an underline = %v, want %v", cells, want)
	}

	// On screen, a misspelling outside the span and the span share a line.
	m2 := codeForm(t, "teh `code` here", 100)
	m2.spellOn = true
	m2.loadSpellDict()
	if m2.spellDict == nil {
		t.Fatal("the dictionary did not load")
	}
	m2.promptArea.MoveToEnd()
	view := m2.promptEditorView()
	if !strings.Contains(view, promptSpellStyle(base).Render("teh")) {
		t.Errorf("the misspelling beside code lost its underline:\n%q", view)
	}
	if !strings.Contains(view, promptCodeStyle(base).Render("`code`")) {
		t.Errorf("the span beside a misspelling lost the code hue:\n%q", view)
	}
	if got, want := ansi.Strip(view), ansi.Strip(m2.promptArea.View()); got != want {
		t.Errorf("the marks changed the text:\n%q\nwant\n%q", got, want)
	}

	// A selection over part of the span takes those cells.
	m.spellOn = false
	m.promptArea.MoveToBegin()
	m.promptArea.SetCursorColumn(3)
	m.anchorPromptSel()
	m.promptArea.SetCursorColumn(6) // "teh" selected
	if got := m.selectedPromptText(); got != "teh" {
		t.Fatalf("selected %q, want %q", got, "teh")
	}
	view = m.promptEditorView()
	if !strings.Contains(view, promptSelStyle.Render("teh")) {
		t.Errorf("the selection inside code is not drawn as a selection:\n%q", view)
	}
	if strings.Contains(view, promptCodeStyle(base).Render("teh")) {
		t.Errorf("the selected cells are still drawn as code:\n%q", view)
	}
}

// TestCodeFollowsSoftWraps: a span longer than the editor is wide is coloured
// on every display line it is drawn across.
func TestCodeFollowsSoftWraps(t *testing.T) {
	m := codeForm(t, "`"+strings.TrimSpace(strings.Repeat("wrapme ", 8))+"`", 30)
	m.promptArea.MoveToBegin()
	st := m.promptArea.Styles().Focused
	coded := 0
	for _, ln := range strings.Split(m.promptEditorView(), "\n") {
		if strings.Contains(ln, ansiOf(promptCodeStyle(st.Text))+"wrapme") ||
			strings.Contains(ln, ansiOf(promptCodeStyle(st.CursorLine))+"wrapme") {
			coded++
		}
	}
	if coded < 2 {
		t.Errorf("only %d display lines carry the code hue, want every wrapped one:\n%s", coded, m.promptEditorView())
	}
}

// TestNoCodeLeavesTheEditorAlone: a prompt with no code, no selection and no
// spell marks is the textarea's own view, byte for byte.
func TestNoCodeLeavesTheEditorAlone(t *testing.T) {
	m := codeForm(t, "nothing to see here", 100)
	if got, want := m.promptEditorView(), m.promptArea.View(); got != want {
		t.Error("with no code the editor view differs from the textarea's own")
	}
}

// TestCodeIsDrawnInTheView: the read-only view colours code, and leaves the
// session and attachment lines it appends out of an unclosed fence.
func TestCodeIsDrawnInTheView(t *testing.T) {
	m, _, _ := newModelInTemp(t)
	m.width, m.height = 100, 40
	m.viewRef = todoRef{scope: scopeProject}
	td := Todo{
		Prompt:  "run `go test`\n```\nunclosed",
		Session: &SessionOpts{Model: "opus"},
	}
	out := m.viewContent(td)
	code := ansiOf(viewCodeStyle)
	for _, want := range []string{"`go test`", "```", "unclosed"} {
		if !strings.Contains(out, code+want) {
			t.Errorf("%q is not drawn as code in the view:\n%q", want, out)
		}
	}
	if strings.Contains(out, code+"⚙") {
		t.Errorf("the session line was drawn as part of the open fence:\n%q", out)
	}
	if !strings.Contains(ansi.Strip(out), "⚙ session:") {
		t.Errorf("the session line is missing:\n%q", out)
	}
}

// TestStyleCodeSpansSurvivesWrap pins the lipgloss behaviour viewContent relies
// on: a styled span that the wrap breaks is closed at the end of each line and
// reopened at the start of the next, so every line the viewport shows is
// complete on its own. If an upgrade stops doing this, a scrolled view would
// show a wrapped span's later lines as prose.
func TestStyleCodeSpansSurvivesWrap(t *testing.T) {
	text := "see `some code that is long enough to wrap across lines` ok"
	styled := styleCodeSpans(text, promptCodeSpans(text), viewCodeStyle)
	out := lipgloss.NewStyle().Width(20).Render(styled)
	code := ansiOf(viewCodeStyle)
	lines := strings.Split(out, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected the span to wrap, got %d lines: %q", len(lines), out)
	}
	for i, ln := range lines {
		if !strings.Contains(ln, "code") && !strings.Contains(ln, "enough") && !strings.Contains(ln, "wrap") {
			continue
		}
		if !strings.Contains(ln, code) {
			t.Errorf("wrapped line %d does not reopen the code style: %q", i, ln)
		}
	}
	if got := ansi.Strip(out); !strings.Contains(strings.Join(strings.Fields(got), " "), strings.Join(strings.Fields(text), " ")) {
		t.Errorf("styling changed the text: %q", got)
	}
}
