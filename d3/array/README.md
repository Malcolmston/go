# array — Go port of d3-array — the data-wrangling half of d3

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/array.svg)](https://pkg.go.dev/github.com/malcolmston/d3/array)

Package array is a Go port of d3-array — the data-wrangling half of d3. It
provides the statistics (`Mean`, `Median`, `Quantile`, `Deviation`), the
binary-search primitives (`Bisect`, `Bisector`), the tick generator that every
d3 scale is built on (`Ticks`, `TickStep`, `TickIncrement`, `NiceTicks`), the
grouping and reshaping helpers (`Group`, `Rollup`, `Transpose`, `Zip`), the
set operations (`Union`, `Intersection`, `Difference`) and the histogram
generator (`NewBin`).

The package is standard-library only and has no notion of a DOM: it computes
numbers, and something else draws them.

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/array"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func Ascending[T cmp.Ordered](a, b T) int` | Ascending compares two values in natural ascending order, returning -1, 0 or 1, and is the default comparator throughout this package. |
| `func Bisect[T cmp.Ordered](a []T, x T) int` | Bisect is `BisectRight`. |
| `func BisectCenter(a []float64, x float64) int` | BisectCenter returns the index of the element in the ascending-sorted slice a that is *nearest* to x, rather than an insertion point. |
| `func BisectLeft[T cmp.Ordered](a []T, x T) int` | BisectLeft returns the insertion point for x in the ascending-sorted slice a such that every element before the returned index is strictly less than… |
| `func BisectLeftRange[T cmp.Ordered](a []T, x T, lo, hi int) int` | BisectLeftRange is `BisectLeft` restricted to the half-open index range [lo, hi). |
| `func BisectRight[T cmp.Ordered](a []T, x T) int` | BisectRight returns the insertion point for x in the ascending-sorted slice a such that every element before the returned index is less than *or… |
| `func BisectRightRange[T cmp.Ordered](a []T, x T, lo, hi int) int` | BisectRightRange is `BisectRight` restricted to the half-open index range [lo, hi). |
| `func Count[T any](values []T, value func(T) float64) int` | Count returns the number of values whose accessor result is defined — that is, not NaN. It is the denominator every other statistic in this file… |
| `func CountOf(values []float64) int` | CountOf is `Count` for a slice of numbers. |
| `func Cumsum[T any](values []T, value func(T) float64) []float64` | Cumsum returns the running total of values: a slice of the same length whose element i is the sum of elements 0 through i. |
| `func CumsumOf(values []float64) []float64` | CumsumOf is `Cumsum` for a slice of numbers. |
| `func Descending[T cmp.Ordered](a, b T) int` | Descending compares two values in natural descending order. |
| `func Deviation[T any](values []T, value func(T) float64) (float64, bool)` | Deviation returns the standard deviation — the square root of `Variance` — and false when the variance is undefined. |
| `func DeviationOf(values []float64) (float64, bool)` | DeviationOf is `Deviation` for a slice of numbers. |
| `func Difference[T comparable](values []T, others ...[]T) []T` | Difference returns the distinct elements of values that appear in none of the others, in the order they appear in values. |
| `func Disjoint[T comparable](a, b []T) bool` | Disjoint reports whether a and b share no elements. |
| `func Extent[T any](values []T, value func(T) float64) (min, max float64, ok bool)` | Extent returns the smallest and largest defined values in a single pass, and false if there are none. |
| `func ExtentOf(values []float64) (min, max float64, ok bool)` | ExtentOf is `Extent` for a slice of numbers. |
| `func FSum[T any](values []T, value func(T) float64) float64` | FSum returns the full-precision sum of the defined values: the float64 nearest the exact mathematical total, independent of the order in which the… |
| `func FSumOf(values []float64) float64` | FSumOf is `FSum` for a slice of numbers. |
| `func Flat[T any](values [][]T) []T` | Flat flattens a slice of slices by one level. |
| `func Group[T any, K comparable](values []T, key func(T) K) map[K][]T` | Group partitions values into a map from key to the elements sharing that key, preserving input order within each group. |
| `func Index[T any, K comparable](values []T, key func(T) K) (map[K]T, error)` | Index builds a map from key to the single element with that key. |
| `func Intersection[T comparable](values ...[]T) []T` | Intersection returns the distinct elements of the first slice that also appear in every one of the others, in the order they appear in the first. |
| `func Max[T any](values []T, value func(T) float64) (float64, bool)` | Max returns the largest defined value, and false if there is none. |
| `func MaxOf(values []float64) (float64, bool)` | MaxOf is `Max` for a slice of numbers. |
| `func Mean[T any](values []T, value func(T) float64) (float64, bool)` | Mean returns the arithmetic mean of the defined values, and false if there are none. |
| `func MeanOf(values []float64) (float64, bool)` | MeanOf is `Mean` for a slice of numbers. |
| `func Median[T any](values []T, value func(T) float64) (float64, bool)` | Median returns the 0.5-quantile of the defined values — the middle value of an odd-length series, the average of the two middle values of an… |
| `func MedianOf(values []float64) (float64, bool)` | MedianOf is `Median` for a slice of numbers. |
| `func Merge[T any](groups ...[]T) []T` | Merge concatenates the given slices into one, in order. |
| `func Min[T any](values []T, value func(T) float64) (float64, bool)` | Min returns the smallest defined value, and false if there is none. |
| `func MinOf(values []float64) (float64, bool)` | MinOf is `Min` for a slice of numbers. |
| `func Mode[T any](values []T, value func(T) float64) (float64, bool)` | Mode returns the most frequently occurring defined value, and false if there are none. |
| `func ModeOf(values []float64) (float64, bool)` | ModeOf is `Mode` for a slice of numbers. |
| `func NiceTicks(start, stop float64, count int) (float64, float64)` | NiceTicks extends the domain [start, stop] outward to the nearest round values for the given tick count, and returns the extended domain. |
| `func Pairs[T any](values []T) [][2]T` | Pairs returns the adjacent pairs of values: for n inputs there are n-1 pairs, and none at all for fewer than two. |
| `func PairsWith[T, R any](values []T, reduce func(a, b T) R) []R` | PairsWith returns the adjacent pairs of values reduced by the given function — PairsWith(xs, func(a, b float64) float64 { return b - a }) gives the… |
| `func Permute[T any](source []T, indexes []int) []T` | Permute returns a new slice containing source`i` for each i in indexes, in that order. |
| `func Quantile[T any](values []T, p float64, value func(T) float64) (float64, bool)` | Quantile returns the p-quantile of the defined values, where p is in [0, 1]: 0 is the minimum, 0.5 the median, 1 the maximum. |
| `func QuantileOf(values []float64, p float64) (float64, bool)` | QuantileOf is `Quantile` for a slice of numbers. |
| `func QuantileSorted[T any](sorted []T, p float64, value func(T) float64) (float64, bool)` | QuantileSorted returns the p-quantile of values that are *already sorted in ascending order*, using the same R-7 interpolation as `Quantile` but… |
| `func QuantileSortedOf(sorted []float64, p float64) (float64, bool)` | QuantileSortedOf is `QuantileSorted` for a sorted slice of numbers. |
| `func Quickselect[T any](values []T, k int, compare func(a, b T) int) []T` | Quickselect partially sorts values in place so that the element at index k ends up where it would be in a fully sorted slice, every element before k… |
| `func QuickselectOf(values []float64, k int) []float64` | QuickselectOf is `Quickselect` for a slice of numbers, ordered ascending. |
| `func QuickselectRange[T any](values []T, k, left, right int, compare func(a, b T) int) []T` | QuickselectRange is `Quickselect` restricted to the inclusive index range [left, right]; elements outside that range are not moved. |
| `func Range(start, stop, step float64) []float64` | Range returns the arithmetic progression start, start+step, start+2*step, … stopping *before* stop. |
| `func RangeTo(stop float64) []float64` | RangeTo returns Range(0, stop, 1) — the integers 0 through stop-1 as float64. |
| `func Rank[T any](values []T, compare func(a, b T) int) []float64` | Rank returns the zero-based rank of each element under the given comparator, index-aligned with the input: the smallest element ranks 0, the next 1,… |
| `func RankBy[T any](values []T, value func(T) float64) []float64` | RankBy ranks elements ascending by a numeric accessor, with the same NaN handling as `RankOf`. |
| `func RankOf(values []float64) []float64` | RankOf is `Rank` for a slice of numbers, ranked ascending. |
| `func Rollup[T any, K comparable, R any](values []T, reduce func([]T) R, key func(T) K) map[K]R` | Rollup groups values by key and then reduces each group to a single value — group-and-aggregate in one pass, the shape almost every summary table… |
| `func Shuffle[T any](values []T) []T` | Shuffle randomizes the order of values in place using the Fisher–Yates algorithm and returns the same slice. |
| `func ShuffleWith[T any](values []T, random func() float64) []T` | ShuffleWith is `Shuffle` with a caller-supplied source of randomness: random must return a value in [0, 1). |
| `func Subset[T comparable](a, b []T) bool` | Subset reports whether every element of a is also in b — Superset with its arguments swapped, spelled out because reading a nested Superset call… |
| `func Sum[T any](values []T, value func(T) float64) float64` | Sum returns the total of the defined values, treating NaN as zero. |
| `func SumOf(values []float64) float64` | SumOf is `Sum` for a slice of numbers. |
| `func Superset[T comparable](a, b []T) bool` | Superset reports whether every element of b is also in a. |
| `func ThresholdFreedmanDiaconis(values []float64, min, max float64) int` | ThresholdFreedmanDiaconis is the Freedman–Diaconis rule, which sizes bins by the interquartile range instead of the standard deviation:… |
| `func ThresholdScott(values []float64, min, max float64) int` | ThresholdScott is Scott's normal reference rule, which chooses a bin width of 3.49·σ·n^(-1/3). |
| `func ThresholdSturges(values []float64, min, max float64) int` | ThresholdSturges is Sturges' formula, ceil(log2(n)) + 1, and is the default rule. |
| `func TickIncrement(start, stop float64, count int) float64` | TickIncrement returns the raw increment `Ticks` would use for the domain [start, stop] at approximately count steps, in d3's signed encoding: a… |
| `func TickStep(start, stop float64, count int) float64` | TickStep returns the difference between adjacent values of Ticks(start, stop, count) — a "nice" step: a power of ten, optionally multiplied by two… |
| `func Ticks(start, stop float64, count int) []float64` | Ticks returns approximately count representative values spanning the domain [start, stop], chosen so that they are "nice" round numbers — multiples… |
| `func Transpose[T any](matrix [][]T) [][]T` | Transpose exchanges the rows and columns of a matrix. |
| `func Union[T comparable](values ...[]T) []T` | Union returns the distinct elements of all the given slices, in the order they are first encountered. |
| `func Variance[T any](values []T, value func(T) float64) (float64, bool)` | Variance returns the unbiased sample variance (dividing by n-1, not n) of the defined values, and false if there are fewer than two of them — a… |
| `func VarianceOf(values []float64) (float64, bool)` | VarianceOf is `Variance` for a slice of numbers. |
| `func Zip[T any](arrays ...[]T) [][]T` | Zip interleaves the given slices: the result's i-th element holds the i-th element of every input. |

### Types

| Type | What it is |
| --- | --- |
| `Adder` | Adder is a full-precision accumulator for float64 addition: however many values you add, and in whatever order, its `Adder.Sum` is the correctly… |
| `Bin` | Bin is one bucket of a histogram: the elements that fell into it, together with the half-open interval [X0, X1) it covers. |
| `Binner` | Binner is a configurable histogram generator: it computes how to divide a series into buckets and then assigns the data to them. |
| `Bisector` | Bisector performs binary search over a slice of arbitrary elements against a needle of a possibly different type, using a caller-supplied comparator. |
| `Entry` | Entry is one key/value pair from a grouping or rollup, used wherever this package returns results in a defined order. |
| `ThresholdFunc` | ThresholdFunc computes how many bins a series should be divided into, given its defined values and the domain those values span. |

<details>
<summary><code>Adder</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (a *Adder) Add(x float64) *Adder` | Add accumulates x and returns the receiver so that calls can be chained. |
| `func (a *Adder) Reset()` | Reset discards all accumulated values, returning the Adder to its zero state while keeping its allocated storage for reuse. |
| `func (a *Adder) Sum() float64` | Sum collapses the partial sums into the single float64 nearest the exact total. |

</details>

<details>
<summary><code>Bin</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (b Bin[T]) Len() int` | Len returns the number of elements in the bin, the quantity a histogram's bar height is normally proportional to. |

</details>

<details>
<summary><code>Binner</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewBin[T any](value func(T) float64) *Binner[T]` | NewBin returns a `Binner` over elements of type T, reading each element's numeric value through the given accessor. |
| `func NewBinOf() *Binner[float64]` | NewBinOf returns a `Binner` over a slice of numbers. |
| `func (b *Binner[T]) Apply(data []T) []Bin[T]` | Apply bins the data and returns the bins in ascending order of X0. |
| `func (b *Binner[T]) Domain(domain func(values []float64) (min, max float64, ok bool)) *Binner[T]` | Domain sets a function computing the [min, max] interval the bins must cover from the data's values; it should return false when there is no domain,… |
| `func (b *Binner[T]) DomainRange(min, max float64) *Binner[T]` | DomainRange fixes the domain to the constant interval [min, max]. |
| `func (b *Binner[T]) ThresholdCount(n int) *Binner[T]` | ThresholdCount asks for approximately n bins, with boundaries chosen by `Ticks`. |
| `func (b *Binner[T]) ThresholdValues(thresholds []float64) *Binner[T]` | ThresholdValues sets the bin boundaries explicitly: the given values become the interior cut points, so n thresholds produce n+1 bins (thresholds… |
| `func (b *Binner[T]) Thresholds(t ThresholdFunc) *Binner[T]` | Thresholds sets the rule that decides how many bins to use. |
| `func (b *Binner[T]) Value(value func(T) float64) *Binner[T]` | Value sets the accessor that reads each element's numeric value. |

</details>

<details>
<summary><code>Bisector</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func BisectorBy[T any, U cmp.Ordered](value func(T) U) *Bisector[T, U]` | BisectorBy returns a `Bisector` that compares elements to needles by projecting each element onto an ordered key — the common case of searching a… |
| `func BisectorOf[T any](value func(T) float64) *Bisector[T, float64]` | BisectorOf returns a `Bisector` over a numeric accessor. |
| `func NewBisector[T, X any](compare func(d T, x X) int) *Bisector[T, X]` | NewBisector returns a `Bisector` driven by a comparator that orders an element against a needle: compare(d, x) must be negative when d sorts before… |
| `func (b *Bisector[T, X]) Center(a []T, x X) int` | Center returns the index of the element of a nearest to x. |
| `func (b *Bisector[T, X]) Left(a []T, x X) int` | Left returns the leftmost insertion point for x in the sorted slice a. |
| `func (b *Bisector[T, X]) LeftRange(a []T, x X, lo, hi int) int` | LeftRange is `Bisector.Left` restricted to the half-open index range [lo, hi). |
| `func (b *Bisector[T, X]) Right(a []T, x X) int` | Right returns the rightmost insertion point for x in the sorted slice a. |
| `func (b *Bisector[T, X]) RightRange(a []T, x X, lo, hi int) int` | RightRange is `Bisector.Right` restricted to the half-open index range [lo, hi). |

</details>

<details>
<summary><code>Entry</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func Groups[T any, K comparable](values []T, key func(T) K) []Entry[K, []T]` | Groups is `Group` with the keys in first-seen order. |
| `func Indexes[T any, K comparable](values []T, key func(T) K) ([]Entry[K, T], error)` | Indexes is `Index` with the keys in first-seen order. |
| `func Rollups[T any, K comparable, R any](values []T, reduce func([]T) R, key func(T) K) []Entry[K, R]` | Rollups is `Rollup` with the keys in first-seen order. |

</details>

### Variables

`ErrDuplicateKey`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/array).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
