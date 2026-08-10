# `jwt` — upstream API inventory vs. the Go port

| | |
| --- | --- |
| upstream | `jsonwebtoken@9.0.2` (`auth0/node-jsonwebtoken`), installed in `parity/jwt/node/` |
| port | `github.com/malcolmston/jwt@v0.3.0` (published module, no `replace`) |
| harness | `GOWORK=off go test ./parity/jwt/` |
| cases | 155 (152 compared + 3 deviations) — see `parity.json` |
| case parity | **141 / 152 = 92.76 %** |
| symbol parity | **31 / 40 = 77.50 %** (see the count at the bottom) |

Fixed keys live in `keys/` (generated once with `openssl genpkey`, committed, never
regenerated at run time). Every verification case passes an explicit `clockTimestamp`,
mapped to upstream's `options.clockTimestamp` and to the port's `jwt.WithTimeFunc`, so
`exp`/`nbf`/`iat`/`maxAge` never depend on wall time. `cases/generate.py` regenerates the
case files, including the committed cross-verification fixture tokens.

## How the upstream inventory was derived

Mechanically, from the installed package — not from the README and not from memory. Run
from `parity/jwt/node/`:

```sh
# 1. top-level exports
node -e "const j=require('jsonwebtoken');console.log(Object.keys(j).sort().join('\n'))"

# 2. own properties of each error class
node -e "
const j=require('jsonwebtoken');
for (const n of ['JsonWebTokenError','NotBeforeError','TokenExpiredError']) {
  const C=j[n];
  const e = n==='TokenExpiredError'?new C('m',new Date(0))
          : n==='NotBeforeError'   ?new C('m',new Date(0))
          :                         new C('m',new Error('i'));
  console.log(n, JSON.stringify(Object.getOwnPropertyNames(e).sort()));
}"

# 3. sign() option keys — read straight off sign_options_schema
node -e "
const s=require('fs').readFileSync('node_modules/jsonwebtoken/sign.js','utf8');
const b=s.match(/sign_options_schema = \{([\s\S]*?)\n\};/)[1];
console.log([...b.matchAll(/^\s{2}([A-Za-z]+):/gm)].map(m=>m[1]).sort().join('\n'))"

# 4. verify() option keys — every options.<key> reference in verify.js
node -e "
const s=require('fs').readFileSync('node_modules/jsonwebtoken/verify.js','utf8');
console.log([...new Set([...s.matchAll(/options\.([A-Za-z]+)/g)].map(m=>m[1]))].sort().join('\n'))"

# 5. decode() option keys (decode.js plus the jws stream it delegates to)
node -e "
const d=require('fs').readFileSync('node_modules/jsonwebtoken/decode.js','utf8');
const w=require('fs').readFileSync('node_modules/jws/lib/verify-stream.js','utf8');
console.log([...new Set([...d.matchAll(/options\.([A-Za-z]+)/g),
                         ...w.matchAll(/options\.([A-Za-z]+)/g)].map(m=>m[1]))].sort().join('\n'))"

# 6. supported algorithms
node -e "console.log(require('jws').ALGORITHMS.join(' '))"
```

That yields exactly 6 exports, 4 + 4 + 4 error properties, 14 sign option keys, 13 verify
option keys, 1 decode option key, and 12 algorithms (plus `none`, which `jsonwebtoken`
handles explicitly in `verify.js` but `jws.ALGORITHMS` omits). The Go side was enumerated
with:

```sh
GOWORK=off go doc -all github.com/malcolmston/jwt
```

## Security findings

> **The port accepts a token upstream rejects — twice.** Both are missing *defence-in-depth*
> checks, not signature-verification bugs: in each case the port did verify the MAC
> correctly. But both let a token through that upstream stops.

### 1. No algorithm inference from key material (`alg-confusion-hs-token-pem-secret-no-allowlist`)

The classic algorithm-confusion attack. An attacker takes the RSA **public** key PEM the
service publishes, uses its bytes as an HMAC secret, and signs `{"alg":"HS256"}`. A verifier
that does the naive thing — hand the parser the PEM bytes, set **no** algorithm allowlist —
gets opposite answers:

| | outcome |
| --- | --- |
| `jwt.verify(tok, pubPem, {})` | **rejects** — `invalid algorithm` |
| `jwt.Parse(tok, func(*Token)(any,error){return pemBytes,nil})` | **accepts**, claims `{"sub":"attacker",…}` |

