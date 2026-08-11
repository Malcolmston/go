// Package contour is a Go port of d3-contour: isolines and isobands over a
// rectangular grid of values, and two-dimensional kernel density estimation
// that produces such a grid from scattered points.
//
// [Contours] runs marching squares. Given a grid of values and a threshold, it
// returns the region where the value is at or above the threshold, as a
// GeoJSON-shaped [MultiPolygon] whose exterior rings are wound one way and
// whose holes are wound the other. Given several thresholds it returns one
// MultiPolygon per threshold, and because the regions are nested — everything
// above 20 is inside everything above 10 — the result stacks into the familiar
// filled contour map.
//
//	c := contour.NewContours().Size(width, height).ThresholdCount(10)
//	for _, mp := range c.Compute(values) {
//		// mp.Value is the threshold, mp.Coordinates the polygons
//	}
//
// [ContourDensity] is the other half: it bins scattered points onto a grid,
// blurs the grid with a Gaussian approximation, and contours the result, which
// is the standard way to draw a density heat map of a point cloud too large to
// plot honestly as a scatterplot.
//
// # The coordinate system
//
// Grid value i sits at coordinate (i%dx + 0.5, i/dx + 0.5) — cell centers, not
// cell corners. A ring therefore runs along cell boundaries, at half-integer
// coordinates, and the outermost possible ring is the rectangle from (0, 0) to
// (dx, dy) with its four corners cut. This is d3's convention and matters as
// soon as the output is scaled: multiply by a cell size and the ring lands
// correctly between the pixels it separates.
//
// # Smoothing
//
// With smoothing on (the default), a ring vertex that crosses a cell edge is
// moved along that edge by linear interpolation between the two values it lies
// between, so a contour of a smooth field comes out smooth. With smoothing off
// every vertex stays at the midpoint of its edge, which makes every segment
// axis-aligned or diagonal at 45° — visibly blocky, and occasionally what you
// want when the grid is categorical rather than sampled.
//
// # Deviations from d3-contour
//
// Configuration follows the port-wide write-only fluent convention (see
// API-DEVIATIONS.md): setters take the value, return the receiver and have no
// getter. d3's overloaded thresholds() is split into [Contours.Thresholds] for
// an explicit list and [Contours.ThresholdCount] for a count. Geometry is
// returned as typed values rather than untyped GeoJSON objects. Invalid
// configuration (a size below 1, a values slice whose length is not dx*dy, a
// non-finite threshold) panics rather than being silently accepted, since there
// is no meaningful contour to return instead.
package contour

import (
	"math"
	"sort"

	"github.com/malcolmston/d3/array"
)

// Point is a position in grid coordinates.
type Point struct{ X, Y float64 }

// Ring is a closed sequence of points: the last equals the first.
type Ring []Point

// Polygon is an exterior ring followed by its holes.
type Polygon []Ring

// MultiPolygon is the region at or above one threshold, shaped like the GeoJSON
// geometry of the same name.
type MultiPolygon struct {
	// Type is always "MultiPolygon", so the value marshals to valid GeoJSON.
	Type string `json:"type"`
	// Value is the threshold this region was computed for. GeoJSON has no such
	// member; d3 adds it too, because a contour without its level is useless.
	Value       float64   `json:"value"`
	Coordinates []Polygon `json:"coordinates"`
}

// Contours computes the contours of a grid of values.
//
// The zero value is not usable; construct with [NewContours].
type Contours struct {
	dx, dy int
	smooth bool

	// thresholds, when non-nil, is the explicit list; otherwise count decides,
	// and count == 0 means Sturges' formula.
	thresholds []float64
	count      int
}

// NewContours returns a Contours over a 1×1 grid, with smoothing on and the
// threshold count chosen by Sturges' formula. [Contours.Size] is effectively
// mandatory.
func NewContours() *Contours {
	return &Contours{dx: 1, dy: 1, smooth: true}
}

// Size sets the grid dimensions in cells. Both must be at least 1.
func (c *Contours) Size(dx, dy int) *Contours {
	if dx < 1 || dy < 1 {
		panic("contour: invalid size")
	}
	c.dx, c.dy = dx, dy
	return c
}

// Smooth turns linear interpolation of ring vertices along cell edges on or
// off. It is on by default; see the package documentation for what it buys.
func (c *Contours) Smooth(on bool) *Contours { c.smooth = on; return c }

