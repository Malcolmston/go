# ease — Go port of d3-ease — the easing functions that shape how a value moves from its start

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/ease.svg)](https://pkg.go.dev/github.com/malcolmston/d3/ease)

Package ease is a Go port of d3-ease — the easing functions that shape how a
value moves from its start to its end over the course of an animation.

An easing is nothing more than a reparameterisation of time: a function that
takes normalized time t in [0, 1] and returns normalized progress, also
nominally in [0, 1]. Interpolation asks "what value is 40% of the way from a
to b?"; easing asks "how far along are we when 40% of the wall-clock time has
passed?". Compose the two and you get motion with character — a transition
that accelerates out of rest (`CubicIn`), settles gently into place
(`CubicOut`), or overshoots and springs back (`ElasticOut`):

```go
p := ease.CubicInOut(elapsed / duration)
x := x0 + (x1-x0)*p
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
import "github.com/malcolmston/d3/ease"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func BackIn(t float64) float64` | BackIn is `BackInWith` at `DefaultOvershoot`. |
| `func BackInOut(t float64) float64` | BackInOut is `BackInOutWith` at `DefaultOvershoot`. |
| `func BackOut(t float64) float64` | BackOut is `BackOutWith` at `DefaultOvershoot`. |
| `func BounceIn(t float64) float64` | BounceIn is the time-reverse of `BounceOut`: the bounces happen at the start. |
| `func BounceInOut(t float64) float64` | BounceInOut bounces at both ends, symmetric about t = 0.5. |
| `func BounceOut(t float64) float64` | BounceOut eases out with a bouncing motion, as if the value were a ball dropped onto the endpoint: it arrives, rebounds, and arrives again with… |
| `func CircleIn(t float64) float64` | CircleIn eases in along the arc of a circle: gentle at first, with a vertical tangent at t = 1. |
| `func CircleInOut(t float64) float64` | CircleInOut eases in and out along circular arcs, symmetric about t = 0.5. |
| `func CircleOut(t float64) float64` | CircleOut eases out along the arc of a circle, the mirror of `CircleIn`. |
| `func CubicIn(t float64) float64` | CubicIn accelerates from rest: t³. |
| `func CubicInOut(t float64) float64` | CubicInOut accelerates then decelerates, symmetric about t = 0.5. |
| `func CubicOut(t float64) float64` | CubicOut decelerates to rest: the mirror of `CubicIn`. |
| `func ElasticIn(t float64) float64` | ElasticIn is `ElasticInWith` at `DefaultAmplitude` and `DefaultPeriod`. |
| `func ElasticInOut(t float64) float64` | ElasticInOut is `ElasticInOutWith` at `DefaultAmplitude` and `DefaultPeriod`. |
| `func ElasticOut(t float64) float64` | ElasticOut is `ElasticOutWith` at `DefaultAmplitude` and `DefaultPeriod`. |
| `func ExpIn(t float64) float64` | ExpIn eases in exponentially: very slow at first, then very fast. |
| `func ExpInOut(t float64) float64` | ExpInOut eases in and out exponentially, symmetric about t = 0.5. |
| `func ExpOut(t float64) float64` | ExpOut eases out exponentially, the mirror of `ExpIn`. |
| `func Linear(t float64) float64` | Linear is the identity easing: progress equals elapsed time. |
| `func PolyIn(t float64) float64` | PolyIn is `PolyInWith` at `DefaultExponent`, and so agrees with `CubicIn`. |
| `func PolyInOut(t float64) float64` | PolyInOut is `PolyInOutWith` at `DefaultExponent`. |
| `func PolyOut(t float64) float64` | PolyOut is `PolyOutWith` at `DefaultExponent`, and so agrees with `CubicOut`. |
| `func QuadIn(t float64) float64` | QuadIn accelerates from rest: t². |
| `func QuadInOut(t float64) float64` | QuadInOut accelerates then decelerates, symmetric about t = 0.5. |
| `func QuadOut(t float64) float64` | QuadOut decelerates to rest: the mirror of `QuadIn`. |
| `func SinIn(t float64) float64` | SinIn eases in along a quarter of a cosine wave. |
| `func SinInOut(t float64) float64` | SinInOut eases in and out along half a cosine wave. |
| `func SinOut(t float64) float64` | SinOut eases out along a quarter of a sine wave. |

### Types

| Type | What it is |
| --- | --- |
| `Ease` | Ease is an easing function: it maps normalized time to normalized progress. |

<details>
<summary><code>Ease</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func BackInOutWith(overshoot float64) Ease` | BackInOutWith returns a back ease-in-out, anticipating at the start and overshooting at the end. |
| `func BackInWith(overshoot float64) Ease` | BackInWith returns a back ease-in with the given overshoot: motion pulls back below zero before moving forward, the "anticipation" of classical… |
| `func BackOutWith(overshoot float64) Ease` | BackOutWith returns a back ease-out: motion overshoots the endpoint and settles back onto it, the "follow-through" of classical animation. |
| `func ElasticInOutWith(amplitude, period float64) Ease` | ElasticInOutWith returns an elastic ease-in-out, oscillating at both ends. |
| `func ElasticInWith(amplitude, period float64) Ease` | ElasticInWith returns an elastic ease-in with the given amplitude and period: an oscillation that grows until it snaps into the endpoint. |
| `func ElasticOutWith(amplitude, period float64) Ease` | ElasticOutWith returns an elastic ease-out: the value overshoots the endpoint and rings down onto it. |
| `func PolyInOutWith(exponent float64) Ease` | PolyInOutWith returns the symmetric polynomial ease-in-out. |
| `func PolyInWith(exponent float64) Ease` | PolyInWith returns the polynomial ease-in t^exponent. |
| `func PolyOutWith(exponent float64) Ease` | PolyOutWith returns the polynomial ease-out, the mirror of `PolyInWith`. |
| `func (e Ease) Clamped() Ease` | Clamped returns an easing that clamps its input to [0, 1] before applying e. |

</details>

### Constants

`DefaultExponent`, `DefaultOvershoot`, `DefaultAmplitude`, `DefaultPeriod`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/ease).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
