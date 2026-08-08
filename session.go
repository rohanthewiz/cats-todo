// session.go — per-todo session options: how the agent that receives a dropped
// prompt should be set up.
//
// A drop used to deliver one thing, the prompt text. Everything about *how* the
// receiving agent ran — which model, at what effort, starting from what prior
// context, and what to do once the work is finished — was whatever the default
// was, and had to be arranged by hand on every drop. These options make those
// choices a property of the prompt instead, stored with it in todos.json and
// travelling with the repo, so a drop reproduces the whole setup whether it was
// fired by a keystroke or by a schedule at 3am.
//
// Three delivery mechanisms carry them, and which one a given option rides is
// forced by what the receiving end can accept:
//
//	model / effort / permission → flags on the agent's argv   (new sessions only)
//	clear first                 → "/clear" as its own message  (existing panes)
//	context loading, wrap-up    → text around the prompt body  (every drop)
//
// This file holds the record and the pure logic. The delivery itself is drop.go,
// the editor is ui.go's stageSession, and the flags are cli.go's.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// SessionOpts is one prompt's answer to "how should the agent that gets this be
// set up". Every field's zero value means "inherit the default", so a zero
// SessionOpts — and a nil *SessionOpts — behaves exactly as a drop did before
// this existed. That is not a convenience: it is the compatibility contract, and
// composePrompt's byte-for-byte tests are what hold us to it.
//
// The string fields hold wire format, like scheduleKindPane: they are written to
// todos.json, which is committed and shared, so the constants below are the
// spelling and not just this binary's internal names.
type SessionOpts struct {
	Model      string   `json:"model,omitempty"`      // "sonnet" | "claude-opus-5" | ""
	Effort     string   `json:"effort,omitempty"`     // low|medium|high|xhigh|max
	Permission string   `json:"permission,omitempty"` // canonical claude spelling
	Clear      bool     `json:"clear,omitempty"`      // /clear an existing pane first
	Context    string   `json:"context,omitempty"`    // ctxNone|ctxLoad|ctxUse
	ContextArg string   `json:"contextArg,omitempty"` // "2" for sess-load, a pattern for sess-use
	Files      []string `json:"files,omitempty"`      // extra files to read, repeatable
	Finish     string   `json:"finish,omitempty"`     // finishNone|finishCommit|finishPush|finishWrap
	Reviews    []string `json:"reviews,omitempty"`    // code-review | security-review | simplify
	Release    bool     `json:"release,omitempty"`
}

// The prior-context modes. ctxNone is the empty string so that "not set" and
// "explicitly none" are the same value — there is nothing a third state could
// mean here, and one fewer state is one fewer way for a hand-edited backlog to
// say something this binary would have to guess about.
const (
	ctxNone = ""
	ctxLoad = "load" // /sess-load [n] — the last n saved session docs
	ctxUse  = "use"  // /sess-use <pattern> — the saved session matching it
)

// What to do once the work is done. Same empty-means-default rule as ctxNone.
// Exiting the agent is deliberately not among them: a drop that closes the
// session it just used takes the transcript with it, and the one thing worth
// having after an unattended run is the conversation that produced it.
const (
	finishNone   = ""
	finishCommit = "commit"
	finishPush   = "push"
	finishWrap   = "wrap"
)

// The review skills offered in the editor. Any skill name is accepted from the
// CLI — they vary per machine — but these three are the ones with a row in the
// panel, so they get names here rather than being spelled out at each use.
const (
	reviewCode     = "code-review"
	reviewSecurity = "security-review"
	reviewSimplify = "simplify"
)

// The effort levels `claude --effort` accepts, in order of increasing depth.
const (
	effortLow    = "low"
	effortMedium = "medium"
	effortHigh   = "high"
	effortXHigh  = "xhigh"
	effortMax    = "max"
)

// The permission modes `claude --permission-mode` accepts, in its own spelling.
// The camelCase is claude's, not ours — normalizePermission is what lets a user
// type "accept-edits" and still land on the string the flag will take.
const (
	permAcceptEdits = "acceptEdits"
	permAuto        = "auto"
	permPlan        = "plan"
	permManual      = "manual"
	permDontAsk     = "dontAsk"
	permBypass      = "bypassPermissions"
)

