// bundle.go — the portable form of a set of prompts.
//
// export.go carries a prompt from one backlog to another *on this machine*, by
// writing straight into the destination's todos.json. That works because both
// ends are directories this process can open. Everything else a prompt might
// have to cross — a disk it will be picked up from later, a mail message, a
// socket to another machine — needs the prompts to become a self-describing
// thing that stands on its own. That thing is a bundle.
//
// A bundle is deliberately close to a backlog:
//
//	{"schema":1, "created":…, "from":…, "source":…, "todos":[ … ]}
//
// where each element of "todos" is a Todo marshalled by exactly the rules
// todos.json uses. That is the whole compatibility story: the `omitempty`
// discipline the backlog format already keeps (see the Todo doc comments)
// carries into the wire format for nothing, so a bundle written by a newer
// cats-todo loses only the fields an older one never knew about, and a bundle
// written by an older one imports with today's defaults.
//
// Two containers, chosen mechanically by whether anything is attached:
//
//	<name>.catstodo.json   the manifest alone — the common case, and a file a
//	                       human can read, diff, or paste into a message
//	<name>.catstodo.zip    manifest.json + images/<todo id>/<file>, when at
//	                       least one prompt carries an attachment
//
// The attachment paths inside a bundle are the same strings the backlog stores
// (`images/<id>/<file>`, forward-slashed — see attachOne), which means the zip's
// layout *is* the manifest's own view of itself and no rewriting happens at
// either end.
//
// What never travels: the schedule. A schedule names a pane id and a launch
// directory of the machine the prompt is leaving, and a prompt that fired
// itself into a stranger's session would be worse than one that quietly needs
// re-scheduling — the same call exportTodo makes, for the same reason. The
// count comes back from buildBundle so the caller can say so rather than let a
// row's clock vanish without a word (contract: refuse in words).
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// bundleSchema is the version stamped into every bundle this binary writes.
// It is a *format* version, not the app's: it changes only when the shape of
// the envelope changes in a way a reader has to know about, which adding
// `omitempty` fields to Todo is not.
const bundleSchema = 1

// The two container extensions. Double extensions on purpose: the ".json" and
// ".zip" halves keep every tool that sniffs by extension working, while the
// ".catstodo" half is what the import file browser filters on and what tells a
// user which of the JSON files in their Downloads folder this one is.
const (
	bundleExtJSON = ".catstodo.json"
	bundleExtZip  = ".catstodo.zip"
	// bundleManifestName is the manifest's name inside a zip container.
	bundleManifestName = "manifest.json"
)

// maxBundleBytes caps a bundle read from disk or off the network. A bundle is
// read wholly into memory (see readBundleBytes for why), so the cap is what
// keeps a hostile or corrupt file from being an out-of-memory instead of an
// error message. Generous next to maxImageBytes: a bundle is allowed to be a
// backlog's worth of screenshots.
const maxBundleBytes = 64 << 20 // 64 MiB

// Bundle is the manifest: an envelope naming where the prompts came from, and
// the prompts themselves in the backlog's own JSON.
type Bundle struct {
	Schema  int       `json:"schema"`
	Created time.Time `json:"created"`
	// From is who wrote it — "cats-todo v0.25.0 on studio.local". Provenance for
	// a human reading the file or the mail, never anything the importer acts on.
	From string `json:"from,omitempty"`
	// Source names the backlog it left: a project's directory name, or "global".
	Source string `json:"source,omitempty"`
	Todos  []Todo `json:"todos"`
}

// bundleFile is one attachment travelling with a bundle: the bundle-relative
// name (forward-slashed, exactly the string the Todo carries in Images) and its
// bytes.
type bundleFile struct {
	name string
	data []byte
}

// bundleOpener resolves a bundle-relative attachment name to its bytes. It is
// how the import side reads attachments without caring whether the bundle it
// came from was a directory-less JSON file (no attachments at all), a zip, or a
// response body already in memory.
type bundleOpener func(name string) ([]byte, error)

