package main

import (
	"fmt"
	"math"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// prioModel adds one todo per priority (plus the closed and scheduled cases the
// caller asks for) and returns the built model.
func prioModel(t *testing.T, tds ...Todo) model {
	t.Helper()
	m, project, _ := newModelInTemp(t)
	for _, td := range tds {
		if err := project.add(td); err != nil {
			t.Fatal(err)
		}
	}
	m.width = 200
	m.rebuildList()
	return m
}

// rowNamed finds the built row with this title, so a test can ask about one
// row's marks without walking the whole list.
func rowNamed(t *testing.T, m model, name string) listItem {
	t.Helper()
	for _, it := range m.list.items {
		if it.selectable && it.name == name {
			return it
		}
	}
	t.Fatalf("no row named %q", name)
	return listItem{}
}

// keptSlots recomputes which annotation columns survived the trim, the same way
// rebuildList decided: a column stays when any visible row fills it. Derived
// rather than assumed, so these tests keep pointing at the right column as slots
// are added — and so they fail if the trim ever disagrees with its own rule.
func keptSlots(m model) []int {
	used := make([]bool, len(annotSlots))
	for _, st := range []*store{m.project, m.global} {
		for _, td := range st.todos {
			if m.folded(td) {
				continue
			}
			for i, sl := range annotSlots {
				if glyph, _, _ := sl.mark(td); glyph != "" {
					used[i] = true
				}
			}
		}
	}
	var kept []int
	for i, u := range used {
		if u {
			kept = append(kept, i)
		}
	}
	return kept
}

// annotMarkFor is the column a named slot occupies on a named row, or a blank
// mark when the trim dropped that column list-wide — which is the same answer
// the row gives the reader.
func annotMarkFor(t *testing.T, m model, name, slot string) annotMark {
	t.Helper()
	it := rowNamed(t, m, name)
	for pos, i := range keptSlots(m) {
		if annotSlots[i].name != slot {
			continue
		}
		if pos >= len(it.annots) {
			t.Fatalf("row %q has %d annotation columns, want the %s column at %d",
				name, len(it.annots), slot, pos)
		}
		return it.annots[pos]
	}
	return annotMark{}
}

// prioMark is the priority column of a row.
func prioMark(t *testing.T, m model, name string) annotMark {
	t.Helper()
	return annotMarkFor(t, m, name, "priority")
}

// TestOnlyRaisedRowsCarryAPriorityMark pins the rule the column changed to.
// Standard used to draw a dot on every row, which meant the column could not be
// scanned for the rows that actually wanted attention — every row looked the
// same. None draws nothing now, so the marks that are there are the answer.
func TestOnlyRaisedRowsCarryAPriorityMark(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "open", Title: "open none", Prompt: "p"},
		Todo{ID: "crit", Title: "open critical", Prompt: "p", Priority: priorityCritical},
		Todo{ID: "high", Title: "open high", Prompt: "p", Priority: priorityHigh},
		Todo{ID: "old", Title: "retired low", Prompt: "p", Priority: "low"},
		Todo{ID: "frozen", Title: "frozen", Prompt: "p", Frozen: true},
		Todo{ID: "done", Title: "done", Prompt: "p", Done: true},
	)
	want := map[string]string{
		"open none":     "",
		"open critical": prioCriticalGlyph,
		"open high":     prioHighGlyph,
		// A backlog written by the old scheme draws nothing, which is what
		// "low" always meant: not raised.
		"retired low": "",
		"frozen":      "",
		"done":        "",
	}
	for name, glyph := range want {
		if got := prioMark(t, m, name).text; got != glyph {
			t.Errorf("row %q carries priority mark %q, want %q", name, got, glyph)
		}
	}
	// Group headings carry no columns at all.
	for _, it := range m.list.items {
		if !it.selectable && len(it.annots) != 0 {
			t.Errorf("the %q heading carries annotation columns", it.name)
		}
	}
}

