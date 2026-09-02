package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// errTestDrop stands in for a drop failure in tests.
var errTestDrop = errors.New("drop failed")

// newModelInTemp builds a manager whose project and global backlogs are backed
// by fresh files under t.TempDir(), so form saves, toggles, and deletes actually
// persist somewhere isolated and auto-cleaned. The project store has a path, so
// available() is true — the in-project launch where scope defaults matter.
func newModelInTemp(t *testing.T) (model, *store, *store) {
	t.Helper()
	dir := t.TempDir()
	// newModel reads the saved preferences, and two of them (priority order,
	// frozen prompts) decide which rows are drawn at all. Without this the suite
	// would be answering to whatever the developer running it last toggled — so
	// the config directory is pointed somewhere empty and every test sees the
	// documented defaults.
	t.Setenv(configDirEnvVar, filepath.Join(dir, "config"))
	project := &store{scope: scopeProject, path: filepath.Join(dir, "project", "todos.json")}
	global := &store{scope: scopeGlobal, path: filepath.Join(dir, "global", "todos.json")}
	m := newModel(RunContext{WorkDir: filepath.Join(dir, "project")}, project, global, nil)
	return m, project, global
}

// TestSaveFormAddsTodo walks the add flow the way a keypress would: beginAdd
// enters the form, the prompt gets text, saveForm persists. It checks the todo
// reaches both the in-memory store and disk, that a blank title is derived from
// the prompt's first line, and that the UI returns to the list.
func TestSaveFormAddsTodo(t *testing.T) {
	m, project, _ := newModelInTemp(t)

	next, _ := m.beginAdd()
	m = next.(model)
	if m.stage != stageForm {
		t.Fatalf("beginAdd stage = %v, want stageForm", m.stage)
	}
	// In a project, a new todo defaults to the project backlog.
	if m.formScope != scopeProject {
		t.Errorf("formScope = %v, want scopeProject in an available project", m.formScope)
	}

	// Leave the title blank so it gets derived from the prompt's first line.
	m.promptArea.SetValue("Wire up the export button\nplus follow-up details")
	next, _ = m.saveForm()
	m = next.(model)

	if m.stage != stageList {
		t.Fatalf("after save stage = %v, want stageList", m.stage)
	}
	if len(project.todos) != 1 {
		t.Fatalf("project has %d todos, want 1", len(project.todos))
	}
	if got := project.todos[0].Title; got != "Wire up the export button" {
		t.Errorf("derived title = %q, want the prompt's first line", got)
	}

	// Confirm it persisted, not just mutated in memory.
	reloaded := &store{scope: scopeProject, path: project.path}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if len(reloaded.todos) != 1 || reloaded.todos[0].Prompt != "Wire up the export button\nplus follow-up details" {
		t.Errorf("disk todos = %+v, want the saved prompt", reloaded.todos)
	}
}

func TestSaveFormRejectsEmptyPrompt(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	next, _ := m.beginAdd()
	m = next.(model)

	// Only whitespace in the prompt — saveForm should refuse and stay on the form.
	m.promptArea.SetValue("   \n  ")
	next, _ = m.saveForm()
	m = next.(model)

	if m.stage != stageForm {
		t.Errorf("stage = %v, want to stay on stageForm for an empty prompt", m.stage)
	}
	if m.formErr == "" {
		t.Error("expected a formErr for an empty prompt")
	}
	if len(project.todos) != 0 {
		t.Errorf("an empty prompt added %d todos, want 0", len(project.todos))
	}
}

// TestBeginAddDefaultsToGlobalOutsideProject pins the scope default: with no
// project store available, a new todo can only go to the global backlog.
func TestBeginAddDefaultsToGlobalOutsideProject(t *testing.T) {
	project := &store{scope: scopeProject, path: ""} // unavailable: no project
	global := &store{scope: scopeGlobal, path: filepath.Join(t.TempDir(), "todos.json")}
	m := newModel(RunContext{}, project, global, nil)

	next, _ := m.beginAdd()
	m = next.(model)
	if m.formScope != scopeGlobal {
		t.Errorf("formScope = %v, want scopeGlobal when no project is available", m.formScope)
	}
}

// TestBeginAddRefusesWithNoBacklog covers the one launch where neither store is
// writable: --project from a directory no project owns (a pane at the filesystem
// root). An unavailable store's save is a silent no-op that reports success, so
// the form must not open at all — otherwise the user types a prompt, is told it
// was added, and it is gone.
func TestBeginAddRefusesWithNoBacklog(t *testing.T) {
	project := &store{scope: scopeProject, path: ""} // no project owns the cwd
	global := &store{scope: scopeGlobal, path: ""}   // --project withholds it
	m := newModel(RunContext{WorkDir: "/"}, project, global, nil)

	next, _ := m.beginAdd()
	m = next.(model)
	if m.stage != stageList {
		t.Errorf("stage = %v, want stageList — the form must not open with nowhere to write", m.stage)
	}
	if !m.statusErr || !strings.Contains(m.status, "no project backlog here") {
		t.Errorf("status = %q (err %v), want an error naming the missing backlog", m.status, m.statusErr)
	}

	// The list itself has to say so too: ctrl+a is not the way out of this
	// state, so the empty-list hint must not point at it.
	view := m.viewList()
	if !strings.Contains(view, "No backlog here") || strings.Contains(view, "press ctrl+a") {
		t.Errorf("viewList = %q, want the no-backlog hint instead of the ctrl+a one", view)
	}
	if !strings.Contains(view, "no backlog here") {
		t.Errorf("viewList = %q, want the header to read \"no backlog here\"", view)
	}
}

