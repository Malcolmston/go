package geo

import "math"

// --- spherical area ----------------------------------------------------------

// areaState accumulates the spherical excess of each polygon ring.
//
// The identity used is Cagnoli's: the signed area of a spherical polygon is the
// sum, over its edges, of the excess of the triangle formed by that edge and
// the south pole. That makes the whole computation one pass with three
// remembered values, and it makes the *sign* meaningful — a ring wound the
// other way encloses the complement, which is exactly how GeoJSON distinguishes
// a hole from an island.
type areaState struct {
	ringSum, sum              adder
	lambda00, phi00           float64
	lambda0, cosPhi0, sinPhi0 float64
	inRing                    bool
	first                     bool
}

func (a *areaState) PolygonStart() { a.ringSum.Reset(); a.inRing = true }

func (a *areaState) PolygonEnd() {
	v := a.ringSum.Value()
	if v < 0 {
		v += tau
	}
	a.sum.Add(v)
	a.inRing = false
}

func (a *areaState) LineStart() {
	if a.inRing {
		a.first = true
	}
}

func (a *areaState) LineEnd() {
	if a.inRing {
		// Close the ring: GeoJSON's repeated last point was dropped by the
		// stream, so the final edge back to the first point is added here.
		a.point(a.lambda00, a.phi00)
	}
}

func (a *areaState) Point(x, y, _ float64) {
	if !a.inRing {
		return
	}
	if a.first {
		a.first = false
		a.lambda00, a.phi00 = x, y
		lambda, phi := x*radians, y*radians
		a.lambda0 = lambda
		phi = phi/2 + quarterPi
		a.cosPhi0, a.sinPhi0 = math.Cos(phi), math.Sin(phi)
		return
	}
	a.point(x, y)
}

func (a *areaState) point(x, y float64) {
	lambda, phi := x*radians, y*radians
	phi = phi/2 + quarterPi // half the angular distance from the south pole

	dLambda := lambda - a.lambda0
	sdLambda := 1.0
	if dLambda < 0 {
		sdLambda = -1
	}
	adLambda := sdLambda * dLambda
	cosPhi, sinPhi := math.Cos(phi), math.Sin(phi)
	k := a.sinPhi0 * sinPhi
	u := a.cosPhi0*cosPhi + k*math.Cos(adLambda)
	v := k * sdLambda * math.Sin(adLambda)
	a.ringSum.Add(math.Atan2(v, u))

	a.lambda0, a.cosPhi0, a.sinPhi0 = lambda, cosPhi, sinPhi
}

func (a *areaState) Sphere() { a.sum.Add(tau) }

// Area returns the spherical area of a GeoJSON object in steradians.
//
// Steradians, not square kilometres: the sphere has area 4π, a hemisphere 2π.
// Multiply by the square of a radius to get a real area (6 371 008.8 m for
// Earth's mean radius).
//
// Only polygons and the [Sphere] have area; points and lines contribute
// nothing. Ring winding matters — an exterior ring must wind counterclockwise
// (right-hand rule) or you get the complement of the region, whose area is
// 4π minus the one you wanted.
func Area(o Object) float64 {
	var a areaState
	StreamObject(o, &a)
	return a.sum.Value() * 2
}

// --- spherical length --------------------------------------------------------

type lengthState struct {
	sum                       adder
	lambda0, sinPhi0, cosPhi0 float64
	lon00, lat00              float64
	inLine                    bool
	inRing                    bool
	first                     bool
}

func (l *lengthState) PolygonStart() { l.inRing = true }
func (l *lengthState) PolygonEnd()   { l.inRing = false }
func (l *lengthState) Sphere()       {}
func (l *lengthState) LineStart()    { l.inLine, l.first = true, true }
func (l *lengthState) LineEnd() {
	// The stream drops a ring's repeated closing position, so the edge back to
	// its first point has to be added here for a perimeter to be a perimeter.
	if l.inRing && !l.first {
		l.point(l.lon00, l.lat00)
	}
	l.inLine = false
}

func (l *lengthState) Point(x, y, _ float64) {
	if !l.inLine {
		return
	}
	if l.first {
		l.first = false
		l.lon00, l.lat00 = x, y
		lambda, phi := x*radians, y*radians
		l.lambda0, l.sinPhi0, l.cosPhi0 = lambda, math.Sin(phi), math.Cos(phi)
		return
	}
	l.point(x, y)
}

