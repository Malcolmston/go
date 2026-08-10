// Command quartz-example exercises the github.com/malcolmston/quartz job
// scheduler: cron parsing and next-fire-time arithmetic against fixed
// reference timestamps, simple/calendar/daily-time-interval triggers, the
// fluent builders, calendars, deterministic firing through an injected clock,
// pause/resume/unschedule, listeners with veto, real-time execution with a
// worker pool, failing jobs, and both graceful and cancelling shutdown.
//
// It makes no network calls, needs no external store, and always terminates.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/malcolmston/quartz"
)

// ---------------------------------------------------------------------------
// verification helpers
// ---------------------------------------------------------------------------

var mismatches []string

const stamp = "2006-01-02 15:04:05 MST"

// check compares a computed time against a hand-calculated expectation.
func check(label string, got, want time.Time) {
	gs, ws := fmtTime(got), fmtTime(want)
	if got.Equal(want) {
		fmt.Printf("    PASS  %-46s %s\n", label, gs)
		return
	}
	fmt.Printf("    FAIL  %-46s got %s want %s\n", label, gs, ws)
	mismatches = append(mismatches,
		fmt.Sprintf("%s: got %s, hand-calculated %s", label, gs, ws))
}

func checkBool(label string, got, want bool) {
	if got == want {
		fmt.Printf("    PASS  %-46s %t\n", label, got)
		return
	}
	fmt.Printf("    FAIL  %-46s got %t want %t\n", label, got, want)
	mismatches = append(mismatches, fmt.Sprintf("%s: got %t, want %t", label, got, want))
}

func checkInt(label string, got, want int) {
	if got == want {
		fmt.Printf("    PASS  %-46s %d\n", label, got)
		return
	}
	fmt.Printf("    FAIL  %-46s got %d want %d\n", label, got, want)
	mismatches = append(mismatches, fmt.Sprintf("%s: got %d, want %d", label, got, want))
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return "<never>"
	}
	return t.Format(stamp)
}

func section(title string) {
	fmt.Printf("\n=== %s ===\n", title)
}

func utc(y int, mo time.Month, d, h, mi, s int) time.Time {
	return time.Date(y, mo, d, h, mi, s, 0, time.UTC)
}

// ---------------------------------------------------------------------------
// listeners
// ---------------------------------------------------------------------------

// auditListener records every execution and reports failures.
type auditListener struct {
	quartz.BaseJobListener
	mu       sync.Mutex
	started  int
	finished int
	failed   int
	errs     []string
}

func (a *auditListener) JobToBeExecuted(_ context.Context, jec *quartz.JobExecutionContext) {
	a.mu.Lock()
	a.started++
	a.mu.Unlock()
}

func (a *auditListener) JobWasExecuted(_ context.Context, jec *quartz.JobExecutionContext) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.finished++
	if jec.Result != nil {
		a.failed++
		a.errs = append(a.errs, fmt.Sprintf("%s @ %s: %v",
			jec.Detail.Key(), fmtTime(jec.FireTime), jec.Result))
	}
}

func (a *auditListener) snapshot() (started, finished, failed int, errs []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.started, a.finished, a.failed, append([]string(nil), a.errs...)
}

// vetoListener blocks any trigger whose name starts with "veto-".
type vetoListener struct {
	quartz.BaseTriggerListener
	mu     sync.Mutex
	fired  int
	vetoed int
}

func (v *vetoListener) TriggerFired(_ context.Context, t quartz.Trigger, _ *quartz.JobExecutionContext) {
	v.mu.Lock()
	v.fired++
	v.mu.Unlock()
}

func (v *vetoListener) VetoJobExecution(_ context.Context, t quartz.Trigger, _ *quartz.JobExecutionContext) bool {
	if strings.HasPrefix(t.Key().Name, "veto-") {
		v.mu.Lock()
		v.vetoed++
		v.mu.Unlock()
		return true
	}
	return false
}

func (v *vetoListener) counts() (int, int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.fired, v.vetoed
}

// counterJob is a Job implementation (not just a JobFunc) that reads its
// per-execution data out of the merged JobDataMap the scheduler hands it.
type counterJob struct {
	mu    sync.Mutex
	runs  int
	seen  []string
	label string
}

func (c *counterJob) Execute(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runs++
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	c.seen = append(c.seen, fmt.Sprintf("%s#%d", c.label, c.runs))
	return nil
}

func (c *counterJob) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runs
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	cronParsing()
	cronNextFireTimes()
	cronTriggerLifecycle()
	simpleTriggerArithmetic()
	calendarIntervalTrigger()
	dailyTimeIntervalTrigger()
	fluentBuilders()
	calendarExclusions()
	deterministicScheduler()
	misfirePolicies()
	realTimeExecution()
	cancellingShutdown()

	section("SUMMARY")
	if len(mismatches) == 0 {
		fmt.Println("  every computed value matched its hand-calculated expectation")
	} else {
		fmt.Printf("  %d divergence(s) from hand-calculated (Java-Quartz-parity) values:\n", len(mismatches))
		for _, m := range mismatches {
			fmt.Printf("    - %s\n", m)
		}
	}
	fmt.Println("\ndone.")
	if len(mismatches) > 0 {
		// Still exit 0: the mismatches are the report, not a crash.
		_ = os.Stdout.Sync()
	}
}

// ---------------------------------------------------------------------------
// 1. cron parsing
// ---------------------------------------------------------------------------

func cronParsing() {
	section("1. Cron parsing (quartz.ParseCron)")

	good := []string{
		"0 0 10 * * *",
		"*/15 * * * * *",
		"0 30 9 * * MON-FRI",
		"0 0,15,30,45 * * * ?",
		"0 10-30/5 * * * *",
		"0 0 12 1 JAN *",
		"30 9 * * *", // five-field form, seconds defaulted to 0
	}
	for _, expr := range good {
		ce, err := quartz.ParseCron(expr)
		if err != nil {
			fmt.Printf("  UNEXPECTED ERROR %-24q %v\n", expr, err)
			mismatches = append(mismatches, fmt.Sprintf("ParseCron(%q) rejected a valid expression: %v", expr, err))
			continue
		}
		fmt.Printf("  ok      %-24q -> %s\n", expr, ce)
	}

	bad := []string{
		"",
		"* * *",
		"0 0 99 * * *",
		"0 0 10 * * MONDAY",
		"0 */0 * * * *",
		"0 0 10 * * * *",
	}
	for _, expr := range bad {
		if _, err := quartz.ParseCron(expr); err != nil {
			fmt.Printf("  reject  %-24q -> %v\n", expr, err)
		} else {
			fmt.Printf("  ACCEPTED INVALID %-24q\n", expr)
			mismatches = append(mismatches, fmt.Sprintf("ParseCron(%q) accepted an invalid expression", expr))
		}
	}

	// IsSatisfiedBy is a whole-second predicate.
	daily10 := quartz.MustParseCron("0 0 10 * * *")
	checkBool("IsSatisfiedBy(2026-01-01 10:00:00Z)", daily10.IsSatisfiedBy(utc(2026, 1, 1, 10, 0, 0)), true)
	checkBool("IsSatisfiedBy(2026-01-01 10:00:01Z)", daily10.IsSatisfiedBy(utc(2026, 1, 1, 10, 0, 1)), false)
}

// ---------------------------------------------------------------------------
// 2. cron next-fire-time arithmetic against fixed reference timestamps
// ---------------------------------------------------------------------------

