package array

import (
	"math"
	"slices"
	"testing"
)

func TestAscendingDescending(t *testing.T) {
	tests := []struct {
		name     string
		a, b     float64
		wantAsc  int
		wantDesc int
	}{
		{"less", 1, 2, -1, 1},
		{"greater", 2, 1, 1, -1},
		{"equal", 1, 1, 0, 0},
		{"negatives", -2, -1, -1, 1},
		{"zero and negative zero", 0, math.Copysign(0, -1), 0, 0},
		// NaN sorts last in *both* directions. Reversing an order should not
		// drag missing data to the front of a chart.
		{"nan is last ascending", nan, 1, 1, 1},
		{"nan is last descending", 1, nan, -1, -1},
		{"nan vs nan", nan, nan, 0, 0},
		{"infinities", math.Inf(-1), math.Inf(1), -1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Ascending(tt.a, tt.b); got != tt.wantAsc {
				t.Errorf("Ascending(%v, %v) = %d, want %d", tt.a, tt.b, got, tt.wantAsc)
			}
			if got := Descending(tt.a, tt.b); got != tt.wantDesc {
				t.Errorf("Descending(%v, %v) = %d, want %d", tt.a, tt.b, got, tt.wantDesc)
			}
		})
	}
}

func TestAscendingStrings(t *testing.T) {
	// The comparators are generic over cmp.Ordered, not just numbers.
	if got := Ascending("a", "b"); got != -1 {
		t.Errorf(`Ascending("a","b") = %d, want -1`, got)
	}
	if got := Descending("a", "b"); got != 1 {
		t.Errorf(`Descending("a","b") = %d, want 1`, got)
	}
}

// TestAscendingSortsNaNLast is the practical consequence, and the reason
// Ascending is not simply cmp.Compare (which sorts NaN first).
func TestAscendingSortsNaNLast(t *testing.T) {
	in := []float64{3, nan, 1, nan, 2}
	slices.SortStableFunc(in, Ascending[float64])
	if in[0] != 1 || in[1] != 2 || in[2] != 3 {
		t.Errorf("defined values did not sort to the front: %v", in)
	}
	if !math.IsNaN(in[3]) || !math.IsNaN(in[4]) {
		t.Errorf("NaN did not sort last: %v", in)
	}
}

func TestBisect(t *testing.T) {
	tests := []struct {
		name      string
		a         []float64
		x         float64
		wantLeft  int
		wantRight int
	}{
		{"empty slice", nil, 1, 0, 0},
		{"single, below", []float64{2}, 1, 0, 0},
		{"single, equal", []float64{2}, 2, 0, 1},
		{"single, above", []float64{2}, 3, 1, 1},
		{"before everything", []float64{1, 2, 3}, 0, 0, 0},
		{"after everything", []float64{1, 2, 3}, 4, 3, 3},
		{"between", []float64{1, 2, 3}, 1.5, 1, 1},
		// The whole point of having both: on an exact hit, left lands before
		// the match and right lands after it.
		{"exact hit at start", []float64{1, 2, 3}, 1, 0, 1},
		{"exact hit in middle", []float64{1, 2, 3}, 2, 1, 2},
		{"exact hit at end", []float64{1, 2, 3}, 3, 2, 3},
		// With duplicates the gap between left and right is the run length.
		{"run of duplicates", []float64{1, 2, 2, 2, 3}, 2, 1, 4},
		{"all equal, hit", []float64{5, 5, 5}, 5, 0, 3},
		{"all equal, below", []float64{5, 5, 5}, 4, 0, 0},
		{"all equal, above", []float64{5, 5, 5}, 6, 3, 3},
		// An undefined needle belongs past the end rather than at an arbitrary
		// position, matching d3.
		{"nan needle", []float64{1, 2, 3}, nan, 3, 3},
		{"nan needle, empty slice", nil, nan, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BisectLeft(tt.a, tt.x); got != tt.wantLeft {
				t.Errorf("BisectLeft(%v, %v) = %d, want %d", tt.a, tt.x, got, tt.wantLeft)
			}
			if got := BisectRight(tt.a, tt.x); got != tt.wantRight {
				t.Errorf("BisectRight(%v, %v) = %d, want %d", tt.a, tt.x, got, tt.wantRight)
			}
			// Bisect is BisectRight, as in d3.
			if got := Bisect(tt.a, tt.x); got != tt.wantRight {
				t.Errorf("Bisect(%v, %v) = %d, want %d", tt.a, tt.x, got, tt.wantRight)
			}
		})
	}
}

func TestBisectStrings(t *testing.T) {
	a := []string{"a", "c", "e"}
	if got := BisectLeft(a, "c"); got != 1 {
		t.Errorf(`BisectLeft(%v, "c") = %d, want 1`, a, got)
	}
	if got := BisectRight(a, "c"); got != 2 {
		t.Errorf(`BisectRight(%v, "c") = %d, want 2`, a, got)
	}
	if got := BisectLeft(a, "d"); got != 2 {
		t.Errorf(`BisectLeft(%v, "d") = %d, want 2`, a, got)
	}
}

