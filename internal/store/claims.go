package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/retrieval"
)

// ClaimsField is the goal field that carries claims (spec §8.1). Any ratified
// kind may declare it; the kernel reads it wherever it is declared.
const ClaimsField = "claims"

// claimsDir holds the kernel's claim results, one JSON file per goal at the
// goal's own path (D2). Nothing under it is a record: not markdown, not
// indexed, not listed, and MemoryPath refuses to write a record there.
const claimsDir = "claims"

// Claim is one entry of a claims block: what true would look like, and the
// adapter that gathers the evidence with its arguments as data (§8.1).
type Claim struct {
	Text    string
	Adapter string
	Args    map[string]string
}

// ClaimState is a claim's result (spec §8.1): pass and fail are evidence,
// no-evidence is a fault in the deployment, unchecked is no result yet.
type ClaimState string

const (
	Pass       ClaimState = "pass"
	Fail       ClaimState = "fail"
	NoEvidence ClaimState = "no-evidence"
	// Unchecked is a claim with no result yet: a manual claim nobody has
	// answered, or an adapter claim before its first run.
	Unchecked ClaimState = "unchecked"
)

func (st ClaimState) valid() bool {
	switch st {
	case Pass, Fail, NoEvidence, Unchecked:
		return true
	}
	return false
}

// ClaimResult is one claim's state, kernel-owned. Index is the claim's
// 0-based position on the goal and Text its wording when the result was
// taken, so a result outlives neither a reorder nor a rewording. Since is
// when the state was entered; Recorded when this entry was last written.
type ClaimResult struct {
	Index    int        `json:"index"`
	Text     string     `json:"text"`
	Adapter  string     `json:"adapter"`
	State    ClaimState `json:"state"`
	Detail   string     `json:"detail,omitempty"`
	Since    time.Time  `json:"since"`
	Recorded time.Time  `json:"recorded"`
}

// resultsFile is one goal's results on disk (D2): the goal it belongs to,
// and one entry per claim that has a result.
type resultsFile struct {
	Goal   string        `json:"goal"`
	Claims []ClaimResult `json:"claims"`
}

// Parenthetical renders a detail as " (detail)", or nothing when there is
// none: the one form the commit message and the agenda's question share.
func Parenthetical(s string) string {
	if s == "" {
		return ""
	}
	return " (" + s + ")"
}

// JoinResults gives each claim its result: the one recorded at the same
// position with the same text, or Unchecked when there is none. Matching on
// both is what keeps a result for a claim the person has since reworded or
// moved from being read as the new claim's. The agenda, the claims tool and
// the runner all join here, so they cannot disagree about which result is
// whose.
func JoinResults(claims []Claim, results []ClaimResult) []ClaimResult {
	out := make([]ClaimResult, len(claims))
	for i, c := range claims {
		out[i] = ClaimResult{Index: i, Text: c.Text, Adapter: c.Adapter, State: Unchecked}
		for _, r := range results {
			if r.Index == i && r.Text == c.Text {
				out[i] = r
				out[i].Adapter = c.Adapter
			}
		}
	}
	return out
}

// ParseClaims reads the block spec §8.1 shows: items beginning "- text:",
// each with a "check:" flow map naming the adapter and its arguments. It
// knows the shape; what an adapter's arguments mean is the adapter's, applied
// through Store.ValidateClaim.
func ParseClaims(block string) ([]Claim, error) {
	var out []Claim
	finish := func() error {
		if len(out) == 0 {
			return nil
		}
		c := out[len(out)-1]
		if c.Text == "" {
			return fmt.Errorf("claim %d has no text", len(out))
		}
		if c.Adapter == "" {
			return fmt.Errorf("claim %d (%q) has no check naming an adapter", len(out), c.Text)
		}
		return nil
	}
	for n, raw := range strings.Split(block, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			if err := finish(); err != nil {
				return nil, err
			}
			out = append(out, Claim{Args: map[string]string{}})
			line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("claims: line %d is outside any item; an item starts with \"- text:\"", n+1)
		}
		cur := &out[len(out)-1]
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("claims: line %d is not key: value", n+1)
		}
		value = strings.TrimSpace(value)
		key = strings.TrimSpace(key)
		if (key == "text" && cur.Text != "") || (key == "check" && cur.Adapter != "") {
			return nil, fmt.Errorf("claim %d has %s twice; an item has one text and one check", len(out), key)
		}
		switch key {
		case "text":
			cur.Text = retrieval.Unquote(value)
		case "check":
			inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "{"), "}"))
			if inner == "" {
				return nil, fmt.Errorf("claim %d: check is empty", len(out))
			}
			for _, pair := range strings.Split(inner, ",") {
				k, v, ok := strings.Cut(pair, ":")
				if !ok {
					return nil, fmt.Errorf("claim %d: check entry %q is not key: value", len(out), strings.TrimSpace(pair))
				}
				k, v = strings.TrimSpace(k), retrieval.Unquote(strings.TrimSpace(v))
				if k == "adapter" {
					cur.Adapter = v
				} else {
					cur.Args[k] = v
				}
			}
		default:
			return nil, fmt.Errorf("claim %d has a key %q; an item has text and check only", len(out), key)
		}
	}
	if err := finish(); err != nil {
		return nil, err
	}
	return out, nil
}

