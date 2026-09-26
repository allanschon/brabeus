package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// stubOllama stands in for the sidecar. The unit tests never reach a real
// one: ci.sh runs inside golang:1.25 with no sidecar and no network, and that
// has to keep being true — it is the whole reason the fixture embedder exists.
func stubOllama(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return stubOllamaFunc(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	})
}

func stubOllamaFunc(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// These two strings are load-bearing, not cosmetic: EmbeddingGemma reports
// 33.3% vs 87.5% hit@1 without and with them. Ollama applies NOTHING — its
// Modelfile is `TEMPLATE {{ .Prompt }}`, checked 2026-09-08 — so the server is
// the only thing that can add them, and the only thing that can double them.
// If a future serving layer starts applying its own template, delete the
// prefix here rather than nesting the two.
func TestPromptPrefixesMatchTheModelCard(t *testing.T) {
	if got, want := documentText("hello"), "title: none | text: hello"; got != want {
		t.Errorf("documentText = %q, want %q", got, want)
	}
	if got, want := queryText("hello"), "task: search result | query: hello"; got != want {
		t.Errorf("queryText = %q, want %q", got, want)
	}
}

// The client must not silently return a short slice: a caller zips embeddings
// back against its inputs by position, so a count mismatch would misattribute
// every vector after the gap rather than fail.
func TestEmbedRejectsACountMismatch(t *testing.T) {
	srv := stubOllama(t, `{"embeddings":[[1,0]]}`)
	if _, err := NewOllamaEmbedder(srv.URL, "m").Embed(t.Context(), []string{"a", "b"}); err == nil {
		t.Fatal("want an error when the sidecar returns fewer embeddings than inputs")
	}
}

// Vectors are normalised on receipt so Task 3's scan is a dot product rather
// than a division per comparison. A caller that skipped this would still get
// plausible-looking scores, just wrong ones — the failure worth pinning.
func TestEmbedNormalisesToUnitLength(t *testing.T) {
	srv := stubOllama(t, `{"embeddings":[[3,4]]}`)
	got, err := NewOllamaEmbedder(srv.URL, "m").Embed(t.Context(), []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []float32{0.6, 0.8}; !closeEnough(got[0], want) {
		t.Errorf("Embed = %v, want %v (unit length)", got[0], want)
	}
}

// A zero vector cannot be normalised, and dividing by its length yields NaN,
// which poisons every comparison it touches and sorts unpredictably. It must
// come back as zeros — scoring 0 against everything — not as NaN.
func TestEmbedLeavesAZeroVectorAlone(t *testing.T) {
	srv := stubOllama(t, `{"embeddings":[[0,0]]}`)
	got, err := NewOllamaEmbedder(srv.URL, "m").Embed(t.Context(), []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range got[0] {
		if math.IsNaN(float64(v)) {
			t.Fatalf("Embed[%d] = NaN, want 0", i)
		}
	}
}

// The request has to be the shape Ollama's /api/embed actually takes, and it
// has to carry the model — a wrong field name would return an error the client
// reports as the sidecar being broken.
func TestEmbedPostsTheDocumentedRequestShape(t *testing.T) {
	var got struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	srv := stubOllamaFunc(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("path = %q, want /api/embed", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		fmt.Fprint(w, `{"embeddings":[[1,0],[0,1]]}`)
	})
	if _, err := NewOllamaEmbedder(srv.URL, "embeddinggemma").Embed(t.Context(), []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if got.Model != "embeddinggemma" {
		t.Errorf("model = %q, want embeddinggemma", got.Model)
	}
	if len(got.Input) != 2 || got.Input[0] != "a" {
		t.Errorf("input = %v, want [a b]", got.Input)
	}
}

// Chosen during calibration: the sidecar gets 2 seconds and then search
// degrades. A client with no deadline would hang the whole search rather
// than degrade it, which is the failure the timeout exists to prevent.
func TestEmbedGivesUpOnASlowSidecar(t *testing.T) {
	// The handler must be releasable by the TEST, not only by the request
	// context. A handler parked on `<-r.Context().Done()` alone never returns:
	// the server does not notice the client hanging up while the handler is
	// neither reading nor writing, so httptest's Close waits on it forever and
	// the whole package times out. Cleanups run LIFO, so registering this one
	// after stubOllamaFunc's is what makes it run BEFORE srv.Close.
	release := make(chan struct{})
	srv := stubOllamaFunc(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	t.Cleanup(func() { close(release) })
	e := NewOllamaEmbedder(srv.URL, "m")
	e.Timeout = 50 * time.Millisecond
	start := time.Now()
	if _, err := e.Embed(t.Context(), []string{"a"}); err == nil {
		t.Fatal("want an error when the sidecar does not answer")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("took %s; the client did not honour its own timeout", elapsed)
	}
}

// An empty batch is a real case — a store with no memories, or a filter that
// matched nothing. It must not become a pointless HTTP round trip.
func TestEmbedSkipsTheRoundTripForNoInput(t *testing.T) {
	srv := stubOllamaFunc(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("the client called the sidecar for an empty batch")
	})
	got, err := NewOllamaEmbedder(srv.URL, "m").Embed(t.Context(), nil)
	if err != nil || len(got) != 0 {
		t.Errorf("Embed(nil) = %v, %v; want no vectors and no error", got, err)
	}
}

func closeEnough(got, want []float32) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if math.Abs(float64(got[i]-want[i])) > 1e-6 {
			return false
		}
	}
	return true
}
