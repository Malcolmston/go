# lucene example

A single runnable program that exercises `github.com/malcolmston/lucene`
end to end against a six-document corpus of programming-book records
(fields: `title`, `body`, `lang`, `year`).

The library is consumed as a **published Go module** — there is no `replace`
directive and no reference to the local checkout.

Resolved module version (the repo has no semver tags, so `@latest` yields a
pseudo-version):

```
github.com/malcolmston/lucene v0.0.0-20260719012641-dfa60563d0ba
```

Fetching it worked on the first try:

```sh
GOWORK=off go get github.com/malcolmston/lucene@latest
# go: downloading github.com/malcolmston/lucene v0.0.0-20260719012641-dfa60563d0ba
```

For the record, the published module's exported API is byte-for-byte identical
to the local `../../lucene` working tree, so nothing in the notes below is an
artefact of uncommitted local changes.

## Run

```sh
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

The program prints twelve labelled sections and exits on its own.

## What it demonstrates

1. **Analyzers** — `NewStandardAnalyzer`, `NewAnalyzer` with `WithStemming` and
   `WithStopWords`, token positions, `AnalyzeTerm`, `DefaultStopWords`, plus the
   `NGrams` / `EdgeNGrams` / `Shingles` helpers.
2. **Indexing** — `NewIndex`, `Add`, upsert-by-ID semantics, `NumDocs`,
   `Fields`, `Has`, and the typed `*lucene.Error` returned for an empty doc ID.
3. **Term queries and BM25** — score ordering, the near-zero IDF of a term
   present in every document, `BoostQuery`, `ConstantScoreQuery`.
4. **Phrase queries** — positional adjacency, and the negative case that the
   reversed phrase matches nothing.
5. **Boolean queries** — `Must` / `MustNot` / `Should`, `TermsQuery`,
   `DisjunctionMaxQuery`, `MatchAllQuery`.
6. **Prefix / wildcard / regexp / fuzzy** — `PrefixQuery`, `WildcardQuery`
   (`*` and `?`), `WildcardMatch`, `RegexpQuery` (including rejection of an
   invalid pattern), `FuzzyQuery`, and the string-distance helpers
   (`LevenshteinDistance`, `DamerauLevenshteinDistance`, `JaroSimilarity`,
   `JaroWinklerSimilarity`, `LevenshteinSimilarity`, `NGramSimilarity`,
   `Soundex`).
7. **Range queries** — inclusive, exclusive and half-open lexical ranges over
   the `year` field.
8. **Query parser** — `SearchString` and `NewParser`/`Parse`, structural error
   reporting, and a probe of which classic Lucene query-string syntaxes are
   actually supported.
9. **Statistics and facets** — `TermCount`, `DocFreq`, `TotalTermFreq`,
   `Terms`, `FacetCounts`.
10. **Suggest / SpellCheck / MoreLikeThis**.
11. **Highlighting** — `NewHighlighter` over term, boolean and prefix queries.
12. **Deletion** — `Delete`, live-document accounting, and facet counts after a
    delete.

Everything compiles and runs with no `// HOLE:` stub-outs: no API in the package
is missing or panicking.

## Holes found

Nothing crashed and nothing had to be commented out. The gaps are all in
*coverage* and in query-string parsing.

### 1. No persistence — the index is memory-only

There is no `Directory`, no `Open`/`Save`/`Load`/`Close`, no reader/writer
split, no commit, no segment merging. The whole index is a `map` behind a
`sync.RWMutex`, so a process restart loses it, and an index larger than RAM is
impossible. The README is honest about being "in-memory", but for a library
named after Lucene this is the single largest absence, and it means the
"use a temp dir for the on-disk index" part of a normal Lucene example has
nothing to bind to.

### 2. Query parser is much weaker than the constructor API

Several query types exist as Go constructors but have **no query-string
syntax**, and — worse — the parser silently accepts the Lucene spelling of them
as a *literal term* instead of erroring. Observed output from section 8:

