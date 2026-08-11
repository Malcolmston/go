package path_test

import (
	"math"
	"strings"
	"testing"

	"github.com/malcolmston/d3/path"
)

// Path data is one of the few places in a graphics port where exact assertions
// are legitimate: the SVG grammar is fixed and d3's serialization is fully
// specified, so every string below was derived by hand from the command
// definitions rather than captured from an implementation.

func TestEmpty(t *testing.T) {
	if got := path.New().String(); got != "" {
		t.Errorf("empty path = %q, want %q", got, "")
	}
	// ClosePath on an empty path must be a no-op; the shape generators rely on
	// it when a series has no defined points.
	if got := path.New().ClosePath().String(); got != "" {
		t.Errorf("empty ClosePath = %q, want %q", got, "")
	}
}

func TestCommands(t *testing.T) {
	tests := []struct {
		name string
		draw func(p *path.Path) *path.Path
		want string
	}{
		{"moveTo", func(p *path.Path) *path.Path { return p.MoveTo(150, 50) }, "M150,50"},
		{"line through two points",
			func(p *path.Path) *path.Path { return p.MoveTo(0, 0).LineTo(10, 20) },
			"M0,0L10,20"},
		{"closed triangle",
			func(p *path.Path) *path.Path { return p.MoveTo(0, 0).LineTo(4, 0).LineTo(0, 3).ClosePath() },
			"M0,0L4,0L0,3Z"},
		{"rect",
			func(p *path.Path) *path.Path { return p.Rect(1, 2, 10, 20) },
			"M1,2h10v20h-10Z"},
		{"rect with negative size",
			func(p *path.Path) *path.Path { return p.Rect(0, 0, -5, -7) },
			"M0,0h-5v-7h5Z"},
		{"quadratic",
			func(p *path.Path) *path.Path { return p.MoveTo(0, 0).QuadraticCurveTo(1, 2, 3, 4) },
			"M0,0Q1,2,3,4"},
		{"cubic",
			func(p *path.Path) *path.Path { return p.MoveTo(0, 0).BezierCurveTo(1, 2, 3, 4, 5, 6) },
			"M0,0C1,2,3,4,5,6"},
		{"closePath returns to subpath start",
			// The second LineTo must start from (0,0), the subpath origin, not
			// from (4,0) — only observable through a later command.
			func(p *path.Path) *path.Path { return p.MoveTo(0, 0).LineTo(4, 0).ClosePath().LineTo(1, 1) },
			"M0,0L4,0ZL1,1"},
		{"two subpaths",
			func(p *path.Path) *path.Path { return p.MoveTo(0, 0).LineTo(1, 1).MoveTo(5, 5).LineTo(6, 6) },
			"M0,0L1,1M5,5L6,6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.draw(path.New()).String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNumberFormatting(t *testing.T) {
	tests := []struct {
		name string
		x, y float64
		want string
	}{
		{"integers have no decimal point", 1, 2, "M1,2"},
		{"integral floats are not padded", 1.0, 2.0, "M1,2"},
		{"fractions keep only significant digits", 0.5, -0.25, "M0.5,-0.25"},
		{"negative zero prints as zero", math.Copysign(0, -1), 0, "M0,0"},
		{"one third round-trips", 1.0 / 3.0, 0, "M0.3333333333333333,0"},
		{"no exponent notation", 1e-7, 1e21, "M0.0000001,1000000000000000000000"},
		{"large integers", 123456789, -987654321, "M123456789,-987654321"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := path.New().MoveTo(tt.x, tt.y).String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrecision(t *testing.T) {
	tests := []struct {
		name   string
		digits int
		x, y   float64
		want   string
	}{
		{"zero digits rounds to integers", 0, 1.4, 1.6, "M1,2"},
		{"three digits", 3, 1.0 / 3.0, 2.0 / 3.0, "M0.333,0.667"},
		{"trailing zeros are still dropped", 3, 1.5, 2.0, "M1.5,2"},
		{"half rounds up, not away from zero", 0, -0.5, 0.5, "M0,1"},
		{"more than 15 digits is full precision", 17, 1.0 / 3.0, 0, "M0.3333333333333333,0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := path.NewWithPrecision(tt.digits).MoveTo(tt.x, tt.y).String()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestArc(t *testing.T) {
	// Rounded to three digits so the irrational endpoints of the cardinal
	// angles (cos(π/2) is 6.1e-17, not 0) do not obscure the command grammar,
	// which is what these cases are about.
	tests := []struct {
		name string
		draw func(p *path.Path) *path.Path
		want string
	}{
		{"quarter arc on an empty path moves to the start",
			func(p *path.Path) *path.Path { return p.Arc(0, 0, 1, 0, math.Pi/2, false) },
			"M1,0A1,1,0,0,1,0,1"},
		{"counterclockwise quarter arc flips the sweep flag",
			func(p *path.Path) *path.Path { return p.Arc(0, 0, 1, 0, -math.Pi/2, true) },
			"M1,0A1,1,0,0,0,0,-1"},
		{"half turn sets the large-arc flag",
			func(p *path.Path) *path.Path { return p.Arc(0, 0, 1, 0, math.Pi, false) },
			"M1,0A1,1,0,1,1,-1,0"},
		{"three-quarter turn is large",
			func(p *path.Path) *path.Path { return p.Arc(0, 0, 1, 0, 3*math.Pi/2, false) },
			"M1,0A1,1,0,1,1,0,-1"},
		{"full circle becomes two arcs through the antipode",
			func(p *path.Path) *path.Path { return p.Arc(0, 0, 1, 0, 2*math.Pi, false) },
			"M1,0A1,1,0,1,1,-1,0A1,1,0,1,1,1,0"},
		{"more than a full turn is still two arcs",
			func(p *path.Path) *path.Path { return p.Arc(0, 0, 1, 0, 3*math.Pi, false) },
			"M1,0A1,1,0,1,1,-1,0A1,1,0,1,1,1,0"},
		{"zero radius draws only the join",
			func(p *path.Path) *path.Path { return p.Arc(5, 6, 0, 0, math.Pi, false) },
			"M5,6"},
		{"zero sweep draws only the join",
			func(p *path.Path) *path.Path { return p.Arc(0, 0, 1, 1, 1, false) },
			"M0.54,0.841"},
		{"start point is joined with a line when the path is in progress",
			func(p *path.Path) *path.Path { return p.MoveTo(-1, 0).Arc(0, 0, 1, 0, math.Pi/2, false) },
			"M-1,0L1,0A1,1,0,0,1,0,1"},
		{"no line is emitted when already at the start point",
			func(p *path.Path) *path.Path { return p.MoveTo(1, 0).Arc(0, 0, 1, 0, math.Pi/2, false) },
			"M1,0A1,1,0,0,1,0,1"},
		{"a negative sweep normalizes into a positive turn",
			// start π, end 0, clockwise: da = -π normalizes to +π.
			func(p *path.Path) *path.Path { return p.Arc(0, 0, 1, math.Pi, 0, false) },
			"M-1,0A1,1,0,1,1,1,0"},
		{"offset center",
			func(p *path.Path) *path.Path { return p.Arc(10, 20, 2, 0, math.Pi/2, false) },
			"M12,20A2,2,0,0,1,10,22"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.draw(path.NewWithPrecision(3)).String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestArcTo(t *testing.T) {
	tests := []struct {
		name string
		draw func(p *path.Path) *path.Path
		want string
	}{
		{"on an empty path it just moves",
			func(p *path.Path) *path.Path { return p.ArcTo(1, 2, 3, 4, 5) },
			"M1,2"},
		{"corner coincident with the current point draws nothing",
			func(p *path.Path) *path.Path { return p.MoveTo(1, 2).ArcTo(1, 2, 3, 4, 5) },
			"M1,2"},
		{"collinear points degrade to a line",
			func(p *path.Path) *path.Path { return p.MoveTo(0, 0).ArcTo(1, 0, 2, 0, 1) },
			"M0,0L1,0"},
		{"repeated end point degrades to a line",
			func(p *path.Path) *path.Path { return p.MoveTo(0, 0).ArcTo(1, 0, 1, 0, 1) },
			"M0,0L1,0"},
		{"zero radius degrades to a line",
			func(p *path.Path) *path.Path { return p.MoveTo(0, 0).ArcTo(1, 0, 1, 1, 0) },
			"M0,0L1,0"},
		{"negative radius is clamped to zero rather than panicking",
			func(p *path.Path) *path.Path { return p.MoveTo(0, 0).ArcTo(1, 0, 1, 1, -1) },
			"M0,0L1,0"},
		{"right-angle fillet of radius 1",
			// From (0,0) toward the corner (2,0) and on to (2,2): the inscribed
			// circle of radius 1 touches the legs 1 unit back from the corner,
			// so a line to (1,0) then an arc to (2,1).
			func(p *path.Path) *path.Path { return p.MoveTo(0, 0).ArcTo(2, 0, 2, 2, 1) },
			"M0,0L1,0A1,1,0,0,1,2,1"},
		{"tangent point already reached emits no line",
			func(p *path.Path) *path.Path { return p.MoveTo(1, 0).ArcTo(2, 0, 2, 2, 1) },
			"M1,0A1,1,0,0,1,2,1"},
		{"the sweep flag follows the turn direction",
			// Mirror of the previous corner: turning the other way must flip
			// the sweep flag to 0.
			func(p *path.Path) *path.Path { return p.MoveTo(0, 0).ArcTo(2, 0, 2, -2, 1) },
			"M0,0L1,0A1,1,0,0,0,2,-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.draw(path.NewWithPrecision(6)).String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNonFiniteFormatting(t *testing.T) {
	got := path.New().MoveTo(math.NaN(), math.Inf(1)).LineTo(math.Inf(-1), 0).String()
	want := "MNaN,InfinityL-Infinity,0"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestChaining documents that every builder method returns the receiver, so a
// whole shape is one expression.
func TestChaining(t *testing.T) {
	p := path.New()
	if p.MoveTo(0, 0) != p || p.LineTo(1, 1) != p || p.ClosePath() != p ||
		p.Rect(0, 0, 1, 1) != p || p.Arc(0, 0, 1, 0, 1, false) != p ||
		p.ArcTo(1, 1, 2, 2, 1) != p || p.QuadraticCurveTo(0, 0, 1, 1) != p ||
		p.BezierCurveTo(0, 0, 1, 1, 2, 2) != p {
		t.Fatal("builder methods must return the receiver")
	}
	if !strings.HasPrefix(p.String(), "M0,0L1,1Z") {
		t.Errorf("unexpected accumulation: %q", p.String())
	}
}