func TestBisectRange(t *testing.T) {
	a := []float64{0, 1, 2, 3, 4, 5}
	tests := []struct {
		name         string
		x            float64
		lo, hi       int
		wantL, wantR int
	}{
		{"full range", 3, 0, 6, 3, 4},
		{"window excludes the hit above", 3, 0, 3, 3, 3},
		{"window excludes the hit below", 1, 3, 6, 3, 3},
		{"empty window", 3, 2, 2, 2, 2},
		{"single-element window, hit", 3, 3, 4, 3, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BisectLeftRange(a, tt.x, tt.lo, tt.hi); got != tt.wantL {
				t.Errorf("BisectLeftRange(%v, %v, %d, %d) = %d, want %d", a, tt.x, tt.lo, tt.hi, got, tt.wantL)
			}
			if got := BisectRightRange(a, tt.x, tt.lo, tt.hi); got != tt.wantR {
				t.Errorf("BisectRightRange(%v, %v, %d, %d) = %d, want %d", a, tt.x, tt.lo, tt.hi, got, tt.wantR)
			}
		})
	}
}

// TestBisectInsertionPoint is the defining property: inserting x at the
// returned index keeps the slice sorted, whichever variant you use.
func TestBisectInsertionPoint(t *testing.T) {
	a := []float64{-5, -1, 0, 0, 2, 7, 7, 7, 11}
	for _, x := range []float64{-9, -5, -3, 0, 1, 7, 8, 11, 99} {
		for _, i := range []int{BisectLeft(a, x), BisectRight(a, x)} {
			inserted := slices.Insert(slices.Clone(a), i, x)
			if !slices.IsSorted(inserted) {
				t.Errorf("inserting %v at %d gave an unsorted slice: %v", x, i, inserted)
			}
		}
	}
}

func TestBisectCenter(t *testing.T) {
	a := []float64{0, 10, 20, 30}
	tests := []struct {
		name string
		x    float64
		want int
	}{
		{"below everything", -5, 0},
		{"exact hit", 20, 2},
		{"nearer the lower neighbour", 12, 1},
		{"nearer the upper neighbour", 18, 2},
		// A value exactly between two samples goes to the later one.
		{"exact midpoint", 15, 2},
		{"above everything", 99, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BisectCenter(a, tt.x); got != tt.want {
				t.Errorf("BisectCenter(%v, %v) = %d, want %d", a, tt.x, got, tt.want)
			}
		})
	}
	if got := BisectCenter(nil, 1); got != 0 {
		t.Errorf("BisectCenter(nil, 1) = %d, want 0", got)
	}
	if got := BisectCenter([]float64{7}, 100); got != 0 {
		t.Errorf("BisectCenter([7], 100) = %d, want 0", got)
	}
}

// TestBisectCenterIsNearest checks the property directly across the whole
// number line rather than trusting a handful of hand-picked cases.
func TestBisectCenterIsNearest(t *testing.T) {
	a := []float64{-3, 0, 1, 1.5, 8, 100}
	for x := -10.0; x <= 110; x += 0.25 {
		i := BisectCenter(a, x)
		best := math.Abs(a[i] - x)
		for _, v := range a {
			if d := math.Abs(v - x); d < best-1e-12 {
				t.Fatalf("BisectCenter(a, %v) chose %v (distance %v) over %v (distance %v)", x, a[i], best, v, d)
			}
		}
	}
}

// trade exercises the Bisector over a struct sorted by one of its fields, the
// case the type actually exists for.
type trade struct {
	at    float64
	price float64
}

func TestBisector(t *testing.T) {
	trades := []trade{{1, 10}, {2, 20}, {2, 21}, {5, 50}, {9, 90}}
	b := BisectorOf(func(tr trade) float64 { return tr.at })

	tests := []struct {
		name         string
		x            float64
		wantL, wantR int
		wantCenter   int
	}{
		{"before everything", 0, 0, 0, 0},
		{"exact hit with duplicates", 2, 1, 3, 1},
		{"between samples, nearer below", 5.5, 4, 4, 3},
		{"between samples, nearer above", 8.5, 4, 4, 4},
		{"after everything", 100, 5, 5, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := b.Left(trades, tt.x); got != tt.wantL {
				t.Errorf("Left(%v) = %d, want %d", tt.x, got, tt.wantL)
			}
			if got := b.Right(trades, tt.x); got != tt.wantR {
				t.Errorf("Right(%v) = %d, want %d", tt.x, got, tt.wantR)
			}
			if got := b.Center(trades, tt.x); got != tt.wantCenter {
				t.Errorf("Center(%v) = %d, want %d", tt.x, got, tt.wantCenter)
			}
		})
	}

	if got := b.Left(nil, 1); got != 0 {
		t.Errorf("Left on an empty slice = %d, want 0", got)
	}
	if got := b.Center(nil, 1); got != 0 {
		t.Errorf("Center on an empty slice = %d, want 0", got)
	}
}

