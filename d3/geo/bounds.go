package geo

import (
	"math"
	"sort"
)

// boundsState computes a spherical bounding box.
//
// This is the operation that a planar min/max gets wrong, and it is worth
// saying how. Longitude is circular, so "the smallest box containing these
// points" is not min/max but *the complement of the largest empty gap* — a
// dataset spanning Fiji is bounded by [177°, -178°], a range that wraps the
// antimeridian and whose lower value exceeds its upper. Latitude is not
// circular, but a great-circle arc between two points bulges poleward of both,
// so the extreme latitude of a segment is usually at neither endpoint; it is at
// the arc's inflection, which is found here by intersecting the arc's plane
// with the plane through the poles.
//
// Polygons add a third case: a ring that encircles a pole contains it, so the
// box must extend to ±90° even though no vertex does.
type boundsState struct {
	lambda0, phi0, lambda1, phi1 float64 // the box under construction
	lambda2                      float64 // previous longitude
	lambda00, phi00              float64 // first point of a ring
	p0                           vec3    // previous point, Cartesian
	hasP0                        bool
	deltaSum                     adder
	ranges                       [][2]float64
	rangeIdx                     int // index into ranges of the current run
	area                         areaState
	inPolygon                    bool
	inLine                       bool
	first                        bool
}

func newBoundsState() *boundsState {
	return &boundsState{
		lambda0: math.Inf(1), phi0: math.Inf(1),
		lambda1: math.Inf(-1), phi1: math.Inf(-1),
		rangeIdx: -1,
	}
}

func (b *boundsState) Sphere() {
	b.lambda0, b.lambda1 = -180, 180
	b.phi0, b.phi1 = -90, 90
}

func (b *boundsState) PolygonStart() {
	b.inPolygon = true
	b.deltaSum.Reset()
	b.area.PolygonStart()
}

func (b *boundsState) PolygonEnd() {
	b.area.PolygonEnd()
	b.inPolygon = false
	switch {
	case b.area.ringSum.Value() < 0:
		// Negative spherical area: the ring encloses everything outside it, so
		// the bounds are the whole globe.
		b.lambda0, b.lambda1 = -180, 180
		b.phi0, b.phi1 = -90, 90
	case b.deltaSum.Value() > epsilon:
		b.phi1 = 90 // winds around the north pole
	case b.deltaSum.Value() < -epsilon:
		b.phi0 = -90 // winds around the south pole
	}
	b.setRange()
}

func (b *boundsState) LineStart() {
	b.inLine, b.first = true, true
	if b.inPolygon {
		b.area.LineStart()
	}
}

func (b *boundsState) LineEnd() {
	if b.inPolygon {
		b.ringPoint(b.lambda00, b.phi00)
		b.area.LineEnd()
		if math.Abs(b.deltaSum.Value()) > epsilon {
			// The ring's longitudes wrap all the way around.
			b.lambda0, b.lambda1 = -180, 180
		}
	}
	b.setRange()
	b.inLine = false
	b.hasP0 = false
}

func (b *boundsState) setRange() {
	if b.rangeIdx >= 0 {
		b.ranges[b.rangeIdx] = [2]float64{b.lambda0, b.lambda1}
	}
}

func (b *boundsState) Point(x, y, _ float64) {
	switch {
	case b.inPolygon:
		if b.first {
			b.first = false
			b.lambda00, b.phi00 = x, y
		}
		b.ringPoint(x, y)
	case b.inLine:
		b.first = false
		b.linePoint(x, y)
	default:
		b.pushRange(x)
		if y < b.phi0 {
			b.phi0 = y
		}
		if y > b.phi1 {
			b.phi1 = y
		}
	}
}

func (b *boundsState) ringPoint(lambda, phi float64) {
	if b.hasP0 {
		delta := lambda - b.lambda2
		// d3's expression, kept verbatim, and it is not the usual wrap-to-±180:
		// a delta of -240° becomes -600°, not +120°. The effect on a ring that
		// crosses the antimeridian once is to negate the total, which is exactly
		// what makes the sign test below read "positive means the ring encircles
		// the north pole". Replacing it with an ordinary wrap inverts the answer.
		if math.Abs(delta) > 180 {
			if delta > 0 {
				delta += 360
			} else {
				delta -= 360
			}
		}
		b.deltaSum.Add(delta)
	}
	b.area.Point(lambda, phi, 0)
	b.linePoint(lambda, phi)
}

func (b *boundsState) pushRange(lambda float64) {
	b.lambda0, b.lambda1 = lambda, lambda
	b.ranges = append(b.ranges, [2]float64{lambda, lambda})
	b.rangeIdx = len(b.ranges) - 1
}

