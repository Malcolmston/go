# passport example

A single runnable program that exercises
[`github.com/malcolmston/passport`](https://github.com/Malcolmston/passport)
against an in-process `net/http/httptest` server.

- **Module under test:** `github.com/malcolmston/passport`
- **Resolved version:** `v0.3.0` (a real semver tag, not a pseudo-version;
  `go get github.com/malcolmston/passport@latest` selected it)
- No `replace` directive: the library is consumed exactly as a published module.
- No outbound network calls, no external identity provider, and the program
  terminates on its own (exit 0 when every assertion holds).

## Run

```sh
cd examples/passport
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

Output is a numbered list of scenarios; each line is `ok`, `FAIL`, or `BUG`
(a documented library defect the example records rather than asserts). The
program currently prints `65 checks, 0 failed, 5 library bugs recorded`.

## What it demonstrates

| Scenario | API used |
| --- | --- |
| Strategy registration, and two instances of the same strategy type | `Use`, `UseNamed` |
| Username/password login: success, wrong password, unknown user, missing field, JSON body | `strategies/local`, `local.ErrInvalidCredentials`, `UsernameField`/`PasswordField` |
| Session bridge and a protected route | `SerializeUser`, `DeserializeUser`, `Initialize`, `Session`, `RequireLogin`, `User`, `IsAuthenticated`, `Chain` |
| Unauthenticated request rejected (401) and redirected (302) | `RequireLogin("")` / `RequireLogin("/login")` |
| Forged session id does not authenticate | `MemoryStore`, `DefaultCookieName` |
| Session id rotates on every login (fixation defence) | `LogIn` |
| Logout | `LogOut` |
| HTTP Basic, plus header encode/parse/challenge helpers | `strategies/basic`, `httpauth.EncodeBasic`/`ParseBasic`/`BasicChallenge`/`SchemeOf`/`HasScheme`/`ErrWrongScheme` |
| HTTP Digest challenge/response, wrong password, unknown user, replay | `strategies/digest` (client side hand-rolled, see holes) |
| Opaque bearer tokens via header and `?access_token=` | `strategies/bearer`, `httpauth.EncodeBearer`, `token` |
| JWT HS256: valid token plus 7 forgery/expiry attempts and out-of-band `Parse` | `strategies/jwt`, `jwt.Sign`, `jwt.Claims`, `jwt.ErrSignature`, `Leeway` |
| TOTP second factor incl. skew window | `strategies/totp`, `totp.Generate`, `totp.Step` |
| Multi-strategy and optional auth | `AuthenticateAny`, `strategies/anonymous` |
| Custom callback login | `AuthenticateCallback` |
| Unregistered strategy name | `Authenticate("saml")` → 500 |
| Stateless routes create no session | `Options{Session: false}` |
| Password hashing and token generation | `pwhash.Hash`/`Verify`/`Decode`, `token.New`/`Equal`/`Numeric`/`Hex` |

## Holes found

### Security

1. **`Options.Session` does not default to `true`, contrary to its own
   documentation.** `Authenticate` does
   `o := defaultOptions(); if len(opts) > 0 { merged := opts[0]; o = &merged }`,
   which throws the defaults away, so any `Options{...}` literal that does not
   spell out `Session: true` silently creates **no** login session — the user
   appears authenticated for that one request and is anonymous afterwards. The
   library's own `doc.go` and `example_test.go` login routes pass only
   `SuccessRedirect`/`FailureRedirect` and therefore never log anybody in.
   Scenario 18 demonstrates it; every session route in this example must say
   `Session: true` explicitly.
2. **HTTP Digest has no replay protection.** `strategies/digest` in v0.3.0 has no
   hook for remembering issued nonces or `(nonce, nc)` pairs, so a captured
   `Authorization: Digest ...` header can be replayed forever by anyone who
   observes it (scenario 11). It also never checks that the client-supplied
   `uri` parameter matches the request target, so a header captured for one path
   authenticates a request to another.
3. **JWT has no `exp`-required, issuer, or audience validation.** v0.3.0's
   `jwt.Strategy` exposes only `Secret` and `Leeway`. A token with no `exp`
   claim is accepted indefinitely and cannot be revoked, and a token minted for
   a different service is equally acceptable here (scenario 13). Signature,
   `exp`, `nbf`, and `alg != HS256` (including the `alg: none` downgrade) are
   all correctly rejected.

### Missing / absent APIs

4. **The published module is far behind the repo working tree.** Several APIs
   that exist in the local `passport/` folder are absent from `v0.3.0`, so an
   outside user cannot use them:
   - `httpauth` has **no digest support at all** (`DigestResponse`,
     `DigestChallenge`, `ParseDigest`, `VerifyDigest`, `ParseParams`,
     `QuoteString`, `SecureCompare`, `DigestParams`, the `Alg*` constants). This
     example therefore hand-rolls the MD5 digest client with `crypto/md5`.
   - No layered serializer API (`serde.go`: `AddSerializer`, `AddDeserializer`,
     `AddInfoTransformer`, `Serialize`, `Deserialize`, `TransformAuthInfo`).
   - `Passport` has no `CookieName` or `SameSite` setter — the cookie name is
     fixed to `passport.sid`.
   - `jwt`: no `Issuer`, `Audience`, `RequireExpiry`, `Extractor`
     (`FromHeader`, `FromURLQueryParameter`, …), `Claims.Issuer`,
     `Claims.Audience`, `ErrIssuer`, `ErrAudience`.
   - `bearer`: no `Scope`; `digest`: no `ValidateNonce`; `token`: no `Hash`,
     `Verify`, `Base32`, `EqualBytes`; `pwhash`: no `NeedsRehash`.
5. **No way to look up a registered strategy.** `*Passport` has no
   `Strategy(name)`/`Strategies()` accessor, so calling something like
   `jwt.Strategy.Parse` outside the middleware means holding a second reference
   to the strategy value from wiring time.
6. **`digest` is the only credential strategy without a verify callback.** It
   reports `c.Success(username)` — a bare `string` — so `passport.User(r)` yields
   a username instead of the application's user object, unlike `local`, `basic`,
   `bearer`, and `jwt`. Applications must do a second lookup by hand.

### Wrong behaviour / README mismatches

7. **`AuthenticateAny` cannot express optional auth.** It treats a `Pass`
   outcome as a decline and always falls through to a failure response, so
   `AuthenticateAny([]string{"jwt", "anonymous"}, ...)` returns 401 rather than
   passing the request through unauthenticated — exactly the "try a real
   strategy, then fall back to anonymous" pattern the README advertises. The
   single-strategy `Authenticate("anonymous")` form does honour `Pass`
   (scenario 15).
8. **README strategy count is wrong**: it claims "**55 strategies** under
   `strategies/`" and "**20 OAuth2 providers**", while the published tree
   actually contains 154 strategy packages (and `doc.go` in the working tree
   says 104 / 67). The counts disagree with each other and with reality.
9. **The README's `Options` table states `Session` "create a login session on
   success (default `true`)"**, which is the documentation half of hole 1 — the
   code does not honour it once you pass an `Options` literal.
   (Checked and *not* holes: the README's WebAuthn section, `jwt.Sign` /
   `strategy.Parse`, and the JWKS RS256/ES256 claims all do exist in `v0.3.0`.)

### Packaging

10. **The published module ships eight Linux ELF binaries** at its root
    (`gen`, `hotp`, `local`, `magiclink`, `remembercookie`, `saml`, `totp`,
    `webauthn`), roughly 80 MB of build artifacts that every consumer downloads
    into their module cache. They are checked into git, not generated.
11. The module declares `go 1.23`, which is fine, and has **zero third-party
    dependencies** — that part of the pitch holds.

### Not exercised (out of scope for an offline example)

OAuth 1.0a, OAuth 2.0 (all ~70 provider presets), OpenID Connect, JWKS, LDAP,
CAS, SAML, and WebAuthn are **not** exercised: each needs a live remote identity
provider (authorization/token endpoints, IdP metadata, JWKS URL) or a browser
authenticator. Only their redirect leg could be observed offline; the callback
leg cannot be driven without a real peer.
