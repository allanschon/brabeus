package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/allanschon/brabeus/internal/block"
	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/store"
)

func TestPickRepoDefaultsToMemory(t *testing.T) {
	mem, proj := &store.Store{}, &store.Store{}
	for _, name := range []string{"", "memory", "MEMORY"} {
		got, err := pickRepo(name, mem, proj, false)
		if err != nil {
			t.Fatalf("pickRepo(%q): %v", name, err)
		}
		if got != mem {
			t.Errorf("pickRepo(%q) did not return the memory store", name)
		}
	}
}

func TestPickRepoReturnsTheProjectsMirror(t *testing.T) {
	mem, proj := &store.Store{}, &store.Store{}
	got, err := pickRepo("projects", mem, proj, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != proj {
		t.Error("pickRepo(projects) did not return the projects mirror")
	}
}

// The mirror is optional, and its absence must be an explicit error rather than
// a silent fallback to the memory store — which would let a caller read the
// wrong repository and never know.
func TestPickRepoSaysSoWhenTheMirrorIsAbsent(t *testing.T) {
	if _, err := pickRepo("projects", &store.Store{}, nil, false); err == nil {
		t.Error("an absent projects mirror was not reported")
	}
}

func TestPickRepoRejectsAnUnknownName(t *testing.T) {
	if _, err := pickRepo("secrets", &store.Store{}, &store.Store{}, false); err == nil {
		t.Error("an unknown repo name was accepted")
	}
}

// The mirror is a second corpus with no modules of its own (spec §11):
// a consumer is refused it outright, not filtered within it.
func TestPickRepoRefusesTheMirrorToAConsumer(t *testing.T) {
	mem, proj := &store.Store{}, &store.Store{}
	if _, err := pickRepo("projects", mem, proj, true); err == nil || !strings.Contains(err.Error(), "not readable by this caller") {
		t.Errorf("a consumer was allowed the projects mirror, or the wording drifted: %v", err)
	}
	if got, err := pickRepo("projects", mem, proj, false); err != nil || got != proj {
		t.Errorf("a self caller must still get the mirror: got=%v err=%v", got, err)
	}
	if got, err := pickRepo("memory", mem, proj, true); err != nil || got != mem {
		t.Errorf("a consumer must still get memory: got=%v err=%v", got, err)
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot) // distinctive: proves we reached through
	})
}

func TestAuthMiddlewareNoneLetsEverythingThrough(t *testing.T) {
	h, err := AuthMiddleware(okHandler(), "none", "")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/mcp", nil))
	if w.Code != http.StatusTeapot {
		t.Errorf("status %d, want the handler to be reached", w.Code)
	}
}

func TestAuthMiddlewareBearerRejectsAWrongOrMissingToken(t *testing.T) {
	h, err := AuthMiddleware(okHandler(), "bearer", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"", "Bearer wrong", "s3cret", "Basic s3cret"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/mcp", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		h.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("Authorization %q got status %d, want 401", header, w.Code)
		}
	}
}

func TestAuthMiddlewareBearerAcceptsTheRightToken(t *testing.T) {
	h, err := AuthMiddleware(okHandler(), "bearer", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Header.Set("Authorization", "Bearer s3cret")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusTeapot {
		t.Errorf("status %d, want the handler to be reached", w.Code)
	}
}

// Bearer mode with no token would accept nothing while looking configured, and
// an unknown mode must not quietly behave like "none".
func TestAuthMiddlewareRefusesUnusableConfiguration(t *testing.T) {
	if _, err := AuthMiddleware(okHandler(), "bearer", ""); err == nil {
		t.Error("bearer mode with an empty token was accepted")
	}
	if _, err := AuthMiddleware(okHandler(), "opportunistic", "x"); err == nil {
		t.Error("an unknown auth mode was accepted")
	}
}

func TestFilterEntriesHidesOtherMachines(t *testing.T) {
	in := []store.Entry{
		{Path: "personal/a.md", Scope: "global"},
		{Path: "infra/b.md", Scope: "machine/gamma"},
		{Path: "infra/c.md", Scope: "machine/beta"},
	}
	got := filterEntries(in, "beta", false, store.Visibility{})
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %v", len(got), got)
	}
	for _, e := range got {
		if e.Path == "infra/b.md" {
			t.Error("another machine's memory was returned")
		}
	}
	if len(filterEntries(in, "beta", true, store.Visibility{})) != 3 {
		t.Error("include_all_scopes did not return everything")
	}
}

