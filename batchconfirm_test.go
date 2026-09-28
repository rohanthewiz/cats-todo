package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// composerWith opens the composer and picks titles, in that order.
func composerWith(t *testing.T, m model, titles ...string) model {
	t.Helper()
	m = openComposer(t, m)
	for _, title := range titles {
		m.toggleBatchCand(candIndex(t, m, title))
	}
	return m
}

// summaryText is the dialog's body as one string, for content assertions.
func summaryText(m model) string {
	var b strings.Builder
	for _, l := range m.batch.confirm.lines {
		b.WriteString(l.lead + l.text + "\n")
	}
	return b.String()
}

// noBatchesFile says nothing was written: the dialog asks, it does not act.
func noBatchesFile(t *testing.T, s *store) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(filepath.Dir(s.path), batchesFileName)); err == nil {
		t.Fatal("batches.json was written before the drop was confirmed")
	}
}

// TestDropNowAsksFirst: ▶ Drop now — the chord and the button — opens the
// dialog and sends nothing; esc (or n) goes back to the composer with the
// draft intact and says nothing was sent.
func TestDropNowAsksFirst(t *testing.T) {
	for _, open := range []string{"alt+enter", "button"} {
		for _, back := range []tea.KeyPressMsg{{Code: tea.KeyEscape}, {Code: 'n', Text: "n"}} {
			m, project, _ := batchModel(t, 120, 30)
			m.client = &catsClient{}
			m = composerWith(t, m, "Fix flaky drop test", "Rename headings")

			var next tea.Model
			var cmd tea.Cmd
			if open == "button" {
				next, cmd = m.pressBatchButton(batchBtnDrop)
			} else {
				next, cmd = m.updateBatchCompose(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})
			}
			m = next.(model)
			if cmd != nil || !m.batch.confirm.open || m.dropping || m.batchRun != nil {
				t.Fatalf("%s: cmd %v open %v dropping %v", open, cmd != nil, m.batch.confirm.open, m.dropping)
			}
			noBatchesFile(t, project)

			next, _ = m.updateBatchCompose(back)
			m = next.(model)
			if m.batch.confirm.open || m.stage != stageBatchCompose || len(m.batch.picked) != 2 {
				t.Errorf("%s then %s: open %v stage %v picks %d", open, back.String(), m.batch.confirm.open, m.stage, len(m.batch.picked))
			}
			if !strings.Contains(m.batch.note, "nothing was sent") {
				t.Errorf("%s then %s: note %q, want it to say nothing was sent", open, back.String(), m.batch.note)
			}
			noBatchesFile(t, project)
		}
	}
}

// TestDropNowRefusesBeforeAsking: a batch that cannot go says why at once, on
// the note line, rather than behind a confirmation it could never honour.
func TestDropNowRefusesBeforeAsking(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	m.client = &catsClient{}
	m = openComposer(t, m)
	next, _ := m.askDropBatch()
	m = next.(model)
	if m.batch.confirm.open || !strings.Contains(m.batch.note, "pick at least one prompt") {
		t.Errorf("no picks: open %v note %q", m.batch.confirm.open, m.batch.note)
	}
}

// TestDropConfirmSends: enter on the highlighted ▶ Drop now (and y) runs the
// ordinary drop: the record goes to disk as running and the first step is
// returned. The dialog is closed first.
func TestDropConfirmSends(t *testing.T) {
	for _, yes := range []tea.KeyPressMsg{{Code: tea.KeyEnter}, {Code: 'y', Text: "y"}, {Code: tea.KeyEnter, Mod: tea.ModShift}} {
		m, project, _ := batchModel(t, 120, 30)
		m.client = &catsClient{}
		m = composerWith(t, m, "Fix flaky drop test", "Rename headings")
		m.batch.deliver = deliverEach
		next, _ := m.askDropBatch()
		m = next.(model)
		next, cmd := m.updateBatchCompose(yes)
		m = next.(model)
		if cmd == nil || !m.dropping || m.batchRun == nil || m.batch.confirm.open {
			t.Fatalf("%s: cmd %v dropping %v run %v open %v", yes.String(), cmd != nil, m.dropping, m.batchRun != nil, m.batch.confirm.open)
		}
		bs := batchStoreFor(project)
		if err := bs.load(); err != nil || len(bs.batches) != 1 || bs.batches[0].State != batchRunning {
			t.Errorf("%s: records %+v (%v), want one running", yes.String(), bs.batches, err)
		}
	}
}

