# `express/escaperegexp` vs npm `escape-string-regexp`

Oracle: **escape-string-regexp@5.0.0** (pinned in `node/package.json`).
Port: `github.com/malcolmston/express/escaperegexp`.
Harness: `GOWORK=off go test .` in this directory, after `npm install` in `node/`.

`escape-string-regexp` 5.x is pure ESM, so `node/package.json` sets
`"type": "module"` and `node/run.js` is an ES module.

## How the upstream inventory was derived

```
$ cd node && node --input-type=module -e "import * as m from 'escape-string-regexp';
  console.log(JSON.stringify(Object.keys(m)));
  console.log(typeof m.default, m.default.name, m.default.length)"
["default"]
function escapeStringRegexp 1
```

One default export, no named exports, no properties on the function. The Go side
was enumerated with `GOWORK=off go doc -all ./escaperegexp` in the `express`
submodule.

## Symbol inventory

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `escapeStringRegexp(string)` (default export) | `escaperegexp.EscapeRegExp(string) string` | match | all 61 | Same metacharacter set, same `\x2d` treatment of the hyphen, same pass-through for everything else. |
| — | — | — | — | Upstream exports nothing else. |

### Counts

| status | symbols |
| --- | --- |
| match | 1 |
| differs | 0 |
| missing | 0 |
| extra | 0 |
| untested | 0 |

**Parity: 1/1 compared symbols (100%).**
**Cases: 61 total, 61 match, 0 mismatch, 0 declared deviations — 100% of compared cases.**

## Behaviour verified by the cases

`cases/escape.json` (group `escape`, 46 cases) compares the output bytes;
`cases/roundtrip.json` (group `roundtrip`, 15 cases) compiles the escaped needle
on both sides and compares what it actually matches, so the *purpose* of the
function is measured and not only its spelling.

* Every character in upstream's `[|\\{}()[\]^$+*?.]` class, one case each, plus
  the whole set in one string (`meta-pipe` … `meta-dot`, `meta-all`).
* The hyphen, which upstream renders as the hexadecimal escape `\x2d` rather than
  `\-` so that it is safe both inside and outside a character class
  (`meta-hyphen`, `hyphen-run`, `hyphen-in-class-shape`, `lone-hyphen-emoji`,
  `unicode-dash` — only ASCII HYPHEN-MINUS is rewritten).
* The characters deliberately left alone: `/` (`not-escaped-slash`) and the rest
  of ASCII punctuation (`not-escaped-punct`).
* Double escaping: an input that is already an escape sequence
  (`already-escaped`, `backslash-x2d` — a literal `\x2d` in the input must not be
  confused with the hyphen's replacement).
* The interpolation attacks the function exists to stop, each confirmed inert by a
  round-trip case as well as by its output: closing a character class early
  (`class-breakout`, `rt-class`), closing a group and starting an alternation
  (`group-breakout`, `rt-group-alternation`), anchors (`anchors`, `rt-anchors`),
  quantifiers (`quantifier-braces`, `rt-quantifier-braces`, `rt-plus`),
  lookahead, named groups, backreferences and class shorthands (`lookahead`,
  `named-group`, `backref`, `class-shorthand`, `rt-backslash`) and the classic
  catastrophic-backtracking pattern (`redos-nested-quantifier`).
* `rt-no-match` is the load-bearing behavioural case: the escaped `.*` must match
  **nothing** in `"anything at all"`, which is only true if it was neutralised.
* Non-ASCII and astral input, where upstream iterates UTF-16 code units and the
  port iterates runes (`non-ascii-latin`, `non-ascii-cjk`, `emoji`, `rt-emoji`,
  `rt-non-ascii`).
* Whitespace and NUL pass through unescaped (`whitespace`, `nul`).

## Error cases

Upstream throws `TypeError('Expected a string')` for a non-string argument. The Go
function takes a typed `string`, so the Go runner rejects the argument shape,
which is the same observable outcome — `ok:false` on both sides. Six cases cover
it: `err-number`, `err-null`, `err-array`, `err-object`, `err-bool`,
`err-missing-arg`, plus `rt-err-nonstring-needle`.

Nothing is recorded in `API-DEVIATIONS.md` for this package.
