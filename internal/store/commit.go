package store

import (
	"fmt"
	"strings"
)

// commitAndPush is the tail every writer shares: stage whatever is on disk,
// skip the commit when nothing actually changed, commit under the calling
// machine's name, push, reindex and report the new HEAD. Write, Delete,
// Review and Migrate all end here so the four cannot drift into four
// different ideas of what "nothing changed" or "pushed" means.
func (s *Store) commitAndPush(msg, caller string) (string, error) {
	if _, err := s.git(s.Dir, "add", "-A"); err != nil {
		return "", err
	}
	status, err := s.git(s.Dir, "status", "--porcelain")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(status) == "" {
		return "no change", nil
	}
	if _, err := s.git(s.Dir, "commit", "--quiet", "--author", authorFor(caller), "-m", msg); err != nil {
		return "", err
	}
	if _, err := s.git(s.Dir, "push", "--quiet", "origin", s.Branch); err != nil {
		return "", err
	}
	if err := s.reindex(); err != nil {
		return "", err
	}
	head, err := s.git(s.Dir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(head), nil
}

// malformedError names every kernel-owned key on a record that failed to
// parse, in the one wording Write, Review and Migrate all use: composing
// over a stamp that did not parse would silently erase it — only a review
// may move reviewed/retired/snoozes (§9) — so this refuses instead, naming
// what it saw and saying nothing was written.
func malformedError(fm map[string]string, keys []string) error {
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		want := "an RFC3339 stamp"
		if key == "snoozes" {
			want = "an integer"
		}
		parts = append(parts, fmt.Sprintf("%s: %q is not %s", key, fm[key], want))
	}
	return fmt.Errorf("%s; fix the file by hand, nothing was written", strings.Join(parts, "; "))
}
