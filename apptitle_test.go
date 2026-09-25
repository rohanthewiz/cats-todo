package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestAppTitleLeadsBothScreens pins the program title to the first line of the
// list and of the form, naming the section each one is. The row constants of
// both screens are counted past it (appTitleLines), so a title that went
// missing — or moved below the header — would aim every click one row off.
func TestAppTitleLeadsBothScreens(t *testing.T) {
	list := newTestModel()
	list.width = 100
	first := strings.Split(list.viewList(), "\n")[appTitleRow]
	if want := "CatsTodo v" + version + " - Prompts"; ansi.Strip(first) != want {
		t.Errorf("the list's first line is %q, want %q", ansi.Strip(first), want)
	}

	form := withForm(t, "a title", "body", 100, 40)
	first = strings.Split(form.viewForm(), "\n")[appTitleRow]
	if want := "CatsTodo v" + version + " - Prompt Editor"; ansi.Strip(first) != want {
		t.Errorf("the form's first line is %q, want %q", ansi.Strip(first), want)
	}
}

// TestAppTitleNeverWraps holds the title to one line in a pane too narrow for
// it: appTitleLines is a constant every hit-test below the title depends on.
func TestAppTitleNeverWraps(t *testing.T) {
	m := newTestModel()
	m.width = 12
	line := m.titleLine("Prompt Editor")
	if strings.Contains(line, "\n") || lipgloss.Width(line) > m.width {
		t.Errorf("title %q is %d cells in a %d-cell pane", ansi.Strip(line), lipgloss.Width(line), m.width)
	}
}

// openAddForm is a form over real temp stores, sized for clicks, with the
// given prompt typed in — the fixture the title-click tests share.
func openAddForm(t testing.TB, title, prompt string) (model, *store) {
	t.Helper()
	m, project, _ := newModelInTemp(t)
	m.width, m.height = 100, 40
	next, _ := m.beginAdd()
	m = next.(model)
	m.titleInput.SetValue(title)
	m.promptArea.SetValue(prompt)
	return m, project
}

// TestClickingFormTitleSavesAndGoesHome: a click on the editor's program title
// returns to the Prompts list with the edit kept, the same write ✔ Save makes.
func TestClickingFormTitleSavesAndGoesHome(t *testing.T) {
	m, project := openAddForm(t, "keep me", "the prompt body")
	m = clickForm(m, 3, appTitleRow)
	if m.stage != stageList {
		t.Fatalf("stage after clicking the title = %v, want stageList", m.stage)
	}
	if len(project.todos) != 1 || project.todos[0].Prompt != "the prompt body" {
		t.Errorf("project todos = %+v, want the typed prompt saved", project.todos)
	}
}

// TestEscFromFormDiscards is the other half of the contract the title click is
// defined against: esc leaves the editor without writing anything.
func TestEscFromFormDiscards(t *testing.T) {
	m, project := openAddForm(t, "drop me", "never saved")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)
	if m.stage != stageList {
		t.Fatalf("stage after esc = %v, want stageList", m.stage)
	}
	if len(project.todos) != 0 {
		t.Errorf("project todos = %+v, want nothing saved on esc", project.todos)
	}
}

// TestClickingFormTitleOnEmptyPrompt: the save's own refusal still applies —
// a title with no prompt keeps the form open and says why — but a new form
// with nothing in it at all simply goes home, since there is nothing to keep.
func TestClickingFormTitleOnEmptyPrompt(t *testing.T) {
	m, project := openAddForm(t, "title only", "")
	m = clickForm(m, 3, appTitleRow)
	if m.stage != stageForm || m.formErr == "" {
		t.Errorf("stage = %v, formErr = %q; want the form kept open with the empty-prompt refusal", m.stage, m.formErr)
	}

	m, project = openAddForm(t, "", "")
	m = clickForm(m, 3, appTitleRow)
	if m.stage != stageList {
		t.Errorf("stage after clicking the title on a blank form = %v, want stageList", m.stage)
	}
	if len(project.todos) != 0 {
		t.Errorf("project todos = %+v, want nothing saved from a blank form", project.todos)
	}
}