`verify.js` calls `crypto.createPublicKey(secretOrPublicKey)`; when that succeeds it sets
`options.algorithms` to the RSA set, so an `HS256` header can never match. The port performs
no such inference: with no `WithValidMethods`, any registered `alg` in the header is honoured
and `SigningMethodHMAC` happily accepts the `[]byte` it was given.

Mitigated on the port by always passing `jwt.WithValidMethods([...])` — case
`alg-confusion-hs-token-rs-allowlist` shows both sides then reject. Also mitigated by
returning a typed `*rsa.PublicKey` from the `Keyfunc` rather than raw bytes
(`alg-confusion-hs-token-rsa-pubkey-no-allowlist`: both reject, the port via
`ErrInvalidKeyType`).

### 2. An empty algorithm allowlist is silently ignored (`alg-allowlist-empty`)

`jwt.WithValidMethods([]string{})` should permit nothing. Upstream's
`options.algorithms.indexOf(alg) === -1` rejects everything for `[]`; the port treats the
empty slice as "unset" and accepts an `HS256` token. A caller that computes its allowlist
(from config, a JWKS `alg` field, an intersection) and legitimately ends up with an empty
list gets *no* algorithm restriction instead of a closed door.

## Upstream exports

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `jwt.sign` | `jwt.Sign`, `jwt.NewWithClaims` + `Token.SignedString` | match | `sign-hs256-exact`, `sign-hs384-exact`, `sign-hs512-exact`, `sign-hs256-minimal-exact`, `sign-hs256-notimestamp-exact`, `sign-hs256-default-alg-exact`, `sign-none-exact`, `sign-hs256-unknown-alg`, `parts-*`, `opt-*` | HS\*/`none` tokens are **byte-identical** when the claim keys are alphabetical; see the ordering deviation below |
| `jwt.verify` | `jwt.Parse`, `jwt.ParseWithClaims` | differs | `cross-*`, `vc-*`, `rej-*`, `alg-*` (86 cases) | signature and claim semantics agree on all 12 shared algorithms and on the whole rejection surface; diverges on the two security findings above |
| `jwt.decode` | `jwt.ParseUnverified` | differs | `dec-payload`, `dec-complete`, `dec-unverified-bad-signature`, `dec-no-signature`, `dec-alg-none`, `dec-garbage`, `dec-two-parts`, `dec-empty` | upstream returns `null` for a token it cannot decode; the port returns an error. Fail-closed, so not a security concern, but callers that branch on `null` need rewriting |
| `jwt.JsonWebTokenError` | `jwt.ErrInvalidToken`, `ErrTokenMalformed`, `ErrSignatureInvalid`, `ErrTokenUnverifiable`, `ErrInvalidKeyType`, `ErrTokenInvalidAudience`, `ErrTokenInvalidIssuer`, `ErrTokenInvalidSubject`, `ErrNoneAlgDisallowed`, `ErrSigningMethodUnavailable` | differs | all `rej-*`, `alg-*` | a single JS class vs. a family of `errors.Is`-testable sentinels. Every message text differs (table below); the harness compares *whether* a call failed |
| `jwt.TokenExpiredError` | `jwt.ErrTokenExpired` | match | `vc-exp-expired`, `vc-clocktolerance-insufficient`, `rej-expired` | |
| `jwt.NotBeforeError` | `jwt.ErrTokenNotValidYet` | match | `vc-nbf-future`, `rej-not-yet-valid` | |

### Error object properties

The harness deliberately compares failure/success, never message text, so these are
`untested` by design. Recorded here as required.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `JsonWebTokenError.name` | — | untested | — | JS `Error` convention; the port uses sentinel identity instead |
| `JsonWebTokenError.message` | `error.Error()` | untested | — | see message table |
| `JsonWebTokenError.inner` | `errors.Unwrap` | untested | — | the port wraps sentinels, so `errors.Is` covers the same ground |
| `JsonWebTokenError.stack` | — | untested | — | JS-only |
| `TokenExpiredError.expiredAt` | (in the `ErrTokenExpired` message text) | untested | — | the port formats the expiry into the message rather than exposing a field |
| `NotBeforeError.date` | (in the `ErrTokenNotValidYet` message text) | untested | — | as above |

### Observed message differences

Same outcome, different text, in every case:

