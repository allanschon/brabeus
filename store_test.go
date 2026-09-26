package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// These tests run against a real git repository rather than a mock. The write
// path is pull-write-commit-push, and the failures worth catching live in git's
// behaviour, not in our description of it.

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// newTestRemote builds a bare repository standing in for the record, seeded
// the way the real store was bootstrapped.
func newTestRemote(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	bare := filepath.Join(base, "record.git")
	run(t, base, "init", "--bare", "-b", "main", bare)

	seed := filepath.Join(base, "seed")
	run(t, base, "clone", "-q", bare, seed)
	write := func(rel, content string) {
		full := filepath.Join(seed, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("MEMORY.md", baseIndex)
	write("personal/style.md", "---\nname: style\ndescription: how the person likes it\ntype: feedback\nscope: global\n---\n\nPlain speech.\n")
	write("infra/delta.md", "---\nname: delta\ndescription: the NAS box\ntype: reference\nscope: machine/delta\n---\n\nNAS 7.3.2.\n")
	write("CONVENTIONS.md", "# House style\n\nPlain speech. Short sentences. No frontmatter here; this file is not a memory.\n")
	run(t, seed, "add", "-A")
	run(t, seed, "commit", "-q", "-m", "seed")
	run(t, seed, "push", "-q", "origin", "main")
	return bare
}

func newTestStore(t *testing.T, remote string) *Store {
	t.Helper()
	s := &Store{
		Dir:         filepath.Join(t.TempDir(), "work"),
		RemoteURL:   remote,
		Branch:      "main",
		CommitName:  "brabeus",
		CommitEmail: "brabeus@example.com",
	}
	if err := s.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	return s
}

func TestEnsureClonesWhenTheWorkingCopyIsMissing(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := os.Stat(filepath.Join(s.Dir, "MEMORY.md")); err != nil {
		t.Fatalf("working copy was not populated: %v", err)
	}
}

func TestEnsureIsSafeToRunTwice(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if err := s.Ensure(); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
}

func TestListReportsFrontmatterForEachMemory(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	entries, err := s.List("")
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]Entry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}
	got, ok := byPath["infra/delta.md"]
	if !ok {
		t.Fatalf("infra/delta.md missing from %v", byPath)
	}
	if got.Scope != "machine/delta" || got.Type != "reference" || got.Description != "the NAS box" {
		t.Errorf("frontmatter not reported: %+v", got)
	}
	if _, ok := byPath["MEMORY.md"]; !ok {
		t.Error("the index itself should be listed")
	}
}

func TestListHonoursAPrefix(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	entries, err := s.List("infra/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != "infra/delta.md" {
		t.Errorf("prefix filter returned %v", entries)
	}
}

func TestReadReturnsContentAndRefusesEscapes(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	got, err := s.Read("personal/style.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Plain speech.") {
		t.Errorf("content missing: %q", got)
	}
	if _, err := s.Read("../../etc/passwd"); err == nil {
		t.Error("Read accepted a path outside the store")
	}
}

func TestSearchIsCaseInsensitiveAndReportsWhere(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	hits, _, err := s.Search("PLAIN SPEECH", 50, SearchFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1: %v", len(hits), hits)
	}
	if hits[0].Path != "personal/style.md" || hits[0].Line == 0 {
		t.Errorf("hit does not locate the match: %+v", hits[0])
	}
}

func TestSearchHonoursTheLimit(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	for _, p := range []string{"infra/a.md", "infra/b.md", "infra/c.md"} {
		if _, err := s.Write(p, Memory{
			Name: "n", Description: "the UPS battery", Type: "reference",
			Scope: "global", Body: "battery",
		}, "test-machine"); err != nil {
			t.Fatal(err)
		}
	}
	// Without a limit all three rank, so a returned 2 is the limit working
	// rather than the corpus running out.
	all, _, err := s.Search("battery", 10, SearchFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d candidates, want 3 before the limit is tested", len(all))
	}
	hits, _, err := s.Search("battery", 2, SearchFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Errorf("limit ignored: got %d hits, want 2", len(hits))
	}
}

