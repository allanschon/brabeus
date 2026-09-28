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

// giteaServer serves routes by path, answers 401 to any other token, and
// records which page of each path was asked for.
type giteaServer struct {
	*httptest.Server
	mu    sync.Mutex
	asked map[string][]int
}

func newGiteaServer(t *testing.T, token string, routes map[string][]string) *giteaServer {
	t.Helper()
	g := &giteaServer{asked: map[string][]int{}}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token "+token {
			http.Error(w, `{"message":"token is required"}`, http.StatusUnauthorized)
			return
		}
		pages, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		g.mu.Lock()
		g.asked[r.URL.Path] = append(g.asked[r.URL.Path], page)
		g.mu.Unlock()
		if page < 1 || page > len(pages) {
			fmt.Fprint(w, "[]")
			return
		}
		fmt.Fprint(w, pages[page-1])
	}))
	return g
}

func (g *giteaServer) pages(path string) []int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]int{}, g.asked[path]...)
}

func commitsJSON(dates ...string) string {
	var parts []string
	for _, d := range dates {
		parts = append(parts, fmt.Sprintf(`{"sha":"x","commit":{"committer":{"date":%q}}}`, d))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

const giteaCommits = "/api/v1/repos/me/side/commits"

func giteaCommitPages() map[string][]string {
	return map[string][]string{giteaCommits: {
		commitsJSON("2026-09-28T10:00:00Z", "2026-09-20T10:00:00Z"),
		commitsJSON("2026-09-01T10:00:00Z", "2026-08-01T10:00:00Z"),
		commitsJSON("2026-07-01T10:00:00Z"),
	}}
}

// Gitea's commit listing takes no since, so the adapter pages newest first
// and stops at the first commit older than since, or once min is reached.
func TestGiteaCountsCommitsNewestFirstAndStopsAtSince(t *testing.T) {
	srv := newGiteaServer(t, "tok", giteaCommitPages())
	defer srv.Close()
	g := &Gitea{URL: srv.URL, Token: "tok", Client: srv.Client(), perPage: 2}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) // -14d is 2026-09-17
	out, err := g.Check(context.Background(), map[string]string{"repo": "me/side", "since": "-14d", "min": "3"}, now)
	if err != nil || out.State != store.Fail || out.Detail != "2 found" {
		t.Errorf("min 3: %+v %v", out, err)
	}
	if got := srv.pages(giteaCommits); fmt.Sprint(got) != "[1 2]" {
		t.Errorf("min 3 asked for pages %v; it must stop at the 09-01 commit on page 2 and never ask for page 3", got)
	}
}

func TestGiteaStopsAsSoonAsMinIsReached(t *testing.T) {
	srv := newGiteaServer(t, "tok", giteaCommitPages())
	defer srv.Close()
	g := &Gitea{URL: srv.URL, Token: "tok", Client: srv.Client(), perPage: 2}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	out, err := g.Check(context.Background(), map[string]string{"repo": "me/side", "since": "-14d", "min": "1"}, now)
	if err != nil || out.State != store.Pass || out.Detail != "1 found" {
		t.Errorf("min 1: %+v %v", out, err)
	}
	if got := srv.pages(giteaCommits); fmt.Sprint(got) != "[1]" {
		t.Errorf("min 1 asked for pages %v; page 2 must never be requested", got)
	}
}

func TestGiteaResolvesABareRepositoryAgainstTheOwner(t *testing.T) {
	srv := newGiteaServer(t, "tok", giteaCommitPages())
	defer srv.Close()
	g := &Gitea{URL: srv.URL, Token: "tok", Owner: "me", Client: srv.Client(), perPage: 2}
	out, err := g.Check(context.Background(), map[string]string{"repo": "side", "since": "-14d", "min": "1"}, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || out.State != store.Pass {
		t.Errorf("bare name with owner: %+v %v", out, err)
	}
}

func TestGiteaCountsMergedPullRequests(t *testing.T) {
	srv := newGiteaServer(t, "tok", map[string][]string{"/api/v1/repos/me/side/pulls": {
		`[{"number":4,"merged_at":"2026-09-25T10:00:00Z"},{"number":3,"merged_at":null}]`,
		`[{"number":2,"merged_at":"2026-09-18T10:00:00Z"},{"number":1,"merged_at":"2026-08-01T10:00:00Z"}]`,
	}})
	defer srv.Close()
	g := &Gitea{URL: srv.URL, Token: "tok", Client: srv.Client(), perPage: 2}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	out, err := g.Check(context.Background(), map[string]string{"repo": "me/side", "since": "-14d", "min": "3", "merged": "true"}, now)
	if err != nil || out.State != store.Fail || out.Detail != "2 found" {
		t.Errorf("merged, min 3: %+v %v", out, err) // 4 and 2; 3 was closed unmerged; 1 is before since
	}
	out, _ = g.Check(context.Background(), map[string]string{"repo": "me/side", "since": "-14d", "min": "2", "merged": "true"}, now)
	if out.State != store.Pass {
		t.Errorf("merged, min 2: %+v", out)
	}
}

// §13: a revoked credential or a repository the forge does not have is a
// fault in the deployment, never the person falling behind.
func TestGiteaAnswersNoEvidenceOnARevokedTokenOrAMissingRepository(t *testing.T) {
	srv := newGiteaServer(t, "tok", giteaCommitPages())
	defer srv.Close()
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	revoked := &Gitea{URL: srv.URL, Token: "old", Client: srv.Client()}
	for _, merged := range []string{"false", "true"} {
		out, err := revoked.Check(context.Background(), map[string]string{"repo": "me/side", "since": "-14d", "min": "1", "merged": merged}, now)
		if err != nil || out.State != store.NoEvidence || !strings.Contains(out.Detail, "401") {
			t.Errorf("revoked, merged=%s: %+v %v", merged, out, err)
		}
	}
	g := &Gitea{URL: srv.URL, Token: "tok", Client: srv.Client()}
	out, err := g.Check(context.Background(), map[string]string{"repo": "me/gone", "since": "-14d", "min": "1"}, now)
	if err != nil || out.State != store.NoEvidence || !strings.Contains(out.Detail, "404") {
		t.Errorf("missing repository: %+v %v", out, err)
	}
}

func TestGiteaReadsPastAShortPage(t *testing.T) {
	srv := newGiteaServer(t, "tok", map[string][]string{giteaCommits: {
		commitsJSON("2026-09-28T10:00:00Z"),
		commitsJSON("2026-09-20T10:00:00Z"),
	}})
	defer srv.Close()
	g := &Gitea{URL: srv.URL, Token: "tok", Client: srv.Client()} // asks for 50, gets 1
	out, err := g.Check(context.Background(), map[string]string{"repo": "me/side", "since": "-14d", "min": "3"}, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || out.State != store.Fail || out.Detail != "2 found" {
		t.Errorf("short pages: %+v %v", out, err)
	}
	if got := srv.pages(giteaCommits); fmt.Sprint(got) != "[1 2 3]" {
		t.Errorf("asked for pages %v; only the empty page 3 ends the count", got)
	}
}
