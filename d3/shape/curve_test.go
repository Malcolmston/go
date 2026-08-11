package shape_test

import (
	"math"
	"testing"

	"github.com/malcolmston/d3/shape"
)

// Curves come in two flavors for testing. A few have output simple enough to
// derive from the published formula by hand — those are asserted exactly. The
// rest are asserted by property: well-formed path, right number of segments,
// starts and ends where the data does, no NaNs. Inventing "expected" control
// points for a Catmull-Rom spline would only test that the test matches the
// code.

// TestCurveExactValues pins the curves whose control points follow directly
// from their definitions.
func TestCurveExactValues(t *testing.T) {
	two := []pt{{0, 0, true}, {10, 10, true}}
	three := []pt{{0, 0, true}, {1, 1, true}, {2, 0, true}}

	tests := []struct {
		name  string
		curve shape.CurveFactory
		data  []pt
		want  string
	}{
		{"linear", shape.CurveLinear, three, "M0,0L1,1L2,0"},
		{"linearClosed", shape.CurveLinearClosed, three, "M0,0L1,1L2,0Z"},

		// A step's transition is a fraction t of the way between the points.
		{"step (t=0.5) turns midway", shape.CurveStep, two, "M0,0L5,0L5,10L10,10"},
		{"stepBefore (t=0) rises first", shape.CurveStepBefore, two, "M0,0L0,10L10,10"},
		{"stepAfter (t=1) runs first", shape.CurveStepAfter, two, "M0,0L10,0L10,10"},

		// Two points give a B-spline no interior to smooth, so it degenerates
		// to the straight run-out alone.
		{"basis with two points is a straight line", shape.CurveBasis, two, "M0,0L10,10"},

		// Three points: run-in to one sixth, one interior segment, run-out.
		// Control points are (2*x0+x1)/3, (x0+2*x1)/3 and (x0+4*x1+x)/6.
		{"basis with three points", shape.CurveBasis, three,
			"M0,0L0.167,0.167C0.333,0.333,0.667,0.667,1,0.667C1.333,0.667,1.667,0.333,1.833,0.167L2,0"},

		// MonotoneX at a local maximum: the Fritsch-Carlson filter forces the
		// tangent to zero because the two secants disagree in sign, so the
		// curve flattens at the peak instead of overshooting it. The end
		// tangents then follow from slope2: (3*secant - t)/2 = ±1.5.
		{"monotoneX flattens at a local maximum", shape.CurveMonotoneX, three,
			"M0,0C0.333,0.5,0.667,1,1,1C1.333,1,1.667,0.5,2,0"},

		// A natural spline through two points has no interior to solve.
		{"natural with two points is a straight line", shape.CurveNatural, two, "M0,0L10,10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shape.NewLine[pt]().X(xs).Y(ys).Curve(tt.curve).Digits(3).Generate(tt.data)
			if got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

// curveCases enumerates every exported curve with the properties it is
// expected to have, so a new curve cannot be added without declaring them.
type curveCase struct {
	name string
	f    shape.CurveFactory
	// startsAtFirst is false for the "open" curves, which deliberately begin
	// inside the data rather than at its first point.
	startsAtFirst bool
	endsAtLast    bool
	// wraps is true for the closed curves, whose last point must coincide with
	// their first. Note that the smooth closed curves do NOT emit a Z: the
	// spline is wrapped around geometrically, so the ends already meet.
	wraps bool
	// supportsArea is false for curves that ignore the area protocol.
	supportsArea bool
}

func allCurves() []curveCase {
	return []curveCase{
		{"Linear", shape.CurveLinear, true, true, false, true},
		{"LinearClosed", shape.CurveLinearClosed, true, false, true, false},
		{"Basis", shape.CurveBasis, true, true, false, true},
		{"BasisClosed", shape.CurveBasisClosed, false, false, true, false},
		{"BasisOpen", shape.CurveBasisOpen, false, false, false, true},
		{"Bundle", shape.CurveBundle, false, false, false, false},
		{"BundleBeta1", shape.CurveBundleFactory(1), true, true, false, false},
		{"Cardinal", shape.CurveCardinal, true, true, false, true},
		{"CardinalClosed", shape.CurveCardinalClosed, false, false, true, false},
		{"CardinalOpen", shape.CurveCardinalOpen, false, false, false, true},
		{"CatmullRom", shape.CurveCatmullRom, true, true, false, true},
		{"CatmullRomClosed", shape.CurveCatmullRomClosed, false, false, true, false},
		{"CatmullRomOpen", shape.CurveCatmullRomOpen, false, false, false, true},
		{"MonotoneX", shape.CurveMonotoneX, true, true, false, true},
		{"MonotoneY", shape.CurveMonotoneY, true, true, false, true},
		{"Natural", shape.CurveNatural, true, true, false, true},
		{"Step", shape.CurveStep, true, true, false, true},
		{"StepBefore", shape.CurveStepBefore, true, true, false, true},
		{"StepAfter", shape.CurveStepAfter, true, true, false, true},
		{"CardinalTension", shape.CurveCardinalFactory(0.5), true, true, false, true},
		{"CatmullRomAlpha0", shape.CurveCatmullRomFactory(0), true, true, false, true},
		{"CatmullRomChordal", shape.CurveCatmullRomFactory(1), true, true, false, true},
	}
}

// wave is deliberately non-monotone in y and unevenly spaced in x, which is
// where the interesting failure modes of the smoothing curves live.
var wave = []pt{
	{0, 0, true}, {10, 30, true}, {25, 10, true},
	{40, 45, true}, {55, 20, true}, {70, 35, true},
}

func TestCurveProperties(t *testing.T) {
	for _, c := range allCurves() {
		t.Run(c.name, func(t *testing.T) {
			got := shape.NewLine[pt]().X(xs).Y(ys).Curve(c.f).Generate(wave)
			if got == "" {
				t.Fatal("curve produced no path")
			}
			cmds := parsePath(t, got)
			assertFinite(t, got, cmds)
			if n := subpaths(cmds); n != 1 {
				t.Errorf("subpaths = %d, want 1", n)
			}
			pts := endpoints(cmds)
			if c.wraps {
				pointCloseTo(t, "a closed curve must return to its start",
					pts[len(pts)-1], pts[0][0], pts[0][1], 1e-6)
			}
			if c.startsAtFirst {
				pointCloseTo(t, "start", pts[0], wave[0].x, wave[0].y, 1e-9)
			}
			if c.endsAtLast {
				last := pts[len(pts)-1]
				n := len(wave) - 1
				pointCloseTo(t, "end", last, wave[n].x, wave[n].y, 1e-9)
			}
		})
	}
}

// TestCurveDefinedGaps checks that every curve honors the gap protocol: a
// break in the data must produce a second subpath, never a bridging segment.
func TestCurveDefinedGaps(t *testing.T) {
	data := []pt{
		{0, 0, true}, {1, 10, true}, {2, 5, true}, {3, 8, true},
		{4, 0, false},
		{5, 0, true}, {6, 10, true}, {7, 5, true}, {8, 8, true},
	}
	for _, c := range allCurves() {
		t.Run(c.name, func(t *testing.T) {
			got := shape.NewLine[pt]().X(xs).Y(ys).Curve(c.f).
				Defined(func(d pt, _ int, _ []pt) bool { return d.ok }).Generate(data)
			cmds := parsePath(t, got)
			assertFinite(t, got, cmds)
			if n := subpaths(cmds); n != 2 {
				t.Errorf("subpaths = %d, want 2 (one per contiguous run): %q", n, got)
			}
		})
	}
}

// TestCurveAreas checks the two-lines-one-ring protocol for every curve that
// participates in it.
func TestCurveAreas(t *testing.T) {
	for _, c := range allCurves() {
		if !c.supportsArea {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			got := shape.NewArea[pt]().X(xs).Y1(ys).Y0Const(0).Curve(c.f).Generate(wave)
			cmds := parsePath(t, got)
			assertFinite(t, got, cmds)
			if n := subpaths(cmds); n != 1 {
				t.Errorf("an area is one ring, got %d subpaths: %q", n, got)
			}
			if n := countOp(cmds, 'Z'); n != 1 {
				t.Errorf("an area must close exactly once, got %d: %q", n, got)
			}
		})
	}
}

// TestMonotoneXDoesNotOvershoot is the property that gives CurveMonotoneX its
// name and its purpose: a spline through non-negative data must never dip
// below zero or rise above the data's maximum, however sharp the corner.
func TestMonotoneXDoesNotOvershoot(t *testing.T) {
	data := []pt{{0, 0, true}, {1, 0, true}, {2, 100, true}, {3, 100, true}, {4, 0, true}}
	got := shape.NewLine[pt]().X(xs).Y(ys).Curve(shape.CurveMonotoneX).Generate(data)
	cmds := parsePath(t, got)
	// Every control point of a cubic bounds the curve, so checking them is a
	// sufficient (conservative) test of the curve's own range.
	for _, c := range cmds {
		for i := 1; i < len(c.args); i += 2 {
			y := c.args[i]
			if y < -1e-9 || y > 100+1e-9 {
				t.Fatalf("monotoneX overshot to y=%v in %q", y, got)
			}
		}
	}
}

// TestMonotoneXIgnoresCoincidentPoints guards the guard: a repeated point must
// not produce a zero-length secant and a NaN slope.
func TestMonotoneXIgnoresCoincidentPoints(t *testing.T) {
	data := []pt{{0, 0, true}, {1, 1, true}, {1, 1, true}, {2, 2, true}}
	got := shape.NewLine[pt]().X(xs).Y(ys).Curve(shape.CurveMonotoneX).Generate(data)
	cmds := parsePath(t, got)
	assertFinite(t, got, cmds)
	if n := countOp(cmds, 'C'); n != 2 {
		t.Errorf("segments = %d, want 2 (the duplicate contributes none): %q", n, got)
	}
}

// TestMonotoneYIsTheTranspose: running MonotoneY on transposed data must give
// the transpose of MonotoneX's output, since that is literally how it is
// implemented — and a transposition bug in the reflect context would not show
// up in any single-axis test.
func TestMonotoneYIsTheTranspose(t *testing.T) {
	forward := []pt{{0, 0, true}, {1, 5, true}, {2, 3, true}, {3, 9, true}}
	transposed := make([]pt, len(forward))
	for i, d := range forward {
		transposed[i] = pt{d.y, d.x, true}
	}
	gotX := shape.NewLine[pt]().X(xs).Y(ys).Curve(shape.CurveMonotoneX).Digits(6).Generate(forward)
	gotY := shape.NewLine[pt]().X(xs).Y(ys).Curve(shape.CurveMonotoneY).Digits(6).Generate(transposed)

	cx := parsePath(t, gotX)
	cy := parsePath(t, gotY)
	if len(cx) != len(cy) {
		t.Fatalf("command counts differ: %q vs %q", gotX, gotY)
	}
	for i := range cx {
		if cx[i].op != cy[i].op {
			t.Fatalf("command %d differs: %q vs %q", i, gotX, gotY)
		}
		for j := 0; j < len(cx[i].args); j += 2 {
			closeTo(t, "x/y swap", cy[i].args[j], cx[i].args[j+1], 1e-6)
			closeTo(t, "y/x swap", cy[i].args[j+1], cx[i].args[j], 1e-6)
		}
	}
}

// TestCurveBundleBeta0 collapses to the straight chord between the endpoints,
// which is the one bundle configuration with an obvious expected answer.
func TestCurveBundleBeta0(t *testing.T) {
	data := []pt{{0, 0, true}, {1, 100, true}, {2, -100, true}, {3, 0, true}}
	got := shape.NewLine[pt]().X(xs).Y(ys).
		Curve(shape.CurveBundleFactory(0)).Digits(6).Generate(data)
	cmds := parsePath(t, got)
	for _, v := range allNumbers(cmds) {
		if math.Abs(v) > 3+1e-6 {
			t.Fatalf("beta=0 must collapse onto the chord y=0 from x=0 to x=3, got %q", got)
		}
	}
	// All the y coordinates must be zero: the chord is horizontal.
	for _, c := range cmds {
		for i := 1; i < len(c.args); i += 2 {
			closeTo(t, "chord y", c.args[i], 0, 1e-6)
		}
	}
}

// TestCurveNaturalInterpolates: a natural spline passes through every data
// point, so each cubic's endpoint must be a datum.
func TestCurveNaturalInterpolates(t *testing.T) {
	got := shape.NewLine[pt]().X(xs).Y(ys).Curve(shape.CurveNatural).Digits(6).Generate(wave)
	cmds := parsePath(t, got)
	pts := endpoints(cmds)
	if len(pts) != len(wave) {
		t.Fatalf("expected one command per point, got %d for %d points", len(pts), len(wave))
	}
	for i, want := range wave {
		pointCloseTo(t, "interpolated point", pts[i], want.x, want.y, 1e-6)
	}
}

// TestCurveStepMirrorsInAreas: the baseline of an area is traversed in
// reverse, so the step must flip to land on the same x positions — otherwise
// the fill is offset from the line by one step width.
func TestCurveStepMirrorsInAreas(t *testing.T) {
	data := []pt{{0, 10, true}, {10, 20, true}}
	got := shape.NewArea[pt]().X(xs).Y1(ys).Y0Const(0).Curve(shape.CurveStep).Generate(data)
	want := "M0,10L5,10L5,20L10,20L10,0L5,0L5,0L0,0Z"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