| input | parsed as | expected |
| --- | --- | --- |
| `net?ork` | `body:net?ork` (literal term) | `WildcardQuery` — `NewWildcardQuery` supports `?` |
| `net*work` | `body:net* body:work` (two clauses!) | interior wildcard |
| `rust~1` | `body:rust~1` (literal term) | `FuzzyQuery` — `NewFuzzyQuery` exists |
| `go^2` | `body:go^2` (literal term) | `BoostQuery` — `NewBoostQuery` exists |
| `go AND rust` | `body:go body:AND body:rust` | keyword operators |
| `go OR rust` | `body:go body:OR body:rust` | keyword operators |
| `NOT rust` | `body:NOT body:rust` | keyword operator |
| `title:"a b"~2` | `title:"a b"` **plus a junk clause** `body:~2` | phrase slop |
| `body:/pyth.n/` | `body:/pyth.n/` (literal term) | `RegexpQuery` — `NewRegexpQuery` exists |

Silently turning `net*work` into two OR'd clauses and `"a b"~2` into a spurious
extra clause is the worst of these: the query changes meaning with no error.
The README's syntax table does document only the supported subset, so this is a
capability gap rather than a documentation lie — but the mismatch between "the
`FuzzyQuery`/`RegexpQuery`/`BoostQuery` types exist" and "you cannot write them
in a query string" is jarring.

### 3. Stemmer is not Porter and conflates/splits oddly

`NewStandardAnalyzer().Analyze("The Runners were Running QUICKLY through 12 Networks!")`
yields `runn, were, run, quick, through, 12, network`. So:

- `Runners` → `runn` but `Running` → `run`: two forms of the same word land on
  **different** terms, which is exactly what a stemmer exists to prevent. A
  Porter stemmer gives `runner` and `run`; markdown-style "suffix stripping"
  here produces the non-word `runn`.
- The README calls it "a Porter-style suffix-stripping stemmer", which is fair
  warning, but the consequence (recall loss on `-ers` vs `-ing`) is real.

`DefaultStopWords()` is a 33-word list that omits common function words such as
`were`, `has`, `from`, `we`, `you` — noticeably thinner than Lucene's
English stop set.

### 4. No phrase slop, no per-field boosts at index time, no positional/span queries

`PhraseQuery` is strictly adjacent (`Slop` does not exist as a field or
argument), so `NewPhraseQuery("body","distributed","partitions")` can never
match. There is no `SpanNearQuery` equivalent, no `MultiPhraseQuery`, and
`Document` has no per-field boost or per-field analyzer — one analyzer applies
to every field of every document, so a `year` or `lang` keyword field gets
stemmed and stop-worded like prose. In this example `year` values survive only
because digits pass through the stemmer unchanged.

### 5. No explanation / debugging API

There is no `Explain` (Lucene's per-clause score breakdown), and `Hit` exposes
only `ID` and `Score` — the internal `docNum` field is unexported, and there is
no way to retrieve the stored `Document` back out of the index (no `Doc(id)`
accessor). An application must keep its own ID → document map, as this example
does.

### 6. Minor / non-idiomatic

- `*lucene.Error` does not implement `Unwrap` and there are no sentinel error
  values, so `errors.Is` cannot classify a failure; callers must type-assert and
  compare the `Op`/`Msg` strings.
- `Index.Search` takes `topN int` where `topN <= 0` means "all", which is easy
  to hit by accident with a zero value.
- Range bounds are `string`-typed and compared lexically, so numeric ranges only
  work if every value is zero-padded to the same width — there is no numeric or
  time field type (`NewRangeQuery("year","900","2000",...)` would behave
  surprisingly).
- `Result.Total` is the count of matching documents while `Hits` is truncated,
  which is right, but `Search(q, 0)` returning everything makes `Total` and
  `len(Hits)` inconsistent in a way that is easy to misread.
