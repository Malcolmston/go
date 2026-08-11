// Package timer is a Go port of d3-timer — with one large, deliberate change
// of design, described below before anything else, because it changes what the
// package is for.
//
// # There is no frame loop, so this package does not pretend there is one
//
// d3-timer is a scheduler built on requestAnimationFrame. Its whole value in
// the browser comes from being synchronized to the display: every timer's
// callback runs once per frame, all of them see the same timestamp, and the
// browser decides when a frame happens. None of that exists here. There is no
// display, no compositor, and nothing that can tell this package when a frame
// would have been.
//
// Shipping a requestAnimationFrame imitation on top of a 16-millisecond ticker
// would be a lie in the shape of an API: it would promise a frame budget it
// cannot observe and cannot meet, and it would drift under load with no way to
// notice. So this port keeps the parts of d3-timer that are real without a
// browser and is explicit about the part that is not.
//
// What survives, and why it is worth having:
//
//   - **A shared clock.** Every timer on a [Clock] sees exactly the same
//     "now" for a given pass. That is the property that makes several animated
//     quantities stay in step, and it is independent of where time comes from.
//   - **d3's elapsed-time contract.** A callback receives the milliseconds
//     since it became due, not since it was created, so easing code ported
//     from d3 keeps working unchanged.
//   - **Deterministic stepping.** A [Clock] can be advanced by hand.
//
// That last one is the reason this package earns its place. A server-rendered
// animation, an offline frame dump, a [github.com/malcolmston/d3/ease] curve
// sampled at fixed intervals, or a force simulation run to convergence all
// want the *same* frames every time, as fast as the CPU will produce them —
// not whatever the wall clock happened to deliver. Under a manual clock this
// package is a pure function of the step sequence, so a test asserts exact
// values and a render is reproducible byte for byte.
//
// Real time is still available, for the case where a program genuinely is
// animating against a wall clock, but it is opt-in and honest about being a
// poll rather than a frame.
//
// # Two kinds of clock
//
// A manual clock advances only when you say so:
//
//	c := timer.NewClock()
//	c.Timer(func(elapsed float64) { fmt.Println(elapsed) }, 0)
//	c.Advance(16) // prints 16
//	c.Advance(16) // prints 32
//
// A wall clock reads time.Now, and is driven either by calling
// [Clock.Sync] from your own loop or by [Clock.Run], which polls on a
// [time.Ticker]:
//
//	c := timer.NewWallClock()
//	c.Timeout(func(float64) { fmt.Println("done") }, 250)
//	c.Run(ctx, timer.DefaultInterval) // returns when no timers are left
//
// The package-level [New], [Timeout], [Interval], [Now] and [FlushTimers]
// operate on a default wall clock that starts polling on demand and stops
// again once no timers remain — the closest honest analogue of d3's global
// timer queue.
//
// # Names
//
//	d3.timer(cb, delay, time)  → timer.New(cb, delay) / timer.NewAt(cb, delay, at)
//	d3.timeout(cb, delay)      → timer.Timeout(cb, delay)
//	d3.interval(cb, delay)     → timer.Interval(cb, delay)
//	d3.now()                   → timer.Now()
//	d3.timerFlush()            → timer.FlushTimers()
//	timer.restart(cb, delay)   → (*Timer).Restart(cb, delay)
//	timer.stop()               → (*Timer).Stop()
//
// d3.timer becomes New because Timer is the type here; Go cannot have both.
//
// # Goroutine safety
//
// Creating, restarting and stopping timers, and reading [Clock.Now] or
// [Clock.Active], are safe from any goroutine at any time.
//
// Callbacks run on whichever goroutine advanced the clock — the caller of
// [Clock.Advance] or [Clock.Sync], or the goroutine running [Clock.Run] — and
// they run without the clock's lock held, so a callback may freely create,
// restart or stop timers, including its own. Timers created during a pass do
// not run in that pass.
//
// One clock should have **one driver**: exactly one goroutine calling
// Advance, AdvanceTo, Sync, Flush or Run at a time. That is not a locking
// limitation but a semantic one — two goroutines advancing the same clock
// would run the same callback concurrently, and a callback is written on the
// assumption that it owns the state it is animating. Many clocks may of
// course run at once, each with its own driver.
package timer

