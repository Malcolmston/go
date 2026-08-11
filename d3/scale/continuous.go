package scale

import (
	"math"

	"github.com/malcolmston/d3/array"
	"github.com/malcolmston/d3/interpolate"
)

// Interpolator builds the function that walks from a to b as t goes 0→1. It is
// the same shape as interpolate.Number and interpolate.RGB, so those can be
// passed directly as the range interpolator of a continuous scale.
type Interpolator[R any] func(a, b R) func(t float64) R

// Uninterpolator is the inverse of an [Interpolator]: given the two endpoints
// it returns the function that recovers t from a value. Only scales that have
// one can be inverted; numeric ranges get one automatically, arbitrary ones do
// not, which mirrors d3 (where `invert` is documented as meaningful only for a
// numeric range).
type Uninterpolator[R any] func(a, b R) func(y R) float64

// kind selects the domain transform a continuous scale applies before
// normalizing. Keeping all the continuous variants in one type — rather than
// one Go type per d3 constructor — is a deliberate trade: d3 builds pow/log/
// symlog by wrapping the same `transformer` closure, and duplicating the
// twenty-odd accessor methods across seven Go types to model that would be
// pure boilerplate with more places to drift. The price is that a few accessors
// are only meaningful for some kinds ([Continuous.SetBase] on a linear scale is
// a no-op), which is documented on each.
type kind int

const (
	kindLinear kind = iota
	kindPow
	kindLog
	kindSymlog
	kindIdentity
)

// Continuous is a continuous, quantitative scale: it maps a continuous domain
// onto a continuous range by normalizing the input to a position in [0, 1]
// within the domain and interpolating the range at that position. Construct one
// with [NewLinear], [NewPow], [NewSqrt], [NewLog], [NewSymlog] or
// [NewIdentity]; [NewLinearOf] builds one over a non-numeric range such as a
// color.
//
// Two behaviors are worth internalizing because they are the source of most
// surprises. First, a continuous scale EXTRAPOLATES: an input outside the
// domain produces an output outside the range, linearly continued, unless
// [Continuous.SetClamp] is on. Second, when the domain and range have more than
// two entries the scale becomes *polylinear* — each adjacent pair of domain
// values maps onto the corresponding pair of range values, so a three-stop
// domain with a three-color range is a two-segment color ramp with a fixed
// pivot. The number of segments is min(len(domain), len(range)) - 1; extra
// entries on either side are ignored, exactly as in d3.
//
// The zero value is not usable; always use a constructor.
type Continuous[R any] struct {
	kind      kind
	domain    []float64
	rangeVals []R
	clamp     bool

	interp      Interpolator[R]
	roundInterp Interpolator[R] // used by SetRangeRound; nil for non-numeric R
	uninterp    Uninterpolator[R]

	unknown R

	exponent float64 // pow/sqrt
	base     float64 // log
	constant float64 // symlog

	// Derived by rescale.
	tf, utf    func(float64) float64
	logs, pows func(float64) float64 // log only

	output func(float64) R // lazily built piecewise map, cleared by rescale
}

// ---------------------------------------------------------------------------
// construction
// ---------------------------------------------------------------------------

func newContinuous[R any](k kind, interp Interpolator[R]) *Continuous[R] {
	s := &Continuous[R]{
		kind:      k,
		domain:    []float64{0, 1},
		rangeVals: nil,
		interp:    interp,
		exponent:  1,
		base:      10,
		constant:  1,
	}
	return s
}

func numericBase(k kind) *Continuous[float64] {
	s := newContinuous[float64](k, Interpolator[float64](interpolate.Number))
	s.rangeVals = []float64{0, 1}
	s.roundInterp = Interpolator[float64](interpolate.Round)
	s.uninterp = uninterpolateNumber
	s.rescale()
	return s
}

// NewLinear returns a linear scale with the unit domain and unit range, the
// workhorse of the package: position, size, opacity and anything else where
// equal steps in the data should be equal steps on screen.
func NewLinear() *Continuous[float64] { return numericBase(kindLinear) }

