# `negotiator` — API coverage

Nested harness: the oracle is the npm **`negotiator`** package, not express.
Upstream pinned at `negotiator@0.6.3` in [`node/package.json`](node/package.json);
Go port is `github.com/malcolmston/express/negotiator` in the `express` submodule.

## How the upstream inventory was derived

```console
$ cd node && node -e "const N=require('negotiator'); \
    console.log(typeof N); \
    console.log(Object.getOwnPropertyNames(N).join(', ')); \
    console.log(Object.getOwnPropertyNames(N.prototype).join(', '))"
function
length, name, prototype, Negotiator
constructor, charset, charsets, encoding, encodings, language, languages, mediaType, mediaTypes, preferredCharset, preferredCharsets, preferredEncoding, preferredEncodings, preferredLanguage, preferredLanguages, preferredMediaType, preferredMediaTypes
```

The four `lib/*.js` modules (`charset`, `encoding`, `language`, `mediaType`) are
not part of the public surface — `require('negotiator/lib/mediaType')` is not an
exported entry point — so they are covered through the prototype methods that
call them.

The Go side is `GOWORK=off go doc -short ./negotiator` plus
`go doc ./negotiator.Negotiator` in the submodule.

## Symbol inventory

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Negotiator(req)` / `new Negotiator(req)` | `negotiator.New(http.Header)` | match | every case | upstream takes a request-like object and reads `.headers` |
| `Negotiator.Negotiator` (self-reference) | — | missing | — | a CommonJS interop convenience with no Go analogue |
| `Negotiator#mediaTypes(available?)` | `Negotiator.MediaTypes(available...)` | match | 26 in `media-types` | |
| `Negotiator#mediaType(available?)` | `Negotiator.MediaType(available...)` | match | 26 in `media-types` | |
| `Negotiator#languages(available?)` | `Negotiator.Languages(available...)` | match | 14 in `languages` | |
| `Negotiator#language(available?)` | `Negotiator.Language(available...)` | match | 17 in `languages` | |
| `Negotiator#charsets(available?)` | `Negotiator.Charsets(available...)` | match | 12 in `charsets` | |
| `Negotiator#charset(available?)` | `Negotiator.Charset(available...)` | match | 11 in `charsets` | |
| `Negotiator#encodings(available?)` | `Negotiator.Encodings(available...)` | match | 16 in `encodings` | |
| `Negotiator#encoding(available?)` | `Negotiator.Encoding(available...)` | match | 18 in `encodings` | |
| `Negotiator#preferredMediaType(s)` | `Negotiator.MediaType(s)` | match | `mt-alias-preferred` | upstream keeps these as backwards-compatibility aliases of the same functions; the port has one spelling, and the runner routes the alias to it |
| `Negotiator#preferredLanguage(s)` | `Negotiator.Language(s)` | match | `lang-alias-preferred` | |
| `Negotiator#preferredCharset(s)` | `Negotiator.Charset(s)` | match | `cs-alias-preferred` | |
| `Negotiator#preferredEncoding(s)` | `Negotiator.Encoding(s)` | match | `enc-alias-preferred` | |
| `Negotiator#constructor` | `negotiator.New` | match | every case | |

## Representation mapping

- the singular accessors return `undefined` upstream and `""` in Go when nothing
  is acceptable; both runners emit JSON `null`.
- a case's `available` argument is `null` to mean "call the method with no
  argument". An **empty array** is deliberately never used as a case input:
  JavaScript treats `[]` as truthy, so `mediaTypes([])` returns `[]` while
  `mediaTypes()` returns the whole header — a distinction a variadic Go
  signature cannot express. That single input shape is the only untested corner
  of the surface.

## Counts

| status | symbols |
| --- | --- |
| match | 13 |
| differs | 0 |
| missing | 1 |
| extra | 0 |
| untested | 0 |

Parity over the 13 symbols actually compared: **13/13 = 100%**.

**Cases: 142. Measured parity: 142/142 = 100.0%** (see
[`parity.json`](parity.json)), up from 126/142 = 88.7% before the port was
resynchronised.

## What the harness fixed in the port

The port was rewritten to mirror `lib/*.js` structurally rather than
approximating it. The 16 closed mismatches were:

- **absent vs present-but-empty headers.** Upstream passes
  `accept === undefined ? '*/*' : accept || ''`, so an absent `Accept` means
  `*/*` but `Accept:` (empty) accepts *nothing*. The port collapsed both to
  `*/*`, i.e. it accepted content the client had refused
  (`mt-all-empty-header`, `cs-all-empty-header`, `lang-all-empty-header`,
  `mt-pick-empty-header`, …).
- **unparseable q-values.** `parseFloat('abc')` is `NaN` and `NaN > 0` is false,
  so `text/html;q=abc` is unacceptable upstream; the port defaulted to `q=1`
  (`mt-all-malformed-q`, `mt-all-empty-q`, `mt-all-bare-q`, `cs-all-malformed-q`,
  `lang-all-malformed-q`, `enc-all-malformed-q`). The port now models NaN
  qualities and JavaScript's `||`-chained comparators exactly, including the way
  a NaN comparison falls through to the next tiebreak.
- **the media-type / token / language grammars.** `text/html extra` and
  `text /html` are not media types (`mt-all-internal-space`,
  `mt-all-space-before-slash`) and `gzip deflate` is not an encoding
  (`enc-all-internal-space`); the port used to accept all three by trimming.
- **wildcard parameter values.** `Accept: text/html;level=*` matches any `level`
  (`mt-pick-param-wildcard-value`).
- **parameter specificity.** Parameters contribute a single bit to the
  specificity score, not one point each (`mt-pick-param-two`).
- **the implicit `identity` encoding** now carries the lowest quality seen in the
  header, as `Math.min(minQuality, encoding.q || 1)` produces, instead of a
  hard-coded epsilon.
