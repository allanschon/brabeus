// Package agenda decides what the interview asks first (spec §9). It is a
// pure function of the record and the module set: nothing is stored about
// what was asked, so the kernel stays stateless.
package agenda

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/allanschon/brabeus/internal/instructions"
	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/store"
)

type Reason string

const (
	Fail       Reason = "fail"
	Behind     Reason = "behind"
	Draft      Reason = "draft"
	Stale      Reason = "stale"
	Onboarding Reason = "onboarding"
	Budget     Reason = "budget"
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
	// Body is the delivered text of an instructions record, beside its question, so the interviewer can show it (spec §9).
	Body string `json:"body,omitempty"`
	// ClaimText and ClaimIndex name the failed claim on a fail item. The
	// index is 0-based, as the claims tool lists it, so it is a pointer:
	// the first claim is 0 and must still be on the wire.
	ClaimText  string `json:"claim,omitempty"`
	ClaimIndex *int   `json:"claim_index,omitempty"`
}

// byField is the goal's date (spec §9 orders failed claims by it). A field
// name in the kernel is product semantics, and §9 names this one.
const byField = "by"

// deferral is how long a goal reviewed or put off waits before its behind
// claims rise above the stale tier (spec §9).
const deferral = 7 * 24 * time.Hour

// failCandidate is a claim item with what orders it: its deadline, when
// readable, and since when it has failed.
type failCandidate struct {
	item  Item
	by    time.Time
	byOK  bool
	since time.Time
}

func sortByDeadline(l []failCandidate) {
	sort.SliceStable(l, func(i, j int) bool {
		if l[i].byOK != l[j].byOK {
			return l[i].byOK
		}
		if l[i].byOK && !l[i].by.Equal(l[j].by) {
			return l[i].by.Before(l[j].by)
		}
		return l[i].since.Before(l[j].since)
	})
}

// latest is the later of two times; a zero time means never.
func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// failQuestion names what failed: a standing claim the day it began failing,
// an end-state claim the deadline it missed (spec §9).
func failQuestion(res store.ClaimResult, rd store.Reading) string {
	if rd.Standing {
		return fmt.Sprintf("the claim %q failed on %s%s.", res.Text, res.Since.UTC().Format("2006-01-02"), store.Parenthetical(res.Detail))
	}
	return fmt.Sprintf("the claim %q was due by %s and is not met%s.", res.Text, rd.Deadline, store.Parenthetical(res.Detail))
}

// behindQuestion names the numbers the reading was made from, so the person
// sees why the claim is behind, or which date could not be read (spec §9).
// It uses the reading's numbers, never the stored result's: an unchecked
// paced claim reads as zero done.
func behindQuestion(c store.Claim, rd store.Reading) string {
	switch rd.Unreadable {
	case "deadline":
		return fmt.Sprintf("the claim %q has a deadline %q that could not be read; fix the date.", c.Text, rd.Deadline)
	case "since":
		return fmt.Sprintf("the claim %q has a since %q that could not be read; fix the date.", c.Text, c.Args["since"])
	case "effort":
		return fmt.Sprintf("the claim %q has an effort %q that could not be read; fix it.", c.Text, c.Effort)
	case "rolling":
		return fmt.Sprintf("the claim %q counts a rolling window (since %s) but does not say standing: true; should it hold all the time?", c.Text, c.Args["since"])
	case "window":
		return fmt.Sprintf("the claim %q starts counting on %s, after its deadline %s; fix the dates.", c.Text, c.Args["since"], rd.Deadline)
	}
	if rd.Target != nil && rd.Expected != nil {
		unit := "found"
		if c.Adapter == store.AdapterManual {
			unit = "done"
		}
		return fmt.Sprintf("the claim %q is behind: %d of %d %s, %d expected by now, %d days left.",
			c.Text, deref(rd.Count), *rd.Target, unit, *rd.Expected, deref(rd.DaysLeft))
	}
	return fmt.Sprintf("the claim %q is behind: %d days left, about %d days of work.", c.Text, deref(rd.DaysLeft), deref(rd.Effort))
}

func parseDay(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	return t, err == nil
}

type candidate struct {
	item     Item
	priority int
	age      time.Duration
	pref     bool // preferences sort last within each reason (§9)
}

