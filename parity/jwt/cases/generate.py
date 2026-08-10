#!/usr/bin/env python3
"""Regenerates parity/jwt/cases/*.json.

Run from the repository root:

    python3 parity/jwt/cases/generate.py

Fixture tokens for the cross-verification cases are minted by running the two
parity runners themselves in `sign` mode, then committed into the case files, so
the suite never signs anything at test time and stays deterministic. Keys are the
fixed PEMs in parity/jwt/keys/, generated once with openssl and committed.

Regenerating changes the RS*/PS*/ES* fixture signature bytes (those algorithms
are randomised) but not the meaning of any case.
"""
import base64, json, os, subprocess, sys

D = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CASES = os.path.join(D, "cases")
UPSTREAM = "jsonwebtoken@9.0.2"
SECRET = "parity-hmac-secret-256-bits-long-enough"
OTHER_SECRET = "a-completely-different-secret-value"

HS = {"kind": "oct", "secret": SECRET}
HS_WRONG = {"kind": "oct", "secret": OTHER_SECRET}
RSA_PRIV = {"kind": "rsa", "file": "rsa-2048.priv.pem", "private": True}
RSA_PUB = {"kind": "rsa", "file": "rsa-2048.pub.pem"}
RSA_PUB_OTHER = {"kind": "rsa", "file": "rsa-2048-other.pub.pem"}
ED_PRIV = {"kind": "ed", "file": "ed25519.priv.pem", "private": True}
ED_PUB = {"kind": "ed", "file": "ed25519.pub.pem"}


def ec(curve, priv=False):
    return {"kind": "ec", "file": "ec-%s.%s.pem" % (curve, "priv" if priv else "pub"),
            **({"private": True} if priv else {})}


NOW = 1700000000
LATER = NOW + 100

# alg -> (signing key, verification key, an unrelated verification key)
ALGS = {
    "HS256": (HS, HS, HS_WRONG),
    "HS384": (HS, HS, HS_WRONG),
    "HS512": (HS, HS, HS_WRONG),
    "RS256": (RSA_PRIV, RSA_PUB, RSA_PUB_OTHER),
    "RS384": (RSA_PRIV, RSA_PUB, RSA_PUB_OTHER),
    "RS512": (RSA_PRIV, RSA_PUB, RSA_PUB_OTHER),
    "PS256": (RSA_PRIV, RSA_PUB, RSA_PUB_OTHER),
    "PS384": (RSA_PRIV, RSA_PUB, RSA_PUB_OTHER),
    "PS512": (RSA_PRIV, RSA_PUB, RSA_PUB_OTHER),
    "ES256": (ec("p256", True), ec("p256"), ec("p256-other")),
    "ES384": (ec("p384", True), ec("p384"), ec("p384-other")),
    "ES512": (ec("p521", True), ec("p521"), ec("p521-other")),
}


# ------------------------------------------------------------------ runners
def run(side, reqs):
    payload = "".join(json.dumps(r) + "\n" for r in reqs)
    if side == "node":
        cmd = ["node", os.path.join(D, "node", "run.js"), os.path.join(D, "keys")]
    else:
        cmd = ["go", "run", "./go", os.path.join(D, "keys")]
    env = dict(os.environ, GOWORK="off")
    p = subprocess.run(cmd, input=payload, capture_output=True, text=True, cwd=D, env=env)
    if p.returncode != 0:
        sys.exit("runner %s failed: %s" % (side, p.stderr[:2000]))
    out = {}
    for line in p.stdout.splitlines():
        if line.strip():
            o = json.loads(line)
            out[o["id"]] = o
    return out


def mint(side, specs):
    """specs: {id: (payload, keyspec, signopts)} -> {id: token}"""
    reqs = [{"id": i, "fn": "sign", "args": [pl, k, o]} for i, (pl, k, o) in specs.items()]
    res = run(side, reqs)
    toks = {}
    for i in specs:
        r = res[i]
        if not r.get("ok"):
            sys.exit("minting %s on %s failed: %s" % (i, side, r.get("error")))
        toks[i] = r["value"]
    return toks


def write(group, cases, note=None):
    doc = {"group": group, "upstream": UPSTREAM}
    if note:
        doc["note"] = note
    doc["cases"] = cases
    with open(os.path.join(CASES, group + ".json"), "w") as f:
        json.dump(doc, f, indent=2)
        f.write("\n")
    print("%-24s %3d cases" % (group + ".json", len(cases)))


def vo(**kw):
    o = {"clockTimestamp": kw.pop("now", LATER), "complete": True}
    o.update(kw)
    return o


