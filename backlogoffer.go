// backlogoffer.go — the launch-time offer to create a project backlog.
//
// The root walk (walkProjectRoot) stops at the first repo root it meets, so a
// repo with no .cats-todo of its own no longer borrows an unrelated one from
// further up the tree. That leaves a state the manager has to say out loud: this
// project has no backlog yet. Rather than conjure .cats-todo/ on the first save
// — the file is meant to be committed (see init.go), so it should appear on
// request, not as a side effect — the manager opens on a confirm:
//
//	launch ─▶ project root has .cats-todo? ── yes ─▶ list (as before)
//	                     │
//	                     no
//	                     ▼
//	          "No backlog in <name>" confirm
//	             │ y / enter          │ n / esc
//	             ▼                    ▼
//	   create .cats-todo/todos.json   project store made unavailable:
//	   list shows it (empty)          global only (or "no backlog here"
//	                                  on a --project launch)
//
// Declining is per launch, not remembered: the question is cheap to answer and
// a remembered "no" would be one more hidden piece of state deciding which
// backlog is on screen — the exact confusion this change removes.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// offerCreateBacklog opens the manager on the create-backlog confirm when the
// launch is scoped to a project whose root has no .cats-todo yet. It is applied
// by runTodoUI after newModel rather than inside it, so every test model built
// on a temp path that does not exist yet still opens on the list.
//
// A --global launch never asks: it has no project store to create. Neither does
// a launch with no project at all (the filesystem root), whose store is
// already unavailable.
func (m model) offerCreateBacklog() model {
	if m.ctx.Scope == launchGlobalOnly || !m.project.available() {
		return m
	}
	if !projectBacklogMissing(backlogRoot(m.project)) {
		return m
	}
	m.confirmKind = confirmCreateBacklog
	m.stage = stageConfirm
	return m
}

// createProjectBacklog answers the offer with yes: write an empty todos.json,
// which creates .cats-todo/ on the way (store.save makes the directory). A
// failure — a read-only checkout, say — is reported and leaves the store as it
// was, so the backlog is still offered implicitly by the first save, which will
// then fail with the same words in the status line.
func (m *model) createProjectBacklog() {
	if m.project.todos == nil {
		// nil would marshal as `null`; an empty backlog is `[]`, the shape
		// init writes and every reader expects.
		m.project.todos = []Todo{}
	}
	if err := m.project.save(); err != nil {
		m.setStatus("could not create the backlog: "+err.Error(), true)
		return
	}
	m.setStatus(fmt.Sprintf("created %s — commit it with `git add %s` when you are ready",
		shortenHome(m.project.path), projectConfigDirName), false)
}

// declineProjectBacklog answers the offer with no: the project store becomes
// unavailable for this launch (an empty path, the state the header and the add
// form already render as "global only"), so nothing can create the directory
// behind the user's back by way of a save.
func (m *model) declineProjectBacklog() {
	root := backlogRoot(m.project)
	m.project = &store{scope: scopeProject}
	m.rebuildList()
	if !m.global.available() {
		// A --project launch: nothing left to show.
		m.setStatus("no backlog here — `cats-todo init` creates one, or relaunch with --global", true)
		return
	}
	m.setStatus(fmt.Sprintf("no backlog in %s — showing the global backlog only (`cats-todo init` creates one)",
		baseName(root)), false)
}

// viewCreateBacklog draws the offer. It names the full path it would create, so
// the answer is about a place, not a guess, and says why the walk stopped where
// it did when that was a repo root — the user who had a backlog further up the
// tree needs to hear that it is deliberately no longer used.
func (m model) viewCreateBacklog() string {
	root := backlogRoot(m.project)
	var b strings.Builder
	b.WriteString(titleStyle.Render("No backlog in " + firstNonEmpty(baseName(root), "this project")))
	b.WriteString("\n\n")
	b.WriteString(m.wrapToPane(nameStyle, "  "+shortenHome(root)+" has no "+projectConfigDirName+" directory."))
	b.WriteString("\n")
	if isRepoRoot(root) {
		b.WriteString(m.wrapToPane(descStyle, "  The search for a backlog stops at the repo root, so one further up the tree is not used here."))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.wrapToPane(nameStyle, "  Create "+shortenHome(m.project.path)+"?"))
	b.WriteString("\n\n")
	decline := "continue with the global backlog"
	if !m.global.available() {
		decline = "continue without a backlog"
	}
	b.WriteString(footerStyle.Render("y create · n / esc " + decline))
	return b.String()
}

// isRepoRoot reports whether dir is a checkout's root (.git as a directory, or
// the file a linked worktree or submodule has) — the same test the root walk
// stops on.
func isRepoRoot(dir string) bool {
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}
