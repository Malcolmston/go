# `type-is` — API coverage

Nested harness: the oracle is the npm **`type-is`** package, not express.
Upstream pinned at `type-is@1.6.18` in [`node/package.json`](node/package.json);
Go port is `github.com/malcolmston/express/typeis` in the `express` submodule.

## How the upstream inventory was derived

```console
$ cd node && node -e "const t=require('type-is'); \
    console.log(typeof t); \
    console.log(Object.getOwnPropertyNames(t).join(', '))"
function
length, name, prototype, is, hasBody, normalize, match
```

`length`, `name` and `prototype` are intrinsic function properties, not API. The
Go side is `GOWORK=off go doc -short ./typeis` in the submodule:

```
func Is(contentType string, types ...string) (string, bool)
func Match(expected, actual string) bool
func Normalize(t string) (string, bool)
func NormalizeType(value string) string
```

## Symbol inventory

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `typeis.is(value, types)` | `typeis.Is(contentType, types...)` | match | 30 in `is` | candidate order, wildcards, `+suffix`, extension shorthands, specials |
| `typeis.is(value)` (no types) | `typeis.NormalizeType(value)` | match | 30 in `normalize-type` | with no candidates upstream returns the normalised type, which is what `NormalizeType` exposes as its own function |
| `typeis.normalize(type)` | `typeis.Normalize(t)` | match | 24 in `normalize` | |
| `typeis.match(expected, actual)` | `typeis.Match(expected, actual)` | match | 21 in `match` | wildcards and the `*+suffix` length rule |
| `typeis(req, types)` (the module callable) | — | missing | — | takes a live request: it checks `hasBody(req)` first, returns `null` for a bodyless request and otherwise defers to `typeis.is(req.headers['content-type'], types)`. The deferred part is fully covered by `Is`; the request plumbing belongs to the framework |
| `typeis.hasBody(req)` | — | missing | — | inspects `transfer-encoding` / `content-length` on a live request object |

## Representation mapping

Upstream returns `false` for "no match / not a media type" and the Go port
returns `("", false)`; both runners emit JSON `null`. Applied symmetrically.

## Counts

| status | symbols |
| --- | --- |
| match | 4 |
| differs | 0 |
| missing | 2 |
| extra | 0 |
| untested | 0 |

Parity over the 4 symbols actually compared: **4/4 = 100%**. The two missing
symbols are both request-object helpers with no pure-function equivalent.

**Cases: 105. Measured parity: 105/105 = 100.0%** (see
[`parity.json`](parity.json)), up from 91/105 = 86.7%.

## What the harness fixed in the port

- **`NormalizeType` was far too lenient.** It split at the first `;` and threw
  the parameters away without looking at them, so `text/html;`,
  `application/json;;`, `text/html; charset` (no value), `text/html; char
  set=utf-8` and `text/html; charset=utf-8 junk` all normalised happily to a
  valid type. Upstream runs the value through `media-typer`'s `parse`, which
  validates the *entire* parameter list before `format` discards it, so all five
  are errors and match nothing. `NormalizeType` is now a port of
  `media-typer@0.3.0`'s parse+format pair, including:
  - only spaces (not tabs) padding the type — `\tapplication/json` is invalid;
  - the `{0,126}`-bounded RFC 6838 name grammar for the type and subtype;
  - splitting the subtype at its **last** `+` and re-validating each half, so
    `image/svg+xml` round-trips, `text/html++x` is rejected, and `text/html+`
    normalises to `text/html` because an empty suffix is not re-emitted.
- **the extension table.** `normalize()` resolved bare extensions through a
  12-entry hand-written map, so `csv`, `svg` and `pdf` were unknown. It now goes
  through the sibling `mimetypes` package, which is what upstream's `mime.lookup`
  does.
