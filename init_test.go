package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// initTestEnv returns an init environment answering with the given input, plus
// the buffer its output lands in. answer "" means stdin is not a terminal (the
// plugin-build / piped case), which the caller uses to pin the never-prompt
// contract.
func initTestEnv(answer string, force bool) (initEnv, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return initEnv{
		in:          strings.NewReader(answer),
		out:         out,
		interactive: answer != "",
		force:       force,
	}, out
}

// seedBacklog writes n todos into root's backlog and returns its path.
func seedBacklog(t *testing.T, root string, titles ...string) string {
	t.Helper()
	path := projectTodosPath(root)
	s := &store{scope: scopeProject, path: path}
	for _, title := range titles {
		s.todos = append(s.todos, Todo{ID: newID(), Title: title, Prompt: title, Created: time.Now()})
	}
	if err := s.save(); err != nil {
		t.Fatalf("seeding %s: %v", path, err)
	}
	return path
}

// loadBacklog reads root's backlog back off disk.
func loadBacklog(t *testing.T, root string) []Todo {
	t.Helper()
	s := &store{scope: scopeProject, path: projectTodosPath(root)}
	if err := s.load(); err != nil {
		t.Fatalf("loading backlog: %v", err)
	}
	return s.todos
}

func TestInitProjectCreatesBacklog(t *testing.T) {
	root := t.TempDir()
	env, out := initTestEnv("", false)

	if err := initProject(root, env); err != nil {
		t.Fatalf("initProject = %v, want nil", err)
	}

	path := projectTodosPath(root)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading created backlog: %v", err)
	}
	// An empty JSON array, not "null": the file is committed, so a human reads
	// it in a diff and a later `add` appends to it.
	if got := strings.TrimSpace(string(data)); got != "[]" {
		t.Errorf("created backlog = %q, want %q", got, "[]")
	}
	if !strings.Contains(out.String(), path) {
		t.Errorf("output %q does not name the created path %q", out.String(), path)
	}
}

// TestInitProjectNonInteractiveNeverWipes is the core safety contract: with no
// terminal to confirm at, an existing backlog survives and init fails loudly
// rather than quietly reading EOF as consent.
func TestInitProjectNonInteractiveNeverWipes(t *testing.T) {
	root := t.TempDir()
	seedBacklog(t, root, "keep me", "and me")
	env, out := initTestEnv("", false)

	err := initProject(root, env)
	if err == nil {
		t.Fatal("initProject over an existing backlog with no terminal = nil, want an error")
	}
	if got := loadBacklog(t, root); len(got) != 2 {
		t.Fatalf("backlog has %d todos after a refused init, want 2 (it was wiped)", len(got))
	}
	// The user still learns what is there, so the refusal is actionable.
	if !strings.Contains(out.String(), "keep me") {
		t.Errorf("output %q does not summarize the existing backlog", out.String())
	}
}

func TestInitProjectDeclinedKeepsBacklog(t *testing.T) {
	root := t.TempDir()
	seedBacklog(t, root, "one", "two", "three")
	env, out := initTestEnv("n\n", false)

	if err := initProject(root, env); err != nil {
		t.Fatalf("declining init = %v, want nil (declining is not a failure)", err)
	}
	if got := loadBacklog(t, root); len(got) != 3 {
		t.Fatalf("backlog has %d todos after declining, want 3", len(got))
	}
	if !strings.Contains(out.String(), "kept") {
		t.Errorf("output %q does not report that the backlog was kept", out.String())
	}
}

// TestInitProjectBareEnterKeepsBacklog pins the default: the destructive answer
// must never be the one you get by holding enter.
func TestInitProjectBareEnterKeepsBacklog(t *testing.T) {
	root := t.TempDir()
	seedBacklog(t, root, "one")
	env, _ := initTestEnv("\n", false)

	if err := initProject(root, env); err != nil {
		t.Fatalf("initProject = %v, want nil", err)
	}
	if got := loadBacklog(t, root); len(got) != 1 {
		t.Fatalf("backlog has %d todos after a bare enter, want 1", len(got))
	}
}

