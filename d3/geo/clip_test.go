package geo

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// subpaths splits SVG path data into its "M…Z" pieces and reports, for each,
// whether it is closed and the coordinates it visits.
func subpaths(d string) []string {
	if d == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(d, "M")[1:] {
		out = append(out, "M"+part)
	}
	return out
}

func coordsOf(t *testing.T, d string) [][2]float64 {
	t.Helper()
	var pts [][2]float64
	for _, tok := range strings.FieldsFunc(d, func(r rune) bool {
		return r == 'M' || r == 'L' || r == 'Z' || r == 'A'
	}) {
		parts := strings.Split(tok, ",")
		if len(parts) != 2 {
			continue
		}
		x, err1 := parseFloat(parts[0])
		y, err2 := parseFloat(parts[1])
		if err1 == nil && err2 == nil {
			pts = append(pts, [2]float64{x, y})
		}
	}
	return pts
}

func parseFloat(s string) (float64, error) { return strconv.ParseFloat(s, 64) }

// --- antimeridian clipping ---------------------------------------------------

func TestAntimeridianSplitsACrossingPolygon(t *testing.T) {
	// A 20°-wide box straddling ±180°. Drawn without clipping it would stretch
	// across the entire map in the wrong direction; clipped, it is two pieces.
	box := square(170, -10, 190, 10)
	p := NewPath().Projection(NewEquirectangular().Precision(0)).Digits(3)
	d := p.Generate(box)

	parts := subpaths(d)
	if len(parts) != 2 {
		t.Fatalf("expected 2 subpaths, got %d: %s", len(parts), d)
	}
	for i, part := range parts {
		if !strings.HasSuffix(part, "Z") {
			t.Errorf("subpath %d is not closed: %s", i, part)
		}
	}

	// Neither piece may span more than the width of the map, and each must hug
	// one edge of it.
	x0, _, x1, _ := p.Bounds(box)
	if x1-x0 > 960.1 {
		t.Errorf("clipped width %v exceeds the map", x1-x0)
	}
	left, right := false, false
	for _, c := range coordsOf(t, d) {
		if math.Abs(c[0]-0) < 1 {
			left = true
		}
		if math.Abs(c[0]-960) < 1 {
			right = true
		}
	}
	if !left || !right {
		t.Errorf("the two pieces should meet both edges of the map: left=%v right=%v", left, right)
	}
}

func TestAntimeridianLeavesOrdinaryGeometryAlone(t *testing.T) {
	// A polygon nowhere near ±180° must come out untouched: one subpath with
	// exactly its four corners.
	box := square(-10, -10, 10, 10)
	p := NewPath().Projection(NewEquirectangular().Precision(0)).Digits(4)
	d := p.Generate(box)
	if n := len(subpaths(d)); n != 1 {
		t.Fatalf("expected 1 subpath, got %d: %s", n, d)
	}
	if got := len(coordsOf(t, d)); got != 4 {
		t.Errorf("expected 4 points, got %d: %s", got, d)
	}
}

func TestAntimeridianClipsALine(t *testing.T) {
	// An open line crossing the antimeridian must become two open lines, not
	// one line running the wrong way round the world.
	line := &LineString{Coordinates: []Position{P(170, 0), P(-170, 0)}}
	d := NewPath().Projection(NewEquirectangular().Precision(0)).Digits(3).Generate(line)
	if n := len(subpaths(d)); n != 2 {
		t.Fatalf("expected 2 subpaths, got %d: %s", n, d)
	}
	if strings.Contains(d, "Z") {
		t.Errorf("an open line must not be closed: %s", d)
	}
}

