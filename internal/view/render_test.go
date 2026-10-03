package view

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/allanschon/brabeus/internal/agenda"
	"github.com/allanschon/brabeus/internal/block"
	"github.com/allanschon/brabeus/internal/instructions"
	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/store"
)

// modWithTemplate writes a ratified-record module whose goal kind ships tmpl.
func modWithTemplate(t *testing.T, tmpl string) *module.Set {
	t.Helper()
	dir := t.TempDir()
	m := filepath.Join(dir, "telos")
	os.MkdirAll(filepath.Join(m, "view"), 0o755)
	os.WriteFile(filepath.Join(m, "summary.md.tmpl"), []byte("telos:\n"), 0o644)
	os.WriteFile(filepath.Join(m, "view", "goal.html.tmpl"), []byte(tmpl), 0o644)
	os.WriteFile(filepath.Join(m, "module.json"), []byte(`{"name": "telos", "version": 1, "profile": "ratified-record",
	  "priority": 10, "budget_bytes": 600, "summary": "summary.md.tmpl",
	  "kinds": {"goal": {"fields": ["title"], "view": {"template": "view/goal.html.tmpl"}}}}`), 0o644)
	set, err := module.Load(dir, []string{"telos"})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func goalPage(set *module.Set, recs ...Record) ModulePage {
	return ModulePage{Module: set.Modules[0], Kinds: []KindPage{{Module: "telos", Kind: "goal",
		View: module.View{Template: "view/goal.html.tmpl"}, Records: recs}}}
}

// Spec §6: a view template that does not parse refuses its module on load.
func TestAViewTemplateThatDoesNotParseRefusesTheKernel(t *testing.T) {
	_, err := New(modWithTemplate(t, `{{range .Records}}`))
	if err == nil || !strings.Contains(err.Error(), "module telos") {
		t.Fatalf("want a load error naming the module, got %v", err)
	}
}

// Spec §6: a template may not redefine a kernel partial.
func TestAViewTemplateThatRedefinesAKernelPartialIsRefused(t *testing.T) {
	_, err := New(modWithTemplate(t, `{{define "claims"}}mine{{end}}<p>x</p>`))
	if err == nil || !strings.Contains(err.Error(), `"claims"`) {
		t.Fatalf("want refusal naming the partial, got %v", err)
	}
}

// Spec §6: the reserved names are the kernel's own template names, so a
// partial added later is reserved without anyone listing it.
func TestTheReservedNamesAreTheKernelsTemplates(t *testing.T) {
	for _, want := range []string{"claims", "freshness", "page"} {
		if !slices.Contains(Reserved, want) {
			t.Errorf("Reserved lacks %q: %v", want, Reserved)
		}
	}
	for _, n := range Reserved {
		if n == "kernel" || strings.HasSuffix(n, ".html.tmpl") {
			t.Errorf("Reserved carries the file or root name %q", n)
		}
	}
	if !slices.IsSorted(Reserved) {
		t.Errorf("Reserved is not sorted: %v", Reserved)
	}
}

// Spec §6: one that fails while rendering is one line on its page, and the
// rest of the page renders.
func TestAViewTemplateThatFailsWhileRenderingIsOneLine(t *testing.T) {
	set := modWithTemplate(t, `{{range .Records}}{{index .Claims 5}}{{end}}`)
	r, err := New(set)
	if err != nil {
		t.Fatal(err)
	}
	rec := Record{Path: "telos/goal/g.md", Module: "telos", Kind: "goal", Name: "g"}
	var buf bytes.Buffer
	if err := r.Module(&buf, goalPage(set, rec)); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `class="render-failure"`) || !strings.Contains(out, "</html>") {
		t.Errorf("want the failure line inside a whole page:\n%s", out)
	}
	if !strings.Contains(out, "The telos goal records could not be shown:</span> the module's view template failed. The kernel log has the detail.") {
		t.Errorf("the failure sentence is missing:\n%s", out)
	}
}

