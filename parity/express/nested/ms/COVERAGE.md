# `express/ms` vs npm `ms` — API coverage

Nested parity harness for `github.com/malcolmston/express/ms`, a port of the npm
package [`ms`](https://github.com/vercel/ms). Scored by
`GOWORK=off go test .` in this directory, which regenerates `parity.json`.

## Which upstream, and why

The pinned oracle is **`ms@4.0.0-nightly.202508271359`** — the published build of
`vercel/ms` `main`. That pin is deliberate and needs stating, because npm's
`latest` tag for `ms` is still the much older `2.1.3`:

| | `ms@2.1.3` (npm `latest`) | `ms@4.0.0-nightly.202508271359` (`main`) |
| --- | --- | --- |
| exports | one default function | `ms`, `parse`, `format`, `parseStrict` |
| format ladder | d, h, m, s, ms | **y, mo, w**, d, h, m, s, ms |
| `mo` / `month(s)` parse unit | not accepted | accepted (`y/12`, i.e. 30.4375 d) |
| package type | CJS | ESM-only |

The Go port implements the `main` behaviour: it has the year/month/week rungs,
it accepts month units, and its own `ms_parity_test.go` transcribes vectors from
`vercel/ms` `main`'s `src/index.test.ts`. Measuring it against `2.1.3` would
score it down for features it deliberately ports, so the oracle is the revision
the port actually targets. The nightly is an exact, immutable npm version, so
the score stays reproducible.

If you care about `2.1.3` parity instead, the delta is exactly the table above:
this port emits `w`/`mo`/`y` where `2.1.3` emits days, and accepts month units
`2.1.3` rejects. Everything else in the two upstreams is identical.

## How the symbol lists were derived

Upstream, from the installed package in `node/node_modules`:

```
$ cd node && node --input-type=module -e 'import * as m from "ms"; console.log(Object.keys(m).sort().join("\n"))'
format
ms
parse
parseStrict
```

Port, from the local submodule:

```
$ cd ../../../../express && GOWORK=off go doc ./ms | grep -E '^func|^type|^const|^var'
func Format(d time.Duration) string
func FormatLong(d time.Duration) string
func Parse(s string) (time.Duration, error)
```

## Symbol inventory

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `ms.parse(str)` | `ms.Parse` | match | all 27 `p-*` in `parse-short`, all 29 in `parse-long`, 25 of 27 in `parse-invalid` | every unit spelling, sign, fraction, whitespace and rejection form |
| `ms.parseStrict(str)` | `ms.Parse` | match | `x-strict-2h`, `x-strict-30d`, `x-strict-1y`, `x-strict-empty`, `p-err-strict-word` | upstream differs from `parse` only in its TypeScript argument type; at runtime it *is* `parse`. Go has no `StringValue` template-literal type, so there is nothing to separate. |
| `ms.format(ms)` | `ms.Format` | match | all 34 `f-*` | short ladder y/mo/w/d/h/m/s/ms, both signs, every boundary |
| `ms.format(ms, {long:true})` | `ms.FormatLong` | match | all 33 `fl-*` | the `long` option is a second function in Go, not an option struct |
| `ms(value, options?)` (default, dual dispatch) | — (`ms.Parse` / `ms.Format` / `ms.FormatLong`) | differs | `x-ms-string-*`, `x-ms-number-*`, `p-err-ms-empty` | Upstream dispatches on `typeof value` at runtime. A statically typed Go API cannot and should not: the two directions are separate functions. The *behaviour* of both arms is compared and matches; only the spelling differs. |
| — | `ms.Format` / `ms.FormatLong` split | extra | as above | consequence of the row above, not an added feature |

Options surface (upstream's only option object):

| upstream option | Go equivalent | status | cases |
| --- | --- | --- | --- |
| `{ long: boolean }` on `format` / `ms` | `ms.FormatLong` vs `ms.Format` | match | every `fl-*` and `x-ms-number-long*` |

## Deviations

Both are counted separately from mismatches in `parity.json` and are listed in
`express/API-DEVIATIONS.md`.

| case | deviation |
| --- | --- |
| `p-err-overflow-1e30` | `ms.parse("1000000000000000000000000000000")` returns the JavaScript number `1e30`. A `time.Duration` is `int64` nanoseconds and spans about ±292 years, so the port returns an explicit range error. |
| `p-err-overflow-years` | Same cause for `"100000y"`. |

Both are inherent to the return type, not defects: before this harness the port
performed an out-of-range `float64 → time.Duration` conversion, whose result Go
leaves implementation-defined. The fix made the refusal explicit.

## Behaviour differences that are *not* mismatches

- **Failure signalling.** `ms.parse` throws for a non-string, empty or over-100
  character input and returns `NaN` for a string that does not match the
  grammar. The Go port returns an `error` in both cases. `node/run.js`
  normalises `NaN` into a throw so the harness compares the comparable fact —
  *whether* the call failed — as `parity/HARNESS.md` prescribes. Message text is
  not compared.
- **`0.5ms` formatting.** Upstream's milliseconds rung interpolates the number
  unrounded (`` `${ms}ms` ``). The port now does the same via `numStr`; it used
  to round.
- **Half-value rounding.** Upstream uses `Math.round`, which rounds a half
  towards +∞ (`Math.round(-1.5) === -1`). Go's `math.Round` rounds half away
  from zero. The port now reproduces the JavaScript rule with
  `math.Floor(x+0.5)`.

## Counts

| | |
| --- | --- |
| upstream symbols enumerated | 4 |
| compared (`match` + `differs`) | 4 |
| `match` | 3 |
| `differs` | 1 (`ms()` dual dispatch — behaviour matches, shape cannot) |
| `missing` | 0 |
| `extra` | 0 new capability |
| `untested` | 0 |
| **symbol parity** | **3/4 exact, 4/4 behaviourally reachable — 75% / 100%** |
| cases | **180** in 6 groups |
| cases compared | 178 |
| cases matching | 178 |
| **case parity** | **100.00%** |
| deviations | 2 |

Every upstream symbol has at least one case; nothing is `untested`.
