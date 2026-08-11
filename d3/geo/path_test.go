package geo

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// A one-degree square under an unresampled equirectangular projection is an
// exact rectangle of side scale·(π/180), which makes every path measurement
// below arithmetic rather than a fixture.
func unitSquare() (*Polygon, *Path, float64) {
	const scale = 100
	side := scale * radians
	p := NewPath().Projection(NewEquirectangular().Scale(scale).Translate(0, 0).Precision(0))
	return square(0, 0, 1, 1), p, side
}

func TestPathGenerateExactRectangle(t *testing.T) {
	sq, p, side := unitSquare()
	// (0, 0) is the origin, and latitude increases upward while SVG's y
	// increases downward, so the square occupies x ∈ [0, side], y ∈ [-side, 0].
	// Path numbers are shortest-round-trip decimals, the same formatting
	// d3/path uses, so the expectation is built the same way.
	n := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	want := "M0,0L0," + n(-side) + "L" + n(side) + "," + n(-side) + "L" + n(side) + ",0Z"
	if got := p.Generate(sq); got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
}

func TestPathMeasurements(t *testing.T) {
	sq, p, side := unitSquare()

	near(t, p.Area(sq), side*side, 1e-9, "area of the projected square")
	near(t, p.Measure(sq), 4*side, 1e-9, "perimeter of the projected square")

	x0, y0, x1, y1 := p.Bounds(sq)
	near(t, x0, 0, 1e-9, "bounds x0")
	near(t, y0, -side, 1e-9, "bounds y0")
	near(t, x1, side, 1e-9, "bounds x1")
	near(t, y1, 0, 1e-9, "bounds y1")

	cx, cy := p.Centroid(sq)
	near(t, cx, side/2, 1e-9, "centroid x")
	near(t, cy, -side/2, 1e-9, "centroid y")
}

func TestPathAreaScalesQuadratically(t *testing.T) {
	sq := square(0, 0, 1, 1)
	a := NewPath().Projection(NewEquirectangular().Scale(100).Precision(0)).Area(sq)
	b := NewPath().Projection(NewEquirectangular().Scale(300).Precision(0)).Area(sq)
	near(t, b/a, 9, 1e-9, "area ratio when scale triples")
}

func TestPathAreaIsUnsignedAndIgnoresHoles(t *testing.T) {
	// A hole subtracts (the rings of one polygon share a signed accumulator),
	// but the polygon as a whole reports a positive area whichever way it is
	// wound. Both halves of that are easy to get backwards.
	withHole := &Polygon{Coordinates: [][]Position{
		{P(0, 0), P(0, 4), P(4, 4), P(4, 0), P(0, 0)},
		{P(1, 1), P(2, 1), P(2, 2), P(1, 2), P(1, 1)},
	}}
	solid := square(0, 0, 4, 4)
	hole := square(1, 1, 2, 2)
	// Measured through Identity, so this is purely about the planar shoelace
	// sum: a spherical projection would first reinterpret a reversed ring as the
	// complement of the globe, which is a different question.
	p := NewPath().Projection(NewIdentity())
	near(t, p.Area(withHole), p.Area(solid)-p.Area(hole), 1e-9,
		"a hole subtracts from its polygon")

	// And reversing a ring does not change its area.
	rev := &Polygon{Coordinates: [][]Position{{P(0, 0), P(4, 0), P(4, 4), P(0, 4), P(0, 0)}}}
	near(t, p.Area(rev), p.Area(solid), 1e-9, "area is unsigned")
}

func TestPathMeasureOnLines(t *testing.T) {
	p := NewPath().Projection(NewEquirectangular().Scale(100).Precision(0))
	line := &LineString{Coordinates: []Position{P(0, 0), P(1, 0), P(1, 1)}}
	near(t, p.Measure(line), 2*100*radians, 1e-9, "length of an L-shaped line")
	// An open line is not closed, so its measure is 2 sides, not 3.
	ring := square(0, 0, 1, 1)
	near(t, p.Measure(ring), 4*100*radians, 1e-9, "a ring's perimeter closes")
}

func TestPathCentroidFallsBackByDimension(t *testing.T) {
	p := NewPath().Projection(NewEquirectangular().Scale(100).Translate(0, 0).Precision(0))

	// Points: the arithmetic mean.
	x, y := p.Centroid(&MultiPoint{Coordinates: []Position{P(0, 0), P(2, 0)}})
	near(t, x, 100*radians, 1e-9, "point-mean x")
	near(t, y, 0, 1e-9, "point-mean y")

	// Lines: length-weighted, so a long segment outvotes a short one.
	x, _ = p.Centroid(&LineString{Coordinates: []Position{P(0, 0), P(4, 0)}})
	near(t, x, 2*100*radians, 1e-9, "line centroid x")

	// Polygons: area-weighted.
	x, y = p.Centroid(square(0, 0, 2, 2))
	near(t, x, 100*radians, 1e-9, "polygon centroid x")
	near(t, y, -100*radians, 1e-9, "polygon centroid y")
}

func TestPathCentroidUndefined(t *testing.T) {
	x, y := NewPath().Centroid(&MultiPoint{})
	if !math.IsNaN(x) || !math.IsNaN(y) {
		t.Errorf("empty centroid = (%v, %v), want NaN", x, y)
	}
}

