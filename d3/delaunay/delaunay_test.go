package delaunay

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Independent geometry, written for the tests only. Nothing here calls into the
// package's own predicates: a test that reuses the implementation's arithmetic
// cannot catch the implementation being wrong about it.
// ---------------------------------------------------------------------------

// testCircumcircle returns the center and squared radius of the circle through
// three points, and whether one exists.
func testCircumcircle(a, b, c Point) (Point, float64, bool) {
	d := 2 * (a.X*(b.Y-c.Y) + b.X*(c.Y-a.Y) + c.X*(a.Y-b.Y))
	if d == 0 || math.IsNaN(d) || math.IsInf(d, 0) {
		return Point{}, 0, false
	}
	a2 := a.X*a.X + a.Y*a.Y
	b2 := b.X*b.X + b.Y*b.Y
	c2 := c.X*c.X + c.Y*c.Y
	ux := (a2*(b.Y-c.Y) + b2*(c.Y-a.Y) + c2*(a.Y-b.Y)) / d
	uy := (a2*(c.X-b.X) + b2*(a.X-c.X) + c2*(b.X-a.X)) / d
	ctr := Point{ux, uy}
	r2 := (a.X-ux)*(a.X-ux) + (a.Y-uy)*(a.Y-uy)
	return ctr, r2, true
}

// testCross is the signed area (times two) of the triangle a, b, c, positive
// when the turn a→b→c is counterclockwise in the usual y-up sense.
func testCross(a, b, c Point) float64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

// testPolygonArea is twice the signed area of a ring, which may or may not
// repeat its first point at the end.
func testPolygonArea(ring []Point) float64 {
	n := len(ring)
	if n > 1 && ring[0] == ring[n-1] {
		n--
	}
	if n < 3 {
		return 0
	}
	a := 0.0
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		a += ring[i].X*ring[j].Y - ring[j].X*ring[i].Y
	}
	return a
}

// testInPolygon is the even-odd crossing test, with points on the boundary
// reported as inside.
func testInPolygon(ring []Point, p Point) bool {
	n := len(ring)
	if n > 1 && ring[0] == ring[n-1] {
		n--
	}
	in := false
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		pi, pj := ring[i], ring[j]
		// On an edge counts as inside; cells share boundaries and a strict test
		// would make a shared point belong to neither.
		if math.Abs(testCross(pi, pj, p)) < 1e-9 &&
			math.Min(pi.X, pj.X)-1e-9 <= p.X && p.X <= math.Max(pi.X, pj.X)+1e-9 &&
			math.Min(pi.Y, pj.Y)-1e-9 <= p.Y && p.Y <= math.Max(pi.Y, pj.Y)+1e-9 {
			return true
		}
		if (pi.Y > p.Y) != (pj.Y > p.Y) &&
			p.X < (pj.X-pi.X)*(p.Y-pi.Y)/(pj.Y-pi.Y)+pi.X {
			in = !in
		}
	}
	return in
}

func randomPoints(rng *rand.Rand, n int, scale float64) []Point {
	pts := make([]Point, n)
	for i := range pts {
		pts[i] = Point{rng.Float64() * scale, rng.Float64() * scale}
	}
	return pts
}

// ---------------------------------------------------------------------------
// Values that can be derived by hand.
// ---------------------------------------------------------------------------

func TestSingleTriangle(t *testing.T) {
	d := New([]Point{{0, 0}, {1, 0}, {0, 1}})
	if got := len(d.Triangles()); got != 3 {
		t.Fatalf("triangles = %d half-edges, want 3", got)
	}
	seen := map[int]bool{}
	for _, v := range d.Triangles() {
		seen[v] = true
	}
	if len(seen) != 3 {
		t.Errorf("triangle vertices = %v, want all of 0,1,2", d.Triangles())
	}
	for _, h := range d.Halfedges() {
		if h != -1 {
			t.Errorf("halfedges = %v, want all -1 for a lone triangle", d.Halfedges())
		}
	}
	if got := len(d.Hull()); got != 3 {
		t.Errorf("hull = %v, want 3 points", d.Hull())
	}
	if d.Collinear() {
		t.Error("Collinear() = true for a non-degenerate triangle")
	}
}

