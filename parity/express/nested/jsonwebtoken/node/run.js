'use strict';
// Upstream parity runner for jsonwebtoken@9.0.3 (auth0/node-jsonwebtoken).
//
// Speaks JSON Lines on stdio, per parity/HARNESS.md:
//   in : {"id":..,"fn":..,"args":[..]}
//   out: {"id":..,"ok":true,"value":..} | {"id":..,"ok":false,"error":".."}
//
// Determinism: every verify case passes clockTimestamp, and every sign case
// carries an explicit iat, so no reply depends on the wall clock. Never exits on
// a failing case; logs go to stderr only.

const fs = require('fs');
const path = require('path');
const readline = require('readline');
const jwt = require('jsonwebtoken');

const FIXTURES = process.argv[2] || path.join(__dirname, '..', 'fixtures');

// ---------------------------------------------------------------- key specs
// {"kind":"oct","secret":"..."}             a literal symmetric secret
// {"kind":"oct","file":"rsa-2048.pub.pem"}  the bytes of a PEM used as the HMAC
//                                           secret — the algorithm-confusion
//                                           pattern
// {"kind":"none"}                           no key at all
function readFixture(name) {
  if (name.includes('..') || path.isAbsolute(name)) throw new Error('bad fixture: ' + name);
  return fs.readFileSync(path.join(FIXTURES, name), 'utf8');
}

function resolveKey(spec) {
  if (spec === null || spec === undefined) return null;
  switch (spec.kind) {
    case 'none':
      return null;
    case 'oct':
      return spec.file !== undefined ? readFixture(spec.file) : spec.secret;
    default:
      throw new Error('unknown key kind: ' + JSON.stringify(spec && spec.kind));
  }
}

// ------------------------------------------------------------ normalisation
function norm(v) {
  if (Array.isArray(v)) return v.map(norm);
  if (v && typeof v === 'object') {
    const out = {};
    for (const k of Object.keys(v).sort()) out[k] = norm(v[k]);
    return out;
  }
  if (typeof v === 'undefined') return null;
  return v;
}

const SIGN_KEYS = ['algorithm', 'expiresIn', 'notBefore', 'audience', 'issuer',
  'subject', 'jwtid', 'keyid', 'noTimestamp', 'header'];
const VERIFY_KEYS = ['algorithms', 'audience', 'issuer', 'subject', 'jwtid',
  'clockTolerance', 'clockTimestamp', 'maxAge', 'ignoreExpiration',
  'ignoreNotBefore'];

function pick(o, keys) {
  const out = {};
  for (const k of keys) if (o && o[k] !== undefined) out[k] = o[k];
  return out;
}

function call(fn, args) {
  switch (fn) {
    case 'sign':
      return jwt.sign(args[0], resolveKey(args[1]), pick(args[2], SIGN_KEYS));
    case 'verify': {
      const opts = pick(args[2], VERIFY_KEYS);
      const complete = args[2] && args[2].complete === true;
      const out = jwt.verify(args[0], resolveKey(args[1]), Object.assign({}, opts, { complete }));
      return out;
    }
    case 'decode': {
      const complete = args[1] && args[1].complete === true;
      return jwt.decode(args[0], { complete });
    }
    default:
      throw new Error('unknown fn: ' + fn);
  }
}

const rl = readline.createInterface({ input: process.stdin, terminal: false });
rl.on('line', (line) => {
  line = line.trim();
  if (!line) return;
  let req;
  try {
    req = JSON.parse(line);
  } catch (e) {
    process.stdout.write(JSON.stringify({ id: null, ok: false, error: 'bad request: ' + e.message }) + '\n');
    return;
  }
  let out;
  try {
    out = { id: req.id, ok: true, value: norm(call(req.fn, req.args || [])) };
  } catch (e) {
    out = { id: req.id, ok: false, error: String((e && e.message) || e) };
  }
  process.stdout.write(JSON.stringify(out) + '\n');
});