// TestTheBadgeLeadsTheAnnotations pins the row's reading order: state first,
// then what is true about the prompt. The badge is what the list is grouped by,
// so it is what the eye arriving at a row wants before anything else.
func TestTheBadgeLeadsTheAnnotations(t *testing.T) {
	m := prioModel(t, Todo{ID: "a", Title: "a", Prompt: "p", Priority: priorityCritical, Fruit: true})
	m.list.filter()
	// Stripped of its styling, so the order being asserted is the order the
	// reader sees rather than the order the escape sequences happen to fall in.
	row := stripANSI(strings.Split(m.list.rowsView("", m.width), "\n")[0])
	want := []string{"○", prioCriticalGlyph, fruitGlyph, "a"}
	at := -1
	for _, seg := range want {
		i := strings.Index(row, seg)
		if i < 0 {
			t.Fatalf("row %q is missing %q", row, seg)
		}
		if i <= at {
			t.Fatalf("row %q has %q out of order — want %s in that order",
				row, seg, strings.Join(want, " then "))
		}
		at = i
	}
}

// TestUnusedAnnotationColumnsAreDropped pins what keeps the marks from costing
// every backlog that does not use them: a list with nothing annotated draws no
// annotation columns at all, and one that uses a single mark pays for one.
func TestUnusedAnnotationColumnsAreDropped(t *testing.T) {
	plain := prioModel(t, Todo{ID: "a", Title: "a", Prompt: "p"})
	if n := len(rowNamed(t, plain, "a").annots); n != 0 {
		t.Errorf("an unannotated list kept %d annotation columns, want 0", n)
	}

	one := prioModel(t,
		Todo{ID: "a", Title: "a", Prompt: "p", Priority: priorityCritical},
		Todo{ID: "b", Title: "b", Prompt: "p"},
	)
	// Both rows keep the same one column — the unmarked row's is blank, which is
	// what holds the names in line.
	for _, name := range []string{"a", "b"} {
		if n := len(rowNamed(t, one, name).annots); n != 1 {
			t.Errorf("row %q has %d annotation columns, want 1", name, n)
		}
	}
	if got := rowNamed(t, one, "b").annots[0]; got.text != "" || got.width != 1 {
		t.Errorf("the unmarked row's column = %+v, want a blank one cell wide", got)
	}

	both := prioModel(t,
		Todo{ID: "a", Title: "a", Prompt: "p", Priority: priorityHigh},
		Todo{ID: "b", Title: "b", Prompt: "p", Fruit: true},
	)
	if n := len(rowNamed(t, both, "a").annots); n != len(annotSlots) {
		t.Errorf("a list using every mark kept %d columns, want %d", n, len(annotSlots))
	}
}

// TestFruitMarksTheRow pins the second annotation end to end: the flag is on the
// todo, the apple is on the row, and it is independent of the priority beside it.
func TestFruitMarksTheRow(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "cheap", Title: "cheap", Prompt: "p", Fruit: true},
		Todo{ID: "both", Title: "both", Prompt: "p", Fruit: true, Priority: priorityCritical},
		Todo{ID: "plain", Title: "plain", Prompt: "p"},
	)
	if got := annotMarkFor(t, m, "cheap", "low-hanging fruit").text; got != fruitGlyph {
		t.Errorf("a fruit row carries %q, want %q", got, fruitGlyph)
	}
	if got := annotMarkFor(t, m, "plain", "low-hanging fruit").text; got != "" {
		t.Errorf("an unmarked row carries a fruit %q", got)
	}
	// Both at once is the whole reason these are columns rather than one badge.
	both := rowNamed(t, m, "both")
	if annotMarkFor(t, m, "both", "priority").text != prioCriticalGlyph ||
		annotMarkFor(t, m, "both", "low-hanging fruit").text != fruitGlyph {
		t.Errorf("a critical quick win lost one of its marks: %+v", both.annots)
	}
}

// TestFruitGlyphFitsItsColumn pins the emoji against the width annotSlots
// declares for it. It is the one mark that is not one cell, so a slot width that
// stopped matching would push every name on the list a column right.
func TestFruitGlyphFitsItsColumn(t *testing.T) {
	var width int
	for _, sl := range annotSlots {
		if sl.name == "low-hanging fruit" {
			width = sl.width
		}
	}
	if w := lipgloss.Width(fruitGlyph); w != width {
		t.Errorf("the fruit glyph is %d cells wide, its column reserves %d", w, width)
	}
}

