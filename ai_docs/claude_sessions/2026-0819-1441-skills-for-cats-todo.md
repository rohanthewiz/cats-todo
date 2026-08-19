# Session: skills for cats-todo

Session ID: `bf4265d8-c8cc-4c65-8672-52d8ef729347`
Date: 2026-08-19

The ask, in one line:

> "create a skill for cats-todo"

then:

> "Yes also add a project-local skill"
> "Fix the .gitignore so .claude/skills does travel"
> "commit and push"

New: `.claude/skills/cats-todo-dev/SKILL.md` (this repo),
`~/.claude/skills/cats-todo-prompt-backlog/SKILL.md` (the user's synced skills dir,
not this repo).
Touched: `.gitignore`.

## Two skills, because "a skill for cats-todo" has two readings

The other skills in `~/.claude/skills/` — btypedb, rweb, serr, element, logger — are
all *usage* guides for rohanthewiz tools, written so an agent in any project can reach
for the tool correctly. Read against that shelf, "a skill for cats-todo" most plainly
means the same thing: teach an agent to capture prompts into a backlog and to know
what the manager does. That is the first skill, and it went where its siblings live.

The second reading — "help me work *on* cats-todo" — is a different document with a
different audience (an agent standing in this checkout), so it became a separate,
project-local skill rather than a second half bolted onto the first. Offered as a
follow-up; taken.

## `cats-todo-prompt-backlog` (global): the tool as a user sees it

Built from the README, `cli.go`, `store.go`, `session.go`, `priority.go`, and the
footer strings in `ui.go`, and checked against the live `.cats-todo/todos.json` here.
What it carries:

- the CLI quick reference and the backlog-root rule (nearest `.cats-todo/` → git
  root → cwd; `add` refuses outside a project and points at `-g`);
- every `add` flag in one table, with the folded spellings the normalizers accept
  (`urgent`→critical, `accept-edits`→`acceptEdits`, the three `--sess-load` forms,
  `--sess-load`/`--sess-use` exclusivity, `--priority` long-only);
- the three delivery mechanisms for session options, so an agent writing a prompt
  knows which parts only apply to a new `claude` session;
- the `todos.json` shape with an annotated example — `done`/`frozen` exclusivity,
  standard priority stored as nothing, image paths relative to the file, the
  `schedule` object — plus two `jq` recipes for reading it and a rule to prefer
  `cats-todo add` for writes (ids, image copies, validation);
- the manager's key map, the drop picker's rows, install lines, and recipes for the
  moment an agent most needs this: capturing out-of-scope findings at the end of a
  session with enough context to act on cold.

The description line is the trigger surface; it names the verbs a user actually says
("add a todo", "jot a prompt for later", "capture follow-up work", "list the backlog",
"init a project backlog").

## `cats-todo-dev` (project): the contracts that are easy to break

The dev skill is mostly a list of things that are true about this codebase but not
visible from any one file:

- Bubble Tea **v2** under the `charm.land/*` import paths — the first thing a fresh
  agent gets wrong;
- a file map with what each `.go` file owns, taken from the file-header comments;
- the eight contracts: `todos.json` omitempty/zero-default compat; the **two-place
  version bump** (`main.go` const and `cats-plugin.toml`) behind every
  `chore(release)`; lockstep with cats for `internal/app`, `internal/ctlproto`,
  `internal/integration`, the `styles.go` palette, and `claudeReadyProbes` (and the
  silent 12s-per-drop cost of a stale probe); refuse-in-words; mouse reporting only
  where there is something to click; button rows that shrink and never drop a chip;
  text-only wire; shared CLI/TUI normalizers;
- key-chord ownership per screen, verified against `ui.go` — including why the
  session panel is `ctrl+r` (`ctrl+e` is the caret's end-of-line), that `ctrl+g`
  toggles scope only in add mode with both backlogs available, and that **Send** is
  click-only by design;
- how tests drive the model (`tea.KeyPressMsg` through the stage `update*` funcs;
  `newTestModel`, `newModelInTemp`, `tempStore`, `pressList`, `enterKey`);
- the README-as-spec habit, `ai_docs/plans` and `ai_docs/claude_sessions`, and the
  `feat(form|list|drop|ui)`/`docs(session)`/`chore(release)` commit vocabulary.

One correction surfaced while writing it: the palette comment in `styles.go` and the
README both say cats' colors live in `internal/config`'s `defaultColors`, but cats
moved them to `internal/theme/builtin.go` when named themes arrived. The skill says
where they are now; the stale pointers in the comment and README are left for a
later pass rather than touched in a docs-only commit.

## `.claude/` was gitignored, which would have kept the dev skill at home

The repo ignored `.claude/` wholesale (for `settings.local.json`), so the project
skill would have existed only in this checkout. The fix is the one git forces:
`.claude/*` plus `!.claude/skills/`. A negation cannot reach under an ignored
*directory*, only under an ignored *glob* — with `.claude/` the `!` line is inert,
with `.claude/*` it works. Verified with `git check-ignore -v`: `settings.local.json`
still matches line 2, `SKILL.md` matches nothing. The `.gitignore` comment says all
of this so the next person does not "simplify" it back.

## Commits

- `ff7ccf5` docs(skills): a project skill for working on cats-todo, and un-ignore
  `.claude/skills` so it travels — pushed to `main`.

## Left open

- The `internal/config` → `internal/theme` pointer in `styles.go` and the README.
- `bin/cats-todo` in this checkout is a stale 0.14.0 build; `go build -o bin/cats-todo .`
  when it next matters.
- The global skill lives in the rosync-managed skills dir, so it syncs on rosync's
  schedule, not this repo's.