// TestSaveFormRefusesUnavailableStore is the backstop behind beginAdd's guard:
// reaching the form with an unavailable target must fail loudly rather than
// report a save that wrote nothing.
func TestSaveFormRefusesUnavailableStore(t *testing.T) {
	project := &store{scope: scopeProject, path: ""}
	global := &store{scope: scopeGlobal, path: ""}
	m := newModel(RunContext{WorkDir: "/"}, project, global, nil)

	// Enter the form the way beginAdd would if its guard were not there.
	m.formMode = formAdd
	m.formScope = scopeProject
	m.titleInput, m.promptArea = m.newFormInputs("", "")
	m.promptArea.SetValue("something worth keeping")
	m.stage = stageForm

	next, _ := m.saveForm()
	m = next.(model)
	if m.stage != stageForm {
		t.Errorf("stage = %v, want stageForm — a refused save stays in the form", m.stage)
	}
	if !strings.Contains(m.formErr, "no project backlog is available") {
		t.Errorf("formErr = %q, want it to name the unavailable backlog", m.formErr)
	}
	if len(project.todos) != 0 {
		t.Errorf("project has %d todos, want none saved", len(project.todos))
	}
}

// TestToggleSelected flips the highlighted todo's done flag and reports the right
// status, both directions — and keeps the highlight on the todo through both, so
// the key that closed a prompt is the key that reopens it. Completing one files
// it at the head of the done group, which can move the row most of a pane; a
// cursor left at the old index would put the second press on a prompt nobody
// chose, which is how a slipped ctrl+t turns into two mistakes.
func TestToggleSelected(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	for _, td := range []Todo{
		{ID: "x", Title: "task", Prompt: "do it"},
		{ID: "y", Title: "other", Prompt: "later"},
	} {
		if err := project.add(td); err != nil {
			t.Fatal(err)
		}
	}
	m.rebuildList() // pick up the new todos and park the cursor on the first

	next, _ := m.toggleSelected()
	m = next.(model)
	if got, _ := project.find("x"); !got.Done {
		t.Error("toggleSelected did not mark the todo done")
	}
	if !strings.Contains(m.status, "marked done") {
		t.Errorf("status = %q, want it to say the todo was marked done", m.status)
	}
	// The way back is named, since the row has just jumped into the done group —
	// or out of sight entirely with the closed fold on.
	if !strings.Contains(m.status, "ctrl+t") {
		t.Errorf("status = %q, want the way back named", m.status)
	}
	if ref, _ := m.selectedRef(); ref.id != "x" {
		t.Fatalf("highlight = %q after the todo was filed under done, want it to ride along", ref.id)
	}

	// So the second press lands on the same prompt rather than on whatever slid
	// up into the gap.
	next, _ = m.toggleSelected()
	m = next.(model)
	if got, _ := project.find("x"); got.Done {
		t.Error("second toggleSelected did not reopen the todo")
	}
	if other, _ := project.find("y"); other.Done {
		t.Error("the second press landed on the prompt that slid up into the gap")
	}
	if !strings.Contains(m.status, "reopened") {
		t.Errorf("status = %q, want it to say the todo was reopened", m.status)
	}
	if ref, _ := m.selectedRef(); ref.id != "x" {
		t.Errorf("highlight = %q after the reopen, want it still on the todo", ref.id)
	}
}

