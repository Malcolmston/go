// Package ease is a Go port of d3-ease — the easing functions that shape how a
// value moves from its start to its end over the course of an animation.
//
// An easing is nothing more than a reparameterisation of time: a function that
// takes normalized time t in [0, 1] and returns normalized progress, also
// nominally in [0, 1]. Interpolation asks "what value is 40% of the way from a
// to b?"; easing asks "how far along are we when 40% of the wall-clock time has
// passed?". Compose the two and you get motion with character — a transition
// that accelerates out of rest ([CubicIn]), settles gently into place
// ([CubicOut]), or overshoots and springs back ([ElasticOut]):
//
//	p := ease.CubicInOut(elapsed / duration)
//	x := x0 + (x1-x0)*p
//
// Every easing here has the signature func(float64) float64, and the named type
// [Ease] exists so that easings can be stored, passed around and selected at
// runtime:
//
//	var e ease.Ease = ease.BounceOut
//	fmt.Println(e(0.25))
//
// # The three families
//
// Most easings come in three variants, following d3's naming. The "In" variant
// applies the shaping function forwards, so motion starts slow and finishes
// fast. The "Out" variant reverses it (out(t) = 1 - in(1-t)), so motion starts
// fast and decelerates. The "InOut" variant runs In over the first half and Out
// over the second, giving the symmetric ease-in-ease-out curve that reads as
// "natural" for most UI motion. Every InOut variant in this package satisfies
// f(0.5+x) + f(0.5-x) == 1 and therefore f(0.5) == 0.5; that symmetry is
// asserted in the tests.
//
// # Endpoints
//
// Every exported easing satisfies f(0) == 0 and f(1) == 1 exactly — not to
// within a tolerance, but bit-for-bit. That is the single strongest cheap check
// available for this package, so it is written as one table-driven test over
// every exported easing. Several implementations are shaped specifically to
// make it hold: [SinIn] special-cases t == 1 because math.Cos(π/2) is not
// exactly zero, and the exponential family routes through tpmt, whose scaling
// constant is chosen so that tpmt(0) is exactly 1.
//
// # Outside the unit interval
//
// d3's easings are deliberately *not* clamped. Passing t outside [0, 1] gives a
// smooth extrapolation of the curve rather than a clamped endpoint, which is
// what you want when an easing is composed with another function or sampled
// slightly past its domain — but it does mean a caller who cannot guarantee
// t ∈ [0, 1] should clamp first. [Ease.Clamped] does that.
//
// Separately, and by design, two families leave the unit interval *inside* it:
// [BackIn] dips below zero near the start and [BackOut] overshoots past one near
// the end (that is the "anticipation" and "follow-through" of the animation
// principle it is named for), and the [ElasticIn]/[ElasticOut] family oscillates
// around the endpoint before settling. If you are feeding an easing into
// something that cannot tolerate values outside [0, 1] — a color channel, an
// opacity — do not use Back or Elastic, or clamp the *result* rather than the
// input.
//
// # Parameterised easings
//
// d3 exposes tunable easings through chained setters (easePolyIn.exponent(2),
// easeElasticOut.period(0.5)). Go has no such idiom, so this package splits each
// tunable family in two: a plain function carrying d3's default parameter
// ([PolyIn], [BackOut], [ElasticInOut]) and a "With" constructor that returns a
// configured [Ease] ([PolyInWith], [BackOutWith], [ElasticInOutWith]). The
// defaults are exported as [DefaultExponent], [DefaultOvershoot],
// [DefaultAmplitude] and [DefaultPeriod], and the plain functions are exactly
// their With counterparts at those values.
//
// The formulas are ported directly from d3-ease rather than re-derived, so
// results agree with the JavaScript original to floating-point rounding.
package ease

import "math"

// Ease is an easing function: it maps normalized time to normalized progress.
//
// The type exists so easings can be stored in variables, maps and struct fields
// and chosen at runtime. Any func(float64) float64 converts to it implicitly on
// assignment, so every exported easing in this package can be used as an Ease
// without a conversion.
type Ease func(t float64) float64

// Clamped returns an easing that clamps its input to [0, 1] before applying e.
//
// d3's easings extrapolate outside the unit interval (see the package doc), and
// that is usually the right default. Clamped is for the case where a caller
// cannot guarantee the domain — a timer that overshoots its duration, say — and
// would rather pin the endpoints than extrapolate. Note that this clamps the
// *input*: [BackOut] and [ElasticOut] still return values outside [0, 1] for
// inputs inside it, because that overshoot is the whole point of those curves.
func (e Ease) Clamped() Ease {
	return func(t float64) float64 {
		switch {
		case t <= 0:
			return e(0)
		case t >= 1:
			return e(1)
		default:
			return e(t)
		}
	}
}

