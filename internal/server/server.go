package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/allanschon/brabeus/internal/scope"
	"github.com/allanschon/brabeus/internal/store"
)

const Version = "0.2.0"

// pickRepo resolves the repo argument. An absent projects mirror is an error
// rather than a fallback: silently reading the wrong repository is worse than
// failing the call.
func pickRepo(name string, memory, projects *store.Store) (*store.Store, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "memory":
		return memory, nil
	case "projects":
		if projects == nil {
			return nil, fmt.Errorf("the projects mirror is not configured")
		}
		return projects, nil
	}
	return nil, fmt.Errorf("unknown repo %q: use memory or projects", name)
}

// authMiddleware sits in the request path even when it does nothing. On the
// Tailscale network "none" is correct; putting the seam here from the start
// makes a later Tailscale Funnel a configuration change rather than a refactor.
//
// Misconfiguration is refused at construction, not per request: a server that
// starts in bearer mode with no token would reject every call while looking
// configured.
func AuthMiddleware(next http.Handler, mode, token string) (http.Handler, error) {
	switch mode {
	case "none":
		return next, nil
	case "bearer":
		if token == "" {
			return nil, fmt.Errorf("auth mode is bearer but no token is set")
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || strings.TrimSpace(got) != token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		}), nil
	}
	return nil, fmt.Errorf("unknown auth mode %q: use none or bearer", mode)
}

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

func filterEntries(in []store.Entry, caller string, includeAll bool) []store.Entry {
	out := make([]store.Entry, 0, len(in))
	for _, e := range in {
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
	Repo             string `json:"repo,omitempty" jsonschema:"memory (default) or projects for the read-only mirror"`
	IncludeAllScopes bool   `json:"include_all_scopes,omitempty" jsonschema:"read a memory scoped to another machine anyway"`
}
type readOut struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type searchIn struct {
	Query            string `json:"query" jsonschema:"what to look for, in your own words; matching is by relevance, not exact text. Wrap a phrase in double quotes to require it literally, e.g. \"grub.cfg\""`
	Repo             string `json:"repo,omitempty" jsonschema:"memory (default) or projects"`
	Limit            int    `json:"limit,omitempty" jsonschema:"maximum memories to return, default 50"`
	Scope            string `json:"scope,omitempty" jsonschema:"only this scope, e.g. global or machine/desk; a scope belonging to another machine needs include_all_scopes to return anything"`
	Module           string `json:"module,omitempty" jsonschema:"only this module"`
	Kind             string `json:"kind,omitempty" jsonschema:"only this kind, e.g. trap"`
	Prefix           string `json:"prefix,omitempty" jsonschema:"only memories in this directory and below, e.g. infra/"`
	IncludeAllScopes bool   `json:"include_all_scopes,omitempty" jsonschema:"also search memories scoped to other machines"`
}
type searchOut struct {
	Vocabulary store.Vocabulary `json:"vocabulary" jsonschema:"what the store holds, to narrow a further search by"`
	Dense      string           `json:"dense" jsonschema:"whether the meaning-based leg served this query: on, building (the corpus is still being embedded after a restart) or unavailable. Anything but on means these results are keyword-only, so a paraphrase may have missed"`
	Matches    []store.Match    `json:"matches"`
}

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
	Scope       string            `json:"scope" jsonschema:"global, project/<slug> or machine/<host>"`
	Body        string            `json:"body" jsonschema:"the record itself, markdown, without frontmatter"`
	Type        string            `json:"type,omitempty" jsonschema:"deprecated: the pre-module type (user, feedback, project, reference), mapped to a module and kind for one milestone; name module and kind instead"`
}
type writeOut struct {
	Path   string `json:"path"`
	Commit string `json:"commit"`
	// Similar is a WARNING, never a refusal: the memory is already saved.
	Similar []store.Match `json:"similar,omitempty" jsonschema:"existing memories that appear to say the same thing, closest first. The write SUCCEEDED - this is for you to judge whether to merge or delete one of them. Empty when nothing is close, or when the meaning index is unavailable"`
}