func cronNextFireTimes() {
	section("2. CronExpression.Next against FIXED reference timestamps")
	fmt.Println("  (expectations below are hand-calculated, not read back from the library)")

	// Daily at 10:00.
	daily := quartz.MustParseCron("0 0 10 * * *")
	check("'0 0 10 * * *' after 2026-01-01 09:59:00Z",
		daily.Next(utc(2026, 1, 1, 9, 59, 0)), utc(2026, 1, 1, 10, 0, 0))
	// Next is strictly after, so 10:00:00 rolls to the following day.
	check("'0 0 10 * * *' after 2026-01-01 10:00:00Z",
		daily.Next(utc(2026, 1, 1, 10, 0, 0)), utc(2026, 1, 2, 10, 0, 0))

	// Every 15 seconds.
	q15 := quartz.MustParseCron("*/15 * * * * *")
	check("'*/15 * * * * *' after 2026-03-01 12:00:07Z",
		q15.Next(utc(2026, 3, 1, 12, 0, 7)), utc(2026, 3, 1, 12, 0, 15))
	check("'*/15 * * * * *' after 2026-03-01 12:00:45Z",
		q15.Next(utc(2026, 3, 1, 12, 0, 45)), utc(2026, 3, 1, 12, 1, 0))

	// Weekdays at 09:30. 2026-01-01 is a Thursday, so 01-02 is Friday and the
	// next weekday fire after Friday 09:30 is Monday 2026-01-05 09:30.
	weekday := quartz.MustParseCron("0 30 9 * * MON-FRI")
	check("'0 30 9 * * MON-FRI' after Fri 2026-01-02 09:30Z",
		weekday.Next(utc(2026, 1, 2, 9, 30, 0)), utc(2026, 1, 5, 9, 30, 0))

	// Leap day: the next 29 February after 2026-01-01 is in 2028.
	leap := quartz.MustParseCron("0 0 12 29 2 ?")
	check("'0 0 12 29 2 ?' after 2026-01-01 00:00Z",
		leap.Next(utc(2026, 1, 1, 0, 0, 0)), utc(2028, 2, 29, 12, 0, 0))

	// Impossible schedule: 30 February never occurs, so Next gives up.
	never := quartz.MustParseCron("0 0 12 30 2 ?")
	check("'0 0 12 30 2 ?' (impossible) after 2026-01-01",
		never.Next(utc(2026, 1, 1, 0, 0, 0)), time.Time{})

	// Five-field form: seconds default to 0.
	five := quartz.MustParseCron("30 9 * * *")
	check("'30 9 * * *' after 2026-01-01 09:00:00Z",
		five.Next(utc(2026, 1, 1, 9, 0, 0)), utc(2026, 1, 1, 9, 30, 0))

	// Unix OR semantics: day-of-month 13 OR Friday. February 2026 starts on a
	// Sunday, so the first Friday is 2026-02-06 (before the 13th).
	orDays := quartz.MustParseCron("0 0 0 13 * FRI")
	check("'0 0 0 13 * FRI' after 2026-02-01 00:00Z",
		orDays.Next(utc(2026, 2, 1, 0, 0, 0)), utc(2026, 2, 6, 0, 0, 0))

	// Month names and a year rollover.
	jan1 := quartz.MustParseCron("0 0 0 1 JAN *")
	check("'0 0 0 1 JAN *' after 2026-06-15 00:00Z",
		jan1.Next(utc(2026, 6, 15, 0, 0, 0)), utc(2027, 1, 1, 0, 0, 0))

	// Sunday accepted as both 0 and 7.
	sun0 := quartz.MustParseCron("0 0 6 ? * 0")
	sun7 := quartz.MustParseCron("0 0 6 ? * 7")
	ref := utc(2026, 1, 1, 0, 0, 0) // Thursday; the next Sunday is 2026-01-04.
	check("'0 0 6 ? * 0' after 2026-01-01", sun0.Next(ref), utc(2026, 1, 4, 6, 0, 0))
	check("'0 0 6 ? * 7' after 2026-01-01", sun7.Next(ref), utc(2026, 1, 4, 6, 0, 0))

	// Evaluation follows the location of the reference time.
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		fmt.Printf("  (skipping tz case: %v)\n", err)
		return
	}
	// 2026-01-01 12:00 UTC is 07:00 EST, so the next 10:00 New York local time
	// is later the same day: 10:00 EST == 15:00 UTC.
	check("'0 0 10 * * *' in America/New_York after 12:00Z",
		daily.Next(utc(2026, 1, 1, 12, 0, 0).In(ny)).UTC(), utc(2026, 1, 1, 15, 0, 0))
}

// ---------------------------------------------------------------------------
// 3. CronTrigger lifecycle
// ---------------------------------------------------------------------------

func cronTriggerLifecycle() {
	section("3. CronTrigger: first fire, advancement, start/end window, time zone")

	job := quartz.NewKey("nightly")
	trig, err := quartz.NewCronTriggerWithKeys(quartz.NewKey("nightly-trigger"), job, "0 0 10 * * *")
	if err != nil {
		fmt.Println("  ParseCron failed:", err)
		return
	}
	trig.WithDescription("every day at 10:00 UTC")

	first := trig.ComputeFirstFireTime(utc(2026, 1, 1, 9, 0, 0))
	check("first fire from 2026-01-01 09:00Z", first, utc(2026, 1, 1, 10, 0, 0))
	check("next after fire 1", trig.Triggered(utc(2026, 1, 1, 10, 0, 0)), utc(2026, 1, 2, 10, 0, 0))
	check("previous fire time", trig.PreviousFireTime(), utc(2026, 1, 1, 10, 0, 0))
	check("next after fire 2", trig.Triggered(utc(2026, 1, 2, 10, 0, 0)), utc(2026, 1, 3, 10, 0, 0))
	checkBool("WillFireAgain", trig.WillFireAgain(), true)
	fmt.Println("   ", trig)

	// A start time in the future is itself a legal fire time.
	windowed, _ := quartz.NewCronTriggerWithKeys(quartz.NewKey("windowed"), job, "0 0 10 * * *")
	windowed.StartingAt(utc(2026, 6, 1, 10, 0, 0)).EndingAt(utc(2026, 6, 3, 10, 0, 0))
	check("StartingAt is a valid first fire",
		windowed.ComputeFirstFireTime(utc(2026, 1, 1, 0, 0, 0)), utc(2026, 6, 1, 10, 0, 0))
	check("2nd fire inside window", windowed.Triggered(utc(2026, 6, 1, 10, 0, 0)), utc(2026, 6, 2, 10, 0, 0))
	check("3rd fire is the end time", windowed.Triggered(utc(2026, 6, 2, 10, 0, 0)), utc(2026, 6, 3, 10, 0, 0))
	check("past the end time -> never", windowed.Triggered(utc(2026, 6, 3, 10, 0, 0)), time.Time{})
	checkBool("WillFireAgain after end", windowed.WillFireAgain(), false)

	if ny, err := time.LoadLocation("America/New_York"); err == nil {
		tz, _ := quartz.NewCronTriggerWithKeys(quartz.NewKey("tz"), job, "0 0 10 * * *")
		tz.In(ny)
		// 10:00 EST on 2026-01-01 is 15:00 UTC.
		check("CronTrigger.In(NY) first fire (as UTC)",
			tz.ComputeFirstFireTime(utc(2026, 1, 1, 0, 0, 0)).UTC(), utc(2026, 1, 1, 15, 0, 0))
		// Across the 2026-03-08 US DST switch, 10:00 local becomes 14:00 UTC.
		tz2, _ := quartz.NewCronTriggerWithKeys(quartz.NewKey("tz2"), job, "0 0 10 * * *")
		tz2.In(ny)
		check("CronTrigger.In(NY) after DST start (as UTC)",
			tz2.ComputeFirstFireTime(utc(2026, 3, 9, 0, 0, 0)).UTC(), utc(2026, 3, 9, 14, 0, 0))
	}
}

// ---------------------------------------------------------------------------
// 4. SimpleTrigger
// ---------------------------------------------------------------------------

