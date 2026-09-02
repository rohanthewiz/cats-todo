// transfercli.go — `cats-todo export`, `import` and `serve`.
//
// The manager's pickers are the way these are used day to day; these are the
// same three operations for the places a TUI cannot go — a script, an ssh
// session, a machine that is only ever the receiving end. They share every
// piece of the implementation with the pickers (bundle.go, peer.go): what is
// here is argument parsing and the words printed afterwards.
//
//	cats-todo export [-g] [--all] [--out DIR] [--markdown] [--to HOST] [--mail]
//	cats-todo import [-g] <file|dir|host>
//	cats-todo serve  [--port N] [--name LABEL] [--inbox project|global] [--allow-remote]
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// exportFromCLI writes, mails or posts a backlog without opening the manager.
//
// The subject is a whole backlog rather than a selection — there is no
// highlighted row on a command line, and `--all` versus "the open ones" is the
// only distinction worth a flag out here.
func exportFromCLI(args []string) {
	fs := flag.NewFlagSet("cats-todo export", flag.ExitOnError)
	global := fs.Bool("g", false, "export the global backlog instead of this project's")
	fs.BoolVar(global, "global", false, "export the global backlog instead of this project's")
	all := fs.Bool("all", false, "include done and frozen prompts (default: the open ones)")
	out := fs.String("out", "", "directory to write the bundle into (default: the current directory)")
	markdown := fs.Bool("markdown", false, "write the prompts as readable markdown instead of a bundle")
	to := fs.String("to", "", "host[:port] of a machine running `cats-todo serve` to send it to")
	mail := fs.Bool("mail", false, "open a mail composer with the prompts in the body")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cats-todo export [-g] [--all] [--out DIR] [--markdown] [--to HOST] [--mail]")
		fmt.Fprintln(os.Stderr, "  with no destination flag, writes a bundle file into --out (or the current directory)")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	st := cliStore(*global)
	if err := st.load(); err != nil {
		errExit(fmt.Sprintf("read %s: %v", shortenHome(st.path), err))
	}
	todos := st.todos
	if !*all {
		todos = openTodos(st.todos)
	}
	if len(todos) == 0 {
		errExit("that backlog has no prompts to export")
	}

	source := "global"
	if !*global {
		source = firstNonEmpty(baseName(backlogRoot(st)), "cats-todo")
	}
	b, files, dropped := buildBundle(st, todos, bundleFrom(), source)

	switch {
	case *to != "":
		data, _, err := encodeBundle(b, files)
		if err != nil {
			errExit(err.Error())
		}
		reply, err := peerSend(*to, data)
		if err != nil {
			errExit(err.Error())
		}
		fmt.Println(*to + ": " + firstNonEmpty(reply, promptWord(len(b.Todos))+" sent"))
	case *mail:
		u := mailtoURL("", mailSubject(b.Source, len(b.Todos)), renderBundleMarkdown(b))
		if mailtoTooLong(u) {
			errExit("too much text for a mail composer — export a bundle and attach it instead")
		}
		if err := openURL(u); err != nil {
			errExit(err.Error())
		}
		fmt.Println(promptWord(len(b.Todos)) + " → a new mail message")
	case *markdown:
		path, err := writeText(firstNonEmpty(*out, "."), slugForFile(b.Source)+"-"+b.Created.Format("2006-01-02")+".md",
			renderBundleMarkdown(b))
		if err != nil {
			errExit(err.Error())
		}
		fmt.Println(promptWord(len(b.Todos)) + " → " + shortenHome(path))
	default:
		path, err := writeBundle(firstNonEmpty(*out, "."), "", b, files)
		if err != nil {
			errExit(err.Error())
		}
		fmt.Println(promptWord(len(b.Todos)) + " → " + shortenHome(path))
	}
	if dropped > 0 {
		fmt.Println("  " + scheduleDropNote(dropped))
	}
}

// importFromCLI reads a bundle — a file, a directory holding exactly one, or a
// machine — into a backlog.
func importFromCLI(args []string) {
	fs := flag.NewFlagSet("cats-todo import", flag.ExitOnError)
	global := fs.Bool("g", false, "import into the global backlog instead of this project's")
	fs.BoolVar(global, "global", false, "import into the global backlog instead of this project's")
	keepIDs := fs.Bool("keep-ids", false, "keep the prompts' original ids instead of minting new ones")
	dupes := fs.Bool("allow-duplicates", false, "import prompts this backlog already holds")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cats-todo import [-g] [--keep-ids] [--allow-duplicates] <file|directory|host>")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	src := fs.Arg(0)

	b, open, err := readBundleFrom(src)
	if err != nil {
		errExit(err.Error())
	}
	st := cliStore(*global)
	res, err := importBundle(st, b, open, importOpts{keepIDs: *keepIDs, allowDuplicates: *dupes})
	if err != nil {
		errExit(err.Error())
	}
	where := "project"
	if *global {
		where = "global"
	}
	fmt.Printf("imported %s from %s → the %s backlog\n", promptWord(res.added), src, where)
	if res.skipped > 0 {
		fmt.Printf("  %d were already there\n", res.skipped)
	}
	if res.noFiles > 0 {
		fmt.Printf("  %d arrived without their attachments\n", res.noFiles)
	}
}

