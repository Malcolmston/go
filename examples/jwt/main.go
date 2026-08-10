// Command jwt-example exercises github.com/malcolmston/jwt: signing and
// verifying with HMAC, RSA, ECDSA and Ed25519, registered-claim validation
// (exp/nbf/aud/iss/sub), tamper detection, algorithm-confusion defences,
// JWK/JWKS key handling, PEM round-trips, detached payloads (RFC 7797) and the
// "none" algorithm policy.
//
// No network access; every key is generated in-process.
package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/malcolmston/jwt"
)

func section(name string) {
	fmt.Printf("\n=== %s ===\n", name)
}

func ok(label string, err error) {
	if err != nil {
		fmt.Printf("  [FAIL] %-46s %v\n", label, err)
		return
	}
	fmt.Printf("  [ ok ] %s\n", label)
}

// mustFail reports success when err is non-nil and matches want (if want != nil).
func mustFail(label string, err error, want error) {
	switch {
	case err == nil:
		fmt.Printf("  [FAIL] %-46s expected an error, got nil\n", label)
	case want != nil && !errors.Is(err, want):
		fmt.Printf("  [WARN] %-46s rejected, but not as %v: %v\n", label, want, err)
	default:
		fmt.Printf("  [ ok ] %-46s rejected: %v\n", label, err)
	}
}

