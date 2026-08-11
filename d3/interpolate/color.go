package interpolate

import (
	"math"

	"github.com/malcolmston/d3/color"
)

// This file holds the color-space interpolators. Each converts both endpoints
// into a working space once, closes over the per-channel [Number]
// interpolators, and converts back on every evaluation. Choosing the space is
// the whole point — see the package documentation for what each one looks like.
//
// Two concerns recur across all of them and are handled the same way
// everywhere.
//
// Opacity is always interpolated linearly in the same pass as the color
// channels, so fading and shifting hue at once behaves as expected.
//
// Hue needs special care in the cylindrical spaces ([HSL], [HCL], [Cubehelix]).
// See hueInterp for the shortest-path rule and for how an undefined (NaN) hue
// is handled.

// hueInterp returns an interpolator between two hue angles in degrees, taking
// the shortest path around the color circle.
//
// Naively interpolating 350 to 10 would sweep 340 degrees backwards through the
// entire wheel; taking the delta modulo 360 and folding anything past 180 to
// the other side turns it into a 20-degree step through red, which is what
// anyone looking at the two colors expects. The intermediate values are allowed
// to leave [0, 360) — the 350→10 case passes through 360 — because wrapping
// eagerly would reintroduce the discontinuity; the color types normalize hue at
// conversion time instead.
//
// A NaN hue means the color is achromatic and has no hue to speak of (see
// [color.ToHSL]). When exactly one endpoint is NaN, the other endpoint's hue is
// held constant for the whole sweep, so fading gray to red stays on red's hue
// and just gains saturation rather than sweeping through the spectrum. When
// both are NaN the result is NaN, which the color types render as 0.
func hueInterp(a, b float64) func(t float64) float64 {
	aNaN, bNaN := math.IsNaN(a), math.IsNaN(b)
	switch {
	case aNaN && bNaN:
		return func(float64) float64 { return math.NaN() }
	case aNaN:
		return func(float64) float64 { return b }
	case bNaN:
		return func(float64) float64 { return a }
	}
	d := math.Mod(b-a, 360)
	if d > 180 {
		d -= 360
	} else if d < -180 {
		d += 360
	}
	return func(t float64) float64 {
		switch t {
		case 0:
			return a
		case 1:
			return b
		}
		return a + d*t
	}
}

// RGB returns an interpolator between two colors in the sRGB space.
//
// Each of the red, green, blue and opacity channels is interpolated
// independently and linearly. This is what CSS transitions and canvas gradients
// do, so reach for it when you need to match browser rendering exactly.
//
// It is a poor choice for a sequential scale. Because sRGB is gamma-encoded and
// not perceptually uniform, a linear sweep produces uneven visual steps and
// desaturated midpoints — blue to yellow passes through a dead gray. [Lab] is
// the fix.
//
// t is not clamped: extrapolation pushes channels past 0 or 255, which is
// preserved rather than clamped (see [color.RGB]) until the result is
// formatted. The endpoints are exact: at t = 0 the returned color equals a's
// sRGB form and at t = 1 it equals b's.
func RGB(a, b color.Color) func(t float64) color.Color {
	ca, cb := color.ToRGB(a), color.ToRGB(b)
	r := Number(ca.R, cb.R)
	g := Number(ca.G, cb.G)
	bl := Number(ca.B, cb.B)
	o := Number(ca.Opacity, cb.Opacity)
	return func(t float64) color.Color {
		return color.RGB{R: r(t), G: g(t), B: bl(t), Opacity: o(t)}
	}
}

// HSL returns an interpolator between two colors in the CSS HSL space, taking
// the shortest path around the hue circle.
//
// Use it when you want the transition to read as a sweep through the spectrum —
// red to green really does pass through yellow. Saturation, lightness and
// opacity interpolate linearly. See hueInterp for the shortest-path rule and
// for how an achromatic endpoint (a gray, whose hue is undefined) is handled:
// the other endpoint's hue is held, so gray to red does not spin the wheel.
//
// HSL is not perceptually uniform; a sweep at constant S and L varies a lot in
// apparent brightness. [HCL] is the perceptual equivalent of this operation.
//
// t is not clamped. Endpoints are exact.
func HSL(a, b color.Color) func(t float64) color.Color {
	ca, cb := color.ToHSL(a), color.ToHSL(b)
	h := hueInterp(ca.H, cb.H)
	s := Number(ca.S, cb.S)
	l := Number(ca.L, cb.L)
	o := Number(ca.Opacity, cb.Opacity)
	return func(t float64) color.Color {
		return color.HSL{H: h(t), S: s(t), L: l(t), Opacity: o(t)}
	}
}

