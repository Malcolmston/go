package delaunay

import (
	"math"
	"sort"
)

// epsilonDup is the coordinate tolerance below which two consecutive points in
// the insertion order are considered the same point and the second is skipped.
// It is 2^-52, Delaunator's value.
const epsilonDup = 2.220446049250313e-16

// edgeStackCap bounds the flip stack. Delaunator uses a fixed 512-entry stack
// and simply stops pushing when it is full, on the grounds that overflowing it
// requires input degenerate enough that the mesh is already meaningless. This
// port grows the stack instead — the allocation is bounded by the number of
// edges and the correctness question does not arise.
const edgeStackCap = 512

// delaunator is the triangulation kernel: a port of Mapbox's Delaunator, the
// incremental sweep-hull algorithm that d3-delaunay wraps.
//
// The shape of the algorithm: sort the points by distance from the circumcenter
// of a small seed triangle, then insert them one at a time, each time attaching
// a fan of triangles to the part of the current convex hull that the new point
// can see, and restoring the Delaunay condition by flipping edges outward from
// the fan. Because the points arrive roughly in order of distance from a fixed
// center, the visible portion of the hull is found in near-constant time via a
// hash of the pseudo-angle around that center.
type delaunator struct {
	coords []float64 // interleaved x, y

	triangles []int // vertex index per half-edge, three per triangle
	halfedges []int // opposite half-edge id, or -1 on the hull
	hull      []int // point indices, counterclockwise

	trianglesLen int

	hullPrev  []int
	hullNext  []int
	hullTri   []int
	hullHash  []int
	hashSize  int
	hullStart int

	ids   []int
	dists []float64

	cx, cy float64

	edgeStack []int
}

// newDelaunator triangulates the interleaved coordinate slice, which it does
// not copy — the caller owns it and must not mutate it afterwards.
func newDelaunator(coords []float64) *delaunator {
	d := &delaunator{coords: coords}
	d.update()
	return d
}

