# moment example

A single runnable program that exercises the real API of the **published**
`github.com/malcolmston/moment` module (the moment.js port) against fixed
reference timestamps, so its output is fully deterministic.

Resolved module version (from `go get github.com/malcolmston/moment@latest`):

```
github.com/malcolmston/moment v0.0.0-20260719133128-52105fccbcf9
```

There is no `replace` directive: the example consumes the module exactly as an
outside user would, from the module proxy.

## Run

```sh
cd examples/moment
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

The program prints labelled sections and, for every value it can hand-compute,
a `[ok]` / `[MISMATCH]` line plus a summary count. It always terminates and
exits 0.

## What it demonstrates

- **Parsing** — `Parse` (auto ISO), `ParseISO` (date-only), `ParseRFC2822`,
  `ParseFormat` with moment tokens (`DD/MM/YYYY HH:mm`, `MMMM D YYYY h:mm a`),
  `ParseFormats` (first matching layout wins), `ParseInLocation`,
  `ParseLayout` (raw Go layout), the `X` unix-seconds token, `ParseZone`, and
  `CreationData`.
- **Formatting** — the full token set: `YYYY/MM/DD`, `dddd`, `Do`, `Qo`,
  `DDD`/`DDDo`, `w`/`wo`/`ww`, `W`/`WW`, `E`/`e`, `gggg`/`GGGG`, `k`/`kk`,
  `h`/`hh`/`H`/`HH`, `a`/`A`, `X`/`x`, `Z`/`ZZ`/`z`, runs of `S`, bracketed
  literals, and the long-date tokens `LT`/`LTS`/`L`/`LL`/`LLL`/`LLLL` — plus
  `fr`/`de` locale formatting and `AvailableLocales`.
- **Manipulation** — `Add`/`Subtract`/`AddDuration`, unit aliases (`"days"`,
  `"Q"`), end-of-month clamping (2017-01-31 + 1 month = 2017-02-28), the
  setters (`SetYear` … `SetDayOfYear`, `SetAll` with `DateSpec`), `Max`/`Min`,
  the comparison predicates and `ToArray`. Immutability of the receiver is
  asserted.
- **Start/end of unit** — `StartOf`/`EndOf` for year, quarter, month, week,
  isoWeek, day, hour, minute, including the Sunday-vs-Monday difference between
  `Week` and `ISOWeek`.
- **Diffing** — `Diff` (float), `DiffInt` (truncating), `DiffDuration`, sign
  flip, and the fractional month diff.
- **Relative time** — `FromNow`/`From`/`To`/`ToNow` driven by
  `FixedClock` + `WithClock` (so "in 2 hours" is reproducible), `Calendar`
  against an explicit reference, localized output (`fr`/`de`/`ru`), the
  package-level `Humanize`, and `SetRelativeTimeThreshold` /
  `RelativeTimeThreshold`.
- **Durations** — `NewDuration`, `DurationFromTime`, `DurationBetween`,
  `ParseDuration` ISO-8601 round trip, `As*`/component getters,
  `Add`/`Subtract`/`Abs`, and `Humanize`.
- **Time zones** — `In`, `UTC`, `Location`, `ZoneAbbr`, `UTCOffset`, `IsDST`,
  `SetUTCOffset`, zone-local `StartOf(Day)`, instant preservation across zones,
  and a New York spring-forward crossing (01:30 + 1h = 03:30).
- **Invalid input** — failed token parses, `ParseFormatStrict` rejecting
  trailing garbage, `Invalid()`, the zero value, and a bad ISO duration.

All hand-computed expectations currently match (98 checks, 0 mismatches).

## Holes and rough edges found

1. **Out-of-range date components are silently normalized, not rejected.**
   `ParseFormat("2017-13-45", "YYYY-MM-DD")` returns a *valid* Moment for
   `2018-02-14`; `2017-07-32` becomes `2017-08-01` and `2017-02-30` becomes
   `2017-03-02`. `ParseFormatStrict` behaves identically. moment.js reports
   these as invalid dates even in non-strict mode, so `IsValid()` cannot be
   used to validate user input here. This is the most significant deviation
   found and is not documented in the README.

2. **`ParseFormatStrict` only checks for leftover input, not value ranges.**
   It correctly rejects `"2017-07-14 trailing"` against `"YYYY-MM-DD"`, but
   accepts every out-of-range value above — so "strict" is narrower than the
   name suggests.

3. **The zero-value `Moment` and `Invalid()` disagree on formatting.**
   `Invalid().Format("YYYY-MM-DD")` returns `"Invalid date"`, but a zero-value
   `Moment` — also `IsValid() == false` — formats as `"0001-01-01"`. Both
   return `""` from `ToJSON`, so the inconsistency is only in `Format`.

4. **`EndOf` lands on the last nanosecond, not the last millisecond.**
   `EndOf(Month).ISO()` is `2017-07-31T23:59:59.999999999Z` where moment.js
   yields `.999`. It is documented in the method comment, but it changes
   `IsSame`/`Diff` results at unit boundaries versus moment.js.

5. **`ISO()` is not moment.js `toISOString()`.** `ISO()` is `time.RFC3339Nano`,
   so an all-zero fraction is dropped (`2020-12-25T09:30:00Z`) and a
   sub-millisecond fraction is printed in full. The moment.js-compatible,
   fixed-3-digit form is the separately named `ToISOString()` /
   `ToISOStringZone()` / `ToJSON()`. Easy to reach for the wrong one; the
   README's API list does not flag the difference.

6. **Parse errors leak Go layout strings.** `Parse("total nonsense")` returns
   `parsing time "total nonsense" as "2006-01-02T15:04:05.999999999Z07:00": …`
   — a raw `time` package error mentioning a Go reference layout the caller
   never wrote. Errors from the token parser are wrapped nicely; the ISO path
   is not.

7. **`DateSpec.Month` is an `*int`, not a `*time.Month`**, while
   `SetMonth` takes a `time.Month`. Mixing the two in one program requires
   `iptr(int(time.December))`. Minor, but inconsistent within the package.

8. **Only ~20 locales are bundled** (documented): `ar cs de en en-gb es fr hi
   it ja ko nl pl pt pt-br ru sv tr zh-cn zh-tw`. Anything else needs
   `RegisterLocale`.

No compile failures, no panics, and no dependency problems: the module is
standard-library only and `go mod tidy` adds nothing.
