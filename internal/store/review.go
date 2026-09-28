package store

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Verdict string

const (
	Confirmed Verdict = "confirmed"
	Corrected Verdict = "corrected"
	Retired   Verdict = "retired"
	Later     Verdict = "later"
)

// ReviewInput is everything a review is given (the glossary's "review
// input"): the question asked, the verdict, the person's answer in their own
// words, and for corrected the new content. Question and Answer are refused
// empty in one place, so a reviewed date in history always has both behind it.
type ReviewInput struct {
	Question string
	Verdict  Verdict
	Answer   string
	Body     string
	Fields   map[string]string
}

// Review applies the answer to one record. It is the only path that moves
// reviewed (spec §9): a plain Write carries the old value through untouched,
// so a reviewed date in history is, by construction, a question that was
// asked and answered — and both are in the commit message, so the answer can
// be read back.
func (s *Store) Review(rel string, in ReviewInput, caller string) (string, error) {
	if s.ReadOnly {
		return "", fmt.Errorf("this store is read-only")
	}
	if s.modules == nil {
		return "", fmt.Errorf("no module set is loaded")
	}
	question, answer := oneLine(in.Question), oneLine(in.Answer)
	if question == "" {
		return "", fmt.Errorf("a review carries the question that was asked; none was given")
	}
	if answer == "" {
		return "", fmt.Errorf("a review carries the person's answer in their own words; none was given (spec §9)")
	}
	// A fixed order, not a map range: the shape check runs before either
	// value reaches git, and the refusal it names must not vary run to run.
	for _, c := range [...]struct{ what, text string }{{"question", question}, {"answer", answer}} {
		if shape := credentialShape(c.text); shape != "" {
			return "", fmt.Errorf("refused: the %s looks like it contains %s; credentials never enter the record (spec §11)", c.what, shape)
		}
	}
	switch in.Verdict {
	case Confirmed, Corrected, Retired, Later:
	default:
		return "", fmt.Errorf("verdict %q: use confirmed, corrected, retired or later", in.Verdict)
	}
	rel, err := MemoryPath(rel)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	full, err := resolvePath(s.Dir, rel)
	if err != nil {
		return "", err
	}
	if err := s.sync(); err != nil {
		return "", err
	}
	old, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("no record at %q", rel)
	}
	r, meta := ParseRecord(string(old))
	// A kernel-owned key that failed to parse must stop the review rather
	// than be silently dropped: compose only emits reviewed/retired/snoozes
	// when they're non-zero, so composing anyway would erase whichever one
	// did not parse — and only a review may move them (§9). Same rule and
	// wording as Write's refusal.
	if len(meta.Malformed) > 0 {
		return "", malformedError(parseFrontmatter(string(old)), meta.Malformed)
	}
	if r.Module == "" {
		return "", fmt.Errorf("%s predates modules; run the migration before reviewing it", rel)
	}
	man, ok := s.modules.Module(r.Module)
	if !ok {
		return "", fmt.Errorf("module %q is not enabled", r.Module)
	}
	kind, ok := man.Kinds[r.Kind]
	if !ok {
		return "", fmt.Errorf("module %s has no kind %q", r.Module, r.Kind)
	}
	// A retired record has already left the record (§9): reviewing it again
	// would let a second verdict overwrite the one that retired it.
	if !meta.Retired.IsZero() {
		return "", fmt.Errorf("%s was retired on %s and has left the record; write the new statement at a new path if it applies again (spec §9)",
			rel, meta.Retired.UTC().Format("2006-01-02"))
	}
	// review accepts only a record governed by a ratified-record module
	// (spec §1.1, §9): a working-memory note has no confirmed state for a
	// review to set, and a crossing preference is governed by the
	// ratified-record module it crosses into (module.Set.RuleFor, spec §7).
	gov, govKind, ok := s.modules.RuleFor(r.Module, r.Kind)
	if bundle, _ := gov.Profile.Bundle(); !ok || !bundle.Interviewed {
		return "", fmt.Errorf("%s is governed by %s, a working-memory module; review confirms the person's record and accepts only a ratified-record kind (spec §1.1)", rel, r.Module)
	}

	stamp := now()
	fieldOrder := append(append([]string{}, kind.Fields...), kind.Optional...)
	switch in.Verdict {
	case Confirmed:
		meta.Reviewed = stamp
	case Corrected:
		if strings.TrimSpace(in.Body) == "" && len(in.Fields) == 0 {
			return "", fmt.Errorf("corrected with nothing to correct; use confirmed if the record is right")
		}
		if in.Body != "" {
			if shape := credentialShape(in.Body); shape != "" {
				return "", fmt.Errorf("refused: the correction looks like it contains %s (spec §11)", shape)
			}
			r.Body = in.Body
		}
		for k, v := range in.Fields {
			if shape := credentialShape(v); shape != "" {
				return "", fmt.Errorf("refused: field %s looks like it contains %s (spec §11)", k, shape)
			}
			r.Fields[k] = v
		}
		// Only a correction that actually supplies a field is checked against
		// a kind at all: a body-only correction changes nothing about the
		// fields, so it must pass exactly as a plain Write of the same record
		// would, not be re-validated against a kind whose required fields it
		// never claimed to touch (a memory/preference body-only correction
		// was refused for lacking identity's required "statement" before this
		// fix, though nothing about statement was being corrected).
		if len(in.Fields) > 0 {
			// gov/govKind are the kind that governs a crossing record (spec
			// §7): a memory/preference correction may carry identity's
			// "statement". The gate above already refused anything not
			// governed by a ratified-record module, so govKind here is
			// never the zero Kind — unlike before that gate existed, this
			// no longer needs a fallback to the record's own kind.
			if err := checkFields(govKind, r.Fields); err != nil {
				return "", err
			}
			if err := s.checkClaims(gov, r.Fields); err != nil {
				return "", err
			}
			fieldOrder = append(append([]string{}, govKind.Fields...), govKind.Optional...)
		}
		r.ID = r.Fields["id"]
		meta.Updated, meta.Reviewed = stamp, stamp
	case Retired:
		meta.Retired, meta.Reviewed = stamp, stamp
	case Later:
		meta.Snoozes++
	}

	content := compose(r, meta, fieldOrder)
	if err := os.WriteFile(full, []byte(content), 0o640); err != nil {
		return "", err
	}
	msg := fmt.Sprintf("review %s/%s %s: %s\n\nQ: %s\nVerdict: %s\nA: %s\n\nReviewed through the kernel at %s.",
		r.Module, r.Kind, r.Name, in.Verdict, question, in.Verdict, answer, time.Now().UTC().Format(time.RFC3339))
	return s.commitAndPush(msg, caller)
}

