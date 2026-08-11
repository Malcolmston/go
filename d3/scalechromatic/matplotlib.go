package scalechromatic

import (
	"math"

	"github.com/malcolmston/d3/color"
)

var (
	interpViridis = tableRamp(tableViridis)
	interpMagma   = tableRamp(tableMagma)
	interpInferno = tableRamp(tableInferno)
	interpPlasma  = tableRamp(tablePlasma)
)

// InterpolateViridis is matplotlib's default sequential ramp, and the safe
// default here too: it is monotone in lightness, so it survives being printed in
// grayscale; it is perceptually uniform, so equal steps in t look like equal
// steps in value; and it is legible under the common forms of color vision
// deficiency. Nearly every criticism of the old jet/rainbow ramps is a criticism
// viridis was designed to answer.
//
// It is a 256-step lookup rather than a spline — see [tableRamp].
func InterpolateViridis(t float64) color.Color { return interpViridis(t) }

// InterpolateMagma is one of matplotlib's perceptually uniform ramps, running
// black through purple and magenta to pale yellow. Like [InterpolateViridis] it
// is a 256-step lookup.
func InterpolateMagma(t float64) color.Color { return interpMagma(t) }

// InterpolateInferno is one of matplotlib's perceptually uniform ramps, running
// black through red and orange to pale yellow — the most saturated of the four,
// and the one that reads most like heat. A 256-step lookup.
func InterpolateInferno(t float64) color.Color { return interpInferno(t) }

// InterpolatePlasma is one of matplotlib's perceptually uniform ramps, running
// dark blue through magenta to yellow. A 256-step lookup.
func InterpolatePlasma(t float64) color.Color { return interpPlasma(t) }

// InterpolateCividis is a sequential ramp derived from viridis and optimized so
// that a viewer with deuteranomaly or protanomaly sees very nearly the same
// ramp a trichromatic viewer does — not merely a legible one, but the same one.
// That is what distinguishes it from "colorblind-safe" ramps generally, and it
// is why it is the right choice when a figure has to mean the same thing to
// every reader.
//
// Unlike the matplotlib ramps it is a polynomial fit per channel rather than a
// table, evaluated on a t clamped to [0, 1] and rounded to whole channel values.
// The rounding is upstream's, not an approximation of it: d3 builds an
// "rgb(r, g, b)" string from rounded integers, so returning the unrounded
// polynomial would be a different function from d3's.
func InterpolateCividis(t float64) color.Color {
	t = clamp01(t)
	return color.RGB{
		R:       channel(-4.54 - t*(35.34-t*(2381.73-t*(6402.7-t*(7024.72-t*2710.57))))),
		G:       channel(32.49 + t*(170.73+t*(52.82-t*(131.46-t*(176.58-t*67.37))))),
		B:       channel(81.24 + t*(442.36-t*(2482.43-t*(6167.24-t*(6614.94-t*2475.67))))),
		Opacity: 1,
	}
}

// InterpolateTurbo is Google's replacement for the jet rainbow ramp: it keeps
// the high color contrast that makes rainbow ramps popular for spotting fine
// structure, while removing jet's false bands — the sharp luminance reversals at
// cyan and yellow that invent edges the data does not have.
//
// It is still a rainbow, and so still a poor choice when the reader has to judge
// which of two values is larger; reach for [InterpolateViridis] for that. Where
// it earns its place is depth maps, spectrograms and other dense fields read for
// texture rather than magnitude.
//
// A polynomial fit per channel, clamped and rounded exactly as
// [InterpolateCividis] is.
func InterpolateTurbo(t float64) color.Color {
	t = clamp01(t)
	return color.RGB{
		R:       channel(34.61 + t*(1172.33-t*(10793.56-t*(33300.12-t*(38394.49-t*14825.05))))),
		G:       channel(23.31 + t*(557.33+t*(1225.33-t*(3574.96-t*(1073.77+t*707.56))))),
		B:       channel(27.2 + t*(3211.1-t*(15327.97-t*(27814-t*(22569.18-t*6838.66))))),
		Opacity: 1,
	}
}

// channel rounds a polynomial's output to a whole sRGB channel value and clamps
// it to 0–255.
//
// The rounding is floor(x + 0.5) rather than [math.Round]. JavaScript's
// Math.round breaks halves toward +infinity and Go's breaks them away from zero,
// which differ for negative x — and these polynomials do go negative near the
// ends of their range, which is exactly where the clamp then bites. See
// API-DEVIATIONS.md; the same substitution is made everywhere this port rounds.
func channel(x float64) float64 {
	if math.IsNaN(x) {
		return 0
	}
	return math.Max(0, math.Min(255, math.Floor(x+0.5)))
}

// clamp01 clamps t into [0, 1], mapping NaN to 0.
//
// The NaN case is a deliberate deviation. d3 lets a NaN t propagate through the
// polynomial and emits the literal string "rgb(NaN, NaN, NaN)", which no
// renderer accepts; returning the ramp's first color keeps the package's promise
// that every interpolator yields a displayable color for every input. This is
// the one spot where the port declines to reproduce upstream exactly, and it is
// reproducing a bug that would be reproduced.
func clamp01(t float64) float64 {
	switch {
	case math.IsNaN(t) || t < 0:
		return 0
	case t > 1:
		return 1
	}
	return t
}
