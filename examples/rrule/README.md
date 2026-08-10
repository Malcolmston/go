# rrule example

A single runnable program that exercises the real API of the **published**
`github.com/malcolmston/rrule` module (RFC 5545 recurrence rules) against fixed
reference timestamps, so its output is deterministic.

Resolved module version (from `go get github.com/malcolmston/rrule@latest`):

```
github.com/malcolmston/rrule v0.0.0-20260725030041-05426c3367ad
```

There is no `replace` directive: the example consumes the module exactly as an
outside user would, from the module proxy.

## Run

```sh
cd examples/rrule
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

The program prints labelled sections and, for every expansion it can compute by
hand, an `[ok]` / `[MISMATCH]` line plus a summary count. It always terminates
and exits 0. Current state: **105 checks, 0 mismatches**.

## What it demonstrates

- **Building rules** with `rrule.New(rrule.Options{…})` for every frequency
  (`Yearly` … `Secondly`), with `Interval`, `Count` and `Until`, plus the
  `Options()` / `DTStart()` / `Freq()` read-back accessors and `Freq.String()`.
- **BYxxx rule parts** — `ByWeekday` (plain and ordinal, via `FR.Nth(1)` /
  `SU.Nth(-1)`), `ByMonthDay` (including `-1`), `ByMonth`, `ByYearDay`,
  `ByWeekNo`, `ByHour`, and `BySetPos` (`-1` = last weekday of the month, `2` =
  second Tue-or-Thu). Includes the RFC 5545 §3.3.10 `WKST` worked example, where
  `WKST=MO` and `WKST=SU` give genuinely different answers, using the exported
  `rrule.SundayStart` constant, and `Weekday.String()`.
- **COUNT vs UNTIL** — UNTIL inclusivity, UNTIL one second earlier dropping the
  final occurrence, COUNT and UNTIL together, and `All()` being capped at
  `MaxAllOccurrences` on an unbounded rule.
- **Window queries** — `Between` (inclusive and exclusive), `After`, `Before`,
  a zero result past the end, and the pull-based `Iterator` (independence,
  exhaustion, and using it to bound a rule with ~2.5e11 occurrences).
- **Text round trips** — `Parse` (bare value and `RRULE:`-prefixed),
  `StrToRRule` (DTSTART + RRULE), `String()`, `RuleString()`, TZID
  serialization, a rule carrying every rule part, a UTC `UNTIL`, and RFC 5545
  §3.1 line unfolding.
- **Recurrence sets** — `NewSet`, `RRule`, `ExRule`, `RDate`, `ExDate`, the
  `RRules`/`ExRules`/`RDates`/`ExDates`/`DTStart` accessors, RDATE
  deduplication, two merged rules, set-level `All`/`Between`/`After`/`Before`/
  `Iterator`, `Set.String()`, `StrToSet`, and `StrToRRule` correctly refusing a
  block that is really a set.
- **iCalendar** — `Calendar.Encode` (CRLF, text escaping, DTEND, RRULE),
  `ParseCalendar` round trip including `Event.Set`, raw `Event.Props`, a VEVENT
  carrying RDATE/EXDATE, all-day `VALUE=DATE` events, and a malformed ICS
  producing an error rather than a panic.
- **DST** — wall-clock preservation across fall-back, the repeated hour
  resolving to the earlier offset, an HOURLY rule crossing the spring-forward
  gap without emitting a duplicate or out-of-order instant, and the RFC
  §3.8.5.3 hourly example where UNTIL is compared as an instant.
- **Errors and edges** — `New` rejecting a bad `Freq`, `BYMONTH=13`,
  `BYMONTHDAY=0`, `BYSETPOS=0`, `BYHOUR=24` and a negative interval; `Parse`
  rejecting an unknown FREQ, a bad BYDAY, an unknown rule part, a
  non-numeric COUNT and a bad UNTIL; an impossible rule
  (`FREQ=MONTHLY;BYMONTHDAY=31;BYMONTH=2`) terminating empty; a sparse leap-day
  rule; a DTSTART that does not match the BYxxx parts; sub-second DTSTART
  truncation.

## Holes found in the published module

1. **A very large `INTERVAL` panics during iteration.**
   `FREQ=DAILY;INTERVAL=9223372036854775807;COUNT=2` parses without error and
   then panics with `index out of range [-9223372036854775565]` when the rule is
   expanded. Any service that parses user-supplied RRULE text can be crashed by
   one line of input. The example wraps this case in `recover()` so the program
   still terminates, and prints the panic message. (The library's own working
   tree has since added a `maxInterval` bound; the published version has not.)

2. **`DTSTART` in a `time.FixedZone` serializes to an empty TZID and silently
   moves every occurrence.** `String()` emits
   `DTSTART;TZID=:19970902T090000` — not valid iCalendar — and `StrToRRule`
   re-parses that line *without error* in the default location, so a rule
   anchored at 09:00-05:00 comes back as 09:00 UTC. The instants differ by five
   hours with no diagnostic anywhere. A `FixedZone` with a name emits
   `TZID=EST`, which is likewise not a tzdata identifier.

3. **A rule with no `FREQ` at all is accepted and silently becomes
   `FREQ=YEARLY`.** `rrule.Parse("BYDAY=MO")` and `rrule.Parse("COUNT=3")` both
   succeed, yielding `FREQ=YEARLY;BYDAY=MO` and `FREQ=YEARLY;COUNT=3`. FREQ is
   the one mandatory rule part in RFC 5545 §3.3.10, and the package's own
   `ErrInvalidFreq` doc comment says it "indicates a **missing** or unrecognized
   FREQ rule part" — so this is a README/doc-vs-behaviour mismatch as well as a
   spec violation. (`FREQ=NEVER` *is* rejected.)

4. **Degenerate rule-part values are accepted and read as their opposite.**
   All three of these parse cleanly:
   - `COUNT=0` → treated as no COUNT, i.e. an **infinite** rule;
   - `INTERVAL=0` → treated as `INTERVAL=1`;
   - `BYMONTH=` (empty list) → treated as an **absent** rule part, letting
     DTSTART supply the default.
   RFC 5545 §3.3.10 spells COUNT and INTERVAL as positive integers and every
   BYxxx value as a non-empty list.

5. **A nonexistent local time is emitted at a different wall clock instead of
   being skipped.** A daily 02:30 rule in `America/New_York` starting
   2017-03-11 yields `02:30 EST`, then **`01:30 EST`** on 2017-03-12 (the day
   the gap removes 02:30), then `02:30 EDT`. RFC 5545 §3.3.10 says a
   nonexistent local time "MUST be ignored and MUST NOT be counted as part of
   the recurrence set". The published README documents following `time.Date`
   here, so it is a deliberate choice, but it is non-conformant and the
   occurrence it produces is at a time the rule never asks for. (The HOURLY
   case is handled: the duplicate instant is dropped and ordering is preserved.)

6. **No bounded `All`.** The published version has neither
   `(*RRule).AllLimit` nor `(*Set).AllLimit`, so for untrusted rule text the
   only safe entry points are `Iterator` and `Between`; `All()` is capped only
   when the rule is *unbounded*, and a rule that is finite-but-enormous
   (`FREQ=SECONDLY;UNTIL=99991231T235959Z`, ~2.5e11 occurrences) will try to
   materialize everything. This is documented, but it makes the obvious method
   the dangerous one.

## Smaller friction points

- `Options.Wkst` is a `time.Weekday` whose zero value has to mean Monday, so
  selecting Sunday requires the out-of-band constant `rrule.SundayStart`
  (`time.Weekday(7)`). Documented in API-DEVIATIONS.md, but it means
  `Wkst: time.Sunday` compiles and silently means Monday.
- `RuleString()` reorders rule parts relative to the input
  (`FREQ=WEEKLY;BYDAY=MO,WE;COUNT=6` comes back as
  `FREQ=WEEKLY;COUNT=6;BYDAY=MO,WE`), so text comparison of a round trip needs
  a re-parse rather than a string match. It is stable under repeated round
  trips.
- `Set` has no `RemoveRRule`/`Clear`; a set is append-only once built.

No compile failures and no dependency problems: the module is standard-library
only, and `go get`/`go mod tidy` resolved it from the proxy without incident.
