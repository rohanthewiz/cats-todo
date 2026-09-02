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
	// peerToken is the shared secret the LAN service demands on every request
	// (see peer.go). Empty until the first `cats-todo serve`, which generates
	// one and prints it — the two machines have to hold the same string, and
	// the honest way to arrange that is for a person to copy it across.
	peerToken string
	// peerName is what this machine calls itself in another manager's picker.
	// Empty means the hostname, which is right often enough that most people
	// will never set this.
	peerName string
	// peerPort is where `cats-todo serve` listens. Zero means peerDefaultPort.
	peerPort int
	// peerInbox is the backlog an arriving bundle lands in. Project by default:
	// a machine serving from a project is almost always being sent work about
	// that project.
	peerInbox scope
	// peerAllowRemote turns off the local-network check in the service. Off,
	// and only worth turning on for someone who has deliberately tunnelled in.
	peerAllowRemote bool
	// peers are machines this manager has been told about by hand — the ones
	// the beacon cannot reach (another subnet, multicast filtered) and the ones
	// worth a row even while they are asleep.
	peers []settingsPeer
}

// settingsPeer is one remembered machine.
type settingsPeer struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
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
	// The LAN service's settings. Strings and numbers rather than pointers:
	// their zero values are already "not set" and mean the documented default,
	// so there is nothing for a pointer to distinguish. peerInbox is spelled
	// on the wire ("project"/"global") rather than numbered, because a config
	// file a person edits should not ask them to remember that 1 is global.
	PeerToken       string         `json:"peerToken,omitempty"`
	PeerName        string         `json:"peerName,omitempty"`
	PeerPort        int            `json:"peerPort,omitempty"`
	PeerInbox       string         `json:"peerInbox,omitempty"`
	PeerAllowRemote *bool          `json:"peerAllowRemote,omitempty"`
	Peers           []settingsPeer `json:"peers,omitempty"`
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
	s.peerToken, s.peerName, s.peerPort, s.peers = f.PeerToken, f.PeerName, f.PeerPort, f.Peers
	if f.PeerInbox == "global" {
		s.peerInbox = scopeGlobal
	}
	if f.PeerAllowRemote != nil {
		s.peerAllowRemote = *f.PeerAllowRemote
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
		PeerToken:       s.peerToken,
		PeerName:        s.peerName,
		PeerPort:        s.peerPort,
		PeerAllowRemote: &s.peerAllowRemote,
		Peers:           s.peers,
	}
	// Written only when it is not the default, so a settings file belonging to
	// someone who has never run `serve` says nothing about peers at all.
	if s.peerInbox == scopeGlobal {
		f.PeerInbox = "global"
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
