package store

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/retrieval"
)

// now is compose's clock. A variable so a test can pin it: the stamp's
// whole contract is about which instant it records, and proving that with
// real time means sleeping through a second boundary.
var now = func() time.Time { return time.Now().UTC() }

// Record is what a caller may say about a file. The kernel composes the
// frontmatter from it, so malformed frontmatter is impossible rather than
// discouraged, and nothing here lets a caller touch a key the kernel owns.
type Record struct {
	Name        string
	Description string
	Module      string
	Kind        string
	ID          string
	Scope       string
	Fields      map[string]string
	Body        string
	// Type is the pre-module alias, accepted until M2 so that clients and queued
	// outbox files written before modules still land. Write maps it through the
	// module set when Module is empty.
	Type string
}

// Meta is what the kernel owns about a file. A caller never sets any of it:
// updated moves on every write, reviewed and snoozes only on review (§9),
// retired only on a review whose answer is retired.
type Meta struct {
	Updated    time.Time
	Reviewed   time.Time
	Retired    time.Time
	Snoozes    int
	LegacyType string
	// Malformed lists kernel keys (reviewed, retired, snoozes) whose value was
	// present in the file but did not parse. Write refuses rather than
	// composing anyway: silently dropping one of these would erase it, and
	// only a review may move them (§9). updated is deliberately not tracked
	// here — a value that fails to parse there means "restamp", which
	// staleStamp already implements.
	Malformed []string
}

// kernelKeys are the frontmatter keys compose writes itself: module.Reserved
// plus "type", which predates modules and so is not in that
// list. Built from the shared list at init so the two cannot drift apart.
var kernelKeys = func() map[string]bool {
	m := map[string]bool{"type": true}
	for _, k := range module.Reserved {
		m[k] = true
	}
	return m
}()

// oneLine collapses a value onto a single line. The frontmatter and the index
// are both line-oriented: a newline in either ends the entry early and leaves
// the remainder dangling where nothing can match or remove it.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// yamlValue quotes a scalar when it would otherwise be ambiguous. Descriptions
// are prose written by a model and prose contains colons.
func yamlValue(s string) string {
	s = oneLine(s)
	if strings.ContainsAny(s, ":#\"'") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

// writeField writes one field. The claims field is a block (spec §8.1): the
// key alone, then each line indented two spaces, which is exactly what
// ParseFrontmatter strips back off. Every other field stays one line, as it
// always was: the summary templates, the agenda's prompts and the revision
// line all put a field's value on a single line, and a newline there would
// read as a second entry.
func writeField(b *strings.Builder, k, v string) {
	if k != ClaimsField || !strings.Contains(v, "\n") {
		fmt.Fprintf(b, "%s: %s\n", k, yamlValue(v))
		return
	}
	fmt.Fprintf(b, "%s:\n", k)
	for _, line := range strings.Split(strings.TrimRight(v, "\n"), "\n") {
		fmt.Fprintf(b, "  %s\n", line)
	}
}

// compose renders the file. fieldOrder is the kind's declared order; fields
// not in it (there are none after validation) are appended sorted so the
// output is still deterministic.
func compose(r Record, meta Meta, fieldOrder []string) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", yamlValue(r.Name))
	fmt.Fprintf(&b, "description: %s\n", yamlValue(r.Description))
	fmt.Fprintf(&b, "module: %s\n", yamlValue(r.Module))
	fmt.Fprintf(&b, "kind: %s\n", yamlValue(r.Kind))
	if r.ID != "" {
		fmt.Fprintf(&b, "id: %s\n", yamlValue(r.ID))
	}
	fmt.Fprintf(&b, "scope: %s\n", yamlValue(r.Scope))
	written := map[string]bool{"id": true}
	for _, k := range fieldOrder {
		if v, ok := r.Fields[k]; ok && !written[k] {
			writeField(&b, k, v)
			written[k] = true
		}
	}
	rest := make([]string, 0, len(r.Fields))
	for k := range r.Fields {
		if !written[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		writeField(&b, k, r.Fields[k])
	}
	// A full RFC3339 stamp in UTC, not a bare date: this field is read by a
	// model, so it has to be unambiguous on its own.
	fmt.Fprintf(&b, "updated: %s\n", meta.Updated.UTC().Format(time.RFC3339))
	if !meta.Reviewed.IsZero() {
		fmt.Fprintf(&b, "reviewed: %s\n", meta.Reviewed.UTC().Format(time.RFC3339))
	}
	if !meta.Retired.IsZero() {
		fmt.Fprintf(&b, "retired: %s\n", meta.Retired.UTC().Format(time.RFC3339))
	}
	if meta.Snoozes > 0 {
		fmt.Fprintf(&b, "snoozes: %d\n", meta.Snoozes)
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimRight(r.Body, "\n"))
	b.WriteString("\n")
	return b.String()
}

// ParseRecord reads a file back into what a caller could have said and what
// the kernel owns. A file from before modules parses with Module empty and
// Meta.LegacyType set; nothing refuses it on read.
func ParseRecord(content string) (Record, Meta) {
	fm := parseFrontmatter(content)
	r := Record{Name: fm["name"], Description: fm["description"], Module: fm["module"], Kind: fm["kind"],
		ID: fm["id"], Scope: fm["scope"], Fields: map[string]string{}}
	meta := Meta{LegacyType: fm["type"]}
	for k, v := range fm {
		if !kernelKeys[k] {
			r.Fields[k] = v
		}
	}
	if r.ID != "" {
		r.Fields["id"] = r.ID
	}
	// updated: a value that fails to parse means "restamp" (staleStamp already
	// treats it that way), so it is never added to Malformed.
	meta.Updated, _ = time.Parse(time.RFC3339, fm["updated"])
	// reviewed and retired: absent is fine (zero value, never reviewed/retired);
	// present but unparseable is not, and must stop the write rather than
	// silently erase the key on rewrite (§9).
	parseKernelTime := func(key string) time.Time {
		s := fm[key]
		if s == "" {
			return time.Time{}
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			meta.Malformed = append(meta.Malformed, key)
			return time.Time{}
		}
		return t
	}
	meta.Reviewed = parseKernelTime("reviewed")
	meta.Retired = parseKernelTime("retired")
	if s := fm["snoozes"]; s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			meta.Snoozes = n
		} else {
			meta.Malformed = append(meta.Malformed, "snoozes")
		}
	}
	r.Body = bodyAfterFrontmatter(content)
	return r, meta
}

