# Session: the drop picker tags blank agents [new] / [cleared]

Session ID: a2a92ca2-573a-49cb-adfb-2a1d68185844
Date: 2026-10-16
Not released. Under the release rules this is a patch: it changes the words
on an existing picker row and adds no capability.

## The ask

From the user's backlog, with a screenshot of the picker showing
`claude · ced (this project) [idle] ~/projs/go/ced`:

> In sibling project cats-todo when I am dropping onto an already cleared
> agent, show "cleared" instead of idle.

Follow-up in the same session: show `[new]` for fresh sessions too.

## Why it needed cats

cats already knows when an agent's conversation is untouched. It reads the
agent's history for it (`cmd/catway/agentblank.go`) and draws the sidebar's
blank dot from it ("new" for a fresh launch, "cleared" for a `/clear`). But
the word only reached the browser (`AgentItem.Blank`, `PaneAgent.Blank`), not
`pane.list`, which is the only thing cats-todo reads. `AgentSessionOrigin`
("clear") was already on the wire but is no substitute: it outlasts the first
prompt, so a pane cleared and then given work would still read as cleared.

## What changed

**cats `ab1d06a`** (pushed to main):

- `wire/vocab.go`: `PaneMeta.AgentBlank` (`agent_blank`, omitempty), "new" |
  "cleared" | omitted. The doc comment says it is the sidebar's word, goes
  when the agent works, and leaves `AgentState` "idle".
- `cmd/catway/catway.go` `orch.PaneMeta`: `meta.AgentBlank = rt.blank` inside
  the agent branch, beside the model and session origin.
- `cmd/catway/agentblank_test.go` `TestBlankFollowsTheConversation`: asserts
  `PaneMeta` reads "cleared" while the rollup does, and "" once working.
- Full `go test -tags ghostty ./cmd/catway/` and `./wire ./internal/app` pass.

**cats-todo `42e89cc`, then `af887d2`** (pushed to main):

- `go.mod`: cats pinned to `ab1d06a`
  (`v0.5.1-0.20261016212909-ab1d06a9726b`). The jump is from the Sep 22 pin,
  so it brings a month of other cats changes; build and
  `go test -race ./...` passed with it.
- `context.go` `paneStateWord(p)`: the row's state tag. It is cats' blank word
  when the agent is idle and the word is set, else `p.AgentState`. Gated on
  "idle" so a stale word cannot mask working/blocked. Display only:
  `p.AgentState` is what the batch loop waits on, and is untouched.
- `ui.go` `buildTargetsFor`: the `[state]` prefix uses `paneStateWord`.
- `context_test.go` `TestPaneStateWord`: idle+cleared, idle+new, idle alone,
  working+cleared, blocked, empty.

`42e89cc` mapped only "cleared"; `af887d2` passes the word through, so
"new" shows too.

```
cats agentblank.go ─ rt.blank ─┬─▶ AgentItem.Blank ─▶ sidebar blank dot
                               └─▶ PaneMeta.AgentBlank ─▶ pane.list
                                                            │
                         cats-todo paneStateWord ◀──────────┘
                           idle + "new"     → [new]
                           idle + "cleared" → [cleared]
                           otherwise        → [<AgentState>]
```

## Notes

- Not visible until the Mac app runs a cats build with `ab1d06a`. An older
  catway omits the field and every row stays `[idle]`, which is harmless.
- `listhover.go` is flagged by `gofmt -l`. That predates this session and was
  left alone.
- `42e89cc`'s subject doesn't follow the repo's `feat(drop): …` style. It was
  already pushed, so it was left.

## Next

Closed: None. Declined: None. Raised: N-086.
Deferred: None. Promoted: None. Moved: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
