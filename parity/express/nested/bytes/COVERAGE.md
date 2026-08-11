# `express/bytes` vs npm `bytes` — API coverage

Upstream oracle: **bytes@3.1.2** (npm `latest`; `npm view bytes version` → `3.1.2`).
That is also the revision the port was written against — `express/bytes/bytes_parity_test.go`
cites this tag's `index.js`, `test/bytes.js` and `Readme.md` by URL.

Port: `github.com/malcolmston/express/bytes` (local submodule, via the `replace` in `go.mod`).

## How the two inventories were produced

Upstream, from the installed package rather than the README:

```console
$ cd node && node -e 'const b=require("bytes"); console.log(typeof b, JSON.stringify(Object.keys(b).sort()), b.name)'
function ["format","parse"] bytes
$ cd node && node -e 'const b=require("bytes"); console.log(Object.getOwnPropertyNames(b).sort().join(" "))'
format length name parse prototype
$ cd node && node -e 'console.log(require("bytes/package.json").version)'
3.1.2
```

So the whole public surface is three callables — the default export `bytes`,
`bytes.parse` and `bytes.format` — plus the options object `bytes.format` reads.
(`length`, `name` and `prototype` are intrinsic function properties, not API.)
The options object is not reflectable, so its fields are enumerated from the
installed `node_modules/bytes/index.js` lines 90-94, which read exactly
`thousandsSeparator`, `unitSeparator`, `decimalPlaces`, `fixedDecimals`, `unit`.

The port:

```console
$ cd ../../../../express && GOWORK=off go doc ./bytes
const ( B, KB, MB, GB, TB, PB int64 )
func Format(n int64) string
func FormatOpts(n int64, opts FormatOptions) string
func Parse(s string) (int64, error)
type FormatOptions struct { DecimalPlaces *int; FixedDecimals bool; UnitSeparator string; ThousandsSeparator string; Unit string }
```

