package claims

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Gitea counts commits or merged pull requests. Its commit listing takes no
// since (checked against the live API 2026-09-28: sha, path, page, limit,
// not), so it pages newest first and stops at the first commit older than
// since, once min is reached, or at an empty page. Only an empty page is the
// end: a server whose MAX_RESPONSE_ITEMS is below the page size asked for
// answers short pages, and stopping at one would undercount into a fail.
type Gitea struct {
	URL, Token string
	Owner      string // what a bare repository name resolves against
	Client     *http.Client
	perPage    int // tests lower it to exercise paging
	maxPages   int // tests lower it to exercise the cap
}

func (g *Gitea) Name() string { return "forge" }

func (g *Gitea) headers() map[string]string {
	return map[string]string{"Authorization": "token " + g.Token}
}

func (g *Gitea) pageSize() int {
	if g.perPage == 0 {
		return 50
	}
	return g.perPage
}

func (g *Gitea) Check(ctx context.Context, args map[string]string, now time.Time) (Outcome, error) {
	owner, name, o := resolveRepo(args["repo"], g.Owner)
	if o != nil {
		return *o, nil
	}
	since, err := resolveDate(args["since"], now)
	if err != nil {
		return Outcome{}, err
	}
	min, err := strconv.Atoi(args["min"])
	if err != nil {
		return Outcome{}, fmt.Errorf("min: %w", err)
	}
	repo := g.URL + "/api/v1/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name)
	if args["merged"] == "true" {
		return g.merged(ctx, repo, since, min)
	}
	perPage, count := g.pageSize(), 0
	for page, limit := 1, pageLimit(g.maxPages); page <= limit; page++ {
		var commits []struct {
			Commit struct {
				Committer struct {
					Date string `json:"date"`
				} `json:"committer"`
			} `json:"commit"`
		}
		endpoint := fmt.Sprintf("%s/commits?limit=%d&page=%d&stat=false&verification=false&files=false", repo, perPage, page)
		if o, err := get(ctx, g.Client, endpoint, g.headers(), &commits); err != nil {
			return Outcome{}, err
		} else if o != nil {
			return *o, nil
		}
		if len(commits) == 0 {
			return counted(count, min), nil
		}
		for _, c := range commits {
			when, o := parseStamp("commit date", c.Commit.Committer.Date)
			if o != nil {
				return *o, nil
			}
			if when.Before(since) {
				return counted(count, min), nil
			}
			count++
			if count >= min {
				return counted(count, min), nil
			}
		}
	}
	return exhausted(count, min, pageLimit(g.maxPages)), nil
}

// merged counts closed pull requests whose merged_at is on or after since.
func (g *Gitea) merged(ctx context.Context, repo string, since time.Time, min int) (Outcome, error) {
	perPage, count := g.pageSize(), 0
	for page, limit := 1, pageLimit(g.maxPages); page <= limit; page++ {
		var pulls []struct {
			MergedAt *string `json:"merged_at"`
		}
		endpoint := fmt.Sprintf("%s/pulls?state=closed&limit=%d&page=%d", repo, perPage, page)
		if o, err := get(ctx, g.Client, endpoint, g.headers(), &pulls); err != nil {
			return Outcome{}, err
		} else if o != nil {
			return *o, nil
		}
		if len(pulls) == 0 {
			return counted(count, min), nil
		}
		for _, p := range pulls {
			if p.MergedAt == nil {
				continue
			}
			when, o := parseStamp("merged_at", *p.MergedAt)
			if o != nil {
				return *o, nil
			}
			if !when.Before(since) {
				count++
			}
		}
	}
	return exhausted(count, min, pageLimit(g.maxPages)), nil
}
