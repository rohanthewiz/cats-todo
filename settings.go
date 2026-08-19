// The manager's persisted preferences — the handful of toggles that should
// survive a relaunch, kept apart from the backlogs they are not about.
//
// There is one file, <config>/settings.json in the global config directory
// (see configBaseDir), and it holds preferences rather than data: nothing in it
// is a todo, and losing it costs a keystroke, not work. Absent keys mean their
// defaults, so the file is only ever as long as the things someone has changed,
// and a version that learns a new preference reads an old file unchanged.
package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// settingsFileName is the preferences file's name inside configBaseDir.
const settingsFileName = "settings.json"

// settings is the resolved preferences: every field has its default applied,
// so callers read it without caring whether the file said anything.
type settings struct {
	// spellcheck is whether the prompt editor underlines words its dictionary
	// does not know (see spell.go). On by default: the checker is quiet enough
	// on prose that the people who want it off are the ones with a reason
	// (another language, mostly code) and a chord to say so.
	spellcheck bool
	// orderByPriority is whether the list is sorted by priority rather than
	// shown in the backlog's own order (see rebuildList). Off by default: the
	// order in the file is the one the user set by hand, and a manager that
	// opened having quietly rearranged it would be answering a question nobody
	// asked. It persists because it is a way of working rather than a glance —
	// someone who triages by priority does it every session.
	orderByPriority bool
	// showFrozen is whether "will not do" prompts are drawn at all. On by
	// default, which is what the list has always done. Its own preference
	// rather than a half of the ctrl+d fold because the two are different
	// questions: hiding what is finished tidies the pile, while hiding what was
	// declined is a standing decision about whether that record is worth the
	// rows.
	showFrozen bool
}

// defaultSettings is what a missing or empty file means.
func defaultSettings() settings {
	return settings{spellcheck: true, orderByPriority: false, showFrozen: true}
}

// settingsFile is the on-disk shape. Pointers, so that "not mentioned" and
// "set to the default" are different things when reading — a bool alone would
// read a missing key as false, and every default-true preference would need
// its sense inverted to survive that.
type settingsFile struct {
	Spellcheck      *bool `json:"spellcheck,omitempty"`
	OrderByPriority *bool `json:"orderByPriority,omitempty"`
	ShowFrozen      *bool `json:"showFrozen,omitempty"`
}

// settingsPath is where the file lives, or "" when the config directory
// cannot be resolved (no home directory) — in which case preferences simply
// do not persist for this run, and the caller carries on with defaults.
func settingsPath() string {
	base, err := configBaseDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, settingsFileName)
}

// loadSettings reads the file, applying defaults for anything it does not
// say. Every failure resolves to the defaults rather than an error: a
// preferences file that cannot be read is not a reason to refuse to run, and
// the one message that would matter — "your toggle did not stick" — is
// delivered where the toggle is made, when saving fails.
func loadSettings() settings {
	s := defaultSettings()
	path := settingsPath()
	if path == "" {
		return s
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var f settingsFile
	if json.Unmarshal(data, &f) != nil {
		return s
	}
	if f.Spellcheck != nil {
		s.spellcheck = *f.Spellcheck
	}
	if f.OrderByPriority != nil {
		s.orderByPriority = *f.OrderByPriority
	}
	if f.ShowFrozen != nil {
		s.showFrozen = *f.ShowFrozen
	}
	return s
}

// save writes the preferences back. It writes every field, default or not:
// once someone has touched a toggle the file is the record of what they
// chose, and "spellcheck: true" written out is what lets a later change of
// default leave their choice alone.
func (s settings) save() error {
	path := settingsPath()
	if path == "" {
		return errors.New("no config directory to save settings in")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f := settingsFile{
		Spellcheck:      &s.spellcheck,
		OrderByPriority: &s.orderByPriority,
		ShowFrozen:      &s.showFrozen,
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	// Write-then-rename, so a crash mid-write leaves the old file rather than
	// a truncated one — the same shape store.save uses for the backlogs.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
