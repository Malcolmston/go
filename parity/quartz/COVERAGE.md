# COVERAGE — `github.com/malcolmston/quartz` vs `org.quartz-scheduler:quartz`

| | |
| --- | --- |
| upstream (oracle) | `org.quartz-scheduler:quartz` **2.5.0** (pinned in `java/pom.xml`, `<quartz.version>`) |
| Go module under test | `github.com/malcolmston/quartz@v0.0.0-20260810111605-f2c1d7b7b70a` (published module, no `replace`) |
| harness | `GOWORK=off go test ./parity/quartz/` |
| cases | 225 across 9 groups |
| result | **158 match / 67 differ / 0 deviations → parity 70.2%** |

## How the upstream inventory was derived

Mechanically, from the real installed artifact — never from the README or from
memory:

```sh
Q=~/.m2/repository/org/quartz-scheduler/quartz/2.5.0/quartz-2.5.0.jar

# every public top-level class in the jar (231 classes, 44 of them in org.quartz)
unzip -Z1 "$Q" '*.class' | grep -v '\$' | sed 's#/#.#g;s#\.class$##' | sort

# every public member of the classes this harness targets
javap -cp "$Q" \
  org.quartz.CronExpression org.quartz.Trigger org.quartz.CronTrigger \
  org.quartz.SimpleTrigger org.quartz.CalendarIntervalTrigger \
  org.quartz.DailyTimeIntervalTrigger org.quartz.TimeOfDay \
  org.quartz.DateBuilder 'org.quartz.DateBuilder$IntervalUnit' \
  org.quartz.impl.triggers.CronTriggerImpl \
  org.quartz.impl.triggers.SimpleTriggerImpl \
  org.quartz.impl.triggers.CalendarIntervalTriggerImpl \
  org.quartz.impl.triggers.DailyTimeIntervalTriggerImpl
```

The Go side was enumerated with `GOWORK=off go doc -all github.com/malcolmston/quartz`
against the same module version that `go.mod` pins.

## Scope

The harness targets exactly the part of Quartz that is a **pure function**:
trigger fire-time computation. Given a trigger definition, a start instant and a
time zone, both sides emit the next *N* fire times as UTC ISO-8601 strings, and
the comparison is exact. Live scheduler execution timing is deliberately not
compared — it is not deterministic and says nothing about correctness.

`org.quartz` has 44 public top-level types; the 32 outside the fire-time surface
(`Scheduler`, `SchedulerFactory`, `JobDetail`, `JobBuilder`, `JobDataMap`,
`JobExecutionContext`, `ListenerManager`, `JobListener`, `TriggerListener`,
`SchedulerListener`, `Matcher`, `TriggerUtils`, `SchedulerContext`,
`SchedulerMetaData`, the seven exception types, the four annotations,
`InterruptableJob`, `StatefulJob`, `Job`, `JobKey`, `TriggerKey`,
`ScheduleBuilder`, `TriggerBuilder`, `ValueSet`) are listed at the end as
`untested` with the reason.

