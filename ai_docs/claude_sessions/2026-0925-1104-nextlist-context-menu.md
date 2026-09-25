# Next List context menu

Session: `42b90c53-2b7e-4a3b-8f07-1d8dd8be66ce`
Date: 2026-09-25

## Ask

"Include a context menu for the Next list too" (after loading
`2026-0924-1939-nextlist-hover-card`). Then: "The Next list prompt Editor
(btw the title should say Next List in there somewhere — like 'Next List
Prompt Editor') has all these options, so why is the Next List context menu
so sparse?" Then `/sw`.

## First pass: a small menu

Right-click an item for a `menuBox` (menu.go), the same box, keys, placement
and dim-rather-than-omit rule as the backlog's menu (`listmenu.go`). The first
version had four rows: ✚ New prompt…, ✉ Send…, ⧉ Copy ID, ⧉ Copy as prompt
(the exact bytes Send delivers, citation included, for an agent outside cats).

Wiring:

- `nextmenu.go` (new): `nextMenu{menuBox; item nextItem}`. It carries the
  item by value, as `listMenu` carries its `todoRef`: the labels were resolved
  from that item. It also holds `rightClickNext`, `openNextMenu`,
  `updateNextMenu`, `clickNextMenu`, `pressNextMenu` and `overlayNextMenu`.
- `nextlist.go`: `promptFromNext` and `sendFromNext` were split so the menu
  can pass its carried item: `promptFromNextItem(it)` and `sendNextItem(it)`.
  `updateNextList` hands keys to the open menu first, and `clickNext` hands
  clicks to it. `beginNextList` resets it. The footer gained
  `right-click menu` at the tail, so a narrow pane drops it first.
- `ui.go`: a `nextMenu` field. `updateMouse` routes `MouseRight` on
  `stageNextList` to it. A resize and `backToList` clear it, and
  `renderStage` composites it over the hover card.
- The hover card is refused while the menu is up, both in `nextHoverMotion`
  and in `hoverDwell`'s Next List branch.
- `menu.go`'s header now says there are three menus.

## Why it was sparse, and the fuller menu

The user compared it with the add form's options (Images, Session, Save,
Send, and the Quick win / Value / Priority / Info / Flag bar). The reason:
all of those are settings on a **backlog prompt**, and a Next List item is a
paragraph in a file the page only reads. The fix was to make every extra row
*create* that prompt with the setting applied. Asked which rows to add, the
user took all four options:

```
╭────────────────────────────────╮
│ ✚ New prompt…            enter │   the draft form
│ ⚙ Session…                     │   … with the session panel up
│ ◫ Images…                      │   … with the attachments editor up
│ ✉ Send…            shift+enter │   unsaved, to an agent
│ ◷ Schedule…                    │   add (or reuse the copy), list's scheduler
│ ⤓ Add to backlog               │   one press, no form, stays on the page
│ ⤓ Add as 🍏 quick win          │   … with one mark (nextMenuAddMark)
│ ⤓ Add as △ high priority       │
│ ⤓ Add as ▲ critical priority   │
│ ⤓ Add as ｉ info               │
│ ⤓ Add as ⚑ flagged             │
│ ⧉ Copy ID: N-014               │
│ ⧉ Copy as prompt               │
╰────────────────────────────────╯
```

- **`addNextItem`** saves the prompt the draft form would have opened with
  (`nextItemTitle` / `nextItemPrompt`). The marks go on through
  `annots.applyTo`, the path `saveForm` uses. It appends to the file and
  rebuilds the list behind the page. The heading says
  `added N-014 as a quick win to the project backlog`, using
  `strings.ToLower(scope.String())`, since `String()` is capitalized.
- **Duplicates:** `nextBacklogCopy` looks for an open (not done) prompt in
  the target backlog that starts with `nextItemCite(id)`
  (`"Next list item N-014 ("`). `nextItemPrompt` now builds its prompt from
  that same helper. A match greys every Add row, and the row names the copy.
  ◷ Schedule… reuses the copy instead of adding another. A done copy doesn't
  block a re-add.
- **Schedule** sets `listFocus` to the prompt, runs `backToList`, then
  `beginSchedule`. It is the one row that leaves the page, because the
  scheduler belongs to the list.
- **Target backlog** is `nextAddScope`: project if available, else global.
  That matches `beginAddWith`.
- **Dim rows:** with no writable backlog, the draft rows and the Add rows are
  dim. Send is dim with no socket or while a drop is in flight. Schedule is
  dim with no socket.

## The draft form carries the item

- **Title:** a `formNextID` field is set by `promptFromNextItem` and cleared
  by `beginAddWith` and `backToList`. When it is set, `viewForm` titles the
  editor `Next List Prompt Editor`.
- **Value:** the item's value is now copied into `formAnnots.Value`. Before
  this, every draft opened at low, whatever the file said. The user's N-005
  showed low only because it *is* low. After setting the value, autosave's
  baseline is re-taken (`nm.autosave.saved = nm.formSig()`), so the carried
  value doesn't count as an edit and arm a save.

## Slip

While rewriting the README's menu section, I spliced from
`**Right-click an item**` to `## The list's context menu`. Those two aren't
adjacent, so the splice deleted *Sending to a machine on the local network*
(and its subsection) and *The hover card*. I caught it from the diff stat
(−118) and rebuilt README.md from `HEAD` with only this session's two
changes: the editor-title sentence and the new section. The final diff
changes one existing line.

## Tests

- `nextmenu_test.go`: opens on the right button, only over an item, Send
  dims without a socket, each row runs, the menu owns the keys (ctrl+r
  doesn't refresh from behind it), a click off dismisses without acting, it
  closes with its page, it refuses the hover card, it opens the form's panels
  over the draft, each Add row saves with its mark and the value and then
  dims, Add works again after the copy is done, and Schedule adds, then
  reuses the copy.
- `nextlist_test.go` `TestNextListDraftCarriesTheItem`: the title, the
  carried value, the autosave baseline, and that a later plain add is titled
  `Prompt Editor` again.
- `go test ./...` passes.

## Docs

The README has a new part at the end of **The Next List**, and the
title-line sentence names the `Next List Prompt Editor` variant. The dev
skill's file map has a `nextmenu.go` row, and its key-chord line mentions the
right-click.

Not released. v0.38.0 would be a minor bump, since this is a new capability.

## Next

Closed: None. Declined: None. Raised: N-046, N-047.
Deferred: None. Promoted: None.
Updated: N-040. Full list: `ai_docs/todo/next-list.md`.
