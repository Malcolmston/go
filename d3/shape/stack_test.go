package shape_test

import (
	"math"
	"testing"

	"github.com/malcolmston/d3/shape"
)

type row map[string]float64

func rowValue(d row, key string, _ int, _ []row) float64 { return d[key] }

func newStack(keys ...string) *shape.Stack[row] {
	return shape.NewStack[row]().Keys(keys...).Value(rowValue)
}

// bounds flattens a stack result into [series][datum][lower, upper] so the
// tests can assert on numbers rather than on structs.
func bounds[T any](series []shape.Series[T]) [][][2]float64 {
	out := make([][][2]float64, len(series))
	for i, s := range series {
		out[i] = make([][2]float64, len(s.Points))
		for j, p := range s.Points {
			out[i][j] = [2]float64{p.Lower, p.Upper}
		}
	}
	return out
}

func assertBounds(t *testing.T, got [][][2]float64, want [][][2]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("series count = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("series %d length = %d, want %d", i, len(got[i]), len(want[i]))
		}
		for j := range want[i] {
			for k := 0; k < 2; k++ {
				if math.Abs(got[i][j][k]-want[i][j][k]) > 1e-9 {
					t.Errorf("series %d datum %d bound %d = %v, want %v",
						i, j, k, got[i][j][k], want[i][j][k])
				}
			}
		}
	}
}

var stackData = []row{
	{"a": 1, "b": 2, "c": 3},
	{"a": 4, "b": 5, "c": 6},
}

// TestStackOffsetNone is the plain stacked chart: each layer sits on the
// cumulative total below it.
func TestStackOffsetNone(t *testing.T) {
	got := bounds(newStack("a", "b", "c").Generate(stackData))
	assertBounds(t, got, [][][2]float64{
		{{0, 1}, {0, 4}},
		{{1, 3}, {4, 9}},
		{{3, 6}, {9, 15}},
	})
}

// TestStackOffsetExpand normalizes each column to 1 — the defining property.
func TestStackOffsetExpand(t *testing.T) {
	series := newStack("a", "b", "c").Offset(shape.OffsetExpand).Generate(stackData)
	got := bounds(series)
	assertBounds(t, got, [][][2]float64{
		{{0, 1.0 / 6}, {0, 4.0 / 15}},
		{{1.0 / 6, 3.0 / 6}, {4.0 / 15, 9.0 / 15}},
		{{3.0 / 6, 1}, {9.0 / 15, 1}},
	})
	// The property, stated directly: every column tops out at exactly 1.
	for j := range stackData {
		closeTo(t, "column total", got[len(got)-1][j][1], 1, 1e-12)
		closeTo(t, "column base", got[0][j][0], 0, 1e-12)
	}
}

// TestStackOffsetExpandZeroColumn: a column that sums to zero must be left
// alone rather than producing infinities.
func TestStackOffsetExpandZeroColumn(t *testing.T) {
	data := []row{{"a": 0, "b": 0}}
	got := bounds(newStack("a", "b").Offset(shape.OffsetExpand).Generate(data))
	assertBounds(t, got, [][][2]float64{{{0, 0}}, {{0, 0}}})
}

// TestStackOffsetDiverging sends positives up and negatives down from zero, so
// a chart of gains and losses does not cancel out.
func TestStackOffsetDiverging(t *testing.T) {
	data := []row{{"a": 1, "b": -2, "c": 3}}
	got := bounds(newStack("a", "b", "c").Offset(shape.OffsetDiverging).Generate(data))
	assertBounds(t, got, [][][2]float64{
		{{0, 1}},  // first positive: 0..1
		{{-2, 0}}, // negative: hangs below zero
		{{1, 4}},  // second positive: continues from 1
	})
}

// TestStackOffsetSilhouette centers each column on zero.
func TestStackOffsetSilhouette(t *testing.T) {
	got := bounds(newStack("a", "b", "c").Offset(shape.OffsetSilhouette).Generate(stackData))
	// Column totals are 6 and 15, so the baselines are -3 and -7.5.
	assertBounds(t, got, [][][2]float64{
		{{-3, -2}, {-7.5, -3.5}},
		{{-2, 0}, {-3.5, 1.5}},
		{{0, 3}, {1.5, 7.5}},
	})
	for j := range stackData {
		top := got[len(got)-1][j][1]
		bottom := got[0][j][0]
		closeTo(t, "symmetric about zero", top+bottom, 0, 1e-12)
	}
}

// TestStackOffsetWiggle: the documented behavior is that the baseline starts
// at zero and then wanders to minimize the layers' weighted slope, while every
// layer keeps its original thickness.
func TestStackOffsetWiggle(t *testing.T) {
	data := []row{
		{"a": 1, "b": 2, "c": 1},
		{"a": 2, "b": 1, "c": 3},
		{"a": 3, "b": 4, "c": 1},
	}
	series := newStack("a", "b", "c").Offset(shape.OffsetWiggle).Generate(data)
	got := bounds(series)

	closeTo(t, "the wiggle baseline starts at zero", got[0][0][0], 0, 1e-12)

	// Thicknesses are preserved: an offset only moves layers, never resizes
	// them.
	for i, key := range []string{"a", "b", "c"} {
		for j, d := range data {
			closeTo(t, "thickness", got[i][j][1]-got[i][j][0], d[key], 1e-9)
		}
	}
	// Layers stay contiguous: each one starts where the previous ended.
	for i := 1; i < len(got); i++ {
		for j := range data {
			closeTo(t, "contiguous", got[i][j][0], got[i-1][j][1], 1e-9)
		}
	}
	// The baseline actually moves — otherwise this is just silhouette.
	if math.Abs(got[0][1][0]-got[0][0][0]) < 1e-12 {
		t.Error("wiggle must move the baseline between columns")
	}
}

