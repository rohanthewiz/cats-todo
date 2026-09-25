// batches.go — the Batches page: every batch this backlog has sent, and what
// became of each prompt in it.
//
//	Batches  2 project · 1 global
//
//	╭ 🔍 query ───────────────────╮  3/3
//
//	  ＋ New ctrl+a  ⧉ Duplicate ctrl+d  ✖ Delete ctrl+x  ← Back esc
//
//	❯ ▶ refactor trio      3 prompts · all at once · new worktree · Thu 14:02 · 1/3 so far
//	  ✓ quick wins         5 prompts · one prompt, listed · Wed 18:40 · 5/5
//	  ⚠ release prep       2 prompts · all at once · Wed 09:00 · 1/2
//
// A batch is a record, not a queue: once dropped there is nothing left to do to
// it but read it, run it again, or throw the record away. So the page is those
// three, and enter opens the record (stageBatchView) — which prompt went where,
// and why any did not.
//
// The rows read both batches.json files, the project's and the global one, and
// are drawn newest first with a running batch on top. The files themselves keep
// creation order; the page sorts, the file is never rewritten to match it.
package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// batchesPage is the open page.
type batchesPage struct {
	list    fuzzyList
	rows    []Batch // a listItem's ref indexes this
	note    string
	noteErr bool
	// armDelete is the batch a first ctrl+x armed. The second press deletes;
	// anything else disarms. A history record is cheap to lose but not free,
	// and one keystroke on the wrong row should not be the whole cost.
	armDelete string
	// view is the batch the record screen (stageBatchView) is showing.
	view Batch
}

// The page's buttons, in order.
const (
	batchesBtnNew = iota
	batchesBtnDup
	batchesBtnDelete
	batchesBtnBack
)

// beginBatches is ctrl+k on the list. With prompts selected (ctrl+space) it
// goes straight to a composer holding them — "batch these" is what a
// selection followed by the batch chord means — and otherwise to the page.
func (m model) beginBatches() (tea.Model, tea.Cmd) {
	return m.beginBatchesWith(todoRef{})
}

// beginBatchesWith is beginBatches with one more prompt in the batch: the row
// the list's context menu was opened on, which belongs in the batch whether or
// not it is ticked. It is added to the preset rather than to the selection, so
// abandoning the composer leaves the list exactly as it was.
func (m model) beginBatchesWith(extra todoRef) (tea.Model, tea.Cmd) {
	n := m.markCount()
	if extra != (todoRef{}) && !m.marked[extra] {
		n++
	}
	if n > 0 {
		var preset []batchCand
		for _, s := range []*store{m.project, m.global} {
			for _, t := range s.todos {
				ref := todoRef{scope: s.scope, id: t.ID}
				if (m.marked[ref] || ref == extra) && !t.closed() && !t.Info {
					preset = append(preset, candFromTodo(ref, t))
				}
			}
		}
		next, cmd := m.beginBatchCompose(stageList, batchSrcBacklog, preset)
		if nm, ok := next.(model); ok && len(preset) < n {
			nm.batchSay(fmt.Sprintf("%d of the %d selected are closed or info notes and were left out", n-len(preset), n), false)
			return nm, cmd
		}
		return next, cmd
	}
	m.openBatchesPage()
	return m, nil
}

// candFromTodo is a backlog prompt as a composer pick.
func candFromTodo(ref todoRef, t Todo) batchCand {
	return batchCand{
		ref:     ref,
		title:   firstNonEmpty(t.Title, firstLine(t.Prompt, 60)),
		session: t.Session,
		marks:   annotMarksFor(t),
	}
}

// openBatchesPage shows the page, read fresh from both files.
func (m *model) openBatchesPage() {
	m.clearHover()
	m.stage = stageBatches
	keep := ""
	if i := m.batches.list.selectedIndex(); i >= 0 && i < len(m.batches.rows) {
		keep = m.batches.rows[i].ID
	}
	note, noteErr := m.batches.note, m.batches.noteErr
	m.batches = batchesPage{list: newFuzzyList("type to filter", nil), note: note, noteErr: noteErr}
	m.reloadBatches()
	for i, b := range m.batches.rows {
		if b.ID == keep {
			m.batches.list.selectRef(i)
		}
	}
}

// reloadBatches reads both files and rebuilds the rows.
func (m *model) reloadBatches() {
	var all []Batch
	var errs []string
	for _, s := range []*store{m.project, m.global} {
		bs := batchStoreFor(s)
		if err := bs.load(); err != nil {
			errs = append(errs, s.scope.String()+" batches: "+err.Error())
			continue
		}
		all = append(all, bs.batches...)
	}
	// Running first (it is the one still changing), then newest first.
	slices.SortStableFunc(all, func(a, b Batch) int {
		if (a.State == batchRunning) != (b.State == batchRunning) {
			if a.State == batchRunning {
				return -1
			}
			return 1
		}
		return b.batchTime().Compare(a.batchTime())
	})
	m.batches.rows = all
	if len(errs) > 0 {
		m.batches.say(strings.Join(errs, " · "), true)
	}
	m.sizeBatchesPage()
}

