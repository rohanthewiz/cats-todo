package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// rightClickAt opens the editor's context menu with a right-click at a column of
// a line of the prompt, counted past the "┃ " gutter.
func rightClickAt(t *testing.T, m model, col, row int) model {
	t.Helper()
	x := promptGutterWidth(m.promptArea) + col
	next, _ := m.Update(tea.MouseClickMsg{X: x, Y: formPromptRow + row, Button: tea.MouseRight})
	got := next.(model)
	if !got.menu.open {
		t.Fatalf("a right-click inside the editor did not open the menu")
	}
	return got
}

// menuLabels is the menu as a reader sees it: each row's label, with a "·"
// prefix on the ones that are drawn dim.
func menuLabels(m model) []string {
	out := make([]string, 0, len(m.menu.items))
	for _, it := range m.menu.items {
		if it.live() {
			out = append(out, it.label)
			continue
		}
		out = append(out, "·"+it.label)
	}
	return out
}

// TestPromptMenuOpensOnTheRightButton: the menu is the right button's whole
// meaning inside the editor now, and it comes up naming every item whether or
// not it can act — an item that vanished between presses is one nobody learns
// the position of.
func TestPromptMenuOpensOnTheRightButton(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "- one\n- two")
	m = selectWholePrompt(t, m)
	m = rightClickAt(t, m, 3, 0)

	if got, want := len(m.menu.items), menuActionCount; got != want {
		t.Fatalf("menu has %d rows, want %d", got, want)
	}
	for i, it := range m.menu.items {
		if it.act != i {
			t.Errorf("row %d holds action %d — the order is what the menu is learned by", i, it.act)
		}
	}
	// With a list swept, the three selection items are all live.
	if got := menuLabels(m); strings.Contains(strings.Join(got[:3], " "), "·") {
		t.Errorf("rows = %q, want split, sort and carets all live over a swept list", got)
	}
	// And the cursor opens on the first row that can act, so enter is never a
	// refusal straight after the click.
	if m.menu.cursor != menuSplit {
		t.Errorf("cursor on row %d, want the split", m.menu.cursor)
	}
}

// TestPromptMenuOutsideTheEditorDoesNothing: the right button means something
// only over the prompt, which is the only place this program draws anything a
// press could be aimed at.
func TestPromptMenuOutsideTheEditorDoesNothing(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "- one\n- two")
	next, _ := m.Update(tea.MouseClickMsg{X: 4, Y: formTitleRow, Button: tea.MouseRight})
	if next.(model).menu.open {
		t.Error("a right-click on the title field opened the editor's menu")
	}
}

// TestPromptMenuDimsWhatCannotAct: every item says what is wrong with the
// selection rather than disappearing, and pressing a dim row puts that on the
// note line.
func TestPromptMenuDimsWhatCannotAct(t *testing.T) {
	for _, tc := range []struct {
		name  string
		body  string
		sweep bool
		row   int
		want  string
	}{
		{name: "nothing swept", body: "- one\n- two", row: menuSplit, want: "sweep some text first"},
		{name: "a sweep with no bullets", body: "just a paragraph", sweep: true, row: menuSplit, want: "no bulleted list"},
		{name: "a single line", body: "- one", sweep: true, row: menuSort, want: "two or more lines"},
		{name: "a single line, carets", body: "- one", sweep: true, row: menuCarets, want: "two or more lines"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, project, _ := splitFormInTemp(t, tc.body)
			if tc.sweep {
				m = selectWholePrompt(t, m)
			} else {
				m.focusForm(formFieldPrompt)
			}
			m = rightClickAt(t, m, 1, 0)
			if m.menu.items[tc.row].live() {
				t.Fatalf("row %d is live, want it dim", tc.row)
			}

			got, _ := m.pressPromptMenu(tc.row)
			after := got.(model)
			if after.menu.open {
				t.Error("the menu stayed up over its own explanation")
			}
			if !strings.Contains(after.formNote, tc.want) {
				t.Errorf("form note = %q, want it to mention %q", after.formNote, tc.want)
			}
			if len(project.todos) != 0 {
				t.Errorf("a dim row wrote %d todos", len(project.todos))
			}
		})
	}
}

