package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// ctrlX is the split chord as a terminal reports it.
var ctrlX = tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl}

// splitFormInTemp opens the add form over real temp stores, sized wide enough
// that nothing in the editor soft-wraps — the split works on logical lines, and
// a wrap in the fixture would only make a failure harder to read.
func splitFormInTemp(t *testing.T, prompt string) (model, *store, *store) {
	t.Helper()
	m, project, global := newModelInTemp(t)
	m.width, m.height = 120, 40
	next, _ := m.beginAdd()
	m = next.(model)
	m.promptArea.SetValue(prompt)
	return m, project, global
}

// selectPromptRange sweeps [lo, hi) in the editor the way shift+→ would: an
// anchor dropped at lo, the caret walked to hi.
func selectPromptRange(t *testing.T, m model, lo, hi int) model {
	t.Helper()
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, lo)
	m.anchorPromptSel()
	setPromptCaretOffset(&m.promptArea, hi)
	if _, _, ok := m.promptSelSpan(); !ok {
		t.Fatalf("nothing selected for [%d, %d) — the fixture is wrong, not the code", lo, hi)
	}
	return m
}

// selectWholePrompt sweeps the entire body, which is the case that replaces the
// prompt rather than editing it.
func selectWholePrompt(t *testing.T, m model) model {
	t.Helper()
	return selectPromptRange(t, m, 0, len([]rune(m.promptArea.Value())))
}

func promptsOf(s *store) []string {
	out := make([]string, 0, len(s.todos))
	for _, t := range s.todos {
		out = append(out, t.Prompt)
	}
	return out
}

// TestSplitBulletList is the parser on its own: what counts as an item, what
// counts as part of the item above it, and where the consumed run starts.
func TestSplitBulletList(t *testing.T) {
	for _, tc := range []struct {
		name  string
		in    string
		head  string // the run before the first bullet, which is never consumed
		items []string
	}{
		{
			name:  "the three unordered markers are all lists",
			in:    "- dash\n* star\n+ plus",
			items: []string{"dash", "star", "plus"},
		},
		{
			name:  "ordered items, both spellings of the marker",
			in:    "1. first\n2) second",
			items: []string{"first", "second"},
		},
		{
			name:  "a nested list is the detail of its parent, not an item",
			in:    "- write the notes\n  - link the diff\n    - and the tag\n- announce it",
			items: []string{"write the notes\n- link the diff\n  - and the tag", "announce it"},
		},
		{
			name:  "a plain line under an item continues it",
			in:    "- write the notes\n  see the changelog\n- announce it",
			items: []string{"write the notes\nsee the changelog", "announce it"},
		},
		{
			name:  "blank lines between items are not part of either",
			in:    "- one\n\n- two\n",
			items: []string{"one", "two"},
		},
		{
			name:  "text above the first bullet is the head, and stays",
			in:    "Ship the release:\n- tag it\n- announce it",
			head:  "Ship the release:\n",
			items: []string{"tag it", "announce it"},
		},
		{
			name:  "an indented list is still a list, and is dedented into its prompts",
			in:    "    - one\n      more\n    - two",
			items: []string{"one\nmore", "two"},
		},
		{
			name:  "tabs indent too, and a tab is four columns",
			in:    "- parent\n\t- child",
			items: []string{"parent\n- child"},
		},
		{
			name: "no bullets at all: nothing to split, and the whole run is head",
			in:   "just a paragraph\nand another line",
			head: "just a paragraph\nand another line",
		},
		{
			name: "a horizontal rule is not a bullet",
			in:   "---\n***",
			head: "---\n***",
		},
		{
			name:  "a bare marker is an empty item and is dropped",
			in:    "- \n- real",
			items: []string{"real"},
		},
		{
			name: "a date is not an ordered item — the digit run has to be short",
			in:   "2024-08-29 was the deadline",
			head: "2024-08-29 was the deadline",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			head, items := splitBulletList(tc.in)
			if want := len([]rune(tc.head)); head != want {
				t.Errorf("head = %d runes (%q), want %d (%q)",
					head, string([]rune(tc.in)[:head]), want, tc.head)
			}
			bodies := make([]string, len(items))
			for i, it := range items {
				bodies[i] = it.body
			}
			if len(bodies) != len(tc.items) {
				t.Fatalf("got %d items %q, want %d %q", len(bodies), bodies, len(tc.items), tc.items)
			}
			for i := range bodies {
				if bodies[i] != tc.items[i] {
					t.Errorf("item %d = %q, want %q", i, bodies[i], tc.items[i])
				}
			}
			// The marker each body came in is kept verbatim, which is what lets
			// the sort put a body back where it was (see sortPromptLines).
			for _, it := range items {
				if got := it.render(); !strings.Contains(tc.in, strings.SplitN(got, "\n", 2)[0]) {
					t.Errorf("rendered %q is not a line of the input", got)
				}
			}
		})
	}
}

