package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestNormalizeOptions covers the folding every entry point shares: the CLI and
// the TUI both go through these, so a value accepted in one is accepted in the
// other and a bad one fails with the same words.
func TestNormalizeOptions(t *testing.T) {
	t.Run("model passes through", func(t *testing.T) {
		for _, in := range []string{"sonnet", "opus", "claude-opus-5", "  haiku  "} {
			got, err := normalizeModel(in)
			if err != nil {
				t.Errorf("normalizeModel(%q) = %v, want it accepted", in, err)
			}
			if got != strings.TrimSpace(in) {
				t.Errorf("normalizeModel(%q) = %q, want the id unchanged", in, got)
			}
		}
		// Unknown ids are deliberately allowed: new model names ship faster than
		// this binary does.
		if got, err := normalizeModel("claude-opus-9"); err != nil || got != "claude-opus-9" {
			t.Errorf("normalizeModel of a future id = %q, %v — want it passed through", got, err)
		}
		if _, err := normalizeModel("two words"); err == nil {
			t.Error("normalizeModel accepted a value with a space — it would become two argv entries")
		}
		if _, err := normalizeModel("--effort"); err == nil {
			t.Error("normalizeModel accepted a flag-shaped value")
		}
	})

	t.Run("effort is a closed set", func(t *testing.T) {
		for in, want := range map[string]string{
			"":       "",
			"low":    effortLow,
			"MEDIUM": effortMedium,
			"med":    effortMedium,
			"high":   effortHigh,
			"x-high": effortXHigh,
			"xhigh":  effortXHigh,
			"max":    effortMax,
		} {
			got, err := normalizeEffort(in)
			if err != nil || got != want {
				t.Errorf("normalizeEffort(%q) = %q, %v — want %q", in, got, err, want)
			}
		}
		if _, err := normalizeEffort("turbo"); err == nil {
			t.Error("normalizeEffort accepted a value claude would reject at launch")
		}
	})

	t.Run("permission folds onto claude's spelling", func(t *testing.T) {
		for in, want := range map[string]string{
			"":                  "",
			"accept-edits":      permAcceptEdits,
			"acceptEdits":       permAcceptEdits,
			"accept_edits":      permAcceptEdits,
			"accept":            permAcceptEdits,
			"bypass":            permBypass,
			"bypassPermissions": permBypass,
			"dont-ask":          permDontAsk,
			"plan":              permPlan,
			"manual":            permManual,
			"auto":              permAuto,
		} {
			got, err := normalizePermission(in)
			if err != nil || got != want {
				t.Errorf("normalizePermission(%q) = %q, %v — want %q", in, got, err, want)
			}
		}
		if _, err := normalizePermission("yolo"); err == nil {
			t.Error("normalizePermission accepted an unknown mode")
		}
	})

	t.Run("finish and review", func(t *testing.T) {
		for in, want := range map[string]string{
			"": finishNone, "none": finishNone, "commit": finishCommit,
			"push": finishPush, "wrap": finishWrap, "sess-wrap": finishWrap,
		} {
			got, err := normalizeFinish(in)
			if err != nil || got != want {
				t.Errorf("normalizeFinish(%q) = %q, %v — want %q", in, got, err, want)
			}
		}
		if _, err := normalizeFinish("exit"); err == nil {
			t.Error("normalizeFinish accepted a wrap-up it does not offer")
		}
		// The slash is how a skill is invoked, not what it is called.
		if got, _ := normalizeReview("/code-review"); got != reviewCode {
			t.Errorf("normalizeReview(%q) = %q, want the leading slash off", "/code-review", got)
		}
		// Any skill name passes — which ones exist depends on the machine the
		// prompt lands on, not on this one.
		if got, err := normalizeReview("house-style"); err != nil || got != "house-style" {
			t.Errorf("normalizeReview of an unknown skill = %q, %v — want it accepted", got, err)
		}
	})
}

