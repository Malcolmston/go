package array

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

// point is the stand-in for "a row of data with an accessor" used throughout
// the tests, so that the generic form of each statistic is exercised and not
// just its …Of convenience wrapper.
type point struct {
	name string
	v    float64
}

func points(vs ...float64) []point {
	out := make([]point, len(vs))
	for i, v := range vs {
		out[i] = point{v: v}
	}
	return out
}

func pointValue(p point) float64 { return p.v }

var nan = math.NaN()

func TestMinMaxExtent(t *testing.T) {
	tests := []struct {
		name    string
		in      []float64
		wantMin float64
		wantMax float64
		wantOK  bool
	}{
		{"empty", nil, 0, 0, false},
		{"empty slice", []float64{}, 0, 0, false},
		{"single", []float64{3}, 3, 3, true},
		{"ordered", []float64{1, 2, 3}, 1, 3, true},
		{"unordered", []float64{2, 3, 1}, 1, 3, true},
		{"all equal", []float64{5, 5, 5}, 5, 5, true},
		{"negative", []float64{-3, -1, -2}, -3, -1, true},
		{"mixed signs", []float64{-1, 0, 1}, -1, 1, true},
		// NaN is skipped, not propagated: one bad reading must not blank an axis.
		{"nan among values", []float64{2, nan, 1, 3}, 1, 3, true},
		{"leading nan", []float64{nan, 7}, 7, 7, true},
		{"all nan", []float64{nan, nan}, 0, 0, false},
		{"extremes", []float64{math.SmallestNonzeroFloat64, math.MaxFloat64}, math.SmallestNonzeroFloat64, math.MaxFloat64, true},
		{"infinities", []float64{math.Inf(-1), 0, math.Inf(1)}, math.Inf(-1), math.Inf(1), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMin, ok := MinOf(tt.in)
			if ok != tt.wantOK || (ok && gotMin != tt.wantMin) {
				t.Errorf("MinOf(%v) = (%v, %v), want (%v, %v)", tt.in, gotMin, ok, tt.wantMin, tt.wantOK)
			}
			gotMax, ok := MaxOf(tt.in)
			if ok != tt.wantOK || (ok && gotMax != tt.wantMax) {
				t.Errorf("MaxOf(%v) = (%v, %v), want (%v, %v)", tt.in, gotMax, ok, tt.wantMax, tt.wantOK)
			}
			lo, hi, ok := ExtentOf(tt.in)
			if ok != tt.wantOK || (ok && (lo != tt.wantMin || hi != tt.wantMax)) {
				t.Errorf("ExtentOf(%v) = (%v, %v, %v), want (%v, %v, %v)", tt.in, lo, hi, ok, tt.wantMin, tt.wantMax, tt.wantOK)
			}
			// The generic form must agree with the specialized one.
			gLo, gHi, gOK := Extent(points(tt.in...), pointValue)
			if gOK != ok || (ok && (gLo != lo || gHi != hi)) {
				t.Errorf("Extent via accessor = (%v,%v,%v), want (%v,%v,%v)", gLo, gHi, gOK, lo, hi, ok)
			}
		})
	}
}

// TestMinMaxNotLexical guards against the classic JavaScript-flavoured bug of
// comparing numbers as strings, where "10" < "9".
func TestMinMaxNotLexical(t *testing.T) {
	in := []float64{10, 9, 100, 20}
	if got, _ := MinOf(in); got != 9 {
		t.Errorf("MinOf(%v) = %v, want 9", in, got)
	}
	if got, _ := MaxOf(in); got != 100 {
		t.Errorf("MaxOf(%v) = %v, want 100", in, got)
	}
}

