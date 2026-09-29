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

// VisibleIn is Visible with a project half added, for the one place that
// needs it: the session block. Everywhere else (search, list, read, claims,
// reflect) answers about the whole record and keeps Visible, because those
// answer a question, not render a project's view.
//
// A machine-scoped memory is judged exactly as Visible judges it. A
// project-scoped memory is judged the other way around from Visible: Visible
// treats every project as visible everywhere, because the pre-M2 server had
// no notion of "the current project" to compare against. Here there is one,
// so a project-scoped memory renders only when it matches the caller's own
// project, and an empty project (no git repository, or one with no origin)
// matches none.
func VisibleIn(sc, caller, project string, includeAll bool) bool {
	if includeAll {
		return true
	}
	rest, isProjectScoped := strings.CutPrefix(NormaliseHost(sc), "project/")
	if !isProjectScoped {
		return Visible(sc, caller, false)
	}
	if rest == "" || project == "" {
		return false
	}
	return rest == NormaliseHost(project)
}

// validSlug is the one predicate both CheckScope (the writer) and
// CheckProjectKey (the reader) use for the half after "project/" or
// "machine/". Spec §5 names <owner>--<repo> as the hook's own convention for
// computing a project key, but does not make "--" part of the scope's
// grammar, and GitHub and Gitea both allow "--" in an owner or repository
// name — so this does not count or split on it. A slug is anything non-empty
// with no "/" (so machine/a/b is not two scopes at once) and no whitespace of
// any kind (oneLine would otherwise rewrite it on the way to disk into a form
// Visible/VisibleIn never matches). Sharing this one rule is what keeps the
// reader from refusing a key the writer already accepted.
func validSlug(s string) bool {
	return s != "" && !strings.Contains(s, "/") && !strings.ContainsFunc(s, unicode.IsSpace)
}

// CheckProjectKey validates a project value arriving over the network — the
// context tool's `project` argument, or GET /context's `?project=` query
// parameter — before it reaches VisibleIn as a filter. It accepts exactly what
// CheckScope accepts for a project scope's slug (validSlug), so a key that
// round-trips through a write also round-trips through the block: a
// repository whose owner or name contains "--" produces a key CheckScope has
// always accepted, and refusing it here only on read would leave that
// project's records writable but never rendered. An empty project is not an
// error: it means no project, exactly as an empty query parameter or a
// missing argument does.
func CheckProjectKey(project string) (string, error) {
	p := NormaliseHost(project)
	if p == "" {
		return "", nil
	}
	if !validSlug(p) {
		return "", fmt.Errorf("project %q is not a valid project scope slug (spec §5)", project)
	}
	return p, nil
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
		if rest, ok := strings.CutPrefix(sc, prefix); ok && validSlug(rest) {
			return sc, nil
		}
	}
	return "", fmt.Errorf("scope %q is not global, project/<slug> or machine/<host>", scope)
}
