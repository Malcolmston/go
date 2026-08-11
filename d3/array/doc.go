// Package array is a Go port of d3-array — the data-wrangling half of d3.
// It provides the statistics ([Mean], [Median], [Quantile], [Deviation]), the
// binary-search primitives ([Bisect], [Bisector]), the tick generator that every
// d3 scale is built on ([Ticks], [TickStep], [TickIncrement], [NiceTicks]), the
// grouping and reshaping helpers ([Group], [Rollup], [Transpose], [Zip]), the
// set operations ([Union], [Intersection], [Difference]) and the histogram
// generator ([NewBin]).
//
// The package is standard-library only and has no notion of a DOM: it computes
// numbers, and something else draws them.
//
// # Accessors and generics
//
// d3's array functions are generic over the element type and take an optional
// accessor — d3.mean(data, d => d.price). Go generics express this directly, so
// the convention throughout this package is:
//
//	Mean(values []T, value func(T) float64) (float64, bool)   // generic form
//	MeanOf(values []float64) (float64, bool)                  // []float64 form
//
// Every statistic that reduces to a number comes in both flavors. The generic
// form takes an accessor func(T) float64; the …Of form is the same function
// specialized to a plain []float64 (it exists because Mean(xs, func(x float64)
// float64 { return x }) reads badly, and slices of numbers are the common case).
// The accessor deliberately does *not* receive the element index — d3 passes
// (d, i, data) but the index is used vanishingly rarely, and dropping it makes
// every call site shorter. Where an index genuinely matters, index the slice
// yourself and use the …Of form.
//
// Functions that are generic over an *ordering* rather than a number — [Bisect],
// [Ascending], [Quickselect] — constrain to [cmp.Ordered] or take a comparator
// func(a, b T) int in the shape the standard library's slices package uses, so
// they compose with slices.SortFunc directly.
//
// # Empty input and NaN — read this once
//
// JavaScript has undefined; Go does not, and a silent zero would be a bug
// factory. Every statistic that can fail to have an answer returns a second
// bool result, ok, which is false when there was nothing to compute from:
//
//	m, ok := array.MinOf(nil)      // m == 0, ok == false
//	m, ok := array.MinOf([]float64{2, 1}) // m == 1, ok == true
//
// Never use the value when ok is false. This mirrors d3 returning undefined for
// min/max/extent/mean/median/mode/quantile/variance/deviation of an empty input.
//
// NaN is treated as d3 treats null, undefined and NaN: it is *skipped* by the
// statistics. So MeanOf([]float64{1, NaN, 3}) is 2 (ok), not NaN, and
// CountOf reports how many defined values a slice actually has. A slice
// containing nothing but NaN is, for statistical purposes, empty — ok is false.
// [Sum] and [FSum] are the exception in shape only: they return 0 for empty
// input without an ok, exactly as d3.sum does, because the sum of nothing is
// conventionally zero.
//
// Ordering functions take the opposite view, because a sort needs a total
// order: [Ascending] and [Descending] both place NaN *last*, matching the
// ascendingDefined comparator d3 sorts with. [Rank] likewise assigns NaN as
// the rank of an undefined value rather than dropping it, so the result is
// index-aligned with the input.
//
// # Ticks
//
// [Ticks], [TickIncrement] and [TickStep] are the foundation of every d3 scale's
// axis, and this port follows d3-array's tickSpec algorithm literally, including
// the trick of tracking a negative "increment" to mean 1/n so that ticks below 1
// come out as exact divisions (3/10 == 0.3, not 0.1+0.1+0.1). JavaScript's
// Math.round rounds halves toward +∞ while Go's math.Round rounds them away from
// zero, so the port uses floor(x+0.5) internally; without that, negative domains
// would drift by one tick. Scale implementations should call [TickStep] to pick
// a step and [NiceTicks] to round a domain outward — not reimplement either.
package array
