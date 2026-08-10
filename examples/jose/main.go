// Command jose-example exercises github.com/malcolmston/jose: JWS signing and
// verification (compact, flattened/general JSON, multi-signature, detached and
// RFC 7797 unencoded payloads), JWE encryption across every family of key
// management algorithm (RSA-OAEP, AES-KW, AES-GCM-KW, dir, ECDH-ES, PBES2),
// JWK/JWKSet handling and thumbprints, the exported primitives (AES key wrap,
// PBKDF2, Concat KDF), and the library's security properties: tamper detection,
// algorithm confusion, "none" rejection, crit handling and compression limits.
//
// No network access; every key is generated in-process.
package main

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/malcolmston/jose"
)

func section(name string) { fmt.Printf("\n=== %s ===\n", name) }

func ok(label string, err error) bool {
	if err != nil {
		fmt.Printf("  [FAIL] %-52s %v\n", label, err)
		return false
	}
	fmt.Printf("  [ ok ] %s\n", label)
	return true
}

// weak marks a probe where a hardened JOSE implementation should reject the
// input. Unlike mustFail it labels a nil error as [HOLE] rather than [FAIL],
// because it is documenting the library's behaviour rather than the example's.
func weak(label string, err error) {
	if err == nil {
		fmt.Printf("  [HOLE] %-52s ACCEPTED (a hardened implementation rejects this)\n", label)
		return
	}
	fmt.Printf("  [ ok ] %-52s rejected: %v\n", label, err)
}

func mustFail(label string, err error, want error) {
	switch {
	case err == nil:
		fmt.Printf("  [FAIL] %-52s expected an error, got nil\n", label)
	case want != nil && !errors.Is(err, want):
		fmt.Printf("  [WARN] %-52s rejected, but not as %v: %v\n", label, want, err)
	default:
		fmt.Printf("  [ ok ] %-52s rejected: %v\n", label, err)
	}
}

