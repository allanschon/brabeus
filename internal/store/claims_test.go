package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// at is the store tests' one stamp parser: a fixture with a stamp that
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
		"two texts":    "- text: \"x\"\n  text: \"y\"\n  check: {adapter: manual}",
		"two checks":   "- text: \"x\"\n  check: {adapter: manual}\n  check: {adapter: tracker}",
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

// strictHook stands in for the adapters' own validation: a relative
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

// §8.1, AN: a result never changes the goal's content, its updated stamp or
// its history; it lives under claims/ and is committed only when a state changes.
func TestAClaimResultNeverTouchesTheGoalAndCommitsOnlyOnAChange(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeGoal(t, s, "telos/goal/g3.md", goalClaims)
	goalBefore := mustRead(t, filepath.Join(s.Dir, "telos/goal/g3.md"))
	logBefore := run(t, s.Dir, "log", "--format=%s", "--", "telos/goal/g3.md")
	now := at("2026-09-20T04:00:00Z")
	results := []ClaimResult{
		{Index: 0, Text: "At least three articles published since the quarter began", Adapter: "tracker", State: Fail, Detail: "2 found", Since: now, Recorded: now},
		{Index: 1, Text: "The target date still holds", Adapter: "manual", State: Unchecked},
	}
	commit, err := s.RecordClaimResults("telos/goal/g3.md", results, "kernel")
	if err != nil || commit == "" || commit == "no change" {
		t.Fatalf("commit=%q err=%v", commit, err)
	}
	if mustRead(t, filepath.Join(s.Dir, "telos/goal/g3.md")) != goalBefore {
		t.Error("the goal file changed")
	}
	if run(t, s.Dir, "log", "--format=%s", "--", "telos/goal/g3.md") != logBefore {
		t.Error("the goal's history gained a commit")
	}
	subject := run(t, s.Dir, "log", "-1", "--format=%s")
	if !strings.HasPrefix(subject, "claims telos/goal g3: 1 changed") {
		t.Errorf("subject = %q", subject)
	}
	var file struct {
		Goal   string        `json:"goal"`
		Claims []ClaimResult `json:"claims"`
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, "claims/telos/goal/g3.json"))
	if err != nil || json.Unmarshal(b, &file) != nil || file.Goal != "telos/goal/g3.md" || len(file.Claims) != 2 || file.Claims[0].State != Fail {
		t.Fatalf("results file: %v %s", err, b)
	}
	if commit, err := s.RecordClaimResults("telos/goal/g3.md", results, "kernel"); err != nil || commit != "no change" {
		t.Errorf("the same results again must not commit: %q %v", commit, err)
	}
	all, err := s.ClaimResults()
	if err != nil || len(all["telos/goal/g3.md"]) != 2 || all["telos/goal/g3.md"][0].Detail != "2 found" {
		t.Errorf("ClaimResults = %+v %v", all, err)
	}
	if _, err := s.Write("claims/telos/goal/x.md", Record{Name: "x", Description: "d", Module: "memory", Kind: "note", Scope: "global", Body: "b"}, "m"); err == nil {
		t.Error("claims/ is the kernel's; a record may not be written there")
	}
	entries, _ := s.List("")
	for _, e := range entries {
		if strings.HasPrefix(e.Path, "claims/") {
			t.Errorf("a results file is not a record: %+v", e)
		}
	}
}

// The rule is "committed only when something the person or the adapter
// said changed", judged on State and Detail, not on the file's bytes. A run
// that only restamps Recorded commits nothing; a new manual answer in the
// same state is recorded (§8.1) and says so.
func TestAResultCommitsOnAStateOrANewAnswerAndNeverOnARestamp(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeGoal(t, s, "telos/goal/g3.md", goalClaims)
	first, later := at("2026-09-20T04:00:00Z"), at("2026-09-21T04:00:00Z")
	manual := func(detail string, when time.Time) []ClaimResult {
		return []ClaimResult{{Index: 1, Text: "The target date still holds", Adapter: "manual", State: Pass, Detail: detail, Since: when, Recorded: when}}
	}
	if c, err := s.RecordClaimResults("telos/goal/g3.md", manual("holds, said the person", first), "desk"); err != nil || c == "no change" {
		t.Fatalf("first answer: %q %v", c, err)
	}
	if c, err := s.RecordClaimResults("telos/goal/g3.md", manual("holds, said the person", later), "desk"); err != nil || c != "no change" {
		t.Errorf("a restamp alone must not commit: %q %v", c, err)
	}
	if c, err := s.RecordClaimResults("telos/goal/g3.md", manual("still holds, the venue confirmed", later), "desk"); err != nil || c == "no change" {
		t.Fatalf("a new answer must commit: %q %v", c, err)
	}
	if subject := run(t, s.Dir, "log", "-1", "--format=%s"); !strings.HasPrefix(subject, "claims telos/goal g3: 0 changed, 1 answered") {
		t.Errorf("subject = %q", subject)
	}
	got, _ := s.ClaimResults()
	if r := got["telos/goal/g3.md"]; len(r) != 1 || !r[0].Since.Equal(first) || r[0].Detail != "still holds, the venue confirmed" {
		t.Errorf("Since is when the state was entered and survives a new answer: %+v", r)
	}
}

