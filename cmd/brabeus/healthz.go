package main

import (
	"fmt"
	"strings"

	"github.com/allanschon/brabeus/internal/module"
)

// healthz is the one-line body of /healthz. A pure function so the format is
// tested; the handler in main.go only writes it. claims is when the claims
// schedule last ran: off, never, or an RFC3339 stamp. It comes after
// profiles because the plugin reads profiles= up to the next space.
// claimErrs is the last run's count of goals whose store write failed; it is
// appended as errors=<n> only when non-zero, so a healthy line is unchanged
// and the field a monitor has never seen does not need to mean anything.
func healthz(version, identityMode string, set *module.Set, claims string, claimErrs int) string {
	profiles := make([]string, 0, 2)
	for _, p := range set.Profiles() {
		profiles = append(profiles, string(p))
	}
	line := fmt.Sprintf("ok %s identity=%s modules=%s profiles=%s claims=%s",
		version, identityMode, strings.Join(set.Names(), ","), strings.Join(profiles, ","), claims)
	if claimErrs > 0 {
		line += fmt.Sprintf(" errors=%d", claimErrs)
	}
	return line + "\n"
}