func (d *delaunator) update() {
	coords := d.coords
	n := len(coords) / 2

	d.triangles = d.triangles[:0]
	d.halfedges = d.halfedges[:0]
	d.hull = d.hull[:0]
	d.trianglesLen = 0

	if n == 0 {
		d.triangles = []int{}
		d.halfedges = []int{}
		d.hull = []int{}
		return
	}

	maxTriangles := 2*n - 5
	if maxTriangles < 0 {
		maxTriangles = 0
	}
	d.triangles = make([]int, maxTriangles*3)
	d.halfedges = make([]int, maxTriangles*3)
	d.hashSize = int(math.Ceil(math.Sqrt(float64(n))))
	d.hullPrev = make([]int, n)
	d.hullNext = make([]int, n)
	d.hullTri = make([]int, n)
	d.hullHash = make([]int, d.hashSize)
	d.ids = make([]int, n)
	d.dists = make([]float64, n)
	d.edgeStack = make([]int, 0, edgeStackCap)

	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for i := 0; i < n; i++ {
		x, y := coords[2*i], coords[2*i+1]
		if x < minX {
			minX = x
		}
		if y < minY {
			minY = y
		}
		if x > maxX {
			maxX = x
		}
		if y > maxY {
			maxY = y
		}
		d.ids[i] = i
	}
	cx := (minX + maxX) / 2
	cy := (minY + maxY) / 2

	// Seed point: the one closest to the bounding-box center. Starting near the
	// center is what keeps the seed triangle small and the radial sort useful.
	i0 := 0
	minDist := math.Inf(1)
	for i := 0; i < n; i++ {
		if dd := dist2(cx, cy, coords[2*i], coords[2*i+1]); dd < minDist {
			i0, minDist = i, dd
		}
	}
	i0x, i0y := coords[2*i0], coords[2*i0+1]

	// Second seed: the nearest *distinct* point. "d > 0" is what excludes
	// duplicates of the seed, which would otherwise give a degenerate edge.
	i1 := -1
	minDist = math.Inf(1)
	for i := 0; i < n; i++ {
		if i == i0 {
			continue
		}
		dd := dist2(i0x, i0y, coords[2*i], coords[2*i+1])
		if dd < minDist && dd > 0 {
			i1, minDist = i, dd
		}
	}
	if i1 == -1 {
		// Every point is a duplicate of the seed.
		d.degenerateHull()
		return
	}
	i1x, i1y := coords[2*i1], coords[2*i1+1]

	// Third seed: the point forming the smallest circumcircle with the first
	// two. Smallest, because it makes the seed triangle a good approximation of
	// the local Delaunay neighborhood, so few flips are needed early on.
	i2 := -1
	minRadius := math.Inf(1)
	for i := 0; i < n; i++ {
		if i == i0 || i == i1 {
			continue
		}
		if r := circumradius2(i0x, i0y, i1x, i1y, coords[2*i], coords[2*i+1]); r < minRadius {
			i2, minRadius = i, r
		}
	}
	if math.IsInf(minRadius, 1) || i2 == -1 {
		// No three points form a circle: the input is collinear (or has fewer
		// than three distinct points). There are no triangles, and the "hull"
		// is the sorted sequence of distinct points along the line.
		d.degenerateHull()
		return
	}
	i2x, i2y := coords[2*i2], coords[2*i2+1]

	// Orient the seed triangle counterclockwise; every predicate downstream
	// assumes it.
	if orient2d(i0x, i0y, i1x, i1y, i2x, i2y) < 0 {
		i1, i2 = i2, i1
		i1x, i2x = i2x, i1x
		i1y, i2y = i2y, i1y
	}

	d.cx, d.cy = circumcenter(i0x, i0y, i1x, i1y, i2x, i2y)

	for i := 0; i < n; i++ {
		d.dists[i] = dist2(coords[2*i], coords[2*i+1], d.cx, d.cy)
	}
	d.sortIDs()

	d.hullStart = i0
	hullSize := 3

	d.hullNext[i0], d.hullPrev[i2] = i1, i1
	d.hullNext[i1], d.hullPrev[i0] = i2, i2
	d.hullNext[i2], d.hullPrev[i1] = i0, i0

	d.hullTri[i0] = 0
	d.hullTri[i1] = 1
	d.hullTri[i2] = 2

	for i := range d.hullHash {
		d.hullHash[i] = -1
	}
	d.hullHash[d.hashKey(i0x, i0y)] = i0
	d.hullHash[d.hashKey(i1x, i1y)] = i1
	d.hullHash[d.hashKey(i2x, i2y)] = i2

	d.addTriangle(i0, i1, i2, -1, -1, -1)

	var xp, yp float64
	for k, i := range d.ids {
		x, y := coords[2*i], coords[2*i+1]

		// Near-duplicate points are skipped outright. Inserting one would give
		// a zero-area triangle whose circumcircle is undefined, and from there
		// every predicate is noise.
		if k > 0 && math.Abs(x-xp) <= epsilonDup && math.Abs(y-yp) <= epsilonDup {
			continue
		}
		xp, yp = x, y

		if i == i0 || i == i1 || i == i2 {
			continue
		}

		// Find some hull vertex near the new point's direction, then walk
		// backward to the first edge it can see.
		start := -1
		key := d.hashKey(x, y)
		for j := 0; j < d.hashSize; j++ {
			start = d.hullHash[(key+j)%d.hashSize]
			if start != -1 && start != d.hullNext[start] {
				break
			}
		}
		if start == -1 {
			start = d.hullStart
		}

		start = d.hullPrev[start]
		e := start
		for {
			q := d.hullNext[e]
			if orient2d(x, y, coords[2*e], coords[2*e+1], coords[2*q], coords[2*q+1]) < 0 {
				break
			}
			e = q
			if e == start {
				e = -1
				break
			}
		}
		if e == -1 {
			// The point sees no hull edge: it is a duplicate that survived the
			// epsilon test above (the radial sort can separate exact
			// duplicates). Skipping it is the only sound answer.
			continue
		}

		// Attach the first triangle of the fan, then legalize outward.
		t := d.addTriangle(e, i, d.hullNext[e], -1, -1, d.hullTri[e])
		d.hullTri[i] = d.legalize(t + 2)
		d.hullTri[e] = t
		hullSize++

		// Walk forward along the hull, adding triangles for as long as the new
		// point can see the next edge.
		nn := d.hullNext[e]
		for {
			q := d.hullNext[nn]
			if orient2d(x, y, coords[2*nn], coords[2*nn+1], coords[2*q], coords[2*q+1]) >= 0 {
				break
			}
			t = d.addTriangle(nn, i, q, d.hullTri[i], -1, d.hullTri[nn])
			d.hullTri[i] = d.legalize(t + 2)
			d.hullNext[nn] = nn // mark as removed from the hull
			hullSize--
			nn = q
		}

		// And backward, but only if the search above did not already move off
		// the starting edge.
		if e == start {
			for {
				q := d.hullPrev[e]
				if orient2d(x, y, coords[2*q], coords[2*q+1], coords[2*e], coords[2*e+1]) >= 0 {
					break
				}
				t = d.addTriangle(q, i, e, -1, d.hullTri[e], d.hullTri[q])
				d.legalize(t + 2)
				d.hullTri[q] = t
				d.hullNext[e] = e // mark as removed
				hullSize--
				e = q
			}
		}

		d.hullStart = e
		d.hullPrev[i] = e
		d.hullNext[e] = i
		d.hullPrev[nn] = i
		d.hullNext[i] = nn

		d.hullHash[d.hashKey(x, y)] = i
		d.hullHash[d.hashKey(coords[2*e], coords[2*e+1])] = e
	}

	d.hull = make([]int, hullSize)
	e := d.hullStart
	for i := 0; i < hullSize; i++ {
		d.hull[i] = e
		e = d.hullNext[e]
	}

	d.triangles = d.triangles[:d.trianglesLen]
	d.halfedges = d.halfedges[:d.trianglesLen]
}

