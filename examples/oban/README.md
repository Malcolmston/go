# oban example

A runnable validation program for the **published** module
`github.com/malcolmston/oban`, consumed exactly as an outside user would (no
`replace` directive).

Resolved version: **`v0.0.0-20260719021426-5168b7eb3e6a`** (pseudo-version — the
repo has no semver tags). The published Go sources are byte-identical to the
local working tree at the time of writing.

Everything runs in process against the shipped `InMemoryStore`: no database, no
network, and every wait is deadline-bounded, so the program always terminates on
its own (about half a second).

## Run

```sh
cd examples/oban
GOWORK=off go get github.com/malcolmston/oban@latest
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

## What it demonstrates

1. **Jobs and options** — `NewJob` with `WithQueue`, `WithMaxAttempts`,
   `WithPriority`, `WithScheduleIn`, `WithUnique`; `UnmarshalArgs`, `Clone`,
   defaults, the empty-worker error, and the query helpers `IsFinished`,
   `InState`, `AttemptsRemaining`, `HasErrored`, `ExecutionDuration`, `Age`,
   `ValidatePriority`.
2. **Cron** — `MustParseCron` / `ParseCron` for steps, lists, ranges and
   day/month names; `Next`, `Matches`, `Upcoming`, `Prev`; the `@hourly` …
   `@yearly` macros via `MustParseCronSpec`; both error paths; and the Vixie
   "either day field matches" rule (`0 0 13 * fri`).
3. **Backoff** — a delay table for attempts 1–7 across `ExponentialBackoff`
   (with and without jitter), `ConstantBackoff`, `LinearBackoff`,
   `FibonacciBackoff` and a custom `BackoffFunc`, plus proof that a seeded
   jitter source is reproducible.
4. **`Drain` (deterministic, no goroutines)** — with a manual `Clock`:
   a job retried twice then discarded (with the full `Errors` history printed),
   a fail-then-succeed job, `Snooze` (attempt *not* consumed), `CancelJob`,
   an unregistered worker (counted `Failed`), a panicking worker converted to an
   error, middleware ordering (`mw[0]` outermost), and a per-attempt
   `Timeout` firing.
5. **The live engine** — two queues at different concurrency, `PollInterval`,
   `QueueTimeouts`, priority ordering observed on a single-slot queue,
   `EnqueueMany`, a job exhausting its attempts, `Telemetry` hooks, a
   `Recorder` installed as middleware (event counts reconcile with 13 attempts
   over 11 jobs), `ErrorHandler`, graceful `Stop`, and the `New`/`Start`
   validation errors.
6. **Unique jobs** — dedup on `(queue, worker, key)`, the period lapsing under
   a manual clock, a finished holder no longer blocking, and a distinct key.
7. **Rich `Insert`** — `UniqueBy` over `worker+queue+args` and over
   `worker` + explicit `Keys`, `Tags`/`Meta` persisted and read back through
   `TaggableStore`, priority validation, the nil-job error, and `InsertMany`.
8. **Scheduled / delayed jobs** — `WithScheduleIn` and `WithScheduledAt`,
   state normalisation to `scheduled`, and three drains at advancing clock
   positions plus `WithScheduled` to ignore the schedule entirely.
9. **Operator control** — `Controller.Cancel` / `Retry` / `Delete`,
   `ErrJobNotFound`, and `ErrUnsupported` from a store that hides the optional
   capabilities.
10. **`ControlledStore`** — `Pause` (drain does nothing), `Resume`, `Scale`
    (batch-size cap observable across one vs. recursive drains).
11. **Plugins** — `Pruner`, `Lifeline` and `CronPlugin` under a `Supervisor`,
    driven by a manual clock: two old finished jobs are pruned, a job orphaned
    in `executing` is rescued to `available`, and cron enqueues
    `refresh_cache` / `hourly_rollup` for the elapsed ticks. Double `Stop` is a
    no-op.
12. **`RateLimiter`** — token bucket burst, exhaustion, refill and partial
    refill under a manual clock.
13. **Testing helpers** — `PerformJob` / `RunJob` classifying complete / error /
    snooze / cancel, `AssertEnqueued` / `RefuteEnqueued`, `CountJobs` and
    `ListJobs` with a `JobFilter` (queue, state, priority range), plus
    `InMemoryStore.Len` / `States` / `CountByQueue` / `Clear`.
14. **Per-worker configuration** — `ConfiguredWorker` + `BuildJob` (worker
    defaults, caller overrides win, plain workers unaffected), `WorkerMiddleware`
    in a live engine, and `IsSnooze` / `IsCancel` on wrapped errors.
15. **`SQLStore`** — dialect constants, `DefaultSQLTable`, and a runtime
    capability matrix comparing `*SQLStore` with `*InMemoryStore`.

## Holes found

### 1. `SQLStore` does not implement the capability interfaces it claims to

`sqlstore.go`'s type doc says:

> SQLStore satisfies Store and, in addition, the optional capability interfaces
> the plugins, control and insert areas type-assert for: pruning, rescuing,
> cancelling, deleting, forcing a retry, unique-conflict lookup and tag/meta
> updates.

It does not. Three signatures disagree with the interfaces, and because every
consumer discovers them with a **type assertion**, the mismatch is invisible at
compile time and the features simply never activate:

| interface | required | `*SQLStore` has |
| --- | --- | --- |
| `PrunableStore` | `DeleteFinishedBefore(ctx, states []State, cutoff time.Time, limit int) (int64, error)` | `DeleteFinishedBefore(ctx, cutoff time.Time) (int64, error)` |
| `RescuableStore` | `RescueExecuting(ctx, olderThan, now) (rescued, discarded int64, err error)` | `RescueExecuting(ctx, olderThan, now) (int64, error)` |
| `UniqueStore` | `FindConflict(ctx, job *Job, by UniqueBy, now time.Time) (*Job, error)` | `FindConflict(ctx, job *Job, now time.Time) (*Job, error)` |

`TaggableStore` also fails, because `*SQLStore` has `SetTagsMeta` but no
`TagsMeta` reader; `SnoozableStore` and `ListableStore` are simply absent.
Section 15 of the example prints the full matrix (a nil `*sql.DB` is enough).

Consequences for anyone using the SQL store: `Pruner` and `Lifeline` cannot be
constructed with it (compile error at `NewPruner(store, …)`), and `Insert` with
`InsertOpts.Unique` silently falls back to single-key dedup, throwing away the
requested `Fields`/`States`. `oban.Tags` always returns nil.
`Controller.Cancel/Retry/Delete` are the only optional capabilities that do work.

### 2. `WorkerOptions.Timeout` and `WorkerOptions.Backoff` are unreachable

`WorkerMiddleware` recovers the executing worker from the attempt context:

```go
if w, ok := worker_configWorkerFromContext(ctx); ok { … }
```

and its doc claims *"The engine wiring attaches the resolved worker to the
attempt context with it"*. It does not: `worker_configWithWorker` is referenced
**only from `worker_config_test.go`**. `Oban.execute` builds
`context.WithTimeout(o.jobCtx, o.timeoutFor(job.Queue))` and never adds the
worker. So in a real engine the lookup always fails and `opts` stays nil,
meaning:

- `WorkerOptions.Timeout` never bounds an attempt, and
- `WorkerOptions.Backoff` never applies; the engine's global `Backoff` wins.

Section 14 shows this: a worker declaring `Timeout: 25ms` and
`Backoff: Constant(90s)` runs its full 60ms body and is then rescheduled **1h**
out (the engine's policy). `WorkerOptions` therefore only affects `BuildJob`
(queue / max-attempts / priority). The snooze and cancel halves of
`WorkerMiddleware` *do* work, since they only inspect the returned error.

### 3. `InsertOpts.Replace` silently does nothing for most fields

`insertApplyReplace` mutates the `*Job` that `FindConflict` returned:

```go
case ReplacePriority:
    conflict.Priority = incoming.Priority
