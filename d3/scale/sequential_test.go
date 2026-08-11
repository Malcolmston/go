package scale

import (
	"math"
	"testing"

	"github.com/malcolmston/d3/color"
	"github.com/malcolmston/d3/interpolate"
)

// identityInterp is the simplest possible interpolator: it hands back t, so a
// sequential scale built on it exposes the normalized position directly and the
// tests can assert on numbers rather than colors.
func identityInterp(t float64) float64 { return t }

func TestSequential(t *testing.T) {
	s := NewSequential(identityInterp).SetDomain(0, 100)
	closeTo(t, s.Scale(0), 0, "sequential low")
	closeTo(t, s.Scale(50), 0.5, "sequential mid")
	closeTo(t, s.Scale(100), 1, "sequential high")

	// Unclamped, it extrapolates — the same rule as a continuous scale.
	closeTo(t, s.Scale(150), 1.5, "sequential extrapolates")
	closeTo(t, s.Scale(-50), -0.5, "sequential extrapolates below")

	s.SetClamp(true)
	closeTo(t, s.Scale(150), 1, "clamped above")
	closeTo(t, s.Scale(-50), 0, "clamped below")

	// A reversed domain reverses the ramp.
	r := NewSequential(identityInterp).SetDomain(100, 0)
	closeTo(t, r.Scale(0), 1, "reversed sequential")
	closeTo(t, r.Scale(100), 0, "reversed sequential")

	// A degenerate domain lands in the middle, as it does for continuous.
	d := NewSequential(identityInterp).SetDomain(5, 5)
	closeTo(t, d.Scale(5), 0.5, "degenerate sequential")

	// NaN yields the unknown.
	u := NewSequential(identityInterp).SetUnknown(-99)
	closeTo(t, u.Scale(math.NaN()), -99, "sequential unknown")
}

func TestSequentialTransforms(t *testing.T) {
	// Log: the geometric mean of the domain sits at the middle of the ramp.
	l := NewSequentialLog(identityInterp).SetDomain(1, 100)
	closeTo(t, l.Scale(1), 0, "sequential log low")
	closeTo(t, l.Scale(10), 0.5, "sequential log mid")
	closeTo(t, l.Scale(100), 1, "sequential log high")

	// Sqrt: the quarter point of the domain sits at the halfway point.
	q := NewSequentialSqrt(identityInterp).SetDomain(0, 100)
	closeTo(t, q.Scale(25), 0.5, "sequential sqrt mid")

	// Pow with exponent 2.
	p := NewSequentialPow(identityInterp).SetDomain(0, 10)
	p.SetExponent(2)
	closeTo(t, p.Scale(5), 0.25, "sequential pow mid")

	// Symlog spans zero.
	y := NewSequentialSymlog(identityInterp).SetDomain(-100, 100)
	closeTo(t, y.Scale(0), 0.5, "sequential symlog centre")
}

func TestSequentialRangeAndColor(t *testing.T) {
	black := color.RGB{R: 0, G: 0, B: 0, Opacity: 1}
	white := color.RGB{R: 255, G: 255, B: 255, Opacity: 1}
	s := NewSequential[color.Color](nil).
		SetRange(interpolate.RGB, black, white).
		SetDomain(0, 10).
		SetClamp(true)

	closeTo(t, s.Scale(5).RGBA().R, 127.5, "sequential color mid")
	closeTo(t, s.Scale(1000).RGBA().R, 255, "clamped sequential color")
}

func TestSequentialTicksAndNice(t *testing.T) {
	s := NewSequential(identityInterp).SetDomain(0.5, 9.5).Nice(10)
	d := s.Domain()
	closeTo(t, d[0], 0, "sequential Nice low")
	closeTo(t, d[1], 10, "sequential Nice high")
	if len(s.Ticks(10)) == 0 {
		t.Error("sequential Ticks returned nothing")
	}
	if f := s.TickFormat(10); f(5) == "" {
		t.Error("sequential TickFormat returned an empty label")
	}
}

// TestSequentialQuantile: the output is uniform over the ramp no matter how
// skewed the input is, which is the whole reason to reach for it.
func TestSequentialQuantile(t *testing.T) {
	s := NewSequentialQuantile(identityInterp).SetDomain(1, 2, 3, 4, 900)

	closeTo(t, s.Scale(1), 0, "first observation is at 0")
	closeTo(t, s.Scale(900), 1, "last observation is at 1")
	closeTo(t, s.Scale(2), 0.25, "second of five")
	closeTo(t, s.Scale(3), 0.5, "third of five")
	closeTo(t, s.Scale(4), 0.75, "fourth of five")

	// The outlier does not compress the rest, which it would in a plain
	// sequential scale.
	plain := NewSequential(identityInterp).SetDomain(1, 900)
	if plain.Scale(4) > 0.01 {
		t.Fatal("test premise wrong: the outlier should dominate a plain scale")
	}

	// Unsorted input is sorted; NaNs are dropped.
	u := NewSequentialQuantile(identityInterp).SetDomain(900, math.NaN(), 1, 3)
	d := u.Domain()
	if len(d) != 3 || d[0] != 1 || d[2] != 900 {
		t.Errorf("SetDomain did not sort/filter: %v", d)
	}

	// Quantiles are the legend breakpoints.
	q := NewSequentialQuantile(identityInterp).SetDomain(3, 6, 7, 8, 8, 10, 13, 15, 16, 20)
	got := q.Quantiles(4)
	want := []float64{3, 7.25, 9, 14.5, 20}
	for i := range want {
		closeTo(t, got[i], want[i], "Quantiles(4)")
	}
}

