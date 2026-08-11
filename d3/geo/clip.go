package geo

import "sort"

// This file holds the machinery shared by antimeridian clipping and circle
// clipping. Both answer the same question — which parts of a geometry are on
// the visible side of a boundary, and how do the surviving pieces get sewn back
// into closed polygons — and differ only in three plug-ins: a visibility test,
// a line cutter, and an interpolator that walks along the boundary.
//
// Getting this right is what stops a world map from smearing: a country that
// straddles the antimeridian, cut naively, produces a polygon whose edge runs
// all the way back across the map. The rejoin step below is what prevents that,
// by closing each cut piece along the boundary instead of across the map.

// clipPoint is (λ, φ, m). The third slot marks a synthesized point: it is set
// when circle clipping had to invent an intersection, and rejoin uses it to
// decide whether a closed segment is a genuine ring or an artifact.
type clipPoint = [3]float64

// clipBuffer is a Stream that records lines instead of drawing them, so a ring
// can be cut into segments and inspected before anything is emitted.
type clipBuffer struct {
	lines [][]clipPoint
	line  []clipPoint
}

func (b *clipBuffer) Point(x, y, m float64) {
	if len(b.lines) == 0 {
		b.lines = append(b.lines, nil)
	}
	b.line = append(b.line, clipPoint{x, y, m})
	b.lines[len(b.lines)-1] = b.line
}
func (b *clipBuffer) LineStart() {
	b.line = nil
	b.lines = append(b.lines, nil)
}
func (b *clipBuffer) LineEnd()      {}
func (b *clipBuffer) PolygonStart() {}
func (b *clipBuffer) PolygonEnd()   {}
func (b *clipBuffer) Sphere()       {}

// rejoin concatenates the last recorded line onto the first. A ring cut at a
// point that happens to be its own start would otherwise arrive as two segments
// that are really one.
func (b *clipBuffer) rejoin() {
	if len(b.lines) > 1 {
		last := b.lines[len(b.lines)-1]
		b.lines = b.lines[:len(b.lines)-1]
		b.lines[0] = append(last, b.lines[0]...)
	}
}

func (b *clipBuffer) result() [][]clipPoint {
	r := b.lines
	b.lines, b.line = nil, nil
	return r
}

// lineClipper is d3's clipLine: a sub-stream that consumes one line and writes
// its visible pieces downstream, reporting afterwards (via clean) what happened.
//
// clean is a two-bit code: bit 0 set means there were no intersections, bit 1
// set means the first and last segments belong together and should be rejoined.
type lineClipper struct {
	lineStart func()
	point     func(lambda, phi float64)
	lineEnd   func()
	clean     func() int
}

// intersection is a node in the doubly linked lists that rejoin walks: one list
// ordered along the input geometry, one ordered along the clip boundary.
type intersection struct {
	x    clipPoint   // the intersection point
	z    []clipPoint // the segment leaving (entry) or arriving at (exit) it
	o    *intersection
	e    bool // is this an entry into the visible region?
	v    bool // visited
	n, p *intersection
}

func link(a []*intersection) {
	n := len(a)
	if n == 0 {
		return
	}
	for i := 0; i < n-1; i++ {
		a[i].n = a[i+1]
		a[i+1].p = a[i]
	}
	a[n-1].n = a[0]
	a[0].p = a[n-1]
}

// rejoin sews clipped line segments back into closed rings.
//
// Each segment enters the visible region at one end and leaves at the other.
// Sorting the endpoints along the clip boundary and alternating entry/exit
// flags turns "which piece of boundary joins these two segments?" into "walk to
// the next node". The traversal then alternates between following a segment
// (through the geometry) and interpolating along the boundary, which is what
// closes the polygon *on the boundary* rather than across the map.
func rejoin(segments [][]clipPoint, compare func(a, b *intersection) bool, startInside bool,
	interpolate func(from, to *clipPoint, direction float64, s Stream), stream Stream) {

	var subject, clip []*intersection

	for _, segment := range segments {
		n := len(segment) - 1
		if n <= 0 {
			continue
		}
		p0, p1 := segment[0], segment[n]

		if pointEqual(p0, p1) {
			if p0[2] == 0 && p1[2] == 0 {
				// A genuine closed ring, untouched by the boundary.
				stream.LineStart()
				for i := 0; i < n; i++ {
					stream.Point(segment[i][0], segment[i][1], 0)
				}
				stream.LineEnd()
				continue
			}
			// Degenerate: nudge the end so the two nodes are distinguishable.
			segment[n][0] += 2 * epsilon
			p1 = segment[n]
		}

		a := &intersection{x: p0, z: segment, e: true}
		b := &intersection{x: p0, o: a, e: false}
		a.o = b
		subject = append(subject, a)
		clip = append(clip, b)

		c := &intersection{x: p1, z: segment, e: false}
		d := &intersection{x: p1, o: c, e: true}
		c.o = d
		subject = append(subject, c)
		clip = append(clip, d)
	}

	if len(subject) == 0 {
		return
	}

	sort.SliceStable(clip, func(i, j int) bool { return compare(clip[i], clip[j]) })
	link(subject)
	link(clip)

	// Entry/exit alternates along the boundary; where it starts depends on
	// whether the untouched geometry began inside the visible region.
	for _, c := range clip {
		startInside = !startInside
		c.e = startInside
	}

	start := subject[0]
	for {
		current := start
		isSubject := true
		for current.v {
			current = current.n
			if current == start {
				return
			}
		}
		points := current.z
		stream.LineStart()
		for {
			current.v, current.o.v = true, true
			if current.e {
				if isSubject {
					for _, pt := range points {
						stream.Point(pt[0], pt[1], 0)
					}
				} else {
					from, to := current.x, current.n.x
					interpolate(&from, &to, 1, stream)
				}
				current = current.n
			} else {
				if isSubject {
					points = current.p.z
					for i := len(points) - 1; i >= 0; i-- {
						stream.Point(points[i][0], points[i][1], 0)
					}
				} else {
					from, to := current.x, current.p.x
					interpolate(&from, &to, -1, stream)
				}
				current = current.p
			}
			current = current.o
			points = current.z
			isSubject = !isSubject
			if current.v {
				break
			}
		}
		stream.LineEnd()
	}
}

