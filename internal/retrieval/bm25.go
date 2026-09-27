package retrieval

import (
	"bufio"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

// stopWords are dropped on both sides of a search. They appear in nearly every
// memory, so they cost index size and add nothing to ranking.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "it": true, "of": true, "to": true,
	"and": true, "in": true, "on": true, "for": true, "that": true, "this": true,
	"with": true, "as": true, "by": true, "at": true, "or": true, "be": true,
	"are": true, "was": true, "from": true, "not": true, "i": true, "my": true,
	"do": true, "does": true, "how": true, "what": true, "when": true, "why": true,
	"where": true, "who": true, "can": true, "if": true, "so": true,
}

// tokenize splits text into the terms the index is built from.
//
// The suffix folding is deliberately crude, not a stemmer: it collapses the
// plurals and gerunds that actually differ between how a memory is written and
// how it is asked for. It does not and cannot reach synonyms — "brief" never
// folds to "terse", which is the gap the dense leg exists to close.
//
// The rune predicate is deliberately ASCII-only (a-z, 0-9); widening it would
// invalidate the field weights measured against the golden set. Non-English
// text tokenizes poorly here — quote it instead: quotedPhrases lowercases with
// strings.ToLower and matches with strings.Contains, both of which handle
// UTF-8.
func tokenize(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if len(f) < 2 || stopWords[f] {
			continue
		}
		for _, suffix := range []string{"ing", "ers", "es", "s"} {
			if len(f) > len(suffix)+3 && strings.HasSuffix(f, suffix) {
				f = f[:len(f)-len(suffix)]
				break
			}
		}
		out = append(out, f)
	}
	return out
}

// indexedDoc is one memory in indexed form. The content is kept so a result
// can carry the line it matched on without re-reading the file.
type Doc struct {
	Path, Scope string
	// Module and Kind are empty on a file the one-time migration has not yet
	// retagged; LegacyType then carries its pre-module `type`.
	Module, Kind, LegacyType string
	// desc is kept because it is a dense LANE of its own, not merely a
	// weighted field: it is the one line a memory is written to be found by.
	// Re-parsing the frontmatter downstream would be a second definition of
	// what the description is, free to disagree with this one.
	Desc    string
	Content string
	terms   map[string]float64 // term -> field-weighted frequency
	length  float64
}

// bodyOf returns everything after the frontmatter, so the body is weighted
// once rather than twice through the fields parsed out of it.
//
// Deliberately stricter than parseFrontmatter: it requires the literal
// prefix "---\n" rather than a trimmed-equal check. That strictness is the
// safe direction — a missed frontmatter over-weights the body by including
// the fields again, never discards a real body.
func bodyOf(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	if end := strings.Index(content[4:], "\n---"); end >= 0 {
		return content[4+end+4:]
	}
	return content
}

// unquote strips one layer of double quotes, as written by yamlValue in the
// store package. Kept alongside ParseFrontmatter, its only caller.
func unquote(s string) string {
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		return strings.ReplaceAll(s[1:len(s)-1], `\"`, `"`)
	}
	return s
}

// ParseFrontmatter reads the leading --- block and nothing else. A file whose
// first line is not a divider has no frontmatter, however many dividers appear
// later: the body is markdown and markdown uses them as rules.
//
// Exported because both retrieval (IndexDoc) and store (staleStamp and its
// scope/`updated` reads) need it; store's own parseFrontmatter delegates here
// rather than keeping a second definition.
func ParseFrontmatter(content string) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(content))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return out
	}
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || strings.HasPrefix(key, "-") || strings.HasPrefix(key, "#") {
			continue
		}
		out[key] = unquote(strings.TrimSpace(value))
	}
	return out
}

