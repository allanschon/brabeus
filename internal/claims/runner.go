package claims

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/allanschon/brabeus/internal/store"
)

// Runner answers every non-manual claim on the deployment's interval (spec
// §8.1) and records the results through the store's claim-result operation,
// so the kernel stays the record's only writer and a goal is never opened.
type Runner struct {
	Store    *store.Store
	Adapters map[string]Adapter
	// Interval is the schedule. 0 means never scheduled, for a deployment
	// that runs claims elsewhere; Run may still be called.
	Interval time.Duration
	Now      func() time.Time
	// StatePath holds the last run's stamp, on the data volume, so a
	// restart does not report never.
	StatePath string

	mu   sync.Mutex
	last time.Time
	ran  bool
	read bool
}

// Report counts one run: the goals with claims, their claims, the claims
// whose state changed, and the claims that got no evidence.
type Report struct{ Goals, Claims, Changed, NoEvidence int }

func (r *Runner) now() time.Time {
	if r.Now == nil {
		return time.Now().UTC()
	}
	return r.Now().UTC()
}

// Run evaluates every non-manual claim on every live goal and records the
// results. It never touches a goal, never marks a claim fail without
// evidence, and a run that changes no state commits nothing.
//
// Evidence is gathered before the store's lock is taken, because an adapter
// is a network call; the results are merged into what is recorded at the
// moment of writing, so a manual answer that lands mid-run is kept.
func (r *Runner) Run(ctx context.Context) (Report, error) {
	now := r.now()
	records, err := r.Store.Records()
	if err != nil {
		return Report{}, err
	}
	var rep Report
	for _, rec := range records {
		block := rec.Fields[store.ClaimsField]
		if rec.Module == "" || !rec.Retired.IsZero() || strings.TrimSpace(block) == "" {
			continue
		}
		claims, err := store.ParseClaims(block)
		if err != nil {
			log.Printf("claims: %s: %v", rec.Path, err)
			continue
		}
		rep.Goals++
		rep.Claims += len(claims)
		outcomes := make([]Outcome, len(claims))
		for i, c := range claims {
			if c.Adapter == "manual" {
				continue
			}
			if err := ctx.Err(); err != nil {
				return rep, err
			}
			out, err := r.check(ctx, c, now)
			if err := ctx.Err(); err != nil {
				// "context canceled" is not evidence of anything: stop
				// without recording this goal or stamping a run.
				return rep, err
			}
			if err != nil {
				out = Outcome{State: store.NoEvidence, Detail: err.Error()}
			}
			if out.State == store.NoEvidence {
				rep.NoEvidence++
			}
			outcomes[i] = out
		}
		changed := 0
		if _, err := r.Store.UpdateClaimResults(rec.Path, func(prev []store.ClaimResult) []store.ClaimResult {
			changed = 0
			joined := store.JoinResults(claims, prev)
			for i, c := range claims {
				if c.Adapter == "manual" || joined[i].State == outcomes[i].State {
					// An unchanged state keeps its detail as well as its
					// since, so the count inside "2 found" moving within
					// one state does not churn the results file.
					continue
				}
				joined[i].State, joined[i].Detail = outcomes[i].State, outcomes[i].Detail
				joined[i].Since, joined[i].Recorded = now, now
				changed++
			}
			return joined
		}, "kernel"); err != nil {
			return rep, fmt.Errorf("%s: %w", rec.Path, err)
		}
		rep.Changed += changed
	}
	r.mu.Lock()
	r.last, r.ran, r.read = now, true, true
	r.mu.Unlock()
	if r.StatePath != "" {
		if err := os.WriteFile(r.StatePath, []byte(now.Format(time.RFC3339)+"\n"), 0o640); err != nil {
			log.Printf("claims: saving the last run to %s: %v; a restart will report never until the next run", r.StatePath, err)
		}
	}
	return rep, nil
}

// check asks one adapter. A missing adapter or a state an adapter may not
// answer is no-evidence with the reason (§8.1): a fault in the deployment,
// never the person falling behind, and never a claim skipped.
func (r *Runner) check(ctx context.Context, c store.Claim, now time.Time) (Outcome, error) {
	a := r.Adapters[c.Adapter]
	if a == nil {
		return Outcome{State: store.NoEvidence, Detail: c.Adapter + " not configured on this deployment"}, nil
	}
	out, err := a.Check(ctx, c.Args, now)
	if err != nil {
		return Outcome{}, err
	}
	switch out.State {
	case store.Pass, store.Fail, store.NoEvidence:
		return out, nil
	}
	return Outcome{State: store.NoEvidence, Detail: fmt.Sprintf("%s answered %q, which is not pass, fail or no-evidence", c.Adapter, out.State)}, nil
}

// LastRun is when the last run finished, false for never. The stamp file is
// read once, at the first call, so a restart does not report never.
func (r *Runner) LastRun() (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.read {
		r.read = true
		if b, err := os.ReadFile(r.StatePath); err == nil {
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(b))); err == nil {
				r.last, r.ran = t, true
			}
		}
	}
	return r.last, r.ran
}

// Start runs now and then every Interval until ctx ends. It returns at once
// when Interval is 0. A run in progress when ctx ends stops before its next
// adapter call and records nothing further.
func (r *Runner) Start(ctx context.Context) {
	if r.Interval <= 0 {
		return
	}
	run := func() {
		rep, err := r.Run(ctx)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		if err != nil {
			log.Printf("claims: run failed: %v", err)
			return
		}
		log.Printf("claims: %d goals, %d claims, %d changed, %d no-evidence", rep.Goals, rep.Claims, rep.Changed, rep.NoEvidence)
	}
	run()
	t := time.NewTicker(r.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}

// Status is the claims= field of /healthz: "off" when nothing is scheduled,
// "never" before the first run, otherwise the last run's RFC3339 stamp.
func Status(r *Runner) string {
	if r == nil || r.Interval <= 0 {
		return "off"
	}
	if last, ok := r.LastRun(); ok {
		return last.UTC().Format(time.RFC3339)
	}
	return "never"
}
