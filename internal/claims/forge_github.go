package claims

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// GitHub counts commits or merged pull requests through the REST API.
type GitHub struct {
	URL, Token string // URL is the API root, https://api.github.com by default
	Owner      string // what a bare repository name resolves against
	Client     *http.Client
	maxPages   int // tests lower it to exercise the cap
}

func (g *GitHub) Name() string { return "forge" }

func (g *GitHub) headers() map[string]string {
	return map[string]string{
		"Authorization":        "Bearer " + g.Token,
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": "2022-11-28",
	}
}

func (g *GitHub) Check(ctx context.Context, args map[string]string, now time.Time) (Outcome, error) {
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
	repo := g.URL + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name)
	if args["merged"] == "true" {
		return g.merged(ctx, repo, since, min)
	}
	// GitHub filters by since itself, so every commit returned counts. Paging
	// runs to an empty page: a short page is not a promise of the end.
	count := 0
	for page, limit := 1, pageLimit(g.maxPages); page <= limit; page++ {
		var commits []struct {
			SHA string `json:"sha"`
		}
		q := url.Values{}
		q.Set("since", since.UTC().Format(time.RFC3339))
		q.Set("per_page", "100")
		q.Set("page", strconv.Itoa(page))
		if o, err := get(ctx, g.Client, repo+"/commits?"+q.Encode(), g.headers(), &commits); err != nil {
			return Outcome{}, err
		} else if o != nil {
			return *o, nil
		}
		if len(commits) == 0 {
			return counted(count, min), nil
		}
		count += len(commits)
	}
	return exhausted(count, min, pageLimit(g.maxPages)), nil
}

// merged lists closed pulls most recently updated first and stops at the
// first last updated before since: a merge updates the pull, so nothing
// merged since can follow it.
func (g *GitHub) merged(ctx context.Context, repo string, since time.Time, min int) (Outcome, error) {
	count := 0
	for page, limit := 1, pageLimit(g.maxPages); page <= limit; page++ {
		var pulls []struct {
			UpdatedAt string  `json:"updated_at"`
			MergedAt  *string `json:"merged_at"`
		}
		q := url.Values{}
		q.Set("state", "closed")
		q.Set("sort", "updated")
		q.Set("direction", "desc")
		q.Set("per_page", "100")
		q.Set("page", strconv.Itoa(page))
		if o, err := get(ctx, g.Client, repo+"/pulls?"+q.Encode(), g.headers(), &pulls); err != nil {
			return Outcome{}, err
		} else if o != nil {
			return *o, nil
		}
		if len(pulls) == 0 {
			return counted(count, min), nil
		}
		for _, p := range pulls {
			updated, o := parseStamp("updated_at", p.UpdatedAt)
			if o != nil {
				return *o, nil
			}
			if updated.Before(since) {
				return counted(count, min), nil
			}
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
