// Package random is a Go port of d3-random — generators for numbers drawn from
// the distributions that turn up in data visualization: normal and log-normal,
// exponential and Poisson, gamma and beta, and a dozen more.
//
// The shape of the API follows d3's. A distribution function does not return a
// number; it returns a [Gen], a zero-argument closure that yields a fresh draw
// each time it is called. Constructing the generator once lets it hoist the
// per-distribution setup — the shape constants of a gamma, the log of a
// geometric's success probability — out of the hot loop:
//
//	next := random.Normal(0, 1)
//	for i := range xs {
//		xs[i] = next()
//	}
//
// Every distribution comes in two forms. The package-level function draws from a
// process-wide, non-deterministic source (math/rand/v2). The identically named
// method on [Rand] draws from an explicit source, which is how you get
// reproducibility:
//
//	r := random.Source(42)
//	next := r.Normal(0, 1)
//
// # Why a hand-written LCG rather than math/rand
//
// Go already ships two good pseudo-random generators, so implementing another
// one needs a reason. The reason is cross-language reproducibility: [Source]
// implements d3's randomLcg exactly — the same 32-bit linear congruential
// generator, the same multiplier 0x19660D and increment 0x3C6EF35F, the same
// seed normalization, the same conversion to a float in [0, 1). A seeded Go run
// therefore produces the same uniform stream as the same seed in JavaScript,
// which is what makes it possible to port a notebook or check a Go chart against
// its d3 original. math/rand/v2's PCG and ChaCha8 are better generators by every
// statistical measure, and that is precisely why they are the *default* source
// here — but they cannot reproduce a JavaScript stream, so they cannot be the
// seeded one.
//
// The uniform stream matches d3 bit for bit. The distributions built on it are
// ported statement by statement from d3's implementations — the same rejection
// sampling in [Rand.Gamma], the same recursive splitting in [Rand.Binomial], the
// same cached second polar deviate in [Rand.Normal] — so they consume the
// stream in the same order and should agree to floating-point rounding. That
// last clause is a real caveat: Go and JavaScript may round math.Log or math.Pow
// differently in the final ulp, and a rejection sampler that lands exactly on
// its acceptance boundary could then diverge. Treat exact parity as expected for
// the uniform source and as very likely, but not guaranteed, downstream.
//
// # Concurrency
//
// Neither a [Rand] nor a [Gen] is safe for concurrent use: a Gen may cache state
// between calls (the normal generator keeps the second of each pair of polar
// deviates), and a Rand advances its LCG state on every draw. Give each
// goroutine its own [Source]. The package-level functions are backed by a single
// shared Rand and inherit the same restriction, even though math/rand/v2's own
// functions are goroutine-safe.
//
// # Errors
//
// d3 throws a RangeError for out-of-domain parameters (a negative shape, a
// probability outside [0, 1]). A [Gen] has nowhere to return an error, so this
// package panics instead, at construction time rather than on the first draw.
// These are programmer errors — a parameter computed from data should be
// validated before it reaches here.
package random

import (
	"math"
	"math/rand/v2"
)

// Gen produces one draw from a distribution per call.
//
// A Gen usually closes over precomputed constants and over the [Rand] it was
// created from, and may carry state of its own; see the package doc on
// concurrency.
type Gen func() float64

// Rand is a source of randomness that distributions draw from — the equivalent
// of d3's .source() on every random function.
//
// Get a reproducible one from [Source], an arbitrary one from [SourceFunc], or
// use the package-level distribution functions for the shared default source.
type Rand struct {
	// next yields a uniform deviate in [0, 1).
	next func() float64

	// d3 composes its distributions as modules: gamma holds a single standard
	// normal generator shared by every gamma it creates, beta instantiates its
	// own copy of the gamma module, binomial its own copy of beta, and poisson
	// its own copies of gamma and binomial. Because the normal generator caches
	// one deviate between calls, *which* copy a draw comes from changes the
	// order in which the uniform stream is consumed — so reproducing d3's
	// numbers means reproducing that module graph. These fields are those
	// copies, created lazily since an unused branch of the graph consumes
	// nothing.
	gammaNormal        Gen
	betaNormal         Gen
	binomialNormal     Gen
	poissonGammaNormal Gen
	poissonBetaNormal  Gen
}