// Default parameters for the tunable easing families, matching d3-ease.
const (
	// DefaultExponent is the exponent used by [PolyIn], [PolyOut] and
	// [PolyInOut]. At 3 they coincide exactly with the cubic easings.
	DefaultExponent = 3

	// DefaultOvershoot is the overshoot used by [BackIn], [BackOut] and
	// [BackInOut]. The oddly specific 1.70158 comes from Robert Penner's
	// original easing equations, where it is the value that makes the curve
	// overshoot by 10%.
	DefaultOvershoot = 1.70158

	// DefaultAmplitude is the amplitude used by the [ElasticIn] family. Values
	// below 1 are raised to 1, since a smaller amplitude cannot reach the
	// endpoint.
	DefaultAmplitude = 1

	// DefaultPeriod is the period used by the [ElasticIn] family, expressed as a
	// fraction of the total duration.
	DefaultPeriod = 0.3
)

const (
	halfPi = math.Pi / 2
	tau    = 2 * math.Pi
)

// tpmt is "two power minus ten times t", rescaled so that tpmt(0) == 1 and
// tpmt(1) == 0 exactly.
//
// The naive exponential easing 2^(-10t) is 1 at t=0 but 2^-10 ≈ 0.000977 at
// t=1, so an animation built on it never quite arrives. d3 fixes that by
// subtracting the endpoint and rescaling; the magic constant is the double
// nearest 1/(1 - 2^-10), chosen so the endpoints land on exact 1 and 0 after
// rounding. Both the exponential and elastic families are built on it.
//
// The explicit float64 conversion is load-bearing. Go permits an implementation
// to fuse x*y + z into a single fused multiply-add with only one rounding, and
// on arm64 the gc compiler does exactly that when tpmt is inlined into
// [ExpOut]'s "1 - tpmt(t)". The extra precision is not a gift here: it makes
// ExpOut(0) come out as 2^-60 instead of the exact 0 that JavaScript (which has
// no FMA) produces and that this package's endpoint invariant requires. Per the
// Go spec an explicit conversion rounds the intermediate to float64 and so
// forbids the fusion.
func tpmt(x float64) float64 {
	return float64((math.Pow(2, -10*x) - 0.0009765625) * 1.0009775171065494)
}

// Linear is the identity easing: progress equals elapsed time. Motion under it
// starts and stops abruptly, which is why it is rarely the right choice for UI
// animation, but it is the correct choice whenever the underlying quantity is
// genuinely linear in time.
func Linear(t float64) float64 { return t }

// QuadIn accelerates from rest: t².
func QuadIn(t float64) float64 { return t * t }

// QuadOut decelerates to rest: the mirror of [QuadIn].
func QuadOut(t float64) float64 { return t * (2 - t) }

// QuadInOut accelerates then decelerates, symmetric about t = 0.5.
func QuadInOut(t float64) float64 {
	t *= 2
	if t <= 1 {
		return t * t / 2
	}
	t--
	return (t*(2-t) + 1) / 2
}

// CubicIn accelerates from rest: t³. This is the most commonly used easing in
// UI work, together with [CubicOut] and [CubicInOut].
func CubicIn(t float64) float64 { return t * t * t }

// CubicOut decelerates to rest: the mirror of [CubicIn].
func CubicOut(t float64) float64 {
	t--
	return t*t*t + 1
}

// CubicInOut accelerates then decelerates, symmetric about t = 0.5.
func CubicInOut(t float64) float64 {
	t *= 2
	if t <= 1 {
		return t * t * t / 2
	}
	t -= 2
	return (t*t*t + 2) / 2
}

// PolyInWith returns the polynomial ease-in t^exponent.
//
// Larger exponents hold the animation near zero for longer and then snap; an
// exponent of 1 degenerates to [Linear] and 2 and 3 reproduce [QuadIn] and
// [CubicIn].
func PolyInWith(exponent float64) Ease {
	return func(t float64) float64 { return math.Pow(t, exponent) }
}

// PolyOutWith returns the polynomial ease-out, the mirror of [PolyInWith].
func PolyOutWith(exponent float64) Ease {
	return func(t float64) float64 { return 1 - math.Pow(1-t, exponent) }
}

// PolyInOutWith returns the symmetric polynomial ease-in-out.
func PolyInOutWith(exponent float64) Ease {
	return func(t float64) float64 {
		t *= 2
		if t <= 1 {
			return math.Pow(t, exponent) / 2
		}
		return (2 - math.Pow(2-t, exponent)) / 2
	}
}