func TestInitProjectConfirmedReplacesBacklog(t *testing.T) {
	root := t.TempDir()
	seedBacklog(t, root, "one", "two")
	env, _ := initTestEnv("y\n", false)

	if err := initProject(root, env); err != nil {
		t.Fatalf("initProject = %v, want nil", err)
	}
	if got := loadBacklog(t, root); len(got) != 0 {
		t.Fatalf("backlog has %d todos after a confirmed replace, want 0", len(got))
	}
}

func TestInitProjectForceReplacesWithoutAsking(t *testing.T) {
	root := t.TempDir()
	seedBacklog(t, root, "one", "two")
	env, out := initTestEnv("", true) // -f, and no terminal at all

	if err := initProject(root, env); err != nil {
		t.Fatalf("forced init = %v, want nil", err)
	}
	if got := loadBacklog(t, root); len(got) != 0 {
		t.Fatalf("backlog has %d todos after -f, want 0", len(got))
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Errorf("-f asked for confirmation: %q", out.String())
	}
}

// TestInitProjectEmptyBacklogNeedsNoConfirmation: an existing file with no
// todos is nothing to lose, so re-initializing it is not a destructive act.
func TestInitProjectEmptyBacklogNeedsNoConfirmation(t *testing.T) {
	root := t.TempDir()
	seedBacklog(t, root) // creates the file with zero todos
	env, out := initTestEnv("", false)

	if err := initProject(root, env); err != nil {
		t.Fatalf("initProject over an empty backlog = %v, want nil", err)
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Errorf("an empty backlog triggered a confirmation prompt: %q", out.String())
	}
}

func TestBacklogSummary(t *testing.T) {
	todos := []Todo{
		{Title: "fix the flaky reconnect"},
		{Title: "port the drop picker"},
		{Title: "document the socket"},
		{Title: "fourth"},
		{Prompt: "fifth, from the prompt"},
	}
	got := backlogSummary("/tmp/foo", todos)

	for _, want := range []string{"foo", "5 todos", "fix the flaky reconnect", "port the drop picker", "document the socket", "…and 2 more"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "fourth") {
		t.Errorf("summary %q listed past the first %d", got, initSampleSize)
	}
}

func TestBacklogSummaryShortAndUntitled(t *testing.T) {
	got := backlogSummary("/tmp/foo", []Todo{{Prompt: "  \n derived from the prompt \n more"}})
	if !strings.Contains(got, "1 todo\n") {
		t.Errorf("summary %q does not say %q", got, "1 todo")
	}
	if strings.Contains(got, "more") && strings.Contains(got, "…and") {
		t.Errorf("summary %q claims more todos than it has", got)
	}
	if !strings.Contains(got, "derived from the prompt") {
		t.Errorf("summary %q did not fall back to the prompt for an untitled todo", got)
	}
}

// TestInstallOfferRunsOnceThenStaysQuiet pins the fresh-install-vs-upgrade
// rule: the first install asks (or explains), and every later upgrade — which
// re-runs the same build step — says nothing.
func TestInstallOfferRunsOnceThenStaysQuiet(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cfg") // must not exist yet: that is what "fresh" means
	t.Setenv(configDirEnvVar, cfg)

	env, out := initTestEnv("", false)
	runInstallOffer(env)
	if out.Len() == 0 {
		t.Fatal("a fresh install said nothing about the project backlog")
	}

	env2, out2 := initTestEnv("", false)
	runInstallOffer(env2)
	if out2.Len() != 0 {
		t.Errorf("the upgrade re-asked: %q", out2.String())
	}
}