// degenerateHull handles the two inputs that have no triangulation at all:
// fewer than three distinct points, and any number of collinear points. The
// result is a hull listing the distinct points in order along the line, and no
// triangles. Callers must be prepared for len(triangles) == 0; d3-delaunay's
// wrapper is, and so is [Delaunay].
func (d *delaunator) degenerateHull() {
	coords := d.coords
	n := len(coords) / 2
	// Order by displacement along the line: x first, y as the tiebreak for a
	// vertical line.
	for i := 0; i < n; i++ {
		dx := coords[2*i] - coords[0]
		if dx == 0 {
			dx = coords[2*i+1] - coords[1]
		}
		d.dists[i] = dx
	}
	d.sortIDs()

	hull := make([]int, 0, n)
	d0 := math.Inf(-1)
	for _, id := range d.ids {
		if dd := d.dists[id]; dd > d0 {
			hull = append(hull, id)
			d0 = dd
		}
	}
	d.hull = hull
	d.triangles = []int{}
	d.halfedges = []int{}
	d.trianglesLen = 0
}

// sortIDs orders the point indices by their dists value. Delaunator uses a
// hand-rolled unstable quicksort; this port sorts with an index tiebreak
// instead, which costs nothing measurable and makes the output a deterministic
// function of the input rather than of the sort's pivot choices.
func (d *delaunator) sortIDs() {
	ids, dists := d.ids, d.dists
	sort.Slice(ids, func(a, b int) bool {
		ia, ib := ids[a], ids[b]
		if dists[ia] != dists[ib] {
			return dists[ia] < dists[ib]
		}
		return ia < ib
	})
}

func (d *delaunator) hashKey(x, y float64) int {
	k := int(math.Floor(pseudoAngle(x-d.cx, y-d.cy) * float64(d.hashSize)))
	if k < 0 || k >= d.hashSize {
		// pseudoAngle is in [0, 1), so this only happens on non-finite input.
		k = ((k % d.hashSize) + d.hashSize) % d.hashSize
	}
	return k
}

// legalize restores the Delaunay condition around half-edge a by flipping
// illegal edges, and returns the half-edge that ends up opposite the newly
// inserted point (which the caller records as the hull triangle).
//
//	      pl                    pl
//	     /||\                  /  \
//	  al/ || \bl            al/    \a
//	   /  ||  \              /      \
//	  /  a||b  \    flip    /___ar___\
//	p0\   ||   /p1   =>   p0\---bl---/p1
//	   \  ||  /              \      /
//	  ar\ || /br             b\    /br
//	     \||/                  \  /
//	      pr                    pr
//
// Flipping is the whole algorithm: inserting a point can only violate the
// condition on edges facing it, and each flip pushes the two newly exposed
// edges back onto the stack. The recursion terminates because every flip
// strictly increases the minimum angle of the mesh.
func (d *delaunator) legalize(a int) int {
	triangles, halfedges, coords := d.triangles, d.halfedges, d.coords
	d.edgeStack = d.edgeStack[:0]
	ar := 0

	for {
		b := halfedges[a]

		a0 := a - a%3
		ar = a0 + (a+2)%3

		if b == -1 { // hull edge: nothing on the other side to be illegal
			if len(d.edgeStack) == 0 {
				break
			}
			a = d.edgeStack[len(d.edgeStack)-1]
			d.edgeStack = d.edgeStack[:len(d.edgeStack)-1]
			continue
		}

		b0 := b - b%3
		al := a0 + (a+1)%3
		bl := b0 + (b+2)%3

		p0 := triangles[ar]
		pr := triangles[a]
		pl := triangles[al]
		p1 := triangles[bl]

		illegal := inCircle(
			coords[2*p0], coords[2*p0+1],
			coords[2*pr], coords[2*pr+1],
			coords[2*pl], coords[2*pl+1],
			coords[2*p1], coords[2*p1+1])

		if illegal {
			triangles[a] = p1
			triangles[b] = p0

			hbl := halfedges[bl]

			// Rare: the flipped edge was itself on the hull, so a hull vertex's
			// recorded triangle now points at the wrong half-edge.
			if hbl == -1 {
				e := d.hullStart
				for {
					if d.hullTri[e] == bl {
						d.hullTri[e] = a
						break
					}
					e = d.hullPrev[e]
					if e == d.hullStart {
						break
					}
				}
			}
			d.link(a, hbl)
			d.link(b, halfedges[ar])
			d.link(ar, bl)

			br := b0 + (b+1)%3
			d.edgeStack = append(d.edgeStack, br)
		} else {
			if len(d.edgeStack) == 0 {
				break
			}
			a = d.edgeStack[len(d.edgeStack)-1]
			d.edgeStack = d.edgeStack[:len(d.edgeStack)-1]
		}
	}

	return ar
}

func (d *delaunator) link(a, b int) {
	d.halfedges[a] = b
	if b != -1 {
		d.halfedges[b] = a
	}
}

// addTriangle appends a triangle with the given vertices and adjacent
// half-edges, returning the id of its first half-edge.
func (d *delaunator) addTriangle(i0, i1, i2, a, b, c int) int {
	t := d.trianglesLen
	d.triangles[t] = i0
	d.triangles[t+1] = i1
	d.triangles[t+2] = i2
	d.link(t, a)
	d.link(t+1, b)
	d.link(t+2, c)
	d.trianglesLen += 3
	return t
}
