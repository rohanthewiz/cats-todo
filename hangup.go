package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
)

// errTerminalGone is the cancel cause once the terminal the TUI runs in has
// hung up — by SIGHUP or by its tty reads failing, whichever lands first.
var errTerminalGone = errors.New("terminal hung up")

// terminalWatch ends the program when its terminal goes away. bubbletea
// v2.0.8 does not do this on its own, on either of the two channels a hangup
// arrives by:
//
//   - SIGHUP. Its handler (tea.go handleSignals) listens for SIGINT and SIGTERM
//     only, so SIGHUP keeps whatever disposition the process inherited. That is
//     "exit" by default, but a pane spawned by cathost inherits the SIG_IGN
//     cathost sets for itself (signal.Ignore survives fork+exec), and a Go
//     program started with SIGHUP ignored keeps it ignored.
//   - EOF on input. When the pty master closes, a read on the slave returns 0
//     bytes. ultraviolet's TerminalReader.StreamEvents maps io.EOF to a nil
//     error (terminal_reader.go, the streamData goroutine), so bubbletea's
//     readLoop ends silently and nothing reaches the event loop: Run keeps
//     ticking the renderer and the model's timers against a dead tty forever.
//     (EIO does surface as an error and ends Run, but it is folded into the
//     same cause here so launch.go can tell a hangup from a normal quit.)
//
// The two channels end the program by different roads, because bubbletea's
// kill path is only safe once the input reader has stopped:
//
//   - A failed read cancels the context handed to tea.WithContext. bubbletea
//     treats a cancelled external context as a kill: the event loop returns,
//     the final frame is skipped (stopRenderer(kill=true)), and what little
//     shutdown still writes fails fast with EIO on a hung-up tty rather than
//     blocking. The read that failed was the reader goroutine's last, so
//     nothing is left reading when shutdown closes the reader.
//   - SIGHUP asks the program to quit (p.Quit) instead. A kill here would race:
//     shutdown(kill=true) cancels the cancelreader but skips waitForReadLoop,
//     then Closes the reader's cancel pipe while the woken reader goroutine is
//     still in kqueueCancelReader.wait calling Fd on that same *os.File
//     (cancelreader v0.2.2, bubbletea v2.0.8 through v2.0.10). The race
//     detector kills the process with exit 66 for it. The graceful path waits
//     for the read loop (bounded at 500ms) before it closes anything. Its
//     final frame and terminal reset write to the tty, which either succeeds
//     (a SIGHUP with the terminal still attached) or fails fast with EIO (a
//     truly hung-up one).
//
//     SIGHUP ──▶ hup=true ──▶ p.Quit() ──▶ graceful shutdown (waits for reader)
//     EOF/EIO ─▶ cancel(errTerminalGone) ──▶ kill (reader already done)
//
// gone() reports either one, so launch.go skips its post-Run output for both.
type terminalWatch struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	sigs   chan os.Signal
	// hup is set before the SIGHUP path's Quit, so it is visible once Run
	// returns. A cancel cause cannot carry this: cancelling the context is the
	// very kill being avoided.
	hup atomic.Bool
}

// watchTerminal installs the SIGHUP handler. Call start with the program once
// it exists, and stop when Run returns so the handler is released and the
// watcher goroutine exits. The handler goes in before the program is built so
// a hangup during startup is not lost: it waits in the buffered channel until
// start reads it.
func watchTerminal() *terminalWatch {
	ctx, cancel := context.WithCancelCause(context.Background())
	w := &terminalWatch{ctx: ctx, cancel: cancel, sigs: make(chan os.Signal, 1)}

	// Notify installs a real handler even when SIGHUP was ignored at startup,
	// which is exactly the case that needs it.
	signal.Notify(w.sigs, syscall.SIGHUP)
	return w
}

// start turns a SIGHUP into a quit of p. It is split from watchTerminal
// because the program is built from the watch's options, so it cannot exist
// yet when the handler is installed. Quit is safe to call before Run has
// reached its event loop: Send blocks until the loop takes the message, or
// until the program's context ends.
func (w *terminalWatch) start(p *tea.Program) {
	go func() {
		select {
		case <-w.sigs:
			w.hup.Store(true)
			p.Quit()
		case <-w.ctx.Done():
		}
	}()
}

// options returns the ProgramOptions that wire the watch into bubbletea.
func (w *terminalWatch) options() []tea.ProgramOption {
	opts := []tea.ProgramOption{tea.WithContext(w.ctx)}

	// Only a terminal stdin gets wrapped. Anything else (a pipe, /dev/null) is
	// left to bubbletea's own default of opening the controlling TTY, and EOF on
	// a pipe is not a hangup anyway.
	if term.IsTerminal(os.Stdin.Fd()) {
		opts = append(opts, tea.WithInput(&hangupInput{File: os.Stdin, w: w}))
	}
	return opts
}

// gone reports whether the program ended because the terminal went away. In
// that case there is nobody left to show an error to, and nothing to restore.
func (w *terminalWatch) gone() bool {
	return w.hup.Load() || errors.Is(context.Cause(w.ctx), errTerminalGone)
}

// stop releases the SIGHUP handler and ends the watcher goroutine.
func (w *terminalWatch) stop() {
	signal.Stop(w.sigs)
	w.cancel(context.Canceled)
}

// hangupInput is stdin with one change: a read that reports the terminal has
// hung up also cancels the watch.
//
// It embeds *os.File rather than wrapping an io.Reader because bubbletea and
// cancelreader both type-assert the input: term.File (Fd) decides whether raw
// mode is entered, and cancelreader.File (Fd, Name) decides whether reads can
// be interrupted with kqueue/epoll. A plain io.Reader would silently lose both.
// cancelreader calls Read on this value (not on the inner *os.File) once the fd
// polls readable, so every real tty read passes through here.
//
// In raw mode (VMIN=1, VTIME=0) a tty read blocks until at least one byte
// arrives — ctrl+d is the byte 0x04, not an EOF — so a zero-byte read can only
// mean the other end is gone. EIO is the other shape of the same event (a
// revoked tty, or Linux's slave read after master close).
type hangupInput struct {
	*os.File
	w *terminalWatch
}

func (in *hangupInput) Read(p []byte) (int, error) {
	n, err := in.File.Read(p)
	if errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO) {
		in.w.cancel(errTerminalGone)
	}
	return n, err
}
