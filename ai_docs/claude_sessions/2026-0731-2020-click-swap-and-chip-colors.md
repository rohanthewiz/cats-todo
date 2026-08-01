# Session: the double-click swaps to edit, and the list gets its colors

(The chips, then the row cursor. The filename predates the cursor work.)

Session ID: `0a9f006d-57cf-44c0-84c1-054eb6d37838`
Date: 2026-07-31
Repo: `cats-todo`

## The ask

Seven asks, in order:

1. "Could this click handling work? A single click selects and goes into edit
   mode for the todo, while dbl clicking sends the todo to the agent?"
2. "Is it possible to use some color on the chips. Like blue for edit, green for
   send, red for delete?"
3. "Let's give the blue color to Add and make edit a light yellow. What do you
   think?"
4. "Change the selected row indicator from the vertical green bar to a thick
   muted magenta prompt arrow."
5. "Lighten the yellow for Edit by 12% and take the hue a tad towards red."
6. "Refocussing the search doesn't give me any indication it is back in focus."
7. "Can we do some kind of a subtle whole row highlight on selected, even for a
   completed todo?"

The first was a design question, answered before anything was changed. The
middle ones were color work, where the interesting parts were *where* the color
goes and which colors were already spoken for. The last two started as polish
and turned out to be bugs — both of the same shape, which is the thing worth
carrying out of this session:

> **A widget that never changes state cannot signal a state change.** The query
> box was focused once at construction and never blurred. A completed row let
> `strike` win over `selected` unconditionally. In both cases the request read
> as "add an indicator", and in both cases the indicator could not have worked,
> because the thing it would indicate was never being tracked.

## The click proposal, and why it was turned down

The proposal was: single click → select **and** open the edit form;
double click → send to the agent. Three problems, in ascending order of
seriousness.

**It doesn't detect.** `beginEdit()` sets `m.stage = stageForm`, and
`updateMouse` (ui.go:427) only handles `stageList` and `stageTarget`. Click #2
of the double would arrive after the list is gone and be dropped on the floor,
so the double-click never fires. Making it work means *deferring* the single
click: start a `tea.Tick(doubleClickWindow)` carrying a click-generation
counter, open the form only when the tick returns unsuperseded. Every ordinary
single click then stalls a visible 500ms before anything happens; shrink the
window to hide the lag and the double gets hard to land.

**It costs the action bar its subject.** `➜ Send` and `✖ Delete` both carry
`needsSel: true` — they act on the highlight. If clicking a row opens a form,
there is no way to move the highlight with the pointer without leaving the list,
so those two chips become mouse-unreachable outright. This is exactly what the
existing comment at ui.go:445-450 was protecting.

**It inverts the risk.** Today the cheap gesture is the harmless one and
escalation runs select → picker. Under the proposal the *accidental* gesture is
the one that opens a modal, and a double-click landing slightly too slow puts
click #2 into the form's textarea.

## What was done instead

Keep single = select; make double = **edit** rather than send. One line in
`clickRow`: `m.beginDrop()` → `m.beginEdit()`.

The reasoning, which is now in the function's doc comment:

- Double-click-to-open is what a pointer means everywhere else.
- It is the gesture's safest reading. Opening a form is undone by `esc`; the one
  action that reaches out of the program — handing a prompt to a live agent —
  moves off a gesture the hand can make by accident.
- Sending with the pointer becomes two clicks in two places (row, then
  `➜ Send`). That deliberateness is the point, not a regression.

### Follow-on prose

- Footer: `enter edit · alt+enter / dbl-click drop` →
  `enter / dbl-click edit · alt+enter drop`.
- README's pointer paragraph rewritten (README.md:70) — the double-click now
  edits, and Send is named as the route to the picker.

### Tests

- "a second click hands the prompt to an agent" → "a second click opens the
  prompt for editing", asserting `stageForm`/`formEdit` and `editID`.
- **New:** "no gesture on this screen reaches an agent" — 2, 3, and 4 clicks on
  a row, none of which may reach `stageTarget` or set `dropping`. This is the
  property the swap actually buys, so it is worth a test of its own rather than
  being implied by the edit assertion.
