package scale

import "math"

// Diverging is a sequential-style scale with three domain stops instead of two:
// a low end, a *pivot*, and a high end. The pivot always lands at t = 0.5, and
// each half of the domain is normalized independently onto its half of the
// interpolator.
//
// The point of the independent halves is that the pivot is usually a
// semantically fixed value — zero, the mean, last year's number — and it must
// sit at the neutral middle of the color ramp even when the data is lopsided.
// A domain of [-1, 0, 100] puts zero at the middle of the ramp; a plain
// sequential scale would put it 1% of the way along and colour almost the whole
// chart as "above average".
//
// Like [Sequential], a diverging scale has no Invert.
type Diverging[R any] struct {
	// core carries the three-stop domain and the domain transform; its own
	// range is unused. See [Sequential] for why this is shared.
	core *Continuous[float64]

	interpolator func(float64) R
	clamp        bool
	unknown      R
}

func newDiverging[R any](core *Continuous[float64], interp func(float64) R) *Diverging[R] {
	d := &Diverging[R]{core: core, interpolator: interp}
	dom := core.Domain()
	if len(dom) == 2 {
		core.SetDomain(dom[0], (dom[0]+dom[1])/2, dom[1])
	}
	return d
}

// NewDiverging returns a diverging scale with the domain [0, 0.5, 1].
func NewDiverging[R any](interp func(float64) R) *Diverging[R] {
	return newDiverging(NewLinear(), interp)
}

// NewDivergingLog returns a diverging scale with a log domain transform. As
// with [NewLog] the domain may not cross zero, which in practice means the
// pivot is a ratio of 1 rather than a difference of 0.
func NewDivergingLog[R any](interp func(float64) R) *Diverging[R] {
	return newDiverging(NewLog(), interp).SetDomain(0.1, 1, 10)
}

// NewDivergingPow returns a diverging scale with a power domain transform.
func NewDivergingPow[R any](interp func(float64) R) *Diverging[R] {
	return newDiverging(NewPow(), interp)
}

// NewDivergingSqrt returns a diverging scale with a square-root domain
// transform.
func NewDivergingSqrt[R any](interp func(float64) R) *Diverging[R] {
	return newDiverging(NewSqrt(), interp)
}

// NewDivergingSymlog returns a diverging scale with a symmetric-log domain
// transform, the usual choice for signed data that spans orders of magnitude
// on both sides of zero.
func NewDivergingSymlog[R any](interp func(float64) R) *Diverging[R] {
	return newDiverging(NewSymlog(), interp)
}

// Scale maps a domain value through the interpolator, with the pivot at 0.5.
func (d *Diverging[R]) Scale(x float64) R {
	if math.IsNaN(x) {
		return d.unknown
	}
	dom := d.core.domain
	if len(dom) < 3 {
		return d.unknown
	}
	t0, t1, t2 := d.core.tf(dom[0]), d.core.tf(dom[1]), d.core.tf(dom[2])
	var k10, k21 float64
	if t0 != t1 {
		k10 = 0.5 / (t1 - t0)
	}
	if t1 != t2 {
		k21 = 0.5 / (t2 - t1)
	}
	// s orients the "which half am I in" test so that a descending domain
	// (high, pivot, low) still picks the correct half.
	s := 1.0
	if t1 < t0 {
		s = -1
	}
	tx := d.core.tf(x)
	k := k21
	if s*tx < s*t1 {
		k = k10
	}
	t := 0.5 + (tx-t1)*k
	if d.clamp {
		t = clamp01(t)
	}
	return d.interpolator(t)
}

// Domain returns a copy of the three-element domain.
func (d *Diverging[R]) Domain() []float64 { return d.core.Domain() }

// SetDomain sets the low end, the pivot and the high end.
func (d *Diverging[R]) SetDomain(x0, x1, x2 float64) *Diverging[R] {
	d.core.SetDomain(x0, x1, x2)
	return d
}

// Interpolator returns the interpolator in force.
func (d *Diverging[R]) Interpolator() func(float64) R { return d.interpolator }

// SetInterpolator replaces the interpolator.
func (d *Diverging[R]) SetInterpolator(f func(float64) R) *Diverging[R] {
	d.interpolator = f
	return d
}

// SetRange builds the interpolator from three endpoints — low, middle, high —
// using interpolate.Piecewise semantics. See [Sequential.SetRange] for why the
// factory is explicit.
func (d *Diverging[R]) SetRange(interp Interpolator[R], r0, r1, r2 R) *Diverging[R] {
	lo := interp(r0, r1)
	hi := interp(r1, r2)
	d.interpolator = func(t float64) R {
		if t < 0.5 {
			return lo(t * 2)
		}
		return hi(t*2 - 1)
	}
	return d
}

// Clamp reports whether clamping is on.
func (d *Diverging[R]) Clamp() bool { return d.clamp }

// SetClamp turns clamping on or off.
func (d *Diverging[R]) SetClamp(c bool) *Diverging[R] { d.clamp = c; return d }

// Unknown returns the value produced for a NaN input.
func (d *Diverging[R]) Unknown() R { return d.unknown }

// SetUnknown sets the value produced for a NaN input.
func (d *Diverging[R]) SetUnknown(v R) *Diverging[R] { d.unknown = v; return d }

// Ticks returns approximately count ticks across the full domain (ignoring the
// pivot, which is not necessarily a round number).
func (d *Diverging[R]) Ticks(count int) []float64 { return d.core.Ticks(count) }

// TickFormat returns a formatter for the values from [Diverging.Ticks].
func (d *Diverging[R]) TickFormat(count int) func(float64) string { return d.core.TickFormat(count) }

// Nice extends the outer ends of the domain to round values, leaving the pivot
// where it is — moving the pivot would change the meaning of the scale.
func (d *Diverging[R]) Nice(count int) *Diverging[R] { d.core.Nice(count); return d }

// Copy returns an independent copy of the scale.
func (d *Diverging[R]) Copy() *Diverging[R] {
	c := *d
	c.core = d.core.Copy()
	return &c
}

// Exponent returns the power transform's exponent.
func (d *Diverging[R]) Exponent() float64 { return d.core.Exponent() }

// SetExponent sets the power transform's exponent (meaningful for
// [NewDivergingPow] and [NewDivergingSqrt]).
func (d *Diverging[R]) SetExponent(e float64) *Diverging[R] { d.core.SetExponent(e); return d }

// Base returns the log transform's base.
func (d *Diverging[R]) Base() float64 { return d.core.Base() }

// SetBase sets the log transform's base (meaningful for [NewDivergingLog]).
func (d *Diverging[R]) SetBase(b float64) *Diverging[R] { d.core.SetBase(b); return d }

// Constant returns the symlog transform's constant.
func (d *Diverging[R]) Constant() float64 { return d.core.Constant() }

// SetConstant sets the symlog transform's constant (meaningful for
// [NewDivergingSymlog]).
func (d *Diverging[R]) SetConstant(c float64) *Diverging[R] { d.core.SetConstant(c); return d }
