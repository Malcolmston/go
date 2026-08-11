package scale

import (
	"math"
	"sort"
)

// The three scales in this file all map a continuous domain onto a discrete
// range, and they are the trio people most often reach for by the wrong name.
// The difference is entirely in where the cut points come from:
//
//   - [Quantize] slices the DOMAIN into equal-width buckets. Given [0, 100] and
//     four colors, the cuts are 25/50/75 regardless of what the data looks
//     like. Buckets can be empty or wildly unequal in population.
//   - [Quantile] slices the DATA so that each bucket holds the same NUMBER of
//     observations. You give it every value, not an extent; the cuts land at
//     the quartiles (for four buckets) of the actual distribution, so the
//     buckets are equally populated and unequally wide.
//   - [Threshold] uses the cuts YOU supply. Nothing is derived; it is the right
//     tool when the breakpoints are external facts (freezing at 0 °C, the
//     poverty line, an SLA at 200 ms).
//
// All three answer InvertExtent, which recovers the domain interval that maps
// to a given range value — that is how a legend draws its swatch labels.

// ---------------------------------------------------------------------------
// quantize
// ---------------------------------------------------------------------------

// Quantize maps a continuous domain onto a discrete range by cutting the domain
// into len(range) equal-width slices. See the file comment for how it differs
// from [Quantile] and [Threshold].
type Quantize[R comparable] struct {
	x0, x1     float64
	thresholds []float64
	rangeVals  []R
	unknown    R
}

// NewQuantize returns a quantize scale with the unit domain and the given
// range. With one range value the scale is constant; with n it has n-1
// internal thresholds.
func NewQuantize[R comparable](rng ...R) *Quantize[R] {
	q := &Quantize[R]{x0: 0, x1: 1, rangeVals: append([]R(nil), rng...)}
	q.rescale()
	return q
}

func (q *Quantize[R]) rescale() {
	n := len(q.rangeVals) - 1
	if n < 0 {
		n = 0
	}
	q.thresholds = make([]float64, n)
	for i := 0; i < n; i++ {
		// The d3 formula, kept verbatim: it interpolates the i-th cut between
		// x0 and x1 without accumulating rounding the way x0 + i*(x1-x0)/(n+1)
		// would.
		fi := float64(i)
		fn := float64(n)
		q.thresholds[i] = ((fi+1)*q.x1 - (fi-fn)*q.x0) / (fn + 1)
	}
}

// Scale maps a value to its bucket.
func (q *Quantize[R]) Scale(x float64) R {
	if math.IsNaN(x) || len(q.rangeVals) == 0 {
		return q.unknown
	}
	i := bisectRight(q.thresholds, x, 0, len(q.thresholds))
	return q.rangeVals[i]
}

// InvertExtent returns the domain interval [lo, hi) that maps to y, or NaN,NaN
// when y is not in the range.
func (q *Quantize[R]) InvertExtent(y R) (float64, float64) {
	i := indexOf(q.rangeVals, y)
	n := len(q.thresholds)
	switch {
	case i < 0:
		return math.NaN(), math.NaN()
	case n == 0:
		return q.x0, q.x1
	case i < 1:
		return q.x0, q.thresholds[0]
	case i >= n:
		return q.thresholds[n-1], q.x1
	default:
		return q.thresholds[i-1], q.thresholds[i]
	}
}

// Domain returns the two-element domain extent.
func (q *Quantize[R]) Domain() []float64 { return []float64{q.x0, q.x1} }

// SetDomain sets the domain extent.
func (q *Quantize[R]) SetDomain(x0, x1 float64) *Quantize[R] {
	q.x0, q.x1 = x0, x1
	q.rescale()
	return q
}

// Range returns a copy of the range.
func (q *Quantize[R]) Range() []R { return append([]R(nil), q.rangeVals...) }

// SetRange sets the range; the number of buckets follows from its length.
func (q *Quantize[R]) SetRange(rng ...R) *Quantize[R] {
	q.rangeVals = append([]R(nil), rng...)
	q.rescale()
	return q
}

// Thresholds returns a copy of the computed cut points.
func (q *Quantize[R]) Thresholds() []float64 { return append([]float64(nil), q.thresholds...) }

// Unknown returns the value produced for a NaN input.
func (q *Quantize[R]) Unknown() R { return q.unknown }

