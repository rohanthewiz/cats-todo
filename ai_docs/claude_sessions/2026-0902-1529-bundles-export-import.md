# Session: bundles — export and import, to disk, mail and the LAN — v0.25.0

Session ID: `f189352e-30cb-4ea4-9168-fbe2522e8974`
Date: 2026-09-02

The ask, in full:

> "We already have some kind of export mechanism, but it is not mature. I would
> like to be able to:
> - export selected or all todos
> - export targets
>     - email attachment or in-body
>     - to disk
>     - machine on local network running Cats
>
> - import targets
>     - disk
>     - machine on local net running Cats"

and, mid-build:

> "Note that on this machine (work laptop) some attachments are not allowed.
> That's okay, skip if necessary"

Four decisions were settled with the user before any code: **mailto hand-off**
rather than SMTP; a **peer daemon plus a hand-rolled UDP beacon** rather than
mDNS; **JSON, upgraded to a zip when there are attachments**; and **per-row
marks** plus a scope widening at export time.

New: `bundle.go`, `marks.go`, `email.go`, `import.go`, `peer.go`, `beacon.go`,
`transfercli.go` and their tests. Touched: `export.go`, `ui.go`, `fuzzylist.go`,
`filepick.go`, `listmenu.go`, `settings.go`, `styles.go`, `main.go`,
`complete.go`, `README.md`, and the two version places.

## What the old export was, and why it could not stretch

`export.go` moved **one** highlighted prompt into **another project's backlog on
this machine**, by opening that backlog's `todos.json` and writing into it. Every
one of those three words is load-bearing: one prompt, a backlog, this machine.
The ask breaks all three at once, and the thing that unlocks it is noticing that
the destinations divide cleanly:

- another backlog this process can open → keep writing prompts, as today;
- anywhere else → the prompts have to become **a file that stands on its own**.

So the picker grew a second block under an "Off this machine" heading, and
everything below that heading speaks bundles. The old path is untouched — its
code, its tests and its README section all still say what they said.

## The bundle is a backlog with an envelope

