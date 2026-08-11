package geo

import "math"

const resampleMaxDepth = 16

var cosMinDistance = math.Cos(30 * radians)

// resample inserts extra points along projected segments so that curved lines
// stay curved.
//
// A meridian is a straight line in geographic coordinates and a curve in most
// projections. Projecting only its endpoints draws a chord. Resampling
// subdivides each segment recursively, keeping a midpoint whenever the
// projected midpoint strays from the chord by more than delta2 (the square of
// [Projection.Precision]) — an adaptive scheme, so a nearly-straight segment
// costs nothing and a strongly-curved one gets as many points as it needs.
//
// The two extra tests beside perpendicular distance matter more than they look:
// one rejects midpoints that land near an endpoint (which would recurse
// forever without improving anything), and one forces subdivision of segments
// spanning more than 30° of arc, where the chord can pass close to the true
// curve while being on the wrong side of the globe entirely.
func resample(project xform, delta2 float64) func(Stream) Stream {
	if delta2 == 0 {
		return func(stream Stream) Stream {
			return &funcStream{
				point: func(x, y, _ float64) {
					px, py := project.fwd(x, y)
					stream.Point(px, py, 0)
				},
				lineStart:    stream.LineStart,
				lineEnd:      stream.LineEnd,
				polygonStart: stream.PolygonStart,
				polygonEnd:   stream.PolygonEnd,
				sphere:       stream.Sphere,
			}
		}
	}

	var resampleLineTo func(x0, y0, lambda0, a0, b0, c0, x1, y1, lambda1, a1, b1, c1 float64, depth int, stream Stream)
	resampleLineTo = func(x0, y0, lambda0, a0, b0, c0, x1, y1, lambda1, a1, b1, c1 float64, depth int, stream Stream) {
		dx, dy := x1-x0, y1-y0
		d2 := dx*dx + dy*dy
		if !(d2 > 4*delta2) || depth == 0 {
			return
		}
		depth--
		a, b, c := a0+a1, b0+b1, c0+c1
		m := math.Sqrt(a*a + b*b + c*c)
		c /= m
		phi2 := asin(c)
		var lambda2 float64
		if math.Abs(math.Abs(c)-1) < epsilon || math.Abs(lambda0-lambda1) < epsilon {
			lambda2 = (lambda0 + lambda1) / 2
		} else {
			lambda2 = math.Atan2(b, a)
		}
		x2, y2 := project.fwd(lambda2, phi2)
		dx2, dy2 := x2-x0, y2-y0
		dz := dy*dx2 - dx*dy2
		if dz*dz/d2 > delta2 || // perpendicular distance from the chord
			math.Abs((dx*dx2+dy*dy2)/d2-0.5) > 0.3 || // midpoint too near an end
			a0*a1+b0*b1+c0*c1 < cosMinDistance { // more than 30° of arc
			a /= m
			b /= m
			resampleLineTo(x0, y0, lambda0, a0, b0, c0, x2, y2, lambda2, a, b, c, depth, stream)
			stream.Point(x2, y2, 0)
			resampleLineTo(x2, y2, lambda2, a, b, c, x1, y1, lambda1, a1, b1, c1, depth, stream)
		}
	}

	return func(stream Stream) Stream {
		var lambda00, x00, y00, a00, b00, c00 float64 // first point of a ring
		var lambda0, x0, y0, a0, b0, c0 float64       // previous point

		s := &funcStream{}

		point := func(x, y, _ float64) {
			px, py := project.fwd(x, y)
			stream.Point(px, py, 0)
		}

		linePoint := func(lambda, phi float64) {
			v := cartesian(lambda, phi)
			px, py := project.fwd(lambda, phi)
			resampleLineTo(x0, y0, lambda0, a0, b0, c0, px, py, lambda, v[0], v[1], v[2],
				resampleMaxDepth, stream)
			x0, y0, lambda0, a0, b0, c0 = px, py, lambda, v[0], v[1], v[2]
			stream.Point(x0, y0, 0)
		}

		lineStart := func() {
			// NaN makes the first resampleLineTo's d2 comparison false, which is
			// how the first point of a line skips subdivision.
			x0 = math.NaN()
			s.point = func(x, y, _ float64) { linePoint(x, y) }
			stream.LineStart()
		}
		lineEnd := func() {
			s.point = point
			stream.LineEnd()
		}

		var ringEnd func()
		ringPoint := func(x, y, _ float64) {
			linePoint(x, y)
			lambda00, x00, y00, a00, b00, c00 = x, x0, y0, a0, b0, c0
			s.point = func(px, py, _ float64) { linePoint(px, py) }
		}
		ringStart := func() {
			lineStart()
			s.point = ringPoint
			s.lineEnd = ringEnd
		}
		ringEnd = func() {
			resampleLineTo(x0, y0, lambda0, a0, b0, c0, x00, y00, lambda00, a00, b00, c00,
				resampleMaxDepth, stream)
			s.lineEnd = lineEnd
			lineEnd()
		}

		s.point, s.lineStart, s.lineEnd = point, lineStart, lineEnd
		s.polygonStart = func() {
			stream.PolygonStart()
			s.lineStart = ringStart
		}
		s.polygonEnd = func() {
			stream.PolygonEnd()
			s.lineStart = lineStart
		}
		s.sphere = stream.Sphere
		return s
	}
}
