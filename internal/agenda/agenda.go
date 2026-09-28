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
	Fail       Reason = "fail"
	Draft      Reason = "draft"
	Stale      Reason = "stale"
	Onboarding Reason = "onboarding"
)

// The questions a kind is asked when it declares none (spec §6): so a new or
// third-party module never produces an empty first line.
const (
	DefaultInterview = "Is this still right?"
	DefaultDraft     = "Is this right as written?"
)

type Item struct {
	Path     string            `json:"path,omitempty"`
	Module   string            `json:"module,omitempty"`
	Kind     string            `json:"kind,omitempty"`
	ID       string            `json:"id,omitempty"`
	Name     string            `json:"name,omitempty"`
	Reason   Reason            `json:"reason,omitempty"`
	Question string            `json:"question,omitempty"`
	Revision string            `json:"revision,omitempty"`
	Snoozes  int               `json:"snoozes,omitempty"`
	Fields   map[string]string `json:"fields,omitempty"`
}

type candidate struct {
	item     Item
	priority int
	age      time.Duration
	pref     bool // preferences sort last within each reason (§9)
}

// Compute orders the agenda (spec §9): claims in fail first (M2; none yet
// here), then drafts — records never reviewed — oldest first, then records
// past their kind's freshness or due_field by module priority then age, then
// the onboarding gaps — kinds a module wants on file and has none of. Within
// the drafts and stale tiers, preferences sort last, native and crossing
// alike.
func Compute(set *module.Set, records []store.Stored, now time.Time) []Item {
	var drafts, stale []candidate
	present := map[string]bool{} // module/kind with at least one live record

	for _, r := range records {
		if !r.Retired.IsZero() {
			continue
		}
		if len(r.Malformed) > 0 || r.Module == "" {
			stale = append(stale, fixByHand(r))
			continue
		}
		present[r.Module+"/"+r.Kind] = true

		man, kind, ok := set.RuleFor(r.Module, r.Kind)
		if !ok {
			continue
		}
		if bundle, _ := man.Profile.Bundle(); !bundle.Interviewed {
			continue
		}
		// The governing module's kind is on file too: a memory/preference
		// satisfies identity's onboarding for preference, or the person is
		// asked "how do you want to be worked with" beside the record that
		// already says so.
		present[man.Name+"/"+r.Kind] = true

		base := Item{Path: r.Path, Module: r.Module, Kind: r.Kind, ID: r.ID, Name: r.Name, Fields: copyFields(r.Fields), Revision: revision(r), Snoozes: r.Snoozes}
		_, pref := module.Crossing[r.Kind]

		if r.Reviewed.IsZero() {
			base.Reason, base.Question = Draft, render(orDefault(kind.Draft, DefaultDraft), r)
			drafts = append(drafts, candidate{base, man.Priority, now.Sub(r.Updated), pref})
			continue
		}
		due := false
		if kind.FreshnessDays > 0 && now.Sub(r.Reviewed) > time.Duration(kind.FreshnessDays)*24*time.Hour {
			due = true
		}
		if kind.DueField != "" {
			if d, err := time.Parse("2006-01-02", r.Fields[kind.DueField]); err == nil && d.Before(now) && r.Reviewed.Before(d) {
				due = true
			}
		}
		if !due {
			continue
		}
		base.Reason, base.Question = Stale, render(orDefault(kind.Interview, DefaultInterview), r)
		stale = append(stale, candidate{base, man.Priority, now.Sub(r.Reviewed), pref})
	}

	// Drafts: oldest first, preferences last. Stale: priority, then age, preferences last.
	sort.SliceStable(drafts, func(i, j int) bool {
		if drafts[i].pref != drafts[j].pref {
			return !drafts[i].pref
		}
		return drafts[i].age > drafts[j].age
	})
	sort.SliceStable(stale, func(i, j int) bool {
		if stale[i].pref != stale[j].pref {
			return !stale[i].pref
		}
		if stale[i].priority != stale[j].priority {
			return stale[i].priority < stale[j].priority
		}
		return stale[i].age > stale[j].age
	})

	var out []Item
	for _, c := range drafts {
		out = append(out, c.item)
	}
	for _, c := range stale {
		out = append(out, c.item)
	}
	for _, man := range set.Modules {
		for _, k := range man.Onboarding {
			if !present[man.Name+"/"+k] {
				out = append(out, Item{Module: man.Name, Kind: k, Reason: Onboarding, Question: man.Kinds[k].First})
			}
		}
	}
	return out
}

// fixByHand surfaces a record the kernel cannot interview normally: its
// kernel keys failed to parse, or it predates modules. Either way it is a
// stale item asking to fix the file by hand, with priority -1 so it leads
// its tier — unchanged from M1.
//
// A kernel key that failed to parse (store.Meta.Malformed) means the stamp
// cannot be trusted: treating it as "never reviewed" would render {reviewed}
// as a lie, and every verdict Review is given would be refused by the same
// Malformed check, so the item could never clear.
func fixByHand(r store.Stored) candidate {
	q := fmt.Sprintf("%s predates modules and cannot be reviewed until the migration has run.", r.Path)
	if len(r.Malformed) > 0 {
		q = fmt.Sprintf("%s: %s malformed; fix the file by hand before it can be reviewed.", r.Path, strings.Join(r.Malformed, ", "))
	}
	item := Item{Path: r.Path, Name: r.Name, Module: r.Module, Kind: r.Kind, Reason: Stale, Fields: copyFields(r.Fields), Question: q}
	return candidate{item, -1, 0, false}
}

// copyFields clones a record's field map so a caller editing an item's
// Fields cannot edit the record it was computed from (the M1 ledger).
func copyFields(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// orDefault returns prompt, or def when the kind declared none.
func orDefault(prompt, def string) string {
	if strings.TrimSpace(prompt) == "" {
		return def
	}
	return prompt
}

// render fills {reviewed} and any {field} in a kind's prompt.
func render(prompt string, r store.Stored) string {
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