// Spec §10: record content is text, never markup, in kernel layouts and
// module templates alike.
func TestRecordContentNeverBecomesMarkup(t *testing.T) {
	set := modWithTemplate(t, `{{range .Records}}<h3>{{.Fields.title}}</h3>{{.Body}}{{end}}`)
	r, err := New(set)
	if err != nil {
		t.Fatal(err)
	}
	evil := `<script>alert(1)</script><b onclick="x()">b</b>`
	rec := Record{Module: "telos", Kind: "goal", Name: "g", Fields: map[string]string{"title": evil}, Body: r.Markdown(evil)}
	var buf bytes.Buffer
	r.Module(&buf, goalPage(set, rec))
	out := buf.String()
	// html/template escapes the title, so "onclick=" survives as text; what
	// must never appear is a live tag.
	if strings.Contains(out, "<script") || strings.Contains(out, "<b ") || !strings.Contains(out, "&lt;script") {
		t.Errorf("markup leaked, or the title was not shown as text:\n%s", out)
	}
	// The same record through each kernel layout.
	shipped := shipped(t)
	kr, _ := New(shipped)
	man, _ := shipped.Module("identity")
	for _, layout := range module.Layouts {
		buf.Reset()
		kp := KindPage{Module: "identity", Kind: "fact", View: module.View{Layout: layout, Title: "title"}, Records: []Record{rec}}
		kr.Module(&buf, ModulePage{Module: man, Kinds: []KindPage{kp}})
		if out := buf.String(); strings.Contains(out, "<script") || strings.Contains(out, "<b ") || !strings.Contains(out, "&lt;script") {
			t.Errorf("%s layout leaked markup, or did not show the title:\n%s", layout, out)
		}
	}
}

// Spec §10: the block is shown line by line, so each line is text, and a
// record that put markup into its summary still cannot put it on the page.
func TestTheBlockIsEscapedLinesWithAKeyPerModule(t *testing.T) {
	r, err := New(shipped(t))
	if err != nil {
		t.Fatal(err)
	}
	parts := []block.Part{{Text: "agenda: nothing due\n"},
		{Module: "identity", Text: "identity:\n- value: <script>alert(1)</script>\n- plain line\n"}}
	var buf bytes.Buffer
	if err := r.Home(&buf, Home{Parts: parts}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "<script>alert") || !strings.Contains(out, "&lt;script&gt;") {
		t.Errorf("a block line became markup:\n%s", out)
	}
	if !strings.Contains(out, `<a class="block__key" href="/view/identity/">identity:</a>`) {
		t.Errorf("the share's key does not link to its module:\n%s", out)
	}
	if !strings.Contains(out, "<li>plain line</li>") {
		t.Errorf("a line kept its list marker, or is missing:\n%s", out)
	}
	// The byte count is the joined block's, as /context returns it.
	if want := len(parts[0].Text) + len(parts[1].Text); !strings.Contains(out, "every session · "+commas(want)+" bytes") {
		t.Errorf("block__meta lacks the byte count %d:\n%s", want, out)
	}
}

// Spec §10: emptiness is a sentence, never an empty table.
func TestAnEmptyHomeSaysSo(t *testing.T) {
	r, err := New(shipped(t))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := r.Home(&buf, Home{Parts: []block.Part{{Text: "agenda: nothing due\n"}}}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Nothing due.", "Nothing behind.", "agenda: nothing due"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("home lacks %q", want)
		}
	}
}

// Spec §10: the stylesheet declares its values as custom properties, light
// and dark; kernel markup carries no style attribute.
func TestTheStylesheetIsTokensAndMarkupHasNoStyle(t *testing.T) {
	css := string(Stylesheet())
	if !strings.Contains(css, ":root {") || !strings.Contains(css, "prefers-color-scheme: dark") {
		t.Error("stylesheet must declare tokens on :root with a dark variant")
	}
	if !strings.Contains(css, `url("/view/fonts/barlow-latin-400-normal.woff2")`) {
		t.Error("stylesheet must load the self-hosted fonts")
	}
	r, _ := New(shipped(t))
	var buf bytes.Buffer
	r.Home(&buf, fullHome())
	if strings.Contains(buf.String(), "style=") {
		t.Error("kernel markup must not carry style attributes")
	}
}

func ip(n int) *int { return &n }

