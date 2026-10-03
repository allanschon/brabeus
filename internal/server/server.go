package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/allanschon/brabeus/internal/block"
	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/store"
)

const Version = "0.7.0"

// pickRepo resolves the repo argument. notebook is the current name for the
// person's own notes repository; projects is the same store under its older
// name, kept working for this milestone (spec §5). An absent notebook is an
// error rather than a fallback: silently reading the wrong repository is
// worse than failing the call. A consumer is refused it outright (spec §11):
// it is a second corpus with no modules of its own, so no per-module
// audience check could stand in for refusing it entirely.
func pickRepo(name string, memory, projects *store.Store, consumer bool) (*store.Store, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "memory":
		return memory, nil
	case "projects", "notebook":
		if consumer {
			return nil, fmt.Errorf("the notebook is not readable by this caller")
		}
		if projects == nil {
			return nil, fmt.Errorf("the notebook is not configured")
		}
		return projects, nil
	}
	return nil, fmt.Errorf("unknown repo %q: use memory or notebook (also accepted as projects)", name)
}

// Deps is what the server needs to build tools and the context block, gathered
// so New and RenderContext take one thing rather than drifting parameter lists.
type Deps struct {
	Memory, Projects *store.Store
	Set              *module.Set
	Block            *block.Renderer
	Now              func() time.Time // time.Now in production; tests pin it
	// LastRun reports when the scheduled claims last ran, false for never
	// (spec §8.1). Nil means no runner is wired, so nothing vouches for an
	// adapter's pass.
	LastRun func() (time.Time, bool)
	// ClaimInterval is the schedule's interval; 0 means off. A pass is
	// stale once the last run is two intervals old.
	ClaimInterval time.Duration
}

// New builds a server bound to one caller. Identity is fixed when the session
// is created, so the tools close over it rather than re-deriving it. consumer
// marks a caller that is not the person's own session (spec §11): it sees
// only modules with audience any, and never writes, deletes or reviews.
func New(d Deps, caller string, consumer bool) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:        "brabeus",
		Title:       "Brabeus",
		Description: "A private, git-backed personal record. Durable facts about the person and their projects.",
		Version:     Version,
	}, nil)

	audience := audienceFor(d.Set, consumer)

	registerReadTools(s, d, caller, consumer, audience)
	registerWriteTools(s, d, caller, consumer, audience)
	registerContextTool(s, d, caller, consumer)
	registerClaimTools(s, d, caller, consumer, audience)

	return s
}
