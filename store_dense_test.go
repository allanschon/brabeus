package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// blockingEmbedder parks inside Embed until the test releases it, and announces
// that it has arrived.
//
// The announcement is what makes the concurrency test real. Without it the
// test could call Search before the pass has even reached the embedder, and
// would pass with the lock held for the whole rebuild — the exact regression it
// exists to catch.
type blockingEmbedder struct {
	entered chan struct{}
	release chan struct{}
	once    bool
}

func newBlockingEmbedder() *blockingEmbedder {
	return &blockingEmbedder{entered: make(chan struct{}, 1), release: make(chan struct{})}
}

func (e *blockingEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if !e.once {
		e.once = true
		select {
		case e.entered <- struct{}{}:
		default:
		}
		select {
		case <-e.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{1, 0}
	}
	return out, nil
}

func writeMemory(t *testing.T, s *Store, path, desc, body string) {
	t.Helper()
	if _, err := s.Write(path, Memory{
		Name: "n", Description: desc, Type: "reference", Scope: "global", Body: body,
	}, "test"); err != nil {
		t.Fatalf("Write %s: %v", path, err)
	}
}

// THE CONTRACT WITH THE DEPLOY AGENT. It decides readiness by issuing a
// search and reading the status back, and requires that "still-embedding
// and sidecar-down both report degraded". A Search that BLOCKS on the embed
// pass instead of answering turns that poll into a stack of timeouts, and a
// ~30 s pass is long enough for it to matter.
func TestSearchAnswersWhileTheEmbedPassIsRunning(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeMemory(t, s, "infra/a.md", "the printer jams", "the printer jams often")

	e := newBlockingEmbedder()
	s.Embedder = e

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := s.Sync(); err != nil {
			t.Errorf("Sync: %v", err)
		}
	}()

	// Wait until the pass is genuinely INSIDE the embedder before searching.
	select {
	case <-e.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the embed pass never reached the embedder")
	}

	answered := make(chan string, 1)
	go func() {
		_, state, err := s.Search("printer", 10, SearchFilter{})
		if err != nil {
			t.Errorf("Search during the embed pass: %v", err)
		}
		answered <- state
	}()

	select {
	case state := <-answered:
		if state != denseBuilding {
			t.Errorf("Search reported %q mid-pass, want %q", state, denseBuilding)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Search blocked on the embed pass instead of answering degraded — " +
			"the embed is holding the store lock")
	}

	close(e.release)
	<-done

	// Sync returns as soon as the LEXICAL index is rebuilt — it does not wait
	// for the embed pass, which is the entire point of this design. So poll for
	// the state to settle rather than assuming Sync was a barrier.
	waitForDense(t, s, denseOn)
}

// waitForDense polls until the store reports the state, or gives up loudly.
func waitForDense(t *testing.T, s *Store, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		s.mu.Lock()
		got = s.denseState
		s.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("dense state settled at %q, want %q", got, want)
}

// With no embedder at all the store is a keyword-only server: lexical
// results, and it says so rather than pretending.
func TestSearchIsLexicalOnlyWithNoEmbedder(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeMemory(t, s, "infra/a.md", "the printer jams", "the printer jams often")

	hits, state, err := s.Search("printer", 10, SearchFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Error("lexical search returned nothing")
	}
	if state != denseUnavailable {
		t.Errorf("state = %q, want %q", state, denseUnavailable)
	}
}

// A sidecar that errors must degrade, never fail the search. A degraded
// answer beats no answer, and the caller is told which it got.
func TestSearchDegradesWhenTheSidecarErrors(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeMemory(t, s, "infra/a.md", "the printer jams", "the printer jams often")
	s.Embedder = &stubEmbedder{
		fn:  func(string) []float32 { return []float32{1, 0} },
		err: fmt.Errorf("sidecar down"),
	}
	if err := s.Sync(); err != nil {
		t.Fatalf("Sync must not fail because the sidecar is down: %v", err)
	}

	hits, state, err := s.Search("printer", 10, SearchFilter{})
	if err != nil {
		t.Fatalf("Search must not fail because the sidecar is down: %v", err)
	}
	if len(hits) == 0 {
		t.Error("lexical results were lost when the dense leg failed")
	}
	if state == denseOn {
		t.Errorf("state = %q, want a degraded state", state)
	}
}

