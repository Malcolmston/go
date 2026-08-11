package ease

import (
	"math"
	"testing"
)

// allEasings is the single source of truth for the package-wide invariant
// tests. Every exported easing must appear here; the endpoint test below is the
// cheapest and strongest correctness check this package has, and it is only as
// good as this table is complete.
var allEasings = []struct {
	name string
	fn   Ease
	// monotone reports whether the easing is non-decreasing on [0, 1]. Bounce
	// is not (it rebounds), and Back and Elastic are not (they overshoot).
	monotone bool
	// bounded reports whether the easing stays within [0, 1] on [0, 1]. Back
	// and Elastic deliberately do not.
	bounded bool
}{
	{"Linear", Linear, true, true},
	{"QuadIn", QuadIn, true, true},
	{"QuadOut", QuadOut, true, true},
	{"QuadInOut", QuadInOut, true, true},
	{"CubicIn", CubicIn, true, true},
	{"CubicOut", CubicOut, true, true},
	{"CubicInOut", CubicInOut, true, true},
	{"PolyIn", PolyIn, true, true},
	{"PolyOut", PolyOut, true, true},
	{"PolyInOut", PolyInOut, true, true},
	{"SinIn", SinIn, true, true},
	{"SinOut", SinOut, true, true},
	{"SinInOut", SinInOut, true, true},
	{"ExpIn", ExpIn, true, true},
	{"ExpOut", ExpOut, true, true},
	{"ExpInOut", ExpInOut, true, true},
	{"CircleIn", CircleIn, true, true},
	{"CircleOut", CircleOut, true, true},
	{"CircleInOut", CircleInOut, true, true},
	{"ElasticIn", ElasticIn, false, false},
	{"ElasticOut", ElasticOut, false, false},
	{"ElasticInOut", ElasticInOut, false, false},
	{"BackIn", BackIn, false, false},
	{"BackOut", BackOut, false, false},
	{"BackInOut", BackInOut, false, false},
	{"BounceIn", BounceIn, false, true},
	{"BounceOut", BounceOut, false, true},
	{"BounceInOut", BounceInOut, false, true},

	// The parameterised constructors at their defaults must behave identically,
	// so they participate in the same invariants.
	{"PolyInWith(3)", PolyInWith(DefaultExponent), true, true},
	{"PolyOutWith(3)", PolyOutWith(DefaultExponent), true, true},
	{"PolyInOutWith(3)", PolyInOutWith(DefaultExponent), true, true},
	{"PolyInWith(1)", PolyInWith(1), true, true},
	{"PolyOutWith(6)", PolyOutWith(6), true, true},
	{"PolyInOutWith(0.5)", PolyInOutWith(0.5), true, true},
	{"ElasticInWith(2,0.5)", ElasticInWith(2, 0.5), false, false},
	{"ElasticOutWith(2,0.5)", ElasticOutWith(2, 0.5), false, false},
	{"ElasticInOutWith(2,0.5)", ElasticInOutWith(2, 0.5), false, false},
	{"BackInWith(3)", BackInWith(3), false, false},
	{"BackOutWith(3)", BackOutWith(3), false, false},
	{"BackInOutWith(3)", BackInOutWith(3), false, false},
}

// TestEndpoints is the headline invariant: every easing pins both ends of the
// unit interval exactly. Equality is deliberately exact rather than
// approximate — d3's implementations are shaped to make it so, and a tolerance
// here would hide the very bugs (SinIn's cosine, tpmt's scaling constant) that
// this test exists to catch.
func TestEndpoints(t *testing.T) {
	for _, tc := range allEasings {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.fn(0); got != 0 {
				t.Errorf("f(0) = %v, want exactly 0", got)
			}
			if got := tc.fn(1); got != 1 {
				t.Errorf("f(1) = %v, want exactly 1", got)
			}
		})
	}
}

// inOutEasings are the symmetric variants: each must satisfy
// f(0.5+x) + f(0.5-x) == 1 for every x, and therefore f(0.5) == 0.5.
var inOutEasings = []struct {
	name string
	fn   Ease
}{
	{"QuadInOut", QuadInOut},
	{"CubicInOut", CubicInOut},
	{"PolyInOut", PolyInOut},
	{"PolyInOutWith(2)", PolyInOutWith(2)},
	{"PolyInOutWith(5)", PolyInOutWith(5)},
	{"SinInOut", SinInOut},
	{"ExpInOut", ExpInOut},
	{"CircleInOut", CircleInOut},
	{"ElasticInOut", ElasticInOut},
	{"ElasticInOutWith(2,0.5)", ElasticInOutWith(2, 0.5)},
	{"BackInOut", BackInOut},
	{"BackInOutWith(3)", BackInOutWith(3)},
	{"BounceInOut", BounceInOut},
}

