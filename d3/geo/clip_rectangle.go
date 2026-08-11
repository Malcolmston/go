package geo

import "math"

const (
	clipMax = 1e9
	clipMin = -clipMax
)

// clipRectangleLine is the Liang–Barsky segment clip. It reports whether any of
// the segment survives, and rewrites a and b to the surviving portion.
//
// The divisions by a zero delta are deliberate: they produce ±Inf, and the
// comparisons that follow are then exactly the "parallel to this edge" cases,
// which is why the algorithm needs no special-casing for axis-aligned segments.
func clipRectangleLine(a, b *[2]float64, x0, y0, x1, y1 float64) bool {
	ax, ay := a[0], a[1]
	bx, by := b[0], b[1]
	t0, t1 := 0.0, 1.0
	dx, dy := bx-ax, by-ay

	r := x0 - ax
	if dx == 0 && r > 0 {
		return false
	}
	r /= dx
	if dx < 0 {
		if r < t0 {
			return false
		}
		if r < t1 {
			t1 = r
		}
	} else if dx > 0 {
		if r > t1 {
			return false
		}
		if r > t0 {
			t0 = r
		}
	}

	r = x1 - ax
	if dx == 0 && r < 0 {
		return false
	}
	r /= dx
	if dx < 0 {
		if r > t1 {
			return false
		}
		if r > t0 {
			t0 = r
		}
	} else if dx > 0 {
		if r < t0 {
			return false
		}
		if r < t1 {
			t1 = r
		}
	}

	r = y0 - ay
	if dy == 0 && r > 0 {
		return false
	}
	r /= dy
	if dy < 0 {
		if r < t0 {
			return false
		}
		if r < t1 {
			t1 = r
		}
	} else if dy > 0 {
		if r > t1 {
			return false
		}
		if r > t0 {
			t0 = r
		}
	}

	r = y1 - ay
	if dy == 0 && r < 0 {
		return false
	}
	r /= dy
	if dy < 0 {
		if r > t1 {
			return false
		}
		if r > t0 {
			t0 = r
		}
	} else if dy > 0 {
		if r < t0 {
			return false
		}
		if r < t1 {
			t1 = r
		}
	}

	if t0 > 0 {
		a[0], a[1] = ax+t0*dx, ay+t0*dy
	}
	if t1 < 1 {
		b[0], b[1] = ax+t1*dx, ay+t1*dy
	}
	return true
}