import (
	"context"
	"sync"
	"time"
)

// DefaultInterval is the poll period [Clock.Run] and the default clock use
// when none is given.
//
// It is 16 milliseconds because that is roughly a 60 Hz frame, which is the
// rate an animation is usually authored against. It is **not** a frame budget
// and nothing is synchronized to a display: it is how often the wall clock is
// read. Callbacks that overrun it delay the next poll rather than dropping a
// frame, because there is no frame to drop.
const DefaultInterval = 16 * time.Millisecond

// Callback is a timer's callback. elapsed is the number of milliseconds
// between the time the timer became due and the clock's current time, so it
// starts at approximately 0 and grows — the value an easing curve is
// parameterized on.
type Callback func(elapsed float64)

// Timer is one scheduled callback. Create one through a [Clock] or through the
// package-level constructors; the zero value is not usable.
//
// A Timer is safe for concurrent use, and stopping an already-stopped timer is
// a no-op, so shutdown paths do not have to be careful.
type Timer struct {
	clock *Clock

	mu      sync.Mutex
	cb      Callback
	due     float64 // clock time at which the callback becomes due
	stopped bool
}

// Restart reschedules the timer with a new callback and delay, measured from
// the clock's current time. It is the way to reuse a timer rather than
// allocate another, and it works on a stopped timer.
func (t *Timer) Restart(cb Callback, delay float64) {
	t.RestartAt(cb, delay, t.clock.Now())
}

// RestartAt reschedules the timer relative to an explicit clock time rather
// than to now — d3's third argument to timer.restart. Passing a time in the
// past makes the timer immediately due, and its first elapsed value is
// correspondingly positive.
func (t *Timer) RestartAt(cb Callback, delay, at float64) {
	t.mu.Lock()
	t.cb = cb
	t.due = at + delay
	wasStopped := t.stopped
	t.stopped = false
	t.mu.Unlock()
	if wasStopped {
		t.clock.add(t)
	}
	t.clock.wake()
}

// Stop cancels the timer. A callback already running is not interrupted, and a
// timer that has been stopped can be revived with [Timer.Restart].
func (t *Timer) Stop() {
	t.mu.Lock()
	already := t.stopped
	t.stopped = true
	t.mu.Unlock()
	if !already {
		t.clock.remove(t)
	}
}

// Stopped reports whether the timer has been cancelled.
func (t *Timer) Stopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stopped
}

// Clock is a set of timers sharing one notion of the current time.
//
// Construct with [NewClock] for a manual clock or [NewWallClock] for one that
// reads time.Now; the zero value is not usable. A Clock is safe for concurrent
// use.
type Clock struct {
	mu     sync.Mutex
	wall   bool
	origin time.Time
	now    float64 // manual clocks only
	timers []*Timer

	// woken is closed and replaced whenever a timer is added or restarted, so
	// Run can react to a new short delay instead of waiting out its tick.
	woken chan struct{}
}

// NewClock returns a manual clock at time zero. Time advances only through
// [Clock.Advance] or [Clock.AdvanceTo]; nothing runs in the background.
//
// This is the clock to use for tests, for offline rendering, and for anything
// whose output has to be reproducible.
func NewClock() *Clock {
	return &Clock{woken: make(chan struct{})}
}

// NewWallClock returns a clock whose time is milliseconds since it was
// created, read from time.Now.
//
// Creating it starts nothing. Drive it with [Clock.Sync] from a loop you own,
// or with [Clock.Run].
func NewWallClock() *Clock {
	return &Clock{wall: true, origin: time.Now(), woken: make(chan struct{})}
}

// Now returns the clock's current time in milliseconds since its origin. For a
// wall clock that is read from time.Now on every call, so it is accurate
// between polls; for a manual clock it is whatever it was last advanced to.
func (c *Clock) Now() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nowLocked()
}

func (c *Clock) nowLocked() float64 {
	if c.wall {
		return float64(time.Since(c.origin).Nanoseconds()) / 1e6
	}
	return c.now
}

// Timer schedules cb to run on every pass of the clock, starting delay
// milliseconds from now. Pass a delay of 0 for "starting on the next pass".
func (c *Clock) Timer(cb Callback, delay float64) *Timer {
	return c.TimerAt(cb, delay, c.Now())
}