// Filtering must never return nil where an empty list is meant: a nil slice
// marshals as null, and a client reading null as "unknown" rather than "none"
// is a difference that only shows up in production.
func TestFilterEntriesReturnsAnEmptySliceNotNil(t *testing.T) {
	got := filterEntries([]store.Entry{{Path: "x.md", Scope: "machine/elsewhere"}}, "beta", false, store.Visibility{})
	if got == nil {
		t.Fatal("filterEntries returned nil")
	}
	if len(got) != 0 {
		t.Errorf("got %d entries, want 0", len(got))
	}
}

// A limit above the ceiling is clamped to it, not reset to the default: a
// caller who asked for 1000 wants many, not 50.
func TestSearchLimitClampsRatherThanResets(t *testing.T) {
	for in, want := range map[int]int{0: 50, -1: 50, 1: 1, 50: 50, 500: 500, 501: 500, 1000: 500} {
		if got := searchLimit(in); got != want {
			t.Errorf("searchLimit(%d) = %d, want %d", in, got, want)
		}
	}
}

// The deprecated alias (decision D6) must be visible as deprecated in its own
// schema, not merely in a comment nobody outside this repository reads.
func TestWriteInCarriesModuleKindAndFieldsAndTheDeprecatedAlias(t *testing.T) {
	for _, f := range []string{"Module", "Kind", "Fields", "Type"} {
		if _, ok := reflect.TypeOf(writeIn{}).FieldByName(f); !ok {
			t.Errorf("writeIn lacks %s", f)
		}
	}
	tag, _ := reflect.TypeOf(writeIn{}).FieldByName("Type")
	if !strings.Contains(strings.ToLower(tag.Tag.Get("jsonschema")), "deprecated") {
		t.Error("the type alias must say it is deprecated in its schema")
	}
}

