package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPickRepoDefaultsToMemory(t *testing.T) {
	mem, proj := &Store{}, &Store{}
	for _, name := range []string{"", "memory", "MEMORY"} {
		got, err := pickRepo(name, mem, proj)
		if err != nil {
			t.Fatalf("pickRepo(%q): %v", name, err)
		}
		if got != mem {
			t.Errorf("pickRepo(%q) did not return the memory store", name)
		}
	}
}

func TestPickRepoReturnsTheProjectsMirror(t *testing.T) {
	mem, proj := &Store{}, &Store{}
	got, err := pickRepo("projects", mem, proj)
	if err != nil {
		t.Fatal(err)
	}
	if got != proj {
		t.Error("pickRepo(projects) did not return the projects mirror")
	}
}

// The mirror is optional, and its absence must be an explicit error rather than
// a silent fallback to the memory store — which would let a caller read the
// wrong repository and never know.
func TestPickRepoSaysSoWhenTheMirrorIsAbsent(t *testing.T) {
	if _, err := pickRepo("projects", &Store{}, nil); err == nil {
		t.Error("an absent projects mirror was not reported")
	}
}

func TestPickRepoRejectsAnUnknownName(t *testing.T) {
	if _, err := pickRepo("secrets", &Store{}, &Store{}); err == nil {
		t.Error("an unknown repo name was accepted")
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot) // distinctive: proves we reached through
	})
}

func TestAuthMiddlewareNoneLetsEverythingThrough(t *testing.T) {
	h, err := authMiddleware(okHandler(), "none", "")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/mcp", nil))
	if w.Code != http.StatusTeapot {
		t.Errorf("status %d, want the handler to be reached", w.Code)
	}
}

func TestAuthMiddlewareBearerRejectsAWrongOrMissingToken(t *testing.T) {
	h, err := authMiddleware(okHandler(), "bearer", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"", "Bearer wrong", "s3cret", "Basic s3cret"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/mcp", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		h.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("Authorization %q got status %d, want 401", header, w.Code)
		}
	}
}

func TestAuthMiddlewareBearerAcceptsTheRightToken(t *testing.T) {
	h, err := authMiddleware(okHandler(), "bearer", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Header.Set("Authorization", "Bearer s3cret")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusTeapot {
		t.Errorf("status %d, want the handler to be reached", w.Code)
	}
}

// Bearer mode with no token would accept nothing while looking configured, and
// an unknown mode must not quietly behave like "none".
func TestAuthMiddlewareRefusesUnusableConfiguration(t *testing.T) {
	if _, err := authMiddleware(okHandler(), "bearer", ""); err == nil {
		t.Error("bearer mode with an empty token was accepted")
	}
	if _, err := authMiddleware(okHandler(), "opportunistic", "x"); err == nil {
		t.Error("an unknown auth mode was accepted")
	}
}

func TestFilterEntriesHidesOtherMachines(t *testing.T) {
	in := []Entry{
		{Path: "personal/a.md", Scope: "global"},
		{Path: "infra/b.md", Scope: "machine/gamma"},
		{Path: "infra/c.md", Scope: "machine/beta"},
	}
	got := filterEntries(in, "beta", false)
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %v", len(got), got)
	}
	for _, e := range got {
		if e.Path == "infra/b.md" {
			t.Error("another machine's memory was returned")
		}
	}
	if len(filterEntries(in, "beta", true)) != 3 {
		t.Error("include_all_scopes did not return everything")
	}
}

// Filtering must never return nil where an empty list is meant: a nil slice
// marshals as null, and a client reading null as "unknown" rather than "none"
// is a difference that only shows up in production.
func TestFilterEntriesReturnsAnEmptySliceNotNil(t *testing.T) {
	got := filterEntries([]Entry{{Path: "x.md", Scope: "machine/elsewhere"}}, "beta", false)
	if got == nil {
		t.Fatal("filterEntries returned nil")
	}
	if len(got) != 0 {
		t.Errorf("got %d entries, want 0", len(got))
	}
}

// A limit above the ceiling is clamped to it, not reset to the default: a
// caller who asked for 1000 wants many, not 50.
func TestSearchLimitClampsRatherThanResets(t *testing.T) {
	for in, want := range map[int]int{0: 50, -1: 50, 1: 1, 50: 50, 500: 500, 501: 500, 1000: 500} {
		if got := searchLimit(in); got != want {
			t.Errorf("searchLimit(%d) = %d, want %d", in, got, want)
		}
	}
}
