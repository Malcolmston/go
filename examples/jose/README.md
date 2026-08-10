# jose example

A single runnable program that exercises `github.com/malcolmston/jose` (the Go
port of `panva/jose`) across JWS, JWE, JWK and the exported JWA primitives. All
keys are generated in-process; there are no network calls at runtime and the
program terminates on its own.

The library is consumed as a **published Go module**, exactly as an outside user
would — there is no `replace` directive and no reference to the local
`../../jose` working tree.

## Resolved version

```
github.com/malcolmston/jose v0.0.0-20260725030041-a41abfdb5595
```

The repository carries no semver tags, so `@latest` resolves to that
pseudo-version (its `VERSION` file says `0.1.0`).

**This matters here.** The local working tree contains substantial *uncommitted*
hardening that the published module does not have: nine non-test files differ
(`base64.go`, `doc.go`, `header.go`, `jwa_keymgmt.go`, `jwa_sign.go`, `jwe.go`,
`jwe_json.go`, `jwk.go`, `jws.go`). This example is written against the
published module, so several security probes that pass locally are recorded
below as holes.

## Run

```sh
cd examples/jose
GOWORK=off go get github.com/malcolmston/jose@latest
GOWORK=off go mod tidy && GOWORK=off go build ./... && GOWORK=off go run .
```