// batchTime is when a batch happened, for sorting and for its row: when it was
// dropped, or when it was made if it never was.
func (b Batch) batchTime() time.Time {
	if !b.Dropped.IsZero() {
		return b.Dropped
	}
	return b.Created
}

func (p *batchesPage) say(s string, isErr bool) { p.note, p.noteErr = s, isErr }

// rebuildBatchRows turns the records into rows.
func (m *model) rebuildBatchRows() {
	now := time.Now()
	var items []listItem
	for i, b := range m.batches.rows {
		glyph, st := batchStateMark(b)
		ok, total := b.deliveredCounts()
		desc := fmt.Sprintf("%d prompt%s · %s · %s · %s · %d/%d",
			total, plural(total), deliverLabel(b.Deliver),
			strings.TrimPrefix(firstNonEmpty(b.Target.Label, targetDesc(b.Target.dropTarget())), "＋ "),
			formatDoneTime(b.batchTime(), now), ok, total)
		if b.State == batchRunning {
			desc += " so far"
		}
		if b.scope == scopeGlobal && m.project.available() {
			desc += " · global"
		}
		// Cut to the pane, the Next List page's rule: a row that wrapped would
		// put every row under it a line below where the click's hit-test looks.
		// The name keeps up to 40 cells; the description gets what is left
		// after the cursor, the badge, the gaps and a scroll marker's room.
		name := truncate(b.displayName(), 40)
		if m.width > 0 {
			room := m.width - indentWidth - lipgloss.Width(glyph) - 1 - lipgloss.Width(name) - 2 - 8
			desc = truncate(desc, max(room, 4))
		}
		items = append(items, listItem{
			name:       name,
			badge:      glyph,
			badgeStyle: st,
			desc:       desc,
			selectable: true,
			ref:        i,
		})
	}
	m.batches.list.setItems(items)
}

// batchStateMark is a batch's badge: ▶ running, ✓ every prompt landed, ⚠ some
// did, ✗ none did.
func batchStateMark(b Batch) (string, lipgloss.Style) {
	if b.State == batchRunning {
		return "▶", schedStyle
	}
	ok, total := b.deliveredCounts()
	switch {
	case total > 0 && ok == total:
		return "✓", checkStyle
	case ok > 0:
		return "⚠", schedStyle
	}
	return "✗", errStyle
}

// Where the page's two hit-tested things are drawn: the bar and the first row
// (the Next List page's geometry — heading, blank, query, blank, bar, blank).
const (
	batchesBarRow  = 4
	batchesRowsRow = 6
)

// sizeBatchesPage fits the window to the pane, re-cutting the rows to its
// width.
func (m *model) sizeBatchesPage() {
	m.rebuildBatchRows()
	if w := m.width - 4; w >= 20 {
		m.batches.list.input.SetWidth(min(w, searchFieldWidth))
	}
	if m.height > 0 {
		m.batches.list.setMaxRows(max(m.height-batchesRowsRow-2, 1))
	}
}

// highlightedBatch is the record under the cursor.
func (m model) highlightedBatch() (Batch, bool) {
	i := m.batches.list.selectedIndex()
	if i < 0 || i >= len(m.batches.rows) {
		return Batch{}, false
	}
	return m.batches.rows[i], true
}

// batchesActions is the page's bar.
func (m model) batchesActions() []listAction {
	return []listAction{
		{label: "＋ New", hint: "ctrl+a", tint: colInfo},
		{label: "⧉ Duplicate", hint: "ctrl+d", tint: colCyan, needsSel: true},
		{label: "✖ Delete", hint: "ctrl+x", tint: colErr, needsSel: true},
		{label: "← Back", hint: "esc", tint: colStraw},
	}
}

func (m model) batchesChips() []actionChip {
	tier := barTier(m.batchesActions(), m.width, indentWidth)
	var chips []actionChip
	x := indentWidth
	for i, a := range m.batchesActions() {
		if i > 0 {
			x += chipGap(tier)
		}
		text := a.chipText(tier)
		w := lipgloss.Width(btnStyle.Render(text))
		chips = append(chips, actionChip{text: text, start: x, end: x + w})
		x += w
	}
	return chips
}

