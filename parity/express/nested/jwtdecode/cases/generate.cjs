'use strict';
// Regenerates the jwtdecode case files.
//
//   node cases/generate.cjs      (from parity/express/nested/jwtdecode)
//
// jwt-decode performs no cryptography, so every token here is a hand-built,
// fixed string: no clock, no keys, no randomness. Signatures are arbitrary —
// what is under test is segment selection, base64url decoding and JSON parsing,
// including everything that must be REJECTED.

const fs = require('fs');
const path = require('path');

const UPSTREAM = 'jwt-decode@4.0.0';

const b64 = (s) => Buffer.from(s, 'utf8').toString('base64url');
const tok = (header, payload, sig) =>
  b64(JSON.stringify(header)) + '.' + (typeof payload === 'string' ? b64(payload) : b64(JSON.stringify(payload))) + '.' + (sig === undefined ? 'c2lnbmF0dXJl' : sig);

const c = (id, fn, token, note, deviation) => {
  const o = {
    id,
    fn,
    args: [token],
    upstreamFn: fn === 'decode' ? 'jwtDecode' : 'jwtDecode(token, {header: true})',
    goFn: fn === 'decode' ? 'jwtdecode.Decode' : 'jwtdecode.DecodeHeader',
  };
  if (note) o.note = note;
  if (deviation) o.deviation = deviation;
  return o;
};

const H = { alg: 'HS256', typ: 'JWT' };

// ---------------------------------------------------------------- payloads

const payloads = [
  ['simple', { sub: '1234567890', name: 'John Doe', iat: 1516239022 }],
  ['empty-object', {}],
  ['registered-claims', { iss: 'https://issuer.example', sub: 'user-1', aud: 'api', exp: 1700003600, nbf: 1700000000, iat: 1700000000, jti: 'id-1' }],
  ['aud-array', { aud: ['a', 'b', 'c'] }],
  ['nested', { user: { id: 7, roles: ['admin', 'ops'], meta: { active: true } } }],
  ['unicode', { name: 'naïve café 你好', city: 'Kraków' }],
  ['emoji', { mood: '\u{1F680}\u{1F510}' }],
  ['numbers', { int: 42, negative: -7, float: 1.5, zero: 0, exp: 1.0e10 }],
  ['big-number', { n: 9007199254740993 }],
  ['booleans-and-null', { yes: true, no: false, nothing: null }],
  ['empty-string-values', { a: '', b: ' ' }],
  ['deep-array', { list: [1, [2, [3, [4]]]] }],
  ['many-claims', Object.fromEntries(Array.from({ length: 50 }, (_, i) => ['k' + i, i]))],
  ['escaped-json', { quote: 'he said "hi"', backslash: 'a\\b', newline: 'a\nb', tab: 'a\tb' }],
  ['slash-and-plus', { s: '???????', p: '++++++' }],
];

const decodeCases = payloads.map(([n, p]) => c('decode-' + n, 'decode', tok(H, p)));

const headerCases = [
  c('header-hs256', 'decodeHeader', tok(H, { sub: 'x' })),
  c('header-with-kid', 'decodeHeader', tok({ alg: 'RS256', typ: 'JWT', kid: 'key-1' }, { sub: 'x' })),
  c('header-alg-none', 'decodeHeader', tok({ alg: 'none' }, { sub: 'x' }, '')),
  c('header-extra-params', 'decodeHeader', tok({ alg: 'HS512', typ: 'JWT', cty: 'JWT', crit: ['exp'] }, { sub: 'x' })),
  c('header-empty-object', 'decodeHeader', tok({}, { sub: 'x' })),
  c('header-unicode', 'decodeHeader', tok({ alg: 'HS256', note: 'héader' }, { sub: 'x' })),
];

// ------------------------------------------------------------- the rejects