Output is labelled `[ ok ]` (behaved as expected), `[HOLE]` (the library
accepted something a hardened JOSE implementation rejects), `[WARN]`
(rejected, but not with the documented sentinel error) and `[FAIL]` (the
example's own expectation was wrong). The current run prints 158 `[ ok ]`, no
`[FAIL]`, 12 `[HOLE]` lines (8 distinct findings) and 1 `[WARN]`.

## What it demonstrates

1. **JWS compact** sign + verify for HS256, HS512, RS256, PS384, ES256, ES384
   and EdDSA, with `kid`/`cty` protected headers, plus tampered-payload and
   wrong-key rejection for each.
2. **Algorithm confusion, `none`, and segment decoding**: `VerifyOptions.Algorithms`
   pinning; an attacker re-signing as HS256 with the RSA modulus as the HMAC
   secret (rejected with `ErrInvalidKeyType`); `Sign` refusing `alg: none` and
   `Verify` rejecting both a hand-built `none` JWS and an HS256 token downgraded
   to `none`; a zero-length HMAC secret; canonical-base64url probes; a
   two-segment token; an empty signature segment.
3. **`crit`, detached payload, RFC 7797**: an undeclared critical header is
   rejected and `KnownCritical` admits it; `SignOptions.DetachPayload` with
   `ErrDetachedPayload` when no payload is supplied and
   `VerifyOptions.DetachedPayload` when it is (and rejection when the
   out-of-band payload is mutated); `b64:false` signing and verification,
   including the `'.'`-in-payload and `b64`-must-be-in-`crit` guards.
4. **JWS JSON**: `SignJSON`/`VerifyJSON`, `Unprotected` rejected in the compact
   form, `SignJSONMulti` with three signers each verified by its own key, and an
   injected unprotected `{"b64":false}` (see holes).
5. **JWE compact across every key-management family**: `RSA-OAEP-256`,
   `RSA-OAEP`, `RSA1_5`, `A128KW`, `A256KW`, `A256GCMKW`, `dir`, `ECDH-ES` and
   `ECDH-ES+A128/A256KW` over P-256/P-384, `ECDH-ES` over X25519, and
   `PBES2-HS512+A256KW` — each paired with a GCM or CBC-HMAC `enc`, then probed
   with a mutated ciphertext, a mutated protected header (the AAD), and a wrong
   key.
6. **Inference and limits**: algorithm inference from the key type;
   `Compress`/`zip:DEF` (11200 → 199 bytes) and `DecryptOptions.MaxDecompressed`
   refusing the inflate; PBES2 `p2c` clamped to `[MinPBES2Count, MaxPBES2Count]`;
   a `dir` key of the wrong length; `ContentEncryptionKeySize`.
7. **JWE JSON**: `EncryptJSON` with `aad` and a shared unprotected header (and
   `aad` mutation breaking the tag), `aad` rejected in the compact form, and
   `EncryptJSONMulti` addressing three recipients from one content encryption,
   each decrypting with its own key.
8. **JWK**: `FromKey` for RSA/EC/Ed25519/X25519/oct, RFC 7638 `Thumbprint`,
   `Key()` round-trip, `IsPrivate`/`Public()` (which correctly drops `d`/`p`),
   building and re-parsing a `JWKSet`, `LookupKeyID`, passing a `*JWK` straight
   to `Verify`, and malformed-JWK rejection.
9. **Exported primitives**: `AESKeyWrap`/`AESKeyUnwrap` (including corrupt
   wrapping and wrong KEK), `PBKDF2` (determinism and salt sensitivity), and
   `ConcatKDF`.

## Holes found

Everything the published module's `README.md`, `doc.go` and `API-DEVIATIONS.md`
advertise exists with the documented signatures and compiles. Nothing had to be
commented out. The holes are behavioural, and all but one are security-relevant.

### Security

1. **`b64` is honoured from the unprotected header (payload substitution).**
   RFC 7797 §6 requires `b64` to be integrity protected, but
   `verifySignature` reads it from the *merged* protected + unprotected view
   (`jws.go`, `b64 := true; if v, present := merged["b64"]`). Adding
   `{"b64":false}` to a signature's unprotected `header` member of a genuine JWS
   JSON document makes `VerifyJSON` **succeed** and return the base64url *text*
   of the payload instead of the decoded octets — the payload the caller acts on
   changes while the signature still verifies, and no key is needed. The example
   prints this as
   `[HOLE] unprotected {"b64":false} injected into a JWS JSON  VERIFIED, payload changed to …`.
   `b64` is also not required to appear in `crit` at verification time, contrary
   to RFC 7797 §3. (Fixed in the uncommitted local tree, which reads `b64` from
   the protected header only and rejects an unprotected copy.)
2. **HMAC verification accepts a zero-length secret.** `Sign` refuses to produce
   one (`jose: key is invalid: HS256 secret is empty`) but `verify` does not
   refuse to accept one. HMAC keyed with no octets is a public function, so an
   application whose secret came back empty — an unset environment variable, a
   truncated config value — fails *open* on every token rather than closed.
3. **`DecodeSegment` is not canonical, so tokens are malleable.** The published
   implementation falls back to the padded decoder "for robustness" and uses
   the non-strict `base64.RawURLEncoding`, so it accepts `=` padding, embedded
   `\r`/`\n`, and a final quantum with non-zero unused bits. One value therefore
   has many spellings: the example re-spells an HS256 signature segment with
   `=` padding and it still verifies, and re-spells a JWE ciphertext's final
   quantum and it still decrypts to the same plaintext. Anything that keys a
   replay cache, audit log, or revocation list on the serialized token can be
   bypassed by re-spelling it. (`'+'`/`'/'` *are* rejected.)
4. **JWK `use`, `key_ops` and `alg` are not enforced.** RFC 7517 §4.2–§4.4 let a
   key state what it is for; the published module ignores all three. A JWK
   published with `"use":"enc"` happily verifies a signature, a `"use":"sig"`
   key is accepted as an encryption recipient, a key whose `key_ops` is
   `["deriveKey"]` verifies, and a key declaring `"alg":"ES384"` verifies an
   ES256 token. There are no `UseSig`/`UseEnc`/`Op*` constants either, although
   the uncommitted local tree adds both the constants and the enforcement.
5. **`ErrDecryptFailed` is not uniform for JWE JSON.** `errors.go` and `doc.go`
   both promise that "every cryptographic decryption failure — wrong key, bad
   tag, invalid padding — reports the same `ErrDecryptFailed`, so the API cannot
   be used as a padding oracle". `DecryptJSON` on a multi-recipient document
   with an unrelated key instead returns
   `jose: key is of invalid type: ECDH-ES requires an EC or X25519 private key, got *rsa.PrivateKey`,
   which is `ErrInvalidKeyType`, not `ErrDecryptFailed` — and it names both the
   recipient's algorithm and the caller's key type. This is a documented-vs-actual
   mismatch; the leak is structural rather than a padding oracle, but it defeats
   `errors.Is(err, jose.ErrDecryptFailed)` as the caller's only check.

### Usability

6. **`crit` is only validated on signing when `SignOptions.Critical` is set.**
   Putting `"crit": ["x-tenant"]` directly in `SignOptions.Header` succeeds, and
   the resulting JWS is then rejected by this same library's `Verify` with
   `critical parameter "x-tenant" is not present` — the producer is allowed to
   emit a document no recipient can read. (The local tree adds a
   `checkCriticalProduced` pass to `Sign`, `Encrypt` and `EncryptJSONMulti`.)
7. **`SignJSON` always emits the general form.** The doc comment on
   `VerifyJSON` says both the general and flattened forms are accepted on
   decode, which is true, but there is no way to *produce* the flattened
   serialization (RFC 7515 §7.2.2) — `SignJSON` with a single key still writes a
   `"signatures"` array. `SignOptions.Unprotected` ends up in
   `signatures[0].header`, which is easy to misread as a top-level `header`.
8. **A JWS JSON document may mix the general and flattened forms.**
   `VerifyJSONWithOptions` appends the top-level `protected`/`header`/`signature`
   to `doc.Signatures` whenever `signature` is non-empty, instead of rejecting
   the ambiguous document RFC 7515 §7.2.2 forbids. The example takes a valid
   general-form JWS, grafts a bogus top-level `protected`/`signature` onto it,
   and `VerifyJSON` still returns success (from the genuine entry in
   `signatures`) rather than refusing the document.

Two things that are correct but worth knowing when writing against the library:
`b64:false` requires a payload with no `'.'` in the compact serialization (the
error message is clear); and the three registry accessors
(`SignatureAlgorithms`, `KeyManagementAlgorithms`, `ContentEncryptionAlgorithms`)
return sorted slices, which the sibling `jwt` package's `GetAlgorithms` does not.
