package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Revision describes what changed in a record since the person last reviewed
// it, read from the store's own history: the file at the last review commit
// against the file now, field by field. It reads commit subjects, which
// docs/ddd/domain-events.md makes a contract for exactly this reason. The
// date it names is meta.Updated — the kernel's own "when this changed",
// which a test's clock controls — not the commit's own timestamp, which can
// differ from it by seconds.
func (s *Store) Revision(rel string, meta Meta) (string, error) {
	if meta.Reviewed.IsZero() || !meta.Updated.After(meta.Reviewed) {
		return "", nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out, err := s.git(s.Dir, "log", "--format=%H%x09%s", "--", rel)
	if err != nil {
		return "", err
	}
	var reviewHash string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		hash, subject, _ := strings.Cut(line, "\t")
		if strings.HasPrefix(subject, "review ") {
			reviewHash = hash
			break
		}
	}
	if reviewHash == "" {
		return "", nil
	}
	then, err := s.git(s.Dir, "show", reviewHash+":"+rel)
	if err != nil {
		return "", err
	}
	now, err := os.ReadFile(filepath.Join(s.Dir, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	old, _ := ParseRecord(then)
	cur, _ := ParseRecord(string(now))
	var changes []string
	keys := map[string]bool{}
	for k := range old.Fields {
		keys[k] = true
	}
	for k := range cur.Fields {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		if old.Fields[k] != cur.Fields[k] {
			changes = append(changes, fmt.Sprintf("%s moved from %s to %s", k, old.Fields[k], cur.Fields[k]))
		}
	}
	if len(changes) == 0 && strings.TrimSpace(old.Body) != strings.TrimSpace(cur.Body) {
		changes = append(changes, "the text was reworded")
	}
	if len(changes) == 0 {
		return "", nil
	}
	return strings.Join(changes, "; ") + " on " + meta.Updated.UTC().Format("2006-01-02"), nil
}
