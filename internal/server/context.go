package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/allanschon/brabeus/internal/agenda"
	"github.com/allanschon/brabeus/internal/block"
	"github.com/allanschon/brabeus/internal/identity"
	"github.com/allanschon/brabeus/internal/scope"
	"github.com/allanschon/brabeus/internal/store"
)

// RenderContext is the block for one caller: the records they may see, the
// agenda over exactly those, the render. Shared by the context tool and GET
// /context so the two cannot drift.
func RenderContext(d Deps, caller string, consumer bool) (string, []block.Fault, *agenda.Item, error) {
	all, err := d.Memory.Records()
	if err != nil {
		return "", nil, nil, err
	}
	audience := audienceFor(d.Set, consumer)
	recs := make([]store.Stored, 0, len(all))
	for _, r := range all {
		if !r.Retired.IsZero() || audience.Hides(r.Module) || !scope.Visible(r.Scope, caller, false) {
			continue
		}
		recs = append(recs, r)
	}
	now := d.Now()
	// A consumer never reviews, so it is asked nothing: onboarding gaps come
	// from the set, not the records, and would otherwise name modules the
	// consumer may not see.
	var items []agenda.Item
	if !consumer {
		// A results file that does not parse loses only its own goal's
		// fails: the agenda still has every other reason to ask something.
		results, err := d.Memory.ClaimResults()
		if err != nil {
			log.Printf("claim results: %v", err)
		}
		items = agenda.Compute(d.Set, recs, results, now)
	}
	text, faults := d.Block.Render(items, recs, now, audience)
	if top, ok := agenda.Top(items); ok {
		return text, faults, &top, nil
	}
	return text, faults, nil, nil
}

// ContextHandler serves the same block as text for the plugin's SessionStart
// hook, under the same identity and auth as /mcp. RefuseUnidentified sits in
// front of this handler (main.go), so caller here is always resolved.
func ContextHandler(d Deps, id identity.Identity, consumers map[string]bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
			return
		}
		caller, consumer, _ := Caller(id, consumers, r)
		text, _, _, err := RenderContext(d, caller, consumer)
		if err != nil {
			log.Printf("context for %q: %v", caller, err)
			http.Error(w, "context unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, text)
	})
}

type contextIn struct{}
type contextOut struct {
	Block  string        `json:"block" jsonschema:"the session context block, at most 2048 bytes: the agenda line first, then each enabled ratified-record module's summary in priority order. Scope-filtered to your machine; may be up to one sync interval behind another machine's writes"`
	Faults []block.Fault `json:"faults,omitempty" jsonschema:"modules whose summary exceeded their budget and were replaced by one line saying so, or agenda when its line had to be cut"`
	Agenda *agenda.Item  `json:"agenda,omitempty" jsonschema:"the top agenda item: the record, why it is asked, the question to ask, its revision line and snooze count. Absent when nothing is due"`
}

// registerContextTool registers the context tool: the same block RenderContext
// builds for GET /context, reachable from an MCP session.
func registerContextTool(s *mcp.Server, d Deps, caller string, consumer bool) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "context",
		Description: "The session context block: the one agenda item to raise first, then the person's confirmed " +
			"record by module, within 2 KB. Read it at the start of a session; it is your view, scope-filtered, " +
			"and may lag another machine's writes by up to one sync interval.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in contextIn) (*mcp.CallToolResult, contextOut, error) {
		blockText, faults, top, err := RenderContext(d, caller, consumer)
		if err != nil {
			return nil, contextOut{}, err
		}
		return nil, contextOut{Block: blockText, Faults: faults, Agenda: top}, nil
	})
}