// bundleBuilder accumulates prompts into one bundle, a backlog at a time.
//
// It exists because a selection can span both backlogs — a project prompt and a
// global one ticked in the same list — and each prompt's attachments have to be
// read out of the store *it* lives in. A single buildBundle(src, todos) call
// cannot express that, and grouping the selection by scope before building
// would reorder it, losing the order the user is looking at.
type bundleBuilder struct {
	b       Bundle
	files   []bundleFile
	seen    map[string]bool
	dropped int
}

// newBundleBuilder starts an empty bundle stamped with its provenance.
func newBundleBuilder(from, source string) *bundleBuilder {
	return &bundleBuilder{
		b: Bundle{
			Schema:  bundleSchema,
			Created: time.Now(),
			From:    from,
			Source:  source,
		},
		seen: map[string]bool{},
	}
}

// add appends todos as they stand in src — which may be nil, for prompts with
// no backlog behind them, and which then travel without attachments.
//
// Attachments whose files have gone missing on disk are left out rather than
// named and absent, which is the same view a drop delivers (imagePaths already
// drops them). Schedules are counted and dropped; see the file comment.
func (bb *bundleBuilder) add(src *store, todos ...Todo) {
	for _, td := range todos {
		out := td
		if out.Schedule != nil {
			bb.dropped++
			out.Schedule = nil
		}
		// The session record is shared by pointer on the Todo; a bundle that
		// aliased the live one would let a later edit of the todo mutate a
		// message already "sent" (compare exportTodo's copy).
		if td.Session != nil {
			c := td.Session.clone()
			out.Session = &c
		}

		var rels []string
		for _, ref := range resolveBundleImages(src, td) {
			data, err := os.ReadFile(ref.abs)
			if err != nil {
				continue // vanished between the resolve and the read: same as missing
			}
			rels = append(rels, ref.rel)
			// Attachment names carry their todo's id, so they are unique within
			// one backlog; across two they could in principle collide, and the
			// zip must never be asked to hold one name twice.
			if !bb.seen[ref.rel] {
				bb.seen[ref.rel] = true
				bb.files = append(bb.files, bundleFile{name: ref.rel, data: data})
			}
		}
		out.Images = rels
		bb.b.Todos = append(bb.b.Todos, out)
	}
}

// done returns the assembled bundle, its attachment bytes, and how many
// schedules were dropped along the way.
func (bb *bundleBuilder) done() (Bundle, []bundleFile, int) {
	return bb.b, bb.files, bb.dropped
}

// buildBundle is the single-backlog case of bundleBuilder, which is every case
// but a selection spanning both scopes.
func buildBundle(src *store, todos []Todo, from, source string) (Bundle, []bundleFile, int) {
	bb := newBundleBuilder(from, source)
	bb.add(src, todos...)
	return bb.done()
}

// resolveBundleImages is src.resolveImages guarded for a nil store and filtered
// to the attachments that are actually on disk.
func resolveBundleImages(src *store, td Todo) []imageRef {
	if src == nil || src.path == "" {
		return nil
	}
	var out []imageRef
	for _, ref := range src.resolveImages(td) {
		if !ref.missing {
			out = append(out, ref)
		}
	}
	return out
}

// encodeBundle serialises a bundle into the bytes that get written, mailed or
// posted, and reports which container it chose — the extension, which is also
// the content type in every sense this tool needs one.
//
// The manifest is indented like todos.json is (two spaces): a bundle is a file
// people open in an editor at least as often as a program opens it, and the
// cost of the whitespace is nothing next to being readable.
func encodeBundle(b Bundle, files []bundleFile) (data []byte, ext string, err error) {
	manifest, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return nil, "", err
	}
	manifest = append(manifest, '\n')
	if len(files) == 0 {
		return manifest, bundleExtJSON, nil
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := writeZipEntry(zw, bundleManifestName, manifest); err != nil {
		return nil, "", err
	}
	// Sorted so the same bundle encodes to the same bytes twice — a property
	// worth having for tests and for anyone diffing two exports.
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	for _, f := range files {
		if err := writeZipEntry(zw, f.name, f.data); err != nil {
			return nil, "", err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), bundleExtZip, nil
}

// writeZipEntry adds one deflated member. Attachments are already-compressed
// image formats, but the manifest is not, and one method for both keeps the
// archive uniform for readers that care.
func writeZipEntry(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()})
	if err != nil {
		return fmt.Errorf("bundle entry %s: %w", name, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("bundle entry %s: %w", name, err)
	}
	return nil
}

