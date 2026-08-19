# Session: a priority system, and a View panel to sort by it

Session ID: `9fa5dccf-c266-4a7f-932d-36b837d22160`
Date: 2026-08-19

The ask, in one line:

> "Let's come up with a priority system that will show up in Cats Mux as -
> critical: red, standard: yellow, low: white"

then, after the shape was put up:

> "In the TUI rows give the priority colors, but don't auto-order. Give a
> checkbox to order by priority."
> "The toggle can live in a 'view' popup that also includes a toggle for
> 'frozen' todos. Yes these should persist across restarts and use the same pale
> yellow for standard as we use for the paw icon in cats today."

New: `priority.go`, `priority_test.go`, `prioview_test.go`.
Touched: `store.go`, `styles.go`, `fuzzylist.go`, `ui.go`, `settings.go`,
`cli.go`, `complete.go`, `export.go`, `model_test.go`, `ui_test.go`,
`export_test.go`, `README.md`.

## "Cats Mux" turned out to be the wrong end of the telescope

The first thing exploration settled, and it changed the question. cats-todo
never hands the mux individual todos: it advertises an open *count* by putting
it in its pane title (`model.paneTitle`), and catway scrapes that with
`/^todo(?:: .+?)? \((\d+)\)$/` to draw one paw badge per workspace row. So
"show up in Cats Mux" could have meant the sidebar badge — a two-repo change
plus a wider channel than a count — or the manager's rows as seen inside a cats
pane. It was the rows. **The pane-title wire format is untouched**, on purpose:
a critical-count suffix would break that regex.

The one real cross-repo tie is the colour, below.

## Colours, and the amber that was already spoken for

The literal ask was "standard: yellow", and the obvious yellow — `colWarn
#e0b64e` — is unavailable. `styles.go` reserves amber for `matchStyle`, which
paints fuzzy hits *inside the row names* the dot would sit a few columns from,
and standard is the default, so its dot lands on more rows than any other mark
on screen. A second amber on every one of those lines costs the highlight the
one thing it exists to do.

Put to the user, who answered it better than either option on the table: use
the yellow **cats already paints the todo paw in**. That is the `todo` key of
cats' theme table (`internal/theme/builtin.go`, `--todo` in the served page),
`#f0dfa0`, and it is now `colTodo` here. The dot and the workspace badge that
counts the row are the same colour by construction rather than by two people
picking a yellow — which is exactly the argument `styles.go`'s header already
makes for mirroring cats' palette.

Final set: critical `colErr`, standard `colTodo`, low `colBrown` `#b5835a`.

Low started as plain `colFg` — "white", as asked — and moved to a brown on
review. The brown is the better answer for a reason worth recording: the three
dots are one cell each, and at that size **lightness separates them faster than
hue does**. `colFg` at 85% sat almost level with `colTodo` at 78%, so standard
and low were told apart only by warmth. The brown at 53% turns the three into a
ramp — standard 78, critical 67, low 53 — over the 40% the closed rows recede
to. It is warm and saturated enough (28° 38%) not to be mistaken for
`colFaint`'s near-neutral grey one step below, and clears 4.7:1 on the page.

It is the one hue here that is neither cats' nor mixed from something that is;
the warm end of the palette had nothing dark left in it, and this level needed
to be dark.

## Where the dot goes — the one departure from the obvious

Two slots existed and both were wrong.

The **badge** column is documented as holding one mutually-exclusive *state*
(`○` open, `✓` done, `❄` frozen, `◷` scheduled), and priority is orthogonal to
all four — a critical prompt can also be scheduled. Tinting the badge would make
hue mean priority on some rows and nothing on others, and would lose the colour
on precisely the row where it matters most.

**descMarks** (`⏰ ⚙ 📎N`) follow the name, so they start at a column that moves
with every title's length. A triage colour that cannot be scanned straight down
the pane is not doing the job it was added for.

So: a **new fixed column between the cursor and the badge**, `●` U+25CF. The
width is not a gamble — it is East Asian Ambiguous, the same class as the `○`
this list has always drawn as its open badge, so it costs exactly one cell
wherever that one does (pinned by `TestPriorityGlyphIsOneCell`). Filled-and-
coloured against hollow-and-grey is what keeps the two adjacent circles from
reading as one thing.