// The alignment invariant. The dense lanes are indexed BY POSITION against
// the document slice they were built from, so a rebuild must never leave an
// older dense index installed beside a newer corpus — every position would be
// off by one memory and every answer confidently wrong.
func TestARebuildNeverLeavesAStaleDenseIndexInstalled(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeMemory(t, s, "infra/a.md", "the printer jams", "the printer jams often")
	s.Embedder = &stubEmbedder{fn: func(string) []float32 { return []float32{1, 0} }}
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}

	// A second memory changes the corpus. Whatever the dense state is
	// afterwards, it must describe the NEW corpus, not the old one.
	writeMemory(t, s, "infra/b.md", "the scanner jams", "the scanner jams often")

	s.mu.Lock()
	docs, dense := len(s.index.docs), s.dense
	s.mu.Unlock()
	if dense != nil && len(dense.desc) != docs {
		t.Errorf("dense index has %d entries against %d documents — the lanes are "+
			"misaligned with the corpus", len(dense.desc), docs)
	}
}

// Filters bind both legs. A scope the caller may not see must not leak back
// through the new one.
func TestFiltersApplyToBothLegs(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeMemory(t, s, "infra/a.md", "the printer jams", "the printer jams often")
	s.Embedder = &stubEmbedder{fn: func(string) []float32 { return []float32{1, 0} }}
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}

	hits, _, err := s.Search("printer", 10, SearchFilter{
		Keep: func(scope string) bool { return false },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("a filter that keeps nothing returned %d hits — a leg is ignoring it", len(hits))
	}
}

// The live example this exists for: two memories record the same standing
// preference, written a day apart under different types, and lexical ranking
// puts them adjacent. No ranking change fixes that — it is a write-path
// problem wearing a retrieval problem's clothes.
func TestWriteWarnsAboutANearDuplicate(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	// Identical descriptions embed identically, so this is a duplicate by any
	// threshold — the test is about the warning arriving, not about calibration.
	s.Embedder = &stubEmbedder{fn: func(text string) []float32 {
		if text == documentText("secure boot stays off") {
			return []float32{1, 0}
		}
		return []float32{0, 1}
	}}
	writeMemory(t, s, "personal/a.md", "secure boot stays off", "first")
	waitForDense(t, s, denseOn)

	similar, err := s.SimilarTo("secure boot stays off", s.DuplicateThreshold(), "personal/new.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(similar) == 0 || similar[0].Path != "personal/a.md" {
		t.Errorf("SimilarTo found %v, want personal/a.md", similar)
	}
}

// The write SUCCEEDS regardless. Losing a memory is the worst failure this
// system has; a missed duplicate warning is not close. Any threshold is
// sometimes wrong, and when it is wrong a refusal blocks a real memory.
func TestWriteSucceedsEvenWhenItLooksLikeADuplicate(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	s.Embedder = &stubEmbedder{fn: func(string) []float32 { return []float32{1, 0} }}
	writeMemory(t, s, "personal/a.md", "identical", "first")
	waitForDense(t, s, denseOn)

	if _, err := s.Write("personal/b.md", Memory{
		Name: "n", Description: "identical", Type: "reference", Scope: "global", Body: "second",
	}, "test"); err != nil {
		t.Fatalf("a near-duplicate write must still succeed: %v", err)
	}
	if _, err := s.Read("personal/b.md"); err != nil {
		t.Errorf("the second memory was not written: %v", err)
	}
}

