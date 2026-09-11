# Session: the high-value gem, and the third bar tier it forced — v0.30.0

Session ID: `afcfdad2-98c4-46b2-a786-0d3cd6d6796e`
Date: 2026-09-11

The ask, in full:

> "Introduce a gold badge annotation next to Quick win that would indicate a high
> value todo"

and, after it was built, the question that changed the glyph:

> "Hmm, just checking. Do you have a gold bar emoji rather than the gold medal
> one?"

Touched: `store.go`, `annotations.go`, `styles.go`, `annotbar.go`, `listmenu.go`,
`fuzzylist.go`, `ui.go`, `cli.go`, `complete.go`, `bundle.go`, `main.go`,
`cats-plugin.toml`, `README.md`, `highvalue_test.go` (new), `annotations_test.go`,
`prioview_test.go`.

## The fourth annotation was cheap; the bar it sits on was not

`annotations.go` still says in as many words what a mark costs — "a field on
Todo, a field here, a line in each of the three methods below, and an entry in
`annotSlots`" — and that held exactly. The list row, the prompt view's meta line
and the CLI's echo all walk `annotSlots`, so adding the slot lit all three up
without any of them being touched.

What was *not* free was the form's annotation bar. It had two tiers and the
compact one came to exactly 30 cells with five segments on it — 30 being the
narrowest pane this form is drawn in at all, pinned by
`TestAnnotBarFitsNarrowPanes`. A sixth segment is a box, a two-cell emoji and a
gap: the compact tier went to 36 and the floor was gone.

The bar may not wrap (it sits on `formAnnotRow`, hit-tested, and a wrapped bar
would put the prompt editor a line below where every click on it is aimed) and
may not drop a segment. So it concedes the way every chip bar in this program
concedes — words, then gaps, then bare glyphs — and now does it three times:

```
full     ☑ 🍏 Quick win   ☑ 💎 High value   Priority  (•) none  …    95 cells
compact  ☑ 🍏  ☑ 💎  ( ) –  ( ) △  (•) ▲  ☐ ⚑                        36 cells
tight    ☑🍏  ☑💎  ( )–  ( )△  (•)▲  ☐⚑                              30 cells
```

The tight tier's concession is the space *inside* each segment, so the box sits
against the mark it is the state of. That reads as one token rather than two,
which is the right reading anyway, and it is the last cell the bar has to give. A
seventh mark will have to find its cells somewhere else again — the comment on
`annotBarTiers` says so.

Two ad-hoc branches became a table. `annotBarTier{texts, gap, divider}` is chosen
once as a unit, widest-that-fits wins, and the narrowest is a **floor** drawn even
when the pane cannot hold it: an overflowing bar is visible, a silently missing
control is not.

## Cost and payoff are two facts, not one

The framing that decided the rest. The apple says how *cheap* a prompt is; the
new mark says how much it *pays*. Either alone is half an answer — a five-minute
typo fix is cheap and worth almost nothing, a month-long migration is worth a
great deal and will not be picked up between two meetings — and a row wearing
both is the one to reach for.

That is also why it is not a fourth priority level, for the same reason the fruit
is not a third. Priority asks *how much does this matter right now*; value asks
*how much is it worth at all*, and the two come apart in both directions. A
refactor that pays forever but can wait until the release is out is high value and
not critical. A build break that has to clear this morning is critical and worth
nothing once it has cleared.

So the slot goes next to the fruit, not next to priority: `priority, fruit,
value, flag`. The two qualifiers are adjacent because together they are one
reading, and the bar puts them side by side for the same reason — a hand that has
just answered "cheap" is one `→` away from answering "and worth it".

## The glyph, and the argument that had to be rewritten with it

Built first as `🏅`, on this reasoning: the badge wants gold, both of the
palette's warm hues are already spoken for on the very same row (`colTodo` is
high priority's `△`, `colWarn` is the fuzzy-match highlight), a text glyph would
have had to borrow one and mean a third thing by it — and an emoji brings its own
gold and borrows nothing.

