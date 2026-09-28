package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/allanschon/brabeus/internal/identity"
	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/scope"
	"github.com/allanschon/brabeus/internal/store"
)

// The scope-refusal wording gate hands to each caller: read and delete point
// at include_all_scopes, which exists on those tools; review has no such
// parameter, so it points at the machine instead.
const readScopeRefusal = "%s is scoped %q and you are %q; pass include_all_scopes to read it anyway"

// ParseConsumers reads BRABEUS_CONSUMERS: the machine names that are not the
// person's own sessions. Spec §11: a consumer sees only modules whose
// audience is any, and never writes.
func ParseConsumers(spec string) map[string]bool {
	out := map[string]bool{}
	for _, n := range strings.Split(spec, ",") {
		if n = scope.NormaliseHost(n); n != "" {
			out[n] = true
		}
	}
	return out
}

// Caller resolves who is calling and whether they are a consumer. A caller
// identity cannot name is a consumer: it already sees only global scope, and
// unnamed is not a class the person's record should trust.
func Caller(id identity.Identity, consumers map[string]bool, r *http.Request) (string, bool) {
	caller := scope.NormaliseHost(id.Machine(r))
	return caller, caller == "" || consumers[caller]
}

// audienceFor is the hide-set spec §11 requires: for a consumer, every module
// not declared audience any, and every file the migration has not tagged.
func audienceFor(set *module.Set, consumer bool) store.Visibility {
	v := store.Visibility{HideModules: map[string]bool{}}
	if !consumer {
		return v
	}
	v.HideUntagged = true
	for _, m := range set.Modules {
		if m.Audience != module.Any {
			v.HideModules[strings.ToLower(m.Name)] = true
		}
	}
	return v
}

// hiddenFor folds the search profile (§1.1) into the audience: by default
// ratified-record modules are searched only when asked, and naming a module
// asks for it. Audience is never overridden.
func hiddenFor(set *module.Set, consumer bool, profile, named string) (store.Visibility, error) {
	v := audienceFor(set, consumer)
	named = strings.ToLower(strings.TrimSpace(named))
	var hideProfile module.Profile
	switch profile {
	case "", "working-memory":
		hideProfile = module.RatifiedRecord
	case "ratified-record":
		hideProfile = module.WorkingMemory
	case "all":
	default:
		return v, fmt.Errorf("profile %q: use working-memory (default), ratified-record or all", profile)
	}
	for _, m := range set.Modules {
		if hideProfile != "" && m.Profile == hideProfile && strings.ToLower(m.Name) != named {
			v.HideModules[strings.ToLower(m.Name)] = true
		}
	}
	return v, nil
}

// gate is the shared read-before-touch check for read, delete and review:
// missing file, then audience, then scope, in that order. refusalFmt takes
// the path, the record's scope and the caller, in that order, and is the one
// place the three tools' wording differs.
//
// Audience before scope: the scope refusal names the record's machine, which
// a consumer should not learn about a record it may not read at all — a
// consumer asking after a machine-scoped record in a hidden module must get
// the audience wording, never the scope one.
func gate(st *store.Store, rel, caller string, includeAll bool, audience store.Visibility, refusalFmt string) (store.Record, error) {
	r, _, ok := st.Peek(rel)
	if !ok {
		return store.Record{}, fmt.Errorf("no record at %q", rel)
	}
	if audience.Hides(r.Module) {
		return store.Record{}, fmt.Errorf("%s is not readable by this caller", rel)
	}
	if !scope.Visible(r.Scope, caller, includeAll) {
		return store.Record{}, fmt.Errorf(refusalFmt, rel, r.Scope, caller)
	}
	return r, nil
}

// authMiddleware sits in the request path even when it does nothing. On the
// Tailscale network "none" is correct; putting the seam here from the start
// makes a later Tailscale Funnel a configuration change rather than a refactor.
//
// Misconfiguration is refused at construction, not per request: a server that
// starts in bearer mode with no token would reject every call while looking
// configured.
func AuthMiddleware(next http.Handler, mode, token string) (http.Handler, error) {
	switch mode {
	case "none":
		return next, nil
	case "bearer":
		if token == "" {
			return nil, fmt.Errorf("auth mode is bearer but no token is set")
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || strings.TrimSpace(got) != token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		}), nil
	}
	return nil, fmt.Errorf("unknown auth mode %q: use none or bearer", mode)
}
