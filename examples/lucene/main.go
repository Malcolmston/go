// Command lucene-example exercises github.com/malcolmston/lucene: it builds an
// in-memory inverted index over a small document corpus and then runs term,
// phrase, boolean, prefix/wildcard, regexp, fuzzy and range queries against it,
// inspecting scoring order, analyzer behaviour, statistics, facets,
// highlighting, suggestions and MoreLikeThis.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/malcolmston/lucene"
)

// corpus is the document set indexed by every section below. Field "year" is
// stored as text so that lexical range queries can be demonstrated over it.
var corpus = []lucene.Document{
	{ID: "d1", Fields: map[string]string{
		"title": "The Go Programming Language",
		"body":  "Go is an open source programming language designed at Google for building simple, reliable and efficient software.",
		"lang":  "go",
		"year":  "2015",
	}},
	{ID: "d2", Fields: map[string]string{
		"title": "Network Programming with Go",
		"body":  "Building network servers and clients using the Go programming language, with TCP, UDP and HTTP examples.",
		"lang":  "go",
		"year":  "2018",
	}},
	{ID: "d3", Fields: map[string]string{
		"title": "Programming Rust",
		"body":  "Rust is a systems programming language focused on safety and speed without a garbage collector.",
		"lang":  "rust",
		"year":  "2021",
	}},
	{ID: "d4", Fields: map[string]string{
		"title": "Effective Python",
		"body":  "Python is a dynamic programming language; this book collects idiomatic recipes for writing better Python.",
		"lang":  "python",
		"year":  "2019",
	}},
	{ID: "d5", Fields: map[string]string{
		"title": "Distributed Systems in Go",
		"body":  "Designing distributed systems: consensus, replication and network partitions, with worked Go implementations.",
		"lang":  "go",
		"year":  "2023",
	}},
	{ID: "d6", Fields: map[string]string{
		"title": "The Rust Networking Cookbook",
		"body":  "Networking recipes in Rust: async servers, clients and protocol parsers built on top of the standard library.",
		"lang":  "rust",
		"year":  "2022",
	}},
}

func section(name string) {
	fmt.Printf("\n=== %s ===\n", name)
}

func show(label string, q lucene.Query, res lucene.Result) {
	fmt.Printf("%-28s query=%-38s total=%d\n", label, q.String(), res.Total)
	if len(res.Hits) == 0 {
		fmt.Println("   (no hits)")
		return
	}
	for i, h := range res.Hits {
		fmt.Printf("   %d. %-4s score=%.4f  %s\n", i+1, h.ID, h.Score, titleOf(h.ID))
	}
}

func titleOf(id string) string {
	for _, d := range corpus {
		if d.ID == id {
			return d.Fields["title"]
		}
	}
	return "?"
}

