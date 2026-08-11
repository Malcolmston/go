// Code generated from d3-scale-chromatic v3.1.0. DO NOT EDIT.
//
// One interpolator per ColorBrewer family, each a B-spline through the family's
// widest published scheme. See [rgbBasis] for why a spline and why sRGB.
//
// The closures are built once at init and the exported names are functions rather
// than function-valued variables, so no caller can reassign a scheme out from
// under another package.

package scalechromatic

import "github.com/malcolmston/d3/color"

var (
	interpBrBG     = rgbBasis(SchemeBrBG.Largest())
	interpPRGn     = rgbBasis(SchemePRGn.Largest())
	interpPiYG     = rgbBasis(SchemePiYG.Largest())
	interpPuOr     = rgbBasis(SchemePuOr.Largest())
	interpRdBu     = rgbBasis(SchemeRdBu.Largest())
	interpRdGy     = rgbBasis(SchemeRdGy.Largest())
	interpRdYlBu   = rgbBasis(SchemeRdYlBu.Largest())
	interpRdYlGn   = rgbBasis(SchemeRdYlGn.Largest())
	interpSpectral = rgbBasis(SchemeSpectral.Largest())
	interpBuGn     = rgbBasis(SchemeBuGn.Largest())
	interpBuPu     = rgbBasis(SchemeBuPu.Largest())
	interpGnBu     = rgbBasis(SchemeGnBu.Largest())
	interpOrRd     = rgbBasis(SchemeOrRd.Largest())
	interpPuBuGn   = rgbBasis(SchemePuBuGn.Largest())
	interpPuBu     = rgbBasis(SchemePuBu.Largest())
	interpPuRd     = rgbBasis(SchemePuRd.Largest())
	interpRdPu     = rgbBasis(SchemeRdPu.Largest())
	interpYlGnBu   = rgbBasis(SchemeYlGnBu.Largest())
	interpYlGn     = rgbBasis(SchemeYlGn.Largest())
	interpYlOrBr   = rgbBasis(SchemeYlOrBr.Largest())
	interpYlOrRd   = rgbBasis(SchemeYlOrRd.Largest())
	interpBlues    = rgbBasis(SchemeBlues.Largest())
	interpGreens   = rgbBasis(SchemeGreens.Largest())
	interpGreys    = rgbBasis(SchemeGreys.Largest())
	interpPurples  = rgbBasis(SchemePurples.Largest())
	interpReds     = rgbBasis(SchemeReds.Largest())
	interpOranges  = rgbBasis(SchemeOranges.Largest())
)

// InterpolateBrBG is the continuous form of the ColorBrewer "BrBG" diverging
// family, running brown to blue-green. It splines through SchemeBrBG[11], so
// InterpolateBrBG(0) and (1) are that list's first and last colors exactly.
func InterpolateBrBG(t float64) color.Color { return interpBrBG(t) }

// InterpolatePRGn is the continuous form of the ColorBrewer "PRGn" diverging
// family, running purple to green. It splines through SchemePRGn[11], so
// InterpolatePRGn(0) and (1) are that list's first and last colors exactly.
func InterpolatePRGn(t float64) color.Color { return interpPRGn(t) }

// InterpolatePiYG is the continuous form of the ColorBrewer "PiYG" diverging
// family, running pink to yellow-green. It splines through SchemePiYG[11], so
// InterpolatePiYG(0) and (1) are that list's first and last colors exactly.
func InterpolatePiYG(t float64) color.Color { return interpPiYG(t) }

// InterpolatePuOr is the continuous form of the ColorBrewer "PuOr" diverging
// family, running orange to purple. It splines through SchemePuOr[11], so
// InterpolatePuOr(0) and (1) are that list's first and last colors exactly.
func InterpolatePuOr(t float64) color.Color { return interpPuOr(t) }

// InterpolateRdBu is the continuous form of the ColorBrewer "RdBu" diverging
// family, running red to blue. It splines through SchemeRdBu[11], so
// InterpolateRdBu(0) and (1) are that list's first and last colors exactly.
func InterpolateRdBu(t float64) color.Color { return interpRdBu(t) }

// InterpolateRdGy is the continuous form of the ColorBrewer "RdGy" diverging
// family, running red to grey. It splines through SchemeRdGy[11], so
// InterpolateRdGy(0) and (1) are that list's first and last colors exactly.
func InterpolateRdGy(t float64) color.Color { return interpRdGy(t) }

// InterpolateRdYlBu is the continuous form of the ColorBrewer "RdYlBu" diverging
// family, running red to yellow to blue. It splines through SchemeRdYlBu[11], so
// InterpolateRdYlBu(0) and (1) are that list's first and last colors exactly.
func InterpolateRdYlBu(t float64) color.Color { return interpRdYlBu(t) }

// InterpolateRdYlGn is the continuous form of the ColorBrewer "RdYlGn" diverging
// family, running red to yellow to green. It splines through SchemeRdYlGn[11], so
// InterpolateRdYlGn(0) and (1) are that list's first and last colors exactly.
func InterpolateRdYlGn(t float64) color.Color { return interpRdYlGn(t) }

// InterpolateSpectral is the continuous form of the ColorBrewer "Spectral" diverging
// family, running the full red-to-blue spectral sweep. It splines through SchemeSpectral[11], so
// InterpolateSpectral(0) and (1) are that list's first and last colors exactly.
func InterpolateSpectral(t float64) color.Color { return interpSpectral(t) }