Then the user asked for a gold *bar*. There isn't one; Unicode has no ingot. What
the question was really pointing at, though, is a semantic split worth having:

- a **medal** is *awarded* — "this earned something", a judgement about work
  already done;
- a **gem** is simply *worth* something, which is the fact being recorded about a
  prompt nobody has started.

They picked `💎`. Widths were measured before offering anything, since the packed
group depends on it — `🏅 🥇 🪙 💎 🟨 ⭐` are all two cells, `✦ ◆` are one.

`💎` is blue, so the gold argument above was no longer true as written. The
honest replacement is broader and stronger: this palette has **three** hues that
mean something on a list row and all three are taken — yellow (high priority),
amber (fuzzy match), and the one cool blue (`colInfo`, the flag). A fourth text
glyph would have to borrow one and mean something new by it; an emoji paints
itself and takes no hue out of the palette at all.

That does put a blue gem a slot away from the blue pennant, and the comment says
so rather than papering over it: a filled faceted solid against an outlined
pennant is a shape difference before it is a colour one, and the gem's saturated
cyan is the font's rather than `colInfo`'s muted blue. The distinct-glyph
contract holds on *shape*, which is what `TestEverySlotDrawsADistinctGlyph` pins.

Nothing in the code had to be renamed for the swap — `HighValue`, `valueGlyph`,
`valueStyle`, `valueMark`, `annotSegValue`, `listMenuValue`, `--high-value` were
all named glyph-agnostically. Only prose moved: "the gold badge" → "the gem",
plus two test names.

## Closed rows: it goes quiet, like the apple

The three marks now part company three ways on a done or frozen row, and each for
a mechanical reason rather than a different opinion:

| Mark | On a closed row | Why |
|---|---|---|
| `▲ △` | recedes to greys | text glyph, a foreground reaches it |
| `⚑` | recedes to greys | text glyph, and the note is worth as much on finished work |
| `🍏` | **goes away** | emoji, no grey to recede into |
| `💎` | **goes away** | same, and "this one pays" argues for picking work up |

`valueMark` mirrors `fruitMark` exactly, including handing the *closed* styles
back alongside the empty glyph — the prompt view prints the label in them, which
is how a finished prompt still reads `high value` in words on the screen someone
opens to find out what was said about it.

## `--high-value`, not `--value`

The one CLI judgement call. `Todo.HighValue` / `"highValue"` for the field, but
the flag is spelled out because a bare boolean called `--value` reads on a command
line as a flag that wants one: `cats-todo add --value "fix the thing"` looks like
it is swallowing the prompt behind it. Long-only, like `--priority`, `--fruit` and
`--flag` — it is a fact about the prompt, not one of the short flags that say
where the prompt goes.

Verified end to end:

```
$ cats-todo add --high-value --fruit -t "split the store" "it pays every time …"
added to the project backlog, marked low-hanging fruit · high value (…/.cats-todo/todos.json)

  "fruit": true,
  "highValue": true
```

## Compatibility, as always

`HighValue bool json:"highValue,omitempty"`. An unmarked backlog is byte-identical
to what it was before the field existed; clearing the mark takes the key back out
rather than storing `false`; an older binary ignores it. Pinned by
`TestAnnotationsStayOutOfTheJSONWhenUnset`, which now checks all three annotation
keys.

## Tests

New `highvalue_test.go` covers the mark end to end: the row (and the apple-then-gem
order), the two-cell width, the closed-row recession *and* that the words survive
it, the form checkbox through a real `→`/`space` walk, the menu row both ways with
its status line, and `TestAnnotBarConcedesInOrder` — tiers strictly narrow, no tier
drops a segment, the widest that fits wins at the seam on either side, and the
floor is drawn even one cell under.

Two existing tests walked the bar by a literal count of `→` presses and broke.
They now count off the constants (`annotSegPrioCritical - annotSegFruit`), so the
*next* mark moves them instead of breaking them.

`go vet` clean, `go test ./...` green, version bumped in both places to
**v0.30.0** (a feature → minor).
