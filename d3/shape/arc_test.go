package shape_test

import (
	"math"
	"testing"

	"github.com/malcolmston/d3/shape"
)

// TestArcValues pins the simple arcs whose geometry is exact: a quarter wedge,
// a half wedge, a full circle and a full annulus. Everything is rounded to
// three digits because the cardinal angles have irrational cosines.
func TestArcValues(t *testing.T) {
	tests := []struct {
		name string
		arc  func() *shape.Arc
		want string
	}{
		{"a quarter wedge from twelve to three o'clock",
			func() *shape.Arc {
				return shape.NewArc().InnerRadius(0).OuterRadius(100).
					StartAngle(0).EndAngle(math.Pi / 2)
			},
			"M0,-100A100,100,0,0,1,100,0L0,0Z"},
		{"a half wedge sets the large-arc flag",
			func() *shape.Arc {
				return shape.NewArc().InnerRadius(0).OuterRadius(100).
					StartAngle(0).EndAngle(math.Pi)
			},
			"M0,-100A100,100,0,1,1,0,100L0,0Z"},
		{"a full circle needs two arcs",
			func() *shape.Arc {
				return shape.NewArc().InnerRadius(0).OuterRadius(100).
					StartAngle(0).EndAngle(2 * math.Pi)
			},
			"M0,-100A100,100,0,1,1,0,100A100,100,0,1,1,0,-100Z"},
		{"a full annulus punches the hole with a counter-wound circle",
			func() *shape.Arc {
				return shape.NewArc().InnerRadius(50).OuterRadius(100).
					StartAngle(0).EndAngle(2 * math.Pi)
			},
			"M0,-100A100,100,0,1,1,0,100A100,100,0,1,1,0,-100M0,-50A50,50,0,1,0,0,50A50,50,0,1,0,0,-50Z"},
		{"a zero outer radius degenerates to the origin",
			func() *shape.Arc {
				return shape.NewArc().InnerRadius(0).OuterRadius(0).
					StartAngle(0).EndAngle(math.Pi)
			},
			"M0,0Z"},
		{"radii given the wrong way round are swapped",
			func() *shape.Arc {
				return shape.NewArc().InnerRadius(100).OuterRadius(0).
					StartAngle(0).EndAngle(math.Pi / 2)
			},
			"M0,-100A100,100,0,0,1,100,0L0,0Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.arc().Digits(3).Generate(shape.ArcDatum{}); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestArcReadsTheDatum(t *testing.T) {
	// The default accessors take angles and radii off the datum, which is what
	// makes Pie's output directly drawable.
	got := shape.NewArc().Digits(3).Generate(shape.ArcDatum{
		InnerRadius: 0, OuterRadius: 100, StartAngle: 0, EndAngle: math.Pi / 2,
	})
	if want := "M0,-100A100,100,0,0,1,100,0L0,0Z"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestArcCentroid(t *testing.T) {
	tests := []struct {
		name         string
		start, end   float64
		inner, outer float64
		wantX, wantY float64
	}{
		{"a wedge pointing up", 0, 0, 0, 100, 0, -50},
		{"a wedge pointing right", math.Pi / 2, math.Pi / 2, 0, 100, 50, 0},
		{"an annulus centroid sits between the radii", 0, 0, 50, 100, 0, -75},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := shape.NewArc().InnerRadius(tt.inner).OuterRadius(tt.outer)
			x, y := a.Centroid(shape.ArcDatum{StartAngle: tt.start, EndAngle: tt.end})
			closeTo(t, "x", x, tt.wantX, 1e-9)
			closeTo(t, "y", y, tt.wantY, 1e-9)
		})
	}
}

// clockAngle converts an emitted point back to the package's angular
// convention: 0 at twelve o'clock, increasing clockwise.
func clockAngle(x, y float64) float64 {
	a := math.Atan2(x, -y)
	if a < 0 {
		a += 2 * math.Pi
	}
	return a
}

// TestArcPadAngleInsetsTheEdges: padding must pull both edges of the wedge
// inward, and by a larger angle at the inner radius than at the outer, so the
// visible gap has parallel sides.
func TestArcPadAngleInsetsTheEdges(t *testing.T) {
	const r0, r1 = 50.0, 100.0
	const start, end = 0.0, math.Pi / 2
	a := shape.NewArc().InnerRadius(r0).OuterRadius(r1).PadAngle(0.2)
	got := a.Generate(shape.ArcDatum{StartAngle: start, EndAngle: end})
	cmds := parsePath(t, got)
	assertFinite(t, got, cmds)

	// The first point is on the outer ring at the padded start angle.
	outerStart := endpoints(cmds)[0]
	closeTo(t, "outer start radius", math.Hypot(outerStart[0], outerStart[1]), r1, 1e-9)
	outerInset := clockAngle(outerStart[0], outerStart[1]) - start
	if outerInset <= 0 {
		t.Fatalf("padding must inset the outer edge, got %v", outerInset)
	}

	// The inner edge starts where the outer arc's LineTo lands.
	var innerPoint [2]float64
	for i, c := range cmds {
		if c.op == 'L' && i > 0 {
			innerPoint = [2]float64{c.args[0], c.args[1]}
			break
		}
	}
	closeTo(t, "inner radius", math.Hypot(innerPoint[0], innerPoint[1]), r0, 1e-9)
	innerInset := end - clockAngle(innerPoint[0], innerPoint[1])
	if innerInset <= outerInset {
		t.Errorf("inner inset %v must exceed outer inset %v so the gap has parallel sides",
			innerInset, outerInset)
	}
}

// TestArcPadAngleCollapse: when padding exceeds the wedge, the sector must
// collapse to its bisector rather than invert.
func TestArcPadAngleCollapse(t *testing.T) {
	a := shape.NewArc().InnerRadius(50).OuterRadius(100).PadAngle(3)
	got := a.Digits(6).Generate(shape.ArcDatum{StartAngle: 0, EndAngle: 0.05})
	cmds := parsePath(t, got)
	assertFinite(t, got, cmds)
	for _, p := range endpoints(cmds) {
		r := math.Hypot(p[0], p[1])
		if r > 100+1e-6 {
			t.Fatalf("collapsed sector escaped the outer radius: %q", got)
		}
	}
}

// TestArcCornerRadius: rounded corners must stay inside the annulus, and the
// corner radius must be clamped to half the ring thickness so opposite corners
// cannot cross.
func TestArcCornerRadius(t *testing.T) {
	const r0, r1 = 40.0, 60.0
	for _, rc := range []float64{2, 10, 1000} {
		a := shape.NewArc().InnerRadius(r0).OuterRadius(r1).CornerRadius(rc)
		got := a.Generate(shape.ArcDatum{StartAngle: 0, EndAngle: math.Pi / 3})
		cmds := parsePath(t, got)
		assertFinite(t, got, cmds)
		for _, p := range endpoints(cmds) {
			r := math.Hypot(p[0], p[1])
			if r > r1+1e-6 || r < r0-1e-6 {
				t.Errorf("cornerRadius %v put a point at r=%v, outside [%v,%v]: %q",
					rc, r, r0, r1, got)
			}
		}
		// Every corner arc's radius must be at most half the thickness.
		for _, c := range cmds {
			if c.op == 'A' && c.args[0] < r0 {
				if c.args[0] > (r1-r0)/2+1e-9 {
					t.Errorf("corner radius %v exceeds half the ring thickness", c.args[0])
				}
			}
		}
	}
}

// TestArcCornerRadiusIgnoredOnFullCircle: a complete annulus has no corners,
// so the corner radius must make no difference at all.
func TestArcCornerRadiusIgnoredOnFullCircle(t *testing.T) {
	d := shape.ArcDatum{StartAngle: 0, EndAngle: 2 * math.Pi}
	plain := shape.NewArc().InnerRadius(50).OuterRadius(100).Digits(6).Generate(d)
	round := shape.NewArc().InnerRadius(50).OuterRadius(100).CornerRadius(10).Digits(6).Generate(d)
	if plain != round {
		t.Errorf("corner radius changed a full annulus:\n%q\n%q", plain, round)
	}
}

// TestArcCornerAndPadTogether is the donut-chart configuration that exercises
// both features at once; it must still produce a single well-formed closed
// path inside the annulus.
func TestArcCornerAndPadTogether(t *testing.T) {
	a := shape.NewArc().InnerRadius(60).OuterRadius(100).CornerRadius(6).PadAngle(0.04)
	pie := shape.NewPie[float64]().Value(func(v float64, _ int, _ []float64) float64 { return v })
	for _, arc := range pie.Generate([]float64{3, 1, 1, 5}) {
		got := a.Generate(arc.Arc())
		cmds := parsePath(t, got)
		assertFinite(t, got, cmds)
		if n := subpaths(cmds); n != 1 {
			t.Errorf("a sector is one subpath, got %d: %q", n, got)
		}
		if countOp(cmds, 'Z') != 1 {
			t.Errorf("a sector must close exactly once: %q", got)
		}
		for _, p := range endpoints(cmds) {
			if r := math.Hypot(p[0], p[1]); r > 100+1e-6 || r < 60-1e-6 {
				t.Errorf("point at r=%v outside [60,100]: %q", r, got)
			}
		}
	}
}

// TestArcPadRadius: the pad radius controls where the pad angle is measured,
// so a larger pad radius must produce a wider gap.
func TestArcPadRadius(t *testing.T) {
	d := shape.ArcDatum{StartAngle: 0, EndAngle: math.Pi / 2}
	inset := func(padRadius float64) float64 {
		a := shape.NewArc().InnerRadius(50).OuterRadius(100).PadAngle(0.1).PadRadius(padRadius)
		cmds := parsePath(t, a.Generate(d))
		p := endpoints(cmds)[0]
		return clockAngle(p[0], p[1])
	}
	small, large := inset(50), inset(150)
	if !(large > small) {
		t.Errorf("a larger pad radius must widen the gap: %v vs %v", large, small)
	}
}
