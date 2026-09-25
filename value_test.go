package main

import (
	"encoding/json"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The value mark, end to end. It was one bit (the 💎 gem) and is now a level —
// low (the default), medium, high — so what is pinned here is the storage that keeps
// the gem's key, the row it shares with the marks beside it, and the two
// screens it is set on: the form's bar and the list's context menu.

// TestValueLevelsMarkTheRow: each raised level draws its own diamond on the
// row, and low — the default, what an unrated prompt is — draws nothing. The fruit still sits right before it, the
// cost/payoff pair read as one fact.
func TestValueLevelsMarkTheRow(t *testing.T) {
	td := func(id, v string) Todo {
		t := Todo{ID: id, Title: id, Prompt: "p"}
		t.setValueLevel(v)
		return t
	}
	pair := td("pair", valueHigh)
	pair.Fruit = true
	m := prioModel(t, td("high", valueHigh), td("medium", valueMedium), td("low", valueLow), pair)

	for name, want := range map[string]string{"high": valueHighGlyph, "medium": valueMediumGlyph, "low": ""} {
		if got := annotMarkFor(t, m, name, "value").text; got != want {
			t.Errorf("the %s row carries %q, want %q", name, got, want)
		}
	}
	marks := rowNamed(t, m, "pair").annots
	if len(marks) != 2 || marks[0].text != fruitGlyph || marks[1].text != valueHighGlyph {
		t.Errorf("a cheap, valuable row carries %+v, want the apple then the blue diamond", marks)
	}
}

// TestValueGlyphWidths pins the cells each mark takes. Nothing reserves cells
// for them on a backlog row, but the Next List pads every mark to two
// (nextValueMarkWidth), and a glyph that measured differently from what the
// terminal paints would leave the packed marks and the name overlapping.
func TestValueGlyphWidths(t *testing.T) {
	for glyph, want := range map[string]int{valueHighGlyph: 2, valueMediumGlyph: 1, valueLowGlyph: 1} {
		if w := lipgloss.Width(glyph); w != want {
			t.Errorf("%q is %d cells wide, want %d", glyph, w, want)
		}
	}
}

// TestValueStorageKeepsTheGemKey pins the compat bargain in value.go: high is
// still written as `highValue: true` and nothing else, so a backlog of gems is
// byte-identical to what it was, and an older binary still reads every one.
// Only medium uses the new `value` key, never alongside the old one, and low —
// the default — writes nothing at all.
func TestValueStorageKeepsTheGemKey(t *testing.T) {
	enc := func(v string) string {
		td := Todo{ID: "a", Prompt: "p"}
		td.setValueLevel(v)
		b, err := json.Marshal(td)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if got := enc(valueHigh); !strings.Contains(got, `"highValue":true`) || strings.Contains(got, `"value"`) {
		t.Errorf("high encodes as %s, want the gem's key alone", got)
	}
	if got := enc(valueMedium); !strings.Contains(got, `"value":"medium"`) || strings.Contains(got, "highValue") {
		t.Errorf("medium encodes as %s, want the value key alone", got)
	}
	if got := enc(valueLow); strings.Contains(got, "alue") {
		t.Errorf("low encodes as %s, want neither key", got)
	}

	// Reading: the old key, a hand-written "high", "low" spelled out, and a
	// level this program has no mark for — the last two both the default.
	for src, want := range map[string]string{
		`{"id":"a","prompt":"p","highValue":true}`: valueHigh,
		`{"id":"a","prompt":"p","value":"high"}`:   valueHigh,
		`{"id":"a","prompt":"p","value":"low"}`:    valueLow,
		`{"id":"a","prompt":"p","value":"huge"}`:   valueLow,
		`{"id":"a","prompt":"p"}`:                  valueLow,
	} {
		var td Todo
		if err := json.Unmarshal([]byte(src), &td); err != nil {
			t.Fatal(err)
		}
		if got := td.valueLevel(); got != want {
			t.Errorf("%s reads as %q, want %q", src, got, want)
		}
	}
}

// TestClosedRowsDropTheValueMark: both raised levels go quiet on finished and
// shelved work, the way the apple does. The high step is an emoji with no grey
// to recede into, and medium goes with it so the done tier does not look as if
// only its lesser prompts were rated. The prompt view still says it.
func TestClosedRowsDropTheValueMark(t *testing.T) {
	var tds []Todo
	for _, v := range []string{valueMedium, valueHigh} {
		done := Todo{ID: "done-" + v, Title: "done-" + v, Prompt: "p", Done: true}
		done.setValueLevel(v)
		frozen := Todo{ID: "frozen-" + v, Title: "frozen-" + v, Prompt: "p", Frozen: true}
		frozen.setValueLevel(v)
		tds = append(tds, done, frozen)
	}
	m := prioModel(t, tds...)
	for _, td := range tds {
		if got := annotMarkFor(t, m, td.Title, "value").text; got != "" {
			t.Errorf("the closed row %s still draws %q", td.Title, got)
		}
		if n := len(rowNamed(t, m, td.Title).annots); n != 0 {
			t.Errorf("the closed row %s has %d annotations, want 0", td.Title, n)
		}
	}

	d := Todo{ID: "d", Title: "done", Prompt: "body", Done: true}
	d.setValueLevel(valueMedium)
	view := prioModel(t, d)
	view.height = 40
	next, _ := view.beginView()
	view = next.(model)
	if got := stripANSI(view.View().Content); !strings.Contains(got, "medium value") {
		t.Errorf("the prompt view of a finished todo never says \"medium value\":\n%s", got)
	}
}

// TestValueRadiosOnTheBarAndSave drives the radios the way a hand does: → off
// the Quick win box lands on Value's low (the chosen default), one more
// reaches medium, space chooses it, and the save carries it to the backlog
// without touching its neighbours.
func TestValueRadiosOnTheBarAndSave(t *testing.T) {
	m := prioModel(t, Todo{ID: "a", Title: "a", Prompt: "p", Priority: priorityHigh, Fruit: true})
	m.list.cursor = 0

	mm, _ := m.beginEdit()
	m = mm.(model)
	if m.formAnnots.Value != valueLow {
		t.Fatalf("the form opened with value %q already set", m.formAnnots.Value)
	}
	m.focusForm(formFieldAnnots)
	mm, _ = m.updateForm(pressKey("right"))
	m = mm.(model)
	if m.annotCursor != annotSegValueLow {
		t.Fatalf("→ from the Quick win box landed on segment %d, want Value: low (%d)", m.annotCursor, annotSegValueLow)
	}
	for range annotSegValueMedium - annotSegValueLow {
		mm, _ = m.updateForm(pressKey("right"))
		m = mm.(model)
	}
	mm, _ = m.updateForm(pressKey("space"))
	m = mm.(model)
	if m.formAnnots.Value != valueMedium {
		t.Fatalf("space on the medium radio set %q", m.formAnnots.Value)
	}
	if td, _ := m.project.find("a"); td.valueLevel() != valueLow {
		t.Error("the bar wrote to the backlog before the form was saved")
	}

	saved, _, ok := m.persistForm()
	if !ok {
		t.Fatalf("save refused: %s", saved.formErr)
	}
	m = saved
	td, _ := m.project.find("a")
	if td.valueLevel() != valueMedium || !td.Fruit || td.Priority != priorityHigh {
		t.Errorf("saved %+v, want medium value beside the untouched fruit and priority", annotsOf(td))
	}
	if got := annotMarkFor(t, m, "a", "value").text; got != valueMediumGlyph {
		t.Errorf("the row carries %q, want the medium diamond", got)
	}

	// A radio does not un-choose: pressing medium again keeps it, and low is
	// the way back to the default.
	m.formAnnots.Value = valueMedium
	m.activateAnnotSeg(annotSegValueMedium)
	if m.formAnnots.Value != valueMedium {
		t.Error("pressing the chosen radio changed the level")
	}
	m.activateAnnotSeg(annotSegValueLow)
	if m.formAnnots.Value != valueLow {
		t.Error("pressing Value: low did not return to the default")
	}
}

// TestListMenuSetsValue: the menu's three value rows show which level is held
// and set the one pressed, and the status line names it, since the menu closes
// with the press and leaves nothing else to confirm it landed.
func TestListMenuSetsValue(t *testing.T) {
	m := withTodos(t, "first", "second")
	m = rightClickRow(t, m, 0)
	for act, want := range map[int]string{
		listMenuValueLow:    "(•) Value: " + valueLowGlyph + " low",
		listMenuValueMedium: "( ) Value: " + valueMediumGlyph + " medium",
		listMenuValueHigh:   "( ) Value: " + valueHighGlyph + " high",
	} {
		if got := m.listMenu.items[act].label; got != want {
			t.Errorf("row %d reads %q, want %q", act, got, want)
		}
	}

	next, _ := m.pressListMenu(listMenuValueHigh)
	m = next.(model)
	if td, _ := m.project.find("a"); td.valueLevel() != valueHigh || !td.HighValue {
		t.Fatalf("pressing Value: high stored %+v", td)
	}
	if !strings.Contains(m.status, "value high") {
		t.Errorf("the status line says %q, want it to name the level", m.status)
	}

	m = rightClickRow(t, m, 0)
	if got := m.listMenu.items[listMenuValueHigh].label; !strings.HasPrefix(got, "(•)") {
		t.Errorf("reopened, the high row reads %q, want it filled", got)
	}
	next, _ = m.pressListMenu(listMenuValueLow)
	m = next.(model)
	if td, _ := m.project.find("a"); td.valueLevel() != valueLow || td.HighValue || td.Value != "" {
		t.Errorf("stepping down to low stored %+v, want both keys cleared", td)
	}
}

// TestAnnotBarConcedesInOrder pins the tiers. The bar sits on a hit-tested
// row, so it may never wrap and may never drop a segment; all it can give up is
// words, then gaps, then the radios' holes. The widest tier that fits must win,
// and the narrowest must still fit 30 cells, the narrowest pane this form is
// drawn in at all. Measured with every mark set, the widest the texts get.
func TestAnnotBarConcedesInOrder(t *testing.T) {
	m := withForm(t, "t", "p", 200, 40)
	m.formAnnots = annots{Priority: priorityCritical, Fruit: true, Value: valueHigh, Info: true, Flag: true}
	tiers := m.annotBarTiers()

	for i := 1; i < len(tiers); i++ {
		if tiers[i].width() >= tiers[i-1].width() {
			t.Errorf("tier %d (%d cells) does not concede against tier %d (%d cells)",
				i, tiers[i].width(), i-1, tiers[i-1].width())
		}
	}
	if w := tiers[len(tiers)-1].width(); w > 30 {
		t.Errorf("the narrowest tier is %d cells, want it to fit a 30-cell pane", w)
	}
	for i, tier := range tiers {
		for seg, text := range tier.texts {
			if text == "" {
				t.Errorf("tier %d dropped segment %d", i, seg)
			}
		}
	}

	for i, tier := range tiers {
		m.width = tier.width()
		if _, line := m.annotBarLayout(); lipgloss.Width(line) != tier.width() {
			t.Errorf("a pane of exactly %d cells did not take the tier that fits it", tier.width())
		}
		m.width = tier.width() - 1
		_, line := m.annotBarLayout()
		if i == len(tiers)-1 {
			if lipgloss.Width(line) != tier.width() {
				t.Errorf("a pane one cell short of the floor drew %d cells, want the floor's %d",
					lipgloss.Width(line), tier.width())
			}
			continue
		}
		if lipgloss.Width(line) >= tier.width() {
			t.Errorf("a pane one cell short of %d did not concede", tier.width())
		}
	}
}

// TestAnnotBarSeparatesItsGroups: every tier draws a rule between the four
// groups (fruit │ value │ priority │ info and flag), the labelled tiers name
// the two radio groups, and a click on a rule or a label presses nothing.
// Without the rule, the narrow tiers' two radio groups ran together into one
// run of holes.
func TestAnnotBarSeparatesItsGroups(t *testing.T) {
	m := withForm(t, "t", "p", 200, 40)
	for _, tier := range m.annotBarTiers() {
		m.width = tier.width()
		segs, line := m.annotBarLayout()
		plain := ansi.Strip(line)
		if n := strings.Count(plain, annotSepGlyph); n != 3 {
			t.Errorf("the %d-cell tier draws %d rules, want 3: %q", tier.width(), n, plain)
		}
		if tier.labels && (!strings.Contains(plain, "Value") || !strings.Contains(plain, "Priority")) {
			t.Errorf("the labelled %d-cell tier does not name its groups: %q", tier.width(), plain)
		}
		// Every cell between a group's last segment and the next group's first
		// is inert.
		for i := range annotSegCount {
			if _, ok := annotGroupStart[i]; !ok {
				continue
			}
			for x := segs[i-1].end; x < segs[i].start; x++ {
				before := m.formAnnots
				got := clickAnnot(m, x)
				if got.formAnnots != before {
					t.Errorf("the %d-cell tier: a click at %d, between groups, changed the marks", tier.width(), x)
				}
			}
		}
	}
}

// TestAnnotBarBareTierLightsTheChoice: on the bare tier the radios have no
// holes, so the chosen one is drawn in reverse — the one thing on that tier
// saying which level is chosen without relying on colour.
func TestAnnotBarBareTierLightsTheChoice(t *testing.T) {
	m := withForm(t, "t", "p", 30, 40)
	m.formAnnots.Value = valueMedium
	if !m.annotSegStyle(annotSegValueMedium, true).GetReverse() {
		t.Error("the chosen value radio is not lit on the bare tier")
	}
	if m.annotSegStyle(annotSegValueLow, true).GetReverse() {
		t.Error("an unchosen value radio is lit on the bare tier")
	}
	if m.annotSegStyle(annotSegValueMedium, false).GetReverse() {
		t.Error("the chosen radio is lit on a tier that still draws its hole")
	}
}

// TestNormalizeValue pins --value's spellings, the same closed set the bar
// and the menu offer.
func TestNormalizeValue(t *testing.T) {
	for in, want := range map[string]string{
		"": valueLow, "none": valueLow, "LOW": valueLow, "lo": valueLow,
		"med": valueMedium, "Medium": valueMedium, "hi": valueHigh, "high": valueHigh,
	} {
		if got, err := normalizeValue(in); err != nil || got != want {
			t.Errorf("normalizeValue(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := normalizeValue("huge"); err == nil {
		t.Error("normalizeValue accepted \"huge\"")
	}
}
