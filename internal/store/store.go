package store

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/retrieval"
	"github.com/allanschon/brabeus/internal/scope"
)

// Store is a git working copy. This kernel is the only writer of the record's
// repository, so serialising every mutation here is what makes conflicts
// structurally impossible rather than merely unlikely.
//
// The working copy is disposable. The remote is the record.
type Store struct {
	mu sync.Mutex

	Dir         string
	RemoteURL   string
	Branch      string
	ReadOnly    bool
	SSHCommand  string // empty for a local path, e.g. in tests
	CommitName  string
	CommitEmail string

	// index is derived state, rebuilt from the markdown on every sync and
	// after every mutation. It is never persisted: the markdown is the
	// record, so there is nothing here that can drift from it.
	index *retrieval.BM25

	// Embedder is the dense leg's model. Nil is a supported state: the store
	// is then a keyword-only server — lexical results, and it says so.
	Embedder retrieval.Embedder

	// dense holds the vectors for index.Docs, or nil.
	//
	// THE ALIGNMENT INVARIANT: dense's lanes are indexed BY POSITION
	// against index.Docs. An older dense index left installed beside a newer
	// corpus scores every memory against its neighbour's meaning and looks
	// perfectly healthy doing it. ⇒ reindex drops it, and a pass installs its
	// result only if no newer pass has started since.
	dense      *retrieval.Dense
	denseState string
	// denseGen is that check. reindex bumps it; a finishing pass compares.
	denseGen uint64
	// denseFingerprint is the corpus the installed index was built from, so a
	// sync that changed nothing can keep it instead of rebuilding.
	denseFingerprint string
	// denseCache wraps Embedder for the CORPUS PASS ONLY.
	//
	// The read path is deliberately uncached, and the reason is not
	// performance. A cached query never reaches the sidecar, so a sidecar that
	// is DOWN keeps reporting `on` for any query anyone has asked before —
	// masking the exact failure the `dense` state exists to expose, and
	// defeating any monitor built on it, because a monitor asks the SAME
	// question every time. Measured against the live server 2026-09-09.
	//
	// The corpus pass keeps its cache: without it, adding one memory
	// re-embeds all of them and the fifteen-minute git pull burns ~30 s of CPU
	// forever. Queries are one-off and gain nothing from being remembered.
	denseCache *retrieval.CachingEmbedder

	// modules is the loaded set every write is validated against. Nil means
	// no validation, which only the read-only mirror uses.
	modules *module.Set
}

func (s *Store) SetModules(set *module.Set) { s.modules = set }

// The three states the caller is told about. The deploy agent decides
// readiness by reading this back from a search: only denseOn is ready. It
// collapses correctly — still-embedding and sidecar-down are different causes
// and both mean not ready.
const (
	denseOn          = "on"
	denseBuilding    = "building"
	denseUnavailable = "unavailable"
)

func (s *Store) env() []string {
	e := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if s.SSHCommand != "" {
		e = append(e, "GIT_SSH_COMMAND="+s.SSHCommand)
	}
	return e
}

func (s *Store) git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = s.env()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// Ensure clones the repository if the working copy is missing, and otherwise
// brings it in line with the remote. Safe to call repeatedly.
func (s *Store) Ensure() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(filepath.Join(s.Dir, ".git")); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(s.Dir), 0o750); err != nil {
			return err
		}
		if _, err := s.git("", "clone", "--quiet", "--branch", s.Branch, s.RemoteURL, s.Dir); err != nil {
			return err
		}
	}
	if !s.ReadOnly {
		if _, err := s.git(s.Dir, "config", "user.name", s.CommitName); err != nil {
			return err
		}
		if _, err := s.git(s.Dir, "config", "user.email", s.CommitEmail); err != nil {
			return err
		}
	}
	return s.sync()
}

// sync discards local state and takes the remote's. Assumes the lock is held.
//
// Discarding rather than merging is deliberate: a dirty working copy is the
// residue of an earlier failure, and inheriting it would ship one caller's
// abandoned edit inside another caller's commit.
func (s *Store) sync() error {
	if _, err := s.git(s.Dir, "fetch", "--quiet", "origin", s.Branch); err != nil {
		return err
	}
	if _, err := s.git(s.Dir, "reset", "--hard", "--quiet", "origin/"+s.Branch); err != nil {
		return err
	}
	if _, err := s.git(s.Dir, "clean", "-qfd"); err != nil {
		return err
	}
	return s.reindex()
}

