package scope

import (
	"strings"
	"testing"
)

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

// visibleIn is Visible plus a project half: the block filters by the
// caller's own project, not by every project at once, so a record scoped to
// someone else's repository must not render just because Visible would let
// every project through.
func TestVisibleIn(t *testing.T) {
	cases := []struct {
		name       string
		scope      string
		caller     string
		project    string
		includeAll bool
		want       bool
	}{
		{"global reaches everyone, no project needed", "global", "beta", "", false, true},
		{"a memory with no scope is not project-scoped", "", "beta", "", false, true},
		{"machine scope is unaffected by project", "machine/beta", "beta", "example--repo", false, true},
		{"another machine's is still hidden with a project set", "machine/gamma", "beta", "example--repo", false, false},

		{"a project record renders inside its own project", "project/example--repo", "beta", "example--repo", false, true},
		{"a project record is hidden outside its project", "project/example--repo", "beta", "other--repo", false, false},
		{"a project record is hidden when no project is asked for", "project/example--repo", "beta", "", false, false},

		{"tailscale capitalisation still matches on the project half", "project/Example--Repo", "beta", "example--repo", false, true},

		{"include_all_scopes overrides a project mismatch", "project/example--repo", "beta", "other--repo", true, true},
	}
	for _, c := range cases {
		if got := VisibleIn(c.scope, c.caller, c.project, c.includeAll); got != c.want {
			t.Errorf("%s: visibleIn(%q, %q, %q, %v) = %v, want %v",
				c.name, c.scope, c.caller, c.project, c.includeAll, got, c.want)
		}
	}
}

// checkProjectKey is the boundary check for a project value arriving over the
// network (the context tool's argument, or /context's query parameter). It
// must accept exactly what CheckScope already accepts for a project scope's
// slug — otherwise a repository whose owner or name contains "--" (legal on
// both GitHub and Gitea) gets a key the writer stores but the reader refuses,
// so the record is written yet never renders under its own project.
func TestCheckProjectKey(t *testing.T) {
	if got, err := CheckProjectKey(""); got != "" || err != nil {
		t.Errorf("empty project must pass through as none: got=%q err=%v", got, err)
	}
	for _, good := range []string{"example--repo", "  Example--Repo  ", "example", "--repo", "example--", "a--b--c", "acme--my--tool"} {
		if _, err := CheckProjectKey(good); err != nil {
			t.Errorf("%q must be accepted: CheckScope already accepts project/%s (%v)", good, strings.TrimSpace(good), err)
		}
	}
	if got, err := CheckProjectKey("  Example--Repo  "); got != "example--repo" || err != nil {
		t.Errorf("a well-formed project is normalised: got=%q err=%v", got, err)
	}
	for _, bad := range []string{"own/er--repo", "example repo--x"} {
		if _, err := CheckProjectKey(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		} else if !strings.Contains(err.Error(), "§5") {
			t.Errorf("%q: refusal must name the spec section: %v", bad, err)
		}
	}
}

// The reader accepts exactly what the writer accepts: CheckProjectKey must
// never refuse a slug CheckScope would let a write use as project/<slug>.
func TestCheckProjectKeyAgreesWithCheckScope(t *testing.T) {
	for _, slug := range []string{"example--repo", "example", "acme--my--tool", "a--b--c"} {
		if _, err := CheckScope("project/" + slug); err != nil {
			t.Fatalf("test fixture: CheckScope(project/%s) = %v, want accepted", slug, err)
		}
		if _, err := CheckProjectKey(slug); err != nil {
			t.Errorf("CheckScope accepts project/%s but CheckProjectKey refuses %q: %v", slug, slug, err)
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