func (m model) batchesBar() string {
	acts := m.batchesActions()
	tier := barTier(acts, m.width, indentWidth)
	_, hasSel := m.highlightedBatch()
	gap := strings.Repeat(" ", chipGap(tier))
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", indentWidth))
	for i, c := range m.batchesChips() {
		if i > 0 {
			b.WriteString(gap)
		}
		st, hintFg := btnStyle.Foreground(lipgloss.Color(acts[i].tint)), colDim
		if acts[i].needsSel && !hasSel {
			st, hintFg = btnOffStyle, colFaint
		}
		b.WriteString(renderChipDimHint(st, hintFg, acts[i], tier, c.text))
	}
	return b.String()
}

// pressBatches runs the page's button i.
func (m model) pressBatches(i int) (tea.Model, tea.Cmd) {
	if i != batchesBtnDelete {
		m.batches.armDelete = ""
	}
	switch i {
	case batchesBtnNew:
		return m.beginBatchCompose(stageBatches, batchSrcBacklog, nil)
	case batchesBtnDup:
		return m.duplicateBatch()
	case batchesBtnDelete:
		return m.deleteBatch()
	case batchesBtnBack:
		m.backToList()
	}
	return m, nil
}

// duplicateBatch is ⧉ Duplicate: a composer holding the batch's settings and
// whichever of its prompts are still open — after a partial drop, exactly the
// ones that did not land, since the ones that did were marked done.
func (m model) duplicateBatch() (tea.Model, tea.Cmd) {
	b, ok := m.highlightedBatch()
	if !ok {
		m.batches.say("highlight a batch first — ↑/↓ to choose one", false)
		return m, nil
	}
	if m.stage == stageBatchView {
		b = m.batches.view
	}
	var preset []batchCand
	for _, it := range b.Items {
		ref := it.ref()
		if td, ok := m.resolve(ref); ok && !td.closed() && !td.Info {
			preset = append(preset, candFromTodo(ref, td))
		}
	}
	if len(preset) == 0 {
		m.batches.say("every prompt in "+b.displayName()+" is done or gone — reopen them on the list (ctrl+t) to run it again", false)
		m.stage = stageBatches
		return m, nil
	}
	next, cmd := m.beginBatchCompose(stageBatches, batchSrcBacklog, preset)
	nm, ok := next.(model)
	if !ok {
		return next, cmd
	}
	nm.batch.name.SetValue(b.Name)
	nm.batch.deliver = b.Deliver
	nm.batch.target = b.Target.dropTarget()
	nm.batch.session = sessionValue(b.Session)
	nm.rebuildBatchPick()
	if len(preset) < len(b.Items) {
		nm.batchSay(fmt.Sprintf("%d of %d prompts are still open and were picked; the rest are done or gone", len(preset), len(b.Items)), false)
	}
	return nm, cmd
}

// deleteBatch is ✖ Delete, on the second press.
func (m model) deleteBatch() (tea.Model, tea.Cmd) {
	b, ok := m.highlightedBatch()
	if !ok {
		m.batches.say("highlight a batch first — ↑/↓ to choose one", false)
		return m, nil
	}
	if b.State == batchRunning && m.batchRun != nil && m.batchRun.batch.ID == b.ID {
		m.batches.say("that batch is still being dropped — delete it once it has finished", true)
		return m, nil
	}
	if m.batches.armDelete != b.ID {
		m.batches.armDelete = b.ID
		m.batches.say("press ctrl+x again to delete the record of “"+truncate(b.displayName(), 40)+"” — its prompts are not touched", true)
		return m, nil
	}
	m.batches.armDelete = ""
	if err := m.batchStoreForScope(b.scope).delete(b.ID); err != nil {
		m.batches.say("delete failed: "+err.Error(), true)
		return m, nil
	}
	m.batches.say("deleted the record of “"+truncate(b.displayName(), 40)+"”", false)
	m.reloadBatches()
	return m, nil
}

func (m model) updateBatches(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := msg.String()
	if s != "ctrl+x" {
		m.batches.armDelete = ""
	}
	switch s {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		if strings.TrimSpace(m.batches.list.input.Value()) != "" {
			m.batches.list.input.SetValue("")
			m.batches.list.filter()
			return m, nil
		}
		m.backToList()
		return m, nil
	case "up", "ctrl+p":
		m.batches.list.moveUp()
		return m, nil
	case "down", "ctrl+n":
		m.batches.list.moveDown()
		return m, nil
	case "enter":
		return m.beginBatchView()
	case "ctrl+a":
		return m.pressBatches(batchesBtnNew)
	case "ctrl+d":
		return m.pressBatches(batchesBtnDup)
	case "ctrl+x":
		return m.pressBatches(batchesBtnDelete)
	}
	m.batches.say("", false)
	return m, m.batches.list.editQuery(msg)
}

