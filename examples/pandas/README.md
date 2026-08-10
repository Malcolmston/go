# pandas example

A single runnable program that exercises `github.com/malcolmston/pandas` (the
Go DataFrame/Series port) against a small six-row sales fixture with a
deliberate missing value, and checks ~90 results against hand-computed values.

## Module under test

The example consumes the library as a **published** module — there is no
`replace` directive and nothing references the local `../../pandas` folder.

| | |
| --- | --- |
| Module path | `github.com/malcolmston/pandas` |
| Resolved version | `v0.0.0-20260719012934-41f0765ac587` (pseudo-version; the repo has no semver tags, so `@latest` resolves to the tip commit `41f0765`) |
| Third-party deps | none — the library imports only the standard library |

The published module contents are byte-identical to the repository working tree
for every `.go` file, so nothing in this example depends on uncommitted local
changes.

## Run

```sh
cd examples/pandas
GOWORK=off go get github.com/malcolmston/pandas@latest
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

No network access is required and the program terminates on its own. CSV files
are written into an `os.MkdirTemp` directory that is removed on exit.

## What it demonstrates

| Section | Coverage |
| --- | --- |
| 1 | `NewSeries` / `NewSeriesTyped`, dtype inference (`Float64`/`Int64`/`String`/`Bool`/`Object`), explicit coercion (`"1.5"` → 1.5, `"oops"` → NA), `IsNA`, `Count` |
| 2 | `FromMap` (with explicit column order), `FromRecords` (with `pandas:"..."` struct tags, unexported fields skipped), `NewDataFrame`, ragged-column error |
| 3 | `Col`/`MustCol`, `Select`, `Head`, `Tail`, `ILoc` (with clamping), `Take`, `Loc` on the default index and on a string index via `SetIndex`, `ResetIndex` |
| 4 | `Filter` with a boolean mask, `FilterFunc` with a `Row` predicate, `Series.Between`, `Series.IsIn`, `Str().Contains`, `Str().Upper` |
| 5 | `WithColumn` (add + NA propagation), `Series.Div` arithmetic, `Round`, `Drop`, `Rename`, wrong-length `WithColumn` error |
| 6 | `SortBy` with mixed ascending/descending keys and NA-sorts-last, `Series.Sort`, `NLargest`, unknown-column error |
| 7 | `GroupBy` single and multi-key, `Sum`/`Mean`/`Count`, general `Agg` with `AggSum`/`AggMean`/`AggMax`/`AggCount`/`AggMin`/`AggStd`, `ValueCounts`, `Unique`, `Nunique`, unknown-key error |
| 8 | `Merge` with `InnerJoin` and `LeftJoin`, `_left`/`_right` suffixing on colliding columns, unmatched-key NA filling, missing-key error, `Concat` |
| 9 | `IsNA`, `DataFrame.DropNA`, `Series.DropNA`, `FillNA` and the resulting change in mean (126 → 105) |
| 10 | `Describe`, `Median`, `Var`, `Quantile`, `Prod`, `ArgMax`/`ArgMin`, `Mode`, frame-level `Sum`/`Mean`/`Max`, `Corr` matrix, `CumSum`/`Diff`/`Rank`/`Shift`, `Astype` round-trip |
| 11 | `WriteCSVFile`/`ReadCSVFile` round-trip through a temp dir, `WriteCSV` to `os.Stdout`, `ReadCSV` from an in-memory `strings.NewReader` with a custom `Delimiter`, `NAValues`, and `NoHeader` |
| 12 | `DropDuplicates`, `Transpose`, `Copy` independence |

### Verified against hand-computed values

All of these matched:

- `revenue` sum 630, mean 126 (NA excluded); after `FillNA(0)` mean 105.
- `units` sum 59, mean 59/6, median 9 (`(8+10)/2`), sample variance 33.766667,
  sample std 5.810909281, product 384000, argmax 1, argmin 5.
- GroupBy by region: East revenue 530 / units 42 / non-NA revenue count 3;
  West revenue 100 (the NA row skipped) / units 17 / count 2; West units mean
  17/3.
- GroupBy by (region, product): `(East, widget)` revenue sum 350, mean 175,
  max 250, count 2; units sum 30, min 10, sample std `sqrt(50)` = 7.0710678.
  Singleton groups correctly yield NA for std.
- `Describe` on `units`: count 6, mean 9.833333, std 5.810909, min 4, max 20;
  `revenue` count 5 (NA excluded).
- Join row counts: inner 6, left 6 with 3 non-NA `target`, inner against an
  East-only lookup 3.

The library got every one of these right. The only numeric surprise was in the
CSV round-trip, and it is a dtype issue rather than a value issue.

## Holes found

Three are surfaced at runtime by the program (printed under `HOLE`), the rest
are API-shape observations made while writing it. Nothing had to be commented
out — every feature the README advertises exists and works.

### Behavioural

1. **CSV round-trip demotes `Float64` to `Int64`.** `WriteCSV` formats values
   via the internal `formatValue` (`strconv.FormatFloat(..., 'g', -1, 64)`), so
   `100.0` is written as `100`. `ReadCSV`'s inference then sees an all-integral
   column and returns `Int64`. Values survive (sum is still 630) but the dtype
   does not, and `ReadCSVOptions` exposes only `Delimiter`, `NoHeader` and
   `NAValues` — there is no `DTypes`/converter hook to force a column back to
   float. Workaround: `Astype(pandas.Float64)` after reading.
2. **`Loc` label matching is type-exact and silently empty on a miss.**
   `defaultIndex` stores `int64`, and `Loc` looks labels up in a
   `map[any][]int`, so the natural-looking `df.Loc(0, 3)` returns **zero rows**
   rather than an error — you must write `df.Loc(int64(0), int64(3))`. Labels
   that are not in the index are skipped with no diagnostic, so a typo in a
   string index is indistinguishable from an empty selection. `Take` behaves
   the same way for out-of-range positions.
3. **Inconsistent dtype widening on transforms.** `CumSum`, `CumProd`, `Diff`,
   `PctChange`, `Rank` and `Abs` return `Float64` for an `Int64` input, but
   `Shift` preserves `Int64`. There is no documented rule for which transforms
   widen.

### Missing / absent APIs (relative to pandas)

4. No `DataFrame.DTypes()` (or `Info()`) accessor — you must loop over
   `Names()` and call `MustCol(n).DType()` yourself.
5. The `AggFunc` set is limited to `AggSum`/`AggMean`/`AggMin`/`AggMax`/
   `AggCount`/`AggStd`. There is no `median`, `var`, `nunique`, `first`/`last`
   or user-supplied aggregation, even though `Series` itself has `Median`,
   `Var`, `Quantile` and `Mode`. `GroupBy` also has no `Apply`, no iteration
   over groups, and no accessor for a single group's rows — `Groups()` only
   returns the count, so the group keys are not reachable from outside.
6. Joins are inner/left only: no `RightJoin`, no `OuterJoin`, no multi-key
   `on`, and no join-on-index.
7. No reshaping: no `Pivot`, `PivotTable`, `Melt`, `Stack`/`Unstack`,
   `Rolling`, `Resample`, `Explode` or `Crosstab`.
8. No datetime/temporal dtype at all, so no time-series indexing, no
   `Resample`, and no `Dt` accessor to match the `Str` accessor.
9. No column-wise or row-wise `DataFrame.Apply`/`Map` — `Apply` exists only on
   `Series`, so a multi-column derived column means building a `[]any` by hand
   and calling `WithColumn` (as section 5 does).
10. `FillNA` cannot forward/backward fill or fill per-column from a map: the
    signature is `FillNA(column string, value any)`, with `""` meaning "all
    columns, same value". `DropNA` has no `how`/`subset`/`thresh` equivalent —
    it is always "drop rows with any NA".
11. `Describe` covers only numeric columns and only count/mean/std/min/max; no
    quartiles (even though `Series.Quantile` exists) and no object-column
    summary. It also returns the statistic names in a `stat` **column** rather
    than as the index, which means `Describe()` output cannot be indexed with
    `Loc("mean")` without a `SetIndex("stat")` first.
12. No `Series.Loc`/`Series.ILoc`, no `Series.Reindex`, no index alignment on
    arithmetic — `Add`/`Sub`/`Mul`/`Div` are strictly positional, so two
    Series with different indexes are combined by position, silently.

### Non-idiomatic / friction

13. `(value, bool)` is used for both "column does not exist" and "value is NA":
    `Row.Get`, `Row.Float` and `Series.At` all return `ok=false` for either
    case, so a typo'd column name reads exactly like a missing value. An
    `error` (or a separate `HasColumn` guard) would separate the two.
14. `DataFrame.String()` deliberately omits the index, so `SetIndex` results
    look like they lost a column until you call `Index()` or `ResetIndex()`.
    Printing an index-bearing frame usefully requires `ResetIndex()` first.
15. Constructors that cannot fail still return `error` (`FromMap` on
    well-formed input, `GroupBy` on a known key), which forces `must`-style
    wrappers in example code; conversely genuinely lossy operations (`Loc`,
    `Take`, `Filter` with a short mask) return no error at all.
16. `Series.Take` is documented as "used by GroupBy aggregation" but is
    exported, and it lives in `groupby.go` rather than `series.go`.
17. Every builder takes `[]any`, so numeric data must be boxed element by
    element (`{10, 20, 5}`); there is no `NewSeriesFloat64([]float64)`-style
    typed constructor despite the package being generics-capable.

## Compatibility notes

- The README quick-start snippet compiles and runs verbatim.
- `doc.go` accurately describes the shipped API; no README-vs-code mismatch was
  found in names or signatures.
- The module fetches cleanly from the proxy at
  `v0.0.0-20260719012934-41f0765ac587` and has no third-party dependencies, so
  `go mod tidy` produces a two-line `go.sum`.
- The module publishes no semver tags, so consumers get a pseudo-version and
  `go get github.com/malcolmston/pandas` cannot be pinned to a release. The
  repo does carry a `VERSION` file, which is not visible to the module system.
