package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pngBytes is a one-pixel PNG — small, valid enough to be copied around, and
// with the extension the attachment rules accept.
var pngBytes = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89,
}

// storeWithAttachment builds a store holding one todo with a real attached file,
// which is what most of the bundle round-trips need.
func storeWithAttachment(t *testing.T) (*store, Todo) {
	t.Helper()
	s := tempStore(t)
	src := filepath.Join(t.TempDir(), "shot.png")
	if err := os.WriteFile(src, pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	td := Todo{ID: newID(), Title: "with art", Prompt: "look at this", Created: time.Now()}
	rels, err := s.attachImages(td.ID, []string{src})
	if err != nil {
		t.Fatalf("attachImages: %v", err)
	}
	td.Images = rels
	if err := s.add(td); err != nil {
		t.Fatal(err)
	}
	return s, td
}

func TestBuildBundleDropsScheduleAndKeepsTheRest(t *testing.T) {
	s := tempStore(t)
	td := Todo{
		ID: "a1", Title: "ship it", Prompt: "the body", Created: time.Now(),
		Priority: priorityHigh, Fruit: true, Frozen: true,
		Schedule: &Schedule{At: time.Now(), Kind: scheduleKindNew},
		Session:  &SessionOpts{Model: "opus"},
	}

	b, files, dropped := buildBundle(s, []Todo{td}, "test", "proj")

	if dropped != 1 {
		t.Errorf("dropped = %d, want 1", dropped)
	}
	if len(files) != 0 {
		t.Errorf("files = %d, want 0 for a todo with no attachments", len(files))
	}
	if b.Schema != bundleSchema || b.Source != "proj" || len(b.Todos) != 1 {
		t.Fatalf("envelope = %+v", b)
	}
	got := b.Todos[0]
	if got.Schedule != nil {
		t.Error("the schedule must not travel — it names a pane on the machine being left")
	}
	if got.Priority != priorityHigh || !got.Fruit || !got.Frozen || got.Title != "ship it" || got.Prompt != "the body" {
		t.Errorf("todo lost fields in the bundle: %+v", got)
	}
	// The session must be a copy, not the pointer the live todo still holds.
	if got.Session == td.Session {
		t.Error("bundle aliases the source todo's session record")
	}
	if got.Session == nil || got.Session.Model != "opus" {
		t.Errorf("session = %+v, want a copy carrying the model", got.Session)
	}
	// The source todo is untouched by having been bundled.
	if td.Schedule == nil {
		t.Error("buildBundle mutated the caller's todo")
	}
}

func TestBundleRoundTripsThroughAZip(t *testing.T) {
	src, td := storeWithAttachment(t)

	b, files, _ := buildBundle(src, []Todo{td}, "test", "proj")
	if len(files) != 1 {
		t.Fatalf("files = %d, want the one attachment", len(files))
	}
	data, ext, err := encodeBundle(b, files)
	if err != nil {
		t.Fatal(err)
	}
	if ext != bundleExtZip {
		t.Fatalf("ext = %q, want %q when an attachment travels", ext, bundleExtZip)
	}

	// The archive holds the manifest under its known name and the attachment
	// under exactly the path the manifest names.
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names[bundleManifestName] || !names[b.Todos[0].Images[0]] {
		t.Fatalf("archive members = %v", names)
	}

	got, open, err := readBundleBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Todos) != 1 || got.Todos[0].Title != "with art" {
		t.Fatalf("decoded todos = %+v", got.Todos)
	}
	raw, err := open(got.Todos[0].Images[0])
	if err != nil {
		t.Fatalf("open attachment: %v", err)
	}
	if !bytes.Equal(raw, pngBytes) {
		t.Error("attachment bytes did not survive the round trip")
	}
}

func TestEncodeBundlePlainJSONWithoutAttachments(t *testing.T) {
	b, files, _ := buildBundle(nil, []Todo{{ID: "x", Title: "t", Prompt: "p"}}, "", "global")
	data, ext, err := encodeBundle(b, files)
	if err != nil {
		t.Fatal(err)
	}
	if ext != bundleExtJSON {
		t.Fatalf("ext = %q, want %q", ext, bundleExtJSON)
	}
	var round Bundle
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("a bundle with no attachments must be readable JSON: %v", err)
	}
	if round.Todos[0].Title != "t" {
		t.Errorf("todos = %+v", round.Todos)
	}
}