// Thresholds sets the explicit list of thresholds, one contour per entry. The
// list is copied and sorted ascending.
func (c *Contours) Thresholds(t []float64) *Contours {
	c.thresholds = append([]float64(nil), t...)
	sort.Float64s(c.thresholds)
	return c
}

// ThresholdCount asks for approximately n uniformly spaced thresholds, chosen
// with the same nice-round-number algorithm the scales use ([array.Ticks]), so
// contour levels read as 10, 20, 30 rather than 11.37, 22.74. A count of zero
// restores the default, Sturges' formula.
func (c *Contours) ThresholdCount(n int) *Contours {
	c.thresholds = nil
	if n < 0 {
		n = 0
	}
	c.count = n
	return c
}

// Compute returns one MultiPolygon per threshold, in ascending threshold order.
// values must have exactly dx*dy entries, in row-major order.
func (c *Contours) Compute(values []float64) []MultiPolygon {
	c.checkValues(values)
	tz := c.thresholdsFor(values)
	out := make([]MultiPolygon, 0, len(tz))
	for _, t := range tz {
		out = append(out, c.ComputeOne(values, t))
	}
	return out
}

// ComputeOne returns the single contour at the given threshold, ignoring the
// configured thresholds.
func (c *Contours) ComputeOne(values []float64, threshold float64) MultiPolygon {
	c.checkValues(values)
	if math.IsNaN(threshold) {
		panic("contour: invalid threshold")
	}

	var polygons []Polygon
	var holes []Ring

	c.isorings(values, threshold, func(ring Ring) {
		if c.smooth {
			c.smoothRing(ring, values, threshold)
		}
		// Winding is what distinguishes a shell from a hole, and marching
		// squares produces it for free: the region above the threshold is
		// always on the same side of a segment, so a ring enclosing high values
		// winds one way and a ring enclosing a pocket of low values the other.
		if ringArea(ring) > 0 {
			polygons = append(polygons, Polygon{ring})
		} else {
			holes = append(holes, ring)
		}
	})

	for _, hole := range holes {
		for i := range polygons {
			if ringContainsRing(polygons[i][0], hole) != -1 {
				polygons[i] = append(polygons[i], hole)
				break
			}
		}
	}

	return MultiPolygon{Type: "MultiPolygon", Value: threshold, Coordinates: polygons}
}

func (c *Contours) checkValues(values []float64) {
	if len(values) != c.dx*c.dy {
		panic("contour: values length does not match size")
	}
}

// thresholdsFor resolves the configured thresholds against the data.
func (c *Contours) thresholdsFor(values []float64) []float64 {
	if c.thresholds != nil {
		return c.thresholds
	}
	n := c.count
	lo, hi, ok := finiteExtent(values)
	if n == 0 {
		n = array.ThresholdSturges(values, lo, hi)
	}
	if !ok {
		return nil
	}
	niceLo, niceHi := array.NiceTicks(lo, hi, n)
	tz := array.Ticks(niceLo, niceHi, n)
	// Drop the levels that no value reaches, and the leading ones so far below
	// the data that their contour would just be the whole grid repeated.
	for len(tz) > 0 && tz[len(tz)-1] >= hi {
		tz = tz[:len(tz)-1]
	}
	for len(tz) > 1 && tz[1] < lo {
		tz = tz[1:]
	}
	return tz
}

func finiteExtent(values []float64) (lo, hi float64, ok bool) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		ok = true
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return lo, hi, ok
}

