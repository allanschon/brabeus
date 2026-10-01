package agenda

import (
	"encoding/json"
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
				"goal":     {Fields: []string{"id", "title", "ideal", "by"}, Optional: []string{"claims", "serves"}, FreshnessDays: 90, Interview: "Still right? Progress since {reviewed}?", First: "What are you working toward?"},
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
	items := Compute(testSet(), records, nil, now)
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
	items := Compute(testSet(), records, nil, now)
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
	items := Compute(testSet(), []store.Stored{r}, nil, now)
	if len(items) == 0 || items[0].Revision != "revised 2026-09-04, last reviewed 2026-05-01" {
		t.Errorf("items = %+v", items)
	}
	fresh := rec("telos/goal/g2.md", "telos", "goal", nil, "2026-05-01T00:00:00Z", "2026-05-01T00:00:00Z", 0)
	if items := Compute(testSet(), []store.Stored{fresh}, nil, now); items[0].Revision != "" {
		t.Errorf("no revision when updated == reviewed: %q", items[0].Revision)
	}
}

// When Records() (M2) has already filled Stored.Revision with the
// field-level line read from git, Compute must pass it through verbatim
// rather than recompute the M1 two-stamp wording from Updated/Reviewed —
// even though those stamps are present and would otherwise produce a
// different string.
func TestAStoredRevisionIsPreferredOverTheStampFallback(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	r := rec("telos/goal/g1.md", "telos", "goal", nil, "2026-09-04T00:00:00Z", "2026-05-01T00:00:00Z", 0)
	r.Revision = "by moved from 2026-10-01 to 2026-12-01 on 2026-09-04"
	items := Compute(testSet(), []store.Stored{r}, nil, now)
	if len(items) == 0 || items[0].Revision != r.Revision {
		t.Errorf("items[0].Revision = %q, want the stored line %q", items[0].Revision, r.Revision)
	}
}

