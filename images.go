// images.go — image attachments on a todo.
//
// The drop path is text-only: pane.send_input types keystrokes into a pane, and
// the cats control vocabulary has nothing that carries bytes. So an image never
// travels as an image — it travels as an absolute path in the delivered prompt,
// which the agent then reads off disk. That makes an attachment a file the
// backlog has to keep alive for as long as the todo does, which is why attaching
// copies the file in rather than recording wherever the user's screenshot
// happened to be sitting.

package main

import (
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

// imagesDirName is the directory beside todos.json that holds attachment
// copies, one subdirectory per todo id.
const imagesDirName = "images"

// maxImageBytes caps one attachment. Attachments are copied into the backlog —
// which for a project lives inside the repo — so an accidental giant file would
// quietly bloat it, and anything near this size is past what an agent will read
// anyway.
const maxImageBytes = 10 << 20 // 10 MiB

// imageExts are the extensions accepted as attachments: the formats agents can
// actually read. Anything else is refused at attach time, while the user is
// standing right there, rather than at drop time when they are not.
var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
}

// imageRef is one attachment resolved against its backlog: the stored
// (backlog-relative) path, the absolute path an agent would read, and whether
// the file is still there.
type imageRef struct {
	rel     string
	abs     string
	missing bool
}

// imagesDir is the attachment root for this store — images/ beside todos.json.
// Empty for an unavailable store, which has nowhere to put anything.
func (s *store) imagesDir() string {
	if s.path == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(s.path), imagesDirName)
}

// imagePath resolves one stored attachment path to the absolute path that gets
// handed to an agent. Stored paths are relative to the backlog file's directory
// (so a committed project backlog survives being cloned somewhere else) and use
// forward slashes on the wire; an absolute stored path is passed through
// untouched, for anything hand-written into todos.json.
func (s *store) imagePath(rel string) string {
	if s.path == "" || rel == "" {
		return ""
	}
	local := filepath.FromSlash(rel)
	if filepath.IsAbs(local) {
		return local
	}
	return filepath.Join(filepath.Dir(s.path), local)
}

// resolveImages resolves every attachment of t for display: each one's absolute
// path and whether it still exists. Missing files are reported rather than
// hidden — the view stage is where a user can actually do something about one.
func (s *store) resolveImages(t Todo) []imageRef {
	if len(t.Images) == 0 {
		return nil
	}
	refs := make([]imageRef, 0, len(t.Images))
	for _, rel := range t.Images {
		abs := s.imagePath(rel)
		_, err := os.Stat(abs)
		refs = append(refs, imageRef{rel: rel, abs: abs, missing: abs == "" || err != nil})
	}
	return refs
}

// imagePaths is the delivery view of the same list: absolute paths of the
// attachments that still exist. A path to a file that has since been deleted
// would only send the agent chasing it, so those are left out here.
func (s *store) imagePaths(t Todo) []string {
	var paths []string
	for _, ref := range s.resolveImages(t) {
		if !ref.missing {
			paths = append(paths, ref.abs)
		}
	}
	return paths
}