func main() {
	fmt.Println("github.com/malcolmston/jose — example program")
	fmt.Println("  sig algs:  ", strings.Join(jose.SignatureAlgorithms(), " "))
	fmt.Println("  key mgmt:  ", strings.Join(jose.KeyManagementAlgorithms(), " "))
	fmt.Println("  content enc:", strings.Join(jose.ContentEncryptionAlgorithms(), " "))
	fmt.Printf("  limits: MaxDecompressedSize=%d PBES2 p2c in [%d,%d] default %d\n",
		jose.MaxDecompressedSize, jose.MinPBES2Count, jose.MaxPBES2Count, jose.DefaultPBES2Count)

	// -------------------------------------------------------------- key setup
	secret := []byte("a-32-byte-hmac-secret-for-hs256!")
	rsaKey := mustRSA()
	otherRSA := mustRSA()
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(err)
	ecP384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	must(err)
	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	must(err)
	xPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	must(err)

	payload := []byte(`{"iss":"auth.example.com","sub":"user-42","scope":"read write"}`)

	// ------------------------------------------------------------------- JWS
	section("1. JWS compact: sign + verify across algorithms")

	type sigCase struct {
		alg      string
		priv     any
		pub      any
		wrongPub any
	}
	sigCases := []sigCase{
		{jose.HS256, secret, secret, []byte("some-other-secret-of-32-bytes!!!")},
		{jose.HS512, secret, secret, nil},
		{jose.RS256, rsaKey, &rsaKey.PublicKey, &otherRSA.PublicKey},
		{jose.PS384, rsaKey, &rsaKey.PublicKey, &otherRSA.PublicKey},
		{jose.ES256, ecKey, &ecKey.PublicKey, nil},
		{jose.ES384, ecP384, &ecP384.PublicKey, nil},
		{jose.EdDSA, edPriv, edPub, nil},
	}

	var hsCompact string
	for _, c := range sigCases {
		token, err := jose.Sign(payload, c.priv, jose.SignOptions{
			Algorithm: c.alg,
			KeyID:     "key-" + strings.ToLower(c.alg),
			Header:    map[string]any{"cty": "application/json"},
		})
		if !ok(fmt.Sprintf("Sign(%s) -> %d bytes", c.alg, len(token)), err) {
			continue
		}
		if c.alg == jose.HS256 {
			hsCompact = token
		}
		got, hdr, err := jose.VerifyWithOptions(token, c.pub, jose.VerifyOptions{
			Algorithms: []string{c.alg},
		})
		h := jose.Header(hdr)
		ok(fmt.Sprintf("Verify(%s): payload match=%v kid=%q cty=%q",
			c.alg, bytes.Equal(got, payload), h.KeyID(), h.ContentType()), err)

		// Tamper: mutate the payload segment, keep the signature.
		bad := tamperPayload(token, func(m map[string]any) { m["scope"] = "admin" })
		_, _, err = jose.Verify(bad, c.pub)
		mustFail(c.alg+": mutated payload, signature reused", err, jose.ErrSignatureInvalid)

		if c.wrongPub != nil {
			_, _, err = jose.Verify(token, c.wrongPub)
			mustFail(c.alg+": verified with a different key", err, jose.ErrSignatureInvalid)
		}
	}

	section("2. JWS: algorithm confusion, \"none\", and strict decoding")

	rsToken, err := jose.Sign(payload, rsaKey, jose.SignOptions{Algorithm: jose.RS256})
	must(err)

	// An RS256 token handed to a caller who pinned ES256.
	_, _, err = jose.VerifyWithOptions(rsToken, &rsaKey.PublicKey,
		jose.VerifyOptions{Algorithms: []string{jose.ES256}})
	mustFail("RS256 token, verifier pinned to ES256", err, nil)

	// Classic confusion: attacker re-signs as HS256 using the RSA modulus as the
	// HMAC secret; the verifier still holds an *rsa.PublicKey.
	forged, err := jose.Sign(payload, rsaKey.PublicKey.N.Bytes(), jose.SignOptions{Algorithm: jose.HS256})
	ok("attacker signs HS256 with the RSA modulus as the secret", err)
	_, _, err = jose.Verify(forged, &rsaKey.PublicKey)
	mustFail("HS256-for-RS256 confusion", err, jose.ErrInvalidKeyType)

	// "none" is refused in both directions, with no opt-in.
	_, err = jose.Sign(payload, nil, jose.SignOptions{Algorithm: jose.None})
	mustFail("Sign with alg=none", err, nil)
	noneToken := forgeNone(payload)
	_, _, err = jose.Verify(noneToken, secret)
	mustFail("Verify a hand-built alg=none JWS", err, jose.ErrNoneAlgDisallowed)
	_, _, err = jose.Verify(stripToNone(hsCompact), secret)
	mustFail("HS256 token downgraded to alg=none", err, jose.ErrNoneAlgDisallowed)

	// Zero-length HMAC secret. HMAC keyed with no octets is a public function,
	// so a secret that resolved to nothing must fail closed, not open.
	_, err = jose.Sign(payload, []byte{}, jose.SignOptions{Algorithm: jose.HS256})
	mustFail("Sign(HS256) with a zero-length secret", err, nil)
	emptySecretToken, err := forgeHMAC(payload, jose.HS256, []byte{})
	must(err)
	_, _, err = jose.Verify(emptySecretToken, []byte{})
	weak("Verify(HS256) against a zero-length secret", err)

	// Canonical base64url: RFC 7515 §2 omits all padding, so padding, the
	// standard alphabet's '+'/'/', and embedded whitespace should all be
	// refused — otherwise one value has several spellings and the serialized
	// token is malleable.
	parts := strings.Split(hsCompact, ".")
	_, _, err = jose.Verify(parts[0]+"."+parts[1]+"."+padTo4(parts[2]), secret)
	weak("signature segment re-spelled with '=' padding", err)
	weak("DecodeSegment with embedded newline", func() error {
		_, e := jose.DecodeSegment(parts[1][:4] + "\n" + parts[1][4:])
		return e
	}())
	weak("DecodeSegment(\"a+b/c\") (standard alphabet)", func() error {
		_, e := jose.DecodeSegment("a+b/c")
		return e
	}())
	weak("DecodeSegment with non-zero unused bits in the final quantum",
		func() error { _, e := jose.DecodeSegment(nonCanonical(parts[2])); return e }())
	_, _, err = jose.Verify("only.two", secret)
	mustFail("two-segment compact serialization", err, jose.ErrMalformed)
	_, _, err = jose.Verify(parts[0]+"."+parts[1]+".", secret)
	mustFail("empty signature segment", err, nil)

	section("3. JWS: crit, detached payload, RFC 7797 b64=false")

	critToken, err := jose.Sign(payload, ecKey, jose.SignOptions{
		Algorithm: jose.ES256,
		Header:    map[string]any{"x-tenant": "acme"},
		Critical:  []string{"x-tenant"},
	})
	ok("Sign with crit=[x-tenant]", err)
	_, _, err = jose.Verify(critToken, &ecKey.PublicKey)
	mustFail("undeclared critical header", err, jose.ErrInvalidCrit)
	_, hdr, err := jose.VerifyWithOptions(critToken, &ecKey.PublicKey,
		jose.VerifyOptions{Algorithms: []string{jose.ES256}, KnownCritical: []string{"x-tenant"}})
	ok(fmt.Sprintf("accepted with KnownCritical (x-tenant=%v)", hdr["x-tenant"]), err)

	detached, err := jose.Sign(payload, edPriv, jose.SignOptions{
		Algorithm:     jose.EdDSA,
		DetachPayload: true,
	})
	ok("Sign with DetachPayload", err)
	if err == nil {
		fmt.Printf("        detached: %s (payload segment empty: %v)\n",
			abbrev(detached), strings.Split(detached, ".")[1] == "")
		_, _, err = jose.Verify(detached, edPub)
		mustFail("Verify detached JWS with no payload supplied", err, jose.ErrDetachedPayload)
		got, _, err := jose.VerifyWithOptions(detached, edPub,
			jose.VerifyOptions{Algorithms: []string{jose.EdDSA}, DetachedPayload: payload})
		ok(fmt.Sprintf("VerifyWithOptions with DetachedPayload (match=%v)", bytes.Equal(got, payload)), err)
		_, _, err = jose.VerifyWithOptions(detached, edPub,
			jose.VerifyOptions{DetachedPayload: []byte(`{"sub":"root"}`)})
		mustFail("detached verify with the wrong out-of-band payload", err, jose.ErrSignatureInvalid)
	}

	// RFC 7797: b64=false, which must also appear in crit. The unencoded payload
	// travels verbatim in the compact serialization, so it must not contain '.'.
	rawPayload := []byte(`{"sub":"user-42","scope":"read write"}`)
	unencoded, err := jose.Sign(rawPayload, edPriv, jose.SignOptions{
		Algorithm: jose.EdDSA,
		Header:    map[string]any{"b64": false},
		Critical:  []string{"b64"},
	})
	if ok("Sign with b64=false (RFC 7797 unencoded payload)", err) {
		fmt.Printf("        payload segment is raw JSON: %q\n", strings.Split(unencoded, ".")[1])
		got, _, err := jose.VerifyWithOptions(unencoded, edPub,
			jose.VerifyOptions{Algorithms: []string{jose.EdDSA}})
		ok(fmt.Sprintf("Verify b64=false token (match=%v)", bytes.Equal(got, rawPayload)), err)
	}
	// A payload containing '.' cannot be carried unencoded.
	_, err = jose.Sign(payload, edPriv, jose.SignOptions{
		Algorithm: jose.EdDSA,
		Header:    map[string]any{"b64": false},
		Critical:  []string{"b64"},
	})
	mustFail("b64=false with a payload containing '.'", err, nil)
	// b64=false without crit must be refused on signing.
	_, err = jose.Sign(rawPayload, edPriv, jose.SignOptions{
		Algorithm: jose.EdDSA,
		Header:    map[string]any{"b64": false},
	})
	mustFail("b64=false not listed in crit", err, jose.ErrInvalidCrit)

	section("4. JWS JSON serializations")

	flat, err := jose.SignJSON(payload, ecKey, jose.SignOptions{
		Algorithm:   jose.ES256,
		KeyID:       "ec-1",
		Unprotected: map[string]any{"x-note": "unprotected"},
	})
	if ok(fmt.Sprintf("SignJSON (flattened, %d bytes)", len(flat)), err) {
		fmt.Printf("        %s\n", compact(flat))
		got, _, err := jose.VerifyJSON(flat, &ecKey.PublicKey)
		ok(fmt.Sprintf("VerifyJSON (match=%v)", bytes.Equal(got, payload)), err)
	}
	// Unprotected headers cannot ride in the compact serialization.
	_, err = jose.Sign(payload, ecKey, jose.SignOptions{
		Algorithm:   jose.ES256,
		Unprotected: map[string]any{"x-note": "nope"},
	})
	mustFail("Sign (compact) with Unprotected set", err, jose.ErrInvalidHeader)

	// RFC 7797 §6 requires "b64" to be integrity protected. If a verifier reads
	// it from the merged (protected + unprotected) view, anyone can append
	// {"b64":false} to a genuine flattened JWS and change the payload the caller
	// receives — from the decoded octets to the base64url text — while the
	// signature still verifies.
	if flat != nil {
		injected := injectSignatureHeader(flat, "b64", false)
		got, _, err := jose.VerifyJSON(injected, &ecKey.PublicKey)
		switch {
		case err != nil:
			fmt.Printf("  [ ok ] %-52s rejected: %v\n", "unprotected {\"b64\":false} injected into a flat JWS", err)
		case bytes.Equal(got, payload):
			fmt.Printf("  [ ok ] %-52s ignored (payload unchanged)\n", "unprotected {\"b64\":false} injected into a JWS JSON")
		default:
			fmt.Printf("  [HOLE] %-52s VERIFIED, payload changed to %q\n",
				"unprotected {\"b64\":false} injected into a JWS JSON", abbrev(string(got)))
		}
	}

	// RFC 7515 §7.2.2: the flattened syntax carries its members at the top level
	// *instead of* "signatures". A document holding both is ambiguous.
	if flat != nil {
		mixed := setJSONMember(flat, "signature", "AAAA")
		mixed = setJSONMember(mixed, "protected", strings.Split(hsCompact, ".")[0])
		_, _, err = jose.VerifyJSON(mixed, &ecKey.PublicKey)
		weak("JWS JSON mixing the general and flattened forms", err)
	}

	// A "crit" placed directly in SignOptions.Header rather than in Critical.
	critInHeader, err := jose.Sign(payload, ecKey, jose.SignOptions{
		Algorithm: jose.ES256,
		Header:    map[string]any{"crit": []string{"x-tenant"}},
	})
	if err != nil {
		fmt.Printf("  [ ok ] %-52s rejected at signing: %v\n", "crit in Header, naming an absent parameter", err)
	} else {
		_, _, verr := jose.VerifyWithOptions(critInHeader, &ecKey.PublicKey,
			jose.VerifyOptions{KnownCritical: []string{"x-tenant"}})
		if verr != nil {
			fmt.Printf("  [HOLE] %-52s signed, but no recipient can read it: %v\n",
				"crit in Header, naming an absent parameter", verr)
		} else {
			fmt.Printf("  [ ok ] %-52s round-trips\n", "crit in Header, naming an absent parameter")
		}
	}

	multi, err := jose.SignJSONMulti(payload,
		jose.Signer{Key: rsaKey, Options: jose.SignOptions{Algorithm: jose.RS256, KeyID: "rsa-1"}},
		jose.Signer{Key: ecKey, Options: jose.SignOptions{Algorithm: jose.ES256, KeyID: "ec-1"}},
		jose.Signer{Key: edPriv, Options: jose.SignOptions{Algorithm: jose.EdDSA, KeyID: "ed-1"}},
	)
	if ok(fmt.Sprintf("SignJSONMulti with 3 signers (%d bytes)", len(multi)), err) {
		for _, v := range []struct {
			name string
			key  any
		}{{"RSA", &rsaKey.PublicKey}, {"EC", &ecKey.PublicKey}, {"Ed25519", edPub}, {"HMAC (not a signer)", secret}} {
			got, h, err := jose.VerifyJSON(multi, v.key)
			if v.name == "HMAC (not a signer)" {
				mustFail("multi-sig JWS verified with an unrelated key", err, nil)
				continue
			}
			ok(fmt.Sprintf("multi-sig JWS verifies with the %s key (alg=%v, match=%v)",
				v.name, h["alg"], bytes.Equal(got, payload)), err)
		}
	}

	// ------------------------------------------------------------------- JWE
	section("5. JWE compact: every key management family")

	plaintext := []byte("Attack at dawn. Bring the good coffee. " + strings.Repeat("padding ", 8))
	aes128 := mustRandom(16)
	aes256 := mustRandom(32)

	type encCase struct {
		alg      string
		enc      string
		encKey   any
		decKey   any
		wrongKey any
	}
	encCases := []encCase{
		{jose.RSA_OAEP_256, jose.A256GCM, &rsaKey.PublicKey, rsaKey, otherRSA},
		{jose.RSA_OAEP, jose.A128CBC_HS256, &rsaKey.PublicKey, rsaKey, nil},
		{jose.RSA1_5, jose.A128GCM, &rsaKey.PublicKey, rsaKey, nil},
		{jose.A128KW, jose.A128GCM, aes128, aes128, mustRandom(16)},
		{jose.A256KW, jose.A256CBC_HS512, aes256, aes256, nil},
		{jose.A256GCMKW, jose.A192GCM, aes256, aes256, nil},
		{jose.Dir, jose.A256GCM, aes256, aes256, mustRandom(32)},
		{jose.ECDH_ES, jose.A256GCM, &ecKey.PublicKey, ecKey, nil},
		{jose.ECDH_ES_A256KW, jose.A256GCM, &ecKey.PublicKey, ecKey, nil},
		{jose.ECDH_ES_A128KW, jose.A128CBC_HS256, ecP384.Public(), ecP384, nil},
		{jose.ECDH_ES, jose.A256GCM, xPriv.PublicKey(), xPriv, nil},
		{jose.PBES2_HS512_A256KW, jose.A256GCM, []byte("correct horse battery staple"), []byte("correct horse battery staple"), []byte("wrong password")},
	}

	for _, c := range encCases {
		label := c.alg + " / " + c.enc
		if c.alg == jose.ECDH_ES && isX25519(c.encKey) {
			label += " (X25519)"
		}
		token, err := jose.Encrypt(plaintext, c.encKey, jose.EncryptOptions{
			Algorithm:  c.alg,
			Encryption: c.enc,
			KeyID:      "recipient-1",
		})
		if !ok(fmt.Sprintf("Encrypt %-42s -> %d bytes, %d segments", label, len(token), len(strings.Split(token, "."))), err) {
			continue
		}
		got, hdr, err := jose.DecryptWithOptions(token, c.decKey, jose.DecryptOptions{
			Algorithms:  []string{c.alg},
			Encryptions: []string{c.enc},
		})
		h := jose.Header(hdr)
		ok(fmt.Sprintf("Decrypt %-42s match=%v alg=%q enc=%q", label, bytes.Equal(got, plaintext), h.Algorithm(), h.Encryption()), err)

		// Tamper with the ciphertext segment: the AEAD tag must catch it.
		seg := strings.Split(token, ".")
		seg[3] = flipFirst(seg[3])
		_, _, err = jose.Decrypt(strings.Join(seg, "."), c.decKey)
		mustFail(label+": ciphertext mutated", err, jose.ErrDecryptFailed)

		// Re-spell the ciphertext's final quantum with non-zero unused bits. The
		// octets are identical, so a strict decoder rejects the new spelling and
		// a lenient one silently accepts a second serialization of the same
		// token — which breaks any replay cache keyed on the token string.
		seg = strings.Split(token, ".")
		if respelt := nonCanonical(seg[3]); respelt != seg[3] && c.alg == jose.RSA_OAEP_256 {
			seg[3] = respelt
			_, _, err = jose.Decrypt(strings.Join(seg, "."), c.decKey)
			weak(label+": ciphertext re-spelled (same octets)", err)
		}

		// Tamper with the protected header (which is the AAD).
		seg = strings.Split(token, ".")
		seg[0] = forgeHeaderKid(seg[0], "recipient-2")
		_, _, err = jose.Decrypt(strings.Join(seg, "."), c.decKey)
		mustFail(label+": protected header mutated (AAD)", err, nil)

		if c.wrongKey != nil {
			_, _, err = jose.Decrypt(token, c.wrongKey)
			mustFail(label+": wrong key", err, jose.ErrDecryptFailed)
		}
	}

	section("6. JWE: inferred algorithms, compression, and limits")

	// Algorithm inference from the key type.
	inferred, err := jose.Encrypt(plaintext, &rsaKey.PublicKey, jose.EncryptOptions{})
	if ok("Encrypt with no Algorithm/Encryption (inferred from *rsa.PublicKey)", err) {
		_, hdr, err := jose.Decrypt(inferred, rsaKey)
		ok(fmt.Sprintf("  inferred alg=%q enc=%q", hdr["alg"], hdr["enc"]), err)
	}

	// zip=DEF
	compressible := []byte(strings.Repeat("the same line over and over\n", 400))
	zipped, err := jose.Encrypt(compressible, aes256, jose.EncryptOptions{
		Algorithm: jose.Dir, Encryption: jose.A256GCM, Compress: true,
	})
	if ok("Encrypt with Compress (zip=DEF)", err) {
		uncompressed, _ := jose.Encrypt(compressible, aes256, jose.EncryptOptions{
			Algorithm: jose.Dir, Encryption: jose.A256GCM,
		})
		fmt.Printf("        %d bytes plaintext -> %d bytes zipped vs %d bytes plain\n",
			len(compressible), len(zipped), len(uncompressed))
		got, hdr, err := jose.Decrypt(zipped, aes256)
		ok(fmt.Sprintf("Decrypt zip=%v (match=%v)", hdr["zip"], bytes.Equal(got, compressible)), err)
		// A tight MaxDecompressed must refuse the inflate.
		_, _, err = jose.DecryptWithOptions(zipped, aes256, jose.DecryptOptions{MaxDecompressed: 64})
		mustFail("zip=DEF with MaxDecompressed=64", err, jose.ErrCompressionLimit)
	}

	// PBES2 p2c bounds: attacker-supplied iteration counts are clamped.
	_, err = jose.Encrypt(plaintext, []byte("pw"), jose.EncryptOptions{
		Algorithm: jose.PBES2_HS256_A128KW, Encryption: jose.A128GCM,
		Header: map[string]any{"p2c": jose.MaxPBES2Count + 1},
	})
	mustFail(fmt.Sprintf("PBES2 with p2c=%d (> MaxPBES2Count)", jose.MaxPBES2Count+1), err, jose.ErrIterationCount)
	_, err = jose.Encrypt(plaintext, []byte("pw"), jose.EncryptOptions{
		Algorithm: jose.PBES2_HS256_A128KW, Encryption: jose.A128GCM,
		Header: map[string]any{"p2c": 10},
	})
	mustFail("PBES2 with p2c=10 (< MinPBES2Count)", err, jose.ErrIterationCount)

	// dir with a key of the wrong length for the enc.
	_, err = jose.Encrypt(plaintext, aes128, jose.EncryptOptions{Algorithm: jose.Dir, Encryption: jose.A256GCM})
	mustFail("dir with a 16-byte key for A256GCM", err, nil)

	// ContentEncryptionKeySize
	for _, enc := range []string{jose.A128GCM, jose.A256GCM, jose.A256CBC_HS512} {
		n, err := jose.ContentEncryptionKeySize(enc)
		ok(fmt.Sprintf("ContentEncryptionKeySize(%s) = %d", enc, n), err)
	}
	mustFail("ContentEncryptionKeySize(\"A999GCM\")", func() error {
		_, e := jose.ContentEncryptionKeySize("A999GCM")
		return e
	}(), jose.ErrUnsupportedEncryption)

	section("7. JWE JSON serializations, aad and multiple recipients")

	aad := []byte(`{"purpose":"audit"}`)
	jweFlat, err := jose.EncryptJSON(plaintext, &rsaKey.PublicKey, jose.EncryptOptions{
		Algorithm:                   jose.RSA_OAEP_256,
		Encryption:                  jose.A256GCM,
		Unprotected:                 map[string]any{"x-note": "shared unprotected"},
		AdditionalAuthenticatedData: aad,
	})
	if ok(fmt.Sprintf("EncryptJSON with aad + shared unprotected header (%d bytes)", len(jweFlat)), err) {
		fmt.Printf("        members: %v\n", jsonKeys(jweFlat))
		got, hdr, err := jose.DecryptJSON(jweFlat, rsaKey)
		ok(fmt.Sprintf("DecryptJSON (match=%v, x-note=%v)", bytes.Equal(got, plaintext), hdr["x-note"]), err)

		// Mutating "aad" must break the tag.
		mutated := mutateJSONMember(jweFlat, "aad", jose.EncodeSegment([]byte(`{"purpose":"other"}`)))
		_, _, err = jose.DecryptJSON(mutated, rsaKey)
		mustFail("aad member mutated", err, jose.ErrDecryptFailed)
	}
	// aad is meaningless in the compact form.
	_, err = jose.Encrypt(plaintext, &rsaKey.PublicKey, jose.EncryptOptions{
		Algorithm: jose.RSA_OAEP_256, AdditionalAuthenticatedData: aad,
	})
	mustFail("Encrypt (compact) with aad set", err, jose.ErrInvalidHeader)

	jweMulti, err := jose.EncryptJSONMulti(plaintext,
		jose.EncryptOptions{Encryption: jose.A256GCM},
		jose.Recipient{Key: &rsaKey.PublicKey, Algorithm: jose.RSA_OAEP_256, KeyID: "rsa-1"},
		jose.Recipient{Key: aes256, Algorithm: jose.A256KW, KeyID: "sym-1"},
		jose.Recipient{Key: &ecKey.PublicKey, Algorithm: jose.ECDH_ES_A256KW, KeyID: "ec-1"},
	)
	if ok(fmt.Sprintf("EncryptJSONMulti for 3 recipients (%d bytes)", len(jweMulti)), err) {
		for _, r := range []struct {
			name string
			key  any
		}{{"RSA", rsaKey}, {"AES-KW", aes256}, {"EC", ecKey}} {
			got, hdr, err := jose.DecryptJSON(jweMulti, r.key)
			ok(fmt.Sprintf("  recipient %-7s decrypts (alg=%v, match=%v)", r.name, hdr["alg"], bytes.Equal(got, plaintext)), err)
		}
		// The package documents a single, uniform ErrDecryptFailed for every
		// cryptographic failure so that nothing resembling an oracle leaks.
		_, _, err = jose.DecryptJSON(jweMulti, otherRSA)
		mustFail("  a fourth party with an unrelated RSA key", err, jose.ErrDecryptFailed)
	}

	// ------------------------------------------------------------------- JWK
	section("8. JWK, JWKSet, thumbprints and use/key_ops enforcement")

	for _, k := range []struct {
		name string
		key  any
	}{
		{"*rsa.PublicKey", &rsaKey.PublicKey},
		{"*ecdsa.PublicKey", &ecKey.PublicKey},
		{"ed25519.PublicKey", edPub},
		{"*ecdh.PublicKey (X25519)", xPriv.PublicKey()},
		{"[]byte (oct)", aes256},
	} {
		j, err := jose.FromKey(k.key)
		if !ok(fmt.Sprintf("FromKey(%-26s) -> kty=%s crv=%s", k.name, safeKty(j), safeCrv(j)), err) {
			continue
		}
		tp, err := j.Thumbprint()
		ok(fmt.Sprintf("  Thumbprint = %s", tp), err)
		back, err := j.Key()
		ok(fmt.Sprintf("  JWK.Key() -> %T", back), err)
	}

	// Private key -> JWK -> Public() view must drop the private parameters.
	privJWK, err := jose.FromKey(rsaKey)
	if ok(fmt.Sprintf("FromKey(*rsa.PrivateKey) (IsPrivate=%v)", privJWK != nil && privJWK.IsPrivate()), err) {
		pubJWK, err := privJWK.Public()
		ok(fmt.Sprintf("  Public() view: IsPrivate=%v d=%q p=%q", pubJWK.IsPrivate(), pubJWK.D, pubJWK.P), err)
	}

	// Build and publish a JWKSet, then use it to verify.
	sigJWK, _ := jose.FromKey(&ecKey.PublicKey)
	tp, _ := sigJWK.Thumbprint()
	sigJWK.Kid, sigJWK.Use, sigJWK.Alg = tp, "sig", jose.ES256
	encJWK, _ := jose.FromKey(&rsaKey.PublicKey)
	tp2, _ := encJWK.Thumbprint()
	encJWK.Kid, encJWK.Use, encJWK.Alg = tp2, "enc", jose.RSA_OAEP_256

	setJSON, err := json.Marshal(&jose.JWKSet{Keys: []*jose.JWK{sigJWK, encJWK}})
	ok(fmt.Sprintf("marshal a JWKSet with 2 annotated keys (%d bytes)", len(setJSON)), err)
	set, err := jose.ParseJWKSet(setJSON)
	ok("ParseJWKSet", err)
	if err == nil {
		found, okLookup := set.LookupKeyID(sigJWK.Kid)
		ok(fmt.Sprintf("LookupKeyID(%s…) found=%v use=%q alg=%q", sigJWK.Kid[:8], okLookup, found.Use, found.Alg), nil)
		_, missing := set.LookupKeyID("no-such-kid")
		ok(fmt.Sprintf("LookupKeyID(\"no-such-kid\") found=%v (expected false)", missing), nil)

		// A *JWK may be passed straight to Sign/Verify/Encrypt/Decrypt.
		signedWithJWK, err := jose.Sign(payload, ecKey, jose.SignOptions{Algorithm: jose.ES256, KeyID: sigJWK.Kid})
		ok("Sign with kid taken from the JWKSet", err)
		got, _, err := jose.Verify(signedWithJWK, found)
		ok(fmt.Sprintf("Verify passing the *JWK itself as the key (match=%v)", bytes.Equal(got, payload)), err)

		// RFC 7517 §4.2–§4.4: a key that states what it is for should not be
		// used for anything else. The "enc" key should not verify a signature,
		// and the "sig" key should not be used to encrypt.
		_, _, err = jose.Verify(rsToken, encJWK)
		weak("verify with a JWK declaring use=enc", err)
		_, err = jose.Encrypt(plaintext, found, jose.EncryptOptions{Algorithm: jose.ECDH_ES, Encryption: jose.A256GCM})
		weak("encrypt with a JWK declaring use=sig", err)
	}

	// key_ops (RFC 7517 §4.3).
	opsJWK, _ := jose.FromKey(&ecKey.PublicKey)
	opsJWK.KeyOps = []string{"deriveKey"}
	esToken, _ := jose.Sign(payload, ecKey, jose.SignOptions{Algorithm: jose.ES256})
	_, _, err = jose.Verify(esToken, opsJWK)
	weak("verify with a JWK whose key_ops omits \"verify\"", err)

	// A JWK declaring the wrong alg for the token (RFC 7517 §4.4).
	mismatch, _ := jose.FromKey(&ecKey.PublicKey)
	mismatch.Alg = jose.ES384
	_, _, err = jose.Verify(esToken, mismatch)
	weak("ES256 token against a JWK declaring alg=ES384", err)

	// Malformed JWK.
	mustFail("ParseJWK on an RSA key with a bad modulus", func() error {
		_, e := jose.ParseJWK([]byte(`{"kty":"RSA","n":"!!!","e":"AQAB"}`))
		return e
	}(), jose.ErrInvalidKey)
	mustFail("ParseJWK on an unknown kty", func() error {
		_, e := jose.ParseJWK([]byte(`{"kty":"MAGIC"}`))
		return e
	}(), nil)

	// ------------------------------------------------------------ primitives
	section("9. Exported primitives")

	kek := mustRandom(32)
	cek := mustRandom(32)
	wrapped, err := jose.AESKeyWrap(kek, cek)
	ok(fmt.Sprintf("AESKeyWrap: %d -> %d bytes", len(cek), len(wrapped)), err)
	unwrapped, err := jose.AESKeyUnwrap(kek, wrapped)
	ok(fmt.Sprintf("AESKeyUnwrap round-trip (match=%v)", bytes.Equal(unwrapped, cek)), err)
	badWrapped := append([]byte(nil), wrapped...)
	badWrapped[0] ^= 0xff
	mustFail("AESKeyUnwrap on a corrupted wrapping", func() error {
		_, e := jose.AESKeyUnwrap(kek, badWrapped)
		return e
	}(), nil)
	mustFail("AESKeyUnwrap with the wrong KEK", func() error {
		_, e := jose.AESKeyUnwrap(mustRandom(32), wrapped)
		return e
	}(), nil)

	// RFC 6070 PBKDF2-HMAC-SHA-256 style check: deterministic and stable.
	dk1, err := jose.PBKDF2([]byte("password"), []byte("salt"), 4096, 32, sha256.New)
	ok(fmt.Sprintf("PBKDF2(4096, 32) = %x…", firstN(dk1, 8)), err)
	dk2, _ := jose.PBKDF2([]byte("password"), []byte("salt"), 4096, 32, sha256.New)
	ok(fmt.Sprintf("PBKDF2 is deterministic (match=%v)", bytes.Equal(dk1, dk2)), nil)
	dk3, _ := jose.PBKDF2([]byte("password"), []byte("salt2"), 4096, 32, sha256.New)
	ok(fmt.Sprintf("PBKDF2 differs on a different salt (differs=%v)", !bytes.Equal(dk1, dk3)), nil)

	ck, err := jose.ConcatKDF(mustRandom(32), jose.A256GCM, []byte("Alice"), []byte("Bob"), 32)
	ok(fmt.Sprintf("ConcatKDF -> %d bytes (%x…)", len(ck), firstN(ck, 8)), err)

	fmt.Println("\nDone.")
}

