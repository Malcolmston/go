package scale

import "math"

// Band is the scale every bar chart is built on. It divides a continuous range
// — the pixel width of the plot — into one band per category, and reports both
// where each band starts ([Band.Scale]) and how wide it is
// ([Band.Bandwidth]).
//
// # The arithmetic
//
// Three knobs shape the layout, and getting them straight is the difference
// between bars that line up with their axis and bars that do not:
//
//   - paddingInner ∈ [0, 1] is the fraction of a STEP left as a gap between
//     adjacent bands. 0 means the bands touch; 1 means they have no width at
//     all (which is what [Point] does deliberately).
//   - paddingOuter ∈ [0, 1] is the gap before the first band and after the
//     last, again measured in steps.
//   - align ∈ [0, 1] distributes any leftover range — which appears only when
//     rounding is on — between the two ends; 0.5, the default, centers it.
//
// From those, with n categories and a range of width W:
//
//	step      = W / max(1, n - paddingInner + 2·paddingOuter)
//	bandwidth = step · (1 - paddingInner)
//	start_i   = start + step·i
//
// The n - paddingInner term (rather than n) is the part people get wrong: the
// last band contributes its width but not its trailing gap, so the divisor
// counts n steps minus the one inner gap that does not exist. A sanity check
// that this package tests: with no padding at all, n·bandwidth == W exactly.
//
// [Band.SetRound] snaps step, start and bandwidth to integers, which keeps bars
// crisp on a pixel grid at the cost of leaving a remainder that align then
// distributes.
//
// A band scale is an [Ordinal] underneath, so a value outside the domain
// returns the unknown sentinel (NaN by default) rather than extending the
// domain — an unexpected category must not silently resize every bar.
type Band[K comparable] struct {
	ord *Ordinal[K, float64]

	r0, r1 float64
	round  bool

	paddingInner float64
	paddingOuter float64
	align        float64

	step      float64
	bandwidth float64
}

// NewBand returns a band scale with an empty domain and the unit range.
func NewBand[K comparable]() *Band[K] {
	b := &Band[K]{
		ord:   NewOrdinal[K, float64]().SetUnknown(math.NaN()),
		r0:    0,
		r1:    1,
		align: 0.5,
	}
	b.rescale()
	return b
}

// rescale recomputes step, bandwidth and the per-band positions. It runs after
// every setter because all of them feed the same formula.
func (b *Band[K]) rescale() *Band[K] {
	n := float64(len(b.ord.domain))
	reverse := b.r1 < b.r0
	start, stop := b.r0, b.r1
	if reverse {
		start, stop = b.r1, b.r0
	}

	b.step = (stop - start) / math.Max(1, n-b.paddingInner+b.paddingOuter*2)
	if b.round {
		b.step = math.Floor(b.step)
	}
	start += (stop - start - b.step*(n-b.paddingInner)) * b.align
	b.bandwidth = b.step * (1 - b.paddingInner)
	if b.round {
		start = math.Round(start)
		b.bandwidth = math.Round(b.bandwidth)
	}

	values := make([]float64, int(n))
	for i := range values {
		values[i] = start + b.step*float64(i)
	}
	if reverse {
		reverseFloats(values)
	}
	b.ord.SetRange(values...)
	return b
}

// Scale returns the start coordinate of the band for d, or NaN when d is not in
// the domain.
func (b *Band[K]) Scale(d K) float64 { return b.ord.Scale(d) }

// Domain returns a copy of the domain.
func (b *Band[K]) Domain() []K { return b.ord.Domain() }

// SetDomain replaces the domain, dropping duplicates.
func (b *Band[K]) SetDomain(d ...K) *Band[K] {
	b.ord.SetDomain(d...)
	return b.rescale()
}

// Range returns the two-element range extent (not the per-band positions; use
// [Band.Scale] for those).
func (b *Band[K]) Range() []float64 { return []float64{b.r0, b.r1} }

// SetRange sets the range extent. A descending range (SetRange(height, 0))
// lays the bands out from the far end, which is how a vertical bar chart puts
// the first category at the bottom.
func (b *Band[K]) SetRange(r0, r1 float64) *Band[K] {
	b.r0, b.r1 = r0, r1
	return b.rescale()
}

// SetRangeRound sets the range extent and turns on rounding.
func (b *Band[K]) SetRangeRound(r0, r1 float64) *Band[K] {
	b.r0, b.r1 = r0, r1
	b.round = true
	return b.rescale()
}

// Round reports whether positions and widths are snapped to integers.
func (b *Band[K]) Round() bool { return b.round }

// SetRound turns integer snapping on or off.
func (b *Band[K]) SetRound(v bool) *Band[K] { b.round = v; return b.rescale() }

// PaddingInner returns the inner padding.
func (b *Band[K]) PaddingInner() float64 { return b.paddingInner }

// SetPaddingInner sets the gap between adjacent bands, as a fraction of a step,
// clamped to [0, 1].
func (b *Band[K]) SetPaddingInner(p float64) *Band[K] {
	b.paddingInner = clamp01(p)
	return b.rescale()
}

// PaddingOuter returns the outer padding.
func (b *Band[K]) PaddingOuter() float64 { return b.paddingOuter }