// TestPriorityDotHues pins which style each level draws in — including the one
// that is not a level: a closed row recedes with the rest of its row rather than
// arguing for attention priority is meant to direct elsewhere.
func TestPriorityDotHues(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "crit", Title: "critical", Prompt: "p", Priority: priorityCritical},
		Todo{ID: "high", Title: "high", Prompt: "p", Priority: priorityHigh},
		Todo{ID: "donecrit", Title: "done critical", Prompt: "p", Priority: priorityCritical, Done: true},
	)
	want := map[string]lipgloss.Style{
		"critical":      prioCriticalStyle,
		"high":          prioHighStyle,
		"done critical": prioClosedStyle,
	}
	for name, w := range want {
		got := prioMark(t, m, name)
		if got.style.GetForeground() != w.GetForeground() {
			t.Errorf("row %q mark is %v, want %v", name, got.style.GetForeground(), w.GetForeground())
		}
	}
	// A scheduled row is still open work, so it keeps its hue — the case a
	// priority-tinted state badge would have lost.
	m2 := prioModel(t, Todo{ID: "s", Title: "sched", Prompt: "p", Priority: priorityCritical,
		Schedule: &Schedule{Kind: scheduleKindPane}})
	if got := prioMark(t, m2, "sched"); got.style.GetForeground() != prioCriticalStyle.GetForeground() {
		t.Error("a scheduled critical row lost its priority hue")
	}
}

// TestPriorityOrderIsALensNotARewrite is the load-bearing promise of the whole
// toggle: the rows resort, the file does not. Turning the lens off has to give
// back the exact order the user dragged the backlog into, and one pane's view
// preference must never rewrite a file the other panes are reading.
func TestPriorityOrderIsALensNotARewrite(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "a", Title: "a", Prompt: "p"},
		Todo{ID: "b", Title: "b", Prompt: "p", Priority: priorityHigh},
		Todo{ID: "c", Title: "c", Prompt: "p", Priority: priorityCritical},
		Todo{ID: "d", Title: "d", Prompt: "p"},
	)
	before := []string{"a", "b", "c", "d"}
	if got := ids(m.rows); !equalIDs(got, before) {
		t.Fatalf("initial rows = %v, want %v", got, before)
	}

	m.orderByPriority = true
	m.rebuildList()
	// Critical first, then high, then the two unmarked ones in the order the
	// file holds them (a stable sort).
	want := []string{"c", "b", "a", "d"}
	if got := ids(m.rows); !equalIDs(got, want) {
		t.Fatalf("rows under the lens = %v, want %v", got, want)
	}
	// The array itself is untouched — that is what makes the lens reversible.
	var arr []string
	for _, td := range m.project.todos {
		arr = append(arr, td.ID)
	}
	if !equalIDs(arr, before) {
		t.Fatalf("the lens rewrote the backlog: %v, want %v", arr, before)
	}

	m.orderByPriority = false
	m.rebuildList()
	if got := ids(m.rows); !equalIDs(got, before) {
		t.Fatalf("rows after the lens = %v, want the hand-set order %v", got, before)
	}
}

// TestPriorityOrderStaysInsideGroups pins the frame: priority orders what sits
// inside a group, never the groups themselves. A finished critical prompt rising
// above open work would be the lens answering a question nobody asked.
func TestPriorityOrderStaysInsideGroups(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "open", Title: "open", Prompt: "p"},
		Todo{ID: "donecrit", Title: "done", Prompt: "p", Priority: priorityCritical, Done: true},
	)
	m.orderByPriority = true
	m.rebuildList()
	if got := ids(m.rows); !equalIDs(got, []string{"open", "donecrit"}) {
		t.Errorf("rows = %v, want the open row first however the done one is ranked", got)
	}
}

