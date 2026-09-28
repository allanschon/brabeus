package agenda

import (
	"sort"
	"strings"
	"time"

	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/store"
)

// The value and goal kinds the reflection is computed over (spec §7, §9): the
// person's values and the goals that name them in serves.
const (
	valueModule = "identity"
	valueKind   = "value"
	goalKind    = "goal"
	servesField = "serves"
	titleField  = "title"
)

// Reflection is the gap by value (spec §9): facts only, for the interviewer to phrase.
type Reflection struct {
	Values   []ValueGap `json:"values"`
	Unserved []GoalGap  `json:"unserved" jsonschema:"goals that name no value, or only names that match none"`
	Unknown  []string   `json:"unknown,omitempty" jsonschema:"names in a serves list that match no live value"`
}
type ValueGap struct {
	Name      string    `json:"name"`
	Statement string    `json:"statement"`
	Confirmed bool      `json:"confirmed"`
	Goals     []GoalGap `json:"goals"`
}
type GoalGap struct {
	Path               string     `json:"path"`
	ID                 string     `json:"id,omitempty"`
	Title              string     `json:"title"`
	By                 string     `json:"by,omitempty"`
	DaysSinceConfirmed int        `json:"days_since_confirmed"` // -1 when never
	Claims             []ClaimGap `json:"claims,omitempty"`
}
type ClaimGap struct {
	Text   string           `json:"text"`
	State  store.ClaimState `json:"state"`
	Since  string           `json:"since,omitempty"`
	Manual bool             `json:"manual"`
}

// valueName is the value's slug, taken from its path rather than its name
// field: a goal's serves list names values by slug (D5), and the path's
// final segment without its extension is the one identifier every value
// record has, unlike a free-text name.
func valueName(path string) string {
	base := path
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	return strings.TrimSuffix(base, ".md")
}

// daysSince is DaysSinceConfirmed: -1 when the record has never been
// reviewed, since "0 days" and "never" are not the same fact.
func daysSince(now time.Time, reviewed time.Time) int {
	if reviewed.IsZero() {
		return -1
	}
	return int(now.Sub(reviewed).Hours() / 24)
}

// claimGaps joins a goal's claims block to its results (store.JoinResults,
// C7: the one join the agenda, the claims tool and the runner all share) and
// renders each as a fact the interviewer can phrase.
func claimGaps(fields map[string]string, results []store.ClaimResult) []ClaimGap {
	block := fields[store.ClaimsField]
	if block == "" {
		return nil
	}
	claims, err := store.ParseClaims(block)
	if err != nil {
		return nil
	}
	var out []ClaimGap
	for _, res := range store.JoinResults(claims, results) {
		// "manual" names the claims the person answers at interview (§8.1);
		// the server package holds the same constant, unreachable here
		// because agenda cannot import server.
		g := ClaimGap{Text: res.Text, State: res.State, Manual: res.Adapter == "manual"}
		if !res.Since.IsZero() {
			g.Since = res.Since.UTC().Format(time.RFC3339)
		}
		out = append(out, g)
	}
	return out
}

// goalGap builds one goal's facts.
func goalGap(r store.Stored, now time.Time, results []store.ClaimResult) GoalGap {
	return GoalGap{
		Path: r.Path, ID: r.ID, Title: r.Fields[titleField], By: r.Fields[byField],
		DaysSinceConfirmed: daysSince(now, r.Reviewed),
		Claims:             claimGaps(r.Fields, results),
	}
}

// liveValues returns the live identity/value records in review order
// (confirmed first, most recently reviewed first; then unreviewed by most
// recently updated), via store.LessByReview — the comparator block.Data.Kind
// also uses (C7), so agenda need not import block, which imports agenda, to
// share it.
func liveValues(records []store.Stored) []store.Stored {
	var out []store.Stored
	for _, r := range records {
		if r.Retired.IsZero() && r.Module == valueModule && r.Kind == valueKind {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return store.LessByReview(out[i], out[j]) })
	return out
}

// Reflect computes the gap by value (spec §9): for each live value, the live
// goals that serve it with their claim states and days since confirmed; then
// the goals that name no value, or only names that match none. records is the
// caller's whole visible record (K14); results are the claim results keyed by
// goal path.
func Reflect(set *module.Set, records []store.Stored, results map[string][]store.ClaimResult, now time.Time) Reflection {
	values := liveValues(records)
	ref := Reflection{Values: make([]ValueGap, len(values))}
	index := make(map[string]int, len(values)) // lower-cased name -> position in ref.Values
	for i, v := range values {
		name := valueName(v.Path)
		ref.Values[i] = ValueGap{Name: name, Statement: v.Fields["statement"], Confirmed: !v.Reviewed.IsZero()}
		index[strings.ToLower(name)] = i
	}

	unknown := map[string]bool{}
	for _, r := range records {
		if !r.Retired.IsZero() || r.Kind != goalKind {
			continue
		}
		if _, _, ok := set.RuleFor(r.Module, r.Kind); !ok {
			continue // not governed by a ratified module
		}
		gap := goalGap(r, now, results[r.Path])
		matched := false
		for _, name := range strings.Split(r.Fields[servesField], ",") {
			name = strings.ToLower(strings.TrimSpace(name))
			if name == "" {
				continue
			}
			if i, ok := index[name]; ok {
				ref.Values[i].Goals = append(ref.Values[i].Goals, gap)
				matched = true
			} else {
				unknown[name] = true
			}
		}
		if !matched {
			ref.Unserved = append(ref.Unserved, gap)
		}
	}
	for name := range unknown {
		ref.Unknown = append(ref.Unknown, name)
	}
	sort.Strings(ref.Unknown)
	return ref
}
