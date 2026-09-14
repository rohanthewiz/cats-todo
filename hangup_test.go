//go:build darwin || linux

package main

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/sys/unix"
)

// hangupHelperEnv switches TestHangupHelperProcess from a skip into the
// program under test. The real test re-executes this test binary with it set,
// because a hangup has to be observed from outside: the process that owns the
// pty is the one being torn down.
const hangupHelperEnv = "CATS_TODO_HANGUP_HELPER"

// TestHangupHelperProcess is the child side of TestProgramExitsOnHangup. It
// runs the real runProgram wiring over a model that never quits, so the only
// way this process exits 0 is runProgram returning on a hangup.
func TestHangupHelperProcess(t *testing.T) {
	if os.Getenv(hangupHelperEnv) != "1" {
		t.Skip("child process of TestProgramExitsOnHangup")
	}
	// A cats pane inherits SIGHUP ignored from cathost. Ignoring it here puts
	// the helper in the same state, so the sighup case proves the handler
	// overrides the inherited disposition rather than the default action
	// killing the process for us.
	signal.Ignore(syscall.SIGHUP)
	runProgram(idleModel{})
	os.Exit(0)
}

// idleModel renders a screen and ignores every message: nothing inside the
// program can end it.
type idleModel struct{}

func (idleModel) Init() tea.Cmd                         { return nil }
func (m idleModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (idleModel) View() tea.View {
	v := tea.NewView("waiting for the terminal to hang up")
	v.AltScreen = true
	return v
}

// TestProgramExitsOnHangup runs the TUI on a real pty and takes the terminal
// away, once per channel a hangup arrives by. Before terminalWatch both cases
// hung until the deadline: the EOF was swallowed inside ultraviolet's reader,
// and the SIGHUP had no handler over an ignored disposition.
func TestProgramExitsOnHangup(t *testing.T) {
	cases := []struct {
		name string
		// hangUp takes the terminal away. It reports whether it closed the
		// master, so the wait loop knows whether there is still output to drain.
		hangUp func(t *testing.T, master int, cmd *exec.Cmd) (closedMaster bool)
	}{
		{
			// The pty master closes — the terminal app's backend died. The
			// helper has no controlling tty (Setsid without Setctty), so no
			// SIGHUP is generated: this isolates the EOF-on-read path.
			name: "master closed",
			hangUp: func(t *testing.T, master int, _ *exec.Cmd) bool {
				if err := unix.Close(master); err != nil {
					t.Fatalf("close master: %v", err)
				}
				return true
			},
		},
		{
			// SIGHUP with the master still open and still drained, so reads
			// never fail: this isolates the signal path.
			name: "sighup",
			hangUp: func(t *testing.T, _ int, cmd *exec.Cmd) bool {
				if err := cmd.Process.Signal(syscall.SIGHUP); err != nil {
					t.Fatalf("signal helper: %v", err)
				}
				return false
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			master, slave, err := openPTY()
			if err != nil {
				t.Skipf("no pty available: %v", err)
			}

			cmd := exec.Command(os.Args[0], "-test.run=^TestHangupHelperProcess$")
			cmd.Env = append(os.Environ(), hangupHelperEnv+"=1", "TERM=xterm-256color")
			cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			if err := cmd.Start(); err != nil {
				t.Fatalf("start helper: %v", err)
			}
			// Only the child holds the slave from here on, so closing the master
			// is a true hangup rather than one reference among two.
			_ = slave.Close()

			masterOpen := true
			defer func() {
				if masterOpen {
					_ = unix.Close(master)
				}
			}()

			// Wait for the first frame, then for the output to go quiet, so the
			// hangup lands on a program already inside its event loop rather than
			// one still starting up (which would fail for a different reason).
			if drainPTY(master, 10*time.Second, true) == 0 {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatal("helper never drew to the pty")
			}
			drainPTY(master, 300*time.Millisecond, false)

			if tc.hangUp(t, master, cmd) {
				masterOpen = false
			}

			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			deadline := time.Now().Add(5 * time.Second)
			for {
				select {
				case err := <-done:
					if err != nil {
						t.Fatalf("helper did not return from runProgram cleanly: %v", err)
					}
					return
				default:
				}
				if time.Now().After(deadline) {
					_ = cmd.Process.Kill()
					<-done
					t.Fatal("program still running 5s after the terminal hung up")
				}
				// Keep reading while the master is open: a pty whose buffer fills
				// blocks the child's writes, which would look like the hang this
				// test is about.
				if masterOpen {
					drainPTY(master, 50*time.Millisecond, false)
				} else {
					time.Sleep(50 * time.Millisecond)
				}
			}
		})
	}
}

// drainPTY reads and discards output from a pty master and returns how many
// bytes it read. With untilData it returns as soon as a read succeeds (or after
// d with nothing); otherwise it reads until d passes with no output. Polling
// instead of a blocking read keeps the master closable from the same
// goroutine: on darwin a close does not take effect while another thread is
// parked in read(2) on the fd.
func drainPTY(master int, d time.Duration, untilData bool) int {
	buf := make([]byte, 4096)
	total := 0
	for {
		fds := []unix.PollFd{{Fd: int32(master), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, int(d/time.Millisecond))
		if err == unix.EINTR {
			continue
		}
		if err != nil || n == 0 || fds[0].Revents&unix.POLLIN == 0 {
			return total
		}
		r, err := unix.Read(master, buf)
		if err != nil || r <= 0 {
			return total
		}
		total += r
		if untilData {
			return total
		}
	}
}
