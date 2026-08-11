'use strict';
// Upstream parity runner for jwt-decode@4.0.0 (auth0/jwt-decode), the tiny
// helper that reads a JWT payload WITHOUT verifying it.
//
// Speaks JSON Lines on stdio, per parity/HARNESS.md:
//   in : {"id":..,"fn":"decode"|"decodeHeader","args":["<token>"]}
//   out: {"id":..,"ok":true,"value":..} | {"id":..,"ok":false,"error":".."}
//
// Never exits on a failing case. Logs go to stderr only.

const readline = require('readline');
const { jwtDecode } = require('jwt-decode');

// Deep-sort object keys so both runners emit comparable JSON regardless of
// insertion order.
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

function call(fn, args) {
  switch (fn) {
    case 'decode':
      return jwtDecode(args[0]);
    case 'decodeHeader':
      return jwtDecode(args[0], { header: true });
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