func TestStackOrders(t *testing.T) {
	// Totals: a = 5, b = 7, c = 9.
	data := []row{{"a": 1, "b": 3, "c": 4}, {"a": 4, "b": 4, "c": 5}}

	tests := []struct {
		name  string
		order shape.StackOrder
		// wantIndex is the stacking position of series a, b, c.
		wantIndex []int
	}{
		{"none keeps key order", shape.OrderNone, []int{0, 1, 2}},
		{"reverse", shape.OrderReverse, []int{2, 1, 0}},
		{"ascending puts the smallest at the bottom", shape.OrderAscending, []int{0, 1, 2}},
		{"descending puts the largest at the bottom", shape.OrderDescending, []int{2, 1, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			series := newStack("a", "b", "c").Order(tt.order).Generate(data)
			for i, s := range series {
				if s.Index != tt.wantIndex[i] {
					t.Errorf("series %q index = %d, want %d", s.Key, s.Index, tt.wantIndex[i])
				}
			}
		})
	}
}

// TestStackOrderAscendingStacksAccordingly checks that the order actually
// drives the offset, not just the reported Index.
func TestStackOrderAscendingStacksAccordingly(t *testing.T) {
	data := []row{{"big": 10, "small": 1}}
	series := newStack("big", "small").Order(shape.OrderAscending).Generate(data)
	got := bounds(series)
	// "small" (total 1) goes at the bottom, "big" on top of it.
	assertBounds(t, got, [][][2]float64{
		{{1, 11}},
		{{0, 1}},
	})
}

// TestStackOrderInsideOut places the earliest-peaking, largest series in the
// middle — the ordering a streamgraph needs.
func TestStackOrderInsideOut(t *testing.T) {
	// Each series peaks at a different column; totals are all equal so the
	// balancing is driven purely by peak order.
	data := []row{
		{"p0": 9, "p1": 1, "p2": 1, "p3": 1},
		{"p0": 1, "p1": 9, "p2": 1, "p3": 1},
		{"p0": 1, "p1": 1, "p2": 9, "p3": 1},
		{"p0": 1, "p1": 1, "p2": 1, "p3": 9},
	}
	series := newStack("p0", "p1", "p2", "p3").Order(shape.OrderInsideOut).Generate(data)
	index := map[string]int{}
	for _, s := range series {
		index[s.Key] = s.Index
	}
	// Peak order is p0, p1, p2, p3; insideOut alternates bottom, top, bottom,
	// top and then reverses the bottom half, giving p2, p0, p1, p3.
	want := map[string]int{"p2": 0, "p0": 1, "p1": 2, "p3": 3}
	for k, v := range want {
		if index[k] != v {
			t.Errorf("index = %v, want %v", index, want)
			break
		}
	}
	// The earliest-peaking series must not be at either edge.
	if index["p0"] == 0 || index["p0"] == 3 {
		t.Errorf("insideOut must keep the earliest peak away from the edges, got %d", index["p0"])
	}
}

func TestStackAppearanceOrder(t *testing.T) {
	data := []row{
		{"late": 1, "early": 5},
		{"late": 5, "early": 1},
	}
	series := newStack("late", "early").Order(shape.OrderAppearance).Generate(data)
	for _, s := range series {
		want := 0
		if s.Key == "late" {
			want = 1
		}
		if s.Index != want {
			t.Errorf("%q index = %d, want %d", s.Key, s.Index, want)
		}
	}
}

func TestStackEmpty(t *testing.T) {
	if s := shape.NewStack[row]().Generate(stackData); len(s) != 0 {
		t.Errorf("no keys must give no series, got %d", len(s))
	}
	s := newStack("a").Generate(nil)
	if len(s) != 1 || len(s[0].Points) != 0 {
		t.Errorf("no data must give an empty series, got %+v", s)
	}
}

// TestStackFeedsArea is the intended downstream use: each series becomes a
// filled band.
func TestStackFeedsArea(t *testing.T) {
	series := newStack("a", "b").Generate(stackData)
	area := shape.NewArea[shape.SeriesPoint[row]]().
		X(func(_ shape.SeriesPoint[row], i int, _ []shape.SeriesPoint[row]) float64 {
			return float64(i) * 10
		}).
		Y0(func(d shape.SeriesPoint[row], _ int, _ []shape.SeriesPoint[row]) float64 { return d.Lower }).
		Y1(func(d shape.SeriesPoint[row], _ int, _ []shape.SeriesPoint[row]) float64 { return d.Upper })
	if got := area.Generate(series[1].Points); got != "M0,3L10,9L10,4L0,1Z" {
		t.Errorf("stacked band = %q", got)
	}
}
