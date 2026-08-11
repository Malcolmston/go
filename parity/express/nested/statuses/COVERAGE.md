# `statuses` — API coverage

Nested harness: the oracle is the npm **`statuses`** package, not express.
Upstream pinned at `statuses@2.0.1` in [`node/package.json`](node/package.json);
Go port is `github.com/malcolmston/express/statuses` in the `express` submodule.

## How the upstream inventory was derived

```console
$ cd node && node -e "const s=require('statuses'); \
    console.log(typeof s); console.log(Object.getOwnPropertyNames(s).join(', '))"
function
length, name, prototype, message, code, codes, redirect, empty, retry
```

(`length`, `name` and `prototype` are intrinsic function properties, not API.)

The status table itself was diffed against the port's mechanically:

```console
$ cd node && node -e "const s=require('statuses'); \
    console.log(s.codes.length); \
    console.log(JSON.stringify(s.codes.map(c=>[c,s.message[c]])))"
63
...
```

The Go side is `GOWORK=off go doc -short ./statuses` in the submodule:

```
func Code(message string) (int, error)
func Codes() []int
func IsEmpty(code int) bool
func IsRedirect(code int) bool
func IsRetry(code int) bool
func Message(code int) string
```

## Symbol inventory

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `statuses.message` (code → phrase) | `statuses.Message(code)` | match | 68 in `messages` | all 63 registered codes plus 5 unregistered ones |
| `statuses.code` (phrase → code) | `statuses.Code(message)` | match | 39 in `codes` | case-insensitive, untrimmed, misses included |
| `statuses.codes` | `statuses.Codes()` | match | `codes-all` | 63 codes; sorted ascending on both sides |
| `statuses.redirect` | `statuses.IsRedirect(code)` | match | 12 `redirect-*` | including 304 (not a redirect) and 306 (withdrawn) |
| `statuses.empty` | `statuses.IsEmpty(code)` | match | 7 `empty-*` | 204, 205, 304 |
| `statuses.retry` | `statuses.IsRetry(code)` | match | 8 `retry-*` | 502, 503, 504 |
| `statuses(codeOrMessage)` (the callable) | `Message` / `Code` | differs | 7 in `callable` | Go has no polymorphic callable, so the runner dispatches on the JSON argument type. 6 of the 7 cases match; `status-numeric-string` is a declared deviation (below) |

Where upstream throws for an unknown code or phrase, the Go port reports its
zero value (`""` from `Message`, a non-nil error from `Code`). The runners map
`Message`'s `""` sentinel onto the same `ok:false` reply the thrown `TypeError`
produces, so "unknown" is compared as "unknown" rather than as the value `""`.

## Declared deviation

One case is marked `"deviation"` and is scored separately from both a match and a
bug, per [`../../../HARNESS.md`](../../../HARNESS.md). It is listed in the
express submodule's `API-DEVIATIONS.md`.

| case | upstream | port | why |
| --- | --- | --- | --- |
| `status-numeric-string` | `statuses('404')` → `"Not Found"` | `Code("404")` → error | upstream's callable `parseInt`s a string argument and, if it is numeric, treats it as a *code* instead of a reason phrase. That coercion is a JavaScript artefact of overloading one callable on argument type; Go splits the operation into `Message(int)` and `Code(string)`, and `"404"` is simply not a registered reason phrase. Upstream is the outlier here — no HTTP status is named "404" |

## Counts

| status | symbols |
| --- | --- |
| match | 6 |
| differs | 1 |
| missing | 0 |
| extra | 0 |
| untested | 0 |

Parity over the 7 symbols actually compared: **6/7 exact = 85.7%**, with the one
`differs` row being the polymorphic callable that a statically typed API cannot
have.

**Cases: 142 (141 compared, 1 declared deviation). Measured parity:
141/141 = 100.0%** (see [`parity.json`](parity.json)).

## What the harness fixed in the port

The data tables were already an exact match for `statuses@2.0.1`: the same 63
codes, the same phrases (including `"I'm a Teapot"` and `"Unprocessable
Entity"`, which differ between major versions), and the same three
classification sets. One behavioural fix was needed:

- `Code` trimmed surrounding whitespace before looking a phrase up, so
  `Code(" Not Found ")` returned 404 where upstream reports a miss. Upstream
  indexes its table by the raw lower-cased phrase and nothing else. The trim was
  removed — it made the port accept input upstream rejects, which is a divergence
  regardless of how convenient it reads. `case code-padded` pins the corrected
  behaviour.
