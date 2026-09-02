// import.go — bringing a bundle into a backlog.
//
// Export's counterpart, and the reason a bundle is worth writing at all: a
// .catstodo.json in a Downloads folder, or a colleague's mail attachment, is
// only a file until something turns it back into prompts. `ctrl+r` on the list
// opens the **Import from…** picker, which is the export picker's shape read
// backwards — a source instead of a destination.
//
// Where a bundle can come from, in the order the picker offers them:
//
//	a file on disk     — the folder browser in its files flavour, showing
//	                     directories and bundles and nothing else, so the one
//	                     file in a folder of two hundred that can be imported
//	                     is the one that is listed.
//	a machine on the   — a cats-todo peer found on the local network (peer.go),
//	local network        pulled over HTTP; and "Enter a host…" for one the
//	                     beacon did not reach.
//
// Whatever the source, the bundle is read *before* anything is written, and
// what would happen is put on screen as a confirm: how many prompts, into which
// backlog, and how many of them this backlog already holds and will therefore
// skip. An import is the one operation here that takes work from somewhere else
// and mixes it into the user's own list, and doing that without showing the
// arithmetic first would be the wrong kind of helpful.
package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// importKind is where a row in the import picker reads a bundle from.
type importKind int

const (
	importFromFile importKind = iota // browse the filesystem for a bundle
	importFromPeer                   // a cats-todo found on the local network
	importFromAddr                   // a host typed in, for a peer the beacon missed
	importSection                    // a heading row: not a source, not selectable
)

// importTarget is one selectable source in the import picker.
type importTarget struct {
	kind importKind
	// addr is the peer's host:port for importFromPeer.
	addr  string
	label string
	desc  string
}

// buildImportTargets assembles the sources, in the order the picker shows them.
// peers may be empty — no beacon, no network, nothing listening — and the
// picker is simply shorter for it, the way the export picker is without a
// control socket.
func buildImportTargets(peers []peer) []importTarget {
	targets := []importTarget{{
		kind:  importFromFile,
		label: "A bundle file on disk…",
		desc:  "browse for a " + bundleExtJSON + " or " + bundleExtZip,
	}}
	targets = append(targets, importTarget{kind: importSection, label: "On the local network"})
	for _, p := range peers {
		targets = append(targets, importTarget{
			kind:  importFromPeer,
			addr:  p.addr,
			label: p.name,
			desc:  p.describe(),
		})
	}
	targets = append(targets, importTarget{
		kind:  importFromAddr,
		label: "Enter a host…",
		desc:  "host or host:port of a machine running `cats-todo serve`",
	})
	return targets
}

// pendingImport is a bundle that has been read and described but not yet
// written: what the confirm screen is about, and what answering it acts on.
type pendingImport struct {
	bundle Bundle
	open   bundleOpener
	// from names where it came from, for the confirm line and the status: a
	// shortened path, or a peer's name.
	from string
	// scope is the backlog it will land in — the confirm's `tab` toggles it,
	// because "which backlog" is exactly the decision a user makes while
	// looking at how many prompts are about to arrive.
	scope scope
	// duplicates is how many of the bundle's prompts this backlog already
	// holds, counted at confirm time so the number on screen is the number that
	// will be skipped.
	duplicates int
}

// countDuplicates is how many of a bundle's prompts dst already holds, by the
// text a human wrote (see todoKey). Counted against the bundle's own repeats
// too, so the confirm's arithmetic adds up: added + skipped = what is in the
// file.
func countDuplicates(dst *store, b Bundle) int {
	if dst == nil || !dst.available() {
		return 0
	}
	have := existingKeys(dst)
	n := 0
	for _, td := range b.Todos {
		k := todoKey(td)
		if have[k] {
			n++
			continue
		}
		have[k] = true
	}
	return n
}

// --- The stage -----------------------------------------------------------------

