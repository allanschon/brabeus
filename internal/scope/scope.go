package scope

import (
	"fmt"
	"strings"
	"unicode"
)

// normaliseHost lowercases and trims a machine name.
//
// Tailscale reports "Desk" and "Laptop" while every scope written by hand says
// "desk" and "laptop". Comparing them raw is a silent mismatch: the memory
// simply stops appearing, with no error anywhere.
func NormaliseHost(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// visible reports whether a memory with this scope should be returned to this
// caller.
//
// Machine-scoped memories are hidden from every other machine, and from a
// caller whose machine could not be resolved. That direction is deliberate: of
// the two ways to be wrong, handing one machine's facts to an unknown caller is
// the worse.
func Visible(scope, caller string, includeAll bool) bool {
	if includeAll {
		return true
	}
	host, isMachineScoped := strings.CutPrefix(NormaliseHost(scope), "machine/")
	if !isMachineScoped {
		return true // global and project scopes are for everyone
	}
	if host == "" {
		return false // malformed scope matches nothing
	}
	return host == NormaliseHost(caller)
}

// checkScope accepts global, project/<slug> and machine/<host>, and nothing
// else, and returns the form visible() compares: lowercased and trimmed.
// Scope is enforced on read, so a scope that does not parse is not an error
// there: "desk" is quietly global and "machine/" is quietly invisible to
// everyone. The write is the one place to refuse it.
func CheckScope(scope string) (string, error) {
	sc := NormaliseHost(scope)
	if sc == "global" {
		return sc, nil
	}
	for _, prefix := range []string{"project/", "machine/"} {
		rest, ok := strings.CutPrefix(sc, prefix)
		// No slash, so machine/a/b is not two scopes at once, and no
		// whitespace of any kind: oneLine would rewrite it on the way to
		// disk into a form visible() never matches.
		if ok && rest != "" && !strings.Contains(rest, "/") && !strings.ContainsFunc(rest, unicode.IsSpace) {
			return sc, nil
		}
	}
	return "", fmt.Errorf("scope %q is not global, project/<slug> or machine/<host>", scope)
}
