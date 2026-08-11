# random — Go port of d3-random — generators for numbers drawn from the distributions that turn

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/random.svg)](https://pkg.go.dev/github.com/malcolmston/d3/random)

Package random is a Go port of d3-random — generators for numbers drawn from
the distributions that turn up in data visualization: normal and log-normal,
exponential and Poisson, gamma and beta, and a dozen more.

The shape of the API follows d3's. A distribution function does not return a
number; it returns a `Gen`, a zero-argument closure that yields a fresh draw
each time it is called. Constructing the generator once lets it hoist the
per-distribution setup — the shape constants of a gamma, the log of a
geometric's success probability — out of the hot loop:

```go
next := random.Normal(0, 1)
for i := range xs {
	xs[i] = next()
}
```

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/random"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func Float64() float64` | Float64 returns a uniform deviate in [0, 1) from the default source. |

### Types

| Type | What it is |
| --- | --- |
| `Gen` | Gen produces one draw from a distribution per call. |
| `Rand` | Rand is a source of randomness that distributions draw from — the equivalent of d3's .source() on every random function. |

<details>
<summary><code>Gen</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func Bates(n float64) Gen` | Bates returns a Bates generator. |
| `func Bernoulli(p float64) Gen` | Bernoulli returns a Bernoulli generator. |
| `func Beta(alpha, beta float64) Gen` | Beta returns a beta generator. |
| `func Binomial(n, p float64) Gen` | Binomial returns a binomial generator. |
| `func Cauchy(a, b float64) Gen` | Cauchy returns a Cauchy generator. |
| `func Exponential(lambda float64) Gen` | Exponential returns an exponential generator. |
| `func Gamma(k, theta float64) Gen` | Gamma returns a gamma generator. |
| `func Geometric(p float64) Gen` | Geometric returns a geometric generator. |
| `func IrwinHall(n float64) Gen` | IrwinHall returns an Irwin–Hall generator. |
| `func LogNormal(mu, sigma float64) Gen` | LogNormal returns a log-normal generator. |
| `func Logistic(a, b float64) Gen` | Logistic returns a logistic generator. |
| `func Normal(mu, sigma float64) Gen` | Normal returns a generator of normally distributed numbers. |
| `func Pareto(alpha float64) Gen` | Pareto returns a Pareto generator. |
| `func Poisson(lambda float64) Gen` | Poisson returns a Poisson generator. |
| `func Uniform(min, max float64) Gen` | Uniform returns a generator of numbers uniformly distributed in [min, max). |
| `func Weibull(k, a, b float64) Gen` | Weibull returns a Weibull/Gumbel/Fréchet generator. |

</details>

<details>
<summary><code>Rand</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func Source(seed float64) *Rand` | Source returns a reproducible `Rand` implementing d3's randomLcg with the given seed. |
| `func SourceFunc(next func() float64) *Rand` | SourceFunc wraps an arbitrary uniform generator as a `Rand`, the analogue of passing your own function to d3's .source(). |
| `func (r *Rand) Bates(n float64) Gen` | Bates returns a generator for the Bates distribution: the *mean* of n independent uniform [0, 1) variables, so mean 1/2 and variance 1/(12n). |
| `func (r *Rand) Bernoulli(p float64) Gen` | Bernoulli returns a generator yielding 1 with probability p and 0 otherwise. |
| `func (r *Rand) Beta(alpha, beta float64) Gen` | Beta returns a generator for the beta distribution with shape parameters alpha and beta, supported on [0, 1] with mean alpha/(alpha+beta). |
| `func (r *Rand) Binomial(n, p float64) Gen` | Binomial returns a generator for the number of successes in n independent trials each succeeding with probability p: mean n·p and variance… |
| `func (r *Rand) Cauchy(a, b float64) Gen` | Cauchy returns a generator for the Cauchy distribution with location a and scale b. |
| `func (r *Rand) Exponential(lambda float64) Gen` | Exponential returns a generator for the exponential distribution with rate lambda: mean and standard deviation both 1/lambda. |
| `func (r *Rand) Float64() float64` | Float64 returns the next raw uniform deviate in [0, 1) from this source. |
| `func (r *Rand) Gamma(k, theta float64) Gen` | Gamma returns a generator for the gamma distribution with shape k and scale theta: mean k·theta and variance k·theta². |
| `func (r *Rand) Geometric(p float64) Gen` | Geometric returns a generator for the number of Bernoulli trials up to and including the first success, so the support is {1, 2, 3, …} and the mean… |
| `func (r *Rand) IrwinHall(n float64) Gen` | IrwinHall returns a generator for the Irwin–Hall distribution: the sum of n independent uniform [0, 1) variables, with mean n/2 and variance n/12. |
| `func (r *Rand) LogNormal(mu, sigma float64) Gen` | LogNormal returns a generator whose logarithm is normally distributed with mean mu and standard deviation sigma. |
| `func (r *Rand) Logistic(a, b float64) Gen` | Logistic returns a generator for the logistic distribution with location a and scale b: mean a and variance (b·π)²/3. |
| `func (r *Rand) Normal(mu, sigma float64) Gen` | Normal returns a generator of normally distributed numbers with mean mu and standard deviation sigma. |
| `func (r *Rand) Pareto(alpha float64) Gen` | Pareto returns a generator for the Pareto distribution with shape alpha, supported on [1, ∞). |
| `func (r *Rand) Poisson(lambda float64) Gen` | Poisson returns a generator for the number of events in a unit interval of a Poisson process with rate lambda: mean and variance both lambda. |
| `func (r *Rand) Uniform(min, max float64) Gen` | Uniform returns a generator of numbers uniformly distributed in [min, max). |
| `func (r *Rand) Weibull(k, a, b float64) Gen` | Weibull returns a generator for the Weibull distribution with shape k, location a and scale b. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/random).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
