package geo

import "math"

// --- azimuthal ---------------------------------------------------------------

// azimuthalRaw builds an azimuthal projection from its radial scaling function,
// which takes cos of the angular distance from the center and returns the
// factor by which to stretch the plane there. Every azimuthal projection in
// this file is that one function; the rest is shared.
func azimuthalRaw(scale func(cosC float64) float64) func(x, y float64) (float64, float64) {
	return func(x, y float64) (float64, float64) {
		cx, cy := math.Cos(x), math.Cos(y)
		k := scale(cx * cy)
		if math.IsInf(k, 1) {
			return 2, 0
		}
		return k * cy * math.Sin(x), k * math.Sin(y)
	}
}

func azimuthalInvert(angle func(z float64) float64) func(x, y float64) (float64, float64) {
	return func(x, y float64) (float64, float64) {
		z := math.Sqrt(x*x + y*y)
		c := angle(z)
		sc, cc := math.Sin(c), math.Cos(c)
		q := 0.0
		if z != 0 {
			q = y * sc / z
		}
		return math.Atan2(x*sc, z*cc), asin(q)
	}
}

// AzimuthalEqualAreaRaw is Lambert's azimuthal equal-area projection: areas are
// exactly preserved, shapes are not, and the whole globe fits in one disc.
var AzimuthalEqualAreaRaw = Raw{
	Forward: azimuthalRaw(func(cxcy float64) float64 { return math.Sqrt(2 / (1 + cxcy)) }),
	Inverse: azimuthalInvert(func(z float64) float64 { return 2 * asin(z/2) }),
}

// NewAzimuthalEqualArea returns Lambert's azimuthal equal-area projection.
func NewAzimuthalEqualArea() *Projection {
	return NewProjection(AzimuthalEqualAreaRaw).Scale(124.75).ClipAngle(180 - 1e-3)
}

// AzimuthalEquidistantRaw preserves distance from the center: a straight line
// from the middle of the map is a great circle, at true scale.
var AzimuthalEquidistantRaw = Raw{
	Forward: azimuthalRaw(func(c float64) float64 {
		a := acos(c)
		if a == 0 {
			return 0
		}
		return a / math.Sin(a)
	}),
	Inverse: azimuthalInvert(func(z float64) float64 { return z }),
}

// NewAzimuthalEquidistant returns the azimuthal equidistant projection.
func NewAzimuthalEquidistant() *Projection {
	return NewProjection(AzimuthalEquidistantRaw).Scale(79.4188).ClipAngle(180 - 1e-3)
}

// OrthographicRaw is the view of the globe from infinitely far away: exactly
// what a photograph of the Earth from space looks like. Only a hemisphere is
// visible, so it must be clipped at 90°.
var OrthographicRaw = Raw{
	Forward: func(x, y float64) (float64, float64) {
		return math.Cos(y) * math.Sin(x), math.Sin(y)
	},
	Inverse: azimuthalInvert(asin),
}

// NewOrthographic returns the orthographic projection, clipped to the visible
// hemisphere.
func NewOrthographic() *Projection {
	return NewProjection(OrthographicRaw).Scale(249.5).ClipAngle(90 + epsilon)
}

// StereographicRaw is conformal — it preserves angles, and therefore local
// shape — and maps circles on the globe to circles on the map.
var StereographicRaw = Raw{
	Forward: func(x, y float64) (float64, float64) {
		cy := math.Cos(y)
		k := 1 + math.Cos(x)*cy
		return cy * math.Sin(x) / k, math.Sin(y) / k
	},
	Inverse: azimuthalInvert(func(z float64) float64 { return 2 * math.Atan(z) }),
}

// NewStereographic returns the stereographic projection.
func NewStereographic() *Projection {
	return NewProjection(StereographicRaw).Scale(250).ClipAngle(142)
}