// TestSplitPromptListWholeBodyLeavesTheForm: the list was everything the prompt
// held, so there is nothing left to be a prompt — the items land in the backlog
// and the editor is done with.
func TestSplitPromptListWholeBodyLeavesTheForm(t *testing.T) {
	m, project, _ := splitFormInTemp(t, "- tag v2\n- write the notes\n- announce it")
	m = selectWholePrompt(t, m)

	got := typeInForm(t, m, ctrlX)
	if got.stage != stageList {
		t.Errorf("stage = %v, want stageList — the whole body became prompts", got.stage)
	}
	want := []string{"tag v2", "write the notes", "announce it"}
	if p := promptsOf(project); len(p) != 3 || p[0] != want[0] || p[2] != want[2] {
		t.Fatalf("backlog holds %q, want %q", p, want)
	}
	// Titles are derived the way every other blank-titled prompt's is.
	if project.todos[1].Title != "write the notes" {
		t.Errorf("title = %q, want it derived from the bullet", project.todos[1].Title)
	}
	if !strings.Contains(got.status, "3 prompts") {
		t.Errorf("status = %q, want it to report three prompts", got.status)
	}
	// And it is on disk, not only in memory: the split writes immediately rather
	// than waiting for a save the user will never make.
	fresh := &store{scope: scopeProject, path: project.path}
	if err := fresh.load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(fresh.todos) != 3 {
		t.Errorf("disk holds %d todos, want 3", len(fresh.todos))
	}
}

