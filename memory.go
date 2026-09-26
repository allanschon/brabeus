package main

import (
	"bufio"
	"fmt"
	"strings"
	"time"
)

// now is composeMemory's clock. A variable so a test can pin it: the stamp's
// whole contract is about which instant it records, and proving that with
// real time means sleeping through a second boundary.
var now = func() time.Time { return time.Now().UTC() }

// Memory is one fact in the store. The server composes the file from these
// fields rather than accepting raw content, so malformed frontmatter is
// impossible instead of merely discouraged.
type Memory struct {
	Name        string
	Description string
	Type        string
	Scope       string
	Body        string
}

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

func unquote(s string) string {
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		return strings.ReplaceAll(s[1:len(s)-1], `\"`, `"`)
	}
	return s
}

func composeMemory(m Memory) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", yamlValue(m.Name))
	fmt.Fprintf(&b, "description: %s\n", yamlValue(m.Description))
	fmt.Fprintf(&b, "type: %s\n", yamlValue(m.Type))
	fmt.Fprintf(&b, "scope: %s\n", yamlValue(m.Scope))
	// A full RFC3339 stamp in UTC, not a bare date. This field is read by a
	// model rather than by a person, so it has to be unambiguous on its own:
	// the time is part of the record, and the trailing Z says which zone it is
	// in. A bare date silently meant "whatever zone the host runs", which on
	// these hosts is UTC and is a day ahead of Eastern all evening.
	fmt.Fprintf(&b, "updated: %s\n", now().Format(time.RFC3339))
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimRight(m.Body, "\n"))
	b.WriteString("\n")
	return b.String()
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
		if i > 0 && strings.TrimSpace(l) == "---" {
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

// parseFrontmatter reads the leading --- block and nothing else. A file whose
// first line is not a divider has no frontmatter, however many dividers appear
// later: the body is markdown and markdown uses them as rules.
func parseFrontmatter(content string) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(content))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return out
	}
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || strings.HasPrefix(key, "-") || strings.HasPrefix(key, "#") {
			continue
		}
		out[key] = unquote(strings.TrimSpace(value))
	}
	return out
}
