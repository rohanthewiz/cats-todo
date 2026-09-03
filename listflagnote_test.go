package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// onAPane gives a test model a pane big enough for a floating box. The pad
// measures itself against the pane (see beginFlagNote) and refuses to open in
// one too small, so a test about the pad has to say how big the terminal is —
// withTodos sets a width and leaves the height at zero, which is the fallback
// case rather than this one.
func onAPane(m model) model {
	m.width, m.height = 100, 30
	return m
}

// typeInPad sends a string to the open pad one keystroke at a time, the way a
// terminal reports typing.
func typeInPad(t *testing.T, m model, s string) model {
	t.Helper()
	for _, r := range s {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(model)
	}
	return m
}

// flagFromMenu right-clicks a row and presses ⚑ Flag, which is the gesture the
// pad exists for.
func flagFromMenu(t *testing.T, m model, row int) model {
	t.Helper()
	next, _ := rightClickRow(t, m, row).pressListMenu(listMenuFlag)
	return next.(model)
}

// TestFlagNotePadOpensOnTheFlag: raising the flag from the list's context menu
// asks for the note right there rather than pointing at the edit form. The mark
// is saved by the press itself, so the pad is an invitation and never a gate —
// which is what makes escaping it safe.
func TestFlagNotePadOpensOnTheFlag(t *testing.T) {
	t.Run("the mark lands before the pad opens", func(t *testing.T) {
		m := flagFromMenu(t, onAPane(withTodos(t, "the prompt")), 0)

		if !m.flagPad.open {
			t.Fatal("the flag row did not open the note pad")
		}
		// On disk already: the press said "flag this", and that answer stands
		// whatever the pad's next keystroke turns out to be.
		td, _ := m.project.find("a")
		if !td.Flag {
			t.Fatal("the flag was not saved before the pad opened")
		}
		if m.listMenu.open {
			t.Error("the menu is still up behind the pad")
		}
	})

	t.Run("enter saves what was typed", func(t *testing.T) {
		m := flagFromMenu(t, onAPane(withTodos(t, "the prompt")), 0)
		m = typeInPad(t, m, "blocked on the api")
		next, _ := m.Update(pressKey("enter"))
		got := next.(model)

		td, _ := got.project.find("a")
		if !td.Flag || td.FlagNote != "blocked on the api" {
			t.Fatalf("todo = %+v, want the flag up and the note saved", td)
		}
		if got.flagPad.open {
			t.Error("the pad is still up after saving")
		}
		if !strings.Contains(got.status, "note") {
			t.Errorf("status = %q, want the save named", got.status)
		}
	})

	// The whole point of raising the mark first: walking away costs the words,
	// never the flag, and the status says exactly what stands.
	t.Run("esc leaves the flag bare", func(t *testing.T) {
		m := flagFromMenu(t, onAPane(withTodos(t, "the prompt")), 0)
		m = typeInPad(t, m, "never mind")
		next, _ := m.Update(pressKey("esc"))
		got := next.(model)

		td, _ := got.project.find("a")
		if !td.Flag || td.FlagNote != "" {
			t.Fatalf("todo = %+v, want a bare flag", td)
		}
		if got.flagPad.open {
			t.Error("esc left the pad up")
		}
		if !strings.Contains(got.status, "flagged") {
			t.Errorf("status = %q, want the standing mark named", got.status)
		}
	})

	// A note is trimmed on the way in, and a field left empty is an answer too:
	// the mark stays, the words do not.
	t.Run("an empty note keeps the mark", func(t *testing.T) {
		m := flagFromMenu(t, onAPane(withTodos(t, "the prompt")), 0)
		m = typeInPad(t, m, "   ")
		next, _ := m.Update(pressKey("enter"))

		td, _ := next.(model).project.find("a")
		if !td.Flag || td.FlagNote != "" {
			t.Fatalf("todo = %+v, want the flag standing with no words", td)
		}
	})

	// A pane with no room for a floating box falls back to the behaviour the
	// pad replaced: the flag is still raised, and the status names the road to
	// the words. withTodos leaves the height at zero, which is that pane.
	t.Run("a pane too small falls back to the status line", func(t *testing.T) {
		m := flagFromMenu(t, withTodos(t, "the prompt"), 0)

		if m.flagPad.open {
			t.Fatal("the pad opened on a pane with no room for it")
		}
		td, _ := m.project.find("a")
		if !td.Flag {
			t.Fatal("the flag was not raised")
		}
		if !strings.Contains(m.status, "note") {
			t.Errorf("status = %q, want the note's road named", m.status)
		}
	})
}

// TestFlagNotePadOwnsTheKeys: the pad is a field, so while it is up every key
// is its own. A list chord fired from inside it would act on a row while the
// hand is typing a sentence about that row.
func TestFlagNotePadOwnsTheKeys(t *testing.T) {
	m := flagFromMenu(t, onAPane(withTodos(t, "the prompt")), 0)

	// ctrl+t is "mark done" on the list and nothing at all in a text field.
	next, _ := m.Update(pressKey("ctrl+t"))
	got := next.(model)
	if td, _ := got.project.find("a"); td.Done {
		t.Error("a list chord acted on the row from inside the note pad")
	}
	if !got.flagPad.open {
		t.Fatal("the pad closed on a key that was not a way out")
	}

	// And a printable character lands in the field rather than in the list's
	// query box behind it.
	got = typeInPad(t, got, "why")
	if got.flagPad.input.Value() != "why" {
		t.Errorf("field = %q, want the typed note", got.flagPad.input.Value())
	}
	if q := got.list.input.Value(); q != "" {
		t.Errorf("the list's filter took the keys too: %q", q)
	}
}