// TestSplitPromptListWholeBodyDeletesTheOriginal: in edit mode the prompt the
// list came out of is replaced by its pieces, not kept beside them.
func TestSplitPromptListWholeBodyDeletesTheOriginal(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	m.width, m.height = 120, 40
	if err := project.add(Todo{ID: "t1", Title: "the plan", Prompt: "- one\n- two"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m.rebuildList()

	next, _ := m.beginEditRef(todoRef{scope: scopeProject, id: "t1"})
	m = selectWholePrompt(t, next.(model))

	got := typeInForm(t, m, ctrlX)
	if got.stage != stageList {
		t.Errorf("stage = %v, want stageList", got.stage)
	}
	if p := promptsOf(project); len(p) != 2 || p[0] != "one" || p[1] != "two" {
		t.Errorf("backlog holds %q, want the two bullets and nothing else", p)
	}
	if _, ok := project.find("t1"); ok {
		t.Error("the original is still in the backlog — the list replaced it, it did not join it")
	}
}

// TestSplitPromptListPartialKeepsTheForm: only the list was swept, so the rest
// of the prompt is still being written — the items are written out and the
// editor keeps what was not part of the list.
func TestSplitPromptListPartialKeepsTheForm(t *testing.T) {
	body := "Ship the release:\n- tag v2\n- announce it"
	m, project, _ := splitFormInTemp(t, body)
	// From the head's start through the end, so the head is inside the selection
	// and must survive anyway.
	m = selectWholePrompt(t, m)

	got := typeInForm(t, m, ctrlX)
	if got.stage != stageForm {
		t.Fatalf("stage = %v, want stageForm — text survived the split", got.stage)
	}
	if v := got.promptArea.Value(); v != "Ship the release:\n" {
		t.Errorf("editor holds %q, want only the line above the list", v)
	}
	if p := promptsOf(project); len(p) != 2 || p[0] != "tag v2" {
		t.Errorf("backlog holds %q, want the two bullets", p)
	}
	if _, _, ok := got.promptSelSpan(); ok {
		t.Error("the selection outlived the run it named")
	}
	if !strings.Contains(got.formNote, "2 prompts") {
		t.Errorf("form note = %q, want it to report two prompts", got.formNote)
	}
}

// TestSplitPromptListLandsBehindTheOriginal: the array order is the user's
// order, so prompts born from an item belong where that item was — not at the
// far end of the backlog.
func TestSplitPromptListLandsBehindTheOriginal(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	m.width, m.height = 120, 40
	for _, td := range []Todo{
		{ID: "a", Title: "first", Prompt: "first"},
		{ID: "b", Title: "the plan", Prompt: "- one\n- two"},
		{ID: "c", Title: "last", Prompt: "last"},
	} {
		if err := project.add(td); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	m.rebuildList()

	next, _ := m.beginEditRef(todoRef{scope: scopeProject, id: "b"})
	m = selectWholePrompt(t, next.(model))
	typeInForm(t, m, ctrlX)

	want := []string{"first", "one", "two", "last"}
	got := promptsOf(project)
	if len(got) != len(want) {
		t.Fatalf("backlog = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("backlog = %q, want %q", got, want)
		}
	}
}

// TestSplitPromptListInheritsHowItRuns: the annotations and the session options
// are the same for every bullet of one list — they say how the work should run,
// and the list is one piece of work being cut up.
func TestSplitPromptListInheritsHowItRuns(t *testing.T) {
	m, project, _ := splitFormInTemp(t, "- one\n- two")
	m.formAnnots = annots{Priority: priorityHigh, Fruit: true}
	m.formSession = SessionOpts{Model: "opus", Effort: "high"}
	m = selectWholePrompt(t, m)

	typeInForm(t, m, ctrlX)
	if len(project.todos) != 2 {
		t.Fatalf("backlog holds %d todos, want 2", len(project.todos))
	}
	for _, td := range project.todos {
		if td.Priority != priorityHigh || !td.Fruit {
			t.Errorf("%q: annotations = %q/%v, want high/true", td.Prompt, td.Priority, td.Fruit)
		}
		if td.Session == nil || td.Session.Model != "opus" || td.Session.Effort != "high" {
			t.Errorf("%q: session = %+v, want the form's options", td.Prompt, td.Session)
		}
	}
	// Clones, not one shared record: editing one prompt's options later must not
	// reach into its siblings'.
	if project.todos[0].Session == project.todos[1].Session {
		t.Error("both prompts point at the same SessionOpts")
	}
}

// TestSplitPromptListRefusesInWords covers every way the chord can not apply.
// Each one says why: a key that quietly does nothing on a screen the user is
// typing into reads as a broken editor.
func TestSplitPromptListRefusesInWords(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T) model
		want  string
	}{
		{
			name: "nothing selected",
			setup: func(t *testing.T) model {
				m, _, _ := splitFormInTemp(t, "- one\n- two")
				m.focusForm(formFieldPrompt)
				return m
			},
			want: "nothing selected",
		},
		{
			name: "a selection with no bullets in it",
			setup: func(t *testing.T) model {
				m, _, _ := splitFormInTemp(t, "just a paragraph")
				return selectWholePrompt(t, m)
			},
			want: "no bulleted list",
		},
		{
			name: "the chord pressed from the title",
			setup: func(t *testing.T) model {
				m, _, _ := splitFormInTemp(t, "- one\n- two")
				m.focusForm(formFieldTitle)
				return m
			},
			want: "works in the prompt",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := typeInForm(t, tc.setup(t), ctrlX)
			if got.stage != stageForm {
				t.Errorf("stage = %v, want the form still open", got.stage)
			}
			if !strings.Contains(got.formNote, tc.want) {
				t.Errorf("form note = %q, want it to mention %q", got.formNote, tc.want)
			}
		})
	}
}