// TestInstallOfferQuietForExistingUsers: someone who was using cats-todo before
// the offer existed has a config dir but no marker, and must not be greeted as
// a new user on their next upgrade.
func TestInstallOfferQuietForExistingUsers(t *testing.T) {
	cfg := t.TempDir() // exists = prior use
	t.Setenv(configDirEnvVar, cfg)

	env, out := initTestEnv("", false)
	runInstallOffer(env)
	if out.Len() != 0 {
		t.Errorf("an existing user was prompted on upgrade: %q", out.String())
	}
}

func TestOfferAlreadyMade(t *testing.T) {
	base := filepath.Join(t.TempDir(), "cfg")
	marker := filepath.Join(base, installOfferMarkerName)

	if offerAlreadyMade(marker) {
		t.Error("offerAlreadyMade with no config dir = true, want false (this is a fresh install)")
	}
	markOfferMade(marker)
	if !offerAlreadyMade(marker) {
		t.Error("offerAlreadyMade after marking = false, want true")
	}
}

// TestIsPluginRoot guards the install-time footgun: build steps run in the
// plugin checkout, which must never be mistaken for a project to initialize.
func TestIsPluginRoot(t *testing.T) {
	dir := t.TempDir()
	if isPluginRoot(dir) {
		t.Error("a plain directory reported as a plugin root")
	}
	if err := os.WriteFile(filepath.Join(dir, "cats-plugin.toml"), []byte("id = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isPluginRoot(dir) {
		t.Error("a directory with cats-plugin.toml not reported as a plugin root")
	}
}

// TestInstallOfferRootPrefersHostCwd: the host tells us where the user was
// standing (a build step's own cwd is the plugin checkout), and the
// cats-todo-specific override wins over it.
func TestInstallOfferRootPrefersHostCwd(t *testing.T) {
	project := t.TempDir()
	other := t.TempDir()

	t.Setenv(hostInstallCwdEnvVar, project)
	t.Setenv(installCwdEnvVar, "")
	if got := installOfferRoot(); got != project {
		t.Errorf("installOfferRoot = %q, want the host-provided %q", got, project)
	}

	t.Setenv(installCwdEnvVar, other)
	if got := installOfferRoot(); got != other {
		t.Errorf("installOfferRoot = %q, want the override %q", got, other)
	}
}

// TestInstallOfferSkipsPluginCheckout: a build step runs in the plugin root,
// and a backlog created there would belong to cats-todo's own repo.
func TestInstallOfferSkipsPluginCheckout(t *testing.T) {
	checkout := t.TempDir()
	if err := os.WriteFile(filepath.Join(checkout, "cats-plugin.toml"), []byte("id = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(hostInstallCwdEnvVar, checkout)
	t.Setenv(installCwdEnvVar, "")

	if got := installOfferRoot(); got != "" {
		t.Errorf("installOfferRoot in a plugin checkout = %q, want \"\"", got)
	}
}

// TestInitProjectReplaceRemovesAttachments: a replaced todo's images/<id>/
// directory must go with it, or the backlog is committed alongside binaries
// nothing references.
func TestInitProjectReplaceRemovesAttachments(t *testing.T) {
	root := t.TempDir()
	seedBacklog(t, root, "has an image")

	s := &store{scope: scopeProject, path: projectTodosPath(root)}
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	id := s.todos[0].ID
	imgDir := filepath.Join(s.imagesDir(), id)
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(imgDir, "shot.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.todos[0].Images = []string{filepath.Join("images", id, "shot.png")}
	if err := s.save(); err != nil {
		t.Fatal(err)
	}

	env, out := initTestEnv("y\n", false)
	if err := initProject(root, env); err != nil {
		t.Fatalf("initProject = %v, want nil", err)
	}
	if _, err := os.Stat(imgDir); !os.IsNotExist(err) {
		t.Errorf("attachment dir %s survived the replace (err %v)", imgDir, err)
	}
	// The user was told the files were at stake, not just the prompts.
	if !strings.Contains(out.String(), "attachments") {
		t.Errorf("confirmation %q did not mention the attachments", out.String())
	}
}
