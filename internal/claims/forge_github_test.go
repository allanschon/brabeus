package claims

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/allanschon/brabeus/internal/store"
)

// githubServer checks GitHub's headers, answers 401 to any other token, and
// records every query it was asked.
type githubServer struct {
	*httptest.Server
	mu      sync.Mutex
	queries []string
}

func newGitHubServer(t *testing.T, token string, routes map[string][]string) *githubServer {
	t.Helper()
	g := &githubServer{}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
			t.Errorf("headers: Accept %q, X-GitHub-Api-Version %q", r.Header.Get("Accept"), r.Header.Get("X-GitHub-Api-Version"))
		}
		pages, ok := routes[r.URL.Path]
		if !ok {
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			return
		}
		g.mu.Lock()
		g.queries = append(g.queries, r.URL.Path+"?"+r.URL.RawQuery)
		g.mu.Unlock()
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 || page > len(pages) {
			fmt.Fprint(w, "[]")
			return
		}
		fmt.Fprint(w, pages[page-1])
	}))
	return g
}

func (g *githubServer) asked() []*http.Request {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []*http.Request
	for _, q := range g.queries {
		r, _ := http.NewRequest(http.MethodGet, "http://forge.example"+q, nil)
		out = append(out, r)
	}
	return out
}

// GitHub filters commits by since itself, so the adapter counts every commit
// it is given and pages until an empty page — a short page is not the end.
func TestGitHubSendsSinceAsRFC3339AndPagesToAnEmptyPage(t *testing.T) {
	srv := newGitHubServer(t, "tok", map[string][]string{"/repos/me/side/commits": {
		commitsJSON("2026-09-28T10:00:00Z", "2026-09-20T10:00:00Z"),
		commitsJSON("2026-09-18T10:00:00Z"),
	}})
	defer srv.Close()
	gh := &GitHub{URL: srv.URL, Token: "tok", Client: srv.Client()}
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	out, err := gh.Check(context.Background(), map[string]string{"repo": "me/side", "since": "-14d", "min": "4"}, now)
	if err != nil || out.State != store.Fail || out.Detail != "3 found" {
		t.Errorf("min 4: %+v %v", out, err)
	}
	asked := srv.asked()
	var pages []string
	for _, r := range asked {
		q := r.URL.Query()
		if q.Get("since") != "2026-09-17T12:00:00Z" {
			t.Errorf("since sent as %q, want RFC 3339 2026-09-17T12:00:00Z", q.Get("since"))
		}
		if _, err := time.Parse(time.RFC3339, q.Get("since")); err != nil {
			t.Errorf("since %q is not RFC 3339: %v", q.Get("since"), err)
		}
		pages = append(pages, q.Get("page"))
	}
	if fmt.Sprint(pages) != "[1 2 3]" {
		t.Errorf("asked for pages %v; paging must run to the empty page 3", pages)
	}
	out, _ = gh.Check(context.Background(), map[string]string{"repo": "me/side", "since": "-14d", "min": "3"}, now)
	if out.State != store.Pass || out.Detail != "3 found" {
		t.Errorf("min 3: %+v", out)
	}
}

func TestGitHubResolvesABareRepositoryAgainstTheOwner(t *testing.T) {
	srv := newGitHubServer(t, "tok", map[string][]string{"/repos/me/side-project/commits": {commitsJSON("2026-09-28T10:00:00Z")}})
	defer srv.Close()
	gh := &GitHub{URL: srv.URL, Token: "tok", Owner: "me", Client: srv.Client()}
	out, err := gh.Check(context.Background(), map[string]string{"repo": "side-project", "since": "-14d", "min": "1"}, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || out.State != store.Pass {
		t.Errorf("bare name with owner: %+v %v", out, err)
	}
}

// Pulls are listed most recently updated first; a merge updates the pull, so
// nothing merged since can follow the first pull last updated before since.
func TestGitHubCountsMergedPullRequestsAndStopsAtTheFirstOlderUpdate(t *testing.T) {
	srv := newGitHubServer(t, "tok", map[string][]string{"/repos/me/side/pulls": {
		`[{"number":5,"updated_at":"2026-09-29T10:00:00Z","merged_at":"2026-09-25T10:00:00Z"},
		  {"number":4,"updated_at":"2026-09-26T10:00:00Z","merged_at":null},
		  {"number":3,"updated_at":"2026-09-20T10:00:00Z","merged_at":"2026-09-10T10:00:00Z"}]`,
		`[{"number":2,"updated_at":"2026-09-18T10:00:00Z","merged_at":"2026-09-18T09:00:00Z"},
		  {"number":1,"updated_at":"2026-08-01T10:00:00Z","merged_at":"2026-08-01T10:00:00Z"}]`,
		`[{"number":0,"updated_at":"2026-07-01T10:00:00Z","merged_at":"2026-07-01T10:00:00Z"}]`,
	}})
	defer srv.Close()
	gh := &GitHub{URL: srv.URL, Token: "tok", Client: srv.Client()}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	out, err := gh.Check(context.Background(), map[string]string{"repo": "me/side", "since": "-14d", "min": "3", "merged": "true"}, now)
	if err != nil || out.State != store.Fail || out.Detail != "2 found" {
		t.Errorf("merged, min 3: %+v %v", out, err) // 5 and 2; 4 unmerged; 3 merged before since
	}
	for _, r := range srv.asked() {
		q := r.URL.Query()
		if q.Get("state") != "closed" || q.Get("sort") != "updated" || q.Get("direction") != "desc" {
			t.Errorf("pulls query %v", q)
		}
		if q.Get("page") == "3" {
			t.Error("page 3 requested after pull 1 was last updated before since")
		}
	}
}

func TestGitHubAnswersNoEvidenceOnARevokedTokenOrAMissingRepository(t *testing.T) {
	srv := newGitHubServer(t, "tok", map[string][]string{"/repos/me/side/commits": {commitsJSON("2026-09-28T10:00:00Z")}})
	defer srv.Close()
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	revoked := &GitHub{URL: srv.URL, Token: "old", Client: srv.Client()}
	for _, merged := range []string{"false", "true"} {
		out, err := revoked.Check(context.Background(), map[string]string{"repo": "me/side", "since": "-14d", "min": "1", "merged": merged}, now)
		if err != nil || out.State != store.NoEvidence || !strings.Contains(out.Detail, "401") {
			t.Errorf("revoked, merged=%s: %+v %v", merged, out, err)
		}
	}
	gh := &GitHub{URL: srv.URL, Token: "tok", Client: srv.Client()}
	out, err := gh.Check(context.Background(), map[string]string{"repo": "me/gone", "since": "-14d", "min": "1"}, now)
	if err != nil || out.State != store.NoEvidence || !strings.Contains(out.Detail, "404") {
		t.Errorf("missing repository: %+v %v", out, err)
	}
}