func simpleTriggerArithmetic() {
	section("4. SimpleTrigger: interval, repeat count, end time, FireTimeAfter")

	start := utc(2026, 1, 1, 0, 0, 0)
	job := quartz.NewKey("batch")

	// repeatCount 3 => 4 fires total: 00:00, 00:10, 00:20, 00:30.
	t := quartz.NewSimpleTrigger(quartz.NewKey("every-10m"), job, start, 10*time.Minute, 3)
	t.WithDescription("4 fires, 10 minutes apart")
	check("first fire == start time", t.ComputeFirstFireTime(start.Add(-time.Hour)), start)
	check("fire 2", t.Triggered(start), utc(2026, 1, 1, 0, 10, 0))
	check("fire 3", t.Triggered(utc(2026, 1, 1, 0, 10, 0)), utc(2026, 1, 1, 0, 20, 0))
	check("fire 4", t.Triggered(utc(2026, 1, 1, 0, 20, 0)), utc(2026, 1, 1, 0, 30, 0))
	check("exhausted after repeatCount+1 fires", t.Triggered(utc(2026, 1, 1, 0, 30, 0)), time.Time{})
	checkInt("TimesTriggered", t.TimesTriggered(), 4)
	fmt.Println("   ", t)

	// FireTimeAfter is a pure query independent of running state.
	q := quartz.NewSimpleTrigger(quartz.NewKey("q"), job, start, 10*time.Minute, 3)
	check("FireTimeAfter(start-1s) == start", q.FireTimeAfter(start.Add(-time.Second)), start)
	check("FireTimeAfter(start)", q.FireTimeAfter(start), utc(2026, 1, 1, 0, 10, 0))
	check("FireTimeAfter(00:15)", q.FireTimeAfter(utc(2026, 1, 1, 0, 15, 0)), utc(2026, 1, 1, 0, 20, 0))
	check("FireTimeAfter(00:30) past last fire", q.FireTimeAfter(utc(2026, 1, 1, 0, 30, 0)), time.Time{})

	// RepeatForever with an end time.
	forever := quartz.NewSimpleTrigger(quartz.NewKey("hb"), job, start, time.Hour, quartz.RepeatForever)
	forever.WithEndTime(utc(2026, 1, 1, 2, 30, 0))
	check("forever: first", forever.ComputeFirstFireTime(start), start)
	check("forever: +1h", forever.Triggered(start), utc(2026, 1, 1, 1, 0, 0))
	check("forever: +2h", forever.Triggered(utc(2026, 1, 1, 1, 0, 0)), utc(2026, 1, 1, 2, 0, 0))
	check("forever: +3h is past end time", forever.Triggered(utc(2026, 1, 1, 2, 0, 0)), time.Time{})

	// A trigger whose start is in the past still starts at its start time,
	// which is what makes misfire handling necessary.
	late := quartz.NewSimpleTrigger(quartz.NewKey("late"), job, start, time.Hour, quartz.RepeatForever)
	check("start in the past is not skipped",
		late.ComputeFirstFireTime(utc(2026, 1, 1, 5, 0, 0)), start)
}

// ---------------------------------------------------------------------------
// 5. CalendarIntervalTrigger
// ---------------------------------------------------------------------------

func calendarIntervalTrigger() {
	section("5. CalendarIntervalTrigger: calendar-unit advancement")

	job := quartz.NewKey("monthly")

	// Monthly from the 15th: clean month arithmetic.
	m := quartz.NewCalendarIntervalTrigger(quartz.NewKey("m15"), job,
		utc(2026, 1, 15, 12, 0, 0), quartz.IntervalMonth, 1)
	check("monthly: first", m.ComputeFirstFireTime(utc(2026, 1, 1, 0, 0, 0)), utc(2026, 1, 15, 12, 0, 0))
	check("monthly: +1", m.Triggered(utc(2026, 1, 15, 12, 0, 0)), utc(2026, 2, 15, 12, 0, 0))
	check("monthly: +2", m.Triggered(utc(2026, 2, 15, 12, 0, 0)), utc(2026, 3, 15, 12, 0, 0))
	fmt.Println("   ", m)

	// Quarterly (3 months) crossing a year boundary.
	q := quartz.NewCalendarIntervalTrigger(quartz.NewKey("q"), job,
		utc(2026, 11, 1, 6, 0, 0), quartz.IntervalMonth, 3)
	check("quarterly: first", q.ComputeFirstFireTime(utc(2026, 1, 1, 0, 0, 0)), utc(2026, 11, 1, 6, 0, 0))
	check("quarterly: +3 months", q.Triggered(utc(2026, 11, 1, 6, 0, 0)), utc(2027, 2, 1, 6, 0, 0))

	// Weekly and yearly.
	w := quartz.NewCalendarIntervalTrigger(quartz.NewKey("w"), job,
		utc(2026, 1, 1, 0, 0, 0), quartz.IntervalWeek, 2)
	w.ComputeFirstFireTime(utc(2026, 1, 1, 0, 0, 0))
	check("every 2 weeks", w.Triggered(utc(2026, 1, 1, 0, 0, 0)), utc(2026, 1, 15, 0, 0, 0))

	y := quartz.NewCalendarIntervalTrigger(quartz.NewKey("y"), job,
		utc(2026, 5, 4, 8, 0, 0), quartz.IntervalYear, 1)
	y.ComputeFirstFireTime(utc(2026, 1, 1, 0, 0, 0))
	check("yearly", y.Triggered(utc(2026, 5, 4, 8, 0, 0)), utc(2027, 5, 4, 8, 0, 0))

	// FireTimeAfter is a pure query.
	pure := quartz.NewCalendarIntervalTrigger(quartz.NewKey("pure"), job,
		utc(2026, 1, 15, 12, 0, 0), quartz.IntervalMonth, 1)
	check("FireTimeAfter(2026-04-01)", pure.FireTimeAfter(utc(2026, 4, 1, 0, 0, 0)), utc(2026, 4, 15, 12, 0, 0))

	// End-of-month behaviour. Java Quartz uses Calendar.add(MONTH), which
	// CLAMPS: 2026-01-31 + 1 month == 2026-02-28. This port uses
	// time.Time.AddDate, which OVERFLOWS to 2026-03-03. The expectation below
	// is the Java/Quartz value, so the reported mismatch documents the
	// divergence rather than hiding it.
	eom := quartz.NewCalendarIntervalTrigger(quartz.NewKey("eom"), job,
		utc(2026, 1, 31, 12, 0, 0), quartz.IntervalMonth, 1)
	eom.ComputeFirstFireTime(utc(2026, 1, 1, 0, 0, 0))
	check("Jan 31 + 1 month (Quartz clamps to Feb 28)",
		eom.Triggered(utc(2026, 1, 31, 12, 0, 0)), utc(2026, 2, 28, 12, 0, 0))

	// Same story for a leap day plus one year: Java clamps to 2029-02-28.
	leap := quartz.NewCalendarIntervalTrigger(quartz.NewKey("leap"), job,
		utc(2028, 2, 29, 12, 0, 0), quartz.IntervalYear, 1)
	leap.ComputeFirstFireTime(utc(2028, 1, 1, 0, 0, 0))
	check("Feb 29 + 1 year (Quartz clamps to Feb 28)",
		leap.Triggered(utc(2028, 2, 29, 12, 0, 0)), utc(2029, 2, 28, 12, 0, 0))
}

// ---------------------------------------------------------------------------
// 6. DailyTimeIntervalTrigger
// ---------------------------------------------------------------------------