// attachImages copies srcs into the backlog's own storage under todoID and
// returns the backlog-relative paths to record on the todo.
//
// It is all-or-nothing: a partial copy would leave a todo carrying some of what
// the user asked for and no sign of the rest, so the first failure removes
// whatever this call had already written and reports. Files that were already
// under todoID (from an earlier attach) are left alone.
func (s *store) attachImages(todoID string, srcs []string) ([]string, error) {
	if len(srcs) == 0 {
		return nil, nil
	}
	if s.path == "" {
		return nil, errors.New("no backlog here to attach images to")
	}
	if todoID == "" {
		return nil, errors.New("cannot attach images without a todo id")
	}

	dir := filepath.Join(s.imagesDir(), todoID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	rels := make([]string, 0, len(srcs))
	written := make([]string, 0, len(srcs))
	for _, src := range srcs {
		rel, dst, err := attachOne(dir, todoID, src)
		if err != nil {
			for _, w := range written {
				os.Remove(w)
			}
			// Only if this call created it: a non-empty directory is one that
			// already held attachments, and Remove leaves it alone.
			os.Remove(dir)
			return nil, err
		}
		rels = append(rels, rel)
		written = append(written, dst)
	}
	return rels, nil
}

// attachOne validates and copies a single source file into dir, returning the
// backlog-relative path to record and the absolute path written (so the caller
// can roll it back).
func attachOne(dir, todoID, src string) (rel, dst string, err error) {
	abs, err := validateImageSource(src)
	if err != nil {
		return "", "", err
	}

	name := uniqueName(dir, filepath.Base(abs))
	dst = filepath.Join(dir, name)
	if err = copyFile(abs, dst); err != nil {
		return "", "", fmt.Errorf("%s: %w", src, err)
	}
	// path.Join, not filepath.Join: the stored form is forward-slashed so a
	// backlog written on one platform reads on another.
	return path.Join(imagesDirName, todoID, name), dst, nil
}

// validateImageSource checks that src is something we can attach and returns
// its absolute path. Split out from the copy so the form can refuse a bad path
// the moment it is typed — the user is right there to fix it, which they will
// not be at save time, let alone at drop time.
func validateImageSource(src string) (string, error) {
	abs, err := filepath.Abs(src)
	if err != nil {
		return "", fmt.Errorf("%s: %w", src, err)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("%s: %w", src, err)
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%s: not a regular file", src)
	}
	if fi.Size() > maxImageBytes {
		return "", fmt.Errorf("%s: %.1f MiB is over the %d MiB attachment limit",
			src, float64(fi.Size())/(1<<20), maxImageBytes>>20)
	}
	if ext := strings.ToLower(filepath.Ext(abs)); !imageExts[ext] {
		return "", fmt.Errorf("%s: %q is not an image (accepted: %s)", src, ext, acceptedExts())
	}
	return abs, nil
}

// cleanSourcePath turns what a terminal actually delivers into a path we can
// stat. Dragging a file onto a pane inserts it the way a shell would need it —
// quoted, or with spaces backslash-escaped — and some sources hand over a
// file:// URL; a hand-typed path may start with ~. None of that is a filename,
// and all of it arrives in the same one-line input.
func cleanSourcePath(in string) string {
	s := strings.TrimSpace(in)
	if len(s) >= 2 {
		if q := s[0]; (q == '\'' || q == '"') && s[len(s)-1] == q {
			s = s[1 : len(s)-1]
		}
	}
	s = strings.TrimPrefix(s, "file://")
	// Shell escaping, not Go escaping: a backslash before any character means
	// that literal character. Only the escapes a terminal's drop actually emits
	// matter here (space, and the backslash itself), but treating every pair
	// this way is both simpler and right.
	if strings.Contains(s, `\`) {
		var b strings.Builder
		for i := 0; i < len(s); i++ {
			if s[i] == '\\' && i+1 < len(s) {
				i++
				b.WriteByte(s[i])
				continue
			}
			b.WriteByte(s[i])
		}
		s = b.String()
	}
	if s == "~" || strings.HasPrefix(s, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			s = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(s, "~"), "/"))
		}
	}
	return strings.TrimSpace(s)
}

// imageSourceDirEnvVar overrides where recentImages looks. macOS can be told to
// drop screenshots anywhere, and we do not shell out to `defaults` to find out —
// so anyone who moved theirs points this at it instead.
const imageSourceDirEnvVar = "CATS_TODO_IMAGE_DIR"

// recentImageLimit bounds how many recent files the form will cycle through.
// Enough to reach the screenshot before last, short enough that cycling stays
// faster than typing a path.
const recentImageLimit = 12

// recentImageDirs are the directories scanned for a recent image: the override
// when set, else the two places a screenshot or a download actually lands.
func recentImageDirs() []string {
	if d := os.Getenv(imageSourceDirEnvVar); d != "" {
		return []string{d}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{filepath.Join(home, "Desktop"), filepath.Join(home, "Downloads")}
}

// recentImages returns the newest attachable image files from recentImageDirs,
// most recent first — the "I just took a screenshot" path, which is the usual
// reason an image is going onto a todo at all. Directories are scanned one level
// deep only; unreadable ones are skipped rather than reported, since this is a
// convenience offered next to a perfectly good text input.
func recentImages() []string {
	type candidate struct {
		path string
		mod  time.Time
	}
	var found []candidate
	for _, dir := range recentImageDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !imageExts[strings.ToLower(filepath.Ext(e.Name()))] {
				continue
			}
			fi, err := e.Info()
			if err != nil || fi.Size() > maxImageBytes {
				continue
			}
			found = append(found, candidate{path: filepath.Join(dir, e.Name()), mod: fi.ModTime()})
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].mod.After(found[j].mod) })
	if len(found) > recentImageLimit {
		found = found[:recentImageLimit]
	}
	paths := make([]string, len(found))
	for i, c := range found {
		paths[i] = c.path
	}
	return paths
}

// removeImageFiles deletes specific attachments of todoID — the ones detached in
// the form, or ones just copied in for a save that then failed. Best effort, for
// the same reason as removeImages. The now-possibly-empty directory goes too,
// but only if it is empty (Remove, not RemoveAll), so surviving attachments are
// never caught up in it.
func (s *store) removeImageFiles(todoID string, rels []string) {
	if s.path == "" || len(rels) == 0 {
		return
	}
	for _, rel := range rels {
		if abs := s.imagePath(rel); abs != "" {
			_ = os.Remove(abs)
		}
	}
	_ = os.Remove(filepath.Join(s.imagesDir(), todoID))
}

// acceptedExts lists the accepted extensions for an error message, in a stable
// order (map iteration is not one).
func acceptedExts() string {
	exts := make([]string, 0, len(imageExts))
	for ext := range imageExts {
		exts = append(exts, ext)
	}
	sort.Strings(exts)
	return strings.Join(exts, " ")
}

// uniqueName returns name, or name with a -1, -2, … suffix before its extension
// when that file is already in dir — two screenshots both called
// "Screenshot.png" must not silently become one attachment.
func uniqueName(dir, name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	candidate := name
	for i := 1; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, candidate)); err != nil {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
}

// copyFile copies src to dst, which must not already exist (O_EXCL — uniqueName
// picked a free name, and racing another pane onto the same one should fail
// rather than overwrite).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
	}
	return err
}

// removeImages deletes the attachment directory of todoID. Best effort by
// design: it runs after the todo is already gone from the backlog, and a
// failure to remove files must not turn a completed delete into a reported
// failure.
func (s *store) removeImages(todoID string) {
	if s.path == "" || todoID == "" {
		return
	}
	_ = os.RemoveAll(filepath.Join(s.imagesDir(), todoID))
}
