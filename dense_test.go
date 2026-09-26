package main

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// stubEmbedder returns a vector per text from a function, so a test can shape
// the geometry it wants without a model.
//
// Guarded by a mutex because overlapping embed passes are a DESIGNED state,
// not an accident: reindex starts a pass without waiting for the previous one,
// and the generation counter sorts out which result gets installed. The race
// detector found this stub sharing seen/calls across two live passes, which is
// evidence that overlap really happens rather than a reason to prevent it.
type stubEmbedder struct {
	mu     sync.Mutex
	fn     func(text string) []float32
	err    error
	seen   []string
	calls  int
	failAt int // return err once this many texts have been seen; 0 disables
}

func (e *stubEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	e.seen = append(e.seen, texts...)
	if e.err != nil && (e.failAt == 0 || len(e.seen) >= e.failAt) {
		return nil, e.err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = e.fn(t)
	}
	return out, nil
}

func doc(path, desc, body string) indexedDoc {
	return indexDoc(path, "---\nname: n\ndescription: "+desc+"\n---\n"+body)
}

// Both lanes, for every document, indexed by position. Position is how the
// scan attributes a vector to a memory; a lane shorter than the corpus would
// silently score each memory against its neighbour's meaning.
func TestBuildDenseProducesBothLanesAlignedWithTheCorpus(t *testing.T) {
	docs := []indexedDoc{
		doc("a.md", "alpha", "body one"),
		doc("b.md", "beta", "body two"),
	}
	e := &stubEmbedder{fn: func(string) []float32 { return []float32{1, 0} }}
	ix, err := buildDense(t.Context(), e, docs)
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.desc) != len(docs) || len(ix.body) != len(docs) {
		t.Fatalf("lanes are %d/%d long, want %d each", len(ix.desc), len(ix.body), len(docs))
	}
	// The description lane must carry the description, and the body lane the
	// body — with the prefixes, since those are what the model was given.
	want := map[string]bool{
		documentText("alpha"):    true,
		documentText("beta"):     true,
		documentText("body one"): true,
		documentText("body two"): true,
	}
	for _, s := range e.seen {
		delete(want, s)
	}
	if len(want) > 0 {
		t.Errorf("these lane texts were never embedded: %v\ngot: %q", want, e.seen)
	}
}

// A partial index is worse than none: it ranks a subset of the corpus while
// looking complete, so a memory that exists simply never appears and nothing
// says why.
func TestBuildDenseReturnsNoIndexAtAllWhenABatchFails(t *testing.T) {
	docs := make([]indexedDoc, 40) // more than one batch
	for i := range docs {
		docs[i] = doc(fmt.Sprintf("%d.md", i), "d", "b")
	}
	e := &stubEmbedder{
		fn:     func(string) []float32 { return []float32{1, 0} },
		err:    fmt.Errorf("sidecar down"),
		failAt: 40, // succeed on the first batch, fail later
	}
	ix, err := buildDense(t.Context(), e, docs)
	if err == nil {
		t.Fatal("want an error when a batch fails")
	}
	if ix != nil {
		t.Errorf("want no index at all, got one with %d entries", len(ix.desc))
	}
}

// keep runs BEFORE scoring, the same as bm25Index.rank, so a caller's limit is
// spent on memories they may actually see.
func TestDenseRankAppliesTheFilter(t *testing.T) {
	docs := []indexedDoc{doc("keep.md", "a", "a"), doc("drop.md", "b", "b")}
	e := &stubEmbedder{fn: func(string) []float32 { return []float32{1, 0} }}
	ix, err := buildDense(t.Context(), e, docs)
	if err != nil {
		t.Fatal(err)
	}
	got := ix.rankDesc([]float32{1, 0}, docs, func(d *indexedDoc) bool { return d.path == "keep.md" })
	if len(got) != 1 || got[0].doc.path != "keep.md" {
		t.Errorf("rankDesc returned %v, want only keep.md", paths(got))
	}
}