- The old "double-click with no socket says so and stays put" subtest covered
  `startDrop`'s `client == nil` path. That path is now only reachable from the
  chip, so the coverage moved to `TestActionBarClick` as "Send with no socket
  says so and stays put" rather than being dropped.

## The chip colors

The request named blue / green / red for Edit / Send / Delete. The design
question was whether to color the *fields* or the *letters*.

**Letters.** Four saturated backgrounds in a row would out-shout the list the
bar acts on, and the fields are already carrying live-vs-inert — hue on top of
them would be a second signal for something the surface already says. A chip
fills with its own hue **only when the focus is on it**, so exactly one field is
lit at a time, which is what "pressed" has to look like to be worth the ink.

`listAction` grew a `tint` field: the letters' color ordinarily, and the whole
field the chip lights up with under the focus.

| chip | tint | why |
| --- | --- | --- |
| ✚ Add | `colInfo` blue | the calm end of the row, and the one always-live button |
| ✎ Edit | `colStraw` pale yellow | not amber — see below |
| ➜ Send | `colAccent` green | the palette's own color of consequence |
| ✖ Delete | `colErr` red | the only warning before the confirm asks |

The first pass put blue on Edit and left Add neutral white. The user asked to
move blue to Add and make Edit a light yellow, which is better for a reason
beyond the hues: **Add is the only chip that is never greyed** — it needs nothing
selected. Under the first assignment the always-live button was also the only
colorless one, which read backwards. Now, with nothing highlighted, the bar is
one blue chip and three grey ones, and that picture *is* the state.

It also puts the row in the order a prompt's life actually goes — make it,
change it, send it, throw it away — running cool to hot, left to right.

Decisions worth keeping:

- **Send takes `colAccent`, not `colOk`.** `colAccent` is the palette's declared
  "green everything of consequence is drawn in", and sending is the one action
  on this screen that reaches out of the program. Using `colOk` would also have
  put two near-identical greens on one row.
- **The yellow is straw `#e0d49a`, not `colWarn`'s amber `#e0b64e`.** `styles.go`
  reserves amber hard: it is the one warm thing on screen so a fuzzy hit jumps
  out of the letters around it — and `matchStyle` paints those hits in the rows
  *directly under* the bar. An amber Edit chip one line above them would cost
  that highlight the only job it has. Paleness has a second reason: a yellow
  taken down to `colOk`/`colErr`'s brightness stops being yellow and turns
  olive, so straw is the set's one deliberate exception to the level-brightness
  rule, and the comment says so rather than leaving it looking like an oversight.
- **Inert chips drop the tint.** `btnOffStyle` is untouched, so a button with
  nothing to act on goes grey *and* colorless. Grey is what "nothing to act on"
  says; a tinted inert chip would be saying two things.

Giving every chip a hue also **collapsed the code**. The first pass needed two
fields, `tint` and `fill`, purely because Add's near-white would have glared as
a lit field — the same failure the title chip's comment already records. With
all four carrying a color, `tint` does both jobs and the second field is gone.

Styles are derived, never nested: `btnStyle.Foreground(...)` returns a copy, so
each chip is still rendered by exactly one style. That constraint is load-bearing
— the note in `styles.go` about an outer reset clobbering an inner one still
holds, and was extended rather than worked around.

## The row cursor

The highlighted row was marked by a bold green `▌` block. It is now a bold
muted-magenta `❯`.

Magenta is the only color in the program from outside the palette, and that is
the point: the mark saying *here* has to be findable at a glance in a pane full
of green, and a green mark on green rows is the one thing that can't be. Muted
(`#b47fae`) rather than full magenta because it is a pointer, not an alarm — it
should be the first thing found, not the loudest.

The arrow is the shell's own prompt glyph. A list of prompts waiting to be handed
to an agent is closer to a shell than to a menu, and `❯` says "this one is next"
where the block only said "this one". Thickness comes from `Bold` — `❯` is
already the heavy form of the chevron, and weight is the only other lever a
terminal gives.