func TestSquareIsTwoTriangles(t *testing.T) {
	d := New([]Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}})
	if got := len(d.Triangles()); got != 6 {
		t.Fatalf("triangles = %d half-edges, want 6 (two triangles)", got)
	}
	if got := len(d.Hull()); got != 4 {
		t.Errorf("hull = %v, want all four corners", d.Hull())
	}
	// The two triangles share exactly one edge, so exactly two half-edges are
	// paired and four lie on the hull.
	paired := 0
	for _, h := range d.Halfedges() {
		if h != -1 {
			paired++
		}
	}
	if paired != 2 {
		t.Errorf("paired half-edges = %d, want 2", paired)
	}
	// Every corner appears in exactly one or two triangles, and the union of
	// the two triangle areas is the unit square.
	total := 0.0
	for _, ring := range d.TrianglePolygons() {
		total += math.Abs(testPolygonArea(ring)) / 2
	}
	if math.Abs(total-1) > 1e-12 {
		t.Errorf("total triangle area = %v, want 1", total)
	}
}

func TestRenderShapes(t *testing.T) {
	d := New([]Point{{0, 0}, {1, 0}, {0, 1}})
	if got := d.RenderHull(); got != "M0,0L1,0L0,1Z" && got != "M0,0L0,1L1,0Z" &&
		got != "M1,0L0,1L0,0Z" && got != "M1,0L0,0L0,1Z" &&
		got != "M0,1L0,0L1,0Z" && got != "M0,1L1,0L0,0Z" {
		t.Errorf("RenderHull() = %q, want the closed unit triangle in some rotation", got)
	}
	if got := d.RenderTriangle(0); !strings.HasPrefix(got, "M") || !strings.HasSuffix(got, "Z") {
		t.Errorf("RenderTriangle(0) = %q, want a closed subpath", got)
	}
	if got := d.RenderTriangle(7); got != "" {
		t.Errorf("RenderTriangle(out of range) = %q, want \"\"", got)
	}
	if got := d.RenderPoints(2); strings.Count(got, "M") != 3 {
		t.Errorf("RenderPoints drew %d subpaths, want 3", strings.Count(got, "M"))
	}
	if got := d.Render(); !strings.HasPrefix(got, "M") {
		t.Errorf("Render() = %q, want path data", got)
	}
}

// ---------------------------------------------------------------------------
// The Delaunay condition, and the structural invariants that go with it.
// ---------------------------------------------------------------------------

// checkDelaunay verifies the defining property: no input point lies strictly
// inside the circumcircle of any triangle.
func checkDelaunay(t *testing.T, d *Delaunay, label string) {
	t.Helper()
	pts := d.Points()
	tri := d.Triangles()
	for i := 0; i < len(tri); i += 3 {
		a, b, c := pts[tri[i]], pts[tri[i+1]], pts[tri[i+2]]
		ctr, r2, ok := testCircumcircle(a, b, c)
		if !ok {
			t.Errorf("%s: triangle %d is degenerate: %v %v %v", label, i/3, a, b, c)
			continue
		}
		for k, p := range pts {
			if k == tri[i] || k == tri[i+1] || k == tri[i+2] {
				continue
			}
			dd := (p.X-ctr.X)*(p.X-ctr.X) + (p.Y-ctr.Y)*(p.Y-ctr.Y)
			// Relative tolerance: cocircular points are legal, only strictly
			// interior ones are not.
			if dd < r2*(1-1e-9) {
				t.Fatalf("%s: point %d %v lies inside the circumcircle of triangle %d (%v %v %v): d2=%v r2=%v",
					label, k, p, i/3, a, b, c, dd, r2)
			}
		}
	}
}

