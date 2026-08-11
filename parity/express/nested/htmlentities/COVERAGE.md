# `express/htmlentities` vs npm `html-entities`

Oracle: **html-entities@2.6.0** (pinned in `node/package.json`).
Port: `github.com/malcolmston/express/htmlentities`.
Harness: `GOWORK=off go test .` in this directory, after `npm install` in `node/`.

## How the upstream inventory was derived

```
$ cd node && node -e "const m=require('html-entities');
  console.log(JSON.stringify(Object.keys(m).sort()));
  for (const k of Object.keys(m)) console.log(k, typeof m[k], m[k].length)"
["decode","decodeEntity","encode"]
encode function 2
decodeEntity function 2
decode function 2
```

Three exported functions. Their option surfaces are types, not runtime values, so
they were read from the installed `src/index.ts` (`EncodeMode`, `Level`,
`DecodeScope`, `EncodeOptions`, `DecodeOptions`) — the same file the reference
tables are generated from. The Go side was enumerated with
`GOWORK=off go doc -all ./htmlentities` in the `express` submodule.

The port's `tables.go` is **generated** from the installed package by
`node/gen-tables.js`, so the entity data is mechanically derived from the oracle
rather than transcribed:

```
$ cd node && node gen-tables.js > ../../../../../express/htmlentities/tables.go
```

That emits, per level, upstream's `entities` and `characters` maps (5 / 5 for xml,
353 / 253 for html4, 2231 / 1513 for html5), the 28-entry `numericUnicodeMap`, and
the three body-scope matching patterns verbatim.

## Symbol inventory

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `encode(text, options?)` | `htmlentities.Encode(string, ...EncodeOptions) string` | match | `enc-*` (43), `rt-*` (6) | |
| `decode(text, options?)` | `htmlentities.Decode(string, ...DecodeOptions) string` | match | `dec-*` (51) | |
| `decodeEntity(entity, options?)` | `htmlentities.DecodeEntity(string, ...DecodeOptions) string` | match | `ent-*` (13) | |
| `EncodeOptions.mode` | `EncodeOptions.Mode` | match | `enc-nonascii-*`, `enc-printable-*`, `enc-extensive*` | |
| `EncodeOptions.level` | `EncodeOptions.Level` | match | `enc-level-*`, `enc-nonascii-xml/html4/html5/all` | |
| `EncodeOptions.numeric` | `EncodeOptions.Numeric` | match | `enc-nonascii-hex`, `enc-nonascii-hex-astral`, `enc-nonascii-decimal-explicit`, `enc-extensive-hex` | |
| `DecodeOptions.level` | `DecodeOptions.Level` | match | `dec-level-*`, `ent-level-*` | |
| `DecodeOptions.scope` | `DecodeOptions.Scope` | match | `dec-scope-*` | |
| `EncodeMode` `'specialChars'` | `ModeSpecialChars` | match | `enc-specials-*`, `enc-empty-options` | |
| `EncodeMode` `'nonAscii'` | `ModeNonASCII` | match | `enc-nonascii-*` (12) | |
| `EncodeMode` `'nonAsciiPrintable'` | `ModeNonASCIIPrintable` | match | `enc-printable-controls`, `enc-printable-gaps`, `enc-printable-del`, `enc-printable-specials` | Upstream's ranges have gaps (U+0000, U+0009..U+0010 and U+0016 are *not* encoded); `enc-printable-gaps` pins them. |
| `EncodeMode` `'nonAsciiPrintableOnly'` | `ModeNonASCIIPrintableOnly` | match | `enc-printable-only-specials`, `enc-printable-only-xml` | |
| `EncodeMode` `'extensive'` | `ModeExtensive` | match | `enc-extensive*` (6) | |
| `Level` `'xml'` / `'html4'` / `'html5'` / `'all'` | `LevelXML` / `LevelHTML4` / `LevelHTML5` / `LevelAll` | match | `enc-level-*`, `enc-nonascii-{xml,html4,html5,all}`, `dec-level-*` | |
| `'decimal'` / `'hexadecimal'` | `NumericDecimal` / `NumericHexadecimal` | match | `enc-nonascii-hex`, `enc-nonascii-decimal-explicit` | |
| `DecodeScope` `'strict'` / `'body'` / `'attribute'` | `ScopeStrict` / `ScopeBody` / `ScopeAttribute` | match | `dec-scope-strict-*`, `dec-scope-body-*`, `dec-scope-attribute-*` | |
| `namedReferences` (internal, `src/named-references.ts`) | `entitiesXML/HTML4/HTML5`, `charactersXML/HTML4/HTML5` (unexported) | match | `dec-rare-named`, `dec-uppercase-named`, `dec-level-*`, `enc-nonascii-*` | Not exported by either side; generated from the oracle. |
| `numericUnicodeMap` (internal) | `numericUnicodeMap` (unexported) | match | `dec-numeric-nul`, `dec-numeric-c1`, `dec-numeric-c1-gap` | |
| `bodyRegExps` (internal) | `bodyPattern` (unexported) | match | `dec-legacy-nosemi*`, `dec-level-html4-nosemi`, `dec-level-html5-ambiguous` | Upstream's regex sources verbatim; RE2-compatible, and Go alternation is leftmost-first as JavaScript's is. |
| — | `htmlentities.Level`/`Mode`/`Scope` constants | extra | as above | Named constants for values upstream expresses as TypeScript string-literal unions. Not a behavioural addition. |

