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

// TestEveryRowCarriesAPriorityDot pins the requirement the column exists for:
// standard is a level and not an absence, so no row is allowed to leave the
// column blank. A column with holes in it cannot be read down, which is the only
// thing a color on a row is worth anything for.
func TestEveryRowCarriesAPriorityDot(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "open", Title: "open standard", Prompt: "p"},
		Todo{ID: "crit", Title: "open critical", Prompt: "p", Priority: priorityCritical},
		Todo{ID: "low", Title: "open low", Prompt: "p", Priority: priorityLow},
		Todo{ID: "frozen", Title: "frozen", Prompt: "p", Frozen: true},
		Todo{ID: "done", Title: "done", Prompt: "p", Done: true},
	)
	n := 0
	for _, it := range m.list.items {
		if !it.selectable {
			continue // group headings carry no marks
		}
		n++
		if it.prio != prioGlyph {
			t.Errorf("row %q carries prio %q, want %q", it.name, it.prio, prioGlyph)
		}
	}
	if n != 5 {
		t.Fatalf("checked %d selectable rows, want 5", n)
	}
}

// TestPriorityDotHues pins which style each level draws in — including the one
// that is not a level: a closed row recedes with the rest of its row rather than
// arguing for attention priority is meant to direct elsewhere.
func TestPriorityDotHues(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "crit", Title: "critical", Prompt: "p", Priority: priorityCritical},
		Todo{ID: "std", Title: "standard", Prompt: "p"},
		Todo{ID: "low", Title: "low", Prompt: "p", Priority: priorityLow},
		Todo{ID: "donecrit", Title: "done critical", Prompt: "p", Priority: priorityCritical, Done: true},
	)
	want := map[string]lipgloss.Style{
		"critical":      prioCriticalStyle,
		"standard":      prioStandardStyle,
		"low":           prioLowStyle,
		"done critical": prioClosedStyle,
	}
	for _, it := range m.list.items {
		w, ok := want[it.name]
		if !ok {
			continue
		}
		if it.prioStyle.GetForeground() != w.GetForeground() {
			t.Errorf("row %q dot is %v, want %v", it.name, it.prioStyle.GetForeground(), w.GetForeground())
		}
	}
	// A scheduled row is still open work, so it keeps its hue — the case a
	// priority-tinted state badge would have lost.
	m2 := prioModel(t, Todo{ID: "s", Title: "sched", Prompt: "p", Priority: priorityCritical,
		Schedule: &Schedule{Kind: scheduleKindPane}})
	for _, it := range m2.list.items {
		if it.name == "sched" && it.prioStyle.GetForeground() != prioCriticalStyle.GetForeground() {
			t.Error("a scheduled critical row lost its priority hue")
		}
	}
}

// TestPriorityOrderIsALensNotARewrite is the load-bearing promise of the whole
// toggle: the rows resort, the file does not. Turning the lens off has to give
// back the exact order the user dragged the backlog into, and one pane's view
// preference must never rewrite a file the other panes are reading.
func TestPriorityOrderIsALensNotARewrite(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "a", Title: "a", Prompt: "p"},
		Todo{ID: "b", Title: "b", Prompt: "p", Priority: priorityLow},
		Todo{ID: "c", Title: "c", Prompt: "p", Priority: priorityCritical},
		Todo{ID: "d", Title: "d", Prompt: "p"},
	)
	before := []string{"a", "b", "c", "d"}
	if got := ids(m.rows); !equalIDs(got, before) {
		t.Fatalf("initial rows = %v, want %v", got, before)
	}

	m.orderByPriority = true
	m.rebuildList()
	// Critical first, then the two standards in the order the file holds them
	// (a stable sort), then low.
	want := []string{"c", "a", "d", "b"}
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
		Todo{ID: "open", Title: "open", Prompt: "p", Priority: priorityLow},
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

