package array

import (
	"cmp"
	"math"
)

// Ascending compares two values in natural ascending order, returning -1, 0 or
// 1, and is the default comparator throughout this package.
//
// It differs from cmp.Compare in one respect that matters for plotting: NaN
// sorts *last* rather than first. d3 sorts with a comparator that pushes
// undefined values to the end so that missing data collects at the tail of a
// sorted series instead of at the head where it would be mistaken for the
// minimum; this port does the same for NaN, which is Go's stand-in for
// undefined. Two NaNs compare equal, which is what makes the order total and
// therefore safe to sort with.
func Ascending[T cmp.Ordered](a, b T) int {
	// x != x is true only for NaN, and is legal for every cmp.Ordered type.
	if a != a {
		if b != b {
			return 0
		}
		return 1
	}
	if b != b {
		return -1
	}
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Descending compares two values in natural descending order. NaN still sorts
// last — reversing the order should not drag missing data to the front — so
// Descending is not simply the negation of [Ascending].
func Descending[T cmp.Ordered](a, b T) int {
	if a != a {
		if b != b {
			return 0
		}
		return 1
	}
	if b != b {
		return -1
	}
	switch {
	case b < a:
		return -1
	case b > a:
		return 1
	}
	return 0
}

// BisectLeft returns the insertion point for x in the ascending-sorted slice a
// such that every element before the returned index is strictly less than x. If
// x is already present, the index is that of its *first* occurrence, so
// inserting there puts x ahead of its equals.
//
// On an empty slice the answer is 0; if x is NaN the answer is len(a), matching
// d3's rule that an undefined needle belongs past the end rather than at an
// arbitrary position.
func BisectLeft[T cmp.Ordered](a []T, x T) int {
	return BisectLeftRange(a, x, 0, len(a))
}

// BisectRight returns the insertion point for x in the ascending-sorted slice a
// such that every element before the returned index is less than *or equal to*
// x. If x is already present, the index is one past its last occurrence, so
// inserting there puts x behind its equals.
//
// The left/right distinction is the whole point of having both: BisectRight is
// what you want to answer "which bucket does this value fall into" for buckets
// that are half-open on the right, which is why [Bin] uses it and why d3 makes
// it the default.
func BisectRight[T cmp.Ordered](a []T, x T) int {
	return BisectRightRange(a, x, 0, len(a))
}

// Bisect is [BisectRight]. d3 exports bisectRight under the bare name bisect,
// and so does this package, because the right-hand variant is the one that
// behaves correctly when assigning values to half-open intervals.
func Bisect[T cmp.Ordered](a []T, x T) int { return BisectRight(a, x) }

// BisectLeftRange is [BisectLeft] restricted to the half-open index range
// [lo, hi). It is the primitive the other bisect functions are built from, and
// is exported because bisecting a window of a larger buffer — a viewport into a
// time series, say — is common enough to deserve not copying the slice.
func BisectLeftRange[T cmp.Ordered](a []T, x T, lo, hi int) int {
	if lo < hi {
		if x != x { // NaN needle: it orders after everything.
			return hi
		}
		for lo < hi {
			mid := int(uint(lo+hi) >> 1)
			if a[mid] < x {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
	}
	return lo
}

// BisectRightRange is [BisectRight] restricted to the half-open index range
// [lo, hi).
func BisectRightRange[T cmp.Ordered](a []T, x T, lo, hi int) int {
	if lo < hi {
		if x != x {
			return hi
		}
		for lo < hi {
			mid := int(uint(lo+hi) >> 1)
			if x < a[mid] {
				hi = mid
			} else {
				lo = mid + 1
			}
		}
	}
	return lo
}

// BisectCenter returns the index of the element in the ascending-sorted slice a
// that is *nearest* to x, rather than an insertion point. It is d3's
// bisector().center, the function behind "snap the tooltip to the closest data
// point": for a value that falls between two samples it picks whichever sample
// is closer, breaking the exact midpoint in favour of the later one.
//
// The slice must be sorted ascending and non-empty; for an empty slice the
// result is 0, which indexes nothing and should be checked for.
func BisectCenter(a []float64, x float64) int {
	return bisectCenterRange(len(a), x, 0, len(a), func(i int) float64 { return a[i] })
}

// bisectCenterRange is the shared implementation of the nearest-index search,
// parameterised by an indexed accessor so that both [BisectCenter] and
// [Bisector.Center] can use it.
func bisectCenterRange(n int, x float64, lo, hi int, at func(int) float64) int {
	if n == 0 || lo >= hi {
		return lo
	}
	i := hi - 1
	if x == x {
		i = bisectLeftBy(x, lo, hi-1, at)
	}
	if i > lo && at(i-1)-x > -(at(i)-x) {
		return i - 1
	}
	return i
}

// bisectLeftBy is BisectLeftRange over an indexed accessor.
func bisectLeftBy(x float64, lo, hi int, at func(int) float64) int {
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if at(mid) < x {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

// Bisector performs binary search over a slice of arbitrary elements against a
// needle of a possibly different type, using a caller-supplied comparator. It is
// d3's d3.bisector, and it exists because the interesting searches are not over
// bare numbers: given []Trade sorted by time, you want the index of a
// time.Time, and the elements and the needle are not the same type at all.
//
// Construct one with [NewBisector] (comparator), [BisectorBy] (accessor to an
// ordered key) or [BisectorOf] (numeric accessor, which additionally enables
// [Bisector.Center]).
//
// The type parameters are T, the slice element type, and X, the needle type.
// They are separate on purpose; when they coincide, [BisectorBy] infers both.
type Bisector[T, X any] struct {
	compare func(d T, x X) int
	// delta is the signed distance from a needle to an element, used only by
	// Center to decide which of two neighbours is nearer. It is nil for
	// bisectors built from a bare comparator, since an ordering alone cannot
	// say which of two elements is *closer* to the needle; Center then falls
	// back to Left.
	delta func(d T, x X) float64
}

// NewBisector returns a [Bisector] driven by a comparator that orders an
// element against a needle: compare(d, x) must be negative when d sorts before
// x, positive when after, zero when equivalent. The slice being searched must be
// sorted consistently with that comparator.
//
// [Bisector.Center] degrades to [Bisector.Left] for a bisector built this way,
// because a comparator says which side a value falls on but not how far; use
// [BisectorOf] when you need Center.
func NewBisector[T, X any](compare func(d T, x X) int) *Bisector[T, X] {
	return &Bisector[T, X]{compare: compare}
}

// BisectorBy returns a [Bisector] that compares elements to needles by
// projecting each element onto an ordered key — the common case of searching a
// slice of structs sorted by one of their fields.
func BisectorBy[T any, U cmp.Ordered](value func(T) U) *Bisector[T, U] {
	return &Bisector[T, U]{compare: func(d T, x U) int { return Ascending(value(d), x) }}
}

// BisectorOf returns a [Bisector] over a numeric accessor. It is [BisectorBy]
// specialized to float64, and is the only constructor that can supply the
// distance metric [Bisector.Center] needs, so use it when you want
// nearest-neighbour lookups.
func BisectorOf[T any](value func(T) float64) *Bisector[T, float64] {
	return &Bisector[T, float64]{
		compare: func(d T, x float64) int { return Ascending(value(d), x) },
		delta:   func(d T, x float64) float64 { return value(d) - x },
	}
}

// Left returns the leftmost insertion point for x in the sorted slice a.
func (b *Bisector[T, X]) Left(a []T, x X) int { return b.LeftRange(a, x, 0, len(a)) }

// Right returns the rightmost insertion point for x in the sorted slice a.
func (b *Bisector[T, X]) Right(a []T, x X) int { return b.RightRange(a, x, 0, len(a)) }

// LeftRange is [Bisector.Left] restricted to the half-open index range [lo, hi).
func (b *Bisector[T, X]) LeftRange(a []T, x X, lo, hi int) int {
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if b.compare(a[mid], x) < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

// RightRange is [Bisector.Right] restricted to the half-open index range
// [lo, hi).
func (b *Bisector[T, X]) RightRange(a []T, x X, lo, hi int) int {
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if b.compare(a[mid], x) <= 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

// Center returns the index of the element of a nearest to x. For a bisector
// built without a distance metric (see [NewBisector]) it is equivalent to
// [Bisector.Left]. For an empty slice it returns 0, which indexes nothing.
func (b *Bisector[T, X]) Center(a []T, x X) int {
	if len(a) == 0 {
		return 0
	}
	if b.delta == nil {
		return b.Left(a, x)
	}
	i := b.LeftRange(a, x, 0, len(a)-1)
	if i > 0 && b.delta(a[i-1], x) > -b.delta(a[i], x) {
		return i - 1
	}
	return i
}

// Quickselect partially sorts values in place so that the element at index k
// ends up where it would be in a fully sorted slice, every element before k
// compares less than or equal to it, and every element after compares greater
// than or equal. It returns the same slice for convenience.
//
// This is Floyd–Rivest, and it is worth having rather than just sorting because
// it is O(n) expected rather than O(n log n): finding a median or a single
// quantile of a large series does not need the other n-1 positions resolved.
// The trade-off is that the slice is left in an arbitrary order apart from the
// guarantee above, so the caller must not assume anything else about it.
//
// k is clamped into range; an empty slice is returned untouched.
func Quickselect[T any](values []T, k int, compare func(a, b T) int) []T {
	return QuickselectRange(values, k, 0, len(values)-1, compare)
}

// QuickselectOf is [Quickselect] for a slice of numbers, ordered ascending.
func QuickselectOf(values []float64, k int) []float64 {
	return Quickselect(values, k, Ascending[float64])
}

// QuickselectRange is [Quickselect] restricted to the inclusive index range
// [left, right]; elements outside that range are not moved.
func QuickselectRange[T any](values []T, k, left, right int, compare func(a, b T) int) []T {
	if len(values) == 0 {
		return values
	}
	if left < 0 {
		left = 0
	}
	if right > len(values)-1 {
		right = len(values) - 1
	}
	if k < left {
		k = left
	}
	if k > right {
		k = right
	}

	for right > left {
		// On a wide range, recurse into a sub-range chosen so that k is near
		// its centre. This is the Floyd–Rivest refinement: sampling the data to
		// guess where k lives turns the expected comparison count from 2n into
		// n + min(k, n-k) + O(sqrt(n)), which is why this beats a plain
		// quickselect on large inputs.
		if right-left > 600 {
			n := float64(right - left + 1)
			m := float64(k - left + 1)
			z := math.Log(n)
			s := 0.5 * math.Exp(2*z/3)
			sd := 0.5 * math.Sqrt(z*s*(n-s)/n)
			if m-n/2 < 0 {
				sd = -sd
			}
			newLeft := math.Max(float64(left), math.Floor(float64(k)-m*s/n+sd))
			newRight := math.Min(float64(right), math.Floor(float64(k)+(n-m)*s/n+sd))
			QuickselectRange(values, k, int(newLeft), int(newRight), compare)
		}

		t := values[k]
		i, j := left, right
		values[left], values[k] = values[k], values[left]
		if compare(values[right], t) > 0 {
			values[left], values[right] = values[right], values[left]
		}
		for i < j {
			values[i], values[j] = values[j], values[i]
			i++
			j--
			for compare(values[i], t) < 0 {
				i++
			}
			for compare(values[j], t) > 0 {
				j--
			}
		}
		if compare(values[left], t) == 0 {
			values[left], values[j] = values[j], values[left]
		} else {
			j++
			values[j], values[right] = values[right], values[j]
		}
		if j <= k {
			left = j + 1
		}
		if k <= j {
			right = j - 1
		}
	}
	return values
}