// SetPaddingOuter sets the gap before the first and after the last band, as a
// fraction of a step, clamped to [0, 1].
func (b *Band[K]) SetPaddingOuter(p float64) *Band[K] {
	b.paddingOuter = clamp01(p)
	return b.rescale()
}

// Padding returns the inner padding, matching d3's `padding()` getter.
func (b *Band[K]) Padding() float64 { return b.paddingInner }

// SetPadding sets the inner AND outer padding to the same value — d3's
// `padding(p)` shorthand, and the setter most charts want.
func (b *Band[K]) SetPadding(p float64) *Band[K] {
	p = clamp01(p)
	b.paddingInner, b.paddingOuter = p, p
	return b.rescale()
}

// Align returns the alignment.
func (b *Band[K]) Align() float64 { return b.align }

// SetAlign sets how leftover range is distributed between the two ends, clamped
// to [0, 1]; 0.5 (the default) centers it.
func (b *Band[K]) SetAlign(a float64) *Band[K] {
	b.align = clamp01(a)
	return b.rescale()
}

// Bandwidth returns the width of each band.
func (b *Band[K]) Bandwidth() float64 { return b.bandwidth }

// Step returns the distance between the starts of adjacent bands, which is the
// bandwidth plus the inner gap.
func (b *Band[K]) Step() float64 { return b.step }

// Unknown returns the value produced for a domain value the scale does not
// know, NaN by default.
func (b *Band[K]) Unknown() float64 { return b.ord.Unknown() }

// SetUnknown sets the value produced for an unknown domain value.
func (b *Band[K]) SetUnknown(v float64) *Band[K] { b.ord.SetUnknown(v); return b }

// Copy returns an independent copy of the scale.
func (b *Band[K]) Copy() *Band[K] {
	c := *b
	c.ord = b.ord.Copy()
	return &c
}

// ---------------------------------------------------------------------------
// point
// ---------------------------------------------------------------------------

// Point is a band scale with zero bandwidth: each category gets a single
// coordinate rather than a slice of the range. It is what a scatter plot, dot
// plot or line chart with categorical x uses, where the mark is centred on a
// position instead of filling an interval.
//
// It is implemented exactly as d3 does it — a [Band] with paddingInner pinned
// to 1, so the entire step is gap and the band collapses to a point. Only the
// outer padding remains adjustable, and it is exposed as [Point.SetPadding],
// which is why a point scale has one padding knob where a band scale has three.
type Point[K comparable] struct {
	band *Band[K]
}

// NewPoint returns a point scale with an empty domain and the unit range.
func NewPoint[K comparable]() *Point[K] {
	p := &Point[K]{band: NewBand[K]()}
	p.band.SetPaddingInner(1)
	return p
}

// Scale returns the coordinate for d, or NaN when d is not in the domain.
func (p *Point[K]) Scale(d K) float64 { return p.band.Scale(d) }

// Domain returns a copy of the domain.
func (p *Point[K]) Domain() []K { return p.band.Domain() }

// SetDomain replaces the domain.
func (p *Point[K]) SetDomain(d ...K) *Point[K] { p.band.SetDomain(d...); return p }

// Range returns the two-element range extent.
func (p *Point[K]) Range() []float64 { return p.band.Range() }

// SetRange sets the range extent.
func (p *Point[K]) SetRange(r0, r1 float64) *Point[K] { p.band.SetRange(r0, r1); return p }

// SetRangeRound sets the range extent and turns on rounding.
func (p *Point[K]) SetRangeRound(r0, r1 float64) *Point[K] {
	p.band.SetRangeRound(r0, r1)
	return p
}

// Round reports whether positions are snapped to integers.
func (p *Point[K]) Round() bool { return p.band.Round() }

// SetRound turns integer snapping on or off.
func (p *Point[K]) SetRound(v bool) *Point[K] { p.band.SetRound(v); return p }

// Padding returns the outer padding — the only padding a point scale has.
func (p *Point[K]) Padding() float64 { return p.band.PaddingOuter() }

// SetPadding sets the outer padding, the space before the first and after the
// last point, as a fraction of a step.
func (p *Point[K]) SetPadding(v float64) *Point[K] { p.band.SetPaddingOuter(v); return p }

// Align returns the alignment.
func (p *Point[K]) Align() float64 { return p.band.Align() }

// SetAlign sets how leftover range is distributed between the two ends.
func (p *Point[K]) SetAlign(a float64) *Point[K] { p.band.SetAlign(a); return p }

// Step returns the distance between adjacent points.
func (p *Point[K]) Step() float64 { return p.band.Step() }

// Bandwidth always returns 0 for a point scale. It exists so that code written
// against a band scale keeps working when handed a point scale.
func (p *Point[K]) Bandwidth() float64 { return 0 }

// Unknown returns the value produced for an unknown domain value.
func (p *Point[K]) Unknown() float64 { return p.band.Unknown() }

// SetUnknown sets the value produced for an unknown domain value.
func (p *Point[K]) SetUnknown(v float64) *Point[K] { p.band.SetUnknown(v); return p }

// Copy returns an independent copy of the scale.
func (p *Point[K]) Copy() *Point[K] { return &Point[K]{band: p.band.Copy()} }
