package array

import (
	"errors"
	"math"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
)

// fruit is a small fixture with an obvious grouping key.
type fruit struct {
	name  string
	kind  string
	count int
}

var basket = []fruit{
	{"apple", "pome", 3},
	{"pear", "pome", 5},
	{"cherry", "drupe", 7},
	{"plum", "drupe", 2},
	{"fig", "syconium", 1},
}

func kindOf(f fruit) string { return f.kind }

func TestGroup(t *testing.T) {
	got := Group(basket, kindOf)
	want := map[string][]fruit{
		"pome":     {basket[0], basket[1]},
		"drupe":    {basket[2], basket[3]},
		"syconium": {basket[4]},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Group = %v, want %v", got, want)
	}
	if got := Group([]fruit(nil), kindOf); len(got) != 0 {
		t.Errorf("Group(nil) = %v, want empty", got)
	}
}

// TestGroupsOrder is the reason Groups exists alongside Group: Go's map
// iteration is randomized, so anything that renders groups in order needs a
// slice. Running it repeatedly would catch an implementation that ranged over
// the map.
func TestGroupsOrder(t *testing.T) {
	for trial := 0; trial < 20; trial++ {
		got := Groups(basket, kindOf)
		wantKeys := []string{"pome", "drupe", "syconium"}
		gotKeys := make([]string, len(got))
		for i, e := range got {
			gotKeys[i] = e.Key
		}
		if !slices.Equal(gotKeys, wantKeys) {
			t.Fatalf("Groups keys = %v, want %v (first-seen order)", gotKeys, wantKeys)
		}
	}
	got := Groups(basket, kindOf)
	if len(got[0].Value) != 2 || got[0].Value[0].name != "apple" || got[0].Value[1].name != "pear" {
		t.Errorf("first group = %v, want apple then pear in input order", got[0].Value)
	}
	if len(Groups([]fruit(nil), kindOf)) != 0 {
		t.Error("Groups(nil) should be empty")
	}
}

func TestRollup(t *testing.T) {
	total := func(fs []fruit) int {
		n := 0
		for _, f := range fs {
			n += f.count
		}
		return n
	}
	got := Rollup(basket, total, kindOf)
	want := map[string]int{"pome": 8, "drupe": 9, "syconium": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Rollup = %v, want %v", got, want)
	}

	gotOrdered := Rollups(basket, total, kindOf)
	wantOrdered := []Entry[string, int]{{"pome", 8}, {"drupe", 9}, {"syconium", 1}}
	if !reflect.DeepEqual(gotOrdered, wantOrdered) {
		t.Errorf("Rollups = %v, want %v", gotOrdered, wantOrdered)
	}

	if len(Rollups([]fruit(nil), total, kindOf)) != 0 {
		t.Error("Rollups(nil) should be empty")
	}
}

func TestIndex(t *testing.T) {
	byName := func(f fruit) string { return f.name }
	got, err := Index(basket, byName)
	if err != nil {
		t.Fatalf("Index of unique keys returned %v", err)
	}
	if len(got) != len(basket) || got["cherry"].count != 7 {
		t.Errorf("Index = %v", got)
	}

	gotOrdered, err := Indexes(basket, byName)
	if err != nil {
		t.Fatalf("Indexes of unique keys returned %v", err)
	}
	if len(gotOrdered) != len(basket) || gotOrdered[0].Key != "apple" {
		t.Errorf("Indexes = %v", gotOrdered)
	}
}

// TestIndexDuplicateKey: an index asserts uniqueness, so a collision is an
// error rather than a silent overwrite. The partial result is still returned so
// a caller who wants last-wins can have it.
func TestIndexDuplicateKey(t *testing.T) {
	got, err := Index(basket, kindOf)
	if !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("Index by a non-unique key returned err = %v, want ErrDuplicateKey", err)
	}
	if got["pome"].name != "pear" {
		t.Errorf("partial index kept %q for pome, want the last one (pear)", got["pome"].name)
	}

	gotOrdered, err := Indexes(basket, kindOf)
	if !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("Indexes by a non-unique key returned err = %v, want ErrDuplicateKey", err)
	}
	if len(gotOrdered) != 3 {
		t.Errorf("Indexes returned %d entries, want 3 distinct keys", len(gotOrdered))
	}
}

