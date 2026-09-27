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

// Answer is the person's reply to one question.
type Answer struct {
	Verdict Verdict
	Body    string
	Fields  map[string]string
}

// Review applies the answer to one record. It is the only path that moves
// reviewed (spec §9): a plain Write carries the old value through untouched,
// so a reviewed date in history is, by construction, a question that was
// asked and answered — and the question is in the commit message.
func (s *Store) Review(rel, question string, a Answer, caller string) (string, error) {
	if s.ReadOnly {
		return "", fmt.Errorf("this store is read-only")
	}
	if s.modules == nil {
		return "", fmt.Errorf("no module set is loaded")
	}
	question = oneLine(question)
	if question == "" {
		return "", fmt.Errorf("a review carries the question that was asked; none was given")
	}
	if shape := credentialShape(question); shape != "" {
		return "", fmt.Errorf("refused: the question looks like it contains %s; credentials never enter the record (spec §11)", shape)
	}
	switch a.Verdict {
	case Confirmed, Corrected, Retired, Later:
	default:
		return "", fmt.Errorf("verdict %q: use confirmed, corrected, retired or later", a.Verdict)
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

	stamp := now()
	fieldOrder := append(append([]string{}, kind.Fields...), kind.Optional...)
	switch a.Verdict {
	case Confirmed:
		meta.Reviewed = stamp
	case Corrected:
		if strings.TrimSpace(a.Body) == "" && len(a.Fields) == 0 {
			return "", fmt.Errorf("corrected with nothing to correct; use confirmed if the record is right")
		}
		if a.Body != "" {
			if shape := credentialShape(a.Body); shape != "" {
				return "", fmt.Errorf("refused: the correction looks like it contains %s (spec §11)", shape)
			}
			r.Body = a.Body
		}
		for k, v := range a.Fields {
			if shape := credentialShape(v); shape != "" {
				return "", fmt.Errorf("refused: field %s looks like it contains %s (spec §11)", k, shape)
			}
			r.Fields[k] = v
		}
		// A correction is validated and composed against the kind that
		// governs the record (module.Set.RuleFor), not necessarily its own:
		// a memory/preference is governed by identity's preference kind
		// (spec §7), so a correction to it may carry identity's fields.
		_, govKind, _ := s.modules.RuleFor(r.Module, r.Kind)
		if err := checkFields(govKind, r.Fields); err != nil {
			return "", err
		}
		fieldOrder = append(append([]string{}, govKind.Fields...), govKind.Optional...)
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
	msg := fmt.Sprintf("review %s/%s %s: %s\n\nQ: %s\nA: %s\n\nReviewed through the kernel at %s.",
		r.Module, r.Kind, r.Name, a.Verdict, question, a.Verdict, time.Now().UTC().Format(time.RFC3339))
	return s.commitAndPush(msg, caller)
}

// Stored is one record with what the kernel knows about it.
type Stored struct {
	Path string
	Record
	Meta
}

// Records enumerates every record with its metadata. The index and the
// conventions file are structure, not records. Pre-module files are included
// with Module empty, so the agenda can say "migrate first" rather than
// silently skipping them.
func (s *Store) Records() ([]Stored, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Stored
	err := s.walkMarkdown(func(rel, full string) error {
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
	return out, err
}
