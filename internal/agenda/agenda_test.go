package agenda

import (
	"strings"
	"testing"
	"time"

	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/store"
)

func at(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }

func testSet() *module.Set {
	return &module.Set{Modules: []module.Manifest{
		{Name: "identity", Profile: module.RatifiedRecord, Priority: 5, Onboarding: []string{"value", "preference"},
			Kinds: map[string]module.Kind{
				"value":      {Fields: []string{"statement"}, FreshnessDays: 365, Interview: "Still one of the things you weigh decisions against?", First: "What do you weigh decisions against?"},
				"preference": {Fields: []string{"statement"}, FreshnessDays: 120, Interview: "Still how you want to be worked with?", First: "How do you want to be worked with?"},
			}},
		{Name: "telos", Profile: module.RatifiedRecord, Priority: 10, Onboarding: []string{"goal"},
			Kinds: map[string]module.Kind{
				"goal":    {Fields: []string{"id", "title", "ideal", "by"}, FreshnessDays: 90, Interview: "Still right? Progress since {reviewed}?", First: "What are you working toward?"},
				"current": {Fields: []string{"dimension", "text"}, FreshnessDays: 90, Interview: "Where are you now on {dimension}?"},
			}},
		{Name: "memory", Profile: module.WorkingMemory, Priority: 20, Layout: "free",
			Kinds: map[string]module.Kind{"note": {}, "preference": {}}},
	}}
}

func rec(path, mod, kind string, fields map[string]string, updated, reviewed string, snoozes int) store.Stored {
	s := store.Stored{Path: path}
	s.Module, s.Kind, s.Fields, s.Name = mod, kind, fields, path
	s.ID = fields["id"] // as ParseRecord fills it from the id key
	s.Updated, s.Snoozes = at(updated), snoozes
	if reviewed != "" {
		s.Reviewed = at(reviewed)
	}
	return s
}

func TestStaleRecordsComeFirstByModulePriorityThenAge(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	records := []store.Stored{
		rec("telos/goal/g1.md", "telos", "goal", map[string]string{"id": "G1"}, "2026-05-01T00:00:00Z", "2026-05-01T00:00:00Z", 0), // 153 days > 90: stale
		rec("identity/value/v1.md", "identity", "value", nil, "2026-09-01T00:00:00Z", "2026-09-01T00:00:00Z", 0),                   // 30 days < 365: fresh
		rec("identity/value/v2.md", "identity", "value", nil, "2025-01-01T00:00:00Z", "2025-01-01T00:00:00Z", 2),                   // stale, snoozed twice
		rec("identity/value/v3.md", "identity", "value", nil, "2026-09-20T00:00:00Z", "", 0),                                       // never reviewed: stale, first among identity
		rec("memory/n.md", "memory", "note", nil, "2020-01-01T00:00:00Z", "", 0),                                                   // working-memory: never on the agenda
	}
	items := Compute(testSet(), records, now)
	got := make([]string, 0, len(items))
	for _, it := range items {
		if it.Reason == Stale {
			got = append(got, it.Path)
		}
	}
	want := "identity/value/v3.md,identity/value/v2.md,telos/goal/g1.md"
	if strings.Join(got, ",") != want {
		t.Errorf("stale order = %v, want %s", got, want)
	}
	if items[1].Snoozes != 2 {
		t.Errorf("snoozes must travel with the item: %+v", items[1])
	}
}

func TestQuestionsAreRenderedFromTheKindsPrompt(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	records := []store.Stored{
		rec("telos/goal/g1.md", "telos", "goal", map[string]string{"id": "G1"}, "2026-05-01T00:00:00Z", "2026-05-01T00:00:00Z", 0),
		rec("telos/current/c.md", "telos", "current", map[string]string{"dimension": "fitness", "text": "ok"}, "2026-01-01T00:00:00Z", "", 0),
	}
	items := Compute(testSet(), records, now)
	byPath := map[string]Item{}
	for _, it := range items {
		byPath[it.Path] = it
	}
	if q := byPath["telos/goal/g1.md"].Question; q != "Still right? Progress since 2026-05-01?" {
		t.Errorf("goal question = %q", q)
	}
	if q := byPath["telos/current/c.md"].Question; q != "Where are you now on fitness?" {
		t.Errorf("current question = %q", q)
	}
	if id := byPath["telos/goal/g1.md"].ID; id != "G1" {
		t.Errorf("id = %q", id)
	}
	if id := byPath["telos/goal/g1.md"].Fields["id"]; id != "G1" {
		t.Errorf("Fields[id] = %q, want it filled from the record", id)
	}
}

