package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeImage creates a fake image file of n bytes in dir and returns its path.
// Nothing reads the contents — attach validates by extension and size, not by
// decoding — so the bytes only have to exist.
func writeImage(t *testing.T, dir, name string, n int) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestAttachImagesCopiesIn pins the core promise: the source file is copied
// into the backlog, recorded as a backlog-relative path, and still readable
// after the original is gone.
func TestAttachImagesCopiesIn(t *testing.T) {
	s := tempStore(t)
	src := writeImage(t, t.TempDir(), "shot.png", 32)

	rels, err := s.attachImages("abc123", []string{src})
	if err != nil {
		t.Fatalf("attachImages: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("attachImages returned %d paths, want 1", len(rels))
	}
	if want := "images/abc123/shot.png"; rels[0] != want {
		t.Errorf("stored path = %q, want %q (backlog-relative, forward-slashed)", rels[0], want)
	}

	// The recorded path must resolve next to todos.json, not next to the source.
	abs := s.imagePath(rels[0])
	if want := filepath.Join(filepath.Dir(s.path), "images", "abc123", "shot.png"); abs != want {
		t.Errorf("imagePath = %q, want %q", abs, want)
	}

	// Deleting the original is the ordinary case — a screenshot cleared off the
	// Desktop — and must not touch the attachment.
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Errorf("attachment gone after the source was deleted: %v", err)
	}
}

func TestAttachImagesDedupesNames(t *testing.T) {
	s := tempStore(t)
	dirA, dirB := t.TempDir(), t.TempDir()
	// Two different screenshots, both named the way macOS names them.
	a := writeImage(t, dirA, "Screenshot.png", 8)
	b := writeImage(t, dirB, "Screenshot.png", 16)

	rels, err := s.attachImages("id", []string{a, b})
	if err != nil {
		t.Fatalf("attachImages: %v", err)
	}
	if rels[0] == rels[1] {
		t.Fatalf("both attachments stored as %q — one overwrote the other", rels[0])
	}
	if want := "images/id/Screenshot-1.png"; rels[1] != want {
		t.Errorf("second path = %q, want %q", rels[1], want)
	}
	// Sizes differ, so this also proves the right bytes landed in each.
	for i, wantSize := range []int64{8, 16} {
		fi, err := os.Stat(s.imagePath(rels[i]))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Size() != wantSize {
			t.Errorf("attachment %d is %d bytes, want %d", i, fi.Size(), wantSize)
		}
	}
}

