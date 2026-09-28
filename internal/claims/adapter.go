// Package claims holds the evidence adapters a goal's claims name (spec §8.1)
// and the argument schema each one takes. It imports store for ClaimState and
// nothing else internal.
package claims

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/allanschon/brabeus/internal/store"
)

// Outcome is one claim's result as an adapter saw it.
type Outcome struct {
	State  store.ClaimState
	Detail string // "2 found", "credential refused (HTTP 401)", "label \"article\" unknown to the tracker"
}

// Adapter is one evidence source (spec §8.1). An error from Check is a fault
// in the deployment and becomes no-evidence with the error as detail; Check
// returns Fail only when it saw evidence and the claim did not hold.
type Adapter interface {
	Name() string
	Check(ctx context.Context, args map[string]string, now time.Time) (Outcome, error)
}

var relativeDays = regexp.MustCompile(`^-([1-9][0-9]*)d$`)

// resolveDate reads YYYY-MM-DD, or -Nd as N days before now.
func resolveDate(s string, now time.Time) (time.Time, error) {
	if m := relativeDays.FindStringSubmatch(s); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return time.Time{}, fmt.Errorf("%q: %w", s, err)
		}
		return now.AddDate(0, 0, -n), nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not YYYY-MM-DD or -Nd", s)
	}
	return t, nil
}

type schema struct {
	required []string
	optional []string
}

// schemas is the argument schema per adapter. No argument's value can need a
// comma: store.ParseClaims splits a check map on every one.
var schemas = map[string]schema{
	"tracker": {required: []string{"label", "since", "min"}, optional: []string{"done"}},
	"forge":   {required: []string{"repo", "since", "min"}, optional: []string{"merged"}},
	"date":    {optional: []string{"before", "after"}},
	"manual":  {},
}

// Validate checks a claim's arguments against its adapter's schema. It is the
// value Store.ValidateClaim is set to, and it needs no configuration, so a
// goal written on a deployment without a tracker still carries a claim a
// deployment with one could run.
func Validate(adapter string, args map[string]string) error {
	sc, ok := schemas[adapter]
	if !ok {
		return fmt.Errorf("adapter %q is not one this kernel implements (spec §8.1)", adapter)
	}
	if adapter == "manual" && len(args) > 0 {
		return fmt.Errorf("a manual claim takes no arguments; it is asked at interview (spec §8.1)")
	}
	allowed := map[string]bool{}
	for _, k := range append(append([]string{}, sc.required...), sc.optional...) {
		allowed[k] = true
	}
	for k := range args {
		if !allowed[k] {
			return fmt.Errorf("%s does not take %q (spec §8.1)", adapter, k)
		}
	}
	for _, k := range sc.required {
		if strings.TrimSpace(args[k]) == "" {
			return fmt.Errorf("%s needs %s (spec §8.1)", adapter, k)
		}
	}
	if v, ok := args["since"]; ok {
		if _, err := resolveDate(v, time.Now()); err != nil {
			return fmt.Errorf("since: %w", err)
		}
	}
	if v, ok := args["min"]; ok {
		if n, err := strconv.Atoi(v); err != nil || n < 1 {
			return fmt.Errorf("min: %q is not an integer of at least 1", v)
		}
	}
	for _, k := range []string{"done", "merged"} {
		if v, ok := args[k]; ok && v != "true" && v != "false" {
			return fmt.Errorf("%s: %q is not true or false", k, v)
		}
	}
	if v, ok := args["repo"]; ok {
		parts := strings.Split(v, "/")
		for _, p := range parts {
			if strings.TrimSpace(p) == "" {
				parts = nil
			}
		}
		if len(parts) != 1 && len(parts) != 2 {
			return fmt.Errorf("repo: %q is not name or owner/name", v)
		}
	}
	if adapter == "date" {
		return validateDates(args)
	}
	return nil
}

func validateDates(args map[string]string) error {
	before, hasBefore := args["before"]
	after, hasAfter := args["after"]
	if !hasBefore && !hasAfter {
		return fmt.Errorf("date needs before, after or both (spec §8.1)")
	}
	var b, a time.Time
	var err error
	if hasBefore {
		if b, err = time.Parse("2006-01-02", before); err != nil {
			return fmt.Errorf("before: %q is not YYYY-MM-DD", before)
		}
	}
	if hasAfter {
		if a, err = time.Parse("2006-01-02", after); err != nil {
			return fmt.Errorf("after: %q is not YYYY-MM-DD", after)
		}
	}
	if hasBefore && hasAfter && !a.Before(b) {
		return fmt.Errorf("after must be earlier than before")
	}
	return nil
}

// Config names the backends a deployment runs. Tracker is "vikunja" or "";
// Forge is "gitea", "github" or "". ForgeOwner is what a bare repository name
// in a forge claim resolves against.
type Config struct {
	Tracker, TrackerURL, TrackerToken string
	Forge, ForgeURL, ForgeToken       string
	ForgeOwner                        string
}

