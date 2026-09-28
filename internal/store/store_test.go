package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/scope"
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
	s.SetModules(testModules(t))
	return s
}

// testModules is the shipped memory manifest plus a small ratified module,
// loaded from a temp dir so the tests do not depend on ../../modules.
func testModules(t *testing.T) *module.Set {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, "module.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("memory", `{"name":"memory","version":1,"profile":"working-memory","priority":20,"layout":"free",
	  "scope_keys":["machine","project"],
	  "legacy_types":{"user":"note","feedback":"preference","project":"project","reference":"note"},
	  "kinds":{"note":{"fields":[]},"trap":{"fields":[]},"preference":{"fields":[]},"project":{"fields":[]}}}`)
	write("telos", `{"name":"telos","version":1,"profile":"ratified-record","priority":10,"budget_bytes":600,
	  "kinds":{"goal":{"fields":["id","title","ideal","by"],"optional":["serves"],"freshness_days":90}},
	  "summary":"summary.md.tmpl"}`)
	write("identity", `{"name":"identity","version":1,"profile":"ratified-record","priority":5,"budget_bytes":400,
	  "kinds":{"value":{"fields":["statement"],"freshness_days":365,"interview":"Still one of the things you weigh decisions against?","first":"What do you weigh decisions against?"},
	           "preference":{"fields":["statement"],"freshness_days":120}},
	  "onboarding":["value"],"summary":"summary.md.tmpl"}`)
	set, err := module.Load(dir, []string{"memory", "telos", "identity"})
	if err != nil {
		t.Fatal(err)
	}
	return set
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
	// infra/delta.md predates modules: it carries type, not module/kind, so
	// List reports those two empty rather than refusing the pre-module file.
	if got.Scope != "machine/delta" || got.Module != "" || got.Kind != "" || got.Description != "the NAS box" {
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
		if _, err := s.Write(p, Record{
			Name: "n", Description: "the UPS battery", Module: "memory", Kind: "note",
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
	if _, err := s.Write("infra/ups.md", Record{
		Name: "ups", Description: "the UPS battery", Module: "memory", Kind: "note", Scope: "global",
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

func TestSearchFiltersByScopeAndKind(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	write := func(path, kind, scope string) {
		if _, err := s.Write(path, Record{
			Name: "n", Description: "the UPS battery", Module: "memory", Kind: kind, Scope: scope, Body: "battery",
		}, "test-machine"); err != nil {
			t.Fatal(err)
		}
	}
	write("infra/a.md", "note", "global")
	write("infra/b.md", "preference", "global")
	write("infra/c.md", "note", "machine/delta")
	for _, tc := range []struct {
		f    SearchFilter
		want string
	}{
		{SearchFilter{Module: "memory", Kind: "preference"}, "infra/b.md"},
		{SearchFilter{Scope: "machine/delta"}, "infra/c.md"},
	} {
		hits, _, err := s.Search("battery", 10, tc.f)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 1 || hits[0].Path != tc.want {
			t.Errorf("filter %+v: got %v, want only %s", tc.f, hits, tc.want)
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
		if _, err := s.Write(path, Record{
			Name: "n", Description: "the UPS battery", Module: "memory", Kind: "note", Scope: "global", Body: "battery",
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

	if _, err := s.Write("infra/ups.md", Record{
		Name: "ups", Description: "the UPS", Module: "memory", Kind: "note", Scope: "global", Body: "48 minutes.",
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
	m := Record{Name: "ups", Description: "the UPS", Module: "memory", Kind: "note", Scope: "global", Body: "48 minutes."}

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
	if _, err := s.Write("infra/ups.txt", Record{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "global"}, "test-machine"); err == nil {
		t.Error("a non-markdown path was accepted")
	}
}

func TestWriteRefusesToEscapeTheStore(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("../escape.md", Record{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "global"}, "test-machine"); err == nil {
		t.Error("a path outside the store was accepted")
	}
}

func TestAReadOnlyStoreRefusesToWrite(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	s.ReadOnly = true
	if _, err := s.Write("infra/ups.md", Record{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "global"}, "test-machine"); err == nil {
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

	if _, err := s.Write("infra/ups.md", Record{
		Name: "ups", Description: "the UPS", Module: "memory", Kind: "note", Scope: "global", Body: "48 minutes.",
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

	if _, err := s.Write("infra/ups.md", Record{
		Name: "ups", Description: "the UPS", Module: "memory", Kind: "note", Scope: "global", Body: "48 minutes.",
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

	if _, err := s.Write("infra/ups.md", Record{
		Name: "ups", Description: "the UPS", Module: "memory", Kind: "note", Scope: "global", Body: "48 minutes."}, "test-machine"); err != nil {
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
	m := Record{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "global", Body: "x"}
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
	if _, err := s.Write("personal/x.md", Record{
		Name: "x", Description: "d", Module: "memory", Kind: "note", Scope: "global", Body: "b",
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
	if _, err := s.Write("personal/y.md", Record{
		Name: "y", Description: "d", Module: "memory", Kind: "note", Scope: "global", Body: "b",
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
// The delete commit's subject has the review commit's shape, so the history
// can be read back as an event log with one shape per kind of change.
func TestDeleteCommitNamesTheModuleKindAndRecord(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("infra/ups.md", Record{
		Name: "ups", Description: "the UPS", Module: "memory", Kind: "note", Scope: "global", Body: "48 minutes."}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Delete("infra/ups.md", "test-machine"); err != nil {
		t.Fatal(err)
	}
	out, _ := s.git(s.Dir, "log", "-1", "--format=%s%n%b")
	if !strings.HasPrefix(out, "delete memory/note ups\n") {
		t.Errorf("subject = %q, want it to start %q", out, "delete memory/note ups")
	}
	if !strings.Contains(out, "Deleted through the kernel at ") || strings.Contains(out, "MCP server") {
		t.Errorf("body = %q, want the kernel named and not the old server name", out)
	}
}

func TestDeleteStampsCallerAsAuthor(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("personal/z.md", Record{
		Name: "z", Description: "d", Module: "memory", Kind: "note", Scope: "global", Body: "b",
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
		if got, err := MemoryPath(rel); err == nil {
			t.Errorf("%q was accepted as %q", rel, got)
		}
	}
	for rel, want := range map[string]string{
		"notes/MEMORY.md":  "notes/MEMORY.md",
		"foo.md":           "foo.md",
		"MEMORY.md.bak.md": "MEMORY.md.bak.md",
		"./infra//ups.md":  "infra/ups.md",
	} {
		got, err := MemoryPath(rel)
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
	if _, err := s.Write("infrastructure/x.md", Record{Name: "x", Description: "d", Module: "memory", Kind: "note", Scope: "global"}, "test-machine"); err != nil {
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
	m := Record{Name: "foo", Description: "d", Module: "memory", Kind: "note", Scope: "global", Body: "x"}
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
	for _, m := range []Record{
		{Name: "   ", Description: "d", Scope: "global"},
		{Name: "n", Description: "\n", Scope: "global"},
	} {
		if _, err := s.Write("infra/blank.md", m, "test-machine"); err == nil {
			t.Errorf("%+v was accepted with a blank name or description", m)
		}
	}
	if _, err := s.Write("infra/ups.md", Record{Name: "ups\nbattery", Description: "line one\nline two", Module: "memory", Kind: "note", Scope: "global"}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	if subject := strings.TrimSpace(run(t, remote, "log", "-1", "--format=%s", "main")); subject != "memory/note: ups battery" {
		t.Errorf("commit subject %q, want the flattened name", subject)
	}
	if body := run(t, remote, "log", "-1", "--format=%b", "main"); !strings.Contains(body, "line one line two") {
		t.Errorf("commit body does not carry the flattened description:\n%s", body)
	}
}

// Scope is enforced on read, so a scope that does not parse is a memory that
// is quietly global or quietly invisible. Checked at the one place a memory
// is written. Module and kind validity have their own test
// (TestWriteRequiresAModuleAndAKnownKind); this one is scope only.
func TestWriteRefusesAMalformedScope(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	for _, m := range []Record{
		{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: ""},
		{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "delta"},
		{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "machine/"},
		{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "project/"},
		{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "machine:delta"},
		{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "global/x"},
		{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "machine/ delta"},
		{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "project/example repo"},
		{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "machine/to\nwer"},
		{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "machine/to\u00a0wer"},
	} {
		if _, err := s.Write("infra/bad.md", m, "test-machine"); err == nil {
			t.Errorf("scope %q was accepted", m.Scope)
		}
	}
	for _, m := range []Record{
		{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "global"},
		{Name: "n", Description: "d", Module: "memory", Kind: "preference", Scope: "project/example--repo"},
		{Name: "n", Description: "d", Module: "memory", Kind: "project", Scope: "machine/delta"},
		{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "Machine/Delta"},
	} {
		if _, err := s.Write("infra/good.md", m, "test-machine"); err != nil {
			t.Errorf("scope %q was refused: %v", m.Scope, err)
		}
	}
}

func TestReadRefusesGitInternals(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if got, err := s.Read(".git/config"); err == nil {
		t.Errorf("Read(.git/config) returned %q", got)
	}
}

// scope.Visible() compares scopes lowercased, so what is written might as well be
// what is compared. A scope that validates but is stored in a form scope.Visible()
// cannot match is the silent invisibility the check exists to refuse.
func TestWriteStoresTheNormalisedScopeAndModuleKind(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("infra/t.md", Record{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: " Machine/Delta "}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	fm := parseFrontmatter(mustRead(t, filepath.Join(s.Dir, "infra", "t.md")))
	if fm["scope"] != "machine/delta" || fm["module"] != "memory" || fm["kind"] != "note" {
		t.Errorf("stored scope %q module %q kind %q, want machine/delta, memory, note", fm["scope"], fm["module"], fm["kind"])
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
	if _, err := s.Write("infra/a.md", Record{Name: "a", Description: "d", Module: "memory", Kind: "note", Scope: "machine/delta", Body: hidden}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write("personal/z.md", Record{Name: "z", Description: "d", Module: "memory", Kind: "note", Scope: "global", Body: "the needle"}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	keep := func(sc string) bool { return scope.Visible(sc, "beta", false) }
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

	m := Record{Name: "ups", Description: "the UPS", Module: "memory", Kind: "note", Scope: "global", Body: "48 minutes."}
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

	m := Record{Name: "old", Description: "d", Module: "memory", Kind: "note", Scope: "global", Body: "body"}
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
	prev := now
	now = func() time.Time { return ts }
	t.Cleanup(func() { now = prev })
}

// The index is derived state. It must exist as soon as the store does, follow
// a write without waiting for the fifteen-minute timer, and be rebuildable
// from the markdown alone — there is no persisted index to go stale.
func TestTheIndexFollowsTheMarkdown(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if s.index == nil || len(s.index.Docs) == 0 {
		t.Fatal("Ensure left the store with no index")
	}
	before := len(s.index.Docs)

	if _, err := s.Write("infra/ups.md", Record{
		Name: "ups", Description: "the UPS", Module: "memory", Kind: "note", Scope: "global", Body: "48 minutes.",
	}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	if len(s.index.Docs) != before+1 {
		t.Errorf("index holds %d documents after a write, want %d", len(s.index.Docs), before+1)
	}

	if _, err := s.Delete("infra/ups.md", "test-machine"); err != nil {
		t.Fatal(err)
	}
	if len(s.index.Docs) != before {
		t.Errorf("index holds %d documents after a delete, want %d", len(s.index.Docs), before)
	}

	// MEMORY.md holds every description, so indexing it returns the index
	// beside the memory it points at for almost any query.
	for _, d := range s.index.Docs {
		if strings.EqualFold(d.Path, indexFile) || strings.EqualFold(d.Path, conventionsFile) {
			t.Errorf("%s is indexed; it is not a retrievable memory", d.Path)
		}
	}
}

func TestVocabularyReportsWhatExistsWithoutTheContent(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	v := s.Vocabulary(Visibility{})
	if v.Memories != 2 {
		t.Errorf("Memories = %d, want 2", v.Memories)
	}
	// Exact comparisons, not containment: the fixture is small enough to spell
	// out in full, and only an exact match catches both an unsorted list and
	// an extra entry that containment would miss.
	if want := []string{"global", "machine/delta"}; !reflect.DeepEqual(v.Scopes, want) {
		t.Errorf("Scopes = %v, want %v", v.Scopes, want)
	}
	// Both fixture files predate modules: neither carries a module or kind,
	// so there is nothing here for Modules or Kinds to report.
	if len(v.Modules) != 0 || len(v.Kinds) != 0 {
		t.Errorf("Modules = %v, Kinds = %v, want both empty for a pre-module corpus", v.Modules, v.Kinds)
	}
	if want := []string{"infra", "personal"}; !reflect.DeepEqual(v.Prefixes, want) {
		t.Errorf("Prefixes = %v, want %v", v.Prefixes, want)
	}
}

func TestWriteRequiresAModuleAndAKnownKind(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	for _, r := range []Record{
		{Name: "n", Description: "d", Scope: "global", Body: "b"},                                   // no module
		{Name: "n", Description: "d", Module: "health", Kind: "metric", Scope: "global", Body: "b"}, // not enabled
		{Name: "n", Description: "d", Module: "memory", Kind: "fact", Scope: "global", Body: "b"},   // no such kind
	} {
		if _, err := s.Write("infra/x.md", r, "test-machine"); err == nil {
			t.Errorf("accepted %+v", r)
		}
	}
}

func TestTheTypeAliasMapsThroughLegacyTypes(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("personal/p.md", Record{Name: "p", Description: "d", Type: "feedback", Scope: "global", Body: "b"}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	r, _ := ParseRecord(mustRead(t, filepath.Join(s.Dir, "personal/p.md")))
	if r.Module != "memory" || r.Kind != "preference" {
		t.Errorf("alias: module=%q kind=%q", r.Module, r.Kind)
	}
	if _, err := s.Write("personal/q.md", Record{Name: "q", Description: "d", Type: "nonsense", Scope: "global", Body: "b"}, "test-machine"); err == nil {
		t.Error("an unmapped type must be refused, not guessed")
	}
}

func TestARatifiedWriteIsValidatedAgainstTheKind(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	good := Record{Name: "g1", Description: "ship it", Module: "telos", Kind: "goal", Scope: "global",
		Fields: map[string]string{"id": "G1", "title": "Ship", "ideal": "shipped", "by": "2026-12-01"}, Body: "b"}
	if _, err := s.Write("telos/goal/g1.md", good, "test-machine"); err != nil {
		t.Fatalf("a complete goal must write: %v", err)
	}
	r, _ := ParseRecord(mustRead(t, filepath.Join(s.Dir, "telos/goal/g1.md")))
	if r.ID != "G1" || r.Fields["title"] != "Ship" {
		t.Errorf("stored: %+v", r)
	}
	missing := good
	missing.Fields = map[string]string{"id": "G2", "title": "Ship"}
	if _, err := s.Write("telos/goal/g2.md", missing, "test-machine"); err == nil || !strings.Contains(err.Error(), "ideal") {
		t.Errorf("a missing required field must be named: %v", err)
	}
	unknown := good
	unknown.Fields = map[string]string{"id": "G3", "title": "Ship", "ideal": "x", "by": "y", "colour": "red"}
	if _, err := s.Write("telos/goal/g3.md", unknown, "test-machine"); err == nil || !strings.Contains(err.Error(), "colour") {
		t.Errorf("an undeclared field must be named: %v", err)
	}
}

func TestAWriteMayNotCarryAKernelOwnedKey(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	r := Record{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "global", Body: "b",
		Fields: map[string]string{"reviewed": "2026-09-01T00:00:00Z"}}
	if _, err := s.Write("infra/x.md", r, "test-machine"); err == nil || !strings.Contains(err.Error(), "reviewed") {
		t.Errorf("reviewed via a write must be refused: %v", err)
	}
}

func TestThePathRuleFollowsTheProfile(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	goal := Record{Name: "g", Description: "d", Module: "telos", Kind: "goal", Scope: "global",
		Fields: map[string]string{"id": "G1", "title": "t", "ideal": "i", "by": "b"}, Body: "b"}
	if _, err := s.Write("infra/g.md", goal, "test-machine"); err == nil {
		t.Error("a ratified record outside <module>/<kind>/ must be refused")
	}
	if _, err := s.Write("telos/goal/deeper/g.md", goal, "test-machine"); err == nil {
		t.Error("a ratified record below <module>/<kind>/ must be refused")
	}
	note := Record{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "global", Body: "b"}
	if _, err := s.Write("anywhere/at/all.md", note, "test-machine"); err != nil {
		t.Errorf("layout: free allows the store's own tree: %v", err)
	}
}

func TestARewriteKeepsReviewedWhereItWas(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	rel := "personal/p.md"
	r := Record{Name: "p", Description: "d", Module: "memory", Kind: "preference", Scope: "global", Body: "first"}
	if _, err := s.Write(rel, r, "test-machine"); err != nil {
		t.Fatal(err)
	}
	// Simulate a review having happened: write reviewed into the file directly,
	// commit, push — the way Task 5's Review will. Then rewrite through Write.
	full := filepath.Join(s.Dir, rel)
	content := strings.Replace(mustRead(t, full), "---\n\nfirst", "reviewed: 2026-09-01T00:00:00Z\n---\n\nfirst", 1)
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, s.Dir, "commit", "-qam", "reviewed")
	run(t, s.Dir, "push", "-q", "origin", "main")
	r.Body = "second"
	if _, err := s.Write(rel, r, "test-machine"); err != nil {
		t.Fatal(err)
	}
	_, meta := ParseRecord(mustRead(t, full))
	if meta.Reviewed.IsZero() {
		t.Error("a plain write must not drop reviewed")
	}
}

func TestACredentialShapedWriteIsRefusedBeforeGit(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	before := run(t, s.Dir, "rev-parse", "HEAD")
	r := Record{Name: "n", Description: "d", Module: "memory", Kind: "note", Scope: "global",
		Body: "-----BEGIN " + "OPENSSH PRIVATE KEY-----\nabc\n-----END " + "OPENSSH PRIVATE KEY-----\n"} // built, not literal: see credential_test.go
	if _, err := s.Write("infra/x.md", r, "test-machine"); err == nil || !strings.Contains(err.Error(), "private key") {
		t.Errorf("want a refusal naming the shape, got %v", err)
	}
	if after := run(t, s.Dir, "rev-parse", "HEAD"); after != before {
		t.Error("the refusal must happen before anything is committed")
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "infra/x.md")); err == nil {
		t.Error("nothing may be left on disk")
	}
}

// The description reaches the frontmatter, MEMORY.md and the commit message,
// same as the body — a credential-shaped description must be refused before
// any of them are touched, same as the body is.
func TestACredentialShapedDescriptionIsRefusedBeforeGit(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	before := run(t, s.Dir, "rev-parse", "HEAD")
	r := Record{Name: "n", Description: "password: " + "Tr0ub4dor&3" + strings.Repeat("A", 9),
		Module: "memory", Kind: "note", Scope: "global", Body: "b"} // built, not literal: see credential_test.go
	if _, err := s.Write("infra/x.md", r, "test-machine"); err == nil || !strings.Contains(err.Error(), "description") {
		t.Errorf("want a refusal naming the description, got %v", err)
	}
	if after := run(t, s.Dir, "rev-parse", "HEAD"); after != before {
		t.Error("the refusal must happen before anything is committed")
	}
}

// The deleted scope.CheckType lowercased and trimmed. The replacement lookup
// must do the same, or an outbox file queued before this PR with
// "type: Reference" — or any caller passing "  Memory  " / "NOTE" — is
// permanently rejected, breaking the constraint that nothing queued is
// rejected by this PR.
func TestTheTypeAliasAndModuleKindAreNormalisedBeforeLookup(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("personal/r.md", Record{Name: "r", Description: "d", Type: "Reference", Scope: "global", Body: "b"}, "test-machine"); err != nil {
		t.Fatalf("a differently-cased type must still map: %v", err)
	}
	r, _ := ParseRecord(mustRead(t, filepath.Join(s.Dir, "personal/r.md")))
	if r.Module != "memory" || r.Kind != "note" {
		t.Errorf("alias with mixed case: module=%q kind=%q", r.Module, r.Kind)
	}
	if _, err := s.Write("personal/s.md", Record{Name: "s", Description: "d", Module: " Memory ", Kind: "NOTE", Scope: "global", Body: "b"}, "test-machine"); err != nil {
		t.Fatalf("a differently-cased module/kind must still validate: %v", err)
	}
	r2, _ := ParseRecord(mustRead(t, filepath.Join(s.Dir, "personal/s.md")))
	if r2.Module != "memory" || r2.Kind != "note" {
		t.Errorf("module/kind with mixed case: module=%q kind=%q", r2.Module, r2.Kind)
	}
}

// A malformed kernel key must stop the write rather than be silently dropped:
// compose only emits reviewed/retired/snoozes when non-zero, so composing
// over a bare-date reviewed would erase it forever, and only review may move
// it (§9).
func TestAMalformedReviewedRefusesARewriteAndLeavesTheFileUnchanged(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	rel := "personal/p.md"
	full := filepath.Join(s.Dir, rel)
	seeded := "---\nname: p\ndescription: d\nmodule: memory\nkind: preference\nscope: global\nupdated: 2026-09-01T00:00:00Z\nreviewed: 2026-09-01\n---\n\nfirst\n"
	if err := os.WriteFile(full, []byte(seeded), 0o640); err != nil {
		t.Fatal(err)
	}
	run(t, s.Dir, "add", "-A")
	run(t, s.Dir, "commit", "-q", "-m", "seed a malformed reviewed")
	run(t, s.Dir, "push", "-q", "origin", "main")

	r := Record{Name: "p", Description: "d", Module: "memory", Kind: "preference", Scope: "global", Body: "second"}
	if _, err := s.Write(rel, r, "test-machine"); err == nil || !strings.Contains(err.Error(), "reviewed") {
		t.Errorf("a malformed reviewed must refuse the write, got %v", err)
	}
	if got := mustRead(t, full); got != seeded {
		t.Error("a refused write must leave the file unchanged")
	}
}

// The common case: a record with no reviewed at all must write without
// tripping the malformed-key refusal above.
func TestAFileWithNoReviewedStillWrites(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	r := Record{Name: "p", Description: "d", Module: "memory", Kind: "preference", Scope: "global", Body: "first"}
	if _, err := s.Write("personal/p.md", r, "test-machine"); err != nil {
		t.Fatalf("a record with no reviewed must still write: %v", err)
	}
	// And a rewrite of a file that still has no reviewed must also still write.
	r.Body = "second"
	if _, err := s.Write("personal/p.md", r, "test-machine"); err != nil {
		t.Fatalf("a rewrite with no reviewed must still write: %v", err)
	}
}

func TestListAndVocabularyReportModuleAndKind(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("infra/x.md", Record{Name: "x", Description: "d", Module: "memory", Kind: "trap", Scope: "global", Body: "b"}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	entries, err := s.List("infra")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range entries {
		if e.Path == "infra/x.md" {
			found = e.Module == "memory" && e.Kind == "trap"
		}
	}
	if !found {
		t.Errorf("entry for infra/x.md lacks module/kind: %+v", entries)
	}
	v := s.Vocabulary(Visibility{})
	if !slices.Contains(v.Modules, "memory") || !slices.Contains(v.Kinds, "memory/trap") {
		t.Errorf("vocabulary = %+v", v)
	}
}

func TestVisibilityHidesModulesAndOptionallyUntagged(t *testing.T) {
	v := Visibility{HideModules: map[string]bool{"health": true}}
	if !v.Hides("health") || v.Hides("memory") || v.Hides("") {
		t.Errorf("hide-set only: health=%v memory=%v untagged=%v", v.Hides("health"), v.Hides("memory"), v.Hides(""))
	}
	v.HideUntagged = true
	if !v.Hides("") {
		t.Error("HideUntagged must hide the empty module")
	}
	if (Visibility{}).Hides("") {
		t.Error("a zero Visibility hides nothing")
	}
	// A hand-committed file can carry any casing in its module: key; the
	// hide-set must not be dodgeable by capitalising it.
	if !v.Hides("Health") {
		t.Error("Hides must compare case-insensitively")
	}
}

func TestSearchAndVocabularyRespectVisibility(t *testing.T) {
	s := newTestStore(t, newTestRemote(t)) // seeds two pre-module files
	if _, err := s.Write("infra/n.md", Record{Name: "n", Description: "battery notes", Module: "memory", Kind: "note", Scope: "global", Body: "battery"}, "m"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write("telos/goal/g1.md", Record{Name: "g1", Description: "battery goal", Module: "telos", Kind: "goal", Scope: "global",
		Fields: map[string]string{"id": "G1", "title": "battery", "ideal": "i", "by": "b"}, Body: "battery"}, "m"); err != nil {
		t.Fatal(err)
	}
	all, _, err := s.Search("battery", 10, SearchFilter{})
	if err != nil {
		t.Fatal(err)
	}
	hidden, _, err := s.Search("battery", 10, SearchFilter{Visibility: Visibility{HideModules: map[string]bool{"telos": true}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || len(hidden) != 1 || hidden[0].Path != "infra/n.md" {
		t.Errorf("all=%v hidden=%v", paths(all), paths(hidden))
	}
	// Pre-module files: visible unless HideUntagged.
	seen, _, _ := s.Search("plain speech", 10, SearchFilter{Visibility: Visibility{HideModules: map[string]bool{"telos": true}}})
	gone, _, _ := s.Search("plain speech", 10, SearchFilter{Visibility: Visibility{HideUntagged: true}})
	if len(seen) == 0 || len(gone) != 0 {
		t.Errorf("untagged: seen=%v gone=%v", paths(seen), paths(gone))
	}
	v := s.Vocabulary(Visibility{HideModules: map[string]bool{"telos": true}})
	if slices.Contains(v.Modules, "telos") || slices.Contains(v.Kinds, "telos/goal") {
		t.Errorf("vocabulary leaks a hidden module: %+v", v)
	}
	full := s.Vocabulary(Visibility{})
	if full.Memories != v.Memories+1 {
		t.Errorf("hidden records must not be counted: full=%d hidden=%d", full.Memories, v.Memories)
	}
}

func TestPeekReturnsTheFrontmatterOrFalse(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	r, meta, ok := s.Peek("personal/style.md")
	if !ok || r.Scope != "global" || meta.LegacyType != "feedback" {
		t.Errorf("peek legacy: %+v %+v %v", r, meta, ok)
	}
	if _, _, ok := s.Peek("nope/none.md"); ok {
		t.Error("peek of a missing file must be false")
	}
	if _, _, ok := s.Peek("../escape.md"); ok {
		t.Error("peek must refuse an escape")
	}
}
