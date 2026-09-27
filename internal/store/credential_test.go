package store

import (
	"strings"
	"testing"
)

// Spec §11: a write matching a small set of credential shapes is rejected
// before it reaches git. The set is deliberately small and named: prose
// about credentials is a legitimate memory, a credential is not.
//
// The refused samples are BUILT, never written as literals: this repository
// runs gitleaks on every push (CI and GitHub push protection), and a literal
// token shape in a test file trips both. A leak scanner that cannot see the
// sample is the point of assembling it at run time.
func TestCredentialShapesAreRefusedAndProseIsNot(t *testing.T) {
	up := func(n int) string { return strings.Repeat("A", n) }
	refused := []string{
		"-----BEGIN " + "OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\n-----END " + "OPENSSH PRIVATE KEY-----",
		"the key is " + "AKIA" + "IOSFODNN7EXAMPLE" + " and it works",
		"token " + "ghp" + "_" + up(36) + "0123",
		"glpat" + "-" + up(20),
		"xoxb" + "-" + "123456789012-" + up(24),
		"password: " + "Tr0ub4dor&3" + up(9),
		"secret = " + "ZmFrZXNlY3JldGZha2VzZWNyZXQ=",
	}
	for _, s := range refused {
		if shape := credentialShape(s); shape == "" {
			t.Errorf("not refused: %q", s)
		}
	}
	allowed := []string{
		"the token lives in ~/.config/gh/hosts.yml, mode 600",
		"password: rotate it quarterly; the value is in the secrets file",
		"secret: /etc/example/env holds it, root:root 600",
		"AKIA is the prefix AWS access key ids start with",
		"tokens expire after 90 days",
	}
	for _, s := range allowed {
		if shape := credentialShape(s); shape != "" {
			t.Errorf("wrongly refused as %q: %q", shape, s)
		}
	}
}