// TestPriorityRowCyclesAndSaves walks the whole editor path: open a prompt, open
// the ⚙ panel, cycle the Priority row, save — and the level is on the todo and
// on its row's dot. It also pins that the row cycles formPriority and not the
// session record, which are two different places a value could wrongly land.
func TestPriorityRowCyclesAndSaves(t *testing.T) {
	m := prioModel(t, Todo{ID: "a", Title: "a", Prompt: "p"})
	m.list.cursor = 0

	mm, _ := m.beginEdit()
	m = mm.(model)
	if m.formPriority != priorityStandard {
		t.Fatalf("the form opened on %q, want standard", m.formPriority)
	}
	mm, _ = m.beginSession()
	m = mm.(model)
	if m.sessCursor != sessRowPriority {
		t.Fatalf("the panel opened on row %d, want Priority (%d) first", m.sessCursor, sessRowPriority)
	}

	// → once: standard → critical, and the value column says so.
	mm, _ = m.updateSession(tea.KeyPressMsg{Code: tea.KeyRight})
	m = mm.(model)
	if m.formPriority != priorityCritical {
		t.Fatalf("one press left %q, want critical", m.formPriority)
	}
	if got := m.sessValueLabel(sessRowPriority); got != "critical" {
		t.Errorf("value column reads %q, want %q", got, "critical")
	}
	// The session record is untouched — priority is the Todo's, not its.
	if m.formSession.configured() {
		t.Error("cycling Priority wrote into the session options")
	}

	// Nothing is stored until the form is saved.
	if td, _ := m.project.find("a"); td.Priority != priorityStandard {
		t.Error("the panel wrote to the backlog before the form was saved")
	}

	m.stage = stageForm
	saved, _, ok := m.persistForm()
	if !ok {
		t.Fatalf("save refused: %s", saved.formErr)
	}
	m = saved
	if td, _ := m.project.find("a"); td.Priority != priorityCritical {
		t.Errorf("saved priority is %q, want critical", td.Priority)
	}
	for _, it := range m.list.items {
		if it.name == "a" && it.prioStyle.GetForeground() != prioCriticalStyle.GetForeground() {
			t.Error("the row's dot did not follow the saved priority")
		}
	}
}

// TestFormSessionLineShowsPriority pins the one place a priority would otherwise
// be invisible: the editor is the only screen without the dot on it, and it is
// the screen that sets the level.
func TestFormSessionLineShowsPriority(t *testing.T) {
	m := prioModel(t, Todo{ID: "a", Title: "a", Prompt: "p", Priority: priorityCritical})
	m.list.cursor = 0
	mm, _ := m.beginEdit()
	m = mm.(model)
	if got := m.sessionNote(); !strings.HasPrefix(got, "critical · ") {
		t.Errorf("⚙ line = %q, want it to lead with the priority", got)
	}

	// Standard stays silent — the line says what is not ordinary.
	m2 := prioModel(t, Todo{ID: "b", Title: "b", Prompt: "p"})
	m2.list.cursor = 0
	mm, _ = m2.beginEdit()
	if got := mm.(model).sessionNote(); strings.Contains(got, "standard") {
		t.Errorf("⚙ line = %q, want standard to say nothing", got)
	}
}

// TestCancellingTheFormLeavesPriorityAlone pins the promise every other field on
// this form makes: an abandoned edit changes nothing. formPriority is held by
// value for exactly this reason.
func TestCancellingTheFormLeavesPriorityAlone(t *testing.T) {
	m := prioModel(t, Todo{ID: "a", Title: "a", Prompt: "p", Priority: priorityCritical})
	m.list.cursor = 0

	mm, _ := m.beginEdit()
	m = mm.(model)
	mm, _ = m.beginSession()
	m = mm.(model)
	mm, _ = m.updateSession(tea.KeyPressMsg{Code: tea.KeyRight})
	m = mm.(model)
	m.cancelForm()

	if td, _ := m.project.find("a"); td.Priority != priorityCritical {
		t.Errorf("a cancelled edit left priority %q, want it untouched at critical", td.Priority)
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
	if td, _ := got.project.find("b"); td.Priority != priorityStandard {
		t.Errorf("ctrl+p on the list changed a priority to %q — the list does not set them", td.Priority)
	}
}

// TestPriorityDotUsesTheBrownForLow pins low's hue. It is the one dot whose
// colour is not taken from an existing palette entry, so a change to it is a
// deliberate act rather than a knock-on from something else moving.
func TestPriorityDotUsesTheBrownForLow(t *testing.T) {
	if got := prioLowStyle.GetForeground(); got != lipgloss.Color(colBrown) {
		t.Errorf("low dot is %v, want the brown %v", got, lipgloss.Color(colBrown))
	}
	// The ramp the three are read by: standard lightest, then critical, then
	// low, all clear of the tone closed rows recede to. Lightness is what
	// separates them at one cell, so the order is worth pinning.
	for _, pair := range [][2]string{{colTodo, colErr}, {colErr, colBrown}, {colBrown, colFaint}} {
		if relLum(pair[0]) <= relLum(pair[1]) {
			t.Errorf("%s is not lighter than %s — the priority ramp is out of order", pair[0], pair[1])
		}
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
