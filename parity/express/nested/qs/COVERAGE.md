# `express/qs` — upstream API inventory vs the Go port

Upstream oracle: **`qs@6.15.3`** (ljharb/qs), pinned in [`node/package.json`](node/package.json).
Go port under measurement: `github.com/malcolmston/express/qs`, via the `replace`
in [`go.mod`](go.mod) — the harness measures the local submodule, not a published
tag.

Score: [`parity.json`](parity.json), rewritten by `GOWORK=off go test .`

## How this inventory was produced

Nothing below is from memory or from the README. The three lists were taken
mechanically from the installed package and from the built port:

```sh
# 1. the module's exported names
cd node && node -e 'const qs=require("qs"); console.log(Object.keys(qs).sort())'
#    -> [ 'formats', 'parse', 'stringify' ]

# 2. the documented option keys of parse and of stringify, read out of the
#    `defaults` object each module normalises its options against
cd node && node -e '
  const fs=require("fs");
  const grab=(f)=>fs.readFileSync(f,"utf8").match(/var defaults = \{([\s\S]*?)\n\};/)[1]
    .split("\n").map(l=>l.trim().match(/^([A-Za-z]+):/)).filter(Boolean).map(m=>m[1]).sort();
  console.log("parse:", grab("node_modules/qs/lib/parse.js"));
  console.log("stringify:", grab("node_modules/qs/lib/stringify.js"));'

# 3. the port's exported surface
cd ../../../../../express && GOWORK=off go doc -all ./qs
```

The option keys are part of the upstream surface — `qs.parse(str, opts)` is the
whole API, so an unimplemented option is an unimplemented feature — and each one
therefore gets its own row. `sort` does not appear in stringify's `defaults`
object (it is read straight off `opts` and defaults to `null`) but is documented
and is used by this harness, so it is listed too.

## Determinism: what the harness normalises, and why

Upstream qs follows JavaScript object insertion order; a Go map has no order at
all. Two normalisations make the two answers comparable, both applied on **both**
sides, as `parity/HARNESS.md` requires:

* **parse** — every object in a result is rebuilt with its keys deep-sorted before
  it is emitted (`normalize()` in `node/run.js`; Go's `encoding/json` sorts map
  keys itself). Array order is meaningful in both languages and is left alone.
* **stringify** — key order is part of the output *string*, so it cannot be sorted
  afterwards. The harness instead hands upstream **its own `sort` option** with a
  code-unit comparator (`(a,b) => a<b ? -1 : a>b ? 1 : 0`), which is the order
  Go's `sort.Strings` produces; the port always sorts. Array indices are object
  keys too, so stringify cases keep arrays under ten elements, where `"0".."9"`
  sort identically as text and as numbers.

Numeric limits are written the way upstream takes them, with the string
`"Infinity"` standing in for `Infinity` so the same case file feeds both runners;
the Go runner maps it onto the port's negative "no limit".

## Inventory

Status is one of `match`, `differs`, `missing`, `extra`, `untested`.

### Exported names

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `qs.parse` | `qs.Parse`, `qs.ParseWith` | match | `x-exports`, and all 178 `parse` cases | `Parse` is the no-options form |
| `qs.stringify` | `qs.StringifyWith` | match | `x-exports`, and all 71 `stringify` cases | |
| `qs.formats` | `qs.FormatRFC3986`, `qs.FormatRFC1738` | match | `x-formats` | an object upstream, two constants here; `formats.default` is RFC3986 on both |
| — | `qs.Stringify` | extra | `s-*` indirectly | Go-only convenience with this port's historical shape (brackets array format, `x-www-form-urlencoded` encoding). `StringifyWith` is the upstream-faithful entry point |
| — | `qs.DefaultDepth`, `DefaultParameterLimit`, `DefaultArrayLimit`, `DefaultDelimiter` | extra | — | the upstream defaults named, since a Go zero value cannot carry them |
| — | `qs.DuplicatesCombine/First/Last`, `qs.ArrayFormat*` | extra | `d-*`, `t-af-*` | string constants for upstream's string-valued options |

### `qs.parse` options

