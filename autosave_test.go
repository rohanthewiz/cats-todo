package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// fireTick delivers the tick for the model's current arming, which is what
// tea.Tick would hand back a minute later.
func fireTick(t *testing.T, m model) model {
	t.Helper()
	next, _ := m.Update(autosaveTickMsg{gen: m.autosave.gen})
	return next.(model)
}

// TestAutosaveOpeningIsNotAChange pins the baseline: a form that has only been
// opened, and has had non-editing messages (a resize) since, has nothing to
// save and must not arm the timer.
func TestAutosaveOpeningIsNotAChange(t *testing.T) {
	m, _, _ := newModelInTemp(t)
	next, _ := m.beginAdd()
	m = next.(model)
	next, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(model)
	if !m.autosave.live {
		t.Fatal("autosave not live on an open form")
	}
	if m.autosave.armed {
		t.Error("autosave armed with nothing typed")
	}
}

// TestAutosaveAddThenUpdate walks the main path. The first tick creates the
// todo without leaving the form, the form becomes an edit of it, the second
// tick updates that same todo instead of adding another, and ✔ Save then
// reports the add the user actually made.
func TestAutosaveAddThenUpdate(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	next, _ := m.beginAdd()
	m = next.(model)

	m = typeInto(t, m, "a")
	if !m.autosave.armed {
		t.Fatal("typing did not arm the autosave")
	}
	gen := m.autosave.gen
	// More typing rides on the tick already in flight (a throttle, not a
	// debounce), so the generation must not move.
	m = typeInto(t, m, "b")
	if m.autosave.gen != gen {
		t.Errorf("gen moved from %d to %d on a second key; the throttle re-armed", gen, m.autosave.gen)
	}

	m = fireTick(t, m)
	if m.stage != stageForm {
		t.Fatalf("autosave left the form: stage = %v", m.stage)
	}
	if len(project.todos) != 1 || project.todos[0].Prompt != "ab" {
		t.Fatalf("after first tick todos = %+v, want one todo with prompt \"ab\"", project.todos)
	}
	if m.formMode != formEdit || m.editID != project.todos[0].ID || !m.autosave.added {
		t.Errorf("form not switched to editing the autosaved todo: mode=%v editID=%q added=%v",
			m.formMode, m.editID, m.autosave.added)
	}
	if !strings.HasPrefix(m.formNote, "autosaved ") {
		t.Errorf("formNote = %q, want an autosaved note", m.formNote)
	}

	m = typeInto(t, m, "c")
	if !m.autosave.armed {
		t.Fatal("a change after the first write did not re-arm")
	}
	m = fireTick(t, m)
	if len(project.todos) != 1 || project.todos[0].Prompt != "abc" {
		t.Fatalf("after second tick todos = %+v, want the same todo updated to \"abc\"", project.todos)
	}

	next, _ = m.saveForm()
	m = next.(model)
	if m.stage != stageList || len(project.todos) != 1 {
		t.Fatalf("save: stage = %v, %d todos; want the list and still one todo", m.stage, len(project.todos))
	}
	if !strings.HasPrefix(m.status, "added to ") {
		t.Errorf("status = %q, want an add to be reported as an add", m.status)
	}
}

// TestAutosaveStaleTickIsIgnored is the generation guard. A tick armed by a
// form that has since closed must not write, even though a tea.Tick cannot be
// cancelled and will still arrive.
func TestAutosaveStaleTickIsIgnored(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	next, _ := m.beginAdd()
	m = next.(model)
	m = typeInto(t, m, "x")
	stale := autosaveTickMsg{gen: m.autosave.gen}

	next, _ = m.cancelForm()
	m = next.(model)
	next, _ = m.beginAdd()
	m = next.(model)
	m = typeInto(t, m, "y")

	next, _ = m.Update(stale)
	m = next.(model)
	if len(project.todos) != 0 {
		t.Errorf("a stale tick wrote %+v", project.todos)
	}
}

// TestAutosaveEmptyPromptStaysQuiet: a tick on a form with only a title has
// nothing persistForm would accept. It writes nothing, and it does not put an
// error up unprompted.
func TestAutosaveEmptyPromptStaysQuiet(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	next, _ := m.beginAdd()
	m = next.(model)
	m.titleInput.SetValue("title only")
	next, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40}) // any message arms
	m = next.(model)
	if !m.autosave.armed {
		t.Fatal("a title change did not arm")
	}
	m = fireTick(t, m)
	if len(project.todos) != 0 || m.formErr != "" {
		t.Errorf("empty prompt: todos=%d formErr=%q, want nothing written and no error", len(project.todos), m.formErr)
	}
}

