# axios — upstream API coverage

| | |
| --- | --- |
| upstream oracle | `axios@1.7.9` (pinned in `node/package.json`) |
| Go port | `github.com/malcolmston/axios@v0.3.0` (published module, no `replace`) |
| harness | `go test ./parity/axios/` — one local echo server, two long-lived JSON-Lines runners |
| cases | 151 (`cases/*.json`) — 101 match, 50 differ, 0 declared deviations = **66.9 %** |

## How the upstream list was derived

Mechanically, from the *installed* package — never from the README or memory:

```sh
cd parity/axios/node && node inventory.js      # 140 symbols
```

`inventory.js` enumerates, with `Object.keys` / `Object.getOwnPropertyNames` over the
live module: the `axios` default export, `Axios.prototype`, `AxiosHeaders.prototype`,
`AxiosError` (statics + prototype), `CancelToken.prototype`, `axios.defaults`,
`axios.defaults.headers`, `axios.defaults.transitional`; plus every field of
`AxiosRequestConfig` scraped from the shipped `node_modules/axios/index.d.ts`
(config options are types, so they have no runtime keys to reflect over).
JS intrinsics (`constructor`, `default`, `_request`, `length`, `name`, `apply`, …) are
excluded.

The Go side was enumerated with:

```sh
GOWORK=off go doc -all github.com/malcolmston/axios
```

