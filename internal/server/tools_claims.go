package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/allanschon/brabeus/internal/agenda"
	"github.com/allanschon/brabeus/internal/scope"
	"github.com/allanschon/brabeus/internal/store"
)

// manualAdapter names the claims the person answers at interview (§8.1).
const manualAdapter = "manual"

// goalByField is the goal field that holds its deadline, which a claim without
// its own deadline reads (spec §8.1).
const goalByField = "by"

// defaultClaimInterval is spec §8.1's "daily by default": the interval a
// pass is judged against when the deployment's is off.
const defaultClaimInterval = 24 * time.Hour

type claimsIn struct {
	Goal string `json:"goal,omitempty" jsonschema:"one goal's path; empty lists every goal you may see"`
}
type claimOut struct {
	Goal    string `json:"goal"`
	ID      string `json:"id,omitempty"`
	Title   string `json:"title,omitempty"`
	Index   int    `json:"index"`
	Text    string `json:"text"`
	Adapter string `json:"adapter"`
	Manual  bool   `json:"manual"`
	State   string `json:"state" jsonschema:"today's state: pass, open, behind, fail, no-evidence or unchecked (spec §8.1)"`
	// Measured is the stored result State was read from.
	Measured   string `json:"measured" jsonschema:"what the last check found: pass, fail, no-evidence or unchecked"`
	Standing   bool   `json:"standing,omitempty"`
	Deadline   string `json:"deadline,omitempty"`
	Unreadable string `json:"unreadable,omitempty" jsonschema:"the value to fix when behind because a date or effort could not be read: deadline, since, window, effort or rolling"`
	DaysLeft   *int   `json:"days_left,omitempty"`
	Count      *int   `json:"count,omitempty"`
	Target     *int   `json:"target,omitempty"`
	Expected   *int   `json:"expected,omitempty"`
	Effort     *int   `json:"effort,omitempty"`
	Detail     string `json:"detail,omitempty"`
	Since      string `json:"since,omitempty" jsonschema:"when this state was entered"`
	Stale      bool   `json:"stale,omitempty" jsonschema:"an adapter's pass older than two claim intervals: the scheduler has not run"`
}
type claimsOut struct {
	LastRun  string     `json:"last_run" jsonschema:"when non-manual claims last ran: an RFC3339 stamp, never, off, or unknown"`
	Interval string     `json:"interval"`
	Claims   []claimOut `json:"claims"`
}

type claimResultIn struct {
	Goal  string `json:"goal" jsonschema:"the goal's path"`
	Index int    `json:"index" jsonschema:"the claim's position on the goal, from 0, as the claims tool lists it"`
	State string `json:"state" jsonschema:"pass, fail or no-evidence"`
	Count *int   `json:"count,omitempty" jsonschema:"for a manual count (a claim with of): the count so far; the state follows from it"`
	Note  string `json:"note,omitempty" jsonschema:"what the person said about the evidence, one line"`
}
type claimResultOut struct {
	Goal   string `json:"goal"`
	Commit string `json:"commit"`
}

// governedByRatified reports whether a record's governing module is one the
// person ratifies: the only records whose claims the kernel reads (§8.1).
// module.Set.Interviewed is the shared predicate; agenda.Reflect uses the
// same one, since agenda cannot import server to call this directly.
func governedByRatified(d Deps, r store.Stored) bool {
	return d.Set.Interviewed(r.Module, r.Kind)
}

func nowFor(d Deps) time.Time {
	if d.Now == nil {
		return time.Now()
	}
	return d.Now()
}

// runStatus says when the schedule last ran, and whether an adapter's pass
// can still be trusted as of now (§8.1: stale after two intervals).
func runStatus(d Deps, at time.Time) (lastRun, interval string, fresh bool) {
	every := d.ClaimInterval
	interval = every.String()
	if every <= 0 {
		interval, every = "off", defaultClaimInterval
	}
	if d.LastRun == nil {
		if d.ClaimInterval <= 0 {
			interval = "unknown"
		}
		return "unknown", interval, false
	}
	last, ok := d.LastRun()
	switch {
	case d.ClaimInterval <= 0:
		lastRun = "off"
	case !ok:
		lastRun = "never"
	default:
		lastRun = last.UTC().Format(time.RFC3339)
	}
	return lastRun, interval, ok && at.Sub(last) <= 2*every
}