// PolyIn is [PolyInWith] at [DefaultExponent], and so agrees with [CubicIn].
func PolyIn(t float64) float64 { return math.Pow(t, DefaultExponent) }

// PolyOut is [PolyOutWith] at [DefaultExponent], and so agrees with [CubicOut].
func PolyOut(t float64) float64 { return 1 - math.Pow(1-t, DefaultExponent) }

// PolyInOut is [PolyInOutWith] at [DefaultExponent].
func PolyInOut(t float64) float64 {
	t *= 2
	if t <= 1 {
		return math.Pow(t, DefaultExponent) / 2
	}
	return (2 - math.Pow(2-t, DefaultExponent)) / 2
}

// SinIn eases in along a quarter of a cosine wave.
//
// The explicit t == 1 case is not paranoia: math.Cos(π/2) is about 6.1e-17
// rather than 0, because π/2 is not exactly representable, so without the
// special case SinIn(1) would be 1 - 6.1e-17 and the package-wide endpoint
// invariant would fail. d3 carries the same guard.
func SinIn(t float64) float64 {
	if t == 1 {
		return 1
	}
	return 1 - math.Cos(t*halfPi)
}

// SinOut eases out along a quarter of a sine wave.
func SinOut(t float64) float64 { return math.Sin(t * halfPi) }

// SinInOut eases in and out along half a cosine wave. Of all the InOut
// easings this one has the gentlest curvature.
func SinInOut(t float64) float64 { return (1 - math.Cos(math.Pi*t)) / 2 }

// ExpIn eases in exponentially: very slow at first, then very fast. See tpmt
// for why this is 2^(-10t) rescaled rather than the raw power.
func ExpIn(t float64) float64 { return tpmt(1 - t) }

// ExpOut eases out exponentially, the mirror of [ExpIn].
func ExpOut(t float64) float64 { return 1 - tpmt(t) }

// ExpInOut eases in and out exponentially, symmetric about t = 0.5.
func ExpInOut(t float64) float64 {
	t *= 2
	if t <= 1 {
		return tpmt(1-t) / 2
	}
	return (2 - tpmt(t-1)) / 2
}

// CircleIn eases in along the arc of a circle: gentle at first, with a vertical
// tangent at t = 1.
func CircleIn(t float64) float64 { return 1 - math.Sqrt(1-t*t) }

// CircleOut eases out along the arc of a circle, the mirror of [CircleIn].
func CircleOut(t float64) float64 {
	t--
	return math.Sqrt(1 - t*t)
}

// CircleInOut eases in and out along circular arcs, symmetric about t = 0.5.
func CircleInOut(t float64) float64 {
	t *= 2
	if t <= 1 {
		return (1 - math.Sqrt(1-t*t)) / 2
	}
	t -= 2
	return (math.Sqrt(1-t*t) + 1) / 2
}

// elasticParams normalizes an amplitude/period pair the way d3 does: the
// amplitude is floored at 1 (an amplitude below 1 could never reach the
// endpoint, so d3 clamps rather than producing a curve that misses), the period
// is divided by τ to convert it into radians per unit time, and the phase shift
// s is chosen so the oscillation passes exactly through the endpoint.
func elasticParams(amplitude, period float64) (a, p, s float64) {
	a = math.Max(1, amplitude)
	p = period / tau
	s = math.Asin(1/a) * p
	return
}

// ElasticInWith returns an elastic ease-in with the given amplitude and period:
// an oscillation that grows until it snaps into the endpoint.
//
// Elastic easings deliberately leave the unit interval — an ease-in dips below
// zero on its way up. See the package doc.
func ElasticInWith(amplitude, period float64) Ease {
	a, p, s := elasticParams(amplitude, period)
	return func(t float64) float64 {
		t--
		return a * tpmt(-t) * math.Sin((s-t)/p)
	}
}

// ElasticOutWith returns an elastic ease-out: the value overshoots the endpoint
// and rings down onto it. This is the elastic variant you almost always want,
// because the oscillation happens where the eye is looking — at the end.
func ElasticOutWith(amplitude, period float64) Ease {
	a, p, s := elasticParams(amplitude, period)
	return func(t float64) float64 {
		return 1 - a*tpmt(t)*math.Sin((t+s)/p)
	}
}

