// cli.go — the `add` subcommand: quick capture into a backlog from a shell,
// keybinding, or pipe, without opening the manager UI.

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// addFromCLI implements `cats-todo add [-g] [-t title] [-i image]... [prompt...]`.
// The prompt is the remaining arguments joined by spaces; with none it is read
// from a piped stdin. The default target is the project backlog rooted at the
// nearest .cats-todo (or .git) directory above the current directory — the same
// root the manager TUI resolves, so both entry points reach one backlog per
// project; -g targets the global backlog instead.
//
// -i attaches an image, repeatably. The file is copied into the backlog rather
// than referenced (see images.go), so a screenshot can be attached and then
// cleared off the Desktop; at drop time its path rides along in the prompt for
// the agent to read.
//
// The session flags (--model, --finish, …) are the CLI's half of the form's ⚙
// panel: the same record, the same normalization, so a value the TUI would
// refuse is refused here with the same words (see session.go).
func addFromCLI(args []string) {
	fs := flag.NewFlagSet("cats-todo add", flag.ExitOnError)
	global := fs.Bool("g", false, "add to the global backlog instead of the project's")
	fs.BoolVar(global, "global", false, "alias for -g")
	title := fs.String("t", "", "short title (derived from the prompt's first line when blank)")
	fs.StringVar(title, "title", "", "alias for -t")
	var images stringList
	fs.Var(&images, "i", "attach an image file (repeatable); copied into the backlog")
	fs.Var(&images, "image", "alias for -i")

	// Session options. Every one of them is optional and every zero value means
	// "inherit the default", so `add` with none of them behaves exactly as it
	// did before they existed.
	model := fs.String("model", "", "model for a new claude session (sonnet, opus, claude-opus-5, …)")
	effort := fs.String("effort", "", "effort for a new claude session (low|medium|high|xhigh|max)")
	perm := fs.String("perm", "", "permission mode for a new claude session (acceptEdits|auto|plan|manual|dontAsk|bypassPermissions)")
	clear := fs.Bool("clear", false, "send /clear before the prompt when dropping into an existing pane")
	var sessLoad optString
	fs.Var(&sessLoad, "sess-load", "start by running /sess-load [n] (--sess-load, --sess-load=2, or --sess-load 2)")
	sessUse := fs.String("sess-use", "", "start by running /sess-use <pattern>")
	var ctxFiles stringList
	fs.Var(&ctxFiles, "ctx", "also read this file first (repeatable)")
	finish := fs.String("finish", "", "when the work is done: none|commit|push|wrap")
	var reviews stringList
	fs.Var(&reviews, "review", "run this review skill first (repeatable): code-review, security-review, simplify")
	release := fs.Bool("release", false, "cut a release once the work is done")

	// Long-only, unlike -g/-t/-i above. A bare -p is free as far as Go's flag
	// package is concerned — it does no prefix matching, so it would not collide
	// with --perm — but -p sitting on the same command line as --perm reads to a
	// person as an abbreviation of it, and a flag that looks like it means
	// permissions while meaning priority is the kind of thing that gets found
	// out at the wrong moment.
	priority := fs.String("priority", "", "how much it matters: critical|standard|low (default standard)")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cats-todo add [-g] [-t title] [-i image]... [--priority p] [session options] [prompt...]")
		fmt.Fprintln(os.Stderr, "  the prompt is the remaining args joined; with none it is read from a piped stdin")
		fmt.Fprintln(os.Stderr, "  session options: --model --effort --perm --clear --sess-load[=n] --sess-use --ctx")
		fmt.Fprintln(os.Stderr, "                   --finish --review --release")
		fs.PrintDefaults()
	}
	// ExitOnError: a bad flag prints usage and exits. The rewrite has to happen
	// first — see expandSessLoad, which is what lets `--sess-load 2` mean what
	// it looks like it means.
	_ = fs.Parse(expandSessLoad(args))

	opts := sessionFromFlags(*model, *effort, *perm, *clear, sessLoad.set, sessLoad.value, *sessUse,
		ctxFiles, *finish, reviews, *release)

	// Checked before anything is written, and through the same normalizer the
	// TUI's ring is built on, so a value this program would refuse is refused
	// here in the same words.
	prio, err := normalizePriority(*priority)
	if err != nil {
		errExit(err)
	}

	prompt := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if prompt == "" {
		prompt = strings.TrimSpace(readPipedStdin())
	}
	if prompt == "" {
		fs.Usage()
		os.Exit(2)
	}

	var st *store
	if *global {
		path, err := globalTodosPath()
		if err != nil {
			errExit("could not resolve the global backlog:", err)
		}
		st = &store{scope: scopeGlobal, path: path}
	} else {
		wd, err := os.Getwd()
		if err != nil {
			errExit("could not determine the working directory:", err)
		}
		root := findProjectRoot(wd)
		// No project owns this directory (the cwd is the filesystem root). An
		// unavailable store saves to nothing and reports success, so without
		// this the prompt would be swallowed and the command would still print
		// "added".
		if root == "" {
			errExit("no project backlog here — run from a project directory, or use -g for the global backlog")
		}
		st = &store{scope: scopeProject, path: projectTodosPath(root)}
	}

	t := strings.TrimSpace(*title)
	if t == "" {
		t = firstLine(prompt, 60)
	}

	// The id has to exist before the copy — it names the attachment directory —
	// so it is minted here rather than inline in the Todo below.
	id := newID()
	attached, err := st.attachImages(id, images)
	if err != nil {
		errExit("could not attach image:", err)
	}
	if err := st.add(Todo{
		ID: id, Title: t, Prompt: prompt, Images: attached,
		Session:  sessionPtr(opts),
		Priority: prio,
		Created:  time.Now(),
	}); err != nil {
		// The copies are already on disk but no todo will ever reference them.
		st.removeImages(id)
		errExit("could not save:", err)
	}

	note := ""
	if n := len(attached); n == 1 {
		note = " with 1 image"
	} else if n > 1 {
		note = fmt.Sprintf(" with %d images", n)
	}
	// The priority rides on the same line rather than with the session options
	// below: it is a fact about the prompt, like the images, not part of how the
	// agent that receives it will be set up. It trails the backlog clause rather
	// than joining the image note, so both halves stay readable together
	// ("added with 1 image to the project backlog, marked critical").
	//
	// Standard says nothing at all — the default announcing itself on every add
	// is noise, and silence is what "ordinary" should sound like.
	prioNote := ""
	if prio != priorityStandard {
		prioNote = ", marked " + priorityLabel(prio)
	}
	fmt.Printf("added%s to the %s backlog%s (%s)\n", note, strings.ToLower(st.scope.String()), prioNote, st.path)
	// Echoed on its own line, and only when there is something to echo: the
	// options are the part of this command that is easiest to get subtly wrong
	// (a folded spelling, a --sess-load count that landed as a count rather than
	// as the first word of the prompt), and one line is what makes that visible
	// at the moment it happened.
	if s := opts.summary(); s != "" {
		fmt.Println("  ⚙ " + s)
	}
}