// readBundleFrom resolves what the user typed: a bundle file, a directory
// holding one, or a machine to pull from. The three are told apart by the
// filesystem rather than by a flag — a path that exists is a path, and anything
// else is a host, which is the guess a person would make reading the same
// argument.
func readBundleFrom(src string) (Bundle, bundleOpener, error) {
	if isDir(src) {
		file, err := bundleInDir(src)
		if err != nil {
			return Bundle{}, nil, err
		}
		return readBundle(file)
	}
	if _, err := os.Stat(src); err == nil {
		return readBundle(src)
	}
	return peerFetch(src)
}

// serveFromCLI runs the LAN service until the process is stopped.
//
// It prints the token on every start, not only the first: the token is the
// thing the operator has to carry to the other machine, and a service that
// showed it once and then hid it would be asking them to go and find a JSON
// file.
func serveFromCLI(args []string) {
	fs := flag.NewFlagSet("cats-todo serve", flag.ExitOnError)
	port := fs.Int("port", 0, "port to listen on (default: the saved one, else 8422)")
	name := fs.String("name", "", "what this machine calls itself in another manager's picker")
	inbox := fs.String("inbox", "", "backlog arriving prompts land in: project or global")
	allowRemote := fs.Bool("allow-remote", false, "answer requests from outside the local network")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cats-todo serve [--port N] [--name LABEL] [--inbox project|global] [--allow-remote]")
		fmt.Fprintln(os.Stderr, "  serves this directory's backlog (and the global one) to the local network")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	set := loadSettings()
	if set.peerToken == "" {
		tok, err := newPeerToken()
		if err != nil {
			errExit("could not generate a token: " + err.Error())
		}
		set.peerToken = tok
		if err := set.save(); err != nil {
			errExit("could not save the token: " + err.Error())
		}
	}
	if *name != "" {
		set.peerName = *name
	}
	if *port != 0 {
		set.peerPort = *port
	}
	if set.peerPort == 0 {
		set.peerPort = peerDefaultPort
	}
	switch *inbox {
	case "global":
		set.peerInbox = scopeGlobal
	case "project":
		set.peerInbox = scopeProject
	case "":
	default:
		errExit("--inbox takes project or global")
	}
	if *allowRemote {
		set.peerAllowRemote = true
	}

	project, global, err := loadStores(gatherRunContext(nil, launchBoth))
	if err != nil {
		errExit(err.Error())
	}
	ps := &peerServer{
		name:        set.peerName,
		token:       set.peerToken,
		project:     project,
		global:      global,
		inbox:       set.peerInbox,
		allowRemote: set.peerAllowRemote,
	}
	if s := ps.storeFor(ps.inbox); s == nil || !s.available() {
		errExit(fmt.Sprintf("there is no %s backlog here to receive into — run from a project, or --inbox global",
			strings.ToLower(ps.inbox.String())))
	}

	addr := fmt.Sprintf(":%d", set.peerPort)
	ln, _, err := listenPeer(ps, addr)
	if err != nil {
		errExit("could not listen on " + addr + ": " + err.Error())
	}
	defer ln.Close()

	fmt.Printf("cats-todo v%s serving on %s\n", version, ln.Addr())
	for _, b := range ps.peerBacklogs() {
		fmt.Printf("  %-8s %s (%d open)\n", b.ID, b.Label, b.Open)
	}
	fmt.Printf("  inbox    the %s backlog\n", strings.ToLower(ps.inbox.String()))
	fmt.Printf("  token    %s\n", set.peerToken)
	fmt.Println("  the machine sending to this one needs that token in its settings.json (peerToken)")
	if ps.allowRemote {
		fmt.Println("  ⚠ --allow-remote: requests from outside the local network are answered")
	}

	if beacon, err := serveBeacon(set.peerName, set.peerPort); err != nil {
		fmt.Fprintln(os.Stderr, "  (not discoverable: "+err.Error()+" — other machines can still use the address above)")
	} else {
		defer beacon.Close()
	}

	// Ctrl-C and SIGTERM are the way this ends; the deferred closes above are
	// what make that tidy rather than abrupt.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	fmt.Println("\nstopped")
}

// storeFor picks the server's store for a scope — peerServer's own, since it
// has no model to borrow one from.
func (ps *peerServer) storeFor(s scope) *store {
	if s == scopeGlobal {
		return ps.global
	}
	return ps.project
}

// cliStore opens the backlog a command-line operation acts on: the global one,
// or this directory's project — erroring out where the manager would simply
// show an empty list, because a command that silently did nothing would be
// worse than one that says there is no backlog here.
func cliStore(global bool) *store {
	if global {
		path, err := globalTodosPath()
		if err != nil {
			errExit(err.Error())
		}
		return &store{scope: scopeGlobal, path: path}
	}
	wd, err := os.Getwd()
	if err != nil {
		errExit(err.Error())
	}
	root := findProjectRoot(wd)
	if root == "" {
		errExit("no project backlog here — run this inside a project, or pass -g")
	}
	return &store{scope: scopeProject, path: projectTodosPath(root)}
}

// openTodos is the prompts that are still work — the default subject of a
// command-line export, since "send me the backlog" almost never means the
// finished half of it.
func openTodos(todos []Todo) []Todo {
	out := make([]Todo, 0, len(todos))
	for _, t := range todos {
		if !t.closed() {
			out = append(out, t)
		}
	}
	return out
}

// writeText writes the markdown rendering beside where a bundle would have
// gone, never overwriting (uniqueName, the same rule writeBundle keeps).
func writeText(dir, name, body string) (string, error) {
	if !isDir(dir) {
		return "", fmt.Errorf("%s is not a directory", shortenHome(dir))
	}
	path := dir + string(os.PathSeparator) + uniqueName(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
