package main

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// The high-value gem, end to end. It is the newest annotation and the one that
// pushed the form's bar onto a third tier, so what it is pinned against here is
// the two things a mark can break: the row it shares with the marks beside it,
// and the bar it is set on.

// TestHighValueMarksTheRow pins the mark end to end — the field is on the todo,
// the gem is on the row, and it is independent of everything it sits beside.
// Independence is the whole claim: a fourth priority level could not have said
// "cheap and worth a lot" at the same time, which is exactly what the pair the
// gem completes is for.
func TestHighValueMarksTheRow(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "worth", Title: "worth", Prompt: "p", HighValue: true},
		Todo{ID: "pair", Title: "pair", Prompt: "p", HighValue: true, Fruit: true},
		Todo{ID: "plain", Title: "plain", Prompt: "p"},
	)
	if got := annotMarkFor(t, m, "worth", "high value").text; got != valueGlyph {
		t.Errorf("a high-value row carries %q, want %q", got, valueGlyph)
	}
	if got := annotMarkFor(t, m, "plain", "high value").text; got != "" {
		t.Errorf("an unmarked row carries a gem %q", got)
	}
	// The cost/payoff pair on one row, in slot order: the apple, then the gem.
	// The order is fixed even though the positions are not, so the two always
	// read the same way round however many other marks the row wears.
	marks := rowNamed(t, m, "pair").annots
	if len(marks) != 2 || marks[0].text != fruitGlyph || marks[1].text != valueGlyph {
		t.Errorf("a cheap, valuable row carries %+v, want the apple then the gem", marks)
	}
}

// TestTheGemStaysTwoCells pins the emoji's width for the reason the
// apple's is pinned: nothing reserves cells for it, but it shares a row with the
// marks and the name after it, and a glyph that measured differently from what
// the terminal paints would leave the packed group and the name overlapping or a
// cell apart.
func TestTheGemStaysTwoCells(t *testing.T) {
	if w := lipgloss.Width(valueGlyph); w != 2 {
		t.Errorf("the gem is %d cells wide, want 2", w)
	}
}

// TestClosedRowsDropTheGem: the gem goes quiet on finished and shelved
// work exactly as the apple does, and for the same reason — an emoji ignores a
// foreground, so there is no grey for it to recede into, and a full-colour mark
// in the tier of the list that exists to stop shouting would be the loudest
// thing on the screen. "This one pays" is an argument for picking a prompt up,
// and there is nothing to pick up down there.
func TestClosedRowsDropTheGem(t *testing.T) {
	m := prioModel(t,
		Todo{ID: "open", Title: "open", Prompt: "p", HighValue: true},
		Todo{ID: "done", Title: "done", Prompt: "p", HighValue: true, Done: true},
		Todo{ID: "frozen", Title: "frozen", Prompt: "p", HighValue: true, Frozen: true},
	)
	if got := annotMarkFor(t, m, "open", "high value").text; got != valueGlyph {
		t.Errorf("the open high-value row carries %q, want %q", got, valueGlyph)
	}
	for _, name := range []string{"done", "frozen"} {
		if got := annotMarkFor(t, m, name, "high value").text; got != "" {
			t.Errorf("the %s high-value row still draws %q", name, got)
		}
		// And it takes its cells with it, like the apple.
		if n := len(rowNamed(t, m, name).annots); n != 0 {
			t.Errorf("the %s row has %d annotations, want 0 once the gem is gone", name, n)
		}
	}

	// The fact survives the glyph: ctrl+v is the screen someone opens to find
	// out what was said about a prompt, and "this was worth a lot" does not stop
	// being true when the work is done.
	view := prioModel(t, Todo{ID: "d", Title: "done", Prompt: "body", HighValue: true, Done: true})
	view.height = 40
	next, _ := view.beginView()
	view = next.(model)
	got := stripANSI(view.View().Content)
	if !strings.Contains(got, "high value") {
		t.Errorf("the prompt view of a finished todo never says \"high value\":\n%s", got)
	}
	if strings.Contains(got, valueGlyph) {
		t.Errorf("the prompt view of a finished high-value prompt still draws the gem:\n%s", got)
	}
}

