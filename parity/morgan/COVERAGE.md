# morgan — upstream API inventory vs the Go port

| | |
| --- | --- |
| upstream oracle | `morgan@1.10.0` (driven through `express@4.21.2`) |
| Go module under test | `github.com/malcolmston/morgan v1.2.0` (published module, no `replace`) |
| cases | 98 |
| compared | 98 (`match` + `mismatch`) |
| match | 66 |
| mismatch | 32 |
| deliberate deviations | 0 |
| **parity** | **67.35 %** |

Regenerate with:

```sh
cd parity/morgan/node && npm install     # once, installs the pinned oracle
cd ..                 && GOWORK=off go test ./...
```

`parity.json` is rewritten by `TestParity` on every full run. (Running with
`-run` filters shrinks it to the filtered subset; always regenerate with a full
run.)

## How a case is compared

morgan produces one access-log line per request, so the comparable artefact is
**the log line itself**. Each case is a request spec (method, path, headers,
body) + a response spec (status, headers, body, how Content-Length is produced)
+ a format name or raw format string. Both runners mount the library on a real
in-process HTTP server on `127.0.0.1`, perform that exact exchange once, and
emit the resulting log line as the case `value`; a suppressed line (`skip`) is
reported as the empty string.

* upstream runner: `node/run.js` — `express@4.21.2` + `morgan@1.10.0`, log
  stream captured in memory. Express is used (rather than a bare `http` server)
  because `res.send()` is what makes `res.getHeader('content-length')`
  observable, which is exactly the asymmetry the port has to cope with.
* Go runner: `go/run.go` — `net/http` + `morgan.New(...)`, `Config.Stream`
  captured in memory.

The Go handler is the idiomatic equivalent of the express one: same status, same
response headers, same body bytes.

### What is normalised (and nothing else)

Response time and wall-clock date are inherently volatile. Both runners apply
the **same** substitutions to the finished line before emitting it
(`normalise()` in `node/run.js` and `go/run.go` — the two regex sets are kept
byte-identical):

| pattern | replacement | why |
| --- | --- | --- |
| `\d{2}/[A-Z][a-z]{2}/\d{4}:\d{2}:\d{2}:\d{2} [+-]\d{4}` | `<DATE-CLF>` | `:date[clf]` output |
| `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z` | `<DATE-ISO>` | `:date[iso]` output |
| `[A-Z][a-z]{2}, \d{2} [A-Z][a-z]{2} \d{4} \d{2}:\d{2}:\d{2} GMT` | `<DATE-WEB>` | `:date[web]` / `:date` output |
| `(\d+)\.(\d+) ms` | `<T:n> ms`, `n` = decimal digit count | `:response-time` / `:total-time` |
| `(\d+) ms` | `<T:0> ms` | same tokens with 0 decimals |
| `"(responseTime\|totalTime)":<number>` | `"…":<T:n>` | the port-only `json` format |
| `127\.0\.0\.1:\d+` | `127.0.0.1:<PORT>` | ephemeral listen port (`:host`) |
| `PID=\d+` | `PID=<PID>` | the port-only `:pid` token |

The digit **count** survives normalisation on purpose, so a divergence in how
the `:response-time[digits]` argument is interpreted is still caught (it is —
see `tok-response-time-digits-invalid`). Everything else — method, url, status,
referrer, user-agent, remote-addr, remote-user, http-version, content-length,
literal text, ANSI colour codes — is compared byte for byte.

## Where the inventory comes from

Upstream tokens and formats, enumerated from the installed package (tokens and
formats are both stored as own properties of the exported `morgan` function):

```sh
cd parity/morgan/node && node -e "
const morgan=require('morgan');
const keys=Object.keys(morgan).sort();
console.log('tokens :', keys.filter(k=>typeof morgan[k]==='function' && !['format','token','compile'].includes(k)));
console.log('formats:', keys.filter(k=>typeof morgan[k]==='string'));"
```

```
tokens : [ 'date','dev','http-version','method','referrer','remote-addr',
           'remote-user','req','res','response-time','status','total-time',
           'url','user-agent' ]              # 'dev' is a format function, not a token
formats: [ 'combined','common','default','short','tiny' ]   # + 'dev' (function)
module.exports = morgan; .compile; .format; .token
```

The port's registered tokens and formats, enumerated from the published module
source:

```sh
D=$(GOWORK=off go env GOMODCACHE)/github.com/malcolmston/morgan@v1.2.0
ls $D/*.go | grep -v _test.go | xargs grep -ho 'Token("[a-z-]*"' | sort -u
ls $D/*.go | grep -v _test.go | xargs grep -ho 'RegisterFormat\(Func\)\?("[a-z]*"' | sort -u
```

