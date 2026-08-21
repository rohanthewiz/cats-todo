# Right click — a plan

Right click is currently a dead gesture: `updateMouse` (ui.go:682) drops every
button that isn't `tea.MouseLeft`, on every screen. Meanwhile the terminal's own
right-click menu is suppressed wherever we ask for mouse reporting
(`MouseModeCellMotion`), so the user pays for the gesture and gets nothing back.
This plan gives right click one consistent grammar across every screen that
already listens to the pointer.

## The grammar

Two rules, applied everywhere, so the hand only has to learn them once:

1. **Right click on a thing opens that thing's menu.** A small context menu
   anchored at the pointer, listing what can be done to the row/word under it.
   Left-clicking an entry (or ↑/↓ + enter) runs it; esc, a click elsewhere, or a
   second right click dismisses it. This is the desktop convention, and it is
   also the *safe* reading: a menu never acts on the press itself, so a stray
   right click costs nothing — the same bargain single-left-click-only-selects
   already makes.
2. **Right click on nothing means "back".** On a sub-screen (pickers, panels),
   a right click that lands on chrome or empty space does what esc does. On the
   root list it does nothing — there is no "back" from home, and a no-op beats
   a surprise. This gives a mouse-only hand a way *out* of every panel it can
   click its way *into*, which today it doesn't have.

Deliberateness is preserved: everything reachable from a menu takes two clicks
in two places, which is the same bar the ✉ Send chip already sets for the one
action that reaches outside the program. Delete keeps routing through
`stageConfirm`. Entries the state refuses (drop a frozen prompt, reorder under a
filter) render greyed but stay clickable, and clicking one says why in the
status line — contract #4, "refuse in words", never a silent no-op.

## What each screen's menu holds

Only screens already in the `MouseMode` list (ui.go:3641) get right click —
contract #5 stands: no new mouse reporting, and `stageView` keeps the
terminal's own selection untouched. `stageConfirm`, `stageImages`,
`stageSchedule`, `stageSession` stay pointer-free for now (see Phase 4).

| Screen | Right click on… | Menu |
|---|---|---|
| **List** (`stageList`) | a todo row | ✎ Edit · 👁 View · ✉ Send (paste) · ⚡ Send & run · ⏰ Schedule… · ➦ Export… · ✔ Done / ↩ Reopen · ❄ Freeze / Thaw · ✖ Delete… |
| | a group heading | Fold / Unfold; on the Done heading also 🧹 Clear done… |
| | header / action bar / empty | nothing (root screen — rule 2's no-op case) |
| **Drop picker** (`stageTarget`) | an agent row | ⎘ Paste prompt · ⚡ Paste & run — the pointer finally gets the modifier-chord path (`chooseTarget(dropRun)` is keyboard-only today) |
| **Export picker** (`stageExport`) | a destination row | ⎘ Copy here · ➦ Move here — same gap: move is chord-only today |
| **File picker** (`stageFiles`) | a directory row | ↳ Enter folder · @ Insert path (export-browse mode: 📂 Export here) |
| | a file row | @ Insert path (a one-entry menu is fine; it confirms the target before acting, which on a dense file list is worth a click) |
| **Form** (`stageForm`) | a misspelled word in the prompt | its top corrections (≤5) · ＋ Add to dictionary — the desktop editor gesture, and a faster path than caret-to-word + ctrl+l |
| | a selection (promptsel) | ⧉ Copy · ✂ Cut |
| | elsewhere in either field | ⧉ Paste · ⌘ Select all |
| | toolbar / labels | nothing |
| **Spell panel** (`stageSpell`) | anywhere | back (rows are single-meaning; a menu would just restate the left click) |
| **View panel** (`stageViewOpts`) | empty space | back |

Menu entries reuse the chips' glyphs and tints (`styles.go`) and print their
key chords on the right edge, so the menu teaches the keyboard the same way the
action bar does.

## The component — `ctxmenu.go`

One reusable overlay, owned by the model like the drag state is:

```go
type ctxMenu struct {
    items  []ctxItem // label, chord hint, tint, disabled+reason, run func-id
    x, y   int       // anchor: the click, clamped so the box stays on screen
    sel    int       // keyboard cursor
    onRow  int       // what the menu is about (row index / word span), so
}                    // entries act on the clicked thing, not the highlight
```

- **Open** = focus first: like every left click, opening a row's menu moves the
  list highlight onto that row before the menu draws, so pointer and keyboard
  agree on the subject and the menu's entries can mostly delegate to the
  existing per-row actions (`runAction`, `beginEdit`, `toggleDone`, …).
- **Render**: composite over the stage's frame with lipgloss v2's
  Canvas/Layer (in our pinned v2.0.5 — canvas.go/layer.go). Clamp the box so it
  never crosses the pane edge; open upward when the click is in the bottom
  rows. No new mouse mode needed — cell motion already reports the press.
- **Routing**: while a menu is open, `Update` sends keys and clicks to the menu
  first (a stage-orthogonal overlay flag, not a new `uiStage` — the menu must
  work identically on five stages, and a stage would fork every screen's
  update). Esc / outside-click closes and falls through *swallowed* — the
  click that dismisses a menu must not also select what it landed on.
- Right-press **does not arm a drag** and clears any pending double-click
  window — the two gestures stay disjoint.

## Phases

Each phase ships alone: code + tests + README + minor version bump at the end.

1. **`ctxmenu.go` + the list.** The component, overlay routing, the todo-row
   and heading menus, and rule 2's "back" on the two simple panels (spell,
   view opts). This is the flagship and settles every convention the rest
   inherit.
2. **The pickers.** Target (paste/run), export (copy/move), files
   (enter/insert). Small menus, big payoff: the chord-only strong actions get
   a pointer path.
3. **The editor.** Spell suggestions under the pointer (reuse
   `internal/spell` + the panel's correction machinery), copy/cut on a
   selection, paste/select-all elsewhere. Paste needs `clipboard.go`'s
   read path; corrections reuse the panel's replace-word code so undo behaves
   identically.
4. **(Optional, separate decision.)** Give `stageImages` / `stageSession` /
   `stageSchedule` mouse reporting at all. That drags in *left*-click
   expectations too, so it is its own feature, not part of this one.
   `stageView` is never included — its whole point is terminal-native
   selection (contract #5).

## Tests

- Drive with `tea.MouseClickMsg{Button: tea.MouseRight, X: …, Y: …}` through
  `m.Update`, the way `formmouse_test.go` / `dragorder_test.go` drive left
  click. Per phase: menu opens on the right thing (row focus moved), entries
  match the row's state (frozen row shows Thaw, greyed Send), disabled entry
  click sets the status line and does nothing else, outside click dismisses
  without selecting, esc closes, geometry pinned against a real rendered frame
  (the `TestExportRowsMatchWhatIsDrawn` pattern), and a regression test that a
  right click mid-drag / mid-double-click-window cancels both cleanly.
- A frame test that the menu clamps at every pane edge and flips upward near
  the footer (`TestWindowSizeMsgNeverPanics` is the precedent for geometry
  paranoia).

## Open questions (decide in phase 1)

- **⚡ Send & run in the list menu**: the target picker still has to be chosen,
  so "run" here means "open the picker pre-armed for run". Worth it, or does
  the list menu offer only ✉ Send and let the picker's own menu carry run?
  Leaning: list menu offers Send only; run lives where the target is named.
- **Heading right click**: menu (consistent) vs. instant fold (faster)? A
  one-entry menu that says "Fold" is honest and still one extra click; instant
  fold is undoable by the same gesture. Leaning: instant fold, since it obeys
  the menu grammar's spirit (nothing irreversible on the press).
