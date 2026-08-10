# moment — upstream API inventory vs the Go port

| | |
| --- | --- |
| upstream (oracle) | `moment@2.30.1` — pinned in `node/package.json` |
| Go module under test | `github.com/malcolmston/moment` `v0.0.0-20260810111552-e5d96a99ef23` (published module, no `replace`) |
| cases | 1731 across `cases/*.json` |
| case-level parity | **93.92%** (1621 match / 1726 compared, 5 declared deviations) |
| symbol-level parity | **78.13%** (75 match / 96 compared symbols) |
| regenerate | `GOWORK=off go test ./parity/moment/` (rewrites `parity.json`) |

Every runner in this directory is deterministic: `TZ=UTC` is forced for both
processes, no operation calls "now", and every relative-time case carries an
explicit reference instant *and* an explicit target.

## How the upstream list was derived

The symbol lists below are enumerated by reflection over the installed package,
not from the README or from memory:

```sh
cd parity/moment/node
TZ=UTC node -e '
  const m = require("moment");
  console.log(Object.keys(m).sort().join("\n"));                       # 40 statics
  console.log(Object.keys(m.fn).sort().join("\n"));                    # 89 moment.prototype methods
  console.log(Object.keys(m.duration(1).constructor.prototype).sort().join("\n"));  # 34 Duration methods
'
```

The Go side was enumerated with:

```sh
GOWORK=off go doc -all github.com/malcolmston/moment
GOWORK=off go run <<< 'moment.SupportedTokens()'   # the format-token table
```

Statuses are one of `match`, `differs`, `missing`, `extra`, `untested`.
A symbol with no case is `untested`, never `match`.

## `moment.*` — static exports (40)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `moment.HTML5_FMT` | — | missing | — | no exported table of HTML5 input format strings |
| `moment.ISO_8601` | `moment.ParseISO` | differs | `parse-iso-full-z`, `parse-iso-no-ms`, `parse-iso-offset` (+30) | exposed as a parsing function, not a reusable format sentinel; 7/33 cases mismatch |
| `moment.RFC_2822` | `moment.ParseRFC2822` | differs | `parse-rfc-basic`, `parse-rfc-no-day`, `parse-rfc-gmt` (+2) | function rather than sentinel; two-digit-year RFC dates rejected (`parse-rfc-two-digit-year`); 1/5 cases mismatch |
| `moment.calendarFormat` | — | missing | — | bucket-selection hook not exposed; CalendarWith takes templates instead |
| `moment.createFromInputFallback` | — | missing | — | no Date-constructor fallback hook |
| `moment.defaultFormat` | — | missing | — |  |
| `moment.defaultFormatUtc` | — | missing | — |  |
| `moment.defineLocale` | `moment.RegisterLocale` | untested | — | registry mutation is global; excluded to keep the run deterministic |
| `moment.deprecationHandler` | — | missing | — | the port has no deprecation machinery |
| `moment.duration` | `moment.NewDuration / NewDurationFromObject / ParseDuration` | differs | `parseDuration-0`, `parseDuration-1`, `parseDuration-2` (+16) | object and unit forms match; the string form rejects what moment accepts (`parseDuration-13`…`-18`); 7/19 cases mismatch |
| `moment.fn` | — | missing | — | no prototype handle (Moment is a struct) |
| `moment.invalid` | `moment.Invalid` | untested | — | reachable, but every invalid value in the suite comes from a failed parse |
| `moment.isDate` | `moment.IsDate` | match | `isDate-moment`, `isDate-duration`, `isDate-date` |  |
| `moment.isDuration` | `moment.IsDuration` | match | `isDuration-moment`, `isDuration-duration`, `isDuration-date` |  |
| `moment.isMoment` | `moment.IsMoment` | match | `isMoment-moment`, `isMoment-duration`, `isMoment-date` |  |
| `moment.lang` | — | missing | — | deprecated alias of locale |
| `moment.langData` | — | missing | — | deprecated alias of localeData |
| `moment.locale` | `moment.SetGlobalLocale / moment.GlobalLocale` | untested | — | global mutation; per-value Moment.Locale is tested instead |
| `moment.localeData` | `moment.LookupLocale + the moment.Months/Weekdays/Ordinal/LongDateFormat/Meridiem queries` | differs | `months-en`, `monthsShort-en`, `weekdays-en` (+321) | shape differs (free functions, not an object); ru/hi month lists and ru/tr/fr ordinals disagree; 26/324 cases mismatch |
| `moment.locales` | `moment.AvailableLocales` | differs | — | the port bundles ~21 locales against moment's ~140 |
| `moment.max` | `moment.Max` | match | `max-three` |  |
| `moment.min` | `moment.Min` | match | `min-three` |  |
| `moment.momentProperties` | — | missing | — |  |
| `moment.months` | `moment.Months` | differs | `months-en`, `monthsShort-en`, `months-en-gb` (+33) | ru returns nominative forms where moment returns genitive (`months-ru`); hi spellings differ (`months-hi`); 3/36 cases mismatch |
| `moment.monthsShort` | `moment.MonthsShort` | differs | `monthsShort-en`, `monthsShort-en-gb`, `monthsShort-fr` (+15) | ru abbreviations differ (`monthsShort-ru`); 1/18 cases mismatch |
| `moment.normalizeUnits` | `moment.NormalizeUnit` | differs | `normalizeUnit-days`, `normalizeUnit-d`, `normalizeUnit-D` (+10) | "e" is not recognised (moment maps it to "weekday"); 1/13 cases mismatch |
| `moment.now` | — | missing | — | replaced by the Clock interface / NowWith; deliberately untestable here |
| `moment.parseTwoDigitYear` | — | missing | — | the pivot is not overridable, though YY parsing itself matches |
| `moment.parseZone` | `moment.ParseZone` | match | `parseZone-0`, `parseZone-1`, `parseZone-2` (+2) |  |
| `moment.relativeTimeRounding` | — | missing | — |  |
| `moment.relativeTimeThreshold` | `moment.RelativeTimeThreshold / SetRelativeTimeThreshold` | untested | — | global mutation; default thresholds are exercised through `from-*` |
| `moment.suppressDeprecationWarnings` | — | missing | — |  |
| `moment.unix` | `moment.Unix` | untested | — | the static seconds constructor; Moment.Unix (the getter) is tested |
| `moment.updateLocale` | — | missing | — | RegisterLocale replaces wholesale rather than patching |
| `moment.updateOffset` | — | missing | — |  |
| `moment.utc` | `moment.Parse / Moment.UTC` | differs | `parse-iso-full-z`, `parse-iso-no-ms`, `parse-iso-offset` (+30) | see the ISO divergences below; 7/33 cases mismatch |
| `moment.version` | — | missing | — | no exported version constant (a VERSION file exists) |
| `moment.weekdays` | `moment.Weekdays` | match | `weekdays-en`, `weekdaysShort-en`, `weekdaysMin-en` (+51) |  |
| `moment.weekdaysMin` | `moment.WeekdaysMin` | match | `weekdaysMin-en`, `weekdaysMin-en-gb`, `weekdaysMin-fr` (+15) |  |
| `moment.weekdaysShort` | `moment.WeekdaysShort` | match | `weekdaysShort-en`, `weekdaysShort-en-gb`, `weekdaysShort-fr` (+15) |  |