// reindex rebuilds the search index from the working copy. Assumes the lock is
// held. A full rebuild rather than an incremental update is deliberate: the
// corpus is small, the rebuild is milliseconds, and an incremental path is a
// second definition of the index that can disagree with the first.
func (s *Store) reindex() error {
	var docs []retrieval.Doc
	err := s.walkMarkdown(func(rel, full string) error {
		// MEMORY.md carries every memory's name and description, so it
		// matches nearly every query and would come back beside the memory it
		// points at — one need, two results. CONVENTIONS.md is house style,
		// not a memory. Neither is retrievable content, and skipping them here
		// is also what makes Vocabulary's count the true memory count.
		if strings.EqualFold(rel, indexFile) || strings.EqualFold(rel, conventionsFile) {
			return nil
		}
		b, readErr := os.ReadFile(full)
		if readErr != nil {
			return nil // a file we cannot read is not a memory we can rank
		}
		docs = append(docs, retrieval.IndexDoc(rel, string(b)))
		return nil
	})
	s.index = retrieval.NewBM25(docs)

	// The fifteen-minute timer is a GIT PULL — fetch, reset, clean — not a
	// rebuild schedule, and most of the time it changes nothing. When the
	// corpus is byte-for-byte what the current dense index was built from, keep
	// it: the lanes are aligned by POSITION, and identical content in identical
	// walk order means the positions still line up.
	//
	// Without this the index is dropped and rebuilt every quarter hour
	// forever, reporting "building" each time for no reason — which is a flap
	// under the deploy agent's readiness poll.
	fp := corpusFingerprint(s.index.Docs)
	if s.dense != nil && fp == s.denseFingerprint {
		return err
	}

	// Otherwise drop the old vectors in the SAME critical section that
	// replaces the corpus. Holding on to them for even a moment would leave
	// lanes indexed against documents that have moved.
	s.dense = nil
	s.denseGen++
	if s.Embedder == nil {
		s.denseState = denseUnavailable
	} else {
		s.denseState = denseBuilding
		// Rebuilt when the embedder itself changes, so a test that swaps one in
		// does not inherit the previous one's vectors.
		if s.denseCache == nil || s.denseCache.Inner != s.Embedder {
			s.denseCache = retrieval.NewCachingEmbedder(s.Embedder)
		}
		go s.buildDenseAsync(s.denseGen, fp, s.index.Docs, s.denseCache)
	}
	return err
}

// corpusFingerprint identifies the exact corpus a dense index belongs to.
//
// It covers path AND content AND order, because all three are load-bearing:
// content because an edited memory needs re-embedding, path because a rename
// changes the description lane's fallback, and order because the lanes are
// indexed positionally.
func corpusFingerprint(docs []retrieval.Doc) string {
	h := sha256.New()
	for i := range docs {
		h.Write([]byte(docs[i].Path))
		h.Write([]byte{0})
		h.Write([]byte(docs[i].Content))
		h.Write([]byte{0})
	}
	return string(h.Sum(nil))
}

// buildDenseAsync embeds the corpus WITHOUT holding the store lock.
//
// This is the whole point of the function existing. Search and reindex share
// s.mu, and a ~30 s embed pass inside that lock would block every search for
// its duration rather than answering degraded — which is precisely what the
// deploy agent's readiness poll cannot tolerate, because it reads the status
// back from a search.
//
// Overlapping passes are ALLOWED and are cheap to permit: a pass that
// finishes after a newer one started simply discards its work. Serialising
// them instead would put a second mechanism beside the generation check, and
// the newest content must win either way.
func (s *Store) buildDenseAsync(gen uint64, fp string, docs []retrieval.Doc, e retrieval.Embedder) {
	// A generous deadline. The read path gets 2 s (chosen during calibration);
	// a corpus pass is ~30 s today and grows with the store, so a read-path
	// timeout here would fail every rebuild.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	// Rotate the cache generation so lane texts no memory uses any more fall
	// out rather than accumulating for the life of the process.
	//
	// A local, unnamed interface rather than retrieval.passBeginner: the type
	// is unexported there (retrieval imports nothing, so it cannot hand this
	// name across the boundary), and the one-method structural check needs no
	// name to be useful — anything with BeginPass() satisfies it.
	if p, ok := e.(interface{ BeginPass() }); ok {
		p.BeginPass()
	}
	ix, err := retrieval.BuildDense(ctx, e, docs)

	s.mu.Lock()
	defer s.mu.Unlock()
	if gen != s.denseGen {
		return // a newer pass owns the answer now
	}
	if err != nil {
		s.denseState = denseUnavailable
		log.Printf("dense index: %v", err)
		return
	}
	s.dense, s.denseState, s.denseFingerprint = ix, denseOn, fp
}

