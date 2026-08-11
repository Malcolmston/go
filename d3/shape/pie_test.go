package shape_test

import (
	"math"
	"testing"

	"github.com/malcolmston/d3/shape"
)

func value(v float64, _ int, _ []float64) float64 { return v }

func newPie() *shape.Pie[float64] { return shape.NewPie[float64]().Value(value) }

// TestPieEqualValues is the case that can be checked exactly: four equal
// values must divide the circle into quarters, starting at twelve o'clock.
func TestPieEqualValues(t *testing.T) {
	arcs := newPie().Generate([]float64{1, 1, 1, 1})
	if len(arcs) != 4 {
		t.Fatalf("arcs = %d, want 4", len(arcs))
	}
	for i, a := range arcs {
		closeTo(t, "start", a.StartAngle, float64(i)*math.Pi/2, 1e-12)
		closeTo(t, "end", a.EndAngle, float64(i+1)*math.Pi/2, 1e-12)
		if a.Value != 1 {
			t.Errorf("value = %v, want 1", a.Value)
		}
	}
}

func TestPieProportions(t *testing.T) {
	// 1:2:1 over a full turn, sorted descending by default so the 2 leads.
	arcs := newPie().Generate([]float64{1, 2, 1})
	widths := map[float64]float64{}
	total := 0.0
	for _, a := range arcs {
		w := a.EndAngle - a.StartAngle
		widths[a.Value] += w
		total += w
	}
	closeTo(t, "total angle", total, 2*math.Pi, 1e-12)
	closeTo(t, "the 2 gets half the circle", widths[2], math.Pi, 1e-12)
	closeTo(t, "the 1s share the rest", widths[1], math.Pi, 1e-12)
}

func TestPieSorting(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*shape.Pie[float64]) *shape.Pie[float64]
		data      []float64
		// wantOrder[i] is the input index drawn i-th.
		wantOrder []int
	}{
		{"descending by value is the default",
			func(p *shape.Pie[float64]) *shape.Pie[float64] { return p },
			[]float64{1, 3, 2}, []int{1, 2, 0}},
		{"input order when sorting is disabled",
			func(p *shape.Pie[float64]) *shape.Pie[float64] { return p.SortValues(nil) },
			[]float64{1, 3, 2}, []int{0, 1, 2}},
		{"ascending by value",
			func(p *shape.Pie[float64]) *shape.Pie[float64] {
				return p.SortValues(func(a, b float64) int {
					switch {
					case a < b:
						return -1
					case a > b:
						return 1
					}
					return 0
				})
			},
			[]float64{1, 3, 2}, []int{0, 2, 1}},
		{"sorting by data disables sorting by value",
			func(p *shape.Pie[float64]) *shape.Pie[float64] {
				return p.Sort(func(a, b float64) int {
					switch {
					case a < b:
						return -1
					case a > b:
						return 1
					}
					return 0
				})
			},
			[]float64{1, 3, 2}, []int{0, 2, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arcs := tt.configure(newPie()).Generate(tt.data)
			got := make([]int, len(arcs))
			for i, a := range arcs {
				got[a.Index] = i
			}
			for i := range tt.wantOrder {
				if got[i] != tt.wantOrder[i] {
					t.Fatalf("draw order = %v, want %v", got, tt.wantOrder)
				}
			}
			// The returned slice is always in input order.
			for i, a := range arcs {
				if a.Value != tt.data[i] {
					t.Fatalf("arcs must stay in input order; arcs[%d].Value = %v", i, a.Value)
				}
			}
		})
	}
}

func TestPieAngleRange(t *testing.T) {
	// A gauge: half a turn starting at nine o'clock.
	arcs := newPie().SortValues(nil).
		StartAngle(-math.Pi / 2).EndAngle(math.Pi / 2).
		Generate([]float64{1, 1})
	closeTo(t, "first start", arcs[0].StartAngle, -math.Pi/2, 1e-12)
	closeTo(t, "last end", arcs[1].EndAngle, math.Pi/2, 1e-12)
	closeTo(t, "each half", arcs[0].EndAngle-arcs[0].StartAngle, math.Pi/2, 1e-12)
}

// TestPiePadAngle: padding comes out of the available angle, so the pie still
// spans exactly the configured range and each slice loses an equal share.
func TestPiePadAngle(t *testing.T) {
	const pad = 0.1
	arcs := newPie().SortValues(nil).PadAngle(pad).Generate([]float64{1, 1, 1, 1})
	closeTo(t, "first start", arcs[0].StartAngle, 0, 1e-12)
	closeTo(t, "last end", arcs[3].EndAngle, 2*math.Pi, 1e-12)
	for _, a := range arcs {
		closeTo(t, "slice width", a.EndAngle-a.StartAngle, (2*math.Pi-4*pad)/4+pad, 1e-12)
		closeTo(t, "pad angle is reported to Arc", a.PadAngle, pad, 1e-12)
	}
}

// TestPiePadAngleClamped: the padding can never exceed the angle available per
// slice, or the pie would invert.
func TestPiePadAngleClamped(t *testing.T) {
	arcs := newPie().SortValues(nil).PadAngle(10).Generate([]float64{1, 1})
	for _, a := range arcs {
		if a.EndAngle < a.StartAngle {
			t.Fatalf("slice inverted: %v..%v", a.StartAngle, a.EndAngle)
		}
	}
	closeTo(t, "still a full turn", arcs[1].EndAngle, 2*math.Pi, 1e-12)
}

// TestPieNonPositiveValues: negatives and zeros take no angle but keep their
// slot, which is what keeps colors and legends aligned with the data.
func TestPieNonPositiveValues(t *testing.T) {
	arcs := newPie().SortValues(nil).Generate([]float64{1, 0, -5, 1})
	for i, a := range arcs {
		w := a.EndAngle - a.StartAngle
		if i == 1 || i == 2 {
			closeTo(t, "non-positive slice has no width", w, 0, 1e-12)
		} else {
			closeTo(t, "positive slice", w, math.Pi, 1e-12)
		}
	}
	if len(arcs) != 4 {
		t.Errorf("all data must keep a slot, got %d arcs", len(arcs))
	}
}

func TestPieEmpty(t *testing.T) {
	if arcs := newPie().Generate(nil); len(arcs) != 0 {
		t.Errorf("empty data must yield no arcs, got %d", len(arcs))
	}
}

// TestPieFeedsArc is the integration this package exists for.
func TestPieFeedsArc(t *testing.T) {
	arc := shape.NewArc().InnerRadius(0).OuterRadius(100).Digits(3)
	arcs := newPie().SortValues(nil).Generate([]float64{1, 1, 1, 1})
	if got := arc.Generate(arcs[0].Arc()); got != "M0,-100A100,100,0,0,1,100,0L0,0Z" {
		t.Errorf("first quarter = %q", got)
	}
}