func testSet(t *testing.T) *module.Set {
	t.Helper()
	set, err := module.Load(filepath.Join("..", "..", "modules"), []string{"memory", "identity", "telos", "health"})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestAudienceForAConsumerHidesSelfModulesAndUntagged(t *testing.T) {
	set := testSet(t)
	self := audienceFor(set, false)
	if len(self.HideModules) != 0 || self.HideUntagged {
		t.Errorf("the person's own session sees everything: %+v", self)
	}
	cons := audienceFor(set, true)
	for _, m := range []string{"memory", "identity", "telos", "health"} { // all four ship as audience self
		if !cons.Hides(m) {
			t.Errorf("consumer must not see %s", m)
		}
	}
	if !cons.HideUntagged {
		t.Error("consumer must not see untagged files")
	}

	// Hide-set keys are lower-cased at insertion: a manifest whose directory
	// and name are both capitalised must still be hidden under its
	// lower-cased key, since Module values on disk are not guaranteed to
	// match a manifest's exact casing.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Health"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"Health","version":1,"profile":"ratified-record","priority":1,"budget_bytes":100,
	  "kinds":{"note":{"fields":[]}},"summary":"summary.md.tmpl"}`
	if err := os.WriteFile(filepath.Join(dir, "Health", "module.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	capSet, err := module.Load(dir, []string{"Health"})
	if err != nil {
		t.Skipf("module.Load rejects a capitalised manifest name/directory, so this case cannot be exercised: %v", err)
	}
	if capCons := audienceFor(capSet, true); !capCons.Hides("health") {
		t.Error("a capitalised module name must still be hidden under its lower-cased key")
	}
}

func TestHiddenForFoldsProfileIntoTheAudience(t *testing.T) {
	set := testSet(t)
	for _, tc := range []struct {
		profile, module string
		wantHidden      []string
		wantSeen        []string
		wantErr         bool
	}{
		{"", "", []string{"identity", "telos", "health"}, []string{"memory"}, false},
		{"working-memory", "", []string{"identity", "telos", "health"}, []string{"memory"}, false},
		{"ratified-record", "", []string{"memory"}, []string{"identity", "telos", "health"}, false},
		{"all", "", nil, []string{"memory", "identity", "telos", "health"}, false},
		{"", "telos", []string{"identity", "health"}, []string{"memory", "telos"}, false}, // naming a module un-hides it
		{"shared", "", nil, nil, true},
	} {
		v, err := hiddenFor(set, false, tc.profile, tc.module)
		if (err != nil) != tc.wantErr {
			t.Errorf("%+v: err=%v", tc, err)
			continue
		}
		for _, m := range tc.wantHidden {
			if !v.Hides(m) {
				t.Errorf("profile %q module %q: %s should be hidden", tc.profile, tc.module, m)
			}
		}
		for _, m := range tc.wantSeen {
			if v.Hides(m) {
				t.Errorf("profile %q module %q: %s should be seen", tc.profile, tc.module, m)
			}
		}
	}
	// Audience is never overridden by profile or module.
	v, _ := hiddenFor(set, true, "all", "health")
	if !v.Hides("health") {
		t.Error("a consumer naming a self module still does not see it")
	}
}

func TestFilterEntriesAppliesScopeAndVisibility(t *testing.T) {
	in := []store.Entry{
		{Path: "a.md", Module: "memory", Scope: "global"},
		{Path: "b.md", Module: "health", Scope: "global"},
		{Path: "c.md", Module: "", Scope: "global"},
		{Path: "d.md", Module: "memory", Scope: "machine/other"},
	}
	got := filterEntries(in, "desk", false, store.Visibility{HideModules: map[string]bool{"health": true}, HideUntagged: true})
	if len(got) != 1 || got[0].Path != "a.md" {
		t.Errorf("got %+v", got)
	}
}

func TestParseConsumersAndCaller(t *testing.T) {
	c := ParseConsumers(" dashboard , Exchange-Box ,")
	if !c["dashboard"] || !c["exchange-box"] || len(c) != 2 {
		t.Errorf("consumers = %v", c)
	}
	id := fakeIdentity{name: "Exchange-Box"}
	caller, consumer := Caller(id, c, nil)
	if caller != "exchange-box" || !consumer {
		t.Errorf("caller=%q consumer=%v", caller, consumer)
	}
	if caller, consumer := Caller(fakeIdentity{name: "desk"}, c, nil); caller != "desk" || consumer {
		t.Errorf("own machine: caller=%q consumer=%v", caller, consumer)
	}
	if _, consumer := Caller(fakeIdentity{}, c, nil); !consumer {
		t.Error("an unresolved caller is a consumer")
	}
}

type fakeIdentity struct{ name string }

func (f fakeIdentity) Machine(*http.Request) string { return f.name }
func (f fakeIdentity) Mode() string                 { return "fake" }
func (f fakeIdentity) Describe() string             { return "fake" }

// These tests run against a real git repository rather than a mock, mirroring
// internal/store/store_test.go's own helpers — which cannot be imported
// across packages, so the small subset this package needs is copied here.

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

const seedIndex = `---
type: index
---

# Memory index

## global

## infra

## projects
`

// newRemote builds a bare repository standing in for the record, seeded with
// just the index and conventions files — no pre-module record files, which
// would sort ahead of every module in the agenda (priority -1) and break the
// stale-record-first assertion below.
func newRemote(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	bare := filepath.Join(base, "record.git")
	gitRun(t, base, "init", "--bare", "-b", "main", bare)

	seed := filepath.Join(base, "seed")
	gitRun(t, base, "clone", "-q", bare, seed)
	write := func(rel, content string) {
		full := filepath.Join(seed, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("MEMORY.md", seedIndex)
	write("CONVENTIONS.md", "# House style\n\nPlain speech. Short sentences. No frontmatter here; this file is not a memory.\n")
	gitRun(t, seed, "add", "-A")
	gitRun(t, seed, "commit", "-q", "-m", "seed")
	gitRun(t, seed, "push", "-q", "origin", "main")
	return bare
}

func newServerStore(t *testing.T) *store.Store {
	t.Helper()
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
	s.SetModules(testSet(t))
	return s
}

func TestGate(t *testing.T) {
	st := newServerStore(t)
	if _, err := st.Write("identity/value/family.md", store.Record{Name: "family", Description: "family first", Module: "identity", Kind: "value", Scope: "global",
		Fields: map[string]string{"statement": "family first"}, Body: "family first"}, "desk"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write("infra/laptop.md", store.Record{Name: "laptop", Description: "the other machine", Module: "memory", Kind: "note", Scope: "machine/other", Body: "laptop notes"}, "desk"); err != nil {
		t.Fatal(err)
	}

	if _, err := gate(st, "nowhere.md", "desk", false, store.Visibility{}, readScopeRefusal); err == nil || !strings.Contains(err.Error(), `no record at "nowhere.md"`) {
		t.Errorf("missing file: %v", err)
	}
	if _, err := gate(st, "infra/laptop.md", "desk", false, store.Visibility{}, readScopeRefusal); err == nil || !strings.Contains(err.Error(), "pass include_all_scopes to read it anyway") {
		t.Errorf("read scope refusal: %v", err)
	}
	if _, err := gate(st, "infra/laptop.md", "desk", false, store.Visibility{}, reviewScopeRefusal); err == nil ||
		strings.Contains(err.Error(), "include_all_scopes") || !strings.Contains(err.Error(), "review it from that machine") {
		t.Errorf("review scope refusal must not mention include_all_scopes: %v", err)
	}
	if _, err := gate(st, "identity/value/family.md", "desk", false, store.Visibility{HideModules: map[string]bool{"identity": true}}, readScopeRefusal); err == nil ||
		!strings.Contains(err.Error(), "is not readable by this caller") {
		t.Errorf("hidden module: %v", err)
	}
	// Audience before scope: a hidden module AND a machine-scoped record this
	// caller cannot see must still report the audience wording, and the error
	// must never leak the other machine's name through the scope wording.
	if _, err := gate(st, "infra/laptop.md", "desk", false, store.Visibility{HideModules: map[string]bool{"memory": true}}, readScopeRefusal); err == nil ||
		!strings.Contains(err.Error(), "is not readable by this caller") || strings.Contains(err.Error(), "other") {
		t.Errorf("audience must be checked before scope: %v", err)
	}
	r, err := gate(st, "identity/value/family.md", "desk", false, store.Visibility{}, readScopeRefusal)
	if err != nil || r.Name != "family" {
		t.Errorf("pass case: r=%+v err=%v", r, err)
	}
}

func TestRenderContextEndToEnd(t *testing.T) {
	st := newServerStore(t)
	set := testSet(t)
	renderer, err := block.New(set)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write("identity/value/family.md", store.Record{Name: "family", Description: "family first", Module: "identity", Kind: "value", Scope: "global",
		Fields: map[string]string{"statement": "family first"}, Body: "family first"}, "desk"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Review("identity/value/family.md", "Still one of the things you weigh decisions against?", store.Answer{Verdict: store.Confirmed}, "desk"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write("infra/laptop.md", store.Record{Name: "laptop", Description: "the other machine", Module: "memory", Kind: "note", Scope: "machine/other", Body: "laptop notes"}, "desk"); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(400 * 24 * time.Hour)
	d := Deps{Memory: st, Set: set, Block: renderer, Now: func() time.Time { return later }}

	text, faults, top, err := RenderContext(d, "desk", false)
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := strings.Cut(text, "\n")
	if !strings.HasPrefix(first, "agenda: [identity/value family]") || top == nil || top.Path != "identity/value/family.md" {
		t.Errorf("stale record must be the first line: %q top=%+v", first, top)
	}
	if len(faults) != 0 || len(text) > block.Cap || !strings.Contains(text, "- value: family first") {
		t.Errorf("faults=%v len=%d\n%s", faults, len(text), text)
	}
	if strings.Contains(text, "laptop") {
		t.Errorf("another machine's record leaked into the block:\n%s", text)
	}

	text, _, top, err = RenderContext(d, "desk", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(text, "agenda: nothing due\n") || top != nil || strings.Contains(text, "identity:") || strings.Contains(text, "family") {
		t.Errorf("a consumer sees no self module and is asked nothing:\n%s", text)
	}
}

// New's tool registration is otherwise never exercised: no test calls it, and
// the container smoke dies at memory.Ensure before a request would reach
// mcp.NewStreamableHTTPHandler's callback. contextIn is this codebase's first
// empty tool-input struct, and contextOut's []block.Fault and *agenda.Item
// (an untagged struct with a named string field) are its first schema
// inference over those shapes — either could panic mcp.AddTool at construction,
// which would happen inside the per-request server factory and take down every
// session while /healthz stayed green.
func TestNewRegistersTheToolsForBothCallerClasses(t *testing.T) {
	d := Deps{Set: testSet(t)}
	for _, consumer := range []bool{false, true} {
		if s := New(d, "desk", consumer); s == nil {
			t.Fatalf("consumer=%v: nil server", consumer)
		}
	}
}