func TestARevisionSinceTheLastReviewIsNamed(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	r := rec("telos/goal/g1.md", "telos", "goal", nil, "2026-09-04T00:00:00Z", "2026-05-01T00:00:00Z", 0)
	items := Compute(testSet(), []store.Stored{r}, now)
	if len(items) == 0 || items[0].Revision != "revised 2026-09-04, last reviewed 2026-05-01" {
		t.Errorf("items = %+v", items)
	}
	fresh := rec("telos/goal/g2.md", "telos", "goal", nil, "2026-05-01T00:00:00Z", "2026-05-01T00:00:00Z", 0)
	if items := Compute(testSet(), []store.Stored{fresh}, now); items[0].Revision != "" {
		t.Errorf("no revision when updated == reviewed: %q", items[0].Revision)
	}
}

func TestRetiredRecordsAreNeverAsked(t *testing.T) {
	r := rec("identity/value/v.md", "identity", "value", nil, "2020-01-01T00:00:00Z", "2020-01-01T00:00:00Z", 0)
	r.Retired = at("2026-01-01T00:00:00Z")
	for _, it := range Compute(testSet(), []store.Stored{r}, at("2026-10-01T00:00:00Z")) {
		if it.Path == r.Path {
			t.Errorf("retired record on the agenda: %+v", it)
		}
	}
}

func TestOnboardingGapsFollowStaleItemsInModuleOrder(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	records := []store.Stored{
		rec("identity/value/v.md", "identity", "value", nil, "2026-09-01T00:00:00Z", "2026-09-01T00:00:00Z", 0), // fresh; a value exists
	}
	items := Compute(testSet(), records, now)
	var got []string
	for _, it := range items {
		if it.Reason == Empty {
			got = append(got, it.Module+"/"+it.Kind+": "+it.Question)
		}
	}
	want := "identity/preference: How do you want to be worked with?,telos/goal: What are you working toward?"
	if strings.Join(got, ",") != want {
		t.Errorf("onboarding = %v", got)
	}
}

func TestAnEmptyRecordAsksTheFirstQuestionFirst(t *testing.T) {
	top, ok := Top(Compute(testSet(), nil, at("2026-10-01T00:00:00Z")))
	if !ok || top.Reason != Empty || top.Module != "identity" || top.Kind != "value" {
		t.Errorf("top = %+v %v", top, ok)
	}
	if _, ok := Top(nil); ok {
		t.Error("nothing to ask is a valid state")
	}
}

// §7: a preference the model wrote under memory is reviewed like an identity
// preference. It is the only kind that crosses.
func TestAWorkingMemoryPreferenceIsAskedUnderIdentitysRule(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	records := []store.Stored{
		// Updated well past identity's 120-day freshness for preference, so it
		// is stale under the new never-reviewed rule for a crossing record
		// (age = now - Updated, not the native never-reviewed sentinel).
		rec("personal/terse.md", "memory", "preference", nil, "2026-01-01T00:00:00Z", "", 0),
		rec("personal/note.md", "memory", "note", nil, "2020-01-01T00:00:00Z", "", 0),
	}
	items := Compute(testSet(), records, now)
	var sawPref, sawNote bool
	for _, it := range items {
		sawPref = sawPref || (it.Path == "personal/terse.md" && it.Question == "Still how you want to be worked with?")
		sawNote = sawNote || it.Path == "personal/note.md"
	}
	if !sawPref || sawNote {
		t.Errorf("pref=%v note=%v: %+v", sawPref, sawNote, items)
	}
	for _, it := range items {
		if it.Reason == Empty && it.Module == "identity" && it.Kind == "preference" {
			t.Errorf("a memory/preference on file satisfies identity's onboarding for preference; asked anyway: %+v", it)
		}
	}
}