// TimerAt schedules cb relative to an explicit clock time rather than to now.
func (c *Clock) TimerAt(cb Callback, delay, at float64) *Timer {
	t := &Timer{clock: c, cb: cb, due: at + delay}
	c.add(t)
	c.wake()
	return t
}

// Timeout schedules cb to run once, delay milliseconds from now, and then stop
// itself.
//
// The elapsed value the callback receives includes the delay — so a timeout of
// 250ms reports about 250, not about 0. That is upstream's contract and it is
// the useful one: it tells you how late you actually are.
func (c *Clock) Timeout(cb Callback, delay float64) *Timer {
	// The timer is fully built before it is registered: once it is on the
	// clock a driver goroutine may call it immediately, so nothing about it
	// may still be being assigned.
	t := &Timer{clock: c, due: c.Now() + delay}
	t.cb = func(elapsed float64) {
		t.Stop()
		cb(elapsed + delay)
	}
	c.add(t)
	c.wake()
	return t
}

// Interval schedules cb to run every delay milliseconds, rather than on every
// pass of the clock.
//
// The schedule is anchored to the start time, not to when the previous
// callback returned, so an interval does not drift even if a callback is slow
// or the clock is advanced in uneven steps. A delay of zero or less degenerates
// to a plain [Clock.Timer], which is upstream's behavior.
func (c *Clock) Interval(cb Callback, delay float64) *Timer {
	if !(delay > 0) {
		return c.Timer(cb, delay)
	}
	start := c.Now()
	t := &Timer{clock: c}
	total := delay
	var tick Callback
	tick = func(elapsed float64) {
		elapsed += total
		total += delay
		t.RestartAt(tick, total, start)
		cb(elapsed)
	}
	t.cb = tick
	t.due = start + delay
	c.add(t)
	c.wake()
	return t
}

// Advance moves a manual clock forward by ms milliseconds and then runs every
// timer that has become due. It panics on a wall clock, whose time is not
// this package's to set.
func (c *Clock) Advance(ms float64) {
	c.mu.Lock()
	if c.wall {
		c.mu.Unlock()
		panic("timer: Advance called on a wall clock; drive it with Sync or Run instead")
	}
	c.now += ms
	c.mu.Unlock()
	c.Flush()
}

// AdvanceTo moves a manual clock to an absolute time and runs every timer that
// has become due. Moving backwards is allowed and simply makes fewer timers
// due; it does not un-run anything.
func (c *Clock) AdvanceTo(ms float64) {
	c.mu.Lock()
	if c.wall {
		c.mu.Unlock()
		panic("timer: AdvanceTo called on a wall clock; drive it with Sync or Run instead")
	}
	c.now = ms
	c.mu.Unlock()
	c.Flush()
}

// Sync runs one pass over a wall clock at the current time. Call it from your
// own loop when you do not want [Clock.Run]'s goroutine. On a manual clock it
// is the same as [Clock.Flush].
func (c *Clock) Sync() { c.Flush() }

// Flush runs every timer that is due at the clock's current time, without
// changing that time. It is d3.timerFlush, and its use is the same: to give
// timers created during this pass a chance to run before the first frame is
// drawn, so a zero-delay timer does not show a frame of its initial state.
//
// Callbacks run in creation order, on the calling goroutine, and without the
// clock's lock held.
func (c *Clock) Flush() {
	c.mu.Lock()
	now := c.nowLocked()
	// Snapshot: a timer created by a callback belongs to the next pass, and
	// the lock must not be held while a callback runs.
	pass := make([]*Timer, len(c.timers))
	copy(pass, c.timers)
	c.mu.Unlock()

	for _, t := range pass {
		t.mu.Lock()
		cb, due, stopped := t.cb, t.due, t.stopped
		t.mu.Unlock()
		if stopped || cb == nil {
			continue
		}
		if e := now - due; e >= 0 {
			cb(e)
		}
	}
}

// Active reports how many timers are scheduled. It reaches zero when every
// timer has been stopped, which is what [Clock.Run] waits for.
func (c *Clock) Active() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

// StopAll stops every timer on the clock. Use it to tear down a simulation
// without tracking each handle.
func (c *Clock) StopAll() {
	c.mu.Lock()
	pass := make([]*Timer, len(c.timers))
	copy(pass, c.timers)
	c.mu.Unlock()
	for _, t := range pass {
		t.Stop()
	}
}