func dailyTimeIntervalTrigger() {
	section("6. DailyTimeIntervalTrigger: intra-day window on selected weekdays")

	job := quartz.NewKey("market-poll")
	t := quartz.NewDailyTimeIntervalTrigger(quartz.NewKey("biz-hours"), job,
		quartz.NewTimeOfDay(9, 0, 0), quartz.NewTimeOfDay(17, 0, 0),
		quartz.IntervalHour, 4)
	// NOTE: OnMondayThroughFriday / OnEveryDay / OnSaturdayAndSunday exist only
	// on DailyTimeIntervalScheduleBuilder, not on the trigger itself, so the
	// trigger API needs the explicit weekday list.
	t.OnDaysOfWeek(time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday).In(time.UTC)
	fmt.Println("   ", t)
	fmt.Printf("    active weekdays: %v\n", t.DaysOfWeek())

	// Slots are 09:00, 13:00, 17:00. 2026-01-02 is a Friday.
	check("first slot at/after Fri 10:30",
		t.ComputeFirstFireTime(utc(2026, 1, 2, 10, 30, 0)), utc(2026, 1, 2, 13, 0, 0))
	check("next slot same day", t.Triggered(utc(2026, 1, 2, 13, 0, 0)), utc(2026, 1, 2, 17, 0, 0))
	// Sat/Sun are skipped, so the next slot is Monday 2026-01-05 09:00.
	check("weekend skipped -> Mon 09:00", t.Triggered(utc(2026, 1, 2, 17, 0, 0)), utc(2026, 1, 5, 9, 0, 0))

	// A bounded repeat count.
	b := quartz.NewDailyTimeIntervalTrigger(quartz.NewKey("bounded"), job,
		quartz.NewTimeOfDay(9, 0, 0), quartz.NewTimeOfDay(17, 0, 0), quartz.IntervalHour, 4)
	b.OnDaysOfWeek(time.Sunday, time.Monday, time.Tuesday, time.Wednesday,
		time.Thursday, time.Friday, time.Saturday).WithRepeatCount(1)
	check("bounded: first", b.ComputeFirstFireTime(utc(2026, 1, 2, 0, 0, 0)), utc(2026, 1, 2, 9, 0, 0))
	check("bounded: second", b.Triggered(utc(2026, 1, 2, 9, 0, 0)), utc(2026, 1, 2, 13, 0, 0))
	check("bounded: exhausted", b.Triggered(utc(2026, 1, 2, 13, 0, 0)), time.Time{})

	// Weekend-only, one fire per day (no sub-day unit => window start only).
	we := quartz.NewDailyTimeIntervalTrigger(quartz.NewKey("weekend"), job,
		quartz.NewTimeOfDay(8, 30, 0), quartz.NewTimeOfDay(8, 30, 0), quartz.IntervalDay, 1)
	we.OnDaysOfWeek(time.Saturday, time.Sunday)
	// 2026-01-01 is a Thursday; the next Saturday is 2026-01-03.
	check("weekend-only first fire",
		we.ComputeFirstFireTime(utc(2026, 1, 1, 0, 0, 0)), utc(2026, 1, 3, 8, 30, 0))
	check("weekend-only second fire", we.Triggered(utc(2026, 1, 3, 8, 30, 0)), utc(2026, 1, 4, 8, 30, 0))
	check("weekend-only third fire (next Sat)", we.Triggered(utc(2026, 1, 4, 8, 30, 0)), utc(2026, 1, 10, 8, 30, 0))
}

// ---------------------------------------------------------------------------
// 7. fluent builders
// ---------------------------------------------------------------------------

func fluentBuilders() {
	section("7. Fluent builders (NewJob / NewTrigger / *Schedule)")

	detail, err := quartz.NewJob().
		OfJob(quartz.JobFunc(func(ctx context.Context) error { return nil })).
		WithIdentityNameGroup("invoice-sweep", "billing").
		WithDescription("sweep unpaid invoices").
		UsingJobData("tenant", "acme").
		UsingJobData("limit", 500).
		StoreDurably(true).
		Build()
	if err != nil {
		fmt.Println("  JobBuilder failed:", err)
		return
	}
	fmt.Printf("  job:      %s  key=%s durable=%t desc=%q\n", detail, detail.Key(), detail.IsDurable(), detail.Description())
	tenant, okT := detail.Data().GetString("tenant")
	limit, okL := detail.Data().GetInt("limit")
	checkBool("JobDataMap.GetString(\"tenant\") ok", okT, true)
	checkBool("JobDataMap.GetInt(\"limit\") ok", okL, true)
	fmt.Printf("  data:     tenant=%q limit=%d\n", tenant, limit)
	if _, ok := detail.Data().GetInt("tenant"); ok {
		mismatches = append(mismatches, "JobDataMap.GetInt on a string value reported ok")
	}

	// Missing Job is an error, not a panic.
	if _, err := quartz.NewJob().Build(); err != nil {
		fmt.Println("  reject:   NewJob().Build() ->", err)
	} else {
		mismatches = append(mismatches, "JobBuilder.Build() accepted a nil Job")
	}
	if _, err := quartz.NewTrigger().Build(); err != nil {
		fmt.Println("  reject:   NewTrigger().Build() ->", err)
	} else {
		mismatches = append(mismatches, "TriggerBuilder.Build() accepted a missing job key")
	}

	// Cron schedule via the builder.
	cs, err := quartz.CronSchedule("0 0 3 * * *")
	if err != nil {
		fmt.Println("  CronSchedule failed:", err)
		return
	}
	ct, err := quartz.NewTrigger().
		WithIdentityName("nightly-sweep").
		ForJobDetail(detail).
		WithDescription("03:00 UTC daily").
		StartAt(utc(2026, 1, 1, 0, 0, 0)).
		EndAt(utc(2026, 1, 4, 0, 0, 0)).
		WithSchedule(cs.WithMisfirePolicy(quartz.MisfireDoNothing)).
		Build()
	if err != nil {
		fmt.Println("  TriggerBuilder failed:", err)
		return
	}
	fmt.Printf("  trigger:  %s\n", ct)
	check("builder cron first fire", ct.ComputeFirstFireTime(utc(2026, 1, 1, 0, 0, 0)), utc(2026, 1, 1, 3, 0, 0))
	check("builder cron 2nd fire", ct.Triggered(utc(2026, 1, 1, 3, 0, 0)), utc(2026, 1, 2, 3, 0, 0))
	check("builder cron 3rd fire", ct.Triggered(utc(2026, 1, 2, 3, 0, 0)), utc(2026, 1, 3, 3, 0, 0))
	check("builder cron stops at EndAt", ct.Triggered(utc(2026, 1, 3, 3, 0, 0)), time.Time{})

	// Convenience cron helpers.
	daily := quartz.CronScheduleDailyAt(6, 45)
	dt, err := daily.BuildTrigger(quartz.NewKey("daily-645"), detail.Key(), utc(2026, 1, 1, 0, 0, 0), time.Time{}, "")
	if err != nil {
		fmt.Println("  CronScheduleDailyAt failed:", err)
	} else {
		check("CronScheduleDailyAt(6,45) first", dt.ComputeFirstFireTime(utc(2026, 1, 1, 0, 0, 0)), utc(2026, 1, 1, 6, 45, 0))
	}
	weekly := quartz.CronScheduleWeeklyOn(time.Monday, 7, 15)
	wt, err := weekly.BuildTrigger(quartz.NewKey("weekly-mon"), detail.Key(), utc(2026, 1, 1, 0, 0, 0), time.Time{}, "")
	if err != nil {
		fmt.Println("  CronScheduleWeeklyOn failed:", err)
	} else {
		// 2026-01-01 is a Thursday, so the next Monday is 2026-01-05.
		check("CronScheduleWeeklyOn(Mon,7,15)", wt.ComputeFirstFireTime(utc(2026, 1, 1, 0, 0, 0)), utc(2026, 1, 5, 7, 15, 0))
	}
	monthly := quartz.CronScheduleMonthlyOn(9, 5, 0)
	mt, err := monthly.BuildTrigger(quartz.NewKey("monthly-9th"), detail.Key(), utc(2026, 1, 1, 0, 0, 0), time.Time{}, "")
	if err != nil {
		fmt.Println("  CronScheduleMonthlyOn failed:", err)
	} else {
		check("CronScheduleMonthlyOn(9,5,0)", mt.ComputeFirstFireTime(utc(2026, 1, 1, 0, 0, 0)), utc(2026, 1, 9, 5, 0, 0))
	}

	// SimpleSchedule via the builder.
	st, err := quartz.NewTrigger().
		WithIdentityName("simple-built").
		ForJob(detail.Key()).
		StartAt(utc(2026, 1, 1, 0, 0, 0)).
		WithSchedule(quartz.SimpleSchedule().WithIntervalInMinutes(90).WithRepeatCount(2)).
		Build()
	if err != nil {
		fmt.Println("  SimpleSchedule build failed:", err)
	} else {
		check("SimpleSchedule 90m: first", st.ComputeFirstFireTime(utc(2026, 1, 1, 0, 0, 0)), utc(2026, 1, 1, 0, 0, 0))
		check("SimpleSchedule 90m: fire 2", st.Triggered(utc(2026, 1, 1, 0, 0, 0)), utc(2026, 1, 1, 1, 30, 0))
		check("SimpleSchedule 90m: fire 3", st.Triggered(utc(2026, 1, 1, 1, 30, 0)), utc(2026, 1, 1, 3, 0, 0))
		check("SimpleSchedule 90m: exhausted", st.Triggered(utc(2026, 1, 1, 3, 0, 0)), time.Time{})
	}

	// Matchers over stored keys.
	keys := []quartz.Key{
		quartz.NewKeyInGroup("invoice-sweep", "billing"),
		quartz.NewKeyInGroup("invoice-retry", "billing"),
		quartz.NewKey("healthcheck"),
	}
	billing := quartz.MatchKeys(quartz.GroupEquals("billing"), keys)
	invoices := quartz.MatchKeys(quartz.AndMatcher(quartz.GroupEquals("billing"), quartz.NameStartsWith("invoice-")), keys)
	notBilling := quartz.MatchKeys(quartz.NotMatcher(quartz.GroupEquals("billing")), keys)
	checkInt("matcher: group=billing", len(billing), 2)
	checkInt("matcher: billing AND invoice-*", len(invoices), 2)
	checkInt("matcher: NOT group=billing", len(notBilling), 1)
}

