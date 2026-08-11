package array

import (
	"math"
	"slices"
)

// identity is the accessor the …Of variants use. It is a package-level function
// rather than a closure allocated per call so the specialized wrappers cost
// nothing beyond the generic call itself.
func identity(x float64) float64 { return x }

// defined reports whether v is a value the statistics should consider. NaN
// stands in for JavaScript's null and undefined as well as for NaN itself; see
// the package documentation for why they are skipped rather than propagated.
func defined(v float64) bool { return !math.IsNaN(v) }

// Count returns the number of values whose accessor result is defined — that
// is, not NaN. It is the denominator every other statistic in this file uses,
// and it is exported because "how many of these points actually have a value?"
// is a question worth asking before plotting them.
func Count[T any](values []T, value func(T) float64) int {
	n := 0
	for _, d := range values {
		if defined(value(d)) {
			n++
		}
	}
	return n
}

// CountOf is [Count] for a slice of numbers.
func CountOf(values []float64) int { return Count(values, identity) }

// Min returns the smallest defined value, and false if there is none. Unlike
// sorting, it compares numerically rather than lexically, and unlike a naive
// loop it ignores NaN entirely rather than letting one poison the comparison.
func Min[T any](values []T, value func(T) float64) (float64, bool) {
	min, ok := 0.0, false
	for _, d := range values {
		v := value(d)
		if !defined(v) {
			continue
		}
		if !ok || v < min {
			min, ok = v, true
		}
	}
	return min, ok
}

// MinOf is [Min] for a slice of numbers.
func MinOf(values []float64) (float64, bool) { return Min(values, identity) }

// Max returns the largest defined value, and false if there is none.
func Max[T any](values []T, value func(T) float64) (float64, bool) {
	max, ok := 0.0, false
	for _, d := range values {
		v := value(d)
		if !defined(v) {
			continue
		}
		if !ok || v > max {
			max, ok = v, true
		}
	}
	return max, ok
}

// MaxOf is [Max] for a slice of numbers.
func MaxOf(values []float64) (float64, bool) { return Max(values, identity) }

// Extent returns the smallest and largest defined values in a single pass, and
// false if there are none. This is the function to build a scale's domain from:
// it is what d3.extent exists for, and doing it in one pass rather than calling
// Min and Max separately matters when the accessor is expensive.
func Extent[T any](values []T, value func(T) float64) (min, max float64, ok bool) {
	for _, d := range values {
		v := value(d)
		if !defined(v) {
			continue
		}
		switch {
		case !ok:
			min, max, ok = v, v, true
		case v < min:
			min = v
		case v > max:
			max = v
		}
	}
	return min, max, ok
}

// ExtentOf is [Extent] for a slice of numbers.
func ExtentOf(values []float64) (min, max float64, ok bool) { return Extent(values, identity) }

// Sum returns the total of the defined values, treating NaN as zero. It returns
// 0 rather than a false ok for empty input, because the sum of nothing is
// conventionally zero and callers overwhelmingly want that; use [Count] if you
// need to distinguish "empty" from "sums to zero".
//
// Values are added in order with ordinary float64 arithmetic, so the result
// depends on that order to within rounding. When it must not, use [FSum].
func Sum[T any](values []T, value func(T) float64) float64 {
	total := 0.0
	for _, d := range values {
		if v := value(d); defined(v) {
			total += v
		}
	}
	return total
}

// SumOf is [Sum] for a slice of numbers.
func SumOf(values []float64) float64 { return Sum(values, identity) }

// FSum returns the full-precision sum of the defined values: the float64
// nearest the exact mathematical total, independent of the order in which the
// values appear. See [Adder] for how and why.
func FSum[T any](values []T, value func(T) float64) float64 {
	var a Adder
	for _, d := range values {
		if v := value(d); defined(v) {
			a.Add(v)
		}
	}
	return a.Sum()
}

// FSumOf is [FSum] for a slice of numbers.
func FSumOf(values []float64) float64 { return FSum(values, identity) }

// Cumsum returns the running total of values: a slice of the same length whose
// element i is the sum of elements 0 through i. NaN contributes zero but still
// occupies its position, so the result stays index-aligned with the input — a
// cumulative series is usually plotted against the original x values, and
// dropping entries would silently shift the curve.
//
// The running total is accumulated with an [Adder], so a long series does not
// drift the way repeated += would.
func Cumsum[T any](values []T, value func(T) float64) []float64 {
	out := make([]float64, len(values))
	var a Adder
	for i, d := range values {
		if v := value(d); defined(v) {
			a.Add(v)
		}
		out[i] = a.Sum()
	}
	return out
}