## `moment.prototype.*` (89)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `moment.prototype.add` | `moment.Moment.Add` | match | `add-year-1`, `add-year-3`, `add-year-0` (+45) | includes month/year day clamping (`clamp-*`) |
| `moment.prototype.calendar` | `moment.Moment.Calendar / CalendarWith / CalendarNow` | differs | `calendar--8d`, `calendar--7d`, `calendar--2d` (+7) | en matches; ar/hi differ because the port does not localise digits (`calendarLocale-ar`, `calendarLocale-hi`) |
| `moment.prototype.clone` | `moment.Moment.Clone` | match | `serialize-clone` |  |
| `moment.prototype.creationData` | `moment.Moment.CreationData` | untested | — | field set differs (no parsing flags), so no comparable case |
| `moment.prototype.date` | `moment.Moment.Date / SetDate` | match | `setDate`, `setDate-clamped` |  |
| `moment.prototype.dates` | — | missing | — | deprecated alias of date |
| `moment.prototype.day` | `moment.Moment.Weekday / SetWeekday` | match | `setDayOfYear`, `setDayOfYear-366`, `setWeekday` (+1) |  |
| `moment.prototype.dayOfYear` | `moment.Moment.DayOfYear / SetDayOfYear` | match | `setDayOfYear`, `setDayOfYear-366` |  |
| `moment.prototype.days` | — | missing | — | deprecated alias of day |
| `moment.prototype.daysInMonth` | `moment.Moment.DaysInMonth` | match | `components-a`, `components-b`, `components-c` (+3) | 3/6 cases mismatch |
| `moment.prototype.diff` | `moment.Moment.Diff / DiffInt / DiffDuration` | match | `diff-year-same`, `diffFloat-year-same`, `diff-year-ms` (+195) | all 9 units, integer and float, in both directions |
| `moment.prototype.endOf` | `moment.Moment.EndOf` | match | `endOf-year-a`, `endOf-year-b`, `endOf-year-c` (+41) | all 11 units |
| `moment.prototype.eraAbbr` | — | missing | — | no era support |
| `moment.prototype.eraName` | — | missing | — | no era support |
| `moment.prototype.eraNarrow` | — | missing | — | no era support |
| `moment.prototype.eraYear` | — | missing | — | no era support |
| `moment.prototype.format` | `moment.Moment.Format` | differs | `token-YY-a`, `token-YY-b`, `token-YY-c` (+273) | `zz`, `l`, `ll`, `lll`, `llll` and the empty format string diverge; the other 62 tokens match; 21/276 cases mismatch |
| `moment.prototype.from` | `moment.Moment.From` | differs | `from-future-0s`, `from-past-0s`, `from-future-1s` (+65) | zero-length and the 46-day / 548-day buckets disagree; 5/68 cases mismatch |
| `moment.prototype.fromNow` | `moment.Moment.FromNow` | untested | — | depends on "now"; the port can inject a clock but moment cannot, so From carries the coverage |
| `moment.prototype.get` | `moment.Moment.Get` | differs | `get-year`, `get-month`, `get-date` (+8) | declared deviation: `get("month")` is 0-based upstream, 1-based in the port (`get-month`) |
| `moment.prototype.hasAlignedHourOffset` | — | missing | — |  |
| `moment.prototype.hour` | `moment.Moment.Hour / SetHour` | match | — |  |
| `moment.prototype.hours` | — | missing | — | deprecated alias of hour |
| `moment.prototype.inspect` | — | missing | — | no debug-representation method |
| `moment.prototype.invalidAt` | — | missing | — | the port reports no per-component failure index |
| `moment.prototype.isAfter` | `moment.Moment.IsAfter` | match | `isAfter-same`, `isAfter-ms`, `isAfter-minute` (+5) |  |
| `moment.prototype.isBefore` | `moment.Moment.IsBefore` | match | `isBefore-same`, `isBefore-ms`, `isBefore-minute` (+5) |  |
| `moment.prototype.isBetween` | `moment.Moment.IsBetween` | match | `isBetween-inside`, `isBetween-outside`, `isBetween-on-lower-exclusive` (+4) | declared deviation for reversed bounds (`isBetween-reversed-bounds`); the port has no granularity argument |
| `moment.prototype.isDST` | `moment.Moment.IsDST` | untested | — | the suite pins TZ=UTC, so no DST case is comparable without a tz database on the node side |
| `moment.prototype.isDSTShifted` | — | missing | — | deprecated upstream |
| `moment.prototype.isLeapYear` | `moment.Moment.IsLeapYear` | match | `components-a`, `components-b`, `components-c` (+3) | 3/6 cases mismatch |
| `moment.prototype.isLocal` | `moment.Moment.IsLocal` | untested | — | host-zone dependent |
| `moment.prototype.isSame` | `moment.Moment.IsSame / IsSameUnit` | match | `isSame-same`, `isSame-ms`, `isSame-minute` (+54) | instant form and the unit-granularity form, all 11 units |
| `moment.prototype.isSameOrAfter` | `moment.Moment.IsSameOrAfter` | match | `isSameOrAfter-same`, `isSameOrAfter-ms`, `isSameOrAfter-minute` (+5) |  |
| `moment.prototype.isSameOrBefore` | `moment.Moment.IsSameOrBefore` | match | `isSameOrBefore-same`, `isSameOrBefore-ms`, `isSameOrBefore-minute` (+5) |  |
| `moment.prototype.isUTC` | `moment.Moment.IsUTC` | match | `components-a`, `components-b`, `components-c` (+3) | compared inside the `components-*` cases; 3/6 cases mismatch |
| `moment.prototype.isUtc` | `moment.Moment.IsUTC` | match | `components-a`, `components-b`, `components-c` (+3) | alias of isUTC; 3/6 cases mismatch |
| `moment.prototype.isUtcOffset` | — | missing | — |  |
| `moment.prototype.isValid` | `moment.Moment.IsValid` | differs | `validity-oor-feb-30`, `validity-oor-feb-29-nonleap`, `validity-oor-feb-29-leap` (+24) | the headline divergence: out-of-range token input is normalised instead of invalidated; 8/27 cases mismatch |
| `moment.prototype.isoWeek` | `moment.Moment.ISOWeekNumber / ISOWeek` | match | `components-a`, `components-b`, `components-c` (+5) | 3/8 cases mismatch |
| `moment.prototype.isoWeekYear` | `moment.Moment.ISOWeekYear` | match | `components-a`, `components-b`, `components-c` (+3) | 3/6 cases mismatch |
| `moment.prototype.isoWeekday` | `moment.Moment.ISOWeekday / SetISOWeekday` | match | `setISOWeekday`, `setISOWeekday-7` |  |
| `moment.prototype.isoWeeks` | — | missing | — | alias of isoWeek |
| `moment.prototype.isoWeeksInISOWeekYear` | — | missing | — |  |
| `moment.prototype.isoWeeksInYear` | `moment.Moment.ISOWeeksInYear` | differs | `components-a`, `components-b`, `components-c` (+3) | returns 53 where moment returns 52 for ISO-week-year 2020 and 2015 (`components-d`, `-e`, `-f`); 3/6 cases mismatch |
| `moment.prototype.lang` | — | missing | — | deprecated alias of locale |
| `moment.prototype.local` | `moment.Moment.Local` | untested | — | host-zone dependent |
| `moment.prototype.locale` | `moment.Moment.Locale / LocaleName` | differs | `formatLocale-en-LLLL`, `calendarLocale-en`, `formatLocale-en-gb-LLLL` (+33) | works, but zh-cn/pl/cs/ar/hi long-date output disagrees (`formatLocale-*`); 7/36 cases mismatch |
| `moment.prototype.localeData` | `moment.Moment.LocaleData` | untested | — | returns a *Locale struct with no comparable JSON shape |
| `moment.prototype.max` | — | missing | — | deprecated prototype form (moment.Max exists) |
| `moment.prototype.millisecond` | `moment.Moment.Millisecond / SetMillisecond` | match | `set-year-2000`, `set-year-2020`, `set-month-3` (+12) |  |
| `moment.prototype.milliseconds` | — | missing | — | deprecated alias |
| `moment.prototype.min` | — | missing | — | deprecated prototype form (moment.Min exists) |
| `moment.prototype.minute` | `moment.Moment.Minute / SetMinute` | match | `set-year-2000`, `set-year-2020`, `set-month-3` (+12) |  |
| `moment.prototype.minutes` | — | missing | — | deprecated alias |
| `moment.prototype.month` | `moment.Moment.Month / SetMonth` | match | `setMonth1-apr-from-may31`, `setMonth1-feb-from-jan31` | clamping on assignment matches (`setMonth1-*`) |
| `moment.prototype.months` | — | missing | — | deprecated alias of month |
| `moment.prototype.parseZone` | `moment.ParseZone` | match | `parseZone-0`, `parseZone-1`, `parseZone-2` (+2) | package-level in the port, not a method |
| `moment.prototype.parsingFlags` | — | missing | — | no parsing-flag introspection |
| `moment.prototype.quarter` | `moment.Moment.Quarter / SetQuarter` | match | `setQuarter`, `setQuarter-4` |  |
| `moment.prototype.quarters` | — | missing | — | deprecated alias |
| `moment.prototype.second` | `moment.Moment.Second / SetSecond` | match | `set-year-2000`, `set-year-2020`, `set-month-3` (+12) |  |
| `moment.prototype.seconds` | — | missing | — | deprecated alias |
| `moment.prototype.set` | `moment.Moment.Set / SetAll` | match | `set-year-2000`, `set-year-2020`, `set-month-3` (+12) | declared deviation for the month index (`set-month-*`) |
| `moment.prototype.startOf` | `moment.Moment.StartOf` | match | `startOf-year-a`, `startOf-year-b`, `startOf-year-c` (+41) | all 11 units |
| `moment.prototype.subtract` | `moment.Moment.Subtract` | match | `subtract-year-1`, `subtract-year-3`, `subtract-year-0` (+33) |  |
| `moment.prototype.to` | `moment.Moment.To` | differs | `toISOStringZone-utc`, `toISOStringZone-ist`, `toISOStringZone-est` (+19) | zero-length difference is rendered as future rather than past (`to-0s`); 2/22 cases mismatch |
| `moment.prototype.toArray` | `moment.Moment.ToArray` | match | `serialize-toArray-0`, `serialize-toArray-1`, `serialize-toArray-2` |  |
| `moment.prototype.toDate` | `moment.Moment.ToDate / Time` | untested | — | returns a time.Time; no JSON-comparable form |
| `moment.prototype.toISOString` | `moment.Moment.ToISOString / ToISOStringZone` | differs | `toISOStringZone-utc`, `toISOStringZone-ist`, `toISOStringZone-est` (+5) | UTC form matches; the keep-offset form prints `Z` where moment prints `+00:00` (`toISOStringZone-utc`); 1/8 cases mismatch |
| `moment.prototype.toJSON` | `moment.Moment.ToJSON` | match | `serialize-toJSON-0`, `serialize-toJSON-1`, `serialize-toJSON-2` |  |
| `moment.prototype.toNow` | `moment.Moment.ToNow` | untested | — | depends on "now"; To carries the coverage |
| `moment.prototype.toObject` | `moment.Moment.ToObject` | match | `serialize-toObject-0`, `serialize-toObject-1`, `serialize-toObject-2` |  |
| `moment.prototype.toString` | `moment.Moment.String` | untested | — | the port returns ISO-8601 where moment returns an RFC-ish date string; no case asserts it |
| `moment.prototype.unix` | `moment.Moment.Unix` | match | `serialize-unix-0`, `serialize-unix-1`, `serialize-unix-2` |  |
| `moment.prototype.utc` | `moment.Moment.UTC` | match | `parse-iso-full-z`, `parse-iso-no-ms`, `parse-iso-offset` (+30) | 7/33 cases mismatch |
| `moment.prototype.utcOffset` | `moment.Moment.UTCOffset / SetUTCOffset` | match | `components-a`, `components-b`, `components-c` (+8) | 3/11 cases mismatch |
| `moment.prototype.valueOf` | `moment.Moment.ValueOf` | match | `serialize-valueOf-0`, `serialize-valueOf-1`, `serialize-valueOf-2` |  |
| `moment.prototype.week` | `moment.Moment.Week` | match | `components-a`, `components-b`, `components-c` (+3) | compared inside `components-*`; 3/6 cases mismatch |
| `moment.prototype.weekYear` | `moment.Moment.WeekYear` | match | `components-a`, `components-b`, `components-c` (+3) | compared inside `components-*`; 3/6 cases mismatch |
| `moment.prototype.weekday` | `moment.Moment.LocaleWeekday` | match | `components-a`, `components-b`, `components-c` (+3) | compared inside `components-*`; 3/6 cases mismatch |
| `moment.prototype.weeks` | — | missing | — | alias of week |
| `moment.prototype.weeksInWeekYear` | — | missing | — |  |
| `moment.prototype.weeksInYear` | `moment.Moment.WeeksInYear` | match | `components-a`, `components-b`, `components-c` (+3) | compared inside `components-*`; 3/6 cases mismatch |
| `moment.prototype.year` | `moment.Moment.Year / SetYear` | match | `setYear` |  |
| `moment.prototype.years` | — | missing | — | deprecated alias |
| `moment.prototype.zone` | — | missing | — | deprecated upstream (SetUTCOffset covers the intent) |
| `moment.prototype.zoneAbbr` | `moment.Moment.ZoneAbbr` | match | `components-a`, `components-b`, `components-c` (+3) | compared inside `components-*`; 3/6 cases mismatch |
| `moment.prototype.zoneName` | `moment.Moment.ZoneName` | untested | — | the port returns the IANA location name, moment returns the abbreviation |