// fullHome exercises every section of the home page.
func fullHome() Home {
	behind := Claim{Goal: "telos/goal/g.md", GoalHref: "/view/telos/#r-goal-g", ID: "G1", Title: "Plant the beds", Text: "all planted",
		State: "behind", Adapter: "manual", Manual: true, Count: ip(3), Target: ip(1200), Expected: ip(400), DaysLeft: ip(20),
		Since: "2026-09-28T09:00:00Z"}
	return Home{
		Nav:          []Nav{{Name: "telos", Href: "/view/telos/"}, {Name: "memory", Href: "/view/memory/", Notes: true}},
		Parts:        []block.Part{{Text: "agenda: [telos/goal G1] Still right?\n"}, {Module: "telos", Text: "telos:\n- G1 Plant the beds\n"}},
		Instructions: []instructions.Record{{Module: "identity", Path: "identity/register/plain.md", Text: "Say it plainly."}},
		Sizes:        []instructions.Size{{Module: "identity", Bytes: 5000, Budget: 4096}},
		Due:          []agenda.Item{{Path: "telos/goal/g.md", Module: "telos", Kind: "goal", ID: "G1", Name: "g", Reason: agenda.Behind, Question: "Still right?"}},
		Behind:       []Claim{behind},
		Links:        map[string]string{"telos/goal/g.md": "/view/telos/#r-goal-g"},
		Faults: Faults{Manual: 1, Total: 3, LastRun: "2026-10-03T08:00:00Z", Interval: "24h",
			NoEvidence: []Claim{{GoalHref: "/view/telos/#r-goal-h", ID: "G2", Text: "tasks done", State: "no-evidence", Detail: "check could not run"}},
			Block:      []block.Fault{{Module: "telos", Bytes: 700, Budget: 600}}},
		Now: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC),
	}
}

// Spec §10: the home page shows what is due, what is behind with its
// progress, the instructions against their budget, and the faults.
func TestAFullHomeShowsEverySection(t *testing.T) {
	r, err := New(shipped(t))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := r.Home(&buf, fullHome()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`<a class="site-nav__link" href="/view/" aria-current="page">Home</a>`,
		`site-nav__group--notes`, `>Memory</a>`,
		`Your record as of Saturday, 3 October 2026`,
		`<a href="/view/telos/#r-goal-g">[telos/goal G1]</a>`,
		`reason reason--behind`, `<a class="due__record" href="/view/telos/#r-goal-g">telos/goal G1</a>`,
		`behind-entry behind-entry--behind`, `G1 · Plant the beds`,
		`<progress class="bar bar--behind" value="3" max="1200">3 of 1,200</progress>`,
		`<progress class="bar bar--tick" value="400" max="1200" aria-hidden="true"></progress>`,
		`expected 400 by now`, `20 days left · checked by hand`,
		`<h3 class="instruction__kind">Register</h3>`, `<p>Say it plainly.</p>`,
		`value="4096" max="4096"`, `5,000 / 4,096 bytes`, `over budget by 904 bytes`,
		`1 of 3 claims · 33%`, `state state--no-evidence`, `telos: 700 of 600 bytes`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("home lacks %q", want)
		}
	}
}