## `org.quartz.CronExpression`

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `CronExpression(String)` | `quartz.ParseCron` | differs | all 55 `parse-*` | full grammar; see divergences D1–D6 |
| `CronExpression(CronExpression)` | — | missing | — | no copy constructor |
| `getNextValidTimeAfter(Date)` | `CronExpression.Next` | differs | all 43 `next-*`, 19 `tz-*` | see D1–D8 |
| `getNextInvalidTimeAfter(Date)` | — | missing | — | not ported |
| `isSatisfiedBy(Date)` | `CronExpression.IsSatisfiedBy` | differs | 16 `sat-*` | see D2, D5, D6 |
| `getTimeZone()` / `setTimeZone(TimeZone)` | *(implicit: `Next` uses the instant's own `*time.Location`)* | differs | 19 `tz-*` | the port has no zone on the expression; the caller supplies one via `time.Time.In` |
| `getCronExpression()` | `CronExpression.String` | untested | — | source echo, compared only indirectly |
| `toString()` | `CronExpression.String` | untested | — | |
| `isValidExpression(String)` | — | missing | — | no boolean-returning validator (`ParseCron` returns `error`) |
| `validateExpression(String)` | `quartz.ParseCron` (error return) | match | all `parse-*` | accept/reject compared, message text not |
| `getExpressionSummary()` | — | missing | — | no field-set summary |
| `getTimeAfter(Date)` | `CronExpression.Next` | differs | `next-*` | public alias of `getNextValidTimeAfter` |
| `getTimeBefore(Date)` | — | missing | — | no backwards search |
| `getFinalFireTime()` | — | missing | — | not ported |
| `MAX_YEAR` | — | missing | — | no year field at all (D3) |
| `clone()` | — | missing | — | |
| — | `quartz.MustParseCron` | extra | — | panicking convenience wrapper |

## `org.quartz.CronTrigger` / `org.quartz.impl.triggers.CronTriggerImpl`

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `setCronExpression(String)` | `quartz.NewCronTrigger`, `NewCronTriggerWithKeys` | differs | `crontrig-bad-expression` | grammar divergences propagate |
| `setCronExpression(CronExpression)` | — | missing | — | |
| `getCronExpression()` | `CronTrigger.Expression` | untested | — | returns the parsed value, not the source |
| `getTimeZone()` / `setTimeZone(TimeZone)` | `CronTrigger.In` | match | `crontrig-zoned`, `crontrig-zoned-fall-back` | |
| `getStartTime()` / `setStartTime(Date)` | `CronTrigger.StartingAt` | match | `crontrig-after-before-start`, `crontrig-start-is-a-fire-time` | |
| `getEndTime()` / `setEndTime(Date)` | `CronTrigger.EndingAt` | untested | — | exercised only through `SimpleTrigger` end-time cases |
| `computeFirstFireTime(Calendar)` | `CronTrigger.ComputeFirstFireTime` | differs | 9 `crontrig-*` | the port searches strictly after `now`; upstream searches from `startTime-1s` |
| `getFireTimeAfter(Date)` | *(no pure query; `ComputeFirstFireTime` + `Triggered`)* | differs | 9 `crontrig-*` | the port has no `CronTrigger.FireTimeAfter` |
| `triggered(Calendar)` | `CronTrigger.Triggered` | differs | 9 `crontrig-*` | signature differs (no `org.quartz.Calendar` argument) |
| `getNextFireTime()` / `getPreviousFireTime()` | `NextFireTime`, `PreviousFireTime` | match | `crontrig-*` | |
| `setNextFireTime` / `setPreviousFireTime` | — | missing | — | no state injection |
| `mayFireAgain()` | `CronTrigger.WillFireAgain` | untested | — | |
| `getFinalFireTime()` | — | missing | — | |
| `willFireOn(Calendar[, boolean])` | — | missing | — | |
| `updateAfterMisfire(Calendar)` | `CronTrigger.UpdateAfterMisfire` | match (ignore path only) | `misfire-ignore-cron-*` | see "Misfire" below |
| `updateWithNewCalendar(Calendar, long)` | — | missing | — | the port's `CalendarTrigger` decorator is a different design |
| `getExpressionSummary()` | — | missing | — | |
| `getScheduleBuilder()` / `getTriggerBuilder()` | `quartz.CronSchedule` builder | untested | — | builders not compared |
| `MISFIRE_INSTRUCTION_FIRE_ONCE_NOW` (1) | `quartz.MisfireFireNow` (1) | untested | — | numeric values coincide by accident |
| `MISFIRE_INSTRUCTION_DO_NOTHING` (2) | `quartz.MisfireDoNothing` (3) | differs | — | the port has its own enum, not Quartz's wire constants |

## `org.quartz.SimpleTrigger` / `SimpleTriggerImpl`

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `getFireTimeAfter(Date)` | `SimpleTrigger.FireTimeAfter` | differs | 20 `simple-*` | see D9, D10 |
| `getStartTime()` / `setStartTime(Date)` | `NewSimpleTrigger` (start arg), `StartTime` | match | `simple-forever-*` | |
| `getEndTime()` / `setEndTime(Date)` | `WithEndTime` | differs | `simple-end-time-equals-fire`, `simple-end-time-before-start` | D9, D10 |
| `getRepeatCount()` / `setRepeatCount(int)` | `RepeatCount`, `NewSimpleTrigger` | differs | `simple-repeat-negative-2` | D10 |
| `getRepeatInterval()` / `setRepeatInterval(long)` | `Interval`, `NewSimpleTrigger` | differs | `simple-interval-zero`, `simple-interval-negative` | D10 |
| `getTimesTriggered()` / `setTimesTriggered(int)` | `TimesTriggered` (getter only) | untested | — | no setter in the port |
| `computeFirstFireTime(Calendar)` | `ComputeFirstFireTime` | match | `misfire-ignore-simple-*` | both return the start time verbatim |
| `triggered(Calendar)` | `Triggered` | untested | — | compared indirectly through `FireTimeAfter` |
| `getFireTimeBefore(Date)` | — | missing | — | |
| `computeNumTimesFiredBetween(Date, Date)` | — | missing | — | |
| `getFinalFireTime()` | — | missing | — | |
| `mayFireAgain()` | `WillFireAgain` | untested | — | |
| `updateAfterMisfire(Calendar)` | `UpdateAfterMisfire` | match (ignore path only) | `misfire-ignore-simple-*` | |
| `updateWithNewCalendar(Calendar, long)` | — | missing | — | |
| `validate()` | — | missing | — | the port validates nothing (D10) |
| `REPEAT_INDEFINITELY` (-1) | `quartz.RepeatForever` (-1) | match | `simple-forever-*` | |
| `MISFIRE_INSTRUCTION_FIRE_NOW` (1) | `quartz.MisfireFireNow` (1) | untested | — | |
| `MISFIRE_INSTRUCTION_RESCHEDULE_NOW_WITH_EXISTING_REPEAT_COUNT` (2) | — | missing | — | |
| `MISFIRE_INSTRUCTION_RESCHEDULE_NOW_WITH_REMAINING_REPEAT_COUNT` (3) | — | missing | — | |
| `MISFIRE_INSTRUCTION_RESCHEDULE_NEXT_WITH_REMAINING_COUNT` (4) | — | missing | — | |
| `MISFIRE_INSTRUCTION_RESCHEDULE_NEXT_WITH_EXISTING_COUNT` (5) | — | missing | — | |
| — | `SimpleTrigger.String` | extra | — | debug string |

## `org.quartz.CalendarIntervalTrigger` / `CalendarIntervalTriggerImpl`

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `getFireTimeAfter(Date)` | `CalendarIntervalTrigger.FireTimeAfter` | differs | 29 `cal-*` | see D11, D12 |
| `getRepeatIntervalUnit()` / `setRepeatIntervalUnit(IntervalUnit)` | `Unit`, `NewCalendarIntervalTrigger` | differs | `cal-millisecond-unit` | no `MILLISECOND` unit in the port |
| `getRepeatInterval()` / `setRepeatInterval(int)` | `Count`, `NewCalendarIntervalTrigger` | differs | `cal-count-zero`, `cal-count-negative` | D13 |
| `getTimeZone()` / `setTimeZone(TimeZone)` | `In`, `Location` | differs | 7 zoned `cal-*` | D12 |
| `isPreserveHourOfDayAcrossDaylightSavings()` / setter | `PreserveWallClock` | differs | `cal-day-across-*-preserve` | different default and different meaning (D12) |
| `isSkipDayIfHourDoesNotExist()` / setter | — | missing | `cal-day-in-nonexistent-local-time` | not ported |
| `getStartTime()` / `setStartTime(Date)` | `StartTime` | match | `cal-after-before-start` | |
| `getEndTime()` / `setEndTime(Date)` | `WithEndTime` | untested | — | |
| `getTimesTriggered()` / setter | `TimesTriggered` (getter only) | untested | — | |
| `computeFirstFireTime(Calendar)` | `ComputeFirstFireTime` | untested | — | |
| `triggered(Calendar)` | `Triggered` | untested | — | compared indirectly through `FireTimeAfter` |
| `getFinalFireTime()` | — | missing | — | |
| `mayFireAgain()` | `WillFireAgain` | untested | — | |
| `updateAfterMisfire(Calendar)` | `UpdateAfterMisfire` | untested | — | not deterministically comparable (see "Misfire") |
| `updateWithNewCalendar` | — | missing | — | |
| `validate()` | — | missing | — | |
| `MISFIRE_INSTRUCTION_FIRE_ONCE_NOW` (1) | `MisfireFireNow` (1) | untested | — | |
| `MISFIRE_INSTRUCTION_DO_NOTHING` (2) | `MisfireDoNothing` (3) | differs | — | |
| — | `PreviousFireTime`, `String` | extra | — | |

## `org.quartz.DailyTimeIntervalTrigger` / `DailyTimeIntervalTriggerImpl`

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `getFireTimeAfter(Date)` | — (no pure query; `ComputeFirstFireTime` + `Triggered`) | differs | 26 `dtit-*` | see D14–D17 |
| `getStartTimeOfDay()` / `setStartTimeOfDay(TimeOfDay)` | `StartTimeOfDay`, `NewDailyTimeIntervalTrigger` | match | `dtit-hourly-window-utc` etc. | |
| `getEndTimeOfDay()` / `setEndTimeOfDay(TimeOfDay)` | `EndTimeOfDay`, `NewDailyTimeIntervalTrigger` | differs | `dtit-inverted-window` | D14 |
| `getDaysOfWeek()` / `setDaysOfWeek(Set<Integer>)` | `DaysOfWeek`, `OnDaysOfWeek` | match | `dtit-monday-only`, `dtit-weekend-only`, `dtit-sunday-only` | numbering differs (`Calendar.SUNDAY==1` vs `time.Sunday==0`) but the harness maps names, and the resulting fire times agree |
| `getRepeatIntervalUnit()` / setter | `Unit` | differs | `dtit-day-unit-unsupported` | D15 |
| `getRepeatInterval()` / setter | `Count` | differs | `dtit-count-zero` | D13 |
| `getRepeatCount()` / `setRepeatCount(int)` | `RepeatCount`, `WithRepeatCount` | untested | — | |
| `getStartTime()` / `setStartTime(Date)` | `StartingAt`, `StartTime` | match | `dtit-after-before-start`, `dtit-start-mid-window` | |
| `getEndTime()` / `setEndTime(Date)` | `EndingAt`, `EndTime` | untested | — | |
| *(none — no per-trigger time zone in 2.5.0)* | `In`, `Location` | extra | 4 zoned `dtit-*` | upstream resolves the window against the JVM default zone; the runner installs the requested zone as the default to make the comparison possible |
| `computeFirstFireTime(Calendar)` | `ComputeFirstFireTime` | differs | 26 `dtit-*` | |
| `triggered(Calendar)` | `Triggered` | differs | 26 `dtit-*` | |
| `getTimesTriggered()` / setter | `TimesTriggered` (getter only) | untested | — | |
| `getFinalFireTime()` | — | missing | — | |
| `mayFireAgain()` | `WillFireAgain` | untested | — | |
| `updateAfterMisfire(Calendar)` | `UpdateAfterMisfire` | untested | — | not deterministically comparable |
| `validate()` | — | missing | — | |
| `REPEAT_INDEFINITELY` (-1) | `quartz.RepeatForever` (-1) | match | — | shared sentinel |
| `MISFIRE_INSTRUCTION_FIRE_ONCE_NOW` / `_DO_NOTHING` | `MisfireFireNow` / `MisfireDoNothing` | differs | — | |

## `org.quartz.TimeOfDay`, `org.quartz.DateBuilder$IntervalUnit`, `org.quartz.Trigger`

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `TimeOfDay(int,int,int)` | `quartz.NewTimeOfDay` | match | all `dtit-*` | |
| `TimeOfDay(int,int)` | — | missing | — | no two-arg form |
| `hourMinuteAndSecondOfDay`, `hourAndMinuteOfDay` | — | missing | — | static factories |
| `getHour()` / `getMinute()` / `getSecond()` | `TimeOfDay.Hour/.Minute/.Second` fields | match | all `dtit-*` | exported struct fields, not methods |
| `before(TimeOfDay)` | — | missing | — | |
| `getTimeOfDayForDate(Date)` | — | missing | — | |
| `hourAndMinuteAndSecondFromDate([TimeZone])` | — | missing | — | |
| `hourAndMinuteFromDate([TimeZone])` | — | missing | — | |
| `IntervalUnit.MILLISECOND` | — | missing | `cal-millisecond-unit` | |
| `IntervalUnit.SECOND/MINUTE/HOUR/DAY/WEEK/MONTH/YEAR` | `quartz.IntervalSecond` … `IntervalYear` | match | `cal-second-45`, `cal-minute-90`, `cal-hour-6`, `cal-day-1`, `cal-week-1`, `cal-month-6`, `cal-year-1` | |
| `Trigger.MISFIRE_INSTRUCTION_SMART_POLICY` (0) | `quartz.MisfireSmart` (0) | untested | — | not comparable (see "Misfire") |
| `Trigger.MISFIRE_INSTRUCTION_IGNORE_MISFIRE_POLICY` (-1) | `quartz.MisfireIgnore` (2) | differs | `misfire-ignore-*` | the *behaviour* matches; the constant does not |
| `Trigger.getKey/getJobKey/getDescription` | `Key`, `JobKey`, `Description` | untested | — | identity only |
| `Trigger.getPriority`, `getCalendarName`, `getJobDataMap` | — | missing | — | not on the port's `Trigger` interface |
| `Trigger.compareTo` | — | missing | — | |
| — | `quartz.CalendarTrigger` / `quartz.WithCalendar` | extra | — | Go-only decorator replacing `updateWithNewCalendar` |
| — | `quartz.CalendarIntervalTrigger.PreserveWallClock` | extra | `cal-day-across-*-preserve` | no direct upstream equivalent |

## Misfire instructions — why only one path is compared

Upstream's `updateAfterMisfire` is implemented against `new Date()` in every
trigger class (`CronTriggerImpl`, `SimpleTriggerImpl`,
`CalendarIntervalTriggerImpl`, `DailyTimeIntervalTriggerImpl`). It is therefore
**not a pure function of a supplied instant** and cannot be compared
deterministically without patching upstream, which would destroy its value as an
oracle. The one instruction whose handling reads no clock on either side is
`MISFIRE_INSTRUCTION_IGNORE_MISFIRE_POLICY` / `quartz.MisfireIgnore`: it must
leave the computed next fire time untouched. That path is compared by the
`misfire` group (8 cases, all matching). Every other instruction is `untested`
with this reason, and the numeric constants themselves `differ` because the port
defines its own `MisfirePolicy` iota rather than reusing Quartz's wire values.

## Divergences found

Every item below is a real, reproducible difference in fire-time computation,
observed by running both implementations.

**D1 — `CronExpression.Next` never leaves the repeated hour of a fall-back
transition (hang / wrong answer).** The highest-severity finding. The port's
`startOfNextHour` is `time.Date(y, m, d, hour, 0, 0, 0, loc).Add(time.Hour)`;
for an ambiguous local hour `time.Date` always resolves to the *first* (DST)
occurrence, so adding an hour lands back on the *second* occurrence of the same
wall-clock hour and the search cannot advance.
- `tz-ny-fall-back-across` (`0 0 9 * * ?`, `America/New_York`, from
  2026-10-30T13:00:01Z): the port **loops forever** — the harness has to kill and
  restart the runner (`runnerRestarts.go == 1` in `parity.json`).
- `tz-ny-fall-back-hourly` (`0 0 * * * ?` from 2026-11-01T04:00:00Z): upstream
  `06:00, 07:00, 08:00, 09:00, 10:00`; port `05:00, 06:00, 06:00, 06:00, 06:00` —
  it emits the same instant forever.
- `tz-ny-fall-back-ambiguous-0130` (`0 30 1 * * ?`): upstream picks 01:30 EDT
  once per day (`11-01T06:30`, `11-02T06:30`, `11-03T06:30`); the port returns
  `11-01T05:30` three times.

**D2 — day-of-week numbering.** Quartz uses 1–7 with **1 = Sunday**; the port
uses Unix numbering, 0–6 with 0 = Sunday (and accepts 7 as Sunday). Confirmed:
`next-dow-numeric-1` (upstream fires on Sundays 2026-01-04…, port on Mondays
2026-01-05…), `next-dow-numeric-2` (upstream Monday, port Tuesday),
`next-dow-numeric-range` (`2-6` is Mon–Fri upstream, Tue–Sat in the port),
`next-dow-numeric-7` (upstream Saturday, port Sunday),
`sat-dow-numeric-1-on-sunday` / `-on-monday` (the predicate is inverted),
`parse-dow-numeric-0` (upstream rejects `0`, the port accepts it). **Weekday
*names* do agree** — `next-dow-name-monfri`, `next-dow-name-sun`,
`next-dow-name-sat`, `next-dow-name-list` all match — so only numeric
day-of-week fields are affected.

**D3 — no year field / no 7-field form.** `parse-seven-field-year`,
`parse-seven-field-year-range`, `parse-seven-field-year-star`,
`next-seven-field-year-bounded`, `next-seven-field-year-range`: upstream accepts
6- or 7-field expressions; the port accepts 5 or 6 and rejects 7 outright.

**D4 — 5-field expressions.** `parse-five-field`, `crontrig-bad-expression`:
upstream rejects `0 12 * * *` ("Unexpected end of expression"); the port accepts
it and silently prepends a `0` seconds field. A 5-field crontab line therefore
schedules successfully on the port and fails upstream.

**D5 — `?` vs `*` day-field rules.** Quartz requires exactly one of
day-of-month / day-of-week to be `?`:
- both `*` (`parse-star-both-day-fields`, `next-dow-and-dom-both-star`) —
  upstream `ParseException: Support for specifying both a day-of-week AND a
  day-of-month parameter is not implemented`; the port accepts.
- both restricted (`parse-numeric-both-day-fields`, `next-dow-and-dom-both-set`)
  — upstream rejects; the port accepts and applies Unix **OR** semantics
  (`0 0 12 15 * 3` fires on the 15th *or* every Tuesday).
- both `?` (`parse-question-both-day-fields`, `next-question-both-day-fields`) —
  upstream `ParseException: '?' can only be specified for Day-of-Month -OR-
  Day-of-Week`; the port accepts and treats `?` as `*`.

**D6 — `L`, `L-n`, `LW`, `nW`, `nL`, `n#m` are not implemented.** All rejected by
the port's parser: `parse-l-dom`, `parse-l-offset-dom`, `parse-lw-dom`,
`parse-w-dom`, `parse-l-dow`, `parse-hash-dow`, `parse-hash-dow-name`,
`next-l-last-day-of-month`, `next-l-offset`, `next-lw-last-weekday`,
`next-w-nearest-weekday`, `next-w-first-of-month`, `next-dow-last-friday`,
`next-dow-third-friday`, `next-dow-first-monday-name`, `sat-l-last-day`,
`sat-hash-third-friday`, `crontrig-monthly-last-day`. `L`/`W`/`#` cover the
common "last day of month", "nearest weekday" and "third Friday" schedules, so
this is the largest functional gap.

**D7 — reversed ranges.** `parse-reversed-range` (`30-10` in minutes) and
`parse-dow-reversed-range` (`FRI-MON`): upstream accepts both, wrapping around
the field; the port rejects them as out of range.

**D8 — step validation.** `parse-step-zero` (`*/0`) and `parse-step-negative`
(`*/-1`): upstream accepts (treating the step as 1); the port rejects. Here the
port is arguably stricter-and-better, but it is still a difference.

**D9 — `SimpleTrigger` end-time boundary is inclusive in the port.**
`simple-end-time-equals-fire` (end == 03:00, interval 1h): upstream stops at
02:00; the port also emits the 03:00 fire. Upstream's test is
`endMillis <= fireTime`, the port's is `fire.After(endTime)`.

**D10 — no argument validation on `SimpleTrigger` / interval triggers.**
`simple-interval-zero` (upstream `ArithmeticException: / by zero`),
`simple-interval-negative` (`Repeat interval must be >= 0`),
`simple-repeat-negative-2` (`Repeat count must be >= 0, use the constant
REPEAT_INDEFINITELY`), `simple-end-time-before-start` (`End time cannot be
before start time`): upstream throws in all four; the port silently yields a
trigger that never fires.

**D11 — `CalendarIntervalTrigger` month/year addition overflows instead of
clamping (the known port bug, confirmed).** The port uses `time.Time.AddDate`,
which normalises day overflow forward; `java.util.Calendar.add(MONTH)` clamps to
the last valid day of the target month.
- `cal-month-1-from-jan-31`: 2026-01-31 + 1 month — upstream **2026-02-28**, port
  **2026-03-03**. The port then stays on the 3rd forever (03-03, 04-03, 05-03…),
  while upstream stays on the 28th.
- `cal-month-1-from-mar-31`: upstream 2026-04-30, port 2026-05-01.
- `cal-month-1-from-jan-30-leap`: upstream 2028-02-29, port 2028-03-01.
- `cal-month-2-from-dec-31`: upstream 2027-02-28, port 2027-03-03.
- `cal-year-1-from-leap-day`: 2028-02-29 + 1 year — upstream 2029-02-28, port
  2029-03-01.

**D12 — `CalendarIntervalTrigger` DST handling is inverted by default.**
Upstream's plain `Calendar.add(DAY_OF_YEAR/WEEK/MONTH)` preserves the local
wall-clock time of day; the port's default (`preserveWallClock == false`) shifts
the result by the change in UTC offset, preserving the absolute time of day
instead. A 09:00-local daily calendar-interval trigger therefore drifts to 10:00
local after a spring-forward.
- `cal-day-across-spring-forward`: upstream `03-07T14:00, 03-08T13:00, 03-09T13:00`
  (09:00 local throughout); port `03-08T14:00, 03-09T14:00` (10:00 local).
- `cal-day-across-fall-back`, `cal-week-across-dst-berlin`,
  `cal-month-across-dst-ny`, `cal-day-in-nonexistent-local-time`: same shape.
- The `*-preserve` variants (`cal-day-across-spring-forward-preserve`,
  `cal-day-across-fall-back-preserve`) **match**, so `PreserveWallClock(true)` is
  the setting that reproduces upstream — i.e. the port's default is the wrong
  way round.

**D13 — zero / negative interval counts.** `cal-count-zero` (upstream
`ArithmeticException: / by zero` — itself an upstream bug, but still a
difference), `cal-count-negative` and `dtit-count-zero` (upstream
`IllegalArgumentException: Repeat interval must be >= 1`): the port returns a
trigger that fires once or never.

**D14 — inverted daily window.** `dtit-inverted-window` (window 17:00→09:00):
upstream ignores the inversion and fires at 17:00 on alternate days
(`01-01T17:00, 01-03T17:00, 01-05T17:00`); the port produces nothing.

**D15 — `DailyTimeIntervalTrigger` interval unit is not validated.**
`dtit-day-unit-unsupported` (unit `DAY`): upstream falls back to a 1-second step
(`09:00:00, 09:00:01, 09:00:02`); the port fires once per day at the window
start. Neither is obviously right, but they disagree.

**D16 — daily window inside a spring-forward gap.**
`dtit-zoned-ny-window-in-gap` (02:00–03:00 local, 30-minute step, New York,
from 2026-03-07T12:00Z): upstream skips 2026-03-08 entirely except for the
single 03:00 slot (`03-08T07:00`, then 03-09 and 03-10); the port produces three
slots on 03-08 (`06:00, 06:30, 07:00`) because `time.Date` folds the missing
02:00 and 02:30 onto 03:00.

**D17 — daily window inside a fall-back repeated hour.**
`dtit-zoned-ny-across-fall-back` (01:00–02:00 local, New York): upstream
`11-01T06:00, 06:30, 07:00`; the port `11-01T05:00, 05:30, 07:00` — it picks the
first (EDT) occurrence of 01:00/01:30 where upstream picks the second (EST).

### Cron syntax and trigger types the port lacks

- `L`, `L-n`, `LW`, `nW`, `nL`, `n#m` (D6).
- The year field and the 7-field expression form (D3).
- Wrapping/reversed ranges (D7).
- `IntervalUnit.MILLISECOND` (`cal-millisecond-unit`).
- `isSkipDayIfHourDoesNotExist` on `CalendarIntervalTrigger`.
- Backwards and terminal queries: `getTimeBefore`, `getNextInvalidTimeAfter`,
  `getFinalFireTime`, `SimpleTriggerImpl.getFireTimeBefore`,
  `computeNumTimesFiredBetween`.
- A pure `getFireTimeAfter` on `CronTrigger` and `DailyTimeIntervalTrigger`
  (`SimpleTrigger` and `CalendarIntervalTrigger` do have `FireTimeAfter`).
- Trigger priority, calendar name and job-data map on the `Trigger` interface.
- Quartz's misfire-instruction constants (the port has its own 4-value enum
  covering `SMART`, `FIRE_NOW`, `IGNORE`, `DO_NOTHING`; upstream's five
  `SimpleTrigger` reschedule instructions have no equivalent).