// Ties break on path, or equal scores come back in slice order and the same
// query answers differently once anything upstream reorders the corpus. The
// same trap bm25.go already carries a comment about.
func TestDenseRankBreaksTiesOnPath(t *testing.T) {
	docs := []indexedDoc{doc("z.md", "same", "same"), doc("a.md", "same", "same")}
	e := &stubEmbedder{fn: func(string) []float32 { return []float32{0.6, 0.8} }}
	ix, err := buildDense(t.Context(), e, docs)
	if err != nil {
		t.Fatal(err)
	}
	got := ix.rankDesc([]float32{0.6, 0.8}, docs, nil)
	if len(got) != 2 || got[0].doc.path != "a.md" {
		t.Errorf("rankDesc returned %v, want a.md first on the tie", paths(got))
	}
}

// Best first, by cosine. Vectors arrive unit-normalised, so this is a dot
// product.
func TestDenseRankOrdersByCloseness(t *testing.T) {
	docs := []indexedDoc{doc("far.md", "far", "far"), doc("near.md", "near", "near")}
	e := &stubEmbedder{fn: func(text string) []float32 {
		if text == documentText("near") {
			return []float32{1, 0}
		}
		return []float32{0, 1}
	}}
	ix, err := buildDense(t.Context(), e, docs)
	if err != nil {
		t.Fatal(err)
	}
	got := ix.rankDesc([]float32{1, 0}, docs, nil)
	if len(got) != 2 || got[0].doc.path != "near.md" {
		t.Fatalf("rankDesc returned %v, want near.md first", paths(got))
	}
	if got[0].score <= got[1].score {
		t.Errorf("scores %v are not ordered", []float64{got[0].score, got[1].score})
	}
}

// A quoted phrase is a HARD requirement on both legs. If only BM25 honoured
// it, the dense leg would rank a different view of the corpus and an exact
// identifier like "grub.cfg" would come back beside memories that never
// mention it — the guarantee quoting exists to give.
func TestDenseRankHonoursQuotedPhrases(t *testing.T) {
	docs := []indexedDoc{doc("has.md", "d", "the grub.cfg file"), doc("lacks.md", "d", "unrelated")}
	e := &stubEmbedder{fn: func(string) []float32 { return []float32{1, 0} }}
	ix, err := buildDense(t.Context(), e, docs)
	if err != nil {
		t.Fatal(err)
	}
	keep := phraseFilter([]string{"grub.cfg"}, nil)
	got := ix.rankBody([]float32{1, 0}, docs, keep)
	if len(got) != 1 || got[0].doc.path != "has.md" {
		t.Errorf("rankBody returned %v, want only has.md", paths(got))
	}
}

