# Prompt editor autosave, released as v0.34.0

Session: `aa9e24d4-2916-423a-84bd-632a48cc0115`
Date: 2026-09-24

## Ask

1. "In the prompt editor autosave after 1 min."
2. "Make the autosave time configurable and default it to 45s."
3. Save the session, commit, and release v0.34.0.

## What was built

### The autosave (`autosave.go`, new)

- **Throttle, not debounce.** The first change since the last write arms one
  tick. Later changes ride on that tick, and it fires a full delay later even
  if you're still typing. A debounce would never fire for someone typing
  steadily, and a steady writer has the most to lose.
- **Change detection by fingerprint.** `formSig()` covers the raw title,
  prompt, JSON-encoded `formSession` and `%+v` of `formAnnots`.
  `watchAutosave` runs on the result of every `Update` and arms when the
  fingerprint differs from `autosave.saved`. This follows the undo history's
  choke-point argument: no editing path can forget to report itself.
  Attachments are left out of the fingerprint on purpose.
- **Generation guard.** A `tea.Tick` can't be cancelled, so each tick carries
  `gen`. `stopAutosave` (called from `backToList`) and `startAutosave` both
  bump it, so a tick from a closed form writes nothing. This is the same
  pattern the hover card uses in `listhover.go`.
- **What gets written.** `autosaveForm` writes title, prompt, session options
  and marks, without leaving the form. It does **not** write images: attaching
  copies files into `images/<id>/` and detaching deletes them, and a timer
  shouldn't do either. ✔ Save still handles attachments.
- **An add becomes an edit.** The first autosave of a new prompt calls
  `st.add` and switches the form to `formMode = formEdit` with the new
  `editID`, and sets `autosave.added`. Later writes update that same todo. The
  new `formIsNew()` (`formMode == formAdd || autosave.added`) replaces the
  `formAdd` check on the title-click road home, and `persistForm` still reports
  "added to … backlog". The scope toggle (`ctrl+g`) and the toolbar's scope
  tag go away after the first autosave, since the prompt already lives in one
  backlog.
- **esc still keeps nothing.** `cancelForm` calls `revertAutosave` before
  `backToList`. An autosaved add is deleted. An autosaved edit gets its
  original title, prompt, session options and marks back from `autosave.orig`
  (its `Session` is cloned when the form opens). If the revert fails, that's
  reported on the list's status line and you still leave the form.
- **Stays quiet.** A tick on an empty prompt or an unavailable backlog writes
  nothing, shows no error, and moves the baseline. A real write failure goes
  on `formErr` as `autosave failed: …` and is retried after the next delay.
  Success sets `formNote = "autosaved 15:04"` and rebuilds the list.

### Wiring in `ui.go`

- `Update` is now `watchAutosave(m.recordUndo(msg))`. `recordUndo` is the old
  `Update` body, with the undo doc kept on `Update`. The watch sees every
  stage, because the form's ⚙ panel (`stageSession`) edits `formSession`
  directly and isn't one of the `editsPrompt` stages.
- `autosaveTickMsg` is handled above the stage switch in `route`, so it fires
  while the form is behind its images, ⚙, spelling or @-file panels.
- `beginAdd` / `beginEditRef` call `startAutosave` last, once every field has
  its opening value.

### Configurable delay (`settings.go`)

- `autosaveSeconds` in `settings.json` is a `*int`, because `0` has to mean
  "off" and so must read differently from a missing key.
- Missing means `defaultAutosave` (45s). `0` or negative turns it off. Values
  below `minAutosave` are raised to 5s, since every autosave reloads and
  rewrites the whole backlog.
- `save()` writes it back like every other field, which is the house rule. A
  test pins that an explicit "off" survives an unrelated save.
- Read once at launch into `m.autosaveEvery`. A zero delay never arms, but the
  watch still starts and stops with the form, so `formIsNew` and cancel behave
  the same either way.

### Tests (`autosave_test.go`)

These cover: opening a form isn't a change; add → autosave → the same todo is
updated → Save reports an add; a stale tick is ignored; an empty prompt stays
quiet; cancel deletes an autosaved add; cancel restores an autosaved edit; the
settings table (missing, explicit, 0, negative, floor); "off" surviving a save;
and "off" never arming. The first version declared a `typeInto` that clashed
with `promptundo_test.go`'s string version, so the tests use the existing
helper.

### Docs

- README: a new `## Autosave` section before `## Undo`, including the setting.
  The title-click paragraph now says esc also discards what the autosave
  wrote.
- `cats-todo-dev` skill: `autosave.go` added to the file map, and
  `autosaveSeconds` added to the settings row.

## Release

- `b5fe5b3 feat(form): autosave the prompt editor, every 45s by default`
- `8547e92 chore(release): v0.34.0` (main.go and cats-plugin.toml), with an
  annotated tag `v0.34.0 — the prompt editor autosaves every 45 seconds
  (configurable)`.
- It's a minor bump because this is a new capability.

## Next

- **Try autosave in a live cats pane** (new). Only the tests have exercised it
  so far. Check that the `autosaved HH:MM` note is readable, that it appears
  while typing without moving the caret, and that esc after an autosaved add
  leaves no row behind.
- **Change the autosave delay from inside the app** (new, optional). Today the
  delay is only read from settings.json at launch. The list's View panel could
  offer it if hand-editing turns out to be a nuisance.
- **Check the chip by eye** (carried over). Look at an info row in cats, both
  plain and highlighted, plus the bar and the menu. If the fullwidth `ｉ` looks
  thin or odd in the fallback font, the fallbacks are a plain `i` padded inside
  the chip, or the `ℹ️` emoji.
- **Send info prompts to gonotes** (carried over). Add a drop target for
  info-marked prompts that delivers them to a notes plugin (gonotes) instead
  of an agent. Probably an Export-like "➦ Send to notes" that is available
  only when `Info` is set, through the cats control socket or a gonotes CLI or
  API. Work out gonotes' intake contract first.
- **Editor flag on the wire** (carried over). cats could add an `Editor bool`
  field (or similar) to `PaneMeta` in `pane.list`, computed from `EditorInfo`.
  Then cats-todo could drop its `editorAgents` copy. This may be mostly covered
  by the `plugin_type` change in v0.33.1, so re-check the premise.