// checkClaims validates a record's claims block against its governing
// module: every adapter declared (§8.1), and every argument list accepted by
// the hook when one is set. The error names the claim by its 1-based
// position and its text, so the person knows which of several to fix.
func (s *Store) checkClaims(man module.Manifest, fields map[string]string) error {
	block, ok := fields[ClaimsField]
	if !ok || strings.TrimSpace(block) == "" {
		return nil
	}
	claims, err := ParseClaims(block)
	if err != nil {
		return fmt.Errorf("%w (spec §8.1)", err)
	}
	declared := map[string]bool{}
	for _, a := range man.Adapters {
		declared[a] = true
	}
	for i, c := range claims {
		if !declared[c.Adapter] {
			return fmt.Errorf("claim %d (%q) names adapter %q, which module %s does not declare (spec §8.1)", i+1, c.Text, c.Adapter, man.Name)
		}
		if s.ValidateClaim != nil {
			if err := s.ValidateClaim(c.Adapter, c.Args); err != nil {
				return fmt.Errorf("claim %d (%q): %w (spec §8.1)", i+1, c.Text, err)
			}
		}
	}
	return nil
}

// resultsRel is where a goal's results live: under claims/, at the goal's
// own path, as JSON.
func resultsRel(goal string) string {
	return claimsDir + "/" + strings.TrimSuffix(goal, ".md") + ".json"
}

// readResults reads one results file. A missing file is no results.
func readResults(full string) ([]ClaimResult, error) {
	b, err := os.ReadFile(full)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f resultsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s does not parse (%v); it is the kernel's file, so fix or remove it by hand", filepath.Base(full), err)
	}
	return f.Claims, nil
}

// ClaimResults reads every results file, keyed by the goal's path. The key is
// taken from where the file sits, not from what it says, so a file cannot
// claim to be another goal's. A file that does not parse is skipped and
// reported in the error, with every other goal's results still returned: one
// bad file must not take the whole agenda down with it.
func (s *Store) ClaimResults() (map[string][]ClaimResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string][]ClaimResult{}
	root := filepath.Join(s.Dir, claimsDir)
	var bad []error
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if errors.Is(err, os.ErrNotExist) && p == root {
			return filepath.SkipDir
		}
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(p, ".json") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		results, err := readResults(p)
		if err != nil {
			bad = append(bad, err)
			return nil
		}
		out[strings.TrimSuffix(filepath.ToSlash(rel), ".json")+".md"] = results
		return nil
	})
	if err != nil {
		return out, err
	}
	return out, errors.Join(bad...)
}

// RecordClaimResults is the claim-result operation (spec §8.1): it replaces
// the goal's results with these. See UpdateClaimResults for the rules.
func (s *Store) RecordClaimResults(goal string, results []ClaimResult, caller string) (string, error) {
	return s.UpdateClaimResults(goal, func([]ClaimResult) []ClaimResult {
		return append([]ClaimResult(nil), results...)
	}, caller)
}