// A sidecar that is down must not make the duplicate check fail the write.
func TestSimilarToIsSilentWhenTheSidecarIsDown(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeMemory(t, s, "personal/a.md", "anything", "first")
	similar, err := s.SimilarTo("anything", 0.85, "personal/new.md")
	if err != nil {
		t.Errorf("SimilarTo must not error when there is no dense index: %v", err)
	}
	if len(similar) != 0 {
		t.Errorf("want no warnings without a dense index, got %v", similar)
	}
}

// The threshold is configurable without a rebuild. Calibration, measured
// 2026-09-08: the real Secure Boot pair scores 0.853.
func TestDuplicateThresholdComesFromTheEnvironment(t *testing.T) {
	s := &Store{}
	if got := s.DuplicateThreshold(); got != defaultDuplicateThreshold {
		t.Errorf("default threshold = %v, want %v", got, defaultDuplicateThreshold)
	}
	t.Setenv("BRABEUS_DUPLICATE_THRESHOLD", "0.42")
	if got := s.DuplicateThreshold(); got != 0.42 {
		t.Errorf("threshold = %v, want 0.42", got)
	}
	t.Setenv("BRABEUS_DUPLICATE_THRESHOLD", "not a number")
	if got := s.DuplicateThreshold(); got != defaultDuplicateThreshold {
		t.Errorf("a bad value gave %v; it must fall back to %v rather than 0, "+
			"which would flag every memory as a duplicate", got, defaultDuplicateThreshold)
	}
}

// The fifteen-minute timer is a GIT PULL, not a rebuild schedule: fetch,
// reset, clean. When it changes nothing, the dense index must survive it
// untouched. Without this the index is dropped and rebuilt every quarter hour
// forever, and the store reports "building" each time for no reason — which
// the deploy agent's readiness poll would see as a flap.
func TestAnUnchangedSyncDoesNotRebuildTheDenseIndex(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeMemory(t, s, "infra/a.md", "the printer jams", "the printer jams often")
	e := &stubEmbedder{fn: func(string) []float32 { return []float32{1, 0} }}
	s.Embedder = e
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	waitForDense(t, s, denseOn)

	e.mu.Lock()
	before := e.calls
	e.mu.Unlock()

	for range 3 {
		if err := s.Sync(); err != nil {
			t.Fatal(err)
		}
	}

	e.mu.Lock()
	after := e.calls
	e.mu.Unlock()
	if after != before {
		t.Errorf("three no-op syncs made %d extra embed calls; want 0", after-before)
	}
	s.mu.Lock()
	state, dense := s.denseState, s.dense
	s.mu.Unlock()
	if state != denseOn || dense == nil {
		t.Errorf("after a no-op sync the state is %q (index nil: %v); it should never have left %q",
			state, dense == nil, denseOn)
	}
}

// ...but a corpus that DID change must rebuild, or the lanes would be aligned
// against documents that have moved.
func TestAChangedCorpusStillRebuilds(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeMemory(t, s, "infra/a.md", "the printer jams", "the printer jams often")
	e := &stubEmbedder{fn: func(string) []float32 { return []float32{1, 0} }}
	s.Embedder = e
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	waitForDense(t, s, denseOn)
	e.mu.Lock()
	before := e.calls
	e.mu.Unlock()

	writeMemory(t, s, "infra/b.md", "the scanner jams", "the scanner jams often")
	waitForDense(t, s, denseOn)

	e.mu.Lock()
	after := e.calls
	e.mu.Unlock()
	if after == before {
		t.Error("a new memory did not trigger an embed pass")
	}
	s.mu.Lock()
	docs, dense := len(s.index.docs), s.dense
	s.mu.Unlock()
	if dense == nil || len(dense.desc) != docs {
		t.Errorf("dense index does not match the new corpus (%d docs)", docs)
	}
}

