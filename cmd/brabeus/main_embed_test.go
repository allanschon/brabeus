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

// The notebook gets the dense leg only when asked, because its cold pass is a
// few minutes (227 s for 467 files, measured 2026-09-28) and a deployment
// that never searches it by meaning should not pay that on every change.
func TestTheNotebookEmbedsOnlyWhenTheSwitchIsSet(t *testing.T) {
	t.Setenv("BRABEUS_EMBED_URL", "http://127.0.0.1:11434")
	t.Setenv("BRABEUS_NOTEBOOK_EMBED", "")
	if e := notebookEmbedder(newEmbedder()); e != nil {
		t.Error("without the switch the notebook stays keyword-only")
	}
	t.Setenv("BRABEUS_NOTEBOOK_EMBED", "1")
	if e := notebookEmbedder(newEmbedder()); e == nil {
		t.Error("with the switch the notebook shares the record's embedder")
	}
}

func TestTheNotebookVariablesAreReadUnderBothNames(t *testing.T) {
	t.Setenv("BRABEUS_MIRROR_REPO", "ssh://old.example/notes.git")
	t.Setenv("BRABEUS_NOTEBOOK_REPO", "")
	if got := envEither("BRABEUS_NOTEBOOK_REPO", "BRABEUS_MIRROR_REPO", ""); got != "ssh://old.example/notes.git" {
		t.Errorf("old name: %q", got)
	}
	t.Setenv("BRABEUS_NOTEBOOK_REPO", "ssh://new.example/notes.git")
	if got := envEither("BRABEUS_NOTEBOOK_REPO", "BRABEUS_MIRROR_REPO", ""); got != "ssh://new.example/notes.git" {
		t.Errorf("new name wins: %q", got)
	}
}
