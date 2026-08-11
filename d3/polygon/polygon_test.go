package polygon

import (
	"math"
	"testing"
)

func closeTo(t *testing.T, got, want float64, what string) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// The two windings of the unit square. Both are exactly derivable, and between
// them they pin the sign convention documented on the package.
var (
	// Top-left, bottom-left, bottom-right, top-right: counter-clockwise when
	// y points down, which is what SVG does.
	squareCCW = Polygon{{0, 0}, {0, 1}, {1, 1}, {1, 0}}
	// The same ring the other way round.
	squareCW = Polygon{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
)

func TestAreaUnitSquareIsSignedByWinding(t *testing.T) {
	closeTo(t, squareCCW.Area(), 1, "counter-clockwise unit square area")
	closeTo(t, squareCW.Area(), -1, "clockwise unit square area")
}

func TestAreaTriangleMatchesShoelace(t *testing.T) {
	// A right triangle with legs 3 and 4: area 6 by (1/2)·base·height, and the
	// shoelace formula must agree.
	tri := Polygon{{0, 0}, {0, 3}, {4, 3}}
	closeTo(t, math.Abs(tri.Area()), 6, "3-4-5 triangle area")

	// An arbitrary triangle, computed independently by the shoelace formula.
	p := Polygon{{2, 1}, {8, 3}, {5, 9}}
	want := 0.5 * math.Abs(
		p[0].X*(p[1].Y-p[2].Y)+p[1].X*(p[2].Y-p[0].Y)+p[2].X*(p[0].Y-p[1].Y))
	closeTo(t, math.Abs(p.Area()), want, "arbitrary triangle area")
	closeTo(t, want, 21, "shoelace of the arbitrary triangle")
}

func TestAreaOfDegeneratePolygons(t *testing.T) {
	for _, p := range []Polygon{nil, {{1, 1}}, {{0, 0}, {1, 1}}} {
		if got := p.Area(); got != 0 {
			t.Errorf("Area(%v) = %v, want 0", p, got)
		}
	}
	// Collinear points enclose nothing.
	closeTo(t, Polygon{{0, 0}, {1, 1}, {2, 2}, {3, 3}}.Area(), 0, "collinear area")
}

func TestCentroid(t *testing.T) {
	for name, p := range map[string]Polygon{"ccw": squareCCW, "cw": squareCW} {
		c := p.Centroid()
		closeTo(t, c.X, 0.5, name+" unit square centroid x")
		closeTo(t, c.Y, 0.5, name+" unit square centroid y")
	}

	// A triangle's area centroid is the mean of its vertices — the one case
	// where the two definitions coincide, so it is exactly checkable.
	tri := Polygon{{0, 0}, {6, 0}, {0, 9}}
	c := tri.Centroid()
	closeTo(t, c.X, 2, "triangle centroid x")
	closeTo(t, c.Y, 3, "triangle centroid y")

	// A square is not the mean of its vertices once they are unevenly spaced:
	// adding a midpoint vertex must not move the centroid.
	withMidpoint := Polygon{{0, 0}, {0, 1}, {1, 1}, {1, 0.5}, {1, 0}}
	c = withMidpoint.Centroid()
	closeTo(t, c.X, 0.5, "midpoint-split square centroid x")
	closeTo(t, c.Y, 0.5, "midpoint-split square centroid y")
}

func TestCentroidOfDegeneratePolygonIsNotFinite(t *testing.T) {
	c := Polygon{{0, 0}, {1, 1}, {2, 2}}.Centroid()
	if !math.IsNaN(c.X) && !math.IsInf(c.X, 0) {
		t.Errorf("collinear centroid x = %v, want NaN or Inf", c.X)
	}
	c = Polygon(nil).Centroid()
	if !math.IsNaN(c.X) || !math.IsNaN(c.Y) {
		t.Errorf("empty centroid = %v, want NaN", c)
	}
}

func TestLength(t *testing.T) {
	closeTo(t, squareCCW.Length(), 4, "unit square perimeter")
	closeTo(t, squareCW.Length(), 4, "unit square perimeter, other winding")
	// 3-4-5: the closing edge is the hypotenuse.
	closeTo(t, Polygon{{0, 0}, {0, 3}, {4, 3}}.Length(), 3+4+5, "3-4-5 perimeter")
	// The ring is closed, so a two-point polygon is traversed out and back.
	closeTo(t, Polygon{{0, 0}, {0, 7}}.Length(), 14, "segment perimeter")
	closeTo(t, Polygon{{1, 1}}.Length(), 0, "point perimeter")
}

func TestContains(t *testing.T) {
	inside := []Point{{0.5, 0.5}, {0.01, 0.99}, {0.999, 0.001}}
	outside := []Point{{-0.5, 0.5}, {1.5, 0.5}, {0.5, -0.5}, {0.5, 1.5}, {2, 2}}
	for _, sq := range []Polygon{squareCCW, squareCW} {
		for _, q := range inside {
			if !sq.Contains(q) {
				t.Errorf("Contains(%v) = false, want true", q)
			}
		}
		for _, q := range outside {
			if sq.Contains(q) {
				t.Errorf("Contains(%v) = true, want false", q)
			}
		}
	}

	// A concave polygon: the notch must not count as inside.
	l := Polygon{{0, 0}, {0, 3}, {3, 3}, {3, 2}, {1, 2}, {1, 0}}
	if !l.Contains(Point{0.5, 1}) {
		t.Error("point in the leg of the L should be inside")
	}
	if l.Contains(Point{2, 1}) {
		t.Error("point in the notch of the L should be outside")
	}

	if squareCCW[:2].Contains(Point{0, 0}) {
		t.Error("a degenerate polygon contains nothing")
	}
}

func TestHullOfSquareWithInteriorPoint(t *testing.T) {
	points := []Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0.5, 0.5}}
	h := Hull(points)
	if len(h) != 4 {
		t.Fatalf("Hull returned %d points (%v), want the 4 corners", len(h), h)
	}
	for _, corner := range points[:4] {
		found := false
		for _, p := range h {
			if p == corner {
				found = true
			}
		}
		if !found {
			t.Errorf("corner %v missing from hull %v", corner, h)
		}
	}
	for _, p := range h {
		if p == (Point{0.5, 0.5}) {
			t.Errorf("interior point survived into hull %v", h)
		}
	}
	// The hull of the unit square is the unit square, whichever order its
	// vertices come out in, and it is wound counter-clockwise on screen.
	closeTo(t, h.Area(), 1, "hull area")
	closeTo(t, h.Length(), 4, "hull perimeter")
}

