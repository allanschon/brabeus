package main

import (
	"fmt"
	"strings"

	"github.com/allanschon/brabeus/internal/module"
)

// healthz is the one-line body of /healthz. A pure function so the format is
// tested; the handler in main.go only writes it.
func healthz(version, identityMode string, set *module.Set) string {
	profiles := make([]string, 0, 2)
	for _, p := range set.Profiles() {
		profiles = append(profiles, string(p))
	}
	return fmt.Sprintf("ok %s identity=%s modules=%s profiles=%s\n",
		version, identityMode, strings.Join(set.Names(), ","), strings.Join(profiles, ","))
}
