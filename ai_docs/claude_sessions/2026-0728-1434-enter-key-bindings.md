# Session: enter-key bindings + the bubbletea v2 upgrade that makes them possible

Session ID: `b52bf46d-032c-4218-a370-8071ef5890eb`
Date: 2026-07-28

## The ask

Put the two most common actions on `enter` — enter on an empty list adds a
todo, enter in the editor saves it — and move the newline, plus the drop into
Claude, onto a modifier+enter chord. Which modifier was left open for
discussion.

## The constraint that shaped everything

Checked before proposing anything, because it decides what is even bindable:

- **`alt+enter` works today.** cats encodes alt as an ESC prefix
  (`internal/inputenc`), and bubbletea parses `ESC CR` as `alt+enter`.
- **`shift+enter` could not work on bubbletea v1.** cats only emits a
  distinguishable shift+enter under the kitty keyboard protocol (gated on
  `KittyKeyboardFlags`, `internal/inputenc/encoder.go`), and v1 never requests
  it and cannot parse CSI-u. Shift+enter arrived as a bare `CR` — byte-identical
  to enter.
- **`ctrl+j`** (raw LF) works in every terminal regardless.

Two decisions came back from the user: **enter on a selected todo opens the
edit form** (not view, not drop), and **upgrade to bubbletea v2 first** so
shift+enter becomes a real key rather than settling for alt+enter alone.

## The keymap

| Where | `enter` | `shift+enter` = `alt+enter` |
|---|---|---|
| List | edit the highlighted todo; add one when nothing is highlighted | drop into an agent |
| Editor | save | insert a newline (`ctrl+j` also) |
| Prompt view | edit | drop |
| Target picker | paste, staged for review | drop & run (`ctrl+r` kept) |

`ctrl+a` / `ctrl+e` / `ctrl+s` stay bound; the list footer drops `ctrl+e` to
make room and now reads `enter edit · shift+enter drop · …`.

Both chord spellings are bound to the same handlers **always** — the binding
never depends on what the terminal negotiated. Only the *advertised* name
adapts: `model.kbEnhanced` flips on `tea.KeyboardEnhancementsMsg` and
`modEnter()` returns `shift+enter` once the terminal answers the kitty
handshake, `alt+enter` until then. Every footer builds its hint from it.

The empty-list hint changed from "press ctrl+a to add one" to "press enter".

### The ctrl+m trap

`textarea`'s default `InsertNewline` binds **`enter` and `ctrl+m`**. `ctrl+m`
*is* the CR that plain enter sends on a legacy terminal — leaving it in place
would have meant enter still inserted a newline there instead of saving, on
exactly the terminals least able to send shift+enter. The rebinding drops it:

```go
ta.KeyMap.InsertNewline = key.NewBinding(
    key.WithKeys("shift+enter", "alt+enter", "ctrl+j"), …)
```

Rebinding the textarea's own keymap (rather than intercepting the chord in
`updateForm` and forwarding a synthetic enter) keeps the textarea in charge of
its own editing.

## The upgrade

bubbletea v2.0.8 / bubbles v2.1.1 / lipgloss v2.0.5. **The v2 modules moved to
`charm.land/*` import paths** — `github.com/charmbracelet/bubbletea/v2` does
not resolve; the module declares itself as `charm.land/bubbletea/v2`.

v2 requests kitty basic key disambiguation on every frame by default
(`keyboardEnhancementsFlags` starts at `flags := 1`), which is the whole reason
shift+enter is now distinguishable. Nothing had to be opted into.

API changes absorbed:

- `View() string` → `View() tea.View`. The alt screen and window title are
  **properties of the view**, set every frame, not startup options — so
  `tea.WithAltScreen` and `tea.SetWindowTitle` are gone. `Init` now only starts
  the cursor blink. The renderer diffs the title, so re-declaring it per frame
  costs nothing, and v2 clears it on teardown (the explicit OSC-2 clear in
  `runTodoUI` stays as a backstop for an exit that skips the renderer).
- `tea.KeyMsg` → `tea.KeyPressMsg` (KeyMsg is now an interface). All five
  `update*` stage handlers take the concrete press type.
- `textinput.Width` field → `SetWidth()`; `viewport.New(w, h)` → option funcs.
- Virtual cursors stay on by default in both textinput and textarea, so no
  cursor wiring was needed in the views.

## Tests

- `model_test.go`: per-stage binding tests — list enter (empty / selection /
  filter-matching-nothing), list chord drops (both spellings), form enter-saves
  vs all three newline keys, picker chord routing. Existing tests ported to v2
  key construction (`tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}`),
  and the view-stage test updated for enter→edit.
- **`keys_e2e_test.go` (new)**: drives the actual program with the raw bytes a
  terminal sends — `\r` opens the add form on the empty list, `\x1b\r` (a
  legacy alt+enter) inserts a newline mid-prompt, `\r` saves
  `"line one\nline two"`, `\r` reopens it, `\x03` exits clean. This is the only
  layer that proves the wire encoding maps to the intended binding; the
  hand-built `KeyPressMsg` tests cannot.

  Gotcha found while writing it: the parser reads a buffer at a time, so an ESC
  byte in front of `\x03` is read as one chord (`ctrl+alt+c`) and the quit never
  fires. The test comment records this.

Version bumped 0.2.3 → **0.3.0** (`main.go` + `cats-plugin.toml`) — the keys
changed incompatibly and the TUI framework moved.

## Standing notes

- The wire-contract copies in `internal/{app,ctlproto,integration}` must stay
  in lockstep with cats (unchanged this session — no wire changes).
- Terminals that cannot speak the kitty protocol degrade predictably: a bare
  shift+enter reads as plain enter (so it edits/saves instead of dropping), and
  the footers name `alt+enter` there so the user is never pointed at a key the
  terminal cannot send.
