package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
)

// denseIndex is the corpus as meaning rather than as words. Like bm25Index
// it is derived state held only in memory: the markdown is the record, and
// this is rebuilt from it. There is no persisted copy that could drift, which
// is also what keeps a rollback a tag swap rather than a migration.
//
// Both lanes are indexed BY POSITION against the same []indexedDoc the
// ranker is handed. That is the whole alignment contract — a lane of a
// different length would score each memory against its neighbour's meaning
// while looking perfectly healthy.
type denseIndex struct {
	desc [][]float32
	body [][]float32
}

// embedBatch is how many lane texts go in one request.
//
// Measured 2026-09-08: 32 lanes is ~3.7 s of CPU, and the whole 276-lane
// corpus pass is ~30 s. Smaller batches pay more round trips for no gain;
// much larger ones make a single failure cost more work.
const embedBatch = 32

// descLaneWeight is how much more the description lane counts than the body.
// The same 2:1 bm25.go applies to those fields, and for the same reason.
const descLaneWeight = 2

// defaultDenseFloor is the cosine below which a dense-only hit is filler.
//
// Why a floor exists at all: a cosine scan scores EVERY document, so fusing
// complete rankings returns the whole corpus for any query. Before this, every
// search on the real store answered with 50 memories regardless of relevance
// and "nothing matched" became unsayable.
//
// Measured over the fixture's 17 golden queries, 2026-09-09:
//
//	correct answers    min 0.452, median 0.594
//	best wrong answer  max 0.407, median 0.345
//	all wrong answers  median 0.154
//
// 0.35 sits well clear of BOTH edges rather than in the gap between them.
// Tuning it to the measured boundary (~0.42) would separate this 16-document
// fixture perfectly and be overfitting: on a corpus nine times larger a real
// match scoring 0.44 would be dropped silently, and silently losing a real
// answer is the worse of the two failures.
//
// The floor binds the DENSE legs ONLY. The lexical leg keeps its own
// score>0 rule, so fused results remain a SUPERSET of the lexical leg's — the
// floor can remove a dense-only addition and never a keyword match.
const defaultDenseFloor = 0.35

// denseFloor is settable without a rebuild, like the duplicate threshold.
// A malformed value falls back rather than to zero: zero is no floor at all,
// which is the bug this constant exists to fix.
func denseFloor() float64 {
	if v := os.Getenv("BRABEUS_DENSE_FLOOR"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		log.Printf("BRABEUS_DENSE_FLOOR=%q is not a number; using %v", v, defaultDenseFloor)
	}
	return defaultDenseFloor
}

// buildDense embeds every memory's two lanes.
//
// It returns an index or an error, never a partial one. A half-built index
// ranks a subset of the corpus while looking complete, so a memory that exists
// simply never comes back and nothing anywhere says why.
func buildDense(ctx context.Context, e Embedder, docs []indexedDoc) (*denseIndex, error) {
	texts := make([]string, 0, len(docs)*2)
	for _, d := range docs {
		desc, body := laneTexts(d)
		texts = append(texts, desc, body)
	}

	vecs := make([][]float32, 0, len(texts))
	for i := 0; i < len(texts); i += embedBatch {
		batch, err := e.Embed(ctx, texts[i:min(i+embedBatch, len(texts))])
		if err != nil {
			return nil, err
		}
		vecs = append(vecs, batch...)
	}
	if len(vecs) != len(texts) {
		return nil, errShortEmbed{want: len(texts), got: len(vecs)}
	}

	ix := &denseIndex{
		desc: make([][]float32, len(docs)),
		body: make([][]float32, len(docs)),
	}
	// The lanes were appended in pairs, so document i owns 2i and 2i+1.
	for i := range docs {
		ix.desc[i] = vecs[2*i]
		ix.body[i] = vecs[2*i+1]
	}
	return ix, nil
}

type errShortEmbed struct{ want, got int }

func (e errShortEmbed) Error() string {
	return fmt.Sprintf("dense: embedded %d lanes, wanted %d", e.got, e.want)
}

func (d *denseIndex) rankDesc(q []float32, docs []indexedDoc, keep func(*indexedDoc) bool) []scored {
	return d.rankLane(d.desc, q, docs, keep)
}

func (d *denseIndex) rankBody(q []float32, docs []indexedDoc, keep func(*indexedDoc) bool) []scored {
	return d.rankLane(d.body, q, docs, keep)
}

