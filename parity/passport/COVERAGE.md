# `passport` parity coverage

| | |
| --- | --- |
| Go port | `github.com/malcolmston/passport` **v0.3.0** (resolved by `GOWORK=off go get github.com/malcolmston/passport@latest`; available tags are `v0.1.0 v0.2.0 v0.3.0`) |
| Upstream oracle | `passport@0.7.0`, `passport-local@1.0.0`, `passport-http@0.3.0`, `passport-http-bearer@1.0.1` |
| Upstream host app | `express@4.21.2` + `express-session@1.18.1` (transitively `body-parser@1.20.3`) |
| Runners | `node/run.js` (upstream), `go/run.go` (port) — JSON Lines on stdio, see `../HARNESS.md` |
| Cases | 74, in `cases/local.json`, `cases/basic.json`, `cases/digest.json`, `cases/bearer.json`, `cases/session.json` |
| Harness | `GOWORK=off go test ./parity/passport/` |

## What is compared

The comparable artefact is **the authentication outcome for a given request**. Each
runner builds an equivalent application over the same four strategies (local,
HTTP Basic, HTTP Digest, HTTP Bearer), the same fixed in-memory user store
(`alice/s3cr3t-alice` → `u-alice`, `bob/hunter2` → `u-bob`) and the same fixed
bearer-token table (`tok-alice`, `tok-bob`), replays the request(s) a case
describes, and emits

```jsonc
{"status": 401, "authenticated": false, "user": null,
 "wwwAuthenticate": "Basic realm=\"Parity\"", "setCookiePresent": false, "location": null}
```

(one object for a `request` case, an array of them for a `sequence` case). Both
runners rebuild the whole application **per case**, so no session, cookie or
digest-nonce state leaks between cases. Nothing touches the network beyond
127.0.0.1 loopback and no remote identity provider is contacted.

### Determinism and normalisation

Fixed on both sides:

* the digest server nonce (`digest.Options.Nonce` on the port, and upstream's
  nonce is only ever *echoed*, never validated for issuance) — pinned to
  `deadbeefdeadbeefdeadbeefdeadbeef`;
* the client digest inputs (`cnonce=0a4f113b`, `nc=00000001`/`00000002`), and the
  `Authorization` headers themselves, which are precomputed and committed —
  regenerate with `node cases/generate-digest.js`;
* the session secret (`parity-fixed-session-secret`) and cookie name per side;
* form-field iteration order (the Go runner sorts form and cookie keys).

Normalised before emitting:

