package interpolate

import (
	"math"
	"testing"
)

func TestNumber(t *testing.T) {
	tests := []struct {
		name string
		a, b float64
		t    float64
		want float64
	}{
		{"start", 10, 20, 0, 10},
		{"mid", 10, 20, 0.5, 15},
		{"end", 10, 20, 1, 20},
		{"quarter", 0, 100, 0.25, 25},
		{"descending", 20, 10, 0.5, 15},
		{"negative range", -10, 10, 0.5, 0},
		{"degenerate", 7, 7, 0.5, 7},
		// t is not clamped: these extrapolate.
		{"extrapolate above", 10, 20, 1.5, 25},
		{"extrapolate below", 10, 20, -0.5, 5},
		{"extrapolate far", 0, 1, 10, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Number(tt.a, tt.b)(tt.t); got != tt.want {
				t.Errorf("Number(%v, %v)(%v) = %v, want %v", tt.a, tt.b, tt.t, got, tt.want)
			}
		})
	}
}

// TestNumberEndpointsExact checks bit-for-bit endpoint exactness across
// magnitudes where a + t*(b-a) would drift.
func TestNumberEndpointsExact(t *testing.T) {
	pairs := [][2]float64{
		{0, 1},
		{1e-300, 1e300},
		{-1e17, 1e17},
		{0.1, 0.3},
		{1e16, 1e16 + 1},
		{math.MaxFloat64 / 4, math.MaxFloat64 / 2},
	}
	for _, p := range pairs {
		f := Number(p[0], p[1])
		if got := f(0); got != p[0] {
			t.Errorf("Number(%v, %v)(0) = %v, want exactly %v", p[0], p[1], got, p[0])
		}
		if got := f(1); got != p[1] {
			t.Errorf("Number(%v, %v)(1) = %v, want exactly %v", p[0], p[1], got, p[1])
		}
	}
}

func TestNumberMonotone(t *testing.T) {
	f := Number(-5, 42)
	prev := math.Inf(-1)
	for x := -0.5; x <= 1.5; x += 0.01 {
		v := f(x)
		if v < prev {
			t.Fatalf("Number is not monotone at t=%v: %v after %v", x, v, prev)
		}
		prev = v
	}
}

func TestRound(t *testing.T) {
	f := Round(0, 10)
	tests := []struct {
		t    float64
		want float64
	}{
		{0, 0},
		{0.04, 0},
		{0.05, 1}, // 0.5 rounds away from zero
		{0.5, 5},
		{0.51, 5},
		{1, 10},
		{1.5, 15}, // extrapolation still rounds
	}
	for _, tt := range tests {
		if got := f(tt.t); got != tt.want {
			t.Errorf("Round(0, 10)(%v) = %v, want %v", tt.t, got, tt.want)
		}
	}
	// The result is always an integer.
	for x := -1.0; x <= 2; x += 0.017 {
		v := Round(3.7, 19.2)(x)
		if v != math.Trunc(v) {
			t.Fatalf("Round produced non-integer %v at t=%v", v, x)
		}
	}
	// Endpoints are exact after rounding.
	if got := Round(3.7, 19.2)(0); got != 4 {
		t.Errorf("Round(3.7, 19.2)(0) = %v, want 4", got)
	}
	if got := Round(3.7, 19.2)(1); got != 19 {
		t.Errorf("Round(3.7, 19.2)(1) = %v, want 19", got)
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		t    float64
		want string
	}{
		// The motivating case: animating an SVG transform.
		{"translate mid", "translate(0,0)", "translate(10,20)", 0.5, "translate(5,10)"},
		{"translate start", "translate(0,0)", "translate(10,20)", 0, "translate(0,0)"},
		{"translate end", "translate(0,0)", "translate(10,20)", 1, "translate(10,20)"},
		// Non-numeric text comes from b.
		{"mixed", "1px solid red", "5px solid blue", 0.5, "3px solid blue"},
		{"mixed start", "1px solid red", "5px solid blue", 0, "1px solid blue"},
		// No numbers at all: constant b.
		{"no numbers", "red", "blue", 0.5, "blue"},
		{"no numbers at 0", "red", "blue", 0, "blue"},
		// Identical strings short-circuit to a constant.
		{"identical", "abc", "abc", 0.5, "abc"},
		// b has more numbers than a: extras are constant.
		{"extra in b", "a 1", "a 1 100", 0, "a 1 100"},
		{"extra in b mid", "a 0", "a 10 100", 0.5, "a 5 100"},
		// a has more numbers than b: extras are dropped.
		{"extra in a", "a 0 999", "a 10", 0.5, "a 5"},
		// Signs, decimals and exponents are all recognized.
		{"negatives", "x-10y", "x10y", 0.5, "x0y"},
		{"decimals", "0.0", "1.0", 0.5, "0.5"},
		{"exponent", "1e0", "1e1", 0, "1"},
		{"exponent end", "1e0", "1e1", 1, "10"},
		// Extrapolation.
		{"extrapolate", "translate(0,0)", "translate(10,20)", 2, "translate(20,40)"},
		{"extrapolate below", "translate(0,0)", "translate(10,20)", -1, "translate(-10,-20)"},
		// The rgb() case that makes this useful for colors written as strings.
		{"rgb", "rgb(255, 0, 0)", "rgb(0, 0, 255)", 0.5, "rgb(127.5, 0, 127.5)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := String(tt.a, tt.b)(tt.t); got != tt.want {
				t.Errorf("String(%q, %q)(%v) = %q, want %q", tt.a, tt.b, tt.t, got, tt.want)
			}
		})
	}
}