## `Duration.prototype.*` (34)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `moment.duration()._bubble` | — | missing | — | internal |
| `moment.duration().abs` | `moment.Duration.Abs` | match | `durationAbs-negative` |  |
| `moment.duration().add` | `moment.Duration.Add` | match | `durationAdd` |  |
| `moment.duration().as` | `moment.Duration.As` | match | `durationAs-zero-year`, `durationAs-zero-month`, `durationAs-zero-week` (+125) | 8 units × 16 durations |
| `moment.duration().asDays` | `moment.Duration.AsDays` | match | `durationAs-zero-year`, `durationAs-zero-month`, `durationAs-zero-week` (+125) | covered through As("day") |
| `moment.duration().asHours` | `moment.Duration.AsHours` | match | `durationAs-zero-year`, `durationAs-zero-month`, `durationAs-zero-week` (+125) | covered through As("hour") |
| `moment.duration().asMilliseconds` | `moment.Duration.AsMilliseconds` | match | `durationAs-zero-year`, `durationAs-zero-month`, `durationAs-zero-week` (+125) | covered through As("millisecond") |
| `moment.duration().asMinutes` | `moment.Duration.AsMinutes` | match | `durationAs-zero-year`, `durationAs-zero-month`, `durationAs-zero-week` (+125) | covered through As("minute") |
| `moment.duration().asMonths` | `moment.Duration.AsMonths` | match | `durationAs-zero-year`, `durationAs-zero-month`, `durationAs-zero-week` (+125) | covered through As("month") |
| `moment.duration().asQuarters` | — | missing | — | no quarter conversion on Duration |
| `moment.duration().asSeconds` | `moment.Duration.AsSeconds` | match | `durationAs-zero-year`, `durationAs-zero-month`, `durationAs-zero-week` (+125) | covered through As("second") |
| `moment.duration().asWeeks` | `moment.Duration.AsWeeks` | match | `durationAs-zero-year`, `durationAs-zero-month`, `durationAs-zero-week` (+125) | covered through As("week") |
| `moment.duration().asYears` | `moment.Duration.AsYears` | match | `durationAs-zero-year`, `durationAs-zero-month`, `durationAs-zero-week` (+125) | covered through As("year") |
| `moment.duration().clone` | `moment.Duration.Clone` | untested | — |  |
| `moment.duration().days` | `moment.Duration.Days` | match | `durationGet-zero-year`, `durationGet-zero-month`, `durationGet-zero-week` (+125) | covered through Get("day") |
| `moment.duration().get` | `moment.Duration.Get` | match | `durationGet-zero-year`, `durationGet-zero-month`, `durationGet-zero-week` (+125) |  |
| `moment.duration().hours` | `moment.Duration.Hours` | match | `durationGet-zero-year`, `durationGet-zero-month`, `durationGet-zero-week` (+125) | covered through Get("hour") |
| `moment.duration().humanize` | `moment.Duration.Humanize` | differs | `humanize-zero`, `humanize-suffix-zero`, `humanize-ms` (+29) | a zero-length duration is suffixed as future, upstream as past (`humanize-suffix-zero`); 1/32 cases mismatch |
| `moment.duration().isValid` | `moment.ParseDuration error / Duration validity` | differs | `parseDuration-0`, `parseDuration-1`, `parseDuration-2` (+16) | moment treats unparseable duration strings as a valid zero duration; the port returns an error; 7/19 cases mismatch |
| `moment.duration().lang` | — | missing | — | deprecated alias |
| `moment.duration().locale` | `moment.Duration.Locale` | differs | `humanizeLocale-en`, `humanizeLocale-en-gb`, `humanizeLocale-fr` (+15) | ar/hi humanisation is not digit-localised; 2/18 cases mismatch |
| `moment.duration().localeData` | — | missing | — |  |
| `moment.duration().milliseconds` | `moment.Duration.Milliseconds` | match | `durationGet-zero-year`, `durationGet-zero-month`, `durationGet-zero-week` (+125) | covered through Get("millisecond") |
| `moment.duration().minutes` | `moment.Duration.Minutes` | match | `durationGet-zero-year`, `durationGet-zero-month`, `durationGet-zero-week` (+125) | covered through Get("minute") |
| `moment.duration().months` | `moment.Duration.Months` | match | `durationGet-zero-year`, `durationGet-zero-month`, `durationGet-zero-week` (+125) | covered through Get("month") |
| `moment.duration().seconds` | `moment.Duration.Seconds` | match | `durationGet-zero-year`, `durationGet-zero-month`, `durationGet-zero-week` (+125) | covered through Get("second") |
| `moment.duration().subtract` | `moment.Duration.Subtract` | match | `durationSubtract` |  |
| `moment.duration().toISOString` | `moment.Duration.ISOString` | match | `durationISO-zero`, `durationISO-ms`, `durationISO-seconds` (+13) | 16 durations including mixed signs |
| `moment.duration().toIsoString` | — | missing | — | deprecated casing alias |
| `moment.duration().toJSON` | `moment.Duration.ToJSON` | untested | — | aliases ISOString, which is tested |
| `moment.duration().toString` | `moment.Duration.String` | untested | — | aliases ISOString, which is tested |
| `moment.duration().valueOf` | `moment.Duration.ValueOf` | match | `durationValueOf-zero`, `durationValueOf-ms`, `durationValueOf-seconds` (+13) |  |
| `moment.duration().weeks` | `moment.Duration.Weeks` | match | `durationGet-zero-year`, `durationGet-zero-month`, `durationGet-zero-week` (+125) | covered through Get("week") |
| `moment.duration().years` | `moment.Duration.Years` | match | `durationGet-zero-year`, `durationGet-zero-month`, `durationGet-zero-week` (+125) | covered through Get("year") |

