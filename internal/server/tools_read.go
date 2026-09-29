package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/scope"
	"github.com/allanschon/brabeus/internal/store"
)

// searchLimit applies the default and the ceiling. Above the ceiling clamps
// rather than resets: a caller who asked for a thousand wants many, not fifty.
func searchLimit(n int) int {
	switch {
	case n <= 0:
		return 50
	case n > 500:
		return 500
	}
	return n
}

func filterEntries(in []store.Entry, caller string, includeAll bool, v store.Visibility) []store.Entry {
	out := make([]store.Entry, 0, len(in))
	for _, e := range in {
		if v.Hides(e.Module) {
			continue
		}
		if scope.Visible(e.Scope, caller, includeAll) {
			out = append(out, e)
		}
	}
	return out
}

type listIn struct {
	Prefix           string `json:"prefix,omitempty" jsonschema:"only return memories in this directory and below, e.g. projects/example--repo"`
	IncludeAllScopes bool   `json:"include_all_scopes,omitempty" jsonschema:"also return memories scoped to other machines; off by default because they are usually wrong here"`
}
type listOut struct {
	Caller     string           `json:"caller" jsonschema:"the machine this server resolved you to, empty if it could not"`
	Vocabulary store.Vocabulary `json:"vocabulary" jsonschema:"what the store holds, to search or filter by"`
	Entries    []store.Entry    `json:"entries"`
}

type readIn struct {
	Path             string `json:"path" jsonschema:"path relative to the repository root, e.g. personal/style.md"`
	Repo             string `json:"repo,omitempty" jsonschema:"memory (default), or notebook (also accepted as projects) for the person's own notes"`
	IncludeAllScopes bool   `json:"include_all_scopes,omitempty" jsonschema:"read a memory scoped to another machine anyway"`
}
type readOut struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type searchIn struct {
	Query            string `json:"query" jsonschema:"what to look for, in your own words; matching is by relevance, not exact text. Wrap a phrase in double quotes to require it literally, e.g. \"grub.cfg\""`
	Repo             string `json:"repo,omitempty" jsonschema:"memory (default), or notebook (also accepted as projects) for the person's own notes"`
	Limit            int    `json:"limit,omitempty" jsonschema:"maximum memories to return, default 50"`
	Scope            string `json:"scope,omitempty" jsonschema:"only this scope, e.g. global or machine/desk; a scope belonging to another machine needs include_all_scopes to return anything"`
	Module           string `json:"module,omitempty" jsonschema:"only this module"`
	Kind             string `json:"kind,omitempty" jsonschema:"only this kind, e.g. trap"`
	Prefix           string `json:"prefix,omitempty" jsonschema:"only memories in this directory and below, e.g. infra/"`
	IncludeAllScopes bool   `json:"include_all_scopes,omitempty" jsonschema:"also search memories scoped to other machines"`
	Profile          string `json:"profile,omitempty" jsonschema:"working-memory (default): the assistant's own notes; ratified-record: the person's confirmed record; all"`
}
type searchOut struct {
	Vocabulary store.Vocabulary `json:"vocabulary" jsonschema:"what the store holds, to narrow a further search by"`
	Dense      string           `json:"dense" jsonschema:"whether the meaning-based leg served this query: on, building (the corpus is still being embedded after a restart) or unavailable. Anything but on means these results are keyword-only, so a paraphrase may have missed"`
	Matches    []store.Match    `json:"matches"`
}

type kindOut struct {
	Fields        []string `json:"fields"`
	Optional      []string `json:"optional,omitempty"`
	FreshnessDays int      `json:"freshness_days,omitempty"`
	Interview     string   `json:"interview,omitempty"`
	First         string   `json:"first,omitempty"`
	Lenses        []string `json:"lenses,omitempty"`
	Draft         string   `json:"draft,omitempty"`
	DueField      string   `json:"due_field,omitempty"`
}
type moduleOut struct {
	Name       string             `json:"name"`
	Profile    string             `json:"profile"`
	Priority   int                `json:"priority"`
	Intro      string             `json:"intro,omitempty"`
	Onboarding []string           `json:"onboarding,omitempty"`
	Kinds      map[string]kindOut `json:"kinds"`
}
type modulesIn struct{}
type modulesOut struct {
	Modules []moduleOut `json:"modules" jsonschema:"the enabled modules this caller may read, in priority order, with each kind's fields and the interview's questions: first, lenses, draft, interview"`
}

