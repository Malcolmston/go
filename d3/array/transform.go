package array

import (
	"errors"
	"math"
	"math/rand/v2"
)

// Entry is one key/value pair from a grouping or rollup, used wherever this
// package returns results in a defined order. d3 returns a Map, which in
// JavaScript remembers insertion order; Go maps do not, so the ordered variants
// ([Groups], [Rollups], [Indexes]) return a slice of Entry instead. The order is
// always the order in which each key was first seen in the input, which makes
// the output reproducible run to run.
type Entry[K comparable, V any] struct {
	Key   K
	Value V
}

// ErrDuplicateKey is returned by [Index] and [Indexes] when two elements share
// a key. An index asserts that the key is unique; silently keeping the last
// element would hide a data error at exactly the moment it matters.
var ErrDuplicateKey = errors.New("array: duplicate key")

// Group partitions values into a map from key to the elements sharing that key,
// preserving input order within each group.
//
// Use [Groups] when the order of the *keys* matters — for rendering a legend or
// stacking series, where Go's randomized map iteration would otherwise reshuffle
// the chart on every run.
func Group[T any, K comparable](values []T, key func(T) K) map[K][]T {
	out := make(map[K][]T)
	for _, d := range values {
		k := key(d)
		out[k] = append(out[k], d)
	}
	return out
}

// Groups is [Group] with the keys in first-seen order.
func Groups[T any, K comparable](values []T, key func(T) K) []Entry[K, []T] {
	pos := make(map[K]int)
	var out []Entry[K, []T]
	for _, d := range values {
		k := key(d)
		i, seen := pos[k]
		if !seen {
			i = len(out)
			pos[k] = i
			out = append(out, Entry[K, []T]{Key: k})
		}
		out[i].Value = append(out[i].Value, d)
	}
	return out
}

// Rollup groups values by key and then reduces each group to a single value —
// group-and-aggregate in one pass, the shape almost every summary table wants.
// The reducer receives the group's elements in input order.
func Rollup[T any, K comparable, R any](values []T, reduce func([]T) R, key func(T) K) map[K]R {
	groups := Groups(values, key)
	out := make(map[K]R, len(groups))
	for _, g := range groups {
		out[g.Key] = reduce(g.Value)
	}
	return out
}

// Rollups is [Rollup] with the keys in first-seen order.
func Rollups[T any, K comparable, R any](values []T, reduce func([]T) R, key func(T) K) []Entry[K, R] {
	groups := Groups(values, key)
	out := make([]Entry[K, R], len(groups))
	for i, g := range groups {
		out[i] = Entry[K, R]{Key: g.Key, Value: reduce(g.Value)}
	}
	return out
}

// Index builds a map from key to the single element with that key. It returns
// [ErrDuplicateKey] if two elements collide; the partial map is returned
// alongside the error so a caller that wants last-wins behaviour can still use
// it after inspecting the failure.
func Index[T any, K comparable](values []T, key func(T) K) (map[K]T, error) {
	out := make(map[K]T, len(values))
	var err error
	for _, d := range values {
		k := key(d)
		if _, dup := out[k]; dup && err == nil {
			err = ErrDuplicateKey
		}
		out[k] = d
	}
	return out, err
}

// Indexes is [Index] with the keys in first-seen order.
func Indexes[T any, K comparable](values []T, key func(T) K) ([]Entry[K, T], error) {
	pos := make(map[K]int)
	var out []Entry[K, T]
	var err error
	for _, d := range values {
		k := key(d)
		if i, dup := pos[k]; dup {
			if err == nil {
				err = ErrDuplicateKey
			}
			out[i].Value = d
			continue
		}
		pos[k] = len(out)
		out = append(out, Entry[K, T]{Key: k, Value: d})
	}
	return out, err
}