// LCG parameters, from d3's randomLcg (Numerical Recipes' "quick and dirty"
// choice, also used by glibc's mixed generator).
const (
	lcgMul = 0x19660D
	lcgInc = 0x3C6EF35F
	// two32 is 2^32; dividing the 32-bit state by it maps to [0, 1).
	two32 = 4294967296
)

// Source returns a reproducible [Rand] implementing d3's randomLcg with the
// given seed.
//
// Seeds are interpreted exactly as d3 does, which is idiosyncratic enough to
// spell out: a seed in [0, 1) is scaled by 2^32 (so fractional seeds spread over
// the whole state space, letting math.Float64 output be used directly as a
// seed), and any other seed has its absolute value taken. Either way the result
// is truncated toward zero and reduced modulo 2^32, matching JavaScript's "| 0".
// A NaN or infinite seed yields state 0, again as in JavaScript.
//
// The generator has a period of 2^32 and is not remotely cryptographic; its
// low-order bits are weak, as in every LCG of this form. Use it for
// reproducibility, not for security or for anything that depends on
// high-dimensional equidistribution.
func Source(seed float64) *Rand {
	var state uint32
	if seed >= 0 && seed < 1 {
		state = toInt32(seed * two32)
	} else {
		state = toInt32(math.Abs(seed))
	}
	return &Rand{next: func() float64 {
		state = state*lcgMul + lcgInc
		return float64(state) / two32
	}}
}

// toInt32 reproduces JavaScript's ToInt32 (the "x | 0" idiom): truncate toward
// zero, reduce modulo 2^32, and keep the low 32 bits. Non-finite inputs map to
// zero. Only the bit pattern matters downstream, so the signed/unsigned
// distinction JavaScript makes here does not.
func toInt32(x float64) uint32 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return 0
	}
	m := math.Mod(math.Trunc(x), two32)
	if m < 0 {
		m += two32
	}
	return uint32(m)
}

// SourceFunc wraps an arbitrary uniform generator as a [Rand], the analogue of
// passing your own function to d3's .source(). next must return values in
// [0, 1); several distributions rely on that range and will produce nonsense
// (or diverge) if it is violated.
//
// Use this to drive the distributions from math/rand/v2's [rand.Rand], from a
// cryptographic source, or from a fixed sequence in a test.
func SourceFunc(next func() float64) *Rand {
	return &Rand{next: next}
}

// defaultRand is the source behind the package-level distribution functions. It
// uses math/rand/v2's global generator, which is seeded unpredictably at startup
// — so package-level draws are not reproducible by design. Reach for [Source]
// when they need to be.
var defaultRand = &Rand{next: rand.Float64}

// Float64 returns the next raw uniform deviate in [0, 1) from this source.
func (r *Rand) Float64() float64 { return r.next() }

// Float64 returns a uniform deviate in [0, 1) from the default source.
func Float64() float64 { return defaultRand.next() }

// stdNormal lazily creates the standard normal generator held in one of the
// module slots described on [Rand].
func (r *Rand) stdNormal(slot *Gen) Gen {
	if *slot == nil {
		*slot = r.Normal(0, 1)
	}
	return *slot
}

// Uniform returns a generator of numbers uniformly distributed in [min, max).
func (r *Rand) Uniform(min, max float64) Gen {
	span := max - min
	return func() float64 { return r.next()*span + min }
}

// Normal returns a generator of normally distributed numbers with mean mu and
// standard deviation sigma.
//
// The algorithm is Marsaglia's polar method: sample points in the square
// [-1, 1]², reject those outside the unit circle, and transform the survivors.
// It produces two independent deviates at a time, so the generator returns one
// and caches the other for the next call — which is why a Gen from this
// function is stateful and must not be shared between goroutines.
func (r *Rand) Normal(mu, sigma float64) Gen {
	var x float64
	var haveX bool
	var rr float64
	return func() float64 {
		var y float64
		if haveX {
			y, haveX = x, false
		} else {
			for {
				x = r.next()*2 - 1
				y = r.next()*2 - 1
				rr = x*x + y*y
				if rr != 0 && rr <= 1 {
					break
				}
			}
			haveX = true
		}
		return mu + sigma*y*math.Sqrt(-2*math.Log(rr)/rr)
	}
}

// LogNormal returns a generator whose logarithm is normally distributed with
// mean mu and standard deviation sigma. Note that mu and sigma parameterise the
// underlying normal, not the log-normal itself: the resulting mean is
// exp(mu + sigma²/2).
func (r *Rand) LogNormal(mu, sigma float64) Gen {
	n := r.Normal(mu, sigma)
	return func() float64 { return math.Exp(n()) }
}

