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
}

// crossing is the one kind that exists in both profiles (spec §7): a
// preference the model wrote under a working-memory module is asked under
// the named ratified-record module's rule for the same kind. Nothing else
// crosses, which is why this is a constant and not a manifest key.
var crossing = map[string]string{"preference": "identity"}

// Compute orders the agenda: claims in fail first (M2; empty here), then
// records past their kind's freshness by module priority and age, then the
// onboarding gaps — kinds a module wants on file and has none of.
func Compute(set *module.Set, records []store.Stored, now time.Time) []Item {
	var stale []struct {
		item     Item
		priority int
		age      time.Duration
	}
	present := map[string]bool{} // module/kind with at least one live record

	for _, r := range records {
		if !r.Retired.IsZero() {
			continue
		}
		if r.Module == "" {
			stale = append(stale, struct {
				item     Item
				priority int
				age      time.Duration
			}{Item{Path: r.Path, Name: r.Name, Reason: Stale,
				Question: fmt.Sprintf("%s predates modules and cannot be reviewed until the migration has run.", r.Path)}, -1, 0})
			continue
		}
		present[r.Module+"/"+r.Kind] = true

		man, kind, ok := ruleFor(set, r.Module, r.Kind)
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
		var age time.Duration
		if r.Reviewed.IsZero() {
			age = 1<<62 - 1 // never reviewed sorts before everything that was
		} else {
			age = now.Sub(r.Reviewed)
			if age <= time.Duration(kind.FreshnessDays)*24*time.Hour {
				continue
			}
		}
		stale = append(stale, struct {
			item     Item
			priority int
			age      time.Duration
		}{Item{Path: r.Path, Module: r.Module, Kind: r.Kind, ID: r.ID, Name: r.Name, Reason: Stale,
			Question: render(kind.Interview, r), Revision: revision(r), Snoozes: r.Snoozes}, man.Priority, age})
	}
	sort.SliceStable(stale, func(i, j int) bool {
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

// ruleFor finds the manifest and kind that govern a record: its own module,
// or, for the one crossing kind, the ratified module it is reviewed under.
func ruleFor(set *module.Set, mod, kind string) (module.Manifest, module.Kind, bool) {
	man, ok := set.Module(mod)
	if !ok {
		return module.Manifest{}, module.Kind{}, false
	}
	if bundle, _ := man.Profile.Bundle(); !bundle.Interviewed {
		if target, crosses := crossing[kind]; crosses {
			if tm, ok := set.Module(target); ok {
				if tk, ok := tm.Kinds[kind]; ok {
					return tm, tk, true
				}
			}
		}
		return man, module.Kind{}, true
	}
	k, ok := man.Kinds[kind]
	return man, k, ok
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
