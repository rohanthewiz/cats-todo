// clipboard.go — pulling an image off the system clipboard.
//
// Every other capture path starts from a file: -i takes one, the editor's box
// takes a path, ctrl+r finds one on the Desktop. This is the one that cannot.
// An image copied out of a browser, or a screenshot taken with
// shift+cmd+ctrl+4, exists only on the pasteboard — and no terminal hands
// clipboard image bytes to the foreground app, so a paste keystroke inside the
// TUI can never carry it. Asking the OS directly is the only way in.
//
// macOS only, via osascript: the pasteboard is the system's, not the
// terminal's, and there is no portable way to read it. On anything else the
// feature reports itself unavailable rather than pretending — the editor's
// footer only offers the key where it works.

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// clipboardTimeout bounds an osascript call. The read is normally a few
// milliseconds, which is why it runs inline in Update rather than off the UI
// thread like a drop does; this is the backstop for the case where the
// pasteboard server is wedged and the whole call hangs.
const clipboardTimeout = 3 * time.Second

// clipboardImageName is what a captured image is called once it reaches a
// backlog. The temp file is named this so attachOne's basename lands something
// meaningful — a stored "cats-todo-clip-1847.png" would name the plumbing
// instead of the picture. Collisions get uniqueName's -1, -2 treatment.
const clipboardImageName = "clipboard.png"

// clipboardImageSupported reports whether this platform can be asked for a
// clipboard image at all. It gates the editor's key and its footer hint, so the
// UI never advertises something that cannot work here.
func clipboardImageSupported() bool { return runtime.GOOS == "darwin" }

// copyTextToClipboard hands text to the system clipboard, from wherever this
// program happens to be running.
//
// Two mechanisms, on purpose, because neither one covers the ground alone:
//
//   - OSC 52, the escape sequence that asks the terminal to set its own
//     clipboard. It is the only one that works when the manager is running on
//     the far side of an ssh session or inside a multiplexer, since it travels
//     the same channel the drawing does. Not every terminal honours it, and
//     several that do have it off by default.
//   - pbcopy, which is unconditional but only exists — and only means the right
//     clipboard — when the process is on the same machine as the user's Mac.
//     The same reasoning that put osascript in this file applies: the pasteboard
//     is the system's, and there is no portable way to reach it.
//
// Both carry the same bytes, so the two racing is harmless: whichever lands
// second writes what the first one already wrote.
func copyTextToClipboard(text string) tea.Cmd {
	osc := tea.SetClipboard(text)
	if runtime.GOOS != "darwin" {
		return osc
	}
	return tea.Batch(osc, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "pbcopy")
		cmd.Stdin = strings.NewReader(text)
		// Best effort by design. OSC 52 has already been asked for, the user has
		// been told the copy happened, and there is no second thing to try — a
		// failure here is a message about plumbing in place of the text they
		// wanted, which is worse than the copy the other path probably made.
		_ = cmd.Run()
		return nil
	})
}

// clipboardImageOffer is what the pasteboard is currently offering, which is
// what ctrl+v has to decide between: capture an image, explain why it cannot, or
// get out of the way of an ordinary text paste.
type clipboardImageOffer int

const (
	// clipNoImage: nothing image-shaped on the clipboard. ctrl+v then means what
	// it has always meant in a text box — paste the text.
	clipNoImage clipboardImageOffer = iota
	// clipPNG: an image we can ask for as PNG.
	clipPNG
	// clipUnusableImage: an image, but not in a flavor we can take. Rare —
	// macOS synthesizes a PNG flavor for most copied images — but worth its own
	// answer, because silently pasting nothing is the one outcome that leaves
	// the user with no idea what happened.
	clipUnusableImage
)

// clipboardOffer asks the pasteboard what it is holding. Metadata only: it reads
// flavor names and byte counts, never contents.
func clipboardOffer() clipboardImageOffer {
	if !clipboardImageSupported() {
		return clipNoImage
	}
	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "osascript", "-e", "clipboard info").Output()
	if err != nil {
		return clipNoImage
	}
	return readClipboardInfo(string(out))
}

