# morgan example

A runnable program that exercises the **published** module
`github.com/malcolmston/morgan` (Go port of Node
[morgan](https://github.com/expressjs/morgan)).

- **Resolved version: `v1.2.0`** (a real semver tag, not a pseudo-version — `go get
  github.com/malcolmston/morgan@latest` reported `v0.0.0 => v1.2.0`).
- No `replace` directive: the module is consumed exactly as an outside user would.

Everything runs in-process against `httptest.NewServer`, log output is captured into
a mutex-guarded buffer per scenario, and the program terminates on its own (exit 0).

## What it demonstrates

1. All six predefined formats (`Combined`, `Common`, `Dev`, `Short`, `Tiny`, `JSON`)
   plus a `json.Valid` check on the JSON line.
2. The same formats resolved by lowercase name (`"combined"`, `"dev"`, …).
3. A wide custom format string: `:method :url :path :query :status :http-version
   :protocol :host :res[header] :req[header] :user-agent :referrer :incoming
   :response-time[1] :total-time[2] :pid :date[iso|clf|web]`, plus an unknown token
   (renders `-`) and a multi-valued request header (joined with `", "`).
4. Status variety (200/302/404/500) and a handler that tries to `Flush`.
5. Basic auth → `:remote-user`, including a log-injection probe.
6. Custom tokens via `morgan.Token`, including the `:name[arg]` argument form.
7. `RegisterFormat` (named format string) and `RegisterFormatFunc` (full control,
   using `StatusCategory`, `StatusColorCode`, `FormatDuration`).
8. Every skip helper: `SkipPaths`, `SkipStatusBelow`, `SkipStatusBetween`,
   `SkipUserAgents`, `CombineSkips`.
9. `Config.Immediate`.
10. `Config.Buffer` (batched flush on an interval).
11. `X-Forwarded-For` / `X-Forwarded-Proto` handling.
12. 20 concurrent requests, checking whole non-interleaved lines.
13. Standalone helpers: `Clfdate`, `ClientIP`, `RequestURL`, `RequestProtocol`,
    `StatusCategory`, `StatusColorCode`, `FormatDuration`, `IP.String`.
14. Building a line by hand: `FromRequest`, `Log.String`, `Compile`.

## Run

```sh
cd examples/morgan
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

## Holes found in v1.2.0

All marked `// HOLE:` in `main.go`. Nothing had to be deleted, but two intended
demos had to be rewritten (a `Flush` and a `TrustProxy` toggle) because the library
cannot support them.

### Correctness / security

1. **`:res[content-length]` is always `-` for ordinary handlers.** Go's `net/http`
   computes `Content-Length` inside the server and never writes it into the header
   map, and v1.2.0 has no observed-body-size fallback. Since `Combined`, `Common`,
   `Short` and `Tiny` all use this token, **every predefined format silently loses
   the response size** — the single most visible parity break with Node morgan.
2. **The `JSON` format does not emit valid JSON.** Placeholders are interpolated
   unquoted, producing `"contentLength":-` (and `"status":-` when there is no
   response). `json.Unmarshal` fails with `invalid character '}' in numeric literal`.
   The example asserts this at runtime.
3. **No token sanitisation → log injection.** Header and credential values are
   written verbatim. A request with `Authorization: Basic base64("ev\nil 999 FORGED:x")`
   produces two log lines. The same applies to `:user-agent`, `:referrer` and
   `:req[...]`.
4. **`RequestURL` / `Log.URL` use the decoded `r.URL.Path`**, so a target of
   `/api/it%0Aems` puts a real newline into the log line — a second injection vector.
5. **`X-Forwarded-For` is trusted unconditionally and cannot be turned off.**
   `Config` has no `TrustProxy` field, so any client on a direct connection can
   choose the address recorded against its own requests (`X-Forwarded-For: 203.0.113.7`
   is logged as the remote address). There is no opt-out.
6. **`FromRequest` does not split `X-Forwarded-For` on `,`.** It assigns the whole
   header to `REMOTE_IP`, so a normal two-hop chain (`"203.0.113.7, 10.0.0.1"`) fails
   `net.ParseIP` and `:remote-addr` renders the literal Go string **`<nil>`**. The
   exported `ClientIP` helper *does* split, so the middleware and the helper disagree
   on the same request.
7. **`:protocol` ignores `X-Forwarded-Proto`** even though the exported
   `RequestProtocol` helper honours it — again the middleware and helper disagree
   (`:protocol` → `http`, `RequestProtocol(req)` → `https`, same request).
8. **The response wrapper hides `http.Flusher`/`Hijacker`/`Pusher`/`ReaderFrom`.**
   `responseRecorder` embeds only `http.ResponseWriter` and forwards nothing else, so
   a handler doing `w.(http.Flusher).Flush()` — perfectly fine without the middleware
   — **panics** once morgan wraps it. Any SSE, chunked-streaming or WebSocket-upgrade
   handler breaks. The example had to guard the assertion with `if f, ok := …`.
9. **No write synchronisation.** `New` calls `fmt.Fprintln(out, line)` straight from
   the request goroutine with no mutex, and with `Config.Buffer > 0` the delayed
   flush runs on a `time.AfterFunc` goroutine. Passing a plain `*bytes.Buffer`
   (or two handlers sharing an `*os.File`) is a data race; `go run -race .` reported
   it before this example switched to its own mutex-guarded writer.
10. **Buffered lines can be lost.** There is no `Flush`/`Close` on the returned
    handler, so anything still in the buffer at shutdown is dropped, and there is no
    way to establish a happens-before edge with the timer goroutine. The example has
    to `time.Sleep` past the interval to observe its own output.
11. **`Immediate` reports timings as `0.000` instead of `-`.** It calls
    `FromRequest(r, 0, 0, 0)`; `:status` and `:res[content-length]` correctly render
    `-`, but `:response-time`/`:total-time` print `0.000`, so an unmeasured request
    looks instantaneous rather than unknown.
12. **`Dev` silently loses its colors when `Stream` is not a terminal**, with no
    force-color option. Node morgan colors regardless.

### Missing API (would not compile)

- `Config.TrustProxy`
- `FromRequestTrustProxy`, `ClientIPTrustProxy`, `RequestProtocolTrustProxy`
- `Log.REMOTE_ADDR` (string), `Log.PROTOCOL`, `Log.RESPONSE_SIZE`,
  `Log.RESPONSE_STREAMED` — only `REMOTE_IP IP` exists, which cannot represent a
  forwarded chain or a Unix-socket peer (hence the `<nil>` above). A size-based
  custom token has to fall back to the *request's* `Content-Length`.

### Ergonomics

- **`New(next, format, cfg)` is not a Go middleware.** Idiomatic Go middleware is
  `func(http.Handler) http.Handler`, which composes with `chi`/`alice`/`mux.Use`.
  This signature has to be adapted by hand for any router that expects the standard
  shape.
- **`Token` and `RegisterFormat`/`RegisterFormatFunc` mutate process-global
  registries** with no per-instance alternative, so two independently configured
  middlewares in one binary cannot have different tokens, and registration order
  matters. (Faithful to Node, unidiomatic in Go.)
- `Config.Skip` is declared as a raw `func(*http.Request, int) bool` rather than the
  exported `SkipFunc` type defined right next to it.

### Note on the local working tree

The local `morgan/` directory contains uncommitted work that fixes most of the above
(`Config.TrustProxy` and the `*TrustProxy` helpers, `REMOTE_ADDR`/`PROTOCOL`/
`RESPONSE_SIZE`/`RESPONSE_STREAMED` fields, a `sanitizeLogValue`/`escapeLogField`
pass over every token, a size fallback for `:res[content-length]`, a capability-
preserving response wrapper, and a `lineWriter` mutex). None of it is in the
published `v1.2.0`, which is what this example is written against.
