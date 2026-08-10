// Command oban-example exercises the github.com/malcolmston/oban background job
// engine end to end against its in-memory store. Everything runs in process:
// no database, no network, and every loop is bounded so the program terminates
// on its own.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/malcolmston/oban"
)

func section(title string) {
	fmt.Printf("\n=== %s %s\n", title, strings.Repeat("=", maxInt(0, 62-len(title))))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// quiet silences the engine's operational logger so the example's own output is
// the only thing on stdout.
var quiet = log.New(io.Discard, "", 0)

// manualClock is a Clock whose time only moves when the test moves it. The
// engine, stores, backoff windows and cron plugin all read time through it,
// which is what makes the scheduled / unique / retry behaviour reproducible.
type manualClock struct {
	mu sync.Mutex
	t  time.Time
}

func newManualClock(t time.Time) *manualClock { return &manualClock{t: t} }

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *manualClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

var epoch = time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

func main() {
	log.SetFlags(0)
	ctx := context.Background()

	jobsAndOptions()
	cronSchedules()
	backoffPolicies(ctx)
	deterministicDrain(ctx)
	liveEngine(ctx)
	uniqueJobs(ctx)
	richInsert(ctx)
	scheduledJobs(ctx)
	operatorControl(ctx)
	pauseAndScale(ctx)
	plugins(ctx)
	rateLimiter()
	testingHelpers(ctx)
	workerConfig(ctx)
	sqlStoreReality()

	fmt.Println("\nall oban example steps completed")
}

// ----------------------------------------------------------------------
// 1. Job construction, options and query helpers
// ----------------------------------------------------------------------

type emailArgs struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
}