// cases is the marching-squares lookup table: for each of the sixteen possible
// above/below patterns of a cell's four corners, the line segments separating
// them.
//
// Bit 0 is the corner at local (0.5, 1.5), bit 1 at (1.5, 1.5), bit 2 at
// (1.5, 0.5) and bit 3 at (0.5, 0.5); segment endpoints are the midpoints of
// the four edges of that square. Every segment is directed so that the region
// above the threshold lies to its right, which is what makes the stitched rings
// come out consistently wound and lets [ringArea]'s sign distinguish shells
// from holes. The two ambiguous saddle cases (5 and 10) are resolved by
// separating each above-corner individually, matching d3.
var cases = [16][][2]Point{
	{},
	{{{1.0, 1.5}, {0.5, 1.0}}},
	{{{1.5, 1.0}, {1.0, 1.5}}},
	{{{1.5, 1.0}, {0.5, 1.0}}},
	{{{1.0, 0.5}, {1.5, 1.0}}},
	{{{1.0, 1.5}, {0.5, 1.0}}, {{1.0, 0.5}, {1.5, 1.0}}},
	{{{1.0, 0.5}, {1.0, 1.5}}},
	{{{1.0, 0.5}, {0.5, 1.0}}},
	{{{0.5, 1.0}, {1.0, 0.5}}},
	{{{1.0, 1.5}, {1.0, 0.5}}},
	{{{0.5, 1.0}, {1.0, 0.5}}, {{1.5, 1.0}, {1.0, 1.5}}},
	{{{1.5, 1.0}, {1.0, 0.5}}},
	{{{0.5, 1.0}, {1.5, 1.0}}},
	{{{1.0, 1.5}, {1.5, 1.0}}},
	{{{0.5, 1.0}, {1.0, 1.5}}},
	{},
}

// fragment is a partially built ring, indexed by the grid position of each end
// so that a new segment can be attached in constant time.
type fragment struct {
	start, end int
	ring       Ring
}

// isorings walks the grid once, emitting the segments of every cell and
// stitching them into closed rings as it goes. A ring is reported the moment
// its two ends meet, so no post-pass over disconnected segments is needed.
//
// The grid is walked one row of *cells* beyond its edges in each direction —
// the loops start at -1 — with the out-of-bounds corners treated as below the
// threshold. That is what closes a region running off the edge of the grid.
func (c *Contours) isorings(values []float64, value float64, callback func(Ring)) {
	dx, dy := c.dx, c.dy
	byStart := map[int]*fragment{}
	byEnd := map[int]*fragment{}

	var x, y int

	index := func(p Point) int {
		return int(math.Round(p.X*2)) + int(math.Round(p.Y*2))*(dx+1)*2
	}

	stitch := func(seg [2]Point) {
		start := Point{seg[0].X + float64(x), seg[0].Y + float64(y)}
		end := Point{seg[1].X + float64(x), seg[1].Y + float64(y)}
		si, ei := index(start), index(end)

		if f, ok := byEnd[si]; ok {
			if g, ok2 := byStart[ei]; ok2 {
				delete(byEnd, f.end)
				delete(byStart, g.start)
				if f == g {
					f.ring = append(f.ring, end)
					callback(f.ring)
				} else {
					ring := make(Ring, 0, len(f.ring)+len(g.ring))
					ring = append(ring, f.ring...)
					ring = append(ring, g.ring...)
					nf := &fragment{start: f.start, end: g.end, ring: ring}
					byStart[nf.start] = nf
					byEnd[nf.end] = nf
				}
			} else {
				delete(byEnd, f.end)
				f.ring = append(f.ring, end)
				f.end = ei
				byEnd[ei] = f
			}
		} else if f, ok := byStart[ei]; ok {
			delete(byStart, f.start)
			f.ring = append(Ring{start}, f.ring...)
			f.start = si
			byStart[si] = f
		} else {
			f := &fragment{start: si, end: ei, ring: Ring{start, end}}
			byStart[si] = f
			byEnd[ei] = f
		}
	}

	emit := func(k int) {
		for _, seg := range cases[k] {
			stitch(seg)
		}
	}

	above := func(v float64) int {
		if v >= value {
			return 1
		}
		return 0
	}

	// First row of cells (y == -1): the two lower corners are outside.
	x, y = -1, -1
	t1 := above(values[0])
	emit(t1 << 1)
	for x = 0; x < dx-1; x++ {
		t0 := t1
		t1 = above(values[x+1])
		emit(t0 | t1<<1)
	}
	x = dx - 1
	emit(t1)

	// Interior rows.
	for y = 0; y < dy-1; y++ {
		x = -1
		t1 = above(values[y*dx+dx])
		t2 := above(values[y*dx])
		emit(t1<<1 | t2<<2)
		for x = 0; x < dx-1; x++ {
			t0 := t1
			t3 := t2
			t1 = above(values[y*dx+dx+x+1])
			t2 = above(values[y*dx+x+1])
			emit(t0 | t1<<1 | t2<<2 | t3<<3)
		}
		x = dx - 1
		emit(t1 | t2<<3)
	}

	// Last row of cells: the two upper corners are outside.
	y = dy - 1
	x = -1
	t2 := above(values[y*dx])
	emit(t2 << 2)
	for x = 0; x < dx-1; x++ {
		t3 := t2
		t2 = above(values[y*dx+x+1])
		emit(t2<<2 | t3<<3)
	}
	x = dx - 1
	emit(t2 << 3)
}

