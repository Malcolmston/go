# timefmt — Go port of d3-time and d3-time-format: calendar intervals

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/timefmt.svg)](https://pkg.go.dev/github.com/malcolmston/d3/timefmt)

Package timefmt is a Go port of d3-time and d3-time-format: calendar
intervals, and strftime-style formatting and parsing of instants.

It exists so that a d3 specifier string works unchanged in Go. Go has
`time.Time` and its own reference-time layouts ("2006-01-02"), which are
lovely to read and completely incompatible with everything the d3 ecosystem
writes down. A time axis is configured with strings like "%b %d" or
"%Y-%m-%dT%H:%M:%S.%LZ", and those strings travel: they come from
configuration files, from URL parameters, from a JavaScript sibling rendering
the same chart. So this package implements d3's directive grammar directly
rather than translating it into Go layouts — a translation that could not be
faithful anyway, because Go has no layout for the ISO week-based year, the day
of the year, or the quarter.

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/timefmt"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func Format(specifier string) func(time.Time) string` | Format compiles a specifier into a formatting function using the default locale. |
| `func ISOFormat(t time.Time) string` | ISOFormat renders an instant as ISO 8601 in UTC, e.g. |
| `func ISOParse(s string) (time.Time, error)` | ISOParse parses the format `ISOFormat` produces. |
| `func Parse(specifier string) func(string) (time.Time, error)` | Parse compiles a specifier into a parsing function using the default locale. |
| `func Ticks(start, stop time.Time, count int) []time.Time` | Ticks returns approximately count evenly spaced, calendar-aligned instants covering [start, stop] inclusive. |
| `func UTCFormat(specifier string) func(time.Time) string` | UTCFormat is `Format` but renders every instant in UTC, so %H is the UTC hour and %Z is always "+0000". |
| `func UTCParse(specifier string) func(string) (time.Time, error)` | UTCParse is `Parse` but interprets unzoned fields as UTC. |
| `func UTCTicks(start, stop time.Time, count int) []time.Time` | UTCTicks is `Ticks` in UTC. |

### Types

| Type | What it is |
| --- | --- |
| `Interval` | An Interval is a unit of calendar time — a second, a day, a month, "every third hour", "every Monday" — that knows how to snap an instant to its… |
| `TimeLocale` | TimeLocale carries the names and the date/time patterns that make a format locale-specific. |

<details>
<summary><code>Interval</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func TickInterval(start, stop time.Time, count int) *Interval` | TickInterval returns the `Interval` a time scale should place its ticks on to get approximately count of them between start and stop, working in the… |
| `func UTCTickInterval(start, stop time.Time, count int) *Interval` | UTCTickInterval is `TickInterval` in UTC, for scales whose domain should not follow daylight saving. |
| `func (i *Interval) Ceil(t time.Time) time.Time` | Ceil returns the earliest boundary of this interval at or after t. |
| `func (i *Interval) Count(start, end time.Time) int` | Count returns the number of interval boundaries strictly after start and at or before end, after flooring both — d3's semantics. |
| `func (i *Interval) Every(step int) *Interval` | Every returns an interval containing every step-th boundary of this one, aligned to the natural start of the larger unit rather than to an arbitrary… |
| `func (i *Interval) Filter(test func(time.Time) bool) *Interval` | Filter returns a new interval containing only the boundaries of this one for which test reports true — every third hour, or only weekdays: |
| `func (i *Interval) Floor(t time.Time) time.Time` | Floor returns the latest boundary of this interval at or before t. |
| `func (i *Interval) Offset(t time.Time, step int) time.Time` | Offset advances t by step intervals without flooring, preserving the parts of the value smaller than the interval. |
| `func (i *Interval) Range(start, stop time.Time, step int) []time.Time` | Range returns every boundary in [start, stop), advancing step intervals at a time. |
| `func (i *Interval) Round(t time.Time) time.Time` | Round returns the boundary nearest to t, breaking a tie towards the later one — the same rule d3 uses, and the one that makes "round to the nearest… |
| `func (i *Interval) String() string` | String returns the interval's name, e.g. |

</details>

<details>
<summary><code>TimeLocale</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewTimeLocale(l TimeLocale) *TimeLocale` | NewTimeLocale returns a ready-to-use locale, filling in en-US defaults for every field left empty. |
| `func (l *TimeLocale) Format(specifier string) func(time.Time) string` | Format compiles a specifier against this locale. |
| `func (l *TimeLocale) Parse(specifier string) func(string) (time.Time, error)` | Parse compiles a parsing function that resolves unzoned fields in time.Local. |
| `func (l *TimeLocale) ParseIn(specifier string, loc *time.Location) func(string) (time.Time, error)` | ParseIn compiles a parsing function that resolves unzoned fields in loc. |
| `func (l *TimeLocale) UTCFormat(specifier string) func(time.Time) string` | UTCFormat compiles a specifier against this locale, rendering in UTC. |
| `func (l *TimeLocale) UTCParse(specifier string) func(string) (time.Time, error)` | UTCParse compiles a parsing function that resolves unzoned fields in UTC. |

</details>

### Constants

`ISOSpecifier`

### Variables

`Minute`, `Hour`, `Day`, `Month`, `Year`, `Sunday`, `Monday`, `Tuesday`, `Wednesday`, `Thursday`, `Friday`, `Saturday`, `Week`, `UTCMinute`, `UTCHour`, `UTCDay`, `UTCMonth`, `UTCYear`, `UTCSunday`, `UTCMonday`, `UTCTuesday`, `UTCWednesday`, `UTCThursday`, `UTCFriday`, `UTCSaturday`, `UTCWeek`, `EnUS`, `ErrParse`, `Millisecond`, `Second`, `UTCMillisecond`, `UTCSecond`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/timefmt).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
