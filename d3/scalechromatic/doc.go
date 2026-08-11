// Package scalechromatic is a Go port of d3-scale-chromatic: the color schemes
// and the continuous color ramps that d3's sequential and diverging scales are
// almost always given as their range.
//
//	import (
//		"github.com/malcolmston/d3/scale"
//		"github.com/malcolmston/d3/scalechromatic"
//	)
//
//	heat := scale.NewSequential(scalechromatic.InterpolateViridis).SetDomain(0, max)
//	heat.Scale(v)                                       // a color.Color
//
//	legend := scalechromatic.SchemeBlues[5]             // five classes, light to dark
//
// The package is two things that happen to ship together. One is data — the
// ColorBrewer tables, chosen by Cynthia Brewer from cartographic research into
// which color sequences people actually read correctly, plus the matplotlib
// ramps. The other is a handful of formulas — Cubehelix, Turbo, Cividis,
// Rainbow, Sinebow — that compute a color from t directly. The distinction
// matters more here than anywhere else in this port, and it is why the two
// halves are documented and tested differently: a formula can be checked, and a
// table can only be transcribed carefully. See "Provenance" below.
//
// # Three kinds of scheme, for three kinds of data
//
// Picking the wrong kind is the most common charting mistake this package can
// prevent, so it is worth stating plainly:
//
//   - Categorical ([SchemeCategory10], [SchemeSet1], [SchemeTableau10], …) is a
//     fixed list of visually distinct colors for data with no order — countries,
//     product lines, error classes. Hand it to an ordinal scale. Sampling one as
//     a ramp produces nonsense.
//   - Sequential ([SchemeBlues], [InterpolateViridis], …) runs light to dark for
//     data that has a low end and a high end and nothing special in between —
//     population, revenue, temperature in Kelvin.
//   - Diverging ([SchemeRdBu], [InterpolateSpectral], …) is symmetric about a
//     pale midpoint, for data with a meaningful zero: profit and loss,
//     temperature anomaly, swing between two parties. Use it with
//     scale.NewDiverging so the midpoint lands where the data's does, not
//     halfway along the domain.
//
// # Discrete and continuous are the same scheme twice
//
// Every ColorBrewer family appears in both forms, and they are not
// interchangeable. SchemeBlues[5] is five colors chosen as a set of five —
// Brewer picked them so that five adjacent classes stay distinguishable, which
// is not the same as five evenly spaced samples of a ramp. Use the k-indexed
// form when the data is classed (a choropleth with five buckets, a legend with
// five swatches) and the interpolator when it is continuous.
//
// A [Family] is indexable by class count directly, because that is how
// ColorBrewer publishes it and how callers think about it:
//
//	scalechromatic.SchemeBlues[5]      // 5 colors
//	scalechromatic.SchemeBrBG[11]      // 11 colors, the widest diverging set
//	scalechromatic.SchemeBlues.Min()   // 3
//	scalechromatic.SchemeBlues.Max()   // 9
//
// Indices below Min are nil rather than absent, exactly as upstream's
// new Array(3).concat(…) leaves holes: SchemeBlues[2] is nil, and asking for two
// classes of a nine-class scheme is a question ColorBrewer declines to answer.
// Use [Family.K] if you would rather not think about the bounds.
//
// # Every interpolator returns a displayable color
//
// This is the one place the port tightens a guarantee rather than mirroring
// upstream. d3's interpolators return a CSS string, and the conversion to that
// string is what clamps an out-of-gamut result into sRGB. This port returns a
// [color.Color], which has no such boundary, so each interpolator clamps
// explicitly and every one of them returns an opaque, displayable [color.RGB]
// for every t. Without that, [InterpolateRainbow] — whose cubehelix saturation
// of 1.5 is deliberately outside the gamut — would hand back channels a caller
// would have to know to clamp, and the first symptom would be a wrong color in
// somebody else's renderer.
//
// t is clamped to [0, 1] by the tabulated and polynomial ramps and wrapped by
// [InterpolateRainbow], following upstream in each case. [InterpolateSinebow] is
// periodic and accepts any t.
//
// # How the ramps are built
//
// The ColorBrewer ramps are a uniform cubic B-spline through the widest
// published scheme — [interpolate.Basis] applied independently to the R, G and B
// channels of, for example, all nine colors of SchemeBlues[9]. That is d3's
// interpolateRgbBasis, and the choice of a spline over a polyline is what keeps
// a color ramp from showing a visible crease at every stop. Endpoints are exact:
// InterpolateBlues(0) is SchemeBlues[9][0].
//
// The four matplotlib ramps are not splines. They ship as 256 precomputed colors
// and are looked up by nearest index, because they were computed to be
// perceptually uniform and interpolating between a few stops would not preserve
// that. The consequence is worth knowing: [InterpolateViridis] is a step
// function with 256 steps, not a continuous curve, and two nearby t can return
// the identical color. This matches d3 exactly.
//
// [InterpolateTurbo] and [InterpolateCividis] are polynomial fits evaluated per
// channel and rounded to integers, which is upstream's definition rather than an
// approximation of it. [InterpolateCubehelixDefault], [InterpolateWarm] and
// [InterpolateCool] are cubehelix interpolations between two fixed endpoints,
// and [InterpolateRainbow] and [InterpolateSinebow] are closed-form.
//
// # Choosing a scheme from configuration
//
// The registries exist so a scheme name can come from a config file or a query
// string without a switch statement at every call site. Lookup is
// case-insensitive; the canonical names are d3's, minus the prefix:
//
//	f, ok := scalechromatic.Interpolator("viridis")     // or "Viridis", "VIRIDIS"
//	cs, ok := scalechromatic.Scheme("Category10")
//	fam, ok := scalechromatic.SchemeFamily("RdBu")
//
// [Schemes], [SchemeFamilies] and [Interpolators] return the whole tables, for
// building a picker.
//
// # Provenance, and what is deliberately missing
//
// This is the one package in the port where the data is the content and a wrong
// hex digit is undetectable by any property test — a mistyped ColorBrewer value
// still ramps smoothly, still passes every endpoint and gamut check, and is
// simply the wrong color forever. So every table here was extracted
// mechanically from the published d3-scale-chromatic v3.1.0 bundle rather than
// transcribed by hand, and data.go keeps them in upstream's packed
// six-hex-digit form so the file can be diffed against it character for
// character.
//
// Nothing was included from recollection. Schemes absent here are absent for
// that reason and not because they are unwanted.
//
// The ColorBrewer color specifications are copyright 2002 Cynthia Brewer, Mark
// Harrower and The Pennsylvania State University.
package scalechromatic
