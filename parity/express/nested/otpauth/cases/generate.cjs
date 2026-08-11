'use strict';
// Regenerates the otpauth case files.
//
//   node cases/generate.cjs      (from parity/express/nested/otpauth)
//
// Everything is pinned: fixed base32 secrets, fixed millisecond timestamps and
// fixed HOTP counters, so no case depends on the wall clock. Tokens fed to the
// validate corpus were produced by the pinned upstream, which makes the
// accepting cases cross-implementation verification and the rest a rejection
// corpus (wrong code, wrong digits, wrong algorithm, outside the window).

const fs = require('fs');
const path = require('path');
const { HOTP, TOTP, Secret } = require(path.join(__dirname, '..', 'node', 'node_modules', 'otpauth'));

const UPSTREAM = 'otpauth@9.5.1';

// "Hello!\xde\xad\xbe\xef" — the canonical otpauth README secret.
const S = 'JBSWY3DPEHPK3PXP';
// The RFC 4226 / RFC 6238 test-vector secret "12345678901234567890".
const RFC = 'GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ';

// Fixed instants (milliseconds, as the npm API takes them).
const T0 = 0;
const T59 = 59000; // RFC 6238 vector 1
const T1234567890 = 1234567890000; // RFC 6238 vector
const TNOW = 1700000000000; // 2023-11-14T22:13:20Z, a fixed "now"

const c = (id, fn, args, note, deviation) => {
  const o = { id, fn, args };
  o.upstreamFn = {
    generate: 'TOTP#generate / HOTP#generate',
    validate: 'TOTP#validate / HOTP#validate',
    counter: 'TOTP.counter',
    uri: 'TOTP#toString / HOTP#toString',
    stringify: 'URI.stringify',
    parse: 'URI.parse',
    parseThenUri: 'URI.parse + toString',
    secretHex: 'Secret.fromBase32(...).hex',
  }[fn];
  o.goFn = {
    generate: 'Config.Generate',
    validate: 'Config.Validate',
    counter: 'Config.CounterAt',
    uri: 'otpauth.URL',
    stringify: 'otpauth.URL',
    parse: 'otpauth.Parse',
    parseThenUri: 'otpauth.Parse + otpauth.URL',
    secretHex: 'otpauth.DecodeSecret',
  }[fn];
  if (note) o.note = note;
  if (deviation) o.deviation = deviation;
  return o;
};

const totp = (over) => Object.assign({ type: 'totp', label: 'alice', secret: S, algorithm: 'SHA1', digits: 6, period: 30 }, over);
const hotp = (over) => Object.assign({ type: 'hotp', label: 'alice', secret: S, algorithm: 'SHA1', digits: 6, counter: 0 }, over);

const gen = (cfg, opts) => {
  const secret = Secret.fromBase32(cfg.secret);
  if ((cfg.type || 'totp') === 'hotp') {
    return HOTP.generate({ secret, algorithm: cfg.algorithm, digits: cfg.digits, counter: opts.counter !== undefined ? opts.counter : cfg.counter });
  }
  return TOTP.generate({ secret, algorithm: cfg.algorithm, digits: cfg.digits, period: cfg.period, timestamp: opts.timestamp });
};

// ------------------------------------------------------------------ codes