func jobsAndOptions() {
	section("1. jobs, options and query helpers")

	job, err := oban.NewJob("email",
		emailArgs{To: "ada@example.com", Subject: "hi"},
		oban.WithQueue("mailers"),
		oban.WithMaxAttempts(5),
		oban.WithPriority(3),
		oban.WithScheduleIn(90*time.Second),
		oban.WithUnique("welcome:42", time.Hour),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("queue=%s worker=%s max_attempts=%d priority=%d state=%s\n",
		job.Queue, job.Worker, job.MaxAttempts, job.Priority, job.State)
	fmt.Printf("unique=%q period=%s args=%s\n", job.UniqueKey, job.UniquePeriod, job.Args)

	var decoded emailArgs
	if err := job.UnmarshalArgs(&decoded); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("UnmarshalArgs -> %+v\n", decoded)

	clone := job.Clone()
	clone.Queue = "other"
	fmt.Printf("Clone is independent? %v (original queue still %q)\n", job.Queue != clone.Queue, job.Queue)

	fmt.Printf("defaults: NewJob(\"w\", nil) -> ")
	d, _ := oban.NewJob("w", nil)
	fmt.Printf("queue=%s max_attempts=%d args=%s state=%s\n", d.Queue, d.MaxAttempts, d.Args, d.State)

	if _, err := oban.NewJob("", nil); err != nil {
		fmt.Printf("empty worker rejected: %v\n", err)
	}

	// Job query helpers.
	j := &oban.Job{
		State: oban.StateRetryable, Attempt: 2, MaxAttempts: 5,
		InsertedAt: epoch, AttemptedAt: epoch.Add(time.Second), CompletedAt: epoch.Add(3 * time.Second),
		LastError: "boom",
	}
	fmt.Printf("IsFinished=%v InState(retryable,available)=%v AttemptsRemaining=%d HasErrored=%v\n",
		j.IsFinished(), j.InState(oban.StateRetryable, oban.StateAvailable), j.AttemptsRemaining(), j.HasErrored())
	fmt.Printf("ExecutionDuration=%s Age=%s\n", j.ExecutionDuration(), j.Age(epoch.Add(time.Minute)))

	fmt.Printf("ValidatePriority(0)=%v ValidatePriority(9)=%v ValidatePriority(10)=%v\n",
		oban.ValidatePriority(0), oban.ValidatePriority(9), oban.ValidatePriority(10))
}

// ----------------------------------------------------------------------
// 2. Cron schedules (pure)
// ----------------------------------------------------------------------

func cronSchedules() {
	section("2. cron schedules")

	base := time.Date(2026, 3, 4, 12, 34, 0, 0, time.UTC) // a Wednesday

	for _, expr := range []string{"*/15 * * * *", "0 9 * * mon-fri", "0 0 1,15 * *", "30 3 * mar *"} {
		s := oban.MustParseCron(expr)
		fmt.Printf("%-18s next=%s matches(now)=%v\n", s.String(),
			s.Next(base).Format(time.RFC3339), s.Matches(base))
	}

	every15 := oban.MustParseCron("*/15 * * * *")
	var up []string
	for _, t := range every15.Upcoming(base, 4) {
		up = append(up, t.Format("15:04"))
	}
	fmt.Printf("Upcoming(4)  : %s\n", strings.Join(up, ", "))
	fmt.Printf("Prev         : %s\n", every15.Prev(base).Format(time.RFC3339))

	for _, spec := range []string{"@hourly", "@daily", "@weekly", "@monthly", "@yearly"} {
		s := oban.MustParseCronSpec(spec)
		fmt.Printf("%-10s -> %-12s next=%s\n", spec, s.String(), s.Next(base).Format(time.RFC3339))
	}

	if _, err := oban.ParseCron("bogus"); err != nil {
		fmt.Printf("bad cron rejected      : %v\n", err)
	}
	if _, err := oban.ParseCronSpec("@never"); err != nil {
		fmt.Printf("bad cron macro rejected: %v\n", err)
	}

	// Vixie "either matches" rule when both day fields are restricted.
	both := oban.MustParseCron("0 0 13 * fri")
	fmt.Printf("0 0 13 * fri next from %s -> %s\n",
		base.Format("2006-01-02"), both.Next(base).Format("2006-01-02 (Mon)"))
}

// ----------------------------------------------------------------------
// 3. Backoff policies
// ----------------------------------------------------------------------

func backoffPolicies(_ context.Context) {
	section("3. backoff policies")

	policies := []struct {
		name string
		b    oban.Backoff
	}{
		{"Exponential(1s,1h)", &oban.ExponentialBackoff{Base: time.Second, Max: time.Hour}},
		{"Exponential+jitter", oban.NewExponentialBackoff(time.Second, time.Hour, 0.5, 42)},
		{"Constant(750ms)", oban.NewConstantBackoff(750 * time.Millisecond)},
		{"Linear(1s,+2s,30s)", oban.NewLinearBackoff(time.Second, 2*time.Second, 30*time.Second)},
		{"Fibonacci(1s,1m)", oban.NewFibonacciBackoff(time.Second, time.Minute)},
		{"BackoffFunc(n^2 s)", oban.BackoffFunc(func(n int) time.Duration {
			return time.Duration(n*n) * time.Second
		})},
	}
	for _, p := range policies {
		var out []string
		for attempt := 1; attempt <= 7; attempt++ {
			out = append(out, p.b.Next(attempt).String())
		}
		fmt.Printf("%-20s %s\n", p.name, strings.Join(out, " "))
	}

	// The seeded jitter source makes the sequence reproducible.
	a := oban.NewExponentialBackoff(time.Second, time.Hour, 0.5, 7)
	b := oban.NewExponentialBackoff(time.Second, time.Hour, 0.5, 7)
	same := true
	for attempt := 1; attempt <= 5; attempt++ {
		if a.Next(attempt) != b.Next(attempt) {
			same = false
		}
	}
	fmt.Printf("seeded jitter reproducible? %v\n", same)
}

// ----------------------------------------------------------------------
// 4. Deterministic Drain: retries, discard, snooze, cancel, missing worker
// ----------------------------------------------------------------------

func deterministicDrain(ctx context.Context) {
	section("4. Drain: retry / discard / snooze / cancel (no goroutines)")

	clock := newManualClock(epoch)

	// --- a failing job retried until it is discarded.
	store := oban.NewInMemoryStore()
	reg := oban.NewRegistry()
	attempts := 0
	reg.RegisterFunc("flaky", func(ctx context.Context, j *oban.Job) error {
		attempts++
		return fmt.Errorf("attempt %d failed", j.Attempt)
	})
	j, _ := oban.NewJob("flaky", nil, oban.WithMaxAttempts(3))
	stored, _, err := enqueueAt(ctx, store, clock, j)
	if err != nil {
		log.Fatal(err)
	}
	res, err := oban.Drain(ctx, store, reg, clock, oban.DrainOptions{
		Queue:         "default",
		WithRecursion: true,
		WithScheduled: true, // ignore the backoff delay so the retries run now
		Backoff:       oban.NewConstantBackoff(30 * time.Second),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("failing job : completed=%d retried=%d discarded=%d total=%d (worker ran %d times)\n",
		res.Completed, res.Retried, res.Discarded, res.Total(), attempts)
	final, _ := store.Get(ctx, stored.ID)
	fmt.Printf("            : state=%s attempt=%d/%d last_error=%q errors_recorded=%d\n",
		final.State, final.Attempt, final.MaxAttempts, final.LastError, len(final.Errors))
	for _, e := range final.Errors {
		fmt.Printf("            :   attempt %d -> %s\n", e.Attempt, e.Message)
	}

	// --- a job that fails once then succeeds.
	store2 := oban.NewInMemoryStore()
	reg2 := oban.NewRegistry()
	tries := 0
	reg2.RegisterFunc("eventually", func(ctx context.Context, j *oban.Job) error {
		tries++
		if tries == 1 {
			return errors.New("transient")
		}
		return nil
	})
	j2, _ := oban.NewJob("eventually", nil, oban.WithMaxAttempts(3))
	s2, _, _ := enqueueAt(ctx, store2, clock, j2)
	res, _ = oban.Drain(ctx, store2, reg2, clock, oban.DrainOptions{
		Queue: "default", WithRecursion: true, WithScheduled: true,
		Backoff: oban.NewConstantBackoff(time.Second),
	})
	got, _ := store2.Get(ctx, s2.ID)
	fmt.Printf("retry->ok   : retried=%d completed=%d final state=%s attempt=%d\n",
		res.Retried, res.Completed, got.State, got.Attempt)

	// --- snooze: the attempt is given back, so it does not count as a failure.
	store3 := oban.NewInMemoryStore()
	reg3 := oban.NewRegistry()
	snoozed := false
	reg3.RegisterFunc("sleepy", func(ctx context.Context, j *oban.Job) error {
		if !snoozed {
			snoozed = true
			return oban.Snooze(5 * time.Minute)
		}
		return nil
	})
	j3, _ := oban.NewJob("sleepy", nil, oban.WithMaxAttempts(3))
	s3, _, _ := enqueueAt(ctx, store3, clock, j3)
	res, _ = oban.Drain(ctx, store3, reg3, clock, oban.DrainOptions{
		Queue: "default", WithRecursion: true, WithScheduled: true,
	})
	got3, _ := store3.Get(ctx, s3.ID)
	fmt.Printf("snooze      : snoozed=%d completed=%d final state=%s attempt=%d (attempt not consumed)\n",
		res.Snoozed, res.Completed, got3.State, got3.Attempt)

	// --- cancel: terminal, no retry.
	store4 := oban.NewInMemoryStore()
	reg4 := oban.NewRegistry()
	reg4.RegisterFunc("doomed", func(ctx context.Context, j *oban.Job) error {
		return oban.CancelJob("customer deleted")
	})
	j4, _ := oban.NewJob("doomed", nil, oban.WithMaxAttempts(5))
	s4, _, _ := enqueueAt(ctx, store4, clock, j4)
	res, _ = oban.Drain(ctx, store4, reg4, clock, oban.DrainOptions{
		Queue: "default", WithRecursion: true, WithScheduled: true,
	})
	got4, _ := store4.Get(ctx, s4.ID)
	fmt.Printf("cancel      : cancelled=%d final state=%s\n", res.Cancelled, got4.State)

	// --- unregistered worker: discarded so the drain can make progress.
	store5 := oban.NewInMemoryStore()
	j5, _ := oban.NewJob("nobody-home", nil)
	s5, _, _ := enqueueAt(ctx, store5, clock, j5)
	res, _ = oban.Drain(ctx, store5, oban.NewRegistry(), clock, oban.DrainOptions{Queue: "default"})
	got5, _ := store5.Get(ctx, s5.ID)
	fmt.Printf("no worker   : failed=%d final state=%s last_error=%q\n", res.Failed, got5.State, got5.LastError)

	// --- a panicking worker is converted into an ordinary error.
	store6 := oban.NewInMemoryStore()
	reg6 := oban.NewRegistry()
	reg6.RegisterFunc("boom", func(ctx context.Context, j *oban.Job) error {
		panic("kaboom")
	})
	j6, _ := oban.NewJob("boom", nil, oban.WithMaxAttempts(1))
	s6, _, _ := enqueueAt(ctx, store6, clock, j6)
	res, _ = oban.Drain(ctx, store6, reg6, clock, oban.DrainOptions{Queue: "default"})
	got6, _ := store6.Get(ctx, s6.ID)
	fmt.Printf("panic       : discarded=%d state=%s last_error=%q\n", res.Discarded, got6.State, got6.LastError)

	// --- middleware ordering inside Drain (mw[0] is outermost).
	var order []string
	mk := func(name string) oban.Middleware {
		return func(next oban.Handler) oban.Handler {
			return func(ctx context.Context, j *oban.Job) error {
				order = append(order, "->"+name)
				err := next(ctx, j)
				order = append(order, "<-"+name)
				return err
			}
		}
	}
	store7 := oban.NewInMemoryStore()
	reg7 := oban.NewRegistry()
	reg7.RegisterFunc("mw", func(ctx context.Context, j *oban.Job) error {
		order = append(order, "worker")
		return nil
	})
	j7, _ := oban.NewJob("mw", nil)
	_, _, _ = enqueueAt(ctx, store7, clock, j7)
	_, _ = oban.Drain(ctx, store7, reg7, clock, oban.DrainOptions{
		Queue:      "default",
		Middleware: []oban.Middleware{mk("outer"), mk("inner")},
	})
	fmt.Printf("middleware  : %s\n", strings.Join(order, " "))

	// --- a per-attempt timeout is honoured.
	store8 := oban.NewInMemoryStore()
	reg8 := oban.NewRegistry()
	reg8.RegisterFunc("slow", func(ctx context.Context, j *oban.Job) error {
		select {
		case <-ctx.Done():
			return fmt.Errorf("attempt context: %w", ctx.Err())
		case <-time.After(2 * time.Second):
			return nil
		}
	})
	j8, _ := oban.NewJob("slow", nil, oban.WithMaxAttempts(1))
	s8, _, _ := enqueueAt(ctx, store8, clock, j8)
	_, _ = oban.Drain(ctx, store8, reg8, clock, oban.DrainOptions{
		Queue: "default", Timeout: 20 * time.Millisecond,
	})
	got8, _ := store8.Get(ctx, s8.ID)
	fmt.Printf("timeout     : state=%s last_error=%q\n", got8.State, got8.LastError)

	if _, err := oban.Drain(ctx, store, reg, clock, oban.DrainOptions{}); err != nil {
		fmt.Printf("missing queue rejected: %v\n", err)
	}
}

// ----------------------------------------------------------------------
// 5. The live engine: queues, concurrency, priorities, telemetry, events
// ----------------------------------------------------------------------

func liveEngine(ctx context.Context) {
	section("5. live engine: queues, priority ordering, telemetry, events")

	store := oban.NewInMemoryStore()
	recorder := oban.NewRecorder()

	var mu sync.Mutex
	var ran []string
	var telemetry []string

	var mwOrder []string
	trace := func(name string) oban.Middleware {
		return func(next oban.Handler) oban.Handler {
			return func(ctx context.Context, j *oban.Job) error {
				mu.Lock()
				mwOrder = append(mwOrder, name)
				mu.Unlock()
				return next(ctx, j)
			}
		}
	}

	var errored []string
	engine, err := oban.New(oban.Config{
		Store:        store,
		Queues:       map[string]int{"default": 1, "mailers": 3},
		Backoff:      oban.NewConstantBackoff(5 * time.Millisecond),
		PollInterval: 2 * time.Millisecond,
		JobTimeout:   time.Second,
		QueueTimeouts: map[string]time.Duration{
			"mailers": 30 * time.Millisecond,
		},
		Middleware: []oban.Middleware{trace("outer"), trace("inner"), recorder.Middleware(nil)},
		Telemetry: &oban.Telemetry{
			OnStart: func(ctx context.Context, j *oban.Job) {
				mu.Lock()
				telemetry = append(telemetry, "start:"+j.Worker)
				mu.Unlock()
			},
			OnComplete: func(ctx context.Context, j *oban.Job, d time.Duration) {
				mu.Lock()
				telemetry = append(telemetry, "complete:"+j.Worker)
				mu.Unlock()
			},
			OnError: func(ctx context.Context, j *oban.Job, err error, d time.Duration) {
				mu.Lock()
				telemetry = append(telemetry, "error:"+j.Worker)
				mu.Unlock()
			},
		},
		ErrorHandler: func(j *oban.Job, err error) {
			mu.Lock()
			errored = append(errored, fmt.Sprintf("%s#%d:%v", j.Worker, j.Attempt, err))
			mu.Unlock()
		},
		Logger: quiet,
	})
	if err != nil {
		log.Fatal(err)
	}

	detach := recorder.Attach(func(ev oban.Event) {
		_ = ev // counts are inspected below; a handler proves fan-out works
	})
	defer detach()

	engine.RegisterFunc("ordered", func(ctx context.Context, j *oban.Job) error {
		var a struct{ Label string `json:"label"` }
		if err := j.UnmarshalArgs(&a); err != nil {
			return err
		}
		mu.Lock()
		ran = append(ran, a.Label)
		mu.Unlock()
		return nil
	})
	engine.Register("mailer", oban.WorkerFunc(func(ctx context.Context, j *oban.Job) error {
		return nil
	}))
	engine.RegisterFunc("always-fails", func(ctx context.Context, j *oban.Job) error {
		return errors.New("nope")
	})

	fmt.Printf("registered workers: %v\n", sorted(engine.Registry().Names()))

	// Enqueue before starting so the priority ordering is observable on the
	// single-slot "default" queue.
	type spec struct {
		label    string
		priority int
	}
	for _, s := range []spec{{"p5-first-in", 5}, {"p0", 0}, {"p9", 9}, {"p1", 1}} {
		j, err := oban.NewJob("ordered", map[string]string{"label": s.label},
			oban.WithPriority(s.priority))
		if err != nil {
			log.Fatal(err)
		}
		if _, _, err := engine.Enqueue(ctx, j); err != nil {
			log.Fatal(err)
		}
	}

	// A batch insert onto the concurrent queue.
	var batch []*oban.Job
	for i := 0; i < 6; i++ {
		j, _ := oban.NewJob("mailer", map[string]int{"n": i}, oban.WithQueue("mailers"))
		batch = append(batch, j)
	}
	results, err := engine.EnqueueMany(ctx, batch)
	if err != nil {
		log.Fatal(err)
	}
	inserted := 0
	for _, r := range results {
		if r.Inserted {
			inserted++
		}
	}
	fmt.Printf("EnqueueMany: %d results, %d inserted\n", len(results), inserted)

	// One job that will exhaust its attempts.
	fail, _ := oban.NewJob("always-fails", nil, oban.WithMaxAttempts(3))
	if _, _, err := engine.Enqueue(ctx, fail); err != nil {
		log.Fatal(err)
	}

	if err := engine.Start(ctx); err != nil {
		log.Fatal(err)
	}

	// Bounded wait: everything must finish, or we give up after 5s.
	ok := waitFor(5*time.Second, func() bool {
		return store.CountByState(oban.StateCompleted) == 10 &&
			store.CountByState(oban.StateDiscarded) == 1
	})

	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := engine.Stop(shutdown); err != nil {
		log.Fatalf("Stop: %v", err)
	}

	fmt.Printf("all work finished within the deadline? %v\n", ok)
	fmt.Printf("store states: %v\n", statesString(store.States()))
	fmt.Printf("counts by queue: default=%d mailers=%d (Len=%d)\n",
		store.CountByQueue("default"), store.CountByQueue("mailers"), store.Len())

	mu.Lock()
	fmt.Printf("priority execution order: %v\n", ran)
	fmt.Printf("middleware chain (first 3): %v\n", mwOrder[:minInt(3, len(mwOrder))])
	fmt.Printf("telemetry events: %d (e.g. %v)\n", len(telemetry), telemetry[:minInt(3, len(telemetry))])
	fmt.Printf("ErrorHandler saw %d failed attempts, last: %s\n", len(errored), errored[len(errored)-1])
	mu.Unlock()

	fmt.Printf("recorder counts: %v\n", eventCounts(recorder.Counts()))

	// Start/Stop are guarded.
	if err := engine.Start(ctx); err != nil {
		fmt.Printf("restart rejected: %v\n", err)
	}
	if _, err := oban.New(oban.Config{Queues: map[string]int{"bad": 0}}); err != nil {
		fmt.Printf("zero concurrency rejected: %v\n", err)
	}
	if _, err := oban.New(oban.Config{Periodic: []oban.Periodic{{Worker: "x"}}}); err != nil {
		fmt.Printf("nil cron schedule rejected: %v\n", err)
	}
}

// ----------------------------------------------------------------------
// 6. Unique jobs (single key)
// ----------------------------------------------------------------------

func uniqueJobs(ctx context.Context) {
	section("6. unique jobs")

	clock := newManualClock(epoch)
	store := oban.NewInMemoryStore()
	engine, err := oban.New(oban.Config{Store: store, Clock: clock, Logger: quiet})
	if err != nil {
		log.Fatal(err)
	}
	engine.RegisterFunc("welcome", func(ctx context.Context, j *oban.Job) error { return nil })

	mk := func() *oban.Job {
		j, _ := oban.NewJob("welcome", map[string]int{"user": 42},
			oban.WithUnique("welcome:42", 10*time.Minute))
		return j
	}

	first, ins1, _ := engine.Enqueue(ctx, mk())
	second, ins2, _ := engine.Enqueue(ctx, mk())
	fmt.Printf("first : id=%d inserted=%v\n", first.ID, ins1)
	fmt.Printf("second: id=%d inserted=%v (de-duplicated onto the first)\n", second.ID, ins2)

	// After the unique period lapses a new job is accepted again.
	clock.Advance(11 * time.Minute)
	third, ins3, _ := engine.Enqueue(ctx, mk())
	fmt.Printf("after period: id=%d inserted=%v\n", third.ID, ins3)

	// A finished job never blocks new work.
	if err := store.Complete(ctx, third.ID, clock.Now()); err != nil {
		log.Fatal(err)
	}
	fourth, ins4, _ := engine.Enqueue(ctx, mk())
	fmt.Printf("after completing the holder: id=%d inserted=%v\n", fourth.ID, ins4)

	// A different key, or a different queue, is a different job.
	other, _ := oban.NewJob("welcome", nil, oban.WithUnique("welcome:99", time.Hour))
	_, ins5, _ := engine.Enqueue(ctx, other)
	fmt.Printf("different key inserted=%v; store now holds %d jobs\n", ins5, store.Len())
}

// ----------------------------------------------------------------------
// 7. Rich Insert: multi-dimension uniqueness, replace, tags and meta
// ----------------------------------------------------------------------

func richInsert(ctx context.Context) {
	section("7. Insert: UniqueBy / Replace / tags + meta")

	store := oban.NewInMemoryStore()
	mk := func(args any, opts ...oban.JobOption) *oban.Job {
		j, err := oban.NewJob("report", args, opts...)
		if err != nil {
			log.Fatal(err)
		}
		j.InsertedAt = epoch
		return j
	}

	opts := oban.InsertOpts{
		Unique: &oban.UniqueBy{
			Period: time.Hour,
			Fields: []oban.UniqueField{oban.UniqueWorker, oban.UniqueQueue, oban.UniqueArgs},
			States: []oban.State{oban.StateAvailable, oban.StateScheduled},
		},
		Tags: []string{"reporting", "nightly"},
		Meta: map[string]any{"tenant": "acme", "retries_allowed": 3},
	}

	first, ins, err := oban.Insert(ctx, store, mk(map[string]int{"day": 1}), opts)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("first insert : id=%d inserted=%v\n", first.ID, ins)

	tags, err := oban.Tags(ctx, store, first.ID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("tags read back: %v\n", tags)

	// Same worker+queue+args -> conflict, insert skipped.
	dup, ins, err := oban.Insert(ctx, store, mk(map[string]int{"day": 1}), opts)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("duplicate args: id=%d inserted=%v (same row)\n", dup.ID, ins)

	// Different args -> not a conflict, because UniqueArgs is one of the fields.
	other, ins, err := oban.Insert(ctx, store, mk(map[string]int{"day": 2}), opts)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("different args: id=%d inserted=%v\n", other.ID, ins)

	// Replace-on-conflict overwrites selected fields of the existing job.
	replaceOpts := opts
	replaceOpts.Replace = []oban.ReplaceField{oban.ReplacePriority, oban.ReplaceScheduledAt, oban.ReplaceMaxAttempts}
	later := epoch.Add(2 * time.Hour)
	conflict, ins, err := oban.Insert(ctx, store, mk(map[string]int{"day": 1},
		oban.WithPriority(7), oban.WithMaxAttempts(4), oban.WithScheduledAt(later)), replaceOpts)
	if err != nil {
		log.Fatal(err)
	}
	stored, _ := store.Get(ctx, conflict.ID)
	fmt.Printf("replace      : id=%d inserted=%v priority=%d max_attempts=%d scheduled_at=%s\n",
		stored.ID, ins, stored.Priority, stored.MaxAttempts, stored.ScheduledAt.Format(time.RFC3339))
	fmt.Printf("             : incoming wanted priority=7 max_attempts=4 scheduled_at=%s\n",
		later.Format(time.RFC3339))
	// HOLE: InsertOpts.Replace is a no-op for ScheduledAt / Priority /
	// MaxAttempts / Args. insertApplyReplace mutates the *Job that FindConflict
	// returned, but InMemoryStore.FindConflict (like any conforming Store,
	// whose docs require returned jobs to be copies) hands back a Clone, and
	// the Store interface has no update method, so nothing is written back.
	// Only ReplaceTags / ReplaceMeta persist, because those go through
	// TaggableStore.SetTagsMeta.
	if stored.Priority != 7 {
		fmt.Println("             : ^ HOLE: Replace silently did not persist (see README)")
	}
	// Tags/Meta replacement, by contrast, does persist.
	tagOpts := opts
	tagOpts.Replace = []oban.ReplaceField{oban.ReplaceTags}
	tagOpts.Tags = []string{"replaced"}
	if _, _, err := oban.Insert(ctx, store, mk(map[string]int{"day": 1}), tagOpts); err != nil {
		log.Fatal(err)
	}
	newTags, _ := oban.Tags(ctx, store, first.ID)
	fmt.Printf("ReplaceTags  : tags now %v (this one does persist)\n", newTags)

	// Uniqueness restricted to worker only: any args collide.
	byWorker := oban.InsertOpts{Unique: &oban.UniqueBy{
		Period: time.Hour,
		Fields: []oban.UniqueField{oban.UniqueWorker},
		Keys:   []string{"nightly-batch"},
	}}
	fresh := oban.NewInMemoryStore()
	_, ins1, _ := oban.Insert(ctx, fresh, mk(map[string]int{"day": 100}), byWorker)
	_, ins2, _ := oban.Insert(ctx, fresh, mk(map[string]int{"day": 200}), byWorker)
	fmt.Printf("by worker+key: first inserted=%v, second inserted=%v\n", ins1, ins2)

	// Priority validation happens before anything is stored.
	if _, _, err := oban.Insert(ctx, store, mk(nil, oban.WithPriority(42)), oban.InsertOpts{}); err != nil {
		fmt.Printf("bad priority rejected: %v\n", err)
	}
	if _, _, err := oban.Insert(ctx, store, nil, oban.InsertOpts{}); err != nil {
		fmt.Printf("nil job rejected     : %v\n", err)
	}
	fmt.Printf("store holds %d jobs\n", store.Len())

	// InsertMany against a bare store.
	batch := []*oban.Job{mk(map[string]int{"day": 7}), mk(map[string]int{"day": 8})}
	rs, err := oban.InsertMany(ctx, store, batch)
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range rs {
		fmt.Printf("InsertMany   : id=%d inserted=%v\n", r.Job.ID, r.Inserted)
	}
}

// ----------------------------------------------------------------------
// 8. Scheduled / delayed jobs
// ----------------------------------------------------------------------

func scheduledJobs(ctx context.Context) {
	section("8. scheduled and delayed jobs")

	clock := newManualClock(epoch)
	store := oban.NewInMemoryStore()
	reg := oban.NewRegistry()
	var ran []string
	reg.RegisterFunc("later", func(ctx context.Context, j *oban.Job) error {
		var a struct{ Label string `json:"label"` }
		_ = j.UnmarshalArgs(&a)
		ran = append(ran, a.Label)
		return nil
	})

	now, _ := oban.NewJob("later", map[string]string{"label": "now"})
	in5, _ := oban.NewJob("later", map[string]string{"label": "in-5m"}, oban.WithScheduleIn(5*time.Minute))
	at1h, _ := oban.NewJob("later", map[string]string{"label": "at-+1h"},
		oban.WithScheduledAt(epoch.Add(time.Hour)))

	for _, j := range []*oban.Job{now, in5, at1h} {
		j.InsertedAt = clock.Now()
		s, _, err := store.Enqueue(ctx, j)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("enqueued %-7s state=%-9s scheduled_at=%s\n",
			mustLabel(s), s.State, s.ScheduledAt.Format("15:04:05"))
	}

	res, _ := oban.Drain(ctx, store, reg, clock, oban.DrainOptions{Queue: "default"})
	fmt.Printf("drain at %s          -> completed=%d ran=%v\n", clock.Now().Format("15:04"), res.Completed, ran)

	clock.Advance(6 * time.Minute)
	res, _ = oban.Drain(ctx, store, reg, clock, oban.DrainOptions{Queue: "default"})
	fmt.Printf("drain at %s (+6m)    -> completed=%d ran=%v\n", clock.Now().Format("15:04"), res.Completed, ran)

	// WithScheduled ignores the clock entirely.
	res, _ = oban.Drain(ctx, store, reg, clock, oban.DrainOptions{Queue: "default", WithScheduled: true})
	fmt.Printf("drain WithScheduled    -> completed=%d ran=%v\n", res.Completed, ran)
	fmt.Printf("final states: %v\n", statesString(store.States()))
}

func mustLabel(j *oban.Job) string {
	var a struct{ Label string `json:"label"` }
	_ = j.UnmarshalArgs(&a)
	return a.Label
}

// ----------------------------------------------------------------------
// 9. Operator control: cancel, retry-now, delete
// ----------------------------------------------------------------------

func operatorControl(ctx context.Context) {
	section("9. operator control (Controller)")

	clock := newManualClock(epoch)
	store := oban.NewInMemoryStore()
	ctl := oban.NewController(store, clock)

	mk := func(worker string) *oban.Job {
		j, _ := oban.NewJob(worker, nil, oban.WithMaxAttempts(1))
		s, _, err := enqueueAt(ctx, store, clock, j)
		if err != nil {
			log.Fatal(err)
		}
		return s
	}

	a, b, c := mk("a"), mk("b"), mk("c")

	if err := ctl.Cancel(ctx, a.ID); err != nil {
		log.Fatal(err)
	}
	got, _ := store.Get(ctx, a.ID)
	fmt.Printf("Cancel   -> state=%s\n", got.State)

	// Discard b, then force it to run again.
	if err := store.Discard(ctx, b.ID, errors.New("gave up"), clock.Now()); err != nil {
		log.Fatal(err)
	}
	before, _ := store.Get(ctx, b.ID)
	if err := ctl.Retry(ctx, b.ID); err != nil {
		log.Fatal(err)
	}
	after, _ := store.Get(ctx, b.ID)
	fmt.Printf("Retry    -> %s -> %s (scheduled_at=%s)\n",
		before.State, after.State, after.ScheduledAt.Format("15:04:05"))

	if err := ctl.Delete(ctx, c.ID); err != nil {
		log.Fatal(err)
	}
	if _, err := store.Get(ctx, c.ID); errors.Is(err, oban.ErrJobNotFound) {
		fmt.Printf("Delete   -> %v\n", err)
	}

	if err := ctl.Cancel(ctx, 9999); err != nil {
		fmt.Printf("unknown id -> %v\n", err)
	}

	// A store without the optional capabilities degrades to ErrUnsupported.
	bare := oban.NewController(&minimalStore{inner: oban.NewInMemoryStore()}, clock)
	if err := bare.Cancel(ctx, 1); errors.Is(err, oban.ErrUnsupported) {
		fmt.Printf("capability-less store -> %v\n", err)
	} else {
		fmt.Printf("capability-less store -> unexpected: %v\n", err)
	}
}

// minimalStore implements only the required Store methods, hiding the optional
// capabilities of the store it wraps, so ErrUnsupported can be demonstrated.
type minimalStore struct{ inner oban.Store }

func (m *minimalStore) Enqueue(ctx context.Context, j *oban.Job) (*oban.Job, bool, error) {
	return m.inner.Enqueue(ctx, j)
}
func (m *minimalStore) FetchAvailable(ctx context.Context, q string, limit int, now time.Time) ([]*oban.Job, error) {
	return m.inner.FetchAvailable(ctx, q, limit, now)
}
func (m *minimalStore) Complete(ctx context.Context, id int64, now time.Time) error {
	return m.inner.Complete(ctx, id, now)
}
func (m *minimalStore) Retry(ctx context.Context, id int64, at time.Time, lastErr error, now time.Time) error {
	return m.inner.Retry(ctx, id, at, lastErr, now)
}
func (m *minimalStore) Discard(ctx context.Context, id int64, lastErr error, now time.Time) error {
	return m.inner.Discard(ctx, id, lastErr, now)
}
func (m *minimalStore) Get(ctx context.Context, id int64) (*oban.Job, error) {
	return m.inner.Get(ctx, id)
}

// ----------------------------------------------------------------------
// 10. Pausing and scaling a queue
// ----------------------------------------------------------------------

func pauseAndScale(ctx context.Context) {
	section("10. ControlledStore: pause / resume / scale")

	clock := newManualClock(epoch)
	ctrl := oban.NewControlledStore(oban.NewInMemoryStore())
	reg := oban.NewRegistry()
	ran := 0
	reg.RegisterFunc("w", func(ctx context.Context, j *oban.Job) error { ran++; return nil })

	for i := 0; i < 5; i++ {
		j, _ := oban.NewJob("w", map[string]int{"i": i})
		if _, _, err := enqueueAt(ctx, ctrl, clock, j); err != nil {
			log.Fatal(err)
		}
	}

	ctrl.Pause("default")
	fmt.Printf("paused? %v\n", ctrl.IsPaused("default"))
	res, _ := oban.Drain(ctx, ctrl, reg, clock, oban.DrainOptions{Queue: "default"})
	fmt.Printf("drain while paused  -> total=%d (worker ran %d times)\n", res.Total(), ran)

	ctrl.Resume("default")
	ctrl.Scale("default", 2) // cap each fetch batch at 2 jobs
	res, _ = oban.Drain(ctx, ctrl, reg, clock, oban.DrainOptions{Queue: "default"})
	fmt.Printf("resumed, scale=2    -> total=%d (one batch)\n", res.Total())

	res, _ = oban.Drain(ctx, ctrl, reg, clock, oban.DrainOptions{Queue: "default", WithRecursion: true})
	fmt.Printf("recursive drain     -> total=%d; %d jobs ran overall\n", res.Total(), ran)
}

// ----------------------------------------------------------------------
// 11. Plugins: Pruner, Lifeline, CronPlugin under a Supervisor
// ----------------------------------------------------------------------

// storeEnqueuer adapts a Store to the oban.Enqueuer interface the CronPlugin
// needs, so cron can be driven without a running engine.
type storeEnqueuer struct{ store oban.Store }

func (s storeEnqueuer) Enqueue(ctx context.Context, j *oban.Job) (*oban.Job, bool, error) {
	return s.store.Enqueue(ctx, j)
}

func plugins(ctx context.Context) {
	section("11. plugins: Pruner, Lifeline, CronPlugin (Supervisor)")

	clock := newManualClock(epoch)
	store := oban.NewInMemoryStore()

	// Seed: two old finished jobs (prunable) and one job stuck in executing.
	for _, st := range []oban.State{oban.StateCompleted, oban.StateDiscarded} {
		j, _ := oban.NewJob("old", nil)
		s, _, _ := enqueueAt(ctx, store, clock, j)
		switch st {
		case oban.StateCompleted:
			_ = store.Complete(ctx, s.ID, clock.Now())
		default:
			_ = store.Discard(ctx, s.ID, errors.New("old failure"), clock.Now())
		}
	}
	stuck, _ := oban.NewJob("stuck", nil, oban.WithMaxAttempts(5))
	_, _, _ = enqueueAt(ctx, store, clock, stuck)
	fetched, err := store.FetchAvailable(ctx, "default", 10, clock.Now())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("seeded: %v (fetched %d job into executing)\n", statesString(store.States()), len(fetched))

	pruner := oban.NewPruner(store, clock, oban.PrunerConfig{
		Interval: 5 * time.Millisecond,
		MaxAge:   time.Minute,
		States:   []oban.State{oban.StateCompleted, oban.StateDiscarded, oban.StateCancelled},
		Limit:    100,
	})
	lifeline := oban.NewLifeline(store, clock, oban.LifelineConfig{
		Interval:    5 * time.Millisecond,
		RescueAfter: 30 * time.Second,
	})
	cron := oban.NewCronPlugin(storeEnqueuer{store}, clock,
		[]oban.Periodic{
			{Schedule: oban.MustParseCron("*/15 * * * *"), Worker: "refresh_cache"},
			{Schedule: oban.MustParseCronSpec("@hourly"), Worker: "hourly_rollup",
				Options: []oban.JobOption{oban.WithQueue("cron"), oban.WithPriority(2)}},
		}, 5*time.Millisecond)

	sup := oban.NewSupervisor(clock, quiet, pruner, lifeline, cron)
	if err := sup.Start(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("supervisor started: pruner, lifeline, cron")

	// Walk the clock forward in steps so the cron plugin's one-time buildHeap
	// (which seeds "next fire" from the clock) is never jumped over, and so the
	// prune / rescue cutoffs are crossed while the loops are ticking.
	ok := waitFor(5*time.Second, func() bool {
		clock.Advance(10 * time.Minute)
		s := store.States()
		return s[oban.StateCompleted] == 0 && s[oban.StateDiscarded] == 0 &&
			s[oban.StateAvailable] >= 3
	})

	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sup.Stop(shutdown); err != nil {
		log.Fatalf("supervisor stop: %v", err)
	}
	fmt.Printf("plugins converged within the deadline? %v\n", ok)
	fmt.Printf("after plugins: %v\n", statesString(store.States()))

	cronJobs, err := oban.ListJobs(ctx, store, oban.JobFilter{})
	if err != nil {
		log.Fatal(err)
	}
	byWorker := map[string]int{}
	for _, j := range cronJobs {
		byWorker[j.Worker]++
	}
	fmt.Printf("jobs by worker: %v\n", countsString(byWorker))
	fmt.Println("(the two old finished jobs were pruned; the stuck job was rescued to available;")
	fmt.Println(" cron enqueued refresh_cache and hourly_rollup for the elapsed schedule ticks)")

	if err := sup.Stop(shutdown); err != nil {
		fmt.Printf("second Stop -> %v\n", err)
	}
}

// ----------------------------------------------------------------------
// 12. Rate limiter
// ----------------------------------------------------------------------

func rateLimiter() {
	section("12. rate limiter")

	clock := newManualClock(epoch)
	rl := oban.NewRateLimiter(3, time.Second, clock)
	var got []bool
	for i := 0; i < 5; i++ {
		got = append(got, rl.Allow())
	}
	fmt.Printf("burst of 5 against 3/s: %v (tokens left=%d)\n", got, rl.Tokens())

	clock.Advance(time.Second)
	fmt.Printf("after 1s: AllowN(3)=%v then Allow()=%v\n", rl.AllowN(3), rl.Allow())

	clock.Advance(500 * time.Millisecond)
	fmt.Printf("after 500ms (partial refill): tokens=%d Allow()=%v\n", rl.Tokens(), rl.Allow())
}

// ----------------------------------------------------------------------
// 13. Testing helpers
// ----------------------------------------------------------------------

func testingHelpers(ctx context.Context) {
	section("13. testing helpers")

	reg := oban.NewRegistry()
	reg.RegisterFunc("ok", func(ctx context.Context, j *oban.Job) error { return nil })
	reg.RegisterFunc("bad", func(ctx context.Context, j *oban.Job) error { return errors.New("kaput") })
	reg.RegisterFunc("nap", func(ctx context.Context, j *oban.Job) error { return oban.Snooze(time.Minute) })
	reg.RegisterFunc("stop", func(ctx context.Context, j *oban.Job) error { return oban.CancelJob("obsolete") })

	for _, name := range []string{"ok", "bad", "nap", "stop"} {
		j, _ := oban.NewJob(name, nil)
		res, err := oban.PerformJob(ctx, reg, j)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("PerformJob(%-4s) outcome=%-8s err=%-28v snooze=%-4s cancel=%q\n",
			name, res.Outcome, res.Err, res.SnoozeFor, res.CancelReason)
	}

	missing, _ := oban.NewJob("ghost", nil)
	if _, err := oban.PerformJob(ctx, reg, missing); err != nil {
		fmt.Printf("PerformJob(ghost) -> %v\n", err)
	}

	// RunJob bypasses the registry entirely.
	direct := oban.RunJob(ctx, oban.WorkerFunc(func(ctx context.Context, j *oban.Job) error {
		return nil
	}), missing)
	fmt.Printf("RunJob directly   -> %s\n", direct.Outcome)

	// Enqueue assertions and filtered queries.
	store := oban.NewInMemoryStore()
	for i, q := range []string{"default", "default", "mailers"} {
		j, _ := oban.NewJob("ok", map[string]int{"i": i}, oban.WithQueue(q), oban.WithPriority(i))
		if _, _, err := store.Enqueue(ctx, j); err != nil {
			log.Fatal(err)
		}
	}
	yes, _ := oban.AssertEnqueued(ctx, store, oban.JobFilter{Worker: "ok", Queue: "mailers"})
	no, _ := oban.RefuteEnqueued(ctx, store, oban.JobFilter{Worker: "ok", Queue: "nowhere"})
	fmt.Printf("AssertEnqueued(mailers)=%v RefuteEnqueued(nowhere)=%v\n", yes, no)

	minP, maxP := 1, 2
	n, _ := oban.CountJobs(ctx, store, oban.JobFilter{
		States:      []oban.State{oban.StateAvailable},
		MinPriority: &minP,
		MaxPriority: &maxP,
	})
	fmt.Printf("CountJobs(priority 1..2, available)=%d\n", n)

	listed, _ := oban.ListJobs(ctx, store, oban.JobFilter{Queue: "default"})
	fmt.Printf("ListJobs(default) ids=%v\n", ids(listed))

	fmt.Printf("Len=%d states=%v\n", store.Len(), statesString(store.States()))
	store.Clear()
	fmt.Printf("after Clear: Len=%d\n", store.Len())
}

// ----------------------------------------------------------------------
// 14. Per-worker configuration + WorkerMiddleware
// ----------------------------------------------------------------------

// reportWorker advertises its own queue, attempt cap, priority and per-attempt
// timeout via ConfiguredWorker.
type reportWorker struct {
	calls int
}

func (w *reportWorker) Config() oban.WorkerOptions {
	return oban.WorkerOptions{
		Queue:       "reports",
		MaxAttempts: 4,
		Priority:    2,
		Timeout:     25 * time.Millisecond,
		Tags:        []string{"reporting"},
		Backoff:     oban.NewConstantBackoff(90 * time.Second),
	}
}

func (w *reportWorker) Perform(ctx context.Context, j *oban.Job) error {
	w.calls++
	// Work for 60ms, which is well past the worker's own 25ms Timeout. If
	// WorkerMiddleware applied that budget we would observe ctx.Err() here.
	select {
	case <-ctx.Done():
		return fmt.Errorf("hit the per-worker timeout: %w", ctx.Err())
	case <-time.After(60 * time.Millisecond):
		return errors.New("report backend unavailable")
	}
}

func workerConfig(ctx context.Context) {
	section("14. ConfiguredWorker, BuildJob and WorkerMiddleware")

	w := &reportWorker{}
	job, err := oban.BuildJob("report", w, map[string]string{"span": "daily"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("BuildJob picked up worker defaults: queue=%s max_attempts=%d priority=%d\n",
		job.Queue, job.MaxAttempts, job.Priority)

	override, _ := oban.BuildJob("report", w, nil, oban.WithQueue("urgent"), oban.WithPriority(0))
	fmt.Printf("caller options win           : queue=%s priority=%d\n", override.Queue, override.Priority)

	// A plain worker with no Config() keeps the package defaults.
	plain, _ := oban.BuildJob("plain", oban.WorkerFunc(func(context.Context, *oban.Job) error { return nil }), nil)
	fmt.Printf("plain worker                 : queue=%s max_attempts=%d\n", plain.Queue, plain.MaxAttempts)

	// Run it through the engine with WorkerMiddleware installed. Per its
	// documentation the worker's own 25ms Timeout should bound the attempt and
	// its per-worker Backoff (90s) should reschedule the job instead of the
	// engine's global 1h policy. Neither happens — see the HOLE note below.
	clock := newManualClock(epoch)
	store := oban.NewInMemoryStore()
	engine, err := oban.New(oban.Config{
		Store:        store,
		Clock:        clock,
		Queues:       map[string]int{"reports": 1},
		PollInterval: 2 * time.Millisecond,
		JobTimeout:   5 * time.Second, // deliberately looser than the worker's 25ms
		Middleware:   []oban.Middleware{oban.WorkerMiddleware(store, clock)},
		Backoff:      oban.NewConstantBackoff(time.Hour),
		Logger:       quiet,
	})
	if err != nil {
		log.Fatal(err)
	}
	engine.Register("report", w)

	stored, _, err := engine.Enqueue(ctx, job)
	if err != nil {
		log.Fatal(err)
	}
	if err := engine.Start(ctx); err != nil {
		log.Fatal(err)
	}
	ok := waitFor(3*time.Second, func() bool {
		j, err := store.Get(ctx, stored.ID)
		return err == nil && j.State == oban.StateRetryable
	})
	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := engine.Stop(shutdown); err != nil {
		log.Fatal(err)
	}
	final, _ := store.Get(ctx, stored.ID)
	fmt.Printf("job reached retryable? %v\n", ok)
	fmt.Printf("state=%s attempt=%d last_error=%q\n", final.State, final.Attempt, final.LastError)
	fmt.Printf("rescheduled %s out\n", final.ScheduledAt.Sub(clock.Now()))
	// HOLE: WorkerMiddleware recovers the running Worker from the attempt
	// context, but nothing in the engine ever puts it there
	// (worker_configWithWorker is referenced only by the package's own tests).
	// So WorkerOptions.Timeout and WorkerOptions.Backoff are unreachable:
	// the 60ms attempt was never cut off at 25ms, and the retry used the
	// engine's 1h Backoff rather than the worker's 90s one.
	if !strings.Contains(final.LastError, "per-worker timeout") {
		fmt.Println("^ HOLE: WorkerOptions.Timeout was not applied (see README)")
	}
	if final.ScheduledAt.Sub(clock.Now()) != 90*time.Second {
		fmt.Println("^ HOLE: WorkerOptions.Backoff was not applied; the engine policy won")
	}

	// Snooze/cancel signal helpers.
	d, isSnooze := oban.IsSnooze(fmt.Errorf("wrapped: %w", oban.Snooze(2*time.Minute)))
	fmt.Printf("IsSnooze(wrapped)=%v for=%s\n", isSnooze, d)
	reason, isCancel := oban.IsCancel(fmt.Errorf("wrapped: %w", oban.CancelJob("done")))
	fmt.Printf("IsCancel(wrapped)=%v reason=%q\n", isCancel, reason)
}

// ----------------------------------------------------------------------
// 15. SQLStore: what is and is not reachable without a database
// ----------------------------------------------------------------------

func sqlStoreReality() {
	section("15. SQLStore (no database available here)")

	for _, d := range []oban.SQLDialect{oban.Postgres, oban.SQLite, oban.MySQL} {
		fmt.Printf("dialect %d -> %q\n", int(d), d.String())
	}
	fmt.Printf("DefaultSQLTable = %q\n", oban.DefaultSQLTable)

	// A nil *sql.DB is enough to inspect which interfaces the type satisfies.
	s := oban.NewSQLStore(nil, oban.SQLite, oban.WithTableName("jobs"))

	var asStore oban.Store = s
	fmt.Printf("*SQLStore satisfies Store            : %v\n", asStore != nil)

	checks := []struct {
		name string
		ok   bool
	}{
		{"PrunableStore  (Pruner)", isPrunable(asStore)},
		{"RescuableStore (Lifeline)", isRescuable(asStore)},
		{"UniqueStore    (Insert/UniqueBy)", isUnique(asStore)},
		{"CancelableStore(Controller)", isCancelable(asStore)},
		{"DeletableStore (Controller)", isDeletable(asStore)},
		{"RetryableStore (Controller)", isRetryable(asStore)},
		{"TaggableStore  (Insert tags)", isTaggable(asStore)},
		{"SnoozableStore (Snooze)", isSnoozable(asStore)},
		{"ListableStore  (ListJobs)", isListable(asStore)},
	}
	inmem := oban.Store(oban.NewInMemoryStore())
	memChecks := []bool{isPrunable(inmem), isRescuable(inmem), isUnique(inmem), isCancelable(inmem),
		isDeletable(inmem), isRetryable(inmem), isTaggable(inmem), isSnoozable(inmem), isListable(inmem)}

	fmt.Printf("%-34s %-10s %-10s\n", "capability", "SQLStore", "InMemory")
	for i, c := range checks {
		fmt.Printf("%-34s %-10v %-10v\n", c.name, c.ok, memChecks[i])
	}
	fmt.Println("HOLE: SQLStore's doc comment claims it satisfies all of these, but its")
	fmt.Println("      DeleteFinishedBefore/RescueExecuting/FindConflict signatures differ")
	fmt.Println("      from the interfaces, so Pruner/Lifeline/UniqueBy silently do not")
	fmt.Println("      work with it. See README.")
	fmt.Println("HOLE: exercising SQLStore at all needs a third-party database/sql driver,")
	fmt.Println("      which the module cannot provide (it is stdlib-only), so no SQL-backed")
	fmt.Println("      persistence is demonstrated here.")
}

func isPrunable(s oban.Store) bool   { _, ok := s.(oban.PrunableStore); return ok }
func isRescuable(s oban.Store) bool  { _, ok := s.(oban.RescuableStore); return ok }
func isUnique(s oban.Store) bool     { _, ok := s.(oban.UniqueStore); return ok }
func isCancelable(s oban.Store) bool { _, ok := s.(oban.CancelableStore); return ok }
func isDeletable(s oban.Store) bool  { _, ok := s.(oban.DeletableStore); return ok }
func isRetryable(s oban.Store) bool  { _, ok := s.(oban.RetryableStore); return ok }
func isTaggable(s oban.Store) bool   { _, ok := s.(oban.TaggableStore); return ok }
func isSnoozable(s oban.Store) bool  { _, ok := s.(oban.SnoozableStore); return ok }
func isListable(s oban.Store) bool   { _, ok := s.(oban.ListableStore); return ok }

// ----------------------------------------------------------------------
// small helpers
// ----------------------------------------------------------------------

// enqueueAt stamps the job's insertion time from the supplied clock before
// enqueuing. Without it InMemoryStore.Enqueue falls back to time.Now(), which
// would put ScheduledAt beyond a manual clock and make the job unfetchable.
func enqueueAt(ctx context.Context, store oban.Store, clock oban.Clock, j *oban.Job) (*oban.Job, bool, error) {
	j.InsertedAt = clock.Now()
	return store.Enqueue(ctx, j)
}

// waitFor polls cond every millisecond until it holds or the deadline passes.
// It guarantees the example never blocks indefinitely on a worker loop.
func waitFor(limit time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return cond()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

func ids(jobs []*oban.Job) []int64 {
	out := make([]int64, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, j.ID)
	}
	return out
}

func statesString(m map[oban.State]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[oban.State(k)]))
	}
	return strings.Join(parts, " ")
}

func eventCounts(m map[oban.EventName]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[oban.EventName(k)]))
	}
	return strings.Join(parts, " ")
}

func countsString(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, " ")
}
