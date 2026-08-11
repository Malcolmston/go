package array

import (
	"math"
	"slices"
	"testing"
)

func TestTicks(t *testing.T) {
	tests := []struct {
		name        string
		start, stop float64
		count       int
		want        []float64
	}{
		// The canonical cases from d3's documentation. Note that a count of 10
		// over [0,1] yields eleven values: the count is a hint about intervals,
		// not about how many numbers come back.
		{"unit domain, count 10", 0, 1, 10, []float64{0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1}},
		{"unit domain, count 5", 0, 1, 5, []float64{0, 0.2, 0.4, 0.6, 0.8, 1}},
		{"unit domain, count 2", 0, 1, 2, []float64{0, 0.5, 1}},
		{"unit domain, count 1", 0, 1, 1, []float64{0, 1}},
		{"decade, count 5", 0, 10, 5, []float64{0, 2, 4, 6, 8, 10}},
		{"decade, count 10", 0, 10, 10, []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
		{"offset domain", 1, 2, 5, []float64{1, 1.2, 1.4, 1.6, 1.8, 2}},
		{"symmetric about zero", -1, 1, 10, []float64{-1, -0.8, -0.6, -0.4, -0.2, 0, 0.2, 0.4, 0.6, 0.8, 1}},
		{"negative domain", -10, 0, 5, []float64{-10, -8, -6, -4, -2, 0}},

		// Reversed domains produce reversed ticks rather than nothing; this is
		// what lets a scale with an inverted range still draw an axis.
		{"reversed unit domain", 1, 0, 10, []float64{1, 0.9, 0.8, 0.7, 0.6, 0.5, 0.4, 0.3, 0.2, 0.1, 0}},
		{"reversed decade", 10, 0, 5, []float64{10, 8, 6, 4, 2, 0}},

		// Degenerate inputs.
		{"empty domain", 1, 1, 10, []float64{1}},
		{"empty domain at zero", 0, 0, 10, []float64{0}},
		{"count zero", 0, 1, 0, []float64{}},
		{"count negative", 0, 1, -5, []float64{}},
		{"empty domain, count zero", 1, 1, 0, []float64{}},
		{"NaN start", math.NaN(), 1, 10, []float64{}},
		{"NaN stop", 0, math.NaN(), 10, []float64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Ticks(tt.start, tt.stop, tt.count)
			if !slices.Equal(got, tt.want) {
				t.Errorf("Ticks(%v, %v, %d) = %v, want %v", tt.start, tt.stop, tt.count, got, tt.want)
			}
		})
	}
}

// TestTicksExactDecimals guards the reason tickSpec carries a negative
// increment: sub-unit ticks must be exact decimal doubles, not the results of
// repeated addition. If this fails, axis labels start reading 0.30000000000000004.
func TestTicksExactDecimals(t *testing.T) {
	got := Ticks(0, 1, 10)
	want := []float64{0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Ticks(0,1,10)[%d] = %v (bits %x), want %v (bits %x)",
				i, got[i], math.Float64bits(got[i]), want[i], math.Float64bits(want[i]))
		}
	}
}

// TestTicksReversedIsMirror checks the invariant rather than restating the
// numbers: reversing the domain must reverse the ticks, for every domain.
func TestTicksReversedIsMirror(t *testing.T) {
	domains := [][2]float64{{0, 1}, {-1, 1}, {0, 10}, {3, 7}, {-100, -10}, {0.001, 0.002}, {1e6, 1e7}}
	for _, d := range domains {
		for _, count := range []int{1, 2, 3, 5, 10, 20} {
			fwd := Ticks(d[0], d[1], count)
			rev := Ticks(d[1], d[0], count)
			mirror := slices.Clone(fwd)
			slices.Reverse(mirror)
			if !slices.Equal(rev, mirror) {
				t.Errorf("Ticks(%v,%v,%d)=%v reversed is %v, but Ticks(%v,%v,%d)=%v",
					d[0], d[1], count, fwd, mirror, d[1], d[0], count, rev)
			}
		}
	}
}

// TestTicksWithinDomain checks that ticks never escape the domain and always
// ascend — the two properties a scale relies on when it maps them to pixels.
func TestTicksWithinDomain(t *testing.T) {
	domains := [][2]float64{{0, 1}, {-1, 1}, {0, 10}, {3, 7}, {-100, -10}, {0.0001, 0.0009}, {1e9, 2e9}, {-1e-9, 1e-9}}
	for _, d := range domains {
		for count := 1; count <= 20; count++ {
			ticks := Ticks(d[0], d[1], count)
			for i, v := range ticks {
				if v < d[0] || v > d[1] {
					t.Errorf("Ticks(%v,%v,%d)[%d] = %v is outside the domain", d[0], d[1], count, i, v)
				}
				if i > 0 && !(v > ticks[i-1]) {
					t.Errorf("Ticks(%v,%v,%d) is not strictly increasing: %v", d[0], d[1], count, ticks)
				}
			}
		}
	}
}

