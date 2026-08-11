package format

import (
	"strconv"
	"testing"
)

// TestPrecisionFixed asserts the exact counts, which follow directly from the
// definition: the number of decimal places needed to see a step of that size.
func TestPrecisionFixed(t *testing.T) {
	tests := []struct {
		step float64
		want int
	}{
		{1, 0},
		{10, 0},
		{100, 0},
		{0.1, 1},
		{0.01, 2},
		{0.001, 3},
		{0.0001, 4},
		{0.5, 1},
		{0.25, 1}, // one decimal already distinguishes steps of 0.25
		{-0.01, 2},
		{0, 0},
	}
	for _, tt := range tests {
		if got := PrecisionFixed(tt.step); got != tt.want {
			t.Errorf("PrecisionFixed(%v) = %d, want %d", tt.step, got, tt.want)
		}
	}
}

func TestPrecisionRound(t *testing.T) {
	tests := []struct {
		step, max float64
		want      int
	}{
		{1, 100, 2},
		{1, 10, 1},
		{0.1, 1, 1},
		{0.01, 1, 2},
		{10, 100, 1},
		{1, 1, 1},
		{0.001, 1, 3},
		{1, 1000, 3},
	}
	for _, tt := range tests {
		if got := PrecisionRound(tt.step, tt.max); got != tt.want {
			t.Errorf("PrecisionRound(%v, %v) = %d, want %d", tt.step, tt.max, got, tt.want)
		}
	}
}

func TestPrecisionPrefix(t *testing.T) {
	tests := []struct {
		step, value float64
		want        int
	}{
		{1e5, 1.3e6, 1}, // ticks 100k apart on a megawatt axis: one decimal
		{1e6, 1.3e6, 0}, // ticks 1M apart: none
		{1e3, 1.3e6, 3}, // ticks 1k apart: three
		{1e-6, 1e-3, 3}, // milli axis, micro steps
		{1, 1, 0},       // no prefix at all
	}
	for _, tt := range tests {
		if got := PrecisionPrefix(tt.step, tt.value); got != tt.want {
			t.Errorf("PrecisionPrefix(%v, %v) = %d, want %d", tt.step, tt.value, got, tt.want)
		}
	}
}

// TestPrecisionFixedDistinguishesTicks is the property the function exists for:
// with the precision it recommends, consecutive ticks never render identically.
func TestPrecisionFixedDistinguishesTicks(t *testing.T) {
	steps := []float64{1, 5, 10, 0.5, 0.1, 0.05, 0.02, 0.01, 0.002, 0.001}
	for _, step := range steps {
		f := MustNew("." + strconv.Itoa(PrecisionFixed(step)) + "f")
		for i := 0; i < 20; i++ {
			a := f(float64(i) * step)
			b := f(float64(i+1) * step)
			if a == b {
				t.Errorf("step %v: ticks %d and %d both render as %q", step, i, i+1, a)
			}
		}
	}
}

// TestPrecisionRoundDistinguishesTicks does the same for significant-digit
// notation, over a decade of tick steps.
func TestPrecisionRoundDistinguishesTicks(t *testing.T) {
	for _, tc := range []struct{ step, max float64 }{
		{1, 10}, {1, 100}, {0.1, 1}, {0.01, 0.1}, {5, 100}, {2, 20}, {250, 1000},
	} {
		f := MustNew("." + strconv.Itoa(PrecisionRound(tc.step, tc.max)) + "r")
		for v := tc.step; v+tc.step <= tc.max; v += tc.step {
			if a, b := f(v), f(v+tc.step); a == b {
				t.Errorf("step %v max %v: %v and %v both render as %q", tc.step, tc.max, v, v+tc.step, a)
			}
		}
	}
}

// TestPrecisionPrefixDistinguishesTicks: the SI-prefixed labels of an axis all
// share a prefix and still differ from one another.
func TestPrecisionPrefixDistinguishesTicks(t *testing.T) {
	for _, tc := range []struct{ step, max float64 }{
		{1e5, 1e6}, {1e3, 1e4}, {1e-6, 1e-5}, {2e6, 1e7},
	} {
		p := PrecisionPrefix(tc.step, tc.max)
		f := FormatPrefix("."+strconv.Itoa(p)+"f", tc.max)
		for v := tc.step; v+tc.step <= tc.max; v += tc.step {
			if a, b := f(v), f(v+tc.step); a == b {
				t.Errorf("step %v max %v: %v and %v both render as %q", tc.step, tc.max, v, v+tc.step, a)
			}
		}
	}
}

func TestSIPrefix(t *testing.T) {
	tests := []struct {
		exp  int
		want string
	}{
		{0, ""}, {3, "k"}, {6, "M"}, {9, "G"}, {12, "T"}, {-3, "m"}, {-6, "µ"},
		{24, "Y"}, {-24, "y"}, {30, "Y"}, {-30, "y"},
	}
	for _, tt := range tests {
		if got := SIPrefix(tt.exp); got != tt.want {
			t.Errorf("SIPrefix(%d) = %q, want %q", tt.exp, got, tt.want)
		}
	}
}
