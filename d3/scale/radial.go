package scale

import "math"

// Radial is a scale whose range is interpolated by *squared* value, so that
// equal steps in the domain produce equal steps in the AREA swept by the
// output radius rather than in the radius itself. It is the scale to reach for
// when the visual variable is a radius on a radial chart or a sunburst: the eye
// reads area, and a plain linear radius overstates large values by the square.
//
// Internally it is a linear scale over the squared range plus an unsquare on
// the way out — the same construction as d3's scaleRadial. The square is signed
// (sign(x)·x²) so that a range that crosses zero still behaves monotonically.
type Radial struct {
	squared *Continuous[float64]
	rng     []float64
	round   bool
	unknown float64
}

// NewRadial returns a radial scale with the unit domain and unit range.
func NewRadial() *Radial {
	r := &Radial{
		squared: NewLinear(),
		rng:     []float64{0, 1},
		unknown: math.NaN(),
	}
	r.squared.SetRange(square(0), square(1))
	return r
}

func square(x float64) float64 {
	if x < 0 {
		return -x * x
	}
	return x * x
}

func unsquare(x float64) float64 {
	if x < 0 {
		return -math.Sqrt(-x)
	}
	return math.Sqrt(x)
}

// Scale maps a domain value to a radius.
func (r *Radial) Scale(x float64) float64 {
	if math.IsNaN(x) {
		return r.unknown
	}
	y := unsquare(r.squared.Scale(x))
	if math.IsNaN(y) {
		return r.unknown
	}
	if r.round {
		return math.Round(y)
	}
	return y
}

// Invert maps a radius back to a domain value.
func (r *Radial) Invert(y float64) float64 { return r.squared.Invert(square(y)) }

// Domain returns a copy of the domain.
func (r *Radial) Domain() []float64 { return r.squared.Domain() }

// SetDomain sets the domain and returns the scale.
func (r *Radial) SetDomain(d ...float64) *Radial { r.squared.SetDomain(d...); return r }

// Range returns a copy of the range, in radius units (not squared).
func (r *Radial) Range() []float64 { return append([]float64(nil), r.rng...) }

// SetRange sets the range in radius units and returns the scale.
func (r *Radial) SetRange(v ...float64) *Radial {
	r.rng = append([]float64(nil), v...)
	sq := make([]float64, len(v))
	for i, x := range v {
		sq[i] = square(x)
	}
	r.squared.SetRange(sq...)
	return r
}

// SetRangeRound sets the range and rounds outputs to whole radii.
func (r *Radial) SetRangeRound(v ...float64) *Radial {
	r.round = true
	return r.SetRange(v...)
}

// Round reports whether outputs are rounded.
func (r *Radial) Round() bool { return r.round }

// SetRound turns output rounding on or off.
func (r *Radial) SetRound(b bool) *Radial { r.round = b; return r }

// Clamp reports whether clamping is on.
func (r *Radial) Clamp() bool { return r.squared.Clamp() }

// SetClamp turns clamping on or off.
func (r *Radial) SetClamp(c bool) *Radial { r.squared.SetClamp(c); return r }

// Unknown returns the value produced for a NaN input.
func (r *Radial) Unknown() float64 { return r.unknown }

// SetUnknown sets the value produced for a NaN input.
func (r *Radial) SetUnknown(v float64) *Radial { r.unknown = v; return r }

// Ticks returns approximately count domain ticks.
func (r *Radial) Ticks(count int) []float64 { return r.squared.Ticks(count) }

// TickFormat returns a formatter for the values from [Radial.Ticks].
func (r *Radial) TickFormat(count int) func(float64) string { return r.squared.TickFormat(count) }

// Nice extends the domain outward to round values.
func (r *Radial) Nice(count int) *Radial { r.squared.Nice(count); return r }

// Copy returns an independent copy of the scale.
func (r *Radial) Copy() *Radial {
	c := *r
	c.squared = r.squared.Copy()
	c.rng = append([]float64(nil), r.rng...)
	return &c
}
