package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeValue(t *testing.T, s *Store, rel, statement string) {
	t.Helper()
	if _, err := s.Write(rel, Record{Name: filepath.Base(strings.TrimSuffix(rel, ".md")), Description: statement,
		Module: "identity", Kind: "value", Scope: "global", Fields: map[string]string{"statement": statement}, Body: statement}, "test-machine"); err != nil {
		t.Fatal(err)
	}
}

func metaOf(t *testing.T, s *Store, rel string) Meta {
	t.Helper()
	_, meta := ParseRecord(mustRead(t, filepath.Join(s.Dir, rel)))
	return meta
}

func TestConfirmedMovesReviewedAndNothingElse(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeValue(t, s, "identity/value/family.md", "family time")
	before := metaOf(t, s, "identity/value/family.md")
	setClock(t, "2026-10-01T09:00:00Z")
	commit, err := s.Review("identity/value/family.md", "Still one of the things you weigh decisions against?", Answer{Verdict: Confirmed}, "test-machine")
	if err != nil {
		t.Fatal(err)
	}
	after := metaOf(t, s, "identity/value/family.md")
	if after.Reviewed.Format(time.RFC3339) != "2026-10-01T09:00:00Z" {
		t.Errorf("reviewed = %v", after.Reviewed)
	}
	if !after.Updated.Equal(before.Updated) {
		t.Errorf("updated moved on a confirmation: %v -> %v", before.Updated, after.Updated)
	}
	if after.Snoozes != 0 || !after.Retired.IsZero() {
		t.Errorf("meta = %+v", after)
	}
	subject := run(t, s.Dir, "log", "-1", "--format=%s%n%b")
	for _, want := range []string{"review identity/value", "Q: Still one of the things", "A: confirmed"} {
		if !strings.Contains(subject, want) {
			t.Errorf("commit message lacks %q:\n%s", want, subject)
		}
	}
	if commit == "" || run(t, s.Dir, "rev-parse", "HEAD") != run(t, s.Dir, "rev-parse", "origin/main") {
		t.Error("the review must be committed and pushed")
	}
}

func TestLaterCountsASnoozeAndLeavesReviewedAlone(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeValue(t, s, "identity/value/craft.md", "craft")
	for i := 1; i <= 2; i++ {
		if _, err := s.Review("identity/value/craft.md", "Still?", Answer{Verdict: Later}, "test-machine"); err != nil {
			t.Fatal(err)
		}
		if m := metaOf(t, s, "identity/value/craft.md"); m.Snoozes != i || !m.Reviewed.IsZero() {
			t.Errorf("after %d laters: %+v", i, m)
		}
	}
}

