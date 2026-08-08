# Per-todo session options for drops

## Context

Today a drop delivers one thing: the prompt text (plus attachment paths). How the
receiving agent *runs* is whatever the default is — `claude` with no flags, no prior
context loaded, and no instruction about what to do once the work is finished. So a
prompt that wants a cheap model at low effort, or one that should start from the last
saved session doc and end with `/sess-wrap`, has to be set up by hand every single time
it is dropped.

The fix is to make those choices a property of the *prompt*, stored with it in
`todos.json` and travelling with the repo, so a drop — manual or scheduled — reproduces
the whole setup. Three delivery mechanisms carry it:

| Option | How it reaches the agent | Applies to |
|---|---|---|
| model / effort / permission-mode | `claude` launch flags on the tab's argv | new-session drops, `claude` only |
| clear first | `/clear` as its own submitted message | existing-pane drops |
| context loading, wrap-up | preamble/postamble text in the prompt body | every drop |

`claude --help` confirms the flag spellings: `--model <alias\|full-id>`,
`--effort low\|medium\|high\|xhigh\|max`, `--permission-mode
acceptEdits\|auto\|bypassPermissions\|manual\|dontAsk\|plan` (the last two of those were
missing from the original ask).

## Design

### 1. `SessionOpts` — the record (new file `session.go`)

Add one nullable field to `Todo` in `store.go`, following the exact compat contract
`Images`/`Schedule` already document (`omitempty`, older binaries ignore it):

```go
Session *SessionOpts `json:"session,omitempty"`
```

```go
type SessionOpts struct {
    Model      string   `json:"model,omitempty"`      // "sonnet" | "claude-opus-5" | ""
    Effort     string   `json:"effort,omitempty"`     // low|medium|high|xhigh|max
    Permission string   `json:"permission,omitempty"` // canonical claude spelling
    Clear      bool     `json:"clear,omitempty"`      // /clear an existing pane first
    Context    string   `json:"context,omitempty"`    // ctxNone|ctxLoad|ctxUse
    ContextArg string   `json:"contextArg,omitempty"` // "2" for sess-load, pattern for sess-use
    Files      []string `json:"files,omitempty"`      // extra files to read, repeatable
    Finish     string   `json:"finish,omitempty"`     // finishNone|finishCommit|finishPush|finishWrap
    Reviews    []string `json:"reviews,omitempty"`    // code-review | security-review | simplify
    Release    bool     `json:"release,omitempty"`
}
```

Empty string always means "inherit the default" — a zero `SessionOpts` (and a nil
pointer) must behave exactly as today. The string constants are wire format, like
`scheduleKindPane`.

`session.go` also holds the pure logic, which is where the tests go:

- `normalizeModel/Effort/Permission(string) (string, error)` — accept the friendly
  spellings the user asked for (`accept-edits` → `acceptEdits`, `bypass` →
  `bypassPermissions`) and reject anything else. Model is *not* validated against a
  list: aliases and full IDs both pass through, since new model names ship faster than
  this binary does.
- `(o *SessionOpts) launchArgs(command string) []string` — `--model/--effort/
  --permission-mode` for `claude` only; empty slice for any other agent (chosen
  behaviour: drop the flags, keep the text).
- `(o *SessionOpts) preamble() string` / `postamble() string` — the text blocks.
- `(o *SessionOpts) summary() string` — the one-line `⚙ sonnet · high · acceptEdits ·
  sess-load 2 · wrap` shown in the form and the picker.
- `sessSkillsAvailable() bool` — the sess-* gate. Checks for `sess-load.md` in
  `~/.claude/commands/`, `$CLAUDE_CONFIG_DIR/commands/`, and the project's
  `.claude/commands/` (that is where they actually live on this machine — *not*
  `skills/`). Used only to grey the context rows out with a "no sess-* skills found"
  note; it never blocks a save, since a backlog is committed and travels to machines
  that do have them.

### 2. Composing the prompt (`drop.go`)

`composePrompt` currently takes `(prompt, images)`. Extend it to
`composePrompt(prompt string, images []string, opts *SessionOpts)` and have it layer:

```
First, load prior context: run /sess-load 2
Also read these files: ai_docs/design.md

<prompt body>

Attached images — read these files:
/abs/path.png

When the work is done and the tests pass:
- run /code-review
- run /sess-wrap
- cut a release
```

Rules that keep it honest: the image block stays where it is relative to the body (its
existing tests pin that); the postamble is omitted entirely when nothing is set; the
result still never ends in a newline (`sendInput`'s contract). Keep the block headers as
named constants next to `imageBlockHeader`.

Wrap-up wording per `Finish`: none → nothing; `commit` → "commit the work"; `push` →
"commit and push"; `wrap` → "run /sess-wrap (saves a session doc, commits, and pushes)".
`Release` appends a release line, phrased against the repo's own convention (bump
`version` in `main.go` and `cats-plugin.toml`, `chore(release): vX.Y.Z`, tag).
Exiting the agent is deliberately **not** offered.

