'use strict';
// Regenerates the jsonwebtoken case files.
//
//   node cases/generate.cjs      (from parity/express/nested/jsonwebtoken)
//
// Determinism is not optional here: every payload carries an explicit iat (so
// exp/nbf derived from expiresIn/notBefore are fixed), and every verify case
// passes clockTimestamp, so nothing depends on the wall clock. Secrets are
// fixed strings, and the asymmetric fixtures in ../fixtures are fixed PEMs.
//
// Tokens in the verify and reject corpora are minted by the pinned upstream, so
// the accepting cases are genuine cross-implementation verification and the
// rejecting ones are a real forgery corpus.

const fs = require('fs');
const path = require('path');
const jwt = require(path.join(__dirname, '..', 'node', 'node_modules', 'jsonwebtoken'));
const crypto = require('crypto');

const UPSTREAM = 'jsonwebtoken@9.0.3';
const FIXTURES = path.join(__dirname, '..', 'fixtures');

const SECRET = 'parity-hmac-secret-256-bits-long-enough';
const OTHER_SECRET = 'a-completely-different-secret-value';
const OCT = { kind: 'oct', secret: SECRET };
const OCT_OTHER = { kind: 'oct', secret: OTHER_SECRET };
const PUB_PEM_AS_SECRET = { kind: 'oct', file: 'rsa-2048.pub.pem' };
const EC_PEM_AS_SECRET = { kind: 'oct', file: 'ec-p256.pub.pem' };

// Fixed instants.
const IAT = 1700000000;   // 2023-11-14T22:13:20Z
const NOW = 1700000100;   // 100 seconds later: the pinned "current time"
const EXP = 1700003600;   // an hour after IAT
const PAST = 1600000000;  // long expired

// Sort object keys so the JSON in the case file — and therefore the byte order
// the two implementations sign — is the same on both sides. Go maps have no
// insertion order, so the port emits caller claims sorted; the case files match
// that by construction.
const sorted = (o) => {
  if (Array.isArray(o)) return o.map(sorted);
  if (o && typeof o === 'object') {
    const out = {};
    for (const k of Object.keys(o).sort()) out[k] = sorted(o[k]);
    return out;
  }
  return o;
};

const c = (id, fn, args, note, deviation) => {
  const o = { id, fn, args };
  o.upstreamFn = 'jwt.' + fn;
  o.goFn = 'jsonwebtoken.' + (fn === 'sign' ? 'Sign' : fn === 'verify' ? 'Verify' : 'Decode');
  if (note) o.note = note;
  if (deviation) o.deviation = deviation;
  return o;
};

const V = (over) => Object.assign({ clockTimestamp: NOW, algorithms: ['HS256'] }, over);

// ------------------------------------------------------------------- signing

