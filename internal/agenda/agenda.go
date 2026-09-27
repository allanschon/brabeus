// Package agenda decides what the interview asks first (spec §9). It is a
// pure function of the record and the module set: nothing is stored about
// what was asked, so the kernel stays stateless.
package agenda

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/store"
)

type Reason string

const (
	Fail  Reason = "fail"
	Stale Reason = "stale"
	Empty Reason = "empty"
)

type Item struct {
	Path, Module, Kind, ID, Name string
	Reason                       Reason
	Question                     string
	Revision                     string
	Snoozes                      int
	Fields                       map[string]string
}

// Compute orders the agenda: claims in fail first (M2; empty here), then
// records past their kind's freshness — native items (the record's own
// module governs it) by module priority then age, ahead of every crossing
// item (a working-memory record governed by a different, ratified module),
// which sort among themselves by age alone (spec ruling) — then the
// onboarding gaps — kinds a module wants on file and has none of.
func Compute(set *module.Set, records []store.Stored, now time.Time) []Item {
	var stale []struct {
		item     Item
		tier     int // 0 native (governed by its own module), 1 crossing
		priority int
		age      time.Duration
	}
	present := map[string]bool{} // module/kind with at least one live record

	for _, r := range records {
		if !r.Retired.IsZero() {
			continue
		}
		// A kernel key that failed to parse (store.Meta.Malformed) means the
		// stamp cannot be trusted: treating it as "never reviewed" would
		// render {reviewed} as a lie, and every verdict Review is given would
		// be refused by the same Malformed check, so the item could never
		// clear. Surface it as a fix-by-hand item instead, same as a
		// pre-module record.
		if len(r.Malformed) > 0 {
			stale = append(stale, struct {
				item     Item
				tier     int
				priority int
				age      time.Duration
			}{Item{Path: r.Path, Name: r.Name, Module: r.Module, Kind: r.Kind, Reason: Stale, Fields: r.Fields,
				Question: fmt.Sprintf("%s: %s malformed; fix the file by hand before it can be reviewed.", r.Path, strings.Join(r.Malformed, ", "))}, 0, -1, 0})
			continue
		}
		if r.Module == "" {
			stale = append(stale, struct {
				item     Item
				tier     int
				priority int
				age      time.Duration
			}{Item{Path: r.Path, Name: r.Name, Reason: Stale, Fields: r.Fields,
				Question: fmt.Sprintf("%s predates modules and cannot be reviewed until the migration has run.", r.Path)}, 0, -1, 0})
			continue
		}
		present[r.Module+"/"+r.Kind] = true

		man, kind, ok := set.RuleFor(r.Module, r.Kind)
		if !ok {
			continue
		}
		bundle, _ := man.Profile.Bundle()
		if !bundle.Interviewed {
			continue
		}
		// The governing module's kind is on file too: a memory/preference
		// satisfies identity's onboarding for preference, or the person is
		// asked "how do you want to be worked with" beside the record that
		// already says so.
		present[man.Name+"/"+r.Kind] = true
		if kind.FreshnessDays <= 0 {
			continue
		}
		tier := 0
		if man.Name != r.Module {
			tier = 1
		}
		var age time.Duration
		switch {
		case r.Reviewed.IsZero() && tier == 0:
			age = 1<<62 - 1 // never reviewed sorts before every native item that was
		case r.Reviewed.IsZero():
			// A crossing record the model just wrote must not be asked at
			// once: its age is how long it has sat unreviewed, not the
			// never-reviewed sentinel a native record gets.
			age = now.Sub(r.Updated)
			if age <= time.Duration(kind.FreshnessDays)*24*time.Hour {
				continue
			}
		default:
			age = now.Sub(r.Reviewed)
			if age <= time.Duration(kind.FreshnessDays)*24*time.Hour {
				continue
			}
		}
		stale = append(stale, struct {
			item     Item
			tier     int
			priority int
			age      time.Duration
		}{Item{Path: r.Path, Module: r.Module, Kind: r.Kind, ID: r.ID, Name: r.Name, Reason: Stale, Fields: r.Fields,
			Question: render(kind.Interview, r), Revision: revision(r), Snoozes: r.Snoozes}, tier, man.Priority, age})
	}
	sort.SliceStable(stale, func(i, j int) bool {
		if stale[i].tier != stale[j].tier {
			return stale[i].tier < stale[j].tier
		}
		if stale[i].priority != stale[j].priority {
			return stale[i].priority < stale[j].priority
		}
		return stale[i].age > stale[j].age
	})

	var out []Item
	for _, s := range stale {
		out = append(out, s.item)
	}
	for _, man := range set.Modules {
		for _, k := range man.Onboarding {
			if present[man.Name+"/"+k] {
				continue
			}
			out = append(out, Item{Module: man.Name, Kind: k, Reason: Empty, Question: man.Kinds[k].First})
		}
	}
	return out
}

// render fills {reviewed} and any {field} in a kind's prompt.
func render(prompt string, r store.Stored) string {
	if strings.TrimSpace(prompt) == "" {
		prompt = "Still right?"
	}
	reviewed := "never"
	if !r.Reviewed.IsZero() {
		reviewed = r.Reviewed.UTC().Format("2006-01-02")
	}
	out := strings.ReplaceAll(prompt, "{reviewed}", reviewed)
	for k, v := range r.Fields {
		out = strings.ReplaceAll(out, "{"+k+"}", v)
	}
	return out
}

// revision names a change since the last review, from the two stamps the
// record carries. The field-level diff from git ("target lowered from 3 to
// 2") lands with the claims in M2.
func revision(r store.Stored) string {
	if r.Reviewed.IsZero() || !r.Updated.After(r.Reviewed) {
		return ""
	}
	return fmt.Sprintf("revised %s, last reviewed %s", r.Updated.UTC().Format("2006-01-02"), r.Reviewed.UTC().Format("2006-01-02"))
}

// Top is the one item the context block renders (spec §9: one item, never a list).
func Top(items []Item) (Item, bool) {
	if len(items) == 0 {
		return Item{}, false
	}
	return items[0], true
}
