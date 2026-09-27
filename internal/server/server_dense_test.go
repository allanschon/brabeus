package server

import (
	"reflect"
	"strings"
	"testing"

	"github.com/allanschon/brabeus/internal/store"
)

// store.Match.Score was documented as "BM25 relevance". After fusion that is a
// lie the caller acts on: the number is a fused rank score, and the two legs'
// raw scores are not comparable — which is the whole reason RRF exists.
func TestScoreIsNoLongerDocumentedAsBM25(t *testing.T) {
	f, ok := reflect.TypeOf(store.Match{}).FieldByName("Score")
	if !ok {
		t.Fatal("store.Match has no Score field")
	}
	// Deliberately blunt: the tag must not mention BM25 AT ALL, not merely
	// avoid claiming the score is BM25. A description free to explain itself
	// "because a BM25 score and a cosine distance differ" reads as accurate
	// while leaving the caller a half-step from the old wrong conclusion, and
	// the explanation works without naming the algorithm.
	tag := f.Tag.Get("jsonschema")
	if strings.Contains(strings.ToLower(tag), "bm25") {
		t.Errorf("Score is still documented as BM25: %q", tag)
	}
	if !strings.Contains(strings.ToLower(tag), "fused") {
		t.Errorf("Score should say the number is fused across both legs: %q", tag)
	}
}

// The caller has to be able to tell a keyword-only answer from a full one, or
// a degraded search is indistinguishable from a working one.
func TestSearchOutCarriesTheDenseState(t *testing.T) {
	f, ok := reflect.TypeOf(searchOut{}).FieldByName("Dense")
	if !ok {
		t.Fatal("searchOut has no Dense field")
	}
	for _, want := range []string{"on", "building", "unavailable"} {
		if !strings.Contains(f.Tag.Get("jsonschema"), want) {
			t.Errorf("the Dense schema does not mention %q: %q", want, f.Tag.Get("jsonschema"))
		}
	}
}
