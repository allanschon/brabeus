package claims

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/allanschon/brabeus/internal/store"
)

func vikunjaServer(t *testing.T, token string, pages ...string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/v1/tasks/all" {
			// The real 2.5.0 server reads "all" as a task id, not a listing
			// route, and answers 400 rather than a page of tasks.
			http.Error(w, `{"message":"Invalid model provided.","code":1002}`, http.StatusBadRequest)
			return
		}
		if r.URL.Path != "/api/v1/tasks" {
			http.NotFound(w, r)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 || page > len(pages) {
			fmt.Fprint(w, "[]")
			return
		}
		fmt.Fprint(w, pages[page-1])
	}))
}

const tasksPage1 = `[
 {"id":1,"done":true,"done_at":"2026-08-10T10:00:00Z","created":"2026-07-01T00:00:00Z","labels":[{"title":"article"}]},
 {"id":2,"done":true,"done_at":"2026-06-10T10:00:00Z","created":"2026-05-01T00:00:00Z","labels":[{"title":"article"}]},
 {"id":3,"done":false,"done_at":"0001-01-01T00:00:00Z","created":"2026-08-01T00:00:00Z","labels":[{"title":"article"}]},
 {"id":4,"done":true,"done_at":"2026-09-01T10:00:00Z","created":"2026-08-01T00:00:00Z","labels":[{"title":"chore"}]}]`
const tasksPage2 = `[{"id":5,"done":true,"done_at":"2026-09-15T10:00:00Z","created":"2026-08-20T00:00:00Z","labels":[{"title":"Article"}]}]`

func TestVikunjaCountsDoneTasksByLabelAndDateAcrossPages(t *testing.T) {
	srv := vikunjaServer(t, "tok", tasksPage1, tasksPage2)
	defer srv.Close()
	v := &Vikunja{URL: srv.URL, Token: "tok", Client: srv.Client(), perPage: 4}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	out, err := v.Check(context.Background(), map[string]string{"label": "article", "since": "2026-07-01", "min": "3"}, now)
	if err != nil || out.State != store.Fail || out.Detail != "2 found" {
		t.Errorf("min 3: %+v %v", out, err) // ids 1 and 5 (label matched case-insensitively); 2 is before since; 3 is open; 4 is another label
	}
	out, _ = v.Check(context.Background(), map[string]string{"label": "article", "since": "2026-07-01", "min": "2"}, now)
	if out.State != store.Pass {
		t.Errorf("min 2: %+v", out)
	}
	out, _ = v.Check(context.Background(), map[string]string{"label": "article", "since": "2026-07-01", "min": "1", "done": "false"}, now)
	if out.State != store.Pass || out.Detail != "1 found" {
		t.Errorf("open tasks: %+v", out)
	}
}

// A label the tracker holds on some task, with none inside the window,
// is evidence that the claim did not hold — fail by count.
func TestVikunjaFailsALabelItKnowsWithNothingInTheWindow(t *testing.T) {
	srv := vikunjaServer(t, "tok", tasksPage1, tasksPage2)
	defer srv.Close()
	v := &Vikunja{URL: srv.URL, Token: "tok", Client: srv.Client(), perPage: 4}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	out, err := v.Check(context.Background(), map[string]string{"label": "chore", "since": "2026-09-15", "min": "1"}, now)
	if err != nil || out.State != store.Fail || out.Detail != "0 found" {
		t.Errorf("chore since 2026-09-15: %+v %v", out, err)
	}
}