// An empty corpus is a real state — a fresh clone before the first memory.
func TestBuildDenseHandlesAnEmptyCorpus(t *testing.T) {
	e := &stubEmbedder{fn: func(string) []float32 { return []float32{1, 0} }}
	ix, err := buildDense(t.Context(), e, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ix == nil || len(ix.desc) != 0 {
		t.Errorf("want an empty index, got %v", ix)
	}
	if len(ix.rankDesc([]float32{1, 0}, nil, nil)) != 0 {
		t.Error("ranking an empty index returned results")
	}
}

func paths(s []scored) []string {
	out := make([]string, len(s))
	for i, x := range s {
		out[i] = x.doc.path
	}
	return out
}

// k=60 is the standard and it is deliberate. Graphiti ships k=1, which
// collapses fusion into "whichever list ranked it first" — wrong for short,
// overlapping candidate lists like these. This pins the difference: agreement
// across both legs must beat a single leg's first place.
func TestFusionPrefersAgreementOverASingleLegsFirstPlace(t *testing.T) {
	docs := []indexedDoc{
		doc("agreed.md", "a", "a"), doc("solo.md", "b", "b"),
		doc("f1.md", "c", "c"), doc("f2.md", "d", "d"), doc("f3.md", "e", "e"),
		doc("f4.md", "f", "f"), doc("f5.md", "g", "g"),
	}
	agreed, solo := &docs[0], &docs[1]
	f := func(i int) *indexedDoc { return &docs[i] }

	// solo is 1st in one list and absent from the other. agreed is 4th in BOTH.
	//
	// The depth matters. If agreed were 2nd in both it would win at every k,
	// and this test would pass while proving nothing about k. At rank 4:
	// k=60 → solo 1/61=0.0164, agreed 2/64=0.0313  ⇒ agreement wins
	// k=1  → solo 1/2 =0.5,    agreed 2/5 =0.4     ⇒ first place wins
	left := []scored{{solo, 9}, {f(2), 8}, {f(3), 7}, {agreed, 1}}
	right := []scored{{f(4), 9}, {f(5), 8}, {f(6), 7}, {agreed, 1}}

	// Compare the two against EACH OTHER, not against the whole list. The
	// filler documents are ranked first in one leg apiece, so at k=1 they tie
	// with solo and take global first place on the path tiebreak — which says
	// nothing about the claim being made here.
	if at(t, fuseRRF(60, left, right), "agreed.md") > at(t, fuseRRF(60, left, right), "solo.md") {
		t.Errorf("k=60 ranked solo.md above agreed.md; agreement across legs should win")
	}
	// The contrast that shows k is doing the work.
	k1 := fuseRRF(1, left, right)
	if at(t, k1, "solo.md") > at(t, k1, "agreed.md") {
		t.Errorf("k=1 ranked agreed.md above solo.md — if this changed, the k=60 " +
			"case above may be passing for some reason other than k")
	}
}

// at reports where a path landed, failing the test if it is missing.
func at(t *testing.T, s []scored, path string) int {
	t.Helper()
	for i, x := range s {
		if x.doc.path == path {
			return i
		}
	}
	t.Fatalf("%s is absent from %v", path, paths(s))
	return -1
}

// Only positions count, never raw scores — which is the entire point,
// because a BM25 score and a cosine similarity are not comparable quantities.
func TestFusionIgnoresRawScoreMagnitude(t *testing.T) {
	docs := []indexedDoc{doc("a.md", "a", "a"), doc("b.md", "b", "b")}
	small := []scored{{&docs[0], 0.002}, {&docs[1], 0.001}}
	huge := []scored{{&docs[0], 2000}, {&docs[1], 1000}}
	if a, b := paths(fuseRRF(60, small)), paths(fuseRRF(60, huge)); !equalStrings(a, b) {
		t.Errorf("scaling scores changed the order: %v vs %v", a, b)
	}
}

// A leg that returned nothing — the dense index is nil, or a filter excluded
// everything — must not drop the other leg's results.
func TestFusionSurvivesAnEmptyLeg(t *testing.T) {
	docs := []indexedDoc{doc("a.md", "a", "a")}
	got := fuseRRF(60, []scored{{&docs[0], 1}}, nil)
	if len(got) != 1 || got[0].doc.path != "a.md" {
		t.Errorf("fuseRRF with an empty leg gave %v, want a.md", paths(got))
	}
}

// Ties break on path here too. Two documents each ranked first by one leg
// have identical fused scores, and without this the answer depends on map
// iteration order — different on every run.
func TestFusionBreaksTiesOnPath(t *testing.T) {
	docs := []indexedDoc{doc("z.md", "z", "z"), doc("a.md", "a", "a")}
	got := fuseRRF(60, []scored{{&docs[0], 1}}, []scored{{&docs[1], 1}})
	if len(got) != 2 || got[0].doc.path != "a.md" {
		t.Errorf("fuseRRF gave %v, want a.md first on the tie", paths(got))
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Pins the description lane's extra weight, and the failure it exists to
// prevent. Two lanes over one corpus produce EXACT ties: a document ranked 1st
// by description and 2nd by body ties with the document ranked the other way
// round, and the path tiebreak then decides on alphabetical order — which is
// no answer at all. Measured on the fixture, that is precisely how "be brief
// with me" returned short-commits.md instead of terse-answers.md.
func TestTheDescriptionLaneOutweighsTheBody(t *testing.T) {
	docs := []indexedDoc{doc("s-wrong.md", "a", "a"), doc("t-right.md", "b", "b")}
	wrong, right := &docs[0], &docs[1]

	// The mirror-image disagreement, with "s" sorting before "t" so the path
	// tiebreak would pick the wrong one if the lanes counted equally.
	descLane := []scored{{right, 0.49}, {wrong, 0.38}}
	bodyLane := []scored{{wrong, 0.36}, {right, 0.27}}

	equal := fuseRRF(60, descLane, bodyLane)
	if equal[0].doc.path != "s-wrong.md" {
		t.Fatalf("unweighted fusion gave %v; this test assumes the tie resolves "+
			"alphabetically, and no longer demonstrates anything if it does not", paths(equal))
	}

	got := fuseRRFWeighted(60,
		rrfLeg{descLane, descLaneWeight},
		rrfLeg{bodyLane, 1},
	)
	if got[0].doc.path != "t-right.md" {
		t.Errorf("weighted fusion gave %v, want t-right.md — the description lane "+
			"is not outweighing the body", paths(got))
	}
}

// THE REGRESSION THIS PREVENTS, found by reviewing the whole implementation
// rather than task by task: a cosine scan scores EVERY document, so fusing
// complete rankings returned the entire corpus for any query. On the real store
// that meant every search answering with 50 memories regardless of relevance,
// and the "nothing matched" signal disappearing entirely.
func TestDenseLanesDropDocumentsBelowTheFloor(t *testing.T) {
	docs := []indexedDoc{doc("near.md", "near", "near"), doc("far.md", "far", "far")}
	e := &stubEmbedder{fn: func(text string) []float32 {
		if text == documentText("near") {
			return []float32{1, 0}
		}
		return []float32{0, 1} // orthogonal: cosine 0 against the query
	}}
	ix, err := buildDense(t.Context(), e, docs)
	if err != nil {
		t.Fatal(err)
	}
	got := rankFused(newIndex(docs), ix, []float32{1, 0}, "nomatchhere", nil)
	if len(got) != 1 || got[0].doc.path != "near.md" {
		t.Errorf("fused returned %v; want only near.md — far.md scores 0 and is filler", paths(got))
	}
}

// The safety property that makes the floor safe to have at all: it applies
// to the DENSE legs only, so anything the keyword leg matched still comes back.
// Fused results are a SUPERSET of the lexical leg's results, and the floor can only
// ever remove a dense-only addition — never a keyword hit.
func TestTheFloorNeverDropsAKeywordMatch(t *testing.T) {
	docs := []indexedDoc{doc("a.md", "unrelated words", "the grub.cfg file lives here")}
	e := &stubEmbedder{fn: func(string) []float32 { return []float32{0, 1} }} // orthogonal
	ix, err := buildDense(t.Context(), e, docs)
	if err != nil {
		t.Fatal(err)
	}
	bm := newIndex(docs)
	// The dense legs contribute nothing at this query vector, but BM25 matches.
	got := rankFused(bm, ix, []float32{1, 0}, "grub.cfg", nil)
	if len(got) != 1 || got[0].doc.path != "a.md" {
		t.Errorf("fused returned %v; a keyword match must survive the floor", paths(got))
	}
}

// A query the store has nothing on must come back EMPTY, not filled.
func TestAQueryWithNothingRelevantReturnsNothing(t *testing.T) {
	docs := []indexedDoc{doc("a.md", "printers", "printers jam")}
	e := &stubEmbedder{fn: func(string) []float32 { return []float32{0, 1} }}
	ix, _ := buildDense(t.Context(), e, docs)
	got := rankFused(newIndex(docs), ix, []float32{1, 0}, "zzzz", nil)
	if len(got) != 0 {
		t.Errorf("fused returned %v for a query nothing matches; want nothing", paths(got))
	}
}