# ================================================================== sign-hs
# HS*/none signing is deterministic, so the exact compact token is compared.
# Claim keys are listed alphabetically because the Go port serialises maps with
# sorted keys (encoding/json) while node preserves insertion order; see the
# sign-hs256-insertion-order deviation case and COVERAGE.md.
cases = []
for alg in ("HS256", "HS384", "HS512"):
    cases.append({
        "id": "sign-%s-exact" % alg.lower(), "fn": "sign",
        "args": [{"aud": "parity-aud", "exp": NOW + 3600, "iat": NOW, "iss": "parity-iss",
                  "jti": "id-1", "nbf": NOW - 10, "role": "admin", "sub": "user-1"}, HS,
                 {"algorithm": alg}],
        "upstreamFn": "jwt.sign", "goFn": "jwt.Sign/Token.SignedString",
        "note": "exact token string; HMAC is deterministic",
    })
cases += [
    {"id": "sign-hs256-minimal-exact", "fn": "sign",
     "args": [{"iat": NOW}, HS, {"algorithm": "HS256"}],
     "upstreamFn": "jwt.sign", "goFn": "jwt.NewWithClaims"},
    {"id": "sign-hs256-notimestamp-exact", "fn": "sign",
     "args": [{"exp": NOW + 60, "sub": "s"}, HS, {"algorithm": "HS256", "noTimestamp": True}],
     "upstreamFn": "jwt.sign options.noTimestamp", "goFn": "jwt.MapClaims (no iat)",
     "note": "exact token string"},
    {"id": "sign-hs256-default-alg-exact", "fn": "sign",
     "args": [{"iat": NOW}, HS, {}],
     "upstreamFn": "jwt.sign (default algorithm)", "goFn": "jwt.SigningMethodHS256",
     "note": "both default to HS256"},
    {"id": "sign-none-exact", "fn": "sign",
     "args": [{"iat": NOW}, {"kind": "none"}, {"algorithm": "none"}],
     "upstreamFn": "jwt.sign algorithm:'none'", "goFn": "jwt.SigningMethodNoneAlg",
     "note": "unsecured token; exact string, empty signature"},
    {"id": "sign-hs256-unknown-alg", "fn": "sign",
     "args": [{"iat": NOW}, HS, {"algorithm": "HS999"}],
     "upstreamFn": "jwt.sign", "goFn": "jwt.GetSigningMethod",
     "note": "both must fail: unknown alg"},
    {"id": "sign-hs256-insertion-order", "fn": "sign",
     "args": [{"sub": "user-1", "iat": NOW, "aud": "parity-aud"}, HS, {"algorithm": "HS256"}],
     "upstreamFn": "jwt.sign", "goFn": "jwt.Sign",
     "deviation": "the port serialises claims with sorted keys (Go encoding/json) while "
                  "node preserves insertion order, so the token bytes differ whenever the "
                  "claim order is not already alphabetical. Both tokens carry identical "
                  "claims and each side verifies the other's (see crossverify)."},
]
write("sign-hs", cases,
      "HS*/none are deterministic: the exact compact serialization is compared.")

# ============================================================= sign-headers
cases = []
for alg, (sk, _vk, _wk) in ALGS.items():
    cases.append({
        "id": "parts-%s" % alg.lower(), "fn": "tokenParts",
        "args": [{"exp": NOW + 3600, "iat": NOW, "sub": "user-1"}, sk, {"algorithm": alg}],
        "upstreamFn": "jwt.sign", "goFn": "jwt.Token.SignedString",
        "note": "decoded header+payload and signature length; signature bytes are "
                "randomised for PS*/ES* so they are not compared",
    })
cases += [
    {"id": "parts-keyid", "fn": "tokenParts",
     "args": [{"iat": NOW}, HS, {"algorithm": "HS256", "keyid": "key-2024"}],
     "upstreamFn": "jwt.sign options.keyid", "goFn": "jwt.Token.SetKID"},
    {"id": "parts-header-extra", "fn": "tokenParts",
     "args": [{"iat": NOW}, HS, {"algorithm": "HS256", "header": {"cty": "example", "x5t": "abc"}}],
     "upstreamFn": "jwt.sign options.header", "goFn": "jwt.Token.SetHeader"},
    {"id": "parts-header-typ", "fn": "tokenParts",
     "args": [{"iat": NOW}, HS, {"algorithm": "HS256", "header": {"typ": "at+jwt"}}],
     "upstreamFn": "jwt.sign options.header.typ", "goFn": "jwt.Token.SetType"},
]
write("sign-headers", cases,
      "Header/segment shape per algorithm, independent of randomised signature bytes.")