// ---------------------------------------------------------------- helpers

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func mustRSA() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	must(err)
	return k
}

func mustRandom(n int) []byte {
	b := make([]byte, n)
	_, err := rand.Read(b)
	must(err)
	return b
}

func isX25519(key any) bool {
	_, ok := key.(*ecdh.PublicKey)
	return ok
}

func safeKty(j *jose.JWK) string {
	if j == nil {
		return "?"
	}
	return j.Kty
}

func safeCrv(j *jose.JWK) string {
	if j == nil || j.Crv == "" {
		return "-"
	}
	return j.Crv
}

// tamperPayload rewrites the JSON payload of a compact JWS, leaving the header
// and signature untouched.
func tamperPayload(token string, mutate func(map[string]any)) string {
	parts := strings.Split(token, ".")
	raw, err := jose.DecodeSegment(parts[1])
	must(err)
	var m map[string]any
	must(json.Unmarshal(raw, &m))
	mutate(m)
	out, err := json.Marshal(m)
	must(err)
	return parts[0] + "." + jose.EncodeSegment(out) + "." + parts[2]
}

// forgeHMAC builds a compact JWS by hand so that a secret jose.Sign refuses to
// use (an empty one) can still be probed on the verification side.
func forgeHMAC(payload []byte, alg string, secret []byte) (string, error) {
	hdr, err := json.Marshal(map[string]any{"alg": alg})
	if err != nil {
		return "", err
	}
	input := jose.EncodeSegment(hdr) + "." + jose.EncodeSegment(payload)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(input))
	return input + "." + jose.EncodeSegment(mac.Sum(nil)), nil
}