// claimsFor lists the claims on every live goal the caller may see, each
// joined to its result. It answers about the person's whole record, so scope
// is checked without a project: a machine-scoped goal is hidden from
// every other machine, and nothing else is.
func claimsFor(d Deps, caller string, audience store.Visibility, goal string) (claimsOut, error) {
	if goal != "" {
		rel, err := store.MemoryPath(goal)
		if err != nil {
			return claimsOut{}, err
		}
		goal = rel
	}
	recs, err := d.Memory.Records()
	if err != nil {
		return claimsOut{}, err
	}
	results, err := d.Memory.ClaimResults()
	if err != nil {
		log.Printf("claim results: %v", err)
	}
	lastRun, interval, fresh := runStatus(d, nowFor(d))
	out := claimsOut{LastRun: lastRun, Interval: interval, Claims: []claimOut{}}
	found := false
	for _, r := range recs {
		if goal != "" && r.Path != goal {
			continue
		}
		if !r.Retired.IsZero() || audience.Hides(r.Module) || !scope.Visible(r.Scope, caller, false) || !governedByRatified(d, r) {
			continue
		}
		block := r.Fields[store.ClaimsField]
		if block == "" {
			continue
		}
		claims, err := store.ParseClaims(block)
		if err != nil {
			log.Printf("claims on %s: %v", r.Path, err)
			continue
		}
		found = true
		for _, res := range store.JoinResults(claims, results[r.Path]) {
			rd := store.Read(claims[res.Index], res, r.Fields[goalByField], nowFor(d))
			c := claimOut{Goal: r.Path, ID: r.ID, Title: r.Fields["title"], Index: res.Index, Text: res.Text,
				Adapter: res.Adapter, Manual: res.Adapter == manualAdapter, State: string(rd.State), Measured: string(rd.Measured),
				Standing: rd.Standing, Deadline: rd.Deadline, Unreadable: rd.Unreadable, DaysLeft: rd.DaysLeft,
				Count: rd.Count, Target: rd.Target, Expected: rd.Expected, Effort: rd.Effort, Detail: res.Detail}
			if !res.Since.IsZero() {
				c.Since = res.Since.UTC().Format(time.RFC3339)
			}
			// The schedule runs adapters only (§8.1), so only an adapter's
			// pass can go stale for want of a run.
			c.Stale = res.State == store.Pass && !c.Manual && !fresh
			out.Claims = append(out.Claims, c)
		}
	}
	if goal != "" && !found {
		return claimsOut{}, fmt.Errorf("no goal with claims at %q that you may see", goal)
	}
	return out, nil
}

type reflectIn struct{}

// reflectFor computes the reflection (spec §9): the gap between the person's
// values and their goals, over the caller's whole visible record — filtered
// as RenderContext filters it, with no project, since the reflection
// answers about the whole person, not one session's view.
func reflectFor(d Deps, caller string, consumer bool, audience store.Visibility) (agenda.Reflection, error) {
	if consumer {
		return agenda.Reflection{}, fmt.Errorf("a consumer is asked nothing and the values are self (spec §11)")
	}
	all, err := d.Memory.Records()
	if err != nil {
		return agenda.Reflection{}, err
	}
	recs := make([]store.Stored, 0, len(all))
	for _, r := range all {
		if !r.Retired.IsZero() || audience.Hides(r.Module) || !scope.Visible(r.Scope, caller, false) {
			continue
		}
		recs = append(recs, r)
	}
	results, err := d.Memory.ClaimResults()
	if err != nil {
		log.Printf("claim results: %v", err)
	}
	ref := agenda.Reflect(d.Set, recs, results, nowFor(d))
	// Only an adapter's pass can go stale for want of a run (§8.1); the
	// claims tool marks it the same way.
	_, _, fresh := runStatus(d, nowFor(d))
	mark := func(goals []agenda.GoalGap) {
		for i := range goals {
			for j := range goals[i].Claims {
				c := &goals[i].Claims[j]
				c.Stale = c.Measured == store.Pass && !c.Manual && !fresh
			}
		}
	}
	for i := range ref.Values {
		mark(ref.Values[i].Goals)
	}
	mark(ref.Unserved)
	return ref, nil
}

