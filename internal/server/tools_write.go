package server

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/allanschon/brabeus/internal/store"
)

// The scope-refusal wording gate hands to delete and review: delete points at
// include_all_scopes, which exists on that tool; review has no such
// parameter, so it points at the machine instead.
const (
	deleteScopeRefusal = "%s is scoped %q and you are %q; pass include_all_scopes to delete it anyway"
	reviewScopeRefusal = "%s is scoped %q and you are %q; review it from that machine"
)

type deleteIn struct {
	Path             string `json:"path" jsonschema:"path of the memory to remove, relative to the repository root"`
	IncludeAllScopes bool   `json:"include_all_scopes,omitempty" jsonschema:"delete a memory scoped to another machine anyway"`
}
type deleteOut struct {
	Path   string `json:"path"`
	Commit string `json:"commit"`
}

type writeIn struct {
	Path        string            `json:"path" jsonschema:"path relative to the repository root, ending in .md; a ratified-record module's records live at <module>/<kind>/<slug>.md"`
	Name        string            `json:"name" jsonschema:"short kebab-case slug, also the display name in the index"`
	Description string            `json:"description" jsonschema:"one line, used to judge relevance during recall"`
	Module      string            `json:"module,omitempty" jsonschema:"the module this record belongs to, e.g. memory; list shows what is enabled"`
	Kind        string            `json:"kind,omitempty" jsonschema:"the module's kind, e.g. note, trap, preference, project"`
	Fields      map[string]string `json:"fields,omitempty" jsonschema:"the kind's declared fields, required ones included; a working-memory kind has none"`
	Scope       string            `json:"scope,omitempty" jsonschema:"global, project/<slug> or machine/<host>; defaults to global when omitted"`
	Body        string            `json:"body" jsonschema:"the record itself, markdown, without frontmatter"`
	Type        string            `json:"type,omitempty" jsonschema:"deprecated: the pre-module type (user, feedback, project, reference), mapped to a module and kind for one milestone; name module and kind instead"`
}
type writeOut struct {
	Path   string `json:"path"`
	Commit string `json:"commit"`
	// Similar is a WARNING, never a refusal: the memory is already saved.
	Similar []store.Match `json:"similar,omitempty" jsonschema:"existing memories that appear to say the same thing, closest first. The write SUCCEEDED - this is for you to judge whether to merge or delete one of them. Empty when nothing is close, or when the meaning index is unavailable"`
}

type reviewIn struct {
	Path     string            `json:"path" jsonschema:"the record's path, as list or context give it"`
	Question string            `json:"question" jsonschema:"the question that was asked, verbatim; it goes into the commit"`
	Verdict  string            `json:"verdict" jsonschema:"confirmed, corrected, retired or later"`
	Answer   string            `json:"answer" jsonschema:"the person's answer, in their own words; it goes into the commit and is required"`
	Body     string            `json:"body,omitempty" jsonschema:"corrected only: the new body; empty keeps the old"`
	Fields   map[string]string `json:"fields,omitempty" jsonschema:"corrected only: fields to replace, validated like a write"`
}
type reviewOut struct {
	Path   string `json:"path"`
	Commit string `json:"commit"`
}

// registerWriteTools registers the tools that change the record: write,
// delete and review. Each refuses a consumer outright (spec §11): a write
// influenced by an untrusted reader would be injection into the next session.
func registerWriteTools(s *mcp.Server, d Deps, caller string, consumer bool, audience store.Visibility) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "write",
		Description: "Save a record. Name its module and kind (list shows what is enabled); a ratified-record " +
			"module's kinds declare required fields, passed in `fields`. Composes the frontmatter, updates the " +
			"index and pushes. " +
			"The reply may carry `similar`: existing memories that appear to say the same thing. " +
			"The write always succeeds - read them and decide whether to merge or delete one, " +
			"because a duplicate is not a ranking problem and no search change will fix it.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in writeIn) (*mcp.CallToolResult, writeOut, error) {
		if consumer {
			return nil, writeOut{}, fmt.Errorf("a consumer never writes (spec §11)")
		}
		// Report the path the store used, not the spelling we were given.
		rel, err := store.MemoryPath(in.Path)
		if err != nil {
			return nil, writeOut{}, err
		}
		// Asked BEFORE the write, or the new memory is in the index and is
		// its own nearest duplicate. Its error is deliberately discarded:
		// this is a warning, and nothing about it may stand between a fact and
		// being recorded.
		similar, _ := d.Memory.SimilarTo(in.Description, d.Memory.DuplicateThreshold(), rel)

		commit, err := d.Memory.Write(rel, store.Record{
			Name: in.Name, Description: in.Description, Module: in.Module, Kind: in.Kind,
			Fields: in.Fields, Scope: in.Scope, Body: in.Body, Type: in.Type,
		}, caller)
		if err != nil {
			return nil, writeOut{}, err
		}
		return nil, writeOut{Path: rel, Commit: commit, Similar: similar}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "delete",
		Description: "Remove a memory. Deletes the file, drops its line from MEMORY.md and pushes. " +
			"Use when a memory is wrong in a way no rewrite fixes, or superseded. Git keeps the history.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in deleteIn) (*mcp.CallToolResult, deleteOut, error) {
		if consumer {
			return nil, deleteOut{}, fmt.Errorf("a consumer never writes (spec §11)")
		}
		rel, err := store.MemoryPath(in.Path)
		if err != nil {
			return nil, deleteOut{}, err
		}
		// Same gate as read: you should not be able to remove something this
		// machine is not allowed to see, or a module this caller may not see.
		if _, err := gate(d.Memory, rel, caller, in.IncludeAllScopes, audience, deleteScopeRefusal); err != nil {
			return nil, deleteOut{}, err
		}
		commit, err := d.Memory.Delete(rel, caller)
		if err != nil {
			return nil, deleteOut{}, err
		}
		return nil, deleteOut{Path: rel, Commit: commit}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "review",
		Description: "Answer one agenda question. The only operation that moves a record's reviewed date; the " +
			"question, the verdict and the person's answer go into the commit. Verdicts: confirmed, corrected " +
			"(with body and/or fields), retired, later (counts a snooze).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in reviewIn) (*mcp.CallToolResult, reviewOut, error) {
		if consumer {
			return nil, reviewOut{}, fmt.Errorf("a consumer never reviews (spec §11)")
		}
		rel, err := store.MemoryPath(in.Path)
		if err != nil {
			return nil, reviewOut{}, err
		}
		// A review is the person's own act on their own machine's view: no
		// include_all_scopes, and its own scope-refusal wording.
		if _, err := gate(d.Memory, rel, caller, false, audience, reviewScopeRefusal); err != nil {
			return nil, reviewOut{}, err
		}
		commit, err := d.Memory.Review(rel, store.ReviewInput{
			Question: in.Question, Verdict: store.Verdict(in.Verdict), Answer: in.Answer, Body: in.Body, Fields: in.Fields,
		}, caller)
		if err != nil {
			return nil, reviewOut{}, err
		}
		return nil, reviewOut{Path: rel, Commit: commit}, nil
	})
}
