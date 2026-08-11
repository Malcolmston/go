package contour

import (
	"math"
	"math/rand"
	"testing"
)

// ---------------------------------------------------------------------------
// Test-only geometry, deliberately not reusing the package's own helpers.
// ---------------------------------------------------------------------------

// area is the signed area of a ring in screen coordinates — x right, y down,
// which is what an SVG contour is drawn in — so it is positive for a ring wound
// clockwise on the page. That is the winding the package gives exterior rings,
// and the sign of this function is what the tests check it against.
//
// It is the shoelace formula written out here rather than a call to the
// package's own ringArea, so that a sign error in the implementation cannot
// cancel itself out.
func area(r Ring) float64 {
	n := len(r)
	if n > 1 && r[0] == r[n-1] {
		n--
	}
	if n < 3 {
		return 0
	}
	a := 0.0
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		a += r[j].X*r[i].Y - r[i].X*r[j].Y
	}
	return a / 2
}

func inRing(r Ring, p Point) bool {
	n := len(r)
	if n > 1 && r[0] == r[n-1] {
		n--
	}
	in := false
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		pi, pj := r[i], r[j]
		if (pi.Y > p.Y) != (pj.Y > p.Y) && p.X < (pj.X-pi.X)*(p.Y-pi.Y)/(pj.Y-pi.Y)+pi.X {
			in = !in
		}
	}
	return in
}

func bbox(r Ring) (x0, y0, x1, y1 float64) {
	x0, y0 = math.Inf(1), math.Inf(1)
	x1, y1 = math.Inf(-1), math.Inf(-1)
	for _, p := range r {
		x0, x1 = math.Min(x0, p.X), math.Max(x1, p.X)
		y0, y1 = math.Min(y0, p.Y), math.Max(y1, p.Y)
	}
	return
}

