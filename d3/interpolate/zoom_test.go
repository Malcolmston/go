package interpolate

import (
	"math"
	"testing"
)

func TestZoomEndpoints(t *testing.T) {
	cases := [][2]ZoomView{
		{{0, 0, 100}, {500, 500, 50}},
		{{0, 0, 1}, {0, 0, 1000}},      // pure zoom in
		{{0, 0, 1000}, {0, 0, 1}},      // pure zoom out
		{{0, 0, 100}, {0, 0, 100}},     // identical
		{{10, 20, 5}, {10, 20, 5}},     // identical, offset
		{{-100, 50, 3}, {100, -50, 3}}, // pure pan
	}
	for _, c := range cases {
		f := Zoom(c[0], c[1])
		if got := f(0); got != c[0] {
			t.Errorf("Zoom(%v, %v)(0) = %v, want %v", c[0], c[1], got, c[0])
		}
		if got := f(1); got != c[1] {
			t.Errorf("Zoom(%v, %v)(1) = %v, want %v", c[0], c[1], got, c[1])
		}
	}
}

// TestZoomContinuousAtEndpoints checks that the exact endpoints are not a
// discontinuity papered over a diverging closed form: approaching t = 0 and
// t = 1 must converge on the returned values.
func TestZoomContinuousAtEndpoints(t *testing.T) {
	a := ZoomView{0, 0, 100}
	b := ZoomView{500, 500, 50}
	f := Zoom(a, b)
	const h = 1e-6
	near0 := f(h)
	near1 := f(1 - h)
	if math.Abs(near0.CX-a.CX) > 1e-2 || math.Abs(near0.Width-a.Width) > 1e-2 {
		t.Errorf("Zoom is discontinuous at 0: f(%v) = %v, f(0) = %v", h, near0, a)
	}
	if math.Abs(near1.CX-b.CX) > 1e-2 || math.Abs(near1.Width-b.Width) > 1e-2 {
		t.Errorf("Zoom is discontinuous at 1: f(%v) = %v, f(1) = %v", 1-h, near1, b)
	}
}

// TestZoomNoNaN checks that the closed form stays finite over and past the
// range, including the degenerate cases the implementation special-cases.
func TestZoomNoNaN(t *testing.T) {
	cases := []struct {
		a, b   ZoomView
		lo, hi float64 // the range of t to sweep
	}{
		{ZoomView{0, 0, 100}, ZoomView{500, 500, 50}, -0.5, 1.5},
		{ZoomView{0, 0, 1}, ZoomView{0, 0, 1000}, -0.5, 1.5},
		{ZoomView{0, 0, 100}, ZoomView{0, 0, 100}, -0.5, 1.5},
		// An extreme scale change: nine orders of magnitude in width across a
		// long pan. This is the case that goes NaN if the r terms are computed
		// as log(sqrt(b^2+1) - b) rather than as -asinh(b).
		{ZoomView{-1e5, 1e5, 1e-3}, ZoomView{1e5, -1e5, 1e6}, -0.5, 1.5},
	}
	for _, c := range cases {
		f := Zoom(c.a, c.b)
		for x := c.lo; x <= c.hi; x += 0.01 {
			v := f(x)
			if math.IsNaN(v.CX) || math.IsNaN(v.CY) || math.IsNaN(v.Width) ||
				math.IsInf(v.CX, 0) || math.IsInf(v.CY, 0) || math.IsInf(v.Width, 0) {
				t.Fatalf("Zoom(%v, %v)(%v) = %v is not finite", c.a, c.b, x, v)
			}
			if v.Width <= 0 {
				t.Fatalf("Zoom(%v, %v)(%v) width = %v, want positive", c.a, c.b, x, v.Width)
			}
		}
	}
}

