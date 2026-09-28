package claims

import (
	"context"
	"fmt"
	"time"

	"github.com/allanschon/brabeus/internal/store"
)

// Date is pure: it compares now with the claim's dates. before passes while
// today is before it; after passes once today is past it; both is between.
// Days are UTC, as the dates carry no zone.
type Date struct{}

func (Date) Name() string { return "date" }

func (Date) Check(_ context.Context, args map[string]string, now time.Time) (Outcome, error) {
	day := now.UTC().Truncate(24 * time.Hour)
	if v, ok := args["before"]; ok {
		b, err := time.Parse("2006-01-02", v)
		if err != nil {
			return Outcome{}, fmt.Errorf("before: %q is not YYYY-MM-DD", v)
		}
		if !day.Before(b) {
			return Outcome{State: store.Fail, Detail: "the date " + v + " has arrived"}, nil
		}
	}
	if v, ok := args["after"]; ok {
		a, err := time.Parse("2006-01-02", v)
		if err != nil {
			return Outcome{}, fmt.Errorf("after: %q is not YYYY-MM-DD", v)
		}
		if !day.After(a) {
			return Outcome{State: store.Fail, Detail: "not yet past " + v}, nil
		}
	}
	return Outcome{State: store.Pass, Detail: "within the dates"}, nil
}
