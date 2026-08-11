# timer — Go port of d3-timer — with one large, deliberate change of design

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/timer.svg)](https://pkg.go.dev/github.com/malcolmston/d3/timer)

Package timer is a Go port of d3-timer — with one large, deliberate change
of design, described below before anything else, because it changes what the
package is for.

d3-timer is a scheduler built on requestAnimationFrame. Its whole value in the
browser comes from being synchronized to the display: every timer's callback
runs once per frame, all of them see the same timestamp, and the browser
decides when a frame happens. None of that exists here. There is no display,
no compositor, and nothing that can tell this package when a frame would have
been.

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/timer"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func FlushTimers()` | FlushTimers runs every due timer on the default clock immediately, without waiting for the next poll. |
| `func Now() float64` | Now returns the current time of the default clock, in milliseconds since the process started. |

### Types

| Type | What it is |
| --- | --- |
| `Callback` | Callback is a timer's callback. |
| `Clock` | Clock is a set of timers sharing one notion of the current time. |
| `Timer` | Timer is one scheduled callback. |

<details>
<summary><code>Clock</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func DefaultClock() *Clock` | DefaultClock returns the wall clock the package-level functions use. |
| `func NewClock() *Clock` | NewClock returns a manual clock at time zero. |
| `func NewWallClock() *Clock` | NewWallClock returns a clock whose time is milliseconds since it was created, read from time.Now. |
| `func (c *Clock) Active() int` | Active reports how many timers are scheduled. |
| `func (c *Clock) Advance(ms float64)` | Advance moves a manual clock forward by ms milliseconds and then runs every timer that has become due. |
| `func (c *Clock) AdvanceTo(ms float64)` | AdvanceTo moves a manual clock to an absolute time and runs every timer that has become due. |
| `func (c *Clock) Flush()` | Flush runs every timer that is due at the clock's current time, without changing that time. |
| `func (c *Clock) Interval(cb Callback, delay float64) *Timer` | Interval schedules cb to run every delay milliseconds, rather than on every pass of the clock. |
| `func (c *Clock) Now() float64` | Now returns the clock's current time in milliseconds since its origin. |
| `func (c *Clock) Run(ctx context.Context, interval time.Duration) error` | Run drives a wall clock from a time.Ticker until the context is cancelled or no timers remain, and returns the reason: ctx.Err(), or nil for "nothing… |
| `func (c *Clock) StopAll()` | StopAll stops every timer on the clock. |
| `func (c *Clock) Sync()` | Sync runs one pass over a wall clock at the current time. |
| `func (c *Clock) Timeout(cb Callback, delay float64) *Timer` | Timeout schedules cb to run once, delay milliseconds from now, and then stop itself. |
| `func (c *Clock) Timer(cb Callback, delay float64) *Timer` | Timer schedules cb to run on every pass of the clock, starting delay milliseconds from now. |
| `func (c *Clock) TimerAt(cb Callback, delay, at float64) *Timer` | TimerAt schedules cb relative to an explicit clock time rather than to now. |

</details>

<details>
<summary><code>Timer</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func Interval(cb Callback, delay float64) *Timer` | Interval schedules cb to run every delay milliseconds on the default clock. |
| `func New(cb Callback, delay float64) *Timer` | New schedules cb on the default clock, starting delay milliseconds from now, and starts the background poll if it is not already running. |
| `func NewAt(cb Callback, delay, at float64) *Timer` | NewAt is `New` relative to an explicit clock time. |
| `func Timeout(cb Callback, delay float64) *Timer` | Timeout schedules cb to run once on the default clock. |
| `func (t *Timer) Restart(cb Callback, delay float64)` | Restart reschedules the timer with a new callback and delay, measured from the clock's current time. |
| `func (t *Timer) RestartAt(cb Callback, delay, at float64)` | RestartAt reschedules the timer relative to an explicit clock time rather than to now — d3's third argument to timer.restart. |
| `func (t *Timer) Stop()` | Stop cancels the timer. |
| `func (t *Timer) Stopped() bool` | Stopped reports whether the timer has been cancelled. |

</details>

### Constants

`DefaultInterval`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/timer).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