func TestInOutSymmetry(t *testing.T) {
	const tol = 1e-12
	for _, tc := range inOutEasings {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i <= 500; i++ {
				x := float64(i) / 1000 // x in [0, 0.5]
				lo, hi := tc.fn(0.5-x), tc.fn(0.5+x)
				if math.Abs(lo+hi-1) > tol {
					t.Fatalf("f(%v)+f(%v) = %v, want 1", 0.5-x, 0.5+x, lo+hi)
				}
			}
		})
	}
}

func TestInOutMidpoint(t *testing.T) {
	const tol = 1e-15
	for _, tc := range inOutEasings {
		if got := tc.fn(0.5); math.Abs(got-0.5) > tol {
			t.Errorf("%s(0.5) = %v, want 0.5", tc.name, got)
		}
	}
}

func TestMonotonicity(t *testing.T) {
	for _, tc := range allEasings {
		if !tc.monotone {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			prev := tc.fn(0)
			for i := 1; i <= 1000; i++ {
				cur := tc.fn(float64(i) / 1000)
				if cur < prev-1e-15 {
					t.Fatalf("not monotone at t=%v: %v then %v", float64(i)/1000, prev, cur)
				}
				prev = cur
			}
		})
	}
}

func TestBounds(t *testing.T) {
	for _, tc := range allEasings {
		if !tc.bounded {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i <= 1000; i++ {
				v := tc.fn(float64(i) / 1000)
				if v < -1e-15 || v > 1+1e-15 {
					t.Fatalf("f(%v) = %v, outside [0,1]", float64(i)/1000, v)
				}
			}
		})
	}
}

// TestOvershoot asserts the documented misbehaviour: Back and Elastic leave the
// unit interval on purpose. If a future change accidentally clamped them, the
// package would silently stop matching d3, and no other test here would notice.
func TestOvershoot(t *testing.T) {
	tests := []struct {
		name         string
		fn           Ease
		wantBelowMin bool // dips below 0 somewhere in (0, 1)
		wantAboveMax bool // rises above 1 somewhere in (0, 1)
	}{
		{"BackIn", BackIn, true, false},
		{"BackOut", BackOut, false, true},
		{"BackInOut", BackInOut, true, true},
		{"ElasticIn", ElasticIn, true, false},
		{"ElasticOut", ElasticOut, false, true},
		{"ElasticInOut", ElasticInOut, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var below, above bool
			for i := 1; i < 1000; i++ {
				v := tc.fn(float64(i) / 1000)
				if v < 0 {
					below = true
				}
				if v > 1 {
					above = true
				}
			}
			if below != tc.wantBelowMin {
				t.Errorf("dips below 0 = %v, want %v", below, tc.wantBelowMin)
			}
			if above != tc.wantAboveMax {
				t.Errorf("rises above 1 = %v, want %v", above, tc.wantAboveMax)
			}
		})
	}
}

