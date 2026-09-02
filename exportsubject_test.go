package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// exportModelWithSet is exportModel with three prompts, since a subject is only
// interesting when there is more than one thing to choose from.
func exportModelWithSet(t *testing.T) model {
	t.Helper()
	m, _ := exportModel(t)
	for _, td := range []Todo{
		{ID: "t2", Title: "second", Prompt: "two"},
		{ID: "t3", Title: "third", Prompt: "three"},
	} {
		if err := m.project.add(td); err != nil {
			t.Fatal(err)
		}
	}
	m.rebuildList()
	return m
}

// The selection is what ctrl+o sends: two ticked prompts, and both land in the
// destination while the third stays put.
func TestExportSendsTheSelection(t *testing.T) {
	m := exportModelWithSet(t)
	m.marked = map[todoRef]bool{
		{scope: scopeProject, id: "t1"}: true,
		{scope: scopeProject, id: "t3"}: true,
	}
	m.rebuildList()

	next, _ := m.Update(pressKey("ctrl+o"))
	m = next.(model)
	if m.exportSub.count() != 2 {
		t.Fatalf("subject = %+v, want the two selected prompts", m.exportSub.refs)
	}
	if !strings.Contains(m.viewExport(), "2 prompts") {
		t.Errorf("the heading should name the subject:\n%s", m.viewExport())
	}

	m.exportList.selectRef(exportRowOfKind(t, m, exportToStore))
	next, _ = m.Update(enterKey(0))
	m = next.(model)

	if len(m.global.todos) != 2 {
		t.Fatalf("global holds %+v, want the two selected prompts", m.global.todos)
	}
	if got := []string{m.global.todos[0].Title, m.global.todos[1].Title}; got[0] != "stray prompt" || got[1] != "third" {
		t.Errorf("copied %v, want them in list order", got)
	}
	if len(m.project.todos) != 3 {
		t.Errorf("a copy must leave the source backlog alone: %+v", m.project.todos)
	}
	if !strings.Contains(m.status, "2 prompts") {
		t.Errorf("status = %q, want the count", m.status)
	}
	// The set has been spent: leaving it ticked would make the next ctrl+o send
	// the same prompts again.
	if m.markCount() != 0 {
		t.Errorf("the selection should be cleared after an export, %d left", m.markCount())
	}
}

// With nothing selected the picker is about the highlighted row, exactly as it
// was before selections existed — including the heading, which names the prompt
// rather than counting it.
func TestExportWithoutASelectionUsesTheHighlight(t *testing.T) {
	m := exportModelWithSet(t)
	next, _ := m.Update(pressKey("ctrl+o"))
	m = next.(model)

	if m.exportSub.count() != 1 || m.exportSub.refs[0].id != "t1" {
		t.Fatalf("subject = %+v, want the highlighted prompt", m.exportSub.refs)
	}
	if !strings.Contains(m.viewExport(), "stray prompt") {
		t.Errorf("the heading should name the prompt:\n%s", m.viewExport())
	}
}

// ctrl+a widens the subject to the whole backlog and puts it back — including
// the closed rows, since a backlog handed to another machine is a record.
func TestExportCtrlAWidensAndNarrows(t *testing.T) {
	m := exportModelWithSet(t)
	m.project.todos[2].Done = true
	m.rebuildList()

	next, _ := m.Update(pressKey("ctrl+o"))
	m = next.(model)
	next, _ = m.Update(pressKey("ctrl+a"))
	m = next.(model)

	if !m.exportSub.all || m.exportSub.count() != 3 {
		t.Fatalf("after ctrl+a: all=%v count=%d, want the whole backlog of 3", m.exportSub.all, m.exportSub.count())
	}
	if !strings.Contains(m.viewExport(), "everything in the project backlog") {
		t.Errorf("the heading should say what widened to:\n%s", m.viewExport())
	}

	next, _ = m.Update(pressKey("ctrl+a"))
	m = next.(model)
	if m.exportSub.all || m.exportSub.count() != 1 {
		t.Errorf("ctrl+a again should narrow back, got all=%v count=%d", m.exportSub.all, m.exportSub.count())
	}
}

// The disk row opens the folder browser in its bundle flavour, and a choice
// there writes a bundle rather than a prompt into a backlog.
func TestExportBundleToDisk(t *testing.T) {
	captureOpens(t)
	m := exportModelWithSet(t)
	out := t.TempDir()

	next, _ := m.Update(pressKey("ctrl+o"))
	m = next.(model)
	m.exportList.selectRef(exportRowOfKind(t, m, exportToFile))
	next, _ = m.Update(enterKey(0))
	m = next.(model)

	if m.stage != stageFiles || m.files.purpose != filesForBundle || !m.files.dirsOnly {
		t.Fatalf("stage %v purpose %v dirsOnly %v, want the bundle browser", m.stage, m.files.purpose, m.files.dirsOnly)
	}
	if !strings.Contains(m.viewFiles(), "Save a bundle in") {
		t.Errorf("the browser should say what a choice does:\n%s", m.viewFiles())
	}

	// Point it at a folder of our own and take the "./" row.
	m.files = newFilePicker(out)
	m.files.purpose = filesForBundle
	m.files.dirsOnly = true
	m.files.refresh()
	next, _ = m.chooseExportFolder(false)
	m = next.(model)

	if m.stage != stageList || m.statusErr {
		t.Fatalf("stage = %v, status = %q (err=%v)", m.stage, m.status, m.statusErr)
	}
	entries, err := os.ReadDir(out)
	if err != nil || len(entries) != 1 {
		t.Fatalf("wrote %v (%v), want one bundle", entries, err)
	}
	written := filepath.Join(out, entries[0].Name())
	if !strings.HasSuffix(written, bundleExtJSON) {
		t.Errorf("wrote %q, want a plain JSON bundle (nothing is attached)", written)
	}
	b, _, err := readBundle(written)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Todos) != 1 || b.Todos[0].Title != "stray prompt" {
		t.Errorf("bundle = %+v, want the highlighted prompt", b.Todos)
	}
	if b.Source == "" || b.From == "" {
		t.Errorf("bundle carries no provenance: source=%q from=%q", b.Source, b.From)
	}
	if !strings.Contains(m.status, shortenHome(written)) {
		t.Errorf("status = %q, want it to name the file", m.status)
	}
}

// A schedule does not travel, and the status line says so rather than letting a
// row's clock vanish without a word.
func TestExportBundleReportsDroppedSchedules(t *testing.T) {
	captureOpens(t)
	m, _ := exportModel(t)
	out := t.TempDir()
	m.project.todos[0].Schedule = &Schedule{At: time.Now(), Kind: scheduleKindNew}
	m.rebuildList()

	next, _ := m.Update(pressKey("ctrl+o"))
	m = next.(model)
	m.files = newFilePicker(out)
	m.files.purpose = filesForBundle
	m.files.dirsOnly = true
	m.files.refresh()
	next, _ = m.chooseExportFolder(false)
	m = next.(model)

	if !strings.Contains(m.status, "schedule") {
		t.Errorf("status = %q, want the dropped schedule named", m.status)
	}
	entries, _ := os.ReadDir(out)
	b, _, err := readBundle(filepath.Join(out, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if b.Todos[0].Schedule != nil {
		t.Error("a schedule must never be in a bundle — it names a pane on this machine")
	}
}