// bundleFileName is the name a bundle is written under: a stem describing where
// it came from, the date, and the container's extension. The date is in the
// name rather than only in the manifest because the file will sit in a
// Downloads folder among others like it.
func bundleFileName(source string, when time.Time, ext string) string {
	stem := slugForFile(source)
	if stem == "" {
		stem = "cats-todo"
	}
	return fmt.Sprintf("%s-%s%s", stem, when.Format("2006-01-02"), ext)
}

// slugForFile reduces a project or backlog name to something safe in a filename
// on any platform: lower-case, alphanumerics and dashes, no runs, no edges.
func slugForFile(s string) string {
	var b strings.Builder
	lastDash := true // leading dashes are edges too
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// writeBundle encodes a bundle and writes it into dir, returning the path
// written. The name is chosen from the bundle's own Source and date unless base
// names one; an existing file is never overwritten (uniqueName gives the -1,
// -2 … suffix the attachment copier already uses), because a bundle is the
// user's outbound copy of their own work and quietly replacing yesterday's is
// not a thing to do on their behalf.
func writeBundle(dir, base string, b Bundle, files []bundleFile) (string, error) {
	if dir == "" {
		return "", errors.New("no directory to write the bundle to")
	}
	if !isDir(dir) {
		return "", fmt.Errorf("%s is not a directory", shortenHome(dir))
	}
	data, ext, err := encodeBundle(b, files)
	if err != nil {
		return "", err
	}
	name := bundleFileName(b.Source, b.Created, ext)
	if base != "" {
		name = strings.TrimSuffix(base, ext) + ext
	}
	path := filepath.Join(dir, uniqueBundleName(dir, name, ext))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// uniqueBundleName is uniqueName for a double extension: filepath.Ext would see
// only ".json" and turn "todos.catstodo.json" into "todos.catstodo-1.json",
// which is right but reads as if the *bundle* were versioned rather than the
// file. Splitting on the full extension keeps the marker where it belongs.
func uniqueBundleName(dir, name, ext string) string {
	stem := strings.TrimSuffix(name, ext)
	candidate := name
	for i := 1; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, candidate)); err != nil {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d%s", stem, i, ext)
	}
}

// readBundle reads a bundle off disk, whatever container it is in, and returns
// the manifest with an opener for its attachments.
func readBundle(file string) (Bundle, bundleOpener, error) {
	fi, err := os.Stat(file)
	if err != nil {
		return Bundle{}, nil, err
	}
	if fi.Size() > maxBundleBytes {
		return Bundle{}, nil, fmt.Errorf("%s is %.1f MiB, over the %d MiB bundle limit",
			shortenHome(file), float64(fi.Size())/(1<<20), maxBundleBytes>>20)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return Bundle{}, nil, err
	}
	b, open, err := readBundleBytes(data)
	if err != nil {
		return Bundle{}, nil, fmt.Errorf("%s: %w", shortenHome(file), err)
	}
	return b, open, nil
}

// readBundleBytes parses a bundle already in memory — a file just read, or a
// response body — sniffing the container from the bytes rather than from a
// name, since the network path has no name to go by.
//
// Everything is held in memory rather than streamed: a bundle is capped
// (maxBundleBytes), the zip reader wants a ReaderAt anyway, and an opener that
// closed over an open file would make every caller responsible for a lifetime
// they have no reason to think about.
func readBundleBytes(data []byte) (Bundle, bundleOpener, error) {
	if len(data) > maxBundleBytes {
		return Bundle{}, nil, fmt.Errorf("%.1f MiB is over the %d MiB bundle limit",
			float64(len(data))/(1<<20), maxBundleBytes>>20)
	}
	if isZipBytes(data) {
		return readZipBundle(data)
	}
	var b Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return Bundle{}, nil, fmt.Errorf("not a cats-todo bundle: %w", err)
	}
	if err := validateBundle(b); err != nil {
		return Bundle{}, nil, err
	}
	// A JSON container carries no files; an attachment named by the manifest is
	// therefore simply not here, and the opener says so rather than pretending.
	open := func(name string) ([]byte, error) {
		return nil, fmt.Errorf("attachment %s is not in this bundle", name)
	}
	return b, open, nil
}