func TestMergeAndFlat(t *testing.T) {
	tests := []struct {
		name string
		in   [][]int
		want []int
	}{
		{"nothing", nil, []int{}},
		{"one empty group", [][]int{{}}, []int{}},
		{"several empty groups", [][]int{{}, {}, nil}, []int{}},
		{"single group", [][]int{{1, 2}}, []int{1, 2}},
		{"several groups", [][]int{{1}, {2, 3}, {4}}, []int{1, 2, 3, 4}},
		{"holes preserved in order", [][]int{{1}, nil, {2}}, []int{1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Merge(tt.in...); !slices.Equal(got, tt.want) {
				t.Errorf("Merge(%v) = %v, want %v", tt.in, got, tt.want)
			}
			if got := Flat(tt.in); !slices.Equal(got, tt.want) {
				t.Errorf("Flat(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestMergeCopies: the result must not alias an input, or appending to it would
// corrupt the caller's data.
func TestMergeCopies(t *testing.T) {
	a := []int{1, 2, 3}
	got := Merge(a)
	got[0] = 99
	if a[0] != 1 {
		t.Errorf("Merge aliased its input: %v", a)
	}
}

func TestPairs(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		want [][2]int
	}{
		{"empty", nil, [][2]int{}},
		{"single element has no pairs", []int{1}, [][2]int{}},
		{"two elements", []int{1, 2}, [][2]int{{1, 2}}},
		{"several", []int{1, 2, 3, 4}, [][2]int{{1, 2}, {2, 3}, {3, 4}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pairs(tt.in)
			if !slices.Equal(got, tt.want) {
				t.Errorf("Pairs(%v) = %v, want %v", tt.in, got, tt.want)
			}
			if len(tt.in) > 0 && len(got) != len(tt.in)-1 {
				t.Errorf("Pairs(%v) returned %d pairs, want %d", tt.in, len(got), len(tt.in)-1)
			}
		})
	}
}

func TestPairsWith(t *testing.T) {
	// The canonical use: successive differences of a series.
	got := PairsWith([]float64{1, 4, 9, 16}, func(a, b float64) float64 { return b - a })
	want := []float64{3, 5, 7}
	if !slices.Equal(got, want) {
		t.Errorf("PairsWith differences = %v, want %v", got, want)
	}
}

func TestPermute(t *testing.T) {
	src := []string{"a", "b", "c", "d"}
	tests := []struct {
		name    string
		indexes []int
		want    []string
	}{
		{"identity", []int{0, 1, 2, 3}, []string{"a", "b", "c", "d"}},
		{"reverse", []int{3, 2, 1, 0}, []string{"d", "c", "b", "a"}},
		{"subset", []int{1, 3}, []string{"b", "d"}},
		{"repeats", []int{0, 0, 0}, []string{"a", "a", "a"}},
		{"empty index list", nil, []string{}},
		// Out-of-range indexes are dropped: Go has no undefined to insert.
		{"out of range dropped", []int{0, 99, -1, 2}, []string{"a", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Permute(src, tt.indexes); !slices.Equal(got, tt.want) {
				t.Errorf("Permute(%v, %v) = %v, want %v", src, tt.indexes, got, tt.want)
			}
		})
	}
	if got := Permute([]string(nil), []int{0}); len(got) != 0 {
		t.Errorf("Permute(nil, [0]) = %v, want empty", got)
	}
}

// TestShuffleIsAPermutation checks the only thing a shuffle can be checked for
// without asserting a particular random sequence: it must rearrange, never add
// or drop.
func TestShuffleIsAPermutation(t *testing.T) {
	for n := 0; n < 30; n++ {
		orig := make([]int, n)
		for i := range orig {
			orig[i] = i
		}
		got := slices.Clone(orig)
		ret := Shuffle(got)
		if n > 0 && &ret[0] != &got[0] {
			t.Error("Shuffle should shuffle in place and return the same slice")
		}
		sorted := slices.Clone(got)
		slices.Sort(sorted)
		if !slices.Equal(sorted, orig) {
			t.Fatalf("Shuffle(%d elements) is not a permutation: %v", n, got)
		}
	}
}

// TestShuffleWithIsReproducible pins the reason ShuffleWith exists: the same
// generator state must produce the same permutation, so a deterministic render
// stays deterministic.
func TestShuffleWithIsReproducible(t *testing.T) {
	run := func() []int {
		v := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		r := rand.New(rand.NewPCG(42, 42))
		return ShuffleWith(v, r.Float64)
	}
	a, b := run(), run()
	if !slices.Equal(a, b) {
		t.Errorf("ShuffleWith is not reproducible: %v vs %v", a, b)
	}
	// A degenerate generator that always returns 0 must still terminate and
	// still yield a permutation.
	v := []int{1, 2, 3, 4}
	ShuffleWith(v, func() float64 { return 0 })
	sorted := slices.Clone(v)
	slices.Sort(sorted)
	if !slices.Equal(sorted, []int{1, 2, 3, 4}) {
		t.Errorf("ShuffleWith with a constant generator lost elements: %v", v)
	}
	// So must one that returns values at the top of the range.
	v = []int{1, 2, 3, 4}
	ShuffleWith(v, func() float64 { return math.Nextafter(1, 0) })
	sorted = slices.Clone(v)
	slices.Sort(sorted)
	if !slices.Equal(sorted, []int{1, 2, 3, 4}) {
		t.Errorf("ShuffleWith near 1 lost elements: %v", v)
	}
}

func TestRange(t *testing.T) {
	tests := []struct {
		name              string
		start, stop, step float64
		want              []float64
	}{
		// The stop value is excluded, as in Python and d3.
		{"unit steps", 0, 5, 1, []float64{0, 1, 2, 3, 4}},
		{"offset start", 2, 5, 1, []float64{2, 3, 4}},
		{"fractional step", 0, 1, 0.25, []float64{0, 0.25, 0.5, 0.75}},
		{"negative step", 5, 0, -1, []float64{5, 4, 3, 2, 1}},
		{"start equals stop", 3, 3, 1, []float64{}},
		{"stop below start with positive step", 5, 0, 1, []float64{}},
		{"stop above start with negative step", 0, 5, -1, []float64{}},
		{"non-integral count rounds up", 0, 1, 0.3, []float64{0, 0.3, 0.6, 0.8999999999999999}},
		// A zero or NaN step would never terminate; an empty result is the only
		// safe answer.
		{"zero step", 0, 5, 0, []float64{}},
		{"NaN step", 0, 5, nan, []float64{}},
		{"NaN start", nan, 5, 1, []float64{}},
		{"NaN stop", 0, nan, 1, []float64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Range(tt.start, tt.stop, tt.step)
			if !slices.Equal(got, tt.want) {
				t.Errorf("Range(%v, %v, %v) = %v, want %v", tt.start, tt.stop, tt.step, got, tt.want)
			}
		})
	}
	if got := RangeTo(4); !slices.Equal(got, []float64{0, 1, 2, 3}) {
		t.Errorf("RangeTo(4) = %v, want [0 1 2 3]", got)
	}
}

// TestRangeDoesNotAccumulateError is why the count is computed up front: adding
// 0.1 a hundred times drifts, multiplying does not.
func TestRangeDoesNotAccumulateError(t *testing.T) {
	got := Range(0, 1, 0.1)
	if len(got) != 10 {
		t.Fatalf("Range(0,1,0.1) has %d elements, want 10", len(got))
	}
	if got[3] != 0.30000000000000004 {
		// 3*0.1 is exactly this double; the point is that it equals 3*0.1 and
		// not the drifted 0.1+0.1+0.1.
		t.Errorf("Range(0,1,0.1)[3] = %v, want %v", got[3], 3*0.1)
	}
}

func TestTranspose(t *testing.T) {
	tests := []struct {
		name string
		in   [][]int
		want [][]int
	}{
		{"empty", nil, [][]int{}},
		{"single row", [][]int{{1, 2, 3}}, [][]int{{1}, {2}, {3}}},
		{"single column", [][]int{{1}, {2}, {3}}, [][]int{{1, 2, 3}}},
		{"square", [][]int{{1, 2}, {3, 4}}, [][]int{{1, 3}, {2, 4}}},
		{"rectangular", [][]int{{1, 2, 3}, {4, 5, 6}}, [][]int{{1, 4}, {2, 5}, {3, 6}}},
		// Ragged input is truncated to the shortest row so the result stays
		// rectangular; a partly-filled tuple would be worse than a missing one.
		{"ragged truncates", [][]int{{1, 2, 3}, {4, 5}}, [][]int{{1, 4}, {2, 5}}},
		{"one empty row makes it empty", [][]int{{1, 2}, {}}, [][]int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Transpose(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Transpose(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestTransposeInvolution: transposing a rectangular matrix twice gives it back.
func TestTransposeInvolution(t *testing.T) {
	in := [][]int{{1, 2, 3}, {4, 5, 6}, {7, 8, 9}, {10, 11, 12}}
	if got := Transpose(Transpose(in)); !reflect.DeepEqual(got, in) {
		t.Errorf("Transpose twice = %v, want %v", got, in)
	}
}

func TestZip(t *testing.T) {
	got := Zip([]int{1, 2}, []int{3, 4}, []int{5, 6})
	want := [][]int{{1, 3, 5}, {2, 4, 6}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Zip = %v, want %v", got, want)
	}
	if got := Zip[int](); len(got) != 0 {
		t.Errorf("Zip() = %v, want empty", got)
	}
	if got := Zip([]int{1, 2, 3}, []int{4}); !reflect.DeepEqual(got, [][]int{{1, 4}}) {
		t.Errorf("Zip with a short input = %v, want [[1 4]]", got)
	}
}

// TestShuffleWithOutOfRangeGenerator: a generator that violates the [0,1)
// contract must not index out of bounds. Clamping is cheaper than a panic in
// somebody's render loop.
func TestShuffleWithOutOfRangeGenerator(t *testing.T) {
	for _, r := range []func() float64{
		func() float64 { return -1 },
		func() float64 { return 1 },
		func() float64 { return 42 },
	} {
		v := []int{1, 2, 3, 4, 5}
		ShuffleWith(v, r)
		sorted := slices.Clone(v)
		slices.Sort(sorted)
		if !slices.Equal(sorted, []int{1, 2, 3, 4, 5}) {
			t.Errorf("ShuffleWith with an out-of-range generator lost elements: %v", v)
		}
	}
}

// TestRangeRefusesAbsurdLengths: a denormal step over a wide domain would ask
// for more elements than exist atoms; an empty result beats an OOM.
func TestRangeRefusesAbsurdLengths(t *testing.T) {
	if got := Range(0, 1e300, 1e-300); len(got) != 0 {
		t.Errorf("Range with an absurd length returned %d elements, want 0", len(got))
	}
}