`Bundle{schema, created, from, source, todos}` where `todos` are `Todo` values
marshalled by exactly the rules `todos.json` uses. That is the whole
compatibility story, and it is free: the `omitempty` discipline the backlog
format already keeps (contract #1) carries into the wire format, so a bundle
from a newer cats-todo loses only fields an older one never knew about. A schema
from the future is *read*, not refused — refusing it would throw away the
property the additive rule exists to give.

Attachment paths inside a bundle are the same strings the backlog stores
(`images/<id>/<file>`), so the zip's layout **is** the manifest's own view of
itself and nothing is rewritten at either end.

Three things the format refuses to carry, each for a stated reason:

- **schedules** — a schedule names a pane id and a cwd of the machine being
  left. Stripped on the way out *and* again on the way in, because a bundle is a
  file anyone can hand-edit and a hand-written one must not be able to arm a
  timer on the receiving machine.
- **aliased session records** — `Session` is a pointer on `Todo`; a bundle that
  aliased the live one would let a later edit mutate a message already sent.
- **paths in attachment names** — `safeAttachmentName` reduces every name to a
  bare file name, which is where zip-slip stops. Everything else about
  attachments (which extensions, what size, what collides, how a half-finished
  copy rolls back) is left to `attachImages`, deliberately: the import stages
  bytes into a temp dir and hands them over as ordinary source paths, so there
  is exactly one piece of code deciding where an attachment lives.

## Marks are keyed by prompt, never by row

The selection was the one place a shortcut would have been genuinely dangerous.
A `map[int]bool` over row indices is the obvious implementation and it is wrong:
a row index means a different prompt after a move, a delete, a fold or a change
of filter, and a set that quietly re-pointed itself at other prompts is the worst
possible bug in a feature whose whole job is sending work to other machines. So
the set is `map[todoRef]bool` — scope plus id — and the cost of that choice is
`marks.go`: pruning against the backlogs on every rebuild, and ordering against
the list rather than simply indexing. `TestMarksFollowThePromptThroughAMove` is
the test that would have caught the shortcut.

Two consequences worth keeping: a prompt hidden by the filter or the fold stays
in the set (those are ways of *looking*, not ways of unselecting), and the set is
cleared once it has been spent, since a set left ticked after an export is a set
the next `ctrl+o` would send again.

The ✓ column is a whole-list decision, like `trimAnnotColumns`': it exists
exactly while something is ticked, so a backlog nobody selects in looks exactly
as it always did.

## mailto cannot carry an attachment, and the UI says so

SMTP means a server, a username and a password — three things to configure, one
of them a secret this tool would then be storing — for a feature whose whole job
is "get this prompt to a person". The machine already has something trusted with
all three. So `email.go` is a hand-off: a `mailto:` URL opened the way a link is.

The honest cost is that RFC 6068 has no attachment field and no client accepts
one. Rather than pretend, there are two rows: **in the message body** (markdown,
one click) and **with a bundle file** (the bundle written, revealed in the file
manager, and the composer opened with a line naming it to drag in). The status
line says exactly that. The user's note about attachments being blocked on the
work laptop lands on the right side of this: the body path needs nothing.

A body over ~8 KB is refused in words with the offer to write a bundle instead —
a prompt that arrives with its last paragraph silently missing is a failure the
sender cannot see.

## The LAN service, and the one place the house style lost

cats' control socket is a unix socket, so the machine across the room needed a
service of its own. `cats-todo serve` is three routes: hello, GET a backlog as a
bundle, POST a bundle into the inbox. Three refusals hold it up — a bearer token
is required and a `serve` with no token will not start; requests from outside
this machine's private ranges are refused unless `--allow-remote`; and nothing
that arrives is ever *run*, only written as rows.

**rweb would have been the house choice and it lost on a fact**: its `Context`
exposes no client address, and rule 2 needs one. Three routes on stdlib
`net/http` costs the plugin no dependency at all, so that is what it is — noted
here because the next person to touch this file will wonder.

Discovery is `beacon.go`, and the shape is a **question, not an announcement**: a
manager opening a picker asks a multicast group who is there and every server
answers unicast. A server that announced on a timer would leave a picker opened
between two announcements looking empty, and one announcing fast enough to fix
that would chatter all day for a screen nobody has open. Asking is both quicker
and quieter. Nothing secret is in a datagram; the token is the HTTP layer's
business.

One note for future debugging: the beacon test skipped on its first run and has
passed every run since — almost certainly the macOS firewall prompt on the first
bind. The test skips rather than fails where multicast is unavailable, which is
right for a CI box, so a real regression there will look like a skip. If
discovery ever seems broken, run `TestBeaconAnswersAQuestion` and read whether it
passed or skipped.

## An import shows its arithmetic first

Import is the only operation here that takes someone else's work and mixes it
into the user's own list, so it stops at a confirm that states the sum: how many
prompts, into which backlog (`tab` switches, re-counting as it goes), and how
many are already here and will be skipped. Fresh ids by default — an import is
new work in *this* backlog — and a prompt whose attachment cannot be brought
across still lands, bare and counted, because the text is the part with the
value.

## Verified end to end, not just in tests

Two temp projects and a real server: `export --out` → `import` round-trips
prompts with their priority; a second import adds nothing and says "2 were
already there"; a zip with an attachment arrives with **byte-identical** image
data; `/v1/hello` without a token is a 401; a pull dedupes; and a push's status
line is the receiver's own sentence rather than an optimistic restatement.

One thing that went wrong and is worth remembering: an existing test walked to
"the last row" of the export picker, which used to be Browse and is now an email
row — so the suite wrote two bundles into the developer's real `~/Downloads` and
tried to open a mail client. Fixed twice over: the tests now find rows by kind,
and `email_test.go` has an `init()` that stubs `openURL`, `revealFile` and
`bundleBrowseRoot` for the whole package, so a test has to *opt in* to touching
the machine instead of having to remember to opt out.

## Key chords added

- List: `ctrl+space` (alias `ctrl+b`) select a row · `ctrl+r` import.
- Export picker: `ctrl+a` widen to the whole backlog, and back.
- Import confirm: `tab` the other backlog.

`ctrl+r` is also "run" in the *drop* picker; different stage, no collision.
