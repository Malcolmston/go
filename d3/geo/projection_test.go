package geo

import (
	"math"
	"testing"
)

// allProjections is every projection this package ships, in its default
// configuration, together with a longitude/latitude window inside which it is
// well defined (a gnomonic projection is not defined on the far hemisphere).
var allProjections = []struct {
	name       string
	new        func() *Projection
	lonR, latR float64 // half-width of the safe sampling window, degrees
}{
	{"AzimuthalEqualArea", NewAzimuthalEqualArea, 170, 85},
	{"AzimuthalEquidistant", NewAzimuthalEquidistant, 170, 85},
	{"ConicConformal", NewConicConformal, 150, 80},
	{"ConicEqualArea", NewConicEqualArea, 150, 80},
	{"ConicEquidistant", NewConicEquidistant, 150, 80},
	{"Equirectangular", NewEquirectangular, 180, 90},
	{"Gnomonic", NewGnomonic, 55, 55},
	{"Mercator", NewMercator, 180, 80},
	{"TransverseMercator", NewTransverseMercator, 80, 80},
	{"Orthographic", NewOrthographic, 85, 85},
	{"Stereographic", NewStereographic, 140, 85},
	{"NaturalEarth1", NewNaturalEarth1, 180, 90},
	{"EqualEarth", NewEqualEarth, 180, 90},
}

// --- values derivable by hand ------------------------------------------------

func TestProjectionCentersTheOrigin(t *testing.T) {
	// With no rotation and no center offset, (0°, 0°) must land exactly on the
	// translate point. This is true of every projection here by construction —
	// recentering subtracts the projected center — and it is the one value that
	// is safe to assert exactly.
	for _, p := range allProjections {
		// The conic projections ship with a non-zero default center, so it is
		// reset here; that default is d3's and is tested separately.
		proj := p.new().Center(0, 0).Translate(300, 200)
		x, y := proj.Project(0, 0)
		near(t, x, 300, 1e-9, "%s x at the origin", p.name)
		near(t, y, 200, 1e-9, "%s y at the origin", p.name)
	}
}

func TestEquirectangularExactValues(t *testing.T) {
	// The equirectangular projection is longitude and latitude scaled, so every
	// value below is arithmetic, not a fixture.
	p := NewEquirectangular().Scale(100).Translate(0, 0)
	cases := []struct{ lon, lat, x, y float64 }{
		{0, 0, 0, 0},
		{90, 0, 100 * halfPi, 0},
		{-90, 0, -100 * halfPi, 0},
		{180, 0, 100 * pi, 0},
		{0, 90, 0, -100 * halfPi}, // y is negated: SVG's origin is top-left
		{0, -90, 0, 100 * halfPi},
		{1, 1, 100 * radians, -100 * radians},
	}
	for _, c := range cases {
		x, y := p.Project(c.lon, c.lat)
		near(t, x, c.x, 1e-9, "x at (%v, %v)", c.lon, c.lat)
		near(t, y, c.y, 1e-9, "y at (%v, %v)", c.lon, c.lat)
	}
}

func TestOrthographicExactValues(t *testing.T) {
	// x = R sin λ cos φ, y = -R sin φ. At the horizon the projected radius is
	// exactly the scale.
	p := NewOrthographic().Scale(100).Translate(0, 0)
	cases := []struct{ lon, lat, x, y float64 }{
		{0, 0, 0, 0},
		{90, 0, 100, 0},
		{-90, 0, -100, 0},
		{0, 90, 0, -100},
		{0, -90, 0, 100},
		{0, 30, 0, -50}, // sin 30° = 1/2
		{30, 0, 50, 0},
	}
	for _, c := range cases {
		x, y := p.Project(c.lon, c.lat)
		near(t, x, c.x, 1e-9, "x at (%v, %v)", c.lon, c.lat)
		near(t, y, c.y, 1e-9, "y at (%v, %v)", c.lon, c.lat)
	}
}

