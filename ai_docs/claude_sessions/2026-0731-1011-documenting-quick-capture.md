# Session: documenting quick capture — the `add` section the README never had

Session ID: `91d29006-48b2-4a2e-87f1-756cda7f376f`
Date: 2026-07-31
Repo: `cats-todo` (docs only — no Go code changed)

## The ask

"I need a command line method of adding to a project's todo."

The answer was that it already exists — `cats-todo add`, in `cli.go`. So the
session turned into the question behind the question: if the feature is built
and the user is asking for it anyway, what does the README fail to say?

## What was already documented, and what wasn't

`add` had no section of its own. It was documented in fragments across three
places:

- the synopsis block up top (`-g`, `-t`, `-i`, piped stdin, all as bare examples)
- the scoping paragraph at line 20, which does explain the project-root rule
- the **Images** section, which covers `-i` properly because images earned a
  section and `add` never did

Three things were nowhere:

1. **`-t` was never explained.** It appeared exactly once, in
   `git log -p | cats-todo add -g -t huh`, where "huh" reads as placeholder
   noise rather than a title. Nothing said the title defaults to the prompt's
   first line, trimmed to 60 characters.
2. **The no-project refusal.** Run `add` where no project owns the directory and
   it stops rather than inventing a backlog in the cwd. That is a deliberate
   choice with a comment in `cli.go` explaining it — an unavailable store saves
   to nothing and still reports success, so without the guard the prompt would
   be swallowed and the command would print "added" anyway. Worth documenting as
   behaviour, not just leaving as a surprise.
3. **How to get the binary on your PATH.** Installing covered
   `catctl plugin install` and `go build -o bin/cats-todo .` — and that build
   leaves the binary in `bin/`, not runnable as `cats-todo` from some *other*
   project, which is the entire point of `add`. `go install .` was missing.

That third one was the actual gap behind the ask. `bin/cats-todo` existed in the
checkout; `command -v cats-todo` found nothing.

## Changes

**New `## Quick capture from a shell` section** (between the manager prose and
**Images**): four common invocations, then the args-else-piped-stdin rule and
why an interactive stdin is never read (so `add` is safe to bind to a key or put
in a script), what `-t` does and its first-line fallback, and the no-project
refusal quoted as it actually prints.

**`go install .`** added beside `go build -o bin/cats-todo .` under
**Installing**, with a line on why that is the one that matters for `add`.

**`-t huh` → `-t "review this diff"`** in the synopsis, matching the stdin
example in the new section so the two read as one idea. The synopsis comment
was shortened to "capture piped stdin, global backlog" because the longer title
pushed the line past the aligned comment column.

## One detail worth keeping

The quoted error message was checked against the real code path rather than
copied from the string literal in `cli.go`. `errExit` prepends `cats-todo:`
(`util.go:13`), so the literal alone would have been wrong in the README by
exactly one prefix — the kind of thing nobody notices until they grep their
terminal for the message and find nothing.

## Notes

- No code changed; `cli.go` was read, not edited.
- The scoping paragraph at line 20 was left alone — it already states the
  nearest-`.cats-todo`-else-repo-root-else-cwd rule for both the manager and
  `add`, and the new section refers back to it rather than restating it.