// TestExactValues covers the handful of points whose value is derivable by hand
// from the definition, so the formulas themselves — not just their endpoints —
// are pinned.
func TestExactValues(t *testing.T) {
	sqrt2over2 := math.Sqrt2 / 2
	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{"Linear(0.25)", Linear(0.25), 0.25},
		{"QuadIn(0.5)", QuadIn(0.5), 0.25},
		{"QuadOut(0.5)", QuadOut(0.5), 0.75},
		{"CubicIn(0.5)", CubicIn(0.5), 0.125},
		{"CubicOut(0.5)", CubicOut(0.5), 0.875},
		{"PolyIn(0.5)", PolyIn(0.5), 0.125},
		{"PolyInWith(2)(0.5)", PolyInWith(2)(0.5), 0.25},
		{"PolyInWith(1)(0.3)", PolyInWith(1)(0.3), 0.3},
		{"SinOut(0.5)", SinOut(0.5), sqrt2over2},
		{"SinIn(0.5)", SinIn(0.5), 1 - sqrt2over2},
		{"SinInOut(0.25)", SinInOut(0.25), (1 - sqrt2over2) / 2},
		{"CircleIn(1)", CircleIn(1), 1},
		{"CircleOut(0.6)", CircleOut(0.6), math.Sqrt(1 - 0.16)},
		// The first bounce segment peaks at exactly 1 at t = 4/11.
		{"BounceOut(4/11)", BounceOut(4.0 / 11), 1},
		// ...and lands on the plateau b4 = 3/4 at t = 6/11.
		{"BounceOut(6/11)", BounceOut(6.0 / 11), 0.75},
		{"BounceIn(7/11)", BounceIn(7.0 / 11), 1 - BounceOut(4.0/11)},
		// tpmt is the exponential family's building block; both ends are exact.
		{"tpmt(0)", tpmt(0), 1},
		{"tpmt(1)", tpmt(1), 0},
	}
	for _, tc := range tests {
		if math.Abs(tc.got-tc.want) > 1e-15 {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

// TestDefaultsMatchConstructors checks that each plain easing really is its
// "With" counterpart at the documented default, which is the contract the
// package doc advertises.
func TestDefaultsMatchConstructors(t *testing.T) {
	tests := []struct {
		name  string
		plain Ease
		with  Ease
	}{
		{"PolyIn", PolyIn, PolyInWith(DefaultExponent)},
		{"PolyOut", PolyOut, PolyOutWith(DefaultExponent)},
		{"PolyInOut", PolyInOut, PolyInOutWith(DefaultExponent)},
		{"BackIn", BackIn, BackInWith(DefaultOvershoot)},
		{"BackOut", BackOut, BackOutWith(DefaultOvershoot)},
		{"BackInOut", BackInOut, BackInOutWith(DefaultOvershoot)},
		{"ElasticIn", ElasticIn, ElasticInWith(DefaultAmplitude, DefaultPeriod)},
		{"ElasticOut", ElasticOut, ElasticOutWith(DefaultAmplitude, DefaultPeriod)},
		{"ElasticInOut", ElasticInOut, ElasticInOutWith(DefaultAmplitude, DefaultPeriod)},
		// The cubic easings are the polynomial family at exponent 3.
		{"CubicIn", CubicIn, PolyInWith(3)},
		{"CubicOut", CubicOut, PolyOutWith(3)},
		{"CubicInOut", CubicInOut, PolyInOutWith(3)},
		{"QuadIn", QuadIn, PolyInWith(2)},
		{"QuadOut", QuadOut, PolyOutWith(2)},
		{"QuadInOut", QuadInOut, PolyInOutWith(2)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i <= 100; i++ {
				x := float64(i) / 100
				a, b := tc.plain(x), tc.with(x)
				if math.Abs(a-b) > 1e-12 {
					t.Fatalf("at t=%v: %v vs %v", x, a, b)
				}
			}
		})
	}
}

// TestOutIsMirrorOfIn checks the defining relation out(t) = 1 - in(1-t) for the
// families where d3 defines it that way.
func TestOutIsMirrorOfIn(t *testing.T) {
	tests := []struct {
		name    string
		in, out Ease
	}{
		{"Quad", QuadIn, QuadOut},
		{"Cubic", CubicIn, CubicOut},
		{"Poly", PolyIn, PolyOut},
		{"Sin", SinIn, SinOut},
		{"Exp", ExpIn, ExpOut},
		{"Circle", CircleIn, CircleOut},
		{"Bounce", BounceIn, BounceOut},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i <= 100; i++ {
				x := float64(i) / 100
				if got, want := tc.out(x), 1-tc.in(1-x); math.Abs(got-want) > 1e-12 {
					t.Fatalf("out(%v) = %v, want 1-in(1-t) = %v", x, got, want)
				}
			}
		})
	}
}

// TestNotClamped documents that easings extrapolate outside [0, 1] rather than
// saturating, and that [Ease.Clamped] is the opt-in fix.
func TestNotClamped(t *testing.T) {
	if got := CubicIn(2); got != 8 {
		t.Errorf("CubicIn(2) = %v, want 8 (unclamped extrapolation)", got)
	}
	if got := QuadIn(-1); got != 1 {
		t.Errorf("QuadIn(-1) = %v, want 1 (unclamped extrapolation)", got)
	}

	clamped := Ease(CubicIn).Clamped()
	if got := clamped(2); got != 1 {
		t.Errorf("Clamped CubicIn(2) = %v, want 1", got)
	}
	if got := clamped(-1); got != 0 {
		t.Errorf("Clamped CubicIn(-1) = %v, want 0", got)
	}
	if got := clamped(0.5); got != CubicIn(0.5) {
		t.Errorf("Clamped CubicIn(0.5) = %v, want %v", got, CubicIn(0.5))
	}
}

// TestElasticAmplitudeFloor covers d3's rule that amplitudes below 1 are raised
// to 1: a smaller amplitude could not reach the endpoint, so the curve would
// stop short and the endpoint invariant would break.
func TestElasticAmplitudeFloor(t *testing.T) {
	small := ElasticOutWith(0.5, DefaultPeriod)
	one := ElasticOutWith(1, DefaultPeriod)
	for i := 0; i <= 100; i++ {
		x := float64(i) / 100
		if got, want := small(x), one(x); got != want {
			t.Fatalf("amplitude 0.5 not floored to 1 at t=%v: %v vs %v", x, got, want)
		}
	}
}