func TestMercatorExactValues(t *testing.T) {
	// x is longitude scaled; y is the inverse Gudermannian. At 45°N,
	// ln(tan(45° + 45°/2)) = ln(1 + √2), the one closed-form value worth pinning.
	p := NewMercator().Scale(100).Translate(0, 0)
	x, y := p.Project(90, 0)
	near(t, x, 100*halfPi, 1e-9, "mercator x at 90°E")
	near(t, y, 0, 1e-9, "mercator y at the equator")

	_, y = p.Project(0, 45)
	near(t, y, -100*math.Log(1+math.Sqrt2), 1e-9, "mercator y at 45°N")

	// And it is antisymmetric about the equator.
	_, yn := p.Project(0, 60)
	_, ys := p.Project(0, -60)
	near(t, yn, -ys, 1e-9, "mercator symmetry about the equator")
}

func TestTranslateAndScaleAreLinear(t *testing.T) {
	base := NewEquirectangular().Scale(100).Translate(0, 0)
	x0, y0 := base.Project(20, 30)

	moved := NewEquirectangular().Scale(100).Translate(7, -3)
	x1, y1 := moved.Project(20, 30)
	near(t, x1, x0+7, 1e-9, "translate x")
	near(t, y1, y0-3, 1e-9, "translate y")

	bigger := NewEquirectangular().Scale(300).Translate(0, 0)
	x2, y2 := bigger.Project(20, 30)
	near(t, x2, 3*x0, 1e-9, "scale x")
	near(t, y2, 3*y0, 1e-9, "scale y")
}

func TestCenterMovesThePointToTheTranslate(t *testing.T) {
	for _, p := range allProjections {
		if p.name == "TransverseMercator" {
			// Center is applied *after* rotation, and a transverse Mercator has
			// a built-in 90° roll, so its center is not the geographic point.
			// See TestTransverseMercatorAxes.
			continue
		}
		proj := p.new().Translate(0, 0).Center(20, 30)
		x, y := proj.Project(20, 30)
		near(t, x, 0, 1e-6, "%s x at its center", p.name)
		near(t, y, 0, 1e-6, "%s y at its center", p.name)
	}
}

func TestRotateMovesThePointToTheOrigin(t *testing.T) {
	for _, p := range allProjections {
		proj := p.new().Center(0, 0).Translate(0, 0).Rotate(-20, -30, 0)
		x, y := proj.Project(20, 30)
		near(t, x, 0, 1e-6, "%s x after rotating to it", p.name)
		near(t, y, 0, 1e-6, "%s y after rotating to it", p.name)
	}
}

// --- properties --------------------------------------------------------------

func TestProjectionInvertRoundTrips(t *testing.T) {
	for _, p := range allProjections {
		proj := p.new()
		for lon := -p.lonR; lon <= p.lonR; lon += p.lonR / 4 {
			for lat := -p.latR; lat <= p.latR; lat += p.latR / 4 {
				x, y := proj.Project(lon, lat)
				back0, back1, ok := proj.Invert(x, y)
				if !ok {
					t.Fatalf("%s reports no inverse", p.name)
				}
				if math.IsNaN(back0) || math.IsNaN(back1) {
					t.Errorf("%s: invert(%v, %v) = NaN for (%v, %v)", p.name, x, y, lon, lat)
					continue
				}
				if d := Distance(lon, lat, back0, back1); d > 1e-6 {
					t.Errorf("%s: (%v, %v) round-tripped to (%v, %v), %v rad away",
						p.name, lon, lat, back0, back1, d)
				}
			}
		}
	}
}

func TestProjectionInvertRoundTripsAfterConfiguration(t *testing.T) {
	// The inverse has to survive scale, translate, rotation, plane rotation and
	// reflection, because those are composed into the same transform.
	for _, p := range allProjections {
		proj := p.new().Scale(321).Translate(-40, 90).Rotate(-11, 7, 13).Angle(20).ReflectY(true)
		for _, q := range [][2]float64{{0, 0}, {10, 20}, {-30, -10}, {40, 50}} {
			x, y := proj.Project(q[0], q[1])
			b0, b1, ok := proj.Invert(x, y)
			if !ok {
				t.Fatalf("%s reports no inverse", p.name)
			}
			if d := Distance(q[0], q[1], b0, b1); d > 1e-6 {
				t.Errorf("%s: %v round-tripped %v rad away", p.name, q, d)
			}
		}
	}
}