// bodyAfterFrontmatter returns everything after the closing divider, with the
// blank line compose puts there removed.
func bodyAfterFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	rest := content[4:]
	i := strings.Index(rest, "\n---\n")
	if i < 0 {
		return ""
	}
	return strings.TrimLeft(rest[i+5:], "\n")
}

// staleStamp reports whether a memory's `updated` field predates the RFC3339
// format. Such a memory must be rewritten even when nothing else changed, or
// the store keeps two stamp formats at once — worse for the model that reads
// them than the single ambiguous format it replaced.
func staleStamp(content string) bool {
	got := parseFrontmatter(content)["updated"]
	if got == "" {
		return true // no stamp at all; give it one
	}
	_, err := time.Parse(time.RFC3339, got)
	return err != nil
}

// sansUpdated returns the memory without its `updated` line. Two memories that
// differ only there have the same content, and the stamp means "when this last
// changed" rather than "when it was last written" — so a rewrite of identical
// content must not move it. Without this the full-resolution stamp defeats the
// no-change path and every rewrite produces an empty commit.
func sansUpdated(content string) string {
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		if i > 0 && retrieval.IsDivider(l) {
			break // past the frontmatter; the body is markdown and may say anything
		}
		if strings.HasPrefix(l, "updated:") {
			out := make([]string, 0, len(lines)-1)
			out = append(out, lines[:i]...)
			out = append(out, lines[i+1:]...)
			return strings.Join(out, "\n")
		}
	}
	return content
}

// parseFrontmatter reads the leading --- block and nothing else. Delegates to
// retrieval.ParseFrontmatter, which owns the one definition — IndexDoc needs
// it too, and retrieval cannot import store, so the definition lives there
// and store calls in rather than keeping a second copy.
func parseFrontmatter(content string) map[string]string {
	return retrieval.ParseFrontmatter(content)
}