func (l *lengthState) point(x, y float64) {
	lambda, phi := x*radians, y*radians
	sinPhi, cosPhi := math.Sin(phi), math.Cos(phi)
	delta := math.Abs(lambda - l.lambda0)
	cosDelta, sinDelta := math.Cos(delta), math.Sin(delta)
	// The Vincenty formula for angular distance: numerically well behaved for
	// both very short and antipodal separations, unlike the plain acos form.
	xx := cosPhi * sinDelta
	yy := l.cosPhi0*sinPhi - l.sinPhi0*cosPhi*cosDelta
	zz := l.sinPhi0*sinPhi + l.cosPhi0*cosPhi*cosDelta
	l.sum.Add(math.Atan2(math.Sqrt(xx*xx+yy*yy), zz))
	l.lambda0, l.sinPhi0, l.cosPhi0 = lambda, sinPhi, cosPhi
}

// Length returns the great-circle length of a GeoJSON object in radians: the
// length of its lines, and the perimeter of each of a polygon's rings — the
// exterior one plus every hole.
//
// Radians, not metres: multiply by a radius to get a distance. Points
// contribute nothing.
func Length(o Object) float64 {
	var l lengthState
	StreamObject(o, &l)
	return l.sum.Value()
}

// Distance returns the great-circle distance in radians between two points
// given in degrees.
//
// Two identical points are 0 apart; two antipodes are π apart; the poles are π
// apart regardless of longitude. Multiply by a radius for metres.
func Distance(lon0, lat0, lon1, lat1 float64) float64 {
	return Length(&LineString{Coordinates: []Position{P(lon0, lat0), P(lon1, lat1)}})
}

// --- spherical centroid ------------------------------------------------------

type centroidState struct {
	// W0/X0/Y0/Z0: running arithmetic mean of point vectors.
	w0            float64
	x0v, y0v, z0v float64
	// W1/X1/Y1/Z1: length-weighted sums along arcs.
	w1         float64
	x1, y1, z1 float64
	// X2/Y2/Z2: area-weighted sums over rings.
	x2, y2, z2      adder
	lambda00, phi00 float64
	px, py, pz      float64 // previous point, Cartesian
	mode            int     // 0 point, 1 line, 2 ring
	first           bool
}

func (c *centroidState) Sphere() {}

func (c *centroidState) PolygonStart() { c.mode = 2 }
func (c *centroidState) PolygonEnd()   { c.mode = 0 }

func (c *centroidState) LineStart() {
	if c.mode != 2 {
		c.mode = 1
	}
	c.first = true
}

func (c *centroidState) LineEnd() {
	if c.mode == 2 {
		c.ringPoint(c.lambda00, c.phi00)
		c.first = true
		return
	}
	c.mode = 0
}

func (c *centroidState) Point(x, y, _ float64) {
	switch c.mode {
	case 2:
		if c.first {
			c.first = false
			c.lambda00, c.phi00 = x, y
			c.begin(x, y)
			return
		}
		c.ringPoint(x, y)
	case 1:
		if c.first {
			c.first = false
			c.begin(x, y)
			return
		}
		c.linePoint(x, y)
	default:
		lambda, phi := x*radians, y*radians
		cosPhi := math.Cos(phi)
		c.addPoint(cosPhi*math.Cos(lambda), cosPhi*math.Sin(lambda), math.Sin(phi))
	}
}

func (c *centroidState) begin(x, y float64) {
	lambda, phi := x*radians, y*radians
	cosPhi := math.Cos(phi)
	c.px, c.py, c.pz = cosPhi*math.Cos(lambda), cosPhi*math.Sin(lambda), math.Sin(phi)
	c.addPoint(c.px, c.py, c.pz)
}

func (c *centroidState) addPoint(x, y, z float64) {
	c.w0++
	c.x0v += (x - c.x0v) / c.w0
	c.y0v += (y - c.y0v) / c.w0
	c.z0v += (z - c.z0v) / c.w0
}