// TestReorderRefusedUnderPriorityOrder pins the correctness trap. Under the lens
// the rows are in an order the array does not hold, so "put this one there"
// names a slot that does not exist — both the gesture and the chord have to say
// so rather than guess.
func TestReorderRefusedUnderPriorityOrder(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "a", Title: "a", Prompt: "p"},
		Todo{ID: "b", Title: "b", Prompt: "p"},
	)
	m.orderByPriority = true
	m.rebuildList()
	if m.canReorder() {
		t.Error("a drag was allowed while the priority lens was on")
	}

	m.list.cursor = 0
	mm, _ := m.moveSelected(1)
	got := mm.(model)
	if got.project.todos[0].ID != "a" {
		t.Error("ctrl+↓ reordered the backlog while the priority lens was on")
	}
	if !strings.Contains(got.status, "priority order") {
		t.Errorf("refusal said %q, want it to name priority order", got.status)
	}

	// The chord still works under a filter, which it always has: store.move
	// walks the array, and the array stays coherent however the rows are
	// filtered. Only the lens breaks it.
	m2 := prioModel(t,
		Todo{ID: "a", Title: "alpha", Prompt: "p"},
		Todo{ID: "b", Title: "beta", Prompt: "p"},
	)
	m2.list.input.SetValue("a")
	m2.list.filter()
	m2.list.cursor = 0
	mm2, _ := m2.moveSelected(1)
	if got := mm2.(model); got.project.todos[0].ID == "a" {
		t.Error("ctrl+↓ under a filter stopped working — that is a behaviour change beyond the lens")
	}
}

// TestShowFrozenHidesOnlyFrozen pins the split the View panel exists for: the
// ctrl+d fold takes both closed states, and this preference takes exactly one.
func TestShowFrozenHidesOnlyFrozen(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "open", Title: "open", Prompt: "p"},
		Todo{ID: "frozen", Title: "frozen", Prompt: "p", Frozen: true},
		Todo{ID: "done", Title: "done", Prompt: "p", Done: true},
	)
	m.showFrozen = false
	m.rebuildList()
	if got := ids(m.rows); !equalIDs(got, []string{"open", "done"}) {
		t.Fatalf("rows = %v, want the frozen one gone and the done one kept", got)
	}
	if n := m.hiddenClosedCount(); n != 1 {
		t.Errorf("hidden count = %d, want 1 (only the frozen row is hidden)", n)
	}
	if !strings.Contains(m.hiddenNote(), "1 hidden") {
		t.Errorf("header note = %q, want it to report the one hidden row", m.hiddenNote())
	}
}

// TestViewOptsRowsMatchWhatIsDrawn pins viewOptsRowsRow against the panel it
// aims clicks at. The hit test cannot re-measure the frame, so a line gained
// above the rows would send every click one row off.
func TestViewOptsRowsMatchWhatIsDrawn(t *testing.T) {
	m := prioModel(t, Todo{ID: "a", Title: "a", Prompt: "p"})
	mm, _ := m.beginViewOpts()
	m = mm.(model)
	lines := strings.Split(m.viewViewOpts(), "\n")
	for row := range viewRowCount {
		y := viewOptsRowsRow + row
		if y >= len(lines) {
			t.Fatalf("row %d is drawn at line %d, past the end of the panel", row, y)
		}
		if !strings.Contains(lines[y], viewRowLabels[row].label) {
			t.Errorf("line %d = %q, want the %q row", y, lines[y], viewRowLabels[row].label)
		}
	}
}

// TestViewOptsClickTogglesTheRowUnderIt pins that the pointer and the keyboard
// act from one place: a click moves the cursor to the row it lands on and flips
// exactly that switch.
func TestViewOptsClickTogglesTheRowUnderIt(t *testing.T) {
	m := prioModel(t, Todo{ID: "a", Title: "a", Prompt: "p"})
	mm, _ := m.beginViewOpts()
	m = mm.(model)

	before := m.orderByPriority
	mm, _ = m.clickViewOpts(tea.MouseClickMsg{Y: viewOptsRowsRow + viewRowPriority})
	m = mm.(model)
	if m.orderByPriority == before {
		t.Error("a click on the priority row did not flip it")
	}
	if m.viewOptsCursor != viewRowPriority {
		t.Errorf("cursor = %d, want it moved to the clicked row", m.viewOptsCursor)
	}

	before = m.showFrozen
	mm, _ = m.clickViewOpts(tea.MouseClickMsg{Y: viewOptsRowsRow + viewRowShowFrozen})
	if got := mm.(model); got.showFrozen == before {
		t.Error("a click on the frozen row did not flip it")
	}
	// A click off the rows does nothing rather than flipping whatever is
	// nearest.
	mm, _ = m.clickViewOpts(tea.MouseClickMsg{Y: viewOptsRowsRow + viewRowCount + 3})
	if got := mm.(model); got.orderByPriority != m.orderByPriority || got.showFrozen != m.showFrozen {
		t.Error("a click below the rows changed a switch")
	}
}

