package geo

import (
	"fmt"
	"math"
	"testing"
)

// near asserts that got is within tol of want, naming the assertion.
func near(t *testing.T, got, want, tol float64, format string, args ...any) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %v, want %v (±%v)", fmt.Sprintf(format, args...), got, want, tol)
	}
}

// --- Distance ----------------------------------------------------------------

func TestDistanceDerivableValues(t *testing.T) {
	// Every one of these is exact by hand: the poles are half a turn apart, a
	// quarter of the equator is a right angle, antipodes are π, and a point is
	// zero from itself.
	cases := []struct {
		name                 string
		a0, a1, b0, b1, want float64
	}{
		{"poles", 0, -90, 0, 90, pi},
		{"poles, different longitudes", 137, -90, -22, 90, pi},
		{"quarter equator", 0, 0, 90, 0, halfPi},
		{"antipodes on the equator", 0, 0, 180, 0, pi},
		{"antipodes off the equator", 0, 30, 180, -30, pi},
		{"equator to north pole", 0, 0, 0, 90, halfPi},
		{"identical", 12, 34, 12, 34, 0},
		{"one degree along the equator", 0, 0, 1, 0, radians},
		{"one degree along a meridian", 0, 0, 0, 1, radians},
	}
	for _, c := range cases {
		got := Distance(c.a0, c.a1, c.b0, c.b1)
		if math.Abs(got-c.want) > 1e-12 {
			t.Errorf("Distance(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDistanceSymmetryAndTriangleInequality(t *testing.T) {
	pts := [][2]float64{{0, 0}, {90, 0}, {-30, 45}, {170, -80}, {-179, 12}, {45, 89}}
	for _, a := range pts {
		for _, b := range pts {
			d1 := Distance(a[0], a[1], b[0], b[1])
			d2 := Distance(b[0], b[1], a[0], a[1])
			if math.Abs(d1-d2) > 1e-12 {
				t.Errorf("asymmetric: %v vs %v", d1, d2)
			}
			if d1 < 0 || d1 > pi+1e-12 {
				t.Errorf("distance %v out of [0, π]", d1)
			}
			for _, c := range pts {
				ac := Distance(a[0], a[1], c[0], c[1])
				cb := Distance(c[0], c[1], b[0], b[1])
				if d1 > ac+cb+1e-9 {
					t.Errorf("triangle inequality violated: %v > %v + %v", d1, ac, cb)
				}
			}
		}
	}
}

// --- Length ------------------------------------------------------------------

func TestLengthDerivableValues(t *testing.T) {
	quarter := &LineString{Coordinates: []Position{P(0, 0), P(90, 0)}}
	near(t, Length(quarter), halfPi, 1e-12, "quarter equator")

	// Pole to pole and back down the other side is a full turn.
	meridian := &LineString{Coordinates: []Position{P(0, -90), P(0, 0), P(0, 90)}}
	near(t, Length(meridian), pi, 1e-12, "pole to pole")

	// A polygon's length is its perimeter, closing edge included. Four sides of
	// one degree each, and the two along the equator and the two along meridians
	// are all exactly one degree of arc.
	poly := &Polygon{Coordinates: [][]Position{{P(0, 0), P(0, 1), P(1, 1), P(1, 0), P(0, 0)}}}
	if got := Length(poly); got < 4*radians-1e-4 || got > 4*radians+1e-4 {
		t.Errorf("Length(Polygon) = %v, want about %v", got, 4*radians)
	}
	if got := Length(&Point{Coordinates: P(1, 2)}); got != 0 {
		t.Errorf("Length(Point) = %v, want 0", got)
	}
}

func TestLengthIsAdditive(t *testing.T) {
	// Splitting a line at an intermediate point must not change its length.
	whole := &LineString{Coordinates: []Position{P(-100, -20), P(140, 60)}}
	f, _ := Interpolate(-100, -20, 140, 60)
	mx, my := f(0.37)
	split := &LineString{Coordinates: []Position{P(-100, -20), P(mx, my), P(140, 60)}}
	near(t, Length(split), Length(whole), 1e-9, "split line")
}

// --- Area --------------------------------------------------------------------

func TestAreaDerivableValues(t *testing.T) {
	near(t, Area(&Sphere{}), 4*pi, 1e-12, "sphere")

	// A hemisphere is half the sphere, whichever way round the equator is walked.
	north := &Polygon{Coordinates: [][]Position{{P(0, 0), P(-90, 0), P(180, 0), P(90, 0), P(0, 0)}}}
	south := &Polygon{Coordinates: [][]Position{{P(0, 0), P(90, 0), P(180, 0), P(-90, 0), P(0, 0)}}}
	near(t, Area(north), 2*pi, 1e-9, "northern hemisphere")
	near(t, Area(south), 2*pi, 1e-9, "southern hemisphere")

	// One octant of the sphere: equator to pole, a quarter turn, back down.
	octant := &Polygon{Coordinates: [][]Position{{P(0, 0), P(0, 90), P(90, 0), P(0, 0)}}}
	near(t, Area(octant), pi/2, 1e-9, "octant")

	// A lune of 90° is a quarter of the sphere.
	lune := &Polygon{Coordinates: [][]Position{{P(0, 0), P(0, 90), P(90, 0), P(0, -90), P(0, 0)}}}
	near(t, Area(lune), pi, 1e-9, "quarter lune")

	// Only polygons have area.
	if got := Area(&LineString{Coordinates: []Position{P(0, 0), P(1, 1)}}); got != 0 {
		t.Errorf("Area(LineString) = %v, want 0", got)
	}
}

func TestAreaWindingGivesComplement(t *testing.T) {
	fwd := [][]Position{{P(0, 0), P(0, 10), P(10, 10), P(10, 0), P(0, 0)}}
	rev := [][]Position{{P(0, 0), P(10, 0), P(10, 10), P(0, 10), P(0, 0)}}
	a := Area(&Polygon{Coordinates: fwd})
	b := Area(&Polygon{Coordinates: rev})
	near(t, a+b, 4*pi, 1e-9, "a ring and its reverse partition the sphere")
	if !(a < b) {
		t.Errorf("clockwise ring should be the small one: %v vs %v", a, b)
	}
}

func TestAreaIsAdditiveAcrossParts(t *testing.T) {
	// Two disjoint squares as a MultiPolygon must have the sum of their areas.
	sq := func(lon float64) [][]Position {
		return [][]Position{{P(lon, 0), P(lon, 5), P(lon+5, 5), P(lon+5, 0), P(lon, 0)}}
	}
	one := Area(&Polygon{Coordinates: sq(0)})
	two := Area(&MultiPolygon{Coordinates: [][][]Position{sq(0), sq(20)}})
	near(t, two, 2*one, 1e-12, "two congruent squares")
}

func TestAreaGrowsWithSize(t *testing.T) {
	prev := 0.0
	for _, side := range []float64{1, 2, 5, 10, 20, 40} {
		a := Area(&Polygon{Coordinates: [][]Position{
			{P(0, 0), P(0, side), P(side, side), P(side, 0), P(0, 0)},
		}})
		if !(a > prev) {
			t.Fatalf("area not increasing at side %v: %v after %v", side, a, prev)
		}
		prev = a
	}
}

// --- Centroid ----------------------------------------------------------------

func TestCentroidDerivableValues(t *testing.T) {
	lon, lat := Centroid(&Point{Coordinates: P(30, 40)})
	near(t, lon, 30, 1e-9, "point centroid lon")
	near(t, lat, 40, 1e-9, "point centroid lat")

	// Two points symmetric about the prime meridian on the equator.
	lon, lat = Centroid(&MultiPoint{Coordinates: []Position{P(-10, 0), P(10, 0)}})
	near(t, lon, 0, 1e-9, "symmetric pair lon")
	near(t, lat, 0, 1e-9, "symmetric pair lat")

	// A polar cap's centroid is the pole. Longitude there is arbitrary, so only
	// the latitude is asserted.
	polarCap := &Polygon{Coordinates: [][]Position{{P(0, 80), P(-90, 80), P(180, 80), P(90, 80), P(0, 80)}}}
	_, lat = Centroid(polarCap)
	near(t, lat, 90, 1e-9, "polar cap centroid latitude")

	// The midpoint of a meridian segment.
	lon, lat = Centroid(&LineString{Coordinates: []Position{P(0, -20), P(0, 20)}})
	near(t, lon, 0, 1e-9, "meridian centroid lon")
	near(t, lat, 0, 1e-9, "meridian centroid lat")
}

func TestCentroidUndefined(t *testing.T) {
	// Antipodes have no centroid; so does an empty object.
	lon, lat := Centroid(&MultiPoint{Coordinates: []Position{P(0, 0), P(180, 0)}})
	if !math.IsNaN(lon) || !math.IsNaN(lat) {
		t.Errorf("antipodes centroid = (%v, %v), want NaN", lon, lat)
	}
	lon, lat = Centroid(&MultiPoint{})
	if !math.IsNaN(lon) || !math.IsNaN(lat) {
		t.Errorf("empty centroid = (%v, %v), want NaN", lon, lat)
	}
}

func TestCentroidLiesInsideItsPolygon(t *testing.T) {
	for _, poly := range []*Polygon{
		{Coordinates: [][]Position{{P(0, 0), P(0, 20), P(20, 20), P(20, 0), P(0, 0)}}},
		{Coordinates: [][]Position{{P(-100, -30), P(-100, -10), P(-60, -10), P(-60, -30), P(-100, -30)}}},
		{Coordinates: [][]Position{{P(170, -5), P(170, 5), P(-170, 5), P(-170, -5), P(170, -5)}}},
	} {
		lon, lat := Centroid(poly)
		if !Contains(poly, lon, lat) {
			t.Errorf("centroid (%v, %v) is outside its own convex polygon", lon, lat)
		}
	}
}

// --- Interpolate -------------------------------------------------------------

func TestInterpolateDerivableValues(t *testing.T) {
	f, d := Interpolate(0, 0, 90, 0)
	near(t, d, halfPi, 1e-12, "quarter equator distance")
	lon, lat := f(0.5)
	near(t, lon, 45, 1e-9, "equator midpoint lon")
	near(t, lat, 0, 1e-9, "equator midpoint lat")

	f, _ = Interpolate(0, 0, 0, 90)
	lon, lat = f(0.5)
	near(t, lon, 0, 1e-9, "meridian midpoint lon")
	near(t, lat, 45, 1e-9, "meridian midpoint lat")
}

func TestInterpolateEndpointsAndDegenerate(t *testing.T) {
	f, _ := Interpolate(-13, 27, 140, -8)
	lon, lat := f(0)
	near(t, lon, -13, 1e-9, "t=0 lon")
	near(t, lat, 27, 1e-9, "t=0 lat")
	lon, lat = f(1)
	near(t, lon, 140, 1e-9, "t=1 lon")
	near(t, lat, -8, 1e-9, "t=1 lat")

	// Two identical points: the distance is zero (up to the last bit of a
	// floating-point subtraction) and the interpolator is constant.
	f, d := Interpolate(5, 5, 5, 5)
	if d > 1e-12 {
		t.Errorf("degenerate distance = %v, want 0", d)
	}
	lon, lat = f(0.7)
	near(t, lon, 5, 1e-9, "degenerate lon")
	near(t, lat, 5, 1e-9, "degenerate lat")
}

func TestInterpolateStaysOnTheGreatCircle(t *testing.T) {
	// The defining property: an interpolated point splits the arc, so the two
	// pieces sum to the whole. A linear lon/lat blend would fail this badly.
	a0, a1, b0, b1 := -73.9, 40.7, 139.7, 35.7 // New York to Tokyo
	f, d := Interpolate(a0, a1, b0, b1)
	if math.Abs(d-Distance(a0, a1, b0, b1)) > 1e-12 {
		t.Fatalf("reported distance %v disagrees with Distance", d)
	}
	for _, tt := range []float64{0.1, 0.25, 0.5, 0.75, 0.9} {
		lon, lat := f(tt)
		lead := Distance(a0, a1, lon, lat)
		rest := Distance(lon, lat, b0, b1)
		near(t, lead+rest, d, 1e-9, "arc split at t=%v", tt)
		near(t, lead, tt*d, 1e-9, "uniform speed at t=%v", tt)
	}
	// And it really does go north of both endpoints, which is the whole point.
	lon, lat := f(0.5)
	_ = lon
	if lat <= 40.7 {
		t.Errorf("great-circle midpoint latitude %v should exceed both endpoints", lat)
	}
}