func TestSphereDrawsTheOutlineOfTheMap(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    *Projection
	}{
		{"Equirectangular", NewEquirectangular()},
		{"Orthographic", NewOrthographic()},
		{"EqualEarth", NewEqualEarth()},
		{"AzimuthalEqualArea", NewAzimuthalEqualArea()},
	} {
		d := NewPath().Projection(tc.p).Generate(&Sphere{})
		if d == "" {
			t.Errorf("%s: Sphere drew nothing", tc.name)
			continue
		}
		if !strings.HasSuffix(d, "Z") {
			t.Errorf("%s: the sphere outline is not closed", tc.name)
		}
		if n := len(subpaths(d)); n != 1 {
			t.Errorf("%s: the sphere outline is %d subpaths, want 1", tc.name, n)
		}
	}
}

func TestOrthographicSphereIsTheProjectionDisc(t *testing.T) {
	// Every point of the outline must sit on the circle of radius = scale about
	// the translate point. This is exact geometry, not a fixture.
	p := NewOrthographic().Scale(100).Translate(0, 0)
	d := NewPath().Projection(p).Generate(&Sphere{})
	pts := coordsOf(t, d)
	if len(pts) < 30 {
		t.Fatalf("outline has only %d points", len(pts))
	}
	for _, c := range pts {
		near(t, math.Hypot(c[0], c[1]), 100, 1e-6, "outline radius at %v", c)
	}
}

// --- circle clipping ---------------------------------------------------------

func TestClipAngleRemovesTheFarSide(t *testing.T) {
	p := NewPath().Projection(NewOrthographic().Precision(0))
	behind := square(150, -10, 210, 10) // centered on the antipode
	if d := p.Generate(behind); d != "" {
		t.Errorf("geometry on the far side of the globe was drawn: %s", d)
	}
	front := square(-10, -10, 10, 10)
	if d := p.Generate(front); d == "" {
		t.Error("geometry facing the viewer was not drawn")
	}
}

func TestClipAngleKeepsGeometryInsideTheDisc(t *testing.T) {
	// Whatever survives clipping must lie within the projection's disc. A
	// polygon straddling the horizon is the case that exercises the hard path:
	// each edge is cut, and the pieces are rejoined along the horizon.
	p := NewOrthographic().Scale(100).Translate(0, 0).Precision(0.5)
	path := NewPath().Projection(p)
	for _, poly := range []*Polygon{
		square(60, -20, 120, 20),   // straddles the eastern horizon
		square(-120, -20, -60, 20), // straddles the western horizon
		square(-60, 60, 60, 100),   // over the pole and past the horizon
	} {
		d := path.Generate(poly)
		if d == "" {
			t.Errorf("straddling polygon %v drew nothing", poly.Coordinates[0][0])
			continue
		}
		for _, c := range coordsOf(t, d) {
			if r := math.Hypot(c[0], c[1]); r > 100+1e-6 {
				t.Errorf("clipped point %v is %v from the center, outside the disc", c, r)
			}
		}
		if !strings.HasSuffix(d, "Z") {
			t.Errorf("clipped polygon is not closed: %s", d)
		}
	}
}

func TestClipAngleProducesBalancedSubpaths(t *testing.T) {
	// Every subpath of a clipped polygon must be closed: an unbalanced M/Z is
	// exactly the "smeared across the screen" failure.
	p := NewPath().Projection(NewOrthographic().Rotate(-30, -20, 0))
	for _, o := range []Object{
		square(0, 0, 90, 60),
		square(-100, -60, 100, 60),
		&MultiPolygon{Coordinates: [][][]Position{
			square(0, 0, 40, 40).Coordinates,
			square(150, -20, 200, 20).Coordinates,
		}},
		&Sphere{},
	} {
		d := p.Generate(o)
		if d == "" {
			continue
		}
		if strings.Count(d, "M") != strings.Count(d, "Z") {
			t.Errorf("unbalanced subpaths (%d M, %d Z): %s",
				strings.Count(d, "M"), strings.Count(d, "Z"), d)
		}
	}
}

