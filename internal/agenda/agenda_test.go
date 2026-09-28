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
				"value":      {Fields: []string{"statement"}, FreshnessDays: 365, Interview: "Still one of the things you weigh decisions against?", Draft: "Is this the value as you would put it?", First: "What do you weigh decisions against?"},
				"preference": {Fields: []string{"statement"}, FreshnessDays: 120, Interview: "Still how you want to be worked with?", First: "How do you want to be worked with?"},
			}},
		{Name: "telos", Profile: module.RatifiedRecord, Priority: 10, Onboarding: []string{"goal"},
			Kinds: map[string]module.Kind{
				"goal":     {Fields: []string{"id", "title", "ideal", "by"}, FreshnessDays: 90, Interview: "Still right? Progress since {reviewed}?", First: "What are you working toward?"},
				"current":  {Fields: []string{"dimension", "text"}, FreshnessDays: 90, Interview: "Where are you now on {dimension}?"},
				"decision": {Fields: []string{"revisit"}, DueField: "revisit", Interview: "The revisit date passed. Did the prediction hold?"},
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
		rec("identity/value/v3.md", "identity", "value", nil, "2026-09-20T00:00:00Z", "", 0),                                       // never reviewed: a draft, leads everything
		rec("memory/n.md", "memory", "note", nil, "2020-01-01T00:00:00Z", "", 0),                                                   // working-memory: never on the agenda
	}
	items := Compute(testSet(), records, now)
	var got []string
	var snoozes int
	for _, it := range items {
		if it.Reason == Draft || it.Reason == Stale {
			got = append(got, string(it.Reason)+":"+it.Path)
		}
		if it.Path == "identity/value/v2.md" {
			snoozes = it.Snoozes
		}
	}
	want := "draft:identity/value/v3.md,stale:identity/value/v2.md,stale:telos/goal/g1.md"
	if strings.Join(got, ",") != want {
		t.Errorf("order = %v, want %s", got, want)
	}
	if snoozes != 2 {
		t.Errorf("snoozes must travel with the item: %d", snoozes)
	}
}