## Inventory

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `axios.request` | `Client.Request` | differs | verb-method-uppercase, verb-default-method, verb-absolute-url-overrides-baseurl, verb-baseurl-with-path, body-get-with-data | a baseURL that carries a path is dropped by the port |
| `axios.getUri` | `Client.GetUri` | differs | geturi-* | bracket encoding, default array format, baseURL path, param order |
| `axios.delete` | `Client.Delete` | match | verb-delete, verb-delete-with-body | neither side accepts a body argument |
| `axios.get` | `Client.Get` | match | verb-get |  |
| `axios.head` | `Client.Head` | match | verb-head |  |
| `axios.options` | `Client.Options` | match | verb-options |  |
| `axios.post` | `Client.Post` | match | verb-post-json |  |
| `axios.postForm` | — | missing | — | no multipart convenience wrapper |
| `axios.put` | `Client.Put` | match | verb-put-json |  |
| `axios.putForm` | — | missing | — |  |
| `axios.patch` | `Client.Patch` | match | verb-patch-json |  |
| `axios.patchForm` | — | missing | — |  |
| `axios.defaults` | `Default(), SetDefault()` | untested | — | the port exposes a default *Client, not a mutable defaults object |
| `axios.interceptors` | `Config.RequestInterceptors, Config.ResponseInterceptors` | differs | interceptor-* | a rejecting interceptor is wrapped in an AxiosError by the port; axios propagates it verbatim |
| `axios.create` | `Create(), New()` | match | every request case |  |
| `axios.Axios` | `Client` | match | every request case |  |
| `axios.CanceledError` | `CanceledError` | untested | — | no cancellation case (cancel timing is not deterministic across runners) |
| `axios.CancelToken` | `CancelToken, NewCancelToken` | untested | — |  |
| `axios.isCancel` | `IsCancel` | match | every error case (isCancel field) |  |
| `axios.VERSION` | — | missing | — | the repo has a VERSION file but no exported constant |
| `axios.toFormData` | `NewFormData(), FormData` | differs | body-multipart | the port omits the trailing CRLF after the closing boundary |
| `axios.AxiosError` | `Error` | differs | status-4xx/5xx, error-*, timeout-* | different `code` values (see divergences) |
| `axios.Cancel` | — | missing | — | no `Cancel` alias for CanceledError |
| `axios.all` | `All` | match | verb-all-parallel |  |
| `axios.spread` | `Spread` | untested | — | the Go signature only accepts func(...*Response) T, so no shared case is expressible |
| `axios.isAxiosError` | `IsAxiosError` | differs | interceptor-request-rejects, transform-request-throws, error-invalid-url | the port reports true for errors axios leaves as plain Error/TypeError |
| `axios.mergeConfig` | `MergeConfig` | match | mergeconfig-* |  |
| `axios.AxiosHeaders` | — | missing | — | the port uses net/http.Header directly |
| `axios.formToJSON` | `FormToJSON` | match | formtojson-* |  |
| `axios.getAdapter` | — | missing | — | no pluggable adapter registry |
| `axios.HttpStatusCode` | — | missing | — | no status-code enum |
| `Axios#request` | `Client.Request` | differs | see axios.request |  |
| `Axios#getUri` | `Client.GetUri` | differs | see axios.getUri |  |
| `Axios#delete` | `Client.Delete` | match | verb-delete |  |
| `Axios#get` | `Client.Get` | match | verb-get |  |
| `Axios#head` | `Client.Head` | match | verb-head |  |
| `Axios#options` | `Client.Options` | match | verb-options |  |
| `Axios#post` | `Client.Post` | match | verb-post-json |  |
| `Axios#postForm` | — | missing | — |  |
| `Axios#put` | `Client.Put` | match | verb-put-json |  |
| `Axios#putForm` | — | missing | — |  |
| `Axios#patch` | `Client.Patch` | match | verb-patch-json |  |
| `Axios#patchForm` | — | missing | — |  |
| `AxiosHeaders#set` | `http.Header.Set (stdlib)` | missing | — | no port-owned headers type; stdlib covers the operation |
| `AxiosHeaders#get` | `http.Header.Get / Response.Header` | missing | — | stdlib equivalent |
| `AxiosHeaders#has` | — | missing | — |  |
| `AxiosHeaders#delete` | `http.Header.Del (stdlib)` | missing | — | stdlib equivalent |
| `AxiosHeaders#clear` | — | missing | — |  |
| `AxiosHeaders#normalize` | — | missing | — | http.Header canonicalises on Set/Get instead |
| `AxiosHeaders#concat` | — | missing | — |  |
| `AxiosHeaders#toJSON` | — | missing | — |  |
| `AxiosHeaders#getContentType` | `Response.ContentType` | missing | — | response-side only; no request-header accessor |
| `AxiosHeaders#setContentType` | `RequestConfig.ContentType` | missing | — | a config field, not a headers method |
| `AxiosHeaders#hasContentType` | — | missing | — |  |
| `AxiosHeaders#getContentLength` | `Response.ContentLength` | missing | — | response-side only |
| `AxiosHeaders#setContentLength` | — | missing | — |  |
| `AxiosHeaders#hasContentLength` | — | missing | — |  |
| `AxiosHeaders#getAccept` | — | missing | — |  |
| `AxiosHeaders#setAccept` | — | missing | — |  |
| `AxiosHeaders#hasAccept` | — | missing | — |  |
| `AxiosHeaders#getAcceptEncoding` | — | missing | — |  |
| `AxiosHeaders#setAcceptEncoding` | — | missing | — |  |
| `AxiosHeaders#hasAcceptEncoding` | — | missing | — |  |
| `AxiosHeaders#getUserAgent` | — | missing | — |  |
| `AxiosHeaders#setUserAgent` | — | missing | — |  |
| `AxiosHeaders#hasUserAgent` | — | missing | — |  |
| `AxiosHeaders#getAuthorization` | — | missing | — |  |
| `AxiosHeaders#setAuthorization` | `Config.BasicAuth, Config.BearerToken` | missing | headers-basic-auth | expressed as config, not a headers method |
| `AxiosHeaders#hasAuthorization` | — | missing | — |  |
| `AxiosError.from` | — | missing | — | no error-wrapping constructor |
| `AxiosError#toJSON` | `Error.ToJSON, Error.MarshalJSON` | untested | — | field sets differ; no shared case |
| `AxiosError#isAxiosError` | `IsAxiosError` | differs | see axios.isAxiosError |  |
| `CancelToken#throwIfRequested` | — | missing | — |  |
| `CancelToken#subscribe` | — | missing | — | the port exposes CancelToken.Context() instead |
| `CancelToken#unsubscribe` | — | missing | — |  |
| `CancelToken#toAbortSignal` | `AbortSignal, AbortController` | missing | — | no conversion from CancelToken |
| `defaults.transitional` | — | missing | — |  |
| `defaults.adapter` | — | missing | — |  |
| `defaults.transformRequest` | `Config.TransformRequest` | differs | transform-request-* | the port transforms the *encoded* bytes and still sets Content-Type; axios replaces the serializer |
| `defaults.transformResponse` | `Config.TransformResponse` | match | transform-response-* |  |
| `defaults.timeout` | `Config.Timeout` | differs | timeout-* | error code differs (ECONNABORTED vs ERR_NETWORK) |
| `defaults.xsrfCookieName` | `Config.XSRFCookieName` | untested | — | the port needs a cookie jar; not exercised |
| `defaults.xsrfHeaderName` | `Config.XSRFHeaderName` | untested | — |  |
| `defaults.maxContentLength` | `Config.MaxContentLength` | differs | response-max-content-length-* | error code differs |
| `defaults.maxBodyLength` | `Config.MaxBodyLength` | differs | body-max-body-length-* | see config.data — Content-Type divergence dominates |
| `defaults.env` | — | missing | — |  |
| `defaults.validateStatus` | `Config.ValidateStatus` | differs | status-* | error code differs for 4xx and for non-2xx rejected by a custom predicate |
| `defaults.headers` | `Config.Headers` | differs | headers-* | null does not remove a header in the port; no default Accept |
| `defaults.headers.common` | `HeaderDefaults.Common` | match | headers-group-common, headers-group-and-flat-together |  |
| `defaults.headers.delete` | `HeaderDefaults.Delete` | match | headers-group-delete |  |
| `defaults.headers.get` | `HeaderDefaults.Get` | match | headers-group-get |  |
| `defaults.headers.head` | `HeaderDefaults.Head` | match | headers-group-head |  |
| `defaults.headers.post` | `HeaderDefaults.Post` | match | headers-group-method-matching, headers-group-method-not-matching |  |
| `defaults.headers.put` | `HeaderDefaults.Put` | match | headers-group-put |  |
| `defaults.headers.patch` | `HeaderDefaults.Patch` | untested | — | field exists, no case |
| `transitional.silentJSONParsing` | `Response.Text fallback` | match | response-invalid-json-body | both yield the raw string for malformed application/json |
| `transitional.forcedJSONParsing` | — | differs | response-autoparse-json-without-content-type | axios JSON-parses regardless of Content-Type; the port never auto-parses |
| `transitional.clarifyTimeoutError` | — | missing | — |  |
| `config.url` | `Client.Request rawURL` | match | verb-* |  |
| `config.method` | `Client.Request method` | match | verb-method-uppercase, verb-default-method |  |
| `config.baseURL` | `Config.BaseURL` | differs | verb-baseurl-with-path, geturi-with-baseurl | the path component of baseURL is discarded |
| `config.transformRequest` | `Config.TransformRequest, RequestConfig.TransformRequest` | differs | transform-request-* |  |
| `config.transformResponse` | `Config.TransformResponse, RequestConfig.TransformResponse` | match | transform-response-* |  |
| `config.headers` | `Config.Headers, RequestConfig.Headers` | differs | headers-* | null value does not delete; no default Accept |
| `config.params` | `Config.ParamsMap, RequestConfig.ParamsMap / Params` | differs | params-* | key order, bracket encoding, arrays of objects |
| `config.paramsSerializer` | `ParamsSerializer, ArrayFormat` | differs | params-array-*, params-custom-serializer | default array format and [] encoding differ |
| `config.data` | `body argument to Request` | differs | body-* | Content-Type selection differs for string/bytes/urlencoded/nil |
| `config.timeout` | `Config.Timeout, RequestConfig.Timeout` | differs | timeout-* |  |
| `config.timeoutErrorMessage` | — | missing | — |  |
| `config.withCredentials` | — | missing | — | browser-only in axios |
| `config.adapter` | — | missing | — |  |
| `config.auth` | `Config.BasicAuth, RequestConfig.BasicAuth` | match | headers-basic-auth, headers-basic-auth-per-request, headers-basic-auth-unicode-password, headers-explicit-authorization-vs-auth |  |
| `config.responseType` | `Config.ResponseType, ResponseType constants` | match | response-type-text, response-type-json | only buffered types compared; 'stream' has no shared shape |
| `config.responseEncoding` | — | missing | — |  |
| `config.xsrfCookieName` | `Config.XSRFCookieName` | untested | — |  |
| `config.xsrfHeaderName` | `Config.XSRFHeaderName` | untested | — |  |
| `config.onUploadProgress` | `Config.OnUploadProgress, ProgressFunc` | untested | — | progress event shapes are not comparable across runners |
| `config.onDownloadProgress` | `Config.OnDownloadProgress` | untested | — |  |
| `config.maxContentLength` | `Config.MaxContentLength` | differs | response-max-content-length-* |  |
| `config.validateStatus` | `Config.ValidateStatus, RequestConfig.ValidateStatus` | differs | status-* | behaviour matches; error `code` does not. validateStatus:null has no distinct port representation |
| `config.maxBodyLength` | `Config.MaxBodyLength` | differs | body-max-body-length-* |  |
| `config.maxRedirects` | `Config.MaxRedirects` | differs | redirect-* | 0 means 'do not follow' upstream, 'use the default 10' in the port; negative means 'do not follow' in the port, 'fail' upstream |
| `config.maxRate` | — | missing | — |  |
| `config.beforeRedirect` | — | missing | — | Config.RedirectPolicy is a different contract (see extras) |
| `config.socketPath` | — | missing | — |  |
| `config.transport` | `Config.Transport` | untested | — |  |
| `config.httpAgent` | `Config.Transport, Config.HTTPClient` | untested | — | not a 1:1 mapping |
| `config.httpsAgent` | `Config.Transport, Config.HTTPClient` | untested | — |  |
| `config.proxy` | `Config.Proxy, ProxyConfig` | untested | — | no proxy fixture in the harness |
| `config.cancelToken` | `RequestConfig.CancelToken` | untested | — |  |
| `config.decompress` | `Config.Decompress` | differs | response-decompress-false | the port sends no Accept-Encoding at all; axios always advertises the full codec list |
| `config.transitional` | — | missing | — |  |
| `config.signal` | `RequestConfig.Signal, Config.Signal, AbortSignal` | untested | — |  |
| `config.insecureHTTPParser` | — | missing | — |  |
| `config.env` | — | missing | — |  |
| `config.formSerializer` | — | missing | — |  |
| `config.family` | — | missing | — |  |
| `config.lookup` | — | missing | — |  |
| `config.withXSRFToken` | — | missing | — |  |
| `config.fetchOptions` | — | missing | — |  |