// GnomonicRaw projects from the center of the Earth onto a tangent plane. Every
// great circle is a straight line, which is why it is the navigator's
// projection — and why it diverges at 90° and must be clipped well short of it.
var GnomonicRaw = Raw{
	Forward: func(x, y float64) (float64, float64) {
		cy := math.Cos(y)
		k := math.Cos(x) * cy
		return cy * math.Sin(x) / k, math.Sin(y) / k
	},
	Inverse: azimuthalInvert(math.Atan),
}

// NewGnomonic returns the gnomonic projection, clipped to 60°.
func NewGnomonic() *Projection {
	return NewProjection(GnomonicRaw).Scale(144.049).ClipAngle(60)
}

// --- cylindrical -------------------------------------------------------------

// EquirectangularRaw is the identity: longitude becomes x, latitude becomes y.
// Simple, badly distorted at high latitudes, and the reason so many world maps
// make Greenland look enormous.
var EquirectangularRaw = Raw{
	Forward: func(lambda, phi float64) (float64, float64) { return lambda, phi },
	Inverse: func(x, y float64) (float64, float64) { return x, y },
}

// NewEquirectangular returns the equirectangular (plate carrée) projection.
func NewEquirectangular() *Projection {
	return NewProjection(EquirectangularRaw).Scale(152.63)
}

// MercatorRaw is conformal: angles and local shapes are preserved, which is why
// it is the projection of every web map. The price is that area grows without
// bound toward the poles, so the map is clipped short of them.
var MercatorRaw = Raw{
	Forward: func(lambda, phi float64) (float64, float64) {
		return lambda, math.Log(math.Tan((halfPi + phi) / 2))
	},
	Inverse: func(x, y float64) (float64, float64) {
		return x, 2*math.Atan(math.Exp(y)) - halfPi
	},
}

// NewMercator returns the Mercator projection.
//
// It carries an extra clip that the other projections do not need: the
// projection is periodic in longitude, so without it the world repeats
// horizontally forever. See [Projection.ClipExtent], which composes with that
// clip rather than replacing it.
func NewMercator() *Projection {
	p := NewProjection(MercatorRaw)
	p.mercatorClip = true
	return p.Scale(961 / tau)
}

// TransverseMercatorRaw is the Mercator projection turned on its side: true
// shape along a *meridian* instead of along the equator, which is what makes it
// the basis of the UTM grid and of most national mapping systems.
var TransverseMercatorRaw = Raw{
	Forward: func(lambda, phi float64) (float64, float64) {
		return math.Log(math.Tan((halfPi + phi) / 2)), -lambda
	},
	Inverse: func(x, y float64) (float64, float64) {
		return -y, 2*math.Atan(math.Exp(x)) - halfPi
	},
}

// NewTransverseMercator returns the transverse Mercator projection.
//
// [Projection.Center] and [Projection.Rotate] compensate for the swapped axes,
// so both take ordinary (longitude, latitude) and the extra 90° of roll is
// applied internally — as in d3.
func NewTransverseMercator() *Projection {
	p := NewProjection(TransverseMercatorRaw)
	p.mercatorClip = true
	p.transverse = true
	return p.Rotate(0, 0, 0).Scale(159.155)
}

// cylindricalEqualAreaRaw is the degenerate case of a conic equal-area
// projection, used when the two standard parallels are symmetric about the
// equator and the "cone" is a cylinder.
func cylindricalEqualAreaRaw(phi0 float64) Raw {
	cosPhi0 := math.Cos(phi0)
	return Raw{
		Forward: func(lambda, phi float64) (float64, float64) {
			return lambda * cosPhi0, math.Sin(phi) / cosPhi0
		},
		Inverse: func(x, y float64) (float64, float64) {
			return x / cosPhi0, asin(y * cosPhi0)
		},
	}
}

// --- conic -------------------------------------------------------------------

func tany(y float64) float64 { return math.Tan((halfPi + y) / 2) }

