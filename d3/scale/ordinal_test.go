package scale

import (
	"math"
	"testing"
)

// TestOrdinalImplicitDomain pins the behavior that makes ordinal scales
// convenient and occasionally astonishing: an unseen value is APPENDED to the
// domain and given the next range value, and the assignment is then stable.
func TestOrdinalImplicitDomain(t *testing.T) {
	s := NewOrdinal[string, string]("red", "green", "blue")

	if got := s.Scale("apples"); got != "red" {
		t.Errorf("first unseen value = %q, want %q", got, "red")
	}
	if got := s.Scale("oranges"); got != "green" {
		t.Errorf("second unseen value = %q, want %q", got, "green")
	}
	if got := s.Scale("apples"); got != "red" {
		t.Errorf("repeat lookup = %q, want a stable %q", got, "red")
	}

	want := []string{"apples", "oranges"}
	got := s.Domain()
	if len(got) != len(want) {
		t.Fatalf("Domain() = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("Domain()[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// The range cycles when the domain outgrows it: the fourth category is at
	// index 3, and 3 % 3 == 0, so it silently reuses the first color.
	for _, v := range []string{"pears", "plums"} {
		s.Scale(v)
	}
	if got := s.Scale("pears"); got != "blue" {
		t.Errorf("third category = %q, want %q", got, "blue")
	}
	if got := s.Scale("plums"); got != "red" {
		t.Errorf("fourth category = %q, want the range to cycle back to %q", got, "red")
	}
}

// TestOrdinalUnknownSentinel: setting Unknown closes the domain. This is the
// switch from "learn as you go" to "closed set", and the domain must stop
// growing.
func TestOrdinalUnknownSentinel(t *testing.T) {
	s := NewOrdinal[string, string]("red", "green").
		SetDomain("a", "b").
		SetUnknown("#ccc")

	if got := s.Scale("a"); got != "red" {
		t.Errorf("Scale(a) = %q, want red", got)
	}
	if got := s.Scale("zzz"); got != "#ccc" {
		t.Errorf("Scale(unseen) = %q, want the unknown sentinel", got)
	}
	if n := len(s.Domain()); n != 2 {
		t.Errorf("domain grew to %d after an unknown lookup; it must not", n)
	}
	if s.Implicit() {
		t.Error("SetUnknown must turn off implicit domain extension")
	}

	// SetImplicit puts it back.
	s.SetImplicit()
	s.Scale("zzz")
	if n := len(s.Domain()); n != 3 {
		t.Errorf("after SetImplicit the domain should grow, got %d", n)
	}
}

func TestOrdinalSetDomainDeduplicates(t *testing.T) {
	s := NewOrdinal[string, int](1, 2, 3).SetDomain("a", "b", "a", "c")
	got := s.Domain()
	if len(got) != 3 {
		t.Fatalf("Domain() = %v, want 3 unique values", got)
	}
	if s.Scale("a") != 1 || s.Scale("b") != 2 || s.Scale("c") != 3 {
		t.Errorf("dedup changed the positional mapping: %d %d %d", s.Scale("a"), s.Scale("b"), s.Scale("c"))
	}
}

func TestOrdinalCopy(t *testing.T) {
	a := NewOrdinal[string, string]("x", "y")
	a.Scale("one")
	b := a.Copy()
	b.Scale("two")
	if len(a.Domain()) != 1 {
		t.Errorf("the original learned from the copy: %v", a.Domain())
	}
	if len(b.Domain()) != 2 {
		t.Errorf("the copy did not keep the learned domain: %v", b.Domain())
	}
}

// TestBandArithmetic is the precise test the whole package earns its keep on:
// every bar chart depends on step and bandwidth being exactly right.
func TestBandArithmetic(t *testing.T) {
	tests := []struct {
		name          string
		domain        []string
		r0, r1        float64
		padInner      float64
		padOuter      float64
		align         float64
		round         bool
		wantStep      float64
		wantBandwidth float64
		wantPos       []float64
	}{
		{
			name: "no padding fills the range exactly",
			// step = 120/3 = 40, bandwidth = step, so 3 bands tile [0,120].
			domain: []string{"a", "b", "c"}, r0: 0, r1: 120,
			align:    0.5,
			wantStep: 40, wantBandwidth: 40,
			wantPos: []float64{0, 40, 80},
		},
		{
			name: "outer padding only",
			// step = 120/(3 - 0 + 2*0.5) = 30; leading gap = 0.5 step = 15.
			domain: []string{"a", "b", "c"}, r0: 0, r1: 120,
			padOuter: 0.5, align: 0.5,
			wantStep: 30, wantBandwidth: 30,
			wantPos: []float64{15, 45, 75},
		},
		{
			name: "inner padding leaves gaps but no outer margin",
			// step = 120/(3 - 0.5) = 48; bandwidth = 48*0.5 = 24.
			domain: []string{"a", "b", "c"}, r0: 0, r1: 120,
			padInner: 0.5, align: 0.5,
			wantStep: 48, wantBandwidth: 24,
			wantPos: []float64{0, 48, 96},
		},
		{
			name: "padding sets both",
			// step = 120/(3 - 0.5 + 1) = 34.2857…; bandwidth = 17.1428…
			domain: []string{"a", "b", "c"}, r0: 0, r1: 120,
			padInner: 0.5, padOuter: 0.5, align: 0.5,
			wantStep: 120.0 / 3.5, wantBandwidth: 120.0 / 3.5 * 0.5,
			wantPos: []float64{120.0 / 3.5 * 0.5, 120.0/3.5*0.5 + 120.0/3.5, 120.0/3.5*0.5 + 2*120.0/3.5},
		},
		{
			name: "single band",
			// max(1, 1-0) = 1, so the band is the whole range.
			domain: []string{"only"}, r0: 0, r1: 100,
			align:    0.5,
			wantStep: 100, wantBandwidth: 100,
			wantPos: []float64{0},
		},
		{
			name:   "reversed range lays out from the far end",
			domain: []string{"a", "b", "c"}, r0: 120, r1: 0,
			align:    0.5,
			wantStep: 40, wantBandwidth: 40,
			wantPos: []float64{80, 40, 0},
		},
		{
			name: "round floors the step and centres the remainder",
			// 100/3 = 33.33 → step 33; leftover 1 split by align 0.5 → 0.5,
			// then start is rounded to 1 (math.Round of 0.5).
			domain: []string{"a", "b", "c"}, r0: 0, r1: 100,
			align: 0.5, round: true,
			wantStep: 33, wantBandwidth: 33,
			wantPos: []float64{1, 34, 67},
		},
		{
			name:   "round with align 0 puts the remainder at the end",
			domain: []string{"a", "b", "c"}, r0: 0, r1: 100,
			align: 0, round: true,
			wantStep: 33, wantBandwidth: 33,
			wantPos: []float64{0, 33, 66},
		},
		{
			name:   "round with align 1 puts the remainder at the start",
			domain: []string{"a", "b", "c"}, r0: 0, r1: 100,
			align: 1, round: true,
			wantStep: 33, wantBandwidth: 33,
			wantPos: []float64{1, 34, 67},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBand[string]().
				SetDomain(tt.domain...).
				SetPaddingInner(tt.padInner).
				SetPaddingOuter(tt.padOuter).
				SetAlign(tt.align).
				SetRound(tt.round).
				SetRange(tt.r0, tt.r1)

			closeTo(t, b.Step(), tt.wantStep, "Step()")
			closeTo(t, b.Bandwidth(), tt.wantBandwidth, "Bandwidth()")
			for i, d := range tt.domain {
				closeTo(t, b.Scale(d), tt.wantPos[i], "Scale("+d+")")
			}
		})
	}
}

// TestBandTilesTheRange is the arithmetic sanity check: with no padding the
// bands tile the range exactly, and with padding the widths and gaps still sum
// to the range width.
func TestBandTilesTheRange(t *testing.T) {
	domain := []string{"a", "b", "c", "d", "e", "f", "g"}
	width := 640.0

	t.Run("no padding", func(t *testing.T) {
		b := NewBand[string]().SetDomain(domain...).SetRange(0, width)
		closeTo(t, float64(len(domain))*b.Bandwidth(), width, "n·bandwidth")
		last := b.Scale(domain[len(domain)-1])
		closeTo(t, last+b.Bandwidth(), width, "last band ends at the range end")
	})

	for _, p := range []float64{0.1, 0.25, 0.5, 0.9} {
		b := NewBand[string]().SetDomain(domain...).SetPadding(p).SetRange(0, width)
		n := float64(len(domain))
		// n bands + (n-1) inner gaps + 2 outer gaps == width.
		inner := b.Step() - b.Bandwidth()
		outer := b.Scale(domain[0])
		total := n*b.Bandwidth() + (n-1)*inner + 2*outer
		closeTo(t, total, width, "bands + gaps sum to the range width")
		// And the first band starts one outer gap in, symmetrically.
		last := b.Scale(domain[len(domain)-1]) + b.Bandwidth()
		closeTo(t, width-last, outer, "outer gaps are symmetric with align 0.5")
	}
}

func TestBandPaddingAccessors(t *testing.T) {
	b := NewBand[string]().SetDomain("a", "b").SetPadding(0.3)
	closeTo(t, b.PaddingInner(), 0.3, "SetPadding sets inner")
	closeTo(t, b.PaddingOuter(), 0.3, "SetPadding sets outer")
	closeTo(t, b.Padding(), 0.3, "Padding() reads inner")

	b.SetPaddingInner(0.1)
	closeTo(t, b.PaddingInner(), 0.1, "SetPaddingInner")
	closeTo(t, b.PaddingOuter(), 0.3, "SetPaddingInner leaves outer alone")

	// Padding is clamped to [0, 1].
	b.SetPaddingInner(5).SetPaddingOuter(-3)
	closeTo(t, b.PaddingInner(), 1, "padding clamped high")
	closeTo(t, b.PaddingOuter(), 0, "padding clamped low")
}

func TestBandUnknownValue(t *testing.T) {
	b := NewBand[string]().SetDomain("a", "b").SetRange(0, 100)
	if !math.IsNaN(b.Scale("nope")) {
		t.Errorf("Scale of an unknown category = %v, want NaN", b.Scale("nope"))
	}
	if n := len(b.Domain()); n != 2 {
		t.Errorf("an unknown lookup extended the band domain to %d", n)
	}
	b.SetUnknown(-1)
	closeTo(t, b.Scale("nope"), -1, "Scale with a custom unknown")
}

func TestBandRangeRound(t *testing.T) {
	b := NewBand[string]().SetDomain("a", "b", "c").SetRangeRound(0, 100)
	if !b.Round() {
		t.Error("SetRangeRound should turn rounding on")
	}
	for _, d := range b.Domain() {
		v := b.Scale(d)
		if v != math.Trunc(v) {
			t.Errorf("rounded band position %v is not an integer", v)
		}
	}
	if bw := b.Bandwidth(); bw != math.Trunc(bw) {
		t.Errorf("rounded bandwidth %v is not an integer", bw)
	}
}

func TestBandCopy(t *testing.T) {
	a := NewBand[string]().SetDomain("a", "b").SetRange(0, 100)
	c := a.Copy().SetRange(0, 200)
	closeTo(t, a.Bandwidth(), 50, "original bandwidth")
	closeTo(t, c.Bandwidth(), 100, "copy bandwidth")
}

// TestPoint checks the collapse to zero bandwidth and the classic even spacing.
func TestPoint(t *testing.T) {
	p := NewPoint[string]().SetDomain("a", "b", "c").SetRange(0, 120)
	closeTo(t, p.Bandwidth(), 0, "point bandwidth is always 0")
	closeTo(t, p.Step(), 60, "point step")
	closeTo(t, p.Scale("a"), 0, "first point")
	closeTo(t, p.Scale("b"), 60, "middle point")
	closeTo(t, p.Scale("c"), 120, "last point")

	// Outer padding pulls the ends in.
	q := NewPoint[string]().SetDomain("a", "b", "c").SetPadding(0.5).SetRange(0, 120)
	closeTo(t, q.Step(), 40, "padded point step")
	closeTo(t, q.Scale("a"), 20, "padded first point")
	closeTo(t, q.Scale("c"), 100, "padded last point")

	if !math.IsNaN(p.Scale("zzz")) {
		t.Error("unknown category should be NaN")
	}

	// A single point sits in the middle of the range.
	one := NewPoint[string]().SetDomain("only").SetRange(0, 100)
	closeTo(t, one.Scale("only"), 50, "single point is centred")
}
