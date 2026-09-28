package claims

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allanschon/brabeus/internal/store"
)

// backend is one listing an adapter reads, pointed at a server that answers
// every request with the same body.
type backend struct {
	name string
	make func(url string, c *http.Client) Adapter
	args map[string]string
}

func backends() []backend {
	return []backend{
		{"vikunja", func(u string, c *http.Client) Adapter { return &Vikunja{URL: u, Token: "tok", Client: c} },
			map[string]string{"label": "article", "since": "2026-07-01", "min": "1"}},
		{"gitea commits", func(u string, c *http.Client) Adapter { return &Gitea{URL: u, Token: "tok", Client: c} },
			map[string]string{"repo": "me/side", "since": "-14d", "min": "1"}},
		{"gitea merged", func(u string, c *http.Client) Adapter { return &Gitea{URL: u, Token: "tok", Client: c} },
			map[string]string{"repo": "me/side", "since": "-14d", "min": "1", "merged": "true"}},
		{"github commits", func(u string, c *http.Client) Adapter { return &GitHub{URL: u, Token: "tok", Client: c} },
			map[string]string{"repo": "me/side", "since": "-14d", "min": "1"}},
		{"github merged", func(u string, c *http.Client) Adapter { return &GitHub{URL: u, Token: "tok", Client: c} },
			map[string]string{"repo": "me/side", "since": "-14d", "min": "1", "merged": "true"}},
	}
}

func answering(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
}

// §8.1: an answer that is not a list is the source failing to answer, never
// a count of zero. A null body decodes to an empty slice without complaint,
// so it has to be caught before the count is read.
func TestAnAnswerThatIsNotAListIsNoEvidenceOnEveryBackend(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for _, body := range []string{"null", "{}", `"ok"`, "<html>login</html>", ""} {
		srv := answering(body)
		for _, b := range backends() {
			out, err := b.make(srv.URL, srv.Client()).Check(context.Background(), b.args, now)
			if err != nil || out.State != store.NoEvidence || !strings.Contains(out.Detail, "unreadable") {
				t.Errorf("%s answering %q: %+v %v", b.name, body, out, err)
			}
		}
		srv.Close()
	}
}

// §8.1: a timestamp the adapter cannot read is an answer it cannot count.
// Skipping it would undercount into a fail, so it reads no-evidence naming
// the field.
func TestATimestampThatWillNotParseIsNoEvidenceNamingTheField(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		make  func(url string, c *http.Client) Adapter
		args  map[string]string
		body  string
		field string
	}{
		{"vikunja done", func(u string, c *http.Client) Adapter { return &Vikunja{URL: u, Token: "tok", Client: c} },
			map[string]string{"label": "article", "since": "2026-07-01", "min": "1"},
			`[{"id":7,"done":true,"done_at":"yesterday","labels":[{"title":"article"}]}]`, "done_at"},
		{"vikunja open", func(u string, c *http.Client) Adapter { return &Vikunja{URL: u, Token: "tok", Client: c} },
			map[string]string{"label": "article", "since": "2026-07-01", "min": "1", "done": "false"},
			`[{"id":7,"done":false,"created":"soon","labels":[{"title":"article"}]}]`, "created"},
		{"gitea commit", func(u string, c *http.Client) Adapter { return &Gitea{URL: u, Token: "tok", Client: c} },
			map[string]string{"repo": "me/side", "since": "-14d", "min": "1"},
			`[{"commit":{"committer":{"date":"last week"}}}]`, "date"},
		{"gitea merged", func(u string, c *http.Client) Adapter { return &Gitea{URL: u, Token: "tok", Client: c} },
			map[string]string{"repo": "me/side", "since": "-14d", "min": "1", "merged": "true"},
			`[{"number":3,"merged_at":"last week"}]`, "merged_at"},
		{"github updated", func(u string, c *http.Client) Adapter { return &GitHub{URL: u, Token: "tok", Client: c} },
			map[string]string{"repo": "me/side", "since": "-14d", "min": "1", "merged": "true"},
			`[{"number":3,"updated_at":"last week","merged_at":null}]`, "updated_at"},
		{"github merged", func(u string, c *http.Client) Adapter { return &GitHub{URL: u, Token: "tok", Client: c} },
			map[string]string{"repo": "me/side", "since": "-14d", "min": "1", "merged": "true"},
			`[{"number":3,"updated_at":"2026-09-29T10:00:00Z","merged_at":"last week"}]`, "merged_at"},
	}
	for _, c := range cases {
		srv := answering(c.body)
		out, err := c.make(srv.URL, srv.Client()).Check(context.Background(), c.args, now)
		if err != nil || out.State != store.NoEvidence || !strings.Contains(out.Detail, c.field) || !strings.Contains(out.Detail, "last week") && !strings.Contains(out.Detail, "yesterday") && !strings.Contains(out.Detail, "soon") {
			t.Errorf("%s: %+v %v, want no-evidence naming %s and the value", c.name, out, err, c.field)
		}
		srv.Close()
	}
}