func TestClipAngleLargerThanAHemisphere(t *testing.T) {
	// A clip angle above 90° keeps the *complement* of a small circle, which
	// flips the sense of nearly every test in the circle clipper.
	p := NewPath().Projection(NewAzimuthalEqualArea().ClipAngle(150).Precision(0))
	if d := p.Generate(square(-10, -10, 10, 10)); d == "" {
		t.Error("geometry near the center was clipped away")
	}
	// A patch at the antipode is beyond 150° and must go.
	if d := p.Generate(square(175, -3, 185, 3)); d != "" {
		t.Errorf("geometry beyond the clip angle was drawn: %s", d)
	}
}

func TestClipAngleZeroRestoresAntimeridianClipping(t *testing.T) {
	withCircle := NewPath().Projection(NewEquirectangular().ClipAngle(60).Precision(0))
	restored := NewPath().Projection(NewEquirectangular().ClipAngle(60).ClipAngle(0).Precision(0))
	plain := NewPath().Projection(NewEquirectangular().Precision(0))
	box := square(-80, -20, 80, 20)
	if withCircle.Generate(box) == plain.Generate(box) {
		t.Error("a 60° clip angle had no effect on a 160°-wide box")
	}
	if restored.Generate(box) != plain.Generate(box) {
		t.Error("ClipAngle(0) did not restore the default antimeridian clipping")
	}
}

// --- rectangle clipping ------------------------------------------------------

func TestClipExtentKeepsOutputInTheViewport(t *testing.T) {
	const x0, y0, x1, y1 = 400.0, 200.0, 600.0, 300.0
	p := NewPath().Projection(NewEquirectangular().Precision(0).ClipExtent(x0, y0, x1, y1))
	for _, o := range []Object{
		square(-40, -40, 40, 40),
		&LineString{Coordinates: []Position{P(-60, -30), P(60, 30)}},
		&MultiPoint{Coordinates: []Position{P(0, 0), P(-90, 0), P(90, 0)}},
	} {
		d := p.Generate(o)
		for _, c := range coordsOf(t, d) {
			if c[0] < x0-1e-6 || c[0] > x1+1e-6 || c[1] < y0-1e-6 || c[1] > y1+1e-6 {
				t.Errorf("point %v escapes the clip extent: %s", c, d)
			}
		}
	}
}

func TestClipExtentExactRectangle(t *testing.T) {
	// A polygon that covers the whole viewport must come out as the viewport
	// itself — four corners, closed — and that rectangle is exactly the extent.
	p := NewPath().Projection(NewEquirectangular().Precision(0).ClipExtent(400, 200, 600, 300)).Digits(4)
	// The box must be well under 180° wide: two vertices 180° apart in longitude
	// are antipodal, and the great circle between them is not unique.
	cover := square(-80, -50, 80, 50)
	d := p.Generate(cover)
	x0, y0, x1, y1 := p.Bounds(cover)
	near(t, x0, 400, 1e-6, "clipped x0")
	near(t, y0, 200, 1e-6, "clipped y0")
	near(t, x1, 600, 1e-6, "clipped x1")
	near(t, y1, 300, 1e-6, "clipped y1")
	if !strings.HasSuffix(d, "Z") {
		t.Errorf("clipped polygon is not closed: %s", d)
	}
}

func TestClipExtentRemovesGeometryEntirely(t *testing.T) {
	p := NewPath().Projection(NewEquirectangular().Precision(0).ClipExtent(0, 0, 10, 10))
	if d := p.Generate(square(-10, -10, 10, 10)); d != "" {
		t.Errorf("geometry outside the viewport was drawn: %s", d)
	}
}

func TestClearClipExtent(t *testing.T) {
	plain := NewPath().Projection(NewEquirectangular().Precision(0))
	cleared := NewPath().Projection(
		NewEquirectangular().Precision(0).ClipExtent(400, 200, 600, 300).ClearClipExtent())
	box := square(-40, -40, 40, 40)
	if cleared.Generate(box) != plain.Generate(box) {
		t.Error("ClearClipExtent did not restore unclipped output")
	}
}