// clipRectangle clips *projected* geometry to an axis-aligned rectangle — the
// viewport. It is [Projection.ClipExtent], and it runs after projection, where
// antimeridian and circle clipping run before it.
//
// The polygon case is the interesting one: rather than clip each ring
// independently, the whole polygon is buffered, then rejoined along the
// rectangle's edges, so a country cut by the left edge of the viewport comes
// out closed along that edge instead of open.
func clipRectangle(x0, y0, x1, y1 float64) func(Stream) Stream {
	visible := func(x, y float64) bool {
		return x0 <= x && x <= x1 && y0 <= y && y <= y1
	}

	// corner numbers the rectangle's sides 0..3 so that walking from one
	// intersection to the next is modular arithmetic.
	corner := func(p clipPoint, direction float64) int {
		switch {
		case math.Abs(p[0]-x0) < epsilon:
			if direction > 0 {
				return 0
			}
			return 3
		case math.Abs(p[0]-x1) < epsilon:
			if direction > 0 {
				return 2
			}
			return 1
		case math.Abs(p[1]-y0) < epsilon:
			if direction > 0 {
				return 1
			}
			return 0
		default:
			if direction > 0 {
				return 3
			}
			return 2
		}
	}

	comparePoint := func(a, b clipPoint) float64 {
		ca, cb := corner(a, 1), corner(b, 1)
		switch {
		case ca != cb:
			return float64(ca - cb)
		case ca == 0:
			return b[1] - a[1]
		case ca == 1:
			return a[0] - b[0]
		case ca == 2:
			return a[1] - b[1]
		default:
			return b[0] - a[0]
		}
	}

	interpolate := func(from, to *clipPoint, direction float64, stream Stream) {
		a, a1 := 0, 0
		if from != nil {
			a = corner(*from, direction)
			a1 = corner(*to, direction)
		}
		if from == nil || a != a1 || (comparePoint(*from, *to) < 0) != (direction > 0) {
			for {
				px, py := x1, y0
				if a == 0 || a == 3 {
					px = x0
				}
				if a > 1 {
					py = y1
				}
				stream.Point(px, py, 0)
				a = (a + int(direction) + 4) % 4
				if a == a1 {
					break
				}
			}
			return
		}
		stream.Point(to[0], to[1], 0)
	}

	compare := func(a, b *intersection) bool { return comparePoint(a.x, b.x) < 0 }

	return func(stream Stream) Stream {
		activeStream := stream
		bufferStream := &clipBuffer{}
		var segments [][]clipPoint
		var polygon [][][2]float64
		var ring [][2]float64
		inPolygon := false

		var xx, yy float64 // first point of the current line
		var vv bool
		var xp, yp float64 // previous point
		var vp bool
		first := false
		clean := true

		s := &funcStream{}

		point := func(x, y, _ float64) {
			if visible(x, y) {
				activeStream.Point(x, y, 0)
			}
		}

		// polygonInside counts how many rings wind around the rectangle's
		// top-left corner, which decides whether an uncut polygon covers the
		// whole viewport.
		polygonInside := func() int {
			winding := 0
			for _, r := range polygon {
				if len(r) == 0 {
					continue
				}
				b0, b1 := r[0][0], r[0][1]
				for j := 1; j < len(r); j++ {
					a0, a1 := b0, b1
					b0, b1 = r[j][0], r[j][1]
					if a1 <= y1 {
						if b1 > y1 && (b0-a0)*(y1-a1) > (b1-a1)*(x0-a0) {
							winding++
						}
					} else if b1 <= y1 && (b0-a0)*(y1-a1) < (b1-a1)*(x0-a0) {
						winding--
					}
				}
			}
			return winding
		}

		linePoint := func(x, y, _ float64) {
			v := visible(x, y)
			if inPolygon {
				ring = append(ring, [2]float64{x, y})
			}
			if first {
				xx, yy, vv = x, y, v
				first = false
				if v {
					activeStream.LineStart()
					activeStream.Point(x, y, 0)
				}
			} else if v && vp {
				activeStream.Point(x, y, 0)
			} else {
				a := [2]float64{
					math.Max(clipMin, math.Min(clipMax, xp)),
					math.Max(clipMin, math.Min(clipMax, yp)),
				}
				bx := math.Max(clipMin, math.Min(clipMax, x))
				by := math.Max(clipMin, math.Min(clipMax, y))
				b := [2]float64{bx, by}
				xp, yp = a[0], a[1]
				x, y = bx, by
				if clipRectangleLine(&a, &b, x0, y0, x1, y1) {
					if !vp {
						activeStream.LineStart()
						activeStream.Point(a[0], a[1], 0)
					}
					activeStream.Point(b[0], b[1], 0)
					if !v {
						activeStream.LineEnd()
					}
					clean = false
				} else if v {
					activeStream.LineStart()
					activeStream.Point(x, y, 0)
					clean = false
				}
			}
			xp, yp, vp = x, y, v
		}

		lineStart := func() {
			s.point = linePoint
			if inPolygon {
				polygon = append(polygon, nil)
				ring = nil
			}
			first = true
			vp = false
			xp, yp = math.NaN(), math.NaN()
		}

		lineEnd := func() {
			if inPolygon {
				linePoint(xx, yy, 0)
				polygon[len(polygon)-1] = ring
				if vv && vp {
					bufferStream.rejoin()
				}
				segments = append(segments, bufferStream.result()...)
			}
			s.point = point
			if vp {
				activeStream.LineEnd()
			}
		}

		s.point, s.lineStart, s.lineEnd = point, lineStart, lineEnd

		s.polygonStart = func() {
			inPolygon = true
			activeStream = bufferStream
			segments, polygon, clean = nil, nil, true
		}

		s.polygonEnd = func() {
			startInside := polygonInside() != 0
			cleanInside := clean && startInside
			hasSegments := len(segments) > 0
			if cleanInside || hasSegments {
				stream.PolygonStart()
				if cleanInside {
					stream.LineStart()
					interpolate(nil, nil, 1, stream)
					stream.LineEnd()
				}
				if hasSegments {
					rejoin(segments, compare, startInside, interpolate, stream)
				}
				stream.PolygonEnd()
			}
			inPolygon = false
			activeStream = stream
			segments, polygon, ring = nil, nil, nil
		}

		s.sphere = stream.Sphere
		return s
	}
}
