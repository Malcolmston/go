# quartz example

A single runnable program that exercises the public API of
[`github.com/malcolmston/quartz`](https://github.com/malcolmston/quartz), a
Quartz-style in-process job scheduler.

**Module under test:** `github.com/malcolmston/quartz v0.0.0-20260719012946-caf334802f9e`
(resolved by `go get github.com/malcolmston/quartz@latest`; the repo has no
semver tags, so `@latest` yields a pseudo-version). The example consumes the
published module — there is no `replace` directive.

## Run it

```sh
cd examples/quartz
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

It makes no network calls beyond the module download, needs no database or
external store, and always terminates (a few hundred milliseconds). It also runs
clean under `GOWORK=off go run -race .`.

Every computed value is compared against a **hand-calculated** expectation and
printed as `PASS` / `FAIL`; a summary at the end lists any divergences.

## What it demonstrates

| Section | Covers |
| --- | --- |
| 1 | `ParseCron` / `MustParseCron`: accepted syntax (ranges, steps, lists, names, `?`, 5- and 6-field forms) and rejected syntax, plus `CronExpression.IsSatisfiedBy` |
| 2 | `CronExpression.Next` against fixed reference timestamps: daily, `*/15`, `MON-FRI`, leap day, impossible schedule (`Feb 30` → `<never>`), Unix OR day semantics, month names, Sunday as both `0` and `7`, evaluation in `America/New_York` |
| 3 | `CronTrigger`: `ComputeFirstFireTime`, `Triggered`, `PreviousFireTime`, `StartingAt`/`EndingAt` window, `In(loc)` including a DST transition |
| 4 | `SimpleTrigger`: interval + `repeatCount` (fires `repeatCount+1` times), `RepeatForever` with `WithEndTime`, the pure `FireTimeAfter` query, `TimesTriggered` |
| 5 | `CalendarIntervalTrigger`: month / quarter / week / year advancement, `FireTimeAfter`, end-of-month behaviour |
| 6 | `DailyTimeIntervalTrigger`: intra-day window with an hourly step, weekday selection, bounded repeat count |
| 7 | Fluent builders `NewJob()` / `NewTrigger()` / `SimpleSchedule` / `CronSchedule` / `CronScheduleDailyAt`/`WeeklyOn`/`MonthlyOn`, `JobDataMap.GetString`/`GetInt`, builder validation errors, and the `Matcher` combinators |
| 8 | Calendars: `HolidayCalendar`, `WeeklyCalendar`, `DailyCalendar` (+ `Invert`), `CronCalendar`, `AnnualCalendar`, base-calendar chaining, and the `WithCalendar` trigger decorator skipping excluded fire times |
| 9 | `Scheduler` driven **deterministically** by `Options.Clock` + `ProcessDue` (no sleeping, no background goroutines): job data / `JobExecutionContext.MergedData` passing, a `JobListener`, a `TriggerListener` that vetoes, a deliberately failing job whose wrapped error reaches the listener via `jec.Result`, `PauseTrigger`/`ResumeTrigger`/`PauseJob`/`ResumeJob`, `TriggerJob` (out-of-band fire), `UnscheduleJob` with non-durable job cascade, `AddJob` for durable jobs, `DeleteJob`, and the error paths (`ErrJobNotFound`, `ErrTriggerNotFound`, `ErrSchedulerShutdown`, mismatched keys) |
| 10 | All four `MisfirePolicy` values via `UpdateAfterMisfire` on a fixed timeline, plus end-to-end through the scheduler: `MisfireIgnore` replays the four missed fires, `MisfireDoNothing` drops them |
| 11 | Real-time execution: `Concurrency: 4` worker pool, 20 ms / 25 ms intervals, a job that always fails, then `Shutdown(true)` graceful drain (and a second `Shutdown` as a no-op) |
| 12 | `Shutdown(false)`: the in-flight job's context is cancelled and shutdown returns immediately |

## Holes and rough edges found

1. **`CalendarIntervalTrigger` month/year overflow instead of clamping (parity
   divergence — reported as the two `FAIL`s the program prints).** The port
   advances with `time.Time.AddDate`, which overflows: `2026-01-31 + 1 month`
   becomes **2026-03-03**, and `2028-02-29 + 1 year` becomes **2029-03-01**.
   Java Quartz uses `java.util.Calendar.add(MONTH|YEAR)`, which clamps to the
   last valid day of the target month (`2026-02-28`, `2029-02-28`). A monthly
   job scheduled on the 29th–31st therefore drifts and can skip a month
   entirely. The example deliberately asserts the Java values so the divergence
   shows up in output rather than being papered over.
2. **`On*` weekday convenience methods exist only on the schedule builder.**
   `DailyTimeIntervalScheduleBuilder` has `OnEveryDay`,
   `OnMondayThroughFriday` and `OnSaturdayAndSunday`, but
   `DailyTimeIntervalTrigger` itself only has `OnDaysOfWeek(...)`. Using the
   trigger constructor directly (as this example does) means spelling out
   `time.Monday, ... time.Friday` by hand. Symmetric methods on the trigger
   would be nicer.
3. **`Scheduler.dispatch` runs jobs inline when the scheduler is not started.**
   That is what makes section 9 fully deterministic, and it is documented on
   `TriggerJob` — but it is surprising: `ProcessDue` on a *stopped* scheduler
   executes user code synchronously on the caller's goroutine, so a job that
   blocks will block `ProcessDue`. It also means listener callbacks fire on the
   caller's goroutine in that mode.
4. **Listener registration is documented as "must be called before Start" but
   is not enforced.** `AddJobListener` / `AddTriggerListener` take the mutex and
   append at any time, and `ProcessDue`/`execute` read the slices without it,
   so adding a listener after `Start` is a latent data race that the API
   silently permits rather than rejecting with `ErrSchedulerRunning` — which is
   exported and documented as "returned by operations that are not allowed while
   the scheduler is running" but is never actually returned anywhere in the
   package (its only occurrence is its own declaration).
5. **`ProcessDue` cannot replay more than one missed fire per call.**
   `MemoryJobStore.AcquireNextTriggers` returns each trigger at most once per
   call, so catching up N missed fires under `MisfireIgnore` requires N
   `ProcessDue` calls (section 10 loops six times to replay four fires). With
   the real-time loop this is bounded by `PollInterval`, but it is a
   non-obvious coupling between store semantics and misfire behaviour.
6. **Misfire threshold interacts badly with sub-second schedules.** The default
   `MisfireThreshold` is 5 s; a virtual-clock test that jumps hours, or a
   trigger whose start time is in the past, immediately trips misfire handling.
   Sections 9 and 11 have to set `MisfireThreshold: time.Hour` to get the
   straightforward behaviour. A "misfire only if the trigger was actually
   eligible" rule would be less surprising.
7. **`CronCalendar` is a blocklist, not an allowlist.** `NewCronCalendar(expr)`
   *excludes* instants matching `expr`. This matches Java Quartz and is
   documented, but the name reads like the opposite; the example asserts the
   exclusion semantics explicitly.
8. **`Scheduler.notifyMisfire` is a no-op stub** with no exported hook, so there
   is no way to observe misfires. `TriggerListener` has no `TriggerMisfired`
   callback (Java Quartz does), meaning silently-dropped fires under
   `MisfireDoNothing` are unobservable.
9. **`Trigger.Triggered(now)` ignores `now` in every implementation** (it
   advances from the stored `next`). Harmless, but the interface implies
   otherwise.

### Non-issues verified

- No data races reported by `-race` across the real-time worker-pool sections.
- No goroutine leak or hang: `Shutdown(true)` drains and `Shutdown(false)`
  cancels the job context and returns in ~0 ms; a second `Shutdown` is a safe
  no-op (it does not double-close the work channel).
- README/doc.go examples match the real API — `NewScheduler(Options{...})`,
  `NewJobDetail`, `JobFunc`, `NewCronTriggerWithKeys`, `NewSimpleTrigger`,
  `RepeatForever`, `ScheduleJob`, `Start`, `Shutdown(true)`, and the injected
  clock + `ProcessDue` recipe all compile and behave as documented.
- The module has no third-party dependencies, so `go mod tidy` pulls nothing
  else.
