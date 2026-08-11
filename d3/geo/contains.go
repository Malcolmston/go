package geo

import "math"

// longitudeWrap normalizes a longitude in radians into (-π, π].
func longitudeWrap(l float64) float64 {
	if math.Abs(l) <= pi {
		return l
	}
	return sgn(l) * (math.Mod(math.Abs(l)+pi, tau) - pi)
}

// polygonContains reports whether a point lies inside a spherical polygon.
//
// polygon is a list of rings in radians, each *without* its repeated closing
// point; point is (λ, φ) in radians.
//
// Two facts are combined. First, the total signed angle swept by the rings
// decides whether the south pole is inside: a clockwise winding around it, or
// no winding but a negative area, means it is. Second, the number of times the
// polygon's edges cross the meridian arc from the point down to the south pole
// decides whether the point is on the same side as the pole. Parity of the two
// gives the answer, and it works for rings that contain a pole or wrap the
// antimeridian — the cases where a planar ray-cast silently fails.
func polygonContains(polygon [][][2]float64, pointLambda, pointPhi float64) bool {
	lambda := longitudeWrap(pointLambda)
	phi := pointPhi
	sinPhi := math.Sin(phi)
	normal := vec3{math.Sin(lambda), -math.Cos(lambda), 0}
	angle := 0.0
	winding := 0
	var sum adder

	// Nudge an exactly-polar query point off the singularity.
	if sinPhi == 1 {
		phi = halfPi + epsilon
	} else if sinPhi == -1 {
		phi = -halfPi - epsilon
	}

	for _, ring := range polygon {
		m := len(ring)
		if m == 0 {
			continue
		}
		point0 := ring[m-1]
		lambda0 := longitudeWrap(point0[0])
		phi0 := point0[1]/2 + quarterPi
		sinPhi0, cosPhi0 := math.Sin(phi0), math.Cos(phi0)

		for j := 0; j < m; j++ {
			point1 := ring[j]
			lambda1 := longitudeWrap(point1[0])
			phi1 := point1[1]/2 + quarterPi
			sinPhi1, cosPhi1 := math.Sin(phi1), math.Cos(phi1)
			delta := lambda1 - lambda0
			sign := 1.0
			if delta < 0 {
				sign = -1
			}
			absDelta := sign * delta
			antimeridian := absDelta > pi
			k := sinPhi0 * sinPhi1

			sum.Add(math.Atan2(k*sign*math.Sin(absDelta), cosPhi0*cosPhi1+k*math.Cos(absDelta)))
			if antimeridian {
				angle += delta + sign*tau
			} else {
				angle += delta
			}

			// Does this edge straddle the point's meridian?
			if antimeridian != (lambda0 >= lambda) != (lambda1 >= lambda) {
				arc := cartesianNormalize(cartesianCross(
					cartesian(point0[0], point0[1]),
					cartesian(point1[0], point1[1]),
				))
				intersection := cartesianNormalize(cartesianCross(normal, arc))
				s := 1.0
				if antimeridian != (delta >= 0) {
					s = -1
				}
				phiArc := s * asin(intersection[2])
				if phi > phiArc || (phi == phiArc && (arc[0] != 0 || arc[1] != 0)) {
					if antimeridian != (delta >= 0) {
						winding++
					} else {
						winding--
					}
				}
			}

			lambda0, sinPhi0, cosPhi0, point0 = lambda1, sinPhi1, cosPhi1, point1
		}
	}

	southPoleInside := angle < -epsilon || (angle < epsilon && sum.Value() < -epsilon2)
	return southPoleInside != (winding&1 == 1)
}

// Contains reports whether a GeoJSON object contains the given point, in
// degrees.
//
// The test is spherical, not planar: polygon edges are great-circle arcs, rings
// may enclose a pole or cross the antimeridian, and none of that needs special
// handling by the caller.
//
// What "contains" means depends on the geometry: a Point contains only itself
// (to sub-micrometre tolerance), a LineString contains points lying on it (to a
// small
// tolerance), a Polygon contains its interior with holes excluded, a
// [Sphere] contains everything, and a collection contains a point if any member
// does. Points *on* a polygon's boundary are not reliably classified — that is
// upstream behavior and is inherent to floating point, not a gap here.
func Contains(o Object, lon, lat float64) bool {
	switch g := o.(type) {
	case nil:
		return false
	case *Sphere:
		return true
	case *Point:
		return containsPoint(g.Coordinates, lon, lat)
	case *MultiPoint:
		for _, c := range g.Coordinates {
			if containsPoint(c, lon, lat) {
				return true
			}
		}
	case *LineString:
		return containsLine(g.Coordinates, lon, lat)
	case *MultiLineString:
		for _, l := range g.Coordinates {
			if containsLine(l, lon, lat) {
				return true
			}
		}
	case *Polygon:
		return containsPolygon(g.Coordinates, lon, lat)
	case *MultiPolygon:
		for _, p := range g.Coordinates {
			if containsPolygon(p, lon, lat) {
				return true
			}
		}
	case *GeometryCollection:
		for _, child := range g.Geometries {
			if Contains(child, lon, lat) {
				return true
			}
		}
	case *Feature:
		if g.Geometry != nil {
			return Contains(g.Geometry, lon, lat)
		}
	case *FeatureCollection:
		for _, f := range g.Features {
			if f != nil && Contains(f, lon, lat) {
				return true
			}
		}
	}
	return false
}

// containsPoint is d3's "distance is exactly zero" test, relaxed to a
// tolerance. Exact equality is not portable: on a target where the compiler
// fuses multiply-add, converting two identical degree values to radians and
// subtracting can leave a residue of a few times 1e-18 rather than zero.
// epsilon2 radians is under a micrometre on Earth, so the relaxation costs
// nothing and the exact test costs correctness.
func containsPoint(c Position, lon, lat float64) bool {
	return Distance(c.Lon(), c.Lat(), lon, lat) < epsilon2
}

// containsLine tests each segment by the triangle inequality: the point is on
// the segment when the detour through it costs nothing. The (1 - ((ao-bo)/ab)²)
// factor is d3's, and it is what keeps the tolerance from ballooning near the
// segment's endpoints.
func containsLine(coords []Position, lon, lat float64) bool {
	var ao float64
	for i, c := range coords {
		bo := Distance(c.Lon(), c.Lat(), lon, lat)
		if bo < epsilon2 {
			return true
		}
		if i > 0 {
			prev := coords[i-1]
			ab := Distance(c.Lon(), c.Lat(), prev.Lon(), prev.Lat())
			if ab > 0 && ao <= ab && bo <= ab {
				r := (ao - bo) / ab
				if (ao+bo-ab)*(1-r*r) < epsilon2*ab {
					return true
				}
			}
		}
		ao = bo
	}
	return false
}

func containsPolygon(rings [][]Position, lon, lat float64) bool {
	poly := make([][][2]float64, 0, len(rings))
	for _, ring := range rings {
		n := len(ring)
		if n == 0 {
			continue
		}
		// Drop the repeated closing position: polygonContains closes rings itself.
		r := make([][2]float64, 0, n-1)
		for _, c := range ring[:n-1] {
			r = append(r, [2]float64{c.Lon() * radians, c.Lat() * radians})
		}
		poly = append(poly, r)
	}
	return polygonContains(poly, lon*radians, lat*radians)
}