### 3. Delivering it (`drop.go`)

- `dropIntoNewSession`: argv becomes `append(strings.Fields(command),
  opts.launchArgs(command)...)`. Everything downstream — `tabCreate`,
  `waitForAgentReady`, `sendInput` — is untouched.
- `performDrop`, `targetExistingPane`: when `Clear` is set, `sendInput(pane, "/clear",
  true)` first, then the existing send. `/clear` is a built-in, not a skill, so it must
  be its own submitted message. A failure on the `/clear` call aborts the drop rather
  than typing the prompt into an unknown state.
- `performScheduledDrop` needs no change: `performScheduledDropCmd` (ui.go:2348) already
  builds its `pendingAction` from the live `td`, so a scheduled fire picks up whatever
  the todo's options say at fire time.

`pendingAction` reads the options off `act.todo.Session` — no new field needed.

### 4. Persisting it (`store.go`)

Add `setSession(id string, o *SessionOpts) error`, mirroring `setImages`
(store.go:297) exactly — and for the same stated reason: `update()` stays a text-only
operation, so a caller that knows nothing about session options cannot blank them.

### 5. The editor (`ui.go`) — a sub-stage of the form

New `stageSession`, built as a mirror of `stageImages`:

- `beginSession()` on **`ctrl+e`** (ctrl+s is save, ctrl+o is images) and from a new
  `⚙ Session` toolbar chip appended to `formActions()` (ui.go:3079) with a matching
  `formActionSession` index in the const block and a `clickFormBar` case.
- The panel is a cursor over ~9 rows. `up`/`down` move; `left`/`right` (and `space`)
  cycle the enum rows through their values; the two free-text rows (context arg, files)
  use a `textinput` like `imgInput`, with the file row repeatable via enter/ctrl+x the
  way `addFormImage`/`removeFormImage` work. `esc` closes back to the form.
- `closeSession()` restores focus to the field that had it, exactly as `closeImages`
  (ui.go:1472).
- The form's state gains `formSession SessionOpts`, seeded in `beginAdd`/`beginEditRef`
  from the todo, and written by `saveForm` via `st.setSession` — placed alongside the
  `setImages` call so the same "record, then reconcile" ordering holds.
- `viewForm` gains one line under the 📎 line: `⚙ ` + summary, or "default session" when
  nothing is set. `viewSession()` renders the panel; `targetDesc`/the picker's
  new-session rows note when flags will be dropped for a non-claude agent.
- The list row already shows markers for images/schedules — add a `⚙` marker there and
  in `viewContent` so a configured prompt is visible without opening it.

### 6. CLI parity (`cli.go`)

`addFromCLI` gains, using the existing `stringList` type for the repeatables:

```
--model <m>  --effort <l>  --perm <mode>  --clear
--sess-load [n]  --sess-use <pattern>  --ctx <file>   (repeatable)
--finish none|commit|push|wrap  --review <skill>      (repeatable)
--release
```

They run through the same `normalize*` functions, so a bad value exits with the same
message the TUI shows. `--sess-load` and `--sess-use` are mutually exclusive.
`complete.go` (the shell-completion table) gets the new flags too.

## Files touched

`session.go` (new), `session_test.go` (new), `store.go` (Todo field + `setSession`),
`drop.go` (`composePrompt`, `dropIntoNewSession`, `performDrop`), `ui.go` (stage, form
state, panel view, toolbar chip, list marker), `cli.go`, `complete.go`, `README.md`.

## Verification

1. `go build ./... && go test ./...` — the existing suites (`ui_test.go`,
   `model_test.go`, `formmouse_test.go`, `schedule_test.go`, `store_test.go`) must stay
   green; `composePrompt`'s current tests pin the no-options output byte for byte, which
   is the regression guard that a nil `Session` changes nothing.
2. New table tests in `session_test.go`: normalization (good and bad values, alias
   folding), `launchArgs` for claude vs copilot vs empty opts, and `composePrompt`
   across the matrix of preamble/postamble/images present and absent.
3. New store test: a todo saved with options, reloaded, round-trips; and a `update()`
   after that leaves `Session` intact (the same guarantee `setImages` has a test for).
4. TUI tests in the existing style: `ctrl+e` opens the panel, cycling a row changes the
   value, esc returns focus to the field that had it, and save writes the options to
   disk.
5. Manual, in a cats pane: `cats-todo add --model sonnet --effort low --finish wrap
   "say hi"`, then drop it into a new session — confirm the tab's argv carries
   `--model sonnet --effort low` (`ps` on the pane) and the prompt shows the wrap-up
   block. Then drop a `--clear` todo into an existing pane and confirm `/clear` lands
   as its own message before the prompt.
6. `go vet ./...`.

## Wrap-up

Per the ask, once this is built and green: cut a release — bump `version` in `main.go`
and `cats-plugin.toml` to **v0.11.0**, README section for session options,
`chore(release): v0.11.0`, tag, push.