// CumsumOf is [Cumsum] for a slice of numbers.
func CumsumOf(values []float64) []float64 { return Cumsum(values, identity) }

// Mean returns the arithmetic mean of the defined values, and false if there
// are none. NaN entries are excluded from both the sum and the count, so a
// series with holes in it means what you would expect.
func Mean[T any](values []T, value func(T) float64) (float64, bool) {
	total, n := 0.0, 0
	for _, d := range values {
		if v := value(d); defined(v) {
			total += v
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return total / float64(n), true
}

// MeanOf is [Mean] for a slice of numbers.
func MeanOf(values []float64) (float64, bool) { return Mean(values, identity) }

// Median returns the 0.5-quantile of the defined values — the middle value of
// an odd-length series, the average of the two middle values of an even-length
// one — and false if there are none.
func Median[T any](values []T, value func(T) float64) (float64, bool) {
	return Quantile(values, 0.5, value)
}

// MedianOf is [Median] for a slice of numbers.
func MedianOf(values []float64) (float64, bool) { return Median(values, identity) }

// Variance returns the unbiased sample variance (dividing by n-1, not n) of the
// defined values, and false if there are fewer than two of them — a single
// observation has no spread to measure, and Bessel's correction would divide by
// zero.
//
// The computation is Welford's online algorithm rather than the textbook
// E[x²]-E[x]²: the latter subtracts two large nearly-equal numbers and can
// return a negative "variance" for tightly clustered data, which then produces
// NaN in [Deviation] and a broken axis downstream.
func Variance[T any](values []T, value func(T) float64) (float64, bool) {
	count := 0
	mean, sum := 0.0, 0.0
	for _, d := range values {
		v := value(d)
		if !defined(v) {
			continue
		}
		count++
		delta := v - mean
		mean += delta / float64(count)
		sum += delta * (v - mean)
	}
	if count < 2 {
		return 0, false
	}
	return sum / float64(count-1), true
}

// VarianceOf is [Variance] for a slice of numbers.
func VarianceOf(values []float64) (float64, bool) { return Variance(values, identity) }

// Deviation returns the standard deviation — the square root of [Variance] —
// and false when the variance is undefined.
func Deviation[T any](values []T, value func(T) float64) (float64, bool) {
	v, ok := Variance(values, value)
	if !ok {
		return 0, false
	}
	return math.Sqrt(v), true
}

// DeviationOf is [Deviation] for a slice of numbers.
func DeviationOf(values []float64) (float64, bool) { return Deviation(values, identity) }

// Mode returns the most frequently occurring defined value, and false if there
// are none. Ties are broken in favour of the value that appears first in the
// input, which makes the result deterministic — Go's map iteration order is not,
// so this function tracks first-seen order explicitly rather than ranging over
// the counts.
func Mode[T any](values []T, value func(T) float64) (float64, bool) {
	counts := make(map[float64]int)
	order := make([]float64, 0, len(values))
	for _, d := range values {
		v := value(d)
		if !defined(v) {
			continue
		}
		if _, seen := counts[v]; !seen {
			order = append(order, v)
		}
		counts[v]++
	}
	best, bestCount, ok := 0.0, 0, false
	for _, v := range order {
		if c := counts[v]; c > bestCount {
			best, bestCount, ok = v, c, true
		}
	}
	return best, ok
}

// ModeOf is [Mode] for a slice of numbers.
func ModeOf(values []float64) (float64, bool) { return Mode(values, identity) }

// Quantile returns the p-quantile of the defined values, where p is in [0, 1]:
// 0 is the minimum, 0.5 the median, 1 the maximum. It returns false when there
// are no defined values or p is NaN.
//
// The interpolation is R's type 7, the default in R and the one d3 uses, and it
// is worth being precise about because quantile definitions differ and a
// box plot drawn with the wrong one is subtly wrong everywhere. Type 7 places
// the p-quantile at position (n-1)p in the sorted values and interpolates
// linearly between the two neighbours; consequently the quartiles of [1,2,3,4]
// are 1.75 and 3.25, not 1.5 and 3.5.
//
// The input is not modified: Quantile sorts a filtered copy. When the values are
// already sorted and free of NaN, [QuantileSorted] skips that work.
func Quantile[T any](values []T, p float64, value func(T) float64) (float64, bool) {
	if math.IsNaN(p) {
		return 0, false
	}
	nums := make([]float64, 0, len(values))
	for _, d := range values {
		if v := value(d); defined(v) {
			nums = append(nums, v)
		}
	}
	n := len(nums)
	if n == 0 {
		return 0, false
	}
	slices.Sort(nums)
	if p <= 0 || n < 2 {
		return nums[0], true
	}
	if p >= 1 {
		return nums[n-1], true
	}
	i := float64(n-1) * p
	i0 := int(math.Floor(i))
	v0, v1 := nums[i0], nums[i0+1]
	return v0 + (v1-v0)*(i-float64(i0)), true
}

// QuantileOf is [Quantile] for a slice of numbers.
func QuantileOf(values []float64, p float64) (float64, bool) {
	return Quantile(values, p, identity)
}

// QuantileSorted returns the p-quantile of values that are *already sorted in
// ascending order*, using the same R-7 interpolation as [Quantile] but without
// copying or sorting. It does not filter NaN — the caller has asserted the input
// is sorted, and a NaN in a sorted slice is a contradiction — so a NaN in the
// interpolation window will propagate into the result.
//
// This is the variant to use inside a loop over many quantiles of the same
// series: sort once, then call QuantileSorted repeatedly.
func QuantileSorted[T any](sorted []T, p float64, value func(T) float64) (float64, bool) {
	n := len(sorted)
	if n == 0 || math.IsNaN(p) {
		return 0, false
	}
	if p <= 0 || n < 2 {
		return value(sorted[0]), true
	}
	if p >= 1 {
		return value(sorted[n-1]), true
	}
	i := float64(n-1) * p
	i0 := int(math.Floor(i))
	v0, v1 := value(sorted[i0]), value(sorted[i0+1])
	return v0 + (v1-v0)*(i-float64(i0)), true
}

// QuantileSortedOf is [QuantileSorted] for a sorted slice of numbers.
func QuantileSortedOf(sorted []float64, p float64) (float64, bool) {
	return QuantileSorted(sorted, p, identity)
}

// Rank returns the zero-based rank of each element under the given comparator,
// index-aligned with the input: the smallest element ranks 0, the next 1, and so
// on. Tied elements all take the *lowest* rank of their group, so the ranks of
// [1, 1, 3] are [0, 0, 2] — ranks are skipped after a tie rather than being
// crowded together, which keeps the maximum rank equal to n-1.
//
// The comparator must define a total order (it is used to sort), and has the
// same shape as the one slices.SortStableFunc takes, so [Ascending] and
// [Descending] can be passed directly. The result is []float64 rather than
// []int so that [RankOf] can report an undefined value's rank as NaN; ranks
// produced by Rank itself are always integral.
//
// The input slice is not modified.
func Rank[T any](values []T, compare func(a, b T) int) []float64 {
	n := len(values)
	out := make([]float64, n)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int {
		return compare(values[a], values[b])
	})
	// Walk in sorted order, advancing the rank only when an element compares
	// strictly greater than the previous one. That is what gives a run of ties
	// the lowest rank of the run while leaving the skipped ranks unused.
	r := 0
	for i, j := range idx {
		if i > 0 && compare(values[j], values[idx[i-1]]) > 0 {
			r = i
		}
		out[j] = float64(r)
	}
	return out
}

// RankOf is [Rank] for a slice of numbers, ranked ascending. NaN values are
// unranked: [Ascending] sorts them last, and RankOf then replaces their ranks
// with NaN, so the result stays index-aligned with the input without pretending
// an undefined value has a position among the defined ones.
func RankOf(values []float64) []float64 {
	return RankBy(values, identity)
}

// RankBy ranks elements ascending by a numeric accessor, with the same NaN
// handling as [RankOf].
func RankBy[T any](values []T, value func(T) float64) []float64 {
	out := Rank(values, func(a, b T) int { return Ascending(value(a), value(b)) })
	for i, d := range values {
		if !defined(value(d)) {
			out[i] = math.NaN()
		}
	}
	return out
}
