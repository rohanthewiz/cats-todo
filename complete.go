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
	{"version", "print the version"},
	{"help", "usage summary"},
	{"--project", "open the manager on this project's backlog only"},
	{"--global", "open the manager on the global backlog only"},
}

var addCompletions = []completion{
	{"--global", "add to the global backlog instead of this project's"},
	{"--title", "short title (derived from the prompt's first line when blank)"},
	{"--image", "attach an image file (repeatable); copied into the backlog"},
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
	// what the word is, and there is no cats-todo vocabulary left to offer.
	if len(prior) > 0 {
		switch prior[len(prior)-1] {
		case "-i", "--image":
			emitCompletions(nil, completeFiles)
			return
		case "-t", "--title":
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
