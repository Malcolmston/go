// Package delaunay is a Go port of d3-delaunay, and with it of the Delaunator
// triangulation kernel that d3-delaunay wraps.
//
// A [Delaunay] is the Delaunay triangulation of a set of points: the unique
// triangulation (for points in general position) in which no point lies inside
// the circumcircle of any triangle. That property is not a detail — it is the
// definition, it is what makes the triangulation the "right" one for
// interpolation and nearest-neighbor work, and it is what this package's tests
// verify over random inputs rather than against recorded output.
//
// Its dual is the [Voronoi] diagram: one convex cell per point, containing
// exactly the plane that is nearer to that point than to any other. Because a
// Voronoi cell can be unbounded, a diagram must be clipped to a rectangle,
// which is why [Delaunay.Voronoi] takes an extent.
//
//	d := delaunay.New([]delaunay.Point{{0, 0}, {100, 0}, {50, 80}, {20, 60}})
//	v := d.Voronoi(0, 0, 200, 200)
//	react.H("path", react.Props{"d": v.Render(), "stroke": "#ccc"})
//
// # Rendering
//
// Every Render method returns a string of SVG path data built with
// [github.com/malcolmston/d3/path], so the integration with a view layer is a
// single attribute, exactly as it is for d3/shape. The Cell and Polygon methods
// return the same geometry as coordinates instead, for callers that want to
// measure or hit-test rather than draw.
//
// # Degenerate input
//
// The interesting cases are the ones with no triangulation at all, and they are
// handled explicitly rather than left to produce garbage:
//
//   - Zero points: everything is empty; no method panics.
//   - One point, or several coincident points: there are no triangles. The hull
//     is the single distinct point, and its Voronoi cell is the whole extent.
//   - Two distinct points: no triangles, a two-point hull, and two half-plane
//     cells meeting on the perpendicular bisector.
//   - Three or more collinear points: no triangulation exists, so following d3
//     the points are perturbed by a jitter proportional to their extent and
//     re-triangulated. [Delaunay.Points] then reports the perturbed positions —
//     see [Delaunay.Collinear]. Neighbor queries bypass the mesh entirely and
//     use the sorted order along the line, which is exact.
//   - Duplicate points inside a larger set: the duplicate is dropped from the
//     mesh. Its Neighbors are empty and its Voronoi cell is nil.
//
// # Deviations from d3-delaunay
//
// Configuration follows the port-wide convention (see API-DEVIATIONS.md):
// accessors take (datum, index, data), generators are invoked by name rather
// than by call, and getters that return slices return defensive copies.
// [Delaunay.Neighbors], [Voronoi.Neighbors] and [Voronoi.CellPolygons] return
// slices where d3 returns generators. The point order fed to the triangulation
// is sorted with an index tiebreak, making the output a deterministic function
// of the input; d3 inherits Delaunator's unstable quicksort, so its half-edge
// numbering can differ between runs on tied inputs. Both satisfy the Delaunay
// condition, which is the only thing that is actually specified.
package delaunay

import (
	"math"
	"sort"

	"github.com/malcolmston/d3/path"
)

// Point is a position in the plane.
type Point struct{ X, Y float64 }

// Accessor reads one coordinate out of a datum, with the (datum, index, data)
// signature used throughout this port.
type Accessor[T any] func(d T, i int, data []T) float64

// Delaunay is the Delaunay triangulation of a point set, plus the indices that
// make neighbor and nearest-point queries fast.
//
// The mesh is stored the way Delaunator stores it, as a flat array of
// half-edges. Half-edge e belongs to triangle e/3 and runs from point
// Triangles()[e] to point Triangles()[nextHalfedge(e)]; Halfedges()[e] is the
// opposite half-edge, or -1 when e is on the convex hull. Every traversal in
// this package — cells, neighbors, the nearest-point walk — is a loop over that
// structure, so it is worth understanding rather than hiding.
type Delaunay struct {
	points []float64 // interleaved, owned by this Delaunay

	d         *delaunator
	triangles []int
	halfedges []int
	hull      []int
	inedges   []int
	hullIndex []int

	// collinear, when non-nil, holds the point indices sorted along the line
	// they lie on. Its presence is the signal that the coordinates were
	// jittered and that neighbor queries must not trust the mesh.
	collinear []int
}

// New returns the Delaunay triangulation of points.
//
// The points are copied, so the caller's slice is never modified — note that d3
// jitters its input array in place for collinear input, and this does not.
func New(points []Point) *Delaunay {
	coords := make([]float64, len(points)*2)
	for i, p := range points {
		coords[2*i] = p.X
		coords[2*i+1] = p.Y
	}
	return NewFromCoords(coords)
}

