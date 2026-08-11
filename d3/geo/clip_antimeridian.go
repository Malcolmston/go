package geo

import "math"

// clipAntimeridian cuts geometry at ±180°.
//
// This is the default pre-clip for every projection, and it is what stops a
// polygon like Russia — whose vertices run from 19°E round to 169°W — from
// being drawn as a band across the entire map in the wrong direction. Every
// point is visible; what changes is that a segment crossing the antimeridian is
// split in two, each half terminating at the meridian at the latitude where the
// great circle actually crosses it, and polygons are re-closed along ±180° and,
// where they enclose a pole, along the top or bottom edge of the map.
var clipAntimeridian = newClip(
	func(float64, float64) bool { return true },
	clipAntimeridianLine,
	clipAntimeridianInterpolate,
	clipPoint{-pi, -halfPi},
)

func clipAntimeridianLine(stream Stream) *lineClipper {
	lambda0, phi0 := math.NaN(), math.NaN()
	sign0 := math.NaN()
	clean := 0

	c := &lineClipper{}
	c.lineStart = func() {
		stream.LineStart()
		clean = 1
	}
	c.point = func(lambda1, phi1 float64) {
		sign1 := -pi
		if lambda1 > 0 {
			sign1 = pi
		}
		delta := math.Abs(lambda1 - lambda0)

		switch {
		case math.Abs(delta-pi) < epsilon:
			// The segment passes over a pole: run up to the pole, across the
			// top of the map, and back down on the far side.
			if (phi0+phi1)/2 > 0 {
				phi0 = halfPi
			} else {
				phi0 = -halfPi
			}
			stream.Point(lambda0, phi0, 0)
			stream.Point(sign0, phi0, 0)
			stream.LineEnd()
			stream.LineStart()
			stream.Point(sign1, phi0, 0)
			stream.Point(lambda1, phi0, 0)
			clean = 0
		case sign0 != sign1 && delta >= pi:
			// The segment crosses the antimeridian.
			if math.Abs(lambda0-sign0) < epsilon {
				lambda0 -= sign0 * epsilon
			}
			if math.Abs(lambda1-sign1) < epsilon {
				lambda1 -= sign1 * epsilon
			}
			phi0 = clipAntimeridianIntersect(lambda0, phi0, lambda1, phi1)
			stream.Point(sign0, phi0, 0)
			stream.LineEnd()
			stream.LineStart()
			stream.Point(sign1, phi0, 0)
			clean = 0
		}
		lambda0, phi0 = lambda1, phi1
		stream.Point(lambda0, phi0, 0)
		sign0 = sign1
	}
	c.lineEnd = func() {
		stream.LineEnd()
		lambda0, phi0 = math.NaN(), math.NaN()
	}
	c.clean = func() int { return 2 - clean }
	return c
}

// clipAntimeridianIntersect returns the latitude at which the great circle
// through the two points crosses the antimeridian.
func clipAntimeridianIntersect(lambda0, phi0, lambda1, phi1 float64) float64 {
	sinLambda0Lambda1 := math.Sin(lambda0 - lambda1)
	if math.Abs(sinLambda0Lambda1) <= epsilon {
		return (phi0 + phi1) / 2
	}
	cosPhi0, cosPhi1 := math.Cos(phi0), math.Cos(phi1)
	return math.Atan((math.Sin(phi0)*cosPhi1*math.Sin(lambda1) -
		math.Sin(phi1)*cosPhi0*math.Sin(lambda0)) /
		(cosPhi0 * cosPhi1 * sinLambda0Lambda1))
}

// clipAntimeridianInterpolate walks the clip boundary. With no endpoints it
// traces the whole frame of the map — down one edge of the antimeridian, along
// a pole, and back up the other — which is the outline a Sphere draws.
func clipAntimeridianInterpolate(from, to *clipPoint, direction float64, stream Stream) {
	if from == nil {
		phi := direction * halfPi
		stream.Point(-pi, phi, 0)
		stream.Point(0, phi, 0)
		stream.Point(pi, phi, 0)
		stream.Point(pi, 0, 0)
		stream.Point(pi, -phi, 0)
		stream.Point(0, -phi, 0)
		stream.Point(-pi, -phi, 0)
		stream.Point(-pi, 0, 0)
		stream.Point(-pi, phi, 0)
		return
	}
	if math.Abs(from[0]-to[0]) > epsilon {
		lambda := -pi
		if from[0] < to[0] {
			lambda = pi
		}
		phi := direction * lambda / 2
		stream.Point(-lambda, phi, 0)
		stream.Point(0, phi, 0)
		stream.Point(lambda, phi, 0)
		return
	}
	stream.Point(to[0], to[1], 0)
}
