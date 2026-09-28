package store

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// at is the store tests' one stamp parser (K1): a fixture with a stamp that
// does not parse is a broken test, so it fails loudly rather than yield zero.
func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

const goalClaims = "- text: \"At least three articles published since the quarter began\"\n  check: {adapter: tracker, done: true, label: article, since: 2026-07-01, min: 3}\n- text: \"The target date still holds\"\n  check: {adapter: manual}"

func TestParseClaimsReadsTextAdapterAndArguments(t *testing.T) {
	claims, err := ParseClaims(goalClaims)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 2 || claims[0].Adapter != "tracker" || claims[0].Args["label"] != "article" || claims[0].Args["min"] != "3" {
		t.Fatalf("claims = %+v", claims)
	}
	if claims[0].Text != "At least three articles published since the quarter began" || claims[1].Adapter != "manual" || len(claims[1].Args) != 0 {
		t.Errorf("claims = %+v", claims)
	}
	for name, bad := range map[string]string{
		"no text":      "- check: {adapter: manual}",
		"no adapter":   "- text: \"x\"\n  check: {min: 3}",
		"unknown key":  "- text: \"x\"\n  check: {adapter: manual}\n  when: now",
		"outside item": "text: x",
		"no check":     "- text: \"x\"",
	} {
		if _, err := ParseClaims(bad); err == nil {
			t.Errorf("%s: accepted %q", name, bad)
		}
	}
}

func writeGoal(t *testing.T, s *Store, rel, claims string) {
	t.Helper()
	if _, err := s.Write(rel, Record{Name: filepath.Base(strings.TrimSuffix(rel, ".md")), Description: "a goal", Module: "telos", Kind: "goal", Scope: "global",
		Fields: map[string]string{"id": "G3", "title": "Ship the guide", "ideal": "published", "by": "2026-12-01", "claims": claims}, Body: "b"}, "test-machine"); err != nil {
		t.Fatal(err)
	}
}

// §8.1: the kernel refuses an adapter the module did not declare, and hands
// the arguments to whoever knows what they mean.
func TestAWriteValidatesTheClaimsBlockAgainstTheModuleAndTheHook(t *testing.T) {
	s := newTestStore(t, newTestRemote(t)) // testModules' telos declares adapters tracker, forge, date, manual
	writeGoal(t, s, "telos/goal/g3.md", goalClaims)
	_, err := s.Write("telos/goal/g4.md", Record{Name: "g4", Description: "d", Module: "telos", Kind: "goal", Scope: "global",
		Fields: map[string]string{"id": "G4", "title": "t", "ideal": "i", "by": "2026-12-01", "claims": "- text: \"x\"\n  check: {adapter: wearable}"}, Body: "b"}, "m")
	if err == nil || !strings.Contains(err.Error(), "wearable") || !strings.Contains(err.Error(), "claim 1") {
		t.Errorf("an undeclared adapter must be refused naming the claim: %v", err)
	}
	s.ValidateClaim = func(adapter string, args map[string]string) error {
		if args["since"] == "-0d" {
			return fmt.Errorf("since: -0d is no interval")
		}
		return nil
	}
	_, err = s.Write("telos/goal/g5.md", Record{Name: "g5", Description: "d", Module: "telos", Kind: "goal", Scope: "global",
		Fields: map[string]string{"id": "G5", "title": "t", "ideal": "i", "by": "2026-12-01", "claims": "- text: \"x\"\n  check: {adapter: forge, repo: a/b, since: -0d, min: 1}"}, Body: "b"}, "m")
	if err == nil || !strings.Contains(err.Error(), "-0d") || !strings.Contains(err.Error(), "claim 1") {
		t.Errorf("the hook's refusal must surface with the claim's index: %v", err)
	}
}

// strictHook stands in for the adapters' own validation (Task 8): a relative
// since of -0d is no interval, and a counting adapter needs its min.
func strictHook(adapter string, args map[string]string) error {
	if args["since"] == "-0d" {
		return fmt.Errorf("since: -0d is no interval")
	}
	if (adapter == "tracker" || adapter == "forge") && args["min"] == "" {
		return fmt.Errorf("%s counts, so it needs min", adapter)
	}
	return nil
}

// §8.1: a bad claim is refused at write time by its 1-based position, so the
// person is told which of several claims to fix — here the second, after a
// first that is fine — and nothing is written.
func TestABadClaimIsRefusedAtWriteTimeByItsPositionInTheBlock(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	s.ValidateClaim = strictHook
	first := "- text: \"The target date still holds\"\n  check: {adapter: manual}\n"
	head := run(t, s.Dir, "rev-parse", "HEAD")
	for name, c := range map[string]struct{ second, want string }{
		"an adapter the module did not declare": {"- text: \"steps\"\n  check: {adapter: wearable, min: 3}", "wearable"},
		"a relative since of -0d":               {"- text: \"a commit\"\n  check: {adapter: forge, repo: a/b, since: -0d, min: 1}", "-0d"},
		"a counting claim without min":          {"- text: \"articles\"\n  check: {adapter: tracker, label: article}", "needs min"},
	} {
		_, err := s.Write("telos/goal/g6.md", Record{Name: "g6", Description: "d", Module: "telos", Kind: "goal", Scope: "global",
			Fields: map[string]string{"id": "G6", "title": "t", "ideal": "i", "by": "2026-12-01", "claims": first + c.second}, Body: "b"}, "m")
		if err == nil || !strings.Contains(err.Error(), "claim 2") || strings.Contains(err.Error(), "claim 1") || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want a refusal naming claim 2 and %q, got %v", name, c.want, err)
		}
	}
	if run(t, s.Dir, "rev-parse", "HEAD") != head {
		t.Error("a refused claims block must not reach git")
	}
}

// A corrected review replaces fields as a write would, so it validates the
// claims block the same way (§8.1, §9).
func TestACorrectedReviewValidatesTheClaimsBlockToo(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	s.ValidateClaim = strictHook
	writeGoal(t, s, "telos/goal/g3.md", goalClaims)
	_, err := s.Review("telos/goal/g3.md", ReviewInput{Question: "Still right?", Verdict: Corrected, Answer: "add a commit claim",
		Fields: map[string]string{"claims": goalClaims + "\n- text: \"a commit\"\n  check: {adapter: forge, repo: a/b, since: -14d}"}}, "m")
	if err == nil || !strings.Contains(err.Error(), "claim 3") || !strings.Contains(err.Error(), "needs min") {
		t.Errorf("a corrected review must validate the claims block: %v", err)
	}
}