Considered and rejected: retiring `○` to avoid the twin circles (it is a visible
change to a row people already read, for a problem that fill and hue already
solve), and a `▌` stripe instead of a dot (handsome, but consecutive rows draw
an unbroken rail of pale yellow, which is far more ink — and the ask said dot).

**Closed rows recede.** A done or frozen row's dot drops to `colFaint`, the same
argument `doneStyle` makes: finished work must not compete with open work for
the attention priority exists to direct. The glyph stays so the column stays
straight. A *scheduled* row keeps its hue — it is still work outstanding.

Cost, stated plainly: two extra columns before every title.

## Sorting is a lens, never a rewrite

`rebuildList` sorts a **copy** of each group, stably, and only when the toggle
is on. Three reasons it cannot touch the array: the array *is* the order the
user dragged the backlog into and the only record of it, so turning the lens off
would have nothing to restore; multiple manager panes share the global backlog,
and one pane's view preference must not rewrite a file another is reading; and
the toggle lives in `settings.json`, so a preference editing data would be a
category error.

It sorts inside one backlog and one group and never across them — a finished
critical prompt does not climb above open work.

### The correctness trap, and a gap found on the way

Under the lens the rows are in an order the array does not hold, so reordering
has to be refused. `canReorder` already refuses drags under a filter with
reasoning that generalises exactly, so the lens joins that predicate.

But `moveSelected` (`ctrl+↑/↓`) **never called `canReorder` at all** — only the
drag path did. That is correct today, because `store.move` walks the array and
stays coherent under a filter, so the chord has always worked there. Widening
`canReorder` and using it in both places would have silently taken that away.

Hence `orderIsBacklogOrder()` split out from `canReorder()`: the drag needs the
whole predicate, `ctrl+↑/↓` needs only the lens half. `reorderRefusal()` names
which of the two orders is in the way, because on screen they look identical and
what to do about them differs. `TestReorderRefusedUnderPriorityOrder` pins both
halves, including the filter case, which had no test before.

## The View panel, and splitting a fold that was one switch

`stageView` was already taken — it means "read this prompt's text" — so the new
stage is `stageViewOpts`, titled **View**. Modelled on the Session panel, which
is the house pattern for a label · value table.

The fiddly part was `hideClosed`, which folded done *and* frozen together and
whose own comment defended that: the question it answers is "show me what is
left to do". Giving frozen its own switch means splitting it into `hideDone` and
`showFrozen`, and the compatibility rule chosen is that **`ctrl+d` still moves
both together** — the one key that has always meant "what is left to do" still
means exactly that, and the panel is the fine control rather than a replacement.
Mixed state resolves toward *showing*, because revealing is the recoverable
direction.

`hiddenClosedCount` now counts what is actually hidden rather than everything
closed, so the header's `· N hidden` stays honest when only one of the two is
folded. `hiddenNote` lost its guard and just reports that count.

Persistence is uneven on purpose: **priority order and show-frozen are saved**
(the two the ask named), while the `ctrl+d` fold stays per-session, as it always
has been. Quietly giving it a memory it has never had would have been scope
creep. `ctrl+d` does write `showFrozen`, since it changes it.

## Storage

`Todo.Priority` is a plain string tagged `json:"priority,omitempty"`, with
**standard as the empty string** — the `ctxNone` precedent. "Never said" and
"explicitly ordinary" are deliberately one value, so a hand-edited backlog has
one fewer state to
guess about, most todos write no key at all, and every todo that existed before
this field decodes as standard with no migration. Verified by hand as well as by
test: `cats-todo add` of an unranked prompt writes no `priority` key.

`setPriority` is its own setter because `store.update` is deliberately text-only
— an edit to a prompt's wording must not reset a triage decision the editing
path knows nothing about (`TestUpdateDoesNotClearPriority`).

`exportTodo`'s whole-struct copy carries it for free, which is right: priority
is a fact about the prompt and stays true in any backlog, unlike the schedule,
which names a pane in the project being left. Pinned rather than left implicit.

## Chords

Priority is set **only in the editor**, as a row in the ⚙ panel (`ctrl+r`), and
the list does not set it at all. `ctrl+l` opens the View panel — the same chord
that opens a panel in the form, which is a better justification than any free
initial of "view".

It got there by two reversals worth recording, because the reasoning is the
useful part.