func TestHullDropsPointsOnAnEdge(t *testing.T) {
	// (0.5, 0) lies on the top edge but is not a vertex of the hull.
	h := Hull([]Point{{0, 0}, {0.5, 0}, {1, 0}, {1, 1}, {0, 1}})
	if len(h) != 4 {
		t.Fatalf("Hull = %v, want 4 corners", h)
	}
	closeTo(t, h.Area(), 1, "hull area")
}

func TestHullNeedsThreePoints(t *testing.T) {
	for _, in := range [][]Point{nil, {{0, 0}}, {{0, 0}, {1, 1}}} {
		if h := Hull(in); h != nil {
			t.Errorf("Hull(%v) = %v, want nil", in, h)
		}
	}
}

func TestHullProperties(t *testing.T) {
	// A deterministic pseudo-random cloud; no fixture values are asserted,
	// only invariants that must hold for any convex hull.
	var points []Point
	seed := uint64(0x2545F4914F6CDD1D)
	next := func() float64 {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		return float64(seed%10001) / 10000
	}
	for i := 0; i < 200; i++ {
		points = append(points, Point{next() * 100, next() * 100})
	}

	h := Hull(points)
	if len(h) < 3 {
		t.Fatalf("hull of 200 points has only %d vertices", len(h))
	}

	// 1. Counter-clockwise on screen, so the signed area is positive.
	if h.Area() <= 0 {
		t.Errorf("hull area = %v, want positive (counter-clockwise winding)", h.Area())
	}

	// 2. Convex: every consecutive triple turns the same way. The sign is the
	//    same fact as the winding convention seen from the other side — the
	//    ring is counter-clockwise on screen, so the *standard* (y-up) cross
	//    product is consistently negative, exactly as the area is consistently
	//    positive.
	for i := range h {
		o, a, b := h[i], h[(i+1)%len(h)], h[(i+2)%len(h)]
		if c := cross(o, a, b); c > 1e-9 {
			t.Errorf("hull is not convex at vertex %d: cross = %v", i, c)
		}
	}

	// 3. Every input point is inside the hull or on its boundary.
	for _, p := range points {
		if h.Contains(p) {
			continue
		}
		onBoundary := false
		for i := range h {
			a, b := h[i], h[(i+1)%len(h)]
			if math.Abs(cross(a, b, p)) < 1e-6 {
				onBoundary = true
				break
			}
		}
		if !onBoundary {
			t.Errorf("input point %v lies outside its own hull", p)
		}
	}

	// 4. The hull is a subset of the input.
	for _, hp := range h {
		found := false
		for _, p := range points {
			if p == hp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("hull vertex %v is not one of the input points", hp)
		}
	}

	// 5. The hull encloses at least as much as any triangle of input points.
	tri := Polygon{points[0], points[1], points[2]}
	if math.Abs(h.Area()) < math.Abs(tri.Area())-1e-9 {
		t.Errorf("hull area %v is smaller than an inscribed triangle's %v", h.Area(), tri.Area())
	}

	// 6. Hull is idempotent.
	again := Hull(h)
	if len(again) != len(h) {
		t.Errorf("Hull(Hull(p)) has %d vertices, want %d", len(again), len(h))
	}
	closeTo(t, again.Area(), h.Area(), "Hull(Hull(p)) area")
}

func TestHullDoesNotModifyInput(t *testing.T) {
	points := []Point{{3, 1}, {0, 0}, {1, 5}, {2, 2}}
	before := append([]Point(nil), points...)
	Hull(points)
	for i := range points {
		if points[i] != before[i] {
			t.Fatalf("Hull modified its input: %v, want %v", points, before)
		}
	}
}

func TestHullOfCoincidentPoints(t *testing.T) {
	// Degenerate but must not panic, and must enclose nothing.
	h := Hull([]Point{{1, 1}, {1, 1}, {1, 1}, {1, 1}})
	closeTo(t, h.Area(), 0, "hull of coincident points")
}