func checkStructure(t *testing.T, d *Delaunay, label string) {
	t.Helper()
	tri, half, hull, pts := d.Triangles(), d.Halfedges(), d.Hull(), d.Points()

	if len(tri) != len(half) {
		t.Fatalf("%s: %d triangle entries but %d half-edges", label, len(tri), len(half))
	}

	// Half-edge pairing is an involution.
	for e, o := range half {
		if o == -1 {
			continue
		}
		if o < 0 || o >= len(half) {
			t.Fatalf("%s: halfedges[%d] = %d out of range", label, e, o)
		}
		if half[o] != e {
			t.Fatalf("%s: halfedges[halfedges[%d]] = %d, want %d", label, e, half[o], e)
		}
		// Paired half-edges join the same two points in opposite directions.
		if tri[e] != tri[nextHalfedgeTest(o)] || tri[o] != tri[nextHalfedgeTest(e)] {
			t.Fatalf("%s: half-edges %d and %d are paired but do not share an edge", label, e, o)
		}
	}

	// Every triangle is wound the same way and has non-zero area.
	var wind float64
	for i := 0; i < len(tri); i += 3 {
		s := testCross(pts[tri[i]], pts[tri[i+1]], pts[tri[i+2]])
		if s == 0 {
			t.Fatalf("%s: triangle %d has zero area", label, i/3)
		}
		if wind == 0 {
			wind = s
		} else if (s > 0) != (wind > 0) {
			t.Fatalf("%s: triangle %d is wound against the others", label, i/3)
		}
	}

	// Every distinct point takes part in the mesh (duplicates are dropped, and
	// those are exactly the points with no incoming half-edge).
	inTri := map[int]bool{}
	for _, v := range tri {
		inTri[v] = true
	}
	seen := map[Point]bool{}
	for i, p := range pts {
		if seen[p] {
			continue // a duplicate; only the first copy is required to appear
		}
		seen[p] = true
		if !inTri[i] && len(tri) > 0 {
			t.Errorf("%s: point %d %v is in no triangle", label, i, p)
		}
	}

	// The hull is convex, consistently wound, and no point lies outside it.
	if n := len(hull); n >= 3 {
		var hwind float64
		for i := range hull {
			a, b, c := pts[hull[i]], pts[hull[(i+1)%n]], pts[hull[(i+2)%n]]
			s := testCross(a, b, c)
			if math.Abs(s) < 1e-12 {
				continue // collinear hull points are permitted
			}
			if hwind == 0 {
				hwind = s
			} else if (s > 0) != (hwind > 0) {
				t.Fatalf("%s: hull is not convex at %d (%v %v %v)", label, i, a, b, c)
			}
		}
		if wind != 0 && hwind != 0 && (wind > 0) != (hwind > 0) {
			t.Errorf("%s: hull winds against the triangles", label)
		}
		ring := make([]Point, 0, n)
		for _, h := range hull {
			ring = append(ring, pts[h])
		}
		for i, p := range pts {
			if !testInPolygon(ring, p) {
				t.Errorf("%s: point %d %v lies outside the hull", label, i, p)
			}
		}
	}
}

func nextHalfedgeTest(e int) int {
	if e%3 == 2 {
		return e - 2
	}
	return e + 1
}

func TestDelaunayConditionOnRandomPoints(t *testing.T) {
	// Several hundred independent point sets. The Delaunay condition is not a
	// smoke test: an implementation with a sign error, a missed edge flip or a
	// broken half-edge link cannot satisfy it even once by accident.
	sets := 0
	for seed := int64(0); seed < 200; seed++ {
		rng := rand.New(rand.NewSource(seed))
		n := 3 + rng.Intn(40)
		pts := randomPoints(rng, n, 100)
		d := New(pts)
		label := fmt.Sprintf("seed %d (n=%d)", seed, n)
		checkDelaunay(t, d, label)
		checkStructure(t, d, label)
		sets++
		if t.Failed() {
			return
		}
	}
	if sets < 200 {
		t.Fatalf("only checked %d point sets", sets)
	}
}

func TestDelaunayConditionOnLargeAndClusteredPoints(t *testing.T) {
	for seed := int64(0); seed < 30; seed++ {
		rng := rand.New(rand.NewSource(seed + 1000))

		// A few hundred uniform points.
		big := randomPoints(rng, 300, 1000)
		label := fmt.Sprintf("uniform seed %d", seed)
		checkDelaunay(t, New(big), label)
		checkStructure(t, New(big), label)

		// Clustered points, which stress the hull hash and produce many flips.
		clustered := make([]Point, 0, 200)
		for c := 0; c < 5; c++ {
			cx, cy := rng.Float64()*1000, rng.Float64()*1000
			for i := 0; i < 40; i++ {
				clustered = append(clustered, Point{cx + rng.NormFloat64(), cy + rng.NormFloat64()})
			}
		}
		label = fmt.Sprintf("clustered seed %d", seed)
		checkDelaunay(t, New(clustered), label)
		checkStructure(t, New(clustered), label)

		// Points on a grid, which are massively cocircular — the case where a
		// naive in-circle test flips forever.
		var grid []Point
		for i := 0; i < 12; i++ {
			for j := 0; j < 12; j++ {
				grid = append(grid, Point{float64(i), float64(j)})
			}
		}
		label = "12x12 grid"
		checkDelaunay(t, New(grid), label)
		checkStructure(t, New(grid), label)

		if t.Failed() {
			return
		}
	}
}