// Spec §10: a working-memory module's page lists its notes grouped by
// scope, global first, under a banner saying they are not the person's word.
func TestTheNotesPageGroupsByScope(t *testing.T) {
	set := shipped(t)
	r, err := New(set)
	if err != nil {
		t.Fatal(err)
	}
	man, _ := set.Module("memory")
	notes := []Record{
		{Module: "memory", Kind: "note", Name: "b", Description: "desk sleeps", Scope: "machine/desk", ScopeKind: "machine"},
		{Module: "memory", Kind: "note", Name: "a", Description: "use the api", Scope: "global", ScopeKind: "global"},
	}
	var buf bytes.Buffer
	if err := r.Module(&buf, ModulePage{Module: man, Notes: notes}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	g, m := strings.Index(out, `id="scope-global"`), strings.Index(out, `id="scope-machine-desk"`)
	if g < 0 || m < 0 || g > m {
		t.Errorf("want a global section before a machine/desk section:\n%s", out)
	}
	for _, want := range []string{`class="notes-banner"`, `<span class="scope scope--machine">`, "use the api", `<a href="#scope-machine-desk">machine/desk</a>`} {
		if !strings.Contains(out, want) {
			t.Errorf("notes page lacks %q", want)
		}
	}
}

// Spec §10: a kind section is headed by the kernel, laid out in its
// declared layout, and an empty module says so.
func TestKernelLayoutsRenderRecordsAndEmptiness(t *testing.T) {
	set := shipped(t)
	r, err := New(set)
	if err != nil {
		t.Fatal(err)
	}
	man, _ := set.Module("identity")
	rec := Record{Module: "identity", Kind: "fact", Name: "tall", Scope: "global", ScopeKind: "global",
		Fields: map[string]string{"statement": "Is tall."}, Freshness: Freshness{Unconfirmed: true, Class: "draft"}}
	for _, layout := range module.Layouts {
		var buf bytes.Buffer
		kp := KindPage{Module: "identity", Kind: "fact", Title: "Fact", View: module.View{Layout: layout, Title: "statement", Fields: []string{"statement"}}, Records: []Record{rec}}
		if err := r.Module(&buf, ModulePage{Module: man, Kinds: []KindPage{kp}}); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		for _, want := range []string{`class="layout-` + layout + `"`, "Is tall.", "freshness freshness--draft", `id="kind-fact"`, `href="#kind-fact"`} {
			if !strings.Contains(out, want) {
				t.Errorf("%s layout lacks %q:\n%s", layout, want, out)
			}
		}
		if strings.Contains(out, "render-failure") {
			t.Errorf("%s layout failed:\n%s", layout, out)
		}
	}
	var buf bytes.Buffer
	r.Module(&buf, ModulePage{Module: man})
	if !strings.Contains(buf.String(), "Nothing recorded here yet.") {
		t.Errorf("an empty module page lacks its sentence")
	}
}

// Spec §11: Font serves a font file by exact name and nothing else, so
// it cannot be used to read the embedded tree.
func TestFontServesOnlyAWoff2ByName(t *testing.T) {
	if b, ok := Font("barlow-latin-400-normal.woff2"); !ok || !bytes.HasPrefix(b, []byte("wOF2")) {
		t.Errorf("Font refused or mangled a shipped font: ok=%v", ok)
	}
	for _, bad := range []string{"../x", "../fonts/barlow-latin-400-normal.woff2", "OFL.txt", "fonts/barlow-latin-400-normal.woff2", "missing.woff2", ""} {
		if _, ok := Font(bad); ok {
			t.Errorf("Font(%q) answered", bad)
		}
	}
}

// The figures the markup needs and a template cannot compute.
func TestCommasGroupsThousands(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 12345: "12,345", 1234567: "1,234,567", -2718: "-2,718"} {
		if got := commas(n); got != want {
			t.Errorf("commas(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestClaimsGainProgressAndASinceDate(t *testing.T) {
	cs := annotate([]Claim{
		{State: "behind", Count: ip(3), Target: ip(1200), Expected: ip(400), Since: "2026-09-28T09:00:00Z"},
		{State: "open", Count: ip(2)},
		{State: "pass", Count: ip(1), Target: ip(1)},
	})
	p := cs[0].Progress
	if p == nil || p.Value != 3 || p.Max != 1200 || !p.Tick || p.Expected != 400 || p.MaxText != "1,200" || cs[0].SinceDate != "2026-09-28" {
		t.Errorf("behind claim = %+v %+v", cs[0], p)
	}
	if cs[1].Progress != nil {
		t.Errorf("a count with no target has no bar: %+v", cs[1].Progress)
	}
	if p := cs[2].Progress; p == nil || p.Tick {
		t.Errorf("a claim with no expected figure has a bar and no tick: %+v", p)
	}
}

func TestBuildRecordsAddsScopeKindFreshnessClassAndDaysLeft(t *testing.T) {
	set := shipped(t)
	goal := stored("telos/goal/g.md", "telos", "goal", now.AddDate(0, 0, -100))
	goal.Fields = map[string]string{"title": "t", "by": "2026-10-17"}
	goal.Scope = "machine/desk"
	late := stored("telos/goal/late.md", "telos", "goal", now)
	late.Fields = map[string]string{"by": "2026-09-27"}
	late.Scope = "project/garden"
	draft := stored("identity/value/v.md", "identity", "value", time.Time{})
	thread := stored("memory/thread/t.md", "memory", "thread", now)
	recs := BuildRecords(set, []store.Stored{goal, late, draft, thread},
		[]Claim{{Goal: goal.Path, State: "open", Count: ip(1), Target: ip(2)}}, plain, now)
	by := map[string]Record{}
	for _, r := range recs {
		by[r.Path] = r
	}
	g, l, d, th := by[goal.Path], by[late.Path], by[draft.Path], by[thread.Path]
	if g.ScopeKind != "machine" || l.ScopeKind != "project" || d.ScopeKind != "global" {
		t.Errorf("scope kinds = %q %q %q", g.ScopeKind, l.ScopeKind, d.ScopeKind)
	}
	if g.Freshness.Class != "due" || d.Freshness.Class != "draft" || th.Freshness.Class != "unknown" || l.Freshness.Class != "" {
		t.Errorf("freshness classes = %q %q %q %q", g.Freshness.Class, d.Freshness.Class, th.Freshness.Class, l.Freshness.Class)
	}
	if g.DaysLeft == nil || *g.DaysLeft != 14 || g.DaysPast != 0 {
		t.Errorf("goal by 2026-10-17 on 2026-10-03: left %v past %d", g.DaysLeft, g.DaysPast)
	}
	if l.DaysLeft != nil || l.DaysPast != 6 {
		t.Errorf("goal by 2026-09-27 on 2026-10-03: left %v past %d", l.DaysLeft, l.DaysPast)
	}
	if d.DaysLeft != nil || d.DaysPast != 0 {
		t.Errorf("a record with no by has no days: %v %d", d.DaysLeft, d.DaysPast)
	}
	if len(g.Claims) != 1 || g.Claims[0].Progress == nil || g.Claims[0].Progress.Max != 2 {
		t.Errorf("record claims are not annotated: %+v", g.Claims)
	}
}

func TestKindsTitlesEachKind(t *testing.T) {
	set := shipped(t)
	recs := BuildRecords(set, []store.Stored{stored("telos/mission/m.md", "telos", "mission", now)}, nil, plain, now)
	if pages := Kinds(set, "telos", recs, now); len(pages) != 1 || pages[0].Title != "Mission" {
		t.Errorf("kind pages = %+v", pages)
	}
}

func TestAgendaLineSplitsItsReference(t *testing.T) {
	items := []agenda.Item{{Path: "telos/goal/g.md", Module: "telos", Kind: "goal", ID: "G1", Name: "g", Reason: agenda.Behind}}
	links := map[string]string{"telos/goal/g.md": "/view/telos/#r-goal-g"}
	a := agendaOf(block.Part{Text: "agenda: [telos/goal G1] Still right?\n"}, items, links)
	if a.Ref != "[telos/goal G1]" || a.Href != "/view/telos/#r-goal-g" || a.Question != "Still right?" {
		t.Errorf("agenda = %+v", a)
	}
	// A record with no visible page is named, never linked.
	if a := agendaOf(block.Part{Text: "agenda: [telos/goal G1] Still right?\n"}, items, nil); a.Href != "" {
		t.Errorf("an unlisted record is linked: %+v", a)
	}
	if a := agendaOf(block.Part{Text: "agenda: nothing due\n"}, nil, nil); a.Ref != "" || a.Line != "agenda: nothing due" {
		t.Errorf("empty agenda = %+v", a)
	}
}

// §13: a telos goal shows its claims, with behind apart from fail, and its
// revision line, through the module's own view template.
func TestTheTelosGoalTemplateShowsClaimsAndRevision(t *testing.T) {
	set := shipped(t)
	r, err := New(set)
	if err != nil {
		t.Fatal(err)
	}
	man, _ := set.Module("telos")
	days := 20
	g := Record{Module: "telos", Kind: "goal", Name: "cull", ID: "G1", Scope: "global", ScopeKind: "global",
		Fields:   map[string]string{"id": "G1", "title": "Cull photos", "by": "2026-12-31"},
		DaysLeft: &days,
		Claims: []Claim{
			{Goal: "telos/goal/cull.md", ID: "G1", Text: "all reviewed", State: "behind"},
			{Text: "labelled", State: "fail"}},
		Revision: "target raised from 5 to 6",
		Serves:   []Ref{{Name: "sharing", Href: "/view/identity/#sharing"}, {Name: "plainvalue"}}}
	var buf bytes.Buffer
	if err := r.Module(&buf, ModulePage{Module: man, Kinds: Kinds(set, "telos", []Record{g}, now)}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{`<ol class="goals">`, `id="r-goal-cull"`, "G1", "Cull photos", "2026-12-31", "20 days left",
		"claim--behind", "claim--fail", "target raised from 5 to 6", `<a href="/view/identity/#sharing">sharing</a>`, "plainvalue"} {
		if !strings.Contains(out, want) {
			t.Errorf("goal page lacks %q", want)
		}
	}
	if strings.Contains(out, `href="">`) || strings.Contains(out, "render-failure") {
		t.Errorf("the shipped template failed or linked nothing:\n%s", out)
	}
	if n := strings.Count(out, `id="kind-goal"`); n != 1 {
		t.Errorf("the template drew its own section: %d", n)
	}
}

// The shipped modules lay values, currents and ideals out as cards.
func TestShippedValuesCurrentsAndIdealsAreCards(t *testing.T) {
	set := shipped(t)
	r, err := New(set)
	if err != nil {
		t.Fatal(err)
	}
	for mod, rec := range map[string]Record{
		"identity": {Module: "identity", Kind: "value", Name: "v", Fields: map[string]string{"statement": "Keep it simple"}},
		"telos":    {Module: "telos", Kind: "current", Name: "c", Fields: map[string]string{"dimension": "sleep", "text": "seven hours"}},
	} {
		man, _ := set.Module(mod)
		var buf bytes.Buffer
		if err := r.Module(&buf, ModulePage{Module: man, Kinds: Kinds(set, mod, []Record{rec}, now)}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "layout-cards") {
			t.Errorf("%s is not laid out as cards", mod)
		}
	}
}

// Spec §10: two kinds on one page may share a record name; each record
// keeps its own id, and none is a kernel id.
func TestTwoKindsSharingANameGetDistinctIds(t *testing.T) {
	set := shipped(t)
	r, err := New(set)
	if err != nil {
		t.Fatal(err)
	}
	man, _ := set.Module("telos")
	recs := BuildRecords(set, []store.Stored{
		stored("telos/mission/due.md", "telos", "mission", now),
		stored("telos/problem/due.md", "telos", "problem", now),
	}, nil, plain, now)
	var buf bytes.Buffer
	if err := r.Module(&buf, ModulePage{Module: man, Kinds: Kinds(set, "telos", recs, now)}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{`id="r-mission-due"`, `id="r-problem-due"`} {
		if strings.Count(out, want) != 1 {
			t.Errorf("want one %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, `id="due"`) {
		t.Error("a record took the kernel's due id")
	}
}

// A count of days reads as English: one day, today, and past.
func TestDaysReadAsEnglish(t *testing.T) {
	for _, c := range []struct {
		left *int
		past int
		want string
	}{
		{ip(0), 0, "due today"}, {ip(1), 0, "1 day left"}, {ip(2), 0, "2 days left"},
		{nil, 1, "1 day past"}, {nil, 2, "2 days past"}, {nil, 0, ""},
	} {
		if got := (Record{DaysLeft: c.left, DaysPast: c.past}).DaysText(); got != c.want {
			t.Errorf("record left %v past %d = %q, want %q", c.left, c.past, got, c.want)
		}
		if c.left != nil {
			if got := (Claim{DaysLeft: c.left}).DaysText(); got != c.want {
				t.Errorf("claim left %d = %q, want %q", *c.left, got, c.want)
			}
		}
	}
	set := shipped(t)
	r, err := New(set)
	if err != nil {
		t.Fatal(err)
	}
	man, _ := set.Module("telos")
	for _, c := range []struct {
		left *int
		past int
		want string
	}{{ip(0), 0, "due today"}, {ip(1), 0, "1 day left"}, {ip(2), 0, "2 days left"}, {nil, 1, "1 day past"}} {
		g := Record{Module: "telos", Kind: "goal", Name: "g", Fields: map[string]string{"by": "2026-10-03"}, DaysLeft: c.left, DaysPast: c.past}
		var buf bytes.Buffer
		if err := r.Module(&buf, ModulePage{Module: man, Kinds: Kinds(set, "telos", []Record{g}, now)}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), `<span class="goal__days">`+c.want+`</span>`) {
			t.Errorf("goal page lacks %q:\n%s", c.want, buf.String())
		}
	}
	h := fullHome()
	h.Behind[0].DaysLeft = ip(1)
	var buf bytes.Buffer
	if err := r.Home(&buf, h); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "1 day left · checked by hand") {
		t.Errorf("home's behind entry lacks the singular day")
	}
}

// A due item whose record has no visible page is named, never linked.
func TestADueItemWithNoPageIsText(t *testing.T) {
	r, err := New(shipped(t))
	if err != nil {
		t.Fatal(err)
	}
	h := fullHome()
	h.Links = nil
	var buf bytes.Buffer
	if err := r.Home(&buf, h); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `<span class="due__record">telos/goal G1</span>`) {
		t.Errorf("an unlinked due item is not text:\n%s", buf.String())
	}
}