// ConicConformalRaw is Lambert's conformal conic: the standard for aeronautical
// charts and for mid-latitude countries wider than they are tall.
func ConicConformalRaw(y0, y1 float64) Raw {
	cy0 := math.Cos(y0)
	var n float64
	if y0 == y1 {
		n = math.Sin(y0)
	} else {
		n = math.Log(cy0/math.Cos(y1)) / math.Log(tany(y1)/tany(y0))
	}
	if n == 0 {
		return MercatorRaw
	}
	f := cy0 * math.Pow(tany(y0), n) / n

	return Raw{
		Forward: func(x, y float64) (float64, float64) {
			if f > 0 {
				if y < -halfPi+epsilon {
					y = -halfPi + epsilon
				}
			} else if y > halfPi-epsilon {
				y = halfPi - epsilon
			}
			r := f / math.Pow(tany(y), n)
			return r * math.Sin(n*x), f - r*math.Cos(n*x)
		},
		Inverse: func(x, y float64) (float64, float64) {
			fy := f - y
			r := sgn(n) * math.Sqrt(x*x+fy*fy)
			l := math.Atan2(x, math.Abs(fy)) * sgn(fy)
			if fy*n < 0 {
				l -= pi * sgn(x) * sgn(fy)
			}
			return l / n, 2*math.Atan(math.Pow(f/r, 1/n)) - halfPi
		},
	}
}

// NewConicConformal returns Lambert's conformal conic projection, with both
// standard parallels at 30°.
func NewConicConformal() *Projection {
	return NewConicProjection(ConicConformalRaw).Scale(109.5).Parallels(30, 30)
}

// ConicEqualAreaRaw is Albers' conic equal-area projection: the usual choice
// for a thematic map of a country, because a choropleth whose areas are wrong
// misleads by construction.
func ConicEqualAreaRaw(y0, y1 float64) Raw {
	sy0 := math.Sin(y0)
	n := (sy0 + math.Sin(y1)) / 2
	if math.Abs(n) < epsilon {
		return cylindricalEqualAreaRaw(y0)
	}
	c := 1 + sy0*(2*n-sy0)
	r0 := math.Sqrt(c) / n

	return Raw{
		Forward: func(x, y float64) (float64, float64) {
			r := math.Sqrt(c-2*n*math.Sin(y)) / n
			x *= n
			return r * math.Sin(x), r0 - r*math.Cos(x)
		},
		Inverse: func(x, y float64) (float64, float64) {
			r0y := r0 - y
			l := math.Atan2(x, math.Abs(r0y)) * sgn(r0y)
			if r0y*n < 0 {
				l -= pi * sgn(x) * sgn(r0y)
			}
			return l / n, asin((c - (x*x+r0y*r0y)*n*n) / (2 * n))
		},
	}
}

// NewConicEqualArea returns Albers' conic equal-area projection.
func NewConicEqualArea() *Projection {
	return NewConicProjection(ConicEqualAreaRaw).Scale(155.424).Center(0, 33.6442)
}

// ConicEquidistantRaw preserves distance along meridians and along the two
// standard parallels.
func ConicEquidistantRaw(y0, y1 float64) Raw {
	cy0 := math.Cos(y0)
	var n float64
	if y0 == y1 {
		n = math.Sin(y0)
	} else {
		n = (cy0 - math.Cos(y1)) / (y1 - y0)
	}
	if math.Abs(n) < epsilon {
		return EquirectangularRaw
	}
	g := cy0/n + y0

	return Raw{
		Forward: func(x, y float64) (float64, float64) {
			gy := g - y
			nx := n * x
			return gy * math.Sin(nx), g - gy*math.Cos(nx)
		},
		Inverse: func(x, y float64) (float64, float64) {
			gy := g - y
			l := math.Atan2(x, math.Abs(gy)) * sgn(gy)
			if gy*n < 0 {
				l -= pi * sgn(x) * sgn(gy)
			}
			return l / n, g - sgn(n)*math.Sqrt(x*x+gy*gy)
		},
	}
}