| case | upstream | port |
| --- | --- | --- |
| `rej-expired` | `jwt expired` | `jwt: token is expired: expired at 2023-11-14 23:13:20 +0000 UTC` |
| `rej-not-yet-valid` | `jwt not active` | `jwt: token is not valid yet: not before …` |
| `rej-wrong-key`, `rej-tampered-payload` | `invalid signature` | `jwt: signature is invalid` |
| `rej-two-parts` | `jwt malformed` | `jwt: token is malformed: expected 3 parts, got 2` |
| `rej-signature-stripped` | `jwt signature is required` | `jwt: signature is invalid` |
| `rej-empty-secret` | `secret or public key must be provided` | `jwt: signature is invalid` |
| `alg-allowlist-excludes` | `invalid algorithm` | `jwt: signature is invalid: alg "HS256" is not accepted` |
| `alg-confusion-hs-token-rsa-pubkey-object` | `secretOrPublicKey must be a symmetric key when using HS256` | `jwt: key is of invalid type: HMAC requires []byte, got *rsa.PublicKey` |
| `vc-aud-wrong` | `jwt audience invalid. expected: other` | `jwt: token has invalid audience: "other" not in [aud-a]` |
| `vc-iss-wrong` | `jwt issuer invalid. expected: other` | `jwt: token has invalid issuer: got "iss-a" want "other"` |
| `vc-maxage-exceeded` | `maxAge exceeded` | `jwt: token is older than the maximum allowed age: age 11m40s exceeds 5m0s` |
| `rej-none-with-secret` | `jwt signature is required` | `jwt: 'none' signature type is not allowed` |

## `jwt.sign` option keys (14)

The port has **no sign-time options at all**: `jwt.Sign` signs exactly the claims it is
handed. Where a row says `match`, the parity runner performs upstream's option→claim
projection (`expiresIn` → `exp`, `audience` → `aud`, …) and the resulting token is compared;
the port supplies the claim field or header setter named in the Go column.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `algorithm` | `jwt.GetSigningMethod`, `jwt.SigningMethod*` | match | `parts-*` (12 algs), `sign-hs256-default-alg-exact`, `sign-hs256-unknown-alg` | both default to HS256; both reject an unknown name |
| `expiresIn` | `jwt.MapClaims["exp"]`, `RegisteredClaims.ExpiresAt` | differs | `opt-expiresin-int`, `opt-all-claims`, `opt-expiresin-timespan`, `opt-expiresin-conflict` | integer seconds agree; upstream also accepts `ms` timespan strings (`"1h"`, `"2 days"`) with no port equivalent |
| `notBefore` | `jwt.MapClaims["nbf"]`, `RegisteredClaims.NotBefore` | differs | `opt-notbefore-int`, `opt-all-claims`, `opt-notbefore-timespan` | same timespan-string gap |
| `audience` | `jwt.ClaimStrings`, `RegisteredClaims.Audience` | match | `opt-audience-string`, `opt-audience-array`, `opt-all-claims` | string and array forms both round-trip |
| `issuer` | `jwt.RegisteredClaims.Issuer` | match | `opt-issuer`, `opt-all-claims` | |
| `subject` | `jwt.RegisteredClaims.Subject` | match | `opt-subject`, `opt-all-claims` | |
| `jwtid` | `jwt.RegisteredClaims.ID` | match | `opt-jwtid`, `opt-all-claims` | |
| `keyid` | `jwt.Token.SetKID` | match | `parts-keyid`, `opt-all-claims` | |
| `header` | `jwt.Token.SetHeader`, `Token.SetType` | match | `parts-header-extra`, `parts-header-typ` | |
| `noTimestamp` | omit `iat` from the claims | match | `sign-hs256-notimestamp-exact` | |
| `encoding` | — | missing | — | upstream's payload text encoding; the port always emits UTF-8 JSON |
| `mutatePayload` | — | missing | — | mutates the caller's JS object in place; meaningless for a Go value |
| `allowInsecureKeySizes` | — | missing | — | upstream can be told to sign with an RSA key under 2048 bits; the port has no size gate either way |
| `allowInvalidAsymmetricKeyTypes` | — | missing | — | upstream escape hatch for mismatched key types; the port always type-asserts (`ErrInvalidKeyType`) |