// modulesFor is the read-only view of the module set spec §11's `modules`
// tool serves: every enabled module the caller's audience does not hide, with
// its kinds' interview scaffolding. A hidden module is absent, not empty —
// its name is itself something a consumer may not learn (spec §11).
func modulesFor(set *module.Set, v store.Visibility) modulesOut {
	out := modulesOut{Modules: []moduleOut{}}
	for _, m := range set.Modules {
		if v.Hides(m.Name) {
			continue
		}
		mo := moduleOut{Name: m.Name, Profile: string(m.Profile), Priority: m.Priority, Intro: m.Intro, Onboarding: m.Onboarding, Kinds: map[string]kindOut{}}
		for name, k := range m.Kinds {
			mo.Kinds[name] = kindOut{Fields: k.Fields, Optional: k.Optional, FreshnessDays: k.FreshnessDays, Interview: k.Interview, First: k.First, Lenses: k.Lenses, Draft: k.Draft, DueField: k.DueField}
		}
		out.Modules = append(out.Modules, mo)
	}
	return out
}

// registerReadTools registers the tools that only ever read: list, read,
// search and modules. Kept apart from the write tools so the read/write
// boundary in the server package matches the boundary spec §11 enforces at
// runtime.
func registerReadTools(s *mcp.Server, d Deps, caller string, consumer bool, audience store.Visibility) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list",
		Description: "List memories with their module, kind, scope and one-line description. Start here to see what is known.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in listIn) (*mcp.CallToolResult, listOut, error) {
		all, err := d.Memory.List(in.Prefix)
		if err != nil {
			return nil, listOut{}, err
		}
		return nil, listOut{Caller: caller, Vocabulary: d.Memory.Vocabulary(audience), Entries: filterEntries(all, caller, in.IncludeAllScopes, audience)}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "read",
		Description: "Read one memory in full. Set repo=notebook (also accepted as projects) to read the person's own notes instead.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in readIn) (*mcp.CallToolResult, readOut, error) {
		st, err := pickRepo(in.Repo, d.Memory, d.Projects, consumer)
		if err != nil {
			return nil, readOut{}, err
		}
		rel, err := store.CanonicalPath(in.Path)
		if err != nil {
			return nil, readOut{}, err
		}
		if st == d.Memory {
			if _, err := gate(st, rel, caller, in.IncludeAllScopes, audience, readScopeRefusal); err != nil {
				return nil, readOut{}, err
			}
		}
		c, err := st.Read(rel)
		if err != nil {
			return nil, readOut{}, err
		}
		return nil, readOut{Path: rel, Content: c}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "search",
		Description: "Search memories by relevance, best first, one result per memory with the line it matched. " +
			"Ask in your own words and in your own wording - it matches MEANING as well as keywords, so a memory " +
			"that says \"terse\" is found by asking for \"brief\". Wrap a phrase in double quotes to require it " +
			"literally, e.g. \"grub.cfg\". Narrow with scope, module, kind or prefix. By default only working-memory " +
			"modules are searched; pass profile: ratified-record or all, or name a module, to widen. Set repo=notebook " +
			"(also accepted as projects) to search the person's own notes. An empty result can mean the scope you asked for is not visible to you; " +
			"include_all_scopes will include it. Check the `dense` field: anything but \"on\" means these results " +
			"are keyword-only and a paraphrase may have missed.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, searchOut, error) {
		// Validated before the repo branch, so an invalid profile is refused
		// on the mirror too rather than silently ignored there.
		v, err := hiddenFor(d.Set, consumer, in.Profile, in.Module)
		if err != nil {
			return nil, searchOut{}, err
		}
		st, err := pickRepo(in.Repo, d.Memory, d.Projects, consumer)
		if err != nil {
			return nil, searchOut{}, err
		}
		f := store.SearchFilter{Scope: in.Scope, Module: in.Module, Kind: in.Kind, Prefix: in.Prefix}
		// The mirror has no scopes and no modules of its own; it ignores
		// profile and scope filtering exactly the same way.
		if st == d.Memory {
			f.Keep = func(sc string) bool { return scope.Visible(sc, caller, in.IncludeAllScopes) }
			f.Visibility = v
		}
		hits, dense, err := st.Search(in.Query, searchLimit(in.Limit), f)
		if err != nil {
			return nil, searchOut{}, err
		}
		// The vocabulary is the session's own menu — the audience hide-set,
		// not this call's profile-narrowed one — so widening the profile on
		// the next search is visibly still possible.
		return nil, searchOut{Vocabulary: st.Vocabulary(audience), Matches: hits, Dense: dense}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "modules",
		Description: "The enabled modules and their kinds, fields and interview questions — lenses, first, draft " +
			"and interview — for the caller. Read it before an interview; a module you may not read is absent.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in modulesIn) (*mcp.CallToolResult, modulesOut, error) {
		return nil, modulesFor(d.Set, audience), nil
	})
}
