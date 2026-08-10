package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/malcolmston/lucene"
)

// indexArg is the shape of the single argument object used by every case that
// touches an index.
type indexArg struct {
	Index string          `json:"index"`
	Field string          `json:"field"`
	Term  string          `json:"term"`
	TopN  int             `json:"topN"`
	Query json.RawMessage `json:"query"`
}

type analyzeArg struct {
	Analyzer string `json:"analyzer"`
	Text     string `json:"text"`
}

func dispatch(c caseIn) (any, error) {
	switch c.Fn {
	case "analyze", "analyzePositions":
		var a analyzeArg
		if err := json.Unmarshal(c.Args[0], &a); err != nil {
			return nil, err
		}
		toks := analyzerOf(a.Analyzer).Analyze(a.Text)
		out := make([]string, 0, len(toks))
		for _, t := range toks {
			if c.Fn == "analyze" {
				out = append(out, t.Text)
			} else {
				out = append(out, fmt.Sprintf("%d:%s", t.Position, t.Text))
			}
		}
		return out, nil

	case "defaultStopWords":
		return sortedCopy(lucene.DefaultStopWords()), nil

	case "ngrams":
		var s string
		var n int
		if err := args2(c, &s, &n); err != nil {
			return nil, err
		}
		return nonNil(lucene.NGrams(s, n)), nil

	case "edgeNgrams":
		var s string
		var lo, hi int
		if err := args3(c, &s, &lo, &hi); err != nil {
			return nil, err
		}
		return nonNil(lucene.EdgeNGrams(s, lo, hi)), nil

	case "shingles":
		var toks []string
		var n int
		if err := args2(c, &toks, &n); err != nil {
			return nil, err
		}
		return nonNil(lucene.Shingles(toks, n)), nil

	case "wildcardMatch":
		var pattern, s string
		if err := args2(c, &pattern, &s); err != nil {
			return nil, err
		}
		return lucene.WildcardMatch(pattern, s), nil

	case "withinEdits":
		var a, b string
		var n int
		if err := args3(c, &a, &b, &n); err != nil {
			return nil, err
		}
		return lucene.LevenshteinDistance(a, b) <= n, nil
	}

	var a indexArg
	if len(c.Args) == 0 {
		return nil, errors.New("missing argument object")
	}
	if err := json.Unmarshal(c.Args[0], &a); err != nil {
		return nil, err
	}
	return indexed(c.Fn, a)
}

func indexed(fn string, a indexArg) (any, error) {
	idx := indexOf(a.Index)
	field := a.Field
	if field == "" && (fn == "terms" || fn == "termCount" || fn == "docFreq" || fn == "totalTermFreq") {
		field = "text"
	}
	switch fn {
	case "numDocs":
		return idx.NumDocs(), nil
	case "fields":
		return sortedCopy(idx.Fields()), nil
	case "terms":
		return sortedCopy(idx.Terms(field)), nil
	case "termCount":
		return idx.TermCount(field), nil
	case "docFreq":
		return idx.DocFreq(field, a.Term), nil
	case "totalTermFreq":
		return idx.TotalTermFreq(field, a.Term), nil
	case "search", "searchSet":
		q, err := buildQuery(a.Query)
		if err != nil {
			return nil, err
		}
		return hits(idx.Search(q, a.TopN), fn == "search", a.TopN), nil
	case "parse", "parseRanked":
		var qs string
		if err := json.Unmarshal(a.Query, &qs); err != nil {
			return nil, err
		}
		q, err := lucene.NewParser(idx.Analyzer(), "text").Parse(qs)
		if err != nil {
			return nil, err
		}
		return hits(idx.Search(q, a.TopN), fn == "parseRanked", a.TopN), nil
	}
	return nil, fmt.Errorf("unknown fn: %s", fn)
}

// hits reduces a Result to the only things that are comparable across
// independent implementations: the match count and the doc ids. Absolute BM25
// scores are deliberately never emitted. For a ranked comparison, ids that
// scored equal are sorted so an arbitrary intra-tie ordering cannot look like a
// divergence; for a set comparison every id is sorted.
func hits(r lucene.Result, ranked bool, topN int) map[string]any {
	ids := make([]string, 0, len(r.Hits))
	if !ranked {
		for _, h := range r.Hits {
			ids = append(ids, h.ID)
		}
		sort.Strings(ids)
	} else {
		var tie []string
		last := 0.0
		for i, h := range r.Hits {
			s := round6(h.Score)
			if i > 0 && s != last {
				sort.Strings(tie)
				ids = append(ids, tie...)
				tie = tie[:0]
			}
			last = s
			tie = append(tie, h.ID)
		}
		sort.Strings(tie)
		ids = append(ids, tie...)
	}
	if topN > 0 && len(ids) > topN {
		ids = ids[:topN]
	}
	return map[string]any{"total": r.Total, "ids": ids}
}

func round6(f float64) float64 {
	return float64(int64(f*1e6+0.5)) / 1e6
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func args2(c caseIn, a, b any) error {
	if len(c.Args) < 2 {
		return errors.New("expected two arguments")
	}
	if err := json.Unmarshal(c.Args[0], a); err != nil {
		return err
	}
	return json.Unmarshal(c.Args[1], b)
}

func args3(c caseIn, a, b, d any) error {
	if err := args2(c, a, b); err != nil {
		return err
	}
	if len(c.Args) < 3 {
		return errors.New("expected three arguments")
	}
	return json.Unmarshal(c.Args[2], d)
}
