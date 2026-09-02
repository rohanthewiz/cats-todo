// email.go — handing prompts to the user's mail client.
//
// Sending mail properly means SMTP, and SMTP means a server, a username and a
// password: three things to configure, one of them a secret this program would
// have to store, for a feature whose whole job is "get this prompt to a person".
// The machine already has something that knows all three and is trusted with
// them — the mail client — so this is a hand-off rather than a sender. A
// mailto: URL with the subject and body filled in, opened the way a link is
// opened, and the composer comes up with the prompts in it for the user to
// address and send.
//
// The honest limit of that choice, stated here because the UI has to state it
// too: **a mailto: URL cannot carry an attachment.** There is no field for one
// in RFC 6068 and no mail client accepts one. So "email the prompts" puts them
// in the body as markdown (renderBundleMarkdown), and "email a bundle" writes
// the bundle to disk, shows the user where it is, and opens the same composer
// with a line naming the file to drag in. What it does not do is claim to have
// attached something.
package main

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// maxMailtoBytes caps the URL a mail client is handed.
//
// There is no standard limit, and the real ones are all somebody's buffer:
// macOS's opener, the browser in the middle when the handler is webmail, the
// client itself. Around 8 KB everything still works and past it the failure is
// silent truncation — a prompt that arrives with its last paragraph missing,
// which the sender has no way to notice. So the limit is enforced here, and
// over it the offer changes to writing a bundle instead. Refused in words,
// never quietly cut.
const maxMailtoBytes = 8 << 10

// openTimeout bounds the handoff to the desktop's opener. It returns as soon as
// the launcher has taken the URL — it does not wait for the mail client — so
// this is only the backstop for an opener that hangs.
const openTimeout = 10 * time.Second

// mailtoURL builds the composer link. Both fields are percent-encoded per
// RFC 6068, which is url.QueryEscape's encoding except that a space is %20
// rather than '+': a '+' in a mailto body is a literal plus in most clients and
// a space in some, and a subject line reading "ship+the+thing" is the kind of
// small wrongness that makes a tool look careless.
func mailtoURL(to, subject, body string) string {
	q := "subject=" + escapeMailto(subject) + "&body=" + escapeMailto(body)
	return "mailto:" + escapeMailtoAddr(to) + "?" + q
}

// escapeMailto is QueryEscape with the plus problem fixed (see mailtoURL).
func escapeMailto(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// escapeMailtoAddr encodes the address part, which is a path segment rather
// than a query value — an empty address is normal and means "let the user
// choose the recipient", which is the usual case here.
func escapeMailtoAddr(to string) string {
	if to == "" {
		return ""
	}
	return strings.ReplaceAll(url.PathEscape(to), "+", "%20")
}

// mailtoTooLong reports whether a composer link would be past what a client can
// be trusted to carry whole.
func mailtoTooLong(u string) bool { return len(u) > maxMailtoBytes }

// openURL hands a URL to the desktop, which decides what opens it. The command
// per platform is the platform's own: `open` on macOS, `xdg-open` on the Unixes,
// and `rundll32` on Windows, which cats does not run on today but which costs a
// line to be right about.
//
// Bounded with a context for the same reason the clipboard calls are
// (clipboard.go): this runs on the UI thread, and a launcher that never returns
// would take the manager with it.
var openURL = func(u string) error {
	name, args := openCommand(u)
	if name == "" {
		return fmt.Errorf("no way to open a link on %s", runtime.GOOS)
	}
	ctx, cancel := context.WithTimeout(context.Background(), openTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s: %s", name, msg)
		}
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// openCommand is the platform's opener and its arguments.
func openCommand(target string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{target}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", target}
	default:
		return "xdg-open", []string{target}
	}
}

// revealFile opens the file's folder with the file selected, so a bundle the
// user has to drag into a message is in front of them rather than named in a
// status line they have to go and find. macOS has `open -R` for exactly this;
// elsewhere the folder itself is the closest thing, and a failure is not worth
// reporting — the status line already carries the path, which is the part that
// matters.
var revealFile = func(path string) {
	ctx, cancel := context.WithTimeout(context.Background(), openTimeout)
	defer cancel()
	if runtime.GOOS == "darwin" {
		_ = exec.CommandContext(ctx, "open", "-R", path).Run()
		return
	}
	name, args := openCommand(filepath.Dir(path))
	if name == "" {
		return
	}
	_ = exec.CommandContext(ctx, name, args...).Run()
}

// mailSubject is the subject line for a set of prompts: what they are and where
// they came from, in the words the status line uses for the same set.
func mailSubject(sub string, n int) string {
	if sub == "" {
		return fmt.Sprintf("%s from cats-todo", promptWord(n))
	}
	return fmt.Sprintf("%s from %s", promptWord(n), sub)
}

// promptWord is "1 prompt" / "4 prompts" — the phrase every status line, mail
// subject and picker heading in this feature counts with, in one place so they
// cannot disagree about the plural.
func promptWord(n int) string {
	if n == 1 {
		return "1 prompt"
	}
	return fmt.Sprintf("%d prompts", n)
}
