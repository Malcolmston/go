package array

import "math"

// Bin is one bucket of a histogram: the elements that fell into it, together
// with the half-open interval [X0, X1) it covers. The last bin of a histogram is
// closed on the right — it includes the domain's maximum — because otherwise the
// largest observation would have nowhere to go.
type Bin[T any] struct {
	X0, X1 float64
	Values []T
}

// Len returns the number of elements in the bin, the quantity a histogram's bar
// height is normally proportional to.
func (b Bin[T]) Len() int { return len(b.Values) }

// ThresholdFunc computes how many bins a series should be divided into, given
// its defined values and the domain those values span. The three
// implementations in this package — [ThresholdSturges], [ThresholdScott] and
// [ThresholdFreedmanDiaconis] — are the standard rules; a custom one may ignore
// its arguments entirely and return a constant.
//
// A threshold function returns a *count*, which the binner then converts into
// evenly spaced round thresholds via [Ticks]. To specify the cut points
// explicitly instead, use [Binner.ThresholdValues].
type ThresholdFunc func(values []float64, min, max float64) int

// ThresholdSturges is Sturges' formula, ceil(log2(n)) + 1, and is the default
// rule. It assumes roughly normal data and gets visibly too coarse for large n,
// but it is d3's default and R's, so it is what a reader expects an unspecified
// histogram to look like.
func ThresholdSturges(values []float64, min, max float64) int {
	n := CountOf(values)
	if n <= 0 {
		return 1
	}
	c := int(math.Ceil(math.Log(float64(n))/math.Ln2)) + 1
	if c < 1 {
		return 1
	}
	return c
}

// ThresholdScott is Scott's normal reference rule, which chooses a bin width of
// 3.49·σ·n^(-1/3). It adapts the bin width to the spread of the data rather than
// only to its count, so it degrades more gracefully than [ThresholdSturges] on
// large samples — but being built on the standard deviation, it is sensitive to
// outliers.
//
// It falls back to a single bin when the data has no spread (every value equal,
// or fewer than two values), since any bin width would then be zero.
func ThresholdScott(values []float64, min, max float64) int {
	n := CountOf(values)
	d, ok := DeviationOf(values)
	if n == 0 || !ok || d == 0 {
		return 1
	}
	c := math.Ceil((max - min) * math.Cbrt(float64(n)) / (3.49 * d))
	if !(c >= 1) {
		return 1
	}
	return clampCount(c)
}

// ThresholdFreedmanDiaconis is the Freedman–Diaconis rule, which sizes bins by
// the interquartile range instead of the standard deviation: 2·IQR·n^(-1/3).
// Because the IQR ignores the tails, it is the robust choice for skewed or
// outlier-heavy data, which is why it is usually the right answer when
// [ThresholdScott] produces one enormous bar and a lot of empty space.
//
// It falls back to a single bin when the interquartile range is zero.
func ThresholdFreedmanDiaconis(values []float64, min, max float64) int {
	n := CountOf(values)
	q3, ok3 := QuantileOf(values, 0.75)
	q1, ok1 := QuantileOf(values, 0.25)
	if n == 0 || !ok1 || !ok3 {
		return 1
	}
	d := q3 - q1
	if d == 0 {
		return 1
	}
	c := math.Ceil((max - min) / (2 * d * math.Pow(float64(n), -1.0/3)))
	if !(c >= 1) {
		return 1
	}
	return clampCount(c)
}

// clampCount converts a threshold rule's floating-point answer into a sane bin
// count. The rules can return absurd values on degenerate data — a near-zero
// deviation makes Scott's rule ask for billions of bins — and allocating that
// many empty slices helps nobody.
func clampCount(c float64) int {
	const maxBins = 1 << 20
	if math.IsNaN(c) || c < 1 {
		return 1
	}
	if c > maxBins {
		return maxBins
	}
	return int(c)
}

// Binner is a configurable histogram generator: it computes how to divide a
// series into buckets and then assigns the data to them. It is d3.bin, and like
// d3's generators it is configured with chained setters and then applied:
//
//	bins := array.NewBin(func(t Trade) float64 { return t.Price }).
//		DomainRange(0, 100).
//		ThresholdCount(20).
//		Apply(trades)
//
// A Binner is a value you configure once and apply many times; the setters
// mutate and return the receiver, so a Binner shared across goroutines must not
// be reconfigured concurrently. [Binner.Apply] itself does not mutate the
// Binner and is safe to call concurrently.
type Binner[T any] struct {
	value      func(T) float64
	domain     func(values []float64) (min, max float64, ok bool)
	defDomain  bool // domain is still the default (extent); see Apply
	thresholds ThresholdFunc
	fixed      []float64
}

// NewBin returns a [Binner] over elements of type T, reading each element's
// numeric value through the given accessor. By default the domain is the extent
// of the data and the bin count comes from [ThresholdSturges].
func NewBin[T any](value func(T) float64) *Binner[T] {
	return &Binner[T]{
		value:      value,
		domain:     ExtentOf,
		defDomain:  true,
		thresholds: ThresholdSturges,
	}
}

// NewBinOf returns a [Binner] over a slice of numbers.
func NewBinOf() *Binner[float64] { return NewBin(identity) }

// Value sets the accessor that reads each element's numeric value.
func (b *Binner[T]) Value(value func(T) float64) *Binner[T] {
	b.value = value
	return b
}

