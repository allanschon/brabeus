package main

import (
	"fmt"
	"strings"
)

// updateIndexContent keeps MEMORY.md pointing at every memory.
//
// The index is maintained by the same call that writes the file, so nobody has
// to remember to add the line. That is the design axiom applied to the step
// most likely to be skipped — and it only holds if this function is right, so
// it is a content transform rather than a file mutation, and tested as one.
func updateIndexContent(existing, rel, name, description string) string {
	line := fmt.Sprintf("- [%s](%s) — %s", oneLine(name), rel, oneLine(description))
	// The link target is matched with its delimiters so that infra/ups.md is
	// not mistaken for infra/ups-detail.md.
	marker := "](" + rel + ")"

	lines := strings.Split(existing, "\n")
	for i, l := range lines {
		if strings.Contains(l, marker) {
			lines[i] = line
			return strings.Join(lines, "\n")
		}
	}

	section := "## global"
	switch {
	case strings.HasPrefix(rel, "projects/"):
		section = "## projects"
	case strings.HasPrefix(rel, "infra/"):
		section = "## infra"
	}
	for i, l := range lines {
		if strings.TrimSpace(l) == section {
			out := make([]string, 0, len(lines)+1)
			out = append(out, lines[:i+1]...)
			out = append(out, line)
			out = append(out, lines[i+1:]...)
			return strings.Join(out, "\n")
		}
	}

	// No such heading. Record it anyway: losing the entry silently is worse
	// than putting it somewhere unexpected.
	if !strings.HasSuffix(existing, "\n") {
		lines = append(lines, "")
	}
	lines = append(lines, line, "")
	return strings.Join(lines, "\n")
}

// removeIndexEntry drops a memory's line from MEMORY.md.
//
// A stale entry pointing at a deleted file is worse than no entry: it reads
// as a live memory and sends the reader after something that is gone. Deleting
// the file and leaving the line is the failure mode this exists to prevent.
func removeIndexEntry(existing, rel string) string {
	// Same delimiters as updateIndexContent, so infra/ups.md does not take
	// infra/ups-detail.md with it.
	marker := "](" + rel + ")"

	lines := strings.Split(existing, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.Contains(l, marker) {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}