func TestQuestionsAreRenderedFromTheKindsPrompt(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	records := []store.Stored{
		rec("telos/goal/g1.md", "telos", "goal", map[string]string{"id": "G1"}, "2026-05-01T00:00:00Z", "2026-05-01T00:00:00Z", 0),
		rec("telos/current/c.md", "telos", "current", map[string]string{"dimension": "fitness", "text": "ok"}, "2026-01-01T00:00:00Z", "2025-01-01T00:00:00Z", 0), // reviewed well past its 90-day freshness: stays a stale-question test
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
		if it.Reason == Onboarding {
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
	if !ok || top.Reason != Onboarding || top.Module != "identity" || top.Kind != "value" {
		t.Errorf("top = %+v %v", top, ok)
	}
	if _, ok := Top(nil); ok {
		t.Error("nothing to ask is a valid state")
	}
}

// §7: a preference the model wrote under memory is reviewed like an identity
// preference. It is the only kind that crosses, and never reviewed, it is a
// draft at once, asked under identity's rule.
func TestAWorkingMemoryPreferenceIsAskedUnderIdentitysRule(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	records := []store.Stored{
		rec("personal/terse.md", "memory", "preference", nil, "2026-01-01T00:00:00Z", "", 0),
		rec("personal/note.md", "memory", "note", nil, "2020-01-01T00:00:00Z", "", 0),
	}
	items := Compute(testSet(), records, now)
	var sawPref, sawNote bool
	for _, it := range items {
		sawPref = sawPref || (it.Path == "personal/terse.md" && it.Reason == Draft && it.Question == DefaultDraft)
		sawNote = sawNote || it.Path == "personal/note.md"
	}
	if !sawPref || sawNote {
		t.Errorf("pref=%v note=%v: %+v", sawPref, sawNote, items)
	}
	for _, it := range items {
		if it.Reason == Onboarding && it.Module == "identity" && it.Kind == "preference" {
			t.Errorf("a memory/preference on file satisfies identity's onboarding for preference; asked anyway: %+v", it)
		}
	}
}

// C: once a crossing record is reviewed before and has gone stale again, it
// still sorts after a native ratified item — because it is a preference, and
// preferences sort last, not because it crosses.
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

// AA, AM: a record never reviewed is a draft, asked with the kind's draft
// question, ahead of every stale record — an approval left over from an
// earlier conversation comes before a question about age.
func TestDraftsComeBeforeStaleRecordsAndAskTheDraftQuestion(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	records := []store.Stored{
		rec("identity/value/v2.md", "identity", "value", nil, "2025-01-01T00:00:00Z", "2025-01-01T00:00:00Z", 0), // stale
		rec("telos/goal/g1.md", "telos", "goal", map[string]string{"id": "G1"}, "2026-09-30T00:00:00Z", "", 0),   // draft, written yesterday
		rec("identity/value/v3.md", "identity", "value", nil, "2026-09-20T00:00:00Z", "", 0),                     // draft, older
	}
	items := Compute(testSet(), records, now)
	var got []string
	for _, it := range items {
		got = append(got, string(it.Reason)+":"+it.Path)
	}
	want := "draft:identity/value/v3.md,draft:telos/goal/g1.md,stale:identity/value/v2.md"
	if strings.Join(got[:3], ",") != want {
		t.Errorf("order = %v, want %s", got, want)
	}
	if q := items[0].Question; q != "Is this the value as you would put it?" {
		t.Errorf("draft question = %q", q)
	}
	if q := items[1].Question; q != DefaultDraft {
		t.Errorf("a kind with no draft question is asked %q, got %q", DefaultDraft, q)
	}
}

// AM: a preference the model wrote is a draft at once, and preferences sort
// last within each reason, native and crossing alike.
func TestPreferencesSortLastWithinEachReasonAndACrossingOneIsADraftAtOnce(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	records := []store.Stored{
		rec("personal/terse.md", "memory", "preference", nil, "2026-09-30T00:00:00Z", "", 0),                                       // crossing draft, written yesterday
		rec("identity/preference/p.md", "identity", "preference", nil, "2026-09-01T00:00:00Z", "", 0),                              // native draft
		rec("telos/goal/g1.md", "telos", "goal", map[string]string{"id": "G1"}, "2026-09-29T00:00:00Z", "", 0),                     // draft, newer than both
		rec("identity/preference/old.md", "identity", "preference", nil, "2025-01-01T00:00:00Z", "2025-01-01T00:00:00Z", 0),        // stale preference
		rec("telos/goal/g2.md", "telos", "goal", map[string]string{"id": "G2"}, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", 0), // stale goal, lower priority module
	}
	items := Compute(testSet(), records, now)
	var got []string
	for _, it := range items {
		if it.Reason == Draft || it.Reason == Stale {
			got = append(got, it.Path)
		}
	}
	want := "telos/goal/g1.md,identity/preference/p.md,personal/terse.md,telos/goal/g2.md,identity/preference/old.md"
	if strings.Join(got, ",") != want {
		t.Errorf("order = %v, want %s", got, want)
	}
}

// AK: a stale record whose kind declares no interview prompt is asked
// "Is this still right?", so no module leaves the first line empty.
func TestAStaleRecordWithNoInterviewPromptIsAskedTheDefault(t *testing.T) {
	r := rec("telos/current/c.md", "telos", "current", map[string]string{"dimension": "x", "text": "y"}, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", 0)
	set := testSet()
	k := set.Modules[1].Kinds["current"]
	k.Interview = ""
	set.Modules[1].Kinds["current"] = k
	items := Compute(set, []store.Stored{r}, at("2026-10-01T00:00:00Z"))
	if len(items) == 0 || items[0].Question != DefaultInterview {
		t.Errorf("items = %+v", items)
	}
}

// AI: a kind with a due_field is due once the date has passed and the record
// has not been reviewed since; it is stale, asked with the interview question.
func TestADueFieldMakesARecordStaleOnceTheDatePasses(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	past := rec("telos/decision/d1.md", "telos", "decision", map[string]string{"revisit": "2026-09-15"}, "2026-06-01T00:00:00Z", "2026-06-01T00:00:00Z", 0)
	future := rec("telos/decision/d2.md", "telos", "decision", map[string]string{"revisit": "2027-01-01"}, "2026-06-01T00:00:00Z", "2026-06-01T00:00:00Z", 0)
	reviewedAfter := rec("telos/decision/d3.md", "telos", "decision", map[string]string{"revisit": "2026-09-15"}, "2026-06-01T00:00:00Z", "2026-09-20T00:00:00Z", 0)
	items := Compute(testSet(), []store.Stored{past, future, reviewedAfter}, now)
	var got []string
	for _, it := range items {
		if it.Reason == Stale {
			got = append(got, it.Path+":"+it.Question)
		}
	}
	if want := "telos/decision/d1.md:The revisit date passed. Did the prediction hold?"; strings.Join(got, ",") != want {
		t.Errorf("stale = %v, want %s", got, want)
	}
}

func TestOnboardingIsTheReasonsName(t *testing.T) {
	top, _ := Top(Compute(testSet(), nil, at("2026-10-01T00:00:00Z")))
	if top.Reason != Onboarding || string(top.Reason) != "onboarding" {
		t.Errorf("reason = %q", top.Reason)
	}
}

// The M1 ledger: Item.Fields aliased the record's live map, so a caller editing
// an item could edit the record it was computed from.
func TestAnItemsFieldsAreACopy(t *testing.T) {
	r := rec("identity/value/v.md", "identity", "value", map[string]string{"statement": "s"}, "2025-01-01T00:00:00Z", "2025-01-01T00:00:00Z", 0)
	items := Compute(testSet(), []store.Stored{r}, at("2026-10-01T00:00:00Z"))
	items[0].Fields["statement"] = "changed"
	if r.Fields["statement"] != "s" {
		t.Error("the item's fields alias the record's map")
	}
}
