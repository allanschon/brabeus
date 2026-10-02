package agenda

import (
	"strings"
	"testing"

	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/store"
)

func TestReflectGroupsGoalsByTheValuesTheyServeAndListsTheRest(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	claims := "- text: \"three articles\"\n  standing: true\n  check: {adapter: tracker, label: a, since: -30d, min: 3}\n- text: \"date holds\"\n  check: {adapter: manual}"
	records := []store.Stored{
		rec("identity/value/family-time.md", "identity", "value", map[string]string{"statement": "family time matters most"}, "2026-06-01T00:00:00Z", "2026-06-01T00:00:00Z", 0),
		rec("identity/value/craft.md", "identity", "value", map[string]string{"statement": "craft"}, "2026-06-01T00:00:00Z", "", 0),
		rec("telos/goal/g1.md", "telos", "goal", map[string]string{"id": "G1", "title": "Ship the guide", "by": "2026-12-01", "serves": "Craft, family-time", "claims": claims}, "2026-06-28T00:00:00Z", "2026-06-28T00:00:00Z", 0),
		rec("telos/goal/g2.md", "telos", "goal", map[string]string{"id": "G2", "title": "Run a 10k", "by": "2027-05-01", "serves": "health"}, "2026-09-01T00:00:00Z", "", 0),
		rec("telos/goal/g3.md", "telos", "goal", map[string]string{"id": "G3", "title": "Nothing named", "by": "2027-01-01"}, "2026-09-01T00:00:00Z", "2026-09-01T00:00:00Z", 0),
	}
	retired := rec("identity/value/old.md", "identity", "value", map[string]string{"statement": "old"}, "2025-01-01T00:00:00Z", "2025-01-01T00:00:00Z", 0)
	retired.Retired = at("2026-01-01T00:00:00Z")
	records = append(records, retired)
	results := map[string][]store.ClaimResult{"telos/goal/g1.md": {{Index: 0, Text: "three articles", Adapter: "tracker", State: store.Fail, Since: at("2026-09-20T00:00:00Z")}}}
	ref := Reflect(testSet(), records, results, now)
	if len(ref.Values) != 2 || ref.Values[0].Name != "family-time" || ref.Values[1].Name != "craft" || ref.Values[1].Confirmed {
		t.Fatalf("values = %+v", ref.Values)
	}
	g := ref.Values[0].Goals
	if len(g) != 1 || g[0].ID != "G1" || g[0].DaysSinceConfirmed != 95 || len(g[0].Claims) != 2 {
		t.Errorf("family-time goals = %+v", g)
	}
	if g[0].Claims[0].State != store.Fail || g[0].Claims[1].State != store.Open || g[0].Claims[1].Measured != store.Unchecked || !g[0].Claims[1].Manual {
		t.Errorf("claims = %+v", g[0].Claims)
	}
	if len(ref.Values[1].Goals) != 1 || ref.Values[1].Goals[0].ID != "G1" {
		t.Errorf("craft goals = %+v (matching is case-insensitive)", ref.Values[1].Goals)
	}
	var unserved []string
	for _, u := range ref.Unserved {
		unserved = append(unserved, u.ID)
	}
	if strings.Join(unserved, ",") != "G2,G3" || ref.Unserved[0].DaysSinceConfirmed != -1 {
		t.Errorf("unserved = %+v", ref.Unserved)
	}
	if len(ref.Unknown) != 1 || ref.Unknown[0] != "health" {
		t.Errorf("unknown = %v; a name matching no live value is reported, not dropped", ref.Unknown)
	}
}

// RuleFor alone returns ok=true for a working-memory module's own kind (a
// zero Kind, an uninterviewed manifest); Reflect must also check
// Bundle().Interviewed — through module.Set.Interviewed — or a working-memory
// kind named "goal" would be read as one of the person's goals.
func TestReflectExcludesAGoalKindGovernedByAWorkingMemoryModule(t *testing.T) {
	set := &module.Set{Modules: []module.Manifest{
		{Name: "identity", Profile: module.RatifiedRecord, Priority: 5,
			Kinds: map[string]module.Kind{"value": {Fields: []string{"statement"}}}},
		{Name: "memory", Profile: module.WorkingMemory, Priority: 20, Layout: "free",
			Kinds: map[string]module.Kind{"goal": {}}},
	}}
	records := []store.Stored{
		rec("identity/value/craft.md", "identity", "value", map[string]string{"statement": "craft"}, "2026-06-01T00:00:00Z", "2026-06-01T00:00:00Z", 0),
		rec("memory/goal/g1.md", "memory", "goal", map[string]string{"id": "G1", "title": "Not a real goal"}, "2026-06-01T00:00:00Z", "", 0),
	}
	ref := Reflect(set, records, nil, at("2026-10-01T00:00:00Z"))
	if len(ref.Values[0].Goals) != 0 || len(ref.Unserved) != 0 {
		t.Errorf("a working-memory goal is not interviewed; it must not appear at all: values=%+v unserved=%+v", ref.Values, ref.Unserved)
	}
}

// §8.1: the reflection carries today's state beside what was measured: an
// open paced claim is work remaining, with its count and days left, and a
// yes-or-no claim past its pace is behind; neither is the stored fail.
func TestReflectShowsTodaysClaimStateBesideWhatWasMeasured(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	claims := "- text: \"three articles\"\n  by: 2026-10-31\n  check: {adapter: tracker, label: a, since: 2026-09-01, min: 3}\n" +
		"- text: \"ship it\"\n  by: 2026-10-05\n  effort: 10d\n  check: {adapter: manual}"
	records := []store.Stored{
		rec("identity/value/craft.md", "identity", "value", map[string]string{"statement": "craft"}, "2026-06-01T00:00:00Z", "2026-06-01T00:00:00Z", 0),
		rec("telos/goal/g1.md", "telos", "goal", map[string]string{"id": "G1", "title": "Ship", "by": "2026-12-01", "serves": "craft", "claims": claims}, "2026-06-28T00:00:00Z", "2026-06-28T00:00:00Z", 0),
	}
	two := 2
	results := map[string][]store.ClaimResult{"telos/goal/g1.md": {
		{Index: 0, Text: "three articles", Adapter: "tracker", State: store.Fail, Count: &two, Since: at("2026-09-20T00:00:00Z")},
		{Index: 1, Text: "ship it", Adapter: "manual", State: store.Fail, Since: at("2026-09-20T00:00:00Z")},
	}}
	g := Reflect(testSet(), records, results, now).Values[0].Goals[0].Claims
	if len(g) != 2 {
		t.Fatalf("claims = %+v", g)
	}
	p := g[0]
	if p.State != store.Open || p.Measured != store.Fail || p.Count == nil || *p.Count != 2 || p.Target == nil || *p.Target != 3 ||
		p.Expected == nil || *p.Expected != 1 || p.DaysLeft == nil || *p.DaysLeft != 31 || p.Deadline != "2026-10-31" {
		t.Errorf("the paced claim = %+v", p)
	}
	y := g[1]
	if y.State != store.Behind || y.Measured != store.Fail || y.Effort == nil || *y.Effort != 10 || y.DaysLeft == nil || *y.DaysLeft != 5 {
		t.Errorf("the yes-or-no claim = %+v", y)
	}
}