// A bundle whose schema is from the future is read, not refused: the fields
// this binary knows still mean what they meant, which is the whole point of the
// additive rule. Something that is not a bundle at all is refused in words.
func TestReadBundleSchemaTolerance(t *testing.T) {
	future := []byte(`{"schema":99,"todos":[{"id":"a","title":"hi","prompt":"p","done":false}],"whatever":true}`)
	b, _, err := readBundleBytes(future)
	if err != nil {
		t.Fatalf("a future schema should still read: %v", err)
	}
	if len(b.Todos) != 1 {
		t.Fatalf("todos = %+v", b.Todos)
	}

	for _, bad := range [][]byte{
		[]byte(`{"hello":"world"}`),
		[]byte(`not json at all`),
	} {
		if _, _, err := readBundleBytes(bad); err == nil {
			t.Errorf("readBundleBytes(%s) = nil error, want a refusal", bad)
		}
	}
}

func TestWriteBundleNamesAndNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	b, files, _ := buildBundle(nil, []Todo{{ID: "x", Prompt: "p"}}, "", "My Project")
	b.Created = time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

	first, err := writeBundle(dir, "", b, files)
	if err != nil {
		t.Fatal(err)
	}
	if want := "my-project-2026-09-02" + bundleExtJSON; filepath.Base(first) != want {
		t.Errorf("name = %q, want %q", filepath.Base(first), want)
	}
	second, err := writeBundle(dir, "", b, files)
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("a second write must not overwrite the first")
	}
	if want := "my-project-2026-09-02-1" + bundleExtJSON; filepath.Base(second) != want {
		t.Errorf("second name = %q, want %q — the suffix belongs before the full extension",
			filepath.Base(second), want)
	}
}

func TestReadBundleFromDisk(t *testing.T) {
	dir := t.TempDir()
	src, td := storeWithAttachment(t)
	b, files, _ := buildBundle(src, []Todo{td}, "test", "proj")
	path, err := writeBundle(dir, "", b, files)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, bundleExtZip) {
		t.Fatalf("path = %q, want a zip", path)
	}
	got, open, err := readBundle(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Todos) != 1 {
		t.Fatalf("todos = %+v", got.Todos)
	}
	if _, err := open(got.Todos[0].Images[0]); err != nil {
		t.Errorf("open attachment: %v", err)
	}
}

func TestImportBundleFreshIDsAndAttachments(t *testing.T) {
	src, td := storeWithAttachment(t)
	b, files, _ := buildBundle(src, []Todo{td}, "test", "proj")
	data, _, err := encodeBundle(b, files)
	if err != nil {
		t.Fatal(err)
	}
	got, open, err := readBundleBytes(data)
	if err != nil {
		t.Fatal(err)
	}

	dst := tempStore(t)
	res, err := importBundle(dst, got, open, importOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.added != 1 || res.skipped != 0 || res.noFiles != 0 {
		t.Fatalf("result = %+v, want one added", res)
	}
	if len(dst.todos) != 1 {
		t.Fatalf("destination holds %d todos", len(dst.todos))
	}
	landed := dst.todos[0]
	if landed.ID == td.ID {
		t.Error("an imported prompt is a new prompt here — it should take a fresh id")
	}
	if landed.Title != "with art" {
		t.Errorf("title = %q", landed.Title)
	}
	if len(landed.Images) != 1 {
		t.Fatalf("images = %v, want the attachment copied in", landed.Images)
	}
	// The attachment lives under the *destination's* images dir, keyed by the
	// new id, and its bytes are there.
	abs := dst.imagePath(landed.Images[0])
	raw, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("attachment not written into the destination: %v", err)
	}
	if !bytes.Equal(raw, pngBytes) {
		t.Error("attachment bytes differ after import")
	}
	if !strings.Contains(landed.Images[0], landed.ID) {
		t.Errorf("stored path %q should be keyed by the new id %q", landed.Images[0], landed.ID)
	}
}