// ---------------------------------------------------------------------------
// 8. calendars
// ---------------------------------------------------------------------------

func calendarExclusions() {
	section("8. Calendars: excluding instants from a schedule")

	hol := quartz.NewHolidayCalendar()
	hol.AddExcludedDate(utc(2026, 1, 2, 0, 0, 0))
	hol.AddExcludedDate(utc(2026, 1, 3, 0, 0, 0))
	checkBool("holiday excludes 2026-01-02", hol.IsTimeIncluded(utc(2026, 1, 2, 10, 0, 0)), false)
	checkBool("holiday includes 2026-01-04", hol.IsTimeIncluded(utc(2026, 1, 4, 10, 0, 0)), true)

	inner, err := quartz.NewCronTriggerWithKeys(quartz.NewKey("daily-10"), quartz.NewKey("j"), "0 0 10 * * *")
	if err != nil {
		fmt.Println("  cron parse failed:", err)
		return
	}
	wrapped := quartz.WithCalendar(inner, hol)
	check("wrapped first fire", wrapped.ComputeFirstFireTime(utc(2026, 1, 1, 0, 0, 0)), utc(2026, 1, 1, 10, 0, 0))
	// Jan 2 and Jan 3 are excluded, so the next fire is Jan 4.
	check("wrapped skips two holidays", wrapped.Triggered(utc(2026, 1, 1, 10, 0, 0)), utc(2026, 1, 4, 10, 0, 0))

	// A weekly calendar excluding weekends.
	wk := quartz.NewWeeklyCalendar()
	wk.SetDayExcluded(time.Saturday, true)
	wk.SetDayExcluded(time.Sunday, true)
	checkBool("weekly excludes Sat 2026-01-03", wk.IsTimeIncluded(utc(2026, 1, 3, 12, 0, 0)), false)
	checkBool("weekly includes Mon 2026-01-05", wk.IsTimeIncluded(utc(2026, 1, 5, 12, 0, 0)), true)

	inner2, _ := quartz.NewCronTriggerWithKeys(quartz.NewKey("daily-10b"), quartz.NewKey("j"), "0 0 10 * * *")
	wd := quartz.WithCalendar(inner2, wk)
	// 2026-01-02 is a Friday.
	check("weekday-only first fire", wd.ComputeFirstFireTime(utc(2026, 1, 2, 0, 0, 0)), utc(2026, 1, 2, 10, 0, 0))
	check("weekday-only skips the weekend", wd.Triggered(utc(2026, 1, 2, 10, 0, 0)), utc(2026, 1, 5, 10, 0, 0))

	// A daily calendar excluding a maintenance window, then inverted.
	maint := quartz.NewDailyCalendar(quartz.NewTimeOfDay(1, 0, 0), quartz.NewTimeOfDay(2, 0, 0))
	checkBool("daily cal excludes 01:30", maint.IsTimeIncluded(utc(2026, 1, 1, 1, 30, 0)), false)
	checkBool("daily cal includes 03:00", maint.IsTimeIncluded(utc(2026, 1, 1, 3, 0, 0)), true)
	only := quartz.NewDailyCalendar(quartz.NewTimeOfDay(1, 0, 0), quartz.NewTimeOfDay(2, 0, 0)).Invert()
	checkBool("inverted daily cal includes 01:30", only.IsTimeIncluded(utc(2026, 1, 1, 1, 30, 0)), true)

	// A cron calendar EXCLUDES the instants matching its expression (matching
	// Java Quartz's CronCalendar), so it is a blocklist, not an allowlist.
	cc, err := quartz.NewCronCalendar("0 0 10 * * *")
	if err != nil {
		fmt.Println("  NewCronCalendar failed:", err)
	} else {
		checkBool("cron cal excludes the matching 10:00:00", cc.IsTimeIncluded(utc(2026, 1, 1, 10, 0, 0)), false)
		checkBool("cron cal includes 11:00:00", cc.IsTimeIncluded(utc(2026, 1, 1, 11, 0, 0)), true)
		check("cron cal next included after 10:00:00",
			cc.NextIncludedTime(utc(2026, 1, 1, 10, 0, 0)), utc(2026, 1, 1, 10, 0, 1))
	}

	// Annual calendar (a fixed holiday every year).
	an := quartz.NewAnnualCalendar()
	an.SetDayExcluded(time.December, 25, true)
	checkBool("annual excludes 2026-12-25", an.IsTimeIncluded(utc(2026, 12, 25, 9, 0, 0)), false)
	checkBool("annual excludes 2027-12-25", an.IsTimeIncluded(utc(2027, 12, 25, 9, 0, 0)), false)

	// Chaining via a base calendar: weekends AND Christmas are both excluded.
	an.SetBaseCalendar(wk)
	checkBool("chained: Sat is excluded via base", an.IsTimeIncluded(utc(2026, 1, 3, 9, 0, 0)), false)
	checkBool("chained: Mon is included", an.IsTimeIncluded(utc(2026, 1, 5, 9, 0, 0)), true)
}

// ---------------------------------------------------------------------------
// 9. deterministic scheduler driven by an injected clock
// ---------------------------------------------------------------------------

