# Session: "ALT click for multiple cursors did not work" — tracing the alt bit

Session ID: `dc4faafa-0fde-4233-b5ca-51b70d38bc1f`
Date: 2026-08-30

Follow-on to commit `2a25983` — "alt+click drops a caret per line — the
pointer's road into the column mode", released as `v0.21.0` and the one recent
feature with no session doc of its own. The report was that the gesture does
nothing in a live cats pane.

No code changed. This is a diagnosis session: the whole chain from the pointer to
`altClickPrompt` was walked link by link, two of them with a program rather than
by reading, and the feature came out clean at every point I can reach from here.

## What was verified, and how

**1. The app answers the wire bytes correctly.** The decisive test: drive the
*real binary* through a pty and feed it the exact escape sequence an alt+left
press is on the wire. Harness in the session scratchpad (`creack/pty`, a temp
project with a four-line prompt, `\r` to open the form, then):

```go
send(fmt.Sprintf("\x1b[<8;%d;%dM", x, 8+row+1)) // press, SGR is 1-based
send(fmt.Sprintf("\x1b[<8;%d;%dm", x, 8+row+1)) // release
```

The captured frame:

```
┃ line three
┃ line four   2 carets · alt+click adds or removes one
typing goes on every line · backspace deletes · ←/→ moves them · ctrl+a/e line ends · esc ends
```

So `updateMouse → clickForm → altClickPrompt` works end to end against a real
terminal, not just against `tea.MouseClickMsg` in a test.

**2. cats encodes the alt bit correctly.** A throwaway test in cats'
`internal/inputenc` (`go test -tags ghostty`, deleted afterwards) ran the
browser-proto mouse message through the ghostty encoder:

```
mods=0 → "\x1b[<0;11;4M"    mods=2 (alt)   → "\x1b[<8;11;4M"
mods=1 → "\x1b[<4;11;4M"    mods=4 (ctrl)  → "\x1b[<16;11;4M"
```

**3. The rest of the chain, by reading.** catway's `22-mouse.js` sends
`mods: mods(ev)` on every pane mouse event and `20-keys.js` sets bit 2 from
`e.altKey`; `browserproto.ModAlt = 2` matches; `catway.go`'s `handleUp` passes
the whole struct to `enc.Mouse` without rebuilding it; the JS is one shared
closure (`web/assets.go`), so `mods` is in scope where the mouse code calls it;
and ultraviolet's `parseMouseButton` maps `0b0000_1000` to `ModAlt` for a click
event. Nothing in cats claims alt+click for itself (no hit for it anywhere in
that tree).

## Two stale binaries found on the way

- `bin/cats-todo` in this checkout was built **Aug 29 23:21**, before the
  alt+click commit landed at Aug 30 19:44 — `strings` found no caret code in it
  at all. Anything launched from the repo build could only have moved the caret.
  Rebuilt.
- `~/bin/cats-todo` is from **Jul 26** and shadows everything on PATH. Typing
  `cats-todo` in a pane gets a binary a month older than the caret mode. Worth
  deleting or re-pointing.

The installed plugin (`~/.config/cats/plugins/rohanthewiz.cats-todo/bin/`, built
19:47, `v0.21.0`) *does* contain the feature — which is what the user was
running, so neither stale copy explains this report on its own. A pane opened
before 19:47 would still be the old process, though.

## Where it stands

The user was in a cats pane on the plugin binary, and the alt+click **moved the
caret** — which is exactly what a press with the alt bit stripped does. Two
readings remain, and one command in a cats pane separates them:

```sh
printf '\033[?1002h\033[?1006h'; cat -v     # click, alt+click, ctrl+c
printf '\033[?1002l\033[?1006l'
```

- `^[[<8;…M` → the alt bit arrives, the bug is here. The remaining suspect in
  this repo is `altClickPrompt`'s deliberate `cr == r` branch: a press on the
  line the editor's caret is already on is a plain caret move, alt or no alt,
  because one line in play means nothing multiple was asked for. If the report
  reproduces with the press on a *different* line, that branch is wrong about
  something.
- `^[[<0;…M` → cats never put the bit on the wire (the browser side's
  `mods(ev)` did not see `altKey`), and the fix belongs in the front-end.

## The constraint worth writing down

**Alt is the only modifier this gesture can ever use.** An SGR mouse report
carries shift (4), alt (8) and ctrl (16) and nothing else — there is no Super
bit, so ⌘+click cannot reach a TUI at all no matter what the terminal does, and
ctrl+click is right-click on macOS. So there is no alias to add as a fallback:
either the terminal forwards alt on a press or the pointer's road into the
column mode is closed, and the menu's ⌶ Caret on every line is the way in.
