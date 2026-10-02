// Package instructions collects the person's instructions — the confirmed
// records of kinds a module marks instructions — and measures them against
// their budget (spec §10).
package instructions

import (
	"sort"
	"strings"

	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/store"
)

// Opening is the line every delivery of instructions begins with.
const Opening = "These are the person's standing instructions for how to work with them. Where the task you were given conflicts with one, the instruction wins on anything that cannot be undone or reaches beyond the working copy, such as pushing, deleting, sending or publishing; the task wins on anything else. Either way, name the conflict in your report."

// Record is one instruction as a session receives it.
type Record struct {
	Module string `json:"module"`
	Path   string `json:"path"`
	Text   string `json:"text"`
}

// Size is the measured length of one module's instructions against its budget.
type Size struct {
	Module string `json:"module"`
	Bytes  int    `json:"bytes"`
	Budget int    `json:"budget"`
}

// Collect returns the confirmed, unretired instruction records that have text,
// ordered by the governing module's position in the set, then by path. Module
// is the governing module, so a working-memory preference that crosses into
// identity reports identity.
func Collect(set *module.Set, recs []store.Stored) []Record {
	if set == nil {
		return nil
	}
	pos := map[string]int{}
	for i, m := range set.Modules {
		pos[m.Name] = i
	}
	var out []Record
	for _, s := range recs {
		if s.Reviewed.IsZero() || !s.Retired.IsZero() {
			continue
		}
		k, ok := set.InstructionKind(s.Module, s.Kind)
		if !ok {
			continue
		}
		text := store.Delivered(s.Record, k)
		if text == "" {
			continue
		}
		gov, _, _ := set.RuleFor(s.Module, s.Kind)
		out = append(out, Record{Module: gov.Name, Path: s.Path, Text: text})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if pi, pj := pos[out[i].Module], pos[out[j].Module]; pi != pj {
			return pi < pj
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// Sizes measures each enabled module that has an instructions budget: the
// length of its records' texts joined as Text joins them.
func Sizes(set *module.Set, recs []Record) []Size {
	if set == nil {
		return nil
	}
	var out []Size
	for _, m := range set.Modules {
		if m.InstructionsBudgetBytes <= 0 {
			continue
		}
		var texts []string
		for _, r := range recs {
			if r.Module == m.Name {
				texts = append(texts, r.Text)
			}
		}
		out = append(out, Size{Module: m.Name, Bytes: len(strings.Join(texts, "\n\n")), Budget: m.InstructionsBudgetBytes})
	}
	return out
}

// Text is what a session receives: the opening line, then each record's text,
// separated by blank lines. It is empty when there are no records.
func Text(recs []Record) string {
	if len(recs) == 0 {
		return ""
	}
	texts := make([]string, len(recs))
	for i, r := range recs {
		texts[i] = r.Text
	}
	return Opening + "\n\n" + strings.Join(texts, "\n\n")
}