// Compute orders the agenda (spec §9): claims in fail first, by the claim's
// own deadline then by how long each has failed, then claims behind schedule
// by deadline, then drafts — records never
// reviewed — oldest first, then records past their kind's freshness or
// due_field by module priority then age, then the behind claims of goals
// reviewed or put off within a week (deferred), then the onboarding gaps — kinds a
// module wants on file and has none of. Within the drafts and stale tiers,
// preferences sort last, native and crossing alike. results are the claim
// results keyed by goal path (store.ClaimResults); nil means none.
//
// A record with a failed or behind claim is still eligible for the draft and
// stale tiers: the same goal can appear twice, once per reason, the claim
// first (or, when deferred, after stale). An open claim is never on the agenda.
// no-evidence never reaches the agenda (§8.1): it is a fault, not the
// person being behind.
func Compute(set *module.Set, records []store.Stored, results map[string][]store.ClaimResult, now time.Time) []Item {
	var drafts, stale []candidate
	var fails, behind, deferred []failCandidate
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

		// Before the draft check, which ends this record's turn: a draft
		// goal's failed or behind claim still counts.
		if block := r.Fields[store.ClaimsField]; block != "" {
			if claims, err := store.ParseClaims(block); err == nil {
				// A goal put off or reviewed within a week keeps its behind
				// claims below the stale tier; the clock is the later of the
				// two stamps (§9). A failed claim is never deferred.
				recent := now.Sub(latest(r.Reviewed, r.Snoozed)) < deferral
				for _, res := range store.JoinResults(claims, results[r.Path]) {
					reading := store.Read(claims[res.Index], res, r.Fields[byField], now)
					if reading.State != store.Fail && reading.State != store.Behind {
						continue
					}
					item := base
					item.Fields = copyFields(r.Fields) // its own copy, as the draft or stale item has
					index := res.Index
					item.ClaimText, item.ClaimIndex = res.Text, &index
					deadline, deadlineOK := parseDay(reading.Deadline)
					if reading.Standing {
						deadline, deadlineOK = parseDay(r.Fields[byField])
					}
					cand := failCandidate{item: item, by: deadline, byOK: deadlineOK, since: res.Since}
					if reading.State == store.Fail {
						cand.item.Reason = Fail
						cand.item.Question = failQuestion(res, reading) + " " + followUp(kind, r)
						fails = append(fails, cand)
						continue
					}
					cand.item.Reason = Behind
					cand.item.Question = behindQuestion(claims[res.Index], reading) + " " + followUp(kind, r)
					if recent {
						deferred = append(deferred, cand)
					} else {
						behind = append(behind, cand)
					}
				}
			}
		}

		// Set before the draft and stale branches copy base.
		if k, ok := set.InstructionKind(r.Module, r.Kind); ok {
			base.Body = store.Delivered(r.Record, k)
		}
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

	// Fails and behind: the claim's own deadline, nearest first; a deadline
	// that does not parse sorts after every one that does rather than
	// breaking the order; then longest failing first (§9).
	for _, list := range [][]failCandidate{fails, behind, deferred} {
		sortByDeadline(list)
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
	for _, c := range fails {
		out = append(out, c.item)
	}
	for _, c := range behind {
		out = append(out, c.item)
	}
	for _, c := range drafts {
		out = append(out, c.item)
	}
	for _, c := range stale {
		out = append(out, c.item)
	}
	for _, c := range deferred {
		out = append(out, c.item)
	}
	for _, man := range set.Modules {
		for _, k := range man.Onboarding {
			if !present[man.Name+"/"+k] {
				out = append(out, Item{Module: man.Name, Kind: k, Reason: Onboarding, Question: man.Kinds[k].First})
			}
		}
	}
	// Instructions over their budget come last: housekeeping, raised only
	// when nothing else is due, and never deferred, because the item names a
	// module rather than a record (spec §9).
	for _, sz := range instructions.Sizes(set, instructions.Collect(set, records)) {
		if sz.Bytes > sz.Budget {
			out = append(out, Item{Module: sz.Module, Reason: Budget,
				Question: fmt.Sprintf("The %s instructions every session receives are %d of %d bytes. Which can be merged or retired?", sz.Module, sz.Bytes, sz.Budget)})
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
// Fields cannot edit the record it was computed from.
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

// followUp is the question asked after a failed claim. A goal the person has
// confirmed is asked its interview question, whether it is still right. A
// draft is asked its draft question instead, because it has no review to
// measure progress from: the interview question would render {reviewed} as
// "never" and read as "nothing has ever been done" (spec §9).
func followUp(kind module.Kind, r store.Stored) string {
	if r.Reviewed.IsZero() {
		return render(orDefault(kind.Draft, DefaultDraft), r)
	}
	return render(orDefault(kind.Interview, DefaultInterview), r)
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

// revision names a change since the last review. store.Revision reads git
// history for the field-level line ("target lowered from 3 to 2 on
// 2026-09-04", spec §9); Records fills it only where updated follows
// reviewed. When that call failed, or was never run, this falls back to the
// M1 stamp wording rather than to nothing, so a git failure degrades the
// agenda line instead of erasing it.
func revision(r store.Stored) string {
	if r.Revision != "" {
		return r.Revision
	}
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
