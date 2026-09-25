# Scheduled pane drops check the agent is still there (N-020)

Session: `6aa4d18f-056a-4547-a527-3320af37cbc1`
Date: 2026-09-25

## Ask

1. The Next List item N-020 was pasted: gate `/clear` on a detected agent, as
   `/model` and `/effort` are gated. Its premise had already narrowed. The
   drop picker lists only `isDropAgent` panes, so the remaining road was a
   **scheduled** existing-pane drop. `performScheduledDrop` checked only that
   the pane still existed, so if its agent had exited before the schedule
   fired, `/clear` and the prompt were typed at a shell and, since a fire
   always runs, executed there. That was fixed.
2. `/sw`.

## The fix

Two parts. The first closes the road, and the second is what the item
literally asked for.

- **`scheduledPaneAgent(panes, id)`** (`drop.go`) is the fire path's check,
  kept pure so it can be tested without a socket. It returns the live agent
  label, or a refusal:
  - "the scheduled pane is gone — send manually" (this refusal already existed);
  - "the scheduled pane's agent has exited — send manually" (new): the pane
    fails `isDropAgent`, the rule the picker used to offer it. A shell left
    behind fails it, and so does an editor pane, on an old or a new cats.

  `performScheduledDropAt` sets `act.target.agent` to the live label before
  `performDropAt`, so `paneSetupCommands` gates `/model` and `/effort` on what
  runs at fire time rather than on the label saved hours ago. A scheduled
  batch fires through the same function (`batchrun.go`), and its `Schedule`
  never carried an agent, so it benefits too.
  - `paneExists` stays and now sits on a new `findPane`, which both share.
- **`paneSetupCommands`** (`session.go`) returns nothing when the agent label
  is blank, `/clear` included. The comment has a table of which command goes
  to which agent. The picker keeps this road closed, so this is a backstop
  for a caller that builds a target by hand.

Design choice: **refuse rather than downgrade.** A fire that finds a shell
could have skipped the setup and typed only the prompt, but the prompt is a
command line at a shell too. Missed is the outcome that already exists for a
vanished pane, and sending by hand goes back through the picker.

## Tests

- `TestScheduledPaneAgent` (`schedule_test.go`) covers:
  - a claude pane;
  - another agent under its live label;
  - a pane that is gone;
  - a shell left behind;
  - an editor with `plugin_type`, and one without (an older cats).

  It also checks that every refusal says "send manually".
- `TestPaneSetupCommands`: the case "an undetected agent gets only /clear"
  now expects nothing, and a blank label counts as undetected too.

`go test ./...` passes. `gofmt -l` names only `promptcode.go` and `ui.go`,
which this session did not touch.

**Docs:** in the README, the schedule paragraph, the batch *Read fresh*
bullet and the existing-pane setup paragraph now mention the agent check.

Not released. This change is added to N-059, and N-056's live-test checklist
has a case for it.

## Next

Closed: N-020. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: N-056, N-059. Full list: `ai_docs/todo/next-list.md`.
