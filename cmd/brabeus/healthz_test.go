package main

import (
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
	got := healthz("0.3.0", "tailscale", set, "never")
	want := "ok 0.3.0 identity=tailscale modules=telos,memory profiles=ratified-record,working-memory claims=never\n"
	if got != want {
		t.Errorf("healthz =\n%q\nwant\n%q", got, want)
	}
}
