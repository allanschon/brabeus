package server

import (
	"log"
	"net/http"
	"strings"

	"github.com/allanschon/brabeus/internal/identity"
	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/scope"
	"github.com/allanschon/brabeus/internal/store"
	"github.com/allanschon/brabeus/internal/view"
)

// CSP allows no script, no inline style and nothing from another origin, so
// a module's view template can only arrange what it is given (spec §11). The
// fonts are the kernel's own, served from the same origin.
const CSP = "default-src 'none'; style-src 'self'; img-src 'self'; font-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// SecureView sets the view's security headers on every response h writes.
// GuardedView puts it outermost, so the 401, the 403 and the redirect carry
// the headers as a page does.
func SecureView(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", CSP)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		h.ServeHTTP(w, r)
	})
}

// getOnly answers every method but GET with 405 (spec §11). The mux would
// let HEAD through to a GET route, and answer the rest with an Allow that
// names HEAD.
func getOnly(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// GuardedView is the view as the kernel serves it: the security headers
// outermost, then auth and identity as /context has them (spec §10, §11).
func GuardedView(d Deps, vr *view.Renderer, id identity.Identity, consumers map[string]bool, mode, token string) (http.Handler, error) {
	h, err := AuthMiddleware(RefuseUnidentified(ViewHandler(d, vr, id, consumers), id, consumers), mode, token)
	if err != nil {
		return nil, err
	}
	return SecureView(h), nil
}

// MountView routes /view and everything under /view/ to the guarded view,
// so the redirect from /view is the view's own and carries its headers.
func MountView(mux *http.ServeMux, guarded http.Handler) {
	mux.Handle("/view", guarded)
	mux.Handle("/view/", guarded)
}

// ViewHandler serves the read-only view (spec §10) under the identity it is
// given. Its routes accept GET only.
func ViewHandler(d Deps, vr *view.Renderer, id identity.Identity, consumers map[string]bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /view", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/view/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /view/static/view.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Write(view.Stylesheet())
	})
	mux.HandleFunc("GET /view/fonts/{file}", func(w http.ResponseWriter, r *http.Request) {
		b, ok := view.Font(r.PathValue("file"))
		if !ok {
			http.Error(w, "no such font", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "font/woff2")
		w.Write(b)
	})
	mux.HandleFunc("GET /view/{$}", func(w http.ResponseWriter, r *http.Request) {
		caller, consumer, _ := Caller(id, consumers, r)
		h, err := homeFor(d, vr, caller, consumer)
		if err != nil {
			log.Printf("view home for %q: %v", caller, err)
			http.Error(w, "view unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := vr.Home(w, h); err != nil {
			log.Printf("view home for %q: %v", caller, err)
		}
	})
	mux.HandleFunc("GET /view/{module}/", func(w http.ResponseWriter, r *http.Request) {
		caller, consumer, _ := Caller(id, consumers, r)
		name := strings.ToLower(r.PathValue("module"))
		man, ok := d.Set.Module(name)
		if !ok || audienceFor(d.Set, consumer).Hides(name) {
			http.Error(w, "no such module", http.StatusNotFound)
			return
		}
		p, err := moduleFor(d, vr, man, caller, consumer)
		if err != nil {
			log.Printf("view %s for %q: %v", name, caller, err)
			http.Error(w, "view unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := vr.Module(w, p); err != nil {
			log.Printf("view %s for %q: %v", name, caller, err)
		}
	})
	return SecureView(getOnly(mux))
}

// navFor is the modules the caller may read, in priority order (spec §10).
func navFor(set *module.Set, consumer bool, current string) []view.Nav {
	aud := audienceFor(set, consumer)
	var out []view.Nav
	for _, m := range set.Modules {
		if aud.Hides(m.Name) {
			continue
		}
		out = append(out, view.Nav{Name: m.Name, Href: "/view/" + m.Name + "/", Intro: m.Intro,
			Notes: m.Profile == module.WorkingMemory, Current: m.Name == current})
	}
	return out
}

// toViewClaims links each claim to its goal through links (view.Links), so
// a goal with no page the caller can open is named, never linked.
func toViewClaims(cs []claimOut, links map[string]string) []view.Claim {
	out := make([]view.Claim, 0, len(cs))
	for _, c := range cs {
		out = append(out, view.Claim{Goal: c.Goal, GoalHref: links[c.Goal], ID: c.ID, Title: c.Title, Index: c.Index, Text: c.Text,
			State: c.State, Measured: c.Measured, Adapter: c.Adapter, Manual: c.Manual, Stale: c.Stale,
			Count: c.Count, Target: c.Target, Expected: c.Expected, DaysLeft: c.DaysLeft,
			Deadline: c.Deadline, Since: c.Since, Detail: c.Detail})
	}
	return out
}

func homeFor(d Deps, vr *view.Renderer, caller string, consumer bool) (view.Home, error) {
	parts, faults, items, err := RenderContextParts(d, caller, "", consumer)
	if err != nil {
		return view.Home{}, err
	}
	ins, sizes, err := visibleInstructions(d, caller, "", consumer)
	if err != nil {
		return view.Home{}, err
	}
	co, err := claimsFor(d, caller, audienceFor(d.Set, consumer), "", !consumer)
	if err != nil {
		return view.Home{}, err
	}
	recs, err := pageRecords(d, caller, consumer)
	if err != nil {
		return view.Home{}, err
	}
	links := view.Links(d.Set, recs)
	h := view.Home{Nav: navFor(d.Set, consumer, ""), Parts: parts, Instructions: ins, Sizes: sizes, Due: items, Now: nowFor(d), Links: links}
	h.Faults = view.Faults{LastRun: co.LastRun, Interval: co.Interval, Block: faults}
	for _, c := range toViewClaims(co.Claims, links) {
		h.Faults.Total++
		if c.Manual {
			h.Faults.Manual++
		}
		switch {
		case c.State == "behind" || c.State == "fail":
			h.Behind = append(h.Behind, c)
		case c.State == "no-evidence":
			h.Faults.NoEvidence = append(h.Faults.NoEvidence, c)
		case c.Stale:
			h.Faults.Stale = append(h.Faults.Stale, c)
		}
	}
	return h, nil
}

// pageRecords are the records the caller's module pages list: every active
// record of a module the caller may read. The person's pages are their read
// of the whole record (spec §11); a consumer's stay scope-filtered. The home
// page links through the same set, so every link it makes lands on a page.
func pageRecords(d Deps, caller string, consumer bool) ([]store.Stored, error) {
	aud := audienceFor(d.Set, consumer)
	all, err := d.Memory.Records()
	if err != nil {
		return nil, err
	}
	var recs []store.Stored
	for _, r := range all {
		if !r.Retired.IsZero() || r.Module == "" || aud.Hides(r.Module) || !scope.Visible(r.Scope, caller, !consumer) {
			continue
		}
		recs = append(recs, r)
	}
	return recs, nil
}

func moduleFor(d Deps, vr *view.Renderer, man module.Manifest, caller string, consumer bool) (view.ModulePage, error) {
	aud := audienceFor(d.Set, consumer)
	recs, err := pageRecords(d, caller, consumer)
	if err != nil {
		return view.ModulePage{}, err
	}
	co, err := claimsFor(d, caller, aud, "", !consumer)
	if err != nil {
		return view.ModulePage{}, err
	}
	built := view.BuildRecords(d.Set, recs, toViewClaims(co.Claims, view.Links(d.Set, recs)), vr.Markdown, nowFor(d))
	p := view.ModulePage{Nav: navFor(d.Set, consumer, man.Name), Module: man}
	if man.Profile == module.WorkingMemory {
		for _, r := range built {
			if view.PageModuleOf(d.Set, r) == man.Name {
				p.Notes = append(p.Notes, r)
			}
		}
		return p, nil
	}
	p.Kinds = view.Kinds(d.Set, man.Name, built, nowFor(d))
	return p, nil
}
