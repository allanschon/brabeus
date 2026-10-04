package block

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
	items := agenda.Compute(set, recs, nil, now)
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

// AA, §10: a record governed by a ratified-record module renders marked
// unconfirmed until reviewed, whatever its kind. The block hands the marker to
// every template as a field, and this test renders every shipped ratified
// template with one unreviewed record per kind and fails on any line without it.
func TestEveryShippedTemplateMarksAnUnreviewedRecordOfEveryKind(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	for _, name := range []string{"identity", "telos", "health", "finance"} {
		set := shippedSet(t, name)
		r, err := New(set)
		if err != nil {
			t.Fatal(err)
		}
		man, _ := set.Module(name)
		for kind, k := range man.Kinds {
			fields := map[string]string{}
			for _, f := range k.Fields {
				fields[f] = "probe-" + f
			}
			if _, ok := fields["revisit"]; ok {
				fields["revisit"] = "2027-01-01"
			}
			probe := rec(fmt.Sprintf("%s/%s/x.md", name, kind), name, kind, fields, "2026-09-20T00:00:00Z", "")
			text, faults := r.Render(nil, []store.Stored{probe}, now, store.Visibility{})
			if len(faults) != 0 {
				t.Errorf("%s/%s: faults %+v", name, kind, faults)
			}
			_, section, _ := strings.Cut(text, "\n")
			if strings.TrimSpace(section) == name+":" {
				continue // a kind the template does not render at all is not a marker failure
			}
			if !strings.Contains(section, "(unconfirmed)") {
				t.Errorf("%s/%s renders unreviewed without the marker:\n%s", name, kind, section)
			}
		}
	}
}