// registerReflectTool registers reflect: the gap by value, for the
// interviewer to phrase (§9). It never takes an input; the reflection is
// always about the whole of what the caller may see.
func registerReflectTool(s *mcp.Server, d Deps, caller string, consumer bool, audience store.Visibility) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "reflect",
		Description: "The gap by value, as facts: each value, the goals that serve it with their claim states and " +
			"days since confirmed, then goals serving no value. Phrase it in the person's register; do not recount it yourself. " +
			"no-evidence is a fault in the deployment, not the person being behind (spec §8.1): leave it out of the " +
			"reflection or say the check could not run, never that the person has not been doing the work. " +
			"Only fail and behind are the gap; report open claims as work remaining with their count and days left, " +
			"and no-evidence, unchecked and stale passes as unknown.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in reflectIn) (*mcp.CallToolResult, agenda.Reflection, error) {
		out, err := reflectFor(d, caller, consumer, audience)
		if err != nil {
			return nil, agenda.Reflection{}, err
		}
		return nil, out, nil
	})
}

// recordClaimResult is the claim_result tool: one claim's state as the person
// gave it, through the claim-result operation and never through review
// (§8.1). The merge runs under the store's lock, so a scheduled run landing
// at the same moment keeps its entries and this one keeps its own.
func recordClaimResult(d Deps, caller string, consumer bool, audience store.Visibility, in claimResultIn) (claimResultOut, error) {
	if consumer {
		return claimResultOut{}, fmt.Errorf("a consumer never records a claim result (spec §11)")
	}
	rel, err := store.MemoryPath(in.Goal)
	if err != nil {
		return claimResultOut{}, err
	}
	r, err := gate(d.Memory, rel, caller, false, audience, reviewScopeRefusal)
	if err != nil {
		return claimResultOut{}, err
	}
	if _, meta, _ := d.Memory.Peek(rel); !meta.Retired.IsZero() {
		return claimResultOut{}, fmt.Errorf("%s was retired on %s and has left the record; its claims are no longer checked (spec §9)", rel, meta.Retired.UTC().Format("2006-01-02"))
	}
	block := r.Fields[store.ClaimsField]
	if block == "" {
		return claimResultOut{}, fmt.Errorf("%s carries no claims (spec §8.1)", rel)
	}
	claims, err := store.ParseClaims(block)
	if err != nil {
		return claimResultOut{}, err
	}
	if in.Index < 0 || in.Index >= len(claims) {
		return claimResultOut{}, fmt.Errorf("%s has %d claims, numbered from 0; there is no claim %d", rel, len(claims), in.Index)
	}
	state := store.ClaimState(in.State)
	switch state {
	case store.Pass, store.Fail, store.NoEvidence:
	default:
		return claimResultOut{}, fmt.Errorf("state %q: use pass, fail or no-evidence (spec §8.1)", in.State)
	}
	c, stamp := claims[in.Index], nowFor(d).UTC()
	// An adapter's pass or fail is the kernel's evidence, written only by
	// the scheduled run (§8.1, AT): a session that could set it could clear
	// the one contradiction claims exist to surface.
	if c.Adapter != manualAdapter {
		return claimResultOut{}, fmt.Errorf("claim %d on %s is checked by the %s adapter; its result comes from the scheduled run, and only a manual claim's answer is recorded here (spec §8.1)", in.Index, rel, c.Adapter)
	}
	var count, target *int
	of, paced := c.Paced()
	switch {
	case paced && !c.IsStanding():
		// A manual count's answer is the number; the state is what the count
		// says against the target, so a session cannot pass a short count.
		switch {
		case in.Count == nil && state == store.NoEvidence:
		case in.Count == nil:
			return claimResultOut{}, fmt.Errorf("a manual count's answer is a number: pass count (claim %d on %s counts toward %d)", in.Index, rel, of)
		case state == store.NoEvidence:
			return claimResultOut{}, fmt.Errorf("no-evidence is no answer, so it takes no count; leave count out")
		case *in.Count < 0:
			return claimResultOut{}, fmt.Errorf("a count is zero or more, not %d", *in.Count)
		case *in.Count >= of && state != store.Pass:
			return claimResultOut{}, fmt.Errorf("a count of %d meets the target of %d, so the state is pass, not %s", *in.Count, of, state)
		case *in.Count < of && state != store.Fail:
			return claimResultOut{}, fmt.Errorf("a count of %d is short of the target of %d, so the state is fail, not %s", *in.Count, of, state)
		default:
			count, target = in.Count, &of
		}
	case in.Count != nil && paced:
		return claimResultOut{}, fmt.Errorf("claim %d on %s is standing, and a standing claim holds or does not: its answer is pass or fail, not a count (spec §8.1)", in.Index, rel)
	case in.Count != nil:
		return claimResultOut{}, fmt.Errorf("claim %d on %s has no of in its check, so it is not a count: leave count out, or add of to the claim (spec §8.1)", in.Index, rel)
	}
	commit, err := d.Memory.UpdateClaimResults(rel, func(prev []store.ClaimResult) []store.ClaimResult {
		out := make([]store.ClaimResult, 0, len(prev)+1)
		for _, p := range prev {
			if p.Index != in.Index {
				out = append(out, p)
			}
		}
		// Since is kept by the store when the state is unchanged.
		return append(out, store.ClaimResult{Index: in.Index, Text: c.Text, Adapter: c.Adapter, State: state, Detail: in.Note, Count: count, Target: target, Since: stamp, Recorded: stamp})
	}, caller)
	if err != nil {
		return claimResultOut{}, err
	}
	return claimResultOut{Goal: rel, Commit: commit}, nil
}