// TestLaunchArgs pins what rides on a new session's argv: claude's three flags,
// in claude's spelling, and nothing at all for any other agent.
func TestLaunchArgs(t *testing.T) {
	full := &SessionOpts{Model: "sonnet", Effort: effortHigh, Permission: permAcceptEdits}
	want := []string{"--model", "sonnet", "--effort", "high", "--permission-mode", "acceptEdits"}
	if got := full.launchArgs("claude"); !slices.Equal(got, want) {
		t.Errorf("launchArgs(claude) = %v, want %v", got, want)
	}
	// A path is still claude — cats reports absolute commands for panes.
	if got := full.launchArgs("/usr/local/bin/claude"); !slices.Equal(got, want) {
		t.Errorf("launchArgs of an absolute claude = %v, want %v", got, want)
	}
	// Any other agent: the flags are dropped and the prompt still goes. The
	// picker says so on the row before the choice is made.
	if got := full.launchArgs("copilot"); len(got) != 0 {
		t.Errorf("launchArgs(copilot) = %v, want none — they are claude's flags", got)
	}
	if got := (&SessionOpts{}).launchArgs("claude"); len(got) != 0 {
		t.Errorf("launchArgs of empty options = %v, want none", got)
	}
	var nilOpts *SessionOpts
	if got := nilOpts.launchArgs("claude"); len(got) != 0 {
		t.Errorf("launchArgs of nil options = %v, want none", got)
	}
	if !full.hasLaunchFlags() || (&SessionOpts{Finish: finishWrap}).hasLaunchFlags() {
		t.Error("hasLaunchFlags does not match which options ride on the argv")
	}
	// Only a claude session gets them, which is what the picker's note is keyed
	// off — the check has to see through a command carrying its own flags.
	if !isClaudeCommand("claude --resume") || isClaudeCommand("copilot") || isClaudeCommand("") {
		t.Error("isClaudeCommand disagrees with what the argv will exec")
	}
}

// TestComposePromptWithOptions walks the matrix of blocks present and absent.
// The image block has to stay between the body and the wrap-up — a "when the
// work is done" instruction only reads as the last word if nothing follows it —
// and the whole thing still has to end without a newline.
func TestComposePromptWithOptions(t *testing.T) {
	body := "fix the sidebar"
	imgs := []string{"/tmp/a.png"}
	imgBlock := imageBlockHeader + "\n/tmp/a.png"

	for _, tc := range []struct {
		name string
		opts *SessionOpts
		imgs []string
		want string
	}{
		{
			name: "nil options change nothing",
			opts: nil,
			want: body,
		},
		{
			name: "empty options change nothing",
			opts: &SessionOpts{},
			imgs: imgs,
			want: body + "\n\n" + imgBlock,
		},
		{
			name: "launch flags are not text",
			opts: &SessionOpts{Model: "sonnet", Effort: effortLow, Permission: permAcceptEdits, Clear: true},
			want: body,
		},
		{
			name: "preamble only",
			opts: &SessionOpts{Context: ctxLoad, ContextArg: "2"},
			want: "First, load prior context: run /sess-load 2\n\n" + body,
		},
		{
			name: "a bare sess-load has a default of its own",
			opts: &SessionOpts{Context: ctxLoad},
			want: "First, load prior context: run /sess-load\n\n" + body,
		},
		{
			name: "a patternless sess-use is not sent",
			opts: &SessionOpts{Context: ctxUse},
			want: body,
		},
		{
			name: "files join the preamble",
			opts: &SessionOpts{Context: ctxUse, ContextArg: "drops", Files: []string{"ai_docs/design.md", "README.md"}},
			want: "First, load prior context: run /sess-use drops\n" +
				"Also read these files: ai_docs/design.md, README.md\n\n" + body,
		},
		{
			name: "postamble only",
			opts: &SessionOpts{Finish: finishCommit},
			want: body + "\n\nWhen the work is done and the tests pass:\n- commit the work",
		},
		{
			name: "reviews lead the wrap-up, the release closes it",
			opts: &SessionOpts{Reviews: []string{reviewCode, reviewSecurity}, Finish: finishWrap, Release: true},
			want: body + "\n\nWhen the work is done and the tests pass:\n" +
				"- run /code-review\n- run /security-review\n" +
				"- run /sess-wrap (saves a session doc, commits, and pushes)\n" +
				"- " + releaseLine,
		},
		{
			name: "everything at once, with the images in the middle",
			opts: &SessionOpts{Context: ctxLoad, ContextArg: "2", Finish: finishPush},
			imgs: imgs,
			want: "First, load prior context: run /sess-load 2\n\n" + body + "\n\n" + imgBlock +
				"\n\nWhen the work is done and the tests pass:\n- commit and push",
		},
		{
			name: "an empty body leaves no blank lines behind it",
			opts: &SessionOpts{Finish: finishCommit},
			want: "When the work is done and the tests pass:\n- commit the work",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := body
			if tc.name == "an empty body leaves no blank lines behind it" {
				in = ""
			}
			got := composePrompt(in, tc.imgs, tc.opts)
			if got != tc.want {
				t.Errorf("composePrompt =\n%q\nwant\n%q", got, tc.want)
			}
			if strings.HasSuffix(got, "\n") {
				t.Errorf("composePrompt = %q, must not end in a newline (sendInput's contract)", got)
			}
		})
	}
}

