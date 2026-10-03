// Package view renders the person's record as read-only HTML (spec §10): a
// home page carrying the block, and one page per module listing its active
// records. It is pure: the server gathers the inputs through the same code
// the tools use, so the view cannot disagree with what a session receives.
package view

import (
	"html/template"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/scope"
	"github.com/allanschon/brabeus/internal/store"
)

// Claim is one claim as the claims tool reports it today (spec §8.1).
type Claim struct {
	Goal, GoalHref, ID, Title         string // GoalHref is the goal's anchor on its module's page
	Index                             int
	Text, State, Measured, Adapter    string
	Manual, Stale                     bool
	Count, Target, Expected, DaysLeft *int
	Deadline, Since, Detail           string

	// Computed for the markup, which a template cannot work out with the
	// three functions it has: the bar's integers, and Since as a date.
	Progress  *Progress // nil unless the claim has both a count and a target
	SinceDate string    // YYYY-MM-DD, or empty when Since does not parse
}

// Progress is a claim's count against its target, as integers for a
// <progress> bar and as grouped figures for its label; Tick marks an
// expected-by-now figure.
type Progress struct {
	Value, Max, Expected             int
	Tick                             bool
	ValueText, MaxText, ExpectedText string
}

// annotate fills each claim's computed fields. It is idempotent, so a claim
// that passes through it twice is unchanged.
func annotate(cs []Claim) []Claim {
	out := make([]Claim, len(cs))
	for i, c := range cs {
		c.Progress = nil
		if c.Count != nil && c.Target != nil && *c.Target > 0 {
			p := &Progress{Value: *c.Count, Max: *c.Target, ValueText: commas(*c.Count), MaxText: commas(*c.Target)}
			if c.Expected != nil {
				p.Tick, p.Expected, p.ExpectedText = true, *c.Expected, commas(*c.Expected)
			}
			c.Progress = p
		}
		c.SinceDate = ""
		if t, err := time.Parse(time.RFC3339, c.Since); err == nil {
			c.SinceDate = t.UTC().Format("2006-01-02")
		}
		out[i] = c
	}
	return out
}

// commas groups an integer's digits in thousands: 12345 is "12,345".
func commas(n int) string {
	s := strconv.Itoa(n)
	sign := ""
	if n < 0 {
		sign, s = "-", s[1:]
	}
	var b strings.Builder
	for i, d := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return sign + b.String()
}

// Ref is a named record a field points at; Href is empty when no record by
// that name is visible, so a dangling name is shown, never linked.
type Ref struct{ Name, Href string }

// Freshness is time since the person last confirmed the record, against how
// often its kind should be asked about (spec §10).
type Freshness struct {
	Reviewed             time.Time
	Age, Every           int // days; Every is 0 for a kind that is never asked about by age
	Unconfirmed, Overdue bool
	// Class is the markup's name for the state: "draft" when unconfirmed,
	// "unknown" when the kind is never asked about by age, "due" when
	// overdue, and empty when confirmed within its interval.
	Class string
}

// Record is a record as a page and a module's view template see it: its own
// content, plus what the kernel computed for it (spec §6).
type Record struct {
	Path, Module, Kind, Name, Description, ID, Scope string
	Fields                                           map[string]string
	Body                                             template.HTML
	Freshness                                        Freshness
	Snoozes                                          int
	Snoozed                                          time.Time
	Claims                                           []Claim
	Revision                                         string
	Serves                                           []Ref

	// ScopeKind is the scope's first half, "global", "machine" or
	// "project", which the scope label is styled by.
	ScopeKind string
	// DaysLeft is the whole days from now until the record's "by" date,
	// nil when it has none or the date has passed; DaysPast is the days
	// since a passed one. A template cannot subtract dates.
	DaysLeft *int
	DaysPast int
}

// KindPage is one kind's section of a module page.
type KindPage struct {
	Module, Kind string
	Title        string // the kind as a heading: "goal" is "Goal"
	View         module.View
	Records      []Record
	Now          time.Time
}

// PageModule is the module whose page lists r: its governing module, except
// that a crossing record is the model's note until the person reviews it,
// which is the block's rule (spec §7), so it is listed on exactly one page.
func PageModule(set *module.Set, r store.Stored) string {
	gov, _, ok := set.RuleFor(r.Module, r.Kind)
	if !ok || (gov.Name != r.Module && r.Reviewed.IsZero()) {
		return r.Module
	}
	return gov.Name
}

const servesField = "serves"

// byField is a goal's date (spec §9); the view counts the days to it.
const byField = "by"