// NewFromCoords returns the triangulation of an interleaved [x0, y0, x1, y1, …]
// coordinate slice, which is copied. An odd-length slice loses its last value.
func NewFromCoords(coords []float64) *Delaunay {
	n := len(coords) / 2
	d := &Delaunay{points: append([]float64(nil), coords[:2*n]...)}
	d.d = newDelaunator(d.points)
	d.inedges = make([]int, n)
	d.hullIndex = make([]int, n)
	d.init()
	return d
}

// From returns the triangulation of arbitrary data through coordinate
// accessors, the analogue of d3.Delaunay.from.
func From[T any](data []T, fx, fy Accessor[T]) *Delaunay {
	coords := make([]float64, len(data)*2)
	for i, v := range data {
		coords[2*i] = fx(v, i, data)
		coords[2*i+1] = fy(v, i, data)
	}
	return NewFromCoords(coords)
}

func (d *Delaunay) init() {
	// Collinear points have no triangulation. d3's answer is to perturb them by
	// an amount proportional to their extent and triangulate the perturbation,
	// which yields a mesh that is arbitrary but well-formed; the exact neighbor
	// relation is recovered separately from the sorted order along the line.
	if len(d.d.hull) > 2 && isCollinear(d.d) {
		n := len(d.points) / 2
		d.collinear = make([]int, n)
		for i := range d.collinear {
			d.collinear[i] = i
		}
		pts := d.points
		sort.SliceStable(d.collinear, func(a, b int) bool {
			ia, ib := d.collinear[a], d.collinear[b]
			if pts[2*ia] != pts[2*ib] {
				return pts[2*ia] < pts[2*ib]
			}
			return pts[2*ia+1] < pts[2*ib+1]
		})
		e := d.collinear[0]
		f := d.collinear[len(d.collinear)-1]
		r := 1e-8 * math.Hypot(pts[2*f+1]-pts[2*e+1], pts[2*f]-pts[2*e])
		for i := 0; i < n; i++ {
			x, y := pts[2*i], pts[2*i+1]
			pts[2*i] = x + math.Sin(x+y)*r
			pts[2*i+1] = y + math.Cos(x-y)*r
		}
		d.d = newDelaunator(pts)
	} else {
		d.collinear = nil
	}

	d.triangles = d.d.triangles
	d.halfedges = d.d.halfedges
	d.hull = d.d.hull

	for i := range d.inedges {
		d.inedges[i] = -1
	}
	for i := range d.hullIndex {
		d.hullIndex[i] = -1
	}

	// One incoming half-edge per point, which is where every traversal starts.
	// Hull points must get an *exterior* half-edge, otherwise a cell or
	// neighbor walk would start in the middle of the fan and stop early.
	for e, n := 0, len(d.halfedges); e < n; e++ {
		var p int
		if e%3 == 2 {
			p = d.triangles[e-2]
		} else {
			p = d.triangles[e+1]
		}
		if d.halfedges[e] == -1 || d.inedges[p] == -1 {
			d.inedges[p] = e
		}
	}
	for i, h := range d.hull {
		d.hullIndex[h] = i
	}

	// One or two distinct points: fabricate a degenerate triangle so that the
	// traversals below have something to walk, exactly as d3 does. Its
	// half-edges are all -1, so every walk terminates immediately.
	if len(d.hull) > 0 && len(d.hull) <= 2 {
		d.triangles = []int{-1, -1, -1}
		d.halfedges = []int{-1, -1, -1}
		d.triangles[0] = d.hull[0]
		d.inedges[d.hull[0]] = 1
		if len(d.hull) == 2 {
			d.inedges[d.hull[1]] = 0
			d.triangles[1] = d.hull[1]
			d.triangles[2] = d.hull[1]
		}
	}
}

// isCollinear reports whether every triangle in the mesh is degenerate, which
// is how d3 detects collinear input after the fact. The threshold is absolute,
// not relative, so point sets confined to a very small region are treated as
// collinear; that is d3's behavior and this port keeps it rather than silently
// diverging.
func isCollinear(d *delaunator) bool {
	tri, coords := d.triangles, d.coords
	for i := 0; i < len(tri); i += 3 {
		a := 2 * tri[i]
		b := 2 * tri[i+1]
		c := 2 * tri[i+2]
		cross := float64((coords[c]-coords[a])*(coords[b+1]-coords[a+1])) -
			float64((coords[b]-coords[a])*(coords[c+1]-coords[a+1]))
		if cross > 1e-10 {
			return false
		}
	}
	return true
}

// Points returns the triangulated positions.
//
// These are the input positions, except for collinear input, where they are the
// jittered positions the mesh was actually built from — see [Delaunay.Collinear].
func (d *Delaunay) Points() []Point {
	out := make([]Point, len(d.points)/2)
	for i := range out {
		out[i] = Point{d.points[2*i], d.points[2*i+1]}
	}
	return out
}