// NewLinearOf returns a linear scale over an arbitrary range type, given the
// interpolator that walks between two range values. Pass interpolate.RGB for a
// color ramp:
//
//	s := scale.NewLinearOf[color.Color](interpolate.RGB).
//		SetDomain(0, 100).SetRangeValues(red, blue)
//
// The result cannot be inverted unless you also supply an [Uninterpolator] via
// [Continuous.SetUninterpolate]; [Continuous.Invert] returns NaN otherwise.
func NewLinearOf[R any](interp Interpolator[R]) *Continuous[R] {
	s := newContinuous[R](kindLinear, interp)
	s.rescale()
	return s
}

// NewPow returns a power scale, which applies x ↦ sign(x)·|x|^k before
// normalizing. The exponent defaults to 1 (making it linear); set it with
// [Continuous.SetExponent]. Negative inputs are handled by reflecting the
// transform through the origin rather than producing NaN, so a domain that
// straddles zero is legal — unlike a log scale.
func NewPow() *Continuous[float64] { return numericBase(kindPow) }

// NewSqrt returns a power scale with exponent 0.5. It is the right scale for
// encoding a quantity as the *area* of a circle: area grows with the square of
// the radius, so radius must grow with the square root of the value if the
// reader is to compare areas honestly.
func NewSqrt() *Continuous[float64] {
	s := numericBase(kindPow)
	return s.SetExponent(0.5)
}

// NewLog returns a log scale with base 10 and the domain [1, 10]. A log scale's
// domain must not include or cross zero: log is undefined there, so d3 (and
// this port) require a domain that is entirely positive or entirely negative.
// An all-negative domain is fully supported — the transform is reflected — and
// is the usual way to plot drawdowns or losses on a log axis.
func NewLog() *Continuous[float64] {
	s := numericBase(kindLog)
	s.domain = []float64{1, 10}
	s.rescale()
	return s
}

// NewSymlog returns a symmetric-log scale: sign(x)·log1p(|x/C|). Unlike a log
// scale it is defined at and across zero, which makes it the practical choice
// for data that spans several orders of magnitude in both directions. C is the
// [Continuous.Constant], default 1, and sets how wide the near-linear region
// around zero is.
func NewSymlog() *Continuous[float64] { return numericBase(kindSymlog) }

// NewIdentity returns an identity scale: the domain and range are the same
// values and Scale is the identity function. It exists so that code which is
// generic over scales can opt out of scaling while keeping [Continuous.Ticks],
// [Continuous.TickFormat] and [Continuous.Nice] — which is exactly what an axis
// over already-in-pixel data needs.
func NewIdentity() *Continuous[float64] {
	s := numericBase(kindIdentity)
	return s
}

// ---------------------------------------------------------------------------
// accessors
// ---------------------------------------------------------------------------

// Domain returns a copy of the domain.
func (s *Continuous[R]) Domain() []float64 { return append([]float64(nil), s.domain...) }

// SetDomain sets the domain and returns the scale. Two values give the usual
// linear map; three or more make the scale polylinear, with one segment per
// adjacent pair. The domain may be descending (SetDomain(100, 0)), which
// reverses the mapping. A degenerate domain where both ends are equal maps
// every input to the midpoint of the range, matching d3.
//
// For an identity scale the range is kept equal to the domain.
func (s *Continuous[R]) SetDomain(d ...float64) *Continuous[R] {
	s.domain = append([]float64(nil), d...)
	if s.kind == kindIdentity {
		s.syncIdentityRange()
	}
	return s.rescale()
}

// Range returns a copy of the range.
func (s *Continuous[R]) Range() []R { return append([]R(nil), s.rangeVals...) }

// SetRange sets the range and returns the scale. See [Continuous.SetDomain] for
// the polylinear case.
func (s *Continuous[R]) SetRange(r ...R) *Continuous[R] {
	s.rangeVals = append([]R(nil), r...)
	if s.kind == kindIdentity {
		s.syncIdentityDomain()
	}
	return s.rescale()
}

// SetRangeRound sets the range and switches to a rounding interpolator, so
// outputs land on whole numbers. This is d3's rangeRound, and it is what you
// want when the output is device pixels: it removes the half-pixel blurring
// that comes from drawing on fractional coordinates. Note that it rounds the
// *output*, not the domain — [Continuous.Invert] still returns fractional
// domain values.
//
// For a non-numeric range type there is no rounding interpolator, so this is
// equivalent to [Continuous.SetRange].
func (s *Continuous[R]) SetRangeRound(r ...R) *Continuous[R] {
	if s.roundInterp != nil {
		s.interp = s.roundInterp
	}
	return s.SetRange(r...)
}