func main() {
	// ---------------------------------------------------------------
	section("1. Analyzers")
	// ---------------------------------------------------------------
	std := lucene.NewStandardAnalyzer()
	raw := "The Runners were Running QUICKLY through 12 Networks!"
	fmt.Printf("standard analyzer  %q\n", raw)
	for _, t := range std.Analyze(raw) {
		fmt.Printf("   pos=%d %q\n", t.Position, t.Text)
	}

	noStem := lucene.NewAnalyzer(lucene.WithStemming(false), lucene.WithStopWords(nil))
	fmt.Print("no stemming, no stop words: ")
	var toks []string
	for _, t := range noStem.Analyze(raw) {
		toks = append(toks, t.Text)
	}
	fmt.Println(strings.Join(toks, " "))

	custom := lucene.NewAnalyzer(lucene.WithStopWords([]string{"through", "the"}), lucene.WithStemming(true))
	fmt.Print("custom stop words:          ")
	toks = toks[:0]
	for _, t := range custom.Analyze(raw) {
		toks = append(toks, t.Text)
	}
	fmt.Println(strings.Join(toks, " "))

	fmt.Printf("AnalyzeTerm(%q) = %q   AnalyzeTerm(%q) = %q\n",
		"Programming", std.AnalyzeTerm("Programming"),
		"networks", std.AnalyzeTerm("networks"))
	fmt.Printf("default stop words (%d): %v\n", len(lucene.DefaultStopWords()), lucene.DefaultStopWords())

	// Character-level helpers used by autocomplete / shingling pipelines.
	fmt.Printf("NGrams(%q,3)      = %v\n", "search", lucene.NGrams("search", 3))
	fmt.Printf("EdgeNGrams(%q,2,4)= %v\n", "search", lucene.EdgeNGrams("search", 2, 4))
	fmt.Printf("Shingles(...,2)    = %v\n", lucene.Shingles([]string{"go", "programming", "language"}, 2))

	// ---------------------------------------------------------------
	section("2. Building the index")
	// ---------------------------------------------------------------
	// NOTE: this library is purely in-memory; there is no on-disk directory or
	// persistence API, so no temp dir is needed. See README "Holes".
	idx := lucene.NewIndex(std)
	for _, d := range corpus {
		if err := idx.Add(d); err != nil {
			fmt.Fprintln(os.Stderr, "add:", err)
			os.Exit(1)
		}
	}
	fmt.Printf("NumDocs=%d Fields=%v\n", idx.NumDocs(), idx.Fields())
	fmt.Printf("Has(\"d3\")=%v Has(\"nope\")=%v\n", idx.Has("d3"), idx.Has("nope"))

	// Adding a duplicate ID replaces the previous version rather than growing
	// the index.
	corpus[3].Fields["title"] = "Effective Python, Second Edition"
	if err := idx.Add(corpus[3]); err != nil {
		fmt.Fprintln(os.Stderr, "re-add:", err)
		os.Exit(1)
	}
	fmt.Printf("after re-Add(d4): NumDocs=%d title=%q\n", idx.NumDocs(), corpus[3].Fields["title"])

	// An empty ID is rejected with the library's *Error type.
	err := idx.Add(lucene.Document{ID: "", Fields: map[string]string{"body": "x"}})
	var lerr *lucene.Error
	if err != nil {
		fmt.Printf("Add with empty ID -> %v (typed *lucene.Error: %v)\n", err, asLuceneError(err, &lerr))
	}

	// ---------------------------------------------------------------
	section("3. Term queries and BM25 ranking")
	// ---------------------------------------------------------------
	tq := lucene.NewTermQuery("body", "network")
	show("TermQuery body:network", tq, idx.Search(tq, 10))

	// "programming" appears in every body, so its IDF is ~0 and scores collapse:
	// a useful check that BM25 is really being computed.
	tq2 := lucene.NewTermQuery("body", "programming")
	show("TermQuery body:programming", tq2, idx.Search(tq2, 10))

	// Boost changes ranking magnitude but not order for a single term.
	boosted := lucene.NewBoostQuery(lucene.NewTermQuery("title", "rust"), 3)
	show("BoostQuery title:rust^3", boosted, idx.Search(boosted, 10))
	plain := lucene.NewTermQuery("title", "rust")
	show("TermQuery title:rust", plain, idx.Search(plain, 10))

	// ConstantScoreQuery flattens scores, so ordering falls back to doc ID.
	cs := lucene.NewConstantScoreQuery(lucene.NewTermQuery("lang", "go"), 1.5)
	show("ConstantScore lang:go", cs, idx.Search(cs, 10))

	// ---------------------------------------------------------------
	section("4. Phrase queries (positional)")
	// ---------------------------------------------------------------
	pq := lucene.NewPhraseQuery("body", "programming", "language")
	show("Phrase \"programming language\"", pq, idx.Search(pq, 10))

	// Reversed order must not match: positions are ordered.
	pqRev := lucene.NewPhraseQuery("body", "language", "programming")
	show("Phrase \"language programming\"", pqRev, idx.Search(pqRev, 10))

	// Phrase terms are analyzed (stemmed) at match time, so the surface form
	// "systems" still finds the indexed stem.
	pq3 := lucene.NewPhraseQuery("body", "distributed", "systems")
	show("Phrase \"distributed systems\"", pq3, idx.Search(pq3, 10))

	// ---------------------------------------------------------------
	section("5. Boolean queries")
	// ---------------------------------------------------------------
	bq := lucene.NewBooleanQuery().
		Add(lucene.NewTermQuery("body", "network"), lucene.Must).
		Add(lucene.NewTermQuery("lang", "rust"), lucene.MustNot).
		Add(lucene.NewTermQuery("body", "servers"), lucene.Should)
	show("+network -rust servers", bq, idx.Search(bq, 10))

	orQ := lucene.NewBooleanQuery().
		Add(lucene.NewTermQuery("lang", "rust"), lucene.Should).
		Add(lucene.NewTermQuery("lang", "python"), lucene.Should)
	show("lang:rust OR lang:python", orQ, idx.Search(orQ, 10))

	// TermsQuery is the compact equivalent of the Should-only boolean above.
	terms := lucene.NewTermsQuery("lang", "rust", "python")
	show("TermsQuery lang:{rust,python}", terms, idx.Search(terms, 10))

	// DisjunctionMax: best-of-fields scoring with a tie breaker.
	dis := lucene.NewDisjunctionMaxQuery(0.3,
		lucene.NewTermQuery("title", "go"),
		lucene.NewTermQuery("body", "go"))
	show("DisMax title:go | body:go", dis, idx.Search(dis, 10))

	// MatchAll.
	all := &lucene.MatchAllQuery{}
	show("MatchAllQuery", all, idx.Search(all, 3))

	// ---------------------------------------------------------------
	section("6. Prefix, wildcard, regexp and fuzzy")
	// ---------------------------------------------------------------
	pre := lucene.NewPrefixQuery("body", "net")
	show("PrefixQuery body:net*", pre, idx.Search(pre, 10))

	wc := lucene.NewWildcardQuery("body", "netw*k*")
	show("WildcardQuery body:netw*k*", wc, idx.Search(wc, 10))

	wc2 := lucene.NewWildcardQuery("title", "?ust")
	show("WildcardQuery title:?ust", wc2, idx.Search(wc2, 10))
	fmt.Printf("WildcardMatch(%q,%q)=%v  WildcardMatch(%q,%q)=%v\n",
		"net*k", "network", lucene.WildcardMatch("net*k", "network"),
		"net?", "network", lucene.WildcardMatch("net?", "network"))

	re, err := lucene.NewRegexpQuery("body", "pytho[nm]")
	if err != nil {
		fmt.Fprintln(os.Stderr, "regexp:", err)
		os.Exit(1)
	}
	show("RegexpQuery body:/pytho[nm]/", re, idx.Search(re, 10))
	if _, err := lucene.NewRegexpQuery("body", "([unclosed"); err != nil {
		fmt.Printf("invalid regexp rejected: %v\n", err)
	}

	fz := lucene.NewFuzzyQuery("body", "pythom", 1)
	show("FuzzyQuery body:pythom~1", fz, idx.Search(fz, 10))
	fmt.Printf("LevenshteinDistance(%q,%q)=%d  Damerau(%q,%q)=%d\n",
		"kitten", "sitting", lucene.LevenshteinDistance("kitten", "sitting"),
		"ab", "ba", lucene.DamerauLevenshteinDistance("ab", "ba"))
	fmt.Printf("Jaro=%.4f JaroWinkler=%.4f LevSim=%.4f NGramSim=%.4f Soundex(%q)=%s\n",
		lucene.JaroSimilarity("martha", "marhta"),
		lucene.JaroWinklerSimilarity("martha", "marhta"),
		lucene.LevenshteinSimilarity("network", "netwerk"),
		lucene.NGramSimilarity("network", "netwerk", 2),
		"Robert", lucene.Soundex("Robert"))

	// ---------------------------------------------------------------
	section("7. Range queries")
	// ---------------------------------------------------------------
	// Ranges are lexical, so zero-padded years behave like numbers.
	rq := lucene.NewRangeQuery("year", "2018", "2021", true, true)
	show("year:[2018 TO 2021]", rq, idx.Search(rq, 10))
	rqEx := lucene.NewRangeQuery("year", "2018", "2021", false, false)
	show("year:{2018 TO 2021}", rqEx, idx.Search(rqEx, 10))
	rqOpen := lucene.NewRangeQuery("year", "2022", "", true, true)
	show("year:[2022 TO *]", rqOpen, idx.Search(rqOpen, 10))

	// ---------------------------------------------------------------
	section("8. Query parser (query strings)")
	// ---------------------------------------------------------------
	for _, qs := range []string{
		"body:programming +body:go -lang:rust",
		`title:"network programming"`,
		"body:net*",
		"year:[2019 TO 2023]",
		"(lang:rust lang:python) +body:programming",
		"lang:go",
	} {
		res, err := idx.SearchString(qs, 5)
		if err != nil {
			fmt.Printf("SearchString(%q) -> error: %v\n", qs, err)
			continue
		}
		fmt.Printf("%-46s total=%d hits=%s\n", qs, res.Total, ids(res))
	}

	// Parser errors are reported, not panicked.
	for _, bad := range []string{`"unterminated`, "(unbalanced", "[a TO"} {
		if _, err := idx.SearchString(bad, 5); err != nil {
			fmt.Printf("bad query %-16q -> %v\n", bad, err)
		} else {
			fmt.Printf("bad query %-16q -> accepted (no error)\n", bad)
		}
	}

	// Probe which of classic Lucene's query-string syntaxes the parser accepts.
	// Several are unsupported even though the corresponding Query types exist as
	// Go constructors (see README "Holes").
	fmt.Println("-- advanced syntax support --")
	probe := lucene.NewParser(std, "body")
	for _, qs := range []string{
		"net?ork",       // single-char wildcard
		"net*work",      // interior wildcard
		"rust~1",        // fuzzy
		"go^2",          // term boost
		"go AND rust",   // keyword operators
		"go OR rust",    //
		"NOT rust",      //
		`title:"a b"~2`, // phrase slop
		"*:*",           // match-all literal
		"body:/pyth.n/", // regexp literal
	} {
		q, err := probe.Parse(qs)
		if err != nil {
			fmt.Printf("   %-16s -> ERROR %v\n", qs, err)
			continue
		}
		fmt.Printf("   %-16s -> %s\n", qs, q.String())
	}

	// A parser with an explicit default field lets unqualified terms hit "body".
	p := lucene.NewParser(std, "body")
	q, err := p.Parse("distributed +consensus")
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse:", err)
		os.Exit(1)
	}
	show("Parser(defaultField=body)", q, idx.Search(q, 10))

	// ---------------------------------------------------------------
	section("9. Index statistics and facets")
	// ---------------------------------------------------------------
	fmt.Printf("TermCount(body)=%d  DocFreq(body,network)=%d  TotalTermFreq(body,network)=%d\n",
		idx.TermCount("body"),
		idx.DocFreq("body", "network"),
		idx.TotalTermFreq("body", "network"))
	langTerms := idx.Terms("lang")
	fmt.Printf("Terms(lang)=%v\n", langTerms)
	fmt.Println("FacetCounts(lang):")
	for _, f := range idx.FacetCounts("lang") {
		fmt.Printf("   %-8s %d\n", f.Value, f.Count)
	}
	fmt.Println("FacetCounts(body) top 5:")
	for i, f := range idx.FacetCounts("body") {
		if i == 5 {
			break
		}
		fmt.Printf("   %-14s %d\n", f.Value, f.Count)
	}

	// ---------------------------------------------------------------
	section("10. Suggest, spellcheck and MoreLikeThis")
	// ---------------------------------------------------------------
	fmt.Printf("Suggest(body,\"net\",5)      = %v\n", idx.Suggest("body", "net", 5))
	fmt.Printf("Suggest(title,\"pro\",5)     = %v\n", idx.Suggest("title", "pro", 5))
	fmt.Printf("SpellCheck(body,\"netwrok\") = %v\n", idx.SpellCheck("body", "netwrok", 2, 5))
	fmt.Printf("SpellCheck(lang,\"rast\")    = %v\n", idx.SpellCheck("lang", "rast", 2, 5))

	mlt := idx.MoreLikeThis("d2", "body", 5)
	fmt.Printf("MoreLikeThis(d2, body): total=%d\n", mlt.Total)
	for i, h := range mlt.Hits {
		fmt.Printf("   %d. %-4s score=%.4f  %s\n", i+1, h.ID, h.Score, titleOf(h.ID))
	}

	// ---------------------------------------------------------------
	section("11. Highlighting")
	// ---------------------------------------------------------------
	h := lucene.NewHighlighter(idx.Analyzer(), "[", "]")
	snippet := corpus[1].Fields["body"]
	fmt.Println("term:   ", h.Highlight(snippet, lucene.NewTermQuery("body", "programming")))
	fmt.Println("boolean:", h.Highlight(snippet, bq))
	fmt.Println("prefix: ", h.Highlight(snippet, lucene.NewPrefixQuery("body", "net")))

	// ---------------------------------------------------------------
	section("12. Deletion and live-document accounting")
	// ---------------------------------------------------------------
	before := idx.Search(&lucene.MatchAllQuery{}, 0).Total
	fmt.Printf("Delete(d3)=%v Delete(d3 again)=%v\n", idx.Delete("d3"), idx.Delete("d3"))
	after := idx.Search(&lucene.MatchAllQuery{}, 0).Total
	fmt.Printf("MatchAll total before=%d after=%d NumDocs=%d Has(d3)=%v\n",
		before, after, idx.NumDocs(), idx.Has("d3"))
	res := idx.Search(lucene.NewTermQuery("lang", "rust"), 10)
	fmt.Printf("lang:rust after delete -> total=%d hits=%s\n", res.Total, ids(res))
	fmt.Printf("FacetCounts(lang) after delete: %v\n", idx.FacetCounts("lang"))

	fmt.Println("\nAll sections completed.")
}

func ids(r lucene.Result) string {
	if len(r.Hits) == 0 {
		return "[]"
	}
	out := make([]string, 0, len(r.Hits))
	for _, h := range r.Hits {
		out = append(out, fmt.Sprintf("%s(%.3f)", h.ID, h.Score))
	}
	return "[" + strings.Join(out, " ") + "]"
}

// asLuceneError reports whether err is the library's concrete *Error type.
func asLuceneError(err error, target **lucene.Error) bool {
	e, ok := err.(*lucene.Error)
	if ok {
		*target = e
	}
	return ok
}