// IrwinHall returns a generator for the Irwin–Hall distribution: the sum of n
// independent uniform [0, 1) variables, with mean n/2 and variance n/12.
//
// A fractional n is allowed and is interpreted as d3 does — floor(n) whole
// uniforms plus the fractional remainder times one more — which interpolates
// smoothly between the integer cases. n <= 0 gives a generator that always
// returns 0.
func (r *Rand) IrwinHall(n float64) Gen {
	if n <= 0 {
		return func() float64 { return 0 }
	}
	return func() float64 {
		sum := 0.0
		i := n
		for ; i > 1; i-- {
			sum += r.next()
		}
		return sum + i*r.next()
	}
}

// Bates returns a generator for the Bates distribution: the *mean* of n
// independent uniform [0, 1) variables, so mean 1/2 and variance 1/(12n). It is
// [Rand.IrwinHall] divided by n, and converges on a normal as n grows.
//
// n == 0 falls back to the raw uniform source, matching d3's use of the
// limiting distribution there.
func (r *Rand) Bates(n float64) Gen {
	if n == 0 {
		return r.next
	}
	ih := r.IrwinHall(n)
	return func() float64 { return ih() / n }
}

// Exponential returns a generator for the exponential distribution with rate
// lambda: mean and standard deviation both 1/lambda. It is the waiting time
// between events in a Poisson process of that rate.
func (r *Rand) Exponential(lambda float64) Gen {
	return func() float64 { return -math.Log1p(-r.next()) / lambda }
}

// Pareto returns a generator for the Pareto distribution with shape alpha,
// supported on [1, ∞). The mean is finite only for alpha > 1 and the variance
// only for alpha > 2; below those thresholds the sample statistics of a large
// draw simply do not converge, which is the point of the distribution rather
// than a defect.
//
// Panics if alpha is negative.
func (r *Rand) Pareto(alpha float64) Gen {
	if alpha < 0 {
		panic("random: Pareto: invalid alpha")
	}
	e := 1 / -alpha
	return func() float64 { return math.Pow(1-r.next(), e) }
}

// Bernoulli returns a generator yielding 1 with probability p and 0 otherwise.
// Panics unless p is in [0, 1].
func (r *Rand) Bernoulli(p float64) Gen {
	if p < 0 || p > 1 {
		panic("random: Bernoulli: invalid p")
	}
	return func() float64 { return math.Floor(r.next() + p) }
}

// Geometric returns a generator for the number of Bernoulli trials up to and
// including the first success, so the support is {1, 2, 3, …} and the mean is
// 1/p.
//
// p == 0 gives a generator that always returns +Inf (the success never comes),
// and p == 1 one that always returns 1. Panics unless p is in [0, 1].
func (r *Rand) Geometric(p float64) Gen {
	if p < 0 || p > 1 {
		panic("random: Geometric: invalid p")
	}
	switch p {
	case 0:
		return func() float64 { return math.Inf(1) }
	case 1:
		return func() float64 { return 1 }
	}
	lp := math.Log1p(-p)
	return func() float64 { return 1 + math.Floor(math.Log1p(-r.next())/lp) }
}

// Gamma returns a generator for the gamma distribution with shape k and scale
// theta: mean k·theta and variance k·theta². Pass theta = 1 for the standard
// gamma (d3 defaults it; Go has no optional arguments).
//
// The implementation is Marsaglia and Tsang's squeeze method, which draws a
// normal deviate and a uniform and accepts on a cheap polynomial test that
// almost always short-circuits the logarithms. Shapes below 1 are handled by
// sampling at k+1 and applying the standard power correction, since the density
// is unbounded at zero there.
//
// k == 0 degenerates to a constant 0 and k == 1 to an exponential, both of which
// are special-cased. Panics if k is negative.
func (r *Rand) Gamma(k, theta float64) Gen {
	return r.gammaWith(r.stdNormal(&r.gammaNormal), k, theta)
}