// Two writers of one goal's results — the scheduled run and a manual
// answer — merge under the store's lock, so neither loses the other's entry,
// and the caller's slice is not reordered behind its back.
func TestUpdateClaimResultsMergesWithWhatIsAlreadyRecorded(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeGoal(t, s, "telos/goal/g3.md", goalClaims)
	now := at("2026-09-20T04:00:00Z")
	mine := []ClaimResult{
		{Index: 1, Text: "The target date still holds", Adapter: "manual", State: Pass, Since: now, Recorded: now},
		{Index: 0, Text: "At least three articles published since the quarter began", Adapter: "tracker", State: Fail, Detail: "2 found", Since: now, Recorded: now},
	}
	if _, err := s.RecordClaimResults("telos/goal/g3.md", mine, "kernel"); err != nil {
		t.Fatal(err)
	}
	if mine[0].Index != 1 {
		t.Error("RecordClaimResults sorted the caller's slice")
	}
	_, err := s.UpdateClaimResults("telos/goal/g3.md", func(prev []ClaimResult) []ClaimResult {
		if len(prev) != 2 {
			t.Errorf("prev = %+v", prev)
		}
		out := append([]ClaimResult{}, prev...)
		for i := range out {
			if out[i].Index == 0 {
				out[i].State, out[i].Detail, out[i].Since = Pass, "3 found", now.Add(time.Hour)
			}
		}
		return out
	}, "kernel")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.ClaimResults()
	r := got["telos/goal/g3.md"]
	if len(r) != 2 || r[0].Index != 0 || r[0].State != Pass || r[1].State != Pass || r[1].Adapter != "manual" {
		t.Errorf("merged results = %+v", r)
	}
}

// A result's detail enters git, so it meets the same rules as any text that
// does: one line, and no credential (§11). A state outside §8.1's is refused.
func TestAResultIsRefusedAnUnknownStateOrACredentialInItsDetail(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeGoal(t, s, "telos/goal/g3.md", goalClaims)
	one := func(state ClaimState, detail string) []ClaimResult {
		return []ClaimResult{{Index: 1, Text: "The target date still holds", Adapter: "manual", State: state, Detail: detail}}
	}
	if _, err := s.RecordClaimResults("telos/goal/g3.md", one("maybe", ""), "m"); err == nil || !strings.Contains(err.Error(), "maybe") {
		t.Errorf("an unknown state: %v", err)
	}
	if _, err := s.RecordClaimResults("telos/goal/g3.md", one(Pass, "token="+strings.Repeat("x", 24)), "m"); err == nil || !strings.Contains(err.Error(), "§11") {
		t.Errorf("a credential in a detail: %v", err)
	}
	if _, err := s.RecordClaimResults("telos/goal/g3.md", one(Pass, "held\nand then\n  some"), "m"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.ClaimResults()
	if d := got["telos/goal/g3.md"][0].Detail; d != "held and then some" {
		t.Errorf("detail = %q", d)
	}
	if _, err := s.RecordClaimResults("telos/goal/none.md", one(Pass, ""), "m"); err == nil {
		t.Error("results for a goal that is not there")
	}
}

// One join, used by the agenda, the claims tool and the runner: a result
// belongs to a claim when both its position and its text match, so a result
// left from a reworded claim reads as unchecked rather than as the new one's.
func TestJoinResultsMatchesByPositionAndTextAndOtherwiseIsUnchecked(t *testing.T) {
	claims, _ := ParseClaims(goalClaims)
	now := at("2026-09-20T04:00:00Z")
	joined := JoinResults(claims, []ClaimResult{
		{Index: 0, Text: "Two articles", Adapter: "tracker", State: Fail, Since: now},
		{Index: 1, Text: "The target date still holds", Adapter: "manual", State: Pass, Detail: "yes", Since: now},
		{Index: 5, Text: "gone", State: Fail},
	})
	if len(joined) != 2 || joined[0].State != Unchecked || joined[0].Text != claims[0].Text || joined[0].Adapter != "tracker" || joined[0].Index != 0 {
		t.Errorf("joined[0] = %+v", joined)
	}
	if joined[1].State != Pass || joined[1].Detail != "yes" || !joined[1].Since.Equal(now) {
		t.Errorf("joined[1] = %+v", joined[1])
	}
}

// claims/ is the kernel's: a markdown file placed there by hand is not a
// record, since no tool could reach it to change or remove it.
func TestAMarkdownFileUnderClaimsIsNeitherListedNorSearched(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeGoal(t, s, "telos/goal/g3.md", goalClaims)
	stray := filepath.Join(s.Dir, "claims", "stray.md")
	if err := os.MkdirAll(filepath.Dir(stray), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stray, []byte("---\nname: stray\ndescription: zebra marker\nscope: global\n---\n\nzebra\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	err := s.reindex()
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := s.List("")
	for _, e := range entries {
		if strings.HasPrefix(e.Path, "claims/") {
			t.Errorf("listed: %+v", e)
		}
	}
	if hits, _, _ := s.Search("zebra", 10, SearchFilter{}); len(hits) != 0 {
		t.Errorf("searched: %+v", hits)
	}
}
