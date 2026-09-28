package store

import (
	"strings"
	"testing"
	"time"
)

// Scope drives who can read a memory, so a file without frontmatter must
// report nothing rather than something.
func TestParseFrontmatterOnAFileWithoutAnyReturnsNothing(t *testing.T) {
	for _, content := range []string{
		"",
		"# Just a heading\n\nsome text\n",
		"not a divider\n---\nname: sneaky\n---\n",
	} {
		if fm := parseFrontmatter(content); len(fm) != 0 {
			t.Errorf("parseFrontmatter(%q) = %v, want no fields", content, fm)
		}
	}
}

func TestComposeWritesTheKeysInTheFixedOrder(t *testing.T) {
	restore := fixClock(t, "2026-09-27T10:00:00Z")
	defer restore()
	r := Record{Name: "g1", Description: "ship the guide", Module: "telos", Kind: "goal", ID: "G1",
		Scope: "global", Fields: map[string]string{"title": "Ship the guide", "ideal": "published", "by": "2026-12-01", "id": "G1"},
		Body: "Why this matters.\n"}
	got := compose(r, Meta{Updated: now()}, []string{"id", "title", "ideal", "by"})
	want := "---\nname: g1\ndescription: ship the guide\nmodule: telos\nkind: goal\nid: G1\nscope: global\ntitle: Ship the guide\nideal: published\nby: 2026-12-01\nupdated: 2026-09-27T10:00:00Z\n---\n\nWhy this matters.\n"
	if got != want {
		t.Errorf("compose =\n%s\nwant\n%s", got, want)
	}
}

func TestComposeCarriesTheKernelOwnedKeysThrough(t *testing.T) {
	restore := fixClock(t, "2026-09-27T10:00:00Z")
	defer restore()
	reviewed, _ := time.Parse(time.RFC3339, "2026-09-01T00:00:00Z")
	got := compose(Record{Name: "n", Description: "d", Module: "identity", Kind: "value", Scope: "global",
		Fields: map[string]string{"statement": "family time"}, Body: "b"},
		Meta{Updated: now(), Reviewed: reviewed, Snoozes: 2}, []string{"statement"})
	for _, line := range []string{"reviewed: 2026-09-01T00:00:00Z\n", "snoozes: 2\n"} {
		if !strings.Contains(got, line) {
			t.Errorf("missing %q in\n%s", line, got)
		}
	}
	if strings.Contains(got, "retired:") {
		t.Errorf("a zero retired must not be written:\n%s", got)
	}
}

func TestParseRecordSplitsKernelKeysFromFields(t *testing.T) {
	content := "---\nname: g1\ndescription: d\nmodule: telos\nkind: goal\nid: G1\nscope: global\ntitle: T\nby: 2026-12-01\nupdated: 2026-09-27T10:00:00Z\nreviewed: 2026-09-01T00:00:00Z\nsnoozes: 1\n---\n\nbody\n"
	r, meta := ParseRecord(content)
	if r.Module != "telos" || r.Kind != "goal" || r.ID != "G1" || r.Scope != "global" || r.Body != "body\n" {
		t.Errorf("record = %+v", r)
	}
	if r.Fields["title"] != "T" || r.Fields["by"] != "2026-12-01" || r.Fields["id"] != "G1" {
		t.Errorf("fields = %v", r.Fields)
	}
	for _, k := range []string{"name", "module", "updated", "reviewed", "snoozes"} {
		if _, ok := r.Fields[k]; ok {
			t.Errorf("%s leaked into Fields", k)
		}
	}
	if meta.Reviewed.IsZero() || meta.Snoozes != 1 || meta.Updated.IsZero() {
		t.Errorf("meta = %+v", meta)
	}
}

func TestParseRecordKeepsAPreModuleFileReadable(t *testing.T) {
	r, meta := ParseRecord("---\nname: n\ndescription: d\ntype: feedback\nscope: global\nupdated: 2026-09-27T10:00:00Z\n---\n\nb\n")
	if r.Module != "" || r.Kind != "" || meta.LegacyType != "feedback" {
		t.Errorf("legacy file: record=%+v meta=%+v", r, meta)
	}
}

func TestComposeThenParseRoundTrips(t *testing.T) {
	restore := fixClock(t, "2026-09-27T10:00:00Z")
	defer restore()
	r := Record{Name: "n", Description: "a line with: a colon", Module: "memory", Kind: "note", Scope: "machine/desk",
		Fields: map[string]string{}, Body: "The body.\n"}
	back, meta := ParseRecord(compose(r, Meta{Updated: now()}, nil))
	if back.Description != r.Description || back.Module != "memory" || back.Kind != "note" || back.Scope != r.Scope || back.Body != r.Body {
		t.Errorf("round trip lost something: %+v", back)
	}
	if meta.Updated.Format(time.RFC3339) != "2026-09-27T10:00:00Z" {
		t.Errorf("updated = %v", meta.Updated)
	}
}

func TestAMultiLineFieldRoundTripsThroughComposeAndParse(t *testing.T) {
	block := "- text: \"three articles\"\n  check: {adapter: tracker, min: 3}"
	r := Record{Name: "g3", Description: "d", Module: "telos", Kind: "goal", Scope: "global",
		Fields: map[string]string{"title": "t", "claims": block}}
	text := compose(r, Meta{Updated: at("2026-09-01T00:00:00Z")}, []string{"title", "claims"})
	if !strings.Contains(text, "claims:\n  - text: \"three articles\"\n    check: {adapter: tracker, min: 3}\nupdated:") {
		t.Errorf("block not written as a block:\n%s", text)
	}
	back, _ := ParseRecord(text)
	if back.Fields["claims"] != block {
		t.Errorf("round trip changed the block:\n%q\nwant\n%q", back.Fields["claims"], block)
	}
	// The no-change path compares files without their updated line; a block
	// line that reads "---" must not end that comparison early.
	notes := Record{Name: "g3", Description: "d", Module: "telos", Kind: "goal", Scope: "global",
		Fields: map[string]string{"notes": "above\n---\nbelow"}}
	a := compose(notes, Meta{Updated: at("2026-09-01T00:00:00Z")}, []string{"notes"})
	b := compose(notes, Meta{Updated: at("2026-09-02T00:00:00Z")}, []string{"notes"})
	if sansUpdated(a) != sansUpdated(b) {
		t.Errorf("a block containing a divider defeats the no-change path:\n%s", a)
	}
}