const generateCases = [
  // RFC 6238 appendix B vectors, SHA1.
  c('gen-rfc6238-t59-sha1', 'generate', [totp({ secret: RFC, digits: 8 }), { timestamp: T59 }], 'RFC 6238 vector: 94287082'),
  c('gen-rfc6238-t1111111109', 'generate', [totp({ secret: RFC, digits: 8 }), { timestamp: 1111111109000 }]),
  c('gen-rfc6238-t1234567890', 'generate', [totp({ secret: RFC, digits: 8 }), { timestamp: T1234567890 }]),
  c('gen-rfc6238-t2000000000', 'generate', [totp({ secret: RFC, digits: 8 }), { timestamp: 2000000000000 }]),
  c('gen-rfc6238-t59-sha256', 'generate', [totp({ secret: RFC, digits: 8, algorithm: 'SHA256' }), { timestamp: T59 }]),
  c('gen-rfc6238-t59-sha512', 'generate', [totp({ secret: RFC, digits: 8, algorithm: 'SHA512' }), { timestamp: T59 }]),
  c('gen-rfc6238-t1234567890-sha256', 'generate', [totp({ secret: RFC, digits: 8, algorithm: 'SHA256' }), { timestamp: T1234567890 }]),
  c('gen-rfc6238-t1234567890-sha512', 'generate', [totp({ secret: RFC, digits: 8, algorithm: 'SHA512' }), { timestamp: T1234567890 }]),
  c('gen-totp-epoch', 'generate', [totp({}), { timestamp: T0 }], 'counter 0'),
  c('gen-totp-fixed-now', 'generate', [totp({}), { timestamp: TNOW }]),
  c('gen-totp-one-step-later', 'generate', [totp({}), { timestamp: TNOW + 30000 }]),
  c('gen-totp-within-same-step', 'generate', [totp({}), { timestamp: TNOW + 29999 }], 'same 30s step, so the same code'),
  c('gen-totp-period-60', 'generate', [totp({ period: 60 }), { timestamp: TNOW }]),
  c('gen-totp-period-1', 'generate', [totp({ period: 1 }), { timestamp: TNOW }]),
  c('gen-totp-period-90', 'generate', [totp({ period: 90 }), { timestamp: TNOW }]),
  c('gen-totp-digits-4', 'generate', [totp({ digits: 4 }), { timestamp: TNOW }]),
  c('gen-totp-digits-7', 'generate', [totp({ digits: 7 }), { timestamp: TNOW }]),
  c('gen-totp-digits-8', 'generate', [totp({ digits: 8 }), { timestamp: TNOW }]),
  c('gen-totp-digits-10', 'generate', [totp({ digits: 10 }), { timestamp: TNOW }]),
  c('gen-totp-digits-12', 'generate', [totp({ digits: 12 }), { timestamp: TNOW }], 'past 10 digits the code is left-padded with zeros'),
  c('gen-totp-sha224', 'generate', [totp({ algorithm: 'SHA224' }), { timestamp: TNOW }]),
  c('gen-totp-sha384', 'generate', [totp({ algorithm: 'SHA384' }), { timestamp: TNOW }]),
  c('gen-totp-lowercase-algorithm', 'generate', [totp({ algorithm: 'sha256' }), { timestamp: TNOW }], 'algorithm names are case-insensitive'),
  c('gen-totp-lowercase-secret', 'generate', [totp({ secret: S.toLowerCase() }), { timestamp: TNOW }]),
  c('gen-totp-spaced-secret', 'generate', [totp({ secret: 'JBSW Y3DP EHPK 3PXP' }), { timestamp: TNOW }], 'the spaced form users copy from enrollment pages'),
  c('gen-totp-padded-secret', 'generate', [totp({ secret: 'JBSWY3DPEHPK3PXP====' }), { timestamp: TNOW }]),
  c('gen-totp-long-secret', 'generate', [totp({ secret: RFC + RFC }), { timestamp: TNOW }], 'a key longer than the SHA-1 block size'),
  // HOTP: counters, not clocks.
  c('gen-hotp-counter-0', 'generate', [hotp({}), { counter: 0 }], 'RFC 4226 appendix D vector: 755224'),
  c('gen-hotp-counter-0-rfc', 'generate', [hotp({ secret: RFC }), { counter: 0 }], 'RFC 4226 vector 0: 755224'),
  c('gen-hotp-counter-1-rfc', 'generate', [hotp({ secret: RFC }), { counter: 1 }], 'RFC 4226 vector 1: 287082'),
  c('gen-hotp-counter-5-rfc', 'generate', [hotp({ secret: RFC }), { counter: 5 }], 'RFC 4226 vector 5: 254676'),
  c('gen-hotp-counter-9-rfc', 'generate', [hotp({ secret: RFC }), { counter: 9 }], 'RFC 4226 vector 9: 520489'),
  c('gen-hotp-counter-from-config', 'generate', [hotp({ counter: 7 }), {}], 'the counter carried by the config'),
  c('gen-hotp-large-counter', 'generate', [hotp({ counter: 4294967296 }), {}], 'a counter past 2^32'),
  c('gen-hotp-digits-8', 'generate', [hotp({ secret: RFC, digits: 8, counter: 3 }), {}]),
  c('gen-hotp-sha512', 'generate', [hotp({ secret: RFC, algorithm: 'SHA512', counter: 3 }), {}]),
  // Failures.
  c('gen-bad-secret-chars', 'generate', [totp({ secret: 'not-base32!' }), { timestamp: TNOW }], 'must fail on both sides'),
  c('gen-bad-secret-digit-1', 'generate', [totp({ secret: 'ABC1' }), { timestamp: TNOW }], '1 is not in the base32 alphabet'),
  c('gen-unknown-algorithm', 'generate', [totp({ algorithm: 'MD5' }), { timestamp: TNOW }], 'must fail on both sides'),
  c('gen-sha3-algorithm', 'generate', [totp({ algorithm: 'SHA3-256' }), { timestamp: TNOW }],
    'upstream delegates to node crypto, which has SHA-3',
    'SHA-3 is outside the Go standard library, so the port does not implement the SHA3-* algorithms the URI scheme allows'),
  c('gen-empty-secret', 'generate', [totp({ secret: '' }), { timestamp: TNOW }],
    'upstream accepts an empty secret and HMACs an empty key',
    'the port rejects an empty secret (ErrInvalidSecret) rather than signing with a zero-length key'),
  c('gen-single-char-secret', 'generate', [totp({ secret: 'A' }), { timestamp: TNOW }],
    'a single base32 character carries no whole byte',
    'the port rejects an incomplete base32 block; upstream decodes it to zero bytes'),
];

