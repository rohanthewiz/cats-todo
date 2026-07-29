package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestReadClipboardInfo covers the three-way decision ctrl+v makes, against
// what AppleScript actually prints. The first two samples are real output: one
// from a copied PNG (which is why the list is so long — macOS synthesizes
// flavors), one from an empty-string clipboard.
func TestReadClipboardInfo(t *testing.T) {
	cases := []struct {
		name string
		info string
		want clipboardImageOffer
	}{
		{
			"copied png (real output)",
			"«class PNGf», 70, «class AVIF», 578, «class 8BPS», 3466, GIF picture, 49, «class jp2 », 3259, JPEG picture, 156\n",
			clipPNG,
		},
		{
			"empty string clipboard (real output)",
			"Unicode text, 0, string, 0, styled Clipboard text, 2, «class utf8», 0, «class ut16», 2\n",
			clipNoImage,
		},
		{"screenshot", "«class PNGf», 51610, TIFF picture, 349908\n", clipPNG},
		{"copied text", "string, 12, Unicode text, 24\n", clipNoImage},
		{"image with no png flavor", "TIFF picture, 349908, «class 8BPS», 262212\n", clipUnusableImage},
		{"empty clipboard", "\n", clipNoImage},
		{"nothing at all", "", clipNoImage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := readClipboardInfo(tc.info); got != tc.want {
				t.Errorf("readClipboardInfo(%q) = %v, want %v", tc.info, got, tc.want)
			}
		})
	}
}

func TestClipboardImageSupportedMatchesPlatform(t *testing.T) {
	if got, want := clipboardImageSupported(), runtime.GOOS == "darwin"; got != want {
		t.Errorf("clipboardImageSupported() = %v on %s, want %v", got, runtime.GOOS, want)
	}
}

// TestCaptureUnsupportedPlatformFails pins the honest failure off macOS: the key
// is not advertised there, but a caller that reaches the capture anyway gets an
// error rather than a mystery.
func TestCaptureUnsupportedPlatformFails(t *testing.T) {
	if clipboardImageSupported() {
		t.Skip("this platform supports clipboard capture; the unsupported path cannot be reached")
	}
	if _, err := captureClipboardImage(); err == nil {
		t.Error("captureClipboardImage succeeded on an unsupported platform, want an error")
	}
}

// fakeCapture stands in for pasteClipboardImage's result: the queued entry and
// the temp file, without the pasteboard. The tests below are about what the form
// does with a capture — copy it in, clean it up, throw it away — which is where
// the state machine can go wrong. The osascript call itself is deliberately not
// exercised: putting a known image on the clipboard to test it would clobber
// whatever the person running the tests had copied.
func fakeCapture(t *testing.T, m model) (model, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "cats-todo-clip-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	src := filepath.Join(dir, clipboardImageName)
	if err := os.WriteFile(src, make([]byte, 24), 0o644); err != nil {
		t.Fatal(err)
	}
	m.clipboardDirs = append(m.clipboardDirs, dir)
	m.formImages = append(m.formImages, formImage{src: src, name: clipboardImageName, pasted: true})
	return m, dir
}

// TestPastedImageIsCopiedInAndCleanedUp is the whole point of the capture path:
// the pasted image ends up in the backlog under a name that means something, and
// the temp file it passed through does not survive the save.
func TestPastedImageIsCopiedInAndCleanedUp(t *testing.T) {
	m, project := openImageEditor(t)
	m, dir := fakeCapture(t, m)

	next, _ := m.updateImages(pressKey("esc"))
	m = next.(model)
	m.promptArea.SetValue("this is what I mean")
	next, _ = m.saveForm()
	m = next.(model)
	if m.formErr != "" {
		t.Fatalf("save reported %q", m.formErr)
	}

	if len(project.todos) != 1 {
		t.Fatalf("project has %d todos, want 1", len(project.todos))
	}
	td := project.todos[0]
	if len(td.Images) != 1 || !strings.HasSuffix(td.Images[0], "/"+clipboardImageName) {
		t.Fatalf("Images = %v, want one path ending in %s", td.Images, clipboardImageName)
	}
	if _, err := os.Stat(project.imagePath(td.Images[0])); err != nil {
		t.Errorf("the pasted image was not copied into the backlog: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the capture's temp directory survived the save (stat err = %v)", err)
	}
}

// TestCancellingFormDiscardsCapture is the other half: a capture the user thinks
// better of leaves nothing anywhere.
func TestCancellingFormDiscardsCapture(t *testing.T) {
	m, project := openImageEditor(t)
	m, dir := fakeCapture(t, m)

	next, _ := m.updateImages(pressKey("esc")) // back to the form
	m = next.(model)
	next, _ = m.updateForm(pressKey("esc")) // cancel the form
	m = next.(model)

	if len(project.todos) != 0 {
		t.Errorf("project has %d todos, want none", len(project.todos))
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cancelling left the capture's temp directory behind (stat err = %v)", err)
	}
	if m.clipboardDirs != nil {
		t.Errorf("clipboardDirs = %v, want them cleared", m.clipboardDirs)
	}
}

// TestRemovingPastedRowDeletesItNow separates a capture from a stored
// attachment: removing an attachment is only a decision until the save, but a
// capture nothing else refers to can go immediately.
func TestRemovingPastedRowDeletesItNow(t *testing.T) {
	m, _ := openImageEditor(t)
	m, dir := fakeCapture(t, m)
	m.imgCursor = 0

	next, _ := m.updateImages(pressKey("ctrl+x"))
	m = next.(model)

	if len(m.formImages) != 0 {
		t.Fatalf("formImages = %+v, want the row removed", m.formImages)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the capture's temp directory survived its removal (stat err = %v)", err)
	}
	if !strings.Contains(m.imgStatus, "pasted") {
		t.Errorf("imgStatus = %q, want it to name what was removed", m.imgStatus)
	}
}

// TestReopeningFormDiscardsStaleCaptures guards the case where a form is
// abandoned by a route that does not pass through esc — the next form must not
// inherit its captures, in the list or on disk.
func TestReopeningFormDiscardsStaleCaptures(t *testing.T) {
	m, _ := openImageEditor(t)
	m, dir := fakeCapture(t, m)

	next, _ := m.beginAdd() // straight into a fresh form
	m = next.(model)

	if len(m.formImages) != 0 {
		t.Errorf("formImages = %+v, want a fresh form to start empty", m.formImages)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the stale capture's temp directory survived (stat err = %v)", err)
	}
}
