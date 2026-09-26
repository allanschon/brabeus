package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// resolvePath is a security boundary, not a convenience: the path arrives
// inside a tool call, so it is attacker-shaped input by construction.

func TestResolvePathAcceptsPathsInsideTheRoot(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"MEMORY.md",
		"personal/working-style.md",
		"projects/example--repo/Gaming/notes.md",
		"a/../b.md", // legal normalisation that stays inside
	} {
		got, err := resolvePath(root, rel)
		if err != nil {
			t.Fatalf("resolvePath(%q) returned an error: %v", rel, err)
		}
		if !strings.HasPrefix(got, root+string(filepath.Separator)) {
			t.Errorf("resolvePath(%q) = %q, which is not under %q", rel, got, root)
		}
	}
}

func TestResolvePathRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"../escape.md",
		"../../etc/passwd",
		"personal/../../escape.md",
		"/etc/passwd",          // absolute
		"..\\windows\\path.md", // backslash separators
		"",
		".",
		"..",
	} {
		if got, err := resolvePath(root, rel); err == nil {
			t.Errorf("resolvePath(%q) = %q with no error; it must be refused", rel, got)
		}
	}
}

// A root that is a prefix of a sibling directory is the classic prefix bug:
// /var/lib/mem must not accept a path resolving into /var/lib/mem-other.
func TestResolvePathIsNotFooledByASiblingWithTheSamePrefix(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "mem")
	if _, err := resolvePath(root, "../mem-other/secret.md"); err == nil {
		t.Error("a sibling directory sharing the root's prefix was accepted")
	}
}

// canonicalPath is the one spelling of a path the store uses everywhere: on
// disk, in the index, in commit messages and in what it tells the caller.
func TestCanonicalPathNormalisesEquivalentSpellings(t *testing.T) {
	for _, rel := range []string{"projects/foo.md", "./projects/foo.md", "projects//foo.md", "projects/./foo.md", "projects/bar/../foo.md"} {
		got, err := canonicalPath(rel)
		if err != nil {
			t.Fatalf("canonicalPath(%q): %v", rel, err)
		}
		if got != "projects/foo.md" {
			t.Errorf("canonicalPath(%q) = %q", rel, got)
		}
	}
}

// The working copy's .git directory is inside the root but is not the store.
// Reading it leaks the remote and the committer; deleting from it breaks the
// clone in a way Ensure does not repair.
func TestCanonicalPathRefusesGitInternals(t *testing.T) {
	for _, rel := range []string{".git", ".git/", ".git/config", ".git/HEAD", "a/../.git/HEAD", "./.git/x.md", ".GIT/config"} {
		if got, err := canonicalPath(rel); err == nil {
			t.Errorf("canonicalPath(%q) = %q with no error", rel, got)
		}
	}
	// A memory may mention git in its name; only the top-level .git is special.
	for _, rel := range []string{"infra/.gitea-notes.md", "git/x.md", "infra/.git-hooks.md", "projects/.github.md"} {
		if _, err := canonicalPath(rel); err != nil {
			t.Errorf("canonicalPath(%q) was refused: %v", rel, err)
		}
	}
}