// clickBatches is the pointer on the page: a chip presses, a row highlights,
// and a second click on the same row opens its record.
func (m model) clickBatches(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Y == batchesBarRow {
		for i, c := range m.batchesChips() {
			if msg.X >= c.start && msg.X < c.end {
				return m.pressBatches(i)
			}
		}
		return m, nil
	}
	i, ok := m.batches.list.rowAtLine(msg.Y - batchesRowsRow)
	if !ok || !m.batches.list.focusRow(i) {
		return m, nil
	}
	if i == m.lastClickRow && time.Since(m.lastClickAt) < doubleClickWindow {
		m.lastClickAt = time.Time{}
		return m.beginBatchView()
	}
	m.lastClickRow, m.lastClickAt = i, time.Now()
	return m, nil
}

func (m model) viewBatches() string {
	var b strings.Builder
	const title = "Batches"
	b.WriteString(titleStyle.Render(title))
	b.WriteString("  ")
	noun := "batches"
	if len(m.batches.rows) == 1 {
		noun = "batch"
	}
	side, style := fmt.Sprintf("%d %s", len(m.batches.rows), noun), descStyle
	if m.batches.note != "" {
		side, style = m.batches.note, okStyle
		if m.batches.noteErr {
			style = errStyle
		}
	}
	if m.width > 0 {
		if room := m.width - lipgloss.Width(titleStyle.Render(title)) - 4; room > 1 {
			side = truncate(side, room)
		}
	}
	b.WriteString(style.Render(side))
	b.WriteString("\n\n")
	b.WriteString(m.batches.list.view("no batches yet — ＋ New (ctrl+a) to make one", m.batchesBar(), m.width))
	b.WriteString("\n")
	b.WriteString(footerStyle.Render(m.fitFooter([]string{"enter open", "↑/↓ choose", "type to filter", "dbl-click open"})))
	return b.String()
}

// --- One batch's record ------------------------------------------------------------

// beginBatchView opens the highlighted batch's record.
func (m model) beginBatchView() (tea.Model, tea.Cmd) {
	b, ok := m.highlightedBatch()
	if !ok {
		m.batches.say("highlight a batch first — ↑/↓ to choose one", false)
		return m, nil
	}
	m.batches.view = b
	m.stage = stageBatchView
	return m, nil
}

func (m model) updateBatchView(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "enter", "q":
		m.stage = stageBatches
		return m, nil
	case "ctrl+d":
		return m.duplicateBatch()
	}
	return m, nil
}

// viewBatchView draws one batch's record: how it went out, then each prompt
// with where it landed or why it did not.
func (m model) viewBatchView() string {
	b := m.batches.view
	now := time.Now()
	var out []string
	glyph, st := batchStateMark(b)
	out = append(out, titleStyle.Render("Batch")+"  "+st.Render(glyph)+" "+headerNameStyle.Render(b.displayName()), "")
	ok, total := b.deliveredCounts()
	field := func(label, value string) string {
		return "  " + nameStyle.Render(fmt.Sprintf("%-10s", label)) + descStyle.Render(value)
	}
	when := "not dropped"
	if !b.Dropped.IsZero() {
		when = formatDoneTime(b.Dropped, now)
	}
	out = append(out,
		field("Dropped", when),
		field("Delivered", fmt.Sprintf("%d of %d", ok, total)),
		field("Deliver", deliverLabel(b.Deliver)),
		field("Target", firstNonEmpty(b.Target.Label, targetDesc(b.Target.dropTarget()))),
		field("Session", firstNonEmpty(b.Session.summary(), "each prompt's own")),
		"",
		headingStyle.Render("Prompts"),
	)
	for i, it := range b.Items {
		line := fmt.Sprintf("%2d. %s", i+1, it.Title)
		run, ran := b.itemRun(i)
		switch {
		case !ran && b.State == batchRunning:
			out = append(out, "  "+descStyle.Render("· "+line+" — waiting"))
		case !ran:
			out = append(out, "  "+errStyle.Render("✗ ")+nameStyle.Render(line)+descStyle.Render(" — never sent"))
		case run.Err != "":
			out = append(out, "  "+errStyle.Render("✗ ")+nameStyle.Render(line)+errStyle.Render(" — "+run.Err))
		default:
			out = append(out, "  "+checkStyle.Render("✓ ")+nameStyle.Render(line)+descStyle.Render(" → "+run.Where+" · "+formatDoneTime(run.At, now)))
		}
	}
	// The lines are styled, so they are cut by cells with the escapes kept
	// intact rather than by runes (see fitToPane, which is for plain text).
	if m.width > 0 {
		for i := range out {
			out[i] = ansi.Truncate(out[i], m.width, "…")
		}
	}
	if m.height > 2 && len(out) > m.height-2 {
		out = out[:m.height-2]
	}
	out = append(out, "", footerStyle.Render(m.fitFooter([]string{"ctrl+d duplicate", "esc back"})))
	return strings.Join(out, "\n")
}
