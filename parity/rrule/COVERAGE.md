# `rrule` parity coverage

- **Port:** `github.com/malcolmston/rrule v0.1.0` (published module, no `replace`)
- **Upstream oracle:** `python-dateutil==2.9.0.post0`, module `dateutil.rrule`
  (CPython 3.13.5)
- **Cases:** 261 across 17 groups — `GOWORK=off go test ./parity/rrule/`
- **Case score:** 224 match / 37 mismatch / 0 deviations → **85.82 %** (see
  `parity.json`, rewritten by the test)
- **Symbol score:** see [Totals](#totals) — **39 / 51 = 76.47 %** of the
  upstream symbols that could be compared behave identically.

Occurrences are compared as fixed-format ISO-8601 strings
(`YYYY-MM-DDTHH:MM:SS` plus a numeric `±HH:MM` offset when the rule is
zone-aware), so a one-hour or one-day error is a byte difference, never a
rounding question. Every case carries an explicit `DTSTART`; nothing reads the
clock.

## How the upstream inventory was produced

Mechanically, against the installed package — not from the README:

```console
$ python3 -c "import dateutil; print(dateutil.__version__)"
2.9.0.post0
$ python3 -c "import dateutil.rrule as R; print(sorted(n for n in dir(R) if not n.startswith('_')))"
['DAILY', 'FR', 'FREQNAMES', 'HOURLY', 'M365MASK', 'M365RANGE', 'M366MASK',
 'M366RANGE', 'MDAY365MASK', 'MDAY366MASK', 'MINUTELY', 'MO', 'MONTHLY',
 'NMDAY365MASK', 'NMDAY366MASK', 'SA', 'SECONDLY', 'SU', 'TH', 'TU',
 'WDAYMASK', 'WE', 'WEEKLY', 'YEARLY', 'advance_iterator', 'calendar',
 'datetime', 'easter', 'gcd', 'integer_types', 'itertools', 'parser', 'range',
 're', 'rrule', 'rrulebase', 'rruleset', 'rrulestr', 'sys', 'warn', 'weekday',
 'weekdaybase', 'weekdays', 'wraps']
$ python3 -c "import dateutil.rrule as R
for c in ('rrule','rrulebase','rruleset','weekday'):
    print(c, sorted(n for n in dir(getattr(R,c)) if not n.startswith('_')))"
rrule ['after', 'before', 'between', 'count', 'replace', 'xafter']
rrulebase ['after', 'before', 'between', 'count', 'xafter']
rruleset ['after', 'before', 'between', 'count', 'exdate', 'exrule', 'rdate', 'rrule', 'xafter']
weekday ['n', 'weekday']
$ python3 -c "import dateutil.rrule as R, inspect; print(inspect.signature(R.rrule.__init__))"
(self, freq, dtstart=None, interval=1, wkst=None, count=None, until=None,
 bysetpos=None, bymonth=None, bymonthday=None, byyearday=None, byeaster=None,
 byweekno=None, byweekday=None, byhour=None, byminute=None, bysecond=None,
 cache=False)
```

Names that `dir()` reports only because `dateutil.rrule` imports them
(`calendar`, `datetime`, `heapq`, `itertools`, `re`, `sys`, `parser`, `easter`,
`gcd`, `warn`, `wraps`, `advance_iterator`, `integer_types`, `range`) are not
part of the library's API and are excluded. Everything else appears below,
including the precomputed mask tables, which upstream exports without an
underscore.

The Go side was enumerated with `go doc -all github.com/malcolmston/rrule` on
the same pinned version.

## Module-level symbols

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `dateutil.rrule.YEARLY` | `rrule.Yearly` | match | `freq-yearly`, `freq-yearly-unbounded`, `interval-yearly-2` | |
| `dateutil.rrule.MONTHLY` | `rrule.Monthly` | match | `freq-monthly`, `freq-monthly-day31` | |
| `dateutil.rrule.WEEKLY` | `rrule.Weekly` | match | `freq-weekly`, `wkst-weekly-interval2-*` | |
| `dateutil.rrule.DAILY` | `rrule.Daily` | match | `freq-daily`, `daily-count10` | |
| `dateutil.rrule.HOURLY` | `rrule.Hourly` | match | `freq-hourly`, `hourly-3-until-1700` | |
| `dateutil.rrule.MINUTELY` | `rrule.Minutely` | match | `freq-minutely`, `minutely-15-count6` | |
| `dateutil.rrule.SECONDLY` | `rrule.Secondly` | match | `freq-secondly`, `secondly-filtered` | |
| `dateutil.rrule.MO` | `rrule.MO` | match | `byday-plain-mo`, `byweekno-20-mo` | |
| `dateutil.rrule.TU` | `rrule.TU` | match | `byday-plain-multi`, `us-election-day` | |
| `dateutil.rrule.WE` | `rrule.WE` | match | `byday-hourly`, `byweekno-multi` | |
| `dateutil.rrule.TH` | `rrule.TH` | match | `byday-plain-multi`, `bymonth-with-byday-th` | |
| `dateutil.rrule.FR` | `rrule.FR` | match | `byday-monthly-last-fr`, `friday-the-13th` | |
| `dateutil.rrule.SA` | `rrule.SA` | match | `saturday-after-first-sunday`, `byday-all-seven` | |
| `dateutil.rrule.SU` | `rrule.SU` | match | `byday-monthly-first-and-last-su`, `byweekno-1-wkst-su` | |
| `dateutil.rrule.weekday` | `rrule.Weekday` | match | `byday-plain-mo`, `byday-monthly-mixed-ordinals` | |
| `dateutil.rrule.weekdaybase` | `rrule.Weekday` | match | `byday-plain-mo` | upstream splits base/subclass; the port has one struct |
| `dateutil.rrule.weekday.n` | `rrule.Weekday.N` | match | `byday-monthly-1fr`, `byday-monthly-2nd-last-mo` | |
| `dateutil.rrule.weekday.weekday` | `rrule.Weekday.Day` | match | `byday-plain-mo` | |
| `dateutil.rrule.weekday.__call__(n)` | `rrule.Weekday.Nth` | match | `byday-monthly-plus1mo`, `byday-monthly-last-fr`, `byday-yearly-20mo` | `MO(+1)` / `FR(-1)` vs `MO.Nth(1)` / `FR.Nth(-1)` |
| `dateutil.rrule.weekdays` | — | missing | — | no aggregate `[7]Weekday` slice is exported |
| `dateutil.rrule.FREQNAMES` | `rrule.Freq.String` | match | `rulestring-daily-count`, `rulestring-byweekno-byday` | same seven spellings |
| `dateutil.rrule.rrule` | `rrule.RRule` / `rrule.New` | match | all 197 `expand` cases | |
| `dateutil.rrule.rrulebase` | (methods on `*RRule` and `*Set`) | match | `after-basic`, `before-basic`, `between-basic` | the port has no separate base type |
| `dateutil.rrule.rruleset` | `rrule.Set` / `rrule.NewSet` | match | `setbuild-*`, `set-*` | |
| `dateutil.rrule.rrulestr` | `rrule.Parse` / `rrule.StrToRRule` | differs | `parse-rrule-prefix`, `parse-trailing-semicolon`, `parse-whitespace`, `duplicate-part`, `byeaster-0` | the port accepts text upstream rejects, and rejects `BYEASTER` |
| `dateutil.rrule.WDAYMASK` | — | missing | — | precomputed weekday mask; the port's equivalent (`iterInfo`) is unexported |
| `dateutil.rrule.M365MASK` | — | missing | — | as above |
| `dateutil.rrule.M366MASK` | — | missing | — | as above |
| `dateutil.rrule.M365RANGE` | — | missing | — | as above |
| `dateutil.rrule.M366RANGE` | — | missing | — | as above |
| `dateutil.rrule.MDAY365MASK` | — | missing | — | as above |
| `dateutil.rrule.MDAY366MASK` | — | missing | — | as above |
| `dateutil.rrule.NMDAY365MASK` | — | missing | — | as above |
| `dateutil.rrule.NMDAY366MASK` | — | missing | — | as above |

## `rrule` / `rrulebase` methods

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `rrule.__iter__` | `rrule.RRule.Iterator` | match | every `expand` case | the port's `All` is bounded by `MaxAllOccurrences` |
| `rrule.__str__` | `rrule.RRule.String` | differs | `rulestring-until-utc`, `rulestring-byyearday`, `rulestring-unsorted-byday`, `rulestring-duplicate-bymonthday`, `rulestring-tz-named-zone` | see [Serialisation](#serialisation) |
| `rrulebase.after` | `rrule.RRule.After` | match | `after-basic`, `after-inclusive`, `after-past-end` | upstream `None` and the port's zero `time.Time` both encode as JSON `null` |
| `rrulebase.before` | `rrule.RRule.Before` | match | `before-basic`, `before-inclusive`, `before-start` | |
| `rrulebase.between` | `rrule.RRule.Between` | match | `between-basic`, `between-inclusive`, `between-exclusive-endpoints`, `between-empty`, `between-unbounded-rule` | |
| `rrulebase.count` | `len(rrule.RRule.All())` | match | `count-daily-10`, `count-until`, `count-empty` | |
| `rrule.replace` | — | missing | — | no "rebuild with changed parts" helper; `Options()` + `New()` is the manual equivalent |
| `rrulebase.xafter` | — | missing | — | no "iterate n occurrences after t" helper |
| `rrulebase.__getitem__` | — | missing | — | no index/slice access into a rule |
| `rrulebase.__contains__` | — | missing | — | no membership test |

## `rruleset` methods

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `rruleset.rrule` | `rrule.Set.RRule` | match | `setbuild-two-rules-tz`, `set-two-rrules`, `set-duplicate-instants` | |
| `rruleset.exrule` | `rrule.Set.ExRule` | match | `setbuild-exrule`, `set-exrule`, `set-exrule-weekday` | |
| `rruleset.rdate` | `rrule.Set.RDate` | match | `set-rdate`, `set-rdate-multi`, `setbuild-rrule-rdate-exdate` | |
| `rruleset.exdate` | `rrule.Set.ExDate` | match | `set-exdate`, `set-exdate-multi`, `set-exdate-cancels-rdate`, `set-friday-13th-exdate` | |
| `rruleset.__iter__` | `rrule.Set.Iterator` | match | every `set` / `setbuild` case | |
| `rruleset.after` | `rrule.Set.After` | untested | — | inherited from `rrulebase`; only the single-rule path is exercised |
| `rruleset.before` | `rrule.Set.Before` | untested | — | as above |
| `rruleset.between` | `rrule.Set.Between` | untested | — | as above |
| `rruleset.count` | `len(rrule.Set.All())` | untested | — | as above |
| `rruleset.xafter` | — | missing | — | |

## Constructor parameters — i.e. the RFC 5545 rule parts

The port has **every** RFC 5545 `RRULE` rule part. The only upstream rule part
it lacks is `byeaster`, which is a dateutil extension with no RFC counterpart.

| upstream symbol | RFC 5545 part | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- | --- |
| `rrule(freq=…)` | `FREQ` | `Options.Freq` | differs | `freq-*` (7), `bug-missing-freq`, `bug-missing-freq-byday`, `unknown-freq` | all seven frequencies agree; an **absent** `FREQ` is rejected upstream and silently read as `YEARLY` by the port |
| `rrule(dtstart=…)` | `DTSTART` | `Options.DTStart` | differs | `tz-*` (11), `bug-dst-*` (3), `set-tzid`, `set-utc-z` | zone handling of a non-existent / ambiguous wall clock diverges — see [DST](#dst-and-timezones) |
| `rrule(interval=…)` | `INTERVAL` | `Options.Interval` | differs | `interval-*` (10), `bug-interval-zero`, `bug-interval-zero-hourly`, `bug-huge-interval`, `bug-huge-interval-monthly`, `bug-large-interval`, `interval-negative` | normal intervals agree; `INTERVAL=0` and `INTERVAL=2^63-1` do not |
| `rrule(wkst=…)` | `WKST` | `Options.Wkst` | match | `wkst-weekly-interval2-{mo…su}` (7), `byweekno-20-wkst-su`, `byweekno-1-wkst-su`, `byweekno-1-wkst-mo-su`, `wkst-bad` | all seven week starts agree, including the RFC 5545 §3.8.5.3 WKST example |
| `rrule(count=…)` | `COUNT` | `Options.Count` | differs | `count-1`, `count-exact-limit`, `count-overrun-probe`, `bug-count-zero`, `bug-count-zero-with-until`, `count-negative`, `count-not-int` | `COUNT=0` means "none" upstream, "unlimited" in the port; `COUNT=-1` is accepted upstream, rejected by the port |
| `rrule(until=…)` | `UNTIL` | `Options.Until` | match | `until-utc`, `until-naive`, `until-equals-dtstart`, `until-utc-before-dtstart`, `until-mid-day`, `count-and-until`, `until-and-count-until-wins`, `until-malformed` | inclusive on both sides; `UNTIL` wins over `COUNT` on both sides |
| `rrule(bysetpos=…)` | `BYSETPOS` | `Options.BySetPos` | match | all 11 `bysetpos-*`, `weekly-byday-bysetpos` | including the two RFC 5545 §3.8.5.3 BYSETPOS examples and out-of-range positions |
| `rrule(bymonth=…)` | `BYMONTH` | `Options.ByMonth` | differs | `bymonth-*` (10), `bymonth-13`, `bymonth-0` | in-range values agree; out-of-range values are ignored upstream, rejected by the port |
| `rrule(bymonthday=…)` | `BYMONTHDAY` | `Options.ByMonthDay` | differs | `bymonthday-*` (12) | negatives, `-31`, Feb 29 and "invalid date ignored" all agree; `0` / `32` diverge |
| `rrule(byyearday=…)` | `BYYEARDAY` | `Options.ByYearDay` | differs | `byyearday-*` (8) | `1,100,200`, `366`, `-366` and MONTHLY misuse agree; `0` / `367` diverge |
| `rrule(byweekno=…)` | `BYWEEKNO` | `Options.ByWeekNo` | differs | `byweekno-*` (11 + 7 WKST variants) | weeks 1/20/53/-1 and every WKST agree; `0` / `54` diverge |
| `rrule(byweekday=…)` | `BYDAY` | `Options.ByWeekday` | differs | all 19 `byday-*`, `byday-empty-value` | plain, `+1MO`, `-1FR`, `-2MO`, `5SU`, `20MO`, `53MO`, mixed ordinals and ordinal-ignored-below-MONTHLY all agree; an empty `BYDAY=` value is rejected upstream, accepted by the port |
| `rrule(byhour=…)` | `BYHOUR` | `Options.ByHour` | match | `byhour-daily`, `byhour-byminute-daily`, `byhour-minutely`, `byhour-hourly-filter`, `byhour-yearly`, `byhour-24`, `byhour-unreachable-interval` | |
| `rrule(byminute=…)` | `BYMINUTE` | `Options.ByMinute` | match | `byminute-daily`, `byminute-hourly-expand`, `byminute-60`, `all-three-times` | |
| `rrule(bysecond=…)` | `BYSECOND` | `Options.BySecond` | differs | `bysecond-daily`, `bysecond-minutely-expand`, `bysecond-60`, `bysecond-61` | `BYSECOND=60` (the RFC 5545 leap second) is rejected upstream and accepted by the port, which folds it into the next minute |
| `rrule(byeaster=…)` | — (dateutil extension) | — | missing | `byeaster-0`, `byeaster-negative` | not an RFC 5545 rule part; the port rejects it as an unknown rule part |
| `rrule(cache=…)` | — | missing | — | the port's rules are immutable and every iterator is independent, so there is nothing to cache |

## `rrulestr` keyword arguments

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `rrulestr(forceset=True)` | `rrule.StrToSet` | match | all 20 `set-*` cases | the port splits the two return types into two functions instead of a flag |
| `rrulestr(dtstart=…)` | — | missing | — | the port takes the anchor only from a `DTSTART` line; the runner works around this with `Options()` + `New()` |
| `rrulestr(unfold=…)` | (always unfolds) | untested | — | the port unfolds RFC 5545 continuation lines unconditionally |
| `rrulestr(ignoretz=…)` | — | missing | — | |
| `rrulestr(tzids=…)`, `rrulestr(tzinfos=…)` | — | missing | — | the port always resolves `TZID` through `time.LoadLocation` |
| `rrulestr(compatible=…)` | — | missing | — | no pre-RFC compatibility mode |
| `rrulestr(cache=…)` | — | missing | — | |

## Go-only surface

Present in the port, absent from `dateutil.rrule`. Listed for completeness;
extras cannot raise or lower the parity score.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| — | `rrule.RRule.RuleString` | extra | all 13 `rulestring-*` | the `RRULE` value without the property name |
| — | `rrule.SundayStart` | extra | `byweekno-*-wkst-su`, `wkst-weekly-interval2-su` | `Options.Wkst`'s zero value must mean Monday, so Sunday needs a sentinel |
| — | `rrule.MaxAllOccurrences` | extra | — | documented bound on `All` for an unbounded rule |
| — | `rrule.RRule.Options` / `.DTStart` / `.Freq` | extra | every case (the runner re-anchors through `Options()`) | read the compiled rule back |
| — | `rrule.Set.RRules` / `.ExRules` / `.RDates` / `.ExDates` / `.DTStart` / `.String` | extra | — | read a set's parts back |
| — | `rrule.ErrInvalidRule` / `ErrInvalidFreq` / `ErrInvalidWeekday` / `ErrEmptyRule` / `ErrParse` / `ErrNoRRule` / `ErrInvalidCalendar` | extra | every failing case | `errors.Is`-classifiable sentinels; upstream raises bare `ValueError`/`TypeError` |
| — | `rrule.Calendar` / `Event` / `Property` / `ParseCalendar` / `Calendar.Encode` | extra | — | an iCalendar container layer; outside `dateutil.rrule` entirely |

## Totals

Counted over the tables above.

| status | symbols |
| --- | --- |
| match | 39 |
| differs | 12 |
| missing | 22 |
| untested | 5 |
| extra | 7 |

**Parity = 39 / (39 + 12) = 76.47 %** of the upstream symbols that could be
compared (85 symbols in total: 39 match, 12 differ, 22 missing, 5 untested, 7
Go-only extras). The 22 `missing` symbols are the 9 internal mask tables, 5
`rrulestr` flags, `weekdays`, `replace`, `xafter` (on both `rrulebase` and
`rruleset`), `__getitem__`, `__contains__`, `byeaster` and the `cache` flag —
**no RFC 5545 rule part is among them.**

Case totals: **261 cases, 224 match, 37 mismatch, 0 deviations (85.82 %)**.

## Divergences

Ordered by severity: wrong dates first, then over-permissive parsing, then
serialisation.

### DST and timezones

The port generates occurrences from wall-clock fields via `time.Date`, which
*normalises* a wall clock that a spring-forward transition removed. Upstream
keeps the requested wall clock and pairs it with the pre-transition offset. RFC
5545 §3.3.5 says an instance at a non-existent local time is skipped — **neither
side does that**, and they disagree about what to emit instead. Five cases,
every one a wrong date:

| case | upstream | go |
| --- | --- | --- |
| `tz-ny-spring-forward-0230` | `2024-03-10T02:30:00-04:00` | `2024-03-10T01:30:00-05:00` |
| `bug-dst-spring-forward-daily` | `2024-03-10T02:30:00-04:00` | `2024-03-10T01:30:00-05:00` |
| `bug-dst-spring-forward-weekly` | `2024-03-10T02:30:00-04:00` | `2024-03-10T01:30:00-05:00` |
| `bug-dst-spring-forward-berlin` | `2024-03-31T02:30:00+02:00` | `2024-03-31T03:30:00+02:00` |
| `setbuild-two-rules-tz` | `2024-03-10T02:30:00-04:00` | `2024-03-10T01:30:00-05:00` |

Note that the port's error is not even self-consistent: in `America/New_York`
the missing 02:30 comes back an hour *earlier* (01:30 EST) while in
`Europe/Berlin` it comes back an hour *later* (03:30 CEST). Both are one hour
away from upstream's instant, so a caller who stores the result as UTC gets a
different instant on either side of the gap. `API-DEVIATIONS.md` claims this
case "is resolved with the offset in effect before the transition", which would
give `02:30-05:00`; that is not what v0.1.0 produces, so these are scored as
mismatches rather than declared deviations.

Two further zone divergences:

- `tz-ny-spring-forward-hourly` — an `FREQ=HOURLY` rule crossing the gap.
  Upstream emits `00:00-05:00, 01:00-05:00, 02:00-04:00, 03:00-04:00, …`; the
  port emits `00:00, 01:00, 03:00, 04:00, …`, i.e. it drops one occurrence and
  is one step ahead from the transition on. Over eight occurrences the port
  ends at 08:00 where upstream ends at 07:00.
- `tz-australia-sydney-daily` — an *ambiguous* wall clock (`2024-04-07T02:30`
  in `Australia/Sydney`, fall-back). Upstream resolves it to the first pass
  (`+11:00`), the port to the second (`+10:00`) — one hour apart. The same
  ambiguity in `America/New_York` (`tz-ny-fall-back-0130`,
  `set-tzid-across-dst`) happens to agree, so the disagreement is
  zone-dependent, not a uniform fold policy.

The zone-agnostic behaviour is otherwise solid: fixed-offset zones
(`Asia/Kathmandu`, `+05:45`), naive DTSTARTs, UTC, `DTSTART;TZID=` inside a set,
fall-back in New York and 14 monthly occurrences across two US transitions all
match exactly.

### Panic on a huge INTERVAL

`FREQ=DAILY;INTERVAL=9223372036854775807;COUNT=2` **panics** the port:

```
panic: runtime error: index out of range [-9223372036854775565]
```

(`bug-huge-interval`; the MONTHLY form panics identically with
`[-9223372036854775801]`). The interval is added to a day-of-year index and
overflows `int`. Upstream simply yields `DTSTART` and stops. The Go runner wraps
every case in `recover()` precisely so this reports `ok:false` instead of
killing the process and losing the remaining 200-odd cases. `INTERVAL=1000000000`
(`bug-large-interval`) is handled correctly by both, so the fault is overflow,
not size.

### Wrong occurrences from degenerate limits

| case | rule | upstream | go |
| --- | --- | --- | --- |
| `bug-count-zero` | `FREQ=DAILY;COUNT=0` | *(no occurrences)* | unlimited — 5 dates and counting |
| `bug-count-zero-with-until` | `FREQ=DAILY;COUNT=0;UNTIL=…` | *(no occurrences)* | 4 dates |
| `bug-interval-zero` | `FREQ=DAILY;INTERVAL=0;COUNT=3` | `09-02` ×3 (no advance) | `09-02, 09-03, 09-04` |
| `bug-interval-zero-hourly` | `FREQ=HOURLY;INTERVAL=0;COUNT=3` | `09:00` ×3 | `09:00, 10:00, 11:00` |

`Options.Count == 0` is the port's encoding of "unlimited" and
`Options.Interval == 0` its encoding of "1", so a parsed `COUNT=0` / `INTERVAL=0`
is indistinguishable from the part being absent. Both are documented sentinels
for the *Go struct*; the bug is that the *parser* forwards the RFC 5545 values
into them.

### Over-permissive parsing (port accepts, upstream rejects)

| case | input | upstream | go |
| --- | --- | --- | --- |
| `bug-missing-freq` | `COUNT=3` | `TypeError: missing … 'freq'` | 3 yearly occurrences |
| `bug-missing-freq-byday` | `BYDAY=MO;COUNT=3` | `TypeError: missing … 'freq'` | 3 yearly occurrences |
| `parse-trailing-semicolon` | `FREQ=DAILY;COUNT=3;` | `ValueError` | 3 occurrences |
| `parse-whitespace` | `" FREQ=DAILY ; COUNT=3 "` | `ValueError` | 3 occurrences |
| `byday-empty-value` | `FREQ=WEEKLY;BYDAY=;COUNT=3` | `ValueError: invalid 'BYDAY'` | 3 occurrences |
| `bysecond-60` | `FREQ=DAILY;BYSECOND=60;COUNT=3` | `ValueError: second must be in 0..59` | `09:01:00` ×3 |

`FREQ` is the important one: `Freq` is a `Freq(int)` whose zero value is
`Yearly`, so a rule with no `FREQ` at all compiles into a yearly rule instead of
failing. Tolerating a trailing `;` or surrounding whitespace is arguably an
improvement; silently inventing a frequency is not. `BYSECOND=60` is the RFC
5545 leap second, which the RFC does allow — here the port is right and dateutil
is strict, but it still scores as a mismatch because upstream is the oracle.

### Over-strict parsing (port rejects, upstream accepts)

Upstream ignores out-of-range `BYxxx` values (producing an empty series); the
port returns `ErrInvalidRule`. Nine cases: `bymonth-0`, `bymonth-13`,
`bymonthday-zero`, `bymonthday-32`, `byweekno-0`, `byweekno-54`, `byyearday-0`,
`byyearday-367`, `count-negative`. It also rejects a duplicated rule part
(`duplicate-part`: `FREQ=DAILY;COUNT=3;COUNT=4`), where upstream lets the last
one win. Failing loudly on nonsense is defensible, but it is not parity, and a
caller feeding through third-party iCalendar data will see errors where dateutil
sees an empty recurrence.

`BYEASTER` (`byeaster-0`, `byeaster-negative`) is rejected as an unknown rule
part. That is a genuinely missing feature rather than strictness, though it is a
dateutil extension, not RFC 5545.

### Serialisation

`rulestring-*` compares the `RRULE` line only, with its parts uppercased and
sorted, because upstream's `__str__` drops the zone from its `DTSTART` line
while the port emits `DTSTART;TZID=…` (`rulestring-tz-named-zone`, which
therefore compares equal on the rule part alone). Four differences remain:

| case | upstream | go |
| --- | --- | --- |
| `rulestring-until-utc` | `UNTIL=19971224T000000` | `UNTIL=19971224T000000Z` |
| `rulestring-byyearday` | `BYYEARDAY=-1,1,100,200` | `BYYEARDAY=1,100,200,-1` |
| `rulestring-unsorted-byday` | `BYDAY=MO,WE,FR` | `BYDAY=FR,MO,WE` |
| `rulestring-duplicate-bymonthday` | `BYMONTHDAY=15` | `BYMONTHDAY=15,15` |

Upstream normalises list values (sorted, de-duplicated) and the port echoes what
was parsed. Only the first is a defect, and it is upstream's: dropping the `Z`
makes dateutil's own output un-re-parseable, which is exactly what
`roundtrip-until-utc` records — feeding `str(rule)` back into `rrulestr` with the
original aware `DTSTART` raises `ValueError: RRULE UNTIL values must be
specified in UTC when DTSTART is timezone-aware`, while the port round trips
cleanly. All twelve other `roundtrip-*` cases agree.

## What matched

Worth stating, since the mismatch list is long. All 26 `combinations` cases
match, including every worked example from RFC 5545 §3.8.5.3 that this harness
carries: Friday the 13th, the Saturday following the first Sunday, US election
day, the WKST-sensitive every-other-week rule in all seven week starts, both
`BYSETPOS` examples, "every 20 minutes from 9:00 to 16:40" in both its `DAILY`
and `MINUTELY` spellings, and the "February 30 is ignored" example. All seven
frequencies, all ten `INTERVAL` cases, all 19 `BYDAY` cases (including
`+1MO`, `-1FR`, `-2MO`, `5SU`, `20MO`, `53MO` and ordinal-ignored-below-MONTHLY),
all 11 `BYSETPOS` cases, all 14 window-API cases and 23 of 24 `rruleset` cases
match exactly.