// sessionPtr is the record as it goes onto a Todo: nil when nothing is set, so
// an unconfigured prompt writes no "session" key at all and a backlog reads
// exactly as it did before the field existed.
func sessionPtr(o SessionOpts) *SessionOpts {
	if !o.configured() {
		return nil
	}
	c := o.clone()
	return &c
}

// clone is a copy that shares nothing with the original. A plain struct
// assignment copies the two slice headers, which would leave an editor's Files
// list appending into the stored todo's backing array — the form's whole
// contract is that cancelling changes nothing, and a shared array is how that
// quietly stops being true.
func (o SessionOpts) clone() SessionOpts {
	o.Files = slices.Clone(o.Files)
	o.Reviews = slices.Clone(o.Reviews)
	return o
}

// configured reports whether any option is set — the difference between "this
// prompt has opinions" and "this prompt takes the defaults". Written out field
// by field rather than compared against a zero value because the struct holds
// slices and is therefore not comparable.
func (o *SessionOpts) configured() bool {
	if o == nil {
		return false
	}
	return o.Model != "" || o.Effort != "" || o.Permission != "" || o.Clear ||
		o.Context != ctxNone || strings.TrimSpace(o.ContextArg) != "" ||
		len(o.Files) > 0 || o.Finish != finishNone || len(o.Reviews) > 0 || o.Release
}

// --- Normalization ---------------------------------------------------------
//
// One set of functions for both entry points, so a value rejected in the TUI is
// rejected by the CLI with the same words. They are also the guard on what
// reaches an argv: everything here ends up after a "--flag" on a command line,
// and a value with a space in it would silently become two arguments.

// normalizeModel accepts a model alias or a full model id and passes it through.
// It is deliberately not checked against a list: new model names ship far faster
// than this binary does, and a manager that refused "claude-opus-6" the week it
// landed would be wrong in the one direction that costs the user something. The
// checks are only for values that could not be a model name at all.
func normalizeModel(s string) (string, error) {
	v := strings.TrimSpace(s)
	if v == "" {
		return "", nil
	}
	if strings.ContainsAny(v, " \t\n") {
		return "", fmt.Errorf("model %q looks wrong — give one alias or id (sonnet, opus, claude-opus-5)", s)
	}
	if strings.HasPrefix(v, "-") {
		return "", fmt.Errorf("model %q looks like a flag — did a value go missing?", s)
	}
	return v, nil
}

// normalizeEffort folds the friendly spellings onto claude's own and rejects
// anything else. The list is short and closed, so unlike the model there is
// nothing to be gained by letting an unknown value through to fail at launch.
func normalizeEffort(s string) (string, error) {
	switch foldOption(s) {
	case "":
		return "", nil
	case "low":
		return effortLow, nil
	case "medium", "med":
		return effortMedium, nil
	case "high":
		return effortHigh, nil
	case "xhigh", "extrahigh":
		return effortXHigh, nil
	case "max":
		return effortMax, nil
	}
	return "", fmt.Errorf("effort %q is not one of low, medium, high, xhigh, max", s)
}

// normalizePermission folds the spellings a person would actually type onto the
// camelCase claude expects: "accept-edits" and "bypass" are what the flag means
// to a reader, and "acceptEdits" is what it means to the binary.
func normalizePermission(s string) (string, error) {
	switch foldOption(s) {
	case "":
		return "", nil
	case "acceptedits", "accept":
		return permAcceptEdits, nil
	case "auto":
		return permAuto, nil
	case "plan":
		return permPlan, nil
	case "manual":
		return permManual, nil
	case "dontask", "noask":
		return permDontAsk, nil
	case "bypasspermissions", "bypass":
		return permBypass, nil
	}
	return "", fmt.Errorf("permission mode %q is not one of acceptEdits, auto, plan, manual, dontAsk, bypassPermissions", s)
}

// normalizeFinish reads the wrap-up choice. "none" is spelled out as well as
// left blank, so `--finish none` can override a value rather than only being the
// absence of one.
func normalizeFinish(s string) (string, error) {
	switch foldOption(s) {
	case "", "none":
		return finishNone, nil
	case "commit":
		return finishCommit, nil
	case "push":
		return finishPush, nil
	case "wrap", "sesswrap":
		return finishWrap, nil
	}
	return "", fmt.Errorf("finish %q is not one of none, commit, push, wrap", s)
}