func TestBisectorBy(t *testing.T) {
	// A needle of a different type from the element: search []trade by name is
	// the shape, here approximated with a string key.
	type row struct{ key string }
	rows := []row{{"alpha"}, {"beta"}, {"gamma"}}
	b := BisectorBy(func(r row) string { return r.key })
	if got := b.Left(rows, "beta"); got != 1 {
		t.Errorf(`Left("beta") = %d, want 1`, got)
	}
	if got := b.Right(rows, "beta"); got != 2 {
		t.Errorf(`Right("beta") = %d, want 2`, got)
	}
	// Without a distance metric, Center degrades to Left rather than lying.
	if got, want := b.Center(rows, "beta"), b.Left(rows, "beta"); got != want {
		t.Errorf(`Center("beta") = %d, want %d (Left)`, got, want)
	}
}

func TestNewBisectorComparator(t *testing.T) {
	// Elements and needles of genuinely different types: search a slice of
	// trades by price bucket index.
	trades := []trade{{1, 10}, {2, 20}, {5, 50}}
	b := NewBisector(func(tr trade, x int) int { return Ascending(int(tr.price), x) })
	if got := b.Left(trades, 20); got != 1 {
		t.Errorf("Left(20) = %d, want 1", got)
	}
	if got := b.Right(trades, 20); got != 2 {
		t.Errorf("Right(20) = %d, want 2", got)
	}
}

func TestQuickselect(t *testing.T) {
	tests := []struct {
		name string
		in   []float64
		k    int
	}{
		{"empty", nil, 0},
		{"single", []float64{1}, 0},
		{"two, first", []float64{2, 1}, 0},
		{"two, second", []float64{2, 1}, 1},
		{"median of odd", []float64{9, 3, 7, 1, 5}, 2},
		{"first of many", []float64{9, 3, 7, 1, 5, 2, 8, 4, 6}, 0},
		{"last of many", []float64{9, 3, 7, 1, 5, 2, 8, 4, 6}, 8},
		{"all equal", []float64{5, 5, 5, 5}, 2},
		{"already sorted", []float64{1, 2, 3, 4, 5}, 3},
		{"reverse sorted", []float64{5, 4, 3, 2, 1}, 1},
		{"with duplicates", []float64{3, 1, 3, 1, 3, 1}, 3},
		{"negatives", []float64{-1, -5, 3, 0}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := slices.Clone(tt.in)
			QuickselectOf(in, tt.k)
			if len(in) == 0 {
				return
			}
			sorted := slices.Clone(tt.in)
			slices.Sort(sorted)
			// The k-th element must be exactly where a full sort would put it…
			if in[tt.k] != sorted[tt.k] {
				t.Errorf("QuickselectOf(%v, %d): element at k is %v, want %v", tt.in, tt.k, in[tt.k], sorted[tt.k])
			}
			// …with everything below it on the left and above it on the right.
			for i := 0; i < tt.k; i++ {
				if in[i] > in[tt.k] {
					t.Errorf("element %v at %d is greater than the pivot %v", in[i], i, in[tt.k])
				}
			}
			for i := tt.k + 1; i < len(in); i++ {
				if in[i] < in[tt.k] {
					t.Errorf("element %v at %d is less than the pivot %v", in[i], i, in[tt.k])
				}
			}
			// And it is a permutation of the input — nothing lost or invented.
			permuted := slices.Clone(in)
			slices.Sort(permuted)
			if !slices.Equal(permuted, sorted) {
				t.Errorf("QuickselectOf is not a permutation: %v vs %v", permuted, sorted)
			}
		})
	}
}

// TestQuickselectLarge exercises the Floyd–Rivest sampling branch, which only
// runs on ranges wider than 600 elements and would otherwise never be covered.
func TestQuickselectLarge(t *testing.T) {
	const n = 5000
	in := make([]float64, n)
	for i := range in {
		in[i] = float64((i * 7919) % n) // a fixed, thoroughly scrambled permutation
	}
	for _, k := range []int{0, 1, n / 2, n - 2, n - 1} {
		v := slices.Clone(in)
		QuickselectOf(v, k)
		if v[k] != float64(k) {
			t.Errorf("QuickselectOf(large, %d) put %v at k, want %v", k, v[k], float64(k))
		}
	}
}

func TestQuickselectOutOfRangeK(t *testing.T) {
	// k outside the slice is clamped rather than panicking.
	in := []float64{3, 1, 2}
	QuickselectOf(in, -5)
	QuickselectOf(in, 99)
	sorted := slices.Clone(in)
	slices.Sort(sorted)
	if !slices.Equal(sorted, []float64{1, 2, 3}) {
		t.Errorf("Quickselect lost elements: %v", in)
	}
	QuickselectOf(nil, 0) // must not panic
}

func TestQuickselectCustomComparator(t *testing.T) {
	trades := []trade{{5, 50}, {1, 10}, {9, 90}, {2, 20}}
	Quickselect(trades, 1, func(a, b trade) int { return Ascending(a.at, b.at) })
	if trades[1].at != 2 {
		t.Errorf("Quickselect by .at put %v at index 1, want at == 2", trades[1])
	}
}
