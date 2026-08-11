package shape_test

import (
	"math"
	"testing"

	"github.com/malcolmston/d3/shape"
)

// TestSymbolValues pins the symbols whose geometry is exact at a chosen size:
// a square of area 4 is 2×2, a circle of area π has radius 1, a cross of area
// 5 is built from five unit squares.
func TestSymbolValues(t *testing.T) {
	tests := []struct {
		name string
		typ  shape.SymbolType
		size float64
		want string
	}{
		{"square of area 4 is 2 by 2", shape.SymbolSquare, 4, "M-1,-1h2v2h-2Z"},
		{"circle of area pi has radius 1", shape.SymbolCircle, math.Pi,
			"M1,0A1,1,0,1,1,-1,0A1,1,0,1,1,1,0"},
		{"cross of area 5 is five unit squares", shape.SymbolCross, 5,
			"M-1.5,-0.5L-0.5,-0.5L-0.5,-1.5L0.5,-1.5L0.5,-0.5L1.5,-0.5L1.5,0.5L0.5,0.5L0.5,1.5L-0.5,1.5L-0.5,0.5L-1.5,0.5Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shape.NewSymbol().Type(tt.typ).Size(tt.size).Digits(6).Generate()
			if got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

// TestSymbolAreas is the property that makes symbols comparable: whatever the
// shape, the drawn area must equal the requested size, because that is what
// the viewer reads. Areas are measured with the shoelace formula over the
// polygon's vertices (the circle is excluded, having no vertices).
func TestSymbolAreas(t *testing.T) {
	polygons := []struct {
		name string
		typ  shape.SymbolType
	}{
		{"Cross", shape.SymbolCross},
		{"Diamond", shape.SymbolDiamond},
		{"Square", shape.SymbolSquare},
		{"Star", shape.SymbolStar},
		{"Triangle", shape.SymbolTriangle},
		{"Wye", shape.SymbolWye},
	}
	for _, p := range polygons {
		t.Run(p.name, func(t *testing.T) {
			for _, size := range []float64{1, 64, 500} {
				got := shape.NewSymbol().Type(p.typ).Size(size).Generate()
				cmds := parsePath(t, got)
				area := math.Abs(shoelace(endpoints(cmds)))
				if math.Abs(area-size)/size > 1e-6 {
					t.Errorf("size %v: drawn area = %v", size, area)
				}
			}
		})
	}
}

// shoelace returns twice-signed-area/2 of the polygon through pts, dropping
// the trailing repeat produced by Z.
func shoelace(pts [][2]float64) float64 {
	if len(pts) > 1 && pts[len(pts)-1] == pts[0] {
		pts = pts[:len(pts)-1]
	}
	sum := 0.0
	for i := range pts {
		j := (i + 1) % len(pts)
		sum += pts[i][0]*pts[j][1] - pts[j][0]*pts[i][1]
	}
	return sum / 2
}

func TestSymbolCircleArea(t *testing.T) {
	// The circle is drawn as two arcs; check its radius instead of its area.
	const size = 64
	got := shape.NewSymbol().Type(shape.SymbolCircle).Size(size).Generate()
	cmds := parsePath(t, got)
	wantR := math.Sqrt(size / math.Pi)
	for _, c := range cmds {
		if c.op == 'A' {
			closeTo(t, "arc radius", c.args[0], wantR, 1e-9)
		}
	}
	closeTo(t, "start x", cmds[0].args[0], wantR, 1e-9)
}

// TestSymbolsAreCentered: every symbol must be centered on the origin so it
// can be positioned with a plain translate.
func TestSymbolsAreCentered(t *testing.T) {
	for i, typ := range shape.SymbolsFill {
		got := shape.NewSymbol().Type(typ).Size(100).Generate()
		cmds := parsePath(t, got)
		assertFinite(t, got, cmds)
		pts := endpoints(cmds)
		minX, maxX := math.Inf(1), math.Inf(-1)
		minY, maxY := math.Inf(1), math.Inf(-1)
		for _, p := range pts {
			minX, maxX = math.Min(minX, p[0]), math.Max(maxX, p[0])
			minY, maxY = math.Min(minY, p[1]), math.Max(maxY, p[1])
		}
		// Every symbol is left-right symmetric, so its horizontal extent must
		// straddle the origin. (Vertical symmetry does not hold for the star or
		// the triangle, and the circle's on-curve points are all on the x-axis,
		// so neither is asserted here.)
		if math.Abs(minX+maxX) > 1e-6 {
			t.Errorf("symbol %d is not horizontally centered: x in [%v,%v]", i, minX, maxX)
		}
		if maxX <= minX || maxY < minY {
			t.Errorf("symbol %d has no extent", i)
		}
	}
}

func TestSymbolDefaults(t *testing.T) {
	// A circle of area 64.
	want := shape.NewSymbol().Type(shape.SymbolCircle).Size(64).Generate()
	if got := shape.NewSymbol().Generate(); got != want {
		t.Errorf("default symbol = %q, want %q", got, want)
	}
	if n := len(shape.SymbolsFill); n != 7 {
		t.Errorf("SymbolsFill has %d entries, want 7", n)
	}
}