// rankLane scores one lane against the query, best first.
//
// keep is applied BEFORE scoring, exactly as bm25Index.rank does it, so a
// caller's limit is spent on documents they may actually see.
func (d *denseIndex) rankLane(lane [][]float32, q []float32, docs []indexedDoc, keep func(*indexedDoc) bool) []scored {
	out := make([]scored, 0, len(docs))
	for i := range docs {
		if i >= len(lane) {
			break // the alignment contract failed; rank what is trustworthy
		}
		doc := &docs[i]
		if keep != nil && !keep(doc) {
			continue
		}
		out = append(out, scored{doc, dot(q, lane[i])})
	}
	// Path breaks ties, or equal scores come back in slice order and the
	// same query answers differently between runs.
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].doc.path < out[j].doc.path
	})
	return out
}

// dot is cosine similarity, because both vectors arrive unit-normalised from
// the embedder — see normalise in embed.go. If that ever stops being true
// this silently becomes an unnormalised dot product, which ranks long
// documents higher for no reason.
func dot(a, b []float32) float64 {
	n := min(len(a), len(b))
	var s float64
	for i := 0; i < n; i++ {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

// phraseFilter turns quoted phrases into a keep predicate, so BOTH legs
// enforce them through one definition.
//
// If only the lexical leg honoured a quoted phrase, the dense leg would
// rank a different view of the corpus: "grub.cfg" would come back beside
// memories that never mention it, which is exactly the guarantee quoting
// exists to give. Matched against content AND path for the same reason
// bm25Index.rank is — a filename can BE the identifier and appear nowhere in
// the body.
func phraseFilter(phrases []string, next func(*indexedDoc) bool) func(*indexedDoc) bool {
	if len(phrases) == 0 {
		return next
	}
	return func(d *indexedDoc) bool {
		if next != nil && !next(d) {
			return false
		}
		lower := strings.ToLower(d.content)
		path := strings.ToLower(d.path)
		for _, p := range phrases {
			if !strings.Contains(lower, p) && !strings.Contains(path, p) {
				return false
			}
		}
		return true
	}
}

// fuseRRF merges ranked lists by reciprocal rank fusion.
//
// Each list contributes 1/(k+rank) to every document it returned, and the
// sums give the final order. ONLY POSITIONS COUNT, never the raw scores —
// which is the entire point, because a BM25 score and a cosine similarity are
// not comparable quantities and any attempt to weigh them directly is a
// judgement about units that do not exist.
//
// k=60 is the convention and it is load-bearing. Graphiti ships k=1, which
// makes 1/(1+1)=0.5 against 1/(1+2)=0.33 — a gap wide enough that whichever
// list ranked something first simply wins, so fusion stops being fusion. At 60
// the gap between adjacent ranks is small enough that agreement across legs
// outweighs one leg's enthusiasm, which is what a short overlapping candidate
// list needs.
// rrfLeg is one ranked list and how much its opinion counts.
type rrfLeg struct {
	items  []scored
	weight float64
}

// fuseRRF fuses lists that all count equally.
func fuseRRF(k float64, lists ...[]scored) []scored {
	legs := make([]rrfLeg, len(lists))
	for i, l := range lists {
		legs[i] = rrfLeg{items: l, weight: 1}
	}
	return fuseRRFWeighted(k, legs...)
}

// fuseRRFWeighted is the same, with a say per leg.
//
// Why weights exist at all: with two dense lanes over the same corpus, a
// document ranked 1st by one lane and 2nd by the other ties EXACTLY with the
// document that ranked the other way round, and the winner is then decided by
// the path tiebreak — alphabetical order, which is no answer at all. Measured
// on the fixture: "be brief with me" put terse-answers 1st by description and
// short-commits 1st by body, fused to 0.032522 apiece, and the wrong one won
// because "s" sorts before "t".
//
// ⇒ The description lane is weighted above the body, for the reason bm25.go
// already weights that field x2: the description is written BY HAND as a
// retrieval hook, and the body is whatever the memory happens to say. That is
// a statement about the corpus, not a tuning knob — do not adjust it to move a
// single query.
func fuseRRFWeighted(k float64, legs ...rrfLeg) []scored {
	type acc struct {
		doc   *indexedDoc
		score float64
	}
	// Keyed by path rather than by pointer: the legs are handed the same
	// []indexedDoc today, but keying on identity would make this quietly wrong
	// the first time a caller ranks over a copy.
	seen := map[string]*acc{}
	for _, leg := range legs {
		for i, s := range leg.items {
			a, ok := seen[s.doc.path]
			if !ok {
				a = &acc{doc: s.doc}
				seen[s.doc.path] = a
			}
			a.score += leg.weight / (k + float64(i+1))
		}
	}

	out := make([]scored, 0, len(seen))
	for _, a := range seen {
		out = append(out, scored{a.doc, a.score})
	}
	// Path breaks ties. Two documents each ranked first by one leg have
	// identical fused scores, and without this the answer would come back in
	// map iteration order — different on every single run.
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].doc.path < out[j].doc.path
	})
	return out
}

