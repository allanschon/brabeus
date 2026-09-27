package main

import (
	"testing"

	"github.com/allanschon/brabeus/internal/retrieval"
)

// Without this the whole stage is dead code in production: the server would
// start, answer "unavailable" forever, and every test would still pass.
func TestEmbedderIsBuiltFromTheEnvironment(t *testing.T) {
	t.Setenv("BRABEUS_EMBED_URL", "")
	if e := newEmbedder(); e != nil {
		t.Errorf("with no URL the dense leg must be off, got %#v", e)
	}

	t.Setenv("BRABEUS_EMBED_URL", "http://127.0.0.1:11434")
	e := newEmbedder()
	if e == nil {
		t.Fatal("BRABEUS_EMBED_URL is set but no embedder was built")
	}
	// RAW here, not cached: the Store adds the cache around the corpus pass
	// and leaves the read path uncached, so a dead sidecar cannot keep
	// answering `on` from a remembered query.
	inner, ok := e.(*retrieval.OllamaEmbedder)
	if !ok {
		t.Fatalf("want a raw Ollama embedder, got %T", e)
	}
	if inner.Model != retrieval.DefaultEmbedModel {
		t.Errorf("model = %q, want %q", inner.Model, retrieval.DefaultEmbedModel)
	}
}