// imageFlavors are the non-PNG image flavors worth recognizing — enough to tell
// "there is a picture here I cannot take" from "there is no picture here".
var imageFlavors = []string{"TIFF", "8BPS", "JPEG", "GIF picture", "AVIF", "jp2"}

// readClipboardInfo classifies AppleScript's `clipboard info` output, a flat list
// of flavor/size pairs:
//
//	«class PNGf», 51610, «class 8BPS», 262212, TIFF picture, 349908
//
// PNG is looked for specifically rather than "any image", because PNG is what
// the capture below coerces to — a clipboard holding only TIFF would pass a
// looser check and then fail at the write.
func readClipboardInfo(info string) clipboardImageOffer {
	if strings.Contains(info, "PNGf") {
		return clipPNG
	}
	for _, flavor := range imageFlavors {
		if strings.Contains(info, flavor) {
			return clipUnusableImage
		}
	}
	return clipNoImage
}

// captureClipboardImage writes the clipboard's image into a fresh temp directory
// as clipboard.png and returns the path. The caller owns the directory and is
// expected to remove it once the image has been copied into a backlog (or the
// form abandoned) — see model.discardClipboardCaptures.
//
// A temp file rather than the backlog itself, because an add has no todo id to
// file it under until it saves, and because a capture the user then cancels must
// leave the backlog untouched. It is the same pending-source contract the typed
// path already has; this one just materializes its file first.
func captureClipboardImage() (string, error) {
	if !clipboardImageSupported() {
		return "", errors.New("reading a clipboard image is only supported on macOS")
	}

	dir, err := os.MkdirTemp("", "cats-todo-clip-*")
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dir, clipboardImageName)

	// `open for access` creates the file, and the write coerces the pasteboard
	// to PNG — the coercion fails loudly if the clipboard holds no image, which
	// is why clipboardHasImage is only an optimization and not the guard.
	script := fmt.Sprintf(`set f to (open for access POSIX file %q with write permission)
try
	set eof f to 0
	write (the clipboard as «class PNGf») to f
	close access f
on error errMsg
	close access f
	error errMsg
end try`, dst)

	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "osascript", "-e", script).CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("could not read an image from the clipboard: %s", firstLine(msg, 120))
	}

	// osascript can exit 0 having written nothing at all; an empty file would
	// otherwise sail through as a valid attachment and reach the agent as a
	// broken image.
	fi, err := os.Stat(dst)
	if err != nil || fi.Size() == 0 {
		os.RemoveAll(dir)
		return "", errors.New("the clipboard produced an empty image")
	}
	return dst, nil
}

// readClipboardText asks the system pasteboard for the text it is holding.
//
// The mirror of copyTextToClipboard, and deliberately not symmetric with it. A
// copy can be shouted down both channels at once because both carry the same
// bytes; a read has to come back with an answer, and only one of the two can
// give one here and now:
//
//   - pbpaste answers immediately, and it answers with the pasteboard the user
//     actually copied into — the same reasoning that put osascript in this file.
//     It only means the right clipboard when the process is on the user's Mac,
//     which is the case this tool lives in (a drop targets panes on this
//     machine).
//   - OSC 52's read half (tea.ReadClipboard) is the one that survives ssh or a
//     multiplexer, but the reply arrives later, as a tea.ClipboardMsg, and many
//     terminals refuse the read outright — letting a program on the far end read
//     the local clipboard is a leak, so it is off by default nearly everywhere.
//     That is the caller's fallback, not this function's job.
//
// supported=false is "there is no local pasteboard to reach here, ask the
// terminal instead"; it is not a failure.
//
// A variable, like cdxStateFile in export.go and for the same reason: the tests
// drive the editor's paste and must be handed a clipboard rather than whatever
// the machine running them happens to be holding.
var readClipboardText = func() (text string, supported bool, err error) {
	if runtime.GOOS != "darwin" {
		return "", false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "pbpaste").Output()
	if err != nil {
		return "", true, fmt.Errorf("could not read the clipboard: %s", firstLine(err.Error(), 120))
	}
	return string(out), true, nil
}
