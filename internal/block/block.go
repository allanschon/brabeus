// Package block renders the session context block (spec §10): the agenda's
// top item on the first line, in reserved space, then each enabled
// ratified-record module's summary template in priority order, each within
// its byte budget. Nothing here is cut silently: a module over budget becomes
// one line that says so, and the caller is told.
package block

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"
	"unicode/utf8"

	"github.com/allanschon/brabeus/internal/agenda"
	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/store"
)

const (
	Cap         = module.Cap
	Reservation = module.Reservation
)

// Fault is a share of the block that could not be rendered as asked. A fault
// with Error set is a broken template; one without is over budget, or the
// agenda line was cut.
type Fault struct {
	Module string `json:"module"`
	Bytes  int    `json:"bytes"`
	Budget int    `json:"budget"`
	Error  string `json:"error,omitempty"`
}

// Rec is a record as a template sees it: the stored record plus the
// unconfirmed marker, filled by the block rather than left to each template,
// because a draft must never read as the person's word (§10) and a template
// that forgets is a template that lies.
type Rec struct {
	store.Stored
	Mark string
}

// Unconfirmed is what Mark carries for a record with no Reviewed stamp yet.
const Unconfirmed = " (unconfirmed)"

// mark returns Unconfirmed for a record that has never been reviewed, and ""
// once it has (spec §10: a ratified record renders as the person's word only
// after review).
func mark(rec store.Stored) string {
	if rec.Reviewed.IsZero() {
		return Unconfirmed
	}
	return ""
}

// Data is what one module's template sees: its own records, already
// scope-filtered and without retired ones, plus for the governing module of a
// crossing kind the ratified crossing records (spec §7).
type Data struct {
	Module  module.Manifest
	Records []Rec
	Now     time.Time
}

// Kind returns the records of one kind, confirmed ones first (most recently
// reviewed first), then unconfirmed by most recently updated. Templates
// range over this and cap it with first, so a template never has to decide
// what "confirmed" means.
func (d Data) Kind(name string) []Rec {
	var out []Rec
	for _, r := range d.Records {
		if r.Kind == name {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return store.LessByReview(out[i].Stored, out[j].Stored) })
	return out
}

func first(n int, recs []Rec) []Rec {
	if n < len(recs) {
		return recs[:n]
	}
	return recs
}

var funcs = template.FuncMap{
	"first": first,
	"date":  func(t time.Time) string { return t.UTC().Format("2006-01-02") },
	"age":   func(t time.Time, now time.Time) int { return int(now.Sub(t).Hours() / 24) },
}

type Renderer struct {
	set       *module.Set
	templates map[string]*template.Template
}

// New parses every ratified-record module's summary template. A missing or
// malformed template is an error here, at startup, rather than a surprise at
// the first session: this is the existence check the loader defers.
func New(set *module.Set) (*Renderer, error) {
	r := &Renderer{set: set, templates: map[string]*template.Template{}}
	for _, m := range set.Modules {
		if m.Profile != module.RatifiedRecord {
			continue
		}
		path := filepath.Join(m.Dir, m.Summary)
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("module %s: summary template: %w", m.Name, err)
		}
		t, err := template.New(m.Name).Funcs(funcs).Parse(string(b))
		if err != nil {
			return nil, fmt.Errorf("module %s: %s: %w", m.Name, m.Summary, err)
		}
		r.templates[m.Name] = t
	}
	return r, nil
}

// Part is one share of the block: the agenda line (Module empty) or one
// module's summary, each ending in a newline. The view renders them apart so
// it can link each share to its module; joined, they are the block.
type Part struct {
	Module string `json:"module,omitempty"`
	Text   string `json:"text"`
}

// Render composes the block: RenderParts, joined.
func (r *Renderer) Render(items []agenda.Item, recs []store.Stored, now time.Time, v store.Visibility) (string, []Fault) {
	parts, faults := r.RenderParts(items, recs, now, v)
	var out strings.Builder
	for _, p := range parts {
		out.WriteString(p.Text)
	}
	return out.String(), faults
}