Three details:

- **One column wide**, checked with `lipgloss.Width` before committing to it
  (against `▌ ❯ ➤ ▶ ❱ ⯈ ‣`, all of which measure 1). A glyph the font drew double
  would push every row one column right of the action bar, which is hit-tested
  against a constant.
- **Both call sites changed**, and now share a `cursorGlyph` constant. The
  backlog list and the attachment editor had the same literal in two places;
  leaving one green would have read as a bug, and the constant stops them
  drifting.
- **`barStyle` was renamed `cursorStyle`.** It no longer draws a bar, and a style
  named for a shape it stopped having is how the next reader gets misled.

## Lightening the straw

`colStraw` went from HSL 50° 53% 74% to **45° 52% 86%** — twelve points of
lightness, five degrees of hue.

"Lighten by 12%" was read the way colour tooling means it (SCSS/Less `lighten`):
twelve *percentage points* of HSL lightness, not a 12% relative bump. The
relative reading would have been `#ebdfbd` at L=83%, and it is the fallback if
the chosen value proves too pale.

The five degrees deliberately stop short of "as red as it goes". `colWarn` sits
at 42.7°, and landing on it would leave Edit and the fuzzy-match highlight
sharing a hue, separated by paleness alone — which is most of what the earlier
straw/amber decision was buying.

Flagged at the time, and still worth an eye in a real pane: at L=86% the lit Edit
chip is close enough to `colFgHi` (95%) to be the near-white glare that made Add
need a separate `fill` in the first place. Focused Send fills at L=50%; focused
Edit now fills at 86%, so the two "pressed" states no longer look like the same
kind of event.

## The query box never said it had the focus

Reported as "refocussing the search doesn't give me any indication it is back in
focus". The cause was worse than a missing highlight.

`ti.Focus()` was called once in `newFuzzyList` and **never blurred**. The box
rendered a blinking cursor the whole time an action-bar chip was lit — it
claimed a focus it did not have. So there was nothing for *regaining* the focus
to reverse: the box looked identical before, during, and after.

Both halves move now:

- **The cursor.** All five focus flips route through one `setActionFocus`, which
  blurs the input when a chip lights and refocuses it when the focus comes home.
- **The rails.** `searchFieldOnStyle` draws them in `colAccent` when focused,
  falling back to the existing `colChrome` otherwise — so the unfocused state
  looks exactly as it always did and only the focused state gained a signal.

`fuzzyList.view` reads `l.input.Focused()` rather than taking a second flag, so
the rails and the cursor cannot disagree, and the drop picker (which never
blurs) simply stays lit for free.

**The hazard this introduced.** A blurred `textinput` silently drops the keys it
is handed. Typing from a lit chip is supposed to bring the focus home *and* land
the character, so `setActionFocus(false)` has to run **before** `editQuery` — or
the one keystroke that returned the focus is the one keystroke swallowed. That
ordering is now the subject of its own test.

`backToList` drops the returned blink command rather than threading it through
ten callers. A focused box still draws a steady cursor, and a stage change
redraws the screen anyway; the comment says so, so it does not read as an
oversight.

## The whole-row highlight

Asked for as "some kind of a subtle whole row highlight on selected, even for a
completed todo". The "even for a completed todo" half was the tell.

**The bug.** `highlightName` ran `if strike { base = doneStyle }` *after* the
selected branch, so done won outright. A completed row under the cursor rendered
**identically** to a completed row anywhere else — the highlight did not exist
for half the list. They are independent now: the strike says what the todo is,
the field says which row the keys are on. A selected done row also comes up one
tier of grey (`colFaint` → `colDim`, the new `doneSelStyle`), because receding is
right for a row nobody asked about and wrong for the row being looked at — and
it recedes further still against the lifted field.

