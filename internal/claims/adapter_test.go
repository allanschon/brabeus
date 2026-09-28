package claims

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/store"
)

func TestValidateEnforcesEachAdaptersArguments(t *testing.T) {
	ok := []struct {
		adapter string
		args    map[string]string
	}{
		{"tracker", map[string]string{"label": "article", "since": "2026-07-01", "min": "3"}},
		{"forge", map[string]string{"repo": "me/side-project", "since": "-14d", "min": "1", "merged": "true"}},
		{"date", map[string]string{"before": "2026-12-31"}},
		{"manual", map[string]string{}},
		// Spec §8.1's own example, verbatim: a bare repository name is
		// accepted and resolved against BRABEUS_FORGE_OWNER at run time.
		{"tracker", map[string]string{"done": "true", "label": "article", "since": "2026-07-01", "min": "3"}},
		{"forge", map[string]string{"repo": "side-project", "since": "-14d", "min": "1"}},
		{"manual", nil},
	}
	for _, o := range ok {
		if err := Validate(o.adapter, o.args); err != nil {
			t.Errorf("%s %v: %v", o.adapter, o.args, err)
		}
	}
	bad := []struct {
		adapter string
		args    map[string]string
		want    string
	}{
		{"tracker", map[string]string{"label": "x", "since": "-0d", "min": "1"}, "since"},
		{"tracker", map[string]string{"label": "x", "since": "2026-07-01"}, "min"},
		{"tracker", map[string]string{"label": "x", "since": "2026-07-01", "min": "0"}, "min"},
		{"tracker", map[string]string{"label": "x", "since": "2026-07-01", "min": "1", "colour": "red"}, "colour"},
		{"tracker", map[string]string{"label": "x", "since": "2026-07-01", "min": "1", "done": "yes"}, "done"},
		{"forge", map[string]string{"repo": "me/side/project", "since": "-14d", "min": "1"}, "owner/name"},
		{"forge", map[string]string{"repo": "/side-project", "since": "-14d", "min": "1"}, "owner/name"},
		{"forge", map[string]string{"repo": " me / side ", "since": "-14d", "min": "1"}, "owner/name"},
		{"forge", map[string]string{"repo": "me/side", "since": "-14d", "min": "1", "merged": "1"}, "merged"},
		{"date", map[string]string{}, "before"},
		{"date", map[string]string{"before": "31/12/2026"}, "before"},
		{"date", map[string]string{"after": "2026-12-31", "before": "2026-01-01"}, "after"},
		{"manual", map[string]string{"when": "later"}, "no arguments"},
		{"wearable", map[string]string{}, "wearable"},
	}
	for _, b := range bad {
		err := Validate(b.adapter, b.args)
		if err == nil || !strings.Contains(err.Error(), b.want) {
			t.Errorf("%s %v: err=%v, want it to mention %q", b.adapter, b.args, err, b.want)
		} else if !strings.Contains(err.Error(), "spec §8.1") {
			t.Errorf("%s %v: refusal %q does not name spec §8.1", b.adapter, b.args, err)
		}
	}
}

// The adapter list exists twice — the manifest's closed set and the argument
// schemas — so this is what keeps them from drifting.
func TestTheSchemasCoverExactlyTheAdaptersAManifestMayDeclare(t *testing.T) {
	var keys []string
	for k := range schemas {
		keys = append(keys, k)
	}
	want := append([]string{}, module.Adapters...)
	sort.Strings(keys)
	sort.Strings(want)
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("schemas cover %v, module.Adapters is %v", keys, want)
	}
}