const signCases = [
  c('sign-minimal', 'sign', [sorted({ iat: IAT }), OCT, {}], 'the smallest possible payload'),
  c('sign-empty-payload', 'sign', [{}, OCT, { noTimestamp: true }], 'no claims at all'),
  c('sign-sub-and-iat', 'sign', [sorted({ iat: IAT, sub: 'user-1' }), OCT, {}]),
  c('sign-hs384', 'sign', [sorted({ iat: IAT, sub: 'user-1' }), OCT, { algorithm: 'HS384' }]),
  c('sign-hs512', 'sign', [sorted({ iat: IAT, sub: 'user-1' }), OCT, { algorithm: 'HS512' }]),
  c('sign-expires-in-seconds', 'sign', [sorted({ iat: IAT }), OCT, { expiresIn: 3600 }], 'exp is measured from the payload iat'),
  c('sign-not-before-seconds', 'sign', [sorted({ iat: IAT }), OCT, { notBefore: 60 }]),
  c('sign-expires-and-not-before', 'sign', [sorted({ iat: IAT }), OCT, { expiresIn: 3600, notBefore: 60 }]),
  c('sign-negative-expires-in', 'sign', [sorted({ iat: IAT }), OCT, { expiresIn: -3600 }],
    'a negative expiresIn mints an already-expired token',
    'the port treats only a positive ExpiresIn as set, since a time.Duration has no "absent" value; upstream honours a negative one'),
  c('sign-zero-expires-in', 'sign', [sorted({ iat: IAT }), OCT, { expiresIn: 0 }],
    'zero means "expires at iat" upstream',
    'the port treats a zero ExpiresIn as "option not set", since a time.Duration has no "absent" value'),
  c('sign-issuer', 'sign', [sorted({ iat: IAT }), OCT, { issuer: 'https://issuer.example' }]),
  c('sign-subject', 'sign', [sorted({ iat: IAT }), OCT, { subject: 'user-1' }]),
  c('sign-audience-string', 'sign', [sorted({ iat: IAT }), OCT, { audience: 'api.example' }]),
  c('sign-audience-array', 'sign', [sorted({ iat: IAT }), OCT, { audience: ['api.example', 'admin.example'] }]),
  c('sign-audience-single-element-array', 'sign', [sorted({ iat: IAT }), OCT, { audience: ['api.example'] }],
    'upstream writes a one-element array as a JSON array',
    'SignOptions.Audience is a []string, so a single entry is written as a JSON string to match the common Node form audience: "x"; Node\'s audience: ["x"] — an array of exactly one — has no distinct Go spelling'),
  c('sign-jwtid', 'sign', [sorted({ iat: IAT }), OCT, { jwtid: 'token-id-1' }]),
  c('sign-keyid', 'sign', [sorted({ iat: IAT }), OCT, { keyid: 'key-2023' }], 'kid goes in the header'),
  c('sign-all-options', 'sign', [sorted({ iat: IAT }), OCT, {
    algorithm: 'HS512', expiresIn: 3600, notBefore: 60, audience: 'api.example',
    issuer: 'https://issuer.example', subject: 'user-1', jwtid: 'token-id-1', keyid: 'key-2023',
  }]),
  c('sign-no-timestamp', 'sign', [sorted({ sub: 'user-1' }), OCT, { noTimestamp: true }], 'no iat is stamped'),
  c('sign-no-timestamp-drops-explicit-iat', 'sign', [sorted({ iat: IAT, sub: 'user-1' }), OCT, { noTimestamp: true }],
    'noTimestamp removes an iat the caller supplied'),
  c('sign-no-timestamp-with-expires-in', 'sign', [sorted({ iat: IAT }), OCT, { noTimestamp: true, expiresIn: 3600 }],
    'iat is dropped but exp is still measured from it'),
  c('sign-explicit-exp-claim', 'sign', [sorted({ exp: EXP, iat: IAT }), OCT, {}], 'exp supplied as a claim, not an option'),
  c('sign-explicit-nbf-claim', 'sign', [sorted({ iat: IAT, nbf: IAT + 60 }), OCT, {}]),
  c('sign-extra-header-params', 'sign', [sorted({ iat: IAT }), OCT, { header: sorted({ cty: 'JWT', foo: 'bar' }) }]),
  c('sign-header-with-kid', 'sign', [sorted({ iat: IAT }), OCT, { keyid: 'k1', header: sorted({ cty: 'JWT' }) }]),
  c('sign-unicode-claims', 'sign', [sorted({ city: 'Kraków', iat: IAT, name: 'naïve café 你好' }), OCT, {}]),
  c('sign-nested-claims', 'sign', [sorted({ iat: IAT, user: sorted({ id: 7, meta: { active: true }, roles: ['admin', 'ops'] }) }), OCT, {}]),
  c('sign-numeric-claims', 'sign', [sorted({ float: 1.5, iat: IAT, negative: -7, zero: 0 }), OCT, {}]),
  c('sign-null-and-bool-claims', 'sign', [sorted({ iat: IAT, nothing: null, yes: true }), OCT, {}]),
  c('sign-unicode-secret', 'sign', [sorted({ iat: IAT }), { kind: 'oct', secret: 'sécret-你-🔑' }, {}]),
  c('sign-long-secret', 'sign', [sorted({ iat: IAT }), { kind: 'oct', secret: 'k'.repeat(300) }, {}],
    'a secret longer than the hash block size'),
  // Failures.
  c('sign-no-secret', 'sign', [sorted({ iat: IAT }), { kind: 'none' }, {}], 'must fail on both sides'),
  c('sign-empty-secret', 'sign', [sorted({ iat: IAT }), { kind: 'oct', secret: '' }, {}],
    'an empty HMAC key is a key everybody knows: must fail on both sides'),
  c('sign-unsupported-algorithm', 'sign', [sorted({ iat: IAT }), OCT, { algorithm: 'HS999' }], 'must fail on both sides'),
  c('sign-rs256-with-hmac-secret', 'sign', [sorted({ iat: IAT }), OCT, { algorithm: 'RS256' }],
    'an asymmetric algorithm with a symmetric key: must fail on both sides'),
  c('sign-alg-none', 'sign', [sorted({ iat: IAT }), OCT, { algorithm: 'none' }],
    'upstream can mint an unsigned token',
    'the port implements the HMAC family only and refuses the unsecured "none" algorithm outright'),
  c('sign-exp-claim-and-expires-in', 'sign', [sorted({ exp: EXP, iat: IAT }), OCT, { expiresIn: 3600 }],
    'the option would overwrite the claim: must fail on both sides'),
  c('sign-nbf-claim-and-not-before', 'sign', [sorted({ iat: IAT, nbf: IAT }), OCT, { notBefore: 60 }], 'must fail on both sides'),
  c('sign-aud-claim-and-audience', 'sign', [sorted({ aud: 'a', iat: IAT }), OCT, { audience: 'b' }], 'must fail on both sides'),
  c('sign-iss-claim-and-issuer', 'sign', [sorted({ iat: IAT, iss: 'a' }), OCT, { issuer: 'b' }], 'must fail on both sides'),
  c('sign-sub-claim-and-subject', 'sign', [sorted({ iat: IAT, sub: 'a' }), OCT, { subject: 'b' }], 'must fail on both sides'),
  c('sign-jti-claim-and-jwtid', 'sign', [sorted({ iat: IAT, jti: 'a' }), OCT, { jwtid: 'b' }], 'must fail on both sides'),
  c('sign-hs256-with-public-key-pem', 'sign', [sorted({ iat: IAT }), PUB_PEM_AS_SECRET, {}],
    'upstream happily HMACs with the bytes of a public key — which is how the algorithm-confusion forgery is built',
    'the port refuses asymmetric key material as an HMAC secret when SIGNING as well as when verifying; upstream only refuses it on verify, so a Node caller can mint exactly the forgery its own verify() rejects'),
];