func TestCylindricalProjectionsAreMonotone(t *testing.T) {
	// x must increase with longitude and y must decrease with latitude (SVG's y
	// points down). A projection that failed this would draw a mirrored map.
	for _, name := range []string{"Equirectangular", "Mercator", "EqualEarth", "NaturalEarth1"} {
		var proj *Projection
		for _, p := range allProjections {
			if p.name == name {
				proj = p.new()
			}
		}
		prevX := math.Inf(-1)
		for lon := -170.0; lon <= 170; lon += 10 {
			x, _ := proj.Project(lon, 0)
			if !(x > prevX) {
				t.Errorf("%s: x not increasing with longitude at %v", name, lon)
			}
			prevX = x
		}
		prevY := math.Inf(1)
		for lat := -80.0; lat <= 80; lat += 10 {
			_, y := proj.Project(0, lat)
			if !(y < prevY) {
				t.Errorf("%s: y not decreasing with latitude at %v", name, lat)
			}
			prevY = y
		}
	}
}

func TestEqualAreaProjectionsPreserveArea(t *testing.T) {
	// The claim these projections make, tested as a claim: two regions of equal
	// spherical area must project to equal planar areas, wherever they sit.
	//
	// Cells of equal spherical area are built by taking equal steps in sin(φ),
	// which is what makes a 10°-wide band near the pole as large as a much
	// taller one at the equator.
	cells := equalAreaCells()
	for _, tc := range []struct {
		name string
		new  func() *Projection
	}{
		{"AzimuthalEqualArea", NewAzimuthalEqualArea},
		{"ConicEqualArea", NewConicEqualArea},
		{"EqualEarth", NewEqualEarth},
	} {
		proj := tc.new().Precision(0.1)
		path := NewPath().Projection(proj)
		var first float64
		for i, c := range cells {
			a := path.Area(c)
			if i == 0 {
				first = a
				continue
			}
			if r := a / first; r < 0.98 || r > 1.02 {
				lon, lat := Centroid(c)
				t.Errorf("%s: cell at (%.0f, %.0f) has %.1f%% of the area of the first",
					tc.name, lon, lat, r*100)
			}
		}
	}
}

func TestNonEqualAreaProjectionDistortsArea(t *testing.T) {
	// The control for the test above: Mercator must *fail* it, or the test
	// above is proving nothing.
	cells := equalAreaCells()
	path := NewPath().Projection(NewMercator().Precision(0.1))
	base := path.Area(cells[0])
	worst := 1.0
	for _, c := range cells[1:] {
		if r := path.Area(c) / base; r > worst {
			worst = r
		}
	}
	if worst < 2 {
		t.Errorf("Mercator inflated area by only %.2f×; the equal-area test is not discriminating", worst)
	}
}

// equalAreaCells returns spherical quadrilaterals of equal area at a range of
// latitudes. Equal steps in sin(φ) bound equal areas — Archimedes' hat-box
// theorem — so no projection is needed to know they match.
func equalAreaCells() []*Polygon {
	var out []*Polygon
	const dLon = 10.0
	for _, s := range []float64{0, 0.2, 0.4, 0.6, 0.8} {
		lat0 := math.Asin(s) * degrees
		lat1 := math.Asin(s+0.1) * degrees
		out = append(out, square(0, lat0, dLon, lat1))
	}
	return out
}

func TestConformalProjectionsPreserveAngles(t *testing.T) {
	// A conformal projection maps an infinitesimal circle to a circle. Sampled
	// with a tiny square, that means the two diagonals stay equal in length and
	// the sides stay perpendicular.
	for _, tc := range []struct {
		name string
		new  func() *Projection
	}{
		{"Mercator", NewMercator},
		{"Stereographic", NewStereographic},
		{"ConicConformal", NewConicConformal},
	} {
		proj := tc.new()
		for _, c := range [][2]float64{{0, 0}, {20, 30}, {-40, -20}, {10, 55}} {
			ratio := aspectAt(proj, c[0], c[1])
			if ratio < 0.999 || ratio > 1.001 {
				t.Errorf("%s at (%v, %v): local aspect ratio %v, want 1", tc.name, c[0], c[1], ratio)
			}
		}
	}
}