// TestViewOptsPersist pins that both switches survive a relaunch, and that a
// config directory with nothing in it gives the documented defaults.
func TestViewOptsPersist(t *testing.T) {
	m := prioModel(t, Todo{ID: "a", Title: "a", Prompt: "p"})
	if pref := loadSettings(); pref.orderByPriority || !pref.showFrozen {
		t.Fatalf("defaults = order:%v frozen:%v, want order off and frozen shown",
			pref.orderByPriority, pref.showFrozen)
	}

	mm, _ := m.beginViewOpts()
	m = mm.(model)
	mm, _ = m.toggleViewOptsRow(viewRowPriority)
	m = mm.(model)
	mm, _ = m.toggleViewOptsRow(viewRowShowFrozen)
	m = mm.(model)
	if m.viewOptsNote != "" {
		t.Fatalf("saving the preferences reported %q", m.viewOptsNote)
	}

	pref := loadSettings()
	if !pref.orderByPriority || pref.showFrozen {
		t.Errorf("saved = order:%v frozen:%v, want order on and frozen hidden",
			pref.orderByPriority, pref.showFrozen)
	}
}

// TestPriorityRadioSetsAndSaves walks the whole editor path: open a prompt, tab
// onto the annotation bar, walk to a level, press it, save — and the level is on
// the todo and on its row's mark. It also pins that the bar edits formAnnots and
// not the session record, which are two different places a value could wrongly
// land.
func TestPriorityRadioSetsAndSaves(t *testing.T) {
	m := prioModel(t, Todo{ID: "a", Title: "a", Prompt: "p"})
	m.list.cursor = 0

	mm, _ := m.beginEdit()
	m = mm.(model)
	if m.formAnnots.Priority != priorityNone {
		t.Fatalf("the form opened on %q, want none", m.formAnnots.Priority)
	}
	// The ring runs title → prompt → annotation bar, and the form opens in the
	// prompt — so one tab lands on the bar.
	mm, _ = m.updateForm(pressKey("tab"))
	m = mm.(model)
	if m.formFocus != formFieldAnnots {
		t.Fatalf("tab from the prompt left focus %d, want the annotation bar (%d)", m.formFocus, formFieldAnnots)
	}
	// → three times from the checkbox, across none and high, onto critical.
	for range 3 {
		mm, _ = m.updateForm(pressKey("right"))
		m = mm.(model)
	}
	if m.annotCursor != annotSegPrioCritical {
		t.Fatalf("three presses landed on segment %d, want critical (%d)", m.annotCursor, annotSegPrioCritical)
	}
	mm, _ = m.updateForm(pressKey("space"))
	m = mm.(model)
	if m.formAnnots.Priority != priorityCritical {
		t.Fatalf("pressing the critical radio left %q, want critical", m.formAnnots.Priority)
	}
	// The session record is untouched — the annotations are the Todo's, not its.
	if m.formSession.configured() {
		t.Error("pressing a radio wrote into the session options")
	}

	// Nothing is stored until the form is saved.
	if td, _ := m.project.find("a"); td.Priority != priorityNone {
		t.Error("the bar wrote to the backlog before the form was saved")
	}

	saved, _, ok := m.persistForm()
	if !ok {
		t.Fatalf("save refused: %s", saved.formErr)
	}
	m = saved
	if td, _ := m.project.find("a"); td.Priority != priorityCritical {
		t.Errorf("saved priority is %q, want critical", td.Priority)
	}
	if got := prioMark(t, m, "a"); got.text != prioCriticalGlyph {
		t.Errorf("the row's mark is %q, want it to follow the saved priority", got.text)
	}
}

