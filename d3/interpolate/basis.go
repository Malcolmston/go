package interpolate

import "math"

// basisFn evaluates one span of a uniform cubic B-spline.
//
// t1 is the local parameter within the span and v0..v3 are the four control
// values influencing it. The coefficients are the standard uniform cubic
// B-spline basis matrix divided by 6; they are written out inline rather than
// as a matrix multiply because this runs on every evaluation of every basis
// interpolator.
//
// Note that a B-spline approximates its control points rather than passing
// through them — the curve at a control point is the weighted average of it and
// its two neighbors, (1, 4, 1)/6. That is precisely why [Basis] adds reflected
// phantom controls at its ends to pin the endpoints, and why [BasisClosed],
// which has no ends, does not.
func basisFn(t1, v0, v1, v2, v3 float64) float64 {
	t2 := t1 * t1
	t3 := t2 * t1
	return ((1-3*t1+3*t2-t3)*v0 +
		(4-6*t2+3*t3)*v1 +
		(1+3*t1+3*t2-3*t3)*v2 +
		t3*v3) / 6
}

// Basis returns a uniform cubic B-spline through the given values, evaluated
// over t in [0, 1].
//
// Where [Piecewise] with [Number] gives a polyline — continuous but with a
// visible kink at every stop — a B-spline is smooth in its first and second
// derivatives, so a color ramp or an easing curve built from it has no
// perceptible seams. That smoothness is the reason to prefer it for multi-stop
// color scales, and it is what d3's interpolateBasis provides.
//
// The trade-off is that the curve does not pass through the interior values; it
// is pulled toward them. Only the first and last are hit exactly, and that is
// arranged deliberately: the two phantom control points the end segments need
// are *reflections* of their neighbors (2*v0 - v1 before the start, and the
// mirror after the end), not duplicates. Reflection is what pins the curve —
// the basis weights at a segment start are (1, 4, 1)/6, so with the reflected
// point they collapse to exactly v0, whereas duplicating would land a sixth of
// the way toward v1. A useful side effect is that the spline reproduces a
// linear control sequence exactly, end segments included.
//
// So f(0) == values[0] and f(1) == values[len-1] exactly, while f at an interior
// knot is near, not at, the corresponding value.
//
// t is clamped to [0, 1] here, unlike most of this package. A cubic spline has
// no meaningful continuation past its control polygon — extrapolating a cubic
// diverges cubically — so returning the endpoint is the only sane answer, and
// it is what d3 does. An empty slice panics; a single value gives a constant.
func Basis(values []float64) func(t float64) float64 {
	v := append([]float64(nil), values...)
	n := len(v)
	if n == 0 {
		panic("interpolate: Basis requires at least one value")
	}
	if n == 1 {
		x := v[0]
		return func(float64) float64 { return x }
	}
	return func(t float64) float64 {
		switch {
		case math.IsNaN(t) || t <= 0:
			return v[0]
		case t >= 1:
			return v[n-1]
		}
		i := int(t * float64(n-1))
		if i >= n-1 {
			i = n - 2
		}
		t1 := t*float64(n-1) - float64(i)
		v1 := v[i]
		v2 := v[i+1]
		// Reflect rather than duplicate at the ends; see the doc comment.
		v0 := 2*v1 - v2
		if i > 0 {
			v0 = v[i-1]
		}
		v3 := 2*v2 - v1
		if i < n-2 {
			v3 = v[i+2]
		}
		return basisFn(t1, v0, v1, v2, v3)
	}
}

// BasisClosed returns a closed uniform cubic B-spline through the given values.
//
// "Closed" means the control values wrap: the spline joins its end back to its
// start smoothly, so f(0) == f(1) and the curve is periodic. That makes it the
// right primitive for a cyclic color scale — a hue ramp that has to loop
// without a visible seam where it restarts — which is what d3 uses it for.
//
// Because the spline is periodic, t is wrapped into [0, 1] rather than clamped,
// and every t is meaningful: f(1.25) equals f(0.25). This is the one
// interpolator in the package where out-of-range t is neither clamped nor
// extrapolated, and it follows from the geometry rather than from a policy
// choice.
//
// The curve passes through none of the control values — not even the first —
// since nothing is duplicated to pin it. If you need the endpoints hit exactly,
// you want [Basis].
//
// An empty slice panics; a single value gives a constant.
func BasisClosed(values []float64) func(t float64) float64 {
	v := append([]float64(nil), values...)
	n := len(v)
	if n == 0 {
		panic("interpolate: BasisClosed requires at least one value")
	}
	if n == 1 {
		x := v[0]
		return func(float64) float64 { return x }
	}
	return func(t float64) float64 {
		if math.IsNaN(t) {
			return v[0]
		}
		t -= math.Floor(t)
		f := t * float64(n)
		i := int(math.Floor(f))
		t1 := f - float64(i)
		return basisFn(t1,
			v[(i+n-1)%n],
			v[i%n],
			v[(i+1)%n],
			v[(i+2)%n])
	}
}
