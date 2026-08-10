# Session: Send from the editor, and a bar that shrinks instead of wrapping

Session ID: `3c75743a-9d1b-4f82-a7d0-4253ab9dea5f`
Date: 2026-08-10

The ask:

> "I want to be able to send to an agent directly from the editor. Use the green
> color for the new Send button. Use white for save, cyan for Images (make that
> true throughout the app), blue for Session. Here Send has no shortcut key, so
> we don't accidentally send. Also don't send if the prompt is empty."

then, after the first pass flagged the narrow-pane cost of a sixth chip:

> "1) Please add the cyan hue to the toolbar chip also - the list's 📎N badge
> 2) go ahead with an icon-only tier below the 63 col width."

Touched: `ui.go`, `styles.go`, `fuzzylist.go`, `README.md`, `ui_test.go`,
`formmouse_test.go`. No new files.

## The editor's toolbar

Six chips now: **✔ Save · ↵ Newline · ❐ Images · ⚙ Session · ✉ Send · ✖ Cancel**.

| chip | hue | why |
|---|---|---|
| Save | `colFgHi` white | the row's default; a default reads best with no hue at all |
| Newline | `colStraw` | unchanged |
| Images | `colCyan` **(new)** | the app's attachment hue |
| Session | `colInfo` blue | |
| Send | `colAccent` green | the palette's color of consequence |
| Cancel | `colErr` red | the row's only warning, and last where a row is read from |

Save gave the accent up because green is what the list's bar already spends on
Send — the one action on either screen that reaches out of the program.

**`sendForm` saves first, then opens the picker.** It calls `startDrop(ref)` —
the list's own entry point — so a send from the editor takes exactly the path
`shift+enter` takes from a row: same guards, same picker, same `performDropCmd`.
Saving first is what makes it unsurprising in both directions: the agent gets the
text that is on disk, and a send the picker then refuses (no socket, a drop in
flight, a frozen prompt) still leaves the edit kept. The refusal costs the send,
never the typing.

**`saveForm` split into `persistForm`.** The add path mints the todo's id
internally, so there was no way to name what had just been written. `persistForm`
returns `(model, todoRef, bool)`; `saveForm` ignores the ref, `sendForm` needs
it. One writer, so the two exits cannot drift into saving different things.

**Send is click-only, and that is the point.** No chord anywhere reaches
`formActionSend`. Every other button on the row is recoverable — a save can be
edited again, a cancel loses one edit — but a prompt handed to a live agent has
left the program, and the form is a screen being typed into with both hands.
`TestFormSendIsClickOnly` pins it, including that `shift+enter`/`alt+enter` (the
list's own drop chord, the keys most likely to be pressed out of habit) still
insert a newline here.

**Empty prompts never reach the picker.** `persistForm` refuses them with the
same message ✔ Save gives and returns `ok=false`.

**Send sits between Session and Cancel, not beside Save.** The two writes are
deliberately not neighbours: Save is the button the hand goes to without looking,
and a send is not undoable. Where it does sit reads as a sentence — set how it
runs, then run it.

## The cyan, made true elsewhere

`colCyan = #6ed8d0` is built as `colInfo`'s sibling rather than picked freehand:
same lightness (64%) and saturation (58%), 31° round the wheel (176° against
207°). The two chips are now adjacent on the toolbar, and matching everything but
hue is what lets a glance separate them while the row still reads as one set —
brightness is the bar's grammar for live-versus-inert and must not be spent on
telling two live chips apart.

**The list's `📎N` could not simply be tinted.** It was `fmt.Sprintf` pasted onto
the front of the row's `desc` string, and a highlighted row draws its field per
segment — styled text inside a larger string ends in a reset that drops the
field for everything after it. So `listItem` gained `descMarks []descMark` (text
plus style), exactly mirroring the existing `badge`/`badgeStyle` pair and for the
same documented reason.

`rebuildList` now emits the three leading flags as marks in the order they were
already drawn — `⏰ time`, `⚙`, `📎N` — the first two staying grey, the count
taking the new `attachStyle`. One cosmetic side effect, accepted: the `⚙`→`📎`
gap widened from one space to the two every other mark boundary uses.

`attachStyle` is deliberately *not* on the badge ramp. The badge column holds the
row's state (done, frozen, scheduled) and there is only ever one; the marks are a
list leading the description. Sharing the badge's styles would make a picture
look like a state.

## The tier, and the problem that earned it

A sixth chip pushed the toolbar's no-hints width from ~54 to ~63 columns. Past
that the row wraps — and a wrapped chip is one the pointer can no longer find,
because every click is hit-tested against the single row `formBarRow` names. This
was flagged at the end of the first pass rather than papered over; the follow-up
asked for the fix.

`chipTier` replaces the old `withHints bool`:

```
tierHints  → "✔ Save enter"
tierLabels → "✔ Save"
tierIcons  → "✔"
```

`barTier(acts, width, indent)` picks the widest tier that fits, and both bars go
through it, so the list's bar gained the third tier too:

| | hints | labels | icons |
|---|---|---|---|
| editor toolbar (6 chips) | ≥97 | ≥63 | 23 cells |
| list bar (4 chips) | ≥69 | ≥38 | 16 cells |

**Dropping a button was the alternative and is the wrong trade.** A control that
vanishes below some width is one the user cannot learn is there — and the bars
are the only place two of these actions exist at all, since ✉ Send has no chord
anywhere. `barShowsHints`/`formBarShowsHints` are now one-liners over the tier,
so the footers' existing dedup logic is untouched.

**The one gap the icon tier opened:** at glyphs-only, ✉ is the single chip no
footer chord stands for. The form footer prepends `✉ click sends` at
`tierIcons` — at the *front*, because `fitFooter` trims from the right and the
pane doing the trimming is exactly the pane that needs the hint.

`icon()` is the label's first rune, which the pre-existing one-cell-dingbat tests
(`TestFormBarIconsAreOneCell` and its list counterpart) already guarantee is a
single column. Hit-test spans come from the same `chipText` that draws, so every
chip stays clickable at every tier.

## Tests

- `TestFormBarClick` — two new cases: Send saves and aims the picker at the todo
  the form just wrote (`dropTodo` compared against the saved id, so a regression
  that sent the list's highlight instead would fail); Send refuses an empty
  prompt with the form still open and nothing written.
- `TestFormSendIsClickOnly` — no hint, bare label even at `tierHints` (a
  trailing pad would hang a live column of button off the right edge), and the
  two drop chords do not send.
- `TestFormBarTiers` — tier per width, no wrap, every button present with a
  non-empty span, footer teaches ✉, and a click on the bare ✉ glyph still sends.
- `TestActionBarRender` — extended to the third tier.
- `TestRowDescMarks` — mark order and which one carries a hue. Styles are
  compared by what they *render*: `lipgloss.Style` holds slices and is not
  comparable with `!=`.

`gofmt`, `go vet`, `go test ./...` all green.

## Verified by eye, then deleted

A throwaway `zz_scratch_bar_test.go` printed the bars at each width and the rows
with marks, then was removed. Worth recording what it confirmed, since no unit
test asserts on raw escape sequences: `📎2` renders `38;2;110;216;208` (the cyan)
with the highlighted row's field `48;2;59;82;69` intact behind it, and the
icon-only toolbar measures 23 cells.

## Not verified

The live in-cats path: a send from the editor into a real pane over the control
socket. The picker is reached and aimed correctly in tests with a client whose
socket does not resolve, which exercises everything up to the drop itself.
