package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBranchSlug pins the reduction free text goes through before it becomes
// half a branch name. The character set is the safety property: git's ref rules
// are a list of prohibitions, and [a-z0-9-] with no edge dashes cannot spell any
// of them.
func TestBranchSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Fix the sidebar", "fix-the-sidebar"},
		{"  Trim   the   gaps  ", "trim-the-gaps"},
		{"Punctuation: it's here!", "punctuation-it-s-here"},
		{"CVE-2026-1234", "cve-2026-1234"},
		{"---leading and trailing---", "leading-and-trailing"},
		{"", ""},
		{"!!!", ""},               // nothing survives; todoBranchName supplies the fallback
		{"…ellipsis", "ellipsis"}, // firstLine's truncation marker is not a branch character
	}
	for _, c := range cases {
		if got := branchSlug(c.in); got != c.want {
			t.Errorf("branchSlug(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	t.Run("capped without a trailing separator", func(t *testing.T) {
		got := branchSlug(strings.Repeat("word ", 30))
		if len(got) > branchSlugMaxRunes {
			t.Errorf("branchSlug = %q (%d bytes), want at most %d", got, len(got), branchSlugMaxRunes)
		}
		if strings.HasSuffix(got, "-") || strings.HasPrefix(got, "-") {
			t.Errorf("branchSlug = %q, want no edge separators", got)
		}
	})

	t.Run("only safe characters survive", func(t *testing.T) {
		got := branchSlug("a..b @{c} d\te/f.lock")
		for _, r := range got {
			ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
			if !ok {
				t.Fatalf("branchSlug = %q, contains %q", got, r)
			}
		}
	})
}

// TestTodoBranchName covers the whole generated branch: the namespace prefix,
// the readable middle, and the suffix that keeps a second drop of the same todo
// from colliding with the first (`git worktree add -b` refuses an existing
// branch).
func TestTodoBranchName(t *testing.T) {
	t.Run("titles the branch after the todo", func(t *testing.T) {
		got := todoBranchName(Todo{Title: "Fix the sidebar"}, 0x1234)
		if want := "todo/fix-the-sidebar-1234"; got != want {
			t.Errorf("todoBranchName = %q, want %q", got, want)
		}
	})

	t.Run("falls back to the prompt's first line", func(t *testing.T) {
		got := todoBranchName(Todo{Prompt: "\n\nMake the tests pass\nand then some"}, 0)
		if !strings.HasPrefix(got, worktreeBranchPrefix+"make-the-tests-pass") {
			t.Errorf("todoBranchName = %q, want it derived from the prompt's first line", got)
		}
	})

	t.Run("nameless todos still get a branch", func(t *testing.T) {
		got := todoBranchName(Todo{Prompt: "!!!"}, 0xabcd)
		if want := "todo/todo-abcd"; got != want {
			t.Errorf("todoBranchName = %q, want %q", got, want)
		}
	})

	t.Run("the same todo dropped twice gets two branches", func(t *testing.T) {
		td := Todo{ID: "same", Title: "Same todo"}
		if a, b := todoBranchName(td, 1), todoBranchName(td, 2); a == b {
			t.Errorf("both drops named the branch %q; a second drop would fail to create it", a)
		}
	})
}

// TestGitRepoRoot covers the picker's gate on the worktree rows.
func TestGitRepoRoot(t *testing.T) {
	t.Run("no repo above", func(t *testing.T) {
		// t.TempDir sits under the OS temp root, which is never inside a repo.
		if got := gitRepoRoot(t.TempDir()); got != "" {
			t.Errorf("gitRepoRoot = %q, want \"\" outside a repo", got)
		}
	})

	t.Run("empty dir", func(t *testing.T) {
		if got := gitRepoRoot(""); got != "" {
			t.Errorf("gitRepoRoot(\"\") = %q, want \"\"", got)
		}
	})

	t.Run("found from a subdirectory", func(t *testing.T) {
		root := t.TempDir()
		mkdir(t, filepath.Join(root, ".git"))
		sub := filepath.Join(root, "a", "b")
		mkdir(t, sub)
		if got := gitRepoRoot(sub); got != root {
			t.Errorf("gitRepoRoot = %q, want the repo root %q", got, root)
		}
	})

	t.Run("a linked worktree's .git file counts", func(t *testing.T) {
		// This is the checkout shape a worktree drop itself produces: .git is a
		// file pointing into the main repo. A manager opened there must still be
		// able to cut the next worktree.
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := gitRepoRoot(root); got != root {
			t.Errorf("gitRepoRoot = %q, want %q — a .git file is a checkout too", got, root)
		}
	})
}
