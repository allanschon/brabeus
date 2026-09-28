package store

import (
	"fmt"
	"strings"

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
		switch strings.TrimSpace(key) {
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
			return nil, fmt.Errorf("claim %d has a key %q; an item has text and check only", len(out), strings.TrimSpace(key))
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
