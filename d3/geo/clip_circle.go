package geo

import "math"

// clipCircle clips to a small circle of angular radius radius (radians) about
// the projection's center — [Projection.ClipAngle].
//
// This is what makes a globe look like a globe. An orthographic projection can
// only show a hemisphere, so everything beyond 90° has to be removed before it
// is projected, or the far side of the Earth folds back over the near side. A
// gnomonic projection diverges at 90° and is normally clipped to 60°.
//
// The geometry is harder than antimeridian clipping in one specific way: a
// segment can enter *and* leave the circle without either endpoint being
// inside, so each segment is tested for two intersections as well as one, and
// the "large circle" case (radius > 90°, where the visible region is the
// complement of a small circle) inverts the sense of nearly every test.
func clipCircle(radius float64) func(Stream) Stream {
	cr := math.Cos(radius)
	delta := 6 * radians
	smallRadius := cr > 0
	notHemisphere := math.Abs(cr) > epsilon

	visible := func(lambda, phi float64) bool {
		return math.Cos(lambda)*math.Cos(phi) > cr
	}

	interpolate := func(from, to *clipPoint, direction float64, stream Stream) {
		circleStream(stream, radius, delta, direction, from, to)
	}

	// code is a Cohen–Sutherland-style outcode against the circle's bounding
	// box in (λ, φ). It is only a cheap rejection: two points whose codes share
	// a bit cannot possibly bracket the circle.
	code := func(lambda, phi float64) int {
		r := radius
		if !smallRadius {
			r = pi - radius
		}
		c := 0
		if lambda < -r {
			c |= 1
		} else if lambda > r {
			c |= 2
		}
		if phi < -r {
			c |= 4
		} else if phi > r {
			c |= 8
		}
		return c
	}

	// intersect finds where the great circle through a and b meets the clip
	// circle. Both are planes through the origin, so their intersection is a
	// line; the points where that line meets the unit sphere are the answers.
	// n reports how many of them lie on the arc between a and b.
	intersect := func(a, b clipPoint, two bool) (q0, q1 clipPoint, n int) {
		pa := cartesian(a[0], a[1])
		pb := cartesian(b[0], b[1])

		n1 := vec3{1, 0, 0}
		n2 := cartesianCross(pa, pb)
		n2n2 := cartesianDot(n2, n2)
		n1n2 := n2[0]
		determinant := n2n2 - n1n2*n1n2

		if determinant == 0 {
			// The two points are polar opposites of the clip axis.
			if !two {
				return a, clipPoint{}, 1
			}
			return clipPoint{}, clipPoint{}, 0
		}

		c1 := cr * n2n2 / determinant
		c2 := -cr * n1n2 / determinant
		n1xn2 := cartesianCross(n1, n2)
		A := cartesianAdd(cartesianScale(n1, c1), cartesianScale(n2, c2))

		u := n1xn2
		w := cartesianDot(A, u)
		uu := cartesianDot(u, u)
		t2 := w*w - uu*(cartesianDot(A, A)-1)
		if t2 < 0 {
			return clipPoint{}, clipPoint{}, 0
		}

		t := math.Sqrt(t2)
		q := cartesianAdd(cartesianScale(u, (-w-t)/uu), A)
		ql, qp := spherical(q)
		qq := clipPoint{ql, qp, 0}
		if !two {
			return qq, clipPoint{}, 1
		}

		// Two intersections: keep them only if the first lies between a and b.
		lambda0, lambda1 := a[0], b[0]
		phi0, phi1 := a[1], b[1]
		if lambda1 < lambda0 {
			lambda0, lambda1 = lambda1, lambda0
		}
		d := lambda1 - lambda0
		polar := math.Abs(d-pi) < epsilon
		meridian := polar || d < epsilon
		if !polar && phi1 < phi0 {
			phi0, phi1 = phi1, phi0
		}

		var between bool
		switch {
		case meridian && polar:
			bound := phi1
			if math.Abs(qq[0]-lambda0) < epsilon {
				bound = phi0
			}
			between = (phi0+phi1 > 0) != (qq[1] < bound)
		case meridian:
			between = phi0 <= qq[1] && qq[1] <= phi1
		default:
			between = (d > pi) != (lambda0 <= qq[0] && qq[0] <= lambda1)
		}
		if !between {
			return clipPoint{}, clipPoint{}, 0
		}
		q2 := cartesianAdd(cartesianScale(u, (-w+t)/uu), A)
		l2, p2 := spherical(q2)
		return qq, clipPoint{l2, p2, 0}, 2
	}

	clipLine := func(stream Stream) *lineClipper {
		var point0 clipPoint
		hasPoint0 := false
		c0 := 0
		v0, v00 := false, false
		clean := 0

		c := &lineClipper{}
		c.lineStart = func() {
			v00, v0 = false, false
			clean = 1
			hasPoint0 = false
		}
		c.point = func(lambda, phi float64) {
			point1 := clipPoint{lambda, phi, 0}
			v := visible(lambda, phi)
			var cc int
			if smallRadius {
				if !v {
					cc = code(lambda, phi)
				}
			} else if v {
				l := lambda + pi
				if lambda >= 0 {
					l = lambda - pi
				}
				cc = code(l, phi)
			}

			if !hasPoint0 {
				v00, v0 = v, v
				if v {
					stream.LineStart()
				}
			}

			if v != v0 {
				p2, _, n := intersect(point0, point1, false)
				if n == 0 || pointEqual(point0, p2) || pointEqual(point1, p2) {
					point1[2] = 1
				}
			}

			switch {
			case v != v0:
				clean = 0
				if v {
					// Outside going in.
					stream.LineStart()
					p2, _, n := intersect(point1, point0, false)
					if n > 0 {
						stream.Point(p2[0], p2[1], 0)
						point0, hasPoint0 = p2, true
					}
				} else {
					// Inside going out.
					p2, _, n := intersect(point0, point1, false)
					if n > 0 {
						stream.Point(p2[0], p2[1], 2)
						point0, hasPoint0 = p2, true
					}
					stream.LineEnd()
				}
			case notHemisphere && hasPoint0 && smallRadius != v:
				// Both endpoints on the same side, but the segment may still
				// cross the circle twice.
				if cc&c0 == 0 {
					if t0, t1, n := intersect(point1, point0, true); n == 2 {
						clean = 0
						if smallRadius {
							stream.LineStart()
							stream.Point(t0[0], t0[1], 0)
							stream.Point(t1[0], t1[1], 0)
							stream.LineEnd()
						} else {
							stream.Point(t1[0], t1[1], 0)
							stream.LineEnd()
							stream.LineStart()
							stream.Point(t0[0], t0[1], 3)
						}
					}
				}
			}

			if v && (!hasPoint0 || !pointEqual(point0, point1)) {
				stream.Point(point1[0], point1[1], 0)
			}
			point0, hasPoint0 = point1, true
			v0, c0 = v, cc
		}
		c.lineEnd = func() {
			if v0 {
				stream.LineEnd()
			}
			hasPoint0 = false
		}
		c.clean = func() int {
			b := 0
			if v00 && v0 {
				b = 2
			}
			return clean | b
		}
		return c
	}

	start := clipPoint{0, -radius, 0}
	if !smallRadius {
		start = clipPoint{-pi, radius - pi, 0}
	}
	return newClip(visible, clipLine, interpolate, start)
}
