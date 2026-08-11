package delaunay

import (
	"math"

	"github.com/malcolmston/d3/path"
)

// Voronoi is the Voronoi diagram dual to a [Delaunay] triangulation, clipped to
// a rectangle.
//
// The duality is direct: each Voronoi vertex is the circumcenter of a Delaunay
// triangle, and each Voronoi edge joins the circumcenters of two triangles that
// share a Delaunay edge. That is the whole construction, and it is why building
// the diagram costs a single pass over the triangles once the triangulation
// exists. What costs real code is the clipping — a cell on the hull is
// unbounded, so it has to be closed against the rectangle, walking the
// rectangle's corners in the right order.
type Voronoi struct {
	d *Delaunay

	// circumcenters holds one point per triangle, interleaved.
	circumcenters []float64
	// vectors holds, per point, two outward ray directions (four floats:
	// incoming then outgoing). They are zero for interior points, which is
	// exactly the test for "is this cell unbounded".
	vectors []float64

	xmin, ymin, xmax, ymax float64

	// lastEdgeJ carries the insertion index out of edge, which splices corners
	// into a slice and so cannot return both the slice and the new position
	// through one value the way JavaScript's in-place splice lets d3 do. It
	// also makes explicit that a Voronoi is not safe for concurrent use.
	lastEdgeJ int
}

// Bounds of the clipping rectangle: the four sides of a region code.
const (
	edgeLeft   = 0b0001
	edgeRight  = 0b0010
	edgeTop    = 0b0100
	edgeBottom = 0b1000
)

func newVoronoi(d *Delaunay, xmin, ymin, xmax, ymax float64) *Voronoi {
	if !(xmax >= xmin) || !(ymax >= ymin) || !allFinite(xmin, ymin, xmax, ymax) {
		panic("delaunay: invalid Voronoi extent")
	}
	v := &Voronoi{d: d, xmin: xmin, ymin: ymin, xmax: xmax, ymax: ymax}
	v.init()
	return v
}

// Delaunay returns the triangulation this diagram is dual to.
func (v *Voronoi) Delaunay() *Delaunay { return v.d }

// Extent returns the clipping rectangle.
func (v *Voronoi) Extent() (xmin, ymin, xmax, ymax float64) {
	return v.xmin, v.ymin, v.xmax, v.ymax
}

