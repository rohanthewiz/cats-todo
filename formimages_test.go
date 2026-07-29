package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// pressKey builds the KeyPressMsg a keystroke arrives as, for driving the attachment
// editor the way the other model tests drive the list and form.
func pressKey(s string) tea.KeyPressMsg {
	if len(s) == 1 {
		return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	}
	// ctrl+<letter>
	if r, ok := strings.CutPrefix(s, "ctrl+"); ok && len(r) == 1 {
		return tea.KeyPressMsg{Code: rune(r[0]), Mod: tea.ModCtrl}
	}
	panic("unhandled key " + s)
}

// openImageEditor puts a model in the attachment editor over a fresh add form.
func openImageEditor(t *testing.T) (model, *store) {
	t.Helper()
	m, project, _ := newModelInTemp(t)
	next, _ := m.beginAdd()
	m = next.(model)
	next, _ = m.updateForm(pressKey("ctrl+o"))
	m = next.(model)
	if m.stage != stageImages {
		t.Fatalf("ctrl+o from the form left stage = %v, want stageImages", m.stage)
	}
	return m, project
}

// TestAttachThroughForm walks the whole phase-2 path: open the editor, type a
// path, save, and find the file copied into the backlog with the todo pointing
// at it.
func TestAttachThroughForm(t *testing.T) {
	m, project := openImageEditor(t)
	src := writeImage(t, t.TempDir(), "shot.png", 16)

	m.imgInput.SetValue(src)
	next, _ := m.updateImages(pressKey("enter"))
	m = next.(model)

	if len(m.formImages) != 1 {
		t.Fatalf("formImages = %+v, want the one attachment", m.formImages)
	}
	if m.formImages[0].src != src || m.formImages[0].rel != "" {
		t.Errorf("entry = %+v, want a pending src with no rel yet", m.formImages[0])
	}
	if m.imgInput.Value() != "" {
		t.Errorf("input still holds %q, want it cleared after attaching", m.imgInput.Value())
	}
	// Nothing may be copied before the form is saved — cancelling has to be free.
	if _, err := os.Stat(project.imagesDir()); !os.IsNotExist(err) {
		t.Errorf("attaching copied files before save (stat err = %v)", err)
	}

	// Back to the form, then save.
	next, _ = m.updateImages(pressKey("esc"))
	m = next.(model)
	if m.stage != stageForm {
		t.Fatalf("esc left stage = %v, want stageForm", m.stage)
	}
	m.promptArea.SetValue("this layout is wrong")
	next, _ = m.saveForm()
	m = next.(model)

	if len(project.todos) != 1 {
		t.Fatalf("project has %d todos, want 1", len(project.todos))
	}
	td := project.todos[0]
	if len(td.Images) != 1 || td.Images[0] != "images/"+td.ID+"/shot.png" {
		t.Fatalf("Images = %v, want one path under the todo's id", td.Images)
	}
	if _, err := os.Stat(project.imagePath(td.Images[0])); err != nil {
		t.Errorf("the attachment was not copied in: %v", err)
	}
	if !strings.Contains(m.status, "1 image") {
		t.Errorf("status = %q, want it to mention the attachment", m.status)
	}
}

// TestCancellingFormAttachesNothing pins the promise the editor's footer makes:
// nothing is copied until the prompt is saved.
func TestCancellingFormAttachesNothing(t *testing.T) {
	m, project := openImageEditor(t)
	m.imgInput.SetValue(writeImage(t, t.TempDir(), "shot.png", 16))
	next, _ := m.updateImages(pressKey("enter"))
	m = next.(model)

	next, _ = m.updateImages(pressKey("esc")) // back to the form
	m = next.(model)
	next, _ = m.updateForm(pressKey("esc")) // cancel the form
	m = next.(model)

	if m.stage != stageList {
		t.Fatalf("stage = %v, want stageList after cancelling", m.stage)
	}
	if len(project.todos) != 0 {
		t.Errorf("project has %d todos, want none", len(project.todos))
	}
	if _, err := os.Stat(project.imagesDir()); !os.IsNotExist(err) {
		t.Errorf("cancelling left copied files behind (stat err = %v)", err)
	}
}

// TestEmptyEnterClosesEditor pins the "do what's in front of me" split: enter
// with a path attaches it, enter with an empty box means done.
func TestEmptyEnterClosesEditor(t *testing.T) {
	m, _ := openImageEditor(t)
	next, _ := m.updateImages(pressKey("enter"))
	m = next.(model)
	if m.stage != stageForm {
		t.Errorf("enter on an empty box left stage = %v, want stageForm", m.stage)
	}
}

