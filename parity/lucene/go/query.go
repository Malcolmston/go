package main

import (
	"encoding/json"
	"fmt"

	"github.com/malcolmston/lucene"
)

// queryDesc mirrors the JSON query descriptions in cases/query.json. The same
// descriptions drive the Java runner, so both sides build the same query tree
// through each library's own constructor API.
type queryDesc struct {
	Type         string            `json:"type"`
	Field        string            `json:"field"`
	Term         string            `json:"term"`
	Terms        []string          `json:"terms"`
	Prefix       string            `json:"prefix"`
	Pattern      string            `json:"pattern"`
	MaxEdits     *int              `json:"maxEdits"`
	Lower        string            `json:"lower"`
	Upper        string            `json:"upper"`
	IncludeLower bool              `json:"includeLower"`
	IncludeUpper bool              `json:"includeUpper"`
	Boost        float64           `json:"boost"`
	Score        float64           `json:"score"`
	TieBreaker   float64           `json:"tieBreaker"`
	Query        json.RawMessage   `json:"query"`
	Disjuncts    []json.RawMessage `json:"disjuncts"`
	Clauses      []clauseDesc      `json:"clauses"`
}

type clauseDesc struct {
	Occur string          `json:"occur"`
	Query json.RawMessage `json:"query"`
}

func buildQuery(raw json.RawMessage) (lucene.Query, error) {
	var d queryDesc
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	field := d.Field
	if field == "" {
		field = "text"
	}
	switch d.Type {
	case "matchall":
		return &lucene.MatchAllQuery{}, nil
	case "term":
		return lucene.NewTermQuery(field, d.Term), nil
	case "terms":
		return lucene.NewTermsQuery(field, d.Terms...), nil
	case "phrase":
		return lucene.NewPhraseQuery(field, d.Terms...), nil
	case "prefix":
		return lucene.NewPrefixQuery(field, d.Prefix), nil
	case "wildcard":
		return lucene.NewWildcardQuery(field, d.Pattern), nil
	case "regexp":
		return lucene.NewRegexpQuery(field, d.Pattern)
	case "fuzzy":
		n := 2
		if d.MaxEdits != nil {
			n = *d.MaxEdits
		}
		return lucene.NewFuzzyQuery(field, d.Term, n), nil
	case "range":
		return lucene.NewRangeQuery(field, d.Lower, d.Upper, d.IncludeLower, d.IncludeUpper), nil
	case "boost":
		sub, err := buildQuery(d.Query)
		if err != nil {
			return nil, err
		}
		return lucene.NewBoostQuery(sub, d.Boost), nil
	case "constant":
		sub, err := buildQuery(d.Query)
		if err != nil {
			return nil, err
		}
		return lucene.NewConstantScoreQuery(sub, d.Score), nil
	case "dismax":
		subs := make([]lucene.Query, 0, len(d.Disjuncts))
		for _, r := range d.Disjuncts {
			sub, err := buildQuery(r)
			if err != nil {
				return nil, err
			}
			subs = append(subs, sub)
		}
		return lucene.NewDisjunctionMaxQuery(d.TieBreaker, subs...), nil
	case "bool":
		b := lucene.NewBooleanQuery()
		for _, c := range d.Clauses {
			sub, err := buildQuery(c.Query)
			if err != nil {
				return nil, err
			}
			b.Add(sub, occurOf(c.Occur))
		}
		return b, nil
	}
	return nil, fmt.Errorf("unknown query type: %s", d.Type)
}

func occurOf(s string) lucene.Occur {
	switch s {
	case "must":
		return lucene.Must
	case "must_not":
		return lucene.MustNot
	default:
		return lucene.Should
	}
}
