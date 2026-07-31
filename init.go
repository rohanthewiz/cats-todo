// init.go — `cats-todo init`: give a project its own backlog file, and the
// one-time offer to do so when the plugin is first installed.
//
// A project backlog is committed with the repo (see README), which is what
// makes init worth having as an explicit step rather than a side effect of the
// first save: creating .cats-todo/todos.json adds a file to someone's version
// control, and that should be a decision, not a surprise.
//
// It also makes init destructive in a way the rest of the tool never is —
// re-initializing a project that already has a backlog would throw away
// prompts, and a cloned repo now *arrives* carrying its author's backlog. So
// the existing-backlog path never writes without showing what is at stake
// (count + the first few titles) and getting an explicit yes.

package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/term"
)

// initSampleSize is how many todo titles the "you are about to replace this"
// summary lists before falling back to "…and N more". Three is enough to
// recognize a backlog you have seen before without turning a confirmation
// prompt into a scrolling list.
const initSampleSize = 3

// installOfferMarkerName records, inside the global config dir, that the
// install-time offer has already been made. See offerAlreadyMade for why the
// marker exists rather than inferring "fresh" from the backlogs themselves.
const installOfferMarkerName = ".install-offered"

// initEnv is everything the init logic needs from the outside world. Gathered
// into a struct so the decision path — especially "would this have wiped a
// backlog?" — is testable without a terminal attached.
type initEnv struct {
	in  io.Reader // where a y/N answer is read from
	out io.Writer // where summaries and results are written
	// interactive reports whether in is a terminal we may block on for an
	// answer. False for a pipe, a closed stdin, or a plugin build step (which
	// the cats host runs with no stdin at all) — in which case a prompt would
	// silently read EOF, and treating that as an answer is exactly the failure
	// mode this file exists to prevent.
	interactive bool
	// force replaces an existing backlog without asking. The escape hatch for
	// scripts and for a non-interactive shell, where declining is otherwise
	// the only possible outcome.
	force bool
}

// stdinInitEnv builds the real environment: stdin/stdout, prompting only when
// stdin is an interactive terminal.
func stdinInitEnv(force bool) initEnv {
	return initEnv{in: os.Stdin, out: os.Stdout, interactive: stdinIsTerminal(), force: force}
}

// stdinIsTerminal reports whether stdin is a real terminal we can hold a
// conversation with.
//
// This asks the kernel (a termios query) rather than checking for a character
// device the way the piped-stdin helpers elsewhere do. The two are not the same
// question: /dev/null is a character device but answers nothing, and it is
// exactly what a child process inherits when its parent leaves stdin unset —
// the plugin-host build step case. Treating that as a terminal would mean
// prompting into the void and reading the resulting EOF as an answer.
func stdinIsTerminal() bool {
	return term.IsTerminal(os.Stdin.Fd())
}

// initFromCLI implements `cats-todo init [-f] [--post-install]`. It creates the
// backlog for the project that owns the current directory — the same root the
// manager and `add` resolve, so all three agree on which project you are in.
func initFromCLI(args []string) {
	fs := flag.NewFlagSet("cats-todo init", flag.ExitOnError)
	force := fs.Bool("f", false, "replace an existing backlog without asking (destructive)")
	fs.BoolVar(force, "force", false, "alias for -f")
	postInstall := fs.Bool("post-install", false, "install-time mode: offer once, stay quiet on an upgrade")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cats-todo init [-f]")
		fmt.Fprintln(os.Stderr, "  creates .cats-todo/todos.json for the project that owns this directory")
		fmt.Fprintln(os.Stderr, "  the backlog is committed with the repo, so it travels to everyone who clones it")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args) // ExitOnError: a bad flag prints usage and exits

	env := stdinInitEnv(*force)
	if *postInstall {
		runInstallOffer(env)
		return
	}

	root, err := initTargetRoot()
	if err != nil {
		errExit(err)
	}
	if err := initProject(root, env); err != nil {
		errExit(err)
	}
}

// initTargetRoot resolves which directory gets the backlog: the project root
// above the current directory, so running init from a subdirectory initializes
// the project rather than the subdirectory. An empty root (the filesystem root
// owns no project — see findProjectRoot) is refused rather than turned into a
// /.cats-todo nobody can write to.
func initTargetRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("could not determine the working directory: %w", err)
	}
	root := findProjectRoot(wd)
	if root == "" {
		return "", fmt.Errorf("no project here — run init from a project directory (the global backlog needs no init)")
	}
	return root, nil
}

