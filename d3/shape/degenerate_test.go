package shape_test

import (
	"math"
	"testing"

	"github.com/malcolmston/d3/shape"
)

// A closed curve needs at least three points before its wrap-around spline is
// defined. With one or two it falls back to a degenerate figure, and those
// fallbacks are simple enough to state exactly — which matters, because they
// are what a chart renders while data is still loading.
func TestClosedCurveDegenerateCases(t *testing.T) {
	one := []pt{{0, 0, true}}
	two := []pt{{0, 0, true}, {3, 0, true}}

	tests := []struct {
		name  string
		curve shape.CurveFactory
		data  []pt
		want  string
	}{
		{"linearClosed, one point", shape.CurveLinearClosed, one, "M0,0Z"},
		{"linearClosed, two points", shape.CurveLinearClosed, two, "M0,0L3,0Z"},

		{"basisClosed, one point", shape.CurveBasisClosed, one, "M0,0Z"},
		// Two points collapse to the middle third of the segment, which is
		// where a closed quadratic B-spline over two control points lives.
		{"basisClosed, two points", shape.CurveBasisClosed, two, "M2,0L1,0Z"},

		{"cardinalClosed, one point", shape.CurveCardinalClosed, one, "M0,0Z"},
		{"cardinalClosed, two points", shape.CurveCardinalClosed, two, "M3,0L0,0Z"},

		{"catmullRomClosed, one point", shape.CurveCatmullRomClosed, one, "M0,0Z"},
		{"catmullRomClosed, two points", shape.CurveCatmullRomClosed, two, "M3,0L0,0Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shape.NewLine[pt]().X(xs).Y(ys).Curve(tt.curve).Digits(6).Generate(tt.data)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// The "open" curves need four points before they draw anything at all, since
// they skip the first and last segments.
func TestOpenCurveDegenerateCases(t *testing.T) {
	for _, c := range []struct {
		name  string
		curve shape.CurveFactory
	}{
		{"basisOpen", shape.CurveBasisOpen},
		{"cardinalOpen", shape.CurveCardinalOpen},
		{"catmullRomOpen", shape.CurveCatmullRomOpen},
	} {
		t.Run(c.name, func(t *testing.T) {
			data := []pt{{0, 0, true}, {1, 1, true}}
			if got := shape.NewLine[pt]().X(xs).Y(ys).Curve(c.curve).Generate(data); got != "" {
				t.Errorf("two points must draw nothing, got %q", got)
			}
		})
	}
}

// TestArcPerDatumAccessors is the radial bar chart: the outer radius encodes
// the value, so it has to come from the datum rather than a constant.
func TestArcPerDatumAccessors(t *testing.T) {
	a := shape.NewArc().
		InnerRadiusFunc(func(shape.ArcDatum) float64 { return 0 }).
		OuterRadiusFunc(func(d shape.ArcDatum) float64 { return d.OuterRadius * 2 }).
		StartAngleFunc(func(d shape.ArcDatum) float64 { return d.StartAngle }).
		EndAngleFunc(func(d shape.ArcDatum) float64 { return d.EndAngle }).
		CornerRadiusFunc(func(shape.ArcDatum) float64 { return 0 }).
		PadAngleFunc(func(shape.ArcDatum) float64 { return 0 }).
		Digits(3)
	got := a.Generate(shape.ArcDatum{OuterRadius: 50, StartAngle: 0, EndAngle: math.Pi / 2})
	if want := "M0,-100A100,100,0,0,1,100,0L0,0Z"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestArcPadRadiusFuncAndReset covers the two ways of controlling where the
// pad angle is measured, including the negative sentinel that restores the
// default.
func TestArcPadRadiusFuncAndReset(t *testing.T) {
	d := shape.ArcDatum{StartAngle: 0, EndAngle: math.Pi / 2}
	base := func() *shape.Arc {
		return shape.NewArc().InnerRadius(50).OuterRadius(100).PadAngle(0.1).Digits(6)
	}
	def := base().Generate(d)
	// The default pad radius is the quadratic mean of the radii, so setting it
	// explicitly to that value must reproduce the default exactly.
	explicit := base().PadRadiusFunc(func(shape.ArcDatum) float64 {
		return math.Sqrt(50*50 + 100*100)
	}).Generate(d)
	if def != explicit {
		t.Errorf("explicit quadratic-mean pad radius differs from the default:\n%q\n%q", def, explicit)
	}
	reset := base().PadRadius(20).PadRadius(-1).Generate(d)
	if reset != def {
		t.Errorf("a negative pad radius must restore the default:\n%q\n%q", reset, def)
	}
}

// TestAreaRadialAccessors exercises the polar spelling of the area accessors.
func TestAreaRadialAccessors(t *testing.T) {
	data := []pt{{0, 0, true}, {math.Pi / 2, 0, true}}
	got := shape.NewAreaRadial[pt]().
		StartAngle(xs).EndAngle(xs).
		InnerRadius(func(pt, int, []pt) float64 { return 40 }).
		OuterRadius(func(pt, int, []pt) float64 { return 80 }).
		Defined(func(d pt, _ int, _ []pt) bool { return true }).
		Curve(shape.CurveLinear).
		Digits(3).Generate(data)
	want := "M0,-80L80,0L40,0L0,-40Z"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestAreaConstSetters covers the constant spellings, which is how a fixed
// baseline or a fixed left edge is expressed.
func TestAreaConstSetters(t *testing.T) {
	data := []pt{{0, 1, true}, {1, 2, true}}
	got := shape.NewArea[pt]().XConst(5).YConst(7).Generate(data)
	if want := "M5,7L5,7L5,7L5,7Z"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	got = shape.NewArea[pt]().X0(xs).X1Const(9).Y(ys).Generate(data)
	if want := "M9,1L9,2L1,2L0,1Z"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestBundleIgnoresTheAreaProtocol: bundle is a line-only curve, so asking it
// for an area must not produce a half-drawn ring.
func TestBundleIgnoresTheAreaProtocol(t *testing.T) {
	got := shape.NewArea[pt]().X(xs).Y1(ys).Y0Const(0).Curve(shape.CurveBundle).Generate(wave)
	cmds := parsePath(t, got)
	assertFinite(t, got, cmds)
	if countOp(cmds, 'Z') != 0 {
		t.Errorf("a line-only curve must not close an area ring: %q", got)
	}
}

// TestRadialCurveWrapsAnyCurve: the radial transform composes with any curve,
// not just the linear default.
func TestRadialCurveWrapsAnyCurve(t *testing.T) {
	data := []pt{{0, 50, true}, {1, 60, true}, {2, 40, true}, {3, 70, true}}
	got := shape.NewLineRadial[pt]().Angle(xs).Radius(ys).
		Curve(shape.CurveCatmullRom).Generate(data)
	cmds := parsePath(t, got)
	assertFinite(t, got, cmds)
	if countOp(cmds, 'C') == 0 {
		t.Errorf("a smooth radial line must emit cubics: %q", got)
	}
	// The first point must land at angle 0, radius 50: straight up.
	pointCloseTo(t, "first point", endpoints(cmds)[0], 0, -50, 1e-9)
}
