# SIGHUP no longer kills bubbletea under a live reader (N-045)

Session: `aeb0bc13-056a-4a78-8700-522ad0a40773`
Date: 2026-09-26

## Ask

Work N-045: `TestProgramExitsOnHangup/sighup` failed under `go test -race`
with the helper's exit 66, the race detector's code.

## 1. Reproducing it

- In isolation it passed 20 of 20 runs, and the full `-race` suite passed 3
  of 3. That's why the raising session saw it come and go.
- It reproduced with a race-built test binary run at `-test.cpu` 1/2/4/8
  while six `yes > /dev/null` processes loaded the machine: 8 of 60 runs
  failed.
- The report goes to the helper's pty, so the item suggested teeing the
  drained bytes. `GORACE=log_path=<scratch>/race` writes it to a file instead,
  with no test change needed.

## 2. Cause: bubbletea's kill path (not our code)

All eight reports were the same race:

- **Write:** `bubbletea.(*Program).shutdown` → `kqueueCancelReader.Close`
  closes the cancel pipe's `*os.File` (`cancelreader_bsd.go:109`).
- **Read:** the reader goroutine, woken by the cancel, is still in
  `kqueueCancelReader.wait` calling `Fd()` on that file (`:141`).

`terminalWatch` turned SIGHUP into a cancelled external context, which
bubbletea treats as a kill. `shutdown(kill=true)` calls `Cancel()` but skips
`waitForReadLoop` (`if !kill`), then `Close()`s at once. The code is
unchanged in bubbletea v2.0.10, so a bump wouldn't help. The master-closed
case never raced: EOF ends the read loop before the cancel, so nothing is
reading when the reader is closed.

## 3. Fix (`hangup.go`, `launch.go`)

```
SIGHUP ──▶ hup=true ──▶ p.Quit() ──▶ graceful shutdown (waits ≤500ms for reader)
EOF/EIO ─▶ cancel(errTerminalGone) ──▶ kill (reader already done)
```

- `watchTerminal()` still installs `signal.Notify` up front, so a SIGHUP
  during startup waits in the buffered channel. The watcher goroutine moved
  to a new `(*terminalWatch).start(p *tea.Program)`, which `runProgram` calls
  right after `tea.NewProgram`. The program is built from the watch's
  options, so it cannot exist when the handler goes in.
- On SIGHUP it sets `hup atomic.Bool`, then calls `p.Quit()`. `p.ctx` is made
  in `NewProgram`, so `Send` before `Run` just blocks until the event loop
  takes it, or until the program's context ends.
- `gone()` is `hup || cause == errTerminalGone`, so `runProgram` still skips
  its post-Run error print and OSC reset for both kinds of hangup.
- The trade-off: the graceful path renders a final frame and restores the
  terminal. On a truly dead tty those writes fail fast with EIO. With the
  terminal still attached (the test's case) they succeed.
- The type comment in `hangup.go` now explains the two exits, with the
  diagram above. The sighup test case's comment records the race and how it
  showed up.

## 4. Verification

- Same stress after the fix: 0 of 60 failed, and no race logs were written.
- `go vet .`, `go test ./...` and `go test -race ./...` are all green.
- No README change: nothing a user sees changed.

## Next

Closed: N-045. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