// TestQuickWinTogglesAndSaves is the same walk for the other annotation, and the
// reason the two are one set: the bar's checkbox flips it, the save writes it,
// and it lands beside the priority rather than instead of it. Enter presses a
// segment too — it is the other key that means "push this button".
func TestQuickWinTogglesAndSaves(t *testing.T) {
	m := prioModel(t, Todo{ID: "a", Title: "a", Prompt: "p", Priority: priorityHigh})
	m.list.cursor = 0

	mm, _ := m.beginEdit()
	m = mm.(model)
	if m.formAnnots.Fruit {
		t.Fatal("the form opened with the fruit already set")
	}
	mm, _ = m.updateForm(pressKey("tab"))
	m = mm.(model)
	if m.annotCursor != annotSegFruit {
		t.Fatalf("the bar opened on segment %d, want the checkbox (%d) first", m.annotCursor, annotSegFruit)
	}
	mm, _ = m.updateForm(pressKey("enter"))
	m = mm.(model)
	if !m.formAnnots.Fruit {
		t.Fatal("enter on the Quick win checkbox did not set it")
	}
	if m.stage != stageForm {
		t.Fatalf("enter on the bar left stage %v — it must press the segment, not save", m.stage)
	}

	saved, _, ok := m.persistForm()
	if !ok {
		t.Fatalf("save refused: %s", saved.formErr)
	}
	m = saved
	td, _ := m.project.find("a")
	if !td.Fruit {
		t.Error("the saved todo did not keep the fruit")
	}
	if td.Priority != priorityHigh {
		t.Errorf("saving the fruit clobbered the priority: %q", td.Priority)
	}
	if got := annotMarkFor(t, m, "a", "low-hanging fruit").text; got != fruitGlyph {
		t.Errorf("the row carries %q, want the apple", got)
	}
}

// TestFormShowsAnnotationsOnTheBar pins the fix the bar exists for: the editor
// used to be the one screen a priority was invisible on, and now it is drawn
// there live — the chosen radio filled, the checkbox ticked — while the ⚙ line
// stays out of it rather than reading the same facts aloud one line down.
func TestFormShowsAnnotationsOnTheBar(t *testing.T) {
	m := prioModel(t, Todo{ID: "a", Title: "a", Prompt: "p", Priority: priorityCritical, Fruit: true})
	m.list.cursor = 0
	mm, _ := m.beginEdit()
	m = mm.(model)

	lines := strings.Split(m.viewForm(), "\n")
	if len(lines) <= formAnnotRow {
		t.Fatalf("the form renders %d lines, none at formAnnotRow (%d)", len(lines), formAnnotRow)
	}
	bar := lines[formAnnotRow]
	if !strings.Contains(bar, "☑") {
		t.Errorf("bar %q — want the Quick win checkbox ticked", bar)
	}
	if !strings.Contains(bar, "(•) "+prioCriticalGlyph) {
		t.Errorf("bar %q — want the critical radio filled", bar)
	}
	if got := m.sessionNote(); strings.Contains(got, "critical") || strings.Contains(got, "fruit") {
		t.Errorf("⚙ line = %q — the bar already shows the annotations; the line must not repeat them", got)
	}

	// An unannotated prompt: the box empty, and only the none radio filled.
	m2 := prioModel(t, Todo{ID: "b", Title: "b", Prompt: "p"})
	m2.list.cursor = 0
	mm, _ = m2.beginEdit()
	m2 = mm.(model)
	bar = strings.Split(m2.viewForm(), "\n")[formAnnotRow]
	if !strings.Contains(bar, "☐") || !strings.Contains(bar, "(•) none") {
		t.Errorf("bar %q — want an empty box and the none radio filled", bar)
	}
}

// TestCancellingTheFormLeavesPriorityAlone pins the promise every other field on
// this form makes: an abandoned edit changes nothing. formAnnots is held by
// value for exactly this reason.
func TestCancellingTheFormLeavesPriorityAlone(t *testing.T) {
	m := prioModel(t, Todo{ID: "a", Title: "a", Prompt: "p", Priority: priorityCritical})
	m.list.cursor = 0

	mm, _ := m.beginEdit()
	m = mm.(model)
	mm, _ = m.updateForm(pressKey("tab")) // prompt → the annotation bar
	m = mm.(model)
	mm, _ = m.updateForm(pressKey("space")) // set the fruit…
	m = mm.(model)
	mm, _ = m.updateForm(pressKey("right")) // …and walk onto none to clear the level
	m = mm.(model)
	mm, _ = m.updateForm(pressKey("space"))
	m = mm.(model)
	if m.formAnnots.Priority != priorityNone || !m.formAnnots.Fruit {
		t.Fatalf("the bar holds %+v, want the edit staged before cancelling", m.formAnnots)
	}
	m.cancelForm()

	td, _ := m.project.find("a")
	if td.Priority != priorityCritical {
		t.Errorf("a cancelled edit left priority %q, want it untouched at critical", td.Priority)
	}
	if td.Fruit {
		t.Error("a cancelled edit set the low-hanging-fruit mark")
	}
}

