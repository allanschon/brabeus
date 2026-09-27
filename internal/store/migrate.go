package store

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// MigrateReport says what one run did.
type MigrateReport struct {
	Retagged int    // files rewritten
	Tagged   int    // files that already carried a module; untouched
	Stamped  int    // of the retagged, files that had no parseable updated stamp and were given one
	Commit   string // short sha, or "" when nothing changed
}

// Migrate retags every pre-module record — one with `type` and no `module` —
// through the module set's legacy_types (decision D2), in one commit. Paths
// and the index are untouched and each file keeps its own updated stamp: a
// retag is not a content change (spec §5).
//
// All or nothing. Every file is classified before any is written, so a file
// the set cannot map aborts the run with nothing on disk and nothing in git.
// A second run finds nothing to do and makes no commit.
func (s *Store) Migrate(caller string) (MigrateReport, error) {
	var rep MigrateReport
	if s.ReadOnly {
		return rep, fmt.Errorf("this store is read-only")
	}
	if s.modules == nil {
		return rep, fmt.Errorf("no module set is loaded; nothing to migrate into")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.sync(); err != nil {
		return rep, err
	}

	type change struct {
		full    string
		content string
	}
	var changes []change
	var problems []string
	err := s.walkMarkdown(func(rel, full string) error {
		if strings.EqualFold(rel, indexFile) || strings.EqualFold(rel, conventionsFile) {
			return nil
		}
		b, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		r, meta := ParseRecord(string(b))
		if r.Module != "" {
			rep.Tagged++
			return nil
		}
		// A kernel key present but unparseable is a problem Migrate cannot
		// map either: composing anyway would silently drop it, and only a
		// review may move reviewed/retired/snoozes (§9). Same helper Write and
		// Review refuse with, so the three agree on the wording.
		if len(meta.Malformed) > 0 {
			problems = append(problems, rel+": "+malformedError(parseFrontmatter(string(b)), meta.Malformed).Error())
			return nil
		}
		// Normalised the same way Write normalises r.Type before its own
		// LegacyKind lookup (store.go): an outbox file queued before this PR
		// — or any pre-module writer — may carry "type: Reference" or
		// "  feedback  ", and an exact-match lookup would abort the whole
		// migration on a type Write itself would have accepted.
		legacyType := strings.ToLower(strings.TrimSpace(meta.LegacyType))
		if legacyType == "" {
			problems = append(problems, rel+": neither type nor module")
			return nil
		}
		m, k, ok := s.modules.LegacyKind(legacyType)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: type %q maps to no enabled module's kind", rel, legacyType))
			return nil
		}
		r.Module, r.Kind = m, k
		meta.LegacyType = ""
		if meta.Updated.IsZero() {
			// No stamp, or a bare date from before the RFC3339 rule: give it
			// one, as staleStamp would on the next rewrite, and say so.
			meta.Updated = now()
			rep.Stamped++
		}
		changes = append(changes, change{full, compose(r, meta, nil)})
		return nil
	})
	if err != nil {
		return rep, err
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return rep, fmt.Errorf("migration aborted, nothing written; %d file(s) cannot be mapped:\n  %s",
			len(problems), strings.Join(problems, "\n  "))
	}
	if len(changes) == 0 {
		return rep, nil
	}
	for _, c := range changes {
		if err := os.WriteFile(c.full, []byte(c.content), 0o640); err != nil {
			return rep, err
		}
	}
	rep.Retagged = len(changes)

	msg := fmt.Sprintf("migrate: tag %d records with module and kind\n\nEvery record that carried a pre-module type now names its module and kind, through the memory module's legacy_types. Paths unchanged; updated stamps kept, except %d that had none and were given one. Migrated through the kernel at %s.",
		rep.Retagged, rep.Stamped, time.Now().UTC().Format(time.RFC3339))
	head, err := s.commitAndPush(msg, caller)
	if err != nil {
		return rep, err
	}
	rep.Commit = head
	return rep, nil
}
