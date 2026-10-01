package store

import "time"

const (
	// Open and Behind are derived (§8.1) and never written to a results file.
	Open   ClaimState = "open"
	Behind ClaimState = "behind"
)

// Reading is what a claim's result means today (spec §8.1): the stored
// measurement read against the claim's keys, its deadline and the clock.
type Reading struct {
	State          ClaimState `json:"state"`
	Measured       ClaimState `json:"measured"`
	Standing       bool       `json:"standing,omitempty"`
	Deadline       string     `json:"deadline,omitempty"`
	DateUnreadable bool       `json:"date_unreadable,omitempty"`
	DaysLeft       *int       `json:"days_left,omitempty"`
	Count          *int       `json:"count,omitempty"`
	Target         *int       `json:"target,omitempty"`
	Expected       *int       `json:"expected,omitempty"`
	Effort         *int       `json:"effort,omitempty"`
}

func intp(n int) *int { return &n }

// day is the calendar date of t in loc, as midnight.
func day(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// days is the whole calendar days from a to b (b − a), negative when b is
// earlier. Each time's date is taken in its own location and compared as UTC
// midnights, so a daylight-saving change (a 23- or 25-hour day) cannot move
// the count and a negative span is exact.
func days(a, b time.Time) int {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	a2 := time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC)
	b2 := time.Date(by, bm, bd, 0, 0, 0, 0, time.UTC)
	return int(b2.Sub(a2).Hours()) / 24
}

// Read derives a claim's state today. Dates are calendar dates in now's
// location, which is the kernel's TZ (§8.1). Where the arithmetic cannot run,
// the claim is raised as behind rather than hidden.
func Read(c Claim, res ClaimResult, goalBy string, now time.Time) Reading {
	measured := res.State
	if measured == "" {
		measured = Unchecked
	}
	r := Reading{Measured: measured, Standing: c.IsStanding(), Count: res.Count, Target: res.Target}
	if r.Standing {
		r.State = measured
		return r
	}
	switch measured {
	case Pass, NoEvidence:
		r.State = measured
		return r
	}
	// Not met: fail or unchecked, which for an end-state claim counts as not met.
	loc := now.Location()
	deadlineText := c.By
	if deadlineText == "" {
		deadlineText = goalBy
	}
	r.Deadline = deadlineText
	deadline, ok := ParseDay(deadlineText, loc)
	if !ok {
		r.State, r.DateUnreadable = Behind, true
		return r
	}
	today := day(now, loc)
	if today.After(deadline) {
		r.State = Fail
		return r
	}
	r.DaysLeft = intp(days(today, deadline) + 1)
	if target, paced := c.Paced(); paced && res.Count != nil {
		r.Target = intp(target)
		since, ok := ParseDay(c.Args["since"], loc)
		if !ok {
			r.State, r.DateUnreadable = Behind, true
			return r
		}
		window := days(since, deadline) + 1
		if window <= 0 {
			r.State = Behind
			return r
		}
		elapsed := days(since, today)
		if elapsed < 0 {
			elapsed = 0
		}
		expected := target * elapsed / window
		r.Expected = intp(expected)
		if *res.Count < expected/2 {
			r.State = Behind
			return r
		}
		r.State = Open
		return r
	}
	if c.Effort != "" {
		if e, err := EffortDays(c.Effort); err == nil {
			r.Effort = intp(e)
			if *r.DaysLeft <= e {
				r.State = Behind
				return r
			}
		}
	}
	r.State = Open
	return r
}