const rejectCases = [
  c('reject-empty-string', 'decode', '', 'no segment #2 at all'),
  c('reject-single-segment', 'decode', b64(JSON.stringify({ sub: 'x' })), 'no dot, so no payload segment'),
  c('reject-two-segments', 'decode', b64(JSON.stringify(H)) + '.' + b64(JSON.stringify({ sub: 'x' })),
    'upstream reads part #2 without requiring a signature segment'),
  c('reject-four-segments', 'decode', tok(H, { sub: 'x' }) + '.extra',
    'upstream ignores anything past the part it needs'),
  c('reject-empty-payload-segment', 'decode', b64(JSON.stringify(H)) + '..c2ln', 'part #2 is empty'),
  c('reject-only-dots', 'decode', '..'),
  c('reject-one-dot', 'decode', '.'),
  c('reject-invalid-base64-char', 'decode', b64(JSON.stringify(H)) + '.!!!!.sig', 'not base64 at all'),
  c('reject-base64-bad-length', 'decode', b64(JSON.stringify(H)) + '.abcde.sig', 'length % 4 === 1 is impossible base64'),
  c('reject-standard-base64-payload', 'decode', b64(JSON.stringify(H)) + '.' + Buffer.from(JSON.stringify({ s: '?a/b+c?' })).toString('base64').replace(/=+$/, '') + '.sig',
    'a payload encoded with the standard (+/) alphabet rather than base64url'),
  c('reject-padded-payload', 'decode', b64(JSON.stringify(H)) + '.' + Buffer.from(JSON.stringify({ a: 1 })).toString('base64') + '.sig',
    'base64 with = padding retained'),
  c('reject-not-json', 'decode', b64(JSON.stringify(H)) + '.' + b64('not json at all') + '.sig'),
  c('reject-truncated-json', 'decode', b64(JSON.stringify(H)) + '.' + b64('{"sub":"x"') + '.sig'),
  c('reject-json-number-payload', 'decode', b64(JSON.stringify(H)) + '.' + b64('12345') + '.sig',
    'a bare JSON number is valid JSON but not an object',
    'the port returns map[string]interface{}, so a non-object payload is an error; upstream returns the bare JSON value'),
  c('reject-json-string-payload', 'decode', b64(JSON.stringify(H)) + '.' + b64('"just a string"') + '.sig',
    'a bare JSON string',
    'the port returns map[string]interface{}, so a non-object payload is an error; upstream returns the bare JSON value'),
  c('reject-json-array-payload', 'decode', b64(JSON.stringify(H)) + '.' + b64('[1,2,3]') + '.sig',
    'a JSON array',
    'the port returns map[string]interface{}, so a non-object payload is an error; upstream returns the bare JSON value'),
  c('reject-json-true-payload', 'decode', b64(JSON.stringify(H)) + '.' + b64('true') + '.sig',
    'a bare JSON boolean',
    'the port returns map[string]interface{}, so a non-object payload is an error; upstream returns the bare JSON value'),
  c('accept-json-null-payload', 'decode', b64(JSON.stringify(H)) + '.' + b64('null') + '.sig',
    'JSON null decodes to null on both sides'),
  c('reject-whitespace-json', 'decode', b64(JSON.stringify(H)) + '.' + b64('  {"a": 1}  ') + '.sig',
    'JSON parsers skip surrounding whitespace'),
  c('reject-header-missing-part', 'decodeHeader', '', 'no part #1'),
  c('reject-header-invalid-base64', 'decodeHeader', '!!!.' + b64(JSON.stringify({ sub: 'x' })) + '.sig'),
  c('reject-header-not-json', 'decodeHeader', b64('nope') + '.' + b64(JSON.stringify({ sub: 'x' })) + '.sig'),
  c('reject-header-two-segments', 'decodeHeader', b64(JSON.stringify(H)) + '.' + b64(JSON.stringify({ sub: 'x' })),
    'part #1 is present, so upstream succeeds'),
  c('reject-header-single-segment', 'decodeHeader', b64(JSON.stringify(H)),
    'part #1 is present even with no dots at all'),
  c('reject-leading-whitespace-token', 'decode', ' ' + tok(H, { sub: 'x' }), 'a leading space corrupts part #1 but not part #2'),
  c('reject-trailing-newline-token', 'decode', tok(H, { sub: 'x' }) + '\n', 'the newline lands in the signature segment'),
  c('reject-utf16-bom-payload', 'decode', b64(JSON.stringify(H)) + '.' + b64('﻿{"a":1}') + '.sig', 'a BOM before the JSON'),
  c('reject-double-encoded', 'decode', b64(JSON.stringify(H)) + '.' + b64(b64(JSON.stringify({ a: 1 }))) + '.sig',
    'base64 of base64: decodes cleanly but is not JSON'),
];

const write = (file, group, note, cases) => {
  fs.writeFileSync(
    path.join(__dirname, file),
    JSON.stringify({ group, upstream: UPSTREAM, note, cases }, null, 2) + '\n'
  );
  console.error(file + ': ' + cases.length + ' cases');
};

write('decode.json', 'decode',
  'Payload decoding over fixed tokens. jwt-decode never verifies anything, so signatures are arbitrary strings and nothing depends on a clock or a key.',
  decodeCases);
write('header.json', 'header',
  'Header decoding, the jwtDecode(token, {header: true}) path, which the port exposes as DecodeHeader.',
  headerCases);
write('reject.json', 'reject',
  'The rejection corpus: missing segments, empty segments, the wrong base64 alphabet, impossible base64 lengths, non-JSON and non-object payloads. Errors are cases too — what is compared is whether each side failed.',
  rejectCases);