func TestResolveDateReadsAbsoluteAndRelativeForms(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if d, err := resolveDate("-14d", now); err != nil || !d.Equal(time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("-14d = %v %v", d, err)
	}
	if d, err := resolveDate("2026-07-01", now); err != nil || d.Day() != 1 || d.Month() != 7 {
		t.Errorf("date = %v %v", d, err)
	}
	for _, bad := range []string{"-0d", "14d", "yesterday", "2026-7-1"} {
		if _, err := resolveDate(bad, now); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestNewBuildsOnlyWhatIsConfiguredAndRefusesAnUnknownBackend(t *testing.T) {
	got, err := New(Config{}, nil)
	if err != nil || got["date"] == nil || got["tracker"] != nil || got["forge"] != nil || got["manual"] != nil {
		t.Errorf("bare config: %v %v", got, err)
	}
	got, err = New(Config{Tracker: "vikunja", TrackerURL: "https://tracker.example", TrackerToken: "t", Forge: "github", ForgeURL: "https://api.github.com", ForgeToken: "t"}, nil)
	if err != nil || got["tracker"] == nil || got["forge"] == nil {
		t.Errorf("full config: %v %v", got, err)
	}
	for name, a := range got {
		if a.Name() != name {
			t.Errorf("adapter under %q names itself %q", name, a.Name())
		}
	}
	if _, err := New(Config{Forge: "sourcehut"}, nil); err == nil || !strings.Contains(err.Error(), "sourcehut") {
		t.Errorf("an unknown backend must be refused at startup: %v", err)
	}
	if _, err := New(Config{Tracker: "vikunja"}, nil); err == nil || !strings.Contains(err.Error(), "BRABEUS_TRACKER_URL") || !strings.Contains(err.Error(), "spec §8.1") {
		t.Errorf("a backend without its URL must be refused naming the variable: %v", err)
	}
	if _, err := New(Config{Forge: "gitea"}, nil); err == nil || !strings.Contains(err.Error(), "BRABEUS_FORGE_URL") || !strings.Contains(err.Error(), "spec §8.1") {
		t.Errorf("gitea without its URL must be refused naming the variable: %v", err)
	}
}

func TestFromEnvReadsEveryVariableAndNormalisesTheBackendNames(t *testing.T) {
	env := map[string]string{
		"BRABEUS_TRACKER": " Vikunja ", "BRABEUS_TRACKER_URL": "https://tracker.example/", "BRABEUS_TRACKER_TOKEN": "tt",
		"BRABEUS_FORGE": "GitHub", "BRABEUS_FORGE_URL": "https://forge.example/", "BRABEUS_FORGE_TOKEN": "ft",
		"BRABEUS_FORGE_OWNER": "me",
	}
	got := FromEnv(func(k string) string { return env[k] })
	want := Config{Tracker: "vikunja", TrackerURL: "https://tracker.example", TrackerToken: "tt",
		Forge: "github", ForgeURL: "https://forge.example", ForgeToken: "ft", ForgeOwner: "me"}
	if got != want {
		t.Errorf("FromEnv = %+v, want %+v", got, want)
	}
}

func TestResolveRepoUsesTheOwnerOnlyForABareName(t *testing.T) {
	if owner, name, o := resolveRepo("me/side", "other"); o != nil || owner != "me" || name != "side" {
		t.Errorf("owner/name: %q %q %+v", owner, name, o)
	}
	if owner, name, o := resolveRepo("side-project", "me"); o != nil || owner != "me" || name != "side-project" {
		t.Errorf("bare name with owner: %q %q %+v", owner, name, o)
	}
	_, _, o := resolveRepo("side-project", "")
	if o == nil || o.State != store.NoEvidence || !strings.Contains(o.Detail, "BRABEUS_FORGE_OWNER") || !strings.Contains(o.Detail, "side-project") {
		t.Errorf("bare name without owner: %+v", o)
	}
}

// A bare repository name with no owner configured is a gap in the deployment:
// no-evidence naming the variable, and no request is made at all.
func TestABareRepositoryWithoutAnOwnerIsNoEvidenceBeforeAnyRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request should be made, got %s", r.URL)
	}))
	defer srv.Close()
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	args := map[string]string{"repo": "side-project", "since": "-14d", "min": "1"}
	for _, a := range []Adapter{
		&Gitea{URL: srv.URL, Token: "tok", Client: srv.Client()},
		&GitHub{URL: srv.URL, Token: "tok", Client: srv.Client()},
	} {
		out, err := a.Check(context.Background(), args, now)
		if err != nil || out.State != store.NoEvidence || !strings.Contains(out.Detail, "BRABEUS_FORGE_OWNER") {
			t.Errorf("%T: %+v %v", a, out, err)
		}
	}
}

// K8 end to end: spec §8.1's example block, verbatim, through the store's own
// parser and then Validate — so the README's worked example is one the kernel
// accepts, and none of its arguments needs a comma.
func TestTheSpecsExampleClaimsParseAndValidate(t *testing.T) {
	block := `- text: "At least three articles published since the quarter began"
  check: { adapter: tracker, done: true, label: article, since: 2026-07-01, min: 3 }
- text: "The side project has a commit in the last fortnight"
  check: { adapter: forge, repo: side-project, since: -14d, min: 1 }
- text: "The target date still holds"
  check: { adapter: manual }`
	got, err := store.ParseClaims(block)
	if err != nil || len(got) != 3 {
		t.Fatalf("ParseClaims: %d claims, %v", len(got), err)
	}
	for i, c := range got {
		if err := Validate(c.Adapter, c.Args); err != nil {
			t.Errorf("claim %d (%s %v): %v", i+1, c.Adapter, c.Args, err)
		}
	}
	if got[1].Args["repo"] != "side-project" || got[0].Args["label"] != "article" {
		t.Errorf("parsed args: %v %v", got[0].Args, got[1].Args)
	}
}