func deterministicScheduler() {
	section("9. Scheduler with an injected clock (deterministic, no sleeping)")

	// The clock is only advanced from this goroutine and the scheduler is
	// never Start()ed, so every ProcessDue call runs jobs inline: fully
	// deterministic, no background goroutines at all.
	now := utc(2026, 1, 1, 9, 59, 0)
	var clockMu sync.Mutex
	clock := func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return now
	}
	advance := func(t time.Time) {
		clockMu.Lock()
		now = t
		clockMu.Unlock()
		fmt.Printf("  clock -> %s\n", fmtTime(t))
	}

	s := quartz.NewScheduler(quartz.Options{
		Clock:            clock,
		Concurrency:      1,
		MisfireThreshold: time.Hour, // large, so only section 10 exercises misfires
	})

	audit := &auditListener{BaseJobListener: quartz.BaseJobListener{ListenerName: "audit"}}
	veto := &vetoListener{BaseTriggerListener: quartz.BaseTriggerListener{ListenerName: "veto"}}
	s.AddJobListener(audit)
	s.AddTriggerListener(veto)

	report := &counterJob{label: "report"}
	reportDetail := quartz.NewJobDetail(quartz.NewKey("report"), report).
		WithDescription("daily report").
		WithData(quartz.JobDataMap{"region": "eu-west", "batch": 42})

	reportTrig, err := quartz.NewCronTriggerWithKeys(
		quartz.NewKey("report-trigger"), reportDetail.Key(), "0 0 10 * * *")
	if err != nil {
		fmt.Println("  cron parse failed:", err)
		return
	}
	if err := s.ScheduleJob(reportDetail, reportTrig); err != nil {
		fmt.Println("  ScheduleJob failed:", err)
		return
	}
	check("scheduled first fire", reportTrig.NextFireTime(), utc(2026, 1, 1, 10, 0, 0))

	// A second job whose trigger the listener vetoes; the job data is read
	// back inside Execute to prove context passing works.
	skipped := &counterJob{label: "skipped"}
	skippedDetail := quartz.NewJobDetail(quartz.NewKey("skipped-job"), skipped)
	skippedTrig := quartz.NewSimpleTrigger(quartz.NewKey("veto-trigger"), skippedDetail.Key(),
		utc(2026, 1, 1, 10, 0, 0), 24*time.Hour, quartz.RepeatForever)
	if err := s.ScheduleJob(skippedDetail, skippedTrig); err != nil {
		fmt.Println("  ScheduleJob(veto) failed:", err)
	}

	// A job that reads its merged data and reports what it saw.
	var sawRegion string
	var sawBatch int
	dataJob := quartz.NewJobDetail(quartz.NewKey("data-job"), quartz.JobFunc(func(ctx context.Context) error {
		return nil
	})).WithData(quartz.JobDataMap{"region": "us-east", "batch": 7})
	s.AddJobListener(&dataProbe{
		BaseJobListener: quartz.BaseJobListener{ListenerName: "probe"},
		onExec: func(jec *quartz.JobExecutionContext) {
			if jec.Detail.Key() != dataJob.Key() {
				return
			}
			sawRegion, _ = jec.MergedData.GetString("region")
			sawBatch, _ = jec.MergedData.GetInt("batch")
		},
	})
	dataTrig := quartz.NewSimpleTrigger(quartz.NewKey("data-trigger"), dataJob.Key(),
		utc(2026, 1, 1, 10, 0, 0), 0, 0)
	if err := s.ScheduleJob(dataJob, dataTrig); err != nil {
		fmt.Println("  ScheduleJob(data) failed:", err)
	}

	// A deliberately failing job to show error propagation.
	boom := errors.New("upstream unavailable")
	failing := quartz.NewJobDetail(quartz.NewKey("failing-job"), quartz.JobFunc(func(ctx context.Context) error {
		return fmt.Errorf("refresh cache: %w", boom)
	}))
	failTrig := quartz.NewSimpleTrigger(quartz.NewKey("fail-trigger"), failing.Key(),
		utc(2026, 1, 1, 10, 0, 0), 0, 0)
	if err := s.ScheduleJob(failing, failTrig); err != nil {
		fmt.Println("  ScheduleJob(failing) failed:", err)
	}

	fmt.Printf("  jobs in store:     %v\n", s.Store().JobKeys())
	fmt.Printf("  triggers in store: %v\n", s.Store().TriggerKeys())

	fmt.Println("  -- nothing is due yet --")
	checkInt("ProcessDue at 09:59 fires nothing", s.ProcessDue(), 0)
	checkInt("report job runs", report.count(), 0)

	advance(utc(2026, 1, 1, 10, 0, 0))
	fired := s.ProcessDue()
	// report + data-job + failing-job dispatch; veto-trigger fires but is vetoed.
	checkInt("ProcessDue at 10:00 dispatches", fired, 3)
	checkInt("report job runs", report.count(), 1)
	checkInt("vetoed job never ran", skipped.count(), 0)
	fv, vv := veto.counts()
	checkInt("TriggerListener.TriggerFired calls", fv, 4)
	checkInt("TriggerListener vetoes", vv, 1)
	checkBool("merged data reached the job (region)", sawRegion == "us-east", true)
	checkInt("merged data reached the job (batch)", sawBatch, 7)

	started, finished, failed, errs := audit.snapshot()
	checkInt("JobListener started", started, 3)
	checkInt("JobListener finished", finished, 3)
	checkInt("JobListener observed failures", failed, 1)
	for _, e := range errs {
		fmt.Printf("    job error observed: %s\n", e)
	}
	if len(errs) == 1 && !strings.Contains(errs[0], boom.Error()) {
		mismatches = append(mismatches, "wrapped job error did not reach the listener intact")
	}

	// Fire-once triggers are now complete.
	if st, err := s.Store().GetTrigger(quartz.NewKey("fail-trigger")); err == nil {
		checkBool("fire-once trigger is COMPLETE", st.State == quartz.TriggerStateComplete, true)
		fmt.Printf("    fail-trigger state: %s\n", st.State)
	}

	// -- pause / resume --
	fmt.Println("  -- pause, miss a day, resume --")
	if err := s.PauseTrigger(reportTrig.Key()); err != nil {
		fmt.Println("  PauseTrigger failed:", err)
	}
	if st, err := s.Store().GetTrigger(reportTrig.Key()); err == nil {
		fmt.Printf("    state after PauseTrigger: %s\n", st.State)
		checkBool("paused state stored", st.State == quartz.TriggerStatePaused, true)
	}
	advance(utc(2026, 1, 2, 10, 0, 0))
	checkInt("paused trigger does not fire", s.ProcessDue(), 0)
	checkInt("report job runs (still)", report.count(), 1)

	// Resume 23 hours after the missed fire: past the 1h misfire threshold, so
	// the MisfireSmart policy advances to the next valid time after now.
	advance(utc(2026, 1, 3, 9, 0, 0))
	if err := s.ResumeTrigger(reportTrig.Key()); err != nil {
		fmt.Println("  ResumeTrigger failed:", err)
	}
	if st, err := s.Store().GetTrigger(reportTrig.Key()); err == nil {
		fmt.Printf("    state after ResumeTrigger: %s\n", st.State)
	}
	check("misfire-smart resume skips the missed fire",
		reportTrig.NextFireTime(), utc(2026, 1, 3, 10, 0, 0))

	advance(utc(2026, 1, 3, 10, 0, 0))
	checkInt("resumed trigger fires", s.ProcessDue(), 1)
	checkInt("report job runs", report.count(), 2)

	// PauseJob / ResumeJob operate on every trigger of a job.
	if err := s.PauseJob(reportDetail.Key()); err != nil {
		fmt.Println("  PauseJob failed:", err)
	}
	if st, err := s.Store().GetTrigger(reportTrig.Key()); err == nil {
		checkBool("PauseJob paused the job's trigger", st.State == quartz.TriggerStatePaused, true)
	}
	if err := s.ResumeJob(reportDetail.Key()); err != nil {
		fmt.Println("  ResumeJob failed:", err)
	}
	if st, err := s.Store().GetTrigger(reportTrig.Key()); err == nil {
		checkBool("ResumeJob restored NORMAL", st.State == quartz.TriggerStateNormal, true)
	}

	// -- out-of-band fire --
	fmt.Println("  -- TriggerJob fires out of band --")
	before := report.count()
	if err := s.TriggerJob(reportDetail.Key()); err != nil {
		fmt.Println("  TriggerJob failed:", err)
	}
	checkInt("TriggerJob ran the job once", report.count(), before+1)

	// -- unschedule --
	fmt.Println("  -- unschedule and delete --")
	removed, err := s.UnscheduleJob(reportTrig.Key())
	if err != nil {
		fmt.Println("  UnscheduleJob failed:", err)
	}
	checkBool("UnscheduleJob reported removal", removed, true)
	if _, err := s.Store().GetTrigger(reportTrig.Key()); !errors.Is(err, quartz.ErrTriggerNotFound) {
		mismatches = append(mismatches, "unscheduled trigger still present in the store")
	} else {
		fmt.Println("    trigger gone:", err)
	}
	// The job was not durable and had no other triggers, so it was removed too.
	if _, err := s.Store().GetJob(reportDetail.Key()); !errors.Is(err, quartz.ErrJobNotFound) {
		mismatches = append(mismatches, "non-durable job survived its last trigger being unscheduled")
	} else {
		fmt.Println("    non-durable job cascaded away:", err)
	}

	// Durable jobs may be added with no trigger at all and survive.
	durable := quartz.NewJobDetail(quartz.NewKey("durable-job"),
		quartz.JobFunc(func(ctx context.Context) error { return nil })).Durable(true)
	if err := s.AddJob(durable); err != nil {
		fmt.Println("  AddJob failed:", err)
	}
	if _, err := s.Store().GetJob(durable.Key()); err != nil {
		mismatches = append(mismatches, "durable job added without a trigger was not stored")
	} else {
		fmt.Println("    durable job stored with no trigger")
	}
	nonDurable := quartz.NewJobDetail(quartz.NewKey("nd"), quartz.JobFunc(func(ctx context.Context) error { return nil }))
	if err := s.AddJob(nonDurable); err != nil {
		fmt.Println("    reject: AddJob(non-durable) ->", err)
	} else {
		mismatches = append(mismatches, "AddJob accepted a non-durable job with no trigger")
	}

	deleted, err := s.DeleteJob(skippedDetail.Key())
	if err != nil {
		fmt.Println("  DeleteJob failed:", err)
	}
	checkBool("DeleteJob removed the job", deleted, true)
	if len(s.Store().TriggersForJob(skippedDetail.Key())) != 0 {
		mismatches = append(mismatches, "DeleteJob left triggers behind")
	}

	// Mismatched keys are rejected.
	badTrig := quartz.NewSimpleTrigger(quartz.NewKey("bad"), quartz.NewKey("nobody"),
		utc(2026, 1, 1, 0, 0, 0), time.Minute, 0)
	if err := s.ScheduleJob(durable, badTrig); err != nil {
		fmt.Println("    reject: mismatched trigger/job keys ->", err)
	} else {
		mismatches = append(mismatches, "ScheduleJob accepted a trigger whose job key does not match the detail")
	}
	// Scheduling against an unknown job is rejected too.
	orphan := quartz.NewSimpleTrigger(quartz.NewKey("orphan"), quartz.NewKey("ghost"),
		utc(2026, 1, 1, 0, 0, 0), time.Minute, 0)
	if err := s.ScheduleJob(nil, orphan); errors.Is(err, quartz.ErrJobNotFound) {
		fmt.Println("    reject: trigger for unknown job ->", err)
	} else {
		mismatches = append(mismatches, fmt.Sprintf("ScheduleJob(nil, orphan) returned %v, want ErrJobNotFound", err))
	}

	// Shutdown of a never-started scheduler must be safe.
	s.Shutdown(true)
	checkBool("IsStarted after Shutdown", s.IsStarted(), false)
	if err := s.Start(); !errors.Is(err, quartz.ErrSchedulerShutdown) {
		mismatches = append(mismatches, fmt.Sprintf("Start() after Shutdown returned %v, want ErrSchedulerShutdown", err))
	} else {
		fmt.Println("    reject: Start() after Shutdown ->", err)
	}
	if err := s.ScheduleJob(durable, quartz.NewSimpleTrigger(quartz.NewKey("late2"), durable.Key(),
		utc(2026, 1, 1, 0, 0, 0), time.Minute, 0)); !errors.Is(err, quartz.ErrSchedulerShutdown) {
		mismatches = append(mismatches, fmt.Sprintf("ScheduleJob after Shutdown returned %v, want ErrSchedulerShutdown", err))
	} else {
		fmt.Println("    reject: ScheduleJob after Shutdown ->", err)
	}
}

