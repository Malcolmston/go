package interpolate

import (
	"math"
	"testing"
)

// TestBasisEndpoints checks the pinning: Basis duplicates the first and last
// control values so the spline passes exactly through them, unlike its interior
// values.
func TestBasisEndpoints(t *testing.T) {
	cases := [][]float64{
		{0, 1},
		{0, 1, 2, 3},
		{5, -3, 12, 0, 7},
		{1, 1, 1},
	}
	for _, values := range cases {
		f := Basis(values)
		if got, want := f(0), values[0]; math.Abs(got-want) > 1e-12 {
			t.Errorf("Basis(%v)(0) = %v, want %v", values, got, want)
		}
		if got, want := f(1), values[len(values)-1]; math.Abs(got-want) > 1e-12 {
			t.Errorf("Basis(%v)(1) = %v, want %v", values, got, want)
		}
	}
}

// TestBasisClampsT documents the one place the package deliberately clamps: a
// cubic has no meaningful continuation past its control polygon.
func TestBasisClampsT(t *testing.T) {
	f := Basis([]float64{0, 1, 2, 3})
	if got := f(-1); got != 0 {
		t.Errorf("Basis(-1) = %v, want the clamped 0", got)
	}
	if got := f(2); got != 3 {
		t.Errorf("Basis(2) = %v, want the clamped 3", got)
	}
	if got := f(math.NaN()); got != 0 {
		t.Errorf("Basis(NaN) = %v, want 0", got)
	}
}

// TestBasisSmoothAndMonotone checks that a spline over an increasing control
// sequence is itself increasing and free of jumps — the smoothness that makes
// it preferable to a polyline for a color ramp.
func TestBasisSmoothAndMonotone(t *testing.T) {
	f := Basis([]float64{0, 1, 2, 3, 4})
	prev := f(0)
	for x := 0.001; x <= 1; x += 0.001 {
		v := f(x)
		if v < prev-1e-12 {
			t.Fatalf("Basis is not monotone at t=%v: %v after %v", x, v, prev)
		}
		if v-prev > 0.05 {
			t.Fatalf("Basis jumped at t=%v: %v after %v", x, v, prev)
		}
		prev = v
	}
}

// TestBasisLinearThroughLinearControls: for evenly spaced control values, the
// uniform B-spline reproduces the straight line exactly, because the basis
// weights sum to 1 and are symmetric. That is a real identity, not a fitted
// number, which is why it is safe to assert exactly.
func TestBasisLinearThroughLinearControls(t *testing.T) {
	values := []float64{0, 1, 2, 3, 4}
	f := Basis(values)
	for x := 0.0; x <= 1.0001; x += 0.05 {
		want := x * 4
		if got := f(x); math.Abs(got-want) > 1e-9 {
			t.Errorf("Basis over a linear ramp at t=%v = %v, want %v", x, got, want)
		}
	}
}

func TestBasisConstant(t *testing.T) {
	f := Basis([]float64{7})
	for _, x := range []float64{-1, 0, 0.5, 1, 2} {
		if got := f(x); got != 7 {
			t.Errorf("single-value Basis(%v) = %v, want 7", x, got)
		}
	}
	// A constant control sequence gives a constant curve.
	g := Basis([]float64{3, 3, 3, 3})
	for x := 0.0; x <= 1; x += 0.1 {
		if math.Abs(g(x)-3) > 1e-12 {
			t.Errorf("constant Basis(%v) = %v, want 3", x, g(x))
		}
	}
}

// TestBasisClosedIsPeriodic checks the property that makes BasisClosed the
// right primitive for a cyclic hue ramp: f(0) == f(1), and t wraps rather than
// clamping.
func TestBasisClosedIsPeriodic(t *testing.T) {
	f := BasisClosed([]float64{0, 1, 2, 3})
	if got, want := f(0), f(1); math.Abs(got-want) > 1e-12 {
		t.Errorf("BasisClosed(0) = %v but BasisClosed(1) = %v; the curve is not closed", got, want)
	}
	for _, x := range []float64{0.1, 0.25, 0.5, 0.77} {
		for _, k := range []float64{-2, -1, 1, 2, 5} {
			if got, want := f(x+k), f(x); math.Abs(got-want) > 1e-12 {
				t.Errorf("BasisClosed(%v) = %v, want f(%v) = %v (period 1)", x+k, got, x, want)
			}
		}
	}
}

// TestBasisClosedSmoothAcrossTheSeam checks that the wrap point is not a
// discontinuity — the reason to use this rather than Basis for a loop.
func TestBasisClosedSmoothAcrossTheSeam(t *testing.T) {
	f := BasisClosed([]float64{0, 10, 20, 10})
	const h = 1e-4
	before := f(1 - h)
	at := f(1)
	after := f(h)
	if math.Abs(at-before) > 1e-2 || math.Abs(after-at) > 1e-2 {
		t.Errorf("BasisClosed has a seam at t=1: %v -> %v -> %v", before, at, after)
	}
}

func TestBasisClosedConstant(t *testing.T) {
	f := BasisClosed([]float64{4})
	for _, x := range []float64{-1, 0, 0.5, 1, 2} {
		if got := f(x); got != 4 {
			t.Errorf("single-value BasisClosed(%v) = %v, want 4", x, got)
		}
	}
	g := BasisClosed([]float64{2, 2, 2})
	for x := 0.0; x <= 1; x += 0.1 {
		if math.Abs(g(x)-2) > 1e-12 {
			t.Errorf("constant BasisClosed(%v) = %v, want 2", x, g(x))
		}
	}
}

// TestBasisStaysInHull checks a defining property of a B-spline: the curve
// never leaves the range of its control values, so a color ramp built on it
// cannot overshoot into a color that was not asked for.
func TestBasisStaysInHull(t *testing.T) {
	values := []float64{5, -3, 12, 0, 7}
	lo, hi := values[0], values[0]
	for _, v := range values {
		lo = math.Min(lo, v)
		hi = math.Max(hi, v)
	}
	for _, f := range []func(float64) float64{Basis(values), BasisClosed(values)} {
		for x := -1.0; x <= 2; x += 0.005 {
			v := f(x)
			if v < lo-1e-9 || v > hi+1e-9 {
				t.Fatalf("spline left the control hull [%v, %v] at t=%v: %v", lo, hi, x, v)
			}
		}
	}
}

func TestBasisPanicsOnEmpty(t *testing.T) {
	for name, fn := range map[string]func([]float64) func(float64) float64{
		"Basis":       Basis,
		"BasisClosed": BasisClosed,
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s with no values did not panic", name)
				}
			}()
			fn(nil)
		}()
	}
}

// TestBasisCopiesInput checks that mutating the caller's slice afterwards does
// not change the interpolator, since the closure is built once and reused.
func TestBasisCopiesInput(t *testing.T) {
	values := []float64{0, 1, 2}
	f := Basis(values)
	before := f(0.5)
	values[1] = 100
	if after := f(0.5); after != before {
		t.Errorf("Basis aliased its input: %v became %v", before, after)
	}
}