// scopeKind is "machine" or "project" for those scopes and "global" for
// everything else, which is how scope.Visible reads them too.
func scopeKind(sc string) string {
	if k, _, ok := strings.Cut(scope.NormaliseHost(sc), "/"); ok && (k == "machine" || k == "project") {
		return k
	}
	return "global"
}

// BuildRecords turns stored records into view records. md renders a body;
// the server passes the goldmark renderer, tests pass an escaper.
func BuildRecords(set *module.Set, recs []store.Stored, claims []Claim, md func(string) template.HTML, now time.Time) []Record {
	sorted := append([]store.Stored{}, recs...)
	sort.SliceStable(sorted, func(i, j int) bool { return store.LessByReview(sorted[i], sorted[j]) })
	byName := map[string]string{} // record name -> page href
	for _, r := range sorted {
		byName[r.Name] = "/view/" + PageModule(set, r) + "/#" + r.Name
	}
	byGoal := map[string][]Claim{}
	for _, c := range claims {
		byGoal[c.Goal] = append(byGoal[c.Goal], c)
	}
	out := make([]Record, 0, len(sorted))
	for _, r := range sorted {
		_, kind, _ := set.RuleFor(r.Module, r.Kind)
		v := Record{Path: r.Path, Module: r.Module, Kind: r.Kind, Name: r.Name, Description: r.Description,
			ID: r.ID, Scope: r.Scope, Fields: r.Fields, Body: md(r.Body), Snoozes: r.Snoozes, Snoozed: r.Snoozed,
			Claims: annotate(byGoal[r.Path]), Revision: r.Revision, ScopeKind: scopeKind(r.Scope)}
		v.Freshness = Freshness{Reviewed: r.Reviewed, Every: kind.FreshnessDays, Unconfirmed: r.Reviewed.IsZero()}
		if !v.Freshness.Unconfirmed {
			v.Freshness.Age = int(now.Sub(r.Reviewed).Hours() / 24)
			v.Freshness.Overdue = kind.FreshnessDays > 0 && v.Freshness.Age > kind.FreshnessDays
		}
		switch {
		case v.Freshness.Unconfirmed:
			v.Freshness.Class = "draft"
		case kind.FreshnessDays == 0:
			v.Freshness.Class = "unknown"
		case v.Freshness.Overdue:
			v.Freshness.Class = "due"
		}
		if by, err := time.Parse("2006-01-02", r.Fields[byField]); err == nil {
			today, _ := time.Parse("2006-01-02", now.UTC().Format("2006-01-02"))
			if d := int(by.Sub(today).Hours() / 24); d >= 0 {
				v.DaysLeft = &d
			} else {
				v.DaysPast = -d
			}
		}
		for _, name := range strings.Split(r.Fields[servesField], ",") {
			if name = strings.TrimSpace(name); name != "" {
				v.Serves = append(v.Serves, Ref{Name: name, Href: byName[name]})
			}
		}
		out = append(out, v)
	}
	return out
}

// Kinds groups one module's records by kind, in onboarding order, each in
// its declared view; a kind with no records has no section.
func Kinds(set *module.Set, mod string, recs []Record, now time.Time) []KindPage {
	man, ok := set.Module(mod)
	if !ok {
		return nil
	}
	byKind := map[string][]Record{}
	for _, r := range recs {
		if PageModuleOf(set, r) == mod {
			byKind[r.Kind] = append(byKind[r.Kind], r)
		}
	}
	// Kinds in the module's onboarding order, which is the order the person
	// was first asked about them, then the rest by name.
	rank := map[string]int{}
	for i, k := range man.Onboarding {
		rank[k] = i + 1
	}
	names := make([]string, 0, len(byKind))
	for k := range byKind {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		ri, rj := rank[names[i]], rank[names[j]]
		if ri == 0 {
			ri = len(rank) + 1
		}
		if rj == 0 {
			rj = len(rank) + 1
		}
		if ri != rj {
			return ri < rj
		}
		return names[i] < names[j]
	})
	var out []KindPage
	for _, k := range names {
		kind := man.Kinds[k]
		v := kind.ViewOrDefault()
		rs := byKind[k]
		if v.Sort != "" {
			sort.SliceStable(rs, func(i, j int) bool { return rs[i].Fields[v.Sort] < rs[j].Fields[v.Sort] })
		}
		out = append(out, KindPage{Module: mod, Kind: k, Title: capital(k), View: v, Records: rs, Now: now})
	}
	return out
}

// PageModuleOf is PageModule for a view record.
func PageModuleOf(set *module.Set, r Record) string {
	s := store.Stored{}
	s.Module, s.Kind, s.Reviewed = r.Module, r.Kind, r.Freshness.Reviewed
	return PageModule(set, s)
}

// capital upper-cases a name's first letter, for headings and labels.
func capital(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
