package scale

import (
	"math"
	"testing"
)

func TestQuantize(t *testing.T) {
	q := NewQuantize[string]("a", "b", "c", "d").SetDomain(0, 1)

	th := q.Thresholds()
	want := []float64{0.25, 0.5, 0.75}
	if len(th) != len(want) {
		t.Fatalf("Thresholds() = %v, want %v", th, want)
	}
	for i := range th {
		closeTo(t, th[i], want[i], "Thresholds")
	}

	tests := []struct {
		in   float64
		want string
	}{
		{-1, "a"}, // below the domain still lands in the first bucket
		{0, "a"},
		{0.24, "a"},
		{0.25, "b"}, // a value exactly on a cut goes to the bucket ABOVE it
		{0.49, "b"},
		{0.5, "c"},
		{0.75, "d"},
		{1, "d"},
		{2, "d"}, // above the domain lands in the last bucket
	}
	for _, tt := range tests {
		if got := q.Scale(tt.in); got != tt.want {
			t.Errorf("Quantize.Scale(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}

	lo, hi := q.InvertExtent("b")
	closeTo(t, lo, 0.25, "InvertExtent(b) lo")
	closeTo(t, hi, 0.5, "InvertExtent(b) hi")
	lo, hi = q.InvertExtent("a")
	closeTo(t, lo, 0, "InvertExtent(a) lo")
	closeTo(t, hi, 0.25, "InvertExtent(a) hi")
	lo, hi = q.InvertExtent("d")
	closeTo(t, lo, 0.75, "InvertExtent(d) lo")
	closeTo(t, hi, 1, "InvertExtent(d) hi")
	lo, hi = q.InvertExtent("zzz")
	if !math.IsNaN(lo) || !math.IsNaN(hi) {
		t.Errorf("InvertExtent of a non-range value = %v,%v; want NaN,NaN", lo, hi)
	}

	// Uniform slicing is the defining property: every bucket is the same width.
	w := NewQuantize[int](0, 1, 2, 3, 4).SetDomain(0, 100)
	prev := math.NaN()
	for i := 0; i < 5; i++ {
		a, b := w.InvertExtent(i)
		if !math.IsNaN(prev) {
			closeTo(t, b-a, prev, "every quantize bucket has the same width")
		}
		prev = b - a
	}
}

func TestQuantizeNiceAndTicks(t *testing.T) {
	q := NewQuantize[string]("a", "b").SetDomain(0.5, 9.5).Nice(10)
	d := q.Domain()
	closeTo(t, d[0], 0, "quantize Nice low")
	closeTo(t, d[1], 10, "quantize Nice high")
	if len(q.Ticks(10)) == 0 {
		t.Error("quantize Ticks returned nothing")
	}
}

// TestQuantile uses d3's own worked example so the R-7 interpolation is pinned
// to a known vector rather than to my arithmetic.
func TestQuantile(t *testing.T) {
	data := []float64{3, 6, 7, 8, 8, 10, 13, 15, 16, 20}
	q := NewQuantile[string]("a", "b", "c", "d").SetDomain(data...)

	got := q.Quantiles()
	want := []float64{7.25, 9, 14.5}
	if len(got) != len(want) {
		t.Fatalf("Quantiles() = %v, want %v", got, want)
	}
	for i := range got {
		closeTo(t, got[i], want[i], "Quantiles")
	}

	tests := []struct {
		in   float64
		want string
	}{
		{3, "a"},
		{7.24, "a"},
		{7.25, "b"},
		{9, "c"},
		{14.5, "d"},
		{20, "d"},
		{1000, "d"},
	}
	for _, tt := range tests {
		if v := q.Scale(tt.in); v != tt.want {
			t.Errorf("Quantile.Scale(%v) = %q, want %q", tt.in, v, tt.want)
		}
	}

	lo, hi := q.InvertExtent("a")
	closeTo(t, lo, 3, "InvertExtent(a) lo is the data minimum")
	closeTo(t, hi, 7.25, "InvertExtent(a) hi")
	lo, hi = q.InvertExtent("d")
	closeTo(t, lo, 14.5, "InvertExtent(d) lo")
	closeTo(t, hi, 20, "InvertExtent(d) hi is the data maximum")

	// The defining property: the buckets are equally POPULATED, up to the
	// granularity the sample allows. Ten observations across four buckets
	// cannot be exactly 2.5 each, and a tie (there are two 8s here) can push a
	// value across a cut, so the guarantee is ±1 rather than exact.
	counts := map[string]int{}
	for _, v := range data {
		counts[q.Scale(v)]++
	}
	for _, k := range []string{"a", "b", "c", "d"} {
		if counts[k] < 2 || counts[k] > 3 {
			t.Errorf("bucket %q holds %d observations, want 2 or 3", k, counts[k])
		}
	}

	// Unsorted input and NaNs are handled by SetDomain.
	u := NewQuantile[string]("a", "b").SetDomain(20, math.NaN(), 3, 10)
	d := u.Domain()
	if len(d) != 3 || d[0] != 3 || d[2] != 20 {
		t.Errorf("SetDomain did not sort/filter: %v", d)
	}
}

func TestThreshold(t *testing.T) {
	th := NewThreshold[string]([]float64{0, 1}, "cold", "mild", "hot")

	tests := []struct {
		in   float64
		want string
	}{
		{-100, "cold"},
		{-0.001, "cold"},
		{0, "mild"}, // on a cut goes to the bucket above
		{0.5, "mild"},
		{0.999, "mild"},
		{1, "hot"},
		{1000, "hot"},
	}
	for _, tt := range tests {
		if got := th.Scale(tt.in); got != tt.want {
			t.Errorf("Threshold.Scale(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}

	lo, hi := th.InvertExtent("mild")
	closeTo(t, lo, 0, "InvertExtent(mild) lo")
	closeTo(t, hi, 1, "InvertExtent(mild) hi")

	// The outermost buckets are unbounded.
	lo, hi = th.InvertExtent("cold")
	if !math.IsInf(lo, -1) {
		t.Errorf("InvertExtent(cold) lo = %v, want -Inf", lo)
	}
	closeTo(t, hi, 0, "InvertExtent(cold) hi")
	lo, hi = th.InvertExtent("hot")
	closeTo(t, lo, 1, "InvertExtent(hot) lo")
	if !math.IsInf(hi, 1) {
		t.Errorf("InvertExtent(hot) hi = %v, want +Inf", hi)
	}
}

// TestQuantizeVersusQuantileVersusThreshold runs one skewed sample through all
// three scales. They disagree, and the disagreement IS the point: quantize cuts
// the domain evenly, quantile cuts the population evenly, threshold cuts where
// you say.
func TestQuantizeVersusQuantileVersusThreshold(t *testing.T) {
	// Nine values, eight of them small and one enormous.
	data := []float64{1, 2, 3, 4, 5, 6, 7, 8, 900}

	quantize := NewQuantize[string]("low", "mid", "high").SetDomain(1, 900)
	quantile := NewQuantile[string]("low", "mid", "high").SetDomain(data...)
	threshold := NewThreshold[string]([]float64{4, 8}, "low", "mid", "high")

	count := func(f func(float64) string) map[string]int {
		m := map[string]int{}
		for _, v := range data {
			m[f(v)]++
		}
		return m
	}

	qz := count(quantize.Scale)
	ql := count(quantile.Scale)
	th := count(threshold.Scale)

	// Quantize splits [1, 900] into thirds, so the outlier is alone at the top
	// and everything else piles into the first bucket.
	if qz["low"] != 8 || qz["mid"] != 0 || qz["high"] != 1 {
		t.Errorf("quantize buckets = %v, want 8/0/1", qz)
	}

	// Quantile splits the population into thirds regardless of the outlier.
	if ql["low"] != 3 || ql["mid"] != 3 || ql["high"] != 3 {
		t.Errorf("quantile buckets = %v, want 3/3/3", ql)
	}

	// Threshold does exactly what the cuts say: <4, [4,8), >=8.
	if th["low"] != 3 || th["mid"] != 4 || th["high"] != 2 {
		t.Errorf("threshold buckets = %v, want 3/4/2", th)
	}

	// And the three disagree on a specific value, which is the trap.
	if quantize.Scale(5) == quantile.Scale(5) && quantile.Scale(5) == threshold.Scale(5) {
		t.Error("expected the three scales to disagree on 5")
	}

	// Their domains mean different things, too: quantize and threshold take
	// two numbers and cut points respectively, quantile takes every datum.
	if len(quantile.Domain()) != len(data) {
		t.Errorf("quantile domain should hold every observation, got %d", len(quantile.Domain()))
	}
	if len(quantize.Domain()) != 2 {
		t.Errorf("quantize domain should be an extent, got %v", quantize.Domain())
	}
	if len(threshold.Domain()) != 2 {
		t.Errorf("threshold domain should be the cut points, got %v", threshold.Domain())
	}
}

func TestDiscreteUnknown(t *testing.T) {
	q := NewQuantize[string]("a", "b").SetUnknown("?")
	if got := q.Scale(math.NaN()); got != "?" {
		t.Errorf("Quantize.Scale(NaN) = %q, want %q", got, "?")
	}
	ql := NewQuantile[string]("a", "b").SetUnknown("?").SetDomain(1, 2, 3)
	if got := ql.Scale(math.NaN()); got != "?" {
		t.Errorf("Quantile.Scale(NaN) = %q, want %q", got, "?")
	}
	th := NewThreshold[string]([]float64{1}, "a", "b").SetUnknown("?")
	if got := th.Scale(math.NaN()); got != "?" {
		t.Errorf("Threshold.Scale(NaN) = %q, want %q", got, "?")
	}
}

func TestDiscreteCopiesAreIndependent(t *testing.T) {
	q := NewQuantize[string]("a", "b").SetDomain(0, 10)
	c := q.Copy().SetDomain(0, 100)
	if q.Scale(6) != "b" || c.Scale(6) != "a" {
		t.Errorf("Quantize.Copy shares state: %q %q", q.Scale(6), c.Scale(6))
	}
}