// Identity marks register as instructions, which are delivered in full beside
// the block (spec §7), so the summary no longer renders it.
func TestARegisterRecordIsNotInTheSummary(t *testing.T) {
	set := shippedSet(t, "identity")
	r, err := New(set)
	if err != nil {
		t.Fatal(err)
	}
	now := at("2026-10-01T00:00:00Z")
	recs := []store.Stored{
		rec("identity/register/x.md", "identity", "register", map[string]string{"statement": "dry, no fluff"}, "2026-09-20T00:00:00Z", "2026-09-20T00:00:00Z"),
	}
	text, faults := r.Render(nil, recs, now, store.Visibility{})
	if len(faults) != 0 {
		t.Fatalf("faults: %+v", faults)
	}
	if strings.Contains(text, "dry, no fluff") {
		t.Errorf("register is an instruction and stays out of the summary:\n%s", text)
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
	text, _ := r.Render(agenda.Compute(set, recs, nil, now), recs, now, store.Visibility{})
	i, tl := strings.Index(text, "\nidentity:"), strings.Index(text, "\ntelos:")
	if i < 0 || tl < 0 || i > tl {
		t.Errorf("identity (priority 5) must precede telos (10):\n%s", text)
	}
	if strings.Contains(text, "infra/n.md") || strings.Contains(text, "memory:") {
		t.Errorf("working-memory records never render:\n%s", text)
	}
}

func TestACrossingPreferenceIsNotInTheSummary(t *testing.T) {
	set := shippedSet(t, "memory", "identity")
	r, _ := New(set)
	now := at("2026-10-01T00:00:00Z")
	recs := []store.Stored{
		rec("personal/terse.md", "memory", "preference", nil, "2026-09-01T00:00:00Z", "2026-09-02T00:00:00Z"),
		rec("personal/draft.md", "memory", "preference", nil, "2026-09-01T00:00:00Z", ""),
	}
	recs[0].Description, recs[1].Description = "terse answers", "an unratified guess"
	text, _ := r.Render(nil, recs, now, store.Visibility{})
	if strings.Contains(text, "terse answers") || strings.Contains(text, "unratified guess") {
		t.Errorf("preference is an instruction and stays out of the summary:\n%s", text)
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

// TestEveryShippedTemplateFitsItsBudgetWhenFullyPopulated guards the "first n"
// caps in the shipped templates: they must not overflow the manifest's
// budget_bytes at realistic field lengths, because the budgets already sum to
// 1750 of the 1792 bytes available under the cap and the reservation, and
// only the register line's addition moved that sum at all. It runs once with
// every record reviewed, as before, and once with none of them reviewed, so
// the caps hold with every line carrying the " (unconfirmed)" marker too. The
// only state-dependent content left in any shipped line is the marker itself
// — the reviewed date that used to follow a telos goal was dropped along with
// it, because keeping both suffixes overflows telos in a goal-reviewed,
// everything-else-unreviewed block: 477 (base) + 4×22 (the dropped suffix,
// worst case) + 4×14 (the marker) = 621 against telos's 600-byte budget, a
// state neither the all-reviewed nor the all-unreviewed pass alone would
// catch. With the suffix gone, the unreviewed pass is provably each
// template's worst case; a future template that adds another state-dependent
// suffix needs a pass of its own. If this fails, the caps go down further;
// the budgets never go up.
func TestEveryShippedTemplateFitsItsBudgetWhenFullyPopulated(t *testing.T) {
	now := at("2026-10-01T00:00:00Z")
	long := strings.Repeat("x", 40) // a plausible 40-byte field value
	for _, reviewed := range []string{"2026-09-20T00:00:00Z", ""} {
		for _, name := range []string{"identity", "telos", "health", "finance"} {
			set := shippedSet(t, name)
			r, err := New(set)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			man, ok := set.Module(name)
			if !ok {
				t.Fatalf("%s: not in its own set", name)
			}
			var recs []store.Stored
			for kind, k := range man.Kinds {
				for i := 0; i < 10; i++ { // more than any shipped cap
					fields := map[string]string{}
					for _, f := range k.Fields {
						switch f {
						case "id":
							fields[f] = fmt.Sprintf("G%d", i)
						case "by", "due", "revisit", "measured":
							fields[f] = "2027-01-01"
						case "value":
							fields[f] = "80 kg"
						default:
							fields[f] = long
						}
					}
					path := fmt.Sprintf("%s/%s/%d.md", name, kind, i)
					recs = append(recs, rec(path, name, kind, fields, "2026-09-20T00:00:00Z", reviewed))
				}
			}
			text, faults := r.Render(nil, recs, now, store.Visibility{})
			if len(faults) != 0 {
				t.Errorf("%s reviewed=%q: faults = %+v", name, reviewed, faults)
			}
			_, section, ok := strings.Cut(text, "\n")
			if !ok {
				t.Fatalf("%s: no module section:\n%s", name, text)
			}
			if len(section) > man.BudgetBytes {
				t.Errorf("%s reviewed=%q: section is %d bytes, budget is %d:\n%s", name, reviewed, len(section), man.BudgetBytes, section)
			}
		}
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
	line, _ = AgendaLine([]agenda.Item{{Module: "identity", Kind: "value", Reason: agenda.Onboarding, Question: "What do you weigh decisions against?"}})
	if !strings.HasPrefix(line, "agenda: [identity/value] What do you") {
		t.Errorf("onboarding line = %q", line)
	}
	// An untagged item (no module: a pre-module record, or one whose kernel
	// keys failed to parse) is named by its path, not "[/ name]".
	line, _ = AgendaLine([]agenda.Item{{Path: "personal/old.md", Reason: agenda.Stale, Question: "personal/old.md predates modules and cannot be reviewed until the migration has run."}})
	if !strings.HasPrefix(line, "agenda: [personal/old.md] ") {
		t.Errorf("untagged line = %q", line)
	}
	// A multi-byte-rune question must still cut to a valid, in-budget line:
	// the back-off must never land mid-rune.
	multibyte := strings.Repeat("é", 300)
	line, f = AgendaLine([]agenda.Item{{Module: "telos", Kind: "goal", ID: "G1", Reason: agenda.Stale, Question: multibyte}})
	if !utf8.ValidString(line) || len(line) > Reservation || f == nil || !strings.HasSuffix(line, "…") {
		t.Errorf("multibyte cut: valid=%v bytes=%d fault=%+v line=%q", utf8.ValidString(line), len(line), f, line)
	}
}

func TestKindOrdersReviewedFirstAndFirstCaps(t *testing.T) {
	d := Data{Records: []Rec{
		{Stored: rec("a.md", "telos", "goal", map[string]string{"id": "A"}, "2026-09-25T00:00:00Z", "")},
		{Stored: rec("b.md", "telos", "goal", map[string]string{"id": "B"}, "2026-09-01T00:00:00Z", "2026-09-10T00:00:00Z")},
		{Stored: rec("c.md", "telos", "goal", map[string]string{"id": "C"}, "2026-09-01T00:00:00Z", "2026-09-20T00:00:00Z")},
		{Stored: rec("d.md", "telos", "problem", nil, "2026-09-01T00:00:00Z", "")},
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

func TestAgendaLineNamesAModuleOnlyItem(t *testing.T) {
	line, _ := AgendaLine([]agenda.Item{{Module: "identity", Reason: agenda.Budget, Question: "Q?"}})
	if line != "agenda: [identity] Q?" {
		t.Errorf("line = %q", line)
	}
}

// Spec §10: the view shows the block exactly as a session receives it, so
// the parts must join to Render's output byte for byte.
func TestRenderPartsJoinToTheBlock(t *testing.T) {
	set := shippedSet(t, "identity", "telos")
	r, err := New(set)
	if err != nil {
		t.Fatal(err)
	}
	recs := []store.Stored{
		rec("identity/value/family.md", "identity", "value", map[string]string{"statement": "Family first."}, "2026-09-01T00:00:00Z", "2026-09-02T00:00:00Z"),
		rec("telos/mission/m.md", "telos", "mission", map[string]string{"statement": "Refine the network."}, "2026-09-01T00:00:00Z", "2026-09-02T00:00:00Z"),
	}
	now := at("2026-10-03T00:00:00Z")
	whole, _ := r.Render(nil, recs, now, store.Visibility{})
	parts, _ := r.RenderParts(nil, recs, now, store.Visibility{})
	var joined strings.Builder
	for _, p := range parts {
		joined.WriteString(p.Text)
	}
	if joined.String() != whole {
		t.Fatalf("parts joined:\n%q\nblock:\n%q", joined.String(), whole)
	}
	if parts[0].Module != "" || !strings.HasPrefix(parts[0].Text, "agenda:") {
		t.Errorf("first part must be the agenda line: %+v", parts[0])
	}
	if parts[1].Module != "identity" || parts[2].Module != "telos" {
		t.Errorf("modules in priority order: %+v", parts)
	}
}