// TestDeleteConfirmFlow runs the two-step delete: beginDelete arms the confirm
// stage, then a "y" key carries it out and returns to the list.
func TestDeleteConfirmFlow(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	if err := project.add(Todo{ID: "gone", Title: "remove me", Prompt: "p"}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()

	next, _ := m.beginDelete()
	m = next.(model)
	if m.stage != stageConfirm {
		t.Fatalf("beginDelete stage = %v, want stageConfirm", m.stage)
	}
	if m.pendingTitle != "remove me" {
		t.Errorf("pendingTitle = %q, want 'remove me'", m.pendingTitle)
	}

	next, _ = m.updateConfirm(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next.(model)
	if m.stage != stageList {
		t.Errorf("after confirm stage = %v, want stageList", m.stage)
	}
	if _, ok := project.find("gone"); ok {
		t.Error("confirmed delete did not remove the todo")
	}
	if m.status != "deleted" {
		t.Errorf("status = %q, want 'deleted'", m.status)
	}
}

// TestDeleteConfirmCancel pins that answering "n" keeps the todo.
func TestDeleteConfirmCancel(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	if err := project.add(Todo{ID: "keep", Title: "keep me", Prompt: "p"}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()

	next, _ := m.beginDelete()
	m = next.(model)
	next, _ = m.updateConfirm(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = next.(model)
	if m.stage != stageList {
		t.Errorf("after cancel stage = %v, want stageList", m.stage)
	}
	if _, ok := project.find("keep"); !ok {
		t.Error("cancelling the confirm deleted the todo anyway")
	}
}

// hasHeading reports whether the rebuilt list contains a non-selectable group
// heading with the given name.
func hasHeading(items []listItem, name string) bool {
	for _, it := range items {
		if !it.selectable && it.name == name {
			return true
		}
	}
	return false
}

// TestRebuildListGroupsOnlyWhenBothScopesHaveTodos covers the grouping rule:
// "Project"/"Global" headings appear only when both backlogs are non-empty.
func TestRebuildListGroupsOnlyWhenBothScopesHaveTodos(t *testing.T) {
	t.Run("both populated shows headings", func(t *testing.T) {
		m, project, global := newModelInTemp(t)
		if err := project.add(Todo{ID: "p", Prompt: "proj todo"}); err != nil {
			t.Fatal(err)
		}
		if err := global.add(Todo{ID: "g", Prompt: "glob todo"}); err != nil {
			t.Fatal(err)
		}
		m.rebuildList()
		if !hasHeading(m.list.items, "Project") || !hasHeading(m.list.items, "Global") {
			t.Errorf("expected both group headings; items = %+v", m.list.items)
		}
		if len(m.rows) != 2 {
			t.Errorf("rows = %d, want 2 selectable todos", len(m.rows))
		}
	})

	t.Run("single scope shows no headings", func(t *testing.T) {
		m, _, global := newModelInTemp(t)
		if err := global.add(Todo{ID: "g", Prompt: "glob only"}); err != nil {
			t.Fatal(err)
		}
		m.rebuildList()
		if hasHeading(m.list.items, "Project") || hasHeading(m.list.items, "Global") {
			t.Errorf("expected no headings with a single populated scope; items = %+v", m.list.items)
		}
	})
}

// TestRebuildListSortsDoneToBottom pins the list order: within a scope, open
// todos keep their backlog (array) order and done todos sink below them.
func TestRebuildListSortsDoneToBottom(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	for _, td := range []Todo{
		{ID: "done-first", Prompt: "d", Done: true},
		{ID: "open1", Prompt: "o1"},
		{ID: "open2", Prompt: "o2"},
	} {
		if err := project.add(td); err != nil {
			t.Fatal(err)
		}
	}
	m.rebuildList()

	got := make([]string, len(m.rows))
	for i, r := range m.rows {
		got[i] = r.id
	}
	want := []string{"open1", "open2", "done-first"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows = %v, want %v (open first, done last)", got, want)
		}
	}
}

// TestHideClosedFoldsCompletedAndFrozen pins ctrl+d's fold: it still takes done
// *and* frozen todos out of the rows together, and lifting it brings both back.
//
// The fold is two flags now that the View panel gives frozen prompts a switch of
// their own (see toggleClosedFold), so this asserts through the key rather than
// through the flags — the key is the promise that has to survive the split.
func TestHideClosedFoldsCompletedAndFrozen(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	if err := project.add(Todo{ID: "open", Prompt: "o"}); err != nil {
		t.Fatal(err)
	}
	if err := project.add(Todo{ID: "done", Prompt: "d", Done: true}); err != nil {
		t.Fatal(err)
	}
	if err := project.add(Todo{ID: "frozen", Prompt: "f", Frozen: true}); err != nil {
		t.Fatal(err)
	}

	mm, _ := m.toggleClosedFold()
	m = mm.(model)
	if !m.hideDone || m.showFrozen {
		t.Fatalf("ctrl+d left hideDone=%v showFrozen=%v, want the pair folded", m.hideDone, m.showFrozen)
	}
	if len(m.rows) != 1 || m.rows[0].id != "open" {
		t.Errorf("hidden rows = %+v, want only the open todo", m.rows)
	}
	if m.hiddenClosedCount() != 2 {
		t.Errorf("hiddenClosedCount = %d, want 2 (done + frozen)", m.hiddenClosedCount())
	}
	// The clear-completed sweep counts only what it will actually delete.
	if m.doneCount() != 1 {
		t.Errorf("doneCount = %d, want 1 (frozen is not cleared)", m.doneCount())
	}

	mm, _ = m.toggleClosedFold()
	m = mm.(model)
	if m.hideDone || !m.showFrozen {
		t.Fatalf("a second ctrl+d left hideDone=%v showFrozen=%v, want both showing", m.hideDone, m.showFrozen)
	}
	if len(m.rows) != 3 {
		t.Errorf("unhidden rows = %+v, want all three todos back", m.rows)
	}
	// Render order within a scope: open, then frozen, then done.
	want := []string{"open", "frozen", "done"}
	for i, id := range want {
		if m.rows[i].id != id {
			t.Fatalf("row %d = %q, want %q (order: open, frozen, done)", i, m.rows[i].id, id)
		}
	}
}

// TestFilterMatchesDeepPromptLines pins the full-body search: a query that only
// appears past the first line of a multi-line prompt still matches.
func TestFilterMatchesDeepPromptLines(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	err := project.add(Todo{ID: "deep", Title: "refactor", Prompt: "clean up the store\nand also fix the flaky websocket reconnect"})
	if err != nil {
		t.Fatal(err)
	}
	if err := project.add(Todo{ID: "other", Title: "docs", Prompt: "update the readme"}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()

	m.list.input.SetValue("websocket")
	m.list.filter()
	idx := m.list.selectedIndex()
	if idx < 0 || m.rows[idx].id != "deep" {
		t.Errorf("filtering for a deep prompt line selected row %d, want the multi-line todo", idx)
	}
}

// TestMoveSelectedKeepsHighlight pins reordering: ctrl+down swaps the todo with
// its neighbor, persists the order, and the highlight follows the moved todo.
func TestMoveSelectedKeepsHighlight(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	for _, td := range []Todo{{ID: "a", Prompt: "a"}, {ID: "b", Prompt: "b"}} {
		if err := project.add(td); err != nil {
			t.Fatal(err)
		}
	}
	m.rebuildList() // cursor parks on the first row ("a")

	next, _ := m.moveSelected(1)
	m = next.(model)

	if project.todos[0].ID != "b" || project.todos[1].ID != "a" {
		t.Errorf("store order = %+v, want b then a", project.todos)
	}
	if ref, ok := m.selectedRef(); !ok || ref.id != "a" {
		t.Errorf("selected = %+v, want the highlight to follow the moved todo a", ref)
	}
}

// TestClearDoneConfirmFlow runs the bulk cleanup: ctrl+w arms the confirm stage
// with the done count, "y" removes completed todos from both scopes, and open
// todos survive. With nothing done, it short-circuits to a status message.
func TestClearDoneConfirmFlow(t *testing.T) {
	m, project, global := newModelInTemp(t)
	for _, td := range []Todo{{ID: "p-open", Prompt: "p"}, {ID: "p-done", Prompt: "p", Done: true}} {
		if err := project.add(td); err != nil {
			t.Fatal(err)
		}
	}
	if err := global.add(Todo{ID: "g-done", Prompt: "g", Done: true}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()

	next, _ := m.beginClearDone()
	m = next.(model)
	if m.stage != stageConfirm || m.confirmKind != confirmClearDone {
		t.Fatalf("beginClearDone stage/kind = %v/%v, want stageConfirm/confirmClearDone", m.stage, m.confirmKind)
	}
	if m.pendingClearCount != 2 {
		t.Errorf("pendingClearCount = %d, want 2", m.pendingClearCount)
	}

	next, _ = m.updateConfirm(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next.(model)
	if m.stage != stageList {
		t.Errorf("after confirm stage = %v, want stageList", m.stage)
	}
	if _, ok := project.find("p-done"); ok {
		t.Error("project's done todo survived the clear")
	}
	if _, ok := global.find("g-done"); ok {
		t.Error("global's done todo survived the clear")
	}
	if _, ok := project.find("p-open"); !ok {
		t.Error("the open todo was cleared; only done todos should go")
	}
	if !strings.Contains(m.status, "cleared 2") {
		t.Errorf("status = %q, want it to report clearing 2", m.status)
	}

	// A second clear finds nothing and never enters the confirm stage.
	next, _ = m.beginClearDone()
	m = next.(model)
	if m.stage != stageList {
		t.Errorf("empty clear stage = %v, want to stay on stageList", m.stage)
	}
	if !strings.Contains(m.status, "no completed") {
		t.Errorf("status = %q, want the nothing-to-clear note", m.status)
	}
}

// TestViewStageFlow covers the read-only prompt view: ctrl+v opens it on the
// highlighted todo, its rendering carries the full body, esc returns to the
// list, and enter hands off to the drop flow (which, socket-less here, lands
// back on the list with an error status).
func TestViewStageFlow(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	body := "first line\nsecond line with the details"
	if err := project.add(Todo{ID: "v", Title: "view me", Prompt: body}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()

	next, _ := m.beginView()
	m = next.(model)
	if m.stage != stageView {
		t.Fatalf("beginView stage = %v, want stageView", m.stage)
	}
	if got := m.View().Content; !strings.Contains(got, "second line with the details") {
		t.Errorf("view rendering lacks the prompt's later lines:\n%s", got)
	}

	next, _ = m.updateView(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)
	if m.stage != stageList {
		t.Errorf("esc from view stage = %v, want stageList", m.stage)
	}

	next, _ = m.beginView()
	m = next.(model)
	next, _ = m.updateView(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if m.stage != stageForm || m.editID != "v" {
		t.Errorf("enter from the view stage: stage=%v editID=%q, want the edit form on this todo", m.stage, m.editID)
	}

	next, _ = m.beginView()
	m = next.(model)
	next, _ = m.updateView(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	m = next.(model)
	if m.stage != stageList || !m.statusErr {
		t.Errorf("shift+enter (drop) without a control socket: stage=%v statusErr=%v, want stageList with an error status", m.stage, m.statusErr)
	}
}

// TestChooseTargetStaysOpen pins the persistent-pane behavior: choosing a drop
// target no longer quits the program. Instead it returns to the list, marks a
// drop in flight, shows a "dropping…" status, and hands back a command that will
// perform the drop off the UI thread. The pane lives on so more prompts can drop.
func TestChooseTargetStaysOpen(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	if err := project.add(Todo{ID: "d", Prompt: "drop me"}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()

	// Stand up the target picker the way beginDrop would, but without a control socket:
	// buildTargets degrades to just the new-session target (selected by default).
	m.dropTodo = todoRef{scope: scopeProject, id: "d"}
	m.targets, m.targetList = m.buildTargets()
	m.stage = stageTarget

	next, cmd := m.chooseTarget(dropPaste)
	m = next.(model)
	if m.quitting {
		t.Error("chooseTarget set quitting; the persistent pane must stay open")
	}
	if !m.dropping {
		t.Error("chooseTarget did not mark a drop in flight")
	}
	if m.stage != stageList {
		t.Errorf("stage = %v, want stageList after starting a drop", m.stage)
	}
	if m.status == "" || m.statusErr {
		t.Errorf("expected a non-error 'dropping…' status; got status=%q err=%v", m.status, m.statusErr)
	}
	if cmd == nil {
		t.Fatal("chooseTarget returned no command to perform the drop")
	}

	// The command performs the drop and reports back. With a nil socket the drop
	// fails, but the result still flows through Update, which clears the in-flight
	// flag and surfaces the error — leaving the manager usable.
	msg := cmd()
	res, ok := msg.(dropResultMsg)
	if !ok {
		t.Fatalf("drop command returned %T, want dropResultMsg", msg)
	}
	if res.err == nil {
		t.Error("expected the socket-less drop to fail")
	}
	next, _ = m.Update(res)
	m = next.(model)
	if m.dropping {
		t.Error("dropResultMsg did not clear the in-flight flag")
	}
	if !m.statusErr {
		t.Errorf("expected an error status after a failed drop; got status=%q", m.status)
	}
}

// TestDropMarksTodoDone pins the auto-complete behavior: the dropResultMsg
// handler closes the dropped todo out and notes it in the status, for both drop
// modes. Only a failed drop leaves the todo open.
func TestDropMarksTodoDone(t *testing.T) {
	t.Run("run drop marks done", func(t *testing.T) {
		m, project, _ := newModelInTemp(t)
		if err := project.add(Todo{ID: "r", Prompt: "run me"}); err != nil {
			t.Fatal(err)
		}
		m.rebuildList()

		// A run drop reports success for an existing pane; mark it done.
		res := dropResultMsg{desc: "claude", ref: todoRef{scope: scopeProject, id: "r"}}
		next, _ := m.Update(res)
		m = next.(model)

		if got, _ := project.find("r"); !got.Done {
			t.Error("a successful run drop did not mark the todo done")
		}
		if !strings.Contains(m.status, "marked done") {
			t.Errorf("status = %q, want it to note 'marked done'", m.status)
		}
		// It must persist, not just mutate in memory.
		reloaded := &store{scope: scopeProject, path: project.path}
		if err := reloaded.load(); err != nil {
			t.Fatal(err)
		}
		if got, _ := reloaded.find("r"); !got.Done {
			t.Error("the auto-complete did not persist to disk")
		}
	})

	// A paste drop delivers the prompt too (just unsubmitted), so it closes the
	// todo out the same way — the backlog shouldn't hold a duplicate of work that
	// is already sitting in an agent's input.
	t.Run("paste drop marks done", func(t *testing.T) {
		m, project, _ := newModelInTemp(t)
		if err := project.add(Todo{ID: "p", Prompt: "paste me"}); err != nil {
			t.Fatal(err)
		}
		m.rebuildList()

		res := dropResultMsg{desc: "claude", ref: todoRef{scope: scopeProject, id: "p"}}
		next, _ := m.Update(res)
		m = next.(model)

		if got, _ := project.find("p"); !got.Done {
			t.Error("a successful paste drop did not mark the todo done")
		}
		if !strings.Contains(m.status, "marked done") {
			t.Errorf("status = %q, want it to note 'marked done'", m.status)
		}
	})

	t.Run("failed run drop leaves it open", func(t *testing.T) {
		m, project, _ := newModelInTemp(t)
		if err := project.add(Todo{ID: "f", Prompt: "fail me"}); err != nil {
			t.Fatal(err)
		}
		m.rebuildList()

		res := dropResultMsg{desc: "claude", ref: todoRef{scope: scopeProject, id: "f"}, err: errTestDrop}
		next, _ := m.Update(res)
		m = next.(model)

		if got, _ := project.find("f"); got.Done {
			t.Error("a failed run drop marked the todo done; it should stay open")
		}
		if !m.statusErr {
			t.Errorf("expected an error status after a failed drop; got status=%q", m.status)
		}
	})
}

// TestChooseTargetCarriesRef pins that both drop modes carry the dropped todo's
// ref through to the result — the handle the dropResultMsg handler needs to
// auto-mark it done.
func TestChooseTargetCarriesRef(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode dropMode
	}{
		{"run", dropRun},
		{"paste", dropPaste},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, project, _ := newModelInTemp(t)
			if err := project.add(Todo{ID: "d", Prompt: "drop me"}); err != nil {
				t.Fatal(err)
			}
			m.rebuildList()
			m.dropTodo = todoRef{scope: scopeProject, id: "d"}
			m.targets, m.targetList = m.buildTargets()
			m.stage = stageTarget

			_, cmd := m.chooseTarget(tc.mode)
			if cmd == nil {
				t.Fatal("chooseTarget returned no drop command")
			}
			res, ok := cmd().(dropResultMsg)
			if !ok {
				t.Fatalf("drop command returned %T, want dropResultMsg", cmd())
			}
			if res.ref != (todoRef{scope: scopeProject, id: "d"}) {
				t.Errorf("ref = %+v, want the dropped todo's ref for a %s drop", res.ref, tc.name)
			}
		})
	}
}

// TestBeginDropWhileDroppingIsRejected pins that a second drop can't start while
// one is still in flight — the guard that keeps two performDrop goroutines from
// racing on the same manager.
func TestBeginDropWhileDroppingIsRejected(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	if err := project.add(Todo{ID: "d", Prompt: "drop me"}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()
	m.dropping = true

	next, _ := m.beginDrop()
	m = next.(model)
	if m.stage != stageList {
		t.Errorf("stage = %v, want to stay on stageList while a drop is in flight", m.stage)
	}
	if m.status == "" || m.statusErr {
		t.Errorf("expected an informational 'in progress' status; got status=%q err=%v", m.status, m.statusErr)
	}
}

// TestBuildTargetsOffersCopilot pins the PATH-gated new-session targets: with no
// control socket the picker is just the newSessionAgents table, so copilot shows
// up exactly when a `copilot` executable is resolvable — and never ahead of
// claude, which stays the default highlight.
func TestBuildTargetsOffersCopilot(t *testing.T) {
	// A PATH holding nothing but an empty temp dir: LookPath("copilot") fails
	// even on a machine where copilot really is installed.
	emptyDir := t.TempDir()

	// …and one holding an executable of that name, which is all LookPath checks.
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "copilot"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	commands := func(targets []dropTarget) []string {
		var out []string
		for _, tg := range targets {
			out = append(out, tg.command)
		}
		return out
	}

	t.Run("absent from PATH", func(t *testing.T) {
		t.Setenv("PATH", emptyDir)
		m, _, _ := newModelInTemp(t)
		targets, _ := m.buildTargets()
		if got := commands(targets); len(got) != 1 || got[0] != "claude" {
			t.Errorf("targets = %v, want just claude when copilot is not installed", got)
		}
	})

	t.Run("on PATH", func(t *testing.T) {
		t.Setenv("PATH", binDir)
		m, _, _ := newModelInTemp(t)
		targets, list := m.buildTargets()
		got := commands(targets)
		if len(got) != 2 || got[0] != "claude" || got[1] != "copilot" {
			t.Fatalf("targets = %v, want [claude copilot]", got)
		}
		if targets[1].label == "" || !strings.Contains(targets[1].desc, "launch copilot") {
			t.Errorf("copilot target = %+v, want a label and a 'launch copilot' description", targets[1])
		}
		// The picker's default highlight must still be the Claude session.
		if idx := list.selectedIndex(); idx != 0 {
			t.Errorf("default selection = %d, want the first (claude) target", idx)
		}
	})
}

// TestBuildTargetsOffersWorktrees pins the second new-session block: one
// worktree row per launchable agent, but only where there is a repo to branch
// from, and never ahead of the plain rows (the default highlight must stay on
// "New Claude Code session" — the drop that makes no branch).
func TestBuildTargetsOffersWorktrees(t *testing.T) {
	// Keep the agent set to claude alone so the row counts below are about the
	// two flavours and not about what happens to be installed on the machine.
	t.Setenv("PATH", t.TempDir())

	t.Run("outside a repo there is nothing to branch", func(t *testing.T) {
		m, _, _ := newModelInTemp(t)
		targets, _ := m.buildTargets()
		for _, tg := range targets {
			if tg.worktree {
				t.Fatalf("targets = %+v, want no worktree rows outside a repo", targets)
			}
		}
	})

	t.Run("inside a repo, one worktree row per agent", func(t *testing.T) {
		m, _, _ := newModelInTemp(t)
		mkdir(t, filepath.Join(m.ctx.WorkDir, ".git"))

		targets, list := m.buildTargets()
		if len(targets) != 2 {
			t.Fatalf("targets = %+v, want the plain claude row and its worktree twin", targets)
		}
		if targets[0].worktree {
			t.Errorf("the first row is a worktree row; the plain drop must stay the default")
		}
		wt := targets[1]
		if !wt.worktree || wt.kind != targetNewSession || wt.command != "claude" {
			t.Fatalf("worktree row = %+v, want a claude new-session target with the flag set", wt)
		}
		if !strings.Contains(wt.label, "worktree") {
			t.Errorf("worktree row label = %q, want it to say so", wt.label)
		}
		if !strings.Contains(wt.desc, baseName(m.ctx.WorkDir)) {
			t.Errorf("worktree row desc = %q, want the repo it branches from", wt.desc)
		}
		if idx := list.selectedIndex(); idx != 0 {
			t.Errorf("default selection = %d, want the plain claude row", idx)
		}
	})

	t.Run("found from a subdirectory of the repo", func(t *testing.T) {
		// The backlog can live below the repo root; the branch point is still
		// the repo, so the rows must still be offered.
		m, _, _ := newModelInTemp(t)
		mkdir(t, filepath.Join(filepath.Dir(m.ctx.WorkDir), ".git"))

		targets, _ := m.buildTargets()
		if len(targets) != 2 || !targets[1].worktree {
			t.Fatalf("targets = %+v, want a worktree row from inside the repo", targets)
		}
	})
}

// TestTargetDesc covers the short destination labels used in the status line.
func TestTargetDesc(t *testing.T) {
	if got := targetDesc(dropTarget{kind: targetNewSession}); got != "new Claude Code session" {
		t.Errorf("new-session desc = %q", got)
	}
	if got := targetDesc(dropTarget{kind: targetExistingPane, agent: "claude"}); got != "claude" {
		t.Errorf("existing-pane desc = %q, want the agent name", got)
	}
	if got := targetDesc(dropTarget{kind: targetExistingPane}); got != "session" {
		t.Errorf("agentless existing-pane desc = %q, want the 'session' fallback", got)
	}
	// A drop that is about to cut a branch says so in the status line.
	if got := targetDesc(dropTarget{kind: targetNewSession, worktree: true}); got != "new Claude Code session on a new worktree" {
		t.Errorf("worktree new-session desc = %q", got)
	}
	if got := targetDesc(dropTarget{kind: targetNewSession, command: "codex", worktree: true}); got != "new codex session on a new worktree" {
		t.Errorf("worktree codex desc = %q", got)
	}
}

// TestBeginDropWithoutSocketReportsError pins that dropping with no cats control socket
// surfaces a status error instead of advancing to the (unusable) target picker.
func TestBeginDropWithoutSocketReportsError(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	if err := project.add(Todo{ID: "d", Prompt: "drop me"}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()

	next, _ := m.beginDrop()
	m = next.(model)
	if m.stage != stageList {
		t.Errorf("stage = %v, want to stay on stageList without a socket", m.stage)
	}
	if !m.statusErr || m.status == "" {
		t.Errorf("expected an error status; got status=%q err=%v", m.status, m.statusErr)
	}
}

// --- Enter-key bindings ---------------------------------------------------------
//
// The manager puts its two most common actions on enter and reserves the
// modifier chord for the one that leaves the pane. These tests pin that split
// on every stage that has it, including the fact that both spellings of the
// chord — shift+enter (kitty protocol) and alt+enter (legacy ESC CR) — reach
// the same handler, since which one the terminal can send is not ours to
// choose.

// enterKey builds an enter press carrying mod, so a test can name the chord it
// means rather than a KeyPressMsg literal.
func enterKey(mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEnter, Mod: mod}
}

// TestListEnterOpensTheForm pins the list's enter: the highlighted todo into
// the edit form, and — with nothing highlighted, the empty backlog the binding
// was asked for — a new entry instead.
func TestListEnterOpensTheForm(t *testing.T) {
	t.Run("empty list adds", func(t *testing.T) {
		m, _, _ := newModelInTemp(t)
		next, _ := m.updateList(enterKey(0))
		m = next.(model)
		if m.stage != stageForm || m.formMode != formAdd {
			t.Errorf("enter on an empty list: stage=%v formMode=%v, want the add form", m.stage, m.formMode)
		}
	})

	t.Run("selection edits", func(t *testing.T) {
		m, project, _ := newModelInTemp(t)
		if err := project.add(Todo{ID: "e", Title: "edit me", Prompt: "body"}); err != nil {
			t.Fatal(err)
		}
		m.rebuildList()

		next, _ := m.updateList(enterKey(0))
		m = next.(model)
		if m.stage != stageForm || m.formMode != formEdit || m.editID != "e" {
			t.Errorf("enter on a selection: stage=%v formMode=%v editID=%q, want the edit form on 'e'",
				m.stage, m.formMode, m.editID)
		}
	})

	// A filter that matches nothing leaves no selection, so enter falls to the
	// same "nothing to open, so open a new one" branch as the empty backlog.
	t.Run("filter matching nothing adds", func(t *testing.T) {
		m, project, _ := newModelInTemp(t)
		if err := project.add(Todo{ID: "e", Title: "edit me", Prompt: "body"}); err != nil {
			t.Fatal(err)
		}
		m.rebuildList()
		m.list.input.SetValue("zzzznomatch")
		m.list.filter()

		next, _ := m.updateList(enterKey(0))
		m = next.(model)
		if m.stage != stageForm || m.formMode != formAdd {
			t.Errorf("enter with an empty filter result: stage=%v formMode=%v, want the add form", m.stage, m.formMode)
		}
	})
}

// TestListModifierEnterDrops pins that both chord spellings start a drop —
// here without a control socket, so the drop reports its error on the list
// rather than reaching the picker. Reaching that error at all is the proof the
// key routed to startDrop.
func TestListModifierEnterDrops(t *testing.T) {
	for name, mod := range map[string]tea.KeyMod{"shift+enter": tea.ModShift, "alt+enter": tea.ModAlt} {
		t.Run(name, func(t *testing.T) {
			m, project, _ := newModelInTemp(t)
			if err := project.add(Todo{ID: "d", Prompt: "drop me"}); err != nil {
				t.Fatal(err)
			}
			m.rebuildList()

			next, _ := m.updateList(enterKey(mod))
			m = next.(model)
			if m.stage != stageList || !m.statusErr {
				t.Errorf("%s: stage=%v statusErr=%v, want a socketless drop error on the list", name, m.stage, m.statusErr)
			}
			if m.formMode == formEdit && m.stage == stageForm {
				t.Errorf("%s opened the edit form — the chord must not fall through to plain enter", name)
			}
		})
	}
}

// TestFormEnterInsertsNewlineAndCtrlSSaves pins the form's key split after
// enter went back to meaning what it means in every other editor. In the prompt
// field enter — and each chord spelling beside it — reaches the textarea's
// InsertNewline; ctrl+j is in that set as the raw line feed that survives
// terminals which swallow Option. Saving is ctrl+s from either field, and enter
// from the title field, which cannot hold a newline at all.
func TestFormEnterInsertsNewlineAndCtrlSSaves(t *testing.T) {
	newline := map[string]tea.KeyPressMsg{
		"enter":       enterKey(0),
		"shift+enter": enterKey(tea.ModShift),
		"alt+enter":   enterKey(tea.ModAlt),
		"ctrl+j":      {Code: 'j', Mod: tea.ModCtrl},
	}
	for name, key := range newline {
		t.Run(name+" inserts a newline", func(t *testing.T) {
			m, project, _ := newModelInTemp(t)
			next, _ := m.beginAdd()
			m = next.(model)
			m.promptArea.SetValue("first")

			next, _ = m.updateForm(key)
			m = next.(model)
			if m.stage != stageForm {
				t.Fatalf("%s in the form: stage=%v, want to stay on the form", name, m.stage)
			}
			if got := m.promptArea.Value(); !strings.Contains(got, "\n") {
				t.Errorf("%s left the prompt as %q, want a newline inserted", name, got)
			}
			if len(project.todos) != 0 {
				t.Errorf("%s saved %d todos, want 0 — the key must not save", name, len(project.todos))
			}
		})
	}

	t.Run("ctrl+s saves", func(t *testing.T) {
		m, project, _ := newModelInTemp(t)
		next, _ := m.beginAdd()
		m = next.(model)
		m.promptArea.SetValue("ship it")

		next, _ = m.updateForm(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		m = next.(model)
		if m.stage != stageList {
			t.Fatalf("ctrl+s in the form: stage=%v, want a save back to the list", m.stage)
		}
		if len(project.todos) != 1 || project.todos[0].Prompt != "ship it" {
			t.Errorf("project todos = %+v, want the saved prompt", project.todos)
		}
	})

	// Cmd+S, when a terminal is willing to report it. tea.ModSuper is the bit a
	// kitty-protocol terminal sets for the Command key, and ModMeta is what a
	// terminal that spells the same press with the meta bit sends instead; both
	// have to land on the save, because which one arrives is the terminal's
	// choice and not the user's.
	for name, key := range map[string]tea.KeyPressMsg{
		"super+s": {Code: 's', Mod: tea.ModSuper},
		"meta+s":  {Code: 's', Mod: tea.ModMeta},
	} {
		t.Run(name+" saves", func(t *testing.T) {
			m, project, _ := newModelInTemp(t)
			next, _ := m.beginAdd()
			m = next.(model)
			m.promptArea.SetValue("ship it")

			next, _ = m.updateForm(key)
			m = next.(model)
			if m.stage != stageList {
				t.Fatalf("%s in the form: stage=%v, want a save back to the list", name, m.stage)
			}
			if len(project.todos) != 1 || project.todos[0].Prompt != "ship it" {
				t.Errorf("%s: project todos = %+v, want the saved prompt", name, project.todos)
			}
		})
	}

	t.Run("enter from the title field saves", func(t *testing.T) {
		m, project, _ := newModelInTemp(t)
		next, _ := m.beginAdd()
		m = next.(model)
		m.promptArea.SetValue("ship it")
		// shift+tab walks the ring backwards from the prompt to the title.
		next, _ = m.cycleFormFocus(-1)
		m = next.(model)
		if m.formFocus == formFieldPrompt {
			t.Fatalf("focus did not leave the prompt field")
		}

		next, _ = m.updateForm(enterKey(0))
		m = next.(model)
		if m.stage != stageList {
			t.Fatalf("enter from the title field: stage=%v, want a save back to the list", m.stage)
		}
		if len(project.todos) != 1 || project.todos[0].Prompt != "ship it" {
			t.Errorf("project todos = %+v, want the saved prompt", project.todos)
		}
	})
}

// TestTargetPickerEnterRunsChordPauses pins the picker's key routing and the
// mode each key chooses. Enter is the default drop and submits; the
// modifier+enter chord is the opt-in "pause after drop" that leaves the prompt
// unsubmitted; ctrl+r stays a spelling of run.
//
// The mode itself is sealed inside the returned command's pendingAction, which
// only a live socket could unseal — so the assertion rides on the status line
// chooseTarget writes synchronously, which exists precisely because the two
// modes are two different promises and have to say which one they made.
func TestTargetPickerEnterRunsChordPauses(t *testing.T) {
	build := func(t *testing.T) model {
		t.Helper()
		m, project, _ := newModelInTemp(t)
		if err := project.add(Todo{ID: "d", Prompt: "drop me"}); err != nil {
			t.Fatal(err)
		}
		m.rebuildList()
		// Stand the picker up without a socket: buildTargets degrades to the
		// new-session target, which is selected by default.
		m.dropTodo = todoRef{scope: scopeProject, id: "d"}
		m.targets, m.targetList = m.buildTargets()
		m.stage = stageTarget
		return m
	}

	for name, tc := range map[string]struct {
		key  tea.KeyPressMsg
		verb string
	}{
		"enter":       {enterKey(0), "dropping into "},
		"ctrl+r":      {tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}, "dropping into "},
		"shift+enter": {enterKey(tea.ModShift), "pasting into "},
		"alt+enter":   {enterKey(tea.ModAlt), "pasting into "},
	} {
		t.Run(name, func(t *testing.T) {
			m := build(t)
			next, cmd := m.updateTarget(tc.key)
			m = next.(model)
			if !m.dropping || cmd == nil {
				t.Fatalf("%s: dropping=%v cmd=%v, want a dispatched drop", name, m.dropping, cmd != nil)
			}
			if !strings.HasPrefix(m.status, tc.verb) {
				t.Errorf("%s: status = %q, want it to start with %q", name, m.status, tc.verb)
			}
		})
	}

	// The pointer takes the default too — a click on a row is the same choice
	// enter makes, mode included (see clickTarget).
	t.Run("click runs", func(t *testing.T) {
		m := build(t)
		next, cmd := m.clickTarget(tea.MouseClickMsg{Y: targetRowsRow})
		m = next.(model)
		if !m.dropping || cmd == nil {
			t.Fatalf("click: dropping=%v cmd=%v, want a dispatched drop", m.dropping, cmd != nil)
		}
		if !strings.HasPrefix(m.status, "dropping into ") {
			t.Errorf("click: status = %q, want a run drop", m.status)
		}
	})
}

// TestFreezeSelected walks ctrl+f through the manager: the flip persists, the
// highlight follows the row down into the frozen group, and the status line says
// which way it went.
func TestFreezeSelected(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	for _, td := range []Todo{{ID: "a", Prompt: "a"}, {ID: "b", Prompt: "b"}} {
		if err := project.add(td); err != nil {
			t.Fatal(err)
		}
	}
	m.rebuildList()

	// Freeze the first row: it drops below the still-open second one.
	next, _ := m.updateList(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m = next.(model)
	if td, _ := project.find("a"); !td.Frozen {
		t.Fatal("ctrl+f did not freeze the highlighted todo")
	}
	if len(m.rows) != 2 || m.rows[0].id != "b" || m.rows[1].id != "a" {
		t.Fatalf("rows = %+v, want the frozen todo below the open one", m.rows)
	}
	if idx := m.list.selectedIndex(); idx < 0 || m.rows[idx].id != "a" {
		t.Errorf("highlight sits on row %d, want it to follow the frozen todo", idx)
	}
	if !strings.Contains(m.status, "frozen") || m.statusErr {
		t.Errorf("status = %q, want it to report the freeze", m.status)
	}

	// And back: the thaw returns it to the position it held.
	next, _ = m.updateList(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m = next.(model)
	if td, _ := project.find("a"); td.Frozen {
		t.Fatal("a second ctrl+f did not unfreeze the todo")
	}
	if m.rows[0].id != "a" {
		t.Errorf("rows = %+v, want the thawed todo back in its original place", m.rows)
	}
}

// TestFrozenPromptsRefuseToLeave pins the two ways a frozen prompt could reach
// an agent behind the user's back — a stray shift+enter, and a schedule set
// after the fact. Both are refused with a way out rather than silently doing
// nothing, which on a keystroke reads as a broken binding.
func TestFrozenPromptsRefuseToLeave(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"drop", tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}},
		{"schedule", tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, project, _ := newModelInTemp(t)
			if err := project.add(Todo{ID: "a", Prompt: "not doing this", Frozen: true}); err != nil {
				t.Fatal(err)
			}
			m.rebuildList()
			// A live client, so the refusal cannot be the socket guard passing
			// for the frozen one.
			m.client = &catsClient{}

			next, _ := m.updateList(tc.key)
			m = next.(model)
			if m.stage != stageList {
				t.Fatalf("stage = %v, want to stay on the list — a frozen prompt has nowhere to go", m.stage)
			}
			if !strings.Contains(m.status, "frozen") || !strings.Contains(m.status, "ctrl+f") {
				t.Errorf("status = %q, want it to name the state and the way out", m.status)
			}
		})
	}
}
