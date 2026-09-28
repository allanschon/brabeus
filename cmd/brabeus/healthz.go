package main

import (
	"fmt"
	"strings"

	"github.com/allanschon/brabeus/internal/module"
)

// healthz is the one-line body of /healthz. A pure function so the format is
// tested; the handler in main.go only writes it. claims is when the claims
// schedule last ran: off, never, or an RFC3339 stamp. It comes last because
// the plugin reads profiles= up to the next space.
func healthz(version, identityMode string, set *module.Set, claims string) string {
	profiles := make([]string, 0, 2)
	for _, p := range set.Profiles() {
		profiles = append(profiles, string(p))
	}
	return fmt.Sprintf("ok %s identity=%s modules=%s profiles=%s claims=%s\n",
		version, identityMode, strings.Join(set.Names(), ","), strings.Join(profiles, ","), claims)
}