// TestStringEndpoints checks that when the two templates match, t = 0 and t = 1
// reproduce a and b (up to canonical number formatting, which is why the inputs
// here are written canonically).
func TestStringEndpoints(t *testing.T) {
	pairs := [][2]string{
		{"translate(0,0)", "translate(10,20)"},
		{"M0,0L1,1", "M5,5L6,6"},
		{"matrix(1, 0, 0, 1, 0, 0)", "matrix(2, 0, 0, 2, 10, 10)"},
		{"0", "1"},
		{"a1b2c3", "a4b5c6"},
	}
	for _, p := range pairs {
		f := String(p[0], p[1])
		if got := f(0); got != p[0] {
			t.Errorf("String(%q, %q)(0) = %q, want %q", p[0], p[1], got, p[0])
		}
		if got := f(1); got != p[1] {
			t.Errorf("String(%q, %q)(1) = %q, want %q", p[0], p[1], got, p[1])
		}
	}
}

func TestFindNumbers(t *testing.T) {
	tests := []struct {
		in   string
		want []float64
	}{
		{"", nil},
		{"abc", nil},
		{"1", []float64{1}},
		{"-1", []float64{-1}},
		{"+1", []float64{1}},
		{"1.5", []float64{1.5}},
		{".5", []float64{0.5}},
		{"-.5", []float64{-0.5}},
		{"1.", []float64{1}},
		{"1e3", []float64{1000}},
		{"1E3", []float64{1000}},
		{"1e-3", []float64{0.001}},
		{"1e+3", []float64{1000}},
		// An incomplete exponent is not part of the number.
		{"1e", []float64{1}},
		{"1ex", []float64{1}},
		// A sign only counts when it introduces a number.
		{"1-1", []float64{1, -1}},
		{"a-1", []float64{-1}},
		{"translate(10,-20)", []float64{10, -20}},
		{"rgb(255, 0, 0)", []float64{255, 0, 0}},
		{"--", nil},
		{"...", nil},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := findNumbers(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("findNumbers(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("findNumbers(%q) = %v, want %v", tt.in, got, tt.want)
				}
			}
		})
	}
}

func TestNumbers(t *testing.T) {
	f := Numbers([]float64{0, 10}, []float64{100, 20})
	if got := f(0); got[0] != 0 || got[1] != 10 {
		t.Errorf("Numbers(0) = %v, want [0 10]", got)
	}
	if got := f(0.5); got[0] != 50 || got[1] != 15 {
		t.Errorf("Numbers(0.5) = %v, want [50 15]", got)
	}
	if got := f(1); got[0] != 100 || got[1] != 20 {
		t.Errorf("Numbers(1) = %v, want [100 20]", got)
	}
	// Length follows b: extras in b are constant, extras in a are dropped.
	if got := Numbers([]float64{0}, []float64{10, 99})(0); len(got) != 2 || got[1] != 99 {
		t.Errorf("Numbers with longer b = %v, want [0 99]", got)
	}
	if got := Numbers([]float64{0, 999}, []float64{10})(0.5); len(got) != 1 || got[0] != 5 {
		t.Errorf("Numbers with longer a = %v, want [5]", got)
	}
	// The result is freshly allocated each call.
	g := Numbers([]float64{0}, []float64{10})
	x, y := g(0), g(1)
	if &x[0] == &y[0] {
		t.Error("Numbers reused its output slice")
	}
}