// ------------------------------------------------------------- the counter

const counterCases = [
  c('counter-epoch', 'counter', [totp({}), { timestamp: T0 }]),
  c('counter-t59', 'counter', [totp({}), { timestamp: T59 }]),
  c('counter-fixed-now', 'counter', [totp({}), { timestamp: TNOW }]),
  c('counter-period-60', 'counter', [totp({ period: 60 }), { timestamp: TNOW }]),
  c('counter-period-1', 'counter', [totp({ period: 1 }), { timestamp: TNOW }]),
  c('counter-sub-second', 'counter', [totp({}), { timestamp: TNOW + 999 }], 'milliseconds are truncated'),
];

// ------------------------------------------------------------ the verdicts

const validCode = gen(totp({}), { timestamp: TNOW });
const prevCode = gen(totp({}), { timestamp: TNOW - 30000 });
const nextCode = gen(totp({}), { timestamp: TNOW + 30000 });
const farCode = gen(totp({}), { timestamp: TNOW + 300000 });
const hotpCode = (n) => gen(hotp({}), { counter: n });

const validateCases = [
  c('val-totp-exact', 'validate', [totp({}), { token: validCode, timestamp: TNOW, window: 1 }], 'delta 0'),
  c('val-totp-zero-window', 'validate', [totp({}), { token: validCode, timestamp: TNOW, window: 0 }]),
  c('val-totp-previous-step', 'validate', [totp({}), { token: prevCode, timestamp: TNOW, window: 1 }], 'delta -1'),
  c('val-totp-next-step', 'validate', [totp({}), { token: nextCode, timestamp: TNOW, window: 1 }], 'delta +1'),
  c('val-totp-previous-step-zero-window', 'validate', [totp({}), { token: prevCode, timestamp: TNOW, window: 0 }], 'outside the window: null'),
  c('val-totp-next-step-zero-window', 'validate', [totp({}), { token: nextCode, timestamp: TNOW, window: 0 }], 'outside the window: null'),
  c('val-totp-far-outside-window', 'validate', [totp({}), { token: farCode, timestamp: TNOW, window: 1 }], 'ten steps away: null'),
  c('val-totp-far-inside-wide-window', 'validate', [totp({}), { token: farCode, timestamp: TNOW, window: 10 }], 'delta +10'),
  c('val-totp-wrong-code', 'validate', [totp({}), { token: '000000', timestamp: TNOW, window: 1 }], 'must be null on both sides'),
  c('val-totp-code-off-by-one-digit', 'validate', [totp({}), { token: validCode.slice(0, -1) + ((Number(validCode.slice(-1)) + 1) % 10), timestamp: TNOW, window: 1 }], 'one digit changed: null'),
  c('val-totp-wrong-digit-count', 'validate', [totp({}), { token: validCode.slice(0, 5), timestamp: TNOW, window: 1 }], 'a 5-digit token against a 6-digit config: null'),
  c('val-totp-too-many-digits', 'validate', [totp({}), { token: validCode + '0', timestamp: TNOW, window: 1 }], 'null'),
  c('val-totp-empty-token', 'validate', [totp({}), { token: '', timestamp: TNOW, window: 1 }]),
  c('val-totp-non-numeric-token', 'validate', [totp({}), { token: 'abcdef', timestamp: TNOW, window: 1 }]),
  c('val-totp-code-of-other-period', 'validate', [totp({}), { token: gen(totp({ period: 60 }), { timestamp: TNOW }), timestamp: TNOW, window: 1 }], 'a 60s-period code against a 30s config'),
  c('val-totp-code-of-other-algorithm', 'validate', [totp({}), { token: gen(totp({ algorithm: 'SHA256' }), { timestamp: TNOW }), timestamp: TNOW, window: 1 }], 'a SHA256 code against a SHA1 config'),
  c('val-totp-code-of-other-secret', 'validate', [totp({}), { token: gen(totp({ secret: RFC }), { timestamp: TNOW }), timestamp: TNOW, window: 1 }], 'a code for a different secret'),
  c('val-totp-sha256-matching', 'validate', [totp({ algorithm: 'SHA256' }), { token: gen(totp({ algorithm: 'SHA256' }), { timestamp: TNOW }), timestamp: TNOW, window: 1 }]),
  c('val-totp-8-digit-matching', 'validate', [totp({ digits: 8 }), { token: gen(totp({ digits: 8 }), { timestamp: TNOW }), timestamp: TNOW, window: 1 }]),
  c('val-totp-leading-zero-code', 'validate', [totp({ secret: RFC, digits: 8 }), { token: gen(totp({ secret: RFC, digits: 8 }), { timestamp: T59 }), timestamp: T59, window: 0 }], 'the RFC vector 94287082'),
  c('val-totp-default-window', 'validate', [totp({}), { token: prevCode, timestamp: TNOW }], 'window defaults to 1 on both sides'),
  c('val-hotp-exact', 'validate', [hotp({ counter: 5 }), { token: hotpCode(5), window: 1 }], 'delta 0'),
  c('val-hotp-behind', 'validate', [hotp({ counter: 5 }), { token: hotpCode(4), window: 1 }], 'delta -1'),
  c('val-hotp-ahead', 'validate', [hotp({ counter: 5 }), { token: hotpCode(6), window: 1 }], 'delta +1'),
  c('val-hotp-ahead-3-window-3', 'validate', [hotp({ counter: 5 }), { token: hotpCode(8), window: 3 }], 'delta +3'),
  c('val-hotp-ahead-4-window-3', 'validate', [hotp({ counter: 5 }), { token: hotpCode(9), window: 3 }], 'outside the window: null'),
  c('val-hotp-zero-window-behind', 'validate', [hotp({ counter: 5 }), { token: hotpCode(4), window: 0 }], 'null'),
  c('val-hotp-wrong-code', 'validate', [hotp({ counter: 5 }), { token: '000000', window: 1 }]),
  c('val-hotp-counter-override', 'validate', [hotp({ counter: 0 }), { token: hotpCode(42), counter: 42, window: 0 }], 'the counter from the options wins'),
  c('val-hotp-totp-code', 'validate', [hotp({ counter: 5 }), { token: validCode, window: 1 }], 'a TOTP code against an HOTP config'),
  c('val-bad-secret', 'validate', [totp({ secret: '!!!' }), { token: '123456', timestamp: TNOW, window: 1 }], 'must fail on both sides'),
];