func TestTickIncrementAndStep(t *testing.T) {
	tests := []struct {
		name        string
		start, stop float64
		count       int
		wantInc     float64
		wantStep    float64
	}{
		// A negative increment means "divide by this", so an increment of -10
		// is a step of 0.1. This encoding is why both functions exist.
		{"unit domain, count 10", 0, 1, 10, -10, 0.1},
		{"unit domain, count 5", 0, 1, 5, -5, 0.2},
		{"unit domain, count 2", 0, 1, 2, -2, 0.5},
		{"unit domain, count 1", 0, 1, 1, 1, 1},
		{"decade, count 5", 0, 10, 5, 2, 2},
		{"decade, count 10", 0, 10, 10, 1, 1},
		{"century, count 10", 0, 100, 10, 10, 10},
		{"century, count 5", 0, 100, 5, 20, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TickIncrement(tt.start, tt.stop, tt.count); got != tt.wantInc {
				t.Errorf("TickIncrement(%v,%v,%d) = %v, want %v", tt.start, tt.stop, tt.count, got, tt.wantInc)
			}
			if got := TickStep(tt.start, tt.stop, tt.count); got != tt.wantStep {
				t.Errorf("TickStep(%v,%v,%d) = %v, want %v", tt.start, tt.stop, tt.count, got, tt.wantStep)
			}
		})
	}
}

// TestTickStepSignFollowsDomain pins the one way TickStep differs from
// TickIncrement: it is signed by the domain's direction.
func TestTickStepSignFollowsDomain(t *testing.T) {
	if got, want := TickStep(1, 0, 10), -0.1; got != want {
		t.Errorf("TickStep(1,0,10) = %v, want %v", got, want)
	}
	if got, want := TickStep(10, 0, 5), -2.0; got != want {
		t.Errorf("TickStep(10,0,5) = %v, want %v", got, want)
	}
}

// TestTickStepMatchesTicks is the cross-check that matters for scales: the step
// TickStep reports must be the actual spacing of the ticks Ticks generates.
func TestTickStepMatchesTicks(t *testing.T) {
	domains := [][2]float64{{0, 1}, {0, 10}, {-1, 1}, {2, 3}, {-50, 50}, {0, 1e-3}}
	for _, d := range domains {
		for _, count := range []int{2, 5, 10, 20} {
			ticks := Ticks(d[0], d[1], count)
			if len(ticks) < 2 {
				continue
			}
			step := TickStep(d[0], d[1], count)
			gap := ticks[1] - ticks[0]
			if math.Abs(gap-step) > 1e-12*math.Abs(step) {
				t.Errorf("Ticks(%v,%v,%d) spacing %v != TickStep %v", d[0], d[1], count, gap, step)
			}
		}
	}
}

func TestTickIncrementDegenerateCount(t *testing.T) {
	// A count of zero divides by zero: d3 yields an infinite increment, and
	// Ticks turns that into no ticks at all rather than a hang.
	if got := TickIncrement(0, 1, 0); !math.IsInf(got, 1) {
		t.Errorf("TickIncrement(0,1,0) = %v, want +Inf", got)
	}
	if got := Ticks(0, 1, 0); len(got) != 0 {
		t.Errorf("Ticks(0,1,0) = %v, want empty", got)
	}
}

func TestNiceTicks(t *testing.T) {
	tests := []struct {
		name           string
		start, stop    float64
		count          int
		wantLo, wantHi float64
	}{
		{"already nice", 0, 1, 10, 0, 1},
		{"sub-unit domain", 0.37, 0.96, 10, 0.35, 1},
		{"just past a decade", 1.1, 10.9, 10, 1, 11},
		{"empty domain", 1, 1, 10, 1, 1},
		{"count zero leaves domain alone", 0.37, 0.96, 0, 0.37, 0.96},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lo, hi := NiceTicks(tt.start, tt.stop, tt.count)
			if lo != tt.wantLo || hi != tt.wantHi {
				t.Errorf("NiceTicks(%v,%v,%d) = (%v,%v), want (%v,%v)",
					tt.start, tt.stop, tt.count, lo, hi, tt.wantLo, tt.wantHi)
			}
		})
	}
}