func (r *Rand) gammaWith(norm Gen, k, theta float64) Gen {
	if k < 0 {
		panic("random: Gamma: invalid k")
	}
	if k == 0 {
		return func() float64 { return 0 }
	}
	if k == 1 {
		return func() float64 { return -math.Log1p(-r.next()) * theta }
	}

	base := k
	if k < 1 {
		base = k + 1
	}
	d := base - 1.0/3
	c := 1 / (3 * math.Sqrt(d))

	return func() float64 {
		for {
			var x, v float64
			for {
				x = norm()
				v = 1 + c*x
				if v > 0 {
					break
				}
			}
			v *= v * v
			u := 1 - r.next()
			// The first test is the cheap squeeze; only when it fails is the
			// exact log-density comparison needed.
			if u < 1-0.0331*x*x*x*x || math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
				m := 1.0
				if k < 1 {
					m = math.Pow(r.next(), 1/k)
				}
				return d * v * m * theta
			}
		}
	}
}

// Beta returns a generator for the beta distribution with shape parameters
// alpha and beta, supported on [0, 1] with mean alpha/(alpha+beta).
//
// It is built from two gamma draws, X ~ Γ(alpha) and Y ~ Γ(beta), returning
// X/(X+Y) — the standard construction, and the one d3 uses.
func (r *Rand) Beta(alpha, beta float64) Gen {
	return r.betaWith(r.stdNormal(&r.betaNormal), alpha, beta)
}

func (r *Rand) betaWith(norm Gen, alpha, beta float64) Gen {
	x := r.gammaWith(norm, alpha, 1)
	y := r.gammaWith(norm, beta, 1)
	return func() float64 {
		xv := x()
		if xv == 0 {
			return 0
		}
		return xv / (xv + y())
	}
}

// Binomial returns a generator for the number of successes in n independent
// trials each succeeding with probability p: mean n·p and variance n·p·(1-p).
//
// Naively summing n Bernoulli trials would cost O(n) uniforms per draw, so d3
// uses the standard recursive decomposition instead: while the distribution is
// large enough in both tails, draw a beta deviate to split the problem in two
// and recurse on the smaller half; finish the remainder by summing geometric
// waiting times. Cost is roughly logarithmic in n.
//
// p >= 1 degenerates to a constant n and p <= 0 to a constant 0.
func (r *Rand) Binomial(n, p float64) Gen {
	return r.binomialWith(r.stdNormal(&r.binomialNormal), n, p)
}

func (r *Rand) binomialWith(norm Gen, n, p float64) Gen {
	if p >= 1 {
		return func() float64 { return n }
	}
	if p <= 0 {
		return func() float64 { return 0 }
	}
	return func() float64 {
		acc, nn, pp := 0.0, n, p
		for nn*pp > 16 && nn*(1-pp) > 16 {
			i := math.Floor((nn + 1) * pp)
			y := r.betaWith(norm, i, nn-i+1)()
			if y <= pp {
				acc += i
				nn -= i
				pp = (pp - y) / (1 - y)
			} else {
				nn = i - 1
				pp /= y
			}
		}
		// Sum geometric waiting times over the remaining trials, working with
		// whichever of pp and 1-pp is smaller so the loop is short.
		useSmall := pp < 0.5
		pFinal := pp
		if !useSmall {
			pFinal = 1 - pp
		}
		g := r.Geometric(pFinal)
		s, k := g(), 0.0
		for s <= nn {
			s += g()
			k++
		}
		if useSmall {
			return acc + k
		}
		return acc + nn - k
	}
}

// Poisson returns a generator for the number of events in a unit interval of a
// Poisson process with rate lambda: mean and variance both lambda.
//
// For small lambda this counts exponential waiting times until they exceed the
// interval (Knuth's method). For lambda above 16 that loop would be long, so d3
// first draws a gamma deviate to consume a large block of the interval at once,
// falling back to a binomial when the block overshoots — the standard
// gamma/binomial reduction.
func (r *Rand) Poisson(lambda float64) Gen {
	gNorm := r.stdNormal(&r.poissonGammaNormal)
	bNorm := r.stdNormal(&r.poissonBetaNormal)
	return func() float64 {
		acc, l := 0.0, lambda
		for l > 16 {
			n := math.Floor(0.875 * l)
			t := r.gammaWith(gNorm, n, 1)()
			if t > l {
				return acc + r.binomialWith(bNorm, n-1, l/t)()
			}
			acc += n
			l -= t
		}
		s, k := -math.Log1p(-r.next()), 0.0
		for s <= l {
			s -= math.Log1p(-r.next())
			k++
		}
		return acc + k
	}
}

