package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

// Embedder turns text into vectors. It is an interface because ci.sh runs
// inside golang:1.25 with no sidecar and no network: production talks to
// Ollama, tests read precomputed vectors out of testdata. Without that seam the
// acceptance criterion for this whole stage — a paraphrase reaching its memory
// — could not run in CI at all.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// EmbeddingGemma's retrieval prompts, and they are load-bearing rather than
// cosmetic: Google reports 33.3% hit@1 without them against 87.5% with them.
//
// Ollama applies NOTHING of its own — its Modelfile for this model is
// `TEMPLATE {{ .Prompt }}`, a passthrough, verified 2026-09-08. So this server
// is the only thing that can add them, and the only thing that can double
// them. Both mistakes score badly and NEITHER fails loudly.
//
// If a serving layer ever starts applying its own template, delete the
// prefix here rather than nesting the two.
const (
	docPrefix   = "title: none | text: "
	queryPrefix = "task: search result | query: "
)

// defaultEmbedModel is the model chosen for the dense leg during calibration.
// It reaches both pinned paraphrases at rank 1 where nomic-embed-text reaches
// one, and it discriminates the known duplicate pair at 0.853 where nomic
// scores nearly everything at 0.94 — which is what makes the write-time check
// possible.
const defaultEmbedModel = "embeddinggemma"

func documentText(s string) string { return docPrefix + s }
func queryText(s string) string    { return queryPrefix + s }

// OllamaEmbedder reaches the sidecar over loopback. The sidecar is never on
// the Tailscale network and never published: every memory's full text passes
// through it, which is the single largest surface this design adds, and it is
// contained entirely by the model running locally.
type OllamaEmbedder struct {
	Base  string
	Model string

	// Timeout is a FLOOR for callers that set no deadline of their own, not a
	// ceiling on every call.
	//
	// The trap it dodges: calibration gave the read path a 2 second budget, but
	// a corpus pass batches 32 lanes at ~117 ms each — ~3.7 s for one batch,
	// measured 2026-09-08. A blanket 2 s cap here would fail every build batch
	// while leaving the read path looking fine, so the caller's own deadline
	// always wins and the build pass sets a generous one.
	Timeout time.Duration

	client *http.Client
}

func NewOllamaEmbedder(base, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		Base:    base,
		Model:   model,
		Timeout: 2 * time.Second,
		client:  &http.Client{},
	}
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

func (e *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	// A store with no memories, or a filter that matched nothing, is a real
	// case and not worth a round trip.
	if len(texts) == 0 {
		return nil, nil
	}
	if _, ok := ctx.Deadline(); !ok && e.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.Timeout)
		defer cancel()
	}

	body, err := json.Marshal(embedRequest{Model: e.Model, Input: texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.Base+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := e.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed: sidecar returned %s", resp.Status)
	}

	var out embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	// A caller zips these back against its inputs BY POSITION, so a short
	// reply would misattribute every vector after the gap — each memory scored
	// against its neighbour's meaning — rather than fail. Refuse it here, where
	// the mismatch is still visible.
	if len(out.Embeddings) != len(texts) {
		return nil, fmt.Errorf("embed: asked for %d embeddings, got %d", len(texts), len(out.Embeddings))
	}
	for _, v := range out.Embeddings {
		normalise(v)
	}
	return out.Embeddings, nil
}

// normalise scales a vector to unit length in place, so Task 3's scan is a dot
// product rather than a division per comparison.
//
// A zero-length vector is left alone. Dividing by its length yields NaN,
// which is not merely wrong — it compares false against everything, so a NaN
// score sorts unpredictably and would make one bad memory reorder the whole
// result set. Zeros score 0 against everything, which is the honest answer.
func normalise(v []float32) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return
	}
	inv := float32(1 / math.Sqrt(sum))
	for i := range v {
		v[i] *= inv
	}
}

// laneTexts returns the two strings a memory is embedded as, prefixes applied.
//
// Two lanes because the description is hand-written as a retrieval hook —
// the store's best free asset, and the reason bm25.go already weights it x2.
// Diluting it into the body wastes it.
//
// A memory with no description falls back to its PATH rather than to the
// empty string. An empty lane embeds to a vector that is meaningless but not
// zero, so it would score arbitrarily against real queries; the path is at
// least what the memory is called. This also keeps both lanes exactly as long
// as the document slice, which is what lets the scan index them by position.
func laneTexts(d indexedDoc) (desc, body string) {
	hook := d.desc
	if hook == "" {
		hook = strings.ReplaceAll(d.path, "/", " ")
	}
	// Trimmed: bodyOf keeps the newline that ended the frontmatter, so an
	// untrimmed body renders as "title: none | text: \n…" — the prefix and the
	// text split across a line break, which is not the format the model card
	// specifies and not what the 87.5% figure was measured on.
	return documentText(hook), documentText(strings.TrimSpace(bodyOf(d.content)))
}