// SetInterpolate replaces the range interpolator and returns the scale.
func (s *Continuous[R]) SetInterpolate(i Interpolator[R]) *Continuous[R] {
	s.interp = i
	return s.rescale()
}

// Interpolate returns the range interpolator in force.
func (s *Continuous[R]) Interpolate() Interpolator[R] { return s.interp }

// SetUninterpolate supplies the inverse of the range interpolator, which is
// what makes [Continuous.Invert] work for a non-numeric range.
func (s *Continuous[R]) SetUninterpolate(u Uninterpolator[R]) *Continuous[R] {
	s.uninterp = u
	return s.rescale()
}

// Clamp reports whether clamping is on.
func (s *Continuous[R]) Clamp() bool { return s.clamp }

// SetClamp turns clamping on or off and returns the scale. With clamping off
// (the default) the scale extrapolates: an input below the domain produces an
// output below the range. With it on, inputs are pinned to the domain first, so
// the output never escapes the range — which is what you want when the range is
// a color ramp or a bounded pixel area.
func (s *Continuous[R]) SetClamp(c bool) *Continuous[R] {
	s.clamp = c
	return s
}

// Unknown returns the value produced for a NaN input.
func (s *Continuous[R]) Unknown() R { return s.unknown }

// SetUnknown sets the value produced for a NaN input and returns the scale.
func (s *Continuous[R]) SetUnknown(v R) *Continuous[R] {
	s.unknown = v
	return s
}

// Exponent returns the power scale's exponent (1 for other kinds).
func (s *Continuous[R]) Exponent() float64 { return s.exponent }

// SetExponent sets the exponent of a power scale. It has no effect on other
// kinds, matching d3, where the accessor simply does not exist on them.
func (s *Continuous[R]) SetExponent(e float64) *Continuous[R] {
	s.exponent = e
	return s.rescale()
}

// Base returns the log scale's base (10 by default).
func (s *Continuous[R]) Base() float64 { return s.base }

// SetBase sets the base of a log scale. Non-integer bases are allowed; tick
// generation notices and falls back to evenly spaced ticks in log space rather
// than trying to enumerate the (meaningless) "digits" between powers.
func (s *Continuous[R]) SetBase(b float64) *Continuous[R] {
	s.base = b
	return s.rescale()
}

// Constant returns the symlog scale's constant (1 by default).
func (s *Continuous[R]) Constant() float64 { return s.constant }

// SetConstant sets the symlog constant, which controls the width of the
// near-linear region around zero.
func (s *Continuous[R]) SetConstant(c float64) *Continuous[R] {
	s.constant = c
	return s.rescale()
}

// ---------------------------------------------------------------------------
// internals
// ---------------------------------------------------------------------------

func (s *Continuous[R]) syncIdentityRange() {
	if c, ok := any(s).(*Continuous[float64]); ok {
		c.rangeVals = append([]float64(nil), c.domain...)
	}
}

func (s *Continuous[R]) syncIdentityDomain() {
	if c, ok := any(s).(*Continuous[float64]); ok {
		c.domain = append([]float64(nil), c.rangeVals...)
	}
}

// rescale recomputes the domain transform and drops the cached piecewise map.
// Every setter funnels through it, which is why the cached map can be trusted
// without a dirty flag.
func (s *Continuous[R]) rescale() *Continuous[R] {
	switch s.kind {
	case kindPow:
		e := s.exponent
		switch {
		case e == 1:
			s.tf, s.utf = identityF, identityF
		case e == 0.5:
			s.tf, s.utf = transformSqrt, transformSquare
		default:
			s.tf = transformPow(e)
			s.utf = transformPow(1 / e)
		}
	case kindLog:
		s.logs = logp(s.base)
		s.pows = powp(s.base)
		if len(s.domain) > 0 && s.domain[0] < 0 {
			s.logs = reflect(s.logs)
			s.pows = reflect(s.pows)
			s.tf, s.utf = transformLogn, transformExpn
		} else {
			s.tf, s.utf = transformLog, transformExp
		}
	case kindSymlog:
		s.tf = transformSymlog(s.constant)
		s.utf = transformSymexp(s.constant)
	default:
		s.tf, s.utf = identityF, identityF
	}
	s.output = nil
	return s
}