func main() {
	fmt.Println("github.com/malcolmston/jwt — example program")
	fmt.Println("registered algorithms:", strings.Join(jwt.GetAlgorithms(), " "))

	secret := []byte("correct-horse-battery-staple")
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	clock := jwt.ClockFunc(func() time.Time { return now })

	baseClaims := func() jwt.RegisteredClaims {
		return jwt.RegisteredClaims{
			Issuer:    "auth.example.com",
			Subject:   "user-42",
			Audience:  jwt.ClaimStrings{"api.example.com", "admin.example.com"},
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now.Add(-time.Minute)),
			ID:        "tok-0001",
		}
	}

	// ---------------------------------------------------------------- HMAC
	section("1. HS256 sign + verify + claim validation")

	hsToken, err := jwt.Sign(baseClaims(), jwt.SigningMethodHS256, secret)
	ok("Sign(HS256)", err)
	fmt.Printf("        token: %s...%s (%d bytes)\n", hsToken[:24], hsToken[len(hsToken)-8:], len(hsToken))

	hmacKeyfunc := func(*jwt.Token) (any, error) { return secret, nil }

	var got jwt.RegisteredClaims
	tok, err := jwt.ParseWithClaims(hsToken, &got, hmacKeyfunc,
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithClock(clock),
		jwt.WithAudience("api.example.com"),
		jwt.WithIssuer("auth.example.com"),
		jwt.WithSubject("user-42"),
		jwt.WithIssuedAt(),
		jwt.WithExpirationRequired(),
		jwt.WithRequiredClaims("jti", "sub"),
		jwt.WithLeeway(5*time.Second),
	)
	ok("ParseWithClaims with full validation", err)
	if err == nil {
		fmt.Printf("        valid=%v alg=%q typ=%q sub=%q aud=%v jti=%q\n",
			tok.Valid, tok.Header["alg"], tok.TokenType(), got.Subject, got.Audience, got.ID)
	}

	// Parse into MapClaims and use the map helpers.
	mtok, err := jwt.Parse(hsToken, hmacKeyfunc, jwt.WithValidMethods([]string{"HS256"}), jwt.WithClock(clock))
	ok("Parse into MapClaims", err)
	if err == nil {
		mc, _ := mtok.Claims.(jwt.MapClaims)
		fmt.Printf("        MapClaims: iss=%q sub=%q jti=%q has(nbf)=%v aud=%v\n",
			mc.GetIssuer(), mc.GetSubject(), mc.GetID(), mc.Has("nbf"), mc.GetAudience())
		expT, okT := mc.GetTime("exp")
		fmt.Printf("        GetTime(exp)=%v (ok=%v)  VerifyIssuer=%v\n",
			expT.UTC().Format(time.RFC3339), okT, mc.VerifyIssuer("auth.example.com", true))
	}

	section("2. Claim validation failures")

	// exp in the past.
	expired := baseClaims()
	expired.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Hour))
	expiredTok, _ := jwt.Sign(expired, jwt.SigningMethodHS256, secret)
	_, err = jwt.Parse(expiredTok, hmacKeyfunc, jwt.WithValidMethods([]string{"HS256"}), jwt.WithClock(clock))
	mustFail("expired token (exp in the past)", err, jwt.ErrTokenExpired)

	// ... but WithIgnoreExpiration lets it through.
	_, err = jwt.Parse(expiredTok, hmacKeyfunc, jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithClock(clock), jwt.WithIgnoreExpiration())
	ok("same token accepted with WithIgnoreExpiration", err)

	// nbf in the future.
	future := baseClaims()
	future.NotBefore = jwt.NewNumericDate(now.Add(time.Hour))
	futureTok, _ := jwt.Sign(future, jwt.SigningMethodHS256, secret)
	_, err = jwt.Parse(futureTok, hmacKeyfunc, jwt.WithValidMethods([]string{"HS256"}), jwt.WithClock(clock))
	mustFail("not-yet-valid token (nbf in the future)", err, jwt.ErrTokenNotValidYet)

	// Wrong audience / issuer / subject.
	_, err = jwt.Parse(hsToken, hmacKeyfunc, jwt.WithClock(clock), jwt.WithAudience("evil.example.com"))
	mustFail("wrong audience", err, jwt.ErrTokenInvalidAudience)
	_, err = jwt.Parse(hsToken, hmacKeyfunc, jwt.WithClock(clock), jwt.WithIssuer("evil.example.com"))
	mustFail("wrong issuer", err, jwt.ErrTokenInvalidIssuer)
	_, err = jwt.Parse(hsToken, hmacKeyfunc, jwt.WithClock(clock), jwt.WithSubject("someone-else"))
	mustFail("wrong subject", err, jwt.ErrTokenInvalidSubject)

	// Missing required claim.
	_, err = jwt.Parse(hsToken, hmacKeyfunc, jwt.WithClock(clock), jwt.WithRequiredClaims("scope"))
	mustFail("missing required claim \"scope\"", err, jwt.ErrTokenRequiredClaimMissing)

	// Max token age.
	_, err = jwt.Parse(hsToken, hmacKeyfunc, jwt.WithClock(clock), jwt.WithMaxTokenAge(10*time.Second))
	mustFail("token older than WithMaxTokenAge(10s)", err, nil)

	// Standalone Validator over claims obtained without verification.
	var unverified jwt.RegisteredClaims
	_, parts, err := jwt.ParseUnverified(hsToken, &unverified)
	ok(fmt.Sprintf("ParseUnverified (%d parts, sub=%q)", len(parts), unverified.Subject), err)
	err = jwt.NewValidator(jwt.WithClock(clock), jwt.WithIssuer("auth.example.com")).Validate(unverified)
	ok("NewValidator(...).Validate on those claims", err)
	err = jwt.NewValidator(jwt.WithClock(clock), jwt.WithAudience("nope")).Validate(unverified)
	mustFail("Validator with wrong audience", err, jwt.ErrTokenInvalidAudience)

	// ------------------------------------------------------------- tamper
	section("3. Tamper detection")

	// Flip the payload but keep the original signature.
	tampered := tamperPayload(hsToken, func(m map[string]any) { m["sub"] = "admin" })
	_, err = jwt.Parse(tampered, hmacKeyfunc, jwt.WithValidMethods([]string{"HS256"}), jwt.WithClock(clock))
	mustFail("payload mutated, signature reused", err, jwt.ErrSignatureInvalid)

	// Mutate the last byte of the signature segment.
	seg := strings.Split(hsToken, ".")
	sigRunes := []byte(seg[2])
	if sigRunes[len(sigRunes)-1] == 'A' {
		sigRunes[len(sigRunes)-1] = 'B'
	} else {
		sigRunes[len(sigRunes)-1] = 'A'
	}
	_, err = jwt.Parse(seg[0]+"."+seg[1]+"."+string(sigRunes), hmacKeyfunc,
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithClock(clock))
	mustFail("signature byte flipped", err, jwt.ErrSignatureInvalid)

	// Wrong secret.
	_, err = jwt.Parse(hsToken, func(*jwt.Token) (any, error) { return []byte("wrong-secret"), nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithClock(clock))
	mustFail("wrong HMAC secret", err, jwt.ErrSignatureInvalid)

	// Truncated / malformed.
	_, err = jwt.Parse(seg[0]+"."+seg[1], hmacKeyfunc, jwt.WithClock(clock))
	mustFail("only two segments", err, jwt.ErrTokenMalformed)

	// ---------------------------------------------------- asymmetric algs
	section("4. Asymmetric signing: RS256 / PS256 / ES256 / EdDSA")

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}

	type algCase struct {
		name     string
		method   jwt.SigningMethod
		priv     any
		pub      any
		wrongPub any
	}
	cases := []algCase{
		{"RS256", jwt.SigningMethodRS256, rsaKey, &rsaKey.PublicKey, &mustRSA().PublicKey},
		{"PS256", jwt.SigningMethodPS256, rsaKey, &rsaKey.PublicKey, nil},
		{"ES256", jwt.SigningMethodES256, ecKey, &ecKey.PublicKey, nil},
		{"EdDSA", jwt.SigningMethodEdDSA, edPriv, edPub, nil},
	}

	for _, c := range cases {
		signed, err := jwt.Sign(baseClaims(), c.method, c.priv)
		if err != nil {
			fmt.Printf("  [FAIL] Sign(%s): %v\n", c.name, err)
			continue
		}
		var rc jwt.RegisteredClaims
		_, err = jwt.ParseWithClaims(signed, &rc, func(*jwt.Token) (any, error) { return c.pub, nil },
			jwt.WithValidMethods([]string{c.name}), jwt.WithClock(clock), jwt.WithIssuer("auth.example.com"))
		ok(fmt.Sprintf("%s round-trip (sub=%q, sig %d bytes)", c.name, rc.Subject, sigLen(signed)), err)

		// Tampered payload must be rejected for every algorithm.
		bad := tamperPayload(signed, func(m map[string]any) { m["sub"] = "root" })
		_, err = jwt.Parse(bad, func(*jwt.Token) (any, error) { return c.pub, nil },
			jwt.WithValidMethods([]string{c.name}), jwt.WithClock(clock))
		mustFail(c.name+": tampered payload", err, jwt.ErrSignatureInvalid)

		if c.wrongPub != nil {
			_, err = jwt.Parse(signed, func(*jwt.Token) (any, error) { return c.wrongPub, nil },
				jwt.WithValidMethods([]string{c.name}), jwt.WithClock(clock))
			mustFail(c.name+": verified with a different key", err, jwt.ErrSignatureInvalid)
		}
	}

	section("5. Algorithm confusion and \"none\"")

	rsToken, _ := jwt.Sign(baseClaims(), jwt.SigningMethodRS256, rsaKey)

	// Classic attack: re-sign as HS256 using the RSA public key bytes as the
	// HMAC secret, then offer it to a verifier that has no WithValidMethods.
	pubPEM, err := jwt.EncodeRSAPublicKeyToPEM(&rsaKey.PublicKey)
	ok("EncodeRSAPublicKeyToPEM", err)
	forged, err := jwt.Sign(baseClaims(), jwt.SigningMethodHS256, pubPEM)
	ok("attacker signs HS256 with the public key PEM as secret", err)
	_, err = jwt.Parse(forged, func(*jwt.Token) (any, error) { return &rsaKey.PublicKey, nil }, jwt.WithClock(clock))
	mustFail("HS256-for-RS256 confusion (keyfunc returns *rsa.PublicKey)", err, jwt.ErrInvalidKeyType)

	// WithValidMethods pins the algorithm.
	_, err = jwt.Parse(rsToken, func(*jwt.Token) (any, error) { return &rsaKey.PublicKey, nil },
		jwt.WithValidMethods([]string{"ES256"}), jwt.WithClock(clock))
	mustFail("RS256 token offered to a verifier pinned to ES256", err, nil)

	// "none": must not be accepted by default.
	noneTok, err := jwt.Sign(baseClaims(), jwt.SigningMethodNoneAlg, jwt.UnsafeAllowNoneSignatureType)
	ok("Sign with alg=none (explicit sentinel key)", err)
	_, err = jwt.Parse(noneTok, func(*jwt.Token) (any, error) { return jwt.UnsafeAllowNoneSignatureType, nil },
		jwt.WithClock(clock))
	mustFail("alg=none without WithAllowNone", err, nil)
	_, err = jwt.Parse(noneTok, hmacKeyfunc, jwt.WithClock(clock), jwt.WithAllowNone())
	mustFail("alg=none with WithAllowNone but a non-sentinel key", err, nil)
	_, err = jwt.Parse(noneTok, func(*jwt.Token) (any, error) { return jwt.UnsafeAllowNoneSignatureType, nil },
		jwt.WithClock(clock), jwt.WithAllowNone())
	ok("alg=none accepted only with WithAllowNone + sentinel key (opt-in twice)", err)

	// Stripping the signature off a real HS256 token and relabelling it "none".
	stripped := stripToNone(hsToken)
	_, err = jwt.Parse(stripped, func(*jwt.Token) (any, error) { return secret, nil },
		jwt.WithClock(clock), jwt.WithAllowNone())
	mustFail("HS256 token downgraded to alg=none", err, nil)

	// --------------------------------------------------------------- keys
	section("6. PEM round-trip")

	rsaPrivPEM := jwt.EncodeRSAPrivateKeyToPEM(rsaKey)
	rsaBack, err := jwt.ParseRSAPrivateKeyFromPEM(rsaPrivPEM)
	ok(fmt.Sprintf("RSA private PEM round-trip (equal=%v)", rsaBack != nil && rsaBack.Equal(rsaKey)), err)
	rsaPubBack, err := jwt.ParseRSAPublicKeyFromPEM(pubPEM)
	ok(fmt.Sprintf("RSA public PEM round-trip (equal=%v)", rsaPubBack != nil && rsaPubBack.Equal(&rsaKey.PublicKey)), err)

	ecPrivPEM, err := jwt.EncodeECPrivateKeyToPEM(ecKey)
	ok("EncodeECPrivateKeyToPEM", err)
	ecBack, err := jwt.ParseECPrivateKeyFromPEM(ecPrivPEM)
	ok(fmt.Sprintf("EC private PEM round-trip (equal=%v)", ecBack != nil && ecBack.Equal(ecKey)), err)

	edPrivPEM, err := jwt.EncodeEdPrivateKeyToPEM(edPriv)
	ok("EncodeEdPrivateKeyToPEM", err)
	edBack, err := jwt.ParseEdPrivateKeyFromPEM(edPrivPEM)
	ok(fmt.Sprintf("Ed25519 private PEM round-trip (equal=%v)", edBack != nil && edBack.Equal(edPriv)), err)

	genericPEM, err := jwt.EncodePublicKeyToPEM(&ecKey.PublicKey)
	ok("EncodePublicKeyToPEM(*ecdsa.PublicKey)", err)
	_, err = jwt.ParseECPublicKeyFromPEM(genericPEM)
	ok("ParseECPublicKeyFromPEM on it", err)

	mustFail("ParseRSAPrivateKeyFromPEM on garbage", func() error {
		_, e := jwt.ParseRSAPrivateKeyFromPEM([]byte("not a pem"))
		return e
	}(), jwt.ErrKeyMustBePEMEncoded)

	// ------------------------------------------------------------ JWK/JWKS
	section("7. JWK and JWKS")

	rsaJWK, err := jwt.NewJSONWebKey(&rsaKey.PublicKey)
	ok("NewJSONWebKey(*rsa.PublicKey)", err)
	ecJWK, err := jwt.NewJSONWebKey(&ecKey.PublicKey)
	ok("NewJSONWebKey(*ecdsa.PublicKey)", err)
	edJWK, err := jwt.NewJSONWebKey(edPub)
	ok("NewJSONWebKey(ed25519.PublicKey)", err)

	rsaKID, err := jwt.ComputeKeyID(&rsaKey.PublicKey)
	ok(fmt.Sprintf("ComputeKeyID(RSA) = %s", rsaKID), err)
	ecKID, _ := jwt.ComputeKeyID(&ecKey.PublicKey)
	edKID, _ := jwt.ComputeKeyID(edPub)

	thumbURI, err := rsaJWK.ThumbprintURI(crypto.SHA256)
	ok("ThumbprintURI = "+thumbURI, err)

	set := (&jwt.JSONWebKeySet{}).Add(
		*rsaJWK.WithKeyID(rsaKID).WithAlgorithm("RS256").WithUse("sig"),
		*ecJWK.WithKeyID(ecKID).WithAlgorithm("ES256").WithUse("sig"),
		*edJWK.WithKeyID(edKID).WithAlgorithm("EdDSA").WithUse("sig"),
	)
	jwksJSON, err := json.Marshal(set)
	ok(fmt.Sprintf("marshal JWKS with %d keys (%d bytes)", len(set.Keys), len(jwksJSON)), err)

	// Re-parse the published JWKS (this is what a resource server would fetch)
	// and verify tokens through its Keyfunc, selected by "kid".
	parsedSet, err := jwt.ParseJWKSet(jwksJSON)
	ok("ParseJWKSet", err)
	if err == nil {
		keyfunc := parsedSet.Keyfunc()
		for _, c := range []struct {
			alg    string
			kid    string
			method jwt.SigningMethod
			priv   any
		}{
			{"RS256", rsaKID, jwt.SigningMethodRS256, rsaKey},
			{"ES256", ecKID, jwt.SigningMethodES256, ecKey},
			{"EdDSA", edKID, jwt.SigningMethodEdDSA, edPriv},
		} {
			t := jwt.NewWithClaims(c.method, baseClaims()).SetKID(c.kid)
			signed, err := t.SignedString(c.priv)
			if err != nil {
				fmt.Printf("  [FAIL] sign %s with kid: %v\n", c.alg, err)
				continue
			}
			parsed, err := jwt.Parse(signed, keyfunc,
				jwt.WithValidMethods([]string{"RS256", "ES256", "EdDSA"}), jwt.WithClock(clock))
			ok(fmt.Sprintf("JWKS Keyfunc resolved kid=%s… for %s (valid=%v)",
				c.kid[:8], c.alg, parsed != nil && parsed.Valid), err)
		}

		// Unknown kid must not resolve.
		unknown := jwt.NewWithClaims(jwt.SigningMethodRS256, baseClaims()).SetKID("no-such-kid")
		s, _ := unknown.SignedString(rsaKey)
		_, err = jwt.Parse(s, keyfunc, jwt.WithValidMethods([]string{"RS256"}), jwt.WithClock(clock))
		mustFail("unknown kid", err, jwt.ErrKeyNotFound)

		// The JWKS Keyfunc also pins the JWK's declared "alg": an RS256-declared
		// key must not verify a PS256 token.
		ps := jwt.NewWithClaims(jwt.SigningMethodPS256, baseClaims()).SetKID(rsaKID)
		s2, _ := ps.SignedString(rsaKey)
		_, err = jwt.Parse(s2, keyfunc, jwt.WithValidMethods([]string{"PS256"}), jwt.WithClock(clock))
		mustFail("PS256 token against a JWK declaring alg=RS256", err, nil)
	}

	// Symmetric ("oct") JWK.
	octJWK, err := jwt.NewJSONWebKey(secret)
	ok("NewJSONWebKey([]byte) -> kty=oct", err)
	if err == nil {
		b, _ := json.Marshal(octJWK)
		back, err := jwt.ParseJWK(b)
		ok(fmt.Sprintf("oct JWK round-trip (kty=%s, private=%v)", back.Kty, back.IsPrivate()), err)
	}

	// A private JWK's Public() view must not leak "d".
	rsaPrivJWK, err := jwt.NewJSONWebKey(rsaKey)
	ok(fmt.Sprintf("NewJSONWebKey(*rsa.PrivateKey) (IsPrivate=%v)", rsaPrivJWK != nil && rsaPrivJWK.IsPrivate()), err)
	if err == nil {
		pubView := rsaPrivJWK.Public()
		fmt.Printf("        Public() view: IsPrivate=%v d=%q p=%q\n", pubView.IsPrivate(), pubView.D, pubView.P)
	}

	// ------------------------------------------------------ RFC 7797 detached
	section("8. Detached payload (RFC 7797)")

	payload := []byte(`{"event":"transfer","amount":"100.00"}`)
	detached, err := jwt.SignDetached(payload, jwt.SigningMethodEdDSA, edPriv, nil)
	ok("SignDetached(EdDSA)", err)
	if err == nil {
		fmt.Printf("        detached JWS: %s (payload segment empty: %v)\n",
			abbrev(detached), strings.Split(detached, ".")[1] == "")
		_, err = jwt.VerifyDetached(detached, payload, func(*jwt.Token) (any, error) { return edPub, nil },
			jwt.WithValidMethods([]string{"EdDSA"}))
		ok("VerifyDetached with the correct payload", err)
		_, err = jwt.VerifyDetached(detached, []byte(`{"event":"transfer","amount":"999999.00"}`),
			func(*jwt.Token) (any, error) { return edPub, nil }, jwt.WithValidMethods([]string{"EdDSA"}))
		mustFail("VerifyDetached with a mutated payload", err, jwt.ErrSignatureInvalid)
	}

	// ---------------------------------------------------------- misc headers
	section("9. Headers, crit and typ")

	crit := jwt.NewWithClaims(jwt.SigningMethodHS256, baseClaims())
	crit.SetHeader("crit", []any{"x-example"}).SetHeader("x-example", "value").SetType("at+jwt")
	critTok, err := crit.SignedString(secret)
	ok("sign a token with crit + typ=at+jwt", err)
	_, err = jwt.Parse(critTok, hmacKeyfunc, jwt.WithClock(clock))
	mustFail("undeclared critical header \"x-example\"", err, nil)
	parsedCrit, err := jwt.Parse(critTok, hmacKeyfunc, jwt.WithClock(clock),
		jwt.WithKnownCriticalHeaders("x-example"), jwt.WithValidTypes("at+jwt"))
	ok("accepted with WithKnownCriticalHeaders + WithValidTypes", err)
	if err == nil {
		fmt.Printf("        typ=%q crit=%v\n", parsedCrit.TokenType(), parsedCrit.Header["crit"])
	}
	_, err = jwt.Parse(critTok, hmacKeyfunc, jwt.WithClock(clock),
		jwt.WithKnownCriticalHeaders("x-example"), jwt.WithValidTypes("JWT"))
	mustFail("typ=at+jwt against WithValidTypes(\"JWT\")", err, nil)

	// Reusable parser + Token.String()
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}), jwt.WithClock(clock),
		jwt.WithIssuer("auth.example.com"))
	for i := 0; i < 3; i++ {
		if _, err := parser.Parse(hsToken, hmacKeyfunc); err != nil {
			ok(fmt.Sprintf("reused Parser, call %d", i+1), err)
		}
	}
	ok("reused Parser across 3 calls", nil)
	reparsed, _ := parser.Parse(hsToken, hmacKeyfunc)
	ok(fmt.Sprintf("Token.String() reproduces the input: %v", reparsed.String() == hsToken), nil)

	// Empty HMAC secret must fail closed.
	mustFail("HS256 verify with a zero-length secret", func() error {
		_, e := jwt.Parse(hsToken, func(*jwt.Token) (any, error) { return []byte{}, nil },
			jwt.WithValidMethods([]string{"HS256"}), jwt.WithClock(clock))
		return e
	}(), nil)

	// Unregistered alg.
	mustFail("token with an unregistered alg", func() error {
		fake := forgeHeaderAlg(hsToken, "HS999")
		_, e := jwt.Parse(fake, hmacKeyfunc, jwt.WithClock(clock))
		return e
	}(), jwt.ErrSigningMethodUnavailable)

	fmt.Println("\nDone.")
}