// TestSessionSummary covers the one line the form, the list and the prompt view
// all show — including that an unconfigured record has nothing to say.
func TestSessionSummary(t *testing.T) {
	var nilOpts *SessionOpts
	if nilOpts.summary() != "" || (&SessionOpts{}).summary() != "" {
		t.Error("an unset record produced a summary — the form would show options nobody chose")
	}
	if nilOpts.configured() || (&SessionOpts{}).configured() {
		t.Error("an unset record reported itself configured")
	}
	o := &SessionOpts{
		Model: "sonnet", Effort: effortHigh, Permission: permAcceptEdits,
		Context: ctxLoad, ContextArg: "2", Finish: finishWrap,
	}
	if got, want := o.summary(), "sonnet · high · acceptEdits · sess-load 2 · wrap"; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
	if !o.configured() {
		t.Error("a record with options reported itself unconfigured")
	}
	// sessionPtr is the gate on the JSON: nothing set, nothing written.
	if sessionPtr(SessionOpts{}) != nil {
		t.Error("sessionPtr of an empty record returned a pointer — the backlog would grow a session key for every todo")
	}
	if sessionPtr(*o) == nil {
		t.Error("sessionPtr dropped a configured record")
	}
}

// TestSessionClone is the guard on the form's "cancelling changes nothing"
// contract: a plain struct copy shares the slice headers, so an editor
// appending to Files would be appending into the stored todo's own list.
func TestSessionClone(t *testing.T) {
	orig := SessionOpts{Files: []string{"a.md"}, Reviews: []string{reviewCode}}
	c := orig.clone()
	c.Files = append(c.Files, "b.md")
	c.Reviews[0] = reviewSimplify
	if len(orig.Files) != 1 || orig.Reviews[0] != reviewCode {
		t.Errorf("editing the clone reached the original: %+v", orig)
	}
}

// TestCycleValue covers the panel's ←/→, including the case that matters most:
// a value set somewhere this ring does not offer — a model alias typed at the
// CLI — must not be thrown away by cycling past it.
func TestCycleValue(t *testing.T) {
	vals := []string{"", "a", "b"}
	if got := cycleValue(vals, "", 1); got != "a" {
		t.Errorf("cycle forward from the default = %q, want %q", got, "a")
	}
	if got := cycleValue(vals, "", -1); got != "b" {
		t.Errorf("cycle back from the default = %q, want the last value %q", got, "b")
	}
	if got := cycleValue(vals, "b", 1); got != "" {
		t.Errorf("cycle past the end = %q, want it wrapped to the default", got)
	}
	if got := cycleValue(vals, "claude-opus-5", 1); got != "" {
		t.Errorf("cycling from an off-ring value = %q, want it to join the ring and step to %q", got, "")
	}
	if got := cycleValue(vals, "claude-opus-5", -1); got != "b" {
		t.Errorf("cycling back from an off-ring value = %q, want %q", got, "b")
	}
}

// TestCycleSessReviews pins the review row's presets: it steps through the sets
// and a combination built with repeated --review flags lands somewhere sane
// rather than sticking.
func TestCycleSessReviews(t *testing.T) {
	if got := cycleSessReviews(nil, 1); !slices.Equal(got, []string{reviewCode}) {
		t.Errorf("first step = %v, want the code review alone", got)
	}
	if got := cycleSessReviews(nil, -1); !slices.Equal(got, sessReviewValues[len(sessReviewValues)-1]) {
		t.Errorf("step back from none = %v, want the last preset", got)
	}
	if got := cycleSessReviews([]string{"house-style"}, 1); len(got) != 0 {
		t.Errorf("stepping from an off-ring set = %v, want the ring's first entry", got)
	}
}

