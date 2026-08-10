# express example

A single runnable program that exercises the real, public API of
[`github.com/malcolmston/express`](https://github.com/Malcolmston/express) — a Go
port of Express.js.

**Module version used: `github.com/malcolmston/express v0.4.0`** (resolved by
`go get github.com/malcolmston/express@latest`; the repo has semver tags, so this
is a real tag rather than a pseudo-version). There is **no `replace` directive** —
the example consumes the published module exactly as an outside user would.

## Run

```sh
cd examples/express
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

The program drives the app entirely in-process with `net/http/httptest`
(`app.ServeHTTP`), prints a labelled transcript of every request, and exits on
its own. It never binds a port and never blocks.

Run it from this directory: the `views/` and `public/` template/static
directories are resolved relative to the working directory.

## What it demonstrates

| Section | Feature |
| --- | --- |
| 1 | `express.New`, `app.Set` / `Enable` / `Disabled` / `GetSetting`, `app.Locals()` |
| 2 | `app.Get`, route params, optional params (`:name?`), regex params (`:id(\d+)`), wildcard (`*`), the built-in 404 |
| 3 | `app.Route(...).Get(...).Delete(...)` chaining, `app.Param` preprocessing, `req.Set` / `req.Value` |
| 4 | `express.NewRouter(RouterOptions{MergeParams: true})` mounted at a parameterised prefix, router-local middleware |
| 5 | `express.JSON()` body parsing, `req.Body()`, `req.Is("json")`, `next(err)` error propagation, `httperrors`, `express.Recover()`, `middleware/errorjson` |
| 6 | `middleware/cors` (incl. preflight), `middleware/pagination`, `middleware/ratelimit`, `middleware/requestid`, `middleware/helmet`, `express.ResponseTime()` |
| 7 | `res.Format` content negotiation, `req.Accepts` / `AcceptsLanguages` / `AcceptsEncodings`, `req.IP` / `Protocol` / `Hostname` / `Xhr` / `BaseURL` with `trust proxy` |
| 8 | `res.Cookie` / `req.Cookie`, `express.Session` + `req.Session()` round-trip through a cookie jar, `jsonwebtoken` sign/verify |
| 9 | `res.Render` with the built-in `html/template` engine, a custom engine via `app.Engine`, `express.Static`, `res.SendFile`, `res.Download`, `res.ETag` / `LastModified` / `Fresh` / `NotModified` (304) |
| 10 | `res.SendChunked`, `res.SSE()` (`Send`, `SendJSON`, `SendID`, `Comment`) |
| 11 | `app.Routes()` / `RouteInfo.OpenAPIPath()`, `app.Describe` + `app.OpenAPI()` / `OpenAPIYAML()`, `app.Channel` + `app.AsyncAPI()` / `ChannelNames()` |
| 12 | Utility subpackages: `ms`, `bytes`, `slugify`, `uuid`, `qs`, `statuses`, `httperrors`, `jsonwebtoken` |

Everything above works. The example compiles and runs clean; no feature had to be
commented out.

## Holes found (all against the published v0.4.0)

1. **`express.JSON()` and `req.BodyJSON()` are mutually exclusive, and the
   collision is silent.** `JSON()` does `io.ReadAll(req.Raw.Body)` and never
   restores the reader. `BodyJSON` then reads the same drained body, hits its
   `len(data) == 0 { return nil }` branch, and returns **`nil` error with a
   zero-valued destination**. Both are advertised as primary body-reading paths
   in the README. Demonstrated live by `POST /api/bodyjson-after-json`; worked
   around in `POST /api/users` by re-marshalling `req.Body()`.

2. **Errors do not skip mounted sub-routers.** `Router.handle` skips non-error
   handlers while an error is propagating, but the `l.mounted != nil` branch is
   checked *before* the `carriedErr != nil` test, so a mounted sub-router is
   entered normally in error mode and its route handlers run as if nothing had
   failed. Concretely: a malformed JSON body makes `express.JSON()` call
   `next(err)`, yet the `POST /api/users` handler still executes. This diverges
   from Express and can turn a rejected request into a successful one.

3. **`express.Static` cannot be mounted under a path prefix.** Only mounted
   `*Router`s get a residual (prefix-stripped) path; a plain `Handler` registered
   with `app.Use("/static", h)` still sees the full path in `req.path`. So
   `app.Use("/static", express.Static("public"))` looks for
   `public/static/hello.txt` and always 404s. Only `app.Use(express.Static("public"))`
   at the root works. The same trap applies to any prefix-mounted middleware that
   reads the path (e.g. `middleware/spa`, `middleware/serveindex`, `rewrite`).

4. **`middleware/helmet`'s "hidepoweredby" is a no-op.** helmet registers
   `res.OnBeforeWrite(func(){ Header().Del("X-Powered-By") })`, but
   `writeHeaderOnce` runs the before-write hooks *first* and then sets
   `X-Powered-By: Express`, so the header always comes back. helmet's own test
   hides this by calling `app.Disable("x-powered-by")` before asserting.
   `app.Disable("x-powered-by")` is the actual fix; the example shows both.

5. **No HEAD → GET fallback.** `HEAD /users/1` returns
   `404 Cannot HEAD /users/1` even though a GET route matches. Express rewrites
   `head` to `get` when no explicit HEAD route exists.

6. **`req.Hostname()` ignores `X-Forwarded-Host`** even with
   `app.Enable("trust proxy")`, while `req.IP()` and `req.Protocol()` do honour
   their forwarding headers. Inconsistent, and wrong behind a proxy.

7. **`middleware/errorjson` flattens all statuses.** It uses one fixed
   `Options.Status` and never inspects `*httperrors.Error`, so
   `next(httperrors.New(418, ...))` is reported to the client as a 500
   (`GET /errors/teapot`). It also ignores the `Expose` flag, so 5xx internals are
   echoed to the client verbatim. Any app using `httperrors` must hand-write its
   own error handler (as this example does at app level).

8. **README-vs-API mismatch:** the published README's middleware example uses
   `cors.Options{AllowOrigins: ...}`, but the field is `AllowedOrigins`. The
   README snippet does not compile.

9. **`jsonwebtoken.Verify` has no verification options in v0.4.0.** The signature
   is `Verify(token string, secret []byte) (Claims, error)` — there is no
   `VerifyOptions`, so no algorithm allowlist and no `iss`/`aud`/`sub`/`jti`/
   `maxAge`/clock-tolerance checks; `SignOptions` likewise lacks `Audience`,
   `JwtID`, `KeyID`, `NoTimestamp` and `Header`. Callers must re-check registered
   claims by hand (the `/protected` handler does). Note the library's *repository
   HEAD* has all of this — it is simply absent from the published tag.

## Non-idiomatic / awkward bits (not bugs)

- Handlers take `next express.Next` where `Next` is `func(err ...error)`. A
  variadic error is how Express's `next(err?)` is spelled, but it means a typo
  like `next(err1, err2)` compiles and silently drops the second error, and
  nothing in the type system distinguishes "continue" from "fail".
- `app.Use(args ...any)` takes an `any` slice and **panics** at registration time
  on an unsupported handler type instead of failing to compile. Error handlers are
  distinguished from normal handlers only by arity at runtime.
- `Application.GetSetting`/`Set` use free-form string keys with `any` values
  (`"view engine"`, `"trust proxy"`, …); typos are silent no-ops.
- There is no `Shutdown`/`ListenWithContext` — only `Listen` and
  `ListenWithServer`, both of which block. Graceful shutdown requires building
  your own `*http.Server` and calling `Shutdown` on it yourself.
- `res.Send(any)` guesses the content type from the Go type: `string` →
  `text/html`, `[]byte` → `application/octet-stream`, anything else → JSON. So a
  `[]byte` holding JSON and a `map` holding the same data get different
  `Content-Type`s, and echoing a user-supplied string defaults to HTML. `res.JSON`
  / an explicit `res.Type(...)` are the predictable choices. (Express behaves the
  same way for strings, so this is faithful rather than wrong.)
