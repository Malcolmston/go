// Regenerates the Authorization headers embedded in cases/digest.json.
//
//   node cases/generate-digest.js
//
// The values are committed into the case file so the harness has no build step
// and both runners are fed byte-identical input. Nothing here is random: the
// server nonce is pinned to the same literal both runners return from their
// nonce hook, and cnonce/nc are fixed.
'use strict';
const crypto = require('crypto');
const md5 = (s) => crypto.createHash('md5').update(s).digest('hex');

const NONCE = 'deadbeefdeadbeefdeadbeefdeadbeef';
const CNONCE = '0a4f113b';

function header({
  username = 'alice',
  password = 's3cr3t-alice',
  realm = 'Parity',
  nonce = NONCE,
  uri = '/digest',
  qop = 'auth',
  nc = '00000001',
  cnonce = CNONCE,
  method = 'GET',
}) {
  const ha1 = md5(`${username}:${realm}:${password}`);
  const ha2 = md5(`${method}:${uri}`);
  const response = qop
    ? md5([ha1, nonce, nc, cnonce, qop, ha2].join(':'))
    : md5(`${ha1}:${nonce}:${ha2}`);
  let s = `Digest username="${username}", realm="${realm}", nonce="${nonce}", uri="${uri}"`;
  if (qop) s += `, qop=${qop}, nc=${nc}, cnonce="${cnonce}"`;
  return s + `, response="${response}"`;
}

const fixtures = {
  'digest-correct-response': header({}),
  'digest-correct-response-noqop': header({ qop: null }),
  'digest-wrong-response': header({ password: 'wrong-password' }),
  'digest-unknown-user': header({ username: 'carol', password: 'whatever' }),
  'digest-replay-same-nonce-nc (both requests)': header({}),
  'digest-replay-incremented-nc (second request)': header({ nc: '00000002' }),
  'digest-uri-replayed-at-other-path': header({}),
  'digest-uri-bound-to-other-path': header({ uri: '/digest/other' }),
  'digest-wrong-realm': header({ realm: 'Wrong' }),
};
console.log(JSON.stringify(fixtures, null, 2));
