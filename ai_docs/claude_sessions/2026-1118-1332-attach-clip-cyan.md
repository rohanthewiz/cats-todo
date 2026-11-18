# Attachment clip turns cyan when an image is attached

Session: `dcae79ff-181d-4e20-b29f-8d92fb799862`

## Ask

At the bottom of the prompt editor, the paper-clip icon on the attachment line
(`📎 no images`) should be cyan when an image is attached.

## Finding

The code already rendered the 📎 with `attachStyle` (cyan), all the time, yet it
showed up grey. cats renders through libghostty, which draws 📎 (U+1F4CE) from
Apple Color Emoji as a color bitmap. Foreground color has no effect on that
bitmap, and Apple's clip is grey. A VS15 text-presentation selector would not
help: no regular macOS font has U+1F4CE, so libghostty's `CodepointResolver`
falls back to the emoji face anyway. Any clip that can be tinted therefore has
to be a different glyph.

The user was asked which glyph to use and chose the Nerd Font paperclip on
**both** the editor line and the list's clip-and-count mark.

## Changes

- `styles.go`: new `attachGlyph = ""` (nf-fa-paperclip), with the
  rationale. It is an outline glyph, so it takes the foreground color, and
  libghostty embeds Symbols Nerd Font as its last fallback, so it always
  resolves in cats. The cost: outside cats, a terminal without a Nerd Font
  shows a missing-glyph box. It is one cell wide where 📎 was two.
- `ui.go` `viewForm`: the editor's attachment-line clip is `descStyle` grey with
  no images and `attachStyle` cyan with one or more. The count text stays grey.
- `ui.go` `rebuildList`: the list row's mark is `attachGlyph + N`. Its colors are
  unchanged: cyan on open rows, grey on done or frozen rows.
- `ui_test.go`: `TestRowDescMarks` expects the new glyph.
- `formimages_test.go`: new `TestFormClipLightsOnlyWithImages` checks grey
  when empty and cyan with an image. It guards against grey and cyan
  rendering identically, which would make the check pass vacuously.
- `README.md`: the attachments paragraph describes the clip on both screens and
  why it is not the emoji.

The dimmed `📎 n attached:` heading of the `ctrl+v` file list is left as the
emoji. It is meant to be grey, so the emoji's grey is harmless there.

`go build`, `go vet` and `go test ./...` pass. Nothing has been seen live in
cats yet (N-098).

## Next

Closed: None. Declined: None. Raised: N-098.
Deferred: None. Promoted: None. Moved: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
