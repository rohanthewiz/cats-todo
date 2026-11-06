# Export announces a new backlog (N-080)

Session: `9270567b-330f-42fc-be36-e058f8d4cc18`

## Ask

N-080 from the Next List. Exporting into a project directory with no
`.cats-todo` created the backlog without a word. The manager's launch asks
before creating one (`backlogoffer.go`), and `add` says so when it starts one,
but export did neither. The item offered two fixes: say it in the status line
("started a new backlog in <dir>"), or ask first.

## What was done

Said, not asked. The picker row already reads `no backlog yet — will be
created` before the user picks it (the Browse row's description says the same),
so a confirm after the choice would be a second question about one decision.

- **`performExport`** (export.go): before the first write, a project-scope
  destination whose root has no `.cats-todo` directory is noted (`newRoot`).
  This is the same directory marker the launch offer and `add` use
  (`projectBacklogMissing`). After the run, if the directory now exists, the
  status line gets `started a new backlog in <dir> — git add .cats-todo when
  you are ready`.
- The check happens after the run, not by counting successful transfers. A
  failed first transfer can still leave the directory behind, because
  attachments are copied in before the save. The failure line carries the
  note as well.
- The note comes first among the trailing notes, ahead of `N already there` and
  the dropped-schedules note. It is the one consequence the user did not watch
  happen.
- **`startedBacklogNote`** builds the clause, with the same `git add` hint the
  launch offer and `add` give.
- The check also covers a global prompt's "This project" row. The launch offer
  has normally created that backlog already, but a `.cats-todo` removed while
  the manager is open would otherwise come back silently.

## Tests and docs

- `TestExportAnnouncesOnlyANewBacklog`: the first export into a bare sibling
  announces the backlog and the git hint. The second into the same place is the
  plain `copied → sibling`.
- `TestExportBrowseFlow` now also checks the note on the browse road.
- README, *Exporting a prompt to another project*: the note, and why export
  doesn't ask.
- `go test ./...` passes. Not run live in cats.
- `gofmt -l` flags `listhover.go`, which this session didn't touch. Left as is.

## Next

Closed: N-080. Declined: None. Raised: None. Deferred: N-071 (moved to Roadmap
by the user before this session; committed with this doc). Promoted: None.
Moved: None. Updated: None. Full list: `ai_docs/todo/next-list.md`.