## `jwt.verify` option keys (13)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `algorithms` | `jwt.WithValidMethods` | differs | `alg-allowlist-match`, `alg-allowlist-multi`, `alg-allowlist-excludes`, `alg-allowlist-empty`, `alg-confusion-*`, `alg-rs256-*`, `alg-es256-token-es384-allowlist`, `alg-unknown-in-header`, plus every `rt-*`/`cross-*` | **SECURITY:** an empty allowlist is ignored by the port. Upstream also *infers* an allowlist from the key material when the option is absent; the port does not — see finding 1 |
| `audience` | `jwt.WithAudience` | differs | `vc-aud-ok`, `vc-aud-wrong`, `vc-aud-array-claim-member`, `vc-aud-array-claim-nonmember`, `vc-aud-missing-claim`, `vc-aud-option-array`, `rej-wrong-audience` | single-value agreement is exact, including matching a member of an array-valued `aud` claim. Upstream also accepts an **array** of acceptable audiences (and a RegExp); `WithAudience` takes one string |
| `issuer` | `jwt.WithIssuer` | differs | `vc-iss-ok`, `vc-iss-wrong`, `vc-iss-option-array`, `rej-wrong-issuer` | same array gap |
| `subject` | `jwt.WithSubject` | match | `vc-sub-ok`, `vc-sub-wrong` | |
| `jwtid` | — | missing | `vc-jwtid-ok`, `vc-jwtid-wrong` | the port has no `jti`-equality option. `WithRequiredClaims("jti")` only checks presence. `vc-jwtid-wrong` "matches" only because both sides fail — for different reasons |
| `nonce` | — | missing | `vc-nonce-wrong` | `MapClaims.GetNonce` reads the claim but no parser option compares it. Same accidental match caveat |
| `clockTolerance` | `jwt.WithLeeway` | match | `vc-clocktolerance-saves`, `vc-clocktolerance-insufficient`, `vc-nbf-tolerance` | symmetric, applied to `exp` and `nbf` identically on both sides |
| `clockTimestamp` | `jwt.WithTimeFunc`, `jwt.WithClock` | match | every `vc-*`, `rej-*`, `alg-*`, `rt-*`, `cross-*` case | the harness requires it |
| `maxAge` | `jwt.WithMaxTokenAge` | differs | `vc-maxage-ok`, `vc-maxage-exceeded`, `vc-maxage-timespan` | integer seconds agree, including making `iat` required; upstream also takes `ms` timespan strings |
| `ignoreExpiration` | `jwt.WithIgnoreExpiration` | match | `vc-ignoreexpiration` | |
| `ignoreNotBefore` | `jwt.WithIgnoreNotBefore` | match | `vc-ignorenotbefore` | |
| `complete` | `jwt.Token.Header` + `Token.Claims` | match | `vc-complete-false-shape` and every case with `complete:true` | |
| `allowInvalidAsymmetricKeyTypes` | — | missing | — | upstream escape hatch; the port always type-asserts |

## `jwt.decode` option keys (1)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `complete` | `jwt.Token.Header` + `Token.Claims` from `ParseUnverified` | match | `dec-payload`, `dec-complete` | the port has no `signature` field in the decoded result; not compared |

## Algorithms (13)