// ElasticInOutWith returns an elastic ease-in-out, oscillating at both ends.
func ElasticInOutWith(amplitude, period float64) Ease {
	a, p, s := elasticParams(amplitude, period)
	return func(t float64) float64 {
		t = t*2 - 1
		if t < 0 {
			return a * tpmt(-t) * math.Sin((s-t)/p) / 2
		}
		return (2 - a*tpmt(t)*math.Sin((s+t)/p)) / 2
	}
}

var (
	elasticIn    = ElasticInWith(DefaultAmplitude, DefaultPeriod)
	elasticOut   = ElasticOutWith(DefaultAmplitude, DefaultPeriod)
	elasticInOut = ElasticInOutWith(DefaultAmplitude, DefaultPeriod)
)

// ElasticIn is [ElasticInWith] at [DefaultAmplitude] and [DefaultPeriod].
func ElasticIn(t float64) float64 { return elasticIn(t) }

// ElasticOut is [ElasticOutWith] at [DefaultAmplitude] and [DefaultPeriod].
func ElasticOut(t float64) float64 { return elasticOut(t) }

// ElasticInOut is [ElasticInOutWith] at [DefaultAmplitude] and [DefaultPeriod].
func ElasticInOut(t float64) float64 { return elasticInOut(t) }

// BackInWith returns a back ease-in with the given overshoot: motion pulls back
// below zero before moving forward, the "anticipation" of classical animation.
//
// Back easings deliberately leave the unit interval. See the package doc.
func BackInWith(overshoot float64) Ease {
	return func(t float64) float64 { return t * t * (overshoot*(t-1) + t) }
}

// BackOutWith returns a back ease-out: motion overshoots the endpoint and
// settles back onto it, the "follow-through" of classical animation.
func BackOutWith(overshoot float64) Ease {
	return func(t float64) float64 {
		t--
		return t*t*((t+1)*overshoot+t) + 1
	}
}

// BackInOutWith returns a back ease-in-out, anticipating at the start and
// overshooting at the end.
func BackInOutWith(overshoot float64) Ease {
	return func(t float64) float64 {
		t *= 2
		if t < 1 {
			return t * t * ((overshoot+1)*t - overshoot) / 2
		}
		t -= 2
		return (t*t*((overshoot+1)*t+overshoot) + 2) / 2
	}
}

// BackIn is [BackInWith] at [DefaultOvershoot].
func BackIn(t float64) float64 { return t * t * (DefaultOvershoot*(t-1) + t) }

// BackOut is [BackOutWith] at [DefaultOvershoot].
func BackOut(t float64) float64 {
	t--
	return t*t*((t+1)*DefaultOvershoot+t) + 1
}

// BackInOut is [BackInOutWith] at [DefaultOvershoot].
func BackInOut(t float64) float64 {
	t *= 2
	if t < 1 {
		return t * t * ((DefaultOvershoot+1)*t - DefaultOvershoot) / 2
	}
	t -= 2
	return (t*t*((DefaultOvershoot+1)*t+DefaultOvershoot) + 2) / 2
}

// The bounce breakpoints. Each segment is a parabola; the constants are chosen
// so the segments meet with matching height and the final one lands exactly on
// 1 at t = 1. b0 is the shared leading coefficient, (11/4)².
const (
	b1 = 4.0 / 11
	b2 = 6.0 / 11
	b3 = 8.0 / 11
	b4 = 3.0 / 4
	b5 = 9.0 / 11
	b6 = 10.0 / 11
	b7 = 15.0 / 16
	b8 = 21.0 / 22
	b9 = 63.0 / 64
	b0 = 1 / b1 / b1
)

// BounceOut eases out with a bouncing motion, as if the value were a ball
// dropped onto the endpoint: it arrives, rebounds, and arrives again with
// diminishing energy.
//
// Unlike the other "Out" easings this one is not monotone — it reaches 1 at
// t = 4/11 and falls away again — so it is excluded from the package's
// monotonicity test. It never leaves [0, 1], however.
func BounceOut(t float64) float64 {
	switch {
	case t < b1:
		return b0 * t * t
	case t < b3:
		t -= b2
		return b0*t*t + b4
	case t < b6:
		t -= b5
		return b0*t*t + b7
	default:
		t -= b8
		return b0*t*t + b9
	}
}

// BounceIn is the time-reverse of [BounceOut]: the bounces happen at the start.
func BounceIn(t float64) float64 { return 1 - BounceOut(1-t) }

// BounceInOut bounces at both ends, symmetric about t = 0.5.
func BounceInOut(t float64) float64 {
	t *= 2
	if t <= 1 {
		return (1 - BounceOut(1-t)) / 2
	}
	return (BounceOut(t-1) + 1) / 2
}