// dataProbe is a JobListener that runs a callback before each execution.
type dataProbe struct {
	quartz.BaseJobListener
	onExec func(*quartz.JobExecutionContext)
}

func (p *dataProbe) JobToBeExecuted(_ context.Context, jec *quartz.JobExecutionContext) {
	if p.onExec != nil {
		p.onExec(jec)
	}
}

// ---------------------------------------------------------------------------
// 10. misfire policies
// ---------------------------------------------------------------------------

func misfirePolicies() {
	section("10. Misfire policies (UpdateAfterMisfire on a fixed timeline)")

	start := utc(2026, 1, 1, 0, 0, 0)
	late := utc(2026, 1, 1, 5, 30, 0) // 5.5 hours after the start
	job := quartz.NewKey("j")

	mk := func(p quartz.MisfirePolicy) *quartz.SimpleTrigger {
		t := quartz.NewSimpleTrigger(quartz.NewKey("t"), job, start, time.Hour, quartz.RepeatForever)
		t.WithMisfirePolicy(p)
		t.ComputeFirstFireTime(start)
		return t
	}

	// Smart / DoNothing: advance whole intervals until strictly after now.
	check("MisfireSmart -> first slot after 05:30", mk(quartz.MisfireSmart).UpdateAfterMisfire(late), utc(2026, 1, 1, 6, 0, 0))
	check("MisfireDoNothing -> same", mk(quartz.MisfireDoNothing).UpdateAfterMisfire(late), utc(2026, 1, 1, 6, 0, 0))
	// FireNow: fire immediately, at 'now'.
	check("MisfireFireNow -> now", mk(quartz.MisfireFireNow).UpdateAfterMisfire(late), late)
	// Ignore: leave the stale fire time so the scheduler catches up fire by fire.
	check("MisfireIgnore -> unchanged (catch up)", mk(quartz.MisfireIgnore).UpdateAfterMisfire(late), start)

	// Cron triggers dispatch the same way.
	mkCron := func(p quartz.MisfirePolicy) *quartz.CronTrigger {
		t, _ := quartz.NewCronTriggerWithKeys(quartz.NewKey("c"), job, "0 0 * * * *")
		t.WithMisfirePolicy(p)
		t.ComputeFirstFireTime(utc(2025, 12, 31, 23, 59, 0))
		return t
	}
	check("cron MisfireSmart", mkCron(quartz.MisfireSmart).UpdateAfterMisfire(late), utc(2026, 1, 1, 6, 0, 0))
	check("cron MisfireFireNow", mkCron(quartz.MisfireFireNow).UpdateAfterMisfire(late), late)
	check("cron MisfireIgnore", mkCron(quartz.MisfireIgnore).UpdateAfterMisfire(late), start)

	// End to end through the scheduler: MisfireIgnore catches up, one fire per
	// ProcessDue call, because AcquireNextTriggers only sees one due fire time
	// at a time.
	now := utc(2026, 1, 1, 3, 30, 0)
	s := quartz.NewScheduler(quartz.Options{
		Clock:            func() time.Time { return now },
		MisfireThreshold: time.Minute,
	})
	catchUp := &counterJob{label: "catchup"}
	detail := quartz.NewJobDetail(quartz.NewKey("catchup"), catchUp)
	trig := quartz.NewSimpleTrigger(quartz.NewKey("catchup-trigger"), detail.Key(), start, time.Hour, quartz.RepeatForever)
	trig.WithMisfirePolicy(quartz.MisfireIgnore)
	if err := s.ScheduleJob(detail, trig); err != nil {
		fmt.Println("  ScheduleJob failed:", err)
		return
	}
	for i := 0; i < 6; i++ {
		s.ProcessDue()
	}
	// Missed fires at 00:00,01:00,02:00,03:00 are replayed, then 04:00 > now.
	checkInt("MisfireIgnore replayed the missed fires", catchUp.count(), 4)
	check("next fire after catch-up", trig.NextFireTime(), utc(2026, 1, 1, 4, 0, 0))

	// MisfireDoNothing on the same timeline drops the missed fires entirely.
	now2 := utc(2026, 1, 1, 3, 30, 0)
	s2 := quartz.NewScheduler(quartz.Options{
		Clock:            func() time.Time { return now2 },
		MisfireThreshold: time.Minute,
	})
	drop := &counterJob{label: "drop"}
	d2 := quartz.NewJobDetail(quartz.NewKey("drop"), drop)
	t2 := quartz.NewSimpleTrigger(quartz.NewKey("drop-trigger"), d2.Key(), start, time.Hour, quartz.RepeatForever)
	t2.WithMisfirePolicy(quartz.MisfireDoNothing)
	if err := s2.ScheduleJob(d2, t2); err != nil {
		fmt.Println("  ScheduleJob failed:", err)
		return
	}
	for i := 0; i < 6; i++ {
		s2.ProcessDue()
	}
	checkInt("MisfireDoNothing dropped the missed fires", drop.count(), 0)
	check("next fire after dropping", t2.NextFireTime(), utc(2026, 1, 1, 4, 0, 0))

	s.Shutdown(true)
	s2.Shutdown(true)
}

