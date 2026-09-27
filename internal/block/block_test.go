package block

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allanschon/brabeus/internal/agenda"
	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/store"
)

func at(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }

// shippedSet loads the real manifests and templates from ../../modules, so the
// templates that ship are the ones under test.
func shippedSet(t *testing.T, names ...string) *module.Set {
	t.Helper()
	set, err := module.Load(filepath.Join("..", "..", "modules"), names)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func rec(path, mod, kind string, fields map[string]string, updated, reviewed string) store.Stored {
	s := store.Stored{Path: path}
	s.Module, s.Kind, s.Fields, s.Name, s.Description, s.Scope = mod, kind, fields, strings.TrimSuffix(filepath.Base(path), ".md"), "about "+path, "global"
	s.ID = fields["id"]
	s.Updated = at(updated)
	if reviewed != "" {
		s.Reviewed = at(reviewed)
	}
	return s
}

func TestNewParsesEveryShippedTemplateAndFailsOnAMissingOne(t *testing.T) {
	if _, err := New(shippedSet(t, "memory", "identity", "telos", "health", "finance")); err != nil {
		t.Fatalf("shipped templates must parse: %v", err)
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "telos"), 0o755)
	os.WriteFile(filepath.Join(dir, "telos", "module.json"), []byte(`{"name":"telos","version":1,"profile":"ratified-record","priority":10,"budget_bytes":600,"kinds":{"goal":{"fields":["id","title"]}},"summary":"summary.md.tmpl"}`), 0o644)
	set, err := module.Load(dir, []string{"telos"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(set); err == nil || !strings.Contains(err.Error(), "summary.md.tmpl") {
		t.Errorf("a missing template must fail New and name the file: %v", err)
	}
}

// Spec §14: a stale record surfaces as the first line without /interview.
func TestAStaleRecordIsTheFirstLine(t *testing.T) {
	set := shippedSet(t, "identity", "telos")
	r, err := New(set)
	if err != nil {
		t.Fatal(err)
	}
	now := at("2027-10-01T00:00:00Z")
	recs := []store.Stored{
		rec("identity/value/family.md", "identity", "value", map[string]string{"statement": "family time"}, "2026-09-01T00:00:00Z", "2026-09-01T00:00:00Z"), // 395 days: stale at 365
		rec("telos/goal/g1.md", "telos", "goal", map[string]string{"id": "G1", "title": "Ship the guide", "ideal": "published", "by": "2027-12-01"}, "2027-09-20T00:00:00Z", "2027-09-20T00:00:00Z"),
	}
	items := agenda.Compute(set, recs, now)
	text, faults := r.Render(items, recs, now, store.Visibility{})
	first, _, _ := strings.Cut(text, "\n")
	if !strings.HasPrefix(first, "agenda: [identity/value family]") || !strings.Contains(first, "Still one of the things") {
		t.Errorf("first line = %q", first)
	}
	if len(faults) != 0 || len(text) > Cap {
		t.Errorf("faults=%v len=%d", faults, len(text))
	}
	if !strings.Contains(text, "G1 Ship the guide by 2027-12-01") {
		t.Errorf("telos summary missing the goal:\n%s", text)
	}
}

func TestModulesRenderInPriorityOrderAndOnlyRatifiedOnes(t *testing.T) {
	set := shippedSet(t, "memory", "identity", "telos")
	r, _ := New(set)
	now := at("2026-10-01T00:00:00Z")
	recs := []store.Stored{
		rec("infra/n.md", "memory", "note", nil, "2026-09-01T00:00:00Z", ""),
		rec("telos/goal/g1.md", "telos", "goal", map[string]string{"id": "G1", "title": "T", "ideal": "i", "by": "b"}, "2026-09-20T00:00:00Z", "2026-09-20T00:00:00Z"),
		rec("identity/value/v.md", "identity", "value", map[string]string{"statement": "craft"}, "2026-09-20T00:00:00Z", "2026-09-20T00:00:00Z"),
	}
	text, _ := r.Render(agenda.Compute(set, recs, now), recs, now, store.Visibility{})
	i, tl := strings.Index(text, "\nidentity:"), strings.Index(text, "\ntelos:")
	if i < 0 || tl < 0 || i > tl {
		t.Errorf("identity (priority 5) must precede telos (10):\n%s", text)
	}
	if strings.Contains(text, "infra/n.md") || strings.Contains(text, "memory:") {
		t.Errorf("working-memory records never render:\n%s", text)
	}
}

func TestAReviewedCrossingPreferenceRendersUnderIdentityAndAnUnreviewedOneDoesNot(t *testing.T) {
	set := shippedSet(t, "memory", "identity")
	r, _ := New(set)
	now := at("2026-10-01T00:00:00Z")
	recs := []store.Stored{
		rec("personal/terse.md", "memory", "preference", nil, "2026-09-01T00:00:00Z", "2026-09-02T00:00:00Z"),
		rec("personal/draft.md", "memory", "preference", nil, "2026-09-01T00:00:00Z", ""),
	}
	recs[0].Description, recs[1].Description = "terse answers", "an unratified guess"
	text, _ := r.Render(nil, recs, now, store.Visibility{})
	if !strings.Contains(text, "terse answers") || strings.Contains(text, "unratified guess") {
		t.Errorf("crossing rule:\n%s", text)
	}
}

func TestAnOverBudgetModuleRendersOneLineAndAFault(t *testing.T) {
	set := shippedSet(t, "telos")
	r, _ := New(set)
	now := at("2026-10-01T00:00:00Z")
	var recs []store.Stored
	for i := 0; i < 40; i++ {
		recs = append(recs, rec("telos/goal/g.md", "telos", "goal", map[string]string{"id": "G", "title": strings.Repeat("long title ", 15), "ideal": "i", "by": "2027-01-01"}, "2026-09-20T00:00:00Z", "2026-09-20T00:00:00Z"))
	}
	text, faults := r.Render(nil, recs, now, store.Visibility{})
	if len(faults) != 1 || faults[0].Module != "telos" || faults[0].Budget != 600 || faults[0].Bytes <= 600 {
		t.Errorf("faults = %+v", faults)
	}
	if !strings.Contains(text, "telos: over budget (") || len(text) > Cap {
		t.Errorf("text:\n%s", text)
	}
}

func TestAHiddenModuleContributesNothingNotEvenItsName(t *testing.T) {
	set := shippedSet(t, "identity", "telos")
	r, _ := New(set)
	now := at("2026-10-01T00:00:00Z")
	recs := []store.Stored{rec("identity/value/v.md", "identity", "value", map[string]string{"statement": "craft"}, "2026-09-20T00:00:00Z", "2026-09-20T00:00:00Z")}
	text, faults := r.Render(nil, recs, now, store.Visibility{HideModules: map[string]bool{"identity": true}})
	if strings.Contains(text, "identity") || strings.Contains(text, "craft") || !strings.Contains(text, "telos:") || len(faults) != 0 {
		t.Errorf("hidden module leaked:\n%s", text)
	}
}

func TestTheAgendaLineOverflowRule(t *testing.T) {
	long := strings.Repeat("x", 300)
	line, f := AgendaLine([]agenda.Item{{Module: "telos", Kind: "goal", ID: "G1", Reason: agenda.Stale, Question: long, Revision: "revised 2026-09-04, last reviewed 2026-05-01", Snoozes: 2}})
	if len(line) > Reservation || f == nil || f.Module != "agenda" || !strings.HasSuffix(line, "…") {
		t.Errorf("line=%d bytes fault=%+v", len(line), f)
	}
	line, f = AgendaLine([]agenda.Item{{Module: "telos", Kind: "goal", ID: "G1", Reason: agenda.Stale, Question: "Still right?", Revision: "revised 2026-09-04, last reviewed 2026-05-01", Snoozes: 2}})
	if f != nil || !strings.Contains(line, "— revised 2026-09-04") || !strings.HasSuffix(line, "(snoozed 2)") {
		t.Errorf("short line = %q fault=%v", line, f)
	}
	if line, f := AgendaLine(nil); line != "agenda: nothing due" || f != nil {
		t.Errorf("empty = %q", line)
	}
	line, _ = AgendaLine([]agenda.Item{{Module: "identity", Kind: "value", Reason: agenda.Empty, Question: "What do you weigh decisions against?"}})
	if !strings.HasPrefix(line, "agenda: [identity/value] What do you") {
		t.Errorf("onboarding line = %q", line)
	}
}

func TestKindOrdersReviewedFirstAndFirstCaps(t *testing.T) {
	d := Data{Records: []store.Stored{
		rec("a.md", "telos", "goal", map[string]string{"id": "A"}, "2026-09-25T00:00:00Z", ""),
		rec("b.md", "telos", "goal", map[string]string{"id": "B"}, "2026-09-01T00:00:00Z", "2026-09-10T00:00:00Z"),
		rec("c.md", "telos", "goal", map[string]string{"id": "C"}, "2026-09-01T00:00:00Z", "2026-09-20T00:00:00Z"),
		rec("d.md", "telos", "problem", nil, "2026-09-01T00:00:00Z", ""),
	}}
	got := d.Kind("goal")
	if len(got) != 3 || got[0].ID != "C" || got[1].ID != "B" || got[2].ID != "A" {
		t.Errorf("order = %v", []string{got[0].ID, got[1].ID, got[2].ID})
	}
	if n := len(first(2, got)); n != 2 {
		t.Errorf("first 2 = %d", n)
	}
	if n := len(first(9, got)); n != 3 {
		t.Errorf("first 9 = %d", n)
	}
}
