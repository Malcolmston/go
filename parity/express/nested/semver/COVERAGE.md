# `express/semver` vs npm `semver` — API coverage

Oracle: **semver@7.8.5** (npm `latest`, pinned in `node/package.json`).
Port: **github.com/malcolmston/express/semver** (measured against the local
submodule via the `replace` in `go.mod`).

Score: **320 cases, 313 compared, 313 match, 0 mismatch, 7 documented
deviations — 100.00% over the cases compared** (see `parity.json`; run with
`cd parity/express/nested/semver && GOWORK=off go test .`).

## How the two inventories were produced

Upstream, from the installed package (not from the README):

```
cd node && node -e 'const s=require("semver"); console.log(Object.keys(s).sort().join("\n"))'
```

That prints the 46 exported top-level symbols listed below. Class members were
enumerated the same way, from the real objects:

```
cd node && node -e 'const s=require("semver");
  console.log(Object.getOwnPropertyNames(s.SemVer.prototype).sort().join(" "));
  console.log(Object.getOwnPropertyNames(s.Range.prototype).sort().join(" "));
  console.log(Object.getOwnPropertyNames(s.Comparator.prototype).sort().join(" "))'
```

The Go side:

```
cd ../../../../express && GOWORK=off go doc ./semver && GOWORK=off go doc ./semver Version && GOWORK=off go doc ./semver Range
```

## Null mapping

node-semver reports "no answer" either by throwing or by returning `null`
(`valid`, `clean`, `coerce`, `parse`, `prerelease`, `inc`, `maxSatisfying`,
`minSatisfying`, `validRange`). The Go port has one channel for both: an error,
a `false` ok return, or a panic from the `Must*` helpers. Both runners therefore
report every one of those situations as `ok:false`, so the harness compares the
comparable fact — *whether* there is an answer, and which one. See the NULL
MAPPING note at the top of `node/run.js`.