func TestPathBoundsOfNothing(t *testing.T) {
	x0, y0, x1, y1 := NewPath().Bounds(&MultiPoint{})
	if !math.IsInf(x0, 1) || !math.IsInf(y0, 1) || !math.IsInf(x1, -1) || !math.IsInf(y1, -1) {
		t.Errorf("empty bounds = %v %v %v %v, want the empty fold", x0, y0, x1, y1)
	}
}

func TestPathBoundsContainEveryProjectedPoint(t *testing.T) {
	// The property: nothing the path draws may sit outside the bounds it
	// reports. Resampling is on, so this also covers the inserted points.
	for _, tc := range allProjections {
		p := NewPath().Projection(tc.new())
		obj := &MultiPolygon{Coordinates: [][][]Position{
			square(-20, -20, 20, 20).Coordinates,
			square(30, 10, 45, 40).Coordinates,
		}}
		x0, y0, x1, y1 := p.Bounds(obj)
		d := p.Generate(obj)
		for _, c := range coordsOf(t, d) {
			if c[0] < x0-1e-6 || c[0] > x1+1e-6 || c[1] < y0-1e-6 || c[1] > y1+1e-6 {
				t.Errorf("%s: drawn point %v is outside the reported bounds [%v %v %v %v]",
					tc.name, c, x0, y0, x1, y1)
				break
			}
		}
	}
}

func TestPathWithoutAProjection(t *testing.T) {
	// Coordinates pass through unchanged, which is what already-projected data
	// needs.
	d := NewPath().Generate(&LineString{Coordinates: []Position{P(0, 0), P(10, 20)}})
	if d != "M0,0L10,20" {
		t.Errorf("got %s, want M0,0L10,20", d)
	}
}

func TestPathPointRadius(t *testing.T) {
	// A standalone point is a circle: one move to (x+r, y) and a full arc.
	d := NewPath().PointRadius(3).Generate(&Point{Coordinates: P(10, 20)})
	if !strings.HasPrefix(d, "M13,20A3,3") {
		t.Errorf("got %s, want a circle of radius 3 at (10, 20)", d)
	}
	// The bounding box of that circle is the point, not the circle: the radius
	// is a drawing decision, not geometry.
	x0, y0, x1, y1 := NewPath().PointRadius(3).Bounds(&Point{Coordinates: P(10, 20)})
	if x0 != 10 || y0 != 20 || x1 != 10 || y1 != 20 {
		t.Errorf("point bounds = %v %v %v %v, want the point itself", x0, y0, x1, y1)
	}
}

func TestPathDigitsShortensOutput(t *testing.T) {
	obj := square(0, 0, 1, 1)
	full := NewPath().Projection(NewEquirectangular().Precision(0)).Generate(obj)
	short := NewPath().Projection(NewEquirectangular().Precision(0)).Digits(1).Generate(obj)
	if len(short) >= len(full) {
		t.Errorf("Digits(1) did not shorten the path: %d vs %d", len(short), len(full))
	}
	// The shape is still there, to within the rounding.
	fp := NewPath().Projection(NewEquirectangular().Precision(0))
	sp := NewPath().Projection(NewEquirectangular().Precision(0)).Digits(1)
	fx0, fy0, fx1, fy1 := fp.Bounds(obj)
	sx0, sy0, sx1, sy1 := sp.Bounds(obj)
	near(t, sx0, fx0, 0.1, "rounded x0")
	near(t, sy0, fy0, 0.1, "rounded y0")
	near(t, sx1, fx1, 0.1, "rounded x1")
	near(t, sy1, fy1, 0.1, "rounded y1")
}

func TestPathEmptyOutput(t *testing.T) {
	if got := NewPath().Generate(nil); got != "" {
		t.Errorf("nil object generated %q", got)
	}
	if got := NewPath().Generate(&MultiPoint{}); got != "" {
		t.Errorf("empty object generated %q", got)
	}
}

func TestPathIsReusableAndIndependent(t *testing.T) {
	// Each call must build a fresh path; a second call must not append to the
	// first.
	p := NewPath().Projection(NewEquirectangular().Precision(0))
	obj := square(0, 0, 1, 1)
	a := p.Generate(obj)
	b := p.Generate(obj)
	if a != b {
		t.Errorf("second call differs:\n%s\n%s", a, b)
	}
}

func TestPathGeometryCollectionAndFeatures(t *testing.T) {
	p := NewPath().Projection(NewEquirectangular().Scale(100).Translate(0, 0).Precision(0))
	inner := []Geometry{square(0, 0, 1, 1), square(2, 2, 3, 3)}
	gc := &GeometryCollection{Geometries: inner}
	want := p.Generate(inner[0]) + p.Generate(inner[1])
	if got := p.Generate(gc); got != want {
		t.Errorf("collection\n got %s\nwant %s", got, want)
	}

	fc := &FeatureCollection{Features: []*Feature{{Geometry: inner[0]}, {Geometry: inner[1]}}}
	if got := p.Generate(fc); got != want {
		t.Errorf("feature collection\n got %s\nwant %s", got, want)
	}
}

func TestPathThroughIdentity(t *testing.T) {
	// Identity is a Projector too, so Path composes with it.
	i := NewIdentity().Scale(2).Translate(1, 1)
	d := NewPath().Projection(i).Generate(&LineString{Coordinates: []Position{P(0, 0), P(5, 5)}})
	if d != "M1,1L11,11" {
		t.Errorf("got %s, want M1,1L11,11", d)
	}
}
