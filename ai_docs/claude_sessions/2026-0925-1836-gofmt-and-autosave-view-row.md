# gofmt on its own (N-052) and the autosave delay on the View panel (N-035)

Session: `6c3de366-73a2-4905-b036-7b359ec5b3bf`
Date: 2026-09-25

## Ask

Two Next List items were pasted in turn. N-052: `gofmt -l` listed
`promptcode.go` and `ui.go` on a clean HEAD, and one formatting commit on its
own would keep that out of feature diffs. N-035: the autosave delay could only
be changed by hand-editing settings.json (`autosaveSeconds`) and restarting;
the list's View panel could offer it. Each was committed (`56f913d`,
`a8ca102`). The session ended with `/sw`.

## N-052 — gofmt

- `ui.go` was only the form-stage field block's alignment (`formMode`,
  `formScope`, `editID`).
- **`promptcode.go` could not take a plain `gofmt -w`.** The doc comment on
  `inlineCodeSpans` had ``` ``a ` b`` ``` in running prose, and gofmt's doc
  comment reformatter reads a double backtick there as an opening quote and
  rewrites it to `“a ` b“`, which breaks the example. The prose now points to
  the same example in the indented block below it (which gofmt leaves
  verbatim) and says why it stays there.
- `gofmt -l .` is empty. Committed as `style: …`, on its own.

## N-035 — Autosave on the View panel

- **A third row, `Autosave`** (`viewRowAutosave`, `ui.go`), a stepper rather
  than a switch: `autosavePresets` = off, 15s, 30s, 45s, 1m, 90s, 2m, 5m.
  `→`, space and a click step longer, `←` shorter (the one key the row reads
  differently from the switches). Both ends wrap, so no press does nothing.
- **`stepAutosave` searches by value, not index**, so a hand-edited 37s steps
  to 45s up or 30s down. `autosaveLabel` prints `off`, whole minutes as `2m`,
  and anything else in seconds; every preset fits the five-cell value column.
- **Its own save (`setViewAutosave`)**, setting only `autosaveSeconds` after a
  fresh `loadSettings`. `saveViewPrefs` is also what `ctrl+d` calls, and
  writing the delay there would put the model's copy back over a hand edit.
- **`beginViewOpts` re-reads the delay from the file**, so the row never shows
  a stale value next to a hand-edited file, and a hand edit now takes effect
  on opening the panel, not only at launch. No form can be open while the
  panel is up, so the next form simply arms with `m.autosaveEvery`.
- With the delay off the row's note says `the editor writes only on ✔ Save`.
  The heading and the notes were shortened so the panel fits 80 columns
  (checked by rendering at 120 and 80); the footer reads `←/→ or space
  change` and `all three are remembered between launches`.
- Tests (`autosave_test.go`): `TestStepAutosave`, `TestAutosaveLabel`,
  `TestViewOptsAutosaveRow` (keys, click, file, the switches untouched, the
  next form arms), `TestViewOptsAutosaveFollowsTheFile` (panel shows a hand
  edit to 0; `ctrl+d` leaves a hand-edited 20s alone).
- README: the View panel's block and the Autosave section's "The wait is a
  setting" paragraph.
- Added to the pending v0.40.0 release item (N-059). It is a small refinement
  of a shipped feature (the patch rule), but that release is already a minor.

`go vet` and `go test ./...` pass. Not live-tested in cats.

## Next

Closed: N-035, N-052. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: N-059. Full list: `ai_docs/todo/next-list.md`.