// TestCtrlPStillNavigatesTheList pins the chord that was handed back. Priority
// is set in the editor now, so the list keeps the emacs alias it briefly gave
// up — and nothing on the list stage sets a priority at all.
func TestCtrlPStillNavigatesTheList(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "a", Title: "a", Prompt: "p"},
		Todo{ID: "b", Title: "b", Prompt: "p"},
	)
	m.list.cursor = 1

	mm, _ := m.updateList(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	got := mm.(model)
	if got.list.cursor != 0 {
		t.Errorf("ctrl+p left the cursor at %d, want it moved up to 0", got.list.cursor)
	}
	if td, _ := got.project.find("b"); td.Priority != priorityNone {
		t.Errorf("ctrl+p on the list changed a priority to %q — the list does not set them", td.Priority)
	}
}

// TestPriorityRampIsInOrder pins the two hues against each other and against the
// tone closed rows recede to. Lightness is what separates the marks at one cell
// on a terminal that has flattened the palette, so the order is worth pinning:
// high lightest, then critical, both clear of the greys.
//
// Neither hue is invented — high takes cats' own todo yellow and critical the
// error red — so this test is what notices if the palette moves underneath them.
func TestPriorityRampIsInOrder(t *testing.T) {
	if got := prioHighStyle.GetForeground(); got != lipgloss.Color(colTodo) {
		t.Errorf("the high mark is %v, want cats' todo yellow %v", got, lipgloss.Color(colTodo))
	}
	if got := prioCriticalStyle.GetForeground(); got != lipgloss.Color(colErr) {
		t.Errorf("the critical mark is %v, want the error red %v", got, lipgloss.Color(colErr))
	}
	for _, pair := range [][2]string{{colTodo, colErr}, {colErr, colFaint}} {
		if relLum(pair[0]) <= relLum(pair[1]) {
			t.Errorf("%s is not lighter than %s — the priority ramp is out of order", pair[0], pair[1])
		}
	}
	// And the shapes carry the level on their own, for the reader the hues do
	// not reach: hollow for high, solid for critical.
	if prioHighGlyph == prioCriticalGlyph {
		t.Error("the two priority marks share a glyph — colour is then the only thing telling them apart")
	}
}

// relLum is the WCAG relative luminance of a #rrggbb string.
func relLum(hex string) float64 {
	var out float64
	for i, weight := range []float64{0.2126, 0.7152, 0.0722} {
		var v int
		fmt.Sscanf(hex[1+i*2:3+i*2], "%02x", &v)
		c := float64(v) / 255
		if c <= 0.04045 {
			c /= 12.92
		} else {
			c = math.Pow((c+0.055)/1.055, 2.4)
		}
		out += weight * c
	}
	return out
}