# ======================================================= sign-claim-options
cases = [
    {"id": "opt-expiresin-int", "fn": "tokenParts",
     "args": [{"iat": NOW}, HS, {"algorithm": "HS256", "expiresIn": 3600}],
     "upstreamFn": "jwt.sign options.expiresIn", "goFn": "jwt.MapClaims[\"exp\"]"},
    {"id": "opt-notbefore-int", "fn": "tokenParts",
     "args": [{"iat": NOW}, HS, {"algorithm": "HS256", "notBefore": 60}],
     "upstreamFn": "jwt.sign options.notBefore", "goFn": "jwt.MapClaims[\"nbf\"]"},
    {"id": "opt-audience-string", "fn": "tokenParts",
     "args": [{"iat": NOW}, HS, {"algorithm": "HS256", "audience": "svc-a"}],
     "upstreamFn": "jwt.sign options.audience", "goFn": "jwt.ClaimStrings"},
    {"id": "opt-audience-array", "fn": "tokenParts",
     "args": [{"iat": NOW}, HS, {"algorithm": "HS256", "audience": ["svc-a", "svc-b"]}],
     "upstreamFn": "jwt.sign options.audience", "goFn": "jwt.ClaimStrings"},
    {"id": "opt-issuer", "fn": "tokenParts",
     "args": [{"iat": NOW}, HS, {"algorithm": "HS256", "issuer": "https://iss.example"}],
     "upstreamFn": "jwt.sign options.issuer", "goFn": "jwt.RegisteredClaims.Issuer"},
    {"id": "opt-subject", "fn": "tokenParts",
     "args": [{"iat": NOW}, HS, {"algorithm": "HS256", "subject": "user-9"}],
     "upstreamFn": "jwt.sign options.subject", "goFn": "jwt.RegisteredClaims.Subject"},
    {"id": "opt-jwtid", "fn": "tokenParts",
     "args": [{"iat": NOW}, HS, {"algorithm": "HS256", "jwtid": "tok-1"}],
     "upstreamFn": "jwt.sign options.jwtid", "goFn": "jwt.RegisteredClaims.ID"},
    {"id": "opt-all-claims", "fn": "tokenParts",
     "args": [{"iat": NOW, "scope": "read"}, HS,
              {"algorithm": "HS256", "expiresIn": 900, "notBefore": 0, "audience": "svc-a",
               "issuer": "iss", "subject": "sub", "jwtid": "jti", "keyid": "kid"}],
     "upstreamFn": "jwt.sign options.*", "goFn": "jwt.MapClaims + Token.SetKID"},
    {"id": "opt-expiresin-timespan", "fn": "tokenParts",
     "args": [{"iat": NOW}, HS, {"algorithm": "HS256", "expiresIn": "1h"}],
     "upstreamFn": "jwt.sign options.expiresIn (string timespan)", "goFn": "—",
     "note": "upstream parses 'ms' timespan strings; the port has no sign options at all"},
    {"id": "opt-notbefore-timespan", "fn": "tokenParts",
     "args": [{"iat": NOW}, HS, {"algorithm": "HS256", "notBefore": "2 days"}],
     "upstreamFn": "jwt.sign options.notBefore (string timespan)", "goFn": "—",
     "note": "upstream parses 'ms' timespan strings"},
    {"id": "opt-expiresin-conflict", "fn": "tokenParts",
     "args": [{"exp": NOW + 5, "iat": NOW}, HS, {"algorithm": "HS256", "expiresIn": 3600}],
     "upstreamFn": "jwt.sign options.expiresIn", "goFn": "—",
     "note": "both must fail: exp claim and expiresIn option conflict"},
]
write("sign-claim-options", cases,
      "node-jsonwebtoken's sign-time option->claim projection. The Go port has no sign "
      "options; the Go runner sets the corresponding claim or header directly.")

# ================================================================ roundtrip
cases = []
for alg, (sk, vk, wk) in ALGS.items():
    cases.append({
        "id": "rt-%s" % alg.lower(), "fn": "roundtrip",
        "args": [{"exp": NOW + 3600, "iat": NOW, "sub": "user-1"}, sk, vk,
                 {"algorithm": alg}, vo(algorithms=[alg])],
        "upstreamFn": "jwt.sign + jwt.verify", "goFn": "jwt.Sign + jwt.Parse",
        "note": "sign then verify in-process; the only safe value comparison for the "
                "randomised PS*/ES* signatures",
    })
    cases.append({
        "id": "rt-%s-wrongkey" % alg.lower(), "fn": "roundtrip",
        "args": [{"exp": NOW + 3600, "iat": NOW}, sk, wk, {"algorithm": alg},
                 vo(algorithms=[alg])],
        "upstreamFn": "jwt.verify", "goFn": "jwt.Parse",
        "note": "both must fail: verification key is unrelated to the signing key",
    })