// Weibull returns a generator for the Weibull distribution with shape k,
// location a and scale b. Pass a = 0 and b = 1 for the standard form (d3
// defaults them).
//
// The shape selects a family, and the two degenerate cases are useful rather
// than accidental: k > 0 gives the Weibull proper, k == 0 the Gumbel
// (extreme-value) distribution, and k < 0 the Fréchet. All three are inverse-CDF
// transforms of a single uniform, so each draw costs one.
func (r *Rand) Weibull(k, a, b float64) Gen {
	var outer func(float64) float64
	if k == 0 {
		outer = func(x float64) float64 { return -math.Log(x) }
	} else {
		inv := 1 / k
		outer = func(x float64) float64 { return math.Pow(x, inv) }
	}
	return func() float64 { return a + b*outer(-math.Log1p(-r.next())) }
}

// Cauchy returns a generator for the Cauchy distribution with location a and
// scale b.
//
// The Cauchy has no mean and no variance — the defining integrals diverge — so
// the sample mean of a large draw does not converge on anything and testing it
// that way is meaningless. Its median is a and its quartiles are a ± b; those
// are what this package's tests check.
func (r *Rand) Cauchy(a, b float64) Gen {
	return func() float64 { return a + b*math.Tan(math.Pi*r.next()) }
}

// Logistic returns a generator for the logistic distribution with location a and
// scale b: mean a and variance (b·π)²/3. It resembles a normal with heavier
// tails, and is the distribution whose CDF is the logistic (sigmoid) function.
func (r *Rand) Logistic(a, b float64) Gen {
	return func() float64 {
		u := r.next()
		return a + b*math.Log(u/(1-u))
	}
}

// Package-level generators drawing from the default (non-reproducible) source.
// Each is the corresponding [Rand] method; see there for the documentation.

// Uniform returns a generator of numbers uniformly distributed in [min, max).
func Uniform(min, max float64) Gen { return defaultRand.Uniform(min, max) }

// Normal returns a generator of normally distributed numbers. See [Rand.Normal].
func Normal(mu, sigma float64) Gen { return defaultRand.Normal(mu, sigma) }

// LogNormal returns a log-normal generator. See [Rand.LogNormal].
func LogNormal(mu, sigma float64) Gen { return defaultRand.LogNormal(mu, sigma) }

// IrwinHall returns an Irwin–Hall generator. See [Rand.IrwinHall].
func IrwinHall(n float64) Gen { return defaultRand.IrwinHall(n) }

// Bates returns a Bates generator. See [Rand.Bates].
func Bates(n float64) Gen { return defaultRand.Bates(n) }

// Exponential returns an exponential generator. See [Rand.Exponential].
func Exponential(lambda float64) Gen { return defaultRand.Exponential(lambda) }

// Pareto returns a Pareto generator. See [Rand.Pareto].
func Pareto(alpha float64) Gen { return defaultRand.Pareto(alpha) }

// Bernoulli returns a Bernoulli generator. See [Rand.Bernoulli].
func Bernoulli(p float64) Gen { return defaultRand.Bernoulli(p) }

// Geometric returns a geometric generator. See [Rand.Geometric].
func Geometric(p float64) Gen { return defaultRand.Geometric(p) }

// Binomial returns a binomial generator. See [Rand.Binomial].
func Binomial(n, p float64) Gen { return defaultRand.Binomial(n, p) }

// Gamma returns a gamma generator. See [Rand.Gamma].
func Gamma(k, theta float64) Gen { return defaultRand.Gamma(k, theta) }

// Beta returns a beta generator. See [Rand.Beta].
func Beta(alpha, beta float64) Gen { return defaultRand.Beta(alpha, beta) }

// Weibull returns a Weibull/Gumbel/Fréchet generator. See [Rand.Weibull].
func Weibull(k, a, b float64) Gen { return defaultRand.Weibull(k, a, b) }

// Cauchy returns a Cauchy generator. See [Rand.Cauchy].
func Cauchy(a, b float64) Gen { return defaultRand.Cauchy(a, b) }

// Logistic returns a logistic generator. See [Rand.Logistic].
func Logistic(a, b float64) Gen { return defaultRand.Logistic(a, b) }

// Poisson returns a Poisson generator. See [Rand.Poisson].
func Poisson(lambda float64) Gen { return defaultRand.Poisson(lambda) }
