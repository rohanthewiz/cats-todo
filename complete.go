// complete.go — the hidden `__complete` subcommand, cats-todo's half of cats's
// shell completion.
//
// cats-plugin.toml declares
//
//	[[completions]]
//	binary  = "cats-todo"
//	command = ["./bin/cats-todo", "__complete"]
//
// which makes `catctl completion <shell>` register cats-todo as a completable
// command in the user's shell. The shell then routes every Tab on a cats-todo
// line through `catctl __complete --for cats-todo`, which execs this.
//
// The protocol is the one in cats's cmd/catctl/complete.go: the words typed so
// far arrive as arguments, the last being the (possibly empty) word under the
// cursor, and we print "value<TAB>description" lines followed by a directive
// telling the shell whether to fall back to filenames.
//
// Doing it here rather than as a static list in the manifest is what lets -i
// offer image files and -t offer nothing, and lets every candidate carry the
// same one-line description `cats-todo help` gives.

package main

import (
	"fmt"
	"os"
	"strings"
)

// The directives the protocol allows at the end of a completion reply.
const (
	completeNoFiles = ":nofiles"
	completeFiles   = ":files"
)

// completion is one candidate: the text inserted and an optional description
// that zsh and fish show beside it.
type completion struct {
	value string
	desc  string
}

// topLevelCompletions is the first word: the subcommands, then the two flags
// that open the manager on a single backlog. The long spellings are offered and
// the one-letter aliases are not — a completion menu is where you learn the
// readable name, and -p/-g still work if you type them.
var topLevelCompletions = []completion{
	{"add", "quick-capture a prompt without opening the manager"},
	{"init", "create this project's backlog (.cats-todo/, committed with the repo)"},
	{"export", "write the backlog as a bundle, mail it, or send it to another machine"},
	{"import", "read a bundle into a backlog"},
	{"serve", "offer this machine's backlogs to the local network"},
	{"version", "print the version"},
	{"help", "usage summary"},
	{"--project", "open the manager on this project's backlog only"},
	{"--global", "open the manager on the global backlog only"},
}

// The transfer commands' flags (transfercli.go). Long spellings only, like
// everything else here.
var exportCompletions = []completion{
	{"--global", "export the global backlog instead of this project's"},
	{"--all", "include done and frozen prompts, not just the open ones"},
	{"--out", "directory to write the bundle into"},
	{"--markdown", "write readable markdown instead of a bundle"},
	{"--to", "host[:port] of a machine running `cats-todo serve`"},
	{"--mail", "open a mail composer with the prompts in the body"},
}

var importCompletions = []completion{
	{"--global", "import into the global backlog instead of this project's"},
	{"--keep-ids", "keep the prompts' original ids instead of minting new ones"},
	{"--allow-duplicates", "import prompts this backlog already holds"},
}

var serveCompletions = []completion{
	{"--port", "port to listen on (default 8422)"},
	{"--name", "what this machine calls itself in another manager's picker"},
	{"--inbox", "backlog arriving prompts land in: project or global"},
	{"--allow-remote", "answer requests from outside the local network"},
}

var addCompletions = []completion{
	{"--global", "add to the global backlog instead of this project's"},
	{"--title", "short title (derived from the prompt's first line when blank)"},
	{"--image", "attach an image file (repeatable); copied into the backlog"},
	{"--priority", "how much it matters: critical|high|none"},
	{"--fruit", "mark as low-hanging fruit — cheap for what it pays"},
	{"--flag", "single it out, optionally with a note (--flag=\"why\")"},
	// The session options (see session.go). They are offered here rather than
	// left to the manual for the reason the flags exist at all: the whole point
	// of recording them on the prompt is not having to remember the setup, and a
	// menu is where the spellings get learned.
	{"--model", "model for a new claude session (sonnet, opus, claude-opus-5, …)"},
	{"--effort", "effort for a new claude session (low|medium|high|xhigh|max)"},
	{"--perm", "permission mode for a new claude session (acceptEdits, plan, …)"},
	{"--clear", "send /clear before the prompt when dropping into an existing pane"},
	{"--sess-load", "start by running /sess-load [n]"},
	{"--sess-use", "start by running /sess-use <pattern>"},
	{"--ctx", "also read this file first (repeatable)"},
	{"--finish", "when the work is done: none|commit|push|wrap"},
	{"--review", "run this review skill first (repeatable)"},
	{"--release", "cut a release once the work is done"},
}

