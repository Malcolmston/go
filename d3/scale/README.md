# scale — Go port of d3-scale — the mappings that turn abstract data into visual variables

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/scale.svg)](https://pkg.go.dev/github.com/malcolmston/d3/scale)

Package scale is a Go port of d3-scale — the mappings that turn abstract
data into visual variables. A scale is a function from a *domain* (the units
your data is in: dollars, seconds, categories) to a *range* (the units your
display is in: pixels, colors, radii). Almost every chart is, at bottom, two
or three scales plus some drawing:

```go
x := scale.NewTime().SetDomain(t0, t1).SetRange(0, 640).Nice(10)
y := scale.NewLinear().SetDomain(0, maxValue).SetRange(480, 0).Nice(10)
for _, d := range data {
	plot(x.Scale(d.When), y.Scale(d.Value))
}
```

This package is the computational half of d3: it has no DOM, emits no SVG and
imports nothing outside the standard library except its sibling packages
`github.com/malcolmston/d3/array` (tick generation),
`github.com/malcolmston/d3/color` and `github.com/malcolmston/d3/interpolate`.
Pair it with a renderer such as github.com/malcolmston/react to actually draw
something.

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/scale"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Types

| Type | What it is |
| --- | --- |
| `Band` | Band is the scale every bar chart is built on. |
| `Continuous` | Continuous is a continuous, quantitative scale: it maps a continuous domain onto a continuous range by normalizing the input to a position in [0, 1]… |
| `Diverging` | Diverging is a sequential-style scale with three domain stops instead of two: a low end, a *pivot*, and a high end. |
| `Interpolator` | Interpolator builds the function that walks from a to b as t goes 0→1. |
| `Ordinal` | Ordinal maps a discrete domain to a discrete range — categories to colors, species to symbols, teams to stroke patterns. |
| `Point` | Point is a band scale with zero bandwidth: each category gets a single coordinate rather than a slice of the range. |
| `Quantile` | Quantile maps a sample of observations onto a discrete range so that each range value receives an equal *number* of observations. |
| `Quantize` | Quantize maps a continuous domain onto a discrete range by cutting the domain into len(range) equal-width slices. |
| `Radial` | Radial is a scale whose range is interpolated by *squared* value, so that equal steps in the domain produce equal steps in the AREA swept by the… |
| `Sequential` | Sequential is a continuous scale whose output side is a single interpolator rather than a pair of endpoints. |
| `SequentialQuantile` | SequentialQuantile is a sequential scale whose domain is the *data itself* rather than an extent. |
| `Threshold` | Threshold maps a continuous domain onto a discrete range using cut points you supply. |
| `Time` | Time is a continuous scale whose domain is calendar time. |
| `Uninterpolator` | Uninterpolator is the inverse of an `Interpolator`: given the two endpoints it returns the function that recovers t from a value. |

<details>
<summary><code>Band</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewBand[K comparable]() *Band[K]` | NewBand returns a band scale with an empty domain and the unit range. |
| `func (b *Band[K]) Align() float64` | Align returns the alignment. |
| `func (b *Band[K]) Bandwidth() float64` | Bandwidth returns the width of each band. |
| `func (b *Band[K]) Copy() *Band[K]` | Copy returns an independent copy of the scale. |
| `func (b *Band[K]) Domain() []K` | Domain returns a copy of the domain. |
| `func (b *Band[K]) Padding() float64` | Padding returns the inner padding, matching d3's `padding()` getter. |
| `func (b *Band[K]) PaddingInner() float64` | PaddingInner returns the inner padding. |
| `func (b *Band[K]) PaddingOuter() float64` | PaddingOuter returns the outer padding. |
| `func (b *Band[K]) Range() []float64` | Range returns the two-element range extent (not the per-band positions; use `Band.Scale` for those). |
| `func (b *Band[K]) Round() bool` | Round reports whether positions and widths are snapped to integers. |
| `func (b *Band[K]) Scale(d K) float64` | Scale returns the start coordinate of the band for d, or NaN when d is not in the domain. |
| `func (b *Band[K]) SetAlign(a float64) *Band[K]` | SetAlign sets how leftover range is distributed between the two ends, clamped to [0, 1]; 0.5 (the default) centers it. |
| `func (b *Band[K]) SetDomain(d ...K) *Band[K]` | SetDomain replaces the domain, dropping duplicates. |
| `func (b *Band[K]) SetPadding(p float64) *Band[K]` | SetPadding sets the inner AND outer padding to the same value — d3's `padding(p)` shorthand, and the setter most charts want. |
| `func (b *Band[K]) SetPaddingInner(p float64) *Band[K]` | SetPaddingInner sets the gap between adjacent bands, as a fraction of a step, clamped to [0, 1]. |
| `func (b *Band[K]) SetPaddingOuter(p float64) *Band[K]` | SetPaddingOuter sets the gap before the first and after the last band, as a fraction of a step, clamped to [0, 1]. |
| `func (b *Band[K]) SetRange(r0, r1 float64) *Band[K]` | SetRange sets the range extent. |
| `func (b *Band[K]) SetRangeRound(r0, r1 float64) *Band[K]` | SetRangeRound sets the range extent and turns on rounding. |
| `func (b *Band[K]) SetRound(v bool) *Band[K]` | SetRound turns integer snapping on or off. |
| `func (b *Band[K]) SetUnknown(v float64) *Band[K]` | SetUnknown sets the value produced for an unknown domain value. |
| `func (b *Band[K]) Step() float64` | Step returns the distance between the starts of adjacent bands, which is the bandwidth plus the inner gap. |
| `func (b *Band[K]) Unknown() float64` | Unknown returns the value produced for a domain value the scale does not know, NaN by default. |

</details>

<details>
<summary><code>Continuous</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewIdentity() *Continuous[float64]` | NewIdentity returns an identity scale: the domain and range are the same values and Scale is the identity function. |
| `func NewLinear() *Continuous[float64]` | NewLinear returns a linear scale with the unit domain and unit range, the workhorse of the package: position, size, opacity and anything else where… |
| `func NewLinearOf[R any](interp Interpolator[R]) *Continuous[R]` | NewLinearOf returns a linear scale over an arbitrary range type, given the interpolator that walks between two range values. |
| `func NewLog() *Continuous[float64]` | NewLog returns a log scale with base 10 and the domain [1, 10]. |
| `func NewPow() *Continuous[float64]` | NewPow returns a power scale, which applies x ↦ sign(x)·\|x\|^k before normalizing. |
| `func NewSqrt() *Continuous[float64]` | NewSqrt returns a power scale with exponent 0.5. |
| `func NewSymlog() *Continuous[float64]` | NewSymlog returns a symmetric-log scale: sign(x)·log1p(\|x/C\|). |
| `func (s *Continuous[R]) Base() float64` | Base returns the log scale's base (10 by default). |
| `func (s *Continuous[R]) Clamp() bool` | Clamp reports whether clamping is on. |
| `func (s *Continuous[R]) Constant() float64` | Constant returns the symlog scale's constant (1 by default). |
| `func (s *Continuous[R]) Copy() *Continuous[R]` | Copy returns an independent copy of the scale; changes to one do not affect the other. |
| `func (s *Continuous[R]) Domain() []float64` | Domain returns a copy of the domain. |
| `func (s *Continuous[R]) Exponent() float64` | Exponent returns the power scale's exponent (1 for other kinds). |
| `func (s *Continuous[R]) Interpolate() Interpolator[R]` | Interpolate returns the range interpolator in force. |
| `func (s *Continuous[R]) Invert(y R) float64` | Invert maps a range value back to a domain value. |
| `func (s *Continuous[R]) Nice(count int) *Continuous[R]` | Nice extends the domain outward to round values, so an axis ends on a label rather than mid-air. |
| `func (s *Continuous[R]) Range() []R` | Range returns a copy of the range. |
| `func (s *Continuous[R]) Scale(x float64) R` | Scale maps a domain value to a range value — d3's `scale(x)`. |
| `func (s *Continuous[R]) SetBase(b float64) *Continuous[R]` | SetBase sets the base of a log scale. |
| `func (s *Continuous[R]) SetClamp(c bool) *Continuous[R]` | SetClamp turns clamping on or off and returns the scale. |
| `func (s *Continuous[R]) SetConstant(c float64) *Continuous[R]` | SetConstant sets the symlog constant, which controls the width of the near-linear region around zero. |
| `func (s *Continuous[R]) SetDomain(d ...float64) *Continuous[R]` | SetDomain sets the domain and returns the scale. |
| `func (s *Continuous[R]) SetExponent(e float64) *Continuous[R]` | SetExponent sets the exponent of a power scale. |
| `func (s *Continuous[R]) SetInterpolate(i Interpolator[R]) *Continuous[R]` | SetInterpolate replaces the range interpolator and returns the scale. |
| `func (s *Continuous[R]) SetRange(r ...R) *Continuous[R]` | SetRange sets the range and returns the scale. |
| `func (s *Continuous[R]) SetRangeRound(r ...R) *Continuous[R]` | SetRangeRound sets the range and switches to a rounding interpolator, so outputs land on whole numbers. |
| `func (s *Continuous[R]) SetUninterpolate(u Uninterpolator[R]) *Continuous[R]` | SetUninterpolate supplies the inverse of the range interpolator, which is what makes `Continuous.Invert` work for a non-numeric range. |
| `func (s *Continuous[R]) SetUnknown(v R) *Continuous[R]` | SetUnknown sets the value produced for a NaN input and returns the scale. |
| `func (s *Continuous[R]) TickFormat(count int) func(float64) string` | TickFormat returns a formatter for the values from `Continuous.Ticks`, using the default specifier for this kind of scale. |
| `func (s *Continuous[R]) TickFormatSpecifier(count int, specifier string) (func(float64) string, error)` | TickFormatSpecifier returns a tick formatter built from a d3-format specifier, such as "$,.2f" or ".1%". |
| `func (s *Continuous[R]) Ticks(count int) []float64` | Ticks returns approximately count representative values spanning the domain, suitable for axis labels. |
| `func (s *Continuous[R]) Unknown() R` | Unknown returns the value produced for a NaN input. |

</details>

<details>
<summary><code>Diverging</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewDiverging[R any](interp func(float64) R) *Diverging[R]` | NewDiverging returns a diverging scale with the domain [0, 0.5, 1]. |
| `func NewDivergingLog[R any](interp func(float64) R) *Diverging[R]` | NewDivergingLog returns a diverging scale with a log domain transform. |
| `func NewDivergingPow[R any](interp func(float64) R) *Diverging[R]` | NewDivergingPow returns a diverging scale with a power domain transform. |
| `func NewDivergingSqrt[R any](interp func(float64) R) *Diverging[R]` | NewDivergingSqrt returns a diverging scale with a square-root domain transform. |
| `func NewDivergingSymlog[R any](interp func(float64) R) *Diverging[R]` | NewDivergingSymlog returns a diverging scale with a symmetric-log domain transform, the usual choice for signed data that spans orders of magnitude… |
| `func (d *Diverging[R]) Base() float64` | Base returns the log transform's base. |
| `func (d *Diverging[R]) Clamp() bool` | Clamp reports whether clamping is on. |
| `func (d *Diverging[R]) Constant() float64` | Constant returns the symlog transform's constant. |
| `func (d *Diverging[R]) Copy() *Diverging[R]` | Copy returns an independent copy of the scale. |
| `func (d *Diverging[R]) Domain() []float64` | Domain returns a copy of the three-element domain. |
| `func (d *Diverging[R]) Exponent() float64` | Exponent returns the power transform's exponent. |
| `func (d *Diverging[R]) Interpolator() func(float64) R` | Interpolator returns the interpolator in force. |
| `func (d *Diverging[R]) Nice(count int) *Diverging[R]` | Nice extends the outer ends of the domain to round values, leaving the pivot where it is — moving the pivot would change the meaning of the scale. |
| `func (d *Diverging[R]) Scale(x float64) R` | Scale maps a domain value through the interpolator, with the pivot at 0.5. |
| `func (d *Diverging[R]) SetBase(b float64) *Diverging[R]` | SetBase sets the log transform's base (meaningful for `NewDivergingLog`). |
| `func (d *Diverging[R]) SetClamp(c bool) *Diverging[R]` | SetClamp turns clamping on or off. |
| `func (d *Diverging[R]) SetConstant(c float64) *Diverging[R]` | SetConstant sets the symlog transform's constant (meaningful for `NewDivergingSymlog`). |
| `func (d *Diverging[R]) SetDomain(x0, x1, x2 float64) *Diverging[R]` | SetDomain sets the low end, the pivot and the high end. |
| `func (d *Diverging[R]) SetExponent(e float64) *Diverging[R]` | SetExponent sets the power transform's exponent (meaningful for `NewDivergingPow` and `NewDivergingSqrt`). |
| `func (d *Diverging[R]) SetInterpolator(f func(float64) R) *Diverging[R]` | SetInterpolator replaces the interpolator. |
| `func (d *Diverging[R]) SetRange(interp Interpolator[R], r0, r1, r2 R) *Diverging[R]` | SetRange builds the interpolator from three endpoints — low, middle, high — using interpolate.Piecewise semantics. |
| `func (d *Diverging[R]) SetUnknown(v R) *Diverging[R]` | SetUnknown sets the value produced for a NaN input. |
| `func (d *Diverging[R]) TickFormat(count int) func(float64) string` | TickFormat returns a formatter for the values from `Diverging.Ticks`. |
| `func (d *Diverging[R]) Ticks(count int) []float64` | Ticks returns approximately count ticks across the full domain (ignoring the pivot, which is not necessarily a round number). |
| `func (d *Diverging[R]) Unknown() R` | Unknown returns the value produced for a NaN input. |

</details>

<details>
<summary><code>Ordinal</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewOrdinal[K comparable, V any](rng ...V) *Ordinal[K, V]` | NewOrdinal returns an ordinal scale with an empty, implicit domain and the given range. |
| `func (o *Ordinal[K, V]) Copy() *Ordinal[K, V]` | Copy returns an independent copy of the scale, including the domain learned so far. |
| `func (o *Ordinal[K, V]) Domain() []K` | Domain returns a copy of the domain, in first-seen order. |
| `func (o *Ordinal[K, V]) Implicit() bool` | Implicit reports whether unseen values extend the domain. |
| `func (o *Ordinal[K, V]) Range() []V` | Range returns a copy of the range. |
| `func (o *Ordinal[K, V]) Scale(d K) V` | Scale maps a domain value to a range value, extending the domain if the value is unseen and the scale is implicit. |
| `func (o *Ordinal[K, V]) SetDomain(d ...K) *Ordinal[K, V]` | SetDomain replaces the domain, dropping duplicates and keeping first occurrence order. |
| `func (o *Ordinal[K, V]) SetImplicit() *Ordinal[K, V]` | SetImplicit re-enables implicit domain extension, undoing `Ordinal.SetUnknown`. |
| `func (o *Ordinal[K, V]) SetRange(r ...V) *Ordinal[K, V]` | SetRange replaces the range. |
| `func (o *Ordinal[K, V]) SetUnknown(v V) *Ordinal[K, V]` | SetUnknown sets the sentinel for unseen values AND turns off implicit domain extension. |
| `func (o *Ordinal[K, V]) Unknown() V` | Unknown returns the sentinel returned for values outside the domain, which is meaningful only when the scale is not implicit. |

</details>

<details>
<summary><code>Point</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewPoint[K comparable]() *Point[K]` | NewPoint returns a point scale with an empty domain and the unit range. |
| `func (p *Point[K]) Align() float64` | Align returns the alignment. |
| `func (p *Point[K]) Bandwidth() float64` | Bandwidth always returns 0 for a point scale. |
| `func (p *Point[K]) Copy() *Point[K]` | Copy returns an independent copy of the scale. |
| `func (p *Point[K]) Domain() []K` | Domain returns a copy of the domain. |
| `func (p *Point[K]) Padding() float64` | Padding returns the outer padding — the only padding a point scale has. |
| `func (p *Point[K]) Range() []float64` | Range returns the two-element range extent. |
| `func (p *Point[K]) Round() bool` | Round reports whether positions are snapped to integers. |
| `func (p *Point[K]) Scale(d K) float64` | Scale returns the coordinate for d, or NaN when d is not in the domain. |
| `func (p *Point[K]) SetAlign(a float64) *Point[K]` | SetAlign sets how leftover range is distributed between the two ends. |
| `func (p *Point[K]) SetDomain(d ...K) *Point[K]` | SetDomain replaces the domain. |
| `func (p *Point[K]) SetPadding(v float64) *Point[K]` | SetPadding sets the outer padding, the space before the first and after the last point, as a fraction of a step. |
| `func (p *Point[K]) SetRange(r0, r1 float64) *Point[K]` | SetRange sets the range extent. |
| `func (p *Point[K]) SetRangeRound(r0, r1 float64) *Point[K]` | SetRangeRound sets the range extent and turns on rounding. |
| `func (p *Point[K]) SetRound(v bool) *Point[K]` | SetRound turns integer snapping on or off. |
| `func (p *Point[K]) SetUnknown(v float64) *Point[K]` | SetUnknown sets the value produced for an unknown domain value. |
| `func (p *Point[K]) Step() float64` | Step returns the distance between adjacent points. |
| `func (p *Point[K]) Unknown() float64` | Unknown returns the value produced for an unknown domain value. |

</details>

<details>
<summary><code>Quantile</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewQuantile[R comparable](rng ...R) *Quantile[R]` | NewQuantile returns an empty quantile scale with the given range. |
| `func (q *Quantile[R]) Copy() *Quantile[R]` | Copy returns an independent copy of the scale. |
| `func (q *Quantile[R]) Domain() []float64` | Domain returns a copy of the sorted observations. |
| `func (q *Quantile[R]) InvertExtent(y R) (float64, float64)` | InvertExtent returns the domain interval that maps to y, or NaN,NaN when y is not in the range. |
| `func (q *Quantile[R]) Quantiles() []float64` | Quantiles returns a copy of the computed cut points. |
| `func (q *Quantile[R]) Range() []R` | Range returns a copy of the range. |
| `func (q *Quantile[R]) Scale(x float64) R` | Scale maps a value to its bucket. |
| `func (q *Quantile[R]) SetDomain(values ...float64) *Quantile[R]` | SetDomain sets the observations, dropping NaNs and sorting ascending. |
| `func (q *Quantile[R]) SetRange(rng ...R) *Quantile[R]` | SetRange sets the range; the number of quantiles follows from its length. |
| `func (q *Quantile[R]) SetUnknown(v R) *Quantile[R]` | SetUnknown sets the value produced for a NaN input. |
| `func (q *Quantile[R]) Unknown() R` | Unknown returns the value produced for a NaN input. |

</details>

<details>
<summary><code>Quantize</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewQuantize[R comparable](rng ...R) *Quantize[R]` | NewQuantize returns a quantize scale with the unit domain and the given range. |
| `func (q *Quantize[R]) Copy() *Quantize[R]` | Copy returns an independent copy of the scale. |
| `func (q *Quantize[R]) Domain() []float64` | Domain returns the two-element domain extent. |
| `func (q *Quantize[R]) InvertExtent(y R) (float64, float64)` | InvertExtent returns the domain interval [lo, hi) that maps to y, or NaN,NaN when y is not in the range. |
| `func (q *Quantize[R]) Nice(count int) *Quantize[R]` | Nice extends the domain outward to round values. |
| `func (q *Quantize[R]) Range() []R` | Range returns a copy of the range. |
| `func (q *Quantize[R]) Scale(x float64) R` | Scale maps a value to its bucket. |
| `func (q *Quantize[R]) SetDomain(x0, x1 float64) *Quantize[R]` | SetDomain sets the domain extent. |
| `func (q *Quantize[R]) SetRange(rng ...R) *Quantize[R]` | SetRange sets the range; the number of buckets follows from its length. |
| `func (q *Quantize[R]) SetUnknown(v R) *Quantize[R]` | SetUnknown sets the value produced for a NaN input. |
| `func (q *Quantize[R]) Thresholds() []float64` | Thresholds returns a copy of the computed cut points. |
| `func (q *Quantize[R]) TickFormat(count int) func(float64) string` | TickFormat returns a formatter for the values from `Quantize.Ticks`. |
| `func (q *Quantize[R]) Ticks(count int) []float64` | Ticks returns approximately count ticks over the domain. |
| `func (q *Quantize[R]) Unknown() R` | Unknown returns the value produced for a NaN input. |

</details>

<details>
<summary><code>Radial</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewRadial() *Radial` | NewRadial returns a radial scale with the unit domain and unit range. |
| `func (r *Radial) Clamp() bool` | Clamp reports whether clamping is on. |
| `func (r *Radial) Copy() *Radial` | Copy returns an independent copy of the scale. |
| `func (r *Radial) Domain() []float64` | Domain returns a copy of the domain. |
| `func (r *Radial) Invert(y float64) float64` | Invert maps a radius back to a domain value. |
| `func (r *Radial) Nice(count int) *Radial` | Nice extends the domain outward to round values. |
| `func (r *Radial) Range() []float64` | Range returns a copy of the range, in radius units (not squared). |
| `func (r *Radial) Round() bool` | Round reports whether outputs are rounded. |
| `func (r *Radial) Scale(x float64) float64` | Scale maps a domain value to a radius. |
| `func (r *Radial) SetClamp(c bool) *Radial` | SetClamp turns clamping on or off. |
| `func (r *Radial) SetDomain(d ...float64) *Radial` | SetDomain sets the domain and returns the scale. |
| `func (r *Radial) SetRange(v ...float64) *Radial` | SetRange sets the range in radius units and returns the scale. |
| `func (r *Radial) SetRangeRound(v ...float64) *Radial` | SetRangeRound sets the range and rounds outputs to whole radii. |
| `func (r *Radial) SetRound(b bool) *Radial` | SetRound turns output rounding on or off. |
| `func (r *Radial) SetUnknown(v float64) *Radial` | SetUnknown sets the value produced for a NaN input. |
| `func (r *Radial) TickFormat(count int) func(float64) string` | TickFormat returns a formatter for the values from `Radial.Ticks`. |
| `func (r *Radial) Ticks(count int) []float64` | Ticks returns approximately count domain ticks. |
| `func (r *Radial) Unknown() float64` | Unknown returns the value produced for a NaN input. |

</details>

<details>
<summary><code>Sequential</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewSequential[R any](interp func(float64) R) *Sequential[R]` | NewSequential returns a sequential scale with the unit domain and the given interpolator. |
| `func NewSequentialLog[R any](interp func(float64) R) *Sequential[R]` | NewSequentialLog returns a sequential scale with a log domain transform and the default domain [1, 10]. |
| `func NewSequentialPow[R any](interp func(float64) R) *Sequential[R]` | NewSequentialPow returns a sequential scale with a power domain transform. |
| `func NewSequentialSqrt[R any](interp func(float64) R) *Sequential[R]` | NewSequentialSqrt returns a sequential scale with a square-root domain transform. |
| `func NewSequentialSymlog[R any](interp func(float64) R) *Sequential[R]` | NewSequentialSymlog returns a sequential scale with a symmetric-log domain transform, which unlike the log variant tolerates zero and negative values. |
| `func (s *Sequential[R]) Base() float64` | Base returns the log transform's base. |
| `func (s *Sequential[R]) Clamp() bool` | Clamp reports whether clamping is on. |
| `func (s *Sequential[R]) Constant() float64` | Constant returns the symlog transform's constant. |
| `func (s *Sequential[R]) Copy() *Sequential[R]` | Copy returns an independent copy of the scale. |
| `func (s *Sequential[R]) Domain() []float64` | Domain returns a copy of the two-element domain. |
| `func (s *Sequential[R]) Exponent() float64` | Exponent returns the power transform's exponent. |
| `func (s *Sequential[R]) Interpolator() func(float64) R` | Interpolator returns the interpolator in force. |
| `func (s *Sequential[R]) Nice(count int) *Sequential[R]` | Nice extends the domain outward to round values. |
| `func (s *Sequential[R]) Scale(x float64) R` | Scale maps a domain value through the interpolator. |
| `func (s *Sequential[R]) SetBase(b float64) *Sequential[R]` | SetBase sets the log transform's base (meaningful for `NewSequentialLog`). |
| `func (s *Sequential[R]) SetClamp(c bool) *Sequential[R]` | SetClamp turns clamping on or off. |
| `func (s *Sequential[R]) SetConstant(c float64) *Sequential[R]` | SetConstant sets the symlog transform's constant (meaningful for `NewSequentialSymlog`). |
| `func (s *Sequential[R]) SetDomain(x0, x1 float64) *Sequential[R]` | SetDomain sets the domain and returns the scale. |
| `func (s *Sequential[R]) SetExponent(e float64) *Sequential[R]` | SetExponent sets the power transform's exponent (meaningful for `NewSequentialPow` and `NewSequentialSqrt`). |
| `func (s *Sequential[R]) SetInterpolator(f func(float64) R) *Sequential[R]` | SetInterpolator replaces the interpolator and returns the scale. |
| `func (s *Sequential[R]) SetRange(interp Interpolator[R], r0, r1 R) *Sequential[R]` | SetRange builds the interpolator from two range endpoints, which is d3's `range([r0, r1])`. |
| `func (s *Sequential[R]) SetUnknown(v R) *Sequential[R]` | SetUnknown sets the value produced for a NaN input. |
| `func (s *Sequential[R]) TickFormat(count int) func(float64) string` | TickFormat returns a formatter for the values from `Sequential.Ticks`. |
| `func (s *Sequential[R]) Ticks(count int) []float64` | Ticks returns approximately count domain ticks. |
| `func (s *Sequential[R]) Unknown() R` | Unknown returns the value produced for a NaN input. |

</details>

<details>
<summary><code>SequentialQuantile</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewSequentialQuantile[R any](interp func(float64) R) *SequentialQuantile[R]` | NewSequentialQuantile returns an empty sequential quantile scale. |
| `func (s *SequentialQuantile[R]) Copy() *SequentialQuantile[R]` | Copy returns an independent copy of the scale. |
| `func (s *SequentialQuantile[R]) Domain() []float64` | Domain returns a copy of the sorted domain. |
| `func (s *SequentialQuantile[R]) Interpolator() func(float64) R` | Interpolator returns the interpolator in force. |
| `func (s *SequentialQuantile[R]) Quantiles(n int) []float64` | Quantiles returns n+1 evenly spaced quantiles of the domain, which are the breakpoints a legend for this scale should be labelled with. |
| `func (s *SequentialQuantile[R]) Scale(x float64) R` | Scale maps a value to its rank position in the domain and through the interpolator. |
| `func (s *SequentialQuantile[R]) SetDomain(values ...float64) *SequentialQuantile[R]` | SetDomain sets the domain to the given observations, dropping NaNs and sorting ascending. |
| `func (s *SequentialQuantile[R]) SetInterpolator(f func(float64) R) *SequentialQuantile[R]` | SetInterpolator replaces the interpolator. |
| `func (s *SequentialQuantile[R]) SetRange(interp Interpolator[R], r0, r1 R) *SequentialQuantile[R]` | SetRange builds the interpolator from two endpoints. |
| `func (s *SequentialQuantile[R]) SetUnknown(v R) *SequentialQuantile[R]` | SetUnknown sets the value produced for a NaN input. |
| `func (s *SequentialQuantile[R]) Unknown() R` | Unknown returns the value produced for a NaN input or an empty domain. |

</details>

<details>
<summary><code>Threshold</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewThreshold[R comparable](cuts []float64, rng ...R) *Threshold[R]` | NewThreshold returns a threshold scale with the given cut points and range. |
| `func (t *Threshold[R]) Copy() *Threshold[R]` | Copy returns an independent copy of the scale. |
| `func (t *Threshold[R]) Domain() []float64` | Domain returns a copy of the cut points. |
| `func (t *Threshold[R]) InvertExtent(y R) (float64, float64)` | InvertExtent returns the domain interval that maps to y. |
| `func (t *Threshold[R]) Range() []R` | Range returns a copy of the range. |
| `func (t *Threshold[R]) Scale(x float64) R` | Scale maps a value to its bucket. |
| `func (t *Threshold[R]) SetDomain(cuts ...float64) *Threshold[R]` | SetDomain sets the cut points, which must be ascending. |
| `func (t *Threshold[R]) SetRange(rng ...R) *Threshold[R]` | SetRange sets the range. |
| `func (t *Threshold[R]) SetUnknown(v R) *Threshold[R]` | SetUnknown sets the value produced for a NaN input. |
| `func (t *Threshold[R]) Unknown() R` | Unknown returns the value produced for a NaN input. |

</details>

<details>
<summary><code>Time</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewTime() *Time` | NewTime returns a time scale in local time, with a domain of 2000-01-01 to 2000-01-02 and the unit range, matching d3's default. |
| `func NewUTC() *Time` | NewUTC returns a time scale in UTC. |
| `func (t *Time) Clamp() bool` | Clamp reports whether clamping is on. |
| `func (t *Time) Copy() *Time` | Copy returns an independent copy of the scale. |
| `func (t *Time) Domain() []time.Time` | Domain returns a copy of the domain. |
| `func (t *Time) Interval(count int) *timefmt.Interval` | Interval returns the tick interval `Time.Ticks` would choose for count ticks. |
| `func (t *Time) Invert(y float64) time.Time` | Invert maps a range value back to an instant. |
| `func (t *Time) Location() *time.Location` | Location returns the time zone instants are returned in. |
| `func (t *Time) Nice(count int) *Time` | Nice extends the domain outward to the nearest boundaries of the interval that `Time.Ticks` would use, so an axis starts and ends on a labelled tick. |
| `func (t *Time) NiceInterval(iv *timefmt.Interval) *Time` | NiceInterval extends the domain outward to the boundaries of a specific interval, for when the automatic choice is not the one you want (a weekly… |
| `func (t *Time) Range() []float64` | Range returns a copy of the range. |
| `func (t *Time) Scale(x time.Time) float64` | Scale maps an instant to a range value. |
| `func (t *Time) SetClamp(c bool) *Time` | SetClamp turns clamping on or off. |
| `func (t *Time) SetDomain(d ...time.Time) *Time` | SetDomain sets the domain. |
| `func (t *Time) SetRange(r ...float64) *Time` | SetRange sets the range. |
| `func (t *Time) SetRangeRound(r ...float64) *Time` | SetRangeRound sets the range and rounds outputs to whole pixels. |
| `func (t *Time) SetUnknown(v float64) *Time` | SetUnknown sets the value produced for a zero instant. |
| `func (t *Time) TickFormat(count int) func(time.Time) string` | TickFormat returns a formatter that applies the cascade described above. |
| `func (t *Time) Ticks(count int) []time.Time` | Ticks returns calendar-aligned ticks spanning the domain: whole seconds, 5-minute marks, midnights, Sundays, first-of-months or whole years,… |
| `func (t *Time) UTC() bool` | UTC reports whether the scale rounds and formats in UTC. |
| `func (t *Time) Unknown() float64` | Unknown returns the value produced for a zero instant. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/scale).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
