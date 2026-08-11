package scalechromatic

import (
	"math"

	"github.com/malcolmston/d3/color"
	"github.com/malcolmston/d3/interpolate"
)

// The three cubehelix ramps are interpolations between two fixed endpoints, not
// tables. Their whole point is that luminance rises monotonically along the
// ramp — see [color.Cubehelix] — which is what lets a colorful scale still be
// read correctly in grayscale.
//
// Each is wrapped so that the result is converted to sRGB and clamped. The
// endpoints deliberately use a saturation of 0.75–1.5, well outside what sRGB
// can show at some lightnesses, so the raw cubehelix value would frequently be
// out of gamut. d3 clamps at the point it formats a CSS string; this package has
// no such point and so clamps here.
var (
	interpCubehelixDefault = clamped(interpolate.CubehelixLong(
		color.NewCubehelix(300, 0.5, 0.0),
		color.NewCubehelix(-240, 0.5, 1.0),
	))
	interpWarm = clamped(interpolate.CubehelixLong(
		color.NewCubehelix(-100, 0.75, 0.35),
		color.NewCubehelix(80, 1.50, 0.8),
	))
	interpCool = clamped(interpolate.CubehelixLong(
		color.NewCubehelix(260, 0.75, 0.35),
		color.NewCubehelix(80, 1.50, 0.8),
	))
)

// clamped converts an interpolator's result to sRGB and forces it into gamut.
func clamped(f func(float64) color.Color) func(float64) color.Color {
	return func(t float64) color.Color { return color.ToRGB(f(t)).Clamp() }
}

// InterpolateCubehelixDefault is Dave Green's cubehelix ramp as d3 configures
// it: black at t = 0, white at t = 1, sweeping a full turn and a half of hue in
// between while luminance climbs monotonically.
//
// It is the ramp to reach for when a figure must survive being photocopied. t is
// not clamped — cubehelix is defined for any t and the hue simply keeps
// turning — but lightness runs outside [0, 1] beyond the endpoints, so the
// result saturates to black or white.
func InterpolateCubehelixDefault(t float64) color.Color { return interpCubehelixDefault(t) }

// InterpolateWarm is a cyclical cubehelix ramp through the warm half of the hue
// circle — purple, red, orange, yellow — at high saturation.
//
// "Cyclical" is the important word: it is meant to be used with a scale whose
// domain wraps, such as a compass bearing or a time of day, where a ramp with
// distinct ends would put a false discontinuity at midnight. Pair it with
// [InterpolateCool] to cover the circle.
func InterpolateWarm(t float64) color.Color { return interpWarm(t) }

// InterpolateCool is a cyclical cubehelix ramp through the cool half of the hue
// circle — green, blue, purple — at high saturation. See [InterpolateWarm].
func InterpolateCool(t float64) color.Color { return interpCool(t) }

// InterpolateRainbow is d3's cyclical rainbow: a full turn of cubehelix hue with
// saturation and lightness peaked at t = 0.5, so that f(0) and f(1) are the same
// color and the ramp closes without a seam.
//
// That seamlessness is the only thing it is good at. As a sequential scale a
// rainbow is actively misleading — its luminance is not monotone, so it invents
// contrast where the data is flat and hides contrast where the data is not, and
// no reader can rank two of its colors reliably. Use it for a cyclical quantity
// (hue of a compass bearing, phase, day of the year) and use
// [InterpolateViridis] for everything else.
//
// t is wrapped rather than clamped, since the ramp is periodic: f(1.25) == f(0.25).
// The definition is upstream's, exactly:
//
//	ts = |t - 0.5|
//	h  = 360t - 100
//	s  = 1.5 - 1.5·ts
//	l  = 0.8 - 0.9·ts
//
// evaluated in cubehelix and clamped into sRGB.
func InterpolateRainbow(t float64) color.Color {
	if t < 0 || t > 1 {
		t -= math.Floor(t)
	}
	ts := math.Abs(t - 0.5)
	return color.ToRGB(color.Cubehelix{
		H:       360*t - 100,
		S:       1.5 - 1.5*ts,
		L:       0.8 - 0.9*ts,
		Opacity: 1,
	}).Clamp()
}

// InterpolateSinebow is Charlie Loyd's sinebow: three sine waves a third of a
// turn apart, one per channel, squared.
//
// Squaring is what makes it worth having. A plain sine rainbow has a luminance
// that swings hard between the primaries and the secondaries; squaring the sine
// makes each channel's contribution proportional to intensity rather than
// amplitude, which flattens that swing considerably. It is still a rainbow and
// still a poor sequential scale, but it is a markedly more even cyclical one
// than [InterpolateRainbow].
//
// The definition is upstream's, exactly, with t' = (0.5 - t)·pi:
//
//	r = 255·sin²(t')
//	g = 255·sin²(t' + pi/3)
//	b = 255·sin²(t' + 2pi/3)
//
// Because sin² of three phases a third of a turn apart sums to 3/2 for every t,
// the total intensity is constant along the whole ramp — which is the property
// the squaring buys.
//
// t is neither clamped nor wrapped: the function is periodic with period 1 by
// construction and is meaningful everywhere. Channels are exact, not rounded;
// they are already inside the gamut for every t. Note that "period 1" holds
// mathematically but not bit for bit — sin is evaluated at a different argument
// for t and t+1, so the two can differ in the last few ulps and occasionally
// round to different integer channels.
//
// A NaN t returns the color at t = 0, for the reason given on clamp01: there is
// no meaningful answer, and a NaN channel would break the package's promise that
// every interpolator returns something a renderer can accept.
func InterpolateSinebow(t float64) color.Color {
	if math.IsNaN(t) {
		t = 0
	}
	t = (0.5 - t) * math.Pi
	r := math.Sin(t)
	g := math.Sin(t + math.Pi/3)
	b := math.Sin(t + 2*math.Pi/3)
	return color.RGB{R: 255 * r * r, G: 255 * g * g, B: 255 * b * b, Opacity: 1}
}