// normalizeReview reads a review skill name. Any name passes — the skills
// installed on the machine the prompt lands on are not knowable from here — but
// the leading slash comes off, because "/code-review" is how it is invoked and
// "code-review" is what it is called.
func normalizeReview(s string) (string, error) {
	v := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "/"))
	if v == "" {
		return "", nil
	}
	if strings.ContainsAny(v, " \t\n") {
		return "", fmt.Errorf("review %q looks wrong — give one skill name (code-review, security-review, simplify)", s)
	}
	return v, nil
}

// foldOption reduces a typed option to its comparable core: case, surrounding
// space, and the separators nobody agrees on ("accept-edits", "accept_edits")
// are all noise around the same choice.
func foldOption(s string) string {
	v := strings.ToLower(strings.TrimSpace(s))
	v = strings.ReplaceAll(v, "-", "")
	return strings.ReplaceAll(v, "_", "")
}

// --- Delivery: launch flags -------------------------------------------------

// launchArgs are the flags appended to a new session's argv. They are claude's
// flags, so any other agent gets none: dropping them and delivering the prompt
// is the chosen behaviour, because the prompt is the part that always works and
// a tab that fails to exec is a drop that delivered nothing at all. The picker
// says so on the row before the choice is made (see buildTargets).
func (o *SessionOpts) launchArgs(command string) []string {
	if o == nil || !isClaudeCommand(command) {
		return nil
	}
	var args []string
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	if o.Effort != "" {
		args = append(args, "--effort", o.Effort)
	}
	if o.Permission != "" {
		args = append(args, "--permission-mode", o.Permission)
	}
	return args
}

// hasLaunchFlags reports whether any option would ride on the argv — what the
// picker needs to know to warn that a non-claude row will drop them.
func (o *SessionOpts) hasLaunchFlags() bool {
	return o != nil && (o.Model != "" || o.Effort != "" || o.Permission != "")
}

// isClaudeCommand reports whether a launch command runs Claude Code. The command
// is an argv, not a shell line, so the agent is its first word; a path is
// allowed for the same reason cats reports one ("/usr/local/bin/claude" is still
// claude).
func isClaudeCommand(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	return filepath.Base(fields[0]) == "claude"
}

// --- Delivery: prompt text --------------------------------------------------

// The headers of the two blocks composePrompt wraps a prompt body in, named
// here next to imageBlockHeader for the same reason it is named: they are the
// delivered text, so a test can pin them and a reader can find them.
const (
	contextBlockLead = "First, load prior context: run "
	filesBlockLead   = "Also read these files: "
	postambleHeader  = "When the work is done and the tests pass:"
	// The release line is phrased against the conventional-commit shape this
	// repo (and most of the ones a prompt lands in) uses, but deliberately does
	// not name which files carry the version: cats-todo drops prompts into every
	// project on the machine, and a line naming this one's main.go would be
	// wrong everywhere else.
	releaseLine = "cut a release: bump the version, commit it as `chore(release): vX.Y.Z`, tag it, and push"
)

// preamble is the text that goes above the prompt body: what to load before
// reading it. Empty when nothing is set, which is what keeps an unconfigured
// drop byte-identical to what it was.
func (o *SessionOpts) preamble() string {
	if o == nil {
		return ""
	}
	var lines []string
	if cmd := o.contextCommand(); cmd != "" {
		lines = append(lines, contextBlockLead+cmd)
	}
	if len(o.Files) > 0 {
		lines = append(lines, filesBlockLead+strings.Join(o.Files, ", "))
	}
	return strings.Join(lines, "\n")
}

// contextCommand is the slash command the Context/ContextArg pair names, or ""
// when there is none to give.
//
// A /sess-use with no pattern is treated as nothing rather than as a bare
// command: the skill's whole input is the pattern, so sending it without one
// asks the agent to guess which saved session was meant — and it would guess
// while holding the keys to the work. /sess-load has a real default (the last
// session), so a bare one is honest.
func (o *SessionOpts) contextCommand() string {
	arg := strings.TrimSpace(o.ContextArg)
	switch o.Context {
	case ctxLoad:
		if arg != "" {
			return "/sess-load " + arg
		}
		return "/sess-load"
	case ctxUse:
		if arg != "" {
			return "/sess-use " + arg
		}
	}
	return ""
}

