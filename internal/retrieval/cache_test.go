package retrieval

import (
	"context"
	"testing"
)

// The whole point: adding one memory must not re-embed the other 137.
func TestCacheOnlySendsTextItHasNotSeen(t *testing.T) {
	inner := &stubEmbedder{fn: func(string) []float32 { return []float32{1, 0} }}
	c := NewCachingEmbedder(inner)

	if _, err := c.Embed(t.Context(), []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Embed(t.Context(), []string{"a", "b", "c"}); err != nil {
		t.Fatal(err)
	}
	inner.mu.Lock()
	seen := append([]string{}, inner.seen...)
	inner.mu.Unlock()
	if len(seen) != 3 {
		t.Errorf("the model saw %v; want a, b once each and c once", seen)
	}
}

// Results must still line up with the inputs when only some were cached —
// a misordered merge would hand each memory its neighbour's vector.
func TestCacheReturnsVectorsInInputOrder(t *testing.T) {
	inner := &stubEmbedder{fn: func(text string) []float32 {
		return []float32{float32(len(text)), 0}
	}}
	c := NewCachingEmbedder(inner)
	if _, err := c.Embed(t.Context(), []string{"bb"}); err != nil {
		t.Fatal(err)
	}
	got, err := c.Embed(t.Context(), []string{"a", "bb", "ccc"})
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []float32{1, 2, 3} {
		if got[i][0] != want {
			t.Errorf("vector %d is %v, want first element %v — the merge is misordered", i, got[i], want)
		}
	}
}

// Bounded: text no pass asks for any more must fall out, or the cache grows
// with every memory ever edited or deleted for the life of the process.
func TestCacheDropsTextNoLongerUsed(t *testing.T) {
	inner := &stubEmbedder{fn: func(string) []float32 { return []float32{1, 0} }}
	c := NewCachingEmbedder(inner)

	c.BeginPass()
	if _, err := c.Embed(t.Context(), []string{"gone"}); err != nil {
		t.Fatal(err)
	}
	// Two rotations without asking for it: once into prev, once out entirely.
	c.BeginPass()
	c.BeginPass()

	c.mu.Lock()
	_, inCur := c.cur[cacheKey("gone")]
	_, inPrev := c.prev[cacheKey("gone")]
	c.mu.Unlock()
	if inCur || inPrev {
		t.Error("text unused for a whole pass is still cached")
	}
}

// A hit during a pass must survive the next rotation, or the cache would throw
// away the entire corpus every 15 minutes and defeat its own purpose.
func TestCacheKeepsTextThatIsStillInUse(t *testing.T) {
	inner := &stubEmbedder{fn: func(string) []float32 { return []float32{1, 0} }}
	c := NewCachingEmbedder(inner)

	for range 3 {
		c.BeginPass()
		if _, err := c.Embed(context.Background(), []string{"kept"}); err != nil {
			t.Fatal(err)
		}
	}
	inner.mu.Lock()
	n := len(inner.seen)
	inner.mu.Unlock()
	if n != 1 {
		t.Errorf("the model saw %q %d times across three passes; want 1", "kept", n)
	}
}
