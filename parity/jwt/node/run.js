'use strict';
// Upstream parity runner for auth0/node-jsonwebtoken (jsonwebtoken@9.0.2).
//
// Speaks JSON Lines on stdio, per parity/HARNESS.md:
//   in : {"id":..,"fn":..,"args":[..]}
//   out: {"id":..,"ok":true,"value":..} | {"id":..,"ok":false,"error":".."}
//
// Never exits on a failing case. Logs go to stderr only.

const fs = require('fs');
const path = require('path');
const readline = require('readline');
const jwt = require('jsonwebtoken');

const KEYS_DIR = process.argv[2] || path.join(__dirname, '..', 'keys');

// ---------------------------------------------------------------- key specs
// {"kind":"oct","secret":"..."}          symmetric secret, literal
// {"kind":"oct","file":"rsa-2048.pub.pem"} symmetric secret whose bytes are a PEM
//                                          (the algorithm-confusion pattern)
// {"kind":"rsa"|"ec"|"ed","file":"..","private":true|false}
// {"kind":"none"}                        the unsecured "none" algorithm
function readKeyFile(name) {
  if (name.includes('..') || path.isAbsolute(name)) throw new Error('bad key file: ' + name);
  return fs.readFileSync(path.join(KEYS_DIR, name), 'utf8');
}

function resolveKey(spec) {
  if (spec === null || spec === undefined) return null;
  switch (spec.kind) {
    case 'none':
      return null;
    case 'oct':
      return spec.file !== undefined ? readKeyFile(spec.file) : spec.secret;
    case 'rsa':
    case 'ec':
    case 'ed':
      return readKeyFile(spec.file);
    default:
      throw new Error('unknown key kind: ' + JSON.stringify(spec && spec.kind));
  }
}

// ------------------------------------------------------------ normalisation
// Deep-sort object keys so both runners emit byte-comparable JSON.
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

function signOptions(o) {
  const opts = {};
  for (const k of ['algorithm', 'expiresIn', 'notBefore', 'audience', 'issuer',
                   'subject', 'jwtid', 'keyid', 'noTimestamp', 'header',
                   'mutatePayload', 'encoding', 'allowInsecureKeySizes',
                   'allowInvalidAsymmetricKeyTypes']) {
    if (o && o[k] !== undefined) opts[k] = o[k];
  }
  return opts;
}

function verifyOptions(o) {
  const opts = {};
  for (const k of ['algorithms', 'audience', 'issuer', 'subject', 'jwtid',
                   'nonce', 'clockTolerance', 'clockTimestamp', 'maxAge',
                   'ignoreExpiration', 'ignoreNotBefore', 'complete',
                   'allowInvalidAsymmetricKeyTypes']) {
    if (o && o[k] !== undefined) opts[k] = o[k];
  }
  // Always ask for the complete shape internally so header parity is visible;
  // `complete` from the case still decides the value we report.
  return opts;
}

function shape(out, complete) {
  if (complete) return { header: norm(out.header), payload: norm(out.payload) };
  return norm(out.payload);
}

function doVerify(token, keySpec, o) {
  const opts = verifyOptions(o);
  const complete = opts.complete === true;
  opts.complete = true;
  const out = jwt.verify(token, resolveKey(keySpec), opts);
  return shape(out, complete);
}

function doSign(payload, keySpec, o) {
  return jwt.sign(payload, resolveKey(keySpec), signOptions(o));
}

// ------------------------------------------------------------------- fns
const FNS = {
  // sign(payload, key, options) -> exact compact token string.
  // Only meaningful to compare byte-for-byte for deterministic algs (HS*, none).
  sign(args) {
    return doSign(args[0], args[1], args[2] || {});
  },

  // verify(token, key, options) -> {header,payload} (complete) or payload.
  verify(args) {
    return doVerify(args[0], args[1], args[2] || {});
  },

  // decode(token, options) -> {header,payload,signature} or payload.
  decode(args) {
    const o = args[1] || {};
    const out = jwt.decode(args[0], { complete: o.complete === true });
    if (out === null) return null;
    if (o.complete === true) {
      return { header: norm(out.header), payload: norm(out.payload) };
    }
    return norm(out);
  },

  // roundtrip(payload, signKey, verifyKey, signOptions, verifyOptions)
  // Sign then verify inside one runner. This is how randomised algorithms
  // (PS*, ES*) are compared: the token bytes differ, the outcome must not.
  roundtrip(args) {
    const token = doSign(args[0], args[1], args[3] || {});
    return doVerify(token, args[2], args[4] || {});
  },

  // tokenParts(payload, key, options) -> {header, payload, sigLen}
  // Compares the JOSE header and payload segments plus the signature length
  // without depending on randomised signature bytes.
  tokenParts(args) {
    const token = doSign(args[0], args[1], args[2] || {});
    const p = token.split('.');
    return {
      header: norm(JSON.parse(Buffer.from(p[0], 'base64url').toString('utf8'))),
      payload: norm(JSON.parse(Buffer.from(p[1], 'base64url').toString('utf8'))),
      sigLen: Buffer.from(p[2], 'base64url').length,
    };
  },
};

// -------------------------------------------------------------------- loop
const rl = readline.createInterface({ input: process.stdin, terminal: false });
rl.on('line', (line) => {
  line = line.trim();
  if (!line) return;
  let c;
  try {
    c = JSON.parse(line);
  } catch (e) {
    process.stdout.write(JSON.stringify({ id: null, ok: false, error: 'bad case json: ' + e.message }) + '\n');
    return;
  }
  let res;
  try {
    const fn = FNS[c.fn];
    if (!fn) throw new Error('unknown fn: ' + c.fn);
    res = { id: c.id, ok: true, value: norm(fn(c.args || [])) };
  } catch (e) {
    res = { id: c.id, ok: false, error: (e && e.message) ? e.message : String(e) };
  }
  process.stdout.write(JSON.stringify(res) + '\n');
});