func (v *Voronoi) init() {
	d := v.d
	points := d.points
	triangles := d.triangles
	hull := d.hull

	nTri := len(triangles) / 3
	v.circumcenters = make([]float64, nTri*2)
	v.vectors = make([]float64, len(points)*2)

	haveBary := false
	var bx, by float64

	for i, j := 0, 0; i < len(triangles); i, j = i+3, j+2 {
		t1, t2, t3 := triangles[i]*2, triangles[i+1]*2, triangles[i+2]*2
		if t1 < 0 || t2 < 0 || t3 < 0 {
			continue
		}
		x1, y1 := points[t1], points[t1+1]
		x2, y2 := points[t2], points[t2+1]
		x3, y3 := points[t3], points[t3+1]

		dx, dy := x2-x1, y2-y1
		ex, ey := x3-x1, y3-y1
		ab := (float64(dx*ey) - float64(dy*ex)) * 2

		var x, y float64
		if math.Abs(ab) < 1e-9 {
			// A degenerate triangle's circumcenter is at infinity. Placing it
			// very far away, orthogonal to the edge and pointing away from the
			// diagram's center, keeps the clipping arithmetic finite and puts
			// the resulting edge outside the extent where it belongs.
			if !haveBary {
				for _, h := range hull {
					bx += points[h*2]
					by += points[h*2+1]
				}
				if len(hull) > 0 {
					bx /= float64(len(hull))
					by /= float64(len(hull))
				}
				haveBary = true
			}
			a := 1e9 * sign(float64((bx-x1)*ey)-float64((by-y1)*ex))
			x = (x1+x3)/2 - a*ey
			y = (y1+y3)/2 + a*ex
		} else {
			dd := 1 / ab
			bl := dx*dx + dy*dy
			cl := ex*ex + ey*ey
			x = x1 + (float64(ey*bl)-float64(dy*cl))*dd
			y = y1 + (float64(dx*cl)-float64(ex*bl))*dd
		}
		v.circumcenters[j] = x
		v.circumcenters[j+1] = y
	}

	// Exterior rays. For consecutive hull points p0, p1 the cell boundary
	// between them runs off to infinity along the outward normal of that hull
	// edge, which is (y0-y1, x1-x0). Each hull point records the normal of the
	// edge arriving at it and of the edge leaving it.
	if len(hull) > 0 {
		h := hull[len(hull)-1]
		p1 := h * 4
		x1, y1 := points[2*h], points[2*h+1]
		for _, hh := range hull {
			p0, x0, y0 := p1, x1, y1
			h = hh
			p1, x1, y1 = h*4, points[2*h], points[2*h+1]
			v.vectors[p0+2] = y0 - y1
			v.vectors[p1] = y0 - y1
			v.vectors[p0+3] = x1 - x0
			v.vectors[p1+1] = x1 - x0
		}
	}
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

// Contains reports whether the cell of point i contains (x, y).
//
// It is answered by the nearest-point walk rather than by a point-in-polygon
// test on the clipped cell, so it costs no clipping and is exact: a point is in
// cell i exactly when i is its nearest neighbor. Points outside the clipping
// extent are still reported as contained by the cell whose generator is
// nearest.
func (v *Voronoi) Contains(i int, x, y float64) bool {
	if math.IsNaN(x) || math.IsNaN(y) || i < 0 || i >= len(v.d.inedges) {
		return false
	}
	return v.d.step(x, y, i) == i
}

// Neighbors returns the points whose clipped cells share a boundary segment
// with the cell of point i.
//
// This is a subset of [Delaunay.Neighbors]: two points can be Delaunay
// neighbors and yet have their shared Voronoi edge fall entirely outside the
// clipping rectangle, in which case the cells do not touch and the pair is not
// reported here.
func (v *Voronoi) Neighbors(i int) []int {
	ci := v.clip(i)
	if len(ci) == 0 {
		return nil
	}
	var out []int
	for _, j := range v.d.Neighbors(i) {
		cj := v.clip(j)
		if len(cj) == 0 {
			continue
		}
		li, lj := len(ci), len(cj)
		found := false
		for ai := 0; ai < li && !found; ai += 2 {
			for aj := 0; aj < lj; aj += 2 {
				if ci[ai] == cj[aj] && ci[ai+1] == cj[aj+1] &&
					ci[(ai+2)%li] == cj[(aj+lj-2)%lj] &&
					ci[(ai+3)%li] == cj[(aj+lj-1)%lj] {
					out = append(out, j)
					found = true
					break
				}
			}
		}
	}
	return out
}

// CellPolygon returns the closed ring of the clipped cell of point i, or nil if
// the cell is empty (a duplicate point, or a cell entirely outside the extent).
// The last point equals the first.
func (v *Voronoi) CellPolygon(i int) []Point {
	c := v.clip(i)
	if len(c) == 0 {
		return nil
	}
	// Drop the trailing repeats and collinear duplicates the way RenderCell
	// does, so the polygon and the path agree.
	n := len(c)
	for n > 2 && c[0] == c[n-2] && c[1] == c[n-1] {
		n -= 2
	}
	ring := make([]Point, 0, n/2+1)
	ring = append(ring, Point{c[0], c[1]})
	for k := 2; k < n; k += 2 {
		if c[k] != c[k-2] || c[k+1] != c[k-1] {
			ring = append(ring, Point{c[k], c[k+1]})
		}
	}
	return append(ring, ring[0])
}

// Cell pairs a cell polygon with the index of the point that generates it.
type Cell struct {
	Index   int
	Polygon []Point
}

// CellPolygons returns every non-empty cell.
func (v *Voronoi) CellPolygons() []Cell {
	out := make([]Cell, 0, len(v.d.points)/2)
	for i := 0; i < len(v.d.points)/2; i++ {
		if p := v.CellPolygon(i); p != nil {
			out = append(out, Cell{Index: i, Polygon: p})
		}
	}
	return out
}

// Render returns SVG path data for the cell boundaries — the mesh of the
// diagram, with each edge drawn once and clipped to the extent. It is empty
// when there are fewer than two distinct points, since a single cell has no
// boundary of its own.
func (v *Voronoi) Render() string {
	if len(v.d.hull) <= 1 {
		return ""
	}
	p := path.New()
	d := v.d
	for i, n := 0, len(d.halfedges); i < n; i++ {
		j := d.halfedges[i]
		if j < i {
			continue
		}
		ti := (i / 3) * 2
		tj := (j / 3) * 2
		v.renderSegment(p, v.circumcenters[ti], v.circumcenters[ti+1], v.circumcenters[tj], v.circumcenters[tj+1])
	}
	// The unbounded boundaries of the hull cells.
	h1 := d.hull[len(d.hull)-1]
	for _, hh := range d.hull {
		h0 := h1
		h1 = hh
		t := (d.inedges[h1] / 3) * 2
		if t < 0 || t+1 >= len(v.circumcenters) {
			continue
		}
		x, y := v.circumcenters[t], v.circumcenters[t+1]
		vv := h0 * 4
		if px, py, ok := v.project(x, y, v.vectors[vv+2], v.vectors[vv+3]); ok {
			v.renderSegment(p, x, y, px, py)
		}
	}
	return p.String()
}

// RenderBounds returns SVG path data for the clipping rectangle.
func (v *Voronoi) RenderBounds() string {
	return path.New().Rect(v.xmin, v.ymin, v.xmax-v.xmin, v.ymax-v.ymin).String()
}

// RenderCell returns SVG path data for the closed clipped cell of point i, or
// "" if the cell is empty.
func (v *Voronoi) RenderCell(i int) string {
	c := v.clip(i)
	if len(c) == 0 {
		return ""
	}
	p := path.New()
	p.MoveTo(c[0], c[1])
	n := len(c)
	for n > 2 && c[0] == c[n-2] && c[1] == c[n-1] {
		n -= 2
	}
	for k := 2; k < n; k += 2 {
		if c[k] != c[k-2] || c[k+1] != c[k-1] {
			p.LineTo(c[k], c[k+1])
		}
	}
	p.ClosePath()
	return p.String()
}

func (v *Voronoi) renderSegment(p *path.Path, x0, y0, x1, y1 float64) {
	c0 := v.regionCode(x0, y0)
	c1 := v.regionCode(x1, y1)
	if c0 == 0 && c1 == 0 {
		p.MoveTo(x0, y0)
		p.LineTo(x1, y1)
		return
	}
	if s, ok := v.clipSegment(x0, y0, x1, y1, c0, c1); ok {
		p.MoveTo(s[0], s[1])
		p.LineTo(s[2], s[3])
	}
}

// cell returns the unclipped cell of point i as a flat coordinate list: the
// circumcenters of the triangles around i, in order. It is nil for a duplicate
// point, and open (missing its infinite part) for a hull point.
func (v *Voronoi) cell(i int) []float64 {
	d := v.d
	e0 := d.inedges[i]
	if e0 == -1 {
		return nil // coincident point
	}
	var pts []float64
	e := e0
	for {
		t := e / 3
		if 2*t+1 < len(v.circumcenters) {
			pts = append(pts, v.circumcenters[t*2], v.circumcenters[t*2+1])
		}
		if e%3 == 2 {
			e -= 2
		} else {
			e++
		}
		if d.triangles[e] != i {
			break // bad triangulation
		}
		e = d.halfedges[e]
		if e == e0 || e == -1 {
			break
		}
	}
	return pts
}

func (v *Voronoi) clip(i int) []float64 {
	// One distinct point: its cell is the whole extent.
	if i == 0 && len(v.d.hull) == 1 {
		return []float64{v.xmax, v.ymin, v.xmax, v.ymax, v.xmin, v.ymax, v.xmin, v.ymin}
	}
	pts := v.cell(i)
	if pts == nil {
		return nil
	}
	vi := i * 4
	if v.vectors[vi] != 0 || v.vectors[vi+1] != 0 {
		return simplify(v.clipInfinite(i, pts, v.vectors[vi], v.vectors[vi+1], v.vectors[vi+2], v.vectors[vi+3]))
	}
	return simplify(v.clipFinite(i, pts))
}

// clipFinite is Sutherland–Hodgman against the rectangle, with the extra step
// that when the polygon leaves through one side and re-enters through another,
// the intervening corners of the rectangle have to be inserted — that is what
// edge does.
func (v *Voronoi) clipFinite(i int, pts []float64) []float64 {
	n := len(pts)
	var P []float64
	x1, y1 := pts[n-2], pts[n-1]
	c1 := v.regionCode(x1, y1)
	e0, e1 := 0, 0

	for j := 0; j < n; j += 2 {
		x0, y0 := x1, y1
		x1, y1 = pts[j], pts[j+1]
		c0 := c1
		c1 = v.regionCode(x1, y1)
		if c0 == 0 && c1 == 0 {
			e0, e1 = e1, 0
			P = append(P, x1, y1)
			continue
		}
		var sx0, sy0, sx1, sy1 float64
		if c0 == 0 {
			s, ok := v.clipSegment(x0, y0, x1, y1, c0, c1)
			if !ok {
				continue
			}
			sx0, sy0, sx1, sy1 = s[0], s[1], s[2], s[3]
		} else {
			s, ok := v.clipSegment(x1, y1, x0, y0, c1, c0)
			if !ok {
				continue
			}
			sx1, sy1, sx0, sy0 = s[0], s[1], s[2], s[3]
			e0, e1 = e1, v.edgeCode(sx0, sy0)
			if e0 != 0 && e1 != 0 {
				P = v.edge(i, e0, e1, P, len(P))
			}
			P = append(P, sx0, sy0)
		}
		e0, e1 = e1, v.edgeCode(sx1, sy1)
		if e0 != 0 && e1 != 0 {
			P = v.edge(i, e0, e1, P, len(P))
		}
		P = append(P, sx1, sy1)
	}

	if len(P) > 0 {
		e0, e1 = e1, v.edgeCode(P[0], P[1])
		if e0 != 0 && e1 != 0 {
			P = v.edge(i, e0, e1, P, len(P))
		}
		return P
	}
	// The cell missed the rectangle entirely — but if the rectangle's center
	// belongs to this cell, the cell in fact covers the whole rectangle.
	if v.Contains(i, (v.xmin+v.xmax)/2, (v.ymin+v.ymax)/2) {
		return []float64{v.xmax, v.ymin, v.xmax, v.ymax, v.xmin, v.ymax, v.xmin, v.ymin}
	}
	return nil
}

// clipSegment is Cohen–Sutherland line clipping. The returned array is
// [x0, y0, x1, y1]; ok is false when the segment misses the rectangle.
func (v *Voronoi) clipSegment(x0, y0, x1, y1 float64, c0, c1 int) ([4]float64, bool) {
	// Always consider the segment in the same order, so that clipping is
	// symmetric and adjacent cells agree on their shared vertex exactly.
	flip := c0 < c1
	if flip {
		x0, y0, x1, y1, c0, c1 = x1, y1, x0, y0, c1, c0
	}
	for {
		if c0 == 0 && c1 == 0 {
			if flip {
				return [4]float64{x1, y1, x0, y0}, true
			}
			return [4]float64{x0, y0, x1, y1}, true
		}
		if c0&c1 != 0 {
			return [4]float64{}, false
		}
		c := c0
		if c == 0 {
			c = c1
		}
		var x, y float64
		switch {
		case c&edgeBottom != 0:
			x, y = x0+(x1-x0)*(v.ymax-y0)/(y1-y0), v.ymax
		case c&edgeTop != 0:
			x, y = x0+(x1-x0)*(v.ymin-y0)/(y1-y0), v.ymin
		case c&edgeRight != 0:
			x, y = v.xmax, y0+(y1-y0)*(v.xmax-x0)/(x1-x0)
		default:
			x, y = v.xmin, y0+(y1-y0)*(v.xmin-x0)/(x1-x0)
		}
		if c0 != 0 {
			x0, y0 = x, y
			c0 = v.regionCode(x0, y0)
		} else {
			x1, y1 = x, y
			c1 = v.regionCode(x1, y1)
		}
	}
}

// clipInfinite closes an unbounded cell by projecting its two open ends onto
// the rectangle, clipping the result, and then filling in whichever corners of
// the rectangle the cell wraps around.
func (v *Voronoi) clipInfinite(i int, pts []float64, vx0, vy0, vxn, vyn float64) []float64 {
	P := append([]float64(nil), pts...)
	if px, py, ok := v.project(P[0], P[1], vx0, vy0); ok {
		P = append([]float64{px, py}, P...)
	}
	if px, py, ok := v.project(P[len(P)-2], P[len(P)-1], vxn, vyn); ok {
		P = append(P, px, py)
	}
	P = v.clipFinite(i, P)
	if len(P) > 0 {
		n := len(P)
		c1 := v.edgeCode(P[n-2], P[n-1])
		for j := 0; j < n; j += 2 {
			c0 := c1
			c1 = v.edgeCode(P[j], P[j+1])
			if c0 != 0 && c1 != 0 {
				P = v.edge(i, c0, c1, P, j)
				j = v.lastEdgeJ
				n = len(P)
			}
		}
		return P
	}
	if v.Contains(i, (v.xmin+v.xmax)/2, (v.ymin+v.ymax)/2) {
		return []float64{v.xmin, v.ymin, v.xmax, v.ymin, v.xmax, v.ymax, v.xmin, v.ymax}
	}
	return nil
}

// edge inserts the corners of the clipping rectangle lying between side e0 and
// side e1 into P at position j, and records the resulting position in
// [Voronoi.lastEdgeJ].
func (v *Voronoi) edge(i, e0, e1 int, P []float64, j int) []float64 {
	// Walk the rectangle's boundary from side e0 to side e1, inserting each
	// corner passed on the way — but only the corners that actually belong to
	// this cell.
	for e0 != e1 {
		var x, y float64
		switch e0 {
		case 0b0101: // top-left
			e0 = 0b0100
			continue
		case 0b0100: // top
			e0, x, y = 0b0110, v.xmax, v.ymin
		case 0b0110: // top-right
			e0 = 0b0010
			continue
		case 0b0010: // right
			e0, x, y = 0b1010, v.xmax, v.ymax
		case 0b1010: // bottom-right
			e0 = 0b1000
			continue
		case 0b1000: // bottom
			e0, x, y = 0b1001, v.xmin, v.ymax
		case 0b1001: // bottom-left
			e0 = 0b0001
			continue
		case 0b0001: // left
			e0, x, y = 0b0101, v.xmin, v.ymin
		default:
			// Not a side code; nothing sensible to walk.
			v.lastEdgeJ = j
			return P
		}
		if (j >= len(P) || P[j] != x || P[j+1] != y) && v.Contains(i, x, y) {
			P = append(P, 0, 0)
			copy(P[j+2:], P[j:])
			P[j], P[j+1] = x, y
			j += 2
		}
	}
	v.lastEdgeJ = j
	return P
}

// project returns where the ray from (x0, y0) in direction (vx, vy) leaves the
// rectangle. ok is false when the ray points away from the rectangle or has no
// direction at all.
func (v *Voronoi) project(x0, y0, vx, vy float64) (float64, float64, bool) {
	t := math.Inf(1)
	var x, y float64
	hit := false
	switch {
	case vy < 0:
		if y0 <= v.ymin {
			return 0, 0, false
		}
		if c := (v.ymin - y0) / vy; c < t {
			t = c
			y, x = v.ymin, x0+t*vx
			hit = true
		}
	case vy > 0:
		if y0 >= v.ymax {
			return 0, 0, false
		}
		if c := (v.ymax - y0) / vy; c < t {
			t = c
			y, x = v.ymax, x0+t*vx
			hit = true
		}
	}
	switch {
	case vx > 0:
		if x0 >= v.xmax {
			return 0, 0, false
		}
		if c := (v.xmax - x0) / vx; c < t {
			t = c
			x, y = v.xmax, y0+t*vy
			hit = true
		}
	case vx < 0:
		if x0 <= v.xmin {
			return 0, 0, false
		}
		if c := (v.xmin - x0) / vx; c < t {
			t = c
			x, y = v.xmin, y0+t*vy
			hit = true
		}
	}
	return x, y, hit
}

// edgeCode reports which side(s) of the rectangle a point lies exactly on.
func (v *Voronoi) edgeCode(x, y float64) int {
	c := 0
	switch {
	case x == v.xmin:
		c |= edgeLeft
	case x == v.xmax:
		c |= edgeRight
	}
	switch {
	case y == v.ymin:
		c |= edgeTop
	case y == v.ymax:
		c |= edgeBottom
	}
	return c
}

// regionCode is the Cohen–Sutherland outcode: which side(s) a point lies beyond.
func (v *Voronoi) regionCode(x, y float64) int {
	c := 0
	switch {
	case x < v.xmin:
		c |= edgeLeft
	case x > v.xmax:
		c |= edgeRight
	}
	switch {
	case y < v.ymin:
		c |= edgeTop
	case y > v.ymax:
		c |= edgeBottom
	}
	return c
}

// simplify drops a vertex that lies between two others on the same horizontal
// or vertical line. Clipping produces plenty of those along the rectangle's
// sides, and they make otherwise-identical adjacent cell boundaries compare
// unequal.
func simplify(P []float64) []float64 {
	if len(P) > 4 {
		for i := 0; i < len(P); i += 2 {
			j := (i + 2) % len(P)
			k := (i + 4) % len(P)
			if (P[i] == P[j] && P[j] == P[k]) || (P[i+1] == P[j+1] && P[j+1] == P[k+1]) {
				P = append(P[:j], P[j+2:]...)
				i -= 2
			}
		}
		if len(P) == 0 {
			return nil
		}
	}
	return P
}
