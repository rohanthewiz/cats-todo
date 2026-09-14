package main

import (
	"strings"
	"testing"

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