func TestImportBundleSkipsDuplicates(t *testing.T) {
	b := Bundle{Schema: 1, Todos: []Todo{
		{ID: "a", Title: "one", Prompt: "body"},
		{ID: "b", Title: "one", Prompt: "body"}, // the same prompt twice inside one bundle
		{ID: "c", Title: "two", Prompt: "other"},
	}}
	dst := tempStore(t)

	res, err := importBundle(dst, b, nil, importOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.added != 2 || res.skipped != 1 {
		t.Fatalf("result = %+v, want 2 added and 1 skipped", res)
	}
	// Importing the same bundle again adds nothing and is not an error — the
	// common mistake is doing it twice.
	res, err = importBundle(dst, b, nil, importOpts{})
	if err != nil {
		t.Fatalf("a re-import should be a quiet no-op, got %v", err)
	}
	if res.added != 0 || res.skipped != 3 {
		t.Fatalf("re-import result = %+v, want nothing added", res)
	}
	if len(dst.todos) != 2 {
		t.Fatalf("backlog holds %d todos after two imports, want 2", len(dst.todos))
	}

	// …unless the caller asks for the duplicates.
	res, err = importBundle(dst, b, nil, importOpts{allowDuplicates: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.added != 3 {
		t.Errorf("allowDuplicates added = %d, want 3", res.added)
	}
}

func TestImportBundleKeepIDs(t *testing.T) {
	b := Bundle{Schema: 1, Todos: []Todo{{ID: "keepme", Title: "t", Prompt: "p"}}}
	dst := tempStore(t)
	if _, err := importBundle(dst, b, nil, importOpts{keepIDs: true}); err != nil {
		t.Fatal(err)
	}
	if dst.todos[0].ID != "keepme" {
		t.Errorf("id = %q, want the bundle's own", dst.todos[0].ID)
	}
}

// A schedule that a hand-edited bundle carries must not arm a timer here: the
// pane and cwd it names belong to another machine.
func TestImportBundleStripsASmuggledSchedule(t *testing.T) {
	b := Bundle{Schema: 1, Todos: []Todo{{
		ID: "a", Title: "t", Prompt: "p",
		Schedule: &Schedule{At: time.Now(), Kind: scheduleKindPane, Pane: 3},
	}}}
	dst := tempStore(t)
	if _, err := importBundle(dst, b, nil, importOpts{}); err != nil {
		t.Fatal(err)
	}
	if dst.todos[0].Schedule != nil {
		t.Error("an imported prompt must never arrive scheduled")
	}
}

// The text is the part with the value: a prompt whose attachment cannot be
// brought across still lands, and is counted so the status line can say so.
func TestImportBundleLandsPromptsWithUnreadableAttachments(t *testing.T) {
	b := Bundle{Schema: 1, Todos: []Todo{{ID: "a", Title: "t", Prompt: "p", Images: []string{"images/a/gone.png"}}}}
	dst := tempStore(t)

	res, err := importBundle(dst, b, func(string) ([]byte, error) { return nil, os.ErrNotExist }, importOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.added != 1 || res.noFiles != 1 {
		t.Fatalf("result = %+v, want the prompt landed bare and counted", res)
	}
	if len(dst.todos[0].Images) != 0 {
		t.Errorf("images = %v, want none recorded", dst.todos[0].Images)
	}
}

// safeAttachmentName is the one place a bundle from elsewhere names a file on
// this filesystem, so it is where a zip-slip has to stop.
func TestSafeAttachmentNameRefusesEscapes(t *testing.T) {
	for _, bad := range []string{
		"../../etc/passwd", "/etc/passwd", "..", "", ".",
		`..\..\windows\system32\x.png`, "images/../../x.png",
	} {
		if got, err := safeAttachmentName(bad); err == nil {
			t.Errorf("safeAttachmentName(%q) = %q, want a refusal", bad, got)
		}
	}
	if got, err := safeAttachmentName("images/a1/shot.png"); err != nil || got != "shot.png" {
		t.Errorf("safeAttachmentName(ordinary) = %q, %v", got, err)
	}
}

func TestRenderBundleMarkdown(t *testing.T) {
	b := Bundle{
		Schema: 1, Created: time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC),
		From: "cats-todo on studio", Source: "cats-todo",
		Todos: []Todo{
			{Title: "first", Prompt: "do the thing", Priority: priorityCritical},
			{Title: "second", Prompt: "later", Done: true, Images: []string{"images/b/shot.png"}},
		},
	}
	md := renderBundleMarkdown(b)

	for _, want := range []string{
		"# Prompts from cats-todo",
		"## 1. first",
		"do the thing",
		"## 2. second",
		"shot.png",
		"done",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	// Ids are bookkeeping; the rendering is for a human.
	if strings.Contains(md, "images/b/") {
		t.Errorf("markdown should name the file, not its storage path:\n%s", md)
	}
}
