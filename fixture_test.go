package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// The fixture embedder serves vectors checked into testdata instead of calling
// a sidecar.
//
// This is what lets the acceptance criterion for the whole stage run in CI.
// ci.sh runs inside golang:1.25 with no sidecar and no network, so without
// precomputed vectors the one test that proves a paraphrase now finds its
// memory could not run at all — and the quality gate would silently stop
// gating the thing the dense leg is about.
type fixtureFile struct {
	Model       string               `json:"model"`
	Generated   string               `json:"generated"`
	DocPrefix   string               `json:"doc_prefix"`
	QueryPrefix string               `json:"query_prefix"`
	Vectors     map[string][]float32 `json:"vectors"`
}

const fixtureVectorsPath = "testdata/vectors.json"

// fixtureKey identifies a vector by the EXACT string that was embedded,
// prefix included. Keying on the bare text instead would make a prefix
// change invisible here while changing every vector's meaning.
func fixtureKey(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

type fixtureEmbedder struct {
	t *testing.T
	f fixtureFile
}

// A miss is loud, never a zero vector. A zero vector scores 0 against
// everything, so a stale fixture would present as "the dense leg ranked it
// last" — a ranking bug that does not exist — rather than as the stale data it
// is. t.Errorf rather than t.Fatalf because Task 5 embeds from a goroutine,
// where only Errorf is legal.
func (e *fixtureEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, s := range texts {
		v, ok := e.f.Vectors[fixtureKey(s)]
		if !ok {
			e.t.Errorf("testdata/vectors.json has no vector for %q\n"+
				"regenerate it: see TestGenerateFixtureVectors", s)
			return nil, fmt.Errorf("fixture: no vector for %q", s)
		}
		out[i] = v
	}
	return out, nil
}

func loadFixtureEmbedder(t *testing.T) *fixtureEmbedder {
	t.Helper()
	b, err := os.ReadFile(fixtureVectorsPath)
	if err != nil {
		t.Fatalf("%v\nregenerate it: see TestGenerateFixtureVectors", err)
	}
	var f fixtureFile
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	// The vectors mean nothing without the prefixes they were generated
	// under. Changing a prefix without regenerating would leave every vector
	// describing text the server no longer sends — a silent quality collapse
	// of exactly the 33%-vs-87% size this design is trying to avoid.
	if f.DocPrefix != docPrefix || f.QueryPrefix != queryPrefix {
		t.Fatalf("testdata/vectors.json was generated under prefixes %q/%q, "+
			"but the server now sends %q/%q — regenerate it",
			f.DocPrefix, f.QueryPrefix, docPrefix, queryPrefix)
	}
	return &fixtureEmbedder{t: t, f: f}
}

// fixtureTexts is every string the fixture must carry a vector for: both lanes
// of every fixture memory, and every query the ranking tests ask.
//
// The generator and this check read the SAME list, so "the fixture is
// complete" and "the fixture is what the tests need" cannot drift apart.
func fixtureTexts(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, d := range fixtureCorpus(t) {
		desc, body := laneTexts(d)
		out = append(out, desc, body)
	}
	for _, c := range append(append([]queryCase{}, goldenQueries...), paraphraseQueries...) {
		out = append(out, queryText(c.query))
	}
	return out
}

func TestFixtureEmbedderCoversEveryFixtureLaneAndQuery(t *testing.T) {
	e := loadFixtureEmbedder(t)
	texts := fixtureTexts(t)
	got, err := e.Embed(t.Context(), texts)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(texts) {
		t.Fatalf("got %d vectors for %d texts", len(got), len(texts))
	}
	for i, v := range got {
		if len(v) == 0 {
			t.Errorf("empty vector for %q", texts[i])
		}
	}
}

// TestGenerateFixtureVectors writes testdata/vectors.json from a live sidecar.
//
// It lives here rather than in a tools/ main package so it can reuse
// fixtureCorpus and the query tables. A generator that kept its own copy of
// those lists is precisely how a fixture comes to be missing the query someone
// just added.
//
// It is SKIPPED unless a sidecar is named, so ci.sh — which has neither a
// sidecar nor a network — never runs it:
//
//	BRABEUS_EMBED_URL=http://127.0.0.1:11500 go test -run TestGenerateFixtureVectors
func TestGenerateFixtureVectors(t *testing.T) {
	base := os.Getenv("BRABEUS_EMBED_URL")
	if base == "" {
		t.Skip("set BRABEUS_EMBED_URL to regenerate testdata/vectors.json")
	}
	model := os.Getenv("BRABEUS_EMBED_MODEL")
	if model == "" {
		model = defaultEmbedModel
	}

	texts := fixtureTexts(t)
	e := NewOllamaEmbedder(base, model)
	f := fixtureFile{
		Model:       model,
		Generated:   os.Getenv("BRABEUS_FIXTURE_DATE"),
		DocPrefix:   docPrefix,
		QueryPrefix: queryPrefix,
		Vectors:     map[string][]float32{},
	}

	// Batched, because one request per text is ~140 round trips.
	const batch = 32
	for i := 0; i < len(texts); i += batch {
		end := min(i+batch, len(texts))
		// A generous deadline: a corpus batch is seconds, not milliseconds,
		// and this is not the read path.
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
		vs, err := e.Embed(ctx, texts[i:end])
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		for j, v := range vs {
			f.Vectors[fixtureKey(texts[i+j])] = round(v)
		}
	}

	b, err := marshalFixture(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(fixtureVectorsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixtureVectorsPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s: %d vectors, model %s", fixtureVectorsPath, len(f.Vectors), model)
}

// marshalFixture writes the file as ONE LINE PER VECTOR, keys sorted.
//
// Two separate churn problems, both of which made the first generated file
// unreviewable at 37,738 lines:
// - json.MarshalIndent puts every one of 768 floats on its own line.
// - Go randomises map iteration order, so an indented map would also
// reshuffle all 49 entries on every regeneration — a whole-file diff that
// says nothing about what actually changed.
//
// Sorted keys and compact arrays give ~55 lines, and a regeneration that only
// changed one memory shows as one changed line.
func marshalFixture(f fixtureFile) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("{\n")
	for _, kv := range [][2]string{
		{"model", f.Model}, {"generated", f.Generated},
		{"doc_prefix", f.DocPrefix}, {"query_prefix", f.QueryPrefix},
	} {
		v, err := json.Marshal(kv[1])
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "  %q: %s,\n", kv[0], v)
	}
	b.WriteString("  \"vectors\": {\n")
	keys := slices.Sorted(maps.Keys(f.Vectors))
	for i, k := range keys {
		v, err := json.Marshal(f.Vectors[k])
		if err != nil {
			return nil, err
		}
		sep := ","
		if i == len(keys)-1 {
			sep = ""
		}
		fmt.Fprintf(&b, "    %q: %s%s\n", k, v, sep)
	}
	b.WriteString("  }\n}\n")
	return b.Bytes(), nil
}

// Rounded, or the file churns on every regeneration and no diff is readable.
// Six decimals is far below what cosine ordering can notice.
func round(v []float32) []float32 {
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(math.Round(float64(x)*1e6) / 1e6)
	}
	return out
}
