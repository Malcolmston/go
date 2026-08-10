# jwt example

A single runnable program that exercises `github.com/malcolmston/jwt` (the Go
port of `jsonwebtoken`) end to end. All keys are generated in-process; there are
no network calls at runtime and the program terminates on its own.

The library is consumed as a **published Go module**, exactly as an outside user
would — there is no `replace` directive and no reference to the local
`../../jwt` working tree.

## Resolved version

```
github.com/malcolmston/jwt v0.0.0-20260719021428-b82d380034e3
```

The repository carries no semver tags, so `@latest` resolves to that
pseudo-version (its `VERSION` file says `0.3.0`). Note that the published
snapshot is byte-identical to the local working tree for every non-test `.go`
file, so nothing in this example is affected by uncommitted local changes.

## Run

```sh
cd examples/jwt
GOWORK=off go get github.com/malcolmston/jwt@latest
GOWORK=off go mod tidy && GOWORK=off go build ./... && GOWORK=off go run .
```

## What it demonstrates

1. **HS256 sign + verify** with the full validation surface: `WithValidMethods`,
   `WithClock`, `WithAudience`, `WithIssuer`, `WithSubject`, `WithIssuedAt`,
   `WithExpirationRequired`, `WithRequiredClaims`, `WithLeeway`. Both
   `ParseWithClaims` into `RegisteredClaims` and `Parse` into `MapClaims`
   (with `GetIssuer`/`GetID`/`GetTime`/`Has`/`VerifyIssuer` helpers).
2. **Claim validation failures**: expired `exp`, future `nbf`, wrong `aud`,
   `iss`, `sub`, a missing required claim, `WithMaxTokenAge`, and
   `WithIgnoreExpiration` as the escape hatch. Also `ParseUnverified` followed by
   a standalone `NewValidator(...).Validate`.
3. **Tamper detection**: payload mutated with the signature reused, a flipped
   signature byte, a wrong HMAC secret, and a two-segment token. All rejected.
4. **Asymmetric algorithms**: RS256, PS256, ES256, EdDSA — round-trip, plus
   tampered-payload and wrong-key rejection for each. ECDSA signatures are the
   fixed-width 64-byte `r||s` form RFC 7518 requires.
5. **Algorithm confusion and `none`**: an attacker re-signing as HS256 using the
   RSA public-key PEM as the HMAC secret is rejected with `ErrInvalidKeyType`;
   `WithValidMethods` pins the algorithm; `alg: none` requires *both*
   `WithAllowNone` and the `UnsafeAllowNoneSignatureType` sentinel key, and a
   real HS256 token downgraded to `alg: none` is rejected.
6. **PEM round-trips** for RSA/EC/Ed25519 private and public keys, plus the
   generic `EncodePublicKeyToPEM`.
7. **JWK / JWKS**: `NewJSONWebKey` from Go keys, `ComputeKeyID` (RFC 7638
   thumbprint), `ThumbprintURI`, building a `JSONWebKeySet` with `Add` +
   `WithKeyID`/`WithAlgorithm`/`WithUse`, marshalling it, re-parsing with
   `ParseJWKSet`, and verifying RS256/ES256/EdDSA tokens through
   `JSONWebKeySet.Keyfunc()` selected by `kid`. Unknown `kid` and a
   `alg`-mismatched JWK are both rejected. `JSONWebKey.Public()` is shown not to
   leak `d`/`p`.
8. **Detached / unencoded payload (RFC 7797)** via `SignDetached` /
   `VerifyDetached`, including rejection when the out-of-band payload is mutated.
9. **Headers**: `SetHeader`, `SetType`, `crit` rejection unless
   `WithKnownCriticalHeaders`, `WithValidTypes`, a reused `Parser`,
   `Token.String()` round-trip, a zero-length HMAC secret failing closed, and an
   unregistered `alg`.

## Holes found

Everything the README and `doc.go` advertise exists with the documented
signatures, compiles, and behaves correctly. Nothing had to be commented out.
Two nits:

- **`errors.Is` breaks across the `Keyfunc` boundary.** `Parser.ParseWithClaims`
  wraps a keyfunc error with `fmt.Errorf("%w: %v", ErrTokenUnverifiable, err)`
  (`parser.go:194`), i.e. `%v`, not `%w`. So when
  `JSONWebKeySet.Keyfunc()` returns `ErrKeyNotFound` for an unknown `kid`, the
  caller *cannot* recover it with `errors.Is(err, jwt.ErrKeyNotFound)` — only
  `ErrTokenUnverifiable` matches, and the sentinel survives as text only. The
  package doc's claim that "all errors are wrapped sentinels; test them with
  `errors.Is`" does not hold here. The program prints this as `[WARN]`.
- **`GetAlgorithms()` is unordered.** It returns map iteration order, so the
  banner line differs run to run. Minor, but surprising for a registry
  accessor — the sibling `jose` package sorts its equivalents.

No security weaknesses were observed: every tampered token was rejected, `none`
is opt-in twice, algorithm confusion is closed by key type assertions, and an
empty HMAC secret fails closed.