cases.append({
    "id": "rt-eddsa", "fn": "roundtrip",
    "args": [{"exp": NOW + 3600, "iat": NOW}, ED_PRIV, ED_PUB, {"algorithm": "EdDSA"},
             vo(algorithms=["EdDSA"])],
    "upstreamFn": "—", "goFn": "jwt.SigningMethodEdDSA",
    "deviation": "the port supports EdDSA (RFC 8037); jsonwebtoken@9.0.2 (jws@3) does not",
})
write("roundtrip", cases,
      "Per-algorithm sign-then-verify with fixed committed keys and a fixed clock.")

# =================================================== crossverify (fixtures)
# Sign on one side, verify on BOTH. This is the strongest parity evidence for
# the randomised algorithms: an upstream-produced token must verify in Go, and a
# Go-produced token must verify upstream.
specs = {"x-%s" % a.lower(): ({"exp": NOW + 3600, "iat": NOW, "sub": "cross"}, sk,
                              {"algorithm": a})
         for a, (sk, _v, _w) in ALGS.items()}
node_toks = mint("node", specs)
go_toks = mint("go", specs)
go_ed = mint("go", {"x-eddsa": ({"exp": NOW + 3600, "iat": NOW, "sub": "cross"}, ED_PRIV,
                                {"algorithm": "EdDSA"})})

cases = []
for alg, (_sk, vk, _wk) in ALGS.items():
    k = "x-%s" % alg.lower()
    cases.append({
        "id": "cross-upstream-signed-%s" % alg.lower(), "fn": "verify",
        "args": [node_toks[k], vk, vo(algorithms=[alg])],
        "upstreamFn": "jwt.verify", "goFn": "jwt.Parse",
        "note": "token minted by jsonwebtoken@9.0.2; both sides must verify it",
    })
    cases.append({
        "id": "cross-go-signed-%s" % alg.lower(), "fn": "verify",
        "args": [go_toks[k], vk, vo(algorithms=[alg])],
        "upstreamFn": "jwt.verify", "goFn": "jwt.Parse",
        "note": "token minted by github.com/malcolmston/jwt; both sides must verify it",
    })
