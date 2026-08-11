'use strict';
// Regenerates cases/sign.json and cases/unsign.json.
//
//   node cases/generate.cjs        (from parity/express/nested/cookiesignature)
//
// Signed inputs for the unsign corpus are produced by the pinned upstream, so
// those cases double as cross-implementation verification: the Go port must
// accept exactly the strings Node produced and reject every mutation of them.
// Nothing here depends on a clock or on randomness — cookie-signature is a pure
// HMAC over the value.

const fs = require('fs');
const path = require('path');
const cs = require(path.join(__dirname, '..', 'node', 'node_modules', 'cookie-signature'));

const UPSTREAM = 'cookie-signature@1.2.2';
const S = 'parity-cookie-secret-abc123';
const S2 = 'a-different-secret';
const NUL = '\u0000';

const c = (id, fn, args, note, deviation) => {
  const o = {
    id,
    fn,
    args,
    upstreamFn: 'cookieSignature.' + fn,
    goFn: 'cookiesignature.' + fn[0].toUpperCase() + fn.slice(1),
  };
  if (note) o.note = note;
  if (deviation) o.deviation = deviation;
  return o;
};

const signValues = [
  ['sign-simple', 's:sessionid123'],
  ['sign-empty', ''],
  ['sign-single-char', 'x'],
  ['sign-with-dots', 'a.b.c'],
  ['sign-leading-dot', '.leading'],
  ['sign-trailing-dot', 'trailing.'],
  ['sign-only-dot', '.'],
  ['sign-unicode', 'naïve-café-你好'],
  ['sign-emoji', 'session-\u{1F680}-id'],
  ['sign-newline', 'line1\nline2'],
  ['sign-tab', 'a\tb'],
  ['sign-nul', 'a' + NUL + 'b'],
  ['sign-spaces', '  padded  '],
  ['sign-long', 'x'.repeat(4096)],
  ['sign-base64ish', 'YWJjZGVmZw=='],
  ['sign-json', '{"a":1,"b":[2,3]}'],
  ['sign-url', 'https://example.com/a?b=c&d=e'],
  ['sign-percent', '%2Fslash%20space'],
  ['sign-high-latin1', 'ÿþý'],
  ['sign-surrogate-pair', '\u{1F4A9}\u{1F600}'],
];

const signCases = [
  ...signValues.map(([id, v]) => c(id, 'sign', [v, S])),
  c('sign-empty-secret', 'sign', ['value', ''], 'an empty secret is not null, so upstream still signs'),
  c('sign-unicode-secret', 'sign', ['value', 'sécret-你']),
  c('sign-long-secret', 'sign', ['value', 'k'.repeat(200)]),
  c('sign-other-secret', 'sign', ['value', S2]),
  c('sign-secret-with-nul', 'sign', ['value', 'a' + NUL + 'b']),
];

const good = (v, secret) => cs.sign(v, secret === undefined ? S : secret);
const sigOf = (t) => t.slice(t.lastIndexOf('.') + 1);
const flipLast = (t) => t.slice(0, -1) + (t.slice(-1) === 'A' ? 'B' : 'A');
const flipSigCase = (t) => {
  const i = t.lastIndexOf('.');
  const s = t.slice(i + 1);
  const f = s[0] === s[0].toUpperCase() ? s[0].toLowerCase() : s[0].toUpperCase();
  return t.slice(0, i + 1) + f + s.slice(1);
};

const unsignCases = [
  c('unsign-roundtrip', 'unsign', [good('s:sessionid123'), S], 'valid on both sides'),
  c('unsign-roundtrip-empty-value', 'unsign', [good(''), S], 'an empty value round-trips and must report success'),
  c('unsign-roundtrip-dots', 'unsign', [good('a.b.c'), S], 'only the final dot separates'),
  c('unsign-roundtrip-many-dots', 'unsign', [good('a.b.c.d.e'), S]),
  c('unsign-roundtrip-unicode', 'unsign', [good('naïve-café-你好'), S]),
  c('unsign-roundtrip-emoji', 'unsign', [good('session-\u{1F680}-id'), S]),
  c('unsign-roundtrip-newline', 'unsign', [good('line1\nline2'), S]),
  c('unsign-roundtrip-nul', 'unsign', [good('a' + NUL + 'b'), S]),
  c('unsign-roundtrip-long', 'unsign', [good('y'.repeat(1024)), S]),
  c('unsign-roundtrip-double-signed', 'unsign', [good(good('inner')), S], 'a signed value signed again'),
  c('unsign-roundtrip-unicode-secret', 'unsign', [good('value', 'sécret-你'), 'sécret-你']),
  c('unsign-roundtrip-empty-secret', 'unsign', [good('value', ''), '']),
  c('unsign-wrong-secret', 'unsign', [good('s:sessionid123'), S2], 'must fail on both sides'),
  c('unsign-tampered-value', 'unsign', [good('s:sessionid123').replace('sessionid123', 'sessionid124'), S], 'payload tampered under a good signature'),
  c('unsign-tampered-value-same-length', 'unsign', [good('admin=0'), S].map((v, i) => (i === 0 ? v.replace('admin=0', 'admin=1') : v)), 'privilege flag flipped: must fail on both sides'),
  c('unsign-tampered-sig-last-char', 'unsign', [flipLast(good('s:sessionid123')), S]),
  c('unsign-tampered-sig-case', 'unsign', [flipSigCase(good('s:x')), S], 'base64 is case sensitive'),
  c('unsign-truncated-sig', 'unsign', [good('s:sessionid123').slice(0, -5), S], 'signature too short'),
  c('unsign-extended-sig', 'unsign', [good('s:sessionid123') + 'AA', S], 'signature too long'),
  c('unsign-padded-sig', 'unsign', [good('s:x') + '=', S], 'upstream strips = padding, so a padded signature must not verify'),
  c('unsign-base64url-sig', 'unsign', [good('s:x').replace(/\+/g, '-').replace(/\//g, '_'), S], 'upstream emits standard base64, so the base64url variant must not verify'),
  c('unsign-sig-of-other-value', 'unsign', ['b.' + sigOf(good('a')), S], 'a genuine signature attached to a different value'),
  c('unsign-empty-sig', 'unsign', ['value.', S], 'empty signature'),
  c('unsign-no-dot', 'unsign', ['novalueatall', S], 'no separator at all'),
  c('unsign-empty-input', 'unsign', ['', S]),
  c('unsign-just-dot', 'unsign', ['.', S]),
  c('unsign-only-sig', 'unsign', [sigOf(good('x')), S], 'a bare signature with no value and no dot'),
  c('unsign-leading-dot-empty-value-sig', 'unsign', ['.' + sigOf(good('')), S], 'the empty-value signature, presented with its separator'),
  c('unsign-secret-as-value', 'unsign', [good(S), 's:sessionid123'], 'value and secret swapped: must fail on both sides'),
];

const write = (file, group, note, cases) => {
  fs.writeFileSync(
    path.join(__dirname, file),
    JSON.stringify({ group, upstream: UPSTREAM, note, cases }, null, 2) + '\n'
  );
  console.error(file + ': ' + cases.length + ' cases');
};

write('sign.json', 'sign',
  'Fixed secrets throughout. cookie-signature is a deterministic HMAC-SHA256 over the value, so no clock, key generation or randomness is involved.',
  signCases);
write('unsign.json', 'unsign',
  'The rejection corpus, which is what matters for a signature check: tampered values, mutated/truncated/re-encoded signatures, wrong secrets and malformed inputs must fail on BOTH sides. Every signed input was produced by the pinned upstream, so the positive cases double as cross-implementation verification.',
  unsignCases);