// isZipBytes reports the PK\x03\x04 local-file-header magic. Cheaper and more
// honest than trying json.Unmarshal first and reading the error.
func isZipBytes(data []byte) bool {
	return len(data) >= 4 && data[0] == 'P' && data[1] == 'K' && data[2] == 0x03 && data[3] == 0x04
}

// readZipBundle pulls the manifest out of a zip container and indexes the rest
// of its members by name for the opener.
func readZipBundle(data []byte) (Bundle, bundleOpener, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Bundle{}, nil, fmt.Errorf("unreadable bundle archive: %w", err)
	}
	members := map[string]*zip.File{}
	var manifest *zip.File
	for _, f := range zr.File {
		if f.Name == bundleManifestName {
			manifest = f
			continue
		}
		members[f.Name] = f
	}
	if manifest == nil {
		return Bundle{}, nil, errors.New("archive has no " + bundleManifestName + " — not a cats-todo bundle")
	}
	raw, err := readZipMember(manifest)
	if err != nil {
		return Bundle{}, nil, err
	}
	var b Bundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return Bundle{}, nil, fmt.Errorf("unreadable %s: %w", bundleManifestName, err)
	}
	if err := validateBundle(b); err != nil {
		return Bundle{}, nil, err
	}
	open := func(name string) ([]byte, error) {
		f, ok := members[name]
		if !ok {
			return nil, fmt.Errorf("attachment %s is not in this bundle", name)
		}
		return readZipMember(f)
	}
	return b, open, nil
}

// readZipMember reads one archive member with the size cap applied to the
// *decompressed* stream: a zip's declared UncompressedSize64 is the archive's
// word, not a fact, so the limit is enforced by the reader rather than trusted.
func readZipMember(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", f.Name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxBundleBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", f.Name, err)
	}
	if len(data) > maxBundleBytes {
		return nil, fmt.Errorf("%s expands past the %d MiB bundle limit", f.Name, maxBundleBytes>>20)
	}
	return data, nil
}

// validateBundle checks the envelope before anything acts on it. A schema from
// the future is accepted rather than refused — the fields this binary knows
// still mean what they meant, which is the whole point of the additive rule —
// but a schema of zero means the JSON was some other object entirely that
// happened to parse, and that is worth saying plainly.
func validateBundle(b Bundle) error {
	if b.Schema <= 0 {
		return errors.New("no schema field — not a cats-todo bundle")
	}
	return nil
}

// --- Import --------------------------------------------------------------------

// importOpts are the two questions an import has to answer. Both defaults
// (fresh ids, skip duplicates) are the safe answer, so the zero value is the
// one the UI uses.
type importOpts struct {
	// keepIDs keeps each todo's original id instead of minting a new one. Off
	// by default: an import is a *new* prompt in this backlog, with its own
	// life from here on, and two backlogs sharing an id would be a claim that
	// nothing in this tool actually honours (compare exportTodo's copy/move
	// split). On, it is how a bundle round-trips exactly — the machine-to-machine
	// case where the two ends are meant to hold the same record.
	keepIDs bool
	// allowDuplicates imports a prompt even when this backlog already holds one
	// with the same title and text. Off by default because the common mistake
	// is importing the same bundle twice, and a silent double of every prompt is
	// a mess to undo by hand.
	allowDuplicates bool
}

// importResult is what the caller tells the user: what landed, what did not,
// and what was quietly left behind.
type importResult struct {
	added   int
	skipped int // duplicates already in this backlog
	noFiles int // prompts whose attachments could not be brought across
}

