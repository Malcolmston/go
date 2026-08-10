# `jose` parity coverage

| | |
| --- | --- |
| upstream | `jose@5.9.6` (panva/jose), installed under `node/` |
| Go module | `github.com/malcolmston/jose v0.0.0-20260810111536-aa9e56a9a81b` (resolved by `GOWORK=off go get github.com/malcolmston/jose@latest`, no `replace`) |
| runner protocol | JSON Lines on stdio, per `parity/HARNESS.md` |
| cases | 157 across 7 groups |
| case parity | **144 / 157 = 91.7 %** (regenerated into `parity.json` by `go test`) |
| symbol parity | **9 / 18 = 50.0 %** over the upstream symbols that have at least one case (see the rule below) |
| deviations | 0 — no divergence found here is listed in the library's `API-DEVIATIONS.md`, so all 13 failures are counted as bugs, not as sanctioned deviations |

Run it with:

```sh
cd parity/jose && GOWORK=off go test .
```

`go test` skips (never fails) when `node` is absent or `node/node_modules/jose`
has not been installed.

## How the upstream inventory was derived

Mechanically, from the installed package's own module namespace — not from the
README:

```sh
cd parity/jose/node
node --input-type=module -e "import * as m from 'jose'; console.log(Object.keys(m).sort().join('\n'))"
node --input-type=module -e "import * as m from 'jose'; console.log(Object.keys(m.errors).sort().join(','))"
node --input-type=module -e "import * as m from 'jose'; console.log(Object.keys(m.base64url).sort().join(','))"
```

That yields **38 top-level exports**, of which `base64url` and `errors` are
namespaces holding a further **2** and **15** members: **55 symbols in total**,
every one of which appears in the tables below.

The Go side was enumerated with:

```sh
cd parity/jose && GOWORK=off go doc -all github.com/malcolmston/jose
```

## Status rule

A symbol is `match` only if **every** case naming it in `upstreamFn` passes; it
is `differs` if any of them fails; `untested` if no case names it; `missing` if
the port has no counterpart at all; `extra` if the port has a symbol upstream
does not. That rule is applied mechanically from `parity.json`'s `failedCases`
list, so a symbol that is right 20 times and wrong 3 times still reads
`differs` — the pass/fail counts in the "cases" column show the balance.

## Case shapes

- **same** — the identical request goes to both runners; `ok` is compared first,
  then the value with a JSON-normalising deep-equal. Used for everything
  deterministic (HMAC, RSASSA-PKCS1-v1_5, Ed25519, JWK import/export,
  thumbprints, base64url) and for every rejection case.
- **cross** — the request is a *producer*; its output is handed to the other
  runner's consumer, in both directions, and the two consumed results are
  compared to each other (and against `expect`). This is the only meaningful
  comparison for RSA-OAEP, RSASSA-PSS, ECDSA, ECDH-ES, AES-GCM (random IV) and
  PBES2 (random salt), where byte equality is impossible by construction. The
  randomised header parameters `iv`, `tag`, `epk`, `p2s` and `p2c` are stripped
  before the two consumed headers are compared; everything else in the header is
  compared.
