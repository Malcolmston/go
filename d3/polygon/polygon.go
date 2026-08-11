// Package polygon is a Go port of d3-polygon: the small set of geometric
// primitives that operate on a closed ring of two-dimensional points — signed
// area, centroid, convex hull, point containment and perimeter length.
//
// It is deliberately tiny and has no configuration, no generators and no
// output format. It computes numbers about a ring of points, and something
// else draws them. Everything here is exact arithmetic on float64s, so the
// results are testable by value rather than by eyeball.
//
//	square := polygon.Polygon{{0, 0}, {0, 1}, {1, 1}, {1, 0}}
//	square.Area()      // 1
//	square.Centroid()  // {0.5, 0.5}
//	square.Length()    // 4
//	square.Contains(polygon.Point{0.5, 0.5}) // true
//
//	hull := polygon.Hull([]polygon.Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0.5, 0.5}})
//	// the four corners; the interior point is dropped
//
// # The ring is implicitly closed
//
// A [Polygon] is the list of its vertices, each appearing once. The closing
// edge from the last vertex back to the first is implied and must not be
// repeated — a duplicated final point adds a zero-length edge, which changes
// [Polygon.Length] and can change [Polygon.Contains] on the boundary. This
// matches d3.
//
// # Winding order and the sign of the area
//
// [Polygon.Area] is a *signed* area, and the sign is the part everybody gets
// wrong, so it is worth stating in one sentence and then in detail:
//
//	The area is positive when the vertices wind counter-clockwise on screen —
//	that is, in a coordinate system whose origin is the top-left corner and
//	whose y-axis points down, which is SVG's and canvas's.
//
// This is upstream's documented behavior and this port reproduces it exactly.
// The reason it surprises people is that the usual textbook shoelace formula
// is positive for counter-clockwise winding in *mathematical* coordinates,
// where y points up. Flipping the y-axis mirrors the picture, which reverses
// apparent handedness, so the two conventions disagree by a sign on the same
// list of numbers. d3 resolves the ambiguity in favour of the screen, because
// that is where the polygon is going to be drawn.
//
// Concretely, with y pointing down:
//
//	polygon.Polygon{{0, 0}, {0, 1}, {1, 1}, {1, 0}}.Area()  // +1, counter-clockwise on screen
//	polygon.Polygon{{0, 0}, {1, 0}, {1, 1}, {0, 1}}.Area()  // -1, clockwise on screen
//
// Take [math.Abs] of it when you want a magnitude; read its sign when you want
// the winding order (which is what a hull consumer or a fill-rule usually
// wants).
//
// [Polygon.Centroid] is computed from the same signed quantities, so its
// result does not depend on winding order. [Hull] returns its ring in
// counter-clockwise screen order, and therefore has a positive area.
//
// # Names
//
// Upstream spells these as free functions with a prefix — d3.polygonArea,
// d3.polygonCentroid, d3.polygonHull, d3.polygonContains, d3.polygonLength.
// Here the prefix is the package name and the ring is a type, so they are
// methods on [Polygon]; [Hull] stays a function because its input is a cloud
// of points rather than a ring.
package polygon

import (
	"cmp"
	"math"
	"slices"
)

// Point is a vertex. The field names are X and Y rather than the [2]float64
// upstream uses, because a named field is harder to transpose by accident and
// composite literals stay just as short: polygon.Point{3, 4}.
type Point struct {
	X, Y float64
}

// Polygon is a closed ring of vertices in order. The edge from the last vertex
// back to the first is implied; do not repeat the first point at the end.
//
// Fewer than three vertices is a degenerate polygon rather than an error: the
// area and the enclosed centroid of a point or a segment are not meaningful,
// but Length is, and every method returns without panicking.
type Polygon []Point

// Area returns the signed area of the ring.
//
// The sign encodes winding order: positive for counter-clockwise on screen
// (origin top-left, y pointing down), negative for clockwise. See the package
// documentation for why that is the opposite of the textbook shoelace
// convention. A polygon of fewer than three vertices has area 0.
func (p Polygon) Area() float64 {
	n := len(p)
	if n < 3 {
		return 0
	}
	// Note the operand order: a.Y*b.X - a.X*b.Y is the negation of the usual
	// shoelace term, which is exactly what puts the sign in screen space.
	area := 0.0
	b := p[n-1]
	for i := 0; i < n; i++ {
		a := b
		b = p[i]
		area += a.Y*b.X - a.X*b.Y
	}
	return area / 2
}

// Centroid returns the centroid of the ring's *enclosed area* — the balance
// point of a uniform lamina with this outline, not the mean of the vertices.
// The two differ for any polygon whose vertices are unevenly spaced, and the
// area centroid is the one a label wants.
//
// The result does not depend on winding order, because the signed quantities
// it divides cancel the sign. For a degenerate polygon — fewer than three
// vertices, or three or more that are collinear — the enclosed area is zero
// and the coordinates are NaN or infinite, matching upstream. Test the area
// first if that matters.
func (p Polygon) Centroid() Point {
	n := len(p)
	if n == 0 {
		return Point{X: math.NaN(), Y: math.NaN()}
	}
	var x, y, k float64
	b := p[n-1]
	for i := 0; i < n; i++ {
		a := b
		b = p[i]
		c := a.X*b.Y - b.X*a.Y
		k += c
		x += (a.X + b.X) * c
		y += (a.Y + b.Y) * c
	}
	k *= 3
	return Point{X: x / k, Y: y / k}
}