// beginImport opens the import picker (ctrl+r). Unlike every other action on
// the list it is not about a row — a backlog with nothing in it is exactly
// where an import is most useful — so there is nothing to refuse.
func (m model) beginImport() (tea.Model, tea.Cmd) {
	if !m.project.available() && !m.global.available() {
		m.setStatus("no backlog here to import into — relaunch from a project, or with --global", true)
		return m, nil
	}
	m.importTargets = buildImportTargets(knownPeers())
	m.importList = newFuzzyList("Filter sources…", importItems(m.importTargets))
	m.stage = stageImport
	return m, tea.Batch(textinput.Blink, discoverPeersCmd())
}

// importItems turns the sources into rows, headings included — the export
// picker's exportItems exactly, and for the same reasons (see there).
func importItems(targets []importTarget) []listItem {
	items := make([]listItem, len(targets))
	for i, t := range targets {
		items[i] = listItem{
			name:       t.label,
			desc:       t.desc,
			selectable: t.kind != importSection,
			ref:        i,
		}
	}
	return items
}

// updateImport is the picker's key loop: the export picker's, minus the
// copy/move split — every import is a copy, since the source keeps whatever it
// had.
func (m model) updateImport(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.backToList()
		return m, nil
	case "up", "ctrl+p":
		m.importList.moveUp()
		return m, nil
	case "down", "ctrl+n":
		m.importList.moveDown()
		return m, nil
	case "enter":
		return m.chooseImport()
	}
	return m, m.importList.editQuery(msg)
}

// chooseImport acts on the highlighted row.
func (m model) chooseImport() (tea.Model, tea.Cmd) {
	idx := m.importList.selectedIndex()
	if idx < 0 || idx >= len(m.importTargets) {
		return m, nil
	}
	t := m.importTargets[idx]
	switch t.kind {
	case importSection:
		return m, nil
	case importFromFile:
		return m.beginImportBrowse()
	case importFromPeer:
		return m.importFromPeer(t.addr, t.label)
	case importFromAddr:
		return m.beginPeerAddr(peerAddrForImport)
	}
	return m, nil
}

// clickImport is the pointer on the picker, clickExport's twin.
func (m model) clickImport(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	i, ok := m.importList.rowAtLine(msg.Y - importRowsRow)
	if !ok || !m.importList.focusRow(i) {
		return m, nil
	}
	return m.chooseImport()
}

// importRowsRow is the first line the import picker's rows are drawn on — the
// export picker's geometry exactly, since viewImport draws the same chrome.
// TestImportRowsMatchWhatIsDrawn pins it to a real frame.
const importRowsRow = exportRowsRow

// beginImportBrowse opens the folder browser to find a bundle: files as well as
// folders this time, and only the files that are bundles (see
// filePicker.onlyBundles) — a folder of two hundred downloads should list the
// one thing that can be imported, not make the user find it.
func (m model) beginImportBrowse() (tea.Model, tea.Cmd) {
	m.files = newFilePicker(bundleBrowseRoot(m.ctx))
	m.files.purpose = filesForImport
	m.files.onlyBundles = true
	m.files.refresh()
	m.files.resize(m.width, m.height)
	m.stage = stageFiles
	return m, textinput.Blink
}

// chooseImportFile reads the highlighted file and moves to the confirm. A
// directory under the cursor is opened rather than read, which is what the
// browser's own keys do anyway and what a stray enter on a folder means.
func (m model) chooseImportFile() (tea.Model, tea.Cmd) {
	e, abs, ok := m.files.highlighted()
	if !ok {
		return m, nil
	}
	if e.dir {
		m.files.descend()
		return m, nil
	}
	b, open, err := readBundle(abs)
	if err != nil {
		// The browser stays up: the answer to "that file is not a bundle" is
		// almost always the file next to it.
		m.files.err = err.Error()
		return m, nil
	}
	return m.confirmImport(b, open, shortenHome(abs))
}