- Keys are **fixed and committed** under `keys/` (JWK JSON plus PKCS#8/SPKI PEM),
  generated once with `jose.generateKeyPair` + `exportJWK`/`exportPKCS8`. Nothing
  is generated at run time, so the suite is deterministic.
- Error **text** is never compared, only whether the call failed. Message
  differences are recorded in the divergence notes below.

## Upstream inventory

### JWS

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `jose.CompactSign` | `jose.Sign` | differs | 13 pass, 3 fail (`jws-hs256-sign` … `jws-hs256-sign-crit`, `rej-jws-jwk-*-sign`) | byte-identical output for HS256/384/512, RS256/384/512, EdDSA, and for `kid`/`typ`/`cty`/`crit` headers. **Accepts JWK `use`, `key_ops` and `alg` misuse that upstream rejects** — see findings 1–3. |
| `jose.compactVerify` | `jose.Verify`, `jose.VerifyWithOptions` | differs | 20 pass, 3 fail | verifies every algorithm cross-language and rejects tampered signatures, tampered payloads, wrong keys, `alg:none`, unknown `alg`, missing `alg`, a disallowed `alg`, unknown/registered/empty/absent `crit`, a detached payload with nothing supplied, non-base64url segments and a two-segment token. Same JWK-misuse gap as `CompactSign`. |
| `jose.FlattenedSign` | — (`jose.SignJSON` emits the **general** form only) | differs | 5 pass (`jws-flat-*`, node → Go only) | the port can verify the flattened serialization but cannot produce it, so those cases run one-directionally. Also: with `b64:false` upstream always omits the `payload` member, the port emits the raw payload inline; both verify each other once the payload is supplied out of band. |
| `jose.flattenedVerify` | `jose.VerifyJSON`, `jose.VerifyJSONWithOptions` | match | 7 pass (`jws-flat-*`, `jws-hs256-roundtrip-detached`, `rej-jws-json-no-protected-no-header`) | also reached internally by `compactVerify` and `generalVerify`. Detached compact verification uses the documented split-and-`flattenedVerify` idiom. |
| `jose.GeneralSign` | `jose.SignJSONMulti` | match | 8 pass | single, double and triple signatures; per-signature unprotected headers; `crit`; detached; RFC 7797 `b64:false`. `jws-gen-hs256-value` and `jws-gen-multi-hs-rs-value` compare the whole JSON document byte for byte. |
| `jose.generalVerify` | `jose.VerifyJSON`, `jose.VerifyJSONWithOptions` | differs | 8 pass, 2 fail | selects the right signature by key in a multi-signature document. **Reads `b64` from the merged header instead of the protected header** — findings 4 and 5. |
| `jose.UnsecuredJWT` | — | missing | — | the port refuses `alg:none` outright and offers no opt-in; `rej-jws-alg-none` confirms neither side will verify one. |
| `jose.EmbeddedJWK` | — | missing | — | the port never resolves a key from the `jwk` header parameter. |

### JWE

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `jose.CompactEncrypt` | `jose.Encrypt` | differs | 27 pass, 1 fail | round-trips in **both** directions for `RSA1_5`, `RSA-OAEP`, `RSA-OAEP-256`, `A128/192/256KW`, `A128/192/256GCMKW`, `dir`, `ECDH-ES` (P-256/P-384/P-521/X25519), `ECDH-ES+A128/192/256KW` (incl. `apu`/`apv`) and `PBES2-HS256/384/512+A128/192/256KW`, against `A128/192/256GCM` and `A128CBC-HS256`/`A192CBC-HS384`/`A256CBC-HS512`, plus empty and multi-byte-UTF-8 plaintext. **`zip:"DEF"` is a divergence** — finding 8. |
| `jose.compactDecrypt` | `jose.Decrypt`, `jose.DecryptWithOptions` | differs | 33 pass, 2 fail | rejects tampered ciphertext/tag/IV/protected header, wrong symmetric and wrong RSA keys, unsupported `alg`, unsupported `enc`, disallowed `alg`, disallowed `enc` and a truncated token. Diverges on PBES2 defaults — findings 6 and 7. |
| `jose.FlattenedEncrypt` | — (`jose.EncryptJSON` emits the **general** form only) | differs | 4 pass (`jwe-flat-*`, node → Go only) | the port decrypts the flattened serialization but cannot produce it. `aad` and a shared unprotected header both survive the crossing. |
| `jose.flattenedDecrypt` | `jose.DecryptJSON`, `jose.DecryptJSONWithOptions` | match | 4 pass (`jwe-flat-*`) | also reached internally by `compactDecrypt` and `generalDecrypt`. |
| `jose.GeneralEncrypt` | `jose.EncryptJSONMulti`, `jose.Recipient` | match | 9 pass | one, two and three recipients over one CEK, mixing `A128KW`, `A256KW`, `A256GCMKW`, `RSA-OAEP-256`, `PBES2-HS256+A128KW` and `ECDH-ES+A256KW`; `aad`; shared unprotected header. |
| `jose.generalDecrypt` | `jose.DecryptJSON`, `jose.DecryptJSONWithOptions` | differs | 8 pass, 1 fail | picks the recipient the supplied key unlocks, whichever position it sits in. Diverges on a malformed mixed-form document — finding 9. |
| `jose.EncryptJWT` | — | missing | — | JWT claim layer; lives in the sibling `github.com/malcolmston/jwt`, not in this port. |
| `jose.jwtDecrypt` | — | missing | — | as above. |

### JWK / JWKS

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `jose.importJWK` | `jose.ParseJWK`, `JWK.Key` | match | 11 pass (`jwk-rt-*`) | RSA (with all CRT parameters), EC P-256/P-384/P-521, OKP Ed25519, OKP X25519, `oct`. |
| `jose.exportJWK` | `jose.FromKey`, `JWK.Public` | match | 15 pass (`jwk-rt-*`, `jwk-public-*`) | import-then-export reproduces every parameter, byte for byte, for every key type. |
| `jose.calculateJwkThumbprint` | `JWK.Thumbprint` | match | 12 pass (`jwk-thumbprint-*`, `jwks-lookup-*`) | RFC 7638 for RSA, EC, OKP and `oct`; unaffected by private parameters and by `use`/`alg`/`kid`. |
| `jose.calculateJwkThumbprintUri` | — | missing | — | the port has no `urn:ietf:params:oauth:jwk-thumbprint:` helper. |
| `jose.createLocalJWKSet` | `jose.ParseJWKSet`, `JWKSet.LookupKeyID` | match | 4 pass (`jwks-lookup-*`) | selection by `kid`; both sides fail on an unknown `kid`. Upstream additionally filters by `alg`/`use` and can raise `JWKSMultipleMatchingKeys`; the port matches on `kid` alone and returns the first hit. |
| `jose.createRemoteJWKSet` | — | missing | — | fetches over HTTP; the port has no network code. |
| `jose.jwksCache` | — | missing | — | belongs to the remote JWKS machinery. |
| `jose.experimental_jwksCache` | — | missing | — | as above. |
| `jose.importPKCS8` | — | missing | — | the port has no PEM/DER import. `keys/*-priv.pem` is committed for reference and for the upstream runner, but the Go runner can only consume the JWK form. |
| `jose.importSPKI` | — | missing | — | as above. |
| `jose.importX509` | — | missing | — | no certificate parsing. |
| `jose.exportPKCS8` | — | missing | — | no PEM/DER export. |
| `jose.exportSPKI` | — | missing | — | as above. |
| `jose.generateKeyPair` | — | missing | — | no key generation in the port; harness keys are generated once, upstream, and committed. |
| `jose.generateSecret` | — | missing | — | as above. |

### JWT claim layer (not part of this port)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `jose.SignJWT` | — | missing | — | the port is deliberately payload-agnostic: `[]byte` in, `[]byte` out, no claim model and no clock. Time-based claim checks (and therefore an explicit `now`) have no counterpart to compare against, so no case supplies one. |
| `jose.jwtVerify` | — | missing | — | as above. |
| `jose.decodeJwt` | — | missing | — | as above. |
| `jose.decodeProtectedHeader` | — | missing | — | the port only ever returns a header as part of `Verify`/`Decrypt`; there is no standalone decoder. |

### Utilities

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `jose.base64url.encode` | `jose.EncodeSegment` | match | 6 pass (`b64-encode-*`) | unpadded, URL-safe alphabet, all input lengths mod 3. |
| `jose.base64url.decode` | `jose.DecodeSegment` | differs | 4 pass, 1 fail (`b64-decode-*`) | agrees on every valid input; diverges on invalid input — finding 10. |
| `jose.cryptoRuntime` | — | missing | — | informational string (`"node:crypto"`); nothing to port. |

### Error taxonomy

Every member of `jose.errors` is listed for completeness. The harness compares
*whether* a call failed, never the error type or message, so each of these is
`untested` by design; the Go sentinel column records the counterpart a caller
would test with `errors.Is`.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `jose.errors.JOSEError` | — | untested | — | base class; the port uses plain sentinel `error` values, not a hierarchy. |
| `jose.errors.JOSEAlgNotAllowed` | `jose.ErrUnsupportedAlgorithm` / `ErrSignatureInvalid` | untested | — | raised by `rej-jws-alg-not-allowed`, `rej-jwe-alg-not-allowed`, `rej-jwe-pbes2-without-opt-in`. |
| `jose.errors.JOSENotSupported` | `jose.ErrUnsupportedAlgorithm`, `ErrUnsupportedEncryption` | untested | — | raised by `rej-jws-alg-unsupported`, `rej-jwe-alg-unsupported`, `rej-jwe-enc-unsupported`, `rej-jwe-zip-def`. |
| `jose.errors.JWEDecryptionFailed` | `jose.ErrDecryptFailed` | untested | — | raised by the `rej-jwe-tampered-*` and `rej-jwe-wrong-*` cases. |
| `jose.errors.JWEInvalid` | `jose.ErrMalformed`, `ErrInvalidHeader` | untested | — | raised by `rej-jwe-truncated`, `rej-jwe-pbes2-p2c-below-minimum`. |
| `jose.errors.JWSInvalid` | `jose.ErrMalformed`, `ErrInvalidHeader`, `ErrInvalidCrit` | untested | — | raised by `rej-jws-alg-missing`, `rej-jws-crit-*`, `rej-jws-not-base64url`, `rej-jws-too-few-segments`, `rej-jws-json-no-protected-no-header`. |
| `jose.errors.JWSSignatureVerificationFailed` | `jose.ErrSignatureInvalid` | untested | — | raised by `rej-jws-tampered-signature`, `rej-jws-tampered-payload`, `rej-jws-wrong-key`. |
| `jose.errors.JWKInvalid` | `jose.ErrInvalidKey` | untested | — | |
| `jose.errors.JWKSInvalid` | `jose.ErrInvalidKey` | untested | — | |
| `jose.errors.JWKSNoMatchingKey` | `jose.ErrKeyNotFound` | untested | — | raised by `jwks-lookup-unknown-kid`. |
| `jose.errors.JWKSMultipleMatchingKeys` | — | untested | — | the port has no ambiguity error: `LookupKeyID` returns the first `kid` match. |
| `jose.errors.JWKSTimeout` | — | untested | — | remote JWKS only. |
| `jose.errors.JWTClaimValidationFailed` | — | untested | — | no claim layer in the port. |
| `jose.errors.JWTExpired` | — | untested | — | as above. |
| `jose.errors.JWTInvalid` | — | untested | — | as above. |

### Go-only symbols (`extra`)

These have no upstream counterpart, so no parity case can exist for them; they
are listed so the inventory is complete in both directions. All are documented
in the library's own `API-DEVIATIONS.md`.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| — | `jose.AESKeyWrap`, `jose.AESKeyUnwrap` | extra | — | RFC 3394, exported because `golang.org/x/crypto` is off-limits for this family. Exercised indirectly by every `*KW` case. |
| — | `jose.PBKDF2` | extra | — | RFC 8018 §5.2; exercised indirectly by the PBES2 cases. |
| — | `jose.ConcatKDF` | extra | — | RFC 7518 §4.6.2; exercised indirectly by the ECDH-ES cases. |
| — | `jose.SignatureAlgorithms`, `jose.KeyManagementAlgorithms`, `jose.ContentEncryptionAlgorithms`, `jose.ContentEncryptionKeySize` | extra | — | registry accessors. |
| — | `jose.Header` + `Algorithm`/`Encryption`/`KeyID`/`Type`/`ContentType`/`Critical`/`String` | extra | — | typed header accessors over `map[string]any`. |
| — | `jose.JWK.IsPrivate` | extra | — | |
| — | `jose.SignOptions`, `VerifyOptions`, `EncryptOptions`, `DecryptOptions`, `Signer`, `Recipient` | extra | — | option/aggregate types; upstream uses builder classes instead. |
| — | `jose.VerifyWithOptions`, `VerifyJSONWithOptions`, `DecryptWithOptions`, `DecryptJSONWithOptions` | extra | — | the option-carrying forms; exercised by the allow-list, `crit` and detached-payload cases. |
| — | `jose.MaxDecompressedSize`, `DefaultPBES2Count`, `MinPBES2Count`, `MaxPBES2Count`, `MaxPBES2SaltInput` | extra | — | exported security bounds. `MinPBES2Count` is the cause of finding 7. |
| — | `jose.Err*` (15 sentinels) | extra | — | see the error table above. |
| — | `jose.HS256` … `jose.PBES2_HS512_A256KW` (alg/enc constants) | extra | — | string constants for every `alg`/`enc`; their values are exercised by every case. |

## Divergences

Ten distinct behaviours, spread over the 13 failing cases.

### Security findings — the port accepts what upstream rejects

**1. JWK `use` is not enforced (`rej-jws-jwk-use-enc-for-signing`, `rej-jws-jwk-use-enc-for-verifying`).**
A JWK carrying `"use":"enc"` is happily used to produce *and* verify a JWS.
Upstream refuses both: `TypeError: Invalid key for this operation, when present
its use must be sig`. RFC 7517 §4.2 makes `use` the key's declared purpose;
ignoring it lets an encryption key be repurposed as a signing key. The port
never reads `JWK.Use` anywhere outside the struct definition.

**2. JWK `key_ops` is not enforced (`rej-jws-jwk-key-ops-without-sign`, `rej-jws-jwk-key-ops-without-verify`).**
A JWK whose `key_ops` is `["encrypt","decrypt"]` signs and verifies without
complaint. Upstream: `TypeError: … its key_ops must include sign` / `… verify`.
RFC 7517 §4.3. Same root cause: `JWK.KeyOps` is parsed and then ignored.

**3. JWK `alg` mismatch is not enforced (`rej-jws-jwk-alg-mismatch-sign`, `rej-jws-jwk-alg-mismatch-verify`).**
A JWK pinned to `"alg":"HS512"` is used with a header `alg` of `HS256`.
Upstream: `TypeError: … its alg must be HS256`. The port treats `JWK.Alg` only
as a *default* when `SignOptions.Algorithm` is empty and never checks it for
conflict, which defeats the point of pinning an algorithm to a key
(RFC 7517 §4.4).

**4. RFC 7797 `b64` is read from the merged header, so `b64:false` can be smuggled in unprotected (`rej-jws-b64-false-in-unprotected-with-crit`).**
Given a JWS whose protected header is `{"alg":"HS256","crit":["b64"]}` and whose
*unprotected* header carries `{"b64":false}`, the port verifies successfully and
reports `b64:false`. Upstream rejects it (`JWSSignatureVerificationFailed`;
`flattenedVerify` reads `b64` from `parsedProt` only and demands a boolean
there). RFC 7797 §6 requires `b64` to be integrity protected. In
`jws.go:verifySignature` the flag is taken from `merged` (protected ∪
unprotected), and `checkCritical` is satisfied by the parameter's presence in
`merged` too, so an unprotected `b64` both satisfies `crit` and changes how the
payload is interpreted.

**5. The same gap turns into payload substitution on an otherwise valid token (`rej-jws-b64-false-in-unprotected-no-crit`).**
Take an ordinary, correctly signed `b64:true` JWS, wrap it in the general JSON
serialization and add `{"b64":false}` to the unprotected header — no key needed.
The signing input is unchanged, so the MAC still verifies on both sides, but:

- upstream returns `It's a dangerous business, Frodo, going out your door.`
- the port returns `SXQncyBhIGRhbmdlcm91cyBidXNpbmVzcywgRnJvZG8s…` — the
  *undecoded* base64url text.

An attacker who cannot forge a signature can still change what a verifying
application believes was signed. Both fixes are the same one: read `b64` from
the protected header only, and require it to be listed in `crit`.

### Divergences where the port is stricter than upstream

**6. PBES2 needs no opt-in on decrypt (`rej-jwe-pbes2-without-opt-in`).**
Upstream refuses *every* `PBES2-*` `alg` during decryption unless the caller
names it in `keyManagementAlgorithms` (`jwe/flattened/decrypt.js`:
`!keyManagementAlgorithms && alg.startsWith('PBES2')` →
`JOSEAlgNotAllowed`). The port decrypts PBES2 by default. That is a missing
guardrail rather than a broken primitive — a password-derived key is much weaker
than a random one, and upstream forces the caller to say so out loud. All PBES2
round-trip cases therefore pass `keyManagement` explicitly.

**7. PBES2 iteration bounds differ (`rej-jwe-pbes2-p2c-below-minimum`).**
The port enforces `MinPBES2Count = 1000` ≤ `p2c` ≤ `MaxPBES2Count = 1000000`,
at encryption *and* decryption. Upstream has no lower bound and caps `p2c` at
10 000 by default (`maxPBES2Count`). A token with `p2c:10` therefore decrypts
upstream and is refused by the port (`jose: PBES2 iteration count out of range`);
conversely the port's default `p2c` of 100 000 exceeds upstream's default cap,
which is why every PBES2 case sets `p2c: 2048` explicitly.

**8. `zip:"DEF"` (`rej-jwe-zip-def`).** jose 5 removed JWE compression outright —
both `FlattenedEncrypt` and `flattenedDecrypt` throw
`JOSENotSupported: JWE "zip" (Compression Algorithm) Header Parameter is not
supported.` The port implements RFC 7516 `DEF` with a bounded inflate
(`MaxDecompressedSize`, `ErrCompressionLimit`). Anything the port compresses is
undecryptable by upstream, and vice versa there is nothing to decrypt. This is
a deliberate feature of the port, but it is **not** recorded in the library's
`API-DEVIATIONS.md`, so the harness counts it as a divergence rather than a
sanctioned deviation.

**9. A JWE mixing the general and flattened forms (`rej-jwe-json-mixed-forms`).**
A document carrying both a `recipients` array and a top-level `encrypted_key`
is refused by the port (`jose: token is malformed: a JWE cannot mix the general
and flattened forms`) and decrypted anyway by upstream, which simply reads the
`recipients` array. RFC 7516 §7.2 does not permit the mixture; the port is
right and upstream is lax.

**10. `base64url.decode` on non-alphabet input (`b64-decode-invalid-char`).**
Upstream returns an empty `Uint8Array` for `"!!!!"` (Node's
`Buffer.from(…, 'base64url')` never throws). The port returns an error. Note
also that both decoders ignore the spare low bits of a trailing base64url
character, which is why the tampered-signature and tampered-tag cases flip a
*leading* character rather than the last one.

### Non-divergences worth recording

- **Header member order.** JOSE puts no ordering requirement on header members,
  but the protected header is serialized *before* it is signed, so byte-for-byte
  comparison is only possible if both sides emit the same order. The port
  marshals a Go map, which `encoding/json` always sorts; the node runner sorts
  its protected header to match. Without that, every case with more than two
  header members would show a spurious signature mismatch.
- **`use:"enc"` on the JWE path.** Upstream's `jwkMatchesOp` hard-codes
  `use === 'sig'` for *all* operations, but JWE key management is bound with
  `allowJwk=false`, so a bare JWK is never accepted there at all. The upstream
  runner consequently imports JWKs before every JWE call, and the JWK-misuse
  cases are confined to the JWS path — the only place upstream actually checks.
- **Detached compact JWS.** Upstream has no detach option on `CompactSign` and no
  detached-payload option on `compactVerify`; the documented idiom is to split
  the compact form and use `flattenedVerify`. The upstream runner does exactly
  that, and `jws-hs256-roundtrip-detached` runs Go → node only.
- **`b64:false` payload member.** Upstream always emits `payload: ""` when
  `b64` is false; the port emits the raw payload inline. Both verify each other
  once the payload is supplied out of band, so this is a serialization-shape
  difference rather than a correctness one.

## What the port lacks that upstream has

The whole JWT claim layer (`SignJWT`, `jwtVerify`, `EncryptJWT`, `jwtDecrypt`,
`decodeJwt`, `UnsecuredJWT`), all PEM/DER key handling (`importPKCS8`,
`importSPKI`, `importX509`, `exportPKCS8`, `exportSPKI`), key generation
(`generateKeyPair`, `generateSecret`), remote JWKS (`createRemoteJWKSet`,
`jwksCache`, `experimental_jwksCache`), `EmbeddedJWK`,
`calculateJwkThumbprintUri`, `decodeProtectedHeader`, and a flattened-JSON
*producer* for either JWS or JWE. Of these, only the flattened producer and the
missing JWK `use`/`key_ops`/`alg` checks affect interoperability or safety; the
claim layer is out of scope by design (it is the sibling `malcolmston/jwt`
port), and PEM support is a genuine ergonomic gap.