// InterpolateBuGn is the continuous form of the ColorBrewer "BuGn" sequential
// family, running blue to green. It splines through SchemeBuGn[9], so
// InterpolateBuGn(0) and (1) are that list's first and last colors exactly.
func InterpolateBuGn(t float64) color.Color { return interpBuGn(t) }

// InterpolateBuPu is the continuous form of the ColorBrewer "BuPu" sequential
// family, running blue to purple. It splines through SchemeBuPu[9], so
// InterpolateBuPu(0) and (1) are that list's first and last colors exactly.
func InterpolateBuPu(t float64) color.Color { return interpBuPu(t) }

// InterpolateGnBu is the continuous form of the ColorBrewer "GnBu" sequential
// family, running green to blue. It splines through SchemeGnBu[9], so
// InterpolateGnBu(0) and (1) are that list's first and last colors exactly.
func InterpolateGnBu(t float64) color.Color { return interpGnBu(t) }

// InterpolateOrRd is the continuous form of the ColorBrewer "OrRd" sequential
// family, running orange to red. It splines through SchemeOrRd[9], so
// InterpolateOrRd(0) and (1) are that list's first and last colors exactly.
func InterpolateOrRd(t float64) color.Color { return interpOrRd(t) }

// InterpolatePuBuGn is the continuous form of the ColorBrewer "PuBuGn" sequential
// family, running purple to blue to green. It splines through SchemePuBuGn[9], so
// InterpolatePuBuGn(0) and (1) are that list's first and last colors exactly.
func InterpolatePuBuGn(t float64) color.Color { return interpPuBuGn(t) }

// InterpolatePuBu is the continuous form of the ColorBrewer "PuBu" sequential
// family, running purple to blue. It splines through SchemePuBu[9], so
// InterpolatePuBu(0) and (1) are that list's first and last colors exactly.
func InterpolatePuBu(t float64) color.Color { return interpPuBu(t) }

// InterpolatePuRd is the continuous form of the ColorBrewer "PuRd" sequential
// family, running purple to red. It splines through SchemePuRd[9], so
// InterpolatePuRd(0) and (1) are that list's first and last colors exactly.
func InterpolatePuRd(t float64) color.Color { return interpPuRd(t) }

// InterpolateRdPu is the continuous form of the ColorBrewer "RdPu" sequential
// family, running red to purple. It splines through SchemeRdPu[9], so
// InterpolateRdPu(0) and (1) are that list's first and last colors exactly.
func InterpolateRdPu(t float64) color.Color { return interpRdPu(t) }

// InterpolateYlGnBu is the continuous form of the ColorBrewer "YlGnBu" sequential
// family, running yellow to green to blue. It splines through SchemeYlGnBu[9], so
// InterpolateYlGnBu(0) and (1) are that list's first and last colors exactly.
func InterpolateYlGnBu(t float64) color.Color { return interpYlGnBu(t) }

// InterpolateYlGn is the continuous form of the ColorBrewer "YlGn" sequential
// family, running yellow to green. It splines through SchemeYlGn[9], so
// InterpolateYlGn(0) and (1) are that list's first and last colors exactly.
func InterpolateYlGn(t float64) color.Color { return interpYlGn(t) }

// InterpolateYlOrBr is the continuous form of the ColorBrewer "YlOrBr" sequential
// family, running yellow to orange to brown. It splines through SchemeYlOrBr[9], so
// InterpolateYlOrBr(0) and (1) are that list's first and last colors exactly.
func InterpolateYlOrBr(t float64) color.Color { return interpYlOrBr(t) }

// InterpolateYlOrRd is the continuous form of the ColorBrewer "YlOrRd" sequential
// family, running yellow to orange to red. It splines through SchemeYlOrRd[9], so
// InterpolateYlOrRd(0) and (1) are that list's first and last colors exactly.
func InterpolateYlOrRd(t float64) color.Color { return interpYlOrRd(t) }

// InterpolateBlues is the continuous form of the ColorBrewer "Blues" sequential
// family, running white to dark blue. It splines through SchemeBlues[9], so
// InterpolateBlues(0) and (1) are that list's first and last colors exactly.
func InterpolateBlues(t float64) color.Color { return interpBlues(t) }

// InterpolateGreens is the continuous form of the ColorBrewer "Greens" sequential
// family, running white to dark green. It splines through SchemeGreens[9], so
// InterpolateGreens(0) and (1) are that list's first and last colors exactly.
func InterpolateGreens(t float64) color.Color { return interpGreens(t) }

// InterpolateGreys is the continuous form of the ColorBrewer "Greys" sequential
// family, running white to black. It splines through SchemeGreys[9], so
// InterpolateGreys(0) and (1) are that list's first and last colors exactly.
func InterpolateGreys(t float64) color.Color { return interpGreys(t) }

// InterpolatePurples is the continuous form of the ColorBrewer "Purples" sequential
// family, running white to dark purple. It splines through SchemePurples[9], so
// InterpolatePurples(0) and (1) are that list's first and last colors exactly.
func InterpolatePurples(t float64) color.Color { return interpPurples(t) }

// InterpolateReds is the continuous form of the ColorBrewer "Reds" sequential
// family, running white to dark red. It splines through SchemeReds[9], so
// InterpolateReds(0) and (1) are that list's first and last colors exactly.
func InterpolateReds(t float64) color.Color { return interpReds(t) }

// InterpolateOranges is the continuous form of the ColorBrewer "Oranges" sequential
// family, running white to dark orange. It splines through SchemeOranges[9], so
// InterpolateOranges(0) and (1) are that list's first and last colors exactly.
func InterpolateOranges(t float64) color.Color { return interpOranges(t) }