// C: a never-reviewed crossing record is not asked at once — a fresh
// model-written preference has to sit for identity's freshness period first.
func TestACrossingRecordNeverReviewedIsNotStaleUntilPastFreshness(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	records := []store.Stored{
		rec("personal/terse.md", "memory", "preference", nil, "2026-09-30T00:00:00Z", "", 0), // updated yesterday
	}
	items := Compute(testSet(), records, now)
	for _, it := range items {
		if it.Path == "personal/terse.md" {
			t.Errorf("a fresh, never-reviewed crossing record must not be on the agenda yet: %+v", it)
		}
	}
}

// C: once a crossing record is stale, it sorts after every native ratified
// item, never mixed in by priority.
func TestACrossingRecordSortsAfterEveryNativeRatifiedItem(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	records := []store.Stored{
		rec("telos/goal/g1.md", "telos", "goal", map[string]string{"id": "G1"}, "2026-05-01T00:00:00Z", "2026-05-01T00:00:00Z", 0), // native, stale
		rec("personal/terse.md", "memory", "preference", nil, "2026-03-15T00:00:00Z", "", 0),                                       // crossing, never reviewed, ~200 days old: stale
	}
	items := Compute(testSet(), records, now)
	var got []string
	for _, it := range items {
		if it.Reason == Stale {
			got = append(got, it.Path)
		}
	}
	if want := "telos/goal/g1.md,personal/terse.md"; strings.Join(got, ",") != want {
		t.Errorf("order = %v, want %s", got, want)
	}
}

// C: the tier applies even once the crossing record has been reviewed before
// and has gone stale again — it still sorts after native items.
func TestAReviewedThenStaleCrossingRecordAlsoSortsAfterNativeItems(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	records := []store.Stored{
		rec("identity/value/v.md", "identity", "value", nil, "2025-01-01T00:00:00Z", "2025-01-01T00:00:00Z", 0),  // native, stale
		rec("personal/terse.md", "memory", "preference", nil, "2025-01-01T00:00:00Z", "2025-01-01T00:00:00Z", 0), // crossing, reviewed long ago, stale again
	}
	items := Compute(testSet(), records, now)
	var got []string
	for _, it := range items {
		if it.Reason == Stale {
			got = append(got, it.Path)
		}
	}
	if want := "identity/value/v.md,personal/terse.md"; strings.Join(got, ",") != want {
		t.Errorf("order = %v, want %s", got, want)
	}
}

func TestAPreModuleRecordIsReportedNotSkipped(t *testing.T) {
	r := store.Stored{Path: "personal/old.md"}
	r.LegacyType = "feedback"
	items := Compute(testSet(), []store.Stored{r}, at("2026-10-01T00:00:00Z"))
	if len(items) == 0 || items[0].Reason != Stale || !strings.Contains(items[0].Question, "migrat") {
		t.Errorf("a pre-module file should surface as a stale item saying to migrate: %+v", items)
	}
}

// A malformed reviewed/retired/snoozes key must not read as "never
// reviewed": that would render {reviewed} as a lie, and Review refuses every
// verdict on a Malformed record (the same check Write uses), so the item
// could never be cleared from inside the system if it were asked under the
// kind's normal prompt.
func TestAMalformedRecordSurfacesAsFixByHandNotNeverReviewed(t *testing.T) {
	r := rec("identity/value/bad.md", "identity", "value", nil, "2026-09-01T00:00:00Z", "2026-09-01T00:00:00Z", 0)
	r.Malformed = []string{"reviewed"}
	items := Compute(testSet(), []store.Stored{r}, at("2026-10-01T00:00:00Z"))
	if len(items) == 0 || items[0].Reason != Stale {
		t.Fatalf("items = %+v", items)
	}
	q := items[0].Question
	if !strings.Contains(q, "malformed") || !strings.Contains(q, "by hand") {
		t.Errorf("question = %q, want it to name the malformed key and say fix by hand", q)
	}
	if q == "Still one of the things you weigh decisions against?" {
		t.Errorf("a malformed record must not be asked under the kind's normal interview prompt: %q", q)
	}
}