// Run drives a wall clock from a [time.Ticker] until the context is cancelled
// or no timers remain, and returns the reason: ctx.Err(), or nil for "nothing
// left to run".
//
// interval is the poll period; zero or less means [DefaultInterval]. The poll
// wakes early when a timer is created or restarted, so a short timeout is not
// rounded up to the next tick.
//
// It returns immediately, having run nothing, if the clock has no timers. That
// makes "start the loop when there is work and let it exit when there is none"
// the default shape, as it is in the browser.
//
// Calling Run on a manual clock panics: its time is yours to advance, and a
// background loop that could not advance it would spin forever.
func (c *Clock) Run(ctx context.Context, interval time.Duration) error {
	c.mu.Lock()
	wall := c.wall
	c.mu.Unlock()
	if !wall {
		panic("timer: Run called on a manual clock; advance it with Advance instead")
	}
	if interval <= 0 {
		interval = DefaultInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if c.Active() == 0 {
			return nil
		}
		c.Flush()
		if c.Active() == 0 {
			return nil
		}

		c.mu.Lock()
		woken := c.woken
		c.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		case <-woken:
		}
	}
}

func (c *Clock) add(t *Timer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, existing := range c.timers {
		if existing == t {
			return
		}
	}
	c.timers = append(c.timers, t)
}

func (c *Clock) remove(t *Timer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, existing := range c.timers {
		if existing == t {
			c.timers = append(c.timers[:i:i], c.timers[i+1:]...)
			return
		}
	}
}

// wake nudges any goroutine parked in Run so it re-evaluates immediately.
func (c *Clock) wake() {
	c.mu.Lock()
	old := c.woken
	c.woken = make(chan struct{})
	c.mu.Unlock()
	close(old)
}

// ---------------------------------------------------------------------------
// The default clock.

var (
	defaultClock = NewWallClock()

	defaultMu      sync.Mutex
	defaultRunning bool
)

// DefaultClock returns the wall clock the package-level functions use. Reach
// for it to inspect [Clock.Active] or to call [Clock.StopAll] at shutdown.
func DefaultClock() *Clock { return defaultClock }

// Now returns the current time of the default clock, in milliseconds since the
// process started. It is d3.now.
func Now() float64 { return defaultClock.Now() }

// FlushTimers runs every due timer on the default clock immediately, without
// waiting for the next poll. It is d3.timerFlush.
func FlushTimers() { defaultClock.Flush() }

// New schedules cb on the default clock, starting delay milliseconds from now,
// and starts the background poll if it is not already running. It is d3.timer.
func New(cb Callback, delay float64) *Timer {
	t := defaultClock.Timer(cb, delay)
	ensureDefaultRunning()
	return t
}

// NewAt is [New] relative to an explicit clock time.
func NewAt(cb Callback, delay, at float64) *Timer {
	t := defaultClock.TimerAt(cb, delay, at)
	ensureDefaultRunning()
	return t
}

// Timeout schedules cb to run once on the default clock. It is d3.timeout.
func Timeout(cb Callback, delay float64) *Timer {
	t := defaultClock.Timeout(cb, delay)
	ensureDefaultRunning()
	return t
}

// Interval schedules cb to run every delay milliseconds on the default clock.
// It is d3.interval.
func Interval(cb Callback, delay float64) *Timer {
	t := defaultClock.Interval(cb, delay)
	ensureDefaultRunning()
	return t
}

// ensureDefaultRunning starts the default clock's poll goroutine if it is not
// already running. The goroutine exits as soon as no timers remain, so an
// idle program holds no timer goroutine — the same lifecycle as the browser's
// rAF loop, which d3 also stops when its queue empties.
func ensureDefaultRunning() {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultRunning {
		return
	}
	defaultRunning = true
	go func() {
		for {
			_ = defaultClock.Run(context.Background(), DefaultInterval)
			// Run exits when the queue empties. Re-check under the same lock
			// ensureDefaultRunning holds, so a timer created in the instant
			// between those two events cannot be left without a driver.
			defaultMu.Lock()
			if defaultClock.Active() == 0 {
				defaultRunning = false
				defaultMu.Unlock()
				return
			}
			defaultMu.Unlock()
		}
	}()
}
