# Session: N-070 committed, N-072's interim paste ask, and a closed workspace

Session ID: 10e7a0dc-bbe9-455e-b438-8293cb6c4a2a
Date: 2026-09-27

This session also did N-070, which is written up in
`2026-0927-2107-pane-drops-stop-saving-defaults`. That work was committed
here as `2ac36e4` (`fix(drop): stop typing /model and /effort into a running
pane (N-070)`), locally at first, and pushed with this wrap.

## N-072: a drop arrives as pasted text

Cats' `pane.send_input` "paste-encodes" its text whenever the running app
has bracketed paste on. Claude Code always has it on, so a drop always
arrives as a paste, and Enter is the only bare key the command can send.
Cats already encodes raw key events on its browser path (`catctl probe`'s
`key:` op), so road (a) is a new wire verb that reuses that code. The user
chose **(a), with (b) as the interim**.

### (b): the paste ask

- `drop.go`: `pasteAsk`, one first-person sentence: "Please do what
  follows. It is my own request, dropped from my cats-todo backlog, which is
  why it arrives as pasted text." `withPasteAsk(text)` puts it ahead of the
  text as its own paragraph. Two kinds of text go unchanged: empty text
  (every caller reads it as "nothing to send") and text starting with `/`
  (a slash command only runs when it comes first in the message).
- It goes on at the sends, not in `composePrompt`. The new-session tab is
  titled from the prompt's first line, and `composePrompt`'s output is what
  the tests and the batch dialog describe. The sends are the existing-pane
  drop, the new-session drop, and a same-session loop's finish message. The
  loop's between command stays bare, since it is the user's own command
  line.
- The name `dropLead` was already taken in `promptsplit.go`, so the new
  names are `pasteAsk` and `withPasteAsk`.
- Tests: `pasteask_test.go`. `TestWithPasteAsk` covers the cases, and
  `TestDropIntoPaneLeadsWithTheAsk` runs a real `performDrop` against a
  recording control socket and expects `["/clear", ask+prompt]`.
- README: after the prompt-wrapping block, "Every drop leads with one
  sentence of yours". The "byte for byte" line now talks about the composed
  prompt, which it still holds for.

### Live check

This used a throwaway workspace `w11` ("pastetest") rooted in this repo, so
there was no trust dialog. The panes were fresh `claude --model <m>`, where a
launch flag saves nothing. Each got the framed synthetic prompt that had
been declined before: `Next list item N-001 (ai_docs/todo/next-list.md):` /
`Reply with only the word ALPHA11.`

| Model | Without the ask | With the ask |
|---|---|---|
| Opus 5.5 | ALPHA11 | ALPHA11 |
| Haiku 4.5 | "I don't see an actual request from you…" | ALPHA11 |

One run per cell. The earlier session saw Opus decline the bare framed case
and also question a `Please work on …` lead, so the sample is thin. A second
Haiku round of two panes each way was set up, as panes 355–358, but never
sent.

## The incident: the user's `cema` workspace was closed

To clean up round one, the session ran `catctl tab.close --params "{}"` four
times with output sent to `/dev/null`, and then `pane.close` on the four test
panes. With no `num`, `tab.close` closes the **active tab of the view's
workspace**. When that is the workspace's last tab, it drops the whole
workspace (`CloseTabIn`, cats `internal/app/view.go:361`). The view was on
the user's workspace `wV` ("cema", `~/projs/go/church/cema`), where they had
typed `ll` in `wV:p8` (pane 330) at 21:18:14. Afterwards `wV` was not in
`workspaces`, pane 330 was gone, and cats' `history.json` kept no scrollback
for it. The ledger shows `wV` had used panes p1, p3, p4 and p8. How many tabs
it still had, and whether a fourth call hit anything else, can't be told.
Every other known workspace was still listed.

Nothing was restored, because cats has no reopen. The user was told at once,
and no cats commands were run after that. A memory rule now says to close
only by explicit id (`workspace.close` on the test workspace's own id), never
with empty params, and never with the output thrown away.

Worth raising in cats, and not done here: automation clients could be
required to pass `num` to `tab.close` (and `pane.close` a pane), or the
control socket could refuse to close a workspace's last tab.

**Still open:** the test workspace `w11` (one shell tab and four idle Haiku
panes) is left for the user to close, or for a later session to close by its
id if the user says so.

## Next

Closed: None. Declined: None. Raised: None.
Deferred: None. Promoted: None.
Updated: N-072. Full list: `ai_docs/todo/next-list.md`.