// SetUnknown sets the value produced for a NaN input.
func (q *Quantize[R]) SetUnknown(v R) *Quantize[R] { q.unknown = v; return q }

// Ticks returns approximately count ticks over the domain.
func (q *Quantize[R]) Ticks(count int) []float64 {
	return NewLinear().SetDomain(q.x0, q.x1).Ticks(count)
}

// TickFormat returns a formatter for the values from [Quantize.Ticks].
func (q *Quantize[R]) TickFormat(count int) func(float64) string {
	return NewLinear().SetDomain(q.x0, q.x1).TickFormat(count)
}

// Nice extends the domain outward to round values.
func (q *Quantize[R]) Nice(count int) *Quantize[R] {
	d := NewLinear().SetDomain(q.x0, q.x1).Nice(count).Domain()
	return q.SetDomain(d[0], d[1])
}

// Copy returns an independent copy of the scale.
func (q *Quantize[R]) Copy() *Quantize[R] {
	c := *q
	c.rangeVals = append([]R(nil), q.rangeVals...)
	c.thresholds = append([]float64(nil), q.thresholds...)
	return &c
}

// ---------------------------------------------------------------------------
// quantile
// ---------------------------------------------------------------------------

// Quantile maps a sample of observations onto a discrete range so that each
// range value receives an equal *number* of observations. Its domain is the
// data, not an extent. See the file comment for the contrast with [Quantize].
type Quantile[R comparable] struct {
	domain     []float64 // sorted ascending, NaN-free
	thresholds []float64
	rangeVals  []R
	unknown    R
}

// NewQuantile returns an empty quantile scale with the given range.
func NewQuantile[R comparable](rng ...R) *Quantile[R] {
	q := &Quantile[R]{rangeVals: append([]R(nil), rng...)}
	q.rescale()
	return q
}

func (q *Quantile[R]) rescale() {
	n := len(q.rangeVals)
	if n < 1 {
		n = 1
	}
	q.thresholds = make([]float64, n-1)
	for i := 1; i < n; i++ {
		q.thresholds[i-1] = quantileSorted(q.domain, float64(i)/float64(n))
	}
}

// Scale maps a value to its bucket.
func (q *Quantile[R]) Scale(x float64) R {
	if math.IsNaN(x) || len(q.rangeVals) == 0 {
		return q.unknown
	}
	i := bisectRight(q.thresholds, x, 0, len(q.thresholds))
	return q.rangeVals[i]
}

// InvertExtent returns the domain interval that maps to y, or NaN,NaN when y is
// not in the range. Unlike [Quantize.InvertExtent] the ends come from the
// observed data, since that is all a quantile scale knows about the domain.
func (q *Quantile[R]) InvertExtent(y R) (float64, float64) {
	i := indexOf(q.rangeVals, y)
	if i < 0 || len(q.domain) == 0 {
		return math.NaN(), math.NaN()
	}
	lo := q.domain[0]
	if i > 0 {
		lo = q.thresholds[i-1]
	}
	hi := q.domain[len(q.domain)-1]
	if i < len(q.thresholds) {
		hi = q.thresholds[i]
	}
	return lo, hi
}

// Domain returns a copy of the sorted observations.
func (q *Quantile[R]) Domain() []float64 { return append([]float64(nil), q.domain...) }

// SetDomain sets the observations, dropping NaNs and sorting ascending.
func (q *Quantile[R]) SetDomain(values ...float64) *Quantile[R] {
	q.domain = q.domain[:0]
	for _, v := range values {
		if !math.IsNaN(v) {
			q.domain = append(q.domain, v)
		}
	}
	sort.Float64s(q.domain)
	q.rescale()
	return q
}

// Range returns a copy of the range.
func (q *Quantile[R]) Range() []R { return append([]R(nil), q.rangeVals...) }

// SetRange sets the range; the number of quantiles follows from its length.
func (q *Quantile[R]) SetRange(rng ...R) *Quantile[R] {
	q.rangeVals = append([]R(nil), rng...)
	q.rescale()
	return q
}

// Quantiles returns a copy of the computed cut points.
func (q *Quantile[R]) Quantiles() []float64 { return append([]float64(nil), q.thresholds...) }

// Unknown returns the value produced for a NaN input.
func (q *Quantile[R]) Unknown() R { return q.unknown }