func TestAttachRejectsBadPathInPlace(t *testing.T) {
	m, _ := openImageEditor(t)
	m.imgInput.SetValue(filepath.Join(t.TempDir(), "nope.png"))
	next, _ := m.updateImages(pressKey("enter"))
	m = next.(model)

	if len(m.formImages) != 0 {
		t.Errorf("formImages = %+v, want the bad path refused", m.formImages)
	}
	if m.imgStatus == "" {
		t.Error("no error shown for a missing file")
	}
	if m.stage != stageImages {
		t.Errorf("stage = %v, want to stay in the editor so the path can be fixed", m.stage)
	}
	// The typed path stays put — the user has to be able to correct it.
	if m.imgInput.Value() == "" {
		t.Error("the rejected path was cleared from the box")
	}
}

func TestAttachRefusesDuplicateSource(t *testing.T) {
	m, _ := openImageEditor(t)
	src := writeImage(t, t.TempDir(), "shot.png", 16)

	for i := range 2 {
		m.imgInput.SetValue(src)
		next, _ := m.updateImages(pressKey("enter"))
		m = next.(model)
		if i == 0 && len(m.formImages) != 1 {
			t.Fatalf("first attach left %d entries, want 1", len(m.formImages))
		}
	}
	if len(m.formImages) != 1 {
		t.Errorf("formImages = %+v, want the duplicate refused", m.formImages)
	}
	if !strings.Contains(m.imgStatus, "already attached") {
		t.Errorf("imgStatus = %q, want it to say the file is already attached", m.imgStatus)
	}
}

