package timer

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- Manual clock: everything here is exact ---------------------------------

func TestManualClockStartsAtZeroAndAdvancesExactly(t *testing.T) {
	c := NewClock()
	if c.Now() != 0 {
		t.Errorf("Now() = %v, want 0", c.Now())
	}
	c.Advance(16)
	if c.Now() != 16 {
		t.Errorf("Now() = %v, want 16", c.Now())
	}
	c.AdvanceTo(1000)
	if c.Now() != 1000 {
		t.Errorf("Now() = %v, want 1000", c.Now())
	}
}

func TestTimerElapsedIsTimeSinceItBecameDue(t *testing.T) {
	c := NewClock()
	var got []float64
	c.Timer(func(e float64) { got = append(got, e) }, 0)

	c.Advance(16)
	c.Advance(16)
	c.Advance(1)

	want := []float64{16, 32, 33}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("elapsed[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestDelayPostponesTheFirstCall(t *testing.T) {
	c := NewClock()
	var got []float64
	c.Timer(func(e float64) { got = append(got, e) }, 100)

	c.Advance(50)
	if len(got) != 0 {
		t.Fatalf("ran before its delay: %v", got)
	}
	c.Advance(50) // now exactly due
	c.Advance(25)
	want := []float64{0, 25}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTimerAtAnchorsToAnExplicitTime(t *testing.T) {
	c := NewClock()
	c.AdvanceTo(1000)
	var got float64
	c.TimerAt(func(e float64) { got = e }, 100, 500) // due at 600, already past
	c.Flush()
	if got != 400 {
		t.Errorf("elapsed = %v, want 400", got)
	}
}

func TestFlushRunsDueTimersWithoutAdvancingTime(t *testing.T) {
	c := NewClock()
	var n int
	c.Timer(func(float64) { n++ }, 0)
	if n != 0 {
		t.Fatal("the timer ran on creation")
	}
	c.Flush()
	c.Flush()
	if n != 2 {
		t.Errorf("n = %d, want 2", n)
	}
	if c.Now() != 0 {
		t.Errorf("Flush advanced the clock to %v", c.Now())
	}
}

func TestTimeoutRunsOnceAndReportsTheFullDelay(t *testing.T) {
	c := NewClock()
	var calls []float64
	c.Timeout(func(e float64) { calls = append(calls, e) }, 250)

	c.Advance(100)
	if len(calls) != 0 {
		t.Fatalf("timeout fired early: %v", calls)
	}
	c.Advance(200) // now at 300, 50ms late
	c.Advance(200)
	c.Advance(200)

	if len(calls) != 1 {
		t.Fatalf("timeout fired %d times, want 1", len(calls))
	}
	// Upstream's contract: elapsed includes the delay, so this reports how
	// late the callback actually was, not zero.
	if calls[0] != 300 {
		t.Errorf("elapsed = %v, want 300 (50 late plus the 250 delay)", calls[0])
	}
	if c.Active() != 0 {
		t.Errorf("a fired timeout left %d timers behind", c.Active())
	}
}

func TestIntervalFiresOnScheduleAndDoesNotDrift(t *testing.T) {
	c := NewClock()
	var got []float64
	c.Interval(func(e float64) { got = append(got, e) }, 100)

	// Uneven steps landing at 30, 60, 105, 305, 310, 400.
	for _, step := range []float64{30, 30, 45, 200, 5, 90} {
		c.Advance(step)
	}
	// The *schedule* is anchored to the start — the due times are 100, 200,
	// 300, 400 — but the elapsed value a callback receives is real time since
	// the start, not the scheduled time, which is upstream's contract and the
	// one an animation wants. So the interval fires four times, at the first
	// pass at or after each due time, reporting when it actually ran.
	//
	// Note also that a single pass fires an interval at most once, however far
	// the clock jumped: the step from 105 to 305 crosses two due times and
	// produces one call, not two. d3 behaves the same way, and it is what
	// stops a stalled tab from replaying a burst of frames.
	want := []float64{105, 305, 310, 400}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("interval[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestIntervalWithoutDelayIsATimer(t *testing.T) {
	c := NewClock()
	var n int
	c.Interval(func(float64) { n++ }, 0)
	c.Advance(1)
	c.Advance(1)
	if n != 2 {
		t.Errorf("n = %d, want 2", n)
	}
}

func TestStopAndRestart(t *testing.T) {
	c := NewClock()
	var n int
	tm := c.Timer(func(float64) { n++ }, 0)

	c.Advance(10)
	tm.Stop()
	if !tm.Stopped() {
		t.Error("Stopped() = false after Stop()")
	}
	c.Advance(10)
	if n != 1 {
		t.Fatalf("n = %d after stopping, want 1", n)
	}
	if c.Active() != 0 {
		t.Errorf("Active() = %d after Stop, want 0", c.Active())
	}

	tm.Stop() // idempotent

	tm.Restart(func(float64) { n += 10 }, 0)
	if c.Active() != 1 {
		t.Fatalf("Active() = %d after Restart, want 1", c.Active())
	}
	c.Advance(10)
	if n != 11 {
		t.Errorf("n = %d after restart, want 11", n)
	}
}

func TestStopFromInsideACallback(t *testing.T) {
	c := NewClock()
	var n int
	var tm *Timer
	tm = c.Timer(func(float64) {
		n++
		tm.Stop()
	}, 0)
	c.Advance(1)
	c.Advance(1)
	if n != 1 {
		t.Errorf("n = %d, want 1", n)
	}
}

func TestTimersCreatedDuringAPassRunOnTheNextOne(t *testing.T) {
	c := NewClock()
	var order []string
	c.Timer(func(float64) {
		order = append(order, "outer")
		if len(order) == 1 {
			c.Timeout(func(float64) { order = append(order, "inner") }, 0)
		}
	}, 0)

	c.Advance(1)
	if len(order) != 1 {
		t.Fatalf("pass 1 ran %v, want just the outer timer", order)
	}
	c.Advance(1)
	if len(order) != 3 || order[1] != "outer" || order[2] != "inner" {
		t.Errorf("pass 2 ran %v, want outer then inner", order)
	}
}

func TestSeveralTimersShareOneNow(t *testing.T) {
	c := NewClock()
	var a, b float64
	c.Timer(func(e float64) { a = e }, 0)
	c.Timer(func(e float64) { b = e }, 0)
	c.Advance(17)
	if a != b || a != 17 {
		t.Errorf("a=%v b=%v, want both 17 — timers must share one clock reading", a, b)
	}
}

func TestStopAll(t *testing.T) {
	c := NewClock()
	var n int
	for i := 0; i < 5; i++ {
		c.Timer(func(float64) { n++ }, 0)
	}
	if c.Active() != 5 {
		t.Fatalf("Active() = %d, want 5", c.Active())
	}
	c.StopAll()
	if c.Active() != 0 {
		t.Fatalf("Active() = %d after StopAll, want 0", c.Active())
	}
	c.Advance(100)
	if n != 0 {
		t.Errorf("n = %d after StopAll, want 0", n)
	}
}

func TestAdvanceBackwardsMakesFewerTimersDue(t *testing.T) {
	c := NewClock()
	var n int
	c.Timer(func(float64) { n++ }, 100)
	c.AdvanceTo(200)
	c.AdvanceTo(50)
	if n != 1 {
		t.Errorf("n = %d, want 1", n)
	}
}

func TestManualClockRejectsWallClockDriving(t *testing.T) {
	c := NewClock()
	defer func() {
		if recover() == nil {
			t.Error("Run on a manual clock did not panic")
		}
	}()
	_ = c.Run(context.Background(), time.Millisecond)
}

func TestWallClockRejectsManualAdvance(t *testing.T) {
	c := NewWallClock()
	defer func() {
		if recover() == nil {
			t.Error("Advance on a wall clock did not panic")
		}
	}()
	c.Advance(1)
}

// A manual clock makes a render reproducible: two runs of the same step
// sequence produce byte-identical output. That is the reason this package
// exists in the shape it does, so it is worth asserting directly.
func TestManualClockIsReproducible(t *testing.T) {
	run := func() []float64 {
		c := NewClock()
		var out []float64
		c.Timer(func(e float64) { out = append(out, math.Round(e*1000)/1000) }, 0)
		for i := 0; i < 60; i++ {
			c.Advance(1000.0 / 60.0)
		}
		return out
	}
	a, b := run(), run()
	if len(a) != 60 {
		t.Fatalf("got %d frames, want 60", len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("frame %d differs between runs: %v vs %v", i, a[i], b[i])
		}
	}
}

// --- Wall clock -------------------------------------------------------------

func TestWallClockNowAdvancesOnItsOwn(t *testing.T) {
	c := NewWallClock()
	first := c.Now()
	time.Sleep(5 * time.Millisecond)
	second := c.Now()
	if !(second > first) {
		t.Errorf("wall clock did not advance: %v then %v", first, second)
	}
}

func TestRunDrivesTimersAndExitsWhenTheQueueEmpties(t *testing.T) {
	c := NewWallClock()
	done := make(chan float64, 1)
	c.Timeout(func(e float64) { done <- e }, 20)

	start := time.Now()
	err := c.Run(context.Background(), 2*time.Millisecond)
	if err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	select {
	case e := <-done:
		if e < 20 {
			t.Errorf("elapsed = %v, want at least the 20ms delay", e)
		}
	default:
		t.Fatal("Run returned without firing the timeout")
	}
	if time.Since(start) < 15*time.Millisecond {
		t.Errorf("Run returned after %v, too soon for a 20ms timeout", time.Since(start))
	}
	if c.Active() != 0 {
		t.Errorf("Active() = %d after Run, want 0", c.Active())
	}
}

func TestRunReturnsImmediatelyWithNoTimers(t *testing.T) {
	c := NewWallClock()
	if err := c.Run(context.Background(), time.Second); err != nil {
		t.Errorf("Run = %v, want nil", err)
	}
}

func TestRunHonoursContextCancellation(t *testing.T) {
	c := NewWallClock()
	c.Timer(func(float64) {}, 0) // never stops on its own
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	if err := c.Run(ctx, 2*time.Millisecond); err != context.Canceled {
		t.Errorf("Run = %v, want context.Canceled", err)
	}
	c.StopAll()
}

// --- The default clock ------------------------------------------------------

func TestDefaultClockRunsWithoutAnExplicitDriver(t *testing.T) {
	done := make(chan struct{})
	Timeout(func(float64) { close(done) }, 10)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the default clock never fired a 10ms timeout")
	}
	if Now() <= 0 {
		t.Errorf("Now() = %v, want positive", Now())
	}
}

func TestDefaultIntervalRepeats(t *testing.T) {
	var n atomic.Int64
	done := make(chan struct{})
	var once sync.Once
	tm := Interval(func(float64) {
		if n.Add(1) >= 3 {
			once.Do(func() { close(done) })
		}
	}, 5)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the default interval did not repeat")
	}
	tm.Stop()
}

func TestFlushTimersOnTheDefaultClock(t *testing.T) {
	var n atomic.Int64
	tm := New(func(float64) { n.Add(1) }, 0)
	defer tm.Stop()
	FlushTimers()
	if n.Load() == 0 {
		t.Error("FlushTimers did not run a due timer")
	}
}

// --- Concurrency ------------------------------------------------------------

// Run with -race. Timers are created, restarted and stopped from many
// goroutines while the clock is being advanced.
func TestConcurrentTimerChurn(t *testing.T) {
	c := NewClock()
	var fired atomic.Int64
	var wg sync.WaitGroup
	stop := make(chan struct{})

	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				select {
				case <-stop:
					return
				default:
				}
				tm := c.Timer(func(float64) { fired.Add(1) }, 0)
				tm.Restart(func(float64) { fired.Add(1) }, 0)
				_ = c.Now()
				_ = c.Active()
				tm.Stop()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			c.Advance(1)
		}
		close(stop)
	}()
	wg.Wait()
	c.StopAll()

	if c.Active() != 0 {
		t.Errorf("Active() = %d after the churn, want 0", c.Active())
	}
}

func TestWallClockConcurrentRunAndCreate(t *testing.T) {
	c := NewWallClock()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	keepalive := c.Timer(func(float64) {}, 0)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = c.Run(ctx, 2*time.Millisecond)
	}()

	var fired atomic.Int64
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				c.Timeout(func(float64) { fired.Add(1) }, 1)
				time.Sleep(time.Millisecond)
			}
		}()
	}
	wg.Wait()
	keepalive.Stop()
	cancel()

	if fired.Load() == 0 {
		t.Error("no timeout ever fired; the test proved nothing")
	}
}

func TestIntervalOnEvenSteps(t *testing.T) {
	c := NewClock()
	var got []float64
	c.Interval(func(e float64) { got = append(got, e) }, 100)
	for i := 0; i < 6; i++ {
		c.Advance(100)
	}
	want := []float64{100, 200, 300, 400, 500, 600}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("interval[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}