// TestDropConfirmBackButton: the highlight moves with ←/→, and enter on
// ✕ Back goes back without sending.
func TestDropConfirmBackButton(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	m.client = &catsClient{}
	m = composerWith(t, m, "Fix flaky drop test")
	next, _ := m.askDropBatch()
	m = next.(model)
	next, _ = m.updateBatchCompose(tea.KeyPressMsg{Code: tea.KeyRight})
	m = next.(model)
	if m.batch.confirm.btn != dropConfirmBtnBack {
		t.Fatalf("→: btn %d, want Back", m.batch.confirm.btn)
	}
	next, cmd := m.updateBatchCompose(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if cmd != nil || m.batch.confirm.open || m.dropping {
		t.Errorf("enter on Back: cmd %v open %v dropping %v", cmd != nil, m.batch.confirm.open, m.dropping)
	}
	noBatchesFile(t, project)
}

// TestDropConfirmSummaryAllAtOnce: all at once onto new sessions names how
// many sessions open, where, and each prompt's launch flags — the batch's
// model laid over the prompt's own options, as overlaySession does.
func TestDropConfirmSummaryAllAtOnce(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	m.client = &catsClient{}
	if err := project.setSession(project.todos[0].ID, &SessionOpts{Effort: "high", Finish: finishPush}); err != nil {
		t.Fatal(err)
	}
	m = composerWith(t, m, "Fix flaky drop test", "Rename headings")
	m.batch.deliver = deliverEach
	m.batch.target.worktree = true
	m.batch.session = SessionOpts{Model: "sonnet"}
	next, _ := m.askDropBatch()
	m = next.(model)

	got := summaryText(m)
	for _, want := range []string{
		"Opens 2 new Claude Code sessions on a new worktree each",
		"1. Fix flaky drop test",
		"starts claude --model sonnet --effort high",
		"then: commit and push",
		"2. Rename headings",
		"starts claude --model sonnet",
		"A record of the batch is written before anything is sent",
		"marked done once it is delivered",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "one at a time") {
		t.Errorf("all at once described as a loop:\n%s", got)
	}
}

// TestDropConfirmSummaryLoop: a same-session loop says the later prompts go
// into the first one's session, what each is sent first there, the between
// command and its last-step rule, the failure rule, and the one wrap-up the
// prompts' own finish steps were lifted into (loopAction / loopFinishText).
func TestDropConfirmSummaryLoop(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	m.client = &catsClient{}
	if err := project.setSession(project.todos[1].ID, &SessionOpts{Finish: finishWrap}); err != nil {
		t.Fatal(err)
	}
	m = composerWith(t, m, "Fix flaky drop test", "Rename headings")
	m.batch.deliver = deliverLoop
	m.batch.session = SessionOpts{Model: "sonnet", Clear: true}
	m.batch.between.SetValue("/compact")
	m.batch.maxWait.SetValue("2h")
	m.batch.onFailSkip = true
	next, _ := m.askDropBatch()
	m = next.(model)

	got := summaryText(m)
	for _, want := range []string{
		"The first opens a new Claude Code session",
		"every later one is typed into that same session",
		"starts claude --model sonnet",
		// N-070: /model is no longer typed into a running pane (it would
		// save the model as the user's default), so the loop's later step
		// submits only /clear and says the model is left alone.
		"first submits /clear · model not applied, the running pane keeps its own",
		"submits “/compact” to that same session",
		"but not after the last prompt",
		"after 2h counts as failed",
		"moves on to the next prompt",
		"one wrap-up message into the session: run /sess-wrap",
		"within 1h can take the loop over",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary lacks %q:\n%s", want, got)
		}
	}
	// The finish is lifted out of each prompt, so no prompt line promises it.
	for _, l := range m.batch.confirm.lines {
		if l.detail && strings.Contains(l.text, "sess-wrap") {
			t.Errorf("a prompt's own line carries the lifted finish: %q", l.text)
		}
	}
}