// FromEnv reads BRABEUS_TRACKER, BRABEUS_TRACKER_URL, BRABEUS_TRACKER_TOKEN,
// BRABEUS_FORGE, BRABEUS_FORGE_URL, BRABEUS_FORGE_TOKEN and BRABEUS_FORGE_OWNER.
func FromEnv(get func(string) string) Config {
	return Config{
		Tracker:      strings.ToLower(strings.TrimSpace(get("BRABEUS_TRACKER"))),
		TrackerURL:   strings.TrimRight(strings.TrimSpace(get("BRABEUS_TRACKER_URL")), "/"),
		TrackerToken: strings.TrimSpace(get("BRABEUS_TRACKER_TOKEN")),
		Forge:        strings.ToLower(strings.TrimSpace(get("BRABEUS_FORGE"))),
		ForgeURL:     strings.TrimRight(strings.TrimSpace(get("BRABEUS_FORGE_URL")), "/"),
		ForgeToken:   strings.TrimSpace(get("BRABEUS_FORGE_TOKEN")),
		ForgeOwner:   strings.TrimSpace(get("BRABEUS_FORGE_OWNER")),
	}
}

// New builds the adapters a deployment configured: date always, tracker and
// forge when named. An unknown backend name is an error at startup. manual is
// never an Adapter: it is asked at interview (§8.1).
func New(cfg Config, client *http.Client) (map[string]Adapter, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	out := map[string]Adapter{"date": Date{}}
	switch cfg.Tracker {
	case "":
	case "vikunja":
		if cfg.TrackerURL == "" {
			return nil, fmt.Errorf("BRABEUS_TRACKER=vikunja needs BRABEUS_TRACKER_URL")
		}
		out["tracker"] = &Vikunja{URL: cfg.TrackerURL, Token: cfg.TrackerToken, Client: client}
	default:
		return nil, fmt.Errorf("BRABEUS_TRACKER=%q: this kernel implements vikunja (spec §8.1)", cfg.Tracker)
	}
	switch cfg.Forge {
	case "":
	case "gitea":
		if cfg.ForgeURL == "" {
			return nil, fmt.Errorf("BRABEUS_FORGE=gitea needs BRABEUS_FORGE_URL")
		}
		out["forge"] = &Gitea{URL: cfg.ForgeURL, Token: cfg.ForgeToken, Owner: cfg.ForgeOwner, Client: client}
	case "github":
		base := cfg.ForgeURL
		if base == "" {
			base = "https://api.github.com"
		}
		out["forge"] = &GitHub{URL: base, Token: cfg.ForgeToken, Owner: cfg.ForgeOwner, Client: client}
	default:
		return nil, fmt.Errorf("BRABEUS_FORGE=%q: this kernel implements gitea and github (spec §8.1)", cfg.Forge)
	}
	return out, nil
}

// get performs one authenticated GET and classifies the answer: a credential
// refusal or a missing resource is no-evidence (a fault in the deployment,
// never the person falling behind). Any other non-200, an unreachable host or
// a body that is not the expected JSON is an error, which the runner records
// as no-evidence with the error as detail. No token is ever put in the URL,
// because an error's text becomes a stored detail.
func get(ctx context.Context, client *http.Client, endpoint string, headers map[string]string, into any) (*Outcome, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return &Outcome{State: store.NoEvidence, Detail: fmt.Sprintf("credential refused (HTTP %d)", resp.StatusCode)}, nil
	case http.StatusNotFound:
		return &Outcome{State: store.NoEvidence, Detail: fmt.Sprintf("not found (HTTP 404): %s", req.URL.Path)}, nil
	default:
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, req.URL.Host)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return nil, fmt.Errorf("unreadable answer from %s: %w", req.URL.Host, err)
	}
	return nil, nil
}

func counted(count, min int) Outcome {
	if count >= min {
		return Outcome{State: store.Pass, Detail: fmt.Sprintf("%d found", count)}
	}
	return Outcome{State: store.Fail, Detail: fmt.Sprintf("%d found", count)}
}

// exhausted is the answer when paging hit its cap before the source ran out:
// the count is only a lower bound, so falling short of min proves nothing.
func exhausted(count, min, pages int) Outcome {
	if count >= min {
		return counted(count, min)
	}
	return Outcome{State: store.NoEvidence, Detail: fmt.Sprintf("%d found in the first %d pages; stopped before the end", count, pages)}
}

// resolveRepo splits a forge claim's repo into owner and name. A bare name
// resolves against the deployment's BRABEUS_FORGE_OWNER, so spec §8.1's
// `repo: side-project` runs as written; without one it is no-evidence.
func resolveRepo(repo, owner string) (string, string, *Outcome) {
	if o, n, ok := strings.Cut(repo, "/"); ok {
		return o, n, nil
	}
	if owner == "" {
		return "", "", &Outcome{State: store.NoEvidence, Detail: fmt.Sprintf("repo %q names no owner and BRABEUS_FORGE_OWNER is unset", repo)}
	}
	return owner, repo, nil
}
