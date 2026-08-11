package geo

import "testing"

// square is a d3-wound (clockwise) ring covering [lon0, lon1] × [lat0, lat1].
func square(lon0, lat0, lon1, lat1 float64) *Polygon {
	return &Polygon{Coordinates: [][]Position{{
		P(lon0, lat0), P(lon0, lat1), P(lon1, lat1), P(lon1, lat0), P(lon0, lat0),
	}}}
}

func TestContainsPolygon(t *testing.T) {
	sq := square(0, 0, 10, 10)
	inside := [][2]float64{{5, 5}, {0.1, 0.1}, {9.9, 9.9}, {1, 8}}
	outside := [][2]float64{{-1, 5}, {11, 5}, {5, -1}, {5, 11}, {180, 0}, {0, 90}, {0, -90}}
	for _, p := range inside {
		if !Contains(sq, p[0], p[1]) {
			t.Errorf("Contains(square, %v) = false, want true", p)
		}
	}
	for _, p := range outside {
		if Contains(sq, p[0], p[1]) {
			t.Errorf("Contains(square, %v) = true, want false", p)
		}
	}
}

func TestContainsRespectsWinding(t *testing.T) {
	// Reversing the ring selects the complement, so exactly one of the two
	// contains any given point that is not on the boundary.
	fwd := square(0, 0, 10, 10)
	rev := &Polygon{Coordinates: [][]Position{{
		P(0, 0), P(10, 0), P(10, 10), P(0, 10), P(0, 0),
	}}}
	for _, p := range [][2]float64{{5, 5}, {-40, 20}, {170, -60}, {0, 90}} {
		if Contains(fwd, p[0], p[1]) == Contains(rev, p[0], p[1]) {
			t.Errorf("a ring and its reverse agree at %v; one must contain it", p)
		}
	}
}

func TestContainsAcrossTheAntimeridian(t *testing.T) {
	// A box from 170°E to 170°W: a planar test would call this the *other* 340°
	// of the world.
	sq := square(170, -10, 190, 10)
	for _, p := range [][2]float64{{175, 0}, {180, 0}, {-175, 0}, {170.1, 9}} {
		if !Contains(sq, p[0], p[1]) {
			t.Errorf("Contains(antimeridian box, %v) = false, want true", p)
		}
	}
	for _, p := range [][2]float64{{0, 0}, {160, 0}, {-160, 0}, {175, 20}} {
		if Contains(sq, p[0], p[1]) {
			t.Errorf("Contains(antimeridian box, %v) = true, want false", p)
		}
	}
}

func TestContainsPolarCap(t *testing.T) {
	// A ring at 80°N wound so that the pole is on the right.
	polarCap := &Polygon{Coordinates: [][]Position{{
		P(0, 80), P(-90, 80), P(180, 80), P(90, 80), P(0, 80),
	}}}
	if !Contains(polarCap, 0, 90) {
		t.Error("polar cap does not contain the north pole")
	}
	if !Contains(polarCap, 137, 88) {
		t.Error("polar cap does not contain a point near the pole")
	}
	if Contains(polarCap, 0, 0) {
		t.Error("polar cap contains the equator")
	}
	if Contains(polarCap, 0, -90) {
		t.Error("polar cap contains the south pole")
	}
}

func TestContainsHole(t *testing.T) {
	// Exterior clockwise, hole counterclockwise.
	withHole := &Polygon{Coordinates: [][]Position{
		{P(0, 0), P(0, 10), P(10, 10), P(10, 0), P(0, 0)},
		{P(4, 4), P(6, 4), P(6, 6), P(4, 6), P(4, 4)},
	}}
	if !Contains(withHole, 1, 1) {
		t.Error("point in the ring but outside the hole should be contained")
	}
	if Contains(withHole, 5, 5) {
		t.Error("point in the hole should not be contained")
	}
}

func TestContainsOtherGeometries(t *testing.T) {
	if !Contains(&Sphere{}, 123, -45) {
		t.Error("the sphere contains everything")
	}
	pt := &Point{Coordinates: P(3, 4)}
	if !Contains(pt, 3, 4) {
		t.Error("a point contains itself")
	}
	if Contains(pt, 3, 4.0001) {
		t.Error("a point contains only itself")
	}

	line := &LineString{Coordinates: []Position{P(0, 0), P(10, 0), P(10, 10)}}
	for _, p := range [][2]float64{{0, 0}, {10, 0}, {10, 10}, {5, 0}} {
		if !Contains(line, p[0], p[1]) {
			t.Errorf("line does not contain %v", p)
		}
	}
	if Contains(line, 5, 1) {
		t.Error("line contains a point off it")
	}

	mp := &MultiPolygon{Coordinates: [][][]Position{
		square(0, 0, 10, 10).Coordinates,
		square(20, 20, 30, 30).Coordinates,
	}}
	if !Contains(mp, 25, 25) {
		t.Error("multipolygon does not contain a point in its second part")
	}
	if Contains(mp, 15, 15) {
		t.Error("multipolygon contains a point between its parts")
	}

	gc := &GeometryCollection{Geometries: []Geometry{pt, square(0, 0, 10, 10)}}
	if !Contains(gc, 5, 5) || !Contains(gc, 3, 4) {
		t.Error("collection should contain what its members contain")
	}

	f := &Feature{Geometry: square(0, 0, 10, 10)}
	fc := &FeatureCollection{Features: []*Feature{f}}
	if !Contains(f, 5, 5) || !Contains(fc, 5, 5) {
		t.Error("feature and collection should delegate to the geometry")
	}
	if Contains(&Feature{}, 0, 0) {
		t.Error("an unlocated feature contains nothing")
	}
}

func TestContainsAgreesWithArea(t *testing.T) {
	// A polygon smaller than a hemisphere cannot contain both poles, and the one
	// it does contain is decided by the same winding that decides its area.
	small := square(0, 0, 10, 10)
	if Area(small) > 2*pi {
		t.Fatalf("test polygon is not small: %v", Area(small))
	}
	if Contains(small, 0, 90) || Contains(small, 0, -90) {
		t.Error("a small equatorial polygon should contain neither pole")
	}
}
