package store

import "regexp"

// credentialShapes are the forms refused before git (spec §11). Each is
// named so the refusal can say what it saw without echoing the match.
// Assignment shapes require a value with no path characters: a path to
// where a secret lives is a legitimate note, the secret itself is not.
var credentialShapes = []struct {
	name string
	re   *regexp.Regexp
}{
	{"a private key block", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE ` + `KEY-----`)},
	{"an AWS access key id", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{"a GitHub token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`)},
	{"a GitLab token", regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}\b`)},
	{"a Slack token", regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`)},
	{"a password or secret assignment", regexp.MustCompile(`(?i)\b(password|passwd|secret|api[_-]?key|token)\s*[:=]\s*[A-Za-z0-9+=_&!@#$%^*-]{16,}(\s|$)`)},
}

// credentialShape returns the name of the first shape found in s, or "".
func credentialShape(s string) string {
	for _, c := range credentialShapes {
		if c.re.MatchString(s) {
			return c.name
		}
	}
	return ""
}