## Format-token table (67 tokens, 4 instants each)

Each token is rendered against four fixed instants: `2017-07-14T02:40:00.000Z`,
`2016-01-31T23:05:09.123Z`, `2020-12-31T13:00:00.500Z` and
`2021-01-03T00:00:00.000Z`.

| token | Go | status | cases | note |
| --- | --- | --- | --- | --- |
| `YY` | `Format("YY")` | match | `token-YY-a`, `token-YY-b`, `token-YY-c` (+1) |  |
| `YYYY` | `Format("YYYY")` | match | `token-YYYY-a`, `token-YYYY-b`, `token-YYYY-c` (+1) |  |
| `Y` | `Format("Y")` | match | `token-Y-a`, `token-Y-b`, `token-Y-c` (+1) |  |
| `Q` | `Format("Q")` | match | `token-Q-a`, `token-Q-b`, `token-Q-c` (+1) |  |
| `Qo` | `Format("Qo")` | match | `token-Qo-a`, `token-Qo-b`, `token-Qo-c` (+1) |  |
| `M` | `Format("M")` | match | `token-M-a`, `token-M-b`, `token-M-c` (+1) |  |
| `Mo` | `Format("Mo")` | match | `token-Mo-a`, `token-Mo-b`, `token-Mo-c` (+1) |  |
| `MM` | `Format("MM")` | match | `token-MM-a`, `token-MM-b`, `token-MM-c` (+1) |  |
| `MMM` | `Format("MMM")` | match | `token-MMM-a`, `token-MMM-b`, `token-MMM-c` (+1) |  |
| `MMMM` | `Format("MMMM")` | match | `token-MMMM-a`, `token-MMMM-b`, `token-MMMM-c` (+1) |  |
| `D` | `Format("D")` | match | `token-D-a`, `token-D-b`, `token-D-c` (+1) |  |
| `Do` | `Format("Do")` | match | `token-Do-a`, `token-Do-b`, `token-Do-c` (+1) |  |
| `DD` | `Format("DD")` | match | `token-DD-a`, `token-DD-b`, `token-DD-c` (+1) |  |
| `DDD` | `Format("DDD")` | match | `token-DDD-a`, `token-DDD-b`, `token-DDD-c` (+1) |  |
| `DDDo` | `Format("DDDo")` | match | `token-DDDo-a`, `token-DDDo-b`, `token-DDDo-c` (+1) |  |
| `DDDD` | `Format("DDDD")` | match | `token-DDDD-a`, `token-DDDD-b`, `token-DDDD-c` (+1) |  |
| `d` | `Format("d")` | match | `token-d-a`, `token-d-b`, `token-d-c` (+1) |  |
| `do` | `Format("do")` | match | `token-do-a`, `token-do-b`, `token-do-c` (+1) |  |
| `dd` | `Format("dd")` | match | `token-dd-a`, `token-dd-b`, `token-dd-c` (+1) |  |
| `ddd` | `Format("ddd")` | match | `token-ddd-a`, `token-ddd-b`, `token-ddd-c` (+1) |  |
| `dddd` | `Format("dddd")` | match | `token-dddd-a`, `token-dddd-b`, `token-dddd-c` (+1) |  |
| `e` | `Format("e")` | match | `token-e-a`, `token-e-b`, `token-e-c` (+1) |  |
| `E` | `Format("E")` | match | `token-E-a`, `token-E-b`, `token-E-c` (+1) |  |
| `w` | `Format("w")` | match | `token-w-a`, `token-w-b`, `token-w-c` (+1) |  |
| `wo` | `Format("wo")` | match | `token-wo-a`, `token-wo-b`, `token-wo-c` (+1) |  |
| `ww` | `Format("ww")` | match | `token-ww-a`, `token-ww-b`, `token-ww-c` (+1) |  |
| `W` | `Format("W")` | match | `token-W-a`, `token-W-b`, `token-W-c` (+1) |  |
| `Wo` | `Format("Wo")` | match | `token-Wo-a`, `token-Wo-b`, `token-Wo-c` (+1) |  |
| `WW` | `Format("WW")` | match | `token-WW-a`, `token-WW-b`, `token-WW-c` (+1) |  |
| `gg` | `Format("gg")` | match | `token-gg-a`, `token-gg-b`, `token-gg-c` (+1) |  |
| `gggg` | `Format("gggg")` | match | `token-gggg-a`, `token-gggg-b`, `token-gggg-c` (+1) |  |
| `GG` | `Format("GG")` | match | `token-GG-a`, `token-GG-b`, `token-GG-c` (+1) |  |
| `GGGG` | `Format("GGGG")` | match | `token-GGGG-a`, `token-GGGG-b`, `token-GGGG-c` (+1) |  |
| `A` | `Format("A")` | match | `token-A-a`, `token-A-b`, `token-A-c` (+1) |  |
| `a` | `Format("a")` | match | `token-a-a`, `token-a-b`, `token-a-c` (+1) |  |
| `H` | `Format("H")` | match | `token-H-a`, `token-H-b`, `token-H-c` (+1) |  |
| `HH` | `Format("HH")` | match | `token-HH-a`, `token-HH-b`, `token-HH-c` (+1) |  |
| `h` | `Format("h")` | match | `token-h-a`, `token-h-b`, `token-h-c` (+1) |  |
| `hh` | `Format("hh")` | match | `token-hh-a`, `token-hh-b`, `token-hh-c` (+1) |  |
| `k` | `Format("k")` | match | `token-k-a`, `token-k-b`, `token-k-c` (+1) |  |
| `kk` | `Format("kk")` | match | `token-kk-a`, `token-kk-b`, `token-kk-c` (+1) |  |
| `m` | `Format("m")` | match | `token-m-a`, `token-m-b`, `token-m-c` (+1) |  |
| `mm` | `Format("mm")` | match | `token-mm-a`, `token-mm-b`, `token-mm-c` (+1) |  |
| `s` | `Format("s")` | match | `token-s-a`, `token-s-b`, `token-s-c` (+1) |  |
| `ss` | `Format("ss")` | match | `token-ss-a`, `token-ss-b`, `token-ss-c` (+1) |  |
| `S` | `Format("S")` | match | `token-S-a`, `token-S-b`, `token-S-c` (+1) |  |
| `SS` | `Format("SS")` | match | `token-SS-a`, `token-SS-b`, `token-SS-c` (+1) |  |
| `SSS` | `Format("SSS")` | match | `token-SSS-a`, `token-SSS-b`, `token-SSS-c` (+1) |  |
| `SSSS` | `Format("SSSS")` | match | `token-SSSS-a`, `token-SSSS-b`, `token-SSSS-c` (+1) |  |
| `SSSSSS` | `Format("SSSSSS")` | match | `token-SSSSSS-a`, `token-SSSSSS-b`, `token-SSSSSS-c` (+1) |  |
| `SSSSSSSSS` | `Format("SSSSSSSSS")` | match | `token-SSSSSSSSS-a`, `token-SSSSSSSSS-b`, `token-SSSSSSSSS-c` (+1) |  |
| `z` | `Format("z")` | match | `token-z-a`, `token-z-b`, `token-z-c` (+1) |  |
| `zz` | `Format("zz")` | differs | `token-zz-a`, `token-zz-b`, `token-zz-c` (+1) | the port emits the zone abbreviation, moment the long zone name |
| `Z` | `Format("Z")` | match | `token-Z-a`, `token-Z-b`, `token-Z-c` (+1) |  |
| `ZZ` | `Format("ZZ")` | match | `token-ZZ-a`, `token-ZZ-b`, `token-ZZ-c` (+1) |  |
| `X` | `Format("X")` | match | `token-X-a`, `token-X-b`, `token-X-c` (+1) |  |
| `x` | `Format("x")` | match | `token-x-a`, `token-x-b`, `token-x-c` (+1) |  |
| `LT` | `Format("LT")` | match | `token-LT-a`, `token-LT-b`, `token-LT-c` (+1) |  |
| `LTS` | `Format("LTS")` | match | `token-LTS-a`, `token-LTS-b`, `token-LTS-c` (+1) |  |
| `L` | `Format("L")` | match | `token-L-a`, `token-L-b`, `token-L-c` (+1) |  |
| `LL` | `Format("LL")` | match | `token-LL-a`, `token-LL-b`, `token-LL-c` (+1) |  |
| `LLL` | `Format("LLL")` | match | `token-LLL-a`, `token-LLL-b`, `token-LLL-c` (+1) |  |
| `LLLL` | `Format("LLLL")` | match | `token-LLLL-a`, `token-LLLL-b`, `token-LLLL-c` (+1) |  |
| `l` | — | missing | `token-l-a`, `token-l-b`, `token-l-c` (+1) | the port emits the token text verbatim |
| `ll` | — | missing | `token-ll-a`, `token-ll-b`, `token-ll-c` (+1) | the port emits the token text verbatim |
| `lll` | — | missing | `token-lll-a`, `token-lll-b`, `token-lll-c` (+1) | the port emits the token text verbatim |
| `llll` | — | missing | `token-llll-a`, `token-llll-b`, `token-llll-c` (+1) | the port emits the token text verbatim |