// segments returns the number of usable segments plus the truncated transformed
// domain and range, which is min(len(domain), len(range)) entries.
func (s *Continuous[R]) segments() (td []float64, r []R) {
	n := len(s.domain)
	if len(s.rangeVals) < n {
		n = len(s.rangeVals)
	}
	if n < 2 {
		return nil, nil
	}
	td = make([]float64, n)
	for i := 0; i < n; i++ {
		td[i] = s.tf(s.domain[i])
	}
	return td, s.rangeVals[:n]
}

// buildOutput assembles the normalize→interpolate pipeline. The two-stop case
// is special-cased not for speed but for exactness: with one segment there is
// no bisection and no reversal bookkeeping, so endpoints come out bit-identical
// to the range endpoints.
func (s *Continuous[R]) buildOutput() func(float64) R {
	td, r := s.segments()
	if td == nil {
		var zero R
		return func(float64) R { return zero }
	}
	if len(td) == 2 {
		return bimap(td[0], td[1], r[0], r[1], s.interp)
	}
	return polymap(td, r, s.interp)
}

func bimap[R any](d0, d1 float64, r0, r1 R, interp Interpolator[R]) func(float64) R {
	if d1 < d0 {
		n := normalize(d1, d0)
		f := interp(r1, r0)
		return func(x float64) R { return f(n(x)) }
	}
	n := normalize(d0, d1)
	f := interp(r0, r1)
	return func(x float64) R { return f(n(x)) }
}

func polymap[R any](td []float64, r []R, interp Interpolator[R]) func(float64) R {
	j := len(td) - 1
	d := append([]float64(nil), td...)
	rr := append([]R(nil), r...)
	if d[j] < d[0] {
		reverseFloats(d)
		reverseSlice(rr)
	}
	norms := make([]func(float64) float64, j)
	interps := make([]func(float64) R, j)
	for i := 0; i < j; i++ {
		norms[i] = normalize(d[i], d[i+1])
		interps[i] = interp(rr[i], rr[i+1])
	}
	return func(x float64) R {
		i := bisectRight(d, x, 1, j) - 1
		return interps[i](norms[i](x))
	}
}

// normalize returns x ↦ (x-a)/(b-a). A degenerate span collapses to the
// constant 0.5 (d3's behavior: a zero-width domain lands in the middle of the
// range rather than producing ±Inf), and a NaN span propagates NaN.
func normalize(a, b float64) func(float64) float64 {
	span := b - a
	switch {
	case math.IsNaN(span):
		return func(float64) float64 { return math.NaN() }
	case span == 0:
		return func(float64) float64 { return 0.5 }
	default:
		return func(x float64) float64 { return (x - a) / span }
	}
}

func clampTo(x, a, b float64) float64 {
	if a > b {
		a, b = b, a
	}
	return math.Max(a, math.Min(b, x))
}

// ---------------------------------------------------------------------------
// the callable
// ---------------------------------------------------------------------------

// Scale maps a domain value to a range value — d3's `scale(x)`. A NaN input
// yields [Continuous.Unknown].
func (s *Continuous[R]) Scale(x float64) R {
	if math.IsNaN(x) {
		return s.unknown
	}
	if s.kind == kindIdentity {
		if v, ok := any(x).(R); ok {
			return v
		}
	}
	if s.clamp && len(s.domain) > 1 {
		n := len(s.domain)
		if len(s.rangeVals) < n {
			n = len(s.rangeVals)
		}
		if n >= 2 {
			x = clampTo(x, s.domain[0], s.domain[n-1])
		}
	}
	if s.output == nil {
		s.output = s.buildOutput()
	}
	return s.output(s.tf(x))
}