// TestAutosaveCancelDeletesAutosavedAdd: esc still means "keep nothing from
// this form", so a new prompt the timer already wrote comes back out.
func TestAutosaveCancelDeletesAutosavedAdd(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	next, _ := m.beginAdd()
	m = next.(model)
	m = typeInto(t, m, "z")
	m = fireTick(t, m)
	if len(project.todos) != 1 {
		t.Fatalf("setup: %d todos after the tick, want 1", len(project.todos))
	}
	next, _ = m.cancelForm()
	m = next.(model)
	if len(project.todos) != 0 {
		t.Errorf("cancel left the autosaved add: %+v", project.todos)
	}
	if len(m.list.items) != 0 {
		t.Errorf("list still shows %d rows after the revert", len(m.list.items))
	}
}

// TestAutosaveCancelRestoresEdit: cancelling an edit the timer already wrote
// puts back the text, the session options and the marks the todo had when the
// form opened.
func TestAutosaveCancelRestoresEdit(t *testing.T) {
	m, project, _ := newModelInTemp(t)
	orig := Todo{
		ID: "t1", Title: "Orig title", Prompt: "orig prompt",
		Priority: priorityHigh, Session: &SessionOpts{Effort: "high"},
	}
	if err := project.add(orig); err != nil {
		t.Fatal(err)
	}
	m.rebuildList()
	next, _ := m.beginEditRef(todoRef{scope: scopeProject, id: "t1"})
	m = next.(model)

	m = typeInto(t, m, "!")
	m.formAnnots.Priority = priorityCritical
	m.formSession.Effort = "low"
	m = fireTick(t, m)
	got, _ := project.find("t1")
	if got.Prompt == orig.Prompt || got.Priority != priorityCritical || got.Session.Effort != "low" {
		t.Fatalf("setup: autosave did not write the edit: %+v", got)
	}

	next, _ = m.cancelForm()
	m = next.(model)
	got, _ = project.find("t1")
	if got.Title != orig.Title || got.Prompt != orig.Prompt || got.Priority != priorityHigh ||
		got.Session == nil || got.Session.Effort != "high" {
		t.Errorf("cancel did not restore the original: %+v (session %+v)", got, got.Session)
	}
}

// TestAutosaveSetting pins how settings.json's autosaveSeconds is read: a
// missing key is the 45-second default, 0 turns the autosave off (which is why
// the key is a pointer), and a tiny value is raised to the floor instead of
// rewriting the backlog every second.
func TestAutosaveSetting(t *testing.T) {
	cases := []struct {
		name string
		file string
		want time.Duration
	}{
		{"no file", "", defaultAutosave},
		{"key absent", `{"spellcheck": true}`, defaultAutosave},
		{"explicit", `{"autosaveSeconds": 90}`, 90 * time.Second},
		{"zero is off", `{"autosaveSeconds": 0}`, 0},
		{"negative is off", `{"autosaveSeconds": -3}`, 0},
		{"raised to the floor", `{"autosaveSeconds": 1}`, minAutosave},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv(configDirEnvVar, dir)
			if c.file != "" {
				if err := os.WriteFile(filepath.Join(dir, settingsFileName), []byte(c.file), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := loadSettings().autosave; got != c.want {
				t.Errorf("autosave = %v, want %v", got, c.want)
			}
		})
	}
}

// TestAutosaveSettingSurvivesASave: another preference being saved (the View
// panel, the spell toggle) rewrites the whole file, and must write the autosave
// delay back as it was. An "off" in particular must not come back as the
// default.
func TestAutosaveSettingSurvivesASave(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(configDirEnvVar, dir)
	if err := os.WriteFile(filepath.Join(dir, settingsFileName), []byte(`{"autosaveSeconds": 0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := loadSettings()
	s.spellcheck = false
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings().autosave; got != 0 {
		t.Errorf("after an unrelated save autosave = %v, want still off", got)
	}
}

// TestAutosaveOffNeverArms: with the delay at zero the form still opens and
// saves normally, but typing never schedules a write.
func TestAutosaveOffNeverArms(t *testing.T) {
	m, _, _ := newModelInTemp(t)
	if m.autosaveEvery != defaultAutosave {
		t.Fatalf("model delay = %v, want the default %v", m.autosaveEvery, defaultAutosave)
	}
	m.autosaveEvery = 0
	next, _ := m.beginAdd()
	m = next.(model)
	m = typeInto(t, m, "q")
	if m.autosave.armed {
		t.Error("autosave armed while turned off")
	}
}