## Top-level exports

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `semver.Comparator` | — | missing | — | the port's comparator type is unexported (`semComparator`); ranges are the only public entry point |
| `semver.RELEASE_TYPES` | — | missing | — | `["major","premajor",…]` is not exposed; `Inc` validates the level itself and errors on an unknown one (`i-bogus-level`) |
| `semver.Range` | `semver.Range` / `semver.ParseRange` | match | `rv-*` (24), `rt-*` (5) | validity verdict identical for every range in the suite; `Range#range` differs, see the class table |
| `semver.SEMVER_SPEC_VERSION` | — | missing | — | the `"2.0.0"` constant is not exposed |
| `semver.SemVer` | `semver.Version` | match | `p-*`, `cm-*`, `m-*` | the parsed-version type; see the class table for its members |
| `semver.clean` | `semver.Clean` | differs | `cl-*` (11) | same verdict and same normalisation, except that the port keeps build metadata — deviation `cl-build` |
| `semver.cmp` | — | missing | — | no operator-string dispatch (`cmp(a,'>',b)`); the six boolean helpers cover the same operators individually |
| `semver.coerce` | `semver.Coerce` | match | `co-*` (24) | including the 16/17-digit component rules and the leading-zero rejection |
| `semver.compare` | `semver.Compare` | match | `c-*` (18) | |
| `semver.compareBuild` | — | missing | `s-sort-build` | not exported, but its ordering is implemented internally (`semCompareBuild`) and is observable through `Sort`, which upstream also defines in terms of it |
| `semver.compareIdentifiers` | — | missing | — | internal (`semCompareIdentifier`); exercised indirectly by every prerelease comparison case |
| `semver.compareLoose` | — | missing | — | the port has no loose mode at all |
| `semver.diff` | — | missing | — | not ported |
| `semver.eq` | `semver.EQ` | match | `b-eq-build`, `b-eq-false`, `b-eq-invalid` | |
| `semver.gt` | `semver.GT` | match | `b-gt-true`, `b-gt-false`, `b-gt-pre` | |
| `semver.gte` | `semver.GTE` | match | `b-gte-eq`, `b-gte-false` | |
| `semver.gtr` | — | missing | — | not ported |
| `semver.inc` | `semver.Inc` / `semver.IncWith` | match | `i-*` (28), `id-*` (12) | every release level, with and without an identifier, including `inc("1.0.0-1","major")` vs `premajor` |
| `semver.intersects` | — | missing | — | not ported |
| `semver.lt` | `semver.LT` | match | `b-lt-true`, `b-lt-false` | |
| `semver.lte` | `semver.LTE` | match | `b-lte-eq`, `b-lte-false` | |
| `semver.ltr` | — | missing | — | not ported |
| `semver.major` | `semver.Major` | match | `a-major*` (4) | `uint64` vs JS number; MAX_SAFE_INTEGER round-trips exactly (`a-major-big`) |
| `semver.maxSatisfying` | `semver.MaxSatisfying` | match | `sel-max-*` (15) | returns the matching input element verbatim, as upstream does |
| `semver.minSatisfying` | `semver.MinSatisfying` | match | `sel-min-*` (7) | |
| `semver.minVersion` | — | missing | — | not ported |
| `semver.minor` | `semver.Minor` | match | `a-minor*` (3) | |
| `semver.neq` | `semver.NEQ` | match | `b-neq-true`, `b-neq-false` | |
| `semver.outside` | — | missing | — | not ported |
| `semver.parse` | `semver.Parse` | differs | `p-*` (11 via `versionString`) | same accept/reject verdict throughout; the rendered string keeps build metadata — deviation `p-pre-and-build` |
| `semver.patch` | `semver.Patch` | match | `a-patch*` (3) | |
| `semver.prerelease` | `semver.Prerelease` | differs | `a-pre-*` (6) | upstream returns `["a",1] \| null`, the port a dot-joined string; the runners join upstream's array, and "none" maps to a failure on both sides |
| `semver.rcompare` | — | differs | `rc-*` (3) | no `Rcompare` symbol; `-Compare(a,b)` is exactly equivalent and is what the cases run |
| `semver.rcompareIdentifiers` | — | missing | — | not ported |
| `semver.re` | — | missing | — | the port exposes no regexps (it is regexp-free apart from one internal operator-whitespace trim) |
| `semver.rsort` | — | missing | — | no descending sort; `Sort` then reverse |
| `semver.satisfies` | `semver.Satisfies` | match | `r-*` (71) | caret, tilde, `~>`, comparators, wildcards, hyphen ranges, `\|\|`, AND, prerelease rules, invalid ranges |
| `semver.simplifyRange` | — | missing | — | not ported |
| `semver.sort` | `semver.Sort` | match | `s-sort-*` (7) | in-place, input strings preserved, ties broken by build metadata like upstream's `compareBuild` |
| `semver.src` | — | missing | — | regexp source strings, N/A |
| `semver.subset` | — | missing | — | not ported |
| `semver.toComparators` | — | missing | — | not ported; the comparator expansion is internal |
| `semver.tokens` | — | missing | — | regexp token table, N/A |
| `semver.truncate` | — | missing | — | `truncate(version, releaseType)` is not ported |
| `semver.valid` | `semver.Valid` / `semver.Parse` | differs | `p-*` (33) | `Valid` returns a bool where upstream returns the string form or `null`; both spellings are tested (`isValid`, `valid`), and the string form keeps build metadata — deviations `p-build`, `p-build-only-dash`, `p-build-leadzero` |
| `semver.validRange` | `semver.ParseRange` | differs | `rv-*` (24) | identical validity verdict for all 24 ranges; upstream returns the normalised comparator text, the port a `*Range` (whose `String` differs — see below) |

