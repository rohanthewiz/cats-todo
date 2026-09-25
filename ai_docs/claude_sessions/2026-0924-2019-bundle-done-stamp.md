# The done stamp in the bundle's markdown

Session: `71d0d416-a283-4979-9387-918c586bcb1c`
Date: 2026-09-24

## Ask

N-013, pasted from the Next List: the markdown export of a bundle
(`bundle.go`) does not show when a done prompt was finished, so consider
adding the `doneAt` stamp. Then `/sw`.

This continues the session that wrote `2026-0924-2013-form-footer-tail`.

## The stamp

The bundle's JSON already carried `doneAt`, because each element of `todos`
is a `Todo` marshalled by the todos.json rules. Only the markdown rendering
(`renderBundleMarkdown` → `bundleTodoNote`) left it out. That markdown is the
email body and the `export --markdown` output, so it is all a reader without
cats-todo sees.

`bundleTodoNote`'s done case now writes the prompt view's full stamp:

```
done 2026-09-24 14:05 CDT
```

- **The zone is kept.** The text crosses machines, so "14:05" alone would not
  tell a reader in another zone which day it was.
- **No stamp, plain `done`.** A todo finished before `DoneAt` existed has
  none, so it reads as the list row does.

## A bug found along the way

The new test's exact-match wants failed with `done · none priority`. Since
`5e14beb` (priority re-cut to none/high/critical), `priorityLabel("")`
returns `"none"` for the annotation panel. `bundleTodoNote` tested that label
for `""`, so every exported prompt without a priority carried
" · none priority". The old `TestRenderBundleMarkdown` only did substring
checks, so it never noticed.

The fix tests the level (`t.Priority != priorityNone`) rather than the label.
`priorityLabel` is unchanged because the panel wants the word. I checked
`valueLevelLabel` for the same trap: it already returns `""` for low, so the
value line is fine.

## Tests

`TestRenderBundleMarkdownStampsDone` checks four notes with exact matches:

| Todo | Note |
|---|---|
| done, with a stamp | `done <date> <time> <zone>` |
| done, no stamp | `done` |
| ordinary open prompt | no line at all |
| critical priority | `critical priority` |

`go vet` and `go test ./...` pass.

## Docs

- The README's export section has a new paragraph on the markdown's one line
  per prompt and the done stamp.
- N-013 has moved to Closed in `ai_docs/todo/next-list.md`, with the priority
  bug noted.

Not released. This and the footer change are both fixes or refinements, so
they would ship together as a patch.

## Next

Closed: N-013. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: None. Full list: `ai_docs/todo/next-list.md`.