Every one is exercised three ways: in-process `roundtrip`, verify an
**upstream-minted** token in Go, and verify a **Go-minted** token upstream. HS\* is also
compared as an exact token string.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `HS256` | `jwt.SigningMethodHS256` | match | `sign-hs256-exact`, `parts-hs256`, `rt-hs256`, `rt-hs256-wrongkey`, `cross-upstream-signed-hs256`, `cross-go-signed-hs256` | byte-identical tokens |
| `HS384` | `jwt.SigningMethodHS384` | match | `sign-hs384-exact`, `parts-hs384`, `rt-hs384`, `rt-hs384-wrongkey`, `cross-*-hs384` | byte-identical |
| `HS512` | `jwt.SigningMethodHS512` | match | `sign-hs512-exact`, `parts-hs512`, `rt-hs512`, `rt-hs512-wrongkey`, `cross-*-hs512` | byte-identical |
| `RS256` | `jwt.SigningMethodRS256` | match | `parts-rs256`, `rt-rs256`, `rt-rs256-wrongkey`, `cross-upstream-signed-rs256`, `cross-go-signed-rs256`, `cross-rs256-tampered-payload`, `cross-rs256-wrong-public-key` | PKCS#1 v1.5 is deterministic, so the cross-signed tokens are also identical |
| `RS384` | `jwt.SigningMethodRS384` | match | `parts-rs384`, `rt-rs384`, `rt-rs384-wrongkey`, `cross-*-rs384` | |
| `RS512` | `jwt.SigningMethodRS512` | match | `parts-rs512`, `rt-rs512`, `rt-rs512-wrongkey`, `cross-*-rs512` | |
| `PS256` | `jwt.SigningMethodPS256` | match | `parts-ps256`, `rt-ps256`, `rt-ps256-wrongkey`, `cross-*-ps256` | PSS is randomised; compared by verification outcome + decoded claims + signature length, and cross-verified both directions. Salt length = hash length on both sides |
| `PS384` | `jwt.SigningMethodPS384` | match | `parts-ps384`, `rt-ps384`, `rt-ps384-wrongkey`, `cross-*-ps384` | |
| `PS512` | `jwt.SigningMethodPS512` | match | `parts-ps512`, `rt-ps512`, `rt-ps512-wrongkey`, `cross-*-ps512` | |
| `ES256` | `jwt.SigningMethodES256` | match | `parts-es256`, `rt-es256`, `rt-es256-wrongkey`, `cross-*-es256`, `alg-es256-token-es384-allowlist` | ECDSA is randomised; cross-verified both directions. Both use the fixed-width `r‖s` form (64 bytes), not ASN.1 DER |
| `ES384` | `jwt.SigningMethodES384` | match | `parts-es384`, `rt-es384`, `rt-es384-wrongkey`, `cross-*-es384` | 96-byte signature |
| `ES512` | `jwt.SigningMethodES512` | match | `parts-es512`, `rt-es512`, `rt-es512-wrongkey`, `cross-*-es512` | P-521, 132-byte signature — both sides pad each coordinate to 66 bytes |
| `none` | `jwt.SigningMethodNoneAlg` + `jwt.WithAllowNone` + `jwt.UnsafeAllowNoneSignatureType` | match | `sign-none-exact`, `dec-alg-none`, `rej-none-with-secret`, `rej-none-no-allowlist`, `rej-none-listed-but-key-given`, `acc-none-explicit-optin` | both refuse `alg:none` unless it is explicitly opted into, and both refuse it when real key material is supplied. The port requires a *second* opt-in (the `UnsafeAllowNoneSignatureType` sentinel as the key) |

## Deviations

Kept as cases, reported by the harness, counted separately from the 152 compared cases.
(The port's own `API-DEVIATIONS.md` lives in the library repo, which this task does not
touch.)