// -------------------------------------------------------------------- URIs

const uriCases = [
  c('uri-totp-basic', 'uri', [totp({ issuer: 'ACME Co', label: 'alice@example.com' })]),
  c('uri-totp-no-issuer', 'uri', [totp({ label: 'alice@example.com' })]),
  c('uri-totp-issuer-out-of-label', 'uri', [totp({ issuer: 'ACME Co', label: 'alice', omitIssuerFromLabel: true })]),
  c('uri-totp-defaults-written-out', 'uri', [{ type: 'totp', issuer: 'ACME', label: 'alice', secret: S }], 'algorithm, digits and period are always emitted'),
  c('uri-totp-sha256-8-60', 'uri', [totp({ issuer: 'ACME', label: 'alice', algorithm: 'SHA256', digits: 8, period: 60 })]),
  c('uri-totp-unicode-issuer', 'uri', [totp({ issuer: 'Ünïcøde Ltd', label: 'josé@example.com' })]),
  c('uri-totp-emoji-label', 'uri', [totp({ issuer: 'ACME', label: 'user-\u{1F510}' })]),
  c('uri-totp-space-and-colon', 'uri', [totp({ issuer: 'A: B', label: 'c: d' })], 'colons in the components must be escaped'),
  c('uri-totp-reserved-chars', 'uri', [totp({ issuer: "!'()*-._~", label: "a/b?c&d=e+f" })], 'encodeURIComponent leaves !\'()*-._~ alone'),
  c('uri-totp-empty-label', 'uri', [totp({ issuer: 'ACME', label: '' })]),
  c('uri-totp-empty-issuer', 'uri', [totp({ issuer: '', label: 'alice' })]),
  c('uri-totp-lowercase-secret', 'uri', [totp({ issuer: 'ACME', label: 'alice', secret: S.toLowerCase() })], 'the secret is canonicalised to upper case'),
  c('uri-totp-spaced-secret', 'uri', [totp({ issuer: 'ACME', label: 'alice', secret: 'JBSW Y3DP EHPK 3PXP' })]),
  c('uri-hotp-basic', 'uri', [hotp({ issuer: 'ACME', label: 'bob', counter: 5 })]),
  c('uri-hotp-counter-zero', 'uri', [hotp({ issuer: 'ACME', label: 'bob', counter: 0 })]),
  c('uri-hotp-large-counter', 'uri', [hotp({ issuer: 'ACME', label: 'bob', counter: 4294967296 })]),
  c('uri-hotp-sha512-8', 'uri', [hotp({ issuer: 'ACME', label: 'bob', algorithm: 'SHA512', digits: 8, counter: 3 })]),
  c('stringify-totp', 'stringify', [totp({ issuer: 'ACME', label: 'alice' })], 'URI.stringify is toString'),
];

