package scale

import (
	"math"
	"testing"
)

// TestRadial checks the property that distinguishes a radial scale from a
// linear one: equal steps in the domain give equal steps in AREA, so the radius
// grows as the square root.
func TestRadial(t *testing.T) {
	r := NewRadial().SetDomain(0, 100).SetRange(0, 10)

	closeTo(t, r.Scale(0), 0, "radial low endpoint")
	closeTo(t, r.Scale(100), 10, "radial high endpoint")
	closeTo(t, r.Scale(50), math.Sqrt(50), "radial midpoint is sqrt, not half")
	closeTo(t, r.Scale(25), 5, "a quarter of the value is half the radius")

	// Areas are proportional to the values.
	area := func(v float64) float64 { x := r.Scale(v); return x * x }
	closeTo(t, area(50)/area(100), 0.5, "half the value is half the area")
	closeTo(t, area(25)/area(100), 0.25, "a quarter of the value is a quarter of the area")

	// Invert round-trips.
	for _, v := range []float64{0, 25, 50, 100, 150} {
		closeTo(t, r.Invert(r.Scale(v)), v, "radial round trip")
	}

	// A linear scale would put 50 at 5; that is the mistake radial exists to
	// prevent.
	if math.Abs(r.Scale(50)-5) < 1 {
		t.Error("radial should not behave linearly")
	}
}

func TestRadialSignedRangeAndOptions(t *testing.T) {
	// A range that crosses zero stays monotone thanks to the signed square.
	s := NewRadial().SetDomain(-1, 1).SetRange(-10, 10)
	closeTo(t, s.Scale(0), 0, "signed radial centre")
	if s.Scale(-1) >= 0 || s.Scale(1) <= 0 {
		t.Error("signed radial should keep the sign of the range")
	}

	// Round snaps the output radius.
	rr := NewRadial().SetDomain(0, 100).SetRangeRound(0, 10)
	if !rr.Round() {
		t.Error("SetRangeRound should turn rounding on")
	}
	v := rr.Scale(50)
	if v != math.Trunc(v) {
		t.Errorf("rounded radial output %v is not an integer", v)
	}

	// Clamp keeps the radius inside the range.
	c := NewRadial().SetDomain(0, 100).SetRange(0, 10).SetClamp(true)
	closeTo(t, c.Scale(1000), 10, "clamped radial")
	closeTo(t, c.Scale(-1000), 0, "clamped radial below")

	// Unknown, ticks and nice come along from the underlying linear scale.
	u := NewRadial().SetUnknown(-1)
	closeTo(t, u.Scale(math.NaN()), -1, "radial unknown")

	n := NewRadial().SetDomain(0.5, 9.5).Nice(10).Domain()
	closeTo(t, n[0], 0, "radial Nice low")
	closeTo(t, n[1], 10, "radial Nice high")

	if len(NewRadial().SetDomain(0, 100).Ticks(5)) == 0 {
		t.Error("radial Ticks returned nothing")
	}

	// Copy is independent.
	a := NewRadial().SetDomain(0, 100).SetRange(0, 10)
	b := a.Copy().SetRange(0, 20)
	closeTo(t, a.Scale(100), 10, "radial original unaffected")
	closeTo(t, b.Scale(100), 20, "radial copy independent")
}