// NewConicEquidistant returns the conic equidistant projection.
func NewConicEquidistant() *Projection {
	return NewConicProjection(ConicEquidistantRaw).Scale(131.154).Center(0, 13.9389)
}

// --- compromise projections --------------------------------------------------

// NaturalEarth1Raw is Tom Patterson's Natural Earth projection: neither equal
// area nor conformal, tuned by eye to look right for a world map. The
// coefficients are the published polynomial fit and mean nothing individually.
var NaturalEarth1Raw = Raw{
	Forward: func(lambda, phi float64) (float64, float64) {
		phi2 := phi * phi
		phi4 := phi2 * phi2
		return lambda * (0.8707 - 0.131979*phi2 + phi4*(-0.013791+phi4*(0.003971*phi2-0.001529*phi4))),
			phi * (1.007226 + phi2*(0.015085+phi4*(-0.044475+0.028874*phi2-0.005916*phi4)))
	},
	Inverse: func(x, y float64) (float64, float64) {
		// No closed form for the latitude; Newton's method converges in a
		// handful of steps.
		phi := y
		for i := 0; i < 25; i++ {
			phi2 := phi * phi
			phi4 := phi2 * phi2
			delta := (phi*(1.007226+phi2*(0.015085+phi4*(-0.044475+0.028874*phi2-0.005916*phi4))) - y) /
				(1.007226 + phi2*(0.015085*3+phi4*(-0.044475*7+0.028874*9*phi2-0.005916*11*phi4)))
			phi -= delta
			if math.Abs(delta) <= epsilon {
				break
			}
		}
		phi2 := phi * phi
		return x / (0.8707 + phi2*(-0.131979+phi2*(-0.013791+phi2*phi2*phi2*(0.003971-0.001529*phi2)))), phi
	},
}

// NewNaturalEarth1 returns the Natural Earth projection.
func NewNaturalEarth1() *Projection {
	return NewProjection(NaturalEarth1Raw).Scale(175.295)
}

const (
	equalEarthA1 = 1.340264
	equalEarthA2 = -0.081106
	equalEarthA3 = 0.000893
	equalEarthA4 = 0.003796
)

var equalEarthM = math.Sqrt(3) / 2

// EqualEarthRaw is the Equal Earth projection (Šavrič, Patterson & Jenny,
// 2018): equal-area like Gall–Peters, but shaped to look like Robinson. It is
// the modern default for a world map that must not misrepresent area.
var EqualEarthRaw = Raw{
	Forward: func(lambda, phi float64) (float64, float64) {
		l := asin(equalEarthM * math.Sin(phi))
		l2 := l * l
		l6 := l2 * l2 * l2
		return lambda * math.Cos(l) / (equalEarthM * (equalEarthA1 + 3*equalEarthA2*l2 + l6*(7*equalEarthA3+9*equalEarthA4*l2))),
			l * (equalEarthA1 + equalEarthA2*l2 + l6*(equalEarthA3+equalEarthA4*l2))
	},
	Inverse: func(x, y float64) (float64, float64) {
		l := y
		l2 := l * l
		l6 := l2 * l2 * l2
		for i := 0; i < 12; i++ {
			fy := l*(equalEarthA1+equalEarthA2*l2+l6*(equalEarthA3+equalEarthA4*l2)) - y
			fpy := equalEarthA1 + 3*equalEarthA2*l2 + l6*(7*equalEarthA3+9*equalEarthA4*l2)
			delta := fy / fpy
			l -= delta
			l2 = l * l
			l6 = l2 * l2 * l2
			if math.Abs(delta) < epsilon2 {
				break
			}
		}
		return equalEarthM * x * (equalEarthA1 + 3*equalEarthA2*l2 + l6*(7*equalEarthA3+9*equalEarthA4*l2)) / math.Cos(l),
			asin(math.Sin(l) / equalEarthM)
	},
}

// NewEqualEarth returns the Equal Earth projection.
func NewEqualEarth() *Projection {
	return NewProjection(EqualEarthRaw).Scale(177.158)
}
