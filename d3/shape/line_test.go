package shape_test

import (
	"math"
	"testing"

	"github.com/malcolmston/d3/shape"
)

type pt struct {
	x, y float64
	ok   bool
}

func xs(d pt, _ int, _ []pt) float64 { return d.x }
func ys(d pt, _ int, _ []pt) float64 { return d.y }

func newLine() *shape.Line[pt] { return shape.NewLine[pt]().X(xs).Y(ys) }

func TestLineValues(t *testing.T) {
	tests := []struct {
		name string
		data []pt
		want string
	}{
		{"empty data draws nothing", nil, ""},
		{"a single point is closed so a round cap still renders it",
			[]pt{{1, 2, true}}, "M1,2Z"},
		{"a line through two points",
			[]pt{{0, 0, true}, {10, 20, true}}, "M0,0L10,20"},
		{"three points",
			[]pt{{0, 0, true}, {1, 1, true}, {2, 4, true}}, "M0,0L1,1L2,4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newLine().Generate(tt.data); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestLineDefinedGaps is the behavior that makes `defined` worth having: one
// path string, several disjoint subpaths, with no segment bridging the hole.
func TestLineDefinedGaps(t *testing.T) {
	defined := func(d pt, _ int, _ []pt) bool { return d.ok }

	tests := []struct {
		name         string
		data         []pt
		want         string
		wantSubpaths int
	}{
		{"a hole in the middle splits the line in two",
			[]pt{{0, 0, true}, {1, 1, true}, {2, 2, false}, {3, 3, true}, {4, 4, true}},
			"M0,0L1,1M3,3L4,4", 2},
		{"leading undefined points are skipped",
			[]pt{{0, 0, false}, {1, 1, true}, {2, 2, true}},
			"M1,1L2,2", 1},
		{"trailing undefined points are skipped",
			[]pt{{0, 0, true}, {1, 1, true}, {2, 2, false}},
			"M0,0L1,1", 1},
		{"two holes make three segments",
			[]pt{{0, 0, true}, {1, 1, false}, {2, 2, true}, {3, 3, false}, {4, 4, true}},
			"M0,0ZM2,2ZM4,4Z", 3},
		{"nothing defined draws nothing",
			[]pt{{0, 0, false}, {1, 1, false}}, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newLine().Defined(defined).Generate(tt.data)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if n := subpaths(parsePath(t, got)); n != tt.wantSubpaths {
				t.Errorf("subpaths = %d, want %d", n, tt.wantSubpaths)
			}
		})
	}
}

func TestLineDigits(t *testing.T) {
	data := []pt{{1.0 / 3.0, 2.0 / 3.0, true}, {1, 1, true}}
	if got := newLine().Digits(2).Generate(data); got != "M0.33,0.67L1,1" {
		t.Errorf("got %q", got)
	}
}

// TestLineRadial checks the polar convention: angle 0 is straight up, and a
// quarter turn is to the right.
func TestLineRadial(t *testing.T) {
	data := []pt{{0, 100, true}, {math.Pi / 2, 100, true}, {math.Pi, 100, true}}
	got := shape.NewLineRadial[pt]().Angle(xs).Radius(ys).Digits(3).Generate(data)
	want := "M0,-100L100,0L0,100"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLineRadialClosed(t *testing.T) {
	data := []pt{{0, 10, true}, {tauOverThree, 10, true}, {2 * tauOverThree, 10, true}}
	got := shape.NewLineRadial[pt]().Angle(xs).Radius(ys).
		Curve(shape.CurveLinearClosed).Digits(3).Generate(data)
	cmds := parsePath(t, got)
	if countOp(cmds, 'Z') != 1 {
		t.Errorf("a closed radial line must close exactly once: %q", got)
	}
	if n := len(cmds); n != 4 { // M, L, L, Z
		t.Errorf("commands = %d, want 4: %q", n, got)
	}
}

const tauOverThree = 2 * math.Pi / 3

func TestLineConstAccessors(t *testing.T) {
	// XConst/YConst are how a baseline or a rule is drawn from the same data.
	data := []pt{{0, 0, true}, {1, 1, true}}
	if got := shape.NewLine[pt]().X(xs).YConst(5).Generate(data); got != "M0,5L1,5" {
		t.Errorf("got %q", got)
	}
}