// Stored is one record with what the kernel knows about it.
type Stored struct {
	Path string
	Record
	Meta
	// Revision is the field-level change since the record's last review
	// (spec §9), filled below only for a record whose updated follows its
	// reviewed — the one case Revision itself does any work for.
	Revision string
}

// LessByReview orders two records confirmed first, most recently reviewed
// first, then unreviewed by most recently updated. Shared by block's
// per-kind grouping and agenda.Reflect's value ordering (C7), so the two
// cannot silently disagree about what "in review order" means.
func LessByReview(a, b Stored) bool {
	ai, bi := a.Reviewed.IsZero(), b.Reviewed.IsZero()
	if ai != bi {
		return !ai
	}
	if !ai {
		return a.Reviewed.After(b.Reviewed)
	}
	return a.Updated.After(b.Updated)
}

// Records enumerates every record with its metadata. The index and the
// conventions file are structure, not records. Pre-module files are included
// with Module empty, so the agenda can say "migrate first" rather than
// silently skipping them.
//
// The walk runs under the store's lock, but Revision is called after that
// lock is released — collected first, computed after — because Revision
// takes the lock itself; calling it from inside the walk would deadlock.
func (s *Store) Records() ([]Stored, error) {
	var out []Stored
	err := func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.walkMarkdown(func(rel, full string) error {
			if strings.EqualFold(rel, indexFile) || strings.EqualFold(rel, conventionsFile) {
				return nil
			}
			b, err := os.ReadFile(full)
			if err != nil {
				return nil
			}
			r, meta := ParseRecord(string(b))
			out = append(out, Stored{Path: rel, Record: r, Meta: meta})
			return nil
		})
	}()
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Reviewed.IsZero() || !out[i].Updated.After(out[i].Reviewed) {
			continue
		}
		// A git failure here must not fail the whole listing: the agenda
		// falls back to the M1 stamp wording when Revision is empty, so
		// degrading to that line is the right outcome, not an error.
		if line, err := s.Revision(out[i].Path, out[i].Meta); err == nil {
			out[i].Revision = line
		}
	}
	return out, nil
}