// Invert maps a range value back to a domain value. It is meaningful only when
// the range is numeric (or an [Uninterpolator] has been supplied); otherwise it
// returns NaN. Inversion extrapolates and clamps on the same terms as
// [Continuous.Scale], so Invert(Scale(x)) == x for any x in the domain and, with
// clamping off, for x outside it too.
//
// For a polylinear scale the segment is found by asking each segment for the
// position of y and taking the one that answers within [0, 1]; values off
// either end are extrapolated from the nearest segment. That is equivalent to
// d3's bisection for the monotone ranges d3 itself requires, and it does not
// need R to be ordered.
func (s *Continuous[R]) Invert(y R) float64 {
	if s.kind == kindIdentity {
		if v, ok := any(y).(float64); ok {
			return v
		}
	}
	if s.uninterp == nil {
		return math.NaN()
	}
	td, r := s.segments()
	if td == nil {
		return math.NaN()
	}
	j := len(td) - 1
	pick := 0
	if j > 1 {
		pick = -1
		for i := 0; i < j; i++ {
			t := s.uninterp(r[i], r[i+1])(y)
			if t >= 0 && t <= 1 {
				pick = i
				break
			}
		}
		if pick < 0 {
			if s.uninterp(r[0], r[1])(y) <= 0 {
				pick = 0
			} else {
				pick = j - 1
			}
		}
	}
	t := s.uninterp(r[pick], r[pick+1])(y)
	x := s.utf(interpolate.Number(td[pick], td[pick+1])(t))
	if s.clamp {
		x = clampTo(x, s.domain[0], s.domain[len(td)-1])
	}
	return x
}

// ---------------------------------------------------------------------------
// ticks, nice, format, copy
// ---------------------------------------------------------------------------

// Ticks returns approximately count representative values spanning the domain,
// suitable for axis labels. Delegates to array.Ticks for every kind except log,
// which enumerates the powers of the base (and the digits between them when
// there is room) instead — a log axis labelled at 10, 20, 30 would be a lie
// about where those values sit.
func (s *Continuous[R]) Ticks(count int) []float64 {
	if len(s.domain) == 0 {
		return nil
	}
	d0, d1 := s.domain[0], s.domain[len(s.domain)-1]
	if count <= 0 {
		count = 10
	}
	if s.kind == kindLog {
		return s.logTicks(d0, d1, count)
	}
	return array.Ticks(d0, d1, count)
}

// Nice extends the domain outward to round values, so an axis ends on a label
// rather than mid-air. It is defined in terms of array.TickIncrement — the same
// function that chooses the tick spacing — so the nicened endpoints are
// guaranteed to be ticks. The loop repeats because the first rounding can widen
// the domain enough to change the chosen increment; it converges in a couple of
// passes and is capped at ten.
//
// A log scale instead rounds outward to whole powers of the base.
func (s *Continuous[R]) Nice(count int) *Continuous[R] {
	if len(s.domain) < 2 {
		return s
	}
	if count <= 0 {
		count = 10
	}
	d := append([]float64(nil), s.domain...)
	i0, i1 := 0, len(d)-1

	if s.kind == kindLog {
		start, stop := d[i0], d[i1]
		if stop < start {
			i0, i1 = i1, i0
			start, stop = stop, start
		}
		d[i0] = s.pows(math.Floor(s.logs(start)))
		d[i1] = s.pows(math.Ceil(s.logs(stop)))
		return s.SetDomain(d...)
	}

	start, stop := d[i0], d[i1]
	if stop < start {
		start, stop = stop, start
		i0, i1 = i1, i0
	}
	prestep := math.NaN()
	for iter := 0; iter < 10; iter++ {
		step := array.TickIncrement(start, stop, count)
		switch {
		case step == prestep:
			d[i0], d[i1] = start, stop
			return s.SetDomain(d...)
		case step > 0:
			start = math.Floor(start/step) * step
			stop = math.Ceil(stop/step) * step
		case step < 0:
			start = math.Ceil(start*step) / step
			stop = math.Floor(stop*step) / step
		default:
			return s
		}
		prestep = step
	}
	return s
}