func TestCount(t *testing.T) {
	tests := []struct {
		name string
		in   []float64
		want int
	}{
		{"empty", nil, 0},
		{"all defined", []float64{1, 2, 3}, 3},
		{"some nan", []float64{1, nan, 3, nan}, 2},
		{"all nan", []float64{nan, nan}, 0},
		{"infinities count", []float64{math.Inf(1), math.Inf(-1)}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CountOf(tt.in); got != tt.want {
				t.Errorf("CountOf(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestSum(t *testing.T) {
	tests := []struct {
		name string
		in   []float64
		want float64
	}{
		{"empty sums to zero", nil, 0},
		{"single", []float64{5}, 5},
		{"several", []float64{1, 2, 3}, 6},
		{"negatives cancel", []float64{-1, 1}, 0},
		{"nan contributes nothing", []float64{1, nan, 2}, 3},
		{"all nan", []float64{nan, nan}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SumOf(tt.in); got != tt.want {
				t.Errorf("SumOf(%v) = %v, want %v", tt.in, got, tt.want)
			}
			if got := FSumOf(tt.in); got != tt.want {
				t.Errorf("FSumOf(%v) = %v, want %v", tt.in, got, tt.want)
			}
			if got := Sum(points(tt.in...), pointValue); got != tt.want {
				t.Errorf("Sum via accessor = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFSumIsExact is the reason FSum exists. Naive summation of these inputs is
// visibly wrong; the compensated sum is the correctly rounded answer.
func TestFSumIsExact(t *testing.T) {
	tests := []struct {
		name string
		in   []float64
		want float64
	}{
		// 0.1+0.2 is 0.30000000000000004; adding 0.3 to that misses 0.6.
		{"tenths", []float64{0.1, 0.2, 0.3}, 0.6},
		// A large value swamps a small one, then the large value is cancelled:
		// naive addition loses the small value entirely and returns 0.
		{"catastrophic cancellation", []float64{1, 1e100, 1, -1e100}, 2},
		{"cancellation reordered", []float64{1e100, 1, -1e100, 1}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FSumOf(tt.in); got != tt.want {
				t.Errorf("FSumOf(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
	// And the defining property: the result must not depend on input order.
	values := make([]float64, 200)
	r := rand.New(rand.NewPCG(7, 11))
	for i := range values {
		values[i] = (r.Float64() - 0.5) * math.Pow(10, float64(r.IntN(20)-10))
	}
	want := FSumOf(values)
	for trial := 0; trial < 20; trial++ {
		shuffled := slices.Clone(values)
		ShuffleWith(shuffled, r.Float64)
		if got := FSumOf(shuffled); got != want {
			t.Fatalf("FSum depends on order: %v vs %v", got, want)
		}
	}
}

func TestAdderReset(t *testing.T) {
	var a Adder
	if got := a.Sum(); got != 0 {
		t.Errorf("zero Adder sums to %v, want 0", got)
	}
	a.Add(1).Add(2)
	if got := a.Sum(); got != 3 {
		t.Errorf("Adder sum = %v, want 3", got)
	}
	// Sum must not consume the accumulated state.
	if got := a.Sum(); got != 3 {
		t.Errorf("Adder sum after re-read = %v, want 3", got)
	}
	a.Reset()
	if got := a.Sum(); got != 0 {
		t.Errorf("Adder sum after Reset = %v, want 0", got)
	}
}

func TestCumsum(t *testing.T) {
	tests := []struct {
		name string
		in   []float64
		want []float64
	}{
		{"empty", nil, []float64{}},
		{"single", []float64{4}, []float64{4}},
		{"running total", []float64{1, 2, 3}, []float64{1, 3, 6}},
		{"negatives", []float64{1, -1, 1}, []float64{1, 0, 1}},
		// NaN contributes zero but keeps its slot, so the result stays aligned
		// with the input series.
		{"nan holds its place", []float64{1, nan, 2}, []float64{1, 1, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CumsumOf(tt.in)
			if !slices.Equal(got, tt.want) {
				t.Errorf("CumsumOf(%v) = %v, want %v", tt.in, got, tt.want)
			}
			if len(got) != len(tt.in) {
				t.Errorf("CumsumOf changed the length: %d vs %d", len(got), len(tt.in))
			}
		})
	}
}

// TestCumsumLastEqualsSum ties the two together: the final running total is the
// total.
func TestCumsumLastEqualsSum(t *testing.T) {
	in := []float64{0.1, 0.2, 0.3, -0.05, 42, nan, 1e-9}
	c := CumsumOf(in)
	if got, want := c[len(c)-1], FSumOf(in); got != want {
		t.Errorf("last cumsum = %v, want FSum %v", got, want)
	}
}

func TestMean(t *testing.T) {
	tests := []struct {
		name   string
		in     []float64
		want   float64
		wantOK bool
	}{
		{"empty", nil, 0, false},
		{"single", []float64{7}, 7, true},
		{"simple", []float64{1, 2, 3}, 2, true},
		{"even count", []float64{1, 2, 3, 4}, 2.5, true},
		{"all equal", []float64{5, 5, 5}, 5, true},
		// The NaN is excluded from both numerator and denominator, so this is
		// the mean of {1, 3}, not of {1, 0, 3}.
		{"nan excluded from denominator", []float64{1, nan, 3}, 2, true},
		{"all nan", []float64{nan}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := MeanOf(tt.in)
			if ok != tt.wantOK || (ok && got != tt.want) {
				t.Errorf("MeanOf(%v) = (%v, %v), want (%v, %v)", tt.in, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestQuantile(t *testing.T) {
	// R-7 places the p-quantile at position (n-1)p and interpolates linearly.
	// The quartiles of [1,2,3,4] are therefore 1.75 and 3.25 — this is the
	// number that distinguishes R-7 from the other eight definitions, so it is
	// worth stating outright.
	tests := []struct {
		name   string
		in     []float64
		p      float64
		want   float64
		wantOK bool
	}{
		{"empty", nil, 0.5, 0, false},
		{"single element, any p", []float64{9}, 0.5, 9, true},
		{"single element, p=0", []float64{9}, 0, 9, true},
		{"single element, p=1", []float64{9}, 1, 9, true},
		{"p=0 is the minimum", []float64{1, 2, 3, 4}, 0, 1, true},
		{"p=1 is the maximum", []float64{1, 2, 3, 4}, 1, 4, true},
		{"lower quartile of four", []float64{1, 2, 3, 4}, 0.25, 1.75, true},
		{"median of four", []float64{1, 2, 3, 4}, 0.5, 2.5, true},
		{"upper quartile of four", []float64{1, 2, 3, 4}, 0.75, 3.25, true},
		{"median of three", []float64{1, 2, 3}, 0.5, 2, true},
		{"quartiles of three", []float64{1, 2, 3}, 0.25, 1.5, true},
		{"exact interior point", []float64{0, 10, 30}, 0.5, 10, true},
		{"unsorted input is sorted first", []float64{3, 1, 4, 2}, 0.5, 2.5, true},
		{"all equal", []float64{5, 5, 5, 5}, 0.37, 5, true},
		{"nan skipped", []float64{1, nan, 2, 3, 4}, 0.25, 1.75, true},
		{"p below zero clamps to min", []float64{1, 2, 3}, -1, 1, true},
		{"p above one clamps to max", []float64{1, 2, 3}, 2, 3, true},
		{"p is NaN", []float64{1, 2, 3}, nan, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := QuantileOf(tt.in, tt.p)
			if ok != tt.wantOK || (ok && got != tt.want) {
				t.Errorf("QuantileOf(%v, %v) = (%v, %v), want (%v, %v)", tt.in, tt.p, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// TestQuantileDoesNotMutate: sorting a caller's slice behind their back is the
// kind of thing that produces bugs three files away.
func TestQuantileDoesNotMutate(t *testing.T) {
	in := []float64{3, 1, 4, 1, 5}
	orig := slices.Clone(in)
	QuantileOf(in, 0.5)
	if !slices.Equal(in, orig) {
		t.Errorf("QuantileOf mutated its input: %v, want %v", in, orig)
	}
}

func TestQuantileSortedMatchesQuantile(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5, 8, 13}
	for _, p := range []float64{0, 0.1, 0.25, 0.5, 0.75, 0.9, 1} {
		want, _ := QuantileOf(sorted, p)
		got, ok := QuantileSortedOf(sorted, p)
		if !ok || got != want {
			t.Errorf("QuantileSortedOf(p=%v) = (%v,%v), want %v", p, got, ok, want)
		}
	}
	if _, ok := QuantileSortedOf(nil, 0.5); ok {
		t.Error("QuantileSortedOf(nil) should report ok == false")
	}
}

// TestQuantileMonotone: whatever the interpolation, the quantile function must
// be non-decreasing in p, and must never leave the data's range.
func TestQuantileMonotone(t *testing.T) {
	in := []float64{5, 1, 9, 3, 7, 2, 8, 100, -4}
	lo, hi, _ := ExtentOf(in)
	prev := math.Inf(-1)
	for i := 0; i <= 100; i++ {
		p := float64(i) / 100
		q, ok := QuantileOf(in, p)
		if !ok {
			t.Fatalf("QuantileOf(p=%v) not ok", p)
		}
		if q < prev {
			t.Errorf("quantile decreased at p=%v: %v after %v", p, q, prev)
		}
		if q < lo || q > hi {
			t.Errorf("quantile at p=%v is %v, outside [%v,%v]", p, q, lo, hi)
		}
		prev = q
	}
}

func TestMedian(t *testing.T) {
	tests := []struct {
		name   string
		in     []float64
		want   float64
		wantOK bool
	}{
		{"empty", nil, 0, false},
		{"single", []float64{3}, 3, true},
		{"odd count", []float64{1, 2, 3}, 2, true},
		{"even count averages the middle pair", []float64{1, 2, 3, 4}, 2.5, true},
		{"unsorted", []float64{4, 1, 3, 2}, 2.5, true},
		{"all nan", []float64{nan, nan}, 0, false},
		{"outliers do not move it", []float64{1, 2, 3, 4, 1e12}, 3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := MedianOf(tt.in)
			if ok != tt.wantOK || (ok && got != tt.want) {
				t.Errorf("MedianOf(%v) = (%v, %v), want (%v, %v)", tt.in, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestMode(t *testing.T) {
	tests := []struct {
		name   string
		in     []float64
		want   float64
		wantOK bool
	}{
		{"empty", nil, 0, false},
		{"single", []float64{1}, 1, true},
		{"clear winner", []float64{1, 2, 2, 3}, 2, true},
		{"all distinct picks the first", []float64{7, 8, 9}, 7, true},
		{"all equal", []float64{4, 4, 4}, 4, true},
		// Ties go to the value seen first, so the answer is deterministic even
		// though the counts live in a Go map.
		{"tie broken by first appearance", []float64{2, 2, 1, 1}, 2, true},
		{"tie broken by first appearance, reversed", []float64{1, 1, 2, 2}, 1, true},
		{"nan is not a value", []float64{nan, nan, nan, 5}, 5, true},
		{"all nan", []float64{nan}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for trial := 0; trial < 20; trial++ { // map order is randomized per run
				got, ok := ModeOf(tt.in)
				if ok != tt.wantOK || (ok && got != tt.want) {
					t.Fatalf("ModeOf(%v) = (%v, %v), want (%v, %v)", tt.in, got, ok, tt.want, tt.wantOK)
				}
			}
		})
	}
}

func TestVarianceAndDeviation(t *testing.T) {
	tests := []struct {
		name   string
		in     []float64
		want   float64 // sample variance, n-1 denominator
		wantOK bool
	}{
		{"empty", nil, 0, false},
		// One observation has no spread to measure and n-1 would be zero.
		{"single element", []float64{5}, 0, false},
		{"one defined value among nan", []float64{5, nan}, 0, false},
		{"all equal", []float64{3, 3, 3}, 0, true},
		// var([1,2,3,4,5]) = 10/4 = 2.5 with Bessel's correction.
		{"textbook", []float64{1, 2, 3, 4, 5}, 2.5, true},
		{"two elements", []float64{1, 3}, 2, true},
		{"nan skipped", []float64{1, nan, 3}, 2, true},
		{"shift invariance", []float64{1001, 1002, 1003, 1004, 1005}, 2.5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := VarianceOf(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("VarianceOf(%v) ok = %v, want %v", tt.in, ok, tt.wantOK)
			}
			if ok && math.Abs(got-tt.want) > 1e-12 {
				t.Errorf("VarianceOf(%v) = %v, want %v", tt.in, got, tt.want)
			}
			dev, devOK := DeviationOf(tt.in)
			if devOK != tt.wantOK {
				t.Fatalf("DeviationOf(%v) ok = %v, want %v", tt.in, devOK, tt.wantOK)
			}
			if ok && math.Abs(dev-math.Sqrt(tt.want)) > 1e-12 {
				t.Errorf("DeviationOf(%v) = %v, want %v", tt.in, dev, math.Sqrt(tt.want))
			}
		})
	}
}

// TestVarianceIsNeverNegative is the point of using Welford's algorithm: the
// naive E[x²]-E[x]² formula returns a small negative number for tightly
// clustered large values, and Deviation then returns NaN.
func TestVarianceIsNeverNegative(t *testing.T) {
	in := []float64{1e9 + 4, 1e9 + 7, 1e9 + 13, 1e9 + 16}
	v, ok := VarianceOf(in)
	if !ok || v < 0 {
		t.Fatalf("VarianceOf(%v) = (%v, %v), want a non-negative variance", in, v, ok)
	}
	// The same data shifted to the origin must have the same variance.
	shifted := []float64{4, 7, 13, 16}
	want, _ := VarianceOf(shifted)
	if math.Abs(v-want) > 1e-6 {
		t.Errorf("variance is not shift-invariant: %v vs %v", v, want)
	}
	if d, ok := DeviationOf(in); !ok || math.IsNaN(d) {
		t.Errorf("DeviationOf(%v) = (%v, %v), want a real number", in, d, ok)
	}
}

func TestRank(t *testing.T) {
	tests := []struct {
		name string
		in   []float64
		want []float64
	}{
		{"empty", nil, []float64{}},
		{"single", []float64{9}, []float64{0}},
		{"already ordered", []float64{1, 2, 3}, []float64{0, 1, 2}},
		{"reversed", []float64{3, 2, 1}, []float64{2, 1, 0}},
		{"unordered", []float64{3, 1, 2}, []float64{2, 0, 1}},
		// Ties take the lowest rank of the run, and the run's other ranks are
		// skipped — so the maximum rank is still n-1.
		{"leading tie", []float64{1, 1, 3}, []float64{0, 0, 2}},
		{"trailing tie", []float64{1, 3, 3}, []float64{0, 1, 1}},
		{"all equal", []float64{5, 5, 5}, []float64{0, 0, 0}},
		{"negatives", []float64{0, -1, 1}, []float64{1, 0, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RankOf(tt.in)
			if !slices.Equal(got, tt.want) {
				t.Errorf("RankOf(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestRankNaN: an undefined value has no position among the defined ones, so it
// gets a NaN rank rather than being crowded in at either end. The result stays
// index-aligned with the input either way.
func TestRankNaN(t *testing.T) {
	got := RankOf([]float64{1, nan, 2})
	if len(got) != 3 {
		t.Fatalf("RankOf returned %d ranks for 3 values", len(got))
	}
	if got[0] != 0 || got[2] != 1 {
		t.Errorf("RankOf([1,NaN,2]) defined ranks = %v, %v; want 0, 1", got[0], got[2])
	}
	if !math.IsNaN(got[1]) {
		t.Errorf("RankOf([1,NaN,2])[1] = %v, want NaN", got[1])
	}
}

func TestRankDescending(t *testing.T) {
	in := []float64{1, 2, 3}
	got := Rank(in, Descending[float64])
	want := []float64{2, 1, 0}
	if !slices.Equal(got, want) {
		t.Errorf("Rank(%v, Descending) = %v, want %v", in, got, want)
	}
}

func TestRankBy(t *testing.T) {
	in := []point{{"c", 3}, {"a", 1}, {"b", 2}}
	got := RankBy(in, pointValue)
	want := []float64{2, 0, 1}
	if !slices.Equal(got, want) {
		t.Errorf("RankBy = %v, want %v", got, want)
	}
}

// TestRankIsAPermutationOfPositions: with no ties, the ranks must be exactly
// 0..n-1, and permuting the input by rank must sort it.
func TestRankSortsWhenPermuted(t *testing.T) {
	in := []float64{5, 1, 9, 3, 7}
	ranks := RankOf(in)
	order := make([]int, len(in))
	for i, r := range ranks {
		order[int(r)] = i
	}
	sorted := Permute(in, order)
	want := slices.Clone(in)
	slices.Sort(want)
	if !slices.Equal(sorted, want) {
		t.Errorf("permuting by rank gave %v, want %v", sorted, want)
	}
}

func TestRankDoesNotMutate(t *testing.T) {
	in := []float64{3, 1, 2}
	orig := slices.Clone(in)
	RankOf(in)
	if !slices.Equal(in, orig) {
		t.Errorf("RankOf mutated its input: %v, want %v", in, orig)
	}
}
