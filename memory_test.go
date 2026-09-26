package main

import (
	"strings"
	"testing"
	"time"
)

func TestComposeAndParseRoundTrip(t *testing.T) {
	m := Memory{
		Name:        "design-must-not-rely-on-memory",
		Description: "if a design needs the person to remember a step, redesign it",
		Type:        "feedback",
		Scope:       "global",
		Body:        "The axiom, stated 2026-09-05.",
	}
	fm := parseFrontmatter(composeMemory(m))

	for field, want := range map[string]string{
		"name":        m.Name,
		"description": m.Description,
		"type":        m.Type,
		"scope":       m.Scope,
	} {
		if got := fm[field]; got != want {
			t.Errorf("%s round-tripped as %q, want %q", field, got, want)
		}
	}
}

// A description is prose written by a model. Colons appear in prose, and an
// unquoted colon is where a naive frontmatter writer silently truncates.
func TestDescriptionContainingAColonSurvives(t *testing.T) {
	want := "the rule: read the log before theorising"
	fm := parseFrontmatter(composeMemory(Memory{
		Name: "n", Description: want, Type: "feedback", Scope: "global", Body: "b",
	}))
	if got := fm["description"]; got != want {
		t.Errorf("description = %q, want %q", got, want)
	}
}

// The body is markdown and markdown uses --- for rules. A body containing one
// must not be mistaken for the end of the frontmatter, or for a second block.
func TestBodyContainingADividerDoesNotCorruptTheFrontmatter(t *testing.T) {
	body := "first paragraph\n\n---\n\nsecond paragraph"
	out := composeMemory(Memory{
		Name: "n", Description: "d", Type: "feedback", Scope: "machine/beta", Body: body,
	})
	fm := parseFrontmatter(out)
	if got := fm["scope"]; got != "machine/beta" {
		t.Errorf("scope = %q, want machine/beta", got)
	}
	if !strings.Contains(out, "second paragraph") {
		t.Error("the body was truncated at its divider")
	}
}

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

func TestComposeAlwaysEndsWithExactlyOneNewline(t *testing.T) {
	out := composeMemory(Memory{Name: "n", Description: "d", Type: "user", Scope: "global", Body: "body\n\n\n"})
	if !strings.HasSuffix(out, "body\n") || strings.HasSuffix(out, "body\n\n") {
		t.Errorf("trailing whitespace not normalised: %q", out[len(out)-12:])
	}
}

// The `updated` stamp is read by a model, not by the person, so it must say exactly
// when a memory was written and in which zone. A bare date is ambiguous twice
// over: it carries no time, and it silently means UTC on hosts that run UTC
// while the reader may be somewhere else. RFC3339 with the Z suffix says both.
func TestUpdatedIsAFullUTCTimestamp(t *testing.T) {
	before := time.Now().UTC().Truncate(time.Second)
	fm := parseFrontmatter(composeMemory(Memory{
		Name: "n", Description: "d", Type: "feedback", Scope: "global", Body: "b",
	}))
	got := fm["updated"]
	if !strings.HasSuffix(got, "Z") {
		t.Errorf("updated = %q, which does not declare UTC", got)
	}
	ts, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("updated = %q, which is not RFC3339: %v", got, err)
	}
	if ts.Before(before) || ts.After(time.Now().UTC().Add(time.Second)) {
		t.Errorf("updated = %q, which is not the time of writing", got)
	}
	if _, off := ts.Zone(); off != 0 {
		t.Errorf("updated = %q, which is not at zero offset", got)
	}
}