It shipped on `ctrl+r` on the list, then moved to `ctrl+p` on request. Then the
question became whether to set it in the editor instead, using `ctrl+p` there —
and **`ctrl+p` is not free in the form**: it is the textarea's caret-up. Verified
rather than assumed (caret line 2 → 1 on `ctrl+p`, back to 2 on `ctrl+n`), which
matters because the form is a screen you type into, where that alias is worth far
more than it is on a list. Surveying what was left — the textarea owns
`a e f b n p w k u h d v t m j`, the form already claims `c o i r l g s` — leaves
only `ctrl+q` (XON), `ctrl+z` (suspend), `ctrl+y` (ruled out in an earlier
session) and `ctrl+x` (delete in the list, remove in the attachment editor). The
form has effectively **no chord left to spend**.

Which made the ⚙ panel the answer: it is already the form's table of per-prompt
enums, `cycleValue` and the row machinery already exist, and a row costs no chord
at all. `ctrl+p` went back to being up-navigation on the list, untouched
everywhere.

The cost, accepted knowingly: triaging several prompts is now open → set → save
each time, where one chord on the list used to do it. Priority is still visible
on every row; it is just not settable from there.

Priority leads the panel and is the only row in it that is not a launch flag —
everything below describes the session that will read the prompt, and this
describes the prompt. That shifted every row index by one and broke
`TestSessionPanelCycles` and `TestSessionSavedWithTodo`, which walk the panel
from row 0; both were updated rather than putting the row last, since trailing
"how it ends" would have been the worse read.

`formPriority` is its own model field rather than a member of `formSession`,
because it is not a session option and is stored on the Todo rather than in its
Session record. Held by value, so an abandoned form leaves the stored level
exactly as it was, and written by `setPriority` on save — its own write, for the
same reason the images and the session record each need one.

The form's ⚙ line leads with the level whenever it is not standard. The editor is
the one screen without the dot on it, and without that the level a prompt was
given would be invisible on the very screen that sets it.

Plain letters were never on the table: anything unmatched on the list falls
through to the filter box.

The footer says **"ctrl+l view options"**, spelled out and never shortened to
"view", because `ctrl+v` is two segments to the left and means read this
prompt's text. Two chords sharing a word on one line is how a reader learns the
wrong one.

## Verification

`gofmt`, `go build`, `go vet`, `go test ./...` clean. New tests were run
verbosely to confirm they execute rather than silently skip.

The colours were checked the way this repo checks colours — dumping `viewList()`
with ANSI intact through a throwaway test and reading the escapes, deleted
afterwards. All four resolved as intended (`229;115;115`, `240;223;160`,
`181;131;90`, `95;111;100`), and `colSel` runs behind the dot on the
highlighted row, which is the `onRow` invariant working. Also dumped with a
filter active, to confirm the standard dot and `matchStyle` stay separable.

The CLI was exercised end to end against a real backlog: the three levels, a
folded spelling, a rejected one, and the compat promise (an unranked prompt
writes no key). Completion was checked with `__complete`.

**Not yet judged**: how `#f0dfa0` reads against the ✎ Edit chip's `colStraw`
`#eee5c9` two lines above the rows — they share a hue (45°) and separate only by
saturation. That is the one thing captured output cannot settle, and the trigger
for falling back to `▌` if the twin circles read poorly in a real pane.

## Standing notes

- **A test-isolation hazard was fixed on the way**: `newModelInTemp` did not
  point the config directory anywhere, so `newModel`'s `loadSettings()` read the
  developer's real `settings.json`. That was harmless while the only preference
  was spellcheck; it is not harmless now that two preferences decide which rows
  are drawn at all. It now uses a temp config dir, so the suite answers to the
  documented defaults rather than to whatever was last toggled.
- `styles.go` gained a fourth entry (`colTodo`) that must stay in sync with
  cats' `todo` key. The file already says to keep the table in sync; this is the
  first constant here whose *name* is the source key rather than the role, which
  is what makes the sync greppable.
- The wire-contract copies in `internal/{app,ctlproto,integration}` are
  unchanged. Nothing here touches the control protocol.
- The narrow-pane footer branch renders two lines longer than the pane and wraps.
  That is pre-existing, not introduced here, but this session added two segments
  to it and it is worth a look.
- Version not bumped.
