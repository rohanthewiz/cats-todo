package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// openedURLs records what the suite tried to hand the desktop.
//
// The init below is a guard, not a convenience: three seams in this package
// reach out of the process — the mail composer, the file manager, and the
// folder a bundle is written into when nobody named one — and a test that
// wandered onto one of them would launch the developer's mail client or leave a
// file in their real Downloads folder. Stubbing them for the whole package
// means a test has to *opt in* to the real thing (nothing does), instead of
// having to remember to opt out.
var openedURLs []string

// revealedFiles is the same for the file manager.
var revealedFiles []string

// testBundleDir is where a test's bundles are written instead of ~/Downloads.
var testBundleDir string

func init() {
	openURL = func(u string) error {
		openedURLs = append(openedURLs, u)
		return nil
	}
	revealFile = func(p string) {
		revealedFiles = append(revealedFiles, p)
	}
	bundleBrowseRoot = func(RunContext) string {
		if testBundleDir == "" {
			// Not inside a test that set one: somewhere disposable is still
			// better than the user's own folders.
			testBundleDir = os.TempDir()
		}
		return testBundleDir
	}
}

// captureOpens points the stubs at this test and hands back what they caught.
func captureOpens(t *testing.T) (urls *[]string, revealed *[]string) {
	t.Helper()
	openedURLs, revealedFiles = nil, nil
	testBundleDir = t.TempDir()
	t.Cleanup(func() { openedURLs, revealedFiles, testBundleDir = nil, nil, "" })
	return &openedURLs, &revealedFiles
}

func TestMailtoURLEncoding(t *testing.T) {
	u := mailtoURL("", "1 prompt from cats-todo", "line one\nline two & a + b")

	if !strings.HasPrefix(u, "mailto:?") {
		t.Fatalf("url = %q, want an addressless mailto", u)
	}
	// A space must never encode as '+': in a mailto body most clients take it
	// literally, and "ship+the+thing" in a subject line is the kind of small
	// wrongness that makes a tool look careless.
	if strings.Contains(u, "+") && !strings.Contains(u, "%2B") {
		t.Errorf("url encodes a space as '+': %q", u)
	}
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("unparseable mailto: %v", err)
	}
	q := parsed.Query()
	if q.Get("subject") != "1 prompt from cats-todo" {
		t.Errorf("subject = %q", q.Get("subject"))
	}
	if q.Get("body") != "line one\nline two & a + b" {
		t.Errorf("body round-tripped as %q", q.Get("body"))
	}
}

func TestMailtoTooLong(t *testing.T) {
	if mailtoTooLong(mailtoURL("", "s", "short body")) {
		t.Error("an ordinary message should not be refused")
	}
	if !mailtoTooLong(mailtoURL("", "s", strings.Repeat("x", maxMailtoBytes+1))) {
		t.Error("a body past the cap should be refused, not truncated")
	}
}

func TestPromptWord(t *testing.T) {
	if promptWord(1) != "1 prompt" || promptWord(0) != "0 prompts" || promptWord(4) != "4 prompts" {
		t.Errorf("promptWord: %q %q %q", promptWord(0), promptWord(1), promptWord(4))
	}
}