// TestZoomPureZoomIsGeometric checks the degenerate case: with no pan, width
// must interpolate geometrically, so the midpoint of 1 and 100 is 10, not 50.5.
// Geometric is right because zooming is multiplicative — each unit of t should
// scale by a constant factor, not add a constant amount.
func TestZoomPureZoomIsGeometric(t *testing.T) {
	f := Zoom(ZoomView{0, 0, 1}, ZoomView{0, 0, 100})
	if got := f(0.5).Width; math.Abs(got-10) > 1e-9 {
		t.Errorf("pure-zoom midpoint width = %v, want 10 (geometric mean)", got)
	}
	// The ratio between equal steps in t is constant.
	prev := f(0).Width
	var ratio float64
	for x := 0.1; x <= 1.0001; x += 0.1 {
		w := f(x).Width
		r := w / prev
		if ratio == 0 {
			ratio = r
		} else if math.Abs(r-ratio) > 1e-9 {
			t.Fatalf("zoom ratio at t=%v is %v, want the constant %v", x, r, ratio)
		}
		prev = w
	}
	// The center does not move.
	if got := f(0.5); got.CX != 0 || got.CY != 0 {
		t.Errorf("pure zoom moved the center to (%v, %v)", got.CX, got.CY)
	}
}

// TestZoomIdenticalIsConstant checks that a no-op transition really is one.
func TestZoomIdenticalIsConstant(t *testing.T) {
	v := ZoomView{42, -17, 3.5}
	f := Zoom(v, v)
	for x := -1.0; x <= 2; x += 0.1 {
		if got := f(x); math.Abs(got.CX-v.CX) > 1e-9 || math.Abs(got.CY-v.CY) > 1e-9 ||
			math.Abs(got.Width-v.Width) > 1e-9 {
			t.Fatalf("Zoom(v, v)(%v) = %v, want %v", x, got, v)
		}
	}
	if d := ZoomDuration(v, v); d != 0 {
		t.Errorf("ZoomDuration(v, v) = %v, want 0", d)
	}
}

// TestZoomSwoops is the qualitative property the van Wijk path exists for: a
// long pan at a tight zoom level must zoom *out* in the middle rather than
// streaking across at full magnification.
func TestZoomSwoops(t *testing.T) {
	a := ZoomView{0, 0, 10}
	b := ZoomView{10000, 0, 10}
	f := Zoom(a, b)
	mid := f(0.5).Width
	if mid <= a.Width*10 {
		t.Errorf("midpoint width = %v; the path did not zoom out (endpoints are %v)", mid, a.Width)
	}
	// The center still travels monotonically from a to b.
	prev := math.Inf(-1)
	for x := 0.0; x <= 1.0001; x += 0.02 {
		cx := f(x).CX
		if cx < prev {
			t.Fatalf("center moved backwards at t=%v: %v after %v", x, cx, prev)
		}
		prev = cx
	}
	// And the width is symmetric about the midpoint for symmetric endpoints.
	if l, r := f(0.25).Width, f(0.75).Width; math.Abs(l-r)/l > 1e-6 {
		t.Errorf("width not symmetric: f(0.25) = %v, f(0.75) = %v", l, r)
	}
}

// TestZoomDurationGrowsWithDistance checks the point of a derived duration: a
// longer journey takes longer.
func TestZoomDurationGrowsWithDistance(t *testing.T) {
	a := ZoomView{0, 0, 100}
	prev := 0.0
	for _, d := range []float64{10, 100, 1000, 10000, 100000} {
		got := ZoomDuration(a, ZoomView{d, 0, 100})
		if got <= prev {
			t.Fatalf("duration for distance %v is %v, not greater than %v", d, got, prev)
		}
		prev = got
	}
	// It is symmetric: there and back take the same time.
	x := ZoomView{0, 0, 100}
	y := ZoomView{500, 300, 25}
	if f, r := ZoomDuration(x, y), ZoomDuration(y, x); math.Abs(f-r) > 1e-9 {
		t.Errorf("ZoomDuration is not symmetric: %v vs %v", f, r)
	}
	// A pure zoom also has a nonzero duration.
	if got := ZoomDuration(ZoomView{0, 0, 1}, ZoomView{0, 0, 100}); got <= 0 {
		t.Errorf("pure-zoom duration = %v, want positive", got)
	}
}

// TestZoomExtrapolates checks that t outside [0, 1] continues along the same
// path rather than clamping, matching the rest of the package.
func TestZoomExtrapolates(t *testing.T) {
	a := ZoomView{0, 0, 100}
	b := ZoomView{100, 0, 100}
	f := Zoom(a, b)
	if got := f(1.5).CX; got <= b.CX {
		t.Errorf("f(1.5).CX = %v, want beyond %v", got, b.CX)
	}
	if got := f(-0.5).CX; got >= a.CX {
		t.Errorf("f(-0.5).CX = %v, want before %v", got, a.CX)
	}
}
