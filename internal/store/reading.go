package store

import (
	"strings"
	"time"
)

const (
	// Open and Behind are derived (§8.1) and never written to a results file.
	Open   ClaimState = "open"
	Behind ClaimState = "behind"
)

// Reading is what a claim's result means today (spec §8.1): the stored
// measurement read against the claim's keys, its deadline and the clock.
type Reading struct {
	State    ClaimState `json:"state"`
	Measured ClaimState `json:"measured"`
	Standing bool       `json:"standing,omitempty"`
	Deadline string     `json:"deadline,omitempty"`
	// Unreadable names what could not be read when State is Behind for that
	// reason: "deadline", "since", "window" (since on or after the deadline)
	// or "effort".
	Unreadable string `json:"unreadable,omitempty"`
	DaysLeft   *int   `json:"days_left,omitempty"`
	Count      *int   `json:"count,omitempty"`
	Target     *int   `json:"target,omitempty"`
	Expected   *int   `json:"expected,omitempty"`
	Effort     *int   `json:"effort,omitempty"`
}

func intp(n int) *int { return &n }

// utcDay is the calendar date of t in t's own zone, as a UTC midnight. All
// date arithmetic runs on these, so a daylight-saving change, even one that
// skips local midnight, cannot shift a day.
func utcDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// days is the whole calendar days from a to b (b − a), negative when b is
// earlier, counted between the dates each time shows in its own zone.
func days(a, b time.Time) int {
	return int((utcDay(b).Unix() - utcDay(a).Unix()) / 86400)
}

// parseDay reads a YYYY-MM-DD date as a UTC midnight.
func parseDay(s string) (time.Time, bool) { return ParseDay(s, time.UTC) }

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
	raise := func(what string) Reading {
		r.State, r.Unreadable = Behind, what
		return r
	}
	deadlineText := strings.TrimSpace(c.By)
	if deadlineText == "" {
		deadlineText = strings.TrimSpace(goalBy)
	}
	r.Deadline = deadlineText
	deadline, ok := parseDay(deadlineText)
	if !ok {
		return raise("deadline")
	}
	today := utcDay(now.In(now.Location()))
	if today.After(deadline) {
		r.State = Fail
		return r
	}
	r.DaysLeft = intp(days(today, deadline) + 1)
	// A paced claim never measured counts as zero done (§8.1); only a result
	// stored before counts existed, which says fail, reads as yes-or-no.
	if target, paced := c.Paced(); paced && (res.Count != nil || measured == Unchecked) {
		count := 0
		if res.Count != nil {
			count = *res.Count
		}
		r.Target = intp(target)
		since, ok := parseDay(c.Args["since"])
		if !ok {
			return raise("since")
		}
		window := days(since, deadline) + 1
		if window <= 0 {
			return raise("window")
		}
		elapsed := days(since, today)
		if elapsed < 0 {
			elapsed = 0
		}
		expected := target * elapsed / window
		r.Expected = intp(expected)
		if count < expected/2 {
			r.State = Behind
			return r
		}
		r.State = Open
		return r
	}
	if c.Effort != "" {
		e, err := EffortDays(c.Effort)
		if err != nil {
			return raise("effort")
		}
		r.Effort = intp(e)
		if *r.DaysLeft <= e {
			r.State = Behind
			return r
		}
	}
	r.State = Open
	return r
}