func TestMercatorClipsTheRepeatingWorld(t *testing.T) {
	// Mercator is periodic in longitude, so it carries a built-in viewport clip
	// of width 2π·scale centered on the projection's center. The sphere must
	// come out exactly that wide.
	p := NewMercator().Scale(100).Translate(0, 0)
	x0, _, x1, _ := NewPath().Projection(p).Bounds(&Sphere{})
	near(t, x1-x0, 2*pi*100, 1e-6, "mercator world width")
}

func TestMercatorClipExtentComposes(t *testing.T) {
	// A user clip extent must intersect with the built-in one rather than
	// replace it, so the world still cannot repeat.
	p := NewMercator().Scale(100).Translate(0, 0).ClipExtent(-1000, -50, 1000, 50)
	x0, y0, x1, y1 := NewPath().Projection(p).Bounds(&Sphere{})
	near(t, x1-x0, 2*pi*100, 1e-6, "width still bounded by the periodic clip")
	near(t, y0, -50, 1e-6, "top from the user extent")
	near(t, y1, 50, 1e-6, "bottom from the user extent")
}

func TestTransverseMercatorAxes(t *testing.T) {
	// The defining property: the central meridian projects to a straight
	// vertical line, where in an ordinary Mercator it is a *parallel* that is
	// straight and horizontal.
	p := NewTransverseMercator().Scale(100).Translate(0, 0)
	var x0 float64
	for i, lat := range []float64{-60, -30, 0, 30, 60} {
		x, _ := p.Project(0, lat)
		if i == 0 {
			x0 = x
			continue
		}
		near(t, x, x0, 1e-9, "central meridian x at %v°", lat)
	}
	// And y increases southward along it.
	_, yn := p.Project(0, 30)
	_, ys := p.Project(0, -30)
	if !(yn < ys) {
		t.Errorf("central meridian is upside down: %v vs %v", yn, ys)
	}
	// It is still conformal.
	for _, c := range [][2]float64{{0, 0}, {5, 40}, {-5, -40}} {
		r := aspectAt(p, c[0], c[1])
		if r < 0.999 || r > 1.001 {
			t.Errorf("transverse Mercator aspect at %v = %v, want 1", c, r)
		}
	}
}

// --- circle generator --------------------------------------------------------

func TestCircleGeneratorIsEquidistantFromItsCenter(t *testing.T) {
	for _, c := range [][3]float64{{0, 0, 30}, {-75, 40, 20}, {0, 90, 45}, {170, -60, 10}} {
		poly := NewCircle().Center(c[0], c[1]).Radius(c[2]).Precision(2).Generate()
		ring := poly.Coordinates[0]
		if len(ring) < 10 {
			t.Fatalf("circle has only %d points", len(ring))
		}
		want := c[2] * radians
		for _, pt := range ring {
			near(t, Distance(c[0], c[1], pt.Lon(), pt.Lat()), want, 1e-9,
				"radius of the circle at %v", c)
		}
	}
}

func TestCircleAreaApproachesTheCap(t *testing.T) {
	// A polygon inscribed in a circle has slightly less area than the cap, and
	// the gap must shrink as the precision improves.
	const radius = 30.0
	capArea := tau * (1 - math.Cos(radius*radians))
	prev := 0.0
	for _, prec := range []float64{10, 5, 1, 0.25} {
		a := Area(NewCircle().Radius(radius).Precision(prec).Generate())
		if a > capArea+1e-12 {
			t.Errorf("inscribed polygon area %v exceeds the cap %v", a, capArea)
		}
		if !(a > prev) {
			t.Errorf("area did not improve at precision %v: %v after %v", prec, a, prev)
		}
		prev = a
	}
	near(t, prev, capArea, 1e-4, "area at the finest precision")
}

