// Package geo is a Go port of d3-geo: spherical geometry, GeoJSON streaming,
// map projections, clipping, and SVG path generation for geographic data.
//
// The module has two halves that are useful independently.
//
// The first is pure spherical trigonometry over GeoJSON, and it needs no
// projection at all: [Area] and [Length] on the unit sphere, [Distance] and
// [Interpolate] along great circles, [Bounds] that understands the
// antimeridian, [Centroid] that is area-weighted where an area exists, and
// [Contains], a point-in-polygon test that works on the sphere rather than on
// a flattened plane. These are the answers a naive planar library gets wrong,
// and they are the most valuable part of the port.
//
// The second half turns geography into pictures. A [Projection] wraps a raw
// projection — a pair of functions between (longitude, latitude) in radians and
// the plane — with the configuration every map needs: [Projection.Scale],
// [Projection.Translate], [Projection.Center], [Projection.Rotate],
// [Projection.Precision], clipping, and the Fit family that sizes a map to a
// viewport. [Path] then runs a GeoJSON object through a projection and emits
// SVG path data.
//
// # Streams
//
// Everything in the second half is built on one small interface, [Stream]:
//
//	Point(x, y, z)  LineStart()  LineEnd()  PolygonStart()  PolygonEnd()  Sphere()
//
// A GeoJSON object is *pushed* through a stream rather than transformed into a
// new object ([StreamObject] does the pushing). Rotation, resampling, clipping,
// affine transformation and path emission are each a stream that wraps another
// stream, so a projection is literally a composition:
//
//	degrees→radians → rotate → clip to the sphere → resample+project → clip to
//	the viewport → draw
//
// That composition is why an adaptively-resampled, antimeridian-cut,
// circle-clipped map costs one pass over the coordinates and no intermediate
// allocation of clipped geometry. It is the architectural spine of the module
// and is ported faithfully, third `z` coordinate included — the clipping code
// uses that slot to mark synthesized intersection points, exactly as d3 does.
//
// # Output is a string
//
// d3-geo can draw into a canvas rendering context. There is no canvas here, so
// [Path.Generate] returns SVG path data as a string, consistent with d3/shape
// and d3/path. Everything else a canvas context was used for — area, bounds,
// centroid, measure — is a separate method that consumes the same stream.
//
// # Configuration style
//
// Following d3/shape, configuration is write-only and chains: a method is named
// for the property, takes the value, and returns the receiver. There are no
// getters. Coordinates are passed and returned as separate float64s rather than
// two-element slices, matching the rest of this port.
//
//	p := geo.NewOrthographic().Rotate(-70, -30, 0).Scale(240).Translate(480, 250)
//	d := geo.NewPath().Projection(p).Generate(land)
//
// # Angles
//
// Every public angle is in *degrees*, as in d3: longitudes, latitudes, clip
// angles, rotation angles, circle radii. Radians appear only inside a stream,
// between the degrees→radians transform and the projection.
//
// # Winding order
//
// The one thing to check before trusting any polygon result: d3's spherical
// winding convention is the *opposite* of RFC 7946's. An exterior ring smaller
// than a hemisphere must wind clockwise, holes counterclockwise. Data wound the
// other way describes the complement of the region you meant, and [Area],
// [Contains] and every clipped path will say so. See [Polygon].
package geo

import "math"

const (
	epsilon   = 1e-6
	epsilon2  = 1e-12
	pi        = math.Pi
	halfPi    = pi / 2
	quarterPi = pi / 4
	tau       = pi * 2
	degrees   = 180 / pi
	radians   = pi / 180
)

// acos and asin clamp their argument, which is not paranoia: a dot product of
// two unit vectors is routinely 1+1e-16 after rounding, and Go's math.Acos
// returns NaN there where the geometry wants 0.
func acos(x float64) float64 {
	switch {
	case x > 1:
		return 0
	case x < -1:
		return pi
	}
	return math.Acos(x)
}

func asin(x float64) float64 {
	switch {
	case x > 1:
		return halfPi
	case x < -1:
		return -halfPi
	}
	return math.Asin(x)
}

// haversin(x) = sin²(x/2), the haversine of x.
func haversin(x float64) float64 {
	s := math.Sin(x / 2)
	return s * s
}

// sgn is JavaScript's Math.sign: zero maps to zero, unlike math.Copysign.
func sgn(x float64) float64 {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	}
	return 0
}

func hypot3(x, y, z float64) float64 { return math.Sqrt(x*x + y*y + z*z) }

// adder is a compensated (Neumaier) accumulator.
//
// Spherical area and winding sums add many terms of similar magnitude and
// opposite sign, and the *sign* of the total decides whether a polygon contains
// a pole. Plain float64 addition loses that sign for large rings; d3 uses a
// full-precision expansion for the same reason. Neumaier compensation is
// cheaper and sufficient at the magnitudes these sums reach.
type adder struct{ sum, c float64 }

func (a *adder) Add(x float64) {
	t := a.sum + x
	if math.Abs(a.sum) >= math.Abs(x) {
		a.c += (a.sum - t) + x
	} else {
		a.c += (x - t) + a.sum
	}
	a.sum = t
}

func (a *adder) Value() float64 { return a.sum + a.c }

func (a *adder) Reset() { a.sum, a.c = 0, 0 }

// --- Cartesian helpers -------------------------------------------------------
//
// A point on the unit sphere is either spherical (λ, φ) in radians or Cartesian
// (x, y, z). Great-circle intersection and cross-product winding tests are far
// easier in Cartesian coordinates, so the two forms are converted freely.

type vec3 [3]float64

func cartesian(lambda, phi float64) vec3 {
	cosPhi := math.Cos(phi)
	return vec3{cosPhi * math.Cos(lambda), cosPhi * math.Sin(lambda), math.Sin(phi)}
}

// spherical is the inverse of cartesian for a unit vector.
func spherical(v vec3) (lambda, phi float64) {
	return math.Atan2(v[1], v[0]), asin(v[2])
}

func cartesianDot(a, b vec3) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func cartesianCross(a, b vec3) vec3 {
	return vec3{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func cartesianAdd(a, b vec3) vec3 { return vec3{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }

func cartesianScale(v vec3, k float64) vec3 { return vec3{v[0] * k, v[1] * k, v[2] * k} }

func cartesianNormalize(v vec3) vec3 {
	l := hypot3(v[0], v[1], v[2])
	return vec3{v[0] / l, v[1] / l, v[2] / l}
}

// pointEqual reports whether two (λ, φ) pairs coincide to within epsilon.
func pointEqual(a, b [3]float64) bool {
	return math.Abs(a[0]-b[0]) < epsilon && math.Abs(a[1]-b[1]) < epsilon
}