### Counts

| status | symbols |
| --- | --- |
| match | 20 |
| differs | 0 |
| missing | 0 |
| extra | 1 |
| untested | 0 |

**Parity: 20/20 compared symbols (100%).**
**Cases: 113 total, 110 match, 0 mismatch, 3 declared deviations — 100% of compared cases.**

Per group: `encode` 43 (41 match, 2 deviations), `decode` 70 (69 match, 1 deviation).

## Declared deviations

Listed in the `express` submodule's `API-DEVIATIONS.md`:

* `enc-unknown-mode`, `enc-unknown-level`, `dec-unknown-level` — an unrecognized
  option value falls back to the safest default instead of throwing. Upstream has
  no defined behaviour there: it indexes a lookup table with the value and hands
  the resulting `undefined` to `String.prototype.replace`, which throws.

## Message text and non-behavioural differences

* **Lone surrogates.** `decode("&#xD800;")` leaves upstream holding an unpaired
  UTF-16 surrogate. A Go string cannot hold one, so `Decode` yields U+FFFD. The
  case `dec-numeric-surrogate` *passes* because the JSON transport between the two
  runners cannot carry a lone surrogate either — both sides arrive as U+FFFD. The
  case is kept as documentation of a real difference the harness cannot observe.
  It is described in `API-DEVIATIONS.md` and is not counted as a deviation,
  because the harness genuinely sees equal values.
* **Null coercion.** `encode(null)`, `encode(undefined)` and `decode(null)` return
  `""` upstream; the Go functions take a `string`, so `""` is the only way to
  express the same call and `enc-empty` / `dec-empty` cover it.
* **NaN path.** `decodeEntity("&#;")` and `decodeEntity("&#x;")` reach
  `String.fromCharCode(NaN)` upstream, which is U+0000. The port reproduces that
  exactly (`ent-hash-only`, `ent-hex-empty`) rather than tidying it up, because
  the two must agree byte for byte.

## What was fixed while measuring

Measured parity went from **55/110 (50.0%)** to **110/110 (100%)**. The pre-parity
port had `Mode` only, a hand-written 32-entry named-entity table, decimal-only
numeric encoding, no `decodeEntity`, no legacy semicolon-less decoding, no
`scope`, and no Windows-1252 remapping for numeric references in the C1 range.
See `API-DEVIATIONS.md` for the full list.
