package geo

import (
	"math"
	"testing"
)

func TestBoundsDerivableValues(t *testing.T) {
	lon0, lat0, lon1, lat1 := Bounds(&Point{Coordinates: P(3, 4)})
	near(t, lon0, 3, 1e-12, "point lon0")
	near(t, lat0, 4, 1e-12, "point lat0")
	near(t, lon1, 3, 1e-12, "point lon1")
	near(t, lat1, 4, 1e-12, "point lat1")

	lon0, lat0, lon1, lat1 = Bounds(&MultiPoint{Coordinates: []Position{
		P(-10, -20), P(30, 5), P(0, 40),
	}})
	near(t, lon0, -10, 1e-12, "multipoint lon0")
	near(t, lat0, -20, 1e-12, "multipoint lat0")
	near(t, lon1, 30, 1e-12, "multipoint lon1")
	near(t, lat1, 40, 1e-12, "multipoint lat1")

	lon0, lat0, lon1, lat1 = Bounds(&Sphere{})
	if lon0 != -180 || lat0 != -90 || lon1 != 180 || lat1 != 90 {
		t.Errorf("Bounds(Sphere) = %v %v %v %v, want the whole globe", lon0, lat0, lon1, lat1)
	}
}

func TestBoundsEmpty(t *testing.T) {
	lon0, lat0, lon1, lat1 := Bounds(&MultiPoint{})
	if !math.IsNaN(lon0) || !math.IsNaN(lat0) || !math.IsNaN(lon1) || !math.IsNaN(lat1) {
		t.Errorf("Bounds(empty) = %v %v %v %v, want NaN", lon0, lat0, lon1, lat1)
	}
}

func TestBoundsContainsEveryPoint(t *testing.T) {
	// The property that matters: no input point may fall outside the box. The
	// longitude test has to account for a box that wraps.
	sets := [][]Position{
		{P(0, 0), P(10, 20), P(-5, -30)},
		{P(179, 10), P(-179, -10), P(175, 0)},
		{P(-170, 80), P(170, 85), P(0, 88)},
		{P(1, 1)},
	}
	for _, pts := range sets {
		lon0, lat0, lon1, lat1 := Bounds(&MultiPoint{Coordinates: pts})
		for _, p := range pts {
			if p.Lat() < lat0-1e-9 || p.Lat() > lat1+1e-9 {
				t.Errorf("latitude %v outside [%v, %v]", p.Lat(), lat0, lat1)
			}
			if !lonInRange(p.Lon(), lon0, lon1) {
				t.Errorf("longitude %v outside [%v, %v]", p.Lon(), lon0, lon1)
			}
		}
	}
}

func lonInRange(x, lo, hi float64) bool {
	if lo <= hi {
		return lo-1e-9 <= x && x <= hi+1e-9
	}
	return x >= lo-1e-9 || x <= hi+1e-9
}

func TestBoundsWrapsTheAntimeridian(t *testing.T) {
	// Two points either side of ±180° must give a *wrapping* box (lon0 > lon1)
	// spanning 20°, not the 340° box a min/max would produce.
	lon0, _, lon1, _ := Bounds(&MultiPoint{Coordinates: []Position{P(170, 0), P(-170, 0)}})
	near(t, lon0, 170, 1e-9, "wrapping lon0")
	near(t, lon1, -170, 1e-9, "wrapping lon1")
	if !(lon0 > lon1) {
		t.Errorf("expected a wrapping box, got [%v, %v]", lon0, lon1)
	}
}

func TestBoundsAccountsForGreatCircleBulge(t *testing.T) {
	// A segment between two points at the same latitude bulges poleward, so the
	// box must be taller than its endpoints. Endpoints at 45°N, 90° apart in
	// longitude, reach about 54.7°N in between.
	line := &LineString{Coordinates: []Position{P(-45, 45), P(45, 45)}}
	_, lat0, _, lat1 := Bounds(line)
	near(t, lat0, 45, 1e-9, "southern edge is an endpoint")
	if !(lat1 > 45.5) {
		t.Errorf("northern edge %v does not account for the great-circle bulge", lat1)
	}
	// And the bulge is real: sample the arc and check the box holds it.
	f, _ := Interpolate(-45, 45, 45, 45)
	for i := 0; i <= 20; i++ {
		_, lat := f(float64(i) / 20)
		if lat < lat0-1e-9 || lat > lat1+1e-9 {
			t.Fatalf("arc point at latitude %v escapes [%v, %v]", lat, lat0, lat1)
		}
	}
}

func TestBoundsSouthPolarPolygon(t *testing.T) {
	// A triangle whose vertices sit at 80°S, 120° apart, encloses the pole; the
	// box has to reach -90 even though no vertex does.
	tri := &Polygon{Coordinates: [][]Position{{
		P(-60, -80), P(60, -80), P(180, -80), P(-60, -80),
	}}}
	lon0, lat0, lon1, lat1 := Bounds(tri)
	near(t, lon0, -180, 1e-9, "polar lon0")
	near(t, lon1, 180, 1e-9, "polar lon1")
	near(t, lat0, -90, 1e-9, "polar lat0")
	near(t, lat1, -80, 1e-9, "polar lat1")
	if !Contains(tri, 0, -90) {
		t.Error("the test polygon does not actually enclose the south pole")
	}
}

func TestBoundsFeatureCollection(t *testing.T) {
	fc := &FeatureCollection{Features: []*Feature{
		{Geometry: &Point{Coordinates: P(0, 0)}},
		{Geometry: &Point{Coordinates: P(10, 10)}},
		{Geometry: nil},
	}}
	lon0, lat0, lon1, lat1 := Bounds(fc)
	near(t, lon0, 0, 1e-9, "fc lon0")
	near(t, lat0, 0, 1e-9, "fc lat0")
	near(t, lon1, 10, 1e-9, "fc lon1")
	near(t, lat1, 10, 1e-9, "fc lat1")
}