// padTo4 re-spells a base64url segment with the '=' padding RFC 7515 forbids.
func padTo4(seg string) string {
	for len(seg)%4 != 0 {
		seg += "="
	}
	return seg
}

// nonCanonical re-spells a base64url segment so that the unused low bits of its
// final quantum are non-zero. The octets it decodes to are unchanged, so a
// non-strict decoder gives one value several spellings.
func nonCanonical(seg string) string {
	if seg == "" {
		return seg
	}
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	last := strings.IndexByte(alphabet, seg[len(seg)-1])
	if last < 0 {
		return seg
	}
	switch len(seg) % 4 {
	case 2: // 4 unused bits
		return seg[:len(seg)-1] + string(alphabet[last|0x0f])
	case 3: // 2 unused bits
		return seg[:len(seg)-1] + string(alphabet[last|0x03])
	default: // no unused bits; nothing to flip
		return seg
	}
}

func setJSONMember(data []byte, member string, value any) []byte {
	var m map[string]json.RawMessage
	must(json.Unmarshal(data, &m))
	v, err := json.Marshal(value)
	must(err)
	m[member] = v
	out, err := json.Marshal(m)
	must(err)
	return out
}

func forgeNone(payload []byte) string {
	hdr, _ := json.Marshal(map[string]any{"alg": "none"})
	return jose.EncodeSegment(hdr) + "." + jose.EncodeSegment(payload) + "."
}

