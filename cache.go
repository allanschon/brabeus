package main

import (
	"context"
	"crypto/sha256"
	"sync"
)

// cachingEmbedder remembers a vector per exact text.
//
// Why it matters: reindex runs on startup, on the fifteen-minute timer and
// after EVERY write and delete, and a full corpus pass is ~30 s of CPU
// (measured 2026-09-08). Without this, adding one memory re-embeds all of them
// and the timer burns half a minute of CPU every quarter hour forever. With it,
// only the memory that actually changed is sent to the model.
//
// This is a CACHE, not storage. It lives in memory, it is keyed by content,
// and a cold start still pays the full pass — deliberately, because that is
// what keeps embeddings derived state with nothing persisted to drift or to be
// rolled back inconsistently with the model that produced it.
type cachingEmbedder struct {
	inner Embedder

	mu sync.Mutex
	// Two generations, rotated per pass. This is what bounds the cache
	// without a sweep: anything not asked for during a whole pass falls out of
	// `prev` on the next rotation and is collected. A plain map would instead
	// grow forever with the lane texts of every memory ever edited or deleted.
	cur, prev map[string][]float32
}

func newCachingEmbedder(inner Embedder) *cachingEmbedder {
	return &cachingEmbedder{inner: inner, cur: map[string][]float32{}, prev: map[string][]float32{}}
}

// BeginPass rotates the generations. Called once at the start of a corpus pass.
func (e *cachingEmbedder) BeginPass() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.prev, e.cur = e.cur, map[string][]float32{}
}

// cacheKey is the content hash, as a map-friendly string. Returned as a
// string rather than [32]byte because Go cannot slice an unaddressable array
// value, and every caller wants a map key.
func cacheKey(text string) string {
	sum := sha256.Sum256([]byte(text))
	return string(sum[:])
}

func (e *cachingEmbedder) lookup(text string) ([]float32, bool) {
	k := cacheKey(text)
	if v, ok := e.cur[k]; ok {
		return v, true
	}
	if v, ok := e.prev[k]; ok {
		e.cur[k] = v // promote, so it survives the next rotation
		return v, true
	}
	return nil, false
}

func (e *cachingEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	var missing []string
	var missingAt []int

	e.mu.Lock()
	for i, t := range texts {
		if v, ok := e.lookup(t); ok {
			out[i] = v
			continue
		}
		missing = append(missing, t)
		missingAt = append(missingAt, i)
	}
	e.mu.Unlock()

	if len(missing) == 0 {
		return out, nil
	}
	// The model call happens OUTSIDE the cache lock. Holding it across the
	// sidecar would serialise every concurrent embed behind the slowest one,
	// which is the same mistake the store lock already avoids.
	vecs, err := e.inner.Embed(ctx, missing)
	if err != nil {
		return nil, err
	}
	if len(vecs) != len(missing) {
		return nil, errShortEmbed{want: len(missing), got: len(vecs)}
	}

	e.mu.Lock()
	for j, v := range vecs {
		e.cur[cacheKey(missing[j])] = v
		out[missingAt[j]] = v
	}
	e.mu.Unlock()
	return out, nil
}

// passBeginner is implemented by an Embedder that keeps per-pass state.
type passBeginner interface{ BeginPass() }