// TestEditRemovesAttachment covers the other half of the editor: opening a todo
// that already has an attachment, dropping it, and finding both the record and
// the file gone — but only after the save.
func TestEditRemovesAttachment(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	rels, err := project.attachImages("t1", []string{writeImage(t, t.TempDir(), "shot.png", 16)})
	if err != nil {
		t.Fatal(err)
	}
	if err := project.add(Todo{ID: "t1", Title: "has an image", Prompt: "look", Images: rels}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()
	abs := project.imagePath(rels[0])

	next, _ := m.beginEditRef(todoRef{scope: scopeProject, id: "t1"})
	m = next.(model)
	if len(m.formImages) != 1 || m.formImages[0].rel != rels[0] {
		t.Fatalf("formImages = %+v, want the stored attachment", m.formImages)
	}

	next, _ = m.updateForm(pressKey("ctrl+o"))
	m = next.(model)
	next, _ = m.updateImages(pressKey("ctrl+x"))
	m = next.(model)
	if len(m.formImages) != 0 {
		t.Fatalf("formImages = %+v, want the row removed", m.formImages)
	}
	// Still on disk: removal is only a decision until the form is saved.
	if _, err := os.Stat(abs); err != nil {
		t.Errorf("the file went before the save: %v", err)
	}

	next, _ = m.updateImages(pressKey("esc"))
	m = next.(model)
	next, _ = m.saveForm()
	m = next.(model)

	td, ok := project.find("t1")
	if !ok {
		t.Fatal("todo vanished")
	}
	if len(td.Images) != 0 {
		t.Errorf("Images = %v, want the attachment dropped from the record", td.Images)
	}
	if _, err := os.Stat(abs); !os.IsNotExist(err) {
		t.Errorf("the detached file survived the save (stat err = %v)", err)
	}
}

// TestEditPreservesUntouchedAttachments is the regression guard for the whole
// design: editing a prompt's text without opening the attachment editor must
// leave its images exactly as they were.
func TestEditPreservesUntouchedAttachments(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	rels, err := project.attachImages("t1", []string{writeImage(t, t.TempDir(), "shot.png", 16)})
	if err != nil {
		t.Fatal(err)
	}
	if err := project.add(Todo{ID: "t1", Title: "t", Prompt: "before", Images: rels}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()

	next, _ := m.beginEditRef(todoRef{scope: scopeProject, id: "t1"})
	m = next.(model)
	m.promptArea.SetValue("after")
	next, _ = m.saveForm()
	m = next.(model)

	td, _ := project.find("t1")
	if td.Prompt != "after" {
		t.Errorf("Prompt = %q, want the edit applied", td.Prompt)
	}
	if len(td.Images) != 1 || td.Images[0] != rels[0] {
		t.Errorf("Images = %v, want them untouched as %v", td.Images, rels)
	}
	if _, err := os.Stat(project.imagePath(rels[0])); err != nil {
		t.Errorf("the file went during a text-only edit: %v", err)
	}
}

// TestAddedAndDroppedInOneEdit exercises both halves at once — the ordering in
// saveForm has to survive a save that copies one file in and deletes another.
func TestAddedAndDroppedInOneEdit(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	dir := t.TempDir()
	rels, err := project.attachImages("t1", []string{writeImage(t, dir, "old.png", 8)})
	if err != nil {
		t.Fatal(err)
	}
	if err := project.add(Todo{ID: "t1", Title: "t", Prompt: "p", Images: rels}); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()
	oldAbs := project.imagePath(rels[0])

	next, _ := m.beginEditRef(todoRef{scope: scopeProject, id: "t1"})
	m = next.(model)
	next, _ = m.updateForm(pressKey("ctrl+o"))
	m = next.(model)

	// Drop the existing one, add a new one.
	next, _ = m.updateImages(pressKey("ctrl+x"))
	m = next.(model)
	m.imgInput.SetValue(writeImage(t, dir, "new.png", 8))
	next, _ = m.updateImages(pressKey("enter"))
	m = next.(model)

	next, _ = m.updateImages(pressKey("esc"))
	m = next.(model)
	next, _ = m.saveForm()
	m = next.(model)
	if m.formErr != "" {
		t.Fatalf("save reported %q", m.formErr)
	}

	td, _ := project.find("t1")
	if len(td.Images) != 1 || !strings.HasSuffix(td.Images[0], "new.png") {
		t.Fatalf("Images = %v, want only the newly attached file", td.Images)
	}
	if _, err := os.Stat(project.imagePath(td.Images[0])); err != nil {
		t.Errorf("the new attachment was not copied in: %v", err)
	}
	if _, err := os.Stat(oldAbs); !os.IsNotExist(err) {
		t.Errorf("the dropped file survived (stat err = %v)", err)
	}
}

func TestCleanSourcePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	cases := []struct {
		name, in, want string
	}{
		{"plain", "/tmp/a.png", "/tmp/a.png"},
		{"surrounding space", "  /tmp/a.png\t", "/tmp/a.png"},
		{"single quoted", "'/tmp/my shot.png'", "/tmp/my shot.png"},
		{"double quoted", `"/tmp/my shot.png"`, "/tmp/my shot.png"},
		{"shell-escaped space", `/tmp/my\ shot.png`, "/tmp/my shot.png"},
		{"file url", "file:///tmp/a.png", "/tmp/a.png"},
		{"tilde", "~/Desktop/a.png", filepath.Join(home, "Desktop", "a.png")},
		{"empty", "   ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanSourcePath(tc.in); got != tc.want {
				t.Errorf("cleanSourcePath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestRecentImagesNewestFirst covers the ctrl+r source: the override directory
// is scanned, non-images are skipped, and the newest file leads.
func TestRecentImagesNewestFirst(t *testing.T) {
	dir := t.TempDir()
	writeImage(t, dir, "notes.txt", 8) // skipped: not an image
	older := writeImage(t, dir, "older.png", 8)
	newer := writeImage(t, dir, "newer.png", 8)

	// Make the ordering explicit rather than trusting write order.
	old := mustParseTime(t, "2026-07-01T00:00:00Z")
	if err := os.Chtimes(older, old, old); err != nil {
		t.Fatal(err)
	}

	t.Setenv(imageSourceDirEnvVar, dir)
	got := recentImages()
	if len(got) != 2 {
		t.Fatalf("recentImages = %v, want the two image files", got)
	}
	if got[0] != newer || got[1] != older {
		t.Errorf("recentImages = %v, want newest first (%s then %s)", got, newer, older)
	}
}

// TestCycleRecentImageFillsBox pins ctrl+r: it offers a path rather than
// attaching one, so a wrong guess costs nothing.
func TestCycleRecentImageFillsBox(t *testing.T) {
	dir := t.TempDir()
	shot := writeImage(t, dir, "shot.png", 8)
	t.Setenv(imageSourceDirEnvVar, dir)

	m, _ := openImageEditor(t)
	next, _ := m.updateImages(pressKey("ctrl+r"))
	m = next.(model)

	if m.imgInput.Value() != shot {
		t.Errorf("input = %q, want the recent screenshot %q", m.imgInput.Value(), shot)
	}
	if len(m.formImages) != 0 {
		t.Error("ctrl+r attached the file itself, want it only offered in the box")
	}
}

// mustParseTime parses an RFC3339 stamp for backdating a file's mtime.
func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}