Token totals: 62 match, 1 differs, 4 missing.

## Go-only surface (`extra`)

These have no upstream counterpart and are exercised only indirectly.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| — | `moment.Clock`, `moment.FixedClock`, `moment.ClockFunc`, `Moment.WithClock`, `moment.NowWith` | extra | — | injectable clock, the port's answer to `moment.now` |
| — | `moment.Moment.FormatLayout`, `moment.ParseLayout` | extra | — | raw Go reference layouts |
| — | `moment.Moment.In`, `moment.Moment.Location`, `moment.DateTime`, `moment.FromTime`, `moment.New`, `moment.UnixMilli` | extra | — | `time.Time`/`time.Location` interop |
| — | `moment.Moment.DiffDuration`, `moment.Moment.AddDuration`, `moment.DurationFromTime`, `moment.Duration.ToDuration` | extra | — | `time.Duration` interop |
| — | `moment.Moment.IsZero`, `moment.Moment.Nanosecond`, `moment.Moment.DaysInYear`, `moment.Moment.SetAll` | extra | — | |
| — | `moment.SupportedTokens`, `moment.FirstDayOfWeek`, `moment.FirstWeekContainsDate`, `moment.Meridiem`, `moment.MonthName`, `moment.MonthShortName`, `moment.WeekdayName`, `moment.WeekdayShortName`, `moment.WeekdayMinName`, `moment.Humanize`, `moment.DurationBetween` | extra | — | introspection and convenience helpers |
| — | `moment.Moment.ToISOStringZone` | extra (differs) | `toISOStringZone-*` | the named form of `toISOString(true)` |

