// worktree.go — the "on a new worktree" flavour of a new-session drop.
//
// An ordinary new-session drop launches its agent in the project's own
// checkout. That is right for one agent at a time and wrong for two: they share
// a single working tree, so the second agent edits files the first one is
// half-way through changing, and neither run is separable afterwards. A
// worktree drop asks cats for a fresh `git worktree` checkout on a new branch
// first and launches the agent *there*, so the prompt gets a tree of its own and
// the branch name records which todo it came from.
//
// cats already owns the git side — worktree.create runs `git worktree add -b`,
// then opens and focuses a new workspace on the checkout, named after the branch
// (see cats internal/worktree + cmd/catway/worktrees.go). So this file is only
// the two decisions the plugin itself has to make: what to call the branch, and
// when the option may be offered at all.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// worktreeBranchPrefix namespaces every branch a worktree drop creates. The
// "/" makes it a directory in git's ref layout, so these branches collapse into
// one foldable group in tooling that renders refs as a tree, and a stale batch
// is one `git branch -D todo/...` glob away. cats derives the checkout folder
// name from the branch (BranchToPathSlug flattens the "/"), so the prefix costs
// nothing on disk.
const worktreeBranchPrefix = "todo/"

// branchSlugMaxRunes caps the readable part of a generated branch name. Long
// enough that a todo title is still recognisable, short enough that the branch
// fits the places it gets echoed — the workspace name, the tab bar, `git
// branch` output.
const branchSlugMaxRunes = 32

// todoBranchName is the branch a worktree drop creates for a todo: the prefix,
// a slug of the todo's title (falling back to the first line of the prompt),
// and a short seed-derived suffix.
//
// The suffix is what makes the name safe to generate blindly. `git worktree add
// -b` refuses an existing branch, so a bare title slug would fail the *second*
// time the same todo is dropped — and dropping one prompt onto several parallel
// worktrees is a thing people actually want (three attempts at the same task,
// compared afterwards). Seeding from the clock rather than the todo id keeps
// each drop distinct while leaving the readable half stable, so the branches of
// one todo sort together.
//
// seed is a parameter rather than a time.Now() call inside so the naming stays
// deterministic under test; callers pass time.Now().UnixMicro(), matching how
// cats seeds its own generated branch slugs.
func todoBranchName(td Todo, seed int64) string {
	slug := branchSlug(firstNonEmpty(td.Title, firstLine(td.Prompt, branchSlugMaxRunes)))
	if slug == "" {
		// Nothing survived — an untitled todo whose prompt is punctuation or
		// non-Latin text. The suffix alone still names a unique branch.
		slug = "todo"
	}
	return fmt.Sprintf("%s%s-%04x", worktreeBranchPrefix, slug, uint16(seed))
}

// branchSlug reduces free text to the safe middle of a branch name: lowercase
// ASCII alphanumerics, with every other run collapsed to a single "-" and the
// edges trimmed, capped at branchSlugMaxRunes.
//
// The conservative character set is deliberate. git's ref rules are a list of
// prohibitions (no "..", no "@{", no trailing ".lock", no leading/trailing "/"
// or "-", no control characters or spaces), and matching them one by one means
// re-deriving them correctly forever. Permitting only [a-z0-9-] and refusing to
// end on a "-" cannot produce any of the forbidden shapes, so the result is
// valid by construction.
//
// The truncation is applied before the final trim, so a cut that lands mid-word
// cannot leave the name ending in the separator.
func branchSlug(s string) string {
	var b strings.Builder
	dash := false // pending separator, emitted only once something follows it
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		default:
			dash = true
		}
		if b.Len() >= branchSlugMaxRunes {
			break
		}
	}
	// Only ASCII was written, so a byte trim is a rune trim.
	return strings.Trim(b.String(), "-")
}

// gitRepoRoot walks up from dir to the nearest ancestor holding a .git entry,
// returning "" when there is none (and for an empty dir). It is what gates the
// worktree rows in the target picker: outside a repo there is nothing to branch
// from, and offering a row whose only possible outcome is "not a git worktree"
// wastes the user's keystroke.
//
// Both forms of .git count. An ordinary checkout has a directory; a *linked*
// worktree has a file pointing into the main repo's .git/worktrees/<name>. The
// second form has to qualify, because it is exactly what this feature produces
// — a manager opened in a checkout created by an earlier worktree drop must
// still be able to make the next one.
//
// This duplicates the .git pass of walkProjectRoot rather than reusing it
// because the two questions differ: that walk answers "which directory owns the
// backlog" and falls back to dir, while this one must be able to answer "no
// repo here" at all.
func gitRepoRoot(dir string) string {
	if dir == "" {
		return ""
	}
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}
