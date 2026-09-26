package main

import (
	"reflect"
	"testing"
)

func TestTokenizeFoldsCaseSuffixesAndPunctuation(t *testing.T) {
	for in, want := range map[string][]string{
		"The Printer prints":  {"printer", "print"},
		"test-file/notes.md":  {"test", "file", "note", "md"},
		"backups are RUNNING": {"backup", "runn"},
		"a of the and is":     nil,
		"UPS":                 {"ups"},
	} {
		if got := tokenize(in); !reflect.DeepEqual(got, want) {
			t.Errorf("tokenize(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestFieldWeightsPutTheNameAheadOfTheBody(t *testing.T) {
	named := indexDoc("infra/ups.md", "---\nname: ups\ndescription: d\nscope: global\n---\n\nnothing relevant here\n")
	mentioned := indexDoc("infra/other.md", "---\nname: other\ndescription: d\nscope: global\n---\n\nups ups ups ups\n")
	ix := newIndex([]indexedDoc{named, mentioned})

	got := ix.rank("ups", nil)
	if len(got) != 2 {
		t.Fatalf("ranked %d documents, want 2", len(got))
	}
	if got[0].doc.path != "infra/ups.md" {
		t.Errorf("ranked %q first; the name and path must outweigh repetition in the body", got[0].doc.path)
	}
}

func TestRankSkipsDocumentsTheFilterRejects(t *testing.T) {
	ix := newIndex([]indexedDoc{
		indexDoc("a.md", "---\nname: a\nscope: machine/delta\n---\n\nups\n"),
		indexDoc("b.md", "---\nname: b\nscope: global\n---\n\nups\n"),
	})
	got := ix.rank("ups", func(d *indexedDoc) bool { return d.scope == "global" })
	if len(got) != 1 || got[0].doc.path != "b.md" {
		t.Errorf("rank returned %d documents, want only b.md", len(got))
	}
}

// Equal scores must not come back in map order, or the same query answers
// differently between runs and no test above this one can be trusted.
func TestRankIsDeterministicOnTies(t *testing.T) {
	var docs []indexedDoc
	for _, p := range []string{"c.md", "a.md", "b.md"} {
		docs = append(docs, indexDoc(p, "---\nname: n\nscope: global\n---\n\nups\n"))
	}
	ix := newIndex(docs)
	for i := 0; i < 20; i++ {
		got := ix.rank("ups", nil)
		if got[0].doc.path != "a.md" {
			t.Fatalf("tie broke to %q, want a.md every time", got[0].doc.path)
		}
	}
}

// snippet must never hand back a frontmatter line: scanning the whole file
// let "name: x" or "description: y" win the earliest-wins tie against a real
// body line, since both are weighted highest and sit on the earliest lines.
// Each case below asserts the exact line number and text, not merely that
// they are non-zero and non-empty — a stub returning (1, "---") for every
// input would pass a looser check but must fail this one.
func TestSnippet(t *testing.T) {
	frontmatter := "---\nname: n\ndescription: the real description\ntype: reference\nscope: global\nupdated: 2024-01-01T00:00:00Z\n---\n\n"

	for _, c := range []struct {
		name           string
		content, query string
		wantLine       int
		wantText       string
	}{
		{
			name:     "body line match wins, and repeats tie-break to the earliest",
			content:  frontmatter + "battery\nbattery\nbattery\n",
			query:    "battery",
			wantLine: 9,
			wantText: "battery",
		},
		{
			name: "a hit on the name alone falls back into the body, not the frontmatter",
			content: "---\nname: widgetxyz\ndescription: d\ntype: reference\nscope: global\n" +
				"updated: 2024-01-01T00:00:00Z\n---\n\nfirst body line\nsecond body line\n",
			query:    "widgetxyz",
			wantLine: 9,
			wantText: "first body line",
		},
		{
			name:     "a body that is entirely key: value lines is still searched, not skipped as frontmatter-shaped",
			content:  frontmatter + "Runtime: 48 minutes.\nCost: $5.\n",
			query:    "nomatch",
			wantLine: 9,
			wantText: "Runtime: 48 minutes.",
		},
		{
			name:     "an empty body floors on the description's real line, not line 1 with no text",
			content:  frontmatter,
			query:    "nomatch",
			wantLine: 3,
			wantText: "description: the real description",
		},
		{
			// "---" inside a BODY is a markdown thematic break, not a
			// frontmatter marker — an unrelated reason from the ":" check
			// above, so it keeps its own skip even though that one was
			// deleted. Without this, a memory whose body opens with a rule
			// reports the bare rule as its matched line, which is exactly
			// the uninformative-snippet problem this function exists to fix.
			name:     "a body-only thematic break is skipped in favour of the prose that follows",
			content:  frontmatter + "---\n\nThe actual prose starts here.\n",
			query:    "nomatch",
			wantLine: 11,
			wantText: "The actual prose starts here.",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			line, text := snippet(c.content, c.query)
			if line != c.wantLine || text != c.wantText {
				t.Errorf("snippet(...) = (%d, %q), want (%d, %q)", line, text, c.wantLine, c.wantText)
			}
		})
	}
}

func TestAQuotedPhraseMustAppearLiterally(t *testing.T) {
	ix := newIndex([]indexedDoc{
		indexDoc("a.md", "---\nname: a\nscope: global\n---\n\nthe file is grub.cfg here\n"),
		indexDoc("b.md", "---\nname: b\nscope: global\n---\n\ngrub and cfg appear apart\n"),
	})
	got := ix.rank(`"grub.cfg"`, nil)
	if len(got) != 1 || got[0].doc.path != "a.md" {
		t.Errorf(`rank("grub.cfg" quoted) returned %d hits, want only a.md`, len(got))
	}
	// Case-insensitive, like the scan it replaces.
	if len(ix.rank(`"GRUB.CFG"`, nil)) != 1 {
		t.Error("a quoted phrase must match case-insensitively")
	}
	// Unquoted, both are candidates.
	if len(ix.rank("grub cfg", nil)) != 2 {
		t.Error("an unquoted query must still rank both documents")
	}
}

// A short or all-stop-word phrase tokenizes to nothing and scores zero, but
// it satisfied the literal check, so it must not be dropped by `s > 0`. The
// line scan this ranker replaces could find "C++"; this is the regression
// Task 7 exists to prevent.
func TestAQuotedPhraseThatTokenizesToNothingStillScores(t *testing.T) {
	ix := newIndex([]indexedDoc{
		indexDoc("a.md", "---\nname: a\nscope: global\n---\n\nbuilt with C++ and clang\n"),
		indexDoc("b.md", "---\nname: b\nscope: global\n---\n\nbuilt with clang only\n"),
	})
	got := ix.rank(`"C++"`, nil)
	if len(got) != 1 || got[0].doc.path != "a.md" {
		t.Errorf(`rank("C++" quoted) returned %d hits, want only a.md`, len(got))
	}
}

// The phrase filter must also see the path: path is weighted highest at index
// time, so a phrase that appears only in the filename (no name convention to
// lean on, as with repo=projects) must still be findable, not silently
// narrowed away by a filter reading a different view of the corpus.
func TestAQuotedPhraseMatchesThePathToo(t *testing.T) {
	ix := newIndex([]indexedDoc{
		indexDoc("infra/grub.cfg-notes.md", "---\nname: notes\nscope: global\n---\n\nsome unrelated prose\n"),
		indexDoc("infra/other.md", "---\nname: other\nscope: global\n---\n\nsome unrelated prose\n"),
	})
	got := ix.rank(`"grub.cfg"`, nil)
	if len(got) != 1 || got[0].doc.path != "infra/grub.cfg-notes.md" {
		t.Errorf(`rank("grub.cfg" quoted) returned %d hits, want only infra/grub.cfg-notes.md`, len(got))
	}
}