// The values the enum-valued flags accept, keyed by flag. A flag that takes one
// of a closed set is the case a completion menu earns its keep on — these are
// exactly the spellings normalize* will accept without folding.
//
// Named for the session options it started as, and now one flag wider than
// that: --priority is an annotation and not a session option (it is a fact about
// the prompt, not about the agent that will read it) but it is the same kind of
// flag, and a second map holding one entry would be worse than a name that
// overstates by one. --fruit is another annotation and takes no value, so it has
// nothing to offer here; --flag's value is free text, which is the other kind of
// flag a completion menu has nothing to say about.
var sessionValueCompletions = map[string][]completion{
	"--priority": {
		{"critical", "do this first"},
		{"high", "before the unmarked work"},
		{"none", "the default — nothing said"},
	},
	"--model": {
		{"opus", "the most capable model"},
		{"sonnet", "the balanced default"},
		{"haiku", "the fastest"},
		{"fable", "the latest Fable model"},
	},
	"--effort": {
		{"low", "shallow reasoning, fastest"},
		{"medium", "the default"},
		{"high", "deeper reasoning"},
		{"xhigh", "deeper still"},
		{"max", "as deep as it goes"},
	},
	"--perm": {
		{"acceptEdits", "accept file edits without asking"},
		{"auto", "decide per action"},
		{"plan", "plan first, change nothing"},
		{"manual", "ask for everything"},
		{"dontAsk", "don't prompt for permission"},
		{"bypassPermissions", "skip every permission check"},
	},
	"--finish": {
		{"none", "stop when the work is done"},
		{"commit", "commit the work"},
		{"push", "commit and push"},
		{"wrap", "run /sess-wrap — session doc, commit, push"},
	},
	"--review": {
		{"code-review", "review the diff for correctness"},
		{"security-review", "review the diff for security"},
		{"simplify", "clean up the changed code"},
	},
}

var initCompletions = []completion{
	{"--force", "replace an existing backlog without asking (destructive)"},
}

// completeFromCLI implements `cats-todo __complete <word>...`. It always exits
// 0 with a well-formed reply: an error message would land where the user
// expects a menu.
func completeFromCLI(args []string) {
	if len(args) == 0 {
		args = []string{""}
	}
	cur := args[len(args)-1]
	prior := args[:len(args)-1]

	// A flag that takes a value, immediately before the cursor: the flag decides
	// what the word is, and there is no cats-todo vocabulary left to offer —
	// except where the value is one of a closed set, which is the one case a
	// menu can answer outright.
	if len(prior) > 0 {
		last := prior[len(prior)-1]
		if vals, ok := sessionValueCompletions[last]; ok {
			emitCompletions(filterCompletions(vals, cur), completeNoFiles)
			return
		}
		switch last {
		case "-i", "--image", "--ctx":
			emitCompletions(nil, completeFiles)
			return
		case "-t", "--title", "--sess-use":
			emitCompletions(nil, completeNoFiles)
			return
		}
	}

	var offer []completion
	switch {
	case len(prior) == 0:
		offer = topLevelCompletions
	case prior[0] == "add" && strings.HasPrefix(cur, "-"):
		offer = addCompletions
	case prior[0] == "init" && strings.HasPrefix(cur, "-"):
		offer = initCompletions
	case prior[0] == "export" && strings.HasPrefix(cur, "-"):
		offer = exportCompletions
	case prior[0] == "import" && strings.HasPrefix(cur, "-"):
		offer = importCompletions
	case prior[0] == "serve" && strings.HasPrefix(cur, "-"):
		offer = serveCompletions
	case prior[0] == "import":
		// The one argument that is a path: the shell's own file completion is
		// better at finding a bundle in a downloads folder than a fixed list
		// could be, and a host is free text either way.
		emitCompletions(nil, completeFiles)
		return
	}
	// Everything else — the prompt words after `add`, anything after `version` —
	// is free text.

	emitCompletions(filterCompletions(offer, cur), completeNoFiles)
}

func filterCompletions(cands []completion, cur string) []completion {
	if cur == "" {
		return cands
	}
	out := make([]completion, 0, len(cands))
	for _, c := range cands {
		if strings.HasPrefix(c.value, cur) {
			out = append(out, c)
		}
	}
	return out
}

func emitCompletions(cands []completion, directive string) {
	var b strings.Builder
	for _, c := range cands {
		b.WriteString(c.value)
		if c.desc != "" {
			b.WriteByte('\t')
			b.WriteString(c.desc)
		}
		b.WriteByte('\n')
	}
	b.WriteString(directive)
	b.WriteByte('\n')
	fmt.Fprint(os.Stdout, b.String())
}