func TestAttachImagesRejects(t *testing.T) {
	dir := t.TempDir()
	notImage := writeImage(t, dir, "notes.txt", 8)
	huge := writeImage(t, dir, "huge.png", maxImageBytes+1)

	cases := []struct {
		name string
		src  string
		want string // substring the error must name
	}{
		{"non-image extension", notImage, "not an image"},
		{"over the size limit", huge, "attachment limit"},
		{"missing file", filepath.Join(dir, "nope.png"), "nope.png"},
		{"a directory", dir, "not a regular file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := tempStore(t)
			_, err := s.attachImages("id", []string{tc.src})
			if err == nil {
				t.Fatal("attachImages succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestAttachImagesIsAllOrNothing pins the rollback: a todo must never end up
// carrying some of the images the user asked for, with no sign of the rest.
func TestAttachImagesIsAllOrNothing(t *testing.T) {
	s := tempStore(t)
	dir := t.TempDir()
	good := writeImage(t, dir, "ok.png", 8)
	bad := writeImage(t, dir, "notes.txt", 8)

	if _, err := s.attachImages("id", []string{good, bad}); err == nil {
		t.Fatal("attachImages succeeded with a bad second file, want an error")
	}
	if _, err := os.Stat(filepath.Join(s.imagesDir(), "id")); !os.IsNotExist(err) {
		t.Errorf("the failed attach left files behind (stat err = %v)", err)
	}
}

func TestAttachImagesOnUnavailableStore(t *testing.T) {
	s := &store{scope: scopeProject, path: ""}
	src := writeImage(t, t.TempDir(), "shot.png", 8)
	if _, err := s.attachImages("id", []string{src}); err == nil {
		t.Error("attaching to an unavailable store succeeded, want an error")
	}
	// No sources is not an error anywhere — it is the ordinary add.
	if rels, err := s.attachImages("id", nil); err != nil || rels != nil {
		t.Errorf("attachImages(nil) = %v, %v; want nil, nil", rels, err)
	}
}

// TestResolveImagesReportsMissing separates the two views of an attachment
// list: the display view reports a file that has gone, the delivery view leaves
// it out rather than sending the agent after it.
func TestResolveImagesReportsMissing(t *testing.T) {
	s := tempStore(t)
	dir := t.TempDir()
	rels, err := s.attachImages("id", []string{
		writeImage(t, dir, "here.png", 8),
		writeImage(t, dir, "gone.png", 8),
	})
	if err != nil {
		t.Fatal(err)
	}
	td := Todo{ID: "id", Images: rels}

	if err := os.Remove(s.imagePath(rels[1])); err != nil {
		t.Fatal(err)
	}

	refs := s.resolveImages(td)
	if len(refs) != 2 {
		t.Fatalf("resolveImages returned %d refs, want 2 (both, missing marked)", len(refs))
	}
	if refs[0].missing {
		t.Error("resolveImages marked a present file missing")
	}
	if !refs[1].missing {
		t.Error("resolveImages did not mark the deleted file missing")
	}

	paths := s.imagePaths(td)
	if len(paths) != 1 || paths[0] != refs[0].abs {
		t.Errorf("imagePaths = %v, want only the file that still exists", paths)
	}
}

// TestDeleteRemovesAttachments pins the cleanup: without it every deletion
// leaks its image directory into the repo forever.
func TestDeleteRemovesAttachments(t *testing.T) {
	s := tempStore(t)
	rels, err := s.attachImages("keep-me", []string{writeImage(t, t.TempDir(), "a.png", 8)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.add(Todo{ID: "keep-me", Prompt: "p", Images: rels}); err != nil {
		t.Fatal(err)
	}
	if err := s.delete("keep-me"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.imagesDir(), "keep-me")); !os.IsNotExist(err) {
		t.Errorf("delete left the attachment directory behind (stat err = %v)", err)
	}
}

func TestClearDoneRemovesAttachments(t *testing.T) {
	s := tempStore(t)
	dir := t.TempDir()
	for _, id := range []string{"done-one", "still-open"} {
		rels, err := s.attachImages(id, []string{writeImage(t, filepath.Join(dir, id), "a.png", 8)})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.add(Todo{ID: id, Prompt: id, Images: rels, Done: id == "done-one"}); err != nil {
			t.Fatal(err)
		}
	}

	n, err := s.clearDone()
	if err != nil || n != 1 {
		t.Fatalf("clearDone = %d, %v; want 1, nil", n, err)
	}
	if _, err := os.Stat(filepath.Join(s.imagesDir(), "done-one")); !os.IsNotExist(err) {
		t.Errorf("clearDone left the cleared todo's attachments behind (stat err = %v)", err)
	}
	if _, err := os.Stat(filepath.Join(s.imagesDir(), "still-open")); err != nil {
		t.Errorf("clearDone removed an open todo's attachments: %v", err)
	}
}

// TestUpdatePreservesImages pins the field-by-field copy in store.update: the
// edit form knows only title and prompt, so editing text must not blank out
// attachments it never showed.
func TestUpdatePreservesImages(t *testing.T) {
	s := tempStore(t)
	rels, err := s.attachImages("id", []string{writeImage(t, t.TempDir(), "a.png", 8)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.add(Todo{ID: "id", Title: "before", Prompt: "before", Images: rels}); err != nil {
		t.Fatal(err)
	}

	// Exactly what saveForm sends: no Images field at all.
	if err := s.update(Todo{ID: "id", Title: "after", Prompt: "after"}); err != nil {
		t.Fatal(err)
	}
	got, ok := s.find("id")
	if !ok {
		t.Fatal("todo vanished after update")
	}
	if got.Prompt != "after" {
		t.Errorf("Prompt = %q, want the edit to have applied", got.Prompt)
	}
	if len(got.Images) != 1 || got.Images[0] != rels[0] {
		t.Errorf("Images = %v, want them preserved as %v", got.Images, rels)
	}
}

// TestImagesSurviveRoundTrip proves the new field persists, and that a backlog
// without attachments still writes no images key at all — an older binary has
// to keep reading these files.
func TestImagesSurviveRoundTrip(t *testing.T) {
	s := tempStore(t)
	if err := s.add(Todo{ID: "with", Prompt: "p", Images: []string{"images/with/a.png"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.add(Todo{ID: "without", Prompt: "p"}); err != nil {
		t.Fatal(err)
	}

	reloaded := &store{scope: s.scope, path: s.path}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	got, _ := reloaded.find("with")
	if len(got.Images) != 1 || got.Images[0] != "images/with/a.png" {
		t.Errorf("Images = %v, want them to survive the round trip", got.Images)
	}
	if got, _ := reloaded.find("without"); got.Images != nil {
		t.Errorf("Images = %v for a todo with none, want nil", got.Images)
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), `"images"`); n != 1 {
		t.Errorf("todos.json has %d images keys, want 1 (omitempty for the todo without)", n)
	}
}
