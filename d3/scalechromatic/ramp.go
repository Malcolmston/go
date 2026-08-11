package scalechromatic

import (
	"math"

	"github.com/malcolmston/d3/color"
	"github.com/malcolmston/d3/interpolate"
)

// rgbBasis returns a continuous ramp through the given colors: a uniform cubic
// B-spline fitted independently to the R, G and B channels. This is d3's
// interpolateRgbBasis.
//
// Splining each channel separately in sRGB is not the perceptually principled
// choice — Lab would be — but it is what d3 does, and for these tables it is the
// right call anyway. ColorBrewer already chose the stops so that equal steps
// along the list read as equal steps of value; re-spacing them in a perceptual
// space would quietly undo that work. The spline's job here is only to remove
// the creases a polyline would leave at each stop.
//
// The B-spline's ends are pinned (see [interpolate.Basis]), so f(0) and f(1) are
// exactly the first and last colors. Interior stops are approached, not hit.
//
// The result is clamped: the basis weights are a convex combination of four
// control values, but the two phantom controls the end segments use are
// reflections and can fall outside 0–255, so a channel can leave the gamut near
// an end. d3 hides this by formatting to a CSS string; this package clamps
// explicitly so that every interpolator's contract — an opaque, displayable
// [color.RGB] for every t — holds without the caller checking.
func rgbBasis(cs []color.Color) func(t float64) color.Color {
	r := make([]float64, len(cs))
	g := make([]float64, len(cs))
	b := make([]float64, len(cs))
	for i, c := range cs {
		rgb := color.ToRGB(c)
		r[i], g[i], b[i] = rgb.R, rgb.G, rgb.B
	}
	fr, fg, fb := interpolate.Basis(r), interpolate.Basis(g), interpolate.Basis(b)
	return func(t float64) color.Color {
		return color.RGB{R: fr(t), G: fg(t), B: fb(t), Opacity: 1}.Clamp()
	}
}

// tableRamp returns d3's nearest-index lookup over a precomputed table: the
// ramp is a step function with len(cs) steps, and t is clamped to [0, 1].
//
// This is deliberately not interpolation. The matplotlib tables were computed to
// be perceptually uniform at 256 samples, which is finer than any display can
// resolve, so smoothing between samples would add nothing and risk taking the
// ramp off the curve it was fitted to.
func tableRamp(cs []color.Color) func(t float64) color.Color {
	n := len(cs)
	return func(t float64) color.Color {
		// t is tested before the multiply so that a NaN never reaches the
		// float-to-int conversion, which Go leaves implementation-defined.
		switch {
		case math.IsNaN(t) || t <= 0:
			return cs[0]
		case t >= 1:
			return cs[n-1]
		}
		i := int(t * float64(n))
		if i > n-1 {
			i = n - 1
		}
		return cs[i]
	}
}
