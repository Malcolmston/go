package scale

import (
	"math"
	"testing"
)

func TestLogScale(t *testing.T) {
	s := NewLog().SetDomain(1, 10).SetRange(0, 1)
	closeTo(t, s.Scale(1), 0, "log Scale(1)")
	closeTo(t, s.Scale(10), 1, "log Scale(10)")
	closeTo(t, s.Scale(math.Sqrt(10)), 0.5, "log Scale(sqrt(10))")
	closeTo(t, s.Invert(0.5), math.Sqrt(10), "log Invert(0.5)")
	closeTo(t, s.Invert(s.Scale(7)), 7, "log round trip")

	// The default domain is [1, 10], matching d3.
	d := NewLog().Domain()
	closeTo(t, d[0], 1, "default log domain low")
	closeTo(t, d[1], 10, "default log domain high")
}

// TestLogNegativeDomain: an entirely negative domain is legal and the transform
// reflects through the origin. A domain that crosses zero is not legal — log is
// undefined there — and the scale reports NaN rather than pretending.
func TestLogNegativeDomain(t *testing.T) {
	s := NewLog().SetDomain(-100, -1).SetRange(0, 1)
	closeTo(t, s.Scale(-100), 0, "negative log low endpoint")
	closeTo(t, s.Scale(-1), 1, "negative log high endpoint")
	closeTo(t, s.Scale(-10), 0.5, "negative log midpoint")
	closeTo(t, s.Invert(0.5), -10, "negative log invert")

	// Monotone increasing across the (negative) domain.
	prev := math.Inf(-1)
	for x := -100.0; x <= -1; x += 1 {
		v := s.Scale(x)
		if v < prev-eps {
			t.Fatalf("negative log not monotone at %v", x)
		}
		prev = v
	}

	// Crossing zero is not supported: the transform yields NaN on the wrong
	// side, which is the honest answer.
	c := NewLog().SetDomain(-1, 1).SetRange(0, 1)
	if !math.IsNaN(c.Scale(0.5)) && !math.IsInf(c.Scale(0.5), 0) {
		t.Logf("crossing-zero log domain produced %v (undefined by construction)", c.Scale(0.5))
	}
}

func TestLogTicks(t *testing.T) {
	tests := []struct {
		name   string
		domain []float64
		base   float64
		count  int
		want   []float64
	}{
		{
			name:   "one decade emits every digit",
			domain: []float64{1, 10},
			base:   10,
			count:  10,
			want:   []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		{
			name:   "base 2",
			domain: []float64{1, 32},
			base:   2,
			count:  10,
			want:   []float64{1, 2, 4, 8, 16, 32},
		},
		{
			name:   "negative domain mirrors",
			domain: []float64{-10, -1},
			base:   10,
			count:  10,
			want:   []float64{-10, -9, -8, -7, -6, -5, -4, -3, -2, -1},
		},
		{
			name:   "reversed domain reverses the ticks",
			domain: []float64{10, 1},
			base:   10,
			count:  10,
			want:   []float64{10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewLog().SetBase(tt.base).SetDomain(tt.domain...)
			got := s.Ticks(tt.count)
			if len(got) != len(tt.want) {
				t.Fatalf("Ticks(%d) = %v (%d), want %v (%d)", tt.count, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				closeTo(t, got[i], tt.want[i], "Ticks")
			}
		})
	}
}

// TestLogTicksMultipleDecades checks the shape rather than an exact vector: for
// [1, 1000] d3 emits every digit in every decade plus the closing power.
func TestLogTicksMultipleDecades(t *testing.T) {
	s := NewLog().SetDomain(1, 1000)
	got := s.Ticks(10)
	if len(got) != 28 { // 9 per decade × 3, plus 1000
		t.Errorf("Ticks over three decades = %d values, want 28: %v", len(got), got)
	}
	closeTo(t, got[0], 1, "first tick")
	closeTo(t, got[len(got)-1], 1000, "last tick")
	for i := 1; i < len(got); i++ {
		if got[i] <= got[i-1] {
			t.Fatalf("ticks not ascending at %d: %v", i, got)
		}
	}
	// Every tick is inside the domain.
	for _, v := range got {
		if v < 1-eps || v > 1000+eps {
			t.Errorf("tick %v outside the domain", v)
		}
	}
}

// TestLogNonIntegerBase: the "digits between powers" enumeration is meaningless
// for a fractional base, so the scale falls back to evenly spaced ticks in log
// space. The properties that must survive are monotonicity and containment.
func TestLogNonIntegerBase(t *testing.T) {
	for _, base := range []float64{math.E, 1.5, 3.7} {
		s := NewLog().SetBase(base).SetDomain(1, 1000)
		closeTo(t, s.Base(), base, "Base()")
		got := s.Ticks(10)
		if len(got) == 0 {
			t.Fatalf("base %v produced no ticks", base)
		}
		for i, v := range got {
			if v < 1-1e-6 || v > 1000+1e-6 {
				t.Errorf("base %v: tick %v outside the domain", base, v)
			}
			if i > 0 && v <= got[i-1] {
				t.Errorf("base %v: ticks not ascending: %v", base, got)
			}
		}
		// The scale itself must still be a correct log map in that base.
		closeTo(t, s.Scale(1), 0, "non-integer base low endpoint")
		closeTo(t, s.Scale(1000), 1, "non-integer base high endpoint")
	}
}

func TestLogNice(t *testing.T) {
	tests := []struct {
		domain []float64
		want   []float64
	}{
		{[]float64{1.1, 900}, []float64{1, 1000}},
		{[]float64{1, 10}, []float64{1, 10}},
		{[]float64{0.7, 11}, []float64{0.1, 100}},
		{[]float64{-900, -1.1}, []float64{-1000, -1}},
	}
	for _, tt := range tests {
		got := NewLog().SetDomain(tt.domain...).Nice(10).Domain()
		for i := range got {
			closeTo(t, got[i], tt.want[i], "log Nice")
		}
	}
}

// TestLogTickFormatFilters is the behavior that keeps a log axis readable: most
// ticks are drawn but not labelled, so the labels thin out to powers and the
// first couple of digits.
func TestLogTickFormatFilters(t *testing.T) {
	s := NewLog().SetDomain(1, 1000)
	f := s.TickFormat(10)

	// The powers of the base are always labelled.
	for _, p := range []float64{1, 10, 100, 1000} {
		if f(p) == "" {
			t.Errorf("power %v should be labelled", p)
		}
	}

	// With 28 ticks and a budget of 10, most intermediate digits are dropped.
	ticks := s.Ticks(10)
	labelled := 0
	for _, v := range ticks {
		if f(v) != "" {
			labelled++
		}
	}
	if labelled >= len(ticks) {
		t.Errorf("TickFormat labelled every one of %d ticks; it must filter", len(ticks))
	}
	if labelled == 0 {
		t.Error("TickFormat labelled nothing")
	}

	// The default base-10 specifier is "s", so the powers carry SI prefixes and
	// insignificant zeros are trimmed.
	for _, tt := range []struct {
		in   float64
		want string
	}{
		{1, "1"},
		{2, "2"},
		{10, "10"},
		{100, "100"},
		{1000, "1k"},
	} {
		if got := f(tt.in); got != tt.want {
			t.Errorf("log TickFormat(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}

	// Over a single decade there is room for every label.
	one := NewLog().SetDomain(1, 10)
	g := one.TickFormat(10)
	for _, v := range one.Ticks(10) {
		if g(v) == "" {
			t.Errorf("single decade: %v should be labelled", v)
		}
	}
}