**The mechanism.** A terminal background only covers the cells a style actually
writes, so wrapping a finished row in one background style cannot work: the
segments end in resets, and the first of them drops the field for the rest of
the line. Every segment therefore goes through one `onRow` helper — cursor,
badge, each letter of the name, the description, and the padding out to the
right edge.

That forced a small refactor: badges arrived at the list **pre-rendered**, which
is exactly the text you cannot put a background behind. `listItem` now carries
the glyph and its style separately (`badge` + `badgeStyle`) and the renderer owns
the whole row. The old comment explaining why badges were written verbatim was
describing the constraint that made this necessary.

`colPanel` is reused rather than a new tone invented — it is already the
palette's one step up from the page. The open question, raised and left: it is
also what an inert chip sits on, so one tone now means both "row you are on" and
"button you cannot press". Different enough in context; a dedicated tone is one
line if it ever reads as ambiguous.

**Width is a parameter to `view()`, not a field on the list.** `applySizes`
returns early below 20 columns and only touches the active stage, so a stored
copy would go stale silently — and it would go stale as a highlight that stops
short of the edge, which is the one failure mode hardest to attribute.

## Verification

Nothing here was judged by reading hex. Throwaway probe tests (deleted after
each look) printed the real output to stdout as truecolor escapes: `actionBar()`
in all six states — no selection, with a selection, and the focus on each of the
four chips — then the whole `View()` with a row highlighted, then the query line
across focused / chip-lit / refocused, then rows with an open and a completed
todo selected.

The probes are why the straw/amber distinction, the glyph width, the swallowed
keystroke, and the full-width field were settled rather than assumed. They are
cheap: a `zz_*_test.go` in the package, one `go test -run`, then `rm`.

**The tests were checked against the old behaviour, not just run.** Reverting
`highlightName`'s switch to the previous strike-wins logic fails two subtests
with the right messages, which is the difference between a regression test and a
decoration.

Notes for next time:

- lipgloss v2 has no `SetColorProfile`/`TrueColor` — styles emit ANSI
  unconditionally and downsampling happens at the writer, so a probe needs no
  profile setup at all.
- `m.View()` returns a `tea.View`, not a string.
- `lipgloss.Width` on a rendered line is the cleanest non-brittle assertion
  available for "the field runs the whole row" — far better than matching escape
  sequences.

`gofmt -l` empty, `go vet ./...` clean, full suite passes.

## Files

Across the session, against `c5dc28d`:

    README.md    |  15 ++---
    fuzzylist.go |  80 ++++++++++++++++------
    styles.go    |  96 ++++++++++++++++++++++++---
    ui.go        | 135 ++++++++++++++++++++++++++--------------
    ui_test.go   | 197 ++++++++++++++++++++++++++++++++++++++++++++++++++-------

## Notes

- The deferred-click version was offered and not built. If it ever comes up
  again, the sketch is in "The click proposal" above — the blocker is the 500ms
  stall on every single click, not the implementation.
- `m.lastClickRow` / `m.lastClickAt` and `doubleClickWindow` are unchanged; only
  what the second click *runs* moved.
- The work shipped as five commits rather than one, each pushed as it was
  finished: the click swap + first chip colors, the palette revision, the straw
  lightening, the focus fix, the row highlight. `styles.go` carried two
  unrelated changes at one point and was split by hunk rather than lumped
  together.
- Session notes are kept in their own `docs:` commit here, per the repo's
  history. Twice in this session `git add -A` swept the doc into the code commit
  and it had to be split back out before pushing — worth staging deliberately
  next time.
- Not yet run in a real cats pane — worth a `go run .` before trusting any of
  this against the actual terminal background. Three things to watch:
  - the lit Edit chip is the brightest field on the bar and got brighter with
    the lightening (fills at L=86% against Send's 50%); dropping `colStraw`
    toward `#d3c489`, or giving Edit its own `fill`, fixes it without going near
    amber;
  - `❯` is a glyph some terminal fonts render from a fallback, which is the one
    way it could still come out looking thin;
  - `colPanel` now means both "the row you are on" and "a button you cannot
    press". Context separates them, but a dedicated tone is one line if it does
    not.
