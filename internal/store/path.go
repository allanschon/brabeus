package store

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// canonicalPath validates a store-relative path and returns the one spelling
// the store uses for it everywhere: on disk, in the index, in commit messages
// and in what it tells the caller. ./a//b.md and a/b.md are the same file, and
// an index keyed by the caller's spelling would carry two lines for it.
//
// This is a boundary rather than a sanity check: the path arrives inside an MCP
// tool call, so it is untrusted input by construction.
func CanonicalPath(rel string) (string, error) {
	if strings.ContainsRune(rel, '\\') {
		return "", fmt.Errorf("path contains a backslash, which is not a separator here: %q", rel)
	}
	if rel == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("path must be relative to the store root: %q", rel)
	}
	clean := path.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path escapes the store root: %q", rel)
	}
	// The working copy's .git is inside the root but is not the store. Reading
	// it leaks the remote and the committer; removing from it breaks the clone
	// in a way Ensure does not repair, since it only re-clones when .git is
	// absent altogether.
	if first, _, _ := strings.Cut(clean, "/"); strings.EqualFold(first, ".git") {
		return "", fmt.Errorf("path is inside the repository's own .git: %q", rel)
	}
	return clean, nil
}

// resolvePath maps a store-relative path to an absolute one, refusing anything
// that would land outside root.
func resolvePath(root, rel string) (string, error) {
	clean, err := CanonicalPath(rel)
	if err != nil {
		return "", err
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	full := filepath.Join(absRoot, filepath.FromSlash(clean))

	// Compare against root plus a separator. Comparing against the bare prefix
	// would accept a sibling directory whose name starts with the root's.
	if !strings.HasPrefix(full, absRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the store root: %q", rel)
	}
	return full, nil
}