### Go-only surface (`extra`)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| — | `BuildURL, SerializeParams, FlattenParams, EncodeURIComponent` | extra | params-*, geturi-* | exported query-building helpers; axios keeps these private |
| — | `BuildFullPath, CombineURLs, IsAbsoluteURL` | extra | verb-absolute-url-overrides-baseurl | exported URL helpers |
| — | `EncodeBody` | extra | body-* | exported body encoder |
| — | `ArrayFormatComma` | extra | — | no axios equivalent (axios has no comma array format) |
| — | `Config.BearerToken, RequestConfig.BearerToken` | extra | — | axios has no bearer shorthand |
| — | `Config.Retry, RetryConfig, DefaultBackoff, DefaultRetryOn` | extra | — | axios has no built-in retry (axios-retry is separate) |
| — | `Config.RedirectPolicy` | extra | — | http.Client CheckRedirect passthrough |
| — | `Config.HTTPClient, Config.Context` | extra | — | Go-specific plumbing |
| — | `ResponseStream, Response.Stream, Response.Close` | extra | — | Go-specific streaming |
| — | `Response.OK/IsClientError/IsServerError/IsRedirect/IsInformational` | extra | — | status-class helpers |
| — | `Response.Text/Bytes/JSON/Cookies/Location/RetryAfter/Header` | extra | response-* | explicit accessors instead of axios' pre-parsed res.data |
| — | `GetJSON/PostJSON/PutJSON/PatchJSON/DeleteJSON/RequestJSON/GetJSONDefault` | extra | — | generic typed helpers |
| — | `AbortController, AbortSignal, NewAbortController` | extra | — | AbortController is a browser global for axios, not an export |
| — | `FormData.AddFile/AddFileBytes/AddFilePart/Boundary/SetBoundary/WriteTo/Reader/Len` | extra | body-multipart | richer multipart builder |
| — | `ErrCodeInvalidURL/BadRequest/Network/Canceled/BadResponse, ErrCanceled` | extra | error-*, status-* | the port's own (smaller) error-code set |
| — | `Error.StatusCode, Error.Unwrap, AsError` | extra | status-* | Go error-idiom helpers |
| — | `HeaderDefaults, DefaultHeaderGroups` | extra | headers-group-* | method-scoped defaults as a struct |
| — | `ProgressEvent.Progress` | extra | — |  |
| — | `Default(), SetDefault()` | extra | — | package-level default client |

