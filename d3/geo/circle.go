package geo

import "math"

// circleStream emits a small circle of angular radius radius (radians) about
// the point (0, 0) of a rotated frame, stepping by delta radians.
//
// direction is +1 or -1. from and to, when present, are the endpoints of an
// arc; when absent the whole circle is emitted. It is used both by [Circle] and
// by circle clipping, which needs to walk along the clip boundary between two
// intersections — that shared use is why the function takes an arc rather than
// only a full circle.
func circleStream(s Stream, radius, delta, direction float64, from, to *[3]float64) {
	if delta == 0 {
		return
	}
	cosRadius, sinRadius := math.Cos(radius), math.Sin(radius)
	step := direction * delta
	var t0, t1 float64
	if from == nil {
		t0 = radius + direction*tau
		t1 = radius - step/2
	} else {
		t0 = circleRadius(cosRadius, *from)
		t1 = circleRadius(cosRadius, *to)
		if (direction > 0 && t0 < t1) || (direction < 0 && t0 > t1) {
			t0 += direction * tau
		}
	}
	for t := t0; (direction > 0 && t > t1) || (direction < 0 && t < t1); t -= step {
		lon, lat := spherical(vec3{cosRadius, -sinRadius * math.Cos(t), -sinRadius * math.Sin(t)})
		s.Point(lon, lat, 0)
	}
}

// circleRadius returns the signed angle of a point relative to [cosRadius,0,0].
func circleRadius(cosRadius float64, point [3]float64) float64 {
	p := cartesian(point[0], point[1])
	p[0] -= cosRadius
	p = cartesianNormalize(p)
	r := acos(-p[1])
	if -p[2] < 0 {
		r = -r
	}
	return math.Mod(r+tau-epsilon, tau)
}

// Circle generates a GeoJSON polygon approximating a circle on the sphere — the
// set of points a fixed angular distance from a center.
//
// It is a real circle on the globe, so a large one is nothing like an ellipse
// on the map: a 60° circle around the north pole is the 30°N parallel, and
// under most projections it draws as a straight line across the top.
//
//	c := geo.NewCircle().Center(-75, 40).Radius(20).Generate()
type Circle struct {
	lon, lat  float64
	radius    float64
	precision float64
}

// NewCircle returns a circle centered at (0, 0) with a radius of 90° and a
// precision of 6°, matching d3's defaults.
func NewCircle() *Circle { return &Circle{radius: 90, precision: 6} }

// Center sets the circle's center in degrees.
func (c *Circle) Center(lon, lat float64) *Circle { c.lon, c.lat = lon, lat; return c }

// Radius sets the angular radius in degrees.
func (c *Circle) Radius(r float64) *Circle { c.radius = r; return c }

// Precision sets the angular step between generated vertices, in degrees.
// Smaller is smoother and larger; 6° is d3's default and is enough for a
// circle drawn at typical map scales.
func (c *Circle) Precision(p float64) *Circle { c.precision = p; return c }

// Generate returns the circle as a single-ring [Polygon] in degrees.
func (c *Circle) Generate() *Polygon {
	rot := rotateRadians(-c.lon*radians, -c.lat*radians, 0)
	var ring []Position
	s := &funcStream{point: func(x, y, _ float64) {
		lon, lat := rot.inv(x, y)
		ring = append(ring, P(lon*degrees, lat*degrees))
	}}
	circleStream(s, c.radius*radians, c.precision*radians, 1, nil, nil)
	return &Polygon{Coordinates: [][]Position{ring}}
}