// ------------------------------------------------------------------- parse

const parseUris = [
  ['parse-totp-full', 'otpauth://totp/ACME%20Co:alice@example.com?issuer=ACME%20Co&secret=JBSWY3DPEHPK3PXP&algorithm=SHA256&digits=8&period=60'],
  ['parse-totp-minimal', 'otpauth://totp/alice?secret=JBSWY3DPEHPK3PXP'],
  ['parse-totp-defaults-filled', 'otpauth://totp/ACME:alice?issuer=ACME&secret=JBSWY3DPEHPK3PXP'],
  ['parse-totp-issuer-only-in-query', 'otpauth://totp/alice?issuer=ACME&secret=JBSWY3DPEHPK3PXP'],
  ['parse-totp-issuer-conflict', 'otpauth://totp/LabelIssuer:alice?issuer=QueryIssuer&secret=JBSWY3DPEHPK3PXP', 'the query parameter wins'],
  ['parse-totp-encoded-colon', 'otpauth://totp/ACME%20Co%3Aalice?secret=JBSWY3DPEHPK3PXP'],
  ['parse-totp-space-after-colon', 'otpauth://totp/ACME: alice?secret=JBSWY3DPEHPK3PXP', 'a space after the label separator is dropped'],
  ['parse-totp-param-order', 'otpauth://totp/alice?digits=7&period=45&secret=JBSWY3DPEHPK3PXP&algorithm=SHA512', 'any parameter order'],
  ['parse-totp-uppercase-type', 'otpauth://TOTP/alice?secret=JBSWY3DPEHPK3PXP', 'the type is case-insensitive'],
  ['parse-totp-lowercase-algorithm', 'otpauth://totp/alice?secret=JBSWY3DPEHPK3PXP&algorithm=sha512'],
  ['parse-totp-lowercase-secret', 'otpauth://totp/alice?secret=jbswy3dpehpk3pxp'],
  ['parse-totp-padded-secret', 'otpauth://totp/alice?secret=JBSWY3DPEHPK3PXP%3D%3D%3D%3D'],
  ['parse-totp-plus-in-label', 'otpauth://totp/a+b?secret=JBSWY3DPEHPK3PXP', 'a plus is a literal plus, not a space'],
  ['parse-totp-plus-in-issuer', 'otpauth://totp/alice?issuer=A+B&secret=JBSWY3DPEHPK3PXP'],
  ['parse-totp-percent-encoded-issuer', 'otpauth://totp/alice?issuer=A%20%26%20B&secret=JBSWY3DPEHPK3PXP'],
  ['parse-totp-unicode', 'otpauth://totp/%C3%9Cn%C3%AFc%C3%B8de:jos%C3%A9?secret=JBSWY3DPEHPK3PXP'],
  ['parse-totp-empty-label', 'otpauth://totp/?secret=JBSWY3DPEHPK3PXP'],
  ['parse-totp-period-1', 'otpauth://totp/alice?secret=JBSWY3DPEHPK3PXP&period=1'],
  ['parse-totp-digits-10', 'otpauth://totp/alice?secret=JBSWY3DPEHPK3PXP&digits=10'],
  ['parse-hotp-full', 'otpauth://hotp/ACME:bob?issuer=ACME&secret=JBSWY3DPEHPK3PXP&algorithm=SHA1&digits=6&counter=5'],
  ['parse-hotp-counter-zero', 'otpauth://hotp/bob?secret=JBSWY3DPEHPK3PXP&counter=0'],
  ['parse-hotp-large-counter', 'otpauth://hotp/bob?secret=JBSWY3DPEHPK3PXP&counter=4294967296'],
  ['parse-hotp-no-counter', 'otpauth://hotp/bob?secret=JBSWY3DPEHPK3PXP', 'counter is mandatory for HOTP: must fail on both sides'],
  ['parse-hotp-bad-counter', 'otpauth://hotp/bob?secret=JBSWY3DPEHPK3PXP&counter=abc', 'must fail on both sides'],
  ['parse-hotp-ignores-period', 'otpauth://hotp/bob?secret=JBSWY3DPEHPK3PXP&counter=1&period=60'],
  ['parse-no-secret', 'otpauth://totp/alice?digits=6', 'secret is mandatory: must fail on both sides'],
  ['parse-empty-secret', 'otpauth://totp/alice?secret=', 'must fail on both sides'],
  ['parse-bad-secret', 'otpauth://totp/alice?secret=not-base32', 'must fail on both sides'],
  ['parse-unknown-type', 'otpauth://xotp/alice?secret=JBSWY3DPEHPK3PXP', 'must fail on both sides'],
  ['parse-wrong-scheme', 'https://totp/alice?secret=JBSWY3DPEHPK3PXP', 'must fail on both sides'],
  ['parse-no-scheme', 'totp/alice?secret=JBSWY3DPEHPK3PXP', 'must fail on both sides'],
  ['parse-empty-string', '', 'must fail on both sides'],
  ['parse-no-query', 'otpauth://totp/alice', 'must fail on both sides'],
  ['parse-bad-digits', 'otpauth://totp/alice?secret=JBSWY3DPEHPK3PXP&digits=abc', 'must fail on both sides'],
  ['parse-zero-digits', 'otpauth://totp/alice?secret=JBSWY3DPEHPK3PXP&digits=0', 'must fail on both sides'],
  ['parse-negative-digits', 'otpauth://totp/alice?secret=JBSWY3DPEHPK3PXP&digits=-6', 'must fail on both sides'],
  ['parse-zero-period', 'otpauth://totp/alice?secret=JBSWY3DPEHPK3PXP&period=0', 'must fail on both sides'],
  ['parse-negative-period', 'otpauth://totp/alice?secret=JBSWY3DPEHPK3PXP&period=-30', 'must fail on both sides'],
  ['parse-bad-algorithm', 'otpauth://totp/alice?secret=JBSWY3DPEHPK3PXP&algorithm=MD5', 'must fail on both sides'],
  ['parse-sha3-algorithm', 'otpauth://totp/alice?secret=JBSWY3DPEHPK3PXP&algorithm=SHA3-256', 'a legal URI algorithm the port cannot compute but must still parse'],
];