## Counts

| status | symbols |
| --- | --- |
| match | 33 |
| differs | 28 |
| missing | 61 |
| untested | 18 |
| extra (Go-only, not scored) | 19 |
| **upstream total** | **140** |

**Symbol parity = 33 match / 61 compared (match + differs) = 54.1 %.**
`missing` (61) and `untested` (18) symbols are excluded from the
denominator because no case compared them; they are the honest ceiling on how much of
axios this harness can currently score.

**Case parity = 101 / 151 = 66.9 %** (see `parity.json`, regenerated by `go test`).

## What the harness normalises, and why

Normalisation happens in `parity_test.go` (`normalise`), applied **identically to both
sides** after decoding and before deep-equal — never inside a runner, so neither
implementation can be flattered.

| normalised | why |
| --- | --- |
| request header `date`, `connection`, `keep-alive`, `transfer-encoding` | transport-level, set by the server/socket layer, not by the client |
| request header `host` | contains the harness' random loopback port |
| request header `content-length` | legitimately differs wherever the encoded body differs (multipart boundary length, Content-Type choice); the body itself *is* compared byte-for-byte |
| request header `user-agent` | `axios/1.7.9` vs `Go-http-client/1.1` — asserted explicitly in `headers-default-user-agent` instead of failing every case |
| request header `accept-encoding` | codec list is a stdlib/adapter decision — asserted explicitly in `headers-default-accept-encoding` and `response-decompress-false` |
| request header `accept` | axios ships a default, the port does not — asserted explicitly in `headers-default-accept` |
| request header `referer` | Go's `net/http` sets it when following a redirect, `follow-redirects` does not; it would otherwise break every redirect case |
| response headers `date`, `content-length`, `connection`, `keep-alive`, `transfer-encoding` | volatile; done in each runner symmetrically |
| multipart boundary tokens | random on both sides; every occurrence of the boundary found in the observed `Content-Type` is rewritten to `BOUNDARY`, in the header *and* in the body |
| `statusText` | not emitted at all unless a case sets `assertStatusText` — `response-status-text` is the one case that compares it |