// importBundle writes a bundle's prompts into dst.
//
// Ordering mirrors exportTodo's, for the same reason: attachments first,
// all-or-nothing per prompt, then a single store write for the whole run. If an
// attachment cannot be brought over, the prompt still lands — without it, and
// counted in noFiles — because the text is the part with the value and a
// refused import over a missing screenshot would be the wrong trade. If the
// *store* write fails, everything this call copied in is removed again, so a
// failed import leaves the backlog exactly as it was.
//
// The whole run is one addAfter("", …) rather than a loop of add(): add
// reloads and saves per call, so N prompts would be N reads and N writes of a
// file this operation is itself growing.
func importBundle(dst *store, b Bundle, open bundleOpener, opts importOpts) (importResult, error) {
	var res importResult
	if dst == nil || !dst.available() {
		return res, errors.New("no backlog to import into")
	}
	if len(b.Todos) == 0 {
		return res, errors.New("that bundle holds no prompts")
	}
	if err := dst.reload(); err != nil {
		return res, err
	}
	have := existingKeys(dst)

	var incoming []Todo
	var written []struct {
		id   string
		rels []string
	}
	rollback := func() {
		for _, w := range written {
			dst.removeImageFiles(w.id, w.rels)
		}
	}

	for _, td := range b.Todos {
		if !opts.allowDuplicates && have[todoKey(td)] {
			res.skipped++
			continue
		}
		out := td
		if !opts.keepIDs {
			out.ID = newID()
		}
		// A schedule should never be in a bundle (buildBundle strips it), but a
		// bundle is a file anyone can edit, and a hand-written one must not be
		// able to arm a timer here.
		out.Schedule = nil
		if td.Session != nil {
			c := td.Session.clone()
			out.Session = &c
		}
		if out.Created.IsZero() {
			out.Created = time.Now()
		}

		rels, err := importAttachments(dst, out.ID, td.Images, open)
		if err != nil {
			// The prompt is worth more than the picture: land it bare and say
			// how many lost their attachments.
			res.noFiles++
			rels = nil
		}
		out.Images = rels
		if len(rels) > 0 {
			written = append(written, struct {
				id   string
				rels []string
			}{out.ID, rels})
		}
		// Guard against a bundle that repeats a prompt inside itself as well as
		// against one this backlog already holds.
		have[todoKey(out)] = true
		incoming = append(incoming, out)
	}

	if len(incoming) == 0 {
		return res, nil // everything was a duplicate; not an error, and nothing written
	}
	if err := dst.addAfter("", incoming); err != nil {
		rollback()
		return importResult{}, fmt.Errorf("write %s: %w", shortenHome(dst.path), err)
	}
	res.added = len(incoming)
	return res, nil
}

// importAttachments materialises a prompt's bundled attachments into dst.
//
// The bytes are staged into a temp directory and then handed to
// store.attachImages as ordinary source paths, rather than being written under
// images/ directly. That is one extra copy of a few hundred KB in exchange for
// having exactly one piece of code that decides where an attachment lives, what
// names collide, what sizes and extensions are allowed, and how a half-finished
// copy is rolled back — validateImageSource and attachImages already are that
// code, and a second implementation of it would be the thing that drifts.
func importAttachments(dst *store, todoID string, names []string, open bundleOpener) ([]string, error) {
	if len(names) == 0 || open == nil {
		return nil, nil
	}
	tmp, err := os.MkdirTemp("", "cats-todo-import-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	srcs := make([]string, 0, len(names))
	for _, name := range names {
		base, err := safeAttachmentName(name)
		if err != nil {
			return nil, err
		}
		data, err := open(name)
		if err != nil {
			return nil, err
		}
		if len(data) > maxImageBytes {
			return nil, fmt.Errorf("%s: over the %d MiB attachment limit", base, maxImageBytes>>20)
		}
		p := filepath.Join(tmp, uniqueName(tmp, base))
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return nil, err
		}
		srcs = append(srcs, p)
	}
	return dst.attachImages(todoID, srcs)
}