```

But `Store`'s own contract says *"Returned jobs should be copies (or otherwise
immutable) so callers cannot mutate stored state by reference"*, and
`InMemoryStore.FindConflict` duly returns `existing.Clone()`. Nothing is ever
written back — the `Store` interface has no update method at all. So
`ReplaceScheduledAt`, `ReplacePriority`, `ReplaceMaxAttempts` and `ReplaceArgs`
are no-ops for any conforming store, with no error reported. Only
`ReplaceTags` / `ReplaceMeta` persist, because they route through
`TaggableStore.SetTagsMeta`.

Section 7 demonstrates both halves: the priority/max-attempts/scheduled-at
replace is dropped, the tag replace lands.

### 4. `SQLStore` needs a third-party driver, so SQL persistence is undemonstrable

The module is deliberately stdlib-only and never imports a driver, which is the
right design — but it means there is no way to exercise `SQLStore.Migrate` or
any SQL code path from an example that must not add dependencies. Nothing here
covers the SQL store's actual behaviour; only its interface surface is checked.
Recorded as a hole in coverage rather than a defect.

### 5. Usability friction

- **`InMemoryStore.Enqueue` ignores the engine `Clock`.** It falls back to
  `time.Now()` when `Job.InsertedAt` is zero, and then derives `ScheduledAt`
  from that. With an injected `Clock` set in the past, jobs land with a
  `ScheduledAt` the clock will never reach, so `FetchAvailable` and `Drain`
  return nothing and the caller sees a silent no-op. The example needs an
  `enqueueAt` helper that stamps `InsertedAt` from the clock before every
  enqueue. The `Store` interface should take `now` on `Enqueue` the way every
  other method does.
- **`CronPlugin` seeds its heap lazily inside the loop goroutine** (`setup =
  buildHeap`), so a test that starts the supervisor and then jumps a manual
  clock can race: if `buildHeap` runs after the jump, the next fire time is
  computed from the new time and nothing ever fires. The example has to advance
  the clock in small steps inside its poll loop.
- **`Job.scheduleIn` is unexported**, so `WithScheduleIn` only resolves inside a
  `Store.Enqueue` implementation. A third-party store written outside the
  package cannot honour it at all — it will see a zero `ScheduledAt` and treat
  the job as immediately available.
- **The `WorkerMiddleware` per-worker retry path abuses `RetryNow`**: it calls
  `RetryNow(ctx, id, now+delay)`, but `RetryNow` is documented as "makes the job
  runnable again as of now" and sets `state = available`, not `retryable`, and
  records no error. Even if hole 2 were fixed, per-worker backoff would produce
  a different job state and lose the error history compared with the engine's
  own retry.
- `InsertResult` has no error field even though `InsertMany` documents stopping
  "at the first error"; the error comes back separately and the results slice is
  truncated, which is fine but easy to misread.

### Not holes (verified working)

- Retry → backoff → discard, with a complete `Errors` history and `LastError`.
- `Snooze` really does hand the attempt back (`Attempt` stays 1).
- Priority ordering on a single-slot queue: `p0 p1 p5 p9`, ties by insertion.
- Middleware ordering matches the docs (`mw[0]` outermost) in both the engine
  and `Drain`.
- Panicking workers are converted to errors, not crashes.
- Unique dedup, period expiry, and finished jobs releasing the key.
- `Pruner`, `Lifeline` and `CronPlugin` all work correctly against
  `InMemoryStore` under a manual clock.
- `Controller` degrades to `ErrUnsupported` rather than panicking.
- Graceful `Stop` drains in-flight work; a second `Start` is rejected.
- `go mod tidy` added no transitive dependencies.