func TestCircleAtThePoleIsAParallel(t *testing.T) {
	// A 10° circle around the north pole is the 80°N parallel, exactly.
	poly := NewCircle().Center(0, 90).Radius(10).Precision(5).Generate()
	for _, pt := range poly.Coordinates[0] {
		near(t, pt.Lat(), 80, 1e-9, "latitude on a polar circle")
	}
}

// --- clipping conserves geometry ---------------------------------------------

func TestAntimeridianClipConservesArea(t *testing.T) {
	// The strongest check available without fixtures: cutting a box at ±180°
	// must not create or destroy area. The two pieces drawn from a single
	// straddling polygon must cover exactly what the two halves cover when
	// drawn separately.
	p := NewPath().Projection(NewEquirectangular().Scale(100).Precision(0))
	whole := square(170, -10, 190, 10)

	// The halves have to be cut at the latitude where the box's *great-circle*
	// edge actually crosses ±180°, which is above 10° because the arc bulges
	// poleward. Splitting at 10° would compare two different regions and prove
	// nothing.
	f, _ := Interpolate(170, 10, -170, 10)
	_, cut := f(0.5)
	east := &Polygon{Coordinates: [][]Position{{
		P(170, -10), P(170, 10), P(180, cut), P(180, -cut), P(170, -10),
	}}}
	west := &Polygon{Coordinates: [][]Position{{
		P(-180, -cut), P(-180, cut), P(-170, 10), P(-170, -10), P(-180, -cut),
	}}}
	near(t, p.Area(whole), p.Area(east)+p.Area(west), 1e-6,
		"area survives the antimeridian cut")
}

func TestCircleClipFillsTheVisibleDisc(t *testing.T) {
	// A polygon covering far more than a hemisphere, clipped to the visible
	// hemisphere, must fill the projection's disc exactly: πR².
	const scale = 100
	p := NewPath().Projection(NewOrthographic().Scale(scale).Translate(0, 0).Precision(0.1))
	big := NewCircle().Center(0, 0).Radius(120).Precision(0.5).Generate()
	got := p.Area(big)
	want := pi * scale * scale
	if r := got / want; r < 0.999 || r > 1.001 {
		t.Errorf("clipped area %v is %.3f× the disc area %v", got, r, want)
	}
	// And the same disc is what the sphere itself draws.
	near(t, p.Area(&Sphere{}), want, want*1e-3, "sphere fills the disc")
}

func TestCircleClipConservesAreaAcrossTheHorizon(t *testing.T) {
	// A band that straddles the horizon must project to the same area as the
	// visible part of it drawn on its own.
	p := NewPath().Projection(NewOrthographic().Scale(100).Translate(0, 0).Precision(0.02))
	straddling := square(60, -20, 120, 20)

	// As above, the visible part is bounded by the meridian at ±90° and by the
	// latitude at which the box's great-circle edges cross it.
	f, _ := Interpolate(60, 20, 120, 20)
	_, cut := f(0.5)
	visibleOnly := &Polygon{Coordinates: [][]Position{{
		P(60, -20), P(60, 20), P(90, cut), P(90, -cut), P(60, -20),
	}}}
	a, b := p.Area(straddling), p.Area(visibleOnly)
	if r := a / b; r < 0.999 || r > 1.001 {
		t.Errorf("clipped area %v differs from the visible part %v by %.3f%%", a, b, (r-1)*100)
	}
}

func TestClipExtentConservesArea(t *testing.T) {
	// Rectangle clipping is exact: a box clipped to a viewport it overlaps has
	// exactly the area of the intersection.
	p := NewPath().Projection(NewEquirectangular().Scale(100).Translate(0, 0).Precision(0).
		ClipExtent(0, -100, 100, 0))
	full := square(0, 0, 90, 90) // x ∈ [0, 157], y ∈ [-157, 0]
	near(t, p.Area(full), 100*100, 1e-6, "area of the clipped intersection")
}