// Length returns the perimeter: the total length of every edge, including the
// implied closing edge from the last vertex back to the first.
//
// A two-point "polygon" therefore has the length of its segment counted twice,
// out and back, which is upstream's behavior and follows from the ring being
// closed.
func (p Polygon) Length() float64 {
	n := len(p)
	if n < 2 {
		return 0
	}
	perimeter := 0.0
	b := p[n-1]
	for i := 0; i < n; i++ {
		a := b
		b = p[i]
		perimeter += math.Hypot(a.X-b.X, a.Y-b.Y)
	}
	return perimeter
}

// Contains reports whether the point lies inside the ring, by the even-odd
// (crossing-number) rule: a ray is cast from the point and the number of edges
// it crosses is counted, odd meaning inside.
//
// Behavior on the boundary itself is not defined and is not stable under
// floating-point perturbation — a vertex or an edge can land on either side.
// This is true of upstream too, and of every implementation of this test; if a
// boundary decision is load-bearing, test distance to the edges instead. The
// rule does mean that for a self-intersecting ring the "inside" is the
// even-odd interior, which is also what SVG's default fill-rule paints.
func (p Polygon) Contains(q Point) bool {
	n := len(p)
	if n < 3 {
		return false
	}
	inside := false
	prev := p[n-1]
	for i := 0; i < n; i++ {
		cur := p[i]
		if ((cur.Y > q.Y) != (prev.Y > q.Y)) &&
			q.X < (prev.X-cur.X)*(q.Y-cur.Y)/(prev.Y-cur.Y)+cur.X {
			inside = !inside
		}
		prev = cur
	}
	return inside
}

// Hull returns the convex hull of a cloud of points, computed with Andrew's
// monotone chain algorithm: sort lexicographically by x then y, sweep once to
// build the upper chain and once over the y-flipped points to build the lower,
// and join them.
//
// The returned ring contains a subset of the input points in counter-clockwise
// screen order, so its [Polygon.Area] is positive. Interior points and points
// lying on a hull edge but not at a vertex are dropped. Duplicate points are
// permitted.
//
// nil is returned when there are fewer than three input points, matching
// upstream — two points have no hull with an interior, and returning a
// degenerate two-point ring would silently produce an area of zero downstream.
// Three or more collinear points do produce a ring, which is also upstream's
// behavior; it will have zero area.
//
// The input slice is not modified.
func Hull(points []Point) Polygon {
	n := len(points)
	if n < 3 {
		return nil
	}

	// Sort indices rather than points, so the returned ring can hand back the
	// caller's own Point values in their original identity and order-of-input
	// is recoverable.
	sorted := make([]int, n)
	for i := range sorted {
		sorted[i] = i
	}
	slices.SortStableFunc(sorted, func(a, b int) int {
		pa, pb := points[a], points[b]
		if c := cmp.Compare(pa.X, pb.X); c != 0 {
			return c
		}
		return cmp.Compare(pa.Y, pb.Y)
	})

	upright := make([]Point, n)
	flipped := make([]Point, n)
	for i, s := range sorted {
		upright[i] = points[s]
		flipped[i] = Point{X: points[s].X, Y: -points[s].Y}
	}

	upper := upperHullIndexes(upright)
	lower := upperHullIndexes(flipped)

	// The two chains share their extreme points; drop the duplicates.
	skipLeft := 0
	if lower[0] == upper[0] {
		skipLeft = 1
	}
	skipRight := 0
	if lower[len(lower)-1] == upper[len(upper)-1] {
		skipRight = 1
	}

	hull := make(Polygon, 0, len(upper)+len(lower))
	for i := len(upper) - 1; i >= 0; i-- {
		hull = append(hull, points[sorted[upper[i]]])
	}
	for i := skipLeft; i < len(lower)-skipRight; i++ {
		hull = append(hull, points[sorted[lower[i]]])
	}
	return hull
}

// upperHullIndexes computes the upper chain of the monotone-chain algorithm
// over points already sorted by x then y, returning indexes into that sorted
// slice in left-to-right order. Running it again on the y-flipped points gives
// the lower chain, which is why there is only one of these functions.
func upperHullIndexes(points []Point) []int {
	n := len(points)
	indexes := make([]int, n)
	indexes[0], indexes[1] = 0, 1
	size := 2
	for i := 2; i < n; i++ {
		// Pop while the last turn is not a strict right turn: a collinear
		// point (cross == 0) is popped too, which is what keeps points lying
		// on an edge out of the result.
		for size > 1 && cross(points[indexes[size-2]], points[indexes[size-1]], points[i]) <= 0 {
			size--
		}
		indexes[size] = i
		size++
	}
	return indexes[:size]
}

// cross returns the z-component of (a-o) × (b-o): positive when o→a→b turns
// one way, negative the other, zero when the three are collinear.
func cross(o, a, b Point) float64 {
	return (a.X-o.X)*(b.Y-o.Y) - (a.Y-o.Y)*(b.X-o.X)
}
