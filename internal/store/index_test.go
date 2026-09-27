package store

import (
	"strings"
	"testing"
)

const baseIndex = `---
type: index
---

# Memory index

## global

## infra

## projects
`

func TestIndexAddsALineUnderTheMatchingSection(t *testing.T) {
	got := updateIndexContent(baseIndex, "infra/delta-ups.md", "delta-ups", "the UPS feeds delta and epsilon")

	line := "- [delta-ups](infra/delta-ups.md) — the UPS feeds delta and epsilon"
	if !strings.Contains(got, line) {
		t.Fatalf("index line missing.\n%s", got)
	}
	infra := strings.Index(got, "## infra")
	projects := strings.Index(got, "## projects")
	at := strings.Index(got, line)
	if at < infra || at > projects {
		t.Errorf("line landed outside the infra section (infra=%d line=%d projects=%d)", infra, at, projects)
	}
}

func TestIndexRoutesByPathPrefix(t *testing.T) {
	for path, section := range map[string]string{
		"personal/style.md":             "## global",
		"infra/delta-ups.md":            "## infra",
		"projects/example--repo/x.md":   "## projects",
		"MEMORY-adjacent-but-nested.md": "## global",
	} {
		got := updateIndexContent(baseIndex, path, "n", "d")
		at := strings.Index(got, "]("+path+")")
		sec := strings.Index(got, section)
		if at < sec {
			t.Errorf("%s was not placed under %s", path, section)
		}
	}
}

// Re-saving a memory must refresh its line, not add a second one. A duplicated
// index entry is the kind of thing nobody reads closely enough to notice.
func TestIndexReplacesAnExistingEntryInPlace(t *testing.T) {
	once := updateIndexContent(baseIndex, "infra/delta-ups.md", "delta-ups", "first description")
	twice := updateIndexContent(once, "infra/delta-ups.md", "delta-ups", "second description")

	if n := strings.Count(twice, "](infra/delta-ups.md)"); n != 1 {
		t.Errorf("entry appears %d times, want 1:\n%s", n, twice)
	}
	if strings.Contains(twice, "first description") {
		t.Error("the stale description survived")
	}
	if !strings.Contains(twice, "second description") {
		t.Error("the new description is missing")
	}
}

// A path that is a prefix of another must not be mistaken for it.
func TestIndexDoesNotConfuseAPathWithALongerOne(t *testing.T) {
	idx := updateIndexContent(baseIndex, "infra/ups.md", "ups", "short")
	idx = updateIndexContent(idx, "infra/ups-detail.md", "ups-detail", "long")

	if n := strings.Count(idx, "](infra/ups.md)"); n != 1 {
		t.Errorf("infra/ups.md appears %d times, want 1", n)
	}
	if n := strings.Count(idx, "](infra/ups-detail.md)"); n != 1 {
		t.Errorf("infra/ups-detail.md appears %d times, want 1", n)
	}
}

func TestIndexKeepsExistingContent(t *testing.T) {
	got := updateIndexContent(baseIndex, "personal/style.md", "style", "d")
	for _, want := range []string{"type: index", "# Memory index", "## global", "## infra", "## projects"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q was lost from the index", want)
		}
	}
}

// An index missing the expected heading must still record the memory. Losing
// the entry silently is worse than putting it in an odd place.
func TestIndexAppendsWhenTheSectionIsAbsent(t *testing.T) {
	got := updateIndexContent("# Memory index\n", "infra/x.md", "x", "d")
	if !strings.Contains(got, "](infra/x.md)") {
		t.Errorf("entry was dropped when the section was missing:\n%s", got)
	}
}

// Removing a memory must remove its index line too. A stale entry pointing at a
// deleted file is the failure the automatic index maintenance exists to prevent,
// and it is worse than no index because it reads as a live memory.
func TestIndexRemovesTheEntryForADeletedMemory(t *testing.T) {
	idx := updateIndexContent(baseIndex, "infra/ups.md", "ups", "the UPS")
	idx = updateIndexContent(idx, "infra/delta.md", "delta", "the NAS box")

	got := removeIndexEntry(idx, "infra/ups.md")

	if strings.Contains(got, "](infra/ups.md)") {
		t.Errorf("the deleted memory is still indexed:\n%s", got)
	}
	if !strings.Contains(got, "](infra/delta.md)") {
		t.Error("an unrelated entry was removed")
	}
	for _, want := range []string{"## global", "## infra", "## projects", "# Memory index"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q was lost from the index", want)
		}
	}
}

// The same prefix trap as adding: infra/ups.md must not take infra/ups-detail.md
// with it.
func TestIndexRemovalDoesNotTakeALongerPathWithIt(t *testing.T) {
	idx := updateIndexContent(baseIndex, "infra/ups.md", "ups", "short")
	idx = updateIndexContent(idx, "infra/ups-detail.md", "ups-detail", "long")

	got := removeIndexEntry(idx, "infra/ups.md")

	if strings.Contains(got, "](infra/ups.md)") {
		t.Error("the target entry survived")
	}
	if !strings.Contains(got, "](infra/ups-detail.md)") {
		t.Error("a path sharing the prefix was removed too")
	}
}

func TestIndexRemovalOfSomethingUnindexedChangesNothing(t *testing.T) {
	idx := updateIndexContent(baseIndex, "infra/ups.md", "ups", "d")
	if got := removeIndexEntry(idx, "infra/never-indexed.md"); got != idx {
		t.Error("removing an absent entry altered the index")
	}
}

// The frontmatter flattens a description's newlines; the index line has to as
// well, or the second line dangles below the marker where nothing can ever
// match or remove it.
func TestUpdateIndexContentKeepsTheEntryOnOneLine(t *testing.T) {
	got := updateIndexContent(baseIndex, "personal/nl.md", "nl", "line one\nline two")
	for _, l := range strings.Split(got, "\n") {
		if strings.Contains(l, "](personal/nl.md)") {
			if !strings.Contains(l, "line one line two") {
				t.Errorf("entry does not carry the whole description: %q", l)
			}
			return
		}
	}
	t.Fatalf("no entry for personal/nl.md:\n%s", got)
}