// TestSessCommandDirs checks where the sess-* gate looks. The override matters:
// it is the only one of the three a test — or a user with a relocated config —
// can point somewhere real.
func TestSessCommandDirs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	want := filepath.Join(dir, sessCommandDir)
	if !slices.Contains(sessCommandDirs(), want) {
		t.Errorf("sessCommandDirs() = %v, want it to include %q", sessCommandDirs(), want)
	}
	// Commands, not skills: they are different directories and only one of them
	// holds /sess-load.
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(want, sessProbeCommand), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !sessSkillsAvailable() {
		t.Error("sessSkillsAvailable missed a command installed in the configured dir")
	}
}

// TestExpandSessLoad covers the one flag whose argument is optional. The bug
// this rewrite exists for was worse than a misplaced count: the flag package
// stops parsing at the first non-flag argument, so `--sess-load 2 --review x`
// put the count *and every later flag* into the prompt text.
func TestExpandSessLoad(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "the count is joined and the later flags survive",
			in:   []string{"--sess-load", "2", "--review", "code-review", "fix", "it"},
			want: []string{"--sess-load=2", "--review", "code-review", "fix", "it"},
		},
		{
			name: "a bare flag is left bare",
			in:   []string{"--sess-load", "fix", "it"},
			want: []string{"--sess-load", "fix", "it"},
		},
		{
			name: "the single-dash spelling too",
			in:   []string{"-sess-load", "3"},
			want: []string{"-sess-load=3"},
		},
		{
			name: "an explicit value is not touched",
			in:   []string{"--sess-load=3", "2", "fix"},
			want: []string{"--sess-load=3", "2", "fix"},
		},
		{
			name: "past the terminator the number is the user's word",
			in:   []string{"--", "--sess-load", "2"},
			want: []string{"--", "--sess-load", "2"},
		},
		{
			name: "a number after any other flag is that flag's business",
			in:   []string{"--effort", "low", "2", "fix"},
			want: []string{"--effort", "low", "2", "fix"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := expandSessLoad(tc.in); !slices.Equal(got, tc.want) {
				t.Errorf("expandSessLoad(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// --- The panel -----------------------------------------------------------------

// openSessionPanel puts a model in the session panel over a fresh add form, the
// way ctrl+r does.
func openSessionPanel(t *testing.T) (model, *store) {
	t.Helper()
	m, project, _ := newModelInTemp(t)
	next, _ := m.beginAdd()
	m = next.(model)
	next, _ = m.updateForm(pressKey("ctrl+r"))
	m = next.(model)
	if m.stage != stageSession {
		t.Fatalf("ctrl+r from the form left stage = %v, want stageSession", m.stage)
	}
	return m, project
}

// stepSession sends one key to the session panel.
func stepSession(t *testing.T, m model, key string) model {
	t.Helper()
	next, _ := m.updateSession(pressKey(key))
	return next.(model)
}

// TestSessionPanelCycles drives the panel the way a user does: walk to a row,
// cycle it, and find the record changed.
func TestSessionPanelCycles(t *testing.T) {
	m, _ := openSessionPanel(t)
	// The annotations lead the panel — the rows here that describe the prompt
	// rather than the session that will read it (see annotations.go).
	if m.sessCursor != sessRowPriority {
		t.Fatalf("the panel opened on row %d, want Priority (%d) first", m.sessCursor, sessRowPriority)
	}

	// Down past both annotation rows to the first launch flag, rather than a
	// fixed number of presses: a row added above Model must not silently retarget
	// the rest of this walk.
	for m.sessCursor < sessRowModel {
		m = stepSession(t, m, "down")
	}
	m = stepSession(t, m, "right")
	if m.formSession.Model != sessModelValues[1] {
		t.Errorf("right on the model row = %q, want %q", m.formSession.Model, sessModelValues[1])
	}
	m = stepSession(t, m, "left")
	if m.formSession.Model != "" {
		t.Errorf("left back = %q, want the default", m.formSession.Model)
	}

	m = stepSession(t, m, "down")
	m = stepSession(t, m, "right")
	if m.sessCursor != sessRowEffort || m.formSession.Effort != effortLow {
		t.Errorf("cursor=%d effort=%q, want the effort row set to %q", m.sessCursor, m.formSession.Effort, effortLow)
	}

	// space is the third way to change a row, and the only one that reads as
	// "toggle" on the two boolean rows.
	for m.sessCursor < sessRowClear {
		m = stepSession(t, m, "down")
	}
	m = stepSession(t, m, "space")
	if !m.formSession.Clear {
		t.Error("space on the clear row did not set it")
	}
}

// TestSessionPanelContextArg covers the two rows that take typed text: the
// argument commits as the cursor leaves it, and switching the context mode drops
// an argument that belonged to the other one.
func TestSessionPanelContextArg(t *testing.T) {
	m, _ := openSessionPanel(t)
	for m.sessCursor < sessRowContext {
		m = stepSession(t, m, "down")
	}
	m = stepSession(t, m, "right")
	if m.formSession.Context != ctxLoad {
		t.Fatalf("context row = %q, want %q", m.formSession.Context, ctxLoad)
	}

	m = stepSession(t, m, "down")
	if m.sessCursor != sessRowContextArg || !m.sessInput.Focused() {
		t.Fatalf("cursor=%d focused=%v — the text row must hold the keys", m.sessCursor, m.sessInput.Focused())
	}
	m.sessInput.SetValue("2")
	// The value commits when the cursor leaves, so no explicit save gesture.
	m = stepSession(t, m, "up")
	if m.formSession.ContextArg != "2" {
		t.Errorf("ContextArg = %q, want it committed on the way out of the row", m.formSession.ContextArg)
	}
	// A count meant for /sess-load is not a pattern for /sess-use.
	m = stepSession(t, m, "right")
	if m.formSession.Context != ctxUse || m.formSession.ContextArg != "" {
		t.Errorf("after switching mode: context=%q arg=%q — the old argument was carried over",
			m.formSession.Context, m.formSession.ContextArg)
	}
}

// TestSessionPanelFiles covers the repeatable row: enter adds what is in the
// box, ctrl+x takes the last one back.
func TestSessionPanelFiles(t *testing.T) {
	m, _ := openSessionPanel(t)
	for m.sessCursor < sessRowFiles {
		m = stepSession(t, m, "down")
	}
	m.sessInput.SetValue("ai_docs/design.md")
	m = stepSession(t, m, "enter")
	if !slices.Equal(m.formSession.Files, []string{"ai_docs/design.md"}) {
		t.Fatalf("Files = %v, want the one path", m.formSession.Files)
	}
	if m.stage != stageSession {
		t.Fatal("enter with a path in the box left the panel instead of adding it")
	}
	if m.sessInput.Value() != "" {
		t.Errorf("the box still holds %q, want it cleared after adding", m.sessInput.Value())
	}
	m = stepSession(t, m, "ctrl+x")
	if len(m.formSession.Files) != 0 {
		t.Errorf("Files = %v, want the last entry removed", m.formSession.Files)
	}
	// An empty box means there is nothing left to say here.
	m = stepSession(t, m, "enter")
	if m.stage != stageForm {
		t.Errorf("enter on an empty box left stage = %v, want stageForm", m.stage)
	}
}

// TestSessionPanelEscRestoresFocus pins the sub-stage contract closeImages
// keeps: the form gets the keys back in the field that had them.
func TestSessionPanelEscRestoresFocus(t *testing.T) {
	m, _ := openSessionPanel(t)
	// beginAdd starts in the prompt, which is where esc has to land.
	m = stepSession(t, m, "esc")
	if m.stage != stageForm {
		t.Fatalf("esc left stage = %v, want stageForm", m.stage)
	}
	if !m.promptArea.Focused() || m.titleInput.Focused() {
		t.Errorf("after esc: prompt focused=%v title focused=%v — want the keys back in the prompt",
			m.promptArea.Focused(), m.titleInput.Focused())
	}

	// And from the title field, back to the title field.
	next, _ := m.updateForm(pressKey("tab"))
	m = next.(model)
	if m.formFocus != formFieldTitle {
		t.Fatal("tab did not move the focus to the title")
	}
	next, _ = m.updateForm(pressKey("ctrl+r"))
	m = stepSession(t, next.(model), "esc")
	if !m.titleInput.Focused() || m.promptArea.Focused() {
		t.Errorf("after esc from the title: title focused=%v prompt focused=%v",
			m.titleInput.Focused(), m.promptArea.Focused())
	}
}

// TestSessionSavedWithTodo is the whole point of the panel: what it collects
// reaches the backlog file, and a prompt that never opened it writes no session
// key at all.
func TestSessionSavedWithTodo(t *testing.T) {
	m, project := openSessionPanel(t)
	for m.sessCursor < sessRowModel { // past the annotation rows, which lead the panel
		m = stepSession(t, m, "down")
	}
	m = stepSession(t, m, "right") // model → the first alias
	m = stepSession(t, m, "down")
	m = stepSession(t, m, "right") // effort → low
	m = stepSession(t, m, "esc")

	m.promptArea.SetValue("fix the sidebar")
	next, _ := m.saveForm()
	m = next.(model)

	reloaded := &store{scope: scopeProject, path: project.path}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if len(reloaded.todos) != 1 {
		t.Fatalf("backlog holds %d todos, want 1", len(reloaded.todos))
	}
	got := reloaded.todos[0].Session
	if got == nil {
		t.Fatal("the saved todo has no session options")
	}
	if got.Model != sessModelValues[1] || got.Effort != effortLow {
		t.Errorf("saved options = %+v, want model %q and effort %q", got, sessModelValues[1], effortLow)
	}

	// Reopening the todo has to show what was saved.
	ref := todoRef{scope: scopeProject, id: reloaded.todos[0].ID}
	m.project = reloaded
	m.rebuildList()
	next, _ = m.beginEditRef(ref)
	m = next.(model)
	if m.formSession.Effort != effortLow {
		t.Errorf("the edit form opened with effort %q, want %q", m.formSession.Effort, effortLow)
	}

	// And a prompt with no options must leave the backlog exactly as it was
	// before the field existed.
	plain, _, _ := newModelInTemp(t)
	next, _ = plain.beginAdd()
	plain = next.(model)
	plain.promptArea.SetValue("just words")
	next, _ = plain.saveForm()
	plain = next.(model)
	raw, err := os.ReadFile(plain.project.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "session") {
		t.Errorf("a todo with no options wrote a session key:\n%s", raw)
	}
}

// TestFormShowsSessionLine covers the form's own ⚙ line and the toolbar chip
// that opens the panel — the two things that make the feature findable at all.
func TestFormShowsSessionLine(t *testing.T) {
	m := withForm(t, "", "body", 120, 40)
	if !strings.Contains(m.viewForm(), "⚙ default session") {
		t.Errorf("the form does not say what session a fresh prompt gets:\n%s", m.viewForm())
	}
	m.formSession = SessionOpts{Model: "sonnet", Finish: finishWrap}
	if !strings.Contains(m.viewForm(), "⚙ sonnet · wrap") {
		t.Errorf("the form does not show the options it holds:\n%s", m.viewForm())
	}

	chips := m.formChips()
	got := clickForm(m, chips[formActionSession].start+1, m.formBarRow())
	if got.stage != stageSession {
		t.Errorf("clicking the Session chip left stage = %v, want stageSession", got.stage)
	}

	// The summary of a fully configured prompt is longer than a pane, and the
	// toolbar is hit-tested against a computed row — so the line has to be
	// trimmed rather than wrapped, or every click on the bar lands one row high.
	m.formSession = SessionOpts{
		Model: "claude-opus-5", Effort: effortXHigh, Permission: permBypass,
		Context: ctxUse, ContextArg: "todo-drop-improvements", Files: []string{"a.md", "b.md"},
		Reviews: []string{reviewCode, reviewSecurity, reviewSimplify},
		Finish:  finishWrap, Release: true,
	}
	lines := strings.Split(m.viewForm(), "\n")
	for i, line := range lines {
		if lipgloss.Width(line) > 120 {
			t.Errorf("form line %d is %d cells wide in a 120-cell pane: %q", i, lipgloss.Width(line), line)
		}
	}
	if row := m.formBarRow(); row >= len(lines) || !strings.Contains(lines[row], "Save") {
		t.Errorf("with the options set, the toolbar is no longer on row %d:\n%s", m.formBarRow(), m.viewForm())
	}
}

// TestSessionMarkerOnRow checks the list marker: a configured prompt is visible
// as one without being opened.
func TestSessionMarkerOnRow(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	project.todos = []Todo{
		{ID: "a1", Title: "plain", Prompt: "plain"},
		{ID: "a2", Title: "tuned", Prompt: "tuned", Session: &SessionOpts{Finish: finishWrap}},
	}
	m.rebuildList()
	view := m.viewList()
	tuned := ""
	for line := range strings.SplitSeq(view, "\n") {
		if strings.Contains(line, "tuned") {
			tuned = line
		}
		if strings.Contains(line, "plain") && strings.Contains(line, "⚙") {
			t.Errorf("an unconfigured prompt got the marker: %q", line)
		}
	}
	if !strings.Contains(tuned, "⚙") {
		t.Errorf("a configured prompt has no marker on its row: %q\n%s", tuned, view)
	}
}

// TestSessionPanelFitsThePane holds the panel to the pane it is drawn in. Its
// rows are a table read by eye, and one row wrapping puts every row below it a
// line off from where the last glance left them — so the row notes are dropped
// rather than allowed to overflow (see viewSession).
func TestSessionPanelFitsThePane(t *testing.T) {
	for _, width := range []int{60, 80, 100, 120} {
		m := withForm(t, "", "body", width, 40)
		next, _ := m.updateForm(pressKey("ctrl+r"))
		m = next.(model)
		m.formSession = SessionOpts{
			Model: "claude-opus-5", Effort: effortXHigh, Permission: permBypass,
			Context: ctxUse, ContextArg: "todo-drop-improvements",
			Files:  []string{"ai_docs/plans/todo-drop-improvements.md"},
			Finish: finishWrap, Reviews: []string{reviewCode, reviewSecurity}, Release: true,
		}
		for row := range sessRowCount {
			m.sessCursor = row
			m.syncSessInput()
			view := m.viewSession()
			for line := range strings.SplitSeq(view, "\n") {
				if lipgloss.Width(line) > width {
					t.Errorf("at width %d with the cursor on row %d, %q is %d cells wide",
						width, row, line, lipgloss.Width(line))
				}
			}
			if n := len(strings.Split(view, "\n")); n > 40 {
				t.Errorf("at height 40 the panel renders %d lines with the cursor on row %d", n, row)
			}
		}
	}
}

// TestFormCaretKeysSurviveTheSessionChord is the reason the panel is on ctrl+r
// and not on ctrl+e. The form's key switch runs before the editor sees anything,
// so a chord taken here is a chord the textarea loses — and ctrl+e is the
// emacs-style "caret to end of line" the form's own footer teaches.
func TestFormCaretKeysSurviveTheSessionChord(t *testing.T) {
	m := withForm(t, "", "alpha beta gamma", 100, 40)
	m.promptArea.SetCursorColumn(0)

	next, _ := m.updateForm(pressKey("ctrl+e"))
	m = next.(model)
	if m.stage != stageForm {
		t.Fatalf("ctrl+e left the form (stage = %v) — it belongs to the editor", m.stage)
	}
	if got := m.promptArea.LineInfo().ColumnOffset; got != len("alpha beta gamma") {
		t.Errorf("after ctrl+e the caret is at column %d, want the end of the line (%d)",
			got, len("alpha beta gamma"))
	}

	// And its counterpart, which was never in question but is half of the pair
	// the footer names.
	next, _ = m.updateForm(pressKey("ctrl+a"))
	m = next.(model)
	if got := m.promptArea.LineInfo().ColumnOffset; got != 0 {
		t.Errorf("after ctrl+a the caret is at column %d, want the line start", got)
	}

	// The chord that does open the panel, and the footer that names it.
	next, _ = m.updateForm(pressKey("ctrl+r"))
	if next.(model).stage != stageSession {
		t.Errorf("ctrl+r left stage = %v, want stageSession", next.(model).stage)
	}
	// Narrow enough that the chips drop their hints, wide enough that the footer
	// can still carry them — where the footer is the only teacher left.
	if foot := withForm(t, "", "body", 88, 40).formFooter(); !strings.Contains(foot, "ctrl+r session") {
		t.Errorf("a pane too narrow for the chip hints does not name the chord: %q", foot)
	}
	if foot := withForm(t, "", "body", 120, 40).formFooter(); !strings.Contains(foot, "ctrl+a/e") {
		t.Errorf("the footer no longer teaches the caret pair it kept: %q", foot)
	}
}