func stripToNone(token string) string {
	parts := strings.Split(token, ".")
	raw, err := jose.DecodeSegment(parts[0])
	must(err)
	var h map[string]any
	must(json.Unmarshal(raw, &h))
	h["alg"] = "none"
	out, err := json.Marshal(h)
	must(err)
	return jose.EncodeSegment(out) + "." + parts[1] + "."
}

func forgeHeaderKid(headerSeg, kid string) string {
	raw, err := jose.DecodeSegment(headerSeg)
	must(err)
	var h map[string]any
	must(json.Unmarshal(raw, &h))
	h["kid"] = kid
	out, err := json.Marshal(h)
	must(err)
	return jose.EncodeSegment(out)
}

// flipFirst changes the first character of a base64url segment, which always
// changes the decoded octets (unlike the last character, whose low bits may be
// unused padding bits).
func flipFirst(seg string) string {
	if seg == "" {
		return "AA"
	}
	b := []byte(seg)
	if b[0] == 'A' {
		b[0] = 'B'
	} else {
		b[0] = 'A'
	}
	return string(b)
}

// injectSignatureHeader adds an unprotected header parameter to every signature
// of a JWS JSON document, and to the flattened form's top-level "header".
func injectSignatureHeader(data []byte, name string, value any) []byte {
	var doc map[string]any
	must(json.Unmarshal(data, &doc))
	inject := func(m map[string]any) map[string]any {
		if m == nil {
			m = map[string]any{}
		}
		m[name] = value
		return m
	}
	if sigs, ok := doc["signatures"].([]any); ok {
		for i, s := range sigs {
			sm, _ := s.(map[string]any)
			if sm == nil {
				continue
			}
			hm, _ := sm["header"].(map[string]any)
			sm["header"] = inject(hm)
			sigs[i] = sm
		}
		doc["signatures"] = sigs
	}
	if _, flattened := doc["signature"]; flattened {
		hm, _ := doc["header"].(map[string]any)
		doc["header"] = inject(hm)
	}
	out, err := json.Marshal(doc)
	must(err)
	return out
}

func jsonKeys(data []byte) []string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

func mutateJSONMember(data []byte, member, value string) []byte {
	var m map[string]json.RawMessage
	must(json.Unmarshal(data, &m))
	v, err := json.Marshal(value)
	must(err)
	m[member] = v
	out, err := json.Marshal(m)
	must(err)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func firstN(b []byte, n int) []byte {
	if len(b) < n {
		return b
	}
	return b[:n]
}

func compact(data []byte) string {
	s := string(data)
	if len(s) <= 160 {
		return s
	}
	return s[:150] + "…"
}

func abbrev(s string) string {
	if len(s) <= 48 {
		return s
	}
	return s[:24] + "…" + s[len(s)-16:]
}
