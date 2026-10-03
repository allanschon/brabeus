// Package view renders the person's record as read-only HTML (spec §10): a
// home page carrying the block, and one page per module listing its active
// records. It is pure: the server gathers the inputs through the same code
// the tools use, so the view cannot disagree with what a session receives.
package view

import (
	"html/template"
	"sort"
	"strings"
	"time"

	"github.com/allanschon/brabeus/internal/module"
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
}

// KindPage is one kind's section of a module page.
type KindPage struct {
	Module, Kind string
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
			Claims: byGoal[r.Path], Revision: r.Revision}
		v.Freshness = Freshness{Reviewed: r.Reviewed, Every: kind.FreshnessDays, Unconfirmed: r.Reviewed.IsZero()}
		if !v.Freshness.Unconfirmed {
			v.Freshness.Age = int(now.Sub(r.Reviewed).Hours() / 24)
			v.Freshness.Overdue = kind.FreshnessDays > 0 && v.Freshness.Age > kind.FreshnessDays
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
		out = append(out, KindPage{Module: mod, Kind: k, View: v, Records: rs, Now: now})
	}
	return out
}

// PageModuleOf is PageModule for a view record.
func PageModuleOf(set *module.Set, r Record) string {
	s := store.Stored{}
	s.Module, s.Kind, s.Reviewed = r.Module, r.Kind, r.Freshness.Reviewed
	return PageModule(set, s)
}