// RenderParts composes the block in parts. items is the agenda computed from
// recs; recs are already the caller's view. The result is at most Cap bytes by construction:
// the agenda line is bounded by Reservation and the budgets were checked
// against Cap−Reservation when the set loaded.
func (r *Renderer) RenderParts(items []agenda.Item, recs []store.Stored, now time.Time, v store.Visibility) ([]Part, []Fault) {
	var parts []Part
	var faults []Fault
	line, f := AgendaLine(items)
	parts = append(parts, Part{Text: line + "\n"})
	if f != nil {
		faults = append(faults, *f)
	}
	for _, m := range r.set.Modules {
		t, ok := r.templates[m.Name]
		if !ok || v.Hides(m.Name) {
			// A hidden module contributes nothing, not even its name (§11).
			continue
		}
		data := Data{Module: m, Now: now}
		for _, rec := range recs {
			gov, _, ok := r.set.RuleFor(rec.Module, rec.Kind)
			if !ok || gov.Name != m.Name {
				continue
			}
			// Own records always; a crossing record only once ratified (§7).
			if rec.Module == m.Name || !rec.Reviewed.IsZero() {
				data.Records = append(data.Records, Rec{Stored: rec, Mark: mark(rec)})
			}
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, data); err != nil {
			faults = append(faults, Fault{Module: m.Name, Budget: m.BudgetBytes, Error: err.Error()})
			parts = append(parts, Part{Module: m.Name, Text: fmt.Sprintf("%s: template error (%v)\n", m.Name, err)})
			continue
		}
		if buf.Len() > m.BudgetBytes {
			faults = append(faults, Fault{Module: m.Name, Bytes: buf.Len(), Budget: m.BudgetBytes})
			parts = append(parts, Part{Module: m.Name, Text: fmt.Sprintf("%s: over budget (%d of %d bytes)\n", m.Name, buf.Len(), m.BudgetBytes)})
			continue
		}
		text := buf.String()
		if text == "" {
			continue // a module with nothing to say adds no part, as it added no bytes
		}
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		parts = append(parts, Part{Module: m.Name, Text: text})
	}
	return parts, faults
}

// AgendaLine renders the top item within Reservation bytes. Overflow drops
// the revision, then the snooze suffix, then cuts the question with an
// ellipsis and reports a fault — the one place the block cuts, and it says so.
func AgendaLine(items []agenda.Item) (string, *Fault) {
	top, ok := agenda.Top(items)
	if !ok {
		return "agenda: nothing due", nil
	}
	// An untagged item — a pre-module record, the one case where Module is
	// empty — has no module or kind to name; its path is the only thing
	// that identifies it. A malformed record still carries its module and
	// kind (agenda.fixByHand keeps them) and renders tagged as usual.
	who := top.Path
	switch {
	case top.Module != "" && top.Kind == "":
		// A module-only item, such as instructions over their budget,
		// concerns the module and no record.
		who = top.Module
	case top.Module != "":
		who = top.Module + "/" + top.Kind
		if top.ID != "" {
			who += " " + top.ID
		} else if top.Name != "" {
			who += " " + top.Name
		}
	}
	base := fmt.Sprintf("agenda: [%s] %s", who, top.Question)
	rev, snooze := "", ""
	if top.Revision != "" {
		rev = " — " + top.Revision
	}
	if top.Snoozes > 0 {
		snooze = fmt.Sprintf(" (snoozed %d)", top.Snoozes)
	}
	for _, line := range []string{base + rev + snooze, base + snooze, base} {
		if len(line) <= Reservation {
			return line, nil
		}
	}
	// Cut on a byte budget, then back off to a rune boundary so the
	// ellipsis never follows half a character.
	cut := base[:Reservation-len("…")]
	for len(cut) > 0 && !utf8.RuneStart(base[len(cut)]) {
		cut = cut[:len(cut)-1]
	}
	return cut + "…", &Fault{Module: "agenda", Bytes: len(base), Budget: Reservation}
}
