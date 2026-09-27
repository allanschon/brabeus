package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// legacyFile is a record as the store wrote it before modules existed.
func legacyFile(name, typ, scope, body string) string {
	return "---\nname: " + name + "\ndescription: about " + name + "\ntype: " + typ +
		"\nscope: " + scope + "\nupdated: 2026-09-01T12:00:00Z\n---\n\n" + body + "\n"
}

// seedLegacy pushes pre-module files into the remote the way an older server
// would have left them, then re-syncs the store so it sees them.
func seedLegacy(t *testing.T, s *Store, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(s.Dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run(t, s.Dir, "add", "-A")
	run(t, s.Dir, "commit", "-qm", "legacy files")
	run(t, s.Dir, "push", "-q", "origin", "main")
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
}

func commits(t *testing.T, s *Store) int {
	t.Helper()
	return len(strings.Split(strings.TrimSpace(run(t, s.Dir, "rev-list", "HEAD")), "\n"))
}

func paths(hits []Match) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Path)
	}
	return out
}

func TestMigrateRetagsEveryLegacyFileInOneCommit(t *testing.T) {
	s := newTestStore(t, newTestRemote(t)) // the seed already holds personal/style.md (feedback) and infra/delta.md (reference)
	seedLegacy(t, s, map[string]string{
		"personal/u.md":   legacyFile("u", "user", "global", "Plain speech, always."),
		"projects/x/p.md": legacyFile("p", "project", "project/x", "The NAS project."),
		"infra/tagged.md": "---\nname: tagged\ndescription: already\nmodule: memory\nkind: note\nscope: global\nupdated: 2026-09-01T12:00:00Z\n---\n\nAlready tagged.\n",
	})
	// The two seed files (personal/style.md, infra/delta.md) carry no updated
	// stamp, so a restorable clock is needed for the duration of this test.
	defer fixClock(t, "2026-10-01T09:00:00Z")()
	queries := []string{"plain speech", "NAS", "already tagged"}
	before := map[string][]string{}
	for _, q := range queries {
		hits, _, err := s.Search(q, 10, SearchFilter{})
		if err != nil {
			t.Fatal(err)
		}
		before[q] = paths(hits)
	}
	n := commits(t, s)

	rep, err := s.Migrate("test-machine")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Retagged != 4 || rep.Tagged != 1 || rep.Stamped != 2 || rep.Commit == "" {
		t.Errorf("report = %+v, want 4 retagged, 1 already tagged, 2 stamped, a commit", rep)
	}
	if got := commits(t, s); got != n+1 {
		t.Errorf("commits went %d -> %d, want exactly one more", n, got)
	}
	want := map[string]string{"personal/style.md": "preference", "infra/delta.md": "note", "personal/u.md": "note", "projects/x/p.md": "project"}
	for rel, kind := range want {
		r, meta := ParseRecord(mustRead(t, filepath.Join(s.Dir, rel)))
		if r.Module != "memory" || r.Kind != kind {
			t.Errorf("%s: module=%q kind=%q, want memory/%s", rel, r.Module, r.Kind, kind)
		}
		if meta.LegacyType != "" {
			t.Errorf("%s still carries type", rel)
		}
		wantStamp := "2026-09-01T12:00:00Z" // the legacyFile records keep their own
		if rel == "personal/style.md" || rel == "infra/delta.md" {
			wantStamp = "2026-10-01T09:00:00Z" // the seed files had none and were given the clock
		}
		if got := meta.Updated.Format("2006-01-02T15:04:05Z"); got != wantStamp {
			t.Errorf("%s: updated = %s, want %s", rel, got, wantStamp)
		}
	}
	if !strings.Contains(run(t, s.Dir, "log", "-1", "--format=%s"), "4") {
		t.Errorf("the commit subject should count what it retagged: %q", run(t, s.Dir, "log", "-1", "--format=%s"))
	}
	for _, q := range queries {
		hits, _, err := s.Search(q, 10, SearchFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if got := paths(hits); strings.Join(got, ",") != strings.Join(before[q], ",") {
			t.Errorf("search %q: before %v, after %v", q, before[q], got)
		}
	}
	// The index and the remote: MEMORY.md untouched, the commit pushed.
	if run(t, s.Dir, "diff", "HEAD~1", "--", "MEMORY.md") != "" {
		t.Error("MEMORY.md must not change")
	}
	if run(t, s.Dir, "rev-parse", "HEAD") != run(t, s.Dir, "rev-parse", "origin/main") {
		t.Error("the migration commit must be pushed")
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Migrate("test-machine"); err != nil {
		t.Fatal(err)
	}
	n := commits(t, s)
	rep, err := s.Migrate("test-machine")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Retagged != 0 || rep.Commit != "" || commits(t, s) != n {
		t.Errorf("second run: %+v, commits %d -> %d", rep, n, commits(t, s))
	}
}

func TestMigrateAbortsWholeOnAFileItCannotMap(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	seedLegacy(t, s, map[string]string{
		"infra/odd.md":  legacyFile("odd", "rumour", "global", "no such type"),
		"infra/none.md": "---\nname: none\ndescription: d\nscope: global\n---\n\nneither type nor module\n",
	})
	n := commits(t, s)
	_, err := s.Migrate("test-machine")
	if err == nil || !strings.Contains(err.Error(), "infra/odd.md") || !strings.Contains(err.Error(), "infra/none.md") {
		t.Fatalf("want an error naming both files, got %v", err)
	}
	if commits(t, s) != n {
		t.Error("nothing may be committed on abort")
	}
	if r, _ := ParseRecord(mustRead(t, filepath.Join(s.Dir, "personal/style.md"))); r.Module != "" {
		t.Error("nothing may be written on abort, not even the files that would have mapped")
	}
}

// A kernel key present but unparseable (Meta.Malformed) is a problem Migrate
// cannot map either: composing anyway would silently drop it, and only a
// review may move reviewed/retired/snoozes (§9). It aborts the whole run the
// same way a file with no mappable type does.
func TestMigrateAbortsOnAMalformedKernelKey(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	seedLegacy(t, s, map[string]string{
		"infra/bad.md": "---\nname: bad\ndescription: d\ntype: reference\nscope: global\nreviewed: 2026-09-05\n---\n\nbody\n",
	})
	n := commits(t, s)
	_, err := s.Migrate("test-machine")
	if err == nil || !strings.Contains(err.Error(), "infra/bad.md: reviewed is malformed; fix the file by hand") {
		t.Fatalf("want an error naming the file and the malformed key, got %v", err)
	}
	if commits(t, s) != n {
		t.Error("nothing may be committed on abort")
	}
	if r, _ := ParseRecord(mustRead(t, filepath.Join(s.Dir, "personal/style.md"))); r.Module != "" {
		t.Error("nothing may be written on abort, not even the files that would have mapped")
	}
}

func TestMigrateRefusesAReadOnlyStore(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	s.ReadOnly = true
	if _, err := s.Migrate("test-machine"); err == nil {
		t.Error("a read-only store cannot migrate")
	}
}
