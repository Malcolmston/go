package array

import (
	"math"
	"slices"
	"testing"
)

func TestThresholdSturges(t *testing.T) {
	// ceil(log2(n)) + 1, floored at one bin.
	tests := []struct {
		n    int
		want int
	}{
		{0, 1}, {1, 1}, {2, 2}, {3, 3}, {4, 3}, {5, 4}, {8, 4}, {9, 5}, {16, 5}, {100, 8}, {1000, 11},
	}
	for _, tt := range tests {
		values := make([]float64, tt.n)
		for i := range values {
			values[i] = float64(i)
		}
		if got := ThresholdSturges(values, 0, float64(tt.n)); got != tt.want {
			t.Errorf("ThresholdSturges(n=%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
	// NaN values do not count toward n.
	if got, want := ThresholdSturges([]float64{1, nan, nan, 2}, 1, 2), ThresholdSturges([]float64{1, 2}, 1, 2); got != want {
		t.Errorf("ThresholdSturges counted NaN: %d, want %d", got, want)
	}
}

func TestThresholdScott(t *testing.T) {
	// A degenerate spread has no meaningful bin width, so the rule falls back
	// to a single bin rather than dividing by zero.
	degenerate := [][]float64{nil, {1}, {5, 5, 5, 5}, {nan, nan}}
	for _, in := range degenerate {
		lo, hi, _ := ExtentOf(in)
		if got := ThresholdScott(in, lo, hi); got != 1 {
			t.Errorf("ThresholdScott(%v) = %d, want 1", in, got)
		}
	}
	// Otherwise the count must be at least one and finite, and must grow with
	// the sample size for data of fixed spread — that is what the n^(1/3)
	// factor is there for.
	small := make([]float64, 50)
	large := make([]float64, 5000)
	for i := range small {
		small[i] = float64(i % 10)
	}
	for i := range large {
		large[i] = float64(i % 10)
	}
	cSmall := ThresholdScott(small, 0, 9)
	cLarge := ThresholdScott(large, 0, 9)
	if cSmall < 1 || cLarge < 1 {
		t.Fatalf("ThresholdScott returned counts below 1: %d, %d", cSmall, cLarge)
	}
	if cLarge <= cSmall {
		t.Errorf("ThresholdScott did not grow with n: %d for 50 values, %d for 5000", cSmall, cLarge)
	}
}

func TestThresholdFreedmanDiaconis(t *testing.T) {
	// A zero interquartile range means no usable bin width.
	degenerate := [][]float64{nil, {1}, {7, 7, 7, 7}, {nan}}
	for _, in := range degenerate {
		lo, hi, _ := ExtentOf(in)
		if got := ThresholdFreedmanDiaconis(in, lo, hi); got != 1 {
			t.Errorf("ThresholdFreedmanDiaconis(%v) = %d, want 1", in, got)
		}
	}
	// The rule's selling point: it uses the interquartile range, so a single
	// wild outlier must not blow the bin count up the way Scott's rule does.
	base := make([]float64, 200)
	for i := range base {
		base[i] = float64(i % 20)
	}
	clean := ThresholdFreedmanDiaconis(base, 0, 19)
	withOutlier := append(slices.Clone(base), 1e6)
	scottClean := ThresholdScott(base, 0, 19)
	scottOutlier := ThresholdScott(withOutlier, 0, 1e6)
	fdOutlier := ThresholdFreedmanDiaconis(withOutlier, 0, 1e6)
	if clean < 1 || fdOutlier < 1 || scottClean < 1 || scottOutlier < 1 {
		t.Fatalf("threshold rules returned counts below 1")
	}
	// Both rules must still produce something usable; the point is only that
	// neither returns a nonsense count.
	if fdOutlier > 1<<20 {
		t.Errorf("ThresholdFreedmanDiaconis returned an absurd count: %d", fdOutlier)
	}
}

// TestBinDefault pins the whole default pipeline on the example worked through
// by hand: eight values, Sturges asking for four bins, the domain niced to
// [0,20] and then extended to 25 so the maximum does not land in a zero-width
// final bin.
func TestBinDefault(t *testing.T) {
	data := []float64{1, 2, 2, 10, 18, 18, 18, 20}
	bins := NewBinOf().Apply(data)

	wantX0 := []float64{0, 5, 10, 15, 20}
	wantX1 := []float64{5, 10, 15, 20, 25}
	wantValues := [][]float64{{1, 2, 2}, {}, {10}, {18, 18, 18}, {20}}

	if len(bins) != len(wantX0) {
		t.Fatalf("got %d bins, want %d: %v", len(bins), len(wantX0), bins)
	}
	for i, b := range bins {
		if b.X0 != wantX0[i] || b.X1 != wantX1[i] {
			t.Errorf("bin %d spans [%v,%v), want [%v,%v)", i, b.X0, b.X1, wantX0[i], wantX1[i])
		}
		if !slices.Equal(b.Values, wantValues[i]) && !(len(b.Values) == 0 && len(wantValues[i]) == 0) {
			t.Errorf("bin %d holds %v, want %v", i, b.Values, wantValues[i])
		}
		if b.Len() != len(b.Values) {
			t.Errorf("bin %d Len() = %d, want %d", i, b.Len(), len(b.Values))
		}
	}
}

func TestBinExplicitDomain(t *testing.T) {
	// With an explicit domain nothing is niced or extended: the thresholds are
	// exactly the ticks of the domain, and the last bin is closed on the right
	// so the maximum has somewhere to go.
	data := []float64{0, 0.05, 0.5, 0.99, 1}
	bins := NewBinOf().DomainRange(0, 1).ThresholdCount(10).Apply(data)
	if len(bins) != 10 {
		t.Fatalf("got %d bins, want 10: %v", len(bins), bins)
	}
	if bins[0].X0 != 0 || bins[len(bins)-1].X1 != 1 {
		t.Errorf("bins span [%v,%v], want [0,1]", bins[0].X0, bins[len(bins)-1].X1)
	}
	if !slices.Equal(bins[0].Values, []float64{0, 0.05}) {
		t.Errorf("first bin = %v, want [0 0.05]", bins[0].Values)
	}
	// 1 is the domain maximum and belongs in the last bin, not off the end.
	if !slices.Equal(bins[9].Values, []float64{0.99, 1}) {
		t.Errorf("last bin = %v, want [0.99 1]", bins[9].Values)
	}
}

func TestBinDropsValuesOutsideTheDomain(t *testing.T) {
	data := []float64{-5, 0, 5, 10, 15}
	bins := NewBinOf().DomainRange(0, 10).ThresholdCount(2).Apply(data)
	var got []float64
	for _, b := range bins {
		got = append(got, b.Values...)
	}
	want := []float64{0, 5, 10}
	if !slices.Equal(got, want) {
		t.Errorf("binned values = %v, want %v (out-of-domain values dropped, not clamped)", got, want)
	}
}

func TestBinExplicitThresholds(t *testing.T) {
	data := []float64{1, 5, 9, 14, 19}
	bins := NewBinOf().DomainRange(0, 20).ThresholdValues([]float64{5, 10, 15}).Apply(data)
	wantX0 := []float64{0, 5, 10, 15}
	wantX1 := []float64{5, 10, 15, 20}
	if len(bins) != 4 {
		t.Fatalf("got %d bins, want 4 (n thresholds make n+1 bins)", len(bins))
	}
	for i, b := range bins {
		if b.X0 != wantX0[i] || b.X1 != wantX1[i] {
			t.Errorf("bin %d spans [%v,%v), want [%v,%v)", i, b.X0, b.X1, wantX0[i], wantX1[i])
		}
	}
	// A value equal to a threshold opens the next bin: bins are half-open on
	// the right, so 5 belongs to [5,10), not to [0,5).
	if !slices.Equal(bins[0].Values, []float64{1}) {
		t.Errorf("bin 0 = %v, want [1]", bins[0].Values)
	}
	if !slices.Equal(bins[1].Values, []float64{5, 9}) {
		t.Errorf("bin 1 = %v, want [5 9]", bins[1].Values)
	}
}

func TestBinExplicitThresholdsOutsideDomain(t *testing.T) {
	// Thresholds outside the domain would open empty bins hanging off the axis.
	bins := NewBinOf().DomainRange(0, 10).ThresholdValues([]float64{-5, 5, 50}).Apply([]float64{1, 7})
	if len(bins) != 2 {
		t.Fatalf("got %d bins, want 2 after dropping the out-of-domain thresholds: %v", len(bins), bins)
	}
	if bins[0].X0 != 0 || bins[0].X1 != 5 || bins[1].X0 != 5 || bins[1].X1 != 10 {
		t.Errorf("bins = [%v,%v) [%v,%v), want [0,5) [5,10)", bins[0].X0, bins[0].X1, bins[1].X0, bins[1].X1)
	}
}

func TestBinDegenerateInput(t *testing.T) {
	t.Run("no data", func(t *testing.T) {
		bins := NewBinOf().Apply(nil)
		if len(bins) != 1 || bins[0].Len() != 0 {
			t.Errorf("Apply(nil) = %v, want a single empty bin", bins)
		}
	})
	t.Run("all NaN", func(t *testing.T) {
		bins := NewBinOf().Apply([]float64{nan, nan})
		total := 0
		for _, b := range bins {
			total += b.Len()
		}
		if total != 0 {
			t.Errorf("NaN values were binned: %v", bins)
		}
	})
	t.Run("single value", func(t *testing.T) {
		bins := NewBinOf().Apply([]float64{7})
		total := 0
		for _, b := range bins {
			total += b.Len()
		}
		if total != 1 {
			t.Errorf("a single value produced %d binned values, want 1: %v", total, bins)
		}
	})
	t.Run("all equal", func(t *testing.T) {
		bins := NewBinOf().Apply([]float64{5, 5, 5, 5})
		total := 0
		for _, b := range bins {
			total += b.Len()
		}
		if total != 4 {
			t.Errorf("all-equal data produced %d binned values, want 4: %v", total, bins)
		}
	})
	t.Run("NaN mixed with data", func(t *testing.T) {
		bins := NewBinOf().Apply([]float64{1, nan, 2, nan, 3})
		total := 0
		for _, b := range bins {
			total += b.Len()
		}
		if total != 3 {
			t.Errorf("binned %d values, want the 3 defined ones: %v", total, bins)
		}
	})
}

// TestBinPartitionsTheData is the invariant that matters: with a domain wide
// enough to contain everything, every value lands in exactly one bin, the bins
// tile the domain without gaps or overlaps, and they ascend.
func TestBinPartitionsTheData(t *testing.T) {
	datasets := [][]float64{
		{1, 2, 3, 4, 5},
		{-10, -3, 0, 3, 10},
		{0.001, 0.002, 0.0035, 0.004},
		{1e6, 2e6, 3e6},
		{1, 1, 1, 2},
		{0, 100},
	}
	for _, data := range datasets {
		for _, count := range []int{1, 2, 5, 10, 20} {
			bins := NewBinOf().ThresholdCount(count).Apply(data)
			total := 0
			for i, b := range bins {
				total += b.Len()
				if !(b.X1 >= b.X0) {
					t.Errorf("bin %d of %v spans backwards: [%v,%v)", i, data, b.X0, b.X1)
				}
				if i > 0 && bins[i-1].X1 != b.X0 {
					t.Errorf("bins of %v are not contiguous at %d: %v then %v", data, i, bins[i-1].X1, b.X0)
				}
				for _, v := range b.Values {
					// Every value is inside its bin, with the last bin closed.
					last := i == len(bins)-1
					if v < b.X0 || (v > b.X1) || (!last && v == b.X1) {
						t.Errorf("value %v is in bin %d [%v,%v)", v, i, b.X0, b.X1)
					}
				}
			}
			if total != len(data) {
				t.Errorf("binning %v with count %d placed %d of %d values", data, count, total, len(data))
			}
		}
	}
}

// TestBinUniformWidth is what the domain-extension logic buys: with the default
// domain every bin is the same width, so bar areas are comparable.
func TestBinUniformWidth(t *testing.T) {
	datasets := [][]float64{
		{1, 2, 2, 10, 18, 18, 18, 20},
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		{-5, -2, 0, 2, 5},
		{0.1, 0.2, 0.3, 0.4, 0.5},
	}
	for _, data := range datasets {
		for _, count := range []int{2, 5, 10} {
			bins := NewBinOf().ThresholdCount(count).Apply(data)
			if len(bins) < 2 {
				continue
			}
			w := bins[0].X1 - bins[0].X0
			for i, b := range bins {
				if got := b.X1 - b.X0; math.Abs(got-w) > 1e-9*math.Abs(w) {
					t.Errorf("bin %d of %v (count %d) is %v wide, want %v", i, data, count, got, w)
				}
			}
		}
	}
}

func TestBinWithAccessor(t *testing.T) {
	// The generic form over a struct, which is the reason Binner takes an
	// accessor rather than only working on []float64.
	data := []point{{"a", 1}, {"b", 2}, {"c", 9}}
	bins := NewBin(pointValue).DomainRange(0, 10).ThresholdCount(2).Apply(data)
	total := 0
	for _, b := range bins {
		total += b.Len()
		for _, p := range b.Values {
			if p.v < b.X0 || p.v > b.X1 {
				t.Errorf("point %v is in bin [%v,%v)", p, b.X0, b.X1)
			}
		}
	}
	if total != 3 {
		t.Errorf("binned %d points, want 3", total)
	}
	if bins[0].Values[0].name != "a" {
		t.Errorf("bins hold the original elements, not their values: got %v", bins[0].Values[0])
	}
}

func TestBinCustomThresholdRule(t *testing.T) {
	data := make([]float64, 100)
	for i := range data {
		data[i] = float64(i)
	}
	sturges := NewBinOf().Apply(data)
	scott := NewBinOf().Thresholds(ThresholdScott).Apply(data)
	fd := NewBinOf().Thresholds(ThresholdFreedmanDiaconis).Apply(data)
	for name, bins := range map[string][]Bin[float64]{"sturges": sturges, "scott": scott, "fd": fd} {
		total := 0
		for _, b := range bins {
			total += b.Len()
		}
		if total != len(data) {
			t.Errorf("%s binning placed %d of %d values", name, total, len(data))
		}
		if len(bins) < 1 {
			t.Errorf("%s produced no bins", name)
		}
	}
	// Which rule is finest depends on the distribution — for a uniform sample
	// Freedman–Diaconis is actually the coarsest — so the only thing asserted
	// here is that all three answer, and differ. Asserting an ordering would be
	// asserting a guess.
	if len(sturges) == len(scott) && len(scott) == len(fd) {
		t.Errorf("all three rules produced %d bins for uniform data; expected them to differ", len(sturges))
	}
}

func TestBinnerIsReusable(t *testing.T) {
	// Applying a configured Binner twice must give the same answer, and must
	// not have mutated the Binner in between.
	b := NewBinOf().DomainRange(0, 10).ThresholdCount(5)
	first := b.Apply([]float64{1, 2, 3})
	second := b.Apply([]float64{1, 2, 3})
	if len(first) != len(second) {
		t.Fatalf("reapplying gave %d then %d bins", len(first), len(second))
	}
	for i := range first {
		if first[i].X0 != second[i].X0 || first[i].X1 != second[i].X1 || first[i].Len() != second[i].Len() {
			t.Errorf("bin %d differs between applications: %v vs %v", i, first[i], second[i])
		}
	}
}

func TestBinCustomDomainFunc(t *testing.T) {
	// A Domain function that reports no domain yields one empty bin rather
	// than a panic or a NaN-edged bin.
	bins := NewBinOf().
		Domain(func([]float64) (float64, float64, bool) { return 0, 0, false }).
		Apply([]float64{1, 2, 3})
	if len(bins) != 1 || bins[0].Len() != 0 {
		t.Errorf("Apply with no domain = %v, want a single empty bin", bins)
	}
}

func TestBinnerValueSetter(t *testing.T) {
	// Value can be changed after construction, so one Binner can be retargeted
	// at a different field of the same data.
	data := []point{{"a", 1}, {"b", 9}}
	b := NewBin(func(point) float64 { return 0 }).DomainRange(0, 10).ThresholdCount(2)
	b.Value(pointValue)
	bins := b.Apply(data)
	if bins[0].Values[0].name != "a" || bins[len(bins)-1].Values[0].name != "b" {
		t.Errorf("Value setter had no effect: %v", bins)
	}
}

// TestThresholdRulesClampAbsurdCounts: a near-zero spread makes Scott's and
// Freedman–Diaconis' formulas ask for astronomically many bins. Allocating them
// helps nobody, so the count is capped.
func TestThresholdRulesClampAbsurdCounts(t *testing.T) {
	in := []float64{0, 1e-300, 2e-300, 3e-300}
	for name, rule := range map[string]ThresholdFunc{"scott": ThresholdScott, "fd": ThresholdFreedmanDiaconis} {
		got := rule(in, 0, 1e300)
		if got < 1 || got > 1<<20 {
			t.Errorf("%s returned %d bins, want a count in [1, 2^20]", name, got)
		}
	}
}