// TestHighValueTogglesOnTheBarAndSaves drives the checkbox the way a hand does:
// one → off the Quick win box lands on it, space ticks it, and the save carries
// it to the backlog without touching the marks either side.
func TestHighValueTogglesOnTheBarAndSaves(t *testing.T) {
	m := prioModel(t, Todo{ID: "a", Title: "a", Prompt: "p", Priority: priorityHigh, Fruit: true})
	m.list.cursor = 0

	mm, _ := m.beginEdit()
	m = mm.(model)
	if m.formAnnots.HighValue {
		t.Fatal("the form opened with the gem already set")
	}
	m.focusForm(formFieldAnnots) // onto the bar, parked on Quick win; tab in the prompt indents now
	mm, _ = m.updateForm(pressKey("right"))
	m = mm.(model)
	if m.annotCursor != annotSegValue {
		t.Fatalf("→ from the Quick win box landed on segment %d, want High value (%d)",
			m.annotCursor, annotSegValue)
	}
	mm, _ = m.updateForm(pressKey("space"))
	m = mm.(model)
	if !m.formAnnots.HighValue {
		t.Fatal("space on the High value checkbox did not set it")
	}
	// Nothing reaches the backlog until the form is saved — the promise every
	// control on this form makes.
	if td, _ := m.project.find("a"); td.HighValue {
		t.Error("the bar wrote to the backlog before the form was saved")
	}

	saved, _, ok := m.persistForm()
	if !ok {
		t.Fatalf("save refused: %s", saved.formErr)
	}
	m = saved
	td, _ := m.project.find("a")
	if !td.HighValue {
		t.Error("the saved todo did not keep the gem")
	}
	if !td.Fruit || td.Priority != priorityHigh {
		t.Errorf("saving the gem clobbered a neighbour: %+v", annotsOf(td))
	}
	if got := annotMarkFor(t, m, "a", "high value").text; got != valueGlyph {
		t.Errorf("the row carries %q, want the gem", got)
	}
}

// TestListMenuMarksHighValue: the menu is the list's only road to the mark, so
// the row has to both show the state and write it — and say so, since a
// checkbox pressed on a menu that closes with it leaves nothing else to confirm
// the press landed.
func TestListMenuMarksHighValue(t *testing.T) {
	m := withTodos(t, "first", "second")
	m = rightClickRow(t, m, 0)
	if got := m.listMenu.items[listMenuValue].label; !strings.HasPrefix(got, "☐ "+valueGlyph) {
		t.Errorf("the row reads %q, want an empty box and the gem", got)
	}

	next, _ := m.pressListMenu(listMenuValue)
	m = next.(model)
	td, _ := m.project.find("a")
	if !td.HighValue {
		t.Fatal("pressing the row did not mark the todo")
	}
	if !strings.Contains(m.status, "high value") {
		t.Errorf("the status line says %q, want it to name the mark", m.status)
	}

	// Re-opened, the row shows the state it wrote, and pressing it again clears
	// it — a checkbox, unlike the priority radios beside it.
	m = rightClickRow(t, m, 0)
	if got := m.listMenu.items[listMenuValue].label; !strings.HasPrefix(got, "☑ "+valueGlyph) {
		t.Errorf("the row reads %q, want a ticked box", got)
	}
	next, _ = m.pressListMenu(listMenuValue)
	m = next.(model)
	if td, _ := m.project.find("a"); td.HighValue {
		t.Error("pressing the row a second time did not clear the mark")
	}
}

// TestAnnotBarConcedesInOrder pins the three tiers the sixth segment forced.
// The bar sits on a hit-tested row, so it may never wrap and may never drop a
// segment; all it can give up is words, then the space inside each segment. The
// widest tier that fits must win, so a pane with room for the words is never
// shown glyphs — and the narrowest must still fit 30 cells, which is the
// narrowest pane this form is drawn in at all.
func TestAnnotBarConcedesInOrder(t *testing.T) {
	m := withForm(t, "t", "p", 200, 40)
	m.formAnnots = annots{Priority: priorityCritical, Fruit: true, HighValue: true}
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
	// Every tier still spells every segment: the concession is never a segment.
	for i, tier := range tiers {
		for seg, text := range tier.texts {
			if text == "" {
				t.Errorf("tier %d dropped segment %d", i, seg)
			}
		}
	}

	// And the pane picks the widest that fits, checked at the seam on either
	// side of each tier's own width.
	for i, tier := range tiers {
		m.width = tier.width()
		if _, line := m.annotBarLayout(); lipgloss.Width(line) != tier.width() {
			t.Errorf("a pane of exactly %d cells did not take the tier that fits it", tier.width())
		}
		m.width = tier.width() - 1
		_, line := m.annotBarLayout()
		if i == len(tiers)-1 {
			// The narrowest tier is the floor and is drawn even when the pane
			// cannot hold it: an overflowing bar is visible and a silently
			// missing control is not.
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