| volatile value | normalisation |
| --- | --- |
| digest challenge nonce | every `nonce="…"` in `WWW-Authenticate` → `nonce="<NONCE>"` |
| session id | never emitted; only `setCookiePresent` (a `Set-Cookie` for this side's session cookie with a non-empty value) |
| session cookie name | `connect.sid` upstream vs `passport.sid` in the port — never emitted, and cookie replay is driven by the case field `session: "carry" \| "tamper" \| "none"` so cases stay name-agnostic |
| authenticated principal | reduced to the **username** — upstream yields the user object, while the port's digest strategy yields a bare username string (`Context.Success(username)`); comparing the principal keeps the outcome comparable and the shape difference is recorded below |
| `Date`, `Content-Length`, `ETag`, `Content-Type`, all other headers | dropped; only `WWW-Authenticate` and `Location` are compared |

## How the upstream inventory was enumerated

Run from `parity/passport/node` against the installed, pinned packages — no
README, no memory:

```sh
# module exports and prototype methods
node -e "const p=require('passport');console.log(Object.keys(p).sort().join(' '));\
console.log(Object.getOwnPropertyNames(Object.getPrototypeOf(p)).sort().join(' '))"
node -e "console.log(Object.getOwnPropertyNames(require('passport/lib/http/request')).sort().join(' '))"
node -e "console.log(Object.getOwnPropertyNames(require('passport-strategy').prototype))"
node -e "console.log(Object.keys(require('passport-local')).join(' '))"
node -e "console.log(Object.keys(require('passport-http')).join(' '))"
node -e "console.log(Object.keys(require('passport-http-bearer')).join(' '))"

# option keys, read off the installed sources (they are plain property reads)
grep -ohE 'options\.[A-Za-z_]+' node_modules/passport/lib/middleware/authenticate.js \
  node_modules/passport/lib/sessionmanager.js node_modules/passport/lib/http/request.js | sort -u
grep -ohE 'options\.[A-Za-z_]+' node_modules/passport-local/lib/strategy.js | sort -u
grep -ohE 'options\.[A-Za-z_]+' node_modules/passport-http/lib/passport-http/strategies/basic.js | sort -u
grep -ohE 'options\.[A-Za-z_]+' node_modules/passport-http/lib/passport-http/strategies/digest.js | sort -u
grep -ohE 'options\.[A-Za-z_]+' node_modules/passport-http-bearer/lib/strategy.js | sort -u

# the five action callbacks passport grafts onto a strategy for one attempt
grep -nE 'strategy\.(success|fail|redirect|pass|error) =' node_modules/passport/lib/middleware/authenticate.js
```

The Go side was enumerated with

```sh
GOWORK=off go doc -short github.com/malcolmston/passport
GOWORK=off go doc -short github.com/malcolmston/passport/strategies/{local,basic,digest,bearer}
GOWORK=off go doc -all github.com/malcolmston/passport   # for struct fields
```

## Inventory — `passport@0.7.0` core

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `passport.Authenticator` | `passport.Passport` / `passport.New` | match | all 74 | |
| `passport.Passport` (alias of `Authenticator`) | `passport.New` | match | all 74 | |
| `passport.Strategy` (abstract base) | `passport.Strategy` (interface) | match | all 74 | upstream inherits, the port satisfies an interface |
| `passport.strategies` (`{SessionStrategy}`) | — | missing | — | the port folds session restore into `Passport.Session()`; there is no addressable session *strategy* |
| `Authenticator#use` | `Passport.Use` / `Passport.UseNamed` | match | all 74 | |
| `Authenticator#unuse` | `Passport.Unuse` | untested | — | no case removes a registered strategy |
| `Authenticator#framework` | — | missing | — | the port is `net/http`-only, there is no framework adapter seam |
| `Authenticator#init` | — | missing | — | internal to `framework()` |
| `Authenticator#initialize` | `Passport.Initialize` | match | all 74 | |
| `Authenticator#authenticate` | `Passport.Authenticate` | **differs** | `local-success-redirect-session-default`, `session-default-options-followup` | **`Options.Session` default bug**, see Divergences #1 |
| `Authenticator#authorize` | — | missing | — | no account-linking flow in the port |
| `Authenticator#session` | `Passport.Session` | match | `session-login-then-whoami`, `session-login-then-protected`, `session-forged-cookie`, … | |
| `Authenticator#serializeUser` | `Passport.SerializeUser` | match | `session-login-then-whoami`, `session-logout-revokes` | |
| `Authenticator#deserializeUser` | `Passport.DeserializeUser` | match | `session-login-then-whoami`, `session-forged-cookie`, `session-tampered-cookie-whoami` | |
| `Authenticator#transformAuthInfo` | — | missing | — | the port has no `authInfo` concept |
| `req.logIn` / `req.login` | `Passport.LogIn` | match | `session-login-then-protected`, `session-fixation-id-rotates` | both regenerate the session id on login |
| `req.logOut` / `req.logout` | `Passport.LogOut` | **differs** | `session-logout-revokes` | see Divergences #6 |
| `req.isAuthenticated` | `passport.IsAuthenticated` | match | every `session-*` case, `/whoami` | |
| `req.isUnauthenticated` | — | missing | — | trivially `!IsAuthenticated` |

### Strategy action callbacks (grafted onto the strategy per attempt)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `strategy.success(user, info)` | `Context.Success` | match | every successful case | the port drops `info` on the floor at the `Authenticate` level |
| `strategy.fail(challenge, status)` | `Context.Fail` | **differs** | `local-wrong-password`, `local-unknown-user`, `local-empty-password`, `local-empty-username`, `local-both-empty`, `local-no-body-at-all`, `local-json-body-wrong` | see Divergences #2 |
| `strategy.redirect(url, status)` | `Context.Redirect` | untested | — | only reachable from remote-IdP strategies, deliberately out of scope |
| `strategy.pass()` | `Context.Pass` | untested | — | no compared strategy declines |
| `strategy.error(err)` | `Context.Error` | untested | — | no compared verify function raises |

### `authenticate()` option keys

| upstream option | Go field | status | cases | note |
| --- | --- | --- | --- | --- |
| `session` | `Options.Session` | **differs** | `local-success-redirect-session-default`, `session-default-options-followup` | Divergences #1 |
| `successRedirect` | `Options.SuccessRedirect` | match | `local-success-redirect-session-default`, `session-default-options-followup` | status 302 and `Location` agree in both cases; those cases fail only on the `session` dimension above |
| `failureRedirect` | `Options.FailureRedirect` | match | `local-failure-redirect` | |
| `failureMessage` | `Options.FailureMessage` | untested | — | semantics already known to differ: upstream appends to `req.session.messages`, the port writes the challenge as the response body |
| `successReturnToOrRedirect` | — | missing | — | |
| `successFlash` | — | missing | — | |
| `successMessage` | — | missing | — | |
| `failureFlash` | — | missing | — | |
| `failWithError` | — | missing | — | |
| `assignProperty` | — | missing | — | |
| `authInfo` | — | missing | — | |
| `keepSessionInfo` | — | missing | — | the port always regenerates and never carries session data across login |
| `key` | — | missing | — | the port hard-codes the session key `passport.user` |

## Inventory — `passport-local@1.0.0`

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `passport-local.Strategy` | `local.New` | match | `local-correct-password`, `local-correct-password-bob`, `local-wrong-password-cross-user`, `local-json-body`, `local-query-credentials`, `local-nosession-success` | |
| `options.usernameField` | `local.Strategy.UsernameField` | untested | — | both default to `username`; no case renames it |
| `options.passwordField` | `local.Strategy.PasswordField` | untested | — | both default to `password` |
| `options.passReqToCallback` | `local.NewWithRequest` | untested | — | |
| `authenticate options.badRequestMessage` | — | missing | — | the port hard-codes `Missing credentials` |
| credential lookup (`req.body` ∪ `req.query`) | `local.Strategy.credentials` | **differs** | `local-json-nonstring-password` | Divergences #7 |

## Inventory — `passport-http@0.3.0` — `BasicStrategy`

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `passport-http.BasicStrategy` | `basic.New` | match | `basic-valid`, `basic-valid-bob`, `basic-wrong-password`, `basic-unknown-user`, `basic-empty-password`, `basic-empty-username`, `basic-extra-colons`, `basic-wrong-scheme`, `basic-scheme-case-insensitive`, `basic-no-credentials` | |
| `options.realm` | `basic.Strategy.Realm` | match | `basic-realm-challenge-admin`, `basic-wrong-realm-route` | Basic carries no realm binding, so credentials minted for one realm are accepted at another by *both* implementations |
| `options.passReqToCallback` | — | missing | — | |
| header parsing | `basic.parseBasicAuth` | **differs** | `basic-missing-colon`, `basic-not-base64`, `basic-malformed-scheme-only`, `basic-malformed-no-scheme` | Divergences #3 |
| `BasicStrategy#_challenge` | `basic.Strategy.Authenticate` challenge | match | `basic-no-credentials`, `basic-realm-challenge-admin` | byte-identical `Basic realm="…"` |

## Inventory — `passport-http@0.3.0` — `DigestStrategy`

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `passport-http.DigestStrategy` | `digest.New` | match | `digest-correct-response`, `digest-correct-response-noqop`, `digest-wrong-response`, `digest-empty-response`, `digest-no-username`, `digest-wrong-scheme`, `digest-uri-bound-to-other-path`, `digest-replay-incremented-nc` | MD5 + `qop=auth` and the RFC 2069 no-`qop` form both agree |
| `secret` callback | `digest.Options.Secret` | match | `digest-correct-response`, `digest-unknown-user` | shape differs and is normalised away: upstream's `done(err, user, password)` yields a *user object* and also accepts `{ha1}`; the port returns only a password string and authenticates the bare **username** |
| `validate` callback (nonce / cnonce / nc / opaque) | — | **missing** | `digest-replay-same-nonce-nc` | **Security finding #1** — the port has no replay hook at all |
| `options.realm` | `digest.Options.Realm` | **differs** | `digest-wrong-realm` | Divergences #5 |
| `req.url === creds.uri` binding | — | **missing** | `digest-uri-replayed-at-other-path` | **Security finding #2** |
| `options.domain` | — | missing | — | not emitted in the port's challenge |
| `options.opaque` | — | missing | — | |
| `options.algorithm` (`MD5` / `MD5-sess`) | — | missing | — | the port is MD5-only and ignores `creds.algorithm`; an `MD5-sess` header is silently treated as MD5 |
| `options.qop` (`auth` / `auth-int`) | — | missing | — | the port hard-codes `qop="auth"` in its challenge and never rejects `auth-int` |
| `DigestStrategy#_challenge` | `digest.Strategy.challenge` | match | `digest-no-credentials` | identical once the nonce is normalised (with upstream configured `{realm, qop:'auth'}`) |

## Inventory — `passport-http-bearer@1.0.1`

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `passport-http-bearer.Strategy` | `bearer.New` | match | `bearer-valid-token`, `bearer-valid-token-bob`, `bearer-invalid-token`, `bearer-wrong-scheme`, `bearer-basic-scheme`, `bearer-scheme-case-insensitive`, `bearer-no-credentials`, `bearer-query-access-token`, `bearer-query-access-token-invalid`, `bearer-form-access-token` | |
| `options.realm` | `bearer.Strategy.Realm` | match | `bearer-no-credentials`, `bearer-invalid-token` | |
| `options.scope` | — | missing | — | no `scope="…"` in the port's challenge |
| `options.passReqToCallback` | — | missing | — | |
| verify `info` / scope third argument | — | missing | — | `VerifyFunc` returns only a user, so `error_description` is never produced |
| token extraction + multi-location guard | `bearer.extractToken` | **differs** | `bearer-two-token-locations`, `bearer-missing-scheme`, `bearer-empty-token`, `bearer-three-part-header` | **Security finding #3** and Divergences #4 |
| `Strategy#_challenge` | `bearer.Strategy.Authenticate` challenge | match | `bearer-invalid-token` | `Bearer realm="…", error="invalid_token"` matches byte for byte |

## Untested — requires a live remote identity provider

These are deliberately **not faked**. Each needs a real IdP (or a fabricated mock
whose answers would tell us nothing about parity), so no case exists and the port
is neither credited nor blamed for them here.

| mechanism | upstream package(s) | Go package(s) in the port | status |
| --- | --- | --- | --- |
| OAuth 1.0a | `passport-oauth1` | `strategies/oauth1`, `strategies/oauth1twitter` | untested — requires live remote IdP |
| OAuth 2.0 (authorization code) | `passport-oauth2` + ~130 provider packages | `strategies/oauth2` + ~130 provider packages | untested — requires live remote IdP |
| OpenID Connect | `passport-openidconnect` | `strategies/openidconnect`, `strategies/jwks`, `strategies/googleidtoken` | untested — requires live remote IdP |
| SAML | `passport-saml` | `strategies/saml`, `saml/` | untested — requires live remote IdP |
| LDAP | `passport-ldapauth` | `strategies/ldap` | untested — requires live remote IdP (directory server) |
| CAS | `passport-cas` | `strategies/cas` | untested — requires live remote IdP |
| WebAuthn / FIDO2 | `passport-fido2-webauthn` | `strategies/webauthn`, `webauthn/` | untested — requires a live authenticator/attestation ceremony |

## Go-only surface (`extra`)

| Go symbol | status | cases | note |
| --- | --- | --- | --- |
| `passport.Chain` | extra | all 74 | upstream composes with express `app.use`; the port needs an explicit combinator |
| `passport.Passport.RequireLogin` | extra | `session-login-then-protected`, `session-protected-without-cookie`, `session-tampered-cookie-protected`, `session-forged-cookie`, `basic-no-credentials-protected-route`, `session-logout-revokes` | compared against upstream's documented `req.isAuthenticated` guard, which the node runner writes by hand |
| `passport.Store`, `passport.MemoryStore`, `passport.NewMemoryStore`, `passport.Passport.SetStore` | extra | every `session-*` case | upstream delegates all storage to `express-session` |
| `passport.Passport.SecureCookies`, `passport.DefaultCookieName` | extra | — | upstream configures cookies through `express-session` |
| `passport.Options.FailureStatus` | extra | — | no upstream counterpart; untested |
| `passport.Context`, `passport.Result` (+ `ResultNone…ResultPass`) | extra | all 74 indirectly | the port promotes the per-attempt callback object to a named exported type |
| `passport.Named`, `passport.Authenticator`, `passport.OAuth2Provider` | extra | — | capability interfaces with no upstream analogue |
| `passport.Middleware`, `passport.SerializeFunc`, `passport.DeserializeFunc` | extra | all 74 indirectly | named function types for what upstream passes as bare callbacks |
| `local.VerifyFunc`, `local.VerifyFuncReq`, `basic.VerifyFunc`, `bearer.VerifyFunc` | extra | all strategy cases | |
| `local.ErrInvalidCredentials`, `basic.ErrInvalidCredentials`, `bearer.ErrInvalidToken` | extra | — | sentinels replacing upstream's `done(null, false)` convention |

## Divergences observed

Ordered security-first. "Port authenticates, upstream rejects" is the only class
the harness labels `SECURITY`; it is detected mechanically in `parity_test.go`
(`portAuthenticatesWhereUpstreamRejects`) and recorded in
`parity.json → securityFindings`.

### Security findings — the port authenticates a request upstream refuses

1. **`digest-replay-same-nonce-nc`** — the identical `(nonce, nc)` pair replayed.
   Upstream: request 1 → 200 alice, request 2 → **401** + fresh challenge (its
   `validate` callback sees the spent pair). Port: **200 alice both times**. The
   port's digest strategy exposes no nonce/nc/opaque bookkeeping hook whatsoever,
   so this is not a misconfiguration — it cannot be closed from the outside. Its
   own package doc concedes the point. **Replay protection is absent.**
2. **`digest-uri-replayed-at-other-path`** — an `Authorization` header captured
   for `/digest` (`uri="/digest"`) replayed against `/digest/other`. Upstream:
   **400** (`req.url !== creds.uri`). Port: **200 alice**. The port hashes
   `creds.uri` into HA2 but never compares it to the actual request path, so one
   captured digest header authenticates *every* route protected by the strategy.
3. **`bearer-two-token-locations`** — `Authorization: Bearer tok-alice` *and*
   `?access_token=tok-alice` on the same request. Upstream: **400** (RFC 6750
   forbids multiple token locations, and the guard exists to stop an attacker
   smuggling a second token past a proxy or WAF that only inspects one of them).
   Port: **200 alice**, silently preferring the header.

### Other divergences (both sides reject, or the port is stricter)

4. **Upstream's `400 Bad Request` vs the port's `401` + challenge for malformed
   credentials.** Upstream distinguishes "your header is syntactically broken"
   (400, no `WWW-Authenticate`) from "authenticate yourself" (401 + challenge);
   the port collapses both into 401 + challenge. Cases:
   `basic-missing-colon`, `basic-not-base64`, `basic-malformed-scheme-only`,
   `basic-malformed-no-scheme`, `digest-malformed-scheme-only`,
   `bearer-empty-token`, `bearer-missing-scheme`, `bearer-three-part-header`.
   Both reject in every one; only the status and header differ.
5. **`digest-wrong-realm` — the port is *stricter* than upstream.** A response
   digest computed under a client-chosen `realm="Wrong"`. Upstream derives
   `HA1` from `creds.realm` (the *client's* value) and therefore **authenticates**
   alice; the port derives `HA1` from its own configured realm and rejects with
   401. This is an upstream weakness the port does not reproduce — the harness
   still counts it as a mismatch, correctly, because the two disagree.
6. **`req.logOut` cookie handling** (`session-logout-revokes`). Upstream
   regenerates the session on logout and therefore emits a **new** session cookie
   (`setCookiePresent: true`); the port only emits an expiring `MaxAge=-1` cookie
   (`setCookiePresent: false`). Both correctly refuse the old cookie afterwards —
   the fourth request in the sequence is 401 on both sides.
7. **`local-json-nonstring-password`** — `{"username":"alice","password":12345}`.
   Upstream passes the number through to the verify callback, which rejects it →
   **401**. The port's JSON decoding type-asserts to `string`, gets `""`, and
   short-circuits to **400 Missing credentials**. Both reject; the port never
   reaches its verify function, so an application that logs failed verifications
   would see nothing.
8. **`WWW-Authenticate` leaking non-HTTP-auth challenge text.** The port's
   `Authenticate` sets `WWW-Authenticate: <challenge>` whenever the challenge
   string is non-empty and regardless of status, so a form login answers
   `400` with `WWW-Authenticate: Missing credentials` and `401` with
   `WWW-Authenticate: Invalid credentials` — neither is a valid challenge, and
   the second discloses that the username exists. Upstream only sets the header
   when the status is 401 *and* the challenge is a string (`passport-local`
   passes an object, so no header is emitted). Cases: `local-wrong-password`,
   `local-wrong-password-cross-user`, `local-unknown-user`,
   `local-empty-password`, `local-empty-username`, `local-both-empty`,
   `local-no-body-at-all`, `local-json-body-wrong`,
   `session-failed-login-no-session`.
9. **`Options.Session` default bug** (`local-success-redirect-session-default`,
   `session-default-options-followup`). `Passport.Authenticate` does
   `merged := opts[0]; o = &merged`, replacing the `defaultOptions()` value
   wholesale, so `Options{SuccessRedirect: "/ok"}` carries `Session: false`.
   Upstream defaults `session: true`. Observed: upstream sets a session cookie on
   the 302 and the follow-up request is authenticated as alice; the port sets no
   cookie and the follow-up is anonymous. This is a **silent availability/logic
   bug** rather than a security hole — it fails closed — but it means the
   idiomatic upstream call `authenticate('local', {successRedirect: '/'})` ports
   to a login that never logs anyone in.

### Things the port lacks outright (no upstream equivalent shipped)

* digest replay protection (`validate`), URI binding, `opaque`, `domain`,
  `MD5-sess`, configurable `qop`;
* bearer `scope` and the verify `info` object (hence no
  `error_description` in challenges);
* `passReqToCallback` on basic and bearer (local has it, as `NewWithRequest`);
* `authorize()`, `transformAuthInfo()`, `req.isUnauthenticated()`, the framework
  adapter seam, and the addressable `SessionStrategy`;
* nine of the thirteen `authenticate()` options: `successReturnToOrRedirect`,
  `successFlash`, `successMessage`, `failureFlash`, `failWithError`,
  `assignProperty`, `authInfo`, `keepSessionInfo`, `key`;
* `badRequestMessage` on local.

## Score

Counted over the 71 upstream inventory rows above (64 compared or absent, plus 7
remote-IdP mechanisms), and separately over the 74 cases.

| bucket | count |
| --- | --- |
| match | 22 |
| differs | 8 |
| missing | 26 |
| untested | 15 (8 in-scope + 7 requiring a live remote IdP) |
| **upstream rows total** | **71** |
| extra (Go-only) | 24 |

**Symbol parity = 22 / (22 + 8) = 73.33 %** over the symbols actually compared.

**Case parity = 49 / 74 = 66.22 %** (0 declared deviations; see `parity.json`,
which the test rewrites on every run).

Of the 25 mismatching cases, **3 are security findings** where the port
authenticates a request upstream refuses (findings #1–#3 above), **1 is the port
being stricter than upstream** (#5), and the remaining 21 are status-code,
challenge-header or session-cookie differences in which both implementations
agree on rejecting — or, for #9, on succeeding without the session upstream
would have created.

---

## Nested packages — triage

The passport submodule is not one port but several: some of its nested packages
are ports of *distinct* upstream npm projects, and per `parity/HARNESS.md` each
of those gets its own harness under `parity/passport/nested/<pkg>/`, with its own
pinned oracle. The rest are internal decomposition of this port with no
independent counterpart, and are covered here rather than given a harness of
their own.

| nested package | upstream | verdict |
| --- | --- | --- |
| `httpauth` | `http-auth-utils@7.0.1` | **own harness** — `nested/httpauth/`. Parses and builds `Authorization` / `WWW-Authenticate` headers for Basic, Bearer and Digest, and computes the RFC 7616 response digest. `http-auth-utils` is exactly that package upstream. 145 cases, 114 compared, 100% |
| `pkce` | `pkce-challenge@6.0.0` | **own harness** — `nested/pkce/`. RFC 7636 verifier/challenge derivation and verification. 49 cases, 42 compared, 100% |
| `otpauth` | `otpauth@9.5.1` (npm) | **own harness** — `nested/otpauth/`. The npm package of the same name owns both the `otpauth://` key-URI format and the HOTP/TOTP construction. 107 cases, 99 compared, 100% |
| `pwhash` | `pbkdf2@3.1.6` | **own harness** — `nested/pwhash/`. The package implements PBKDF2 (RFC 2898), verified against the RFC 6070 vectors — *not* bcrypt, scrypt or argon2 — so the oracle is a PBKDF2 implementation. 62 cases, 57 compared, 100% |
| `token` | — | **internal decomposition, no harness.** Random-token generation (`crypto/rand`), SHA-256 digesting and constant-time comparison, factored out of the bearer, API-key, magic-link and remember-me strategies which each used to roll their own. There is no npm package it is a port of: the closest counterparts are Node's own `crypto.randomBytes` / `crypto.timingSafeEqual`, which are the platform, not a library. Its generation surface is also non-deterministic by construction. Covered by the parent harness through the strategies that consume it (`bearer-*` cases) and by `token/token_test.go` |
| `scope` | — | **internal decomposition, no harness.** An ordered set type for OAuth 2.0 scope strings. It centralises the scope splitting/joining that the `strategies/oauth2` provider presets used to do inline (the `scopeSeparator` knob `passport-oauth2` exposes). No npm package publishes this as a library — searched: `oauth-scope`, `oauth2-scope`, `scope-parser` are all unpublished — and the behaviour it mirrors lives inside `passport-oauth2`, not beside it. Covered by `scope/scope_parity_test.go` and `scope/scope_security_test.go` |
| `oauthstate` | — | **internal decomposition, no harness.** CSRF `state` issuing and verification for the authorization-code flow. The concept mirrors `passport-oauth2`'s state store, but that store is not a published package (`passport-oauth2/lib/state/session`, not an export), its state values are opaque session-backed UUIDs, and the port's stateless `HMACStore` envelope has no upstream at all — there is nothing whose *answers* could be compared. Covered by `oauthstate/oauthstate_test.go` and `oauthstate/sweep_test.go` |
| `strategies` | — | **not a single port, no single harness.** `strategies/` is a directory of ~200 individual strategy packages, each the port of a *different* `passport-*` npm module. There is no one upstream for the tree. The four with official upstreams already have oracle-backed coverage in this harness (`passport-local`, `passport-http` Basic and Digest, `passport-http-bearer`); the rest are provider presets that need a live remote identity provider (see "Untested — requires a live remote identity provider" above) |
| `interop` | — | **not a library, no harness.** `interop/` is `package main`: a two-line command that signs an HS256 token for Node's `jsonwebtoken` to verify and verifies one Node produced. It is itself a cross-ecosystem check, already asserted against checked-in `jsonwebtoken@9` vectors in `interop/jwt_interop_test.go` |

Each nested harness is a **peer** of this one, never a replacement:
`parity/passport/parity.json` stays the score for passport, and
`parity/passport/nested/<pkg>/parity.json` is the score for that package's port.