## Counts

| status | symbols |
| --- | --- |
| match | 75 |
| differs | 21 |
| missing | 49 |
| untested | 18 |
| extra (Go-only) | 33 |
| **total upstream symbols enumerated** | **163** |

Symbol-level parity, over the 96 symbols actually compared (match +
differs; `missing`, `extra` and `untested` are excluded because no
comparison happened):
**78.13%**.

Case-level parity, over the 1726 cases actually
compared: **93.92%** (1621 match, 105
mismatch, 5 declared deviations excluded).

## The divergences, in full

### Validity and out-of-range input — the headline finding

moment invalidates an out-of-range calendar component; the port silently
normalises it, so `isValid()` disagrees and an instant appears where upstream
has none.

| input | upstream | Go port | cases |
| --- | --- | --- | --- |
| `2017-02-30` via `YYYY-MM-DD` | invalid | `2017-03-02T00:00:00Z` | `validity-oor-fmt-feb-30`, `parsed-oor-fmt-feb-30` |
| `2019-02-29` via `YYYY-MM-DD` | invalid | `2019-03-01T00:00:00Z` | `validity-oor-fmt-feb-29-nonleap`, `parsed-…` |
| `2017-13-01` via `YYYY-MM-DD` | invalid | `2018-01-01T00:00:00Z` | `validity-oor-fmt-month-13`, `parsed-…` |
| `2017-01-32` via `YYYY-MM-DD` | invalid | `2017-02-01T00:00:00Z` | `validity-oor-fmt-day-32`, `parsed-…` |
| `25:00` via `HH:mm` | invalid | rolls to the next day 01:00 | `validity-oor-fmt-hour-25`, `parsed-…` |
| `23:60` via `HH:mm` | invalid | rolls to the next day 00:00 | `validity-oor-fmt-minute-60`, `parsed-…` |
| day-of-year `367` | invalid | `2018-01-02` | `validity-oor-fmt-doy-367`, `parsed-…` |
| quarter `5` | invalid | `2018-01-01` | `validity-oor-fmt-quarter-5`, `parsed-…` |
| `[2017, 1, 30]` (array form) | invalid | `2017-03-02` | `construct-array-feb-30` |
| `[2017, 12, 1]` (month 12, 0-based) | invalid | `2018-01-01` | `construct-array-month-12` |
| `[2017, 6, -1]` | invalid | `2017-06-29` | `construct-array-negative-day` |
| `{year:2017, month:1, day:30}` | invalid | `2017-03-02` | `construct-object-feb-30` |
| `{year:2017, month:13, day:1}` | invalid | `2018-02-01` | `construct-object-month-13` |

