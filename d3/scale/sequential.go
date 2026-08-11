package scale

import (
	"math"
	"sort"
)

// Sequential is a continuous scale whose output side is a single interpolator
// rather than a pair of endpoints. Where a [Continuous] asks "which two range
// values do I blend, and how far between them am I?", a sequential scale asks
// only "how far along the domain am I?" and hands that t to a function.
//
// That shape exists for color. A perceptually sensible color ramp is a curve
// through a color space, not a straight line between two colors, so d3 ships
// ramps as functions of t and sequential scales are the adapter that turns a
// data value into that t. Everything else — clamping, the log/pow/symlog
// variants, ticks — follows the continuous scale rules.
//
// Because the range is a function, a sequential scale has no Invert.
type Sequential[R any] struct {
	// core carries the domain and the domain transform. Its own range is never
	// used; reusing [Continuous] here keeps one implementation of the log,
	// pow and symlog transforms and of Nice/Ticks/TickFormat.
	core *Continuous[float64]

	interpolator func(float64) R
	clamp        bool
	unknown      R
}

func newSequential[R any](core *Continuous[float64], interp func(float64) R) *Sequential[R] {
	return &Sequential[R]{core: core, interpolator: interp}
}

// NewSequential returns a sequential scale with the unit domain and the given
// interpolator.
func NewSequential[R any](interp func(float64) R) *Sequential[R] {
	return newSequential(NewLinear(), interp)
}

// NewSequentialLog returns a sequential scale with a log domain transform and
// the default domain [1, 10]. As with [NewLog] the domain must not cross zero.
func NewSequentialLog[R any](interp func(float64) R) *Sequential[R] {
	return newSequential(NewLog(), interp)
}

// NewSequentialPow returns a sequential scale with a power domain transform.
func NewSequentialPow[R any](interp func(float64) R) *Sequential[R] {
	return newSequential(NewPow(), interp)
}

// NewSequentialSqrt returns a sequential scale with a square-root domain
// transform.
func NewSequentialSqrt[R any](interp func(float64) R) *Sequential[R] {
	return newSequential(NewSqrt(), interp)
}

// NewSequentialSymlog returns a sequential scale with a symmetric-log domain
// transform, which unlike the log variant tolerates zero and negative values.
func NewSequentialSymlog[R any](interp func(float64) R) *Sequential[R] {
	return newSequential(NewSymlog(), interp)
}

// Scale maps a domain value through the interpolator.
func (s *Sequential[R]) Scale(x float64) R {
	if math.IsNaN(x) {
		return s.unknown
	}
	d := s.core.domain
	t0, t1 := s.core.tf(d[0]), s.core.tf(d[len(d)-1])
	var t float64
	if t0 == t1 {
		t = 0.5
	} else {
		t = (s.core.tf(x) - t0) / (t1 - t0)
	}
	if s.clamp {
		t = clamp01(t)
	}
	return s.interpolator(t)
}

// Domain returns a copy of the two-element domain.
func (s *Sequential[R]) Domain() []float64 { return s.core.Domain() }

// SetDomain sets the domain and returns the scale.
func (s *Sequential[R]) SetDomain(x0, x1 float64) *Sequential[R] {
	s.core.SetDomain(x0, x1)
	return s
}

// Interpolator returns the interpolator in force.
func (s *Sequential[R]) Interpolator() func(float64) R { return s.interpolator }

// SetInterpolator replaces the interpolator and returns the scale.
func (s *Sequential[R]) SetInterpolator(f func(float64) R) *Sequential[R] {
	s.interpolator = f
	return s
}

// SetRange builds the interpolator from two range endpoints, which is d3's
// `range([r0, r1])`. Go cannot infer how to blend an arbitrary R, so the
// two-argument interpolator factory is explicit — pass interpolate.Number for
// numbers or interpolate.RGB for colors.
func (s *Sequential[R]) SetRange(interp Interpolator[R], r0, r1 R) *Sequential[R] {
	s.interpolator = interp(r0, r1)
	return s
}

// Clamp reports whether clamping is on.
func (s *Sequential[R]) Clamp() bool { return s.clamp }

// SetClamp turns clamping on or off. Off (the default) the scale extrapolates,
// so an out-of-domain value asks the interpolator for a t outside [0, 1] — most
// color interpolators will happily extrapolate into nonsense, which is why
// clamping is nearly always what you want for a color ramp.
func (s *Sequential[R]) SetClamp(c bool) *Sequential[R] { s.clamp = c; return s }

// Unknown returns the value produced for a NaN input.
func (s *Sequential[R]) Unknown() R { return s.unknown }

// SetUnknown sets the value produced for a NaN input.
func (s *Sequential[R]) SetUnknown(v R) *Sequential[R] { s.unknown = v; return s }

// Ticks returns approximately count domain ticks.
func (s *Sequential[R]) Ticks(count int) []float64 { return s.core.Ticks(count) }

// TickFormat returns a formatter for the values from [Sequential.Ticks].
func (s *Sequential[R]) TickFormat(count int) func(float64) string { return s.core.TickFormat(count) }

