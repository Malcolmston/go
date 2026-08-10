# axios example

A single runnable program that exercises `github.com/malcolmston/axios` against an
in-process `net/http/httptest` server. **No outbound network calls are made** and the
program terminates on its own.

## Module version

The library is consumed as a published Go module — there is **no `replace` directive**.

| Module | Resolved version |
|---|---|
| `github.com/malcolmston/axios` | `v0.0.0-20260719012427-7817c1cbf0d4` |

The repository carries no semver tags, so `@latest` resolves to that pseudo-version
(commit `7817c1cbf0d4`, 2026-07-19). The published tree is byte-identical to the local
`axios/` working copy apart from an untracked `web/vendor` directory, so nothing in this
example depends on uncommitted local changes.

## Run

```sh
cd examples/axios
GOWORK=off go get github.com/malcolmston/axios@latest
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

## What it demonstrates

| Section | API surface |
|---|---|
| GET + query params + JSON decode | `New`, `Config.BaseURL/Timeout/BearerToken/Headers`, `Client.Get`, `RequestConfig.Params/Headers`, `Response.JSON/Status/StatusText/OK/Header/ContentType/ContentLength` |
| Method-scoped default headers | `HeaderDefaults{Common, Get, Post}` via `Config.HeaderGroups` |
| Interceptors | `Config.RequestInterceptors` (adds `X-Trace`), `Config.ResponseInterceptors` (logs every response) |
| POST JSON | struct auto-encoded as `application/json` |
| POST form | `url.Values` auto-encoded as `application/x-www-form-urlencoded` |
| All verbs | `Put`, `Patch`, `Delete`, `Head`, `Options` |
| Generics | `axios.GetJSON[T]`, `axios.PostJSON[T]` |
| Error handling | non-2xx → `*axios.Error` with populated `Response`; `Code`, `StatusCode()`, `ToJSON()`, `IsAxiosError`, `AsError`, `Response.IsClientError()` |
| Status validation | per-request `ValidateStatus` accepting 418 |
| Timeouts | `RequestConfig.Timeout` |
| Cancellation | `NewAbortController` / `Signal` / `Abort`, legacy `NewCancelToken` + `CancelFunc`, `IsCancel` |
| Retries | `Config.Retry` (`RetryConfig{Retries, Backoff}`) against an endpoint that 503s twice |
| Transforms | `TransformRequest` (uppercases body, adds a header), `TransformResponse` (prefixes body) |
| Query serialization | all four `ArrayFormat`s, nested `ParamsMap`, custom `ParamsSerializer`, `GetUri` |
| Progress | `OnUploadProgress` / `OnDownloadProgress`, `ProgressEvent.Progress()` |
| Streaming | `ResponseStream` + `Response.Stream` + `Response.Close` |
| Redirects & helpers | `MaxRedirects: -1`, `Response.IsRedirect/Location/RetryAfter/Cookies` |
| Concurrency | `axios.All`, `axios.Spread` |
| Multipart | `NewFormData`, `AddField`, `AddFileBytes`, `Reader`, `ContentType`, `Boundary`, `Len` |
| Utilities | `IsAbsoluteURL`, `CombineURLs`, `SerializeParams`, `FlattenParams`, `FormToJSON`, `EncodeURIComponent`, `MergeConfig` |
| Default client | `SetDefault`, `Default`, package-level `axios.Get` |

Everything listed above compiles and runs; nothing in the example is commented out.

## Holes found

1. **Timeouts are classified `ERR_CODE_NETWORK`, not `ErrCodeCanceled`, contradicting
   `doc.go`.** `errors.go` documents `ErrCodeCanceled` as covering "timeout, abort", but
   a `RequestConfig.Timeout` expiry yields `Code == ERR_NETWORK` and `IsCancel(err) ==
   false`. Cause: `Client.Request` classifies via `ctx.Err() != nil || IsCancel(err)`,
   where `ctx` is the *parent* context, not the per-attempt timeout context created in
   `newReq`; and `IsCancel` only matches `context.Canceled`/`ErrCanceled`, never
   `context.DeadlineExceeded`. Upstream axios uses a distinct `ECONNABORTED`/`ETIMEDOUT`
   code for this case; this port has no timeout code at all, so callers cannot
   distinguish "connection refused" from "timed out" without string matching.

2. **`Response.Close()` on a streaming response cannot report `MaxContentLength`
   violations**, and more generally `MaxContentLength` is only enforced for buffered
   responses (`readAllLimited` is skipped on the `ResponseStream` path). Not exercised in
   the example because the guard silently does nothing when streaming.

3. **Inconsistent config-parameter shape.** Verb methods take variadic
   `cfg ...*RequestConfig` and silently ignore all but the first (`opt()` returns
   `cfg[0]`), while `GetUri` and `Request` take a plain `*RequestConfig`. The variadic
   form buys optionality but hides a programming error.

4. **No `context.Context` in any signature** — non-idiomatic for Go. Context is only
   reachable via `Config.Context` / `RequestConfig.Context` struct fields, so the
   library cannot participate in the standard `ctx`-first convention and vet-style
   context lint rules do not apply.

5. **Progress callbacks are coarse.** `OnDownloadProgress` fires once per `Read` on the
   body, so for a small buffered response it is invoked a single time with
   `Loaded == Total`; there is no way to request a minimum reporting interval.

Non-issues checked and deliberately *not* reported: `EncodeURIComponent` encoding space
as `+` and leaving `: $ ,` unescaped is documented behavior matching axios's internal
`buildURL` encoder (not JavaScript's `encodeURIComponent`). Brotli being unsupported is
documented in `decode.go` and is a stdlib limitation.
