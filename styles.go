package main

import "charm.land/lipgloss/v2"

// Palette — a small, cohesive set of styles for a clean dark-terminal look,
// shared by the fuzzyList component and the manager views.
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#11111B")).
			Background(lipgloss.Color("#7AA2F7")).
			Padding(0, 1)

	promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7AA2F7")).Bold(true)
	countStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))

	nameStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
	nameSelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	descStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	matchStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F2A900")).Bold(true)
	barStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#7AA2F7")).Bold(true)
	footerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563"))
	headingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9D7CD8")).Bold(true)

	// Todo-specific accents. checkStyle is deliberately unbolded: a completed
	// todo should recede, so the green tick reads as a quiet marker rather than
	// competing with the open rows above it for attention.
	doneStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563")).Strikethrough(true)
	checkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ECE6A"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F7768E"))
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ECE6A"))

	// Action-bar buttons. Each chip is rendered by exactly one of these styles —
	// label and key hint together — because nesting a second style inside a
	// chip would let the outer reset clobber the inner one (see the badge note
	// in fuzzylist.view). btnOffStyle marks a button whose action needs a
	// highlighted todo when there isn't one: still readable, plainly inert.
	btnStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#C0CAF5")).Background(lipgloss.Color("#2A2F3A")).Padding(0, 1)
	btnFocusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#11111B")).Background(lipgloss.Color("#7AA2F7")).Bold(true).Padding(0, 1)
	btnOffStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563")).Background(lipgloss.Color("#22262C")).Padding(0, 1)
)