```
tokens : date host http-version incoming method path pid protocol query
         referrer remote-addr remote-user req res response-time status
         total-time url user-agent          # 19 ("type" in the list is a doc example)
formats: combined common short tiny dev json
```

The port's exported Go API:

```sh
cd parity/morgan && GOWORK=off go doc -all github.com/malcolmston/morgan
```

## Tokens

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `:method` | `:method` | match | `tok-method-get`, `tok-method-delete` | |
| `:url` | `:url` | differs | `tok-url-with-query`, `tok-url-root`, `bug-url-percent-decoded`, `bug-url-percent-encoded-space`, `urlencoded-path-combined` | port builds the value from `r.URL.Path`, which `net/http` has already percent-decoded; upstream logs `req.originalUrl` verbatim. Plain ASCII paths match. |
| `:status` | `:status` | match | `tok-status-200`, `tok-status-404`, `tok-status-503`, `immediate-status-unavailable` | including `-` when headers have not been sent |
| `:http-version` | `:http-version` | match | `tok-http-version` | |
| `:referrer` | `:referrer` | match | `tok-referrer-present`, `tok-referrer-misspelled-header`, `tok-referrer-missing` | both accept `Referer` and `Referrer` |
| `:user-agent` | `:user-agent` | match | `tok-user-agent-present`, `tok-user-agent-missing` | |
| `:remote-addr` | `:remote-addr` | differs | `tok-remote-addr`, `xff-single-entry-remote-addr`, `xff-multi-entry-remote-addr`, `xff-combined`, `bug-xff-trusted-unconditionally`, `bug-xff-list-renders-nil`, `bug-xff-non-ip-value` | matches on a direct connection; the port trusts `X-Forwarded-For` unconditionally (upstream needs express `trust proxy`), and a header that is not a bare IP literal renders as `<nil>` |
| `:remote-user` | `:remote-user` | match | `tok-remote-user-basic-auth`, `tok-remote-user-missing`, `tok-remote-user-no-password`, `basic-auth-combined` | |
| `:date`, `:date[clf\|iso\|web]` | `:date`, `:date[…]` | differs | `tok-date-default-arg`, `tok-date-clf`, `tok-date-iso`, `tok-date-web`, `tok-date-unknown-arg` | all three documented arguments and the no-argument default match; an *unknown* argument yields `-` upstream (the `switch` has no default) but falls back to the web format in the port |
| `:response-time`, `:response-time[digits]` | same | differs | `tok-response-time`, `tok-response-time-digits-5`, `tok-response-time-digits-0`, `tok-response-time-digits-invalid`, `immediate-response-time-unavailable` | digit counts match for numeric arguments; a non-numeric argument means 0 decimals upstream (`toFixed`) but 3 in the port, and in `immediate` mode upstream renders `-` while the port renders `0.000` |
| `:total-time`, `:total-time[digits]` | same | match | `tok-total-time`, `tok-total-time-digits-1`, `fmt-raw-rich` | |
| `:req[header]` | `:req[header]` | match | `tok-req-header`, `tok-req-header-missing`, `tok-req-header-repeated`, `tok-req-header-content-length` | repeated headers join with `", "` on both sides |
| `:res[header]` | `:res[header]` | differs | `tok-res-header`, `tok-res-header-missing`, `tok-res-content-length-explicit`, `tok-res-content-length-chunked`, `bug-content-length-*`, `immediate-content-length-unavailable` | arbitrary headers match. `:res[content-length]` matches only when the handler sets `Content-Length` itself; when the framework computes it (express `res.send`) the port has nothing to read and prints `-`, so **combined/common/short/tiny/dev all lose the response size** |
| — | `:pid` | extra | `tok-pid-port-only` | port-only; upstream throws `TypeError` on the unknown token |
| — | `:incoming` | extra | `tok-incoming-port-only` | port-only (request `Content-Length`) |
| — | `:protocol` | extra | `tok-protocol-port-only` | port-only |
| — | `:host` | extra | `tok-host-port-only` | port-only |
| — | `:path` | extra | `tok-path-port-only` | port-only (path without query) |
| — | `:query` | extra | `tok-query-port-only` | port-only (raw query string) |
| unknown token → `TypeError` at render time | unknown token → `-` | differs | `tok-unknown-name` | upstream fails the request's log line loudly; the port silently substitutes `-` |

