package main

import (
	"strings"
	"testing"

	"github.com/allanschon/brabeus/internal/module"
)

// /healthz names what was loaded and when claims last ran, so a deployment
// that enabled the wrong set or whose schedule stopped is visible on the one
// endpoint every monitor already reads.
func TestHealthzNamesModulesProfilesAndTheLastClaimsRun(t *testing.T) {
	set := &module.Set{Modules: []module.Manifest{
		{Name: "telos", Profile: module.RatifiedRecord},
		{Name: "memory", Profile: module.WorkingMemory},
	}}
	got := healthz("0.3.0", "tailscale", set, "never", 0)
	want := "ok 0.3.0 identity=tailscale modules=telos,memory profiles=ratified-record,working-memory claims=never\n"
	if got != want {
		t.Errorf("healthz =\n%q\nwant\n%q", got, want)
	}
}

// A goal whose store write fails on every run would otherwise be visible
// only in the kernel's log; /healthz carries the count too, so a monitor
// reading only this line still sees it. A clean run adds nothing, so the
// line a healthy deployment has always shown does not change.
func TestHealthzAddsErrorsOnlyWhenNonZero(t *testing.T) {
	set := &module.Set{Modules: []module.Manifest{
		{Name: "telos", Profile: module.RatifiedRecord},
	}}
	got := healthz("0.3.0", "tailscale", set, "2026-09-20T04:00:00Z", 2)
	want := "ok 0.3.0 identity=tailscale modules=telos profiles=ratified-record claims=2026-09-20T04:00:00Z errors=2\n"
	if got != want {
		t.Errorf("healthz =\n%q\nwant\n%q", got, want)
	}
	if got := healthz("0.3.0", "tailscale", set, "2026-09-20T04:00:00Z", 0); strings.Contains(got, "errors=") {
		t.Errorf("a clean run must not add errors=: %q", got)
	}
}
