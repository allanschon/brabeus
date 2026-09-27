package module

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeManifest puts JSON at <dir>/<name>/module.json.
func writeManifest(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name, "module.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const memoryJSON = `{
  "name": "memory", "version": 1, "profile": "working-memory", "priority": 20,
  "scope_keys": ["machine", "project"], "layout": "free",
  "legacy_types": {"user": "note", "feedback": "preference", "project": "project", "reference": "note"},
  "kinds": {"note": {"fields": []}, "trap": {"fields": []}, "preference": {"fields": []}, "project": {"fields": []}},
  "adapters": ["manual"], "skills": ["health"]
}`

const telosJSON = `{
  "name": "telos", "version": 1, "profile": "ratified-record", "priority": 10,
  "budget_bytes": 600, "audience": "self",
  "kinds": {
    "goal": {"fields": ["id", "title", "ideal", "by"], "optional": ["claims", "serves", "notes"],
             "freshness_days": 90, "interview": "Still right? Progress since {reviewed}?",
             "first": "What are you working toward, and by when?"},
    "decision": {"fields": ["decided", "alternatives", "prediction", "confidence", "worst_case", "revisit"]}
  },
  "summary": "summary.md.tmpl", "adapters": ["tracker", "forge", "date", "manual"]
}`

func load(t *testing.T, enabled []string, manifests map[string]string) (*Set, error) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range manifests {
		writeManifest(t, dir, name, body)
	}
	return Load(dir, enabled)
}

func wantErr(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), substr) {
		t.Fatalf("want an error containing %q, got %v", substr, err)
	}
}

func TestLoadsTheEnabledManifestsInPriorityOrder(t *testing.T) {
	set, err := load(t, []string{"memory", "telos"}, map[string]string{"memory": memoryJSON, "telos": telosJSON})
	if err != nil {
		t.Fatal(err)
	}
	if got := set.Names(); strings.Join(got, ",") != "telos,memory" {
		t.Errorf("order by priority: got %v", got)
	}
	if k, ok := set.Kind("telos", "goal"); !ok || k.FreshnessDays != 90 || k.First == "" {
		t.Errorf("telos/goal not loaded as written: %+v %v", k, ok)
	}
	if !set.HasProfile(WorkingMemory) || !set.HasProfile(RatifiedRecord) {
		t.Error("both profiles should be present")
	}
	if m, k, ok := set.LegacyKind("feedback"); !ok || m != "memory" || k != "preference" {
		t.Errorf("legacy feedback -> %s/%s %v", m, k, ok)
	}
}

func TestAnEnabledModuleThatIsNotOnDiskIsNamed(t *testing.T) {
	_, err := load(t, []string{"memory", "finance"}, map[string]string{"memory": memoryJSON})
	wantErr(t, err, "finance")
}

func TestAnUnknownKeyRefusesTheModule(t *testing.T) {
	bad := strings.Replace(telosJSON, `"priority": 10,`, `"priority": 10, "author": "x",`, 1)
	_, err := load(t, []string{"telos"}, map[string]string{"telos": bad})
	wantErr(t, err, "author")
}

func TestTheNameMustMatchTheDirectory(t *testing.T) {
	_, err := load(t, []string{"telos"}, map[string]string{"telos": strings.Replace(telosJSON, `"name": "telos"`, `"name": "goals"`, 1)})
	wantErr(t, err, "name")
}

func TestAnUnknownProfileIsRefused(t *testing.T) {
	_, err := load(t, []string{"telos"}, map[string]string{"telos": strings.Replace(telosJSON, "ratified-record", "shared", 1)})
	wantErr(t, err, "profile")
}

func TestACoreModuleMayNotWidenItsAudience(t *testing.T) {
	_, err := load(t, []string{"telos"}, map[string]string{"telos": strings.Replace(telosJSON, `"audience": "self"`, `"audience": "any"`, 1)})
	wantErr(t, err, "audience")
}

func TestANonCoreRatifiedModuleMayBeReadableByAny(t *testing.T) {
	m := strings.Replace(strings.Replace(telosJSON, `"name": "telos"`, `"name": "reading"`, 1), `"audience": "self"`, `"audience": "any"`, 1)
	set, err := load(t, []string{"reading"}, map[string]string{"reading": m})
	if err != nil {
		t.Fatal(err)
	}
	if mod, _ := set.Module("reading"); mod.Audience != Any {
		t.Errorf("audience = %q", mod.Audience)
	}
}

func TestAudienceDefaultsToTheProfiles(t *testing.T) {
	set, err := load(t, []string{"memory"}, map[string]string{"memory": memoryJSON})
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := set.Module("memory"); m.Audience != Self {
		t.Errorf("working-memory default audience = %q, want self", m.Audience)
	}
}

func TestAWorkingMemoryModuleHasNoBudgetAndNoSummary(t *testing.T) {
	_, err := load(t, []string{"memory"}, map[string]string{"memory": strings.Replace(memoryJSON, `"priority": 20,`, `"priority": 20, "budget_bytes": 500,`, 1)})
	wantErr(t, err, "budget_bytes")
	_, err = load(t, []string{"memory"}, map[string]string{"memory": strings.Replace(memoryJSON, `"priority": 20,`, `"priority": 20, "summary": "s.tmpl",`, 1)})
	wantErr(t, err, "summary")
}

func TestARatifiedModuleNeedsABudgetAndASummary(t *testing.T) {
	_, err := load(t, []string{"telos"}, map[string]string{"telos": strings.Replace(telosJSON, `"budget_bytes": 600,`, ``, 1)})
	wantErr(t, err, "budget_bytes")
	_, err = load(t, []string{"telos"}, map[string]string{"telos": strings.Replace(telosJSON, `"summary": "summary.md.tmpl",`, ``, 1)})
	wantErr(t, err, "summary")
}

func TestScopeKeysLayoutAndLegacyTypesAreWorkingMemoryOnly(t *testing.T) {
	for _, extra := range []string{`"scope_keys": ["machine"],`, `"layout": "free",`, `"legacy_types": {"user": "goal"},`} {
		_, err := load(t, []string{"telos"}, map[string]string{"telos": strings.Replace(telosJSON, `"priority": 10,`, `"priority": 10, `+extra, 1)})
		wantErr(t, err, "working-memory")
	}
}

func TestLayoutDefaultsToKindAndAcceptsOnlyTwoValues(t *testing.T) {
	set, err := load(t, []string{"memory"}, map[string]string{"memory": strings.Replace(memoryJSON, `"layout": "free",`, ``, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := set.Module("memory"); m.Layout != "kind" {
		t.Errorf("layout default = %q", m.Layout)
	}
	_, err = load(t, []string{"memory"}, map[string]string{"memory": strings.Replace(memoryJSON, `"layout": "free"`, `"layout": "tree"`, 1)})
	wantErr(t, err, "layout")
}

func TestLegacyTypesMustNameDeclaredKinds(t *testing.T) {
	_, err := load(t, []string{"memory"}, map[string]string{"memory": strings.Replace(memoryJSON, `"user": "note"`, `"user": "fact"`, 1)})
	wantErr(t, err, "fact")
}

func TestAKindMayNotUseAReservedFieldNameExceptId(t *testing.T) {
	_, err := load(t, []string{"telos"}, map[string]string{"telos": strings.Replace(telosJSON, `["id", "title", "ideal", "by"]`, `["id", "title", "scope"]`, 1)})
	wantErr(t, err, "scope")
	if _, err := load(t, []string{"telos"}, map[string]string{"telos": telosJSON}); err != nil {
		t.Errorf("id is allowed: %v", err)
	}
}

func TestOnboardingNamesKindsThatHaveAFirstQuestion(t *testing.T) {
	ok := strings.Replace(telosJSON, `"priority": 10,`, `"priority": 10, "onboarding": ["goal"],`, 1)
	if _, err := load(t, []string{"telos"}, map[string]string{"telos": ok}); err != nil {
		t.Errorf("goal has a first question: %v", err)
	}
	_, err := load(t, []string{"telos"}, map[string]string{"telos": strings.Replace(telosJSON, `"priority": 10,`, `"priority": 10, "onboarding": ["decision"],`, 1)})
	wantErr(t, err, "first")
	_, err = load(t, []string{"telos"}, map[string]string{"telos": strings.Replace(telosJSON, `"priority": 10,`, `"priority": 10, "onboarding": ["wish"],`, 1)})
	wantErr(t, err, "wish")
	_, err = load(t, []string{"memory"}, map[string]string{"memory": strings.Replace(memoryJSON, `"priority": 20,`, `"priority": 20, "onboarding": ["note"],`, 1)})
	wantErr(t, err, "onboarding")
}

func TestAFieldMayNotBeBothRequiredAndOptional(t *testing.T) {
	_, err := load(t, []string{"telos"}, map[string]string{"telos": strings.Replace(telosJSON, `"optional": ["claims", "serves", "notes"]`, `"optional": ["title"]`, 1)})
	wantErr(t, err, "title")
}

func TestAModuleNeedsAtLeastOneKind(t *testing.T) {
	const noKinds = `{"name": "telos", "version": 1, "profile": "ratified-record", "priority": 10,
	  "budget_bytes": 600, "kinds": {}, "summary": "summary.md.tmpl"}`
	_, err := load(t, []string{"telos"}, map[string]string{"telos": noKinds})
	wantErr(t, err, "kind")
}

func TestVersionMustBeOne(t *testing.T) {
	_, err := load(t, []string{"telos"}, map[string]string{"telos": strings.Replace(telosJSON, `"version": 1`, `"version": 2`, 1)})
	wantErr(t, err, "version")
}

func TestBudgetsMustFitUnderTheCapLessTheReservation(t *testing.T) {
	big := strings.Replace(telosJSON, `"budget_bytes": 600`, `"budget_bytes": 1793`, 1)
	_, err := load(t, []string{"telos"}, map[string]string{"telos": big})
	wantErr(t, err, "1792")
	fits := strings.Replace(telosJSON, `"budget_bytes": 600`, `"budget_bytes": 1792`, 1)
	if _, err := load(t, []string{"telos"}, map[string]string{"telos": fits}); err != nil {
		t.Errorf("1792 fits exactly: %v", err)
	}
}

// The shipped manifests are the contract's worked example. They must load,
// and they must fit the budget, or the README's promise is false.
func TestTheShippedManifestsLoad(t *testing.T) {
	dir := filepath.Join("..", "..", "modules")
	set, err := Load(dir, []string{"memory", "identity", "telos", "health", "finance"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(set.Names(), ","); got != "identity,telos,health,memory,finance" {
		t.Errorf("priority order: %s", got)
	}
	if m, _ := set.Module("memory"); m.Layout != "free" || len(m.LegacyTypes) != 4 {
		t.Errorf("memory manifest: layout=%q legacy_types=%v", m.Layout, m.LegacyTypes)
	}
	for _, name := range []string{"identity", "telos", "health", "finance"} {
		if m, _ := set.Module(name); m.Audience != Self {
			t.Errorf("%s audience = %q", name, m.Audience)
		}
	}
	if m, _ := set.Module("identity"); len(m.Onboarding) == 0 || m.Onboarding[0] != "value" {
		t.Errorf("identity onboarding = %v; the first interview starts with values (§9)", m.Onboarding)
	}
}

func TestTheBundleIsFixedByTheProfile(t *testing.T) {
	wm, _ := WorkingMemory.Bundle()
	rr, _ := RatifiedRecord.Bundle()
	if !wm.ModelWrites || wm.Rendered || !wm.SearchedDefault || wm.ReviewRequired || !wm.ScopeKeys || wm.Interviewed {
		t.Errorf("working-memory bundle wrong: %+v", wm)
	}
	if rr.ModelWrites || !rr.Rendered || rr.SearchedDefault || !rr.ReviewRequired || rr.ScopeKeys || !rr.Interviewed {
		t.Errorf("ratified-record bundle wrong: %+v", rr)
	}
	if _, ok := Profile("shared").Bundle(); ok {
		t.Error("an unknown profile has no bundle")
	}
}
