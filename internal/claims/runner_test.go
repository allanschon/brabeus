package claims

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/store"
)

// These tests run against a real git repository, as the store's and the
// server's do. Their helpers cannot be imported across packages, so the small
// subset needed here is copied.

func gitRun(t *testing.T, dir string, args ...string) string {
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

// newRemote builds a bare repository standing in for the record, seeded with
// the index and conventions files only.
func newRemote(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	bare := filepath.Join(base, "record.git")
	gitRun(t, base, "init", "--bare", "-b", "main", bare)
	seed := filepath.Join(base, "seed")
	gitRun(t, base, "clone", "-q", bare, seed)
	for rel, content := range map[string]string{
		"MEMORY.md":      "---\ntype: index\n---\n\n# Memory index\n\n## global\n",
		"CONVENTIONS.md": "# House style\n\nPlain speech.\n",
	} {
		if err := os.WriteFile(filepath.Join(seed, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitRun(t, seed, "add", "-A")
	gitRun(t, seed, "commit", "-q", "-m", "seed")
	gitRun(t, seed, "push", "-q", "origin", "main")
	return bare
}

func newClaimsStore(t *testing.T) *store.Store {
	t.Helper()
	set, err := module.Load(filepath.Join("..", "..", "modules"), []string{"memory", "identity", "telos"})
	if err != nil {
		t.Fatal(err)
	}
	s := &store.Store{
		Dir:         filepath.Join(t.TempDir(), "work"),
		RemoteURL:   newRemote(t),
		Branch:      "main",
		CommitName:  "brabeus",
		CommitEmail: "brabeus@example.com",
	}
	if err := s.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	s.SetModules(set)
	s.ValidateClaim = Validate
	return s
}

// writeGoal writes a telos goal with its four required fields and claims.
func writeGoal(t *testing.T, s *store.Store, rel, claims string) {
	t.Helper()
	if _, err := s.Write(rel, store.Record{Name: filepath.Base(strings.TrimSuffix(rel, ".md")), Description: "a goal", Module: "telos", Kind: "goal", Scope: "global",
		Fields: map[string]string{"id": "G3", "title": "Ship the guide", "ideal": "published", "by": "2026-12-01", "claims": claims}, Body: "b"}, "test-machine"); err != nil {
		t.Fatal(err)
	}
}

type fake struct {
	name   string
	out    Outcome
	err    error
	during func() // runs inside Check, outside the store's lock
	calls  int
}

func (f *fake) Name() string { return f.name }
func (f *fake) Check(context.Context, map[string]string, time.Time) (Outcome, error) {
	f.calls++
	if f.during != nil {
		f.during()
	}
	return f.out, f.err
}

const g3Claims = "- text: \"three articles\"\n  check: {adapter: tracker, label: article, since: 2026-07-01, min: 3}\n- text: \"date holds\"\n  check: {adapter: manual}\n- text: \"a commit\"\n  check: {adapter: forge, repo: a/b, since: -14d, min: 1}"

func TestRunRecordsEveryNonManualClaimAndLeavesManualOnesAlone(t *testing.T) {
	st := newClaimsStore(t)
	writeGoal(t, st, "telos/goal/g3.md", g3Claims)
	goalBytes, err := os.ReadFile(filepath.Join(st.Dir, "telos", "goal", "g3.md"))
	if err != nil {
		t.Fatal(err)
	}
	goalLog := gitRun(t, st.Dir, "log", "--format=%H", "--", "telos/goal/g3.md")
	tracker := &fake{name: "tracker", out: Outcome{State: store.Fail, Detail: "2 found"}}
	now := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
	r := &Runner{Store: st, Adapters: map[string]Adapter{"tracker": tracker}, Now: func() time.Time { return now }, StatePath: filepath.Join(t.TempDir(), "last-run")}
	rep, err := r.Run(context.Background())
	if err != nil || rep.Goals != 1 || rep.Claims != 3 || rep.Changed != 2 || rep.NoEvidence != 1 {
		t.Fatalf("report %+v err %v", rep, err)
	}
	all, _ := st.ClaimResults()
	got := all["telos/goal/g3.md"]
	if len(got) != 3 || got[0].State != store.Fail || got[1].State != store.Unchecked || got[2].State != store.NoEvidence || !strings.Contains(got[2].Detail, "not configured") {
		t.Fatalf("results = %+v", got)
	}
	if !got[0].Since.Equal(now) {
		t.Errorf("since = %v", got[0].Since)
	}
	if last, ok := r.LastRun(); !ok || !last.Equal(now) {
		t.Errorf("last run = %v %v", last, ok)
	}
	// A second run with the same answer changes no state, keeps since, and commits nothing.
	head := gitRun(t, st.Dir, "rev-parse", "HEAD")
	later := now.Add(24 * time.Hour)
	r.Now = func() time.Time { return later }
	tracker.out.Detail = "1 found" // the count moved within the same state
	if rep, _ := r.Run(context.Background()); rep.Changed != 0 {
		t.Errorf("changed = %d", rep.Changed)
	}
	if gitRun(t, st.Dir, "rev-parse", "HEAD") != head {
		t.Error("an unchanged run committed")
	}
	all, _ = st.ClaimResults()
	if got := all["telos/goal/g3.md"][0]; !got.Since.Equal(now) || got.Detail != "2 found" {
		t.Errorf("since or detail moved although the state did not: %+v", got)
	}
	// The state flips: since moves to the run that saw it.
	tracker.out = Outcome{State: store.Pass, Detail: "3 found"}
	if rep, _ := r.Run(context.Background()); rep.Changed != 1 {
		t.Errorf("changed = %d", rep.Changed)
	}
	all, _ = st.ClaimResults()
	if got := all["telos/goal/g3.md"][0]; got.State != store.Pass || !got.Since.Equal(later) {
		t.Errorf("after the flip: %+v", got)
	}
	// None of it touched the goal: not its content, its stamps or its history.
	if b, _ := os.ReadFile(filepath.Join(st.Dir, "telos", "goal", "g3.md")); !bytes.Equal(b, goalBytes) {
		t.Errorf("the goal changed:\n%s", b)
	}
	if l := gitRun(t, st.Dir, "log", "--format=%H", "--", "telos/goal/g3.md"); l != goalLog {
		t.Errorf("the goal's history moved:\n%s\nwas\n%s", l, goalLog)
	}
	// A restart reads the stamp back rather than reporting never.
	fresh := &Runner{Store: st, StatePath: r.StatePath}
	if last, ok := fresh.LastRun(); !ok || !last.Equal(later) {
		t.Errorf("after restart: %v %v", last, ok)
	}
}

// An adapter that errors is a deployment fault: no-evidence with the reason,
// never fail (§8.1), and the run goes on to the next claim.
func TestAnErroringAdapterIsNoEvidenceNotFail(t *testing.T) {
	st := newClaimsStore(t)
	writeGoal(t, st, "telos/goal/g.md", "- text: \"x\"\n  check: {adapter: tracker, label: a, since: -7d, min: 1}\n- text: \"y\"\n  check: {adapter: date, before: 2027-01-01}")
	r := &Runner{Store: st, Adapters: map[string]Adapter{"tracker": &fake{name: "tracker", err: fmt.Errorf("HTTP 502 from tracker.example")}, "date": Date{}}, Now: time.Now, StatePath: filepath.Join(t.TempDir(), "last-run")}
	rep, err := r.Run(context.Background())
	if err != nil || rep.NoEvidence != 1 {
		t.Fatalf("report %+v err %v", rep, err)
	}
	all, _ := st.ClaimResults()
	got := all["telos/goal/g.md"]
	if len(got) != 2 || got[0].State != store.NoEvidence || !strings.Contains(got[0].Detail, "502") || got[1].State != store.Pass {
		t.Errorf("%+v", got)
	}
}

// The same through a real adapter: a server that answers 500 or cannot be
// reached is no-evidence, never a skipped claim and never fail.
func TestATrackerThatAnswers500OrIsDownIsNoEvidence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close()
	for name, url := range map[string]string{"500": srv.URL, "unreachable": down.URL} {
		st := newClaimsStore(t)
		writeGoal(t, st, "telos/goal/g.md", "- text: \"x\"\n  check: {adapter: tracker, label: a, since: -7d, min: 1}")
		adapters, err := New(Config{Tracker: "vikunja", TrackerURL: url, TrackerToken: "synthetic"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		r := &Runner{Store: st, Adapters: adapters, Now: time.Now, StatePath: filepath.Join(t.TempDir(), "last-run")}
		if _, err := r.Run(context.Background()); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		all, _ := st.ClaimResults()
		if got := all["telos/goal/g.md"]; len(got) != 1 || got[0].State != store.NoEvidence || got[0].Detail == "" {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

// An adapter that answers a state it has no business answering is recorded
// as no-evidence rather than refused by the store, which would stop the run.
func TestAnAdapterAnsweringAStateItMayNotIsNoEvidence(t *testing.T) {
	st := newClaimsStore(t)
	writeGoal(t, st, "telos/goal/g.md", "- text: \"x\"\n  check: {adapter: tracker, label: a, since: -7d, min: 1}")
	r := &Runner{Store: st, Adapters: map[string]Adapter{"tracker": &fake{name: "tracker", out: Outcome{State: "maybe"}}}, Now: time.Now}
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	all, _ := st.ClaimResults()
	if got := all["telos/goal/g.md"]; len(got) != 1 || got[0].State != store.NoEvidence || !strings.Contains(got[0].Detail, "maybe") {
		t.Errorf("%+v", got)
	}
}

// A manual answer recorded while a run is gathering evidence survives the
// run: the run merges into what is recorded when it writes, not into what it
// read when it started.
func TestAManualAnswerRecordedDuringARunSurvivesIt(t *testing.T) {
	st := newClaimsStore(t)
	writeGoal(t, st, "telos/goal/g3.md", g3Claims)
	tracker := &fake{name: "tracker", out: Outcome{State: store.Pass, Detail: "3 found"}}
	tracker.during = func() {
		if _, err := st.UpdateClaimResults("telos/goal/g3.md", func(prev []store.ClaimResult) []store.ClaimResult {
			return append(prev, store.ClaimResult{Index: 1, Text: "date holds", Adapter: "manual", State: store.Pass, Detail: "still on"})
		}, "desk"); err != nil {
			t.Error(err)
		}
	}
	r := &Runner{Store: st, Adapters: map[string]Adapter{"tracker": tracker}, Now: time.Now}
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	all, _ := st.ClaimResults()
	got := all["telos/goal/g3.md"]
	if len(got) != 3 || got[0].State != store.Pass || got[1].State != store.Pass || got[1].Detail != "still on" {
		t.Errorf("%+v", got)
	}
}

// One goal's results file failing to parse must not stop the goals after it:
// their claims still get checked and recorded, the broken goal is counted
// and logged rather than aborting the run, and the run is still stamped so
// /healthz reads the one broken goal, not a schedule that stopped.
func TestOneGoalsBrokenResultsFileDoesNotStopTheRun(t *testing.T) {
	st := newClaimsStore(t)
	writeGoal(t, st, "telos/goal/g1.md", "- text: \"x\"\n  check: {adapter: tracker, label: a, since: -7d, min: 1}")
	writeGoal(t, st, "telos/goal/g2.md", "- text: \"y\"\n  check: {adapter: tracker, label: b, since: -7d, min: 1}")
	// The broken file must survive the runner's own sync (a fetch, a hard
	// reset and a clean of anything untracked), so it is committed and
	// pushed like any other result, not just written to the worktree.
	broken := filepath.Join(st.Dir, "claims", "telos", "goal", "g1.json")
	if err := os.MkdirAll(filepath.Dir(broken), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(broken, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, st.Dir, "add", "-A")
	gitRun(t, st.Dir, "commit", "-q", "-m", "a hand-edited results file that no longer parses")
	gitRun(t, st.Dir, "push", "-q", "origin", "main")
	tracker := &fake{name: "tracker", out: Outcome{State: store.Pass, Detail: "1 found"}}
	statePath := filepath.Join(t.TempDir(), "last-run")
	r := &Runner{Store: st, Adapters: map[string]Adapter{"tracker": tracker}, Now: time.Now, StatePath: statePath}
	rep, err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected a joined error naming the broken goal")
	}
	if rep.Goals != 2 || rep.Errors != 1 || rep.Changed != 1 {
		t.Fatalf("report %+v", rep)
	}
	all, _ := st.ClaimResults()
	if got := all["telos/goal/g2.md"]; len(got) != 1 || got[0].State != store.Pass {
		t.Errorf("the goal after the broken one was not recorded: %+v", got)
	}
	if _, ok := r.LastRun(); !ok {
		t.Error("a run with one broken goal must still stamp the last run")
	}
	if _, err := os.ReadFile(statePath); err != nil {
		t.Errorf("the stamp file was not written: %v", err)
	}
}

// A run cut short by shutdown records nothing for the goal it was on and
// does not stamp a last run: "context canceled" is not evidence of anything.
func TestACancelledRunRecordsNothingAndIsNotALastRun(t *testing.T) {
	st := newClaimsStore(t)
	writeGoal(t, st, "telos/goal/g.md", "- text: \"x\"\n  check: {adapter: tracker, label: a, since: -7d, min: 1}")
	ctx, cancel := context.WithCancel(context.Background())
	tracker := &fake{name: "tracker", during: cancel, err: context.Canceled}
	r := &Runner{Store: st, Adapters: map[string]Adapter{"tracker": tracker}, Interval: time.Hour, Now: time.Now, StatePath: filepath.Join(t.TempDir(), "last-run")}
	head := gitRun(t, st.Dir, "rev-parse", "HEAD")
	if _, err := r.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if gitRun(t, st.Dir, "rev-parse", "HEAD") != head {
		t.Error("a cancelled run committed")
	}
	if s := Status(r); s != "never" {
		t.Errorf("status = %q", s)
	}
}

func TestStatusReadsOffNeverOrTheStamp(t *testing.T) {
	if s := Status(nil); s != "off" {
		t.Errorf("nil: %q", s)
	}
	if s := Status(&Runner{Interval: 0, StatePath: filepath.Join(t.TempDir(), "x")}); s != "off" {
		t.Errorf("off: %q", s)
	}
	r := &Runner{Interval: time.Hour, StatePath: filepath.Join(t.TempDir(), "x")}
	if s := Status(r); s != "never" {
		t.Errorf("never: %q", s)
	}
	stamped := filepath.Join(t.TempDir(), "claims-last-run")
	if err := os.WriteFile(stamped, []byte("2026-09-20T04:00:00Z\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if s := Status(&Runner{Interval: time.Hour, StatePath: stamped}); s != "2026-09-20T04:00:00Z" {
		t.Errorf("stamp: %q", s)
	}
}

// A stamp that cannot be written is logged: a restart would otherwise say
// never, and the reason must be findable.
func TestAFailedStampWriteIsLogged(t *testing.T) {
	st := newClaimsStore(t)
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	r := &Runner{Store: st, Now: time.Now, StatePath: filepath.Join(t.TempDir(), "missing", "last-run")}
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "last-run") {
		t.Errorf("log = %q", buf.String())
	}
	if _, ok := r.LastRun(); !ok {
		t.Error("the run happened even though its stamp was not saved")
	}
}

// Start returns at once when the schedule is off, and stops when its context
// ends; it shares the store with the sync ticker without racing it.
func TestStartRunsNowStopsOnCancelAndIsOffAtZero(t *testing.T) {
	done := make(chan struct{})
	go func() { (&Runner{}).Start(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start with no interval did not return")
	}

	st := newClaimsStore(t)
	writeGoal(t, st, "telos/goal/g.md", "- text: \"x\"\n  check: {adapter: date, before: 2027-01-01}")
	r := &Runner{Store: st, Adapters: map[string]Adapter{"date": Date{}}, Interval: 10 * time.Millisecond, Now: time.Now, StatePath: filepath.Join(t.TempDir(), "last-run")}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ctx.Err() == nil {
			_ = st.Sync()
			_ = Status(r)
		}
	}()
	done = make(chan struct{})
	go func() { r.Start(ctx); close(done) }()
	deadline := time.After(10 * time.Second)
	for {
		if _, ok := r.LastRun(); ok {
			break
		}
		select {
		case <-deadline:
			t.Fatal("Start never ran")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Start did not stop on cancel")
	}
	wg.Wait()
}