func (c *centroidState) linePoint(lon, lat float64) {
	lambda, phi := lon*radians, lat*radians
	cosPhi := math.Cos(phi)
	x, y, z := cosPhi*math.Cos(lambda), cosPhi*math.Sin(lambda), math.Sin(phi)
	cx := c.py*z - c.pz*y
	cy := c.pz*x - c.px*z
	cz := c.px*y - c.py*x
	w := math.Atan2(hypot3(cx, cy, cz), c.px*x+c.py*y+c.pz*z)
	c.w1 += w
	c.x1 += w * (c.px + x)
	c.y1 += w * (c.py + y)
	c.z1 += w * (c.pz + z)
	c.px, c.py, c.pz = x, y, z
	c.addPoint(x, y, z)
}

// ringPoint accumulates both the arc weight and the area weight. The area term
// is J. E. Brock's inertia-tensor result for a spherical triangle: the
// normalized cross product of consecutive vertices, scaled by the arc angle.
func (c *centroidState) ringPoint(lon, lat float64) {
	lambda, phi := lon*radians, lat*radians
	cosPhi := math.Cos(phi)
	x, y, z := cosPhi*math.Cos(lambda), cosPhi*math.Sin(lambda), math.Sin(phi)
	cx := c.py*z - c.pz*y
	cy := c.pz*x - c.px*z
	cz := c.px*y - c.py*x
	m := hypot3(cx, cy, cz)
	w := asin(m)
	v := 0.0
	if m != 0 {
		v = -w / m
	}
	c.x2.Add(v * cx)
	c.y2.Add(v * cy)
	c.z2.Add(v * cz)
	c.w1 += w
	c.x1 += w * (c.px + x)
	c.y1 += w * (c.py + y)
	c.z1 += w * (c.pz + z)
	c.px, c.py, c.pz = x, y, z
	c.addPoint(x, y, z)
}

// Centroid returns the spherical centroid of a GeoJSON object, in degrees.
//
// The weighting falls back through three levels, which is what makes the answer
// useful for mixed input: an area-weighted centroid if the object encloses
// area, otherwise a length-weighted centroid along its lines, otherwise the
// arithmetic mean of its points. Objects with no defined centroid — a pair of
// antipodes, an empty object — return (NaN, NaN).
func Centroid(o Object) (lon, lat float64) {
	var c centroidState
	StreamObject(o, &c)

	x, y, z := c.x2.Value(), c.y2.Value(), c.z2.Value()
	m := hypot3(x, y, z)
	if m < epsilon2 {
		x, y, z = c.x1, c.y1, c.z1
		if c.w1 < epsilon {
			x, y, z = c.x0v, c.y0v, c.z0v
		}
		m = hypot3(x, y, z)
		if m < epsilon2 {
			return math.NaN(), math.NaN()
		}
	}
	return math.Atan2(y, x) * degrees, asin(z/m) * degrees
}

// --- great-circle interpolation ----------------------------------------------

// Interpolate returns a function that walks the great-circle arc — the shortest
// path over the sphere — from one point to another, in degrees. t = 0 is the
// start, t = 1 the end, and values outside [0, 1] extend the arc.
//
// This is not linear interpolation of longitude and latitude, which would cut
// through the interior of the Earth and, near the poles, produce a path bearing
// no resemblance to the route a plane flies.
//
// The second result is the angular distance between the two points in radians,
// the same value [Distance] returns. When it is zero the returned function is
// constant.
func Interpolate(lon0, lat0, lon1, lat1 float64) (func(t float64) (lon, lat float64), float64) {
	x0, y0 := lon0*radians, lat0*radians
	x1, y1 := lon1*radians, lat1*radians
	cy0, sy0 := math.Cos(y0), math.Sin(y0)
	cy1, sy1 := math.Cos(y1), math.Sin(y1)
	kx0, ky0 := cy0*math.Cos(x0), cy0*math.Sin(x0)
	kx1, ky1 := cy1*math.Cos(x1), cy1*math.Sin(x1)
	d := 2 * asin(math.Sqrt(haversin(y1-y0)+cy0*cy1*haversin(x1-x0)))
	k := math.Sin(d)

	if d == 0 {
		return func(float64) (float64, float64) { return x0 * degrees, y0 * degrees }, 0
	}
	return func(t float64) (float64, float64) {
		t *= d
		b := math.Sin(t) / k
		a := math.Sin(d-t) / k
		x := a*kx0 + b*kx1
		y := a*ky0 + b*ky1
		z := a*sy0 + b*sy1
		return math.Atan2(y, x) * degrees, math.Atan2(z, math.Sqrt(x*x+y*y)) * degrees
	}, d
}