## Symbol table

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `bytes(string)` | `bytes.Parse` | match | `x-str-1kb`, `x-str-1024`, `x-str-1p5gb`, `x-str-bad` | the default export's string arm; Go has no runtime type dispatch, so it is the same function as `Parse` |
| `bytes(number)` | `bytes.Format` | match | `x-num-1024`, `x-num-zero`, `x-num-neg` | number arm |
| `bytes(number, options)` | `bytes.FormatOpts` | match | `x-num-opts`, `x-num-opts-u` | |
| `bytes(other)` → `null` | — | match | `x-other-true`, `x-other-null`, `x-other-obj`, `x-other-arr` | upstream returns `null` for anything that is neither string nor number; the port has no entry point that accepts such a value at all, so both sides decline |
| `bytes.parse(string)` | `bytes.Parse` | match | 44 cases in `parse-units`, 22 in `parse-invalid`, 19 in `parse-bare` | includes the anchored unit grammar, the `parseInt(val, 10)` fallback, and every failure mode |
| `bytes.parse(string)` overflow | `bytes.Parse` | differs | `pb-int64max`, `pb-1e21`, `pb-1e23`, `pb-pb-overflow` | upstream returns an oversized double; an `int64` cannot, so the port returns an overflow error instead of an implementation-defined conversion |
| `bytes.parse(number)` | — | missing | `pn-int`, `pn-zero`, `pn-neg`, `pn-frac` | the number branch is the identity function; Go has no overloading and `Parse` is string-typed |
| `bytes.parse(NaN)` → `null` | `bytes.Parse` | match | `pi-nan-parse` | both sides fail |
| `bytes.format(number)` | `bytes.Format` | match | 43 cases in `format-auto` | every unit threshold, one byte either side of it, negatives, zero, `toFixed` tie-breaking |
| `bytes.format(number, options)` | `bytes.FormatOpts` | match | 56 cases in `format-options` | |
| `bytes.format(non-finite)` → `null` | — | n/a | — | `Number.isFinite` guard; an `int64` argument is finite by construction, so there is nothing to compare |
| `options.decimalPlaces` | `FormatOptions.DecimalPlaces` | match | `fo-dp0-1740`, `fo-dp0-1536`, `fo-dp0-2560`, `fo-dp0-neg-2560`, `fo-dp0-neg-small`, `fo-dp1`, `fo-dp1-1434`, `fo-dp2`, `fo-dp3-1034`, `fo-dp3-trim`, `fo-dp4-1034`, `fo-dp5`, `fo-dp10`, `fo-dp20-trim`, `fo-dp0-zero` | `*int` stands in for `!== undefined` |
| `options.decimalPlaces` out of range | `FormatOptions.DecimalPlaces` | differs | `fo-dp-negative`, `fo-dp-101` | `toFixed` throws `RangeError` outside 0..100; `FormatOpts` returns a bare string and clamps |
| `options.decimalPlaces: null` | `FormatOptions.DecimalPlaces` | differs | `fo-dp-null` | upstream tests only `!== undefined`, so `null` becomes `toFixed(0)`; a Go `*int` cannot distinguish `null` from absent |
| `options.fixedDecimals` | `FormatOptions.FixedDecimals` | match | `fo-fixed`, `fo-fixed-false`, `fo-fixed-b`, `fo-fixed-zero`, `fo-fixed-dp0`, `fo-fixed-dp4`, `fo-fixed-neg` | |
| `options.thousandsSeparator` | `FormatOptions.ThousandsSeparator` | match | `fo-thou-1000`, `fo-thou-999`, `fo-thou-empty`, `fo-thou-dot`, `fo-thou-7digit`, `fo-thou-neg`, `fo-thou-4digit`, `fo-thou-frac`, `fo-thou-15digit`, `fo-thou-multichar` | |
| `options.unitSeparator` | `FormatOptions.UnitSeparator` | match | `fo-usep-space`, `fo-usep-dash`, `fo-usep-empty`, `fo-usep-b`, `fo-usep-neg` | |
| `options.unit` | `FormatOptions.Unit` | match | `fo-unit-KB`, `fo-unit-kb-lower`, `fo-unit-Mb-mixed`, `fo-unit-B`, `fo-unit-b-lower`, `fo-unit-GB-small`, `fo-unit-PB-small`, `fo-unit-TB`, `fo-unit-bogus`, `fo-unit-empty`, `fo-unit-kib`, `fo-unit-zero`, `fo-unit-neg` | caller's spelling is echoed; an unrecognised unit falls back to auto-selection |
| `map.b`/`kb`/`mb`/`gb`/`tb`/`pb` (private) | `bytes.B`, `KB`, `MB`, `GB`, `TB`, `PB` | extra | `pu-kb-*`, `pu-mb-*`, `pu-gb-*`, `pu-tb-*`, `pu-pb-*`, `fa-kb`, `fa-mb`, `fa-gb`, `fa-tb`, `fa-pb` | upstream's ladder is a module-private object; the port exports the six magnitudes as constants. Their values are exercised through every parse/format case. |
| — | `bytes.Format` (as distinct from `FormatOpts`) | extra | all of `format-auto` | Go has no default arguments, so the zero-options call is its own function |
| — | (composition) | untested-upstream | `roundtrip` group, 21 cases | not an upstream symbol; `format∘parse` and `parse∘format` are compared on both sides anyway |
| `bytes.format` int64 precision | `bytes.FormatOpts` | differs | `fo-unit-b-2pow53` | beyond 2^53 upstream only has the even double neighbour; the port holds the exact `int64` |

## Counts

| | |
| --- | --- |
| upstream symbols/behaviours enumerated | 20 |
| `match` | 12 |
| `differs` | 5 |
| `missing` | 1 |
| `extra` (Go-only) | 2 |
| `n/a` (unrepresentable in Go's type system) | 1 |
| **parity over the symbols actually compared** (`match / (match + differs + missing)`) | **12 / 18 = 66.67 %** |

Symbol-level parity is the pessimistic view: five of the six non-`match` rows are
a single behaviour each (out-of-range `decimalPlaces`, explicit `null`,
past-int64 overflow, past-2^53 precision, the number overload of `parse`), and
every one of them is a consequence of `int64`/`*int` versus a JavaScript double.
Case-level parity is the measured number:

| | |
| --- | --- |
| cases total | **230** |
| compared | 218 |
| match | 218 |
| mismatch | 0 |
| documented deviations (not compared) | 12 |
| **case parity** | **100.00 %** |

Deviations are listed in `express/API-DEVIATIONS.md` and carried in the case
files as `"deviation": "…"`. Regenerate everything with:

```console
cd parity/express/nested/bytes && GOWORK=off go test .
```

## Message-text differences (not scored)

The harness compares *whether* a call failed, never the text. For the record:
upstream signals failure by returning `null` from `bytes.parse`/`bytes()` and
never throws, while the port returns `bytes: invalid value "…"` or
`bytes: value "…" overflows int64`. The one place upstream does throw is
`Number.prototype.toFixed` rejecting a `decimalPlaces` outside 0..100 with a
`RangeError`, which the port cannot reproduce (see `fo-dp-negative`).
