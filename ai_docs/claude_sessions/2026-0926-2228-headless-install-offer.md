# Session: the backlog offer waits out a headless host install

Session ID: 5d81c282-e16e-4e89-97f9-e80669cb23d2
Date: 2026-09-26
Driven from: cats (session doc there:
`2026-0926-2226-default-plugin-seed-cats-todo.md`)

## Why

cats now installs cats-todo automatically on a fresh machine: catway runs
`plugin.InstallHeadless` in the background on first start (cats `53feaff`).
That install runs this repo's `init --post-install` build step with no
terminal, and its output goes to catway's daemon log. `runInstallOffer`
marks the offer made *before* deciding whether it can ask, so a seeded
install spent the one-time "set up a backlog here?" offer on a log line
nobody reads.

## What changed (`56c5c4d`)

- The cats host now sets `CATS_PLUGIN_BUILD_HEADLESS=1` on installs nobody is
  watching (the seed, and peer sync). A missing terminal alone can't say
  this: a scripted install with stdin redirected has no terminal either, but
  its user reads the hint.
- `init.go`: `hostHeadlessEnvVar` + `hostBuildIsHeadless()` (any non-empty
  value). `runInstallOffer` returns first thing, before the marker, printing
  nothing. The offer then comes on the first install or update a person
  watches (e.g. an update from the cats plugins dialog, which runs in a tab).
- `init_test.go`: `TestInstallOfferWaitsOutAHeadlessInstall`. A headless run
  prints nothing and writes no config dir, and the next non-headless run
  still makes the offer.
- `cats-plugin.toml`: the build-step comment now describes the host's
  current env. It still said the host never passes a terminal or the
  invoking directory.

Known gap, left on purpose: `offerAlreadyMade` also treats an existing
global config dir as prior use. So a seeded user who starts using cats-todo
before any watched install or update is never offered, and by then they
know the tool.

## Verification

- `go vet .`, `go test .`: green.
- The built binary: `CATS_PLUGIN_BUILD_HEADLESS=1 cats-todo init
  --post-install` (scratch `CATS_TODO_CONFIG_DIR`) printed nothing and
  created nothing. The next run without it printed the hint and wrote
  `.install-offered`.

## Next

Closed: None. Declined: None. Raised: N-069.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