const parseCases = [
  ...parseUris.map(([id, uri, note]) => c(id, 'parse', [uri], note)),
  ...parseUris
    .filter(([id]) => !id.startsWith('parse-no-') && !id.startsWith('parse-bad-') && !id.startsWith('parse-empty-') &&
      !id.startsWith('parse-unknown-') && !id.startsWith('parse-wrong-') && !id.startsWith('parse-zero-') && !id.startsWith('parse-negative-'))
    .slice(0, 14)
    .map(([id, uri]) => c(id.replace('parse-', 'roundtrip-'), 'parseThenUri', [uri], 'parse then render: the canonical form of the same URI')),
];

// ----------------------------------------------------------------- secrets

const secretCases = [
  c('secret-readme', 'secretHex', [S]),
  c('secret-rfc', 'secretHex', [RFC]),
  c('secret-lowercase', 'secretHex', [S.toLowerCase()]),
  c('secret-spaced', 'secretHex', ['JBSW Y3DP EHPK 3PXP']),
  c('secret-padded', 'secretHex', ['JBSWY3DPEHPK3PXP====']),
  c('secret-mixed-case', 'secretHex', ['JbSwY3dPeHpK3PxP']),
  c('secret-eight-chars', 'secretHex', ['MFRGGZDF']),
  c('secret-invalid-char', 'secretHex', ['ABC!'], 'must fail on both sides'),
  c('secret-digit-one', 'secretHex', ['ABC1'], 'must fail on both sides'),
  c('secret-digit-zero', 'secretHex', ['ABC0'], 'must fail on both sides'),
  c('secret-empty', 'secretHex', [''],
    'upstream decodes an empty secret to zero bytes',
    'the port rejects an empty secret (ErrInvalidSecret) rather than returning a zero-length key'),
  c('secret-single-char', 'secretHex', ['A'],
    'a single base32 character is not a whole byte',
    'the port rejects an incomplete base32 block; upstream discards the partial byte'),
  c('secret-nine-chars', 'secretHex', ['MFRGGZDFA'],
    'nine characters leave a partial byte',
    'the port rejects an incomplete base32 block; upstream discards the partial byte'),
];