// ---------------------------------------------------------------------------
// 11. real-time execution on a worker pool
// ---------------------------------------------------------------------------

func realTimeExecution() {
	section("11. Real-time execution: worker pool, short intervals, graceful drain")

	s := quartz.NewScheduler(quartz.Options{
		Concurrency:      4,
		PollInterval:     5 * time.Millisecond,
		MisfireThreshold: time.Hour, // don't misfire our sub-second schedules
	})
	audit := &auditListener{BaseJobListener: quartz.BaseJobListener{ListenerName: "rt-audit"}}
	s.AddJobListener(audit)

	// A bounded ticker: 5 fires, 20ms apart.
	ticker := &counterJob{label: "tick"}
	tickDetail := quartz.NewJobDetail(quartz.NewKey("ticker"), ticker).
		WithData(quartz.JobDataMap{"source": "realtime"})
	tickTrig := quartz.NewSimpleTrigger(quartz.NewKey("tick-trigger"), tickDetail.Key(),
		time.Now(), 20*time.Millisecond, 4) // repeatCount 4 => 5 fires

	// A job that fails every time, to show error handling under the pool.
	var failCount int
	var failMu sync.Mutex
	failDetail := quartz.NewJobDetail(quartz.NewKey("flaky"), quartz.JobFunc(func(ctx context.Context) error {
		failMu.Lock()
		failCount++
		n := failCount
		failMu.Unlock()
		return fmt.Errorf("attempt %d: simulated downstream failure", n)
	}))
	failTrig := quartz.NewSimpleTrigger(quartz.NewKey("flaky-trigger"), failDetail.Key(),
		time.Now(), 25*time.Millisecond, 2) // 3 fires

	if err := s.ScheduleJob(tickDetail, tickTrig); err != nil {
		fmt.Println("  ScheduleJob(ticker) failed:", err)
		return
	}
	if err := s.ScheduleJob(failDetail, failTrig); err != nil {
		fmt.Println("  ScheduleJob(flaky) failed:", err)
		return
	}
	if err := s.Start(); err != nil {
		fmt.Println("  Start failed:", err)
		return
	}
	checkBool("IsStarted", s.IsStarted(), true)

	// Wait, with a hard deadline, for both triggers to be exhausted.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	waitFor(ctx, 2*time.Millisecond, func() bool {
		_, finished, _, _ := audit.snapshot()
		return finished >= 8 // 5 ticks + 3 failures
	})

	s.Shutdown(true) // graceful: drain queued and in-flight work
	checkBool("IsStarted after Shutdown", s.IsStarted(), false)

	started, finished, failed, errs := audit.snapshot()
	fmt.Printf("  executions: started=%d finished=%d failed=%d\n", started, finished, failed)
	checkInt("ticker fired repeatCount+1 times", ticker.count(), 5)
	checkInt("flaky fired repeatCount+1 times", failCount, 3)
	checkInt("listener saw every execution", finished, 8)
	checkInt("listener saw every failure", failed, 3)
	for _, e := range errs {
		fmt.Printf("    %s\n", e)
	}
	checkBool("ticker trigger WillFireAgain (exhausted)", tickTrig.WillFireAgain(), false)
	if st, err := s.Store().GetTrigger(tickTrig.Key()); err == nil {
		checkBool("ticker trigger stored COMPLETE", st.State == quartz.TriggerStateComplete, true)
	}

	// A second Shutdown must be a no-op rather than a panic on a closed channel.
	s.Shutdown(true)
	fmt.Println("  second Shutdown returned cleanly")
}

// ---------------------------------------------------------------------------
// 12. cancelling shutdown
// ---------------------------------------------------------------------------

func cancellingShutdown() {
	section("12. Shutdown(false): the job context is cancelled")

	s := quartz.NewScheduler(quartz.Options{Concurrency: 2, PollInterval: 5 * time.Millisecond})
	entered := make(chan struct{})
	var once sync.Once
	var cancelled, timedOut int
	var mu sync.Mutex

	detail := quartz.NewJobDetail(quartz.NewKey("long-running"), quartz.JobFunc(func(ctx context.Context) error {
		once.Do(func() { close(entered) })
		select {
		case <-ctx.Done():
			mu.Lock()
			cancelled++
			mu.Unlock()
			return ctx.Err()
		case <-time.After(2 * time.Second):
			mu.Lock()
			timedOut++
			mu.Unlock()
			return errors.New("job was never cancelled")
		}
	}))
	trig := quartz.NewSimpleTrigger(quartz.NewKey("lr-trigger"), detail.Key(), time.Now(), 0, 0)
	if err := s.ScheduleJob(detail, trig); err != nil {
		fmt.Println("  ScheduleJob failed:", err)
		return
	}
	if err := s.Start(); err != nil {
		fmt.Println("  Start failed:", err)
		return
	}

	select {
	case <-entered:
		fmt.Println("  job is running and blocked on its context")
	case <-time.After(2 * time.Second):
		fmt.Println("  job never started (unexpected)")
		mismatches = append(mismatches, "long-running job was never dispatched by the real-time loop")
	}

	start := time.Now()
	s.Shutdown(false) // cancel in-flight work instead of draining
	elapsed := time.Since(start)
	fmt.Printf("  Shutdown(false) returned in %s\n", elapsed.Round(time.Millisecond))

	mu.Lock()
	c, t := cancelled, timedOut
	mu.Unlock()
	checkInt("job observed context cancellation", c, 1)
	checkInt("job hit its own timeout instead", t, 0)
	if elapsed > time.Second {
		mismatches = append(mismatches,
			fmt.Sprintf("Shutdown(false) took %s; it should return promptly after cancelling", elapsed))
	}
}

// waitFor polls cond until it is true or ctx expires.
func waitFor(ctx context.Context, every time.Duration, cond func() bool) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		if cond() {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