// Nice extends the domain outward to round values.
func (s *Sequential[R]) Nice(count int) *Sequential[R] { s.core.Nice(count); return s }

// Copy returns an independent copy of the scale. The interpolator is shared, as
// it is assumed to be a pure function.
func (s *Sequential[R]) Copy() *Sequential[R] {
	c := *s
	c.core = s.core.Copy()
	return &c
}

// ---------------------------------------------------------------------------
// sequential quantile
// ---------------------------------------------------------------------------

// SequentialQuantile is a sequential scale whose domain is the *data itself*
// rather than an extent. It sorts the values it is given and maps each one to
// its rank position, so the output is uniform over the interpolator no matter
// how skewed the input distribution is.
//
// That makes it the right choice when a plain sequential color ramp washes out
// because a handful of outliers own most of the domain: here every quantile of
// the data gets an equal share of the color ramp. The cost is that the mapping
// is no longer proportional to value — two adjacent colors can be a cent or a
// million apart — so a legend must be labelled with [SequentialQuantile.Quantiles].
type SequentialQuantile[R any] struct {
	domain       []float64 // sorted ascending, NaN-free
	interpolator func(float64) R
	unknown      R
}

// NewSequentialQuantile returns an empty sequential quantile scale.
func NewSequentialQuantile[R any](interp func(float64) R) *SequentialQuantile[R] {
	return &SequentialQuantile[R]{interpolator: interp}
}

// Scale maps a value to its rank position in the domain and through the
// interpolator.
func (s *SequentialQuantile[R]) Scale(x float64) R {
	if math.IsNaN(x) || len(s.domain) < 2 {
		return s.unknown
	}
	i := bisectRight(s.domain, x, 1, len(s.domain))
	return s.interpolator(float64(i-1) / float64(len(s.domain)-1))
}

// Domain returns a copy of the sorted domain.
func (s *SequentialQuantile[R]) Domain() []float64 {
	return append([]float64(nil), s.domain...)
}

// SetDomain sets the domain to the given observations, dropping NaNs and
// sorting ascending.
func (s *SequentialQuantile[R]) SetDomain(values ...float64) *SequentialQuantile[R] {
	s.domain = s.domain[:0]
	for _, v := range values {
		if !math.IsNaN(v) {
			s.domain = append(s.domain, v)
		}
	}
	sort.Float64s(s.domain)
	return s
}

// Quantiles returns n+1 evenly spaced quantiles of the domain, which are the
// breakpoints a legend for this scale should be labelled with.
func (s *SequentialQuantile[R]) Quantiles(n int) []float64 {
	out := make([]float64, n+1)
	for i := 0; i <= n; i++ {
		out[i] = quantileSorted(s.domain, float64(i)/float64(n))
	}
	return out
}

// Interpolator returns the interpolator in force.
func (s *SequentialQuantile[R]) Interpolator() func(float64) R { return s.interpolator }

// SetInterpolator replaces the interpolator.
func (s *SequentialQuantile[R]) SetInterpolator(f func(float64) R) *SequentialQuantile[R] {
	s.interpolator = f
	return s
}

// SetRange builds the interpolator from two endpoints. See
// [Sequential.SetRange] for why the factory is explicit.
func (s *SequentialQuantile[R]) SetRange(interp Interpolator[R], r0, r1 R) *SequentialQuantile[R] {
	s.interpolator = interp(r0, r1)
	return s
}

// Unknown returns the value produced for a NaN input or an empty domain.
func (s *SequentialQuantile[R]) Unknown() R { return s.unknown }

// SetUnknown sets the value produced for a NaN input.
func (s *SequentialQuantile[R]) SetUnknown(v R) *SequentialQuantile[R] { s.unknown = v; return s }

// Copy returns an independent copy of the scale.
func (s *SequentialQuantile[R]) Copy() *SequentialQuantile[R] {
	c := *s
	c.domain = append([]float64(nil), s.domain...)
	return &c
}

func clamp01(t float64) float64 { return math.Max(0, math.Min(1, t)) }

// Exponent returns the power transform's exponent.
func (s *Sequential[R]) Exponent() float64 { return s.core.Exponent() }

// SetExponent sets the power transform's exponent (meaningful for
// [NewSequentialPow] and [NewSequentialSqrt]).
func (s *Sequential[R]) SetExponent(e float64) *Sequential[R] { s.core.SetExponent(e); return s }

// Base returns the log transform's base.
func (s *Sequential[R]) Base() float64 { return s.core.Base() }

// SetBase sets the log transform's base (meaningful for [NewSequentialLog]).
func (s *Sequential[R]) SetBase(b float64) *Sequential[R] { s.core.SetBase(b); return s }

// Constant returns the symlog transform's constant.
func (s *Sequential[R]) Constant() float64 { return s.core.Constant() }

// SetConstant sets the symlog transform's constant (meaningful for
// [NewSequentialSymlog]).
func (s *Sequential[R]) SetConstant(c float64) *Sequential[R] { s.core.SetConstant(c); return s }