// TestDiverging pins the defining property: the pivot always lands at the
// middle of the ramp, and each half is normalized independently.
func TestDiverging(t *testing.T) {
	tests := []struct {
		name        string
		x0, x1, x2  float64
		in          float64
		want        float64
		description string
	}{
		{"symmetric low", -1, 0, 1, -1, 0, ""},
		{"symmetric pivot", -1, 0, 1, 0, 0.5, ""},
		{"symmetric high", -1, 0, 1, 1, 1, ""},
		{"symmetric quarter", -1, 0, 1, -0.5, 0.25, ""},
		// The asymmetric case is the point of the scale: zero stays in the
		// middle even though the positive side is a hundred times longer.
		{"asymmetric pivot", -1, 0, 100, 0, 0.5, ""},
		{"asymmetric low", -1, 0, 100, -1, 0, ""},
		{"asymmetric high", -1, 0, 100, 100, 1, ""},
		{"asymmetric halfway up", -1, 0, 100, 50, 0.75, ""},
		{"asymmetric halfway down", -1, 0, 100, -0.5, 0.25, ""},
		// A non-zero pivot.
		{"pivot at 20", 0, 20, 100, 20, 0.5, ""},
		{"pivot at 20, below", 0, 20, 100, 10, 0.25, ""},
		{"pivot at 20, above", 0, 20, 100, 60, 0.75, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDiverging(identityInterp).SetDomain(tt.x0, tt.x1, tt.x2)
			closeTo(t, d.Scale(tt.in), tt.want, "Diverging.Scale")
		})
	}
}

func TestDivergingDescendingAndClamp(t *testing.T) {
	// A descending domain flips the ramp but keeps the pivot centred.
	d := NewDiverging(identityInterp).SetDomain(100, 0, -100)
	closeTo(t, d.Scale(0), 0.5, "descending pivot")
	closeTo(t, d.Scale(100), 0, "descending low end")
	closeTo(t, d.Scale(-100), 1, "descending high end")

	c := NewDiverging(identityInterp).SetDomain(-1, 0, 1)
	closeTo(t, c.Scale(5), 3, "unclamped diverging extrapolates")
	c.SetClamp(true)
	closeTo(t, c.Scale(5), 1, "clamped diverging")
	closeTo(t, c.Scale(-5), 0, "clamped diverging below")
}

func TestDivergingTransformsAndNice(t *testing.T) {
	l := NewDivergingLog(identityInterp).SetDomain(0.01, 1, 100)
	closeTo(t, l.Scale(1), 0.5, "diverging log pivot")
	closeTo(t, l.Scale(0.1), 0.25, "diverging log quarter")
	closeTo(t, l.Scale(10), 0.75, "diverging log three quarters")

	s := NewDivergingSymlog(identityInterp).SetDomain(-100, 0, 100)
	closeTo(t, s.Scale(0), 0.5, "diverging symlog pivot")

	q := NewDivergingSqrt(identityInterp).SetDomain(0, 25, 100)
	closeTo(t, q.Scale(25), 0.5, "diverging sqrt pivot")

	// Nice widens the outer ends and leaves the pivot alone.
	n := NewDiverging(identityInterp).SetDomain(-9.5, 0, 9.5).Nice(10)
	d := n.Domain()
	closeTo(t, d[0], -10, "diverging Nice low")
	closeTo(t, d[1], 0, "diverging Nice leaves the pivot")
	closeTo(t, d[2], 10, "diverging Nice high")
}

func TestDivergingRangeThreeStops(t *testing.T) {
	blue := color.RGB{R: 0, G: 0, B: 255, Opacity: 1}
	grey := color.RGB{R: 128, G: 128, B: 128, Opacity: 1}
	red := color.RGB{R: 255, G: 0, B: 0, Opacity: 1}
	d := NewDiverging[color.Color](nil).
		SetRange(interpolate.RGB, blue, grey, red).
		SetDomain(-1, 0, 1)

	closeTo(t, d.Scale(0).RGBA().R, 128, "pivot colour")
	closeTo(t, d.Scale(-1).RGBA().B, 255, "low colour")
	closeTo(t, d.Scale(1).RGBA().R, 255, "high colour")
}

func TestSequentialAndDivergingCopy(t *testing.T) {
	s := NewSequential(identityInterp).SetDomain(0, 100)
	c := s.Copy().SetDomain(0, 10)
	closeTo(t, s.Scale(50), 0.5, "sequential original unaffected")
	closeTo(t, c.Scale(5), 0.5, "sequential copy independent")

	d := NewDiverging(identityInterp).SetDomain(-1, 0, 1)
	e := d.Copy().SetDomain(-10, 0, 10)
	closeTo(t, d.Scale(0.5), 0.75, "diverging original unaffected")
	closeTo(t, e.Scale(5), 0.75, "diverging copy independent")
}
