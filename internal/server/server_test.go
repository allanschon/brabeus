package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/allanschon/brabeus/internal/agenda"
	"github.com/allanschon/brabeus/internal/block"
	"github.com/allanschon/brabeus/internal/instructions"
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

// notebook is the new spelling of projects (M2): a deployment configured
// before the rename, and one configured after it, must resolve to the same
// store and the same refusal.
func TestPickRepoAcceptsNotebookAsTheNewName(t *testing.T) {
	mem, proj := &store.Store{}, &store.Store{}
	got, err := pickRepo("notebook", mem, proj, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != proj {
		t.Error("pickRepo(notebook) did not return the notebook store")
	}
	if _, err := pickRepo("notebook", mem, proj, true); err == nil || !strings.Contains(err.Error(), "not readable by this caller") {
		t.Errorf("a consumer asking for notebook was allowed it, or the wording drifted: %v", err)
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

// The deprecated `type` alias must be visible as deprecated in its own
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

// The person's answer is a first-class part of the review tool's input, not
// an afterthought folded into another field.
func TestReviewInCarriesTheAnswer(t *testing.T) {
	if _, ok := reflect.TypeOf(reviewIn{}).FieldByName("Answer"); !ok {
		t.Error("reviewIn lacks Answer")
	}
}

// writeIn.Scope must carry omitempty, or the tool's inferred input schema
// marks it required and the SDK refuses a scopeless call before the
// handler — and the store's Write, which is the one place that ever
// happens — is reached at all. So the default-to-global rule (spec §5, AP)
// is unreachable over MCP without this, even though store.Write implements
// it correctly. Round-trips through a real client so the schema the SDK
// actually generates and validates against is exercised, not a stand-in.
func TestWriteToolOmittingScopeLandsGlobal(t *testing.T) {
	st := newServerStore(t)
	srv := New(Deps{Memory: st, Set: testSet(t)}, "desk", false)

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "write",
		Arguments: map[string]any{
			"path": "infra/noscope.md", "name": "n", "description": "d",
			"module": "memory", "kind": "note", "body": "b",
			// scope deliberately omitted
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("a write omitting scope was refused by the tool's own input schema: %+v", res.Content)
	}

	content, err := st.Read("infra/noscope.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "scope: global") {
		t.Errorf("record did not land scoped global:\n%s", content)
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
	caller, consumer, ok := Caller(id, c, nil)
	if caller != "exchange-box" || !consumer || !ok {
		t.Errorf("caller=%q consumer=%v ok=%v", caller, consumer, ok)
	}
	if caller, consumer, ok := Caller(fakeIdentity{name: "desk"}, c, nil); caller != "desk" || consumer || !ok {
		t.Errorf("own machine: caller=%q consumer=%v ok=%v", caller, consumer, ok)
	}
	if _, _, ok := Caller(fakeIdentity{}, c, nil); ok {
		t.Error("an unresolved caller is not ok")
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
	if _, err := st.Review("identity/value/family.md", store.ReviewInput{Question: "Still one of the things you weigh decisions against?", Verdict: store.Confirmed, Answer: "yes"}, "desk"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write("infra/laptop.md", store.Record{Name: "laptop", Description: "the other machine", Module: "memory", Kind: "note", Scope: "machine/other", Body: "laptop notes"}, "desk"); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(400 * 24 * time.Hour)
	d := Deps{Memory: st, Set: set, Block: renderer, Now: func() time.Time { return later }}

	text, faults, top, err := RenderContext(d, "desk", "", false)
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

	text, _, top, err = RenderContext(d, "desk", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(text, "agenda: nothing due\n") || top != nil || strings.Contains(text, "identity:") || strings.Contains(text, "family") {
		t.Errorf("a consumer sees no self module and is asked nothing:\n%s", text)
	}
}

// A record scoped to a project renders only inside that project. Before M2 it
// rendered in every project, because the block had no notion of "the caller's
// own project" to compare against. Claims and reflect keep scope.Visible,
// because they answer about the whole record.
func TestAProjectScopedRecordRendersOnlyInItsProject(t *testing.T) {
	st := newServerStore(t)
	set := testSet(t)
	renderer, err := block.New(set)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write("identity/value/family.md", store.Record{Name: "family", Description: "family first", Module: "identity", Kind: "value", Scope: "project/example--repo",
		Fields: map[string]string{"statement": "family first"}, Body: "family first"}, "desk"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Review("identity/value/family.md", store.ReviewInput{Question: "Still one of the things you weigh decisions against?", Verdict: store.Confirmed, Answer: "yes"}, "desk"); err != nil {
		t.Fatal(err)
	}
	d := Deps{Memory: st, Set: set, Block: renderer, Now: time.Now}

	text, _, _, err := RenderContext(d, "desk", "example--repo", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "family first") {
		t.Errorf("a project-scoped record must render inside its own project:\n%s", text)
	}

	text, _, _, err = RenderContext(d, "desk", "other--repo", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "family first") {
		t.Errorf("a project-scoped record leaked into a different project:\n%s", text)
	}

	text, _, _, err = RenderContext(d, "desk", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "family first") {
		t.Errorf("a project-scoped record leaked outside any project:\n%s", text)
	}
}

// GET /context refuses a malformed project rather than letting it reach the
// filter: the value arrives from the network, so the boundary check runs
// before scope.VisibleIn ever sees it.
func TestContextHandlerRefusesAMalformedProject(t *testing.T) {
	st := newServerStore(t)
	set := testSet(t)
	renderer, err := block.New(set)
	if err != nil {
		t.Fatal(err)
	}
	d := Deps{Memory: st, Set: set, Block: renderer, Now: time.Now}
	h := ContextHandler(d, fakeIdentity{name: "desk"}, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/context?project=not/a/slug", nil))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "§5") {
		t.Errorf("status=%d body=%q", w.Code, w.Body.String())
	}
}

// A project key computed from a repository whose owner or name itself
// contains "--" (legal on both GitHub and Gitea — the hook's own joining
// separator is not reserved) must round-trip: CheckScope has always accepted
// it on write, so GET /context must not refuse it on read.
func TestAProjectKeyWithADoubleDashRoundTrips(t *testing.T) {
	st := newServerStore(t)
	set := testSet(t)
	renderer, err := block.New(set)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write("identity/value/family.md", store.Record{Name: "family", Description: "family first", Module: "identity", Kind: "value", Scope: "project/acme--my--tool",
		Fields: map[string]string{"statement": "family first"}, Body: "family first"}, "desk"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Review("identity/value/family.md", store.ReviewInput{Question: "Still one of the things you weigh decisions against?", Verdict: store.Confirmed, Answer: "yes"}, "desk"); err != nil {
		t.Fatal(err)
	}
	d := Deps{Memory: st, Set: set, Block: renderer, Now: time.Now}
	h := ContextHandler(d, fakeIdentity{name: "desk"}, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/context?project=acme--my--tool", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "family first") {
		t.Errorf("a key the hook computes from a repository named \"my--tool\" must render: status=%d body=%q", w.Code, w.Body.String())
	}
}

// AQ: an unidentified caller is refused before any tool runs. Fail closed: an
// own session whose lookup fails gets a clear error, not a quietly reduced view.
func TestAnUnidentifiedCallerIsRefusedBeforeAnyTool(t *testing.T) {
	if _, _, ok := Caller(fakeIdentity{}, nil, nil); ok {
		t.Error("an empty machine name must not be ok")
	}
	h := RefuseUnidentified(okHandler(), fakeIdentity{}, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/mcp", nil))
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "not identified") {
		t.Errorf("status %d body %q", w.Code, w.Body.String())
	}
	h = RefuseUnidentified(okHandler(), fakeIdentity{name: "desk"}, nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/mcp", nil))
	if w.Code != http.StatusTeapot {
		t.Errorf("a named caller must reach the handler: %d", w.Code)
	}
}

// AC, §11: the modules tool lists to a caller only the modules it may read,
// with their kinds, lenses, drafts and intros — the interview's scaffolding.
func TestTheModulesToolWithholdsWhatTheCallerMayNotRead(t *testing.T) {
	set := testSet(t)
	own := modulesFor(set, audienceFor(set, false))
	if len(own.Modules) != 4 {
		t.Errorf("own session sees every enabled module: %+v", own)
	}
	var identity *moduleOut
	for i := range own.Modules {
		if own.Modules[i].Name == "identity" {
			identity = &own.Modules[i]
		}
	}
	if identity == nil || identity.Intro == "" || len(identity.Kinds["register"].Lenses) < 2 || identity.Onboarding[0] != "register" {
		t.Errorf("identity as served: %+v", identity)
	}
	cons := modulesFor(set, audienceFor(set, true))
	if len(cons.Modules) != 0 {
		t.Errorf("every shipped module is self; a consumer sees none, not even a name: %+v", cons)
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

// "date holds" is standing: an end-state claim with its deadline ahead reads
// open and never reaches the agenda (§8.1), and a test below needs it to fail.
const serverGoalClaims = "- text: \"three articles\"\n  check: {adapter: tracker, label: article, since: 2026-07-01, min: 3}\n- text: \"date holds\"\n  standing: true\n  check: {adapter: manual}\n- text: \"a commit\"\n  standing: true\n  check: {adapter: forge, repo: a/b, since: -14d, min: 1}"

func writeServerGoal(t *testing.T, st *store.Store, rel, id, sc string) {
	t.Helper()
	if _, err := st.Write(rel, store.Record{Name: strings.TrimSuffix(filepath.Base(rel), ".md"), Description: "a goal", Module: "telos", Kind: "goal", Scope: sc,
		Fields: map[string]string{"id": id, "title": "Ship the guide", "ideal": "published", "by": "2026-12-01", "claims": serverGoalClaims}, Body: "b"}, "desk"); err != nil {
		t.Fatal(err)
	}
}

// §8.1: the claims tool shows each claim with its state, unchecked where
// nothing is recorded, and a pass older than two intervals as stale — an
// adapter's pass only, since the schedule never runs a manual claim. It
// answers about the person's whole record, not one project's view.
func TestTheClaimsToolJoinsClaimsToResultsAndMarksAStalePass(t *testing.T) {
	st := newServerStore(t)
	set := testSet(t)
	writeServerGoal(t, st, "telos/goal/g3.md", "G3", "project/other-project")
	writeServerGoal(t, st, "telos/goal/away.md", "G9", "machine/other")
	t0 := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
	if _, err := st.RecordClaimResults("telos/goal/g3.md", []store.ClaimResult{
		{Index: 0, Text: "three articles", Adapter: "tracker", State: store.Pass, Detail: "3 found", Since: t0, Recorded: t0},
		{Index: 1, Text: "date holds", Adapter: "manual", State: store.Pass, Since: t0, Recorded: t0},
	}, "kernel"); err != nil {
		t.Fatal(err)
	}
	now := t0.Add(96 * time.Hour)
	last := now.Add(-72 * time.Hour)
	d := Deps{Memory: st, Set: set, Now: func() time.Time { return now },
		LastRun: func() (time.Time, bool) { return last, true }, ClaimInterval: 24 * time.Hour}
	own := audienceFor(set, false)

	out, err := claimsFor(d, "desk", own, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Claims) != 3 || out.LastRun != last.Format(time.RFC3339) || out.Interval != "24h0m0s" {
		t.Fatalf("out = %+v", out)
	}
	c := out.Claims
	if c[0].Goal != "telos/goal/g3.md" || c[0].ID != "G3" || c[0].Title != "Ship the guide" || c[0].Index != 0 || c[0].State != "pass" || c[0].Detail != "3 found" || !c[0].Stale {
		t.Errorf("a tracker pass three days after the last run, on a daily interval, is stale: %+v", c[0])
	}
	if !c[1].Manual || c[1].State != "pass" || c[1].Stale {
		t.Errorf("a manual pass is never stale by the schedule: %+v", c[1])
	}
	if c[2].State != "unchecked" || c[2].Adapter != "forge" || c[2].Since != "" || c[2].Index != 2 {
		t.Errorf("a claim with no result is unchecked: %+v", c[2])
	}

	last = now.Add(-time.Hour)
	if out, _ := claimsFor(d, "desk", own, "telos/goal/g3.md"); len(out.Claims) != 3 || out.Claims[0].Stale {
		t.Errorf("a pass within two intervals of the last run is not stale: %+v", out)
	}
	if _, err := claimsFor(d, "desk", own, "telos/goal/away.md"); err == nil {
		t.Error("a goal scoped to another machine must not be listed")
	}
	if out, _ := claimsFor(d, "desk", audienceFor(set, true), ""); len(out.Claims) != 0 {
		t.Errorf("telos is audience self; a consumer sees none of its claims: %+v", out)
	}

	d.LastRun = nil
	if out, _ := claimsFor(d, "desk", own, ""); out.LastRun != "unknown" || !out.Claims[0].Stale || out.Claims[1].Stale {
		t.Errorf("with no runner wired, nothing vouches for an adapter's pass: %+v", out)
	}
	d.LastRun = func() (time.Time, bool) { return time.Time{}, false }
	if out, _ := claimsFor(d, "desk", own, ""); out.LastRun != "never" || !out.Claims[0].Stale {
		t.Errorf("never run: %+v", out)
	}
	d.LastRun, d.ClaimInterval = func() (time.Time, bool) { return now.Add(-time.Hour), true }, 0
	if out, _ := claimsFor(d, "desk", own, ""); out.LastRun != "off" || out.Interval != "off" || out.Claims[0].Stale {
		t.Errorf("an interval of 0 is off, and a recent run still vouches on the default daily interval: %+v", out)
	}
}

// §8.1: a manual claim's answer is recorded with the claim-result operation,
// not through review: the goal's file, its reviewed and updated stamps and
// its history stay as they were, and a failed manual claim heads the agenda
// like an adapter's.
func TestClaimResultRecordsAManualAnswerWithoutAReview(t *testing.T) {
	st := newServerStore(t)
	set := testSet(t)
	renderer, err := block.New(set)
	if err != nil {
		t.Fatal(err)
	}
	writeServerGoal(t, st, "telos/goal/g3.md", "G3", "global")
	if _, err := st.Review("telos/goal/g3.md", store.ReviewInput{Question: "Is this the goal as you would put it?", Verdict: store.Confirmed, Answer: "yes"}, "desk"); err != nil {
		t.Fatal(err)
	}
	goalFile := filepath.Join(st.Dir, "telos/goal/g3.md")
	before, err := os.ReadFile(goalFile)
	if err != nil {
		t.Fatal(err)
	}
	history := gitRun(t, st.Dir, "log", "--format=%H", "--", "telos/goal/g3.md")
	now := time.Now().UTC()
	d := Deps{Memory: st, Set: set, Block: renderer, Now: func() time.Time { return now }}
	own := audienceFor(set, false)

	out, err := recordClaimResult(d, "desk", false, own, claimResultIn{Goal: "telos/goal/g3.md", Index: 1, State: "fail", Note: "the venue moved it to January"})
	if err != nil || out.Commit == "" || out.Commit == "no change" || out.Goal != "telos/goal/g3.md" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if after, _ := os.ReadFile(goalFile); string(after) != string(before) {
		t.Errorf("the goal file changed:\n%s", after)
	}
	if gitRun(t, st.Dir, "log", "--format=%H", "--", "telos/goal/g3.md") != history {
		t.Error("the goal's history gained a commit")
	}
	if out, err := recordClaimResult(d, "desk", false, own, claimResultIn{Goal: "telos/goal/g3.md", Index: 1, State: "fail", Note: "the venue moved it to January"}); err != nil || out.Commit != "no change" {
		t.Errorf("the same answer again: %+v %v", out, err)
	}
	if out, err := recordClaimResult(d, "desk", false, own, claimResultIn{Goal: "telos/goal/g3.md", Index: 1, State: "fail", Note: "January, confirmed in writing"}); err != nil || out.Commit == "no change" {
		t.Fatalf("a new answer in the same state: %+v %v", out, err)
	}
	if subject := gitRun(t, st.Dir, "log", "-1", "--format=%s"); !strings.HasPrefix(subject, "claims telos/goal g3: 0 changed, 1 answered") {
		t.Errorf("subject = %q", subject)
	}
	listed, _ := claimsFor(d, "desk", own, "telos/goal/g3.md")
	if len(listed.Claims) != 3 || listed.Claims[1].State != "fail" || listed.Claims[1].Detail != "January, confirmed in writing" {
		t.Errorf("claims = %+v", listed)
	}
	_, _, top, err := RenderContext(d, "desk", "", false)
	if err != nil || top == nil || top.Reason != agenda.Fail || top.ClaimText != "date holds" || top.ClaimIndex == nil || *top.ClaimIndex != 1 {
		t.Errorf("a failed manual claim heads the agenda: %+v %v", top, err)
	}

	for name, in := range map[string]claimResultIn{
		"an index past the last claim":  {Goal: "telos/goal/g3.md", Index: 3, State: "pass"},
		"a negative index":              {Goal: "telos/goal/g3.md", Index: -1, State: "pass"},
		"unchecked, which is no answer": {Goal: "telos/goal/g3.md", Index: 1, State: "unchecked"},
		"a state outside §8.1":          {Goal: "telos/goal/g3.md", Index: 1, State: "maybe"},
		"a goal that is not there":      {Goal: "telos/goal/none.md", Index: 0, State: "pass"},
	} {
		if _, err := recordClaimResult(d, "desk", false, own, in); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := recordClaimResult(d, "desk", true, audienceFor(set, true), claimResultIn{Goal: "telos/goal/g3.md", Index: 1, State: "pass"}); err == nil {
		t.Error("a consumer never records a result")
	}
}

// §8.1, AT: an adapter's pass or fail is the kernel's evidence, so a session
// cannot overwrite it — not even to clear a fail the person disputes. Only a
// manual claim's answer enters through claim_result.
func TestASessionCannotOverwriteAnAdapterClaimsResult(t *testing.T) {
	st := newServerStore(t)
	set := testSet(t)
	writeServerGoal(t, st, "telos/goal/g3.md", "G3", "global")
	t0 := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
	if _, err := st.RecordClaimResults("telos/goal/g3.md", []store.ClaimResult{
		{Index: 0, Text: "three articles", Adapter: "tracker", State: store.Fail, Detail: "2 found", Since: t0, Recorded: t0},
	}, "kernel"); err != nil {
		t.Fatal(err)
	}
	head := gitRun(t, st.Dir, "rev-parse", "HEAD")
	d := Deps{Memory: st, Set: set, Now: func() time.Time { return t0.Add(time.Hour) }}
	_, err := recordClaimResult(d, "desk", false, audienceFor(set, false), claimResultIn{Goal: "telos/goal/g3.md", Index: 0, State: "pass", Note: "I did publish three"})
	if err == nil || !strings.Contains(err.Error(), "tracker") || !strings.Contains(err.Error(), "§8.1") {
		t.Errorf("an adapter claim's result must be refused, naming the adapter and §8.1: %v", err)
	}
	if gitRun(t, st.Dir, "rev-parse", "HEAD") != head {
		t.Error("the refused answer reached git")
	}
	all, _ := st.ClaimResults()
	if r := all["telos/goal/g3.md"]; len(r) != 1 || r[0].State != store.Fail || !r[0].Since.Equal(t0) {
		t.Errorf("the tracker's fail must stand: %+v", r)
	}
}

// §9, §11: reflect answers about the person's whole record and is refused to
// a consumer outright — a consumer is asked nothing, and the values are the
// person's own — the same gate the other ratified-record tools apply.
func TestReflectIsRefusedToAConsumerAndServedToTheOwner(t *testing.T) {
	st := newServerStore(t)
	set := testSet(t)
	if _, err := st.Write("identity/value/family-time.md", store.Record{
		Name: "family-time", Description: "a value", Module: "identity", Kind: "value", Scope: "global",
		Fields: map[string]string{"statement": "family time matters most"}, Body: "b",
	}, "desk"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Review("identity/value/family-time.md", store.ReviewInput{Question: "Still one of the things you weigh decisions against?", Verdict: store.Confirmed, Answer: "yes"}, "desk"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write("telos/goal/g1.md", store.Record{
		Name: "g1", Description: "a goal", Module: "telos", Kind: "goal", Scope: "global",
		Fields: map[string]string{"id": "G1", "title": "Ship the guide", "ideal": "published", "by": "2026-12-01", "serves": "family-time"}, Body: "b",
	}, "desk"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write("telos/goal/away.md", store.Record{
		Name: "away", Description: "a goal", Module: "telos", Kind: "goal", Scope: "machine/other",
		Fields: map[string]string{"id": "G9", "title": "Elsewhere", "ideal": "elsewhere", "by": "2026-12-01"}, Body: "b",
	}, "desk"); err != nil {
		t.Fatal(err)
	}
	d := Deps{Memory: st, Set: set, Now: func() time.Time { return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) }}

	out, err := reflectFor(d, "desk", false, audienceFor(set, false))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Values) != 1 || out.Values[0].Name != "family-time" || !out.Values[0].Confirmed {
		t.Fatalf("values = %+v", out.Values)
	}
	if len(out.Values[0].Goals) != 1 || out.Values[0].Goals[0].ID != "G1" {
		t.Errorf("family-time goals = %+v", out.Values[0].Goals)
	}
	for _, g := range append(out.Values[0].Goals, out.Unserved...) {
		if g.ID == "G9" {
			t.Error("a goal scoped to another machine must not be listed")
		}
	}

	if _, err := reflectFor(d, "desk", true, audienceFor(set, true)); err == nil {
		t.Error("a consumer must be refused")
	}
}

// §8.1: the claims tool shows today's state beside what was measured. An
// open paced claim reads its count and days left, a yes-or-no claim past its
// pace is behind, and a stale pass stays marked.
func TestTheClaimsToolShowsTodaysStateAndWhatWasMeasured(t *testing.T) {
	st := newServerStore(t)
	set := testSet(t)
	claims := "- text: \"three articles\"\n  by: 2026-10-31\n  check: {adapter: tracker, label: a, since: 2026-09-01, min: 3}\n" +
		"- text: \"ship it\"\n  by: 2026-10-05\n  effort: 10d\n  check: {adapter: manual}\n" +
		"- text: \"a commit\"\n  standing: true\n  check: {adapter: forge, repo: a/b, since: -14d, min: 1}"
	if _, err := st.Write("telos/goal/g3.md", store.Record{Name: "g3", Description: "a goal", Module: "telos", Kind: "goal", Scope: "global",
		Fields: map[string]string{"id": "G3", "title": "Ship", "ideal": "published", "by": "2026-12-01", "claims": claims}, Body: "b"}, "desk"); err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
	two := 2
	if _, err := st.RecordClaimResults("telos/goal/g3.md", []store.ClaimResult{
		{Index: 0, Text: "three articles", Adapter: "tracker", State: store.Fail, Count: &two, Since: t0, Recorded: t0},
		{Index: 1, Text: "ship it", Adapter: "manual", State: store.Fail, Since: t0, Recorded: t0},
		{Index: 2, Text: "a commit", Adapter: "forge", State: store.Pass, Since: t0, Recorded: t0},
	}, "kernel"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC)
	d := Deps{Memory: st, Set: set, Now: func() time.Time { return now },
		LastRun: func() (time.Time, bool) { return now.Add(-72 * time.Hour), true }, ClaimInterval: 24 * time.Hour}
	out, err := claimsFor(d, "desk", audienceFor(set, false), "")
	if err != nil || len(out.Claims) != 3 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	c := out.Claims
	if c[0].State != "open" || c[0].Measured != "fail" || c[0].Count == nil || *c[0].Count != 2 || c[0].Target == nil || *c[0].Target != 3 ||
		c[0].Expected == nil || *c[0].Expected != 1 || c[0].Unreadable != "" || c[0].DaysLeft == nil || *c[0].DaysLeft != 31 || c[0].Deadline != "2026-10-31" {
		t.Errorf("the paced claim = %+v", c[0])
	}
	if c[1].State != "behind" || c[1].Measured != "fail" || c[1].Effort == nil || *c[1].Effort != 10 {
		t.Errorf("the yes-or-no claim = %+v", c[1])
	}
	if c[2].State != "pass" || c[2].Measured != "pass" || !c[2].Standing || !c[2].Stale {
		t.Errorf("a stale standing pass keeps its mark: %+v", c[2])
	}
}

// §8.1: a manual count is the answer to a claim with of; the state follows
// from the count, so a session cannot say pass for a count under the target.
func TestClaimResultRecordsAManualCount(t *testing.T) {
	st := newServerStore(t)
	set := testSet(t)
	claims := "- text: \"ten sessions\"\n  check: {adapter: manual, of: 10, since: 2026-09-01}\n" +
		"- text: \"date holds\"\n  standing: true\n  check: {adapter: manual}\n" +
		"- text: \"standing count\"\n  standing: true\n  check: {adapter: manual, of: 10}"
	if _, err := st.Write("telos/goal/g3.md", store.Record{Name: "g3", Description: "a goal", Module: "telos", Kind: "goal", Scope: "global",
		Fields: map[string]string{"id": "G3", "title": "Train", "ideal": "fit", "by": "2026-12-01", "claims": claims}, Body: "b"}, "desk"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC)
	d := Deps{Memory: st, Set: set, Now: func() time.Time { return now }}
	own := audienceFor(set, false)
	four, ten, neg := 4, 10, -1
	rec := func(i int, state string, n *int) error {
		_, err := recordClaimResult(d, "desk", false, own, claimResultIn{Goal: "telos/goal/g3.md", Index: i, State: state, Count: n})
		return err
	}
	if err := rec(0, "fail", &four); err != nil {
		t.Fatal(err)
	}
	res, _ := st.ClaimResults()
	r := res["telos/goal/g3.md"][0]
	if r.State != store.Fail || r.Count == nil || *r.Count != 4 || r.Target == nil || *r.Target != 10 {
		t.Errorf("result = %+v", r)
	}
	if err := rec(0, "pass", &ten); err != nil {
		t.Fatal(err)
	}
	res, _ = st.ClaimResults()
	if r := res["telos/goal/g3.md"][0]; r.State != store.Pass || *r.Count != 10 {
		t.Errorf("result = %+v", r)
	}
	for name, err := range map[string]error{
		"pass under the target":         rec(0, "pass", &four),
		"fail at the target":            rec(0, "fail", &ten),
		"a count on a claim without of": rec(1, "pass", &four),
		"a count on a standing claim":   rec(2, "fail", &four),
		"a negative count":              rec(0, "fail", &neg),
		"a manual count with no count":  rec(0, "fail", nil),
	} {
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := rec(0, "no-evidence", nil); err != nil {
		t.Errorf("no-evidence needs no count: %v", err)
	}
}

// §8.1: the reflection marks an adapter's pass older than two claim
// intervals as stale, so the interviewer can report it as unknown; a manual
// pass is never stale by the schedule.
func TestTheReflectionMarksAStaleAdapterPass(t *testing.T) {
	st := newServerStore(t)
	set := testSet(t)
	writeServerGoal(t, st, "telos/goal/g3.md", "G3", "global")
	t0 := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
	if _, err := st.RecordClaimResults("telos/goal/g3.md", []store.ClaimResult{
		{Index: 0, Text: "three articles", Adapter: "tracker", State: store.Pass, Since: t0, Recorded: t0},
		{Index: 1, Text: "date holds", Adapter: "manual", State: store.Pass, Since: t0, Recorded: t0},
	}, "kernel"); err != nil {
		t.Fatal(err)
	}
	now := t0.Add(96 * time.Hour)
	last := now.Add(-72 * time.Hour)
	d := Deps{Memory: st, Set: set, Now: func() time.Time { return now },
		LastRun: func() (time.Time, bool) { return last, true }, ClaimInterval: 24 * time.Hour}
	ref, err := reflectFor(d, "desk", false, audienceFor(set, false))
	if err != nil || len(ref.Unserved) != 1 || len(ref.Unserved[0].Claims) != 3 {
		t.Fatalf("ref=%+v err=%v", ref, err)
	}
	c := ref.Unserved[0].Claims
	if !c[0].Stale || c[1].Stale || c[2].Stale {
		t.Errorf("only the tracker pass is stale: %+v", c)
	}
	last = now.Add(-time.Hour)
	if ref, _ := reflectFor(d, "desk", false, audienceFor(set, false)); ref.Unserved[0].Claims[0].Stale {
		t.Error("a pass within two intervals of the last run is not stale")
	}
}

// writeConfirmed writes a record and confirms it, as the person's review does.
func writeConfirmed(t *testing.T, st *store.Store, path, mod, kind, sc, body string) {
	t.Helper()
	rec := store.Record{Name: strings.TrimSuffix(filepath.Base(path), ".md"), Description: body, Module: mod, Kind: kind, Scope: sc, Body: body}
	if mod != "memory" { // working memory declares no fields; the ratified kinds require a statement
		rec.Fields = map[string]string{"statement": body}
	}
	if _, err := st.Write(path, rec, "desk"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Review(path, store.ReviewInput{Question: "Still how you want to work?", Verdict: store.Confirmed, Answer: "yes"}, "desk"); err != nil {
		t.Fatal(err)
	}
}

type instructionsBody struct {
	Opening string                `json:"opening"`
	Records []instructions.Record `json:"records"`
	Sizes   []instructions.Size   `json:"sizes"`
}

func getInstructions(t *testing.T, d Deps, consumers map[string]bool, query string) instructionsBody {
	t.Helper()
	h := InstructionsHandler(d, fakeIdentity{name: "desk"}, consumers)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/instructions"+query, nil))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status=%d type=%q body=%q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"records":null`) {
		t.Errorf("records must be [], never null: %s", w.Body.String())
	}
	var got instructionsBody
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func instructionsDeps(t *testing.T, st *store.Store, set *module.Set) Deps {
	t.Helper()
	renderer, err := block.New(set)
	if err != nil {
		t.Fatal(err)
	}
	return Deps{Memory: st, Set: set, Block: renderer, Now: time.Now}
}

func TestInstructionsEndpointServesConfirmedInstructions(t *testing.T) {
	st := newServerStore(t)
	d := instructionsDeps(t, st, testSet(t))
	writeConfirmed(t, st, "identity/preference/answer.md", "identity", "preference", "global", "Answer the ask, then stop.")

	got := getInstructions(t, d, nil, "")
	if len(got.Records) != 1 || got.Records[0].Text != "Answer the ask, then stop." || got.Records[0].Module != "identity" {
		t.Errorf("records = %+v", got.Records)
	}
	if got.Opening != instructions.Opening {
		t.Errorf("opening = %q", got.Opening)
	}
	if len(got.Sizes) != 1 || got.Sizes[0].Module != "identity" || got.Sizes[0].Budget != 4096 {
		t.Errorf("sizes = %+v", got.Sizes)
	}
}

func TestInstructionsEndpointFollowsScope(t *testing.T) {
	st := newServerStore(t)
	d := instructionsDeps(t, st, testSet(t))
	writeConfirmed(t, st, "identity/preference/scoped.md", "identity", "preference", "project/a--b", "Only in a--b.")

	if got := getInstructions(t, d, nil, "?project=a--b"); len(got.Records) != 1 {
		t.Errorf("inside its project: %+v", got.Records)
	}
	if got := getInstructions(t, d, nil, ""); len(got.Records) != 0 {
		t.Errorf("outside any project: %+v", got.Records)
	}
	h := InstructionsHandler(d, fakeIdentity{name: "desk"}, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/instructions?project=not/a/slug", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("a malformed project is 400, got %d", w.Code)
	}
}

func TestInstructionsAreHiddenFromAConsumer(t *testing.T) {
	st := newServerStore(t)
	d := instructionsDeps(t, st, testSet(t))
	writeConfirmed(t, st, "identity/preference/answer.md", "identity", "preference", "global", "Answer the ask, then stop.")

	got := getInstructions(t, d, map[string]bool{"desk": true}, "")
	if len(got.Records) != 0 || len(got.Sizes) != 0 {
		t.Errorf("identity is self: a consumer gets nothing, got %+v", got)
	}
}

// A working-memory preference crosses into identity and is governed there. A
// deployment that declares memory audience any lets a consumer see the record
// itself, but not as an instruction, because identity is self.
func TestACrossingPreferenceIsNotServedToAConsumerWhenMemoryIsAny(t *testing.T) {
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "modules"))); err != nil {
		t.Fatal(err)
	}
	mf := filepath.Join(dir, "memory", "module.json")
	raw, err := os.ReadFile(mf)
	if err != nil {
		t.Fatal(err)
	}
	patched := strings.Replace(string(raw), `"profile": "working-memory",`, `"profile": "working-memory", "audience": "any",`, 1)
	if patched == string(raw) {
		t.Fatal("memory manifest no longer has the line this test patches")
	}
	if err := os.WriteFile(mf, []byte(patched), 0o644); err != nil {
		t.Fatal(err)
	}
	set, err := module.Load(dir, []string{"memory", "identity", "telos", "health"})
	if err != nil {
		t.Fatal(err)
	}
	if audienceFor(set, true).Hides("memory") {
		t.Fatal("fixture: memory should be visible to a consumer")
	}
	st := newServerStore(t)
	st.SetModules(set)
	d := instructionsDeps(t, st, set)
	writeConfirmed(t, st, "memory/preference/crossing.md", "memory", "preference", "global", "Never force-push.")

	own := getInstructions(t, d, nil, "")
	if len(own.Records) != 1 || own.Records[0].Path != "memory/preference/crossing.md" || own.Records[0].Module != "identity" {
		t.Fatalf("the person's own call must carry the crossing preference: %+v", own.Records)
	}
	if got := getInstructions(t, d, map[string]bool{"desk": true}, ""); len(got.Records) != 0 || len(got.Sizes) != 0 {
		t.Errorf("a consumer must not receive an identity instruction: %+v", got)
	}
}

func TestContextToolReportsInstructionSizes(t *testing.T) {
	st := newServerStore(t)
	d := instructionsDeps(t, st, testSet(t))
	writeConfirmed(t, st, "identity/preference/answer.md", "identity", "preference", "global", "Answer the ask, then stop.")

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := New(d, "desk", false).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "context", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out contextOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	want := []instructions.Size{{Module: "identity", Bytes: len("Answer the ask, then stop."), Budget: 4096}}
	if !reflect.DeepEqual(out.Instructions, want) {
		t.Errorf("instructions = %+v, want %+v", out.Instructions, want)
	}
}

func TestRenderContextPartsAgreesWithRenderContext(t *testing.T) {
	st := newServerStore(t)
	d := instructionsDeps(t, st, testSet(t))
	writeConfirmed(t, st, "identity/value/family.md", "identity", "value", "global", "Family first.")
	text, _, _, err := RenderContext(d, "desk", "", false)
	if err != nil {
		t.Fatal(err)
	}
	parts, _, items, err := RenderContextParts(d, "desk", "", false)
	if err != nil {
		t.Fatal(err)
	}
	var joined strings.Builder
	for _, p := range parts {
		joined.WriteString(p.Text)
	}
	if joined.String() != text {
		t.Errorf("parts %q != block %q", joined.String(), text)
	}
	_ = items // the full agenda; the view's What is due reads it
}