// ----------------------------------------------------------------- verifying

const mint = (payload, opts, secret) => jwt.sign(payload, secret === undefined ? SECRET : secret, opts || {});

const T_PLAIN = mint({ iat: IAT, sub: 'user-1' });
const T_EXP = mint({ iat: IAT, exp: EXP, sub: 'user-1' });
const T_EXPIRED = mint({ iat: PAST, exp: PAST + 60, sub: 'user-1' });
const T_NBF_FUTURE = mint({ iat: IAT, nbf: NOW + 3600, sub: 'user-1' });
const T_HS384 = mint({ iat: IAT, sub: 'user-1' }, { algorithm: 'HS384' });
const T_HS512 = mint({ iat: IAT, sub: 'user-1' }, { algorithm: 'HS512' });
const T_AUD = mint({ iat: IAT, aud: 'api.example', sub: 'user-1' });
const T_AUD_ARRAY = mint({ iat: IAT, aud: ['api.example', 'admin.example'], sub: 'user-1' });
const T_ISS = mint({ iat: IAT, iss: 'https://issuer.example', sub: 'user-1' });
const T_JTI = mint({ iat: IAT, jti: 'token-id-1', sub: 'user-1' });
const T_KID = mint({ iat: IAT, sub: 'user-1' }, { keyid: 'key-2023' });
const T_OTHER_SECRET = mint({ iat: IAT, sub: 'attacker' }, {}, OTHER_SECRET);
const T_OLD_IAT = mint({ iat: NOW - 7200, sub: 'user-1' });

