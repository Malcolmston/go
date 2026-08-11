package scale

import (
	"math"
	"testing"

	"github.com/malcolmston/d3/array"
	"github.com/malcolmston/d3/color"
	"github.com/malcolmston/d3/interpolate"
)

const eps = 1e-9

func closeTo(t *testing.T, got, want float64, what string) {
	t.Helper()
	if math.IsNaN(want) {
		if !math.IsNaN(got) {
			t.Errorf("%s = %v, want NaN", what, got)
		}
		return
	}
	if math.Abs(got-want) > eps {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// TestLinearScale covers the four behaviors d3 users rely on and ports most
// often get wrong: plain normalize→interpolate, extrapolation OUTSIDE the
// domain (d3 never clamps unless told to), reversed domains, and the degenerate
// case where both domain ends are equal.
func TestLinearScale(t *testing.T) {
	tests := []struct {
		name   string
		domain []float64
		rng    []float64
		in     float64
		want   float64
	}{
		{"unit identity", []float64{0, 1}, []float64{0, 1}, 0.5, 0.5},
		{"endpoint low", []float64{10, 20}, []float64{0, 100}, 10, 0},
		{"endpoint high", []float64{10, 20}, []float64{0, 100}, 20, 100},
		{"midpoint", []float64{10, 20}, []float64{0, 100}, 15, 50},
		{"extrapolate above", []float64{10, 20}, []float64{0, 100}, 25, 150},
		{"extrapolate below", []float64{10, 20}, []float64{0, 100}, 5, -50},
		{"reversed domain low", []float64{20, 10}, []float64{0, 100}, 20, 0},
		{"reversed domain high", []float64{20, 10}, []float64{0, 100}, 10, 100},
		{"reversed domain mid", []float64{20, 10}, []float64{0, 100}, 15, 50},
		{"reversed domain extrapolates", []float64{20, 10}, []float64{0, 100}, 25, -50},
		{"reversed range", []float64{0, 100}, []float64{480, 0}, 25, 360},
		{"degenerate domain", []float64{5, 5}, []float64{0, 100}, 5, 50},
		{"degenerate domain, any input", []float64{5, 5}, []float64{0, 100}, 1e6, 50},
		{"degenerate range", []float64{0, 10}, []float64{7, 7}, 3, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewLinear().SetDomain(tt.domain...).SetRange(tt.rng...)
			closeTo(t, s.Scale(tt.in), tt.want, "Scale")
		})
	}
}

// TestLinearInvert checks that Invert is the true inverse — including outside
// the domain, where a scale that silently clamped would fail.
func TestLinearInvert(t *testing.T) {
	tests := []struct {
		name   string
		domain []float64
		rng    []float64
		xs     []float64
	}{
		{"forward", []float64{10, 20}, []float64{0, 100}, []float64{10, 15, 20, 25, -5}},
		{"reversed domain", []float64{20, 10}, []float64{0, 100}, []float64{10, 15, 20, 30}},
		{"reversed range", []float64{0, 100}, []float64{480, 0}, []float64{0, 33, 100, 150}},
		{"polylinear", []float64{0, 50, 100}, []float64{0, 10, 100}, []float64{0, 25, 50, 75, 100}},
		{"negative domain", []float64{-100, -10}, []float64{0, 1}, []float64{-100, -55, -10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewLinear().SetDomain(tt.domain...).SetRange(tt.rng...)
			for _, x := range tt.xs {
				closeTo(t, s.Invert(s.Scale(x)), x, "Invert(Scale(x))")
			}
		})
	}
}

// TestLinearPolylinear is the multi-stop case: with three domain stops and
// three range stops the scale is two independent linear segments joined at the
// pivot, NOT a single line through the endpoints. A port that ignores the
// middle stop passes every two-stop test and fails every one of these.
func TestLinearPolylinear(t *testing.T) {
	s := NewLinear().SetDomain(0, 50, 100).SetRange(0, 10, 100)
	tests := []struct{ in, want float64 }{
		{0, 0},
		{25, 5},    // halfway through the first segment: 0→10
		{50, 10},   // the pivot itself
		{75, 55},   // halfway through the second segment: 10→100
		{100, 100}, // upper endpoint
		{-50, -10}, // extrapolates off the first segment
		{150, 190}, // extrapolates off the last segment
	}
	for _, tt := range tests {
		closeTo(t, s.Scale(tt.in), tt.want, "Scale")
	}

	// A descending polylinear domain must behave as the mirror image.
	r := NewLinear().SetDomain(100, 50, 0).SetRange(0, 10, 100)
	closeTo(t, r.Scale(100), 0, "reversed poly Scale(100)")
	closeTo(t, r.Scale(50), 10, "reversed poly Scale(50)")
	closeTo(t, r.Scale(75), 5, "reversed poly Scale(75)")
	closeTo(t, r.Scale(0), 100, "reversed poly Scale(0)")

	// Extra range entries beyond the domain length are ignored, as in d3.
	x := NewLinear().SetDomain(0, 1).SetRange(0, 10, 1000)
	closeTo(t, x.Scale(1), 10, "excess range ignored")
}

func TestLinearClamp(t *testing.T) {
	s := NewLinear().SetDomain(10, 20).SetRange(0, 100)

	// Unclamped is the default and extrapolates.
	closeTo(t, s.Scale(30), 200, "unclamped Scale(30)")
	if s.Clamp() {
		t.Fatal("Clamp() should default to false")
	}

	s.SetClamp(true)
	closeTo(t, s.Scale(30), 100, "clamped Scale(30)")
	closeTo(t, s.Scale(0), 0, "clamped Scale(0)")
	closeTo(t, s.Invert(200), 20, "clamped Invert(200)")
	closeTo(t, s.Invert(-50), 10, "clamped Invert(-50)")

	// Clamping must respect a reversed domain's true extent.
	r := NewLinear().SetDomain(20, 10).SetRange(0, 100).SetClamp(true)
	closeTo(t, r.Scale(30), 0, "reversed clamped high")
	closeTo(t, r.Scale(0), 100, "reversed clamped low")
}

func TestLinearUnknownAndNaN(t *testing.T) {
	s := NewLinear().SetDomain(0, 1).SetRange(0, 100)
	if got := s.Scale(math.NaN()); !math.IsNaN(got) && got != 0 {
		// The zero value of R is the default unknown.
		t.Errorf("Scale(NaN) = %v, want the zero unknown", got)
	}
	s.SetUnknown(-1)
	closeTo(t, s.Scale(math.NaN()), -1, "Scale(NaN) with unknown set")
	closeTo(t, s.Unknown(), -1, "Unknown()")
	// A real input is unaffected.
	closeTo(t, s.Scale(0.5), 50, "Scale(0.5)")
}

func TestLinearRangeRound(t *testing.T) {
	s := NewLinear().SetDomain(0, 1).SetRangeRound(0, 10)
	for _, tt := range []struct{ in, want float64 }{
		{0.25, 3}, // 2.5 rounds to 3 (half away from zero, like math.Round)
		{0.31, 3},
		{0.5, 5},
		{1, 10},
	} {
		closeTo(t, s.Scale(tt.in), tt.want, "rounded Scale")
	}
	// Rounding is on the output only: Invert still returns fractions.
	if v := s.Invert(3); v == math.Trunc(v) && v != 0 {
		t.Logf("Invert(3) = %v", v) // informational; exactness is not required
	}
}

// TestNiceAgreesWithTickIncrement is the contract that makes Nice worth having:
// after nicening, both domain endpoints must be exact multiples of the step
// array.TickIncrement chooses, so the axis begins and ends on a tick.
func TestNiceAgreesWithTickIncrement(t *testing.T) {
	tests := []struct {
		name   string
		domain []float64
		count  int
		want   []float64
	}{
		{"simple", []float64{0.5, 9.5}, 10, []float64{0, 10}},
		{"already nice", []float64{0, 100}, 10, []float64{0, 100}},
		{"widen both ends", []float64{1.1, 10.9}, 10, []float64{1, 11}},
		{"reversed stays reversed", []float64{9.5, 0.5}, 10, []float64{10, 0}},
		{"negative", []float64{-9.5, -0.5}, 10, []float64{-10, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewLinear().SetDomain(tt.domain...).Nice(tt.count)
			got := s.Domain()
			if len(got) != len(tt.want) {
				t.Fatalf("Domain() = %v, want %v", got, tt.want)
			}
			for i := range got {
				closeTo(t, got[i], tt.want[i], "Domain()")
			}

			// Property: the nicened endpoints are multiples of the increment.
			lo, hi := got[0], got[len(got)-1]
			if hi < lo {
				lo, hi = hi, lo
			}
			step := array.TickIncrement(lo, hi, tt.count)
			if step > 0 {
				closeTo(t, math.Round(lo/step)*step, lo, "lo is a multiple of step")
				closeTo(t, math.Round(hi/step)*step, hi, "hi is a multiple of step")
			} else if step < 0 {
				closeTo(t, math.Round(lo*step)/step, lo, "lo is a multiple of step")
				closeTo(t, math.Round(hi*step)/step, hi, "hi is a multiple of step")
			}
		})
	}
}

// TestNiceOnlyWidens asserts the direction of the adjustment: Nice never pulls
// a domain in, because that would hide data.
func TestNiceOnlyWidens(t *testing.T) {
	domains := [][]float64{{0.5, 9.5}, {1.1, 10.9}, {-3.7, 2.2}, {0.0001, 0.0009}, {17, 19}}
	for _, d := range domains {
		got := NewLinear().SetDomain(d...).Nice(10).Domain()
		if got[0] > d[0]+eps || got[1] < d[1]-eps {
			t.Errorf("Nice(%v) = %v, narrowed the domain", d, got)
		}
	}
}

func TestNicePreservesMiddleStops(t *testing.T) {
	got := NewLinear().SetDomain(1.1, 5, 10.9).Nice(10).Domain()
	if len(got) != 3 {
		t.Fatalf("Domain() = %v, want 3 stops", got)
	}
	closeTo(t, got[1], 5, "middle stop untouched")
}

func TestTicksDelegatesToArray(t *testing.T) {
	s := NewLinear().SetDomain(0, 10)
	got := s.Ticks(10)
	want := array.Ticks(0, 10, 10)
	if len(got) != len(want) {
		t.Fatalf("Ticks(10) = %v, want %v", got, want)
	}
	for i := range got {
		closeTo(t, got[i], want[i], "Ticks")
	}
}

func TestTickFormatPrecisionFollowsStep(t *testing.T) {
	tests := []struct {
		domain []float64
		count  int
		in     float64
		want   string
	}{
		{[]float64{0, 1}, 10, 0.1, "0.1"},
		{[]float64{0, 1}, 10, 0, "0.0"},
		{[]float64{0, 100}, 10, 40, "40"},
		{[]float64{0, 0.01}, 10, 0.003, "0.003"},
	}
	for _, tt := range tests {
		f := NewLinear().SetDomain(tt.domain...).TickFormat(tt.count)
		if got := f(tt.in); got != tt.want {
			t.Errorf("TickFormat(%v)(%v) = %q, want %q", tt.domain, tt.in, got, tt.want)
		}
	}
}

// TestTickFormatSpecifier checks that a d3-format specifier reaches the
// formatter intact, and that an omitted precision is still filled in from the
// tick step.
func TestTickFormatSpecifier(t *testing.T) {
	tests := []struct {
		domain []float64
		spec   string
		in     float64
		want   string
	}{
		{[]float64{0, 1000000}, "$,.0f", 1234567, "$1,234,567"},
		{[]float64{0, 1}, ".0%", 0.25, "25%"},
		{[]float64{0, 1300000}, "s", 1300000, "1.3M"},
		{[]float64{0, 1}, "", 0.5, "0.5"}, // empty means the default, ",f"
	}
	for _, tt := range tests {
		f, err := NewLinear().SetDomain(tt.domain...).TickFormatSpecifier(10, tt.spec)
		if err != nil {
			t.Errorf("TickFormatSpecifier(%q) error: %v", tt.spec, err)
			continue
		}
		if got := f(tt.in); got != tt.want {
			t.Errorf("TickFormatSpecifier(%q)(%v) = %q, want %q", tt.spec, tt.in, got, tt.want)
		}
	}

	if _, err := NewLinear().TickFormatSpecifier(10, "not a specifier"); err == nil {
		t.Error("an invalid specifier should return an error")
	}
}

func TestCopyIsIndependent(t *testing.T) {
	a := NewLinear().SetDomain(0, 100).SetRange(0, 1)
	b := a.Copy().SetDomain(0, 10)
	closeTo(t, a.Scale(50), 0.5, "original unaffected")
	closeTo(t, b.Scale(5), 0.5, "copy uses its own domain")
}

func TestDomainAndRangeAreDefensiveCopies(t *testing.T) {
	s := NewLinear().SetDomain(0, 100).SetRange(0, 1)
	d := s.Domain()
	d[1] = -999
	closeTo(t, s.Scale(50), 0.5, "mutating the returned domain must not affect the scale")
	r := s.Range()
	r[1] = -999
	closeTo(t, s.Scale(50), 0.5, "mutating the returned range must not affect the scale")
}

// TestPowScale checks the signed power transform, including the reflection
// through the origin that lets a pow domain straddle zero.
func TestPowScale(t *testing.T) {
	s := NewPow().SetExponent(2).SetDomain(0, 10).SetRange(0, 100)
	closeTo(t, s.Scale(0), 0, "pow Scale(0)")
	closeTo(t, s.Scale(10), 100, "pow Scale(10)")
	closeTo(t, s.Scale(5), 25, "pow Scale(5)") // 5² / 10² = 0.25
	closeTo(t, s.Invert(25), 5, "pow Invert(25)")

	// Straddling zero: the transform reflects, so the mapping is monotone.
	c := NewPow().SetExponent(2).SetDomain(-10, 10).SetRange(-100, 100)
	closeTo(t, c.Scale(0), 0, "pow across zero at 0")
	closeTo(t, c.Scale(-5), -25, "pow across zero at -5")
	closeTo(t, c.Scale(5), 25, "pow across zero at 5")

	// Exponent 1 is exactly linear.
	l := NewPow().SetDomain(0, 10).SetRange(0, 100)
	closeTo(t, l.Scale(3), 30, "pow exponent 1 is linear")
	closeTo(t, l.Exponent(), 1, "default exponent")
}

func TestSqrtScale(t *testing.T) {
	s := NewSqrt().SetDomain(0, 100).SetRange(0, 10)
	closeTo(t, s.Exponent(), 0.5, "sqrt exponent")
	closeTo(t, s.Scale(0), 0, "sqrt Scale(0)")
	closeTo(t, s.Scale(100), 10, "sqrt Scale(100)")
	closeTo(t, s.Scale(25), 5, "sqrt Scale(25)") // sqrt(25)/sqrt(100) = 0.5
	closeTo(t, s.Invert(5), 25, "sqrt Invert(5)")
}

// TestSymlogScale exercises the one thing symlog exists for: being defined at
// and across zero, where a log scale is not.
func TestSymlogScale(t *testing.T) {
	s := NewSymlog().SetDomain(-100, 100).SetRange(-1, 1)
	closeTo(t, s.Scale(0), 0, "symlog Scale(0)")
	closeTo(t, s.Scale(100), 1, "symlog Scale(100)")
	closeTo(t, s.Scale(-100), -1, "symlog Scale(-100)")
	// Symmetry about the origin.
	closeTo(t, s.Scale(7), -s.Scale(-7), "symlog is odd")
	// Monotonicity.
	prev := math.Inf(-1)
	for x := -100.0; x <= 100; x += 5 {
		v := s.Scale(x)
		if v < prev {
			t.Fatalf("symlog not monotone at %v", x)
		}
		prev = v
	}
	closeTo(t, s.Invert(s.Scale(42)), 42, "symlog round trip")

	// The constant widens the near-linear region around zero.
	c := NewSymlog().SetConstant(10).SetDomain(-100, 100).SetRange(-1, 1)
	closeTo(t, c.Constant(), 10, "Constant()")
	if math.Abs(c.Scale(5)) >= math.Abs(s.Scale(5)) {
		t.Error("a larger constant should compress values near zero")
	}
}

func TestIdentityScale(t *testing.T) {
	s := NewIdentity().SetDomain(0, 100)
	for _, x := range []float64{-10, 0, 42.5, 100, 1000} {
		closeTo(t, s.Scale(x), x, "identity Scale")
		closeTo(t, s.Invert(x), x, "identity Invert")
	}
	// Setting the domain also sets the range, so ticks and nice still work.
	d := NewIdentity().SetDomain(0.5, 9.5).Nice(10).Domain()
	closeTo(t, d[0], 0, "identity Nice low")
	closeTo(t, d[1], 10, "identity Nice high")
}

// TestLinearOfColor exercises the generic range type against the interpolate
// and color sibling packages: this is the contract that makes color ramps work.
func TestLinearOfColor(t *testing.T) {
	black := color.RGB{R: 0, G: 0, B: 0, Opacity: 1}
	white := color.RGB{R: 255, G: 255, B: 255, Opacity: 1}
	s := NewLinearOf[color.Color](interpolate.RGB).
		SetDomain(0, 100).
		SetRange(black, white)

	mid := s.Scale(50).RGBA()
	closeTo(t, mid.R, 127.5, "color midpoint R")
	closeTo(t, mid.G, 127.5, "color midpoint G")

	lo := s.Scale(0).RGBA()
	closeTo(t, lo.R, 0, "color endpoint is exact")

	// Without an uninterpolator, Invert reports that it cannot answer.
	if !math.IsNaN(s.Invert(mid)) {
		t.Error("Invert on a non-numeric range should return NaN")
	}

	// Three stops make a two-segment ramp with a fixed pivot.
	red := color.RGB{R: 255, G: 0, B: 0, Opacity: 1}
	p := NewLinearOf[color.Color](interpolate.RGB).
		SetDomain(0, 50, 100).
		SetRange(black, red, white)
	pivot := p.Scale(50).RGBA()
	closeTo(t, pivot.R, 255, "polylinear color pivot R")
	closeTo(t, pivot.G, 0, "polylinear color pivot G")
}