## Formats

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `morgan('combined')` | `morgan.Combined` | differs | `fmt-combined`, `bug-content-length-combined`, `status-404-combined`, `missing-referrer-and-user-agent-combined`, `basic-auth-combined`, `immediate-combined`, `urlencoded-path-combined`, `xff-combined` | identical format string; diverges only through `:res[content-length]`, `:url` and `:remote-addr` |
| `morgan('common')` | `morgan.Common` | differs | `fmt-common`, `post-with-body-common`, `bug-content-length-common` | same, via `:res[content-length]` |
| `morgan('short')` | `morgan.Short` | differs | `fmt-short`, `status-500-short`, `bug-content-length-short` | same, via `:res[content-length]` |
| `morgan('tiny')` | `morgan.Tiny` | differs | `fmt-tiny`, `head-request-tiny`, `bug-content-length-tiny` | same, via `:res[content-length]` |
| `morgan('dev')` | `morgan.Dev` | differs | `fmt-dev-2xx`, `fmt-dev-3xx`, `fmt-dev-4xx`, `fmt-dev-5xx`, `bug-content-length-dev` | the port drops **all ANSI colouring** whenever the stream is not a terminal; upstream always emits `ESC[0m … ESC[<colour>m<status>ESC[0m …`. Layout, status classes and colour selection logic otherwise agree |
| `morgan('default')` (deprecated) | — | missing | `fmt-default-deprecated` | the port has no `default` name, so `"default"` is compiled as a literal format string |
| — | `morgan.JSON` | extra | `fmt-json-port-only`, `bug-json-format-invalid-json` | port-only format, and its output is **not valid JSON**: `contentLength` is interpolated as a bare `-` when unknown (`"contentLength":-`) |
| empty format name | — | differs | `fmt-raw-empty-string` | upstream falls back to `morgan.default`; the port compiles the empty string and therefore writes no line at all |
| `morgan.compile(fmt)` (raw `:token` / `:token[arg]` strings) | `morgan.Compile` | match | `fmt-raw-minimal`, `fmt-raw-rich`, `fmt-raw-literal-only`, `fmt-raw-adjacent-tokens`, `fmt-raw-short-name-not-a-token` | same token grammar, including the `>= 2 name characters` rule and `-` for empty values |
| `morgan.format(name, string)` | `morgan.RegisterFormat` | match | `fmt-registered-name` | |
| `morgan.format(name, fn)` | `morgan.RegisterFormatFunc` | match | `fmt-registered-func` | |
| `morgan.token(name, fn)` | `morgan.Token` | match | `ctok-const`, `ctok-from-request`, `ctok-with-arg`, `ctok-with-arg-missing`, `ctok-empty-string` | including the `:name[arg]` form and `-` for an empty return |

## Middleware and options

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `morgan(format, options)` | `morgan.New(next, format, cfg)` | match | all 98 | the middleware itself: one line per request, written after the response finishes |
| `options.stream` | `morgan.Config.Stream` | match | all 98 | every case captures the line through this option |
| `options.immediate` | `morgan.Config.Immediate` | differs | `immediate-request-fields`, `immediate-status-unavailable`, `immediate-content-length-unavailable`, `immediate-combined`, `immediate-response-time-unavailable` | request-only tokens, `:status` and `:res[…]` agree (all `-`); `:response-time` does not (`-` upstream vs `0.000`) |
| `options.skip` | `morgan.Config.Skip` | match | `skip-none`, `skip-status-below-400-suppresses-200`, `skip-status-below-400-logs-404`, `skip-status-below-400-logs-500`, `skip-path-healthz-suppresses`, `skip-path-healthz-logs-other` | exercised through `morgan.SkipStatusBelow` and `morgan.SkipPaths` |
| `options.buffer` (deprecated) | `morgan.Config.Buffer` | untested | — | upstream deprecates it and it is interval-based, so it cannot be compared deterministically per request |
| `morgan(options)` object form (deprecated) | — | missing | — | the port has no options-only constructor |
| — | `morgan.Config.TrustProxy` | missing in v1.2.0 | — | `Config` in v1.2.0 has no such field; `X-Forwarded-For` is always trusted. Upstream's equivalent is express `trust proxy`, which is off by default |

## Go-only exported API

Mechanically listed with `go doc -all github.com/malcolmston/morgan`. These have
no upstream counterpart (upstream exposes only the middleware factory plus
`compile`/`format`/`token`), so they are `extra`.