Note the asymmetry: the ISO string path *does* reject `2017-02-30`,
`2019-02-29`, `2017-13-01`, `2017-01-32`, `2017-04-31`, `25:00`, `23:60` and
`23:59:60` (all 18 `validity-oor-*`/`validity-garbage-*` cases match). Only the
token-format, array and object paths normalise. Clamping in *arithmetic* is
correct everywhere: all 12 `clamp-*` cases and both `setMonth1-*` cases match.

### ISO / RFC parsing gaps

| input | upstream | Go port | case |
| --- | --- | --- | --- |
| `2017-07-14T02:40:00+0530` (basic offset) | valid | invalid | `parse-iso-offset-basic` |
| `2017` | `2017-01-01` | invalid | `parse-iso-year` |
| `2017-07` | `2017-07-01` | invalid | `parse-iso-year-month` |
| `2017-W28` | `2017-07-10` | invalid | `parse-iso-week` |
| `2017-W28-5` | `2017-07-14` | `2017-05-28` (wrong) | `parse-iso-week-day` |
| `2017-195` (ordinal date) | `2017-07-14` | invalid | `parse-iso-ordinal` |
| `2017-07-14T24:00:00Z` | `2017-07-15T00:00` | invalid | `parse-iso-hour-24` |
| `Tue, 01 Nov 16 …` (2-digit RFC year) | valid | invalid | `parse-rfc-two-digit-year` |
| `["YYYY-MM-DD","YYYY-MM-DD HH:mm"]` over `2017-07-14 02:40` | picks the *better* format (keeps 02:40) | picks the first that parses (drops the time) | `parse-formats-ambiguous` |
| `GGGG WW E` tokens | parses | not supported (falls back to the current year) | *excluded — the port's answer depends on "now"* |