Not lacking, and worth noting as a genuine improvement: the port gives
`DailyTimeIntervalTrigger` an explicit `In(*time.Location)`, which upstream 2.5.0
does not have at all (it uses the JVM default zone).

## Counts

Statuses are counted over the symbol rows in the tables above.

| status | symbols |
| --- | --- |
| match | 17 |
| differs | 28 |
| missing | 38 |
| extra | 6 |
| untested | 25 |
| **total enumerated (fire-time surface)** | **114** |

Parity over the symbols actually compared (`match + differs`):
**17 / 45 = 37.8%**.

Parity over cases (the authoritative score, regenerated into `parity.json` by
`go test`): **158 / 225 = 70.2%**.

Out of scope, all `untested` — the harness deliberately does not compare live
scheduling: `Scheduler`, `SchedulerFactory`, `SchedulerContext`,
`SchedulerMetaData`, `ListenerManager`, `JobListener`, `TriggerListener`,
`SchedulerListener`, `Matcher`, `JobDetail`, `JobBuilder`, `JobDataMap`,
`JobExecutionContext`, `Job`, `InterruptableJob`, `StatefulJob`, `JobKey`,
`TriggerKey`, `TriggerBuilder`, `ScheduleBuilder`, `CronScheduleBuilder`,
`SimpleScheduleBuilder`, `CalendarIntervalScheduleBuilder`,
`DailyTimeIntervalScheduleBuilder`, `TriggerUtils`, `org.quartz.Calendar`,
`ValueSet`, `DateBuilder` (the builder itself, as distinct from its
`IntervalUnit` enum), the four annotations (`DisallowConcurrentExecution`,
`PersistJobDataAfterExecution`, `ExecuteInJTATransaction`) and the seven
exception types. `DateBuilder`'s date helpers are additionally untestable here
because most of them read the wall clock.