// HSLLong returns an interpolator between two colors in HSL that takes the
// long way around the hue circle rather than the short one.
//
// It is the tool for a full-spectrum rainbow ramp: interpolating a color to
// itself with the shortest-path [HSL] gives a constant, while this sweeps all
// the way around and back. d3 exposes it as interpolateHslLong. Saturation,
// lightness and opacity behave exactly as in [HSL].
//
// t is not clamped.
func HSLLong(a, b color.Color) func(t float64) color.Color {
	ca, cb := color.ToHSL(a), color.ToHSL(b)
	h := Number(ca.H, cb.H)
	s := Number(ca.S, cb.S)
	l := Number(ca.L, cb.L)
	o := Number(ca.Opacity, cb.Opacity)
	return func(t float64) color.Color {
		return color.HSL{H: h(t), S: s(t), L: l(t), Opacity: o(t)}
	}
}

// Lab returns an interpolator between two colors in CIELAB.
//
// This is the default worth reaching for in a sequential color scale. Because
// Lab is approximately perceptually uniform, equal steps in t look like equal
// steps in color, so the ramp has no bright bands or dead zones and encodes
// magnitude honestly. L, A, B and opacity all interpolate linearly.
//
// The midpoints of a Lab interpolation between two saturated colors can leave
// the sRGB gamut, in which case the converted [color.RGB] has channels outside
// 0–255. Nothing is clamped here; see [color.RGB] and [color.RGB.Displayable].
//
// t is not clamped. Endpoints are exact in Lab, and therefore exact in sRGB to
// within the conversion's floating-point error.
func Lab(a, b color.Color) func(t float64) color.Color {
	ca, cb := color.ToLab(a), color.ToLab(b)
	l := Number(ca.L, cb.L)
	aa := Number(ca.A, cb.A)
	bb := Number(ca.B, cb.B)
	o := Number(ca.Opacity, cb.Opacity)
	return func(t float64) color.Color {
		return color.Lab{L: l(t), A: aa(t), B: bb(t), Opacity: o(t)}
	}
}

// HCL returns an interpolator between two colors in CIE LCh, taking the
// shortest path around the hue circle.
//
// It is [HSL]'s perceptual counterpart: you get a hue sweep, but at genuinely
// constant perceived lightness rather than HSL's nominal one. That makes it the
// right choice for a diverging scale or a categorical palette, where the colors
// must differ in hue without any of them looking heavier than the others.
//
// The caveat is gamut — see [color.HCL]. A path between two in-gamut colors can
// bulge outside sRGB in the middle, and the resulting channels are left
// unclamped here.
//
// t is not clamped. Endpoints are exact.
func HCL(a, b color.Color) func(t float64) color.Color {
	ca, cb := color.ToHCL(a), color.ToHCL(b)
	h := hueInterp(ca.H, cb.H)
	c := Number(ca.C, cb.C)
	l := Number(ca.L, cb.L)
	o := Number(ca.Opacity, cb.Opacity)
	return func(t float64) color.Color {
		return color.HCL{H: h(t), C: c(t), L: l(t), Opacity: o(t)}
	}
}

// HCLLong returns an interpolator in CIE LCh that takes the long way around the
// hue circle, the perceptual analogue of [HSLLong].
func HCLLong(a, b color.Color) func(t float64) color.Color {
	ca, cb := color.ToHCL(a), color.ToHCL(b)
	h := Number(ca.H, cb.H)
	c := Number(ca.C, cb.C)
	l := Number(ca.L, cb.L)
	o := Number(ca.Opacity, cb.Opacity)
	return func(t float64) color.Color {
		return color.HCL{H: h(t), C: c(t), L: l(t), Opacity: o(t)}
	}
}

// Cubehelix returns an interpolator between two colors in cubehelix space,
// taking the shortest path around the hue circle.
//
// Its distinguishing property is that luminance varies monotonically along the
// ramp, so the result stays readable when printed in grayscale or seen by a
// viewer with color vision deficiency — the reason d3's rainbow scale is built
// on cubehelix rather than on HSL. See [color.Cubehelix].
//
// t is not clamped. Endpoints are exact.
func Cubehelix(a, b color.Color) func(t float64) color.Color {
	ca, cb := color.ToCubehelix(a), color.ToCubehelix(b)
	h := hueInterp(ca.H, cb.H)
	s := Number(ca.S, cb.S)
	l := Number(ca.L, cb.L)
	o := Number(ca.Opacity, cb.Opacity)
	return func(t float64) color.Color {
		return color.Cubehelix{H: h(t), S: s(t), L: l(t), Opacity: o(t)}
	}
}

// CubehelixLong returns a cubehelix interpolator that takes the long way around
// the hue circle. This is the one d3 uses to build its rainbow scale, because
// the full spectral sweep is exactly what is wanted there.
func CubehelixLong(a, b color.Color) func(t float64) color.Color {
	ca, cb := color.ToCubehelix(a), color.ToCubehelix(b)
	h := Number(ca.H, cb.H)
	s := Number(ca.S, cb.S)
	l := Number(ca.L, cb.L)
	o := Number(ca.Opacity, cb.Opacity)
	return func(t float64) color.Color {
		return color.Cubehelix{H: h(t), S: s(t), L: l(t), Opacity: o(t)}
	}
}