// TestPromptMenuKeyboard: ↑/↓ walk the rows, enter presses the one under the
// cursor, and every other key takes the menu down — what a menu does everywhere
// else, so the keystroke after it reaches the editor as usual.
func TestPromptMenuKeyboard(t *testing.T) {
	m, project, _ := splitFormInTemp(t, "- one\n- two")
	m = selectWholePrompt(t, m)
	m = rightClickAt(t, m, 3, 0)

	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.menu.cursor != menuSort {
		t.Fatalf("cursor on row %d after ↓, want the sort", m.menu.cursor)
	}
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.menu.cursor != menuSplit {
		t.Fatalf("cursor on row %d after ↑, want back on the split", m.menu.cursor)
	}
	// ↑ from the top wraps rather than sticking, which is what a four-row menu
	// wants — the last row is one key away from the first.
	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.menu.cursor != menuSpell {
		t.Fatalf("cursor on row %d after wrapping, want the last row", m.menu.cursor)
	}

	m = typeInForm(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = typeInForm(t, m, enterKey(0))
	if m.menu.open {
		t.Error("enter left the menu up")
	}
	if len(project.todos) != 2 {
		t.Errorf("backlog holds %d todos, want the two the split made", len(project.todos))
	}
}

// TestPromptMenuEscapeCloses: esc — and any key that is not a move or a press —
// takes the menu down and is spent doing it, rather than reaching the editor
// underneath.
func TestPromptMenuEscapeCloses(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		{Code: tea.KeyEscape},
		{Code: 'q', Text: "q"},
	} {
		m, _, _ := splitFormInTemp(t, "- one\n- two")
		m = selectWholePrompt(t, m)
		m = rightClickAt(t, m, 3, 0)

		got := typeInForm(t, m, key)
		if got.menu.open {
			t.Errorf("%q left the menu up", key.String())
		}
		if got.promptArea.Value() != "- one\n- two" {
			t.Errorf("%q reached the editor: value = %q", key.String(), got.promptArea.Value())
		}
		if got.stage != stageForm {
			t.Errorf("%q left the form", key.String())
		}
	}
}

// TestPromptMenuClicks: a click on a row presses it, a click on the border is
// inside the menu and presses nothing, and a click anywhere else dismisses.
func TestPromptMenuClicks(t *testing.T) {
	open := func(t *testing.T) model {
		t.Helper()
		m, _, _ := splitFormInTemp(t, "- one\n- two")
		m = selectWholePrompt(t, m)
		return rightClickAt(t, m, 3, 0)
	}
	click := func(m model, x, y int) model {
		next, _ := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
		return next.(model)
	}

	m := open(t)
	// Row 1 of the box's interior is the sort; +1 steps over the top border.
	got := click(m, m.menu.x+2, m.menu.y+1+menuSort)
	if got.menu.open {
		t.Error("a click on a row left the menu up")
	}
	if got.promptArea.Value() != "- one\n- two" {
		t.Errorf("the sort of an already-sorted list changed it: %q", got.promptArea.Value())
	}
	if !strings.Contains(got.formNote, "sorted 2 items") {
		t.Errorf("form note = %q, want the sort's report", got.formNote)
	}

	m = open(t)
	if got := click(m, m.menu.x, m.menu.y); !got.menu.open {
		t.Error("a click on the box's border dismissed the menu")
	}

	m = open(t)
	if got := click(m, 0, formTitleRow); got.menu.open {
		t.Error("a click off the box left the menu up")
	}
}

// TestPromptMenuStaysInsideThePane: the box is placed below-right of the
// pointer and then pulled back inside the pane, so a press near an edge cannot
// put half of it off screen — a menu row nobody can click is worse than one
// drawn on the other side of the pointer.
func TestPromptMenuStaysInsideThePane(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "- one\n- two")
	m = selectWholePrompt(t, m)
	for _, at := range [][2]int{{0, 0}, {m.width - 1, 0}, {m.width - 1, m.height - 1}, {0, m.height - 1}} {
		next, _ := m.Update(tea.MouseClickMsg{X: at[0], Y: formPromptRow, Button: tea.MouseRight})
		got := next.(model)
		got.menu.place(at[0], at[1], m.width, m.height)
		if got.menu.x < 0 || got.menu.x+got.menu.w > m.width {
			t.Errorf("at x=%d the box spans [%d, %d) in a %d-cell pane",
				at[0], got.menu.x, got.menu.x+got.menu.w, m.width)
		}
		if got.menu.y < 0 || got.menu.y+got.menu.h > m.height {
			t.Errorf("at y=%d the box spans [%d, %d) in a %d-row pane",
				at[1], got.menu.y, got.menu.y+got.menu.h, m.height)
		}
	}
}

// TestPromptMenuIsDrawnOverTheForm: the menu floats, so the form is still
// underneath it — a context menu that hid its own context would be asking about
// a selection the user can no longer see. And the frame keeps its line count, so
// nothing the form hit-tests moves while the menu is up.
func TestPromptMenuIsDrawnOverTheForm(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "- one\n- two")
	m = selectWholePrompt(t, m)
	bare := m.viewForm()

	m = rightClickAt(t, m, 3, 0)
	over := m.overlayPromptMenu(m.viewForm())

	if !strings.Contains(over, "Split into prompts") {
		t.Error("the menu is not in the rendered frame")
	}
	if !strings.Contains(over, "New prompt") {
		t.Error("the form's heading is gone — the menu replaced its context instead of floating over it")
	}
	if got, want := strings.Count(over, "\n"), strings.Count(bare, "\n"); got != want {
		t.Errorf("frame is %d lines with the menu up, %d without", got+1, want+1)
	}
}
