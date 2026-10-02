package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// offerModel is a manager launched in a fresh repo with no .cats-todo: the
// project store points at the path the offer would create, the global store at
// a temp file of its own.
func offerModel(t *testing.T, scope launchScope) (model, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(configDirEnvVar, filepath.Join(dir, "config"))
	repo := filepath.Join(dir, "repo")
	mkdir(t, filepath.Join(repo, ".git"))
	project := &store{scope: scopeProject, path: projectTodosPath(repo)}
	globalPath := filepath.Join(dir, "global", "todos.json")
	if scope == launchProjectOnly {
		globalPath = ""
	}
	global := &store{scope: scopeGlobal, path: globalPath}
	ctx := RunContext{WorkDir: repo, ProjectRoot: repo, Scope: scope}
	return newModel(ctx, project, global, nil).offerCreateBacklog(), repo
}

// TestOfferCreateBacklogOpensOnMissingBacklog: a project root with no
// .cats-todo opens on the confirm, naming the path it would create and why the
// walk stopped at the repo root.
func TestOfferCreateBacklogOpensOnMissingBacklog(t *testing.T) {
	m, repo := offerModel(t, launchBoth)
	if m.stage != stageConfirm || m.confirmKind != confirmCreateBacklog {
		t.Fatalf("stage = %v kind = %v, want the create-backlog confirm", m.stage, m.confirmKind)
	}
	m.width = 200
	view := m.viewConfirm()
	if !strings.Contains(view, shortenHome(projectTodosPath(repo))) {
		t.Errorf("confirm does not name the file it would create:\n%s", view)
	}
	if !strings.Contains(view, "repo root") {
		t.Errorf("confirm does not explain the repo-root boundary:\n%s", view)
	}
}

// TestOfferCreateBacklogSkipped: an existing backlog, a --global launch and a
// launch with no project at all never ask.
func TestOfferCreateBacklogSkipped(t *testing.T) {
	t.Run("existing backlog", func(t *testing.T) {
		t.Setenv(configDirEnvVar, filepath.Join(t.TempDir(), "config"))
		repo := t.TempDir()
		mkdir(t, filepath.Join(repo, ".git"))
		mkdir(t, filepath.Join(repo, projectConfigDirName))
		project := &store{scope: scopeProject, path: projectTodosPath(repo)}
		m := newModel(RunContext{WorkDir: repo, ProjectRoot: repo}, project, &store{scope: scopeGlobal}, nil)
		if got := m.offerCreateBacklog(); got.stage != stageList {
			t.Errorf("stage = %v, want the list when .cats-todo exists", got.stage)
		}
	})
	t.Run("global-only launch", func(t *testing.T) {
		m, _ := offerModel(t, launchGlobalOnly)
		if m.stage != stageList {
			t.Errorf("stage = %v, want the list on a --global launch", m.stage)
		}
	})
	t.Run("no project", func(t *testing.T) {
		m := newModel(RunContext{}, &store{scope: scopeProject}, &store{scope: scopeGlobal}, nil)
		if got := m.offerCreateBacklog(); got.stage != stageList {
			t.Errorf("stage = %v, want the list with no project store", got.stage)
		}
	})
}

// TestOfferCreateBacklogYes: y writes an empty `[]` backlog (not `null`) and
// lands on the list with the project backlog in use.
func TestOfferCreateBacklogYes(t *testing.T) {
	m, repo := offerModel(t, launchBoth)
	next, _ := m.updateConfirm(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next.(model)
	if m.stage != stageList {
		t.Fatalf("stage = %v, want the list after accepting", m.stage)
	}
	data, err := os.ReadFile(projectTodosPath(repo))
	if err != nil {
		t.Fatalf("backlog not created: %v", err)
	}
	if strings.TrimSpace(string(data)) != "[]" {
		t.Errorf("new backlog = %q, want an empty array", data)
	}
	if !m.project.available() {
		t.Error("project store unavailable after creating it")
	}
}

// TestOfferCreateBacklogNo: n/esc creates nothing and leaves the launch on the
// global backlog alone — and a later save cannot create .cats-todo behind the
// user's back, because the project store has no path any more.
func TestOfferCreateBacklogNo(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: 'n', Text: "n"}, {Code: tea.KeyEscape}} {
		m, repo := offerModel(t, launchBoth)
		next, _ := m.updateConfirm(key)
		m = next.(model)
		if m.stage != stageList {
			t.Fatalf("%s: stage = %v, want the list after declining", key, m.stage)
		}
		if m.project.available() {
			t.Errorf("%s: project store still available after declining", key)
		}
		if _, err := os.Stat(filepath.Join(repo, projectConfigDirName)); !os.IsNotExist(err) {
			t.Errorf("%s: .cats-todo exists after declining (err=%v)", key, err)
		}
		if got := m.scopeNote(-1); got != "global only" {
			t.Errorf("%s: scopeNote = %q, want \"global only\"", key, got)
		}
	}
}

// TestOfferCreateBacklogNoOnProjectOnly: declining on a --project launch leaves
// no backlog at all, and the add form refuses rather than swallowing a prompt.
func TestOfferCreateBacklogNoOnProjectOnly(t *testing.T) {
	m, _ := offerModel(t, launchProjectOnly)
	next, _ := m.updateConfirm(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = next.(model)
	if got := m.scopeNote(-1); got != "no backlog here" {
		t.Errorf("scopeNote = %q, want \"no backlog here\"", got)
	}
	next, _ = m.beginAdd()
	if next.(model).stage == stageForm {
		t.Error("add form opened with no writable backlog")
	}
}