// Merge concatenates the given slices into one, in order. The result is always a
// fresh slice, even when only one input is non-empty, so callers may modify it
// freely.
func Merge[T any](groups ...[]T) []T {
	n := 0
	for _, g := range groups {
		n += len(g)
	}
	out := make([]T, 0, n)
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// Flat flattens a slice of slices by one level. It is [Merge] applied to a
// slice rather than to variadic arguments, which is the form you get from
// [Groups] or from any nested computation; d3 spells this flat(iterable, 1).
func Flat[T any](values [][]T) []T { return Merge(values...) }

// Pairs returns the adjacent pairs of values: for n inputs there are n-1 pairs,
// and none at all for fewer than two. It is how you turn a series of samples
// into the segments between them.
func Pairs[T any](values []T) [][2]T {
	return PairsWith(values, func(a, b T) [2]T { return [2]T{a, b} })
}

// PairsWith returns the adjacent pairs of values reduced by the given function —
// PairsWith(xs, func(a, b float64) float64 { return b - a }) gives the
// successive differences of a series.
func PairsWith[T, R any](values []T, reduce func(a, b T) R) []R {
	if len(values) < 2 {
		return []R{}
	}
	out := make([]R, len(values)-1)
	for i := range out {
		out[i] = reduce(values[i], values[i+1])
	}
	return out
}

// Permute returns a new slice containing source[i] for each i in indexes, in
// that order. It reorders, duplicates or drops elements according to the index
// list, which makes it the natural companion to [RankOf] and to sorting an
// index array instead of the data.
//
// Out-of-range indexes are skipped rather than panicking, matching d3's
// behaviour of yielding undefined for them — but unlike d3 the result is
// shorter, since Go has no undefined to put in the gap.
func Permute[T any](source []T, indexes []int) []T {
	out := make([]T, 0, len(indexes))
	for _, i := range indexes {
		if i >= 0 && i < len(source) {
			out = append(out, source[i])
		}
	}
	return out
}

// Shuffle randomizes the order of values in place using the Fisher–Yates
// algorithm and returns the same slice. Randomness comes from math/rand/v2's
// global source, which is seeded unpredictably; use [ShuffleWith] for a
// reproducible shuffle.
func Shuffle[T any](values []T) []T {
	return ShuffleWith(values, rand.Float64)
}

// ShuffleWith is [Shuffle] with a caller-supplied source of randomness:
// random must return a value in [0, 1). It mirrors d3.shuffler, and exists so
// that tests and deterministic renders can pin the permutation by passing a
// seeded generator — for example (rand.New(rand.NewPCG(1, 2))).Float64.
func ShuffleWith[T any](values []T, random func() float64) []T {
	for n := len(values); n > 1; {
		i := int(random() * float64(n))
		n--
		if i < 0 {
			i = 0
		} else if i > n {
			i = n
		}
		values[i], values[n] = values[n], values[i]
	}
	return values
}

// Range returns the arithmetic progression start, start+step, start+2*step, …
// stopping *before* stop. The endpoint is excluded, so Range(0, 5, 1) has five
// elements ending at 4 — the same convention as Python's range and d3's, and the
// reason it composes with slice indexing.
//
// The count is computed once from the endpoints rather than by accumulating
// step, so floating-point error does not compound along the series and the
// length is exactly ceil((stop-start)/step). A zero, NaN or wrong-signed step
// yields an empty slice rather than looping forever.
func Range(start, stop, step float64) []float64 {
	if step == 0 || math.IsNaN(step) || math.IsNaN(start) || math.IsNaN(stop) {
		return []float64{}
	}
	nf := math.Ceil((stop - start) / step)
	if !(nf > 0) {
		return []float64{}
	}
	if nf > maxTicks {
		return []float64{}
	}
	n := int(nf)
	out := make([]float64, n)
	for i := range out {
		out[i] = start + float64(i)*step
	}
	return out
}

// RangeTo returns Range(0, stop, 1) — the integers 0 through stop-1 as float64.
func RangeTo(stop float64) []float64 { return Range(0, stop, 1) }

// Transpose exchanges the rows and columns of a matrix. Ragged input is
// truncated to the shortest row, so the result is always rectangular; that is
// d3's behaviour, and it is what keeps [Zip] from producing partly-filled tuples
// that the caller would then have to guard on.
func Transpose[T any](matrix [][]T) [][]T {
	if len(matrix) == 0 {
		return [][]T{}
	}
	m := len(matrix[0])
	for _, row := range matrix[1:] {
		if len(row) < m {
			m = len(row)
		}
	}
	out := make([][]T, m)
	for i := range out {
		row := make([]T, len(matrix))
		for j := range matrix {
			row[j] = matrix[j][i]
		}
		out[i] = row
	}
	return out
}

// Zip interleaves the given slices: the result's i-th element holds the i-th
// element of every input. It is [Transpose] over variadic arguments, and like
// Transpose it stops at the shortest input.
func Zip[T any](arrays ...[]T) [][]T { return Transpose(arrays) }