// newMCPServer builds a server bound to one caller. Identity is fixed when the
// session is created, so the tools close over it rather than re-deriving it.
func New(memory, projects *store.Store, caller string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:        "brabeus",
		Title:       "Brabeus",
		Description: "A private, git-backed personal record. Durable facts about the person and their projects.",
		Version:     Version,
	}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list",
		Description: "List memories with their module, kind, scope and one-line description. Start here to see what is known.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in listIn) (*mcp.CallToolResult, listOut, error) {
		all, err := memory.List(in.Prefix)
		if err != nil {
			return nil, listOut{}, err
		}
		return nil, listOut{Caller: caller, Vocabulary: memory.Vocabulary(store.Visibility{}), Entries: filterEntries(all, caller, in.IncludeAllScopes)}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "read",
		Description: "Read one memory in full. Set repo=projects to read the read-only mirror instead.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in readIn) (*mcp.CallToolResult, readOut, error) {
		st, err := pickRepo(in.Repo, memory, projects)
		if err != nil {
			return nil, readOut{}, err
		}
		rel, err := store.CanonicalPath(in.Path)
		if err != nil {
			return nil, readOut{}, err
		}
		if st == memory {
			if sc := st.ScopeOf(rel); !scope.Visible(sc, caller, in.IncludeAllScopes) {
				return nil, readOut{}, fmt.Errorf(
					"%s is scoped %q and you are %q; pass include_all_scopes to read it anyway",
					rel, sc, caller)
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
			"literally, e.g. \"grub.cfg\". Narrow with scope, module, kind or prefix. Set repo=projects to search the " +
			"read-only mirror. An empty result can mean the scope you asked for is not visible to you; " +
			"include_all_scopes will include it. Check the `dense` field: anything but \"on\" means these results " +
			"are keyword-only and a paraphrase may have missed.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, searchOut, error) {
		st, err := pickRepo(in.Repo, memory, projects)
		if err != nil {
			return nil, searchOut{}, err
		}
		f := store.SearchFilter{Scope: in.Scope, Module: in.Module, Kind: in.Kind, Prefix: in.Prefix}
		// The mirror has no scopes; only memories are filtered.
		if st == memory {
			f.Keep = func(sc string) bool { return scope.Visible(sc, caller, in.IncludeAllScopes) }
		}
		hits, dense, err := st.Search(in.Query, searchLimit(in.Limit), f)
		if err != nil {
			return nil, searchOut{}, err
		}
		return nil, searchOut{Vocabulary: st.Vocabulary(store.Visibility{}), Matches: hits, Dense: dense}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "write",
		Description: "Save a record. Name its module and kind (list shows what is enabled); a ratified-record " +
			"module's kinds declare required fields, passed in `fields`. Composes the frontmatter, updates the " +
			"index and pushes. " +
			"The reply may carry `similar`: existing memories that appear to say the same thing. " +
			"The write always succeeds - read them and decide whether to merge or delete one, " +
			"because a duplicate is not a ranking problem and no search change will fix it.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in writeIn) (*mcp.CallToolResult, writeOut, error) {
		// Report the path the store used, not the spelling we were given.
		rel, err := store.MemoryPath(in.Path)
		if err != nil {
			return nil, writeOut{}, err
		}
		// Asked BEFORE the write, or the new memory is in the index and is
		// its own nearest duplicate. Its error is deliberately discarded:
		// this is a warning, and nothing about it may stand between a fact and
		// being recorded.
		similar, _ := memory.SimilarTo(in.Description, memory.DuplicateThreshold(), rel)

		commit, err := memory.Write(rel, store.Record{
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
		rel, err := store.MemoryPath(in.Path)
		if err != nil {
			return nil, deleteOut{}, err
		}
		// Same visibility rule as read: you should not be able to remove
		// something this machine is not allowed to see.
		if sc := memory.ScopeOf(rel); !scope.Visible(sc, caller, in.IncludeAllScopes) {
			return nil, deleteOut{}, fmt.Errorf(
				"%s is scoped %q and you are %q; pass include_all_scopes to delete it anyway",
				rel, sc, caller)
		}
		commit, err := memory.Delete(rel, caller)
		if err != nil {
			return nil, deleteOut{}, err
		}
		return nil, deleteOut{Path: rel, Commit: commit}, nil
	})

	return s
}