func TestDelaunayWithDuplicatePoints(t *testing.T) {
	for seed := int64(0); seed < 40; seed++ {
		rng := rand.New(rand.NewSource(seed + 2000))
		base := randomPoints(rng, 12, 50)
		pts := append([]Point(nil), base...)
		// Duplicate a third of them, so most points have a twin.
		for i := 0; i < 6; i++ {
			pts = append(pts, base[rng.Intn(len(base))])
		}
		d := New(pts)
		label := fmt.Sprintf("duplicates seed %d", seed)
		checkDelaunay(t, d, label)
		checkStructure(t, d, label)

		// A dropped duplicate has no incoming half-edge and no neighbors,
		// rather than a wrong answer.
		inedges := d.Inedges()
		for i := range pts {
			if inedges[i] == -1 && len(d.Neighbors(i)) != 0 {
				t.Errorf("%s: dropped point %d has neighbors %v", label, i, d.Neighbors(i))
			}
		}
		if t.Failed() {
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Degenerate input.
// ---------------------------------------------------------------------------

func TestNoPoints(t *testing.T) {
	d := New(nil)
	if len(d.Points()) != 0 || len(d.Triangles()) != 0 || len(d.Hull()) != 0 {
		t.Errorf("empty input produced %d points, %d triangle entries, %d hull",
			len(d.Points()), len(d.Triangles()), len(d.Hull()))
	}
	if got := d.Find(0, 0); got != -1 {
		t.Errorf("Find on empty = %d, want -1", got)
	}
	if got := d.Render(); got != "" {
		t.Errorf("Render on empty = %q", got)
	}
	if got := d.Neighbors(0); got != nil {
		t.Errorf("Neighbors(0) on empty = %v", got)
	}
	// A Voronoi of nothing must still be constructible and answer nil.
	v := d.Voronoi(0, 0, 10, 10)
	if got := v.CellPolygons(); len(got) != 0 {
		t.Errorf("CellPolygons on empty = %v", got)
	}
	if got := v.Render(); got != "" {
		t.Errorf("Voronoi.Render on empty = %q", got)
	}
}

func TestOnePoint(t *testing.T) {
	d := New([]Point{{3, 4}})
	if got := len(d.Hull()); got != 1 {
		t.Errorf("hull = %v, want one point", d.Hull())
	}
	if got := d.Find(100, 100); got != 0 {
		t.Errorf("Find = %d, want 0", got)
	}
	if got := d.Neighbors(0); len(got) != 0 {
		t.Errorf("Neighbors(0) = %v, want none", got)
	}
	// Its cell is the whole extent.
	v := d.Voronoi(0, 0, 10, 20)
	ring := v.CellPolygon(0)
	if a := math.Abs(testPolygonArea(ring)) / 2; math.Abs(a-200) > 1e-9 {
		t.Errorf("lone cell area = %v, want 200 (the whole extent)", a)
	}
}

func TestCoincidentPointsOnly(t *testing.T) {
	d := New([]Point{{5, 5}, {5, 5}, {5, 5}})
	if got := len(d.Hull()); got != 1 {
		t.Errorf("hull = %v, want a single distinct point", d.Hull())
	}
	if got := len(d.Triangles()); got != 3 {
		t.Errorf("triangles = %v, want the degenerate placeholder", d.Triangles())
	}
	if got := d.Find(0, 0); got < 0 || got > 2 {
		t.Errorf("Find = %d, want one of the coincident points", got)
	}
}

func TestTwoPoints(t *testing.T) {
	d := New([]Point{{0, 0}, {10, 0}})
	if got := len(d.Hull()); got != 2 {
		t.Errorf("hull = %v, want two points", d.Hull())
	}
	if got := d.Neighbors(0); len(got) != 1 || got[0] != 1 {
		t.Errorf("Neighbors(0) = %v, want [1]", got)
	}
	if got := d.Neighbors(1); len(got) != 1 || got[0] != 0 {
		t.Errorf("Neighbors(1) = %v, want [0]", got)
	}
	if got := d.Find(1, 0); got != 0 {
		t.Errorf("Find(1,0) = %d, want 0", got)
	}
	if got := d.Find(9, 0); got != 1 {
		t.Errorf("Find(9,0) = %d, want 1", got)
	}

	// The two cells are half-planes split by the perpendicular bisector x = 5,
	// so each is exactly half the extent.
	v := d.Voronoi(0, -10, 10, 10)
	for i := 0; i < 2; i++ {
		ring := v.CellPolygon(i)
		if ring == nil {
			t.Fatalf("cell %d is nil", i)
		}
		a := math.Abs(testPolygonArea(ring)) / 2
		if math.Abs(a-100) > 1e-9 {
			t.Errorf("cell %d area = %v, want 100 (half the 200-unit extent)", i, a)
		}
	}
}

func TestCollinearPoints(t *testing.T) {
	for _, tc := range []struct {
		name string
		pts  []Point
	}{
		{"horizontal", []Point{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}}},
		{"vertical", []Point{{0, 0}, {0, 1}, {0, 2}, {0, 3}}},
		{"diagonal", []Point{{0, 0}, {1, 1}, {2, 2}, {3, 3}, {4, 4}, {5, 5}}},
		{"unsorted", []Point{{3, 0}, {0, 0}, {4, 0}, {1, 0}, {2, 0}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := New(tc.pts)
			if !d.Collinear() {
				t.Fatal("Collinear() = false for collinear input")
			}
			// Neighbors come from the order along the line, not from the
			// jittered mesh, so they are exact.
			for i, p := range tc.pts {
				ns := d.Neighbors(i)
				if len(ns) == 0 || len(ns) > 2 {
					t.Errorf("point %d %v has %d neighbors, want 1 or 2", i, p, len(ns))
				}
				for _, j := range ns {
					if j == i {
						t.Errorf("point %d is its own neighbor", i)
					}
				}
			}
			// The endpoints of the line have exactly one neighbor each.
			ends := 0
			for i := range tc.pts {
				if len(d.Neighbors(i)) == 1 {
					ends++
				}
			}
			if ends != 2 {
				t.Errorf("%d endpoints with a single neighbor, want 2", ends)
			}
			// The jitter is tiny relative to the extent.
			for i, p := range d.Points() {
				if math.Hypot(p.X-tc.pts[i].X, p.Y-tc.pts[i].Y) > 1e-6 {
					t.Errorf("point %d moved from %v to %v, more than a jitter", i, tc.pts[i], p)
				}
			}
			// The Voronoi of a line still tiles the extent.
			v := d.Voronoi(-10, -10, 20, 20)
			total := 0.0
			for _, c := range v.CellPolygons() {
				total += math.Abs(testPolygonArea(c.Polygon)) / 2
			}
			if math.Abs(total-900) > 1e-3 {
				t.Errorf("cells cover %v of the 900-unit extent", total)
			}
		})
	}
}

func TestNearlyCollinearThenNot(t *testing.T) {
	// One point off the line is enough to make a real triangulation.
	d := New([]Point{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {1.5, 1}})
	if d.Collinear() {
		t.Fatal("Collinear() = true although one point is off the line")
	}
	checkDelaunay(t, d, "almost collinear")
	checkStructure(t, d, "almost collinear")
}

// ---------------------------------------------------------------------------
// Find.
// ---------------------------------------------------------------------------

func TestFindMatchesBruteForce(t *testing.T) {
	for seed := int64(0); seed < 25; seed++ {
		rng := rand.New(rand.NewSource(seed + 3000))
		pts := randomPoints(rng, 60, 100)
		d := New(pts)
		for q := 0; q < 100; q++ {
			qx, qy := rng.Float64()*120-10, rng.Float64()*120-10
			best, bestD := -1, math.Inf(1)
			for i, p := range pts {
				if dd := (p.X-qx)*(p.X-qx) + (p.Y-qy)*(p.Y-qy); dd < bestD {
					best, bestD = i, dd
				}
			}
			got := d.FindFrom(qx, qy, rng.Intn(len(pts)))
			gp := pts[got]
			gotD := (gp.X-qx)*(gp.X-qx) + (gp.Y-qy)*(gp.Y-qy)
			if math.Abs(gotD-bestD) > 1e-9 {
				t.Fatalf("seed %d: Find(%v,%v) = %d %v (d2=%v), want %d %v (d2=%v)",
					seed, qx, qy, got, gp, gotD, best, pts[best], bestD)
			}
		}
	}
}

func TestFindOnPoints(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	pts := randomPoints(rng, 40, 10)
	d := New(pts)
	for i, p := range pts {
		if got := d.Find(p.X, p.Y); got != i {
			t.Errorf("Find at point %d = %d", i, got)
		}
	}
	if got := d.Find(math.NaN(), 0); got != -1 {
		t.Errorf("Find(NaN, 0) = %d, want -1", got)
	}
}

func TestFrom(t *testing.T) {
	type row struct{ a, b float64 }
	data := []row{{0, 0}, {1, 0}, {0, 1}}
	d := From(data, func(r row, _ int, _ []row) float64 { return r.a },
		func(r row, _ int, _ []row) float64 { return r.b })
	if got := len(d.Triangles()); got != 3 {
		t.Errorf("From produced %d half-edges, want 3", got)
	}
}

func TestNeighborsAreSymmetric(t *testing.T) {
	for seed := int64(0); seed < 20; seed++ {
		rng := rand.New(rand.NewSource(seed + 4000))
		pts := randomPoints(rng, 30, 100)
		d := New(pts)
		for i := range pts {
			for _, j := range d.Neighbors(i) {
				found := false
				for _, k := range d.Neighbors(j) {
					if k == i {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("seed %d: %d lists %d as a neighbor but not the reverse", seed, i, j)
				}
			}
		}
	}
}

func TestInputIsNotMutated(t *testing.T) {
	pts := []Point{{0, 0}, {1, 0}, {2, 0}}
	before := append([]Point(nil), pts...)
	New(pts)
	for i := range pts {
		if pts[i] != before[i] {
			t.Fatalf("collinear input was mutated: %v became %v", before, pts)
		}
	}
}

func TestCoordsAndHullPolygon(t *testing.T) {
	pts := []Point{{0, 0}, {4, 0}, {4, 3}, {0, 3}, {2, 1}}
	d := New(pts)

	coords := d.Coords()
	if len(coords) != 10 {
		t.Fatalf("Coords() has %d entries, want 10", len(coords))
	}
	for i, p := range pts {
		if coords[2*i] != p.X || coords[2*i+1] != p.Y {
			t.Errorf("Coords()[%d] = (%v,%v), want %v", i, coords[2*i], coords[2*i+1], p)
		}
	}
	// The getters hand back copies, so mutating one cannot corrupt the mesh.
	coords[0] = 999
	if d.Coords()[0] != 0 {
		t.Error("Coords() returned a live slice")
	}
	tri := d.Triangles()
	tri[0] = -5
	if d.Triangles()[0] == -5 {
		t.Error("Triangles() returned a live slice")
	}

	ring := d.HullPolygon()
	if ring == nil || ring[0] != ring[len(ring)-1] {
		t.Fatalf("HullPolygon() = %v, want a closed ring", ring)
	}
	// The hull is the 4x3 rectangle; the interior point does not appear.
	if a := math.Abs(testPolygonArea(ring)) / 2; math.Abs(a-12) > 1e-12 {
		t.Errorf("hull area = %v, want 12", a)
	}
	if len(ring) != 5 {
		t.Errorf("hull ring has %d vertices, want 4 plus the closing repeat", len(ring))
	}
	if New(nil).HullPolygon() != nil {
		t.Error("HullPolygon() of an empty triangulation is not nil")
	}
}

func TestRenderDrawsEveryEdgeOnce(t *testing.T) {
	rng := rand.New(rand.NewSource(12))
	pts := randomPoints(rng, 30, 100)
	d := New(pts)

	paired := 0
	for _, h := range d.Halfedges() {
		if h != -1 {
			paired++
		}
	}
	// One subpath per interior edge, plus one for the closed hull.
	want := paired/2 + 1
	if got := strings.Count(d.Render(), "M"); got != want {
		t.Errorf("Render() drew %d subpaths, want %d", got, want)
	}
	if got := len(d.TrianglePolygons()); got != len(d.Triangles())/3 {
		t.Errorf("TrianglePolygons() returned %d rings for %d triangles", got, len(d.Triangles())/3)
	}
	if got := d.TrianglePolygon(-1); got != nil {
		t.Errorf("TrianglePolygon(-1) = %v, want nil", got)
	}
	// A non-positive radius falls back to the default rather than drawing
	// nothing at all.
	if got := strings.Count(d.RenderPoints(0), "M"); got != 30 {
		t.Errorf("RenderPoints(0) drew %d points, want 30", got)
	}
}