// SetUnknown sets the value produced for a NaN input.
func (q *Quantile[R]) SetUnknown(v R) *Quantile[R] { q.unknown = v; return q }

// Copy returns an independent copy of the scale.
func (q *Quantile[R]) Copy() *Quantile[R] {
	c := *q
	c.domain = append([]float64(nil), q.domain...)
	c.rangeVals = append([]R(nil), q.rangeVals...)
	c.thresholds = append([]float64(nil), q.thresholds...)
	return &c
}

// quantileSorted is the R-7 quantile of an already-sorted sample: it indexes
// (n-1)·p and linearly interpolates between the neighbouring order statistics.
// R-7 is the definition d3, NumPy and R's default all use, so the cut points
// here line up with what an analyst computes elsewhere.
func quantileSorted(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return math.NaN()
	}
	if p <= 0 || n < 2 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[n-1]
	}
	i := float64(n-1) * p
	i0 := math.Floor(i)
	v0 := sorted[int(i0)]
	v1 := sorted[int(i0)+1]
	return v0 + (v1-v0)*(i-i0)
}

// ---------------------------------------------------------------------------
// threshold
// ---------------------------------------------------------------------------

// Threshold maps a continuous domain onto a discrete range using cut points you
// supply. The domain holds the n-1 cuts for an n-value range, and the cuts are
// exclusive on the left: a value equal to a cut falls in the bucket ABOVE it.
// See the file comment for the contrast with [Quantize] and [Quantile].
type Threshold[R comparable] struct {
	domain    []float64
	rangeVals []R
	unknown   R
}

// NewThreshold returns a threshold scale with the given cut points and range.
// Callers are responsible for supplying the cuts in ascending order — bisection
// on an unsorted domain is meaningless — and for making len(rng) == len(cuts)+1.
func NewThreshold[R comparable](cuts []float64, rng ...R) *Threshold[R] {
	return &Threshold[R]{
		domain:    append([]float64(nil), cuts...),
		rangeVals: append([]R(nil), rng...),
	}
}

func (t *Threshold[R]) n() int {
	n := len(t.domain)
	if m := len(t.rangeVals) - 1; m < n {
		n = m
	}
	if n < 0 {
		n = 0
	}
	return n
}

// Scale maps a value to its bucket.
func (t *Threshold[R]) Scale(x float64) R {
	if math.IsNaN(x) || len(t.rangeVals) == 0 {
		return t.unknown
	}
	return t.rangeVals[bisectRight(t.domain, x, 0, t.n())]
}

// InvertExtent returns the domain interval that maps to y. The lower bound of
// the first bucket and the upper bound of the last are unbounded, and are
// reported as -Inf and +Inf.
func (t *Threshold[R]) InvertExtent(y R) (float64, float64) {
	i := indexOf(t.rangeVals, y)
	if i < 0 {
		return math.NaN(), math.NaN()
	}
	lo := math.Inf(-1)
	if i-1 >= 0 && i-1 < len(t.domain) {
		lo = t.domain[i-1]
	}
	hi := math.Inf(1)
	if i >= 0 && i < len(t.domain) {
		hi = t.domain[i]
	}
	return lo, hi
}

// Domain returns a copy of the cut points.
func (t *Threshold[R]) Domain() []float64 { return append([]float64(nil), t.domain...) }

// SetDomain sets the cut points, which must be ascending.
func (t *Threshold[R]) SetDomain(cuts ...float64) *Threshold[R] {
	t.domain = append([]float64(nil), cuts...)
	return t
}

// Range returns a copy of the range.
func (t *Threshold[R]) Range() []R { return append([]R(nil), t.rangeVals...) }

// SetRange sets the range.
func (t *Threshold[R]) SetRange(rng ...R) *Threshold[R] {
	t.rangeVals = append([]R(nil), rng...)
	return t
}

// Unknown returns the value produced for a NaN input.
func (t *Threshold[R]) Unknown() R { return t.unknown }

// SetUnknown sets the value produced for a NaN input.
func (t *Threshold[R]) SetUnknown(v R) *Threshold[R] { t.unknown = v; return t }

// Copy returns an independent copy of the scale.
func (t *Threshold[R]) Copy() *Threshold[R] {
	c := *t
	c.domain = append([]float64(nil), t.domain...)
	c.rangeVals = append([]R(nil), t.rangeVals...)
	return &c
}

func indexOf[T comparable](s []T, v T) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