// The read path must NOT be cached, and this is why: a cached query never
// reaches the sidecar, so a sidecar that is DOWN keeps reporting `on` for any
// query anyone has asked before. Measured 2026-09-09 against the live server —
// the same query answered "on" with the container stopped, while a query never
// asked before correctly answered "unavailable".
//
// That masks the exact failure the `dense` field exists to expose, and it
// silently defeats any monitor built on it: the check that watches the dense
// leg asks the SAME question every time, which is the one case the cache
// answers without looking.
func TestSearchNeverServesAQueryFromCache(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeMemory(t, s, "infra/a.md", "the printer jams", "the printer jams often")
	inner := &stubEmbedder{fn: func(string) []float32 { return []float32{1, 0} }}
	s.Embedder = inner
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	waitForDense(t, s, denseOn)

	count := func() int {
		inner.mu.Lock()
		defer inner.mu.Unlock()
		n := 0
		for _, seen := range inner.seen {
			if seen == queryText("printer") {
				n++
			}
		}
		return n
	}
	for range 3 {
		if _, _, err := s.Search("printer", 5, SearchFilter{}); err != nil {
			t.Fatal(err)
		}
	}
	if got := count(); got != 3 {
		t.Errorf("the sidecar saw the query %d times across 3 searches, want 3 — "+
			"a cached query cannot notice the sidecar dying", got)
	}
}

// ...while the CORPUS pass must still be cached, or adding one memory
// re-embeds all of them and the fifteen-minute pull burns ~30 s of CPU forever.
func TestTheCorpusPassIsStillCached(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	writeMemory(t, s, "infra/a.md", "the printer jams", "the printer jams often")
	inner := &stubEmbedder{fn: func(string) []float32 { return []float32{1, 0} }}
	s.Embedder = inner
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	waitForDense(t, s, denseOn)

	lanes := func() int {
		inner.mu.Lock()
		defer inner.mu.Unlock()
		n := 0
		for _, seen := range inner.seen {
			if seen == documentText("the printer jams") {
				n++
			}
		}
		return n
	}
	before := lanes()
	// A second memory: the FIRST one's lanes must not be embedded again.
	writeMemory(t, s, "infra/b.md", "the scanner jams", "the scanner jams often")
	waitForDense(t, s, denseOn)
	if after := lanes(); after != before {
		t.Errorf("an unchanged memory was re-embedded (%d -> %d) when another was added", before, after)
	}
}

// An UPDATE must not flag itself. Rewriting a memory in place leaves the old
// version in the index with the same description, so it matches at ~1.0 and the
// reply says "this duplicates itself" — noise that trains the reader to ignore
// the field, which is worse than not having it.
//
// Seen in production 2026-09-09 while merging the Secure Boot pair: the
// write that RESOLVED the duplicate was answered with a 0.999 self-match.
func TestSimilarToIgnoresTheMemoryBeingWritten(t *testing.T) {
	s := newTestStore(t, newTestRemote(t))
	// Discriminating, not a constant: a stub that returns one vector for
	// everything makes every memory a perfect duplicate of every other, and the
	// test would then pass on a path tiebreak rather than on similarity.
	s.Embedder = &stubEmbedder{fn: func(text string) []float32 {
		if text == documentText("secure boot stays off") {
			return []float32{1, 0}
		}
		return []float32{0, 1}
	}}
	writeMemory(t, s, "personal/a.md", "secure boot stays off", "first")
	waitForDense(t, s, denseOn)

	// Updating personal/a.md: its own previous version must not come back.
	similar, err := s.SimilarTo("secure boot stays off", 0.5, "personal/a.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range similar {
		if m.Path == "personal/a.md" {
			t.Errorf("SimilarTo flagged the memory being written against itself (%.3f)", m.Score)
		}
	}

	// ...but a DIFFERENT memory saying the same thing still must.
	similar, err = s.SimilarTo("secure boot stays off", 0.5, "personal/b.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(similar) == 0 || similar[0].Path != "personal/a.md" {
		t.Errorf("SimilarTo found %v; a genuine duplicate must still be reported", similar)
	}
}