func TestArray(t *testing.T) {
	f := Array(Number, []float64{0, 0, 0}, []float64{1, 2, 3})
	if got := f(0.5); got[0] != 0.5 || got[1] != 1 || got[2] != 1.5 {
		t.Errorf("Array(0.5) = %v, want [0.5 1 1.5]", got)
	}
	// Endpoint exactness carries through from the element interpolator.
	if got := f(1); got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("Array(1) = %v, want [1 2 3]", got)
	}
	// String elements work too.
	sf := Array(String, []string{"a0"}, []string{"a10"})
	if got := sf(0.5); got[0] != "a5" {
		t.Errorf("Array of strings = %v, want [a5]", got)
	}
}

func TestObject(t *testing.T) {
	a := map[string]float64{"x": 0, "y": 10, "gone": 5}
	b := map[string]float64{"x": 100, "y": 20, "new": 7}
	f := Object(Number, a, b)

	got := f(0.5)
	if len(got) != 3 {
		t.Fatalf("Object result has %d keys, want 3 (b's key set)", len(got))
	}
	if got["x"] != 50 || got["y"] != 15 {
		t.Errorf("Object(0.5) = %v, want x=50 y=15", got)
	}
	if got["new"] != 7 {
		t.Errorf("key only in b = %v, want constant 7", got["new"])
	}
	if _, ok := got["gone"]; ok {
		t.Error("key only in a was not dropped")
	}
	// Endpoints.
	if v := f(1); v["x"] != 100 || v["y"] != 20 {
		t.Errorf("Object(1) = %v, want x=100 y=20", v)
	}
	if v := f(0); v["x"] != 0 || v["y"] != 10 {
		t.Errorf("Object(0) = %v, want x=0 y=10", v)
	}
}

func TestPiecewise(t *testing.T) {
	f := Piecewise(Number, []float64{0, 10, 100})
	tests := []struct {
		t    float64
		want float64
	}{
		{0, 0},
		{0.25, 5},
		{0.5, 10}, // exactly the interior knot
		{0.75, 55},
		{1, 100},
	}
	for _, tt := range tests {
		if got := f(tt.t); got != tt.want {
			t.Errorf("Piecewise(%v) = %v, want %v", tt.t, got, tt.want)
		}
	}
	// Extrapolation runs along the end segments.
	if got := f(-0.25); got != -5 {
		t.Errorf("Piecewise(-0.25) = %v, want -5", got)
	}
	if got := f(1.25); got != 145 {
		t.Errorf("Piecewise(1.25) = %v, want 145", got)
	}
	// A single value is a constant.
	if got := Piecewise(Number, []float64{42})(0.7); got != 42 {
		t.Errorf("single-value Piecewise = %v, want 42", got)
	}
	// Two values degenerate to the underlying interpolator.
	g := Piecewise(Number, []float64{3, 9})
	h := Number(3, 9)
	for x := -0.5; x <= 1.5; x += 0.1 {
		if math.Abs(g(x)-h(x)) > 1e-12 {
			t.Fatalf("2-value Piecewise differs from Number at t=%v: %v vs %v", x, g(x), h(x))
		}
	}
}

// TestPiecewiseKnotsExact checks that every interior join is hit exactly, which
// is what keeps a multi-stop ramp free of visible seams.
func TestPiecewiseKnotsExact(t *testing.T) {
	values := []float64{1, 4, 9, 16, 25}
	f := Piecewise(Number, values)
	n := len(values) - 1
	for i, v := range values {
		x := float64(i) / float64(n)
		if got := f(x); got != v {
			t.Errorf("Piecewise at knot %d (t=%v) = %v, want %v", i, x, got, v)
		}
	}
}

func TestPiecewisePanicsOnEmpty(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Piecewise with no values did not panic")
		}
	}()
	Piecewise(Number, []float64{})
}