func TestRetiredStampsRetiredAndReviewed(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeValue(t, s, "identity/value/old.md", "an old value")
	setClock(t, "2026-10-01T09:00:00Z")
	if _, err := s.Review("identity/value/old.md", "Still?", Answer{Verdict: Retired}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	m := metaOf(t, s, "identity/value/old.md")
	if m.Retired.IsZero() || m.Reviewed.IsZero() {
		t.Errorf("meta = %+v", m)
	}
}

func TestCorrectedReplacesContentAndIsValidatedLikeAWrite(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeValue(t, s, "identity/value/v.md", "first wording")
	setClock(t, "2026-10-01T09:00:00Z")
	if _, err := s.Review("identity/value/v.md", "Still?", Answer{Verdict: Corrected, Body: "second wording", Fields: map[string]string{"statement": "second wording"}}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	r, m := ParseRecord(mustRead(t, filepath.Join(s.Dir, "identity/value/v.md")))
	if r.Body != "second wording\n" || r.Fields["statement"] != "second wording" {
		t.Errorf("record = %+v", r)
	}
	if m.Updated.Format(time.RFC3339) != "2026-10-01T09:00:00Z" || m.Reviewed.IsZero() {
		t.Errorf("a correction changes content, so updated and reviewed both move: %+v", m)
	}
	_, err := s.Review("identity/value/v.md", "Still?", Answer{Verdict: Corrected, Fields: map[string]string{"colour": "red"}}, "test-machine")
	if err == nil || !strings.Contains(err.Error(), "colour") {
		t.Errorf("an undeclared field in a correction must be refused: %v", err)
	}
	_, err = s.Review("identity/value/v.md", "Still?", Answer{Verdict: Corrected}, "test-machine")
	if err == nil {
		t.Error("a correction with nothing to correct is a mistake, not a confirmation")
	}
}

// A.1: the question reaches git in the commit message, so it is checked the
// same way a name, description, body or field is (spec §11).
func TestReviewRefusesAQuestionThatLooksLikeACredential(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeValue(t, s, "identity/value/v.md", "v")
	before := run(t, s.Dir, "rev-parse", "HEAD")
	question := "is " + "password: " + strings.Repeat("x", 20) + " still right?"
	_, err := s.Review("identity/value/v.md", question, Answer{Verdict: Confirmed}, "test-machine")
	if err == nil || !strings.Contains(err.Error(), "password") {
		t.Fatalf("a credential-shaped question must be refused, naming the shape: %v", err)
	}
	if run(t, s.Dir, "rev-parse", "HEAD") != before {
		t.Error("HEAD moved on a refused review")
	}
}

// The correction path already checked a.Body for a credential shape; this was
// the missing test for it.
func TestReviewRefusesACorrectedBodyThatLooksLikeACredential(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeValue(t, s, "identity/value/v.md", "v")
	before := run(t, s.Dir, "rev-parse", "HEAD")
	body := "the " + "api_key: " + strings.Repeat("x", 20) + " changed"
	_, err := s.Review("identity/value/v.md", "Still?", Answer{Verdict: Corrected, Body: body}, "test-machine")
	if err == nil || !strings.Contains(err.Error(), "correction") {
		t.Fatalf("a credential-shaped correction body must be refused: %v", err)
	}
	if run(t, s.Dir, "rev-parse", "HEAD") != before {
		t.Error("HEAD moved on a refused review")
	}
}

// B: a correction to a crossing record validates against the kind that
// governs it, not its own — a memory/preference is governed by identity's
// preference kind (spec §7), which declares "statement".
func TestCorrectedFieldsValidateAgainstTheGoverningKindForACrossingRecord(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("personal/terse.md", Record{Name: "terse", Description: "terse answers", Module: "memory", Kind: "preference", Scope: "global", Body: "Prefers terse answers."}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Review("personal/terse.md", "Still how you want to be worked with?", Answer{Verdict: Corrected, Fields: map[string]string{"statement": "second"}}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	r, _ := ParseRecord(mustRead(t, filepath.Join(s.Dir, "personal/terse.md")))
	if r.Fields["statement"] != "second" {
		t.Errorf("statement = %q, want it stored under identity's rule for preference", r.Fields["statement"])
	}

	if _, err := s.Write("personal/note.md", Record{Name: "note", Description: "a note", Module: "memory", Kind: "note", Scope: "global", Body: "note body"}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	_, err := s.Review("personal/note.md", "Still?", Answer{Verdict: Corrected, Fields: map[string]string{"statement": "x"}}, "test-machine")
	if err == nil || !strings.Contains(err.Error(), "statement") {
		t.Errorf("a memory/note correction with an undeclared field must be refused: %v", err)
	}
}

func TestReviewRefusesWhatItCannotReview(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeValue(t, s, "identity/value/v.md", "v")
	for name, try := range map[string]func() error{
		"empty question": func() error {
			_, err := s.Review("identity/value/v.md", "  ", Answer{Verdict: Confirmed}, "m")
			return err
		},
		"unknown verdict": func() error {
			_, err := s.Review("identity/value/v.md", "q", Answer{Verdict: "maybe"}, "m")
			return err
		},
		"missing file": func() error {
			_, err := s.Review("identity/value/nope.md", "q", Answer{Verdict: Confirmed}, "m")
			return err
		},
		"pre-module file": func() error {
			_, err := s.Review("personal/style.md", "q", Answer{Verdict: Confirmed}, "m")
			return err
		},
	} {
		if err := try(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestAWorkingMemoryRecordCanBeReviewedWhichPromotesIt(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	if _, err := s.Write("personal/terse.md", Record{Name: "terse", Description: "terse answers", Module: "memory", Kind: "preference", Scope: "global", Body: "Prefers terse answers."}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Review("personal/terse.md", "Still how you want to be worked with?", Answer{Verdict: Confirmed}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	if m := metaOf(t, s, "personal/terse.md"); m.Reviewed.IsZero() {
		t.Error("a confirmed working-memory preference carries reviewed (§7: model proposes, person ratifies, one file)")
	}
}

// The acceptance clause, as history: reviewed moves only on review commits.
func TestReviewedMovesOnlyOnReviewCommits(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeValue(t, s, "identity/value/a.md", "a")
	if _, err := s.Review("identity/value/a.md", "q", Answer{Verdict: Confirmed}, "m"); err != nil {
		t.Fatal(err)
	}
	writeValue(t, s, "identity/value/a.md", "a, reworded") // a plain rewrite after the review
	if _, err := s.Review("identity/value/a.md", "q", Answer{Verdict: Later}, "m"); err != nil {
		t.Fatal(err)
	}
	// A sentinel no diff line can produce marks each commit's subject; a
	// first-character heuristic would misread a hash beginning with a hex
	// letter that also starts a diff keyword.
	log := run(t, s.Dir, "log", "--format=COMMIT%x09%s", "-p", "--", "identity/value/a.md")
	var subject string
	var sawReviewedAdded int
	for _, line := range strings.Split(log, "\n") {
		if rest, ok := strings.CutPrefix(line, "COMMIT\t"); ok {
			subject = rest
			continue
		}
		if strings.HasPrefix(line, "+reviewed:") {
			sawReviewedAdded++
		}
		if (strings.HasPrefix(line, "+reviewed:") || strings.HasPrefix(line, "-reviewed:")) && !strings.HasPrefix(subject, "review ") {
			t.Errorf("reviewed changed in a commit that is not a review: %q", subject)
		}
	}
	// A vacuous pass — an empty log, or one where Review stopped writing
	// reviewed at all — would satisfy the loop above having found nothing to
	// object to. The claim is that reviewed moves only on review commits, not
	// that it never moves; this asserts it actually moved at least once.
	if sawReviewedAdded == 0 {
		t.Fatal("no +reviewed: line seen in the log; the test proves nothing without one")
	}
}

// F: the four writers share one git tail (commitAndPush), whose guard is a
// backstop — a second Confirmed under the same frozen clock recomposes the
// same bytes, so there is nothing to commit.
func TestASecondIdenticalConfirmedUnderAFrozenClockMakesNoCommit(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeValue(t, s, "identity/value/v.md", "v")
	setClock(t, "2026-10-01T09:00:00Z")
	if _, err := s.Review("identity/value/v.md", "Still?", Answer{Verdict: Confirmed}, "test-machine"); err != nil {
		t.Fatal(err)
	}
	before := run(t, s.Dir, "rev-parse", "HEAD")
	commit, err := s.Review("identity/value/v.md", "Still?", Answer{Verdict: Confirmed}, "test-machine")
	if err != nil {
		t.Fatal(err)
	}
	if commit != "no change" {
		t.Errorf("commit = %q, want \"no change\"", commit)
	}
	if run(t, s.Dir, "rev-parse", "HEAD") != before {
		t.Error("a second identical confirmation must not commit")
	}
}

func TestRecordsEnumeratesEverythingWithMeta(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeValue(t, s, "identity/value/a.md", "a")
	all, err := s.Records()
	if err != nil {
		t.Fatal(err)
	}
	var sawValue, sawLegacy, sawIndex bool
	for _, r := range all {
		switch r.Path {
		case "identity/value/a.md":
			sawValue = r.Module == "identity" && r.Kind == "value" && !r.Updated.IsZero()
		case "personal/style.md":
			sawLegacy = r.Module == "" && r.LegacyType == "feedback"
		case "MEMORY.md", "CONVENTIONS.md":
			sawIndex = true
		}
	}
	if !sawValue || !sawLegacy || sawIndex {
		t.Errorf("value=%v legacy=%v index=%v", sawValue, sawLegacy, sawIndex)
	}
}

// Task 3's review found that a kernel key which fails to parse must stop a
// Write rather than be silently dropped (store.go's Malformed check). Review
// carries the same risk — composing after applying a verdict would erase
// whichever key never parsed — so it refuses on the same condition, with the
// same wording, and leaves the file untouched.
func TestReviewRefusesARecordWithAMalformedKernelKey(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	rel := "identity/value/bad.md"
	full := filepath.Join(s.Dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: bad\ndescription: d\nmodule: identity\nkind: value\nscope: global\nstatement: s\nupdated: 2026-09-01T00:00:00Z\nreviewed: 2026-09-05\n---\n\ns\n"
	if err := os.WriteFile(full, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	run(t, s.Dir, "add", "-A")
	run(t, s.Dir, "commit", "-q", "-m", "seed a bare-date reviewed")
	run(t, s.Dir, "push", "-q", "origin", "main")

	_, err := s.Review(rel, "Still?", Answer{Verdict: Confirmed}, "test-machine")
	if err == nil || !strings.Contains(err.Error(), "reviewed") || !strings.Contains(err.Error(), "nothing was written") {
		t.Fatalf("a malformed reviewed key must refuse the review, naming the key: %v", err)
	}
	if got := mustRead(t, full); got != content {
		t.Errorf("the file changed on a refused review:\n%s", got)
	}
}
