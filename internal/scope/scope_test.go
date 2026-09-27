package scope

import "testing"

// visible decides whether a memory reaches the machine asking for it. Getting
// it wrong in one direction leaks another machine's facts; in the other it
// hides everything. Both failures are silent.
func TestVisible(t *testing.T) {
	cases := []struct {
		name       string
		scope      string
		caller     string
		includeAll bool
		want       bool
	}{
		{"global reaches everyone", "global", "beta", false, true},
		{"project scope reaches everyone", "project/example--repo", "gamma", false, true},
		{"a memory with no scope is not machine-scoped", "", "beta", false, true},
		{"a machine sees its own", "machine/beta", "beta", false, true},
		{"another machine's is hidden", "machine/gamma", "beta", false, false},

		// Tailscale reports "Gamma"; scopes written by hand say
		// "gamma". Without normalisation these never match, and the
		// failure is a memory that quietly stops appearing.
		{"tailscale capitalisation still matches", "machine/Gamma", "gamma", false, true},
		{"caller capitalisation still matches", "machine/beta", "Beta", false, true},
		{"surrounding whitespace still matches", "  machine/beta  ", "beta", false, true},

		// An unresolved caller must fail closed. Returning another machine's
		// facts to an unknown caller is the worse of the two errors.
		{"unknown caller sees no machine-scoped memory", "machine/beta", "", false, false},
		{"unknown caller still sees global", "global", "", false, true},

		// "machine/" with nothing after it is malformed. It must not match a
		// caller whose name could not be resolved.
		{"empty host in scope never matches", "machine/", "", false, false},

		{"include_all_scopes overrides", "machine/gamma", "beta", true, true},
		{"include_all_scopes overrides for an unknown caller too", "machine/beta", "", true, true},
	}
	for _, c := range cases {
		if got := Visible(c.scope, c.caller, c.includeAll); got != c.want {
			t.Errorf("%s: visible(%q, %q, %v) = %v, want %v",
				c.name, c.scope, c.caller, c.includeAll, got, c.want)
		}
	}
}

func TestNormaliseHost(t *testing.T) {
	for in, want := range map[string]string{
		"Gamma":    "gamma",
		"  beta  ": "beta",
		"Delta":    "delta",
		"":         "",
	} {
		if got := NormaliseHost(in); got != want {
			t.Errorf("normaliseHost(%q) = %q, want %q", in, got, want)
		}
	}
}