func TestSearchRanksMemoriesAndReturnsOnePerFile(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("infra/ups.md", Memory{
		Name: "ups", Description: "the UPS battery", Type: "reference", Scope: "global",
		Body: "battery\nbattery\nbattery\n",
	}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	hits, _, err := s.Search("battery", 10, SearchFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want one per memory rather than one per line", len(hits))
	}
	if hits[0].Path != "infra/ups.md" || hits[0].Score <= 0 {
		t.Errorf("hit = %+v, want infra/ups.md with a positive score", hits[0])
	}
	if hits[0].Line == 0 || hits[0].Text == "" {
		t.Errorf("hit = %+v, want the line it matched on", hits[0])
	}
	// Distinguishes a real body line from "description: the UPS battery",
	// which also carries the term and, before the frontmatter fix, used to
	// win this on the earliest-wins tie-break.
	if hits[0].Text != "battery" {
		t.Errorf("hit = %+v, want the body line, not a frontmatter line", hits[0])
	}
}

func TestSearchFiltersByScopeAndType(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	write := func(path, typ, scope string) {
		t.Helper()
		if _, err := s.Write(path, Memory{
			Name: "n", Description: "the UPS battery", Type: typ, Scope: scope, Body: "battery",
		}, "test-machine"); err != nil {
			t.Fatal(err)
		}
	}
	write("infra/a.md", "reference", "global")
	write("infra/b.md", "feedback", "global")
	write("infra/c.md", "reference", "machine/delta")

	for _, c := range []struct {
		filter SearchFilter
		want   string
	}{
		{SearchFilter{Type: "feedback"}, "infra/b.md"},
		{SearchFilter{Scope: "machine/delta"}, "infra/c.md"},
	} {
		hits, _, err := s.Search("battery", 10, c.filter)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 1 || hits[0].Path != c.want {
			t.Errorf("filter %+v returned %+v, want only %s", c.filter, hits, c.want)
		}
	}
}

// Prefix has to agree with List about what a directory is: infra and
// infrastructure/ share a string prefix but are not the same directory. A raw
// strings.HasPrefix, an inverted condition, or a dropped Trim would all leave
// this untested territory, so this asserts the exact set of paths each prefix
// returns rather than merely a count.
func TestSearchFiltersByPrefixLikeADirectoryNotAStringPrefix(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	write := func(path string) {
		t.Helper()
		if _, err := s.Write(path, Memory{
			Name: "n", Description: "the UPS battery", Type: "reference", Scope: "global", Body: "battery",
		}, "test-machine"); err != nil {
			t.Fatal(err)
		}
	}
	write("infra/a.md")
	write("infra/b.md")
	write("infrastructure/z.md")

	paths := func(hits []Match) []string {
		out := make([]string, len(hits))
		for i, h := range hits {
			out[i] = h.Path
		}
		return out
	}
	same := func(got, want []string) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	for _, c := range []struct {
		prefix string
		want   []string
	}{
		// Every candidate here has an identical body, so ties break on path —
		// infra/a.md before infra/b.md — and the expectation can be a fixed
		// order rather than a set comparison.
		{"infra", []string{"infra/a.md", "infra/b.md"}},
		{"./infra/", []string{"infra/a.md", "infra/b.md"}},
		// path.Clean keeps a leading slash that no memory path has, so a
		// dropped Trim would make this filter everything out silently.
		{"/infra", []string{"infra/a.md", "infra/b.md"}},
		{"inf", nil},
	} {
		hits, _, err := s.Search("battery", 10, SearchFilter{Prefix: c.prefix})
		if err != nil {
			t.Fatal(err)
		}
		if got := paths(hits); !same(got, c.want) {
			t.Errorf("prefix %q returned %v, want %v", c.prefix, got, c.want)
		}
	}
}

func TestWritePushesTheMemoryAndTheIndexToTheRemote(t *testing.T) {
	remote := newTestRemote(t)
	s := newTestStore(t, remote)

	if _, err := s.Write("infra/ups.md", Memory{
		Name: "ups", Description: "the UPS", Type: "reference", Scope: "global", Body: "48 minutes.",
	}, "test-machine"); err != nil {
		t.Fatal(err)
	}

	// Read the remote, not the working copy: the point is that it was pushed.
	got := run(t, remote, "show", "main:infra/ups.md")
	if !strings.Contains(got, "48 minutes.") || !strings.Contains(got, "name: ups") {
		t.Errorf("memory not on the remote:\n%s", got)
	}
	idx := run(t, remote, "show", "main:MEMORY.md")
	if !strings.Contains(idx, "](infra/ups.md)") {
		t.Errorf("index not updated on the remote:\n%s", idx)
	}
}

// Writing the same memory twice must not pile up empty commits.
func TestWritingIdenticalContentTwiceMakesOneCommit(t *testing.T) {
	remote := newTestRemote(t)
	s := newTestStore(t, remote)
	m := Memory{Name: "ups", Description: "the UPS", Type: "reference", Scope: "global", Body: "48 minutes."}

	if _, err := s.Write("infra/ups.md", m, "test-machine"); err != nil {
		t.Fatal(err)
	}
	before := strings.TrimSpace(run(t, remote, "rev-parse", "main"))
	if _, err := s.Write("infra/ups.md", m, "test-machine"); err != nil {
		t.Fatal(err)
	}
	after := strings.TrimSpace(run(t, remote, "rev-parse", "main"))
	if before != after {
		t.Errorf("an unchanged write moved the branch from %s to %s", before, after)
	}
}

func TestWriteRefusesAnythingThatIsNotMarkdown(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("infra/ups.txt", Memory{Name: "n", Description: "d", Type: "reference", Scope: "global"}, "test-machine"); err == nil {
		t.Error("a non-markdown path was accepted")
	}
}

func TestWriteRefusesToEscapeTheStore(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("../escape.md", Memory{Name: "n", Description: "d", Type: "reference", Scope: "global"}, "test-machine"); err == nil {
		t.Error("a path outside the store was accepted")
	}
}

func TestAReadOnlyStoreRefusesToWrite(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	s.ReadOnly = true
	if _, err := s.Write("infra/ups.md", Memory{Name: "n", Description: "d", Type: "reference", Scope: "global"}, "test-machine"); err == nil {
		t.Error("a read-only store accepted a write")
	}
}

// A previous failure can leave the working copy dirty. The next write must not
// inherit it, or one caller's abandoned edit ships inside another's commit.
func TestWriteDiscardsAnUnrelatedDirtyWorkingCopy(t *testing.T) {
	remote := newTestRemote(t)
	s := newTestStore(t, remote)

	stray := filepath.Join(s.Dir, "personal", "style.md")
	if err := os.WriteFile(stray, []byte("corrupted by an earlier failure\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Write("infra/ups.md", Memory{
		Name: "ups", Description: "the UPS", Type: "reference", Scope: "global", Body: "48 minutes.",
	}, "test-machine"); err != nil {
		t.Fatal(err)
	}

	got := run(t, remote, "show", "main:personal/style.md")
	if strings.Contains(got, "corrupted") {
		t.Error("a stray working-copy edit was pushed to the remote")
	}
}

// The store is shared, so another writer's commit must not be clobbered.
func TestWriteRebasesOntoWorkDoneElsewhere(t *testing.T) {
	remote := newTestRemote(t)
	s := newTestStore(t, remote)

	// Someone else pushes while our working copy is stale.
	other := filepath.Join(t.TempDir(), "other")
	run(t, filepath.Dir(other), "clone", "-q", remote, other)
	if err := os.WriteFile(filepath.Join(other, "personal", "elsewhere.md"),
		[]byte("---\nname: elsewhere\nscope: global\n---\n\nfrom another clone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, other, "add", "-A")
	run(t, other, "commit", "-q", "-m", "from elsewhere")
	run(t, other, "push", "-q", "origin", "main")

	if _, err := s.Write("infra/ups.md", Memory{
		Name: "ups", Description: "the UPS", Type: "reference", Scope: "global", Body: "48 minutes.",
	}, "test-machine"); err != nil {
		t.Fatal(err)
	}

	if out := run(t, remote, "show", "main:personal/elsewhere.md"); !strings.Contains(out, "from another clone") {
		t.Error("the other writer's commit was lost")
	}
	if out := run(t, remote, "show", "main:infra/ups.md"); !strings.Contains(out, "48 minutes.") {
		t.Error("our own write did not land")
	}
}

func TestDeleteRemovesTheMemoryAndItsIndexEntryFromTheRemote(t *testing.T) {
	remote := newTestRemote(t)
	s := newTestStore(t, remote)

	if _, err := s.Write("infra/ups.md", Memory{
		Name: "ups", Description: "the UPS", Type: "reference", Scope: "global", Body: "48 minutes."}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Delete("infra/ups.md", "test-machine"); err != nil {
		t.Fatal(err)
	}

	// Read the remote: the point is that the deletion was pushed.
	cmd := exec.Command("git", "show", "main:infra/ups.md")
	cmd.Dir = remote
	if err := cmd.Run(); err == nil {
		t.Error("the memory is still on the remote")
	}
	if idx := run(t, remote, "show", "main:MEMORY.md"); strings.Contains(idx, "](infra/ups.md)") {
		t.Errorf("the index still points at the deleted memory:\n%s", idx)
	}
}

// A typo must not read as a successful removal. Silent success here means
// believing something is gone when it is not.
func TestDeleteFailsLoudlyOnAPathThatIsNotThere(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Delete("infra/never-existed.md", "test-machine"); err == nil {
		t.Error("deleting an absent path succeeded")
	}
}

func TestDeleteRefusesToEscapeTheStore(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Delete("../../etc/passwd", "test-machine"); err == nil {
		t.Error("a path outside the store was accepted")
	}
}

// The index is not a memory. Removing it would break every future write.
func TestDeleteRefusesToRemoveTheIndexItself(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Delete("MEMORY.md", "test-machine"); err == nil {
		t.Error("MEMORY.md was deletable")
	}
	// The guard must see the path the filesystem sees, not the spelling the
	// caller used, or ./MEMORY.md walks straight past it.
	if _, err := s.Delete("./MEMORY.md", "test-machine"); err == nil {
		t.Error("./MEMORY.md was deletable")
	}
}

// Writing a memory over the index replaces it with a memory that carries its
// own index line, and every later write extends the wreckage. Same for the
// conventions file.
func TestWriteRefusesToOverwriteTheStoresStructure(t *testing.T) {
	remote := newTestRemote(t)
	s := newTestStore(t, remote)
	m := Memory{Name: "n", Description: "d", Type: "user", Scope: "global", Body: "x"}
	for _, rel := range []string{"MEMORY.md", "./MEMORY.md", "CONVENTIONS.md"} {
		if _, err := s.Write(rel, m, "test-machine"); err == nil {
			t.Errorf("%s was writable as a memory", rel)
		}
	}
	got := run(t, remote, "show", "main:MEMORY.md")
	if got != baseIndex {
		t.Errorf("the index on the remote changed:\n%s", got)
	}
}

func TestAReadOnlyStoreRefusesToDelete(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	s.ReadOnly = true
	if _, err := s.Delete("personal/style.md", "test-machine"); err == nil {
		t.Error("a read-only store accepted a delete")
	}
}

// Another clone pushing while ours is stale must not be clobbered by a delete,
// same as for a write.
func TestDeleteKeepsWorkPushedElsewhere(t *testing.T) {
	remote := newTestRemote(t)
	s := newTestStore(t, remote)

	other := filepath.Join(t.TempDir(), "other")
	run(t, filepath.Dir(other), "clone", "-q", remote, other)
	if err := os.WriteFile(filepath.Join(other, "personal", "elsewhere.md"),
		[]byte("---\nname: elsewhere\nscope: global\n---\n\nfrom another clone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, other, "add", "-A")
	run(t, other, "commit", "-q", "-m", "from elsewhere")
	run(t, other, "push", "-q", "origin", "main")

	if _, err := s.Delete("personal/style.md", "test-machine"); err != nil {
		t.Fatal(err)
	}
	if out := run(t, remote, "show", "main:personal/elsewhere.md"); !strings.Contains(out, "from another clone") {
		t.Error("the other writer's commit was lost")
	}
}

// The store's own history is how "when did each machine last write" is
// answered, so the calling machine has to survive onto the commit. Author
// rather than committer: the SERVER committed, on BEHALF of the machine.
func TestWriteStampsCallerAsAuthor(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("personal/x.md", Memory{
		Name: "x", Description: "d", Type: "reference", Scope: "global", Body: "b",
	}, "beta"); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := s.git(s.Dir, "log", "-1", "--format=%an")
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if got := strings.TrimSpace(out); got != "beta" {
		t.Fatalf("author = %q, want beta", got)
	}
}

// An unresolved caller must not produce a malformed --author, which would fail
// the commit and lose the memory. Empty is a real case: identity.go returns ""
// for a caller arriving from outside the Tailscale network that it cannot name.
func TestWriteWithNoCallerCommitsAsUnknown(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("personal/y.md", Memory{
		Name: "y", Description: "d", Type: "reference", Scope: "global", Body: "b",
	}, ""); err != nil {
		t.Fatalf("write with empty caller must still commit: %v", err)
	}
	out, _ := s.git(s.Dir, "log", "-1", "--format=%an")
	if got := strings.TrimSpace(out); got != "unknown" {
		t.Fatalf("author = %q, want unknown", got)
	}
}

// Delete is a machine talking to the server too; leaving it unstamped would
// make the history lie by omission.
func TestDeleteStampsCallerAsAuthor(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("personal/z.md", Memory{
		Name: "z", Description: "d", Type: "reference", Scope: "global", Body: "b",
	}, "beta"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := s.Delete("personal/z.md", "gamma"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	out, _ := s.git(s.Dir, "log", "-1", "--format=%an")
	if got := strings.TrimSpace(out); got != "gamma" {
		t.Fatalf("author = %q, want gamma", got)
	}
}

// memoryPath is the one rule for what a memory's path may be, shared by Write
// and Delete so the two cannot drift. It hands back the canonical spelling, so
// every spelling that lands on the index is refused, a memory that merely
// shares the name in a subdirectory is not, and the structural names are
// matched case-insensitively so a case-folding filesystem cannot be talked
// into the same inode by a different spelling.
func TestMemoryPathAppliesOneRuleAndReturnsTheCanonicalSpelling(t *testing.T) {
	for _, rel := range []string{"MEMORY.md", "./MEMORY.md", "sub/../MEMORY.md", "memory.md", "CONVENTIONS.md", "conventions.md",
		"infra/notes.txt", "infra/notes", "../escape.md", ""} {
		if got, err := memoryPath(rel); err == nil {
			t.Errorf("%q was accepted as %q", rel, got)
		}
	}
	for rel, want := range map[string]string{
		"notes/MEMORY.md":  "notes/MEMORY.md",
		"foo.md":           "foo.md",
		"MEMORY.md.bak.md": "MEMORY.md.bak.md",
		"./infra//ups.md":  "infra/ups.md",
	} {
		got, err := memoryPath(rel)
		if err != nil {
			t.Errorf("%q was refused: %v", rel, err)
		} else if got != want {
			t.Errorf("memoryPath(%q) = %q, want %q", rel, got, want)
		}
	}
}

// Delete applies the same "what is a memory" rule as Write. Without it, a
// tracked file that is not a memory could be removed through the memory tool.
func TestDeleteRefusesAnythingThatIsNotMarkdown(t *testing.T) {
	remote := newTestRemote(t)
	s := newTestStore(t, remote)
	if err := os.WriteFile(filepath.Join(s.Dir, "infra", "notes.txt"), []byte("not a memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, s.Dir, "add", "-A")
	run(t, s.Dir, "commit", "-q", "-m", "a tracked non-memory")
	run(t, s.Dir, "push", "-q", "origin", "main")

	if _, err := s.Delete("infra/notes.txt", "test-machine"); err == nil {
		t.Error("a non-markdown path was deletable")
	}
	if got := run(t, remote, "show", "main:infra/notes.txt"); got != "not a memory\n" {
		t.Errorf("the file was removed from the remote: %q", got)
	}
}

// The list argument names a directory, whatever it is called. infra, infra/,
// ./infra and infra/. are one directory, and none of them is infrastructure/.
func TestListPrefixIsADirectory(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("infrastructure/x.md", Memory{Name: "x", Description: "d", Type: "reference", Scope: "global"}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"infra", "infra/", "./infra", "infra/.", "./infra//"} {
		got, err := s.List(prefix)
		if err != nil {
			t.Fatalf("List(%q): %v", prefix, err)
		}
		if len(got) != 1 || got[0].Path != "infra/delta.md" {
			t.Errorf("List(%q) = %+v, want the one infra memory", prefix, got)
		}
	}
}

// resolvePath cleans the path before touching the disk, so two spellings of
// one path are one file. The index, the section it is filed under and the
// delete that removes it must agree with the disk, not with the caller.
func TestWriteAndDeleteKeyTheIndexByTheCanonicalPath(t *testing.T) {
	remote := newTestRemote(t)
	s := newTestStore(t, remote)
	m := Memory{Name: "foo", Description: "d", Type: "reference", Scope: "global", Body: "x"}
	for _, rel := range []string{"./projects/foo.md", "projects//foo.md"} {
		if _, err := s.Write(rel, m, "test-machine"); err != nil {
			t.Fatalf("Write(%q): %v", rel, err)
		}
	}
	idx := run(t, remote, "show", "main:MEMORY.md")
	if n := strings.Count(idx, "](projects/foo.md)"); n != 1 {
		t.Errorf("want one canonical index line, got %d:\n%s", n, idx)
	}
	if strings.Contains(idx, "](./") || strings.Contains(idx, "//") {
		t.Errorf("index carries a caller's spelling:\n%s", idx)
	}
	projects := idx[strings.Index(idx, "## projects"):]
	if !strings.Contains(projects, "](projects/foo.md)") {
		t.Errorf("projects/foo.md was not filed under ## projects:\n%s", idx)
	}

	if _, err := s.Delete("projects/foo.md", "test-machine"); err != nil {
		t.Fatal(err)
	}
	if idx := run(t, remote, "show", "main:MEMORY.md"); strings.Contains(idx, "foo.md") {
		t.Errorf("a line for the deleted memory survived:\n%s", idx)
	}
}

// The store root has natural spellings. They mean "everything", not "escape".
func TestListTreatsTheRootAsNoPrefix(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	all, err := s.List("")
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{".", "./", "infra/..", "./."} {
		got, err := s.List(prefix)
		if err != nil {
			t.Fatalf("List(%q): %v", prefix, err)
		}
		if len(got) != len(all) {
			t.Errorf("List(%q) returned %d entries, want all %d", prefix, len(got), len(all))
		}
	}
}

// Name and description are flattened onto one line before anything is
// written, so the file, the index and the commit message agree, and a value
// that is only whitespace is empty and refused as such.
func TestWriteNormalisesNameAndDescriptionOnceForEveryRecord(t *testing.T) {
	remote := newTestRemote(t)
	s := newTestStore(t, remote)
	for _, m := range []Memory{
		{Name: "   ", Description: "d", Scope: "global"},
		{Name: "n", Description: "\n", Scope: "global"},
	} {
		if _, err := s.Write("infra/blank.md", m, "test-machine"); err == nil {
			t.Errorf("%+v was accepted with a blank name or description", m)
		}
	}
	if _, err := s.Write("infra/ups.md", Memory{Name: "ups\nbattery", Description: "line one\nline two", Type: "reference", Scope: "global"}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	if subject := strings.TrimSpace(run(t, remote, "log", "-1", "--format=%s", "main")); subject != "memory: ups battery" {
		t.Errorf("commit subject %q, want the flattened name", subject)
	}
	if body := run(t, remote, "log", "-1", "--format=%b", "main"); !strings.Contains(body, "line one line two") {
		t.Errorf("commit body does not carry the flattened description:\n%s", body)
	}
}

// Scope is enforced on read, so a scope that does not parse is a memory that
// is quietly global or quietly invisible. Type has a four-word vocabulary.
// Both are checked at the one place a memory is written.
func TestWriteRefusesAMalformedScopeOrType(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	for _, m := range []Memory{
		{Name: "n", Description: "d", Type: "reference", Scope: ""},
		{Name: "n", Description: "d", Type: "reference", Scope: "delta"},
		{Name: "n", Description: "d", Type: "reference", Scope: "machine/"},
		{Name: "n", Description: "d", Type: "reference", Scope: "project/"},
		{Name: "n", Description: "d", Type: "reference", Scope: "machine:delta"},
		{Name: "n", Description: "d", Type: "reference", Scope: "global/x"},
		{Name: "n", Description: "d", Type: "reference", Scope: "machine/ delta"},
		{Name: "n", Description: "d", Type: "reference", Scope: "project/example repo"},
		{Name: "n", Description: "d", Type: "reference", Scope: "machine/to\nwer"},
		{Name: "n", Description: "d", Type: "reference", Scope: "machine/to\u00a0wer"},
		{Name: "n", Description: "d", Type: "", Scope: "global"},
		{Name: "n", Description: "d", Type: "index", Scope: "global"},
		{Name: "n", Description: "d", Type: "note", Scope: "global"},
	} {
		if _, err := s.Write("infra/bad.md", m, "test-machine"); err == nil {
			t.Errorf("scope %q type %q was accepted", m.Scope, m.Type)
		}
	}
	for _, m := range []Memory{
		{Name: "n", Description: "d", Type: "user", Scope: "global"},
		{Name: "n", Description: "d", Type: "feedback", Scope: "project/example--repo"},
		{Name: "n", Description: "d", Type: "project", Scope: "machine/delta"},
		{Name: "n", Description: "d", Type: "reference", Scope: "Machine/Delta"},
	} {
		if _, err := s.Write("infra/good.md", m, "test-machine"); err != nil {
			t.Errorf("scope %q type %q was refused: %v", m.Scope, m.Type, err)
		}
	}
}

func TestReadRefusesGitInternals(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if got, err := s.Read(".git/config"); err == nil {
		t.Errorf("Read(.git/config) returned %q", got)
	}
}

// visible() compares scopes lowercased, so what is written might as well be
// what is compared. A scope that validates but is stored in a form visible()
// cannot match is the silent invisibility the check exists to refuse.
func TestWriteStoresTheNormalisedScopeAndType(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("infra/t.md", Memory{Name: "n", Description: "d", Type: " Reference ", Scope: " Machine/Delta "}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	fm := parseFrontmatter(mustRead(t, filepath.Join(s.Dir, "infra", "t.md")))
	if fm["scope"] != "machine/delta" || fm["type"] != "reference" {
		t.Errorf("stored scope %q type %q, want machine/delta and reference", fm["scope"], fm["type"])
	}
}

func mustRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The limit counts what the caller will actually receive. Spending it on hits
// in memories the caller cannot see returns nothing while matches exist.
func TestSearchDoesNotSpendTheLimitOnHiddenMemories(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	hidden := "needle one\nneedle two\nneedle three\nneedle four\nneedle five"
	if _, err := s.Write("infra/a.md", Memory{Name: "a", Description: "d", Type: "reference", Scope: "machine/delta", Body: hidden}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write("personal/z.md", Memory{Name: "z", Description: "d", Type: "reference", Scope: "global", Body: "the needle"}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	keep := func(scope string) bool { return visible(scope, "beta", false) }
	hits, _, err := s.Search("needle", 3, SearchFilter{Keep: keep})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Path != "personal/z.md" {
		t.Errorf("Search returned %+v, want the one visible hit", hits)
	}
}

// `updated` means "when this memory last changed", not "when it was last
// written". A rewrite of identical content must keep the original stamp and
// make no commit, even when it lands in a later second. Without that, a
// full-resolution timestamp defeats the no-change path and every rewrite
// produces an empty commit.
func TestRewritingIdenticalContentKeepsTheOriginalStamp(t *testing.T) {
	remote := newTestRemote(t)
	s := newTestStore(t, remote)
	defer fixClock(t, "2026-09-08T00:00:00Z")()

	m := Memory{Name: "ups", Description: "the UPS", Type: "reference", Scope: "global", Body: "48 minutes."}
	if _, err := s.Write("infra/ups.md", m, "test-machine"); err != nil {
		t.Fatal(err)
	}
	first := parseFrontmatter(mustRead(t, filepath.Join(s.Dir, "infra", "ups.md")))["updated"]
	before := strings.TrimSpace(run(t, remote, "rev-parse", "main"))

	setClock(t, "2026-09-08T00:00:31Z") // a later second, deterministically

	commit, err := s.Write("infra/ups.md", m, "test-machine")
	if err != nil {
		t.Fatal(err)
	}
	if commit != "no change" {
		t.Errorf("second identical write reported %q, want \"no change\"", commit)
	}
	if again := parseFrontmatter(mustRead(t, filepath.Join(s.Dir, "infra", "ups.md")))["updated"]; again != first {
		t.Errorf("stamp moved from %q to %q on an unchanged rewrite", first, again)
	}
	if after := strings.TrimSpace(run(t, remote, "rev-parse", "main")); after != before {
		t.Errorf("an unchanged rewrite moved the branch from %s to %s", before, after)
	}
}

// A memory stamped before the format changed carries a bare date. Leaving it
// that way forever would give the store two `updated` formats at once, which
// is worse for the model that reads them than the single ambiguous format it
// replaced. A rewrite must therefore roll an unparseable stamp forward, even
// when nothing else about the memory changed.
func TestARewriteRollsAnOldBareDateStampForward(t *testing.T) {
	remote := newTestRemote(t)
	s := newTestStore(t, remote)
	defer fixClock(t, "2026-09-08T00:00:00Z")()

	// Seed the remote with a memory in the old format, as the store holds today.
	seeded := "---\nname: old\ndescription: d\ntype: reference\nscope: global\nupdated: 2026-09-05\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(s.Dir, "infra", "old.md"), []byte(seeded), 0o640); err != nil {
		t.Fatal(err)
	}
	run(t, s.Dir, "add", "-A")
	run(t, s.Dir, "commit", "-q", "-m", "a memory in the old format")
	run(t, s.Dir, "push", "-q", "origin", "main")

	m := Memory{Name: "old", Description: "d", Type: "reference", Scope: "global", Body: "body"}
	commit, err := s.Write("infra/old.md", m, "test-machine")
	if err != nil {
		t.Fatal(err)
	}
	if commit == "no change" {
		t.Error("an old bare-date stamp was left in place; the store keeps two formats")
	}
	got := parseFrontmatter(mustRead(t, filepath.Join(s.Dir, "infra", "old.md")))["updated"]
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Errorf("updated = %q, which did not roll forward to RFC3339", got)
	}
}

// fixClock pins composeMemory's clock for a test and returns the restore.
func fixClock(t *testing.T, stamp string) func() {
	t.Helper()
	prev := now
	setClock(t, stamp)
	return func() { now = prev }
}

func setClock(t *testing.T, stamp string) {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		t.Fatal(err)
	}
	now = func() time.Time { return ts }
}

// The index is derived state. It must exist as soon as the store does, follow
// a write without waiting for the fifteen-minute timer, and be rebuildable
// from the markdown alone — there is no persisted index to go stale.
func TestTheIndexFollowsTheMarkdown(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if s.index == nil || len(s.index.docs) == 0 {
		t.Fatal("Ensure left the store with no index")
	}
	before := len(s.index.docs)

	if _, err := s.Write("infra/ups.md", Memory{
		Name: "ups", Description: "the UPS", Type: "reference", Scope: "global", Body: "48 minutes.",
	}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	if len(s.index.docs) != before+1 {
		t.Errorf("index holds %d documents after a write, want %d", len(s.index.docs), before+1)
	}

	if _, err := s.Delete("infra/ups.md", "test-machine"); err != nil {
		t.Fatal(err)
	}
	if len(s.index.docs) != before {
		t.Errorf("index holds %d documents after a delete, want %d", len(s.index.docs), before)
	}

	// MEMORY.md holds every description, so indexing it returns the index
	// beside the memory it points at for almost any query.
	for _, d := range s.index.docs {
		if strings.EqualFold(d.path, indexFile) || strings.EqualFold(d.path, conventionsFile) {
			t.Errorf("%s is indexed; it is not a retrievable memory", d.path)
		}
	}
}

func TestVocabularyReportsWhatExistsWithoutTheContent(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	v := s.Vocabulary()
	if v.Memories != 2 {
		t.Errorf("Memories = %d, want 2", v.Memories)
	}
	// Exact comparisons, not containment: the fixture is small enough to spell
	// out in full, and only an exact match catches both an unsorted list and
	// an extra entry that containment would miss.
	if want := []string{"global", "machine/delta"}; !reflect.DeepEqual(v.Scopes, want) {
		t.Errorf("Scopes = %v, want %v", v.Scopes, want)
	}
	if want := []string{"feedback", "reference"}; !reflect.DeepEqual(v.Types, want) {
		t.Errorf("Types = %v, want %v", v.Types, want)
	}
	if want := []string{"infra", "personal"}; !reflect.DeepEqual(v.Prefixes, want) {
		t.Errorf("Prefixes = %v, want %v", v.Prefixes, want)
	}
}