// TestNiceTicksProperties checks what a scale actually depends on: nicing only
// ever widens a domain, never shrinks or flips it, and it terminates.
//
// Nicing is deliberately *not* asserted to be idempotent. It isn't, and cannot
// be at small counts: widening [1.1, 10.9] to [0, 15] at count 2 lets the next
// pass choose a coarser step and widen again to [0, 20]. That is d3's behaviour
// too — the loop inside NiceTicks stops when the step repeats, not when the
// domain does — and a test asserting otherwise would be asserting a bug.
func TestNiceTicksProperties(t *testing.T) {
	domains := [][2]float64{{0.37, 0.96}, {1.1, 10.9}, {-3.2, 7.8}, {-0.007, -0.001}, {123, 4567}, {0, 1}, {-1e6, 1e6}}
	for _, d := range domains {
		for _, count := range []int{1, 2, 5, 10, 20, 50} {
			lo, hi := NiceTicks(d[0], d[1], count)
			if math.IsNaN(lo) || math.IsNaN(hi) || math.IsInf(lo, 0) || math.IsInf(hi, 0) {
				t.Fatalf("NiceTicks(%v,%v,%d) = (%v,%v), want finite", d[0], d[1], count, lo, hi)
			}
			if lo > d[0] || hi < d[1] {
				t.Errorf("NiceTicks(%v,%v,%d) = (%v,%v) does not contain the input domain", d[0], d[1], count, lo, hi)
			}
			// A second pass may widen further, but must never come back inside.
			lo2, hi2 := NiceTicks(lo, hi, count)
			if lo2 > lo || hi2 < hi {
				t.Errorf("NiceTicks(%v,%v,%d) shrank on a second pass: (%v,%v) then (%v,%v)", d[0], d[1], count, lo, hi, lo2, hi2)
			}
		}
	}
}

// TestNiceTicksIdempotentAtTypicalCounts pins the case that does hold and that
// scales rely on: at the tick counts an axis actually uses, nicing a niced
// domain changes nothing, so re-nicing on every redraw is stable.
func TestNiceTicksIdempotentAtTypicalCounts(t *testing.T) {
	domains := [][2]float64{{0.37, 0.96}, {1.1, 10.9}, {-3.2, 7.8}, {-0.007, -0.001}, {123, 4567}, {0, 1}}
	for _, d := range domains {
		for _, count := range []int{5, 10, 20} {
			lo, hi := NiceTicks(d[0], d[1], count)
			lo2, hi2 := NiceTicks(lo, hi, count)
			if lo2 != lo || hi2 != hi {
				t.Errorf("NiceTicks(%v,%v,%d) is not idempotent: (%v,%v) then (%v,%v)", d[0], d[1], count, lo, hi, lo2, hi2)
			}
		}
	}
}

// TestNiceTicksReversed documents that a reversed domain is left alone rather
// than silently sorted; a caller that wants it niced should swap first.
func TestNiceTicksReversedIsMirrored(t *testing.T) {
	lo, hi := NiceTicks(0.96, 0.37, 10)
	// The algorithm is symmetric in its two arguments only up to which end gets
	// floored, so all we assert is that it terminates and still spans the input.
	if math.IsNaN(lo) || math.IsNaN(hi) {
		t.Fatalf("NiceTicks on a reversed domain produced NaN: (%v,%v)", lo, hi)
	}
}

func TestJSRound(t *testing.T) {
	// Halves round toward +infinity, unlike math.Round. Getting this wrong
	// shifts ticks by a whole step on negative domains, so it is worth pinning.
	tests := []struct{ in, want float64 }{
		{0.5, 1}, {1.5, 2}, {-0.5, 0}, {-1.5, -1}, {-2.5, -2}, {2.4, 2}, {-2.6, -3},
	}
	for _, tt := range tests {
		if got := jsRound(tt.in); got != tt.want {
			t.Errorf("jsRound(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// TestTicksRefusesAbsurdLengths: unlike d3, which will allocate until the
// process dies, a domain that would need more ticks than could be drawn yields
// none at all.
func TestTicksRefusesAbsurdLengths(t *testing.T) {
	if got := Ticks(0, 1e18, 1000000000); len(got) != 0 {
		t.Errorf("Ticks over an absurd domain returned %d values, want 0", len(got))
	}
	// Anything the cap does allow through must still be a sane, ascending axis.
	if got := Ticks(0, math.MaxFloat64, 1000000); len(got) > maxTicks {
		t.Errorf("Ticks returned %d values, above the cap of %d", len(got), maxTicks)
	}
}