Cases that need a stripped header assert it through `observeHeaders`, which copies the
value out of the echo body into `value.pickHeaders` **before** stripping. Nothing is
hidden; it is only attributed to one case instead of fifty.

Error *messages* are never compared (upstream and Go word them differently by
construction). What is compared for a failing case is `ok` plus a structured shape:
`{isAxiosError, isCancel, code, hasResponse, status}`.

## Real behavioural divergences found

Query serialisation (the biggest cluster):

1. **Default array format.** axios serialises `{a:[1,2]}` as `a[]=1&a[]=2`; the port's
   zero-value `ArrayFormat` is `ArrayFormatRepeat`, giving `a=1&a=2`.
   (`params-array-default-format`, `geturi-array-default`)
2. **`[` and `]` are percent-encoded by the port.** axios emits `n[x]=1` literally; the
   port emits `n%5Bx%5D=1`. Affects every bracket format and every nested object.
   (`params-array-indexes-false/true`, `params-nested-object`,
   `params-deeply-nested-object`, `geturi-nested-object`, `geturi-array-indexes-true`)
3. **Parameter order.** axios preserves insertion order; the port goes through
   `url.Values.Encode`, which sorts keys. (`params-multi-key-order`)
4. **Params vs. an existing query string.** axios appends after the URL's own query
   (`?z=9&a=1`); the port merges everything into one sorted set (`?a=1&z=9`).
   (`params-existing-query-in-url`, `geturi-existing-query`)