// rankFused is the ranking the server actually serves: one definition, used by
// Store.Search and by the quality gate.
//
// The gate MUST go through this rather than reimplementing the fusion, or it
// would faithfully test a copy of the logic while the real path drifted — and
// ranking has no compiler to catch that.
//
// A nil dense index or a nil query vector means the leg is unavailable, and the
// lexical ranking is returned unchanged.
func rankFused(ix *bm25Index, dense *denseIndex, qvec []float32, query string, keep func(*indexedDoc) bool) []scored {
	lexical := ix.rank(query, keep)
	if dense == nil || qvec == nil {
		return lexical
	}
	// A quoted phrase binds BOTH legs, or the dense one ranks a different view
	// of the corpus than the lexical one filtered.
	dkeep := phraseFilter(quotedPhrases(query), keep)
	descLane := dense.rankDesc(qvec, ix.docs, dkeep)
	bodyLane := dense.rankBody(qvec, ix.docs, dkeep)

	fused := fuseRRFWeighted(rrfK,
		rrfLeg{lexical, 1},
		rrfLeg{descLane, descLaneWeight},
		rrfLeg{bodyLane, 1},
	)
	// The lexical cutoff is skipped when the query carried a quoted phrase:
	// that is the caller being explicit, every survivor already satisfied it,
	// and second-guessing them with a score rule would break the one guarantee
	// quoting exists to give.
	cut := lexicalCutoff()
	if len(quotedPhrases(query)) > 0 {
		cut = 0
	}
	return keepRelevant(fused, denseFloor(), cut, lexical, descLane, bodyLane)
}

// keepRelevant decides which documents belong in the ANSWER, after the fusion
// has decided their order.
//
// The floor must be applied HERE and not inside the lanes, and the
// difference is not cosmetic. Filtering a lane removes a document's
// contribution to the fusion, which reorders everything else: dropping
// terse-answers from the body lane at 0.27 let short-commits overtake it for
// "keep answers short" — a golden query that had passed under lexical-only
// ranking. Ranking sees complete lists; the floor only decides membership.
//
// A document survives if the LEXICAL leg matched it, or if any dense lane
// scored it at or above the floor. So the answer stays a superset of what the
// lexical leg alone would have returned, and the floor can only ever remove a
// dense-only addition.
func keepRelevant(fused []scored, floor, lexCutoff float64, lexical []scored, lanes ...[]scored) []scored {
	ok := make(map[string]bool, len(fused))
	// A document used to qualify on ANY term overlap, because bm25's test
	// was score > 0. Ask an eleven-word question and every memory containing
	// "file" or "config" came back — measured against the real store, ~45% of
	// the corpus per query, which the caller's limit then padded the answer out
	// with. That is not merely wasteful: search results are material a
	// session REASONS FROM, and fifty half-relevant memories are worse than
	// five good ones.
	//
	// Relative to the best hit, because BM25 scores have no fixed scale —
	// they move with query length, corpus size and IDF, so any absolute number
	// would need re-tuning as the store grows. This asks the only question that
	// travels: "is this much worse than the best thing I found?"
	//
	// And applied HERE, not inside bm25's rank, for the same reason the
	// dense floor is. Measured 2026-09-09: trimming the lexical list itself
	// took a correct answer from rank 3 to rank 10+, because a document that
	// scores low lexically but is found by meaning loses its lexical vote and
	// sinks. Ranking sees complete lists; only membership is filtered.
	var lexMin float64
	if len(lexical) > 0 {
		lexMin = lexical[0].score * lexCutoff
	}
	for _, s := range lexical {
		if s.score >= lexMin {
			ok[s.doc.path] = true
		}
	}
	for _, lane := range lanes {
		for _, s := range lane {
			if s.score >= floor {
				ok[s.doc.path] = true
			}
		}
	}
	out := make([]scored, 0, len(fused))
	for _, s := range fused {
		if ok[s.doc.path] {
			out = append(out, s)
		}
	}
	return out
}
