package server

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/allanschon/brabeus/internal/store"
	"github.com/allanschon/brabeus/internal/view"
)

func viewServer(t *testing.T, consumers map[string]bool) (http.Handler, Deps) {
	t.Helper()
	st := newServerStore(t)
	set := testSet(t)
	d := instructionsDeps(t, st, set)
	vr, err := view.New(set)
	if err != nil {
		t.Fatal(err)
	}
	writeConfirmed(t, st, "identity/value/family.md", "identity", "value", "global", "Family first.")
	writeServerGoal(t, st, "telos/goal/away.md", "G9", "machine/other")
	return ViewHandler(d, vr, fakeIdentity{name: "desk"}, consumers), d
}

func get(h http.Handler, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

// §11, §13: nothing in the view can change anything, and its routes accept
// GET only, so HEAD is refused too.
func TestTheViewRefusesEveryMethodButGet(t *testing.T) {
	h, _ := viewServer(t, nil)
	for _, m := range []string{"HEAD", "POST", "PUT", "DELETE", "PATCH"} {
		for _, p := range []string{"/view/", "/view/telos/", "/view/static/view.css"} {
			w := get(h, m, p)
			if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET" {
				t.Errorf("%s %s = %d, Allow %q; want 405, Allow GET", m, p, w.Code, w.Header().Get("Allow"))
			}
			if w.Header().Get("Content-Security-Policy") != CSP {
				t.Errorf("%s %s lacks the CSP", m, p)
			}
		}
	}
}

// mountView mounts the view as main does, so the test exercises the chain
// a deployment runs.
func mountView(t *testing.T, d Deps, id fakeIdentity, mode, token string) http.Handler {
	t.Helper()
	vr, err := view.New(d.Set)
	if err != nil {
		t.Fatal(err)
	}
	g, err := GuardedView(d, vr, id, nil, mode, token)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	MountView(mux, g)
	return mux
}

// §11: the refusals and the redirect are view responses too, so they carry
// the headers; only the outermost wrapper can see them.
func TestTheViewsRefusalsAndRedirectCarryTheSecurityHeaders(t *testing.T) {
	_, d := viewServer(t, nil)
	for _, c := range []struct {
		name     string
		h        http.Handler
		path     string
		code     int
		location string
	}{
		{"unidentified", mountView(t, d, fakeIdentity{}, "none", ""), "/view/", http.StatusForbidden, ""},
		{"no token", mountView(t, d, fakeIdentity{name: "desk"}, "bearer", "a-token"), "/view/", http.StatusUnauthorized, ""},
		{"redirect", mountView(t, d, fakeIdentity{name: "desk"}, "none", ""), "/view", http.StatusMovedPermanently, "/view/"},
	} {
		w := get(c.h, "GET", c.path)
		if w.Code != c.code || w.Header().Get("Location") != c.location {
			t.Errorf("%s: %d to %q, want %d to %q", c.name, w.Code, w.Header().Get("Location"), c.code, c.location)
		}
		if w.Header().Get("Content-Security-Policy") != CSP || w.Header().Get("X-Content-Type-Options") != "nosniff" ||
			w.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s: headers = %v", c.name, w.Header())
		}
	}
}

// §10: every in-page link lands on an id the target page emits, even when
// a record's name is not its file name.
func TestEveryRecordLinkLandsOnAnId(t *testing.T) {
	h, d := viewServer(t, nil)
	if _, err := d.Memory.Write("identity/value/kin.md", store.Record{Name: "family-first", Description: "kin first", Module: "identity",
		Kind: "value", Scope: "global", Fields: map[string]string{"statement": "Kin first."}, Body: "b"}, "desk"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Memory.Write("telos/goal/file-name.md", store.Record{Name: "launch-guide", Description: "a goal", Module: "telos", Kind: "goal",
		Scope: "global", Fields: map[string]string{"id": "G7", "title": "Launch the guide", "ideal": "out", "by": "2026-12-01",
			"claims": serverGoalClaims, "serves": "family-first"}, Body: "b"}, "desk"); err != nil {
		t.Fatal(err)
	}
	link := regexp.MustCompile(`href="(/view/[a-z]+/)#([^"]+)"`)
	seen := map[string]bool{}
	for _, page := range []string{"/view/", "/view/telos/"} {
		for _, m := range link.FindAllStringSubmatch(get(h, "GET", page).Body.String(), -1) {
			seen[m[1]+"#"+m[2]] = true
			if target := get(h, "GET", m[1]).Body.String(); !strings.Contains(target, `id="`+m[2]+`"`) {
				t.Errorf("%s links to %s#%s, which has no such id", page, m[1], m[2])
			}
		}
	}
	for _, want := range []string{"/view/telos/#" + view.Anchor("goal", "launch-guide"), "/view/identity/#" + view.Anchor("value", "family-first")} {
		if !seen[want] {
			t.Errorf("no link to %s among %v", want, seen)
		}
	}
}

// §11, §13: every response carries the policy that forbids script, inline
// style and other origins.
func TestEveryViewResponseCarriesTheSecurityHeaders(t *testing.T) {
	h, _ := viewServer(t, nil)
	fonts := []string{"/view/fonts/missing.woff2"}
	for _, p := range append([]string{"/view/", "/view/telos/", "/view/static/view.css", "/view/nope/"}, fonts...) {
		w := get(h, "GET", p)
		if w.Header().Get("Content-Security-Policy") != CSP || w.Header().Get("X-Content-Type-Options") != "nosniff" ||
			w.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s headers = %v", p, w.Header())
		}
	}
	if !strings.Contains(CSP, "font-src 'self'") {
		t.Errorf("CSP lacks font-src: %s", CSP)
	}
}