| case | deviation |
| --- | --- |
| `rt-eddsa` | the port implements `EdDSA`/Ed25519 (RFC 8037); `jsonwebtoken@9.0.2` (via `jws@3`) does not |
| `cross-go-signed-eddsa` | as above, from the cross-verification direction |
| `sign-hs256-insertion-order` | the port serialises the JOSE header and the claims with **sorted** keys (Go `encoding/json`), upstream preserves **insertion order**. Byte-identical tokens therefore require alphabetical claim order — which is why the exact-token cases use it and everything else compares decoded objects. Semantically irrelevant (each side verifies the other's tokens; see `crossverify`), but it means the port is not a drop-in producer of byte-stable tokens for a system that pinned upstream's output |

## Port-only surface (`extra`)

`go doc -all` reports 113 exported top-level symbols. Those with no upstream counterpart are
`extra`; none has a parity case, because there is nothing to compare them against, so each is
`extra` / `untested`:

`ComputeKeyID`, `DecodeSegment`, `EncodeSegment`, `GetAlgorithms`, `GetSigningMethod`,
`RegisterSigningMethod`, `UnregisterSigningMethod`, `SignDetached`, `VerifyDetached`
(RFC 7797), `Claims`, `ClaimStrings`, `MapClaims` (+ its 24 accessors), `RegisteredClaims`
(+ 8 accessors), `NumericDate`, `NewNumericDate`, `Clock`, `ClockFunc`, `Keyfunc`, `Token`
(+ `HeaderString`, `KeyID`, `SetHeader`, `SetKID`, `SetType`, `SigningString`, `String`,
`TokenType`), `New`, `Parser`, `NewParser`, `Validator`, `NewValidator`, `SigningMethod`,
`SigningMethodHMAC`, `SigningMethodRSA`, `SigningMethodRSAPSS`, `SigningMethodECDSA`,
`SigningMethodEd25519`, `SigningMethodNone`, `SigningMethodEdDSA`,
`UnsafeAllowNoneSignatureType`, the JWK/JWKS layer (`JSONWebKey`, `JSONWebKeySet`,
`JWKSCache`, `JWKSCacheOption`, `NewJSONWebKey`, `ParseJWK`, `ParseJWKSet`, `NewJWKSCache`,
`WithHTTPClient`, `WithRefreshInterval`, `WithMinRefreshInterval`, `HTTPDoer`), the PEM
layer (`ParseRSAPrivateKeyFromPEM`, `ParseRSAPublicKeyFromPEM`, `ParseECPrivateKeyFromPEM`,
`ParseECPublicKeyFromPEM`, `ParseEdPrivateKeyFromPEM`, `ParseEdPublicKeyFromPEM`,
`EncodeRSAPrivateKeyToPEM`, `EncodeRSAPublicKeyToPEM`, `EncodeECPrivateKeyToPEM`,
`EncodeECPublicKeyToPEM`, `EncodeEdPrivateKeyToPEM`, `EncodeEdPublicKeyToPEM`,
`EncodePrivateKeyToPEM`, `EncodePublicKeyToPEM`, `ErrKeyMustBePEMEncoded`), the 22 `Err*`
sentinels, and the parser options with no upstream analogue: `WithExpirationRequired`,
`WithIssuedAt`, `WithJSONNumber`, `WithKnownCriticalHeaders`, `WithRequiredClaims`,
`WithValidTypes`, `WithoutClaimsValidation`, `WithPaddingAllowed`, `WithStrictDecoding`,
`WithClock`.

Six of these are *used* by the harness as the port's implementation of an upstream
behaviour, and so are credited in the tables above rather than here:
`ParseRSAPublicKeyFromPEM`, `ParseECPublicKeyFromPEM`, `ParseEdPublicKeyFromPEM`,
`ParseRSAPrivateKeyFromPEM`, `ParseECPrivateKeyFromPEM`, `ParseEdPrivateKeyFromPEM`
(key loading), plus `WithAllowNone`, `UnsafeAllowNoneSignatureType`, `Token.SetKID`,
`Token.SetHeader` and `ParseUnverified`.

## Counts

Every row in the tables above, tallied by section:

| section | rows | match | differs | missing | untested |
| --- | --- | --- | --- | --- | --- |
| exports | 6 | 3 | 3 | 0 | 0 |
| error properties | 6 | 0 | 0 | 0 | 6 |
| `sign` option keys | 14 | 8 | 2 | 4 | 0 |
| `verify` option keys | 13 | 6 | 4 | 3 | 0 |
| `decode` option keys | 1 | 1 | 0 | 0 | 0 |
| algorithms | 13 | 13 | 0 | 0 | 0 |
| **total** | **53** | **31** | **9** | **7** | **6** |

- **match (31):** `sign`, `TokenExpiredError`, `NotBeforeError`; sign options `algorithm`,
  `audience`, `issuer`, `subject`, `jwtid`, `keyid`, `header`, `noTimestamp`; verify options
  `subject`, `clockTolerance`, `clockTimestamp`, `ignoreExpiration`, `ignoreNotBefore`,
  `complete`; `decode.complete`; all 13 algorithms.
- **differs (9):** `verify`, `decode`, `JsonWebTokenError`; sign `expiresIn`, `notBefore`;
  verify `algorithms`, `audience`, `issuer`, `maxAge`. These 9 rows trace to 6 distinct
  defects: the two security findings, `decode`'s error-vs-`null`, the `ms` timespan-string
  gap (`expiresIn`/`notBefore`/`maxAge`), the array-valued `audience`/`issuer` gap, and the
  error-type reshaping.
- **missing (7):** sign `encoding`, `mutatePayload`, `allowInsecureKeySizes`,
  `allowInvalidAsymmetricKeyTypes`; verify `jwtid`, `nonce`,
  `allowInvalidAsymmetricKeyTypes`.
- **untested (6):** the six error-object properties, by design — the harness compares
  whether a call failed, not error text.
- **extra:** 113 exported Go symbols, of which roughly 100 have no upstream counterpart.

**Symbol parity = match / (match + differs) over the symbols actually compared**
= 31 / 40 = **77.50 %**. `missing`, `untested` and `extra` are excluded, per `HARNESS.md`.

**Case parity = 141 / 152 = 92.76 %** over the 152 non-deviation cases; the 3 deviation
cases are counted separately. Regenerate both numbers with
`GOWORK=off go test ./parity/jwt/`, which rewrites `parity.json`.