// indexDoc applies the field weights measured in the lexical calibration: a
// memory is found by what it is CALLED far more often than by a word buried in
// its body, and the description is a hook written for exactly this purpose.
func IndexDoc(rel, content string) Doc {
	fm := ParseFrontmatter(content)
	d := Doc{
		Path: rel, Scope: fm["scope"], Module: fm["module"], Kind: fm["kind"], LegacyType: fm["type"],
		Desc: fm["description"], Content: content, terms: map[string]float64{},
	}
	for _, f := range []struct {
		text   string
		weight float64
	}{
		{strings.ReplaceAll(rel, "/", " "), 3},
		{fm["name"], 3},
		{fm["description"], 2},
		{bodyOf(content), 1},
	} {
		for _, t := range tokenize(f.text) {
			d.terms[t] += f.weight
			d.length += f.weight
		}
	}
	return d
}

// bm25Index is the whole searchable corpus. It is derived state held only in
// memory: the markdown is the record, and this is rebuilt from it.
type BM25 struct {
	Docs []Doc
	df   map[string]float64 // term -> how many documents contain it
	avg  float64            // mean weighted length, for length normalisation
}

func NewBM25(docs []Doc) *BM25 {
	ix := &BM25{Docs: docs, df: map[string]float64{}}
	for i := range docs {
		ix.avg += docs[i].length
		for t := range docs[i].terms {
			ix.df[t]++
		}
	}
	if len(docs) > 0 {
		ix.avg /= float64(len(docs))
	}
	return ix
}

// snippet picks the line to show for a hit: the one carrying the most distinct
// query terms, earliest wins. It searches the BODY only, using bodyOf's own
// boundary so this cannot disagree with indexDoc about where the body starts.
//
// Scanning the whole file (frontmatter included) let "name: x" or
// "description: y" win the tie against a real body line, since name and
// description are weighted highest and happen to sit on the file's earliest
// lines — the exact bug this shape was rewritten to fix.
func Snippet(content, query string) (int, string) {
	body := bodyOf(content)
	// The number of newlines in what bodyOf trimmed off is the number of file
	// lines that come before the body's own line 1.
	offset := strings.Count(content[:len(content)-len(body)], "\n")
	lines := strings.Split(body, "\n")

	terms := tokenize(query)
	best, bestLine, bestText := 0, 0, ""
	for i, line := range lines {
		seen := map[string]bool{}
		for _, t := range tokenize(line) {
			for _, q := range terms {
				if t == q {
					seen[q] = true
				}
			}
		}
		if len(seen) > best {
			best, bestLine, bestText = len(seen), offset+i+1, strings.TrimSpace(line)
		}
	}
	if best > 0 {
		return bestLine, bestText
	}
	// A memory can rank on its path or name alone, so falling back to the
	// first non-empty body line matters — a hit with no line looks like a bug
	// to the caller. Frontmatter is already excluded structurally above, so
	// this no longer has to dodge it by skipping any line containing ":" —
	// that check was only ever a proxy for "not frontmatter", and it wrongly
	// rejected real body lines like "Runtime: 48 minutes."
	//
	// "---" is still skipped, but for an unrelated reason: inside a BODY
	// it's a markdown thematic break, not a frontmatter marker, and a memory
	// whose body opens with one would otherwise report a bare rule as its
	// matched line — the same uninformative-snippet problem this function
	// exists to avoid, just arriving through a different line.
	for i, line := range lines {
		if t := strings.TrimSpace(line); t != "" && t != "---" {
			return offset + i + 1, t
		}
	}
	// The floor: Write requires a non-empty description, so a memory whose
	// body has nothing usable still has this to fall back to — never a blank
	// hit the caller cannot act on.
	return DescriptionLine(content)
}

// descriptionLine finds the frontmatter's description field and returns its
// real file line number and text, for snippet's floor.
func DescriptionLine(content string) (int, string) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return 1, ""
	}
	for i, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		key, _, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(key) == "description" {
			return i + 2, strings.TrimSpace(line) // +2: skip lines[0], and i+1 is 1-based
		}
	}
	return 1, ""
}

type Scored struct {
	Doc   *Doc
	Score float64
}