| upstream option | Go field | status | cases | note |
| --- | --- | --- | --- | --- |
| `allowDots` | `ParseOptions.AllowDots` | match | `o-dots-one`, `o-dots-two`, `o-dots-mixed`, `o-dots-bracket-first`, `o-dots-trailing`, `o-dots-leading`, `o-dots-double`, `o-dots-index`, `o-dots-off`, `p-proto-dots`, `r-dots` | |
| `allowEmptyArrays` | `ParseOptions.AllowEmptyArrays` | match | `o-empty-arrays-on`, `o-empty-arrays-off` | |
| `allowPrototypes` | `ParseOptions.AllowPrototypes` | match | `p-proto-allow`, `p-proto-allow-top`, `p-constructor-allow`, `p-tostring-allow` | |
| `allowSparse` | — | missing | — | the port always compacts holes; `[ ,'b']` has no natural Go spelling |
| `arrayLimit` | `ParseOptions.ArrayLimit` | match | `a-index-limit-edge`, `a-index-limit-at`, `a-index-past-limit`, `a-index-huge`, `a-limit-1`, `a-limit-0`, `a-limit-large`, `a-push-limit-1`, `a-limit-push-over` | tested with upstream's strict `index < arrayLimit`, and it also caps how many values one key gathers |
| `charset` | — | missing | — | `iso-8859-1` input is not supported; the port is UTF-8 only |
| `charsetSentinel` | — | missing | — | depends on `charset` |
| `comma` | `ParseOptions.Comma` | match | `o-comma-on`, `o-comma-three`, `o-comma-off`, `o-comma-nested`, `o-comma-empty` | |
| `decodeDotInKeys` | — | missing | — | |
| `decoder` | — | missing | — | no pluggable decoder; a Go function field would not be comparable anyway |
| `delimiter` | `ParseOptions.Delimiter` | match | `o-delim-semi`, `o-delim-comma`, `o-delim-unused`, `o-delim-multichar` | upstream also accepts a RegExp; the port takes a string only |
| `depth` | `ParseOptions.Depth` | differs | `n-six-past-cap`, `n-eight-past-cap`, `n-hundred-brackets`, `o-depth-1`, `o-depth-2`, `o-depth-10`, `o-depth-inf`, `o-depth-zero` | upstream's `depth: 0` is unreachable: the port's 0 means "the documented default". Deviation, see `express/API-DEVIATIONS.md` |
| `duplicates` | `ParseOptions.Duplicates` | differs | `d-last`, `d-first`, `d-combine-explicit`, `d-last-nested`, `d-bad-duplicates` | all three modes behave identically; an *invalid* value throws upstream and falls back to `combine` here, because `ParseWith` has no error return. Deviation |
| `ignoreQueryPrefix` | `ParseOptions.IgnoreQueryPrefix` | match | `f-question-default`, `o-prefix-on`, `o-prefix-off`, `o-prefix-only`, `o-prefix-double` | |
| `interpretNumericEntities` | — | missing | — | depends on `charset: 'iso-8859-1'` |
| `parameterLimit` | `ParseOptions.ParameterLimit` | match | `o-param-limit-1`, `o-param-limit-2`, `o-param-limit-big`, `o-param-limit-inf` | |
| `parseArrays` | — | missing | — | a negative `ArrayLimit` reaches part of the same place, but `parseArrays: false`'s treatment of `a[]` does not |
| `plainObjects` | `ParseOptions.PlainObjects` | match | `p-proto-plain`, `p-tostring-plain`, `p-proto-plain-top` | switches the prototype guard off, as upstream's does. Upstream additionally returns null-prototype objects, which a Go map is already equivalent to |
| `strictDepth` | — | missing | — | upstream throws a `RangeError` past the cap; `ParseWith` has no error return |
| `strictMerge` | — | missing | — | hard-coded to upstream's default `true`, so an object and a scalar at one path stay side by side (`d-object-and-scalar`) |
| `strictNullHandling` | `ParseOptions.StrictNullHandling` | match | `o-null-strict`, `o-null-strict-nested`, `o-null-strict-off` | |
| `throwOnLimitExceeded` | — | missing | — | upstream throws instead of truncating; `ParseWith` has no error return |

### `qs.stringify` options