// confirmImport puts the arithmetic on screen: what is in the bundle, which
// backlog it would land in, and how much of it this backlog already has.
func (m model) confirmImport(b Bundle, open bundleOpener, from string) (tea.Model, tea.Cmd) {
	if len(b.Todos) == 0 {
		m.setStatus("that bundle holds no prompts", true)
		m.backToList()
		return m, nil
	}
	pend := pendingImport{bundle: b, open: open, from: from, scope: m.defaultImportScope()}
	pend.duplicates = countDuplicates(m.storeFor(pend.scope), b)
	m.pendingImport = pend
	m.confirmKind = confirmImport
	m.stage = stageConfirm
	return m, nil
}

// defaultImportScope is the backlog an import lands in unless the user says
// otherwise: this project's, because that is the backlog the manager was
// launched for — and the global one when there is no project, which is the only
// other place there is.
func (m model) defaultImportScope() scope {
	if m.project.available() {
		return scopeProject
	}
	return scopeGlobal
}

// toggleImportScope is the confirm's `tab`: send it to the other backlog
// instead, re-counting the duplicates against the backlog it would now land in.
// A launch with only one store has nowhere to toggle to and says so.
func (m model) toggleImportScope() (tea.Model, tea.Cmd) {
	other := scopeGlobal
	if m.pendingImport.scope == scopeGlobal {
		other = scopeProject
	}
	s := m.storeFor(other)
	if s == nil || !s.available() {
		m.setStatus("there is no other backlog in this launch", true)
		return m, nil
	}
	m.pendingImport.scope = other
	m.pendingImport.duplicates = countDuplicates(s, m.pendingImport.bundle)
	return m, nil
}

// performImport writes the pending bundle into the chosen backlog and lands
// back on the list with what happened.
func (m model) performImport() (tea.Model, tea.Cmd) {
	pend := m.pendingImport
	dst := m.storeFor(pend.scope)
	res, err := importBundle(dst, pend.bundle, pend.open, importOpts{})
	m.pendingImport = pendingImport{}
	m.rebuildList()
	m.backToList()
	if err != nil {
		m.setStatus("import failed: "+err.Error(), true)
		return m, nil
	}
	m.setStatus(importResultNote(res, pend), false)
	return m, nil
}

// importResultNote is the status line for a finished import: what landed, what
// was already here, and what arrived without its attachments.
func importResultNote(res importResult, pend pendingImport) string {
	if res.added == 0 {
		return fmt.Sprintf("nothing new in %s — the %s backlog already has %s",
			pend.from, strings.ToLower(pend.scope.String()), promptWord(res.skipped))
	}
	note := fmt.Sprintf("imported %s from %s → the %s backlog",
		promptWord(res.added), pend.from, strings.ToLower(pend.scope.String()))
	if res.skipped > 0 {
		note += fmt.Sprintf(" · %d already here", res.skipped)
	}
	if res.noFiles > 0 {
		note += fmt.Sprintf(" · %d arrived without their attachments", res.noFiles)
	}
	return note
}

// viewImport draws the picker: the export picker's chrome with the arrow
// reversed.
func (m model) viewImport() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Import from…"))
	b.WriteString("  ")
	b.WriteString(descStyle.Render("prompts land in the " + strings.ToLower(m.defaultImportScope().String()) + " backlog"))
	b.WriteString("\n\n")
	b.WriteString(m.importList.view("nothing matches — clear the filter, or browse for a file", "", m.width))
	b.WriteString("\n")
	b.WriteString(footerStyle.Render(m.fitFooter([]string{
		"enter choose", "esc back",
	})))
	return b.String()
}