func ids(rows []todoRef) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.id
	}
	return out
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestPromptViewSpellsOutTheAnnotations pins the one screen that has room for
// both: the marks a row draws are also named in words on the prompt view, which
// is where someone goes to find out what a glyph on a row was trying to say.
//
// It also pins the agreement the slot table exists to guarantee — a mark that
// draws nothing says nothing, so a prompt cannot read as unmarked on its row and
// as ranked here.
func TestPromptViewSpellsOutTheAnnotations(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "a", Title: "marked", Prompt: "p", Priority: priorityCritical, Fruit: true},
		Todo{ID: "b", Title: "plain", Prompt: "p"},
		Todo{ID: "c", Title: "retired", Prompt: "p", Priority: "low"},
	)
	m.height = 30

	view := func(row int) string {
		t.Helper()
		mm := m
		mm.list.cursor = row
		next, _ := mm.beginView()
		return stripANSI(next.(model).viewPrompt())
	}

	got := view(0)
	for _, want := range []string{
		prioCriticalGlyph + " critical", fruitGlyph + " low-hanging fruit",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the view of a marked prompt is missing %q:\n%s", want, got)
		}
	}
	// And the state still leads the annotations, the same order the row reads in.
	if i, j := strings.Index(got, "backlog"), strings.Index(got, "critical"); i > j {
		t.Error("the annotations came before the backlog and date on the meta line")
	}

	if got := view(1); strings.Contains(got, "critical") || strings.Contains(got, "fruit") {
		t.Errorf("an unannotated prompt's view named a mark:\n%s", got)
	}
	// The retired "low" draws no mark, so it names none either.
	if got := view(2); strings.Contains(got, "low") {
		t.Errorf(`the view named the retired "low", which draws nothing:\n%s`, got)
	}
}

// TestClosedRowsDropTheFruit pins the only recession an emoji can make. The
// priority mark greys itself on a done or frozen row; the apple cannot — the
// font paints it and never sees a foreground — so a mark left there would be the
// one full-colour thing in the tier of the list that exists to stop shouting.
// It leaves instead, and the fixed column still holds the names in line as long
// as anything open fills it.
func TestClosedRowsDropTheFruit(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "open", Title: "open", Prompt: "p", Fruit: true},
		Todo{ID: "done", Title: "done", Prompt: "p", Fruit: true, Done: true},
		Todo{ID: "frozen", Title: "frozen", Prompt: "p", Fruit: true, Frozen: true},
	)
	if got := annotMarkFor(t, m, "open", "low-hanging fruit").text; got != fruitGlyph {
		t.Errorf("the open quick win carries %q, want %q", got, fruitGlyph)
	}
	for _, name := range []string{"done", "frozen"} {
		if got := annotMarkFor(t, m, name, "low-hanging fruit").text; got != "" {
			t.Errorf("the %s quick win still draws %q", name, got)
		}
		// The column itself stays — an open row above fills it, and a row that
		// dropped its columns would put its name a cell to the left of the rest.
		if n := len(rowNamed(t, m, name).annots); n != 1 {
			t.Errorf("the %s row has %d annotation columns, want the 1 the open row keeps", name, n)
		}
	}

	// And when every quick win in the list is finished, nobody fills the column
	// and the usual trim takes it away entirely — the same answer an unannotated
	// backlog gets, which is the point: these rows are no longer marked.
	closed := prioModel(t,
		Todo{ID: "done", Title: "done", Prompt: "p", Fruit: true, Done: true},
		Todo{ID: "plain", Title: "plain", Prompt: "p"},
	)
	for _, name := range []string{"done", "plain"} {
		if n := len(rowNamed(t, closed, name).annots); n != 0 {
			t.Errorf("a list whose only quick win is finished kept %d columns on %q, want 0", n, name)
		}
	}
}

// TestTheClosedFruitStillReadsInWords is the other half of that trade. Dropping
// the glyph must not drop the fact: ctrl+v is the screen someone opens to find
// out what was said about a prompt, and "this was a quick win" does not stop
// being true when the work is done. The words are the record the row no longer
// keeps, which is why the prompt view spells the annotations out from their
// labels rather than from the glyphs the row draws.
func TestTheClosedFruitStillReadsInWords(t *testing.T) {
	m := prioModel(t, Todo{ID: "d", Title: "done", Prompt: "body",
		Fruit: true, Priority: priorityCritical, Done: true})
	m.height = 40
	next, _ := m.beginView()
	m = next.(model)
	got := stripANSI(m.View().Content)
	for _, want := range []string{"critical", "low-hanging fruit"} {
		if !strings.Contains(got, want) {
			t.Errorf("the prompt view of a finished todo never says %q:\n%s", want, got)
		}
	}
	// The glyph the row no longer draws is not smuggled back in here either: the
	// apple was dropped because it cannot recede, and this screen is drawn in the
	// same greys.
	if strings.Contains(got, fruitGlyph) {
		t.Errorf("the prompt view of a finished quick win still draws the apple:\n%s", got)
	}
}
