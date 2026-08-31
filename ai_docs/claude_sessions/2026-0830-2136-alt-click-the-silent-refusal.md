# Session: the silent refusal behind "alt+click doesn't work" — v0.21.1

Session ID: `dc4faafa-0fde-4233-b5ca-51b70d38bc1f`
Date: 2026-08-30

Second half of the session that begins in
[tracing the alt bit](2026-0830-2003-alt-click-carets-did-not-work.md), which
should be read first: it establishes that the binary answers the alt+click wire
bytes correctly and that cats encodes the alt bit correctly, so the chain is
sound at every point reachable from here.

The re-report, and the detail that turned it:

> "Alt with left and right arrow keys does move the cursor by word, but
> left-click with Alt just moves the cursor instead of generating a new one."

Touched: `promptcarets.go`, `promptcarets_test.go`, `README.md`, and the two
version places. Released as `v0.21.1`, and `catctl plugin update` was run so the
installed plugin carries it.

## The branch that was right and silent

`altClickPrompt` has always had this case, and it is correct:

```go
case !m.carets.on:
    cr := min(max(m.promptArea.Line(), 0), len(rows)-1)
    if cr == r {
        m.placePromptCursor(x, row)
        m.formNote = ""      // ← this
        return m, cmd
    }
```

A row carries at most one caret (`indexOf` enforces it, and every edit in the
mode is per-row), so a press on the line the caret is already on has no second
caret to put down. It is a plain move, alt or no alt.

What makes that a bug rather than a rule is the `""`. **A long line that soft
wraps is drawn on several rows of the box but is one line**, and a prompt is
usually exactly that shape — one long paragraph. So a hand aiming at what look
like two lines gets a plain caret move twice, in silence, which is byte for byte
the same experience as the terminal eating the modifier. Two very different
causes, one indistinguishable symptom, and no way for the user (or me, at a
distance) to tell them apart. That is contract #4 — *refuse in words* — and it
was being broken by an empty string.

Now:

```
the caret is already on that line — alt+click another line
```

The note earns its place twice over: it names the reason, and **by appearing at
all it proves alt reached the program**. No note and no second caret means the
modifier never arrived, which is a cats-side question, not a cats-todo one.

## Verifying it the way the first half verified everything

The pty harness from the earlier doc was reused with a different fixture — a
single line of `"wrap " × 60` in a 120-cell pane — and the real binary, driven
by the real escape sequence, answered:

```
| rathe caret is already on that line — alt+click another linerara
```

(the interleaved `ra`/`rathe` is the wrapped body bleeding through the harness's
crude ANSI stripper, not the app).

`TestAltClickOnAWrappedLineSaysSo` pins it in-process: it asserts the fixture
really does wrap onto a second display row *of the same logical row* before
pressing there, so the test fails loudly if a future textarea change stops
wrapping and quietly turns the regression test into a tautology.

## Notes for next time

- **The remaining unknown is still one command.** In a cats pane:
  `printf '\033[?1002h\033[?1006h'; cat -v`, click, alt+click, ctrl+c, then
  `printf '\033[?1002l\033[?1006l'`. `^[[<8;…M` = alt arrived (8 = 0 + the alt
  bit); `^[[<0;…M` = cats stripped it. The new note answers the same question
  from inside the app, which is why it was worth adding.
- **Driving cats from inside a pane works** — `CATS_CONTROL_SOCKET` is in the
  environment, so `catctl split/run/capture/close` can build a probe pane. It
  did not pay off here: a pane split into a workspace no window is currently
  showing captured as empty text, so the probe never ran. Focus the workspace
  first if this is tried again.
- **A running plugin pane is a running old process.** `catctl plugin update
  rohanthewiz.cats-todo` rebuilds the installed binary, but an open cats-todo
  keeps the one it started with. Every "did not work" report should first
  establish which binary was in the pane — `~/bin/cats-todo` (Jul 26!) still
  shadows PATH.

## The design limit this exposed, and did not fix

A line holds one caret. Real multi-cursor editors let alt+click drop a second
cursor *on the same line*, and on a wrapped paragraph that is very likely what
the hand is asking for. Expressing it means `promptCarets` stops being two
parallel row/column arrays with `indexOf(row)` and becomes several columns per
row, with every per-row edit in the mode (`replacePromptRunes` and the motions)
folding them right-to-left so the earlier offsets stay valid. That is a real
change to the mode's core, not a patch, and it is left as an offer rather than
made on a guess about what was wanted.