// The in-body mail row composes a message and leaves the backlog alone: a mail
// is a copy, and nothing about sending one should change what is on disk here.
func TestExportEmailInBodyOpensAComposer(t *testing.T) {
	urls, revealed := captureOpens(t)
	m, _ := exportModel(t)

	next, _ := m.Update(pressKey("ctrl+o"))
	m = next.(model)
	m.exportList.selectRef(exportRowOfKind(t, m, exportToMailBody))
	next, _ = m.Update(enterKey(0))
	m = next.(model)

	if m.stage != stageList {
		t.Fatalf("stage = %v, want back on the list", m.stage)
	}
	if len(*urls) != 1 {
		t.Fatalf("opened %v, want one composer", *urls)
	}
	if len(*revealed) != 0 {
		t.Errorf("the in-body row should not touch the file manager: %v", *revealed)
	}
	body, err := url.Parse((*urls)[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.Query().Get("body"), "stray prompt") {
		t.Errorf("the prompt is not in the message body: %q", body.Query().Get("body"))
	}
	if m.statusErr || !strings.Contains(m.status, "mail message") {
		t.Errorf("status = %q (err=%v)", m.status, m.statusErr)
	}
	if len(m.project.todos) != 1 {
		t.Errorf("mailing a prompt must not change the backlog")
	}
}

// The "with a bundle" row writes the file, reveals it, and says where it is —
// because a mailto: URL cannot carry an attachment and the UI must not pretend
// otherwise.
func TestExportEmailWithBundleWritesAndReveals(t *testing.T) {
	urls, revealed := captureOpens(t)
	m, _ := exportModel(t)

	next, _ := m.Update(pressKey("ctrl+o"))
	m = next.(model)
	m.exportList.selectRef(exportRowOfKind(t, m, exportToMailFile))
	next, _ = m.Update(enterKey(0))
	m = next.(model)

	if len(*urls) != 1 || len(*revealed) != 1 {
		t.Fatalf("opened %v, revealed %v — want one of each", *urls, *revealed)
	}
	written := (*revealed)[0]
	if filepath.Dir(written) != testBundleDir {
		t.Errorf("bundle written to %q, want it under %q", written, testBundleDir)
	}
	if _, err := os.Stat(written); err != nil {
		t.Fatalf("the bundle is not on disk: %v", err)
	}
	b, _, err := readBundle(written)
	if err != nil {
		t.Fatalf("the written bundle does not read back: %v", err)
	}
	if len(b.Todos) != 1 || b.Todos[0].Title != "stray prompt" {
		t.Errorf("bundle todos = %+v", b.Todos)
	}
	if !strings.Contains(m.status, "attach") {
		t.Errorf("status = %q, want it to name the file to attach", m.status)
	}
}

// A prompt too long to prefill is refused in words rather than truncated into
// a message whose last paragraph is silently missing.
func TestExportEmailRefusesAnOversizeBody(t *testing.T) {
	urls, _ := captureOpens(t)
	m, _ := exportModel(t)
	m.project.todos[0].Prompt = strings.Repeat("long ", maxMailtoBytes)
	m.rebuildList()

	next, _ := m.Update(pressKey("ctrl+o"))
	m = next.(model)
	m.exportList.selectRef(exportRowOfKind(t, m, exportToMailBody))
	next, _ = m.Update(enterKey(0))
	m = next.(model)

	if len(*urls) != 0 {
		t.Errorf("nothing should have been opened: %v", *urls)
	}
	if !m.statusErr || !strings.Contains(m.status, "bundle") {
		t.Errorf("status = %q (err=%v), want a refusal pointing at a bundle", m.status, m.statusErr)
	}
}

// Every off-machine destination is a copy: there is no backlog at the far end
// of a file or a message to have moved the prompt into, so the move chord is
// refused in words rather than deleting the only copy.
func TestExportRefusesMoveToABundleDestination(t *testing.T) {
	captureOpens(t)
	m, _ := exportModel(t)
	next, _ := m.Update(pressKey("ctrl+o"))
	m = next.(model)

	for _, k := range []exportKind{exportToFile, exportToMailBody, exportToMailFile} {
		m.exportList.selectRef(exportRowOfKind(t, m, k))
		next, _ = m.chooseExport(true)
		m = next.(model)
		if !m.statusErr || !strings.Contains(m.status, "copy") {
			t.Errorf("kind %v: status = %q (err=%v), want the copy-only refusal", k, m.status, m.statusErr)
		}
		if m.stage != stageExport {
			t.Errorf("kind %v: a refusal should leave the picker up, stage = %v", k, m.stage)
		}
	}
	if len(openedURLs) != 0 {
		t.Errorf("a refused move must not send anything: %v", openedURLs)
	}
}