// Domain sets a function computing the [min, max] interval the bins must cover
// from the data's values; it should return false when there is no domain, in
// which case a single empty bin over [0, 0] results. Data outside the domain is
// discarded rather than clamped.
//
// Setting a custom domain turns off the automatic widening described on
// [Binner.Apply].
func (b *Binner[T]) Domain(domain func(values []float64) (min, max float64, ok bool)) *Binner[T] {
	b.domain = domain
	b.defDomain = false
	return b
}

// DomainRange fixes the domain to the constant interval [min, max]. This is the
// setting to use when several histograms must share an axis: without it each
// one takes its domain from its own data and their bars stop being comparable.
func (b *Binner[T]) DomainRange(min, max float64) *Binner[T] {
	return b.Domain(func([]float64) (float64, float64, bool) { return min, max, true })
}

// Thresholds sets the rule that decides how many bins to use. The count is a
// suggestion: the actual thresholds are chosen by [Ticks] so that bin
// boundaries are round numbers, which usually yields a slightly different count.
func (b *Binner[T]) Thresholds(t ThresholdFunc) *Binner[T] {
	b.thresholds = t
	b.fixed = nil
	return b
}

// ThresholdCount asks for approximately n bins, with boundaries chosen by
// [Ticks]. It is shorthand for a [ThresholdFunc] returning a constant.
func (b *Binner[T]) ThresholdCount(n int) *Binner[T] {
	return b.Thresholds(func([]float64, float64, float64) int { return n })
}

// ThresholdValues sets the bin boundaries explicitly: the given values become
// the interior cut points, so n thresholds produce n+1 bins (thresholds outside
// the domain are dropped first). The values must be sorted ascending.
//
// Use this when the buckets are meaningful rather than statistical — age
// brackets, tax bands, log-spaced decades — since nothing about them will be
// rounded or second-guessed.
func (b *Binner[T]) ThresholdValues(thresholds []float64) *Binner[T] {
	b.fixed = append([]float64(nil), thresholds...)
	return b
}

// Apply bins the data and returns the bins in ascending order of X0. Elements
// whose value is NaN, or falls outside the domain, are dropped; every other
// element appears in exactly one bin, in input order within that bin.
//
// When the domain is the default (the extent of the data) and the thresholds
// come from a count rather than from [Binner.ThresholdValues], the domain is
// first widened to round numbers with [NiceTicks], and then — if the last
// threshold would coincide with the maximum observation, leaving it in a
// zero-width final bin — the upper bound is pushed out by one more tick. That is
// what keeps every bin the same width, which is the only thing that makes bar
// *areas* comparable and therefore the only thing that makes a histogram honest.
// A domain set explicitly is never adjusted.
func (b *Binner[T]) Apply(data []T) []Bin[T] {
	n := len(data)
	values := make([]float64, n)
	for i, d := range data {
		values[i] = b.value(d)
	}

	x0, x1, ok := b.domain(values)
	if !ok || math.IsNaN(x0) || math.IsNaN(x1) {
		return []Bin[T]{{X0: 0, X1: 0}}
	}

	var tz []float64
	if b.fixed != nil {
		tz = append([]float64(nil), b.fixed...)
	} else {
		max := x1
		tn := b.thresholds(values, x0, x1)
		if b.defDomain {
			x0, x1 = NiceTicks(x0, x1, tn)
		}
		tz = Ticks(x0, x1, tn)

		// A final threshold sitting exactly on the upper bound would create a
		// zero-width bin. If the domain is ours to adjust and that bound is the
		// data maximum, extend it by one increment so all bins stay equal
		// width; otherwise just drop the offending threshold.
		if len(tz) > 0 && tz[len(tz)-1] >= x1 {
			if max >= x1 && b.defDomain {
				step := TickIncrement(x0, x1, tn)
				if !math.IsInf(step, 0) && !math.IsNaN(step) {
					if step > 0 {
						x1 = (math.Floor(x1/step) + 1) * step
					} else if step < 0 {
						x1 = (math.Ceil(x1*-step) + 1) / -step
					}
				}
			} else {
				tz = tz[:len(tz)-1]
			}
		}
	}

	// Discard thresholds outside the domain; they would produce empty bins
	// hanging off either end of the axis.
	a, bEnd := 0, len(tz)
	for a < bEnd && tz[a] <= x0 {
		a++
	}
	for bEnd > a && tz[bEnd-1] > x1 {
		bEnd--
	}
	tz = tz[a:bEnd]
	m := len(tz)

	bins := make([]Bin[T], m+1)
	for i := range bins {
		if i > 0 {
			bins[i].X0 = tz[i-1]
		} else {
			bins[i].X0 = x0
		}
		if i < m {
			bins[i].X1 = tz[i]
		} else {
			bins[i].X1 = x1
		}
	}

	for i, d := range data {
		x := values[i]
		if math.IsNaN(x) || x < x0 || x > x1 {
			continue
		}
		// BisectRight, not left: a value equal to a threshold belongs to the
		// bin that threshold opens, so bins are half-open on the right.
		j := 0
		if m > 0 {
			j = BisectRightRange(tz, x, 0, m)
		}
		bins[j].Values = append(bins[j].Values, d)
	}
	return bins
}