5. **Arrays of objects.** axios produces `a[0][k]=v`; `FlattenParams` loses the index and
   produces `a[k]=v`. (`params-array-of-objects`)

URL building:

6. **`baseURL`'s path is discarded.** With `baseURL: "http://h/echo"` and `url: "/sub"`,
   axios requests `/echo/sub`; the port requests `/sub`. The exported
   `BuildFullPath`/`CombineURLs` helpers are correct — `Client.Request` and
   `Client.GetUri` simply do not use them. (`verb-baseurl-with-path`,
   `geturi-with-baseurl`)

Content-Type selection for request bodies:

7. axios inherits `application/x-www-form-urlencoded` from `defaults.headers.post` for a
   string, a `Buffer` and an absent body; the port picks `text/plain; charset=utf-8`,
   `application/octet-stream` and *nothing* respectively.
   (`body-plain-string`, `body-empty-string`, `body-bytes`, `body-json-null`)
8. `URLSearchParams` bodies: axios sends
   `application/x-www-form-urlencoded;charset=utf-8`, the port omits the charset.
   (`body-urlencoded`)
9. **JSON escaping.** Go's `encoding/json` HTML-escapes `<`, `>` and `&`, so
   `{"a":"<b>&c"}` goes out as `{"a":"<b>&c"}`. (`body-unicode-json`)
10. **Multipart trailer.** The port ends the body at `--BOUNDARY--\r\n`; the undici
    FormData encoder axios uses appends an extra `\r\n`. (`body-multipart`)

Headers:

11. **No default `Accept`.** axios sends `application/json, text/plain, */*`; the port
    sends none. (`headers-default-accept`)
12. **`Accept-Encoding`.** axios advertises `gzip, compress, deflate, br` and keeps doing
    so even with `decompress:false`; the port sends `gzip, deflate` (stdlib), and sends
    **no** `Accept-Encoding` at all when `Decompress` is false — so a server may reply
    uncompressed where axios would still accept compression.
    (`headers-default-accept-encoding`, `response-decompress-false`)
13. **`User-Agent`.** axios identifies itself as `axios/<version>`; the port leaves
    `Go-http-client/1.1`. (`headers-default-user-agent`)
14. **A `null` header value does not delete.** In axios, `headers: {'X-Gone': null}`
    removes an inherited header; the port keeps the inherited value.
    (`headers-null-value-removes`)

Response handling:

15. **`statusText`.** axios: `OK`. The port: `200 OK` (code included).
    (`response-status-text`)
16. **No automatic JSON parsing.** axios parses any body that looks like JSON regardless
    of `Content-Type`; the port requires an explicit `Response.JSON`.
    (`response-autoparse-json-without-content-type`)

Transforms and interceptors:

17. **`transformRequest` operates at a different stage.** axios hands the transform the
    *raw* `data` and the transform *replaces* the default serializer, so no JSON
    Content-Type is set. The port encodes first and transforms the bytes, so the
    encoder's Content-Type survives. Observable for every `transformRequest` case.
    (`transform-request-prefix`, `transform-request-prefix-string-body`,
    `transform-request-can-set-header`, `transform-request-client-level`)