// Forgeries. Each is a token an attacker could actually build.
const b64 = (o) => Buffer.from(JSON.stringify(o)).toString('base64url');
const unsigned = (header, payload) => b64(header) + '.' + b64(payload) + '.';
const pubPem = fs.readFileSync(path.join(FIXTURES, 'rsa-2048.pub.pem'), 'utf8');
const ecPem = fs.readFileSync(path.join(FIXTURES, 'ec-p256.pub.pem'), 'utf8');
// The algorithm-confusion forgery: HMAC the token with the *public key bytes*.
const forgeHS = (payload, key, alg) => {
  const header = { alg: alg || 'HS256', typ: 'JWT' };
  const input = b64(header) + '.' + b64(payload);
  const mac = crypto.createHmac((alg || 'HS256').replace('HS', 'sha'), key).update(input).digest('base64url');
  return input + '.' + mac;
};
// mintRaw signs a payload upstream's own sign() would refuse to build (a
// non-numeric exp, for example), so the resulting token has a VALID signature
// and only the claim type is unusual.
const mintRaw = (payload) => forgeHS(payload, SECRET);
const T_CONFUSED_RSA = forgeHS({ iat: IAT, sub: 'attacker' }, pubPem);
const T_CONFUSED_EC = forgeHS({ iat: IAT, sub: 'attacker' }, ecPem);
const T_EMPTY_KEY_FORGERY = forgeHS({ iat: IAT, sub: 'attacker' }, '');
const T_ALG_NONE = unsigned({ alg: 'none', typ: 'JWT' }, { iat: IAT, sub: 'attacker' });
const T_ALG_NONE_MIXED_CASE = unsigned({ alg: 'NoNe', typ: 'JWT' }, { iat: IAT, sub: 'attacker' });
const tamper = (tok, from, to) => {
  const [h, p, s] = tok.split('.');
  const payload = JSON.parse(Buffer.from(p, 'base64url').toString('utf8'));
  payload[from] = to;
  return h + '.' + b64(payload) + '.' + s;
};
const swapHeaderAlg = (tok, alg) => {
  const [, p, s] = tok.split('.');
  return b64({ alg, typ: 'JWT' }) + '.' + p + '.' + s;
};

const verifyCases = [
  c('ver-plain', 'verify', [T_PLAIN, OCT, V({})], 'a genuine upstream-minted token'),
  c('ver-plain-complete', 'verify', [T_PLAIN, OCT, V({ complete: true })], 'header, payload and signature'),
  c('ver-with-exp-still-valid', 'verify', [T_EXP, OCT, V({})]),
  c('ver-hs384', 'verify', [T_HS384, OCT, V({ algorithms: ['HS384'] })]),
  c('ver-hs512', 'verify', [T_HS512, OCT, V({ algorithms: ['HS512'] })]),
  c('ver-multiple-algorithms-allowed', 'verify', [T_HS512, OCT, V({ algorithms: ['HS256', 'HS384', 'HS512'] })]),
  c('ver-no-algorithms-option', 'verify', [T_PLAIN, OCT, { clockTimestamp: NOW }],
    'no allowlist: upstream infers the HMAC family from the symmetric key'),
  c('ver-kid-header', 'verify', [T_KID, OCT, V({ complete: true })]),
  c('ver-audience-match', 'verify', [T_AUD, OCT, V({ audience: 'api.example' })]),
  c('ver-audience-array-claim-match', 'verify', [T_AUD_ARRAY, OCT, V({ audience: 'admin.example' })],
    'the aud claim is an array containing the expected value'),
  c('ver-issuer-match', 'verify', [T_ISS, OCT, V({ issuer: 'https://issuer.example' })]),
  c('ver-issuer-list-match', 'verify', [T_ISS, OCT, V({ issuer: ['other', 'https://issuer.example'] })]),
  c('ver-subject-match', 'verify', [T_PLAIN, OCT, V({ subject: 'user-1' })]),
  c('ver-jwtid-match', 'verify', [T_JTI, OCT, V({ jwtid: 'token-id-1' })]),
  c('ver-expired-but-within-tolerance', 'verify', [T_EXPIRED, OCT, V({ clockTimestamp: PAST + 90, clockTolerance: 60 })],
    'clockTolerance forgives a token that expired 30 seconds ago'),
  c('ver-expired-ignored', 'verify', [T_EXPIRED, OCT, V({ ignoreExpiration: true })]),
  c('ver-not-yet-valid-ignored', 'verify', [T_NBF_FUTURE, OCT, V({ ignoreNotBefore: true })]),
  c('ver-nbf-within-tolerance', 'verify', [T_NBF_FUTURE, OCT, V({ clockTolerance: 7200 })]),
  c('ver-maxage-within', 'verify', [T_PLAIN, OCT, V({ maxAge: 3600 })], 'the token is 100 seconds old'),
  c('ver-exp-exactly-now', 'verify', [T_EXP, OCT, V({ clockTimestamp: EXP })], 'exp is inclusive: expired at exp'),
  c('ver-exp-one-second-before', 'verify', [T_EXP, OCT, V({ clockTimestamp: EXP - 1 })]),
  c('ver-nbf-exactly-now', 'verify', [mint({ iat: IAT, nbf: NOW }), OCT, V({})], 'nbf is satisfied at nbf'),
];

