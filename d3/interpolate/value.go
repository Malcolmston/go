package interpolate

import (
	"github.com/malcolmston/d3/color"
)

// Value returns an interpolator between two arbitrary values, choosing the
// strategy from b's dynamic type. It is the Go equivalent of d3.interpolate,
// the entry point you use when the type is only known at run time — as it is
// inside a scale whose range was configured by the caller.
//
// Dispatch, in order:
//
//   - float64, and the other numeric kinds (int, int64, float32, …) — [Number],
//     with the result returned as a float64. Mixed numeric types are fine;
//     both sides are widened.
//   - [color.Color] — [RGB]. sRGB rather than [Lab] is the default because it
//     matches what CSS would do, which is the least surprising behavior for a
//     value whose type was not declared. Call [Lab] explicitly when you want
//     perceptual interpolation.
//   - string — [String] if either side contains a number, otherwise a
//     constant b. A string that parses as a CSS color is *not* interpolated as
//     a color: see the note below.
//   - bool, nil, and everything else — a constant function returning b, since
//     there is no meaningful blend. This mirrors d3, which treats an
//     uninterpolatable value as an instantaneous switch to the target.
//
// The color-string case deserves the explicit note. d3 sniffs strings for CSS
// colors and interpolates "red" to "blue" through the color space; this port
// does not, and instead treats them as plain strings — which for "red"/"blue"
// means a constant, and for "rgb(255,0,0)"/"rgb(0,0,255)" means numeric
// interpolation of the channels, which happens to give the same answer as
// sRGB interpolation. The reason for the difference is that sniffing makes the
// behavior of Value depend on the *content* of a string rather than its type,
// so "10" and "red" would take entirely different paths, and a value that
// accidentally looks like a color name would silently change behavior. Parse to
// a [color.Color] and the dispatch above does the right thing unambiguously.
//
// Extrapolation behavior is whatever the selected interpolator does; see the
// package documentation.
func Value(a, b any) func(t float64) any {
	if c, ok := b.(color.Color); ok {
		if ca, ok := a.(color.Color); ok {
			f := RGB(ca, c)
			return func(t float64) any { return f(t) }
		}
		return constant(b)
	}
	if bs, ok := b.(string); ok {
		as, ok := a.(string)
		if !ok {
			return constant(b)
		}
		f := String(as, bs)
		return func(t float64) any { return f(t) }
	}
	bf, bok := toFloat(b)
	af, aok := toFloat(a)
	if bok && aok {
		f := Number(af, bf)
		return func(t float64) any { return f(t) }
	}
	return constant(b)
}

// constant returns an interpolator that ignores t and always yields v. It is
// the fallback for value types that cannot be blended.
func constant(v any) func(t float64) any {
	return func(float64) any { return v }
}

// toFloat widens any of Go's numeric types to a float64, reporting whether the
// value was numeric at all.
//
// The explicit type switch is unavoidable: a type assertion to float64 would
// reject an int, and reflection would cost more than this on a path that runs
// once per interpolator but has to be correct for every numeric kind a caller
// might hand in. Untyped constants arrive as int or float64, so those two cases
// carry almost all the traffic.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	}
	return 0, false
}