// TestSplitPromptListLeavesTheSelectionOnARefusal: a refused split keeps the
// highlight, so the run the user swept is still there to be re-swept or
// extended. Only a split that happened consumes it.
func TestSplitPromptListLeavesTheSelectionOnARefusal(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "just a paragraph")
	m = selectWholePrompt(t, m)

	got := typeInForm(t, m, ctrlX)
	if _, _, ok := got.promptSelSpan(); !ok {
		t.Error("the selection was dropped by a split that did not happen")
	}
}

// TestSplitPromptListInTheGlobalBacklog: the prompts follow the form's scope,
// which is the backlog the list was going to be saved into.
func TestSplitPromptListInTheGlobalBacklog(t *testing.T) {
	m, project, global := splitFormInTemp(t, "- one\n- two")
	m.formScope = scopeGlobal
	m = selectWholePrompt(t, m)

	typeInForm(t, m, ctrlX)
	if len(global.todos) != 2 {
		t.Errorf("global backlog holds %d todos, want 2", len(global.todos))
	}
	if len(project.todos) != 0 {
		t.Errorf("project backlog holds %d todos, want none", len(project.todos))
	}
}

// TestCtrlXStillHasNoMeaningInTheTextarea is the regression this chord was
// picked to avoid: ctrl+x is not one of the textarea's own bindings, so binding
// the split over it cannot have taken an editing key away. If the library ever
// claims it, this fails here rather than in a user's prompt.
func TestCtrlXStillHasNoMeaningInTheTextarea(t *testing.T) {
	m, _, _ := splitFormInTemp(t, "alpha")
	m.focusForm(formFieldPrompt)
	setPromptCaretOffset(&m.promptArea, 5)

	got := typeInForm(t, m, ctrlX)
	if got.promptArea.Value() != "alpha" {
		t.Errorf("value = %q, want it untouched by a split that had nothing to do",
			got.promptArea.Value())
	}
}

// TestSplitPromptListTakesOnlyTheSweptItems: the split is bounded by the
// selection, not by the list — sweeping two bullets of four leaves the other two
// in the editor.
func TestSplitPromptListTakesOnlyTheSweptItems(t *testing.T) {
	body := "- one\n- two\n- three\n- four"
	m, project, _ := splitFormInTemp(t, body)
	// Through the end of "- two", which is offset 12: "- one\n" (6) + "- two" (5)
	// leaves the newline after it outside the sweep.
	m = selectPromptRange(t, m, 0, 11)

	got := typeInForm(t, m, ctrlX)
	if got.stage != stageForm {
		t.Fatalf("stage = %v, want stageForm — two bullets survived", got.stage)
	}
	if v := got.promptArea.Value(); v != "\n- three\n- four" {
		t.Errorf("editor holds %q, want the two bullets that were not swept", v)
	}
	if p := promptsOf(project); len(p) != 2 || p[0] != "one" || p[1] != "two" {
		t.Errorf("backlog holds %q, want the two swept bullets", p)
	}
}

// TestSplitFooterTeachesTheMenuWhileSomethingIsSwept: the caret line is full —
// its standing segments come to exactly the 118 cells that let the field switch
// survive a 120-cell pane — so the menu's segment is contextual. It appears the
// moment there is a run to act on, and stays out of the way when there is not.
//
// It names the menu rather than ctrl+x: the menu prints that chord on its own ✂
// row, so one gesture on the footer teaches every key behind it.
func TestSplitFooterTeachesTheMenuWhileSomethingIsSwept(t *testing.T) {
	plain, _, _ := splitFormInTemp(t, "- one\n- two")
	plain.width = 200
	plain.focusForm(formFieldPrompt)
	if foot := plain.formFooter(); strings.Contains(foot, "right-click") {
		t.Errorf("footer names the menu with nothing swept:\n%s", foot)
	}

	swept := selectWholePrompt(t, plain)
	if foot := swept.formFooter(); !strings.Contains(foot, "right-click: split/sort/carets") {
		t.Errorf("footer does not teach the menu over a sweep:\n%s", foot)
	}
	if foot := swept.formFooter(); strings.Contains(foot, "ctrl+x") {
		t.Errorf("footer spends a segment on the chord the menu already prints:\n%s", foot)
	}
}
