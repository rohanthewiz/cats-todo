# Session: the double-click swaps to edit, and the chips get their colors

Session ID: `0a9f006d-57cf-44c0-84c1-054eb6d37838`
Date: 2026-07-31
Repo: `cats-todo`

## The ask

Two questions, one after the other:

1. "Could this click handling work? A single click selects and goes into edit
   mode for the todo, while dbl clicking sends the todo to the agent?"
2. "Is it possible to use some color on the chips. Like blue for edit, green for
   send, red for delete?"

The first was a design question, answered before anything was changed. The
second was a straightforward yes, with the interesting part being *where* the
color goes.

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

`listAction` grew two fields, `tint` (the letters) and `fill` (the field under
focus):

| chip | tint | fill | why |
| --- | --- | --- | --- |
| ✚ Add | `colFgHi` | `colAccent` | neutral — see below |
| ✎ Edit | `colInfo` | `colInfo` | the new blue |
| ➜ Send | `colAccent` | `colAccent` | the palette's own green |
| ✖ Delete | `colErr` | `colErr` | the only warning before the confirm asks |

Three decisions inside that table:

- **Send takes `colAccent`, not `colOk`.** `colAccent` is the palette's declared
  "green everything of consequence is drawn in", and sending is the one action
  on this screen that reaches out of the program. Using `colOk` would also have
  put two near-identical greens on one row.
- **Add stays neutral.** It is the only action that needs nothing selected, and
  a third hue would have made the row read as four warnings. It still *fills*
  with `colAccent` under focus, because a chip filled with near-white would be a
  lamp in a dark pane — the same failure the title chip's comment already
  records. That asymmetry is the entire reason `fill` exists as a separate field.
- **Inert chips drop the tint.** `btnOffStyle` is untouched, so a button with
  nothing to act on goes grey *and* colorless. Grey is what "nothing to act on"
  says; a tinted inert chip would be saying two things.

New constant, `colInfo = "#6ea9d8"` — the one cool hue in a warm-green palette,
mixed to sit at the same brightness as `colOk` and `colErr` so the three read as
one set rather than three unrelated colors that happen to be adjacent.

Styles are derived, never nested: `btnStyle.Foreground(...)` returns a copy, so
each chip is still rendered by exactly one style. That constraint is load-bearing
— the note in `styles.go` about an outer reset clobbering an inner one still
holds, and was extended rather than worked around.

## Verification

Colors were checked by rendering, not by reading hex. A throwaway
`zz_colorprobe_test.go` printed `actionBar()` in six states — no selection, with
a selection, and the focus on each of the four chips — straight to stdout as
truecolor escapes. Confirmed the tints land, the focused chip flips to its own
field with dark text, and the inert row is uniformly grey. The probe was deleted
afterward.

Note for next time: lipgloss v2 has no `SetColorProfile`/`TrueColor` — styles
emit ANSI unconditionally and downsampling happens at the writer, so a probe
needs no profile setup at all.

`go vet ./...` clean, full suite passes.

## Files

    README.md   | 15 +++++++-------
    styles.go   | 18 ++++++++++++--
    ui.go       | 55 ++++++++++++++++++++++++++---------------
    ui_test.go  | 53 +++++++++++++++++++++++++++-------------

## Notes

- The deferred-click version was offered and not built. If it ever comes up
  again, the sketch is in "The click proposal" above — the blocker is the 500ms
  stall on every single click, not the implementation.
- `m.lastClickRow` / `m.lastClickAt` and `doubleClickWindow` are unchanged; only
  what the second click *runs* moved.
- Not yet run in a real cats pane — worth a `go run .` before trusting the
  colors against the actual terminal background.