// sessionFromFlags builds the record the add flags describe, exiting with the
// same message the TUI would show for a value that cannot be normalized.
//
// --sess-load and --sess-use are mutually exclusive: they are two answers to one
// question ("what prior context"), and a record holding both would have to pick
// one silently.
func sessionFromFlags(model, effort, perm string, clear, loadSet bool, loadArg, use string,
	files stringList, finish string, reviews stringList, release bool) SessionOpts {

	var o SessionOpts
	var err error
	if o.Model, err = normalizeModel(model); err != nil {
		errExit(err)
	}
	if o.Effort, err = normalizeEffort(effort); err != nil {
		errExit(err)
	}
	if o.Permission, err = normalizePermission(perm); err != nil {
		errExit(err)
	}
	if o.Finish, err = normalizeFinish(finish); err != nil {
		errExit(err)
	}
	o.Clear = clear
	o.Release = release
	o.Files = files

	if loadSet && strings.TrimSpace(use) != "" {
		errExit("--sess-load and --sess-use both name the prior context to start from — pick one")
	}
	switch {
	case loadSet:
		o.Context, o.ContextArg = ctxLoad, strings.TrimSpace(loadArg)
	case strings.TrimSpace(use) != "":
		o.Context, o.ContextArg = ctxUse, strings.TrimSpace(use)
	}

	for _, r := range reviews {
		v, err := normalizeReview(r)
		if err != nil {
			errExit(err)
		}
		if v != "" {
			o.Reviews = append(o.Reviews, v)
		}
	}
	return o
}

// expandSessLoad rewrites `--sess-load 2` into `--sess-load=2` before parsing,
// which is what lets the flag's optional argument work at all.
//
// The flag package can only express an optional value as a boolean-shaped flag
// (see optString), and a boolean flag never consumes the following word. Two
// things then go wrong with the spelling everyone actually types: the count
// becomes the first word of the prompt, and — because parsing stops dead at the
// first non-flag argument — every flag after it does too. Joining the pair up
// front fixes both, and leaves the rest of the command line to the flag package.
//
// Only a token of nothing but digits is claimed, and only immediately after the
// flag. No prompt worth saving starts with a lone number, and the "⚙" line
// printed after the add says which reading was taken.
func expandSessLoad(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		// The flag terminator: everything past it is the prompt, and a number
		// there is the user's word, not ours.
		if a == "--" {
			return append(out, args[i:]...)
		}
		if (a == "--sess-load" || a == "-sess-load") && i+1 < len(args) && isAllDigits(args[i+1]) {
			out = append(out, a+"="+args[i+1])
			i++
			continue
		}
		out = append(out, a)
	}
	return out
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// stringList collects a repeatable string flag (-i one.png -i two.png) in the
// order given.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ", ") }

func (l *stringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// optString is a flag whose value may be left off: `--sess-load` means "the
// default" and `--sess-load=2` means two. IsBoolFlag is what the flag package
// offers for that — it makes the value optional — at the cost of never
// consuming the *following* word, which expandSessLoad puts back.
//
// set is kept separately from value because "given, with no argument" and "not
// given" are different answers here and both look like "".
type optString struct {
	set   bool
	value string
}

func (o *optString) String() string { return o.value }

func (o *optString) IsBoolFlag() bool { return true }

func (o *optString) Set(v string) error {
	o.set = true
	// The bare form arrives as "true" — that is the flag package saying "no
	// argument was given", not a value anybody typed.
	if v != "true" {
		o.value = v
	}
	return nil
}

// readPipedStdin returns everything on stdin when it is a pipe or file, and ""
// when stdin is an interactive terminal (we never block waiting for typing).
func readPipedStdin() string {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return ""
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return ""
	}
	return string(data)
}
