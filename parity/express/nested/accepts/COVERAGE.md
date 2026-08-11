# `accepts` — API coverage

Nested harness: the oracle is the npm **`accepts`** package, not express.
Upstream pinned at `accepts@1.3.8` in [`node/package.json`](node/package.json);
Go port is `github.com/malcolmston/express/accepts` in the `express` submodule.

## How the upstream inventory was derived

`accepts` exports a constructor whose whole API lives on its prototype, so the
inventory is the prototype's own property names, read off the installed package:

```console
$ cd node && node -e "const a=require('accepts'); \
    console.log(typeof a); \
    console.log(Object.getOwnPropertyNames(a).join(', ')); \
    console.log(Object.getOwnPropertyNames(a.prototype).join(', '))"
function
length, name, prototype
constructor, types, type, encodings, encoding, charsets, charset, languages, language, langs, lang
```

The Go side is `GOWORK=off go doc -short ./accepts` plus `go doc ./accepts.Accepts`
in the submodule.

## Symbol inventory

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `accepts(req)` (callable/constructor) | `accepts.New(http.Header)` | match | every case | upstream takes a request-like object and reads `.headers`; the port takes the `http.Header` directly |
| `Accepts#type(...types)` | `Accepts.Type(offers...)` | match | 36 in `media-types` | including the extension shorthands, the `!headers.accept` short-circuit, and unknown extensions being dropped rather than guessed |
| `Accepts#types()` (no args) | `Accepts.Types()` | match | `type-no-offers-is-types`, `types-*` (8) | reports the header's own media types |
| `Accepts#types(...types)` | `Accepts.Types(offers...)` | differs | via `type-*` | upstream's plural name is an alias of the singular function and returns **one** value when given offers; the Go plural returns the whole ranked list. `Types(o...)[0] == accepts.types(...o)`, which is what `Type` is defined as and what the cases compare |
| `Accepts#language(...langs)` | `Accepts.Language(offers...)` | match | 21 in `languages` | |
| `Accepts#languages()` (no args) | `Accepts.Languages()` | match | `langs-*` (5) | |
| `Accepts#languages(...langs)` | `Accepts.Languages(offers...)` | differs | via `lang-*` | same singular/plural overload as `types` |
| `Accepts#lang` (alias of `language`) | — | missing | — | the port exposes one spelling; nothing to test that `Language` does not already cover |
| `Accepts#langs` (alias of `languages`) | — | missing | — | as above |
| `Accepts#charset(...charsets)` | `Accepts.Charset(offers...)` | match | 14 in `charsets` | |
| `Accepts#charsets()` (no args) | `Accepts.Charsets()` | match | `charsets-*` (5) | |
| `Accepts#charsets(...charsets)` | `Accepts.Charsets(offers...)` | differs | via `charset-*` | singular/plural overload |
| `Accepts#encoding(...encodings)` | `Accepts.Encoding(offers...)` | match | 21 in `encodings` | including the implicit `identity` |
| `Accepts#encodings()` (no args) | `Accepts.Encodings()` | match | `encs-*` (7) | |
| `Accepts#encodings(...encodings)` | `Accepts.Encodings(offers...)` | differs | via `enc-*` | singular/plural overload |
| `Accepts#constructor` | `accepts.New` | match | every case | |

## Representation mapping

Two shape differences are folded by the runners rather than being scored as
mismatches, and both are applied symmetrically:

- upstream returns `false` for "nothing acceptable"; the Go port returns `""`.
  Both runners emit JSON `null`.
- an absent header is expressed by omitting the key from a case's headers
  object, so `http.Header.Values` reports it absent — which the port
  distinguishes from a present-but-empty header, as upstream does.

## Counts

| status | symbols |
| --- | --- |
| match | 11 |
| differs | 4 |
| missing | 2 |
| extra | 0 |
| untested | 0 |

Parity over the 15 symbols actually compared: **11 match, 4 differ → 73.3% of
symbols exact, 100% behaviourally reachable** (each `differs` row is a
superset: taking the first element of the Go plural result reproduces the
upstream function exactly, and the cases verify that).

**Cases: 117. Measured parity: 117/117 = 100.0%** (see
[`parity.json`](parity.json)), up from 96/117 = 82.1% before this harness
existed.

## What the harness fixed in the port

The port previously reimplemented negotiation itself, with its own header
parser and a 14-entry MIME shorthand table. It is now a thin wrapper over the
sibling `negotiator` package plus `mimetypes.Lookup`, exactly as upstream wraps
`negotiator` plus `mime-types`. That closed 21 mismatches at once, among them:

- q-value tie-breaking (`type-first-of-two`): the client's header order decides,
  not the offer order.
- unknown-extension offers (`type-ext-unknown`, `type-ext-urlencoded`): upstream
  drops them, the port used to invent `application/<name>`.
- extensions outside the old shorthand table (`type-ext-csv`, `-svg`, `-woff2`).
- media-type parameters in the header or the offer (`type-params-in-*`).
- malformed q-values (`type-malformed-q`, `lang-malformed-q`, …): a `q` that is
  not a number makes the entry unacceptable, it does not default to 1.
- present-but-empty headers, malformed entries, and language prefix matching.