// compareIntersection orders intersections along the clip boundary, which for
// both the antimeridian and a clip circle is the same traverse: down the
// western half, then up the eastern half.
func compareIntersection(a, b *intersection) bool {
	return intersectionKey(a.x) < intersectionKey(b.x)
}

func intersectionKey(a clipPoint) float64 {
	if a[0] < 0 {
		return a[1] - halfPi - epsilon
	}
	return halfPi - a[1]
}

// newClip builds a clipping stream factory out of the three plug-ins that
// distinguish one clip region from another:
//
//   - pointVisible: is a bare point inside the region?
//   - clipLine: cut a line into visible pieces.
//   - interpolate: walk along the boundary from one point to another; a nil
//     from means "emit the whole boundary", which is how a fully-visible sphere
//     gets its outline.
//   - start: a point known to be *outside* the region, used to decide whether
//     an uncut ring encloses the region or is enclosed by it.
func newClip(
	pointVisible func(lambda, phi float64) bool,
	clipLine func(Stream) *lineClipper,
	interpolate func(from, to *clipPoint, direction float64, s Stream),
	start clipPoint,
) func(Stream) Stream {
	return func(sink Stream) Stream {
		line := clipLine(sink)
		ringBuffer := &clipBuffer{}
		ringSink := clipLine(ringBuffer)

		polygonStarted := false
		var polygon [][][2]float64
		var segments [][]clipPoint
		var ring [][2]float64

		s := &funcStream{}

		point := func(lambda, phi, _ float64) {
			if pointVisible(lambda, phi) {
				sink.Point(lambda, phi, 0)
			}
		}

		pointRing := func(lambda, phi float64) {
			ring = append(ring, [2]float64{lambda, phi})
			ringSink.point(lambda, phi)
		}

		pointLine := func(lambda, phi, _ float64) { line.point(lambda, phi) }
		lineStart := func() {
			s.point = pointLine
			line.lineStart()
		}
		lineEnd := func() {
			s.point = point
			line.lineEnd()
		}

		s.point, s.lineStart, s.lineEnd = point, lineStart, lineEnd

		ringStart := func() {
			ringSink.lineStart()
			ring = nil
		}

		ringEnd := func() {
			if len(ring) == 0 {
				ringSink.lineEnd()
				return
			}
			pointRing(ring[0][0], ring[0][1])
			ringSink.lineEnd()

			clean := ringSink.clean()
			ringSegments := ringBuffer.result()

			ring = ring[:len(ring)-1]
			polygon = append(polygon, ring)
			ring = nil

			n := len(ringSegments)
			if n == 0 {
				return
			}

			// No intersections: the ring is entirely inside or entirely outside.
			if clean&1 != 0 {
				segment := ringSegments[0]
				if m := len(segment) - 1; m > 0 {
					if !polygonStarted {
						sink.PolygonStart()
						polygonStarted = true
					}
					sink.LineStart()
					for i := 0; i < m; i++ {
						sink.Point(segment[i][0], segment[i][1], 0)
					}
					sink.LineEnd()
				}
				return
			}

			if n > 1 && clean&2 != 0 {
				last := ringSegments[n-1]
				ringSegments = ringSegments[:n-1]
				ringSegments[0] = append(last, ringSegments[0]...)
			}
			for _, seg := range ringSegments {
				if len(seg) > 1 {
					segments = append(segments, seg)
				}
			}
		}

		s.polygonStart = func() {
			s.point = func(lambda, phi, _ float64) { pointRing(lambda, phi) }
			s.lineStart = ringStart
			s.lineEnd = ringEnd
			segments = nil
			polygon = nil
		}

		s.polygonEnd = func() {
			s.point, s.lineStart, s.lineEnd = point, lineStart, lineEnd

			startInside := polygonContains(polygon, start[0], start[1])
			if len(segments) > 0 {
				if !polygonStarted {
					sink.PolygonStart()
					polygonStarted = true
				}
				rejoin(segments, compareIntersection, startInside, interpolate, sink)
			} else if startInside {
				// Nothing was cut, yet the region is inside the polygon: the
				// whole clip boundary is the visible outline.
				if !polygonStarted {
					sink.PolygonStart()
					polygonStarted = true
				}
				sink.LineStart()
				interpolate(nil, nil, 1, sink)
				sink.LineEnd()
			}
			if polygonStarted {
				sink.PolygonEnd()
				polygonStarted = false
			}
			segments, polygon = nil, nil
		}

		s.sphere = func() {
			sink.PolygonStart()
			sink.LineStart()
			interpolate(nil, nil, 1, sink)
			sink.LineEnd()
			sink.PolygonEnd()
		}

		return s
	}
}