func (b *boundsState) linePoint(lambda, phi float64) {
	p := cartesian(lambda*radians, phi*radians)
	if b.hasP0 {
		normal := cartesianCross(b.p0, p)
		// The great circle through the two points and the one through the poles
		// meet at the arc's northernmost and southernmost points.
		equatorial := vec3{normal[1], -normal[0], 0}
		inflection := cartesianNormalize(cartesianCross(equatorial, normal))
		infLambda, infPhi := spherical(inflection)

		delta := lambda - b.lambda2
		sign := 1.0
		if delta <= 0 {
			sign = -1
		}
		lambdai := infLambda * degrees * sign
		antimeridian := math.Abs(delta) > 180

		switch {
		case antimeridian != (sign*b.lambda2 < lambdai && lambdai < sign*lambda):
			if phii := infPhi * degrees; phii > b.phi1 {
				b.phi1 = phii
			}
		default:
			lambdai = math.Mod(lambdai+360, 360) - 180
			if antimeridian != (sign*b.lambda2 < lambdai && lambdai < sign*lambda) {
				if phii := -infPhi * degrees; phii < b.phi0 {
					b.phi0 = phii
				}
			} else {
				if phi < b.phi0 {
					b.phi0 = phi
				}
				if phi > b.phi1 {
					b.phi1 = phi
				}
			}
		}

		if antimeridian {
			if lambda < b.lambda2 {
				if angleBetween(b.lambda0, lambda) > angleBetween(b.lambda0, b.lambda1) {
					b.lambda1 = lambda
				}
			} else {
				if angleBetween(lambda, b.lambda1) > angleBetween(b.lambda0, b.lambda1) {
					b.lambda0 = lambda
				}
			}
		} else if b.lambda1 >= b.lambda0 {
			if lambda < b.lambda0 {
				b.lambda0 = lambda
			}
			if lambda > b.lambda1 {
				b.lambda1 = lambda
			}
		} else if lambda > b.lambda2 {
			if angleBetween(b.lambda0, lambda) > angleBetween(b.lambda0, b.lambda1) {
				b.lambda1 = lambda
			}
		} else {
			if angleBetween(lambda, b.lambda1) > angleBetween(b.lambda0, b.lambda1) {
				b.lambda0 = lambda
			}
		}
	} else {
		b.pushRange(lambda)
	}
	if phi < b.phi0 {
		b.phi0 = phi
	}
	if phi > b.phi1 {
		b.phi1 = phi
	}
	b.p0, b.hasP0 = p, true
	b.lambda2 = lambda
}

// angleBetween is the eastward distance from lambda0 to lambda1 in degrees.
// Unlike (lambda1-lambda0+360)%360 it reports 360, not 0, for a full turn,
// which is what makes "the largest gap" well defined.
func angleBetween(lambda0, lambda1 float64) float64 {
	d := lambda1 - lambda0
	if d < 0 {
		return d + 360
	}
	return d
}

func rangeContains(r [2]float64, x float64) bool {
	if r[0] <= r[1] {
		return r[0] <= x && x <= r[1]
	}
	return x < r[0] || r[1] < x
}

// Bounds returns the spherical bounding box of a GeoJSON object as
// (lon0, lat0, lon1, lat1) in degrees: the southwest corner followed by the
// northeast corner.
//
// lon0 may be greater than lon1. That is not a bug — it means the box crosses
// the antimeridian, and it is the only way to describe such a box without
// splitting it in two. An object with nothing to bound gives all NaN.
//
// One known imprecision, inherited from d3: a polygon whose ring sum is
// negative — which includes any polygon enclosing the north pole, such as a
// north polar cap or the northern hemisphere — falls back to the whole globe,
// [[-180, -90], [180, 90]], rather than a tight box. Polygons enclosing the
// south pole are bounded tightly. The asymmetry is d3's and is reproduced here
// so that output matches; if you need a tight box around a north polar cap,
// bound its vertices yourself and force the latitude to 90.
func Bounds(o Object) (lon0, lat0, lon1, lat1 float64) {
	b := newBoundsState()
	StreamObject(o, b)

	if n := len(b.ranges); n > 0 {
		ranges := b.ranges
		sort.SliceStable(ranges, func(i, j int) bool { return ranges[i][0] < ranges[j][0] })

		// Merge overlapping longitude runs.
		merged := [][2]float64{ranges[0]}
		a := &merged[0]
		for i := 1; i < n; i++ {
			r := ranges[i]
			if rangeContains(*a, r[0]) || rangeContains(*a, r[1]) {
				if angleBetween(a[0], r[1]) > angleBetween(a[0], a[1]) {
					a[1] = r[1]
				}
				if angleBetween(r[0], a[1]) > angleBetween(a[0], a[1]) {
					a[0] = r[0]
				}
			} else {
				merged = append(merged, r)
				a = &merged[len(merged)-1]
			}
		}

		// The bounding box is the complement of the largest gap between runs.
		deltaMax := math.Inf(-1)
		m := len(merged) - 1
		prev := merged[m]
		for i := 0; i <= m; i++ {
			cur := merged[i]
			if d := angleBetween(prev[1], cur[0]); d > deltaMax {
				deltaMax, b.lambda0, b.lambda1 = d, cur[0], prev[1]
			}
			prev = cur
		}
	}

	if math.IsInf(b.lambda0, 1) || math.IsInf(b.phi0, 1) {
		n := math.NaN()
		return n, n, n, n
	}
	return b.lambda0, b.phi0, b.lambda1, b.phi1
}