const write = (file, group, note, cases) => {
  fs.writeFileSync(
    path.join(__dirname, file),
    JSON.stringify({ group, upstream: UPSTREAM, note, cases }, null, 2) + '\n'
  );
  console.error(file + ': ' + cases.length + ' cases');
};

write('generate.json', 'generate',
  'Code generation over fixed secrets, fixed millisecond timestamps and fixed HOTP counters, including the RFC 4226 appendix D and RFC 6238 appendix B vectors. No case reads the clock.',
  generateCases);
write('counter.json', 'counter',
  'The RFC 6238 time step for a fixed timestamp, TOTP.counter / Config.CounterAt.',
  counterCases);
write('validate.json', 'validate',
  'The verification surface. Tokens were produced by the pinned upstream, so accepting cases are cross-implementation verification; the rest must be rejected on BOTH sides — wrong code, one digit changed, wrong digit count, wrong period, wrong algorithm, wrong secret, and codes outside the search window.',
  validateCases);
write('uri.json', 'uri',
  'Key-URI construction: parameter order, the defaults upstream always writes out, and encodeURIComponent escaping of issuers and labels.',
  uriCases);
write('parse.json', 'parse',
  'Key-URI parsing, including everything URI.parse refuses: a missing or non-base32 secret, an HOTP URI with no counter, a non-positive period or digits, an unknown algorithm, an unknown type and a wrong scheme.',
  parseCases);
write('secret.json', 'secret',
  'Base32 secret decoding, reported as lower-case hex.',
  secretCases);