// Vikunja 2.5.0 reads /tasks/all as the single-task route with "all" as the
// id, not a listing route, and answers 400 to it; the fake server here does
// the same, so a regression back to that path fails this test with an HTTP
// error instead of quietly returning no-evidence.
func TestVikunjaPagesTheListingRouteNotTheSingleTaskRoute(t *testing.T) {
	srv := vikunjaServer(t, "tok", tasksPage1, tasksPage2)
	defer srv.Close()
	v := &Vikunja{URL: srv.URL, Token: "tok", Client: srv.Client(), perPage: 4}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	out, err := v.Check(context.Background(), map[string]string{"label": "article", "since": "2026-07-01", "min": "2"}, now)
	if err != nil || out.State != store.Pass {
		t.Errorf("expected /api/v1/tasks to page cleanly: %+v %v", out, err)
	}
}

// §13: a revoked credential produces no-evidence, not an accusation; and a
// label the tracker has never seen on any task is no-evidence, not a fail
// (§8.1: "the query matched nothing it could count").
func TestVikunjaAnswersNoEvidenceForARevokedTokenOrAnUnknownLabel(t *testing.T) {
	srv := vikunjaServer(t, "tok", tasksPage1)
	defer srv.Close()
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	revoked := &Vikunja{URL: srv.URL, Token: "old", Client: srv.Client()}
	out, err := revoked.Check(context.Background(), map[string]string{"label": "article", "since": "2026-07-01", "min": "1"}, now)
	if err != nil || out.State != store.NoEvidence || !strings.Contains(out.Detail, "401") {
		t.Errorf("revoked: %+v %v", out, err)
	}
	v := &Vikunja{URL: srv.URL, Token: "tok", Client: srv.Client()}
	out, err = v.Check(context.Background(), map[string]string{"label": "novel", "since": "2026-07-01", "min": "1"}, now)
	if err != nil || out.State != store.NoEvidence || !strings.Contains(out.Detail, "novel") {
		t.Errorf("unknown label: %+v %v", out, err)
	}
	down := &Vikunja{URL: "http://127.0.0.1:1", Token: "tok", Client: &http.Client{Timeout: time.Second}}
	if _, err := down.Check(context.Background(), map[string]string{"label": "article", "since": "2026-07-01", "min": "1"}, now); err == nil {
		t.Error("an unreachable tracker is an error, which the runner records as no-evidence")
	}
}

// Paging stops at a cap; a count short of min when the source had more pages
// is a lower bound, so it proves nothing and reads no-evidence.
func TestVikunjaStoppedAtItsPageCapCannotFail(t *testing.T) {
	srv := vikunjaServer(t, "tok", tasksPage1, tasksPage2)
	defer srv.Close()
	v := &Vikunja{URL: srv.URL, Token: "tok", Client: srv.Client(), perPage: 4, maxPages: 1}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	out, err := v.Check(context.Background(), map[string]string{"label": "article", "since": "2026-07-01", "min": "2"}, now)
	if err != nil || out.State != store.NoEvidence || !strings.Contains(out.Detail, "1 found") {
		t.Errorf("short at the cap: %+v %v", out, err)
	}
	out, _ = v.Check(context.Background(), map[string]string{"label": "article", "since": "2026-07-01", "min": "1"}, now)
	if out.State != store.Pass {
		t.Errorf("min reached before the cap still passes: %+v", out)
	}
}

// A server whose page size is below the one asked for answers short pages;
// only an empty page ends the count, or the tracker would be undercounted
// into a fail.
func TestVikunjaReadsPastAShortPage(t *testing.T) {
	srv := vikunjaServer(t, "tok",
		`[{"id":1,"done":true,"done_at":"2026-08-10T10:00:00Z","labels":[{"title":"article"}]}]`,
		`[{"id":5,"done":true,"done_at":"2026-09-15T10:00:00Z","labels":[{"title":"article"}]}]`)
	defer srv.Close()
	v := &Vikunja{URL: srv.URL, Token: "tok", Client: srv.Client()} // asks for 50, gets 1
	out, err := v.Check(context.Background(), map[string]string{"label": "article", "since": "2026-07-01", "min": "2"}, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || out.State != store.Pass || out.Detail != "2 found" {
		t.Errorf("short pages: %+v %v", out, err)
	}
}
