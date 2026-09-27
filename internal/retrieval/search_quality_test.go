package retrieval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Ranking has no compiler. A change to the tokenizer or the field weights
// can improve one query and quietly ruin five, and nothing else in this suite
// would notice. This is the gate: a fixed corpus, a fixed set of questions,
// and the answer each one must give.
//
// The fixture is deliberately NOT the real store, which is private and lives
// in another repository. These numbers pin behaviour; they do not reproduce
// the measurements in the lexical calibration.
func loadFixtureIndex(t *testing.T) *BM25 {
	t.Helper()
	return NewBM25(fixtureCorpus(t))
}

// fixtureCorpus is the one walk of the fixture memories. The fixture
// embedder's generator reads it too, so the vectors checked into testdata
// cannot come to describe a different set of files than the ranking tests
// score — the drift that would make a stale fixture look like a ranking bug.
func fixtureCorpus(t *testing.T) []Doc {
	t.Helper()
	root := filepath.Join("testdata", "corpus")
	var docs []Doc
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		rel, _ := filepath.Rel(root, p)
		docs = append(docs, IndexDoc(filepath.ToSlash(rel), string(b)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 16 {
		t.Fatalf("fixture holds %d memories, want 16", len(docs))
	}
	return docs
}

// fixtureRanker builds the fused ranking over the fixture corpus, using the
// precomputed vectors. It goes through rankFused — the same function
// Store.Search calls — so the gate cannot pass while the served path drifts.
func fixtureRanker(t *testing.T) func(query string) []Scored {
	t.Helper()
	docs := fixtureCorpus(t)
	ix := NewBM25(docs)
	e := loadFixtureEmbedder(t)
	dense, err := BuildDense(t.Context(), e, docs)
	if err != nil {
		t.Fatal(err)
	}
	return func(query string) []Scored {
		vecs, err := e.Embed(t.Context(), []string{QueryText(query)})
		if err != nil {
			t.Fatalf("embedding %q: %v", query, err)
		}
		return RankFused(ix, dense, vecs[0], query, nil)
	}
}

// queryCase is one question and the memory it must return.
type queryCase struct{ query, want string }

// Package-level so the fixture generator can embed exactly these strings.
// A query added here without regenerating testdata/vectors.json fails loudly
// on a missing vector rather than quietly scoring zero.
var goldenQueries = []queryCase{
	{"printer prints everything twice", "infra/printer-duplicate-queue.md"},
	{"DNS stopped working because of a leftover file", "infra/dnsmasq-conf-dir.md"},
	{"locked out of ssh by the ban tool", "infra/fail2ban-ports.md"},
	{"laptop resets by itself", "infra/laptop-reset-fault.md"},
	{"the plugin update did nothing", "infra/plugin-cache-version.md"},
	{"keep answers short", "personal/terse-answers.md"},
	{"do not guess numbers", "personal/never-guess-numbers.md"},
	{"secure boot", "personal/secure-boot.md"},
	{"how does a new build reach the server", "infra/deploy-path.md"},
	{"where are the business books", "projects/ledger-location.md"},
	{"the printer prints every job twice", "infra/printer-duplicate-queue.md"},
	{"how do I update the printer firmware", "infra/printer-firmware.md"},
	{"which DNS resolver do we use", "infra/dns-upstream.md"},
	{"when should I replace the laptop battery", "infra/laptop-battery.md"},
	{"how short should a commit message be", "personal/short-commits.md"},
}

// The two needs above in different words. These were PINNED AS KNOWN MISSES
// until 2026-09-09, because lexical ranking cannot reach a paraphrase — no
// stemmer folds "brief" into "terse". The dense leg closed them, and their
// moving into the passing set is the evidence the dense leg worked.
//
// If either regresses, the dense leg is down or the fusion is wrong. It is
// NOT a tokenizer problem, and nothing in bm25.go will fix it.
var paraphraseQueries = []queryCase{
	{"be brief with me", "personal/terse-answers.md"},
	{"the notebook shuts down unexpectedly", "infra/laptop-reset-fault.md"},
}

// This gate has blind spots. Against four plausible regressions:
// - Dropped suffix folding      2 of 8 queries fail
// - Dropped length normalisation 1 of 8 queries fail
// - Scored by raw term count     1 of 8 queries fail
// - Dropped IDF entirely         0 of 8 queries fail
// The set detects a dropped-IDF regression not at all, and the others only
// partially. A gate that overstates its coverage is worse than one that
// admits its limits, because the next person trusts it further than it
// deserves. Do not tune to improve those numbers — report, do not chase.
func TestRankingAnswersTheGoldenQueries(t *testing.T) {
	rank := fixtureRanker(t)
	for _, c := range append(append([]queryCase{}, goldenQueries...), paraphraseQueries...) {
		got := rank(c.query)
		if len(got) == 0 {
			t.Errorf("%q returned nothing, want %s", c.query, c.want)
			continue
		}
		if got[0].Doc.Path != c.want {
			t.Errorf("%q ranked %s first, want %s", c.query, got[0].Doc.Path, c.want)
		}
	}
}