// postamble is the text that goes below the prompt body: what to do once the
// work is finished. It is a list under one header rather than a sentence per
// item, so an agent reading it has an unambiguous set of steps and the user
// reading the delivered prompt can see at a glance what was asked for.
func (o *SessionOpts) postamble() string {
	if o == nil {
		return ""
	}
	var items []string
	for _, r := range o.Reviews {
		if r = strings.TrimSpace(r); r != "" {
			items = append(items, "run /"+r)
		}
	}
	switch o.Finish {
	case finishCommit:
		items = append(items, "commit the work")
	case finishPush:
		items = append(items, "commit and push")
	case finishWrap:
		items = append(items, "run /sess-wrap (saves a session doc, commits, and pushes)")
	}
	if o.Release {
		items = append(items, releaseLine)
	}
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(postambleHeader)
	for _, it := range items {
		b.WriteString("\n- ")
		b.WriteString(it)
	}
	return b.String()
}

// --- Display ----------------------------------------------------------------

// summary is the one-line form shown wherever there is a line to spare: the
// form's ⚙ row, the prompt view, the panel's own heading. Empty when nothing is
// set, so a caller can use it as the "is there anything to say" test as well as
// the thing to say.
//
// Order follows the panel's rows, which is the order the options take effect:
// how the session launches, what it starts from, what it does, how it ends.
func (o *SessionOpts) summary() string {
	if o == nil {
		return ""
	}
	var segs []string
	add := func(s string) {
		if s != "" {
			segs = append(segs, s)
		}
	}
	add(o.Model)
	add(o.Effort)
	add(o.Permission)
	if o.Clear {
		add("clear first")
	}
	// Without the slash: the summary is a description of the setup, not a line
	// to be typed, and the panel's own row shows the command form.
	add(strings.TrimPrefix(o.contextCommand(), "/"))
	switch n := len(o.Files); {
	case n == 1:
		add("1 file")
	case n > 1:
		add(fmt.Sprintf("%d files", n))
	}
	for _, r := range o.Reviews {
		add(r)
	}
	switch o.Finish {
	case finishCommit:
		add("commit")
	case finishPush:
		add("push")
	case finishWrap:
		add("wrap")
	}
	if o.Release {
		add("release")
	}
	return strings.Join(segs, " · ")
}

// --- The sess-* gate --------------------------------------------------------

// sessCommandDir is the subdirectory of a claude config dir that holds the
// slash commands the context options invoke. They live in commands/, not
// skills/ — the two are different things and only one of them is /sess-load.
const sessCommandDir = "commands"

// sessProbeCommand is the file whose presence stands for the whole sess-* set.
// One probe rather than five: they are installed together, and a partial set is
// not a state worth a different message.
const sessProbeCommand = "sess-load.md"

// sessCommandDirs are the places a claude slash command can be installed, in
// the order claude itself resolves them: the user's config, an overridden
// config, and the project's own .claude — the last relative to the process's
// working directory, which for a manager launched in a cats pane is the project.
func sessCommandDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".claude", sessCommandDir))
	}
	if cfg := os.Getenv("CLAUDE_CONFIG_DIR"); cfg != "" {
		dirs = append(dirs, filepath.Join(cfg, sessCommandDir))
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(wd, ".claude", sessCommandDir))
	}
	return dirs
}

// sessSkillsAvailable reports whether the sess-* commands are installed
// anywhere this machine's claude would find them.
//
// It only greys the context rows and adds a note. It deliberately does not block
// a save: a backlog is committed and travels, so a prompt written on a machine
// without the commands may well be dropped on one that has them, and refusing to
// record the intent would make the manager wrong about a machine it cannot see.
func sessSkillsAvailable() bool {
	for _, dir := range sessCommandDirs() {
		if _, err := os.Stat(filepath.Join(dir, sessProbeCommand)); err == nil {
			return true
		}
	}
	return false
}

// --- Cycling ----------------------------------------------------------------

// cycleValue steps cur one place through vals, wrapping at both ends — the
// panel's ←/→ on an enum row.
//
// A value that is not in vals (a model alias typed at the CLI, a permission mode
// this build does not offer a row for) is appended to the end of the ring rather
// than replaced, so opening the panel and cycling past a row cannot silently
// throw away a setting the user made somewhere else.
func cycleValue(vals []string, cur string, delta int) string {
	list := vals
	if !slices.Contains(vals, cur) {
		list = append(append([]string{}, vals...), cur)
	}
	i := slices.Index(list, cur)
	n := len(list)
	return list[((i+delta)%n+n)%n]
}