### Relative time and durations

| case | upstream | Go port |
| --- | --- | --- |
| `from-future-0s`, `from-past-0s`, `to-0s`, `humanize-suffix-zero` | `a few seconds ago` | `in a few seconds` — zero-length differences take the future suffix |
| `from-*-3974400s` (46 days) | `in a month` / `a month ago` | `in 2 months` / `2 months ago` |
| `from-future-47347200s` (548 days) | `in a year` | `in 2 years` |
| `parseDuration-6` (`P1YT-1H`) | valid, `P1YT-1H` | error |
| `parseDuration-13`…`-18` (`garbage`, ``, `P`, `PT`, `1Y`, `P1H`) | valid zero duration | error |

Every other relative-time threshold matches, including both sides of the
`ss`=44, `s`=45, `m`=45, `h`=22, `d`=26 and `M`=11 cutoffs, in both directions
(77/83 `relative` cases). All 16 ISO-duration serialisations round-trip
identically, including the mixed-sign `-P1YT-1H` form.

### Formatting

| case | upstream | Go port |
| --- | --- | --- |
| `token-zz-*` | `Coordinated Universal Time` | `UTC` |
| `token-l-*`, `-ll-*`, `-lll-*`, `-llll-*` | short localized dates | the token text verbatim — not implemented |
| `format-empty` (`format("")`) | falls back to the default ISO format | the empty string |
| `toISOStringZone-utc` | `…+00:00` | `…Z` |
| `components-d/-e/-f` (`isoWeeksInYear`) | 52 | 53 for ISO-week-years 2020 and 2015 |

All 62 remaining format tokens match on all four instants, as do all 8 composite /
escaping format strings and every serialisation helper.

### Locale data

The port bundles ~21 locales against moment's ~140, and within the bundled set:

| case | upstream | Go port |
| --- | --- | --- |
| `months-ru` | genitive (`января`) | nominative (`январь`) |
| `monthsShort-ru` | `мар.`/`мая` vs `март`/`май` mixture | different abbreviations |
| `months-hi` | `फरवरी`, `सितंबर`, `नवंबर`, `दिसंबर` | `फ़रवरी`, `सितम्बर`, `नवम्बर`, `दिसम्बर` |
| `ordinal-ru-*` | `1-го` | `1.` |
| `ordinal-tr-*` | `1'inci` | `1.` |
| `ordinal-fr-*` (n > 1) | bare `2` for the `D` token | `2e` |
| `formatLocale-ar-LLLL`, `humanizeLocale-ar`, `calendarLocale-ar` | Arabic-Indic digits (`١٤`) | ASCII digits |
| `formatLocale-hi-LLLL`, `humanizeLocale-hi`, `calendarLocale-hi` | Devanagari digits (`१४`) | ASCII digits; also a different meridiem word |
| `longDateFormat-ar-L` | embeds RLM marks | plain `D/M/YYYY` |
| `longDateFormat-zh-cn-LLL/LLLL` | `Ah点mm分` | `HH:mm` |
| `formatLocale-pl-LLLL` | genitive `lipca` | nominative `lipiec` |
| `formatLocale-cs-LLLL` | genitive `července` | nominative `červenec` |

All weekday name lists (`weekdays`, `weekdaysShort`, `weekdaysMin`) match for
every bundled locale, as do the `en`, `en-gb`, `es`, `it`, `pt-br`, `nl`, `sv`
long-date formats and ordinals.

### `normalizeUnits`

`normalizeUnit-e`: moment maps `"e"` to `"weekday"`; the port does not recognise
it. The other 11 aliases match.

## Declared deviations

Counted separately from mismatches (see `deviation` in the case files):

| case | why |
| --- | --- |
| `set-month-0`, `set-month-3`, `set-month-11` | moment's month index is 0-based; the port documents `Set(Month, …)` and `Get(Month)` as 1-based to match `time.Month` |
| `get-month` | same |
| `isBetween-reversed-bounds` | the port documents accepting the bounds in either order; moment requires `start <= end` |
