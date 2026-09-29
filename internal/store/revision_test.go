package store

import (
	"os"
	"path/filepath"
	"testing"
)

// Under the freeze, a confirmed record's content changes only through a
// corrected review, which moves reviewed to the same stamp as the change. So
// the history that actually produces a revision line today is not "a
// confirmed record was reviewed, then rewritten" (the freeze refuses that
// rewrite) but a plain rewrite committed outside the review path — the shape
// every record written before the freeze existed, or a record touched by
// something other than the kernel. This test hand-seeds exactly that shape,
// the way TestReviewRefusesARecordWithAMalformedKernelKey seeds a malformed
// file: a commit made with run(t, ...) rather than through Store.Write.
func TestRevisionNamesTheFieldsThatChangedSinceTheLastReview(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	goal := Record{Name: "g3", Description: "ship the guide", Module: "telos", Kind: "goal", Scope: "global",
		Fields: map[string]string{"id": "G3", "title": "Ship the guide", "ideal": "published", "by": "2026-10-01"}, Body: "b"}
	if _, err := s.Write("telos/goal/g3.md", goal, "m"); err != nil {
		t.Fatal(err)
	}
	setClock(t, "2026-08-01T09:00:00Z")
	if _, err := s.Review("telos/goal/g3.md", ReviewInput{Question: "Still right?", Verdict: Confirmed, Answer: "yes"}, "m"); err != nil {
		t.Fatal(err)
	}

	rel := "telos/goal/g3.md"
	full := filepath.Join(s.Dir, filepath.FromSlash(rel))
	content := "---\nname: g3\ndescription: ship the guide\nmodule: telos\nkind: goal\nid: G3\nscope: global\n" +
		"title: Ship the guide\nideal: published\nby: 2026-12-01\n" +
		"updated: 2026-09-04T09:00:00Z\nreviewed: 2026-08-01T09:00:00Z\n---\n\nb\n"
	if err := os.WriteFile(full, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	run(t, s.Dir, "add", "-A")
	run(t, s.Dir, "commit", "-q", "-m", "hand-seed a pre-freeze-style rewrite of a reviewed record")
	run(t, s.Dir, "push", "-q", "origin", "main")

	_, meta := ParseRecord(mustRead(t, full))
	line, err := s.Revision(rel, meta)
	if err != nil {
		t.Fatal(err)
	}
	if line != "by moved from 2026-10-01 to 2026-12-01 on 2026-09-04" {
		t.Errorf("line = %q", line)
	}

	all, err := s.Records()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range all {
		if r.Path == rel && r.Revision != line {
			t.Errorf("Records()'s Revision = %q, want %q", r.Revision, line)
		}
	}
}

// A corrected review is itself the review that moves reviewed, so nothing has
// changed since the last review the moment it lands — the revision line is
// empty, and Records() must not compute one for a record whose updated does
// not follow reviewed.
func TestRevisionIsEmptyAfterACorrectedReviewSinceThatIsItselfTheReview(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	goal := Record{Name: "g3", Description: "ship the guide", Module: "telos", Kind: "goal", Scope: "global",
		Fields: map[string]string{"id": "G3", "title": "Ship the guide", "ideal": "published", "by": "2026-10-01"}, Body: "b"}
	if _, err := s.Write("telos/goal/g3.md", goal, "m"); err != nil {
		t.Fatal(err)
	}
	setClock(t, "2026-08-01T09:00:00Z")
	if _, err := s.Review("telos/goal/g3.md", ReviewInput{Question: "Still right?", Verdict: Confirmed, Answer: "yes"}, "m"); err != nil {
		t.Fatal(err)
	}
	// A draft-style rewrite is refused after a review, so the change
	// that produces a revision line is a corrected review, and the line is
	// what a later interview shows beside the next question.
	setClock(t, "2026-09-04T09:00:00Z")
	if _, err := s.Review("telos/goal/g3.md", ReviewInput{Question: "Still right?", Verdict: Corrected, Answer: "December now", Fields: map[string]string{"by": "2026-12-01"}}, "m"); err != nil {
		t.Fatal(err)
	}
	_, meta := ParseRecord(mustRead(t, filepath.Join(s.Dir, "telos/goal/g3.md")))
	line, err := s.Revision("telos/goal/g3.md", meta)
	if err != nil {
		t.Fatal(err)
	}
	if line != "" {
		t.Errorf("a corrected review is a review; nothing has changed since it: %q", line)
	}
	all, _ := s.Records()
	for _, r := range all {
		if r.Path == "telos/goal/g3.md" && r.Revision != "" {
			t.Errorf("Records must not compute a revision for a record whose updated does not follow reviewed: %+v", r)
		}
	}
}