## Class members the port maps onto

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `SemVer#compare` | `Version.Compare` | match | `cm-method-*` (3) | |
| `SemVer#compare` (as a predicate) | `Version.LessThan` / `GreaterThan` / `Equal` | match | `m-*` (6) | upstream has no boolean methods; the cases compare against `compare() <0/>0/===0` |
| `SemVer#inc` | `Version.IncMajor` / `IncMinor` / `IncPatch` | match | `im-*` (7) | |
| `SemVer#major` / `#minor` / `#patch` | `Version.Major` / `Minor` / `Patch` fields, `Version.Core` | match | `a-core-*` (3) | upstream has no "core" accessor; the cases compare against the three getters joined |
| `SemVer#version` / `#toString` | `Version.String` | differs | `p-build`, `p-pre-and-build`, … | upstream drops build metadata from `.version` and keeps the original text in `.raw`; the port has no `.raw`, so `String` keeps build metadata — **deviation** |
| `SemVer#prerelease` | `Version.Prerelease` field | match | `a-pre-*` | array of identifiers; numeric ones are numbers upstream, strings in Go |
| `SemVer#build` | `Version.Build` field | match | `p-build*` | |
| `SemVer#raw` / `#options` / `#loose` | — | missing | — | no loose mode and no original-text field |
| `SemVer#compareMain` / `#comparePre` / `#compareBuild` / `#format` | — | missing | — | internal in the port (`semComparePrerelease`, `semCompareBuild`) |
| `SemVer#satisfies` | — | missing | — | use `Range.Test` or `Satisfies` |
| `Range#test` | `Range.Test` | match | `rt-*` (5) | upstream's `test` catches an invalid version and answers false; `Range.Test` takes a `*Version`, so that state is unrepresentable and the runner reports false |
| `Range#range` | `Range.String` | differs | `rs-caret`, `rs-star` | upstream returns the normalised comparator set (`">=1.2.3 <2.0.0-0"`), the port the expression it parsed — **deviation** |
| `Range#set` | — | missing | — | the comparator sets are unexported |
| `Range#intersects` / `Range#format` | — | missing | — | not ported |
| `Comparator#test` / `#intersects` / `#semver` | — | missing | — | the type is unexported |

## Go-only symbols

| Go symbol | status | cases | note |
| --- | --- | --- | --- |
| `semver.ErrInvalidVersion` | extra | every failing case | sentinel for `errors.Is`; upstream throws `TypeError`/returns `null` |
| `semver.MustParse` | extra | `s-sort-invalid`, `c-invalid-left` | panicking `Parse`, the Go analogue of an uncaught throw; reachable from `Compare`/`Sort` — see `security.json` |
| `semver.Valid` | extra | `p-*` `isValid` cases | boolean form of upstream's string-or-null `valid` |
| `semver.IncWith` | extra | `id-*` (12) | upstream passes the identifier as a third argument to the same `inc` |
| `Version.Core` | extra | `a-core-*` (3) | |
| `Version.LessThan` / `GreaterThan` / `Equal` | extra | `m-*` (6) | |
| `Version.IncMajor` / `IncMinor` / `IncPatch` | extra | `im-*` (7) | |

## Counts

- Upstream top-level exports: **46**
  - `match`: 18 — `Range`, `SemVer`, `coerce`, `compare`, `eq`, `gt`, `gte`,
    `inc`, `lt`, `lte`, `major`, `maxSatisfying`, `minSatisfying`, `minor`,
    `neq`, `patch`, `satisfies`, `sort`
  - `differs`: 6 — `clean`, `parse`, `prerelease`, `rcompare`, `valid`,
    `validRange` (all tested; the difference is return shape, the spelling, or
    build-metadata rendering — never the accept/reject verdict or the ordering)
  - `missing`: 22 — `Comparator`, `RELEASE_TYPES`, `SEMVER_SPEC_VERSION`, `cmp`,
    `compareBuild`, `compareIdentifiers`, `compareLoose`, `diff`, `gtr`,
    `intersects`, `ltr`, `minVersion`, `outside`, `rcompareIdentifiers`, `re`,
    `rsort`, `simplifyRange`, `src`, `subset`, `toComparators`, `tokens`,
    `truncate` — not ported (the range algebra: `subset`, `intersects`,
    `simplifyRange`, `gtr`/`ltr`/`outside`, `minVersion`, `diff`, plus the
    regexp/constant tables and the loose-mode variants)
  - `untested`: 0 — every ported symbol has at least one case
- **Symbols compared: 24 of 46** (18 `match` + 6 `differs`). Counting a
  `differs` row as a pass where the verdict and the ordering are identical and
  only the shape or the build-metadata rendering differs, **parity over the
  compared surface is 24/24 = 100%**; counting `differs` strictly it is
  18/24 = 75.0%. Over the whole upstream export list, 24/46 = 52.2% is ported at
  all.
- **Cases: 320** across 7 groups (`parse` 44, `compare` 51, `accessors` 30,
  `coerce` 24, `inc` 47, `ranges` 102, `select` 22) — 313 compared, 313 match,
  0 mismatch, 7 deviations.

The `missing` rows are honest gaps, not measurement gaps: nothing in the port
answers `subset`, `intersects`, `simplifyRange`, `diff`, `minVersion`,
`gtr`/`ltr`/`outside`, `rsort`, `cmp` or `truncate`, so there is nothing to
compare and no case pretends otherwise.