// viewImportConfirm is the confirm screen for an import: the arithmetic, then
// the keys. It says the whole sum — what lands, what is skipped — because an
// import mixes someone else's work into the user's own list, and "12 prompts"
// with three of them already there is a different thing from twelve new ones.
func (m model) viewImportConfirm() string {
	pend := m.pendingImport
	var b strings.Builder
	b.WriteString(titleStyle.Render("Import"))
	b.WriteString("\n\n")
	b.WriteString(nameStyle.Render("  " + promptWord(len(pend.bundle.Todos)) + " from " + truncate(pend.from, 60)))
	b.WriteString("\n")
	if pend.bundle.From != "" {
		b.WriteString(descStyle.Render("  written by " + truncate(pend.bundle.From, 60)))
		b.WriteString("\n")
	}
	arriving := len(pend.bundle.Todos) - pend.duplicates
	line := fmt.Sprintf("  → the %s backlog · %s new", strings.ToLower(pend.scope.String()), promptWord(arriving))
	if pend.duplicates > 0 {
		line += fmt.Sprintf(" · %d already here, skipped", pend.duplicates)
	}
	b.WriteString(descStyle.Render(line))
	b.WriteString("\n\n")
	b.WriteString(footerStyle.Render(m.fitFooter([]string{
		"y import", "tab other backlog", "n / esc cancel",
	})))
	return b.String()
}

// viewImportBrowse draws the file browser in its import flavour: the same
// chrome, saying what a choice here does.
func (m model) viewImportBrowse() string {
	var b strings.Builder
	title := "Import a bundle from"
	b.WriteString(titleStyle.Render(title))
	b.WriteString("  ")
	room := 0
	if m.width > 0 {
		room = m.width - lipgloss.Width(titleStyle.Render(title)) - 4
	}
	b.WriteString(descStyle.Render(m.files.headingDir(room)))
	b.WriteString("\n\n")
	b.WriteString(m.files.list.view(m.importBrowseEmptyMessage(), "", m.width))
	b.WriteString("\n")
	b.WriteString(footerStyle.Render(m.fitFooter([]string{
		"enter read it", "tab/→ or / open folder", "backspace up", "esc back",
		"~/ and ../ paths", ". shows hidden",
	})))
	return b.String()
}

// importBrowseEmptyMessage explains an empty listing, which in this browser
// usually means "this folder has files, but none of them are bundles" — a
// different thing from an empty folder, and the one the user has to know to
// walk somewhere else.
func (m model) importBrowseEmptyMessage() string {
	if m.files.err != "" {
		return m.files.err
	}
	return "no bundles here — a bundle is a " + bundleExtJSON + " or " + bundleExtZip
}

// --- Sources that are not a file ------------------------------------------------

// importFromPeer pulls a peer's backlog and moves to the confirm. It runs on
// the UI thread like the rest of this feature: the request is bounded (see
// peerTimeout), and a screen that returned before the bundle was in hand would
// have nothing to confirm.
func (m model) importFromPeer(addr, name string) (tea.Model, tea.Cmd) {
	if addr == "" {
		m.setStatus("that machine has no address", true)
		return m, nil
	}
	b, open, err := peerFetch(addr)
	if err != nil {
		m.setStatus("could not read from "+name+": "+err.Error(), true)
		return m, nil
	}
	return m.confirmImport(b, open, name)
}

// errNoBundle is what a source with nothing in it reports.
var errNoBundle = errors.New("no bundle there")

// bundleFileNames reports whether a filename is a bundle, which is what the
// import browser lists and what a peer's response is expected to be.
func isBundleName(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, bundleExtJSON) || strings.HasSuffix(lower, bundleExtZip)
}

// bundleInDir finds the one bundle in a directory, for the CLI's convenience
// when it is handed a folder rather than a file. More than one is an error
// rather than a guess: picking the newest would silently be wrong the one time
// it mattered.
func bundleInDir(dir string) (string, error) {
	entries, err := readDirEntries(dir)
	if err != nil {
		return "", err
	}
	var found []string
	for _, e := range entries {
		if !e.dir && isBundleName(e.name) {
			found = append(found, filepath.Join(dir, e.name))
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("%s: %w", shortenHome(dir), errNoBundle)
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("%s holds %d bundles — name the one you mean", shortenHome(dir), len(found))
	}
}