// Coords returns the interleaved coordinates, a copy.
func (d *Delaunay) Coords() []float64 { return append([]float64(nil), d.points...) }

// Triangles returns the vertex index of every half-edge: three consecutive
// entries are one triangle. A copy.
func (d *Delaunay) Triangles() []int { return append([]int(nil), d.triangles...) }

// Halfedges returns the opposite half-edge of every half-edge, or -1 where the
// half-edge lies on the convex hull. It is its own inverse:
// Halfedges()[Halfedges()[e]] == e wherever the entry is not -1. A copy.
func (d *Delaunay) Halfedges() []int { return append([]int(nil), d.halfedges...) }

// Hull returns the point indices on the convex hull, counterclockwise. A copy.
func (d *Delaunay) Hull() []int { return append([]int(nil), d.hull...) }

// Inedges returns, for each point, the index of an incoming half-edge, or -1
// for a point that was dropped as a duplicate. For hull points the half-edge is
// an exterior one. A copy.
func (d *Delaunay) Inedges() []int { return append([]int(nil), d.inedges...) }

// Collinear reports whether the input was collinear (or too small in extent to
// tell), in which case the coordinates were jittered before triangulating and
// [Delaunay.Points] differs from the input.
func (d *Delaunay) Collinear() bool { return d.collinear != nil }

// Neighbors returns the indices of the points sharing a triangulation edge with
// point i, in no particular order. It is empty for a duplicate point, which
// belongs to no triangle.
//
// For collinear input the mesh is a jittered fiction, so this answers from the
// sorted order along the line instead: at most one neighbor on each side.
func (d *Delaunay) Neighbors(i int) []int {
	var out []int
	if i < 0 || i >= len(d.inedges) {
		return nil
	}

	if d.collinear != nil {
		l := -1
		for k, v := range d.collinear {
			if v == i {
				l = k
				break
			}
		}
		if l < 0 {
			return nil
		}
		if l > 0 {
			out = append(out, d.collinear[l-1])
		}
		if l < len(d.collinear)-1 {
			out = append(out, d.collinear[l+1])
		}
		return out
	}

	e0 := d.inedges[i]
	if e0 == -1 { // coincident point
		return nil
	}
	e := e0
	p0 := -1
	for {
		p0 = d.triangles[e]
		// The fabricated triangle of a one-point mesh has -1 vertices; d3 emits
		// them, which is a wart rather than a feature.
		if p0 >= 0 {
			out = append(out, p0)
		}
		if e%3 == 2 {
			e -= 2
		} else {
			e++
		}
		if d.triangles[e] != i {
			return out // bad triangulation
		}
		e = d.halfedges[e]
		if e == -1 {
			// Walked off the hull: the last neighbor is the next hull point,
			// whose edge to i has no interior twin to arrive by.
			p := d.hull[(d.hullIndex[i]+1)%len(d.hull)]
			if p != p0 {
				out = append(out, p)
			}
			return out
		}
		if e == e0 {
			return out
		}
	}
}

// Find returns the index of the input point closest to (x, y), or -1 if there
// are no points or the query is not finite.
func (d *Delaunay) Find(x, y float64) int { return d.FindFrom(x, y, 0) }

// FindFrom is [Delaunay.Find] starting the search at point i.
//
// The search is a greedy walk: from the current point, step to whichever
// neighbor is closer to the query, and stop when no neighbor improves. On a
// Delaunay triangulation that walk is guaranteed to reach the true nearest
// point, and passing the previous result as i makes a sequence of nearby
// queries (a mouse moving over a chart) cost a few steps instead of a full
// search.
func (d *Delaunay) FindFrom(x, y float64, i int) int {
	if math.IsNaN(x) || math.IsNaN(y) || len(d.points) == 0 {
		return -1
	}
	if i < 0 || i >= len(d.inedges) {
		i = 0
	}
	i0 := i
	for {
		c := d.step(x, y, i)
		if c < 0 || c == i || c == i0 {
			return c
		}
		i = c
	}
}