// initProject creates root's backlog file, and is the only place in cats-todo
// that can destroy todos wholesale. The order below is the safety contract:
//
//	existing todos → summarize → require an explicit yes → only then write
//
// An empty or missing file is not a loss, so it is created without ceremony.
// Declining is a normal outcome, not an error: the existing backlog is kept and
// init reports what it left alone.
func initProject(root string, env initEnv) error {
	path := projectTodosPath(root)
	if path == "" {
		return fmt.Errorf("no project directory to initialize")
	}

	existing := &store{scope: scopeProject, path: path}
	if err := existing.load(); err != nil {
		// An unreadable/corrupt backlog is the one case where we cannot say what
		// replacing it would cost, so we refuse to guess. -f still overrides.
		if !env.force {
			return fmt.Errorf("%s exists but could not be read (%w) — inspect it, or re-run with -f to replace it", path, err)
		}
	}

	replaced := existing.todos
	if len(replaced) > 0 {
		fmt.Fprint(env.out, backlogSummary(root, replaced))
		if !env.force {
			ok, err := askYesNo(env, fmt.Sprintf("Replace it with an empty backlog? This deletes those %s%s. [y/N] ",
				countTodos(len(replaced)), attachmentNote(replaced)))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintf(env.out, "kept %s unchanged\n", path)
				return nil
			}
		}
	}

	fresh := &store{scope: scopeProject, path: path, todos: []Todo{}}
	if err := fresh.save(); err != nil {
		return fmt.Errorf("could not create %s: %w", path, err)
	}
	// Attachments go only after the save succeeds, matching store.delete: a
	// backlog still listing a todo whose images are already gone is the worse
	// of the two failures. Skipping this would leave every replaced todo's
	// images/<id>/ directory orphaned — invisible to the UI, and committed
	// alongside a backlog that no longer references them.
	for _, t := range replaced {
		fresh.removeImages(t.ID)
	}
	fmt.Fprintf(env.out, "created %s — this project's backlog\n", path)
	fmt.Fprintf(env.out, "it is committed with the repo, so `git add %s` when you are ready\n", projectConfigDirName)
	return nil
}

// backlogSummary describes a backlog about to be replaced: the total, then the
// first few titles in backlog order (which is priority order — see store.move —
// so the sample is the top of the list, not an arbitrary slice of it).
func backlogSummary(root string, todos []Todo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s already has a backlog: %s\n", baseName(root), countTodos(len(todos)))
	shown := min(len(todos), initSampleSize)
	for _, t := range todos[:shown] {
		fmt.Fprintf(&b, "  · %s\n", todoLabel(t))
	}
	if rest := len(todos) - shown; rest > 0 {
		fmt.Fprintf(&b, "  …and %d more\n", rest)
	}
	return b.String()
}

// todoLabel is the one-line name for a todo in the summary: its title, falling
// back to the prompt's first line for an entry saved without one.
func todoLabel(t Todo) string {
	if s := strings.TrimSpace(t.Title); s != "" {
		return truncate(s, 60)
	}
	if s := firstLine(t.Prompt, 60); s != "" {
		return s
	}
	return "(untitled)"
}

// attachmentNote adds ", and their attachments" to the confirmation when any of
// the todos about to go owns files. Deleting a prompt is one thing; deleting
// screenshots the user copied in (the originals may well be off their Desktop
// by now — see images.go) is worth naming before they answer.
func attachmentNote(todos []Todo) string {
	for _, t := range todos {
		if len(t.Images) > 0 {
			return ", and their attachments"
		}
	}
	return ""
}

// countTodos renders a todo count with its noun already agreed ("1 todo",
// "12 todos"), so callers can drop it mid-sentence.
func countTodos(n int) string {
	if n == 1 {
		return "1 todo"
	}
	return fmt.Sprintf("%d todos", n)
}