// TickFormat returns a formatter for the values from [Continuous.Ticks], using
// the default specifier for this kind of scale. The precision is filled in from
// the tick step, so a domain of [0, 1] with ten ticks formats as "0.1" and not
// "0.1000000000000000055". See [Continuous.TickFormatSpecifier] to choose the
// notation — currency, percent, SI prefixes — yourself.
//
// For a log scale the formatter additionally *filters*: labelling every one of
// the ~9·log(range) ticks a log axis produces would be unreadable, so ticks
// whose mantissa is too large return the empty string. This mirrors d3 and is
// why a d3 log axis shows 1, 2, 5, 10, 20, 50 rather than every integer.
func (s *Continuous[R]) TickFormat(count int) func(float64) string {
	f, err := s.TickFormatSpecifier(count, "")
	if err != nil {
		// The default specifiers are compile-time constants and always parse.
		panic("scale: default tick format specifier is invalid: " + err.Error())
	}
	return f
}

// TickFormatSpecifier returns a tick formatter built from a d3-format
// specifier, such as "$,.2f" or ".1%". An empty specifier means the default for
// this kind of scale: ",f" for linear-family scales, "s" for a base-10 log
// scale and "," for a log scale in any other base. Where the specifier omits a
// precision, one is derived from the tick step.
func (s *Continuous[R]) TickFormatSpecifier(count int, specifier string) (func(float64) string, error) {
	if count <= 0 {
		count = 10
	}
	if s.kind == kindLog {
		return s.logTickFormat(count, specifier)
	}
	lo, hi := 0.0, 1.0
	if len(s.domain) >= 2 {
		lo, hi = s.domain[0], s.domain[len(s.domain)-1]
	}
	return tickFormatFor(lo, hi, count, specifier)
}

// Copy returns an independent copy of the scale; changes to one do not affect
// the other.
func (s *Continuous[R]) Copy() *Continuous[R] {
	c := *s
	c.domain = append([]float64(nil), s.domain...)
	c.rangeVals = append([]R(nil), s.rangeVals...)
	c.output = nil
	return &c
}

// ---------------------------------------------------------------------------
// transforms
// ---------------------------------------------------------------------------

func identityF(x float64) float64 { return x }

func transformPow(e float64) func(float64) float64 {
	return func(x float64) float64 {
		if x < 0 {
			return -math.Pow(-x, e)
		}
		return math.Pow(x, e)
	}
}

func transformSqrt(x float64) float64 {
	if x < 0 {
		return -math.Sqrt(-x)
	}
	return math.Sqrt(x)
}

func transformSquare(x float64) float64 {
	if x < 0 {
		return -x * x
	}
	return x * x
}

func transformLog(x float64) float64  { return math.Log(x) }
func transformExp(x float64) float64  { return math.Exp(x) }
func transformLogn(x float64) float64 { return -math.Log(-x) }
func transformExpn(x float64) float64 { return -math.Exp(-x) }

func transformSymlog(c float64) func(float64) float64 {
	return func(x float64) float64 {
		return sign(x) * math.Log1p(math.Abs(x/c))
	}
}

func transformSymexp(c float64) func(float64) float64 {
	return func(x float64) float64 {
		return sign(x) * math.Expm1(math.Abs(x)) * c
	}
}

func sign(x float64) float64 {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	default:
		return x // preserves ±0 and NaN, like Math.sign
	}
}

func logp(base float64) func(float64) float64 {
	switch base {
	case math.E:
		return math.Log
	case 10:
		return math.Log10
	case 2:
		return math.Log2
	default:
		lb := math.Log(base)
		return func(x float64) float64 { return math.Log(x) / lb }
	}
}

func powp(base float64) func(float64) float64 {
	switch base {
	case math.E:
		return math.Exp
	default:
		return func(x float64) float64 { return math.Pow(base, x) }
	}
}

func reflect(f func(float64) float64) func(float64) float64 {
	return func(x float64) float64 { return -f(-x) }
}

// uninterpolateNumber is the inverse of interpolate.Number.
func uninterpolateNumber(a, b float64) func(float64) float64 {
	span := b - a
	if span == 0 {
		return func(float64) float64 { return 0 }
	}
	return func(y float64) float64 { return (y - a) / span }
}

// ---------------------------------------------------------------------------
// small helpers
// ---------------------------------------------------------------------------

// bisectRight returns the insertion point for x in the ascending slice a,
// searching only [lo, hi) and returning a value in that range. It is the
// d3-array bisectRight restricted the way polymap needs it.
func bisectRight(a []float64, x float64, lo, hi int) int {
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if x < a[mid] {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}

func reverseFloats(a []float64) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}

func reverseSlice[T any](a []T) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}