func (s *Store) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sync()
}

type Entry struct {
	Path        string `json:"path" jsonschema:"path relative to the repository root"`
	Module      string `json:"module,omitempty" jsonschema:"the module whose schema this record obeys; empty on a record the one-time migration has not yet tagged"`
	Kind        string `json:"kind,omitempty" jsonschema:"the module-defined kind"`
	Scope       string `json:"scope,omitempty" jsonschema:"global, project/<slug> or machine/<host>"`
	Description string `json:"description,omitempty" jsonschema:"one-line summary from the frontmatter"`
}

type Match struct {
	Path  string  `json:"path"`
	Line  int     `json:"line"`
	Text  string  `json:"text"`
	Score float64 `json:"score" jsonschema:"fused relevance across the keyword and meaning legs; higher is better, comparable only within one result set. Not a similarity or a percentage - reciprocal rank fusion scores positions, because a keyword score and a meaning distance are not comparable quantities"`
}

// walkMarkdown visits every tracked markdown file, skipping git's own storage.
func (s *Store) walkMarkdown(fn func(rel, full string) error) error {
	return filepath.Walk(s.Dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(s.Dir, p)
		if err != nil {
			return err
		}
		return fn(filepath.ToSlash(rel), p)
	})
}

// List returns every memory under dir, a directory named however the caller
// likes: infra, infra/, ./infra and infra/. are one directory, and none of
// them is infrastructure/. The root's own spellings mean everything.
func (s *Store) List(dir string) ([]Entry, error) {
	dir = path.Clean(dir)
	if dir == "." {
		dir = ""
	} else {
		clean, err := CanonicalPath(dir)
		if err != nil {
			return nil, err
		}
		dir = clean + "/"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var out []Entry
	err := s.walkMarkdown(func(rel, full string) error {
		if !strings.HasPrefix(rel, dir) {
			return nil
		}
		b, err := os.ReadFile(full)
		if err != nil {
			return nil
		}
		fm := parseFrontmatter(string(b))
		out = append(out, Entry{
			Path:        rel,
			Module:      fm["module"],
			Kind:        fm["kind"],
			Scope:       fm["scope"],
			Description: fm["description"],
		})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}

// ScopeOf reads only the scope of one memory, for filtering search hits.
func (s *Store) ScopeOf(rel string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	full, err := resolvePath(s.Dir, rel)
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return ""
	}
	return parseFrontmatter(string(b))["scope"]
}

func (s *Store) Read(rel string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	full, err := resolvePath(s.Dir, rel)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// SearchFilter narrows a search before ranking. An empty field means "any".
type SearchFilter struct {
	Scope  string // exact scope, e.g. machine/desk
	Module string // exact module, e.g. memory
	Kind   string // exact kind, e.g. trap
	Prefix string // path prefix, e.g. infra/
	Keep   func(scope string) bool
}

func (f SearchFilter) keeps(d *retrieval.Doc) bool {
	if f.Scope != "" && !strings.EqualFold(d.Scope, f.Scope) {
		return false
	}
	if f.Module != "" && !strings.EqualFold(d.Module, f.Module) {
		return false
	}
	if f.Kind != "" && !strings.EqualFold(d.Kind, f.Kind) {
		return false
	}
	if f.Prefix != "" {
		// Directory semantics, the same as List: infra, infra/ and ./infra
		// are one directory, and none of them is infrastructure/. A raw
		// HasPrefix here would make the two tools disagree about what a
		// directory is — the exact bug List was fixed for.
		// Trim the slashes too: path.Clean("/infra") keeps its leading one,
		// and no memory path has one, so an untrimmed prefix would filter
		// everything out and return an empty result rather than an error.
		if dir := strings.Trim(path.Clean(f.Prefix), "/"); dir != "." && dir != "" &&
			!strings.HasPrefix(d.Path, dir+"/") {
			return false
		}
	}
	return f.Keep == nil || f.Keep(d.Scope)
}

// Vocabulary is what the store holds, without any of what it says.
//
// A caller who did not write a memory does not know its wording. Handing it
// the scopes, modules, kinds and directories that exist lets it pick from a
// menu rather than guess, for a few dozen tokens and no content.
type Vocabulary struct {
	Memories int      `json:"memories" jsonschema:"how many memories the store holds; MEMORY.md and CONVENTIONS.md are listed and readable but not counted here or searchable"`
	Scopes   []string `json:"scopes" jsonschema:"every scope in use"`
	Modules  []string `json:"modules" jsonschema:"every module in use"`
	Kinds    []string `json:"kinds" jsonschema:"every module/kind pair in use"`
	Prefixes []string `json:"prefixes" jsonschema:"every top-level directory in use"`
}

func (s *Store) Vocabulary() Vocabulary {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := Vocabulary{}
	if s.index == nil {
		return v
	}
	scopes, modules, kinds, prefixes := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i := range s.index.Docs {
		d := &s.index.Docs[i]
		v.Memories++
		if d.Scope != "" {
			scopes[d.Scope] = true
		}
		if d.Module != "" {
			modules[d.Module] = true
		}
		if d.Module != "" && d.Kind != "" {
			kinds[d.Module+"/"+d.Kind] = true
		}
		if dir, _, ok := strings.Cut(d.Path, "/"); ok {
			prefixes[dir] = true
		}
	}
	v.Scopes, v.Modules, v.Kinds, v.Prefixes = sortedKeys(scopes), sortedKeys(modules), sortedKeys(kinds), sortedKeys(prefixes)
	return v
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Search ranks memories by BM25 over the in-memory index, best first, one
// entry per memory carrying the line it matched on.
//
// One entry per memory, not per line. A substring scan returned every
// matching line, so twenty hits in one file spent a limit of twenty and
// crowded out every other memory.
func (s *Store) Search(query string, limit int, f SearchFilter) ([]Match, string, error) {
	if strings.TrimSpace(query) == "" {
		return nil, "", fmt.Errorf("empty query")
	}

	// One consistent snapshot, then the lock goes. index and dense are read
	// TOGETHER because dense's lanes are positionally aligned with index.Docs;
	// reading them under separate locks could pair a new corpus with old
	// vectors. Everything after this point is pure computation and one model
	// call, and the model call must NEVER happen under the lock — that is
	// what would block every other search behind the sidecar.
	s.mu.Lock()
	ix, dense, state, e := s.index, s.dense, s.denseState, s.Embedder
	s.mu.Unlock()

	if ix == nil {
		return nil, state, fmt.Errorf("the search index is not built yet")
	}

	var qvec []float32
	if dense != nil && e != nil {
		ctx, cancel := context.WithTimeout(context.Background(), denseQueryTimeout)
		vecs, err := e.Embed(ctx, []string{retrieval.QueryText(query)})
		cancel()
		if err != nil || len(vecs) == 0 {
			// Degrade, never fail. A lexical answer beats no answer, and the
			// caller is told which one it got rather than left to guess.
			state = denseUnavailable
		} else {
			qvec = vecs[0]
		}
	}
	ranked := retrieval.RankFused(ix, dense, qvec, query, f.keeps)

	out := make([]Match, 0, limit)
	for _, r := range ranked {
		if len(out) >= limit {
			break
		}
		line, text := retrieval.Snippet(r.Doc.Content, query)
		out = append(out, Match{Path: r.Doc.Path, Line: line, Text: text, Score: r.Score})
	}
	return out, state, nil
}

const (
	// Chosen during calibration. Measured 2026-09-08: a warm query embed is
	// 43-54 ms and a cold one 1,112 ms, which is why OLLAMA_KEEP_ALIVE=-1 is
	// mandatory on the sidecar — without it the common case sits uncomfortably
	// near this bound.
	denseQueryTimeout = 2 * time.Second
)

// The store's structure: the index every write maintains, and the conventions
// it is written under. Neither is a memory.
const (
	indexFile       = "MEMORY.md"
	conventionsFile = "CONVENTIONS.md"
)

// memoryPath is the one rule for what may be written or deleted as a memory,
// and returns the canonical spelling that every later step must use: the
// disk, the index, the commit message and the reply to the caller. Write and
// Delete share it so neither can drift from the other.
//
// The structural files are judged on that canonical path, so ./MEMORY.md does
// not walk past a check that only knows one spelling, and case-insensitively,
// so a case-folding filesystem cannot be reached by a different spelling
// either. Overwriting the index replaces it with a memory carrying its own
// index line; deleting it breaks every future write.
func MemoryPath(rel string) (string, error) {
	clean, err := CanonicalPath(rel)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(clean, ".md") {
		return "", fmt.Errorf("memories are markdown: %q must end in .md", rel)
	}
	if strings.EqualFold(clean, indexFile) || strings.EqualFold(clean, conventionsFile) {
		return "", fmt.Errorf("%s is part of the store's structure, not a memory", rel)
	}
	return clean, nil
}

// authorFor renders the calling machine as a git author, so `git log --format=%an`
// answers "which machine wrote this" without any new storage.
//
// Author, not committer: the SERVER committed, on BEHALF of the machine.
//
//	Ensure() sets repo-local user.name/user.email, so the committer stays the
//	server and --author overrides only the author.
//
// An empty caller must not produce a malformed --author. identity.go returns ""
//
//	for a caller it cannot name, and a failed commit would lose the memory.
func authorFor(caller string) string {
	if strings.TrimSpace(caller) == "" {
		caller = "unknown"
	}
	return fmt.Sprintf("%s <%s@memory.local>", caller, caller)
}

// checkFields enforces the kind's schema (spec §5): every required field
// present and non-empty, nothing undeclared, and nothing the kernel owns.
func checkFields(kind module.Kind, fields map[string]string) error {
	declared := map[string]bool{}
	for _, f := range kind.Fields {
		declared[f] = true
		if strings.TrimSpace(fields[f]) == "" {
			return fmt.Errorf("field %q is required by this kind", f)
		}
	}
	for _, f := range kind.Optional {
		declared[f] = true
	}
	for f := range fields {
		if kernelKeys[f] && f != "id" {
			return fmt.Errorf("field %q is owned by the kernel and cannot be set by a write", f)
		}
		if !declared[f] {
			return fmt.Errorf("field %q is not declared by this kind", f)
		}
	}
	return nil
}

// checkLayout is the path rule per profile (decision D4). memoryPath has
// already canonicalised rel and refused the structural files. The path's
// kind segment must be the record's kind, or the path and the frontmatter
// could disagree about what a file is.
func checkLayout(m module.Manifest, kind, rel string) error {
	if m.Profile == module.WorkingMemory && m.Layout == "free" {
		return nil
	}
	parts := strings.Split(rel, "/")
	if len(parts) != 3 || parts[0] != m.Name {
		return fmt.Errorf("a %s record lives at %s/<kind>/<slug>.md, not %q", m.Name, m.Name, rel)
	}
	if parts[1] != kind {
		return fmt.Errorf("a %s/%s record lives at %s/%s/<slug>.md, not %q", m.Name, kind, m.Name, kind, rel)
	}
	return nil
}

// Write composes the record, refreshes the index and pushes, returning the
// new commit — or "no change" when the store already says exactly this.
func (s *Store) Write(rel string, r Record, caller string) (string, error) {
	if s.ReadOnly {
		return "", fmt.Errorf("this store is read-only")
	}
	rel, err := MemoryPath(rel)
	if err != nil {
		return "", err
	}
	// One line each, once, so the file, the index and the commit message
	// cannot disagree about what was written. Whitespace alone is nothing.
	r.Name, r.Description = oneLine(r.Name), oneLine(r.Description)
	if r.Name == "" || r.Description == "" {
		return "", fmt.Errorf("name and description are required: the index is built from them")
	}
	// Stored in the form scope.Visible() compares, so a scope that validates is a
	// scope that matches.
	if r.Scope, err = scope.CheckScope(r.Scope); err != nil {
		return "", err
	}
	if s.modules == nil {
		return "", fmt.Errorf("this store validates nothing and therefore writes nothing")
	}
	// The deprecated alias (decision D6): type in, module and kind out.
	if r.Module == "" && r.Type != "" {
		m, k, ok := s.modules.LegacyKind(r.Type)
		if !ok {
			return "", fmt.Errorf("type %q is not mapped by any enabled module; name a module and kind", r.Type)
		}
		r.Module, r.Kind = m, k
	}
	man, ok := s.modules.Module(r.Module)
	if !ok {
		return "", fmt.Errorf("module %q is not enabled; enabled: %s", r.Module, strings.Join(s.modules.Names(), ", "))
	}
	kind, ok := man.Kinds[r.Kind]
	if !ok {
		return "", fmt.Errorf("module %s has no kind %q", r.Module, r.Kind)
	}
	if r.Fields == nil {
		r.Fields = map[string]string{}
	}
	if err := checkFields(kind, r.Fields); err != nil {
		return "", err
	}
	r.ID = r.Fields["id"]
	if err := checkLayout(man, r.Kind, rel); err != nil {
		return "", err
	}
	if shape := credentialShape(r.Body); shape != "" {
		return "", fmt.Errorf("refused: the body looks like it contains %s; credentials never enter the record (spec §11)", shape)
	}
	for k, v := range r.Fields {
		if shape := credentialShape(v); shape != "" {
			return "", fmt.Errorf("refused: field %s looks like it contains %s; credentials never enter the record (spec §11)", k, shape)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	full, err := resolvePath(s.Dir, rel)
	if err != nil {
		return "", err
	}
	// Take the remote's state first. This both discards an earlier failure's
	// residue and picks up anything another writer pushed meanwhile.
	if err := s.sync(); err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return "", err
	}
	var meta Meta
	if old, err := os.ReadFile(full); err == nil {
		_, meta = ParseRecord(string(old))
	}
	meta.Updated, meta.LegacyType = now(), ""
	content := compose(r, meta, append(append([]string{}, kind.Fields...), kind.Optional...))
	// Keep the existing file byte-for-byte when only the stamp would move, so
	// an unchanged rewrite stays a no-op rather than an empty commit.
	if old, err := os.ReadFile(full); err == nil &&
		sansUpdated(string(old)) == sansUpdated(content) && !staleStamp(string(old)) {
		content = string(old)
	}
	if err := os.WriteFile(full, []byte(content), 0o640); err != nil {
		return "", err
	}

	idxPath := filepath.Join(s.Dir, indexFile)
	idx, err := os.ReadFile(idxPath)
	if err != nil {
		return "", fmt.Errorf("reading the index: %w", err)
	}
	updated := updateIndexContent(string(idx), rel, r.Name, r.Description)
	if err := os.WriteFile(idxPath, []byte(updated), 0o640); err != nil {
		return "", err
	}

	if _, err := s.git(s.Dir, "add", "-A"); err != nil {
		return "", err
	}
	status, err := s.git(s.Dir, "status", "--porcelain")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(status) == "" {
		return "no change", nil
	}

	msg := fmt.Sprintf("%s/%s: %s\n\n%s\n\nWritten through the kernel at %s.",
		r.Module, r.Kind, r.Name, r.Description, time.Now().UTC().Format(time.RFC3339))
	if _, err := s.git(s.Dir, "commit", "--quiet", "--author", authorFor(caller), "-m", msg); err != nil {
		return "", err
	}
	if _, err := s.git(s.Dir, "push", "--quiet", "origin", s.Branch); err != nil {
		return "", err
	}
	if err := s.reindex(); err != nil {
		return "", err
	}
	head, err := s.git(s.Dir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(head), nil
}

// Delete removes a memory and its index entry, and pushes.
//
// Same lock, same pull-first, same single writer as Write — deleting by hand
// from a clone would break the invariant that makes conflicts impossible.
func (s *Store) Delete(rel string, caller string) (string, error) {
	if s.ReadOnly {
		return "", fmt.Errorf("this store is read-only")
	}
	rel, err := MemoryPath(rel)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	full, err := resolvePath(s.Dir, rel)
	if err != nil {
		return "", err
	}
	if err := s.sync(); err != nil {
		return "", err
	}
	// Checked AFTER the sync, so a memory another clone just pushed is
	// found rather than reported absent.
	if _, err := os.Stat(full); err != nil {
		return "", fmt.Errorf("no memory at %q — nothing was deleted", rel)
	}
	if err := os.Remove(full); err != nil {
		return "", err
	}

	idxPath := filepath.Join(s.Dir, indexFile)
	idx, err := os.ReadFile(idxPath)
	if err != nil {
		return "", fmt.Errorf("reading the index: %w", err)
	}
	if err := os.WriteFile(idxPath, []byte(removeIndexEntry(string(idx), rel)), 0o640); err != nil {
		return "", err
	}

	if _, err := s.git(s.Dir, "add", "-A"); err != nil {
		return "", err
	}
	msg := fmt.Sprintf("memory: remove %s\n\nDeleted via the memory MCP server at %s.",
		rel, time.Now().UTC().Format(time.RFC3339))
	if _, err := s.git(s.Dir, "commit", "--quiet", "--author", authorFor(caller), "-m", msg); err != nil {
		return "", err
	}
	if _, err := s.git(s.Dir, "push", "--quiet", "origin", s.Branch); err != nil {
		return "", err
	}
	if err := s.reindex(); err != nil {
		return "", err
	}
	head, err := s.git(s.Dir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(head), nil
}

// defaultDuplicateThreshold is where the warning starts.
//
// Calibrated, not guessed: measured 2026-09-08, the store's real duplicate
// pair — two memories recording the same standing preference, written a day
// apart under different types — scores 0.853 on the description lane with
// embeddinggemma.
//
// This is why the model matters. nomic-embed-text scores nearly everything
// around 0.94, so under it no threshold separates a duplicate from a
// neighbour and this whole feature would be noise.
const defaultDuplicateThreshold = 0.85

// DuplicateThreshold is settable without a rebuild.
//
// A malformed value falls back to the default rather than to zero. Zero
// would flag EVERY memory as a duplicate of every other, turning a helpful
// warning into noise that gets ignored — and an ignored warning is worse than
// no warning, because it also hides the true positives.
func (s *Store) DuplicateThreshold() float64 {
	if v := os.Getenv("BRABEUS_DUPLICATE_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		log.Printf("BRABEUS_DUPLICATE_THRESHOLD=%q is not a number; using %v", v, defaultDuplicateThreshold)
	}
	return defaultDuplicateThreshold
}

// SimilarTo finds memories whose DESCRIPTION already says what this one says.
//
// The description lane only. Bodies share boilerplate — the "**Why:**" and
// "**How to apply:**" lines the conventions ask for — so comparing them would
// flag half the store against the other half. The description is the one line
// written to say what a memory is about, which is exactly the question being
// asked here.
//
// `writing` is the path about to be written, excluded from the results.
//
// It never blocks a write and never returns an error the caller must handle:
// no dense index, or a sidecar that is down, simply means no warnings. Losing a
// memory is the worst failure this system has; a missed duplicate warning is
// not close.
func (s *Store) SimilarTo(description string, threshold float64, writing string) ([]Match, error) {
	s.mu.Lock()
	ix, dense, e := s.index, s.dense, s.Embedder
	s.mu.Unlock()
	if ix == nil || dense == nil || e == nil {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), denseQueryTimeout)
	defer cancel()
	// documentText, not queryText: this asks "does an existing memory say the
	// same thing", so both sides must be embedded as documents. Comparing a
	// query-prefixed vector against document-prefixed ones would measure the
	// wrong distance and silently mis-calibrate the threshold.
	vecs, err := e.Embed(ctx, []string{retrieval.DocumentText(description)})
	if err != nil || len(vecs) == 0 {
		return nil, nil
	}

	var out []Match
	for _, r := range dense.RankDesc(vecs[0], ix.Docs, nil) {
		if r.Score < threshold {
			break // rankDesc is sorted, so nothing below this can qualify
		}
		// Never the memory being written. On an UPDATE its previous version
		// is still in the index with the same description, so it matches at
		// ~1.0 and the reply says the memory duplicates itself — noise that
		// teaches the reader to ignore the field. Seen in production the first
		// time this was used to RESOLVE a duplicate.
		if writing != "" && r.Doc.Path == writing {
			continue
		}
		line, text := retrieval.DescriptionLine(r.Doc.Content)
		out = append(out, Match{Path: r.Doc.Path, Line: line, Text: text, Score: r.Score})
	}
	return out, nil
}