func TestNonConformalProjectionDistortsAngles(t *testing.T) {
	// The control: an equal-area projection is not conformal away from its
	// standard latitude, and must fail the aspect test somewhere.
	proj := NewAzimuthalEqualArea()
	worst := 1.0
	for _, c := range [][2]float64{{60, 0}, {0, 60}, {80, 40}} {
		r := aspectAt(proj, c[0], c[1])
		if d := math.Abs(r - 1); d > worst-1 {
			worst = 1 + d
		}
	}
	if worst < 1.05 {
		t.Errorf("AzimuthalEqualArea looks conformal (worst aspect %v); the conformality test is not discriminating", worst)
	}
}

// aspectAt measures the ratio of the projected width to the projected height of
// a small geographic cell centered at (lon, lat), corrected for the cos(lat)
// convergence of meridians. It is 1 exactly where the projection is conformal.
func aspectAt(p *Projection, lon, lat float64) float64 {
	const d = 0.01
	xw0, yw0 := p.Project(lon-d, lat)
	xw1, yw1 := p.Project(lon+d, lat)
	xh0, yh0 := p.Project(lon, lat-d)
	xh1, yh1 := p.Project(lon, lat+d)
	w := math.Hypot(xw1-xw0, yw1-yw0) / (2 * d * math.Cos(lat*radians))
	h := math.Hypot(xh1-xh0, yh1-yh0) / (2 * d)
	return w / h
}

// --- fitting -----------------------------------------------------------------

func TestFitExtentAndSize(t *testing.T) {
	// Resampling is off: an adaptive resampler emits different points at
	// different scales, so a fitted map measured again is a fraction of a pixel
	// larger than the box. That is upstream behavior and would only make this
	// test about the resampler.
	obj := square(-10, -10, 10, 10)
	for _, p := range allProjections {
		proj := p.new().Precision(0).FitExtent(20, 30, 220, 130, obj)
		x0, y0, x1, y1 := NewPath().Projection(proj).Bounds(obj)
		// The object is centered in the box and touches it on the tight axis.
		if x0 < 20-1e-6 || x1 > 220+1e-6 || y0 < 30-1e-6 || y1 > 130+1e-6 {
			t.Errorf("%s: fitted bounds [%v %v %v %v] escape the extent", p.name, x0, y0, x1, y1)
		}
		tight := math.Min(220-20-(x1-x0), 130-30-(y1-y0))
		if math.Abs(tight) > 1e-6 {
			t.Errorf("%s: fitted object touches neither pair of edges (slack %v)", p.name, tight)
		}
		near(t, (x0+x1)/2, 120, 1e-6, "%s fitted x center", p.name)
		near(t, (y0+y1)/2, 80, 1e-6, "%s fitted y center", p.name)
	}
}

func TestFitSizeIsFitExtentAtTheOrigin(t *testing.T) {
	obj := square(-30, -20, 30, 20)
	a := NewEqualEarth().FitSize(400, 300, obj)
	b := NewEqualEarth().FitExtent(0, 0, 400, 300, obj)
	ax0, ay0, ax1, ay1 := NewPath().Projection(a).Bounds(obj)
	bx0, by0, bx1, by1 := NewPath().Projection(b).Bounds(obj)
	near(t, ax0, bx0, 1e-9, "x0")
	near(t, ay0, by0, 1e-9, "y0")
	near(t, ax1, bx1, 1e-9, "x1")
	near(t, ay1, by1, 1e-9, "y1")
}