// step returns the neighbor of i (or i itself) closest to (x, y).
func (d *Delaunay) step(x, y float64, i int) int {
	n := len(d.points) >> 1
	if n == 0 {
		return -1
	}
	if d.inedges[i] == -1 {
		return (i + 1) % n
	}
	c := i
	dc := dist2(x, y, d.points[i*2], d.points[i*2+1])
	e0 := d.inedges[i]
	e := e0
	for {
		t := d.triangles[e]
		if t >= 0 {
			if dt := dist2(x, y, d.points[t*2], d.points[t*2+1]); dt < dc {
				dc, c = dt, t
			}
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
		if e == -1 {
			h := d.hull[(d.hullIndex[i]+1)%len(d.hull)]
			if h != t {
				if dist2(x, y, d.points[h*2], d.points[h*2+1]) < dc {
					return h
				}
			}
			break
		}
		if e == e0 {
			break
		}
	}
	return c
}

// Voronoi returns the Voronoi diagram dual to this triangulation, clipped to
// the given rectangle.
//
// A Voronoi cell on the hull is unbounded, so a clipping rectangle is not
// optional — without one, half the diagram has no finite geometry. The extent
// must satisfy xmax >= xmin and ymax >= ymin and be finite; anything else is a
// programming error and panics, since a diagram clipped to a nonsensical box
// has no meaningful output to return instead.
func (d *Delaunay) Voronoi(xmin, ymin, xmax, ymax float64) *Voronoi {
	return newVoronoi(d, xmin, ymin, xmax, ymax)
}

// TrianglePolygon returns the closed ring of triangle i: four points, the last
// equal to the first.
func (d *Delaunay) TrianglePolygon(i int) []Point {
	if i < 0 || (i+1)*3 > len(d.triangles) {
		return nil
	}
	ring := make([]Point, 0, 4)
	for k := 0; k < 3; k++ {
		t := d.triangles[i*3+k]
		if t < 0 {
			return nil
		}
		ring = append(ring, Point{d.points[t*2], d.points[t*2+1]})
	}
	return append(ring, ring[0])
}

// TrianglePolygons returns the closed ring of every triangle.
func (d *Delaunay) TrianglePolygons() [][]Point {
	out := make([][]Point, 0, len(d.triangles)/3)
	for i := 0; i < len(d.triangles)/3; i++ {
		if p := d.TrianglePolygon(i); p != nil {
			out = append(out, p)
		}
	}
	return out
}

// HullPolygon returns the closed ring of the convex hull, or nil when there is
// no hull.
func (d *Delaunay) HullPolygon() []Point {
	if len(d.hull) == 0 {
		return nil
	}
	ring := make([]Point, 0, len(d.hull)+1)
	for _, h := range d.hull {
		ring = append(ring, Point{d.points[h*2], d.points[h*2+1]})
	}
	return append(ring, ring[0])
}

// Render returns SVG path data for every edge of the triangulation, hull
// included. Each edge is emitted once.
func (d *Delaunay) Render() string {
	p := path.New()
	d.render(p)
	return p.String()
}

func (d *Delaunay) render(p *path.Path) {
	for i, n := 0, len(d.halfedges); i < n; i++ {
		j := d.halfedges[i]
		// Emitting only the half-edge with the smaller id draws each interior
		// edge once; hull half-edges (j == -1) are skipped here and drawn by
		// renderHull.
		if j < i {
			continue
		}
		ti := d.triangles[i] * 2
		tj := d.triangles[j] * 2
		p.MoveTo(d.points[ti], d.points[ti+1])
		p.LineTo(d.points[tj], d.points[tj+1])
	}
	d.renderHull(p)
}

// RenderTriangle returns SVG path data for the closed outline of triangle i.
func (d *Delaunay) RenderTriangle(i int) string {
	if i < 0 || (i+1)*3 > len(d.triangles) {
		return ""
	}
	t0, t1, t2 := d.triangles[i*3]*2, d.triangles[i*3+1]*2, d.triangles[i*3+2]*2
	if t0 < 0 || t1 < 0 || t2 < 0 {
		return ""
	}
	p := path.New()
	p.MoveTo(d.points[t0], d.points[t0+1])
	p.LineTo(d.points[t1], d.points[t1+1])
	p.LineTo(d.points[t2], d.points[t2+1])
	p.ClosePath()
	return p.String()
}

// RenderHull returns SVG path data for the closed convex hull.
func (d *Delaunay) RenderHull() string {
	p := path.New()
	d.renderHull(p)
	return p.String()
}

func (d *Delaunay) renderHull(p *path.Path) {
	if len(d.hull) == 0 {
		return
	}
	h := d.hull[0] * 2
	p.MoveTo(d.points[h], d.points[h+1])
	for _, i := range d.hull[1:] {
		p.LineTo(d.points[i*2], d.points[i*2+1])
	}
	p.ClosePath()
}

// RenderPoints returns SVG path data drawing a circle of radius r at every
// point — the scatterplot layer that usually sits on top of a mesh.
func (d *Delaunay) RenderPoints(r float64) string {
	if r <= 0 || math.IsNaN(r) {
		r = 2
	}
	p := path.New()
	for i, n := 0, len(d.points); i < n; i += 2 {
		x, y := d.points[i], d.points[i+1]
		p.MoveTo(x+r, y)
		p.Arc(x, y, r, 0, 2*math.Pi, false)
	}
	return p.String()
}
