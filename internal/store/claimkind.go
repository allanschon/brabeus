package store

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The adapters whose names carry product meaning here (spec §8.1): which
// claims count completed work, and which are standing by nature.
const (
	AdapterManual  = "manual"
	AdapterTracker = "tracker"
	AdapterForge   = "forge"
	AdapterDate    = "date"
)

var (
	effortPattern  = regexp.MustCompile(`^([1-9][0-9]*)d$`)
	rollingPattern = regexp.MustCompile(`^-[1-9][0-9]*d$`)
)

// IsStanding reports whether a claim must hold all the time (§8.1): it says
// so, or it is a date claim, whose before and after are already its dates.
func (c Claim) IsStanding() bool { return c.Standing || c.Adapter == AdapterDate }

// Paced reports a claim whose progress the kernel can measure, and the total
// it is measured against: a count of completed work with a min of 2 or more,
// or a manual count with of. A tracker count of open tasks is not progress,
// because it can fall; a min of 1 is done or not done (§8.1).
func (c Claim) Paced() (int, bool) {
	switch c.Adapter {
	case AdapterTracker, AdapterForge:
		if c.Adapter == AdapterTracker && c.Args["done"] == "false" {
			return 0, false
		}
		n, err := strconv.Atoi(c.Args["min"])
		if err != nil || n < 2 {
			return 0, false
		}
		return n, true
	case AdapterManual:
		n, err := strconv.Atoi(c.Args["of"])
		if err != nil || n < 1 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// EffortDays reads an effort, <n>d with n of at least 1.
func EffortDays(s string) (int, error) {
	m := effortPattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("effort %q is not a number of days written <n>d", s)
	}
	return strconv.Atoi(m[1])
}

// ParseDay reads YYYY-MM-DD as the start of that day in loc.
func ParseDay(s string, loc *time.Location) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(s), loc)
	return t, err == nil
}

func relativeSince(s string) bool { return rollingPattern.MatchString(strings.TrimSpace(s)) }

// checkClaimKeys applies the rules on the claim-level keys (§8.1). goalBy is
// the goal's by field. A rule broken is refused at write, naming the claim.
func checkClaimKeys(c Claim, goalBy string) error {
	standing := c.IsStanding()
	if standing && c.By != "" {
		return fmt.Errorf("a standing claim takes no by, because it has no deadline")
	}
	if standing && c.Effort != "" {
		return fmt.Errorf("a standing claim takes no effort, because it has no deadline to work toward")
	}
	if relativeSince(c.Args["since"]) && !c.Standing {
		return fmt.Errorf("a rolling since (%s) measures a rate, so the claim must say standing: true", c.Args["since"])
	}
	if c.Effort != "" {
		if _, err := EffortDays(c.Effort); err != nil {
			return err
		}
		if _, paced := c.Paced(); paced {
			return fmt.Errorf("effort belongs only to a yes-or-no claim; this claim's pace is measured")
		}
	}
	if standing {
		return nil
	}
	goalDay, goalOK := ParseDay(goalBy, time.UTC)
	if c.By != "" {
		d, ok := ParseDay(c.By, time.UTC)
		if !ok {
			return fmt.Errorf("by %q is not a date (YYYY-MM-DD)", c.By)
		}
		if goalOK && d.After(goalDay) {
			return fmt.Errorf("by %s is later than the goal's by %s", c.By, goalBy)
		}
		return nil
	}
	if !goalOK {
		return fmt.Errorf("the claim takes its deadline from the goal's by, and %q is not a date (YYYY-MM-DD)", goalBy)
	}
	return nil
}
