package shape_test

import (
	"math"
	"testing"

	"github.com/malcolmston/d3/shape"
)

func newArea() *shape.Area[pt] { return shape.NewArea[pt]().X(xs).Y1(ys) }

// TestAreaValues pins the shape of an area path: topline in data order, then
// baseline in reverse, then Z. Everything about how an area reads on screen
// follows from that ordering.
func TestAreaValues(t *testing.T) {
	tests := []struct {
		name string
		area func() *shape.Area[pt]
		data []pt
		want string
	}{
		{"empty data draws nothing", newArea, nil, ""},
		{"two points, default baseline at zero",
			newArea,
			[]pt{{0, -10, true}, {10, -20, true}},
			"M0,-10L10,-20L10,0L0,0Z"},
		{"a constant baseline",
			func() *shape.Area[pt] { return newArea().Y0Const(100) },
			[]pt{{0, 10, true}, {10, 20, true}},
			"M0,10L10,20L10,100L0,100Z"},
		{"a per-datum baseline makes a band",
			func() *shape.Area[pt] {
				return shape.NewArea[pt]().X(xs).
					Y0(func(d pt, _ int, _ []pt) float64 { return d.y - 1 }).
					Y1(func(d pt, _ int, _ []pt) float64 { return d.y + 1 })
			},
			[]pt{{0, 10, true}, {10, 20, true}},
			"M0,11L10,21L10,19L0,9Z"},
		{"a single point still closes",
			newArea,
			[]pt{{5, 7, true}},
			"M5,7L5,0Z"},
		{"a horizontal band uses X0/X1 instead",
			func() *shape.Area[pt] {
				return shape.NewArea[pt]().X0Const(0).X1(xs).Y(ys)
			},
			[]pt{{10, 0, true}, {20, 1, true}},
			"M10,0L20,1L0,1L0,0Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.area().Generate(tt.data); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAreaClosesEachRing is the property that matters when data has holes:
// each contiguous run becomes its own closed ring, so the fill never bridges a
// gap.
func TestAreaClosesEachRing(t *testing.T) {
	data := []pt{
		{0, 1, true}, {1, 2, true},
		{2, 3, false},
		{3, 4, true}, {4, 5, true},
	}
	got := newArea().Defined(func(d pt, _ int, _ []pt) bool { return d.ok }).Generate(data)
	want := "M0,1L1,2L1,0L0,0ZM3,4L4,5L4,0L3,0Z"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	cmds := parsePath(t, got)
	if n := subpaths(cmds); n != 2 {
		t.Errorf("rings = %d, want 2", n)
	}
	if n := countOp(cmds, 'Z'); n != 2 {
		t.Errorf("closes = %d, want 2 (one per ring)", n)
	}
}

// TestAreaLine checks the convenience that lets the top edge of a fill be
// stroked separately.
func TestAreaLine(t *testing.T) {
	data := []pt{{0, 10, true}, {10, 20, true}}
	a := newArea()
	if got := a.LineY1().Generate(data); got != "M0,10L10,20" {
		t.Errorf("LineY1() = %q", got)
	}
	if got := a.LineY0().Generate(data); got != "M0,0L10,0" {
		t.Errorf("LineY0() = %q", got)
	}
}

func TestAreaRadial(t *testing.T) {
	// A radial band from r=50 to r=100 over half a turn.
	data := []pt{{0, 0, true}, {math.Pi, 0, true}}
	got := shape.NewAreaRadial[pt]().Angle(xs).
		InnerRadiusConst(50).OuterRadiusConst(100).Digits(3).Generate(data)
	want := "M0,-100L0,100L0,50L0,-50Z"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
