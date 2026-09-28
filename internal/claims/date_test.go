package claims

import (
	"context"
	"testing"
	"time"

	"github.com/allanschon/brabeus/internal/store"
)

func TestDatePassesInsideTheWindowAndFailsOutsideIt(t *testing.T) {
	args := map[string]string{"before": "2026-12-31", "after": "2026-01-01"}
	cases := []struct {
		now  time.Time
		want store.ClaimState
	}{
		{time.Date(2025, 12, 15, 9, 0, 0, 0, time.UTC), store.Fail}, // not yet past after
		{time.Date(2026, 1, 1, 23, 0, 0, 0, time.UTC), store.Fail},  // the after day itself is not past it
		{time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC), store.Pass},
		{time.Date(2026, 12, 31, 0, 30, 0, 0, time.UTC), store.Fail}, // the before day has arrived
	}
	for _, c := range cases {
		out, err := Date{}.Check(context.Background(), args, c.now)
		if err != nil || out.State != c.want || out.Detail == "" {
			t.Errorf("%v: %+v %v, want %s", c.now, out, err, c.want)
		}
	}
	out, _ := Date{}.Check(context.Background(), map[string]string{"before": "2026-12-31"}, time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))
	if out.State != store.Pass {
		t.Errorf("before alone: %+v", out)
	}
}
