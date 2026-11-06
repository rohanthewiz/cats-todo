# Session: typed drops — pane.send_keys in cats, typing in cats-todo

Date: 2026-11-05

The ask, dropped from the backlog: Next List item N-072, road (a). Claude
Code (2.1.283 on) hands a pasted prompt to the model as `<pasted_content>`,
which the model follows only where the user's own words ask, and a drop is
all paste. The interim (b), `pasteAsk` ahead of every drop, was already in.
Left to do: a key-sending verb in cats' wire, the pin bump here, drops sent
as keys with shift+Enter for newlines, and a live check that Claude Code
doesn't take a fast key burst for a paste.

cats is committed and pushed (`8a9937c`, needed for the pin). cats-todo's
side is in the working tree, not committed. It is not released; typed
delivery is a new capability, so the next release is a minor bump.

## cats: `pane.send_keys`

- **wire** (`wire/vocab.go`): `CmdPaneSendKeys`, `SendKeysParams{Pane, Keys}`,
  `KeyStroke{Text | Code, Key, Mods}`. Validate refuses an empty call, a
  stroke that is both shapes or neither, control characters in typed text (a
  newline is Enter or shift+Enter, the caller's call), invalid UTF-8, and
  over 4096 keys in one call.
- **The rune table** (`internal/inputenc/typed.go`, untagged): `RuneKey` (US
  layout; a rune with no key rides as text with no code, as an IME delivers
  it), `NamedKey` (the `.key` a browser pairs with a code), `StrokeKeys`.
- **catway** `orch.SendKeys`: every event encoded press + release through the
  pane's own key encoder, all of it before anything is sent (a refused key
  types nothing), then ONE input message. No pacing on the loop goroutine:
  the caller paces.
- **Dispatcher**: send_input's gates in the same order and words (unknown
  pane, host down, locked workspace). Recorded, with `keys` plain like
  send_input's `text`.
- **catctl** `type <pane> <text...>` and `keys <pane> <CODE[:mods]...>` (mods
  c s a m, as probe spells them).
- Docs: `docs/protocols/control-api.md`, `docs/reference/cli.md`.
- Tests: `TestSendKeysValidate`, `TestRuneKey`, `TestStrokeKeys`,
  `TestTypedStrokes` (ghostty goldens), `TestDispatchSendKeys(Gated)`,
  `TestBuildTypeKeys`. Plain and `-tags ghostty` suites green.

One surprise in the goldens: with kitty off, ghostty still encodes
shift+Enter as `CSI 27;2;13~`, never a bare `\r`. So a typed newline can't
turn into a submit in either mode.

## cats-todo: typing.go

`deliverPrompt(d, pane, agent, text, submit, hasImages)` is now the one
choice point. `typesKeys(agent, text)` says type when the agent is claude and
the text is prose. Otherwise, and when the server answers `"pane.send_keys"
not supported` (`errNoSendKeys`, nothing typed yet), `pastePrompt` (the old
`deliverPrompt`) pastes it with `pasteAsk`.

- Not typed: `/…` (slash commands run; the typed `/` menu's Enter could pick a
  longer command), a leading `!` (bash mode, seen live), control characters
  other than line breaks and tabs, any non-claude agent.
- `typeBatches`: at most 64 keys per call, shift+Enter per line break, CRLF/CR
  folded, a tab as 4 spaces (a typed Tab completes in Claude Code).
  `typeText` sleeps 10ms after each call, then sends Enter. A failure after
  the first call says "part-way (i of n chunks sent)" and never pastes on
  top of it.
- Typed images: no `[Image #` wait. The attach is Claude Code's paste
  handling, so a typed path stays a path, read as the image block says.
- The batch loop: the finish goes through `deliverPrompt`, the between
  command through `deliverLine` (typed or pasted bare, never with the lead).
- `client.sendKeys`, `paneDriver.sendKeys`; go.mod pins cats `8a9937c`.
- Tests: `TestTypesKeys`, `TestTypeBatches`, `TestDeliverPromptTyped`,
  `TestDeliverPromptTypedFailsPartWay`, `TestDeliverLine`,
  `TestDropIntoPaneTypes`; `TestDropIntoPaneLeadsWithTheAsk` now plays an
  old server; `TestDeliverPromptImages` pins the paste road.

## Live (a dev catway, Claude Code 2.1.318, Haiku 5.5)

Cats.app's own server doesn't have the verb, so a dev `catway` + `cathost`
from the new build ran on `127.0.0.1:8431` with scratch config, state dir and
sockets (`-persist=false`), and were stopped by PID afterwards.

- Named keys: `↓` then `Enter` answered Claude Code's trust dialog.
- Burst threshold: one call of 1649 typed chars lost its head (the input
  started at "141 word142", footer "paste again to expand"). Chunked sweeps at
  5ms: 64…700 chars per call arrived intact, 1000 became `[Pasted text #2]`.
- Provenance: the framed "Next list item N-001 … reply ALPHA11" prompt, typed,
  answered `ALPHA11 / TYPED`; the same text through send_input answered
  `ALPHA11 / PASTED`. The test tells the two apart.
- Through cats-todo's own `performDrop` (a throwaway `livedrop`-tagged test,
  not committed): `/clear` pasted, the prompt typed (its tab as 4 spaces) →
  TYPED. A 3,400-char, 44-line prompt arrived whole ("ALPHA11 40") → TYPED.
- `@README.md` mid-text and at the end: shift+Enter and Enter both worked with
  the file picker open.
- `?` typed first is harmless; `!` switched to bash mode (and `# note` after
  it stayed in bash mode).
- Open (N-096): under the feedback survey, typing `2 things to fix` left the
  text whole and closed the survey; whether it also rated the session is
  unseen.

## Left

- N-079 can now use `client.sendKeys` for `↓ ←/→ s` in the pickers.
- Commit cats-todo's tree; release as a minor.