// smoothRing moves each vertex along the cell edge it sits on, to where the
// value would actually reach the threshold under linear interpolation.
//
// A vertex has one integer coordinate (the axis along which it can slide) and
// one half-integer coordinate (the edge it is pinned to). Vertices on the outer
// boundary of the grid have no second value to interpolate against and are left
// where they are.
func (c *Contours) smoothRing(ring Ring, values []float64, value float64) {
	dx, dy := c.dx, c.dy
	valid := func(v float64) float64 {
		if math.IsNaN(v) {
			return math.Inf(-1)
		}
		return v
	}
	for i, p := range ring {
		x, y := p.X, p.Y
		xt, yt := int(x), int(y)
		if xt < 0 || xt >= dx || yt < 0 || yt >= dy {
			continue
		}
		v1 := valid(values[yt*dx+xt])
		if x > 0 && x < float64(dx) && float64(xt) == x {
			ring[i].X = smooth1(x, valid(values[yt*dx+xt-1]), v1, value)
		}
		if y > 0 && y < float64(dy) && float64(yt) == y {
			ring[i].Y = smooth1(y, valid(values[(yt-1)*dx+xt]), v1, value)
		}
	}
}

// smooth1 places the crossing between the cell centered half a unit below x
// (value v0) and the one centered half a unit above it (value v1).
//
// The fraction d is 0 when the threshold equals v0 and 1 when it equals v1, so
// the result sweeps the full unit between the two cell centers. Non-finite
// values — which is how missing data arrives here — collapse to a sign ratio,
// and a ratio that is not a number at all leaves the vertex at the midpoint.
func smooth1(x, v0, v1, value float64) float64 {
	a := value - v0
	b := v1 - v0
	var d float64
	if !math.IsInf(a, 0) || !math.IsInf(b, 0) {
		d = a / b
	} else {
		d = sign(a) / sign(b)
	}
	if math.IsNaN(d) {
		return x
	}
	return x + d - 0.5
}

func sign(v float64) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// ringArea returns twice the signed area of a ring, positive for an exterior
// ring under this package's winding convention.
func ringArea(ring Ring) float64 {
	n := len(ring)
	if n < 3 {
		return 0
	}
	a := ring[n-1].Y*ring[0].X - ring[n-1].X*ring[0].Y
	for i := 1; i < n; i++ {
		a += ring[i-1].Y*ring[i].X - ring[i-1].X*ring[i].Y
	}
	return a
}

// ringContainsRing reports whether hole lies inside ring: 1 inside, -1 outside,
// 0 when every vertex of the hole lies on the ring itself.
//
// It tests hole vertices one at a time and answers with the first that is not
// on the boundary, which is enough because marching squares can never produce
// two rings that cross.
func ringContainsRing(ring, hole Ring) int {
	for _, p := range hole {
		if c := ringContains(ring, p); c != 0 {
			return c
		}
	}
	return 0
}

// ringContains is the even-odd crossing test, returning 0 for a point lying
// exactly on the ring.
func ringContains(ring Ring, point Point) int {
	contains := -1
	n := len(ring)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		pi, pj := ring[i], ring[j]
		if segmentContains(pi, pj, point) {
			return 0
		}
		if (pi.Y > point.Y) != (pj.Y > point.Y) &&
			point.X < (pj.X-pi.X)*(point.Y-pi.Y)/(pj.Y-pi.Y)+pi.X {
			contains = -contains
		}
	}
	return contains
}

func segmentContains(a, b, c Point) bool {
	if (b.X-a.X)*(c.Y-a.Y) != (b.Y-a.Y)*(c.X-a.X) {
		return false
	}
	// Compare along whichever axis the segment actually spans.
	if a.X == b.X {
		return within(a.Y, c.Y, b.Y)
	}
	return within(a.X, c.X, b.X)
}

func within(p, q, r float64) bool {
	return (p <= q && q <= r) || (r <= q && q <= p)
}