func TestFitWidthAndHeight(t *testing.T) {
	obj := square(-30, -20, 30, 20)
	p := NewEquirectangular().FitWidth(400, obj)
	x0, y0, x1, _ := NewPath().Projection(p).Bounds(obj)
	near(t, x1-x0, 400, 1e-6, "fitted width")
	near(t, y0, 0, 1e-6, "fitted top edge")

	p = NewEquirectangular().FitHeight(300, obj)
	x0, y0, _, y1 := NewPath().Projection(p).Bounds(obj)
	near(t, y1-y0, 300, 1e-6, "fitted height")
	near(t, x0, 0, 1e-6, "fitted left edge")
}

func TestFitIgnoresClipExtent(t *testing.T) {
	// A viewport clip must not shrink the measured bounds, or fitting would
	// depend on the clip it is about to make redundant.
	obj := square(-30, -20, 30, 20)
	a := NewEquirectangular().FitSize(400, 300, obj)
	b := NewEquirectangular().ClipExtent(0, 0, 10, 10).FitSize(400, 300, obj)
	ax0, ay0, ax1, ay1 := NewPath().Projection(a).Bounds(obj)
	b.ClearClipExtent()
	bx0, by0, bx1, by1 := NewPath().Projection(b).Bounds(obj)
	near(t, ax0, bx0, 1e-9, "x0")
	near(t, ay0, by0, 1e-9, "y0")
	near(t, ax1, bx1, 1e-9, "x1")
	near(t, ay1, by1, 1e-9, "y1")
}

// --- conic parallels ---------------------------------------------------------

func TestConicParallelsChangeTheProjection(t *testing.T) {
	a := NewConicEqualArea().Parallels(20, 50)
	b := NewConicEqualArea().Parallels(-10, 10)
	ax, ay := a.Project(30, 35)
	bx, by := b.Project(30, 35)
	if math.Abs(ax-bx) < 1e-6 && math.Abs(ay-by) < 1e-6 {
		t.Error("changing the standard parallels had no effect")
	}
	// Equal parallels are still a valid conic, and still invertible.
	c := NewConicConformal().Parallels(40, 40)
	x, y := c.Project(10, 40)
	lon, lat, ok := c.Invert(x, y)
	if !ok {
		t.Fatal("no inverse")
	}
	if d := Distance(10, 40, lon, lat); d > 1e-9 {
		t.Errorf("equal-parallel conic round trip off by %v", d)
	}
}

func TestConicEqualAreaDegeneratesToCylindrical(t *testing.T) {
	// Parallels symmetric about the equator make the cone a cylinder; the
	// projection must stay finite and still be equal-area.
	p := NewConicEqualArea().Parallels(-30, 30).Precision(0.1)
	path := NewPath().Projection(p)
	cells := equalAreaCells()
	base := path.Area(cells[0])
	for _, c := range cells[1:] {
		if r := path.Area(c) / base; r < 0.98 || r > 1.02 {
			t.Errorf("degenerate conic is not equal-area: ratio %v", r)
		}
	}
}

// --- identity ----------------------------------------------------------------

func TestIdentity(t *testing.T) {
	i := NewIdentity().Scale(2).Translate(10, 20)
	x, y := i.Project(3, 4)
	near(t, x, 16, 1e-12, "identity x")
	near(t, y, 28, 1e-12, "identity y")
	bx, by := i.Invert(x, y)
	near(t, bx, 3, 1e-12, "identity invert x")
	near(t, by, 4, 1e-12, "identity invert y")

	// Unlike a Projection, Identity does not flip y.
	j := NewIdentity()
	_, y = j.Project(0, 5)
	near(t, y, 5, 1e-12, "identity does not negate y")
	_, y = j.ReflectY(true).Project(0, 5)
	near(t, y, -5, 1e-12, "ReflectY negates y")
}

func TestIdentityRoundTripsUnderRotation(t *testing.T) {
	i := NewIdentity().Scale(3).Translate(-7, 11).Angle(37).ReflectX(true)
	for _, p := range [][2]float64{{0, 0}, {1, 2}, {-30, 40}} {
		x, y := i.Project(p[0], p[1])
		b0, b1 := i.Invert(x, y)
		near(t, b0, p[0], 1e-9, "identity invert x for %v", p)
		near(t, b1, p[1], 1e-9, "identity invert y for %v", p)
	}
}