func TestTheViewServesAFontAndNothingElse(t *testing.T) {
	h, _ := viewServer(t, nil)
	// The mux cleans a literal "../" with a redirect, so that path is
	// followed once; the escaped form reaches the handler as a file name.
	for _, p := range []string{"/view/fonts/../x", "/view/fonts/..%2Fx", "/view/fonts/OFL.txt"} {
		w := get(h, "GET", p)
		if w.Code >= 300 && w.Code < 400 {
			w = get(h, "GET", w.Header().Get("Location"))
		}
		if w.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", p, w.Code)
		}
	}
	w := get(h, "GET", "/view/fonts/barlow-latin-400-normal.woff2")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "font/woff2" || w.Body.Len() == 0 {
		t.Errorf("font = %d %q (%d bytes), headers %v", w.Code, w.Header().Get("Content-Type"), w.Body.Len(), w.Header())
	}
	if w.Header().Get("Content-Security-Policy") != CSP {
		t.Error("font response lacks the CSP")
	}
}

// §10: home carries the block as /context returns it.
func TestHomeCarriesTheBlock(t *testing.T) {
	h, d := viewServer(t, nil)
	text, _, _, err := RenderContext(d, "desk", "", false)
	if err != nil {
		t.Fatal(err)
	}
	body := get(h, "GET", "/view/").Body.String()
	if !strings.Contains(body, "Family first.") || !strings.Contains(text, "Family first.") {
		t.Errorf("home lacks the block's content")
	}
	// The nav links to every module too, so only the block's own key proves
	// the share links.
	if !strings.Contains(body, `class="block__key" href="/view/identity/"`) {
		t.Errorf("each share links to its module:\n%s", body)
	}
	if !strings.Contains(body, "Your record as of") {
		t.Error("home lacks its as-of line")
	}
}

// §10, §11: the person's module page lists every scope, labelled, with
// claims; Review Focus 4.
func TestAModulePageListsEveryScopeForThePerson(t *testing.T) {
	h, _ := viewServer(t, nil)
	body := get(h, "GET", "/view/telos/").Body.String()
	if !strings.Contains(body, "scope--machine") || !strings.Contains(body, "machine/other") || !strings.Contains(body, `class="claims"`) {
		t.Errorf("another machine's goal must be listed with its scope and claims:\n%s", body)
	}
}

// §7, Review Focus 1: a reviewed crossing preference is readable on the
// Identity page, though it has no statement field: the title falls back to
// its description.
func TestAReviewedCrossingPreferenceIsReadableOnTheIdentityPage(t *testing.T) {
	h, d := viewServer(t, nil)
	writeConfirmed(t, d.Memory, "memory/preference/plain.md", "memory", "preference", "global", "Say it plainly.")
	identity := get(h, "GET", "/view/identity/").Body.String()
	memory := get(h, "GET", "/view/memory/").Body.String()
	if !strings.Contains(identity, "Say it plainly.") || strings.Contains(memory, "Say it plainly.") {
		t.Errorf("want it on identity only; identity has it: %v, memory has it: %v",
			strings.Contains(identity, "Say it plainly."), strings.Contains(memory, "Say it plainly."))
	}
}

// §11, §13, Review Focus 3: a consumer learns nothing of a hidden module.
func TestAConsumerSeesNoTraceOfAHiddenModule(t *testing.T) {
	h, _ := viewServer(t, map[string]bool{"desk": true})
	home := get(h, "GET", "/view/").Body.String()
	for _, name := range []string{"telos", "identity", "memory"} {
		if strings.Contains(home, "/view/"+name+"/") {
			t.Errorf("consumer's home names %s", name)
		}
	}
	hidden, missing := get(h, "GET", "/view/telos/"), get(h, "GET", "/view/nope/")
	if hidden.Code != http.StatusNotFound || hidden.Body.String() != missing.Body.String() {
		t.Errorf("hidden %d %q vs missing %d %q", hidden.Code, hidden.Body, missing.Code, missing.Body)
	}
}