| upstream option | Go field | status | cases | note |
| --- | --- | --- | --- | --- |
| `addQueryPrefix` | `StringifyOptions.AddQueryPrefix` | match | `t-prefix-on`, `t-prefix-empty`, `t-prefix-off`, `t-combo` | |
| `allowDots` | `StringifyOptions.AllowDots` | match | `t-dots-on`, `t-dots-deep`, `t-dots-array`, `r-dots` | |
| `allowEmptyArrays` | `StringifyOptions.AllowEmptyArrays` | match | `t-empty-arrays-on`, `t-empty-arrays-nested`, `t-empty-arrays-off` | including upstream's quirk of emitting that one marker with unencoded brackets |
| `arrayFormat` | `StringifyOptions.ArrayFormat` | match | `t-af-indices`, `t-af-brackets`, `t-af-repeat`, `t-af-comma`, `t-af-comma-one`, `t-af-comma-encode`, `t-af-*-nested`, `t-af-brackets-objects`, `t-af-repeat-objects`, `r-obj-array-*` | all four generators |
| `charset` | — | missing | — | UTF-8 only |
| `charsetSentinel` | — | missing | — | depends on `charset` |
| `commaRoundTrip` | — | missing | — | the `a[]=b` marker a one-element comma array would need |
| `delimiter` | `StringifyOptions.Delimiter` | match | `t-delim-semi`, `t-delim-comma` | |
| `encode` | `StringifyOptions.SkipEncoding` | match | `t-encode-false`, `t-encode-false-space`, `t-encode-false-array` | inverted sense so the Go zero value is upstream's default `true` |
| `encodeDotInKeys` | — | missing | — | |
| `encodeValuesOnly` | `StringifyOptions.EncodeValuesOnly` | match | `t-values-only`, `t-values-only-array` | |
| `encoder` | — | missing | — | no pluggable encoder |
| `filter` | — | missing | — | neither the function nor the allow-list array form |
| `format` | `StringifyOptions.Format` | differs | `t-fmt-3986`, `t-fmt-1738`, `t-fmt-1738-parens`, `t-fmt-1738-nested`, `t-fmt-1738-array`, `t-fmt-bad`, `s-space-value`, `s-parens`, `r-space-1738` | both formats are byte-identical; an *unknown* format throws upstream and selects RFC3986 here. Deviation |
| `formatter` | — | missing | — | upstream derives it from `format`; not separately settable here |
| `indices` | — | missing | — | deprecated upstream in favour of `arrayFormat` |
| `serializeDate` | — | missing | — | the port has no `time.Time` leaf handling |
| `skipNulls` | `StringifyOptions.SkipNulls` | match | `t-skip-nulls`, `t-skip-nulls-nested` | |
| `sort` | — | differs | every `stringify` case | the port *always* sorts keys at every level, because a Go map has no insertion order to preserve; upstream sorts only when given a comparator. The harness supplies that comparator so the two are comparable — see "Determinism" above |
| `strictNullHandling` | `StringifyOptions.StrictNullHandling` | match | `t-null-strict`, `t-null-strict-nested` | |

## Counts

| status | count |
| --- | --- |
| match | 22 |
| differs | 4 |
| missing | 19 |
| extra | 3 rows (Go-only convenience API and named defaults) |
| untested | 0 |
| **symbols compared** (match + differs) | **26** |

**Symbol-level parity: 22 / 26 = 84.62 %** of the symbols actually compared behave
identically. Counting the 19 unimplemented options as failures instead gives
22 / 45 = 48.9 % of the full upstream surface — the honest reading is that the
port covers all of the parsing and serialisation *behaviour* Express depends on,
including every limit and the prototype guard, and none of the pluggable hooks
(`decoder`/`encoder`/`filter`/`serializeDate`), the `iso-8859-1` charset family,
or the options whose only purpose is to raise an error (`strictDepth`,
`throwOnLimitExceeded`), which a Go signature with no error return cannot do.

## Case total

**271 cases** across 11 groups, of which **268 are compared** and **3 are
documented deviations**: `o-depth-zero`, `d-bad-duplicates`, `t-fmt-bad`. Current
score: 268 / 268 compared cases match — see [`parity.json`](parity.json) and
`express/API-DEVIATIONS.md`.

| group | cases | what it covers |
| --- | --- | --- |
| `parse-flat` | 20 | flat pairs, empty pairs, missing `=`, empty keys, `?` handling |
| `parse-nesting` | 27 | bracket nesting at every depth, past the cap, unbalanced and unterminated brackets, scalar/object collisions |
| `parse-arrays` | 33 | `[]` pushes, numeric indices, gaps, out-of-order and huge indices, `arrayLimit` in both of its roles |
| `parse-duplicates` | 16 | repeated keys under all three `duplicates` modes |
| `parse-encoding` | 24 | percent-decoding, plus-as-space, invalid and truncated escapes, `%5B`/`%5D` normalisation |
| `parse-options` | 36 | every implemented parse option, including the limits |
| `parse-prototype` | 22 | `__proto__`, `constructor[prototype][x]`, `a[__proto__][b]`, the rest of `Object.prototype`, and both escape hatches |
| `stringify-basic` | 33 | defaults: nesting, arrays, encoding, UTF-8 and astral leaves, nulls |
| `stringify-options` | 38 | every implemented stringify option |
| `roundtrip` | 20 | `stringify(parse(s))` and `parse(stringify(o))` in both directions |
| `exports` | 2 | the module surface itself |

## Security note

Several of the fixes this harness prompted made the port *stricter*: it now
applies upstream's prototype guard, tests `arrayLimit` with a strict `<` as
upstream does, and caps how many values a repeated key may gather into one slice.
None of those gaps was exploitable in Go — a map has no prototype chain, and the
looser array bound was an off-by-one on an already-bounded allocation — so no
`security.json` was written. The reason to fix them anyway is second-order: a Go
service that echoes a parsed query back as JSON would otherwise hand a browser
the `__proto__` key upstream had already dropped.