// UpdateClaimResults is the claim-result operation's one primitive (spec
// §8.1, AN, AT). merge is handed what is recorded now and returns what should
// be, all under the store's lock after the pull, so a scheduled run and a
// manual answer landing together merge rather than one overwriting the other.
//
// It never opens the goal for writing, so the goal's content, its updated
// stamp and its history stay what the person made them. It commits only when
// a claim's state or its detail changed (K10): a run that finds what the last
// one found writes nothing, and a new answer from the person in the same
// state is recorded. Since is kept whenever the state is, because it means
// when the state was entered, whoever wrote the entry.
func (s *Store) UpdateClaimResults(goal string, merge func(prev []ClaimResult) []ClaimResult, caller string) (string, error) {
	if s.ReadOnly {
		return "", fmt.Errorf("this store is read-only")
	}
	goal, err := MemoryPath(goal)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.sync(); err != nil {
		return "", err
	}
	full, err := resolvePath(s.Dir, goal)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("no record at %q", goal)
	}
	r, _ := ParseRecord(string(content))
	out := filepath.Join(s.Dir, filepath.FromSlash(resultsRel(goal)))
	prev, err := readResults(out)
	if err != nil {
		return "", err
	}
	next := append([]ClaimResult(nil), merge(append([]ClaimResult(nil), prev...))...)
	seen := map[int]bool{}
	for i := range next {
		n := &next[i]
		if !n.State.valid() {
			return "", fmt.Errorf("claim %d: state %q is not pass, fail, no-evidence or unchecked (spec §8.1)", n.Index+1, n.State)
		}
		if n.Index < 0 || seen[n.Index] {
			return "", fmt.Errorf("claim %d: a goal's results hold one entry per claim position", n.Index+1)
		}
		seen[n.Index] = true
		n.Detail = oneLine(n.Detail)
		if shape := credentialShape(n.Detail); shape != "" {
			return "", fmt.Errorf("refused: claim %d's detail looks like it contains %s; credentials never enter the record (spec §11)", n.Index+1, shape)
		}
	}
	sort.SliceStable(next, func(i, j int) bool { return next[i].Index < next[j].Index })

	type key struct {
		index int
		text  string
	}
	before := map[key]ClaimResult{}
	for _, p := range prev {
		before[key{p.Index, p.Text}] = p
	}
	stamp := now()
	changed, answered := 0, 0
	var lines []string
	for i := range next {
		n := &next[i]
		p, had := before[key{n.Index, n.Text}]
		delete(before, key{n.Index, n.Text})
		was := Unchecked
		if had {
			was = p.State
		}
		switch {
		case was != n.State:
			changed++
			lines = append(lines, fmt.Sprintf("%d. %q: %s → %s%s", n.Index+1, n.Text, was, n.State, Parenthetical(n.Detail)))
		case had && !p.Since.IsZero():
			n.Since = p.Since
			fallthrough
		default:
			if n.Detail == p.Detail {
				if had {
					n.Recorded = p.Recorded
				}
				continue
			}
			answered++
			lines = append(lines, fmt.Sprintf("%d. %q: %s, answered%s", n.Index+1, n.Text, n.State, Parenthetical(n.Detail)))
		}
		if n.Since.IsZero() && n.State != Unchecked {
			n.Since = stamp
		}
		if n.Recorded.IsZero() {
			n.Recorded = stamp
		}
	}
	// A result the new set no longer carries is a change too, unless it only
	// ever said unchecked, which absence says as well.
	gone := make([]ClaimResult, 0, len(before))
	for _, p := range before {
		if p.State != Unchecked {
			gone = append(gone, p)
		}
	}
	sort.Slice(gone, func(i, j int) bool { return gone[i].Index < gone[j].Index })
	for _, p := range gone {
		changed++
		lines = append(lines, fmt.Sprintf("%d. %q: %s → no longer recorded", p.Index+1, p.Text, p.State))
	}
	if changed == 0 && answered == 0 {
		return "no change", nil
	}
	b, err := json.MarshalIndent(resultsFile{Goal: goal, Claims: next}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		return "", err
	}
	if err := os.WriteFile(out, append(b, '\n'), 0o640); err != nil {
		return "", err
	}
	counts := fmt.Sprintf("%d changed", changed)
	if answered > 0 {
		counts += fmt.Sprintf(", %d answered", answered)
	}
	msg := fmt.Sprintf("claims %s/%s %s: %s\n\n%s\n\nRecorded through the kernel at %s.",
		r.Module, r.Kind, r.Name, counts, strings.Join(lines, "\n"), time.Now().UTC().Format(time.RFC3339))
	return s.commitAndPush(msg, caller)
}
