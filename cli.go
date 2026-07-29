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
func addFromCLI(args []string) {
	fs := flag.NewFlagSet("cats-todo add", flag.ExitOnError)
	global := fs.Bool("g", false, "add to the global backlog instead of the project's")
	fs.BoolVar(global, "global", false, "alias for -g")
	title := fs.String("t", "", "short title (derived from the prompt's first line when blank)")
	fs.StringVar(title, "title", "", "alias for -t")
	var images stringList
	fs.Var(&images, "i", "attach an image file (repeatable); copied into the backlog")
	fs.Var(&images, "image", "alias for -i")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cats-todo add [-g] [-t title] [-i image]... [prompt...]")
		fmt.Fprintln(os.Stderr, "  the prompt is the remaining args joined; with none it is read from a piped stdin")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args) // ExitOnError: a bad flag prints usage and exits

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
	if err := st.add(Todo{ID: id, Title: t, Prompt: prompt, Images: attached, Created: time.Now()}); err != nil {
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
	fmt.Printf("added%s to the %s backlog (%s)\n", note, strings.ToLower(st.scope.String()), st.path)
}

// stringList collects a repeatable string flag (-i one.png -i two.png) in the
// order given.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ", ") }

func (l *stringList) Set(v string) error {
	*l = append(*l, v)
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