const rejectCases = [
  c('rej-wrong-secret', 'verify', [T_PLAIN, OCT_OTHER, V({})], 'must fail on both sides'),
  c('rej-token-signed-with-other-secret', 'verify', [T_OTHER_SECRET, OCT, V({})], 'must fail on both sides'),
  c('rej-tampered-payload', 'verify', [tamper(T_PLAIN, 'sub', 'admin'), OCT, V({})],
    'privilege escalation attempt under a genuine signature'),
  c('rej-tampered-exp', 'verify', [tamper(T_EXPIRED, 'exp', 4000000000), OCT, V({})], 'expiry pushed out'),
  c('rej-tampered-header-alg', 'verify', [swapHeaderAlg(T_PLAIN, 'HS512'), OCT, V({ algorithms: ['HS256', 'HS512'] })],
    'alg swapped under an HS256 signature'),
  c('rej-alg-none', 'verify', [T_ALG_NONE, OCT, V({})], 'the unsecured algorithm: must be refused'),
  c('rej-alg-none-no-allowlist', 'verify', [T_ALG_NONE, OCT, { clockTimestamp: NOW }], 'must be refused with no allowlist too'),
  c('rej-alg-none-mixed-case', 'verify', [T_ALG_NONE_MIXED_CASE, OCT, { clockTimestamp: NOW }],
    'an alg of "NoNe" must not slip past a case-sensitive comparison'),
  c('rej-alg-none-in-allowlist', 'verify', [T_ALG_NONE, OCT, V({ algorithms: ['none'] })],
    'even asked for explicitly, an unsigned token presented with a key is refused'),
  c('rej-alg-confusion-rsa-pem-as-hmac-secret', 'verify', [T_CONFUSED_RSA, PUB_PEM_AS_SECRET, { clockTimestamp: NOW }],
    'ALGORITHM CONFUSION: the token is HMAC-signed with the bytes of the RSA public key, which anyone can read'),
  c('rej-alg-confusion-rsa-with-hs-allowlist', 'verify', [T_CONFUSED_RSA, PUB_PEM_AS_SECRET, V({})],
    'the same forgery with algorithms: ["HS256"] explicitly set'),
  c('rej-alg-confusion-ec-pem-as-hmac-secret', 'verify', [T_CONFUSED_EC, EC_PEM_AS_SECRET, { clockTimestamp: NOW }],
    'the same attack with an EC public key'),
  c('rej-alg-confusion-rs256-allowlist', 'verify', [T_CONFUSED_RSA, PUB_PEM_AS_SECRET, V({ algorithms: ['RS256'] })],
    'an RS256 allowlist must reject the HS256 forgery'),
  c('rej-genuine-token-against-public-key', 'verify', [T_PLAIN, PUB_PEM_AS_SECRET, V({})],
    'a real HS256 token checked against a public key must not verify'),
  c('rej-empty-allowlist', 'verify', [T_PLAIN, OCT, V({ algorithms: [] })],
    'an empty allowlist permits NOTHING: a computed-empty list must fail closed'),
  c('rej-empty-allowlist-hs512', 'verify', [T_HS512, OCT, V({ algorithms: [] })], 'must fail on both sides'),
  c('rej-algorithm-not-in-allowlist', 'verify', [T_HS512, OCT, V({ algorithms: ['HS256'] })], 'must fail on both sides'),
  c('rej-empty-issuer-list', 'verify', [T_ISS, OCT, V({ issuer: [] })],
    'an empty issuer list accepts nothing'),
  c('rej-expired', 'verify', [T_EXPIRED, OCT, V({})], 'exp is long past'),
  c('rej-expired-outside-tolerance', 'verify', [T_EXPIRED, OCT, V({ clockTolerance: 5 })]),
  c('rej-not-yet-valid', 'verify', [T_NBF_FUTURE, OCT, V({})], 'nbf is an hour in the future'),
  c('rej-maxage-exceeded', 'verify', [T_OLD_IAT, OCT, V({ maxAge: 60 })], 'the token is two hours old'),
  c('rej-maxage-without-iat', 'verify', [mint({ sub: 'user-1' }, { noTimestamp: true }), OCT, V({ maxAge: 60 })],
    'maxAge cannot be checked without an iat'),
  c('rej-wrong-audience', 'verify', [T_AUD, OCT, V({ audience: 'other.example' })]),
  c('rej-missing-audience-claim', 'verify', [T_PLAIN, OCT, V({ audience: 'api.example' })]),
  c('rej-wrong-issuer', 'verify', [T_ISS, OCT, V({ issuer: 'https://evil.example' })]),
  c('rej-issuer-not-in-list', 'verify', [T_ISS, OCT, V({ issuer: ['a', 'b'] })]),
  c('rej-missing-issuer-claim', 'verify', [T_PLAIN, OCT, V({ issuer: 'https://issuer.example' })]),
  c('rej-wrong-subject', 'verify', [T_PLAIN, OCT, V({ subject: 'user-2' })]),
  c('rej-wrong-jwtid', 'verify', [T_JTI, OCT, V({ jwtid: 'token-id-2' })]),
  c('rej-missing-jwtid-claim', 'verify', [T_PLAIN, OCT, V({ jwtid: 'token-id-1' })]),
  c('rej-string-exp', 'verify', [mintRaw({ iat: IAT, exp: '0' }), OCT, V({})],
    'exp as a string must not be silently ignored'),
  c('rej-null-exp', 'verify', [mintRaw({ iat: IAT, exp: null }), OCT, V({})],
    'a null exp under a valid signature'),
  c('rej-string-nbf', 'verify', [mintRaw({ iat: IAT, nbf: 'tomorrow' }), OCT, V({})], 'nbf as a string, under a valid signature'),
  c('rej-null-nbf', 'verify', [mintRaw({ iat: IAT, nbf: null }), OCT, V({})], 'nbf as null, under a valid signature'),
  c('rej-string-exp-valid-signature', 'verify', [mintRaw({ iat: IAT, exp: '4000000000' }), OCT, V({})],
    'a string exp that looks like a far-future timestamp must still be refused'),
  c('rej-two-segments', 'verify', [T_PLAIN.split('.').slice(0, 2).join('.'), OCT, V({})], 'not a JWT'),
  c('rej-four-segments', 'verify', [T_PLAIN + '.extra', OCT, V({})]),
  c('rej-one-segment', 'verify', [T_PLAIN.split('.')[0], OCT, V({})]),
  c('rej-empty-token', 'verify', ['', OCT, V({})]),
  c('rej-empty-signature', 'verify', [T_PLAIN.split('.').slice(0, 2).join('.') + '.', OCT, V({})],
    'a signature segment that is present but empty'),
  c('rej-truncated-signature', 'verify', [T_PLAIN.slice(0, -4), OCT, V({})]),
  c('rej-extended-signature', 'verify', [T_PLAIN + 'AA', OCT, V({})]),
  c('rej-malformed-base64url-header', 'verify', ['!!!.' + T_PLAIN.split('.').slice(1).join('.'), OCT, V({})]),
  c('rej-malformed-base64url-payload', 'verify', [T_PLAIN.split('.')[0] + '.!!!.' + T_PLAIN.split('.')[2], OCT, V({})]),
  c('rej-header-not-json', 'verify', [Buffer.from('nope').toString('base64url') + '.' + T_PLAIN.split('.').slice(1).join('.'), OCT, V({})]),
  c('rej-no-alg-header', 'verify', [b64({ typ: 'JWT' }) + '.' + b64({ iat: IAT }) + '.x', OCT, V({})]),
  c('rej-numeric-alg-header', 'verify', [b64({ alg: 256, typ: 'JWT' }) + '.' + b64({ iat: IAT }) + '.x', OCT, V({})]),
  c('rej-no-secret', 'verify', [T_PLAIN, { kind: 'none' }, V({})], 'must fail on both sides'),
  c('rej-empty-secret', 'verify', [T_PLAIN, { kind: 'oct', secret: '' }, V({})], 'must fail on both sides'),
  c('rej-empty-secret-forgery', 'verify', [T_EMPTY_KEY_FORGERY, { kind: 'oct', secret: '' }, V({})],
    'a token HMAC-signed with the empty key, checked with the empty key: an empty secret must never be usable'),
  c('rej-signature-base64-standard-alphabet', 'verify',
    [(() => { const [h, p, s] = T_PLAIN.split('.'); return h + '.' + p + '.' + Buffer.from(s, 'base64url').toString('base64'); })(), OCT, V({})],
    'the signature re-encoded with the +/= alphabet must not verify'),
];

