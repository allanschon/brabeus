package module

import (
	"fmt"
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

const identityJSON = `{
  "name": "identity", "version": 1, "profile": "ratified-record", "priority": 5,
  "budget_bytes": 300, "audience": "self",
  "kinds": {
    "preference": {"fields": ["statement"], "freshness_days": 120, "interview": "Still how you want to be worked with?"}
  },
  "summary": "summary.md.tmpl"
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
	if m, _ := set.Module("identity"); len(m.Onboarding) == 0 || m.Onboarding[0] != "register" {
		t.Errorf("identity onboarding = %v; getting to know the person starts by agreeing how to talk (§7)", m.Onboarding)
	}
}

// B: the crossing rule (spec §7) lives once, in module.Set.RuleFor, and both
// the agenda and Review ask it instead of keeping their own copy.
func TestRuleForOwnModuleCase(t *testing.T) {
	set, err := load(t, []string{"telos"}, map[string]string{"telos": telosJSON})
	if err != nil {
		t.Fatal(err)
	}
	man, k, ok := set.RuleFor("telos", "goal")
	if !ok || man.Name != "telos" || k.FreshnessDays != 90 {
		t.Errorf("own-module case: man=%+v k=%+v ok=%v", man, k, ok)
	}
}

func TestRuleForCrossingCase(t *testing.T) {
	set, err := load(t, []string{"memory", "identity"}, map[string]string{"memory": memoryJSON, "identity": identityJSON})
	if err != nil {
		t.Fatal(err)
	}
	man, k, ok := set.RuleFor("memory", "preference")
	if !ok || man.Name != "identity" || len(k.Fields) != 1 || k.Fields[0] != "statement" {
		t.Errorf("crossing case: man=%+v k=%+v ok=%v, want identity's preference kind", man, k, ok)
	}
}

func TestRuleForNonCrossingWorkingMemoryCase(t *testing.T) {
	set, err := load(t, []string{"memory", "identity"}, map[string]string{"memory": memoryJSON, "identity": identityJSON})
	if err != nil {
		t.Fatal(err)
	}
	man, k, ok := set.RuleFor("memory", "note")
	if !ok || man.Name != "memory" || k.FreshnessDays != 0 || len(k.Fields) != 0 {
		t.Errorf("non-crossing working-memory case: man=%+v k=%+v ok=%v, want memory's own manifest and a zero kind", man, k, ok)
	}
}

func TestRuleForCrossingWhenTheGoverningModuleIsNotEnabled(t *testing.T) {
	set, err := load(t, []string{"memory"}, map[string]string{"memory": memoryJSON})
	if err != nil {
		t.Fatal(err)
	}
	man, k, ok := set.RuleFor("memory", "preference")
	if !ok || man.Name != "memory" || len(k.Fields) != 0 || k.FreshnessDays != 0 {
		t.Errorf("identity not enabled: man=%+v k=%+v ok=%v, want the working-memory manifest and a zero kind", man, k, ok)
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

// v1.7 §6: lenses are two to four questions, draft and intro are strings, and a
// working-memory kind declares none of them, because how to ask belongs to a
// ratified module and working memory is never interviewed.
func TestLensesDraftAndIntroAreValidatedAndRatifiedOnly(t *testing.T) {
	withLenses := func(n int) string {
		qs := make([]string, n)
		for i := range qs {
			qs[i] = fmt.Sprintf(`"way %d?"`, i)
		}
		return strings.Replace(telosJSON, `"first": "What are you working toward, and by when?"`,
			`"first": "What are you working toward, and by when?", "lenses": [`+strings.Join(qs, ",")+`], "draft": "Is this the goal as you would put it?"`, 1)
	}
	if _, err := load(t, []string{"telos"}, map[string]string{"telos": strings.Replace(withLenses(2), `"priority": 10,`, `"priority": 10, "intro": "direction",`, 1)}); err != nil {
		t.Fatalf("two lenses, a draft and an intro are valid: %v", err)
	}
	for _, n := range []int{1, 5} {
		_, err := load(t, []string{"telos"}, map[string]string{"telos": withLenses(n)})
		wantErr(t, err, "lenses")
	}
	wm := strings.Replace(memoryJSON, `"note": {"fields": []}`, `"note": {"fields": [], "lenses": ["a?", "b?"]}`, 1)
	_, err := load(t, []string{"memory"}, map[string]string{"memory": wm})
	wantErr(t, err, "lenses")
	wm = strings.Replace(memoryJSON, `"priority": 20,`, `"priority": 20, "intro": "notes",`, 1)
	_, err = load(t, []string{"memory"}, map[string]string{"memory": wm})
	wantErr(t, err, "intro")
}

// v1.7 AI: due_field names a declared field of the kind.
func TestDueFieldMustNameADeclaredField(t *testing.T) {
	ok := strings.Replace(telosJSON, `"worst_case", "revisit"]`, `"worst_case", "revisit"], "due_field": "revisit"`, 1)
	if _, err := load(t, []string{"telos"}, map[string]string{"telos": ok}); err != nil {
		t.Fatalf("revisit is declared: %v", err)
	}
	bad := strings.Replace(telosJSON, `"worst_case", "revisit"]`, `"worst_case", "revisit"], "due_field": "deadline"`, 1)
	_, err := load(t, []string{"telos"}, map[string]string{"telos": bad})
	wantErr(t, err, "deadline")
}

// §8.1: the kernel refuses an adapter the module did not declare, so it must
// know which adapters exist; a manifest naming one it does not know is refused.
func TestAManifestMayDeclareOnlyKnownAdapters(t *testing.T) {
	bad := strings.Replace(telosJSON, `"adapters": ["tracker", "forge", "date", "manual"]`, `"adapters": ["tracker", "wearable"]`, 1)
	_, err := load(t, []string{"telos"}, map[string]string{"telos": bad})
	wantErr(t, err, "wearable")
}

// §7: register is first in identity's onboarding, and memory declares thread
// with belongs_to. Properties of the shipped modules, tested here because the
// kernel attaches no meaning to manifest content.
func TestTheShippedIdentityStartsWithRegisterAndMemoryDeclaresThread(t *testing.T) {
	set, err := Load(filepath.Join("..", "..", "modules"), []string{"memory", "identity", "telos"})
	if err != nil {
		t.Fatal(err)
	}
	id, _ := set.Module("identity")
	if len(id.Onboarding) == 0 || id.Onboarding[0] != "register" {
		t.Errorf("identity onboarding = %v; register comes first (§7)", id.Onboarding)
	}
	if id.Intro == "" {
		t.Error("identity has no intro")
	}
	for kind, k := range id.Kinds {
		if len(k.Lenses) < 2 && k.First != "" {
			t.Errorf("identity/%s has a first question but no lenses", kind)
		}
	}
	mem, _ := set.Module("memory")
	th, ok := mem.Kinds[Thread]
	if !ok || len(th.Fields) != 1 || th.Fields[0] != BelongsTo {
		t.Errorf("memory/thread = %+v %v; it declares belongs_to", th, ok)
	}
	tl, _ := set.Module("telos")
	if tl.Kinds["decision"].DueField != "revisit" {
		t.Errorf("decision.due_field = %q", tl.Kinds["decision"].DueField)
	}
}

func TestInstructionsKeys(t *testing.T) {
	ratified := func(kinds, extra string) string {
		return `{"name":"m","version":1,"profile":"ratified-record","priority":1,"budget_bytes":100,"summary":"s.tmpl",` +
			`"kinds":{` + kinds + `}` + extra + `}`
	}
	cases := []struct {
		name, manifest, wantErr string
	}{
		{"marked with budget and source loads",
			ratified(`"pref":{"fields":["statement"],"optional":["source"],"instructions":true}`, `,"instructions_budget_bytes":4096`), ""},
		{"marked without a budget is refused",
			ratified(`"pref":{"fields":["statement"],"optional":["source"],"instructions":true}`, ``), "instructions_budget_bytes"},
		{"marked without source is refused",
			ratified(`"pref":{"fields":["statement"],"instructions":true}`, `,"instructions_budget_bytes":4096`), "source"},
		{"a budget with nothing marked is refused",
			ratified(`"pref":{"fields":["statement"]}`, `,"instructions_budget_bytes":4096`), "instructions"},
		{"working-memory may not mark a kind",
			`{"name":"m","version":1,"profile":"working-memory","priority":1,"kinds":{"note":{"fields":[],"optional":["source"],"instructions":true}}}`, "working-memory"},
		{"working-memory may not declare a budget",
			`{"name":"m","version":1,"profile":"working-memory","priority":1,"instructions_budget_bytes":10,"kinds":{"note":{"fields":[]}}}`, "working-memory"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := load(t, []string{"m"}, map[string]string{"m": c.manifest})
			if c.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
				t.Fatalf("want error containing %q, got %v", c.wantErr, err)
			}
		})
	}
}

func TestInstructionKindFollowsTheCrossing(t *testing.T) {
	const marked = `{
  "name": "identity", "version": 1, "profile": "ratified-record", "priority": 5,
  "budget_bytes": 300, "instructions_budget_bytes": 4096, "audience": "self",
  "kinds": {
    "preference": {"fields": ["statement"], "optional": ["source"], "instructions": true},
    "value": {"fields": ["statement"]}
  },
  "summary": "summary.md.tmpl"
}`
	set, err := load(t, []string{"memory", "identity"}, map[string]string{"memory": memoryJSON, "identity": marked})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		mod, kind string
		want      bool
	}{
		{"memory", "preference", true},
		{"identity", "preference", true},
		{"identity", "value", false},
		{"memory", "note", false},
	} {
		if k, got := set.InstructionKind(c.mod, c.kind); got != c.want || k.Instructions != c.want {
			t.Errorf("InstructionKind(%q, %q) = %+v, %v; want %v", c.mod, c.kind, k, got, c.want)
		}
	}

	bare, err := load(t, []string{"memory"}, map[string]string{"memory": memoryJSON})
	if err != nil {
		t.Fatal(err)
	}
	if _, got := bare.InstructionKind("memory", "preference"); got {
		t.Error("identity not enabled: memory's preference is not an instruction")
	}
	var none *Set
	if _, got := none.InstructionKind("memory", "preference"); got {
		t.Error("a nil set governs nothing")
	}
}

func ratified(kinds string) string {
	return `{"name": "telos", "version": 1, "profile": "ratified-record", "priority": 10,
	  "budget_bytes": 600, "summary": "summary.md.tmpl", "kinds": ` + kinds + `}`
}

// Spec §6: a kind declares a layout from the kernel's closed set, or a
// template it ships, never both; every field it names must be its own.
func TestAViewKeyIsALayoutOrATemplateAndNamesOnlyDeclaredFields(t *testing.T) {
	for _, c := range []struct{ name, kinds, err string }{
		{"layout", `{"goal": {"fields": ["title", "by"], "view": {"layout": "cards", "title": "title", "fields": ["by"]}}}`, ""},
		{"template", `{"goal": {"fields": ["title"], "view": {"template": "view/goal.html.tmpl"}}}`, ""},
		{"both", `{"goal": {"fields": ["title"], "view": {"layout": "list", "template": "view/goal.html.tmpl"}}}`, "both a template and a layout"},
		{"unknown layout", `{"goal": {"fields": ["title"], "view": {"layout": "grid"}}}`, `layout "grid"`},
		{"undeclared field", `{"goal": {"fields": ["title"], "view": {"layout": "table", "fields": ["owner"]}}}`, `"owner" is not a field of this kind`},
		{"group_by", `{"goal": {"fields": ["title"], "view": {"layout": "list", "group_by": "title"}}}`, "group_by is not supported yet; use sort (spec §6)"},
		{"group_by with a template", `{"goal": {"fields": ["title"], "view": {"template": "view/goal.html.tmpl", "group_by": "title"}}}`, "group_by is not supported yet"},
		{"escaping template", `{"goal": {"fields": ["title"], "view": {"template": "../x.html.tmpl"}}}`, "inside the module"},
		{"unknown view key", `{"goal": {"fields": ["title"], "view": {"layout": "list", "colour": "red"}}}`, "unknown field"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := load(t, []string{"telos"}, map[string]string{"telos": ratified(c.kinds)})
			if c.err == "" {
				if err != nil {
					t.Fatalf("want loaded, got %v", err)
				}
				return
			}
			wantErr(t, err, c.err)
		})
	}
}

// Spec §6: the model's notes are listed by the kernel, so a working-memory
// module has no say in how.
func TestAWorkingMemoryModuleMayNotDeclareAView(t *testing.T) {
	m := strings.Replace(memoryJSON, `"note": {"fields": []}`, `"note": {"fields": [], "view": {"layout": "list"}}`, 1)
	_, err := load(t, []string{"memory"}, map[string]string{"memory": m})
	wantErr(t, err, "a working-memory module may not declare view")
}

// Spec §6: a kind that says nothing about the view is a list titled by its
// first declared field, showing every declared field.
func TestAKindWithNoViewIsAListOfItsDeclaredFields(t *testing.T) {
	k := Kind{Fields: []string{"id", "title"}, Optional: []string{"notes"}}
	v := k.ViewOrDefault()
	if v.Layout != "list" || v.Title != "id" || strings.Join(v.Fields, ",") != "id,title,notes" {
		t.Errorf("default view = %+v", v)
	}
	declared := Kind{Fields: []string{"title"}, View: &View{Layout: "cards", Title: "title"}}
	if got := declared.ViewOrDefault(); got.Layout != "cards" {
		t.Errorf("declared view lost: %+v", got)
	}
}