func TestIdentityFit(t *testing.T) {
	obj := square(0, 0, 10, 20)
	i := NewIdentity().FitSize(100, 200, obj)
	x0, y0, x1, y1 := NewPath().Projection(i).Bounds(obj)
	near(t, x0, 0, 1e-9, "x0")
	near(t, y0, 0, 1e-9, "y0")
	near(t, x1, 100, 1e-9, "x1")
	near(t, y1, 200, 1e-9, "y1")
}

// --- precision ---------------------------------------------------------------

func TestPrecisionControlsResampling(t *testing.T) {
	// With resampling off, only the endpoints are emitted; with it on, points
	// appear in between, and finer precision emits more of them.
	// An equatorial segment under an obliquely rotated orthographic projection:
	// genuinely curved on screen, unlike a meridian in a conic projection, which
	// is straight and correctly needs no extra points at all.
	line := &LineString{Coordinates: []Position{P(-30, 0), P(30, 0)}}
	proj := NewOrthographic().Rotate(-10, -40, 0)

	count := func(delta float64) int {
		r := &recorder{}
		StreamObject(line, proj.Precision(delta).Stream(r))
		n := 0
		for _, e := range r.events {
			if len(e) > 5 && e[:5] == "point" {
				n++
			}
		}
		return n
	}
	none := count(0)
	coarse := count(2)
	fine := count(0.01)
	if none != 2 {
		t.Errorf("Precision(0) emitted %d points, want the 2 endpoints", none)
	}
	if !(coarse > none) {
		t.Errorf("resampling added no points: %d vs %d", coarse, none)
	}
	if !(fine > coarse) {
		t.Errorf("finer precision did not add points: %d vs %d", fine, coarse)
	}
}

func TestIdentityClipExtent(t *testing.T) {
	i := NewIdentity().ClipExtent(0, 0, 10, 10)
	p := NewPath().Projection(i)
	d := p.Generate(square(-5, -5, 20, 20))
	if d == "" {
		t.Fatal("clipped everything away")
	}
	for _, c := range coordsOf(t, d) {
		if c[0] < -1e-9 || c[0] > 10+1e-9 || c[1] < -1e-9 || c[1] > 10+1e-9 {
			t.Errorf("point %v escapes the clip extent", c)
		}
	}
	if got := NewPath().Projection(NewIdentity().ClipExtent(100, 100, 110, 110)).
		Generate(square(0, 0, 10, 10)); got != "" {
		t.Errorf("geometry outside the extent was drawn: %s", got)
	}
	// Clearing restores the unclipped output.
	a := NewPath().Projection(NewIdentity()).Generate(square(-5, -5, 20, 20))
	b := NewPath().Projection(NewIdentity().ClipExtent(0, 0, 10, 10).ClearClipExtent()).
		Generate(square(-5, -5, 20, 20))
	if a != b {
		t.Errorf("ClearClipExtent did not restore output:\n%s\n%s", a, b)
	}
}

func TestIdentityFitWidthAndHeight(t *testing.T) {
	obj := square(0, 0, 10, 20)

	i := NewIdentity().FitWidth(100, obj)
	x0, y0, x1, _ := NewPath().Projection(i).Bounds(obj)
	near(t, x1-x0, 100, 1e-9, "identity fitted width")
	near(t, y0, 0, 1e-9, "identity fitted top")

	j := NewIdentity().FitHeight(100, obj)
	x0, y0, _, y1 := NewPath().Projection(j).Bounds(obj)
	near(t, y1-y0, 100, 1e-9, "identity fitted height")
	near(t, x0, 0, 1e-9, "identity fitted left")

	k := NewIdentity().FitExtent(10, 20, 110, 220, obj)
	x0, y0, x1, y1 = NewPath().Projection(k).Bounds(obj)
	near(t, x0, 10, 1e-9, "identity fitExtent x0")
	near(t, y0, 20, 1e-9, "identity fitExtent y0")
	near(t, x1, 110, 1e-9, "identity fitExtent x1")
	near(t, y1, 220, 1e-9, "identity fitExtent y1")
}