// safeAttachmentName reduces a bundled attachment's path to the bare file name
// it will be stored under, refusing anything that tries to be a path.
//
// This is the one place a bundle from elsewhere reaches the filesystem with a
// name it chose, so it is where a zip-slip is stopped: no absolute paths, no
// "..", no separators surviving into the name. The extension is left to
// validateImageSource, which refuses what is not an image and does it with the
// same words the form uses.
func safeAttachmentName(name string) (string, error) {
	clean := path.Clean(strings.ReplaceAll(name, `\`, "/"))
	if clean == "" || clean == "." || path.IsAbs(clean) || strings.HasPrefix(clean, "../") || clean == ".." {
		return "", fmt.Errorf("attachment %q has an unsafe name", name)
	}
	base := path.Base(clean)
	if base == "" || base == "." || base == ".." || strings.ContainsAny(base, `/\`) {
		return "", fmt.Errorf("attachment %q has an unsafe name", name)
	}
	return base, nil
}

// todoKey is the identity an import dedupes on: the text a human wrote. Not the
// id — a bundle's ids are from another backlog and mean nothing here — and not
// the whole struct, since the same prompt arriving with a priority set is still
// the same prompt.
func todoKey(t Todo) string {
	return strings.TrimSpace(t.Title) + "\x00" + strings.TrimSpace(t.Prompt)
}

// existingKeys indexes a backlog by todoKey, including its done and frozen
// rows: re-importing a bundle whose prompts were finished here should not
// resurrect them as fresh work.
func existingKeys(s *store) map[string]bool {
	keys := make(map[string]bool, len(s.todos))
	for _, t := range s.todos {
		keys[todoKey(t)] = true
	}
	return keys
}

// --- Human rendering -----------------------------------------------------------

// renderBundleMarkdown writes a bundle as readable markdown: what the email
// body carries, and what someone with no cats-todo installed sees when the
// bundle reaches them.
//
// It is a rendering, not a format — nothing parses it back. That is why it can
// afford to say things the JSON says structurally (a done prompt is marked
// "done" in words) and to leave out what only a machine cares about (ids).
func renderBundleMarkdown(b Bundle) string {
	var sb strings.Builder
	sb.WriteString("# Prompts")
	if b.Source != "" {
		sb.WriteString(" from " + b.Source)
	}
	sb.WriteString("\n\n")
	if b.From != "" {
		sb.WriteString("_" + b.From + " · " + b.Created.Format("2006-01-02 15:04") + "_\n\n")
	}
	for i, t := range b.Todos {
		title := firstNonEmpty(t.Title, firstLine(t.Prompt, 60))
		fmt.Fprintf(&sb, "## %d. %s\n", i+1, title)
		if note := bundleTodoNote(t); note != "" {
			sb.WriteString("_" + note + "_\n")
		}
		sb.WriteString("\n")
		if p := strings.TrimRight(t.Prompt, "\n"); p != "" {
			sb.WriteString(p + "\n")
		}
		if len(t.Images) > 0 {
			sb.WriteString("\nAttachments: " + strings.Join(bundleImageNames(t.Images), ", ") + "\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// bundleTodoNote is the one-line state of a prompt in the markdown rendering:
// its group and its marks, or "" when it is an ordinary open prompt with
// nothing set — which is most of them, and deserves no line at all.
func bundleTodoNote(t Todo) string {
	var parts []string
	switch t.group() {
	case groupDone:
		parts = append(parts, "done")
	case groupFrozen:
		parts = append(parts, "frozen")
	}
	if lbl := priorityLabel(t.Priority); lbl != "" {
		parts = append(parts, strings.ToLower(lbl)+" priority")
	}
	if t.Fruit {
		parts = append(parts, "low-hanging fruit")
	}
	if t.Session != nil {
		if s := t.Session.summary(); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " · ")
}

// bundleImageNames is the attachment list a human reads: base names, since the
// images/<id>/ prefix is bookkeeping.
func bundleImageNames(rels []string) []string {
	out := make([]string, 0, len(rels))
	for _, rel := range rels {
		out = append(out, path.Base(rel))
	}
	return out
}