| Go symbol | status | cases | note |
| --- | --- | --- | --- |
| `morgan.Format`, `morgan.Config`, `morgan.Log`, `morgan.FormatFunc`, `morgan.TokenFunc`, `morgan.SkipFunc`, `morgan.IP` | extra | — (types used by every case) | the port is typed; upstream passes loose `req`/`res` objects |
| `morgan.FromRequest` | extra | — | used internally by `New`; no upstream analogue |
| `morgan.Compile` | (see Formats) | see above | |
| `morgan.SkipStatusBelow` | extra | `skip-status-below-400-*` | Go form of morgan's documented "only log errors" example |
| `morgan.SkipPaths` | extra | `skip-path-healthz-*` | |
| `morgan.SkipStatusBetween` | extra, untested | — | |
| `morgan.SkipUserAgents` | extra, untested | — | |
| `morgan.CombineSkips` | extra, untested | — | |
| `morgan.Clfdate` | extra | — (covered indirectly by `tok-date-clf`) | exported helper behind `:date[clf]` |
| `morgan.FormatDuration` | extra | — (covered indirectly by `tok-response-time*`) | exported helper behind `:response-time` |
| `morgan.ClientIP` | extra | — (covered indirectly by `:remote-addr` cases) | |
| `morgan.RequestURL` | extra | — (covered indirectly by `:url` cases) | |
| `morgan.RequestProtocol` | extra | — (covered indirectly by `tok-protocol-port-only`) | |
| `morgan.StatusCategory` | extra, untested | — | no upstream analogue |
| `morgan.StatusColorCode` | extra, untested | — | the colour selection it encodes is compared through `fmt-dev-*` |
| `morgan.IP.String`, `morgan.Log.String` | extra, untested | — | `Log.String()` renders combined format outside the middleware |

## Counts

| status | tokens | formats | options | Go-only API |
| --- | --- | --- | --- | --- |
| match | 8 | 4 | 3 | — |
| differs | 5 (+1 unknown-token behaviour) | 6 | 1 | — |
| missing | 0 | 1 | 2 | — |
| extra | 6 | 1 | 0 | 16 |
| untested | 0 | 0 | 1 | 7 |

Upstream symbols compared: 13 tokens + 5 named formats + `default` +
`compile`/`format`/`token` + the middleware and its 4 live options.
**Parity over the 98 executed cases: 66 match / 32 mismatch = 67.35 %.**

## Every real divergence, in one list

1. **`:res[content-length]` is `-` whenever the framework computes the length.**
   The port only reads the response header map, and `net/http` writes
   `Content-Length` on the wire without putting it there. Costs
   combined/common/short/tiny/dev their size field.
   (`bug-content-length-{tiny,combined,common,short,dev}`)
2. **`dev` loses all colour.** The port falls back to a plain format string when
   the stream is not a terminal; upstream always emits the ANSI sequences.
   (`fmt-dev-2xx/3xx/4xx/5xx`, `bug-content-length-dev`)
3. **`:url` is percent-decoded.** `/a%2Fb/caf%C3%A9` is logged as `/a/b/café`,
   `/two%20words` as `/two words`. (`bug-url-percent-decoded`,
   `bug-url-percent-encoded-space`, `urlencoded-path-combined`)
4. **`X-Forwarded-For` is trusted unconditionally** — a client on a direct
   connection chooses its own `:remote-addr`. (`bug-xff-trusted-unconditionally`,
   `xff-single-entry-remote-addr`, `xff-combined`)
5. **A forwarded chain or non-IP `X-Forwarded-For` renders the literal `<nil>`**,
   because the whole header is stored in a `net.IP`.
   (`bug-xff-list-renders-nil`, `bug-xff-non-ip-value`, `xff-multi-entry-remote-addr`)
6. **The port-only `json` format emits invalid JSON** —
   `"contentLength":-`. (`bug-json-format-invalid-json`, `fmt-json-port-only`)
7. **`:date[<unknown>]` falls back to the web format** instead of `-`.
   (`tok-date-unknown-arg`)
8. **`:response-time[<non-numeric>]` keeps 3 decimals** where `toFixed` gives 0.
   (`tok-response-time-digits-invalid`)
9. **`:response-time` prints `0.000` in `immediate` mode** where upstream prints
   `-`. (`immediate-response-time-unavailable`)
10. **An unknown token renders `-`** instead of throwing.
    (`tok-unknown-name`) — also why every port-only token
    (`:pid`, `:incoming`, `:protocol`, `:host`, `:path`, `:query`) is a mismatch
    rather than a silent extra.
11. **No `default` format**, so `morgan('default')` and an empty format name
    behave differently (literal string / no line at all).
    (`fmt-default-deprecated`, `fmt-raw-empty-string`)
