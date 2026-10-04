package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/allanschon/brabeus/internal/agenda"
	"github.com/allanschon/brabeus/internal/block"
	"github.com/allanschon/brabeus/internal/identity"
	"github.com/allanschon/brabeus/internal/instructions"
	"github.com/allanschon/brabeus/internal/scope"
	"github.com/allanschon/brabeus/internal/store"
)

// RenderContext is the block for one caller: the records they may see, the
// agenda over exactly those, the render. Shared by the context tool and GET
// /context so the two cannot drift.
//
// project is the caller's own project scope key, already validated by
// scope.CheckProjectKey at whichever boundary received it (the tool argument
// or the query parameter) — empty when the caller is not inside a git
// repository with an origin. It filters only this render: claims and reflect
// keep scope.Visible, because they answer about the whole record, not one
// project's view (spec §10).
func RenderContext(d Deps, caller, project string, consumer bool) (string, []block.Fault, *agenda.Item, error) {
	parts, faults, items, err := RenderContextParts(d, caller, project, consumer)
	if err != nil {
		return "", nil, nil, err
	}
	var text strings.Builder
	for _, p := range parts {
		text.WriteString(p.Text)
	}
	if top, ok := agenda.Top(items); ok {
		return text.String(), faults, &top, nil
	}
	return text.String(), faults, nil, nil
}

// RenderContextParts is RenderContext before the join: the block in parts,
// and the full agenda the top item was taken from, so the view can show what
// is due as well as the block.
func RenderContextParts(d Deps, caller, project string, consumer bool) ([]block.Part, []block.Fault, []agenda.Item, error) {
	recs, audience, err := visibleRecords(d, caller, project, consumer)
	if err != nil {
		return nil, nil, nil, err
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
	parts, faults := d.Block.RenderParts(items, recs, now, audience)
	return parts, faults, items, nil
}

// visibleRecords is the caller's view of the record: the unretired records
// their audience allows and their scope reaches, and the audience computed to
// get there. The block and the instructions both start from it.
func visibleRecords(d Deps, caller, project string, consumer bool) ([]store.Stored, store.Visibility, error) {
	audience := audienceFor(d.Set, consumer)
	all, err := d.Memory.Records()
	if err != nil {
		return nil, audience, err
	}
	recs := make([]store.Stored, 0, len(all))
	for _, r := range all {
		if !r.Retired.IsZero() || audience.Hides(r.Module) || !scope.VisibleIn(r.Scope, caller, project, false) {
			continue
		}
		recs = append(recs, r)
	}
	return recs, audience, nil
}

// visibleInstructions is the caller's instructions: those of visibleRecords
// that are instructions, minus any whose governing module the audience hides,
// with the sizes of the modules that remain.
// visibleRecords filters by a record's own module, but a working-memory
// preference is governed by identity, so a deployment that declares memory
// audience any must not hand a consumer an identity instruction. Both the
// endpoint and the context tool read through here so they cannot differ.
func visibleInstructions(d Deps, caller, project string, consumer bool) ([]instructions.Record, []instructions.Size, error) {
	recs, audience, err := visibleRecords(d, caller, project, consumer)
	if err != nil {
		return nil, nil, err
	}
	got := []instructions.Record{}
	for _, r := range instructions.Collect(d.Set, recs) {
		if !audience.Hides(r.Module) {
			got = append(got, r)
		}
	}
	// A hidden module's size is withheld with its records: its name and
	// budget are as much the person's as its text.
	var sizes []instructions.Size
	for _, z := range instructions.Sizes(d.Set, got) {
		if !audience.Hides(z.Module) {
			sizes = append(sizes, z)
		}
	}
	if sizes == nil {
		sizes = []instructions.Size{}
	}
	return got, sizes, nil
}

// InstructionsHandler serves the person's instructions for the plugin's
// hooks (spec §4.2, §10), under the same identity and auth as /context, and
// scope-filtered as the block is. A consumer sees only modules declared any
// (§11), which never includes identity.
func InstructionsHandler(d Deps, id identity.Identity, consumers map[string]bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
			return
		}
		caller, consumer, _ := Caller(id, consumers, r)
		project, err := scope.CheckProjectKey(r.URL.Query().Get("project"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		got, sizes, err := visibleInstructions(d, caller, project, consumer)
		if err != nil {
			log.Printf("instructions for %q: %v", caller, err)
			http.Error(w, "instructions unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Opening string                `json:"opening"`
			Records []instructions.Record `json:"records"`
			Sizes   []instructions.Size   `json:"sizes"`
		}{instructions.Opening, got, sizes})
	})
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
		project, err := scope.CheckProjectKey(r.URL.Query().Get("project"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		text, _, _, err := RenderContext(d, caller, project, consumer)
		if err != nil {
			log.Printf("context for %q: %v", caller, err)
			http.Error(w, "context unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, text)
	})
}

type contextIn struct {
	Project string `json:"project,omitempty" jsonschema:"filter the block to one project's scope: <owner>--<repo>, as the SessionStart hook computes it from the cwd's git remote (spec §5). Omit for none"`
}
type contextOut struct {
	Block        string              `json:"block" jsonschema:"the session context block, at most 2048 bytes: the agenda line first, then each enabled ratified-record module's summary in priority order. Scope-filtered to your machine; may be up to one sync interval behind another machine's writes"`
	Faults       []block.Fault       `json:"faults,omitempty" jsonschema:"modules whose summary exceeded their budget and were replaced by one line saying so, or agenda when its line had to be cut"`
	Agenda       *agenda.Item        `json:"agenda,omitempty" jsonschema:"the top agenda item: the record, why it is asked, the question to ask, its revision line and snooze count. Absent when nothing is due"`
	Instructions []instructions.Size `json:"instructions,omitempty" jsonschema:"each module's instructions — the confirmed records of kinds it marks instructions, delivered in full to every session and subagent beside the block — with their size and budget; bytes over budget is a fault /health reports (spec §10)"`
}

// registerContextTool registers the context tool: the same block RenderContext
// builds for GET /context, reachable from an MCP session.
func registerContextTool(s *mcp.Server, d Deps, caller string, consumer bool) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "context",
		Description: "The session context block: the one agenda item to raise first, then the person's confirmed " +
			"record by module, within 2 KB. Read it at the start of a session; it is your view, scope-filtered, " +
			"and may lag another machine's writes by up to one sync interval. Pass `project` to also include " +
			"that project's records; the SessionStart hook already does, from the cwd's git remote.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in contextIn) (*mcp.CallToolResult, contextOut, error) {
		project, err := scope.CheckProjectKey(in.Project)
		if err != nil {
			return nil, contextOut{}, err
		}
		blockText, faults, top, err := RenderContext(d, caller, project, consumer)
		if err != nil {
			return nil, contextOut{}, err
		}
		_, sizes, err := visibleInstructions(d, caller, project, consumer)
		if err != nil {
			return nil, contextOut{}, err
		}
		return nil, contextOut{Block: blockText, Faults: faults, Agenda: top, Instructions: sizes}, nil
	})
}
