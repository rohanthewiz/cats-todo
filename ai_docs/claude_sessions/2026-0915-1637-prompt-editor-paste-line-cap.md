# Prompt editor: no line cap, so enter works after a large paste — v0.31.5

Session: `b284ba89-b9cb-4e00-80d2-e0df4d7fa87f`
Date: 2026-09-15

## Ask

"In the Prompt Editor, I am unable to type after a large paste"

Then: "agreed to the XCode license. Go ahead and to a wrap while bumping the
release."

## Diagnosis

The textarea's settings in `newFormInputs` (`ui.go`) set `CharLimit = 0` but
left `MaxHeight` at the bubbles v2.1.1 default of **99**. In that library,
`MaxHeight` does two jobs:

```
MaxHeight (default 99)
  ├─ SetHeight clamps the viewport to it          (harmless: we size it ourselves)
  └─ atContentLimit(): len(value) >= MaxHeight    (when MaxContentHeight == 0)
        └─ InsertNewline refuses the key           ← the bug
```

`insertRunesFromUserInput`, which a `tea.PasteMsg` goes through, only checks
`CharLimit`, the hard `maxLines` (10000) and `MaxContentHeight`. It never checks
`MaxHeight`. So a 100+ line paste went in whole, and afterwards every enter was
refused with no message. Letters still went in, so the editor looked half stuck.

A throwaway repro test (paste, then `x`, enter, `y`, timing each key plus
`View()`) confirmed it:

- 150 lines: `x` lands, **enter does not**, `y` lands. About 5ms per key.
- 2000 lines, spell check on: same pattern, about 55ms per key. Slow-ish, not
  a freeze.
- 20k chars on one line: everything lands; enter took about 147ms.

Performance was ruled out as the cause.

A second effect of the default: `Update` shrinks the wrap memo cache to
`MaxHeight` entries whenever `MaxHeight > 0`. So on any prompt past 99 rows, the
extra rows were re-wrapped on every keystroke.

## What shipped

- `ui.go` `newFormInputs`: `ta.MaxHeight = 0`, with a comment on both jobs the
  field does and why neither is wanted. The editor's height is still set from
  the pane (here and in the resize handler, `formChromeHeight`). Line numbers
  are off, so the `numDigits(MaxHeight)` paths in the library don't matter. The
  library's `maxLines` of 10000 still bounds a paste.
- `promptindent.go` `newlineCarryingIndent`: the mirrored `MaxHeight` guard
  stays in place. Its comment now says it cannot trip while the form builds the
  editor with 0, and that it is kept so the two agree if a cap ever returns.
- `promptlimit_test.go`: `TestPromptTypingAfterLargePaste` pastes 150 lines,
  then types `x`, enter, `y`, and expects `paste + "x\ny"`. With `MaxHeight = 99`
  it fails as `value ends "xy"`; with the fix it passes.

`go test ./...` is green. Not exercised by hand in a live cats pane.

No README change: the README never promised or mentioned a line limit.

## Release

A patch, because this fixes a surprise in something already shipped.

- `b779164 fix(form): no line cap in the prompt editor, so enter works after a large paste`
- `chore(release): v0.31.5` (`main.go` + `cats-plugin.toml`)
- Annotated tag `v0.31.5 — enter works in the prompt editor after a paste of 100+ lines`.
  Pushed `main` and the tag.

Housekeeping: git was unusable at first because the Xcode license had not been
accepted (`exit 69`). The user accepted it mid-session.

## Next

- Hand-test in cats: paste a few hundred lines, then type, press enter, scroll,
  click and sweep. Watch whether the caret stays in view on the long prompt.
- Enter on a 20k-char single line took about 147ms (`replacePromptRunes` does a
  `SetValue` on the whole prompt). If big one-line pastes are common, consider
  a cheaper edit path there.
- Pastes past the library's 10000-line `maxLines` are still truncated silently.
  Contract 4 ("refuse in words") suggests a status note if that is ever hit.
- Carried (from 2026-09-13): live-test existing-pane drops with model and effort
  set; `/model` mid-conversation confirm modals; efforts a model rejects; gate
  `/clear` on a detected agent; done stamp in bundle export and hover card; the
  Cmd+V pasteboard-into-carets test; enter inside an indent; column-mode footer
  width; back-tagging older untagged releases (ask first).