// registerClaimTools registers claims (read), claim_result (write) and
// reflect (read).
func registerClaimTools(s *mcp.Server, d Deps, caller string, consumer bool, audience store.Visibility) {
	registerReflectTool(s, d, caller, consumer, audience)
	mcp.AddTool(s, &mcp.Tool{
		Name: "claims",
		Description: "The claims on the person's goals, each with today's state (spec §8.1): pass, open (not met yet, on pace, " +
			"deadline ahead), behind (may miss its deadline), fail (a standing claim not met, or a deadline passed), " +
			"no-evidence or unchecked, and what was last measured. When the scheduled checks last ran is given too, and a " +
			"pass older than two intervals is marked stale. open is never a shortfall; no-evidence is a fault in the deployment.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in claimsIn) (*mcp.CallToolResult, claimsOut, error) {
		out, err := claimsFor(d, caller, audience, in.Goal)
		if err != nil {
			return nil, claimsOut{}, err
		}
		return nil, out, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "claim_result",
		Description: "Record what the person said about a manual claim's evidence — pass, fail or no-evidence — " +
			"without reviewing the goal (spec §8.1). Only manual claims: an adapter's result comes from the " +
			"scheduled run and is refused here. A review of the goal happens only if the person also confirmed " +
			"or corrected the goal itself. For a manual count (a claim with of), pass the count so far.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in claimResultIn) (*mcp.CallToolResult, claimResultOut, error) {
		out, err := recordClaimResult(d, caller, consumer, audience, in)
		if err != nil {
			return nil, claimResultOut{}, err
		}
		return nil, out, nil
	})
}