// quotedPhrases pulls "…" runs out of a query. They are required literally,
// which is what makes an exact identifier findable: the tokenizer splits
// grub.cfg into two terms, so nothing else can demand it as itself.
func quotedPhrases(query string) []string {
	var out []string
	for i := strings.Index(query, `"`); i >= 0; i = strings.Index(query, `"`) {
		rest := query[i+1:]
		j := strings.Index(rest, `"`)
		if j < 0 {
			break
		}
		if p := strings.TrimSpace(rest[:j]); p != "" {
			out = append(out, strings.ToLower(p))
		}
		query = rest[j+1:]
	}
	return out
}

// rank scores every document the filter keeps, best first.
//
// keep is applied BEFORE scoring rather than to the results, so a caller's
// limit is spent on documents they may actually see.
func (ix *BM25) Rank(query string, keep func(*Doc) bool) []Scored {
	const k1, b = 1.2, 0.75
	n := float64(len(ix.Docs))
	terms := tokenize(query)
	phrases := quotedPhrases(query)

	var out []Scored
	for i := range ix.Docs {
		d := &ix.Docs[i]
		if keep != nil && !keep(d) {
			continue
		}
		// Checked after keep so a document the caller may not see is never
		// tested against the phrase. Matched against content AND path: path is
		// weighted highest at index time, so a phrase check that only read
		// content would narrow the candidates by a different view of the
		// corpus than the one that ranks them, and silently miss a filename
		// that IS the identifier and appears nowhere in the body.
		if len(phrases) > 0 {
			lower := strings.ToLower(d.Content)
			path := strings.ToLower(d.Path)
			missing := false
			for _, p := range phrases {
				if !strings.Contains(lower, p) && !strings.Contains(path, p) {
					missing = true
					break
				}
			}
			if missing {
				continue
			}
		}
		var s float64
		for _, t := range terms {
			f := d.terms[t]
			if f == 0 {
				continue
			}
			idf := math.Log(1 + (n-ix.df[t]+0.5)/(ix.df[t]+0.5))
			s += idf * (f * (k1 + 1)) / (f + k1*(1-b+b*d.length/ix.avg))
		}
		// A phrase short enough or common enough to tokenize to nothing (e.g.
		// "C++", "6.1") scores zero even though it satisfied the literal check
		// above — that check is the only way past `continue`, so reaching here
		// with phrases non-empty already means every phrase matched. The
		// tempting cleanup back to a bare `s > 0` reintroduces exactly the
		// regression a quoted phrase exists to prevent: the substring scan
		// this ranker replaced could find every one of these.
		if s > 0 || len(phrases) > 0 {
			out = append(out, Scored{d, s})
		}
	}

	// Path breaks ties, or equal scores come back in walk order and the same
	// query answers differently between runs.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Doc.Path < out[j].Doc.Path
	})

	return out
}

// defaultLexicalCutoff is the share of the best hit's score a document must
// reach to count as a match at all.
//
// Swept against a 140-record store, 2026-09-09, with 8 fresh-worded
// queries — mean results per query, of 140:
//
//	0.00  62.8    0.20  28.5    0.30  16.9    0.50   7.8
//
// top-1 stayed 6/8 and top-3 stayed 8/8 at EVERY value, so the measurement
// does not choose between them and 0.30 is a judgement about margin: a 73% cut
// with no observed loss, while staying clear of the aggressive end because
// eight queries cannot show what a flatter score profile would do.
//
// Do not read the flat quality as licence to raise it freely — the set is
// small and every query in it has one strong answer. Re-run the sweep instead:
// TestEvaluateAgainstARealCorpus.
const defaultLexicalCutoff = 0.30

// lexicalCutoff is settable without a rebuild.
// A malformed value falls back rather than to zero: zero is no cutoff, which
// is the behaviour this exists to replace.
func lexicalCutoff() float64 {
	if v := os.Getenv("BRABEUS_LEXICAL_CUTOFF"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		log.Printf("BRABEUS_LEXICAL_CUTOFF=%q is not a number; using %v", v, defaultLexicalCutoff)
	}
	return defaultLexicalCutoff
}