// TestDropConfirmSummaryCombined: one message, its name, and the prompts
// whose own options it will not apply.
func TestDropConfirmSummaryCombined(t *testing.T) {
	m, project, _ := batchModel(t, 120, 30)
	m.client = &catsClient{}
	if err := project.setSession(project.todos[0].ID, &SessionOpts{Effort: "low"}); err != nil {
		t.Fatal(err)
	}
	m = composerWith(t, m, "Fix flaky drop test", "Rename headings")
	m.batch.deliver = deliverCombined
	m.batch.name.SetValue("nightly")
	next, _ := m.askDropBatch()
	m = next.(model)

	got := summaryText(m)
	for _, want := range []string{
		"Sends ONE message",
		"lists all 2 prompts as numbered sections",
		"Its tab is named “nightly”",
		"One of these prompts has options of its own",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary lacks %q:\n%s", want, got)
		}
	}
	if !strings.Contains(m.batch.confirm.title, "“nightly”") {
		t.Errorf("title %q does not name the batch", m.batch.confirm.title)
	}
}

// TestDropConfirmDrawsInsideThePane: at every width the dialog stays within
// the pane, and a click on ✕ Back — located by the same geometry the frame
// is drawn from — goes back; a click off the box does too.
func TestDropConfirmDrawsInsideThePane(t *testing.T) {
	for _, w := range []int{40, 80, 120, 200} {
		m, _, _ := batchModel(t, w, 30)
		m.client = &catsClient{}
		m = composerWith(t, m, "Fix flaky drop test", "Rename headings", "Add cleanup command")
		next, _ := m.askDropBatch()
		m = next.(model)

		frame := m.renderStage()
		for i, l := range strings.Split(frame, "\n") {
			if cw := ansi.StringWidth(l); cw > w {
				t.Errorf("width %d: row %d is %d cells", w, i, cw)
			}
		}
		if !strings.Contains(frame, "✕ Back") || !strings.Contains(frame, "▶ Drop now") {
			t.Errorf("width %d: the buttons are not drawn", w)
		}

		g := m.dropConfirmGeom()
		sp := g.btnSpans[dropConfirmBtnBack]
		next, cmd := m.clickBatchCompose(tea.MouseClickMsg{X: sp[0] + 1, Y: g.btnY, Button: tea.MouseLeft})
		nm := next.(model)
		if cmd != nil || nm.batch.confirm.open {
			t.Errorf("width %d: click on Back left it open (cmd %v)", w, cmd != nil)
		}
		if g.y > 0 {
			next, _ = m.clickBatchCompose(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
			if next.(model).batch.confirm.open {
				t.Errorf("width %d: a click off the box left it open", w)
			}
		}
	}
}

// TestDropConfirmScrolls: a summary taller than the pane scrolls with ↓ and
// never past its end, and the hint says where the view is.
func TestDropConfirmScrolls(t *testing.T) {
	m, _, _ := batchModel(t, 60, 16)
	m.client = &catsClient{}
	m = composerWith(t, m, "Fix flaky drop test", "Rename headings", "Add cleanup command")
	next, _ := m.askDropBatch()
	m = next.(model)
	g := m.dropConfirmGeom()
	total := len(m.dropConfirmBody(g.textW))
	if total <= g.bodyH {
		t.Fatalf("body %d rows fits in %d; the test needs a short pane", total, g.bodyH)
	}
	if !strings.Contains(m.renderStage(), "scroll") {
		t.Error("an overflowing body does not offer to scroll")
	}
	for range total + 5 {
		next, _ = m.updateBatchCompose(tea.KeyPressMsg{Code: tea.KeyDown})
		m = next.(model)
	}
	if m.batch.confirm.top != total-g.bodyH {
		t.Errorf("top %d after scrolling past the end, want %d", m.batch.confirm.top, total-g.bodyH)
	}
}

// TestDropConfirmHoldsThePaste: a paste while the dialog is up does not reach
// the draft behind it — the dialog is describing that draft.
func TestDropConfirmHoldsThePaste(t *testing.T) {
	m, _, _ := batchModel(t, 120, 30)
	m.client = &catsClient{}
	m = composerWith(t, m, "Fix flaky drop test")
	m.batch.setRow = batchSetName
	m.setBatchFocus(batchFocusSettings)
	next, _ := m.askDropBatch()
	m = next.(model)
	next, _ = m.Update(tea.PasteMsg{Content: "sneaky"})
	m = next.(model)
	if v := m.batch.name.Value(); v != "" {
		t.Errorf("name = %q after a paste under the dialog", v)
	}
}