func TestRetiredRecordsAreNeverAsked(t *testing.T) {
	r := rec("identity/value/v.md", "identity", "value", nil, "2020-01-01T00:00:00Z", "2020-01-01T00:00:00Z", 0)
	r.Retired = at("2026-01-01T00:00:00Z")
	for _, it := range Compute(testSet(), []store.Stored{r}, nil, at("2026-10-01T00:00:00Z")) {
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
	items := Compute(testSet(), records, nil, now)
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
	top, ok := Top(Compute(testSet(), nil, nil, at("2026-10-01T00:00:00Z")))
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
	items := Compute(testSet(), records, nil, now)
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
	items := Compute(testSet(), records, nil, now)
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
	items := Compute(testSet(), []store.Stored{r}, nil, at("2026-10-01T00:00:00Z"))
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
	items := Compute(testSet(), []store.Stored{r}, nil, at("2026-10-01T00:00:00Z"))
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
	items := Compute(testSet(), records, nil, now)
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
	items := Compute(testSet(), records, nil, now)
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
	items := Compute(set, []store.Stored{r}, nil, at("2026-10-01T00:00:00Z"))
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
	items := Compute(testSet(), []store.Stored{past, future, reviewedAfter}, nil, now)
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
	top, _ := Top(Compute(testSet(), nil, nil, at("2026-10-01T00:00:00Z")))
	if top.Reason != Onboarding || string(top.Reason) != "onboarding" {
		t.Errorf("reason = %q", top.Reason)
	}
}

// Item.Fields once aliased the record's live map, so a caller editing an item
// could edit the record it was computed from.
func TestAnItemsFieldsAreACopy(t *testing.T) {
	r := rec("identity/value/v.md", "identity", "value", map[string]string{"statement": "s"}, "2025-01-01T00:00:00Z", "2025-01-01T00:00:00Z", 0)
	items := Compute(testSet(), []store.Stored{r}, nil, at("2026-10-01T00:00:00Z"))
	items[0].Fields["statement"] = "changed"
	if r.Fields["statement"] != "s" {
		t.Error("the item's fields alias the record's map")
	}
}

func result(idx int, text string, state store.ClaimState, since string, detail string) store.ClaimResult {
	return store.ClaimResult{Index: idx, Text: text, Adapter: "tracker", State: state, Detail: detail, Since: at(since), Recorded: at(since)}
}

// AM: failed claims head the agenda, nearest goal date first, then by how
// long the claim has been failing; no-evidence is never on it; a goal whose
// by date does not parse sorts last among fails rather than breaking them.
// The claims are standing: an end-state claim with its deadline ahead reads
// open, not fail (spec §8.1), so only a standing claim exercises the tier here.
func TestFailedClaimsHeadTheAgendaByTheGoalsDateThenByHowLongTheyHaveFailed(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	claims := "- text: \"three articles\"\n  standing: true\n  check: {adapter: tracker, min: 3}\n- text: \"a commit a fortnight\"\n  standing: true\n  check: {adapter: forge, repo: a/b, since: -14d, min: 1}"
	g := func(path, id, by string) store.Stored {
		return rec(path, "telos", "goal", map[string]string{"id": id, "title": "t", "by": by, "claims": claims}, "2026-09-01T00:00:00Z", "2026-09-01T00:00:00Z", 0)
	}
	records := []store.Stored{
		g("telos/goal/late.md", "G1", "2027-06-01"),
		g("telos/goal/soon.md", "G2", "2026-11-01"),
		g("telos/goal/soon2.md", "G3", "2026-11-01"),
		g("telos/goal/odd.md", "G4", "someday"),
		rec("identity/value/v.md", "identity", "value", nil, "2025-01-01T00:00:00Z", "2025-01-01T00:00:00Z", 0), // stale
	}
	results := map[string][]store.ClaimResult{
		"telos/goal/late.md":  {result(0, "three articles", store.Fail, "2026-09-01T00:00:00Z", "1 found")},
		"telos/goal/soon.md":  {result(0, "three articles", store.Fail, "2026-09-20T00:00:00Z", "2 found"), result(1, "a commit a fortnight", store.NoEvidence, "2026-09-20T00:00:00Z", "credential refused")},
		"telos/goal/soon2.md": {result(1, "a commit a fortnight", store.Fail, "2026-09-10T00:00:00Z", "0 found")},
		"telos/goal/odd.md":   {result(0, "three articles", store.Fail, "2026-01-01T00:00:00Z", "0 found")},
	}
	items := Compute(testSet(), records, results, now)
	var got []string
	for _, it := range items {
		if it.Reason == Fail {
			got = append(got, it.ID+":"+it.ClaimText)
		}
	}
	want := "G3:a commit a fortnight,G2:three articles,G1:three articles,G4:three articles"
	if strings.Join(got, ",") != want {
		t.Errorf("fail order = %v, want %s", got, want)
	}
	if len(items) < 2 || items[0].Reason != Fail || items[len(items)-1].Reason == Fail {
		t.Fatalf("fails come first and stale after: %+v", items)
	}
	if q := items[1].Question; q != `the claim "three articles" failed on 2026-09-20 (2 found). Still right? Progress since 2026-09-01?` {
		t.Errorf("question = %q", q)
	}
	for _, it := range items {
		if it.Reason == Fail && strings.Contains(it.ClaimText, "fortnight") && it.ID == "G2" {
			t.Error("a no-evidence claim is a fault, not an agenda item")
		}
	}
}

// A result left over from a claim the person has since reworded matches
// nothing on the goal and is ignored, not shown against the new wording.
func TestAResultForAClaimThatNoLongerExistsIsIgnored(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	r := rec("telos/goal/g.md", "telos", "goal", map[string]string{"id": "G1", "title": "t", "by": "2026-11-01", "claims": "- text: \"four articles\"\n  standing: true\n  check: {adapter: tracker, min: 4}"}, "2026-09-25T00:00:00Z", "2026-09-25T00:00:00Z", 0)
	results := map[string][]store.ClaimResult{"telos/goal/g.md": {result(0, "three articles", store.Fail, "2026-09-20T00:00:00Z", "")}}
	for _, it := range Compute(testSet(), []store.Stored{r}, results, now) {
		if it.Reason == Fail {
			t.Errorf("stale result surfaced: %+v", it)
		}
	}
}

// A goal never reviewed is a draft, and a failed claim on it still heads the
// agenda: the same goal appears twice, once per reason, the fail first. The
// claim's position travels with the item even when it is the first claim, 0.
func TestADraftGoalWithAFailedClaimIsAskedAboutTheFailFirst(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	r := rec("telos/goal/g.md", "telos", "goal", map[string]string{"id": "G1", "title": "t", "by": "2026-11-01", "claims": "- text: \"three articles\"\n  standing: true\n  check: {adapter: tracker, min: 3}"}, "2026-09-25T00:00:00Z", "", 0)
	results := map[string][]store.ClaimResult{"telos/goal/g.md": {result(0, "three articles", store.Fail, "2026-09-20T00:00:00Z", "2 found")}}
	items := Compute(testSet(), []store.Stored{r}, results, now)
	if len(items) < 2 || items[0].Reason != Fail || items[1].Reason != Draft || items[0].Path != items[1].Path {
		t.Fatalf("items = %+v", items)
	}
	b, _ := json.Marshal(items[0])
	if !strings.Contains(string(b), `"claim_index":0`) || !strings.Contains(string(b), `"claim":"three articles"`) {
		t.Errorf("the first claim's index must be on the wire: %s", b)
	}
	if b, _ := json.Marshal(items[1]); strings.Contains(string(b), "claim_index") {
		t.Errorf("an item that is not about a claim carries no index: %s", b)
	}
	// The goal has never been reviewed, so the fail asks the draft question,
	// not "progress since never".
	if want := `the claim "three articles" failed on 2026-09-20 (2 found). Is this right as written?`; items[0].Question != want {
		t.Errorf("question = %q, want %q", items[0].Question, want)
	}
}

// A confirmed goal's fail is followed by its interview question, with the date
// of its last review.
func TestAConfirmedGoalsFailAsksWhetherItIsStillRight(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	r := rec("telos/goal/g.md", "telos", "goal", map[string]string{"id": "G1", "title": "t", "by": "2026-11-01", "claims": "- text: \"three articles\"\n  standing: true\n  check: {adapter: tracker, min: 3}"}, "2026-09-25T00:00:00Z", "2026-09-26T00:00:00Z", 0)
	results := map[string][]store.ClaimResult{"telos/goal/g.md": {result(0, "three articles", store.Fail, "2026-09-27T00:00:00Z", "2 found")}}
	items := Compute(testSet(), []store.Stored{r}, results, now)
	if len(items) == 0 || items[0].Reason != Fail {
		t.Fatalf("items = %+v", items)
	}
	if want := `the claim "three articles" failed on 2026-09-27 (2 found). Still right? Progress since 2026-09-26?`; items[0].Question != want {
		t.Errorf("question = %q, want %q", items[0].Question, want)
	}
}

// goalWith is a confirmed or draft goal carrying one claims block.
func goalWith(path, id, by, claims, reviewed string) store.Stored {
	return rec(path, "telos", "goal", map[string]string{"id": id, "title": "t", "by": by, "claims": claims}, "2026-09-01T00:00:00Z", reviewed, 0)
}

func reasons(items []Item) string {
	var out []string
	for _, it := range items {
		out = append(out, string(it.Reason)+":"+it.ID)
	}
	return strings.Join(out, ",")
}

// An open claim is on schedule: nothing to ask about it (spec §8.1, §9).
func TestAnOpenClaimIsNeverOnTheAgenda(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	r := goalWith("telos/goal/g.md", "G1", "2026-10-31", "- text: \"ten articles\"\n  check: {adapter: tracker, min: 10, since: 2026-09-01}", "2026-08-01T00:00:00Z")
	r.Updated = at("2026-08-01T00:00:00Z")
	counted := map[string][]store.ClaimResult{"telos/goal/g.md": {{Index: 0, Text: "ten articles", State: store.Pass, Count: ip(5), Target: ip(10)}}}
	for _, it := range Compute(testSet(), []store.Stored{r}, counted, now) {
		if it.ClaimText != "" {
			t.Errorf("an open claim reached the agenda: %+v", it)
		}
	}
	// Reviewed 2026-08-01 is 61 days ago, under the goal kind's 90: not stale either.
	r2 := goalWith("telos/goal/h.md", "G2", "2026-10-31", "- text: \"ten articles\"\n  check: {adapter: tracker, min: 10, since: 2026-09-01}", "2026-08-01T00:00:00Z")
	res := map[string][]store.ClaimResult{"telos/goal/h.md": {{Index: 0, Text: "ten articles", State: store.Fail, Count: ip(9), Target: ip(10)}}}
	for _, it := range Compute(testSet(), []store.Stored{r2}, res, now) {
		if it.ClaimText != "" {
			t.Errorf("an open claim with a stored fail reached the agenda: %+v", it)
		}
	}
}

func ip(n int) *int { return &n }

const pacedBehind = "- text: \"ten articles\"\n  check: {adapter: tracker, min: 10, since: 2026-09-01}"

func TestBehindComesAfterFailAndBeforeDrafts(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	records := []store.Stored{
		goalWith("telos/goal/behind.md", "G2", "2026-10-31", pacedBehind, "2026-08-01T00:00:00Z"),
		goalWith("telos/goal/fail.md", "G1", "2026-12-01", "- text: \"keep going\"\n  standing: true\n  check: {adapter: tracker, min: 1}", "2026-08-01T00:00:00Z"),
		rec("identity/value/v3.md", "identity", "value", map[string]string{"id": "V3"}, "2026-09-20T00:00:00Z", "", 0),
	}
	results := map[string][]store.ClaimResult{"telos/goal/fail.md": {result(0, "keep going", store.Fail, "2026-09-20T00:00:00Z", "")}}
	got := reasons(Compute(testSet(), records, results, now))
	if want := "fail:G1,behind:G2,draft:V3"; !strings.HasPrefix(got, want) {
		t.Errorf("order = %s, want it to begin %s", got, want)
	}
}

func TestFailIsOrderedByTheClaimsOwnDeadline(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	early := "- text: \"first\"\n  by: 2026-09-10\n  check: {adapter: tracker, min: 1}"
	late := "- text: \"second\"\n  by: 2026-09-20\n  check: {adapter: tracker, min: 1}"
	records := []store.Stored{
		goalWith("telos/goal/a.md", "G1", "2026-09-30", late, "2026-08-01T00:00:00Z"), // goal date earlier, claim date later
		goalWith("telos/goal/b.md", "G2", "2027-01-01", early, "2026-08-01T00:00:00Z"),
	}
	results := map[string][]store.ClaimResult{
		"telos/goal/a.md": {result(0, "second", store.Fail, "2026-09-25T00:00:00Z", "")},
		"telos/goal/b.md": {result(0, "first", store.Fail, "2026-09-25T00:00:00Z", "")},
	}
	if got := reasons(Compute(testSet(), records, results, now)); !strings.HasPrefix(got, "fail:G2,fail:G1") {
		t.Errorf("order = %s, want the claim due 2026-09-10 (G2) first", got)
	}
}

func TestABehindClaimOnAGoalReviewedThisWeekWaitsBelowStale(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	staleValue := rec("identity/value/v.md", "identity", "value", map[string]string{"id": "V"}, "2025-01-01T00:00:00Z", "2025-01-01T00:00:00Z", 0)
	reviewed := goalWith("telos/goal/g.md", "G1", "2026-10-31", pacedBehind, "2026-09-28T00:00:00Z")
	if got := reasons(Compute(testSet(), []store.Stored{reviewed, staleValue}, nil, now)); !strings.HasPrefix(got, "stale:V,behind:G1") {
		t.Errorf("reviewed 3 days ago: order = %s, want stale:V,behind:G1", got)
	}
	snoozed := goalWith("telos/goal/s.md", "G2", "2026-10-31", pacedBehind, "2026-07-15T00:00:00Z") // 78 days: not stale, but long past a week
	snoozed.Snoozed = at("2026-09-29T00:00:00Z")
	if got := reasons(Compute(testSet(), []store.Stored{snoozed, staleValue}, nil, now)); !strings.HasPrefix(got, "stale:V,behind:G2") {
		t.Errorf("snoozed 2 days ago: order = %s, want stale:V,behind:G2", got)
	}
	// Nothing else to ask: the deferred item leads, and is the top item.
	fresh := rec("identity/value/v.md", "identity", "value", map[string]string{"id": "V"}, "2026-09-01T00:00:00Z", "2026-09-01T00:00:00Z", 0)
	pref := rec("identity/preference/p.md", "identity", "preference", map[string]string{"id": "P"}, "2026-09-01T00:00:00Z", "2026-09-01T00:00:00Z", 0)
	items := Compute(testSet(), []store.Stored{reviewed, fresh, pref}, nil, now)
	if top, ok := Top(items); !ok || top.Reason != Behind || top.ID != "G1" {
		t.Errorf("top = %+v, want the deferred behind item", top)
	}
	// Eight days ago is outside the week: it is no longer deferred.
	old := goalWith("telos/goal/o.md", "G3", "2026-10-31", pacedBehind, "2026-09-23T00:00:00Z")
	if got := reasons(Compute(testSet(), []store.Stored{old, staleValue}, nil, now)); !strings.HasPrefix(got, "behind:G3,stale:V") {
		t.Errorf("reviewed 8 days ago: order = %s, want behind first", got)
	}
}

func TestTheQuestionNamesWhatWasMeasured(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	followUp := "Still right? Progress since 2026-08-01?"
	ask := func(claims string, res []store.ClaimResult) string {
		r := goalWith("telos/goal/g.md", "G1", "2026-10-31", claims, "2026-08-01T00:00:00Z")
		items := Compute(testSet(), []store.Stored{r}, map[string][]store.ClaimResult{"telos/goal/g.md": res}, now)
		if len(items) == 0 || items[0].ClaimText == "" {
			t.Fatalf("no claim item for %q: %+v", claims, items)
		}
		return items[0].Question
	}
	if got, want := ask(pacedBehind, nil), `the claim "ten articles" is behind: 0 of 10 found, 4 expected by now, 31 days left. `+followUp; got != want {
		t.Errorf("paced = %q, want %q", got, want)
	}
	manual := "- text: \"ten chapters\"\n  check: {adapter: manual, of: 10, since: 2026-09-01}"
	if got, want := ask(manual, []store.ClaimResult{{Index: 0, Text: "ten chapters", State: store.Fail, Count: ip(1), Target: ip(10)}}), `the claim "ten chapters" is behind: 1 of 10 done, 4 expected by now, 31 days left. `+followUp; got != want {
		t.Errorf("manual = %q, want %q", got, want)
	}
	yesNo := "- text: \"ship it\"\n  by: 2026-10-11\n  effort: 20d\n  check: {adapter: manual}"
	if got, want := ask(yesNo, nil), `the claim "ship it" is behind: 11 days left, about 20 days of work. `+followUp; got != want {
		t.Errorf("yes-or-no = %q, want %q", got, want)
	}
	unreadable := map[string]string{
		"deadline": "- text: \"ship it\"\n  by: soon\n  check: {adapter: manual}",
		"since":    "- text: \"ten chapters\"\n  check: {adapter: manual, of: 10, since: yesterday}",
		"effort":   "- text: \"ship it\"\n  effort: lots\n  check: {adapter: manual}",
		"window":   "- text: \"ten chapters\"\n  check: {adapter: manual, of: 10, since: 2026-11-05}",
		"rolling":  "- text: \"a commit a fortnight\"\n  check: {adapter: forge, repo: side-project, since: -14d, min: 1}",
	}
	wants := map[string]string{
		"deadline": `the claim "ship it" has a deadline "soon" that could not be read; fix the date. `,
		"since":    `the claim "ten chapters" has a since "yesterday" that could not be read; fix the date. `,
		"effort":   `the claim "ship it" has an effort "lots" that could not be read; fix it. `,
		"window":   `the claim "ten chapters" starts counting on 2026-11-05, after its deadline 2026-10-31; fix the dates. `,
		"rolling":  `the claim "a commit a fortnight" counts a rolling window (since -14d) but does not say standing: true; should it hold all the time? `,
	}
	for k, claims := range unreadable {
		if got, want := ask(claims, nil), wants[k]+followUp; got != want {
			t.Errorf("unreadable %s = %q, want %q", k, got, want)
		}
	}
	endState := "- text: \"ship it\"\n  by: 2026-09-15\n  check: {adapter: tracker, min: 1}"
	if got, want := ask(endState, []store.ClaimResult{result(0, "ship it", store.Fail, "2026-09-20T00:00:00Z", "0 found")}), `the claim "ship it" was due by 2026-09-15 and is not met (0 found). `+followUp; got != want {
		t.Errorf("end-state fail = %q, want %q", got, want)
	}
	// A never-reviewed goal asks the draft question after the claim's.
	d := goalWith("telos/goal/g.md", "G1", "2026-10-31", pacedBehind, "")
	items := Compute(testSet(), []store.Stored{d}, nil, now)
	if want := `the claim "ten articles" is behind: 0 of 10 found, 4 expected by now, 31 days left. Is this right as written?`; len(items) == 0 || items[0].Question != want {
		t.Errorf("draft follow-up: %+v", items)
	}
}