func ringsEqual(got, want Ring, tol float64) bool {
	// Compare as cyclic sequences: the starting vertex of a stitched ring is an
	// artifact of the traversal order, not of the geometry.
	g, w := got, want
	if len(g) > 1 && g[0] == g[len(g)-1] {
		g = g[:len(g)-1]
	}
	if len(w) > 1 && w[0] == w[len(w)-1] {
		w = w[:len(w)-1]
	}
	if len(g) != len(w) {
		return false
	}
	for off := 0; off < len(g); off++ {
		ok := true
		for i := range w {
			p := g[(i+off)%len(g)]
			if math.Abs(p.X-w[i].X) > tol || math.Abs(p.Y-w[i].Y) > tol {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Values that can be derived by hand.
// ---------------------------------------------------------------------------

func TestUniformGridAboveThresholdIsEmpty(t *testing.T) {
	values := make([]float64, 16)
	for i := range values {
		values[i] = 1
	}
	c := NewContours().Size(4, 4)
	got := c.ComputeOne(values, 2)
	if len(got.Coordinates) != 0 {
		t.Errorf("contour of a uniform grid at a higher threshold = %v, want nothing", got.Coordinates)
	}
	if got.Type != "MultiPolygon" || got.Value != 2 {
		t.Errorf("got %+v", got)
	}
}

func TestThresholdBelowMinimumIsTheWholeGrid(t *testing.T) {
	// Every cell is above the threshold, so the contour is the outline of the
	// grid — with its four corners cut, because marching squares places the
	// boundary halfway between the outermost cell centers and the imaginary
	// ring of below-threshold cells around them.
	values := make([]float64, 9)
	for i := range values {
		values[i] = 1
	}
	got := NewContours().Size(3, 3).ComputeOne(values, 0)
	if len(got.Coordinates) != 1 || len(got.Coordinates[0]) != 1 {
		t.Fatalf("got %d polygons, want one polygon with one ring: %v", len(got.Coordinates), got.Coordinates)
	}
	ring := got.Coordinates[0][0]
	if ring[0] != ring[len(ring)-1] {
		t.Errorf("ring is not closed: %v", ring)
	}
	// 3×3 grid = 9 square units, less four corner triangles of 1/8 each.
	if a := math.Abs(area(ring)); math.Abs(a-8.5) > 1e-12 {
		t.Errorf("area = %v, want 8.5", a)
	}
	if x0, y0, x1, y1 := bbox(ring); x0 != 0 || y0 != 0 || x1 != 3 || y1 != 3 {
		t.Errorf("bounds = (%v,%v)-(%v,%v), want (0,0)-(3,3)", x0, y0, x1, y1)
	}
	if area(ring) <= 0 {
		t.Errorf("the whole-grid ring is wound as a hole")
	}
}

func TestSingleCellCrossing(t *testing.T) {
	// One hot cell at grid index (0,0) — cell center (0.5, 0.5) — in a 2×2
	// grid. At a threshold exactly halfway between the hot and cold values the
	// contour is the diamond through the four edge midpoints around it, and
	// smoothing moves nothing because the crossing already is at the midpoint.
	values := []float64{1, 0, 0, 0}
	want := Ring{{0.5, 0}, {0, 0.5}, {0.5, 1}, {1, 0.5}}

	got := NewContours().Size(2, 2).ComputeOne(values, 0.5)
	if len(got.Coordinates) != 1 || len(got.Coordinates[0]) != 1 {
		t.Fatalf("got %v, want a single ring", got.Coordinates)
	}
	ring := got.Coordinates[0][0]
	if !ringsEqual(ring, want, 1e-12) {
		t.Errorf("ring = %v, want the diamond %v", ring, want)
	}
	if a := math.Abs(area(ring)); math.Abs(a-0.5) > 1e-12 {
		t.Errorf("area = %v, want 0.5", a)
	}
}

func TestSmoothingMovesVerticesAlongTheirEdges(t *testing.T) {
	// Same grid, but a threshold three quarters of the way down from the hot
	// value to the cold one. The two vertices with an integer coordinate can
	// slide along their edge and do: 1 + 0.75 - 0.5 = 1.25. The two on the
	// outer boundary of the grid have nothing to interpolate against and stay.
	values := []float64{1, 0, 0, 0}

	smooth := NewContours().Size(2, 2).ComputeOne(values, 0.25).Coordinates[0][0]
	wantSmooth := Ring{{0.5, 0}, {0, 0.5}, {0.5, 1.25}, {1.25, 0.5}}
	if !ringsEqual(smooth, wantSmooth, 1e-12) {
		t.Errorf("smoothed ring = %v, want %v", smooth, wantSmooth)
	}

	blocky := NewContours().Size(2, 2).Smooth(false).ComputeOne(values, 0.25).Coordinates[0][0]
	wantBlocky := Ring{{0.5, 0}, {0, 0.5}, {0.5, 1}, {1, 0.5}}
	if !ringsEqual(blocky, wantBlocky, 1e-12) {
		t.Errorf("unsmoothed ring = %v, want %v", blocky, wantBlocky)
	}
}

func TestUnsmoothedVerticesStayOnTheGrid(t *testing.T) {
	// With smoothing off every vertex sits on a cell corner or an edge
	// midpoint, so every coordinate is a multiple of a half — which is exactly
	// what makes an unsmoothed contour look blocky.
	rng := rand.New(rand.NewSource(3))
	values := make([]float64, 20*20)
	for i := range values {
		values[i] = rng.Float64()
	}
	c := NewContours().Size(20, 20).Smooth(false)
	for _, mp := range c.ThresholdCount(6).Compute(values) {
		for _, poly := range mp.Coordinates {
			for _, ring := range poly {
				for _, p := range ring {
					if math.Mod(p.X*2, 1) != 0 || math.Mod(p.Y*2, 1) != 0 {
						t.Fatalf("unsmoothed vertex %v is not on a half-integer", p)
					}
				}
			}
		}
	}
	// And with smoothing on, at least some vertex is not, or the option would
	// be doing nothing.
	off := 0
	for _, mp := range NewContours().Size(20, 20).ThresholdCount(6).Compute(values) {
		for _, poly := range mp.Coordinates {
			for _, ring := range poly {
				for _, p := range ring {
					if math.Mod(p.X*2, 1) != 0 || math.Mod(p.Y*2, 1) != 0 {
						off++
					}
				}
			}
		}
	}
	if off == 0 {
		t.Error("smoothing left every vertex on the grid")
	}
}

func TestHoleWindingIsOpposite(t *testing.T) {
	// A 5×5 grid of ones with a single zero in the middle: above the threshold
	// the region is the whole grid with a hole punched in it. The shell and the
	// hole must wind opposite ways, or a renderer cannot fill it correctly.
	values := make([]float64, 25)
	for i := range values {
		values[i] = 1
	}
	values[2*5+2] = 0

	got := NewContours().Size(5, 5).ComputeOne(values, 0.5)
	if len(got.Coordinates) != 1 {
		t.Fatalf("got %d polygons, want 1: %v", len(got.Coordinates), got.Coordinates)
	}
	poly := got.Coordinates[0]
	if len(poly) != 2 {
		t.Fatalf("got %d rings, want a shell and a hole", len(poly))
	}
	shell, hole := poly[0], poly[1]
	if area(shell) <= 0 {
		t.Errorf("shell area = %v, want positive", area(shell))
	}
	if area(hole) >= 0 {
		t.Errorf("hole area = %v, want negative (wound against the shell)", area(hole))
	}
	if math.Abs(area(shell)) <= math.Abs(area(hole)) {
		t.Error("the hole is not smaller than the shell")
	}
	// And the hole really is inside the shell.
	for _, p := range hole {
		if !inRing(shell, p) {
			t.Fatalf("hole vertex %v is outside the shell", p)
		}
	}
	// The hole encloses the cell center (2.5, 2.5) that produced it.
	if !inRing(hole, Point{2.5, 2.5}) {
		t.Errorf("the hole does not enclose the low cell it was cut around")
	}
}

// ---------------------------------------------------------------------------
// Properties.
// ---------------------------------------------------------------------------

func TestRingsAreClosedAndNonDegenerate(t *testing.T) {
	for seed := int64(0); seed < 60; seed++ {
		rng := rand.New(rand.NewSource(seed))
		dx, dy := 2+rng.Intn(20), 2+rng.Intn(20)
		values := make([]float64, dx*dy)
		for i := range values {
			values[i] = rng.NormFloat64()
		}
		for _, smooth := range []bool{true, false} {
			c := NewContours().Size(dx, dy).Smooth(smooth).ThresholdCount(5)
			for _, mp := range c.Compute(values) {
				for pi, poly := range mp.Coordinates {
					for ri, ring := range poly {
						if len(ring) < 4 {
							t.Fatalf("seed %d: ring %d/%d has %d vertices", seed, pi, ri, len(ring))
						}
						if ring[0] != ring[len(ring)-1] {
							t.Fatalf("seed %d: ring %d/%d is not closed: %v .. %v",
								seed, pi, ri, ring[0], ring[len(ring)-1])
						}
						if area(ring) == 0 {
							t.Fatalf("seed %d: ring %d/%d has zero area", seed, pi, ri)
						}
						if ri == 0 && area(ring) < 0 {
							t.Fatalf("seed %d: shell %d is wound as a hole", seed, pi)
						}
						if ri > 0 && area(ring) > 0 {
							t.Fatalf("seed %d: hole %d/%d is wound as a shell", seed, pi, ri)
						}
						// Every vertex is within the grid.
						for _, p := range ring {
							if p.X < 0 || p.X > float64(dx) || p.Y < 0 || p.Y > float64(dy) {
								t.Fatalf("seed %d: vertex %v outside the %dx%d grid", seed, p, dx, dy)
							}
						}
					}
				}
			}
		}
	}
}

func TestContoursAreNestedByThreshold(t *testing.T) {
	// A single smooth peak: the region above a higher threshold must lie inside
	// the region above a lower one, and be smaller.
	const n = 40
	values := make([]float64, n*n)
	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			values[j*n+i] = 100 - math.Hypot(float64(i)-19.5, float64(j)-19.5)*4
		}
	}
	thresholds := []float64{10, 30, 50, 70, 90}
	got := NewContours().Size(n, n).Thresholds(thresholds).Compute(values)
	if len(got) != len(thresholds) {
		t.Fatalf("got %d contours, want %d", len(got), len(thresholds))
	}

	prevArea := math.Inf(1)
	var prev Ring
	for k, mp := range got {
		if mp.Value != thresholds[k] {
			t.Errorf("contour %d has value %v, want %v", k, mp.Value, thresholds[k])
		}
		if len(mp.Coordinates) != 1 || len(mp.Coordinates[0]) != 1 {
			t.Fatalf("contour %d: got %v, want a single ring", k, mp.Coordinates)
		}
		ring := mp.Coordinates[0][0]
		a := area(ring)
		if a >= prevArea {
			t.Errorf("contour %d (threshold %v) has area %v, not smaller than %v",
				k, mp.Value, a, prevArea)
		}
		if prev != nil {
			for _, p := range ring {
				if !inRing(prev, p) {
					t.Fatalf("contour %d (threshold %v) has vertex %v outside the previous contour",
						k, mp.Value, p)
				}
			}
		}
		// The peak is inside every contour.
		if !inRing(ring, Point{20, 20}) {
			t.Errorf("contour %d does not contain the peak", k)
		}
		prevArea, prev = a, ring
	}
}

func TestContourAreaShrinksWithThreshold(t *testing.T) {
	// The same monotonicity, but on noisy data and measured in total area
	// rather than by containment: a higher threshold selects a subset of the
	// cells, so it can never enclose more.
	for seed := int64(0); seed < 20; seed++ {
		rng := rand.New(rand.NewSource(seed + 100))
		const n = 24
		values := make([]float64, n*n)
		for i := range values {
			values[i] = rng.Float64()
		}
		prev := math.Inf(1)
		for _, th := range []float64{0.1, 0.3, 0.5, 0.7, 0.9} {
			total := 0.0
			for _, poly := range NewContours().Size(n, n).ComputeOne(values, th).Coordinates {
				for _, ring := range poly {
					total += area(ring) // holes are negative, so this is net area
				}
			}
			if total > prev+1e-9 {
				t.Fatalf("seed %d: area at threshold %v is %v, more than %v at the previous",
					seed, th, total, prev)
			}
			prev = total
		}
	}
}

func TestThresholdSelection(t *testing.T) {
	values := make([]float64, 100)
	for i := range values {
		values[i] = float64(i)
	}
	// Explicit thresholds come back sorted, one contour each.
	got := NewContours().Size(10, 10).Thresholds([]float64{50, 10, 30}).Compute(values)
	if len(got) != 3 {
		t.Fatalf("got %d contours, want 3", len(got))
	}
	for i, want := range []float64{10, 30, 50} {
		if got[i].Value != want {
			t.Errorf("contour %d has value %v, want %v", i, got[i].Value, want)
		}
	}
	// A count gives round numbers strictly inside the data range.
	got = NewContours().Size(10, 10).ThresholdCount(5).Compute(values)
	if len(got) == 0 {
		t.Fatal("no contours for a count of 5")
	}
	for _, mp := range got {
		if mp.Value >= 99 {
			t.Errorf("threshold %v is at or above the maximum value", mp.Value)
		}
		if mp.Value != math.Trunc(mp.Value) {
			t.Errorf("threshold %v is not a round number", mp.Value)
		}
	}
	// The default is Sturges' formula and produces something usable.
	if got := NewContours().Size(10, 10).Compute(values); len(got) == 0 {
		t.Error("the default thresholds produced no contours")
	}
}

func TestInvalidConfigurationPanics(t *testing.T) {
	mustPanic := func(name string, f func()) {
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic", name)
			}
		}()
		f()
	}
	mustPanic("Size(0, 1)", func() { NewContours().Size(0, 1) })
	mustPanic("Size(1, -2)", func() { NewContours().Size(1, -2) })
	mustPanic("wrong values length", func() { NewContours().Size(3, 3).Compute([]float64{1, 2}) })
	mustPanic("NaN threshold", func() { NewContours().Size(1, 1).ComputeOne([]float64{1}, math.NaN()) })
}