// ------------------------------------------------------------------ decoding

const decodeCases = [
  c('dec-plain', 'decode', [T_PLAIN, {}]),
  c('dec-plain-complete', 'decode', [T_PLAIN, { complete: true }]),
  c('dec-kid-complete', 'decode', [T_KID, { complete: true }], 'reading kid to choose a key before verifying'),
  c('dec-expired-token', 'decode', [T_EXPIRED, {}], 'decode never checks time'),
  c('dec-forged-token', 'decode', [T_CONFUSED_RSA, {}], 'decode never checks the signature either'),
  c('dec-alg-none', 'decode', [T_ALG_NONE, { complete: true }]),
  c('dec-aud-array', 'decode', [T_AUD_ARRAY, {}]),
  c('dec-unicode', 'decode', [mint({ iat: IAT, name: 'naïve café 你好' }), {}]),
  c('dec-two-segments', 'decode', [T_PLAIN.split('.').slice(0, 2).join('.'), {}],
    'upstream returns null for a malformed token',
    'the port returns ErrInvalidToken where jwt.decode returns null; a Go caller checks the error'),
  c('dec-empty-token', 'decode', ['', {}],
    'upstream returns null',
    'the port returns ErrInvalidToken where jwt.decode returns null; a Go caller checks the error'),
  c('dec-malformed-base64', 'decode', ['!!!.' + T_PLAIN.split('.').slice(1).join('.'), { complete: true }],
    'upstream returns null',
    'the port returns ErrInvalidToken where jwt.decode returns null; a Go caller checks the error'),
  c('dec-non-json-payload', 'decode', [b64({ alg: 'HS256', typ: 'JWT' }) + '.' + Buffer.from('plain text').toString('base64url') + '.x', {}],
    'upstream returns the raw string when the payload is not JSON',
    'the port returns Claims (a map), so a non-object payload is an error rather than a bare value'),
];

const write = (file, group, note, cases) => {
  fs.writeFileSync(
    path.join(__dirname, file),
    JSON.stringify({ group, upstream: UPSTREAM, note, cases }, null, 2) + '\n'
  );
  console.error(file + ': ' + cases.length + ' cases');
};

write('sign.json', 'sign',
  'Token minting. Every payload carries an explicit iat, so expiresIn/notBefore produce fixed exp/nbf values and the token string is fully deterministic — the comparison is over the exact token bytes, which is what interoperability means.',
  signCases);
write('verify.json', 'verify',
  'Verification of tokens minted by the pinned upstream: cross-implementation verification. clockTimestamp is pinned in every case.',
  verifyCases);
write('reject.json', 'reject',
  'The forgery corpus, and the reason this harness exists. Algorithm confusion (an HS256 token HMAC-signed with the bytes of a public key), alg:none, an empty allowlist, an empty secret, tampered payloads and headers, expiry and nbf, claim mismatches and malformed tokens must ALL fail on both sides. A case where the port succeeds and upstream fails is a vulnerability, not a style difference.',
  rejectCases);
write('decode.json', 'decode',
  'The deliberately unauthenticated read: jwt.decode / Decode and DecodeComplete.',
  decodeCases);