cases.append({
    "id": "cross-go-signed-eddsa", "fn": "verify",
    "args": [go_ed["x-eddsa"], ED_PUB, vo(algorithms=["EdDSA"])],
    "upstreamFn": "—", "goFn": "jwt.Parse",
    "deviation": "EdDSA is port-only; jsonwebtoken@9.0.2 cannot verify it",
})
h, p, s = node_toks["x-rs256"].split(".")
mp = json.loads(base64.urlsafe_b64decode(p + "=="))
mp["sub"] = "attacker"
p2 = base64.urlsafe_b64encode(json.dumps(mp, separators=(",", ":")).encode()).decode().rstrip("=")
cases += [
    {"id": "cross-rs256-tampered-payload", "fn": "verify",
     "args": [".".join([h, p2, s]), RSA_PUB, vo(algorithms=["RS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse",
     "note": "both must fail: payload mutated under an otherwise valid RS256 signature"},
    {"id": "cross-rs256-wrong-public-key", "fn": "verify",
     "args": [node_toks["x-rs256"], RSA_PUB_OTHER, vo(algorithms=["RS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse",
     "note": "both must fail: unrelated RSA public key"},
]
write("crossverify", cases,
      "Committed fixture tokens: one set minted by the upstream runner, one by the Go "
      "runner. Both runners verify both sets.")

# ============================================================ verify-claims
base = mint("node", {
    "b": ({"exp": NOW + 3600, "iat": NOW}, HS, {"algorithm": "HS256"}),
    "nbf": ({"exp": NOW + 10000, "iat": NOW, "nbf": NOW + 9000}, HS, {"algorithm": "HS256"}),
    "full": ({"aud": "aud-a", "iat": NOW, "iss": "iss-a", "jti": "jti-a", "sub": "sub-a"}, HS,
             {"algorithm": "HS256"}),
    "audarr": ({"aud": ["a", "b"], "iat": NOW}, HS, {"algorithm": "HS256"}),
    "noexp": ({"iat": NOW}, HS, {"algorithm": "HS256"}),
    "iatfuture": ({"iat": NOW + 5000}, HS, {"algorithm": "HS256"}),
})


def V(tid, **kw):
    return [base[tid], HS, vo(**kw)]


cases = [
    {"id": "vc-exp-valid", "fn": "verify", "args": V("b", algorithms=["HS256"]),
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse"},
    {"id": "vc-exp-expired", "fn": "verify", "args": V("b", now=NOW + 9999, algorithms=["HS256"]),
     "upstreamFn": "TokenExpiredError", "goFn": "jwt.ErrTokenExpired", "note": "both must fail"},
    {"id": "vc-exp-boundary", "fn": "verify", "args": V("b", now=NOW + 3600, algorithms=["HS256"]),
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse", "note": "clock exactly at exp"},
    {"id": "vc-clocktolerance-saves", "fn": "verify",
     "args": V("b", now=NOW + 3700, clockTolerance=200, algorithms=["HS256"]),
     "upstreamFn": "options.clockTolerance", "goFn": "jwt.WithLeeway"},
    {"id": "vc-clocktolerance-insufficient", "fn": "verify",
     "args": V("b", now=NOW + 3700, clockTolerance=10, algorithms=["HS256"]),
     "upstreamFn": "options.clockTolerance", "goFn": "jwt.WithLeeway", "note": "both must fail"},
    {"id": "vc-ignoreexpiration", "fn": "verify",
     "args": V("b", now=NOW + 9999, ignoreExpiration=True, algorithms=["HS256"]),
     "upstreamFn": "options.ignoreExpiration", "goFn": "jwt.WithIgnoreExpiration"},
    {"id": "vc-nbf-future", "fn": "verify", "args": V("nbf", algorithms=["HS256"]),
     "upstreamFn": "NotBeforeError", "goFn": "jwt.ErrTokenNotValidYet", "note": "both must fail"},
    {"id": "vc-nbf-reached", "fn": "verify", "args": V("nbf", now=NOW + 9500, algorithms=["HS256"]),
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse"},
    {"id": "vc-nbf-tolerance", "fn": "verify",
     "args": V("nbf", now=NOW + 8990, clockTolerance=30, algorithms=["HS256"]),
     "upstreamFn": "options.clockTolerance", "goFn": "jwt.WithLeeway"},
    {"id": "vc-ignorenotbefore", "fn": "verify",
     "args": V("nbf", ignoreNotBefore=True, algorithms=["HS256"]),
     "upstreamFn": "options.ignoreNotBefore", "goFn": "jwt.WithIgnoreNotBefore"},
    {"id": "vc-aud-ok", "fn": "verify", "args": V("full", audience="aud-a", algorithms=["HS256"]),
     "upstreamFn": "options.audience", "goFn": "jwt.WithAudience"},
    {"id": "vc-aud-wrong", "fn": "verify", "args": V("full", audience="other", algorithms=["HS256"]),
     "upstreamFn": "options.audience", "goFn": "jwt.WithAudience", "note": "both must fail"},
    {"id": "vc-aud-array-claim-member", "fn": "verify",
     "args": V("audarr", audience="b", algorithms=["HS256"]),
     "upstreamFn": "options.audience", "goFn": "jwt.WithAudience",
     "note": "aud claim is an array; a single expected value must match a member"},
    {"id": "vc-aud-array-claim-nonmember", "fn": "verify",
     "args": V("audarr", audience="c", algorithms=["HS256"]),
     "upstreamFn": "options.audience", "goFn": "jwt.WithAudience", "note": "both must fail"},
    {"id": "vc-aud-option-array", "fn": "verify",
     "args": V("full", audience=["nope", "aud-a"], algorithms=["HS256"]),
     "upstreamFn": "options.audience (array)", "goFn": "—",
     "note": "upstream accepts an array of acceptable audiences; the port's WithAudience "
             "takes a single value"},
    {"id": "vc-aud-missing-claim", "fn": "verify",
     "args": V("b", audience="aud-a", algorithms=["HS256"]),
     "upstreamFn": "options.audience", "goFn": "jwt.WithAudience",
     "note": "both must fail: aud required by the option but absent from the token"},
    {"id": "vc-iss-ok", "fn": "verify", "args": V("full", issuer="iss-a", algorithms=["HS256"]),
     "upstreamFn": "options.issuer", "goFn": "jwt.WithIssuer"},
    {"id": "vc-iss-wrong", "fn": "verify", "args": V("full", issuer="other", algorithms=["HS256"]),
     "upstreamFn": "options.issuer", "goFn": "jwt.WithIssuer", "note": "both must fail"},
    {"id": "vc-iss-option-array", "fn": "verify",
     "args": V("full", issuer=["nope", "iss-a"], algorithms=["HS256"]),
     "upstreamFn": "options.issuer (array)", "goFn": "—",
     "note": "upstream accepts an array of acceptable issuers"},
    {"id": "vc-sub-ok", "fn": "verify", "args": V("full", subject="sub-a", algorithms=["HS256"]),
     "upstreamFn": "options.subject", "goFn": "jwt.WithSubject"},
    {"id": "vc-sub-wrong", "fn": "verify", "args": V("full", subject="other", algorithms=["HS256"]),
     "upstreamFn": "options.subject", "goFn": "jwt.WithSubject", "note": "both must fail"},
    {"id": "vc-jwtid-ok", "fn": "verify", "args": V("full", jwtid="jti-a", algorithms=["HS256"]),
     "upstreamFn": "options.jwtid", "goFn": "—",
     "note": "the port has no jti-equality parser option"},
    {"id": "vc-jwtid-wrong", "fn": "verify", "args": V("full", jwtid="other", algorithms=["HS256"]),
     "upstreamFn": "options.jwtid", "goFn": "—",
     "note": "upstream rejects a mismatched jti; the port cannot express this check, so "
             "the Go runner reports a failure for a different reason"},
    {"id": "vc-nonce-wrong", "fn": "verify", "args": V("full", nonce="n-1", algorithms=["HS256"]),
     "upstreamFn": "options.nonce", "goFn": "—",
     "note": "upstream rejects a missing/mismatched nonce; the port has no such option, so "
             "the Go runner reports a failure for a different reason"},
    {"id": "vc-maxage-ok", "fn": "verify",
     "args": V("noexp", now=NOW + 200, maxAge=300, algorithms=["HS256"]),
     "upstreamFn": "options.maxAge", "goFn": "jwt.WithMaxTokenAge"},
    {"id": "vc-maxage-exceeded", "fn": "verify",
     "args": V("noexp", now=NOW + 700, maxAge=300, algorithms=["HS256"]),
     "upstreamFn": "options.maxAge", "goFn": "jwt.WithMaxTokenAge", "note": "both must fail"},
    {"id": "vc-maxage-timespan", "fn": "verify",
     "args": V("noexp", now=NOW + 200, maxAge="1h", algorithms=["HS256"]),
     "upstreamFn": "options.maxAge (string timespan)", "goFn": "—",
     "note": "upstream parses 'ms' timespan strings; WithMaxTokenAge takes a Duration"},
    {"id": "vc-iat-in-future-accepted", "fn": "verify", "args": V("iatfuture", algorithms=["HS256"]),
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse",
     "note": "neither side checks iat unless asked (the port has WithIssuedAt)"},
    {"id": "vc-complete-false-shape", "fn": "verify",
     "args": [base["b"], HS, {"clockTimestamp": LATER, "algorithms": ["HS256"]}],
     "upstreamFn": "options.complete (default false)", "goFn": "jwt.Token.Claims",
     "note": "default shape is the payload alone"},
]
write("verify-claims", cases,
      "Claim and option validation surface, always with an explicit clock.")

# =================================================================== decode
plain = base["full"]
none_tok = mint("node", {"n": ({"iat": NOW}, {"kind": "none"}, {"algorithm": "none"})})["n"]
cases = [
    {"id": "dec-payload", "fn": "decode", "args": [plain, {}],
     "upstreamFn": "jwt.decode", "goFn": "jwt.ParseUnverified"},
    {"id": "dec-complete", "fn": "decode", "args": [plain, {"complete": True}],
     "upstreamFn": "jwt.decode options.complete", "goFn": "jwt.ParseUnverified"},
    {"id": "dec-unverified-bad-signature", "fn": "decode",
     "args": [plain.rsplit(".", 1)[0] + ".AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
              {"complete": True}],
     "upstreamFn": "jwt.decode", "goFn": "jwt.ParseUnverified",
     "note": "decode never checks the signature on either side"},
    {"id": "dec-no-signature", "fn": "decode",
     "args": [plain.rsplit(".", 1)[0] + ".", {"complete": True}],
     "upstreamFn": "jwt.decode", "goFn": "jwt.ParseUnverified"},
    {"id": "dec-alg-none", "fn": "decode", "args": [none_tok, {"complete": True}],
     "upstreamFn": "jwt.decode", "goFn": "jwt.ParseUnverified"},
    {"id": "dec-garbage", "fn": "decode", "args": ["not.a.token", {"complete": True}],
     "upstreamFn": "jwt.decode", "goFn": "jwt.ParseUnverified",
     "note": "upstream returns null; the port returns an error"},
    {"id": "dec-two-parts", "fn": "decode", "args": [plain.rsplit(".", 1)[0], {}],
     "upstreamFn": "jwt.decode", "goFn": "jwt.ParseUnverified",
     "note": "structurally invalid compact serialization"},
    {"id": "dec-empty", "fn": "decode", "args": ["", {}],
     "upstreamFn": "jwt.decode", "goFn": "jwt.ParseUnverified"},
]
write("decode", cases, "Signature-free inspection: jwt.decode vs jwt.ParseUnverified.")

# =================================================================== reject
b = base["b"]
h, p, s = b.split(".")
tp = json.loads(base64.urlsafe_b64decode(p + "=="))
tp["admin"] = True
p_t = base64.urlsafe_b64encode(json.dumps(tp, separators=(",", ":")).encode()).decode().rstrip("=")
# An HS256 token whose HMAC secret IS the RSA public-key PEM: the classic
# algorithm-confusion payload.
conf = mint("node", {"c": ({"exp": NOW + 3600, "iat": NOW, "sub": "attacker"},
                           {"kind": "oct", "file": "rsa-2048.pub.pem"},
                           {"algorithm": "HS256"})})["c"]

cases = [
    {"id": "rej-tampered-payload", "fn": "verify",
     "args": [".".join([h, p_t, s]), HS, vo(algorithms=["HS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse", "note": "both must fail"},
    {"id": "rej-tampered-header-alg", "fn": "verify",
     "args": [base64.urlsafe_b64encode(b'{"alg":"HS512","typ":"JWT"}').decode().rstrip("=")
              + "." + p + "." + s, HS, vo(algorithms=["HS256", "HS512"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse",
     "note": "both must fail: alg swapped under an HS256 signature"},
    {"id": "rej-wrong-key", "fn": "verify", "args": [b, HS_WRONG, vo(algorithms=["HS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse", "note": "both must fail"},
    {"id": "rej-empty-secret", "fn": "verify",
     "args": [b, {"kind": "oct", "secret": ""}, vo(algorithms=["HS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse", "note": "both must fail"},
    {"id": "rej-truncated-signature", "fn": "verify",
     "args": [b[:-6], HS, vo(algorithms=["HS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse", "note": "both must fail"},
    {"id": "rej-truncated-token", "fn": "verify", "args": [b[:20], HS, vo(algorithms=["HS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse", "note": "both must fail"},
    {"id": "rej-two-parts", "fn": "verify", "args": [h + "." + p, HS, vo(algorithms=["HS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse", "note": "both must fail"},
    {"id": "rej-four-parts", "fn": "verify", "args": [b + ".extra", HS, vo(algorithms=["HS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse", "note": "both must fail"},
    {"id": "rej-signature-stripped", "fn": "verify",
     "args": [h + "." + p + ".", HS, vo(algorithms=["HS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse",
     "note": "both must fail: empty signature on a signed alg"},
    {"id": "rej-empty-string", "fn": "verify", "args": ["", HS, vo(algorithms=["HS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse", "note": "both must fail"},
    {"id": "rej-non-base64-payload", "fn": "verify",
     "args": [h + ".!!!not-base64!!!." + s, HS, vo(algorithms=["HS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse", "note": "both must fail"},
    {"id": "rej-none-with-secret", "fn": "verify",
     "args": [none_tok, HS, vo(algorithms=["HS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.ErrNoneAlgDisallowed",
     "note": "both must fail: alg:none token offered to an HS256 verifier"},
    {"id": "rej-none-no-allowlist", "fn": "verify",
     "args": [none_tok, {"kind": "none"}, {"clockTimestamp": LATER, "complete": True}],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse",
     "note": "both must fail: 'none' must be opted into explicitly"},
    {"id": "rej-none-listed-but-key-given", "fn": "verify",
     "args": [none_tok, HS, vo(algorithms=["none"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse",
     "note": "both must fail: alg none presented with real key material"},
    {"id": "acc-none-explicit-optin", "fn": "verify",
     "args": [none_tok, {"kind": "none"}, vo(algorithms=["none"])],
     "upstreamFn": "jwt.verify algorithms:['none']", "goFn": "jwt.WithAllowNone",
     "note": "both accept, and only with an explicit opt-in"},
    {"id": "rej-expired", "fn": "verify", "args": [b, HS, vo(now=NOW + 99999, algorithms=["HS256"])],
     "upstreamFn": "TokenExpiredError", "goFn": "jwt.ErrTokenExpired", "note": "both must fail"},
    {"id": "rej-not-yet-valid", "fn": "verify", "args": [base["nbf"], HS, vo(algorithms=["HS256"])],
     "upstreamFn": "NotBeforeError", "goFn": "jwt.ErrTokenNotValidYet", "note": "both must fail"},
    {"id": "rej-wrong-audience", "fn": "verify",
     "args": [base["full"], HS, vo(audience="wrong", algorithms=["HS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse", "note": "both must fail"},
    {"id": "rej-wrong-issuer", "fn": "verify",
     "args": [base["full"], HS, vo(issuer="wrong", algorithms=["HS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse", "note": "both must fail"},
]
write("reject", cases,
      "The rejection surface. Every case must fail on BOTH sides, except "
      "acc-none-explicit-optin which must succeed on both.")

# =============================================================== algorithms
cases = [
    {"id": "alg-allowlist-match", "fn": "verify", "args": [b, HS, vo(algorithms=["HS256"])],
     "upstreamFn": "options.algorithms", "goFn": "jwt.WithValidMethods"},
    {"id": "alg-allowlist-multi", "fn": "verify",
     "args": [b, HS, vo(algorithms=["HS384", "HS256"])],
     "upstreamFn": "options.algorithms", "goFn": "jwt.WithValidMethods"},
    {"id": "alg-allowlist-excludes", "fn": "verify", "args": [b, HS, vo(algorithms=["HS384"])],
     "upstreamFn": "options.algorithms", "goFn": "jwt.WithValidMethods",
     "note": "both must fail: token alg is not in the allowlist"},
    {"id": "alg-allowlist-empty", "fn": "verify", "args": [b, HS, vo(algorithms=[])],
     "upstreamFn": "options.algorithms", "goFn": "jwt.WithValidMethods",
     "note": "both must fail: an empty allowlist permits nothing"},
    {"id": "alg-confusion-hs-token-rs-allowlist", "fn": "verify",
     "args": [conf, {"kind": "oct", "file": "rsa-2048.pub.pem"}, vo(algorithms=["RS256"])],
     "upstreamFn": "options.algorithms", "goFn": "jwt.WithValidMethods",
     "note": "both must fail: HS256 token forged with the RSA public-key PEM as the HMAC "
             "secret, offered to an RS256 verifier"},
    {"id": "alg-confusion-hs-token-rsa-pubkey-object", "fn": "verify",
     "args": [conf, RSA_PUB, vo(algorithms=["RS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse",
     "note": "both must fail: HS256 token, RSA public key"},
    {"id": "alg-confusion-hs-token-rsa-pubkey-no-allowlist", "fn": "verify",
     "args": [conf, RSA_PUB, {"clockTimestamp": LATER, "complete": True}],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse",
     "note": "both must fail: no allowlist, but an HS256 token must never verify against "
             "an RSA public key"},
    {"id": "alg-confusion-hs-token-pem-secret-no-allowlist", "fn": "verify",
     "args": [conf, {"kind": "oct", "file": "rsa-2048.pub.pem"},
              {"clockTimestamp": LATER, "complete": True}],
     "upstreamFn": "jwt.verify (infers algorithms from key material)", "goFn": "jwt.Parse",
     "note": "SECURITY: upstream inspects the key material, sees a PEM public key and "
             "restricts itself to RS*/PS*, rejecting the HS256 token. The port has no such "
             "inference, so with no allowlist it accepts the forged token."},
    {"id": "alg-rs256-token-hs256-allowlist", "fn": "verify",
     "args": [node_toks["x-rs256"], HS, vo(algorithms=["HS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse",
     "note": "both must fail: RS256 token offered to an HS256 verifier"},
    {"id": "alg-rs256-token-rsa-key-ps256-allowlist", "fn": "verify",
     "args": [node_toks["x-rs256"], RSA_PUB, vo(algorithms=["PS256"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse",
     "note": "both must fail: RS256 and PS256 share a key type but are distinct algorithms"},
    {"id": "alg-es256-token-es384-allowlist", "fn": "verify",
     "args": [node_toks["x-es256"], ec("p256"), vo(algorithms=["ES384"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse", "note": "both must fail"},
    {"id": "alg-unknown-in-header", "fn": "verify",
     "args": [base64.urlsafe_b64encode(b'{"alg":"HS129","typ":"JWT"}').decode().rstrip("=")
              + "." + p + "." + s, HS, vo(algorithms=["HS129"])],
     "upstreamFn": "jwt.verify", "goFn": "jwt.Parse",
     "note": "both must fail: unregistered alg"},
]
write("algorithms", cases, "Algorithm allowlisting and algorithm-confusion attacks.")