// TestFlagNoteRow pins the menu's ✎ Flag note… row: dim until there is a mark
// for the words to belong to, saying so in words, and the way back into a note
// that already exists.
func TestFlagNoteRow(t *testing.T) {
	t.Run("dim on an unflagged prompt", func(t *testing.T) {
		m := rightClickRow(t, onAPane(withTodos(t, "the prompt")), 0)
		row := m.listMenu.items[listMenuFlagNote]
		if row.live() {
			t.Fatal("the note row is live on a prompt with no flag")
		}
		if !strings.Contains(row.why, "Flag") {
			t.Errorf("why = %q, want the row that raises the mark named", row.why)
		}
		// Pressing it says why rather than doing nothing (contract 4).
		next, _ := m.pressListMenu(listMenuFlagNote)
		if got := next.(model); got.flagPad.open || got.status == "" {
			t.Error("a dim note row opened a pad, or refused in silence")
		}
	})

	t.Run("opens the pad on the note already there", func(t *testing.T) {
		m := onAPane(withTodos(t, "the prompt"))
		markTodo(t, m.project, "a", false, false, annots{Flag: true, FlagNote: "blocked on the api"})
		m.rebuildList()
		m = rightClickRow(t, m, 0)

		if got := m.listMenu.items[listMenuFlagNote].label; !strings.Contains(got, "Edit") {
			t.Errorf("note row = %q, want the label to say the note is being edited", got)
		}
		next, _ := m.pressListMenu(listMenuFlagNote)
		got := next.(model)
		if !got.flagPad.open {
			t.Fatal("the note row did not open the pad")
		}
		if got.flagPad.input.Value() != "blocked on the api" {
			t.Errorf("field = %q, want the current note to edit", got.flagPad.input.Value())
		}

		// Escaping a pad that raised nothing changed nothing, and says so.
		next, _ = got.Update(pressKey("esc"))
		if td, _ := next.(model).project.find("a"); td.FlagNote != "blocked on the api" {
			t.Errorf("note = %q, want the old words untouched", td.FlagNote)
		}
	})
}

// TestFlagNotePadPointer: a click off the pad dismisses it, the way a click off
// any floating box in this program means "never mind" — and the list must not
// also act on that press.
func TestFlagNotePadPointer(t *testing.T) {
	m := flagFromMenu(t, onAPane(withTodos(t, "the prompt", "another")), 0)
	pad := m.flagPad

	next, _ := m.Update(tea.MouseClickMsg{X: pad.x + pad.w + 4, Y: pad.y + pad.h + 4, Button: tea.MouseLeft})
	got := next.(model)
	if got.flagPad.open {
		t.Fatal("a click off the pad left it up")
	}
	if td, _ := got.project.find("a"); !td.Flag {
		t.Error("dismissing the pad took the mark with it")
	}

	// A right-click is the same press as far as a floating box is concerned: it
	// takes the pad down rather than opening a menu behind it.
	m = flagFromMenu(t, onAPane(withTodos(t, "the prompt")), 0)
	next, _ = m.Update(tea.MouseClickMsg{X: 8, Y: listRowsRow, Button: tea.MouseRight})
	got = next.(model)
	if got.flagPad.open || got.listMenu.open {
		t.Error("a right-click while the pad was up did not simply dismiss it")
	}
}

// TestFlagNotePadSurvivesAResize: the pad holds words someone is in the middle
// of typing, so a resize re-places it instead of dropping it — unlike the menus,
// which are transient answers with nothing in them to lose.
func TestFlagNotePadSurvivesAResize(t *testing.T) {
	m := flagFromMenu(t, onAPane(withTodos(t, "the prompt")), 0)
	m = typeInPad(t, m, "half a sentence")

	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	got := next.(model)
	if !got.flagPad.open {
		t.Fatal("a resize threw away the note being typed")
	}
	if got.flagPad.input.Value() != "half a sentence" {
		t.Errorf("field = %q, want the words kept", got.flagPad.input.Value())
	}
	if got.flagPad.x+got.flagPad.w > 60 || got.flagPad.y+got.flagPad.h > 20 {
		t.Errorf("pad at (%d,%d) %dx%d is outside the new pane", got.flagPad.x, got.flagPad.y, got.flagPad.w, got.flagPad.h)
	}
}

// TestFlagNotePadDrawsAnOpaqueBox: the pad is composited over the list, so
// every cell inside its border has to be painted — a short row left unpadded
// would let the prompt underneath show through the note being typed. The
// rendered size has to be the size the box hit-tests against, too, or a click
// on the field would land a row off it.
func TestFlagNotePadDrawsAnOpaqueBox(t *testing.T) {
	m := flagFromMenu(t, onAPane(withTodos(t, "Fix the drop timeout")), 0)
	m = typeInPad(t, m, "blocked on the api")

	out := m.flagPad.render()
	if w, h := lipgloss.Width(out), lipgloss.Height(out); w != m.flagPad.w || h != m.flagPad.h {
		t.Fatalf("the pad rendered %dx%d, but hit-tests as %dx%d", w, h, m.flagPad.w, m.flagPad.h)
	}
	for _, want := range []string{"Fix the drop timeout", "blocked on the api", flagPadHint} {
		if !strings.Contains(out, want) {
			t.Errorf("the pad does not carry %q", want)
		}
	}
	// And it is on screen: the overlay puts it over the list rather than beside
	// it, which is the only thing the list stage's renderer does with it.
	if !strings.Contains(m.overlayFlagNote(m.viewList()), "blocked on the api") {
		t.Error("the pad was not composited over the list")
	}
}