func TestMissingValuesAreBelowEveryThreshold(t *testing.T) {
	// NaN is how a grid says "no data". It must never be treated as high, or a
	// hole in the data would read as a peak.
	values := []float64{5, 5, 5, math.NaN(), 5, 5, 5, 5, 5}
	got := NewContours().Size(3, 3).ComputeOne(values, 1)
	if len(got.Coordinates) != 1 {
		t.Fatalf("got %d polygons, want 1", len(got.Coordinates))
	}
	// The NaN cell is excluded, so the region is smaller than the whole grid.
	total := 0.0
	for _, ring := range got.Coordinates[0] {
		total += area(ring)
	}
	if total >= 8.5 {
		t.Errorf("area = %v, want less than the whole grid's 8.5", total)
	}
}

func TestSingleCellGrid(t *testing.T) {
	got := NewContours().Size(1, 1).ComputeOne([]float64{1}, 0.5)
	if len(got.Coordinates) != 1 {
		t.Fatalf("got %v, want one polygon", got.Coordinates)
	}
	ring := got.Coordinates[0][0]
	// A lone cell contours to the diamond around its center.
	if a := math.Abs(area(ring)); math.Abs(a-0.5) > 1e-12 {
		t.Errorf("area = %v, want 0.5", a)
	}
	if got := NewContours().Size(1, 1).ComputeOne([]float64{1}, 2); len(got.Coordinates) != 0 {
		t.Errorf("below threshold, got %v", got.Coordinates)
	}
}

func TestTwoSeparateBlobsAreTwoPolygons(t *testing.T) {
	// Two disjoint hot cells at opposite corners of a 5×5 grid.
	values := make([]float64, 25)
	values[0] = 1
	values[24] = 1
	got := NewContours().Size(5, 5).ComputeOne(values, 0.5)
	if len(got.Coordinates) != 2 {
		t.Fatalf("got %d polygons, want 2: %v", len(got.Coordinates), got.Coordinates)
	}
	for i, poly := range got.Coordinates {
		if len(poly) != 1 {
			t.Errorf("polygon %d has %d rings, want 1", i, len(poly))
		}
		if a := math.Abs(area(poly[0])); math.Abs(a-0.5) > 1e-12 {
			t.Errorf("polygon %d area = %v, want 0.5", i, a)
		}
	}
}

func BenchmarkContours(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	const n = 200
	values := make([]float64, n*n)
	for i := range values {
		values[i] = rng.NormFloat64()
	}
	c := NewContours().Size(n, n).ThresholdCount(10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Compute(values)
	}
}