// ---------------------------------------------------------------- helpers

func mustRSA() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}

// tamperPayload decodes the payload segment, applies mutate, and re-encodes it,
// leaving the header and signature untouched.
func tamperPayload(token string, mutate func(map[string]any)) string {
	parts := strings.Split(token, ".")
	raw, err := jwt.DecodeSegment(parts[1])
	if err != nil {
		panic(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		panic(err)
	}
	mutate(m)
	out, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return parts[0] + "." + jwt.EncodeSegment(out) + "." + parts[2]
}

// stripToNone rewrites the header's alg to "none" and drops the signature.
func stripToNone(token string) string {
	parts := strings.Split(token, ".")
	return forgeHeader(parts[0], "none") + "." + parts[1] + "."
}

func forgeHeaderAlg(token, alg string) string {
	parts := strings.Split(token, ".")
	return forgeHeader(parts[0], alg) + "." + parts[1] + "." + parts[2]
}

func forgeHeader(headerSeg, alg string) string {
	raw, err := jwt.DecodeSegment(headerSeg)
	if err != nil {
		panic(err)
	}
	var h map[string]any
	if err := json.Unmarshal(raw, &h); err != nil {
		panic(err)
	}
	h["alg"] = alg
	out, err := json.Marshal(h)
	if err != nil {
		panic(err)
	}
	return jwt.EncodeSegment(out)
}

func sigLen(token string) int {
	parts := strings.Split(token, ".")
	b, err := jwt.DecodeSegment(parts[2])
	if err != nil {
		return -1
	}
	return len(b)
}

func abbrev(s string) string {
	if len(s) <= 40 {
		return s
	}
	return s[:20] + "…" + s[len(s)-16:]
}
