# express — parity coverage

- **Upstream oracle:** `express@4.21.2` (npm, pinned in [`node/package.json`](node/package.json); express 4 was chosen because the Go port's routing syntax — `:id?`, `:id(\d+)`, bare `*` — is express 4 / `path-to-regexp@0.1.x` syntax, not express 5).
- **Go port under test:** `github.com/malcolmston/express v0.4.0`, consumed as a published module (no `replace` directive). Note that `v0.4.0` is the newest tag and lags the working tree in this repo, so working-tree fixes are not reflected here.
- **Toolchain:** `go version go1.26.5 darwin/arm64`, node v24.18.0 / npm 11.16.0.

## What is compared

express is a web framework, so the comparable artefact is **the HTTP response to a
given request**. Both runners build the *same* set of applications ("fixtures"),
listen each one on a random `127.0.0.1` port, replay the request a case
describes, and emit `{status, headers, body}` as the case value. Nothing
contacts the network beyond loopback.

A case therefore looks like:

```json
{ "id": "merge-params-on", "fn": "route.mount",
  "args": [{ "method": "GET", "path": "/u/9/who" }] }
```

where `fn` names the fixture. The fixtures are defined side by side in
[`node/run.js`](node/run.js) and [`go/run.go`](go/run.go) and are kept as literal
translations of each other.

Two case options exist beyond the plain request:

- `"revalidate": "etag"` / `"lastModified"` — the runner issues the request,
  takes the validator from the response, and re-issues the request with
  `If-None-Match` / `If-Modified-Since`. This compares 304 handling without
  depending on the ETag algorithm. If the response carried no validator at all,
  the runner reports `{"status": 0, "validator": "absent"}`.

## Header normalisation

Both runners apply exactly the same normalisation before emitting, and the
harness compares the results verbatim. What is normalised:

| header | treatment | why |
| --- | --- | --- |
| `Date` | dropped | wall clock |
| `Connection`, `Keep-Alive`, `Transfer-Encoding`, `Content-Length` | dropped | HTTP framing differs between Node and Go for identical bodies (chunked vs. sized), and the body itself is compared |
| `X-Powered-By` | dropped | express advertises itself, the Go port does not; this would otherwise fail every single case |
| `ETag`, `Last-Modified` | **dropped** | the values are implementation-defined (hash algorithm, file clock). Conditional-request *behaviour* is still compared, through status codes and the `revalidate` mechanism |
| `Content-Type` | `charset=` token lower-cased | charset tokens are case-insensitive (`UTF-8` vs `utf-8`) |
| `Set-Cookie` | `Expires=<value>` replaced with `Expires=<opaque>` | the value is derived from the wall clock; the parameter's *presence* is still compared |

Everything else is compared exactly: header names are lower-cased, every
name/value pair is emitted (so duplicate `Set-Cookie` headers survive) and the
list is sorted by name then value.

Two fixture-level normalisations are worth naming because they are choices, not
mechanics:

- The routing fixtures end with an explicit `404` handler that sends `NOMATCH`,
  so a routing case measures *routing*, not the framework's default error page.
  The default page is compared on its own in the `route.default404` fixture
  (`default404-*`).
- JSON response bodies built by the fixtures for reporting purposes (`req.params`,
  `req.accepts` results, `req.baseUrl`…) have their keys sorted on the JS side,
  because Go's `map` marshalling sorts and JS preserves insertion order. That
  ordering difference is itself compared once, deliberately, by
  `res-json-key-order`.

## Honest scope statement

**This harness compares a slice of express, not express.** express's own surface
is 244 enumerated symbols (below) and most of the framework's real-world
behaviour lives in its *middleware ecosystem* — `body-parser` options, `serve-static`
options, `cookie-parser`, `express-session`, `morgan`, `compression`, `cors`,
`helmet`, view engines, `trust proxy`, signed cookies, the `qs` extended query
parser, multipart uploads, streaming/SSE — none of which is compared here. Within
what *is* compared, only the default options of `express.json`,
`express.urlencoded` and `express.static` are exercised; every option object is
untested. Views, `res.render` and `app.engine` are entirely out of scope. The Go
port additionally ships 102 middleware subpackages and 88 utility ports that have
no upstream counterpart in the `express` package itself and are not scored here.

## Results

`go test ./parity/express/` — **123/184 cases match (66.8%)**, 61 mismatches, 1 declared deviation.

| group | cases | match | mismatch |
| --- | --- | --- | --- |
| `body` | 15 | 7 | 8 |
| `conditional` | 9 | 5 | 4 |
| `errors` | 8 | 6 | 2 |
| `negotiation` | 14 | 5 | 9 |
| `response` | 31 | 21 | 10 |
| `routing-basic` | 35 | 30 | 5 |
| `routing-mount` | 16 | 13 | 3 |
| `routing-options` | 12 | 10 | 2 |
| `routing-param` | 4 | 4 | 0 |
| `routing-params` | 25 | 18 | 7 |
| `static` | 15 | 4 | 11 |
| **total** | **184** | **123** | **61** |

## Divergences found

### Routing and path matching (the core area)

1. **No HEAD-for-GET.** express answers `HEAD /a` from a `GET /a` route with a
   bodyless 200. The Go port 404s. — `basic-head-for-get`,
   `basic-head-for-get-root`, `etag-head`, `static-head` (partially).
2. **Wildcard params are keyed differently.** `/star/*` captures into `params["*"]`
   in Go and `params["0"]` in express 4. — `wildcard-one`, `wildcard-deep`,
   `wildcard-empty`, `wildcard-middle`, `wildcard-middle-deep`.
3. **An absent optional param is present-but-empty.** For `/opt/:id?` matching
   `/opt`, express reports `{}` and the Go port `{"id": ""}`. — `param-optional-absent`,
   `param-optional-absent-slash`.
4. **`app.set('strict routing')` and `app.set('case sensitive routing')` are inert.**
   The Go `Application` stores the settings but the root router never reads them, so
   `/s/` still matches `/s` and `/foo` still matches `/Foo`. The behaviour *is*
   available, but only via `express.NewRouter(RouterOptions{...})`. — `appset-strict-extra-slash`,
   `appset-case-wrong` (and `strict-*`/`case-*`, which pass because the Go fixture
   uses `RouterOptions`).
5. **Path-scoped middleware does not see the residual path.** With
   `app.use('/pfx', mw)`, express rewrites `req.url`/`req.path` to the part after
   the prefix; the Go port leaves the full path. — `prefix-middleware`,
   `prefix-middleware-bare`.
6. **No `req.baseUrl`, and `req.path` is not remapped inside a mounted router.**
   express reports `baseUrl="/loc"`, `path="/where"`; Go reports `baseUrl=""` (the
   runner has to derive it) and `path="/loc/where"`. — `mount-locations`.
7. **Percent-encoded slashes are decoded before matching.** `GET /a%2Fb` 404s in
   express but matches the `/a/b` route in the Go port. — `basic-encoded-path`.

Everything else in routing matched, including: static path matching and depth,
all common verbs, `app.all`, first-registration-wins precedence, param-vs-static
registration order, multi-handler chains with `next()`, `next()` fall-through to
404, non-strict trailing slashes, default case-insensitivity, query strings not
affecting matching, `:param`, multiple params, `:from-:to` pairs,
`:id(\d+)`/`:w([a-z]+)` constraints (including rejection and whole-segment
anchoring), wildcard middle segments, mounted routers at `/` and at parameterised
prefixes, two-level nesting, router-scoped middleware, `mergeParams` on *and* off,
`RouterOptions` strict/case-sensitive matching, and `app.param` callbacks
(including firing for the right name only and `next(err)` from a callback).

### Error handling

8. **An error raised before a mounted sub-router does not skip it.** For
   `app.use('/sub', erroringMiddleware, subRouter)`, express skips `subRouter` and
   runs the app error handler; the Go port runs the sub-router's route and returns
   200. — `err-skips-subrouter`.
9. **A panic in a handler is not caught.** express catches a synchronous `throw`
   and routes it to error middleware; the Go port needs `express.Recover()`, and
   without it the connection is reset (the Go runner reports `ok:false`). —
   `err-throw`.

`next(err)`, app-level error handlers, router-local error handlers taking
precedence, and a custom terminal 404 all matched.

### Body parsing

10. **`express.URLEncoded` stores `url.Values`.** Bodies come back as
    `{"a":["1"]}` instead of `{"a":"1"}`. — `urlencoded-simple`,
    `urlencoded-escaped`, `urlencoded-flag-only`.
11. **Malformed JSON yields 500, not 400.** The Go error has no HTTP status to
    carry, so the error handler cannot produce 400. — `json-malformed`.
12. **No `strict` JSON mode.** express rejects a top-level scalar body (`42`)
    with 400; the Go port accepts it. — `json-scalar`.
13. **Unparsed bodies are `nil`, not `{}`.** When the content type is not JSON,
    express leaves `req.body = {}` and Go leaves it nil. — `json-wrong-content-type`,
    `json-no-content-type`.
14. **Invalid percent escapes are fatal.** `k=%ZZ` errors in Go (500) and is kept
    verbatim by express. — `urlencoded-malformed-escape`.

### Response helpers

15. `res.sendStatus(418)` — `"I'm a teapot"` (Go `http.StatusText`) vs
    `"I'm a Teapot"`. — `res-send-status-418`.
16. `res.redirect` — body omits the status phrase (`"Redirecting to /target"` vs
    `"Found. Redirecting to /target"`) and no `Vary: Accept`. — `res-redirect-default`,
    `res-redirect-301`.
17. `res.type('application/xml')` — no `charset` appended for a full media type. —
    `res-type-full`.
18. `res.cookie` — no `Expires` derived from `MaxAge`, and attributes in a
    different order. — `res-cookie-opts`.
19. `res.clearCookie` — `Max-Age=0` instead of a 1970 `Expires`. — `res-clear-cookie`.
20. `res.jsonp` — `application/javascript` (no charset) vs `text/javascript; charset=utf-8`;
    the no-callback fallback omits `X-Content-Type-Options: nosniff`. — `res-jsonp`,
    `res-jsonp-nocb`.
21. `res.json` — Go sorts object keys, JS preserves insertion order. —
    `res-json-key-order`.
22. **The default 404 response is plain text.** express's `finalhandler` sends an
    HTML error page with `Content-Security-Policy: default-src 'none'` and
    `X-Content-Type-Options: nosniff`; the Go port sends `Cannot GET /y` with
    neither header. — `default404-miss`, `default404-miss-method`.

### Content negotiation

23. `res.format` does **not** set the negotiated Content-Type — the body chosen for
    `text/plain` is sent as `text/html`. — `format-text`, `format-star`,
    `format-subtype-star`, `format-no-accept`.
24. `res.format` with nothing acceptable answers 406 itself instead of calling
    `next(NotAcceptableError)`, so error middleware never sees it. — `format-406`,
    `format-one-unacceptable`.
25. **`res.Format` takes an unordered Go `map`**, so its documented "first entry is
    the default" fallback has no defined winner. This is a genuine
    non-determinism in the port; the harness works around it by using
    single-entry maps for the fallback cases.
26. `req.acceptsEncodings` does not treat `identity` as implicitly acceptable and
    returns `""` where express returns `identity`. — `accepts-none-acceptable`,
    `accepts-no-headers`.
27. `req.acceptsLanguages` does no language-range prefix matching: `Accept-Language: en-GB`
    against the offer `en` returns `""` in Go, `en` upstream; and with no header at
    all Go returns nothing where express returns the first offer. — `accepts-json-first`,
    `accepts-no-headers`.
28. `req.acceptsCharsets` likewise returns `""` where express falls back. —
    `accepts-none-acceptable`.

Media-type negotiation itself (`req.accepts` with `q` weights, `*/*`, explicit
types) matched.

### Conditional requests / ETag

29. **The Go port generates no ETag whatsoever.** express sets a weak ETag on
    `res.send`/`res.json` by default, so a conditional replay 304s; against the Go
    port there is no validator to replay at all. — `etag-revalidate-304`,
    `etag-revalidate-json-304` (both report `validator: absent`).
30. `If-None-Match: *` is not honoured. — `etag-star-validator`.

Hand-rolled conditional handling (`res.Set("ETag", …)` plus an explicit 304)
matched exactly — `etag-manual-miss`, `etag-manual-hit`.

### Static file serving

31. **`express.Static` is broken under a mount prefix.** `app.Use("/assets", express.Static(dir))`
    serves nothing — every request falls through to the 404 handler — because the
    middleware resolves the file against the *full* request path rather than the
    residual. — `static-mount-text`, `static-mount-nested`, `static-mount-index`.
32. **No `Cache-Control`.** `serve-static` sends `Cache-Control: public, max-age=0`;
    the Go port sends nothing. This alone fails every static case. — all
    `static-*` 200/206/304 cases.
33. **No ETag on static files** (consequence of #29), so a static conditional
    request only revalidates on `Last-Modified`. — `static-revalidate` (Go 304 but
    without `Cache-Control`).
34. **`GET /index.html` 301-redirects to `./`.** The Go port delegates to
    `http.ServeFile`, which rewrites `index.html` requests; express serves the file.
    — `static-html`.
35. `.json` files are served without a `charset` parameter. — `static-json`.

Directory-index serving at the mount root, nested paths, fall-through for missing
files, path-traversal rejection, non-GET fall-through and `Range` handling all
matched apart from the header differences above.

## Declared deviations

| case | deviation |
| --- | --- |
| `req-cookie` | express requires the `cookie-parser` middleware for `req.cookies`, so the upstream fixture reports `undefined`; the Go port reads cookies natively via `Request.Cookie`. The Go behaviour is a deliberate superset. |

## API inventory

Every table below was derived **mechanically from the installed
`express@4.21.2`**, not from the README, using the commands named in each
section's caption. 244 symbols in total: the six prototype/module tables account for the 213 names JavaScript reflection reports, and the remaining 31 are per-request/per-response instance properties and application settings, enumerated at runtime.

### 1. `express` module exports

Enumerated with `Object.getOwnPropertyNames(require('express'))` on the installed package (31 names, including the JS `Function` internals `length`/`name`/`prototype` and the 17 deprecated Connect stubs, which throw on property access).

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `express()` | `express.New` | match | every fixture |  |
| `express.Router` | `express.NewRouter` | match | `mount-*`, `nested-*`, `router-middleware*`, `merge-params-*` |  |
| `express.Route` | `express.Route` | untested | — | `app.route()` chaining not exercised |
| `express.application` | `*express.Application` | match | every fixture | prototype object |
| `express.request` | `*express.Request` | match | every fixture | prototype object |
| `express.response` | `*express.Response` | match | every fixture | prototype object |
| `express.json` | `express.JSON` | differs | `json-*` | status on malformed body (400 vs 500); `strict` mode (upstream rejects non-object JSON); default `{}` vs `nil` when the content type does not match |
| `express.urlencoded` | `express.URLEncoded` | differs | `urlencoded-*` | Go yields `url.Values` (`{"a":["1"]}`) where upstream yields flat strings; invalid `%` escapes error in Go, are kept verbatim upstream |
| `express.text` | `express.Text` | untested | — |  |
| `express.raw` | — | missing | — | no raw/`[]byte` body parser |
| `express.static` | `express.Static` | differs | `static-*` | no `Cache-Control`; no ETag; `index.html` 301-redirects to `./`; broken under a mount prefix |
| `express.query` | built into `Request.QueryValues` | match | `req-query*` | the extended (`qs`) parser itself is not compared |
| `express.length` / `.name` / `.prototype` | — | untested | — | JavaScript `Function` internals, not express API |
| `express.bodyParser` | `express.JSON`/`URLEncoded` | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |
| `express.compress` | `express.Compress` | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |
| `express.cookieParser` | `Request.Cookie` | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |
| `express.cookieSession` | `express.Session` | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |
| `express.csrf` | `middleware/csrf` | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |
| `express.directory` | — | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |
| `express.errorHandler` | `express.Recover` | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |
| `express.favicon` | `express.Favicon` | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |
| `express.limit` | `express.BodyLimit` | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |
| `express.logger` | `express.Logger` | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |
| `express.methodOverride` | `express.MethodOverride` | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |
| `express.multipart` | `express.Multipart` | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |
| `express.responseTime` | `express.ResponseTime` | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |
| `express.session` | `express.Session` | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |
| `express.staticCache` | — | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |
| `express.timeout` | `express.Timeout` | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |
| `express.vhost` | — | untested | — | upstream getter throws by design (express 4 unbundled it); the Go port ships a same-named helper that this harness does not compare |

### 2. `app` (application prototype)

Enumerated with `Object.getOwnPropertyNames(require('express/lib/application'))` (53 names).

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `app.get(path,...)` | `Router.Get` | match | `basic-*`, `param-*`, `mount-*` (most cases) |  |
| `app.get(setting)` | `Application.GetSetting` | untested | — | the reader half of the overloaded `get` |
| `app.post` | `Router.Post` | match | `basic-post`, `basic-all-post`, `err-post-unmatched` |  |
| `app.put` | `Router.Put` | match | `basic-put` |  |
| `app.delete` | `Router.Delete` | match | `basic-delete`, `basic-all-delete` |  |
| `app.del` | — | missing | — | deprecated alias for `delete` |
| `app.patch` | `Router.Patch` | match | `basic-patch` |  |
| `app.head` | `Router.Head` | differs | `basic-head-explicit`, `basic-head-for-get*`, `etag-head`, `static-head` | explicit HEAD routes work; **the Go port does not serve HEAD from a GET route** (404 instead of a bodyless 200) |
| `app.options` | `Router.Options` | match | `basic-options-route`, `basic-options-auto` | both answer an unrouted OPTIONS the same way after normalisation |
| `app.all` | `Router.All` | match | `basic-all-*` |  |
| `app.query` | `Router.Query` | untested | — | the HTTP QUERY method |
| `app.use` | `Router.Use` | differs | `mount-*`, `prefix-middleware*`, `err-*`, `static-mount-*` | mounting works, but path-scoped middleware does not get the residual path, and an error before a mounted router does not skip it |
| `app.param` | `Router.Param` | match | `param-cb-*` |  |
| `app.route` | `Router.Route` | untested | — |  |
| `app.set` | `Application.Set` | differs | `appset-strict-*`, `appset-case-*` | `strict routing` and `case sensitive routing` are stored but never reach the root router in the Go port |
| `app.enable` | `Application.Enable` | untested | — |  |
| `app.disable` | `Application.Disable` | untested | — |  |
| `app.enabled` | `Application.Enabled` | untested | — |  |
| `app.disabled` | `Application.Disabled` | untested | — |  |
| `app.engine` | `Application.Engine` | untested | — | view engines are out of scope for this harness |
| `app.render` | `Application.Render` | untested | — |  |
| `app.listen` | `Application.Listen` | untested | — | the runners use `http.Server`/`ServeHTTP` on a random port instead |
| `app.path` | — | missing | — | no mount-path accessor |
| `app.handle` | `Application.ServeHTTP` | match | every fixture | dispatch entry point |
| `app.init` / `app.defaultConfiguration` / `app.lazyrouter` | `express.New` | match | every fixture | internal construction |
| `app.bind` (`length`-style JS internals) | — | untested | — | JavaScript `Function` internals |
| `app.acl` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.bind` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.checkout` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.connect` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.copy` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.link` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.lock` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.m-search` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.merge` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.mkactivity` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.mkcalendar` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.mkcol` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.move` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.notify` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.propfind` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.proppatch` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.purge` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.rebind` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.report` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.search` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.source` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.subscribe` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.trace` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.unbind` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.unlink` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.unlock` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |
| `app.unsubscribe` | — | missing | — | rare HTTP verb from the `methods` package; the Go port exposes only GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/QUERY/ALL |

### 3. `req` (request prototype + per-request instance properties)

Prototype methods and getters from `Object.getOwnPropertyNames(require('express/lib/request'))` (23 names; getters identified via `Object.getOwnPropertyDescriptor`). The per-request properties were enumerated at runtime with `Object.keys(req)` inside a live handler and filtered down to the ones express itself adds (the rest belong to Node's `http.IncomingMessage`).

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `req.accepts` | `Request.Accepts` | differs | `accepts-*`, `format-*` | media-type selection agrees; see the offer-list findings below |
| `req.acceptsCharsets` | `Request.AcceptsCharsets` | differs | `accepts-*` | upstream falls back to an acceptable default where Go returns "" |
| `req.acceptsEncodings` | `Request.AcceptsEncodings` | differs | `accepts-*` | upstream treats `identity` as always acceptable and defaults to it with no header; Go does not |
| `req.acceptsLanguages` | `Request.AcceptsLanguages` | differs | `accepts-json-first`, `accepts-no-headers` | no language-range prefix matching in Go (`en-GB` does not match the offer `en`); no default when the header is absent |
| `req.acceptsCharset` / `acceptsEncoding` / `acceptsLanguage` | — | missing | — | deprecated singular aliases |
| `req.fresh` | `Request.Fresh` | differs | `etag-revalidate-*`, `etag-star-validator` | `If-None-Match: *` is not honoured; there is no framework ETag to revalidate against |
| `req.stale` | `Request.Stale` | untested | — |  |
| `req.get` | `Request.Get` | match | `req-info`, `etag-manual-*` |  |
| `req.header` | `Request.Header` | match | `req-info` | alias of `get` |
| `req.hostname` | `Request.Hostname` | match | `req-info` | presence only |
| `req.host` | — | missing | — | deprecated alias of `hostname` |
| `req.ip` | `Request.IP` | untested | — | loopback address is not a useful comparison |
| `req.ips` | — | missing | — | needs `trust proxy`, which the Go port lacks |
| `req.is` | `Request.Is` | untested | — | exercised indirectly by the body parsers |
| `req.param(name)` | — | missing | — | Go offers `Params`/`Query` separately, not the merged lookup |
| `req.path` | `Request.Path` | differs | `req-info`, `mount-locations`, `prefix-middleware*` | correct at the top level; inside a mounted router Go reports the full path, upstream the residual |
| `req.protocol` | `Request.Protocol` | match | `req-info` |  |
| `req.secure` | `Request.Secure` | match | `req-info` |  |
| `req.range` | `Request.Ranges` | untested | — | `static-range` exercises the static handler, not this accessor |
| `req.subdomains` | `Request.Subdomains` | untested | — |  |
| `req.xhr` | `Request.Xhr` | match | `req-info-xhr` |  |
| `req.params` | `Request.Params` / `AllParams` | differs | `param-*`, `wildcard-*`, `merge-params-*` | named/regex/mount params match; **wildcards are keyed `*` in Go and `0` upstream**, and an absent optional param is present-but-empty in Go |
| `req.query` | `Request.Query` / `QueryValues` | match | `req-query*` |  |
| `req.body` | `Request.Body` | differs | `json-*`, `urlencoded-*` | see `express.json` / `express.urlencoded` |
| `req.baseUrl` | — | missing | `mount-locations` | the Go runner derives it from `OriginalURL`; the port has no accessor |
| `req.originalUrl` | `Request.OriginalURL` | match | `mount-locations` |  |
| `req.url` | `Request.Path` (nearest) | differs | `mount-locations`, `prefix-middleware*` | upstream rewrites it to the residual path inside a mount and keeps the query string |
| `req.method` | `Request.Method` | match | `basic-all-*`, `req-info` |  |
| `req.route` | — | missing | — | no matched-route introspection on the request |
| `req.app` | — | missing | — |  |
| `req.res` / `req.next` | — | missing | — | back-references |
| `req.cookies` | `Request.Cookie` | differs | `req-cookie` | declared deviation: upstream needs `cookie-parser`; the Go port reads cookies natively |
| `req.signedCookies` | — | missing | — | needs a cookie secret |
| `req.setPath` (none upstream) | `Request.SetPath` | extra | — | Go-only path-rewrite helper |
| `req.session` (via express-session) | `Request.Session` | untested | — |  |

### 4. `res` (response prototype + per-response instance properties)

Prototype methods from `Object.getOwnPropertyNames(require('express/lib/response'))` (23 names, no getters). Instance properties from `Object.keys(res)` in a live handler, filtered to what express adds on top of Node's `http.ServerResponse`.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `res.append` | `Response.Append` | match | `res-append` |  |
| `res.attachment` | `Response.Attachment` | untested | — |  |
| `res.clearCookie` | `Response.ClearCookie` | differs | `res-clear-cookie` | Go emits `Max-Age=0`, upstream a 1970 `Expires` |
| `res.contentType` | `Response.Type` | untested | — | alias of `type` |
| `res.cookie` | `Response.Cookie` | differs | `res-cookie-opts`, `res-cookie-plain` | Go omits the `Expires` that upstream derives from `maxAge`, and emits the attributes in a different order |
| `res.download` | `Response.Download` | untested | — |  |
| `res.format` | `Response.Format` | differs | `format-*` | Go does not set the negotiated Content-Type, answers 406 itself instead of delegating to error middleware, and — because it takes an *unordered Go map* — has no defined "first entry" fallback |
| `res.get` | `Response.GetHeader` | untested | — |  |
| `res.header` | `Response.Set` | untested | — | alias of `set` |
| `res.json` | `Response.JSON` | differs | `res-json`, `res-json-null`, `res-json-key-order` | values and Content-Type match; Go sorts object keys where JS preserves insertion order |
| `res.jsonp` | `Response.JSONP` | differs | `res-jsonp`, `res-jsonp-nocb` | callback wrapping matches; Content-Type is `application/javascript` (no charset) vs `text/javascript; charset=utf-8`, and the no-callback fallback omits `X-Content-Type-Options` |
| `res.links` | `Response.Links` | match | `res-links` |  |
| `res.location` | `Response.Location` | match | `res-location` |  |
| `res.redirect` | `Response.Redirect` | differs | `res-redirect-default`, `res-redirect-301` | status and `Location` match; Go's body omits the status phrase and it does not add `Vary: Accept` |
| `res.render` | `Response.Render` | untested | — | view rendering is out of scope here |
| `res.send` | `Response.Send` | match | `res-send-string`, `res-send-object`, `res-send-buffer`, and most routing cases | string/object/`[]byte` bodies and Content-Type all agree |
| `res.sendFile` | `Response.SendFile` | untested | — | `static-*` covers the middleware instead |
| `res.sendfile` | — | missing | — | deprecated lowercase alias |
| `res.sendStatus` | `Response.SendStatus` | differs | `res-send-status`, `res-send-status-418` | Go uses `http.StatusText`, so 418 reads "I'm a teapot" vs "I'm a Teapot" |
| `res.set` | `Response.Set` | match | `res-set` |  |
| `res.status` | `Response.Status` | match | `res-status`, `res-send-status` |  |
| `res.type` | `Response.Type` | differs | `res-type-json`, `res-type-html`, `res-type-full` | shorthands match; a full media type gets no `charset` in Go |
| `res.vary` | `Response.Vary` | match | `res-vary` |  |
| `res.locals` | `Response.Locals` | untested | — |  |
| `res.end` | `Response.End` | match | `res-end-204`, `basic-head-explicit`, `etag-manual-hit` |  |
| `res.write` | `Response.Write` | untested | — |  |
| `res.statusCode` | `Response.StatusCode` | untested | — |  |
| `res.headersSent` | `Response.Written` | untested | — |  |
| `res.app` / `res.req` | — | missing | — | back-references |
| — (none upstream) | `Response.SSE`, `Stream`, `SendStream`, `SendChunked`, `WriteChunk`, `Flush`, `CacheControl`, `ETag`, `LastModified`, `NotModified`, `OnBeforeWrite`, `RemoveHeader` | extra | `etag-manual-*` (ETag only) | Go-only response helpers |

### 5. `Router.prototype`

Enumerated with `Object.getOwnPropertyNames(require('express/lib/router'))` (44 names).

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Router.prototype.use` | `Router.Use` | differs | `mount-*`, `router-middleware*`, `prefix-middleware*`, `err-skips-subrouter` | see `app.use` |
| `Router.prototype.get` | `Router.Get` | match | `mount-*`, `nested-*` |  |
| `Router.prototype.post` | `Router.Post` | untested | — | only the app-level verb was exercised on a sub-router |
| `Router.prototype.put` | `Router.Put` | untested | — |  |
| `Router.prototype.delete` | `Router.Delete` | untested | — |  |
| `Router.prototype.patch` | `Router.Patch` | untested | — |  |
| `Router.prototype.head` | `Router.Head` | untested | — |  |
| `Router.prototype.options` | `Router.Options` | untested | — |  |
| `Router.prototype.all` | `Router.All` | untested | — |  |
| `Router.prototype.query` | `Router.Query` | untested | — |  |
| `Router.prototype.param` | `Router.Param` | match | `param-cb-*` |  |
| `Router.prototype.route` | `Router.Route` | untested | — |  |
| `Router.prototype.handle` | `Router` dispatch (unexported) | match | every mount fixture |  |
| `Router.prototype.process_params` | `Router` param dispatch (unexported) | match | `param-cb-*` |  |
| `Router({caseSensitive})` | `RouterOptions.CaseSensitive` | match | `case-*` |  |
| `Router({strict})` | `RouterOptions.Strict` | match | `strict-*` |  |
| `Router({mergeParams})` | `RouterOptions.MergeParams` | match | `merge-params-on`, `merge-params-off` |  |
| `Router.prototype.name` / `.length` / `.prototype` | — | untested | — | JavaScript `Function` internals |
| — (none upstream) | `Router.Routes` | extra | — | Go-only route introspection (feeds `Application.Routes`/`Docs`) |
| `Router.prototype.acl` | — | missing | — | rare HTTP verb |
| `Router.prototype.bind` | — | missing | — | rare HTTP verb |
| `Router.prototype.checkout` | — | missing | — | rare HTTP verb |
| `Router.prototype.connect` | — | missing | — | rare HTTP verb |
| `Router.prototype.copy` | — | missing | — | rare HTTP verb |
| `Router.prototype.link` | — | missing | — | rare HTTP verb |
| `Router.prototype.lock` | — | missing | — | rare HTTP verb |
| `Router.prototype.m-search` | — | missing | — | rare HTTP verb |
| `Router.prototype.merge` | — | missing | — | rare HTTP verb |
| `Router.prototype.mkactivity` | — | missing | — | rare HTTP verb |
| `Router.prototype.mkcalendar` | — | missing | — | rare HTTP verb |
| `Router.prototype.mkcol` | — | missing | — | rare HTTP verb |
| `Router.prototype.move` | — | missing | — | rare HTTP verb |
| `Router.prototype.notify` | — | missing | — | rare HTTP verb |
| `Router.prototype.propfind` | — | missing | — | rare HTTP verb |
| `Router.prototype.proppatch` | — | missing | — | rare HTTP verb |
| `Router.prototype.purge` | — | missing | — | rare HTTP verb |
| `Router.prototype.rebind` | — | missing | — | rare HTTP verb |
| `Router.prototype.report` | — | missing | — | rare HTTP verb |
| `Router.prototype.search` | — | missing | — | rare HTTP verb |
| `Router.prototype.source` | — | missing | — | rare HTTP verb |
| `Router.prototype.subscribe` | — | missing | — | rare HTTP verb |
| `Router.prototype.trace` | — | missing | — | rare HTTP verb |
| `Router.prototype.unbind` | — | missing | — | rare HTTP verb |
| `Router.prototype.unlink` | — | missing | — | rare HTTP verb |
| `Router.prototype.unlock` | — | missing | — | rare HTTP verb |
| `Router.prototype.unsubscribe` | — | missing | — | rare HTTP verb |

### 6. `Route.prototype`

Enumerated with `Object.getOwnPropertyNames(require('express/lib/router/route').prototype)` (39 names).

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Route.prototype.all` | `Route.All` | untested | — |  |
| `Route.prototype.get` | `Route.Get` | untested | — |  |
| `Route.prototype.post` | `Route.Post` | untested | — |  |
| `Route.prototype.put` | `Route.Put` | untested | — |  |
| `Route.prototype.delete` | `Route.Delete` | untested | — |  |
| `Route.prototype.patch` | `Route.Patch` | untested | — |  |
| `Route.prototype.head` | — | missing | — | no HEAD on the Go `Route` chain |
| `Route.prototype.options` | — | missing | — | no OPTIONS on the Go `Route` chain |
| `Route.prototype.query` | — | missing | — |  |
| `Route.prototype.dispatch` | `Route` registration (unexported) | untested | — |  |
| `Route.prototype._handles_method` / `_options` | — | missing | — | internal helpers used for automatic OPTIONS/405 handling |
| `Route.prototype.acl` | — | missing | — | rare HTTP verb |
| `Route.prototype.bind` | — | missing | — | rare HTTP verb |
| `Route.prototype.checkout` | — | missing | — | rare HTTP verb |
| `Route.prototype.connect` | — | missing | — | rare HTTP verb |
| `Route.prototype.copy` | — | missing | — | rare HTTP verb |
| `Route.prototype.link` | — | missing | — | rare HTTP verb |
| `Route.prototype.lock` | — | missing | — | rare HTTP verb |
| `Route.prototype.m-search` | — | missing | — | rare HTTP verb |
| `Route.prototype.merge` | — | missing | — | rare HTTP verb |
| `Route.prototype.mkactivity` | — | missing | — | rare HTTP verb |
| `Route.prototype.mkcalendar` | — | missing | — | rare HTTP verb |
| `Route.prototype.mkcol` | — | missing | — | rare HTTP verb |
| `Route.prototype.move` | — | missing | — | rare HTTP verb |
| `Route.prototype.notify` | — | missing | — | rare HTTP verb |
| `Route.prototype.propfind` | — | missing | — | rare HTTP verb |
| `Route.prototype.proppatch` | — | missing | — | rare HTTP verb |
| `Route.prototype.purge` | — | missing | — | rare HTTP verb |
| `Route.prototype.rebind` | — | missing | — | rare HTTP verb |
| `Route.prototype.report` | — | missing | — | rare HTTP verb |
| `Route.prototype.search` | — | missing | — | rare HTTP verb |
| `Route.prototype.source` | — | missing | — | rare HTTP verb |
| `Route.prototype.subscribe` | — | missing | — | rare HTTP verb |
| `Route.prototype.trace` | — | missing | — | rare HTTP verb |
| `Route.prototype.unbind` | — | missing | — | rare HTTP verb |
| `Route.prototype.unlink` | — | missing | — | rare HTTP verb |
| `Route.prototype.unlock` | — | missing | — | rare HTTP verb |
| `Route.prototype.unsubscribe` | — | missing | — | rare HTTP verb |

### 7. Application settings

Enumerated with `Object.keys(app.settings)` on a live app after `defaultConfiguration()`, plus the two routing settings express reads lazily in `lazyrouter()` (`strict routing`, `case sensitive routing`) and the view settings it sets on first `render`.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `env` | `"env"` setting | untested | — | both default to `development` |
| `etag` / `etag fn` | — | missing | `etag-*`, `static-*` | the Go port has no ETag setting and generates no ETag at all |
| `jsonp callback name` | — | missing | `res-jsonp` | Go hard-codes `callback` |
| `query parser` / `query parser fn` | — | missing | `req-query*` | Go always uses `net/url` parsing; no `qs` extended mode |
| `subdomain offset` | `Request.Subdomains(offset)` | differs | — | a per-call argument in Go, not a setting |
| `trust proxy` / `trust proxy fn` | `express.RealIP` (nearest) | missing | — | no trust-proxy model |
| `view` | — | missing | — | no pluggable View class |
| `views` | `"views"` setting | untested | — |  |
| `x-powered-by` | `"x-powered-by"` setting | differs | — | the setting exists but the Go port never emits the header; the harness drops it, so no case scores it |
| `strict routing` | `RouterOptions.Strict` | differs | `appset-strict-*`, `strict-*` | only settable per-router in Go; `app.Set` is inert |
| `case sensitive routing` | `RouterOptions.CaseSensitive` | differs | `appset-case-*`, `case-*` | only settable per-router in Go; `app.Set` is inert |
| — (none upstream) | `"view engine"`, `"view cache"` | extra | — | Go-side view settings |


## Counts

| status | symbols |
| --- | --- |
| `match` | 40 |
| `differs` | 29 |
| `missing` | 104 |
| `extra` | 4 |
| `untested` | 67 |
| **total enumerated** | **244** |

**Parity over the symbols actually compared** (`match` / (`match` + `differs`)):
**40/69 = 58.0%**.

**Case total:** 184 cases across 11 groups; 123 match, 61 mismatch (66.8%).

The two numbers measure different things and should not be conflated. The symbol
percentage asks "of the API surfaces we touched, how many behave identically in
every way we touched them" — one bad header on `res.jsonp` marks the whole symbol
`differs`, so it is the harsher measure. The case percentage asks "how many
concrete responses were byte-identical after normalisation"; it is more forgiving
of a symbol that is mostly right, but it also double-counts systemic differences,
because a single missing `Cache-Control` fails every static case at once. Neither
number should be quoted without the other, and neither covers the 104 `missing`
and 67 `untested` symbols at all.

## Reproducing

```sh
cd parity/express/node && npm install     # installs express@4.21.2
cd .. && GOWORK=off go test ./            # rewrites parity.json
```

`go test` skips (never fails) when `node` is absent from `PATH` or when
`node/node_modules/express` has not been installed.