18. **User errors are re-wrapped.** An interceptor or transform that fails propagates as a
    plain `Error` in axios (`isAxiosError === false`, `code === undefined`); the port
    wraps it in its own `*Error` with `ERR_NETWORK` / `ERR_BAD_RESPONSE` /
    `ERR_BAD_REQUEST`, and for a response interceptor even attaches the 200 response.
    (`interceptor-request-rejects`, `interceptor-response-rejects`,
    `transform-request-throws`)

Error codes — the port has 5 codes where axios has a dozen, so the mapping is lossy:

| situation | axios `code` | port `Code` |
| --- | --- | --- |
| 4xx status | `ERR_BAD_REQUEST` | `ERR_BAD_RESPONSE` |
| 5xx status | `ERR_BAD_RESPONSE` | `ERR_BAD_RESPONSE` (match) |
| 2xx rejected by a custom `validateStatus` | *(undefined)* | `ERR_BAD_RESPONSE` |
| timeout | `ECONNABORTED` | `ERR_NETWORK` |
| connection refused | `ECONNREFUSED` | `ERR_NETWORK` |
| unsupported scheme | `ERR_BAD_REQUEST` | `ERR_NETWORK` |
| `maxContentLength` exceeded | `ERR_BAD_RESPONSE` | `ERR_NETWORK` |
| too many redirects | `ERR_FR_TOO_MANY_REDIRECTS` | `ERR_NETWORK` |
| malformed URL | `ERR_INVALID_URL` (a `TypeError`, `isAxiosError === false`) | `ERR_INVALID_URL` (`isAxiosError === true`) |

(`status-400-rejects`, `status-404-rejects`, `status-418-rejects`,
`status-validate-only418-rejects-200`, `timeout-exceeded`,
`timeout-per-request-overrides-client`, `error-connection-refused`,
`error-unsupported-scheme`, `error-invalid-url`,
`response-max-content-length-exceeded`, `redirect-exceeds-max`)

Redirects:

19. **`maxRedirects: 0`** means "do not follow" in axios (you get the 302, which then
    fails the default `validateStatus`); the port treats `0` as "unset, use the default
    10" and follows. There is no port value that means "return the 3xx and keep the
    default cap". (`redirect-max-redirects-zero`,
    `redirect-max-redirects-zero-validate-all`)
20. **A negative `maxRedirects`** disables following in the port; axios rejects with
    `ERR_FR_TOO_MANY_REDIRECTS`. (`redirect-max-redirects-negative`)

## Upstream features the port has no equivalent for

`postForm`/`putForm`/`patchForm`, `getAdapter` + `config.adapter` (no pluggable
adapters), `HttpStatusCode`, `VERSION`, `toFormData` as a standalone converter, the whole
`AxiosHeaders` class (25 methods), `AxiosError.from`, `CancelToken#subscribe` /
`#unsubscribe` / `#throwIfRequested` / `#toAbortSignal`, `config.transitional` (and all
three transitional flags), `config.timeoutErrorMessage`, `config.withCredentials`,
`config.withXSRFToken`, `config.responseEncoding`, `config.maxRate`,
`config.beforeRedirect`, `config.socketPath`, `config.insecureHTTPParser`, `config.env`,
`config.formSerializer`, `config.family`, `config.lookup`, `config.fetchOptions`.

`validateStatus: null` (axios' "accept every status") also has no distinct
representation: a nil `ValidateStatus` in the port means "use the 2xx default", so the
harness maps `null` onto an accept-all predicate.

## Untested, and why

`config.proxy`, `config.httpAgent`/`httpsAgent`/`transport` (no fixture),
`config.signal`/`cancelToken`/`CanceledError` (cancellation timing is not deterministic
enough to compare across two runtimes), `config.onUploadProgress`/`onDownloadProgress`
(progress event shapes are not comparable), `config.xsrfCookieName`/`xsrfHeaderName`
(needs a cookie jar), `axios.spread` (the Go signature only accepts
`func(...*Response) T`, so no shared case is expressible), `AxiosError#toJSON`
(different field sets), `defaults.headers.patch`, `axios.defaults` as an object.