// askYesNo puts question to the user and reports whether they said yes.
// Anything other than y/yes is no — the callers are destructive, so the default
// (a bare enter) must be the one that changes nothing.
//
// A non-interactive environment never gets asked: reading a prompt off a closed
// stdin returns EOF instantly, and silently reading that as an answer is how a
// backlog gets deleted by a script nobody thought was answering questions.
func askYesNo(env initEnv, question string) (bool, error) {
	if !env.interactive {
		return false, fmt.Errorf("refusing to replace an existing backlog with nothing to confirm at " +
			"(stdin is not a terminal) — re-run with -f to replace it, or leave it and start using it: cats-todo -p")
	}
	fmt.Fprint(env.out, question)
	line, err := bufio.NewReader(env.in).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		// EOF with nothing typed (^D) reads as "no", matching the bare-enter default.
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// runInstallOffer is the install-time entry point (`init --post-install`),
// wired as a build step in cats-plugin.toml. It offers to initialize a backlog
// for the project the install was run from — once, on a first install only.
//
// "Once" is the whole design. An upgrade re-runs every build step, so without a
// marker every `catctl plugin update` would re-ask a question the user already
// answered, in whatever directory they happened to be standing in. The marker
// lives in the global config dir because that is the one location shared across
// every project and every installed copy.
//
// Two things it deliberately will not do:
//
//   - prompt without a terminal. The cats plugin host runs build steps with no
//     stdin and with the working directory set to the *plugin* root, so today
//     this path prints guidance instead of asking. It upgrades to a real prompt
//     on its own if the host ever passes a terminal and the invoking directory
//     (CATS_TODO_INSTALL_CWD) through.
//   - initialize the plugin's own checkout. Without the host's help the
//     working directory is the plugin root, and a backlog there belongs to
//     cats-todo's repo, not to the user's project.
func runInstallOffer(env initEnv) {
	marker, err := installOfferMarkerPath()
	if err != nil {
		// No resolvable config dir (no home) — nothing to record the offer in, so
		// asking could repeat forever. Stay silent; `cats-todo init` still works.
		return
	}
	if offerAlreadyMade(marker) {
		return // an upgrade, or an existing user: they have been here before
	}
	// Recorded before the offer runs, not after: a user who quits at the prompt
	// has still been asked, and the answer to "ask again next upgrade?" is no.
	markOfferMade(marker)

	root := installOfferRoot()
	if root == "" || !env.interactive {
		fmt.Fprintf(env.out, "cats-todo: to keep a backlog with a project (committed with the repo), run:\n    cd <your project> && cats-todo init\n")
		return
	}
	fmt.Fprintf(env.out, "cats-todo keeps a per-project backlog in %s/, committed with the repo.\n", projectConfigDirName)
	ok, err := askYesNo(env, fmt.Sprintf("Initialize one for %s now? [y/N] ", baseName(root)))
	if err != nil || !ok {
		fmt.Fprintln(env.out, "skipped — run `cats-todo init` in any project to create one later")
		return
	}
	if err := initProject(root, env); err != nil {
		// An install must not fail over an optional convenience.
		fmt.Fprintf(env.out, "cats-todo: could not initialize here: %v\n", err)
	}
}

// installOfferRoot is the project the install-time offer targets: the directory
// the user invoked the installer from, which only the host can tell us
// (CATS_TODO_INSTALL_CWD), falling back to the process's own directory for a
// hand-run `cats-todo init --post-install`. Empty when that resolves to no
// project or to a plugin checkout — see runInstallOffer.
func installOfferRoot() string {
	dir := firstNonEmpty(os.Getenv(installCwdEnvVar), os.Getenv(hostInstallCwdEnvVar))
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return ""
		}
		dir = wd
	}
	if isPluginRoot(dir) {
		return ""
	}
	return findProjectRoot(dir)
}

// installCwdEnvVar and hostInstallCwdEnvVar are how the invoking directory
// reaches us: build steps run in the plugin root, so without one of these it is
// simply not knowable from inside a build step.
//
// hostInstallCwdEnvVar is what the cats plugin host exports to every build step
// (plugin.InstallCwdEnvVar). The cats-todo-specific name takes precedence so a
// wrapper script or a hand-run `init --post-install` can aim the offer at a
// directory of its choosing without impersonating the host.
const (
	installCwdEnvVar     = "CATS_TODO_INSTALL_CWD"
	hostInstallCwdEnvVar = "CATS_PLUGIN_INSTALL_CWD"
)

// isPluginRoot reports whether dir is a plugin checkout (it carries a plugin
// manifest) rather than a project someone wants a backlog in.
func isPluginRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "cats-plugin.toml"))
	return err == nil
}

// installOfferMarkerPath is the marker file's path inside the global config dir.
func installOfferMarkerPath() (string, error) {
	base, err := configBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, installOfferMarkerName), nil
}

// offerAlreadyMade reports whether we have already offered — or whether the
// user was already a cats-todo user before the offer existed.
//
// The second clause is what keeps an upgrade quiet for everyone who installed
// before this feature shipped: they have no marker, but they do have a global
// config dir (created the first time cats-todo saved anything), and asking a
// long-time user to "initialize" as if they were new would be noise.
func offerAlreadyMade(marker string) bool {
	if _, err := os.Stat(marker); err == nil {
		return true
	}
	if fi, err := os.Stat(filepath.Dir(marker)); err == nil && fi.IsDir() {
		return true
	}
	return false
}

// markOfferMade records that the offer has happened. Best-effort: a failure to
// write the marker must not block the offer or the install — the worst case is
// being asked once more on the next upgrade.
func markOfferMade(marker string) {
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(marker, []byte("cats-todo asked about a project backlog here\n"), 0o644)
}
