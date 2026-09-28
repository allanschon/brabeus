package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/allanschon/brabeus/internal/agenda"
	"github.com/allanschon/brabeus/internal/block"
	"github.com/allanschon/brabeus/internal/identity"
	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/scope"
	"github.com/allanschon/brabeus/internal/store"
)

const Version = "0.3.0"

// The scope-refusal wording gate hands to each caller: read and delete point
// at include_all_scopes, which exists on those tools; review has no such
// parameter, so it points at the machine instead.
const (
	readScopeRefusal   = "%s is scoped %q and you are %q; pass include_all_scopes to read it anyway"
	deleteScopeRefusal = "%s is scoped %q and you are %q; pass include_all_scopes to delete it anyway"
	reviewScopeRefusal = "%s is scoped %q and you are %q; review it from that machine"
)

// pickRepo resolves the repo argument. An absent projects mirror is an error
// rather than a fallback: silently reading the wrong repository is worse than
// failing the call. A consumer is refused the mirror outright (spec §11): it
// is a second corpus with no modules of its own, so no per-module audience
// check could stand in for refusing it entirely.
func pickRepo(name string, memory, projects *store.Store, consumer bool) (*store.Store, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "memory":
		return memory, nil
	case "projects":
		if consumer {
			return nil, fmt.Errorf("the projects mirror is not readable by this caller")
		}
		if projects == nil {
			return nil, fmt.Errorf("the projects mirror is not configured")
		}
		return projects, nil
	}
	return nil, fmt.Errorf("unknown repo %q: use memory or projects", name)
}

// gate is the shared read-before-touch check for read, delete and review:
// missing file, then audience, then scope, in that order. refusalFmt takes
// the path, the record's scope and the caller, in that order, and is the one
// place the three tools' wording differs.
//
// Audience before scope: the scope refusal names the record's machine, which
// a consumer should not learn about a record it may not read at all — a
// consumer asking after a machine-scoped record in a hidden module must get
// the audience wording, never the scope one.
func gate(st *store.Store, rel, caller string, includeAll bool, audience store.Visibility, refusalFmt string) (store.Record, error) {
	r, _, ok := st.Peek(rel)
	if !ok {
		return store.Record{}, fmt.Errorf("no record at %q", rel)
	}
	if audience.Hides(r.Module) {
		return store.Record{}, fmt.Errorf("%s is not readable by this caller", rel)
	}
	if !scope.Visible(r.Scope, caller, includeAll) {
		return store.Record{}, fmt.Errorf(refusalFmt, rel, r.Scope, caller)
	}
	return r, nil
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

// Deps is what the server needs to build tools and the context block, gathered
// so New and RenderContext take one thing rather than drifting parameter lists.
type Deps struct {
	Memory, Projects *store.Store
	Set              *module.Set
	Block            *block.Renderer
	Now              func() time.Time // time.Now in production; tests pin it
}

// ParseConsumers reads BRABEUS_CONSUMERS: the machine names that are not the
// person's own sessions. Spec §11: a consumer sees only modules whose
// audience is any, and never writes.
func ParseConsumers(spec string) map[string]bool {
	out := map[string]bool{}
	for _, n := range strings.Split(spec, ",") {
		if n = scope.NormaliseHost(n); n != "" {
			out[n] = true
		}
	}
	return out
}

// Caller resolves who is calling and whether they are a consumer. A caller
// identity cannot name is a consumer: it already sees only global scope, and
// unnamed is not a class the person's record should trust.
func Caller(id identity.Identity, consumers map[string]bool, r *http.Request) (string, bool) {
	caller := scope.NormaliseHost(id.Machine(r))
	return caller, caller == "" || consumers[caller]
}

// audienceFor is the hide-set spec §11 requires: for a consumer, every module
// not declared audience any, and every file the migration has not tagged.
func audienceFor(set *module.Set, consumer bool) store.Visibility {
	v := store.Visibility{HideModules: map[string]bool{}}
	if !consumer {
		return v
	}
	v.HideUntagged = true
	for _, m := range set.Modules {
		if m.Audience != module.Any {
			v.HideModules[strings.ToLower(m.Name)] = true
		}
	}
	return v
}

// hiddenFor folds the search profile (§1.1) into the audience: by default
// ratified-record modules are searched only when asked, and naming a module
// asks for it. Audience is never overridden.
func hiddenFor(set *module.Set, consumer bool, profile, named string) (store.Visibility, error) {
	v := audienceFor(set, consumer)
	named = strings.ToLower(strings.TrimSpace(named))
	var hideProfile module.Profile
	switch profile {
	case "", "working-memory":
		hideProfile = module.RatifiedRecord
	case "ratified-record":
		hideProfile = module.WorkingMemory
	case "all":
	default:
		return v, fmt.Errorf("profile %q: use working-memory (default), ratified-record or all", profile)
	}
	for _, m := range set.Modules {
		if hideProfile != "" && m.Profile == hideProfile && strings.ToLower(m.Name) != named {
			v.HideModules[strings.ToLower(m.Name)] = true
		}
	}
	return v, nil
}

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
		items = agenda.Compute(d.Set, recs, now)
	}
	text, faults := d.Block.Render(items, recs, now, audience)
	if top, ok := agenda.Top(items); ok {
		return text, faults, &top, nil
	}
	return text, faults, nil, nil
}

// LogUnresolvedCaller logs the one line both /mcp and /context say when a
// caller could not be resolved. Each entry point calls it itself rather than
// sharing a single choke point, so a resolved caller is never logged at all —
// this would be a line per request otherwise.
func LogUnresolvedCaller(caller, remoteAddr string) {
	if caller == "" {
		log.Printf("unresolved caller from %s: treated as a consumer — machine-scoped memories and every self module are hidden", remoteAddr)
	}
}

// ContextHandler serves the same block as text for the plugin's SessionStart
// hook, under the same identity and auth as /mcp.
func ContextHandler(d Deps, id identity.Identity, consumers map[string]bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
			return
		}
		caller, consumer := Caller(id, consumers, r)
		LogUnresolvedCaller(caller, r.RemoteAddr)
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
	Profile          string `json:"profile,omitempty" jsonschema:"working-memory (default): the assistant's own notes; ratified-record: the person's confirmed record; all"`
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

type contextIn struct{}
type contextOut struct {
	Block  string        `json:"block" jsonschema:"the session context block, at most 2048 bytes: the agenda line first, then each enabled ratified-record module's summary in priority order. Scope-filtered to your machine; may be up to one sync interval behind another machine's writes"`
	Faults []block.Fault `json:"faults,omitempty" jsonschema:"modules whose summary exceeded their budget and were replaced by one line saying so, or agenda when its line had to be cut"`
	Agenda *agenda.Item  `json:"agenda,omitempty" jsonschema:"the top agenda item: the record, why it is asked, the question to ask, its revision line and snooze count. Absent when nothing is due"`
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
		Description: "Read one memory in full. Set repo=projects to read the read-only mirror instead.",
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
			"modules are searched; pass profile: ratified-record or all, or name a module, to widen. Set repo=projects " +
			"to search the read-only mirror. An empty result can mean the scope you asked for is not visible to you; " +
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

	return s
}
